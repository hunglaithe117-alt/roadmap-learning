package syncinfra

import (
	"context"
	"fmt"

	app "langapp/internal/application/sync"
)

// Write implementation of merge for roadmap trees and append-only tables.

// ── roadmap_paths ───────────────────────────────────────────────────────────

type pathMergeRow struct {
	ID        int64
	Slug      string
	Language  string
	Title     string
	Overview  string
	IsBuiltin int
	CreatedAt string
	GUID      string
	UpdatedAt string
	Deleted   int
}

// TableName returns the table name for pathMergeRow.
func (pathMergeRow) TableName() string { return "roadmap_paths" }

// PathRows returns all roadmap paths by GUID.
func (r *Repository) PathRows(ctx context.Context, tx app.Tx) (map[string]app.PathRow, error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return nil, err
	}
	var rows []pathMergeRow
	if err := db.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc roadmap_paths: %w", err)
	}
	out := make(map[string]app.PathRow, len(rows))
	for _, row := range rows {
		if row.GUID == "" {
			continue
		}
		out[row.GUID] = app.PathRow{
			ID: row.ID, GUID: row.GUID, Slug: row.Slug, Language: row.Language,
			Title: row.Title, Overview: row.Overview, IsBuiltin: row.IsBuiltin,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Deleted: row.Deleted,
		}
	}
	return out, nil
}

// UpsertPath inserts or updates a roadmap path.
func (r *Repository) UpsertPath(ctx context.Context, tx app.Tx, p app.PathRow, found bool) (id int64, skipped bool, err error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return 0, false, err
	}
	if !found {
		var id int64
		err := db.Raw(`INSERT INTO roadmap_paths
			(slug, language, title, overview, is_builtin, created_at, guid, updated_at, deleted)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
		RETURNING id`,
			p.Slug, p.Language, p.Title, p.Overview, p.IsBuiltin, p.CreatedAt,
			p.GUID, p.UpdatedAt, p.Deleted).Scan(&id).Error
		return id, id == 0, err
	}
	if p.ID <= 0 {
		return 0, false, fmt.Errorf("update roadmap_paths %s nhưng thiếu id local", p.GUID)
	}
	err = db.Exec(`UPDATE roadmap_paths SET slug = ?, language = ?, title = ?,
		overview = ?, is_builtin = ?, deleted = ?, updated_at = ? WHERE id = ?`,
		p.Slug, p.Language, p.Title, p.Overview, p.IsBuiltin, p.Deleted, p.UpdatedAt, p.ID).Error
	return p.ID, false, err
}

// ── roadmap_stages ──────────────────────────────────────────────────────────

type stageMergeRow struct {
	ID            int64
	PathID        int64
	Slug          string
	Title         string
	Goal          string
	Position      int
	DurationWeeks int
	Status        string
	StatusNote    string
	CompletedAt   *string
	DeckID        *int64
	Terrain       string
	Direction     string
	CreatedAt     string
	GUID          string
	UpdatedAt     string
	Deleted       int
}

// TableName returns the table name for stageMergeRow.
func (stageMergeRow) TableName() string { return "roadmap_stages" }

// StageRows returns all roadmap stages by GUID.
func (r *Repository) StageRows(ctx context.Context, tx app.Tx) (map[string]app.StageRow, error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return nil, err
	}
	var rows []stageMergeRow
	if err := db.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc roadmap_stages: %w", err)
	}
	pathGUID, err := r.guidByID(ctx, tx, "roadmap_paths")
	if err != nil {
		return nil, err
	}
	deckGUID, err := r.deckGUIDByID(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]app.StageRow, len(rows))
	for _, row := range rows {
		if row.GUID == "" {
			continue
		}
		s := app.StageRow{
			ID: row.ID, GUID: row.GUID, PathGUID: pathGUID[row.PathID],
			Slug: row.Slug, Title: row.Title, Goal: row.Goal,
			Position: row.Position, DurationWeeks: row.DurationWeeks,
			Status: row.Status, StatusNote: row.StatusNote, CompletedAt: row.CompletedAt,
			Terrain: row.Terrain, Direction: row.Direction,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Deleted: row.Deleted,
		}
		s.DeckGUID = new(string)
		if row.DeckID != nil {
			if g, ok := deckGUID[*row.DeckID]; ok {
				*s.DeckGUID = g
			}
		}
		out[row.GUID] = s
	}
	return out, nil
}

