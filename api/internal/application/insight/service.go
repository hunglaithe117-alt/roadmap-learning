package insight

import (
	"context"
	"fmt"
	"time"

	domain "langapp/internal/domain/insight"
)

// Service handles dashboard analytics and progress reporting.
type Service struct {
	repo Repository
	now  NowFunc
}

// NewService constructs an insight service.
func NewService(repo Repository, nowFn NowFunc) *Service {
	if nowFn == nil {
		nowFn = Clock
	}
	return &Service{repo: repo, now: nowFn}
}

// Stats represents dashboard review and card statistics.
type Stats struct {
	Range      string
	Days       int
	Done       int
	Total      int
	DoneWindow int
	TotalAll   int
	DueNow     int
	Accuracy   float64
	Streak     int
	Timezone   string
}

// StreakScanLimit is the maximum number of review days scanned for streak calculation.
const StreakScanLimit = 400

// Stats computes dashboard statistics for the specified range ("week" or "month").
func (s *Service) Stats(ctx context.Context, rangeName string) (Stats, error) {
	r := domain.Range(rangeName)
	if rangeName == "" {
		r = domain.RangeWeek
	}
	if !r.Valid() {
		return Stats{}, newError(StatusBadRequest, "range chỉ nhận week hoặc month")
	}
	days, _ := r.Days()
	now := s.now().UTC()
	since := now.Add(-time.Duration(days) * 24 * time.Hour).Format(time.RFC3339)

	total, good, err := s.repo.CountReviewsSince(ctx, since)
	if err != nil {
		return Stats{}, fmt.Errorf("đọc lịch sử ôn: %w", err)
	}
	all, err := s.repo.CountCardsAlive(ctx)
	if err != nil {
		return Stats{}, fmt.Errorf("đọc số thẻ: %w", err)
	}
	due, err := s.repo.CountCardsDueNow(ctx, now.Format(time.RFC3339))
	if err != nil {
		return Stats{}, fmt.Errorf("đọc số thẻ đến hạn: %w", err)
	}
	streak, err := s.ComputeStreak(ctx, now)
	if err != nil {
		return Stats{}, err
	}
	return Stats{
		Range: string(r), Days: days,
		Done: total, Total: all,
		DoneWindow: total, TotalAll: all,
		DueNow:   due,
		Accuracy: domain.ComputeAccuracy(domain.ReviewCount{Total: total, Good: good}),
		Streak:   streak,
		Timezone: "UTC",
	}, nil
}

// ComputeStreak computes consecutive review days up to now in UTC.
func (s *Service) ComputeStreak(ctx context.Context, now time.Time) (int, error) {
	list, err := s.repo.ReviewDays(ctx, StreakScanLimit)
	if err != nil {
		return 0, fmt.Errorf("đọc ngày có review: %w", err)
	}
	days := make(map[string]bool, len(list))
	for _, d := range list {
		days[d] = true
	}
	return domain.ComputeStreak(days, now), nil
}

// TopError represents a frequently missed word.
type TopError struct {
	Word   string
	Count  int
	CardID *int64
	Front  string
}

// TopErrors aggregates the most frequent errors from the error log.
func (s *Service) TopErrors(ctx context.Context, limit int) ([]TopError, error) {
	raw, err := s.repo.ListErrorNotes(ctx, scanLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("đọc sổ lỗi: %w", err)
	}
	entries := make([]domain.ErrorEntry, 0, len(raw))
	fronts := make(map[int64]string, len(raw))
	for _, note := range raw {
		entries = append(entries, domain.ErrorEntry{Wrong: note.Wrong, CardID: note.CardID})
		if note.CardID != nil {
			fronts[*note.CardID] = note.Front
		}
	}
	counts := domain.TopError(entries, limit)
	out := make([]TopError, 0, len(counts))
	for _, c := range counts {
		row := TopError{Word: c.Word, Count: c.Count, CardID: c.CardID}
		if c.CardID != nil {
			row.Front = fronts[*c.CardID]
		}
		out = append(out, row)
	}
	return out, nil
}

const topErrorScanLimit = 500

func scanLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	return topErrorScanLimit
}

// ProgressResult holds the number of completed roadmap topics since a date.
type ProgressResult struct {
	Since     string
	Completed int
}

// ProgressOverRange counts completed roadmap topics since date string (YYYY-MM-DD).
func (s *Service) ProgressOverRange(ctx context.Context, since string) (ProgressResult, error) {
	day, err := time.Parse("2006-01-02", since)
	if err != nil {
		return ProgressResult{}, newError(StatusBadRequest,
			"since phải là ngày YYYY-MM-DD")
	}
	start := day.UTC().Format(time.RFC3339)
	n, err := s.repo.CountTopicsCompletedSince(ctx, start)
	if err != nil {
		return ProgressResult{}, fmt.Errorf("đếm node hoàn thành: %w", err)
	}
	return ProgressResult{Since: day.Format("2006-01-02"), Completed: n}, nil
}
