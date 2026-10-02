package graphql

import (
	"context"

	roadmapapp "langapp/internal/application/roadmap"
)

// stageTopics là đường dùng chung cho `Stage.topics` và `Stage.milestones`.
//
// Nó nạp cây theo PATH (khoá của `stageTree` loader) rồi lọc stage của mình —
// nhờ vậy `Stage.topics` không cần loader thứ 5 theo stageID: khi query hỏi
// `stages { topics }` thì `Path.stages` đã nạp cả path rồi, mọi lần gọi sau vào
// cache, 0 statement.
//
// Side effect cố ý: ghi layout từng topic vào bảng bên cạnh để `Topic.level` /
// `Topic.point` đọc lại được mà không query thêm.
//
// File này TÁCH riêng khỏi `roadmap.resolvers.go` vì gqlgen ghi đè toàn bộ file
// `*.resolvers.go` sinh ra — hàm viết tay nằm trong đó sẽ biến mất mỗi lần
// `go generate`.
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
