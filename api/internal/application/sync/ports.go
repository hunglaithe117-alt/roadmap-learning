// Package sync provides orchestration use cases for peer-to-peer data sync and merge.
package sync

import (
	"context"
	"time"

	domain "langapp/internal/domain/sync"
)

// Tx represents a database transaction handle managed by UnitOfWork.
type Tx = any

// UnitOfWork runs merge operations inside a single database transaction.
type UnitOfWork interface {
	Do(ctx context.Context, fn func(tx Tx) error) error
}

// Repository defines data access methods required by sync.
type Repository interface {
	SchemaVersion(ctx context.Context) (int, error)
	PeerSchemaVersion(ctx context.Context) (int, error)
	LastSyncAt(ctx context.Context) (string, error)
	SetLastSyncAt(ctx context.Context, tx Tx, now string) error

	// LWW (decks and cards)
	DeckRows(ctx context.Context, tx Tx) (map[string]DeckRow, error)
	UpsertDeck(ctx context.Context, tx Tx, d DeckRow, found bool) (int64, error)
	CardRows(ctx context.Context, tx Tx) (map[string]CardRow, error)
	TombstoneCard(ctx context.Context, tx Tx, deckID int64, front string) (int64, string, bool, error)
	LiveCardGUIDByFront(ctx context.Context, tx Tx, deckID int64, front string) (int64, string, bool, error)
	UpsertCard(ctx context.Context, tx Tx, c CardRow, found bool) (id int64, dupKept bool, err error)

	// LWW (roadmap)
	PathRows(ctx context.Context, tx Tx) (map[string]PathRow, error)
	UpsertPath(ctx context.Context, tx Tx, p PathRow, found bool) (id int64, skipped bool, err error)
	StageRows(ctx context.Context, tx Tx) (map[string]StageRow, error)
	UpsertStage(ctx context.Context, tx Tx, s StageRow, found bool) (id int64, skipped bool, err error)
	MilestoneRows(ctx context.Context, tx Tx) (map[string]MilestoneRow, error)
	UpsertMilestone(ctx context.Context, tx Tx, m MilestoneRow, found bool) (id int64, skipped bool, err error)
	TopicRows(ctx context.Context, tx Tx) (map[string]TopicRow, error)
	UpsertTopic(ctx context.Context, tx Tx, tp TopicRow, found bool) (id int64, skipped bool, err error)
	ResourceRows(ctx context.Context, tx Tx) (map[string]ResourceRow, error)
	UpsertResource(ctx context.Context, tx Tx, res ResourceRow, found bool) (id int64, skipped bool, err error)
	BookmarkRows(ctx context.Context, tx Tx) (map[string]BookmarkRow, error)
	UpsertBookmark(ctx context.Context, tx Tx, b BookmarkRow, found bool) (id int64, skipped bool, err error)

	// Append-only (reviews and notes)
	ReviewGUIDs(ctx context.Context, tx Tx) (map[string]bool, error)
	AppendReview(ctx context.Context, tx Tx, r ReviewRow) (bool, error)
	ReviewsOfCard(ctx context.Context, tx Tx, cardID int64) ([]ReplayReview, error)
	ReplayCard(ctx context.Context, tx Tx, cardID int64, r ReplayResult, now string) error
	NoteGUIDs(ctx context.Context, tx Tx) (map[string]bool, error)
	AppendNote(ctx context.Context, tx Tx, n NoteRow) (bool, error)

	// Conflict logging
	LogConflict(ctx context.Context, tx Tx, c domain.Conflict) error
	ListConflicts(ctx context.Context, limit int) ([]domain.Conflict, error)
	CountConflicts(ctx context.Context) (int, error)
}

// SnapshotLoader loads snapshots from peer sources.
type SnapshotLoader interface {
	Load(ctx context.Context) (domain.PeerSnapshot, error)
}

// DeckRow represents a decks row for sync operations.
type DeckRow struct {
	ID        int64
	GUID      string
	Name      string
	Lang      string
	CreatedAt string
	UpdatedAt string
	Deleted   int
}

// CardRow represents a cards row for sync operations.
type CardRow struct {
	ID         int64
	GUID       string
	DeckGUID   string
	DeckID     *int64
	Front      string
	Back       string
	Pinyin     string
	DueAt      string
	Stability  float64
	Difficulty float64
	Reps       int
	Lapses     int
	State      string
	CreatedAt  string
	Tone       *string
	IPA        *string
	Stress     *string
	AudioURL   *string
	UpdatedAt  string
	Deleted    int
}

// PathRow represents a roadmap_paths row for sync operations.
type PathRow struct {
	ID        int64
	GUID      string
	Slug      string
	Language  string
	Title     string
	Overview  string
	IsBuiltin int
	CreatedAt string
	UpdatedAt string
	Deleted   int
}

// StageRow represents a roadmap_stages row for sync operations.
type StageRow struct {
	ID            int64
	GUID          string
	PathGUID      string
	Slug          string
	Title         string
	Goal          string
	Position      int
	DurationWeeks int
	Status        string
	StatusNote    string
	CompletedAt   *string
	DeckGUID      *string
	Terrain       string
	Direction     string
	CreatedAt     string
	UpdatedAt     string
	Deleted       int
}

// MilestoneRow represents a roadmap_milestones row for sync operations.
type MilestoneRow struct {
	ID        int64
	GUID      string
	StageGUID string
	Text      string
	Position  int
	CreatedAt string
	UpdatedAt string
	Deleted   int
}

// TopicRow represents a roadmap_topics row for sync operations.
type TopicRow struct {
	ID          int64
	GUID        string
	StageGUID   string
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
	UpdatedAt   string
	Deleted     int
}

// ResourceRow represents a roadmap_resources row for sync operations.
type ResourceRow struct {
	ID        int64
	GUID      string
	TopicGUID string
	Title     string
	URL       *string
	Kind      string
	Note      string
	Position  int
	CreatedAt string
	UpdatedAt string
	Deleted   int
}

// BookmarkRow represents a roadmap_bookmarks row for sync operations.
type BookmarkRow struct {
	ID        int64
	GUID      string
	Title     string
	URL       *string
	Note      string
	Tags      string
	Status    string
	CreatedAt string
	UpdatedAt string
	Deleted   int
}

// ReviewRow represents a reviews row for sync operations.
type ReviewRow struct {
	GUID       string
	CardGUID   string
	CardID     *int64
	Grade      int
	ReviewedAt string
	NextDueAt  string
}

// NoteRow represents a notes row for sync operations.
type NoteRow struct {
	GUID      string
	CardGUID  string
	CardID    *int64
	Text      string
	CreatedAt string
}

// ReplayReview represents a single review event for replay calculations.
type ReplayReview struct {
	Grade      int
	ReviewedAt string
}

// ReplayResult represents recalculated card scheduling attributes.
type ReplayResult struct {
	Reps       int
	Lapses     int
	Stability  float64
	Difficulty float64
	DueAt      string
}

// Merged holds counts of rows merged per table.
type Merged struct {
	Decks             int
	Cards             int
	Reviews           int
	Notes             int
	RoadmapPaths      int
	RoadmapStages     int
	RoadmapMilestones int
	RoadmapTopics     int
	RoadmapResources  int
	RoadmapBookmarks  int
}

// MergeResult represents the outcome of a merge operation.
type MergeResult struct {
	OK         bool
	Merged     Merged
	Conflicts  []domain.Conflict
	Warnings   []string
	LastSyncAt string
}

// NowFunc returns current time.
type NowFunc func() time.Time

// Clock returns the current UTC time.
func Clock() time.Time { return time.Now().UTC() }
