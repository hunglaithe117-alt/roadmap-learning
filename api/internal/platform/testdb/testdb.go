// Package testdb là helper Postgres cho test — file `.go` CỐ Ý, không phải
// `_test.go`.
//
// Lý do: hằng số và hàm trong `_test.go` không import được qua package khác.
// M1/M2 nhân bản `TestMain` + `newTestDB` + hằng `migrateLockKey` ở 3 package
// test; M3 thêm 4 package nữa thì 7 bản sao là 7 chỗ phải sửa cùng lúc. Gói
// helper vào package thường giữ được 1 bản duy nhất.
package testdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	dbmigrate "langapp/internal/migrate"
)

// MigrateLockKey là khoá advisory toàn database dùng để serialize việc dựng +
// dọn schema test giữa các package chạy SONG SONG (`go test ./...` chạy
// package song song, theo `-p` = số CPU).
//
// Vì sao cần: migration `00002_fts.sql` chạy `CREATE EXTENSION pg_trgm`, mà
// extension là đối tượng cấp DATABASE được cài vào schema đầu tiên của
// `search_path` — tức schema test tạm. Hai hệ quả khi chạy song song:
//  1. `CREATE EXTENSION` chồng nhau → `duplicate key ... pg_extension_name_index`
//     (SQLSTATE 23505).
//  2. `DROP SCHEMA ... CASCADE` ở cleanup của package A giật extension ra khỏi
//     database giữa lúc package B đang `CREATE INDEX ... gin_trgm_ops` →
//     `operator class "gin_trgm_ops" does not exist` (SQLSTATE 42704).
//
// Khoá nắm trên 1 `*sql.Conn` riêng (session-level) và PHẢI giữ suốt vòng đời
// schema — dựng → test → dọn — chứ không chỉ quanh `platform.Migrate`. Nếu chỉ
// khoá lúc migrate thì `DROP EXTENSION` ở cleanup vẫn chạy song song với
// migrate của package khác và lỗi 42704 quay lại.
//
// Đây chính là cách M1/M2 dùng (khoá ở `TestMain` suốt đời package). M3 chuyển
// khoá vào package này và hạ xuống từng lần `Acquire` — mức tương đương về
// thời gian chờ (trong 1 package test vẫn chạy tuần tự) nhưng không cần
// `TestMain` ở 7 package.
const MigrateLockKey int64 = 20260928

// DSNEnv là tên biến môi trường chứa DSN Postgres cho test.
const DSNEnv = "LANGAPP_TEST_POSTGRES_DSN"

// DSN đọc DSN Postgres cho test, đã trim. Chuỗi rỗng nghĩa là chưa bật
// Postgres — `Open` sẽ `t.Skip` kèm lý do thay vì fail vì lỗi kết nối.
func DSN() string { return strings.TrimSpace(os.Getenv(DSNEnv)) }

// Session giữ khoá advisory suốt đời 1 test.
//
// Cần dùng Session (thay vì `Open`) khi 1 test dựng NHIỀU schema — ví dụ test
// merge của context `sync` dựng "máy local" + "máy peer" trong cùng database.
// `Open` chỉ giữ khoá tới hết lần gọi, nên schema thứ hai tạo sau đó sẽ chạy
// `CREATE EXTENSION` mà không có khoá ⇒ race 23505/42704.
//
// Lưu ý: `pg_advisory_lock` KHÔNG xếp chồng giữa 2 session — session thứ 2 sẽ
// chờ vô hạn. Vì vậy phải dùng CHUNG 1 `Session` cho mọi schema của 1 test,
// không phải `Acquire` nhiều lần.
type Session struct {
	conn *sql.Conn
	db   *sql.DB
	dsn  string
}

// Acquire nắm khoá advisory toàn database cho tới khi test kết thúc.
func Acquire(t testing.TB, ctx context.Context) *Session {
	t.Helper()
	dsn := requireDSN(t)
	conn, lockDB, err := connectLock(ctx, dsn)
	if err != nil {
		t.Fatalf("nắm khoá advisory test: %v", err)
	}
	s := &Session{conn: conn, db: lockDB, dsn: dsn}
	// Unlock chạy SAU CÙNG (cleanup LIFO): mọi schema đã drop xong rồi mới
	// buông khoá, nếu không package khác có thể vào giữa lúc ta đang dọn.
	t.Cleanup(func() { s.release() })
	return s
}

