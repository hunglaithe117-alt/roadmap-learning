package contentinfra

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	app "langapp/internal/application/content"
)

// LUẬT GHI — cùng bộ luật với infrastructure/srs và infrastructure/roadmap:
//
//  1. CẤM `db.Model(&X{}).Update(...)` với struct RỖNG. UPDATE không có khoá
//     chính nên GORM không sinh `WHERE id = ?`; với `AllowGlobalUpdate: false`
//     (đặt ở platform.OpenPostgres) GORM trả `ErrMissingWhereClause` chứ không
//     chạy lệnh — nhưng nếu ai đó bật `AllowGlobalUpdate: true` thì câu UPDATE
//     chạy trên TOÀN BỘ bảng và trigger `langapp_touch_updated_at` gõ vào mọi
//     row, làm merge LWW phía peer chọn nhầm bản cũ là bản mới.
//  2. KHÔNG query lồng trong vòng `rows.Next()` — conn đang giữ `rows` chưa
//     trả về pool sẽ kẹp vĩnh viễn. Mọi list thu thập hết row vào slice
//     trước, đóng rồi mới query tiếp.
//  3. Mọi ghi bọc trong UnitOfWork, kể cả ghi 1 dòng.

// unitOfWork hiện thực app.UnitOfWork bằng `gorm.DB.Transaction`.
type unitOfWork struct{ db *gorm.DB }

// NewUnitOfWork dựng UnitOfWork trên pool GORM đã có.
func NewUnitOfWork(db *gorm.DB) app.UnitOfWork { return &unitOfWork{db: db} }

func (u *unitOfWork) Do(ctx context.Context, fn func(app.Tx) error) error {
	return u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(txHandle{tx: tx})
	})
}

// txHandle là hiện thực của app.Tx — struct{} marker + con trỏ GORM đi kèm.
type txHandle struct{ tx *gorm.DB }

// errTxMismatch là lỗi trả về khi `app.Tx` không phải handle do package này tạo.
var errTxMismatch = errors.New("tx handle sai context — không phải do repository này tạo")

// txOf trả *gorm.DB đúng phạm vi:
//   - `tx == nil` → gọi ngoài transaction, dùng pool (hợp lệ cho unit test của
//     application khi `uow` không bind).
//   - handle hợp lệ → *gorm.DB của transaction.
//   - handle sai kiểu → LỖI, KHÔNG rơi về pool.
//
// Vì sao phải lỗi chứ không rơi về pool (finding H4 của cổng Oracle M3): im
// lặng rơi về pool biến 1 lỗi lập trình thành **ghi ra NGOÀI transaction** —
// rollback không ăn, và test vẫn xanh vì dữ liệu "vẫn tới nơi". Handle lấy
// nhầm từ context khác là rất dễ xảy ra khi M3 cho 7 context cùng dùng kiểu
// `app.Tx` trống. Xem `infrastructure/srs/repository.go` — chỗ đầu tiên
// chuyển sang luật này.
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

// txCtx là `txOf` + `WithContext(ctx)` — viết ngắn cho mọi method nhận cả hai.
func txCtx(root *gorm.DB, ctx context.Context, tx app.Tx) (*gorm.DB, error) {
	db, err := txOf(root, tx)
	if err != nil {
		return nil, err
	}
	return db.WithContext(ctx), nil
}

// Repository hiện thực app.Repository.
type Repository struct{ db *gorm.DB }

// NewRepository dựng repository trên pool GORM.
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

var _ app.Repository = (*Repository)(nil)

// SearchZH tra dict qua HÀM SQL `dict_search` — không viết lại truy vấn ở đây.
//
// Hàm đó (migrations/00002_fts.sql) đã lo hết phần khó: `websearch_to_tsquery`
// chịu được cú pháp người dùng gõ thẳng (không ném lỗi như `to_tsquery`),
// `ILIKE` làm lưới vớ cho CJK 1-N ký tự (vì `to_tsvector('simple', '你好')`
// gộp cả chuỗi Hán làm MỘT token), và `regexp_replace` khử ký tự wildcard
// `%_\` của input để "1 ký tự" không khớp toàn bảng (finding F5 của M1).
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

// SearchEN tra en_dict qua hàm SQL `en_dict_search` (xem SearchZH).
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

// LookupEN tra CHÍNH XÁC 1 headword.
//
// `lower(term) = lower(?)` là dạng duy nhất dùng được index
// `idx_en_dict_term_lower` (đã tạo ở migration 00002). KHÔNG dùng
// `COLLATE NOCASE` như api/english.go:274 v1 — đó là cú pháp SQLite và **lỗi
// cú pháp trong Postgres**. File v1 không sửa ở M3 vì app cũ vẫn chạy SQLite.
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

// DictHanziSet trả tập chữ Hán đã có trong dict. Import HSK dùng để không
// chèn lại trong cùng 1 lần (và báo `dict_added` trung thực).
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

// InsertDict chèn 1 dòng dict; trả false nếu chữ đã có (import là idempotent
// theo chữ Hán — chứ không theo level, vì tra từ điển là tra theo chữ).
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

// CountDict đếm số dòng dict.
func (r *Repository) CountDict(ctx context.Context) (int, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&Dict{}).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("đếm dict: %w", err)
	}
	return int(n), nil
}

// InsertEN chèn 1 dòng en_dict (seed tích hợp).
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

// CountEN đếm số dòng en_dict.
func (r *Repository) CountEN(ctx context.Context) (int, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&EnDict{}).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("đếm en_dict: %w", err)
	}
	return int(n), nil
}

// InsertNote ghi 1 note (prefix THIEU| của context content).
//
// `card_id` NULL là trạng thái hợp lệ: checklist THIEU không gắn thẻ. Truyền
// `nil` trong `Note.CardID` sẽ ghi SQL NULL thật — dùng `&0` sẽ vi phạm FK.
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

// ListNotesByPrefix trả note có prefix, MỚI NHẤT TRƯỚC.
//
// `text LIKE 'THIEU|%'` — ký tự `%` phía sau là LIKE, nhưng chuỗi `THIEU|`
// trước đó được truyền qua bind parameter nên không cần escape. Cột `text`
// không có index riêng (xem migration 00005 cho index phục vụ truy vấn này).
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
