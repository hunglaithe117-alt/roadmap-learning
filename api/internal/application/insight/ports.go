// Package insight là bounded context BÁO CÁO (read model): không sở hữu bảng
// nào, chỉ đọc và tổng hợp từ `reviews` (srs), `cards` (srs), `notes`
// (practice) và `roadmap_topics` (roadmap).
//
// Vì vậy mọi hàm ở đây nhận dữ liệu ĐÃ truy vấn sẵn — không có I/O trong tầng
// này. Quy tắc (accuracy, streak, top lỗi) nằm ở domain/insight, tầng này
// chỉ điều phối truy vấn + validate input.
//
// Lớp này KHÔNG import driver DB và KHÔNG import tầng hạ tầng
// (STACK-V2-PLAN §2).
package insight

import (
	"context"
	"time"
)

// Repository là toàn bộ truy vấn READ-ONLY mà context insight cần. Không có
// method ghi: insight không sở hữu bảng nào, chỉ tổng hợp dữ liệu người khác
// đã ghi.
type Repository interface {
	// CountReviewsSince trả tổng số review trong cửa sổ + số review được chấm
	// đạt (grade >= 3). `since` là RFC3339; so sánh chuỗi được vì cột
	// `reviewed_at` là TEXT cùng định dạng (và `idx_reviews_reviewed_at` phục
	// vụ đúng truy vấn này — xem migration 00005).
	CountReviewsSince(ctx context.Context, since string) (total, good int, err error)
	// ReviewDays trả các ngày UTC (YYYY-MM-DD) có ít nhất 1 review, mới nhất
	// trước, tối đa `limit` ngày. Giới hạn 400 là hằng của v1: streak dài hơn
	// 400 ngày là hiếm gặp và quét toàn bộ lịch sử chỉ để đếm là lãng phí.
	ReviewDays(ctx context.Context, limit int) ([]string, error)
	// CountCardsAlive đếm thẻ còn sống (mọi deck) — trả `total_all`.
	CountCardsAlive(ctx context.Context) (int, error)
	// CountCardsDueNow đếm thẻ đã tới hạn (due_at <= now). KHÔNG kéo thẻ
	// `state = 'new'`: `Stats.DueNow` của app v1 đếm đúng như vậy, còn hàng
	// đợi ôn (`srs.DueCards`) mới luôn kéo thẻ mới. Hai con số khác nhau là
	// CỐ Ý — xem comment ở use case `Stats`.
	CountCardsDueNow(ctx context.Context, now string) (int, error)
	// ListErrorNotes trả các note ERR| mới nhất (tối đa `limit` note), MỖI note
	// kèm danh sách từ sai + thẻ mà lần sai đó thuộc về.
	//
	// Thứ tự PHẢI là note mới trước (id giảm dần): `domain.TopError` chọn
	// `cardId` của từ từ lần sai MỚI NHẤT, nên đảo thứ tự ở đây là mọi từ trỏ
	// sang thẻ cũ mà không có lỗi nào báo.
	//
	// Note không phải dạng ghi lỗi (đánh dấu đã xử lý) trả `Wrong` rỗng — vẫn
	// được tính 1 note trong LIMIT.
	ListErrorNotes(ctx context.Context, limit int) ([]ErrorNote, error)
	// CountTopicsCompletedSince đếm node roadmap đánh dấu xong có
	// `completed_at >= since` — nuôi biểu đồ tiến độ theo tuần/tháng.
	CountTopicsCompletedSince(ctx context.Context, since string) (int, error)
}

// ErrorNote là 1 dòng sổ lỗi đã đọc từ `notes` (prefix `ERR|`).
//
// `CardID` là `notes.card_id` — cột CÓ SẴN trong schema từ `00001`, và
// `practice.AppendError` đã ghi nó khi client gửi `cardId`. Mảng `wrong` bên
// trong JSON chỉ lưu TỪ, nên không thể suy ra thẻ từ đó: cấu trúc lưu KHÔNG
// cần đổi (xem bàn giao M6a).
//
// `Front` đọc kèm từ `cards` để UI hiện được từ mà không gọi thêm 1 query. Rỗng
// khi không gắn thẻ HOẶC thẻ đã xoá mềm — 2 trường này luôn đi cùng nhau nên
// `TopError.Front` chỉ hiện khi `CardID` khác nil.
type ErrorNote struct {
	CardID *int64
	Front  string
	Wrong  []string
}

// NowFunc là nguồn thời gian, inject để test được.
type NowFunc func() time.Time

// Clock mặc định: UTC.
//
// Streak, `since` và `due_at` đều tính theo UTC — đây là hợp đồng đã đóng băng
// ở v1 (StatsResponse.Timezone luôn là "UTC"): server chuẩn hóa mọi mốc thời
// gian sang UTC, client VN hiển thị theo giờ địa phương nhưng KHÔNG gửi
// ngược ngày đã convert lên server. Đổi sang local sẽ làm streak đổi theo
// múi giờ client — đó là hồi quy hành vi, không phải cải thiện.
func Clock() time.Time { return time.Now().UTC() }

// DefaultRange là cửa sổ mặc định khi client không gửi `range`.
const DefaultRange = "week"
