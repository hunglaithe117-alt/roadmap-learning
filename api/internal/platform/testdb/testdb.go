// Package testdb provides PostgreSQL test helpers and temporary schema isolation.
package testdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	dbmigrate "langapp/internal/migrate"
)

func quoteLiteral(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `''`) + `'`
}

// MigrateLockKey is the database-wide advisory lock key serializing test schema setup.
const MigrateLockKey int64 = 20260928

// ExtSchema is the dedicated schema holding the pg_trgm extension across test runs.
const ExtSchema = "testext"

// AppDatabase is the production/development database name that tests must never touch.
const AppDatabase = "langapp"

// TestDatabase is the default test database name.
const TestDatabase = "langapp_test"

// DSNEnv is the environment variable containing the PostgreSQL DSN for tests.
const DSNEnv = "LANGAPP_TEST_POSTGRES_DSN"

// DSN returns the trimmed PostgreSQL test DSN from the environment.
func DSN() string { return strings.TrimSpace(os.Getenv(DSNEnv)) }

// Session giữ khoá advisory suốt đời 1 test.
//
// Cần dùng Session (thay vì `Open`) khi 1 test dựng NHIỀU schema — ví dụ test
// merge của context `sync` dựng "máy local" + "máy peer" trong cùng database.
// `Open` chỉ giữ khoá tới hết lần gọi, nên schema thứ hai tạo sau đó sẽ chạy
// `CREATE EXTENSION` mà không có khoá ⇒ race 23505/42704.
//
// Session holds the database-wide advisory lock and tracks active test schemas.
type Session struct {
	conn *sql.Conn
	db   *sql.DB
	dsn  string

	schemas []string
}

// Acquire acquires the advisory lock and verifies database isolation.
func Acquire(t testing.TB, ctx context.Context) *Session {
	t.Helper()
	dsn := requireDSN(t)
	conn, lockDB, err := connectLock(ctx, dsn)
	if err != nil {
		t.Fatalf("nắm khoá advisory test: %v", err)
	}
	s := &Session{conn: conn, db: lockDB, dsn: dsn}
	if err := s.checkIsolatedDB(ctx); err != nil {
		s.release()
		t.Fatal(err)
	}
	if err := s.ensureExtensionSchema(ctx); err != nil {
		s.release()
		t.Fatalf("dựng schema extension %s: %v", ExtSchema, err)
	}
	t.Cleanup(func() { s.release() })
	return s
}

func (s *Session) release() {
	if s.conn == nil {
		return
	}
	_, _ = s.conn.ExecContext(context.Background(),
		"SELECT pg_advisory_unlock($1)", MigrateLockKey)
	_ = s.conn.Close()
	_ = s.db.Close()
	s.conn = nil
	s.db = nil
}

// Open creates a temporary PostgreSQL schema, runs migrations, and registers cleanup.
func Open(t testing.TB, ctx context.Context) *gorm.DB {
	t.Helper()
	db, _ := OpenSchema(t, ctx)
	return db
}

// OpenSchema creates a temporary schema, runs migrations, and returns its name.
func OpenSchema(t testing.TB, ctx context.Context) (*gorm.DB, string) {
	t.Helper()
	return Acquire(t, ctx).OpenSchema(t, ctx)
}

// OpenBareSchema creates a temporary schema without running migrations.
func OpenBareSchema(t testing.TB, ctx context.Context) (*gorm.DB, string) {
	t.Helper()
	return Acquire(t, ctx).openSchema(t, ctx, false)
}

// OpenSchema creates an additional temporary schema within the session lock.
func (s *Session) OpenSchema(t testing.TB, ctx context.Context) (*gorm.DB, string) {
	t.Helper()
	return s.openSchema(t, ctx, true)
}

