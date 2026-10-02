package platform

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	contentapp "langapp/internal/application/content"
	insightapp "langapp/internal/application/insight"
	practiceapp "langapp/internal/application/practice"
	roadmapapp "langapp/internal/application/roadmap"
	srsapp "langapp/internal/application/srs"
	syncapp "langapp/internal/application/sync"
	audioinfra "langapp/internal/infrastructure/audio"
	contentinfra "langapp/internal/infrastructure/content"
	insightinfra "langapp/internal/infrastructure/insight"
	practiceinfra "langapp/internal/infrastructure/practice"
	roadmapinfra "langapp/internal/infrastructure/roadmap"
	srsinfra "langapp/internal/infrastructure/srs"
	syncinfra "langapp/internal/infrastructure/sync"
)

// Container giữ toàn bộ dependency đã dựng.
//
// Thứ tự dựng KHÔNG đổi so với M1 (config → logger → DB pool → migration);
// M4 chỉ BỔ SUNG repository + service + audio client vào sau `Migrate`.
//
// Hướng phụ thuộc vẫn 1 chiều: infrastructure → application → domain
// (STACK-V2-PLAN §2). `platform` import cả hai vì nó là tầng DUY NHẤT được
// phép biết cả hai cùng lúc — nhiệm vụ của nó là gắn, không phải làm.
type Container struct {
	Config Config
	Logger *slog.Logger
	DB     *gorm.DB

	// Migration là trạng thái goose sau Build — test assert version.
	Migration MigrationStatus

	// ── Repository (infrastructure) ─────────────────────────────────────────
	SRSRepository      *srsinfra.Repository
	RoadmapRepository  *roadmapinfra.Repository
	ContentRepository  *contentinfra.Repository
	PracticeRepository *practiceinfra.Repository
	InsightRepository  *insightinfra.Repository
	SyncRepository     *syncinfra.Repository

	// RoadmapSeeder là bootstrap MỘT LẦN, KHÔNG phải dependency runtime.
	//
	// M3-final §9 yêu cầu rõ: "Không đưa Seeder vào `Container` dưới tên
	// repository — nó là bootstrap một lần, không phải dependency runtime. Nếu
	// M4 muốn giữ 1 entry point duy nhất thì để `Container.Seed(ctx)`". Ở đây
	// giữ CẢ HAI: trường để test assert Seeder đã dựng được, và `Seed(ctx)` là
	// entry point duy nhất mà main gọi.
	RoadmapSeeder *roadmapinfra.Seeder

	// ── Use case (application) ───────────────────────────────────────────────
	SRS      *srsapp.Service
	Content  *contentapp.Service
	Roadmap  *roadmapapp.Service
	Practice *practiceapp.Service
	Insight  *insightapp.Service
	Sync     *syncapp.Service

	// Audio là client gRPC tới `services/audio-service`, hoặc stub khi
	// `AUDIO_GRPC_ADDR` rỗng. `TTSSynth`/`STT` là 2 port mà handler REST và
	// use case `practice` bind.
	Audio *audioinfra.Engine
}

// Build dựng dependency theo thứ tự và trả về nil error nếu mọi bước thành
// công. Lỗi ở bất kỳ bước nào cũng đóng những gì đã mở trước đó — kể cả
// audio client, vì `grpc.ClientConn` giữ connection pool riêng.
func Build(ctx context.Context) (*Container, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	logger := NewLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	db, err := OpenPostgres(ctx, cfg)
	if err != nil {
		return nil, err
	}
	migration, err := Migrate(ctx, db)
	if err != nil {
		closeDB(db)
		return nil, err
	}

	c, err := Wire(ctx, db, logger, nil)
	if err != nil {
		closeDB(db)
		return nil, err
	}
	c.Config = cfg
	c.Migration = migration
	return c, nil
}

