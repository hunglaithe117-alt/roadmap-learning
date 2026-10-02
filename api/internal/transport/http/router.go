package httptransport

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	syncapp "langapp/internal/application/sync"
	audiodomain "langapp/internal/domain/audio"
)

// ServerTimeouts là giá trị timeout của `http.Server`.
//
// Bắt buộc vì gosec G112: `http.Server` không set timeout ⇒ 1 client mở
// connection rồi gửi body cực chậm sẽ giữ connection vô hạn, và vài connection
// như vậy là hết pool.
//
// Cụ thể cho app này:
//   - ReadHeaderTimeout 5s: chặn Slowloris (client mở socket không gửi header).
//   - ReadTimeout 30s: `/api/restore` là multipart upload nên cần chỗ, nhưng
//     upload > 30s là client hỏng chứ không phải người dùng chờ.
//   - WriteTimeout 60s: dài hơn ReadTimeout vì `/api/tts` tổng hợp audio (Piper
//     mất ~1s, câu dài vài giây) và `/api/backup` ghi file ra đĩa.
//   - IdleTimeout 120s: connection không dùng thì đóng, để pool connection không
//     bị giữ vô tận bởi client im lặng.
type ServerTimeouts struct {
	ReadHeader time.Duration
	Read       time.Duration
	Write      time.Duration
	Idle       time.Duration
}

// DefaultServerTimeouts là bộ giá trị ở trên.
func DefaultServerTimeouts() ServerTimeouts {
	return ServerTimeouts{
		ReadHeader: 5 * time.Second,
		Read:       30 * time.Second,
		Write:      60 * time.Second,
		Idle:       120 * time.Second,
	}
}

// Options là cấu trúc đầu vào của `NewRouter`, do `platform.Container` dựng từ
// config — transport KHÔNG tự đọc env, để `GIN_MODE` chỉ quyết định 1 chỗ.
type Options struct {
	GinMode string
	// WebDist là thư mục static của SPA. Rỗng = KHÔNG mount static, và mọi
	// đường dẫn lạ trả 404 chứ không trả HTML — dùng cho `go test` và cho
	// trường hợp M5 chưa build web.
	WebDist string
	// Dev bật CORS `*` cho Vite dev server (port khác origin).
	Dev bool
	// MaxMultipartMemory là ngưỡng Gin đệm multipart trong RAM trước khi ghi
	// tạm ra đĩa. 10MB = trần của `/api/stt` và `/api/restore`.
	MaxMultipartMemory int64
	// TTS / STT là port của `domain/audio`. Handler chỉ biết interface, không
	// biết engine là stub hay gRPC client — vì vậy test HTTP chạy được với
	// stub mà không cần audio-service.
	TTS audiodomain.TTSSynthesizer
	STT audiodomain.STTTranscriber
	// BackupPort là `application/sync.BackupPort`. M4 KHÔNG implement (pg_dump
	// thuộc M7) — không có thì `/api/backup` + `/api/restore` trả 501 với
	// message nói rõ thay vì 500 "chưa sẵn sàng".
	//
	// HỢP ĐỒNG khi M7 thêm hiện thực (B1 của `stack-v2-typednil.md`): truyền
	// `nil` để tắt. Truyền CON TRỎ NIL (kiểu `*pgBackup`) cũng tắt — handler
	// dùng `backupDisabled` nên soi tới con trỏ bên trong, không chỉ
	// `port == nil`. Lý do: `port == nil` trên interface chứa con trỏ nil cho
	// FALSE, và hệ quả là panic 500 thay vì 501.
	Backup syncapp.BackupPort
	// GraphQL là handler của `internal/transport/graphql` (gqlgen), nil = không
	// mount `/query`.
	GraphQL http.Handler
	// Playground là handler GraphiQL, nil hoặc trả 404 ở production.
	Playground http.Handler
	// Services dùng cho `/api/health` — xem `healthHandler`.
	DB Pinger
	// Audio là nguồn cung cấp trạng thái engine ĐỌNG TỨC THÌ (không phải bản
	// chụp lúc boot). nil = không quan tâm audio.
	//
	// Vì sao phải là func thay vì `*EngineInfo`: trạng thái engine đổi sau
	// request audio đầu tiên (service báo tên thật qua `SynthesizeResponse.Real`).
	// Truyền con trỏ tới 1 bản sao lúc boot là cách chắc chắn `/api/health` báo
	// `degraded` mãi và giết HEALTHCHECK của Docker (F3).
	Audio func() (name string, real bool)
	Log   *slog.Logger
}

// Router là kết quả `NewRouter`: engine Gin đã cấu hình + handler GraphQL.
type Router struct {
	Engine *gin.Engine
	Log    *slog.Logger
}

