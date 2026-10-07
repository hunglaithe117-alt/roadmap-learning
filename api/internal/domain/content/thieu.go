package content

// THIEUAxis represents a self-assessment checklist rubric axis.
type THIEUAxis struct {
	Code string
	Name string
	Desc string
}

// THIEUAxes defines the 8 evaluation axes (A-H, 1-5 scale).
var THIEUAxes = []THIEUAxis{
	{Code: "A", Name: "Trọng âm từ", Desc: "Từ đa âm tiết có nhấn đúng âm tiết mạnh không? (1 = đều đều, 5 = rõ trọng âm)"},
	{Code: "B", Name: "Nhịp câu (chunking)", Desc: "Câu có ngắt cụm content/function, từ nội dung đọc mạnh hơn không?"},
	{Code: "C", Name: "Nối âm – nuốt âm", Desc: "Có nối phụ âm–nguyên âm, giảm âm function words (to → /tə/) tự nhiên không?"},
	{Code: "D", Name: "Ngữ điệu", Desc: "Câu có lên/xuống giọng đúng ý (hỏi, liệt kê, nhấn mạnh) không?"},
	{Code: "E", Name: "Nguyên âm – phụ âm", Desc: "Các âm khó (/θ ð ʃ tʃ ɪ iː/) có rõ, không Việt hóa không?"},
	{Code: "F", Name: "Cụm PVO", Desc: "Buổi này dùng đúng ≥3 cụm PVO đã học trong câu tự nói không?"},
	{Code: "G", Name: "Trôi chảy", Desc: "Nói liền mạch, ít ậm ừ, đúng nhịp thở theo chunk không?"},
	{Code: "H", Name: "Thói quen", Desc: "Đủ drill ngày (SRS queue hết) + ghi chú lỗi vào notes không?"},
}

// THIEUCode finds a rubric axis by code A-H.
func THIEUCode(code string) (THIEUAxis, bool) {
	for _, ax := range THIEUAxes {
		if ax.Code == code {
			return ax, true
		}
	}
	return THIEUAxis{}, false
}

// THIEUAvg computes the average score over scored axes, ignoring unrated ones.
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
