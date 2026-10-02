package roadmap

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	domain "langapp/internal/domain/roadmap"
)

// Service là use case của context roadmap. Giữ 1 struct cho toàn bộ CRUD vì các
// use case dùng chung repository + uow + clock; tách 5 struct sẽ tăng file mà
// không tách được trách nhiệm nào (M4 bind cả lên 1 resolver theo context).
type Service struct {
	repo    Repository
	uow     UnitOfWork
	decks   DeckReader
	nowFn   NowFunc
	viewBox ViewBox
}

// ViewBox là kích thước canvas bản đồ dùng để validate toạ độ user đặt tay.
// Lấy từ domain để 1 nguồn sự thật với ComputeLayout.
type ViewBox struct {
	Width  float64
	Height float64
}

// NewService dựng service. nowFn nil → UTC thật. viewBox zero → mặc định domain.
func NewService(repo Repository, uow UnitOfWork, decks DeckReader, nowFn NowFunc, viewBox ViewBox) *Service {
	if nowFn == nil {
		nowFn = Clock
	}
	if viewBox.Width == 0 || viewBox.Height == 0 {
		viewBox = ViewBox{Width: domain.MapViewWidth, Height: domain.MapViewHeight}
	}
	return &Service{repo: repo, uow: uow, decks: decks, nowFn: nowFn, viewBox: viewBox}
}

// timestamp là mốc UTC RFC3339 để ghi vào cột TEXT. Mọi cột thời gian trong
// schema là TEXT (giữ nguyên từ SQLite v4) nên format ở đây, không phải ở
// repository — đổi format là đổi contract của cả DB, phải ở 1 chỗ.
func (s *Service) timestamp() string { return s.nowFn().UTC().Format(time.RFC3339) }

// ── Path ────────────────────────────────────────────────────────────────────

// PathInput là input CreatePath.
type PathInput struct {
	Slug     string
	Title    string
	Overview string
	Language string
}

// PathSummary là 1 dòng của ListPaths, kèm progress (UI hiện "2/25 · 8%" ngay
// trong danh sách).
type PathSummary struct {
	Path
	Progress Summary
}

// Summary là số liệu tiến độ rút gọn cho list. Percent chỉ tính topic bắt buộc
// (A1 — node optional không nằm trong mẫu số).
type Summary struct {
	Stages           int `json:"stages"`
	TopicsTotal      int `json:"topics_total"`
	TopicsRequired   int `json:"topics_required"`
	TopicsDone       int `json:"topics_done"`
	TopicsInProgress int `json:"topics_in_progress"`
	Percent          int `json:"percent"`
}

// PathPatch là input UpdatePath — nil = không đụng. Không cho đổi slug: slug là
// natural key của seed, đổi nó khiến seed tạo path trùng ở lần boot sau.
type PathPatch struct {
	Title    *string
	Overview *string
	Language *string
}

// CreatePath tạo learning path. Slug trùng → 409.
func (s *Service) CreatePath(ctx context.Context, in PathInput) (Path, error) {
	slug, err := ValidateSlug(in.Slug)
	if err != nil {
		return Path{}, err
	}
	title, err := ValidateTitle(in.Title)
	if err != nil {
		return Path{}, err
	}
	overview, err := ValidateText(in.Overview, 2000)
	if err != nil {
		return Path{}, err
	}
	lang, err := ValidateLanguage(in.Language)
	if err != nil {
		return Path{}, err
	}

	now := s.timestamp()
	var created Path
	err = s.inTx(ctx, func(tx Tx) error {
		created = Path{
			Slug: slug, Language: lang, Title: title, Overview: overview,
			CreatedAt: now, GUID: NewGUID(), UpdatedAt: now,
		}
		if err := s.repo.CreatePath(ctx, tx, &created); err != nil {
			if isUniqueViolation(err) {
				return newError(StatusConflict, "slug learning path đã tồn tại")
			}
			return fmt.Errorf("tạo learning path: %w", err)
		}
		return nil
	})
	return created, err
}

// ListPaths trả mọi path (deleted = 0) kèm tiến độ RIÊNG của từng path.
func (s *Service) ListPaths(ctx context.Context) ([]PathSummary, error) {
	paths, err := s.repo.ListPaths(ctx)
	if err != nil {
		return nil, fmt.Errorf("đọc danh sách learning path: %w", err)
	}
	ids := make([]int64, 0, len(paths))
	for _, p := range paths {
		ids = append(ids, p.ID)
	}
	// PHẢI là `ProgressByPathIDs` (per-path), KHÔNG phải `ProgressByIDs` (gộp).
	// `ProgressByIDs` gộp MỌI path thành 1 con số rồi gán cùng số đó cho từng
	// dòng — mọi path hiện cùng 1 tổng, sai hoàn toàn về mặt nghiệp vụ. Hồi quy
	// do M4 (khi đổi `progressOf` → batch để hết N+1) mà test chỉ dùng 1 path nên
	// aggregate = per-path ⇒ vô tình xanh. Xem `Test_list_paths_gives_each_path_its_own_progress`.
	perPath, err := s.ProgressByPathIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]PathSummary, 0, len(paths))
	for _, p := range paths {
		prog := perPath[p.ID]
		out = append(out, PathSummary{Path: p, Progress: Summary{
			Stages:           prog.Stages,
			TopicsTotal:      prog.TopicsTotal,
			TopicsRequired:   prog.TopicsRequired,
			TopicsDone:       prog.TopicsDone,
			TopicsInProgress: prog.TopicsInProgress,
			Percent:          prog.Percent,
		}})
	}
	return out, nil
}

// UpdatePath sửa title/overview/language.
func (s *Service) UpdatePath(ctx context.Context, slug string, patch PathPatch) (Path, error) {
	slug, err := ValidateSlug(slug)
	if err != nil {
		return Path{}, err
	}
	if patch.Title == nil && patch.Overview == nil && patch.Language == nil {
		return Path{}, newError(StatusBadRequest, "không có gì để cập nhật")
	}
	var updated Path
	err = s.inTx(ctx, func(tx Tx) error {
		p, err := s.repo.PathBySlug(ctx, slug)
		if err != nil {
			return wrapNotFound(err, "không tìm thấy learning path")
		}
		if patch.Title != nil {
			if p.Title, err = ValidateTitle(*patch.Title); err != nil {
				return err
			}
		}
		if patch.Overview != nil {
			if p.Overview, err = ValidateText(*patch.Overview, 2000); err != nil {
				return err
			}
		}
		if patch.Language != nil {
			if p.Language, err = ValidateLanguage(*patch.Language); err != nil {
				return err
			}
		}
		if err := s.repo.UpdatePath(ctx, tx, &p); err != nil {
			return fmt.Errorf("cập nhật learning path: %w", err)
		}
		updated = p
		return nil
	})
	return updated, err
}

// DeletePath xoá mềm path và toàn bộ nhánh con (cascade do repository thực hiện
// trong cùng transaction) để sync lan truyền tombstone. 404 nếu path đã xoá.
func (s *Service) DeletePath(ctx context.Context, slug string) error {
	slug, err := ValidateSlug(slug)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(tx Tx) error {
		p, err := s.repo.PathBySlug(ctx, slug)
		if err != nil {
			return wrapNotFound(err, "không tìm thấy learning path")
		}
		if err := s.repo.SoftDeletePath(ctx, tx, p.ID); err != nil {
			return fmt.Errorf("xoá learning path: %w", err)
		}
		return nil
	})
}

// ── Stage ───────────────────────────────────────────────────────────────────

// StageInput là input CreateStage.
type StageInput struct {
	Slug          string
	Title         string
	Goal          string
	Position      *int
	DurationWeeks *int
	Status        *string
	// A1 + bản đồ: DeckID gắn deck ôn (nil = chưa gắn); Terrain/Direction là
	// metadata map, rỗng = lấy DEFAULT của cột.
	DeckID    *int64
	Terrain   string
	Direction string
}

// StagePatch là input UpdateStage. Terrain/Direction nil = không đụng (PATCH
// phân biệt "không gửi" với "gửi rỗng" — rỗng sẽ ra DEFAULT).
type StagePatch struct {
	Title         *string
	Goal          *string
	Position      *int
	DurationWeeks *int
	DeckID        *int64
	Terrain       *string
	Direction     *string
}

