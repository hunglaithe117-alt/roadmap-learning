package graphql

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	contentapp "langapp/internal/application/content"
	insightapp "langapp/internal/application/insight"
	practiceapp "langapp/internal/application/practice"
	roadmapapp "langapp/internal/application/roadmap"
	srsapp "langapp/internal/application/srs"
	syncapp "langapp/internal/application/sync"
	domaincontent "langapp/internal/domain/content"
	domainroadmap "langapp/internal/domain/roadmap"
	domainsync "langapp/internal/domain/sync"

	"langapp/internal/transport/graphql/model"
)

// ErrInvalidSince indicates the since parameter does not follow YYYY-MM-DD format.
var ErrInvalidSince = errors.New("since must be in YYYY-MM-DD format")

func idOf(v int64) string { return strconv.FormatInt(v, 10) }

func parseID(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

// ── enum ────────────────────────────────────────────────────────────────────

func statusOfNode(s string) model.Status {
	switch s {
	case roadmapapp.StatusInProgress:
		return model.StatusInProgress
	case roadmapapp.StatusDone:
		return model.StatusDone
	case roadmapapp.StatusSkipped:
		return model.StatusSkipped
	default:
		return model.StatusNotStarted
	}
}

// statusText maps a GraphQL Status enum back to application status string.
func statusText(s model.Status) string {
	switch s {
	case model.StatusInProgress:
		return roadmapapp.StatusInProgress
	case model.StatusDone:
		return roadmapapp.StatusDone
	case model.StatusSkipped:
		return roadmapapp.StatusSkipped
	default:
		return roadmapapp.StatusNotStarted
	}
}

// terrainOf converts a database terrain string to a GraphQL Terrain enum.
func terrainOf(s string) model.Terrain {
	switch domainroadmap.Terrain(s) {
	case domainroadmap.TerrainDesert:
		return model.TerrainDesert
	case domainroadmap.TerrainSnow:
		return model.TerrainSnow
	case domainroadmap.TerrainVolcano:
		return model.TerrainVolcano
	case domainroadmap.TerrainOcean:
		return model.TerrainOcean
	case domainroadmap.TerrainCity:
		return model.TerrainCity
	default:
		return model.TerrainMeadow
	}
}

// terrainText maps a GraphQL Terrain enum back to a database terrain string.
func terrainText(t *model.Terrain) (string, error) {
	if t == nil {
		return "", nil
	}
	switch *t {
	case model.TerrainDesert:
		return string(domainroadmap.TerrainDesert), nil
	case model.TerrainSnow:
		return string(domainroadmap.TerrainSnow), nil
	case model.TerrainVolcano:
		return string(domainroadmap.TerrainVolcano), nil
	case model.TerrainOcean:
		return string(domainroadmap.TerrainOcean), nil
	case model.TerrainCity:
		return string(domainroadmap.TerrainCity), nil
	case model.TerrainMeadow:
		return "", nil
	default:
		return "", errBadRequest(fmt.Errorf("terrain %q cannot be mapped to database value (valid values: %s)",
			string(*t), strings.Join(terrainNames(), ", ")))
	}
}

// terrainNames lists all valid terrain values for client error reporting.
func terrainNames() []string {
	out := make([]string, 0, len(domainroadmap.AllTerrains))
	for _, t := range domainroadmap.AllTerrains {
		out = append(out, string(t))
	}
	return out
}

func directionOf(s string) model.Direction {
	if s == "right" {
		return model.DirectionRight
	}
	return model.DirectionUp
}

// directionText maps a GraphQL Direction enum back to a database direction string.
func directionText(d *model.Direction) (string, error) {
	if d == nil || *d == model.DirectionUp {
		return "", nil
	}
	if *d == model.DirectionRight {
		return string(domainroadmap.DirectionRight), nil
	}
	return "", errBadRequest(fmt.Errorf("direction %q cannot be mapped to database value (valid values: %s, %s)",
		string(*d), domainroadmap.DirectionUp, domainroadmap.DirectionRight))
}

func resourceKindOf(s string) model.ResourceKind {
	switch s {
	case "video":
		return model.ResourceKindVideo
	case "article":
		return model.ResourceKindArticle
	case "tool":
		return model.ResourceKindTool
	case "app":
		return model.ResourceKindApp
	case "book":
		return model.ResourceKindBook
	case "course":
		return model.ResourceKindCourse
	case "site":
		return model.ResourceKindSite
	case "podcast":
		return model.ResourceKindPodcast
	case "channel":
		return model.ResourceKindChannel
	default:
		return model.ResourceKindVideo
	}
}

// resourceKindText trả "" cho `nil` (không gửi) hoặc `VIDEO` vì VIDEO là
// DEFAULT của cột — để `ValidateKind` gán default, khác vì gửi "video" cho mỗi
// tài liệu không phân loại là thêm dữ liệu vô nghĩa vào lịch sử merge.
func resourceKindText(k *model.ResourceKind) string {
	if k == nil || *k == model.ResourceKindVideo {
		return ""
	}
	return strings.ToLower(k.String())
}

func levelOf(s string) model.LevelState {
	switch s {
	case "done":
		return model.LevelStateDone
	case "locked":
		return model.LevelStateLocked
	default:
		return model.LevelStateCurrent
	}
}

func wordStatusOf(s string) model.WordStatus {
	switch s {
	case "wrong":
		return model.WordStatusWrong
	case "missing":
		return model.WordStatusMissing
	case "extra":
		return model.WordStatusExtra
	default:
		return model.WordStatusOk
	}
}

func chunkKindOf(s string) model.ChunkKind {
	if s == "function" {
		return model.ChunkKindFunction
	}
	return model.ChunkKindContent
}

// ── srs ─────────────────────────────────────────────────────────────────────

func deckView(d srsapp.Deck) model.Deck {
	return model.Deck{
		ID:        idOf(d.ID),
		GUID:      d.GUID,
		Name:      d.Name,
		Lang:      d.Lang,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
	}
}

func cardView(c srsapp.Card) model.Card {
	return model.Card{
		ID:         idOf(c.ID),
		GUID:       c.GUID,
		DeckID:     idOf(c.DeckID),
		Front:      c.Front,
		Back:       c.Back,
		Pinyin:     c.Pinyin,
		DueAt:      c.DueAt,
		Stability:  c.Stability,
		Difficulty: c.Difficulty,
		Reps:       c.Reps,
		Lapses:     c.Lapses,
		State:      c.State,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
		Tone:       c.Tone,
		Ipa:        c.IPA,
		Stress:     c.Stress,
		AudioURL:   c.AudioURL,
	}
}

func reviewView(r srsapp.ReviewResult) *model.ReviewResult {
	return &model.ReviewResult{
		CardID:       idOf(r.CardID),
		DueAt:        r.DueAt,
		IntervalDays: r.IntervalDays,
		Stability:    r.Stability,
		Difficulty:   r.Difficulty,
		Reps:         r.Reps,
		Fallback:     r.Fallback,
	}
}

// ── content ─────────────────────────────────────────────────────────────────

func dictViews(in []contentapp.ZHEntry) []model.DictEntry {
	out := make([]model.DictEntry, 0, len(in))
	for _, e := range in {
		out = append(out, model.DictEntry{Hanzi: e.Hanzi, Pinyin: e.Pinyin, Nghia: e.Nghia})
	}
	return out
}

func enViews(in []contentapp.ENEntry) []model.EnglishEntry {
	out := make([]model.EnglishEntry, 0, len(in))
	for _, e := range in {
		out = append(out, model.EnglishEntry{
			Lang: e.Lang, Term: e.Term, Reading: e.Reading, Gloss: e.Gloss,
		})
	}
	return out
}

func thieuAxesView(axes []domaincontent.THIEUAxis) []model.ThieuAxis {
	out := make([]model.ThieuAxis, 0, len(axes))
	for _, a := range axes {
		out = append(out, model.ThieuAxis{Code: a.Code, Name: a.Name, Desc: a.Desc})
	}
	return out
}

func thieuScoresView(scores map[string]int) []model.ThieuScore {
	keys := make([]string, 0, len(scores))
	for k := range scores {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]model.ThieuScore, 0, len(keys))
	for _, k := range keys {
		out = append(out, model.ThieuScore{Axis: k, Value: scores[k]})
	}
	return out
}

