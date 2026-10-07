package roadmapinfra

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"

	app "langapp/internal/application/roadmap"
	"langapp/internal/platform/txtx"
)

type unitOfWork struct{ db *gorm.DB }

// NewUnitOfWork constructs a UnitOfWork backed by GORM.
func NewUnitOfWork(db *gorm.DB) app.UnitOfWork { return &unitOfWork{db: db} }

func (u *unitOfWork) Do(ctx context.Context, fn func(app.Tx) error) error {
	return txtx.Do(ctx, u.db, fn)
}

func txOf(root *gorm.DB, tx app.Tx) (*gorm.DB, error) {
	return txtx.Of(root, tx)
}

func txDB(root *gorm.DB, ctx context.Context, tx app.Tx) (*gorm.DB, error) {
	db, err := txOf(root, tx)
	if err != nil {
		return nil, err
	}
	return db.WithContext(ctx), nil
}

func errNoRows(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return app.ErrNotFound
	}
	return err
}

func isNoRows(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows)
}

// Repository implements app.Repository.
type Repository struct {
	db *gorm.DB
}

// NewRepository constructs a roadmap Repository on a GORM DB.
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

var _ app.Repository = (*Repository)(nil)

// ── Path ────────────────────────────────────────────────────────────────────

// CreatePath inserts a path into the database.
func (r *Repository) CreatePath(ctx context.Context, tx app.Tx, p *app.Path) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := pathFromApp(*p)
	if err := db.Create(&row).Error; err != nil {
		return err
	}
	*p = pathToApp(row)
	return nil
}

// ListPaths returns all non-deleted paths ordered by language and ID.
func (r *Repository) ListPaths(ctx context.Context) ([]app.Path, error) {
	var rows []Path
	err := r.db.WithContext(ctx).Where("deleted = 0").Order("language, id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list roadmap paths: %w", err)
	}
	out := make([]app.Path, 0, len(rows))
	for _, row := range rows {
		out = append(out, pathToApp(row))
	}
	return out, nil
}

// PathBySlug returns a path by slug, or app.ErrNotFound if deleted or absent.
func (r *Repository) PathBySlug(ctx context.Context, slug string) (app.Path, error) {
	var row Path
	err := r.db.WithContext(ctx).Where("slug = ? AND deleted = 0", slug).Take(&row).Error
	if err != nil {
		return app.Path{}, errNoRows(err)
	}
	return pathToApp(row), nil
}

// PathByID returns a path by ID, or app.ErrNotFound if deleted or absent.
func (r *Repository) PathByID(ctx context.Context, id int64) (app.Path, error) {
	var row Path
	err := r.db.WithContext(ctx).Where("id = ? AND deleted = 0", id).Take(&row).Error
	if err != nil {
		return app.Path{}, errNoRows(err)
	}
	return pathToApp(row), nil
}

// UpdatePath updates editable path columns, excluding slug.
func (r *Repository) UpdatePath(ctx context.Context, tx app.Tx, p *app.Path) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := pathFromApp(*p)
	if err := db.Model(&row).Omit("updated_at").
		Updates(map[string]any{
			"language":   row.Language,
			"title":      row.Title,
			"overview":   row.Overview,
			"is_builtin": row.IsBuiltin,
		}).Error; err != nil {
		return err
	}
	if err := db.Where("id = ?", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*p = pathToApp(row)
	return nil
}

// SoftDeletePath marks a path and its child tree as deleted.
func (r *Repository) SoftDeletePath(ctx context.Context, tx app.Tx, id int64) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	if err := db.Model(&Path{ID: id}).Where("deleted = 0").
		UpdateColumn("deleted", 1).Error; err != nil {
		return err
	}
	return r.softDeleteTreeOfPath(ctx, db, id)
}

