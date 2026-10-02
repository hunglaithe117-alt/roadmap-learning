# STACK-V2 — M5 remediation (4 finding + 5 việc nhỏ)

> Hoàn tất 2026-09-28. Sửa findings của cổng Oracle M5.
>
> M5: `stack-v2-m5.md`. File này **bổ sung** cho nó, không thay thế.

## 0. Tóm tắt 1 dòng

4 finding đều **tái hiện được bằng chạy thật** trước khi sửa; sau khi sửa có
**9/9 test mới** và **mutation-check 6/6 FAIL thật**; `vitest` **134 → 170 test
PASS**, `vue-tsc` sạch, `vite build` OK, `grep react` 0 hit.

---

## 1. Bảng 9 mục

| # | Mục | Sửa ở đâu | Test |
|---|---|---|---|
| **F1** | `createDeck` thiếu `afterMutation()` | `src/srs/api.ts` | `src/lib/mutationInvalidation.test.ts` (13) |
| **F2** | `provideClient()` top-level là no-op | `src/main.ts` | `src/main.test.ts` (3) |
| **F3** | Comment nói ngược về `cacheExchange` | `src/graphql/operations.ts` + comment ở `client.ts` / `afterMutation.ts` | `src/graphql/__typename.test.ts` (5) |
| **F4** | Banner nhắc học 20:00 chết vĩnh viễn | `src/routes/Dashboard.vue` | `src/routes/Dashboard.test.ts` (4) |
| **5.1** | `<a download>` tới `/api/backup` tái tạo bug v1 | `src/routes/CaiDat.vue` (xoá thẻ) | `src/routes/CaiDat.test.ts` (+1) |
| **5.2** | `roadmap/order.ts` chưa có test; 2 wrapper trùng lặp | `src/roadmap/order.ts` (xoá `sortStages`/`sortTopics`) | `src/roadmap/order.test.ts` (9, mới) |
| **5.7** | `restKeys.restore()` khai mà không dùng | `src/rest/queryClient.ts` (xoá) | grep + `typecheck` |
| **5.8** | Comment Reader nói neo theo resize — sai | `src/routes/Reader.vue` (`popupStyle` → `computed`) | `vue-tsc` |
| **5.9** | `App.test.ts` assert `ROUTES.length === 14` chặn M6 | `src/App.test.ts` (assert từng path cũ) | 2 test mới |

---

## 2. 4 finding — đã chạy thật để tái hiện

Oracle đưa sẵn kết quả; tôi chạy lại **trước khi sửa** để không tin bừa. 2 probe
đầu của tôi cho kết quả **ngược với Oracle** — nguyên nhân nằm ở cách đo, ghi ở
§2.3 và §2.4.

### 2.1 F1 — `createDeck` không invalidate ⇒ deck vừa tạo không hiện

**Tái hiện** (`@urql/core` 6.0.3 thật):

```
createDeck → fetchDecks:
  rereadHitNetwork = false      ← không ra mạng
  data[0].name      = OLD       ← vẫn là danh sách cũ
  mutationHitNetwork = true     ← mutation có chạy
```

Không có lỗi nào, không cảnh báo nào. `Hoc.vue` xoá input rồi danh sách không đổi.

**Probe đầu của tôi sai ở đâu**: đếm `fetchMock.mock.calls.length > before` với
`before` chụp **trước cả mutation** ⇒ luôn `true` vì bản thân mutation cũng là 1
request. Đúng phải chụp mốc **sau** mutation:

```ts
await run();
const afterMut = fetchMock.mock.calls.length;   // ← chụp ở đây
await reread();
expect(fetchMock.mock.calls.length).toBeGreaterThan(afterMut);
```

Sửa: thêm `await afterMutation()` trong `createDeck`.

**CHẶN TÁI PHÁT (điểm cốt lõi của brief)** — `afterMutation.test.ts` chỉ test
*helper*, nên F1 lọt. Đã thêm `src/lib/mutationInvalidation.test.ts`:

- **11 test `it.each`** — MỌI hàm mutation của app: `srs/{createDeck,postReview,createCard}`,
  `chinese/{answerDrill,postZhBingoScore,postZhImport}`, `english/{postEnSeed,postThieu}`,
  `player/{postProgress,postError,runSync}`. Mỗi test: warm cache → chạy mutation →
  đọc lại query liên quan → **đếm request thật** (không mock rồi assert lời gọi).
- **2 test danh sách** — chặn M6 thêm mutation mà quên nghĩ tới `afterMutation`:
  - `mọi hàm mutation export đều có trong danh sách kiểm` (chiều khai báo thủ công);
  - `không có hàm mutation nào bị bỏ sót khỏi module` (chiều ngược: quét export
    của 4 module client, bắt theo động từ hành động).

