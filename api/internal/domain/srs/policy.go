package srs

import (
	"math"
	"time"
)

// Hằng số engine — port nguyên vẹn từ api/srs.go v1.
const (
	// minReviewsForFSRS: dưới ngưỡng này dùng chuỗi lịch 1-3-7-14-30 thay vì
	// FSRS-lite (thẻ mới chưa đủ dữ liệu để mô hình hoá độ khó).
	minReviewsForFSRS = 3
	// maxIntervalDays trần trên của cả stability lẫn interval.
	maxIntervalDays = 365
)

// fallbackIntervals ánh xạ reps -> số ngày tới hạn (1-3-7-14-30).
var fallbackIntervals = []int{1, 3, 7, 14, 30}

// ScheduleResult là kết quả chấm điểm 1 thẻ.
type ScheduleResult struct {
	DueAt        time.Time
	IntervalDays int
	Stability    float64
	Difficulty   float64
	// Lapse = user bấm "Quên" → tăng lapses của card.
	Lapse bool
	// FallbackUsed = đi dùng chuỗi 1-3-7-14-30 thay vì FSRS-lite.
	FallbackUsed bool
}

// ScheduleNext tính lịch ôn kế tiếp. Hàm thuần — dễ test, không I/O.
//
// V1 dùng FSRS-lite (không phải ts-fsrs: tránh trôi version + giữ build
// offline) với cùng input (grade 1-4, stability, difficulty) và chuỗi
// fallback 1-3-7-14-30 khi reps < 3.
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
		// Bounds check TRƯỚC khi index: thứ tự cũ index trước và sẽ panic nếu
		// reps vượt len(fallbackIntervals).
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

// Replay tính lại reps/lapses/stability/difficulty/due của 1 card từ lịch sử
// review. Sync merge gọi hàm này khi lịch sử từ peer bổ sung thêm review
// (append-only, không LWW) — port từ vòng recompute trong mergeReviews.
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

// initDifficulty ánh xạ grade của lần FSRS đầu -> difficulty (dải FSRS D
// 1..10, thấp = dễ).
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

// initStability ánh xạ grade của lần FSRS đầu -> stability tính bằng ngày.
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

// stabilityFactor nhân stability sau mỗi lần ôn FSRS.
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