// CreateStage thêm stage vào path. 404 path không có, 409 slug stage trùng.
func (s *Service) CreateStage(ctx context.Context, pathSlug string, in StageInput) (Stage, error) {
	slug, err := ValidateSlug(in.Slug)
	if err != nil {
		return Stage{}, err
	}
	title, err := ValidateTitle(in.Title)
	if err != nil {
		return Stage{}, err
	}
	goal, err := ValidateText(in.Goal, 2000)
	if err != nil {
		return Stage{}, err
	}
	if in.DurationWeeks != nil {
		if err := ValidateDurationWeeks(*in.DurationWeeks); err != nil {
			return Stage{}, err
		}
	}
	status, err := parseOptionalStatus(in.Status)
	if err != nil {
		return Stage{}, err
	}
	terrain, err := parseMapField(in.Terrain, domain.ParseTerrain)
	if err != nil {
		return Stage{}, err
	}
	direction, err := parseMapField(in.Direction, domain.ParseDirection)
	if err != nil {
		return Stage{}, err
	}

	var created Stage
	err = s.inTx(ctx, func(tx Tx) error {
		p, err := s.repo.PathBySlug(ctx, pathSlug)
		if err != nil {
			return wrapNotFound(err, "không tìm thấy learning path")
		}
		// Deck kiểm SAU khi đã biết `path.language` — không thể validate ngoài
		// tx vì cần đọc path trước.
		deck, err := s.resolveDeck(ctx, p.Language, in.DeckID)
		if err != nil {
			return err
		}
		var deckID *int64
		if deck != nil {
			deckID = in.DeckID
		}
		pos, err := s.nextStagePosition(ctx, p.ID, in.Position)
		if err != nil {
			return err
		}
		dur := 0
		if in.DurationWeeks != nil {
			dur = *in.DurationWeeks
		}
		now := s.timestamp()
		created = Stage{
			PathID: p.ID, Slug: slug, Title: title, Goal: goal, Position: pos,
			DurationWeeks: dur, Status: status,
			Terrain: terrain, Direction: direction, DeckID: deckID,
			CreatedAt: now, GUID: NewGUID(), UpdatedAt: now,
		}
		if err := s.repo.CreateStage(ctx, tx, &created); err != nil {
			if isUniqueViolation(err) {
				return newError(StatusConflict, "slug stage đã tồn tại trong learning path này")
			}
			return fmt.Errorf("tạo stage: %w", err)
		}
		return nil
	})
	return created, err
}

// UpdateStage sửa stage. Validate terrain/direction/deck trước khi mở tx để
// lỗi 400 không phải rollback trần.
func (s *Service) UpdateStage(ctx context.Context, id int64, patch StagePatch) (Stage, error) {
	if id <= 0 {
		return Stage{}, newError(StatusBadRequest, "id stage không hợp lệ")
	}
	if patch.Title == nil && patch.Goal == nil && patch.Position == nil &&
		patch.DurationWeeks == nil && patch.DeckID == nil &&
		patch.Terrain == nil && patch.Direction == nil {
		return Stage{}, newError(StatusBadRequest, "không có gì để cập nhật")
	}
	if patch.Position != nil {
		if err := ValidatePosition(*patch.Position); err != nil {
			return Stage{}, err
		}
	}
	if patch.DurationWeeks != nil {
		if err := ValidateDurationWeeks(*patch.DurationWeeks); err != nil {
			return Stage{}, err
		}
	}
	var terrain, direction *string
	if patch.Terrain != nil {
		v, err := parseMapField(*patch.Terrain, domain.ParseTerrain)
		if err != nil {
			return Stage{}, err
		}
		terrain = &v
	}
	if patch.Direction != nil {
		v, err := parseMapField(*patch.Direction, domain.ParseDirection)
		if err != nil {
			return Stage{}, err
		}
		direction = &v
	}
	var updated Stage
	err := s.inTx(ctx, func(tx Tx) error {
		st, err := s.repo.StageByID(ctx, id)
		if err != nil {
			return wrapNotFound(err, "không tìm thấy stage")
		}
		if patch.DeckID != nil {
			// Cần `path.language` để chặn deck lệch ngôn ngữ (xem resolveDeck),
			// mà `path_id` chỉ biết sau khi đọc stage.
			p, err := s.repo.PathByID(ctx, st.PathID)
			if err != nil {
				return wrapNotFound(err, "không tìm thấy learning path của stage")
			}
			deck, err := s.resolveDeck(ctx, p.Language, patch.DeckID)
			if err != nil {
				return err
			}
			if deck == nil {
				st.DeckID = nil
			} else {
				st.DeckID = patch.DeckID
			}
		}
		if patch.Title != nil {
			if st.Title, err = ValidateTitle(*patch.Title); err != nil {
				return err
			}
		}
		if patch.Goal != nil {
			if st.Goal, err = ValidateText(*patch.Goal, 2000); err != nil {
				return err
			}
		}
		if patch.Position != nil {
			st.Position = *patch.Position
		}
		if patch.DurationWeeks != nil {
			st.DurationWeeks = *patch.DurationWeeks
		}
		if terrain != nil {
			st.Terrain = *terrain
		}
		if direction != nil {
			st.Direction = *direction
		}
		if err := s.repo.UpdateStage(ctx, tx, &st); err != nil {
			return fmt.Errorf("cập nhật stage: %w", err)
		}
		updated = st
		return nil
	})
	return updated, err
}

// DeleteStage xoá mềm stage + nhánh con (milestone/topic/resource). Cascade do
// repository thực hiện trong cùng transaction — lỗi giữa chừng sẽ rollback cả
// cây, không để lại nửa cây đã xoá.
func (s *Service) DeleteStage(ctx context.Context, id int64) error {
	if id <= 0 {
		return newError(StatusBadRequest, "id stage không hợp lệ")
	}
	return s.inTx(ctx, func(tx Tx) error {
		if _, err := s.repo.StageByID(ctx, id); err != nil {
			return wrapNotFound(err, "không tìm thấy stage")
		}
		if err := s.repo.SoftDeleteStage(ctx, tx, id); err != nil {
			return fmt.Errorf("xoá stage: %w", err)
		}
		return nil
	})
}

// SetStageStatus đổi trạng thái stage. Dùng domain.SetStatus để set/clear
// completed_at đúng luật A1, rồi ghi 1 UPDATE (không set updated_at tường minh
// → trigger chạm, peer nhận thay đổi).
func (s *Service) SetStageStatus(ctx context.Context, id int64, status *string, note *string) (Stage, error) {
	if id <= 0 {
		return Stage{}, newError(StatusBadRequest, "id stage không hợp lệ")
	}
	st, noteText, err := ValidateStatus(status, note)
	if err != nil {
		return Stage{}, err
	}
	var updated Stage
	err = s.inTx(ctx, func(tx Tx) error {
		cur, err := s.repo.StageByID(ctx, id)
		if err != nil {
			return wrapNotFound(err, "không tìm thấy stage")
		}
		now := s.nowFn()
		change := domain.SetStatus(domain.StatusUpdate{
			Current: domain.Status(cur.Status), Note: noteText, Now: now,
		}, domain.Status(st))
		// Ghi vào chính struct vừa đọc, rồi đẩy xuống. Không đọc lại row: lần
		// đọc ngoài transaction sẽ thấy dữ liệu CŨ (P0 của cổng Oracle M2).
		cur.Status = st
		cur.StatusNote = noteText
		cur.CompletedAt = resolveCompletedAt(st, cur.CompletedAt, change.CompletedAt, now)
		if err := s.repo.UpdateStageStatus(ctx, tx, &cur); err != nil {
			return fmt.Errorf("lưu trạng thái stage: %w", err)
		}
		updated = cur
		return nil
	})
	return updated, err
}

// resolveCompletedAt quyết định giá trị completed_at ghi xuống.
//   - Vào done lần đầu → mốc mới từ domain.
//   - Vẫn done (SetStatus trả rỗng để giữ mốc cũ) → giữ mốc đang có. Nếu row
//     chưa từng có mốc (migrate dữ liệu v1 sang Postgres: cột completed_at mới,
//     row cũ chỉ có status) thì gán mốc hiện tại — nếu không `progress?since=`
//     sẽ bỏ sót node này vĩnh viễn vì user không có lý do bấm lại.
//   - Rời done → nil (SQL NULL, không phải "").
func resolveCompletedAt(next Status, current *string, computed string, now time.Time) *string {
	if computed != "" {
		v := computed
		return &v
	}
	if next != StatusDone {
		return nil
	}
	if current != nil {
		return current
	}
	v := now.UTC().Format(time.RFC3339)
	return &v
}

// ── Topic ───────────────────────────────────────────────────────────────────

// TopicInput là input CreateTopic.
type TopicInput struct {
	Title      string
	Why        string
	Activities []string
	Position   *int
	IsOptional *int
	MapX       *float64
	MapY       *float64
}

// TopicPatch là input UpdateTopic.
//
// `MapX`/`MapY` là `*float64` nên KHÔNG phân biệt được "không gửi field" với
// "gửi null": patch có field = gửi giá trị đó. Xoá toạ độ (đưa node về
// auto-layout, cần cho M6) dùng `ClearMap` — cùng cách `ResourcePatch.ClearURL`
// đã làm cho link.
type TopicPatch struct {
	Title      *string
	Why        *string
	Activities *[]string
	Position   *int
	IsOptional *int
	MapX       *float64
	MapY       *float64
	// ClearMap xoá map_x + map_y (NULL thật trong DB, không phải 0 — node ở
	// (0,0) là vị trí hợp lệ). Thắng mọi giá trị MapX/MapY trong cùng patch.
	ClearMap bool
}

