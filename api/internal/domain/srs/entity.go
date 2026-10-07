// Package srs provides domain entities and scheduling policies for spaced repetition.
package srs

import (
	"strings"
	"time"
)

// Supported language codes.
const (
	LangZH = "zh"
	LangEN = "en"
)

// Card review states.
const (
	StateNew    = "new"
	StateReview = "review"
)

// Deck represents a collection of review cards.
type Deck struct {
	ID        int64
	Name      string
	Lang      string
	CreatedAt string
	GUID      string
	UpdatedAt string
	Deleted   int
}

// Card represents a spaced repetition flashcard.
type Card struct {
	ID         int64
	DeckID     int64
	Front      string
	Back       string
	Pinyin     string
	DueAt      time.Time
	Stability  float64
	Difficulty float64
	Reps       int
	Lapses     int
	State      string
	CreatedAt  time.Time
	Tone       *string
	IPA        *string
	Stress     *string
	AudioURL   *string
	GUID       string
	UpdatedAt  time.Time
	Deleted    int
}

// DueNow reports whether a card is due for review based on due time and state.
func DueNow(dueAt time.Time, state string, deleted int, now time.Time) bool {
	if deleted != 0 {
		return false
	}
	if state == StateNew {
		return true
	}
	return !dueAt.After(now)
}

// IsDueNow reports whether the card is due for review at the specified time.
func (c Card) IsDueNow(now time.Time) bool {
	return DueNow(c.DueAt, c.State, c.Deleted, now)
}

// IsDeleted reports whether the card is soft-deleted.
func (c Card) IsDeleted() bool { return c.Deleted != 0 }

// Review records a single card grading event.
type Review struct {
	ID         int64
	CardID     int64
	Grade      Grade
	ReviewedAt time.Time
	NextDueAt  time.Time
	GUID       string
}

// NormalizeLang normalizes language aliases into canonical zh or en codes.
func NormalizeLang(s string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "zh", "zh-cn", "zh_cn", "cn":
		return LangZH, true
	case "en", "en-us", "en_us":
		return LangEN, true
	default:
		return "", false
	}
}

