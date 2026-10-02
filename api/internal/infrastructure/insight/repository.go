// Package insightinfra là hiện thực GORM READ-ONLY của bounded context
// insight. Đây là nơi DUY NHẤT trong context này được phép import gorm.io
// (STACK-V2-PLAN §2).
//
// Package này KHÔNG có UnitOfWork và KHÔNG có method ghi: insight không sở hữu
// bảng nào. Việc chỉ cần SELECT nên đọc ngoài transaction vẫn đúng và bọc vào
// transaction chỉ tốn connection.
package insightinfra

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	app "langapp/internal/application/insight"
	pdp "langapp/internal/domain/practice"
)

// Repository hiện thực app.Repository.
type Repository struct{ db *gorm.DB }

// NewRepository dựng repository trên pool GORM.
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

var _ app.Repository = (*Repository)(nil)

// CountReviewsSince đếm review trong cửa sổ + số lần chấm đạt (grade >= 3).
//
// `COALESCE(SUM(...), 0)` bắt buộc: cửa sổ rỗng trả SUM = NULL, quét vào
// `int` sẽ panic ("converting NULL to int is unsupported").
//
// GRADE_GOOD = 3 trên thang 1-4 đã đóng băng với UI (xem domain/srs.Grade).
func (r *Repository) CountReviewsSince(ctx context.Context, since string) (int, int, error) {
	var total, good int64
	err := r.db.WithContext(ctx).Raw(`
		SELECT COUNT(*), COALESCE(SUM(CASE WHEN grade >= 3 THEN 1 ELSE 0 END), 0)
		FROM reviews WHERE reviewed_at >= ?`, since).Row().Scan(&total, &good)
	if err != nil {
		return 0, 0, fmt.Errorf("đếm review từ %s: %w", since, err)
	}
	return int(total), int(good), nil
}

// ReviewDays trả ngày UTC có review, mới nhất trước.
//
// `substr(reviewed_at, 1, 10)` cắt RFC3339 thành `YYYY-MM-DD` — cùng cách
// `computeStreak` v1 làm, và là cách duy nhất đúng vì cột là TEXT chứ không
// phải TIMESTAMP (đổi sang `reviewed_at::date` thì Postgres vẫn đúng nhưng
// phá index expression nếu sau này thêm index theo ngày).
func (r *Repository) ReviewDays(ctx context.Context, limit int) ([]string, error) {
	var days []string
	if err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT substr(reviewed_at, 1, 10) AS d
		FROM reviews ORDER BY d DESC LIMIT ?`, limit).Scan(&days).Error; err != nil {
		return nil, fmt.Errorf("đọc ngày có review: %w", err)
	}
	return days, nil
}

// CountCardsAlive đếm thẻ còn sống — `deleted` là tombstone INTEGER, KHÔNG
// dùng `gorm.DeletedAt` (xem STACK-V2-PLAN §4.4).
func (r *Repository) CountCardsAlive(ctx context.Context) (int, error) {
	var n int64
	if err := r.db.WithContext(ctx).Raw(
		"SELECT COUNT(*) FROM cards WHERE deleted = 0").Scan(&n).Error; err != nil {
		return 0, fmt.Errorf("đếm thẻ: %w", err)
	}
	return int(n), nil
}

// CountCardsDueNow đếm thẻ đã tới hạn. KHÔNG kéo `state = 'new'` — khác hẳn
// hàng đợi ôn, xem comment ở use case `Stats`.
func (r *Repository) CountCardsDueNow(ctx context.Context, now string) (int, error) {
	var n int64
	if err := r.db.WithContext(ctx).Raw(
		"SELECT COUNT(*) FROM cards WHERE deleted = 0 AND due_at <= ?", now).Scan(&n).Error; err != nil {
		return 0, fmt.Errorf("đếm thẻ đến hạn: %w", err)
	}
	return int(n), nil
}

// ListErrorNotes trả các note ERR| mới nhất kèm thẻ và mặt trước của thẻ.
//
// `LEFT JOIN cards` với điều kiện `c.deleted = 0` trong ON (không phải WHERE):
// thẻ đã xoá mềm làm `front` rỗng nhưng note vẫn phải được tính vào số lần sai.
// Note hỏng (JSON không parse được) bị bỏ qua thay vì làm hỏng cả danh sách.
// Note dạng "đánh dấu đã xử lý" không có mảng `wrong` nên trả nil — vẫn được
// tính 1 note trong LIMIT.
//
// `CardID` trả về NULL khi thẻ không còn sống: `cardId` trong GraphQL là lời
// hứa "bấm là nhảy tới thẻ này", và thẻ đã xoá mềm thì không nhảy được. Trả id
// vẫn còn lưu sẽ khiến UI có 1 nút chết. Note vẫn được đếm như cũ — chỉ mất
// đường nhảy.
func (r *Repository) ListErrorNotes(ctx context.Context, limit int) ([]app.ErrorNote, error) {
	var rows []struct {
		CardID *int64
		Front  string
		Text   string
	}
	if err := r.db.WithContext(ctx).Raw(`
		SELECT n.card_id, c.front, n.text FROM notes n
		LEFT JOIN cards c ON c.id = n.card_id AND c.deleted = 0
		WHERE n.text LIKE ? AND n.text NOT LIKE ?
		ORDER BY n.id DESC LIMIT ?`, pdp.ErrorPrefix+"%", "THIEU|%", limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc note lỗi: %w", err)
	}
	out := make([]app.ErrorNote, 0, len(rows))
	for _, row := range rows {
		var payload struct {
			Wrong []string `json:"wrong"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(row.Text, pdp.ErrorPrefix)), &payload); err != nil {
			continue
		}
		cardID, front := row.CardID, row.Front
		if front == "" {
			cardID, front = nil, ""
		}
		out = append(out, app.ErrorNote{CardID: cardID, Front: front, Wrong: payload.Wrong})
	}
	return out, nil
}

// CountTopicsCompletedSince đếm node roadmap xong từ mốc.
//
// Lọc `deleted = 0`: node đã xoá mềm không tính vào tiến độ dù còn mốc
// `completed_at` cũ.
func (r *Repository) CountTopicsCompletedSince(ctx context.Context, since string) (int, error) {
	var n int64
	err := r.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM roadmap_topics
		WHERE deleted = 0 AND completed_at IS NOT NULL AND completed_at >= ?`, since).Scan(&n).Error
	if err != nil {
		return 0, fmt.Errorf("đếm node hoàn thành: %w", err)
	}
	return int(n), nil
}
