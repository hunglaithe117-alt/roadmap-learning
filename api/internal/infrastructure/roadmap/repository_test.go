package roadmapinfra_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	roadmapapp "langapp/internal/application/roadmap"
	domain "langapp/internal/domain/roadmap"
	roadmapinfra "langapp/internal/infrastructure/roadmap"
)

var fixedNow = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

// zhPathSlug là slug THẬT của path seed tiếng Trung — loader chạy
// `slugify(firstNonEmpty(f.Slug, f.Title))` và file JSON không khai `slug`, nên
// slug ra từ tiêu đề "Tự học tiếng Trung giản thể từ số 0 → HSK 4". Test
// dùng hằng này (tính lại từ `domain.Slugify`) thay vì hardcode 2 lần.
var zhPathSlug = domain.Slugify("Tự học tiếng Trung giản thể từ số 0 → HSK 4")

// buildTree tạo path → 1 stage → 3 topic, trả về id các node.
func buildTree(t *testing.T, svc *roadmapapp.Service) (path roadmapapp.Path, stage roadmapapp.Stage, topics []roadmapapp.Topic) {
	t.Helper()
	ctx := context.Background()
	var err error
	path, err = svc.CreatePath(ctx, roadmapapp.PathInput{
		Slug: zhPathSlug, Title: "Tự học tiếng Trung", Language: "zh",
	})
	require.NoError(t, err)
	stage, err = svc.CreateStage(ctx, path.Slug, roadmapapp.StageInput{
		Slug: "zh-g0", Title: "G0 — Pinyin", Goal: "đọc được pinyin",
		DurationWeeks: intp(3), Terrain: "meadow", Direction: "up",
	})
	require.NoError(t, err)
	for i, title := range []string{"Bopomofo", "Thanh điệu", "Ngữ âm"} {
		tp, err := svc.CreateTopic(ctx, stage.ID, roadmapapp.TopicInput{
			Title: title, Why: "vì sao", Activities: []string{"nghe", "viết"},
		})
		require.NoError(t, err)
		assert.Equal(t, i, tp.Position, "position phải tự tăng khi client không gửi")
		topics = append(topics, tp)
	}
	return path, stage, topics
}

func intp(v int) *int         { return &v }
func strp(v string) *string   { return &v }
func f64p(v float64) *float64 { return &v }
func fixedNoNow() time.Time   { return fixedNow }

func roadmapRepo(t *testing.T, db *gorm.DB) *roadmapinfra.Repository {
	t.Helper()
	return roadmapinfra.NewRepository(db)
}

func Test_roadmap_crud_roundtrip_on_postgres(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()

	path, stage, topics := buildTree(t, svc)
	assert.NotZero(t, path.ID)
	assert.Equal(t, "meadow", stage.Terrain)
	assert.Equal(t, "up", stage.Direction)
	assert.NotEmpty(t, topics[0].GUID)

	// Đọc lại từ DB: mọi cột phải khớp, không chỉ struct trong tay.
	var row struct {
		Slug, Title, Language, Overview string
		IsBuiltin                       int
	}
	require.NoError(t, db.Raw(
		"SELECT slug, title, language, overview, is_builtin FROM roadmap_paths WHERE id = ?",
		path.ID).Scan(&row).Error)
	assert.Equal(t, zhPathSlug, row.Slug)
	assert.Equal(t, 0, row.IsBuiltin, "path tự tạo (không phải seed) phải is_builtin = 0")

	// terrain/direction là cột của migration 00004, nằm trên roadmap_stages.
	var stageRow struct {
		Slug, Terrain, Direction string
	}
	require.NoError(t, db.Raw(
		"SELECT slug, terrain, direction FROM roadmap_stages WHERE id = ?", stage.ID).Scan(&stageRow).Error)
	assert.Equal(t, "meadow", stageRow.Terrain)
	assert.Equal(t, "up", stageRow.Direction)

	var topicRow struct {
		Activities string
		Status     string
		Completed  *string
		MapX       *float64
	}
	require.NoError(t, db.Raw(
		"SELECT activities, status, completed_at, map_x FROM roadmap_topics WHERE id = ?",
		topics[0].ID).Scan(&topicRow).Error)
	assert.Equal(t, `["nghe","viết"]`, topicRow.Activities, "activities lưu thành JSON array")
	assert.Equal(t, "not_started", topicRow.Status)
	assert.Nil(t, topicRow.Completed, "topic mới phải có completed_at = NULL, không phải ''")
	assert.Nil(t, topicRow.MapX, "map_x NULL = để server layout")

	paths, err := svc.ListPaths(ctx)
	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.Equal(t, 3, paths[0].Progress.TopicsTotal, "3 topic đã tạo")
	assert.Equal(t, 3, paths[0].Progress.TopicsRequired)
	assert.Equal(t, 0, paths[0].Progress.Percent)
}

