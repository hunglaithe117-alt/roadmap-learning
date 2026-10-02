// Package audioinfra hiện thực 2 port TTS/STT của `internal/domain/audio` trên
// gRPC client tới `services/audio-service`.
//
// Đây là NƠI DUY NHẤT trong app được phép biết audio nói chuyện với process nào
// (STACK-V2-PLAN §2: infrastructure là tầng duy nhất import gRPC client). Handler
// ở `internal/transport/http` chỉ gọi interface, không biết `AUDIO_GRPC_ADDR`
// có tồn tại hay không — nhờ vậy test HTTP chạy được với stub mà không cần
// service nào chạy.
package audioinfra

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	audiodomain "langapp/internal/domain/audio"
	audiopb "langapp/proto/audio/v1"
)

// Dial mở kết nối tới audio-service.
//
// `addr` rỗng trả (nil, nil) — đó là tín hiệu "không có service", và
// `NewFromEnv` chuyển sang stub. Kiểu trả về này cố ý: `NewClient` bên dưới
// KHÔNG tự quyết định dùng stub hay client thật, nên test có thể dựng client
// thật trên bufconn mà không bị "nếu addr rỗng thì stub" cướp mất.
func Dial(addr string) (*grpc.ClientConn, error) {
	if addr == "" {
		return nil, nil
	}
	// `insecure` là đúng ở đây: audio-service chạy trong cùng network compose,
	// không có TLS, và nó chỉ tổng hợp/giải mã audio chứ không giữ bí mật.
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("mở kết nối audio-service %q: %w", addr, err)
	}
	return conn, nil
}

// engineCache nhớ engine service vổ lò trong lần gọi gần nhất.
//
// `Info()` nằm trong cả `TTSSynthesizer` lẫn `STTTranscriber` nhưng handler gọi
// nó để gắn header `X-Engine` TRƯỚC khi gọi engine — tức là không được gọi
// network. Vì vậy giá trị chỉ có sau request đầu tiên, và trước đó trả tên
// dự kiến với `Real=false`: UI hiện badge "kết quả không thật" thay vì im lặng.
type engineCache struct {
	mu       sync.Mutex
	name     string
	real     bool
	known    bool
	kind     audiodomain.Kind
	fallback string
}

func newEngineCache(kind audiodomain.Kind, fallback string) *engineCache {
	return &engineCache{kind: kind, fallback: fallback}
}

func (e *engineCache) bind(name string, real bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.name, e.real, e.known = name, real, true
}

func (e *engineCache) info() audiodomain.EngineInfo {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.known {
		return audiodomain.EngineInfo{Name: e.fallback, Kind: e.kind, Real: false}
	}
	return audiodomain.EngineInfo{Name: e.name, Kind: e.kind, Real: e.real}
}

// config là tuỳ chọn dùng chung cho cả 2 wrapper.
type config struct {
	ttsTimeout time.Duration
	sttTimeout time.Duration
}

// Option tuỳ chọn cho NewSynthesizer / NewTranscriber.
type Option func(*config)

// WithTTSTimeout ép deadline cho RPC tổng hợp. V1 đã có `ttsTimeout` 30s ở tầng
// handler; đặt ở đây nghĩa là mọi call site — kể cả use case `practice` qua
// port `TTSPort` — đều có trần, không chỉ endpoint HTTP.
func WithTTSTimeout(d time.Duration) Option {
	return func(c *config) { c.ttsTimeout = d }
}

// WithSTTTimeout ép deadline cho RPC nhận dạng (60s ở v1).
func WithSTTTimeout(d time.Duration) Option {
	return func(c *config) { c.sttTimeout = d }
}

// Synthesizer hiện thực `audiodomain.TTSSynthesizer` + `audiodomain.LangSynthesizer`.
type Synthesizer struct {
	audio audiopb.AudioClient
	cfg   config
	eng   *engineCache
}

// NewSynthesizer dựng adapter TTS trên 1 kết nối gRPC đã mở.
func NewSynthesizer(conn grpc.ClientConnInterface, opts ...Option) *Synthesizer {
	cfg := config{}
	for _, o := range opts {
		o(&cfg)
	}
	return &Synthesizer{
		audio: audiopb.NewAudioClient(conn),
		cfg:   cfg,
		eng:   newEngineCache(audiodomain.KindTTS, "audio-service"),
	}
}

func (s *Synthesizer) Info() audiodomain.EngineInfo { return s.eng.info() }

// Synthesize không có tham số lang — `LangSynthesizer.SynthesizeLang` là đường
// đầy đủ; giữ method này để thoả `TTSSynthesizer` (interface của domain) mà
// không phải ép caller đổi.
func (s *Synthesizer) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	return s.SynthesizeLang(ctx, text, "")
}

func (s *Synthesizer) SynthesizeLang(ctx context.Context, text, lang string) ([]byte, string, error) {
	if s.cfg.ttsTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.cfg.ttsTimeout)
		defer cancel()
	}
	resp, err := s.audio.Synthesize(ctx, &audiopb.SynthesizeRequest{Text: text, Lang: lang})
	if err != nil {
		return nil, "", err
	}
	s.eng.bind(resp.GetEngine(), resp.GetReal())
	contentType := resp.GetContentType()
	if contentType == "" {
		contentType = "audio/wav"
	}
	return resp.GetAudio(), contentType, nil
}

// Transcriber hiện thực `audiodomain.STTTranscriber` + `DetailedTranscriber`.
type Transcriber struct {
	audio audiopb.AudioClient
	cfg   config
	eng   *engineCache
}

func NewTranscriber(conn grpc.ClientConnInterface, opts ...Option) *Transcriber {
	cfg := config{}
	for _, o := range opts {
		o(&cfg)
	}
	return &Transcriber{
		audio: audiopb.NewAudioClient(conn),
		cfg:   cfg,
		eng:   newEngineCache(audiodomain.KindSTT, "audio-service"),
	}
}

func (t *Transcriber) Info() audiodomain.EngineInfo { return t.eng.info() }

// Transcribe không trả chi tiết từ — `DetailedTranscriber.TranscribeDetailed`
// là đường đầy đủ và service luôn trả `words` nếu engine có.
func (t *Transcriber) Transcribe(ctx context.Context, audio []byte, filename, contentType string) (audiodomain.Transcript, error) {
	return t.TranscribeDetailed(ctx, audio, filename, contentType, "")
}

// TranscribeDetailed gọi `Audio.Transcribe` và chuyển protobuf → value object
// của domain. Timeout áp ở đây, không áp ở handler, để deadline đi theo
// call site bất kể ai gọi.
func (t *Transcriber) TranscribeDetailed(ctx context.Context, audio []byte, filename, contentType, lang string) (audiodomain.Transcript, error) {
	if t.cfg.sttTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.cfg.sttTimeout)
		defer cancel()
	}
	resp, err := t.audio.Transcribe(ctx, &audiopb.TranscribeRequest{
		Audio: audio, Filename: filename, ContentType: contentType, Lang: lang,
	})
	if err != nil {
		return audiodomain.Transcript{}, err
	}
	t.eng.bind(resp.GetEngine(), resp.GetReal())
	out := audiodomain.Transcript{
		Text:       resp.GetText(),
		Lang:       resp.GetLang(),
		Confidence: resp.Confidence,
	}
	for _, w := range resp.GetWords() {
		out.Words = append(out.Words, audiodomain.WordTimestamp{
			Word:       w.GetWord(),
			Start:      w.Start,
			End:        w.End,
			Confidence: w.Confidence,
		})
	}
	return out, nil
}
