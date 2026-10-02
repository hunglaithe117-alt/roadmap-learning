// Package srs chứa tầng orchestration của bounded context spaced repetition:
// use case deck / card / review, gọi domain/srs để tính lịch ôn (ScheduleNext)
// và gọi repository (định nghĩa ở đây) để đọc/ghi.
//
// Lớp này KHÔNG import driver DB và KHÔNG import tầng hạ tầng (infrastructure)
// (STACK-V2-PLAN §2). Mọi truy cập DB đi qua interface dưới đây.
package srs

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

// UnitOfWork chạy 1 khối ghi trong 1 transaction; callback trả lỗi ⇒ rollback
// toàn bộ. RecordReview dùng nó vì UPDATE card + INSERT reviews phải là 1
// khối nguyên tử: crash giữa chừng để lại thẻ đã lên lịch mà không có lịch sử.
type UnitOfWork interface {
	Do(ctx context.Context, fn func(tx Tx) error) error
}

// Repository là toàn bộ truy cập DB của context srs mà tầng application cần.
// Mọi list đã lọc `deleted = 0` trong chính repository.
//
// MỌI METHOD GHI đều nhận `Tx` làm tham số đầu: đó là cách duy nhất để
// repository biết phải ghi vào transaction nào. Method đọc không nhận vì đọc
// ngoài transaction vẫn đúng (và bọc vào chỉ tốn connection — xem
// tầng infrastructure, repository.go luật 2).
type Repository interface {
	CreateDeck(ctx context.Context, tx Tx, d *Deck) error
	ListDecks(ctx context.Context) ([]Deck, error)
	DeckByID(ctx context.Context, id int64) (Deck, error)
	// DecksByIDs là BATCH của DeckByID cho dataloader `Stage.deck` (M4) —
	// xem `Service.FindDecks` để biết vì sao cần.
	DecksByIDs(ctx context.Context, ids []int64) ([]Deck, error)
	SoftDeleteDeck(ctx context.Context, tx Tx, id int64) error

	CreateCard(ctx context.Context, tx Tx, c *Card) error
	CardByID(ctx context.Context, id int64) (Card, error)
	ListCards(ctx context.Context, deckID int64) ([]Card, error)
	UpdateCard(ctx context.Context, tx Tx, c *Card) error
	SoftDeleteCard(ctx context.Context, tx Tx, id int64) error

	CreateReview(ctx context.Context, tx Tx, r *Review) error
}

// Deck là row `decks` đã đọc từ DB.
type Deck struct {
	ID        int64
	Name      string
	Lang      string
	CreatedAt string
	GUID      string
	UpdatedAt string
	Deleted   int
}

// Card là row `cards`. DoAt/CreatedAt/UpdatedAt là RFC3339 string vì cột là
// TEXT (không đổi từ SQLite v4) — repository quyết định format, domain dùng
// time.Time.
type Card struct {
	ID         int64
	DeckID     int64
	Front      string
	Back       string
	Pinyin     string
	DueAt      string
	Stability  float64
	Difficulty float64
	Reps       int
	Lapses     int
	State      string
	CreatedAt  string
	Tone       *string
	IPA        *string
	Stress     *string
	AudioURL   *string
	GUID       string
	UpdatedAt  string
	Deleted    int
}

// Review là row `reviews` — append-only, KHÔNG có `deleted` (lịch sử ôn không
// xoá mềm; sync merge bằng union theo guid).
type Review struct {
	ID         int64
	CardID     int64
	Grade      int
	ReviewedAt string
	NextDueAt  string
	GUID       string
}

// NewGUID sinh guid cho row mới.
//
// KHÔNG BAO GIỜ sinh guid rỗng: `ux_reviews_guid` là UNIQUE trên cột
// `TEXT NOT NULL DEFAULT ”`, nên 2 review liên tiếp cùng guid rỗng là đụng
// nhau và insert thứ 2 fail (ghi chú M1 remediation). `guid` cũng là natural
// key để peer union lịch sử ôn — 2 row khác cùng guid sẽ bị coi là 1.
func NewGUID() string { return uuid.NewString() }

// NowFunc là nguồn thời gian, inject để test được và để 1 request dùng chung
// 1 mốc.
type NowFunc func() time.Time

// Clock mặc định: UTC.
func Clock() time.Time { return time.Now().UTC() }
