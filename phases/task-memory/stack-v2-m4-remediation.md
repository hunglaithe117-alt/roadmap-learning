# STACK-V2 — M4 Remediation (7 finding cổng Oracle)

Ngày: 28/09/2026 · Phase: remediation của M4 (Gin + GraphQL + audio-service gRPC)
Lệnh: sửa 7 finding cổng Oracle, đo lại N+1 bằng bộ đếm GORM logger hook thật.

Kết quả: **7/7 fix**, **+26 test** (693 tổng, M4 gốc có 85 → M4-remediation có 111),
`go test ./...` **26 package ok, 0 FAIL, 0 SKIP × 3 lần liên tiếp**, schema rác **0**.

---

## 0. Phát hiện thêm NGOÀI 7 finding (F8) — đã sửa, cần Oracle biết

Bắt buộc phải sửa vì test F6 mà Oracle yêu cầu **không thể xanh ổn định** khi còn
lỗi này: test đo "số SQL hằng theo số stage" dùng dữ liệu nhiều stage, và nó
FAIL ngẫu nhiên ~2/3 lần.

### F8 — `path.stages` trả thứ tự NGẪU NHIÊN (duyệt Go map)

`application/roadmap/service.go: StageTreeByPathIDs` gom stage theo path từ
`StageTreeByIDs` (trả `map[int64][]StageView` khoá **stageID**) bằng:

```go
for _, views := range byStage {          // ← duyệt MAP
    for _, sv := range views {
        byPath[sv.PathID] = append(byPath[sv.PathID], sv)
    }
}
```

Go **randomize thứ tự duyệt map** ⇒ `path.stages` ra thứ tự bất kỳ mỗi request.

Hậu quả nghiệp vụ: bản đồ roadmap vẽ theo `position` nên node nhảy lung tung
giữa các lần tải; `LevelState` (CURRENT/LOCKED/DONE) được `decorateTopics` tính
theo thứ tự topic trong stage nên "topic đầu mỗi stage = CURRENT" áp vào **sai
topic**. Không lỗi nào được trả về cho client — im lặng.

Vì sao M4 không bắt: seed M4 chỉ có **1 stage**; 1 phần tử thì thứ tự không thể
sai. Cùng lý do khiến 2 test N+1 M4 có tồn tại không phát hiện F1: **test seed
1 phần tử cho biến cần so sánh**.

Sửa: `sort.SliceStable` theo `(position, id)` cho từng `byPath[id]` — đúng thứ tự
mà repository thật đã `ORDER BY path_id, position, id`; chỉ bỏ lớp trung gian "duyệt
map" mà không đổi hợp đồng.

`GetPath` (`service.go:1001`) duyệt slice `stages` đã sắp nên **không** dính; test
`Test_get_path_and_stage_tree_agree_on_stage_order` ràng buộc 2 đường đọc cây
phải cho cùng thứ tự, để không lệch nhau âm thầm.

**Sửa kèm (test double):** `fakeRepo.ListStages` / `ListResources` / `ListMilestones`
trả **thứ tự map** trong khi repository thật có `ORDER BY`. Đã thêm `sort` cho
3 hàm để fake giữ đúng hợp đồng của repository — nếu không, test viết theo giả
định "thứ tự là của SQL" sẽ xanh ở fake và đỏ ở production (hoặc ngược lại), và
chạy ngẫu nhiên. Đây là nguyên nhân trực tiếp khiến run 3/3 của lượt verify đầu
tiên đỏ.

Ngoài phạm vi ghi (Oracle đã cho phép: `api/internal/application/roadmap/**`).
`internal/infrastructure/roadmap` **không** cần sửa — repository đã `ORDER BY`
đúng từ đầu.

---

## 1. F1 — `paths` trả tiến độ gộp cho MỌI path (CHẶN M5/M6)

**Sửa** — `application/roadmap/service.go`:

- `ListPaths` (`:120`) đổi `ProgressByIDs(ctx, ids, nil)` → `ProgressByPathIDs(ctx, ids)`,
  rồi lấy `prog[p.ID]` cho từng dòng. Batch 2 lệnh, **không** lặp N lần.
