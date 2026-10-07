package roadmap

import (
	"context"
	"fmt"
	"time"

	domain "langapp/internal/domain/roadmap"
)

// ── Stage ───────────────────────────────────────────────────────────────────

// StageInput defines the input payload for creating a stage.
type StageInput struct {
	Slug          string
	Title         string
	Goal          string
	Position      *int
	DurationWeeks *int
	Status        *string
	DeckID        *int64
	Terrain       string
	Direction     string
}

// StagePatch defines fields that can be updated on a stage.
type StagePatch struct {
	Title         *string
	Goal          *string
	Position      *int
	DurationWeeks *int
	DeckID        *int64
	Terrain       *string
	Direction     *string
}

// CreateStage adds a new stage to a path.
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

// UpdateStage updates editable fields of a stage.
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

// DeleteStage soft-deletes a stage and its child nodes.
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

// SetStageStatus updates status and completed_at for a stage.
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

// resolveCompletedAt determines the appropriate completed_at timestamp.
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
