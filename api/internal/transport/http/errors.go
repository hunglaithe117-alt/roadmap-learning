package httptransport

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"langapp/internal/transport/apperr"
)

// writeJSONError trả lỗi dạng JSON `{"error": "..."}` — đúng shape của app v1
// (api/audio.go `writeJSONError`) để client không phải đổi code đọc lỗi khi
// chuyển từ REST sang GraphQL.
func writeJSONError(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"error": msg})
}

// writeAppError trả lỗi của application mà KHÔNG dịch message.
//
// `*application/errors.Error` đã mang sẵn message tiếng Việt đúng loại nút
// (doc của `srs.Error` ghi rõ: "không phải dịch lại chuỗi message"). Lỗi không
// phải nghiệp vụ (GORM/driver) thì log ở tầng trên rồi trả message chung — vì
// message gốc chứa câu SQL và tên bảng.
func writeAppError(c *gin.Context, err error) {
	if !apperr.IsBusiness(err) {
		_ = c.Error(err)
		writeJSONError(c, http.StatusInternalServerError, "lỗi hệ thống")
		return
	}
	writeJSONError(c, apperr.Status(err), err.Error())
}
