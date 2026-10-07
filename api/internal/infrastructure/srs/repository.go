package srsinfra

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	app "langapp/internal/application/srs"
	"langapp/internal/platform/txtx"
)

// Write rules:
//  1. Update on instances with IDs to prevent accidental table-wide updates.
//  2. Re-read within transaction via txOf.
//  3. Do not nest queries inside rows.Next().
//  4. Manually filter deleted = 0.
//  5. Wrap mutations in UnitOfWork.

// unitOfWork implements app.UnitOfWork using GORM transactions.
type unitOfWork struct{ db *gorm.DB }

// NewUnitOfWork constructs a UnitOfWork on the provided GORM DB.
func NewUnitOfWork(db *gorm.DB) app.UnitOfWork { return &unitOfWork{db: db} }

func (u *unitOfWork) Do(ctx context.Context, fn func(app.Tx) error) error {
	return txtx.Do(ctx, u.db, fn)
}

func txOf(root *gorm.DB, tx app.Tx) (*gorm.DB, error) {
	return txtx.Of(root, tx)
}

func txDB(root *gorm.DB, ctx context.Context, tx app.Tx) (*gorm.DB, error) {
	db, err := txOf(root, tx)
	if err != nil {
		return nil, err
	}
	return db.WithContext(ctx), nil
}

func errNoRows(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return app.ErrNotFound
	}
	return err
}

// ── Repository ──────────────────────────────────────────────────────────────

// Repository implements app.Repository.
type Repository struct {
	db *gorm.DB
}

// NewRepository constructs an SRS Repository on a GORM DB.
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

var _ app.Repository = (*Repository)(nil)

// CreateDeck inserts a deck into the database.
func (r *Repository) CreateDeck(ctx context.Context, tx app.Tx, d *app.Deck) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := deckFromApp(*d)
	if err := db.Create(&row).Error; err != nil {
		return errNoRows(err)
	}
	if err := db.Where("id = ?", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*d = row.toApp()
	return nil
}

// ListDecks returns all non-deleted decks ordered by ID.
func (r *Repository) ListDecks(ctx context.Context) ([]app.Deck, error) {
	var rows []Deck
	err := r.db.WithContext(ctx).Where("deleted = 0").Order("id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list decks: %w", err)
	}
	out := make([]app.Deck, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toApp())
	}
	return out, nil
}

// DeckByID returns a deck by ID, or app.ErrNotFound if deleted or absent.
func (r *Repository) DeckByID(ctx context.Context, id int64) (app.Deck, error) {
	var row Deck
	err := r.db.WithContext(ctx).Where("id = ? AND deleted = 0", id).Take(&row).Error
	if err != nil {
		return app.Deck{}, errNoRows(err)
	}
	return row.toApp(), nil
}

// SoftDeleteDeck marks a deck and its cards as deleted.
func (r *Repository) SoftDeleteDeck(ctx context.Context, tx app.Tx, id int64) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	if err := db.Model(&Deck{ID: id}).Where("deleted = 0").
		UpdateColumn("deleted", 1).Error; err != nil {
		return errNoRows(err)
	}
	return errNoRows(db.Model(&Card{}).Where("deck_id = ? AND deleted = 0", id).
		UpdateColumn("deleted", 1).Error)
}

// CreateCard inserts a card or reuses a soft-deleted tombstone with the same front.
func (r *Repository) CreateCard(ctx context.Context, tx app.Tx, c *app.Card) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	var tomb Card
	err = db.Where("deck_id = ? AND front = ? AND deleted = 1", c.DeckID, c.Front).
		Order("id").Take(&tomb).Error
	switch {
	case err == nil:
		guid := tomb.GUID
		if guid == "" {
			guid = c.GUID
		}
		res := db.Model(&tomb).Where("deleted = 1").
			Updates(map[string]any{
				"back":    c.Back,
				"pinyin":  c.Pinyin,
				"due_at":  c.DueAt,
				"state":   c.State,
				"deleted": 0,
				"guid":    guid,
			})
		if res.Error != nil {
			return errNoRows(res.Error)
		}
		var after Card
		if err := db.Where("id = ? AND deleted = 0", tomb.ID).Take(&after).Error; err != nil {
			return errNoRows(err)
		}
		*c = after.toApp()
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		row := cardFromApp(*c)
		if err := db.Create(&row).Error; err != nil {
			return errNoRows(err)
		}
		*c = row.toApp()
		return nil
	default:
		return errNoRows(err)
	}
}

// CardByID returns a card by ID, or app.ErrNotFound if deleted or absent.
func (r *Repository) CardByID(ctx context.Context, id int64) (app.Card, error) {
	var row Card
	err := r.db.WithContext(ctx).Where("id = ? AND deleted = 0", id).Take(&row).Error
	if err != nil {
		return app.Card{}, errNoRows(err)
	}
	return row.toApp(), nil
}

// ListCards returns non-deleted cards for a deck ordered by ID.
func (r *Repository) ListCards(ctx context.Context, deckID int64) ([]app.Card, error) {
	var rows []Card
	err := r.db.WithContext(ctx).Where("deck_id = ? AND deleted = 0", deckID).
		Order("id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list cards: %w", err)
	}
	out := make([]app.Card, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toApp())
	}
	return out, nil
}

// UpdateCard updates editable card columns.
func (r *Repository) UpdateCard(ctx context.Context, tx app.Tx, c *app.Card) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := Card(*c)
	if err := db.Model(&row).Omit("updated_at").
		Updates(map[string]any{
			"front":      row.Front,
			"back":       row.Back,
			"pinyin":     row.Pinyin,
			"due_at":     row.DueAt,
			"stability":  row.Stability,
			"difficulty": row.Difficulty,
			"reps":       row.Reps,
			"lapses":     row.Lapses,
			"state":      row.State,
			"tone":       row.Tone,
			"ipa":        row.IPA,
			"stress":     row.Stress,
			"audio_url":  row.AudioURL,
		}).Error; err != nil {
		return errNoRows(err)
	}
	if err := db.Where("id = ? AND deleted = 0", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*c = row.toApp()
	return nil
}

// SoftDeleteCard marks a card as deleted.
func (r *Repository) SoftDeleteCard(ctx context.Context, tx app.Tx, id int64) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	return errNoRows(db.Model(&Card{ID: id}).Where("deleted = 0").
		UpdateColumn("deleted", 1).Error)
}

// CreateReview appends a review entry.
func (r *Repository) CreateReview(ctx context.Context, tx app.Tx, rv *app.Review) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := reviewFromApp(*rv)
	if err := db.Create(&row).Error; err != nil {
		return errNoRows(err)
	}
	rv.ID = row.ID
	return nil
}
