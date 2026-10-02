# Phase 0 — Precheck & Spike (web)

## Goal

Xác minh toolchain web (Vite SPA + Go API + SQLite) chạy được trên browser,
audio sidecar (TTS/STT) gọi qua backend offline, license từ điển rõ ràng trước khi code foundation.

## Dependencies

Không có. Phase đầu tiên, chặn mọi phase sau.

## Exit criteria

- `go run ./...` + `pnpm dev` chạy song song; SPA gọi API health-check thành công.
- Ghi âm browser → backend → STT sidecar trả transcript offline thành công.
- Backend gọi Piper/Kokoro, stream audio về browser phát được offline.
- Danh sách nguồn dict + license cho phép bundle đã chốt.

## Task list

- `tasks/T0.1-web-toolchain-spike.md` — 4h
- `tasks/T0.2-audio-sidecar-spike.md` — 6h

## Parallelization

T0.1 chạy trước. T0.2 nối tiếp T0.1 (dùng chung backend skeleton).

## Design-doc references

- `learn_chinese/docs/G0-pinyin-tone-plan.md` (yêu cầu audio tone-pair)
- `learn_english/docs/G0-methods.md` (yêu cầu shadowing audio)

## Risk notes

- faster-whisper sidecar Python nặng → tách container riêng, API timeout rõ ràng.
- MediaRecorder khác nhau giữa Chrome/Firefox → spike cả 2 trình duyệt.

## Verification plan (ghi 2026-09-23, user đã duyệt solution web-v1)

- Claims: (1) Vite SPA fetch được Go API `/api/health` + `/api/search` (SQLite FTS5 pure-Go) trên máy này; (2) version Go/pnpm pin được cho phase-1; (3) latency search mẫu < 100ms.
- Uncertainty chính: máy đã có Go/pnpm chưa, build `modernc.org/sqlite` lần đầu có quá chậm không, browser test bằng gì (không có GUI thì dùng curl + vitest thay vì click tay).
- Evidence path: `go version` + `pnpm --version` → dựng skeleton tối thiểu `web/` + `api/` trong `lang-learn-app/` → `go test ./...` (health + FTS5 mẫu) → `pnpm vitest` (fetch hook) → báo cáo `phases/task-memory/T0.1-web-toolchain-spike.md`. Không tải model TTS/STT ở phase này (để T0.2).
- Budget: tối thiểu, không thêm dependency ngoài `modernc.org/sqlite`; browser thật chỉ khi có sẵn, nếu không thì curl + test thay thế.
