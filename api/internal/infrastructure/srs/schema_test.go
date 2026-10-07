package srsinfra

// Chốt chặn ĐỒNG BỘ SCHEMA cho row struct của context `srs`.
//
// VÌ SAO CẦN, DÙ `sync` ĐÃ CÓ `snapshot_columns_test.go`: test ở `sync` so
// struct merge-row ↔ `snapshotColumns` — tức giữa HAI danh sách Go do tay giữ.
// Nó KHÔNG bắt được lớp lỗi đáng sợ nhất: ai đó thêm cột mới bằng MIGRATION mà
// quên cập nhật row struct. Khi đó DB có cột lạ còn struct không map nó, và cả
// hai danh sách tay vẫn "đúng theo nhau".
//
// Test này so row struct ↔ CỘT THẬT trong Postgres qua `information_schema.columns`
// trên schema tạm của `testdb` (chạy đúng migration production). Nó assert lên
// YÊU CẦU ("cột trong DB khớp cột struct khai"), không assert "hàm X gọi đúng
// N cột" — nên nó vẫn đúng khi implementation đổi.
//
// Dùng `schema.Parse` CỦA CHÍNH GORM (cùng bộ phân tích runtime dùng) thay vì
// tự đọc tag `gorm:"column:..."`: tự đọc là lặp lại đúng lớp lỗi
// "quy tắc đặt tên của GORM khác tôi tưởng".

import (
	"context"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"langapp/internal/platform/testdb"
)

// structColumns trả danh sách cột GORM map từ struct — bắt chước
// `structColumns()` ở `infrastructure/sync/snapshot_columns_test.go`.
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

// realColumns đọc cột thật của bảng trong schema test hiện hành.
//
// `current_schema()` chứ không phải `'public'`: `search_path` của `testdb` là
// `t_test_x,testext` và `public` CỐ Ý vắng mặt — hardcode `'public'` sẽ đọc
// nhầm/nhìn rỗng và biến test thành vô nghĩa.
func realColumns(t *testing.T, db *gorm.DB, table string) []string {
	t.Helper()
	rows, err := db.Raw(
		"SELECT column_name FROM information_schema.columns "+
			"WHERE table_name = ? AND table_schema = current_schema() "+
			"ORDER BY column_name", table).Rows()
	require.NoError(t, err, "tra cột thật của %s", table)
	t.Cleanup(func() { _ = rows.Close() })

	var out []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		out = append(out, name)
	}
	require.NoError(t, rows.Err())
	return out
}

// Test_row_structs_match_real_columns là chốt chặn chính: cột row struct khai
// và cột DB thật phải bằng nhau CẢ HAI CHIỀU.
//
//   - thiếu trong struct (DB có, struct không) ⇒ struct không đọc cột mới, ghi
//     xuống sẽ xoá/rỗng cột đó;
//   - thừa trong struct (struct khai, DB không) ⇒ tên cột sai/đã đổi, GORM sẽ
//     chạy lệnh nhắm vào cột không tồn tại.
func Test_row_structs_match_real_columns(t *testing.T) {
	db := testdb.Open(t, context.Background())

	for _, tc := range []struct {
		table string
		row   interface{}
	}{
		{"decks", Deck{}},
		{"cards", Card{}},
	} {
		tc := tc
		t.Run(tc.table, func(t *testing.T) {
			real := map[string]bool{}
			for _, c := range realColumns(t, db, tc.table) {
				real[c] = true
			}
			declared := map[string]bool{}
			for _, c := range structColumns(t, tc.row) {
				declared[c] = true
			}

			var missing, extra []string
			for c := range real {
				if !declared[c] {
					missing = append(missing, c)
				}
			}
			for c := range declared {
				if !real[c] {
					extra = append(extra, c)
				}
			}
			sort.Strings(missing)
			sort.Strings(extra)

			assert.Empty(t, missing,
				"%s: cột có trong DB nhưng row struct không khai ⇒ struct đọc thiếu", tc.table)
			assert.Empty(t, extra,
				"%s: row struct khai cột không có trong DB ⇒ sai tên hoặc schema đã đổi", tc.table)
		})
	}
}
