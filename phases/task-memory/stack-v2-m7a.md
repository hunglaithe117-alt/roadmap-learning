# stack-v2 M7a — enum contract (`enum Terrain` + chiều ghi)

> Worker: fixer. **GATE M6 yêu cầu revert toàn bộ phần backup/restore
> (`pg_dump`) — đã revert. Phần còn lại đúng phạm vi: enum.**
> Phạm vi ghi: **`api/**` + `web/src/**` + file bàn giao này + `.slim/deepwork/stack-v2.md`.
> **KHÔNG** xoá `api/*.go` cũ (M7b).

## 0. Trạng thái bàn giao

| Việc | Trạng thái |
|---|---|
| `enum Terrain` khớp domain + CHECK | **XONG** |
| 3 việc gate M6 yêu cầu kèm | **XONG** (§1.4) |
| Lưới enum⇄domain⇄DB (10 test) | **XONG** (§1.3) |
| Gộp 3 export terrain ở `web/` về 1 nguồn | **XONG** (§2) |
| **backup/restore `pg_dump`/`pg_restore`** | **ĐÃ REVERT theo yêu cầu gate M7a.** `/api/backup` + `/api/restore` trở lại trả **501**. Chi tiết + 2 phát hiện còn treo ở §4. |

---

## 1. Bảng đối chiếu enum (schema ⇄ domain ⇄ DB CHECK)

Đọc **trực tiếp** `graph/schema/*.graphqls` + file migration `.sql`, không đọc
bản codegen (đọc bản sinh thì `go generate` chép lại lỗi và test xanh).

| # | Enum (file) | Giá trị schema | Hằng domain | CHECK ở DB | Trạng thái |
|---|---|---|---|---|---|
| 1 | `ErrorCode` (`common`) | `BAD_REQUEST` `NOT_FOUND` `CONFLICT` `INTERNAL` | `application/<ctx>.Error.Status` 400/404/409/500 | — | **khớp** (không phải whitelist danh sách — kiểm riêng bằng `codeForStatus`) |
| 2 | `Status` (`common`) | `NOT_STARTED` `IN_PROGRESS` `DONE` `SKIPPED` | `domain/roadmap.AllStatuses` (4) | `00001` `roadmap_stages.status`, `roadmap_topics.status` (4) | **khớp** |
| 3 | `LevelState` (`common`) | `DONE` `CURRENT` `LOCKED` | `domain/roadmap.AllLevelStates` (3) | — (suy ra trong code) | **khớp** |
| 4 | `ResourceKind` (`common`) | `VIDEO`…`CHANNEL` (9) | `application/roadmap.AllKinds` (9) | **không có** — cột `kind TEXT NOT NULL DEFAULT ''` | **khớp 1 nguồn** (xem §1.2) |
| 5 | `Terrain` (`common`) | ~~`PLAIN` `FOREST` `HILL` `MOUNTAIN` `WATER` `DESERT`~~ → **`MEADOW` `DESERT` `SNOW` `VOLCANO` `OCEAN` `CITY`** | `domain/roadmap.AllTerrains` (6) | `00004` `roadmap_stages.terrain` (6) | **ĐÃ SỬA — 1 dòng lệch** |
| 6 | `Direction` (`common`) | `UP` `RIGHT` | `domain/roadmap.AllDirections` (2) | `00004` `roadmap_stages.direction` (2) | **khớp** |
| 7 | `BookmarkStatus` (`roadmap`) | `TO_READ` `READING` `DONE` `ARCHIVED` | `domain/roadmap.AllBookmarkStatuses` (4) | `00003` `roadmap_bookmarks.status` (4) | **khớp** |
| 8 | `ChunkKind` (`content`) | `CONTENT` `FUNCTION` | `domain/content.SplitChunks` sinh ra 2 loại | — (hằng trả về, không lưu) | **khớp** |
| 9 | `WordStatus` (`practice`) | `OK` `WRONG` `MISSING` `EXTRA` | `domain/practice.WordDiff` sinh ra 4 loại | — (kết quả trong RAM) | **khớp** |

`srs` / `insight` / `sync` **không có enum** — mọi trường là `String!` vì cột DB
là `TEXT` và v1 đã đóng băng hợp đồng đó.

**Kết luận: 9 enum, 1 dòng lệch (`Terrain`), đã sửa.** 8 enum còn lại đã khớp
từ trước — nhưng **không test nào chứng minh điều đó**, nên "đã khớp" chỉ là
quan sát bằng mắt, và chính vì vậy lần sau vẫn có thể lệch.

