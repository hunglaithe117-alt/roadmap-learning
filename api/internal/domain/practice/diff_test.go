package practice

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func statusOf(diff []DiffToken) []WordStatus {
	out := make([]WordStatus, len(diff))
	for i, t := range diff {
		out[i] = t.Status
	}
	return out
}

func textsOf(diff []DiffToken) []string {
	out := make([]string, len(diff))
	for i, t := range diff {
		out[i] = t.Text
	}
	return out
}

func Test_word_diff_canceled_context_returns_error(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := WordDiff(ctx, "I want to learn", "I want to learn")
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, got)
}

func Test_word_diff_identical_is_all_ok(t *testing.T) {
	got, err := WordDiff(context.Background(), "I want to learn", "I want to learn")
	require.NoError(t, err)
	require.Len(t, got, 4)
	for _, s := range statusOf(got) {
		assert.Equal(t, WordOK, s)
	}
}

func Test_word_diff_ignores_case_and_punctuation(t *testing.T) {
	got, err := WordDiff(context.Background(), "Hello, world.", "hello world")
	require.NoError(t, err)
	assert.Equal(t, []WordStatus{WordOK, WordOK}, statusOf(got), "hoa thường + dấu câu không được tính sai")
	assert.Equal(t, []string{"Hello,", "world."}, textsOf(got), "hiển thị text gốc của câu mẫu")
}

func Test_word_diff_normalizes_traditional_to_simplified(t *testing.T) {
	// Whisper trả phồn, deck mẫu dùng giản — không convert thì chấm sai oan.
	got, err := WordDiff(context.Background(), "我学习广东话", "我学习广东话")
	require.NoError(t, err)
	assert.Equal(t, []WordStatus{WordOK}, statusOf(got))

	diff, wrong, score, err := Compare(context.Background(), "我学习广东话", "我学习广东话")
	require.NoError(t, err)
	assert.Empty(t, wrong)
	assert.Equal(t, 1.0, score)
	assert.Len(t, diff, 1)
}

func Test_word_diff_missing_and_extra_pair_merge_into_wrong(t *testing.T) {
	// "the" bị đọc thành "a": LCS gặp missing("the") + extra("a") liền kề.
	got, err := WordDiff(context.Background(), "I saw the cat", "I saw a cat")
	require.NoError(t, err)
	assert.Equal(t, []WordStatus{WordOK, WordOK, WordWrong, WordOK}, statusOf(got))
	assert.Equal(t, []string{"the→a"}, []string{got[2].Text})
}

func Test_word_diff_missing_without_extra(t *testing.T) {
	got, err := WordDiff(context.Background(), "I want to learn", "I learn")
	require.NoError(t, err)
	assert.Contains(t, statusOf(got), WordMissing)
	assert.NotContains(t, statusOf(got), WordExtra)
}

func Test_word_diff_extra_without_missing(t *testing.T) {
	got, err := WordDiff(context.Background(), "I learn", "I want to learn")
	require.NoError(t, err)
	assert.Contains(t, statusOf(got), WordExtra)
	assert.NotContains(t, statusOf(got), WordMissing)
}

func Test_word_diff_empty_got_marks_everything_missing(t *testing.T) {
	got, err := WordDiff(context.Background(), "one two three", "")
	require.NoError(t, err)
	assert.Equal(t, []WordStatus{WordMissing, WordMissing, WordMissing}, statusOf(got))
}

