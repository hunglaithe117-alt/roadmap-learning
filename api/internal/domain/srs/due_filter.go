package srs

import (
	"sort"
	"time"
)

// DueFilter filters and orders due cards for review.
type DueFilter struct {
	Now    time.Time
	DeckID int64 // 0 matches all decks
}

// NewDueFilter creates a DueFilter freezing the current evaluation time.
func NewDueFilter(now time.Time, deckID int64) DueFilter {
	return DueFilter{Now: now.UTC(), DeckID: deckID}
}

// Keep reports whether a card matches the due review criteria.
func (f DueFilter) Keep(c Card) bool {
	if c.IsDeleted() {
		return false
	}
	if f.DeckID != 0 && c.DeckID != f.DeckID {
		return false
	}
	return c.IsDueNow(f.Now)
}

// Apply filters and returns due cards sorted by DueAt then ID.
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

