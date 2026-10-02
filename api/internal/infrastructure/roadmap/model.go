// Package roadmapinfra là hiện thực GORM của bounded context roadmap. Nơi
// DUY NHẤT trong context này được phép import gorm.io (STACK-V2-PLAN §2).
package roadmapinfra

import (
	app "langapp/internal/application/roadmap"
)

// Path là row `roadmap_paths`.
//
// `UpdatedAt string` CỐ Ý không phải time.Time: trigger
// `trg_roadmap_paths_touch_updated` gán mốc UTC dạng TEXT, còn field tên
// `UpdatedAt` kiểu time.Time sẽ bị GORM tự ghi đè mỗi lần UPDATE (bypass
// trigger → sai mốc LWW phía peer).
//
// `IsBuiltin int` chứ không phải bool: cột là INTEGER, và bool trong GORM sẽ
// ghi 'true'/'false' — Postgres từ chối vì cột kiểu integer.
type Path struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Slug      string `gorm:"column:slug"`
	Language  string `gorm:"column:language"`
	Title     string `gorm:"column:title"`
	Overview  string `gorm:"column:overview"`
	IsBuiltin int    `gorm:"column:is_builtin"`
	CreatedAt string `gorm:"column:created_at;autoCreateTime:false"`
	GUID      string `gorm:"column:guid"`
	UpdatedAt string `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted   int    `gorm:"column:deleted"`
}

// boolToInt chuyển bool của tầng application sang INTEGER 0|1 của Postgres.
// Cột `is_builtin` là integer, GORM sẽ ghi 'true' nếu field kiểu bool.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TableName khoá tên bảng — GORM đoán `roadmap_infra_paths` từ tên package.
func (Path) TableName() string { return "roadmap_paths" }

// Stage là row `roadmap_stages`. `Terrain`/`Direction` là metadata bản đồ
// (migration 00004).
type Stage struct {
	ID            int64   `gorm:"column:id;primaryKey;autoIncrement"`
	PathID        int64   `gorm:"column:path_id"`
	Slug          string  `gorm:"column:slug"`
	Title         string  `gorm:"column:title"`
	Goal          string  `gorm:"column:goal"`
	Position      int     `gorm:"column:position"`
	DurationWeeks int     `gorm:"column:duration_weeks"`
	Status        string  `gorm:"column:status"`
	StatusNote    string  `gorm:"column:status_note"`
	CompletedAt   *string `gorm:"column:completed_at"`
	DeckID        *int64  `gorm:"column:deck_id"`
	Terrain       string  `gorm:"column:terrain"`
	Direction     string  `gorm:"column:direction"`
	CreatedAt     string  `gorm:"column:created_at;autoCreateTime:false"`
	GUID          string  `gorm:"column:guid"`
	UpdatedAt     string  `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted       int     `gorm:"column:deleted"`
}

// TableName khoá tên bảng (xem Path.TableName).
func (Stage) TableName() string { return "roadmap_stages" }

// Topic là row `roadmap_topics`. `MapX`/`MapY` là *float64 vì NULL = để server
// layout — phân biệt NULL với 0 là bắt buộc, node ở (0,0) là vị trí hợp lệ.
type Topic struct {
	ID          int64    `gorm:"column:id;primaryKey;autoIncrement"`
	StageID     int64    `gorm:"column:stage_id"`
	Title       string   `gorm:"column:title"`
	Why         string   `gorm:"column:why"`
	Activities  string   `gorm:"column:activities"`
	Position    int      `gorm:"column:position"`
	Status      string   `gorm:"column:status"`
	StatusNote  string   `gorm:"column:status_note"`
	CompletedAt *string  `gorm:"column:completed_at"`
	IsOptional  int      `gorm:"column:is_optional"`
	MapX        *float64 `gorm:"column:map_x"`
	MapY        *float64 `gorm:"column:map_y"`
	CreatedAt   string   `gorm:"column:created_at;autoCreateTime:false"`
	GUID        string   `gorm:"column:guid"`
	UpdatedAt   string   `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted     int      `gorm:"column:deleted"`
}

// TableName khoá tên bảng (xem Path.TableName).
func (Topic) TableName() string { return "roadmap_topics" }

// Resource là row `roadmap_resources`.
type Resource struct {
	ID        int64   `gorm:"column:id;primaryKey;autoIncrement"`
	TopicID   int64   `gorm:"column:topic_id"`
	Title     string  `gorm:"column:title"`
	URL       *string `gorm:"column:url"`
	Kind      string  `gorm:"column:kind"`
	Note      string  `gorm:"column:note"`
	Position  int     `gorm:"column:position"`
	CreatedAt string  `gorm:"column:created_at;autoCreateTime:false"`
	GUID      string  `gorm:"column:guid"`
	UpdatedAt string  `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted   int     `gorm:"column:deleted"`
}

