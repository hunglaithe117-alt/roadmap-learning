// Package sync chứa bounded context đồng bộ peer-to-peer bằng file backup
// (v1: ATTACH snapshot SQLite của máy kia rồi merge trong 1 transaction).
//
// Ở đây CHỈ có quy tắc thuần — PeerSnapshot / Conflict / MergeDecision.
// Phần đọc-ghi (ATTACH, INSERT, UPDATE) thuộc infrastructure M3; context này
// không import package driver DB nào.
package sync

import "time"

// SchemaVersion là version schema hiện tại. 2 máy lệch version → từ chối
// merge (không fallback): dữ liệu sai kiểu hỏng còn tệ hơn không đồng bộ.
const SchemaVersion = 4

// Table là tên bảng trong snapshot peer.
type Table string

const (
	TableDecks         Table = "decks"
	TableCards         Table = "cards"
	TableReviews       Table = "reviews"
	TableNotes         Table = "notes"
	TableSyncMeta      Table = "sync_meta"
	TableSyncConflicts Table = "sync_conflicts"

	// Cây roadmap (M3 đưa vào danh sách merge — plan M7 ghi đây là lỗ hổng v1:
	// 2 máy lệch roadmap không bao giờ hội tụ).
	TableRoadmapPaths      Table = "roadmap_paths"
	TableRoadmapStages     Table = "roadmap_stages"
	TableRoadmapMilestones Table = "roadmap_milestones"
	TableRoadmapTopics     Table = "roadmap_topics"
	TableRoadmapResources  Table = "roadmap_resources"
	TableRoadmapBookmarks  Table = "roadmap_bookmarks"
)

// Row là 1 bản ghi đã chuẩn hóa từ snapshot peer, đã resolve FK bằng GUID
// (v1 resolve card_id qua guid vì ID 2 máy không khớp nhau).
type Row struct {
	Table Table
	GUID  string
	// DeckGUID là GUID của deck cha (chỉ cards/notes có).
	DeckGUID string
	// CardGUID là GUID của card cha (chỉ notes/reviews có).
	CardGUID string
	// ParentGUID là GUID của cha theo bảng: roadmap_stages → paths,
	// roadmap_milestones/roadmap_topics → stages, roadmap_resources → topics.
	// Tách khỏi DeckGUID vì cùng 1 trường mang 2 ý nghĩa sẽ dễ gán nhầm khi
	// đọc code (đã xảy ra 1 lần khi viết merge lần đầu).
	ParentGUID string
	// UpdatedAt là mốc LWW; rỗng thì lùi về CreatedAt.
	UpdatedAt string
	CreatedAt string
	// Deleted là tombstone 0|1 — KHÔNG phải bool vì merge so sánh nguyên vẹn
	// giá trị lưu trong DB.
	Deleted int
	// Values là payload còn lại dạng chuỗi (đã quyết định merge ở tầng trên).
	Values map[string]string
}

// EffectiveTS là mốc LWW của row: UpdatedAt nếu có, không thì CreatedAt.
// row với cả 2 rỗng có mốc rỗng — sẽ thua mọi row có mốc.
func (r Row) EffectiveTS() string {
	if r.UpdatedAt != "" {
		return r.UpdatedAt
	}
	return r.CreatedAt
}

// IsTombstone là row đã xóa mềm.
func (r Row) IsTombstone() bool { return r.Deleted == 1 }

// PeerSnapshot là 1 bản backup của máy khác.
type PeerSnapshot struct {
	SchemaVersion int
	// LastSync là mốc lần sync trước ở máy này, RFC3339. Rỗng = chưa sync
	// lần nào, khi đó mọi xung đột đều là "first contact" và không ghi log
	// conflict (tránh spam 2 lần đầu).
	LastSync string
	// Rows là toàn bộ dữ liệu peer, đã đọc và chuẩn hóa.
	Rows []Row
	// MaxUpdatedAt là mốc lớn nhất trong snapshot, dùng phát hiện lệch đồng
	// hồ (xem DetectClockSkew).
	MaxUpdatedAt string
}

// Conflict là 1 lần ghi log xung đột merge để user xem lại ở
// GET /api/sync/conflicts.
type Conflict struct {
	Table Table
	GUID  string
	// Winner là "local" hoặc "incoming".
	Winner string
	Detail string
	At     string
}

// Winner của một quyết định merge.
const (
	WinnerLocal    = "local"
	WinnerIncoming = "incoming"
)

// ClockSkewThreshold là ngưỡng lệch đồng hồ để cảnh báo. Lệch lớn hơn thì
// LWW chọn nhầm — phải nói với user thay vì im lặng.
const ClockSkewThreshold = 24 * time.Hour
