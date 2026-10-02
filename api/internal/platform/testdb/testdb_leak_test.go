package testdb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// countTestSchemas đếm schema test còn sót trong database.
//
// Đây là đơn vị đo của F7: "vá testdb để lại lỗi cũ cùng loại". Cơ chế để lại
// 61 schema rác ở M1/M2 là `openSchema` gọi `s.drop(schema, nil)` khi
// `openTestDB` fail — `drop` return sớm khi `db == nil` nên lệnh drop không bao
// giờ chạy, trong khi schema đã `CREATE` ở dòng trên vẫn còn.
func countTestSchemas(t *testing.T, ctx context.Context, s *Session) int {
	t.Helper()
	require.NotNil(t, s.conn, "cần conn của khoá để đếm schema")
	var n int
	err := s.conn.QueryRowContext(ctx,
		"SELECT count(*) FROM pg_namespace WHERE nspname LIKE 't\\_test\\_%'").Scan(&n)
	require.NoError(t, err)
	return n
}

// Test dropSchema dọn được schema đã CREATE dù chưa từng mở pool.
//
// Đây đúng là kịch bản F7: schema được tạo, rồi `openTestDB` fail ⇒ không có
// `*gorm.DB` nào để đưa cho `drop`. Test này dựng đúng shape đó (CREATE bằng
// conn của khoá, không có pool) và khẳng định schema biến mất.
//
// Nếu ai đó đổi lại `openSchema` sang `s.drop(schema, nil)`, hàm này vẫn xanh
// — nên `Test_no_schema_leaks_after_a_full_suite_style_cycle` mới là cái bắt
// đúng đường đi production. Cả hai đều cần: một cái bắt hợp đồng của
// `dropSchema`, một cái bắt hành vi quan sát được.
func Test_dropSchema_removes_a_schema_that_never_got_a_pool(t *testing.T) {
	ctx := context.Background()
	s := Acquire(t, ctx)
	before := countTestSchemas(t, ctx, s)

	// `CREATE SCHEMA` y hệt dòng 149 của `openSchema` — qua conn của khoá,
	// không qua pool test. Không có `*gorm.DB` nào tồn tại ở đây.
	schema := fmt.Sprintf("t_test_%d", time.Now().UnixNano())
	_, err := s.conn.ExecContext(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	require.Equal(t, before+1, countTestSchemas(t, ctx, s), "schema vừa tạo phải được đếm")

	require.NoError(t, s.dropSchema(ctx, schema))
	require.Equal(t, before, countTestSchemas(t, ctx, s),
		"schema phải biến mất — nếu còn thì đây chính là đường rò của F7")

	// Dọn 2 lần phải cho lỗi (schema không còn), không phải thành công âm
	// thầm: lỗi "schema không tồn tại" là thông tin để debug cạnh tranh tên.
	require.Error(t, s.dropSchema(ctx, schema))
}

// Vòng đời đầy đủ qua `OpenSchema` (migrate + cleanup) phải KHÔNG rò schema.
//
// Schema được `OpenSchema` dọn trong `t.Cleanup` của chính test đó, mà
// `t.Cleanup` chạy theo thứ tự LIFO. Vì vậy assertion phải được đăng ký
// TRƯỚC mọi `OpenSchema` thì nó mới chạy SAU CÙNG — đếm trong thân test sẽ
// thấy đủ 10 schema "đang chờ dọn" và báo rò oan.
func Test_no_schema_leaks_after_a_full_suite_style_cycle(t *testing.T) {
	ctx := context.Background()
	s := Acquire(t, ctx)
	before := countTestSchemas(t, ctx, s)

	// Đăng ký TRƯỚC: sẽ là cleanup cuối cùng, chạy sau khi mọi
	// `t.Cleanup` của `OpenSchema` đã dọn xong.
	t.Cleanup(func() {
		require.Equal(t, before, countTestSchemas(t, ctx, s),
			"mỗi OpenSchema phải tự dọn schema của nó — dư là rò")
	})

	// 5 vòng × 2 schema = 10 lần dựng + dọn, đúng hình dạng của package test
	// nặng nhất (`go test ./...` chạy 25 package).
	for round := 0; round < 5; round++ {
		for i := 0; i < 2; i++ {
			db, _ := s.OpenSchema(t, ctx)
			require.NotNil(t, db)
		}
	}
}

// `dropSchema` khi `s.conn` rỗng phải báo lỗi, không panic và không "thành công
// âm thầm" — nếu im lặng thì lại quay về đúng cái lỗi F7.
func Test_dropSchema_reports_error_when_there_is_no_lock_conn(t *testing.T) {
	s := &Session{}
	err := s.dropSchema(context.Background(), "t_test_abc")
	require.Error(t, err)
	require.Contains(t, err.Error(), "khoá")
}
