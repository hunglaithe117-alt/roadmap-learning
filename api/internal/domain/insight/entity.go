// Package insight provides reporting and statistics aggregated across domains.
package insight

import (
	"sort"
	"time"

	srs "langapp/internal/domain/srs"
)

// Range defines a reporting time window.
type Range string

const (
	// RangeWeek defines a 7-day reporting window.
	RangeWeek Range = "week"
	// RangeMonth defines a 30-day reporting window.
	RangeMonth Range = "month"
)

// Days returns the window duration in days and whether the range is valid.
func (r Range) Days() (int, bool) {
	switch r {
	case RangeWeek, "":
		return 7, true
	case RangeMonth:
		return 30, true
	default:
		return 0, false
	}
}

// Valid reports whether the range value is supported.
func (r Range) Valid() bool {
	_, ok := r.Days()
	return ok
}

// Stats holds aggregate dashboard statistics.
type Stats struct {
	Range      Range
	Days       int
	DoneWindow int
	TotalAll   int
	DueNow     int
	Accuracy   float64
	Streak     int
	Timezone   string
}

// ReviewCount aggregates total and successful review counts within a time window.
type ReviewCount struct {
	Total int
	Good  int
}

// ComputeAccuracy calculates the ratio of successful reviews (grade >= 3).
func ComputeAccuracy(rc ReviewCount) float64 {
	if rc.Total <= 0 {
		return 0
	}
	return float64(rc.Good) / float64(rc.Total)
}

// DueNow reports whether a card is currently due for review.
func DueNow(cardDueAt time.Time, state string, deleted int, now time.Time) bool {
	return srs.DueNow(cardDueAt, state, deleted, now)
}

// ComputeStreak calculates consecutive UTC review days leading up to today.
func ComputeStreak(days map[string]bool, now time.Time) int {
	cursor := now.UTC().Truncate(24 * time.Hour)
	key := cursor.Format("2006-01-02")
	if !days[key] {
		cursor = cursor.Add(-24 * time.Hour)
		key = cursor.Format("2006-01-02")
		if !days[key] {
			return 0
		}
	}
	streak := 0
	for days[key] {
		streak++
		cursor = cursor.Add(-24 * time.Hour)
		key = cursor.Format("2006-01-02")
	}
	return streak
}

// ErrorCount associates an erroneous word with its occurrence count and representative card ID.
type ErrorCount struct {
	Word   string
	Count  int
	CardID *int64
}

// TopError aggregates and ranks the most frequent error words.
func TopError(entries []ErrorEntry, limit int) []ErrorCount {
	if limit <= 0 {
		return nil
	}
	counts := map[string]int{}
	cards := map[string]int64{}
	for _, e := range entries {
		for _, w := range e.NormalizedWrong() {
			counts[w]++
			if _, seen := cards[w]; !seen && e.CardID != nil {
				cards[w] = *e.CardID
			}
		}
	}
	out := make([]ErrorCount, 0, len(counts))
	for w, c := range counts {
		row := ErrorCount{Word: w, Count: c}
		if id, ok := cards[w]; ok {
			row.CardID = &id
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Word < out[j].Word
		}
		return out[i].Count > out[j].Count
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// ErrorEntry represents practice errors recorded for an optional card ID.
type ErrorEntry struct {
	Wrong  []string
	CardID *int64
}

// NormalizedWrong returns trimmed lowercase error words, filtering out empty entries.
func (e ErrorEntry) NormalizedWrong() []string {
	out := make([]string, 0, len(e.Wrong))
	for _, w := range e.Wrong {
		w = normalize(w)
		if w == "" {
			continue
		}
		out = append(out, w)
	}
	return out
}

