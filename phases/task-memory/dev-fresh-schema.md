# Dev-fresh schema (v4) — quyết định + những gì đã xóa

Ngày: 2026-09-24. Trạng thái: ĐANG DEV.

## Quyết định (user)

- KHÔNG quan tâm data cũ — DB cũ xóa làm lại, chỉ hỗ trợ schema mới nhất (v4).
- Fresh DB tạo 1 phát ra đúng schema cuối, KHÔNG chạy chuỗi migrate.
- Merge/restore chỉ nhận DB cùng version; khác version trả lỗi 400 rõ ràng
  "xóa DB làm lại", KHÔNG fallback, KHÔNG chép giao cột.

## Giữ nguyên (vẫn cần cho DB mới)

- uuid5 seed guid (`seedDeckGUID`/`seedCardGUID`) → independent-seed hội tụ.
- Tombstone `deleted=1` + REUSE khi tạo lại cùng front (201, không 409).
- Partial index `ux_cards_deck_front WHERE deleted=0`.
- LWW theo `updated_at` + tombstone thắng hòa, conflict log (`sync_conflicts`,
  `both-changed` / `recreate-adopted-tombstone` / `unique-live-duplicate` /
  `reps-recomputed`), append reviews + replay due, union notes, remap
  guid→id (không tin id số), clock-skew warn, trigger chạm `updated_at`.
- `columnExists` (giữ cho contract test), `syncStr` (coerce kiểu SQLite).

## Đã xóa

| File | Đã xóa |
|---|---|
| `api/db.go` | `applyMigrationV2` (shared cols, index unconditional, en_dict, lang triggers — giờ nằm thẳng trong schema.sql); chuỗi `applyMigrationV2/V3/V4` trong `OpenDB` (chỉ exec schema.sql + ghi version 4); khối hội tụ guid cũ trong `SeedDemo` (deck + card `UPDATE ... SET guid`) |
| `api/sync.go` | `addColumnIfMissing`, `backfillGUIDs` (uuid4 backfill), `applyMigrationV3` (cột v3 + backfill + unique guid + trigger + sync tables), `applyMigrationV4` (DROP index cũ + partial index + `normalizeSeedGUIDs`), `seedDeckKeys`, `seedCardFronts`, `normalizeSeedGUIDs`, `incomingCols` (dò cột incoming), `snapshotCols` tương đương; `mergeDecksCards`: natural-key fallback (`localByName`, guid rỗng → khớp name / deck+front), `legacyDeckTS/legacyCardTS` + `legacy-tie-incoming-wins`, nhánh `insert-failed-adopted-local-by-name`; `mergeReviews`: `localRevKey` fallback (card+reviewed_at+grade), nhánh `cardHave`/thiếu cột guid; `mergeNotes`: `localNoteKey` fallback, nhánh `incCardHave`; 2 gọi `applyMigrationV3/V4` trong `SyncHandler` |
| `api/backup.go` | `snapshotCols` + chép giao cột trong `copyTable` (giờ chép thẳng full cột v4); nhánh reset `last_sync_at` khi snapshot thiếu `sync_meta` (fresh snapshot luôn có) |
| `api/sync_test.go` | `Test_sync_merge_legacy_snapshot_without_guid_matches_natural_key` (legacy strip + DROP COLUMN); đổi tên `Test_sync_migration_backfill_guids_unique` → `Test_sync_fresh_schema_guids_unique_stable`; thêm `Test_sync_rejects_version_mismatch` + `Test_sync_restore_rejects_version_mismatch` |
| `api/d2_remediation_test.go` | Đổi tên `Test_d2_migration_version_is_3` → `Test_d2_fresh_schema_version_is_4` |
| `api/schema.sql` | Viết lại thành single source of truth v4 (decks/cards + guid/updated_at/deleted + shared cols + partial index + guid uniques + lang-guard + touch triggers; reviews/notes + guid; dict + en_dict FTS5; sync_meta/sync_conflicts) |

## Thêm mới

- `versionMismatchMessage` + `localSchemaVersion`/`incomingSchemaVersion` (`api/sync.go`):
  dùng chung cho sync và restore.
- `SyncHandler`/`RestoreHandler`: check version sau ATTACH, khác → 400.

## Không đụng

- Audio/compose/UI, `phases/SOLUTION*.md`, `phases/DECISIONS.md` (chỉ đọc).
- `api/decks.go` (REUSE tombstone), `api/chinese_hsk.go` + `api/english.go`
  (seed guid ổn định), web (không có test legacy nào).

## Validation 2026-09-24

- `go build ./...` + `go vet ./...`: sạch.
- `go test ./...`: ok (sync/v2/d2/t6 + 2 test version-mismatch mới pass).
- `pnpm vitest run`: 21 files / 105 tests pass. `tsc --noEmit`: sạch.
