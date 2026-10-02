# M3 — Remediation cổng Oracle (FAIL)

Oracle đã reproduce được 2 blocker, không phải suy đoán. Báo cáo này ghi lại
5 finding, cách sửa, và **mutation-check** (sửa ngược lại hành vi cũ thì test
phải FAIL thật).

Trạng thái cuối: **5/5 fix**, 21/21 package xanh, 3 lần liên tiếp 0 FAIL 0 SKIP.

---

## 1. B1 — BLOCKER: merge xoá `roadmap_stages.deck_id` ⇒ chết feature A1

### Oracle repro

local có stage gắn deck (`deck_id=1`); peer sửa stage đó (`updated_at` mới hơn)
→ sau merge `deck_id = NULL`. Không conflict log, không lỗi — âm thầm.

### Nguyên nhân (3 tầng, tất cả đều phải sửa)

1. `infrastructure/sync/snapshot.go` — `roadmap_stages` chỉ khai **1** FK
   (`path_id`). `deck_id` không có mặt trong `snapshotTable`.
2. `application/sync/snapshot_rows.go` đọc `v(r.Values, "deck_guid")` —
   `deck_guid` **không phải cột của bảng nên không có trong `snapshotColumns`**
   ⇒ luôn ra `""`.
3. `infrastructure/sync/repository_merge_roadmap.go` resolve fail ⇒
   `deckID = nil` ⇒ `UPDATE … deck_id = NULL`.

### Sửa

**a. Cho 1 bảng nhiều FK.** Đổi `snapshotTable{parent, parentOf}` →
`snapshotTable{fks []fkRef}` với `fkRef{column, parent, field}`, `field` là
`fkParent | fkDeck | fkCard`. `roadmap_stages` giờ khai cả `path_id` lẫn
`deck_id`.

**b. Đọc `deck_guid` từ đúng chỗ.** `stageRowsFrom` đọc `r.DeckGUID` (FK đã
resolve trong loader) thay vì tra `Values`.

**c. Phân biệt 3 trạng thái bằng con trỏ.** `app.StageRow.DeckGUID` đổi
`string` → `*string`:

| giá trị | nghĩa | `UpsertStage` |
|---|---|---|
| `nil` | incoming **không mang** thông tin deck | **không đụng cột** `deck_id` |
| `&""` | incoming nói `deck_id IS NULL` | ghi `NULL` |
| `&g` | incoming gắn deck `g` | ghi id của `g` |

`StageRows` (đọc local) luôn trả non-nil — bản local luôn biết `deck_id` của
chính nó.

### Sửa kèm bắt buộc: NULL ≠ `""` trong loader

`readRows` trước đây đổi cột NULL thành chuỗi rỗng. `differs()` so **cả
`len(Values)`** nên `completed_at` NULL ở local vs `""` từ peer ⇒ luôn "khác
nhau" ⇒ luật "2 bản giống hệt → keep" không bao giờ chạy, và tầng hạ tầng ghi
`completed_at = ''` thay vì NULL ⇒ `idx_roadmap_stages_completed`
(`WHERE completed_at IS NOT NULL`) đếm nhầm node chưa xong.

Loader nay **bỏ khoá** của cột NULL thay vì ghi `""`. Cách này giữ đúng dữ
liệu `ipa = ''` của thẻ TMRND: `''` là giá trị thật, khác NULL, vẫn còn trong
map.

`stageValues(st, localDeck)` luôn phát khoá `deck_guid` (khi incoming nil thì
dùng giá trị local) để 2 map luôn cùng khoá.

### Lỗi phát hiện thêm trong lúc sửa (ngoài danh sách Oracle)

`cardMergeRow.IPA` không có tag ⇒ **GORM suy tên cột thành `ip_a`**, cột thật là
`ipa`. Hậu quả không phải lỗi SQL (GORM đọc `SELECT *` rồi map theo tên) mà
`CardRows` **luôn trả `IPA = nil`** ⇒ `cardValues` thiếu khoá `ipa` ⇒
`differs()` luôn true với mọi thẻ có phiên âm. Đã thêm
``gorm:"column:ipa"``. Đây chính là lớp lỗi mà chốt DRY (mục 5) bắt được.

