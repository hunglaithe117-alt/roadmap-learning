package graphql

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/vikstrous/dataloadgen"

	roadmapapp "langapp/internal/application/roadmap"
	srsapp "langapp/internal/application/srs"
	domainroadmap "langapp/internal/domain/roadmap"
)

// Loaders là bộ dataloader sống trong 1 request, theo mẫu của
// https://gqlgen.com/reference/dataloaders/ : 1 loader = 1 hàm `fetch` nhận
// TOÀN BỘ key của request gộp lại, còn `dataloadgen.Loader` lo phần định thời
// và tra đúng kết quả cho từng caller.
//
// Vì sao dataloadgen chứ không phải mục `dataloader:` của gqlgen (dataloaden):
// cây roadmap là 5 tầng và mỗi tầng phải trả về DANH SÁCH con đã gom sẵn (kèm
// layout + LevelState tính 1 lần cho cả stage), không phải map key→value. Mẫu
// generic của dataloaden buộc khai `dataloaden_map.go` riêng cho từng field và
// sinh 1 type key cho mỗi field — 6 field là 6 type + 6 file. Ở đây 4 loader
// viết tay là đủ, đọc được, và test đo được số SQL thật.
//
// Vì sao `WithWait` = 1ms: đủ để các resolver của 1 "wave" kếp key vào cùng
// batch, mà không thành độ trễ đáng kể. dataloadgen mặc định KHÔNG chờ (batch
// ngay lần `Load` đầu tiên) — với 1 path thì không sao, nhưng 1 query list 20
// path sẽ vỡ thành nhiều batch hơn cần.
const loaderWait = time.Millisecond

// Loaders gom 4 nhóm đọc của cây roadmap:
//
//	stageTree  pathID  → []StageView (kèm topics ĐÃ có layout + LevelState, + milestones)
//	topicRes   topicID → []Resource
//	deckRef    deckID  → *DeckRef
//	progress   pathID  → Progress
//
// Số statement của 1 query cây 5 tầng là HẰNG: 1 (path) + 1 (stages IN) +
// 1 (topics IN) + 1 (milestones IN) + 1 (resources IN) + 1 (decks IN) = 6 —
// không đổi khi cây có 1 topic hay 51 topic. Đo thật ở `roadmap_tree_test.go`.
type Loaders struct {
	stageTree *dataloadgen.Loader[int64, []roadmapapp.StageView]
	topicRes  *dataloadgen.Loader[int64, []roadmapapp.Resource]
	deckRef   *dataloadgen.Loader[int64, *roadmapapp.DeckRef]
	progress  *dataloadgen.Loader[int64, roadmapapp.Progress]

	// topicMeta là bảng bên cạnh loader: `level` + `point` + `mapPinned` của
	// từng topic, do `Stage.topics` điền khi nó nhận cây từ `stageTree`.
	//
	// Vì sao cần bảng này thay vì để `Topic.level` tự gọi loader riêng: layout
	// và LevelState phụ thuộc CẢ stage (node trước quyết định node sau), nên
	// không thể suy ra từ 1 topic lẻ — bắt buộc phải đọc cả stage. Nếu mỗi
	// topic tự gọi 1 loader thì tốn N statement, tức đúng loại N+1 mà bộ loader
	// sinh ra để chặn. Ghi 1 lần khi đã có cây rồi đọc lại là 0 statement.
	topicMeta   map[int64]topicMeta
	topicMetaMu sync.RWMutex
}

// topicMeta là phần layout của 1 topic do application service tính.
type topicMeta struct {
	Level     domainroadmap.LevelState
	Point     domainroadmap.MapPoint
	MapPinned bool
}

type loadersKey struct{}

// NewLoaders dựng loader cho 1 request. `roadmap`/`srs` là application service —
// loader KHÔNG tự viết SQL, nó gọi đúng các use case batch mà M4 thêm vào tầng
// application (`StageTreeByPathIDs`, `ResourcesByTopicIDs`, `FindDecks`,
// `ProgressByIDs`).
func NewLoaders(roadmap *roadmapapp.Service, srs *srsapp.Service) *Loaders {
	return &Loaders{
		topicMeta: map[int64]topicMeta{},
		stageTree: dataloadgen.NewMappedLoader(
			func(ctx context.Context, pathIDs []int64) (map[int64][]roadmapapp.StageView, error) {
				return roadmap.StageTreeByPathIDs(ctx, pathIDs)
			},
			dataloadgen.WithWait(loaderWait),
		),
		topicRes: dataloadgen.NewMappedLoader(
			func(ctx context.Context, topicIDs []int64) (map[int64][]roadmapapp.Resource, error) {
				return roadmap.ResourcesByTopicIDs(ctx, topicIDs)
			},
			dataloadgen.WithWait(loaderWait),
		),
		deckRef: dataloadgen.NewMappedLoader(
			func(ctx context.Context, deckIDs []int64) (map[int64]*roadmapapp.DeckRef, error) {
				infos, err := srs.FindDecks(ctx, deckIDs)
				if err != nil {
					return nil, err
				}
				out := make(map[int64]*roadmapapp.DeckRef, len(infos))
				for id, info := range infos {
					if !info.Exists {
						continue
					}
					out[id] = &roadmapapp.DeckRef{ID: id, Name: info.Name, Lang: info.Lang}
				}
				return out, nil
			},
			dataloadgen.WithWait(loaderWait),
		),
		progress: dataloadgen.NewMappedLoader(
			func(ctx context.Context, pathIDs []int64) (map[int64]roadmapapp.Progress, error) {
				// `ProgressByPathIDs` gộp TẤT CẢ path trong 1 lệnh `stages IN` +
				// 1 lệnh `topics IN`, nên 20 path trong list vẫn chỉ 2 statement.
				return roadmap.ProgressByPathIDs(ctx, pathIDs)
			},
			dataloadgen.WithWait(loaderWait),
		),
	}
}