// CreateTopic thêm topic vào stage.
func (s *Service) CreateTopic(ctx context.Context, stageID int64, in TopicInput) (Topic, error) {
	if stageID <= 0 {
		return Topic{}, newError(StatusBadRequest, "id stage không hợp lệ")
	}
	title, err := ValidateTitle(in.Title)
	if err != nil {
		return Topic{}, err
	}
	why, err := ValidateText(in.Why, 2000)
	if err != nil {
		return Topic{}, err
	}
	optional, err := ValidateOptionalFlag(in.IsOptional)
	if err != nil {
		return Topic{}, err
	}
	mapX, err := s.validateMapCoord(in.MapX, "map_x", s.viewBox.Width)
	if err != nil {
		return Topic{}, err
	}
	mapY, err := s.validateMapCoord(in.MapY, "map_y", s.viewBox.Height)
	if err != nil {
		return Topic{}, err
	}

	var created Topic
	err = s.inTx(ctx, func(tx Tx) error {
		if _, err := s.repo.StageByID(ctx, stageID); err != nil {
			return wrapNotFound(err, "không tìm thấy stage")
		}
		pos, err := s.nextTopicPosition(ctx, stageID, in.Position)
		if err != nil {
			return err
		}
		now := s.timestamp()
		created = Topic{
			StageID: stageID, Title: title, Why: why,
			Activities: EncodeActivities(in.Activities), Position: pos,
			Status: StatusNotStarted, IsOptional: optional,
			MapX: mapX, MapY: mapY, CreatedAt: now, GUID: NewGUID(), UpdatedAt: now,
		}
		if err := s.repo.CreateTopic(ctx, tx, &created); err != nil {
			return fmt.Errorf("tạo topic: %w", err)
		}
		return nil
	})
	return created, err
}

// UpdateTopic sửa topic.
func (s *Service) UpdateTopic(ctx context.Context, id int64, patch TopicPatch) (Topic, error) {
	if id <= 0 {
		return Topic{}, newError(StatusBadRequest, "id topic không hợp lệ")
	}
	if patch.Title == nil && patch.Why == nil && patch.Activities == nil &&
		patch.Position == nil && patch.IsOptional == nil &&
		patch.MapX == nil && patch.MapY == nil && !patch.ClearMap {
		return Topic{}, newError(StatusBadRequest, "không có gì để cập nhật")
	}
	var activities *string
	if patch.Activities != nil {
		v := EncodeActivities(*patch.Activities)
		activities = &v
	}
	optional, err := ValidateOptionalFlag(patch.IsOptional)
	if err != nil {
		return Topic{}, err
	}
	var mapX, mapY *float64
	if patch.MapX != nil {
		v, err := s.validateMapCoord(patch.MapX, "map_x", s.viewBox.Width)
		if err != nil {
			return Topic{}, err
		}
		mapX = v
	}
	if patch.MapY != nil {
		v, err := s.validateMapCoord(patch.MapY, "map_y", s.viewBox.Height)
		if err != nil {
			return Topic{}, err
		}
		mapY = v
	}
	if patch.Position != nil {
		if err := ValidatePosition(*patch.Position); err != nil {
			return Topic{}, err
		}
	}

	var updated Topic
	err = s.inTx(ctx, func(tx Tx) error {
		t, err := s.repo.TopicByID(ctx, id)
		if err != nil {
			return wrapNotFound(err, "không tìm thấy topic")
		}
		if patch.Title != nil {
			if t.Title, err = ValidateTitle(*patch.Title); err != nil {
				return err
			}
		}
		if patch.Why != nil {
			if t.Why, err = ValidateText(*patch.Why, 2000); err != nil {
				return err
			}
		}
		if activities != nil {
			t.Activities = *activities
		}
		if patch.Position != nil {
			t.Position = *patch.Position
		}
		if patch.IsOptional != nil {
			t.IsOptional = optional
		}
		if patch.MapX != nil {
			t.MapX = mapX
		}
		if patch.MapY != nil {
			t.MapY = mapY
		}
		if patch.ClearMap {
			// NULL thật, KHÔNG phải 0: node ở (0,0) là vị trí hợp lệ trong
			// viewBox, và `MapPinned` của M6 dựa trên NULL chứ không phải 0.
			t.MapX, t.MapY = nil, nil
		}
		if err := s.repo.UpdateTopic(ctx, tx, &t); err != nil {
			return fmt.Errorf("cập nhật topic: %w", err)
		}
		updated = t
		return nil
	})
	return updated, err
}

// DeleteTopic xoá mềm topic + resource của nó (cascade trong repository).
func (s *Service) DeleteTopic(ctx context.Context, id int64) error {
	if id <= 0 {
		return newError(StatusBadRequest, "id topic không hợp lệ")
	}
	return s.inTx(ctx, func(tx Tx) error {
		if _, err := s.repo.TopicByID(ctx, id); err != nil {
			return wrapNotFound(err, "không tìm thấy topic")
		}
		if err := s.repo.SoftDeleteTopic(ctx, tx, id); err != nil {
			return fmt.Errorf("xoá topic: %w", err)
		}
		return nil
	})
}

// SetTopicStatus đổi trạng thái topic (luật completed_at y hệt stage).
func (s *Service) SetTopicStatus(ctx context.Context, id int64, status *string, note *string) (Topic, error) {
	if id <= 0 {
		return Topic{}, newError(StatusBadRequest, "id topic không hợp lệ")
	}
	st, noteText, err := ValidateStatus(status, note)
	if err != nil {
		return Topic{}, err
	}
	var updated Topic
	err = s.inTx(ctx, func(tx Tx) error {
		cur, err := s.repo.TopicByID(ctx, id)
		if err != nil {
			return wrapNotFound(err, "không tìm thấy topic")
		}
		now := s.nowFn()
		change := domain.SetStatus(domain.StatusUpdate{
			Current: domain.Status(cur.Status), Note: noteText, Now: now,
		}, domain.Status(st))
		// Ghi vào struct vừa đọc, không đọc lại — xem SetStageStatus.
		cur.Status = st
		cur.StatusNote = noteText
		cur.CompletedAt = resolveCompletedAt(st, cur.CompletedAt, change.CompletedAt, now)
		if err := s.repo.UpdateTopicStatus(ctx, tx, &cur); err != nil {
			return fmt.Errorf("lưu trạng thái topic: %w", err)
		}
		updated = cur
		return nil
	})
	return updated, err
}

// ── Resource ────────────────────────────────────────────────────────────────

// ResourceInput là input CreateResource.
type ResourceInput struct {
	Title    string
	URL      *string
	Kind     string
	Note     string
	Position *int
}

// ResourcePatch là input UpdateResource. URL rỗng KHÔNG phải "xoá link" —
// dùng ClearURL, vì `*string` không phân biệt được absent với null.
type ResourcePatch struct {
	Title    *string
	URL      *string
	Kind     *string
	Note     *string
	Position *int
	ClearURL bool
}

// CreateResource thêm tài liệu vào topic.
func (s *Service) CreateResource(ctx context.Context, topicID int64, in ResourceInput) (Resource, error) {
	if topicID <= 0 {
		return Resource{}, newError(StatusBadRequest, "id topic không hợp lệ")
	}
	title, err := ValidateTitle(in.Title)
	if err != nil {
		return Resource{}, err
	}
	u, err := ValidateURL(in.URL)
	if err != nil {
		return Resource{}, err
	}
	kind, err := ValidateKind(in.Kind)
	if err != nil {
		return Resource{}, err
	}
	note, err := ValidateText(in.Note, 2000)
	if err != nil {
		return Resource{}, err
	}
	var created Resource
	err = s.inTx(ctx, func(tx Tx) error {
		if _, err := s.repo.TopicByID(ctx, topicID); err != nil {
			return wrapNotFound(err, "không tìm thấy topic")
		}
		pos, err := s.nextResourcePosition(ctx, topicID, in.Position)
		if err != nil {
			return err
		}
		now := s.timestamp()
		created = Resource{
			TopicID: topicID, Title: title, URL: u, Kind: kind, Note: note,
			Position: pos, CreatedAt: now, GUID: NewGUID(), UpdatedAt: now,
		}
		if err := s.repo.CreateResource(ctx, tx, &created); err != nil {
			return fmt.Errorf("tạo resource: %w", err)
		}
		return nil
	})
	return created, err
}