// Wire dựng repository + service + audio client trên 1 `*gorm.DB` có sẵn.
//
// EXPORT vì 2 lý do:
//  1. `Build` chỉ là "mở pool rồi gọi Wire" — giữ chuỗi boot đọc được.
//  2. Test transport cần wiring ĐÚNG NHÁNG trên schema Postgres tạm. Dựng lại
//     adapter trong test là 3 bản sao sẽ trôi khỏi nhau — và adapter sai thì
//     test xanh còn app hỏng, đúng loại bug test tưởng đang che.
func Wire(ctx context.Context, db *gorm.DB, logger *slog.Logger, audio *audioinfra.Engine) (*Container, error) {
	if logger == nil {
		logger = slog.Default()
	}
	c := &Container{Logger: logger, DB: db}

	// ── Audio client ────────────────────────────────────────────────────────
	// Dựng TRƯỚC service vì `practice.NewService` cần `STTPort` + `TTSPort` ngay
	// lúc khởi tạo. `NewFromEnvOS` trả STUB khi `AUDIO_GRPC_ADDR` rỗng nên
	// `docker compose up` chỉ với Postgres vẫn chạy được, và `go test ./...`
	// không cần service nào.
	if audio == nil {
		var err error
		audio, err = audioinfra.NewFromEnvOS()
		if err != nil {
			return nil, err
		}
	}
	c.Audio = audio

	// ── Repository ──────────────────────────────────────────────────────────
	srsRepo := srsinfra.NewRepository(db)
	roadmapRepo := roadmapinfra.NewRepository(db)
	contentRepo := contentinfra.NewRepository(db)
	practiceRepo := practiceinfra.NewRepository(db)
	insightRepo := insightinfra.NewRepository(db)
	syncRepo := syncinfra.NewPeerRepository(db, nil)
	c.SRSRepository = srsRepo
	c.RoadmapRepository = roadmapRepo
	c.ContentRepository = contentRepo
	c.PracticeRepository = practiceRepo
	c.InsightRepository = insightRepo
	c.SyncRepository = syncRepo
	c.RoadmapSeeder = roadmapinfra.NewSeeder(db, logger, roadmapapp.Clock)

	// ── Cross-context adapters ──────────────────────────────────────────────
	// `content` import HSK cần ghi `decks`/`cards` (bảng của `srs`) và tìm deck
	// theo tên; `roadmap` cần đọc `decks` để kiểm `deck_id` (A1). Cả hai đi
	// qua interface khai ở `application/<ctx>/ports.go`, KHÔNG qua
	// `srs.Repository` — STACK-V2-PLAN §2: context giao tiếp nhau bằng Go
	// interface, không bằng bảng.
	srsAdapter := contentinfra.NewSrsAdapter(db)

	// ── Service ─────────────────────────────────────────────────────────────
	// Không truyền `nowFn` ở đâu cả: mặc định của 6 service là UTC thật, và app
	// đã chốt mọi mốc ghi DB là UTC (cột TEXT, trigger sinh ở UTC). Truyền
	// `time.Now` ở đây sẽ tạo nguồn thời gian thứ hai cho 1 request, và mốc lệch
	// vài mili giây giữa 2 bản ghi là bug đã phải sửa ở M3.
	c.SRS = srsapp.NewService(srsRepo, srsinfra.NewUnitOfWork(db), nil)
	c.Roadmap = roadmapapp.NewService(roadmapRepo, roadmapinfra.NewUnitOfWork(db), &roadmapDeckReader{srs: c.SRS}, nil, roadmapapp.ViewBox{})
	c.Content = contentapp.NewService(contentRepo, contentinfra.NewUnitOfWork(db), srsAdapter, srsAdapter, srsAdapter, contentinfra.NewSeedContent(), nil)
	c.Practice = practiceapp.NewService(practiceRepo, practiceinfra.NewUnitOfWork(db), &sttPort{engine: audio}, &ttsPort{engine: audio}, nil)
	c.Insight = insightapp.NewService(insightRepo, nil)
	// `loader` truyền `nil` THẬT — KHÔNG phải `syncinfra.NewSchemaLoader(nil)`.
	//
	// Lý do (B2 của `stack-v2-m7a` §3.1, lần thứ 3 gặp lớp lỗi typed-nil
	// interface trong dự án):
	//
	//	NewSchemaLoader(nil) trả *SchemaLoader (CON TRỎ, ≠ nil)
	//	  → nhét vào tham số SnapshotLoader (interface) → interface KHÔNG nil
	//	  → `Service.Sync` thấy `s.loader == nil` SAI
	//	  → gọi `Load` với `peer` nil → `l.peer.WithContext(ctx)` deref
	//	    *gorm.DB nil → PANIC
	//
	// `mutation.sync` vì thế trả 500 "internal system error" thay vì lỗi nghiệp
	// vụ "chưa cấu hình nguồn snapshot peer" mà `Service.Sync` đã soạn sẵn.
	//
	// App v1 KHÔNG có nguồn snapshot peer: peer chỉ tồn tại trong test 2 schema
	// (`internal/infrastructure/sync/helper_test.go`) và sẽ là schema tạm sau
	// `pg_restore` ở M7. Vậy nên ở composition root này giá trị ĐÚNG của tham số
	// là "không có" — và "không có" của interface là `nil` thật, KHÔNG phải
	// con trỏ nil bọc trong interface. Truyền `nil` cũng giữ nguyên hợp đồng
	// `Service.Sync` đã viết sẵn cổng chặn ở `s.loader == nil`.
	//
	// KHÔNG xoá tham số `loader` khỏi `syncapp.NewService`: `Merge` vẫn cần nó
	// qua `Sync`, và test 2 máy vẫn truyền loader thật — bỏ đi là xoá hợp đồng
	// đang dùng.
	c.Sync = syncapp.NewService(syncRepo, syncinfra.NewUnitOfWork(db), nil, nil)

	_ = ctx
	return c, nil
}