### 1.1 Bug `Terrain` — hậu quả thật (không phải chỉ "tên sai")

Chuỗi lỗi gốc, đọc từ DB lên UI rồi ghi ngược:

```
DB lưu "snow" → terrainOf("snow") khớp 0 case → default trả "PLAIN"
               → client đổi thành "Cao nguyên" → terrainText trả "hill"
               → CHECK constraint 00004 từ chối (23514) hoặc ghi sai địa hình
```

Nếu `web` chưa bọc bảng lách (trước M6b): **UI không lưu được 5/6 lựa chọn
địa hình**, chỉ ghi được `DESERT`.

### 1.2 `roadmap_resources.kind` không có CHECK ở DB — ghi nhận, KHÔNG sửa ở M7a

Cột `kind TEXT NOT NULL DEFAULT ''` **không có `CHECK IN (...)`** ở bất kỳ
migration nào. Whitelist chỉ nằm ở `application/roadmap.AllKinds` (1 nguồn)
nên chưa có "2 nguồn sự thật" để lệch — nhưng merge của `sync` ghi thẳng SQL
(`UpsertResource` set `updated_at` tường minh) nên dữ liệu peer mang `kind`
lạ sẽ vào DB không bị chặn.

**Không thêm CHECK ở M7a** vì cần migration mới (bump goose 5→6 ⇒ sửa
`migrate_test.go`) và phải xử lý `DEFAULT ''` trong danh sách. `enumSpecs`
khai `dbUnchecked` + `Test_db_unchecked_column_reason_is_recorded` bắt buộc
phải ghi lý do, nên "không có backstop" là **quyết định đã ghi**, không phải
sơ suất. **Nợ → M7b.**

### 1.3 Lưới an toàn mới (2 file test, 14 test)

`api/internal/transport/graphql/enum_contract_test.go`
`api/internal/transport/graphql/enum_roundtrip_test.go`

Đọc **trực tiếp** `graph/schema/*.graphqls` + file migration `.sql` — không đọc
bản codegen (đọc bản sinh thì `go generate` chép lại lỗi và test xanh).

| Test | Chiều | Bắt được gì |
|---|---|---|
| `Test_graphql_enum_has_matching_domain_constant` | schema → domain | enum có giá trị domain không có (bug gốc) |
| `Test_domain_constant_has_matching_enum_value` | domain → schema | thêm hằng lạ vào domain mà quên schema |
| `Test_graphql_enum_matches_db_check_constraint` | domain ⇄ DB | CHECK lệch whitelist |
| `Test_every_schema_enum_is_covered_here` | schema → bảng test | enum MỚI mà không ai đối chiếu |
| `Test_db_unchecked_column_reason_is_recorded` | bảng test | enum lưu DB mà không ghi lý do không có CHECK |
| `Test_graphql_error_code_covers_business_status` | enum ⇄ HTTP status | code chết / gộp 2 status vào 1 code |
| `Test_terrain_survives_db_to_client_to_db_roundtrip` | DB → client → DB | `terrainOf` rơi về default |
| `Test_terrain_text_of_every_enum_value_is_valid_db_value` | enum → DB | enum thừa ⇒ DB từ chối |
| `Test_terrain_of_rejects_out_of_range_values` | DB hỏng → client | trả enum không tồn tại ⇒ serialize 500 |
| `Test_terrain_text_rejects_value_it_cannot_map` | **chiều GHI** | **giá trị lạ bị TỪ CHỐI, không rơi về DEFAULT** |
| `Test_terrain_text_accepts_absent_and_default` | chiều ghi | `nil` + `MEADOW` vẫn hợp lệ, không hỏng khi sửa |
| `Test_direction_survives_db_to_client_to_db_roundtrip` | DB → client → DB | enum kế bên |
| `Test_direction_text_rejects_value_it_cannot_map` | **chiều GHI** | hướng đi lạ không rơi về `"right"` |
| `Test_direction_text_maps_right_explicitly` | chiều ghi | chặn "fix" quá tay biến `RIGHT` thành lỗi |

Quy tắc ánh xạ tên: `strings.ToUpper(domainValue) == enumValue` — dùng quy
tắc (không bảng tra) để lỗi đổi tên hiện ra ở **cả 2 chiều**.

---

## 1.4 Ba việc gate M6 yêu cầu kèm

### (1) `roadmap_tree_test.go` khẳng định điều sai