func thieuSessionView(s contentapp.ThieuSession) model.ThieuSession {
	return model.ThieuSession{
		ID:        idOf(s.ID),
		Session:   s.Session,
		Scores:    thieuScoresView(s.Scores),
		Average:   s.Average,
		Note:      s.Note,
		CreatedAt: s.CreatedAt,
	}
}

func importResultView(r contentapp.ImportResult) *model.ImportResult {
	return &model.ImportResult{
		DeckID:     idOf(r.DeckID),
		Deck:       r.Deck,
		Level:      r.Level,
		CardsAdded: r.CardsAdded,
		CardsTotal: r.CardsTotal,
		DictAdded:  r.DictAdded,
	}
}

func readerArticleViews(in []contentapp.StaticReaderArticle) []model.ReaderArticle {
	out := make([]model.ReaderArticle, 0, len(in))
	for _, a := range in {
		out = append(out, model.ReaderArticle{
			ID: a.ID, Level: a.Level, Lang: a.Lang,
			Title: a.Title, Text: a.Text, Source: a.Source,
		})
	}
	return out
}

func strokeInfoView(info contentapp.StaticStrokeInfo) *model.StrokeInfo {
	steps := make([]model.StrokeStep, 0, len(info.Strokes))
	for _, s := range info.Strokes {
		steps = append(steps, model.StrokeStep{Order: s.Order, Code: s.Code, Name: s.Name})
	}
	return &model.StrokeInfo{
		Hanzi:       info.Hanzi,
		PinyinMarks: info.PinyinMarks,
		Level:       info.Level,
		StrokeCount: info.StrokeCount,
		Steps:       steps,
	}
}

func strokeIndexViews(in []contentapp.StaticStrokeIndex) []model.StrokeIndexEntry {
	out := make([]model.StrokeIndexEntry, 0, len(in))
	for _, e := range in {
		out = append(out, model.StrokeIndexEntry{Hanzi: e.Hanzi, StrokeCount: e.StrokeCount})
	}
	return out
}

// ── roadmap ─────────────────────────────────────────────────────────────────

func pathView(p roadmapapp.Path) model.Path {
	return model.Path{
		ID:        idOf(p.ID),
		GUID:      p.GUID,
		Slug:      p.Slug,
		Title:     p.Title,
		Overview:  p.Overview,
		Language:  p.Language,
		IsBuiltin: p.IsBuiltin,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

func stageView(s roadmapapp.Stage) model.Stage {
	var deckID *string
	if s.DeckID != nil {
		v := idOf(*s.DeckID)
		deckID = &v
	}
	return model.Stage{
		ID:            idOf(s.ID),
		GUID:          s.GUID,
		PathID:        idOf(s.PathID),
		Slug:          s.Slug,
		Title:         s.Title,
		Goal:          s.Goal,
		Position:      s.Position,
		DurationWeeks: s.DurationWeeks,
		Status:        statusOfNode(s.Status),
		StatusNote:    s.StatusNote,
		CompletedAt:   s.CompletedAt,
		DeckID:        deckID,
		Terrain:       terrainOf(s.Terrain),
		Direction:     directionOf(s.Direction),
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}

func topicView(t roadmapapp.Topic) model.Topic {
	return model.Topic{
		ID:          idOf(t.ID),
		GUID:        t.GUID,
		StageID:     idOf(t.StageID),
		Title:       t.Title,
		Why:         t.Why,
		Activities:  t.Activities,
		Position:    t.Position,
		Status:      statusOfNode(t.Status),
		StatusNote:  t.StatusNote,
		CompletedAt: t.CompletedAt,
		IsOptional:  t.IsOptional == 1,
		MapX:        t.MapX,
		MapY:        t.MapY,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

func resourceView(r roadmapapp.Resource) model.Resource {
	return model.Resource{
		ID:        idOf(r.ID),
		GUID:      r.GUID,
		TopicID:   idOf(r.TopicID),
		Title:     r.Title,
		URL:       r.URL,
		Kind:      resourceKindOf(r.Kind),
		Note:      r.Note,
		Position:  r.Position,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func milestoneView(m roadmapapp.Milestone) model.Milestone {
	return model.Milestone{
		ID:        idOf(m.ID),
		GUID:      m.GUID,
		StageID:   idOf(m.StageID),
		Text:      m.Text,
		Position:  m.Position,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

// bookmarkStatusOf maps a database bookmark status string to a GraphQL BookmarkStatus enum.
func bookmarkStatusOf(s string) model.BookmarkStatus {
	switch s {
	case "reading":
		return model.BookmarkStatusReading
	case "done":
		return model.BookmarkStatusDone
	case "archived":
		return model.BookmarkStatusArchived
	default:
		return model.BookmarkStatusToRead
	}
}

// bookmarkStatusText maps a GraphQL BookmarkStatus enum back to a database bookmark status string.
func bookmarkStatusText(s *model.BookmarkStatus) (string, error) {
	if s == nil {
		return "", nil
	}
	switch *s {
	case model.BookmarkStatusToRead:
		return string(domainroadmap.BookmarkToRead), nil
	case model.BookmarkStatusReading:
		return string(domainroadmap.BookmarkReading), nil
	case model.BookmarkStatusDone:
		return string(domainroadmap.BookmarkDone), nil
	case model.BookmarkStatusArchived:
		return string(domainroadmap.BookmarkArchived), nil
	default:
		return "", errBadRequest(fmt.Errorf("bookmark status %q cannot be mapped to database value (valid values: %s)",
			string(*s), strings.Join(bookmarkStatusNames(), ", ")))
	}
}

// bookmarkStatusNames lists all valid bookmark statuses for client error reporting.
func bookmarkStatusNames() []string {
	out := make([]string, 0, len(domainroadmap.AllBookmarkStatuses))
	for _, s := range domainroadmap.AllBookmarkStatuses {
		out = append(out, string(s))
	}
	return out
}

func bookmarkView(b roadmapapp.Bookmark) model.Bookmark {
	return model.Bookmark{
		ID:        idOf(b.ID),
		GUID:      b.GUID,
		Title:     b.Title,
		URL:       b.URL,
		Note:      b.Note,
		Tags:      b.Tags,
		TagList:   decodeTags(b.Tags),
		Status:    bookmarkStatusOf(b.Status),
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}
}

func progressView(p roadmapapp.Progress) *model.Progress {
	return &model.Progress{
		Stages:           p.Stages,
		TopicsTotal:      p.TopicsTotal,
		TopicsRequired:   p.TopicsRequired,
		TopicsOptional:   p.TopicsOptional,
		TopicsDone:       p.TopicsDone,
		TopicsInProgress: p.TopicsInProgress,
		TopicsLocked:     p.TopicsLocked,
		Percent:          p.Percent,
		LastCompletedAt:  p.LastCompletedAt,
		CompletedInRange: p.CompletedInRange,
	}
}

func summaryView(s roadmapapp.Summary) *model.ProgressSummary {
	return &model.ProgressSummary{
		Stages:           s.Stages,
		TopicsTotal:      s.TopicsTotal,
		TopicsRequired:   s.TopicsRequired,
		TopicsDone:       s.TopicsDone,
		TopicsInProgress: s.TopicsInProgress,
		Percent:          s.Percent,
	}
}

func deckRefView(d *roadmapapp.DeckRef) *model.DeckRef {
	if d == nil {
		return nil
	}
	return &model.DeckRef{ID: idOf(d.ID), Name: d.Name, Lang: d.Lang}
}

// ── practice ────────────────────────────────────────────────────────────────

func shadowView(p practiceapp.ShadowProgress) model.ShadowProgress {
	return model.ShadowProgress{
		CardID:    idOf(p.CardID),
		Loops:     p.Loops,
		Rate:      p.Rate,
		UpdatedAt: p.UpdatedAt,
	}
}

func errorEntryViews(in []practiceapp.ErrorEntry) []model.ErrorEntry {
	out := make([]model.ErrorEntry, 0, len(in))
	for _, e := range in {
		var cardID *string
		if e.CardID != nil {
			v := idOf(*e.CardID)
			cardID = &v
		}
		out = append(out, model.ErrorEntry{
			ID:         idOf(e.ID),
			CardID:     cardID,
			Expected:   e.Expected,
			Transcript: e.Transcript,
			Wrong:      orEmpty(e.Wrong),
			CreatedAt:  e.CreatedAt,
		})
	}
	return out
}

func topErrorViews(in []practiceapp.TopErrorCount) []model.TopErrorCount {
	out := make([]model.TopErrorCount, 0, len(in))
	for _, e := range in {
		out = append(out, model.TopErrorCount{Word: e.Word, Count: e.Count})
	}
	return out
}

func cardErrorViews(in []practiceapp.CardErrorCount) []model.CardErrorCount {
	out := make([]model.CardErrorCount, 0, len(in))
	for _, e := range in {
		out = append(out, model.CardErrorCount{
			CardID: idOf(e.CardID), Front: e.Front, Back: e.Back, Errors: e.Errors,
		})
	}
	return out
}

func diffView(d practiceapp.DiffResult) *model.DiffResult {
	tokens := make([]model.DiffToken, 0, len(d.Diff))
	for _, t := range d.Diff {
		tokens = append(tokens, model.DiffToken{Text: t.Text, Status: wordStatusOf(string(t.Status))})
	}
	return &model.DiffResult{
		Transcript: d.Transcript,
		Diff:       tokens,
		Wrong:      orEmpty(d.Wrong),
		Score:      d.Score,
	}
}

// ── insight ─────────────────────────────────────────────────────────────────

func statsView(s insightapp.Stats) *model.Stats {
	return &model.Stats{
		Range: s.Range, Days: s.Days, Done: s.Done, Total: s.Total,
		DoneWindow: s.DoneWindow, TotalAll: s.TotalAll, DueNow: s.DueNow,
		Accuracy: s.Accuracy, Streak: s.Streak, Timezone: s.Timezone,
	}
}

// insightTopErrorViews giữ `cardId`/`front` NULL khi application trả nil — đó là
// trạng thái hợp lệ (lỗi luyện tự do không gắn thẻ), không phải thiếu dữ liệu.
func insightTopErrorViews(in []insightapp.TopError) []model.TopError {
	out := make([]model.TopError, 0, len(in))
	for _, e := range in {
		row := model.TopError{Word: e.Word, Count: e.Count}
		if e.CardID != nil {
			row.CardID = ptrOf(idOf(*e.CardID))
			row.Front = ptrOf(e.Front)
		}
		out = append(out, row)
	}
	return out
}

// ── sync ────────────────────────────────────────────────────────────────────

func syncStatusView(s syncapp.SyncStatus) *model.SyncStatus {
	return &model.SyncStatus{
		Enabled:       s.Enabled,
		Strategy:      s.Strategy,
		LastSyncAt:    s.LastSyncAt,
		ConflictCount: s.ConflictCount,
	}
}

func mergedView(m syncapp.Merged) *model.Merged {
	return &model.Merged{
		Decks:             m.Decks,
		Cards:             m.Cards,
		Reviews:           m.Reviews,
		Notes:             m.Notes,
		RoadmapPaths:      m.RoadmapPaths,
		RoadmapStages:     m.RoadmapStages,
		RoadmapMilestones: m.RoadmapMilestones,
		RoadmapTopics:     m.RoadmapTopics,
		RoadmapResources:  m.RoadmapResources,
		RoadmapBookmarks:  m.RoadmapBookmarks,
	}
}

func mergeResultView(r syncapp.MergeResult, err error) *model.MergeResult {
	out := &model.MergeResult{
		Ok:         err == nil,
		Merged:     mergedView(r.Merged),
		Conflicts:  conflictViews(r.Conflicts),
		Warnings:   orEmpty(r.Warnings),
		LastSyncAt: r.LastSyncAt,
		Error:      toUserError(err),
	}
	return out
}

// ── helpers ─────────────────────────────────────────────────────────────────

// decodeTags splits a CSV tag string into a string slice.
func decodeTags(csv string) []string { return domainroadmap.DecodeTags(csv) }

// decodeActivities unmarshals a JSON array of activity strings.
func decodeActivities(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{}
	}
	if out == nil {
		return []string{}
	}
	return out
}

// orEmpty returns an empty slice if the input is nil.
func orEmpty[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

// conflictViews maps domain conflicts into view models.
func conflictViews(in []domainsync.Conflict) []model.Conflict {
	out := make([]model.Conflict, 0, len(in))
	for _, c := range in {
		out = append(out, model.Conflict{
			GUID:       c.GUID,
			Table:      string(c.Table),
			Winner:     c.Winner,
			Detail:     c.Detail,
			ResolvedAt: c.At,
		})
	}
	return out
}

// parseSince parses a YYYY-MM-DD date string.
func parseSince(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, ErrInvalidSince
	}
	return &t, nil
}
