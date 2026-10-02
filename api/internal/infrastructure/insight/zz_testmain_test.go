package insightinfra

import (
	"testing"

	platformtestdb "langapp/internal/platform/testdb"
)

// TestMain chặn "test xanh vì không chạy": thiếu `LANGAPP_TEST_POSTGRES_DSN`
// thì FAIL cả package thay vì im lặng skip toàn bộ suite DB.
//
// `TestMain` phải nằm trong package IN-PACKAGE (không phải `_test`) vì Go chỉ
// cho phép 1 `TestMain` mỗi thư mục test — mà các thư mục này có cả test
// in-package (cần `txHandle` không export) lẫn test external.
//
// Gate có hiệu lực cho MỌI test của thư mục, kể cả các file `*_test.go`
// external, nên một chỗ đặt là đủ. Xem `testdb.Main` để biết vì sao thiếu DSN
// phải fail thay vì skip.
func TestMain(m *testing.M) { platformtestdb.Main(m) }
