# SOLUTION — Lựa chọn stack web-v1

## Option A: TS Vite SPA + Go backend (khuyến nghị)

- Một ngôn ngữ backend quen thuộc (Go), single binary `net/http`, dễ dựng API cho deck/SRS/dict.
- `modernc.org/sqlite` pure-Go, FTS5 offline ngay trong backend, frontend gọi qua REST.
- Gọi sidecar Piper/Kokoro + faster-whisper bằng `os/exec`, stream audio/file về browser.
- Không cần Wails/Tauri packaging; deploy bằng Docker + static frontend.

## Option B: TS Vite SPA + Python FastAPI (fallback)

- Khớp code-style FastAPI hiện có, gọi faster-whisper native không cần sidecar riêng.
- Giá phải trả: thêm runtime Python khi deploy, SQLite FTS5 cấu hình riêng, team chính quen Go hơn.
- Phù hợp nếu sau này STT/TTS cần nhiều lib Python (transformers, datasets).

## Trade-offs

| Tiêu chí | A: TS + Go backend | B: TS + FastAPI |
|---|---|---|
| Ngôn ngữ backend | Go, team đã quen | Python, khớp style FastAPI sẵn |
| Single binary deploy | Có (`go build`) | Không, cần runtime Python |
| SQLite FTS5 offline | pure-Go, dễ | được, config thêm |
| Gọi sidecar TTS/STT | `os/exec` đơn giản | native Python, gọn hơn |
| STT faster-whisper | Sidecar HTTP riêng | Cùng process/stack |
| Dev frontend | Vite SPA như nhau | Vite SPA như nhau |

## Quyết định

Chọn **Option A (TS SPA + Go backend)** cho web-v1 vì 1 ngôn ngữ backend quen thuộc,
single binary dễ deploy Docker, dễ gọi sidecar `os/exec`. Giữ Option B làm fallback
nếu STT/TTS nghiêng hẳn sang stack Python.

## Câu hỏi cho user — đã tự chốt (2026-09-23, user ủy quyền)

1. TTS mặc định: **Piper** (ONNX <50MB/voice, CPU realtime, sidecar nhẹ; Kokoro để optional khi cần giọng chuẩn hơn).
2. STT: **faster-whisper sidecar HTTP riêng** (model distil-small/base INT8 CPU); backend Go gọi qua HTTP; whisper.cpp làm fallback.
3. Auth/sync v1: **không** — single-user, SQLite local là source of truth; export/import + sync (Turso/push-pull) để phase-6 optional.
