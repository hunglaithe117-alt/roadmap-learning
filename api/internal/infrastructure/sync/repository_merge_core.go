package syncinfra

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"

	app "langapp/internal/application/sync"
)

// File này hiện thực phần GHI của merge cho `decks` / `cards`.
//
// LUẬT KHÔNG ĐƯỢC PHÁ VỠ (xem đầu file package):
//   - MỌI UPDATE/INSERT đều set `updated_at` TƯỜNG MINH, KHÔNG `Omit`.
//   - Bắt buộc `WHERE id = ?` (Model theo INSTANCE có ID, không phải struct rỗng).
//   - Cột NULL dùng `*string`/`*float64` để phân biệt NULL với chuỗi rỗng —
//     `deck_id` NULL (stage chưa gắn deck) và `completed_at` NULL (node chưa
//     xong) là 2 trạng thái khác nhau.

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

// TableName khoá tên bảng: GORM đoán `deck_merge_rows` từ tên struct, sai.
func (deckMergeRow) TableName() string { return "decks" }

// TableName khoá tên bảng (xem deckMergeRow.TableName).
func (cardMergeRow) TableName() string { return "cards" }

// DeckRows trả mọi deck local theo guid, KỂ CẢ tombstone (`deleted = 1`).
// Bỏ tombstone khỏi map thì một deck đã xoá ở local sẽ bị peer "hồi sinh".
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
			continue // guid rỗng không định danh được (dữ liệu hỏng)
		}
		out[row.GUID] = app.DeckRow{
			ID: row.ID, GUID: row.GUID, Name: row.Name, Lang: row.Lang,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Deleted: row.Deleted,
		}
	}
	return out, nil
}

// UpsertDeck chèn (found=false) hoặc ghi đè (found=true) 1 deck, `updated_at`
// tường minh.
//
// `found=true` mà `ID = 0` = caller bảo update nhưng không có id → báo lỗi
// thay vì sinh câu UPDATE không có `WHERE` (nguy hiểm hơn nhiều).
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
	// `IPA` PHẢI có tag: không thì GORM suy tên cột thành `ip_a` (chữ viết
	// tắt bị tách), cột thật trong `cards` là `ipa`. Hậu quả KHÔNG phải lỗi
	// SQL (GORM đọc `SELECT *` rồi map theo tên) mà là field LUÔN nil ⇒
	// `CardRows` không bao giờ trả `ipa` của bản local ⇒ `cardValues` thiếu
	// khoá "ipa" ⇒ `differs()` luôn true với mọi thẻ có phiên âm, và luật
	// "2 bản giống hệt → keep" không chạy. Chốt DRY trong
	// `snapshot_columns_test.go` bắt được lỗi này.
	IPA       *string `gorm:"column:ipa"`
	Stress    *string
	AudioURL  *string
	GUID      string
	UpdatedAt string
	Deleted   int
}

// CardRows trả mọi card local theo guid, kể cả tombstone.
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

// deckGUIDByID dựng bản đồ id→guid của deck. Cần khi đọc card local: `Decks`
// trả map theo guid, còn card mang `deck_id` số.
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

// TombstoneCard tra card đang xoá mềm theo (deck_id, front) — dùng khi peer
// re-create 1 thẻ mà local đã xoá.
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
		// `Row().Scan` trả `sql.ErrNoRows`, KHÔNG phải `gorm.ErrRecordNotFound` —
		// so nhầm biến thành lỗi DB và làm hỏng cả merge.
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", false, nil
		}
		return 0, "", false, fmt.Errorf("tra tombstone card: %w", err)
	}
	return row.ID, row.GUID, true, nil
}

// LiveCardGUIDByFront tra guid của thẻ đang SỐNG cùng front. Dùng khi insert
// đụng `ux_cards_deck_front` vì 2 máy cùng tạo 1 thẻ tay: giữ bản local và
// ghi log xung đột thay vì báo lỗi làm hỏng cả merge.
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

// insertDeck chèn 1 deck, `ON CONFLICT` có arbiter + `RETURNING id`.
//
// VÌ SAO `ON CONFLICT` BẮT BUỘC Ở MỌI INSERT CỦA MERGE: Postgres **hủy cả
// transaction** khi 1 câu lệnh vi phạm UNIQUE (khác hẳn SQLite v1 chỉ trả lỗi
// cho câu lệnh đó). 2 máy cùng tạo 1 deck/thẻ tay là chuyện bình thường; nếu
// để insert nổ UNIQUE thì mọi bảng đã merge trước đó cũng mất.
// `id == 0` là tín hiệu "bị skip" — tương đương `INSERT OR IGNORE` của SQLite.
//
// Arbiter phải khai BÁO CHÍNH XÁC: `ON CONFLICT DO NOTHING` không chỉ định cột
// thì Postgres không suy ra được arbiter và vẫn ném lỗi unique từ mọi index.
// Với PARTIAL unique index (`ux_cards_deck_front ... WHERE deleted = 0`) phải
// kèm cả index predicate — nếu không, lỗi 23505 quay lại và hủy cả merge.
func insertDeck(db *gorm.DB, d app.DeckRow) (int64, error) {
	var id int64
	err := db.Raw(`INSERT INTO decks (name, lang, created_at, guid, updated_at, deleted)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (guid) DO NOTHING
		RETURNING id`,
		d.Name, d.Lang, d.CreatedAt, d.GUID, d.UpdatedAt, d.Deleted).Scan(&id).Error
	return id, err
}

// CardIDByGUID tra id của 1 card theo (deck, guid).
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

// UpsertCard chèn hoặc ghi đè 1 card, `updated_at` tường minh.
//
// Insert đụng `ux_cards_deck_front` (partial UNIQUE trên `deleted = 0`) khi 2
// máy cùng tạo 1 thẻ tay: KHÔNG trả lỗi — tra thẻ sống cùng front, giữ bản
// local, map cả 2 guid về cùng id để 2 máy hội tụ. Báo lỗi ở đây sẽ rollback
// cả merge vì 1 việc vô hại.
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
		// `guid` CỐ Ý được ghi lại: đây là đường hồi sinh tombstone, nơi row
		// local cũ phải nhận guid của peer để 2 máy hội tụ. Ở nhánh update
		// thường `guid` trùng sẵn nên ghi lại là no-op.
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
	// `ON CONFLICT` phải khai đúng arbiter — xem giải thích ở `insertDeck`.
	// `id == 0` = bị skip.
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
	// Bị skip: 2 máy cùng tạo 1 thẻ tay cùng front. Tra thẻ sống cùng front để
	// map 2 guid về cùng id và báo `dupKept` để caller ghi log xung đột —
	// im lặng khiến user tưởng dữ liệu peer bị mất.
	liveID, _, ok, lookupErr := r.LiveCardGUIDByFront(ctx, tx, deckID, c.Front)
	if lookupErr != nil {
		return 0, false, lookupErr
	}
	if ok {
		return liveID, true, nil
	}
	// Skip vì lý do khác (cùng guid): tra theo guid.
	guidID, guidErr := r.CardIDByGUID(ctx, tx, deckID, c.GUID)
	if guidErr != nil {
		return 0, false, guidErr
	}
	if guidID > 0 {
		return guidID, true, nil
	}
	return 0, false, fmt.Errorf("insert card %s bị skip nhưng không tra được thẻ trùng", c.GUID)
}