**Đã kiểm chứng lưới có bắt thật**: thêm `export async function deleteDeck(...)` vào
`srs/api.ts` mà không khai báo ⇒ test đỏ với
`expected [ 'srs/api/deleteDeck' ] to deeply equal []`.

**Sửa 2 lỗi trong lúc viết test** (đáng ghi vì đều là bẫy):
- Regex ban đầu chỉ bắt `^(post|create|run|answer|record|append|seed)` ⇒ **KHÔNG
  bắt `deleteDeck`** — và M6 chắc chắn sẽ thêm `delete*`/`update*`. Đã nới thêm
  `delete|update|set|import|upsert|add`.
- Regex bắt cả `postZhDrillGrade` + `postEnChunks` ⇒ **2 false positive**: tên
  `post*` là di sản REST của app v1, ở app-v2 chúng là **query** (`gradeTone`,
  `chunk`) nên không được invalidate. Đã liệt kê tường minh kèm lý do thay vì
  nới regex. Cùng cơ chế bắt được `player/api/setSyncUiEnabled` (chỉ bật/tắt
  localStorage, không chạm server).

### 2.2 F2 — `provideClient()` ở top-level là no-op

**Tái hiện** (`@urql/vue` 2.1.1 thật):

```
[Vue warn]: provide() can only be used inside setup().
→ component con gọi useClient() ⇒ throw "No urql Client was provided"
```

Đọc source `node_modules/@urql/vue/dist/urql-vue.mjs` để chọn đúng API, không đoán:
- `provideClient(opts)` gọi `provide()` của Vue ⇒ **chỉ chạy được trong `setup()`**;
- `install(app, opts)` (export default) gọi `app.provide('$urql', …)` ⇒ chạy được
  ở cấp app, không cần instance.

Sửa: `app.use(urqlVue, urqlClient())`.

**Ghi chú API**: `@urql/vue` 2.1.1 **không export `useClient`** (chỉ có
`useClientHandle`; `useQuery`/`useMutation` gọi `useClient()` bên trong). Probe
dùng `useClientHandle()` — đường công khai tương đương, và cho phép so **identity**
của client, thứ mà `useQuery` không để lộ.

**Để test bắt được `main.ts` thật, không chỉ bắt "cách làm đúng"**: tách
`installPlugins(app)` ra khỏi top-level của `main.ts` và `main.test.ts` gọi đúng
hàm đó. Nếu không tách, `main.ts` có thể quay lại `provideClient` ở top-level mà
mọi test vẫn xanh — đó chính là lý do F2 lọt lần đầu.
`main.ts` cũng chặn `app.mount()` khi `import.meta.env.VITEST` để import được mà
không cần `#app` trong DOM.

3 test: (1) component con dùng được `useClient` + `useQuery`; (2) client được
provide **đúng instance** `urqlClient()` (client thứ 2 = 2 document cache = lệch
cache); (3) `provideClient` top-level **thật sự** no-op (canh cả warning của Vue
lẫn hậu quả throw).

### 2.3 F3 — `cacheExchange` KHÔNG tự invalidate được

Comment trong `afterMutation.ts` và `client.ts` nói "cacheExchange đã loại bỏ
đúng entity mà mutation trả về" — **sai sự thật**. Đo ma trận 2×2 trên
`@urql/core` 6.0.3 thật (đọc `__typename` từ CẢ query string của GET và body của
POST):

| đọc có `__typename` | ghi có `__typename` | rereadHitNetwork |
|---|---|---|
| ✗ | ✗ | `false` |
| ✓ | ✗ | `false` |
| ✗ | ✓ | `false` |
| ✓ | ✓ | **`true`** |

⇒ `__typename` phải có ở **CẢ HAI vế**. Đây là chi tiết Oracle không nêu, và nó
loại bỏ giả thuyết dễ tin nhầm nhất ("chỉ cần thêm vào query").

**Probe đầu của tôi sai ở đâu**: mock chỉ đọc `operationName` từ query string, mà
urql gửi mutation bằng **POST** ⇒ coi như mutation không có `__typename` ⇒ cả
2 nhánh đều ra `true`, kết luận sai. Sửa mock đọc cả body, và bám theo **document**
( có `__typename` trong câu mutation thì mới trả `__typename` trong response —
vì `cacheExchange` đọc payload, không đọc câu query).

