package roadmapinfra_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	roadmapapp "langapp/internal/application/roadmap"
	domain "langapp/internal/domain/roadmap"
	roadmapinfra "langapp/internal/infrastructure/roadmap"
)

// Test nội bộ của gói remediation cổng Oracle M2. Tách riêng khỏi
// repository_test.go vì cần internal test (package roadmapinfra) cho 2 thứ:
//   - đọc lại TRONG transaction (chỉ làm được khi nắm được `txHandle`)
//   - bọc Repository bằng decorator đảo thứ tự rows trả về

// ── F1: giá trị trả về phải là giá trị vừa ghi ──────────────────────────────
//
// Trước đây các `Update*` đọc lại bằng `r.db` (pool) nên TRẢ VỀ row CŨ dù DB
// đã ghi đúng. Test cũ chỉ assert trong DB nên lọt qua.

func Test_update_stage_returns_new_values_within_transaction(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()
	_, stage, _ := buildTree(t, svc)

	got, err := svc.UpdateStage(ctx, stage.ID, roadmapapp.StagePatch{
		Title: strp("G0 — đã đổi"), DurationWeeks: intp(7),
	})
	require.NoError(t, err)
	assert.Equal(t, "G0 — đã đổi", got.Title, "trả về title cũ = đọc ngoài transaction")
	assert.Equal(t, 7, got.DurationWeeks)
	assert.Equal(t, "meadow", got.Terrain, "cột không bị patch phải giữ nguyên giá trị đọc được")
}