// UpdateResource sửa tài liệu.
func (s *Service) UpdateResource(ctx context.Context, id int64, patch ResourcePatch) (Resource, error) {
	if id <= 0 {
		return Resource{}, newError(StatusBadRequest, "id resource không hợp lệ")
	}
	if patch.Title == nil && patch.URL == nil && patch.Kind == nil &&
		patch.Note == nil && patch.Position == nil && !patch.ClearURL {
		return Resource{}, newError(StatusBadRequest, "không có gì để cập nhật")
	}
	touchesURL := patch.ClearURL || patch.URL != nil
	var u *string
	if patch.URL != nil {
		v, err := ValidateURL(patch.URL)
		if err != nil {
			return Resource{}, err
		}
		u = v
	}
	var kind *string
	if patch.Kind != nil {
		v, err := ValidateKind(*patch.Kind)
		if err != nil {
			return Resource{}, err
		}
		kind = &v
	}
	if patch.Position != nil {
		if err := ValidatePosition(*patch.Position); err != nil {
			return Resource{}, err
		}
	}

	var updated Resource
	err := s.inTx(ctx, func(tx Tx) error {
		r, err := s.repo.ResourceByID(ctx, id)
		if err != nil {
			return wrapNotFound(err, "không tìm thấy resource")
		}
		if patch.Title != nil {
			if r.Title, err = ValidateTitle(*patch.Title); err != nil {
				return err
			}
		}
		if touchesURL {
			r.URL = u
		}
		if kind != nil {
			r.Kind = *kind
		}
		if patch.Note != nil {
			if r.Note, err = ValidateText(*patch.Note, 2000); err != nil {
				return err
			}
		}
		if patch.Position != nil {
			r.Position = *patch.Position
		}
		if err := s.repo.UpdateResource(ctx, tx, &r); err != nil {
			return fmt.Errorf("cập nhật resource: %w", err)
		}
		updated = r
		return nil
	})
	return updated, err
}

// DeleteResource xoá mềm tài liệu.
func (s *Service) DeleteResource(ctx context.Context, id int64) error {
	if id <= 0 {
		return newError(StatusBadRequest, "id resource không hợp lệ")
	}
	return s.inTx(ctx, func(tx Tx) error {
		if _, err := s.repo.ResourceByID(ctx, id); err != nil {
			return wrapNotFound(err, "không tìm thấy resource")
		}
		if err := s.repo.SoftDeleteResource(ctx, tx, id); err != nil {
			return fmt.Errorf("xoá resource: %w", err)
		}
		return nil
	})
}

// ── Milestone ───────────────────────────────────────────────────────────────

// MilestoneInput là input CreateMilestone.
type MilestoneInput struct {
	Text     string
	Position *int
}

// MilestonePatch là input UpdateMilestone.
type MilestonePatch struct {
	Text     *string
	Position *int
}

// CreateMilestone thêm mốc nhỏ vào stage.
func (s *Service) CreateMilestone(ctx context.Context, stageID int64, in MilestoneInput) (Milestone, error) {
	if stageID <= 0 {
		return Milestone{}, newError(StatusBadRequest, "id stage không hợp lệ")
	}
	text, err := ValidateTitle(in.Text)
	if err != nil {
		return Milestone{}, newError(StatusBadRequest, "nội dung milestone không được rỗng")
	}
	var created Milestone
	err = s.inTx(ctx, func(tx Tx) error {
		if _, err := s.repo.StageByID(ctx, stageID); err != nil {
			return wrapNotFound(err, "không tìm thấy stage")
		}
		pos, err := s.nextMilestonePosition(ctx, stageID, in.Position)
		if err != nil {
			return err
		}
		now := s.timestamp()
		created = Milestone{
			StageID: stageID, Text: text, Position: pos,
			CreatedAt: now, GUID: NewGUID(), UpdatedAt: now,
		}
		if err := s.repo.CreateMilestone(ctx, tx, &created); err != nil {
			return fmt.Errorf("tạo milestone: %w", err)
		}
		return nil
	})
	return created, err
}

// UpdateMilestone sửa mốc nhỏ.
func (s *Service) UpdateMilestone(ctx context.Context, id int64, patch MilestonePatch) (Milestone, error) {
	if id <= 0 {
		return Milestone{}, newError(StatusBadRequest, "id milestone không hợp lệ")
	}
	if patch.Text == nil && patch.Position == nil {
		return Milestone{}, newError(StatusBadRequest, "không có gì để cập nhật")
	}
	var text *string
	if patch.Text != nil {
		v, err := ValidateTitle(*patch.Text)
		if err != nil {
			return Milestone{}, newError(StatusBadRequest, "nội dung milestone không được rỗng")
		}
		text = &v
	}
	if patch.Position != nil {
		if err := ValidatePosition(*patch.Position); err != nil {
			return Milestone{}, err
		}
	}
	var updated Milestone
	err := s.inTx(ctx, func(tx Tx) error {
		m, err := s.repo.MilestoneByID(ctx, id)
		if err != nil {
			return wrapNotFound(err, "không tìm thấy milestone")
		}
		if text != nil {
			m.Text = *text
		}
		if patch.Position != nil {
			m.Position = *patch.Position
		}
		if err := s.repo.UpdateMilestone(ctx, tx, &m); err != nil {
			return fmt.Errorf("cập nhật milestone: %w", err)
		}
		updated = m
		return nil
	})
	return updated, err
}

// DeleteMilestone xoá mềm mốc nhỏ.
func (s *Service) DeleteMilestone(ctx context.Context, id int64) error {
	if id <= 0 {
		return newError(StatusBadRequest, "id milestone không hợp lệ")
	}
	return s.inTx(ctx, func(tx Tx) error {
		if _, err := s.repo.MilestoneByID(ctx, id); err != nil {
			return wrapNotFound(err, "không tìm thấy milestone")
		}
		if err := s.repo.SoftDeleteMilestone(ctx, tx, id); err != nil {
			return fmt.Errorf("xoá milestone: %w", err)
		}
		return nil
	})
}

// ── Cây + bản đồ ────────────────────────────────────────────────────────────

// TopicView là topic trong cây, kèm trạng thái màn và toạ độ node. 3 trường này
// là phần M6 cần để vẽ bản đồ: 1 request `GetPath` là đủ, không phải gọi thêm
// endpoint layout.
type TopicView struct {
	Topic
	// Level là trạng thái node suy ra (done/current/locked) — KHÔNG lưu DB.
	Level domain.LevelState
	// Point là toạ độ node trong viewBox 0 0 1000 2000.
	Point domain.MapPoint
	// MapPinned = true khi user đã đặt tay toạ độ (MapX/MapY khác nil).
	MapPinned bool
	Resources []Resource
}

// StageView là stage trong cây, kèm topic đã có layout.
type StageView struct {
	Stage
	Topics     []TopicView
	Milestones []Milestone
	// Deck là deck đã gắn (A1) cho nút "vào /review" của M6 — có `Name` để
	// client hiện "Ôn HSK1" thay vì id. nil = stage chưa gắn deck.
	Deck *DeckRef `json:"deck"`
}

// PathView là cây roadmap đầy đủ — 1 nguồn sự thật cho web lẫn app sau này
// (ROADMAP-MAP-IDEA §7.2).
type PathView struct {
	Path
	Stages   []StageView
	Progress Progress
}

// GetPath trả cây đầy đủ của 1 path: stage → (milestone, topic → resource), kèm
// layout + LevelState cho từng topic. 404 nếu path không tồn tại / đã xoá mềm.
//
// Đường này dùng CÙNG các hàm batch với `StageTreeByPathIDs` (xem `decorateTopics`),
// nên số statement của `GetPath` và của 1 query GraphQL cây là bằng nhau.
func (s *Service) GetPath(ctx context.Context, slug string) (PathView, error) {
	p, err := s.PathBySlug(ctx, slug)
	if err != nil {
		return PathView{}, err
	}
	view := PathView{Path: p, Stages: []StageView{}}

	stages, topics, milestones, err := s.treeParts(ctx, []int64{p.ID})
	if err != nil {
		return PathView{}, err
	}
	milestonesByStage := groupMilestones(milestones)
	viewsByStage, allTopics, byStage := s.decorateTopics(stages, topics)
	resourceByTopic, err := s.resourcesFor(ctx, stages, viewsByStage)
	if err != nil {
		return PathView{}, err
	}
	for _, st := range stages {
		sv := StageView{
			Stage:      st,
			Topics:     viewsByStage[st.ID],
			Milestones: milestonesByStage[st.ID],
		}
		if sv.Milestones == nil {
			sv.Milestones = []Milestone{}
		}
		for i := range sv.Topics {
			sv.Topics[i].Resources = resourceByTopic[sv.Topics[i].ID]
		}
		if err := s.attachDeck(ctx, &sv); err != nil {
			return PathView{}, err
		}
		view.Stages = append(view.Stages, sv)
	}
	view.Progress = s.progressFromTopics(stages, allTopics, byStage, nil)
	return view, nil
}

