# STACK-V2 — M4 (transport: Gin + GraphQL + audio-service gRPC)

> Hoàn tất 2026-09-28. Gate của M4: oracle review port + N+1 + security config.
>
> M1: `stack-v2-m1.md` (+ remediation). M2: `stack-v2-m2.md`. M3: `stack-v2-m3.md`
> + `m3-remediation.md` + `m3-final.md`. M4 **không** sửa `api/*.go` cũ và
> **không** sửa `web/`.

## 0. Tóm tắt 1 dòng

Entrypoint mới `api/cmd/langapp/main.go` phục vụ **32 query + 33 mutation
GraphQL** (`/query`) + **5 endpoint nhị phân REST** + static SPA; `audio-service`
tách process riêng nói chuyện qua gRPC `audio/v1`; **cây roadmap 5 tầng trả
trong 1 request với 8 SQL statement không đổi theo số topic**.

---

## 1. Schema GraphQL

Schema-first: nguồn ở `api/graph/schema/*.graphqls` (7 file, tách theo bounded
context), code sinh ở `api/internal/transport/graphql/`. Cấu hình: `api/gqlgen.yml`.

| File | Nội dung |
|---|---|
| `common.graphqls` | `UserError`, `ErrorCode`, 5 enum dùng chung |
| `srs.graphqls` | deck / card / review |
| `content.graphqls` | dict, strokes, chunk, tone, THIEU, reader, HSK import |
| `roadmap.graphqls` | cây 5 tầng + bản đồ + payload 20 node |
| `practice.graphqls` | shadow, diff, sổ lỗi |
| `insight.graphqls` | stats, streak, top lỗi, progress range |
| `sync.graphqls` | merge, conflict, status |
| `root.graphqls` | `Query` (32 field) + `Mutation` (33 field) |

### 1.1 Query (32)

| Nhóm | Field |
|---|---|
| `srs` (4) | `decks` `deck(id)` `cards(deckId)` `dueCards(deckId)` |
| `content` (11) | `dictSearch` `englishSearch` `stress` `strokes` `chunk` `gradeTone` `thieuAxes` `thieuSessions` `readerArticles` |
| `roadmap` (8) | `paths` `path(slug)` `pathProgress(slug, since)` `stage(id)` `topic(id)` `resource(id)` `milestone(id)` |
| `practice` (5) | `shadowProgress` `errors` `topErrors` `errorSuggestions` `diff` |
| `insight` (4) | `stats(range)` `streak` `insightTopErrors` `progressOverRange(since)` |

`chunk` / `gradeTone` / `strokes` / `stats` / `insightTopErrors` / `progressOverRange`
/ `pathProgress` trả dạng **payload kèm `ok` + `error`** vì use case của chúng có
thể lỗi nghiệp vụ mà `Query` không trả `null` — 1 query có nhiều `alias` thì lỗi
trả từ resolver làm cả response thành `data: null`, mất luôn phần thành công.

### 1.2 Mutation (33)

`srs` 7 · `content` 5 · `roadmap` 18 (path 3 + stage 4 + topic 4 + resource 3 +
milestone 3 + status 1) · `practice` 3 · `sync` 1.

Tất cả trả `ok` + payload + `error { message code }`; message **nguyên văn** từ
`*application/errors.Error` (test `Test_business_error_message_is_verbatim_from_application`
so message của `graphQL` với message của chính use case). `sync` là ngoại lệ duy
nhất: trả thẳng `MergeResult` vốn đã mang `ok` + `error` + `warnings` +
`conflicts` trong schema.

### 1.3 Quy ước đã chốt

- **Mọi type có `guid`** cho client định danh — cùng hợp đồng `guid` mà
  `application/sync.Decide` dùng để nhận diện peer.
- **Cây roadmap trả cả layout + `level`**, lấy từ `roadmap.Service` đã gọi
  `domain/roadmap.ComputeLayout` + `LevelStates`. Resolver **không tính lại**
  (tính lần 2 là 2 bản luật trôi khỏi nhau — loại bug M2 đã gặp khi ghép layout
  theo chỉ số thay vì theo id).
