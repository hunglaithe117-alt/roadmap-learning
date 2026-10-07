package srs

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_schedule_new_card_due_plus_one_day(t *testing.T) {
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	got := ScheduleNext(0, 0, 0, GradeGood, now)
	assert.Equal(t, 1, got.IntervalDays)
	assert.True(t, got.FallbackUsed)
	assert.Equal(t, 24*time.Hour, got.DueAt.Sub(now))
}

func Test_schedule_fallback_1_3_7_before_fsrs(t *testing.T) {
	now := time.Now().UTC()
	for reps, days := range map[int]int{0: 1, 1: 3, 2: 7} {
		got := ScheduleNext(reps, 0, 0, GradeGood, now)
		assert.Equal(t, days, got.IntervalDays, "reps=%d", reps)
		assert.True(t, got.FallbackUsed, "reps=%d phải dùng fallback", reps)
	}
}

func Test_schedule_again_resets_to_one_day(t *testing.T) {
	got := ScheduleNext(5, 10, 4, GradeAgain, time.Now().UTC())
	assert.Equal(t, 1, got.IntervalDays)
	assert.True(t, got.Lapse)
}

func Test_schedule_fsrs_good_increases_stability(t *testing.T) {
	now := time.Now().UTC()
	first := ScheduleNext(3, 0, 0, GradeGood, now)
	require.False(t, first.FallbackUsed, "reps=3 phải vào FSRS")

	second := ScheduleNext(4, first.Stability, first.Difficulty, GradeGood, first.DueAt)
	assert.Greater(t, second.Stability, first.Stability)
	assert.GreaterOrEqual(t, second.IntervalDays, first.IntervalDays)
}

func Test_schedule_difficulty_and_interval_stay_in_range(t *testing.T) {
	now := time.Now().UTC()
	s, d := 2.0, 4.0
	for _, g := range []Grade{GradeAgain, GradeHard, GradeGood, GradeEasy, GradeEasy, GradeAgain, GradeGood} {
		got := ScheduleNext(5, s, d, g, now)
		assert.GreaterOrEqual(t, got.Difficulty, 1.0)
		assert.LessOrEqual(t, got.Difficulty, 10.0)
		assert.GreaterOrEqual(t, got.IntervalDays, 1)
		assert.LessOrEqual(t, got.IntervalDays, maxIntervalDays)
		s, d = got.Stability, got.Difficulty
	}
}

// Test_schedule_never_panics_on_any_reps: nhánh fallback chỉ chạy khi
// reps < minReviewsForFSRS nên bảng 5 phần tử luôn đủ; nhưng nếu ngưỡng đổi
// (thêm FSRS-mới) mà quên bounds-check thì index sẽ panic. reps âm cũng phải
// an toàn — đây là điều kiện v1 đã sửa (D2-6).
func Test_schedule_never_panics_on_any_reps(t *testing.T) {
	for reps := -5; reps < 40; reps++ {
		require.NotPanics(t, func() {
			got := ScheduleNext(reps, 0, 0, GradeGood, time.Now())
			assert.GreaterOrEqual(t, got.IntervalDays, 1, "reps=%d", reps)
		}, "reps=%d", reps)
	}
}

func Test_schedule_fallback_table_covers_whole_fallback_range(t *testing.T) {
	// reps 0..2 là toàn bộ miền fallback (reps >= 3 đã vào FSRS-lite).
	for reps, days := range map[int]int{0: 1, 1: 3, 2: 7} {
		got := ScheduleNext(reps, 0, 0, GradeGood, time.Now().UTC())
		assert.Equal(t, days, got.IntervalDays, "reps=%d", reps)
		assert.True(t, got.FallbackUsed, "reps=%d", reps)
	}
	assert.Equal(t, minReviewsForFSRS, 3, "ngưỡng chuyển sang FSRS-lite là 3 lần ôn")
}

func Test_schedule_normalizes_input_time_to_utc(t *testing.T) {
	loc := time.FixedZone("UTC+7", 7*3600)
	local := time.Date(2026, 9, 23, 7, 0, 0, 0, loc)
	got := ScheduleNext(0, 0, 0, GradeGood, local)
	assert.Equal(t, time.UTC, got.DueAt.Location())
	assert.Equal(t, time.Date(2026, 9, 24, 7, 0, 0, 0, loc).UTC(), got.DueAt)
}

func Test_grade_valid_range(t *testing.T) {
	for _, c := range []struct {
		g     Grade
		valid bool
	}{{GradeAgain, true}, {GradeGood, true}, {GradeEasy, true}, {0, false}, {Grade(5), false}, {Grade(-1), false}} {
		t.Run(fmt.Sprintf("grade=%d", int(c.g)), func(t *testing.T) {
			assert.Equal(t, c.valid, c.g.Valid(), "grade %d", int(c.g))
		})
	}
	assert.Equal(t, "Quên", GradeAgain.String())
	assert.Equal(t, "không hợp lệ", Grade(9).String())
}

