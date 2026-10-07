package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	domain "langapp/internal/domain/sync"
	"langapp/internal/typednil"
)

// Error represents a business error with an associated HTTP status code.
type Error struct {
	Status  int
	Message string
}

// Error returns the business error message.
func (e *Error) Error() string { return e.Message }

// StatusCode returns the HTTP status code.
func (e *Error) StatusCode() int { return e.Status }

// HTTP status codes defined locally to avoid importing net/http in the application layer.
const (
	StatusOK                  = 200
	StatusBadRequest          = 400
	StatusNotFound            = 404
	StatusConflict            = 409
	StatusInternalServerError = 500
	StatusNotImplemented     = 501
)

// ErrNotFound signals that a requested entity was not found.
var ErrNotFound = errors.New("không tìm thấy")

func newError(status int, format string, args ...any) *Error {
	return &Error{Status: status, Message: fmt.Sprintf(format, args...)}
}

// Service provides sync and merge operations.
type Service struct {
	repo   Repository
	uow    UnitOfWork
	loader SnapshotLoader
	now    NowFunc
}

// NewService constructs a sync Service.
func NewService(repo Repository, uow UnitOfWork, loader SnapshotLoader, nowFn NowFunc) *Service {
	if nowFn == nil {
		nowFn = Clock
	}
	return &Service{repo: repo, uow: uow, loader: loader, now: nowFn}
}

// Sync loads a peer snapshot and merges it into the local database.
func (s *Service) Sync(ctx context.Context) (MergeResult, error) {
	if typednil.Is(s.loader) {
		return MergeResult{}, newError(StatusNotImplemented, "chưa cấu hình nguồn snapshot peer")
	}
	snap, err := s.loader.Load(ctx)
	if err != nil {
		return MergeResult{}, fmt.Errorf("nạp snapshot peer: %w", err)
	}
	return s.Merge(ctx, snap)
}

// conflictSink collects merge conflicts and writes them into the database transaction.
type conflictSink struct {
	repo Repository
	tx   Tx
	all  []domain.Conflict
}

func (c *conflictSink) add(ctx context.Context, conflict *domain.Conflict) {
	if conflict == nil {
		return
	}
	_ = c.repo.LogConflict(ctx, c.tx, *conflict)
	c.all = append(c.all, *conflict)
}

// addSkipped logs rows skipped due to conflicting unique constraints under different GUIDs.
func (c *conflictSink) addSkipped(ctx context.Context, table domain.Table, guid, key, now string) {
	c.add(ctx, &domain.Conflict{
		Table: table, GUID: guid, Winner: domain.WinnerLocal,
		Detail: fmt.Sprintf("insert-skipped-duplicate key=%q — peer có row này nhưng "+
			"local đã có row trùng UNIQUE dưới guid khác; dữ liệu peer KHÔNG được ghi",
			key),
		At: now,
	})
}

// SyncStatus represents sync status metadata.
type SyncStatus struct {
	Enabled       bool
	Strategy      string
	LastSyncAt    string
	ConflictCount int
}

// MergeStrategy describes the resolution strategy.
const MergeStrategy = "last-write-win"

// Status reports the current sync engine status and conflict count.
func (s *Service) Status(ctx context.Context) (SyncStatus, error) {
	last, err := s.repo.LastSyncAt(ctx)
	if err != nil {
		return SyncStatus{}, fmt.Errorf("đọc last_sync_at: %w", err)
	}
	n, err := s.repo.CountConflicts(ctx)
	if err != nil {
		return SyncStatus{}, fmt.Errorf("đếm conflict: %w", err)
	}
	return SyncStatus{
		Enabled: true, Strategy: MergeStrategy, LastSyncAt: last, ConflictCount: n,
	}, nil
}

// Conflicts lists logged conflicts up to limit.
func (s *Service) Conflicts(ctx context.Context, limit int) ([]domain.Conflict, error) {
	if limit <= 0 {
		limit = 200
	}
	out, err := s.repo.ListConflicts(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("đọc log xung đột: %w", err)
	}
	return out, nil
}

// timestampNow returns the current UTC timestamp formatted as RFC3339.
func (s *Service) timestampNow() string { return s.now().UTC().Format(time.RFC3339) }
