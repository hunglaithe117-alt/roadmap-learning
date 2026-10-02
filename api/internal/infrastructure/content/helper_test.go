package contentinfra_test

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	app "langapp/internal/application/content"
	contentinfra "langapp/internal/infrastructure/content"
	"langapp/internal/platform/testdb"
)

var fixedNow = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

// newTestDB dựng schema Postgres tạm, chạy migration thật lên đó, và tự dọn
// sạch khi test xong. Toàn bộ dựng schema + khoá advisory `CREATE EXTENSION
// pg_trgm` + `search_path` nằm ở `internal/platform/testdb` — 1 bản duy nhất cho
// mọi package test DB (trước M3 là 3 bản sao).
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testdb.Open(t, context.Background())
}

// twoMachines dựng 2 schema trong CÙNG database làm "2 máy" — dùng cho test
// guid seed phải ỔN ĐỊNH (2 máy cùng import HSK1 cho ra cùng guid thì merge
// khớp, không nhân đôi thẻ).
//
// Cả 2 schema DÙNG CHUNG 1 `testdb.Session` — khoá advisory phải giữ suốt test
// vì extension `pg_trgm` là đối tượng cấp DATABASE (xem `testdb.MigrateLockKey`).
func twoMachines(t *testing.T) (local, peer *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	session := testdb.Acquire(t, ctx)
	local, _ = session.OpenSchema(t, ctx)
	peer, _ = session.OpenSchema(t, ctx)
	return local, peer
}

// newService dựng service đầy đủ: repository dict/en_dict/notes + adapter sang
// `srs` + dữ liệu tĩnh (HSK, PVO/TMRND, bút thuận, bài đọc).
func newService(t *testing.T, db *gorm.DB) *app.Service {
	t.Helper()
	repo := contentinfra.NewRepository(db)
	uow := contentinfra.NewUnitOfWork(db)
	srs := contentinfra.NewSrsAdapter(db)
	return app.NewService(repo, uow, srs, srs, srs, contentinfra.NewSeedContent(),
		func() time.Time { return fixedNow })
}
