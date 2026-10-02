package srs

import (
	"sort"
	"time"
)

// DueFilter lọc thẻ đến hạn cho hàng đợi ôn. V1 dùng SQL
// `WHERE deck_id = ? AND deleted = 0 AND (due_at <= ? OR state = 'new')`
// — điều kiện OR (không có cờ include_new) vì CreateCard gán due_at = +24h nên
// thẻ mới sẽ vô hình với hàng đợi nếu lọc chỉ theo due_at.
type DueFilter struct {
	Now    time.Time
	DeckID int64 // 0 = không lọc theo deck
}

// NewDueFilter đóng băng mốc thời gian 1 lần cho cả 1 request, tránh mỗi
// thẻ so sánh với "bây giờ" khác nhau.
func NewDueFilter(now time.Time, deckID int64) DueFilter {
	return DueFilter{Now: now.UTC(), DeckID: deckID}
}

// Keep áp điều kiện hẹn giờ lên 1 thẻ.
func (f DueFilter) Keep(c Card) bool {
	if c.IsDeleted() {
		return false
	}
	if f.DeckID != 0 && c.DeckID != f.DeckID {
		return false
	}
	return c.IsDueNow(f.Now)
}

// Apply trả về thẻ đến hạn, sắp theo (due_at, id) — thứ tự của `ORDER BY
// due_at, id` ở v1: thẻ quá hạn lâu nhất lên trước, thẻ mới có cùng due_at thì
// theo thứ tự tạo. Không mutate input.
func (f DueFilter) Apply(cards []Card) []Card {
	out := make([]Card, 0, len(cards))
	for _, c := range cards {
		if f.Keep(c) {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].DueAt.Equal(out[j].DueAt) {
			return out[i].DueAt.Before(out[j].DueAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}
