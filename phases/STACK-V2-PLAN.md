# STACK-V2 PLAN — DDD + Gin + GORM + PostgreSQL + GraphQL + Vue 3

> Quyết định của user 2026-09-28. Thay nền cả 3 tầng: FE React→Vue 3, BE net/http→Gin,
> DB SQLite→PostgreSQL. Thêm GORM, thư viện migration, GraphQL, DDD layer theo bounded
> context, gRPC cho service thật. Song song triển khai tính năng **roadmap** (đã xong
> backend) và **bookmark** (mới).

## 0. Bối cảnh đã có (không làm lại)

- Backend Go ~30 endpoint `net/http`, SQLite file, `schema.sql`, ~110 test.
- **Roadmap backend đã xong**: 5 bảng, 21 endpoint, seed 2 path / 10 stage / 51 topic /
  263 resource từ `learn_chinese` + `learn_english`.
- Frontend React 18.3 + Vite 5.4, 20 route component, 105 vitest test. Không router lib, không CSS framework.
- Audio: Piper `os/exec` trong main image (752MB) + Whisper sidecar `onerahmet/openai-whisper-asr-webservice` profile `stt` (~8GB).

## 1. Stack chốt

### Backend

| Hạng mục | Chọn | Version | Ghi chú |
|---|---|---|---|
| Router | `github.com/gin-gonic/gin` | v1.12.0 | Cần Go ≥1.25 (máy 1.27.1). `gin.New()` + `gin.Recovery()` + `gin.LoggerWithConfig`, **không** `gin.Default()` |
| ORM | `gorm.io/gorm` + `gorm.io/driver/postgres` | gorm v1.31.x / driver v1.6.x | Thay `modernc.org/sqlite` |
| Migration | `github.com/pressly/goose/v3` | v3.28.0 | `goose.DialectPostgres` + `embed.FS` + `provider.Up(ctx)` lúc boot |
| GraphQL | `github.com/99designs/gqlgen` | v0.17.95 | go.mod khai `go 1.26` — máy 1.27.1 đủ, không cần pin bản cũ |
| Dataloader | `github.com/vikstrous/dataloadgen` | latest | Chống N+1 khi query cây roadmap |
| gRPC | `google.golang.org/grpc` + `protobuf` | latest | Chỉ client tới `audio-service` |
| Validate | `go-playground/validator/v10` | v10.30.5 | Gin kéo về, tag `binding:` |
| UUID | `github.com/google/uuid` | v1.6.0 | Đã có trong go.mod |
| Log | `log/slog` built-in | — | **Không** thêm zap — single-user, ~30 endpoint |
| Test | `github.com/stretchr/testify` | v1.12.1 | |
| Concurrency | `golang.org/x/sync/errgroup` | v0.23.0 | |
| Lint | `golangci-lint` | v2.14.0 | `gosec` G112 (http.Server thiếu timeout) là quan trọng nhất |

**Cố ý KHÔNG dùng:** zap (thừa cho slog ở quy mô này), viper (đọc config 1 lúc boot),
testcontainers (Postgres chạy sẵn trong compose), entgo (đã chọn GORM), Pinia (xem §5),
vee-validate (20 tháng không cập nhật, kẹt zod v3), vuedraggable 2.x (Vue 2, chết),
`@tanstack/graphql-query` (không tồn tại), `@vue/apollo-composable` (stale 18 tháng),
`@urql/codegen` (đã unpublish).

### Frontend

| Hạng mục | Chọn | Version |
|---|---|---|
| Framework | `vue` | 3.5.43 — `<script setup>` + Composition API + TS |
| Build | `vite` + `@vitejs/plugin-vue` | 8.3.1 / 6.0.9 |
| Router | `vue-router` | 5.3.1 — `createWebHashHistory()` |
| GraphQL client | `@urql/vue` + `@urql/core` | 2.1.1 / 6.0.3 (đang maintain) |
| REST client | `@tanstack/vue-query` | 5.104.0 — chỉ cho 5 endpoint REST còn lại |
| CSS | `tailwindcss` + `@tailwindcss/vite` | 4.3.3 (v4: không `tailwind.config.js`, chỉ `@import "tailwindcss"` + `@theme`) |
| UI kit | `shadcn-vue` (copy-in) | 2.8.2 — chỉ lấy form/modal/table; roadmap tự viết class |
| Icon | `lucide-vue-next` | 1.0.0 (import từng icon để tree-shake) |
| Validate | `zod` | 4.6.5 — schema thuần TS, tái dùng cho mutation input lẫn test |
| Ngày | `date-fns` | 4.4.0 (locale `vi`) |
| Util | `@vueuse/core` | 15.0.0 |
| Test | `vitest` + `@vue/test-utils` + `happy-dom` | 5.x / 2.5.1 / 20.x |
| gql | `graphql` | 17.0.2 |

