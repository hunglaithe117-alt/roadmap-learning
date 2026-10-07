package audioinfra

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	audiodomain "langapp/internal/domain/audio"
)

// StubSynthesizer generates sine wave WAV audio offline when audio-service is disabled.
type StubSynthesizer struct{}

// Info returns stub TTS engine metadata.
func (s StubSynthesizer) Info() audiodomain.EngineInfo {
	return audiodomain.NewEngineInfo("stub", audiodomain.KindTTS, false)
}

// Synthesize synthesizes sine wave WAV audio without language selection.
func (s StubSynthesizer) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	return s.SynthesizeLang(ctx, text, "")
}

// SynthesizeLang synthesizes sine wave WAV audio, ignoring language in the stub.
func (s StubSynthesizer) SynthesizeLang(ctx context.Context, text, lang string) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(text) == "" {
		return nil, "", fmt.Errorf("empty text")
	}
	return SineWAV(16000, 0.5, 440), "audio/wav", nil
}

// SineWAV generates a 16-bit mono PCM WAV audio buffer.
func SineWAV(sampleRate int, seconds, freqHz float64) []byte {
	n := int(float64(sampleRate) * seconds)
	data := make([]byte, 44+n*2)
	copy(data[0:4], "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(36+n*2))
	copy(data[8:12], "WAVE")
	copy(data[12:16], "fmt ")
	binary.LittleEndian.PutUint32(data[16:20], 16)
	binary.LittleEndian.PutUint16(data[20:22], 1)
	binary.LittleEndian.PutUint16(data[22:24], 1)
	binary.LittleEndian.PutUint32(data[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(data[28:32], uint32(sampleRate*2))
	binary.LittleEndian.PutUint16(data[32:34], 2)
	binary.LittleEndian.PutUint16(data[34:36], 16)
	copy(data[36:40], "data")
	binary.LittleEndian.PutUint32(data[40:44], uint32(n*2))
	for i := 0; i < n; i++ {
		v := int16(math.Sin(2*math.Pi*freqHz*float64(i)/float64(sampleRate)) * 16000)
		binary.LittleEndian.PutUint16(data[44+i*2:], uint16(v))
	}
	return data
}

// StubTranscriber returns fixed transcripts for testing offline.
type StubTranscriber struct {
	Text string
	Lang string
}

// Info returns stub STT engine metadata.
func (s StubTranscriber) Info() audiodomain.EngineInfo {
	return audiodomain.NewEngineInfo("stub", audiodomain.KindSTT, false)
}

// Transcribe returns a fixed transcript without word details.
func (s StubTranscriber) Transcribe(ctx context.Context, audio []byte, filename, contentType string) (audiodomain.Transcript, error) {
	return s.TranscribeDetailed(ctx, audio, filename, contentType, "")
}

func (s StubTranscriber) TranscribeDetailed(ctx context.Context, audio []byte, filename, contentType, lang string) (audiodomain.Transcript, error) {
	if err := ctx.Err(); err != nil {
		return audiodomain.Transcript{}, err
	}
	if len(audio) == 0 {
		return audiodomain.Transcript{}, fmt.Errorf("empty audio")
	}
	text := s.Text
	if text == "" {
		text = "ni hao"
	}
	langOut := s.Lang
	if langOut == "" {
		langOut = "zh"
	}
	return audiodomain.Transcript{Text: text, Lang: langOut}, nil
}
