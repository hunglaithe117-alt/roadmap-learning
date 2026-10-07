// Package content provides learning content domain logic: Chinese and English
// dictionaries, pinyin tone processing, sentence chunking, stress analysis, and THIEU checklist.
package content

// DictionaryEntry represents a Chinese dictionary entry.
type DictionaryEntry struct {
	ID     int64
	Hanzi  string
	Pinyin string
	Nghia  string
}

// EnglishEntry represents an English dictionary entry.
type EnglishEntry struct {
	ID      int64
	Lang    string
	Term    string
	Reading string
	Gloss   string
}

// ZHCard represents a Chinese study card with tone and audio metadata.
type ZHCard struct {
	ID          int64
	DeckID      int64
	Front       string
	Back        string
	Pinyin      string
	PinyinMarks string
	Tone        string
	AudioURL    string
	DueAt       string
	State       string
}

// DrillItem represents a tone drill question and its expected answer.
type DrillItem struct {
	CardID     int64
	Expected   string
	Syllables  int
	Tones      []int
	PinyinMark string
}

// TonesFromPinyin extracts tone numbers from numbered pinyin (e.g. "ni3 hao3" -> [3 3]).
// Returns false if any syllable lacks a tone number.
func TonesFromPinyin(pinyin string) ([]int, bool) {
	fields := splitFields(pinyin)
	if len(fields) == 0 {
		return nil, false
	}
	out := make([]int, 0, len(fields))
	for _, f := range fields {
		_, tone, ok := parseNumberedSyllable(f)
		if !ok {
			return nil, false
		}
		out = append(out, tone)
	}
	return out, true
}
