# M3 — Remediation cuối cho gate (PASS có điều kiện)

Oracle reproduce được bằng `roadmapinfra.Seeder` **thật** trên 2 schema Postgres.
3 mục, 3/3 fix. Không mở rộng phạm vi.

Trạng thái cuối: **604 PASS / 0 FAIL / 0 SKIP**, 21/21 package, **3 lần liên
tiếp**. Mutation-check 3/3 FAIL thật.

---

## 1. BLOCKER — seed roadmap sinh `guid` uuid4 ngẫu nhiên ⇒ 2 máy KHÔNG merge được

### Oracle repro (nguyên văn)

```
local path guid = 4a8041b4-fb72-44ae-9b39-ed72b57059fe
peer  path guid = b091f890-ee06-4181-b8df-fc9fa9fb3092   (≠ nhau)
MERGE THẤT BẠI: resolve path cha của stage … không tìm thấy cha
roadmap_paths guid=b091f890-… → rollback cả merge
```

`infrastructure/roadmap/seed.go` dùng `app.NewGUID()` = `uuid.NewString()` ở 5
chỗ. `content/ports.go:186-190` đã ghi sẵn quy tắc vàng: *"Guid seed PHẢI ỔN ĐỊNH
theo natural key: 2 máy cùng import HSK1 cho ra cùng guid thì merge khớp mà
không nhân đôi"*. Seeder roadmap là chỗ duy nhất vi phạm — hồi quy so với v1
(đã sửa đúng ở `phases/task-memory/v2-gate-remediation.md`).

### Sửa

Cả 5 chỗ → `seedGUID(...)`, **uuid5 theo natural key**, dùng chung namespace với
`SeedDeckGUID`/`SeedCardGUID`:

| node | natural key | `position` |
|---|---|---|
| path | `slug` (`ux_roadmap_paths_slug` UNIQUE) | `0` — cố ý |
| stage | `pathSlug + stageSlug` (`ux_roadmap_stages_path_slug` UNIQUE) | `0` — cố ý |
| milestone | `pathSlug + stageSlug + text` | `mi` |
| topic | `pathSlug + stageSlug + title` | `ti` |
| resource | `pathSlug + stageSlug + topicTitle + title` | `ri` |

**Về `position`** — 2 lý do, ngược nhau, đã ghi trong comment `seedGUID`:

- `path`/`stage` có UNIQUE natural key ⇒ `position = 0` cố ý. Truyền chỉ số
  thật sẽ gắn guid vào *thứ tự file*, mà 2 máy có thể cài bộ file khác nhau.
- milestone/topic/resource **không** có UNIQUE natural key (`lookupNaturalKey`
  nói rõ) ⇒ file seed về lý thuyết có thể chứa 2 mục trùng tiêu đề trong cùng
  1 stage/topic. Nếu chỉ khoá theo tiêu đề thì 2 mục sinh **cùng một guid** ⇒
  `ux_*_guid` UNIQUE làm lần seed sau bị skip ⇒ **mất dữ liệu**. Nối thêm
  `position` để mỗi mục có guid riêng.

Bookmark: **không seed** (Seeder không đụng `roadmap_bookmarks`) ⇒ không sửa.

**Tái dùng, không viết lại**: `seedGUID` gọi `uuid.NewSHA1(uuid.MustParse(
contentapp.SeedGUIDNamespace), …)` — cùng hàm, cùng namespace, cùng kiểu tiền
tố `"<kind>:…"` như `SeedDeckGUID`. **Không** tạo namespace mới cho roadmap.
**Không** import `unicode/norm` (GOROOT máy này thiếu package đó — đã ghi ở bàn
giao M1).

Import `application/content` từ `infrastructure/roadmap` là để lấy đúng
namespace đã khai; cùng kiểu với `infrastructure/content` đã import
`application/roadmap`.

### Test

- `Test_seed_guid_is_stable_across_two_independent_schemas` — 2 schema Postgres
  độc lập, `Seeder.Run()` thật trên cả hai với **cùng FS embed thật**, assert
  mọi guid khớp từng đẳng theo từng bảng
