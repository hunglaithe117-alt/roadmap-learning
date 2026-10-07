package syncinfra

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"gorm.io/gorm"

	app "langapp/internal/application/sync"
	domain "langapp/internal/domain/sync"
)

// guidByID builds an ID-to-GUID map for the given table.
func (r *Repository) guidByID(ctx context.Context, tx app.Tx, table string) (map[int64]string, error) {
	var rows []struct {
		ID   int64
		GUID string
	}
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err := db.Table(table).Select("id, guid").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc guid của %s: %w", table, err)
	}
	out := make(map[int64]string, len(rows))
	for _, row := range rows {
		out[row.ID] = row.GUID
	}
	return out, nil
}

// resolve looks up the local integer ID for a parent row by GUID.
func (r *Repository) resolve(ctx context.Context, db *gorm.DB, table, guid string) (int64, error) {
	if guid == "" {
		return 0, fmt.Errorf("thiếu guid của cha %s", table)
	}
	var id int64
	if err := db.Raw(`SELECT id FROM `+table+` WHERE guid = ?`, guid).Scan(&id).Error; err != nil {
		return 0, fmt.Errorf("không tìm thấy cha %s guid=%s: %w", table, guid, err)
	}
	if id == 0 {
		return 0, fmt.Errorf("không tìm thấy cha %s guid=%s", table, guid)
	}
	return id, nil
}

// ── Snapshot loader ─────────────────────────────────────────────────────────

// SchemaLoader implements app.SnapshotLoader by querying the peer Postgres schema.
type SchemaLoader struct {
	peer *gorm.DB
}

// NewSchemaLoader constructs a SchemaLoader on the peer DB pool.
func NewSchemaLoader(peer *gorm.DB) *SchemaLoader { return &SchemaLoader{peer: peer} }

var _ app.SnapshotLoader = (*SchemaLoader)(nil)

// fkField indicates which domain.Row field receives the parent GUID.
type fkField int

const (
	// fkParent maps to Row.ParentGUID.
	fkParent fkField = iota
	// fkDeck maps to Row.DeckGUID.
	fkDeck
	// fkCard maps to Row.CardGUID.
	fkCard
)

// fkRef describes a foreign key reference column and target table.
type fkRef struct {
	column string
	parent domain.Table
	field  fkField
}

// snapshotTable describes a table in the snapshot along with FKs to resolve.
type snapshotTable struct {
	table domain.Table
	fks   []fkRef
}

// snapshotTables defines the tables to read from the peer snapshot.
var snapshotTables = []snapshotTable{
	{table: domain.TableDecks},
	{table: domain.TableCards, fks: []fkRef{{column: "deck_id", parent: domain.TableDecks, field: fkDeck}}},
	{table: domain.TableRoadmapPaths},
	{table: domain.TableRoadmapStages, fks: []fkRef{
		{column: "path_id", parent: domain.TableRoadmapPaths, field: fkParent},
		{column: "deck_id", parent: domain.TableDecks, field: fkDeck},
	}},
	{table: domain.TableRoadmapMilestones, fks: []fkRef{{column: "stage_id", parent: domain.TableRoadmapStages, field: fkParent}}},
	{table: domain.TableRoadmapTopics, fks: []fkRef{{column: "stage_id", parent: domain.TableRoadmapStages, field: fkParent}}},
	{table: domain.TableRoadmapResources, fks: []fkRef{{column: "topic_id", parent: domain.TableRoadmapTopics, field: fkParent}}},
	{table: domain.TableRoadmapBookmarks},
	{table: domain.TableReviews, fks: []fkRef{{column: "card_id", parent: domain.TableCards, field: fkCard}}},
	{table: domain.TableNotes, fks: []fkRef{{column: "card_id", parent: domain.TableCards, field: fkCard}}},
}

// snapshotColumns defines the explicit columns read for each table.
var snapshotColumns = map[domain.Table][]string{
	domain.TableDecks: {"guid", "name", "lang", "created_at", "updated_at", "deleted"},
	domain.TableCards: {"guid", "front", "back", "pinyin", "due_at", "stability",
		"difficulty", "reps", "lapses", "state", "created_at", "tone", "ipa", "stress",
		"audio_url", "updated_at", "deleted"},
	domain.TableRoadmapPaths: {"guid", "slug", "language", "title", "overview", "is_builtin",
		"created_at", "updated_at", "deleted"},
	domain.TableRoadmapStages: {"guid", "slug", "title", "goal", "position", "duration_weeks",
		"status", "status_note", "created_at", "updated_at", "deleted", "completed_at",
		"terrain", "direction"},
	domain.TableRoadmapMilestones: {"guid", "text", "position", "created_at", "updated_at", "deleted"},
	domain.TableRoadmapTopics: {"guid", "title", "why", "activities", "position", "status",
		"status_note", "created_at", "updated_at", "deleted", "completed_at", "is_optional",
		"map_x", "map_y"},
	domain.TableRoadmapResources: {"guid", "title", "url", "kind", "note", "position",
		"created_at", "updated_at", "deleted"},
	domain.TableRoadmapBookmarks: {"guid", "title", "url", "note", "tags", "status",
		"created_at", "updated_at", "deleted"},
	domain.TableReviews: {"guid", "grade", "reviewed_at", "next_due_at"},
	domain.TableNotes:   {"guid", "text", "created_at"},
}

