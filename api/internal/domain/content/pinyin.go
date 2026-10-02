package content

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// PinyinSyllable là 1 âm tiết pinyin: base không dấu + số thanh 1-5.
type PinyinSyllable struct {
	Base string
	// Tone là 1-4 cho 4 thanh, 5 cho thanh trung tính (không dấu).
	Tone int
}

// toneMarks ánh xạ nguyên âm gốc -> [trung tính, thanh1..thanh4].
var toneMarks = map[rune][5]rune{
	'a': {'a', 'ā', 'á', 'ǎ', 'à'},
	'e': {'e', 'ē', 'é', 'ě', 'è'},
	'i': {'i', 'ī', 'í', 'ǐ', 'ì'},
	'o': {'o', 'ō', 'ó', 'ǒ', 'ò'},
	'u': {'u', 'ū', 'ú', 'ǔ', 'ù'},
	'ü': {'ü', 'ǖ', 'ǘ', 'ǚ', 'ǜ'},
}

// NeutralTone là số thanh của thanh trung tính (không dấu).
const NeutralTone = 5

// NormalizeSyllable hạ chữ thường + đổi "u:"/"v" thành "ü" (2 cách gõ phổ
// biến của người Việt cho nguyên âu "ü").
func NormalizeSyllable(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "u:", "ü")
	s = strings.ReplaceAll(s, "v", "ü")
	return s
}

// ParseSyllable tách "hao3" -> ("hao", 3). Không có số cuối = thanh trung
// tính 5 (VD "ma").
func ParseSyllable(s string) PinyinSyllable {
	base, tone, ok := parseNumberedSyllable(s)
	if !ok {
		return PinyinSyllable{Base: base, Tone: NeutralTone}
	}
	return PinyinSyllable{Base: base, Tone: tone}
}

// parseNumberedSyllable là ParseSyllable cộng cờ "có số thanh 1-5 ở cuối
// hay không" — TonesFromPinyin cần cờ này vì pinyin thiếu số là lỗi dữ liệu,
// còn MarkSyllable coi đó là thanh trung tính.
func parseNumberedSyllable(s string) (base string, tone int, hasNumber bool) {
	s = NormalizeSyllable(s)
	if s == "" {
		return "", NeutralTone, false
	}
	last, _ := utf8.DecodeLastRuneInString(s)
	if last >= '1' && last <= '5' {
		return s[:len(s)-1], int(last - '0'), true
	}
	return s, NeutralTone, false
}

func isPinyinVowel(r rune) bool {
	switch r {
	case 'a', 'e', 'i', 'o', 'u', 'ü':
		return true
	}
	return false
}

// MarkSyllable vẽ dấu thanh cho 1 âm tiết có số ("hao3" -> "hǎo"). Thanh
// trung tính giữ nguyên base. Vị trí đặt dấu theo quy tắc chuẩn:
// a > e > ou > nguyên âm cuối (phủ iu->u, ui->i, üe->e).
func MarkSyllable(numbered string) string {
	syll := ParseSyllable(numbered)
	if syll.Base == "" || syll.Tone < 1 || syll.Tone > 4 {
		return syll.Base
	}
	rs := []rune(syll.Base)
	target := -1
	for i, r := range rs {
		if r == 'a' {
			target = i
			break
		}
	}
	if target < 0 {
		for i, r := range rs {
			if r == 'e' {
				target = i
				break
			}
		}
	}
	if target < 0 {
		for i := 0; i+1 < len(rs); i++ {
			if rs[i] == 'o' && rs[i+1] == 'u' {
				target = i
				break
			}
		}
	}
	if target < 0 {
		for i, r := range rs {
			if isPinyinVowel(r) {
				target = i // nguyên âm cuối thắng
			}
		}
	}
	if target < 0 {
		return syll.Base // không có nguyên âm ("m", "ng") — giữ nguyên
	}
	if marks, ok := toneMarks[rs[target]]; ok {
		rs[target] = marks[syll.Tone]
	}
	return string(rs)
}

// PinyinMarks vẽ dấu cho cả cụm ("ni3 hao3" -> "nǐ hǎo").
func PinyinMarks(numbered string) string {
	parts := splitFields(numbered)
	for i, p := range parts {
		parts[i] = MarkSyllable(p)
	}
	return strings.Join(parts, " ")
}

func splitFields(s string) []string { return strings.Fields(s) }

// TonePattern là dãy thanh của 1 thẻ, lưu ở cards.tone ("3", "3 3", "1-4").
type TonePattern []int

// ValidTonePattern báo chuỗi có phải pattern thuần (token 1 chữ số 1-5, phân
// cách bằng space/tab/'-'/',', tối đa 8 token) hay không.
func ValidTonePattern(s string) bool {
	toks := toneTokens(s)
	if len(toks) == 0 || len(toks) > 8 {
		return false
	}
	for _, t := range toks {
		if len(t) != 1 || t[0] < '1' || t[0] > '5' {
			return false
		}
	}
	return true
}

func toneTokens(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '-' || r == ','
	})
}

// ParseToneSequence biến input drill thành dãy số thanh. Mỗi token là số trần
// ("3") hoặc pinyin có số ("hao3"); phân tách bằng space/'-'/','.
func ParseToneSequence(s string) ([]int, error) {
	toks := strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || r == '-' || r == ','
	})
	if len(toks) == 0 {
		return nil, ErrTone("thiếu thanh điệu (VD: 3 3)")
	}
	out := make([]int, 0, len(toks))
	for _, t := range toks {
		t = NormalizeSyllable(t)
		if t == "" {
			return nil, ErrTone("âm tiết rỗng trong dãy thanh điệu")
		}
		last, _ := utf8.DecodeLastRuneInString(t)
		if last < '1' || last > '5' {
			return nil, ErrTone("âm tiết không hợp lệ: " + t + " (mỗi âm tiết cần số 1-5)")
		}
		if len(t) > 1 {
			base := t[:len(t)-1]
			if base == "" {
				return nil, ErrTone("âm tiết không hợp lệ: " + t)
			}
			for _, r := range base {
				if !unicode.IsLetter(r) {
					return nil, ErrTone("âm tiết không hợp lệ: " + t)
				}
			}
		}
		out = append(out, int(last-'0'))
	}
	return out, nil
}

// ToneLabel nối dãi thanh thành nhãn drill ("3-3").
func ToneLabel(tones []int) string {
	parts := make([]string, len(tones))
	for i, t := range tones {
		parts[i] = strconv.Itoa(t)
	}
	return strings.Join(parts, "-")
}

func itoa(n int) string { return strconv.Itoa(n) }
