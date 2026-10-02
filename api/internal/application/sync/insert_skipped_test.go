package sync

// Test mục 2 của gate M3 cuối ở tầng application: chỉ cần chứng minh quyết
// định "INSERT bị skip ⇒ log conflict, KHÔNG đếm `merged`" — tầng hạ tầng đã
// có test trên Postgres thật (`infrastructure/sync/seed_merge_test.go`).
//
// Vì sao vẫn cần test ở đây: `merged` là DTO của tầng này. Nếu ai đó sửa
// `Merged` mà quên chặn `skipped`, test Postgres cũng bắt — nhưng test ở đây
// đọc đúng kỳ vọng: `merged` phải SỐNG, không phải chỉ "DB không đổi".

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "langapp/internal/domain/sync"
)

func Test_B2_insert_skipped_is_logged_not_counted_merged(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()

	// Local đã có path slug "zh" dưới guid `p-local`.
	_, _, err := repo.UpsertPath(ctx, nil,
		PathRow{GUID: "p-local", Slug: "zh", Language: "zh", Title: "T", CreatedAt: tsOld}, false)
	require.NoError(t, err)

	// Peer gửi path slug "zh" dưới guid KHÁC ⇒ `Decide` ra ActionInsert, và
	// UNIQUE `ux_roadmap_paths_slug` sẽ bỏ lần insert đó.
	repo.skipInsertGUID = []string{"p-peer"}

	snap := domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			snapRow(domain.TableRoadmapPaths, "p-peer", "", tsPeer, 0,
				map[string]string{"slug": "zh", "language": "zh", "title": "T"}),
		},
	}
	svc, _ := newService(repo, snap)
	res, err := svc.Merge(ctx, snap)
	require.NoError(t, err, "trùng slug không được làm hỏng merge")

	assert.Equal(t, 0, res.Merged.RoadmapPaths,
		"mục 2: insert bị skip KHÔNG được đếm vào merged — báo cáo 'đã merge' "+
			"khi thực tế bị bỏ im lặng là bug khiến user tin đã sync xong")
	require.Len(t, res.Conflicts, 1)
	assert.Equal(t, domain.TableRoadmapPaths, res.Conflicts[0].Table)
	assert.Equal(t, "p-peer", res.Conflicts[0].GUID)
	assert.Equal(t, domain.WinnerLocal, res.Conflicts[0].Winner,
		"bản local được giữ ⇒ winner phải là local")
	assert.Contains(t, res.Conflicts[0].Detail, "insert-skipped-duplicate")
	assert.Contains(t, res.Conflicts[0].Detail, `key="zh"`,
		"phải nêu natural key bị trùng để user biết dòng nào")

	// Conflict phải vào cả log DB (fake ghi qua `LogConflict`).
	require.Len(t, repo.conflicts, 1)
	assert.Contains(t, repo.conflicts[0].Detail, "insert-skipped-duplicate")
}

// Chốt chống đảo: insert KHÔNG bị skip thì vẫn đếm `merged` và KHÔNG có
// conflict giả. Nếu không có test này thì cách sửa "luôn ghi conflict cho mọi
// insert" cũng xanh — và user sẽ thấy log conflict nhiễu.
func Test_B2_clean_insert_counted_merged_without_conflict(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	// Cố tình khai báo skip cho 1 guid KHÔNG liên quan — phải bị bỏ qua.
	repo.skipInsertGUID = []string{"guid-khong-ton-tai"}

	snap := domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			snapRow(domain.TableRoadmapPaths, "p-peer", "", tsPeer, 0,
				map[string]string{"slug": "zh", "language": "zh", "title": "T"}),
		},
	}
	svc, _ := newService(repo, snap)
	res, err := svc.Merge(ctx, snap)
	require.NoError(t, err)

	assert.Equal(t, 1, res.Merged.RoadmapPaths, "insert thật phải vẫn được đếm")
	assert.Empty(t, res.Conflicts, "insert không bị skip thì không được ghi conflict")
	assert.Contains(t, repo.paths, "p-peer", "row phải thật sự được ghi")
}

