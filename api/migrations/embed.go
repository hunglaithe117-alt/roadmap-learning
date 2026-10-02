// Package migrations nhúng toàn bộ file .sql migration Postgres vào binary
// để `goose` chạy lúc boot mà không cần đọc file từ disk (xem
// STACK-V2-PLAN §1: goose.DialectPostgres + embed.FS + provider.Up(ctx)).
//
// File .sql nằm ở đây — KHÔNG dưới internal/platform/ — vì đây là nơi DDL
// được review (M1 gate: oracle review DDL). Go embed không đi lên khỏi thư mục
// package được, nên tách package riêng là cách duy nhất vừa giữ 1 nguồn DDL
// vừa embed được.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