- `ID` là `String`; mọi mốc thời gian là `String` RFC3339 vì cột DB là TEXT.
- `BookmarkStatus` + type `Bookmark` **bị gỡ khỏi schema**: `application/roadmap`
  chưa có use case CRUD bookmark (M2 chỉ làm đủ để sync không bỏ sót). Thêm
  `bookmarks` vào schema lúc này là khai 1 field luôn trả `null` — xem §8 SKIP.

---

## 2. Endpoint REST (5 nhị phân + health + static)

| Method | Path | Nội dung |
|---|---|---|
| GET | `/api/tts?text=&lang=` | stream `audio/wav` + header `X-Engine` |
| POST | `/api/stt` | multipart field `audio` → JSON transcript |
| GET | `/api/backup` | download — **501** (M7) |
| POST | `/api/restore` | multipart field `file` — **501** (M7) |
| GET | `/api/health` | `{"status":"ok"}` / `degraded` / 503 — cho HEALTHCHECK |
| GET | `*` | static SPA từ `WEB_DIST`, fallback `index.html` |
| POST/GET | `/query` | GraphQL (gqlgen) |
| GET | `/playground` | GraphiQL, **404 ở production** |

`/api/backup` + `/api/restore` trả **501** chứ không 500: `VACUUM INTO` không
tồn tại ở Postgres, tương đương là `pg_dump` thuộc M7. 500 khiến client hiện
"lỗi" cho thứ đơn giản là "chưa làm".

Test `Test_api_group_registers_exactly_the_five_binary_endpoints_plus_health` chốt
đúng 6 route `/api/*` — thêm 1 endpoint JSON vào REST là mở đường cho client quay
lại gọi tuần tự, phá vỡ đúng lý do STACK-V2 chuyển sang GraphQL.

### 2.1 Cấu hình server

- `gin.New()` + `gin.Recovery()` + `gin.LoggerWithConfig` (không `gin.Default()`)
- `SetTrustedProxies(nil)` — mặc định Gin là `0.0.0.0/0`, tức **tín** mọi
  `X-Forwarded-For`. Test khẳng định qua hành vi `ClientIP`.
- CORS middleware đặt **trước** `gin.Recovery()`: panic trong handler sau sẽ
  ghi 500 mà không có header CORS ⇒ trình duyệt báo "CORS bị chặn" thay vì "server
  lỗi", và dev mất dấu vết.
- `MaxMultipartMemory` 10MB; `MaxBytesReader` chặn trước khi parse.
- `http.Server` **có 4 timeout** (ReadHeader 5s / Read 30s / Write 60s / Idle 120s)
  — sửa gosec G112. Test assert từng cái khác 0.
- `BaseContext` gắn context gốc của process để `Shutdown` huỷ lệnh DB/audio.

### 2.2 Cấu hình GraphQL server

| Thiết lập | Giá trị | Ghi chú |
|---|---|---|
| transport | `POST{}` + `GET{}` | GET để mở link debug bằng query string |
| query cache | `lru` 1000 | operation đã parse không phụ thuộc dữ liệu |
| APQ | `lru` 100 | |
| complexity limit | `FixedComplexityLimit(200)` | |
| introspection | tắt ở `GIN_MODE=release` | bật ở dev |

`extension.Introspection` là **struct rỗng** không có field bật/tắt: muốn tắt
thì **không `Use`** nó. Viết `extension.Introspection{Introspection: false}` sẽ
**bật** introspection — đúng thứ cần tắt ở production.

---

## 3. Proto + công cụ sinh code

`api/proto/audio/v1/audio.proto`, package `audio.v1`:

```protobuf
service Audio {
  rpc Synthesize(SynthesizeRequest) returns (SynthesizeResponse);
  rpc Transcribe(TranscribeRequest) returns (TranscribeResponse);
  rpc StreamSynthesize(StreamSynthesizeRequest) returns (stream Chunk);
}
```

**Công cụ: `buf` 1.73.0** (cài được, mạng OK) + `protoc-gen-go` +
`protoc-gen-go-grpc`. Chọn `buf` vì nó tự chứa compiler — không cần hệ thống có
`protoc` cài sẵn (máy này không có). Lệnh: `go generate ./proto/...`
(`api/proto/generate.go`), cấu hình ở `api/proto/buf.yaml` + `buf.gen.yaml`.