func Test_map_coordinates_persist_and_are_preferred_over_layout(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()
	_, stage, _ := buildTree(t, svc)

	// Node 2 user đặt tay.
	pinned, err := svc.CreateTopic(ctx, stage.ID, roadmapapp.TopicInput{
		Title: "Node đặt tay", MapX: f64p(123.5), MapY: f64p(456.5),
	})
	require.NoError(t, err)

	rows := listTopics(t, db, stage.ID)
	domainTopics := make([]domain.Topic, 0, len(rows))
	for _, r := range rows {
		domainTopics = append(domainTopics, roadmapapp.TopicToDomain(r))
	}
	layout := domain.ComputeLayout(domainTopics, domain.TerrainMeadow, domain.DirectionUp)

	var found bool
	for i, dt := range domainTopics {
		if dt.ID == pinned.ID {
			found = true
			require.NotNil(t, dt.MapX)
			assert.Equal(t, 123.5, layout[i].X, "giá trị user phải thắng layout tự động")
			assert.Equal(t, 456.5, layout[i].Y)
		}
	}
	require.True(t, found, "topic vừa tạo phải xuất hiện trong list")
}

func Test_map_coord_outside_viewbox_is_rejected(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()
	_, stage, _ := buildTree(t, svc)

	// viewBox là 0 0 1000 2000.
	_, err := svc.CreateTopic(ctx, stage.ID, roadmapapp.TopicInput{
		Title: "ra ngoài", MapX: f64p(1500),
	})
	require.Error(t, err)
	var appErr *roadmapapp.Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 400, appErr.Status)

	_, err = svc.CreateTopic(ctx, stage.ID, roadmapapp.TopicInput{
		Title: "âm", MapY: f64p(-1),
	})
	require.Error(t, err, "toạ độ âm là node ngoài canvas, M6 không scroll tới được")
}

func Test_terrain_and_direction_whitelist_enforced_by_service(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()
	path, stage, _ := buildTree(t, svc)

	_, err := svc.UpdateStage(ctx, stage.ID, roadmapapp.StagePatch{Terrain: strp("forest")})
	require.Error(t, err)
	var appErr *roadmapapp.Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 400, appErr.Status)
	assert.Contains(t, appErr.Message, "meadow, desert, snow, volcano, ocean, city",
		"lỗi phải liệt kê whitelist để client hiện được lựa chọn hợp lệ")

	_, err = svc.UpdateStage(ctx, stage.ID, roadmapapp.StagePatch{Direction: strp("left")})
	require.Error(t, err)

	// Giá trị hợp lệ được nhận và ghi xuống DB.
	_, err = svc.UpdateStage(ctx, stage.ID, roadmapapp.StagePatch{
		Terrain: strp("volcano"), Direction: strp("right"),
	})
	require.NoError(t, err)

	var stored struct {
		Terrain, Direction string
	}
	require.NoError(t, db.Raw(
		"SELECT terrain, direction FROM roadmap_stages WHERE id = ? AND path_id = ?",
		stage.ID, path.ID).Scan(&stored).Error)
	assert.Equal(t, "volcano", stored.Terrain)
	assert.Equal(t, "right", stored.Direction)
}