func (s *Session) openSchema(t testing.TB, ctx context.Context, migrate bool) (*gorm.DB, string) {
	t.Helper()
	if s.conn == nil {
		t.Fatalf("Session đã được buông khoá (test gọi Acquire ở cleanup)")
	}
	schema := fmt.Sprintf("t_test_%d", time.Now().UnixNano())

	s.requireIsolatedDB(t, ctx, s.exemptSchemas(schema)...)

	if _, err := s.conn.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("tạo schema test: %v", err)
	}
	s.schemas = append(s.schemas, schema)

	searchPath := schema + "," + ExtSchema
	db, err := openTestDB(ctx, WithSearchPath(s.dsn, searchPath))
	if err != nil {
		dropErr := s.dropSchema(ctx, schema)
		t.Fatalf("mở Postgres test: %v (dọn schema %s: %v)", err, schema, dropErr)
	}
	if migrate {
		if _, err := dbmigrate.Up(ctx, db); err != nil {
			s.drop(schema, db)
			t.Fatalf("chạy migration test: %v", err)
		}
	}

	sqlDB, err := db.DB()
	if err != nil {
		s.drop(schema, db)
		t.Fatalf("lấy sql.DB: %v", err)
	}
	t.Cleanup(func() {
		s.drop(schema, db)
		_ = sqlDB.Close()
	})
	return db, schema
}

func (s *Session) exemptSchemas(extra ...string) []string {
	out := make([]string, 0, len(s.schemas)+len(extra))
	out = append(out, extra...)
	out = append(out, s.schemas...)
	return out
}

func (s *Session) untrackSchema(schema string) {
	for i, name := range s.schemas {
		if name == schema {
			s.schemas = append(s.schemas[:i], s.schemas[i+1:]...)
			return
		}
	}
}

// ensureExtensionSchema ensures ExtSchema and pg_trgm exist in the test database.
func (s *Session) ensureExtensionSchema(ctx context.Context) error {
	if s.conn == nil {
		return errors.New("không có conn để tạo schema extension")
	}
	if err := s.checkIsolatedDB(ctx, s.exemptSchemas()...); err != nil {
		return fmt.Errorf("từ chối dựng schema %s: %w", ExtSchema, err)
	}

	var currentSchema string
	err := s.conn.QueryRowContext(ctx,
		"SELECT n.nspname FROM pg_extension e "+
			"JOIN pg_namespace n ON n.oid = e.extnamespace "+
			"WHERE e.extname = 'pg_trgm'").Scan(&currentSchema)
	switch {
	case err == nil && currentSchema != ExtSchema:
		return fmt.Errorf(
			"extension pg_trgm đang nằm ở schema %q chứ không phải %q — "+
				"nếu đó là `public` thì search_path test sẽ phải kéo `public` vào "+
				"và đọc/xoá nhầm bảng thật của app. Sửa: "+
				"`ALTER EXTENSION pg_trgm SET SCHEMA %s;`",
			currentSchema, ExtSchema, ExtSchema)
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("tra extension pg_trgm: %w", err)
	}

	if _, err := s.conn.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS "+ExtSchema); err != nil {
		return fmt.Errorf("tạo schema %s: %w", ExtSchema, err)
	}
	if _, err := s.conn.ExecContext(ctx,
		"CREATE EXTENSION IF NOT EXISTS pg_trgm SCHEMA "+ExtSchema); err != nil {
		return fmt.Errorf("tạo extension pg_trgm trong %s: %w", ExtSchema, err)
	}
	return nil
}

// appTableNames lists application table names checked by the isolation guard.
var appTableNames = []string{
	"schema_migrations",
	"decks",
	"cards",
	"dict",
	"en_dict",
	"reviews",
	"notes",
	"sync_meta",
	"sync_conflicts",
	"roadmap_paths",
	"roadmap_stages",
	"roadmap_topics",
	"roadmap_milestones",
	"roadmap_resources",
	"roadmap_bookmarks",
}

// requireIsolatedDB fails if the DSN points to a database containing application tables.
func (s *Session) requireIsolatedDB(t testing.TB, ctx context.Context, exempt ...string) {
	t.Helper()
	if err := s.checkIsolatedDB(ctx, exempt...); err != nil {
		t.Fatal(err)
	}
}

