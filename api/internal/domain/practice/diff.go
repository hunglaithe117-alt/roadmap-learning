package practice

import (
	"strings"
	"unicode"

	"langapp/internal/domain/content"
)

// WordStatus là trạng thái 1 từ sau khi so transcript với câu mẫu.
type WordStatus string

const (
	// WordOK = từ khớp.
	WordOK WordStatus = "ok"
	// WordWrong = đọc sai (ghép từ thiếu + từ thừa liền kề: "the→a").
	WordWrong WordStatus = "wrong"
	// WordMissing = thiếu hẳn trong transcript.
	WordMissing WordStatus = "missing"
	// WordExtra = có thừa trong transcript.
	WordExtra WordStatus = "extra"
)

// DiffToken là 1 từ trong kết quả so khớp.
type DiffToken struct {
	Text   string
	Status WordStatus
}

// WordDiff so transcript (got) với câu mẫu (expected) bằng LCS trên token
// thường (lowercase, bỏ dấu câu 2 đầu) + đã chuẩn hóa phồn→giản trước khi
// so (whisper trả phồn còn deck mẫu dùng giản — không convert sẽ chấm sai
// oan; bảng dùng chung với content.ToSimplified).
//
// Hành vi port 1:1 từ web/src/player/diff.ts. Sau LCS, cặp missing+extra
// liền kề được gộp thành wrong (đọc sai từ đó, dễ đọc hơn 2 mảnh rời).
func WordDiff(expected, got string) []DiffToken {
	e := tokenize(content.ToSimplified(expected))
	g := tokenize(content.ToSimplified(got))
	en := make([]string, len(e))
	for i, t := range e {
		en[i] = norm(t)
	}
	gn := make([]string, len(g))
	for i, t := range g {
		gn[i] = norm(t)
	}
	m, n := len(en), len(gn)

	// dp[i][j] = độ dài LCS của en[i:] với gn[j:].
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if en[i] == gn[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}

	var raw []DiffToken
	i, j := 0, 0
	for i < m && j < n {
		switch {
		case en[i] == gn[j]:
			raw = append(raw, DiffToken{Text: e[i], Status: WordOK})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			raw = append(raw, DiffToken{Text: e[i], Status: WordMissing})
			i++
		default:
			raw = append(raw, DiffToken{Text: g[j], Status: WordExtra})
			j++
		}
	}
	for ; i < m; i++ {
		raw = append(raw, DiffToken{Text: e[i], Status: WordMissing})
	}
	for ; j < n; j++ {
		raw = append(raw, DiffToken{Text: g[j], Status: WordExtra})
	}
	return mergeAdjacent(raw)
}

// mergeAdjacent gộp cặp missing+extra liền kề thành 1 token wrong.
func mergeAdjacent(raw []DiffToken) []DiffToken {
	out := make([]DiffToken, 0, len(raw))
	for k := 0; k < len(raw); k++ {
		cur := raw[k]
		if nxt, ok := nextToken(raw, k); ok && cur.Status == WordMissing && nxt.Status == WordExtra {
			out = append(out, DiffToken{Text: cur.Text + "→" + nxt.Text, Status: WordWrong})
			k++
			continue
		}
		out = append(out, cur)
	}
	return out
}

func nextToken(in []DiffToken, k int) (DiffToken, bool) {
	if k+1 >= len(in) {
		return DiffToken{}, false
	}
	return in[k+1], true
}

// WrongWords là các từ sai (wrong + missing) để lưu sổ lỗi và bấm nghe lại
// TTS. Không lấy extra: từ thừa là lỗi của engine chứ không phải từ user
// cần ôn.
func WrongWords(diff []DiffToken) []string {
	out := make([]string, 0, len(diff))
	for _, t := range diff {
		if t.Status == WordWrong || t.Status == WordMissing {
			out = append(out, t.Text)
		}
	}
	return out
}

// DiffScore là tỉ lệ đúng = số token ok / tổng token mẫu. Câu mẫu rỗng -> 0
// (không chia 0).
func DiffScore(expected string, diff []DiffToken) float64 {
	total := len(tokenize(content.ToSimplified(expected)))
	if total == 0 {
		return 0
	}
	ok := 0
	for _, t := range diff {
		if t.Status == WordOK {
			ok++
		}
	}
	return float64(ok) / float64(total)
}

// Compare là bước chấm đầy đủ: diff + danh sách từ sai + điểm.
func Compare(expected, got string) (diff []DiffToken, wrong []string, score float64) {
	diff = WordDiff(expected, got)
	return diff, WrongWords(diff), DiffScore(expected, diff)
}

func tokenize(s string) []string {
	return strings.Fields(s)
}

// norm hạ chữ thường + bỏ dấu câu 2 đầu, để "Hello," khớp "hello".
func norm(t string) string {
	return strings.ToLower(strings.Trim(t, ".,!?;:\"'()“”‘’—–-"))
}

// NormalizeWrong chuẩn hóa danh sách từ sai ở CẢ 2 đầu (ghi và đọc):
// trim + hạ chữ thường từng từ, bỏ entry rỗng. Không bỏ entry trùng — sổ
// lỗi cần giữ số lần sai thật để TopErrors đếm được.
func NormalizeWrong(words []string) []string {
	out := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.ToLower(strings.TrimSpace(w))
		if w == "" {
			continue
		}
		out = append(out, w)
	}
	return out
}

// HasCJK báo chuỗi có ký tự Hán hay không — dùng để chọn cách so khớp cho
// câu tiếng Trung (không tách từ theo space).
func HasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