- Doc của `ProgressByIDs` viết lại: nói rõ nó **chỉ đúng cho 1 path** (`Progress(slug)`
  + dataloader `Path.progress`); dùng cho danh sách là sai.
- Doc của `ProgressByPathIDs` viết lại: đây là hình dạng `ListPaths` + `Path.progress`
  của query list cần.

**Test** — `application/roadmap/listpaths_test.go` (4 test):

| Test | Bắt được gì |
|---|---|
| `Test_list_paths_gives_each_path_its_own_progress` | Hồi quy. Data chọn **cố ý** để hồi quy ra kết quả khác hẳn: path A 1 topic xong (100%), path B 3 topic chưa làm (0%); số GỘP của hồi quy = 1/4 = **25% cho cả hai**. Test còn assert `NotEqual(25, …)` để 25 là dấu hiệu đặc biệt. |
| `Test_list_paths_required_excludes_optional_per_path` | `topics_required` loại optional (A1) — tính **riêng** cho từng path. |
| `Test_list_paths_includes_path_without_stage_with_zero_progress` | Path không có stage vẫn có trong list với số 0 (dataloader cần key có mặt). |
| `Test_progress_by_path_ids_matches_single_path_progress_by_ids` | `ProgressByIDs` với 1 path = `ProgressByPathIDs` của path đó. |

Về mặt kỹ thuật: lần viết đầu dùng 2 path khác số topic nhưng **cùng 0%** (chưa
đánh dấu topic nào xong) ⇒ `NotEqual(percent)` bằng 0 = 0 nên **không bắt được
gì**. Sửa bằng cách cho 2 path khác ở **cả mẫu số lẫn tử số**. Ghi lại vì đây là
bẫy của chính loại test này.

**Test tầng transport** — `transport/graphql/resolver_test.go` (2 test), qua đúng
`graph/client`:

- `Test_paths_list_returns_distinct_summary_per_path` — `paths { summary { percent topicsTotal topicsDone } }`
  = 100/1/1 và 0/3/0.
- `Test_paths_list_progress_field_is_per_path_not_aggregate` — `paths { path { progress { … } } }`
  (loader `progress`, khác đường đọc) = 1 và 3, `NotEqual(4)`.

**Mutation-check F1** — sửa ngược `ListPaths` về số gộp: **3/4 test đỏ**
(`gives_each_path_its_own_progress`, `required_excludes_optional`,
`includes_path_without_stage`).

---

## 2. F2 — `toUserError` lộ lỗi GORM/Postgres ra client (BẢO MẬT)

**Sửa** — `transport/graphql/errors.go`, `toUserError` chia 3 nhóm:

1. Lỗi tham số TRANSPORT (`statusError`) → message nguyên văn.
2. Lỗi NGHIỆP VỤ (`apperr.IsBusiness`, status 400/404/409) → message **nguyên văn**,
   không dịch lại (hợp đồng M4 chốt; dịch lại ở transport là cách chắc chắn lệch).
3. Lỗi HỆ THỐNG → `slog.Error` server-side (chỉ ghi `err.Error()`, **không** trả cho
   client) + trả `{Message: "lỗi hệ thống", Code: INTERNAL}`.

Thêm `internalMessage`, `isBusinessStatus`, `logSystemError`.

**Test** — `transport/graphql/errors_test.go` (6 test). Ép lỗi hệ thống **thật**
bằng `DROP TABLE reviews` (không dùng lỗi giả: lỗi thật mới mang SQLSTATE/tên
bảng/tên cột cần kiểm — lỗi giả tự cho ta chọn message sạch rồi tự khẳng định
là sạch, tức test rỗng).

