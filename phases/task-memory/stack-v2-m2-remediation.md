# STACK-V2 — remediation cổng Oracle M2

> Xong 2026-09-28. 9 finding (1 P0 + 4 P1 + 4 P2), tất cả đã sửa và đã có test
> chứng minh. Baseline: `phases/task-memory/stack-v2-m2.md`.
>
> Oracle đã tái hiện P0 bằng test overlay trên Postgres thật. Mọi test dưới đây
> đều **mutation-check**: sửa ngược lại đúng hành vi cũ thì test phải đỏ
> (bảng ở §2).

## 0. Phạm vi

Chỉ ghi trong: `api/internal/infrastructure/{roadmap,srs}/**`,
`api/internal/application/{roadmap,srs}/**`, `api/internal/domain/roadmap/**`.

**Không** đụng `api/internal/platform/**`, `api/internal/{domain,application,
infrastructure}/{content,practice,insight,sync}/**`, `api/*.go` v1,
`api/migrations/**`, `web/`. Lần này **không có** ngoại lệ ghi nào như M2 gốc
(v1 embed + `platform/migrate_test.go` đã xử lý ở phase trước).

## 1. 9 finding, 1 dòng mỗi cái

| # | Mức | Finding | Cách sửa |
|---|---|---|---|
| **F1** | **P0** | `SetStageStatus`, `SetTopicStatus`, `UpdateStage`, `UpdatePath`, `UpdateTopic`, `UpdateResource`, `UpdateMilestone` (roadmap) + `UpdateCard` (srs) đọc lại row qua `r.db` (pool) thay vì `tx` ⇒ **ghi đúng DB nhưng trả về row CŨ** | Đọc lại qua `txOf`/`txDB` trong **mọi** method `Update*`; `UpdateStageStatus`/`UpdateTopicStatus` đổi signature sang nhận `*Stage`/`*Topic` (pointer) và làm mới chính struct đó — tầng application bỏ 2 vòng đọc thừa mà vẫn có `updated_at` mới |
| **F2** | P1 | `txOf` rơi về pool **im lặng** khi tx sai nguồn ⇒ M3 (`sync` ghi `roadmap_*` + `decks/cards` chung 1 tx) lấy nhầm handle là ghi thoát transaction, merge nửa vời không dấu vết | `txOf` → `(*gorm.DB, error)`; handle sai kiểu / `txHandle` rỗng ⇒ `errTxMismatch`. Tách `txDB(root, ctx, tx)` cho đường ghi. Sửa **mọi** call site ở cả 2 context |
| **F3** | P1 | `UpdateTopic` không xoá được `map_x`/`map_y`: `patch.MapX == nil` nghĩa là *không gửi* ⇒ `{"map_x": null}` không có đường nào set NULL ⇒ M6 không "reset node về auto-layout" | Thêm `TopicPatch.ClearMap bool` (cùng cách `ResourcePatch.ClearURL` sẵn có) → `t.MapX, t.MapY = nil, nil`, repository ghi SQL NULL thật (`*float64` nil trong `Updates(map)`), không phải `0` |
| **F4** | P1 | terrain/direction có 3 bản sao, bản trong `migrations/00004` **không test nào chạm** (DB mới ⇒ 10 câu `UPDATE … WHERE slug = ?` khớp 0 dòng) | Test đọc thẳng `migrations/00004_roadmap_map.sql` bằng `os.ReadFile` (đường dẫn tương đối, **không** sửa file migration), regexp ra 10 bộ `(slug, terrain, direction)`, so với `domain.MapDefaults` |
| **F5** | P1 | Dead code domain + 3 bản sao struct lệch kiểu (`domain.Stage.DeckID int64` vs `app.*.DeckID *int64`; `domain.Status` vs `app.Status = string`; `domain.IsOptional bool` vs `app.IsOptional int`) | Xem §3 |
| **F6** | P2 | `GetPath` ghép layout/state với row bằng **chỉ số** `i`; `points`/`states` xếp theo (position, id) còn `rows` theo thứ tự repository ⇒ M4 thay bằng `dataloadgen` là lệch **im lặng** | Gom `pointByID`/`levelByID` sau khi `ComputeLayout`+`LevelStates`, duyệt cây theo `domain.SortedTopics(dts)` ⇒ ghép theo `dt.ID`, thứ tự trả về cũng luôn là thứ tự màn |
| **F7** | P2 | `DeckInfo.Lang` chết (khai, trả, adapter copy, không nơi dùng) | `DeckInfo` thêm `Name`; `StageView.Deck *DeckRef{ID,Name,Lang}` cho nút "vào /review" của M6; `resolveDeck(ctx, pathLang, deckID)` chặn `deck.lang` lệch `path.language` trong `CreateStage`/`UpdateStage` |
| **F8** | P2 | Code chết: `isTx` (srs infra, comment nói "test dùng" — không test nào dùng), `formatGrade` (srs app) | Xoá cả hai |
| **F9** | P2 | Seed lookup không lọc `deleted` ⇒ user xoá mềm path, seed lần boot sau sẽ insert vào cây đã tombstone | `lookupNaturalKey` trả `(id, tombstoned)`; path/stage/topic có tombstone ⇒ **bỏ qua**, không hồi sinh. Chi tiết lý do bên dưới |

