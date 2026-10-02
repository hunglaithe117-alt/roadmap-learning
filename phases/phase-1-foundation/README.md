# Phase 1 — Foundation (Web shell + API + SQLite + SRS)

## Goal

Có web shell (Vite routes Học/Review/Cài đặt), Go API (`net/http`) + SQLite FTS5 pure-Go,
engine SRS ts-fsrs và model deck dùng chung cho cả Anh + Trung.

## Dependencies

Phase 0 xong (toolchain web + license dict).

## Exit criteria

- SPA mở được 3 routes (Học/Review/Cài đặt), gọi API health + deck CRUD thành công.
- Go API tạo DB, FTS5 tìm kiếm từ mẫu <100ms qua endpoint `/api/search`.
- SRS lên lịch đúng FSRS, fallback 1-3-7-14-30 khi chưa đủ dữ liệu.
- CRUD deck/card/note qua Go API handlers + SQLite, có test `go test ./...`.

## Task list

- `tasks/T1.1-web-shell-api.md` — 8h
- `tasks/T1.2-srs-engine.md` — 6h

## Parallelization

T1.1 trước (shell + API + SQLite). T1.2 nối tiếp (SRS dùng deck model từ T1.1).

## Design-doc references

- `learn_chinese/docs/SRS-schedule-1-3-7-14-30.md`
- `learn_english/docs/core-SRS-recorder-errorbook.md`

## Risk notes

- `modernc.org/sqlite` chậm hơn cgo ở DB lớn → đo bench sớm qua API.
- ts-fsrs version drift → pin version trong `package.json`.
