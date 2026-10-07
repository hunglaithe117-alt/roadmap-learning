package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	maxTextRunes = 500
	synthTimeout = 30 * time.Second
	chunkSize    = 32 << 10
)

// synthesizer abstracts speech synthesis engines.
type synthesizer interface {
	SynthesizeLang(ctx context.Context, text, lang string) ([]byte, string, error)
	Name() string
	Real() bool
}

// piperSynthesizer generates audio using the Piper neural TTS binary.
type piperSynthesizer struct {
	bin, model, modelZH, modelEN string
}

// newSynthesizerFromEnv constructs a synthesizer from environment variables or falls back to stub.
func newSynthesizerFromEnv(log *slog.Logger) (synthesizer, error) {
	shared := strings.TrimSpace(os.Getenv("PIPER_MODEL"))
	zh := strings.TrimSpace(os.Getenv("PIPER_MODEL_ZH"))
	if zh == "" {
		zh = shared
	}
	en := strings.TrimSpace(os.Getenv("PIPER_MODEL_EN"))
	if en == "" {
		en = shared
	}
	bin := os.Getenv("PIPER_BIN")
	if bin == "" {
		bin = "piper"
	}
	if _, err := exec.LookPath(bin); err != nil {
		log.Info("piper binary not found, TTS falling back to stub", slog.String("bin", bin))
		return stubSynthesizer{}, nil
	}
	exists := func(p string) bool {
		if p == "" {
			return false
		}
		st, err := os.Stat(p)
		return err == nil && !st.IsDir()
	}
	if !exists(zh) {
		zh = ""
	}
	if !exists(en) {
		en = ""
	}
	if !exists(shared) {
		shared = ""
	}
	if zh == "" && en == "" && shared == "" {
		log.Info("no piper voice models found, TTS falling back to stub")
		return stubSynthesizer{}, nil
	}
	return piperSynthesizer{bin: bin, model: shared, modelZH: zh, modelEN: en}, nil
}

// Name returns the name of the Piper engine.
func (p piperSynthesizer) Name() string { return "piper" }

// Real indicates whether this engine produces real speech audio.
func (p piperSynthesizer) Real() bool { return true }

// ModelFor resolves the model file path based on language hint.
func (p piperSynthesizer) ModelFor(lang string) string {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "zh", "zh-cn", "zh_cn", "cn":
		if p.modelZH != "" {
			return p.modelZH
		}
	case "en", "en-us", "en_us":
		if p.modelEN != "" {
			return p.modelEN
		}
	}
	return p.model
}

// SynthesizeLang synthesizes speech audio for text with language-specific voice.
func (p piperSynthesizer) SynthesizeLang(ctx context.Context, text, lang string) ([]byte, string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, "", fmt.Errorf("empty text")
	}
	if r := len([]rune(text)); r > maxTextRunes {
		return nil, "", fmt.Errorf("text too long: %d characters (max %d)", r, maxTextRunes)
	}
	model := p.ModelFor(lang)
	if model == "" {
		return nil, "", fmt.Errorf("piper: missing voice for lang %q", lang)
	}
	ctx, cancel := context.WithTimeout(ctx, synthTimeout)
	defer cancel()

	tmp, err := os.CreateTemp("", "tts-*.wav")
	if err != nil {
		return nil, "", err
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)

	cmd := exec.CommandContext(ctx, p.bin, "--model", model, "--output_file", path)
	cmd.Stdin = strings.NewReader(text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, "", fmt.Errorf("piper: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	return data, "audio/wav", nil
}

// stubSynthesizer generates synthetic sine-wave audio for testing.
type stubSynthesizer struct{}

// Name returns the stub engine name.
func (stubSynthesizer) Name() string { return "stub" }

// Real reports false for the stub engine.
func (stubSynthesizer) Real() bool { return false }

// SynthesizeLang returns a generated sine-wave WAV.
func (stubSynthesizer) SynthesizeLang(ctx context.Context, text, lang string) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(text) == "" {
		return nil, "", fmt.Errorf("empty text")
	}
	return sineWAV(16000, 0.5, 440), "audio/wav", nil
}

func sineWAV(sampleRate int, seconds, freqHz float64) []byte {
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
