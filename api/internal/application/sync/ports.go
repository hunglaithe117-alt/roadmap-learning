// Package sync chứa tầng orchestration của bounded context đồng bộ
// peer-to-peer: nạp snapshot của máy kia rồi merge vào DB local trong MỘT
// transaction.
//
// ĐÂY LÀ CONTEXT SÂU NHẤT của kiến trúc: nó là context DUY NHẤT ghi vào bảng
// của mọi context khác (decks/cards/reviews = srs, roadmap_* = roadmap,
// notes = practice). Vì vậy nó cần `Merge*` riêng cho từng bảng thay vì dùng
// lại `Update*` của repository khác — xem giải thích ở `Repository`.
//
// Lớp này KHÔNG import driver DB và KHÔNG import tầng hạ tầng
// (STACK-V2-PLAN §2).
package sync

import (
	"context"
	"time"

	domain "langapp/internal/domain/sync"
)

// Tx là handle transaction do UnitOfWork tạo và đưa cho repository.
//
// Kiểu `any` cố ý: application không biết driver, còn infrastructure tự ép
// kiểu về *gorm.DB.
type Tx = any

// UnitOfWork là RANH GIỚI QUAN TRỌNG NHẤT của context này. Merge ghi hàng
// chục bảng; lỗi giữa chừng (FK thiếu, CHECK, mất kết nối) mà để lại nửa
// snapshot thì DB local không còn hội tụ với bất kỳ máy nào — tệ hơn hẳn việc
// merge thất bại trả lỗi cho user thử lại.
type UnitOfWork interface {
	Do(ctx context.Context, fn func(tx Tx) error) error
}