// NewRouter dựng engine Gin với đúng cấu hình an toàn (STACK-V2-PLAN §1):
//
//   - `gin.New()` + `gin.Recovery()` + `gin.LoggerWithConfig` — KHÔNG dùng
//     `gin.Default()` vì nó bật sẵn 2 middleware mà ta đã quyết định khác.
//   - `SetTrustedProxies(nil)` — mặc định của Gin là `[]string{"0.0.0.0/0",
//     "::/0"}`, tức TIN mọi `X-Forwarded-For` là của client. Với app chạy sau
//     reverse proxy thì cần khai đúng, không khai thì tin cả mạng ngoài.
//   - CORS đặt TRƯỚC `gin.Recovery()` — lý do ở `corsMiddleware`.
func NewRouter(opts Options) *Router {
	if opts.GinMode != "" {
		gin.SetMode(opts.GinMode)
	}
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	if opts.MaxMultipartMemory <= 0 {
		opts.MaxMultipartMemory = defaultMaxMultipartMemory
	}

	e := gin.New()
	if err := e.SetTrustedProxies(nil); err != nil {
		// Chỉ fail khi truyền sai định dạng; `nil` là hợp lệ.
		log.Warn("SetTrustedProxies thất bại", slog.String("err", err.Error()))
	}
	// CORS phải LÀ ĐẦU TIÊN: nếu Recovery đứng trước, 1 panic trong handler
	// sau sẽ ghi 500 + stack trace mà không có header CORS ⇒ trình duyệt báo
	// "CORS policy bị chặn" thay vì "server lỗi", và dev không thấy lỗi thật.
	e.Use(corsMiddleware(opts.Dev))
	e.Use(gin.Recovery())
	e.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		// Ghi qua slog để log của tầng HTTP nằm cùng format JSON với lỗi của
		// application/infrastructure — `gin.DefaultWriter` in text thô ra
		// stdout, xen vào JSON làm log không parse được.
		Output: slogWriter{log: log},
		// Bỏ màu ANSI: log đi qua slog JSON, escape sequence sẽ thành ký tự
		// `\u001b[97m` trong JSON — vô nghĩa và làm log khó đọc.
		Formatter: compactLogFormatter,
		// Skip request tĩnh: SPA tải hàng chục asset, mỗi cái 1 dòng log là
		// nhiễu che mất request thật. `/favicon.ico` thì trình duyệt tự gọi 1
		// lần/không gọi, không phải hành động của user.
		SkipPaths: []string{"/favicon.ico"},
	}))
	e.MaxMultipartMemory = opts.MaxMultipartMemory

	registerAPI(e, opts)
	if opts.GraphQL != nil {
		// KHÔNG đặt ở `/api/graphql`: sẽ đụng prefix wildcard của REST (STACK-V2
		// §3). `/query` + `/playground` là 2 path riêng, không tranh nhau với
		// `/api/*`.
		e.POST("/query", gin.WrapF(opts.GraphQL.ServeHTTP))
		e.GET("/query", gin.WrapF(opts.GraphQL.ServeHTTP))
	}
	if opts.Playground != nil {
		e.GET("/playground", gin.WrapH(opts.Playground))
	}
	if opts.WebDist == "" {
		// Không có `WEB_DIST` ⇒ không có SPA để phục vụ. Vẫn phải trả 404 JSON
		// CÓ NỘI DUNG thay vì `404 page not found` (text thô của Gin) — client
		// parse JSON sẽ lỗi, và "chưa build web" trông giống "app hỏng".
		e.NoRoute(func(c *gin.Context) {
			writeJSONError(c, http.StatusNotFound, "không tìm thấy "+c.Request.URL.Path)
		})
	} else {
		registerStatic(e, opts.WebDist, log)
	}
	return &Router{Engine: e, Log: log}
}

// Server dựng `http.Server` với timeout từ `t`.
//
// `BaseContext` gắn context gốc của process: khi `Shutdown` được gọi, mọi
// request đang chạy thấy `ctx.Done()` và các lệnh DB/audio bị huỷ — nếu không,
// 1 request treo sẽ giữ `Shutdown` chờ vô hạn.
func (r *Router) Server(t ServerTimeouts, baseCtx context.Context) *http.Server {
	return &http.Server{
		Addr:              addr(),
		Handler:           r.Engine,
		ReadHeaderTimeout: t.ReadHeader,
		ReadTimeout:       t.Read,
		WriteTimeout:      t.Write,
		IdleTimeout:       t.Idle,
		BaseContext:       func(net.Listener) context.Context { return baseCtx },
	}
}
