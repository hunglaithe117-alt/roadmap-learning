// Package roadmap chứa tầng orchestration của bounded context lộ trình học:
// use case CRUD cây 5 tầng + progress, gọi domain để tính toán và gọi
// repository (định nghĩa ở đây) để ghi.
//
// Lớp này KHÔNG import driver DB và KHÔNG import tầng hạ tầng (infrastructure) —
// grep luật DDD ở application/ phải ra 0 file (STACK-V2-PLAN §2). Mọi truy cập
// DB đi qua interface trong file này, hiện thực nằm ở tầng infrastructure.
package roadmap

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Tx là handle transaction do UnitOfWork tạo và đưa cho repository.
//
// Kiểu `any` cố ý: application không biết driver, còn infrastructure tự ép
// kiểu về *gorm.DB. Đổi sang interface có method sẽ buộc infrastructure phải
// implement marker method của package này — tức là hướng phụ thuộc đảo ngược.
type Tx = any

// UnitOfWork chạy 1 khối ghi trong 1 transaction. Callback trả lỗi ⇒ rollback
// toàn bộ. Đây là ranh giới transaction duy nhất của roadmap: cascade xoá
// mềm cây 4 tầng và RecordReview mà không để lại trạng thái nửa vời.
type UnitOfWork interface {
	Do(ctx context.Context, fn func(tx Tx) error) error
}

// DeckReader là port sang bounded context `srs` (STACK-V2-PLAN §2: roadmap phụ
// thuộc srs để kiểm tra `deck_id` tồn tại). 3 nơi dùng nó:
//   - CreateStage/UpdateStage gắn deck (A1): phải biết deck có tồn tại và
//     `deck.lang` có khớp `path.language` không.
//   - GetPath: trả `deck_name`/`deck_lang` cho nút "vào /review" của M6 —
//     client hiện tên deck thật thay vì id.
//
// Cố ý gộp cả 3 thông tin trong 1 lần gọi `Find` — chúng cùng 1 row, tách ra
// là 3 query cho 1 stage.
type DeckReader interface {
	// Find trả deck theo id. Deck không tồn tại (hoặc đã xoá mềm) trả
	// (DeckInfo{}, nil) — không phải lỗi: "không có" là kết quả hợp lệ, còn
	// lỗi hệ thống mới là error.
	Find(ctx context.Context, id int64) (DeckInfo, error)
}

// DeckInfo là thông tin deck mà context roadmap cần: tồn tại, tên, ngôn ngữ.
type DeckInfo struct {
	Exists bool
	Name   string
	Lang   string
}

// DeckRef là deck đã gắn của 1 stage, đẩy vào `StageView` cho M6 (nút
// "vào /review"). `StageView.Deck == nil` nghĩa là stage chưa gắn deck.
type DeckRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Lang string `json:"lang"`
}

