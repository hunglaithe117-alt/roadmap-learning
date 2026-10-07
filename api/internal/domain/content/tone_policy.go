package content

// ToneError represents an error during tone parsing or validation.
type ToneError string

// Error implements the error interface.
func (e ToneError) Error() string { return string(e) }

func errTone(msg string) error { return ToneError(msg) }

// SRS grade constants for tone grading (aligned with srs.Grade 1-4).
const (
	toneGradeAgain = 1
	toneGradeHard  = 2
	toneGradeGood  = 3
	toneGradeEasy  = 4
)

// ToneGradeResult represents the evaluation result of a tone pair answer.
type ToneGradeResult struct {
	Grade int
	Score float64
	Exact bool
	Hit   int
}

// GradeTonePair compares answered tones against expected tones and returns a GradeToneResult.
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
