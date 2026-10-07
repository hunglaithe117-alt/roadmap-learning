package content

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_mark_syllable_placement_rules(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"hao3", "hǎo"},
		{"ni3", "nǐ"},
		{"ma5", "ma"}, // thanh trung tính: giữ base
		{"ma", "ma"},
		{"liu2", "liú"}, // iu -> dấu trên u
		{"gui4", "guì"}, // ui -> dấu trên i
		{"xue2", "xué"}, // üe -> dấu trên e
		{"lve4", "lüè"}, // "v" -> ü
		{"nv3", "nǚ"},   // "nv" -> nü
		{"jiu3", "jiǔ"}, // iu -> dấu trên u
		{"dou1", "dōu"}, // ou -> dấu trên o
		{"m2", "m"},     // không có nguyên âm nào -> giữ base
		{"ng2", "ng"},   // chỉ phụ âm -> giữ base
		{"n2", "n"},     //
		{"", ""},
	} {
		t.Run(c.in, func(t *testing.T) {
			assert.Equal(t, c.want, MarkSyllable(c.in), "input %q", c.in)
		})
	}
}

func Test_pinyin_marks_renders_whole_phrase(t *testing.T) {
	assert.Equal(t, "nǐ hǎo", PinyinMarks("ni3 hao3"))
	assert.Equal(t, "wǒ shì xué sheng", PinyinMarks("wo3 shi4 xue2 sheng"))
	assert.Equal(t, "", PinyinMarks(""))
}

func Test_parse_syllable_defaults_to_neutral_tone(t *testing.T) {
	assert.Equal(t, PinyinSyllable{Base: "hao", Tone: 3}, ParseSyllable("hao3"))
	assert.Equal(t, PinyinSyllable{Base: "ma", Tone: NeutralTone}, ParseSyllable("ma"))
	assert.Equal(t, PinyinSyllable{Base: "lü", Tone: 4}, ParseSyllable("lv4"))
}

func Test_tones_from_pinyin(t *testing.T) {
	tones, ok := TonesFromPinyin("ni3 hao3 ma5")
	require.True(t, ok)
	assert.Equal(t, []int{3, 3, 5}, tones)

	_, ok = TonesFromPinyin("ni hao")
	assert.False(t, ok, "âm tiết thiếu số thanh phải báo lỗi, không đoán")

	_, ok = TonesFromPinyin("")
	assert.False(t, ok)
}

func Test_valid_tone_pattern(t *testing.T) {
	for _, in := range []string{"3", "3 3", "1-4", "1,2,3", "5"} {
		t.Run(in, func(t *testing.T) {
			assert.True(t, ValidTonePattern(in), "input %q", in)
		})
	}
	for _, in := range []string{"", "0", "6", "3 3 3 3 3 3 3 3 3", "a", "33"} {
		t.Run(in, func(t *testing.T) {
			assert.False(t, ValidTonePattern(in), "input %q", in)
		})
	}
}

func Test_parse_tone_sequence_accepts_digits_and_pinyin(t *testing.T) {
	got, err := ParseToneSequence("3 hao3, 4")
	require.NoError(t, err)
	assert.Equal(t, []int{3, 3, 4}, got)

	_, err = ParseToneSequence("")
	require.Error(t, err)
	_, err = ParseToneSequence("hao")
	require.Error(t, err, "thiếu số thanh phải lỗi")
	_, err = ParseToneSequence("h3o")
	require.Error(t, err, "ký tự không phải chữ cái phải lỗi")
}

func Test_tone_label_joins_with_dash(t *testing.T) {
	assert.Equal(t, "3-3", ToneLabel([]int{3, 3}))
	assert.Equal(t, "1", ToneLabel([]int{1}))
	assert.Equal(t, "", ToneLabel(nil))
}

func Test_grade_tone_pair_grading_ladder(t *testing.T) {
	for _, c := range []struct {
		name             string
		expected, answer string
		wantGrade        int
		wantScore        float64
	}{
		{name: "exact pair", expected: "3 3", answer: "3 3", wantGrade: toneGradeEasy, wantScore: 1.0},
		{name: "half right", expected: "1 4", answer: "1 2", wantGrade: toneGradeGood, wantScore: 0.5},
		{name: "one of three", expected: "1 2 3", answer: "1 5 4", wantGrade: toneGradeHard, wantScore: 1.0 / 3.0},
		{name: "all wrong", expected: "1 4", answer: "2 1", wantGrade: toneGradeAgain, wantScore: 0.0},
		{name: "single exact", expected: "3", answer: "ni3", wantGrade: toneGradeEasy, wantScore: 1.0},
		{name: "single wrong", expected: "3", answer: "4", wantGrade: toneGradeAgain, wantScore: 0.0},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := GradeTonePair(c.expected, c.answer)
			require.NoError(t, err)
			assert.Equal(t, c.wantGrade, got.Grade)
			assert.InDelta(t, c.wantScore, got.Score, 1e-9)
			assert.Equal(t, c.wantScore == 1.0, got.Exact)
		})
	}
}

