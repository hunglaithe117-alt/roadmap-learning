# Phase 2 — Chinese (G0-G4, web)

## Goal

Học pinyin/thanh điệu, 150→1200 từ HSK1-4 trên Vite routes, luyện viết nét chữ,
game tone bingo giữ daily 10+10+10p; dữ liệu qua Go API + FTS5.

## Dependencies

Phase 1 xong (web shell + API + SRS + deck model + FTS5).

## Exit criteria

- Route pinyin: gõ pinyin có dấu đúng, tone-pair drill chấm điểm qua API.
- Deck HSK1-HSK4 import qua endpoint, tra FTS5 ra hán tự/pinyin/nghĩa <200ms.
- Viết chữ theo thứ tự nét trên canvas web, check đúng/sai cơ bản.
- Tone bingo chơi được 1 vòng trên browser, điểm lưu vào SRS qua API.

## Task list

- `tasks/T2.1-tone-engine.md` — 8h
- `tasks/T2.2-hanzi-stroke-bingo.md` — 8h

## Parallelization

T2.1 trước (tone engine + API). T2.2 nối tiếp (dùng chung drill API).

## Design-doc references

- `learn_chinese/docs/G0-pinyin-tone-plan.md`
- `learn_chinese/docs/G1-G4-HSK-roadmap.md`
- `learn_chinese/docs/daily-10-10-10-routine.md`

## Risk notes

- pinyin-pro khác biệt giọng địa phương → cho chọn chuẩn phổ thông.
- Dữ liệu nét chữ nặng → lazy-load route theo HSK level.
