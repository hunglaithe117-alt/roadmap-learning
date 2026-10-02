package insight

import (
	"context"
	"fmt"
	"time"

	domain "langapp/internal/domain/insight"
)

// Service là use case của context insight (dashboard + báo cáo tiến độ).
type Service struct {
	repo Repository
	now  NowFunc
}

// NewService dựng service. nowFn nil → UTC thật.
func NewService(repo Repository, nowFn NowFunc) *Service {
	if nowFn == nil {
		nowFn = Clock
	}
	return &Service{repo: repo, now: nowFn}
}

// Stats là payload dashboard. `Done`/`Total` là alias deprecated của
// `DoneWindow`/`TotalAll` — giữ lại vì app v1 trả cả 2 và UI cũ đang đọc tên
// cũ (hợp đồng đóng băng ở D5 v1).
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
	// Timezone luôn là "UTC" (xem `Clock`).
	Timezone string
}

// StreakScanLimit là số ngày đọc để tính streak — giữ nguyên LIMIT 400 của v1.
const StreakScanLimit = 400

// Stats trả số liệu dashboard cho cửa sổ `range` (week = 7 ngày, month = 30
// ngày; rỗng = week).
//
// Cần nói rõ 2 con số dễ nhầm:
//   - `DoneWindow` = số review trong CỬA SỔ (đã ôn bao nhiêu).
//   - `TotalAll` = tổng số thẻ hiện có, KHÔNG theo cửa sổ (kho học liệu).
//   - `DueNow` = số thẻ đã tới hạn. Hàng đợi ôn (`srs.DueCards`) còn kéo thêm
//     thẻ chưa từng ôn (`state = 'new'`), nên `DueNow` ≤ số thẻ player hiện;
//     đó là hành vi v1 và UI vẫn hiện "x/y" bằng 2 số khác nhau.
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

// ComputeStreak đếm số ngày UTC LIÊN TIẾP có ít nhất 1 review, tính tới hôm
// nay.
//
// Hôm nay chưa học nhưng hôm qua có thì streak GIỮ NGUYÊN (không về 0 sớm
// trong ngày) — đó là hành vi v1 và là điều user mong đợi lúc 8h sáng. Quy
// tắc nằm ở `domain/insight.ComputeStreak`.
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

// TopError là 1 từ bị đọc sai kèm số lần.
//
// `CardID`/`Front` phục vụ yêu cầu "bấm lỗi → nhảy review card" (T5.2): client
// chỉ biết `word` thì không có gì để mở. Cả hai NULLABLE và luôn đi cùng nhau
// — lỗi luyện nói tự do không gắn thẻ nào, và thẻ đã xoá mềm thì không mở
// được. `CardID` cũng null khi cùng 1 từ sai ở nhiều thẻ: ta chỉ chọn thẻ của
// lần sai MỚI NHẤT, và nếu lần đó không gắn thẻ thì để null thay vì trỏ thẻ
// cũ — UI phải chịu được việc bấm không có gì để bấm.
type TopError struct {
	Word  string
	Count int
	// CardID nil = không nhảy review được, xem comment trên.
	CardID *int64
	Front  string
}

// TopErrors trả các từ sai nhiều nhất trong sổ lỗi. Quy tắc đếm + tie-break +
// chọn thẻ đại diện nằm ở `domain/insight.TopError` (tầng này chỉ truy vấn
// rồi chuyển).
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

// topErrorScanLimit là số note lỗi đọc để đếm (giữ nguyên hằng 500 của v1:
// đếm trên toàn bộ lịch sử thì số lần mới đúng, single-user thì 500 note là
// mốc hợp lý).
const topErrorScanLimit = 500

// scanLimit quyết định số note cần quét: luôn đủ 500 để đếm chính xác, dù
// client chỉ xin `limit` kết quả.
func scanLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	return topErrorScanLimit
}

// ProgressResult là tiến độ roadmap trong 1 khoảng thời gian.
type ProgressResult struct {
	// Since là mốc bắt đầu (YYYY-MM-DD), chuẩn hoá từ đầu vào.
	Since string
	// Completed là số node `roadmap_topics` đánh dấu xong trong khoảng.
	Completed int
}

// ProgressOverRange đếm node roadmap hoàn thành từ ngày `since` trở đi, phục
// vụ biểu đồ tiến độ theo tuần/tháng (amendment A1).
//
// `completed_at` được set khi chuyển sang `done` và CLEAR khi rời `done`
// (quy tắc của domain/roadmap.SetStatus). Nên con số này là "số node đang ở
// trạng thái xong tính từ `since`", KHÔNG phải "số node đã hoàn thành rồi bỏ
// sau" — cùng hệ nghĩa với `GET /api/roadmap/progress?since=` của v1.
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
