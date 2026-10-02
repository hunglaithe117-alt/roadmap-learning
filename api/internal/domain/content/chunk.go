package content

import "strings"

// ChunkKind phân loại từ trong chunking: content (từ nội dung, đọc mạnh) hoặc
// function (giới từ, mạo từ, trợ động từ — đọc nhẹ).
const (
	ChunkContent  = "content"
	ChunkFunction = "function"
)

// Chunk là 1 từ của câu đã tách, kèm loại content/function.
type Chunk struct {
	Text string
	Kind string
}

// functionWords là từ chức năng bị giảm trọng âm (mạo từ, giới từ, phó từ,
// trợ động từ, liên từ, dạng từ).
var functionWords = map[string]bool{
	"a": true, "an": true, "the": true,
	"i": true, "you": true, "he": true, "she": true, "it": true,
	"we": true, "they": true, "me": true, "him": true, "her": true,
	"us": true, "them": true, "my": true, "your": true, "his": true,
	"our": true, "their": true, "this": true, "that": true,
	"these": true, "those": true,
	"is": true, "am": true, "are": true, "was": true, "were": true,
	"be": true, "been": true, "being": true, "do": true, "does": true,
	"did": true, "have": true, "has": true, "had": true, "will": true,
	"would": true, "can": true, "could": true, "shall": true, "should": true,
	"may": true, "might": true, "must": true,
	"to": true, "of": true, "in": true, "on": true, "at": true,
	"for": true, "with": true, "by": true, "from": true, "as": true,
	"and": true, "or": true, "but": true, "so": true, "if": true,
	"because": true, "when": true, "while": true, "there": true,
	"not": true, "no": true, "up": true, "out": true, "about": true,
}

// SplitChunks tách câu thành content/function. Quy tắc cố ý đơn giản (so với
// danh sách từ chức năng): KHÔNG phân giải từ đa nghĩa (heteronym) hay ngữ
// cảnh cụm từ — caller hiển thị trọng âm dict cho headword.
func SplitChunks(sentence string) []Chunk {
	words := strings.Fields(sentence)
	out := make([]Chunk, 0, len(words))
	for _, w := range words {
		key := strings.ToLower(strings.Trim(w, ".,!?;:\"'()"))
		kind := ChunkContent
		if functionWords[key] {
			kind = ChunkFunction
		}
		out = append(out, Chunk{Text: w, Kind: kind})
	}
	return out
}

// ContentWords trả về danh sách từ nội dung — dùng để dựng câu drill PVO
// và đo tỉ lệ đúng trong diff.
func ContentWords(sentence string) []string {
	chunks := SplitChunks(sentence)
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if c.Kind == ChunkContent {
			out = append(out, c.Text)
		}
	}
	return out
}
