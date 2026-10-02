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

// Seed loader — port từ `api/roadmap_seed.go` v1 (app v1 vẫn giữ nguyên
// loader cũ để SQLite của nó không đổi hành vi).
//
// QUY TẮC VÀNG (deepwork/roadmap-feature.md): chỉ INSERT khi natural key chưa
// tồn tại, TUYỆT ĐỐI không UPDATE row đã có. User sửa/xoá node rồi restart
// app không được seed mọc lại.
//
// Natural key:
//
//	path      → slug
//	stage     → (path_id, slug)
//	milestone → (stage_id, text)
//	topic     → (stage_id, title)
//	resource  → (topic_id, title)
//
// Khác v1: cả 2 file nạp trong 1 transaction, và terrain/direction được gán
// sẵn cho stage mới theo `domain.MapDefaults` (bản đồ game) — migration
// 00004 chỉ gán cho stage đã có sẵn trong DB.
//
// ── GUID SEED PHẢI ỔN ĐỊNH (quy tắc vàng, xem `application/content/ports.go`) ──
//
// Seed guid KHÔNG được là uuid4 ngẫu nhiên. Lý do: 2 máy cùng cài app mới đều
// chạy Seeder này; nếu path của máy A có guid `4a80…` và của máy B có
// `b091…` thì merge sẽ coi đó là 2 path khác nhau, insert path của peer rồi
// **không resolve được cha cho stage** ("không tìm thấy cha roadmap_paths
// guid=…") ⇒ rollback **cả** merge. Đó chính là blocker của cổng gate M3,
// reproduce được bằng chính `Seeder` thật trên 2 schema Postgres.
//
// Sửa: guid seed = uuid5 theo natural key (dùng chung namespace với
// `SeedDeckGUID`/`SeedCardGUID` — xem `seedGUID` bên dưới), nên 2 máy cùng seed
// ra cùng guid và merge khớp. Guid user tạo tay vẫn là uuid4 qua `app.NewGUID`.

// SeedFile là shape 1 file JSON seed.
type SeedFile struct {
	Language string `json:"language"`
	Title    string `json:"title"`
	Overview string `json:"overview"`
	// Slug tuỳ chọn — bỏ trống thì loader tự sinh từ Title.
	Slug   string      `json:"slug"`
	Stages []SeedStage `json:"stages"`
}

type SeedStage struct {
	ID            string      `json:"id"`
	Title         string      `json:"title"`
	Goal          string      `json:"goal"`
	DurationWeeks int         `json:"duration_weeks"`
	Milestones    []string    `json:"milestones"`
	Topics        []SeedTopic `json:"topics"`
}

type SeedTopic struct {
	Title      string         `json:"title"`
	Why        string         `json:"why"`
	Activities []string       `json:"activities"`
	Resources  []SeedResource `json:"resources"`
}

type SeedResource struct {
	Title string  `json:"title"`
	URL   *string `json:"url"`
	Kind  string  `json:"kind"`
	Note  string  `json:"note"`
}

// Seeder nạp roadmap_seed/*.json vào DB.
type Seeder struct {
	db      *gorm.DB
	log     *slog.Logger
	nowFn   app.NowFunc
	seedFS  fs.FS
	seedDir string
}

// NewSeeder dựng seeder đọc từ embed.FS của package `roadmap_seed` — đúng
// nguồn app dùng lúc boot.
func NewSeeder(db *gorm.DB, log *slog.Logger, nowFn app.NowFunc) *Seeder {
	return NewSeederFromFS(db, seed.FS, log, seed.Dir, nowFn)
}

// NewSeederFromFS dựng seeder từ FS bất kỳ. Tách khỏi NewSeeder để test nạp
// được cả trường hợp file JSON hỏng mà không phải sửa file embed thật.
func NewSeederFromFS(db *gorm.DB, seedFS fs.FS, log *slog.Logger, seedDir string, nowFn app.NowFunc) *Seeder {
	if nowFn == nil {
		nowFn = app.Clock
	}
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Seeder{db: db, log: log, nowFn: nowFn, seedFS: seedFS, seedDir: seedDir}
}

