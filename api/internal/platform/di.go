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

// Container holds application dependencies.
type Container struct {
	Config Config
	Logger *slog.Logger
	DB     *gorm.DB

	// Migration records the Goose migration status after Build.
	Migration MigrationStatus

	SRSRepository      *srsinfra.Repository
	RoadmapRepository  *roadmapinfra.Repository
	ContentRepository  *contentinfra.Repository
	PracticeRepository *practiceinfra.Repository
	InsightRepository  *insightinfra.Repository
	SyncRepository     *syncinfra.Repository

	// RoadmapSeeder runs one-time roadmap bootstrap seeding.
	RoadmapSeeder *roadmapinfra.Seeder

	SRS      *srsapp.Service
	Content  *contentapp.Service
	Roadmap  *roadmapapp.Service
	Practice *practiceapp.Service
	Insight  *insightapp.Service
	Sync     *syncapp.Service

	// Audio provides speech synthesis and recognition.
	Audio *audioinfra.Engine
}

// Build constructs and initializes all application dependencies.
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

	c, err := Wire(db, logger, nil)
	if err != nil {
		closeDB(db)
		return nil, err
	}
	c.Config = cfg
	c.Migration = migration
	return c, nil
}

// Wire constructs repositories, cross-context adapters, and services on an existing DB pool.
func Wire(db *gorm.DB, logger *slog.Logger, audio *audioinfra.Engine) (*Container, error) {
	if logger == nil {
		logger = slog.Default()
	}
	c := &Container{Logger: logger, DB: db}

	if audio == nil {
		var err error
		audio, err = audioinfra.NewFromEnvOS()
		if err != nil {
			return nil, err
		}
	}
	c.Audio = audio

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

	srsAdapter := contentinfra.NewSrsAdapter(db)

	c.SRS = srsapp.NewService(srsRepo, srsinfra.NewUnitOfWork(db), nil)
	c.Roadmap = roadmapapp.NewService(roadmapRepo, roadmapinfra.NewUnitOfWork(db), &roadmapDeckReader{srs: c.SRS}, nil, roadmapapp.ViewBox{})
	c.Content = contentapp.NewService(contentRepo, contentinfra.NewUnitOfWork(db), srsAdapter, srsAdapter, srsAdapter, contentinfra.NewSeedContent(), nil)
	c.Practice = practiceapp.NewService(practiceRepo, practiceinfra.NewUnitOfWork(db), &sttPort{engine: audio}, &ttsPort{engine: audio}, nil)
	c.Insight = insightapp.NewService(insightRepo, nil)
	c.Sync = syncapp.NewService(syncRepo, syncinfra.NewUnitOfWork(db), nil, nil)

	return c, nil
}

// roadmapDeckReader adapts srs.Service to roadmapapp.DeckReader.
type roadmapDeckReader struct{ srs *srsapp.Service }

func (r *roadmapDeckReader) Find(ctx context.Context, id int64) (roadmapapp.DeckInfo, error) {
	info, err := r.srs.FindDeck(ctx, id)
	if err != nil {
		return roadmapapp.DeckInfo{}, err
	}
	return roadmapapp.DeckInfo{Exists: info.Exists, Name: info.Name, Lang: info.Lang}, nil
}

// sttPort adapts audioinfra.Engine to practiceapp.STTPort.
type sttPort struct{ engine *audioinfra.Engine }

func (s *sttPort) Transcribe(ctx context.Context, audio []byte, filename, contentType string) (practiceapp.Transcript, error) {
	res, err := s.engine.STTSTT.Transcribe(ctx, audio, filename, contentType)
	if err != nil {
		return practiceapp.Transcript{}, err
	}
	return practiceapp.Transcript{Text: res.Text, Lang: res.Lang}, nil
}

// ttsPort adapts audioinfra.Engine to practiceapp.TTSPort.
type ttsPort struct{ engine *audioinfra.Engine }

func (t *ttsPort) Synthesize(ctx context.Context, text, lang string) ([]byte, string, error) {
	if ls := t.engine.LangSynthesizer(); ls != nil {
		return ls.SynthesizeLang(ctx, text, lang)
	}
	return t.engine.TTSSynth.Synthesize(ctx, text)
}

// Close gracefully closes all opened connections in reverse order.
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

// SQLDB returns the underlying *sql.DB connection pool.
func (c *Container) SQLDB() (*sql.DB, error) {
	if c == nil || c.DB == nil {
		return nil, fmt.Errorf("platform: connection not open")
	}
	return c.DB.DB()
}

const bootStepTimeout = 2 * time.Minute

// BootStep runs a startup step with a bounded timeout.
func (c *Container) BootStep(ctx context.Context, name string, fn func(context.Context) error) error {
	stepCtx, cancel := context.WithTimeout(ctx, bootStepTimeout)
	defer cancel()
	if err := fn(stepCtx); err != nil {
		return fmt.Errorf("platform: boot step %q: %w", name, err)
	}
	return nil
}