// roadmapDeckReader là adapter `roadmap.DeckReader` → `srs.Service`.
//
// Vì sao cần adapter thay vì dùng thẳng `*srsapp.Service`: `srs.DeckInfo` và
// `roadmap.DeckInfo` là 2 struct CÐNG TÊN nhưng KHÁC KIỂU (2 bounded context
// khai độc lập để không import chéo — xem `application/roadmap/ports.go`). Nên
// `*srs.Service` không thoả `roadmap.DeckReader` về mặt cấu trúc.
//
// Gộp 3 thông tin (tồn tại / tên / ngôn ngữ) trong 1 lần gọi là CỐ Ý: chúng
// cùng 1 row, tách ra là 3 query cho 1 stage.
type roadmapDeckReader struct{ srs *srsapp.Service }

func (r *roadmapDeckReader) Find(ctx context.Context, id int64) (roadmapapp.DeckInfo, error) {
	info, err := r.srs.FindDeck(ctx, id)
	if err != nil {
		return roadmapapp.DeckInfo{}, err
	}
	return roadmapapp.DeckInfo{Exists: info.Exists, Name: info.Name, Lang: info.Lang}, nil
}

// sttPort là adapter `practice.STTPort` → `audioinfra.Engine`.
//
// `practice.STTPort.Transcribe` trả `practice.Transcript{Text, Lang}` — cắt bỏ
// word-level timestamp vì practice không dùng; `domain/audio.Transcript` có đủ
// nhưng interface khai của practice cố ý nhỏ hơn (xem `application/practice/
// ports.go`).
type sttPort struct{ engine *audioinfra.Engine }

func (p *sttPort) Transcribe(ctx context.Context, audio []byte, filename, contentType string) (practiceapp.Transcript, error) {
	res, err := p.engine.STTSTT.Transcribe(ctx, audio, filename, contentType)
	if err != nil {
		return practiceapp.Transcript{}, err
	}
	return practiceapp.Transcript{Text: res.Text, Lang: res.Lang}, nil
}

// ttsPort là adapter `practice.TTSPort` → `audioinfra.Engine`.
//
// `TTSPort.Synthesize` nhận `lang`; `TTSSynthesizer` (interface chính của
// `domain/audio`) không có tham số lang — voice theo ngôn ngữ là phần mở rộng
// `LangSynthesizer`. Không có phần mở rộng thì bỏ qua lang, dùng voice mặc
// định, KHÔNG phải lỗi (giữ hành vi v1 khi `ModelFor` rỗng).
type ttsPort struct{ engine *audioinfra.Engine }

func (p *ttsPort) Synthesize(ctx context.Context, text, lang string) ([]byte, string, error) {
	if ls := p.engine.LangSynthesizer(); ls != nil {
		return ls.SynthesizeLang(ctx, text, lang)
	}
	return p.engine.TTSSynth.Synthesize(ctx, text)
}

// `Container.Seed` — bootstrap roadmap + từ vựng lúc boot — nằm ở `seed.go`.

// Close đóng những gì `Build` đã mở, theo thứ tự ngược. main (cmd/langapp) gọi
// trong graceful shutdown. Nil-safe và idempotent để `defer c.Close()` không cần
// if.
func (c *Container) Close() error {
	if c == nil {
		return nil
	}
	var firstErr error
	if c.Audio != nil {
		if err := c.Audio.Close(); err != nil {
			firstErr = err
		}
	}
	if c.DB == nil {
		return firstErr
	}
	if err := closeDB(c.DB); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func closeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// SQLDB trả `*sql.DB` gốc từ pool GORM.
//
// `internal/transport/http` KHÔNG import gorm (luật tầng), nên health handler
// nhận interface `Pinger` và ta truyền `*sql.DB` vào. Đây là 1 chỗ duy nhất cần
// ép kiểu kiểu database/sql.
func (c *Container) SQLDB() (*sql.DB, error) {
	if c == nil || c.DB == nil {
		return nil, fmt.Errorf("platform: chưa mở connection")
	}
	return c.DB.DB()
}

// Deadline dùng cho các bước boot có trần thời gian (migrate, seed). Không có
// nó, 1 Postgres treo sẽ treo cả container và `docker compose up` không bao
// giờ báo lỗi.
const bootStepTimeout = 2 * time.Minute

// BootStep chạy 1 bước boot với deadline `bootStepTimeout`.
func (c *Container) BootStep(ctx context.Context, name string, fn func(context.Context) error) error {
	stepCtx, cancel := context.WithTimeout(ctx, bootStepTimeout)
	defer cancel()
	if err := fn(stepCtx); err != nil {
		return fmt.Errorf("platform: bước boot %q: %w", name, err)
	}
	return nil
}