// UpsertStage inserts or updates a roadmap stage.
func (r *Repository) UpsertStage(ctx context.Context, tx app.Tx, s app.StageRow, found bool) (id int64, skipped bool, err error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return 0, false, err
	}
	pathID, err := r.resolve(ctx, db, "roadmap_paths", s.PathGUID)
	if err != nil {
		return 0, false, fmt.Errorf("resolve path cha của stage %s: %w", s.GUID, err)
	}
	var deckID *int64
	if s.DeckGUID != nil && *s.DeckGUID != "" {
		d, err := r.resolve(ctx, db, "decks", *s.DeckGUID)
		if err != nil {
			return 0, false, fmt.Errorf("resolve deck của stage %s: %w", s.GUID, err)
		}
		deckID = &d
	}
	if !found {
		var id int64
		err := db.Raw(`INSERT INTO roadmap_stages
			(path_id, slug, title, goal, position, duration_weeks, status, status_note,
			 created_at, guid, updated_at, deleted, completed_at, deck_id, terrain, direction)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
		RETURNING id`,
			pathID, s.Slug, s.Title, s.Goal, s.Position, s.DurationWeeks, s.Status, s.StatusNote,
			s.CreatedAt, s.GUID, s.UpdatedAt, s.Deleted, s.CompletedAt, deckID,
			s.Terrain, s.Direction).Scan(&id).Error
		return id, id == 0, err
	}
	if s.ID <= 0 {
		return 0, false, fmt.Errorf("update roadmap_stages %s nhưng thiếu id local", s.GUID)
	}
	if s.DeckGUID == nil {
		err = db.Exec(`UPDATE roadmap_stages SET path_id = ?, slug = ?, title = ?, goal = ?,
			position = ?, duration_weeks = ?, status = ?, status_note = ?, deleted = ?,
			updated_at = ?, completed_at = ?, terrain = ?, direction = ?
			WHERE id = ?`,
			pathID, s.Slug, s.Title, s.Goal, s.Position, s.DurationWeeks, s.Status, s.StatusNote,
			s.Deleted, s.UpdatedAt, s.CompletedAt, s.Terrain, s.Direction, s.ID).Error
		return s.ID, false, err
	}
	err = db.Exec(`UPDATE roadmap_stages SET path_id = ?, slug = ?, title = ?, goal = ?,
		position = ?, duration_weeks = ?, status = ?, status_note = ?, deleted = ?,
		updated_at = ?, completed_at = ?, deck_id = ?, terrain = ?, direction = ?
		WHERE id = ?`,
		pathID, s.Slug, s.Title, s.Goal, s.Position, s.DurationWeeks, s.Status, s.StatusNote,
		s.Deleted, s.UpdatedAt, s.CompletedAt, deckID, s.Terrain, s.Direction, s.ID).Error
	return s.ID, false, err
}

// ── roadmap_milestones ──────────────────────────────────────────────────────

type milestoneMergeRow struct {
	ID        int64
	StageID   int64
	Text      string
	Position  int
	CreatedAt string
	GUID      string
	UpdatedAt string
	Deleted   int
}

// TableName returns the table name for milestoneMergeRow.
func (milestoneMergeRow) TableName() string { return "roadmap_milestones" }

// MilestoneRows returns all roadmap milestones by GUID.
func (r *Repository) MilestoneRows(ctx context.Context, tx app.Tx) (map[string]app.MilestoneRow, error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return nil, err
	}
	var rows []milestoneMergeRow
	if err := db.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc roadmap_milestones: %w", err)
	}
	stageGUID, err := r.guidByID(ctx, tx, "roadmap_stages")
	if err != nil {
		return nil, err
	}
	out := make(map[string]app.MilestoneRow, len(rows))
	for _, row := range rows {
		if row.GUID == "" {
			continue
		}
		out[row.GUID] = app.MilestoneRow{
			ID: row.ID, GUID: row.GUID, StageGUID: stageGUID[row.StageID], Text: row.Text,
			Position: row.Position, CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt, Deleted: row.Deleted,
		}
	}
	return out, nil
}