// Repository là toàn bộ truy cập DB của context roadmap mà tầng application
// cần. Mọi list đã lọc `deleted = 0` trong chính repository (không để caller
// quên — app v1 quên chỗ này và đó là nguồn bug đã xảy ra).
//
// MỌI METHOD GHI đều nhận `Tx` làm tham số đầu: đó là cách duy nhất để
// repository biết phải ghi vào transaction nào. Method đọc không nhận vì đọc
// ngoài transaction vẫn đúng (bọc vào chỉ tốn connection).
type Repository interface {
	// Paths
	ListPaths(ctx context.Context) ([]Path, error)
	PathBySlug(ctx context.Context, slug string) (Path, error)
	PathByID(ctx context.Context, id int64) (Path, error)
	CreatePath(ctx context.Context, tx Tx, p *Path) error
	UpdatePath(ctx context.Context, tx Tx, p *Path) error
	SoftDeletePath(ctx context.Context, tx Tx, id int64) error

	// Stages
	ListStages(ctx context.Context, pathID int64) ([]Stage, error)
	// Các method `*By*IDs` là BATCH READ cho dataloader GraphQL (M4): gom
	// toàn bộ key của 1 request thành 1 `WHERE id IN (...)`.
	//
	// Vì sao bắt buộc: cây roadmap đọc bằng GraphQL gọi `ListTopics` /
	// `ListResources` 1 lần cho MỖI node. Seed có 51 topic + 263 resource nên
	// 1 request tốn 315 statement, và con số đó tăng TUYẾN TÍNH theo số topic —
	// đúng loại N+1 mà `extension.FixedComplexityLimit` không chặn được ( nó
	// chặn theo độ sâu, không theo số node).
	//
	// Các method này chỉ đọc, không mở transaction: `GetPath` (đường 1 request)
	// và `StageTreeByPathIDs` (đường dataloader) dùng CHUNG phần trang trí
	// topic, nên số statement của cả 2 là bằng nhau.
	ListStagesByPathIDs(ctx context.Context, pathIDs []int64) ([]Stage, error)
	ListTopicsByStageIDs(ctx context.Context, stageIDs []int64) ([]Topic, error)
	ListResourcesByTopicIDs(ctx context.Context, topicIDs []int64) ([]Resource, error)
	ListMilestonesByStageIDs(ctx context.Context, stageIDs []int64) ([]Milestone, error)
	StageByID(ctx context.Context, id int64) (Stage, error)
	CreateStage(ctx context.Context, tx Tx, s *Stage) error
	UpdateStage(ctx context.Context, tx Tx, s *Stage) error
	SoftDeleteStage(ctx context.Context, tx Tx, id int64) error
	// UpdateStageStatus ghi status/status_note/completed_at rồi LÀM MỚI `s`
	// bằng row đọc lại TRONG CÙNG transaction. Nhận con trỏ (thay vì
	// (id, status, note) + đọc lại ở tầng trên) vì caller đã có sẵn struct đầy
	// đủ; việc đọc lại phải nằm trong tx, nếu không sẽ trả về row CŨ.
	UpdateStageStatus(ctx context.Context, tx Tx, s *Stage) error
	MaxStagePosition(ctx context.Context, pathID int64) (int, error)

	// Topics
	ListTopics(ctx context.Context, stageID int64) ([]Topic, error)
	TopicByID(ctx context.Context, id int64) (Topic, error)
	CreateTopic(ctx context.Context, tx Tx, t *Topic) error
	UpdateTopic(ctx context.Context, tx Tx, t *Topic) error
	SoftDeleteTopic(ctx context.Context, tx Tx, id int64) error
	// UpdateTopicStatus xem UpdateStageStatus.
	UpdateTopicStatus(ctx context.Context, tx Tx, t *Topic) error
	MaxTopicPosition(ctx context.Context, stageID int64) (int, error)

	// Resources
	ListResources(ctx context.Context, topicID int64) ([]Resource, error)
	ResourceByID(ctx context.Context, id int64) (Resource, error)
	CreateResource(ctx context.Context, tx Tx, r *Resource) error
	UpdateResource(ctx context.Context, tx Tx, r *Resource) error
	SoftDeleteResource(ctx context.Context, tx Tx, id int64) error
	MaxResourcePosition(ctx context.Context, topicID int64) (int, error)

	// Milestones
	ListMilestones(ctx context.Context, stageID int64) ([]Milestone, error)
	MilestoneByID(ctx context.Context, id int64) (Milestone, error)
	CreateMilestone(ctx context.Context, tx Tx, m *Milestone) error
	UpdateMilestone(ctx context.Context, tx Tx, m *Milestone) error
	SoftDeleteMilestone(ctx context.Context, tx Tx, id int64) error
	MaxMilestonePosition(ctx context.Context, stageID int64) (int, error)

	// Bookmarks — kho link độc lập, KHÔNG thuộc cây roadmap (amendment A1).
	// `ListBookmarks` nhận filter rỗng = xem tất cả, và sắp theo `id` tăng dần:
	// kho link là danh sách user tự thêm, thứ tự thời gian thêm là trực giác,
	// còn `updated_at` thì bị trigger chạm mỗi lần sửa nên nhảy vị trí.
	ListBookmarks(ctx context.Context, filter BookmarkFilter) ([]Bookmark, error)
	BookmarkByID(ctx context.Context, id int64) (Bookmark, error)
	CreateBookmark(ctx context.Context, tx Tx, b *Bookmark) error
	UpdateBookmark(ctx context.Context, tx Tx, b *Bookmark) error
	SoftDeleteBookmark(ctx context.Context, tx Tx, id int64) error
}