// LoadStages trả stage của 1 path. Path không có stage (hoặc đã xoá mềm) trả
// slice rỗng chứ không phải `ErrNotFound`: "không có con" là kết quả hợp lệ, còn
// lỗi hệ thống mới là error. Nếu bỏ nhánh này, client hỏi `stages` của path vừa
// tạo sẽ nhận lỗi thay vì `[]`.
func (l *Loaders) LoadStages(ctx context.Context, pathID int64) ([]roadmapapp.StageView, error) {
	out, err := l.stageTree.Load(ctx, pathID)
	if errors.Is(err, dataloadgen.ErrNotFound) {
		return []roadmapapp.StageView{}, nil
	}
	return out, err
}

// LoadResources trả resource của 1 topic; topic không có resource thì `[]`.
func (l *Loaders) LoadResources(ctx context.Context, topicID int64) ([]roadmapapp.Resource, error) {
	out, err := l.topicRes.Load(ctx, topicID)
	if errors.Is(err, dataloadgen.ErrNotFound) {
		return []roadmapapp.Resource{}, nil
	}
	return out, err
}

// LoadDeck trả deck đã gắn của stage. Deck đã xoá mềm sau khi gắn
// (`ON DELETE SET NULL` chỉ xử lý DELETE cứng) trả nil — đúng như
// `roadmap.Service.GetPath` đã làm, và `deck` nullable trong schema.
func (l *Loaders) LoadDeck(ctx context.Context, deckID int64) (*roadmapapp.DeckRef, error) {
	out, err := l.deckRef.Load(ctx, deckID)
	if errors.Is(err, dataloadgen.ErrNotFound) {
		return nil, nil
	}
	return out, err
}

// LoadProgress trả tiến độ 1 path. Path không có stage trả số 0 (không phải
// lỗi): path mới tạo có `percent = 0` là con số đúng.
func (l *Loaders) LoadProgress(ctx context.Context, pathID int64) (roadmapapp.Progress, error) {
	out, err := l.progress.Load(ctx, pathID)
	if errors.Is(err, dataloadgen.ErrNotFound) {
		return roadmapapp.Progress{}, nil
	}
	return out, err
}

// rememberTopic ghi layout của các topic trong 1 stage vào bảng bên cạnh, để
// `Topic.level` / `Topic.point` đọc lại được mà không query thêm.
func (l *Loaders) rememberTopic(views []roadmapapp.TopicView) {
	l.topicMetaMu.Lock()
	defer l.topicMetaMu.Unlock()
	for _, tv := range views {
		l.topicMeta[tv.ID] = topicMeta{
			Level: tv.Level, Point: tv.Point, MapPinned: tv.MapPinned,
		}
	}
}

// TopicMeta trả layout đã nhớ của 1 topic.
//
// Chưa có trong bảng (client hỏi `topic(id:)` rồi `level` mà chưa đi qua
// `Stage.topics`) trả `LevelCurrent` + điểm (0,0) + `mapPinned=false` — tức là
// "chưa biết", hiển thị tạm ở giữa bản đồ. Không im lặng trả giá trị sai lệch
// vị trí thật: resolver `Query.topic` luôn đi qua `PrimeStageTree` nên bảng
// luôn có dữ liệu trên đường đó.
func (l *Loaders) TopicMeta(topicID int64) topicMeta {
	l.topicMetaMu.RLock()
	defer l.topicMetaMu.RUnlock()
	if m, ok := l.topicMeta[topicID]; ok {
		return m
	}
	return topicMeta{
		Level: domainroadmap.LevelCurrent,
		Point: domainroadmap.MapPoint{},
	}
}

// Middleware bọc mỗi request trong 1 bộ loader MỚI. Bắt buộc: cache của
// dataloadgen sống suốt request đó, dùng chung giữa các request sẽ trả dữ liệu
// cũ cho request mới.
func Middleware(roadmap *roadmapapp.Service, srs *srsapp.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loaders := NewLoaders(roadmap, srs)
		ctx := context.WithValue(r.Context(), loadersKey{}, loaders)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LoadersFrom trả bộ loader của request hiện tại.
//
// Resolver gọi hàm này. Nếu context không có loader (test gọi resolver trực
// tiếp không qua middleware) thì dựng loader tạm: dữ liệu ĐÚNG, chỉ mất
// batching. Nhờ vậy test resolver unit không cần dựng HTTP server, còn test
// N+1 (đi qua `graph/client`) vẫn đo được batching thật.
func LoadersFrom(ctx context.Context, roadmap *roadmapapp.Service, srs *srsapp.Service) *Loaders {
	if l, ok := ctx.Value(loadersKey{}).(*Loaders); ok {
		return l
	}
	return NewLoaders(roadmap, srs)
}