// Load reads the peer database into a domain.PeerSnapshot.
func (l *SchemaLoader) Load(ctx context.Context) (domain.PeerSnapshot, error) {
	if l == nil || l.peer == nil {
		return domain.PeerSnapshot{}, errors.New("chưa cấu hình nguồn snapshot peer: SchemaLoader không có pool đọc schema peer")
	}
	guidByTable := map[domain.Table]map[int64]string{}
	for _, spec := range snapshotTables {
		for _, fk := range spec.fks {
			if _, done := guidByTable[fk.parent]; done {
				continue
			}
			m, err := l.guidByID(ctx, fk.parent)
			if err != nil {
				return domain.PeerSnapshot{}, err
			}
			guidByTable[fk.parent] = m
		}
	}

	out := domain.PeerSnapshot{SchemaVersion: l.schemaVersion(ctx)}
	for _, spec := range snapshotTables {
		if err := ctx.Err(); err != nil {
			return domain.PeerSnapshot{}, fmt.Errorf("đọc snapshot: %w", err)
		}
		rows, err := l.readRows(ctx, spec, guidByTable)
		if err != nil {
			return domain.PeerSnapshot{}, err
		}
		out.Rows = append(out.Rows, rows...)
	}
	for _, r := range out.Rows {
		if err := ctx.Err(); err != nil {
			return domain.PeerSnapshot{}, fmt.Errorf("đọc snapshot: %w", err)
		}
		if r.UpdatedAt > out.MaxUpdatedAt {
			out.MaxUpdatedAt = r.UpdatedAt
		}
	}
	return out, nil
}

func (l *SchemaLoader) schemaVersion(ctx context.Context) int {
	var v int
	if err := l.peer.WithContext(ctx).Raw("SELECT MAX(version) FROM schema_migrations").Scan(&v).Error; err != nil {
		return 0
	}
	return v
}

func (l *SchemaLoader) guidByID(ctx context.Context, table domain.Table) (map[int64]string, error) {
	var rows []struct {
		ID   int64
		GUID string
	}
	if err := l.peer.WithContext(ctx).Table(string(table)).Select("id, guid").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc guid %s của snapshot: %w", table, err)
	}
	out := make(map[int64]string, len(rows))
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("đọc guid %s của snapshot: %w", table, err)
		}
		out[row.ID] = row.GUID
	}
	return out, nil
}

func (l *SchemaLoader) readRows(ctx context.Context, spec snapshotTable, guidByTable map[domain.Table]map[int64]string) ([]domain.Row, error) {
	cols := snapshotColumns[spec.table]
	if cols == nil {
		return nil, fmt.Errorf("bảng %s không có danh sách cột trong snapshot", spec.table)
	}
	selectList := "id, guid"
	if len(cols) > 0 {
		selectList += ", " + joinCols(cols)
	}
	for _, fk := range spec.fks {
		selectList += ", " + fk.column
	}
	var raw []map[string]any
	if err := l.peer.WithContext(ctx).Raw("SELECT " + selectList + " FROM " + string(spec.table)).
		Scan(&raw).Error; err != nil {
		return nil, fmt.Errorf("đọc %s của snapshot: %w", spec.table, err)
	}
	out := make([]domain.Row, 0, len(raw))
	for _, m := range raw {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("đọc %s của snapshot: %w", spec.table, err)
		}
		guid := anyToString(m["guid"])
		if guid == "" {
			continue
		}
		values := make(map[string]string, len(cols))
		for _, c := range cols {
			if m[c] == nil {
				continue
			}
			values[c] = anyToString(m[c])
		}
		r := domain.Row{
			Table:     spec.table,
			GUID:      guid,
			UpdatedAt: anyToString(m["updated_at"]),
			CreatedAt: anyToString(m["created_at"]),
			Deleted:   anyToInt(m["deleted"]),
			Values:    values,
		}
		for _, fk := range spec.fks {
			guid := guidByTable[fk.parent][anyToInt64(m[fk.column])]
			switch fk.field {
			case fkDeck:
				r.DeckGUID = guid
			case fkCard:
				r.CardGUID = guid
			default:
				r.ParentGUID = guid
			}
		}
		out = append(out, r)
	}
	return out, nil
}

func joinCols(cols []string) string {
	out := ""
	for i, c := range cols {
		if i > 0 {
			out += ", "
		}
		out += c
	}
	return out
}

// anyToString converts a driver value to its string representation.
func anyToString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case int16:
		return strconv.FormatInt(int64(t), 10)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	case float32:
		return formatFloat(float64(t))
	case float64:
		return formatFloat(t)
	case bool:
		if t {
			return "1"
		}
		return "0"
	default:
		return ""
	}
}

func anyToInt(v any) int {
	switch t := v.(type) {
	case int16:
		return int(t)
	case int32:
		return int(t)
	case int64:
		return int(t)
	case int:
		return t
	case float64:
		return int(t)
	case []byte:
		if s := string(t); s == "t" || s == "true" {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func anyToInt64(v any) int64 { return int64(anyToInt(v)) }

func formatInt(n int64) string { return strconv.FormatInt(n, 10) }
// formatFloat formats float values preserving precision for string comparison.
func formatFloat(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }
