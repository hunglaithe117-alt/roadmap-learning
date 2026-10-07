package practice

import (
	"errors"
	"time"
)

// Playback rate bounds for audio looping.
const (
	MinRate     = 0.5
	MaxRate     = 1.5
	DefaultRate = 1.0
)

// ErrRateOutOfRange indicates the playback rate is outside [0.5, 1.5].
var ErrRateOutOfRange = errors.New("rate must be between 0.5 and 1.5")

// NormalizeRate validates and defaults playback rate.
func NormalizeRate(rate float64) (float64, error) {
	if rate == 0 {
		return DefaultRate, nil
	}
	if rate < MinRate || rate > MaxRate {
		return 0, ErrRateOutOfRange
	}
	return rate, nil
}

// ErrNegativeLoops indicates a negative loop count.
var ErrNegativeLoops = errors.New("loops cannot be negative")

// ValidateLoops validates the loop count.
func ValidateLoops(loops int) error {
	if loops < 0 {
		return ErrNegativeLoops
	}
	return nil
}

// AdvanceShadowSession increments the loop count and updates the timestamp.
func AdvanceShadowSession(cur ShadowSession, rate float64, now time.Time) (ShadowSession, error) {
	if err := ValidateLoops(cur.Loops + 1); err != nil {
		return ShadowSession{}, err
	}
	normalized, err := NormalizeRate(rate)
	if err != nil {
		return ShadowSession{}, err
	}
	return ShadowSession{
		CardID:    cur.CardID,
		Loops:     cur.Loops + 1,
		Rate:      normalized,
		UpdatedAt: now.UTC(),
	}, nil
}

// NewShadowSession initializes a new shadowing session for a card.
func NewShadowSession(cardID int64) ShadowSession {
	return ShadowSession{CardID: cardID, Loops: 0, Rate: DefaultRate}
}

