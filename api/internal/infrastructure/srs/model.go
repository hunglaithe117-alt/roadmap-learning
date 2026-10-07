// Package srsinfra provides GORM models and repository implementations for SRS.
package srsinfra

import (
	app "langapp/internal/application/srs"
)

// Deck represents a database row in the decks table.
type Deck struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Name      string `gorm:"column:name"`
	Lang      string `gorm:"column:lang"`
	CreatedAt string `gorm:"column:created_at;autoCreateTime:false"`
	GUID      string `gorm:"column:guid"`
	UpdatedAt string `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted   int    `gorm:"column:deleted"`
}

// TableName returns the table name for Deck.
func (Deck) TableName() string { return "decks" }

func (d *Deck) toApp() app.Deck { return app.Deck(*d) }

func deckFromApp(d app.Deck) Deck { return Deck(d) }

// Card represents a database row in the cards table.
type Card struct {
	ID         int64   `gorm:"column:id;primaryKey;autoIncrement"`
	DeckID     int64   `gorm:"column:deck_id"`
	Front      string  `gorm:"column:front"`
	Back       string  `gorm:"column:back"`
	Pinyin     string  `gorm:"column:pinyin"`
	DueAt      string  `gorm:"column:due_at"`
	Stability  float64 `gorm:"column:stability"`
	Difficulty float64 `gorm:"column:difficulty"`
	Reps       int     `gorm:"column:reps"`
	Lapses     int     `gorm:"column:lapses"`
	State      string  `gorm:"column:state"`
	CreatedAt  string  `gorm:"column:created_at;autoCreateTime:false"`
	Tone       *string `gorm:"column:tone"`
	IPA        *string `gorm:"column:ipa"`
	Stress     *string `gorm:"column:stress"`
	AudioURL   *string `gorm:"column:audio_url"`
	GUID       string  `gorm:"column:guid"`
	UpdatedAt  string  `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted    int     `gorm:"column:deleted"`
}

// TableName returns the table name for Card.
func (Card) TableName() string { return "cards" }

func (c *Card) toApp() app.Card { return app.Card(*c) }

func cardFromApp(c app.Card) Card { return Card(c) }

// Review represents an append-only row in the reviews table.
type Review struct {
	ID         int64  `gorm:"column:id;primaryKey;autoIncrement"`
	CardID     int64  `gorm:"column:card_id"`
	Grade      int    `gorm:"column:grade"`
	ReviewedAt string `gorm:"column:reviewed_at;autoCreateTime:false"`
	NextDueAt  string `gorm:"column:next_due_at;autoCreateTime:false"`
	GUID       string `gorm:"column:guid"`
}

// TableName returns the table name for Review.
func (Review) TableName() string { return "reviews" }

func reviewFromApp(r app.Review) Review { return Review(r) }
