package sync

import (
	"sort"
	"strconv"

	domain "langapp/internal/domain/sync"
)

// snapshot_rows.go converts domain.PeerSnapshot.Rows into typed application DTOs.

func v(m map[string]string, key string) string {
	if m == nil {
		return ""
	}
	return m[key]
}

func vp(m map[string]string, key string) *string {
	if m == nil {
		return nil
	}
	s, ok := m[key]
	if !ok {
		return nil
	}
	return &s
}

func vf(m map[string]string, key string) *float64 {
	if m == nil {
		return nil
	}
	s, ok := m[key]
	if !ok || s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

func vi(m map[string]string, key string) int {
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(v(m, key))
	if err != nil {
		return 0
	}
	return n
}

func itoa(n int) string { return strconv.Itoa(n) }

func ftoa(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

func sortRows(rows []domain.Row) []domain.Row {
	out := make([]domain.Row, len(rows))
	copy(out, rows)
	sort.Slice(out, func(i, j int) bool { return out[i].GUID < out[j].GUID })
	return out
}

func rowsOf(rows []domain.Row, table domain.Table) []domain.Row {
	out := make([]domain.Row, 0, len(rows))
	for _, r := range rows {
		if r.Table == table {
			out = append(out, r)
		}
	}
	return sortRows(out)
}

func deckRowsFrom(s domain.PeerSnapshot) []DeckRow {
	rows := rowsOf(s.Rows, domain.TableDecks)
	out := make([]DeckRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, DeckRow{
			GUID: r.GUID, Name: v(r.Values, "name"), Lang: v(r.Values, "lang"),
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Deleted: r.Deleted,
		})
	}
	return out
}

func cardRowsFrom(s domain.PeerSnapshot) []CardRow {
	rows := rowsOf(s.Rows, domain.TableCards)
	out := make([]CardRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, CardRow{
			GUID: r.GUID, DeckGUID: r.DeckGUID,
			Front: v(r.Values, "front"), Back: v(r.Values, "back"),
			Pinyin: v(r.Values, "pinyin"), DueAt: v(r.Values, "due_at"),
			Stability:  parseF(v(r.Values, "stability")),
			Difficulty: parseF(v(r.Values, "difficulty")),
			Reps:       vi(r.Values, "reps"), Lapses: vi(r.Values, "lapses"),
			State: v(r.Values, "state"),
			Tone:  vp(r.Values, "tone"), IPA: vp(r.Values, "ipa"),
			Stress: vp(r.Values, "stress"), AudioURL: vp(r.Values, "audio_url"),
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Deleted: r.Deleted,
		})
	}
	return out
}

func pathRowsFrom(s domain.PeerSnapshot) []PathRow {
	rows := rowsOf(s.Rows, domain.TableRoadmapPaths)
	out := make([]PathRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, PathRow{
			GUID: r.GUID, Slug: v(r.Values, "slug"), Language: v(r.Values, "language"),
			Title: v(r.Values, "title"), Overview: v(r.Values, "overview"),
			IsBuiltin: vi(r.Values, "is_builtin"),
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Deleted: r.Deleted,
		})
	}
	return out
}

func stageRowsFrom(s domain.PeerSnapshot) []StageRow {
	rows := rowsOf(s.Rows, domain.TableRoadmapStages)
	out := make([]StageRow, 0, len(rows))
	for _, r := range rows {
		deckGUID := r.DeckGUID
		out = append(out, StageRow{
			GUID: r.GUID, PathGUID: r.ParentGUID,
			Slug: v(r.Values, "slug"), Title: v(r.Values, "title"), Goal: v(r.Values, "goal"),
			Position: vi(r.Values, "position"), DurationWeeks: vi(r.Values, "duration_weeks"),
			Status: v(r.Values, "status"), StatusNote: v(r.Values, "status_note"),
			CompletedAt: vp(r.Values, "completed_at"),
			DeckGUID:    &deckGUID,
			Terrain:     v(r.Values, "terrain"), Direction: v(r.Values, "direction"),
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Deleted: r.Deleted,
		})
	}
	return out
}

func milestoneRowsFrom(s domain.PeerSnapshot) []MilestoneRow {
	rows := rowsOf(s.Rows, domain.TableRoadmapMilestones)
	out := make([]MilestoneRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, MilestoneRow{
			GUID: r.GUID, StageGUID: r.ParentGUID, Text: v(r.Values, "text"),
			Position:  vi(r.Values, "position"),
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Deleted: r.Deleted,
		})
	}
	return out
}

func topicRowsFrom(s domain.PeerSnapshot) []TopicRow {
	rows := rowsOf(s.Rows, domain.TableRoadmapTopics)
	out := make([]TopicRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, TopicRow{
			GUID: r.GUID, StageGUID: r.ParentGUID,
			Title: v(r.Values, "title"), Why: v(r.Values, "why"),
			Activities: v(r.Values, "activities"), Position: vi(r.Values, "position"),
			Status: v(r.Values, "status"), StatusNote: v(r.Values, "status_note"),
			CompletedAt: vp(r.Values, "completed_at"), IsOptional: vi(r.Values, "is_optional"),
			MapX: vf(r.Values, "map_x"), MapY: vf(r.Values, "map_y"),
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Deleted: r.Deleted,
		})
	}
	return out
}

func resourceRowsFrom(s domain.PeerSnapshot) []ResourceRow {
	rows := rowsOf(s.Rows, domain.TableRoadmapResources)
	out := make([]ResourceRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, ResourceRow{
			GUID: r.GUID, TopicGUID: r.ParentGUID,
			Title: v(r.Values, "title"), URL: vp(r.Values, "url"),
			Kind: v(r.Values, "kind"), Note: v(r.Values, "note"),
			Position:  vi(r.Values, "position"),
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Deleted: r.Deleted,
		})
	}
	return out
}

func bookmarkRowsFrom(s domain.PeerSnapshot) []BookmarkRow {
	rows := rowsOf(s.Rows, domain.TableRoadmapBookmarks)
	out := make([]BookmarkRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, BookmarkRow{
			GUID: r.GUID, Title: v(r.Values, "title"), URL: vp(r.Values, "url"),
			Note: v(r.Values, "note"), Tags: v(r.Values, "tags"), Status: v(r.Values, "status"),
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Deleted: r.Deleted,
		})
	}
	return out
}

func reviewRowsFrom(s domain.PeerSnapshot) []ReviewRow {
	rows := rowsOf(s.Rows, domain.TableReviews)
	out := make([]ReviewRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, ReviewRow{
			GUID: r.GUID, CardGUID: r.CardGUID, Grade: vi(r.Values, "grade"),
			ReviewedAt: v(r.Values, "reviewed_at"), NextDueAt: v(r.Values, "next_due_at"),
		})
	}
	return out
}

func noteRowsFrom(s domain.PeerSnapshot) []NoteRow {
	rows := rowsOf(s.Rows, domain.TableNotes)
	out := make([]NoteRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, NoteRow{
			GUID: r.GUID, CardGUID: r.CardGUID, Text: v(r.Values, "text"),
			CreatedAt: r.CreatedAt,
		})
	}
	return out
}

func parseF(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}
