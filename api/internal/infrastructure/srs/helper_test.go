package srsinfra_test

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	srsapp "langapp/internal/application/srs"
	srs "langapp/internal/infrastructure/srs"
	"langapp/internal/platform/testdb"
)

// newTestDB dựng schema Postgres tạm, chạy migration thật lên đó, và tự dọn
// sạch khi test xong. Toàn bộ dựng schema + khoá advisory `CREATE EXTENSION
// pg_trgm` + `search_path` nằm ở `internal/platform/testdb` — trước M3 đó là
// 3 bản sao (mỗi package test DB một bản) và hằng `migrateLockKey` phải sửa
// cùng lúc 3 chỗ.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testdb.Open(t, context.Background())
}

// newService dựng repo + uow + service với mốc thời gian cố định.
func newService(t *testing.T, db *gorm.DB, now time.Time) (*srsapp.Service, *gorm.DB) {
	t.Helper()
	repo := srs.NewRepository(db)
	uow := srs.NewUnitOfWork(db)
	svc := srsapp.NewService(repo, uow, func() time.Time { return now })
	return svc, db
}
