// Package model chứa view model của tầng transport GraphQL.
//
// ĐA SỐ type ở đây do `gqlgen` sinh (`models_gen.go`) — xem `gqlgen.yml`.
// Ba type dưới đây (`Path`, `Stage`, `Topic`) là **viết tay** và được
// `autobind` trong `gqlgen.yml`, với 1 lý do cụ thể:
//
// Nếu model có sẵn field `Stages []Stage` thì gqlgen đọc thẳng field đó và
// KHÔNG sinh resolver — tức dataloader của `Path.stages` không bao giờ chạy, và
// cây roadmap quay lại N+1. Bỏ field khỏi model buộc gqlgen sinh resolver, và
// resolver đó gọi `Loaders` (xem `roadmap.resolvers.go`).
//
// Đây là lý do các type này CỐ Ý không phải mirror của struct application:
// `application/roadmap` không có `Level`/`Point`/`MapPinned`/`ActivityList` ở
// `Topic` (chúng nằm ở `TopicView` / phải tách JSON), và `ID` ở đây là `string`
// trong khi application dùng `int64`. Ràng buộc field của GraphQL với field
// của DB bằng cách bind thẳng sẽ buộc sửa tầng dưới chỉ để phục vụ transport.
package model

// Path là 1 learning path. KHÔNG có `Stages` / `Progress` — 2 field đó là
// resolver (dataloader), xem `roadmap.resolvers.go`.
type Path struct {
	ID        string `json:"id"`
	GUID      string `json:"guid"`
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Overview  string `json:"overview"`
	Language  string `json:"language"`
	IsBuiltin bool   `json:"isBuiltin"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Stage là 1 stage. KHÔNG có `Topics` / `Milestones` — 2 field đó là resolver.
type Stage struct {
	ID            string    `json:"id"`
	GUID          string    `json:"guid"`
	PathID        string    `json:"pathId"`
	Slug          string    `json:"slug"`
	Title         string    `json:"title"`
	Goal          string    `json:"goal"`
	Position      int       `json:"position"`
	DurationWeeks int       `json:"durationWeeks"`
	Status        Status    `json:"status"`
	StatusNote    string    `json:"statusNote"`
	CompletedAt   *string   `json:"completedAt,omitempty"`
	DeckID        *string   `json:"deckId,omitempty"`
	Terrain       Terrain   `json:"terrain"`
	Direction     Direction `json:"direction"`
	CreatedAt     string    `json:"createdAt"`
	UpdatedAt     string    `json:"updatedAt"`
}

// Topic là 1 node bản đồ. KHÔNG có `Resources` (resolver), `Level` / `Point` /
// `MapPinned` (đọc từ bảng layout của application) và `ActivityList` (tách từ
// cột JSON).
type Topic struct {
	ID          string   `json:"id"`
	GUID        string   `json:"guid"`
	StageID     string   `json:"stageId"`
	Title       string   `json:"title"`
	Why         string   `json:"why"`
	Activities  string   `json:"activities"`
	Position    int      `json:"position"`
	Status      Status   `json:"status"`
	StatusNote  string   `json:"statusNote"`
	CompletedAt *string  `json:"completedAt,omitempty"`
	IsOptional  bool     `json:"isOptional"`
	MapX        *float64 `json:"mapX,omitempty"`
	MapY        *float64 `json:"mapY,omitempty"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
}
