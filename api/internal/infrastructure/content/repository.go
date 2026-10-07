package contentinfra

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	app "langapp/internal/application/content"
	"langapp/internal/platform/txtx"
)

// unitOfWork implements app.UnitOfWork using GORM transactions.
type unitOfWork struct{ db *gorm.DB }

// NewUnitOfWork constructs a UnitOfWork on the provided GORM DB.
func NewUnitOfWork(db *gorm.DB) app.UnitOfWork { return &unitOfWork{db: db} }

func (u *unitOfWork) Do(ctx context.Context, fn func(app.Tx) error) error {
	return txtx.Do(ctx, u.db, fn)
}

// txOf returns the scoped *gorm.DB from tx, falling back to root if tx is nil.
func txOf(root *gorm.DB, tx app.Tx) (*gorm.DB, error) {
	return txtx.Of(root, tx)
}

// txCtx combines txOf with context.
func txCtx(root *gorm.DB, ctx context.Context, tx app.Tx) (*gorm.DB, error) {
	db, err := txOf(root, tx)
	if err != nil {
		return nil, err
	}
	return db.WithContext(ctx), nil
}

// Repository implements app.Repository.
type Repository struct{ db *gorm.DB }

// NewRepository constructs a content Repository on a GORM DB.
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

var _ app.Repository = (*Repository)(nil)

// SearchZH queries the Chinese dictionary using dict_search.
func (r *Repository) SearchZH(ctx context.Context, q string, limit int) ([]app.ZHEntry, error) {
	var rows []app.ZHEntry
	if err := r.db.WithContext(ctx).Raw(
		"SELECT hanzi, pinyin, nghia FROM dict_search(?, ?)", q, limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("tra dict: %w", err)
	}
	if rows == nil {
		rows = []app.ZHEntry{}
	}
	return rows, nil
}

// SearchEN queries the English dictionary using en_dict_search.
func (r *Repository) SearchEN(ctx context.Context, q string, limit int) ([]app.ENEntry, error) {
	var rows []app.ENEntry
	if err := r.db.WithContext(ctx).Raw(
		"SELECT lang, term, reading, gloss FROM en_dict_search(?, ?)", q, limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("tra en_dict: %w", err)
	}
	if rows == nil {
		rows = []app.ENEntry{}
	}
	return rows, nil
}

// LookupEN performs an exact match lookup on an English headword.
func (r *Repository) LookupEN(ctx context.Context, term string) (app.ENEntry, bool, error) {
	var row EnDict
	err := r.db.WithContext(ctx).
		Where("lower(term) = lower(?)", strings.TrimSpace(term)).
		Order("id").Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return app.ENEntry{}, false, nil
		}
		return app.ENEntry{}, false, err
	}
	return enToApp(row), true, nil
}

// DictHanziSet returns the set of Chinese characters present in the dictionary.
func (r *Repository) DictHanziSet(ctx context.Context, tx app.Tx) (map[string]bool, error) {
	db, err := txCtx(r.db, ctx, tx)
	if err != nil {
		return nil, err
	}
	var rows []struct{ Hanzi string }
	if err := db.Model(&Dict{}).Select("hanzi").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc tập chữ dict: %w", err)
	}
	out := make(map[string]bool, len(rows))
	for _, row := range rows {
		out[row.Hanzi] = true
	}
	return out, nil
}

// InsertDict inserts a dictionary entry.
func (r *Repository) InsertDict(ctx context.Context, tx app.Tx, e *app.ZHEntry) (bool, error) {
	db, err := txCtx(r.db, ctx, tx)
	if err != nil {
		return false, err
	}
	row := Dict{Hanzi: e.Hanzi, Pinyin: e.Pinyin, Nghia: e.Nghia}
	if err := db.Create(&row).Error; err != nil {
		return false, err
	}
	return true, nil
}

// CountDict returns the number of rows in the dict table.
func (r *Repository) CountDict(ctx context.Context) (int, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&Dict{}).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("đếm dict: %w", err)
	}
	return int(n), nil
}

// InsertEN inserts an English dictionary entry.
func (r *Repository) InsertEN(ctx context.Context, tx app.Tx, e *app.ENEntry) (bool, error) {
	db, err := txCtx(r.db, ctx, tx)
	if err != nil {
		return false, err
	}
	row := EnDict{Lang: e.Lang, Term: e.Term, Reading: e.Reading, Gloss: e.Gloss}
	if err := db.Create(&row).Error; err != nil {
		return false, err
	}
	return true, nil
}

// CountEN returns the number of rows in the en_dict table.
func (r *Repository) CountEN(ctx context.Context) (int, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&EnDict{}).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("đếm en_dict: %w", err)
	}
	return int(n), nil
}

// InsertNote inserts a note entry.
func (r *Repository) InsertNote(ctx context.Context, tx app.Tx, n *app.Note) error {
	db, err := txCtx(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := Note{CardID: n.CardID, Text: n.Text, CreatedAt: n.CreatedAt, GUID: n.GUID}
	if err := db.Create(&row).Error; err != nil {
		return err
	}
	n.ID = row.ID
	return nil
}

// ListNotesByPrefix returns notes starting with prefix, newest first.
func (r *Repository) ListNotesByPrefix(ctx context.Context, prefix string, limit int) ([]app.Note, error) {
	var rows []Note
	if err := r.db.WithContext(ctx).Where("text LIKE ?", prefix+"%").
		Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc note prefix %s: %w", prefix, err)
	}
	out := make([]app.Note, 0, len(rows))
	for _, row := range rows {
		out = append(out, noteToApp(row))
	}
	return out, nil
}
