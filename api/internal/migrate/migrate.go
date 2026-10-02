// Package migrate là nơi DUY NHẤT chạy migration Postgres (goose).
//
// Vì sao tách khỏi `internal/platform`: từ M4, `platform/di.go` import
// `internal/infrastructure/*` để dựng repository + service. Mà
// `internal/platform/testdb` lại import `platform` để gọi `Migrate` — nên
// in-package test của `infrastructure/{roadmap,srs}` (cần nắm `txHandle` không
// export) sẽ tạo vòng:
//
//	roadmapinfra (test) → platform/testdb → platform → roadmapinfra
//
// Tách `Migrate` ra package này giải quyết đúng gốc: `testdb` import
// `migrate` (không import infrastructure), còn `platform` import cả hai. Không
// có cách nào khác mà không phải sửa 2 file test nội bộ của M2.
package migrate

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"

	"langapp/migrations"
)

// File .sql nằm ở `api/migrations/` chứ không phải dưới `internal/` vì đó là
// nơi review DDL (STACK-V2-PLAN §6 M1 gate: oracle review DDL). Go embed không
// đi lên được khỏi thư mục package, nên `api/migrations/` là package riêng chỉ
// việc export `embed.FS`.

// Status là kết quả Up để caller log/kiểm tra, tách khỏi hàm chạy migration để
// test assert được mà không cần đọc log.
type Status struct {
	// Version là version cuối trong goose_db_version sau Up.
	Version int64
	// Applied là số migration mới được apply ở lần gọi này (0 = idempotent).
	Applied int
}

// Up là ENTRYPOINT DUY NHẤT chạy migration. Không có đường nào khác tạo bảng:
// goose tự quản `goose_db_version` nên chạy lại 2 lần là no-op (STACK-V2-PLAN §6
// M1 exit: "migration up 2 lần idempotent").
func Up(ctx context.Context, db *gorm.DB) (Status, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return Status{}, fmt.Errorf("migrate: lấy sql.DB cho goose: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		return Status{}, fmt.Errorf("migrate: tạo goose provider: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("migrate: goose up: %w", err)
	}
	status := countApplied(results)
	slog.Info("migration xong",
		"version", status.Version, "applied_lần_này", status.Applied)
	return status, nil
}

// countApplied đọc version cuối trong goose_db_version sau Up và đếm số migration
// vừa apply. results rỗng = không có migration mới (lần chạy lại).
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
