package contentinfra

import (
	app "langapp/internal/application/content"
)

// SeedContent là hiện thực `application/content.StaticContent` trên dữ liệu
// bundle trong package này (HSK, en_dict, PVO/TMRND, bút thuận, bài đọc).
//
// Dữ liệu tĩnh đặt ở infrastructure (không phải application) là để giữ tầng
// application chỉ còn interface + quy tắc điều phối — đúng vị trí M2 đã đặt
// loader seed roadmap.
type SeedContent struct{}

// NewSeedContent dựng adapter dữ liệu tĩnh.
func NewSeedContent() *SeedContent { return &SeedContent{} }

var _ app.StaticContent = (*SeedContent)(nil)

// HskSeedByLevel chuyển seed HSK của infrastructure sang DTO của application.
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

// EnDict chuyển từ điển tích hợp sang DTO của application.
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

// PVO, PVOT82, TMRND, TMRNDT82 là 4 nhóm thẻ seed tiếng Anh của v1.
func (SeedContent) PVO() []app.StaticSeedCard      { return toStaticCards(SeedPVOCards()) }
func (SeedContent) PVOT82() []app.StaticSeedCard   { return toStaticCards(SeedPVOCardsT82()) }
func (SeedContent) TMRND() []app.StaticSeedCard    { return toStaticCards(SeedTMRNDCards()) }
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

// StrokeIndex trả index nhẹ bút thuận của 1 level.
func (SeedContent) StrokeIndex(level string) []app.StaticStrokeIndex {
	rows := StrokeIndex(level)
	out := make([]app.StaticStrokeIndex, 0, len(rows))
	for _, r := range rows {
		out = append(out, app.StaticStrokeIndex{Hanzi: r.Hanzi, StrokeCount: r.StrokeCount})
	}
	return out
}

// LookupStroke trả chi tiết bút thuận 1 chữ.
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

// ReaderArticles lọc bài đọc theo level + id.
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
