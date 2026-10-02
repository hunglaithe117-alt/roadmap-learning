package platform

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	contentapp "langapp/internal/application/content"
	contentinfra "langapp/internal/infrastructure/content"
)

// seedStep là 1 seeder boot: tên để log, và việc cần làm.
//
// Tên KHÔNG phải trang trí — nó là thứ duy nhất cho biết seeder nào hỏng khi
// `Seed` log lỗi. Vì vậy tên ở đây phải khớp với tên bước boot, không rút gọn
// ("hs" là gì? "srs" là gì?).
type seedStep struct {
	name string
	run  func(context.Context) error
}

// Seed chạy TOÀN BỘ seeder lúc boot — roadmap trước, rồi từ vựng.
//
// Trước M7c hàm này chỉ gọi `RoadmapSeeder.Run`, nên `content.SeedEnglish` và
// `ImportHSK` (đã có, đã test idempotent) **không bao giờ được gọi**: bản cài
// mới boot lên có roadmap nhưng `cards = 0`, các màn ôn trống tới khi user
// tự bấm nút import. `SeedEnglish` là nguồn của deck PVO/TMRND + `en_dict`,
// `ImportHSK` là nguồn của 4 deck HSK + `dict`.
//
// Hợp đồng 3 điều, cả 3 đều có test (`seed_test.go`):
//
//  1. IDEMPOTENT. Cả 3 đường seed đều "chèn khi natural key chưa có": roadmap
//     theo slug, HSK theo (deck, front) + dict theo chữ Hán, English theo
//     (deck, front) và `en_dict` chỉ nạp khi bảng còn trống. Không đường nào
//     UPDATE nội dung người dùng đã sửa — user sửa nghĩa 1 từ rồi restart
//     không được mất.
//  2. MỘT SEEDER HỎNG KHÔNG GIẾT APP. Lỗi được log kèm TÊN seeder rồi đi
//     tiếp seeder kế tiếp, và `Seed` trả error tổng hợp cho caller quyết
//     định. `cmd/langapp` chỉ log cảnh báo — app vẫn phục vụ được với dữ liệu
//     sẵn có, đúng hành vi `BootStep` đã có từ M3.
//  3. THỨ TỰ ĐỘC LẬP. Roadmap không đọc `cards`, `ImportHSK` không đọc
//     `roadmap_*`, nên thứ tự là quyết định hiển thị log chứ không phải ràng
//     buộc dữ liệu.
func (c *Container) Seed(ctx context.Context) error {
	if c == nil || c.RoadmapSeeder == nil || c.Content == nil {
		return fmt.Errorf("platform: chưa dựng đủ seeder (roadmap=%v content=%v)",
			c != nil && c.RoadmapSeeder != nil, c != nil && c.Content != nil)
	}

	steps := c.seedSteps()
	failed := make([]string, 0, len(steps))
	for _, s := range steps {
		err := c.BootStep(ctx, "seed "+s.name, s.run)
		if err == nil {
			continue
		}
		failed = append(failed, s.name)
		// Log ở ĐÂY chứ không đợi main: `main` chỉ thấy 1 error tổng hợp nên
		// không biết seeder nào hỏng, và `Seed` còn được gọi từ test.
		c.Logger.Error("seeder hỏng, app vẫn boot",
			slog.String("seeder", s.name), slog.String("err", err.Error()))
	}
	if len(failed) > 0 {
		return fmt.Errorf("platform: %d/%d seeder hỏng: %s",
			len(failed), len(steps), strings.Join(failed, ", "))
	}
	return nil
}

// seedSteps liệt kê seeder theo thứ tự chạy, kèm số liệu từng bước trong log.
func (c *Container) seedSteps() []seedStep {
	steps := []seedStep{{
		name: "roadmap",
		run:  func(ctx context.Context) error { return c.RoadmapSeeder.Run(ctx) },
	}}

	// 4 level HSK, mỗi level 1 deck + 1 bản ghi dict cho từng chữ Hán. Danh
	// sách lấy từ `contentinfra` vì đó là nơi duy nhất biết mình bundle được
	// level nào — thêm HSK5 vào seed data mà quên ở đây thì `ImportHSK`
	// vẫn chạy được khi user bấm tay, nhưng lúc boot thì không.
	for _, level := range contentinfra.HskLevels {
		lvl := level
		steps = append(steps, seedStep{
			name: "hsk/" + lvl,
			run: func(ctx context.Context) error {
				res, err := c.Content.ImportHSK(ctx, contentapp.ImportInput{Level: lvl})
				if err != nil {
					return err
				}
				c.Logger.Info("seed xong",
					slog.String("seeder", "hsk/"+lvl),
					slog.Int("cards_added", res.CardsAdded),
					slog.Int("cards_total", res.CardsTotal),
					slog.Int("dict_added", res.DictAdded))
				return nil
			},
		})
	}

	steps = append(steps, seedStep{
		name: "english",
		run: func(ctx context.Context) error {
			res, err := c.Content.SeedEnglish(ctx)
			if err != nil {
				return err
			}
			c.Logger.Info("seed xong",
				slog.String("seeder", "english"),
				slog.Int("en_dict", res.EnDict),
				slog.Int("pvo_added", res.PVOAdded),
				slog.Int("tmrnd_added", res.TMRNDAdded))
			return nil
		},
	})
	return steps
}
