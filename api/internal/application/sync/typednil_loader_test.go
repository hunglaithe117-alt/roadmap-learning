package sync

import (
	"context"
	"errors"
	"runtime/debug"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domain "langapp/internal/domain/sync"
	"langapp/internal/typednil"
)

// Lớp lỗi typed-nil interface — lần thứ 3 trong dự án (xem
// `phases/task-memory/stack-v2-typednil.md`).
//
// Test ở file này phủ TẦNG APPLICATION, tức `Service.Sync`. Test ở
// `internal/transport/graphql/typednil_sync_test.go` phủ đường đầu-cuối qua
// `graph/client`, nhưng đường đó đi qua `platform.Wire` — mà `Wire` sau khi sửa
// đã truyền `nil` thật, nên `s.loader == nil` cũng bắt được và test đó KHÔNG
// chứng minh được tầng này có việc gì.
//
// Nếu không có file này thì đổi `typednil.Is` về `== nil` ở đây sẽ không ai
// phát hiện — và lớp chặn thứ 2 biến mất trong im lặng.

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

// typedNilLoader là `SnapshotLoader` hiện thực bằng CON TRỎ — đúng hình dạng
// của `syncinfra.SchemaLoader`. Method có receiver con trỏ và deref field nên
// gọi trên nil sẽ nổ thật (không phải nổ giả).
type typedNilLoader struct{ called *bool }

func (l *typedNilLoader) Load(context.Context) (domain.PeerSnapshot, error) {
	*l.called = true
	return domain.PeerSnapshot{}, nil
}

// usableLoader là loader THẬT (con trỏ không nil) — dùng để chứng minh lớp chặn
// không nuốt nhầm loader dùng được. Tách khỏi `typedNilLoader` để 2 ý nghĩa
// không lẫn: 1 cái để chứng minh "bắt được", 1 cái để chứng minh "bỏ qua".
type usableLoader struct{ calls *int }

func (l *usableLoader) Load(context.Context) (domain.PeerSnapshot, error) {
	*l.calls++
	return domain.PeerSnapshot{SchemaVersion: 3}, nil
}

// Test_Sync_rejects_typed_nil_loader là test cốt lõi của lớp chặn thứ 2.
//
// Kịch bản: `NewService(repo, uow, (*typedNilLoader)(nil), now)`. Con trỏ nil
// nhét vào tham số `SnapshotLoader` (interface) ⇒ `s.loader == nil` FALSE ⇒ code
// đi vào `s.loader.Load(ctx)` ⇒ deref nil ⇒ PANIC.
//
// Đây CHÍNH XÁT là bug B2 đã gặp ở production (`Wire` từng truyền
// `NewSchemaLoader(nil)`), chỉ khác là test này gọi thẳng tầng application nên
// không cần Postgres và chạy được ở mọi lần `go test`.
func Test_Sync_rejects_typed_nil_loader(t *testing.T) {
	repo := &fakeRepo{}
	uow := &fakeUOW{repo: repo}
	var loader SnapshotLoader = (*typedNilLoader)(nil)

	svc := NewService(repo, uow, loader, func() time.Time { return fixedNow })

	// Tiền đề: chứng minh đây đúng là bẫy, không phải test viết cho có.
	require.False(t, loader == nil, "tiền đề: con trỏ nil trong interface KHÔNG bằng nil")

	var (
		res MergeResult
		err error
	)
	mustNotPanic(t, "Service.Sync với loader = (*typedNilLoader)(nil)", func() {
		res, err = svc.Sync(context.Background())
	})

	require.Error(t, err, "loader nil phải ra LỖI, không được đi vào Load")
	require.False(t, res.OK)
	require.Contains(t, err.Error(), "chưa cấu hình",
		"message phải nói rõ nguyên nhân bằng tiếng Việt")

	// Phải là lỗi NGHIỆP VỤ, không phải 500 — vì `toUserError` ở transport che
	// message của lỗi 500 thành "lỗi hệ thống", và test ở tầng graphql sẽ
	// không thấy được chữ "chưa cấu hình". Gắn status vào đây để lỗi sai loại
	// bị bắt ngay ở tầng thấp, không phải sau khi đã đóng băng UI.
	var appErr *Error
	require.True(t, errors.As(err, &appErr), "phải là *sync.Error để transport đọc được status")
	require.NotEqual(t, StatusInternalServerError, appErr.Status,
		"'chưa cấu hình' là lỗi nghiệp vụ, không phải lỗi hệ thống — status 500 sẽ bị toUserError che mất message")
}

