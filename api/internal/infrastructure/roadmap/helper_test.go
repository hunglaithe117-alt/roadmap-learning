package roadmapinfra_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	roadmapapp "langapp/internal/application/roadmap"
	srsapp "langapp/internal/application/srs"
	roadmapinfra "langapp/internal/infrastructure/roadmap"
	srsinfra "langapp/internal/infrastructure/srs"
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

// discardLogger là logger mọi test dùng — seeder gọi slog.Warn khi seed file có
// dữ liệu lỗi, và test không muốn in ra stdout.
var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// newService dựng repo + uow + service với mốc thời gian cố định và port deck
// rỗng (deck check chỉ dùng khi PATCH/POST có deck_id).
func newService(t *testing.T, db *gorm.DB, now time.Time) *roadmapapp.Service {
	t.Helper()
	repo := roadmapinfra.NewRepository(db)
	uow := roadmapinfra.NewUnitOfWork(db)
	return roadmapapp.NewService(repo, uow, nil, func() time.Time { return now },
		roadmapapp.ViewBox{})
}

// newServiceWithDecks dựng service CÓ port `roadmap.DeckReader` bind vào srs
// thật trên cùng pool — đúng cách platform/di.go sẽ wire ở M4. Dùng cho test
// cần kiểm tra `deck_id` (A1).
func newServiceWithDecks(t *testing.T, db *gorm.DB, now time.Time) *roadmapapp.Service {
	t.Helper()
	srsSvc := srsapp.NewService(
		srsinfra.NewRepository(db), srsinfra.NewUnitOfWork(db),
		func() time.Time { return now })
	return roadmapapp.NewService(
		roadmapinfra.NewRepository(db), roadmapinfra.NewUnitOfWork(db),
		deckReaderAdapter{srsSvc}, func() time.Time { return now }, roadmapapp.ViewBox{})
}

// deckReaderAdapter nối `srsapp.Service.FindDeck` vào port `roadmap.DeckReader`.
// 2 context khai báo port riêng để không import chéo (xem
// application/roadmap/ports.go), nên chỗ nối là 1 adapter nhỏ ở infrastructure.
type deckReaderAdapter struct{ srs *srsapp.Service }

func (a deckReaderAdapter) Find(ctx context.Context, id int64) (roadmapapp.DeckInfo, error) {
	info, err := a.srs.FindDeck(ctx, id)
	if err != nil {
		return roadmapapp.DeckInfo{}, err
	}
	return roadmapapp.DeckInfo{Exists: info.Exists, Lang: info.Lang}, nil
}

// buildSingleTopicStage tạo 1 stage với đúng 1 topic — dùng cho test
// `progress?since=` nơi cần so node "xong hôm nay" với node "xong 1 năm trước".
func buildSingleTopicStage(t *testing.T, svc *roadmapapp.Service, pathSlug, stageSlug string) (roadmapapp.Stage, []roadmapapp.Topic) {
	t.Helper()
	ctx := context.Background()
	stage, err := svc.CreateStage(ctx, pathSlug, roadmapapp.StageInput{Slug: stageSlug, Title: stageSlug})
	require.NoError(t, err)
	tp, err := svc.CreateTopic(ctx, stage.ID, roadmapapp.TopicInput{Title: "Màn 1"})
	require.NoError(t, err)
	return stage, []roadmapapp.Topic{tp}
}

// newSeeder dựng seeder đọc từ embed.FS thật (cùng FS app dùng lúc boot).
func newSeeder(t *testing.T, db *gorm.DB, now time.Time) *roadmapinfra.Seeder {
	t.Helper()
	return roadmapinfra.NewSeeder(db, discardLogger, func() time.Time { return now })
}

// listTopics đọc topic của 1 stage thẳng từ repository — test layout không đi
// qua GetPath để không phụ thuộc vào việc dựng cả cây.
func listTopics(t *testing.T, db *gorm.DB, stageID int64) []roadmapapp.Topic {
	t.Helper()
	rows, err := roadmapinfra.NewRepository(db).ListTopics(context.Background(), stageID)
	require.NoError(t, err)
	return rows
}
