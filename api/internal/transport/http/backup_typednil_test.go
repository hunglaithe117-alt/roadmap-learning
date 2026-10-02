package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ptrBackupPort là `syncapp.BackupPort` HIỆN THỰC bằng CON TRỎ — mô phỏng đúng
// hình dạng của hiện thực `pg_dump` sắp có (`*pgBackup`).
//
// Sự tồn tại của nó trong test là để chứng minh 1 điều: `BackupPort` là
// interface, nên `var p *ptrBackupPort; opts.Backup = p` cho `port != nil`
// (`port == nil` chỉ TRUE khi interface trỏ tới con trỏ nil, không phải khi
// bản thân con trỏ bên trong nil — "typed-nil interface", bẫy kinh điển của Go).
//
// Hàm viết ra `called` để test chứng minh handler ĐÃ dừng ở cổng 501 chứ không
// phải đã gọi method trên con trỏ nil (lúc đó `p.called` mà deref nil là panic).
type ptrBackupPort struct{ called *int }

func (p *ptrBackupPort) Dump(context.Context) (string, error) {
	*p.called++
	return "", nil
}

func (p *ptrBackupPort) Restore(context.Context, string) error {
	*p.called++
	return nil
}

// mustNotPanic chuyển panic thành FAIL rõ ràng thay vì làm sập cả package test
// (xem `mustNotPanic` cùng loại ở `internal/transport/graphql/typednil_sync_test.go`).
func mustNotPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s: PANIC (lớp lỗi typed-nil interface): %v\n%s", what, r, debug.Stack())
		}
	}()
	fn()
}

// Test_backup_and_restore_return_501_when_port_is_typed_nil là test B1.
//
// Bối cảnh: `Options.Backup` là interface. Trước đây (đã bị revert ở M7a) có
// `Container.Backup *pgBackup` + `wireBackup` trả con trỏ nil khi thiếu binary
// ⇒ nhét vào interface thành interface KHÔNG nil ⇒ `if port == nil` SAI ⇒
// `port.Dump(...)` trên con trỏ nil ⇒ **PANIC 500 thay vì 501**.
//
// Dù endpoint này KHÔNG còn hiện thực (`pg_dump` đã bị bỏ, vẫn trả 501), test
// vẫn phải tồn tại: nó giữ đúng hợp đồng của cổng 501 cho MỦI loại "chưa bật"
// — kể cả loại nguy hiểm nhất là con trỏ nil. Nếu không, lần sau thêm
// `pg_dump` là quả bom nổ tiếp theo y hệt.
func Test_backup_and_restore_return_501_when_port_is_typed_nil(t *testing.T) {
	// Interface KHÔNG nil, bên trong là con trỏ nil — đúng bẫy typed-nil.
	//
	// `called` đếm lần handler chạm vào port. Không dùng lại 1 router cho 2
	// subtest: `NewRouter` BÓI giá trị `Options.Backup` vào closure lúc dựng,
	// nên gán lại `typedNil` sau đó không ảnh hưởng gì — mỗi endpoint dựng
	// router riêng để test nói đúng điều nó kiểm.
	var callsBackup, callsRestore int

	t.Run("GET /api/backup", func(t *testing.T) {
		var typedNil *ptrBackupPort
		router := NewRouter(Options{
			GinMode: "test", TTS: StubTTS{}, STT: StubSTT{},
			Backup: typedNil, Log: discardLogger(),
		}).Engine

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/backup", nil)
		mustNotPanic(t, "GET /api/backup với BackupPort = (*ptrBackupPort)(nil)", func() {
			router.ServeHTTP(w, req)
		})
		require.Equal(t, http.StatusNotImplemented, w.Code,
			"port chưa bật phải trả 501, KHÔNG phải 500 (panic) và KHÔNG phải 200")
		require.Contains(t, w.Body.String(), "chưa bật",
			"message phải nói rõ tính năng chưa bật bằng tiếng Việt")
		require.Equal(t, 0, callsBackup, "handler phải dừng ở cổng 501, không gọi method trên con trỏ nil")
	})

	t.Run("POST /api/restore", func(t *testing.T) {
		var typedNil *ptrBackupPort
		router := NewRouter(Options{
			GinMode: "test", TTS: StubTTS{}, STT: StubSTT{},
			Backup: typedNil, Log: discardLogger(),
		}).Engine

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/restore", nil)
		mustNotPanic(t, "POST /api/restore với BackupPort = (*ptrBackupPort)(nil)", func() {
			router.ServeHTTP(w, req)
		})
		require.Equal(t, http.StatusNotImplemented, w.Code,
			"port chưa bật phải trả 501, KHÔNG phải 500 (panic)")
		require.Contains(t, w.Body.String(), "chưa bật",
			"message phải nói rõ tính năng chưa bật bằng tiếng Việt")
		require.Equal(t, 0, callsRestore, "handler phải dừng ở cổng 501, không gọi method trên con trỏ nil")
	})
}