// softDeleteTreeOfPath soft-deletes stages, topics, and milestones under a path bottom-up.
func (r *Repository) softDeleteTreeOfPath(ctx context.Context, db *gorm.DB, pathID int64) error {
	stmts := []struct {
		table  string
		column string
		query  string
	}{
		{
			table: "roadmap_resources", column: "topic_id",
			query: "UPDATE roadmap_resources SET deleted = 1 WHERE deleted = 0 AND topic_id IN " +
				"(SELECT t.id FROM roadmap_topics t JOIN roadmap_stages s ON s.id = t.stage_id WHERE s.path_id = ?)",
		},
		{
			table: "roadmap_topics", column: "stage_id",
			query: "UPDATE roadmap_topics SET deleted = 1 WHERE deleted = 0 AND stage_id IN " +
				"(SELECT id FROM roadmap_stages WHERE path_id = ?)",
		},
		{
			table: "roadmap_milestones", column: "stage_id",
			query: "UPDATE roadmap_milestones SET deleted = 1 WHERE deleted = 0 AND stage_id IN " +
				"(SELECT id FROM roadmap_stages WHERE path_id = ?)",
		},
	}
	for _, s := range stmts {
		if err := db.Exec(s.query, pathID).Error; err != nil {
			return fmt.Errorf("soft delete %s: %w", s.table, err)
		}
	}
	return nil
}

// ── Stage ───────────────────────────────────────────────────────────────────

// CreateStage inserts a stage into the database.
func (r *Repository) CreateStage(ctx context.Context, tx app.Tx, s *app.Stage) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := stageFromApp(*s)
	if err := db.Create(&row).Error; err != nil {
		return err
	}
	*s = stageToApp(row)
	return nil
}

// ListStages returns non-deleted stages for a path ordered by position and ID.
func (r *Repository) ListStages(ctx context.Context, pathID int64) ([]app.Stage, error) {
	var rows []Stage
	err := r.db.WithContext(ctx).Where("path_id = ? AND deleted = 0", pathID).
		Order("position, id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list roadmap stages: %w", err)
	}
	out := make([]app.Stage, 0, len(rows))
	for _, row := range rows {
		out = append(out, stageToApp(row))
	}
	return out, nil
}

// StageByID returns a stage by ID, or app.ErrNotFound if deleted or absent.
func (r *Repository) StageByID(ctx context.Context, id int64) (app.Stage, error) {
	var row Stage
	err := r.db.WithContext(ctx).Where("id = ? AND deleted = 0", id).Take(&row).Error
	if err != nil {
		return app.Stage{}, errNoRows(err)
	}
	return stageToApp(row), nil
}

// UpdateStage updates editable stage columns.
func (r *Repository) UpdateStage(ctx context.Context, tx app.Tx, s *app.Stage) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := stageFromApp(*s)
	if err := db.Model(&row).Omit("updated_at").
		Updates(map[string]any{
			"slug":           row.Slug,
			"title":          row.Title,
			"goal":           row.Goal,
			"position":       row.Position,
			"duration_weeks": row.DurationWeeks,
			"deck_id":        row.DeckID,
			"terrain":        row.Terrain,
			"direction":      row.Direction,
		}).Error; err != nil {
		return err
	}
	if err := db.Where("id = ? AND deleted = 0", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*s = stageToApp(row)
	return nil
}

// UpdateStageStatus updates status, note, and completed_at in a single statement.
func (r *Repository) UpdateStageStatus(ctx context.Context, tx app.Tx, s *app.Stage) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := stageFromApp(*s)
	if err := db.Model(&row).Where("deleted = 0").Omit("updated_at").
		Updates(map[string]any{
			"status":       row.Status,
			"status_note":  row.StatusNote,
			"completed_at": row.CompletedAt,
		}).Error; err != nil {
		return err
	}
	if err := db.Where("id = ? AND deleted = 0", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*s = stageToApp(row)
	return nil
}