// UpsertMilestone inserts or updates a roadmap milestone.
func (r *Repository) UpsertMilestone(ctx context.Context, tx app.Tx, m app.MilestoneRow, found bool) (id int64, skipped bool, err error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return 0, false, err
	}
	stageID, err := r.resolve(ctx, db, "roadmap_stages", m.StageGUID)
	if err != nil {
		return 0, false, fmt.Errorf("resolve stage cha của milestone %s: %w", m.GUID, err)
	}
	if !found {
		var id int64
		err := db.Raw(`INSERT INTO roadmap_milestones
			(stage_id, text, position, created_at, guid, updated_at, deleted)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
		RETURNING id`,
			stageID, m.Text, m.Position, m.CreatedAt, m.GUID, m.UpdatedAt, m.Deleted).Scan(&id).Error
		return id, id == 0, err
	}
	if m.ID <= 0 {
		return 0, false, fmt.Errorf("update roadmap_milestones %s nhưng thiếu id local", m.GUID)
	}
	err = db.Exec(`UPDATE roadmap_milestones SET stage_id = ?, text = ?, position = ?,
		deleted = ?, updated_at = ? WHERE id = ?`,
		stageID, m.Text, m.Position, m.Deleted, m.UpdatedAt, m.ID).Error
	return m.ID, false, err
}

// ── roadmap_topics ──────────────────────────────────────────────────────────

type topicMergeRow struct {
	ID          int64
	StageID     int64
	Title       string
	Why         string
	Activities  string
	Position    int
	Status      string
	StatusNote  string
	CompletedAt *string
	IsOptional  int
	MapX        *float64
	MapY        *float64
	CreatedAt   string
	GUID        string
	UpdatedAt   string
	Deleted     int
}

// TableName returns the table name for topicMergeRow.
func (topicMergeRow) TableName() string { return "roadmap_topics" }

// TopicRows returns all roadmap topics by GUID.
func (r *Repository) TopicRows(ctx context.Context, tx app.Tx) (map[string]app.TopicRow, error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return nil, err
	}
	var rows []topicMergeRow
	if err := db.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc roadmap_topics: %w", err)
	}
	stageGUID, err := r.guidByID(ctx, tx, "roadmap_stages")
	if err != nil {
		return nil, err
	}
	out := make(map[string]app.TopicRow, len(rows))
	for _, row := range rows {
		if row.GUID == "" {
			continue
		}
		out[row.GUID] = app.TopicRow{
			ID: row.ID, GUID: row.GUID, StageGUID: stageGUID[row.StageID],
			Title: row.Title, Why: row.Why, Activities: row.Activities,
			Position: row.Position, Status: row.Status, StatusNote: row.StatusNote,
			CompletedAt: row.CompletedAt, IsOptional: row.IsOptional,
			MapX: row.MapX, MapY: row.MapY,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Deleted: row.Deleted,
		}
	}
	return out, nil
}

// UpsertTopic inserts or updates a roadmap topic.
func (r *Repository) UpsertTopic(ctx context.Context, tx app.Tx, tp app.TopicRow, found bool) (id int64, skipped bool, err error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return 0, false, err
	}
	stageID, err := r.resolve(ctx, db, "roadmap_stages", tp.StageGUID)
	if err != nil {
		return 0, false, fmt.Errorf("resolve stage cha của topic %s: %w", tp.GUID, err)
	}
	if !found {
		var id int64
		err := db.Raw(`INSERT INTO roadmap_topics
			(stage_id, title, why, activities, position, status, status_note, created_at,
			 guid, updated_at, deleted, completed_at, is_optional, map_x, map_y)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
		RETURNING id`,
			stageID, tp.Title, tp.Why, tp.Activities, tp.Position, tp.Status, tp.StatusNote,
			tp.CreatedAt, tp.GUID, tp.UpdatedAt, tp.Deleted, tp.CompletedAt, tp.IsOptional,
			tp.MapX, tp.MapY).Scan(&id).Error
		return id, id == 0, err
	}
	if tp.ID <= 0 {
		return 0, false, fmt.Errorf("update roadmap_topics %s nhưng thiếu id local", tp.GUID)
	}
	err = db.Exec(`UPDATE roadmap_topics SET stage_id = ?, title = ?, why = ?, activities = ?,
		position = ?, status = ?, status_note = ?, deleted = ?, updated_at = ?,
		completed_at = ?, is_optional = ?, map_x = ?, map_y = ? WHERE id = ?`,
		stageID, tp.Title, tp.Why, tp.Activities, tp.Position, tp.Status, tp.StatusNote,
		tp.Deleted, tp.UpdatedAt, tp.CompletedAt, tp.IsOptional, tp.MapX, tp.MapY, tp.ID).Error
	return tp.ID, false, err
}

