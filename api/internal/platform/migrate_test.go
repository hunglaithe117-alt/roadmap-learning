// File này là external test package (`platform_test`) vì `internal/platform/
// testdb` import `platform`: nếu test ở trong package `platform` thì đó là
// import cycle (Go cấm in-package test import package phụ thuộc vào nó).
//
// Toàn bộ dựng schema Postgres + khoá advisory `CREATE EXTENSION pg_trgm` +
// migration thật đã chuyển vào `internal/platform/testdb` (M3) — trước đó nó
// là 3 bản sao, 1 trong mỗi package test DB. Chỉ phần ASSERT còn ở đây.
package platform_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"langapp/internal/platform"
	"langapp/internal/platform/testdb"
)

// openTestDB dựng schema Postgres tạm ĐÃ chạy migration thật (khoá advisory +
// `search_path` trong DSN + cleanup do testdb lo) — dùng cho các test chỉ cần
// schema production.
func openTestDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	return testdb.OpenSchema(t, context.Background())
}

// openBareTestDB dựng schema Postgres tạm CHƯA migrate. Chỉ dùng cho test của
// chính `platform.Migrate` (cần quan sát lần apply đầu tiên); mọi test khác
// dùng `openTestDB`.
func openBareTestDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	return testdb.OpenBareSchema(t, context.Background())
}

// lockAndMigrate chạy `platform.Migrate` và fail test nếu lỗi — tên cũ giữ
// nguyên vì nó nói rõ ý nghĩa: chạy Up TRONG khoá advisory mà testdb đang nắm.
func lockAndMigrate(t *testing.T, db *gorm.DB) platform.MigrationStatus {
	t.Helper()
	status, err := platform.Migrate(context.Background(), db)
	require.NoError(t, err)
	return status
}

func Test_migrate_up_creates_expected_schema(t *testing.T) {
	// Schema CHƯA migrate: test này chính là test cho `platform.Migrate`.
	db, schema := openBareTestDB(t)

	status := lockAndMigrate(t, db)
	// 5 migration: 00001 init + 00002 fts + 00003 roadmap_a1 + 00004 roadmap_map
	// + 00005 insight_index.
	// Con số phải bằng số file .sql trong api/migrations/ — thêm migration mà
	// quên cập nhật là test đỏ, đúng ý đồ.
	assert.EqualValues(t, 5, status.Applied, "phải apply 5 migration")
	assert.EqualValues(t, 5, status.Version)

	// goose tạo sẵn 1 row version_id = 0 làm mốc, nên bảng có 4 + 1 dòng.
	var distinct int
	require.NoError(t, db.Raw("SELECT COUNT(DISTINCT version_id) FROM goose_db_version WHERE is_applied").Scan(&distinct).Error)
	assert.Equal(t, 6, distinct, "goose_db_version phải có version 0 (mốc) + 1..5")

	for _, table := range []string{
		"decks", "cards", "reviews", "notes", "dict", "en_dict",
		"sync_meta", "sync_conflicts",
		"roadmap_paths", "roadmap_stages", "roadmap_milestones",
		"roadmap_topics", "roadmap_resources", "roadmap_bookmarks",
		"schema_migrations",
	} {
		var n int
		require.NoError(t, db.Raw(
			"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = ?",
			schema, table).Scan(&n).Error, "thiếu bảng %s", table)
		assert.Equal(t, 1, n, "thiếu bảng %s", table)
	}

	// 15 bảng ứng dụng + goose_db_version do goose tự tạo. Migration 00004 chỉ
	// thêm CỌT và 00005 chỉ thêm INDEX nên tổng bảng không đổi.
	var total int
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ?", schema).Scan(&total).Error)
	assert.Equal(t, 16, total, "tổng số bảng trong schema sau 5 migration")
}

