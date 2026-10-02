package syncinfra_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	syncapp "langapp/internal/application/sync"
	syncinfra "langapp/internal/infrastructure/sync"
	"langapp/internal/platform/testdb"
)

var fixedNow = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

// twoMachines dựng 2 schema trong CÙNG database: `local` (máy này) và `peer`
// (máy kia).
//
// Đây là tương đương `ATTACH` của SQLite: Postgres không có `ATTACH`, nên
// snapshot peer là 1 schema riêng (M1 §3.8 ghi bắt buộc thiết kế lại cơ chế
// này; M7 sẽ nạp file `pg_restore` vào đúng chỗ đó, interface không đổi).
//
// Cả 2 schema DÙNG CHUNG 1 `testdb.Session` — khoá advisory phải giữ suốt test
// vì extension `pg_trgm` là đối tượng cấp DATABASE (xem `testdb.MigrateLockKey`).
// Schema đầu tiên (local) giữ extension, schema thứ hai (peer) thì không.
func twoMachines(t *testing.T) (local, peer *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	session := testdb.Acquire(t, ctx)
	local, _ = session.OpenSchema(t, ctx)
	peer, _ = session.OpenSchema(t, ctx)
	return local, peer
}

// newService dựng service merge trên 2 pool: `local` để ghi, `peer` chỉ đọc.
func newService(t *testing.T, local, peer *gorm.DB) *syncapp.Service {
	t.Helper()
	return syncapp.NewService(
		syncinfra.NewPeerRepository(local, peer),
		syncinfra.NewUnitOfWork(local),
		syncinfra.NewSchemaLoader(peer),
		func() time.Time { return fixedNow },
	)
}

func countRows(t *testing.T, db *gorm.DB, table string) int {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM "+table).Scan(&n).Error, "đếm %s", table)
	return int(n)
}