---

## 2. B2 — BLOCKER: hồi sinh tombstone bị chính lần merge ghi đè ⇒ oscillation

### Oracle repro

snapshot có `c-aaa` (live) + `c-zzz` (tombstone) cùng `front`; local có
`c-zzz` tombstone. Sau merge local chỉ còn `c-zzz deleted=1` — thẻ sống của
peer biến mất, trong khi conflict log ghi `recreate-adopted-tombstone`.

### Nguyên nhân

`merge.go` đọc `local` **một lần**; nhánh hồi sinh ghi `guid = in.GUID` cho
row `tombID` nhưng **không cập nhật lại map `local`** ⇒ incoming `c-zzz` vẫn
khớp bản cũ ⇒ `Decide` → `ActionUpdate` ⇒
`UPDATE … guid='c-zzz', deleted=1 WHERE id=<cùng id>` — xoá ngược thẻ vừa hồi
sinh. Đúng cái bẫy mà comment `merge.go:142` đã cảnh báo cho `decks` nhưng
`cards` lặp lại.

### Sửa — 3 lớp

1. **Cập nhật `local` sau MỌI lần ghi, ở CẢ 5 bảng** dùng chung logic này
   (`decks`, `cards`, `roadmap_paths`, `roadmap_stages`, `roadmap_topics`,
   `roadmap_milestones`, `roadmap_resources`, `roadmap_bookmarks`). Ở nhánh
   hồi sinh: `delete(local, tombGUID)` **và** `local[in.GUID] = in` (cả hai
   đều cần — `delete` dọn payload cũ, `local[in.GUID] = in` là để các
   incoming sau thấy trạng thái mới).

2. **`adopted` map** (guid cũ → id thẻ hồi sinh) dùng cho 2 việc:
   - Chặn incoming mang guid cũ tạo thêm row. Bắt buộc phải chặn **trước**
     khi insert vì `ux_cards_deck_front` là **partial** (`WHERE deleted = 0`):
     incoming mang `deleted = 1` thì index KHÔNG chặn, lọt vào DB thành 1 row
     tombstone rác mà user không tạo.
   - Giữ alias cho `reviews`/`notes` của peer mang guid cũ vẫn trỏ đúng thẻ
     (`cardIDs` đọc lại từ DB nên không thấy `c-zzz` nữa).

3. **Guard `LiveCardGUIDByFront` trước insert**: nếu `(deck, front)` đã có 1
   thẻ SỐNG ở local dưới guid khác **và ta đang nắm rõ thẻ đó** ⇒ incoming là
   tombstone/alias cũ, bỏ + ghi log. Đây cũng là đường đi của "2 máy cùng gõ
   tay 1 thẻ" (trước đây `UpsertCard` tự bắt qua `ON CONFLICT`, giờ chặn
   sớm hơn nhưng kết quả với user giống hệt: giữ bản local + ghi log).

### Thứ tự row trong snapshot không được giả định

Loader `SELECT … FROM cards` **không `ORDER BY`** — thứ tự là của peer. Chỉ
khi thẻ sống được xử lý **trước** tombstone thì lỗi mới lộ. Test ở tầng
application chạy **cả hai thứ tự**; merge phải đúng bất kể.

---

## 3. H3 — HIGH: đọc pool BÊN TRONG transaction merge

Oracle không reproduce được (các đường UNIQUE partial chặn trước) nhưng nó
vi phạm luật đã viết ở `repository.go:74-77`.

### Đã sửa

5 hàm Oracle liệt kê + rà toàn bộ package:

| hàm | trước | sau |
|---|---|---|
| `TombstoneCard` | `r.read()` | `r.txCtx(ctx, tx)` |
| `LiveCardGUIDByFront` | `r.read()` | `r.txCtx(ctx, tx)` |
| `CardIDByGUID` | `r.read()` | `r.txCtx(ctx, tx)` |
| `ReviewGUIDs` | `r.read()` | `r.txCtx(ctx, tx)` |
| `NoteGUIDs` | `r.read()` | `r.txCtx(ctx, tx)` |