func Test_migrate_up_twice_is_idempotent(t *testing.T) {
	db, _ := openBareTestDB(t)

	first := lockAndMigrate(t, db)
	assert.EqualValues(t, 5, first.Applied)

	second, err := platform.Migrate(context.Background(), db)
	require.NoError(t, err, "chạy Up lần 2 phải không lỗi")
	assert.EqualValues(t, 0, second.Applied, "lần 2 không có migration mới")

	// Goose giữ 1 row version_id = 0 làm mốc, nên bảng có 6 dòng applied
	// sau 5 migration. Điều cần chứng minh là chạy lại KHÔNG sinh row mới.
	var applied int
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM goose_db_version WHERE is_applied").Scan(&applied).Error)
	assert.Equal(t, 6, applied, "chạy Up lần 2 không được ghi thêm version")
}

// Test_touch_updated_trigger_sets_now_guard: contract của trigger
// langapp_touch_updated_at (STACK-V2-PLAN §4.3 + §4.5) — có 2 nhánh:
// UPDATE không set updated_at → trigger chạm; UPDATE set tường minh → giữ.
func Test_touch_updated_trigger_sets_now_when_caller_omits(t *testing.T) {
	db, _ := openTestDB(t)

	row := db.Raw(
		"INSERT INTO decks (name, lang, created_at, guid, updated_at) VALUES ('D', 'zh', '2020-01-01T00:00:00Z', 'g1', '2020-01-01T00:00:00Z') RETURNING id").Row()
	var id int64
	require.NoError(t, row.Scan(&id))

	require.NoError(t, db.Exec("UPDATE decks SET name = 'D2' WHERE id = ?", id).Error)

	var updatedAt string
	require.NoError(t, db.Raw("SELECT updated_at FROM decks WHERE id = ?", id).Scan(&updatedAt).Error)
	assert.NotEqual(t, "2020-01-01T00:00:00Z", updatedAt,
		"UPDATE không set updated_at phải bị trigger chạm sang giờ hiện tại")
	_, parseErr := time.Parse("2006-01-02T15:04:05Z", updatedAt)
	assert.NoError(t, parseErr, "updated_at phải đúng format UTC của trigger, got %q", updatedAt)

	explicit := "2031-12-25T10:11:12Z"
	require.NoError(t, db.Exec("UPDATE decks SET updated_at = ? WHERE id = ?", explicit, id).Error)
	require.NoError(t, db.Raw("SELECT updated_at FROM decks WHERE id = ?", id).Scan(&updatedAt).Error)
	assert.Equal(t, explicit, updatedAt,
		"merge LWW set updated_at tường minh → trigger phải bỏ qua")
}

func Test_decks_lang_check_rejects_unknown(t *testing.T) {
	db, _ := openTestDB(t)

	err := db.Exec("INSERT INTO decks (name, lang, created_at) VALUES ('D', 'fr', '2020-01-01')").Error
	require.Error(t, err, "lang ngoài zh|en phải bị CHECK chặn")
	assert.Contains(t, err.Error(), "decks_lang_check")

	require.NoError(t, db.Exec("INSERT INTO decks (name, lang, created_at) VALUES ('D', 'en', '2020-01-01')").Error)
}

