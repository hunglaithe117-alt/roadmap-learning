// Package platform là tầng hạ tầng: config từ env, logger, connection pool
// Postgres, migration boot và DI wiring. Đây là nơi DUY NHẤT ngoài
// infrastructure được phép import driver DB.
//
// Quyết định stack: đọc config 1 lúc boot bằng os.Getenv, KHÔNG dùng viper
// (xem STACK-V2-PLAN §1 — cố ý loại viper).
package platform

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config là toàn bộ tham số runtime, đọc 1 lần lúc boot. Mọi trường đều có
// default để `go test ./...` và `go run` không cần env nào (domain test KHÔNG
// cần DB — xem STACK-V2-PLAN §6 M1 exit criteria).
type Config struct {
	// Port là cổng HTTP của server (M4 dùng; M1 chưa có http.Server).
	Port string
	// DSN là Postgres connection string, ví dụ
	// `postgres://langapp:langapp@postgres:5432/langapp?sslmode=disable`
	// (cổng TRONG MẠNG compose luôn 5432). Khi chạy ngoài compose thì host
	// publish ra là 5433 — xem `DefaultDSN` bên dưới.
	// Đọc từ `LANGAPP_POSTGRES_DSN`.
	//
	// M7c: KHÔNG còn fallback sang `LANGAPP_DB`. Trước đó cần fallback vì app v1
	// đọc `LANGAPP_DB` như ĐƯỜNG DẪN file SQLite, nên compose buộc phải dùng
	// tên biến riêng cho Postgres. V1 đã bị gỡ ⇒ giữ fallback chỉ còn gây hại:
	// ai đó còn `LANGAPP_DB=/data/langapp.db` trong môi trường cũ sẽ nhận lỗi
	// "phải là DSN postgres://" ở chỗ không liên quan tới cấu hình họ đang sửa.
	DSN string
	// WhisperURL rỗng = STT stub (không cần sidecar, không pull image ~8GB).
	WhisperURL string
	// PiperBin là binary TTS; rỗng = lấy từ PATH.
	PiperBin string
	// PiperModelZh / PiperModelEn là file voice theo ngôn ngữ.
	PiperModelZh string
	PiperModelEn string
	// GinMode: "debug" | "release" | "test".
	GinMode string
	// LogLevel: "debug" | "info" | "warn" | "error".
	LogLevel string

	// Pool sizing. Single-user + ~30 endpoint nên pool nhỏ; MaxIdle = MaxOpen
	// để không trả conn về pool rồi phải mở lại (Postgres có TCP + auth
	// round-trip, không rẻ như SQLite file).
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// DefaultDSN là DSN khi env không set — khớp service `postgres` trong
// docker-compose.yml (POSTGRES_USER/DB/PASSWORD = langapp).
// DefaultDSN trỏ `localhost:5433` — đúng cổng HOST mà docker-compose publish
// (mặc định `POSTGRES_PORT`). Đổi `POSTGRES_PORT` thì phải sửa cả hằng này,
// hoặc set `LANGAPP_POSTGRES_DSN` để không phụ thuộc mặc định.
const DefaultDSN = "postgres://langapp:langapp@localhost:5433/langapp?sslmode=disable"

// LoadConfig đọc env, áp default cho biến thiếu. KHÔNG trả error về biến rỗng
// (trừ số pool không parse được) vì app vẫn phải boot được với config tối
// thiểu cho `go test`/dev.
func LoadConfig() (Config, error) {
	cfg := Config{
		Port:            envString("PORT", "8080"),
		DSN:             envString("LANGAPP_POSTGRES_DSN", DefaultDSN),
		WhisperURL:      strings.TrimSpace(os.Getenv("WHISPER_URL")),
		PiperBin:        os.Getenv("PIPER_BIN"),
		PiperModelZh:    os.Getenv("PIPER_MODEL_ZH"),
		PiperModelEn:    os.Getenv("PIPER_MODEL_EN"),
		GinMode:         envString("GIN_MODE", "release"),
		LogLevel:        envString("LOG_LEVEL", "info"),
		MaxOpenConns:    envInt("DB_MAX_OPEN_CONNS", 10),
		MaxIdleConns:    envInt("DB_MAX_IDLE_CONNS", 10),
		ConnMaxLifetime: envDuration("DB_CONN_MAX_LIFETIME", time.Hour),
	}
	return cfg, nil
}

// Validate kiểm tra những điều kiện DB mà không có default cứu được. Gọi sau
// LoadConfig; `go test` không gọi nên env thiếu không chặn unit test domain.
func (c Config) Validate() error {
	if strings.TrimSpace(c.DSN) == "" {
		return fmt.Errorf("platform: LANGAPP_POSTGRES_DSN rỗng — cần DSN Postgres, ví dụ %s", DefaultDSN)
	}
	if !strings.HasPrefix(c.DSN, "postgres://") && !strings.HasPrefix(c.DSN, "postgresql://") {
		return fmt.Errorf("platform: LANGAPP_POSTGRES_DSN phải là DSN postgres:// hoặc postgresql:// "+
			"(nhận được %q — biến `LANGAPP_DB` kiểu app v1 đã bị gỡ ở M7c, đừng set nó nữa)",
			c.DSN)
	}
	if c.MaxOpenConns <= 0 {
		return fmt.Errorf("platform: DB_MAX_OPEN_CONNS phải > 0, got %d", c.MaxOpenConns)
	}
	return nil
}

func envString(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envDuration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