`buf.yaml` phải nằm trong `api/proto/` chứ không phải `api/`: không có
`buf.yaml` thì buf coi `proto` là `subDirPath` và sinh ra `api/audio/v1/`
(thiếu `proto/`). Đã gặp và đã sửa.

Code sinh: `api/proto/audio/v1/audio.pb.go` + `audio_grpc.pb.go` (~27KB).

---

## 4. Quyết định chỗ đặt bảng phồn→giản

**Đặt Ở `audio-service`** (`api/services/audio-service/simplify.go`), **không**
import từ app.

Lý do:
1. **Ranh giới service.** Tiến trình audio không được import tầng trong của app;
   đó là điều `domain/audio` đã nói ("`audio` sở hữu 0 bảng, stateless").
2. **Đây là tiền xử lý INPUT của engine.** Whisper nghe ra chữ phồn, cần đưa về
   giản **trước khi nó đi qua ranh giới gRPC** — app không thể biết engine nào
   đứng sau nên không thể gánh phần này. Nếu để app chuẩn hoá thì `application/
   practice.DiffAgainstSample` lại chuẩn hoá lần nữa (idempotent, vô hại) —
   nhưng `application/practice` đã tự chuẩn hoá trong `DiffAgainstSample`, tức
   quy tắc nằm ở 2 nơi.
3. `domain/content.ToSimplified` vẫn giữ nguyên cho 2 context dùng để SO khớp
   (LCS diff, sổ lỗi) — đó là quy tắc nghiệp vụ, thuộc domain.

Bảng rune giống hệt `api/simplify.go` v1 (port nguyên văn), **không** dùng
`unicode/norm` — GOROOT máy này bị cắt mất package đó (STACK-V2 §8).

Test: `Test_to_simplified_covers_every_traditional_char_in_migration_seed`,
`Test_decode_whisper_falls_back_to_segment_words`,
`Test_whisper_response_is_normalized_to_simplified_before_leaving_service`, và ở
tầng application: `Test_diff_normalizes_traditional_to_simplified`.

---

## 5. Đo N+1 — KẾT QUẢ

Đo bằng `sqlCounter` (logger GORM đếm statement) gắn vào pool **trước** khi
`platform.Wire` dựng service, rồi chạy query qua `graph/client`:

| Query | Kích thước | Số SQL statement |
|---|---|---|
| Cây 5 tầng `path { stages { topics { resources } milestones } }` | **1 topic** | **8** |
| Cây 5 tầng (cùng trên) | **51 topic** | **8** |
| List `paths { path { stages { topics } } }` | 1 path | 7 |
| List (cùng trên) | 10 path | 7 |

**Chênh lệch = 0.** Không hằng số tuyến tính theo số topic, nên
`Test_roadmap_tree_sql_statement_count_is_constant_as_topics_grow` và
`Test_paths_list_sql_statement_count_is_constant_as_paths_grow` assert bằng
`require.Equal(t, one, fiftyOne)` — lệch 1 là đỏ.

8 statement gồm: 1 (`path` theo slug) + 3 (`stages IN` / `topics IN` /
`milestones IN`) + 1 (`resources IN`) + 1 (`decks IN`) + 2 dư (đếm cả statement
của `pipelines`/`BEGIN` của GORM trong transaction nội bộ).

### 5.1 Vì sao phải thêm batch read ở tầng application

Để đạt "hằng", dataloader **không** đọc `ListTopics`/`ListResources` 1 lần cho mỗi
cha (đường đó là tuyến tính theo số topic). Phải có đường đọc nhiều key 1 lần.
`STACK-V2-PLAN §2` không cấm, chỉ yêu cầu 1 nguồn sự thật — và đây là lý do **M4
đã sửa file trong `internal/{application,infrastructure}`** (nằm ngoài phạm vi ghi
khai ban đầu), liệt kê đầy đủ ở §9.

Bản chất: `GetPath` và `StageTreeByPathIDs` dùng **cùng** `decorateTopics` +
`treeParts`, nên số statement của 2 đường bằng nhau và layout chỉ có 1 bản luật.

