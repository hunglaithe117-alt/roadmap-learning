package insight

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func day(s string) string { return s }

func Test_range_days_matches_frozen_contract(t *testing.T) {
	days, ok := RangeWeek.Days()
	require.True(t, ok)
	assert.Equal(t, 7, days)

	days, ok = RangeMonth.Days()
	require.True(t, ok)
	assert.Equal(t, 30, days)

	days, ok = Range("").Days()
	require.True(t, ok)
	assert.Equal(t, 7, days, "range rỗng = mặc định week")

	_, ok = Range("year").Days()
	assert.False(t, ok, "range ngoài week|month phải bị từ chối")
}

func Test_compute_accuracy_ratio_and_zero_case(t *testing.T) {
	assert.InDelta(t, 0.75, ComputeAccuracy(ReviewCount{Total: 4, Good: 3}), 1e-9)
	assert.Equal(t, 0.0, ComputeAccuracy(ReviewCount{}), "chưa ôn -> 0, không NaN")
	assert.Equal(t, 1.0, ComputeAccuracy(ReviewCount{Total: 3, Good: 3}))
}

func Test_due_now_includes_new_cards(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	past := now.Add(-time.Hour)

	assert.True(t, DueNow(future, "new", 0, now), "thẻ mới có due_at +24h vẫn phải vào hàng đợi")
	assert.True(t, DueNow(past, "review", 0, now))
	assert.True(t, DueNow(now, "review", 0, now), "đúng hạn")
	assert.False(t, DueNow(future, "review", 0, now))
	assert.False(t, DueNow(past, "review", 1, now), "thẻ xóa mềm không đến hạn")
}

func Test_compute_streak_counts_consecutive_days(t *testing.T) {
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	days := map[string]bool{day("2026-09-28"): true, day("2026-09-27"): true, day("2026-09-26"): true}
	assert.Equal(t, 3, ComputeStreak(days, now))
}

func Test_compute_streak_keeps_yesterday_when_today_empty(t *testing.T) {
	now := time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC)
	days := map[string]bool{day("2026-09-27"): true, day("2026-09-26"): true}
	assert.Equal(t, 2, ComputeStreak(days, now),
		"sáng sớm chưa học hôm nay không được làm streak về 0")
}

func Test_compute_streak_zero_when_gap(t *testing.T) {
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, 0, ComputeStreak(map[string]bool{day("2026-09-20"): true}, now),
		"hôm nay + hôm qua đều không có review -> 0")
	assert.Equal(t, 0, ComputeStreak(nil, now))
}

func Test_compute_streak_stops_at_first_gap(t *testing.T) {
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	days := map[string]bool{
		day("2026-09-28"): true,
		day("2026-09-27"): true,
		day("2026-09-24"): true, // có gap 26/25
	}
	assert.Equal(t, 2, ComputeStreak(days, now), "dừng ở chỗ gãy, không nhảy qua ngày trống")
}

func Test_compute_streak_ignores_future_days(t *testing.T) {
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	days := map[string]bool{day("2026-09-28"): true, day("2026-09-29"): true, day("2026-09-30"): true}
	assert.Equal(t, 1, ComputeStreak(days, now), "ngày tương lai không được tính vào streak")
}

func Test_compute_streak_uses_utc_days(t *testing.T) {
	// 23:30 ở UTC+7 là 16:30 cùng ngày UTC -> vẫn là 1 ngày duy nhất.
	loc := time.FixedZone("UTC+7", 7*3600)
	local := time.Date(2026, 9, 28, 23, 30, 0, 0, loc)
	days := map[string]bool{day("2026-09-28"): true}
	assert.Equal(t, 1, ComputeStreak(days, local))
}

func Test_top_error_ranks_by_count_then_alphabet(t *testing.T) {
	entries := []ErrorEntry{
		{Wrong: []string{"the", "cat", " CAT "}},
		{Wrong: []string{"the"}},
		{Wrong: []string{"dog"}},
	}
	got := TopError(entries, 10)
	require.Len(t, got, 3)
	assert.Equal(t, ErrorCount{Word: "cat", Count: 2}, got[0], "cat = the = 2 lần, cat đứng trước theo alphabet")
	assert.Equal(t, ErrorCount{Word: "the", Count: 2}, got[1])
	assert.Equal(t, ErrorCount{Word: "dog", Count: 1}, got[2])
}

func Test_top_error_respects_limit_and_normalizes(t *testing.T) {
	entries := []ErrorEntry{{Wrong: []string{" A ", "a", "", "  ", "b"}}}
	assert.Equal(t, []ErrorCount{{Word: "a", Count: 2}, {Word: "b", Count: 1}}, TopError(entries, 5))
	assert.Len(t, TopError(entries, 1), 1)
	assert.Empty(t, TopError(entries, 0), "limit <= 0 trả rỗng, không trả tất cả")
	assert.Empty(t, TopError(nil, 5))
}

func Test_top_error_is_stable_across_calls(t *testing.T) {
	entries := []ErrorEntry{{Wrong: []string{"z", "y", "x", "w"}}}
	first := TopError(entries, 4)
	for i := 0; i < 20; i++ {
		assert.Equal(t, first, TopError(entries, 4), "thứ tự phải ổn định, UI không nhảy vị trí khi refresh")
	}
}
