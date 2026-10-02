# V2 gate remediation — 2 blockers FINDING CHẶN (memory)

Ngày: 2026-09-24. Đọc trước: `T9.1-migration-merge.md` (luật merge giữ nguyên,
chỉ sửa điểm chặn dưới đây). KHÔNG đụng audio/compose, KHÔNG sửa SOLUTION/DECISIONS.

## Blocker #1 — seed độc lập 2 máy → sync nhân đôi (FIXED)

Nguyên nhân: `ZhImport` / `SeedEnglish` (`ensureEnDeck`) / `SeedDemo` sinh
`newGUID()` (uuid4) ngẫu nhiên → 2 máy cùng import HSK có guid khác nhau,
merge chỉ khớp guid → nhân đôi deck + card.

Fix — guid seed ỔN ĐỊNH uuid5 (`api/sync.go`):
- `seedGUIDNamespace` (UUID cố định trong code) + `uuid.NewSHA1`.
- Deck: `seedDeckGUID(name, lang)` từ `"deck:"+name+":"+lang`.
- Card: `seedCardGUID(deckName, lang, front)` từ `"card:"+deckName+":"+lang+":"+front`.
- Áp cho mọi seed path: `ZhImportHandler` (`chinese_hsk.go`), `ensureEnDeck`
  (`english.go`, PVO/TMRND), `SeedDemo` (`db.go`). Guid user (tạo tay, review,
  note) vẫn uuid4.
- Seed lại trên DB cũ hội tụ: (a) migration v4 `normalizeSeedGUIDs` chuẩn hóa
  guid seed cũ về ổn định khi còn trống (`NOT EXISTS`, không vỡ UNIQUE);
  (b) mỗi seed path tự repair guid + hồi sinh tombstone cùng natural key khi
  seed lại (import lại sau xóa → undelete, tính `cards_added`).
- Test cùng DB import 2 lần → guid giống hệt (trong test mới).

Test mới: `Test_v2_independent_seed_then_sync_no_duplicates` (`api/v2_gate_test.go`):
2 DB cùng import HSK1 → deck guid bằng nhau → import lại guid không đổi →
sync B→A `merged{decks:0,cards:0}`, đúng 1 deck + 36 cards.

## Blocker #2 — tombstone + UNIQUE unconditional nuốt re-create (FIXED)

Nguyên nhân: `ux_cards_deck_front` bao cả `deleted=1` → xóa mềm rồi tạo lại
cùng front bị 409 vĩnh viễn; sync peer re-create (guid mới, cùng front) INSERT
lỗi UNIQUE → `continue` câm, mất nội dung peer.

Fix — migration v4 (`applyMigrationV4`, `schemaVersion`/`syncSchemaVersion` 3→4):
- `DROP INDEX ux_cards_deck_front` + `CREATE UNIQUE INDEX ... ON cards(deck_id, front)
  WHERE deleted=0` (idempotent; `OpenDB` + `SyncHandler` đều chạy).
- `CreateCard` (`decks.go`): tombstone cùng (deck, front) → REUSE
  (`deleted=0, back/pinyin/due mới, updated_at=now`, giữ guid) trả 201 thay vì 409.
  Trùng với row LIVE vẫn 409 như cũ (D2-2 giữ).
- Sync `mergeDecksCards` (`sync.go`): incoming guid lạ + tombstone local cùng
  natural key → ADOPT (UPDATE tombstone: nhận guid + nội dung incoming,
  `deleted=0`) + log `recreate-adopted-tombstone` (merged.Cards++). Cả hai cùng
  deleted → chỉ map id. INSERT lỗi UNIQUE (trùng row LIVE khác guid) → giữ local
  + log `unique-live-duplicate` (KHÔNG continue câm). Deck fallback theo name
  cũng log `insert-failed-adopted-local-by-name`.
- Seed path re-import hồi sinh tombstone (nêu ở #1) cùng triết lý REUSE.

Test mới: `Test_v2_delete_then_recreate_card` (`api/v2_gate_test.go`):
local xóa→tạo lại 201 + giữ guid (1 live row); A xóa + B tạo mới cùng front
(guid khác) → sync vào A: 1 live card nội dung B, guid adopt theo B, conflict
`recreate-adopted-tombstone`.

## Doc (nhỏ, cùng lane)

- UI Cài đặt (`web/src/routes/CaiDat.tsx`): thêm dòng "đồng bộ 1 chiều — muốn
  2 chiều làm 2 lượt ngược nhau".
- `DEPLOY.md` mục "Đồng bộ peer": restore reset `last_sync_at` (code
  `backup.go`: snapshot thiếu `sync_meta` → xóa `last_sync_at`, sync từ baseline
  mới), clock-skew chỉ warn, xóa note không lan (union), `legacy-tie` incoming
  thắng + trigger có thể bump `updated_at` (chấp nhận, có log).

## Test liên quan phải cập nhật (hệ quả version, không phải đổi contract)

- `Test_d2_migration_version_is_3`: expect 4 (migration v4 mới nhất).
- `Test_sync_merge_legacy_snapshot_without_guid_matches_natural_key`: strip thêm
  `DROP INDEX ux_cards_deck_front` trước `DROP COLUMN deleted` (partial index v4
  bám cột deleted).

## Validation

- `go test ./...` (api): PASS (gồm 2 test mới + 15 test sync cũ + D2).
- `pnpm vitest` + `tsc` (web): xem kết quả chạy bên dưới.

## Bẫy đã gặp

- `INSERT OR IGNORE` với guid ổn định + tombstone cùng guid → bị nuốt → phải
  SELECT trước (ưu tiên live) rồi undelete/repair tường minh.
- Partial index khiến `DROP COLUMN deleted` trong test legacy vỡ → gỡ index trước.
- Adopt tombstone nhận guid incoming (không giữ guid tombstone cũ) để 2 máy hội
  tụ guid, tránh conflict log lặp mỗi lần sync.