func Test_roadmap_status_check_and_a1_columns(t *testing.T) {
	db, _ := openTestDB(t)

	require.NoError(t, db.Exec("INSERT INTO roadmap_paths (slug, title, created_at) VALUES ('p', 'P', '2020-01-01')").Error)
	var pathID int64
	require.NoError(t, db.Raw("SELECT id FROM roadmap_paths WHERE slug = 'p'").Scan(&pathID).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO roadmap_stages (path_id, slug, title, created_at, completed_at, deck_id) VALUES (?, 's', 'S', '2020-01-01', NULL, NULL)",
		pathID).Error)
	require.NoError(t, db.Exec("INSERT INTO decks (name, lang, created_at) VALUES ('D', 'zh', '2020-01-01')").Error)
	var deckID int64
	require.NoError(t, db.Raw("SELECT id FROM decks WHERE name = 'D'").Scan(&deckID).Error)
	require.NoError(t, db.Exec("UPDATE roadmap_stages SET deck_id = ?", deckID).Error)

	require.NoError(t, db.Exec(
		"INSERT INTO roadmap_topics (stage_id, title, created_at) SELECT id, 'T', '2020-01-01' FROM roadmap_stages LIMIT 1").Error)
	require.NoError(t, db.Exec("INSERT INTO roadmap_bookmarks (title, created_at) VALUES ('B', '2020-01-01')").Error)

	err := db.Exec("UPDATE roadmap_stages SET status = 'archived'").Error
	require.Error(t, err, "status ngoài 4 hằng phải bị CHECK chặn")

	err = db.Exec("UPDATE roadmap_topics SET is_optional = 2").Error
	require.Error(t, err, "is_optional chỉ nhận 0|1")

	err = db.Exec("UPDATE roadmap_bookmarks SET status = 'pending'").Error
	require.Error(t, err, "bookmark status chỉ nhận 4 hằng")

	err = db.Exec("INSERT INTO roadmap_topics (stage_id, title, position, created_at) SELECT id, 'T2', -1, '2020-01-01' FROM roadmap_stages LIMIT 1").Error
	require.Error(t, err, "position âm phải bị CHECK chặn")

	// ON DELETE SET NULL: xóa deck không mất stage.
	require.NoError(t, db.Exec("DELETE FROM decks WHERE id = ?", deckID).Error)
	var nullCount int
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM roadmap_stages WHERE deck_id IS NOT NULL").Scan(&nullCount).Error)
	assert.Equal(t, 0, nullCount, "deck_id phải set NULL khi xóa deck")
}

func Test_search_functions_find_latin_and_single_cjk(t *testing.T) {
	db, _ := openTestDB(t)

	require.NoError(t, db.Exec("INSERT INTO dict (hanzi, pinyin, nghia) VALUES ('你好', 'ni3 hao3', 'chao')").Error)
	require.NoError(t, db.Exec("INSERT INTO dict (hanzi, pinyin, nghia) VALUES ('谢谢', 'xie4 xie5', 'cam on')").Error)
	require.NoError(t, db.Exec("INSERT INTO en_dict (lang, term, reading, gloss) VALUES ('en', 'abandon', '/əˈbændən/', 'roi bo')").Error)

	var hanzi string
	require.NoError(t, db.Raw("SELECT hanzi FROM dict_search('ni3', 5)").Scan(&hanzi).Error)
	assert.Equal(t, "你好", hanzi, "tra pinyin qua tsvector phải ra chuỗi Hán")

	// FTS5 match được 1 ký tự Hán; tsvector không — pg_trgm ILIKE là lưới vớ.
	var cjk string
	require.NoError(t, db.Raw("SELECT hanzi FROM dict_search('你', 5)").Scan(&cjk).Error)
	assert.Equal(t, "你好", cjk, "tra 1 ký tự Hán phải ra kết quả qua ILIKE fallback")

	var term string
	require.NoError(t, db.Raw("SELECT term FROM en_dict_search('aband', 5)").Scan(&term).Error)
	assert.Equal(t, "abandon", term, "tra tiền tố của en_dict phải ra 1 kết quả")
}

// F4: schema_migrations phải có version 4. Rỗng thì `MAX(version)` = NULL và
// `CheckVersion(0,0)` pass vacuously — 2 máy lệch schema vẫn merge được.
func Test_schema_migrations_seeded_with_version_4(t *testing.T) {
	db, _ := openTestDB(t)

	var v sql.NullInt64
	require.NoError(t, db.Raw("SELECT MAX(version) FROM schema_migrations").Scan(&v).Error)
	require.True(t, v.Valid, "schema_migrations phải có dòng, không được rỗng")
	assert.EqualValues(t, 4, v.Int64, "version phải khớp schema v4 mà sync.go/backup.go v1 đọc")

	// Restore chạy lại không được nhân bản dòng này.
	require.NoError(t, db.Exec(
		"INSERT INTO schema_migrations (version, applied_at) VALUES (4, '2026-01-01T00:00:00Z') ON CONFLICT (version) DO NOTHING").Error)
	var n int
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM schema_migrations").Scan(&n).Error)
	assert.Equal(t, 1, n, "version 4 phải là duy nhất")
}