Sửa: thêm `__typename` vào `DECK_FIELDS` + `CARD_FIELDS` (export ra để test kiểm
được) + viết lại comment ở `operations.ts`, `client.ts`, `afterMutation.ts` nói
đúng: `afterMutation()` là cơ chế invalidate **duy nhất**, `__typename` chỉ giảm
request thừa.

5 test: 3 nhánh ma trận (có-có ⇒ mông; có-không và không-có ⇒ stale) + assert
`DECK_FIELDS`/`CARD_FIELDS`/`CreateDeckMutation` thật sự xin `__typename` + hành
vi thật `createDeck` → `fetchDecks` ra mạng và trả dữ liệu mới.

**Bẫy 2 lần trong lúc viết test**:
- `doc.loc.source.body` **rỗng** với document viết 1 dòng ⇒ regex luôn `false` ⇒
  test xanh nhầm. Dùng `print()` từ `graphql` (serialize từ AST).
- Truyền chuỗi thô vào biến kiểu `TypedDocumentNode` ⇒ `print()` ném
  `Invalid AST Node` và `operationName` đọc sai. Phải dùng `gql` như template tag.

### 2.4 F4 — banner nhắc học 20:00 chết vĩnh viễn

`shouldRemind()` = `giờ >= 20 && !visitedToday()`. `onMounted` gọi `markVisited()`
**trước** ⇒ vừa đánh dấu "đã ghé" rồi hỏi lại ⇒ luôn `false`.

Đo (fake timer 21:00, localStorage sạch):

```
thứ tự HIỆN TẠI (markVisited → shouldRemind): shouldRemind = false   ← bug
thứ tự ĐÚNG (shouldRemind → markVisited):     shouldReminder = true
đối chứng 09:00:                             shouldRemind = false  (đúng)
```

Sửa: đảo 2 dòng.

`streak.test.ts` không bắt được vì nó chỉ phủ `computeStreakDays` — test mới
**mount component thật** (`Dashboard.test.ts`, 4 test): banner hiện lúc 21:00 khi
hôm nay chưa ghé; hỏi-được-rồi-mới-đánh-dấu (`last-visit-day` có trong storage
sau mount); không hiện lại lần mount thứ 2 trong ngày; không hiện trước 20:00.

---

## 3. 5 việc nhỏ

**5.1 — `<a :href="buildBackupUrl()" download>`** trong `CaiDat.vue:313` tái tạo
đúng bug v1: bấm là trình duyệt tải về file JSON báo 501 rồi báo "xong". Đã xoá
thẻ + import. Test mới `test_khong_con_the_a_download_troi_thang_bam_xuong_file_loi`
canh đúng việc "ai đó muốn nút tải nhanh thì thêm lại".

**5.2 — `roadmap/order.ts` chưa có test.** Thêm `src/roadmap/order.test.ts`
(9 test), gồm case quan trọng nhất của M6: `id` CHỮI SỐ — `"10"` phải sau `"9"`,
so chuỗi thì sai (`"10" < "9"`). Thêm: id không phải số so chuỗi, một vế số một
vế chuỗi, không mutate input, mảng rỗng, và **tổng thứ tự** (sort 2 lần với input
đảo ngược phải cho kết quả giống nhau — nếu `compareId` không tổng thì kết quả
phụ thuộc engine).
Xoá `sortStages`/`sortTopics`: 2 wrapper trùng lặp, chỉ tồn tại vì app v1 có 3
hàm gọi tên khác nhau. **Không** ai gọi 2 hàm đó.

**5.7 — `restKeys.restore()`**: `useMutation` của vue-query **không nhận
`queryKey`** (chỉ `mutationKey`) ⇒ khoá khai ra không ai dùng mà vẫn gợi cảm giác
"đã cache". Đã xoá kèm comment giải thích. Xác nhận: `grep -rn "restKeys.restore" src`
→ 0 hit trước khi xoá.

**5.8 — `Reader.vue`**: comment nói popup "neo lại khi resize" nhưng
`window.innerWidth/innerHeight` không được Vue theo dõi ⇒ **không bao giờ neo lại
khi resize**. `popupStyle` là hàm thường nên template gọi lại mỗi render, giá trị
không ai theo dõi. Đã đổi thành `computed` **và** sửa comment nói thẳng hiện tại
chỉ neo 1 lần khi mở (đúng bản v1); muốn neo theo resize phải `onMounted` +
listener — việc tương lai, không phải việc ngầm.

**5.9 — `App.test.ts` assert `ROUTES.length === 14`** ⇒ M6 thêm `/roadmap` +
`/bookmarks` là đỏ test, tức là test cản đúng việc M6 cần làm. Đã đổi sang assert
**từng path cũ còn tồn tại** (`LEGACY_PATHS`), theo cả 2 mặt: link render ra và
bảng route khai báo. Chi tiết: test router dùng `createMemoryHistory` nên `href`
là `/hoc` chứ không phải `#/hoc` — so bằng cách bỏ tiền tố `#` thay vì gắn cứng
(hash thật đã cover ở `router.test.ts` qua `routeHref`).

