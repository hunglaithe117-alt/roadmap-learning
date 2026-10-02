# Phase 6 — Release (Docker + deploy, web)

## Goal

Đóng Docker image (backend Go + frontend static + sidecar STT/TTS),
deploy 1 lệnh chạy được, sync đa thiết bị ở mức optional (tắt mặc định).

## Dependencies

Mọi phase 0-5 xong.

## Exit criteria

- `docker compose up` chạy full stack (web + api + stt sidecar) offline được.
- Frontend static + backend health-check xanh sau deploy, data SQLite persist volume.
- Sync (nếu làm) bật/tắt được, xung đột giải quyết last-write-win + log.

## Task list

- `tasks/T6.1-docker-deploy.md` — 8h
- `tasks/T6.2-sync-optional.md` — 8h

## Parallelization

T6.1 trước. T6.2 sau, có thể cắt khỏi v1 nếu thiếu giờ.

## Design-doc references

- `learn_chinese/docs/G4-HSK4-integration.md` (mục tiêu release nội dung)
- `learn_english/docs/G4-integration-video-writing.md`

## Risk notes

- Image sidecar STT nặng (model whisper) → tách image, cho chọn model size.
- Sync làm phình scope → giữ optional, mặc định tắt, single-user trước.
