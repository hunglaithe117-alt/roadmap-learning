// Package httptransport là tầng HTTP của app: Gin cho 5 endpoint NHỊ PHÂN +
// static SPA, cộng 1 endpoint JSON nhỏ (`/api/health`) cho HEALTHCHECK.
//
// Vì sao 5 endpoint này KHÔNG đi qua GraphQL (STACK-V2-PLAN §3):
//
//	GET  /api/tts       stream audio/wav   — GraphQL không có streaming binary;
//	                                       base64 là +33% payload vô nghĩa
//	POST /api/stt       multipart upload  — `transport.MultipartForm` của
//	                                       gqlgen bắt client gửi map
//	                                       `operations`, thuần file thì REST
//	                                       rõ hơn nhiều
//	GET  /api/backup    download file     — binary download
//	POST /api/restore   multipart         — như stt
//	static web/dist     SPA + fallback    — không phải API
//
// Mọi use case JSON của 6 bounded context đã đi qua GraphQL (`/query`).
package httptransport
