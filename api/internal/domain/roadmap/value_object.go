// Package roadmap chứa phần THUẦN của bounded context lộ trình học: value
// object trạng thái/địa hình, policy tính tiến độ, trạng thái màn
// (done/current/locked) và layout bản đồ.
//
// Đây KHÔNG phải lớp đọc/ghi DB. Row thật do application/roadmap định nghĩa
// và được đổi sang các hàm ở đây qua `TopicToDomain`; các entity đầy đủ
// (Path/Stage/Resource/Milestone) đã bị xoá vì không mang logic nào — chỉ lặp
// lại row mà tầng application đã định nghĩa, và 2 bản sao đã lệch kiểu với
// nhau (`deck_id`, `status`, `is_optional`).
package roadmap

// Status là trạng thái 1 node, frozen contract 4 giá trị. CHECK ở DB khớp
// với whitelist ở application/roadmap — 2 nơi phải sửa cùng lúc.
type Status string

const (
	NotStarted Status = "not_started"
	InProgress Status = "in_progress"
	Done       Status = "done"
	Skipped    Status = "skipped"
)

// AllStatuses là tập hợp hợp lệ, dùng cho validate và cho vòng lặp test.
var AllStatuses = []Status{NotStarted, InProgress, Done, Skipped}

// Valid báo status có thuộc tập 4 hằng không.
func (s Status) Valid() bool {
	for _, v := range AllStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// IsTerminal là trạng thái kết thúc hẳn (không còn việc) — Done và Skipped.
// LevelStates coi cả 2 là "đã qua" nên không khoá màn sau.
func (s Status) IsTerminal() bool { return s == Done || s == Skipped }

// IsOptional là cờ "node tham khảo": không tính vào mẫu số phần trăm tiến độ
// (amendment A1 — STACK-V2-PLAN §5). Ở tầng application cột là INTEGER 0|1 và
// việc chấp nhận giá trị ngoài 0|1 là `ValidateIsOptional` (trả lỗi tiếng Việt
// cho client), không phải ở domain.
type IsOptional bool

// Optional / Required là 2 giá trị của cờ.
const (
	Required IsOptional = false
	Optional IsOptional = true
)
