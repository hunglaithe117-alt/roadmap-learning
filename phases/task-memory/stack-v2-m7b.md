# STACK-V2 — M7b (5 việc chất lượng còn treo)

> Worker: fixer. Ngày 2026-09-29.
> Phạm vi ghi: **`api/**`** + **`web/src/**`** + file bàn giao này +
> `.slim/deepwork/stack-v2.md`.
> **KHÔNG** xoá code cũ · **KHÔNG** sửa `DEPLOY.md` · **KHÔNG** thêm CI —
> 3 việc đó thuộc M7c.

## 0. Kết quả

| # | Việc | Trạng thái |
|---|---|---|
| 1 | `409` sai ngữ nghĩa cho "chưa cấu hình" | **XONG** — 501 + `ErrorCode.NOT_IMPLEMENTED` |
| 2 | `typednil.Is` ngữ nghĩa nil Map/Slice | **XONG** — **đã thu hẹp** (khảo sát ở §2) |
| 3 | `bookmarkStatusText` mẫu tiềm ẩn của F1 | **XONG** — tách 3 trạng thái, giá trị lạ → 400 |
| 4 | `DIRECTIONS` export chết | **XONG** — xoá export, typecheck sạch ⇒ **xác nhận chết** |
| 5 | Thứ tự `TERRAINS` là hợp đồng UI | **XONG** — assertion khớp đúng thứ tự |
| Verify | Go 3 lần + web + mutation-check | **XONG** — §6, §7 |

---

## 1. Việc 1 — `409` → `501` (chưa cấu hình KHÔNG phải merge conflict)

### 1.1 Vì sao 409 sai

409 Conflict nghĩa là **"yêu cầu mâu thuẫn với trạng thái hiện tại"** — đúng
cho merge thật sự gặp xung đột LWW giữa local và peer. Nhánh `Service.Sync`
không có mâu thuẫn nào: app v1 đơn giản là **chưa có** nguồn snapshot peer,
và request bị từ chối **trước khi đọc/ghi bất cứ thứ gì**.

Client nhận 409 sẽ hiểu *"dữ liệu của tôi xang đột với peer"*. Đó là kết
luận sai mà user **không cách nào tự kiểm chứng**, vì app không có peer để
đối chiếu. Nghi ngờ của gate M6 là đúng: 409 ở M7a được chọn để **thoát assert
`NotEqual("INTERNAL")`**, không phải vì đúng nghĩa — bằng chứng nằm ở
`stack-v2-typednil.md` §1.4: 500 bị `toUserError` che thành `"lỗi hệ thống"`,
nên 409 chỉ là cách thoát assert, và 409 là mã nghiệp vụ đầu tiên trong tầm
tay lúc đó.

### 1.2 Chọn 501, không chọn 503 — và vì sao không chọn 409

| Mã | Nghĩa | Đúng không |
|---|---|---|
| 409 Conflict | yêu cầu mâu thuẫn trạng thái hiện tại | **SAI** — không có mâu thuẫn |
| 503 Service Unavailable | tạm thời không phục vụ được, **thử lại sau** | **SAI** — thiếu cấu hình thì thử lại bao nhiêu lần cũng y hệt; bảo user "thử lại" là bảo họ làm vô ích |
| **501 Not Implemented** | server **chưa có cách** phục vụ việc này | **ĐÚNG** |

501 còn có thêm 1 lý do quyết định: **nhất quán với `/api/backup` và
`/api/restore`**, vốn đã trả 501 + message `"tính năng … chưa bật"` cho đúng
tình huống "chưa bật" (`internal/transport/http/backup.go:154`). Cùng một
tình huống, cùng một mã, hai protocol.

### 1.3 Hệ quả bắt buộc: thêm `ErrorCode.NOT_IMPLEMENTED`

Đổi status mà **không** sửa tầng transport thì M7c sẽ bị đảo ngược: `isBusinessStatus`
chỉ liệt kê 400/404/409 ⇒ 501 rơi về `INTERNAL` ⇒ `toUserError` che message
tiếng Việt thành `"lỗi hệ thống"` ⇒ **sửa xong panic thì user lại không đọc
được vì sao**. Đó đúng là bài toán M7c đã sửa, nên phải sửa trọn:

| Tầng | Sửa |
|---|---|
| `application/sync/service.go` | thêm hằng `StatusNotImplemented = 501`, dùng cho nhánh loader nil |
| `graph/schema/common.graphqls` | thêm `NOT_IMPLEMENTED` vào `enum ErrorCode` |
| `transport/graphql/errors.go` | `isBusinessStatus` + `codeForStatus` có nhánh 501 |
| `transport/graphql/{generated,model}` | `go generate` (sinh lại) |

**Vì sao cần hằng enum RIÊNG, không gộp vào `CONFLICT` hay `INTERNAL`:**
client dùng `code` để **quyết định hiển thị**, không chỉ để hiện chữ. Ba hành
vi khác nhau: "bạn gửi sai" · "tính năng đang xây" · "server hỏng, thử lại".
Gộp 501 vào `INTERNAL` thì UI bảo user thử lại một việc sẽ không bao giờ
thành công; gộp vào `CONFLICT` thì UI báo dữ liệu xung đột. `NOT_IMPLEMENTED`
cũng khớp đúng mã mà tầng REST đã dùng (`web/src/rest/client.ts:38`) — 2
protocol giờ nói chung 1 ngôn ngữ.

### 1.4 Test

| File | Test | Bắt gì |
|---|---|---|
| `application/sync/typednil_loader_test.go` | `Test_Sync_never_reports_conflict_for_missing_peer` (2 sub) | **501**; cấm **409** · cấm **503** · cấm **500** cho **mọi** hình nil loader |
| | `Test_Sync_missing_peer_message_stays_vietnamese` | message tiếng Việt còn nguyên (nếu không, đổi mã chỉ là đổi số) |
| | `Test_Sync_rejects_typed_nil_loader` (+`NotEqual 500`) | bổ sung `Equal(StatusNotImplemented)` ở `Test_Sync_rejects_plain_nil_loader` |
| `transport/graphql/enum_roundtrip_test.go` | `Test_graphql_error_code_covers_business_status` | thêm 501→`NOT_IMPLEMENTED`; **503 phải là `INTERNAL`** |
| | `Test_sync_missing_peer_maps_to_not_implemented_not_internal` | `toUserError` giữ message + ra code đúng |
| `transport/graphql/typednil_sync_test.go` | `Test_sync_mutation_returns_business_error_…` | `code == "NOT_IMPLEMENTED"` + `NotEqual "CONFLICT"` |

**Ghi chú về 1 subtest bị bỏ so với bản nháp:** hình "con trỏ tới 1 interface
đang nil" **không dựng được** ở call site `NewService`, vì `Service.loader` có
kiểu cụ thể `SnapshotLoader` và Go không cho `*SnapshotLoader` thỏa
`SnapshotLoader` (compiler báo đúng). Ranh giới đó vẫn thật và vẫn được khoá ở
`internal/typednil` (`Test_Is_pierces_pointer_to_nil_interface`). Ghi rõ ở
comment thay vì giữ 1 dòng không compile được.

---

## 2. Việc 2 — `typednil.Is`: **ĐÃ THU HẸP**

### 2.1 Khảo sát call site (trả lời câu hỏi của đề bài)

`grep -rn "typednil\.Is"` toàn `api/` → **2 chỗ production**, không có chỗ thứ 3:

| # | Chỗ | Kiểu | Hiện thực | Truyền nil map/slice? |
|---|---|---|---|---|
| 1 | `application/sync/service.go:69` (`Service.Sync`) | `SnapshotLoader` (interface) | `*SchemaLoader` — **con trỏ** | **KHÔNG** |
| 2 | `transport/http/backup.go:151` (`backupDisabled`) | `BackupPort` (interface) | `*pgBackup` — **con trỏ** | **KHÔNG** |

Ngoài ra, **15 phép `iface == nil` còn lại** trong `internal/` đều là interface
port, không phải map/slice: `content.Service.{data,tone,decks,cards}`,
`practice.Service.{stt,tts}`, `roadmap.Service.decks`, `{srs,sync,content,
practice}.Service.uow`, `platform/di.go:275` (`c.DB`). Đây đúng là 14 chỗ ở
`stack-v2-typednil.md` §3 — khảo sát lại không phát hiện chỗ nào truyền
container.

⇒ **Không chỗ nào phụ thuộc ngữ nghĩa "nil Map/Slice = không dùng được".**

