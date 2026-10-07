package syncinfra

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"

	app "langapp/internal/application/sync"
)

// Write implementation of merge for decks and cards.

// ── decks ───────────────────────────────────────────────────────────────────

type deckMergeRow struct {
	ID        int64
	Name      string
	Lang      string
	CreatedAt string
	GUID      string
	UpdatedAt string
	Deleted   int
}

// TableName returns the table name for deckMergeRow.
func (deckMergeRow) TableName() string { return "decks" }

// TableName returns the table name for cardMergeRow.
func (cardMergeRow) TableName() string { return "cards" }

// DeckRows returns all local decks by GUID, including tombstones.
func (r *Repository) DeckRows(ctx context.Context, tx app.Tx) (map[string]app.DeckRow, error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return nil, err
	}
	var rows []deckMergeRow
	if err := db.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc decks: %w", err)
	}
	out := make(map[string]app.DeckRow, len(rows))
	for _, row := range rows {
		if row.GUID == "" {
			continue
		}
		out[row.GUID] = app.DeckRow{
			ID: row.ID, GUID: row.GUID, Name: row.Name, Lang: row.Lang,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Deleted: row.Deleted,
		}
	}
	return out, nil
}

// UpsertDeck inserts or updates a deck with explicit updated_at.
func (r *Repository) UpsertDeck(ctx context.Context, tx app.Tx, d app.DeckRow, found bool) (int64, error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return 0, err
	}
	if !found {
		return insertDeck(db, d)
	}
	if d.ID <= 0 {
		return 0, fmt.Errorf("update deck %s nhưng thiếu id local", d.GUID)
	}
	err = db.Exec(`UPDATE decks SET name = ?, lang = ?, deleted = ?, updated_at = ?
		WHERE id = ?`, d.Name, d.Lang, d.Deleted, d.UpdatedAt, d.ID).Error
	return d.ID, err
}

// ── cards ───────────────────────────────────────────────────────────────────

type cardMergeRow struct {
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
	IPA        *string `gorm:"column:ipa"`
	Stress     *string
	AudioURL   *string
	GUID       string
	UpdatedAt  string
	Deleted    int
}

// CardRows returns all local cards by GUID, including tombstones.
func (r *Repository) CardRows(ctx context.Context, tx app.Tx) (map[string]app.CardRow, error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return nil, err
	}
	var rows []cardMergeRow
	if err := db.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc cards: %w", err)
	}
	deckGUID, err := r.deckGUIDByID(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]app.CardRow, len(rows))
	for _, row := range rows {
		if row.GUID == "" {
			continue
		}
		out[row.GUID] = app.CardRow{
			ID: row.ID, GUID: row.GUID, DeckGUID: deckGUID[row.DeckID],
			Front: row.Front, Back: row.Back, Pinyin: row.Pinyin, DueAt: row.DueAt,
			Stability: row.Stability, Difficulty: row.Difficulty,
			Reps: row.Reps, Lapses: row.Lapses, State: row.State,
			CreatedAt: row.CreatedAt, Tone: row.Tone, IPA: row.IPA,
			Stress: row.Stress, AudioURL: row.AudioURL,
			UpdatedAt: row.UpdatedAt, Deleted: row.Deleted,
		}
	}
	return out, nil
}

// deckGUIDByID returns a map of deck ID to GUID.
func (r *Repository) deckGUIDByID(ctx context.Context, tx app.Tx) (map[int64]string, error) {
	var rows []struct {
		ID   int64
		GUID string
	}
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err := db.Table("decks").Select("id, guid").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc guid deck: %w", err)
	}
	out := make(map[int64]string, len(rows))
	for _, row := range rows {
		out[row.ID] = row.GUID
	}
	return out, nil
}

// TombstoneCard looks up a soft-deleted card by deck ID and front.
func (r *Repository) TombstoneCard(ctx context.Context, tx app.Tx, deckID int64, front string) (int64, string, bool, error) {
	var row struct {
		ID   int64
		GUID string
	}
	db, dbErr := r.txCtx(ctx, tx)
	if dbErr != nil {
		return 0, "", false, dbErr
	}
	err := db.Raw(`
		SELECT id, guid FROM cards
		WHERE deck_id = ? AND front = ? AND deleted = 1
		ORDER BY id LIMIT 1`, deckID, front).Row().Scan(&row.ID, &row.GUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", false, nil
		}
		return 0, "", false, fmt.Errorf("tra tombstone card: %w", err)
	}
	return row.ID, row.GUID, true, nil
}