// Run nạp toàn bộ seed file. Idempotent: chạy nhiều lần cho cùng kết quả và
// không bao giờ ghi đè nội dung user đã sửa.
//
// Cả 2 file nạp trong 1 transaction: 1 file JSON hỏng ⇒ rollback toàn bộ, không
// để lại cây nửa vời (đúng như `applyRoadmapSeeds` v1).
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
		// path.Join chứ không cộng chuỗi: embed.FS yêu cầu path không có tiền
		// tố "./", nên seedDir = "." sẽ sinh "./roadmap_en.json" và fail đọc.
		raw, err := fs.ReadFile(s.seedFS, path.Join(s.seedDir, n))
		if err != nil {
			return nil, fmt.Errorf("đọc seed %s: %w", n, err)
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
	// language của seed là contract zh|en (user tạo path qua API thì nhận
	// chuỗi tự do — xem app.ValidateLanguage).
	if f.Language != "zh" && f.Language != "en" {
		return fmt.Errorf("seed %s: language chỉ nhận zh hoặc en", orUnknown(f.Language))
	}
	title, err := app.ValidateTitle(f.Title)
	if err != nil {
		return fmt.Errorf("seed %s: %s", orUnknown(f.Language), err.Error())
	}
	// Title trong seed là tiêu đề người đọc ("Tự học tiếng Trung ... → HSK 4"),
	// không phải slug → tự slugify. Field `slug` (tuỳ chọn) thắng nếu có.
	slug, err := app.ValidateSlug(domain.Slugify(firstNonEmpty(f.Slug, f.Title)))
	if err != nil {
		return fmt.Errorf("seed %s (%s): %s", title, f.Language, err.Error())
	}
	overview, err := app.ValidateText(f.Overview, 2000)
	if err != nil {
		return fmt.Errorf("seed %s: %s", slug, err.Error())
	}

	// Lookup theo natural key CÓ tombstone (xem `lookupNaturalKey`): user xoá
	// mềm path seed thì lần boot sau phải BỎ QUA chứ không insert lại —
	// vừa vi phạm quy tắc vàng "chỉ INSERT khi natural key chưa tồn tại", vừa
	// đụng UNIQUE `ux_roadmap_paths_slug` (tombstone vẫn giữ slug) và làm hỏng
	// boot.
	pathID, pathTombstoned, err := lookupNaturalKey(ctx, tx,
		"SELECT id, deleted FROM roadmap_paths WHERE slug = ?", slug)
	if err != nil {
		return err
	}
	if pathTombstoned {
		s.log.Info("roadmap seed: path đã bị xoá mềm, bỏ qua toàn bộ path",
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
		// id trong docs không phải slug (vd "G0 — Chữ viết") → tự sinh slug ổn
		// định thay vì fail cả file.
		stageSlug = fmt.Sprintf("stage-%d", idx+1)
		s.log.Warn("roadmap seed: stage id không phải slug, dùng slug tự sinh",
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
		// User xoá 1 stage giữa path: bỏ qua stage đó, KHÔNG hồi sinh. Insert
		// lại sẽ đụng `ux_roadmap_stages_path_slug` UNIQUE.
		s.log.Info("roadmap seed: stage đã bị xoá mềm, bỏ qua",
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
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		// Milestone/topic/resource KHÔNG có UNIQUE theo natural key, nên nếu
		// lookup bỏ qua tombstone thì seed sẽ insert bản sao trùng tiêu đề.
		// Vì vậy ở 3 tầng này coi BẤT KỲ row nào (sống hay tombstone) là "đã
		// có" → không insert lại. `id > 0` là row sống, `tombstoned` là
		// tombstone giữ chỗ; cả hai đều chặn insert.
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
		s.log.Info("roadmap seed: topic đã bị xoá mềm, bỏ qua",
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
	// URL rỗng / không phải http(s) → lưu NULL (UI hiện "chưa có link") thay vì
	// bịa link. Cảnh báo để data bug lộ ra thay vì im lặng mất link.
	u, uerr := app.ValidateURL(rs.URL)
	if uerr != nil && rs.URL != nil && strings.TrimSpace(*rs.URL) != "" {
		s.log.Warn("roadmap seed: resource url không dùng được, lưu NULL",
			"title", title, "path", pathSlug, "stage", stageSlug, "reason", uerr.Error())
	}
	kind, kerr := app.ValidateKind(rs.Kind)
	if kerr != nil {
		s.log.Warn("roadmap seed: resource kind không hợp lệ, lưu rỗng",
			"title", title, "kind", rs.Kind)
		kind = ""
	}
	note, err := app.ValidateText(rs.Note, 2000)
	if err != nil {
		return fmt.Errorf("seed %s/%s topic %s resource %s: %s",
			pathSlug, stageSlug, topicTitle, title, err.Error())
	}
	// Xem giải thích ở milestone: row sống hay tombstone đều chặn insert.
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

// ── Lookup helpers (chỉ đọc natural key, KHÔNG update) ─────────────────────

// lookupNaturalKey đọc 1 row theo natural key và trả (id, tombstoned).
//
// `query` PHẢI chiếu `id, deleted`. Trả về:
//   - (id, false)      row còn sống → dùng id.
//   - (0, true)        chỉ có tombstone (deleted = 1) → KHÔNG insert lại.
//   - (0, false)       natural key thật sự chưa dùng → được insert.
//
// Vì sao phải biết tombstone thay vì lọc thẳng `AND deleted = 0`: hai tầng có
// UNIQUE index trên natural key (`ux_roadmap_paths_slug`,
// `ux_roadmap_stages_path_slug`) và tombstone vẫn GIỮ key đó — lọc
// `deleted = 0` rồi insert sẽ đụng 23505 và làm hỏng boot. Ba tầng còn lại
// không có UNIQUE, nên lọc `deleted = 0` sẽ tạo bản sao trùng tiêu đề. Một
// query trả cả hai trạng thái xử lý được cả 4 trường hợp.
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

// ── GUID seed ổn định (uuid5 theo natural key) ─────────────────────────────
//
// Dùng CHUNG namespace `contentapp.SeedGUIDNamespace` với `SeedDeckGUID` /
// `SeedCardGUID`, và CHUNG hàm `uuid.NewSHA1` — không tự viết lại thuật toán,
// không tạo namespace riêng cho roadmap. Vì sao import `application/content`
// ở đây được: đó là nơi khai báo namespace, cùng kiểu với
// `infrastructure/content` đã import `application/roadmap`.
// Không dùng `unicode/norm` để chuẩn hoá (GOROOT máy build thiếu package đó —
// đã ghi ở bàn giao M1).
//
// `kind` là tiền tố tách miềng ("path", "stage", …) y hệt cách `SeedDeckGUID`
// dùng "deck:" — tránh 2 loại node trùng natural key ra cùng guid.
//
// `position` là chỉ số thứ tự trong danh sách cha, và là thứ duy nhất giữ
// `path`/`stage` khỏi việc lệ guid khi 2 máy có bộ file seed khác nhau:
//
//   - `path` → `ux_roadmap_paths_slug` UNIQUE ⇒ slug đã là natural key duy
//     nhất, truyền `position = 0` CỐ Ý. Truyền chỉ số thật sẽ gắn guid vào
//     *thứ tự file*, mà 2 máy có thể cài bộ file khác nhau.
//   - `stage` → `ux_roadmap_stages_path_slug` UNIQUE ⇒ (pathSlug, stageSlug)
//     đủ, `position = 0`.
//   - milestone / topic / resource → **KHÔNG** có UNIQUE theo natural key (xem
//     comment `lookupNaturalKey`), nên file seed về lý thuyết có thể chứa 2 mục
//     trùng tiêu đề trong cùng 1 stage/topic. Nếu chỉ khoá theo tiêu đề thì 2
//     mục đó sinh **cùng một guid** ⇒ `ux_*_guid` UNIQUE làm lần seed sau bị
//     skip ⇒ mất dữ liệu. Nối thêm `position` (thứ tự trong danh sách cha) để
//     mỗi mục có guid riêng. Đổi file seed (thêm 1 topic giữa danh sách) sẽ
//     đổi guid của các topic sau — chấp nhận được: natural key thật sự trùng
//     thì không có cách nào ổn định hơn, và dữ liệu seed chỉ đổi khi file đổi.
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
		return "(thiếu language)"
	}
	return s
}
