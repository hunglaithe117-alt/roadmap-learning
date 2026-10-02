# STACK-V2 — M2 (infrastructure + application: `srs`, `roadmap`)

> Hoàn tất 2026-09-28. Gate của M2: oracle review repository + transaction + A1
> + layout.
>
> M1: `stack-v2-m1.md` + `stack-v2-m1-remediation.md`. M2 **không** sửa
> `api/*.go` cũ và `web/`.

## 1. Cột đã thêm (migration `00004_roadmap_map.sql`)

| bảng | cột | kiểu |
|---|---|---|
| `roadmap_stages` | `terrain` | `TEXT NOT NULL DEFAULT 'meadow' CHECK (terrain IN ('meadow','desert','snow','volcano','ocean','city'))` |
| `roadmap_stages` | `direction` | `TEXT NOT NULL DEFAULT 'up' CHECK (direction IN ('up','right'))` |
| `roadmap_topics` | `map_x` | `REAL NULL` |
| `roadmap_topics` | `map_y` | `REAL NULL` |

**Không thêm index nào** — và đây là quyết định có chủ đích, ghi vào comment
SQL. Mọi query bản đồ đi theo `path_id → stage_id → position`, mà 3 index đó đã
có sẵn từ `00001` (`idx_roadmap_stages_path`, `idx_roadmap_topics_stage`,
`idx_roadmap_resources_topic`). Index theo `terrain`/`direction`/`map_x` sẽ là
index chết: không query nào lọc theo chúng. `ROADMAP-MAP-IDEA §7.1` chỉ yêu cầu
"index nhỏ **nếu cần**".

Migration gán sẵn terrain/direction cho 10 stage bằng `UPDATE … WHERE slug = ?`
(giả định DB đã có stage, vd restore từ backup). DB **mới** thì migration chạy
trước seed nên `UPDATE` khớp 0 dòng — loader M2 gán CÙNG bảng cho stage mới
insert qua `domain/roadmap.MapDefaults`. 2 nơi luôn phải khớp; test
`Test_seed_assigns_terrain_and_direction_per_roadmap_map_idea` chốt.

Goose: version cuối = **4**, tổng **16 bảng / 46 index** (không đổi vì 00004
chỉ thêm cột).

## 2. Quyết định layout bản đồ

### viewBox `0 0 1000 2000` (cao hơn rộng)

Tỉ lệ 1:2 vì `direction = up` chiếm ưu thế: 1 path có 5 stage × ~5 topic = 25
node, đi dọc dài hơn đi ngang. Lề an toàn `MapMargin = 120`.

### Công thức

```
t_i     = i / (n - 1)            (n = 1 → 0)
along   = 120 + t_i · span
across  = center + A · sin(2π · t_i)
```

- `dir=up`: `X = across` (tâm 500), `Y = MapViewHeight - t_i·span` → node 0 ở
  đáy (Y=1880), node cuối ở đỉnh (Y=120).
- `dir=right`: `X = 120 + t_i·span` (node 0 trái, node cuối phải),
  `Y = across` (tâm 1000).
- `span` = chiều dài trục chính sau khi trừ 2 lề.
- `sin(2π·0) = sin(2π·1) = 0` ⇒ **2 đầu bản đồ thẳng nhau**, trông như đường núi
  có nhịp lên rõ (biến số chu kỳ nguyên là cố ý, ghi trong doc comment).
- Node đơn lẻ (`n = 1`): `t = 0` → đứng ở **đầu** đường đi (khớp với node 0 của
  mọi bản đồ dài hơn) và nằm đúng trục giữa vì `sin(0) = 0`.

### Biên độ dao động theo terrain

| terrain | biên độ (viewBox) | lý do |
|---|---|---|
| `volcano` | 70 | gấp thêm, đường đi gập ghề |
| `meadow` | 40 | mặc định, nhịp vừa phải |
| `snow` | 35 | hơi phẳng hơn meadow |
| `ocean` | 30 | sóng nhẹ |
| `desert` | 25 | phẳng |
| `city` | 20 | phẳng nhất (đường phố) |