---

## 4. Số test

| | Trước | Sau |
|---|---|---|
| Test file | 21 | **26** (+5) |
| Test | 134 | **170** (+36) |
| FAIL | 0 | 0 |

| File | Số | Vì sao |
|---|---|---|
| `src/lib/mutationInvalidation.test.ts` | 13 | **F1 bắt buộc** — 11 `it.each` trên MỌI mutation + 2 test danh sách |
| `src/graphql/__typename.test.ts` | 5 | **F3 bắt buộc** — ma trận 2×2 + assert selection set thật |
| `src/routes/Dashboard.test.ts` | 4 | **F4 bắt buộc** — mount component thật, 4 mốc thời gian |
| `src/roadmap/order.test.ts` | 9 | **5.2** — sort `(position, id)`, id số, tổng thứ tự |
| `src/main.test.ts` | 3 | **F2 bắt buộc** — probe component dùng `useClient`/`useQuery` |
| `src/routes/CaiDat.test.ts` | +1 | **5.1** — canh không còn `<a download>` tới `/api/backup` |
| `src/App.test.ts` | +1 | **5.9** — tách thành 2 test assert từng path cũ |

---

## 5. Mutation-check 6/6 FAIL thật

Sửa ngược từng finding, chạy lại, **revert**:

| Mutation | Kết quả |
|---|---|
| **F1** xoá `await afterMutation()` trong `createDeck` | **1 đỏ** — `srs/api/createDeck: đọc lại sau mutation phải RA MẠNG` |
| **F2** `main.ts` về `provideClient()` top-level | **2 đỏ** — `test_installPlugins_cua_main_ts_that_su_provide_client` + `test_cach_plugin_cung_dung_client_khong_phai_client_thu_2` |
| **F3** xoá `__typename` khỏi `DECK_FIELDS`/`CARD_FIELDS` | **1 đỏ** — `test_DECK_FIELDS_va_CARD_FIELDS_co_typename` |
| **F4** đảo lại `markVisited` / `shouldRemind` | **1 đỏ** — `test_banner_hien_khi_giou_20h_va_hom_nay_chua_ghe` |
| **5.1** thêm lại `<a :href="buildBackupUrl()" download>` | **1 đỏ** — `test_khong_con_the_a_download_troi_thang_bam_xuong_file_loi` |
| **5.2** `compareId` so chuỗi thuần (bỏ ép số) | **1 đỏ** — `test_id_số_10_sau_id_số_9_không_so_chuỗi` |

Ngoài ra, 2 **sanity check chứng minh lưới bắt đúng chiều** (không phải mutation
nhưng đáng ghi vì chúng là bằng chứng test không xanh nhầm):

- **5.9 ngược**: thêm `/roadmap` thật vào `router.ts` mà **không** thêm vào
  `LEGACY_PATHS` ⇒ 2 test đỏ (path chưa khai báo). Thêm vào `LEGACY_PATHS` mà
  chưa thêm route ⇒ vẫn đỏ. **Thêm route thật + khai báo đúng ⇒ 17 test vẫn xanh**
  — đó mới là ý nghĩa của "không cản M6".
- **F1 chiều ngược**: thêm `deleteDeck` vào `srs/api.ts` mà không khai báo vào danh
  sách ⇒ 1 đỏ với `expected [ 'srs/api/deleteDeck' ] to deeply equal []`.

Đã revert **toàn bộ** mutation; 4 lệnh verify chạy lại sạch sau khi revert (§6).

---

## 6. Verify (chạy lại SAU khi revert hết mutation)

| Lệnh | Kết quả |
|---|---|
| `pnpm vitest run` | **26 file / 170 test PASS, 0 FAIL** |
| `pnpm typecheck` | sạch |
| `pnpm build` | OK |
| `grep -rn "react" src package.json` | **0 hit** (exit 1) |
| `find src -name "*.tsx"` | **0 file** |
| `cd .. && docker compose config` | hợp lệ (exit 0) |

Một lưu ý về log: test restore của `CaiDat` làm happy-dom in
`ECONNREFUSED 127.0.0.1:3000` ra **stderr**. Đã dò `fetch`, `net.connect`,
`XMLHttpRequest`: cả 3 lần gọi của test đều về `/query` và `/api/restore` đúng
như thiết kế, `:3000` là URL mặc định của chính happy-dom. Đây là **tiếng log môi
trường test, không phải request của app** và không làm test nào đỏ. Đã ghi vào
comment test để không ai mất thời gian debug nhầm lần sau.

