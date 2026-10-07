package roadmapinfra

import (
	"context"
	"fmt"

	app "langapp/internal/application/roadmap"
)

// ListStagesByPathIDs fetches stages for multiple paths in a single batch query.
func (r *Repository) ListStagesByPathIDs(ctx context.Context, pathIDs []int64) ([]app.Stage, error) {
	if len(pathIDs) == 0 {
		return nil, nil
	}
	var rows []Stage
	err := r.db.WithContext(ctx).Where("path_id IN ? AND deleted = 0", pathIDs).
		Order("path_id, position, id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list roadmap stages by path: %w", err)
	}
	out := make([]app.Stage, 0, len(rows))
	for _, row := range rows {
		out = append(out, stageToApp(row))
	}
	return out, nil
}

// ListTopicsByStageIDs fetches topics for multiple stages in a single batch query.
func (r *Repository) ListTopicsByStageIDs(ctx context.Context, stageIDs []int64) ([]app.Topic, error) {
	if len(stageIDs) == 0 {
		return nil, nil
	}
	var rows []Topic
	err := r.db.WithContext(ctx).Where("stage_id IN ? AND deleted = 0", stageIDs).
		Order("stage_id, position, id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list roadmap topics by stage: %w", err)
	}
	out := make([]app.Topic, 0, len(rows))
	for _, row := range rows {
		out = append(out, topicToApp(row))
	}
	return out, nil
}

// ListResourcesByTopicIDs fetches resources for multiple topics in a single batch query.
func (r *Repository) ListResourcesByTopicIDs(ctx context.Context, topicIDs []int64) ([]app.Resource, error) {
	if len(topicIDs) == 0 {
		return nil, nil
	}
	var rows []Resource
	err := r.db.WithContext(ctx).Where("topic_id IN ? AND deleted = 0", topicIDs).
		Order("topic_id, position, id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list roadmap resources by topic: %w", err)
	}
	out := make([]app.Resource, 0, len(rows))
	for _, row := range rows {
		out = append(out, resToApp(row))
	}
	return out, nil
}

// ListMilestonesByStageIDs fetches milestones for multiple stages in a single batch query.
func (r *Repository) ListMilestonesByStageIDs(ctx context.Context, stageIDs []int64) ([]app.Milestone, error) {
	if len(stageIDs) == 0 {
		return nil, nil
	}
	var rows []Milestone
	err := r.db.WithContext(ctx).Where("stage_id IN ? AND deleted = 0", stageIDs).
		Order("stage_id, position, id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list roadmap milestones by stage: %w", err)
	}
	out := make([]app.Milestone, 0, len(rows))
	for _, row := range rows {
		out = append(out, msToApp(row))
	}
	return out, nil
}