func Test_stage_terrain_default_when_omitted(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()

	_, err := svc.CreatePath(ctx, roadmapapp.PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	stage, err := svc.CreateStage(ctx, "p", roadmapapp.StageInput{Slug: "s1", Title: "S1"})
	require.NoError(t, err)
	assert.Equal(t, "meadow", stage.Terrain, "rỗng = DEFAULT của cột")
	assert.Equal(t, "up", stage.Direction)
}

func Test_deck_id_validated_through_srs_port(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	// Dựng 1 deck thật trong cùng schema để port srs trả về Exists.
	require.NoError(t, db.Exec(
		"INSERT INTO decks (name, lang, created_at, guid) VALUES ('HSK1', 'zh', '2026-01-01', ?)",
		roadmapapp.NewGUID()).Error)
	var deckID int64
	require.NoError(t, db.Raw("SELECT id FROM decks WHERE name = 'HSK1'").Scan(&deckID).Error)

	// Port `roadmap.DeckReader` được hiện thực bằng chính service của context
	// `srs` chạy trên cùng pool — đúng cách platform/di.go sẽ bind ở M4.
	svc := newServiceWithDecks(t, db, fixedNow)
	path, err := svc.CreatePath(ctx, roadmapapp.PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	stage, err := svc.CreateStage(ctx, path.Slug, roadmapapp.StageInput{
		Slug: "g1", Title: "G1", DeckID: &deckID,
	})
	require.NoError(t, err)
	require.NotNil(t, stage.DeckID)
	assert.Equal(t, deckID, *stage.DeckID, "A1: bấm stage nhảy thẳng /review")

	// deck không tồn tại → 400, không phải lỗi DB 500.
	bad := int64(999999)
	_, err = svc.CreateStage(ctx, path.Slug, roadmapapp.StageInput{
		Slug: "g2", Title: "G2", DeckID: &bad,
	})
	require.Error(t, err)
	var appErr *roadmapapp.Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 400, appErr.Status)
	assert.Equal(t, "không tìm thấy deck", appErr.Message)
}

// A1: completed_at set khi vào done, clear khi rời done — và ghi SQL NULL thật
// (không phải chuỗi rỗng, xem F11 của M1 remediation).
func Test_topic_status_sets_and_clears_completed_at(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()
	_, _, topics := buildTree(t, svc)
	id := topics[0].ID

	_, err := svc.SetTopicStatus(ctx, id, strp("done"), nil)
	require.NoError(t, err)

	var done struct {
		Completed *string `gorm:"column:completed_at"`
		Status    string  `gorm:"column:status"`
	}
	require.NoError(t, db.Raw("SELECT completed_at, status FROM roadmap_topics WHERE id = ?", id).Scan(&done).Error)
	require.NotNil(t, done.Completed)
	assert.Equal(t, fixedNow.Format(time.RFC3339), *done.Completed)

	// Rời done → NULL thật. `''` sẽ lọt vào `WHERE completed_at IS NOT NULL`
	// và phá `progress?since=`.
	_, err = svc.SetTopicStatus(ctx, id, strp("in_progress"), nil)
	require.NoError(t, err)

	var after struct {
		Completed *string `gorm:"column:completed_at"`
		Status    string  `gorm:"column:status"`
	}
	require.NoError(t, db.Raw("SELECT completed_at, status FROM roadmap_topics WHERE id = ?", id).Scan(&after).Error)
	assert.Nil(t, after.Completed, "rời done phải clear thành SQL NULL")
	assert.Equal(t, "in_progress", after.Status)
}

func Test_stage_status_set_and_clear_completed_at(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()
	_, stage, _ := buildTree(t, svc)

	_, err := svc.SetStageStatus(ctx, stage.ID, strp("done"), nil)
	require.NoError(t, err)
	var v *string
	require.NoError(t, db.Raw("SELECT completed_at FROM roadmap_stages WHERE id = ?", stage.ID).Scan(&v).Error)
	require.NotNil(t, v)

	_, err = svc.SetStageStatus(ctx, stage.ID, strp("not_started"), nil)
	require.NoError(t, err)
	require.NoError(t, db.Raw("SELECT completed_at FROM roadmap_stages WHERE id = ?", stage.ID).Scan(&v).Error)
	assert.Nil(t, v)
}

// Trigger `langapp_touch_updated_at` là cơ sở merge LWW: đổi status (mà ta
// không set updated_at tường minh) phải được trigger chạm.
func Test_updated_at_trigger_touches_on_status_update(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()
	_, _, topics := buildTree(t, svc)

	var before string
	require.NoError(t, db.Raw("SELECT updated_at FROM roadmap_topics WHERE id = ?", topics[0].ID).Scan(&before).Error)
	assert.Equal(t, fixedNow.Format(time.RFC3339), before, "mốc tạo là now cố định của test")

	_, err := svc.SetTopicStatus(ctx, topics[0].ID, strp("in_progress"), nil)
	require.NoError(t, err)

	var after string
	require.NoError(t, db.Raw("SELECT updated_at FROM roadmap_topics WHERE id = ?", topics[0].ID).Scan(&after).Error)
	assert.NotEqual(t, before, after,
		"đổi status phải để trigger chạm updated_at — nếu GORM tự ghi hoặc ta Omit sai, mốc sẽ đứng yên")
}

func Test_progress_counts_optional_separately_from_denominator(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()
	_, stage, _ := buildTree(t, svc)

	// 1 node optional đánh dấu xong + 3 node bắt buộc, 1 trong đó xong.
	_, err := svc.CreateTopic(ctx, stage.ID, roadmapapp.TopicInput{
		Title: "Tham khảo", IsOptional: intp(1),
	})
	require.NoError(t, err)

	rows := listTopics(t, db, stage.ID)
	var optionalID int64
	for _, r := range rows {
		if r.Title == "Tham khảo" {
			optionalID = r.ID
		}
	}
	require.NotZero(t, optionalID)
	_, err = svc.SetTopicStatus(ctx, optionalID, strp("done"), nil)
	require.NoError(t, err)
	_, err = svc.SetTopicStatus(ctx, rows[0].ID, strp("done"), nil)
	require.NoError(t, err)

	prog, err := svc.Progress(ctx, zhPathSlug, nil)
	require.NoError(t, err)
	assert.Equal(t, 4, prog.TopicsTotal, "gồm cả node optional")
	assert.Equal(t, 3, prog.TopicsRequired, "node optional không nằm trong mẫu số")
	assert.Equal(t, 1, prog.TopicsOptional)
	assert.Equal(t, 1, prog.TopicsDone, "node optional đã xong KHÔNG tính vào tử số")
	assert.Equal(t, 33, prog.Percent, "1/3 = 33 (floor)")
}

func Test_progress_since_counts_only_completed_in_window(t *testing.T) {
	db := newTestDB(t)
	// Mốc thời gian 1 năm trước để completed_at nằm ngoài khoảng `since`.
	old := fixedNow.AddDate(-1, 0, 0)
	ctx := context.Background()
	// 2 path riêng: 1 path seed/xong hồi trước, 1 path xong hôm nay. Cùng 1 path
	// sẽ đụng slug UNIQUE.
	oldSvc := newService(t, db, old)
	_, err0 := oldSvc.CreatePath(ctx, roadmapapp.PathInput{Slug: "cu", Title: "Cũ", Language: "zh"})
	require.NoError(t, err0)
	_, oldTopics := buildSingleTopicStage(t, oldSvc, "cu", "cu-g0")

	nowSvc := newService(t, db, fixedNow)
	_, err1 := nowSvc.CreatePath(ctx, roadmapapp.PathInput{Slug: "moi", Title: "Mới", Language: "zh"})
	require.NoError(t, err1)
	_, newTopics := buildSingleTopicStage(t, nowSvc, "moi", "moi-g0")

	// Node "cũ" đánh dấu xong bằng service có mốc 1 năm trước → completed_at
	// nằm ngoài khoảng `since`. Node "mới" đánh dấu hôm nay.
	_, err := oldSvc.SetTopicStatus(ctx, oldTopics[0].ID, strp("done"), nil)
	require.NoError(t, err)
	_, err = nowSvc.SetTopicStatus(ctx, newTopics[0].ID, strp("done"), nil)
	require.NoError(t, err)

	// Path "cu" chỉ có 1 node xong (ngoài khoảng), path "moi" có 1 node xong hôm nay.
	since := fixedNow.AddDate(0, 0, -7)
	oldProg, err := nowSvc.Progress(ctx, "cu", &since)
	require.NoError(t, err)
	assert.Equal(t, 0, oldProg.CompletedInRange, "node xong 1 năm trước không nằm trong 7 ngày")
	assert.Equal(t, 1, oldProg.TopicsDone, "nhưng tổng done vẫn tính")

	newProg, err := nowSvc.Progress(ctx, "moi", &since)
	require.NoError(t, err)
	assert.Equal(t, 1, newProg.CompletedInRange, "node xong hôm nay nằm trong khoảng")
	require.NotNil(t, newProg.LastCompletedAt)
	assert.Equal(t, fixedNow.Format(time.RFC3339), *newProg.LastCompletedAt)

	all, err := nowSvc.Progress(ctx, "moi", nil)
	require.NoError(t, err)
	assert.Zero(t, all.CompletedInRange, "không truyền since thì không đếm khoảng")
}

func Test_soft_delete_stage_cascades_to_tree(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()
	_, stage, topics := buildTree(t, svc)

	_, err := svc.CreateResource(ctx, topics[0].ID, roadmapapp.ResourceInput{
		Title: "Bài giảng", URL: strp("https://example.com/a"), Kind: "video",
	})
	require.NoError(t, err)
	_, err = svc.CreateMilestone(ctx, stage.ID, roadmapapp.MilestoneInput{Text: "Viết được 50 chữ"})
	require.NoError(t, err)

	require.NoError(t, svc.DeleteStage(ctx, stage.ID))

	stages, err := roadmapRepo(t, db).ListStages(ctx, stage.PathID)
	require.NoError(t, err)
	assert.Empty(t, stages, "stage đã xoá mềm không xuất hiện trong list")

	for _, table := range []string{"roadmap_stages", "roadmap_topics", "roadmap_resources", "roadmap_milestones"} {
		var counts struct {
			N   int
			Del int
		}
		require.NoError(t, db.Raw(
			"SELECT COUNT(*) AS n, COUNT(*) FILTER (WHERE deleted = 1) AS del FROM "+table).Scan(&counts).Error)
		assert.Positive(t, counts.N, "%s: xoá mềm phải giữ tombstone, không DELETE hẳn", table)
		assert.Equal(t, counts.N, counts.Del, "%s: toàn bộ nhánh con phải mang tombstone", table)
	}
}

func Test_transaction_rolls_back_whole_delete_cascade(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()
	_, stage, topics := buildTree(t, svc)
	_, err := svc.CreateResource(ctx, topics[0].ID, roadmapapp.ResourceInput{Title: "R"})
	require.NoError(t, err)

	// Chặn UPDATE trên roadmap_milestones — tầng sâu nhất của cascade. Lần xoá
	// phải update stage + topic + resource trước rồi mới tới milestone, nên
	// lỗi xảy ra GIỮA CHỪNG: nếu không bọc transaction, 3 bảng kia đã mang
	// tombstone dở.
	require.NoError(t, db.Exec(`
		CREATE OR REPLACE FUNCTION t_block_update() RETURNS trigger
		LANGUAGE plpgsql AS $$ BEGIN
		  RAISE EXCEPTION 'blocked for test'; END; $$`).Error)
	require.NoError(t, db.Exec(`
		CREATE TRIGGER t_block_ms BEFORE UPDATE ON roadmap_milestones
		FOR EACH ROW EXECUTE FUNCTION t_block_update()`).Error)
	t.Cleanup(func() { db.Exec("DROP TRIGGER IF EXISTS t_block_ms ON roadmap_milestones") })

	_, err = svc.CreateMilestone(ctx, stage.ID, roadmapapp.MilestoneInput{Text: "M"})
	require.NoError(t, err, "tạo milestone là INSERT, chưa đụng trigger UPDATE")

	err = svc.DeleteStage(ctx, stage.ID)
	require.Error(t, err, "cascade chạm bảng bị chặn thì cả lần xoá phải fail")

	var stageDeleted, topicDeleted, resDeleted, msDeleted int
	require.NoError(t, db.Raw("SELECT deleted FROM roadmap_stages WHERE id = ?", stage.ID).Scan(&stageDeleted).Error)
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM roadmap_topics WHERE stage_id = ? AND deleted = 1", stage.ID).Scan(&topicDeleted).Error)
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM roadmap_resources WHERE deleted = 1").Scan(&resDeleted).Error)
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM roadmap_milestones WHERE deleted = 1").Scan(&msDeleted).Error)
	assert.Equal(t, 0, stageDeleted, "UPDATE stage phải bị rollback")
	assert.Equal(t, 0, topicDeleted, "UPDATE topic phải bị rollback")
	assert.Equal(t, 0, resDeleted, "UPDATE resource phải bị rollback")
	assert.Equal(t, 0, msDeleted, "milestone (nơi trigger chặn) không đổi")
}

func Test_duplicate_stage_slug_in_same_path_returns_conflict(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, roadmapapp.PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	_, err = svc.CreateStage(ctx, "p", roadmapapp.StageInput{Slug: "g1", Title: "G1"})
	require.NoError(t, err)

	_, err = svc.CreateStage(ctx, "p", roadmapapp.StageInput{Slug: "g1", Title: "G1 nhân bản"})
	require.Error(t, err)
	var appErr *roadmapapp.Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 409, appErr.Status)
}

