// Package roadmap contains orchestration use cases for learning paths and progress.
package roadmap

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Tx represents a database transaction handle managed by UnitOfWork.
type Tx = any

// UnitOfWork runs database mutations inside a transaction.
type UnitOfWork interface {
	Do(ctx context.Context, fn func(tx Tx) error) error
}

// DeckReader accesses SRS deck metadata.
type DeckReader interface {
	// Find returns deck info for id, returning (DeckInfo{}, nil) if not found.
	Find(ctx context.Context, id int64) (DeckInfo, error)
}

// DeckInfo contains deck metadata needed by roadmap.
type DeckInfo struct {
	Exists bool
	Name   string
	Lang   string
}

// DeckRef represents an attached deck reference for a stage.
type DeckRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Lang string `json:"lang"`
}

// Repository defines data access methods for roadmap entities.
type Repository interface {
	// Paths
	ListPaths(ctx context.Context) ([]Path, error)
	PathBySlug(ctx context.Context, slug string) (Path, error)
	PathByID(ctx context.Context, id int64) (Path, error)
	CreatePath(ctx context.Context, tx Tx, p *Path) error
	UpdatePath(ctx context.Context, tx Tx, p *Path) error
	SoftDeletePath(ctx context.Context, tx Tx, id int64) error

	// Stages
	ListStages(ctx context.Context, pathID int64) ([]Stage, error)
	// Batch methods for GraphQL dataloader optimization.
	ListStagesByPathIDs(ctx context.Context, pathIDs []int64) ([]Stage, error)
	ListTopicsByStageIDs(ctx context.Context, stageIDs []int64) ([]Topic, error)
	ListResourcesByTopicIDs(ctx context.Context, topicIDs []int64) ([]Resource, error)
	ListMilestonesByStageIDs(ctx context.Context, stageIDs []int64) ([]Milestone, error)
	StageByID(ctx context.Context, id int64) (Stage, error)
	CreateStage(ctx context.Context, tx Tx, s *Stage) error
	UpdateStage(ctx context.Context, tx Tx, s *Stage) error
	SoftDeleteStage(ctx context.Context, tx Tx, id int64) error
	// UpdateStageStatus updates status fields and reloads s within the same transaction.
	UpdateStageStatus(ctx context.Context, tx Tx, s *Stage) error
	MaxStagePosition(ctx context.Context, pathID int64) (int, error)

	// Topics
	ListTopics(ctx context.Context, stageID int64) ([]Topic, error)
	TopicByID(ctx context.Context, id int64) (Topic, error)
	CreateTopic(ctx context.Context, tx Tx, t *Topic) error
	UpdateTopic(ctx context.Context, tx Tx, t *Topic) error
	SoftDeleteTopic(ctx context.Context, tx Tx, id int64) error
	UpdateTopicStatus(ctx context.Context, tx Tx, t *Topic) error
	MaxTopicPosition(ctx context.Context, stageID int64) (int, error)

	// Resources
	ListResources(ctx context.Context, topicID int64) ([]Resource, error)
	ResourceByID(ctx context.Context, id int64) (Resource, error)
	CreateResource(ctx context.Context, tx Tx, r *Resource) error
	UpdateResource(ctx context.Context, tx Tx, r *Resource) error
	SoftDeleteResource(ctx context.Context, tx Tx, id int64) error
	MaxResourcePosition(ctx context.Context, topicID int64) (int, error)

	// Milestones
	ListMilestones(ctx context.Context, stageID int64) ([]Milestone, error)
	MilestoneByID(ctx context.Context, id int64) (Milestone, error)
	CreateMilestone(ctx context.Context, tx Tx, m *Milestone) error
	UpdateMilestone(ctx context.Context, tx Tx, m *Milestone) error
	SoftDeleteMilestone(ctx context.Context, tx Tx, id int64) error
	MaxMilestonePosition(ctx context.Context, stageID int64) (int, error)

	// Bookmarks
	ListBookmarks(ctx context.Context, filter BookmarkFilter) ([]Bookmark, error)
	BookmarkByID(ctx context.Context, id int64) (Bookmark, error)
	CreateBookmark(ctx context.Context, tx Tx, b *Bookmark) error
	UpdateBookmark(ctx context.Context, tx Tx, b *Bookmark) error
	SoftDeleteBookmark(ctx context.Context, tx Tx, id int64) error
}

// BookmarkFilter specifies filtering criteria for bookmarks.
type BookmarkFilter struct {
	Status string
	Tag    string
}

// Path represents a roadmap_paths row.
type Path struct {
	ID        int64
	Slug      string
	Language  string
	Title     string
	Overview  string
	IsBuiltin bool
	CreatedAt string
	GUID      string
	UpdatedAt string
	Deleted   int
}

// Stage represents a roadmap_stages row.
type Stage struct {
	ID            int64
	PathID        int64
	Slug          string
	Title         string
	Goal          string
	Position      int
	DurationWeeks int
	Status        Status
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

// Topic represents a roadmap_topics row.
type Topic struct {
	ID          int64
	StageID     int64
	Title       string
	Why         string
	Activities  string
	Position    int
	Status      Status
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

// Resource represents a roadmap_resources row.
type Resource struct {
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

// Milestone represents a roadmap_milestones row.
type Milestone struct {
	ID        int64
	StageID   int64
	Text      string
	Position  int
	CreatedAt string
	GUID      string
	UpdatedAt string
	Deleted   int
}

// Bookmark represents a roadmap_bookmarks row.
type Bookmark struct {
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

// Status represents the completion status of a node.
type Status = string

const (
	StatusNotStarted Status = "not_started"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
	StatusSkipped    Status = "skipped"
)

// AllStatuses contains all valid status values.
var AllStatuses = []Status{StatusNotStarted, StatusInProgress, StatusDone, StatusSkipped}

// ValidStatus reports whether s is a valid Status.
func ValidStatus(s Status) bool {
	for _, v := range AllStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// NowFunc returns the current time.
type NowFunc func() time.Time

// Clock returns the current UTC time.
func Clock() time.Time { return time.Now().UTC() }

// NewGUID generates a new UUID string for new entities.
func NewGUID() string { return uuid.NewString() }