### 2.2 Vì sao bản cũ SAI về ngữ nghĩa Go

| Kiểu | `range` | `len` | `append` | ghi | kết luận |
|---|---|---|---|---|---|
| `nil map` | hợp lệ (0 vòng) | hợp lệ (0) | — | **PANIC** | chỉ GHI mới nổ |
| `nil slice` | hợp lệ (0 vòng) | hợp lệ (0) | **hợp lệ** | hợp lệ | hợp lệ hoàn toàn |

"Nil slice = chưa có dữ liệu" là **sai hoàn toàn**: `append(nil, x)` CHÍNH LÀ
cách khởi tạo slice trong Go. "Nil map = chưa dùng được" thì **nửa đúng**: ghi
thì nổ, nhưng đọc/`range`/`len`/`delete` đều chạy — nên khẳng định "không dùng
được" là sai, và nó biến `Is` thành nơi **trả lời thay một câu hỏi caller không
hỏi**.

### 2.3 Đã làm gì — và ranh giới trách nhiệm

`Is` **giờ chỉ soi `Ptr` / `Func` / `UnsafePointer` ở tầng ngoài cùng** +
`IsNil()`, cộng nhánh đào 1 tầng "con trỏ tới interface nil" (giữ nguyên).
Bỏ `Map` / `Slice` / `Chan`.

Lý do sâu hơn "đúng/sai từng case": hàm này tồn tại vì `== nil` trên interface
**trả lời sai câu hỏi** — nó hỏi *"interface này có chứa con trỏ nil không"*
chứ không phải *"có dùng được không"*. Đó là câu hỏi về **tầng ngoài cùng**.
`map`/`slice`/`chan` là **cái chứa bên trong**; muốn hỏi "ghi vào cái này được
không" thì phải hỏi bằng câu của `map`/`slice` (`if m == nil { m = map[...]... }`),
và câu đó **không phải câu mà hàm này sinh ra để trả lời**. Giữ 3 loại đó vào
chỉ làm hàm rộng hơn mà **không thêm sức bắt nào cho 3 lỗi đã gặp** (M2 F2 ·
M7a B2 · M7a B1 — đều là con trỏ).

### 2.4 Test khoá ranh giới

| Test | Khẳng định |
|---|---|
| `Test_Is_ignores_nil_map_and_slice` | `Is(nilMap) == false`, `Is(nilSlice) == false`, `Is(nilCh) == false`; map/slice **rỗng** vẫn `false` (không dịch chuyển) |
| `Test_Is_ignores_nil_map_hidden_behind_interface` | nil map **bọc trong `any`** cũng `false` — đúng hình thức nguy hiểm nhất mà bản cũ trả `true` |
| `Test_Is_keeps_usable_values` | giữ nguyên: mọi giá trị dùng được phải `false` (đối trọng "không chặn nhầm") |

`Test_Is_separates_nil_container_from_empty_container` cũ **bị thay** bằng 2
test trên, vì chính khẳng định của nó (`Is(nilMap) == true`) là thứ sai.

---

## 3. Việc 3 — `bookmarkStatusText` tách 3 trạng thái

### 3.1 Trạng thái "latent" xác nhận

`enum BookmarkStatus` (4 hằng) khớp domain 1-1, `enum_contract_test.go` khoá
⇒ **nhánh giá trị lạ chưa kích hoạt được từ client**. Nhưng thêm status thứ 5
vào enum là **đúng cơ chế F1** (M4): im lặng biến giá trị lạ thành giá trị
**hợp lệ**, không tín hiệu nào cho user.

Đường mất dữ liệu nếu không chặn (viết ra để người sau thấy nguy cơ, không
phải để trình bày):

```
client chọn status mới (chưa có trong switch)
  → bookmarkStatusText trả "to_read"
  → ValidateBookmarkStatus("to_read") HỢP LỆ ⇒ không lỗi
  → UPDATE ghi "to_read"
  → user thấy bookmark tự nhảy về "Chưa đọc", KHÔNG có message lỗi nào
  → updated_at vẫn đổi ⇒ dòng đó còn bị đẩy qua sync
```

### 3.2 Hàm mới — 3 trạng thái y hệt `terrainText`

```go
func bookmarkStatusText(s *model.BookmarkStatus) (string, error)
```