// SoftDeleteStage marks a stage and its child tree as deleted.
func (r *Repository) SoftDeleteStage(ctx context.Context, tx app.Tx, id int64) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	if err := db.Model(&Stage{ID: id}).Where("deleted = 0").
		UpdateColumn("deleted", 1).Error; err != nil {
		return err
	}
	if err := db.Exec(
		"UPDATE roadmap_resources SET deleted = 1 WHERE deleted = 0 AND topic_id IN "+
			"(SELECT id FROM roadmap_topics WHERE stage_id = ?)", id).Error; err != nil {
		return fmt.Errorf("soft delete roadmap_resources: %w", err)
	}
	if err := db.Exec(
		"UPDATE roadmap_topics SET deleted = 1 WHERE deleted = 0 AND stage_id = ?", id).Error; err != nil {
		return fmt.Errorf("soft delete roadmap_topics: %w", err)
	}
	if err := db.Exec(
		"UPDATE roadmap_milestones SET deleted = 1 WHERE deleted = 0 AND stage_id = ?", id).Error; err != nil {
		return fmt.Errorf("soft delete roadmap_milestones: %w", err)
	}
	return nil
}

// MaxStagePosition returns the highest position in a path, or -1 if empty.
func (r *Repository) MaxStagePosition(ctx context.Context, pathID int64) (int, error) {
	var max *int
	if err := r.db.WithContext(ctx).Model(&Stage{}).
		Where("path_id = ? AND deleted = 0", pathID).
		Select("MAX(position)").Scan(&max).Error; err != nil {
		return 0, fmt.Errorf("max stage position: %w", err)
	}
	if max == nil {
		return -1, nil
	}
	return *max, nil
}

// ── Topic ───────────────────────────────────────────────────────────────────

// CreateTopic inserts a topic into the database.
func (r *Repository) CreateTopic(ctx context.Context, tx app.Tx, t *app.Topic) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := topicFromApp(*t)
	if err := db.Create(&row).Error; err != nil {
		return err
	}
	*t = topicToApp(row)
	return nil
}

// ListTopics returns non-deleted topics for a stage ordered by position and ID.
func (r *Repository) ListTopics(ctx context.Context, stageID int64) ([]app.Topic, error) {
	var rows []Topic
	err := r.db.WithContext(ctx).Where("stage_id = ? AND deleted = 0", stageID).
		Order("position, id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list roadmap topics: %w", err)
	}
	out := make([]app.Topic, 0, len(rows))
	for _, row := range rows {
		out = append(out, topicToApp(row))
	}
	return out, nil
}

// TopicByID returns a topic by ID, or app.ErrNotFound if deleted or absent.
func (r *Repository) TopicByID(ctx context.Context, id int64) (app.Topic, error) {
	var row Topic
	err := r.db.WithContext(ctx).Where("id = ? AND deleted = 0", id).Take(&row).Error
	if err != nil {
		return app.Topic{}, errNoRows(err)
	}
	return topicToApp(row), nil
}

// UpdateTopic updates editable topic columns.
func (r *Repository) UpdateTopic(ctx context.Context, tx app.Tx, t *app.Topic) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := topicFromApp(*t)
	if err := db.Model(&row).Omit("updated_at").
		Updates(map[string]any{
			"title":       row.Title,
			"why":         row.Why,
			"activities":  row.Activities,
			"position":    row.Position,
			"is_optional": row.IsOptional,
			"map_x":       row.MapX,
			"map_y":       row.MapY,
		}).Error; err != nil {
		return err
	}
	if err := db.Where("id = ? AND deleted = 0", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*t = topicToApp(row)
	return nil
}

// UpdateTopicStatus updates status, note, and completed_at for a topic.
func (r *Repository) UpdateTopicStatus(ctx context.Context, tx app.Tx, t *app.Topic) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := topicFromApp(*t)
	if err := db.Model(&row).Where("deleted = 0").Omit("updated_at").
		Updates(map[string]any{
			"status":       row.Status,
			"status_note":  row.StatusNote,
			"completed_at": row.CompletedAt,
		}).Error; err != nil {
		return err
	}
	if err := db.Where("id = ? AND deleted = 0", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*t = topicToApp(row)
	return nil
}

// SoftDeleteTopic marks a topic and its resources as deleted.
func (r *Repository) SoftDeleteTopic(ctx context.Context, tx app.Tx, id int64) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	if err := db.Model(&Topic{ID: id}).Where("deleted = 0").
		UpdateColumn("deleted", 1).Error; err != nil {
		return err
	}
	return errNoRows(db.Model(&Resource{}).Where("topic_id = ? AND deleted = 0", id).
		UpdateColumn("deleted", 1).Error)
}