- `Test_B1_seed_two_machines_then_merge_does_not_rollback` — **đúng repro của
  Oracle**: 2 máy seed rồi `Sync` phải không rollback, không conflict, và hội
  tụ còn 1 path; chạy lần 2 idempotent
- `Test_seed_guid_is_uuid5_not_random_v4` — chốt bằng **bit version trong chính
  giá trị** (`uuid.Parse(g).Version() == 5`), không bằng so 2 lần chạy: so
  2 lần chạy chỉ bắt được "không tất định", không bắt được "vẫn ngẫu nhiên
  nhưng cùng namespace"
- `Test_seed_guid_distinguishes_node_kinds_with_same_natural_key` — không
  guid nào dùng cho 2 bảng khác nhau
- `Test_seed_guid_is_stable_across_reruns` — chạy lại seeder không sinh guid mới

---

## 2. `ON CONFLICT DO NOTHING` bị skip ⇒ đếm `merged` nhưng không log conflict

### Oracle repro

`merged={RoadmapPaths:1}`, `conflicts=0`, DB không đổi. Báo cáo merge nói dữ
liệu đã vào trong khi thực tế bị bỏ im lặng ⇒ user tin là đã sync.

Vì sao `ON CONFLICT DO NOTHING` là bắt buộc ở đây: Postgres **hủy cả
transaction** khi 1 câu lệnh vi phạm UNIQUE (khác hẳn SQLite v1 chỉ trả lỗi
cho câu lệnh đó) — xem `insertDeck`. Nên không thể bỏ; chỉ có thể **không được
nuốt hậu quả của việc bị bỏ**.

### Sửa — 2 tầng

**a. Hạ tầng báo cáo việc bị skip.** 6 `Upsert*` của cây roadmap đổi chữ ký
`(int64, error)` → `(id int64, skipped bool, err error)`. `id == 0` sau
`ON CONFLICT DO NOTHING … RETURNING id` chính là "bị bỏ", nên
`return id, id == 0, err`. Nhánh `UPDATE` luôn `skipped = false`.

**b. Application ghi conflict thay vì đếm `merged`.** Thêm
`conflictSink.addSkipped(table, guid, key, now)` ghi `sync_conflicts` với
`Detail` = `insert-skipped-duplicate key="…" — peer có row này nhưng local đã
có row trùng UNIQUE dưới guid khác; dữ liệu peer KHÔNG được ghi`, `Winner` =
local (bản local được giữ). Cả 6 call site: `if skipped { sink.addSkipped(…);
continue }` — **không** `merged.X++`.

Áp cho **TẤT CẢ** `Upsert*` roadmap: paths · stages · milestones · topics ·
resources · bookmarks.

### Test

- `Test_B2_insert_skipped_by_unique_conflict_logs_conflict_and_is_not_counted_merged`
  (Postgres thật) — peer gửi path slug `zh` dưới guid khác ⇒ trùng
  `ux_roadmap_paths_slug`; assert `merged.RoadmapPaths == 0`, đúng 1 conflict
  với `insert-skipped-duplicate`, DB không đổi, path cũ giữ nguyên, và conflict
  **có nằm trong bảng `sync_conflicts`** (không chỉ trả về rồi bố đi)
- `Test_B2_stage_insert_skipped_by_unique_conflict_logs_conflict` — bảng con,
  để không ai tưởng chỉ `paths` mới bị
- `Test_B2_clean_insert_still_counted_merged_without_conflict` — **chống đảo**:
  insert không bị skip thì `merged` vẫn tăng và **không** có conflict giả. Không
  có test này thì cách sửa "luôn ghi conflict cho mọi insert" cũng xanh
- `Test_B2_insert_skipped_is_logged_not_counted_merged` (application, fake) —
  đọc đúng kỳ vọng của DTO `Merged`
- `Test_B2_insert_skipped_guard_applies_to_every_roadmap_table` — 4 subtest
  (stage/topic/resource/bookmark) chốt "cả 6 bảng", không riêng `mergePaths`

