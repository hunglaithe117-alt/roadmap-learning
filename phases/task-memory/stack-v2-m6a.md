# STACK-V2 — M6a (backend: bookmark CRUD + `TopError.cardId`)

> Hoàn tất 2026-09-28. M6a là phần **server** của M6; M6b (designer) làm UI.
> 2 nợ này ghi ở `stack-v2-m5-remediation.md` §8.4 (TopError) và
> `stack-v2-m2.md` §11 (bookmark CRUD PENDING).
>
> M5: `stack-v2-m5.md` + `m5-remediation.md`. File này **bổ sung** cho chúng.

## 0. Tóm tắt 1 dòng

Cả 2 nợ xong, **không phải đổi cấu trúc lưu** (không migration mới): bookmark
có đủ 5 use case CRUD + lọc `status`/`tag` + 5 field GraphQL; `TopError` trả
thêm `cardId` + `front` lấy từ `notes.card_id` (cột **đã có sẵn** từ
migration `00001`). **777 test PASS, 0 FAIL, 0 SKIP, 3 lần liên tiếp**;
7/7 mutation-check FAIL thật.

---

## 1. NỢ 1 — `roadmap_bookmarks` CRUD

### 1.1 Phân tầng

| Tầng | File | Nội dung |
|---|---|---|
| domain | `internal/domain/roadmap/bookmark.go` (**mới**) | `BookmarkStatus` (4 hằng) · `NormalizeTags` / `EncodeTags` / `DecodeTags` / `TagsContain` · `NormalizeTagFilter` · hằng `MaxBookmarkTags=10`, `MaxBookmarkTagRunes=40` |
| application | `internal/application/roadmap/{ports,service,validate}.go` | `Bookmark` struct · `BookmarkFilter` · 6 method `Repository` · `BookmarkInput`/`BookmarkPatch` · 5 use case |
| infrastructure | `internal/infrastructure/roadmap/{model,repository}.go` | `bookmarkRow` (đã có) + 5 method GORM |
| transport | `graph/schema/roadmap.graphqls` + `root.graphqls` + `convert.go` + `root.resolvers.go` | type `Bookmark`, enum `BookmarkStatus`, 3 input, 4 payload, 2 query, 4 mutation |

### 1.2 Quyết định thiết kế đáng ghi

**`BookmarkStatus` CỐ Ý khác `Status` của node roadmap.** Tập hằng là
`to_read | reading | done | archived` — **không phải**
`not_started | in_progress | done | skipped`. Lý do: bookmark không nằm trên
đường đi nên không có "chưa mở"/"bỏ qua"; đọc xong thì `done`, không còn giá
trị thì `archived`. CHECK ở DB (migration `00003`) đã đóng băng 4 hằng này từ
M1, và đổi nó là phá hợp đồng DB.

Hệ quả bắt buộc đã được test: truyền `not_started` / `in_progress` cho
bookmark là **400**, không phải im lặng rơi về `to_read`.

**Lọc tag: so PHẦN TỬ CSV, không so chuỗi con.** SQL:
`position(',' || ? || ',' in ',' || tags || ',') > 0`. Ba lý do cùng lúc:

1. `tags LIKE '%a%'` khớp tag `ab` — lọc "a" ra kết quả của "abc", sai mà UI
   không có cách nào phát hiện. Test viền `a` vs `ab` chạy ở **cả 2 nơi**:
   `domain.TagsContain` (thuần) và SQL thật trên Postgres.
2. `position` không có khái niệm wildcard ⇒ `%` / `_` trong tên tag là ký tự
   thường (cùng lý do `dict_search` phải khử `%`/`_`).
3. `LIKE` còn phải escape `%`/`_` ở mọi nơi; `position` thì không.

**Chuẩn hoá tag ở 1 chỗ, dùng lại 3 nơi.** `NormalizeTags` (ghi),
`NormalizeTagFilter` (lọc) dùng cùng phép trim + hạ chữ thường + bỏ trùng ⇒
ghi `"HSK3"` mà lọc `"hsk3"` vẫn ra. Dữ liệu peer viết tay (không qua
validate) vẫn được `DecodeTags` miễn nhiễm lỗi.

**CSV không khoảng trắng quanh dấu phẩy** (`EncodeTags` = `strings.Join`).
Rỗng → `""` chứ không phải `","` (khớp DEFAULT của cột `tags` NOT NULL).