// resourcesFor gom resource cho mọi topic của các stage đã decorate, bằng 1
// lệnh `topic_id IN (...)`.
func (s *Service) resourcesFor(ctx context.Context, stages []Stage, viewsByStage map[int64][]TopicView) (map[int64][]Resource, error) {
	topicIDs := make([]int64, 0, len(viewsByStage))
	for _, st := range stages {
		for _, tv := range viewsByStage[st.ID] {
			topicIDs = append(topicIDs, tv.ID)
		}
	}
	if len(topicIDs) == 0 {
		return map[int64][]Resource{}, nil
	}
	rows, err := s.repo.ListResourcesByTopicIDs(ctx, topicIDs)
	if err != nil {
		return nil, fmt.Errorf("đọc resource của cây roadmap: %w", err)
	}
	out := make(map[int64][]Resource, len(topicIDs))
	for _, id := range topicIDs {
		out[id] = []Resource{}
	}
	for _, r := range rows {
		out[r.TopicID] = append(out[r.TopicID], r)
	}
	return out, nil
}

// StageByID / TopicByID / ResourceByID / MilestoneByID trả 1 node theo id, kèm
// 404 khi không có hoặc đã xoá mềm.
//
// 4 hàm này phục vụ `Query.stage/topic/resource/milestone` của M4 (UI sửa 1
// node không cần tải cả cây). Chúng ở đây thay vì để transport tự dựng cây rồi
// lọc: dựng cả cây để lấy 1 node là O(cây) I/O cho 1 việc O(1).
func (s *Service) StageByID(ctx context.Context, id int64) (Stage, error) {
	if id <= 0 {
		return Stage{}, newError(StatusBadRequest, "id stage không hợp lệ")
	}
	st, err := s.repo.StageByID(ctx, id)
	if err != nil {
		return Stage{}, wrapNotFound(err, "không tìm thấy stage")
	}
	return st, nil
}

// TopicByID trả 1 topic kèm layout + LevelState của CẢ stage — layout phụ thuộc
// chuỗi node trước trong stage nên bắt buộc phải đọc cả stage, không suy ra từ
// topic lẻ được.
func (s *Service) TopicByID(ctx context.Context, id int64) (TopicView, error) {
	if id <= 0 {
		return TopicView{}, newError(StatusBadRequest, "id topic không hợp lệ")
	}
	t, err := s.repo.TopicByID(ctx, id)
	if err != nil {
		return TopicView{}, wrapNotFound(err, "không tìm thấy topic")
	}
	views, err := s.StageTreeByIDs(ctx, []int64{t.StageID})
	if err != nil {
		return TopicView{}, err
	}
	for _, sv := range views[t.StageID] {
		for _, tv := range sv.Topics {
			if tv.ID == t.ID {
				return tv, nil
			}
		}
	}
	return TopicView{}, newError(StatusInternalServerError, "topic vừa đọc không nằm trong cây của stage")
}

// ResourceByID trả 1 tài liệu.
func (s *Service) ResourceByID(ctx context.Context, id int64) (Resource, error) {
	if id <= 0 {
		return Resource{}, newError(StatusBadRequest, "id resource không hợp lệ")
	}
	r, err := s.repo.ResourceByID(ctx, id)
	if err != nil {
		return Resource{}, wrapNotFound(err, "không tìm thấy resource")
	}
	return r, nil
}

// MilestoneByID trả 1 mốc nhỏ.
func (s *Service) MilestoneByID(ctx context.Context, id int64) (Milestone, error) {
	if id <= 0 {
		return Milestone{}, newError(StatusBadRequest, "id milestone không hợp lệ")
	}
	m, err := s.repo.MilestoneByID(ctx, id)
	if err != nil {
		return Milestone{}, wrapNotFound(err, "không tìm thấy milestone")
	}
	return m, nil
}

// PathBySlug trả 1 path (404 nếu không có / đã xoá mềm) mà KHÔNG tải cây.
//
// Tách riêng khỏi `GetPath` vì dataloader GraphQL cần đúng phần này: `Query.path`
// trả node path, rồi `Path.stages` / `Stage.topics` / `Topic.resources` lấy từ
// loader. Nếu `Query.path` gọi `GetPath` thì loader thành vô nghĩa (dữ liệu đã
// nằm sẵn trong struct) và số statement quay lại tuyến tính theo số topic.
func (s *Service) PathBySlug(ctx context.Context, slug string) (Path, error) {
	slug, err := ValidateSlug(slug)
	if err != nil {
		return Path{}, err
	}
	p, err := s.repo.PathBySlug(ctx, slug)
	if err != nil {
		return Path{}, wrapNotFound(err, "không tìm thấy learning path")
	}
	return p, nil
}

// StageTreeByPathIDs trả stage + topic (ĐÃ có layout + LevelState) + milestone cho
// NHIỀU path trong 1 lần gọi, gom theo pathID cho dataloader `Path.stages`.
//
// Số statement là HẰNG theo số path/topic: 1 lệnh `stages IN`, 1 lệnh
// `topics IN`, 1 lệnh `milestones IN`. Resource KHÔNG nằm ở đây — nó là loader
// riêng (`ResourcesByTopicIDs`) để query chỉ xin `stages { topics { level } }`
// không phải trả tiền cho 263 resource.
//
// pathID không có stage (hoặc path đã xoá mềm) vẫn có mặt trong map với slice
// rỗng — dataloaden cần key có mặt để phân biệt "không có con" với "chưa đọc".
func (s *Service) StageTreeByPathIDs(ctx context.Context, pathIDs []int64) (map[int64][]StageView, error) {
	byPath := make(map[int64][]StageView, len(pathIDs))
	for _, id := range pathIDs {
		byPath[id] = []StageView{}
	}
	byStage, err := s.StageTreeByIDs(ctx, pathIDs)
	if err != nil {
		return nil, err
	}
	for _, views := range byStage {
		for _, sv := range views {
			byPath[sv.PathID] = append(byPath[sv.PathID], sv)
		}
	}
	// PHẢI sắp lại: `byStage` là map ⇒ `range` ở trên đi thứ tự NGẪU NHIÊN
	// (Go randomize thứ tự duyệt map), nên nếu bỏ bước này thì `path.stages`
	// trả về cây bị xáo mỗi request. Câuy roadmap vẽ theo `position`, đổi thứ
	// tự ở tầng 2 làm node nhảy lung tung trên bản đồ và `LevelState` của
	// topic đầu mỗi stage sai — mà KHÔNG có lỗi nào được trả về cho client.
	//
	// Lỗi này M4 không bắt được vì seed lúc đó chỉ có 1 stage: với 1 phần tử
	// thì thứ tự không thể sai. `Test_stage_order_in_tree_is_stable_across_calls`
	// và `Test_roadmap_tree_sql_statement_count_is_constant_as_stages_grow`
	// (nhiều stage) là 2 test bắt được.
	for id := range byPath {
		views := byPath[id]
		sort.SliceStable(views, func(a, b int) bool {
			if views[a].Position != views[b].Position {
				return views[a].Position < views[b].Position
			}
			return views[a].ID < views[b].ID
		})
	}
	return byPath, nil
}

// StageTreeByIDs là phần dùng chung của `StageTreeByPathIDs` và `TopicByID`:
// gom stage theo path, mỗi stage kèm topic đã decorate + milestone.
func (s *Service) StageTreeByIDs(ctx context.Context, pathIDs []int64) (map[int64][]StageView, error) {
	stages, topics, milestones, err := s.treeParts(ctx, pathIDs)
	if err != nil {
		return nil, err
	}
	milestonesByStage := groupMilestones(milestones)
	viewsByStage, _, _ := s.decorateTopics(stages, topics)
	byStage := make(map[int64][]StageView, len(stages))
	for _, st := range stages {
		views := viewsByStage[st.ID]
		if views == nil {
			views = []TopicView{}
		}
		ms := milestonesByStage[st.ID]
		if ms == nil {
			ms = []Milestone{}
		}
		byStage[st.ID] = []StageView{{Stage: st, Topics: views, Milestones: ms}}
	}
	return byStage, nil
}

// groupMilestones gom milestone theo stageID. Lệnh batch trả 1 slice phẳng vì
// SQL không tự gom theo nhóm; các node đã được sắp `(stage_id, position, id)`
// nên giữ nguyên thứ tự khi ghép lại.
func groupMilestones(rows []Milestone) map[int64][]Milestone {
	out := make(map[int64][]Milestone)
	for _, m := range rows {
		out[m.StageID] = append(out[m.StageID], m)
	}
	return out
}

