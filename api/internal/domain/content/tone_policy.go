package content

// ErrTone là lỗi thuần của bộ thanh điệu, để transport map sang 400 mà không
// cần biết chi tiết (dùng errors.As với type này).
type ErrTone string

func (e ErrTone) Error() string { return string(e) }

func errTone(msg string) error { return ErrTone(msg) }

// ToneGrade là điểm SRS mà GradeTonePair trả về — dùng lại đúng thang của
// bounded context `srs` (1-4) nhưng khai báo cục bộ để `content` không import
// ngược `srs` (hai context độc lập, chỉ gặp nhau ở application/ports.go).
const (
	toneGradeAgain = 1 // Quên
	toneGradeHard  = 2 // Khó
	toneGradeGood  = 3 // Được
	toneGradeEasy  = 4 // Dễ
)

// ToneGradeResult là kết quả chấm 1 cặp thanh.
type ToneGradeResult struct {
	// Grade là điểm SRS 1-4 theo tỉ lệ trùng.
	Grade int
	// Score là tỉ lệ âm tiết trùng (0-1).
	Score float64
	// Exact = trùng toàn bộ.
	Exact bool
	// Hit là số âm tiết trùng.
	Hit int
}

// GradeTonePair chấm đáp án so với mẫu: trùng hết -> 4 (Dễ), >= 1/2 ->
// 3 (Được), có trùng -> 2 (Khó), không trùng -> 1 (Quên). Cặp 1 âm tiết:
// trùng -> 4, sai -> 1.
func GradeTonePair(expected, answered string) (ToneGradeResult, error) {
	exp, err := ParseToneSequence(expected)
	if err != nil {
		return ToneGradeResult{}, err
	}
	ans, err := ParseToneSequence(answered)
	if err != nil {
		return ToneGradeResult{}, err
	}
	if len(exp) != len(ans) {
		return ToneGradeResult{}, errTone(
			"số âm tiết không khớp (mẫu " + itoa(len(exp)) +
				", bạn nhập " + itoa(len(ans)) + ")")
	}
	hit := 0
	for i := range exp {
		if exp[i] == ans[i] {
			hit++
		}
	}
	result := ToneGradeResult{
		Score: float64(hit) / float64(len(exp)),
		Exact: hit == len(exp),
		Hit:   hit,
	}
	switch {
	case result.Exact:
		result.Grade = toneGradeEasy
	case result.Score >= 0.5:
		result.Grade = toneGradeGood
	case hit > 0:
		result.Grade = toneGradeHard
	default:
		result.Grade = toneGradeAgain
	}
	return result, nil
}
