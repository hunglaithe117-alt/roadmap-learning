package roadmap

import "time"

// StatusChange represents the outcome of a status transition.
type StatusChange struct {
	Status Status
	// CompletedAt is UTC RFC3339 timestamp string, or "" to preserve or clear.
	CompletedAt string
}

// StatusUpdate encapsulates input for SetStatus.
type StatusUpdate struct {
	Current Status
	Note    string
	Now     time.Time
}

// SetStatus computes the target Status and CompletedAt transition.
// Transitioning to Done records Now; transitioning away clears timestamp.
func SetStatus(up StatusUpdate, next Status) StatusChange {
	if !next.Valid() {
		return StatusChange{Status: up.Current}
	}
	change := StatusChange{Status: next}
	switch {
	case next == Done && up.Current != Done:
		change.CompletedAt = up.Now.UTC().Format(time.RFC3339)
	case next == Done && up.Current == Done:
		change.CompletedAt = ""
	case next != Done:
		change.CompletedAt = ""
	}
	return change
}

// CompletedSince counts required topics completed within [from, to).
func CompletedSince(topics []Topic, from, to time.Time) int {
	n := 0
	for _, t := range topics {
		if t.Deleted != 0 || t.IsOptional == Optional || t.CompletedAt == nil {
			continue
		}
		at, err := time.Parse(time.RFC3339, *t.CompletedAt)
		if err != nil {
			continue
		}
		if at.Before(from) {
			continue
		}
		if !to.IsZero() && !at.Before(to) {
			continue
		}
		n++
	}
	return n
}
