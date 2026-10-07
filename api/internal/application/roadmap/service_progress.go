package roadmap

import (
	"context"
	"fmt"
	"time"

	domain "langapp/internal/domain/roadmap"
)

// ── Bookmark ────────────────────────────────────────────────────────────────

// BookmarkInput defines the input payload for creating a bookmark.
type BookmarkInput struct {
	Title  string
	URL    *string
	Note   string
	Tags   []string
	Status string
}

// BookmarkPatch defines fields that can be updated on a bookmark.
type BookmarkPatch struct {
	Title    *string
	URL      *string
	Note     *string
	Tags     *[]string
	ClearURL bool
}

// ListBookmarks lists bookmarks according to filter options.
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

// BookmarkByID retrieves a single bookmark by id.
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

// CreateBookmark creates a new bookmark.
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

// UpdateBookmark updates editable fields of a bookmark.
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

// DeleteBookmark soft-deletes a bookmark.
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

// SetBookmarkStatus updates status of a bookmark.
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

// Progress represents learning path completion metrics.
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
	CompletedInRange int     `json:"completed_in_range"`
}

// Progress computes metrics for a single path.
func (s *Service) Progress(ctx context.Context, slug string, since *time.Time) (Progress, error) {
	p, err := s.PathBySlug(ctx, slug)
	if err != nil {
		return Progress{}, err
	}
	return s.ProgressByIDs(ctx, []int64{p.ID}, since)
}

// ProgressByIDs aggregates progress across given path IDs.
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

// ProgressByPathIDs computes progress per path ID for batch requests.
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

// progressFromTopics computes progress metrics from loaded topic domains.
func (s *Service) progressFromTopics(stages []Stage, topics []domain.Topic, byStage map[int64][]domain.Topic, since *time.Time) Progress {
	locked := 0
	liveStages := 0
	for _, st := range stages {
		if st.Deleted != 0 {
			continue
		}
		liveStages++
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

// lastCompletedAt finds the newest completed_at timestamp among topics.
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

// TopicToDomain converts an application Topic row to a domain Topic.
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
