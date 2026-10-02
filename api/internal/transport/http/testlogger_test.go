package httptransport

import (
	"io"
	"log/slog"
)

// discardLogger là logger nuốt output cho test.
//
// `slog.New(slog.NewTextHandler(io.Discard, nil))` thay vì dùng `t.Log`:
// middleware log ở tầng transport không thấy `*testing.T`, nên không có cách nào
// ghi vào log test mà không đổi chữ ký hàm ở khắp 6 file production.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}