// BookmarkFilter là bộ lọc của `ListBookmarks`. Cả 2 trường đều rỗng = không
// lọc — tầng application đã chuẩn hoá (status qua whitelist, tag qua
// `domain.NormalizeTagFilter`) trước khi gọi repository, nên repository chỉ
// cần dựng điều kiện SQL, không hiểu ngữ nghĩa gì.
type BookmarkFilter struct {
	// Status rỗng = mọi trạng thái.
	Status string
	// Tag rỗng = không lọc tag. Khớp theo PHẦN TỬ của CSV, không phải chuỗi con:
	// tag `a` không được khớp bookmark mang tag `ab` (xem `domain.TagsContain`).
	Tag string
}

// Path là row `roadmap_paths` đã đọc từ DB — mirror của entity domain, dùng
// để repository không phải import domain (giữ hướng phụ thuộc 1 chiều
// infrastructure → application → domain chứ không phải chéo).
type Path struct {
	ID        int64
	Slug      string
	Language  string
	Title     string
	Overview  string
	IsBuiltin bool
	CreatedAt string
	GUID      string
	UpdatedAt string
	Deleted   int
}

type Stage struct {
	ID            int64
	PathID        int64
	Slug          string
	Title         string
	Goal          string
	Position      int
	DurationWeeks int
	Status        Status
	StatusNote    string
	CompletedAt   *string
	DeckID        *int64
	Terrain       string
	Direction     string
	CreatedAt     string
	GUID          string
	UpdatedAt     string
	Deleted       int
}

type Topic struct {
	ID          int64
	StageID     int64
	Title       string
	Why         string
	Activities  string
	Position    int
	Status      Status
	StatusNote  string
	CompletedAt *string
	IsOptional  int
	MapX        *float64
	MapY        *float64
	CreatedAt   string
	GUID        string
	UpdatedAt   string
	Deleted     int
}

type Resource struct {
	ID        int64
	TopicID   int64
	Title     string
	URL       *string
	Kind      string
	Note      string
	Position  int
	CreatedAt string
	GUID      string
	UpdatedAt string
	Deleted   int
}

type Milestone struct {
	ID        int64
	StageID   int64
	Text      string
	Position  int
	CreatedAt string
	GUID      string
	UpdatedAt string
	Deleted   int
}

// Bookmark là row `roadmap_bookmarks` đã đọc từ DB.
//
// Cột `tags` là CSV, không phải bảng con — 1 bookmark chỉ 1-3 tag, tách bảng là
// chi phí mà không đổi được truy vấn nào. `EncodeTags`/`DecodeTags` ở
// `domain/roadmap` là nơi duy nhất định nghĩa hình dạng CSV đó.
//
// `URL` là *string vì cột NULLABLE và "bookmark không có link" là trạng thái
// hợp lệ (ghi chú tự nhớ về bài tập chẳng có link nào).
type Bookmark struct {
	ID        int64
	Title     string
	URL       *string
	Note      string
	Tags      string
	Status    string
	CreatedAt string
	GUID      string
	UpdatedAt string
	Deleted   int
}

// Status là trạng thái node — trùng tập hằng với domain/roadmap nhưng khai
// báo cục bộ để application không bắt buộc import domain cho mọi call site.
// CHECK ở DB là backstop chung cho cả 2 nơi.
type Status = string

const (
	StatusNotStarted Status = "not_started"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
	StatusSkipped    Status = "skipped"
)

// AllStatuses là hợp đồng đóng băng 4 giá trị.
var AllStatuses = []Status{StatusNotStarted, StatusInProgress, StatusDone, StatusSkipped}

// ValidStatus báo status có thuộc tập 4 hằng không.
func ValidStatus(s Status) bool {
	for _, v := range AllStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// NowFunc là nguồn thời gian, inject để test được và để 1 request dùng chung
// 1 mốc (tránh completed_at lệch vài mili giây giữa 2 bản ghi).
type NowFunc func() time.Time

// Clock mặc định: UTC. Mọi mốc ghi vào DB đều là UTC RFC3339.
func Clock() time.Time { return time.Now().UTC() }

// NewGUID sinh guid cho row mới.
//
// KHÔNG BAO GIỜ sinh guid rỗng: `ux_reviews_guid` / `ux_roadmap_*_guid` là
// UNIQUE trên cột `TEXT NOT NULL DEFAULT ”`, nên 2 row cùng guid rỗng là
// đụng nhau và insert thứ 2 fail (ghi chú M1 remediation F-guid). `guid` là
// natural key cho peer merge, sinh rỗng cũng làm merge coi 2 row khác là
// cùng 1 thứ.
func NewGUID() string { return uuid.NewString() }
