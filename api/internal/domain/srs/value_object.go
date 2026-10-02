package srs

// Grade là điểm user chấm cho 1 lần ôn, thang 1-4 (thang điểm dành cho
// user mới; 0 và 5 là khoảng dự phòng cho SRS mở rộng, KHÔNG nhận ở API).
type Grade int

// Thang điểm — hợp đồng đóng băng với UI (xem api/srs.go v1).
const (
	GradeAgain Grade = 1 // Quên
	GradeHard  Grade = 2 // Khó
	GradeGood  Grade = 3 // Được
	GradeEasy  Grade = 4 // Dễ

	// MinGrade / MaxGrade là khoảng hợp lệ API nhận (1-4).
	MinGrade Grade = GradeAgain
	MaxGrade Grade = GradeEasy
)

// Valid báo grade có nằm trong thang 1-4 không.
func (g Grade) Valid() bool { return g >= MinGrade && g <= MaxGrade }

// String là nhãn tiếng Việt dùng cho thông báo lỗi.
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
