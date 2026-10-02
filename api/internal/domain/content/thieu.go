package content

// THIEUAxis là 1 trục checklist tự đánh giá với rubric tiếng Việt.
type THIEUAxis struct {
	Code string
	Name string
	Desc string
}

// THIEUAxes là 8 trục A-H, thang 1-5, do tác giả app soạn.
var THIEUAxes = []THIEUAxis{
	{"A", "Trọng âm từ", "Từ đa âm tiết có nhấn đúng âm tiết mạnh không? (1 = đều đều, 5 = rõ trọng âm)"},
	{"B", "Nhịp câu (chunking)", "Câu có ngắt cụm content/function, từ nội dung đọc mạnh hơn không?"},
	{"C", "Nối âm – nuốt âm", "Có nối phụ âm–nguyên âm, giảm âm function words (to → /tə/) tự nhiên không?"},
	{"D", "Ngữ điệu", "Câu có lên/xuống giọng đúng ý (hỏi, liệt kê, nhấn mạnh) không?"},
	{"E", "Nguyên âm – phụ âm", "Các âm khó (/θ ð ʃ tʃ ɪ iː/) có rõ, không Việt hóa không?"},
	{"F", "Cụm PVO", "Buổi này dùng đúng ≥3 cụm PVO đã học trong câu tự nói không?"},
	{"G", "Trôi chảy", "Nói liền mạch, ít ậm ừ, đúng nhịp thở theo chunk không?"},
	{"H", "Thói quen", "Đủ drill ngày (SRS queue hết) + ghi chú lỗi vào notes không?"},
}

// THIEUCode tra trục theo mã A-H; ok=false nếu mã lạ.
func THIEUCode(code string) (THIEUAxis, bool) {
	for _, ax := range THIEUAxes {
		if ax.Code == code {
			return ax, true
		}
	}
	return THIEUAxis{}, false
}

// THIEUAvg là điểm trung bình trên các trục CÓ điểm. Trục bỏ trống không
// tính vào mẫu số (user chấm 4/8 trục thì trung bình phải phản ánh 4 trục
// đó, không chia 8). Không có trục nào điểm -> 0.
func THIEUAvg(scores map[string]int) float64 {
	sum, n := 0, 0
	for _, ax := range THIEUAxes {
		if v, ok := scores[ax.Code]; ok {
			sum += v
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return float64(sum) / float64(n)
}