### F9 — vì sao KHÔNG phải `AND deleted = 0` (đây là điểm Oracle ghi sai)

Oracle đề xuất thêm thẳng `AND deleted = 0` vào 4 query lookup. Làm vậy là
**hỏng boot**, vì tombstone vẫn giữ natural key:

- `roadmap_paths.slug` có `ux_roadmap_paths_slug` UNIQUE,
  `roadmap_stages (path_id, slug)` có `ux_roadmap_stages_path_slug` UNIQUE.
  Lọc `deleted = 0` rồi `INSERT` ⇒ **SQLSTATE 23505** ⇒ seed fail mỗi lần boot.
- `roadmap_topics`, `roadmap_milestones`, `roadmap_resources` **không** có
  UNIQUE theo natural key. Lọc `deleted = 0` rồi `INSERT` ⇒ **bản sao trùng
  tiêu đề** (tệ hơn hẳn: không có lỗi nào báo).

Nên `lookupNaturalKey` đọc `SELECT id, deleted …` (1 query) và trả 3 trạng
thái: `(id>0, live)` dùng id · `(0, true)` chỉ có tombstone ⇒ không hồi sinh ·
`(0, false)` natural key thật sự trống ⇒ được insert. 4 tầng xử lý đúng cả 4
tình huống bằng cùng 1 hàm, không tạo bản sao, không đụng UNIQUE.

Quan trọng: bản sửa đầu tiên của tôi **đảo ngược cờ** ở nhánh
milestone/resource (đọc `tombstoned` thành biến tên `taken`) ⇒ seed nhân bản
đúng những thứ cần tránh (+108 resource, +20 milestone). Bắt được ngay vì test
so **snapshot count trước/sau**, không chỉ so "không lỗi".

## 2. Test bắt buộc — và mutation-check đã chạy

Không chỉ viết test: mỗi test dưới đây đã được sửa ngược về hành vi cũ để
chắc chắn nó **đỏ**, rồi restore.

