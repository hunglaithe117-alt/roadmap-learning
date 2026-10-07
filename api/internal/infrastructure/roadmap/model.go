// Package roadmapinfra provides GORM models and repository implementations for the roadmap bounded context.
package roadmapinfra

import (
	app "langapp/internal/application/roadmap"
)

// Path maps to the roadmap_paths table.
type Path struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Slug      string `gorm:"column:slug"`
	Language  string `gorm:"column:language"`
	Title     string `gorm:"column:title"`
	Overview  string `gorm:"column:overview"`
	IsBuiltin int    `gorm:"column:is_builtin"`
	CreatedAt string `gorm:"column:created_at;autoCreateTime:false"`
	GUID      string `gorm:"column:guid"`
	UpdatedAt string `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted   int    `gorm:"column:deleted"`
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (Path) TableName() string { return "roadmap_paths" }

// Stage maps to the roadmap_stages table.
type Stage struct {
	ID            int64   `gorm:"column:id;primaryKey;autoIncrement"`
	PathID        int64   `gorm:"column:path_id"`
	Slug          string  `gorm:"column:slug"`
	Title         string  `gorm:"column:title"`
	Goal          string  `gorm:"column:goal"`
	Position      int     `gorm:"column:position"`
	DurationWeeks int     `gorm:"column:duration_weeks"`
	Status        string  `gorm:"column:status"`
	StatusNote    string  `gorm:"column:status_note"`
	CompletedAt   *string `gorm:"column:completed_at"`
	DeckID        *int64  `gorm:"column:deck_id"`
	Terrain       string  `gorm:"column:terrain"`
	Direction     string  `gorm:"column:direction"`
	CreatedAt     string  `gorm:"column:created_at;autoCreateTime:false"`
	GUID          string  `gorm:"column:guid"`
	UpdatedAt     string  `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted       int     `gorm:"column:deleted"`
}

func (Stage) TableName() string { return "roadmap_stages" }

// Topic maps to the roadmap_topics table.
type Topic struct {
	ID          int64    `gorm:"column:id;primaryKey;autoIncrement"`
	StageID     int64    `gorm:"column:stage_id"`
	Title       string   `gorm:"column:title"`
	Why         string   `gorm:"column:why"`
	Activities  string   `gorm:"column:activities"`
	Position    int      `gorm:"column:position"`
	Status      string   `gorm:"column:status"`
	StatusNote  string   `gorm:"column:status_note"`
	CompletedAt *string  `gorm:"column:completed_at"`
	IsOptional  int      `gorm:"column:is_optional"`
	MapX        *float64 `gorm:"column:map_x"`
	MapY        *float64 `gorm:"column:map_y"`
	CreatedAt   string   `gorm:"column:created_at;autoCreateTime:false"`
	GUID        string   `gorm:"column:guid"`
	UpdatedAt   string   `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted     int      `gorm:"column:deleted"`
}

func (Topic) TableName() string { return "roadmap_topics" }

// Resource maps to the roadmap_resources table.
type Resource struct {
	ID        int64   `gorm:"column:id;primaryKey;autoIncrement"`
	TopicID   int64   `gorm:"column:topic_id"`
	Title     string  `gorm:"column:title"`
	URL       *string `gorm:"column:url"`
	Kind      string  `gorm:"column:kind"`
	Note      string  `gorm:"column:note"`
	Position  int     `gorm:"column:position"`
	CreatedAt string  `gorm:"column:created_at;autoCreateTime:false"`
	GUID      string  `gorm:"column:guid"`
	UpdatedAt string  `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted   int     `gorm:"column:deleted"`
}

func (Resource) TableName() string { return "roadmap_resources" }

// Milestone maps to the roadmap_milestones table.
type Milestone struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	StageID   int64  `gorm:"column:stage_id"`
	Text      string `gorm:"column:text"`
	Position  int    `gorm:"column:position"`
	CreatedAt string `gorm:"column:created_at;autoCreateTime:false"`
	GUID      string `gorm:"column:guid"`
	UpdatedAt string `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted   int    `gorm:"column:deleted"`
}

func (Milestone) TableName() string { return "roadmap_milestones" }

// bookmarkRow maps to the roadmap_bookmarks table.
type bookmarkRow struct {
	ID        int64   `gorm:"column:id;primaryKey;autoIncrement"`
	Title     string  `gorm:"column:title"`
	URL       *string `gorm:"column:url"`
	Note      string  `gorm:"column:note"`
	Tags      string  `gorm:"column:tags"`
	Status    string  `gorm:"column:status"`
	CreatedAt string  `gorm:"column:created_at;autoCreateTime:false"`
	GUID      string  `gorm:"column:guid"`
	UpdatedAt string  `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted   int     `gorm:"column:deleted"`
}

func (bookmarkRow) TableName() string { return "roadmap_bookmarks" }

func pathToApp(p Path) app.Path {
	out := app.Path{
		ID: p.ID, Slug: p.Slug, Language: p.Language, Title: p.Title,
		Overview: p.Overview, IsBuiltin: p.IsBuiltin == 1,
		CreatedAt: p.CreatedAt, GUID: p.GUID, UpdatedAt: p.UpdatedAt,
		Deleted: p.Deleted,
	}
	return out
}

func pathFromApp(p app.Path) Path {
	return Path{
		ID: p.ID, Slug: p.Slug, Language: p.Language, Title: p.Title,
		Overview: p.Overview, IsBuiltin: boolToInt(p.IsBuiltin),
		CreatedAt: p.CreatedAt, GUID: p.GUID, UpdatedAt: p.UpdatedAt,
		Deleted: p.Deleted,
	}
}

func stageToApp(s Stage) app.Stage        { return app.Stage(s) }
func stageFromApp(s app.Stage) Stage      { return Stage(s) }
func topicToApp(t Topic) app.Topic        { return app.Topic(t) }
func topicFromApp(t app.Topic) Topic      { return Topic(t) }
func resToApp(r Resource) app.Resource    { return app.Resource(r) }
func resFromApp(r app.Resource) Resource  { return Resource(r) }
func msToApp(m Milestone) app.Milestone   { return app.Milestone(m) }
func msFromApp(m app.Milestone) Milestone { return Milestone(m) }

func bmToApp(b bookmarkRow) app.Bookmark   { return app.Bookmark(b) }
func bmFromApp(b app.Bookmark) bookmarkRow { return bookmarkRow(b) }
