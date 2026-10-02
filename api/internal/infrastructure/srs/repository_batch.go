package srsinfra

import (
	"context"
	"fmt"

	app "langapp/internal/application/srs"
)

// DecksByIDs trả deck theo nhiều id trong 1 statement — batch read cho dataloader
// `Stage.deck` của cây roadmap (M4).
//
// Vì sao cần: `roadmap.Service.GetPath` gọi `DeckReader.Find` 1 lần cho mỗi
// stage có `deck_id`. Với 10 stage đó là 10 statement; dataloader gom hết key
// của request lại thành 1, nên số statement hằng theo số path trong query.
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