Cũ: `require.Equal(t, "PLAIN", stage.Terrain, "DEFAULT của cột terrain")`.
Xanh vì `terrainOf` rơi về `PLAIN` cho mọi giá trị — **test tự khẳng định điều
sai**, đúng như bạn nêu.

Mới: `require.Equal(t, "MEADOW", …)`. Đồng thời assertion layout đổi từ
`x == 500.0` (tuyệt đối, chỉ đúng khi biên độ = 0) sang `InDelta(500, x, 60)`
— vì `amp(meadow) = 40` nên node dao động quanh trục giữa, khẳng định tuyệt đối
500 sẽ lại là 1 test xanh nhầm khác.

### (2) `web/` — gộp 3 export về 1 nguồn sự thật

Trước có **3 danh sách/kiểu độc lập** cùng nói về 6 địa hình, và 1 trong đó
lệch hẳn server:

| Nơi | Trước | Vai trò |
|---|---|---|
| `api/graph/schema/common.graphqls` | `PLAIN…DESERT` | enum server (nguồn thật) |
| `web/src/graphql/operations.ts:705` | `MapTerrain = 'PLAIN'…` | **bản sao** |
| `web/src/roadmap/map/terrain.ts:21` | `TERRAINS = ['PLAIN'…]` | **bản sao thứ 2** |
| `web/src/roadmap/map/terrain.ts:41` | `TERRAIN_SKIN` | chất liệu, khoá là 6 tên thứ 3 |

Sau:

- **`operations.ts` giữ DUY NHẤT `MapTerrain`** — file đó tự khai kiểu payload
  ("khai lại ở đây để không import type server"), nên đây là chỗ đúng để đặt
  danh sách 6 tên. Đã đổi sang `MEADOW/DESERT/SNOW/VOLCANO/OCEAN/CITY`.
- **`terrain.ts` dùng `Record<MapTerrain, TerrainSkin>`** — hướng phụ thuộc đúng
  (UI biết kiểu API, không ngược lại). `Record` là **cái cớ compile-time**: thêm
  1 tên vào `MapTerrain` mà quên dòng skin ⇒ `vue-tsc` đỏ ngay.
- **`TERRAINS` suy ra từ `Object.keys(TERRAIN_SKIN)`** — xoá mảng tên rời.
  `StageForm.vue` vẫn dùng `TERRAINS` nên giữ export, nhưng không còn là nguồn.
- `type Terrain = MapTerrain` — giữ tên mà `MapCanvas.vue` đang import.
- Xoá comment shim "vá server sai" ở đầu `terrain.ts`.
- 9 file web đổi tên theo ánh xạ 1:1, giữ nguyên hình dạng dải nền đã chọn:
  `PLAIN→MEADOW`, `FOREST→CITY`, `HILL→SNOW`, `MOUNTAIN→VOLCANO`, `WATER→OCEAN`.

Thêm `test_terrain_skin_covers_exactly_the_server_enum` ở `terrain.test.ts`:
`vitest` **không** typecheck nên lệch 1 chiều mà `Record` bắt vẫn lọt qua
`vitest`; test này chặn nốt chiều runtime.

### (3) Mất dữ liệu ở CHIỀU GHI — đã sửa

Đúng như bạn nêu. Đường mất dữ liệu:

```
client chọn 1 địa hình mà switch của terrainText chưa biết
  → terrainText default trả ""
  → application/roadmap.parseMapField("", domain.ParseTerrain)
  → ParseTerrain("") coi rỗng là "lấy DEFAULT" và trả "meadow", KHÔNG LỖI
  → UPDATE ghi "meadow"
  → user thấy bản đồ đổi sang Đồng cỏ, KHÔNG có message lỗi nào
  → updated_at vẫn đổi nên node còn bị đẩy qua sync
```

Hôm nay enum GraphQL đóng (gqlgen chặn giá trị lạ) nên đường này **chưa kích
hoạt được từ client** — nó kích hoạt ngay khi ai đó thêm giá trị thứ 7 vào
`enum Terrain` mà quên cập nhật `switch`. Đúng cái lỗi đã xảy ra 2 lần trước
(F9 M2, F1 M4) và 1 lần ở chính `enum Terrain`.

Sửa: `terrainText` / `directionText` **trả `(string, error)`** và tách 3 trạng
thái thay vì gộp 2:

| Đầu vào | Kết quả | Vì sao |
|---|---|---|
| `nil` | `("", nil)` | client không gửi |
| `MEADOW` | `("", nil)` | DEFAULT — `ParseTerrain("")` gán `meadow` |
| giá trị hợp lệ | `("tên", nil)` | ghi tường minh |
| **giá trị lạ** | **`("", lỗi 400)`** | **từ chối, kèm liệt kê 6 giá trị hợp lệ** |