| Test | Khẳng định |
|---|---|
| `Test_system_error_from_resolver_does_not_leak_sql_to_client` | `Query.stats` ⇒ message **không** chứa `SQLSTATE`/`42P01`/`relation`/`reviews`/`SELECT`/`pg_`/`ERROR:`, chỉ chứa `lỗi hệ thống` + `INTERNAL`, và `stats` = null (không trả payload dở dang). |
| `Test_system_error_in_mutation_does_not_leak_sql` | Cùng ở **mutation**. Phải tạo thẻ THẬT: `RecordReview` kiểm thẻ tồn tại trước rồi mới chạm `reviews`, nên `cardId` giả sẽ dừng ở 404 và test "xanh" mà không chứng minh gì. |
| `Test_business_error_message_is_not_masked_by_the_system_error_guard` | 409 vẫn ra tiếng Việt nguyên văn **khi DB đang hỏng** — che lỗi hệ thống không được nuốt lỗi nghiệp vụ xảy ra cùng lúc. |
| `Test_transport_error_message_is_not_masked_either` | `since` sai định dạng vẫn nói rõ `YYYY-MM-DD`. |
| `Test_system_and_business_errors_have_different_codes` | `INTERNAL` ≠ `CONFLICT` — client phải phân biệt "bạn gửi sai" với "server hỏng". |
| `Test_no_resolver_leaks_driver_detail_after_table_drop` | Quét **12 resolver** bằng `RawPost` (đọc raw, không `MustPost` giải mã) — chỗ dễ sót nhất là `extensions`, mà `MustPost` không quét. |

**Mutation-check F2** — bỏ guard: **3 test đỏ** (query, mutation, quét 12 resolver).

**Bẫy đã tránh:** bản đầu của `Test_system_and_business_errors_have_different_codes`
gọi `newHarness` **hai lần** trong một test. `testdb.Acquire` nắm `pg_advisory_lock`
mà lock **không xếp hồng giữa 2 session** ⇒ session 2 chờ vô hạn, test bị `timeout`
giết ⇒ `t.Cleanup` không chạy ⇒ **rò schema**. Viết lại thành 1 harness với thứ tự
nghiệp vụ-trước, hệ thống-sau. (Đây là bẫy đã ghi ở `testdb.Acquire`; đáng đưa vào
checklist review.)

---

## 3. F3 — `/api/health` không bao giờ trả `"status":"ok"` ⇒ `app-v2` unhealthy vĩnh viễn

**Nguyên nhân đúng như Oracle chỉ:** `Engine.EngineInfo` + `Engine.Real` là 2
**bản sao tĩnh** lúc boot, không ai gán lại; nguồn sự thật là `engineCache` bên
trong adapter (đổi sau request audio đầu tiên). `main.go` truyền
`&c.Audio.EngineInfo` — bản sao vĩnh viễn.

**Sửa** — bỏ hẳn 2 trường trạng thái, không sửa bằng cách gán lại:

- `infrastructure/audio/engine.go`: xoá field `EngineInfo` + `Real` khỏi `Engine`.
  Thêm `TTSEngine()`, `STTEngine()`, `IsReal()`, `EngineInfo()` — tất cả đọc thẳng
  adapter. Không còn bản sao nào có thể lệch.
- `transport/http/router.go`: `Options.Audio` đổi `*audiodomain.EngineInfo` →
  `func() (name string, real bool)` (đọc **tức thì**).
- `transport/http/health.go`: `healthHandler` nhận func; `degraded` khi `!real`.
- `cmd/langapp/main.go:164`: truyền func đọc `c.Audio.TTSEngine()` + `IsReal()`;
  log lúc boot dùng `EngineInfo().Name` / `IsReal()`.
- `transport/http/audio.go`: `info := engine.Info()` **chuyển vào trong closure**
  (2 chỗ: tts `:53`, stt `:109`) — trước nó bắt ngoài closure nên header `X-Engine`
  ghim "audio-service" mãi.

**Quyết định healthcheck (Oracle yêu cầu chọn 1 + ghi rõ lý do) — làm CẢ HAI:**

1. **Nối `engineCache` thật** (ưu tiên theo Oracle). Stub đọc `Real=false` ngay
   lúc dựng nên `degraded` là **kết quả đúng** từ lần health đầu — trước đó nó là
   ngẫu nhiên.