**`UpdateBookmark` KHÔNG có `status`.** Tách `SetBookmarkStatus` riêng vì UI
đổi trạng thái bằng 1 nút bấm và không muốn mất trạng thái khi patch title sai.
Khác `stage`/`topic`, bookmark **không có `completed_at`** nên 2 use case này
dùng chung 1 đường ghi repository.

**Sắp `bookmarks` theo `id` tăng dần, không theo `updated_at`.** Kho link là
danh sách user tự thêm; thứ tự thời gian thêm là trực giác, còn `updated_at`
bị trigger chạm mỗi lần sửa nên nhảy vị trí dưới tay user.

**`bookmarks` KHÔNG bọc payload `ok`+`error`.** Lọc sai không thể xảy ra từ
GraphQL (`status` là enum do gqlgen chặn, `tag` chỉ có 1 quy tắc độ dài) ⇒ lỗi
còn lại là lỗi hệ thống, trả null + error GraphQL là đúng. 4 **mutation** thì
vẫn bọc payload như toàn bộ mutation khác.

**`bookmark(id:)` trả `null` khi không có** (không phải lỗi) — cùng lý do với
`deck(id:)`: client tự dán id là tình huống bình thường.

### 1.3 Không cần migration

Bảng + trigger + index đã có từ `00003`. `snapshotColumns[roadmap_bookmarks]`
đã đủ 9 cột, `bookmarkMergeRow` đã đủ 11 field, `Merged.RoadmapBookmarks` đã
có. **`sync` không sửa 1 dòng.**

---

## 2. NỢ 2 — `TopError.cardId` (KHÔNG đổi cấu trúc `notes`)

### 2.1 Trả lời câu hỏi: `wrong[]` lưu gì?

`wrong[]` trong note `ERR|` lưu **TỪ** (chuỗi), không lưu card id:

```json
ERR|{"expected":"你好","transcript":"ni hao","wrong":["hau"]}
```

`pdp.Compare` sinh ra danh sách từ sai; `practice.AppendError` marshal thẳng.

### 2.2 Nhưng card id ĐÃ CÓ SẴN — ở cấp NOTE

Cột **`notes.card_id BIGINT NULL REFERENCES cards(id)`** đã tồn tại từ
migration `00001`, và `practice.AppendError(cardID *int64, …)` **đã ghi nó** khi
client gửi `cardId` (`web/src/routes/Recorder.vue` gửi `cardId` khi người dùng
đang luyện 1 thẻ cụ thể).

⇒ **Không cần migration `00005_*`, không cần `card_ref`, không cần đổi format
JSON.** Chỉ sửa `insight` đọc cột đó thay vì bỏ qua.

Một note có nhiều từ sai nhưng **tất cả thuộc cùng 1 thẻ** — nên thẻ đi kèm ở
cấp note là đúng, không phải thiếu dữ liệu ở cấp từ.

### 2.3 `cardId` là NULLABLE — khi nào

| Tình huống | `cardId` | `front` |
|---|---|---|
| Lỗi gắn thẻ, thẻ còn sống | id thẻ | mặt trước |
| Lỗi luyện nói **tự do** (`cardId: null` khi ghi) | `null` | `null` |
| Thẻ đã **xoá mềm** | `null` | `null` |
| Cùng 1 từ sai ở nhiều thẻ | thẻ của lần sai **mới nhất**; `null` nếu lần đó không gắn thẻ | tương ứng |

**Vì sao thẻ đã xoá mềm trả `null` chứ không trả id:** `cardId` trong GraphQL
là lời hứa *"bấm là nhảy tới thẻ này"*. Thẻ đã xoá thì không nhảy được, và
trả id sẽ khiến UI có 1 nút chết. Note **vẫn được đếm** như cũ — chỉ mất
đường nhảy.

**Vì sao chọn lần sai MỚI NHẤT:** `Repository.ListErrorNotes` đọc
`ORDER BY id DESC` (đã vốn thế) ⇒ "mới nhất" = "đầu tiên trong slice".
`domain.TopError` lấy `CardID` đầu tiên khác nil. Điều này **phải được ghi ở
interface** vì caller truyền sai thứ tự thì từ đó trỏ thẻ cũ mà không có lỗi
nào báo — đã ghi ở doc `ListErrorNotes`.

### 2.4 Một lệnh SQL duy nhất

```sql
SELECT n.card_id, c.front, n.text FROM notes n
LEFT JOIN cards c ON c.id = n.card_id AND c.deleted = 0
WHERE n.text LIKE 'ERR|%' AND n.text NOT LIKE 'THIEU|%'
ORDER BY n.id DESC LIMIT ?
```

