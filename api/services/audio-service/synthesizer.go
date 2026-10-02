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
	// maxTextRunes chặn 1 request tts quá dài — khớp `maxTTSTextLen` của v1
	// (api/audio.go). Không có chặn này, 1 request có thể bắt Piper tổng hợp
	// hàng trăm KB và giữ 1 child process trong lúc đó.
	maxTextRunes = 500
	// synthTimeout là trần của 1 lần gọi Piper. Piper mất ~1s cho 1 câu;
	// 30s là ngân sách cho câu dài nhất rồi bỏ.
	synthTimeout = 30 * time.Second
	// chunkSize là kích thước 1 `Chunk` khi stream. 32KB = đúng 1 frame
	// media chunk hợp lý để client ghép, không phải cắt WAV giữa chừng.
	chunkSize = 32 << 10
)

// synthesizer là đằng sau interface `Audio.Synthesize`. Tách interface ở
// đây (thay vì dùng thẳng interface của domain/audio) vì tiến trình này là
// RANH GIỚI SERVICE: nó không được import tầng trong của app, chỉ nói chuyện
// qua proto. Interface ở domain vẫn là hợp đồng mà app bind ở
// internal/infrastructure/audio.
type synthesizer interface {
	SynthesizeLang(ctx context.Context, text, lang string) ([]byte, string, error)
	Name() string
	Real() bool
}

// piperSynthesizer chạy `echo <text> | $PIPER_BIN --model <voice>
// --output_file <tmp.wav>` — port nguyên văn từ `PiperTTSEngine.SynthesizeLang`
// ở api/audio.go v1, giữ nguyên vì hành vi (chọn voice theo lang, fallback
// voice chung) đã có test.
type piperSynthesizer struct {
	bin, model, modelZH, modelEN string
}

// newSynthesizerFromEnv trả stub khi không có voice file nào tồn tại, để app
// chạy được (và test được) không cần kéo model 752MB. Cùng luật với
// `NewTTSEngineFromEnv` v1.
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
		log.Info("không thấy piper binary, TTS dùng stub sine", slog.String("bin", bin))
		return stubSynthesizer{}, nil
	}
	// Bỏ voice cấu hình nhưng không tồn tại, giữ cái nào là file thật — cùng
	// hành vi v1.
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
		log.Info("không có voice piper nào tồn tại, TTS dùng stub sine")
		return stubSynthesizer{}, nil
	}
	return piperSynthesizer{bin: bin, model: shared, modelZH: zh, modelEN: en}, nil
}

func (p piperSynthesizer) Name() string { return "piper" }
func (p piperSynthesizer) Real() bool   { return true }

// ModelFor chọn file voice theo gợi ý lang. Ngôn ngữ lạ rơi về voice chung,
// và voice chung rỗng thì engine stub phía trên đã thay thế rồi.
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

// SynthesizeLang là hiện thực `synthesizer`. `lang` chọn voice; engine không
// có voice khớp thì rơi về voice chung, KHÔNG phải lỗi (giữ hành vi v1 khi
// ModelFor rỗng — `newSynthesizerFromEnv` đã thay bằng stub trong trường hợp đó).
func (p piperSynthesizer) SynthesizeLang(ctx context.Context, text, lang string) ([]byte, string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, "", fmt.Errorf("empty text")
	}
	if r := len([]rune(text)); r > maxTextRunes {
		return nil, "", fmt.Errorf("text quá dài: %d ký tự (tối đa %d)", r, maxTextRunes)
	}
	model := p.ModelFor(lang)
	if model == "" {
		return nil, "", fmt.Errorf("piper: không có voice cho lang %q", lang)
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

// stubSynthesizer sinh WAV sóng sine để `<audio>` của trình duyệt phát được
// offline. Port nguyên văn `genSineWAV` ở api/audio.go v1.
type stubSynthesizer struct{}

func (stubSynthesizer) Name() string { return "stub" }
func (stubSynthesizer) Real() bool   { return false }

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