// ── roadmap_resources ───────────────────────────────────────────────────────

type resourceMergeRow struct {
	ID        int64
	TopicID   int64
	Title     string
	URL       *string
	Kind      string
	Note      string
	Position  int
	CreatedAt string
	GUID      string
	UpdatedAt string
	Deleted   int
}

// TableName returns the table name for resourceMergeRow.
func (resourceMergeRow) TableName() string { return "roadmap_resources" }

// ResourceRows returns all roadmap resources by GUID.
func (r *Repository) ResourceRows(ctx context.Context, tx app.Tx) (map[string]app.ResourceRow, error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return nil, err
	}
	var rows []resourceMergeRow
	if err := db.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc roadmap_resources: %w", err)
	}
	topicGUID, err := r.guidByID(ctx, tx, "roadmap_topics")
	if err != nil {
		return nil, err
	}
	out := make(map[string]app.ResourceRow, len(rows))
	for _, row := range rows {
		if row.GUID == "" {
			continue
		}
		out[row.GUID] = app.ResourceRow{
			ID: row.ID, GUID: row.GUID, TopicGUID: topicGUID[row.TopicID], Title: row.Title,
			URL: row.URL, Kind: row.Kind, Note: row.Note, Position: row.Position,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Deleted: row.Deleted,
		}
	}
	return out, nil
}

// UpsertResource inserts or updates a roadmap resource.
func (r *Repository) UpsertResource(ctx context.Context, tx app.Tx, res app.ResourceRow, found bool) (id int64, skipped bool, err error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return 0, false, err
	}
	topicID, err := r.resolve(ctx, db, "roadmap_topics", res.TopicGUID)
	if err != nil {
		return 0, false, fmt.Errorf("resolve topic cha của resource %s: %w", res.GUID, err)
	}
	if !found {
		var id int64
		err := db.Raw(`INSERT INTO roadmap_resources
			(topic_id, title, url, kind, note, position, created_at, guid, updated_at, deleted)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
		RETURNING id`,
			topicID, res.Title, res.URL, res.Kind, res.Note, res.Position, res.CreatedAt,
			res.GUID, res.UpdatedAt, res.Deleted).Scan(&id).Error
		return id, id == 0, err
	}
	if res.ID <= 0 {
		return 0, false, fmt.Errorf("update roadmap_resources %s nhưng thiếu id local", res.GUID)
	}
	err = db.Exec(`UPDATE roadmap_resources SET topic_id = ?, title = ?, url = ?, kind = ?,
		note = ?, position = ?, deleted = ?, updated_at = ? WHERE id = ?`,
		topicID, res.Title, res.URL, res.Kind, res.Note, res.Position,
		res.Deleted, res.UpdatedAt, res.ID).Error
	return res.ID, false, err
}

// ── roadmap_bookmarks ───────────────────────────────────────────────────────

type bookmarkMergeRow struct {
	ID        int64
	Title     string
	URL       *string
	Note      string
	Tags      string
	Status    string
	CreatedAt string
	GUID      string
	UpdatedAt string
	Deleted   int
}

// TableName returns the table name for bookmarkMergeRow.
func (bookmarkMergeRow) TableName() string { return "roadmap_bookmarks" }

// BookmarkRows returns all roadmap bookmarks by GUID.
func (r *Repository) BookmarkRows(ctx context.Context, tx app.Tx) (map[string]app.BookmarkRow, error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return nil, err
	}
	var rows []bookmarkMergeRow
	if err := db.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc roadmap_bookmarks: %w", err)
	}
	out := make(map[string]app.BookmarkRow, len(rows))
	for _, row := range rows {
		if row.GUID == "" {
			continue
		}
		out[row.GUID] = app.BookmarkRow{
			ID: row.ID, GUID: row.GUID, Title: row.Title, URL: row.URL, Note: row.Note,
			Tags: row.Tags, Status: row.Status, CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt, Deleted: row.Deleted,
		}
	}
	return out, nil
}

