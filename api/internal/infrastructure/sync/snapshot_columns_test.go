package syncinfra

// Chốt chặn DRY (yêu cầu của cổng Oracle M3): hiện mỗi bảng có 4 danh sách cột
// phải KHỚP TAY — `snapshotColumns`, `SELECT` trong `Upsert*`, struct merge-row,
// `domain.Row.Values`. Không có gì kiểm chúng khớp nhau.
//
// B1 chính là triệu chứng của việc thiếu chốt: thêm `deck_id` vào
// `stageMergeRow` mà quên thêm vào phía snapshot ⇒ merge âm thầm xoá
// `deck_id` của mọi stage.
//
// Test này dùng CHÍNH CHIẾN ĐỊNH TÊN CỘT CỦA GORM để đổi tên field → column,
// nên không cần bảng ánh xạ thứ 5 phải cập nhật tay (bảng ánh xạ thứ 5 chính
// là lớp lỗi ta đang cố diệt).

import (
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"

	domain "langapp/internal/domain/sync"
)

// mergeRowSpec là 1 struct merge-row: bảng đọc từ đâu + các cột được phép
// VẮNG MẶT khỏi `snapshotColumns`.
type mergeRowSpec struct {
	// row là struct GORM đọc bảng local.
	row interface{}
	// table là bảng mà `snapshotColumns` phải mô tả.
	table domain.Table
	// fkColumns là cột FK: chúng KHÔNG nằm trong `snapshotColumns` (đó là liên
	// kết, không phải payload LWW) mà phải nằm trong `snapshotTables[].fks`.
	fkColumns []string
}

// mergeRowSpecs liệt kê MỌI struct merge-row của package. Thêm bảng mới mà
// quên khai ở đây thì `Test_merge_rows_table_covers_every_roadmap_table` bắt
// được.
var mergeRowSpecs = []mergeRowSpec{
	{row: deckMergeRow{}, table: domain.TableDecks},
	{row: cardMergeRow{}, table: domain.TableCards, fkColumns: []string{"deck_id"}},
	{row: pathMergeRow{}, table: domain.TableRoadmapPaths},
	{row: stageMergeRow{}, table: domain.TableRoadmapStages, fkColumns: []string{"path_id", "deck_id"}},
	{row: milestoneMergeRow{}, table: domain.TableRoadmapMilestones, fkColumns: []string{"stage_id"}},
	{row: topicMergeRow{}, table: domain.TableRoadmapTopics, fkColumns: []string{"stage_id"}},
	{row: resourceMergeRow{}, table: domain.TableRoadmapResources, fkColumns: []string{"topic_id"}},
	{row: bookmarkMergeRow{}, table: domain.TableRoadmapBookmarks},
}

// structColumns trả danh sách cột mà GORM sẽ map từ struct — dùng CHÍNH
// `schema.Parse` của GORM (cùng bộ phân tích mà runtime dùng) nên tag
// `gorm:"column:ipa"` và quy tắc `IPA → ip_a` đều được tính đúng, không cần
// bảng ánh xạ thứ 5 phải cập nhật tay.
func structColumns(t *testing.T, row interface{}) []string {
	t.Helper()
	sm, err := schema.Parse(row, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse thất bại: %v", err)
	}
	var out []string
	for _, f := range sm.Fields {
		if f.DBName != "" {
			out = append(out, f.DBName)
		}
	}
	return out
}

func fkColumnsOf(table domain.Table) map[string]bool {
	out := map[string]bool{}
	for _, spec := range snapshotTables {
		if spec.table != table {
			continue
		}
		for _, fk := range spec.fks {
			out[fk.column] = true
		}
	}
	return out
}

// Test_merge_row_columns_all_appear_in_snapshot_columns là chốt chặn chính:
// thêm 1 cột vào struct merge-row mà quên khai ở `snapshotColumns` thì test
// FAIL ngay, thay vì lặng lẽ làm merge mất dữ liệu (đúng cơ chế sinh ra B1).
func Test_merge_row_columns_all_appear_in_snapshot_columns(t *testing.T) {
	for _, spec := range mergeRowSpecs {
		spec := spec
		t.Run(string(spec.table), func(t *testing.T) {
			cols, ok := snapshotColumns[spec.table]
			require.True(t, ok, "bảng %s chưa có danh sách cột trong snapshotColumns", spec.table)
			have := map[string]bool{}
			for _, c := range cols {
				have[c] = true
			}
			fks := fkColumnsOf(spec.table)

			var missing []string
			for _, c := range structColumns(t, spec.row) {
				// `id` luôn được loader đọc thành `selectList := "id, guid"`.
				if c == "id" {
					continue
				}
				// Cột FK không nằm trong `snapshotColumns` — chốt ở test kế.
				if fks[c] {
					continue
				}
				if !have[c] {
					missing = append(missing, c)
				}
			}
			sort.Strings(missing)
			assert.Empty(t, missing,
				"B1: cột có trong struct merge-row nhưng thiếu ở snapshotColumns[%s] ⇒ "+
					"loader không đọc nó, `Values` không có ⇒ merge sẽ xoá dữ liệu của cột đó",
				spec.table)
		})
	}
}

