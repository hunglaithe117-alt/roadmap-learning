package srsinfra

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	app "langapp/internal/application/srs"
)

// LUẬT GHI — cả file này và repository.go của roadmap đều tuân:
//
//  1. CẤM `db.Model(&X{}).Update(...)` với struct RỗNG. UPDATE đó không có
//     khoá chính nên GORM không sinh `WHERE id = ?`; với
//     `AllowGlobalUpdate: false` (đặt ở platform.OpenPostgres) GORM trả
//     `ErrMissingWhereClause` chứ không chạy câu lệnh — nhưng nếu ai đó bật
//     `AllowGlobalUpdate: true` thì câu UPDATE chạy trên TOÀN BỘ bảng và
//     trigger `langapp_touch_updated_at` gõ vào mọi row, làm merge LWW phía
//     peer chọn nhầm bản cũ là bản mới. Bắt buộc `db.Model(&x).Updates(...)`
//     theo INSTANCE (x đã load, có ID) — GORM tự thêm `WHERE id = ?`.
//  2. MỌI đọc lại sau khi ghi phải đi qua `txOf` — đọc bằng `r.db` (pool) sẽ
//     thấy dữ liệu CŨ vì transaction chưa commit (P0 cổng Oracle M2).
//  3. KHÔNG query lồng trong vòng `rows.Next()`. Với `MaxOpenConns = 1` (app
//     v1 từng dùng) thì conn đang giữ `rows` chưa trả về pool, query con kẹp
//     vĩnh viễn → deadlock. Pool Postgres của M2 rộng hơn nên pattern này
//     chạy được, nhưng vẫn SAI: mọi list ở đây thu thập hết row vào slice
//     trước, đóng rồi mới query tiếp.
//  4. Mọi list lọc `deleted = 0` thủ công (không dùng `gorm.DeletedAt` — xem
//     model.go).
//  5. Mọi ghi bọc trong UnitOfWork, kể cả ghi 1 dòng: để lỗi FK / CHECK lộ ra
//     trước khi service trả về cho client, và để hành vi 1 request = 1 giao
//     dịch.
//
// ── CẢNH BÁO CHO M7 (merge LWW) ──────────────────────────────────────────────
// `Omit("updated_at")` KHÔNG chỉ bỏ giá trị ta truyền vào — nó loại cột
// `updated_at` khỏi MỆNH LỆNH `SET` kể cả khi `map` có key `updated_at` tường
// minh (probe trên SQL thật: `SET "front"=$1` chứ không phải
// `SET "front"=$1,"updated_at"=$2`). Nhờ vậy trigger `langapp_touch_updated_at`
// luôn chạm, và nó CỐ Ý bỏ qua khi `NEW.updated_at IS NOT DISTINCT FROM
// OLD.updated_at`.
//
// Hệ quả bắt buộc: **M7 KHÔNG được tái dùng `UpdateCard` cho merge peer.**
// Merge phải có method `MergeCard` riêng KHÔNG Omit, ghi `updated_at` tường
// minh bằng mốc của peer, và có test assert mốc peer sống sót sau khi trigger
// chạy. Dùng nhầm sẽ ghi đè mốc peer bằng `now()` của máy nhận → LWW chọn
// nhầm bản cũ là bản mới.

// unitOfWork hiện thực app.UnitOfWork bằng `gorm.DB.Transaction`.
type unitOfWork struct{ db *gorm.DB }

// NewUnitOfWork dựng UnitOfWork trên pool GORM đã có.
func NewUnitOfWork(db *gorm.DB) app.UnitOfWork { return &unitOfWork{db: db} }

func (u *unitOfWork) Do(ctx context.Context, fn func(app.Tx) error) error {
	return u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(txHandle{tx: tx})
	})
}

// txHandle là hiện thực của app.Tx — mang *gorm.DB của transaction.
type txHandle struct{ tx *gorm.DB }

// errTxMismatch là lỗi trả về khi `app.Tx` không phải handle do package này tạo.
// Sự cố này chỉ xảy ra khi 2 context trộn handle — M3 (context `sync` cần ghi
// `decks/cards` + `roadmap_*` trong cùng 1 transaction) rất dễ lấy nhầm handle
// của context kia. Trước đây `txOf` IM LẶNG rơi về pool ⇒ ghi ra NGOÀI
// transaction, merge nửa vời không để lại dấu vết.
var errTxMismatch = errors.New("tx handle sai context — không phải do repository này tạo")

// txOf trả *gorm.DB đúng phạm vi:
//   - `tx == nil` → gọi ngoài transaction, dùng pool (hợp lệ cho unit test của
//     application khi `uow` không bind).
//   - handle hợp lệ → *gorm.DB của transaction.
//   - handle sai kiểu → LỖI, không rơi về pool.
func txOf(root *gorm.DB, tx app.Tx) (*gorm.DB, error) {
	if tx == nil {
		return root, nil
	}
	h, ok := tx.(txHandle)
	if !ok || h.tx == nil {
		return nil, errTxMismatch
	}
	return h.tx, nil
}