// MaxTopicPosition returns the highest position in a stage, or -1 if empty.
func (r *Repository) MaxTopicPosition(ctx context.Context, stageID int64) (int, error) {
	var max *int
	if err := r.db.WithContext(ctx).Model(&Topic{}).
		Where("stage_id = ? AND deleted = 0", stageID).
		Select("MAX(position)").Scan(&max).Error; err != nil {
		return 0, fmt.Errorf("max topic position: %w", err)
	}
	if max == nil {
		return -1, nil
	}
	return *max, nil
}

// ── Resource ────────────────────────────────────────────────────────────────

// CreateResource inserts a resource into the database.
func (r *Repository) CreateResource(ctx context.Context, tx app.Tx, rs *app.Resource) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := resFromApp(*rs)
	if err := db.Create(&row).Error; err != nil {
		return err
	}
	*rs = resToApp(row)
	return nil
}

// ListResources returns non-deleted resources for a topic ordered by position and ID.
func (r *Repository) ListResources(ctx context.Context, topicID int64) ([]app.Resource, error) {
	var rows []Resource
	err := r.db.WithContext(ctx).Where("topic_id = ? AND deleted = 0", topicID).
		Order("position, id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list roadmap resources: %w", err)
	}
	out := make([]app.Resource, 0, len(rows))
	for _, row := range rows {
		out = append(out, resToApp(row))
	}
	return out, nil
}

// ResourceByID returns a resource by ID, or app.ErrNotFound if deleted or absent.
func (r *Repository) ResourceByID(ctx context.Context, id int64) (app.Resource, error) {
	var row Resource
	err := r.db.WithContext(ctx).Where("id = ? AND deleted = 0", id).Take(&row).Error
	if err != nil {
		return app.Resource{}, errNoRows(err)
	}
	return resToApp(row), nil
}

// UpdateResource updates editable resource columns.
func (r *Repository) UpdateResource(ctx context.Context, tx app.Tx, rs *app.Resource) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := resFromApp(*rs)
	if err := db.Model(&row).Omit("updated_at").
		Updates(map[string]any{
			"title":    row.Title,
			"url":      row.URL,
			"kind":     row.Kind,
			"note":     row.Note,
			"position": row.Position,
		}).Error; err != nil {
		return err
	}
	if err := db.Where("id = ? AND deleted = 0", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*rs = resToApp(row)
	return nil
}

// SoftDeleteResource marks a resource as deleted.
func (r *Repository) SoftDeleteResource(ctx context.Context, tx app.Tx, id int64) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	return db.Model(&Resource{ID: id}).Where("deleted = 0").
		UpdateColumn("deleted", 1).Error
}

// MaxResourcePosition returns the highest position in a topic, or -1 if empty.
func (r *Repository) MaxResourcePosition(ctx context.Context, topicID int64) (int, error) {
	var max *int
	if err := r.db.WithContext(ctx).Model(&Resource{}).
		Where("topic_id = ? AND deleted = 0", topicID).
		Select("MAX(position)").Scan(&max).Error; err != nil {
		return 0, fmt.Errorf("max resource position: %w", err)
	}
	if max == nil {
		return -1, nil
	}
	return *max, nil
}

// ── Milestone ───────────────────────────────────────────────────────────────

// CreateMilestone inserts a milestone into the database.
func (r *Repository) CreateMilestone(ctx context.Context, tx app.Tx, m *app.Milestone) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := msFromApp(*m)
	if err := db.Create(&row).Error; err != nil {
		return err
	}
	*m = msToApp(row)
	return nil
}

// ListMilestones returns non-deleted milestones for a stage ordered by position and ID.
func (r *Repository) ListMilestones(ctx context.Context, stageID int64) ([]app.Milestone, error) {
	var rows []Milestone
	err := r.db.WithContext(ctx).Where("stage_id = ? AND deleted = 0", stageID).
		Order("position, id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list roadmap milestones: %w", err)
	}
	out := make([]app.Milestone, 0, len(rows))
	for _, row := range rows {
		out = append(out, msToApp(row))
	}
	return out, nil
}

