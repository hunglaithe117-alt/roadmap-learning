// Package practiceinfra là hiện thực GORM của bounded context practice. Đây là
// nơi DUY NHẤT trong context này được phép import gorm.io
// (STACK-V2-PLAN §2).
//
// Context này KHÔNG sở hữu bảng nào: nó ghi vào `notes` — bảng dùng chung với
// `content` (prefix THIEU|) qua khoá `guid`. Chỉ có 2 prefix là của practice:
// SHADOW| (tiến độ shadowing) và ERR| (sổ lỗi).
package practiceinfra

import (
	"context"
	"errors"

	"gorm.io/gorm"

	app "langapp/internal/application/practice"
	pdp "langapp/internal/domain/practice"
)

// LUẬT GHI — cùng bộ luật với infrastructure/srs và infrastructure/content:
//
//  1. CẤM `db.Model(&X{}).Update(...)` với struct RỖNG (xem giải thích ở
//     infrastructure/srs/repository.go).
//  2. KHÔNG query lồng trong vòng `rows.Next()`.
//  3. Mọi ghi bọc trong UnitOfWork.

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

// noteRow là row `notes`. KHÔNG có `updated_at`/`deleted`: đây là bảng
// append-only, merge bằng union theo guid chứ không LWW.
//
// `CreatedAt string` + `autoCreateTime:false` vì cột là TEXT RFC3339; field
// tên `CreatedAt` kiểu time.Time sẽ bị GORM tự ghi bằng giờ máy.
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

// Repository hiện thực app.Repository.
type Repository struct{ db *gorm.DB }

// NewRepository dựng repository trên pool GORM.
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

var _ app.Repository = (*Repository)(nil)

// AppendNote ghi 1 note và điền ID cho caller.
//
// `card_id` NULL là trạng thái hợp lệ (lỗi luyện tự do, đánh dấu đã xử lý).
// Truyền `&0` sẽ vi phạm FK — caller phải để `nil`.
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

// LatestShadowNote trả note SHADOW| mới nhất của 1 thẻ, (nil, nil) nếu chưa
// có. Chỉ lấy tối đa 1 dòng: `LatestShadowNote` bị gọi mỗi lần mở player,
// quét toàn bộ lịch sử chỉ để đọc dòng cuối là lãng phí I/O.
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

// ListErrorNotes trả note ERR| mới nhất trước.
//
// Lọc `text NOT LIKE 'THIEU|%'`: về mặt kỹ thuật thừa (note THIEU không bao
// giờ bắt đầu bằng `ERR|`) nhưng app v1 có điều kiện này và nó là hàng phòng
// thủ rẻ — 1 ngày nào đó ai đó đổi prefix là hỏng ngay thay vì âm thầm lẫn.
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

// CountErrorNotesByCard đếm lỗi theo thẻ trong 1 query JOIN duy nhất.
//
// 2 điều kiện phải có, cả hai đều từ hành vi v1:
//   - `c.deleted = 0` (INNER JOIN): thẻ đã xoá mềm tự rớt khỏi danh sách gợi
//     ý — gợi ý ôn 1 thẻ không tồn tại là vô nghĩa.
//   - `n.card_id IS NOT NULL`: lỗi luyện tự do không gắn thẻ nào, không gợi
//     ý ôn được.
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
