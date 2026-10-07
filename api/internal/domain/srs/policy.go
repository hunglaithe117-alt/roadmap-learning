package srs

import (
	"math"
	"time"
)

const (
	minReviewsForFSRS = 3
	maxIntervalDays   = 365
)

var fallbackIntervals = []int{1, 3, 7, 14, 30}

// ScheduleResult represents the outcome of an SRS review calculation.
type ScheduleResult struct {
	DueAt        time.Time
	IntervalDays int
	Stability    float64
	Difficulty   float64
	Lapse        bool
	FallbackUsed bool
}

// ScheduleNext computes the next review date and updated stability/difficulty metrics.
func ScheduleNext(reps int, stability, difficulty float64, grade Grade, now time.Time) ScheduleResult {
	now = now.UTC()
	if grade == GradeAgain {
		stability = clamp(stability*stabilityFactor(GradeAgain), 0.1, maxIntervalDays)
		if reps < minReviewsForFSRS || stability < 1 {
			stability = initStability(GradeAgain)
		}
		if difficulty <= 0 {
			difficulty = initDifficulty(GradeAgain)
		} else {
			difficulty = clamp(difficulty+0.3, 1, 10)
		}
		return ScheduleResult{
			DueAt:        now.Add(24 * time.Hour),
			IntervalDays: 1,
			Stability:    stability,
			Difficulty:   difficulty,
			Lapse:        true,
			FallbackUsed: true,
		}
	}
	if reps < minReviewsForFSRS {
		if reps < 0 {
			reps = 0
		}
		days := fallbackIntervals[len(fallbackIntervals)-1]
		if reps < len(fallbackIntervals) {
			days = fallbackIntervals[reps]
		}
		return ScheduleResult{
			DueAt:        now.Add(time.Duration(days) * 24 * time.Hour),
			IntervalDays: days,
			Stability:    stability,
			Difficulty:   difficulty,
			FallbackUsed: true,
		}
	}
	if stability <= 0 {
		stability = initStability(grade)
		difficulty = initDifficulty(grade)
	} else {
		stability = clamp(stability*stabilityFactor(grade), 0.1, maxIntervalDays)
		difficulty = clamp(difficulty-0.2*float64(grade-3), 1, 10)
	}
	days := int(math.Round(stability))
	if days < 1 {
		days = 1
	}
	if days > maxIntervalDays {
		days = maxIntervalDays
	}
	return ScheduleResult{
		DueAt:        now.Add(time.Duration(days) * 24 * time.Hour),
		IntervalDays: days,
		Stability:    stability,
		Difficulty:   difficulty,
	}
}

// Replay recalculates reps, lapses, stability, difficulty, and due date from historical reviews.
func Replay(reviews []Review) (reps, lapses int, stability, difficulty float64, due time.Time) {
	for _, r := range reviews {
		res := ScheduleNext(reps, stability, difficulty, r.Grade, r.ReviewedAt)
		stability, difficulty, due = res.Stability, res.Difficulty, res.DueAt
		if res.Lapse {
			lapses++
		}
		reps++
	}
	return reps, lapses, stability, difficulty, due
}

func initDifficulty(grade Grade) float64 {
	switch grade {
	case GradeAgain:
		return 7.0
	case GradeHard:
		return 5.5
	case GradeGood:
		return 4.0
	default:
		return 2.5
	}
}

func initStability(grade Grade) float64 {
	switch grade {
	case GradeAgain:
		return 0.5
	case GradeHard:
		return 1.0
	case GradeGood:
		return 2.0
	default:
		return 4.0
	}
}

func stabilityFactor(grade Grade) float64 {

	switch grade {
	case GradeAgain:
		return 0.3
	case GradeHard:
		return 1.2
	case GradeGood:
		return 1.6
	default:
		return 2.1
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
