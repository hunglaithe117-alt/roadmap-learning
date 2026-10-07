package graphql

import (
	"context"
	"time"

	contentapp "langapp/internal/application/content"
	practiceapp "langapp/internal/application/practice"
	"langapp/internal/transport/graphql/model"
)

const (
	contentDefaultLimit       = contentapp.DefaultSearchLimit
	practiceDefaultErrorLimit = practiceapp.DefaultErrorLimit
	insightDefaultTopErrors   = 20
	syncDefaultConflictLimit  = 50
)

const contentSourceDict = "dict"

// derefOr returns the dereferenced string or def if nil or empty.
func derefOr(p *string, def string) string {
	if p == nil || *p == "" {
		return def
	}
	return *p
}

// limitOr returns the limit value or def if nil or non-positive.
func limitOr(p *int, def int) int {
	if p == nil || *p <= 0 {
		return def
	}
	return *p
}

// parseIDPtr parses an optional string ID to an *int64.
func parseIDPtr(s *string) *int64 {
	if s == nil || *s == "" {
		return nil
	}
	v := parseID(*s)
	if v <= 0 {
		return nil
	}
	return &v
}

// nowUTC returns the current UTC time.
func nowUTC() time.Time { return time.Now().UTC() }

// ptrOf returns a pointer to the value.
func ptrOf[T any](v T) *T { return &v }

// loaders returns the request-scoped loaders from context.
func (r *Resolver) loaders(ctx context.Context) *Loaders {
	return LoadersFrom(ctx, r.Roadmap, r.SRS)
}

// ── input adapters ──────────────────────────────────────────────────────────

type contentImportInput struct{ Level, Deck string }

func (in contentImportInput) toApp() contentapp.ImportInput {
	return contentapp.ImportInput{Level: in.Level, Deck: in.Deck}
}

type contentThieuInput struct {
	Session string
	Scores  map[string]int
	Note    string
}

func (in contentThieuInput) toApp() contentapp.ThieuInput {
	return contentapp.ThieuInput{Session: in.Session, Scores: in.Scores, Note: in.Note}
}

func contentZHEntry(in model.DictEntryInput) contentapp.ZHEntry {
	return contentapp.ZHEntry{Hanzi: in.Hanzi, Pinyin: in.Pinyin, Nghia: in.Nghia}
}

type practiceErrorEntry = practiceapp.ErrorEntry
