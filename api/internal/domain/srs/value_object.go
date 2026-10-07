package srs

// Grade represents the user-selected review quality grade on a scale of 1-4.
type Grade int

const (
	// GradeAgain indicates complete failure to recall.
	GradeAgain Grade = 1
	// GradeHard indicates significant difficulty during recall.
	GradeHard Grade = 2
	// GradeGood indicates successful recall with standard effort.
	GradeGood Grade = 3
	// GradeEasy indicates immediate and effortless recall.
	GradeEasy Grade = 4

	// MinGrade is the minimum valid review grade.
	MinGrade Grade = GradeAgain
	// MaxGrade is the maximum valid review grade.
	MaxGrade Grade = GradeEasy
)

// Valid reports whether the grade falls within the supported 1-4 range.
func (g Grade) Valid() bool { return g >= MinGrade && g <= MaxGrade }

// String returns the string representation of the grade.
func (g Grade) String() string {
	switch g {
	case GradeAgain:
		return "Quên"
	case GradeHard:
		return "Khó"
	case GradeGood:
		return "Được"
	case GradeEasy:
		return "Dễ"
	default:
		return "không hợp lệ"
	}
}