// txDB là viết ngắn cho `txOf(...).WithContext(ctx)` ở các method ghi.
func txDB(root *gorm.DB, ctx context.Context, tx app.Tx) (*gorm.DB, error) {
	db, err := txOf(root, tx)
	if err != nil {
		return nil, err
	}
	return db.WithContext(ctx), nil
}

// errNoRows chuẩn hoá lỗi "không tìm thấy" của GORM thành sentinel của
// application, để use case bọc thành 404 với message tiếng Việt.
func errNoRows(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return app.ErrNotFound
	}
	return err
}

// ── Repository ──────────────────────────────────────────────────────────────

// Repository hiện thực app.Repository. Giữ 1 con trỏ `*gorm.DB` làm nguồn
// duy nhất; mọi method nhận `app.Tx` để chọn phạm vi ghi.
type Repository struct {
	db *gorm.DB
}

// NewRepository dựng repository trên pool GORM.
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

var _ app.Repository = (*Repository)(nil)

// CreateDeck insert 1 deck. `guid` BẮT BUỘC khác rỗng: `ux_decks_guid` UNIQUE
// trên `NOT NULL DEFAULT ”` — 2 deck cùng guid rỗng là đụng nhau.
func (r *Repository) CreateDeck(ctx context.Context, tx app.Tx, d *app.Deck) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := deckFromApp(*d)
	if err := db.Create(&row).Error; err != nil {
		return errNoRows(err)
	}
	// GORM điền ID từ sequence; đọc lại TRONG tx (luật 2) để caller nhận đúng
	// row vừa ghi.
	if err := db.Where("id = ?", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*d = row.toApp()
	return nil
}

// ListDecks trả mọi deck còn sống, theo id.
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

// DeckByID trả 1 deck, app.ErrNotFound nếu đã xoá mềm.
func (r *Repository) DeckByID(ctx context.Context, id int64) (app.Deck, error) {
	var row Deck
	err := r.db.WithContext(ctx).Where("id = ? AND deleted = 0", id).Take(&row).Error
	if err != nil {
		return app.Deck{}, errNoRows(err)
	}
	return row.toApp(), nil
}

// SoftDeleteDeck đánh dấu xoá (tombstone cho sync) và xoá mềm luôn mọi thẻ
// của deck. Trigger chạm updated_at cả 2 bảng vì ta KHÔNG set updated_at.
//
// Cascade nằm ở repository chứ không ở use case: nó là 1 câu UPDATE duy nhất
// theo `deck_id`, đặt ở tầng trên sẽ phải biết cấu trúc bảng `cards`.
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

// CreateCard insert 1 thẻ. REUSE tombstone: nếu đã có thẻ cùng `front` bị xoá
// mềm thì hồi sinh chính row đó (giữ guid để 2 máy hội tụ) thay vì insert
// mới — `ux_cards_deck_front` là partial index chỉ trên `deleted = 0`, nên
// insert mới sẽ không đụng index nhưng sẽ tạo 2 row cùng front và peer merge
// nhầm là 1 thẻ.
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
		// Đọc lại trong CÙNG transaction để lấy updated_at do trigger chạm +
		// id/guid sau reuse.
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

// CardByID trả 1 thẻ, app.ErrNotFound nếu đã xoá mềm.
func (r *Repository) CardByID(ctx context.Context, id int64) (app.Card, error) {
	var row Card
	err := r.db.WithContext(ctx).Where("id = ? AND deleted = 0", id).Take(&row).Error
	if err != nil {
		return app.Card{}, errNoRows(err)
	}
	return row.toApp(), nil
}

// ListCards trả thẻ của deck theo id.
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

// UpdateCard ghi toàn bộ cột sửa được của 1 thẻ. `Updates` theo instance +
// Omit("updated_at") để trigger chạm (luật 1 ở đầu file).
// UpdateCard ghi toàn bộ cột sửa được của 1 thẻ, rồi làm mới `c` bằng row đọc
// lại TRONG CÙNG transaction (luật 2). Cột nullable (tone/ipa/stress/audio_url)
// là *string nên nil ghi SQL NULL thật — SetCardTone xoá được là nhờ đọc lại
// này xác nhận, chứ không phải GORM tự làm.
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

// SoftDeleteCard đánh dấu xoá.
func (r *Repository) SoftDeleteCard(ctx context.Context, tx app.Tx, id int64) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	return errNoRows(db.Model(&Card{ID: id}).Where("deleted = 0").
		UpdateColumn("deleted", 1).Error)
}

// CreateReview append 1 dòng lịch sử ôn. `guid` phải khác rỗng (xem
// CreateDeck) — đây là chỗ dễ sai nhất: 2 review liên tiếp cùng guid rỗng là
// đụng UNIQUE.
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