`directionText` có **cùng lỗi và nặng hơn**: trước đây rơi về `"right"` cho
mọi giá trị lạ — tức KHÔNG phải DEFAULT, nên bản đồ vẽ sai *chiều cuộn* mà
không lỗi nào. Đã sửa cùng lúc (cùng file, cùng cặp hàm, cùng dạng bug).

Cả 2 call site ở `root.resolvers.go` (`CreateStage` + `UpdateStage`) đã trả
`toUserError(err)` ⇒ 400 kèm message tiếng Việt liệt kê giá trị hợp lệ.

---

## 2. `web/` — verify

| Lệnh | Kết quả |
|---|---|
| `npx vue-tsc --noEmit` | sạch |
| `npx vitest run` | **38 file / 364 test PASS** (M6b: 342) |
| `npx vite build` | OK |

---

## 3. Phần backup/restore — ĐÃ REVERT

Gate yêu cầu bỏ. Đã xoá 5 file và revert 7 file:

**Xoá:** `infrastructure/sync/pgbackup.go` · `infrastructure/sync/restore_loader.go` ·
`infrastructure/sync/pgbackup_test.go` · `transport/http/maintenance.go` ·
`transport/http/backup_test.go`

**Revert:** `transport/http/backup.go` (về 501) · `transport/http/router.go`
(bỏ `Options.Maintenance` + `e.Use`) · `transport/http/routes.go` (bỏ
`maintenanceSkipPaths`, `restoreHandler` 1 tham số) ·
`transport/http/router_test.go` (assert `"M7"`) · `platform/config.go` (bỏ 3
field) · `platform/di.go` (bỏ `Container.Backup` + `wireBackup`) ·
`platform/di_test.go` (bỏ 2 test) · `application/sync/ports.go` (doc gốc) ·
`cmd/langapp/main.go` (`Backup: nil`)

`grep -rn "PgDump|PgRestore|PeerDump|pgbackup|restore_loader|Maintenance|
ErrToolMissing|ErrInvalidDump|LANGAPP_PEER_DUMP" api/` → **0 hit**. Endpoint trở
lại trả **501** với message gốc.

### 3.1 Hai phát hiện CÒN TREO (đã tìm ra, KHÔNG sửa vì nằm ngoài enum)

Cả hai là **typed-nil interface** — cùng lớp lỗi với `txOf` của M2 (F2).

**B1 — `Container.Backup` khai bằng kiểu con trỏ.**
`wireBackup` bỏ trống khi thiếu binary ⇒ con trỏ nil truyền vào
`Options.Backup` (interface) thành interface **KHÔNG nil** ⇒ handler kiểm
`port == nil` thành sai ⇒ gọi method trên con trỏ nil ⇒ **panic 500 thay vì
501**. Sửa: khai `Container.Backup` là `syncapp.BackupPort`.

**B2 — `Wire` truyền `NewSchemaLoader(nil)` cho `NewService`.**
`s.loader == nil` thành sai ⇒ `mutation.sync` đi vào `SchemaLoader.Load` với
`peer` nil ⇒ **panic 500** thay vì lỗi nghiệp vụ "chưa cấu hình nguồn snapshot
peer". Sửa: truyền `nil` thật.

Cả hai sửa đều **1 dòng** và **không thuộc phần enum**, nên đã revert theo yêu
cầu. M7b làm lại sync sẽ phải sửa B2 dù muốn hay không — nên đưa vào việc M7b
ngay, không cần làm lại `pg_dump` trước.

---

## 4. Verify

| Lệnh | Kết quả |
|---|---|
| `go test ./...` × 3 lần | **26 package ok, 0 FAIL** × 3 |
| `go build ./...` | OK |
| `go vet ./...` | sạch |
| `gofmt -l .` | chỉ 2 file cũ được phép (`chinese_test.go`, `reader.go`) |
| grep luật DDD #1 (`domain/` × gorm/gin/grpc) | **0 file** |
| grep luật DDD #2 (`application/` × gorm/gin/grpc) | **0 file** |
| grep luật DDD #3 (import thật qua `go list -deps`) | **0** |
| `go test .` (app cũ `api/*.go`) | **ok langapp** |
| `go generate ./...` | idempotent |
| `vue-tsc --noEmit` | sạch |
| `vitest run` | 38 file / **364 test PASS** |
| `vite build` | OK |