func Test_duplicate_path_slug_returns_conflict(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNoNow())
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, roadmapapp.PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	_, err = svc.CreatePath(ctx, roadmapapp.PathInput{Slug: "p", Title: "P2"})
	require.Error(t, err)
	var appErr *roadmapapp.Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 409, appErr.Status)
	assert.Equal(t, "slug learning path đã tồn tại", appErr.Message)
}

func Test_missing_parent_returns_404_with_vietnamese_message(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNoNow())
	ctx := context.Background()

	_, err := svc.CreateStage(ctx, "khong-ton-tai", roadmapapp.StageInput{Slug: "g", Title: "G"})
	require.Error(t, err)
	var appErr *roadmapapp.Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 404, appErr.Status)
	assert.Equal(t, "không tìm thấy learning path", appErr.Message)

	_, err = svc.CreateTopic(ctx, 999999, roadmapapp.TopicInput{Title: "T"})
	require.Error(t, err)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 404, appErr.Status)
	assert.Equal(t, "không tìm thấy stage", appErr.Message)
}

func Test_layout_from_repository_is_deterministic_across_reads(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNoNow())
	_, stage, _ := buildTree(t, svc)

	layoutOnce := func() []domain.MapPoint {
		rows := listTopics(t, db, stage.ID)
		dts := make([]domain.Topic, 0, len(rows))
		for _, r := range rows {
			dts = append(dts, roadmapapp.TopicToDomain(r))
		}
		return domain.ComputeLayout(dts, domain.TerrainVolcano, domain.DirectionUp)
	}
	first := layoutOnce()
	second := layoutOnce()
	assert.Equal(t, first, second, "2 lần đọc cùng dữ liệu phải ra cùng toạ độ")
	require.NotEmpty(t, first)
	for i, p := range first {
		assert.GreaterOrEqual(t, p.X, 0.0)
		assert.LessOrEqual(t, p.X, domain.MapViewWidth, "node %d", i)
		assert.GreaterOrEqual(t, p.Y, 0.0)
		assert.LessOrEqual(t, p.Y, domain.MapViewHeight, "node %d", i)
	}
}