func Test_grade_tone_pair_rejects_length_mismatch(t *testing.T) {
	_, err := GradeTonePair("3 3", "3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "số âm tiết không khớp")
}

func Test_split_chunks_classifies_content_and_function(t *testing.T) {
	got := SplitChunks("I want to learn a new language.")
	require.Len(t, got, 7)
	assert.Equal(t, Chunk{Text: "I", Kind: ChunkFunction}, got[0])
	assert.Equal(t, Chunk{Text: "want", Kind: ChunkContent}, got[1])
	assert.Equal(t, Chunk{Text: "to", Kind: ChunkFunction}, got[2])
	assert.Equal(t, Chunk{Text: "language.", Kind: ChunkContent}, got[6])
}

func Test_content_words_drops_function_words(t *testing.T) {
	assert.Equal(t, []string{"want", "learn", "new", "language."}, ContentWords("I want to learn a new language."))
}

// StressFromIPA là trợ giúp HIỂN THỊ, không phải bộ dự đoán: nó đếm âm tiết
// theo nhóm nguyên âm rồi đặt dấu ˈ theo hậu tố. Kỳ vọng ở đây là hành vi
// thật của bộ đếm đó — từ nào muốn trọng âm chuẩn thì phải tra en_dict.
func Test_stress_from_dict_marks_stressed_syllable(t *testing.T) {
	for _, c := range []struct{ term, ipa, want string }{
		{term: "hi", ipa: "/ˈhaɪ/", want: "HI"},
		{term: "bird", ipa: "/bɜːd/", want: "bird"},
		{term: "photography", ipa: "/fəˈtɒɡrəfi/", want: "pho-to-GRA-phy"},
		{term: "diligent", ipa: "/ˈdɪlɪdʒənt/", want: "di-LI-ge-nt"},
		{term: "abandon", ipa: "/əˈbændən/", want: "a-BA-ndo-n"},
	} {
		t.Run(c.term, func(t *testing.T) {
			assert.Equal(t, c.want, StressFromIPA(c.term, c.ipa), "term %s", c.term)
		})
	}
}

func Test_lookup_stress_prefers_dict_and_flags_exceptions(t *testing.T) {
	entry := EnglishEntry{Term: "progress", Reading: "/ˈprəʊɡres/ (n) · /prəˈɡres/ (v)", Gloss: "tiến bộ"}
	got := LookupStress("progress", entry, true)
	assert.Equal(t, SourceDict, got.Source)
	assert.True(t, got.Exception, "từ đa nghĩa phải gắn cờ ngoại lệ")
	assert.Contains(t, got.Note, "ngoại lệ")

	fallback := LookupStress("photograph", EnglishEntry{}, false)
	assert.Equal(t, SourceRule, fallback.Source)
	assert.True(t, fallback.Exception, "quy tắc đoán luôn phải gắn cờ ngoại lệ")
}

// Bộ quy tắc hậu tố chỉ là ĐOÁN (luôn kèm exception=true) — kỳ vọng ở đây
// là hành vi thật của bộ tách âm tiết theo nhóm nguyên âm, không phải cách
// phát âm chuẩn. Giữ nguyên để không vô tình "sửa" logic đã port 1:1.
func Test_lookup_stress_rule_shifts_derived_nouns(t *testing.T) {
	plain := LookupStressRule("diligent")
	assert.Equal(t, "DI-li-ge-nt", plain.Stress, "không có hậu tố lùi -> nhấn âm tiết đầu")
	assert.True(t, plain.Exception, "mọi kết quả từ bộ quy tắc đều phải gắn cờ ngoại lệ")

	derived := LookupStressRule("photographic")
	assert.Equal(t, "pho-to-gra-PHI-c", derived.Stress, "hậu tố -ic lùi trọng âm 1 âm tiết")
	assert.True(t, derived.Exception)

	exception := LookupStressRule("record")
	assert.Equal(t, "", exception.Stress, "từ ngoại lệ không đoán, chỉ trả note")
	assert.Contains(t, exception.Note, "ngoại lệ")
}

func Test_thieu_avg_ignores_unscored_axes(t *testing.T) {
	assert.Equal(t, 3.0, THIEUAvg(map[string]int{"A": 3, "B": 3}), "chỉ 2 trục có điểm thì mẫu số là 2")
	assert.Equal(t, 0.0, THIEUAvg(nil), "không trục nào điểm -> 0 chứ không phải NaN")
	assert.Len(t, THIEUAxes, 8)
}

func Test_thieu_code_lookup(t *testing.T) {
	ax, ok := THIEUCode("A")
	require.True(t, ok)
	assert.Equal(t, "Trọng âm từ", ax.Name)
	_, ok = THIEUCode("Z")
	assert.False(t, ok)
}

func Test_to_simplified_is_idempotent(t *testing.T) {
	assert.Equal(t, "广东话", ToSimplified("廣東話"))
	assert.Equal(t, "广东话", ToSimplified("广东话"), "đã giản thì giữ nguyên")
	assert.Equal(t, "", ToSimplified(""))
	assert.Equal(t, "hello", ToSimplified("hello"), "chữ Latin không đổi")
	assert.True(t, HasTraditional("學習"))
	assert.False(t, HasTraditional("学习"))
}
