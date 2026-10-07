package roadmap

import (
	"context"
	"fmt"
)

// ── Milestone ───────────────────────────────────────────────────────────────

// MilestoneInput defines the input payload for creating a milestone.
type MilestoneInput struct {
	Text     string
	Position *int
}

// MilestonePatch defines fields that can be updated on a milestone.
type MilestonePatch struct {
	Text     *string
	Position *int
}

// CreateMilestone adds a milestone to a stage.
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

// UpdateMilestone updates editable fields of a milestone.
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

// DeleteMilestone soft-deletes a milestone.
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