// Repository là toàn bộ truy cập DB của merge.
//
// VÌ SAO CÓ `Merge*` RIÊNG thay vì dùng `Update*` của repository srs/roadmap:
// oracle đã đo được `Omit("updated_at")` của GORM loại `updated_at` khỏi tập
// `SET` **kể cả khi map có key tường minh**. Nếu merge gọi lại
// `srs.Repository.UpdateCard` thì mốc LWW từ peer sẽ bị trigger ghi đè bằng
// giờ máy local → lần sync sau 2 máy so sánh sai và một bên ghi đè bên kia
// mãi. Repository merge vì vậy ghi thẳng, set `updated_at` tường minh, và
// KHÔNG dùng `Omit`.
//
// MỌI method `*Rows` ĐỌC CŨNG nhận `tx` (truyền `nil` = ngoài transaction).
// Lý do: merge ghi `decks` rồi mới cần map `guid→id` của deck để ghi `cards`.
// Đọc ngoài transaction sẽ KHÔNG thấy row vừa insert (chưa commit) và mọi card
// của peer bị bỏ âm thầm. Method đọc thuần (`SchemaVersion`, `LastSyncAt`)
// không cần và vẫn không nhận.
type Repository interface {
	// SchemaVersion đọc `MAX(version)` của `schema_migrations` (bảng app v1
	// đọc; M7 chuyển sang `goose_db_version`). 0 khi bảng rỗng/thiếu.
	SchemaVersion(ctx context.Context) (int, error)
	// PeerSchemaVersion đọc version của snapshot đã nạp.
	PeerSchemaVersion(ctx context.Context) (int, error)
	// LastSyncAt đọc `sync_meta.last_sync_at` ("" khi chưa sync lần nào).
	LastSyncAt(ctx context.Context) (string, error)
	// SetLastSyncAt ghi mốc sync vừa xong.
	SetLastSyncAt(ctx context.Context, tx Tx, now string) error

	// ── LWW (deck + card) ──────────────────────────────────────────────────
	// DeckRows trả mọi deck local (kể cả tombstone) theo guid.
	DeckRows(ctx context.Context, tx Tx) (map[string]DeckRow, error)
	// UpsertDeck chèn (found=false) hoặc ghi đè (found=true) 1 deck với
	// `updated_at` TƯỜNG MINH. Trả id sau ghi.
	UpsertDeck(ctx context.Context, tx Tx, d DeckRow, found bool) (int64, error)
	// CardRows trả mọi card local (kể cả tombstone) theo guid.
	CardRows(ctx context.Context, tx Tx) (map[string]CardRow, error)
	// TombstoneCard tra card đang xoá mềm theo (deck_id, front) — dùng khi
	// peer RE-CREATE 1 thẻ mà local đã xoá: hồi sinh chính row đó thay vì
	// tạo row mới lệch guid (giữ `id` để `reviews.card_id` còn trỏ đúng).
	//
	// NHẬN `tx`: hàm này được gọi BÊN TRONG `uow.Do`, sau khi merge đã ghi
	// card. Đọc ngoài transaction thì Postgres KHÔNG thấy row vừa ghi.
	TombstoneCard(ctx context.Context, tx Tx, deckID int64, front string) (int64, string, bool, error)
	// LiveCardGUIDByFront tra guid của thẻ đang SỐNG cùng front — dùng khi
	// insert đụng `ux_cards_deck_front` vì 2 máy cùng tạo thẻ tay.
	//
	// NHẬN `tx`: lý do như `TombstoneCard`.
	LiveCardGUIDByFront(ctx context.Context, tx Tx, deckID int64, front string) (int64, string, bool, error)
	// UpsertCard như UpsertDeck cho bảng `cards`. Bool thứ hai là `dupKept`:
	// insert đụng `ux_cards_deck_front` (2 máy cùng tạo 1 thẻ tay) → đã giữ
	// bản local và map guid của peer về cùng id; caller ghi log xung đột.
	// Trả về id sau ghi.
	UpsertCard(ctx context.Context, tx Tx, c CardRow, found bool) (id int64, dupKept bool, err error)

	// ── LWW (roadmap_*) ────────────────────────────────────────────────────
	// Các cặp Rows/Upsert dưới đây giữ CÙNG luật LWW + tombstone + set
	// updated_at tường minh như deck/card. Không dùng lại
	// `roadmap.Repository.Update*` vì lý do `Omit("updated_at")` nêu trên.
	//
	// `skipped` = INSERT bị `ON CONFLICT DO NOTHING` bỏ qua (id trả về 0):
	// dữ liệu của peer **KHÔNG** vào DB. Caller PHẢI ghi `sync_conflicts` thay
	// vì đếm vào `merged` — báo cáo "đã merge 1 path" trong khi thực tế bị bỏ
	// im lặng là loại bug khiến user tin rằng đã sync xong.
	PathRows(ctx context.Context, tx Tx) (map[string]PathRow, error)
	UpsertPath(ctx context.Context, tx Tx, p PathRow, found bool) (id int64, skipped bool, err error)
	StageRows(ctx context.Context, tx Tx) (map[string]StageRow, error)
	UpsertStage(ctx context.Context, tx Tx, s StageRow, found bool) (id int64, skipped bool, err error)
	MilestoneRows(ctx context.Context, tx Tx) (map[string]MilestoneRow, error)
	UpsertMilestone(ctx context.Context, tx Tx, m MilestoneRow, found bool) (id int64, skipped bool, err error)
	TopicRows(ctx context.Context, tx Tx) (map[string]TopicRow, error)
	UpsertTopic(ctx context.Context, tx Tx, tp TopicRow, found bool) (id int64, skipped bool, err error)
	ResourceRows(ctx context.Context, tx Tx) (map[string]ResourceRow, error)
	UpsertResource(ctx context.Context, tx Tx, res ResourceRow, found bool) (id int64, skipped bool, err error)
	BookmarkRows(ctx context.Context, tx Tx) (map[string]BookmarkRow, error)
	UpsertBookmark(ctx context.Context, tx Tx, b BookmarkRow, found bool) (id int64, skipped bool, err error)

	// ── Append-only (reviews + notes) ──────────────────────────────────────
	// ReviewGUIDs trả tập guid review đã có (dedupe khi union).
	// NHẬN `tx`: đọc trong transaction để thấy review vừa append ở cùng lô.
	ReviewGUIDs(ctx context.Context, tx Tx) (map[string]bool, error)
	// AppendReview chèn 1 review (ON CONFLICT theo guid DO NOTHING).
	AppendReview(ctx context.Context, tx Tx, r ReviewRow) (bool, error)
	// ReviewsOfCard đọc lịch sử ôn của 1 thẻ theo (reviewed_at, id) để
	// replay — thứ tự phải xác định để cả 2 máy cho cùng kết quả. Đọc trong
	// transaction vì phải thấy review vừa append.
	ReviewsOfCard(ctx context.Context, tx Tx, cardID int64) ([]ReplayReview, error)
	// ReplayCard ghi reps/lapses/stability/difficulty/due_at/state tính lại
	// từ lịch sử ôn. `updated_at` tường minh bằng tham số `now` (mốc merge).
	ReplayCard(ctx context.Context, tx Tx, cardID int64, r ReplayResult, now string) error
	// NoteGUIDs trả tập guid note đã có. NHẬN `tx`: lý do như `ReviewGUIDs`.
	NoteGUIDs(ctx context.Context, tx Tx) (map[string]bool, error)
	// AppendNote chèn 1 note (ON CONFLICT theo guid DO NOTHING).
	AppendNote(ctx context.Context, tx Tx, n NoteRow) (bool, error)

	// ── Nhật ký ────────────────────────────────────────────────────────────
	// LogConflict ghi 1 dòng `sync_conflicts` để user xem lại.
	LogConflict(ctx context.Context, tx Tx, c domain.Conflict) error
	// ListConflicts đọc log xung đột, mới nhất trước (read-only cho UI).
	ListConflicts(ctx context.Context, limit int) ([]domain.Conflict, error)
	// CountConflicts đếm tổng số dòng log (status endpoint).
	CountConflicts(ctx context.Context) (int, error)
}