// checkIsolatedDB verifies that the active database contains no non-exempt application tables.
func (s *Session) checkIsolatedDB(ctx context.Context, exempt ...string) error {
	if s.conn == nil {
		return errors.New(
			"không có conn để kiểm tra cách ly — không thể bảo đảm an toàn dữ liệu, " +
				"nên phải dừng thay vì đoán")
	}

	// Lớp 1 — tên database.
	var dbName string
	if err := s.conn.QueryRowContext(ctx, "SELECT current_database()").Scan(&dbName); err != nil {
		return fmt.Errorf("đọc tên database hiện tại: %w", err)
	}
	if dbName == AppDatabase {
		return errors.New(isolationMsg(dbName,
			[]string{fmt.Sprintf("tên database là %q — đúng tên database ứng dụng", AppDatabase)}))
	}

	// Lớp 2 — bằng chứng cấu trúc. So bảng app nào đang nằm ở schema KHÁC
	// schema test sẽ dùng.
	//
	// Dựng `IN (...)` từ `appTableNames` bằng `quoteLiteral` thay vì truyền
	// tham số: tên bảng là HẰNG SỐ compile-time trong chính file này (xem
	// khai báo), nên về mặt an toàn là tương đương — nhưng `quoteLiteral` từng
	// tên một thì không còn đường nào để sau này đổi `appTableNames` thành
	// biến động mà quên escape.
	//
	// ⚠️ PHẢI là `quoteLiteral` (chuỗi `'...'`) chứ KHÔNG phải `quoteIdent`
	// (định danh `"..."`). Ở vị trí `table_name IN (...)` đây là danh sách
	// GIÁ TRỊ, nên dùng `"..."` sẽ bị Postgres hiểu là tên CỘT → lỗi
	// `column "schema_migrations" does not exist` (42703). Lỗi này đã xảy ra
	// thật khi viết hàm đầu tiên — nhớ để tránh.
	quoted := make([]string, 0, len(appTableNames))
	for _, name := range appTableNames {
		quoted = append(quoted, quoteLiteral(name))
	}
	// Loại trừ schema exempt bằng `NOT (table_schema = ANY($n))` với THAM SỐ
	// thật, không dựng mảng thủ công: `= ANY($1)` với 0 phần tử trả NULL mà
	// `NOT NULL` là NULL ⇒ dòng bị loại ⇒ KHÔNG exempt gì, đúng ý.
	//
	// Dùng `NOT (x = ANY(...))` chứ không phải `x <> ALL(...)`: hai toán tử này
	// KHÁC nhau khi mảng rỗng (`ALL` trả true ⇒ mọi dòng bị loại — ngược hẳn).
	// Dùng nhầm ở đây là guard im lặng cho qua mọi thứ, tức hỏng theo đúng kiểu
	// ta đang diệt.
	args := make([]any, 0, len(exempt))
	conds := make([]string, 0, len(exempt))
	for i, schema := range exempt {
		conds = append(conds, fmt.Sprintf("$%d", i+1))
		args = append(args, schema)
	}
	notExempt := "TRUE"
	if len(conds) > 0 {
		notExempt = "NOT (table_schema = ANY(ARRAY[" + strings.Join(conds, ", ") + "]))"
	}
	rows, err := s.conn.QueryContext(ctx,
		"SELECT DISTINCT table_schema, table_name FROM information_schema.tables "+
			"WHERE table_name IN ("+strings.Join(quoted, ", ")+") "+
			"AND "+notExempt,
		args...)
	if err != nil {
		return fmt.Errorf("tra cứu bảng app để kiểm tra cách ly: %w", err)
	}
	defer rows.Close()
	var found []string
	for rows.Next() {
		var schema, table string
		if err := rows.Scan(&schema, &table); err != nil {
			return fmt.Errorf("đọc kết quả kiểm tra cách ly: %w", err)
		}
		found = append(found, schema+"."+table)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("duyệt kết quả kiểm tra cách ly: %w", err)
	}
	if len(found) > 0 {
		sort.Strings(found)
		return errors.New(isolationMsg(dbName, found))
	}
	return nil
}

// CheckIsolatedDB exposes checkIsolatedDB for external test packages.
func CheckIsolatedDB(ctx context.Context, s *Session, exempt ...string) error {
	return s.checkIsolatedDB(ctx, exempt...)
}

// EnsureExtensionSchemaForTest exposes ensureExtensionSchema for isolation testing.
func EnsureExtensionSchemaForTest(ctx context.Context, s *Session) error {
	return s.ensureExtensionSchema(ctx)
}

// NewSessionForTest wraps a connection into a Session for testing.
func NewSessionForTest(conn *sql.Conn) *Session {
	return &Session{conn: conn}
}