func Test_level_states_from_repository_follow_position_order(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNoNow())
	ctx := context.Background()
	_, stage, topics := buildTree(t, svc)

	_, err := svc.SetTopicStatus(ctx, topics[0].ID, strp("done"), nil)
	require.NoError(t, err)

	rows := listTopics(t, db, stage.ID)
	dts := make([]domain.Topic, 0, len(rows))
	for _, r := range rows {
		dts = append(dts, roadmapapp.TopicToDomain(r))
	}
	assert.Equal(t,
		[]domain.LevelState{domain.LevelDone, domain.LevelCurrent, domain.LevelLocked},
		domain.LevelStates(dts))
}

func Test_update_path_does_not_allow_slug_change(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNoNow())
	ctx := context.Background()
	path, err := svc.CreatePath(ctx, roadmapapp.PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)

	// PathPatch không có field slug — đổi slug là biến cấu trúc, chỉ làm bằng
	// thao tác riêng. Test này assert hành vi hiện tại: sửa title không đụng slug.
	updated, err := svc.UpdatePath(ctx, path.Slug, roadmapapp.PathPatch{Title: strp("Tên mới")})
	require.NoError(t, err)
	assert.Equal(t, "p", updated.Slug)
	assert.Equal(t, "Tên mới", updated.Title)
}