2. **Healthcheck Docker chấp nhận `ok` lẫn `degraded`** (nhánh "nếu kèm theo" của
   Oracle). `AUDIO_GRPC_ADDR` rỗng là **cấu hình được hỗ trợ chính thức** (mặc định
   trong `Dockerfile`), nên coi nó là chết là sai: `app-v2` unhealthy vĩnh viễn và
   M7 thêm service nào `depends_on: service_healthy` là treo. **Postgres chết vẫn
   trả 503** ⇒ healthcheck vẫn bắt được thứ duy sự thực chết người.

Không đổi `/api/health` trả `ok` cho stub: `status` là **thông tin cho người vận
hành**, healthcheck là câu hỏi **"app còn phục vụ được không"**. Hai câu hỏi khác
nhau nên hai chỗ khác nhau.

Sửa `Dockerfile` (HEALTHCHECK) + `docker-compose.yml` (`app-v2`).
`docker-compose.yml` healthcheck của `app` **v1** (`:40`) **không** đụng — app v1
luôn trả `ok` khi DB sống.

**Test** — `transport/http/health_engine_test.go` (7 test):

| Test | Khẳng định |
|---|---|
| `Test_x_engine_header_follows_engine_after_first_request` | Header đọc lại **mỗi** request. Dùng `countingTTS` đổi tên sau lần gọi đầu, mô phỏng đúng `engineCache` (bind **sau** RPC). Vì `Info()` đọc **trước** lệnh gọi (để gắn header kể cả khi lỗi), tên thật xuất hiện ở request **kế tiếp** — và đó là điều cần khẳng định: có đọc lại, không ghim 1 bản. |
| `Test_x_engine_header_always_reports_stub_for_stub_engine` | Stub ⇒ header "stub" ở **mọi** request (UI dựa vào đó hiện badge "kết quả không thật"). |
| `Test_health_reflects_engine_state_read_live_not_snapshot` | `degraded` → request TTS → `ok`. |
| `Test_health_is_ok_when_audio_provider_reports_real_engine` | Engine thật ⇒ `ok`. |
| `Test_health_degrades_when_stub_engine_reports_not_real` | `degraded` **vẫn 200** (app dùng được; 503 sẽ khiến Docker restart vô ích). |
| `Test_health_returns_503_regardless_of_audio_state_when_db_down` | DB chết ⇒ 503 dù audio thật — thứ tự kiểm phải DB trước. |
| `Test_server_base_context_is_the_process_context` | **F4**, xem dưới. |

**Mutation-check F3** — cho health coi như luôn stub: **2 test đỏ**
(`reflects_engine_state_read_live`, `is_ok_when_audio_provider_reports_real_engine`).

---

## 4. F4 — `BaseContext` không nối context của process (doc nói sai)

**Sửa** — `cmd/langapp/main.go`: `buildHTTP(c)` → `buildHTTP(c, ctx)`; truyền `ctx`
thật (đã bám `signal.NotifyContext`) vào `router.Server(…, ctx)`. Nhận SIGTERM ⇒
`BaseContext` bị huỷ theo, mọi lệnh DB và gọi gRPC của request đang chạy dừng ngay
thay vì chờ `Shutdown` hết 15s rồi cắt ngang giữa `/api/tts`.

**Test** — `Test_server_base_context_is_the_process_context`: không spin
`http.Server` thật (cần listen thật) nên test ở mức hành vi: `BaseContext` trả
đúng context đã truyền, và context đó **bị huỷ** khi cancel.

**Mutation-check F4** — cho `Router.Server` bỏ qua `baseCtx` (trả
`context.Background()`): test đỏ đúng message.

---

## 5. F5 — bảng phồn→giản nhân bản 2 nơi; lập luận "ranh giới service" SAI

**Lập luận cũ bị bác bỏ:** "tiến trình audio không được import tầng trong của app"
là **quy ước, không phải ràng buộc kỹ thuật**. `services/audio-service` cùng module
`langapp`; quy tắc `internal/` của Go chỉ chặn import từ **NGOÀI** module, không
có tác dụng gì với `services/`. Comment ở `infrastructure/audio/stub.go` nói vậy là
**sai sự thật** — và nó chính là lý do suy ra `SineWAV` phải nhân bản.

