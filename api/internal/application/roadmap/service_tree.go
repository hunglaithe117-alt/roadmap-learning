package roadmap

import (
	"context"
	"fmt"
	"sort"

	domain "langapp/internal/domain/roadmap"
	"langapp/internal/typednil"
)

// ── Tree + Map ────────────────────────────────────────────────────────────

// TopicView represents a topic with map layout points and level state.
type TopicView struct {
	Topic
	Level     domain.LevelState
	Point     domain.MapPoint
	MapPinned bool
	Resources []Resource
}

// StageView represents a stage with decorated child topics and milestones.
type StageView struct {
	Stage
	Topics     []TopicView
	Milestones []Milestone
	Deck       *DeckRef `json:"deck"`
}

// PathView represents a complete learning path tree with progress.
type PathView struct {
	Path
	Stages   []StageView
	Progress Progress
}

// PathTree loads a full learning path tree including layout coordinates.
func (s *Service) PathTree(ctx context.Context, slug string) (PathView, error) {
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

// resourcesFor fetches resources for all topics across decorated stages in a single query.
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

// StageByID retrieves a single stage by id.
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

// TopicByID retrieves a topic along with stage-wide computed layout and LevelState.
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

// ResourceByID retrieves a resource by id.
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

// MilestoneByID retrieves a milestone by id.
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

// PathBySlug retrieves a learning path by slug without loading the tree.
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

// StageTreeByPathIDs returns stages, decorated topics, and milestones grouped by pathID.
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

// StageTreeByIDs loads stages, decorated topics, and milestones across given path IDs.
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

// groupMilestones groups milestones by stageID.
func groupMilestones(rows []Milestone) map[int64][]Milestone {
	out := make(map[int64][]Milestone)
	for _, m := range rows {
		out[m.StageID] = append(out[m.StageID], m)
	}
	return out
}

// ResourcesByTopicIDs fetches resources grouped by topicID for GraphQL batch loading.
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

// treeParts loads stages, topics, and milestones for paths in three batch queries.
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

// decorateTopics computes map layout points and LevelState for each topic.
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

// attachDeck binds deck information to a stage if configured.
func (s *Service) attachDeck(ctx context.Context, sv *StageView) error {
	if sv.DeckID == nil {
		return nil
	}
	if typednil.Is(s.decks) {
		return newError(StatusInternalServerError, "chưa sẵn sàng đọc deck của stage")
	}
	info, err := s.decks.Find(ctx, *sv.DeckID)
	if err != nil {
		return fmt.Errorf("đọc deck của stage %d: %w", sv.ID, err)
	}
	if info.Exists {
		sv.Deck = &DeckRef{ID: *sv.DeckID, Name: info.Name, Lang: info.Lang}
	}
	return nil
}
