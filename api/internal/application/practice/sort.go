package practice

import "sort"

// sortTopErrors sắp theo số lần GIẢM dần, tie-break chữ cái TĂNG dần.
// Tie-break là bắt buộc: map iteration có thứ tự ngẫu nhiên, UI sẽ nhảy vị
// trí giữa 2 lần gọi nếu 2 từ cùng số lần.
func sortTopErrors(out []TopErrorCount) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Word < out[j].Word
		}
		return out[i].Count > out[j].Count
	})
}
