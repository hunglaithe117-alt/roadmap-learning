// Package syncinfra provides GORM models and repository implementations for sync.
package syncinfra

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	app "langapp/internal/application/sync"
	domain "langapp/internal/domain/sync"
	"langapp/internal/platform/txtx"
)

// unitOfWork implements app.UnitOfWork using GORM transactions.
type unitOfWork struct{ db *gorm.DB }

// NewUnitOfWork constructs a UnitOfWork on the provided GORM DB.
func NewUnitOfWork(db *gorm.DB) app.UnitOfWork { return &unitOfWork{db: db} }

func (u *unitOfWork) Do(ctx context.Context, fn func(app.Tx) error) error {
	return txtx.Do(ctx, u.db, fn)
}

// txOf returns the scoped *gorm.DB from tx, falling back to root if tx is nil.
func txOf(root *gorm.DB, tx app.Tx) (*gorm.DB, error) {
	return txtx.Of(root, tx)
}

// Repository implements app.Repository for local write and peer snapshot read.
type Repository struct {
	local *gorm.DB
	peer  *gorm.DB
}

// NewRepository constructs a repository using the local GORM DB.
func NewRepository(db *gorm.DB) *Repository { return &Repository{local: db} }

// NewPeerRepository constructs a two-sided repository with local DB and read-only peer DB.
func NewPeerRepository(local, peer *gorm.DB) *Repository {
	return &Repository{local: local, peer: peer}
}

var _ app.Repository = (*Repository)(nil)

// write returns the appropriate GORM DB for writes.
func (r *Repository) write(tx app.Tx) (*gorm.DB, error) { return txOf(r.local, tx) }

// read returns the local DB pool for independent reads.
func (r *Repository) read() *gorm.DB { return r.local }

// at returns the reading DB scoped to tx if active.
func (r *Repository) at(tx app.Tx) (*gorm.DB, error) { return r.write(tx) }

// txCtx combines at(tx) with context.
func (r *Repository) txCtx(ctx context.Context, tx app.Tx) (*gorm.DB, error) {
	db, err := r.at(tx)
	if err != nil {
		return nil, err
	}
	return db.WithContext(ctx), nil
}

// errNoRows standardizes record-not-found errors.
func errNoRows(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return app.ErrNotFound
	}
	return err
}

// ── Version + meta ──────────────────────────────────────────────────────────

// SchemaVersion returns the latest version from schema_migrations, or 0 if empty.
func (r *Repository) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	if err := r.read().WithContext(ctx).Raw("SELECT MAX(version) FROM schema_migrations").Scan(&v).Error; err != nil {
		return 0, fmt.Errorf("đọc schema version: %w", err)
	}
	return v, nil
}

// PeerSchemaVersion returns the schema version from the peer snapshot.
func (r *Repository) PeerSchemaVersion(ctx context.Context) (int, error) {
	if r == nil || r.peer == nil {
		return 0, errors.New("chưa cấu hình nguồn snapshot peer: repository không có pool đọc schema peer")
	}
	var v int
	if err := r.peer.WithContext(ctx).Raw("SELECT MAX(version) FROM schema_migrations").Scan(&v).Error; err != nil {
		return 0, fmt.Errorf("đọc schema version peer: %w", err)
	}
	return v, nil
}

// LastSyncAt returns sync_meta.last_sync_at, or empty if never synced.
func (r *Repository) LastSyncAt(ctx context.Context) (string, error) {
	var v string
	err := r.read().WithContext(ctx).Raw(
		"SELECT v FROM sync_meta WHERE k = 'last_sync_at'").Scan(&v).Error
	if err != nil {
		return "", nil
	}
	return v, nil
}

// SetLastSyncAt updates or inserts the sync timestamp.
func (r *Repository) SetLastSyncAt(ctx context.Context, tx app.Tx, now string) error {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return err
	}
	return db.Exec(
		`INSERT INTO sync_meta (k, v) VALUES ('last_sync_at', ?)
		 ON CONFLICT (k) DO UPDATE SET v = excluded.v`, now).Error
}

// ── Conflicts ───────────────────────────────────────────────────────────────

// LogConflict inserts a conflict record into sync_conflicts.
func (r *Repository) LogConflict(ctx context.Context, tx app.Tx, c domain.Conflict) error {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return err
	}
	return db.Exec(
		`INSERT INTO sync_conflicts (table_name, guid, winner, detail, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		string(c.Table), c.GUID, c.Winner, c.Detail, c.At).Error
}

// ListConflicts returns conflict records ordered by newest first.
func (r *Repository) ListConflicts(ctx context.Context, limit int) ([]domain.Conflict, error) {
	var rows []struct {
		Table, GUID, Winner, Detail, CreatedAt string
	}
	if err := r.read().WithContext(ctx).Raw(`
		SELECT table_name, guid, winner, detail, created_at
		FROM sync_conflicts ORDER BY id DESC LIMIT ?`, limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc log xung đột: %w", err)
	}
	out := make([]domain.Conflict, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Conflict{
			Table: domain.Table(row.Table), GUID: row.GUID, Winner: row.Winner,
			Detail: row.Detail, At: row.CreatedAt,
		})
	}
	return out, nil
}

// CountConflicts returns the total count of conflict records.
func (r *Repository) CountConflicts(ctx context.Context) (int, error) {
	var n int64
	if err := r.read().WithContext(ctx).Raw("SELECT COUNT(*) FROM sync_conflicts").Scan(&n).Error; err != nil {
		return 0, fmt.Errorf("đếm conflict: %w", err)
	}
	return int(n), nil
}