func (s *Session) release() {
	if s.conn == nil {
		return
	}
	// Context.Background vì ctx của test có thể đã bị huỷ lúc cleanup.
	_, _ = s.conn.ExecContext(context.Background(),
		"SELECT pg_advisory_unlock($1)", MigrateLockKey)
	_ = s.conn.Close()
	// Đóng pool SAU khi đã trả `conn` về — xem `connectLock` vì sao pool này
	// bị rò trong M1–M3.
	_ = s.db.Close()
	s.conn = nil
	s.db = nil
}

// Open dựng schema Postgres tạm, chạy migration thật lên đó, và tự dọn sạch khi
// test xong. Xem `OpenSchema` — đây là bản bỏ qua tên schema.
func Open(t testing.TB, ctx context.Context) *gorm.DB {
	t.Helper()
	db, _ := OpenSchema(t, ctx)
	return db
}

// OpenSchema như `Open` nhưng trả kèm tên schema đã tạo — test cần tên đó khi
// tra `information_schema` (xem internal/platform/migrate_test.go).
func OpenSchema(t testing.TB, ctx context.Context) (*gorm.DB, string) {
	t.Helper()
	return Acquire(t, ctx).OpenSchema(t, ctx)
}

// OpenBareSchema dựng schema tạm nhưng KHÔNG chạy migration. Chỉ dùng cho test
// của chính `platform.Migrate` (cần quan sát lần apply đầu tiên, không phải
// lần no-op); mọi test khác dùng `OpenSchema`.
func OpenBareSchema(t testing.TB, ctx context.Context) (*gorm.DB, string) {
	t.Helper()
	return Acquire(t, ctx).openSchema(t, ctx, false)
}

// OpenSchema dựng thêm 1 schema tạm dùng CHUNG khoá của session này, chạy
// migration thật, tự dọn khi test xong.
//
// Dùng cho test cần > 1 schema trong cùng database (merge 2 máy). Phần
// `CREATE EXTENSION` của `00002_fts` là no-op với schema thứ hai (extension cấp
// DATABASE, đã có ở schema trước đó hoặc ở `public`) — an toàn vì khoá vẫn còn
// nguyên và `openSchema` tự tra schema thật sự giữ extension.
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

	// Schema phải tồn tại TRƯỚC khi pool test mở kết nối: `search_path` trong
	// DSN trỏ tới schema chưa có thì conn đầu tiên rơi vào "no schema has been
	// selected" và nằm lại trong pool idle → goose lấy đúng conn đó rồi fail.
	// Tạo qua conn của khoá (không thuộc pool test) là cách chắc chắn.
	if _, err := s.conn.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("tạo schema test: %v", err)
	}

	// CHẶN TRƯỚC khi test chạm vào dữ liệu app — xem `requireIsolatedDB`.
	s.requireIsolatedDB(t, ctx)

	// `search_path` gồm schema đang tạo + schema thật sự giữ extension (nếu có).
	//
	// VÌ SAO CẦN: `CREATE EXTENSION pg_trgm` cài operator class `gin_trgm_ops`
	// vào schema ĐẦU TIÊN của `search_path` lúc chạy. Schema sau đó (peer, hoặc
	// lần `OpenSchema` tiếp theo trong cùng session) không có extension ⇒
	// `CREATE INDEX ... gin_trgm_ops` fail với `operator class "gin_trgm_ops"
	// does not exist` (42704).
	//
	// VÌ SAO PHẢI TRA CƠ SỞ DỮ LIỆU chứ không giả định "schema đầu tiên của
	// session": `pg_trgm` là extension cấp DATABASE, và nó ĐÃ TỒN TẠI ở
	// `public` nếu app đã boot vào database này (migration `00002_fts` chạy
	// lúc đó với `search_path` mặc định). Khi đó `CREATE EXTENSION IF NOT
	// EXISTS pg_trgm` — đúng câu trong `00002_fts.sql` — là NO-OP, extension
	// KHÔNG được cài vào schema test, mà vẫn nằm ở `public`.
	//
	// Đây không phải giả định suông: nó là lý do 100% test DB fail bằng
	// `42704` ngay khi có DSN thật, trên database mà app đã từng chạy (đây là
	// trạng thái bình thường của database dev). Giả định "schema đầu tiên
	// giữ extension" chỉ đúng với database hoàn toàn trống.
	//
	// Thứ tự ưu tiên giữ nguyên: schema test ĐỨNG ĐẦU nên `CREATE TABLE` không
	// tên định danh vẫn rơi vào schema test (không pollute `public` — xem
	// `WithSearchPath`), còn schema giữ extension chỉ để Postgres resolve được
	// `gin_trgm_ops` + hàm `similarity()`.
	searchPath := schema
	if ext := s.extensionSchema(ctx); ext != "" && ext != schema {
		searchPath = schema + "," + ext
	}
	db, err := openTestDB(ctx, WithSearchPath(s.dsn, searchPath))
	if err != nil {
		// PHẢI dùng `dropSchema` chứ không gọi `drop(schema, nil)`: `drop`
		// return sớm khi `db == nil` nên lệnh drop KHÔNG BAO GIỜ chạy, còn
		// schema đã `CREATE` ở trên thì vẫn còn. Đây đúng là cơ chế để lại
		// 61 schema rác ở M1/M2 (đếm được trên DB dev).
		dropErr := s.dropSchema(ctx, schema)
		t.Fatalf("mở Postgres test: %v (dọn schema %s: %v)", err, schema, dropErr)
	}
	// Migration THẬT (goose) — test phải đi qua đúng schema production, không
	// dựng bảng tay: nếu không, thiếu CHECK/trigger/cột A1 sẽ không bao giờ lộ.
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
		// Đóng pool trước khi drop: conn nào còn `search_path` trỏ vào schema
		// vừa bị xoá sẽ rơi vào "no schema has been selected" cho test sau.
		s.drop(schema, db)
		_ = sqlDB.Close()
	})
	return db, schema
}

