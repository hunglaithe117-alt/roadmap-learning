// Package srsinfra là hiện thực GORM của bounded context srs. Đây là nơi DUY
// NHẤT trong app được phép import gorm.io (STACK-V2-PLAN §2).
package srsinfra

import (
	app "langapp/internal/application/srs"
)

// Deck là row `decks`.
//
// KHÔNG dùng gorm.DeletedAt: cột `deleted INTEGER 0|1` là tombstone mà toàn bộ
// logic sync LWW đã xong phụ thuộc vào (STACK-V2-PLAN §4.4). GORM sẽ tự thêm
// `WHERE deleted_at IS NULL` nếu field kiểu gorm.DeletedAt — sai hoàn toàn.
// `Deleted int` + lọc thủ công ở repository.
//
// `UpdatedAt string` là CỐ Ý không phải time.Time: trigger
// `trg_decks_touch_updated` gán mốc UTC dạng TEXT, và field tên `UpdatedAt`
// kiểu time.Time sẽ bị GORM tự ghi đè (bypass trigger → sai mốc LWW).
type Deck struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Name      string `gorm:"column:name"`
	Lang      string `gorm:"column:lang"`
	CreatedAt string `gorm:"column:created_at;autoCreateTime:false"`
	GUID      string `gorm:"column:guid"`
	UpdatedAt string `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted   int    `gorm:"column:deleted"`
}

// TableName khoá tên bảng: GORM đoán `srs_infra_decks` từ tên package, sai.
func (Deck) TableName() string { return "decks" }

func (d Deck) toApp() app.Deck { return app.Deck(d) }

func deckFromApp(d app.Deck) Deck { return Deck(d) }

// Card là row `cards`. Các cột NULL (tone/ipa/stress/audio_url) là *string để
// phân biệt NULL với chuỗi rỗng.
type Card struct {
	ID         int64   `gorm:"column:id;primaryKey;autoIncrement"`
	DeckID     int64   `gorm:"column:deck_id"`
	Front      string  `gorm:"column:front"`
	Back       string  `gorm:"column:back"`
	Pinyin     string  `gorm:"column:pinyin"`
	DueAt      string  `gorm:"column:due_at"`
	Stability  float64 `gorm:"column:stability"`
	Difficulty float64 `gorm:"column:difficulty"`
	Reps       int     `gorm:"column:reps"`
	Lapses     int     `gorm:"column:lapses"`
	State      string  `gorm:"column:state"`
	CreatedAt  string  `gorm:"column:created_at;autoCreateTime:false"`
	Tone       *string `gorm:"column:tone"`
	IPA        *string `gorm:"column:ipa"`
	Stress     *string `gorm:"column:stress"`
	AudioURL   *string `gorm:"column:audio_url"`
	GUID       string  `gorm:"column:guid"`
	UpdatedAt  string  `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted    int     `gorm:"column:deleted"`
}

// TableName khoá tên bảng (xem Deck.TableName).
func (Card) TableName() string { return "cards" }

func (c Card) toApp() app.Card { return app.Card(c) }

func cardFromApp(c app.Card) Card { return Card(c) }

// Review là row `reviews`. KHÔNG có `Deleted`: lịch sử ôn là append-only, sync
// merge bằng union theo guid chứ không LWW — thêm tombstone vào đây sẽ làm mất
// lịch sử trên máy peer.
type Review struct {
	ID         int64  `gorm:"column:id;primaryKey;autoIncrement"`
	CardID     int64  `gorm:"column:card_id"`
	Grade      int    `gorm:"column:grade"`
	ReviewedAt string `gorm:"column:reviewed_at;autoCreateTime:false"`
	NextDueAt  string `gorm:"column:next_due_at;autoCreateTime:false"`
	GUID       string `gorm:"column:guid"`
}

// TableName khoá tên bảng (xem Deck.TableName).
func (Review) TableName() string { return "reviews" }

func reviewFromApp(r app.Review) Review { return Review(r) }