Hậu quả đo được: `practice.DiffAgainstSample` (`application/practice/service.go:175`)
gọi `domain.ToSimplified`, audio-service dùng bản riêng ⇒ **2 bảng 100 mục rời
nhau, không test nào ràng buộc** ⇒ lệch bảng là chấm sai oan.

**Sửa** — `services/audio-service/simplify.go`: xoá `var tradToSimp` (100 mục) +
2 bản `toSimplified`/`hasTraditional` cục bộ, thay bằng alias sang
`content.ToSimplified` / `content.HasTraditional` (hàm thuần Go, chỉ có bảng rune
tự chứa, **không** dùng `unicode/norm` vì GOROOT máy build thiếu package đó —
STACK-V2 §8). Comment mới ghi rõ lập luận cũ **SAI** và vì sao.

Sửa comment sai ở `infrastructure/audio/stub.go`.

**Quy tắc giờ có 1 nguồn duy nhất:** `internal/domain/content`.

**Test** — `services/audio-service/audio_test.go`, `Test_audio_service_shares_the_domain_simplify_table`:
quét **TOÀN BỘ** khoảng CJK `U+4E00–U+9FFF` (20992 rune), đối chiếu cả
`toSimplified` lẫn `hasTraditional` với `domain/content`. Quét cả khoảng thay vì
liệt kê mục là chủ ý: danh sách ký tự viết tay trong test chỉ chứng minh được mấy
chữ đã biết, còn quét cả khoảng thì **bất kỳ** mục nào lệch — kể cả mục thêm về
sau — đều làm đỏ. ~21k vòng lặp, không cần DB, chạy 10ms.

Không thêm hàm liệt kê bảng vào `domain/content` vì lượt này không được sửa tầng
`domain` (ngoài phạm vi ghi) — đối chiếu **hành vi** là cách ràng buộc đúng mà
không cần mở API mới.

**Mutation-check F5** — thay bằng bảng riêng chỉ 4 mục: **2 test đỏ**, log ra
`toSimplified("來") = "來", domain/content cho "来" ⇒ lệch bảng`.

---

## 6. F6 — test N+1 mới đo 1 chiều (khoảng trống test, không phải bug)

**Sửa** — `transport/graphql/roadmap_tree_test.go`:

- Thêm `seedPathWithStages(t, h, slug, n)`: 1 path + **n stage**, mỗi stage 1 topic
  (2 resource), **mọi stage gắn chung 1 deck thật**, stage đầu 2 milestone, các
  stage sau 1.
- Thêm `Test_roadmap_tree_sql_statement_count_is_constant_as_stages_grow`:
  **1 stage = 9 statement, 5 stage = 9 statement** (chênh 0).

Ba nhánh trước đây **không được đo**: `Stage.deck` (loader `deckRef` chưa từng chạy
với key thật), `ListMilestonesByStageIDs` (luôn chạy với mảng rỗng ⇒ "hằng số" ấy
là hằng 0 — đo cái không có gì đo), và **số stage tăng** (ai đổi tầng stage sang
đọc từng stage thì 2 test M4 vẫn xanh).

Quyết định thiết kế: **gắn deck cho MỌI stage**, không chỉ stage đầu. Nếu chỉ
1 stage có deck thì một truy vấn deck-theo-từng-stage vẫn ra đúng 1 statement ⇒
test không bắt được. Hình dữ liệu này cũng sát thực tế (các stage của một path
thường dùng chung bộ thẻ).

Test chốt dữ liệu thật sự được đọc trước khi so số statement: n stage, mỗi stage
`deck != nil` và đúng số milestone — "số SQL hằng" trên cây rỗng là vô nghĩa.

**Số đo cuối (bộ đếm GORM logger hook thật):**