| Finding | Test | Mutation (sửa ngược) | Kết quả |
|---|---|---|---|
| F1 | `Test_update_stage_returns_new_values_within_transaction`, `Test_update_topic_and_resource_and_milestone_return_new_values`, `Test_update_path_returns_new_values`, `Test_set_topic_and_stage_status_return_new_values` | `UpdateStage` đọc lại bằng pool | **FAIL** ✔ |
| F1 | `Test_readback_inside_transaction_sees_new_values` (internal) + `Test_update_card_readback_inside_transaction_sees_new_values` (srs) | như trên | **FAIL** ✔ |
| F2 | `Test_tx_of_rejects_foreign_handle` (cả 2 context), `Test_wrong_tx_handle_returns_error_and_writes_nothing`, `Test_wrong_tx_handle_writes_nothing` (srs) | `txOf` rơi về pool im lặng | **FAIL** ✔ |
| F3 | `Test_clear_map_writes_sql_null_not_zero`, `Test_clear_map_resets_node_to_auto_layout`, `Test_clear_map_alone_is_a_valid_patch` | bỏ nhánh `patch.ClearMap` trong service | **FAIL** ✔ |
| F4 | `Test_migration_map_table_matches_domain_defaults` | `zhMapDefaults[3]` snow→ocean **và** `enMapDefaults[2]` direction right→up | **FAIL** (cả 2) ✔ |
| F6 | `Test_get_path_joins_layout_by_id_when_rows_arrive_shuffled` | decorator đảo thứ tự `ListTopics` | **FAIL** ✔ — lần đầu test **bắt được** chính lỗi còn sót: mình mới sửa phần `rows[i]→byID` mà quên `points[i]`, nên test đỏ và buộc sửa tiếp `points[i]→pointByID` |
| F7 | `Test_create_stage_rejects_deck_with_mismatched_language`, `Test_update_stage_rejects_deck_with_mismatched_language`, `Test_deck_with_free_form_path_language_is_allowed` | bỏ khối so `frozenLang` | **FAIL** ✔ |
| F7 | `Test_get_path_exposes_deck_name_and_lang_for_review_button` | bỏ gán `sv.Deck` | **FAIL** ✔ |
| F9 | `Test_seed_does_not_resurrect_soft_deleted_path`, `Test_seed_does_not_duplicate_soft_deleted_topic_and_milestone` | bỏ nhánh `pathTombstoned` | **FAIL** ✔ |

Ngoài ra, tầng application có 8 test assert **giá trị trả về** (F1) — đúng thứ
Oracle nói test cũ trống vì thiếu. Không chỉ assert DB.

## 3. F5 — đã xoá entity/helper nào

**File `entity.go`: XOÁ HẲN.** Nó chứa 6 struct, trong đó 5 struct chỉ lặp lại
row mà `application/roadmap` đã định nghĩa (và lệch kiểu):

| struct | xử lý | lý do |
|---|---|---|
| `domain.Path` | **xoá** | 0 use ngoài package; `app.Path` là bản thật |
| `domain.Stage` | **xoá** | 0 use; `app.Stage` là bản thật (và khác `DeckID *int64`) |
| `domain.Milestone` | **xoá** | 0 use; `app.Milestone` là bản thật |
| `domain.Resource` | **xoá** | 0 use; `app.Resource` là bản thật |
| `domain.Bookmark` | **xoá** | 0 use (M2 chưa làm CRUD bookmark) |
| `domain.Topic` | **giữ, cắt gọn** → file mới `topic.go` | `ComputeProgress` / `LevelStates` / `ComputeLayout` / `CompletedSince` cần field typed. Cắt về **đúng field policy đọc**: `ID, StageID, Position, Status, CompletedAt, IsOptional, MapX, MapY, Deleted`. Bỏ `Title, Why, Activities, StatusNote, CreatedAt, GUID, UpdatedAt, Resources` — không ai đọc. Doc comment ghi rõ đây là **input của policy, không phải mirror của row** ⇒ chấm dứt 2 bản sao |

Helper đã xoá: `CollectTopics`, `CountByStatus`, `CountStages`, `IsOptionalFlag`,
`ValidKind`, cả block `ResourceKind` (`KindVideo`…`KindChannel`, `AllKinds`).

`ApplyStatus` **xoá** — tầng application đã có `resolveCompletedAt` làm đúng
việc đó *và tốt hơn* (còn xử lý row `done` mà `completed_at` NULL — dữ liệu port
từ v1 — nếu không thì `progress?since=` bỏ sót vĩnh viễn). Test 4 trạng thái
được **chuyển** sang application: `Test_apply_status_covers_all_four_statuses`
(domain, đã xoá) → `Test_resolve_completed_at_covers_all_four_statuses`
(application, giữ nguyên độ phủ 4/4 mà M1 remediation F11 từng thiếu).