// ResourcesByTopicIDs gom resource theo topicID cho dataloader `Topic.resources`.
// 1 statement cho toàn bộ topic trong request.
func (s *Service) ResourcesByTopicIDs(ctx context.Context, topicIDs []int64) (map[int64][]Resource, error) {
	rows, err := s.repo.ListResourcesByTopicIDs(ctx, topicIDs)
	if err != nil {
		return nil, fmt.Errorf("đọc resource của cây roadmap: %w", err)
	}
	out := make(map[int64][]Resource, len(topicIDs))
	for _, id := range topicIDs {
		out[id] = []Resource{}
	}
	for _, r := range rows {
		out[r.TopicID] = append(out[r.TopicID], r)
	}
	return out, nil
}

// treeParts là phần ĐỌC của cây: 3 lệnh batch, không phụ thuộc số node.
func (s *Service) treeParts(ctx context.Context, pathIDs []int64) ([]Stage, []Topic, []Milestone, error) {
	if len(pathIDs) == 0 {
		return nil, nil, nil, nil
	}
	stages, err := s.repo.ListStagesByPathIDs(ctx, pathIDs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("đọc stage của learning path: %w", err)
	}
	stageIDs := make([]int64, 0, len(stages))
	for _, st := range stages {
		stageIDs = append(stageIDs, st.ID)
	}
	if len(stageIDs) == 0 {
		return stages, nil, nil, nil
	}
	topics, err := s.repo.ListTopicsByStageIDs(ctx, stageIDs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("đọc topic của learning path: %w", err)
	}
	milestones, err := s.repo.ListMilestonesByStageIDs(ctx, stageIDs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("đọc milestone của learning path: %w", err)
	}
	return stages, topics, milestones, nil
}

// decorateTopics gắn layout (toạ độ) + LevelState lên từng topic, theo đúng
// thứ tự (position, id) của `domain.SortedTopics`.
//
// `ComputeLayout` + `LevelStates` đều SẮP lại input theo (position, id), còn
// `rows` đến từ repository vốn đã sắp nhưng KHÔNG được phép tin vào điều đó —
// ghép bằng CHỈ SỐ là dựa vào việc hai thứ trùng nhau, và lệch đi 1 dòng thì mỗi
// node mang toạ độ + trạng thái của node khác mà không lỗi nào báo. Vì vậy gom
// kết quả theo `ID` rồi duyệt cây theo thứ tự ĐÃ SẮP.
//
// Trả 3 giá trị cho 2 call site: view đã decorate theo stage, tập topic domain
// gộp, và map topic→stage để `progressFromTopics` tính tiến độ. `Resources`
// trong `TopicView` để rỗng — xem `GetPath` (gộp sẵn) và `StageTreeByPathIDs`
// (loader riêng).
func (s *Service) decorateTopics(stages []Stage, topics []Topic) (
	viewsByStage map[int64][]TopicView,
	allTopics []domain.Topic,
	byStage map[int64][]domain.Topic,
) {
	rowsByStage := make(map[int64][]Topic, len(stages))
	for _, t := range topics {
		rowsByStage[t.StageID] = append(rowsByStage[t.StageID], t)
	}
	viewsByStage = make(map[int64][]TopicView, len(stages))
	byStage = make(map[int64][]domain.Topic, len(stages))
	for _, st := range stages {
		rows := rowsByStage[st.ID]
		dts := make([]domain.Topic, 0, len(rows))
		byID := make(map[int64]Topic, len(rows))
		for _, r := range rows {
			dts = append(dts, TopicToDomain(r))
			byID[r.ID] = r
		}
		sorted := domain.SortedTopics(dts)
		points := domain.ComputeLayout(sorted, domain.Terrain(st.Terrain), domain.Direction(st.Direction))
		states := domain.LevelStates(sorted)
		views := make([]TopicView, 0, len(sorted))
		for i, d := range sorted {
			views = append(views, TopicView{
				Topic:     byID[d.ID],
				Level:     states[i],
				Point:     points[i],
				MapPinned: d.MapX != nil || d.MapY != nil,
				Resources: []Resource{},
			})
			allTopics = append(allTopics, d)
			byStage[st.ID] = append(byStage[st.ID], d)
		}
		viewsByStage[st.ID] = views
	}
	return viewsByStage, allTopics, byStage
}

// attachDeck gắn thông tin deck cho stage. Port `srs` chưa bind thì trả 500 tường
// minh thay vì im lặng bỏ trống — lỗi này lộ ra sớm hơn là M6 hiện nút trỏ tới
// deck rỗng.
func (s *Service) attachDeck(ctx context.Context, sv *StageView) error {
	if sv.DeckID == nil {
		return nil
	}
	if s.decks == nil {
		return newError(StatusInternalServerError, "chưa sẵn sàng đọc deck của stage")
	}
	// Deck đã được xác nhận tồn tại lúc gắn, nhưng có thể đã bị xoá mềm sau đó
	// (ON DELETE SET NULL chỉ xử lý DELETE cứng). Đọc lại để client không thấy
	// nút trỏ tới deck không còn tồn tại.
	info, err := s.decks.Find(ctx, *sv.DeckID)
	if err != nil {
		return fmt.Errorf("đọc deck của stage %d: %w", sv.ID, err)
	}
	if info.Exists {
		sv.Deck = &DeckRef{ID: *sv.DeckID, Name: info.Name, Lang: info.Lang}
	}
	return nil
}

// ── Bookmark ────────────────────────────────────────────────────────────────

// BookmarkInput là input CreateBookmark.
//
// `Status` rỗng = `to_read` (DEFAULT của cột). Không có `position`: kho link là
// danh sách người dùng tự thêm, thứ tự là thứ tự thêm.
type BookmarkInput struct {
	Title  string
	URL    *string
	Note   string
	Tags   []string
	Status string
}

// BookmarkPatch là input UpdateBookmark.
//
// `URL` rỗng KHÔNG phải "xoá link" — dùng `ClearURL`, vì `*string` không phân
// biệt được absent với null (cùng cách `ResourcePatch.ClearURL` đã làm).
type BookmarkPatch struct {
	Title    *string
	URL      *string
	Note     *string
	Tags     *[]string
	ClearURL bool
}

// ListBookmarks trả kho link, tuỳ chọn lọc theo trạng thái và theo 1 tag.
// Filter rỗng = xem tất cả.
//
// `limit <= 0` = không giới hạn: kho link là dữ liệu cá nhân 1 người, và giới
// hạn ở đây chỉ tạo ra 1 câu hỏi client phải tự trả lời (mất bao nhiêu dòng?)
// mà không đổi hành vi. Sắp theo `id` tăng dần — xem `Repository.ListBookmarks`.
func (s *Service) ListBookmarks(ctx context.Context, filter BookmarkFilter) ([]Bookmark, error) {
	if filter.Status != "" {
		st, err := ValidateBookmarkStatus(filter.Status)
		if err != nil {
			return nil, err
		}
		filter.Status = st
	}
	tag, err := ValidateTagFilter(filter.Tag)
	if err != nil {
		return nil, err
	}
	filter.Tag = tag

	rows, err := s.repo.ListBookmarks(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("đọc kho bookmark: %w", err)
	}
	return rows, nil
}

// BookmarkByID trả 1 bookmark, 404 khi không có hoặc đã xoá mềm.
func (s *Service) BookmarkByID(ctx context.Context, id int64) (Bookmark, error) {
	if id <= 0 {
		return Bookmark{}, newError(StatusBadRequest, "id bookmark không hợp lệ")
	}
	b, err := s.repo.BookmarkByID(ctx, id)
	if err != nil {
		return Bookmark{}, wrapNotFound(err, "không tìm thấy bookmark")
	}
	return b, nil
}

// CreateBookmark thêm 1 link vào kho. `guid` BẮT BUỘC sinh: `ux_roadmap_bookmarks_guid`
// là UNIQUE trên `NOT NULL DEFAULT ”` nên 2 dòng cùng guid rỗng là đụng nhau.
func (s *Service) CreateBookmark(ctx context.Context, in BookmarkInput) (Bookmark, error) {
	title, err := ValidateTitle(in.Title)
	if err != nil {
		return Bookmark{}, err
	}
	u, err := ValidateURL(in.URL)
	if err != nil {
		return Bookmark{}, err
	}
	note, err := ValidateText(in.Note, 2000)
	if err != nil {
		return Bookmark{}, err
	}
	tags, err := ValidateBookmarkTags(in.Tags)
	if err != nil {
		return Bookmark{}, err
	}
	status, err := ValidateBookmarkStatus(in.Status)
	if err != nil {
		return Bookmark{}, err
	}

	now := s.timestamp()
	var created Bookmark
	err = s.inTx(ctx, func(tx Tx) error {
		created = Bookmark{
			Title: title, URL: u, Note: note, Tags: tags, Status: status,
			CreatedAt: now, GUID: NewGUID(), UpdatedAt: now,
		}
		if err := s.repo.CreateBookmark(ctx, tx, &created); err != nil {
			if isUniqueViolation(err) {
				return newError(StatusConflict, "bookmark đã tồn tại")
			}
			return fmt.Errorf("tạo bookmark: %w", err)
		}
		return nil
	})
	return created, err
}

