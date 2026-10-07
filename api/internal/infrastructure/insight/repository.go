// Package insightinfra provides the read-only GORM implementation for the insight bounded context.
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

// Repository implements app.Repository.
type Repository struct{ db *gorm.DB }

// NewRepository constructs a new insight Repository on a GORM DB.
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

var _ app.Repository = (*Repository)(nil)

// CountReviewsSince counts reviews and passing reviews (grade >= 3) since the given timestamp.
func (r *Repository) CountReviewsSince(ctx context.Context, since string) (int, int, error) {
	var total, good int64
	err := r.db.WithContext(ctx).Raw(`
		SELECT COUNT(*), COALESCE(SUM(CASE WHEN grade >= 3 THEN 1 ELSE 0 END), 0)
		FROM reviews WHERE reviewed_at >= ?`, since).Row().Scan(&total, &good)
	if err != nil {
		return 0, 0, fmt.Errorf("count reviews since %s: %w", since, err)
	}
	return int(total), int(good), nil
}

// ReviewDays returns distinct UTC review dates in descending order.
func (r *Repository) ReviewDays(ctx context.Context, limit int) ([]string, error) {
	var days []string
	if err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT substr(reviewed_at, 1, 10) AS d
		FROM reviews ORDER BY d DESC LIMIT ?`, limit).Scan(&days).Error; err != nil {
		return nil, fmt.Errorf("fetch review days: %w", err)
	}
	return days, nil
}

// CountCardsAlive counts non-deleted cards.
func (r *Repository) CountCardsAlive(ctx context.Context) (int, error) {
	var n int64
	if err := r.db.WithContext(ctx).Raw(
		"SELECT COUNT(*) FROM cards WHERE deleted = 0").Scan(&n).Error; err != nil {
		return 0, fmt.Errorf("count alive cards: %w", err)
	}
	return int(n), nil
}

// CountCardsDueNow counts active cards currently due.
func (r *Repository) CountCardsDueNow(ctx context.Context, now string) (int, error) {
	var n int64
	if err := r.db.WithContext(ctx).Raw(
		"SELECT COUNT(*) FROM cards WHERE deleted = 0 AND due_at <= ?", now).Scan(&n).Error; err != nil {
		return 0, fmt.Errorf("count due cards: %w", err)
	}
	return int(n), nil
}

// ListErrorNotes lists recent error notes with associated card fronts.
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
		return nil, fmt.Errorf("list error notes: %w", err)
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

// CountTopicsCompletedSince counts completed roadmap topics since the given timestamp.
func (r *Repository) CountTopicsCompletedSince(ctx context.Context, since string) (int, error) {
	var n int64
	err := r.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM roadmap_topics
		WHERE deleted = 0 AND completed_at IS NOT NULL AND completed_at >= ?`, since).Scan(&n).Error
	if err != nil {
		return 0, fmt.Errorf("count completed topics: %w", err)
	}
	return int(n), nil
}