// LiveCardGUIDByFront looks up an active card by deck ID and front.
func (r *Repository) LiveCardGUIDByFront(ctx context.Context, tx app.Tx, deckID int64, front string) (int64, string, bool, error) {
	var row struct {
		ID   int64
		GUID string
	}
	db, dbErr := r.txCtx(ctx, tx)
	if dbErr != nil {
		return 0, "", false, dbErr
	}
	err := db.Raw(`
		SELECT id, guid FROM cards
		WHERE deck_id = ? AND front = ? AND deleted = 0
		ORDER BY id LIMIT 1`, deckID, front).Row().Scan(&row.ID, &row.GUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", false, nil
		}
		return 0, "", false, fmt.Errorf("tra thẻ sống cùng front: %w", err)
	}
	return row.ID, row.GUID, true, nil
}

// insertDeck inserts a deck row, returning its ID or 0 on conflict.
func insertDeck(db *gorm.DB, d app.DeckRow) (int64, error) {
	var id int64
	err := db.Raw(`INSERT INTO decks (name, lang, created_at, guid, updated_at, deleted)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (guid) DO NOTHING
		RETURNING id`,
		d.Name, d.Lang, d.CreatedAt, d.GUID, d.UpdatedAt, d.Deleted).Scan(&id).Error
	return id, err
}

// CardIDByGUID looks up a card ID by deck ID and GUID.
func (r *Repository) CardIDByGUID(ctx context.Context, tx app.Tx, deckID int64, guid string) (int64, error) {
	var id int64
	db, dbErr := r.txCtx(ctx, tx)
	if dbErr != nil {
		return 0, dbErr
	}
	err := db.Raw(
		"SELECT id FROM cards WHERE deck_id = ? AND guid = ?", deckID, guid).Scan(&id).Error
	if err != nil {
		return 0, fmt.Errorf("tra card theo guid: %w", err)
	}
	return id, nil
}

// UpsertCard inserts or updates a card with explicit updated_at.
func (r *Repository) UpsertCard(ctx context.Context, tx app.Tx, c app.CardRow, found bool) (int64, bool, error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return 0, false, err
	}
	if c.DeckID == nil {
		return 0, false, fmt.Errorf("card %s chưa resolve được deck cha", c.GUID)
	}
	deckID := *c.DeckID
	if found {
		if c.ID <= 0 {
			return 0, false, fmt.Errorf("update card %s nhưng thiếu id local", c.GUID)
		}
		err := db.Exec(`UPDATE cards SET deck_id = ?, front = ?, back = ?, pinyin = ?,
			due_at = ?, stability = ?, difficulty = ?, reps = ?, lapses = ?, state = ?,
			tone = ?, ipa = ?, stress = ?, audio_url = ?, guid = ?, deleted = ?,
			updated_at = ?
			WHERE id = ?`,
			deckID, c.Front, c.Back, c.Pinyin, c.DueAt, c.Stability, c.Difficulty,
			c.Reps, c.Lapses, c.State, c.Tone, c.IPA, c.Stress, c.AudioURL, c.GUID,
			c.Deleted, c.UpdatedAt, c.ID).Error
		return c.ID, false, err
	}
	var id int64
	err = db.Raw(`INSERT INTO cards (deck_id, front, back, pinyin, due_at,
		stability, difficulty, reps, lapses, state, created_at,
		tone, ipa, stress, audio_url, guid, updated_at, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (deck_id, front) WHERE deleted = 0 DO NOTHING
		RETURNING id`,
		deckID, c.Front, c.Back, c.Pinyin, c.DueAt, c.Stability, c.Difficulty,
		c.Reps, c.Lapses, c.State, c.CreatedAt, c.Tone, c.IPA, c.Stress, c.AudioURL,
		c.GUID, c.UpdatedAt, c.Deleted).Scan(&id).Error
	if err != nil {
		return 0, false, fmt.Errorf("insert card %s: %w", c.GUID, err)
	}
	if id > 0 {
		return id, false, nil
	}
	liveID, _, ok, lookupErr := r.LiveCardGUIDByFront(ctx, tx, deckID, c.Front)
	if lookupErr != nil {
		return 0, false, lookupErr
	}
	if ok {
		return liveID, true, nil
	}
	guidID, guidErr := r.CardIDByGUID(ctx, tx, deckID, c.GUID)
	if guidErr != nil {
		return 0, false, guidErr
	}
	if guidID > 0 {
		return guidID, true, nil
	}
	return 0, false, fmt.Errorf("insert card %s bị skip nhưng không tra được thẻ trùng", c.GUID)
}