func Test_normalize_lang(t *testing.T) {
	for _, in := range []string{"zh", "ZH-CN", "cn"} {
		got, ok := NormalizeLang(in)
		assert.True(t, ok, in)
		assert.Equal(t, LangZH, got)
	}
	for _, in := range []string{"en", " EN_us "} {
		got, ok := NormalizeLang(in)
		assert.True(t, ok, in)
		assert.Equal(t, LangEN, got)
	}
	_, ok := NormalizeLang("fr")
	assert.False(t, ok, "ngôn ngữ ngoài zh|en phải bị từ chối")
}

func Test_replay_matches_incremental_scheduling(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	grades := []Grade{GradeGood, GradeAgain, GradeGood, GradeEasy, GradeGood}
	reviews := make([]Review, 0, len(grades))
	for i, g := range grades {
		reviews = append(reviews, Review{Grade: g, ReviewedAt: base.Add(time.Duration(i) * 24 * time.Hour)})
	}

	reps, lapses, stability, difficulty, due := Replay(reviews)
	assert.Equal(t, len(grades), reps)
	assert.Equal(t, 1, lapses, "đúng 1 lần Quên")
	assert.Positive(t, stability)
	assert.Positive(t, difficulty)
	assert.False(t, due.IsZero())

	// Mốc lịch luôn tính từ reviewed_at của lần ôn đó, KHÔNG nối tiếp từ
	// due_at trước (giống hành vi recompute trong mergeReviews v1).
	s, d := 0.0, 0.0
	for i, r := range reviews {
		res := ScheduleNext(i, s, d, r.Grade, r.ReviewedAt)
		s, d = res.Stability, res.Difficulty
	}
	assert.Equal(t, s, stability)
	assert.Equal(t, d, difficulty)

	// Lần ôn cuối là "Good" ở 2026-01-05 với stability đã đủ lớn → lịch
	// phải nằm sau mốc reviewed_at đó.
	last := reviews[len(reviews)-1]
	assert.True(t, due.After(last.ReviewedAt), "lịch cuối phải sau reviewed_at cuối")
}

func Test_replay_empty_history_is_zero_value(t *testing.T) {
	reps, lapses, stability, difficulty, due := Replay(nil)
	assert.Zero(t, reps)
	assert.Zero(t, lapses)
	assert.Zero(t, stability)
	assert.Zero(t, difficulty)
	assert.True(t, due.IsZero())
}

func Test_due_filter_keeps_overdue_and_new_skips_deleted(t *testing.T) {
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	f := NewDueFilter(now, 1)

	assert.True(t, f.Keep(Card{DeckID: 1, State: StateReview, DueAt: now.Add(-time.Hour)}), "quá hạn")
	assert.True(t, f.Keep(Card{DeckID: 1, State: StateReview, DueAt: now}), "đúng hạn")
	assert.False(t, f.Keep(Card{DeckID: 1, State: StateReview, DueAt: now.Add(time.Hour)}), "chưa tới hạn")
	// state='new' dù due_at ở tương lai vẫn phải vào hàng đợi.
	assert.True(t, f.Keep(Card{DeckID: 1, State: StateNew, DueAt: now.Add(24 * time.Hour)}))
	assert.False(t, f.Keep(Card{DeckID: 1, State: StateNew, Deleted: 1, DueAt: now.Add(-time.Hour)}))
	assert.False(t, f.Keep(Card{DeckID: 2, State: StateNew}), "deck khác phải bị loại")
}

func Test_due_filter_apply_orders_by_due_then_id(t *testing.T) {
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	f := NewDueFilter(now, 1)
	cards := []Card{
		{ID: 4, DeckID: 1, State: StateReview, DueAt: now.Add(-time.Hour)},
		{ID: 2, DeckID: 1, State: StateNew, DueAt: now.Add(48 * time.Hour)},
		{ID: 3, DeckID: 1, State: StateReview, DueAt: now.Add(-time.Hour)},
		{ID: 1, DeckID: 1, State: StateReview, DueAt: now.Add(72 * time.Hour)},
		{ID: 9, DeckID: 1, State: StateReview, Deleted: 1, DueAt: now.Add(-48 * time.Hour)},
	}
	got := f.Apply(cards)
	require.Len(t, got, 3)
	assert.Equal(t, []int64{3, 4, 2}, []int64{got[0].ID, got[1].ID, got[2].ID},
		"quá hạn trước, cùng due_at thì id nhỏ trước; thẻ mới (due_at xa) ở cuối")
}
