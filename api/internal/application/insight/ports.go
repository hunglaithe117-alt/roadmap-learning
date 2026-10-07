// Package insight provides application read models for dashboards, metrics, and progress reporting.
package insight

import (
	"context"
	"time"
)

// Repository defines read-only queries for insight metrics.
type Repository interface {
	CountReviewsSince(ctx context.Context, since string) (total, good int, err error)
	ReviewDays(ctx context.Context, limit int) ([]string, error)
	CountCardsAlive(ctx context.Context) (int, error)
	CountCardsDueNow(ctx context.Context, now string) (int, error)
	ListErrorNotes(ctx context.Context, limit int) ([]ErrorNote, error)
	CountTopicsCompletedSince(ctx context.Context, since string) (int, error)
}

// ErrorNote represents an error notebook note (prefix ERR|).
type ErrorNote struct {
	CardID *int64
	Front  string
	Wrong  []string
}

// NowFunc provides the current time for deterministic testing.
type NowFunc func() time.Time

// Clock returns the current UTC time.
func Clock() time.Time { return time.Now().UTC() }

// DefaultRange is the default dashboard time window.
const DefaultRange = "week"
