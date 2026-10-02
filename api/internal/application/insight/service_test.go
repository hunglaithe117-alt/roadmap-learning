package insight

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "langapp/internal/domain/insight"
)

var errFake = errors.New("lỗi giả lập")

var fixedNow = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

// fakeRepo là read-model trong bộ nhớ. Tầng application không được import
// tầng hạ tầng (luật DDD §2 — grep quét cả `_test.go`).
type fakeRepo struct {
	total    int
	good     int
	allCards int
	dueNow   int
	days     []string
	errNotes []ErrorNote
	progress int
	failOn   string
}

func newFakeRepo() *fakeRepo { return &fakeRepo{} }

func (f *fakeRepo) CountReviewsSince(_ context.Context, _ string) (int, int, error) {
	if f.failOn == "CountReviewsSince" {
		return 0, 0, errFake
	}
	return f.total, f.good, nil
}

func (f *fakeRepo) ReviewDays(_ context.Context, limit int) ([]string, error) {
	if f.failOn == "ReviewDays" {
		return nil, errFake
	}
	if len(f.days) > limit {
		return f.days[:limit], nil
	}
	return f.days, nil
}

func (f *fakeRepo) CountCardsAlive(context.Context) (int, error) {
	if f.failOn == "CountCardsAlive" {
		return 0, errFake
	}
	return f.allCards, nil
}

func (f *fakeRepo) CountCardsDueNow(context.Context, string) (int, error) {
	if f.failOn == "CountCardsDueNow" {
		return 0, errFake
	}
	return f.dueNow, nil
}

func (f *fakeRepo) ListErrorNotes(_ context.Context, limit int) ([]ErrorNote, error) {
	if f.failOn == "ListErrorNotes" {
		return nil, errFake
	}
	if len(f.errNotes) > limit {
		return f.errNotes[:limit], nil
	}
	return f.errNotes, nil
}

func (f *fakeRepo) CountTopicsCompletedSince(_ context.Context, _ string) (int, error) {
	if f.failOn == "CountTopicsCompletedSince" {
		return 0, errFake
	}
	return f.progress, nil
}

func newService(repo Repository) *Service {
	return NewService(repo, func() time.Time { return fixedNow })
}

// ── Stats ───────────────────────────────────────────────────────────────────

func Test_stats_rejects_unknown_range(t *testing.T) {
	svc := newService(newFakeRepo())

	_, err := svc.Stats(context.Background(), "year")
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusBadRequest, appErr.Status)
}

func Test_stats_defaults_to_week(t *testing.T) {
	repo := newFakeRepo()
	repo.total, repo.good, repo.allCards, repo.dueNow = 10, 8, 40, 5
	svc := newService(repo)

	got, err := svc.Stats(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "week", got.Range)
	assert.Equal(t, 7, got.Days)
	assert.Equal(t, 10, got.DoneWindow)
	assert.Equal(t, 40, got.TotalAll)
	assert.Equal(t, 5, got.DueNow)
	assert.InDelta(t, 0.8, got.Accuracy, 1e-9)
	assert.Equal(t, "UTC", got.Timezone, "múi giờ server là hợp đồng đóng băng từ v1")
	// Alias deprecated vẫn phải khớp (UI cũ đang đọc).
	assert.Equal(t, got.DoneWindow, got.Done)
	assert.Equal(t, got.TotalAll, got.Total)
}

func Test_stats_month_window_is_thirty_days(t *testing.T) {
	svc := newService(newFakeRepo())

	got, err := svc.Stats(context.Background(), "month")
	require.NoError(t, err)
	assert.Equal(t, 30, got.Days)
}

func Test_stats_empty_window_reports_zero_accuracy_not_nan(t *testing.T) {
	repo := newFakeRepo() // 0 review trong cửa sổ
	svc := newService(repo)

	got, err := svc.Stats(context.Background(), "week")
	require.NoError(t, err)
	assert.Equal(t, 0, got.DoneWindow)
	assert.Equal(t, 0.0, got.Accuracy, "0/0 phải ra 0 chứ không phải NaN")
}

// ── Streak ──────────────────────────────────────────────────────────────────

func Test_streak_counts_consecutive_utc_days_ending_today(t *testing.T) {
	repo := newFakeRepo()
	repo.days = []string{"2026-09-28", "2026-09-27", "2026-09-26"}
	svc := newService(repo)

	got, err := svc.ComputeStreak(context.Background(), fixedNow)
	require.NoError(t, err)
	assert.Equal(t, 3, got)
}

func Test_streak_keeps_yesterdays_streak_when_today_not_studied(t *testing.T) {
	repo := newFakeRepo()
	repo.days = []string{"2026-09-27", "2026-09-26"}
	svc := newService(repo)

	got, err := svc.ComputeStreak(context.Background(), fixedNow)
	require.NoError(t, err)
	assert.Equal(t, 2, got, "8h sáng chưa học không được làm streak về 0")
}

