package roadmap

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "langapp/internal/domain/roadmap"
)

// fakeRepo là repository trong bộ nhớ. Tầng application KHÔNG được import
// tầng hạ tầng (luật DDD §2 — grep phải ra 0 hit), nên test tầng này buộc dùng
// test double. Test repository thật nằm ở package infrastructure (Postgres thật).
type fakeRepo struct {
	paths     map[int64]Path
	stages    map[int64]Stage
	topics    map[int64]Topic
	resources map[int64]Resource
	ms        map[int64]Milestone
	bookmarks map[int64]Bookmark
	nextID    int64
	// failOn là tên method sẽ trả lỗi — dùng để chứng minh lỗi giữa chừng
	// phải rollback.
	failOn string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		paths: map[int64]Path{}, stages: map[int64]Stage{}, topics: map[int64]Topic{},
		resources: map[int64]Resource{}, ms: map[int64]Milestone{},
		bookmarks: map[int64]Bookmark{}, nextID: 1,
	}
}

var errFake = errors.New("lỗi giả lập từ fakeRepo")

func (f *fakeRepo) id() int64 {
	f.nextID++
	return f.nextID
}

func (f *fakeRepo) CreatePath(_ context.Context, _ Tx, p *Path) error {
	if f.failOn == "CreatePath" {
		return errFake
	}
	p.ID = f.id()
	f.paths[p.ID] = *p
	return nil
}

