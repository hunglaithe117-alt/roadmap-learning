// Package roadmapseed nhúng 2 file JSON seed của learning path vào binary
// để loader đọc lúc boot mà không phụ thuộc file trên disk (giống hệt
// `migrations` package, xem STACK-V2-PLAN §1: goose + embed.FS).
//
// Vì sao phải tách package: Go `embed` KHÔNG đi lên khỏi thư mục package
// được. Loader v1 nằm ở `api/roadmap_seed.go` (package main, thư mục `api/`)
// nên nó embed được cả thư mục con `roadmap_seed/`. Loader mới của STACK-V2
// nằm ở `internal/infrastructure/roadmap/` — sâu hơn 3 cấp, embed từ đó
// không với tới `api/roadmap_seed/`. Cách sạch nhất là đặt 1 package `embed`
// ngay trong thư mục chứa JSON, y hệt cách `api/migrations/embed.go` làm cho
// file `.sql`.
//
// Pattern là `*.json` chứ không phải cả thư mục: `README.md` trong cùng thư mục
// là tài liệu, nhúng vào binary chỉ thêm byte chết. `*.json` luôn khớp file
// (repo luôn có 2 file seed), nên không gặp lỗi "pattern no match".
package roadmapseed

import "embed"

//go:embed *.json
var FS embed.FS

// Dir là tên thư mục chứa JSON trong FS — nếu sau này seed được gom vào
// thư mục con, chỉ sửa hằng này.
const Dir = "."