// extensionSchema trả schema thật sự đang giữ extension `pg_trgm`, hoặc "" nếu
// extension chưa tồn tại.
//
// TRA CƠ SỞ DỮ LIỆU thay vì nhớ "schema nào tạo trước": `pg_trgm` là extension
// cấp DATABASE, nên nó có thể nằm ở `public` (app đã boot vào database này rồi)
// hoặc ở 1 schema test trước đó. Cả 2 đều hợp lệ, và chỉ `pg_extension` mới
// trả lời chính xác. Giả định sai ở đây làm 100% test DB fail bằng `42704`.
//
// Lỗi khi tra cứu bị bỏ qua (trả ""): nếu không tra được thì `search_path` rơi
// về chỉ có schema test và migration sẽ fail KÉO theo, báo lỗi gốc ở ngay
// `goose up` — dễ đọc hơn là fail ở đây với thông báo mơ hồ.
func (s *Session) extensionSchema(ctx context.Context) string {
	if s.conn == nil {
		return ""
	}
	var name string
	_ = s.conn.QueryRowContext(ctx,
		"SELECT n.nspname FROM pg_extension e "+
			"JOIN pg_namespace n ON n.oid = e.extnamespace "+
			"WHERE e.extname = 'pg_trgm'").Scan(&name)
	return name
}