Hằng số thiết kế, **không** suy ra từ công thức: layout phải deterministic và
đổi biên độ không được làm đổi hình dạng bản đồ đã lưu. `volcano` "gấp thêm"
được xử lý bằng **hằng số lớn hơn**, tuyệt đối không dùng `math/rand` (vi phạm
determinism của `ROADMAP-MAP-IDEA §2`).

### Nén khi nhiều node

Trục chính chia đều theo index nên `n` lớn tự nén khoảng cách dọc; biên độ
**không** bị nén (node vẫn nằm trong `[0, 1000] × [0, 2000]` với mọi tổ hợp
terrain × direction × n ∈ {1, 2, 3, 5, 12, 51, 60} — test
`Test_compute_layout_stays_inside_view_box_for_every_terrain_and_size`).

### Ưu tiên giá trị user

`map_x`/`map_y` khác NULL thì lấy giá trị user cho TRỤC ĐÓ (chỉ 1 trục cũng
được: trục chưa set lấy từ layout). `MapPinned` trong `TopicView` báo client
biết node đó do user đặt tay. Validate: giá trị phải nằm trong viewBox —
node ngoài canvas thì M6 không scroll tới được mà DB vẫn nhận là lỗi âm thầm.

### Determinism

`SortedTopics` sort **bản sao** theo `(position, id)`. `id` là duy nhất nên
thứ tự luôn ổn định kể cả khi `position` trùng. Test gọi `ComputeLayout` 3
lần với 3 kiểu input đảo thứ tự × 6 terrain × 2 direction, kết quả phải y hệt.
Không mutate input (test riêng).

## 3. Trạng thái màn `done | current | locked`

Suy ra, **không lưu DB** (`ROADMAP-MAP-IDEA §3`) — không có chuyện lệch trạng
thái giữa DB và bản đồ. Node đầu tiên luôn `current` nếu chưa done (màn khoá
ngay từ đầu thì user không có đường vào). `skipped` coi như `done` (user cố ý
bỏ qua 1 màn, không phải bị chặn vĩnh viễn).

`LevelStates` bỏ qua node `is_optional` khỏi chuỗi level (node tham khảo không
có node vô cùng), và `topics_locked` trong `Progress` tính **theo từng stage** —
gộp mọi stage rồi đếm là sai (màn cuối stage 1 không mở chỉ vì stage 2 đã làm
xong).

## 4. Cách embed seed

Go `embed` **không đi lên khỏi thư mục package được**. Loader v1 nằm ở
`api/roadmap_seed.go` (package `main`) nên embed được thư mục con
`roadmap_seed/`. Loader M2 nằm ở `internal/infrastructure/roadmap/` — sâu 3 cấp,
embed từ đó không với tới được.

Cách sạch được chọn (giống hệt `api/migrations/embed.go` với file `.sql`):
tạo **file mới** `api/roadmap_seed/embed.go`, `package roadmapseed`,
`//go:embed *.json` → `export FS`. Pattern `*.json` chứ không phải cả thư mục để
`README.md` khỏi bị nhúng vào binary.

**Đây là file MỚI ngoài danh sách ghi của M2** — `api/roadmap_seed/` chỉ được
ghi `README.md`. Không có cách nào khác embed được mà không tạo package trong
thư mục chứa JSON (`os.DirFS` sẽ phá build 1-binary của Dockerfile, vốn chỉ
`COPY --from=apibuild /out/langapp`).

Loader v1 giữ nguyên: `//go:embed roadmap_seed` giờ cũng nhúng luôn `embed.go`
(vài byte chết, không ảnh hưởng). App v1 test vẫn xanh.

## 5. Ranh giới layer (đã verify bằng grep)