// Test_fk_columns_are_declared_in_snapshot_tables là nửa còn lại của B1:
// `stageMergeRow.DeckID` được phép vắng ở `snapshotColumns`, nhưng BẮT BUỘC
// phải được khai trong `snapshotTables[].fks` — đó là chỗ duy nhất loader biết
// để đọc cột và resolve thành GUID.
//
// Bỏ `{"deck_id", domain.TableDecks, fkDeck}` khỏi `snapshotTables` thì test
// này FAIL (mất feature A1); bỏ `DeckID` khỏi `stageMergeRow` thì test kia
// FAIL.
func Test_fk_columns_are_declared_in_snapshot_tables(t *testing.T) {
	for _, spec := range mergeRowSpecs {
		spec := spec
		t.Run(string(spec.table), func(t *testing.T) {
			structCols := map[string]bool{}
			for _, c := range structColumns(t, spec.row) {
				structCols[c] = true
			}
			declared := fkColumnsOf(spec.table)
			for _, want := range spec.fkColumns {
				assert.True(t, structCols[want],
					"spec khai %q là FK của %s nhưng struct merge-row không có field tương ứng",
					want, spec.table)
				assert.True(t, declared[want],
					"B1: %s.%s không được khai trong snapshotTables[].fks ⇒ loader không đọc "+
						"cột này ⇒ merge sẽ xoá nó", spec.table, want)
			}
		})
	}
}

// Test_roadmap_stages_declare_deck_fk là chốt riêng cho feature A1 (bấm stage
// nhảy thẳng `/review`). Nếu không có test này, ai đó "dọn code" bỏ dòng
// `{"deck_id", domain.TableDecks, fkDeck}` sẽ không có gì ngăn.
func Test_roadmap_stages_declare_deck_fk(t *testing.T) {
	declared := fkColumnsOf(domain.TableRoadmapStages)
	assert.True(t, declared["deck_id"],
		"roadmap_stages phải khai FK deck_id trong snapshotTables[].fks — bỏ là chết feature A1")
}

// Test_snapshot_columns_all_appear_in_merge_row là CHIỀU NGƯỢC của test trên,
// và nó bắt đúng lỗi còn sót: khai 1 cột mới ở `snapshotColumns` mà quên thêm
// vào struct merge-row.
//
// Hệ quả của lỗi đó KHÔNG phải "thiếu dữ liệu" mà là "conflict giả": loader
// đọc cột đó vào `Row.Values`, còn `bookmarkValues`/`pathValues`… dựng map
// compare KHÔNG có nó ⇒ 2 bản thật giống nhau vẫn được coi là khác nhau mỗi
// lần sync, và `differs()` phát conflict vô tận. Chiều `struct → snapshotColumns`
// không bắt được vì struct hẹp hơn danh sách cột vẫn "hợp lệ" theo test kia.
func Test_snapshot_columns_all_appear_in_merge_row(t *testing.T) {
	for _, spec := range mergeRowSpecs {
		spec := spec
		t.Run(string(spec.table), func(t *testing.T) {
			structCols := map[string]bool{}
			for _, c := range structColumns(t, spec.row) {
				structCols[c] = true
			}
			// `created_at`/`updated_at`/`deleted` được loader đọc vào field riêng
			// của `domain.Row`, không nằm trong `Values` — nhưng chúng VẪN phải có
			// trong struct merge-row vì merge ghi chúng khi upsert.
			var missing []string
			for _, c := range snapshotColumns[spec.table] {
				if !structCols[c] {
					missing = append(missing, c)
				}
			}
			sort.Strings(missing)
			assert.Empty(t, missing,
				"cột khai trong snapshotColumns[%s] nhưng thiếu ở struct merge-row ⇒ "+
					"loader đọc vào Values nhưng hàm *Values của bảng đó không so sánh nó ⇒ "+
					"2 máy có dữ liệu giống nhau vẫn phát conflict mỗi lần sync",
				spec.table)
		})
	}
}

// Test_merge_row_specs_cover_every_merge_table chặn việc thêm bảng merge mới mà
// quên đăng ký vào `mergeRowSpecs` (khi đó 2 test trên im lặng bỏ sót bảng đó).
func Test_merge_row_specs_cover_every_merge_table(t *testing.T) {
	fromSpecs := map[domain.Table]bool{}
	for _, spec := range mergeRowSpecs {
		fromSpecs[spec.table] = true
	}
	for table := range snapshotColumns {
		if table == domain.TableReviews || table == domain.TableNotes {
			continue // append-only, không có struct merge-row LWW
		}
		assert.True(t, fromSpecs[table],
			"bảng %s có trong snapshotColumns nhưng chưa khai trong mergeRowSpecs", table)
	}
}

// Test_snapshot_columns_are_unique_and_ordered_by_load chặn 2 lỗi âm thầm còn
// lại: cột lặp trong `SELECT` (đọc thừa, không chết dữ liệu nhưng che lỗi khác)
// và bảng không có trong `snapshotTables` ⇒ loader không bao giờ đọc tới.
func Test_snapshot_columns_are_unique_and_ordered_by_load(t *testing.T) {
	loaded := map[domain.Table]bool{}
	for _, spec := range snapshotTables {
		loaded[spec.table] = true
	}
	for table, cols := range snapshotColumns {
		assert.True(t, loaded[table], "bảng %s có snapshotColumns nhưng không có trong snapshotTables", table)
		seen := map[string]bool{}
		for _, c := range cols {
			assert.False(t, seen[c], "bảng %s khai cột %q 2 lần trong snapshotColumns", table, c)
			seen[c] = true
			assert.NotEqual(t, "id", c, "cột `id` luôn được loader tự thêm, không cần khai")
		}
	}
}