func isolationMsg(dbName string, evidence []string) string {
	shown := evidence
	const maxShown = 8
	if len(shown) > maxShown {
		shown = append(append([]string{}, shown[:maxShown]...),
			fmt.Sprintf("… (+%d bảng nữa)", len(evidence)-maxShown))
	}
	return fmt.Sprintf(
		"⛔ DSN test trỏ vào database ĐANG CHỨA DỮ LIỆU APP — dừng trước khi tạo schema test.\n"+
			"     database hiện tại : %s\n"+
			"     bằng chứng cấu trúc: %d bảng của app nằm NGOÀI schema test: %v\n\n"+
			"     Test KHÔNG ĐỤNG bảng này được, nhưng nếu không dừng, `search_path` sẽ khiến\n"+
			"     `DROP TABLE` trong test rơi xuống bảng THẬT và xoá dữ liệu học của bạn.\n\n"+
			"     Sửa: chạy `make test` (đã trỏ sẵn database %q), hoặc tự tạo database test:\n"+
			"       docker compose exec -T postgres psql -U langapp -d postgres -c 'CREATE DATABASE \"%s\"'\n"+
			"     rồi trỏ LANGAPP_TEST_POSTGRES_DSN sang database đó.",
		dbName, len(evidence), shown, TestDatabase, TestDatabase)
}

// WithSearchPath sets the search_path query parameter in the DSN, overwriting existing values.
func WithSearchPath(dsn, schemas string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" || u.Host == "" {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		return dsn + sep + "search_path=" + schemas
	}
	q := u.Query()
	q.Set("search_path", schemas)
	u.RawQuery = q.Encode()
	return u.String()
}

// AllowSkipEnv is the environment variable that permits skipping tests when DSN is unset.
const AllowSkipEnv = "ALLOW_SKIP_DB_TESTS"

// Main provides a standard TestMain for packages requiring PostgreSQL.
func Main(m *testing.M) {
	if DSN() == "" && !SkipAllowed() {
		fmt.Fprintf(os.Stderr, `
✗ %s chưa set ⇒ test cần Postgres KHÔNG CHẠY ĐƯỢC, và đây là FAIL có chủ đích.

Lý do: "0 FAIL" khi 1/3 suite im lặng skip là con số xanh giả — đã giấu bug
mất dữ liệu ở chiều ghi suốt 2 phase trước đó.

Sửa: dựng Postgres rồi chạy lại
  docker compose up -d postgres
  make test          # tự trỏ DSN về database test 'langapp_test'

Hoặc nếu CỐ Ý không có Postgres (chỉ muốn chạy test không cần DB):
  ALLOW_SKIP_DB_TESTS=1 go test ./...
`, DSNEnv)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// SkipAllowed reports whether database tests are allowed to skip when DSN is unset.
func SkipAllowed() bool { return os.Getenv(AllowSkipEnv) == "1" }

func requireDSN(t testing.TB) string {
	t.Helper()
	dsn := DSN()
	if dsn == "" {
		if !SkipAllowed() {
			t.Fatalf("%s chưa set (và %s chưa cho phép skip) — "+
				"chạy `make test` hoặc `ALLOW_SKIP_DB_TESTS=1 go test ./...`",
				DSNEnv, AllowSkipEnv)
		}
		t.Skipf("%s chưa set — dựng `docker compose up -d postgres` rồi chạy lại", DSNEnv)
	}
	return dsn
}

// connectLock establishes a dedicated connection and acquires the advisory lock.
func connectLock(ctx context.Context, dsn string) (*sql.Conn, *sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, nil, err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", MigrateLockKey); err != nil {
		_ = conn.Close()
		_ = db.Close()
		return nil, nil, err
	}
	return conn, db, nil
}

// dropSchema drops a schema using the advisory lock connection.
func (s *Session) dropSchema(ctx context.Context, schema string) error {
	if s.conn == nil {
		return errors.New("không có conn của khoá để dọn schema")
	}
	_, err := s.conn.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE")
	if err == nil {
		s.untrackSchema(schema)
	}
	return err
}

// drop drops a test schema using the GORM pool.
func (s *Session) drop(schema string, db *gorm.DB) {
	if db == nil {
		return
	}
	_ = db.Exec("DROP SCHEMA " + schema + " CASCADE")
	s.untrackSchema(schema)
}

