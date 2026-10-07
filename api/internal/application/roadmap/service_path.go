package roadmap

import (
	"context"
	"fmt"
)

// ── Path ────────────────────────────────────────────────────────────────────

// PathInput defines the input payload for creating a path.
type PathInput struct {
	Slug     string
	Title    string
	Overview string
	Language string
}

// PathSummary represents a path listing item along with progress metrics.
type PathSummary struct {
	Path
	Progress Summary
}

// Summary provides progress overview metrics for a path.
type Summary struct {
	Stages           int `json:"stages"`
	TopicsTotal      int `json:"topics_total"`
	TopicsRequired   int `json:"topics_required"`
	TopicsDone       int `json:"topics_done"`
	TopicsInProgress int `json:"topics_in_progress"`
	Percent          int `json:"percent"`
}

// PathPatch defines fields that can be updated on a path.
type PathPatch struct {
	Title    *string
	Overview *string
	Language *string
}

// CreatePath creates a new learning path.
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

// ListPaths lists all non-deleted paths with individual progress.
func (s *Service) ListPaths(ctx context.Context) ([]PathSummary, error) {
	paths, err := s.repo.ListPaths(ctx)
	if err != nil {
		return nil, fmt.Errorf("đọc danh sách learning path: %w", err)
	}
	ids := make([]int64, 0, len(paths))
	for _, p := range paths {
		ids = append(ids, p.ID)
	}
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

// UpdatePath updates the editable fields of a learning path.
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

// DeletePath soft-deletes a path and its child nodes.
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