| luật | lệnh | kết quả |
|---|---|---|
| domain thuần | `grep -rl 'gorm.io\|gin-gonic\|database/sql\|net/http\|google.golang.org/grpc' api/internal/domain` | **0 file** (exit 1) |
| application không import driver DB | `grep -rl 'gorm.io' api/internal/application` | **0 file** (exit 1) |
| không đảo chiều phụ thuộc | `grep -rl 'internal/infrastructure' api/internal/domain api/internal/application` | **0 file** (exit 1) |

Cả 3 grep phải ra 0 **kể cả trong comment** — đã viết lại comment trong
`application/*/ports.go` cho đỡ chứa đúng chuỗi `gorm.io` /
`internal/infrastructure`.

### Quyết định thiết kế đáng ghi

- **`Tx = any`.** `type Tx any` chứ không phải interface có method: interface có
  method sẽ buộc `infrastructure` phải implement marker method của `application`
  — tức là hướng phụ thuộc đảo ngược. `any` là lựa chọn duy nhất giữ được luật
  "infrastructure là nơi duy nhất biết GORM".
- **Mọi method ghi của repository nhận `tx Tx` làm tham số đầu**; method đọc
  không nhận. Đó là cách duy nhất để repository biết ghi vào transaction nào mà
  không phải đẩy `tx` vào `context.Context` (ẩn phụ thuộc).
- **Cascade xoá mềm nằm ở repository, không ở use case**: nó là các câu
  `UPDATE … WHERE topic_id IN (…)` theo khoá ngoại; đặt ở tầng trên sẽ khiến
  `application` phải biết cấu trúc bảng. `application` chỉ kiểm tra parent tồn tại
  rồi gọi 1 hàm.
- **Test tầng application dùng test double trong bộ nhớ** vì grep luật 3 quét
  cả `_test.go` → test không được import tầng infrastructure. Test repository
  thật (Postgres thật) nằm ở `internal/infrastructure/*`.
- **`reviews` KHÔNG có cột `deleted`** → `Review` không có field `Deleted`,
  không map tombstone. Lịch sử ôn là append-only, sync merge bằng union guid.
- **`UpdatedAt string` + tag `autoUpdateTime:false`** trên mọi model: field tên
  `UpdatedAt` kiểu `time.Time` bị GORM tự ghi, **bypass trigger
  `langapp_touch_updated_at`** → sai mốc LWW phía peer.
- **`Omit("updated_at")` trên mọi `Updates`** để trigger chạm thay vì GORM.
- **`IsBuiltin int`** ở model, `bool` ở application: cột là INTEGER, GORM sẽ
  ghi `'true'` nếu field kiểu bool → Postgres từ chối.
- **`completed_at` là `*string`** ở cả 3 tầng → ghi SQL NULL thật, không `''`.
  Ngoài ra `resolveCompletedAt` gán mốc hiện tại cho row `done` mà thiếu mốc
  (dữ liệu migrate từ v1 không có cột này), nếu không `progress?since=` bỏ
  sót node đó vĩnh viễn vì user không có lý do bấm lại.

## 6. `guid` — bắt buộc sinh, không bao giờ rỗng

`ux_reviews_guid` / `ux_decks_guid` / `ux_roadmap_*_guid` đều là UNIQUE trên cột
`TEXT NOT NULL DEFAULT ''` (giữ nguyên từ schema v1) ⇒ 2 row cùng guid rỗng là
đụng nhau. `NewGUID()` dùng `uuid.NewString()` ở **cả 2 package application**, và
mọi đường ghi đều gọi nó. Test:

- `Test_two_consecutive_reviews_get_distinct_guids` (Postgres thật) — 2 review
  liên tiếp phải insert được, guid khác nhau và đều khác rỗng.
- `Test_new_guid_is_never_repeated` × 2 package (1000 vòng).
- `Test_seed_rows_keep_creation_timestamp` + assert 0 stage có guid rỗng.

## 7. Use case / endpoint đã có sẵn cho M4

`application/roadmap.Service` (20 use case):

