package roadmap

import (
	"net/url"
	"strconv"
	"strings"

	domain "langapp/internal/domain/roadmap"
)

// ValidateSlug normalizes and validates a slug string.
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

// ValidateTitle normalizes and validates non-empty title strings up to 200 runes.
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

// ValidateText validates optional text up to max runes.
func ValidateText(s string, max int) (string, error) {
	t := strings.TrimSpace(s)
	if len([]rune(t)) > max {
		return "", newError(StatusBadRequest, "nội dung dài tối đa %s ký tự", strconv.Itoa(max))
	}
	return t, nil
}

// ValidateURL validates optional http or https URLs.
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

// ValidateKind validates optional resource kind against allowed kinds.
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

// AllKinds lists allowed resource kinds.
var AllKinds = []string{
	"video", "article", "tool", "app", "book", "course", "site", "podcast", "channel",
}

// ValidateLanguage validates language code up to 16 characters.
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

// ValidateStatus validates required status and optional note.
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

// ValidatePosition checks position is non-negative.
func ValidatePosition(p int) error {
	if p < 0 {
		return newError(StatusBadRequest, "position không được âm")
	}
	return nil
}

// ValidateDurationWeeks checks duration_weeks is non-negative.
func ValidateDurationWeeks(w int) error {
	if w < 0 {
		return newError(StatusBadRequest, "duration_weeks không được âm")
	}
	return nil
}

// ValidateIsOptional validates optional boolean flag (0 or 1).
func ValidateIsOptional(v int) (int, error) {
	if v == 0 || v == 1 {
		return v, nil
	}
	return 0, newError(StatusBadRequest, "is_optional chỉ nhận 0 hoặc 1")
}

// ValidateOptionalFlag validates optional boolean flag pointer.
func ValidateOptionalFlag(v *int) (int, error) {
	if v == nil {
		return 0, nil
	}
	return ValidateIsOptional(*v)
}

// ValidateBookmarkStatus validates bookmark status string.
func ValidateBookmarkStatus(raw string) (string, error) {
	s, err := domain.ParseBookmarkStatus(raw)
	if err != nil {
		return "", newError(StatusBadRequest, "%s", err.Error())
	}
	return string(s), nil
}

// ValidateBookmarkTags normalizes and encodes bookmark tags.
func ValidateBookmarkTags(in []string) (string, error) {
	tags, err := domain.NormalizeTags(in)
	if err != nil {
		return "", newError(StatusBadRequest, "%s", err.Error())
	}
	return domain.EncodeTags(tags), nil
}

// ValidateTagFilter normalizes the tag filter for bookmark listing.
func ValidateTagFilter(raw string) (string, error) {
	tag, err := domain.NormalizeTagFilter(raw)
	if err != nil {
		return "", newError(StatusBadRequest, "%s", err.Error())
	}
	return tag, nil
}

// ValidateMapCoord validates map coordinate bounds.
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
