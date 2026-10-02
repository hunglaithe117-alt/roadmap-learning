package contentinfra

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	app "langapp/internal/application/content"
)

// SrsAdapter hiện thực 3 port của application/content (`DeckWriter`,
// `CardToneWriter`, `CardReader`) trên bảng `decks`/`cards`.
//
// VÌ SAO NÓ Ở ĐÂY chứ không ở infrastructure/srs: bảng `decks`/`cards` thuộc
// bounded context `srs`, nhưng M3 không được ghi vào
// `internal/infrastructure/srs/**` (phạm vi ghi song song với lane đang sửa
// repository.go của srs). Đặt adapter ở đây giữ ranh giới đúng ở M3; M4 khi
// wiring có thể chuyển nó sang `infrastructure/srs` mà không đổi interface.
//
// Interface vẫn được khai ở application/content: đó là điểm quan trọng —
// `content` phụ thuộc `srs` qua interface, không đọc bảng của srs trực tiếp
// (STACK-V2-PLAN §2).
type SrsAdapter struct{ db *gorm.DB }

// NewSrsAdapter dựng adapter trên pool GORM.
func NewSrsAdapter(db *gorm.DB) *SrsAdapter { return &SrsAdapter{db: db} }

var (
	_ app.DeckWriter     = (*SrsAdapter)(nil)
	_ app.CardToneWriter = (*SrsAdapter)(nil)
	_ app.CardReader     = (*SrsAdapter)(nil)
)

// deckRow là phần cột của `decks` mà adapter này cần. Không dùng struct của
// infrastructure/srs: import chéo giữa 2 package hạ tầng sẽ khóa chúng vào
// nhau khi 1 cái đổi cấu trúc bảng.
type deckRow struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Name      string `gorm:"column:name"`
	Lang      string `gorm:"column:lang"`
	CreatedAt string `gorm:"column:created_at;autoCreateTime:false"`
	GUID      string `gorm:"column:guid"`
	UpdatedAt string `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted   int    `gorm:"column:deleted"`
}

func (deckRow) TableName() string { return "decks" }

// cardRow là phần cột của `cards` mà adapter này cần.
type cardRow struct {
	ID        int64   `gorm:"column:id;primaryKey;autoIncrement"`
	DeckID    int64   `gorm:"column:deck_id"`
	Front     string  `gorm:"column:front"`
	Back      string  `gorm:"column:back"`
	Pinyin    string  `gorm:"column:pinyin"`
	DueAt     string  `gorm:"column:due_at"`
	CreatedAt string  `gorm:"column:created_at;autoCreateTime:false"`
	Tone      *string `gorm:"column:tone"`
	IPA       *string `gorm:"column:ipa"`
	Stress    *string `gorm:"column:stress"`
	State     string  `gorm:"column:state"`
	GUID      string  `gorm:"column:guid"`
	UpdatedAt string  `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted   int     `gorm:"column:deleted"`
}

func (cardRow) TableName() string { return "cards" }

// EnsureSeedDeck trả deck đang sống theo tên, tạo mới nếu chưa có.
//
// `guid` truyền vào là guid ỔN ĐỊNH (uuid5 theo tên + lang) — 2 máy cùng
// import HSK1 cho ra cùng guid thì merge khớp mà không nhân đôi thẻ. Deck đã
// tồn tại mà guid khác rỗng thì KHÔNG ghi đè: guid là natural key đã sinh ra
// ở máy khác, đổi nó là phá hội tụ.
func (a *SrsAdapter) EnsureSeedDeck(ctx context.Context, tx app.Tx, name, lang, guid, now string) (app.DeckRef, error) {
	db, err := txCtx(a.db, ctx, tx)
	if err != nil {
		return app.DeckRef{}, err
	}
	var row deckRow
	err = db.Where("name = ? AND deleted = 0", name).Take(&row).Error
	switch {
	case err == nil:
		if row.Lang != lang {
			return app.DeckRef{}, app.ErrLangMismatch
		}
		return app.DeckRef{ID: row.ID, Name: row.Name, Lang: row.Lang}, nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return app.DeckRef{}, fmt.Errorf("đọc deck %q: %w", name, err)
	}
	row = deckRow{Name: name, Lang: lang, CreatedAt: now, GUID: guid, UpdatedAt: now}
	if err := db.Create(&row).Error; err != nil {
		return app.DeckRef{}, fmt.Errorf("tạo deck %q: %w", name, err)
	}
	return app.DeckRef{ID: row.ID, Name: name, Lang: lang}, nil
}