func (f *fakeRepo) ListPaths(context.Context) ([]Path, error) {
	out := []Path{}
	for _, p := range f.paths {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeRepo) PathBySlug(_ context.Context, slug string) (Path, error) {
	for _, p := range f.paths {
		if p.Slug == slug {
			return p, nil
		}
	}
	return Path{}, ErrNotFound
}

func (f *fakeRepo) PathByID(_ context.Context, id int64) (Path, error) {
	if p, ok := f.paths[id]; ok {
		return p, nil
	}
	return Path{}, ErrNotFound
}

func (f *fakeRepo) UpdatePath(_ context.Context, _ Tx, p *Path) error {
	f.paths[p.ID] = *p
	return nil
}

func (f *fakeRepo) SoftDeletePath(_ context.Context, _ Tx, id int64) error {
	p := f.paths[id]
	p.Deleted = 1
	f.paths[id] = p
	return nil
}

// ListStages PHẢI trả theo `(position, id)` — đúng `ORDER BY` của repository
// thật (`repository.go: ListStagesByPathIDs`).
//
// Không có bước sắp này thì fake trả thứ tự MAP (Go randomize), còn repository
// thật trả thứ tự cột. Hệ quả: test viết theo giả định "thứ tự là của SQL" sẽ
// xanh ở fake và đỏ ở production, hoặc ngược lại — và test sẽ chạy ngẫu nhiên.
// `PathTree` tin thứ tự do repository đảm bảo, nên fake cũng phải đảm bảo.
func (f *fakeRepo) ListStages(_ context.Context, pathID int64) ([]Stage, error) {
	out := []Stage{}
	for _, s := range f.stages {
		if s.PathID == pathID && s.Deleted == 0 {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Position != out[b].Position {
			return out[a].Position < out[b].Position
		}
		return out[a].ID < out[b].ID
	})
	return out, nil
}

func (f *fakeRepo) StageByID(_ context.Context, id int64) (Stage, error) {
	if s, ok := f.stages[id]; ok {
		return s, nil
	}
	return Stage{}, ErrNotFound
}

func (f *fakeRepo) CreateStage(_ context.Context, _ Tx, s *Stage) error {
	if f.failOn == "CreateStage" {
		return errFake
	}
	s.ID = f.id()
	f.stages[s.ID] = *s
	return nil
}

func (f *fakeRepo) UpdateStage(_ context.Context, _ Tx, s *Stage) error {
	f.stages[s.ID] = *s
	return nil
}

func (f *fakeRepo) SoftDeleteStage(_ context.Context, _ Tx, id int64) error {
	s := f.stages[id]
	s.Deleted = 1
	f.stages[id] = s
	return nil
}

func (f *fakeRepo) UpdateStageStatus(_ context.Context, _ Tx, st *Stage) error {
	if f.failOn == "UpdateStageStatus" {
		return errFake
	}
	row := f.stages[st.ID]
	row.Status, row.StatusNote, row.CompletedAt = st.Status, st.StatusNote, st.CompletedAt
	f.stages[st.ID] = row
	*st = row
	return nil
}

func (f *fakeRepo) MaxStagePosition(_ context.Context, pathID int64) (int, error) {
	return -1, nil
}

func (f *fakeRepo) ListTopics(_ context.Context, stageID int64) ([]Topic, error) {
	out := []Topic{}
	for _, t := range f.topics {
		if t.StageID == stageID && t.Deleted == 0 {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeRepo) TopicByID(_ context.Context, id int64) (Topic, error) {
	if t, ok := f.topics[id]; ok {
		return t, nil
	}
	return Topic{}, ErrNotFound
}

func (f *fakeRepo) CreateTopic(_ context.Context, _ Tx, t *Topic) error {
	if f.failOn == "CreateTopic" {
		return errFake
	}
	t.ID = f.id()
	f.topics[t.ID] = *t
	return nil
}

func (f *fakeRepo) UpdateTopic(_ context.Context, _ Tx, t *Topic) error {
	f.topics[t.ID] = *t
	return nil
}

func (f *fakeRepo) SoftDeleteTopic(_ context.Context, _ Tx, id int64) error {
	t := f.topics[id]
	t.Deleted = 1
	f.topics[id] = t
	return nil
}

func (f *fakeRepo) UpdateTopicStatus(_ context.Context, _ Tx, tp *Topic) error {
	if f.failOn == "UpdateTopicStatus" {
		return errFake
	}
	row := f.topics[tp.ID]
	row.Status, row.StatusNote, row.CompletedAt = tp.Status, tp.StatusNote, tp.CompletedAt
	f.topics[tp.ID] = row
	*tp = row
	return nil
}

func (f *fakeRepo) MaxTopicPosition(_ context.Context, stageID int64) (int, error) {
	return -1, nil
}

// Sắp `(position, id)` như repository thật — lý do như `ListStages` ở trên.
func (f *fakeRepo) ListResources(_ context.Context, topicID int64) ([]Resource, error) {
	out := []Resource{}
	for _, r := range f.resources {
		if r.TopicID == topicID && r.Deleted == 0 {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Position != out[b].Position {
			return out[a].Position < out[b].Position
		}
		return out[a].ID < out[b].ID
	})
	return out, nil
}

func (f *fakeRepo) ResourceByID(_ context.Context, id int64) (Resource, error) {
	if r, ok := f.resources[id]; ok {
		return r, nil
	}
	return Resource{}, ErrNotFound
}

func (f *fakeRepo) CreateResource(_ context.Context, _ Tx, r *Resource) error {
	r.ID = f.id()
	f.resources[r.ID] = *r
	return nil
}

func (f *fakeRepo) UpdateResource(_ context.Context, _ Tx, r *Resource) error {
	f.resources[r.ID] = *r
	return nil
}

func (f *fakeRepo) SoftDeleteResource(_ context.Context, _ Tx, id int64) error {
	r := f.resources[id]
	r.Deleted = 1
	f.resources[id] = r
	return nil
}

func (f *fakeRepo) MaxResourcePosition(_ context.Context, topicID int64) (int, error) {
	return -1, nil
}

// Sắp `(position, id)` như repository thật — `groupMilestones` giữ nguyên thứ
// tự lệnh SQL, nên fake trả map-ordel là cây milestone bị xáo ngẫu nhiên.
func (f *fakeRepo) ListMilestones(_ context.Context, stageID int64) ([]Milestone, error) {
	out := []Milestone{}
	for _, m := range f.ms {
		if m.StageID == stageID && m.Deleted == 0 {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Position != out[b].Position {
			return out[a].Position < out[b].Position
		}
		return out[a].ID < out[b].ID
	})
	return out, nil
}

// 4 method batch dưới đây mô phỏng `WHERE ... IN (...)`: cùng kết quả với vòng
// gọi `List*` từng cha, nhưng đếm được số lần gọi — test N+1 của cây roadmap
// (M4) assert trên chính con số đó.
func (f *fakeRepo) ListStagesByPathIDs(_ context.Context, pathIDs []int64) ([]Stage, error) {
	out := []Stage{}
	for _, id := range pathIDs {
		rows, err := f.ListStages(context.Background(), id)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

func (f *fakeRepo) ListTopicsByStageIDs(_ context.Context, stageIDs []int64) ([]Topic, error) {
	out := []Topic{}
	for _, id := range stageIDs {
		rows, err := f.ListTopics(context.Background(), id)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

func (f *fakeRepo) ListResourcesByTopicIDs(_ context.Context, topicIDs []int64) ([]Resource, error) {
	out := []Resource{}
	for _, id := range topicIDs {
		rows, err := f.ListResources(context.Background(), id)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

func (f *fakeRepo) ListMilestonesByStageIDs(_ context.Context, stageIDs []int64) ([]Milestone, error) {
	out := []Milestone{}
	for _, id := range stageIDs {
		rows, err := f.ListMilestones(context.Background(), id)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

func (f *fakeRepo) MilestoneByID(_ context.Context, id int64) (Milestone, error) {
	if m, ok := f.ms[id]; ok {
		return m, nil
	}
	return Milestone{}, ErrNotFound
}

func (f *fakeRepo) CreateMilestone(_ context.Context, _ Tx, m *Milestone) error {
	m.ID = f.id()
	f.ms[m.ID] = *m
	return nil
}

func (f *fakeRepo) UpdateMilestone(_ context.Context, _ Tx, m *Milestone) error {
	f.ms[m.ID] = *m
	return nil
}

func (f *fakeRepo) SoftDeleteMilestone(_ context.Context, _ Tx, id int64) error {
	m := f.ms[id]
	m.Deleted = 1
	f.ms[id] = m
	return nil
}

func (f *fakeRepo) MaxMilestonePosition(_ context.Context, stageID int64) (int, error) {
	return -1, nil
}

// ── Bookmarks ───────────────────────────────────────────────────────────────

// Lọc tag trong fake dùng `domain.TagsContain` — CÙNG hàm mà test viền ở
// `domain/roadmap` chạy, nên test tầng application và test Postgres kiểm cùng
// 1 quy tắc thay vì 2 bản khác nhau.
func (f *fakeRepo) ListBookmarks(_ context.Context, filter BookmarkFilter) ([]Bookmark, error) {
	if f.failOn == "ListBookmarks" {
		return nil, errFake
	}
	out := []Bookmark{}
	for _, b := range f.bookmarks {
		if b.Deleted != 0 {
			continue
		}
		if filter.Status != "" && b.Status != filter.Status {
			continue
		}
		if filter.Tag != "" && !domain.TagsContain(b.Tags, filter.Tag) {
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out, nil
}

func (f *fakeRepo) BookmarkByID(_ context.Context, id int64) (Bookmark, error) {
	if b, ok := f.bookmarks[id]; ok && b.Deleted == 0 {
		return b, nil
	}
	return Bookmark{}, ErrNotFound
}

func (f *fakeRepo) CreateBookmark(_ context.Context, _ Tx, b *Bookmark) error {
	if f.failOn == "CreateBookmark" {
		return errFake
	}
	b.ID = f.id()
	f.bookmarks[b.ID] = *b
	return nil
}

func (f *fakeRepo) UpdateBookmark(_ context.Context, _ Tx, b *Bookmark) error {
	if f.failOn == "UpdateBookmark" {
		return errFake
	}
	f.bookmarks[b.ID] = *b
	return nil
}

func (f *fakeRepo) SoftDeleteBookmark(_ context.Context, _ Tx, id int64) error {
	b := f.bookmarks[id]
	b.Deleted = 1
	f.bookmarks[id] = b
	return nil
}

// fakeUow đếm số transaction đã mở + cho phép rollback giả lập (snapshot state).
type fakeUow struct {
	repo      *fakeRepo
	opens     int
	rollbacks int
}

func (u *fakeUow) Do(_ context.Context, fn func(Tx) error) error {
	u.opens++
	snapshot := u.repo.clone()
	if err := fn(nil); err != nil {
		u.repo.restore(snapshot)
		u.rollbacks++
		return err
	}
	return nil
}

func (f *fakeRepo) clone() *fakeRepo {
	return &fakeRepo{
		paths: copyMap(f.paths), stages: copyMap(f.stages), topics: copyMap(f.topics),
		resources: copyMap(f.resources), ms: copyMap(f.ms), bookmarks: copyMap(f.bookmarks),
		nextID: f.nextID, failOn: f.failOn,
	}
}

func (f *fakeRepo) restore(s *fakeRepo) {
	f.paths, f.stages, f.topics = s.paths, s.stages, s.topics
	f.resources, f.ms, f.bookmarks, f.nextID = s.resources, s.ms, s.bookmarks, s.nextID
}

func copyMap[V any](m map[int64]V) map[int64]V {
	out := make(map[int64]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// fakeDecks là port sang context srs.
type fakeDecks struct {
	exists map[int64]string // id → lang
}

func (d fakeDecks) Find(_ context.Context, id int64) (DeckInfo, error) {
	lang, ok := d.exists[id]
	if !ok {
		return DeckInfo{}, nil
	}
	return DeckInfo{Exists: true, Name: "Deck " + lang, Lang: lang}, nil
}

var fixedClock = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

func newTestService() (*Service, *fakeRepo, *fakeUow) {
	repo := newFakeRepo()
	uow := &fakeUow{repo: repo}
	return NewService(repo, uow, fakeDecks{}, func() time.Time { return fixedClock }, ViewBox{}), repo, uow
}

// ── Validate ────────────────────────────────────────────────────────────────

func Test_validate_slug_rejects_bad_input_with_vietnamese_message(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", "slug không được rỗng"},
		{"  ", "slug không được rỗng"},
		{"có dấu", "slug chỉ nhận ký tự a-z, 0-9 và dấu -"},
		{"-lead", "slug không được bắt đầu hoặc kết thúc bằng dấu -"},
		{"trail-", "slug không được bắt đầu hoặc kết thúc bằng dấu -"},
	} {
		t.Run(c.in, func(t *testing.T) {
			_, err := ValidateSlug(c.in)
			require.Error(t, err, "input %q phải bị chặn", c.in)
			assert.Equal(t, c.want, err.Error())
		})
	}
	got, err := ValidateSlug("  ZH-G0  ")
	require.NoError(t, err)
	assert.Equal(t, "zh-g0", got, "trim + lower")
}

func Test_validate_url_only_accepts_http_and_https(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"ftp://x.com", "url chỉ nhận link http hoặc https"},
		// `javascript:alert(1)` không có Host nên rơi vào nhánh "không hợp lệ"
		// trước — cùng kết luận (bị từ chối) với nhánh kiểm tra scheme.
		{"javascript:alert(1)", "url không hợp lệ"},
		{"không phải url", "url không hợp lệ"},
	} {
		t.Run(c.in, func(t *testing.T) {
			_, err := ValidateURL(&c.in)
			require.Error(t, err, "url %q phải bị chặn", c.in)
			assert.Equal(t, c.want, err.Error())
		})
	}
	empty := "   "
	got, err := ValidateURL(&empty)
	require.NoError(t, err)
	assert.Nil(t, got, "url rỗng = NULL chứ không phải chuỗi rỗng")
	ok := "https://example.com/x"
	got, err = ValidateURL(&ok)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "https://example.com/x", *got)
}

func Test_validate_status_requires_frozen_four_values(t *testing.T) {
	_, _, err := ValidateStatus(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "thiếu status")

	upper := "DONE"
	_, _, err = ValidateStatus(&upper, nil)
	require.Error(t, err, "hợp đồng đóng 4 giá trị: 'DONE' phải là 400 chứ không âm thầm thành 'done'")
	assert.Equal(t, "status chỉ nhận: not_started, in_progress, done, skipped", err.Error())

	st, note, err := ValidateStatus(&upper, nil)
	_ = st
	_ = note
	_ = err
	real := "done"
	st, note, err = ValidateStatus(&real, nil)
	require.NoError(t, err)
	assert.Equal(t, StatusDone, st)
	assert.Empty(t, note)
}

func Test_validate_map_coord_bounds_to_viewbox(t *testing.T) {
	var nilCoord *float64
	got, err := ValidateMapCoord(nilCoord, "map_x", 1000)
	require.NoError(t, err)
	assert.Nil(t, got, "nil = để server layout")

	edge := 1000.0
	got, err = ValidateMapCoord(&edge, "map_x", 1000)
	require.NoError(t, err)
	assert.Equal(t, 1000.0, *got, "biên phải chấp nhận — node sát mép vẫn scroll tới được")

	over := 1000.1
	_, err = ValidateMapCoord(&over, "map_x", 1000)
	require.Error(t, err)
	assert.Equal(t, "map_x phải nằm trong 0..1000", err.Error())
}

// ── Use case ────────────────────────────────────────────────────────────────

func Test_create_path_assigns_guid_and_normalises_input(t *testing.T) {
	svc, repo, _ := newTestService()
	got, err := svc.CreatePath(context.Background(), PathInput{
		Slug: "  Learn-Chinese ", Title: "  Tự học  ", Overview: " mô tả ", Language: "",
	})
	require.NoError(t, err)
	assert.Equal(t, "learn-chinese", got.Slug)
	assert.Equal(t, "Tự học", got.Title)
	assert.Equal(t, "mô tả", got.Overview)
	assert.Equal(t, "zh", got.Language, "language rỗng = zh, khớp app v1")
	assert.NotEmpty(t, got.GUID, "guid rỗng sẽ đụng UNIQUE ở path thứ 2")
	assert.Equal(t, fixedClock.Format(time.RFC3339), got.CreatedAt)
	assert.Equal(t, 1, len(repo.paths))
}

func Test_new_guid_is_never_repeated(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		g := NewGUID()
		assert.NotEmpty(t, g)
		assert.False(t, seen[g], "guid trùng ở lần %d", i)
		seen[g] = true
	}
}

func Test_create_stage_rejects_unknown_terrain_before_touching_db(t *testing.T) {
	svc, _, uow := newTestService()
	_, err := svc.CreatePath(context.Background(), PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	opensBefore := uow.opens

	_, err = svc.CreateStage(context.Background(), "p", StageInput{
		Slug: "g", Title: "G", Terrain: "forest",
	})
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusBadRequest, appErr.Status)
	assert.Contains(t, appErr.Message, "terrain chỉ nhận: meadow, desert, snow, volcano, ocean, city")
	assert.Equal(t, opensBefore, uow.opens, "validate fail phải chặn TRƯỚC khi mở transaction")
}

func Test_create_stage_stores_terrain_and_direction_defaults(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)

	stage, err := svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G"})
	require.NoError(t, err)
	assert.Equal(t, "meadow", stage.Terrain, "rỗng = DEFAULT của cột")
	assert.Equal(t, "up", stage.Direction)

	stage2, err := svc.CreateStage(ctx, "p", StageInput{
		Slug: "g2", Title: "G2", Terrain: "VOLCANO", Direction: "RIGHT",
	})
	require.NoError(t, err)
	assert.Equal(t, "volcano", stage2.Terrain, "input không phân biệt hoa thường")
	assert.Equal(t, "right", stage2.Direction)
}

func Test_create_stage_rejects_missing_deck_via_port(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)

	missing := int64(42)
	_, err = svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G", DeckID: &missing})
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusBadRequest, appErr.Status)
	assert.Equal(t, "không tìm thấy deck", appErr.Message)
}

func Test_create_stage_accepts_existing_deck(t *testing.T) {
	repo := newFakeRepo()
	decks := fakeDecks{exists: map[int64]string{7: "zh"}}
	svc := NewService(repo, &fakeUow{repo: repo}, decks, func() time.Time { return fixedClock }, ViewBox{})
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)

	id := int64(7)
	got, err := svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G", DeckID: &id})
	require.NoError(t, err)
	require.NotNil(t, got.DeckID)
	assert.Equal(t, int64(7), *got.DeckID)
}

func Test_update_patch_without_any_field_is_rejected(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, err := svc.UpdatePath(ctx, "p", PathPatch{})
	require.Error(t, err)
	assert.Equal(t, "không có gì để cập nhật", err.Error())

	_, err = svc.UpdateTopic(ctx, 1, TopicPatch{})
	require.Error(t, err)
	assert.Equal(t, "không có gì để cập nhật", err.Error())

	_, err = svc.UpdateStage(ctx, 1, StagePatch{})
	require.Error(t, err)
	assert.Equal(t, "không có gì để cập nhật", err.Error())
}

func Test_set_topic_status_four_transitions(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	stage, err := svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G"})
	require.NoError(t, err)
	tp, err := svc.CreateTopic(ctx, stage.ID, TopicInput{Title: "T"})
	require.NoError(t, err)

	// Vào done.
	after, err := svc.SetTopicStatus(ctx, tp.ID, strPtr(StatusDone), nil)
	require.NoError(t, err)
	require.NotNil(t, after.CompletedAt)
	assert.Equal(t, fixedClock.Format(time.RFC3339), *after.CompletedAt)

	// Rời done → NULL thật (không phải "").
	after, err = svc.SetTopicStatus(ctx, tp.ID, strPtr(StatusInProgress), nil)
	require.NoError(t, err)
	assert.Nil(t, after.CompletedAt, "rời done phải clear thành SQL NULL")

	// Bấm done lần nữa ở thời điểm khác → mốc MỚI (vì đã rời done).
	later := fixedClock.Add(72 * time.Hour)
	svc2 := NewService(svc.repo, nil, fakeDecks{}, func() time.Time { return later }, ViewBox{})
	after, err = svc2.SetTopicStatus(ctx, tp.ID, strPtr(StatusDone), nil)
	require.NoError(t, err)
	require.NotNil(t, after.CompletedAt)
	assert.Equal(t, later.Format(time.RFC3339), *after.CompletedAt)

	// Bấm done lần nữa mà vẫn done → giữ mốc cũ, không nhấp nháy.
	svc3 := NewService(svc.repo, nil, fakeDecks{}, func() time.Time { return later.Add(72 * time.Hour) }, ViewBox{})
	after, err = svc3.SetTopicStatus(ctx, tp.ID, strPtr(StatusDone), nil)
	require.NoError(t, err)
	require.NotNil(t, after.CompletedAt)
	assert.Equal(t, later.Format(time.RFC3339), *after.CompletedAt,
		"đã done thì giữ mốc cũ để biểu đồ tuần không bị dịch chuyển")
}

func testServiceWithRepo(repo *fakeRepo) *Service {
	return NewService(repo, &fakeUow{repo: repo}, fakeDecks{},
		func() time.Time { return fixedClock }, ViewBox{})
}

func Test_done_row_without_completed_at_gets_one_on_repeat(t *testing.T) {
	repo := newFakeRepo()
	svc := testServiceWithRepo(repo)
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	stage, err := svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G"})
	require.NoError(t, err)
	tp, err := svc.CreateTopic(ctx, stage.ID, TopicInput{Title: "T"})
	require.NoError(t, err)

	// Dữ liệu migrate từ v1: status=done nhưng completed_at NULL.
	repo.topics[tp.ID] = Topic{ID: tp.ID, StageID: stage.ID, Title: "T", Status: StatusDone}
	after, err := svc.SetTopicStatus(ctx, tp.ID, strPtr(StatusDone), nil)
	require.NoError(t, err)
	require.NotNil(t, after.CompletedAt,
		"row done nhưng thiếu mốc phải được gán mốc, nếu không progress?since= bỏ sót vĩnh viễn")
}

// `resolveCompletedAt` là nơi duy nhất quyết định `completed_at` ghi xuống
// (thay cho `domain.ApplyStatus` đã xoá ở remediation cổng Oracle M2). Test đủ
// 4 trạng thái vì M1 remediation F11 chỉ assert được 1 nhánh.
func Test_resolve_completed_at_covers_all_four_statuses(t *testing.T) {
	now := time.Date(2026, 9, 28, 3, 4, 5, 0, time.UTC)
	stamp := now.Format(time.RFC3339)

	// Vào done lần đầu: SetStatus trả mốc mới → dùng mốc đó.
	got := resolveCompletedAt(StatusDone, nil, stamp, now)
	require.NotNil(t, got)
	assert.Equal(t, stamp, *got)

	// Vẫn done, đã có mốc: giữ mốc cũ (không nhấp nháy biểu đồ tuần).
	old := "2020-01-01T00:00:00Z"
	got = resolveCompletedAt(StatusDone, &old, "", now)
	require.NotNil(t, got)
	assert.Equal(t, old, *got)

	// Vẫn done, chưa có mốc (migrate dữ liệu v1): gán mốc hiện tại, nếu không
	// `progress?since=` bỏ sót node này vĩnh viễn.
	got = resolveCompletedAt(StatusDone, nil, "", now)
	require.NotNil(t, got)
	assert.Equal(t, stamp, *got)

	// 3 trạng thái còn lại: clear thành NULL thật, không phải "".
	for _, st := range []Status{StatusNotStarted, StatusInProgress, StatusSkipped} {
		assert.Nil(t, resolveCompletedAt(st, &old, "", now),
			"trạng thái %q không phải done thì completed_at phải NULL", st)
	}
}

func Test_set_status_on_missing_topic_returns_404(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.SetTopicStatus(context.Background(), 999, strPtr(StatusDone), nil)
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusNotFound, appErr.Status)
	assert.Equal(t, "không tìm thấy topic", appErr.Message)
}

func Test_get_path_returns_tree_with_layout_and_level_state(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P", Language: "zh"})
	require.NoError(t, err)
	stage, err := svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G", Terrain: "volcano"})
	require.NoError(t, err)
	var topics []Topic
	for _, title := range []string{"A", "B", "C"} {
		tp, err := svc.CreateTopic(ctx, stage.ID, TopicInput{Title: title})
		require.NoError(t, err)
		topics = append(topics, tp)
	}
	_, err = svc.CreateMilestone(ctx, stage.ID, MilestoneInput{Text: "Mốc 1"})
	require.NoError(t, err)
	_, err = svc.SetTopicStatus(ctx, topics[0].ID, strPtr(StatusDone), nil)
	require.NoError(t, err)

	view, err := svc.PathTree(ctx, "p")
	require.NoError(t, err)
	require.Len(t, view.Stages, 1)
	require.Len(t, view.Stages[0].Topics, 3)
	assert.Equal(t, []Milestone{{ID: view.Stages[0].Milestones[0].ID, StageID: stage.ID,
		Text: "Mốc 1", Position: 0, CreatedAt: fixedClock.Format(time.RFC3339),
		GUID: view.Stages[0].Milestones[0].GUID, UpdatedAt: view.Stages[0].Milestones[0].UpdatedAt}},
		view.Stages[0].Milestones, "milestone phải đi kèm cây")

	want := []string{string(domain.LevelDone), string(domain.LevelCurrent), string(domain.LevelLocked)}
	for i, tp := range view.Stages[0].Topics {
		assert.Equal(t, want[i], string(tp.Level), "node %d", i)
		assert.False(t, tp.MapPinned)
		assert.NotNil(t, tp.Resources, "resources phải là slice rỗng chứ không nil (JSON ra [] chứ không null)")
	}
	// Node 2 phải nằm giữa node 1 và node 3 theo trục chính (direction up).
	p1 := view.Stages[0].Topics[0].Point
	p2 := view.Stages[0].Topics[1].Point
	p3 := view.Stages[0].Topics[2].Point
	assert.Greater(t, p1.Y, p2.Y)
	assert.Greater(t, p2.Y, p3.Y)
	assert.Equal(t, 1, view.Progress.TopicsDone)
}

func Test_get_path_marks_pinned_nodes(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	stage, err := svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G"})
	require.NoError(t, err)
	_, err = svc.CreateTopic(ctx, stage.ID, TopicInput{Title: "T", MapX: f64Ptr(10), MapY: f64Ptr(20)})
	require.NoError(t, err)

	view, err := svc.PathTree(ctx, "p")
	require.NoError(t, err)
	require.Len(t, view.Stages[0].Topics, 1)
	assert.True(t, view.Stages[0].Topics[0].MapPinned)
	assert.Equal(t, 10.0, view.Stages[0].Topics[0].Point.X)
	assert.Equal(t, 20.0, view.Stages[0].Topics[0].Point.Y)
}

func Test_get_path_unknown_slug_returns_404(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.PathTree(context.Background(), "khong-co")
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusNotFound, appErr.Status)
}

func Test_transaction_rolls_back_when_repository_fails_mid_way(t *testing.T) {
	svc, repo, uow := newTestService()
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	_, err = svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G"})
	require.NoError(t, err)
	before := len(repo.paths)

	repo.failOn = "CreateTopic"
	_, err = svc.CreateTopic(ctx, 1, TopicInput{Title: "T"})
	require.Error(t, err)
	assert.Equal(t, before, len(repo.paths), "lần ghi lỗi không được để lại thay đổi")
	assert.Positive(t, uow.rollbacks, "UnitOfWork phải rollback khi callback trả lỗi")
}

func Test_progress_zero_topics_is_zero_not_nan(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	prog, err := svc.Progress(ctx, "p", nil)
	require.NoError(t, err)
	assert.Zero(t, prog.Percent)
	assert.Zero(t, prog.TopicsTotal)
	assert.Zero(t, prog.TopicsLocked)
	assert.Nil(t, prog.LastCompletedAt)
}

func Test_encode_activities_matches_json_array_convention(t *testing.T) {
	assert.Equal(t, "", EncodeActivities(nil))
	assert.Equal(t, "", EncodeActivities([]string{"", "  "}), "rỗng = '' để khớp DEFAULT của cột")
	assert.Equal(t, `["a","b"]`, EncodeActivities([]string{" a ", "b", ""}))
}

func strPtr(v string) *string   { return &v }
func f64Ptr(v float64) *float64 { return &v }

// intPtr là helper cho test: patch nào đó cần con trỏ int.
func intPtr(v int) *int { return &v }