| Query | Kích thước nhỏ | Kích thước lớn |
|---|---|---|
| cây 5 tầng (số topic tăng) | 1 topic = **8** | 51 topic = **8** |
| cây 5 tầng (số **stage** tăng, có deck + milestone) | 1 stage = **9** | 5 stage = **9** |
| `paths { path { stages { topics } } }` | 1 path = **7** | 10 path = **7** |

**Mutation-check F6** — cho `Stage.deck` bypass loader, gọi thẳng
`SRS.FindDecks` mỗi stage: **1 stage = 9, 5 stage = 13** ⇒ test đỏ, chênh đúng 4
(5 stage − 1 query dùng chung).

---

## 7. F7 — vá testdb để lại lỗi cũ cùng loại

**Sửa** — `internal/platform/testdb/testdb.go`:

- `openSchema` (`~:169`) thay `s.drop(schema, nil)` bằng `s.dropSchema(ctx, schema)`.
  `drop` **return sớm khi `db == nil`** nên lệnh drop không bao giờ chạy, trong khi
  schema đã `CREATE` ở dòng trên vẫn còn — đúng cơ chế để lại 61 schema rác ở M1/M2.
- Thêm `dropSchema(ctx, schema)`: dùng conn của **khoá advisory** (không cần pool
  test) nên dọn được cả khi `openTestDB` đã fail. Nhánh đó schema chỉ rỗng (chưa
  chạy migration ⇒ chưa có `pg_trgm` nào cài vào nó) nên **không cần gỡ extension**.

**Test** — `internal/platform/testdb/testdb_leak_test.go` (3 test):

| Test | Khẳng định |
|---|---|
| `Test_dropSchema_removes_a_schema_that_never_got_a_pool` | Đúng shape F7: `CREATE SCHEMA` bằng conn của khoá, **không có `*gorm.DB` nào** ⇒ schema biến mất. Dọn 2 lần phải **báo lỗi** (không phải thành công âm thầm). |
| `Test_no_schema_leaks_after_a_full_suite_style_cycle` | 5 vòng × 2 schema = 10 lần dựng+dọn ⇒ về đúng baseline. |
| `Test_dropSchema_reports_error_when_there_is_no_lock_conn` | `conn` rỗng ⇒ báo lỗi, không panic, không "thành công âm thầm" (im lặng là quay về đúng lỗi F7). |

Bẫy `t.Cleanup` LIFO: schema được `OpenSchema` dọn trong `t.Cleanup` **của chính
test đó**, nên assertion phải đăng ký **trước** mọi `OpenSchema` thì mới chạy
sau cùng. Đếm trong thân test sẽ thấy đủ 10 schema "đang chờ dọn" và báo rò oan
(lần viết đầu đúng như vậy).

**Đếm schema rác trên DB dev:**

| Mốc | Số schema `t_test_*` / `t_peer_*` |
|---|---|
| Trước lượt này (baseline, tích luỹ từ M1/M2) | **25** |
| Sau khi dọn tay + chạy full suite 3 lần | **0** |
| Sau mỗi 1 trong 3 lần verify | **0 → 0** |

Extension còn sót: chỉ `plpgsql -> pg_catalog` (mặc định của Postgres).

---

## 8. Verify

```
docker compose up -d postgres
LANGAPP_TEST_POSTGRES_DSN=postgres://langapp:langapp@localhost:5432/langapp?sslmode=disable \
  go test -count=1 ./...
```

| Hạng mục | Kết quả |
|---|---|
| `go test -count=1 ./...` | **26 package ok, 0 FAIL, 0 SKIP — 3 lần liên tiếp** |
| Schema rác sau mỗi lần | **0 → 0** (3/3) |
| `go build ./...` | sạch (3 binary: `langapp`, `audio-service`, app v1) |
| `go vet ./...` | sạch |
| `gofmt -l api` | 2 file: `chinese_test.go`, `reader.go` — **drift có sẵn ở app v1**, được phép giữ nguyên |
| 3 grep luật DDD (đúng lệnh M4 §7.1) | **0 file** mỗi cái (exit 1) |
| `go list -deps -test` domain+application | **rỗng** cho `gorm.io`/`gin-gonic`/`grpc`/`infrastructure`/`platform`/`migrate` |
| App cũ `go test .` | **`ok langapp 0.909s`** |
| `go generate ./...` | **SKIP** — không đụng `.proto`/`.graphqls`/`gqlgen.yml`; không file generated nào bị sửa |
| Mutation-check | **8/8 finding đều có test đỏ khi sửa ngược** |

