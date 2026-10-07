package roadmap

import (
	"context"
	"fmt"
)

// ── Resource ────────────────────────────────────────────────────────────────

// ResourceInput defines the input payload for creating a resource.
type ResourceInput struct {
	Title    string
	URL      *string
	Kind     string
	Note     string
	Position *int
}

// ResourcePatch defines fields that can be updated on a resource.
type ResourcePatch struct {
	Title    *string
	URL      *string
	Kind     *string
	Note     *string
	Position *int
	ClearURL bool
}

// CreateResource adds a new resource to a topic.
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

// UpdateResource updates editable fields of a resource.
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

// DeleteResource soft-deletes a resource.
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
