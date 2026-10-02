package syncinfra_test

// Test cho gate M3 cuối — mục 1 (phần end-to-end) và mục 2.
//
// Mục 1 e2e: 2 máy cùng seed roadmap bằng CHÍNH `roadmapinfra.Seeder` thật rồi
// merge. Trước fix, guid seed là uuid4 ngẫu nhiên ⇒ 2 máy cho 2 path khác
// guid ⇒ merge insert path của peer rồi không resolve được cha cho stage ⇒
// "không tìm thấy cha roadmap_paths guid=…" ⇒ **rollback CẢ merge**. Đây là
// repro nguyên văn của Oracle.
//
// Mục 2: INSERT bị `ON CONFLICT DO NOTHING` bỏ qua thì phải ghi conflict,
// KHÔNG được đếm vào `merged`.

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	roadmapinfra "langapp/internal/infrastructure/roadmap"
)

// discardLogger — Seeder gọi slog.Warn khi file seed có dữ liệu lỗi.
var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

var seedNow = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

// seedBothSchemas chạy `roadmapinfra.Seeder` thật trên cả 2 schema — 2 máy mới
// cài app. Dùng `NewSeeder` (embed.FS thật) chứ không dựng FS giả, đúng cách
// app gọi lúc boot.
func seedBothSchemas(t *testing.T, local, peer *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, roadmapinfra.NewSeeder(local, discardLogger,
		func() time.Time { return seedNow }).Run(ctx), "seed máy local")
	require.NoError(t, roadmapinfra.NewSeeder(peer, discardLogger,
		func() time.Time { return seedNow }).Run(ctx), "seed máy peer")
}

// REPRO NGUYÊN VĂN của Oracle mục 1.
func Test_B1_seed_two_machines_then_merge_does_not_rollback(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	seedBothSchemas(t, local, peer)

	// Chứng minh đây đúng là kịch bản Oracle: 2 máy có path CÙNG slug.
	localPaths := pathSlugs(t, local)
	peerPaths := pathSlugs(t, peer)
	require.NotEmpty(t, localPaths, "file seed thật phải có path")
	assert.Equal(t, localPaths, peerPaths, "2 máy cùng cài bộ seed thật")
	require.Greater(t, countRows(t, local, "roadmap_stages"), 0, "seed thật phải có stage")

	svc := newService(t, local, peer)
	res, err := svc.Sync(ctx)
	require.NoError(t, err,
		"mục 1: 2 máy cùng seed phải merge được. Trước fix, guid seed uuid4 ⇒ "+
			"\"không tìm thấy cha roadmap_paths\" ⇒ rollback CẢ merge")
	assert.Empty(t, res.Conflicts,
		"mục 1: 2 máy có dữ liệu giống hệt (cùng seed, cùng guid) thì KHÔNG được có xung đột")

	// Merge phải hội tụ: sau sync, local phải có đúng 1 path mà 2 máy cùng có.
	assert.Equal(t, len(localPaths), countRows(t, local, "roadmap_paths"),
		"mục 1: 2 máy cùng seed phải hội tụ còn MỘT path, không nhân đôi")

	// Chạy lại phải idempotent — cây roadmap đã hội tụ thì không ghi gì.
	res2, err := svc.Sync(ctx)
	require.NoError(t, err)
	assert.Empty(t, res2.Conflicts, "lần 2 không được có xung đột")
	assert.Equal(t, len(localPaths), countRows(t, local, "roadmap_paths"),
		"lần 2 không được nhân đôi path")
}

func pathSlugs(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var out []string
	require.NoError(t, db.Raw("SELECT slug FROM roadmap_paths ORDER BY slug").Scan(&out).Error)
	return out
}

// ═══════════════════════════════════════════════════════════════════════════
// Mục 2 — INSERT bị `ON CONFLICT DO NOTHING` bỏ qua phải log conflict
// ═══════════════════════════════════════════════════════════════════════════

