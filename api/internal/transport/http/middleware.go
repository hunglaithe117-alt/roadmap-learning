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

// defaultMaxMultipartMemory is the in-memory buffer limit for multipart requests (10MB).
const defaultMaxMultipartMemory int64 = 10 << 20

// addr returns the network address to listen on, defaulting to :8080.
func addr() string {
	if p := strings.TrimSpace(os.Getenv("PORT")); p != "" {
		return ":" + p
	}
	return ":8080"
}

// corsMiddleware sets CORS headers and handles preflight requests.
func corsMiddleware(dev bool) gin.HandlerFunc {
	cfg := corsConfig(dev)
	return func(c *gin.Context) {
		req := c.Request
		origin := req.Header.Get("Origin")
		if origin == "" {
			c.Next()
			return
		}
		if !originAllowed(origin, cfg) {
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
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// corsConfig returns allowed origins for the environment.
func corsConfig(dev bool) []string {
	if !dev {
		return nil
	}
	return []string{
		"http://localhost:5173",
		"http://127.0.0.1:5173",
	}
}

func originAllowed(origin string, allow []string) bool {
	if len(allow) == 0 {
		return false
	}
	for _, a := range allow {
		if a == origin {
			return true
		}
	}
	return false
}

// slogWriter bridges Gin output to structured slog records.
type slogWriter struct{ log *slog.Logger }

func (w slogWriter) Write(p []byte) (int, error) {
	w.log.Info("http", slog.String("line", strings.TrimRight(string(p), "\n")))
	return len(p), nil
}

// compactLogFormatter formats HTTP request logs concisely.
func compactLogFormatter(p gin.LogFormatterParams) string {
	return fmt.Sprintf("%s %s %d %s",
		p.Method, p.Path, p.StatusCode, p.Latency.Round(time.Millisecond))
}

// registerStatic registers static SPA serving with an index.html fallback.
func registerStatic(e *gin.Engine, webDist string, log *slog.Logger) {
	indexFile := filepath.Join(webDist, "index.html")
	if _, err := os.Stat(indexFile); err != nil {
		log.Warn("WEB_DIST missing index.html, static SPA will 404",
			slog.String("web_dist", webDist))
	}
	e.NoRoute(staticSPA(webDist, log))
}


// staticSPA serves static assets or index.html fallback for SPA routing.
func staticSPA(webDist string, log *slog.Logger) gin.HandlerFunc {
	fileServer := http.FileServer(http.Dir(webDist))
	index := filepath.Join(webDist, "index.html")
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.AbortWithStatus(http.StatusMethodNotAllowed)
			return
		}
		clean := path.Clean(c.Request.URL.Path)
		if clean == "/api" || strings.HasPrefix(clean, "/api/") {
			writeJSONError(c, http.StatusNotFound, "không tìm thấy "+c.Request.URL.Path)
			c.Abort()
			return
		}
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
		log.Warn("static: file not found and missing index.html",
			slog.String("path", c.Request.URL.Path))
		writeJSONError(c, http.StatusNotFound, "không tìm thấy "+c.Request.URL.Path)
		c.Abort()
	}
}

// parseIntParam parses an integer query parameter with a fallback default.
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