`c.deleted = 0` đặt trong **ON chứ không phải WHERE** — thẻ đã xoá mềm làm
`front` rỗng nhưng note vẫn phải được tính vào số lần sai. Đặt sai chỗ này là
mất số liệu dashboard.

### 2.5 Ảnh hưởng `sync`

**Không có.** Không thêm cột ⇒ `snapshotColumns[notes]`, `noteMergeRow` không
đổi. `notes` đã nằm trong snapshot với `card_id` khai ở
`snapshotTables[].fks` (`fkCard`) từ M3 — tức `card_id` của note **đã** được
đồng bộ 2 máy từ trước.

---

## 3. Chốt chặn DRY của `sync` — thêm 1 chiều

`internal/infrastructure/sync/snapshot_columns_test.go` đã có 1 chiều:
struct merge-row → `snapshotColumns` (bắt B1: thêm cột vào struct mà quên khai
snapshot ⇒ merge xoá dữ liệu cột đó).

**Chiều ngược vừa thêm** (`Test_snapshot_columns_all_appear_in_merge_row`):
cột khai trong `snapshotColumns` mà thiếu ở struct merge-row. Hệ quả của lỗi
đó **không phải "thiếu dữ liệu"** mà là **conflict giả vô hạn**: loader đọc cột
đó vào `Row.Values`, còn hàm `bookmarkValues`/`pathValues`… dựng map compare
KHÔNG có nó ⇒ `differs()` báo khác nhau mỗi lần sync dù 2 máy giống nhau.
Chiều cũ **không bắt được** vì struct hẹp hơn danh sách cột vẫn "hợp lệ".

Cả 2 chiều đã mutation-check: thêm `"favourite"` vào `snapshotColumns` ⇒ chiều
mới đỏ; thêm `Favourite` vào `bookmarkMergeRow` ⇒ chiều cũ đỏ.

---

## 4. Test

| File | Số | Vì sao đáng để có |
|---|---|---|
| `internal/domain/roadmap/bookmark_test.go` (**mới**) | 6 | `TagsContain` viền `a` vs `ab` (bản thuần của điều kiện SQL) · chuẩn hoá tag · 4 hằng status |
| `internal/application/roadmap/bookmark_test.go` (**mới**) | 11 | validate URL sai scheme · status sai · tag sai giới hạn · `clearUrl` ghi NULL · `tags: []` xoá hết tag · soft-delete 404 lần 2 |
| `internal/infrastructure/roadmap/bookmark_test.go` (**mới**) | 10 | roundtrip Postgres + trigger chạm `updated_at` · **lọc tag viền `a`/`ab`/`abc`/`ba` trên SQL thật** · `%`/`_` không thành wildcard · `url IS NULL` thật · CHECK 4 hằng |
| `internal/transport/graphql/bookmark_test.go` (**mới**) | 5 | CRUD qua `graph/client` · filter status+tag (AND) · lỗi 400 về trong payload · `TopError.cardId` có/không · card xoá mềm ⇒ null |
| `internal/application/insight/service_test.go` (+2) | 2 | `cardId` = thẻ của lần sai mới nhất · lỗi tự do ⇒ null |
| `internal/infrastructure/insight/repository_test.go` (+2) | 2 | `notes.card_id` thật + `LEFT JOIN` · card xoá mềm ⇒ `cardId` null |
| `internal/infrastructure/sync/snapshot_columns_test.go` (+1) | 1 | chiều ngược chốt chặn DRY |

**Tổng toàn suite sau M6a: 730 test top-level / 777 tính cả subtest, 0 FAIL,
0 SKIP, chạy 3 lần liên tiếp giống hệt nhau.** M6a thêm **37 hàm test** (6 + 11
+ 10 + 5 bookmark, 2 + 2 insight, 1 sync) — không đo lại baseline trước M6a vì
thư mục không phải git repo, và con số "thêm bao nhiêu" đếm trực tiếp từ các
file trong bảng trên thì chính xác hơn.

### 4.1 Mutation-check 7/7 FAIL thật

Sửa ngược từng quyết định, chạy lại, **revert**:

| Mutation | Kết quả |
|---|---|
| `position(...)` → `tags LIKE '%tag%'` | **2 đỏ** — `Test_list_bookmarks_tag_filter_matches_whole_element` (ra `[1,2,3,4]` thay vì `[1,4]`) + `..._escapes_like_wildcards` (lọc `%` ra toàn bảng) |
| Bỏ nhánh gán `cards[w]` trong `domain.TopError` | **1 đỏ** — `Test_top_errors_carries_card_of_the_newest_error` |
| Bỏ `if front == "" { cardID = nil }` ở repository insight | **1 đỏ** — `Test_top_errors_card_id_is_null_when_card_was_soft_deleted` |
| Bỏ `patchIn.Tags = &v` trong resolver `updateBookmark` | **1 đỏ** — `tags: []` không xoá được tag (`[hsk3 ngữ pháp]`) |
| `if touchesURL` → `if touchesURL && u != nil` | **2 đỏ** — test application + test Postgres (ghi `''` thay vì NULL) |
| Thêm `"favourite"` vào `snapshotColumns[bookmarks]` | **1 đỏ** — test DRY chiều mới |
| Thêm `Favourite` vào `bookmarkMergeRow` | **1 đỏ** — test DRY chiều cũ |

---

## 5. Bàn giao cho M6b (designer) — CONTRACT GRAPHQL

### 5.1 Type `Bookmark` (đủ 11 field)

```graphql
type Bookmark {
  id: ID!            # String, ví dụ "42"
  guid: String!      # khoá định danh cho sync — dùng để nhận ra row đã có trên máy peer
  title: String!
  url: String        # NULL = bookmark không có link
  note: String!      # rỗng = không có ghi chú (KHÔNG null)
  tags: String!      # CSV thô như lưu DB, vd "hsk3,ngữ pháp"
  tagList: [String!]!  # đã tách mảng; RỖNG [] khi không có tag (không phải null)
  status: BookmarkStatus!
  createdAt: String! # RFC3339
  updatedAt: String! # RFC3339
}
```

Điểm dễ sai: `tags` (CSV) và `tagList` (mảng) cùng tồn tại. **UI dùng
`tagList`**; `tags` chỉ để hiển thị/so khớp thô. `tagList` luôn hạ chữ thường +
trim (server đã chuẩn hoá lúc ghi).

### 5.2 Enum `BookmarkStatus` (4 giá trị)

```graphql
enum BookmarkStatus { TO_READ  READING  DONE  ARCHIVED }
```

| Value | Nghĩa | Nhãn tiếng Việt gợi ý |
|---|---|---|
| `TO_READ` | chưa đọc (DEFAULT) | Chưa đọc |
| `READING` | đang đọc | Đang đọc |
| `DONE` | đã đọc xong | Xong |
| `ARCHIVED` | cất đi | Cất đi |

⚠️ **KHÁC** `Status` của node roadmap (`NOT_STARTED`/`IN_PROGRESS`/`DONE`/
`SKIPPED`) — đừng dùng nhầm. Gửi giá trị của `Status` cho bookmark là 400.

### 5.3 Query

```graphql
bookmarks(status: BookmarkStatus, tag: String): [Bookmark!]!
bookmark(id: ID!): Bookmark   # null = không tồn tại / đã xoá
```

| Biến | Kiểu | Bắt buộc | Mặc định | Ghi chú |
|---|---|---|---|---|
| `status` | `BookmarkStatus` | không | `null` = không lọc | AND với `tag` |
| `tag` | `String` | không | `null` = không lọc | khớp theo **phần tử** CSV |
| `id` | `ID!` | có | — | |

`bookmarks` **không có payload `ok`/`error`** (khác `stats`) — lỗi lọc không
xảy ra từ GraphQL.

⚠️ **Lọc tag là so phần tử, KHÔNG phải `LIKE`.** Lọc `tag: "a"` **không** ra
bookmark mang tag `ab`/`abc`. Nếu UI cần "gõ là lọc", hãy so khớp client-side
trên `tagList` thay vì gửi `tag` mỗi ký tự.

### 5.4 Mutation — đủ `variables` + kiểu

```graphql
createBookmark(input: BookmarkInput!): CreateBookmarkPayload!
updateBookmark(id: ID!, patch: BookmarkPatch!): UpdateBookmarkPayload!
deleteBookmark(id: ID!): DeleteBookmarkPayload!
setBookmarkStatus(id: ID!, status: BookmarkStatus!): SetBookmarkStatusPayload!
```

**`BookmarkInput`**

| Field | Kiểu | Bắt buộc | Mặc định |
|---|---|---|---|
| `title` | `String!` | có | — (rỗng/sai → 400) |
| `url` | `String` | không | `null` |
| `note` | `String` | không | `""` |
| `tags` | `[String!]` | không | `[]` |
| `status` | `BookmarkStatus` | không | `TO_READ` |

