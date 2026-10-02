// Package content chứa bounded context nội dung học: từ điển Trung/Anh, bộ
// thanh điệu Pinyin, chunking câu, trọng âm và checklist THIEU.
//
// Port từ api/chinese.go + api/english.go v1. Ở đây KHÔNG có SQL — dict/en_dict
// được đọc qua repository (M3).
package content

// DictionaryEntry là 1 dòng dict: chữ Hán, pinyin có số thanh, nghĩa tiếng
// Việt. Tương đương cột hanzi/pinyin/nghia.
type DictionaryEntry struct {
	ID     int64
	Hanzi  string
	Pinyin string // "ni3 hao3" (có số thanh), chưa dấu
	Nghia  string
}

// EnglishEntry là 1 dòng en_dict: lang, headword, IPA, gloss.
type EnglishEntry struct {
	ID      int64
	Lang    string
	Term    string
	Reading string // IPA, có thể chứa 2 cách đọc "(n) · (v)"
	Gloss   string
}

// ZHCard là thẻ học Trung kèm trường do context này sở hữu (tone, audio_url).
// PinyinMarks được tính lúc đọc, không lưu DB.
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

// DrillItem là 1 câu hỏi drill thanh điệu: mẫu (pinyin có số) + đáp án.
type DrillItem struct {
	CardID     int64
	Expected   string
	Syllables  int
	Tones      []int
	PinyinMark string
}

// TonesFromPinyin rút dãy số thanh từ pinyin ("ni3 hao3" -> [3 3]). ok=false
// nếu bất kỳ âm tiết nào không có số.
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