Interface `application/sync.Repository` đổi chữ ký tương ứng (buộc — đây là
thay đổi application duy nhất ngoài `StageRow`, và thật sự cần).

### Rà toàn bộ package

`r.read()` còn đúng **4 chỗ**, tất cả ngoài `uow.Do` và không phụ thuộc dữ liệu
vừa ghi: `SchemaVersion`, `LastSyncAt`, `ListConflicts`, `CountConflicts`.
Comment trên `read()` ghi rõ danh sách này là **đóng** và nêu hậu quả khi
thêm method mới vào.

---

## 4. H4 — MEDIUM: `txOf` chưa migrate hết

### Sửa

3 chỗ (`sync`, `content`, `practice`) đổi `txOf` → `(*gorm.DB, error)` + thêm
`txCtx(root, ctx, tx)` viết ngắn, đúng như `infrastructure/srs/repository.go:77`.
Cả 3 package giờ có sentinel `errTxMismatch`.

Lý do phải lỗi chứ không rơi về pool: im lặng rơi về pool biến 1 lỗi lập
trình thành **toàn bộ merge ghi ra NGOÀI transaction** — rollback không được,
và test vẫn xanh vì dữ liệu "vẫn tới nơi".

Call site đã cập nhật: `content/repository.go` (4 chỗ), `content/srs_adapter.go`
(4 chỗ), `practice/repository.go` (1 chỗ), `sync` (toàn bộ 3 file merge).

---

## 5. Chốt chặn DRY (B1 chính là triệu chứng diverge)

Oracle chấp nhận quyết định tách SQL merge nhưng yêu cầu chốt: hiện mỗi bảng
có 4 danh sách cột phải khớp tay — `snapshotColumns`, `SELECT` trong
`Upsert*`, struct `pathMergeRow`/`stageMergeRow`…, `domain.Row.Values`.

### `infrastructure/sync/snapshot_columns_test.go` (package `syncinfra` — internal)

Dùng **chính `schema.Parse` của GORM** (cùng bộ phân tích runtime dùng) để
đổi tên field → cột, nên **không cần bảng ánh xạ thứ 5** phải cập nhật tay —
bảng ánh xạ thứ 5 chính là lớp lỗi ta đang cố diệt.

| test | chặn cái gì |
|---|---|
| `Test_merge_row_columns_all_appear_in_snapshot_columns` | cột có trong struct merge-row mà thiếu ở `snapshotColumns` ⇒ loader không đọc ⇒ merge xoá dữ liệu cột đó |
| `Test_fk_columns_are_declared_in_snapshot_tables` | cột FK được phép vắng ở `snapshotColumns` nhưng **bắt buộc** phải khai trong `snapshotTables[].fks` |
| `Test_roadmap_stages_declare_deck_fk` | riêng feature A1 |
| `Test_merge_row_specs_cover_every_merge_table` | thêm bảng merge mà quên đăng ký |
| `Test_snapshot_columns_are_unique_and_ordered_by_load` | cột lặp trong `SELECT`; bảng có `snapshotColumns` mà không có trong `snapshotTables` |

**Cột bị bắt ngay khi viết:** `cardMergeRow.IPA` → `ip_a` (mục 1).

---

## 6. Comment sai (rẻ)

`infrastructure/content/repository_test.go:88-94` — comment nói có
`enable_seqscan = off` nhưng code **không** set, lại lặp lại 2 lần. Đã xoá,
thay bằng mô tả đúng: Postgres vẫn chọn `Bitmap Index Scan` vì
`lower(term) = lower($1)` khớp biểu thức index, đã verify plan thật.

---

## 7. Kết quả test

**589 PASS / 0 FAIL / 0 SKIP** (565 test hàm + 24 subtest), 21/21 package,
chạy **3 lần liên tiếp** — cùng kết quả.

Test mới (18 hàm + 2 subtest):

