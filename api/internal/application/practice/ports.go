// Package practice chứa tầng orchestration của bounded context luyện nói:
// tiến độ shadowing (vòng A-B), ghi âm → nhận dạng → so khớp, và sổ lỗi.
//
// Dữ liệu lưu trong bảng `notes` với 2 prefix reserved: SHADOW| (tiến độ
// shadowing, card_id NOT NULL) và ERR| (sổ lỗi, card_id NULL được). Không
// thêm bảng/cột — contract v4 giữ nguyên.
//
// Lớp này KHÔNG import driver DB và KHÔNG import tầng hạ tầng
// (STACK-V2-PLAN §2). Mọi truy cập DB đi qua interface dưới đây; engine
// TTS/STT đi qua port khai ở đây (hiện thực nằm ở transport/grpc M4).
package practice

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Tx là handle transaction do UnitOfWork tạo và đưa cho repository.
//
// Kiểu `any` cố ý: application không biết driver, còn infrastructure tự ép
// kiểu về *gorm.DB. Đổi sang interface có method sẽ buộc infrastructure phải
// implement marker method của package này — tức là hướng phụ thuộc đảo ngược.
type Tx = any

// UnitOfWork chạy 1 khối ghi trong 1 transaction. Sổ lỗi ghi 1 dòng nên
// transaction không bắt buộc về mặt nguyên tử, nhưng AppendError vẫn bọc:
// `notes.card_id` có FK, lỗi ràng buộc phải lộ ra trước khi service trả về
// cho client chứ không phải ở lần đọc sau.
type UnitOfWork interface {
	Do(ctx context.Context, fn func(tx Tx) error) error
}

// Repository là toàn bộ truy cập bảng `notes` mà context practice cần.
// Mọi truy vấn đã lọc theo prefix ở đây — không để caller nhớ viết `LIKE
// 'ERR|%'`, vì app v1 đã từng quên chỗ này và lẫn checklist THIEU vào sổ lỗi.
type Repository interface {
	// AppendNote ghi 1 note (prefix SHADOW| hoặc ERR|) và điền ID.
	AppendNote(ctx context.Context, tx Tx, n *Note) error
	// LatestShadowNote trả note SHADOW| mới nhất của 1 thẻ. Không có thì
	// (nil, nil) — "chưa luyện" là kết quả hợp lệ, không phải lỗi.
	LatestShadowNote(ctx context.Context, cardID int64) (*Note, error)
	// ListErrorNotes trả note ERR| mới nhất trước, tuỳ chọn lọc theo thẻ.
	ListErrorNotes(ctx context.Context, cardID *int64, limit int) ([]Note, error)
	// CountErrorNotesByCard JOIN `cards` để đếm lỗi theo thẻ trong 1 query
	// duy nhất. Thẻ đã xoá mềm tự rớt (INNER JOIN), đồng thời tránh query
	// lồng khi MaxOpenConns = 1.
	CountErrorNotesByCard(ctx context.Context, limit int) ([]CardErrorCount, error)
}

// STTPort là port nhận dạng giọng nói. Hiện thực v1: HTTP client tới Whisper
// sidecar; M4 thay bằng client gRPC. Khai ở application (không dùng
// `domain/audio.STTTranscriber`) vì practice chỉ cần ĐÚNG 1 hàm, còn
// domain/audio mô tả cả 2 hướng TTS/STT cho cả app.
type STTPort interface {
	// Transcribe trả transcript từ bytes audio. filename/contentType đi
	// kèm để engine tự đoán định dạng (wav vs m4a vs webm).
	Transcribe(ctx context.Context, audio []byte, filename, contentType string) (Transcript, error)
}

// TTSPort là port tổng hợp giọng nói cho mẫu luyện. Practice không bắt buộc
// dùng (client tự gọi `GET /api/tts`), nhưng use case `SpeakSample` cần nó để
// trả audio mẫu kèm transcript khi chấm — nếu không sau này sẽ phải thêm 1
// endpoint chỉ để ghép 2 nguồn.
type TTSPort interface {
	Synthesize(ctx context.Context, text, lang string) ([]byte, string, error)
}

// Transcript là kết quả STT đã rút gọn về phần practice dùng (text + ngôn
// ngữ engine tự nhận dạng). Chi tiết theo từ không dùng ở đây nên không khai.
type Transcript struct {
	Text string
	Lang string
}

// Note là 1 dòng `notes` — bảng dùng chung cho 3 context, mỗi context 1 prefix.
type Note struct {
	ID        int64
	CardID    *int64
	Text      string
	CreatedAt string
	GUID      string
}

// CardErrorCount là số lần sai của 1 thẻ + mặt trước/sau để client hiện gợi ý
// ôn lại mà không phải gọi thêm 1 endpoint.
type CardErrorCount struct {
	CardID int64
	Front  string
	Back   string
	Errors int
}

// NowFunc là nguồn thời gian, inject để test được và để 1 request dùng chung
// 1 mốc.
type NowFunc func() time.Time

// Clock mặc định: UTC.
func Clock() time.Time { return time.Now().UTC() }

// NewGUID sinh guid cho row mới.
//
// KHÔNG BAO GIỚ sinh guid rỗng: `ux_notes_guid` là UNIQUE trên cột
// `TEXT NOT NULL DEFAULT ”`, nên 2 note liên tiếp cùng guid rỗng là đụng
// nhau. `guid` cũng là khoá union khi sync: 2 note khác cùng guid rỗng sẽ bị
// coi là 1 và mất 1 note trên máy peer.
func NewGUID() string { return uuid.NewString() }