**`BookmarkPatch`**

| Field | Kiểu | Bắt buộc | Ghi chú |
|---|---|---|---|
| `title` | `String` | không | |
| `url` | `String` | không | ⚠️ **không phải** cách xoá link |
| `note` | `String` | không | |
| `tags` | `[String!]` | không | ⚠️ `[]` = **xoá hết tag**; không gửi = giữ nguyên |
| `clearUrl` | `Boolean` | không | `true` = xoá link (ghi SQL NULL) |

`updateBookmark` **không có `status`** — dùng `setBookmarkStatus`. Gửi `{}`
(hoặc chỉ `url: ""`) là 400 `"không có gì để cập nhật"`.

**4 payload** đều có cùng hình dạng:

```graphql
type CreateBookmarkPayload  { ok: Boolean!, bookmark: Bookmark, error: UserError }
type UpdateBookmarkPayload  { ok: Boolean!, bookmark: Bookmark, error: UserError }
type DeleteBookmarkPayload  { ok: Boolean!, error: UserError }
type SetBookmarkStatusPayload { ok: Boolean!, bookmark: Bookmark, error: UserError }
```

`error { message code }`, `code ∈ {BAD_REQUEST, NOT_FOUND, CONFLICT, INTERNAL}`,
`message` tiếng Việt **nguyên văn** từ application (VD: `"url chỉ nhận link
http hoặc https"`, `"không tìm thấy bookmark"`).

### 5.5 Operation mẫu

```graphql
fragment BookmarkFields on Bookmark {
  id
  guid
  title
  url
  note
  tags
  tagList
  status
  createdAt
  updatedAt
}

query Bookmarks($status: BookmarkStatus, $tag: String) {
  bookmarks(status: $status, tag: $tag) {
    ...BookmarkFields
  }
}

mutation CreateBookmark($input: BookmarkInput!) {
  createBookmark(input: $input) {
    ok
    bookmark { ...BookmarkFields }
    error { message code }
  }
}

mutation UpdateBookmark($id: ID!, $patch: BookmarkPatch!) {
  updateBookmark(id: $id, patch: $patch) {
    ok
    bookmark { ...BookmarkFields }
    error { message code }
  }
}

mutation DeleteBookmark($id: ID!) {
  deleteBookmark(id: $id) { ok error { message code } }
}

mutation SetBookmarkStatus($id: ID!, $status: BookmarkStatus!) {
  setBookmarkStatus(id: $id, status: $status) {
    ok
    bookmark { id status }
    error { message code }
  }
}
```

Sau mỗi mutation phải `await afterMutation()` (đường invalidate cache duy
nhất của app — `src/lib/mutationInvalidation.test.ts` chốt 11 hàm mutation).

### 5.6 `/roadmap` không đổi

`path(slug:)` / `paths` / `stage` / `topic` / `resource` / `milestone` /
`pathProgress` giữ nguyên. Bookmark **không** nằm trong cây roadmap. Nếu `/roadmap`
cần gợi ý "link liên quan" thì phải query `bookmarks(tag:)` riêng.

### 5.7 `TopError` — trả lời câu hỏi của M6b

```graphql
type TopError {
  word: String!
  count: Int!
  cardId: ID      # NULLABLE — xem bảng dưới
  front: String   # NULLABLE, đi cùng cardId
}
```

**`cardId` KHÔNG luôn có** (nullable, 3 trường hợp):

| Tình huống | `cardId` | `front` | UI nên làm |
|---|---|---|---|
| Lỗi gắn thẻ, thẻ còn sống | có | có | dòng bấm được → `/review?card=…` |
| Lỗi luyện nói **tự do** | `null` | `null` | dòng **chỉ hiện chữ**, không bấm được |
| Thẻ đã **xoá mềm** | `null` | `null` | như trên |

⇒ Nút "nhảy review" **phải** kiểm `cardId !== null`; không có id thì render
dòng tĩnh. `front` cho phép hiện mặt trước ngay cạnh từ sai mà không cần gọi
thêm query. `cardId` là thẻ của lần sai **mới nhất** gắn thẻ.

Còn `practice.topErrors` (type `TopErrorCount`) là **đường khác** và **chưa**
có `cardId` — dùng `insightTopErrors` (payload `ok`/`error`) khi cần nhảy
review. M5 đang đọc `topErrors`; chuyển sang `insightTopErrors` là 1 dòng.