| nhóm | test |
|---|---|
| B1 | `Test_B1_merge_keeps_stage_deck_id_when_peer_edits_same_stage` (kịch bản Oracle) |
| B1 | `Test_B1_merge_writes_null_deck_id_when_peer_stage_really_has_no_deck` (chiều ngược — chặn đánh đổi sai) |
| B1 | `Test_B1_UpsertStage_nil_deck_guid_keeps_local_deck_id` (gọi thẳng repo, phủ nhánh `nil`) |
| B1 | `Test_B1_stageValues_match_when_both_copies_identical` / `…_degrade_gracefully_when_incoming_lacks_deck` / `…_differ_when_deck_really_changed` / `…_omit_nil_completed_at` |
| B2 | `Test_B2_merge_is_stable_when_peer_snapshot_contains_both_live_card_and_its_tombstone` × **2 thứ tự** |
| B2 | `Test_B2_revived_card_survives_peer_tombstone_in_same_snapshot` (Postgres thật) |
| B2 | `Test_B2_second_merge_does_not_oscillate` (idempotent lần 2) |
| H3 | `Test_H3_TombstoneCard_reads_inside_merge_transaction` |
| H3 | `Test_H3_LiveCardGUIDByFront_reads_inside_transaction` |
| H3 | `Test_H3_ReviewGUIDs_and_NoteGUIDs_read_inside_transaction` |
| H4 | `Test_H4_{sync,content,practice}_repository_rejects_foreign_tx*` |
| DRY | 5 test ở mục 5 |

### Fake phải trung thực mới test được

`application/sync/fake_repo_test.go` sửa 2 chỗ, nếu không thì test xanh nhầm:

- `UpsertCard(found=true)` **xoá khoá cũ mang cùng `id`** — SQL thật là
  `UPDATE cards SET guid=? WHERE id=?` (đổi khoá), fake cũ tự sinh 1 thẻ ma.
- `LiveCardGUIDByFront` **quét thật** thay vì luôn trả "không thấy" — trước đó
  mọi nhánh merge dựa vào nó đều không bao giờ chạy trong unit test.

---

## 8. Mutation-check — sửa ngược thì test FAIL thật

| # | Mutation (đúng lỗi gốc) | Kết quả |
|---|---|---|
| 1 | Bỏ `{"deck_id", TableDecks, fkDeck}` khỏi `snapshotTables[stages].fks` | **FAIL** 3 test: `Test_B1_merge_keeps_stage_deck_id…`, `Test_fk_columns…/roadmap_stages`, `Test_roadmap_stages_declare_deck_fk` |
| 2 | `stageRowsFrom` đọc `v(r.Values, "deck_guid")` (bug gốc) | **FAIL** `Test_B1_merge_keeps_stage_deck_id…` |
| 3b | Gỡ `delete(local, tombGUID)` + `local[in.GUID] = in` (giữ `adopted`) | **FAIL** `Test_B2_merge_is_stable…` ở **cả 2 thứ tự** |
| 4 | 3 hàm đọc `r.read()` thay vì tx (bug H3 gốc) | **FAIL** cả 3 test `Test_H3_*` |
| 5 | `txOf` rơi về pool (bug H4 gốc) | **FAIL** cả 3 test `Test_H4_*` |

**Hai mutation bị lộ trong lúc làm và phải sửa test, đáng ghi lại:**

- **Mutation 3 (bản đầu: gỡ CẢ `local` lẫn `adopted`) không FAIL.** Vì lớp
  guard `LiveCardGUIDByFront` + `adopted` che được. Đã tách thành mutation 3b
  (chỉ gỡ `local`) để đo đúng từng lớp — và 3b FAIL.
- **Mutation 4 (bản đầu) không FAIL.** Test H3 ban đầu dùng `local.Exec` để
  ghi dữ liệu — mà `local.Exec` đi qua **pool** nên commit ngay, pool vẫn thấy.
  Đã sửa test dùng chính `repo.UpsertCard`/`AppendReview`/`AppendNote` để ghi
  **qua `tx`** — đúng đường merge đi. Sau đó mutation 4 FAIL cả 3.