`Percent` → **unexported** thành `percent`: tầng application không dùng, chỉ
`ComputeProgress` gọi. Test `Test_percent_floors_and_handles_zero` ở lại (cùng
package nên vẫn chạy).

`Progress.Stages` và `Progress.TopicsLocked` — 2 field không ai đọc ⇒ xoá.
`Progress` giữ lại là **kết quả tính** của policy; DTO trả client là `app.Progress`.

Kết quả: **không còn 2 bản sao của cùng 1 khái niệm.** Seam chuyển đổi duy
nhất là `app.TopicToDomain` (`application/roadmap/service.go`), có comment nói
rõ đừng thêm field "cho đủ".

## 4. Quyết định cho M7 — đã ghi vào doc comment, KHÔNG sửa gì

Oracle yêu cầu ghi cảnh báo, không sửa code. Đã ghi ở **đầu cả 2 file
repository** (`infrastructure/roadmap/repository.go`,
`infrastructure/srs/repository.go`), ngay dưới khối "LUẬT GHI":

> `Omit("updated_at")` **loại `updated_at` khỏi SET kể cả khi map có key
> `updated_at` tường minh** (probe trên SQL thật: `SET "front"=$1` chứ không phải
> `SET "front"=$1,"updated_at"=$2`). Nhờ vậy trigger `langapp_touch_updated_at`
> luôn chạm, và nó CỐ Ý bỏ qua khi `NEW.updated_at IS NOT DISTINCT FROM
> OLD.updated_at`.
>
> ⇒ **M7 KHÔNG được tái dùng `Update*` (hay `UpdateCard`) cho merge peer.**
> Phải có method `Merge*` riêng KHÔNG Omit, ghi `updated_at` tường minh bằng
> mốc của peer, + test assert mốc peer sống sót sau khi trigger chạy. Dùng
> nhầm sẽ ghi đè mốc peer bằng `now()` của máy nhận → LWW chọn nhầm bản cũ là
> bản mới.

## 5. Phát sinh trong lúc làm

### 5.1 Test internal mới phải dùng `platform/testdb`, không tự viết bản sao

Hai test hồi quy cần **internal test** (`package roadmapinfra` / `srsinfra`)
vì phải nắm `txHandle` mới đọc lại được trong transaction — mà đọc ngoài
transaction thì thấy dữ liệu cũ, tức là chính bug P0.

Trước hết tôi copy nguyên xi `openTestDBForInternalTest` (schema + migrate +
cleanup). Kết quả: `go test ./...` **đỏ 2 test** với
`duplicate key value violates unique constraint "pg_extension_name_index"`
(23505) — đúng race `CREATE EXTENSION` đã ghi ở handoff M2. Bản sao không nắm
khoá advisory `MigrateLockKey`.

Đã xoá cả 2 bản sao, thay bằng `testdb.Open(t, ctx)` — helper dùng chung đã
có sẵn (nắm khoá quanh **dựng → migrate → dọn**, và dựng schema trước khi pool
mở kết nối). Bài học cho M3–M7: **dùng `platform/testdb`, đừng viết bản sao.**

Lưu ý cấu trúc: 2 package test cùng thư mục (`X` và `X_test`) được build
thành **1 binary** nên chỉ có **1** `TestMain`. Internal test không thêm
`TestMain` được.

### 5.2 F7 — chỉ so ngôn ngữ khi CẢ HAI vế nằm trong tập đóng băng

`decks.lang` chỉ nhận `zh|en`, nhưng `path.language` là **chuỗi tự do** tối đa
16 ký tự (user tạo path `vi`, `zh-Hans`, …). So khớp trong mọi trường hợp sẽ
chặn oan path hợp lệ. Nên `frozenLang()` map về tập đóng băng và chỉ chặn khi
**cả hai** vế đều rơi vào tập đó. Test chốt cả 2 nhánh:
`Test_deck_with_free_form_path_language_is_allowed` (path `vi` + deck `zh` →
OK).

