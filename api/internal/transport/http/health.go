package httptransport

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// healthTimeout là trần cho 1 lần `PingContext`. `pg_isready` mất vài chục ms
// trên loopback nhưng vài giây khi DB đang bận `CREATE INDEX`; quá 2s thì coi
// như hỏng, vì healthcheck cần trả lời nhanh để Docker quyết định restart.
const healthTimeout = 2 * time.Second

// Pinger là phần interface của `*sql.DB` mà health handler cần.
//
// Không nhận `*gorm.DB` để `internal/transport/http` không import driver DB
// (STACK-V2-PLAN §2: infrastructure là nơi duy nhất import gorm). `platform`
// lấy `*sql.DB` ra từ GORM rồi truyền vào đây.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// healthHandler `GET /api/health` cho `HEALTHCHECK` của Docker.
//
// Giữ nguyên shape của app v1 (`{"status":"ok"}`) vì Dockerfile v1 và
// `docker-compose.yml` đang so đúng chuỗi đó — đổi shape là HEALTHCHECK đỏ
// trong khi app vẫn chạy, và người vận hành debug nhầm vào Postgres.
//
// Phạm vi kiểm tra CỐ Ý HẸP: chỉ ping DB. Không kiểm tra audio-service, không
// kiểm tra seed — audio là tuỳ chọn (thiếu thì fallback stub vẫn chạy được), còn
// Postgres thì không có thì app không làm được gì cả. Healthcheck trả lời "sống
// hay chết", không phải "khoẻ mạnh tới mức nào".
//
// `audio` là func đọc TỨC THÌ (xem `Options.Audio`): trạng thái engine đổi sau
// request audio đầu tiên nên không thể chụp 1 bản lúc boot. Trước M4-remediation
// chỗ này nhận `*EngineInfo` lúc boot ⇒ `degraded` mãi ⇒ HEALTHCHECK đỏ vĩnh
// viễn (F3).
func healthHandler(db Pinger, audio func() (string, bool), log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), healthTimeout)
		defer cancel()
		if db == nil {
			writeJSONError(c, http.StatusServiceUnavailable, "chưa cấu hình database")
			return
		}
		if err := db.PingContext(ctx); err != nil {
			_ = c.Error(err)
			writeJSONError(c, http.StatusServiceUnavailable, "database không sẵn sàng")
			return
		}
		status := "ok"
		// `degraded` vẫn trả 200: app DÙNG ĐƯỢC với engine stub, còn trả 503
		// sẽ khiến Docker restart app vô ích. Cảnh báo nằm ở `status` + log để
		// người vận hành thấy mà không để app chết vì thiếu tuỳ chọn.
		if audio != nil {
			if name, real := audio(); !real {
				status = "degraded"
				log.Warn("audio đang dùng engine stub (không có audio-service)",
					slog.String("engine", name))
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": status})
	}
}