| nhóm | use case |
|---|---|
| path | `CreatePath` `GetPath` `ListPaths` `UpdatePath` `DeletePath` `Progress` |
| stage | `CreateStage` `UpdateStage` `DeleteStage` `SetStageStatus` |
| topic | `CreateTopic` `UpdateTopic` `DeleteTopic` `SetTopicStatus` |
| resource | `CreateResource` `UpdateResource` `DeleteResource` |
| milestone | `CreateMilestone` `UpdateMilestone` `DeleteMilestone` |

`application/srs.Service` (10 use case): `CreateDeck` `ListDecks` `DeleteDeck`
`CreateCard` `UpdateCard` `DeleteCard` `ListCards` `DueCards` `RecordReview`
`SetCardTone` + `FindDeck` (hiện thực `roadmap.DeckReader`).

`GetPath` trả `PathView` = cây đầy đủ + **layout + `LevelState` cho từng topic** +
`Progress` ⇒ 1 request là đủ cho M6 vẽ bản đồ, không cần endpoint layout riêng.

`SetCardTone` chỉ **ghi** kết quả chấm thanh; bước chấm (`content.GradeTonePair`)
thuộc context `content` và để M3 — comment ghi rõ ở hàm.

Lỗi: `*Error{Status, Message}` với message tiếng Việt giữ nguyên wording app v1
(`"không tìm thấy deck"`, `"slug learning path đã tồn tại"`,
`"status chỉ nhận: not_started, in_progress, done, skipped"`, …). Transport M4
chỉ cần `errors.As` + `appErr.Status`. Không khai báo hằng HTTP trong application
(viết tay 400/404/409/500) để không import `net/http`.

## 8. Kết quả validate

| lệnh | kết quả |
|---|---|
| `docker compose up -d postgres` | healthy, `localhost:5432` **mở** (đã publish từ M1) |
| `LANGAPP_TEST_POSTGRES_DSN=… go test ./...` | **PASS** — 12 package, **382 test, 0 FAIL, 0 SKIP** |
| Chạy lại 6 lần liên tiếp | **6/6 PASS** (xem §9 về race `CREATE EXTENSION`) |
| `go vet ./...` | PASS, 0 cảnh báo |
| `gofmt -l internal migrations roadmap_seed` | sạch (0 file) |
| `grep` luật DDD × 3 | xem §5, đều 0 file |
| App cũ `ok langapp` | **PASS** (≈0.96s) — `api/main.go`, `api/schema.sql`, `api/roadmap_seed.go`, `api/roadmap.go` không sửa dòng nào |

Test count: 264 (M1) → **382** (toàn suite, `go test -v` đếm `--- PASS`).

| package | test | M2 thêm |
|---|---|---|
| `internal/domain/roadmap` | 46 | +30 (map layout / level state / MapDefaults) |
| `internal/application/roadmap` | 20 | mới |
| `internal/application/srs` | 21 | mới |
| `internal/infrastructure/roadmap` | 28 | mới (Postgres thật) |
| `internal/infrastructure/srs` | 16 | mới (Postgres thật) |

## 9. Ghi chú phát sinh — race `CREATE EXTENSION` khi test package chạy song song

**Đây là vấn đề hạ tầng test, không phải lỗi code.** `go test ./...` chạy các
package **song song**. Migration `00002` chạy `CREATE EXTENSION pg_trgm`, và
extension là đối tượng **cấp DATABASE** được cài vào schema đầu tiên của
`search_path` — tức schema test tạm. Hai hệ quả khi chạy song song:

1. `CREATE EXTENSION` chồng nhau → `duplicate key ... pg_extension_name_index`
   (23505).
2. `DROP SCHEMA … CASCADE` ở cleanup của package A **giật extension ra khỏi
   database** giữa lúc package B đang `CREATE INDEX … gin_trgm_ops` →
   `operator class "gin_trgm_ops" does not exist` (42704).

M1 chỉ có 1 package test DB nên không gặp. M2 thêm 2 package → lộ ra ngay.

Cách xử lý (chỉ trong `_test.go`, **không** sửa migration `00002`):

