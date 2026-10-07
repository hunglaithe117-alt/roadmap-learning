package platform

import (
	"log/slog"
	"os"
	"strings"
)

// NewLogger creates a JSON slog.Logger writing to stdout at the specified level.
func NewLogger(level string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: ParseLevel(level),
	}))
}

// ParseLevel parses a log level string, defaulting to slog.LevelInfo if empty or invalid.
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
