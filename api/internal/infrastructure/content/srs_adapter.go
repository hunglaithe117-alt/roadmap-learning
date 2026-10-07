package contentinfra

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	app "langapp/internal/application/content"
)

// SrsAdapter implements DeckWriter, CardToneWriter, and CardReader for SRS tables.
type SrsAdapter struct{ db *gorm.DB }

// NewSrsAdapter constructs an SrsAdapter on a GORM DB.
func NewSrsAdapter(db *gorm.DB) *SrsAdapter { return &SrsAdapter{db: db} }

var (
	_ app.DeckWriter     = (*SrsAdapter)(nil)
	_ app.CardToneWriter = (*SrsAdapter)(nil)
	_ app.CardReader     = (*SrsAdapter)(nil)
)

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

// EnsureSeedDeck returns an existing active deck by name, or creates it.
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

// UpsertSeedCard inserts or revives a seed card, returning true if newly created or revived.
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
		State:  "new",
	}
	if err := db.Create(&row).Error; err != nil {
		return false, fmt.Errorf("tạo thẻ seed: %w", err)
	}
	return true, nil
}

// CountDeckCards returns the active card count for a deck.
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

// SetCardTone updates cards.tone and returns the updated value.
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
	if err := db.Model(&row).Omit("updated_at").
		UpdateColumn("tone", tone).Error; err != nil {
		return "", fmt.Errorf("lưu thanh điệu: %w", err)
	}
	if tone == nil {
		return "", nil
	}
	return *tone, nil
}

// CardExists reports whether an active card exists by ID.
func (a *SrsAdapter) CardExists(ctx context.Context, id int64) (bool, error) {
	var n int64
	if err := a.db.WithContext(ctx).Model(&cardRow{}).
		Where("id = ? AND deleted = 0", id).Count(&n).Error; err != nil {
		return false, fmt.Errorf("kiểm tra thẻ %d: %w", id, err)
	}
	return n > 0, nil
}