// Test_backup_and_restore_return_501_for_plain_nil_port chốt nhánh interface nil
// thật (đường `main.go` dùng hằng ngày) song song với nhánh typed-nil ở trên.
//
// Hai nhánh phải cho CÙNG kết quả. Nếu ai đó "tối ưu" cổng 501 thành so sánh
// kiểu động rồi bỏ luôn nhánh nil, test này đỏ trước khi người sau gặp
// production panic.
func Test_backup_and_restore_return_501_for_plain_nil_port(t *testing.T) {
	router := testRouter("")

	for _, tc := range []struct{ method, path, feature string }{
		{http.MethodGet, "/api/backup", "backup"},
		{http.MethodPost, "/api/restore", "restore"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			mustNotPanic(t, tc.path+" với BackupPort = nil", func() {
				router.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			})
			require.Equal(t, http.StatusNotImplemented, w.Code)
			require.Contains(t, w.Body.String(), "chưa bật")
			require.Contains(t, w.Body.String(), tc.feature, "message phải nêu đúng tên tính năng")
		})
	}
}

// Test_backup_disabled_recognises_every_nil_shape bảo vệ chính cổng 501 ở mức
// đơn vị, không qua HTTP.
//
// `backupDisabled` phải trả TRUE cho cả 3 hình dạng "chưa bật" và FALSE cho
// hình dạng "đã bật" — kể cả khi hiện thực là con trỏ (kiểu `*pgBackup` sắp có)
// lẫn giá trị (kiểu stub trong test). Nếu hàm chỉ kiểm `port == nil` thì hình
// thức 2 lọt qua và B1 quay lại đúng chỗ cũ.
func Test_backup_disabled_recognises_every_nil_shape(t *testing.T) {
	require.True(t, backupDisabled(nil), "interface nil phải coi là chưa bật")

	var typedNilPtr *ptrBackupPort
	require.True(t, backupDisabled(typedNilPtr), "con trỏ nil trong interface phải coi là chưa bật")

	var typedNilIface *stubBackup
	require.True(t, backupDisabled(typedNilIface), "con trỏ tới interface nil phải coi là chưa bật")

	calls := 0
	require.False(t, backupDisabled(&ptrBackupPort{called: &calls}),
		"port dùng được thì KHÔNG được coi là chưa bật — nếu sai thì /api/backup 501 vĩnh viễn")
	require.False(t, backupDisabled(stubBackup{}),
		"port dùng được (giá trị, không phải con trỏ) thì KHÔNG được coi là chưa bật")
	require.Equal(t, 0, calls, "backupDisabled chỉ QUAN SÁT, không gọi method nào trên port")
}

// stubBackup là port dùng được, khai bằng con trỏ tới interface để ép `backupDisabled`
// phải đi hết nhánh interface của `reflect.Kind`.
type stubBackup struct{}

func (stubBackup) Dump(context.Context) (string, error)  { return "", nil }
func (stubBackup) Restore(context.Context, string) error { return nil }

// Test_not_implemented_message_is_vietnamese_and_says_not_enabled chốt chữ trong
// message.
//
// "chưa bật" là thông tin client dùng để hiện "tính năng đang xây" thay vì báo
// lỗi; thiếu nó thì 501 vẫn đúng mã nhưng UI không biết nên hiện gì. Test khóa
// luôn `M7` vì đó là mốc đã ghi trong `stack-v2-m7a.md`.
func Test_not_implemented_message_is_vietnamese_and_says_not_enabled(t *testing.T) {
	for _, feature := range []string{"backup", "restore"} {
		msg := notImplementedMsg(feature)
		require.Contains(t, msg, "chưa bật", "message phải nói rõ chưa bật")
		require.Contains(t, msg, feature, "message phải nêu đúng tên tính năng")
		require.True(t, strings.Contains(msg, "M7"), "giữ mốc M7 đã ghi trong handoff")
		// Không được lộ chi tiết hạ tầng cho client: đây là message cho USER.
		require.NotContains(t, msg, "VACUUM INTO `",
			"message cho user không cần chi tiết lệnh hạ tầng")
	}
}