Query viết tay trong `.vue`/`.ts` (`gql\`...\``) — 30 operation thì đủ, không bật codegen
client. Nếu sau này muốn types tự sinh thì dùng `@graphql-codegen/cli` với
`preset: 'client'`, schema lấy từ file `schema.graphql` export sẵn (introspection tắt ở prod
thì codegen không introspect được).

## 2. DDD layer + bounded context

**Quyết định:** DDD monolith — layer gọi nhau bằng Go interface trong 1 process. gRPC chỉ dùng
cho tiến trình tách biệt thật. Lý do đã nói thẳng với user và được chấp thuận: gRPC là giao thức
giữa process; gọi domain layer của chính mình qua gRPC loopback chỉ thêm serialize + port +
healthcheck + mất transaction boundary, không đổi lấy lợi ích.

Cơ sở: `~/.config/opencode/instructions/code-style.md` đã bắt buộc đúng pattern này cho Python.

```
api/<cmd>/main.go            entrypoint: DI, boot, graceful shutdown
internal/
  platform/                  config, DI wiring, slog, DB pool, goose boot
  domain/                    PURE Go — không gorm, không gin, không grpc
    srs/ content/ roadmap/ audio/ practice/ insight/ sync/
  application/               orchestration — KHÔNG import gorm
    <ctx>/*_service.go, ports.go (interface cần từ context khác)
  infrastructure/            NƠI DUY NHẤT import gorm / grpc client
    <ctx>/model.go, repository.go, seed.go
  transport/
    http/                    Gin: 5 endpoint REST nhị phân + static SPA
    graphql/                 *.graphqls, resolver → application service
    grpc/                    audio/v1/*.proto + generated client
services/
  audio-service/             process riêng: Piper exec + forward Whisper, expose gRPC
```

Luật bất di bất dịch — grep phải ra 0 hit:
- `domain/**` không import `gorm.io/*`, `gin-gonic/*`, `google.golang.org/grpc`
- `application/**` không import `gorm.io/*`, không giữ `*gorm.DB`
- `infrastructure/**` là nơi duy nhất import `gorm.io/*`
- `domain/**` + `application/**` không import `internal/infrastructure/**`

| Context | Sở hữu bảng | Phụ thuộc ra ngoài |
|---|---|---|
| `srs` | decks, cards, reviews | — |
| `content` | dict, en_dict | `audio` (TTS đọc mẫu) |
| `roadmap` | roadmap_paths…resources, roadmap_bookmarks | `srs` (kiểm tra `deck_id`) |
| `audio` | — stateless | `transport/grpc` |
| `practice` | notes (prefix `SHADOW|`/`ERR|`) | `audio`, `srs`, `content` |
| `insight` | — read model | `srs`, `roadmap`, `practice` |
| `sync` | sync_meta, sync_conflicts | tất cả context có bảng |

Giao tiến chéo context: qua interface Go trong `application/<ctx>/ports.go`, bind trong
`platform/di.go`. Transaction: `UnitOfWork` interface ở application, hiện thực
`db.Transaction` ở infrastructure.

## 3. GraphQL — hybrid, không thay REST

| Loại | Giao thức | Lý do |
|---|---|---|
| deck, card, review SRS, roadmap cây, bookmark, stats, search | **GraphQL** | Query cây 1 lần thay 4 request tuần tự; client chỉ lấy field cần |
| `GET /api/tts` stream `audio/wav` | **REST** | GraphQL không có streaming binary; base64 +33% payload |
| `POST /api/stt` multipart upload | **REST** | gqlgen có `transport.MultipartForm` nhưng client phải gửi `operations` map — thuần file thì REST rõ hơn |
| `GET /api/backup` download file | **REST** | GraphQL binary nên trả signed URL |
| `POST /api/restore` multipart | **REST** | như stt |
| static SPA | **REST** | |