// Test_Sync_never_reports_conflict_for_missing_peer khoá mã 501, và cấm 409.
//
// Vì sao cần cả 2 vế:
//
//  1. **Phải là 501.** 409 Conflict nghĩa là "yêu cầu mâu thuẫn với trạng thái
//     hiện tại" — đúng cho merge thật sự gặp xung đột dữ liệu. Nhánh này thì
//     KHÔNG có mâu thuẫn: app chưa có nguồn snapshot peer, và `Sync` bị từ
//     chối trước khi đọc/ghi bất cứ thứ gì. Client nhận 409 sẽ hiểu "dữ liệu
//     của tôi xung đột với peer" — kết luận sai mà user không cách nào tự
//     kiểm chứng, vì app không có peer để đối chiếu.
//  2. **KHÔNG BAO GIỜ 409.** Gate M6 soi ra 409 có vẻ được chọn chỉ để thoát
//     assert `NotEqual("INTERNAL")` trong test tầng graphql chứ không vì đúng
//     nghĩa. Test này viết ra để lần sau không ai "quay về 409 cho quen" —
//     nếu có ý định đổi mã thì phải sửa test này KÈM comment, để người đọc
//     thấy đây là quyết định chứ không là sơ suất.
//
// 503 cũng bị cấm: 503 là "tạm thời không phục vụ được, thử lại sau" — thiếu
// cấu hình thì thử lại bao nhiêu lần cũng y hệt.
func Test_Sync_never_reports_conflict_for_missing_peer(t *testing.T) {
	// Mọi hình dạng "chưa có loader" phải cho CÙNG status: nếu 1 hình trả
	// 409 và 1 hình trả 501 thì client gặp 2 mã cho cùng 1 nguyên nhân.
	//
	// CHỈ 2 hình là khả thi ở call site này, và đó là điều Go bảo đảm chứ không
	// phải giới hạn của test: `Service.loader` có kiểu cụ thể `SnapshotLoader`
	// (interface), nên Go KHÔNG cho `*SnapshotLoader` thỏa `SnapshotLoader` —
	// hình "con trỏ tới 1 interface đang nil" không thể dựng được ở đây. Hình
	// đó vẫn là ranh giới thật của `typednil.Is` và được khoá ở
	// `internal/typednil` (`Test_Is_pierces_pointer_to_nil_interface`).
	nilLoaders := map[string]SnapshotLoader{
		"interface nil thật": nil,
		"con trỏ nil":        (*typedNilLoader)(nil),
	}
	for name, loader := range nilLoaders {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{}
			uow := &fakeUOW{repo: repo}
			svc := NewService(repo, uow, loader, func() time.Time { return fixedNow })

			_, err := svc.Sync(context.Background())
			require.Error(t, err)

			var appErr *Error
			require.True(t, errors.As(err, &appErr), "phải là *sync.Error để transport đọc được status")

			require.Equal(t, StatusNotImplemented, appErr.Status,
				"'chưa cấu hình nguồn snapshot peer' là TÍNH NĂNG CHƯA BẬT (501), không phải merge conflict (409)")
			require.NotEqual(t, StatusConflict, appErr.Status,
				"409 nghĩa là 'dữ liệu của tôi xung đột với peer' — ở đây KHÔNG có peer nào để xung đột")
			require.NotEqual(t, 503, appErr.Status,
				"503 nghĩa là 'tạm thời, thử lại sau'; thiếu cấu hình thì thử lại vô ích")
			require.NotEqual(t, StatusInternalServerError, appErr.Status,
				"500 bị toUserError che thành \"lỗi hệ thống\" ⇒ user không đọc được vì sao")
		})
	}
}

// Test_Sync_missing_peer_message_stays_vietnamese khoá hợp đồng "user đọc
// được lý do" ở tầng application.
//
// Đây là điều kiện tối thiểu để việc đổi mã 500→409→501 có ý nghĩa: nếu
// `toUserError` ở transport che message đi, thì dù status đúng 501 user vẫn
// chỉ thấy "lỗi hệ thống" và quay về đúng tình trạng trước M7c.
func Test_Sync_missing_peer_message_stays_vietnamese(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, &fakeUOW{repo: repo}, nil, func() time.Time { return fixedNow })

	_, err := svc.Sync(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "chưa cấu hình nguồn snapshot peer",
		"client cần đọc được nguyên nhân bằng tiếng Việt, không phải mã lỗi trừng trơng")
}

// Test_Sync_rejects_plain_nil_loader giữ nguyên hành vi của hợp đồng cũ — cổng
// này có từ trước B2, chỉ là trước đó không ai test. Không có test thì một đợt
// "dọn code" có thể xoá nhầm cổng mà không ai kêu.
func Test_Sync_rejects_plain_nil_loader(t *testing.T) {
	repo := &fakeRepo{}
	uow := &fakeUOW{repo: repo}
	svc := NewService(repo, uow, nil, func() time.Time { return fixedNow })

	res, err := svc.Sync(context.Background())
	require.Error(t, err)
	require.False(t, res.OK)
	require.Contains(t, err.Error(), "chưa cấu hình")

	var appErr *Error
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, StatusNotImplemented, appErr.Status)
}

// Test_Sync_still_merges_when_loader_is_real bảo vệ chiều chạy thật.
//
// Thêm `typednil.Is` dễ gây hại theo hướng ngược: soi quá tay rồi chặn nhầm
// loader ĐANG DÙNG ĐƯỢC. Test này là đối trọng bắt buộc cho mọi test "phải chặn".
//
// Dùng loader hiện thực bằng CON TRỎ (không phải `fakeLoader` là giá trị) vì
// đó mới là hình thức mà `typednil.Is` buộc phải phân biệt: nếu hàm chỉ nhìn
// `Kind() == Pointer` mà quên `IsNil()`, thì chính loader con trỏ này bị chặn
// nhầm — và app v1 sẽ hỏng sync mà test "phải chặn" vẫn xanh.
func Test_Sync_still_merges_when_loader_is_real(t *testing.T) {
	repo := &fakeRepo{schemaV: 3, peerV: 3}
	uow := &fakeUOW{repo: repo}
	calls := 0
	var loader SnapshotLoader = &usableLoader{calls: &calls}

	require.False(t, typednil.Is(loader), "tiền đề: con trỏ KHÔNG nil là loader dùng được")

	svc := NewService(repo, uow, loader, func() time.Time { return fixedNow })
	res, err := svc.Sync(context.Background())

	require.NoError(t, err, "loader thật thì Sync phải chạy tới merge, không bị guard chặn nhầm")
	require.True(t, res.OK)
	require.Equal(t, 1, calls, "Load phải được gọi đúng 1 lần")
}
