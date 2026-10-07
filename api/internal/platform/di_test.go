package platform_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	audioinfra "langapp/internal/infrastructure/audio"
	platform "langapp/internal/platform"
	platformtestdb "langapp/internal/platform/testdb"
)

// Test Container lắp đủ 6 service + repository + Seeder.
//
// Đây là test chặn DI thiếu: thiếu 1 service thì app vẫn build xanh và chạy được
// tới lúc có request chạm vào context đó — lúc đó lỗi là nil-pointer panic,
// tức 500 chứ không phải thông báo rõ. `cmd/langapp` đọc đúng các trường này.
func Test_wire_constructs_every_service_repository_and_seeder(t *testing.T) {
	db := platformtestdb.Open(t, context.Background())
	audio, err := audioinfra.NewFromEnv("")
	require.NoError(t, err)
	t.Cleanup(func() { _ = audio.Close() })

	c, err := platform.Wire(db, testLogger(), audio)
	require.NoError(t, err)

	require.NotNil(t, c.SRS, "context srs chưa được lắp")
	require.NotNil(t, c.Content, "context content chưa được lắp")
	require.NotNil(t, c.Roadmap, "context roadmap chưa được lắp")
	require.NotNil(t, c.Practice, "context practice chưa được lắp")
	require.NotNil(t, c.Insight, "context insight chưa được lắp")
	require.NotNil(t, c.Sync, "context sync chưa được lắp")

	require.NotNil(t, c.SRSRepository)
	require.NotNil(t, c.RoadmapRepository)
	require.NotNil(t, c.ContentRepository)
	require.NotNil(t, c.PracticeRepository)
	require.NotNil(t, c.InsightRepository)
	require.NotNil(t, c.SyncRepository)

	require.NotNil(t, c.RoadmapSeeder,
		"roadmap.Seeder phải được dựng (M3-final §9 cho phép M4 gắn vào DI)")
	require.NotNil(t, c.Audio)
}

// `roadmap` cần đọc `decks` qua port; adapter phải trả đúng tên + ngôn ngữ, và
// trả `Exists=false` cho id không có CHỨ KHÔNG phải lỗi — "không có" là kết quả
// hợp lệ (xem `roadmap.DeckReader`).
func Test_roadmap_can_read_deck_through_srs_port(t *testing.T) {
	db := platformtestdb.Open(t, context.Background())
	audio, err := audioinfra.NewFromEnv("")
	require.NoError(t, err)
	t.Cleanup(func() { _ = audio.Close() })

	c, err := platform.Wire(db, testLogger(), audio)
	require.NoError(t, err)
	ctx := context.Background()

	deck, err := c.SRS.CreateDeck(ctx, "HSK1", "zh")
	require.NoError(t, err)

	// Gắn deck vào stage rồi đọc cây: `StageView.Deck` phải có TÊN thật, không
	// phải chỉ id — nút "vào /review" của M6 cần tên.
	_, err = c.Roadmap.CreatePath(ctx, pathInput("zh"))
	require.NoError(t, err)
	stage, err := c.Roadmap.CreateStage(ctx, "zh", stageInputWithDeck(deck.ID))
	require.NoError(t, err)

	view, err := c.Roadmap.PathTree(ctx, "zh")
	require.NoError(t, err)
	require.Len(t, view.Stages, 1)
	require.NotNil(t, view.Stages[0].Deck)
	require.Equal(t, "HSK1", view.Stages[0].Deck.Name)
	require.Equal(t, "zh", view.Stages[0].Deck.Lang)
	require.Equal(t, deck.ID, view.Stages[0].Deck.ID)
	require.Equal(t, stage.ID, view.Stages[0].ID)
}

// `content` ghi `decks`/`cards` qua adapter; import HSK phải tạo được deck + thẻ
// mà không cần chạm bảng `decks` trực tiếp.
func Test_content_import_hsk_writes_through_srs_adapter(t *testing.T) {
	db := platformtestdb.Open(t, context.Background())
	audio, err := audioinfra.NewFromEnv("")
	require.NoError(t, err)
	t.Cleanup(func() { _ = audio.Close() })

	c, err := platform.Wire(db, testLogger(), audio)
	require.NoError(t, err)
	ctx := context.Background()

	res, err := c.Content.ImportHSK(ctx, hskImport())
	require.NoError(t, err)
	require.Positive(t, res.CardsAdded)

	decks, err := c.SRS.ListDecks(ctx)
	require.NoError(t, err)
	require.Len(t, decks, 1, "deck phải được tạo qua adapter, không phải ghi tay")
	require.Equal(t, "HSK1", decks[0].Name)
}

// Seed phải IDEMPOTENT: gọi 2 lần thì lần 2 không ghi gì (chạy lại lúc boot là
// chuyện thường, và lần 2 ghi lại sẽ đụng UNIQUE).
func Test_seed_is_idempotent_on_second_run(t *testing.T) {
	db := platformtestdb.Open(t, context.Background())
	audio, err := audioinfra.NewFromEnv("")
	require.NoError(t, err)
	t.Cleanup(func() { _ = audio.Close() })

	c, err := platform.Wire(db, testLogger(), audio)
	require.NoError(t, err)
	ctx := context.Background()

	require.NoError(t, c.Seed(ctx))
	first, err := c.Roadmap.ListPaths(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, first, "seed phải nạp path từ bundle embed")

	require.NoError(t, c.Seed(ctx))
	second, err := c.Roadmap.ListPaths(ctx)
	require.NoError(t, err)
	require.Len(t, second, len(first), "lần seed 2 không được tạo thêm hay xoá path nào")
}

// Bước seed KHÔNG được làm hỏng app: `Seed` lỗi trả error, còn `main` tự quyết
// định log cảnh báo. Test này chỉ khẳng định container dùng được khi seed lỗi.
func Test_services_still_work_when_seed_was_never_run(t *testing.T) {
	db := platformtestdb.Open(t, context.Background())
	audio, err := audioinfra.NewFromEnv("")
	require.NoError(t, err)
	t.Cleanup(func() { _ = audio.Close() })

	c, err := platform.Wire(db, testLogger(), audio)
	require.NoError(t, err)

	paths, err := c.Roadmap.ListPaths(ctx())
	require.NoError(t, err)
	require.Empty(t, paths, "DB mới phải rỗng, không có seed ẩn nào chạy lúc Wire")
}

// `SQLDB` trả `*sql.DB` cho health handler mà transport KHÔNG import gorm.
func Test_sql_db_is_available_for_pinger_interface(t *testing.T) {
	db := platformtestdb.Open(t, context.Background())
	c, err := platform.Wire(db, testLogger(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Audio.Close() })

	sqlDB, err := c.SQLDB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.PingContext(ctx()))
}

func ctx() context.Context { return context.Background() }
