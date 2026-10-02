// Package audio là bounded context TTS/STT. Nó KHÔNG sở hữu bảng nào
// (stateless) — chỉ định nghĩa value object trả về và 2 port mà
// application/audio và transport/grpc sẽ hiện thực.
package audio

import "time"

// Transcript là kết quả nhận dạng giọng nói.
type Transcript struct {
	Text string
	// Lang là mã ngôn ngữ engine tự nhận dạng ("zh", "en", có thể rỗng).
	Lang string
	// Words là chi tiết theo từ, RỘNG (nil) nếu engine không có word-level
	// timestamp. Rỗng thì UI bỏ thanh highlight từ.
	Words []WordTimestamp
	// Confidence là độ tin cậy 0-1, RỘNG (nil) nếu engine không trả.
	Confidence *float64
}

// WordTimestamp là 1 từ kèm mốc thời gian trong audio. Mọi trường thời
// gian/độ tin cậy đều RỘNG vì engine khác nhau trả khác nhau.
type WordTimestamp struct {
	Word       string
	Start      *float64
	End        *float64
	Confidence *float64
}

// Duration là độ dài audio (giây) = End - Start. ok=false nếu thiếu mốc.
func (w WordTimestamp) Duration() (time.Duration, bool) {
	if w.Start == nil || w.End == nil {
		return 0, false
	}
	return time.Duration((*w.End - *w.Start) * float64(time.Second)), true
}

// EngineInfo là mô tả engine đang chạy, để response gắn header `X-Engine`
// và UI hiện badge engine. Không chứa bí mật (không kèm URL/credential).
type EngineInfo struct {
	// Name là tên engine ("piper", "stub", "faster-whisper").
	Name string
	// Kind phân biệt hướng: "tts" hoặc "stt".
	Kind Kind
	// Real = false nghĩa là engine stub (synthesize sine / transcript rỗng).
	// UI dùng để cảnh báo "kết quả không thật" thay vì im lặng.
	Real bool
}

// Kind là hướng của engine.
type Kind string

const (
	KindTTS Kind = "tts"
	KindSTT Kind = "stt"
)

// Header là tên header gửi kèm response để client biết engine nào đã xử lý
// (kể cả khi lỗi — client vẫn cần biết để hiển thị thông báo đúng).
const Header = "X-Engine"

// NewEngineInfo dựng info từ tên + hướng; isReal=false đánh dấu stub.
func NewEngineInfo(name string, kind Kind, isReal bool) EngineInfo {
	return EngineInfo{Name: name, Kind: kind, Real: isReal}
}
