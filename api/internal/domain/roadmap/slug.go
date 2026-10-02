package roadmap

import "strings"

// Slugify biến tiêu đề tiếng Việt/Anh/Trung thành slug [a-z0-9-]:
//  1. bỏ dấu tiếng Việt + Latin-1 ("tự học" → "tu hoc")
//  2. ký tự a-z/0-9 giữ nguyên; còn lại (chữ Trung, ký tự đặc biệt) gộp
//     thành 1 dấu '-'
//  3. cắt '-' đầu/cuối, cắt còn tối đa 64 ký tự (giới hạn của ValidateSlug ở
//     application/roadmap).
//
// Bảng bỏ dấu tự chứa thay vì `unicode/norm` — GOROOT của máy build này bị cắt
// bớt stdlib (thiếu package `norm`), và thêm dependency cho việc slugify là
// không đáng. Port nguyên vẹn từ `api/roadmap_seed.go` v1.
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

// latinBase: ký tự có dấu → ký tự gốc. Đủ 10 nhóm nguyên âm tiếng Việt
// (a ă â e ê i o ô ơ u ư y, mỗi nhóm 5 sắc/huyền/hỏi/ngã/nặng) + đ/Đ.
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