| Đầu vào | Kết quả |
|---|---|
| `nil` (client không gửi) | `("", nil)` — create: `ValidateBookmarkStatus("")` → DEFAULT `to_read`; lọc: **không lọc** |
| giá trị hợp lệ (4 hằng) | `(tên hằng domain, nil)` |
| giá trị lạ | `("", LỖI 400)` kèm **liệt kê đủ 4** giá trị hợp lệ |

**`TO_READ` trả TƯỜNG MINH `"to_read"`, KHÔNG rút về `""`** — khác `MEADOW` bên
`terrainText`, và đây là **bẫy thật** (mutation 3b chứng minh): `""` ở
`BookmarkFilter.Status` nghĩa là **không lọc**, nên nếu gộp thì
`bookmarks(status: TO_READ)` trả về **toàn bộ kho link** — sai kiểu dữ liệu,
không phải sai hiển thị.

### 3.3 3 call site + 1 helper chết

| Chỗ | Xử lý |
|---|---|
| `CreateBookmark` | `bookmarkStatusText(input.Status)`; lỗi → payload 400 |
| `SetBookmarkStatus` | `bookmarkStatusText(&status)` (tham số bắt buộc, không có nhánh `nil`); lỗi → payload 400 |
| `Query.Bookmarks` | `bookmarkStatusText(status)`; lỗi → GraphQL error |
| `helpers.go` `derefBookmarkStatus` | **XOÁ** — nó tồn tại để biến `nil` thành `TO_READ`, đúng cái việc mà 3 trạng thái làm tốt hơn. Giữ lại thành code chết và mời gọi sai. Đây là **duy nhất** chỗ bị xoá trong M7b. |

### 3.4 Test (6 hàm, 15 subtest)

`Test_bookmark_status_survives_db_to_client_to_db_roundtrip` (4) ·
`Test_bookmark_status_text_rejects_value_it_cannot_map` (6, **giá trị lạ bị
từ chối + 400 + liệt kê đủ 4 tên**) ·
`Test_bookmark_status_text_separates_absent_from_default` (`nil`→`""` vs
`TO_READ`→`"to_read"`) · `Test_bookmark_status_of_accepts_every_db_value` (6,
chiều đọc) — tất cả trong `transport/graphql/enum_roundtrip_test.go`.

**Bẫy test đã tránh 1 lần:** danh sách "giá trị lạ" đầu tiên có `"TO_READ"` và
test **ĐỎ** — vì ở **chiều ghi** đầu vào là *tên enum GraphQL*, nên `TO_READ`
hợp lệ; `"TO_READ"` chỉ lạ ở **chiều đọc** (`bookmarkStatusOf` nhận chuỗi trong
cột DB). Trộn 2 chiều vào 1 danh sách là loại test "xanh vì thứ vốn dĩ không
sai" — đã tách 2 danh sách và ghi rõ trong comment.

---

## 4. Việc 4 — `DIRECTIONS` export chết: **XOÁ, xác nhận chết**

Xoá `export const DIRECTIONS = ['UP', 'RIGHT'] as const` khỏi
`web/src/roadmap/map/terrain.ts` và thay bằng:

```ts
export type Direction = MapDirection;   // từ graphql/operations.ts
```

Cách này **không chỉ xoá code chết mà còn bỏ 1 nguồn sự thật thứ 2** — trước đó
`DIRECTIONS` khai lại danh sách hướng mà `MapDirection` đã có, và `Terrain =
MapTerrain` ngay cạnh đó đã làm đúng cho địa hình.

**Bằng chứng nó chết:** sau khi xoá, `pnpm typecheck` **sạch** và
`pnpm vitest run` **365/365 xanh** — không import nào đỏ. Import duy nhất là ở
`terrain.test.ts` (mutation 4 xác nhận thêm: thêm hướng thứ 3 vào
`MapDirection` mà quên cập nhật là `vue-tsc` **ĐỎ** nhờ bảng
`HORIZONTAL: Record<Direction, boolean>`).

