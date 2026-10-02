package roadmapinfra_test

// Test cho gate M3 cuối — mục 1: guid seed phải ỔN ĐỊNH.
//
// Oracle reproduce bằng chính `Seeder` thật trên 2 schema Postgres: 2 máy
// cùng cài app đều chạy Seeder, path của máy này có guid `4a80…` còn máy kia
// `b091…` ⇒ merge coi là 2 path khác nhau, insert path của peer rồi **không
// resolve được cha cho stage** ⇒ rollback CẢ merge.
//
// 2 test:
//   - `Test_seed_guid_is_stable_across_two_independent_schemas` — dựng 2
//     schema Postgres, chạy `Seeder.Run()` trên cả hai với cùng file seed
//     thật (embed.FS), assert mọi guid khớp từng đẳng theo từng bảng.
//   - `Test_seed_then_merge_does_not_rollback` — 2 máy seed, rồi merge thật.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	roadmapinfra "langapp/internal/infrastructure/roadmap"
	"langapp/internal/platform/testdb"
)

// seedGUIDs đọc toàn bộ guid seed của 1 schema, nhóm theo bảng và sắp xếp —
// để so 2 máy mà không phụ thuộc thứ tự insert.
func seedGUIDs(t *testing.T, db *gorm.DB) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, tbl := range []string{
		"roadmap_paths", "roadmap_stages", "roadmap_milestones",
		"roadmap_topics", "roadmap_resources",
	} {
		var guids []string
		require.NoError(t, db.Raw("SELECT guid FROM "+tbl+" ORDER BY guid").Scan(&guids).Error, tbl)
		out[tbl] = guids
	}
	return out
}

// twoSeededSchemas dựng 2 schema Postgres độc lập và chạy `Seeder.Run()` thật
// trên cả hai — đúng cách 2 máy mới cài app.
func twoSeededSchemas(t *testing.T) (local, peer *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	session := testdb.Acquire(t, ctx)
	local, _ = session.OpenSchema(t, ctx)
	peer, _ = session.OpenSchema(t, ctx)

	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	// Cùng FS embed thật, cùng 2 file seed — khác nhau ở duy nhất là schema.
	// `now` cũng CỐ Ý giống nhau để test chỉ soi guid, không soi timestamp.
	require.NoError(t, roadmapinfra.NewSeeder(local, discardLogger,
		func() time.Time { return now }).Run(ctx))
	require.NoError(t, roadmapinfra.NewSeeder(peer, discardLogger,
		func() time.Time { return now }).Run(ctx))
	return local, peer
}

// REPRO của Oracle: 2 máy cùng seed phải cho ra CÙNG guid.
func Test_seed_guid_is_stable_across_two_independent_schemas(t *testing.T) {
	local, peer := twoSeededSchemas(t)

	a, b := seedGUIDs(t, local), seedGUIDs(t, peer)
	for tbl, guids := range a {
		require.NotEmpty(t, guids, "%s: file seed thật phải có dữ liệu, không thì test xanh nhầm", tbl)
		assert.Equal(t, guids, b[tbl],
			"mục 1: %s — 2 máy cùng seed phải cho cùng guid. Trước fix dùng "+
				"app.NewGUID() (uuid4) nên mỗi máy một guid khác nhau", tbl)
	}
}

// Guid seed phải là uuid5 (định bit version 5), KHÔNG phải uuid4 ngẫu nhiên.
// Chốt bằng thuộc tính bit trong chính giá trị, không phải bằng so sánh 2 lần
// chạy (lần so sánh chỉ bắt được tính không tất định, không bắt được "vẫn
// ngẫu nhiên nhưng cùng namespace").
func Test_seed_guid_is_uuid5_not_random_v4(t *testing.T) {
	local, _ := twoSeededSchemas(t)
	guids := seedGUIDs(t, local)["roadmap_paths"]
	require.NotEmpty(t, guids)
	for _, g := range guids {
		parsed, err := uuid.Parse(g)
		require.NoError(t, err, "guid seed %s phải parse được như UUID", g)
		assert.Equal(t, uuid.Version(5), parsed.Version(),
			"guid seed %s phải là uuid5 (ổn định theo natural key), không phải uuid4 ngẫu nhiên", g)
	}
}

// Guid phải khác nhau giữa các tầng kể cả khi natural key trùng chuỗi —
// `seedGUID` tiền tố bằng loại node nên path "zh" không đụng stage "zh".
func Test_seed_guid_distinguishes_node_kinds_with_same_natural_key(t *testing.T) {
	local, _ := twoSeededSchemas(t)
	seen := map[string]string{}
	for tbl, guids := range seedGUIDs(t, local) {
		for _, g := range guids {
			prev, dup := seen[g]
			require.False(t, dup, "guid %s bị dùng cho cả %s và %s", g, prev, tbl)
			seen[g] = tbl
		}
	}
}

// Seeder phải idempotent về guid: chạy lần 2 KHÔNG sinh guid mới, cũngng không
// đụng UNIQUE (nếu guid đổi mỗi lần chạy thì row cũ và mới cùng natural key
// sẽ đụng `ux_roadmap_paths_slug` và hỏng boot).
func Test_seed_guid_is_stable_across_reruns(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	seeder := roadmapinfra.NewSeeder(db, discardLogger, func() time.Time { return now })

	require.NoError(t, seeder.Run(ctx))
	first := seedGUIDs(t, db)
	require.NoError(t, seeder.Run(ctx), "chạy lại phải no-op, không đụng UNIQUE")
	assert.Equal(t, first, seedGUIDs(t, db), "chạy lại không được sinh guid mới")
}
