package roadmapinfra_test

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	roadmapinfra "langapp/internal/infrastructure/roadmap"
)

// Số dòng seed thật trong 2 file JSON. Đây là hợp đồng của exit criteria M2
// ("2 path / 10 stage / 51 topic / 263 resource không đổi") — nếu ai đó sửa
// JSON thì phải sửa cả con số này và nói rõ trong PR.
const (
	seedPaths     = 2
	seedStages    = 10
	seedTopics    = 51
	seedResources = 263
)

func Test_seed_loads_expected_tree_from_embedded_json(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	require.NoError(t, newSeeder(t, db, fixedNow).Run(ctx))

	assert.Equal(t, seedPaths, countRows(t, db, "roadmap_paths"), "2 path: learn_chinese + learn_english")
	assert.Equal(t, seedStages, countRows(t, db, "roadmap_stages"))
	assert.Equal(t, seedTopics, countRows(t, db, "roadmap_topics"))
	assert.Equal(t, seedResources, countRows(t, db, "roadmap_resources"))

	// Không stage nào thiếu guid — UNIQUE NOT NULL DEFAULT '' nghĩa là guid
	// rỗng sẽ đụng nhau ở lần insert thứ 2.
	var emptyGUIDs int
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM roadmap_stages WHERE guid = '' OR guid IS NULL").Scan(&emptyGUIDs).Error)
	assert.Zero(t, emptyGUIDs, "mọi stage seed phải có guid khác nhau")

	// Path seed đánh dấu is_builtin = 1 (user vẫn sửa/xoá được).
	var builtin int
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM roadmap_paths WHERE is_builtin = 1").Scan(&builtin).Error)
	assert.Equal(t, seedPaths, builtin)
}

// Terrain/direction gán sẵn cho 10 stage theo ROADMAP-MAP-IDEA §6. Bảng này
// phải khớp domain.MapDefaults — cùng 1 nguồn sự thật, kiểm bằng 2 bên.
func Test_seed_assigns_terrain_and_direction_per_roadmap_map_idea(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, newSeeder(t, db, fixedNow).Run(context.Background()))

	var rows []struct {
		Slug      string `gorm:"column:slug"`
		Terrain   string `gorm:"column:terrain"`
		Direction string `gorm:"column:direction"`
	}
	require.NoError(t, db.Raw(
		"SELECT slug, terrain, direction FROM roadmap_stages ORDER BY slug").Scan(&rows).Error)
	require.Len(t, rows, seedStages)

	got := map[string][2]string{}
	for _, r := range rows {
		got[r.Slug] = [2]string{r.Terrain, r.Direction}
	}
	// MỌI stage đều `right` (đổi 2026-09-29): bản đồ roadmap đi ngang kiểu
	// roadmap.sh. Bảng cũ có 7 stage `up` — gồm G0 của CẢ 2 path — nên landing
	// view luôn mở chặng `up` (dải dọc 261×1952 trong khung 1377×620).
	want := map[string][2]string{
		// Trung: meadow → meadow → desert → snow → volcano.
		"zh-g0": {"meadow", "right"},
		"zh-g1": {"meadow", "right"},
		"zh-g2": {"desert", "right"},
		"zh-g3": {"snow", "right"},
		"zh-g4": {"volcano", "right"},
		// Anh: meadow → ocean → city → snow → volcano.
		"en-g0": {"meadow", "right"},
		"en-g1": {"ocean", "right"},
		"en-g2": {"city", "right"},
		"en-g3": {"snow", "right"},
		"en-g4": {"volcano", "right"},
	}
	assert.Equal(t, want, got, "bảng terrain/direction phải khớp ROADMAP-MAP-IDEA §6")

	// Hàng rào riêng cho hướng: bảng trên vẫn xanh nếu cả 10 stage cùng đổi
	// sang một hướng khác, nên assert thẳng "không stage nào còn `up`".
	var upCount int
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM roadmap_stages WHERE direction = 'up'").Scan(&upCount).Error)
	assert.Zero(t, upCount, "seed KHÔNG được sinh stage `up` — landing view mở `up` là bản đồ dọc hỏng")
}

