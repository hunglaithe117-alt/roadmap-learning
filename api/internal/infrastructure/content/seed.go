package contentinfra

import (
	app "langapp/internal/application/content"
)

// SeedContent implements application/content.StaticContent using bundled seed data.
type SeedContent struct{}

// NewSeedContent constructs a SeedContent adapter.
func NewSeedContent() *SeedContent { return &SeedContent{} }

var _ app.StaticContent = (*SeedContent)(nil)

// HskSeedByLevel returns HSK seed entries for a level.
func (SeedContent) HskSeedByLevel(level string) []app.StaticHskEntry {
	rows := HskSeedByLevel(level)
	if rows == nil {
		return nil
	}
	out := make([]app.StaticHskEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, app.StaticHskEntry{
			Hanzi: r.Hanzi, Pinyin: r.Pinyin, Nghia: r.Nghia,
			Level: r.Level, Tone: r.Tone,
		})
	}
	return out
}

// EnDict returns bundled English dictionary entries.
func (SeedContent) EnDict() []app.StaticENEntry {
	rows := SeedEnDict()
	out := make([]app.StaticENEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, app.StaticENEntry{
			Lang: r.Lang, Term: r.Term, Reading: r.Reading, Gloss: r.Gloss,
		})
	}
	return out
}

// PVO returns bundled PVO seed cards.
func (SeedContent) PVO() []app.StaticSeedCard { return toStaticCards(SeedPVOCards()) }

// PVOT82 returns bundled PVOT82 seed cards.
func (SeedContent) PVOT82() []app.StaticSeedCard { return toStaticCards(SeedPVOCardsT82()) }

// TMRND returns bundled TMRND seed cards.
func (SeedContent) TMRND() []app.StaticSeedCard { return toStaticCards(SeedTMRNDCards()) }

// TMRNDT82 returns bundled TMRNDT82 seed cards.
func (SeedContent) TMRNDT82() []app.StaticSeedCard { return toStaticCards(SeedTMRNDCardsT82()) }

func toStaticCards(rows []SeedCard) []app.StaticSeedCard {
	out := make([]app.StaticSeedCard, 0, len(rows))
	for _, r := range rows {
		out = append(out, app.StaticSeedCard{
			Front: r.Front, Back: r.Back, IPA: r.IPA, Stress: r.Stress,
		})
	}
	return out
}

// StrokeIndex returns the stroke order summary index for a level.
func (SeedContent) StrokeIndex(level string) []app.StaticStrokeIndex {
	rows := StrokeIndex(level)
	out := make([]app.StaticStrokeIndex, 0, len(rows))
	for _, r := range rows {
		out = append(out, app.StaticStrokeIndex{Hanzi: r.Hanzi, StrokeCount: r.StrokeCount})
	}
	return out
}

// LookupStroke returns stroke order details for a character.
func (SeedContent) LookupStroke(level, hanzi string) (app.StaticStrokeInfo, bool) {
	info, ok := LookupStroke(level, hanzi)
	if !ok {
		return app.StaticStrokeInfo{}, false
	}
	steps := make([]app.StaticStrokeStep, 0, len(info.Strokes))
	for _, s := range info.Strokes {
		steps = append(steps, app.StaticStrokeStep{Order: s.Order, Code: s.Code, Name: s.Name})
	}
	return app.StaticStrokeInfo{
		Hanzi: info.Hanzi, PinyinMarks: info.PinyinMarks, Level: info.Level,
		StrokeCount: info.StrokeCount, Strokes: steps,
	}, true
}

// ReaderArticles filters reader articles by level and ID.
func (SeedContent) ReaderArticles(level, id string) []app.StaticReaderArticle {
	rows := ReaderArticles(level, id)
	out := make([]app.StaticReaderArticle, 0, len(rows))
	for _, r := range rows {
		out = append(out, app.StaticReaderArticle{
			ID: r.ID, Level: r.Level, Lang: r.Lang,
			Title: r.Title, Text: r.Text, Source: r.Source,
		})
	}
	return out
}