Gin mount: `r.POST("/query", gin.WrapF(srv.ServeHTTP))` + `r.GET("/playground", …)` (GraphiQL).
**Không** đặt GraphQL ở `/api/graphql` — sẽ đụng prefix wildcard của REST.

Cấu hình server: `transport.Options{}+GET{}+POST{}`, `lru` query cache 1000,
`extension.AutomationPersistedQuery{Cache: lru 100}`, `extension.FixedComplexityLimit(200)`
(trả HTTP 422 khi vượt). Introspection **tắt ở production** (`extension.Introspection{Introspection:false}`).
gqlgen **không có depth limit sẵn** — nếu cần thì tự viết `OperationContextMutator` đếm độ sâu.

Test theo tỉ lệ 70/30:
- **~70% resolver unit** — gọi `resolver.Method(ctx, args)` trực tiếp + GORM test session.
- **~30% `graph/client`** — `c.MustPost(\`{…}\`, &resp)` kiểm schema↔resolver wiring + dataloader batching. Không cần httptest.
- **vài HTTP smoke** — chỉ để chắc route mount + CORS.

Rủi ro vận hành 2 protocol: **cache client lệch nhau** là rủi ro lớn nhất (urql cacheExchange chỉ
biết GraphQL, vue-query chỉ biết REST). Giảm thiểu: sau mutation phải invalidate cả 2 cache;
ghi rõ trong test. Auth chưa có ở v1 nên không lệch pipeline.

## 4. Hệ quả Postgres (chấp nhận, phải xử lý)

1. **Không còn "1 binary chạy offline"** → compose 4 service: `app`, `postgres`, `audio-service`, `stt`(profile).
2. **FTS5 mất** → `tsvector` + GIN + `pg_trgm`. **ĐÍNH CHÍNH sau cổng M1**: nhận định ban đầu trong
   plan là "tra 1 ký tự Hán sẽ yếu hơn" — **sai**. Oracle verify thực nghiệm trên SQLite FTS5 v1:
   `dict MATCH '你好'` trên `'你好世界'` và `MATCH '你'` đều trả `[]`, vì `unicode61` cũng coi cả
   chuỗi Hán là 1 token, y hệt `to_tsvector('simple', …)`. Nghĩa là Postgres **không phải đánh
   đổi**, mà là cải tiến: nhánh `ILIKE` là bắt buộc về đúng tính, các index GIN `pg_trgm` chỉ là
   tối ưu (và chỉ dùng được khi `q` ≥ 3 ký tự; bảng `dict` chỉ ~1084 dòng nên seq scan < 1ms).
   Giữ `pg_trgm` vì rẻ và giúp tra pinyin dài. Vẫn cần escape `%`/`_` trong `q` (F5).
3. **Trigger SQLite → Postgres**: `NEW.updated_at IS OLD.updated_at` không tồn tại →
   `IS NOT DISTINCT FROM`; `strftime(...)` → `to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`.
4. **Soft-delete giữ `deleted INTEGER 0/1`.** KHÔNG dùng `gorm.DeletedAt` — 5 bảng roadmap +
   toàn bộ logic sync LWW đã xong dựa trên integer này.
5. **GORM có thể bypass trigger** khi batch update → cấm `db.Model(&X{}).Update()`, bắt buộc
   `db.Model(&x).Updates(...)` theo instance, `Session{AllowGlobalUpdate: false}`. Test riêng cho trigger.
6. **Backup/restore đổi bản chất**: `VACUUM INTO` không tồn tại → `pg_dump`/`pg_restore` subprocess.

## 5. Thêm gì vào roadmap (amendment A1, user duyệt, chưa làm)

