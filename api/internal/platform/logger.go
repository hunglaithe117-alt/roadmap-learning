package platform

import (
	"log/slog"
	"os"
	"strings"
)

// NewLogger trả slog handler JSON ở mức level từ env. Dùng slog built-in
// (STACK-V2-PLAN §1 — cố ý KHÔNG thêm zap, quy mô ~30 endpoint không cần).
func NewLogger(level string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: ParseLevel(level),
	}))
}

// ParseLevel map string → slog.Level. Biến rỗng hoặc không nhận ra → info
// (mặc định của slog). Không trả error: logger không được làm app chết lúc
// boot vì 1 biến env gõ sai.
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
