package content

import "strings"

// StressResult là kết quả tra trọng âm 1 từ.
type StressResult struct {
	Term string
	IPA  string
	// Stress là pattern in HOA âm tiết có trọng âm, nối bằng "-"
	// ("pho-TO-graph").
	Stress string
	// Exception = đây là ngoại lệ / quy tắc đoán, UI phải gắn nhãn "ngoại lệ"
	// và KHÔNG được coi là chắc chắn.
	Exception bool
	Note      string
	// Source là "dict" (tra từ điển) hoặc "rule" (suy ra từ quy tắc hậu tố).
	Source string
}

// exceptionWords đánh dấu headwords đa nghĩa: danh/tính từ nhấn âm đầu,
// động từ nhấn âm sau. Bộ quy tắc KHÔNG được đoán loại này — nó chỉ gắn cờ
// để UI hiện nhãn "ngoại lệ" và ưu tiên tra dict.
var exceptionWords = map[string]string{
	"progress": "ngoại lệ: danh từ /ˈprəʊɡres/, động từ /prəˈɡres/",
	"present":  "ngoại lệ: danh/tính từ /ˈpreznt/, động từ /prɪˈzent/",
	"record":   "ngoại lệ: danh từ /ˈrekɔːd/, động từ /rɪˈkɔːd/",
	"content":  "ngoại lệ: danh từ /ˈkɒntent/, tính từ /kənˈtent/ (hài lòng)",
	"desert":   "ngoại lệ: danh từ /ˈdezət/, động từ /dɪˈzɜːt/",
	"object":   "ngoại lệ: danh từ /ˈɒbdʒɪkt/, động từ /əbˈdʒekt/",
	"permit":   "ngoại lệ: danh từ /ˈpɜːmɪt/, động từ /pəˈmɪt/",
	"produce":  "ngoại lệ: danh từ /ˈprɒdjuːs/, động từ /prəˈdjuːs/",
	"refuse":   "ngoại lệ: danh từ /ˈrefjuːs/, động từ /rɪˈfjuːz/",
	"subject":  "ngoại lệ: danh từ /ˈsʌbdʒɪkt/, động từ /səbˈdʒekt/",
	"contrast": "ngoại lệ: danh từ /ˈkɒntrɑːst/, động từ /kənˈtrɑːst/",
	"increase": "ngoại lệ: danh từ /ˈɪnkriːs/, động từ /ɪnˈkriːs/",
	"import":   "ngoại lệ: danh từ /ˈɪmpɔːt/, động từ /ɪmˈpɔːt/",
	"export":   "ngoại lệ: danh từ /ˈekspɔːt/, động từ /ɪkˈspɔːt/",
}

// stressShifts là hậu tố khiến trọng âm lùi 1 âm tiết (danh từ phái sinh
// thường nhấn trước hậu tố: pho-TO-graphy).
var stressShifts = []string{"tion", "sion", "ic", "ical", "ity", "ogy", "graphy", "nomy", "meter"}

// ruleShiftSuffix là bộ hậu tố hẹp hơn dùng khi đoán từ chưa có trong dict.
var ruleShiftSuffix = []string{"tion", "sion", "ic", "ity", "ogy", "graphy"}

// StressFromIPA rút pattern in HOA từ IPA bằng cách đánh dấu âm tiết sau ˈ.
// Chỉ là trợ giúp hiển thị, KHÔNG phải bộ dự đoán — từ chưa biết rơi về
// LookupStressRule.
func StressFromIPA(term, ipa string) string {
	if !strings.Contains(ipa, "ˈ") {
		return term
	}
	sylls := splitSyllables(term)
	if len(sylls) <= 1 {
		return strings.ToUpper(term)
	}
	idx := primaryStressIndex(term, ipa, len(sylls))
	return joinStress(sylls, idx)
}

// LookupStressRule là fallback khi từ không có trong en_dict. Trả về pattern
// đoán + exception=true để UI LUÔN gắn nhãn "ngoại lệ / cần kiểm tra dict".
func LookupStressRule(word string) StressResult {
	key := strings.ToLower(strings.TrimSpace(word))
	if note, ok := exceptionWords[key]; ok {
		return StressResult{Term: word, IPA: "", Stress: "", Exception: true, Note: note, Source: SourceRule}
	}
	sylls := splitSyllables(key)
	stress := strings.ToUpper(key)
	if len(sylls) > 1 {
		idx := 0
		for _, suf := range ruleShiftSuffix {
			if strings.HasSuffix(key, suf) {
				idx = len(sylls) - 2
				break
			}
		}
		stress = joinStress(sylls, idx)
	}
	return StressResult{
		Term: word, IPA: "", Stress: stress, Exception: true,
		Note:   "ngoại lệ / quy tắc đoán — cần kiểm tra dict (chưa có trong en_dict)",
		Source: SourceRule,
	}
}

// Source của StressResult.
const (
	SourceDict = "dict"
	SourceRule = "rule"
)

// LookupStress ưu tiên dict (tra qua repository), fallback quy tắc.
func LookupStress(word string, e EnglishEntry, found bool) StressResult {
	key := strings.ToLower(strings.TrimSpace(word))
	if !found {
		return LookupStressRule(word)
	}
	note, exception := exceptionWords[key]
	return StressResult{
		Term: e.Term, IPA: e.Reading, Stress: StressFromIPA(e.Term, e.Reading),
		Exception: exception, Note: note, Source: SourceDict,
	}
}

// splitSyllables cắt từ theo nhóm nguyên âm liên tiếp (a, e, i, o, u, y).
func splitSyllables(word string) []string {
	lower := strings.ToLower(word)
	var sylls []string
	cur := ""
	vowels := "aeiouy"
	for i := 0; i < len(lower); i++ {
		cur += string(word[i])
		if !strings.ContainsRune(vowels, rune(lower[i])) {
			continue
		}
		j := i + 1
		for j < len(lower) && strings.ContainsRune(vowels, rune(lower[j])) {
			cur += string(word[j])
			j++
		}
		i = j - 1
		if j < len(lower) {
			sylls = append(sylls, cur)
			cur = ""
		}
	}
	if cur != "" {
		sylls = append(sylls, cur)
	}
	if len(sylls) == 0 {
		return []string{word}
	}
	return sylls
}

// primaryStressIndex ánh xạ vị trí ˈ trong IPA sang chỉ số âm tiết, dùng
// hậu tố làm tiebreaker.
func primaryStressIndex(term, ipa string, n int) int {
	pre := ipa
	if k := strings.Index(ipa, "ˈ"); k >= 0 {
		pre = ipa[:k]
	}
	// Dấu "ˌ" (nhấn phụ) đứng trước "ˈ" → trọng âm chính lùi 1 âm tiết so
	// với mặc định.
	if strings.Contains(pre, "ˌ") {
		return n - 2
	}
	for _, suf := range stressShifts {
		if strings.HasSuffix(strings.ToLower(term), suf) {
			if n >= 2 {
				return n - 2
			}
			return 0
		}
	}
	if n >= 3 {
		return n - 3
	}
	return 0
}

func joinStress(sylls []string, idx int) string {
	var b strings.Builder
	for i, s := range sylls {
		if i > 0 {
			b.WriteString("-")
		}
		if i == idx {
			b.WriteString(strings.ToUpper(s))
		} else {
			b.WriteString(strings.ToLower(s))
		}
	}
	return b.String()
}
