package syncinfra_test

import (
	"context"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"

	syncinfra "langapp/internal/infrastructure/sync"
	"langapp/internal/platform/testdb"
)

// Lớp lỗi "nil đi vào interface/đối tượng dùng như đã cấu hình" — lần thứ 3
// trong dự án (xem `phases/task-memory/stack-v2-typednil.md`).
//
// Ở `infrastructure/sync` có HAI chỗ giữ con trỏ `*gorm.DB` có thể nil:
// `SchemaLoader.peer` và `Repository.peer`. Cả hai đều được `Wire` truyền `nil`
// trong app thật (`NewSchemaLoader(nil)`, `NewPeerRepository(db, nil)`) vì app
// v1 không có máy peer.
//
// Trước khi sửa, gọi method trên chúng là `l.peer.WithContext(ctx)` với
// `l.peer == nil` ⇒ deref nil ⇒ PANIC. Hai chỗ này KHÔNG cùng lớp typed-nil như
// B2 (con trỏ là kiểu cụ thể `*gorm.DB`, `== nil` của Go là đúng) — nhưng đường
// tới panic thì giống hệt, và người đọc `Wire` một mình rất dễ không thấy.

// mustNotPanic biến panic thành FAIL rõ ràng thay vì làm sập cả package test.
func mustNotPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s: PANIC: %v\n%s", what, r, debug.Stack())
		}
	}()
	fn()
}

// Test_SchemaLoader_Load_without_peer_returns_error_not_panic là lớp chặn thứ 2
// cho B2, đúng hình thức gốc.
//
// `NewSchemaLoader(nil)` trả về con trỏ **KHÔNG nil** bọc `peer` nil. Ở đó
// `typednil.Is` ở tầng application trả FALSE (con trỏ ngoài cùng không nil) nên
// tầng trên không chặn được — chỉ chỗ này mới nói thật.
//
// Test này cố tình bỏ qua `Service.Sync` (cổng typed-nil) và gọi thẳng
// `Load`, vì đó mới là nơu cần lớp chặn thứ 2. Nếu 2 tầng trùng nhau thì một
// trong hai đã thừa — và thừa ở đây là tốt, nhưng phải biết là thừa vì lý do.
func Test_SchemaLoader_Load_without_peer_returns_error_not_panic(t *testing.T) {
	loader := syncinfra.NewSchemaLoader(nil)

	var err error
	mustNotPanic(t, "SchemaLoader.Load với peer=nil", func() {
		_, err = loader.Load(context.Background())
	})

	require.Error(t, err, "thiếu pool peer thì phải là LỖI, không phải im lặng trả snapshot rỗng")
	require.Contains(t, err.Error(), "chưa cấu hình",
		"message phải nói rõ nguyên nhân bằng tiếng Việt")
}

// Test_SchemaLoader_Load_on_nil_receiver_returns_error_not_panic phủ nốt hình
// thức `(*SchemaLoader)(nil)` — con trỏ nil ở CHÍNH receiver.
//
// Khác `NewSchemaLoader(nil)` ở trên: ở đây bản thân `loader` là nil, nên
// method gọi vào được (Go cho phép gọi method trên con trỏ nil) nhưng đọc
// `l.peer` sẽ nổ. Đây là hình thức người khác dễ tạo ra nhất khi viết adapter
// mới, nên chặn luôn.
func Test_SchemaLoader_Load_on_nil_receiver_returns_error_not_panic(t *testing.T) {
	var loader *syncinfra.SchemaLoader

	var err error
	mustNotPanic(t, "SchemaLoader.Load với receiver nil", func() {
		_, err = loader.Load(context.Background())
	})
	require.Error(t, err)
}

// Test_Repository_PeerSchemaVersion_without_peer_returns_error_not_panic chặn
// chỗ thứ 2 giữ `*gorm.DB` nil.
//
// Khác `SchemaLoader`: ở đây KHÔNG dùng `typednil` được, vì `Repository` là
// struct cụ thể và `peer` là `*gorm.DB` cụ thể — so sánh `== nil` là ĐÚNG. Bẫy
// chỉ nằm ở chỗ call site quên kiểm.
func Test_Repository_PeerSchemaVersion_without_peer_returns_error_not_panic(t *testing.T) {
	db := testdb.Open(t, context.Background())
	// Đúng những gì `platform.Wire` làm: `peer` là nil trong app thật.
	repo := syncinfra.NewPeerRepository(db, nil)

	var err error
	mustNotPanic(t, "Repository.PeerSchemaVersion với peer=nil", func() {
		_, err = repo.PeerSchemaVersion(context.Background())
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "chưa cấu hình",
		"message phải nói rõ nguyên nhân bằng tiếng Việt")
}

func Test_Repository_PeerSchemaVersion_on_nil_receiver_returns_error_not_panic(t *testing.T) {
	var repo *syncinfra.Repository

	var err error
	mustNotPanic(t, "Repository.PeerSchemaVersion với receiver nil", func() {
		_, err = repo.PeerSchemaVersion(context.Background())
	})
	require.Error(t, err)
}

// Test_Repository_with_peer_still_works bảo vệ chiều đọc: lớp chặn mới KHÔNG
// được nuốt mất chức năng đang chạy.
//
// Đây là loại hỏng dễ nhất khi thêm guard: viết `if r.peer == nil { return err }`
// rồi đặt nhầm ở chỗ khác, hoặc đặt đúng chỗ nhưng quên rằng `peer` thật sự
// được truyền ở test 2 máy. Test này chứng minh đường có peer vẫn đọc được.
func Test_Repository_with_peer_still_works(t *testing.T) {
	local, peer := twoMachines(t)
	repo := syncinfra.NewPeerRepository(local, peer)

	v, err := repo.PeerSchemaVersion(context.Background())
	require.NoError(t, err, "có peer thật thì đọc version phải chạy — guard mới không được chặn nhầm")
	require.Greater(t, v, 0, "schema peer đã migrate nên version phải > 0")
}

// Test_SchemaLoader_with_peer_still_works — cùng lý do với trên, cho `SchemaLoader`.
func Test_SchemaLoader_with_peer_still_works(t *testing.T) {
	_, peer := twoMachines(t)
	loader := syncinfra.NewSchemaLoader(peer)

	mustNotPanic(t, "SchemaLoader.Load với peer thật", func() {
		_, err := loader.Load(context.Background())
		require.NoError(t, err, "đọc snapshot peer thật phải chạy — guard mới không được chặn nhầm")
		// Cố ý KHÔNG assert gì về nội dung snapshot: schema peer vừa migrate thì
		// rỗng là đúng, còn nội dung đã có test riêng (`remediation_test.go`).
		// Ở đây chỉ cần chứng minh đúng 1 điều: có peer thật thì KHÔNG bị guard
		// mới chặn nhầm.
	})
}