`resolveDeck` phải chạy **trong** transaction, **sau** khi đã đọc
`path.language` — `CreateStage` sửa theo thứ tự đó, `UpdateStage` đọc thêm
`PathByID(st.PathID)`. Hệ quả: 400 từ validate deck giờ nằm trong tx ⇒ rollback
thay vì return sớm. Chấp nhận được (và đúng hơn: 1 request = 1 giao dịch).

`GetPath` khi `decks == nil` mà stage có `deck_id` trả 500 với message rõ,
**không** im lặng bỏ trống `Deck` (lộ sớm hơn là M6 hiện nút trỏ tới deck
rỗng).

## 6. Verify

| lệnh | kết quả |
|---|---|
| `go test ./...` (Postgres thật, `LANGAPP_TEST_POSTGRES_DSN`) | **14 package, 0 FAIL, 0 SKIP** — xanh ở 13/14; `internal/infrastructure/sync` đang được **lane song song** sửa dở nên build fail, **không phải** phần M2 |
| chạy lại 3 lần liên tiếp (flake) | 3/3 xanh |
| `go vet` (phạm vi M2 + platform + app cũ) | sạch |
| `gofmt -l` (phạm vi M2 + platform + migrations + roadmap_seed) | sạch (0 file) |
| grep luật DDD × 3 | grep 1 (domain thuần) **0 file** · grep 2 (`gorm.io` trong application) **0 file** · grep 3 (không đảo chiều) **hit 2 file `application/content/` — thuộc lane song song M3, đã có từ trước và không nằm trong phạm vi ghi của M2** |
| App cũ `ok langapp` | **PASS** (≈0.94s) |

### Số test (đếm bằng `go test -v | grep -c '^--- PASS'`)

| package | M2 gốc | sau remediation | thêm | file mới |
|---|---|---|---|---|
| `internal/domain/roadmap` | 46 | **39** | **−7** | xoá 6 test của symbol chết + `Test_apply_status_covers_all_four_statuses` (chuyển sang application) |
| `internal/application/roadmap` | 20 | **35** | **+15** | `remediation_test.go` (14) + `Test_resolve_completed_at_covers_all_four_statuses` trong `service_test.go` |
| `internal/application/srs` | 21 | **21** | ±0 | `formatGrade` không có test riêng |
| `internal/infrastructure/roadmap` | 28 | **40** | **+12** | `remediation_test.go` (10) + `internal_test.go` (2) |
| `internal/infrastructure/srs` | 16 | **19** | **+3** | `internal_test.go` (3) |
| **tổng 5 package M2** | **131** | **154** | **+23** (30 mới − 7 xoá) | |

**0 SKIP** toàn bộ 5 package. Suite toàn repo: 14 package.

## 7. Còn tồn đọng (không sửa được trong phạm vi này)

1. **`application/content/{ports,service}.go` chứa `internal/infrastructure`** →
   grep luật DDD #3 không còn 0 file. Thuộc write ownership của **M3**; M2
   không được sửa.
2. **`internal/infrastructure/sync` build fail** lúc chạy verify — lane M3 đang
   sửa dở. Không liên quan M2.
3. **`api/english.go:274` `WHERE term = ? COLLATE NOCASE`** — nợ M1, chưa port
   sang `lower(term)=lower(?)`. Ngoài phạm vi, vẫn ghi ở `.slim/deepwork/stack-v2.md`.

## 8. Cho M4

- `GetPath` đã ghép layout/state theo `ID` và tự sắp theo
  `domain.SortedTopics` ⇒ thay bằng `dataloadgen` **không** làm lệch layout.
  Nhưng `ListResources` vẫn là N+1: M4 phải gom.
- `StageView.Deck *DeckRef{ID,Name,Lang}` sẵn sàng cho nút "vào /review".
- `TopicPatch.ClearMap` sẵn sàng cho gesture "reset node" của M6.
- Wire `roadmapinfra.NewRepository` + `srsinfra.NewRepository` +
  `NewUnitOfWork` + `srsapp.Service` (làm `roadmap.DeckReader` qua
  `FindDeck`) vào `platform/di.go`.
