// Package insight là bounded context BÁO CÁO: không sở hữu bảng nào, chỉ
// đọc và tổng hợp từ srs + roadmap + practice. Vì vậy mọi hàm ở đây nhận
// dữ liệu đã truy vấn sẵn, không có I/O.
package insight

import (
	"sort"
	"time"
)

// Range là cửa sổ thống kê. Week = 7 ngày, Month = 30 ngày (hợp đồng đóng
// băng với query param `?range=`).
type Range string

const (
	RangeWeek  Range = "week"
	RangeMonth Range = "month"
)

// Days là độ dài cửa sổ của range.
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

// Valid báo range có được API chấp nhận không. Rỗng = mặc định week.
func (r Range) Valid() bool {
	_, ok := r.Days()
	return ok
}

// Stats là payload dashboard.
type Stats struct {
	Range Range
	Days  int
	// Done là số review trong cửa sổ; Total là tổng thẻ hiện có (mọi deck,
	// không theo cửa sổ). V1 gọi 2 số này là done_window/total_all và giữ
	// done/total làm alias — ở đây dùng tên đầy đủ cho khỏi mơ hồ.
	DoneWindow int
	TotalAll   int
	DueNow     int
	Accuracy   float64
	Streak     int
	// Timezone luôn là "UTC": server chuẩn hóa mọi mốc thời gian sang UTC,
	// client tự hiển thị theo giờ địa phương.
	Timezone string
}

// ReviewCount là tổng hợp số review + số review được chấm >= 3 trong cửa sổ.
type ReviewCount struct {
	Total int
	// Good là số lần chấm Good (grade >= 3) — mẫu số của Accuracy.
	Good int
}

// ComputeAccuracy là tỉ lệ review đạt (grade >= 3). Không có review -> 0,
// không phải NaN. Cửa sổ rỗng (Total = 0) vẫn cho 0 để client hiện "—"
// từ chính Total.
func ComputeAccuracy(rc ReviewCount) float64 {
	if rc.Total <= 0 {
		return 0
	}
	return float64(rc.Good) / float64(rc.Total)
}

// DueNow là số thẻ đến hạn: đã tới hạn HOẶC chưa từng ôn (state='new'),
// đã bỏ qua thẻ xóa mềm. Đây là điều kiện hẹn giờ, KHÔNG phải truy vấn —
// áp dụng cho từng thẻ đã đọc sẵn.
func DueNow(cardDueAt time.Time, state string, deleted int, now time.Time) bool {
	if deleted != 0 {
		return false
	}
	if state == "new" {
		return true
	}
	return !cardDueAt.After(now)
}

// ComputeStreak là số ngày UTC LIÊN TIẾP có ít nhất 1 review, tính tới hôm
// nay; hôm nay chưa học nhưng hôm qua có thì streak giữ nguyên (không về 0
// sớm trong ngày). Ngày không có review nào -> 0.
//
// days là tập ngày UTC đã review, định dạng "2006-01-02".
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

// ErrorCount là 1 từ bị đọc sai, kèm số lần.
type ErrorCount struct {
	Word  string
	Count int
	// CardID là thẻ đại diện cho từ này — thẻ của lần sai MỚI NHẤT mà từ xuất
	// hiện, để UI bấm "lỗi này → nhảy review". nil khi không lần sai nào của từ
	// này gắn thẻ (luyện nói tự do) — xem `ErrorEntry.CardID`.
	CardID *int64
}

// TopError đếm từ sai nhiều nhất. Tie-break theo chữ cái tăng dần để thứ tự
// ổn định giữa 2 lần gọi (UI không nhảy vị trí khi refresh).
// limit <= 0 trả về rỗng.
//
// `CardID` của mỗi từ là thẻ của lần sai MỚI NHẤT gắn thẻ: caller truyền
// `entries` theo thứ tự note id GIẢM DẦN (mới trước) — cùng thứ tự mà
// `Repository.ListErrorNotes` đọc — và ta lấy `CardID` đầu tiên khác nil.
// `entries` không giữ mốc thời gian nên "mới nhất" chính là "đầu tiên", và điều
// này phải được nói ở interface vì caller truyền sai thứ tự thì từ đó trỏ
// thẻ cũ — sai mà không có lỗi nào báo.
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

// ErrorEntry là lỗi luyện nói đã đọc từ sổ lỗi — khai báo cục bộ để `insight`
// không import `practice` (2 context độc lập; chỉ gặp nhau ở
// application/ports.go).
//
// `CardID` là `notes.card_id` của note ghi lỗi — cột đã có sẵn ở `notes` và
// `practice.AppendError` đã ghi khi client gửi `cardId`. Mảng `wrong` chỉ
// lưu TỪ, nên thẻ phải đi kèm ở cấp note chứ không ở từng phần tử: 1 note có
// nhiều từ sai nhưng tất cả thuộc cùng 1 thẻ.
type ErrorEntry struct {
	Wrong  []string
	CardID *int64
}

// NormalizedWrong chuẩn hóa danh sách từ sai (trim + hạ chữ thường, bỏ rỗng).
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
