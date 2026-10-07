// Package practice coordinates speaking practice: shadowing progress,
// audio transcription/diffing, and error notebooks.
package practice

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

// Repository defines data access for notes in practice context.
type Repository interface {
	AppendNote(ctx context.Context, tx Tx, n *Note) error
	LatestShadowNote(ctx context.Context, cardID int64) (*Note, error)
	ListErrorNotes(ctx context.Context, cardID *int64, limit int) ([]Note, error)
	CountErrorNotesByCard(ctx context.Context, limit int) ([]CardErrorCount, error)
}

// STTPort defines speech-to-text transcription.
type STTPort interface {
	Transcribe(ctx context.Context, audio []byte, filename, contentType string) (Transcript, error)
}

// TTSPort defines text-to-speech synthesis for sample sentences.
type TTSPort interface {
	Synthesize(ctx context.Context, text, lang string) ([]byte, string, error)
}

// Transcript holds transcribed text and detected language.
type Transcript struct {
	Text string
	Lang string
}

// Note represents an entry in the notes table.
type Note struct {
	ID        int64
	CardID    *int64
	Text      string
	CreatedAt string
	GUID      string
}

// CardErrorCount aggregates error occurrences for a card.
type CardErrorCount struct {
	CardID int64
	Front  string
	Back   string
	Errors int
}

// NowFunc provides current time, injected for deterministic tests.
type NowFunc func() time.Time

// Clock returns the current UTC time.
func Clock() time.Time { return time.Now().UTC() }

// NewGUID generates a new UUID v4 string.
func NewGUID() string { return uuid.NewString() }
