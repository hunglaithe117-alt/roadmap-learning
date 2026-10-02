# Phase 5 — Review (Reader + Dashboard + Streak, web)

## Goal

Graded reader tra từ nhanh trên web, dashboard weekly 30p/monthly 90p,
streak + nhắc học (browser notification) giữ thói quen.

## Dependencies

Phase 1 + 4 xong (web shell + API + SRS + player + error book).

## Exit criteria

- Bấm vào từ trong bài đọc hiện pinyin/nghĩa <200ms (gọi FTS5 API).
- Dashboard route hiện tiến độ tuần/tháng, lỗi lặp top 10 từ API stats.
- Streak + reminder browser (Notification API) hoạt động, state lưu SQLite.

## Task list

- `tasks/T5.1-reader-dict.md` — 8h
- `tasks/T5.2-dashboard-streak.md` — 6h

## Parallelization

T5.1 và T5.2 song song được (reader API và stats API độc lập).

## Design-doc references

- `learn_chinese/docs/G3-graded-reader.md`
- `learn_english/docs/G4-integration-video-writing.md`
- `learn_english/docs/core-SRS-recorder-errorbook.md` (review dashboard)

## Risk notes

- Bài đọc bản quyền → chỉ bundle text public domain/tự biên.
- Notification browser cần quyền user → fallback nhắc trong-app.