- `completed_at` trên `roadmap_stages` + `roadmap_topics` — set khi chuyển sang `done`, clear khi rời `done`. Nuôi biểu đồ tiến độ theo tuần/tháng (khớp vòng monthly review trong plan học).
- `deck_id` trên `roadmap_stages`, `ON DELETE SET NULL` — `G1 → deck HSK1`, `G2 → deck PVO`; bấm stage nhảy thẳng `/review`.
- `is_optional` trên `roadmap_topics` — node "tham khảo" không tính vào mẫu số phần trăm.
- Bảng mới `roadmap_bookmarks` — kho link độc lập (đây là nghĩa "bookmark" user muốn): title, url NULL, note, tags CSV, status. Đủ 4 cột `guid`/`updated_at`/`deleted`/`created_at` để sync không bỏ sót.
- `progress?since=YYYY-MM-DD` — số node hoàn thành trong khoảng.

**Sửa conflict còn treo:** `roadmap_seed/README.md` nói `title` là slug, `roadmap_seed.go` lại
`slugify(title)`. JSON seed dùng title tiếng Việt đọc được → sửa **README cho khớp code**, không
đụng JSON.

## 6. Delivery phases

Mỗi phase kết thúc bằng validation + **cổng Oracle bắt buộc**. Tuần tự: mỗi phase là nền cho
phase sau. Port theo **context** chứ không theo layer — ranh giới nằm ở package nên vẫn đúng,
mà không phải mở 7 context rỗng rồi mới có chỗ gắn transport vào. 7 phase là nhiều vì đây là
viết lại toàn bộ 3 tầng, mỗi phase vẫn là một bước shippable độc lập.

### M1 — Platform + Postgres + domain thuần (fixer)
`internal/platform` (config từ env, DI wiring, slog, GORM pool, goose boot) +
`internal/domain` 7 context (entity, value object, policy — **thuần Go, không DB**) +
migrations goose `00001_init.sql` (dịch `schema.sql` → Postgres) / `00002_fts.sql` (tsvector) /
`00003_roadmap_a1.sql` (A1) + `docker-compose.yml` thêm `postgres` + healthcheck + volume.
`http.Server` có timeout (gosec G112).
**Exit:** `go test ./...` xanh; migration up 2 lần idempotent; domain test được **không cần DB**;
grep luật DDD 0 hit.
**Gate:** oracle review DDL + ranh giới layer + index FTS.

### M2 — Infrastructure + application: `srs`, `roadmap` (fixer)
GORM model + repository + `UnitOfWork` cho 2 context; use case + `ports.go`; A1 đầy đủ
(`completed_at`, `deck_id`, `is_optional`, bảng `roadmap_bookmarks`, `progress?since=`);
**và bản đồ game** — migration `00004_roadmap_map.sql` (`roadmap_stages.terrain` CHECK 6 giá trị,
`roadmap_stages.direction` CHECK `up|right`, `roadmap_topics.map_x/map_y` REAL NULL) +
logic trạng thái màn `done|current|locked` + layout server-side. Chi tiết ở
`phases/ROADMAP-MAP-IDEA.md`. Giữ `deleted INTEGER` — KHÔNG dùng `gorm.DeletedAt`.
Sửa `roadmap_seed/README.md` cho khớp `slugify(title)`.
**Exit:** repository test trên Postgres thật; roadmap seed load 2 path/10 stage/51 topic/263 resource
không đổi; CRUD + status + progress qua application service; A1 test xanh; layout deterministic
(cùng `position` luôn ra cùng toạ độ).
**Gate:** oracle review repository + transaction + A1 + layout.

### M3 — Infrastructure + application: `content`, `practice`, `insight`, `sync` (fixer)
5 context còn lại: dict/FTS, tone engine, stress/chunk, PVO/TMRND, THIEU, reader, shadowing,
recorder, error book, stats/streak, merge LWW.
**Exit:** toàn bộ business rule hiện có (ScheduleNext, tone grade, chunk split, diff, streak,
merge) nằm trong domain, test đỏ→xanh, application service điều phối qua `ports.go`.
**Gate:** oracle review logic migrate 1:1, không đổi hành vi.