func Test_reviews_grade_check_rejects_out_of_range(t *testing.T) {
	db, _ := openTestDB(t)

	require.NoError(t, db.Exec("INSERT INTO decks (name, lang, created_at) VALUES ('D', 'zh', '2020-01-01')").Error)
	var deckID int64
	require.NoError(t, db.Raw("SELECT id FROM decks WHERE name = 'D'").Scan(&deckID).Error)
	var cardID int64
	require.NoError(t, db.Raw(
		"INSERT INTO cards (deck_id, front, back, due_at, created_at) VALUES (?, 'f', 'b', '2026-01-01', '2026-01-01') RETURNING id",
		deckID).Scan(&cardID).Error)
	insert := func(grade int) error {
		// guid phải khác nhau: `ux_reviews_guid` là UNIQUE trên cột NOT NULL
		// DEFAULT '', nên 2 review cùng guid rỗng là đụng nhau (giữ nguyên hành
		// vi schema v4 — M2 phải luôn sinh guid).
		return db.Exec(
			"INSERT INTO reviews (card_id, grade, reviewed_at, next_due_at, guid) VALUES (?, ?, '2026-01-01', '2026-01-02', ?)",
			cardID, grade, uuid.NewString()).Error
	}
	for _, g := range []int{1, 2, 3, 4} {
		require.NoError(t, insert(g), "grade %d phải hợp lệ", g)
	}
	for _, g := range []int{0, 5, -1} {
		assert.Error(t, insert(g), "grade %d ngoài thang 1-4 phải bị CHECK chặn", g)
	}
}

// F5: `q` là input người dùng — nội suy thô vào ILIKE biến `%`/`_` thành
// wildcard, khiến 1 ký tự đó trả về toàn bảng.
func Test_search_functions_reject_like_wildcards(t *testing.T) {
	db, _ := openTestDB(t)

	for _, h := range []string{"你好", "谢谢", "妈妈"} {
		require.NoError(t, db.Exec("INSERT INTO dict (hanzi, pinyin, nghia) VALUES (?, 'py', 'nghia')", h).Error)
	}
	for _, term := range []string{"abandon", "able", "candid"} {
		require.NoError(t, db.Exec("INSERT INTO en_dict (lang, term, reading, gloss) VALUES ('en', ?, '/x/', 'g')", term).Error)
	}

	countDict := func(q string) int {
		var n int
		require.NoError(t, db.Raw("SELECT COUNT(*) FROM dict_search(?)", q).Scan(&n).Error, "q=%q", q)
		return n
	}
	countEn := func(q string) int {
		var n int
		require.NoError(t, db.Raw("SELECT COUNT(*) FROM en_dict_search(?)", q).Scan(&n).Error, "q=%q", q)
		return n
	}

	for _, q := range []string{"%", "_", "%_", "", "   ", "%%%", `\`} {
		assert.Equal(t, 0, countDict(q), "dict_search(%q) phải trả 0 dòng, không phải toàn bảng", q)
		assert.Equal(t, 0, countEn(q), "en_dict_search(%q) phải trả 0 dòng, không phải toàn bảng", q)
	}

	// Wildcard lẫn ký tự thật vẫn phải tra được (chỉ khử `%`/`_`, không cắt cụt).
	assert.Equal(t, 1, countDict("你"), "sau khi khử wildcard, tra Hán vẫn hoạt động")
	assert.Positive(t, countEn("aband"), "tiền tố Latin vẫn ra kết quả")
}
