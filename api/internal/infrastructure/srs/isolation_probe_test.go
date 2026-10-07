package srsinfra_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"langapp/internal/platform/testdb"
)

// TestIsolatedProbeOpensSchema là "mồi" cho
// `testdb.Test_go_test_process_exits_nonzero_against_app_database`.
//
// Nó tồn tại để subprocess `go test` chạy được MỘT test DB thật sự tới
// `testdb.OpenSchema` — chỗ duy nhất guard cách ly canh. Nếu probe chỉ là
// `-run` không khớp gì, thì `TestMain` có thể là lớp fail duy nhất và ta
// không chứng minh được gì về guard (đúng bài học mutation ở `gate_test.go`
// §2.3: một lớp có thể che lớp kia).
//
// Tên đặt theo tiền tố `TestIsolatedProbe` để `-run` của subprocess khớp chắc
// chắn mà không vô tình kéo theo cả suite nặng của package.
//
// Với DSN đúng (database test) test này XANH — và đó là hành vi mong muốn:
// nó chứng minh đường thông thường vẫn chạy được. Với DSN trỏ database app
// thì PHẢI đỏ, kèm thông báo "CHỨA DỮ LIỆU APP".
func TestIsolatedProbeOpensSchema(t *testing.T) {
	db, schema := testdb.OpenSchema(t, context.Background())
	require.NotNil(t, db)
	require.NotEmpty(t, schema)

	// Schema trả về phải là schema test tạm (tiền tố `t_test_`) chứ không
	// phải `public`/`testext`. Đây là assert nhẹ nhưng bắt được trường hợp
	// `search_path` bị đảo thứ tự — mà đảo thứ tự thì bảng test rơi vào
	// schema của extension và không bao giờ được dọn.
	require.Contains(t, schema, "t_test_", "schema trả về phải là schema test tạm")
}