// TableName khoá tên bảng (xem Path.TableName).
func (Resource) TableName() string { return "roadmap_resources" }

// Milestone là row `roadmap_milestones`.
type Milestone struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	StageID   int64  `gorm:"column:stage_id"`
	Text      string `gorm:"column:text"`
	Position  int    `gorm:"column:position"`
	CreatedAt string `gorm:"column:created_at;autoCreateTime:false"`
	GUID      string `gorm:"column:guid"`
	UpdatedAt string `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted   int    `gorm:"column:deleted"`
}

// TableName khoá tên bảng (xem Path.TableName).
func (Milestone) TableName() string { return "roadmap_milestones" }

// bookmarkRow là row `roadmap_bookmarks` (amendment A1). Cột `tags` là CSV,
// không phải bảng con — mỗi bookmark chỉ 1-3 tag, tách bảng là chi phí mà
// không đổi truy vấn được.
//
// Tên `bookmarkRow` (không export như `Path`/`Stage`…) vì nó là struct GORM
// thuần: DTO trả ra ngoài là `app.Bookmark`, ép kiểu trực tiếp như 5 struct kia.
type bookmarkRow struct {
	ID        int64   `gorm:"column:id;primaryKey;autoIncrement"`
	Title     string  `gorm:"column:title"`
	URL       *string `gorm:"column:url"`
	Note      string  `gorm:"column:note"`
	Tags      string  `gorm:"column:tags"`
	Status    string  `gorm:"column:status"`
	CreatedAt string  `gorm:"column:created_at;autoCreateTime:false"`
	GUID      string  `gorm:"column:guid"`
	UpdatedAt string  `gorm:"column:updated_at;autoUpdateTime:false;autoCreateTime:false"`
	Deleted   int     `gorm:"column:deleted"`
}

// TableName khoá tên bảng (xem Path.TableName).
func (bookmarkRow) TableName() string { return "roadmap_bookmarks" }

// ── Chuyển đổi row ↔ application ────────────────────────────────────────────
// Các struct trên và struct ở application/roadmap có CÙNG layout field (cùng
// tên + cùng kiểu), nên ép kiểu trực tiếp thay vì viết 6 hàm chuyển tay — 1
// đổi tên cột ở đây sẽ không đổi kiểu nên `go build` vẫn xanh. Đây là đánh
// đổi chấp nhận được vì cả 2 đều là DTO nội bộ, không phải contract public.

func pathToApp(p Path) app.Path {
	out := app.Path{
		ID: p.ID, Slug: p.Slug, Language: p.Language, Title: p.Title,
		Overview: p.Overview, IsBuiltin: p.IsBuiltin == 1,
		CreatedAt: p.CreatedAt, GUID: p.GUID, UpdatedAt: p.UpdatedAt,
		Deleted: p.Deleted,
	}
	return out
}

func pathFromApp(p app.Path) Path {
	return Path{
		ID: p.ID, Slug: p.Slug, Language: p.Language, Title: p.Title,
		Overview: p.Overview, IsBuiltin: boolToInt(p.IsBuiltin),
		CreatedAt: p.CreatedAt, GUID: p.GUID, UpdatedAt: p.UpdatedAt,
		Deleted: p.Deleted,
	}
}

func stageToApp(s Stage) app.Stage        { return app.Stage(s) }
func stageFromApp(s app.Stage) Stage      { return Stage(s) }
func topicToApp(t Topic) app.Topic        { return app.Topic(t) }
func topicFromApp(t app.Topic) Topic      { return Topic(t) }
func resToApp(r Resource) app.Resource    { return app.Resource(r) }
func resFromApp(r app.Resource) Resource  { return Resource(r) }
func msToApp(m Milestone) app.Milestone   { return app.Milestone(m) }
func msFromApp(m app.Milestone) Milestone { return Milestone(m) }

func bmToApp(b bookmarkRow) app.Bookmark   { return app.Bookmark(b) }
func bmFromApp(b app.Bookmark) bookmarkRow { return bookmarkRow(b) }
