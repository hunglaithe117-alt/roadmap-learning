// Package content provides language content domain models, dictionary lookups, and pronunciation helpers.
package content

import "strings"

// Chunk classification kinds.
const (
	ChunkContent  = "content"
	ChunkFunction = "function"
)

// Chunk represents a tokenized word classified as content or function.
type Chunk struct {
	Text string
	Kind string
}

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

// SplitChunks divides a sentence into content and function word chunks.
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

// ContentWords extracts only content words from a sentence.
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