// SnapshotLoader nạp snapshot của máy peer. Ở v1 đây là `ATTACH` file SQLite;
// ở Postgres (M3) test dựng 2 schema trong cùng 1 database, còn M7 sẽ nạp từ
// file `pg_restore` — cả 2 đều qua interface này, merge không đổi.
type SnapshotLoader interface {
	Load(ctx context.Context) (domain.PeerSnapshot, error)
}

// BackupPort là interface CHƯA HIỆN THỰC cho `pg_dump` / `pg_restore`.
//
// v1 dùng `VACUUM INTO` (SQLite) — **không tồn tại ở Postgres**. Tương đương
// là gọi subprocess `pg_dump` / `pg_restore`, thuộc M7 (plan §4.6). Khai ở
// đây để chỗ gọi đã có hợp đồng, KHÔNG implement sớm: subprocess trong
// application layer sẽ phá luật "tầng này không biết gì ngoài DB interface".
type BackupPort interface {
	// Dump xuất 1 snapshot nhất quán (tương đương `VACUUM INTO`).
	Dump(ctx context.Context) (path string, err error)
	// Restore ghi đè từ snapshot (KHÔNG phải merge — khác hẳn Merge).
	Restore(ctx context.Context, path string) error
}

// Row là DTO chung cho mọi bảng merge: khoá tự nhiên `GUID`, mốc LWW, tombstone
// và payload. Không dùng chung struct `domain/sync.Row` vì bảng roadmap có
// cột riêng (map_x/map_y, is_optional, completed_at…) và cột NULL cần phân
// biệt với chuỗi rỗng — ép về `map[string]string` sẽ mất thông tin đó.
type DeckRow struct {
	// ID là id số LOCAL, chỉ dùng khi `found = true` (update). Không bao giờ lấy
	// từ snapshot: id 2 máy trùng nhau.
	ID        int64
	GUID      string
	Name      string
	Lang      string
	CreatedAt string
	UpdatedAt string
	Deleted   int
}

type CardRow struct {
	// ID là id số LOCAL, chỉ dùng khi `found = true` (update). Không bao giờ lấy
	// từ snapshot: id 2 máy trùng nhau.
	ID   int64
	GUID string
	// DeckGUID là GUID của deck cha trong snapshot; FK `deck_id` phải remap
	// qua guid (id số 2 máy không khớp nhau). Tầng application resolve sang
	// `DeckID` trước khi gọi repository.
	DeckGUID   string
	DeckID     *int64
	Front      string
	Back       string
	Pinyin     string
	DueAt      string
	Stability  float64
	Difficulty float64
	Reps       int
	Lapses     int
	State      string
	CreatedAt  string
	Tone       *string
	IPA        *string
	Stress     *string
	AudioURL   *string
	UpdatedAt  string
	Deleted    int
}

type PathRow struct {
	// ID là id số LOCAL, chỉ dùng khi `found = true` (update). Không bao giờ lấy
	// từ snapshot: id 2 máy trùng nhau.
	ID        int64
	GUID      string
	Slug      string
	Language  string
	Title     string
	Overview  string
	IsBuiltin int
	CreatedAt string
	UpdatedAt string
	Deleted   int
}