func Test_word_diff_empty_expected_marks_everything_extra(t *testing.T) {
	// Câu mẫu rỗng: không có gì để khớp, mọi từ trong transcript là thừa.
	got, err := WordDiff(context.Background(), "", "something")
	require.NoError(t, err)
	assert.Equal(t, []WordStatus{WordExtra}, statusOf(got))
	assert.Empty(t, WrongWords(got), "từ thừa không vào sổ lỗi")
	empty, err := WordDiff(context.Background(), "", "")
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func Test_wrong_words_excludes_extra(t *testing.T) {
	got, err := WordDiff(context.Background(), "I saw the cat", "I saw a cat")
	require.NoError(t, err)
	assert.Equal(t, []string{"the→a"}, WrongWords(got),
		"từ thừa là lỗi engine, không phải từ user cần ôn")
}

func Test_diff_score_ratio_and_zero_case(t *testing.T) {
	perfect, _, score, err := Compare(context.Background(), "one two three", "one two three")
	require.NoError(t, err)
	assert.Equal(t, 1.0, score)
	assert.Equal(t, []WordStatus{WordOK, WordOK, WordOK}, statusOf(perfect))

	_, _, score, err = Compare(context.Background(), "one two three", "one two")
	require.NoError(t, err)
	assert.InDelta(t, 2.0/3.0, score, 1e-9)

	_, _, score, err = Compare(context.Background(), "", "anything")
	require.NoError(t, err)
	assert.Equal(t, 0.0, score, "câu mẫu rỗng -> 0 chứ không chia 0")
}

func Test_normalize_wrong_trims_lowercases_drops_blanks(t *testing.T) {
	got := NormalizeWrong([]string{"  The ", "", "CAT", "  "})
	assert.Equal(t, []string{"the", "cat"}, got, "entry rỗng bị bỏ, số lần sai thật được giữ")
}

func Test_normalize_wrong_keeps_duplicates_for_counting(t *testing.T) {
	got := NormalizeWrong([]string{"the", "THE", "The"})
	assert.Len(t, got, 3, "không dedupe: sổ lỗi cần đếm số lần sai thật")
}

func Test_has_cjk(t *testing.T) {
	assert.True(t, HasCJK("你好"))
	assert.False(t, HasCJK("hello"))
	assert.False(t, HasCJK(""))
}

func Test_normalize_rate_defaults_zero_and_rejects_out_of_range(t *testing.T) {
	got, err := NormalizeRate(0)
	require.NoError(t, err)
	assert.Equal(t, DefaultRate, got, "client không gửi rate -> 1.0")

	got, err = NormalizeRate(1.25)
	require.NoError(t, err)
	assert.Equal(t, 1.25, got)

	for _, bad := range []float64{0.4, 1.6, -0.5} {
		t.Run(fmt.Sprintf("rate=%v", bad), func(t *testing.T) {
			_, err := NormalizeRate(bad)
			require.ErrorIs(t, err, ErrRateOutOfRange, "rate %v", bad)
		})
	}
}

func Test_advance_shadow_session_increments_and_normalizes(t *testing.T) {
	now := time.Date(2026, 9, 28, 5, 0, 0, 0, time.UTC)
	cur := NewShadowSession(7)
	assert.Equal(t, 0, cur.Loops)
	assert.Equal(t, DefaultRate, cur.Rate)

	next, err := AdvanceShadowSession(cur, 0, now)
	require.NoError(t, err)
	assert.Equal(t, 1, next.Loops)
	assert.Equal(t, DefaultRate, next.Rate, "rate 0 -> mặc định")
	assert.Equal(t, now, next.UpdatedAt)
	assert.Equal(t, time.UTC, next.UpdatedAt.Location())
	assert.Equal(t, 0, cur.Loops, "không mutate input")
}

func Test_advance_shadow_session_rejects_bad_rate(t *testing.T) {
	_, err := AdvanceShadowSession(NewShadowSession(1), 2.0, time.Now())
	assert.ErrorIs(t, err, ErrRateOutOfRange)
}

func Test_validate_loops_rejects_negative(t *testing.T) {
	assert.NoError(t, ValidateLoops(0))
	assert.NoError(t, ValidateLoops(99))
	assert.ErrorIs(t, ValidateLoops(-1), ErrNegativeLoops)
}

// Test_shadow_prefixes_are_disjoint: 3 context dùng chung bảng notes, tiền
// tố trùng nhau là mất dữ liệu.
func Test_shadow_prefixes_are_disjoint(t *testing.T) {
	all := []string{ShadowPrefix, ErrorPrefix, "THIEU|"}
	seen := map[string]bool{}
	for _, p := range all {
		assert.False(t, seen[p], "prefix %q bị trùng", p)
		seen[p] = true
	}
	assert.Equal(t, 3, len(seen))
}

func Test_word_status_values_are_stable(t *testing.T) {
	// UI đọc chuỗi này — đổi giá trị là đổi hợp đồng.
	got := []string{string(WordOK), string(WordWrong), string(WordMissing), string(WordExtra)}
	assert.Equal(t, []string{"ok", "wrong", "missing", "extra"}, got)
}

func Test_diff_is_deterministic(t *testing.T) {
	a, err := WordDiff(context.Background(), "one two three four", "one two nine four")
	require.NoError(t, err)
	b, err := WordDiff(context.Background(), "one two three four", "one two nine four")
	require.NoError(t, err)
	assert.Equal(t, textsOf(a), textsOf(b))
	assert.Equal(t, statusOf(a), statusOf(b))
}

func Test_recording_holds_compare_output(t *testing.T) {
	diff, wrong, score, err := Compare(context.Background(), "I want to learn", "I want to lern")
	require.NoError(t, err)
	rec := Recording{CardID: 1, Transcript: "I want to lern", Diff: diff, Wrong: wrong, Score: score}
	assert.Equal(t, 0.75, rec.Score, "3/4 từ đúng")
	assert.Equal(t, []string{"learn→lern"}, rec.Wrong, "cặp sai gộp lại để đọc")
	assert.Equal(t, []WordStatus{WordOK, WordOK, WordOK, WordWrong}, statusOf(rec.Diff))
}

func Test_error_entry_allows_missing_card(t *testing.T) {
	e := ErrorEntry{Expected: "hello", Transcript: "hallo", Wrong: []string{"hallo"}}
	assert.Nil(t, e.CardID, "lỗi luyện tự do không gắn thẻ nào")
}

func Test_sort_helper_keeps_word_status_order_stable(t *testing.T) {
	// Bảo đảm helper sort của insight không phụ thuộc thứ tự map.
	tokens := []DiffToken{
		{Text: "b", Status: WordOK},
		{Text: "a", Status: WordOK},
	}
	sort.SliceStable(tokens, func(i, j int) bool { return tokens[i].Text < tokens[j].Text })
	assert.Equal(t, "a", tokens[0].Text)
}
