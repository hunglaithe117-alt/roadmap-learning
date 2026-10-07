package roadmap

import "strings"

// Slugify converts a title into an ASCII slug [a-z0-9-] capped at 64 characters.
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if isSlugRune(r) {
			b.WriteRune(r)
			dash = false
			continue
		}
		if base, ok := latinBase[r]; ok {
			b.WriteRune(base)
			dash = false
			continue
		}
		if !dash {
			b.WriteRune('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 64 {
		out = strings.Trim(out[:64], "-")
	}
	return out
}

func isSlugRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

// latinBase maps accented Vietnamese and Latin-1 characters to base runes.
var latinBase = buildLatinBase()

func buildLatinBase() map[rune]rune {
	m := map[rune]rune{}
	add := func(base rune, chars string) {
		for _, c := range chars {
			m[c] = base
		}
	}
	add('a', "àáạảãâầấậẩẫăằắặẳẵ")
	add('a', "ÀÁẠẢÃÂẦẤẬẨẪĂẰẮẶẲẴ")
	add('e', "èéẹẻẽêềếệểễ")
	add('e', "ÈÉẸẺẼÊỀẾỆỂỄ")
	add('i', "ìíịỉĩ")
	add('i', "ÌÍỊỈĨ")
	add('o', "òóọỏõôồốộổỗơờớợởỡ")
	add('o', "ÒÓỌỎÕÔỒỐỘỔỖƠỜỚỢỞỠ")
	add('u', "ùúụủũưừứựửữ")
	add('u', "ÙÚỤỦŨƯỪỨỰỬỮ")
	add('y', "ỳýỵỷỹ")
	add('y', "ỲÝỴỶỸ")
	add('d', "đĐ")
	return m
}
