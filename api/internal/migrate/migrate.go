// Package migrate executes database schema migrations using goose.
package migrate

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"

	"langapp/migrations"
)

// Status represents the migration outcome.
type Status struct {
	Version int64
	Applied int
}

// Up runs all pending database migrations idempotently.
func Up(ctx context.Context, db *gorm.DB) (Status, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return Status{}, fmt.Errorf("migrate: get sql.DB: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migration.FS)
	if err != nil {
		return Status{}, fmt.Errorf("migrate: new goose provider: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("migrate: goose up: %w", err)
	}
	status := countApplied(results)
	slog.Info("migration completed",
		"version", status.Version, "applied", status.Applied)
	return status, nil
}

// countApplied inspects migration results to compute the final version and applied count.
func countApplied(results []*goose.MigrationResult) Status {

	if len(results) == 0 {
		return Status{Applied: 0}
	}
	last := results[len(results)-1]
	version := int64(0)
	if last.Source != nil {
		version = last.Source.Version
	}
	return Status{Version: version, Applied: len(results)}
}