**Hệ quả phải chấp nhận (ghi rõ):** union kiểu TS **không liệt kê được lúc
chạy**, nên sau khi bỏ mảng runtime thì `vitest` **không còn assert được
"đúng 2 hướng"**. Việc đó chuyển sang tầng type (`vue-tsc` + bảng `HORIZONTAL`).
Test cũ `expect(DIRECTIONS).toHaveLength(2)` — vốn là khẳng định vô nghĩa trên 1
export chết — thay bằng `test_exactly_one_of_two_directions_is_horizontal`, vẫn
giữ được ý "đúng 1 trong 2 hướng là cuộn ngang" mà không cần export thừa.

---

## 5. Việc 5 — thứ tự `TERRAINS` là hợp đồng UI

`TERRAINS = Object.keys(TERRAIN_SKIN)`, và `StageForm.vue:132` render
`<option v-for="t in TERRAINS">` ⇒ thứ tự trong `TERRAIN_SKIN` **quyết định thứ
tự chip địa hình trên UI**.

Test cũ chỉ so `.sort()` ⇒ **đảo thứ tự mảng mà test vẫn xanh**. Thêm
`test_terrains_order_is_the_ui_chip_order`, khoá **cả** `TERRAINS` lẫn
`Object.keys(TERRAIN_SKIN)` vào đúng
`MEADOW DESERT SNOW VOLCANO OCEAN CITY`, kèm comment nói rõ đây là hợp đồng
UI (Đồng cỏ — lựa chọn mặc định hợp lý nhất — đứng đầu; Thành phố — mức cao
nhất — đứng cuối).

**Không đổi thứ tự trong code**; test mới và code khớp ngay từ đầu. Mutation 5
đảo `MEADOW`↔`DESERT` trong `TERRAIN_SKIN` ⇒ test **ĐỎ** như yêu cầu.

---

## 6. Verify

### Go

| Lệnh | Kết quả |
|---|---|
| `docker compose up -d postgres` | healthy, `0.0.0.0:5433->5432/tcp` |
| `go test -count=1 ./...` (DSN 5433) × **3 lần liên tiếp** | **27 package ok · 894 PASS · 0 FAIL · 0 SKIP** × 3 |
| `go build ./...` | sạch |
| `go vet ./...` | 0 cảnh báo |
| `gofmt -l .` | chỉ 2 file cũ được phép giữ: `chinese_test.go`, `reader.go` |
| grep luật DDD #1 (`domain/` × gorm/gin/grpc/sql/http) | **0 import thật** (2 hit thô là **comment có sẵn từ trước** trong `internal/domain/audio/*.go` — dòng chú thích nói `transport/grpc` sẽ hiện thực; xác minh bằng `go list -deps` cho `./internal/domain/...` = rỗng) |
| grep luật DDD #2 (`application/` × gorm/gin/grpc) | **0** |
| grep luật DDD #3 (`domain`+`application` × `internal/infrastructure`) | **0** |
| `go list -deps -test ./internal/... ./cmd/... ./services/...` | **không còn `modernc.org/sqlite`**; `gorm`/`gin` chỉ ở transport+hạ tầng |
| `go list -deps -test ./internal/domain/... ./internal/application/...` | không `gorm.io` / `gin-gonic` / `grpc` (`database/sql/driver` chỉ đến từ `github.com/google/uuid` trong **test deps**, có từ trước) |
| `go test .` (app v1 `api/*.go`) | **ok langapp** |
| `go generate ./...` | **idempotent** — md5 `generated.go` + `models_gen.go` **không đổi** |

⚠️ **Ghi chú về `go generate`:** binary `gqlgen` **không có** trong `$PATH` ở máy
này ⇒ `go generate ./...` là **no-op** (giống các lane trước). Đã build thủ công
`gqlgen` từ module cache (offline, `GOPROXY=off`) để sinh lại code sau khi thêm
`ErrorCode.NOT_IMPLEMENTED`, rồi **xác nhận lần `go generate` thứ 2 không đổi 1
byte**. `go.sum` **không bị sửa** (đã xoá 2 dòng `urfave/cli` mà `go build -mod=mod`
thêm vào, và `go build ./...` vẫn sạch sau đó).

### Web

| Lệnh | Kết quả |
|---|---|
| `pnpm vitest run` | **38 file / 365 test PASS** (trước 364: −1 test `DIRECTIONS` chết, +2 test mới) |
| `pnpm typecheck` | sạch |
| `pnpm build` | OK (`✓ built in 644ms`) |
| `grep -rn react src package.json` | **0 hit** |
| `find src -name '*.tsx'` | **0 file** |

