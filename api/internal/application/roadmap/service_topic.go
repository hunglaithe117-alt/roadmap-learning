package roadmap

import (
	"context"
	"fmt"

	domain "langapp/internal/domain/roadmap"
)

// ── Topic ───────────────────────────────────────────────────────────────────

// TopicInput defines the input payload for creating a topic.
type TopicInput struct {
	Title      string
	Why        string
	Activities []string
	Position   *int
	IsOptional *int
	MapX       *float64
	MapY       *float64
}

// TopicPatch defines fields that can be updated on a topic.
type TopicPatch struct {
	Title      *string
	Why        *string
	Activities *[]string
	Position   *int
	IsOptional *int
	MapX       *float64
	MapY       *float64
	ClearMap   bool
}

// CreateTopic adds a new topic to a stage.
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

// UpdateTopic updates editable fields of a topic.
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

// DeleteTopic soft-deletes a topic and its resources.
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

// SetTopicStatus updates status and completed_at for a topic.
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