func Test_update_topic_and_resource_and_milestone_return_new_values(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()
	_, stage, topics := buildTree(t, svc)

	gotT, err := svc.UpdateTopic(ctx, topics[0].ID, roadmapapp.TopicPatch{
		Title: strp("Bopomofo (đã đổi)"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Bopomofo (đã đổi)", gotT.Title)

	res, err := svc.CreateResource(ctx, topics[0].ID, roadmapapp.ResourceInput{Title: "R cũ"})
	require.NoError(t, err)
	gotR, err := svc.UpdateResource(ctx, res.ID, roadmapapp.ResourcePatch{
		Title: strp("R mới"), Kind: strp("video"),
	})
	require.NoError(t, err)
	assert.Equal(t, "R mới", gotR.Title)
	assert.Equal(t, "video", gotR.Kind)

	ms, err := svc.CreateMilestone(ctx, stage.ID, roadmapapp.MilestoneInput{Text: "M cũ"})
	require.NoError(t, err)
	gotM, err := svc.UpdateMilestone(ctx, ms.ID, roadmapapp.MilestonePatch{
		Text: strp("M mới"), Position: intp(5),
	})
	require.NoError(t, err)
	assert.Equal(t, "M mới", gotM.Text)
	assert.Equal(t, 5, gotM.Position)
}

func Test_update_path_returns_new_values(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()
	path, _, _ := buildTree(t, svc)

	got, err := svc.UpdatePath(ctx, path.Slug, roadmapapp.PathPatch{
		Title: strp("Tự học tiếng Trung (đã đổi)"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Tự học tiếng Trung (đã đổi)", got.Title)
}

func Test_set_topic_and_stage_status_return_new_values(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()
	_, stage, topics := buildTree(t, svc)

	gotT, err := svc.SetTopicStatus(ctx, topics[0].ID, strp(roadmapapp.StatusDone), nil)
	require.NoError(t, err)
	assert.Equal(t, roadmapapp.StatusDone, gotT.Status, "trả về not_started = đọc ngoài transaction")
	require.NotNil(t, gotT.CompletedAt)

	gotS, err := svc.SetStageStatus(ctx, stage.ID, strp(roadmapapp.StatusSkipped), strp("bỏ qua"))
	require.NoError(t, err)
	assert.Equal(t, roadmapapp.StatusSkipped, gotS.Status)
	assert.Equal(t, "bỏ qua", gotS.StatusNote)
	assert.Nil(t, gotS.CompletedAt)
}

// ── F2: tx handle sai kiểu phải ra lỗi, KHÔNG gì ra DB ─────────────────────

// M3 (context `sync`) cần ghi `roadmap_*` + `decks/cards` trong cùng 1
// transaction, nên rất dễ lấy nhầm handle của context kia. `txOf` trước đây
// im lặng rơi về pool ⇒ ghi ra NGOÀI transaction, merge nửa vời không để lại
// dấu vết. Test này truyền 1 handle tùy ý (không phải txHandle) cho MỌI
// method ghi và assert DB không đổi.

type notATxHandle struct{ id int64 }

func Test_wrong_tx_handle_returns_error_and_writes_nothing(t *testing.T) {
	db := newTestDB(t)
	repo := roadmapinfra.NewRepository(db)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()
	_, stage, topics := buildTree(t, svc)

	before := map[string]any{}
	require.NoError(t, db.Raw(
		"SELECT title, goal, position, duration_weeks, status, terrain, direction FROM roadmap_stages WHERE id = ?",
		stage.ID).Scan(&before).Error)

	bad := notATxHandle{id: 999}

	// Gọi từng method ghi với handle sai — tất cả phải trả lỗi.
	stageRow := roadmapapp.Stage{ID: stage.ID, Title: "HACK", Slug: stage.Slug, Position: 99}
	assert.Error(t, repo.UpdateStage(ctx, bad, &stageRow), "UpdateStage")

	topicRow := roadmapapp.Topic{ID: topics[0].ID, Title: "HACK", Position: 99}
	assert.Error(t, repo.UpdateTopic(ctx, bad, &topicRow), "UpdateTopic")

	st := roadmapapp.Stage{ID: stage.ID, Status: roadmapapp.StatusDone}
	assert.Error(t, repo.UpdateStageStatus(ctx, bad, &st), "UpdateStageStatus")

	tp := roadmapapp.Topic{ID: topics[0].ID, Status: roadmapapp.StatusDone}
	assert.Error(t, repo.UpdateTopicStatus(ctx, bad, &tp), "UpdateTopicStatus")

	assert.Error(t, repo.SoftDeleteStage(ctx, bad, stage.ID), "SoftDeleteStage")
	assert.Error(t, repo.SoftDeleteTopic(ctx, bad, topics[0].ID), "SoftDeleteTopic")
	assert.Error(t, repo.SoftDeleteResource(ctx, bad, 1), "SoftDeleteResource")
	assert.Error(t, repo.SoftDeleteMilestone(ctx, bad, 1), "SoftDeleteMilestone")

	pathRow := roadmapapp.Path{ID: 1, Title: "HACK"}
	assert.Error(t, repo.UpdatePath(ctx, bad, &pathRow), "UpdatePath")
	assert.Error(t, repo.SoftDeletePath(ctx, bad, 1), "SoftDeletePath")

	msRow := roadmapapp.Milestone{ID: 1, Text: "HACK"}
	assert.Error(t, repo.UpdateMilestone(ctx, bad, &msRow), "UpdateMilestone")
	resRow := roadmapapp.Resource{ID: 1, Title: "HACK"}
	assert.Error(t, repo.UpdateResource(ctx, bad, &resRow), "UpdateResource")

	newPath := roadmapapp.Path{Slug: "hacked", Title: "HACK", Language: "zh", GUID: roadmapapp.NewGUID()}
	assert.Error(t, repo.CreatePath(ctx, bad, &newPath), "CreatePath")
	assert.Error(t, repo.CreateStage(ctx, bad, &roadmapapp.Stage{PathID: 1, Slug: "h", Title: "HACK"}), "CreateStage")
	assert.Error(t, repo.CreateTopic(ctx, bad, &roadmapapp.Topic{StageID: stage.ID, Title: "HACK"}), "CreateTopic")
	assert.Error(t, repo.CreateResource(ctx, bad, &roadmapapp.Resource{TopicID: topics[0].ID, Title: "HACK"}), "CreateResource")
	assert.Error(t, repo.CreateMilestone(ctx, bad, &roadmapapp.Milestone{StageID: stage.ID, Text: "HACK"}), "CreateMilestone")

	// DB phải nguyên trạng: 1 row "HACK" nào cũng không được xuất hiện.
	var hacked int
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM roadmap_stages WHERE title = 'HACK' OR slug = 'h'").Scan(&hacked).Error)
	assert.Zero(t, hacked, "handle sai KHÔNG được ghi ra ngoài transaction")
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM roadmap_topics WHERE title = 'HACK'").Scan(&hacked).Error)
	assert.Zero(t, hacked)
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM roadmap_paths WHERE title = 'HACK' OR slug = 'hacked'").Scan(&hacked).Error)
	assert.Zero(t, hacked)
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM roadmap_milestones WHERE text = 'HACK'").Scan(&hacked).Error)
	assert.Zero(t, hacked)
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM roadmap_resources WHERE title = 'HACK'").Scan(&hacked).Error)
	assert.Zero(t, hacked)

	// Và stage gốc phải còn nguyên (kể cả khi SoftDelete bị chặn).
	var deleted int
	require.NoError(t, db.Raw("SELECT deleted FROM roadmap_stages WHERE id = ?", stage.ID).Scan(&deleted).Error)
	assert.Equal(t, 0, deleted)
	var after map[string]any
	require.NoError(t, db.Raw(
		"SELECT title, goal, position, duration_weeks, status, terrain, direction FROM roadmap_stages WHERE id = ?",
		stage.ID).Scan(&after).Error)
	assert.Equal(t, before, after, "không cột nào được đổi")
}

// ── F3: ClearMap ghi SQL NULL thật ─────────────────────────────────────────

func Test_clear_map_writes_sql_null_not_zero(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()
	_, stage, _ := buildTree(t, svc)

	// Toạ độ đặt tay phải nằm trong biên canvas — dùng HẰNG SỐ domain thay vì
	// con số cứng 1999.5 của viewBox 1000×2000 cũ (trục phụ nay cao 900).
	tp, err := svc.CreateTopic(ctx, stage.ID, roadmapapp.TopicInput{
		Title: "Node đặt tay",
		MapX:  f64p(domain.MapViewWidth / 2),
		MapY:  f64p(domain.MapViewHeight / 2),
	})
	require.NoError(t, err)
	require.NotNil(t, tp.MapX)

	got, err := svc.UpdateTopic(ctx, tp.ID, roadmapapp.TopicPatch{ClearMap: true})
	require.NoError(t, err)
	assert.Nil(t, got.MapX)
	assert.Nil(t, got.MapY)

	// `0` và NULL là 2 thứ khác nhau: (0,0) là vị trí hợp lệ trong viewBox và
	// `MapPinned` của M6 dựa trên NULL.
	var row struct {
		MapX *float64
		MapY *float64
	}
	require.NoError(t, db.Raw("SELECT map_x, map_y FROM roadmap_topics WHERE id = ?", tp.ID).Scan(&row).Error)
	assert.Nil(t, row.MapX, "ClearMap phải ghi SQL NULL, không phải 0")
	assert.Nil(t, row.MapY)

	view, err := svc.PathTree(ctx, zhPathSlug)
	require.NoError(t, err)
	for _, st := range view.Stages {
		for _, tv := range st.Topics {
			if tv.ID != tp.ID {
				continue
			}
			assert.False(t, tv.MapPinned, "node đã bỏ toạ độ thì MapPinned phải false")
		}
	}
}

// ── F4: bảng terrain/direction trong migration phải khớp MapDefaults ─────────
//
// Bảng này có 3 bản sao: (1) file SQL migration 00004, (2)
// `domain.MapDefaults`, (3) bảng spec trong test seed. Trước đây (1) KHÔNG
// được test nào chạm tới vì DB mới thì 10 câu `UPDATE … WHERE slug = ?` khớp
// 0 dòng. Test này đọc thẳng file .sql nên không cần DB.

var seedUpdateRe = regexp.MustCompile(
	`UPDATE roadmap_stages SET terrain = '(\w+)',\s*direction = '(\w+)'\s+WHERE slug = '([\w-]+)'`)

func Test_migration_map_table_matches_domain_defaults(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "00004_roadmap_map.sql"))
	require.NoError(t, err, "đọc migration 00004 (đường dẫn tương đối từ package này)")

	matches := seedUpdateRe.FindAllStringSubmatch(string(raw), -1)
	require.Len(t, matches, 10, "migration 00004 phải gán đúng 10 stage seed")

	got := map[string][2]string{}
	for _, m := range matches {
		got[m[3]] = [2]string{m[1], m[2]}
	}

	// Bảng spec: ROADMAP-MAP-IDEA §6. Nhánh này FAIL nếu 1 trong 3 bản lệch.
	for _, lang := range []string{"zh", "en"} {
		for i := 0; i < 5; i++ {
			slug := fmt.Sprintf("%s-g%d", lang, i)
			wantTerrain, wantDir := domain.MapDefaults(lang, i)
			require.Contains(t, got, slug, "migration thiếu stage %s", slug)
			assert.Equal(t, string(wantTerrain), got[slug][0],
				"terrain của %s trong migration lệch với domain.MapDefaults", slug)
			assert.Equal(t, string(wantDir), got[slug][1],
				"direction của %s trong migration lệch với domain.MapDefaults", slug)
		}
	}
}

// ── F6: GetPath phải ghép layout/state theo ID, không theo chỉ số ────────────

// shuffledTopicsRepo đảo thứ tự `ListTopics` trả về. Hiện `points`/`states`
// xếp theo (position, id) nên ghép theo chỉ số vẫn chạy đúng — nhưng M4 thay
// bằng `dataloadgen` là lệch IM LẶT. Repo này mô phỏng đúng tình huống đó.

type shuffledTopicsRepo struct {
	*roadmapinfra.Repository
	reversed bool
}

func (s *shuffledTopicsRepo) ListTopics(ctx context.Context, stageID int64) ([]roadmapapp.Topic, error) {
	rows, err := s.Repository.ListTopics(ctx, stageID)
	if err != nil || !s.reversed || len(rows) < 2 {
		return rows, err
	}
	out := make([]roadmapapp.Topic, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		out = append(out, rows[i])
	}
	return out, nil
}

func Test_get_path_joins_layout_by_id_when_rows_arrive_shuffled(t *testing.T) {
	db := newTestDB(t)
	base := roadmapinfra.NewRepository(db)
	svcBase := newService(t, db, fixedNow)
	ctx := context.Background()
	path, stage, _ := buildTree(t, svcBase)

	// Baseline: thứ tự chuẩn.
	want, err := svcBase.PathTree(ctx, path.Slug)
	require.NoError(t, err)
	require.Len(t, want.Stages, 1)
	require.Len(t, want.Stages[0].Topics, 3)

	// Rows đảo thứ tự: mỗi TopicView phải giữ đúng toạ độ + trạng thái CỦA
	// CHÍNH nó, dù vị trí trong slice đã đổi.
	shuffled := &shuffledTopicsRepo{Repository: base, reversed: true}
	svc := roadmapapp.NewService(shuffled, roadmapinfra.NewUnitOfWork(db), nil,
		func() time.Time { return fixedNow }, roadmapapp.ViewBox{})
	got, err := svc.PathTree(ctx, path.Slug)
	require.NoError(t, err)
	require.Len(t, got.Stages, 1)
	require.Len(t, got.Stages[0].Topics, 3)

	wantByID := map[int64]roadmapapp.TopicView{}
	for _, tv := range want.Stages[0].Topics {
		wantByID[tv.ID] = tv
	}
	for _, tv := range got.Stages[0].Topics {
		w, ok := wantByID[tv.ID]
		require.True(t, ok, "node %d không có trong baseline", tv.ID)
		assert.Equal(t, w.Point, tv.Point,
			"node %d mang toạ độ của node khác (ghép theo chỉ số thay vì theo ID)", tv.ID)
		assert.Equal(t, w.Level, tv.Level,
			"node %d mang trạng thái của node khác", tv.ID)
		assert.Equal(t, w.Title, tv.Title, "row cũng phải khớp chứ không chỉ layout")
	}
	_ = stage
}

// ── F9: seed không được insert vào cây đã tombstone ────────────────────────

func Test_seed_does_not_resurrect_soft_deleted_path(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	seeder := newSeeder(t, db, fixedNow)
	require.NoError(t, seeder.Run(ctx))

	// User xoá mềm path Trung (cascade đánh dấu toàn bộ nhánh con).
	svc := newService(t, db, fixedNow)
	require.NoError(t, svc.DeletePath(ctx, zhPathSlug))

	before := snapshotCounts(t, db)

	// Boot lại: seed KHÔNG được hồi sinh path, và cũng không được làm lỗi
	// (đụng UNIQUE `ux_roadmap_paths_slug` sẽ làm hỏng boot).
	require.NoError(t, seeder.Run(ctx), "seed chạy lại phải không lỗi")

	after := snapshotCounts(t, db)
	assert.Equal(t, before, after, "seed không được thêm dòng nào vào cây đã tombstone")

	// Path Trung vẫn là đúng 1 row tombstone, không phải 2 row.
	var n int
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM roadmap_paths WHERE slug = ?", zhPathSlug).Scan(&n).Error)
	assert.Equal(t, 1, n, "tombstone giữ UNIQUE slug nên không thể có row thứ 2 cùng slug")

	var tombstoned int
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM roadmap_paths WHERE slug = ? AND deleted = 1", zhPathSlug).
		Scan(&tombstoned).Error)
	assert.Equal(t, 1, tombstoned, "path phải vẫn là tombstone, không bị hồi sinh")
}

func Test_seed_does_not_duplicate_soft_deleted_topic_and_milestone(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	seeder := newSeeder(t, db, fixedNow)
	require.NoError(t, seeder.Run(ctx))
	svc := newService(t, db, fixedNow)

	// 1 topic + 1 milestone của stage zh-g0, xoá mềm.
	var stageID int64
	require.NoError(t, db.Raw("SELECT id FROM roadmap_stages WHERE slug = 'zh-g0'").Scan(&stageID).Error)
	var topicID int64
	require.NoError(t, db.Raw(
		"SELECT id FROM roadmap_topics WHERE stage_id = ? ORDER BY position, id LIMIT 1",
		stageID).Scan(&topicID).Error)
	require.NoError(t, svc.DeleteTopic(ctx, topicID))

	before := snapshotCounts(t, db)
	require.NoError(t, seeder.Run(ctx), "seed chạy lại phải không lỗi")
	after := snapshotCounts(t, db)
	assert.Equal(t, before, after,
		"topic/milestone đã xoá mềm KHÔNG được seed tạo bản sao trùng tiêu đề (không có UNIQUE index chặn)")

	// Cụ thể: đếm row theo tiêu đề, phải vẫn 1 (tombstone) chứ không phải 2.
	var dupes int
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM roadmap_topics WHERE stage_id = ? AND title = "+
			"(SELECT title FROM roadmap_topics WHERE id = ?)", stageID, topicID).Scan(&dupes).Error)
	assert.Equal(t, 1, dupes)
}