type StageRow struct {
	// ID là id số LOCAL, chỉ dùng khi `found = true` (update). Không bao giờ lấy
	// từ snapshot: id 2 máy trùng nhau.
	ID            int64
	GUID          string
	PathGUID      string
	Slug          string
	Title         string
	Goal          string
	Position      int
	DurationWeeks int
	Status        string
	StatusNote    string
	CompletedAt   *string
	// DeckGUID là GUID của deck mà stage gắn (amendment A1: bấm stage nhảy
	// thẳng `/review`). Con trỏ 3 trạng thái, KHÔNG dùng chuỗi rỗng để nói
	// "không có":
	//
	//	nil  → incoming KHÔNG mang thông tin deck (snapshot cũ / loader thiếu
	//	       cột). Tầng hạ tầng phải KHÔNG đụng cột `deck_id`.
	//	&""  → incoming nói rõ stage này `deck_id IS NULL` ⇒ ghi NULL.
	//	&g   → gắn deck guid g.
	//
	// Gộp 2 khả năng đầu thành `string` rỗng là nguyên nhân B1: merge xoá
	// `deck_id` của mọi stage, âm thầm chết feature A1 sau sync đầu tiên.
	DeckGUID  *string
	Terrain   string
	Direction string
	CreatedAt string
	UpdatedAt string
	Deleted   int
}

type MilestoneRow struct {
	// ID là id số LOCAL, chỉ dùng khi `found = true` (update). Không bao giờ lấy
	// từ snapshot: id 2 máy trùng nhau.
	ID        int64
	GUID      string
	StageGUID string
	Text      string
	Position  int
	CreatedAt string
	UpdatedAt string
	Deleted   int
}

type TopicRow struct {
	// ID là id số LOCAL, chỉ dùng khi `found = true` (update). Không bao giờ lấy
	// từ snapshot: id 2 máy trùng nhau.
	ID          int64
	GUID        string
	StageGUID   string
	Title       string
	Why         string
	Activities  string
	Position    int
	Status      string
	StatusNote  string
	CompletedAt *string
	IsOptional  int
	MapX        *float64
	MapY        *float64
	CreatedAt   string
	UpdatedAt   string
	Deleted     int
}

type ResourceRow struct {
	// ID là id số LOCAL, chỉ dùng khi `found = true` (update). Không bao giờ lấy
	// từ snapshot: id 2 máy trùng nhau.
	ID        int64
	GUID      string
	TopicGUID string
	Title     string
	URL       *string
	Kind      string
	Note      string
	Position  int
	CreatedAt string
	UpdatedAt string
	Deleted   int
}

type BookmarkRow struct {
	// ID là id số LOCAL, chỉ dùng khi `found = true` (update). Không bao giờ lấy
	// từ snapshot: id 2 máy trùng nhau.
	ID        int64
	GUID      string
	Title     string
	URL       *string
	Note      string
	Tags      string
	Status    string
	CreatedAt string
	UpdatedAt string
	Deleted   int
}

type ReviewRow struct {
	GUID string
	// CardGUID là GUID của thẻ trong snapshot peer; tầng application resolve
	// sang `CardID` trước khi gọi repository.
	CardGUID   string
	CardID     *int64
	Grade      int
	ReviewedAt string
	NextDueAt  string
}

type NoteRow struct {
	GUID string
	// CardGUID là GUID của thẻ trong SNAPSHOT peer; rỗng = note không gắn thẻ
	// (checklist THIEU, lỗi luyện tự do). Tầng application resolve sang
	// `CardID` trước khi gọi repository.
	CardGUID string
	// CardID là id số ĐÃ resolve trên máy local. nil = ghi SQL NULL thật.
	// Không bao giờ tin `id` của snapshot: 2 máy tự tăng id local trùng nhau.
	CardID    *int64
	Text      string
	CreatedAt string
}

// ReplayReview là 1 lần ôn đọc lại để replay lịch.
type ReplayReview struct {
	Grade      int
	ReviewedAt string
}

// ReplayResult là lịch tính lại từ lịch sử ôn.
type ReplayResult struct {
	Reps       int
	Lapses     int
	Stability  float64
	Difficulty float64
	DueAt      string
}

// Merged là số lượng row đã ghi theo bảng — trả về client để hiện "đã nạp
// bao nhiêu".
type Merged struct {
	Decks             int
	Cards             int
	Reviews           int
	Notes             int
	RoadmapPaths      int
	RoadmapStages     int
	RoadmapMilestones int
	RoadmapTopics     int
	RoadmapResources  int
	RoadmapBookmarks  int
}

// MergeResult là kết quả 1 lần merge.
type MergeResult struct {
	OK        bool
	Merged    Merged
	Conflicts []domain.Conflict
	// Warnings là cảnh báo KHÔNG chặn merge (clock skew). Chỉ cảnh báo, không
	// tự sửa giờ — sửa giờ máy là việc của user.
	Warnings   []string
	LastSyncAt string
}

// NowFunc là nguồn thời gian, inject để test được.
type NowFunc func() time.Time

// Clock mặc định: UTC.
func Clock() time.Time { return time.Now().UTC() }
