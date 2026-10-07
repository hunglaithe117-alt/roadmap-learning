// Package platform provides runtime configuration, database setup, and DI wiring.
package platform

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config represents runtime configuration loaded from environment variables.
type Config struct {
	Port            string
	DSN             string
	WhisperURL      string
	PiperBin        string
	PiperModelZH    string
	PiperModelEN    string
	GinMode         string
	LogLevel        string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// DefaultDSN specifies default PostgreSQL connection string when unset in env.
const DefaultDSN = "postgres://langapp:langapp@localhost:5433/langapp?sslmode=disable"

// LoadConfig reads configuration from environment variables with defaults.
func LoadConfig() (Config, error) {
	cfg := Config{
		Port:            envString("PORT", "8080"),
		DSN:             envString("LANGAPP_POSTGRES_DSN", DefaultDSN),
		WhisperURL:      strings.TrimSpace(os.Getenv("WHISPER_URL")),
		PiperBin:        os.Getenv("PIPER_BIN"),
		PiperModelZH:    os.Getenv("PIPER_MODEL_ZH"),
		PiperModelEN:    os.Getenv("PIPER_MODEL_EN"),
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
