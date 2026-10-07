// Package audioinfra implements audio domain TTS/STT ports via gRPC to audio-service.
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

// Dial connects to audio-service, returning (nil, nil) when addr is empty.
func Dial(addr string) (*grpc.ClientConn, error) {
	if addr == "" {
		return nil, nil
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial audio-service %q: %w", addr, err)
	}
	return conn, nil
}

// engineCache caches engine metadata reported by the latest service response.
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

type config struct {
	ttsTimeout time.Duration
	sttTimeout time.Duration
}

// Option configures NewSynthesizer or NewTranscriber.
type Option func(*config)

// WithTTSTimeout sets the timeout deadline for TTS RPC calls.
func WithTTSTimeout(d time.Duration) Option {
	return func(c *config) { c.ttsTimeout = d }
}

// WithSTTTimeout sets the timeout deadline for STT RPC calls.
func WithSTTTimeout(d time.Duration) Option {
	return func(c *config) { c.sttTimeout = d }
}

// Synthesizer implements audiodomain.TTSSynthesizer and audiodomain.LangSynthesizer.
type Synthesizer struct {
	audio audiopb.AudioClient
	cfg   config
	eng   *engineCache
}

// NewSynthesizer constructs a TTS adapter over an open gRPC connection.
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

// Info returns known TTS engine metadata from the latest request.
func (s *Synthesizer) Info() audiodomain.EngineInfo { return s.eng.info() }

// Synthesize synthesizes speech from text without specifying language.
func (s *Synthesizer) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	return s.SynthesizeLang(ctx, text, "")
}

// SynthesizeLang synthesizes speech for the given text and language.
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

// Transcriber implements audiodomain.STTTranscriber and DetailedTranscriber.
type Transcriber struct {
	audio audiopb.AudioClient
	cfg   config
	eng   *engineCache
}

// NewTranscriber constructs an STT adapter over an open gRPC connection.
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

// Info returns known STT engine metadata from the latest request.
func (t *Transcriber) Info() audiodomain.EngineInfo { return t.eng.info() }

// Transcribe transcribes audio without word-level timestamps.
func (t *Transcriber) Transcribe(ctx context.Context, audio []byte, filename, contentType string) (audiodomain.Transcript, error) {
	return t.TranscribeDetailed(ctx, audio, filename, contentType, "")
}

// TranscribeDetailed transcribes audio and converts protobuf messages to domain objects.
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