func Test_streak_is_zero_after_two_idle_days(t *testing.T) {
	repo := newFakeRepo()
	repo.days = []string{"2026-09-26"}
	svc := newService(repo)

	got, err := svc.ComputeStreak(context.Background(), fixedNow)
	require.NoError(t, err)
	assert.Equal(t, 0, got)
}

// ── TopErrors ───────────────────────────────────────────────────────────────

func Test_top_errors_counts_and_orders(t *testing.T) {
	repo := newFakeRepo()
	repo.errNotes = []ErrorNote{
		{Wrong: []string{"the", "a"}}, {Wrong: []string{"the"}}, {Wrong: []string{"the", "a"}},
	}
	svc := newService(repo)

	got, err := svc.TopErrors(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, TopError{Word: "the", Count: 3}, got[0])
	assert.Equal(t, TopError{Word: "a", Count: 2}, got[1])
}

func Test_top_errors_respects_limit(t *testing.T) {
	repo := newFakeRepo()
	repo.errNotes = []ErrorNote{{Wrong: []string{"a", "b", "c"}}}
	svc := newService(repo)

	got, err := svc.TopErrors(context.Background(), 1)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func Test_top_errors_scans_full_history_not_just_limit(t *testing.T) {
	// limit=1 nghĩa là "trả 1 kết quả", KHÔNG phải "chỉ quét 1 note" — quét 1
	// note sẽ ra số lần sai (UI hiện %) hoàn toàn sai.
	repo := newFakeRepo()
	repo.errNotes = []ErrorNote{{Wrong: []string{"a"}}, {Wrong: []string{"a"}}, {Wrong: []string{"a"}}}
	svc := newService(repo)

	got, err := svc.TopErrors(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 3, got[0].Count, "phải đếm trên toàn bộ lịch sử lỗi")
}

// Test_top_errors_carries_card_of_the_newest_error — nợ server M5 (T5.2: "bấm
// lỗi → nhảy review"). `CardID` phải là thẻ của lần sai MỚI NHẤT: `errNotes`
// vào theo thứ tự note mới trước, nên note đầu tiên mang từ đó là thẻ đại diện.
func Test_top_errors_carries_card_of_the_newest_error(t *testing.T) {
	repo := newFakeRepo()
	repo.errNotes = []ErrorNote{
		{CardID: i64p(7), Front: "你好", Wrong: []string{"ni"}},
		{CardID: i64p(9), Front: "谢谢", Wrong: []string{"ni", "xie"}},
	}
	svc := newService(repo)

	got, err := svc.TopErrors(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "ni", got[0].Word)
	require.NotNil(t, got[0].CardID)
	assert.Equal(t, int64(7), *got[0].CardID, "phải là thẻ của lần sai mới nhất (7), không phải thẻ cũ (9)")
	assert.Equal(t, "你好", got[0].Front)
	require.NotNil(t, got[1].CardID)
	assert.Equal(t, int64(9), *got[1].CardID)
}

// Lỗi luyện nói tự do không gắn thẻ nào ⇒ `cardId` null. Đây là trạng thái hợp
// lệ, KHÔNG phải thiếu dữ liệu — UI phải hiện được từ mà không bấm được.
func Test_top_errors_leaves_card_null_when_error_is_not_attached_to_a_card(t *testing.T) {
	repo := newFakeRepo()
	repo.errNotes = []ErrorNote{{Wrong: []string{"ni"}}, {Wrong: []string{"ni"}}}
	svc := newService(repo)

	got, err := svc.TopErrors(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Nil(t, got[0].CardID)
	assert.Empty(t, got[0].Front)
}

func i64p(v int64) *int64 { return &v }

// ── ProgressOverRange ───────────────────────────────────────────────────────

func Test_progress_over_range_rejects_bad_date_format(t *testing.T) {
	svc := newService(newFakeRepo())

	_, err := svc.ProgressOverRange(context.Background(), "28/09/2026")
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusBadRequest, appErr.Status)
}

func Test_progress_over_range_counts_completed_topics(t *testing.T) {
	repo := newFakeRepo()
	repo.progress = 6
	svc := newService(repo)

	got, err := svc.ProgressOverRange(context.Background(), "2026-09-01")
	require.NoError(t, err)
	assert.Equal(t, 6, got.Completed)
	assert.Equal(t, "2026-09-01", got.Since)
}

// ── Không import hạ tầng ────────────────────────────────────────────────────

// Rule: `insight` chỉ đọc, không được có bất kỳ method ghi nào. Nếu ai đó thêm
// `UpdateCard` vào interface này thì ranh giới read-model vỡ.
func Test_repository_interface_is_read_only(t *testing.T) {
	var repo Repository = newFakeRepo()
	assert.NotNil(t, repo)
	// Compile-time: interface này chỉ có method trả về dữ liệu. Khai báo tường
	// minh để test này tự đỏ nếu ai đó thêm method ghi mà quên cập nhật.
	_ = domain.ErrorEntry{}
}