---

## 6. Kết quả test

`LANGAPP_TEST_POSTGRES_DSN=… go test -count=1 ./...` → **25 package ok, 0 FAIL,
0 SKIP**, chạy **3 lần liên tiếp** cùng kết quả.

**85 test M4** (`internal/transport/**`, `internal/infrastructure/audio/`,
`services/audio-service/`), trong đó 8 test đo N+1/introspection/complexity mà
plan §6 yêu cầu bắt buộc.

### 6.1 Hai lỗi cấu hình test phát hiện (đã sửa)

Cả hai đều là lỗi thật, không phải lỗi test:

1. **Complexity limit trả HTTP 200 thay vì 422.** `errcode.GetErrorKind` chỉ coi
   lỗi là "protocol" (⇒ 422) khi mã extension của nó có trong bảng đăng ký, mà
   `COMPLEXITY_LIMIT_EXCEEDED` thì không. Đã thêm
   `errcode.RegisterErrorType(complexityErrorCode, errcode.KindProtocol)` trong
   `server.go` — không có nó thì client không có tín hiệu nào để biết "query của
   tôi quá nặng", chỉ thấy 200 kèm lỗi.
2. **Lỗi introspection bị che thành "lỗi hệ thống".** `errorPresenter` cũ mask
   MỌI lỗi. Nay chỉ mask lỗi Go thô (có thể chứa SQL/tên bảng); `*gqlerror.Error`
   do chính gqlgen sinh (validation, introspection, complexity) đi nguyên vẹn ra
   client — đó là thứ client CẦN để hiểu vì sao hỏng.

### 6.2 Một bug có thật của M1–M3 lộ ra khi M4 thêm package

`testdb.connectLock` mở `*sql.DB` rồi chỉ đóng `*sql.Conn`, **rò pool mỗi test**.
`sql.Open` mặc định `MaxIdleConns` không giới hạn, `MaxIdleTime = 0` ⇒ connection
đó sống tới hết package. Đo được: peak **99/100 connection**, và test fail bằng
`FATAL: sorry, too many clients already` ngay khi số package test của M4 đủ lớn.

Đã sửa: `connectLock` trả cả `*sql.DB`, `Session.release` đóng pool sau khi
`conn.Close()`. Peak sau khi sửa: **9**. Ngoài ra `testdb` giảm pool 5→2
connection (vẫn > 1 vì M1 §5 ghi rõ `MaxOpenConns = 1` làm repository test treo).

---

## 7. Verify khác

| Lệnh | Kết quả |
|---|---|
| `go build ./...` | PASS |
| `go vet ./...` | PASS (0 cảnh báo) |
| `gofmt -l api` | 2 file: `chinese_test.go`, `reader.go` — **có sẵn từ trước M4**, thuộc app cũ ngoài phạm vi ghi |
| `go test .` (app cũ) | **PASS** `ok langapp 0.893s` |
| `go generate ./...` | PASS, **6 file generated byte-identical** trước/sau (md5sum khớp) |
| `docker compose config` | PASS (mặc định + cả 3 profile) |

### 7.1 Ba grep luật DDD — 0 file mỗi cái

```
$ grep -rl 'gorm\.io\|gin-gonic\|database/sql\|net/http\|google\.golang\.org/grpc' api/internal/domain   → exit 1 (0 file)
$ grep -rl 'gorm\.io'                                api/internal/application                                → exit 1 (0 file)
$ grep -rl 'internal/infrastructure'                 api/internal/domain api/internal/application           → exit 1 (0 file)
```

Kiểm chắc hơn bằng dependency thật (`go list -deps -test`, thấy import chứ không
thấy comment) — rỗng cho `langapp/internal/platform`, `langapp/internal/migrate`,
`langapp/migrations`, `langapp/internal/transport/*`, `langapp/internal/infrastructure/*`.

Ghi chú: `database/sql/driver` **có** xuất hiện trong graph của `application` —
nó đến từ `github.com/google/uuid` v1.6.0 (đã có sẵn từ M1, `application/*` đã
import từ M2). Không phải hồi quy M4.

---

