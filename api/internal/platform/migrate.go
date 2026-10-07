package platform

import (
	"context"

	"gorm.io/gorm"

	"langapp/internal/migrate"
)

// MigrationStatus is an alias for migrate.Status.
type MigrationStatus = migrate.Status

// Migrate runs schema migrations using migrate.Up.
func Migrate(ctx context.Context, db *gorm.DB) (MigrationStatus, error) {
	return migrate.Up(ctx, db)
}