### Mutation-check — 8/8 FAIL thật

| # | Việc | Mutation | Test bắt | FAIL vì |
|---|---|---|---|---|
| 1 | 1 | `StatusNotImplemented` → `StatusConflict` | `Test_Sync_never_reports_conflict_for_missing_peer` | `Should be equal to: 501` |
| 2 | 1 | bỏ 501 khỏi `isBusinessStatus` | `Test_sync_mutation_returns_business_error_…` + `…_maps_to_not_implemented` | message bị che thành `"lỗi hệ thống"`, code `INTERNAL` |
| 3 | 1 | bỏ nhánh 501 trong `codeForStatus` | 3 test (`…_business_error`, `…_not_implemented`, `Test_graphql_error_code_covers_business_status`) | `NOT_IMPLEMENTED` vs `INTERNAL`; "code chết" |
| 4 | 2 | thêm `Map`/`Slice`/`Chan` vào switch | `Test_Is_ignores_nil_map_and_slice` + `…_hidden_behind_interface` | `Should be false` |
| 5 | 3 | `bookmarkStatusText` trả `"to_read"` kèm lỗi (mẫu F1) | `Test_bookmark_status_text_rejects_value_it_cannot_map` | `Should be empty` |
| 6 | 3 | `TO_READ` → `""` | `Test_bookmark_status_text_separates_absent_from_default` + roundtrip | `to_read` ≠ `""` |
| 7 | 4 | thêm `'DIAGONAL'` vào `MapDirection` | `pnpm typecheck` | `Property 'DIAGONAL' is missing` |
| 8 | 5 | đảo `MEADOW`↔`DESERT` trong `TERRAIN_SKIN` | `terrain.test.ts` | `MEADOW…` ≠ `DESERT…` |

Script: `/tmp/opencode/m7b-mutation.sh` (đảo → test → **revert từ `.m7bbak`**;
đã xác nhận **không còn file `.m7bbak` nào** sau khi chạy).

**Mutation 2 đáng ghi:** nó KHÔNG đổi status, chỉ đổi 1 danh sách ở transport —
và test tầng graphql đỏ. Đó là bằng chứng việc thêm hằng enum **không đủ**;
phải sửa cả `isBusinessStatus` lẫn `codeForStatus` thì message mới tới được
user. Cùng lớp bài học với mutation 1 của M7c.

---

## 7. File đã sửa

### Go — sửa (8)
- `internal/application/sync/service.go` — hằng `StatusNotImplemented`, `Sync` trả 501.
- `internal/transport/graphql/errors.go` — `isBusinessStatus` + `codeForStatus` có 501.
- `internal/transport/graphql/convert.go` — `bookmarkStatusText` 3 trạng thái + `bookmarkStatusNames`.
- `internal/transport/graphql/root.resolvers.go` — 3 call site xử lý lỗi.
- `internal/transport/graphql/helpers.go` — **xoá** `derefBookmarkStatus` (§3.3).
- `internal/typednil/typednil.go` — thu hẹp + doc giải thích ranh giới.
- `graph/schema/common.graphqls` — `ErrorCode.NOT_IMPLEMENTED`.
- `internal/transport/graphql/{generated/generated.go, model/models_gen.go}` — sinh máy.

### Go — sửa test (4)
- `internal/application/sync/typednil_loader_test.go` (+2 hàm mới, +assert)
- `internal/transport/graphql/enum_roundtrip_test.go` (+3 hàm mới, cập nhật 1)
- `internal/transport/graphql/typednil_sync_test.go` (+3 assert)
- `internal/typednil/typednil_test.go` (thay 1 hàm bằng 2)

### Web — sửa (3)
- `src/roadmap/map/terrain.ts` — xoá `DIRECTIONS`, `Direction = MapDirection`,
  bảng `HORIZONTAL` + `isHorizontal` đọc từ bảng, comment ở header.
- `src/roadmap/map/terrain.test.ts` — bỏ import `DIRECTIONS`, thay test hướng,
  thêm test thứ tự `TERRAINS`.
- `src/graphql/errors.ts` — comment liệt kê `code` cập nhật `NOT_IMPLEMENTED`.