// MilestoneByID returns a milestone by ID, or app.ErrNotFound if deleted or absent.
func (r *Repository) MilestoneByID(ctx context.Context, id int64) (app.Milestone, error) {
	var row Milestone
	err := r.db.WithContext(ctx).Where("id = ? AND deleted = 0", id).Take(&row).Error
	if err != nil {
		return app.Milestone{}, errNoRows(err)
	}
	return msToApp(row), nil
}

// UpdateMilestone updates milestone text and position.
func (r *Repository) UpdateMilestone(ctx context.Context, tx app.Tx, m *app.Milestone) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := msFromApp(*m)
	if err := db.Model(&row).Omit("updated_at").
		Updates(map[string]any{
			"text":     row.Text,
			"position": row.Position,
		}).Error; err != nil {
		return err
	}
	if err := db.Where("id = ? AND deleted = 0", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*m = msToApp(row)
	return nil
}

// SoftDeleteMilestone marks a milestone as deleted.
func (r *Repository) SoftDeleteMilestone(ctx context.Context, tx app.Tx, id int64) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	return db.Model(&Milestone{ID: id}).Where("deleted = 0").
		UpdateColumn("deleted", 1).Error
}

// MaxMilestonePosition returns the highest position in a stage, or -1 if empty.
func (r *Repository) MaxMilestonePosition(ctx context.Context, stageID int64) (int, error) {
	var max *int
	if err := r.db.WithContext(ctx).Model(&Milestone{}).
		Where("stage_id = ? AND deleted = 0", stageID).
		Select("MAX(position)").Scan(&max).Error; err != nil {
		return 0, fmt.Errorf("max milestone position: %w", err)
	}
	if max == nil {
		return -1, nil
	}
	return *max, nil
}

// ── Bookmark ────────────────────────────────────────────────────────────────

// CreateBookmark inserts a bookmark into the database.
func (r *Repository) CreateBookmark(ctx context.Context, tx app.Tx, b *app.Bookmark) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := bmFromApp(*b)
	if err := db.Create(&row).Error; err != nil {
		return err
	}
	*b = bmToApp(row)
	return nil
}

// ListBookmarks returns non-deleted bookmarks matching filter ordered by ID.
func (r *Repository) ListBookmarks(ctx context.Context, filter app.BookmarkFilter) ([]app.Bookmark, error) {
	q := r.db.WithContext(ctx).Model(&bookmarkRow{}).Where("deleted = 0")
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.Tag != "" {
		q = q.Where("position(',' || ? || ',' in ',' || tags || ',') > 0", filter.Tag)
	}
	var rows []bookmarkRow
	if err := q.Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list roadmap bookmarks: %w", err)
	}
	out := make([]app.Bookmark, 0, len(rows))
	for _, row := range rows {
		out = append(out, bmToApp(row))
	}
	return out, nil
}

// BookmarkByID returns a bookmark by ID, or app.ErrNotFound if deleted or absent.
func (r *Repository) BookmarkByID(ctx context.Context, id int64) (app.Bookmark, error) {
	var row bookmarkRow
	if err := r.db.WithContext(ctx).Where("id = ? AND deleted = 0", id).Take(&row).Error; err != nil {
		return app.Bookmark{}, errNoRows(err)
	}
	return bmToApp(row), nil
}

// UpdateBookmark updates editable bookmark columns and status.
func (r *Repository) UpdateBookmark(ctx context.Context, tx app.Tx, b *app.Bookmark) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := bmFromApp(*b)
	if err := db.Model(&row).Omit("updated_at").
		Updates(map[string]any{
			"title":  row.Title,
			"url":    row.URL,
			"note":   row.Note,
			"tags":   row.Tags,
			"status": row.Status,
		}).Error; err != nil {
		return err
	}
	if err := db.Where("id = ?", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*b = bmToApp(row)
	return nil
}

// SoftDeleteBookmark marks a bookmark as deleted.
func (r *Repository) SoftDeleteBookmark(ctx context.Context, tx app.Tx, id int64) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	return db.Model(&bookmarkRow{ID: id}).Where("deleted = 0").
		UpdateColumn("deleted", 1).Error
}