**Số test:** 693 hàm `Test*` trong repo (M4 gốc 85 → **M4-remediation +26** = 111).
Bộ đếm: `errors_test.go` 6 · `listpaths_test.go` 6 · `health_engine_test.go` 7 ·
`roadmap_tree_test.go` 4 (1 mới) · `testdb_leak_test.go` 3.

**Lưu ý về grep DDD:** nếu grep thẳng `gorm.io|gin-gonic|grpc` vào
`internal/domain` **+** `internal/application` thì hit 3 file
(`domain/audio/value_object.go:3`, `domain/audio/value_object_test.go:48`,
`application/practice/ports.go:10`) — nhưng cả 3 là **văn xuôi trong comment**
("hiện thực nằm ở transport/grpc M4"), **không phải import**, và là file **có
sẵn từ M4 gốc**, không do lượt này sửa. Import block của chúng: 0 hit.
`go list -deps -test` (chỉ thấy import thật) = 0. Lệnh gate chuẩn M4 §7.1
(`application` chỉ grep `gorm.io`) cho **0 file** cả 3 cái.

---

## 9. Backlog phải ghi (nợ M5–M7)

### 9.1 Bookmark CRUD → **M6** (không phải M7)

`application/roadmap` **không có** use case CRUD bookmark nên GraphQL không có
field bookmark (M4 đã quyết định bỏ: thêm field lúc đó là khai 1 field luôn trả
null). M6 cần đủ **3 việc**:

1. **Repository CRUD** — `internal/infrastructure/roadmap` (hiện **không** có
   `bookmarks.go`).
2. **Use case** — `application/roadmap`: list theo `status`/`tag`, create, update,
   **delete mềm** (`deleted = 0` giống mọi bảng khác).
3. **Schema GraphQL** — type `Bookmark`, enum `BookmarkStatus`, `Query.bookmarks`
   (+ mutation nếu UI cần sửa). Cần chạy `go generate ./...` (cần `buf` +
   `protoc-gen-go` + `protoc-gen-go-grpc` trong PATH).

### 9.2 `SineWAV` nhân bản 3 lần → M5–M7

3 bản: `infrastructure/audio/stub.go`, `services/audio-service/synthesizer.go`,
`api/audio.go` (app v1). **Không gộp trong lượt này** (ngoài phạm vi) vì nó là sinh
WAV thuần, không phải quy tắc nghiệp vụ — chênh lệch không sinh hành vi sai, chỉ
tốn vài dòng. Nhưng **lập luận "audio-service không import được package trong của
app" là SAI** (F5), nên khi gộp thì gộp vì lý do thật, và gộp luôn `tradToSimp` đã
gộp ở F5. Bản ở `api/audio.go` chỉ xoá được khi app v1 bị thay (M5).

### 9.3 `GetPath` còn N+1 theo stage qua `attachDeck` + 2 đường đọc cây → M5

`service.go:1287` (`attachDeck`) gọi `s.decks.Find(ctx, *sv.DeckID)` **mỗi stage**
⇒ `GetPath` vẫn là N+1 ở tầng 2. `StageTreeByPathIDs` (đường dataloader) không dính
vì `deckRef` là loader batch.

`GetPath` hiện **chỉ còn test dùng** (GraphQL bỏ qua) ⇒ M5 nên chọn 1 trong hai:

- **xoá hẳn** `GetPath` (đã có `StageTreeByPathIDs` + loader `deckRef` làm đúng
  việc đó), hoặc
- **đổi sang batch** (`attachDeck` nhận map deck đã gom).