// requireIsolatedDB FAIL nếu DSN trỏ vào database đang chứa dữ liệu app.
//
// VÌ SAO CẦN: `search_path` của test phải gồm schema giữ extension `pg_trgm`.
// Extension đó thường nằm ở `public` — cùng schema chứa bảng thật của app. Khi
// `public` nằm trong `search_path`, một câu `SELECT count(*) FROM dict` mà bảng
// `dict` đã bị DROP trong schema test sẽ RƠI XUỐNG `public` và đọc dữ liệu thật
// của user, thay vì báo "relation does not exist".
//
// Đây không phải rủi ro lý thuyết: nó xảy ra thật khi viết test kiểu "ngắt 1
// bảng rồi khẳng định seeder báo lỗi" — test xanh vì đọc nhầm bảng production
// thay vì bảng đã drop, tức đúng loại test tự lừa mình mà M7c đang vá.
// Nguy hiểm hơn nữa: test có thể GHI vào bảng thật.
//
// Cách đúng là database riêng cho test (`langapp_test`), xem `Makefile`:
// `make test` trỏ DSN về `langapp_test`. Hàm này chỉ để biến việc quên ấy
// thành lỗi TO+ nét thay vì kết quả sai.
func (s *Session) requireIsolatedDB(t testing.TB, ctx context.Context) {
	t.Helper()
	if s.conn == nil {
		return
	}
	// Chỉ đếm bảng CÙNG loại với bảng app (đều có cột `deleted`) để không bị
	// vướng bảng hệ thống / `goose_db_version` của chính con DB test.
	var appTables int
	if err := s.conn.QueryRowContext(ctx,
		"SELECT count(*) FROM pg_tables t "+
			"JOIN information_schema.columns c ON c.table_schema = t.schemaname "+
			"  AND c.table_name = t.tablename AND c.column_name = 'deleted' "+
			"WHERE t.schemaname = 'public'").Scan(&appTables); err != nil || appTables == 0 {
		return // không tra được, hoặc database trống ⇒ không có gì để phá
	}
	t.Fatalf("DSN trỏ vào database ĐANG CHỨA DỮ LIỆU APP (%d bảng trong `public`) — "+
		"test có thể đọc/ghi nhầm dữ liệu thật của bạn. Hãy dùng database riêng cho test: "+
		"chạy `make test` (đã trỏ sẵn `langapp_test`), hoặc tự tạo `CREATE DATABASE langapp_test`",
		appTables)
}

// WithSearchPath thêm `search_path` vào DSN, tôn trọng DSN đã có query string.
// `schemas` là danh sách phân tách bởi dấu phẩy, thứ tự = thứ tự ưu tiên.
//
// `search_path` phải nằm trong DSN chứ không `SET search_path` sau khi mở:
// pool có nhiều conn, `SET` chỉ áp cho conn đó, còn conn thứ hai trở đi sẽ tạo
// bảng rơi vào `public` — `DROP SCHEMA … CASCADE` lúc cleanup không xoá được
// chúng, pollute DB dev vĩnh viễn (finding F3 của M1 remediation).
func WithSearchPath(dsn, schemas string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schemas
}

// AllowSkipEnv là biến môi trường cho phép `TestMain` CHẤP NHẬN việc thiếu
// DSN và cho test skip, thay vì fail.
//
// Vì sao cần: có lúc thực sự không có Postgres (laptop offline, agent chạy
// trong container không có docker). Khi đó muốn chạy phần test không cần DB.
// Nhưng mặc định phải FAIL — xem `Main`.
const AllowSkipEnv = "ALLOW_SKIP_DB_TESTS"

// Main là `TestMain` dùng chung cho MỌI package test cần Postgres.
//
// VÌ SAO FAIL KHI THIẾU DSN: gate M6 chạy `go test ./...` không set DSN và báo
// "525 PASS / 219 SKIP / 0 FAIL". Con số "0 FAIL" đó gần như vô nghĩa khi 1/3
// suite im lặng bỏ qua — và đó chính là chỗ đã giấu bug F1 (mất dữ liệu ở chiều
// ghi) suốt 2 phase. Test bỏ qua KHÔNG phải "không có gì sai": nó là tuyên bố
// "tao đã kiểm tra" rồi không kiểm tra gì cả.
//
// Vì vậy 2 điều kiện, và chỉ 2:
//  1. `LANGAPP_TEST_POSTGRES_DSN` rỗng ⇒ FAIL, trừ khi
//  2. `ALLOW_SKIP_DB_TESTS=1` được set TƯỜNG MINH.
//
// Không dùng giá trị mặc định ẩn (kiểu "unset thì coi như cho phép"): người đọc
// `go test ./...` trên máy mới không có cách nào biết mình vừa chạy 1/3 suite.
//
// Cách dùng trong mỗi package:
//
//	func TestMain(m *testing.M) { testdb.Main(m) }
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

// SkipAllowed đọc `ALLOW_SKIP_DB_TESTS`. Chỉ giá trị "1" mới cho phép — không
// nhận "true"/"yes" để tránh "set nhầm 1 biến khác" bật skip ngoài ý muốn.
func SkipAllowed() bool { return os.Getenv(AllowSkipEnv) == "1" }

