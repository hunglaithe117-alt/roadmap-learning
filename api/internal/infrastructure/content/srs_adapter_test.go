package contentinfra

// Chốt chặn ĐỒNG BỘ SCHEMA cho row struct của `content`.
//
// `srs_adapter.go` cố ý khai row HẸP (`deckRow`/`cardRow`) thay vì dùng struct
// của `infrastructure/srs`: import chéo 2 package hạ tầng sẽ khoá chúng vào
// nhau. Cái giá là các struct này có thể lệch schema âm thầm. Test này so
// chúng với CỘT THẬT trong Postgres (`information_schema.columns`) trên schema
// tạm của `testdb` (chạy đúng migration production).
//
// Khác `srs` (row phủ đủ 100% cột), row ở đây HẸP có chủ đích — nên cột vắng
// mặt được liệt kê TƯỜNG MINH trong `intentionallyAbsent`. Cột mới thêm bằng
// migration mà không nằm trong allowlist sẽ bị bắt (nó rơi vào "missing không
// giải thích"), nên không có đường lách.
//
// Dùng `schema.Parse` CỦA CHÍNH GORM (như `sync/snapshot_columns_test.go`) thay
// vì tự đọc tag `gorm:"column:..."`.

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

// structColumns trả danh sách cột GORM map từ struct.
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

// realColumns đọc cột thật trong schema test hiện hành. `current_schema()` chứ
// không `'public'`: `search_path` của `testdb` cố ý không có `public`.
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

// Test_srs_adapter_row_structs_match_real_columns chốt cả 2 chiều:
//
//   - struct khai cột DB không có (thừa) ⇒ sai tên/đã đổi cột;
//   - DB có cột struct không khai VÀ không nằm trong `intentionallyAbsent`
//     (thiếu không giải thích) ⇒ migration thêm cột mới mà adapter không biết.
func Test_srs_adapter_row_structs_match_real_columns(t *testing.T) {
	db := testdb.Open(t, context.Background())

	for _, tc := range []struct {
		table string
		row   interface{}
		// intentionallyAbsent là các cột thật mà row CỐ Ý không khai — row hẹp
		// theo thiết kế (`srs_adapter.go:36-38`), không phải lỗi.
		intentionallyAbsent []string
	}{
		{"decks", deckRow{}, nil},
		{"cards", cardRow{}, []string{"stability", "difficulty", "reps", "lapses", "audio_url"}},
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
			absent := map[string]bool{}
			for _, c := range tc.intentionallyAbsent {
				absent[c] = true
			}

			var extra, unexplained []string
			for c := range declared {
				if !real[c] {
					extra = append(extra, c)
				}
			}
			for c := range real {
				if !declared[c] && !absent[c] {
					unexplained = append(unexplained, c)
				}
			}
			sort.Strings(extra)
			sort.Strings(unexplained)

			assert.Empty(t, extra,
				"%s: row struct khai cột không có trong DB ⇒ sai tên hoặc schema đã đổi", tc.table)
			assert.Empty(t, unexplained,
				"%s: DB có cột row struct không khai và không nằm trong allowlist "+
					"⇒ thêm cột bằng migration mà quên adapter", tc.table)

			// Allowlist phải khớp thực tế: cột cố ý vắng phải CÓ trong DB và
			// KHÔNG được struct khai. Nếu không, allowlist đã mục và có thể che
			// lệch thật ở lần đổi sau.
			for _, c := range tc.intentionallyAbsent {
				assert.True(t, real[c],
					"%s: allowlist khai %q vắng mặt nhưng DB không còn cột đó", tc.table, c)
				assert.False(t, declared[c],
					"%s: %q vừa nằm allowlist vừa được struct khai — allowlist đã mục", tc.table, c)
			}
		})
	}
}
