package audioinfra

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	audiodomain "langapp/internal/domain/audio"
)

// Engine là cặp port TTS/STT mà tầng trên (handler REST + use case `practice`)
// bind, cùng conn để đóng 1 lần.
//
// Hợp đồng: khi `AUDIO_GRPC_ADDR` rỗng, `NewFromEnv` trả STUB cho cả 2 chiều.
// Nhờ vậy `go test ./...` và `docker compose up` với Postgres-only đều chạy được
// mà không cần bật audio-service (image Whisper 8GB). Header `X-Engine` luôn nói
// "stub" + `Real=false` nên UI không hiển thị nhầm kết quả giả là thật.
//
// KHÔNG lưu `EngineInfo`/`Real` làm TRƯỜNG. Nguồn sự thật là `engineCache` bên
// trong adapter, và nó đổi sau request audio đầu tiên (service báo tên engine
// thật). Bản sao lúc boot là nguồn của F3: `/api/health` đọc nó nên luôn báo
// `degraded`, khiến HEALTHCHECK của Docker đỏ vĩnh viễn. Dùng `TTSEngine()` /
// `STTEngine()` / `IsReal()` — chúng đọc thẳng adapter.
type Engine struct {
	TTSSynth audiodomain.TTSSynthesizer
	STTSTT   audiodomain.STTTranscriber
	// Address là `AUDIO_GRPC_ADDR` đã dùng, rỗng = đang chạy stub.
	Address string

	conn *grpc.ClientConn
}

// LangSynthesizer trả adapter có `SynthesizeLang` để chọn voice zh/en. Trả nil
// khi engine chỉ có `TTSSynthesizer` cơ bản (stub) — handler kiểm bằng type
// assertion, đúng kiểu `domain/audio.LangSynthesizer` mô tả.
func (e *Engine) LangSynthesizer() audiodomain.LangSynthesizer {
	if ls, ok := e.TTSSynth.(audiodomain.LangSynthesizer); ok {
		return ls
	}
	return nil
}

// DetailedTranscriber trả adapter có `TranscribeDetailed` (word timestamp).
func (e *Engine) DetailedTranscriber() audiodomain.DetailedTranscriber {
	if dt, ok := e.STTSTT.(audiodomain.DetailedTranscriber); ok {
		return dt
	}
	return nil
}

// Close closes the underlying gRPC client connection.
func (e *Engine) Close() error {
	if e == nil || e.conn == nil {
		return nil
	}
	return e.conn.Close()
}

// TTSEngine returns TTS engine info directly from the adapter.
func (e *Engine) TTSEngine() audiodomain.EngineInfo {
	if e == nil {
		return audiodomain.NewEngineInfo("chưa cấu hình", audiodomain.KindTTS, false)
	}
	return e.TTSSynth.Info()
}

// STTEngine returns STT engine info directly from the adapter.
func (e *Engine) STTEngine() audiodomain.EngineInfo {
	if e == nil {
		return audiodomain.NewEngineInfo("chưa cấu hình", audiodomain.KindSTT, false)
	}
	return e.STTSTT.Info()
}

// EngineInfo returns the TTS engine info.
func (e *Engine) EngineInfo() audiodomain.EngineInfo { return e.TTSEngine() }

// IsReal reports whether both TTS and STT are real engines.
func (e *Engine) IsReal() bool {
	return e.TTSEngine().Real && e.STTEngine().Real
}

// NewFromEnv constructs an Engine using the given gRPC address or returns stubs if empty.
func NewFromEnv(addr string) (*Engine, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return &Engine{
			TTSSynth: StubSynthesizer{},
			STTSTT:   StubTranscriber{},
			Address:  "",
		}, nil
	}
	conn, err := Dial(addr)
	if err != nil {
		return nil, err
	}
	synth := NewSynthesizer(conn, WithTTSTimeout(envDuration("TTS_TIMEOUT", 30*time.Second)))
	stt := NewTranscriber(conn, WithSTTTimeout(envDuration("STT_TIMEOUT", 60*time.Second)))
	return &Engine{
		TTSSynth: synth,
		STTSTT:   stt,
		Address:  addr,
		conn:     conn,
	}, nil
}

// NewFromEnvOS constructs an Engine using AUDIO_GRPC_ADDR from the environment.
func NewFromEnvOS() (*Engine, error) {
	engine, err := NewFromEnv(os.Getenv("AUDIO_GRPC_ADDR"))
	if err != nil {
		return nil, fmt.Errorf("construct audio client: %w", err)
	}
	return engine, nil
}

func envDuration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	// Chấp nhận số giây trần — cấu hình compose hay viết `TTS_TIMEOUT=30`.
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	return def
}
