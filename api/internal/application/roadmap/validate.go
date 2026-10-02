package roadmap

import (
	"net/url"
	"strconv"
	"strings"

	domain "langapp/internal/domain/roadmap"
)

// Validate ở tầng application trả message tiếng Việt, giữ nguyên wording của
// app v1 (bám decks.go v4) để client không phải đổi xử lý lỗi khi M4 thay
// net/http bằng Gin. CHECK ở DB là backstop cho mọi rule dưới đây.

// ValidateSlug: trim + lower, chỉ [a-z0-9-], 1..64 ký tự.
func ValidateSlug(slug string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(slug))
	if s == "" {
		return "", newError(StatusBadRequest, "slug không được rỗng")
	}
	if len(s) > 64 {
		return "", newError(StatusBadRequest, "slug dài tối đa 64 ký tự")
	}
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			continue
		}
		return "", newError(StatusBadRequest, "slug chỉ nhận ký tự a-z, 0-9 và dấu -")
	}
	if strings.HasPrefix(s, "-") || strings.HasSuffix(s, "-") {
		return "", newError(StatusBadRequest, "slug không được bắt đầu hoặc kết thúc bằng dấu -")
	}
	return s, nil
}

// ValidateTitle: bắt buộc có chữ sau trim, tối đa 200 ký tự (đếm rune — tiêu
// đề tiếng Việt có dấu, len() byte sẽ chặn sớm quá mức).
func ValidateTitle(title string) (string, error) {
	t := strings.TrimSpace(title)
	if t == "" {
		return "", newError(StatusBadRequest, "tiêu đề không được rỗng")
	}
	if len([]rune(t)) > 200 {
		return "", newError(StatusBadRequest, "tiêu đề dài tối đa 200 ký tự")
	}
	return t, nil
}

// ValidateText: nội dung tùy chọn (goal/why/overview/note/status_note).
func ValidateText(s string, max int) (string, error) {
	t := strings.TrimSpace(s)
	if len([]rune(t)) > max {
		return "", newError(StatusBadRequest, "nội dung dài tối đa %s ký tự", strconv.Itoa(max))
	}
	return t, nil
}

// ValidateURL: nil/rỗng → nil (SQL NULL). Có giá trị thì phải parse được và
// scheme http|https — link ftp/javascript: bị từ chối vì UI render thẳng ra
// href.
func ValidateURL(u *string) (*string, error) {
	if u == nil {
		return nil, nil
	}
	s := strings.TrimSpace(*u)
	if s == "" {
		return nil, nil
	}
	if len([]rune(s)) > 2000 {
		return nil, newError(StatusBadRequest, "url dài tối đa 2000 ký tự")
	}
	parsed, err := url.Parse(s)
	if err != nil || parsed.Host == "" {
		return nil, newError(StatusBadRequest, "url không hợp lệ")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, newError(StatusBadRequest, "url chỉ nhận link http hoặc https")
	}
	return &s, nil
}

// ValidateKind: rỗng (tài liệu không phân loại) hoặc 1 trong whitelist 9 kind.
func ValidateKind(kind string) (string, error) {
	k := strings.ToLower(strings.TrimSpace(kind))
	if k == "" {
		return "", nil
	}
	for _, v := range AllKinds {
		if v == k {
			return k, nil
		}
	}
	return "", newError(StatusBadRequest, "kind chỉ nhận: %s", strings.Join(AllKinds, ", "))
}

// AllKinds là whitelist kind tài liệu — trùng `roadmap` domain nhưng khai báo
// cục bộ để validate message ở đây không cần import domain.
var AllKinds = []string{
	"video", "article", "tool", "app", "book", "course", "site", "podcast", "channel",
}

// ValidateLanguage: seed là contract zh|en, nhưng user tạo path thì nhận chuỗi
// tự do tối đa 16 ký tự (vd "zh-Hans", "vi"). Rỗng → "zh" (giữ hành vi v1).
func ValidateLanguage(lang string) (string, error) {
	l := strings.ToLower(strings.TrimSpace(lang))
	if l == "" {
		return "zh", nil
	}
	if len(l) > 16 {
		return "", newError(StatusBadRequest, "language dài tối đa 16 ký tự")
	}
	return l, nil
}