// Repro của Oracle mục 2: peer gửi 1 path có guid MỚI nhưng slug đã có sẵn ở
// local dưới guid khác. `ux_roadmap_paths_slug` UNIQUE ⇒ INSERT bị skip ⇒
// `RETURNING id` không trả dòng (id = 0). Trước fix: `merged={RoadmapPaths:1}`,
// `conflicts=0`, DB không đổi ⇒ báo cáo nói dữ liệu đã vào trong khi thực tế bị
// bỏ im lặng.
func Test_B2_insert_skipped_by_unique_conflict_logs_conflict_and_is_not_counted_merged(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	// Local đã có path "zh" dưới guid `p-local`.
	localPath := seedPath(t, local, "zh", "p-local", tsOld)

	// Peer gửi path "zh" dưới guid `p-peer` — KHÁC guid (nên `Decide` ra Insert)
	// nhưng TRÙNG `ux_roadmap_paths_slug`.
	seedPath(t, peer, "zh", "p-peer", tsPeer)

	// Không stage nào để tránh lấy chỗ khác: chỉ cần bảng `roadmap_paths`.
	svc := newService(t, local, peer)
	res, err := svc.Sync(ctx)
	require.NoError(t, err, "trùng slug là chuyện bình thường, KHÔNG được làm hỏng merge")

	assert.Equal(t, 0, res.Merged.RoadmapPaths,
		"mục 2: INSERT bị skip ⇒ KHÔNG được đếm vào merged (báo cáo 'đã merge 1 path' "+
			"trong khi DB không hề đổi là loại bug khiến user tin đã sync xong)")

	require.Len(t, res.Conflicts, 1, "mục 2: phải ghi đúng 1 conflict cho lần insert bị bỏ")
	got := res.Conflicts[0]
	assert.Equal(t, "roadmap_paths", string(got.Table))
	assert.Equal(t, "p-peer", got.GUID)
	assert.Contains(t, got.Detail, "insert-skipped-duplicate",
		"lý do phải nói rõ là insert bị bỏ, không phải xung đột LWW")

	// DB local không được nhận path mới — path cũ vẫn nguyên.
	assert.Equal(t, 1, countRows(t, local, "roadmap_paths"))
	var guid string
	require.NoError(t, local.Raw("SELECT guid FROM roadmap_paths WHERE id = ?", localPath).
		Row().Scan(&guid))
	assert.Equal(t, "p-local", guid, "giữ bản local")

	// Conflict phải được ghi xuống `sync_conflicts` để user mở lại xem được
	// (không chỉ trả về trong response rồi bố đi).
	var logged int64
	require.NoError(t, local.Raw(
		"SELECT COUNT(*) FROM sync_conflicts WHERE guid = ? AND detail LIKE '%insert-skipped-duplicate%'",
		"p-peer").Scan(&logged).Error)
	assert.Equal(t, int64(1), logged, "mục 2: phải ghi xuống bảng sync_conflicts")
}

// Cùng luật cho stage — bảng khác, cùng cơ chế, để không ai tưởng chỉ `paths` mới
// bị. Peer gửi stage slug `s1` dưới guid khác, trùng
// `ux_roadmap_stages_path_slug` (path `p-same` đã có ở cả 2 máy).
func Test_B2_stage_insert_skipped_by_unique_conflict_logs_conflict(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	seedPath(t, local, "zh", "p-same", tsOld)
	seedStage(t, local, mustID(t, local, "roadmap_paths", "p-same"), "s1", "st-local", tsOld)

	seedPath(t, peer, "zh", "p-same", tsPeer)
	seedStage(t, peer, mustID(t, peer, "roadmap_paths", "p-same"), "s1", "st-peer", tsPeer)

	svc := newService(t, local, peer)
	res, err := svc.Sync(ctx)
	require.NoError(t, err)

	assert.Equal(t, 0, res.Merged.RoadmapStages,
		"mục 2: stage bị skip không được đếm vào merged")
	require.Len(t, res.Conflicts, 1)
	assert.Equal(t, "roadmap_stages", string(res.Conflicts[0].Table))
	assert.Equal(t, "st-peer", res.Conflicts[0].GUID)
	assert.Contains(t, res.Conflicts[0].Detail, "insert-skipped-duplicate")

	assert.Equal(t, 1, countRows(t, local, "roadmap_stages"), "không được tạo row thứ 2")
}

// Chốt: khi KHÔNG có skip thì `merged` vẫn phải tăng đúng và không có conflict
// giả. Nếu không có test này, cách sửa "luôn ghi conflict cho mọi insert" cũng
// xanh.
func Test_B2_clean_insert_still_counted_merged_without_conflict(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	seedPath(t, local, "zh", "p-local", tsOld)
	seedPath(t, peer, "en", "p-peer", tsPeer) // slug khác hoàn toàn

	svc := newService(t, local, peer)
	res, err := svc.Sync(ctx)
	require.NoError(t, err)

	assert.Equal(t, 1, res.Merged.RoadmapPaths, "insert thật phải vẫn được đếm")
	assert.Empty(t, res.Conflicts, "insert không bị skip thì không được ghi conflict")
	assert.Equal(t, 2, countRows(t, local, "roadmap_paths"))
}

func mustID(t *testing.T, db *gorm.DB, table, guid string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.Raw("SELECT id FROM "+table+" WHERE guid = ?", guid).Row().Scan(&id))
	return id
}
