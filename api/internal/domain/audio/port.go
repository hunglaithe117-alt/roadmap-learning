package audio

import "context"

// TTSSynthesizer là port tổng hợp giọng nói. Hiện thực v1: Piper (os/exec),
// M4 thay bằng client gRPC tới `services/audio-service` — interface này giữ
// nguyên nên chỗ gọi không đổi.
//
// Synthesize trả bytes WAV + content type; SynthesizeLang cho phép chọn
// voice theo ngôn ngữ (zh/en) — engine không hỗ trợ thì bỏ qua lang và dùng
// voice mặc định, KHÔNG phải lỗi (giữ hành vi v1 khi ModelFor rỗng).
type TTSSynthesizer interface {
	// Info mô tả engine để gắn header X-Engine.
	Info() EngineInfo
	Synthesize(ctx context.Context, text string) (audio []byte, contentType string, err error)
}

// LangSynthesizer là phần MỞ RỘNG (kiểu assertion) của TTSSynthesizer: engine
// biết chọn voice theo ngôn ngữ. Không ép vào interface chính để engine stub
// đơn giản không phải implement thừa.
type LangSynthesizer interface {
	SynthesizeLang(ctx context.Context, text, lang string) (audio []byte, contentType string, err error)
}

// STTTranscriber là port nhận dạng giọng nói. Hiện thực v1: HTTP client tới
// Whisper sidecar; M4 thay bằng client gRPC.
type STTTranscriber interface {
	Info() EngineInfo
	Transcribe(ctx context.Context, audio []byte, filename, contentType string) (Transcript, error)
}

// DetailedTranscriber là phần mở rộng: engine trả được chi tiết theo từ.
// Handler kiểm bằng type assertion — engine không có thì trả Transcript rỗng
// phần Words, không phải lỗi (khớp hành vi v1).
type DetailedTranscriber interface {
	TranscribeDetailed(ctx context.Context, audio []byte, filename, contentType, lang string) (Transcript, error)
}
