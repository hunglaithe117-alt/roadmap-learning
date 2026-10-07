// Package srs coordinates spaced repetition use cases: decks, cards, and review scheduling.
package srs

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Tx represents a database transaction handle.
type Tx = any

// UnitOfWork runs an operation inside a database transaction.
type UnitOfWork interface {
	Do(ctx context.Context, fn func(tx Tx) error) error
}

// Repository defines data access for SRS entities.
type Repository interface {
	CreateDeck(ctx context.Context, tx Tx, d *Deck) error
	ListDecks(ctx context.Context) ([]Deck, error)
	DeckByID(ctx context.Context, id int64) (Deck, error)
	DecksByIDs(ctx context.Context, ids []int64) ([]Deck, error)
	SoftDeleteDeck(ctx context.Context, tx Tx, id int64) error

	CreateCard(ctx context.Context, tx Tx, c *Card) error
	CardByID(ctx context.Context, id int64) (Card, error)
	ListCards(ctx context.Context, deckID int64) ([]Card, error)
	UpdateCard(ctx context.Context, tx Tx, c *Card) error
	SoftDeleteCard(ctx context.Context, tx Tx, id int64) error

	CreateReview(ctx context.Context, tx Tx, r *Review) error
}

// Deck represents a study deck.
type Deck struct {
	ID        int64
	Name      string
	Lang      string
	CreatedAt string
	GUID      string
	UpdatedAt string
	Deleted   int
}

// Card represents a study flashcard.
type Card struct {
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
	IPA        *string
	Stress     *string
	AudioURL   *string
	GUID       string
	UpdatedAt  string
	Deleted    int
}

// Review records an individual card review.
type Review struct {
	ID         int64
	CardID     int64
	Grade      int
	ReviewedAt string
	NextDueAt  string
	GUID       string
}

// NewGUID generates a new UUID v4 string.
func NewGUID() string { return uuid.NewString() }

// NowFunc provides current time, injected for deterministic testing.
type NowFunc func() time.Time

// Clock returns the current UTC time.
func Clock() time.Time { return time.Now().UTC() }
