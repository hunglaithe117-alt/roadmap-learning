package srsinfra

import (
	"context"
	"fmt"

	app "langapp/internal/application/srs"
)

// DecksByIDs returns decks for multiple IDs in a single query.
func (r *Repository) DecksByIDs(ctx context.Context, ids []int64) ([]app.Deck, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []Deck
	err := r.db.WithContext(ctx).Where("id IN ? AND deleted = 0", ids).
		Order("id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list decks by ids: %w", err)
	}
	out := make([]app.Deck, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toApp())
	}
	return out, nil
}