## 8. SKIPPED / BLOCKED

| Việc | Trạng thái | Lý do |
|---|---|---|
| Bookmark trong GraphQL | **SKIP có chủ ý** | `application/roadmap` chưa có use case CRUD bookmark (M2 chỉ đủ để sync không bỏ sót). Thêm `bookmarks`/`BookmarkStatus` vào schema lúc này là khai 1 field luôn trả `null`. Thêm ở M6/M7 khi có use case. **Đã gỡ khỏi schema** thay vì để field chết. |
| `BackupPort` (`pg_dump`/`pg_restore`) | **KHAI, KHÔNG implement** | `VACUUM INTO` không tồn tại ở Postgres (STACK-V2 §4.6). Thuộc M7. `/api/backup` + `/api/restore` trả 501 có message nói rõ. |
| `sync.Sync` qua peer schema thật | **NỐI DÂY, KHÔNG CÓ PEER** | `syncinfra.NewSchemaLoader(nil)` — `pg_restore` vào schema tạm là M7. Mutation `sync` tồn tại và trả `ok=false` + `error` khi chưa có peer. |
| `infrastructure/roadmap`/`srs` `UpsertDeck` `skipped` | **NỢ M7** | Ghi nhận từ M3-final §8.1; M4 không đụng `sync`. |
| Thay `app` v1 trong compose bằng `app-v2` | **SKIP có chủ ý** | UI React hiện tại gọi ~30 endpoint JSON của v1; app v2 chỉ có GraphQL + 5 nhị phân. Thay sẽ làm sản phẩm hỏng cho tới M5. `app` v2 để **profile `v2`**, cùng image, chỉ khác `entrypoint`. |
| Image riêng cho `audio-service` | **SKIP có chủ ý** | Image chính đã có sẵn Piper + 2 voice 752MB ở `/models`. Tách image ⇒ tải 2 lần 752MB, hoặc phải mount volume `models` — mà volume đó rỗng lúc đầu nên Piper không thấy voice và **rơi về stub im lặng**. Dùng chung image + 2 `entrypoint`. |
| `RoadmapSeeder` vào `Container` như repository | **LÀM** | M3-final §9 nói nếu M4 muốn 1 entry point thì `Container.Seed(ctx)`. Cả hai: trường `RoadmapSeeder` để test assert dựng được, `Seed(ctx)` là entry point duy nhất `main` gọi. |

**Không có gì BLOCKED.** Công cụ sinh proto (`buf`) cài và chạy được.

---

## 9. Sửa file NGOÀI phạm vi ghi — liệt kê đầy đủ

Phạm vi ghi ban đầu là `api/internal/transport/**`, `api/internal/infrastructure/
audio/**`, `api/services/audio-service/**`, `api/graph/**`, `api/gqlgen.yml`,
`api/proto/audio/v1/**`, `api/cmd/langapp/main.go`,
`api/internal/platform/di.go`, `api/go.mod`, `Dockerfile`, `docker-compose.yml`.

### 9.1 Bắt buộc: batch read để đạt "số SQL hằng" (§5.1)

Không thể đạt yêu cầu "1 topic vs 51 topic phải là hằng" mà không có đường đọc
nhiều key 1 lần. Đây là lý do duy nhất.

| File | Sửa gì |
|---|---|
| `application/roadmap/ports.go` | `Repository` += `ListStagesByPathIDs`, `ListTopicsByStageIDs`, `ListResourcesByTopicIDs`, `ListMilestonesByStageIDs` |
| `application/roadmap/service.go` | Tách `treeParts` (3 lệnh batch) + `decorateTopics` (layout/level 1 bản luật) khỏi `GetPath`; `GetPath` dùng lại chúng nên hành vi **không đổi**; `PathBySlug`, `StageTreeByPathIDs`, `StageTreeByIDs`, `ResourcesByTopicIDs`, `ProgressByIDs`, `ProgressByPathIDs`, `StageByID`, `TopicByID`, `ResourceByID`, `MilestoneByID`; `ListPaths` chuyển sang `ProgressByIDs` (trước gọi 2 lệnh/path) |
| `infrastructure/roadmap/repository_batch.go` | **MỚI** — 4 hàm `...By*IDs` viết SQL, `ORDER BY` tường minh, thu row vào slice trước khi đóng (luật 3 của repository) |
| `application/srs/ports.go` | `Repository` += `DecksByIDs` |
| `application/srs/service.go` | `FindDecks` (batch của `FindDeck`) |
| `infrastructure/srs/repository_batch.go` | **MỚI** — `DecksByIDs` |
| `application/{roadmap,srs}/*_test.go` | Thêm 5 method batch vào `fakeRepo` (fake mô phỏng `WHERE ... IN`, kết quả y hệt gọi từng cha) |

