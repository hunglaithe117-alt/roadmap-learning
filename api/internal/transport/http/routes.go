package httptransport

import (
	"github.com/gin-gonic/gin"
)

// registerAPI gom 5 endpoint nhị phân + health dưới prefix `/api`.
//
// Vì sao prefix `/api` và không đặt rải: app v1 đã dùng `/api/*` cho toàn bộ
// endpoint, nên giữ prefix này giữ cho client cũ chạy được trong lúc M5 port.
// Đồng thời nó TÁCH bạch với `/query` (GraphQL) — STACK-V2-PLAN §3 cấm đặt
// GraphQL ở `/api/graphql` vì sẽ đụng prefix wildcard này.
//
// Danh sách đầy đủ:
//
//	GET    /api/health    ping DB            (JSON)
//	GET    /api/tts       stream audio/wav   (binary)
//	POST   /api/stt       multipart upload   (JSON ra)
//	GET    /api/backup    download file      (M7 — 501)
//	POST   /api/restore   multipart upload   (M7 — 501)
//
// Cố ý KHÔNG có endpoint JSON nào khác ở đây: mọi use case của 6 context đã đi
// qua GraphQL `/query`. Thêm 1 endpoint JSON ở đây là mở đường cho client quay
// lại gọi tuần tự và phá vỡ đúng lý do STACK-V2 chuyển sang GraphQL.
func registerAPI(e *gin.Engine, opts Options) {
	api := e.Group("/api")
	api.GET("/health", healthHandler(opts.DB, opts.Audio, opts.Log))
	api.GET("/tts", ttsHandler(opts.TTS, opts.Log))
	api.POST("/stt", sttHandler(opts.STT, opts.Log))
	api.GET("/backup", backupHandler(opts.Backup, opts.Log))
	api.POST("/restore", restoreHandler(opts.Backup, opts.Log))
	// Trailing slash (`/api/tts/` → 301 `/api/tts`) đã có sẵn: `gin.New()` bật
	// `RedirectTrailingSlash` mặc định. Ghi ra đây để biết là CỐ Ý — bỏ nó đi
	// thì URL dựng từ cấu hình sẽ 404 và debug mất thời gian vô ích.
}
