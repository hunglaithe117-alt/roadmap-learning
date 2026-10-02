package httptransport

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// defaultMaxMultipartMemory là ngưỡng đệm multipart trong RAM. 10MB khớp trần
// của `/api/stt` (`maxSTTBytes` ở api/audio.go v1): lớn hơn thì tốn RAM cho
// request vô nghĩa, nhỏ hơn thì file hợp lệ bị ghi tạm ra đĩa.
const defaultMaxMultipartMemory int64 = 10 << 20

// shutdownGrace là khoảng chờ cho request đang chạy khi nhận SIGTERM. Lớn hơn
// `WriteTimeout` (60s) để 1 request TTS dài có cơ hội xong; nhỏ hơn
// `docker compose stop` mặc định để container không bị SIGKILL giữa chừng.
const shutdownGrace = 15 * time.Second

// addr đọc `PORT` — hằng số chứ không để trong Options vì nó là thứ duy nhất
// thay đổi giữa các lần chạy và compose đã set sẵn.
func addr() string {
	if p := strings.TrimSpace(os.Getenv("PORT")); p != "" {
		return ":" + p
	}
	return ":8080"
}

// corsMiddleware trả header CORS và trả lời preflight.
//
// Vì sao phải tự viết: Gin's `cors` là package `github.com/gin-contrib/cors` —
// 1 dependency ngoài cho đúng 20 dòng logic. Và `cors.Wrap` của nó tự `Abort`
// preflight, còn ở đây ta muốn giữ hành vi đó (browser không gửi body preflight).
//
// `origin` ở production là `*`? KHÔNG — xem `corsConfig`.
// QUY TẮC ĐẶT TRƯỚC `gin.Recovery()`: nếu Recovery chạy trước CORS thì 1 panic
// sẽ trả 500 mà không có `Access-Control-Allow-Origin` ⇒ trình duyệt báo "CORS
// bị chặn" thay vì "server lỗi", và dev mất dấu vết lỗi thật.
func corsMiddleware(dev bool) gin.HandlerFunc {
	cfg := corsConfig(dev)
	return func(c *gin.Context) {
		req := c.Request
		origin := req.Header.Get("Origin")
		if origin == "" {
			c.Next()
			return
		}
		// Echo đúng origin thay vì ghi `*`: cần `Vary: Origin` để cache HTTP của
		// proxy không trả header của máy A cho trình duyệt của máy B.
		if !originAllowed(origin, cfg) {
			// Origin không nằm trong allow-list ⇒ KHÔNG set header CORS. Browser
			// tự chặn response, đúng như mong đợi; im lặng hơn là set `*`.
			c.Next()
			return
		}
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Vary", "Origin")
		if req.Method == http.MethodOptions {
			h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, Accept, Authorization, X-Requested-With, X-Apollo-Operation-Name")
			h.Set("Access-Control-Max-Age", "600")
			// 204 chứ không phải 200: preflight không có body.
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// corsConfig là allow-list origin. `allowAll` chỉ bật ở dev (Vite chạy port
// khác).
func corsConfig(dev bool) []string {
	if !dev {
		// Production: app phục vụ cả SPA lẫn API từ CÙNG origin ⇒ không cần
		// CORS cho request same-origin. Danh sách rỗng nghĩa là không origin
		// nào khác được phép — đó là mặc định đúng, không phải thiếu sót.
		return nil
	}
	return []string{
		"http://localhost:5173", // Vite dev server
		"http://127.0.0.1:5173",
	}
}

func originAllowed(origin string, allow []string) bool {
	if len(allow) == 0 {
		// Không có allow-list ⇒ chỉ chấp nhận chính app khi nó cung cấp
		// `X-Allowed-Origin` (không dùng) — coi như chặn.
		return false
	}
	for _, a := range allow {
		if a == origin {
			return true
		}
	}
	return false
}

// slogWriter là `io.Writer` nạp vào `gin.LoggerConfig.Output`, chuyển mỗi dòng
// Gin ghi thành 1 record slog.
//
// Hạn chế được ghi rõ: `gin.LogFormatter` ở v1.12 chỉ trả về **chuỗi**, không
// có đường trả field có cấu trúc. Nên record slog ở đây có `line` là chuỗi
// đã định dạng, không phải field `status`/`method` riêng. Đổi được thì phải thay
// bằng middleware tự viết — chưa cần ở quy mô ~30 endpoint nên chưa làm.
type slogWriter struct{ log *slog.Logger }

func (w slogWriter) Write(p []byte) (int, error) {
	w.log.Info("http", slog.String("line", strings.TrimRight(string(p), "\n")))
	return len(p), nil
}

// compactLogFormatter là `gin.LogFormatter` không màu, gọn 1 dòng.
//
// Ưu tiên trường quan trọng nhất trước khi đường dẫn dài: `GET /api/tts 200`
// đọc nhanh hơn `GET /api/tts?text=… 200` khi grep log.
func compactLogFormatter(p gin.LogFormatterParams) string {
	return fmt.Sprintf("%s %s %d %s",
		p.Method, p.Path, p.StatusCode, p.Latency.Round(time.Millisecond))
}

// registerStatic phục vụ SPA với fallback `index.html`.
//
// Vì sao cần fallback: Vue dùng `createWebHashHistory` nên client-side route
// nằm sau `#` và server KHÔNG thấy — nhưng nếu ai đó đổi sang
// `createWebHistory` hoặc user gõ trực tiếp `/roadmap/zh`, server phải trả
// `index.html` chứ không 404.
//
// `StaticFS` dùng `http.FileSystem` (go:embed) sẽ KHÔNG có fallback, nên ở đây
// dùng `http.Dir` + handler tự viết. `gin.Static` cũng vậy.
func registerStatic(e *gin.Engine, webDist string, log *slog.Logger) {
	indexFile := filepath.Join(webDist, "index.html")
	if _, err := os.Stat(indexFile); err != nil {
		// Không có index.html ⇒ chưa build web. Mount vẫn được (để `/assets/*`
		// có cơ hội tồn tại) nhưng log cảnh báo để không debug mãi không ra.
		log.Warn("WEB_DIST không có index.html, static SPA sẽ trả 404",
			slog.String("web_dist", webDist))
	}
	e.NoRoute(staticSPA(webDist, log))
}

// staticSPA trả file nếu tồn tại, không thì `index.html`, không thì 404 JSON.
func staticSPA(webDist string, log *slog.Logger) gin.HandlerFunc {
	fileServer := http.FileServer(http.Dir(webDist))
	index := filepath.Join(webDist, "index.html")
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.AbortWithStatus(http.StatusMethodNotAllowed)
			return
		}
		clean := path.Clean(c.Request.URL.Path)
		// `path.Clean("/../etc/passwd")` = "/etc/passwd" — không thoát khỏi
		// `webDist` vì `http.Dir` tự chặn traversal. Ở đây chỉ chặn đường dẫn
		// rỗng để `index.html` không bị phục vụ 2 lần.
		if candidate := filepath.Join(webDist, filepath.FromSlash(clean)); clean != "/" {
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				fileServer.ServeHTTP(c.Writer, c.Request)
				c.Abort()
				return
			}
		}
		if st, err := os.Stat(index); err == nil && !st.IsDir() {
			c.File(index)
			c.Abort()
			return
		}
		log.Warn("static: không tìm thấy file và cũng không có index.html",
			slog.String("path", c.Request.URL.Path))
		writeJSONError(c, http.StatusNotFound, "không tìm thấy "+c.Request.URL.Path)
		c.Abort()
	}
}

// parseIntParam đọc query param số với default. Dùng cho `?limit=` của
// `/api/tts` — client JS hay gửi chuỗi rỗng, và `strconv.Atoi("")` lỗi.
func parseIntParam(raw string, def int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return def
	}
	return n
}