**KHÔNG** đụng `web/src/roadmap/api.ts`, `MapCanvas.vue`, `StageForm.vue`
(consumer không đổi) · `DEPLOY.md` · `THIRD-PARTY-LICENSES` · CI ·
`api/*.go` app v1 · `migrations/**` · `go.mod`/`go.sum`.

---

## 8. Nợ còn lại (cho M7c)

| # | Việc | Vì sao chưa làm ở M7b |
|---|---|---|
| 1 | Áp `typednil.Is` cho 10 chỗ interface còn lại (`Options.DB`, `Options.TTS/STT`, `content.{decks,cards}`) | Không reachable — `stack-v2-typednil.md` §3.1 đã kiểm từng chỗ. **Sau M7b đã đảm bảo thu hẹp không làm hỏng chúng** vì chúng đều là interface port con trỏ. |
| 2 | `resourceKindText` | **Tình cờ an toàn**: `ValidateKind` từ chối ⇒ 400 (`stack-v2.md` M7a đã ghi). Không sửa vì không có đường mất dữ liệu. |
| 3 | CHECK cho `roadmap_resources.kind` | Cần migration mới (bump goose) + xử lý `DEFAULT ''` — ngoài 5 việc của M7b. |
| 4 | `DEPLOY.md` + `THIRD-PARTY-LICENSES` + CI | Ngoài phạm vi (Orchestrator đã chốt). |
| 5 | `pg_dump`/`pg_restore` | User đã chốt bỏ; 2 endpoint vẫn 501. |
| 6 | Dọn 2 file scratch `zz_repro_attempt2*.go` | Ngoài phạm vi. |
| 7 | **`ErrorCode.NOT_IMPLEMENTED` chưa được `web` dùng để đổi UI** | Client hiện chỉ hiện `message`; `NOT_IMPLEMENTED` mới có string trong `AppError.code`. Dùng nó để hiện "tính năng đang xây" thay vì "lỗi" là **việc của designer/UI lane** — không tự ý sửa layout. |

---

## 9. Bài học rút ra (cho lane sau)

1. **Mã lỗi là một lời hứa, và lời hứa nằm ở TẦNG TRANSPORT chứ không ở tầng
   dưới.** Sửa `409`→`501` ở `application/sync` mà không sửa `isBusinessStatus` +
   `codeForStatus` thì user **vẫn không đọc được message** — và test chỉ bắt
   được nếu có mutation như mutation 2 ở §6. Khi thêm 1 status mới, luôn đi
   tiếp xuống `codeForStatus` **và** enum `ErrorCode`, không dừng ở
   `newError`.
2. **Test cũ có thể đang khoá điều SAI.** `Test_Is_separates_nil_container_…`
   khẳng định `Is(nilMap) == true` bằng lý do "ghi vào nil map thì panic" — lý
   do đúng, **kết luận sai** (đọc vẫn hợp lệ). Không có test nào đỏ vì điều
   đó. Đừng coi "test xanh từ trước" là "hành vi đúng".
3. **`vitest` không typecheck, `vue-tsc` không chạy test.** Một sự thật có thể
   được khoá ở **một trong hai** chỗ, không phải cả hai. Sau khi bỏ mảng
   runtime `DIRECTIONS`, phần "đúng 2 hướng" chỉ còn ở tầng type — ghi rõ
   điều đó thay vì để lại 1 assertion giả.
4. **Truyền tham số bắt buộc ở GraphQL không có nghĩa là không thể có giá trị
   lạ trong `switch`.** `SetBookmarkStatus(status: BookmarkStatus!)` — enum đóng,
   nhưng `bookmarkStatusText` vẫn phải từ chối giá trị lạ ở **tầng transport**,
   vì ranh giới tin cậy đầu tiên sau CHECK constraint là `convert.go`, không
   phải gqlgen. Đây là cùng lý do `terrainText`/`directionText` phải trả lỗi.
5. **Khi tách "3 trạng thái" hãy soi TẪNG CALL SITE của từng trạng thái.** Ở đây
   `nil` và `TO_READ` cùng ra `""` là **đúng** cho `createBookmark` (application
   gán DEFAULT) nhưng **sai** cho `bookmarks(status:)` (`""` = không lọc). Cùng 1
   hàm, 2 nơi, 2 nghĩa của `""` — đã chọn an toàn (ghi tường minh) và test
   khoá.
