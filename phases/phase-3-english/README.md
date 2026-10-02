# Phase 3 — English (G0-G4, web)

## Goal

Luyện stress/chunking/elision/liaison (NGONG S01-S24), PVO 300-500 + TMRND + WRITE WAY,
checklist THIEU 8 trục A-H — tất cả trên Vite routes + Go API.

## Dependencies

Phase 1 xong. Độc lập với phase 2.

## Exit criteria

- Route stress: hiển thị trọng âm, chunking tự tách cụm từ (frontend + API dict).
- Deck PVO/TMRND import qua endpoint, drill theo ngày qua SRS API.
- Checklist THIEU 8 trục chấm điểm từng buổi trên web, lưu lịch sử vào SQLite.

## Task list

- `tasks/T3.1-stress-engine.md` — 6h
- `tasks/T3.2-pvo-tmrnd-thieu.md` — 8h

## Parallelization

T3.1 trước (stress engine). T3.2 nối tiếp (dùng chung drill + checklist API).

## Design-doc references

- `learn_english/docs/G1-NGONG-S01-S24.md`
- `learn_english/docs/G2-TAP-PVO-TMRND-WRITEWAY.md`
- `learn_english/docs/G3-THIEU-8-axes.md`

## Risk notes

- Quy tắc stress Anh nhiều ngoại lệ → hiển thị "ngoại lệ" rõ ràng.
- PVO 300-500 bản quyền → kiểm tra license như phase 0.
