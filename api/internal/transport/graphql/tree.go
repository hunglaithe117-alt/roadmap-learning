package graphql

import (
	"context"

	roadmapapp "langapp/internal/application/roadmap"
)

// stageTopics loads stages for a path and filters by stage ID, caching topic layout metadata.
func stageTopics(ctx context.Context, loaders *Loaders, pathID, stageID int64) ([]roadmapapp.StageView, error) {
	all, err := loaders.LoadStages(ctx, pathID)
	if err != nil {
		return nil, err
	}
	var out []roadmapapp.StageView
	for _, sv := range all {
		if sv.ID == stageID {
			loaders.rememberTopic(sv.Topics)
			out = append(out, sv)
		}
	}
	return out, nil
}