### Fake phải mô phỏng được skip

`fakeRepo.upsertAny` trả `skipped`; thêm field `skipInsertGUID` để test kích
hoạt lần INSERT bị bỏ. Trước đó fake **luôn** insert thành công ⇒ nhánh
`id == 0` không bao giờ chạy trong unit test ⇒ bug im lặng sống dai suốt M3.

Dùng `skipInsertGUID` (guid) thay vì dựng chuỗi khoá UNIQUE bằng tay: bản đầu
dùng `conflictKey` dựng `"paths|Slug=zh|GUID=p-peer"` và **test xanh sai** vì
thiếu `Title` — chi tiết mong manh không đáng giữ.

---

## 3. Comment sai (rẻ)

`application/sync` nói *"cập nhật `local` ở 8 bảng"* — thực tế là **5**
(`decks`, `cards`, `roadmap_milestones`, `roadmap_resources`,
`roadmap_bookmarks`).

Oracle xác nhận **không khai thác được** (3 bảng kia có `UNIQUE (guid)` nên
snapshot không thể có guid trùng, và chỉ `UpsertCard` ghi lại `guid` khi update)
— nhưng comment sai là bẫy cho người sau, nên:

- Sửa `mergeCards`: ghi rõ **5 bảng**, kèm 2 lý do cụ thể tại sao 3 bảng kia
  **không** cần (có `UNIQUE (guid)` ⇒ `local` vốn đã đúng; không method nào
  ghi lại `guid` nên không có tình huống "row đổi guid mà map giữ khoá cũ")
- Sửa `phases/task-memory/stack-v2-m3-remediation.md` và
  `.slim/deepwork/stack-v2.md` (ghi "8 bảng" ở cả 2 chỗ)
- `fake_repo_test.go:28` nói "8 bảng" là về 1 kiểu `any` phục vụ 8 struct bảng
  LWW — **đúng số đó**, nhưng dễ nhầm với "8 bảng cập nhật `local`" nên đã ghi
  rõ phân biệt ngay tại chỗ

---

## 4. Kết quả test

**604 PASS / 0 FAIL / 0 SKIP**, 21/21 package, chạy **3 lần liên tiếp** — cùng
kết quả (trước đợt này: 589).

Test mới cho đợt này: 12 hàm + 6 subtest.

## 5. Mutation-check — 3/3 FAIL thật

| # | Mutation (đúng lỗi gốc) | Kết quả |
|---|---|---|
| A | `seedGUID` trả `app.NewGUID()` thay vì uuid5 | **FAIL** 3 test: `Test_seed_guid_is_stable_across_two_independent_schemas`, `Test_seed_guid_is_uuid5_not_random_v4`, và `Test_B1_seed_two_machines_then_merge_does_not_rollback` (repro Oracle) |
| B | Gỡ cả 6 khối `if skipped { … }` ở `merge.go` | **FAIL** 2 test application (1 + 4 subtest) + **2 test Postgres thật** |
| C | Hạ tầng trả `skipped = false` (nuốt `id == 0`) | **FAIL** 2 test Postgres thật |

Mutation B và C là 2 mặt của cùng mục 2 (quyết định ở application vs báo cáo ở
hạ tầng) — cần cả 2 để không có lớp nào nuốt im lặng.

`Test_seed_guid_is_stable_across_reruns` **PASS** dưới mutation A. Đúng và có
chủ ý: với uuid4, lần seed thứ 2 vẫn no-op vì natural key đã có nên guid không
đổi. Test đó bảo vệ lỗi khác (guid lệch mỗi lần boot khi natural key có), không
bảo vệ tính tất định — việc đó do 2 test kia đảm nhiệm.

## 6. Verify khác

- `go build ./...`, `go vet ./...`, `gofmt -l internal` — sạch
- `go list -deps -test` trên 13 package `domain` + `application`: **rỗng** cho
  `gorm.io`, `langapp/internal/infrastructure`, `langapp/internal/platform`,
  `langapp/migrations`
