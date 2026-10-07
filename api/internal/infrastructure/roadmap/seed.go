// Package roadmapinfra loads seed JSON files into PostgreSQL idempotently.
package roadmapinfra

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	contentapp "langapp/internal/application/content"
	app "langapp/internal/application/roadmap"
	domain "langapp/internal/domain/roadmap"
	seed "langapp/roadmap_seed"
)

// SeedFile represents the JSON structure of a roadmap seed file.
type SeedFile struct {
	Language string      `json:"language"`
	Title    string      `json:"title"`
	Overview string      `json:"overview"`
	Slug     string      `json:"slug"`
	Stages   []SeedStage `json:"stages"`
}

// SeedStage represents a stage in a seed file.
type SeedStage struct {
	ID            string      `json:"id"`
	Title         string      `json:"title"`
	Goal          string      `json:"goal"`
	DurationWeeks int         `json:"duration_weeks"`
	Milestones    []string    `json:"milestones"`
	Topics        []SeedTopic `json:"topics"`
}

// SeedTopic represents a topic in a seed file.
type SeedTopic struct {
	Title      string         `json:"title"`
	Why        string         `json:"why"`
	Activities []string       `json:"activities"`
	Resources  []SeedResource `json:"resources"`
}

// SeedResource represents a resource in a seed file.
type SeedResource struct {
	Title string  `json:"title"`
	URL   *string `json:"url"`
	Kind  string  `json:"kind"`
	Note  string  `json:"note"`
}

// Seeder loads roadmap seed files into PostgreSQL.
type Seeder struct {
	db      *gorm.DB
	log     *slog.Logger
	nowFn   app.NowFunc
	seedFS  fs.FS
	seedDir string
}

// NewSeeder constructs a Seeder using embedded files from package roadmap_seed.
func NewSeeder(db *gorm.DB, log *slog.Logger, nowFn app.NowFunc) *Seeder {
	return NewSeederFromFS(db, seed.FS, log, seed.Dir, nowFn)
}

// NewSeederFromFS constructs a Seeder from an arbitrary fs.FS.
func NewSeederFromFS(db *gorm.DB, seedFS fs.FS, log *slog.Logger, seedDir string, nowFn app.NowFunc) *Seeder {
	if nowFn == nil {
		nowFn = app.Clock
	}
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Seeder{db: db, log: log, nowFn: nowFn, seedFS: seedFS, seedDir: seedDir}
}