### M4 — Transport: Gin + GraphQL + audio-service gRPC (fixer)
Gin cho 5 endpoint nhị phân + static; gqlgen schema + resolver cho toàn bộ JSON; `dataloadgen`
cho cây roadmap; `audio-service` tách process riêng với proto `audio/v1`. Port ~110 test 70/30.
**Exit:** smoke `docker compose up` gọi được `/query` + `/api/tts`; query cây roadmap 1 request
trả đủ 5 tầng; đo số SQL không N+1; introspection tắt, complexity limit 200.
**Gate:** oracle review port + N+1 + security config.

### M5 — Vue 3 + Tailwind + shadcn-vue, port 20 màn (fixer)
Scaffolding mới, urql + vue-query, 12 module logic thuần port 1:1, 20 component + 105 test.
Giữ nguyên path cũ để bookmark cũ còn chạy.
**Exit:** `vitest` xanh, `tsc` sạch, 20 route render đúng, không còn import react.
**Gate:** oracle review port fidelity.

### M6 — Roadmap + Bookmark + bản đồ game UI (designer)
Phần **mới**. `/roadmap` + `/roadmap/:slug` + `/bookmarks`. Bản đồ game theo
`phases/ROADMAP-MAP-IDEA.md`: **SVG** làm xương sống (đường đi, node màn, lớp terrain 6 loại),
**Canvas 2D** chỉ cho confetti (tắt khi `prefers-reduced-motion` + có toggle), **KHÔNG Three.js**.
3 cử chỉ tách vị trí: vuốt = cuộn bản đồ theo `direction` · chạm node = mở màn ·
vuốt lên trong panel = đánh dấu Xong (luôn có nút bấm song song). Giữ list view `?view=list`
làm dự phòng, nhớ vị trí scroll qua localStorage.
**Exit:** roadmap 2 path hiển thị đúng ở cả 2 view; vuốt cuộn đúng chiều `up`/`right`;
tick persist qua reload; thêm node hiện ngay; confetti tắt đúng khi reduced-motion; vitest xanh.
**Gate:** oracle review UX + data flow + cử chỉ.

### M7 — Sync/backup Postgres + tích hợp + validate (fixer)
`pg_dump`/`pg_restore`; đưa `roadmap_*` + `roadmap_bookmarks` vào sync (lỗ hổng v1);
viết lại `DEPLOY.md` + `THIRD-PARTY-LICENSES`; validate cuối.
**Exit:** 4 service chạy; roadmap + bookmark roundtrip; test xanh; không còn
`modernc.org/sqlite` hay handler `net/http` cũ.

## 7. Definition of Done toàn bộ

`docker compose up` chạy `app` + `postgres` + `audio-service` (+ `stt` profile) →
`/query` GraphQL trả được cây roadmap 5 tầng trong 1 request → `/api/tts` trả WAV thật →
Vue SPA chạy 22 route → roadmap 2 path seed + bookmark persist → sync 2 máy hội tụ →
`go test ./...` + `vitest` + `tsc` xanh → grep DDD luật 0 hit.

## 8. Rủi ro đã biết

| Rủi ro | Mức | Giảm bằng |
|---|---|---|
| GORM batch update bypass trigger `updated_at` | Cao | Cấm `Model(&X{}).Update()` + test riêng |
| N+1 khi query cây roadmap 5 tầng trong GraphQL | Cao | `dataloadgen`, đo số query ở gate M2 |
| Port ~110 test Go + 105 test TS sang framework mới | Cao | Port theo context, xanh sau mỗi cụm |
| Mất "1 binary offline" | TB | Chấp nhận; viết lại DEPLOY.md |
| Search tiếng Trung 1 ký tự kém hơn sau khi bỏ FTS5 | TB | `pg_trgm` + `ILIKE`; ghi rõ giới hạn trong UI |
| 2 protocol lệch cache client | TB | Invalidate cả urql lẫn vue-query sau mỗi mutation |
| GORM không map được `tsvector` generated column | TB | Để tsvector do trigger quản lý, GORM không cần biết |
| Mất `unicode/norm` (GOROOT máy này bị cắt, phát hiện ở R1a) | Thấp | Giữ bảng rune tự chứa cho `slugify` |
| Vitest 5 / Vite 8 cần Node mới hơn | Thấp | Máy có Node 24.18, đủ |