### Mutation-check — 7/7 FAIL thật

| # | Mutation | Test bắt |
|---|---|---|
| 1 | thêm `TerrainSwamp` vào domain (chưa khai schema) | `Test_domain_constant_has_matching_enum_value`, `Test_graphql_enum_matches_db_check_constraint`, `Test_terrain_survives_…` |
| 2 | đổi `VOLCANO` → `VOLCANO_X` trong schema | `Test_graphql_enum_has_matching_domain_constant/Terrain` |
| 3 | `terrainText` `default` trả `""` (đúng bug gốc) | `Test_terrain_text_rejects_value_it_cannot_map` |
| 4 | `directionText` rơi về `"right"` (đúng bug gốc) | `Test_direction_text_rejects_value_it_cannot_map` |
| 5 | thêm `SWAMP` vào schema, codegen lại, chưa sửa `switch` | `Test_graphql_enum_has_matching_domain_constant/Terrain`, `Test_terrain_text_of_every_enum_value_is_valid_db_value/SWAMP` |
| 6 | thêm `SWAMP` vào **cả schema lẫn domain**, quên `switch` (kịch bản thật nhất) | `Test_terrain_survives_db_to_client_to_db_roundtrip/swamp`, `Test_terrain_text_of_every_enum_value_is_valid_db_value/SWAMP` |
| 7a | đổi 1 khoá trong `TERRAIN_SKIN` | `test_terrain_skin_covers_exactly_the_server_enum` |
| 7b | thêm `'SWAMP'` vào `MapTerrain` | `vue-tsc` (2 lỗi `TS2741`) — `vitest` **không** bắt, vì không typecheck |

Mutation 7a/7b cho thấy 2 lớp lưới là cần cả hai: `vue-tsc` bắt chiều
compile-time, test runtime bắt chiều còn lại.

---

## 5. File đã sửa

### `api/`
- `graph/schema/common.graphqls` — `enum Terrain` 6 tên đúng.
- `internal/transport/graphql/convert.go` — `terrainOf` (đọc) + `terrainText` /
  `directionText` (ghi, **trả error**) + `terrainNames`.
- `internal/transport/graphql/root.resolvers.go` — 3 call site xử lý lỗi.
- `internal/transport/graphql/model/models_gen.go`, `generated/generated.go` — codegen.
- `internal/transport/graphql/roadmap_tree_test.go` — `PLAIN` → `MEADOW` + assert biên độ.

### Thêm
- `api/internal/transport/graphql/enum_contract_test.go`
- `api/internal/transport/graphql/enum_roundtrip_test.go`

### `web/`
- `src/graphql/operations.ts` — `MapTerrain` 6 tên đúng + comment nguồn.
- `src/roadmap/map/terrain.ts` — `Record<MapTerrain, TerrainSkin>`, `Terrain =
  MapTerrain`, `TERRAINS` suy ra, xoá comment shim.
- `src/roadmap/map/terrain.test.ts` — +1 test chốt nguồn thật.
- `src/roadmap/StageForm.vue`, `src/roadmap/map/MapCanvas.vue` (+ comment) —
  tên mới.
- 4 file test: `api.test.ts`, `MapCanvas.test.ts`, `RoadmapMap.test.ts`,
  `mutationInvalidation.test.ts` — tên mới.

**KHÔNG sửa 1 dòng `api/*.go` cũ.**

---

## 6. Nợ còn lại

| # | Việc | Vì sao |
|---|---|---|
| 1 | **`B2`: `Wire` truyền `NewSchemaLoader(nil)`** ⇒ `mutation.sync` panic 500 | §3.1, sửa 1 dòng, thuộc M7b |
| 2 | `B1`: `Container.Backup` kiểu con trỏ ⇒ panic thay vì 501 | §3.1, chỉ thành hiện khi có `pg_dump` |
| 3 | `roadmap_resources.kind` chưa có CHECK ở DB | §1.2 |
| 4 | `bookmarkStatusText` / `resourceKindText` còn dạng "rơi về default" | `bookmarkStatusText` rơi về `to_read`; `resourceKindText` **tình cờ an toàn** vì `ValidateKind` từ chối ⇒ 400. Cùng lớp bug (3), chưa sửa vì gate chỉ nêu terrain |
| 5 | `pg_dump`/`pg_restore` + `Dockerfile` cài `postgresql-client` | ĐÃ REVERT, M7b làm lại |
| 6 | `DEPLOY.md` + `THIRD-PARTY-LICENSES` | M7 |