// Cùng luật cho topic — bảng con, để không ai sửa riêng `mergePaths` mà bỏ sót
// 5 bảng còn lại.
func Test_B2_insert_skipped_guard_applies_to_every_roadmap_table(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		skipOn string
		rows   []domain.Row
		table  domain.Table
		merged func(Merged) int
	}{
		{
			name:   "stage",
			skipOn: "st-peer",
			rows: []domain.Row{
				snapRow(domain.TableRoadmapPaths, "p-1", "", tsOld, 0,
					map[string]string{"slug": "zh", "language": "zh", "title": "T"}),
				snapRow(domain.TableRoadmapStages, "st-peer", "p-1", tsPeer, 0,
					map[string]string{"slug": "s1", "title": "S", "terrain": "meadow", "direction": "up"}),
			},
			table:  domain.TableRoadmapStages,
			merged: func(m Merged) int { return m.RoadmapStages },
		},
		{
			name:   "topic",
			skipOn: "tp-peer",
			rows: []domain.Row{
				snapRow(domain.TableRoadmapPaths, "p-1", "", tsOld, 0,
					map[string]string{"slug": "zh", "language": "zh", "title": "T"}),
				snapRow(domain.TableRoadmapStages, "st-1", "p-1", tsOld, 0,
					map[string]string{"slug": "s1", "title": "S", "terrain": "meadow", "direction": "up"}),
				snapRow(domain.TableRoadmapTopics, "tp-peer", "st-1", tsPeer, 0,
					map[string]string{"title": "T1"}),
			},
			table:  domain.TableRoadmapTopics,
			merged: func(m Merged) int { return m.RoadmapTopics },
		},
		{
			name:   "resource",
			skipOn: "rs-peer",
			rows: []domain.Row{
				snapRow(domain.TableRoadmapPaths, "p-1", "", tsOld, 0,
					map[string]string{"slug": "zh", "language": "zh", "title": "T"}),
				snapRow(domain.TableRoadmapStages, "st-1", "p-1", tsOld, 0,
					map[string]string{"slug": "s1", "title": "S", "terrain": "meadow", "direction": "up"}),
				snapRow(domain.TableRoadmapTopics, "tp-1", "st-1", tsOld, 0,
					map[string]string{"title": "T1"}),
				snapRow(domain.TableRoadmapResources, "rs-peer", "tp-1", tsPeer, 0,
					map[string]string{"title": "R1", "kind": "video"}),
			},
			table:  domain.TableRoadmapResources,
			merged: func(m Merged) int { return m.RoadmapResources },
		},
		{
			name:   "bookmark",
			skipOn: "bm-peer",
			rows: []domain.Row{
				snapRow(domain.TableRoadmapBookmarks, "bm-peer", "", tsPeer, 0,
					map[string]string{"title": "B1", "status": "to_read"}),
			},
			table:  domain.TableRoadmapBookmarks,
			merged: func(m Merged) int { return m.RoadmapBookmarks },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			// Dựng sẵn cây cha để `resolve` trong fake không chặn — fake không
			// kiểm tra FK, nhưng `mergeRoadmap` cần id cha trong map `ids`.
			repo.skipInsertGUID = []string{tc.skipOn}
			_, _, err := repo.UpsertPath(ctx, nil,
				PathRow{GUID: "p-1", Slug: "zh", Language: "zh", Title: "T", CreatedAt: tsOld}, false)
			require.NoError(t, err)
			_, _, err = repo.UpsertStage(ctx, nil,
				StageRow{GUID: "st-1", PathGUID: "p-1", Slug: "s1", Title: "S", CreatedAt: tsOld}, false)
			require.NoError(t, err)
			_, _, err = repo.UpsertTopic(ctx, nil,
				TopicRow{GUID: "tp-1", StageGUID: "st-1", Title: "T1", CreatedAt: tsOld}, false)
			require.NoError(t, err)

			snap := domain.PeerSnapshot{SchemaVersion: 4, Rows: tc.rows}
			svc, _ := newService(repo, snap)
			res, err := svc.Merge(ctx, snap)
			require.NoError(t, err)

			assert.Equal(t, 0, tc.merged(res.Merged),
				"mục 2: %s bị skip không được đếm vào merged", tc.table)
			var found bool
			for _, c := range res.Conflicts {
				if c.Table == tc.table {
					found = true
					assert.Contains(t, c.Detail, "insert-skipped-duplicate")
				}
			}
			assert.True(t, found, "mục 2: %s bị skip phải ghi conflict", tc.table)
		})
	}
}
