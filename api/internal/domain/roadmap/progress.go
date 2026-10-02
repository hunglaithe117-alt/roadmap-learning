package roadmap

import "math"

// Progress là KẾT QUẢ TÍNH của ComputeProgress — chỉ những số mà policy dùng.
// DTO trả về client là `app.Progress` (thêm `Stages`/`TopicsLocked`/`Percent`
// đã tính theo tầng trên), không phải struct này.
type Progress struct {
	// TopicsTotal giữ cả node optional để client hiện được "x / tổng".
	TopicsTotal int
	// TopicsRequired là mẫu số sau khi loại node optional (amendment A1).
	TopicsRequired   int
	TopicsDone       int
	TopicsInProgress int
	Percent          int
}

// ComputeProgress tính phần trăm hoàn thành = floor(done * 100 / total).
// Node `is_optional` KHÔNG tính vào mẫu số: node tham khảo không làm user
// cảm thấy chưa đạt (STACK-V2-PLAN §5). Done count cũng chỉ tính node bắt
// buộc — nếu không, 1 node optional đã xong trên 3 node bắt buộc chưa xong
// sẽ ra 50% cho 1 path thực tế còn 0%.
func ComputeProgress(topics []Topic) Progress {
	p := Progress{}
	for _, t := range topics {
		if t.Deleted != 0 {
			continue
		}
		p.TopicsTotal++
		if t.IsOptional == Optional {
			continue
		}
		p.TopicsRequired++
		switch t.Status {
		case Done:
			p.TopicsDone++
		case InProgress:
			p.TopicsInProgress++
		}
	}
	p.Percent = percent(p.TopicsDone, p.TopicsRequired)
	return p
}

// percent là phần trăm floor — 7/8 = 87%, không 87.5. Path rỗng hoặc toàn
// optional = 0, không phải phép chia 0.
//
// KHÔNG export: tầng application không cần nó, chỉ ComputeProgress dùng. Để
// unexported tránh thêm 1 API public thứ hai cho cùng 1 quy tắc.
func percent(done, required int) int {
	if required <= 0 {
		return 0
	}
	return int(math.Floor(float64(done) * 100 / float64(required)))
}
