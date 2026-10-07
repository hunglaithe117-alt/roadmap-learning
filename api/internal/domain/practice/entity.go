// Package practice provides domain models for speech practice, shadowing, and error logging.
package practice

import "time"

// Reserved note prefixes for practice domain records.
const (
	// ShadowPrefix identifies a card shadowing progress note.
	ShadowPrefix = "SHADOW|"
	// ErrorPrefix identifies an error entry note.
	ErrorPrefix = "ERR|"
)

// ShadowSession tracks shadowing repetition progress for a card.
type ShadowSession struct {
	CardID    int64
	Loops     int
	Rate      float64
	UpdatedAt time.Time
}

// Recording captures an audio practice attempt and its alignment results.
type Recording struct {
	CardID     int64
	Engine     string
	Transcript string
	Duration   float64
	Diff       []DiffToken
	Score      float64
	Wrong      []string
	CreatedAt  time.Time
}

// ErrorEntry represents a recorded mistake entry with expected and transcribed text.
type ErrorEntry struct {
	ID         int64
	CardID     *int64
	Expected   string
	Transcript string
	Wrong      []string
	CreatedAt  time.Time
}

