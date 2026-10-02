# Contract v2 — frozen shared schema for P2 (Chinese) / P3 (English) parallel lanes

Status: FROZEN after oracle gate D2. Both lanes MUST comply. No lane may
add its own columns — propose a v3 migration instead.

## 1. Shared nullable columns on `cards` (migration v2)

```sql
ALTER TABLE cards ADD COLUMN tone      TEXT NULL;  -- zh: thanh điệu (P2)
ALTER TABLE cards ADD COLUMN ipa       TEXT NULL;  -- en: phiên âm (P3)
ALTER TABLE cards ADD COLUMN stress    TEXT NULL;  -- en: trọng âm (P3)
ALTER TABLE cards ADD COLUMN audio_url TEXT NULL;  -- chung: audio đã sinh (P2/P3)
```

- All four are TEXT NULL, never NOT NULL. Old rows stay NULL.
- P2 owns `tone`; P3 owns `ipa`, `stress`; `audio_url` is shared.
- Go `Card` struct intentionally unchanged in D2 — API exposure of these
  fields belongs to P2/P3 (add JSON fields then, keep omitempty + NULL-safe scan).
- ⛔ CẤM `ALTER TABLE cards ADD COLUMN <riêng>` trong P2/P3.

## 2. Deck lang guard

- Allowed: `lang IN ('zh','en')` ONLY.
- Enforced at two levels (SQLite cannot `ADD CHECK` via ALTER, so):
  1. API: `CreateDeck` returns 400 `lang chỉ nhận zh hoặc en` for others.
  2. DB backstop: triggers `trg_decks_lang_insert` / `trg_decks_lang_update`
     `RAISE(ABORT, 'invalid deck lang (want zh|en)')`.
- Treat the triggers as the CHECK(lang IN ('zh','en')) contract.

## 3. Dictionaries

- `dict` (FTS5 hanzi, pinyin, nghia) — zh, unchanged, owned by P1/P2.
- `en_dict` (FTS5 lang, term, reading, gloss) — en, owned by P3.
  Do NOT merge them; do NOT add a third dict table without v3.

## 4. Invariants lanes must not break

- `PRAGMA foreign_keys = ON` per OpenDB (MaxOpenConns=1) — cascades
  decks→cards→(reviews, notes) rely on it.
- `UNIQUE INDEX ux_cards_deck_front ON cards(deck_id, front)` — duplicate
  card POST returns 409; SeedDemo stays idempotent via COUNT check.
- `POST /api/review` is transactional (cards UPDATE + reviews INSERT in one
  sql.Tx) — do not split.
- `GET /api/decks/{id}/due` returns overdue OR `state='new'` (OR-form, no
  ?include_new flag) — study queue must show unreviewed cards.
- `ScheduleNext`: bounds-check `reps` BEFORE indexing `fallbackIntervals`.

## 5. Migration discipline

- schema.sql v1 is append-only history — never edit in place.
- schema_migrations MAX(version) = 2. Next change = new `applyMigrationV3`,
  bump `schemaVersion`, keep every step idempotent (IF NOT EXISTS /
  column-exists guard).