// ValidateStatus: status bắt buộc + status_note tùy chọn. Chỉ trim, KHÔNG hạ
// chữ hoa — contract đóng 4 giá trị, "DONE" phải là 400 để lỗi client lộ ra
// sớm thay vì im lặng trở thành "done".
func ValidateStatus(status *string, note *string) (Status, string, error) {
	if status == nil {
		return "", "", newError(StatusBadRequest,
			"thiếu status (not_started, in_progress, done, skipped)")
	}
	s := strings.TrimSpace(*status)
	if !ValidStatus(s) {
		return "", "", newError(StatusBadRequest,
			"status chỉ nhận: not_started, in_progress, done, skipped")
	}
	if note == nil {
		return s, "", nil
	}
	n, err := ValidateText(*note, 500)
	if err != nil {
		return "", "", err
	}
	return s, n, nil
}

// ValidatePosition: position >= 0 (CHECK ở DB chặn sớm hơn 1 tầng).
func ValidatePosition(p int) error {
	if p < 0 {
		return newError(StatusBadRequest, "position không được âm")
	}
	return nil
}

// ValidateDurationWeeks: >= 0, tương tự position.
func ValidateDurationWeeks(w int) error {
	if w < 0 {
		return newError(StatusBadRequest, "duration_weeks không được âm")
	}
	return nil
}

// ValidateIsOptional: chỉ nhận 0|1. Giá trị khác là dữ liệu hỏng, không đoán.
func ValidateIsOptional(v int) (int, error) {
	if v == 0 || v == 1 {
		return v, nil
	}
	return 0, newError(StatusBadRequest, "is_optional chỉ nhận 0 hoặc 1")
}

// ValidateOptionalFlag là ValidateIsOptional nhận con trỏ: nil = client không
// gửi field ⇒ 0 (bắt buộc), vì CHECK của cột cũng là NOT NULL DEFAULT 0.
func ValidateOptionalFlag(v *int) (int, error) {
	if v == nil {
		return 0, nil
	}
	return ValidateIsOptional(*v)
}

// ValidateBookmarkStatus: trạng thái kho link. Rỗng → `to_read` (DEFAULT của
// cột), sai thì 400 liệt kê đủ 4 hằng.
func ValidateBookmarkStatus(raw string) (string, error) {
	s, err := domain.ParseBookmarkStatus(raw)
	if err != nil {
		return "", newError(StatusBadRequest, "%s", err.Error())
	}
	return string(s), nil
}

// ValidateBookmarkTags: chuẩn hoá + kiểm tra danh sách tag trả về CSV để ghi.
// Rỗng → "" (không phải ",") để khớp DEFAULT của cột `tags` NOT NULL.
func ValidateBookmarkTags(in []string) (string, error) {
	tags, err := domain.NormalizeTags(in)
	if err != nil {
		return "", newError(StatusBadRequest, "%s", err.Error())
	}
	return domain.EncodeTags(tags), nil
}

// ValidateTagFilter: tham số `tag` của `ListBookmarks`. Rỗng = không lọc.
func ValidateTagFilter(raw string) (string, error) {
	tag, err := domain.NormalizeTagFilter(raw)
	if err != nil {
		return "", newError(StatusBadRequest, "%s", err.Error())
	}
	return tag, nil
}

// ValidateMapCoord: toạ độ node bản đồ (migration 00004). nil = để server
// layout. Giá trị set phải nằm trong viewBox 0 0 1000 2000 — node ngoài canvas
// thì M6 không scroll tới được, mà DB vẫn nhận giá trị là lỗi âm thầm.
func ValidateMapCoord(v *float64, axisName string, max float64) (*float64, error) {
	if v == nil {
		return nil, nil
	}
	if *v < 0 || *v > max {
		return nil, newError(StatusBadRequest,
			"%s phải nằm trong 0..%s", axisName, strconv.FormatFloat(max, 'f', -1, 64))
	}
	out := *v
	return &out, nil
}
