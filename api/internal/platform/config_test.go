package platform

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_load_config_defaults_without_env(t *testing.T) {
	for _, k := range []string{"PORT", "LANGAPP_DB", "LANGAPP_POSTGRES_DSN", "PIPER_BIN", "PIPER_MODEL_ZH", "PIPER_MODEL_EN", "GIN_MODE", "LOG_LEVEL", "DB_MAX_OPEN_CONNS"} {
		t.Setenv(k, "")
	}

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, DefaultDSN, cfg.DSN)
	assert.Equal(t, "release", cfg.GinMode)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, 10, cfg.MaxOpenConns)
	require.NoError(t, cfg.Validate())
}

func Test_load_config_reads_env(t *testing.T) {
	t.Setenv("PORT", "9000")
	t.Setenv("LANGAPP_POSTGRES_DSN", "postgres://u:p@db:5432/x?sslmode=disable")
	t.Setenv("WHISPER_URL", "http://stt:9000/asr")
	t.Setenv("PIPER_BIN", "/usr/local/bin/piper")
	t.Setenv("PIPER_MODEL_ZH", "/models/zh.onnx")
	t.Setenv("PIPER_MODEL_EN", "/models/en.onnx")
	t.Setenv("GIN_MODE", "debug")
	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("DB_MAX_OPEN_CONNS", "42")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "9000", cfg.Port)
	assert.Equal(t, "postgres://u:p@db:5432/x?sslmode=disable", cfg.DSN)
	assert.Equal(t, "http://stt:9000/asr", cfg.WhisperURL)
	assert.Equal(t, "/usr/local/bin/piper", cfg.PiperBin)
	assert.Equal(t, "/models/zh.onnx", cfg.PiperModelZh)
	assert.Equal(t, "/models/en.onnx", cfg.PiperModelEn)
	assert.Equal(t, "debug", cfg.GinMode)
	assert.Equal(t, "warn", cfg.LogLevel)
	assert.Equal(t, 42, cfg.MaxOpenConns)
}

func Test_config_validate_rejects_non_postgres_dsn(t *testing.T) {
	cfg := Config{DSN: "/data/langapp.db", MaxOpenConns: 5}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "postgres://")
}

func Test_load_config_falls_back_when_number_unparsable(t *testing.T) {
	t.Setenv("DB_MAX_OPEN_CONNS", "not-a-number")
	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, 10, cfg.MaxOpenConns, "env hỏng thì về default, không làm app chết lúc boot")
}

func Test_parse_level(t *testing.T) {
	for _, c := range []struct {
		in   string
		want string
	}{
		{"debug", "DEBUG"}, {"INFO", "INFO"}, {" warn ", "WARN"},
		{"warning", "WARN"}, {"error", "ERROR"}, {"", "INFO"}, {"nonsense", "INFO"},
	} {
		assert.Equal(t, c.want, ParseLevel(c.in).String(), "level %q", c.in)
	}
}

func Test_container_close_on_nil_is_noop(t *testing.T) {
	var c *Container
	assert.NoError(t, c.Close(), "Close trên nil container phải no-op, không panic")
}

// F1: compose set LANGAPP_POSTGRES_DSN, nếu LoadConfig không đọc biến đó thì
// M4 thay binary là boot-crash ở Validate() vì DSN vẫn là path file SQLite.
//
// M7c: `LANGAPP_DB` kiểu v1 vẫn được set trong test để chứng minh nó BỊ BỎ QUA
// hoàn toàn — app v1 đã bị gỡ nên đường dẫn file không còn ý nghĩa, và nếu
// còn fallback thì `LANGAPP_DB=/data/langapp.db` trong môi trường cũ sẽ làm app
// boot với DSN sai rồi chết ở `Validate()` với lỗi không ai đoán ra nguyên nhân.
func Test_load_config_ignores_legacy_langapp_db(t *testing.T) {
	t.Setenv("LANGAPP_DB", "/data/langapp.db")
	t.Setenv("LANGAPP_POSTGRES_DSN", "postgres://langapp:langapp@postgres:5432/langapp?sslmode=disable")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "postgres://langapp:langapp@postgres:5432/langapp?sslmode=disable", cfg.DSN)
	require.NoError(t, cfg.Validate())
}

// Không set gì thì rơi về `DefaultDSN` (dev local), KHÔNG rơi về `LANGAPP_DB`.
func Test_load_config_falls_back_to_default_dsn(t *testing.T) {
	t.Setenv("LANGAPP_DB", "postgres://u:p@db:5432/x?sslmode=disable")
	t.Setenv("LANGAPP_POSTGRES_DSN", "")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, DefaultDSN, cfg.DSN)
	assert.NotEqual(t, "postgres://u:p@db:5432/x?sslmode=disable", cfg.DSN,
		"`LANGAPP_DB` kiểu v1 phải bị bỏ qua hoàn toàn sau M7c")
}

// `Validate` phải nói rõ biến cần set, kèm gợi ý khi thấy giá trị kiểu file
// path — đây là lỗi dễ gặp nhất sau khi gỡ v1 (môi trường cũ còn `LANGAPP_DB`).
func Test_validate_rejects_a_sqlite_path_with_an_actionable_message(t *testing.T) {
	c := Config{DSN: "/data/langapp.db", MaxOpenConns: 10, MaxIdleConns: 10, ConnMaxLifetime: time.Hour}

	err := c.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LANGAPP_POSTGRES_DSN",
		"lỗi phải nêu đúng biến cần set")
	assert.Contains(t, err.Error(), "LANGAPP_DB",
		"phải nhắc biến cũ để người đọc biết mình đang dùng nhầm cái gì")
}
