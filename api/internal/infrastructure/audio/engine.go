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

// Close đóng conn gRPC. Nil-safe để `defer` không cần if.
func (e *Engine) Close() error {
	if e == nil || e.conn == nil {
		return nil
	}
	return e.conn.Close()
}

// TTSEngine trả `EngineInfo` của chiều TTS, đọc TRỰC TIẾP từ `engineCache` của
// adapter — KHÔNG phải bản sao `Engine.EngineInfo`.
//
// Vì sao: `engineCache` mới là nguồn sự thật sau request audio đầu tiên (service
// báo tên engine thật qua `SynthesizeResponse.Real`). Bản sao `Engine.EngineInfo`
// là giá trị TĨNH lúc boot, nên nó không bao giờ phản ánh engine thật — và
// `/api/health` đọc chính bản sao đó nên luôn báo `degraded`, khiến
// HEALTHCHECK của Docker đỏ vĩnh viễn.
func (e *Engine) TTSEngine() audiodomain.EngineInfo {
	if e == nil {
		return audiodomain.NewEngineInfo("chưa cấu hình", audiodomain.KindTTS, false)
	}
	return e.TTSSynth.Info()
}

// STTEngine là `TTSEngine` cho chiều STT.
func (e *Engine) STTEngine() audiodomain.EngineInfo {
	if e == nil {
		return audiodomain.NewEngineInfo("chưa cấu hình", audiodomain.KindSTT, false)
	}
	return e.STTSTT.Info()
}

// EngineInfo là viết tắt của `TTSEngine()`, dành cho log lúc boot và cho code
// đã quen với tên cũ. Vẫn đọc adapter ⇒ không quay lại bản sao tĩnh.
func (e *Engine) EngineInfo() audiodomain.EngineInfo { return e.TTSEngine() }

// IsReal báo audio có phải engine THẬT không — `/api/health` dùng để phân biệt
// `ok` với `degraded`.
//
// Với stub, `Real` là false VĪA KHI DỰNG (không cần chờ request nào), nên
// `degraded` là KẾT QUẢ ĐÚNG NGAY từ lần health đầu tiên — đây là thứ F3 đòi hỏi:
// trước đó `Engine.Real` là hằng `false` ở cả hai nhánh và không ai gán lại, nên
// `degraded` là ngẫu nhiên chứ không phản ánh gì.
func (e *Engine) IsReal() bool {
	return e.TTSEngine().Real && e.STTEngine().Real
}

// NewFromEnv đọc `AUDIO_GRPC_ADDR` rồi dựng engine tương ứng.
//
// Không `Ping` lúc boot: `grpc.NewClient` là lazy, service có thể còn đang
// khởi động cùng compose. Healthcheck của audio-service (`grpc_health_probe`)
// là chỗ đúng để đòi nó sẵn sàng; app chỉ cần biết có dùng service hay không.
func NewFromEnv(addr string) (*Engine, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return &Engine{
			TTSSynth: StubSynthesizer{},
			STTSTT:   StubTranscriber{},
			// Không còn `EngineInfo`/`Real` làm trạng thái: `TTSEngine()`/`IsReal()`
			// đọc thẳng adapter, nên không có bản sao nào có thể lệch. Giữ 2 field
			// cũ là chính là nguồn của F3.
			Address: "",
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

// NewFromEnvOS là biến thể đọc `AUDIO_GRPC_ADDR` từ môi trường — dùng ở
// `platform.Build` để không phải biết tên biến ở tầng DI.
func NewFromEnvOS() (*Engine, error) {
	engine, err := NewFromEnv(os.Getenv("AUDIO_GRPC_ADDR"))
	if err != nil {
		return nil, fmt.Errorf("dựng audio client: %w", err)
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