// UpdateBookmark sửa bookmark. KHÔNG có `status` trong patch: đổi trạng thái là
// 1 hành động riêng (`SetBookmarkStatus`) vì UI đổi trạng thái bằng 1 nút bấm
// và không muốn mất trạng thái khi patch title lỗi.
func (s *Service) UpdateBookmark(ctx context.Context, id int64, patch BookmarkPatch) (Bookmark, error) {
	if id <= 0 {
		return Bookmark{}, newError(StatusBadRequest, "id bookmark không hợp lệ")
	}
	if patch.Title == nil && patch.URL == nil && patch.Note == nil &&
		patch.Tags == nil && !patch.ClearURL {
		return Bookmark{}, newError(StatusBadRequest, "không có gì để cập nhật")
	}
	touchesURL := patch.ClearURL || patch.URL != nil
	var u *string
	if patch.URL != nil {
		v, err := ValidateURL(patch.URL)
		if err != nil {
			return Bookmark{}, err
		}
		u = v
	}
	var tags *string
	if patch.Tags != nil {
		v, err := ValidateBookmarkTags(*patch.Tags)
		if err != nil {
			return Bookmark{}, err
		}
		tags = &v
	}

	var updated Bookmark
	err := s.inTx(ctx, func(tx Tx) error {
		b, err := s.repo.BookmarkByID(ctx, id)
		if err != nil {
			return wrapNotFound(err, "không tìm thấy bookmark")
		}
		if patch.Title != nil {
			if b.Title, err = ValidateTitle(*patch.Title); err != nil {
				return err
			}
		}
		if touchesURL {
			b.URL = u
		}
		if patch.Note != nil {
			if b.Note, err = ValidateText(*patch.Note, 2000); err != nil {
				return err
			}
		}
		if tags != nil {
			b.Tags = *tags
		}
		if err := s.repo.UpdateBookmark(ctx, tx, &b); err != nil {
			return fmt.Errorf("cập nhật bookmark: %w", err)
		}
		updated = b
		return nil
	})
	return updated, err
}

// DeleteBookmark xoá mềm (tombstone để sync lan truyền). 404 khi đã xoá.
func (s *Service) DeleteBookmark(ctx context.Context, id int64) error {
	if id <= 0 {
		return newError(StatusBadRequest, "id bookmark không hợp lệ")
	}
	return s.inTx(ctx, func(tx Tx) error {
		if _, err := s.repo.BookmarkByID(ctx, id); err != nil {
			return wrapNotFound(err, "không tìm thấy bookmark")
		}
		if err := s.repo.SoftDeleteBookmark(ctx, tx, id); err != nil {
			return fmt.Errorf("xoá bookmark: %w", err)
		}
		return nil
	})
}

// SetBookmarkStatus đổi trạng thái kho link. Không có `completed_at` như node
// roadmap: bookmark không nằm trên đường đi, không có ý nghĩa "hoàn thành màn".
func (s *Service) SetBookmarkStatus(ctx context.Context, id int64, status string) (Bookmark, error) {
	if id <= 0 {
		return Bookmark{}, newError(StatusBadRequest, "id bookmark không hợp lệ")
	}
	st, err := ValidateBookmarkStatus(status)
	if err != nil {
		return Bookmark{}, err
	}
	var updated Bookmark
	err = s.inTx(ctx, func(tx Tx) error {
		cur, err := s.repo.BookmarkByID(ctx, id)
		if err != nil {
			return wrapNotFound(err, "không tìm thấy bookmark")
		}
		// Ghi vào struct vừa đọc, không đọc lại — xem SetStageStatus.
		cur.Status = st
		if err := s.repo.UpdateBookmark(ctx, tx, &cur); err != nil {
			return fmt.Errorf("lưu trạng thái bookmark: %w", err)
		}
		updated = cur
		return nil
	})
	return updated, err
}

// ── Progress ────────────────────────────────────────────────────────────────

// Progress là payload `GET /progress` (A1). Percent chỉ tính topic bắt buộc —
// loại is_optional khỏi mẫu số, floor, 0 topic ⇒ 0.
type Progress struct {
	Stages           int     `json:"stages"`
	TopicsTotal      int     `json:"topics_total"`
	TopicsRequired   int     `json:"topics_required"`
	TopicsOptional   int     `json:"topics_optional"`
	TopicsDone       int     `json:"topics_done"`
	TopicsInProgress int     `json:"topics_in_progress"`
	TopicsLocked     int     `json:"topics_locked"`
	Percent          int     `json:"percent"`
	LastCompletedAt  *string `json:"last_completed_at"`
	// CompletedInRange là số node bắt buộc hoàn thành trong [since, now).
	// 0 khi caller không truyền `since`.
	CompletedInRange int `json:"completed_in_range"`
}

// Progress tính tiến độ 1 path. since nil = không đếm khoảng.
func (s *Service) Progress(ctx context.Context, slug string, since *time.Time) (Progress, error) {
	p, err := s.PathBySlug(ctx, slug)
	if err != nil {
		return Progress{}, err
	}
	return s.ProgressByIDs(ctx, []int64{p.ID}, since)
}

// ProgressByIDs tính tiến độ GỘP của nhiều path bằng 2 lệnh batch (`stages
// IN`, `topics IN`).
//
// CHỈ đúng cho 1 path — tức `Progress(slug)` và dataloader `Path.progress` (mỗi
// lần load 1 pathID). Dùng cho DANH SÁCH là sai: kết quả là tổng của mọi path.
// `ListPaths` và `Path.progress` trong query list phải dùng `ProgressByPathIDs`.
func (s *Service) ProgressByIDs(ctx context.Context, pathIDs []int64, since *time.Time) (Progress, error) {
	stages, topics, _, err := s.treeParts(ctx, pathIDs)
	if err != nil {
		return Progress{}, err
	}
	all := make([]domain.Topic, 0, len(topics))
	byStage := map[int64][]domain.Topic{}
	for _, r := range topics {
		d := TopicToDomain(r)
		all = append(all, d)
		byStage[r.StageID] = append(byStage[r.StageID], d)
	}
	return s.progressFromTopics(stages, all, byStage, since), nil
}

// ProgressByPathIDs là batch của ProgressByIDs: TIẾN ĐỘ RIÊNG cho từng path, trong
// 2 lệnh (`stages IN`, `topics IN`).
//
// `ProgressByIDs` gộp tất cả path thành 1 con số, đúng cho `pathProgress(slug)`
// (1 path) nhưng SAI cho `Path.progress` trong query list — mọi path sẽ hiện
// cùng 1 tổng. Đây là hình dạng `ListPaths` + dataloader `Path.progress` cần.
func (s *Service) ProgressByPathIDs(ctx context.Context, pathIDs []int64) (map[int64]Progress, error) {
	stages, topics, _, err := s.treeParts(ctx, pathIDs)
	if err != nil {
		return nil, err
	}
	topicsByStage := make(map[int64][]Topic, len(stages))
	for _, t := range topics {
		topicsByStage[t.StageID] = append(topicsByStage[t.StageID], t)
	}
	stagesByPath := make(map[int64][]Stage, len(pathIDs))
	for _, st := range stages {
		stagesByPath[st.PathID] = append(stagesByPath[st.PathID], st)
	}
	out := make(map[int64]Progress, len(pathIDs))
	for _, pathID := range pathIDs {
		own := stagesByPath[pathID]
		all := make([]domain.Topic, 0, len(topics))
		byStage := make(map[int64][]domain.Topic, len(own))
		for _, st := range own {
			for _, r := range topicsByStage[st.ID] {
				d := TopicToDomain(r)
				all = append(all, d)
				byStage[st.ID] = append(byStage[st.ID], d)
			}
		}
		out[pathID] = s.progressFromTopics(own, all, byStage, nil)
	}
	return out, nil
}

