package practice

import (
	"context"
	"strings"
	"unicode"

	"langapp/internal/domain/content"
)

// WordStatus describes the alignment status of a word in transcription comparison.
type WordStatus string

const (
	// WordOK indicates a matched word.
	WordOK WordStatus = "ok"
	// WordWrong indicates an incorrect word substitution.
	WordWrong WordStatus = "wrong"
	// WordMissing indicates a missing expected word.
	WordMissing WordStatus = "missing"
	// WordExtra indicates an extraneous transcribed word.
	WordExtra WordStatus = "extra"
)

// DiffToken represents a single token in the comparison diff.
type DiffToken struct {
	Text   string
	Status WordStatus
}

// WordDiff compares transcribed text (got) against expected text using LCS token alignment.
func WordDiff(ctx context.Context, expected, got string) ([]DiffToken, error) {

	if err := ctx.Err(); err != nil {
		return nil, err
	}
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

	// dp[i][j] stores the LCS length of en[i:] and gn[j:].
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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
	return mergeAdjacent(raw), nil
}

// mergeAdjacent merges adjacent missing and extra tokens into a single wrong token.
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

// WrongWords extracts missing and wrong words from diff tokens.
func WrongWords(diff []DiffToken) []string {
	out := make([]string, 0, len(diff))
	for _, t := range diff {
		if t.Status == WordWrong || t.Status == WordMissing {
			out = append(out, t.Text)
		}
	}
	return out
}

// DiffScore computes accuracy as the ratio of matching tokens to total expected tokens.
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

// Compare computes the diff, error word list, and accuracy score.
func Compare(ctx context.Context, expected, got string) (diff []DiffToken, wrong []string, score float64, err error) {
	diff, err = WordDiff(ctx, expected, got)
	if err != nil {
		return nil, nil, 0, err
	}
	return diff, WrongWords(diff), DiffScore(expected, diff), nil
}

func tokenize(s string) []string {
	return strings.Fields(s)
}

// norm converts to lowercase and strips surrounding punctuation.
func norm(t string) string {
	return strings.ToLower(strings.Trim(t, ".,!?;:\"'()“”‘’—–-"))
}

// NormalizeWrong trims and lowercases error words, discarding empty strings.
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

// HasCJK reports whether s contains Han characters.
func HasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