// UpsertSeedCard chèn thẻ seed theo 3 nhánh, port 1:1 từ v1:
//
//  1. Chưa có thẻ nào cùng `front` → INSERT, trả true (thẻ mới).
//  2. Có thẻ đang XOÁ MỀM cùng `front` → hồi sinh chính row đó (giữ `id` và
//     `guid` để 2 máy hội tụ và mọi `reviews` trỏ `card_id` số còn đúng). Không
//     hồi sinh thì import lại tạo row thứ 2 cùng front và peer merge nhầm là 1
//     thẻ.
//  3. Có thẻ đang sống → KHÔNG ghi nội dung (trả false). `ux_cards_deck_front`
//     là partial index trên `deleted = 0` nên về mặt DB lệnh INSERT mới
//     không đụng index — nhưng 2 row cùng front đều sống thì peer merge sẽ
//     nhân đôi. Nên kiểm tra trước, không dựa vào index.
func (a *SrsAdapter) UpsertSeedCard(ctx context.Context, tx app.Tx, c app.SeedCard) (bool, error) {
	db, err := txCtx(a.db, ctx, tx)
	if err != nil {
		return false, err
	}
	var existing cardRow
	err = db.Where("deck_id = ? AND front = ?", c.DeckID, c.Front).
		Order("deleted ASC, id ASC").Take(&existing).Error
	switch {
	case err == nil && existing.Deleted == 0:
		return false, nil
	case err == nil:
		// Hồi sinh tombstone. `updated_at` set TƯỜNG MINH khi TouchUpdated
		// (khớp v1) để peer nhận biết thay đổi; ngược lại để trigger chạm.
		values := map[string]any{
			"back": c.Back, "pinyin": c.Pinyin, "tone": c.Tone,
			"ipa": c.IPA, "stress": c.Stress,
			"due_at": c.DueAt, "deleted": 0,
		}
		if c.TouchUpdated {
			values["updated_at"] = c.Now
		}
		if err := db.Model(&existing).Where("deleted = 1").
			Omit("updated_at").Updates(values).Error; err != nil {
			return false, fmt.Errorf("hồi sinh thẻ seed: %w", err)
		}
		return true, nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return false, fmt.Errorf("đọc thẻ seed: %w", err)
	}
	row := cardRow{
		DeckID: c.DeckID, Front: c.Front, Back: c.Back, Pinyin: c.Pinyin,
		DueAt: c.DueAt, CreatedAt: c.Now, Tone: c.Tone, IPA: c.IPA,
		Stress: c.Stress, GUID: c.GUID, UpdatedAt: c.Now,
		// `state = 'new'` là điều kiện để DueFilter kéo thẻ chưa từng ôn vào
		// hàng đợi dù due_at còn ở tương lai (xem domain/srs DueFilter).
		State: "new",
	}
	if err := db.Create(&row).Error; err != nil {
		return false, fmt.Errorf("tạo thẻ seed: %w", err)
	}
	return true, nil
}

// CountDeckCards đếm thẻ còn sống của deck.
func (a *SrsAdapter) CountDeckCards(ctx context.Context, tx app.Tx, deckID int64) (int, error) {
	db, err := txCtx(a.db, ctx, tx)
	if err != nil {
		return 0, err
	}
	var n int64
	if err := db.Model(&cardRow{}).
		Where("deck_id = ? AND deleted = 0", deckID).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("đếm thẻ deck: %w", err)
	}
	return int(n), nil
}

// SetCardTone ghi `cards.tone` và trả giá trị sau khi ghi.
func (a *SrsAdapter) SetCardTone(ctx context.Context, tx app.Tx, cardID int64, tone *string) (string, error) {
	db, err := txCtx(a.db, ctx, tx)
	if err != nil {
		return "", err
	}
	var row cardRow
	if err := db.Where("id = ? AND deleted = 0", cardID).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", app.ErrNotFound
		}
		return "", err
	}
	// Omit("updated_at") để trigger `trg_cards_touch_updated` chạm: tự ghi
	// updated_at sẽ làm trigger bỏ qua và mốc LWW phía peer sai.
	if err := db.Model(&row).Omit("updated_at").
		UpdateColumn("tone", tone).Error; err != nil {
		return "", fmt.Errorf("lưu thanh điệu: %w", err)
	}
	if tone == nil {
		return "", nil
	}
	return *tone, nil
}

// CardExists báo thẻ còn sống hay không.
func (a *SrsAdapter) CardExists(ctx context.Context, id int64) (bool, error) {
	var n int64
	if err := a.db.WithContext(ctx).Model(&cardRow{}).
		Where("id = ? AND deleted = 0", id).Count(&n).Error; err != nil {
		return false, fmt.Errorf("kiểm tra thẻ %d: %w", id, err)
	}
	return n > 0, nil
}