### 9.2 Bắt buộc: vá lỗi pool rò của M1–M3 (§6.2)

| File | Sửa gì |
|---|---|
| `platform/testdb/testdb.go` | `connectLock` trả cả `*sql.DB`; `Session` giữ pool; `release` đóng pool. Không sửa dòng logic nào khác. |
| `platform/testdb/open.go` | **MỚI** — `openTestDB` tự mở GORM thay vì gọi `platform.OpenPostgres` (xem 9.3) |
| `platform/migrate.go` | Rút gọn thành alias của `internal/migrate` |
| `internal/migrate/migrate.go` | **MỚI** — `Migrate` chuyển từ `platform` sang đây, nguyên văn |

### 9.3 Bắt buộc: cắt vòng import

`platform/di.go` (trong phạm vi ghi) phải import `internal/infrastructure/*`.
Nhưng `platform/testdb` import `platform`, và `infrastructure/{roadmap,srs}` có
**in-package test** (`internal_test.go` — cần nắm `txHandle` không export). Vòng:

```
roadmapinfra (test) → platform/testdb → platform → roadmapinfra
```

2 cách cắt, chọn cách **không sửa test M2**:
1. ✅ Tách `Migrate` ra `internal/migrate` + cho `testdb` tự mở GORM ⇒
   `testdb` **không còn** import `platform`. `platform.Migrate` giữ lại làm alias
   để call site M1 không đổi.
2. ❌ Chuyển `internal_test.go` sang external test package — phải export `txHandle`
   hoặc viết lại test P0 của M2.

---

## 10. Ghi chú cho M5/M7

- **`go generate ./...` cần 3 tool trong PATH**: `buf`, `protoc-gen-go`,
  `protoc-gen-go-grpc`. Không có chúng thì `go generate` fail, nhưng
  `go build` + `go test` vẫn chạy (code sinh đã commit).
- **`model/tree.go` (Path/Stage/Topic) là autobind** — đổi field ở đó sẽ làm
  gqlgen ngừng sinh resolver tương ứng. Xem comment trong file.
- **`graphql/tree.go`** chứa `stageTopics` (hàm viết tay) — tách riêng file
  `*.resolvers.go` vì gqlgen **ghi đè toàn bộ** file sinh ra, hàm viết tay nằm
  trong đó sẽ biến mất mỗi lần `go generate`. Đã gặp.
- **Compose**: `docker compose up` (mặc định) = `postgres` + `app` v1.
  Bật app v2: `docker compose --profile v2 up -d` (port 8081).
  Bật thêm audio thật: `--profile audio` (Piper) và `--profile stt` (Whisper).
- **`AUDIO_GRPC_ADDR` rỗng = stub** sine/giả transcript. Header `X-Engine` nói
  `"stub"`, `Real=false`, `/api/health` trả `degraded` (vẫn 200 — app DÙNG ĐƯỢC,
  trả 503 sẽ khiến Docker restart vô ích).
- **Rủi ro vận hành 2 protocol** (STACK-V2 §3): sau mutation phải invalidate cả
  cache urql (GraphQL) lẫn vue-query (REST). Chưa có test nào phủ — M5 khi thêm
  client mới viết test.
- **2 con số dễ nhầm trong `Stats`** (`doneWindow` vs `totalAll`, `dueNow` không
  kéo `state='new'`) giữ nguyên hành vi v1; đã ghi trong doc của schema.
- `application/roadmap` chưa có `UpsertDeck` guard `skipped` (nợ M3-final §8.1);
  M7 đồng bộ khi đụng `sync`.
