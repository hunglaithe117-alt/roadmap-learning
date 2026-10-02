package roadmap

// Topic là INPUT CỦA CÁC POLICY trong package này: ComputeProgress,
// LevelStates, ComputeLayout, CompletedSince. Nó KHÔNG phải mirror của row
// `roadmap_topics`.
//
// Row thật do application/roadmap định nghĩa (`app.Topic`, cột TEXT/INTEGER
// thô) và được đổi sang đây đúng 1 lần qua `app.TopicToDomain`. Giữ 2 bản sao
// đầy đủ của cùng 1 khái niệm là nguồn của các cột lệch kiểu đã từng xảy ra
// (`deck_id` int64 vs *int64, `status` typed vs string, `is_optional` bool vs
// int) — nên ở đây chỉ giữ đúng những field mà policy thật sự đọc.
//
// Deleted là tombstone 0|1, KHÔNG dùng gorm.DeletedAt (STACK-V2-PLAN §4.4:
// tombstone integer là cơ sở của merge LWW). Repository đã lọc `deleted = 0`
// khi đọc, nhưng policy nhận cả tombstone từ test nên vẫn tự bỏ qua.
type Topic struct {
	ID      int64
	StageID int64
	// Position là thứ tự màn (level 1, 2, 3…) — khoá sắp xếp chính của layout
	// và của chuỗi mở khoá.
	Position int
	Status   Status
	// CompletedAt là amendment A1: mốc UTC khi node chuyển sang 'done', NULL
	// khi rời 'done'. Con trỏ vì cột NULLABLE và NULL khác `''` — ghi `''` sẽ
	// lọt vào `WHERE completed_at IS NOT NULL` và phá `progress?since=`.
	CompletedAt *string
	// IsOptional là amendment A1: node "tham khảo" không tính vào mẫu số %.
	IsOptional IsOptional
	// MapX / MapY là toạ độ node do user đặt tay (migration 00004). NULL =
	// để ComputeLayout tự tính. Chỉ 1 trục có giá trị vẫn dùng được: trục còn
	// lại lấy từ layout, trục đã set thì giữ nguyên ý user.
	MapX *float64
	MapY *float64

	Deleted int
}
