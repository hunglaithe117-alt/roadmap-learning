package roadmap

import "time"

// StatusChange là kết quả chuyển trạng thái: status mới + completed_at mới
// (amendment A1). Repository dùng đúng 1 UPDATE với cả 2 cột, KHÔNG set
// updated_at tường minh để trigger `langapp_touch_updated_at` chạm mốc —
// status là thay đổi cần lan sang máy peer.
type StatusChange struct {
	Status Status
	// CompletedAt là string UTC RFC3339, "" = KHÔNG đổi (giữ mốc cũ) hoặc
	// clear. Ở tầng application `resolveCompletedAt` dịch "" thành nil để ghi
	// SQL NULL thật — xem `Topic.CompletedAt`.
	CompletedAt string
}

// StatusUpdate là input của SetStatus: trạng thái hiện tại + mốc thời gian
// cần ghi. Now truyền vào (không tự gọi time.Now) để test được và để
// application quyết định 1 nguồn thời gian cho cả request.
type StatusUpdate struct {
	Current Status
	Note    string
	Now     time.Time
}

// SetStatus tính trạng thái + completed_at sau khi đổi.
//
// Quy tắc A1: set completed_at khi chuyển SANG done, clear khi RỜI done.
// Chuyển not_started -> done -> done lần sau trả CompletedAt rỗng (nghĩa là
// giữ mốc cũ, không nhấp nháy mỗi lần bấm lại) — mốc cần ổn định để biểu đồ
// theo tuần không bị dịch chuyển. Việc quyết định giá trị ghi xuống (kể cả
// trường hợp row `done` mà chưa có mốc) là `resolveCompletedAt` ở application.
func SetStatus(up StatusUpdate, next Status) StatusChange {
	if !next.Valid() {
		return StatusChange{Status: up.Current}
	}
	change := StatusChange{Status: next}
	switch {
	case next == Done && up.Current != Done:
		change.CompletedAt = up.Now.UTC().Format(time.RFC3339)
	case next == Done && up.Current == Done:
		change.CompletedAt = "" // giữ mốc cũ
	case next != Done:
		change.CompletedAt = "" // clear
	}
	return change
}

// CompletedSince đếm node hoàn thành trong khoảng [from, to) theo
// completed_at (endpoint `progress?since=YYYY-MM-DD`, STACK-V2-PLAN §5).
// Chỉ tính node bắt buộc — node optional không nằm trong mẫu số.
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