- 3 grep luật DDD trong `internal/{domain,application}`: **0 file** mỗi cái
- App cũ: `ok langapp`

## 7. File đã sửa

**`infrastructure/roadmap`** (được ghi)
- `seed.go` — 5 chỗ `app.NewGUID()` → `seedGUID(kind, position, parts…)`;
  thêm import `contentapp` + `uuid` + `strconv`; thêm hàm `seedGUID` kèm lý do
  chọn natural key / `position`
- `seed_guid_test.go` — **mới**, 4 test guid ổn định

**`infrastructure/sync`** (được ghi)
- `repository_merge_roadmap.go` — 6 `Upsert*` trả `(id, skipped, err)`
- `seed_merge_test.go` — **mới**, merge e2e sau seed (repro Oracle) + 3 test mục 2
- `remediation_test.go` — cập nhật chữ ký `UpsertStage`

**`application/sync`** (sửa *khi thật sự cần*)
- `ports.go` — chữ ký 6 `Upsert*` trong interface + comment luật `skipped`
- `service.go` — thêm `conflictSink.addSkipped`
- `merge.go` — 6 guard `if skipped`; sửa comment "8 bảng" → 5 kèm lý do
- `insert_skipped_test.go` — **mới**, 3 test (1 có 4 subtest)
- `fake_repo_test.go` — `upsertAny` trả `skipped` + field `skipInsertGUID`;
  sửa chú thích "8 bảng"

**Không đụng**: `domain/`, `api/migrations/**`, `api/*.go` cũ, `web/`,
`internal/platform/**`.

## 8. SKIPPED

- `decks`/`cards` chưa trả `skipped` → **nợ biết, không sửa** (Oracle giới hạn
  mục 2 ở roadmap). Mức độ: `mergeDecks` có cùng bệnh nhưng cần `guid` trùng mà
  `local` không có — `ux_decks_guid` UNIQUE nên nếu xảy ra thì insert bị skip
  im lặng. `cards` đã có guard `LiveCardGUIDByFront` + `dupKept` log conflict
  riêng nên không hở. Nên: **M4/M7 nên đồng bộ chữ ký cho `UpsertDeck`** khi
  đang đụng `sync` cho DI.
- Wire `roadmap.Seeder` vào `di.go` — **M4 được phép**, xem mục 9.

## 9. Bàn giao cho M4

**`roadmap.Seeder` được phép gắn vào `di.go`.** Cụ thể:

- `roadmapinfra.NewSeeder(db, log, nowFn)` đã có sẵn, dùng `embed.FS` thật từ
  package `roadmap_seed` — dựng y hệt `newSeeder` trong test.
- Gọi `Run(ctx)` **1 lần lúc boot**, thứ tự: migrate → seed → build DI graph.
  `Run` tự bọc 1 transaction và idempotent (chạy lại không ghi, không đụng
  UNIQUE) nên gọi lúc nào cũng được, nhưng đặt trước DI thì các service đọc
  `roadmap_paths` trong test/`ViewBox` thấy dữ liệu seed.
- **Không** đưa Seeder vào `platform.Container` dưới tên "repository" — nó là
  **bootstrap một lần**, không phải dependency runtime. Nếu M4 muốn giữ 1
  entry point duy nhất thì để `Container.Seed(ctx) error` và **không** đưa vào
  interface của service nào.
- Seed chạy trên pool `local` (giống mọi repository khác), **không** cần port
  mới — Seeder đã tự viết SQL, không đi qua `application/roadmap` use case.

## 10. Nợ chuyển M7

1. **`adopted` chỉ sống trong 1 lần merge** (mở từ remediation trước) — review
   peer mang guid cũ ở lần sync **sau** lần hồi sinh sẽ không resolve. Muốn bền
   vững phải lưu alias xuống DB.
2. Loader `SELECT` không `ORDER BY` — merge đúng mọi thứ tự, nhưng log conflict
   và replay có thể không ổn định giữa 2 lần chạy.
3. **`decks` chưa có guard `skipped`** (mục 8) — đồng bộ khi đụng `sync` cho DI.
