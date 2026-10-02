package audioinfra

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	audiodomain "langapp/internal/domain/audio"
)

// StubSynthesizer sinh WAV sóng sine để `<audio>` của trình duyệt phát được
// offline khi không bật `audio-service`.
//
// Đây là port nguyên văn `StubTTSEngine` + `genSineWAV` ở `api/audio.go` v1
// (kể cả hằng 16000Hz / 0.5s / 440Hz). Giữ nguyên để hành vi "chưa cấu hình
// audio" không đổi: trước M4 app trả đúng WAV này, nếu đổi thì mọi test cũ
// và mọi máy đang chạy không có audio-service đều đổi hành vi.
type StubSynthesizer struct{}

func (StubSynthesizer) Info() audiodomain.EngineInfo {
	return audiodomain.NewEngineInfo("stub", audiodomain.KindTTS, false)
}

func (s StubSynthesizer) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	return s.SynthesizeLang(ctx, text, "")
}

// SynthesizeLang bỏ qua `lang`: stub không có voice nào để chọn. Giữ method để
// handler gọi 1 đường duy nhất thay vì phải type-assert (port nguyên văn v1).
func (StubSynthesizer) SynthesizeLang(ctx context.Context, text, lang string) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(text) == "" {
		return nil, "", fmt.Errorf("empty text")
	}
	return SineWAV(16000, 0.5, 440), "audio/wav", nil
}

// SineWAV dựng WAV mono PCM 16-bit.
//
// 3 bản sao đang tồn tại: hàm này, `services/audio-service/synthesizer.go`, và
// `api/audio.go` (app v1). Chỉ 2 bản đầu là của M4.
//
// Lý do M4 chấp nhận nhân bản: đây là sinh WAV thuần, không phải quy tắc nghiệp
// vụ — chênh lệch không sinh ra hành vi sai, chỉ tốn vài dòng. NHƯNG lập luận ban
// đầu ("audio-service không import được package trong của app") là SAI, và nó là
// lý do suy ra rằng `SineWAV` phải nhân bản. Đã gộp `toSimplified` cho đúng
// (bảng 100 mục, lệch bảng = chấm sai oan); `SineWAV` để backlog M5–M7, ghi ở
// `phases/task-memory/stack-v2-m4-remediation.md`.
//
// Hệ số cố định 16000Hz / 0.5s / 440Hz giữ nguyên v1: đổi là mọi máy chưa bật
// audio-service đổi hành vi âm thanh.
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

// StubTranscriber trả transcript cố định — port `StubSTTEngine` v1 (mặc định
// "ni hao" / "zh").
type StubTranscriber struct {
	// Text rỗng = mặc định "ni hao" (khớp v1).
	Text string
	// Lang rỗng = mặc định "zh".
	Lang string
}

func (StubTranscriber) Info() audiodomain.EngineInfo {
	return audiodomain.NewEngineInfo("stub", audiodomain.KindSTT, false)
}

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
