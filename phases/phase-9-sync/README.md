# Phase 9 — Sync (peer file-carry + merge LWW)

## Goal

Đồng bộ 2 máy ngang hàng bằng file-carry: backup → chép tay → `POST /api/sync`
merge, giữ nút restore hủy diệt như cũ; UI Cài đặt có nút Đồng bộ + xem log
xung đột.

## Dependencies

- v1 đã xong (SQLite, API, SPA Cài đặt). Phase-9 sau v1, độc lập phase-7/8.

## Exit criteria

- Migration v3 + merge 1 transaction ATTACH incoming đúng luật: decks/cards LWW
  theo `updated_at` + tombstone thắng, reviews append dedupe guid rồi recompute +
  replay SRS, notes union guid, bỏ qua dict/seed, cảnh báo lệch đồng hồ.
- `POST /api/sync` merge 2 DB lệch nhau không mất dữ liệu mới nhất; `last_sync_at`
  cập nhật; xung đột ghi `sync_conflicts`.
- Nút Đồng bộ + xem log xung đột trong Cài đặt hoạt động.

## Task list

- `tasks/T9.1-migration-merge.md` — 8h
- `tasks/T9.2-sync-ui.md` — 6h

## Parallelization

- T9.1 trước T9.2 (UI cần API merge xong mới làm được).
- Phase-7/8/9 song song được sau khi duyệt plan (độc lập nhau, chỉ chung API/health).

## Design-doc references

- `phases/SOLUTION-v2.md` (mục 9: peer file-carry thay vì 1 máy server)
- `phases/phase-6-release/tasks/T6.2-sync-optional.md` (sync optional v1: LWW + log)
- `phases/DECISIONS.md` (single-user SQLite local là source of truth)

## Risk notes

- Clock-skew: 2 máy lệch giờ → LWW sai; merge phải cảnh báo khi `updated_at`
  tương lai/quá lệch.
- Id số trùng: 2 máy tự tăng id local trùng nhau → merge theo `guid` (UNIQUE),
  không theo id số.
- Restore hủy diệt vẫn giữ nhưng tách rõ khỏi merge (tránh bấm nhầm mất dữ liệu).