// Run loads all seed files into the database idempotently.
func (s *Seeder) Run(ctx context.Context) error {
	files, err := s.readSeedFiles()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := s.nowFn().UTC().Format(time.RFC3339)
		for _, f := range files {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("seed canceled: %w", err)
			}
			if err := s.seedOnePath(ctx, tx, f, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// readSeedFiles đọc + parse mọi *.json. Thứ tự sort theo tên để thứ tự insert
// ổn định giữa các lần boot.
func (s *Seeder) readSeedFiles() ([]SeedFile, error) {
	entries, err := fs.ReadDir(s.seedFS, s.seedDir)
	if err != nil {
		// Thư mục không tồn tại = chưa có seed → no-op, không lỗi.
		return nil, nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) == 0 {
		return nil, nil
	}
	sort.Strings(names)
	files := make([]SeedFile, 0, len(names))
	for _, n := range names {
		raw, err := fs.ReadFile(s.seedFS, path.Join(s.seedDir, n))
		if err != nil {
			return nil, fmt.Errorf("read seed %s: %w", n, err)
		}
		var f SeedFile
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("parse seed %s: %w", n, err)
		}
		files = append(files, f)
	}
	return files, nil
}

func (s *Seeder) seedOnePath(ctx context.Context, tx *gorm.DB, f SeedFile, now string) error {
	if f.Language != "zh" && f.Language != "en" {
		return fmt.Errorf("seed %s: language must be zh or en", orUnknown(f.Language))
	}
	title, err := app.ValidateTitle(f.Title)
	if err != nil {
		return fmt.Errorf("seed %s: %s", orUnknown(f.Language), err.Error())
	}
	slug, err := app.ValidateSlug(domain.Slugify(firstNonEmpty(f.Slug, f.Title)))
	if err != nil {
		return fmt.Errorf("seed %s (%s): %s", title, f.Language, err.Error())
	}
	overview, err := app.ValidateText(f.Overview, 2000)
	if err != nil {
		return fmt.Errorf("seed %s: %s", slug, err.Error())
	}

	pathID, pathTombstoned, err := lookupNaturalKey(ctx, tx,
		"SELECT id, deleted FROM roadmap_paths WHERE slug = ?", slug)
	if err != nil {
		return err
	}
	if pathTombstoned {
		s.log.Info("roadmap seed: path soft deleted, skipping",
			"slug", slug)
		return nil
	}
	if pathID == 0 {
		row := Path{
			Slug: slug, Language: f.Language, Title: title, Overview: overview,
			IsBuiltin: 1, CreatedAt: now,
			GUID: seedGUID("path", 0, slug), UpdatedAt: now,
		}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("seed path %s: %w", slug, err)
		}
		pathID = row.ID
	}
	for i, st := range f.Stages {
		if err := s.seedOneStage(ctx, tx, pathID, slug, f.Language, i, st, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Seeder) seedOneStage(ctx context.Context, tx *gorm.DB, pathID int64,
	pathSlug, lang string, idx int, st SeedStage, now string) error {

	stageSlug, err := app.ValidateSlug(st.ID)
	if err != nil {
		stageSlug = fmt.Sprintf("stage-%d", idx+1)
		s.log.Warn("roadmap seed: invalid stage id, generated fallback slug",
			"stage_id", st.ID, "path", pathSlug, "slug", stageSlug)
	}
	title, err := app.ValidateTitle(st.Title)
	if err != nil {
		return fmt.Errorf("seed %s/stage %s: %s", pathSlug, stageSlug, err.Error())
	}
	goal, err := app.ValidateText(st.Goal, 2000)
	if err != nil {
		return fmt.Errorf("seed %s/stage %s: %s", pathSlug, stageSlug, err.Error())
	}
	if st.DurationWeeks < 0 {
		st.DurationWeeks = 0
	}
	stageID, stageTombstoned, err := lookupNaturalKey(ctx, tx,
		"SELECT id, deleted FROM roadmap_stages WHERE path_id = ? AND slug = ?", pathID, stageSlug)
	if err != nil {
		return err
	}
	if stageTombstoned {
		s.log.Info("roadmap seed: stage soft deleted, skipping",
			"path", pathSlug, "stage", stageSlug)
		return nil
	}
	if stageID == 0 {
		terrain, direction := domain.MapDefaults(lang, idx)
		row := Stage{
			PathID: pathID, Slug: stageSlug, Title: title, Goal: goal,
			Position: idx, DurationWeeks: st.DurationWeeks, Status: app.StatusNotStarted,
			Terrain: string(terrain), Direction: string(direction),
			CreatedAt: now, GUID: seedGUID("stage", 0, pathSlug, stageSlug), UpdatedAt: now,
		}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("seed stage %s/%s: %w", pathSlug, stageSlug, err)
		}
		stageID = row.ID
	}
	for mi, text := range st.Milestones {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("seed %s/%s: %w", pathSlug, stageSlug, err)
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		id, tombstoned, err := lookupNaturalKey(ctx, tx,
			"SELECT id, deleted FROM roadmap_milestones WHERE stage_id = ? AND text = ?", stageID, text)
		if err != nil {
			return err
		}
		if id > 0 || tombstoned {
			continue
		}
		row := Milestone{
			StageID: stageID, Text: text, Position: mi,
			CreatedAt: now, GUID: seedGUID("milestone", mi, pathSlug, stageSlug, text),
			UpdatedAt: now,
		}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("seed milestone %s/%s: %w", pathSlug, stageSlug, err)
		}
	}
	for ti, tp := range st.Topics {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("seed %s/%s: %w", pathSlug, stageSlug, err)
		}
		if err := s.seedOneTopic(ctx, tx, stageID, pathSlug, stageSlug, ti, tp, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Seeder) seedOneTopic(ctx context.Context, tx *gorm.DB, stageID int64,
	pathSlug, stageSlug string, idx int, tp SeedTopic, now string) error {

	title, err := app.ValidateTitle(tp.Title)
	if err != nil {
		return fmt.Errorf("seed %s/%s topic %d: %s", pathSlug, stageSlug, idx, err.Error())
	}
	why, err := app.ValidateText(tp.Why, 2000)
	if err != nil {
		return fmt.Errorf("seed %s/%s topic %s: %s", pathSlug, stageSlug, title, err.Error())
	}
	topicID, topicTombstoned, err := lookupNaturalKey(ctx, tx,
		"SELECT id, deleted FROM roadmap_topics WHERE stage_id = ? AND title = ?", stageID, title)
	if err != nil {
		return err
	}
	if topicTombstoned {
		s.log.Info("roadmap seed: topic soft deleted, skipping",
			"path", pathSlug, "stage", stageSlug, "topic", title)
		return nil
	}
	if topicID == 0 {
		row := Topic{
			StageID: stageID, Title: title, Why: why,
			Activities: app.EncodeActivities(tp.Activities), Position: idx,
			Status: app.StatusNotStarted, CreatedAt: now,
			GUID:      seedGUID("topic", idx, pathSlug, stageSlug, title),
			UpdatedAt: now,
		}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("seed topic %s/%s: %w", pathSlug, stageSlug, err)
		}
		topicID = row.ID
	}
	for ri, rs := range tp.Resources {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("seed %s/%s: %w", pathSlug, stageSlug, err)
		}
		if err := s.seedOneResource(ctx, tx, topicID, pathSlug, stageSlug, title, ri, rs, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Seeder) seedOneResource(ctx context.Context, tx *gorm.DB, topicID int64,
	pathSlug, stageSlug, topicTitle string, idx int, rs SeedResource, now string) error {

	title, err := app.ValidateTitle(rs.Title)
	if err != nil {
		return fmt.Errorf("seed %s/%s topic %s resource %d: %s",
			pathSlug, stageSlug, topicTitle, idx, err.Error())
	}
	u, uerr := app.ValidateURL(rs.URL)
	if uerr != nil && rs.URL != nil && strings.TrimSpace(*rs.URL) != "" {
		s.log.Warn("roadmap seed: unusable resource url, saving NULL",
			"title", title, "path", pathSlug, "stage", stageSlug, "reason", uerr.Error())
	}
	kind, kerr := app.ValidateKind(rs.Kind)
	if kerr != nil {
		s.log.Warn("roadmap seed: invalid resource kind, saving empty",
			"title", title, "kind", rs.Kind)
		kind = ""
	}
	note, err := app.ValidateText(rs.Note, 2000)
	if err != nil {
		return fmt.Errorf("seed %s/%s topic %s resource %s: %s",
			pathSlug, stageSlug, topicTitle, title, err.Error())
	}
	id, tombstoned, err := lookupNaturalKey(ctx, tx,
		"SELECT id, deleted FROM roadmap_resources WHERE topic_id = ? AND title = ?", topicID, title)
	if err != nil {
		return err
	}
	if id > 0 || tombstoned {
		return nil
	}
	row := Resource{
		TopicID: topicID, Title: title, URL: u, Kind: kind, Note: note,
		Position: idx, CreatedAt: now,
		GUID:      seedGUID("resource", idx, pathSlug, stageSlug, topicTitle, title),
		UpdatedAt: now,
	}
	if err := tx.Create(&row).Error; err != nil {
		return fmt.Errorf("seed resource %s/%s: %w", pathSlug, stageSlug, err)
	}
	return nil
}

// lookupNaturalKey reads a row by natural key and returns (id, tombstoned).
func lookupNaturalKey(ctx context.Context, tx *gorm.DB, query string, args ...any) (id int64, tombstoned bool, err error) {
	var row struct {
		ID      int64
		Deleted int
	}
	scanErr := tx.WithContext(ctx).Raw(query, args...).Row().Scan(&row.ID, &row.Deleted)
	if scanErr != nil {
		if isNoRows(scanErr) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("seed lookup: %w", scanErr)
	}
	if row.Deleted != 0 {
		return 0, true, nil
	}
	return row.ID, false, nil
}

// seedGUID generates a deterministic UUIDv5 from natural key parts.
func seedGUID(kind string, position int, parts ...string) string {
	key := kind + ":" + strconv.Itoa(position) + ":" + strings.Join(parts, ":")
	return uuid.NewSHA1(uuid.MustParse(contentapp.SeedGUIDNamespace), []byte(key)).String()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(missing language)"
	}
	return s
}