// QUY TẮC VÀNG: chạy 2 lần không nhân bản, và không ghi đè nội dung user đã sửa.
func Test_seed_is_idempotent_and_never_overwrites_user_edits(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	seeder := newSeeder(t, db, fixedNow)

	require.NoError(t, seeder.Run(ctx))
	first := snapshotCounts(t, db)

	// User sửa tiêu đề 1 path + sửa 1 stage + xoá mềm 1 topic.
	require.NoError(t, db.Exec(
		"UPDATE roadmap_paths SET title = 'Tên do user đổi' WHERE slug = ?", zhPathSlug).Error)
	require.NoError(t, db.Exec(
		"UPDATE roadmap_stages SET title = 'G0 do user đổi' WHERE slug = 'zh-g0'").Error)
	var topicID int64
	require.NoError(t, db.Raw(
		"SELECT id FROM roadmap_topics WHERE title = 'Bảng pinyin và quy tắc viết thanh' LIMIT 1").
		Scan(&topicID).Error)
	require.NotZero(t, topicID)
	require.NoError(t, db.Exec("UPDATE roadmap_topics SET deleted = 1 WHERE id = ?", topicID).Error)

	require.NoError(t, seeder.Run(ctx), "chạy lần 2 phải không lỗi")
	second := snapshotCounts(t, db)
	assert.Equal(t, first, second, "load lần 2 không được thêm dòng nào")

	var pathTitle, stageTitle string
	require.NoError(t, db.Raw("SELECT title FROM roadmap_paths WHERE slug = ?", zhPathSlug).
		Scan(&pathTitle).Error)
	assert.Equal(t, "Tên do user đổi", pathTitle, "seed KHÔNG được ghi đè tiêu đề user đã sửa")

	require.NoError(t, db.Raw("SELECT title FROM roadmap_stages WHERE slug = 'zh-g0'").Scan(&stageTitle).Error)
	assert.Equal(t, "G0 do user đổi", stageTitle, "seed KHÔNG được ghi đè stage user đã sửa")

	var deleted int
	require.NoError(t, db.Raw("SELECT deleted FROM roadmap_topics WHERE id = ?", topicID).Scan(&deleted).Error)
	assert.Equal(t, 1, deleted, "topic user đã xoá mềm không được seed hồi sinh")
}

// 1 file JSON hỏng ⇒ rollback toàn bộ, không để lại cây nửa vời. File hỏng là
// file TÊN ĐẦU (a-broken.json) trong khi b-ok.json parse được — nếu loader đọc
// rồi insert tuần tự mà không bọc transaction, b-ok.json sẽ còn lại trong DB.
func Test_seed_rolls_back_whole_run_when_one_file_is_broken(t *testing.T) {
	db := newTestDB(t)

	broken := roadmapinfra.NewSeederFromFS(db, brokenSeedFS(), discardLogger, ".",
		func() time.Time { return fixedNow })
	err := broken.Run(context.Background())
	require.Error(t, err, "file JSON hỏng phải báo lỗi, không im lặng bỏ qua")

	assert.Zero(t, countRows(t, db, "roadmap_paths"),
		"1 file hỏng ⇒ rollback cả 2 file, không insert được path nào")
	assert.Zero(t, countRows(t, db, "roadmap_stages"))
	assert.Zero(t, countRows(t, db, "roadmap_topics"))
}

// brokenSeedFS dựng 1 FS trong bộ nhớ: 1 file JSON hỏng + 1 file hợp lệ.
func brokenSeedFS() fs.FS {
	good := `{"language":"zh","title":"Path hợp lệ","overview":"","stages":[` +
		`{"id":"g0","title":"G0","goal":"","duration_weeks":1,"milestones":[],` +
		`"topics":[{"title":"T","why":"","activities":[],"resources":[]}]}]}`
	bad := `{"language":"zh","title":"Path hong", "stages": [ }`
	return fstest.MapFS{
		"a-broken.json": &fstest.MapFile{Data: []byte(bad)},
		"b-ok.json":     &fstest.MapFile{Data: []byte(good)},
	}
}

// Seed chỉ INSERT, không UPDATE — nên `updated_at` của row tạo ra phải là mốc
// lúc seed (không bị trigger chạm vì không có UPDATE nào sau đó).
func Test_seed_rows_keep_creation_timestamp(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, newSeeder(t, db, fixedNow).Run(context.Background()))

	var bad int
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM roadmap_paths WHERE created_at <> ?", fixedNow.Format(time.RFC3339)).Scan(&bad).Error)
	assert.Zero(t, bad, "mọi row seed phải mang created_at của lần chạy")
}

func countRows(t *testing.T, db *gorm.DB, table string) int {
	t.Helper()
	var n int
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM "+table).Scan(&n).Error)
	return n
}

type snapshot struct {
	Paths, Stages, Topics, Resources, Milestones int
}

func snapshotCounts(t *testing.T, db *gorm.DB) snapshot {
	t.Helper()
	return snapshot{
		Paths:      countRows(t, db, "roadmap_paths"),
		Stages:     countRows(t, db, "roadmap_stages"),
		Topics:     countRows(t, db, "roadmap_topics"),
		Resources:  countRows(t, db, "roadmap_resources"),
		Milestones: countRows(t, db, "roadmap_milestones"),
	}
}
