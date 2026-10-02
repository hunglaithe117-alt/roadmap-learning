// Package practice chứa bounded context luyện nói: shadowing (A-B loop),
// ghi âm, so khớp transcript và sổ lỗi.
//
// Dữ liệu lưu trong bảng `notes` với prefix reserved, KHÔNG thêm bảng mới
// (xem STACK-V2-PLAN §2: practice sở hữu notes với prefix SHADOW|/ERR|).
// `content` context sở hữu prefix THIEU| (checklist 8 trục).
package practice

import "time"

// Prefixes reserved trong cột notes.text. Mỗi context 1 prefix, không
// trùng nhau — quét note bắt đầu bằng prefix là chọn đúng chủ sở hữu.
const (
	// ShadowPrefix lưu tiến độ shadowing của 1 thẻ (card_id NOT NULL).
	ShadowPrefix = "SHADOW|"
	// ErrorPrefix lưu 1 lần sai khi luyện nói (card_id NULL được).
	ErrorPrefix = "ERR|"
)

// ShadowSession là tiến độ shadowing hiện tại của 1 thẻ.
type ShadowSession struct {
	CardID int64
	// Loops là số vòng A-B đã nghe lại (0 = mới bắt đầu).
	Loops int
	// Rate là tốc độ phát 0.5-1.5. Mặc định 1 khi người dùng không gửi.
	Rate float64
	// UpdatedAt là mốc ghi time.Time; zero = chưa có session nào.
	UpdatedAt time.Time
}

// Recording là 1 lần ghi âm kèm kết quả chấm.
type Recording struct {
	CardID     int64
	Engine     string
	Transcript string
	// Duration là độ dài audio (giây); 0 nếu engine không trả.
	Duration float64
	// Diff là kết quả so khớp từ, có thể rỗng nếu chưa chấm.
	Diff []DiffToken
	// Score là tỉ lệ đúng 0-1.
	Score float64
	// Wrong là các từ sai/thiếu để lưu sổ lỗi.
	Wrong []string
	// CreatedAt là mốc ghi; zero = server sẽ điền.
	CreatedAt time.Time
}

// ErrorEntry là 1 dòng sổ lỗi: cặp (câu mẫu, transcript) + danh sách từ sai.
type ErrorEntry struct {
	ID int64
	// CardID nil khi lỗi không gắn thẻ nào (VD luyện tự do).
	CardID     *int64
	Expected   string
	Transcript string
	Wrong      []string
	CreatedAt  time.Time
}

// ThieuSession không ở đây: checklist 8 trục thuộc context `content`
// (prefix THIEU|, card_id NULL). Xem content.THIEUAxis.