func Test_max_position_helpers_return_minus_one_when_empty(t *testing.T) {
	db := newTestDB(t)
	repo := roadmapRepo(t, db)
	ctx := context.Background()

	last, err := repo.MaxTopicPosition(ctx, 999999)
	require.NoError(t, err)
	assert.Equal(t, -1, last, "không có topic nào thì position kế tiếp là 0, tức last+1 = 0")
}

func Test_repository_update_touches_trigger_for_path(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNoNow())
	ctx := context.Background()
	path, err := svc.CreatePath(ctx, roadmapapp.PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)

	var before string
	require.NoError(t, db.Raw("SELECT updated_at FROM roadmap_paths WHERE id = ?", path.ID).Scan(&before).Error)

	_, err = svc.UpdatePath(ctx, "p", roadmapapp.PathPatch{Title: strp("đổi tên")})
	require.NoError(t, err)

	var after string
	require.NoError(t, db.Raw("SELECT updated_at FROM roadmap_paths WHERE id = ?", path.ID).Scan(&after).Error)
	assert.NotEqual(t, before, after, "sửa path cũng phải được trigger chạm")
}

// CHECK ở DB là backstop cho whitelist 6 terrain / 2 direction — application
// chặn sớm bằng message tiếng Việt, nhưng merge LWW (M7) và `pg_dump` restore
// có thể ghi thẳng SQL, bypass application.
func Test_terrain_and_direction_checks_reject_unknown_values(t *testing.T) {
	db := newTestDB(t)
	svc := newServiceWithDecks(t, db, fixedNoNow())
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, roadmapapp.PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	stage, err := svc.CreateStage(ctx, "p", roadmapapp.StageInput{Slug: "g", Title: "G"})
	require.NoError(t, err)

	for _, bad := range []struct{ col, val string }{
		{"terrain", "swamp"}, {"terrain", "Meadow"},
		{"direction", "left"}, {"direction", "UP"},
	} {
		err := db.Exec("UPDATE roadmap_stages SET "+bad.col+" = ? WHERE id = ?", bad.val, stage.ID).Error
		require.Error(t, err, "%s = %q phải bị CHECK chặn", bad.col, bad.val)
	}

	// 6 + 2 giá trị hợp lệ thì qua.
	for _, ok := range [][2]string{
		{"meadow", "up"}, {"desert", "up"}, {"snow", "right"},
		{"volcano", "up"}, {"ocean", "right"}, {"city", "up"},
	} {
		require.NoError(t, db.Exec(
			"UPDATE roadmap_stages SET terrain = ?, direction = ? WHERE id = ?",
			ok[0], ok[1], stage.ID).Error, "terrain=%s direction=%s phải hợp lệ", ok[0], ok[1])
	}
}

func Test_stage_deck_id_set_null_when_deck_deleted(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	require.NoError(t, db.Exec(
		"INSERT INTO decks (name, lang, created_at, guid) VALUES ('D', 'zh', '2026-01-01', ?)",
		roadmapapp.NewGUID()).Error)
	var deckID int64
	require.NoError(t, db.Raw("SELECT id FROM decks WHERE name = 'D'").Scan(&deckID).Error)

	svc := newServiceWithDecks(t, db, fixedNoNow())
	_, err := svc.CreatePath(ctx, roadmapapp.PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	_, err = svc.CreateStage(ctx, "p", roadmapapp.StageInput{Slug: "g", Title: "G", DeckID: &deckID})
	require.NoError(t, err)

	// A1: xoá deck không được làm mất stage.
	require.NoError(t, db.Exec("DELETE FROM decks WHERE id = ?", deckID).Error)

	var n int
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM roadmap_stages WHERE deck_id IS NOT NULL").Scan(&n).Error)
	assert.Equal(t, 0, n, "ON DELETE SET NULL phải chạy")
}