// progressFromTopics là phần tính tiến độ dùng chung cho Progress và GetPath —
// cả 2 đều đã có sẵn stage + topic trong tay, đọc DB lần nữa chỉ tốn
// connection. `byStage` là map stageID → topic của stage đó.
func (s *Service) progressFromTopics(stages []Stage, topics []domain.Topic, byStage map[int64][]domain.Topic, since *time.Time) Progress {
	locked := 0
	liveStages := 0
	for _, st := range stages {
		if st.Deleted != 0 {
			continue
		}
		liveStages++
		// topics_locked tính theo chuỗi level TRONG TỪNG STAGE: state node suy
		// ra từ node trước cùng stage, nên gộp mọi stage lại rồi đếm là sai
		// (màn cuối stage 1 không mở chỉ vì stage 2 đã làm xong).
		local := make([]domain.Topic, 0, len(byStage[st.ID]))
		for _, t := range byStage[st.ID] {
			if t.IsOptional != domain.Optional {
				local = append(local, t)
			}
		}
		for _, state := range domain.LevelStates(local) {
			if state == domain.LevelLocked {
				locked++
			}
		}
	}

	prog := domain.ComputeProgress(topics)
	out := Progress{
		Stages:           liveStages,
		TopicsTotal:      prog.TopicsTotal,
		TopicsRequired:   prog.TopicsRequired,
		TopicsDone:       prog.TopicsDone,
		TopicsInProgress: prog.TopicsInProgress,
		TopicsLocked:     locked,
		Percent:          prog.Percent,
		LastCompletedAt:  lastCompletedAt(topics),
	}
	for _, t := range topics {
		if t.Deleted != 0 {
			continue
		}
		if t.IsOptional == domain.Optional {
			out.TopicsOptional++
		}
	}
	if since != nil {
		out.CompletedInRange = domain.CompletedSince(topics, since.UTC(), time.Time{})
	}
	return out
}

// lastCompletedAt là mốc completed_at mới nhất ("hoàn thành gần nhất lúc nào")
// cho header biểu đồ tuần.
func lastCompletedAt(topics []domain.Topic) *string {
	var best *string
	for _, t := range topics {
		if t.Deleted != 0 || t.CompletedAt == nil {
			continue
		}
		if best == nil || *t.CompletedAt > *best {
			v := *t.CompletedAt
			best = &v
		}
	}
	return best
}

// TopicToDomain chuyển row application → input của policy domain.
//
// Đây là ĐÚNG 1 chỗ chuyển đổi giữa DTO của tầng này và `domain.Topic`. Domain
// chỉ giữ field mà policy thật sự đọc (xem domain/roadmap/topic.go) — KHÔNG
// phải mirror của row, nên đừng thêm field "cho đủ" ở đây.
func TopicToDomain(t Topic) domain.Topic {
	return domain.Topic{
		ID: t.ID, StageID: t.StageID, Position: t.Position,
		Status:      domain.Status(t.Status),
		CompletedAt: t.CompletedAt,
		IsOptional:  domain.IsOptional(t.IsOptional == 1),
		MapX:        t.MapX, MapY: t.MapY,
		Deleted: t.Deleted,
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// parseMapField bọc lỗi của ParseTerrain/ParseDirection thành *Error 400 với
// message tiếng Việt của tầng application (thay vì lộ lỗi thô của domain).
// Generic vì 2 parser trả 2 kiểu value khác nhau (Terrain / Direction).
func parseMapField[T ~string](raw string, parse func(string) (T, error)) (string, error) {
	v, err := parse(raw)
	if err != nil {
		return "", newError(StatusBadRequest, "%s", err.Error())
	}
	return string(v), nil
}

// parseOptionalStatus đọc status khi tạo node: nil/rỗng = not_started, sai thì
// 400 với đủ 4 giá trị.
func parseOptionalStatus(in *string) (Status, error) {
	if in == nil || strings.TrimSpace(*in) == "" {
		return StatusNotStarted, nil
	}
	s := strings.TrimSpace(*in)
	if !ValidStatus(s) {
		return "", newError(StatusBadRequest, "status chỉ nhận: not_started, in_progress, done, skipped")
	}
	return s, nil
}

// resolveDeck kiểm tra deck_id qua port sang context `srs` và trả về thông tin
// deck đã xác nhận (tên + ngôn ngữ cho M6 hiển thị). nil/<=0 = chưa gắn deck.
//
// Port chưa bind (decks == nil) KHÔNG bị chặn âm thầm: trả 500 với message rõ
// để lộ ra lúc boot/test thay vì giả định "deck tồn tại" và ghi FK hỏng.
//
// `pathLang` là `path.language` của path chứa stage. Chỉ so khi CẢ HAI vế rơi
// vào tập đóng băng {zh, en} mới chặn — `path.language` là chuỗi tự do tối đa
// 16 ký tự (user tạo path "vi", "zh-Hans"), so khớp với deck zh|en trong mọi
// trường hợp sẽ chặn oan những path hợp lệ.
func (s *Service) resolveDeck(ctx context.Context, pathLang string, id *int64) (*DeckInfo, error) {
	if id == nil || *id <= 0 {
		return nil, nil
	}
	if s.decks == nil {
		return nil, newError(StatusInternalServerError, "chưa sẵn sàng kiểm tra deck")
	}
	info, err := s.decks.Find(ctx, *id)
	if err != nil {
		return nil, fmt.Errorf("kiểm tra deck: %w", err)
	}
	if !info.Exists {
		return nil, newError(StatusBadRequest, "không tìm thấy deck")
	}
	if pathFrozen, ok := frozenLang(pathLang); ok {
		if deckLang, ok := frozenLang(info.Lang); ok && deckLang != pathFrozen {
			return nil, newError(StatusBadRequest,
				"deck ngôn ngữ %s không khớp learning path ngôn ngữ %s", deckLang, pathFrozen)
		}
	}
	return &DeckInfo{Exists: true, Name: info.Name, Lang: info.Lang}, nil
}

// frozenLang map ngôn ngữ về tập đóng băng {zh, en} của `decks.lang`; trả
// false cho ngôn ngữ tự do (path user tự tạo).
func frozenLang(l string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(l)) {
	case "zh":
		return "zh", true
	case "en":
		return "en", true
	default:
		return "", false
	}
}

// nextXPosition trả position do client chỉ định, hoặc max+1 khi client không
// gửi — v1 làm vậy để thêm node liên tiếp không phải tự đếm.
func nextPosition(given *int, last int) (int, error) {
	if given != nil {
		if err := ValidatePosition(*given); err != nil {
			return 0, err
		}
		return *given, nil
	}
	return last + 1, nil
}

func (s *Service) nextStagePosition(ctx context.Context, pathID int64, given *int) (int, error) {
	last, err := s.repo.MaxStagePosition(ctx, pathID)
	if err != nil {
		return 0, fmt.Errorf("tính position của stage: %w", err)
	}
	return nextPosition(given, last)
}

func (s *Service) nextTopicPosition(ctx context.Context, stageID int64, given *int) (int, error) {
	last, err := s.repo.MaxTopicPosition(ctx, stageID)
	if err != nil {
		return 0, fmt.Errorf("tính position của topic: %w", err)
	}
	return nextPosition(given, last)
}

func (s *Service) nextResourcePosition(ctx context.Context, topicID int64, given *int) (int, error) {
	last, err := s.repo.MaxResourcePosition(ctx, topicID)
	if err != nil {
		return 0, fmt.Errorf("tính position của resource: %w", err)
	}
	return nextPosition(given, last)
}

func (s *Service) nextMilestonePosition(ctx context.Context, stageID int64, given *int) (int, error) {
	last, err := s.repo.MaxMilestonePosition(ctx, stageID)
	if err != nil {
		return 0, fmt.Errorf("tính position của milestone: %w", err)
	}
	return nextPosition(given, last)
}

// validateMapCoord = ValidateMapCoord + viewBox của service (viewBox lấy từ
// domain nên không có con số lặp lại ở đây).
func (s *Service) validateMapCoord(v *float64, name string, max float64) (*float64, error) {
	return ValidateMapCoord(v, name, max)
}

// inTx chạy fn trong UnitOfWork. uow nil (test chỉ cần validate) → chạy thẳng.
func (s *Service) inTx(ctx context.Context, fn func(tx Tx) error) error {
	if s.uow == nil {
		return fn(nil)
	}
	return s.uow.Do(ctx, fn)
}

// wrapNotFound dịch sentinel ErrNotFound của repository thành *Error 404 với
// message tiếng Việt; lỗi kỹ thuật khác đi nguyên vẹn (transport sẽ 500).
func wrapNotFound(err error, msg string) error {
	if err == ErrNotFound {
		return newError(StatusNotFound, "%s", msg)
	}
	return err
}

// isUniqueViolation nhận diện lỗi UNIQUE. Postgres dùng SQLSTATE 23505 — kiểm
// tra mã thay vì so khớp message (message đổi theo phiên bản). "UNIQUE
// constraint" giữ lại vì lỗi đã bọc qua driver có thể mất SQLSTATE.
func isUniqueViolation(err error) bool {
	return err != nil &&
		(strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "UNIQUE constraint"))
}

// EncodeActivities lưu activities thành JSON array string. Rỗng → "" (không
// phải "[]") để khớp DEFAULT của cột activities NOT NULL.
func EncodeActivities(in []string) string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return ""
	}
	b, err := json.Marshal(out)
	if err != nil {
		// Không xảy ra với []string, nhưng fallback newline-joined để không
		// mất dữ liệu người dùng nếu Marshal đổi hành vi.
		return strings.Join(out, "\n")
	}
	return string(b)
}