**Đừng để 2 đường đọc cây khác nhau.** `Test_get_path_and_stage_tree_agree_on_stage_order`
(đã thêm ở F8) ràng buộc chúng cho cùng thứ tự stage, nhưng chỉ bắt được thứ tự —
không bắt được khác biệt về số statement hay hành vi `Deck`. Nếu M5 xoá `GetPath`
thì xoá luôn test đó; nếu giữ thì phải cho nó cùng hình dạng batch.

### 9.4 `fakeRepo` đã thêm `sort` cho `ListStages`/`ListResources`/`ListMilestones`

Đã sửa trong lượt này (F8). Ghi lại vì đây là **tiền lệ**: fake phải giữ đúng hợp
đồng `ORDER BY` của repository thật, nếu không test sẽ xanh ở fake và đỏ ở
production (hoặc ngược lại) — chạy ngẫu nhiên theo `-count`. Fake hiện còn dùng
`MaxStagePosition` trả cứng `-1` (mọi stage `position = 0`) nên test phải truyền
`Position` tường khi cần kiểm thứ tự.

### 9.5 Bẫy review: `Acquire` không xếp hồng

`testdb.Acquire` nắm `pg_advisory_lock`; lock **không xếp hồng giữa 2 session**.
Gọi `newHarness`/`testdb.Open` **hai lần trong một test** ⇒ session 2 chờ vô hạn
(test bị `timeout` giết ⇒ `t.Cleanup` không chạy ⇒ **rò schema**). Phải `Acquire`
một lần rồi `Session.OpenSchema` nhiều lần. Đã trúng 1 lần trong lượt này.

---

## 10. File đã sửa

**Production (7 finding):**
- `api/internal/application/roadmap/service.go` — F1 (`ListPaths` per-path) + F8 (sort stage)
- `api/internal/application/roadmap/service_test.go` — F8 (fake `sort` 3 hàm)
- `api/internal/transport/graphql/errors.go` — F2 (guard lỗi hệ thống)
- `api/internal/infrastructure/audio/engine.go` — F3 (bỏ 2 field tĩnh, thêm `TTSEngine`/`STTEngine`/`IsReal`)
- `api/internal/infrastructure/audio/stub.go` — F5 (sửa comment sai)
- `api/internal/transport/http/router.go` — F3 (`Options.Audio` thành func) + F4 (`Server(baseCtx)`)
- `api/internal/transport/http/health.go` — F3
- `api/internal/transport/http/audio.go` — F3 (`Info()` vào trong closure, 2 chỗ)
- `api/internal/transport/http/router_test.go` — F3 (đổi sang func provider)
- `api/internal/platform/testdb/testdb.go` — F7 (`dropSchema`)
- `api/cmd/langapp/main.go` — F3 (func provider) + F4 (truyền `ctx` thật)
- `api/services/audio-service/simplify.go` — F5 (xoá bảng nhân bản)
- `Dockerfile` — F3 (healthcheck nhận `ok` lẫn `degraded`)
- `docker-compose.yml` — F3 (healthcheck `app-v2`; **không** đụng `app` v1)

**Test mới (5 file):**
- `api/internal/application/roadmap/listpaths_test.go` (6)
- `api/internal/transport/graphql/errors_test.go` (6)
- `api/internal/transport/graphql/roadmap_tree_test.go` (+1)
- `api/internal/transport/http/health_engine_test.go` (7)
- `api/internal/platform/testdb/testdb_leak_test.go` (3)

**Sửa test hiện có:** `api/internal/transport/graphql/resolver_test.go` (+2),
`api/internal/infrastructure/audio/client_test.go` (`Real` → `IsReal()`),
`api/services/audio-service/audio_test.go` (xoá `hasTraditional` trùng + test F5),
`api/internal/transport/http/router_test.go`.

**KHÔNG sửa:** `api/graph/**`, `api/internal/{domain,application}/{srs,content,practice,insight,sync}/**`,
`api/internal/infrastructure/{roadmap,srs,content,practice,insight,sync}/**`,
`api/*.go` cũ, `api/main.go`, `web/`, `schema.sql`. Không chạy `go generate`.
