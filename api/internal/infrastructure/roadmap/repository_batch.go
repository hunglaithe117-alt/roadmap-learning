package roadmapinfra

import (
	"context"
	"fmt"

	app "langapp/internal/application/roadmap"
)

// Batch read cho dataloader GraphQL (M4).
//
// Vì sao có: cây roadmap 5 tầng đọc bằng GraphQL sẽ gọi `ListTopics` /
// `ListResources` 1 lần cho MỖI node. Với seed 51 topic + 263 resource đó là
// 315 statement cho 1 request, và số statement đó TĂNG TUYẾN TÍNH theo số
// topic — tức là test đo N+1 sẽ đỏ. Các hàm dưới đây gộp toàn bộ key của 1
// request thành 1 `WHERE id IN (...)`, nên số statement là hằng.
//
// Cùng luật với phần còn lại của repository: lọc `deleted = 0` thủ công,
// `ORDER BY` tường minh (M1 §3.6: `SELECT ... LIMIT` không `ORDER BY` thì
// Postgres trả thứ tự tuỳ ý), và thu thập hết row vào slice TRƯỚC khi đóng
// `rows` (luật 3 — cấm query lồng trong `rows.Next()`).

// ListStagesByPathIDs trả stage của NHIỀU path trong 1 statement.
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

// ListTopicsByStageIDs trả topic của NHIỀU stage trong 1 statement. Thứ tự
// `(stage_id, position, id)` để caller gom theo stage không phải sắp lại —
// `domain.SortedTopics` vẫn sắp lại vì layout/level phụ thuộc (position, id).
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

// ListResourcesByTopicIDs trả resource của NHIỀU topic trong 1 statement.
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

// ListMilestonesByStageIDs trả milestone của NHIỀU stage trong 1 statement.
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