// UpsertBookmark inserts or updates a roadmap bookmark.
func (r *Repository) UpsertBookmark(ctx context.Context, tx app.Tx, b app.BookmarkRow, found bool) (id int64, skipped bool, err error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return 0, false, err
	}
	if !found {
		var id int64
		err := db.Raw(`INSERT INTO roadmap_bookmarks
			(title, url, note, tags, status, created_at, guid, updated_at, deleted)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
		RETURNING id`,
			b.Title, b.URL, b.Note, b.Tags, b.Status, b.CreatedAt, b.GUID,
			b.UpdatedAt, b.Deleted).Scan(&id).Error
		return id, id == 0, err
	}
	if b.ID <= 0 {
		return 0, false, fmt.Errorf("update roadmap_bookmarks %s nhưng thiếu id local", b.GUID)
	}
	err = db.Exec(`UPDATE roadmap_bookmarks SET title = ?, url = ?, note = ?, tags = ?,
		status = ?, deleted = ?, updated_at = ? WHERE id = ?`,
		b.Title, b.URL, b.Note, b.Tags, b.Status, b.Deleted, b.UpdatedAt, b.ID).Error
	return b.ID, false, err
}

// ── reviews / notes (append-only) ───────────────────────────────────────────

// ReviewGUIDs returns the set of existing review GUIDs.
func (r *Repository) ReviewGUIDs(ctx context.Context, tx app.Tx) (map[string]bool, error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return nil, err
	}
	var rows []struct{ GUID string }
	if err := db.Table("reviews").
		Where("guid IS NOT NULL AND guid != ''").
		Pluck("guid", &rows).Error; err != nil {
		return nil, fmt.Errorf("đọc guid review: %w", err)
	}
	out := make(map[string]bool, len(rows))
	for _, row := range rows {
		out[row.GUID] = true
	}
	return out, nil
}

// AppendReview inserts a review row, ignoring conflicts on GUID.
func (r *Repository) AppendReview(ctx context.Context, tx app.Tx, rev app.ReviewRow) (bool, error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return false, err
	}
	res := db.Exec(`
		INSERT INTO reviews (card_id, grade, reviewed_at, next_due_at, guid)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (guid) DO NOTHING`,
		rev.CardID, rev.Grade, rev.ReviewedAt, rev.NextDueAt, rev.GUID)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ReviewsOfCard returns the review history for a card ordered by reviewed_at and ID.
func (r *Repository) ReviewsOfCard(ctx context.Context, tx app.Tx, cardID int64) ([]app.ReplayReview, error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return nil, err
	}
	var rows []app.ReplayReview
	if err := db.Raw(`
		SELECT grade, reviewed_at FROM reviews
		WHERE card_id = ? ORDER BY reviewed_at, id`, cardID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc lịch sử ôn card %d: %w", cardID, err)
	}
	return rows, nil
}

// ReplayCard updates card review parameters with an explicit timestamp.
func (r *Repository) ReplayCard(ctx context.Context, tx app.Tx, cardID int64, res app.ReplayResult, now string) error {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return err
	}
	return db.Exec(`UPDATE cards
		SET reps = ?, lapses = ?, stability = ?, difficulty = ?, due_at = ?,
			state = 'review', updated_at = ?
		WHERE id = ?`,
		res.Reps, res.Lapses, res.Stability, res.Difficulty, res.DueAt, now, cardID).Error
}

// NoteGUIDs returns the set of existing note GUIDs.
func (r *Repository) NoteGUIDs(ctx context.Context, tx app.Tx) (map[string]bool, error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return nil, err
	}
	var rows []struct{ GUID string }
	if err := db.Table("notes").
		Where("guid IS NOT NULL AND guid != ''").
		Pluck("guid", &rows).Error; err != nil {
		return nil, fmt.Errorf("đọc guid note: %w", err)
	}
	out := make(map[string]bool, len(rows))
	for _, row := range rows {
		out[row.GUID] = true
	}
	return out, nil
}

// AppendNote inserts a note row, ignoring conflicts on GUID.
func (r *Repository) AppendNote(ctx context.Context, tx app.Tx, n app.NoteRow) (bool, error) {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return false, err
	}
	res := db.Exec(`
		INSERT INTO notes (card_id, text, created_at, guid)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (guid) DO NOTHING`,
		n.CardID, n.Text, n.CreatedAt, n.GUID)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}