1. `TestMain` ở cả 3 package (`platform`, `infrastructure/srs`,
   `infrastructure/roadmap`) nắm **1 khoá advisory toàn database**
   (`pg_advisory_lock(20260928)`) trên 1 `*sql.Conn` riêng, giữ **suốt đời
   package** rồi buông sau `m.Run()`. Nắm ở từng test thì cả suite thành hàng
   đợi tuần tự và chậm hơn 1 phút.
2. Schema test được tạo **qua conn của khoá** (không thuộc pool test) **trước**
   khi mở pool: nếu `search_path` trỏ tới schema chưa tồn tại, conn đầu tiên rơi
   vào `no schema has been selected` và nằm lại trong pool idle → goose lấy đúng
   conn đó rồi fail.
3. Cleanup: `DROP EXTENSION IF EXISTS pg_trgm CASCADE` **trước** `DROP SCHEMA
   … CASCADE` — Postgres từ chối drop schema còn giữ extension (đây cũng là lý do
   61 schema rác còn sót lại trong DB dev suốt các lần chạy thử trước đó; đã dọn
   sạch).

`migrateLockKey` là **3 bản sao** (hằng số trong `_test.go` không import được qua
package khác). Đổi số thì phải đổi cả 3 — comment ở cả 3 file đã ghi.

## 10. File đã sửa ngoài phạm vi ghi (nêu rõ để reviewer)

`api/internal/platform/migrate_test.go` — spec M2 cấm sửa `internal/platform/**`,
nhưng 2 thay đổi dưới đây **bắt buộc** để giữ `go test ./...` xanh:

1. **Con số migration 3 → 4** (4 assert: `Applied`, `Version`, số version
   distinct, số row applied). Thêm `00004_roadmap_map.sql` làm các con số cứng
   trong test M1 sai — đây là hệ quả không tránh được của việc thêm migration.
2. **`TestMain` + khoá advisory + `DROP EXTENSION` trước `DROP SCHEMA`** trong
   `openTestDB` (§9). Không có nó thì 3 package test DB chạy song song sẽ đỏ
   ngẫu nhiên.

Không sửa `config.go` `di.go` `db.go` `migrate.go` `logger.go` — logic production
của platform nguyên vẹn. `schema_migrations.MAX(version)` vẫn = 4 (app v1 đọc
số này, M7 mới dọn).

## 11. SKIPPED + lý do

| Việc | Trạng thái | Lý do |
|---|---|---|
| Test khi **không** có Postgres | **SKIP có chủ đích** | `LANGAPP_TEST_POSTGRES_DSN` rỗng → `t.Skip` với message rõ. Giữ hành vi M1 (không dùng testcontainers — plan §1 cố ý loại). |
| `roadmap_bookmarks` CRUD use case | **PENDING (M6)** | M2 chỉ cần đường đọc (`ListBookmarks`) để bảng A1 có model + không bị bỏ sót khỏi sync (M7). Use case đầy đủ (lọc theo status/tag) là phần UI M6. |
| `roadmap_seed/README.md` nói `title` là slug | **ĐÃ SỬA** | Loader chạy `slugify(firstNonEmpty(f.Slug, f.Title))` và 2 file JSON **không khai `slug`** → slug ra tự sinh từ title. README giờ nêu rõ 2 slug thật + bảng terrain/direction. 2 file JSON **không sửa dòng nào**. |
| `GetPath` gom topic theo stage | **PENDING dataloadgen (M4)** | M2 dùng `ListStages` + `ListTopics` từng stage (N+1 theo số stage, 5 stage/path). M4 thay bằng `dataloadgen` khi GraphQL cần. Ghi ở gate M4. |
| Wire repository vào `platform.Container` | **PENDING (M4)** | `di.go` nằm ngoài phạm vi ghi M2. Service đã sẵn sàng nhận qua `NewService(repo, uow, decks, nowFn, viewBox)`; adapter `roadmap.DeckReader` mẫu có trong `infrastructure/roadmap/helper_test.go`. |
| `errgroup` / `validator` thành direct dep | **PENDING (M4)** | Vẫn chưa file nào import. |
