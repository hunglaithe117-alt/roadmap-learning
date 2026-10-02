// Package graphql là tầng transport GraphQL: schema, resolver và dataloader
// cho 6 bounded context có bảng.
//
// Phân tầng (STACK-V2-PLAN §2):
//   - `internal/transport/graphql` (package này) — resolver, view model, loader.
//     KHÔNG import gorm, chỉ gọi application service.
//   - `graph/schema/*.graphqls` — nguồn schema.
//   - `internal/transport/graphql/generated` — code máy sinh, không sửa tay.
//
// `audio` không có resolver: streaming WAV và multipart upload không thuộc
// GraphQL (§3), chúng ở `internal/transport/http` (`/api/tts`, `/api/stt`).
package graphql

//go:generate gqlgen generate
