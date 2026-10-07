package contentinfra

import (
	"sort"

	"langapp/internal/domain/content"
)

// StrokeStep represents a stroke in the standard stroke order.
type StrokeStep struct {
	Order int
	Code  string
	Name  string
}

// StrokeInfo represents stroke order details for a Chinese character.
type StrokeInfo struct {
	Hanzi       string
	PinyinMarks string
	Level       string
	StrokeCount int
	Strokes     []StrokeStep
}

// strokeIndexItem represents a lightweight summary item for lazy-loading.
type strokeIndexItem struct {
	Hanzi       string
	StrokeCount int
}

var strokeSeed = map[string]StrokeInfo{}

func init() {
	for _, s := range []StrokeInfo{
		stroke("人", "ren2", "HSK1", [][2]string{{"p", "phẩy (撇)"}, {"n", "mác (捺)"}}),
		stroke("大", "da4", "HSK1", [][2]string{{"h", "ngang (横)"}, {"p", "phẩy (撇)"}, {"n", "mác (捺)"}}),
		stroke("小", "xiao3", "HSK1", [][2]string{{"sg", "sổ móc (竖钩)"}, {"d", "chấm (点)"}, {"d", "chấm (点)"}}),
		stroke("中", "zhong1", "HSK1", [][2]string{{"s", "sổ (竖)"}, {"hz", "gập ngang (横折)"}, {"h", "ngang (横)"}, {"s", "sổ (竖)"}}),
		stroke("日", "ri4", "HSK1", [][2]string{{"s", "sổ (竖)"}, {"hz", "gập ngang (横折)"}, {"h", "ngang (横)"}, {"h", "ngang (横)"}}),
		stroke("月", "yue4", "HSK1", [][2]string{{"p", "phẩy (撇)"}, {"hz", "gập ngang (横折)"}, {"h", "ngang (横)"}, {"h", "ngang (横)"}}),
		stroke("水", "shui3", "HSK1", [][2]string{{"sg", "sổ móc (竖钩)"}, {"h", "ngang (横)"}, {"p", "phẩy (撇)"}, {"n", "mác (捺)"}}),
		stroke("女", "nü3", "HSK1", [][2]string{{"pd", "phẩy-chấm (撇点)"}, {"p", "phẩy (撇)"}, {"h", "ngang (横)"}}),
		stroke("子", "zi3", "HSK1", [][2]string{{"h", "ngang (横)"}, {"sg", "sổ móc (竖钩)"}, {"h", "ngang (横)"}}),
		stroke("好", "hao3", "HSK1", [][2]string{{"pd", "phẩy-chấm (撇点)"}, {"p", "phẩy (撇)"}, {"h", "ngang (横)"}, {"h", "ngang (横)"}, {"sg", "sổ móc (竖钩)"}, {"h", "ngang (横)"}}),
		stroke("生", "sheng1", "HSK1", [][2]string{{"p", "phẩy (撇)"}, {"h", "ngang (横)"}, {"h", "ngang (横)"}, {"s", "sổ (竖)"}, {"h", "ngang (横)"}}),
		stroke("天", "tian1", "HSK1", [][2]string{{"h", "ngang (横)"}, {"h", "ngang (横)"}, {"p", "phẩy (撇)"}, {"n", "mác (捺)"}}),
		stroke("千", "qian1", "HSK2", [][2]string{{"p", "phẩy (撇)"}, {"h", "ngang (横)"}, {"s", "sổ (竖)"}}),
		stroke("门", "men2", "HSK2", [][2]string{{"d", "chấm (点)"}, {"s", "sổ (竖)"}, {"hz", "gập ngang (横折)"}}),
		stroke("万", "wan4", "HSK3", [][2]string{{"h", "ngang (横)"}, {"hz", "gập ngang (横折)"}, {"p", "phẩy (撇)"}}),
		stroke("刀", "dao1", "HSK4", [][2]string{{"hz", "gập ngang (横折)"}, {"p", "phẩy (撇)"}}),
	} {
		strokeSeed[s.Hanzi] = s
	}
}

// stroke builds a StrokeInfo entry.
func stroke(hanzi, pinyin, level string, codes [][2]string) StrokeInfo {
	steps := make([]StrokeStep, len(codes))
	for i, c := range codes {
		steps[i] = StrokeStep{Order: i + 1, Code: c[0], Name: c[1]}
	}
	return StrokeInfo{Hanzi: hanzi, PinyinMarks: content.PinyinMarks(pinyin),
		Level: level, StrokeCount: len(steps), Strokes: steps}
}

// StrokeIndex returns a lightweight stroke index for the given level.
func StrokeIndex(level string) []strokeIndexItem {
	var out []strokeIndexItem
	for _, s := range strokeSeed {
		if s.Level == level {
			out = append(out, strokeIndexItem{Hanzi: s.Hanzi, StrokeCount: s.StrokeCount})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hanzi < out[j].Hanzi })
	return out
}

// LookupStroke returns stroke order details for a character in the given level.
func LookupStroke(level, hanzi string) (StrokeInfo, bool) {
	s, ok := strokeSeed[hanzi]
	if !ok || s.Level != level {
		return StrokeInfo{}, false
	}
	return s, true
}
