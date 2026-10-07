// Package sync provides peer-to-peer data synchronization domain entities and conflict resolution policies.
package sync

import "time"

// SchemaVersion is the required database snapshot schema version.
const SchemaVersion = 4

// Table represents a table name in the peer snapshot.
type Table string

const (
	TableDecks             Table = "decks"
	TableCards             Table = "cards"
	TableReviews           Table = "reviews"
	TableNotes             Table = "notes"
	TableSyncMeta          Table = "sync_meta"
	TableSyncConflicts     Table = "sync_conflicts"
	TableRoadmapPaths      Table = "roadmap_paths"
	TableRoadmapStages     Table = "roadmap_stages"
	TableRoadmapMilestones Table = "roadmap_milestones"
	TableRoadmapTopics     Table = "roadmap_topics"
	TableRoadmapResources  Table = "roadmap_resources"
	TableRoadmapBookmarks  Table = "roadmap_bookmarks"
)

// Row represents a normalized record from a peer snapshot.
type Row struct {
	Table      Table
	GUID       string
	DeckGUID   string
	CardGUID   string
	ParentGUID string
	UpdatedAt  string
	CreatedAt  string
	Deleted    int
	Values     map[string]string
}

// EffectiveTS returns UpdatedAt if non-empty, otherwise CreatedAt.
func (r Row) EffectiveTS() string {
	if r.UpdatedAt != "" {
		return r.UpdatedAt
	}
	return r.CreatedAt
}

// IsTombstone reports whether the row is soft-deleted.
func (r Row) IsTombstone() bool { return r.Deleted == 1 }

// PeerSnapshot contains the full data export from a peer instance.
type PeerSnapshot struct {
	SchemaVersion int
	LastSync      string
	Rows          []Row
	MaxUpdatedAt  string
}

// Conflict records a synchronization collision for inspection.
type Conflict struct {
	Table  Table
	GUID   string
	Winner string
	Detail string
	At     string
}

// Conflict resolution winner identifiers.
const (
	WinnerLocal    = "local"
	WinnerIncoming = "incoming"
)

// ClockSkewThreshold is the maximum tolerable clock difference before warning.
const ClockSkewThreshold = 24 * time.Hour

