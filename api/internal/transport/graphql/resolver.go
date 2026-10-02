package graphql

import (
	contentapp "langapp/internal/application/content"
	insightapp "langapp/internal/application/insight"
	practiceapp "langapp/internal/application/practice"
	roadmapapp "langapp/internal/application/roadmap"
	srsapp "langapp/internal/application/srs"
	syncapp "langapp/internal/application/sync"
)

// Resolver là gốc của cây resolver, giữ 6 service của 6 bounded context có
// bảng (`audio` không có mặt — binary qua REST, xem `internal/transport/http`).
//
// 1 struct cho cả 6 thay vì 6 struct: các use case đã gom theo context ở tầng
// application (`srs.Service` lo cả deck/card/review…) nên chia nhỏ ở đây chỉ
// tăng file mà không tách được trách nhiệm nào. Đổi cấu trúc là đổi hợp đồng
// với `cmd/langapp` và với test.
type Resolver struct {
	SRS      *srsapp.Service
	Content  *contentapp.Service
	Roadmap  *roadmapapp.Service
	Practice *practiceapp.Service
	Insight  *insightapp.Service
	Sync     *syncapp.Service
}

// NewResolver dựng gốc resolver. Không kiểm tra nil service: 1 context thiếu
// là lỗi wiring lúc boot (DI gọi `Container` đã dựng đủ), và để nil nổi lên
// đến lúc query thì lỗi 500 khó truy hơn lỗi boot rõ ràng.
func NewResolver(
	srs *srsapp.Service,
	content *contentapp.Service,
	roadmap *roadmapapp.Service,
	practice *practiceapp.Service,
	insight *insightapp.Service,
	sync *syncapp.Service,
) *Resolver {
	return &Resolver{SRS: srs, Content: content, Roadmap: roadmap, Practice: practice, Insight: insight, Sync: sync}
}