Kịch bản B2 ban đầu cũng dễ xanh nhầm: nếu tombstone của peer **cùng mốc** với
bản local thì `Decide` ra `ActionKeep` (cùng mốc + cả hai vế đều đã xoá ⇒
không ai thắng) và bug không lộ. Đã thêm `tsMid` để tombstone của peer mới hơn
hẳn — đúng như Oracle mô tả.

### Verify khác

- `go build ./...`, `go vet ./...`, `gofmt -l internal` — sạch
- `go list -deps -test` trên 13 package `domain` + `application`: **rỗng** cho
  `gorm.io`, `langapp/internal/infrastructure`, `langapp/internal/platform`,
  `langapp/migrations`
- 3 grep luật DDD trong `internal/{domain,application}`: **0 file** mỗi cái
- App cũ: `ok langapp`

---

## 9. File đã sửa

**`infrastructure/sync`** (được ghi)
- `snapshot.go` — `fkRef`/`[]fkRef` (1 bảng nhiều FK), `roadmap_stages` khai
  `deck_id`, loader bỏ khoá cột NULL
- `repository.go` — `txOf` trả `(*gorm.DB, error)` + `errTxMismatch` +
  `txCtx`; comment `read()` ghi danh sách đóng
- `repository_merge_core.go` — 3 hàm nhận `tx`; tag `gorm:"column:ipa"` cho
  `cardMergeRow.IPA`
- `repository_merge_roadmap.go` — `UpsertStage` 3 nhánh theo `*string`;
  `StageRows` luôn non-nil; `ReviewGUIDs`/`NoteGUIDs` nhận `tx`; `txCtx`
- `snapshot_columns_test.go` — **mới**, 5 test chốt DRY
- `remediation_test.go` — **mới**, B1/B2/H3/H4 trên Postgres thật

**`application/sync`** (sửa *khi thật sự cần*)
- `ports.go` — `StageRow.DeckGUID` → `*string`; `TombstoneCard`,
  `LiveCardGUIDByFront`, `ReviewGUIDs`, `NoteGUIDs` nhận `tx`
- `snapshot_rows.go` — `stageRowsFrom` đọc `r.DeckGUID`
- `merge.go` — `stageValues(st, localDeck)`; cập nhật `local` ở **5** bảng
  (`decks`, `cards`, `roadmap_milestones`, `roadmap_resources`,
  `roadmap_bookmarks` — 3 bảng roadmap kia không cần, xem lý do trong comment
  tại `mergeCards`; số "8" ở bản báo cáo đầu là SAI); `adopted`
  + guard `LiveCardGUIDByFront`; alias cho `mergeReviews`/`mergeNotes`
- `remediation_test.go` — **mới**, 5 test
- `fake_repo_test.go` — `LiveCardGUIDByFront` quét thật; `UpsertCard` xoá khoá cũ
- `merge_test.go` — thêm hằng `tsMid`

**`infrastructure/content`** (được ghi)
- `repository.go`, `srs_adapter.go` — `txOf` trả error + `txCtx`
- `repository_test.go` — xoá comment sai `enable_seqscan`

**`infrastructure/practice`** (được ghi)
- `repository.go` — `txOf` trả error + `txCtx`

**Không đụng**: `infrastructure/{roadmap,srs}/**`, `migrations/**`, `api/*.go`
cũ, `web/`.

---

## 10. SKIPPED

- `BackupPort` vẫn chỉ khai chưa implement (M7) — không liên quan remediation
- Không wire `platform.Container` (M4) — ngoài phạm vi

## 11. Nợ chuyển phase

1. **Loader `SELECT` không `ORDER BY`.** Merge hiện đúng ở mọi thứ tự, nhưng
   log conflict và replay có thể không ổn định giữa 2 lần chạy với cùng dữ
   liệu. Cân nhắc `ORDER BY` tất định ở M7 khi nối `pg_restore`.
2. **`adopted` chỉ sống trong 1 lần merge.** Peer review mang guid cũ ở một
   lần sync **sau** lần hồi sinh sẽ không resolve. Muốn bền vững phải lưu alias
   xuống DB (cột riêng) — thuộc M7 khi bàn về `pg_restore`/schema version.
