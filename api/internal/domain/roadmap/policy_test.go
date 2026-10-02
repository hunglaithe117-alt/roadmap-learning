package roadmap

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_status_valid_covers_frozen_contract(t *testing.T) {
	for _, s := range AllStatuses {
		assert.True(t, s.Valid(), "status %q phải hợp lệ", s)
	}
	for _, s := range []Status{"", "archived", "pending", "DONE"} {
		assert.False(t, s.Valid(), "status %q không thuộc tập 4 hằng", s)
	}
	assert.Len(t, AllStatuses, 4, "tập status là hợp đồng đóng băng, không thêm bớt")
}

func Test_status_is_terminal(t *testing.T) {
	assert.True(t, Done.IsTerminal())
	assert.True(t, Skipped.IsTerminal())
	assert.False(t, NotStarted.IsTerminal())
	assert.False(t, InProgress.IsTerminal())
}

// percent là hàm unexported nên test nằm cùng package; ComputeProgress là
// hợp đồng public, test dưới đây gọi nó.
func Test_percent_floors_and_handles_zero(t *testing.T) {
	for _, c := range []struct{ done, required, want int }{
		{0, 0, 0},
		{0, 10, 0},
		{7, 8, 87}, // 87.5 -> 87
		{1, 3, 33}, // 33.3 -> 33
		{10, 10, 100},
		{1, 0, 0},
	} {
		assert.Equal(t, c.want, percent(c.done, c.required), "done=%d required=%d", c.done, c.required)
	}
}

func Test_compute_progress_excludes_optional_from_denominator(t *testing.T) {
	topics := []Topic{
		{Status: Done},
		{Status: Done},
		{Status: InProgress},
		{Status: NotStarted},
		{Status: Done, IsOptional: Optional},       // tham khảo đã xong
		{Status: NotStarted, IsOptional: Optional}, // tham khảo chưa xong
	}
	got := ComputeProgress(topics)
	assert.Equal(t, 4, got.TopicsRequired, "4 node bắt buộc là mẫu số")
	assert.Equal(t, 2, got.TopicsDone, "node optional đã xong không được tính vào tử số")
	assert.Equal(t, 50, got.Percent)
	assert.Equal(t, 6, got.TopicsTotal, "tổng vẫn báo đủ để client hiện x/tổng")
}

func Test_compute_progress_all_optional_is_zero_not_nan(t *testing.T) {
	got := ComputeProgress([]Topic{
		{Status: NotStarted, IsOptional: Optional},
		{Status: Done, IsOptional: Optional},
	})
	assert.Equal(t, 0, got.Percent, "path chỉ có node tham khảo -> 0%, không chia 0")
	assert.Equal(t, 2, got.TopicsTotal)
	assert.Equal(t, 0, got.TopicsRequired)
}

func Test_compute_progress_skips_soft_deleted(t *testing.T) {
	got := ComputeProgress([]Topic{
		{Status: Done},
		{Status: Done, Deleted: 1},
		{Status: NotStarted, Deleted: 1},
	})
	assert.Equal(t, 1, got.TopicsRequired)
	assert.Equal(t, 100, got.Percent)
}

func Test_set_status_sets_completed_at_on_entering_done(t *testing.T) {
	now := time.Date(2026, 9, 28, 3, 4, 5, 0, time.UTC)
	got := SetStatus(StatusUpdate{Current: InProgress, Now: now}, Done)
	assert.Equal(t, Done, got.Status)
	assert.Equal(t, "2026-09-28T03:04:05Z", got.CompletedAt)
}

func Test_set_status_keeps_first_completed_at_when_repeated(t *testing.T) {
	first := SetStatus(StatusUpdate{Current: InProgress, Now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}, Done)
	again := SetStatus(StatusUpdate{Current: Done, Now: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}, Done)
	assert.Equal(t, "2026-01-01T00:00:00Z", first.CompletedAt)
	assert.Empty(t, again.CompletedAt, "đã done rồi thì không nhấp nháy mốc, repository giữ mốc cũ")
}

func Test_set_status_clears_completed_at_when_leaving_done(t *testing.T) {
	for _, next := range []Status{NotStarted, InProgress, Skipped} {
		got := SetStatus(StatusUpdate{Current: Done, Now: time.Now()}, next)
		assert.Equal(t, next, got.Status)
		assert.Empty(t, got.CompletedAt, "rời done thì completed_at phải NULL, status %q", next)
	}
}

func Test_set_status_rejects_unknown_status_keeps_current(t *testing.T) {
	got := SetStatus(StatusUpdate{Current: InProgress}, "archived")
	assert.Equal(t, InProgress, got.Status, "status hỏng phải giữ nguyên hiện trạng")
	assert.Empty(t, got.CompletedAt)
}

func Test_set_status_normalizes_now_to_utc(t *testing.T) {
	loc := time.FixedZone("UTC+7", 7*3600)
	got := SetStatus(StatusUpdate{Current: InProgress, Now: time.Date(2026, 9, 28, 10, 0, 0, 0, loc)}, Done)
	assert.Equal(t, "2026-09-28T03:00:00Z", got.CompletedAt, "mọi mốc thời gian server đều chuẩn hóa UTC")
}

func ts(v string) *string { return &v }

func Test_completed_since_counts_by_completed_at_window(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	topics := []Topic{
		{Status: Done, CompletedAt: ts("2026-09-05T00:00:00Z")},                       // trong khoảng
		{CompletedAt: ts("2026-09-10T00:00:00Z")},                                     // trong khoảng, dù status chưa set? vẫn tính theo mốc
		{Status: Done, CompletedAt: ts("2026-08-20T00:00:00Z")},                       // trước khoảng
		{Status: Done, CompletedAt: ts("2026-10-05T00:00:00Z")},                       // sau khoảng
		{Status: Done, IsOptional: Optional, CompletedAt: ts("2026-09-06T00:00:00Z")}, // optional
		{Status: Done, Deleted: 1, CompletedAt: ts("2026-09-07T00:00:00Z")},           // xóa mềm
		{Status: Done}, // chưa bao giờ done → completed_at NULL
		{Status: Done, CompletedAt: ts("không-parse-được")}, // dữ liệu hỏng
	}
	assert.Equal(t, 2, CompletedSince(topics, from, to))
}

func Test_completed_since_with_zero_to_counts_up_to_now(t *testing.T) {
	topics := []Topic{
		{Status: Done, CompletedAt: ts("2020-01-01T00:00:00Z")},
		{Status: Done, CompletedAt: ts("2999-01-01T00:00:00Z")},
	}
	assert.Equal(t, 2, CompletedSince(topics, time.Time{}, time.Time{}), "to rỗng = không chặn trên")
}
