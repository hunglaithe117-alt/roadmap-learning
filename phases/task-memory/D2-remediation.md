# D2 remediation (oracle gate D2) — 6/6 done

## Fixes
1. `api/db.go` OpenDB: `db.Exec("PRAGMA foreign_keys = ON")` sau open
   (MaxOpenConns=1 nên 1 pragma/conn là đủ) → ON DELETE CASCADE có tác dụng.
2. Migration v2: `CREATE UNIQUE INDEX ux_cards_deck_front ON cards(deck_id, front)`;
   CreateCard trùng → 409 `thẻ đã tồn tại trong deck` (thay vì 500).
3. Migration v2: table FTS5 mới `en_dict(lang, term, reading, gloss)` cho phase-3
   (đã chọn: thêm table mới, không mở rộng `dict` zh hiện có).
4. Contract freeze v2: `cards.tone/ipa/stress/audio_url TEXT NULL` + deck lang
   chỉ zh|en (CreateDeck 400 + trigger backstop `trg_decks_lang_insert/update`,
   vì SQLite không ALTER ADD CHECK) → ghi `phases/task-memory/contract-v2.md`,
   cấm lane tự thêm cột.
5. `ReviewHandler`: UPDATE cards + INSERT reviews trong một `sql.Tx` (atomic).
6. `GET /due`: `AND (due_at <= ? OR state = 'new')` (đã chọn: OR-form, không
   ?include_new flag) + `ScheduleNext` check bounds trước khi index
   (kèm clamp reps<0).

## Migration
- `schemaVersion = 2`; `applyMigrationV2` idempotent (column-exists guard,
  IF NOT EXISTS) → DB v1 cũ reopen tự upgrade, DB mới build thẳng v2.
- schema.sql v1 không sửa (đúng discipline append-only).

## Tests
- Baseline trước fix: 27 test xanh.
- `go vet ./...` + `go test -count=1 ./...`: PASS — 38/38
  (27 cũ giữ xanh + 11 test mới trong `api/d2_remediation_test.go`:
  FK pragma ON, cascade deck→cards, cascade card→reviews/notes,
  unique index + 409 API, review atomic, /due có new cards,
  lang zh|en (API 400 + trigger), v2 columns + en_dict FTS,
  migration version=2, ScheduleNext bounds không panic).
- Ghi chú: đề ghi "28 test cũ" nhưng repo thực tế có 27 test cũ — cả 27 đều xanh.

## Scope守
- Chỉ code + test + 2 file memory (contract-v2.md, D2-remediation.md).
- Không đụng web/, phase-2+, SOLUTION/DECISIONS; không cập nhật deepwork state.
