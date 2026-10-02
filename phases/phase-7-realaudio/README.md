# Phase 7 — Real Audio (Piper TTS + Whisper STT thật)

## Goal

Thay TTS/STT stub bằng giọng thật và transcript thật: `/api/tts` ra giọng Piper
thật (header `X-Engine: piper`), `/api/stt` transcript thật câu mẫu Trung + Anh.
Latency mỗi chiều được ghi vào task-memory.

## Dependencies

- v1 đã xong (Go API + SQLite v2 + Vite SPA 14 routes + Docker). Phase-7 độc lập
  sau v1, không phụ thuộc phase-8/9.

## Exit criteria

- `GET /api/tts?text=...&lang=zh|en` trả audio giọng thật, header `X-Engine: piper`.
- `POST /api/stt` (multipart audio) trả transcript thật cho 1 câu mẫu Trung + 1 câu
  mẫu Anh.
- Latency TTS/STT câu mẫu được ghi trong `phases/task-memory/T7.*.md`.

## Task list

- `tasks/T7.1-piper-tts.md` — 8h
- `tasks/T7.2-whisper-stt.md` — 8h

## Parallelization

- T7.1 và T7.2 song song được (TTS exec local vs STT service HTTP, không chạm nhau).
- Phase-7/8/9 song song được sau khi duyệt plan (độc lập nhau, chỉ chung API/health).

## Design-doc references

- `phases/SOLUTION.md` (TTS Piper + faster-whisper sidecar đã chốt cho v1)
- `phases/SOLUTION-v2.md` (mục 7: Piper exec thay vì wyoming sidecar)
- `phases/phase-6-release/README.md` (compose stack + volume models)

## Risk notes

- Model + voice nặng (~160MB Piper + ~466MB whisper small → tổng ~2GB tính cả
  cache/layer): tách image/layer, volume `stt-cache`, cho chọn model size.
- Voice Piper GPL (mere aggregation khi distribute: kèm offer source + attribution
  voice CC-BY).
- Whisper small CPU int8 cần ~1.5GB RAM; máy yếu thì xuống base + `beam_size=1`.