---

## 7. SKIPPED (có chủ ý)

1. **`api/**`** — không sửa 1 dòng. Xem §8 nợ server.
2. **Không sửa `postZhDrillGrade` / `postEnChunks` thành tên đúng** (`gradeTone` /
   `chunkQuery`). Tên `post*` là di sản REST v1; đổi tên là sửa ngoài phạm vi 9
   mục, và đã ghi tường minh trong test thay vì để ai đó "sửa nhầm" thành
   mutation. **M6 nên đổi tên** khi có việc chạm tới.
3. **Không thêm `resize` listener cho popup Reader** — đó là thay đổi hành vi, M5
   là remediation. Đã sửa comment cho đúng thật.
4. **Không thêm test DOM cho 12 màn còn lại** — ngoài 9 mục. Ghi backlog M6 ở §8.

## 8. Backlog M6 (phải ghi, không tự làm ở M5)

1. **Token shadcn-vue còn thiếu.** `app.css` `@theme` chỉ có token **vai trò**
   (`--color-ink`, `--color-err`, `--radius-card`…). shadcn-vue đòi lớp token
   **HSL** (`--background`, `--foreground`, `--primary`, `--ring`, `--radius`…).
   M6 cài shadcn **phải viết lớp token ánh xạ**, không thể dùng thẳng `@theme`
   hiện tại — nếu không thì mọi component shadcn rơi về màu mặc định.
2. **`EnStress.vue:161-166` không dùng `DeckSelect`.** Drill SRS ở EnStress dùng ô
   nhập `deck id` tự do, trong khi 8 màn khác dùng `DeckSelect`. Đây là **di sản
   app v1** (màn này vốn đã lệch). M6 chuẩn hoá form sẽ vướng ngay; nên chuẩn hoá
   ở M6 khi thiết kế lại form.
3. **10/14 màn không có test DOM.** Hiện chỉ `Review`, `CaiDat`, `App` (+ `Dashboard`
   sau lần này) có test render. 10 màn còn lại (`Hoc`, `ErrorBook`, `Reader`,
   `EnStress`, `EnPvo`, `EnThieu`, `ZhPinyin`, `ZhStroke`, `ZhBingo`, `ShadowPlayer`,
   `Recorder`) **chỉ có test module thuần** ⇒ M6 sửa UI các màn này KHÔNG có lưới
   an toàn. M6 nên thêm test mount cho từng màn trước khi sửa.
4. **Nợ SERVER: `TopError` chỉ có `{word, count}`.** Yêu cầu T5.2 *"click lỗi →
   nhảy review card"* **không làm được** với schema hiện tại: `TopError`
   (`api/graph/schema/insight.graphqls:29`) không có `cardId`, nên client không
   biết lỗi đó thuộc thẻ nào. Cần thêm `cardId` ở **server** (`insight` + query
   `topErrors`/`insightTopErrors`), rồi mới làm được UI.
   **Không sửa `api/` ở M5** — giao M6/M7 cùng đợt roadmap (cùng bounded context
   `insight`, nên gộp lúc đó rẻ hơn).

## 9. Bài học cho M6 (rút ra từ 4 finding)

- **4/4 finding đều là lỗi im lặng** — không lỗi biên dịch, không cảnh báo, app
  vẫn chạy. Loại lỗi này chỉ bắt được bằng test **đo hành vi** (đếm request, đọc
  DOM), không bắt được bằng đọc code hay typecheck.
- **2 probe đầu của tôi cho kết quả ngược Oracle** vì đo sai (đếm nhầm request
  của chính mutation; mock không đọc body POST). Kết luận từ probe sai nguy
  hiểm hơn không có probe — **phải chạy lại từng bước cho tới khi số đo nhất
  quán**, và mô tả chính xác cách đo trong comment test.
- **Test phải bắt được cả CHÍNH file production đã sửa**, không chỉ bắt "cách làm
  đúng". F2 lọt vì `main.test.ts` (nếu có) chỉ chứng minh pattern đúng chứ không
  chứng minh `main.ts` dùng pattern đó. Tách `installPlugins` ra là cách rẻ
  nhất để làm việc đó.
- **Lưới bảo vệ phải kiểm chứng bằng chiều ngược**: thêm mutation mới mà
  không khai báo thì có đỏ không? Thêm route mới thì test cũ còn xanh không? Hai
  câu hỏi này bắt được test xanh nhầm mà không cần đọc lại test.