---

## 6. File đã sửa / tạo

**Mới (7):**
- `api/internal/domain/roadmap/bookmark.go`
- `api/internal/domain/roadmap/bookmark_test.go`
- `api/internal/application/roadmap/bookmark_test.go`
- `api/internal/infrastructure/roadmap/bookmark_test.go`
- `api/internal/transport/graphql/bookmark_test.go`
- `api/graph/schema/*` — sửa (không tạo file mới)
- `phases/task-memory/stack-v2-m6a.md` (file này)

**Sửa (12):**
`api/internal/application/roadmap/{ports,service,validate,service_test}.go` ·
`api/internal/infrastructure/roadmap/{model,repository}.go` ·
`api/internal/application/insight/{ports,service,service_test}.go` ·
`api/internal/domain/insight/entity.go` ·
`api/internal/infrastructure/insight/{repository,repository_test}.go` ·
`api/internal/infrastructure/sync/snapshot_columns_test.go` ·
`api/internal/transport/graphql/{convert,helpers,root.resolvers}.go` ·
`api/graph/schema/{roadmap,insight,root}.graphqlls` ·
`api/internal/transport/graphql/{generated/generated.go,model/models_gen.go}`
(máy sinh từ `go generate`).

**KHÔNG sửa:** `web/**` (M6b) · `api/*.go` cũ (app v1) · `api/migrations/**` ·
`api/internal/infrastructure/sync/{snapshot.go,repository_merge_roadmap.go}` ·
`api/internal/platform/**` (con số migration vẫn 5, test M1 không sửa).

---

## 7. Verify

| Lệnh | Kết quả |
|---|---|
| `docker compose up -d postgres` | healthy |
| `LANGAPP_TEST_POSTGRES_DSN=… go test -count=1 ./...` | **777 PASS / 0 FAIL / 0 SKIP** |
| Chạy lại **3 lần liên tiếp** | **3/3 giống hệt** (730 top-level + 47 subtest) |
| `go build ./...` | sạch |
| `go vet ./...` | 0 cảnh báo |
| `gofmt -l internal migrations roadmap_seed cmd services` | sạch (2 file cũ `chinese_test.go`/`reader.go` ở gốc `api/` được phép giữ) |
| `grep -rl 'gorm.io\|gin-gonic\|grpc\|database/sql\|net/http' internal/domain` | **0 file** (exit 1) |
| `grep -rl 'gorm.io' internal/application` | **0 file** (exit 1) |
| `grep -rl 'internal/infrastructure' internal/domain internal/application` | **0 file** (exit 1) |
| `go list -deps -test ./internal/... ./cmd/... ./services/...` | không package nào còn `modernc.org/sqlite` (còn ở package gốc `langapp` — app v1, M7 dọn) |
| `go test .` (app v1) | **ok langapp** 0.85s |
| `go generate ./...` chạy 2 lần | md5 `generated.go` + `models_gen.go` **không đổi** |

---

## 8. SKIPPED + lý do

| Việc | Trạng thái | Lý do |
|---|---|---|
| `notes.card_ref` / migration `00005_*` | **KHÔNG LÀM** | `notes.card_id` đã có sẵn từ `00001` và `AppendError` đã ghi ⇒ đủ dữ liệu, thêm cột là thừa + phải vào snapshot. Đây là câu trả lời cho câu hỏi của brief. |
| `limit` cho `Query.bookmarks` | **KHÔNG LÀM** | Kho link là dữ liệu cá nhân 1 người (vài chục dòng). Thêm `limit` tạo 1 câu hỏi client phải tự trả lời mà không đổi hành vi. Nếu M6 cần phân trang thì thêm sau, có thể bổ sung `BookmarkListPayload`. |
| Bookmark lọc theo `q` (tìm trong title/note) | **KHÔNG LÀM** | Ngoài 2 nợ đã ghi. UI có thể lọc client-side trên list đã trả về. |
| `practice.topErrors` cũng có `cardId` | **KHÔNG LÀM** | Ngoài phạm vi ghi (`application/insight` + `infrastructure/insight`). Đã ghi ở §5.7: dùng `insightTopErrors`. Nếu M6 cần đồng nhất 2 đường thì đưa vào backlog. |
| Test filter tag bằng index | **KHÔNG LÀM** | Không có index nào dùng được cho CSV `position(...)`; thêm index trên `tags` là index chết (bảng vài chục dòng). Đã ghi cùng lý do ở migration `00004`. |
