// Package practiceinfra provides the GORM implementation for the practice bounded context.
package practiceinfra

import (
	"context"
	"errors"

	"gorm.io/gorm"

	app "langapp/internal/application/practice"
	pdp "langapp/internal/domain/practice"
	"langapp/internal/platform/txtx"
)

type unitOfWork struct{ db *gorm.DB }

// NewUnitOfWork constructs a UnitOfWork backed by GORM.
func NewUnitOfWork(db *gorm.DB) app.UnitOfWork { return &unitOfWork{db: db} }

func (u *unitOfWork) Do(ctx context.Context, fn func(app.Tx) error) error {
	return txtx.Do(ctx, u.db, fn)
}

func txOf(root *gorm.DB, tx app.Tx) (*gorm.DB, error) {
	return txtx.Of(root, tx)
}

func txCtx(root *gorm.DB, ctx context.Context, tx app.Tx) (*gorm.DB, error) {
	db, err := txOf(root, tx)
	if err != nil {
		return nil, err
	}
	return db.WithContext(ctx), nil
}

// noteRow represents a row in the notes table.
type noteRow struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	CardID    *int64 `gorm:"column:card_id"`
	Text      string `gorm:"column:text"`
	CreatedAt string `gorm:"column:created_at;autoCreateTime:false"`
	GUID      string `gorm:"column:guid"`
}

func (noteRow) TableName() string { return "notes" }

func noteToApp(n noteRow) app.Note {
	return app.Note{ID: n.ID, CardID: n.CardID, Text: n.Text,
		CreatedAt: n.CreatedAt, GUID: n.GUID}
}

// Repository implements app.Repository.
type Repository struct{ db *gorm.DB }

// NewRepository constructs a practice Repository on a GORM DB.
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

var _ app.Repository = (*Repository)(nil)

// AppendNote appends a note row and populates its generated ID.
func (r *Repository) AppendNote(ctx context.Context, tx app.Tx, n *app.Note) error {
	db, err := txCtx(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := noteRow{CardID: n.CardID, Text: n.Text, CreatedAt: n.CreatedAt, GUID: n.GUID}
	if err := db.Create(&row).Error; err != nil {
		return err
	}
	n.ID = row.ID
	return nil
}

// LatestShadowNote returns the most recent shadowing note for a card, or (nil, nil) if none exists.
func (r *Repository) LatestShadowNote(ctx context.Context, cardID int64) (*app.Note, error) {
	var row noteRow
	err := r.db.WithContext(ctx).
		Where("card_id = ? AND text LIKE ?", cardID, pdp.ShadowPrefix+"%").
		Order("id DESC").Limit(1).Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	out := noteToApp(row)
	return &out, nil
}

// ListErrorNotes returns recent error notes in descending order.
func (r *Repository) ListErrorNotes(ctx context.Context, cardID *int64, limit int) ([]app.Note, error) {
	q := r.db.WithContext(ctx).Model(&noteRow{}).
		Where("text LIKE ?", pdp.ErrorPrefix+"%").
		Where("text NOT LIKE ?", "THIEU|%")
	if cardID != nil {
		q = q.Where("card_id = ?", *cardID)
	}
	var rows []noteRow
	if err := q.Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]app.Note, 0, len(rows))
	for _, row := range rows {
		out = append(out, noteToApp(row))
	}
	return out, nil
}

// CountErrorNotesByCard aggregates error notes per card in descending order.
func (r *Repository) CountErrorNotesByCard(ctx context.Context, limit int) ([]app.CardErrorCount, error) {
	var rows []app.CardErrorCount
	err := r.db.WithContext(ctx).Raw(`
		SELECT n.card_id, c.front, c.back, COUNT(*) AS errors
		FROM notes n
		JOIN cards c ON c.id = n.card_id AND c.deleted = 0
		WHERE n.card_id IS NOT NULL
		  AND n.text LIKE ?
		  AND n.text NOT LIKE ?
		GROUP BY n.card_id, c.front, c.back
		ORDER BY errors DESC, n.card_id
		LIMIT ?`, pdp.ErrorPrefix+"%", "THIEU|%", limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
