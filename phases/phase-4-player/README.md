# Phase 4 — Player (Shadow + Recorder + ErrorBook, web)

## Goal

Player shadowing A-B loop (HTMLAudio) dùng chung Anh/Trung, ghi âm browser
(MediaRecorder) so sánh với mẫu qua STT API, sổ lỗi tự gom lỗi lặp.

## Dependencies

Phase 1 xong. Nên có phase 2/3 engine để test end-to-end.

## Exit criteria

- Shadowing loop A-B trên web, chỉnh tốc độ 0.5x-1.5x mượt.
- Ghi âm → upload → STT API → diff với câu mẫu, highlight từ sai.
- ErrorBook tự thêm lỗi qua API, gợi ý ôn lại qua SRS.

## Task list

- `tasks/T4.1-shadow-player.md` — 8h
- `tasks/T4.2-recorder-errorbook.md` — 8h

## Parallelization

T4.1 trước (player + audio pipeline). T4.2 nối tiếp (dùng chung upload/STT API).

## Design-doc references

- `learn_chinese/docs/daily-10-10-10-routine.md` (shadowing)
- `learn_english/docs/core-SRS-recorder-errorbook.md`
- `learn_english/docs/G4-integration-video-writing.md`

## Risk notes

- Latency STT sidecar cao → hiển thị progress, cho skip.
- Quyền micro browser khác nhau → test Chrome + Firefox.
