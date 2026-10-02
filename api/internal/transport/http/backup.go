package httptransport

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	syncapp "langapp/internal/application/sync"
	"langapp/internal/typednil"
)

// backupTimeout là trần cho `pg_dump` / `pg_restore`. DB local của app
// single-user vài chục MB ⇒ 1 phút là rộng rãi; vượt trần thì 504 còn hơn treo
// request giữ connection.
const backupTimeout = time.Minute

// restoreFileField là tên field multipart chứa file backup. Khác `audio` của
// `/api/stt` vì 2 endpoint có ý nghĩa khác nhau, và dùng chung tên sẽ khiến 1
// form gửi nhầm còn 2/3.
const restoreFileField = "file"

// maxRestoreBytes là trần upload của `/api/restore`. Khớp `maxSTTBytes`: cùng
// là "1 file do browser gửi lên" nên cùng trần cho dễ nhớ.
const maxRestoreBytes int64 = 10 << 20

// backupHandler `GET /api/backup` → download file snapshot.
//
// M4 KHÔNG implement `BackupPort`: app v1 dùng `VACUUM INTO` — CÚ PHÁP KHÔNG
// TỒN TẠI ở Postgres (STACK-V2-PLAN §4.6). Tương đương là `pg_dump`, thuộc M7
// (cần `pg_dump` trong image + test 2 máy hội tụ). Trong lúc đó endpoint trả
// **501 Not Implemented** với message nói rõ, thay vì 500 "chưa sẵn sàng" —
// 501 là mã đúng nghĩa (chưa có) và client có thể hiện "tính năng đang xây"
// thay vì báo lỗi chung chung.
func backupHandler(port syncapp.BackupPort, log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if backupDisabled(port) {
			writeJSONError(c, http.StatusNotImplemented, notImplementedMsg("backup"))
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), backupTimeout)
		defer cancel()
		path, err := port.Dump(ctx)
		if err != nil {
			_ = c.Error(err)
			writeJSONError(c, http.StatusInternalServerError, "lỗi hệ thống")
			return
		}
		defer os.Remove(path)
		c.Header("Content-Disposition", `attachment; filename="langapp-`+time.Now().UTC().Format("20060102-150405")+`.dump"`)
		// `application/octet-stream` chứ không phải tên type của pg_dump: file
		// dump là định dạng riêng, client chỉ cần tải về rồi nạp lại.
		c.Header("Content-Type", "application/octet-stream")
		c.File(path)
	}
}

// restoreHandler `POST /api/restore` multipart field `file` → ghi đè từ
// snapshot.
//
// KHÁC HẲN `sync.Merge`: merge hợp nhất LWW, restore GHI ĐÈ. `BackupPort.Restore`
// nhận đường dẫn file, nên handler ghi upload ra file tạm rồi trả đường dẫn đó —
// application không được biết HTTP, và không được nhận `io.Reader` (thứ khiến
// nó phải giữ file tạm sống mãi).
func restoreHandler(port syncapp.BackupPort, log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if backupDisabled(port) {
			writeJSONError(c, http.StatusNotImplemented, notImplementedMsg("restore"))
			return
		}
		if !isMultipart(c) {
			writeJSONError(c, http.StatusBadRequest, "thiếu file backup (multipart)")
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRestoreBytes+1024)
		if err := c.Request.ParseMultipartForm(maxRestoreBytes); err != nil {
			writeJSONError(c, http.StatusBadRequest, "multipart không hợp lệ: "+err.Error())
			return
		}
		src, _, err := c.Request.FormFile(restoreFileField)
		if err != nil {
			writeJSONError(c, http.StatusBadRequest, "thiếu file backup (field \""+restoreFileField+"\")")
			return
		}
		defer src.Close()

		// File tạm trong `os.TempDir` chứ không phải thư mục upload của Gin: file
		// backup có thể là .dump (nhị phân) mà `http.Dir` phục vụ ở static —
		// để trong web/dist là mời tải file DB ra ngoài.
		tmp, err := os.CreateTemp("", "langapp-restore-*.dump")
		if err != nil {
			_ = c.Error(err)
			writeJSONError(c, http.StatusInternalServerError, "lỗi hệ thống")
			return
		}
		path := tmp.Name()
		defer os.Remove(path)

		written, err := io.Copy(tmp, io.LimitReader(src, maxRestoreBytes+1))
		closeErr := tmp.Close()
		if err != nil || closeErr != nil {
			_ = c.Error(err)
			writeJSONError(c, http.StatusInternalServerError, "không lưu được file tạm")
			return
		}
		if written > maxRestoreBytes {
			writeJSONError(c, http.StatusBadRequest, "file backup quá lớn (tối đa 10MB)")
			return
		}
		if written == 0 {
			writeJSONError(c, http.StatusBadRequest, "file backup rỗng")
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), backupTimeout)
		defer cancel()
		if err := port.Restore(ctx, path); err != nil {
			log.Error("restore thất bại", slog.String("path", filepath.Base(path)), slog.String("err", err.Error()))
			writeJSONError(c, http.StatusInternalServerError, "restore thất bại")
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "restored": written})
	}
}

// backupDisabled báo tính năng backup/restore có dùng được hay không.
//
// DÙNG HÀM NÀY, KHÔNG `port == nil` — đó là toàn bộ nội dung của B1.
//
// `syncapp.BackupPort` là INTERFACE. Khi hiện thực (`pg_dump`, M7) là con trỏ —
// kiểu `*pgBackup` — thì wiring kiểu cũ (`wireBackup` trả `(*pgBackup)(nil)` khi
// thiếu binary) đặt CON TRỎ NIL vào interface. Interface nhét con trỏ nil KHÔNG
// nil: `port == nil` cho FALSE, nên handler đi thẳng vào `port.Dump(ctx)` —
// method trên con trỏ nil ⇒ deref nil ⇒ **PANIC 500 thay vì 501**.
//
// Nói cách khác: `port == nil` trả lời sai chính câu hỏi mà nó được hỏi. Nó
// hỏi "interface này có chứa con trỏ nil không" chứ không phải "có dùng được
// không", và 2 câu đó khác nhau đúng lúc quan trọng nhất.
//
// `typednil.Is` soi tới tận giá trị bên trong, nên mọi hình dạng "chưa bật"
// — nil thật, con trỏ nil, con trỏ tới interface nil — đều ra cùng một kết quả,
// và kết quả đó KHÔNG phụ thuộc vào việc người sau hiện thực port bằng con trỏ
// hay giá trị.
func backupDisabled(port syncapp.BackupPort) bool {
	return typednil.Is(port)
}

func notImplementedMsg(what string) string {
	// "chưa bật" là từ client cần để hiện "tính năng đang xây" thay vì báo lỗi.
	// Giữ `M7` vì đó là mốc đã ghi trong `phases/task-memory/stack-v2-m7a.md`.
	return "tính năng " + what + " chưa bật: Postgres không có `VACUUM INTO`, cần `pg_dump`/`pg_restore` (M7)"
}

func isMultipart(c *gin.Context) bool {
	return strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/")
}
