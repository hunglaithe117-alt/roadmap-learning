package content

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// PinyinSyllable represents a single pinyin syllable with base and tone (1-5).
type PinyinSyllable struct {
	Base string
	Tone int
}

var toneMarks = map[rune][5]rune{
	'a': {'a', 'ā', 'á', 'ǎ', 'à'},
	'e': {'e', 'ē', 'é', 'ě', 'è'},
	'i': {'i', 'ī', 'í', 'ǐ', 'ì'},
	'o': {'o', 'ō', 'ó', 'ǒ', 'ò'},
	'u': {'u', 'ū', 'ú', 'ǔ', 'ù'},
	'ü': {'ü', 'ǖ', 'ǘ', 'ǚ', 'ǜ'},
}

// NeutralTone denotes the neutral tone number (5).
const NeutralTone = 5

// NormalizeSyllable lowercases and normalizes "u:"/"v" to "ü".
func NormalizeSyllable(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "u:", "ü")
	s = strings.ReplaceAll(s, "v", "ü")
	return s
}

// ParseSyllable parses a syllable into base and tone number (defaulting to NeutralTone).
func ParseSyllable(s string) PinyinSyllable {
	base, tone, ok := parseNumberedSyllable(s)
	if !ok {
		return PinyinSyllable{Base: base, Tone: NeutralTone}
	}
	return PinyinSyllable{Base: base, Tone: tone}
}

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

// MarkSyllable adds tone diacritics to a numbered pinyin syllable (e.g. "hao3" -> "hǎo").
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
				target = i
			}
		}
	}
	if target < 0 {
		return syll.Base
	}
	if marks, ok := toneMarks[rs[target]]; ok {
		rs[target] = marks[syll.Tone]
	}
	return string(rs)
}

// PinyinMarks applies tone diacritics to spaced numbered pinyin (e.g. "ni3 hao3" -> "nǐ hǎo").
func PinyinMarks(numbered string) string {
	parts := splitFields(numbered)
	for i, p := range parts {
		parts[i] = MarkSyllable(p)
	}
	return strings.Join(parts, " ")
}

func splitFields(s string) []string { return strings.Fields(s) }

// TonePattern represents the tone sequence of a card.
type TonePattern []int

// ValidTonePattern reports whether s consists of 1-8 tone digits (1-5).
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

// ParseToneSequence parses input into tone numbers. Each token can be a bare digit or numbered pinyin.
func ParseToneSequence(s string) ([]int, error) {
	toks := strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || r == '-' || r == ','
	})
	if len(toks) == 0 {
		return nil, ToneError("thiếu thanh điệu (VD: 3 3)")
	}
	out := make([]int, 0, len(toks))
	for _, t := range toks {
		t = NormalizeSyllable(t)
		if t == "" {
			return nil, ToneError("âm tiết rỗng trong dãy thanh điệu")
		}
		last, _ := utf8.DecodeLastRuneInString(t)
		if last < '1' || last > '5' {
			return nil, ToneError("âm tiết không hợp lệ: " + t + " (mỗi âm tiết cần số 1-5)")
		}
		if len(t) > 1 {
			base := t[:len(t)-1]
			if base == "" {
				return nil, ToneError("âm tiết không hợp lệ: " + t)
			}
			for _, r := range base {
				if !unicode.IsLetter(r) {
					return nil, ToneError("âm tiết không hợp lệ: " + t)
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
