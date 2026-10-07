package platform_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	audioinfra "langapp/internal/infrastructure/audio"
	contentinfra "langapp/internal/infrastructure/content"
	platform "langapp/internal/platform"
	platformtestdb "langapp/internal/platform/testdb"
)

// Seed PHẢI nạp từ vựng, không chỉ roadmap.
//
// Kịch bản gốc: `Container.Seed` chỉ gọi `RoadmapSeeder.Run`, còn
// `content.SeedEnglish` và `content.ImportHSK` (đã có, đã test idempotent) chỉ
// chạy khi user bấm nút ⇒ bản cài mới boot lên có roadmap nhưng `cards = 0`,
// mọi màn ôn trống. Test này khẳng định cả 2 phía: DB **có** thẻ, và **mọi**
// level HSK trong bundle đều được nạp (thêm HSK5 mà quên khai báo trong
// `seedSteps` thì đỏ).
func Test_seed_populates_vocabulary_decks_not_only_roadmap(t *testing.T) {
	db := platformtestdb.Open(t, context.Background())
	c := wireForSeed(t, db)

	require.NoError(t, c.Seed(context.Background()))

	decks, err := c.SRS.ListDecks(context.Background())
	require.NoError(t, err)
	names := make([]string, 0, len(decks))
	for _, d := range decks {
		names = append(names, d.Name)
	}
	require.Subset(t, names, contentinfra.HskLevels,
		"mọi level trong %v phải có deck sau 1 lần boot", contentinfra.HskLevels)
	require.Subset(t, names, []string{"PVO", "TMRND"},
		"SeedEnglish phải tạo deck PVO + TMRND lúc boot, không chờ user bấm nút")

	paths, err := c.Roadmap.ListPaths(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, paths, "roadmap vẫn phải được seed")
}

// Idempotent theo 2 nghĩa: chạy lại KHÔNG thêm dòng nào, và KHÔNG ghi đè nội
// dung user đã sửa. Chỉ kiểm "không nhân bản" thì vẫn lọt qua `UpsertSeedCard`
// ở nhánh hồi sinh tombstone — vốn cố tình ghi đè, nên phải khẳng định cả việc
// sửa nghĩa 1 thẻ rồi restart vẫn giữ nguyên.
func Test_second_boot_seeds_nothing_and_keeps_user_edits(t *testing.T) {
	db := platformtestdb.Open(t, context.Background())
	c := wireForSeed(t, db)
	ctx := context.Background()

	require.NoError(t, c.Seed(ctx))
	before, err := c.Content.SearchDict(ctx, "你好", 5)
	require.NoError(t, err)
	require.NotEmpty(t, before, "bản đầu phải có dữ liệu từ điển để sửa")
	cardsBefore := countAllCards(t, ctx, c)
	require.Positive(t, cardsBefore)

	// User sửa nghĩa 1 mục từ điển (nội dung học thuật seed, không phải nội
	// dung app) — lần boot sau KHÔNG được ghi đè.
	//
	// Sửa bằng SQL thẳng, KHÔNG qua `Content.UpsertDictEntry`: hàm đó cố ý
	// KHÔNG ghi đè mục đã tồn tại (`ports.go` — "chữ Hán đã có thì không ghi đè
	// nội dung cũ"), nên gọi nó ở đây sẽ là no-op và test sẽ xanh vô nghĩa —
	// đúng loại test tự lừa mình mà M7c đang vá.
	edited := before[0]
	require.NoError(t, db.Exec(
		"UPDATE dict SET nghia = ? WHERE hanzi = ?", "do ạ", edited.Hanzi).Error)

	require.NoError(t, c.Seed(ctx))

	cardsAfter := countAllCards(t, ctx, c)
	require.Equal(t, cardsBefore, cardsAfter,
		"lần seed thứ 2 không được tạo thêm hay xoá thẻ nào")

	stillEdited, err := c.Content.SearchDict(ctx, edited.Hanzi, 5)
	require.NoError(t, err)
	require.NotEmpty(t, stillEdited)
	require.Equal(t, "do ạ", stillEdited[0].Nghia,
		"seed lần 2 đã ghi đè nghĩa user sửa — vi phạm quy tắc vàng của seeder")
}

// countAllCards đếm tổng số thẻ qua MỌI deck.
//
// `SRS.ListCards(ctx, deckID)` chỉ liệt kê 1 deck, còn seed tạo 6 deck (4 HSK
// + PVO + TMRND) nên phải cộng lại — gọi với 1 `deckID` cứng định sẽ đếm sai
// và khiến assert "không nhân bản" so sánh 2 con số vô nghĩa.
func countAllCards(t *testing.T, ctx context.Context, c *platform.Container) int {
	t.Helper()
	decks, err := c.SRS.ListDecks(ctx)
	require.NoError(t, err)
	total := 0
	for _, d := range decks {
		cards, err := c.SRS.ListCards(ctx, d.ID)
		require.NoError(t, err, "đếm thẻ của deck %q", d.Name)
		total += len(cards)
	}
	return total
}

// `Seed` KHÔNG được ném bỏ lỗi của 1 seeder rồi bỏ các seeder sau: app vẫn
// phục vụ được, nhưng log phải chỉ ra ĐÚNG seeder nào hỏng (đó là thứ duy nhất
// cho biết roadmap hay từ vựng là nạn nhân).
func Test_seed_reports_every_failing_seeder_by_name(t *testing.T) {
	db := platformtestdb.Open(t, context.Background())
	c := wireForSeed(t, db)

	// Ngắn 1 seeder giữa chừng: xoá bảng `dict` khiến cả 4 import HSK hỏng,
	// nhưng roadmap (bảng riêng) và English (bảng `en_dict`) vẫn phải chạy.
	require.NoError(t, db.Exec("DROP TABLE dict").Error)

	err := c.Seed(context.Background())
	require.Error(t, err, "seeder hỏng phải được báo, im lặng là kiểu lỗi tệ nhất")
	for _, name := range []string{"hsk/HSK1", "hsk/HSK4"} {
		require.Contains(t, err.Error(), name,
			"error tổng hợp phải nêu tên seeder, không chỉ số lượng")
	}

	paths, err := c.Roadmap.ListPaths(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, paths, "seeder sau seeder hỏng vẫn phải chạy")

	decks, err := c.SRS.ListDecks(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, decks, "seeder English phải chạy dù HSK hỏng")
}

func wireForSeed(t *testing.T, db *gorm.DB) *platform.Container {
	t.Helper()
	audio, err := audioinfra.NewFromEnv("")
	require.NoError(t, err)
	t.Cleanup(func() { _ = audio.Close() })
	c, err := platform.Wire(db, testLogger(), audio)
	require.NoError(t, err)
	return c
}