// requireDSN báo rõ lý do khi chưa bật Postgres, thay vì để test fail vì lỗi
// kết nối khó hiểu. KHÔNG dùng testcontainers (STACK-V2-PLAN §1 cố ý loại):
// Postgres chạy sẵn trong `docker compose`.
//
// Ở đây vẫn `Skip` chứ không `Fatal`: đây là lớp phòng thủ CUỐI cho test gọi
// `testdb` ngoài package nào chạy qua `Main`. Khi thiếu DSN mà chưa set
// `ALLOW_SKIP_DB_TESTS`, `Main` đã fail cả package trước khi tới đây — nên
// nhánh skip này chỉ chạy khi người dùng ĐÃ opt-in (`ALLOW_SKIP_DB_TESTS=1`).
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

// connectLock mở 1 session Postgres riêng và nắm khoá advisory toàn database.
// Session phải TÁCH khỏi pool test: khoá advisory thuộc về session, đóng pool
// là mất khoá.
//
// Trả CẢ `*sql.DB` vì pool này là tài nguyên phải giải phóng: `sql.Open` mở
// pool với `MaxIdleConns` mặc định = không giới hạn và `MaxIdleTime` mặc định =
// 0 (không bao giờ tự đóng). Nếu chỉ đóng `*sql.Conn` mà quên `*sql.DB` thì mỗi
// test rò ≥1 connection vĩnh viễn cho tới hết package — `go test ./...` chạy
// `-p` = số CPU package song song, nên đủ là đụng `max_connections` của image
// `postgres:18` (100) và mọi test sau fail bằng
// `FATAL: sorry, too many clients already`.
//
// Đây là lỗi có thật của M1–M3: chỉ lộ ra khi M4 thêm 3 package test nữa, tức
// khi số package song song đủ lớn. `Session.release` đóng pool sau khi đã
// `conn.Close()` (trả conn về pool) — đảo thứ tự thì mất session giữ khoá.
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

// dropSchema xoá schema bằng conn của KHOÁ ADVISORY (`s.conn`), không cần pool
// test — nên dùng được cả khi `openTestDB` đã fail và chưa có `*gorm.DB`.
//
// Tách riêng khỏi `drop` vì `drop` cần `*gorm.DB` (để gỡ extension trước),
// mà `openTestDB` fail nghĩa là không có `db` nào. Nhánh đó schema mới chỉ rỗng
// (chưa chạy migration ⇒ chưa có `pg_trgm` nào được cài vào nó) nên không cần
// gỡ extension.
//
// `t.Fatalf` trong `openSchema` khiến nhánh lỗi không assert được từ test, nên
// đây là hợp đồng tách riêng: mọi schema `openSchema` đã `CREATE` mà chưa kịp
// mở pool đều phải dọn được bằng hàm này. Test ở `testdb_leak_test.go`.
func (s *Session) dropSchema(ctx context.Context, schema string) error {
	if s.conn == nil {
		return errors.New("không có conn của khoá để dọn schema")
	}
	_, err := s.conn.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE")
	return err
}

// drop dọn schema test. Extension phải gỡ TRƯỚC khi drop schema: Postgres từ
// chối `DROP SCHEMA` khi schema còn giữ extension. Đây cũng là lý do 61 schema
// rác còn sót lại trong DB dev ở M1/M2.
//
// `DROP EXTENSION` là thao tác cấp DATABASE nên chỉ extension của schema
// LOCAL mới cần gỡ; schema thứ hai (peer) chỉ có `DROP SCHEMA`.
func (s *Session) drop(schema string, db *gorm.DB) {
	if db == nil {
		return
	}
	if s.conn != nil {
		// Chỉ gỡ extension khi nó thực sự nằm trong schema đang drop.
		var schemaHasExt bool
		row := s.conn.QueryRowContext(context.Background(),
			"SELECT EXISTS (SELECT 1 FROM pg_extension e "+
				"JOIN pg_namespace n ON n.oid = e.extnamespace "+
				"WHERE n.nspname = $1)", schema)
		if err := row.Scan(&schemaHasExt); err == nil && schemaHasExt {
			_ = db.Exec("DROP EXTENSION IF EXISTS pg_trgm CASCADE")
		}
	}
	_ = db.Exec("DROP SCHEMA " + schema + " CASCADE")
}
