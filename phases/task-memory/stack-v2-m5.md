# STACK-V2 — M5 (Vue 3 + Tailwind 4 + urql + vue-query: port 14 màn)

> Hoàn tất 2026-09-28. Gate của M5: oracle review **port fidelity** (không mất
> tính năng nào của 14 màn React).
>
> M1: `stack-v2-m1.md` (+ remediation). M2: `stack-v2-m2.md`. M3: `stack-v2-m3.md`
> + `m3-remediation.md` + `m3-final.md`. M4: `stack-v2-m4.md` (+ remediation).
> M5 **xoá React khỏi `web/`**, **không** sửa `api/**`.

## 0. Tóm tắt 1 dòng

React → Vue 3.5.43 hoàn tất: **16 file `.tsx` → 22 file `.vue`**, **0 dòng
`react` còn lại trong `web/src`**, `vitest` **134 PASS** (105 trước → 134, không
bớt test nào), `vue-tsc` sạch, `vite build` OK.

---

## 1. Số liệu chính

| Hạng mục | Trước | Sau |
|---|---|---|
| File `.tsx` | 16 | **0** |
| File `.vue` | 0 | **22** (14 màn + `App.vue` + 7 component dùng chung) |
| Test (`vitest run`) | 105 | **134** |
| Test file | 21 | 21 |
| `grep -rn "react" src package.json` | nhiều | **0 hit** (kể cả comment, kể cả `reactive`) |
| `glob src/**/*.tsx` | 16 file | **0 file** |

### 1.1 16 file `.tsx` đã xoá → 22 file `.vue` tạo ra

| Cũ (`.tsx`) | Mới (`.vue`) |
|---|---|
| `src/main.tsx` | `src/main.ts` |
| `src/App.tsx` | `src/App.vue` |
| `src/routes/{Hoc,Review,Recorder,ErrorBook,Reader,Dashboard,ZhPinyin,ZhStroke,ZhBingo,EnStress,EnPvo,EnThieu,CaiDat}.tsx` | 13 file cùng tên trong `src/routes/` |
| `src/player/ShadowPlayer.tsx` | `src/player/ShadowPlayer.vue` |

7 component tái sử dụng mới (app v1 copy cùng 1 khối JSX ở 3–6 màn):

| Component | Thay cho khối lặp ở |
|---|---|
| `components/NoticeBar.vue` | `<p style={{color:'red'}}>` — 13 màn |
| `components/GradeRow.vue` | mảng `GRADES` copy ở Review / EnStress / EnPvo |
| `components/DeckSelect.vue` | `<label>Deck: <select>` — 8 màn |
| `components/SrsCard.vue` | khối "mặt trước → hiện đáp án → 4 nút" ở Review / EnStress / EnPvo |
| `components/ToneCard.vue` | `ToneCard` (audio mẫu + badge `X-Engine`) ở ZhPinyin |
| `components/ThieuChecklist.vue` | `ThieuChecklist` ở EnThieu |
| `components/ThieuChart.vue` | `ThieuChart` ở EnThieu |

### 1.2 File `.ts` cũ bị XOÁ (client REST của app v1)

| Xoá | Vì sao | Test của nó đi đâu |
|---|---|---|
| `src/api.ts` | `/api/decks`, `/api/search`, `/api/review`… **không còn** trong app-v2 (`internal/transport/http/routes.go` cố ý chỉ giữ 5 endpoint nhị phân) | `api.test.ts` + `decks.test.ts` + `audio.test.ts` → `rest/client.test.ts` + `srs/api.test.ts` |
| `src/player/api.ts` | `POST /api/sync` (form file) + `/api/errors` + `/api/reader/levels`… không còn | `sync.test.ts` + `release.test.ts` → `player/api.test.ts` |
| `src/chinese/api.ts` | `/api/zh/*` không còn | `chinese/api.test.ts` → `chinese/api.test.ts` (viết lại) |
| `src/english/api.ts` | `/api/en/*` không còn | `english/api.test.ts` → `english/api.test.ts` (viết lại) |

Thay bằng 3 module client mới: `src/srs/api.ts`, `src/rest/client.ts` +
`src/rest/useRest.ts`, và các `src/{chinese,english,player}/api.ts` viết lại
quanh `runQuery`/`runMutation`.

---

## 2. Helper invalidate 2 cache — đặt ở đâu

**`src/lib/afterMutation.ts`** — đúng 1 hàm duy nhất:

```ts
export async function afterMutation(): Promise<void> {
  markAllGraphQLStale();                                            // urql
  await queryClient.invalidateQueries({ queryKey: [...REST_NAMESPACE] }); // vue-query
}
```

Quy tắc bắt buộc đã chốt: **không màn nào tự gọi `invalidate`**. 17 call site
`assertPayloadOk`/`runMutation` trong `src/{srs,chinese,english,player}/api.ts`
gọi nó **bên trong hàm client**, nên thêm mutation mới vào 1 trong 4 file đó là
tự đúng — không có đường để quên.

- `queryClient` là **instance singleton** ở `src/rest/queryClient.ts`, và
  `main.ts` cài `VueQueryPlugin` với đúng instance đó. Tạo client thứ hai ở
  `main.ts` là invalidate chạy trên client không ai mount ⇒ mọi query REST kẹt
  cache im lặng.
- `markAllGraphQLStale()` (đã có từ lane trước) tăng "kỷ nguyên" ⇒ key nào
  chưa lấy ở kỷ nguyên mới đi `network-only`. Không dùng `purgeExchange` vì
  `cacheExchange` đã loại đúng entity mutation trả về.

### 2.1 Test bắt buộc — `src/lib/afterMutation.test.ts` (4 test)

Test **hành vi thật**, không mock 2 module rồi assert "đã gọi" (dạng đó xanh kể
cả khi hàm gọi sai hàm):

| Test | Chứng minh |
|---|---|
| `test_second_read_same_generation_hits_cache_only` | 2 lần `runQuery` cùng kỷ nguyên ⇒ **1** request mạng (điều kiện để test dưới có nghĩa) |
| `test_read_after_after_mutation_goes_back_to_network` | `afterMutation()` rồi đọc lại ⇒ **2** request |
| `test_rest_query_marked_invalidated_by_after_mutation` | query vue-query dưới namespace `api` chuyển `isInvalidated` false→true |
| `test_after_mutation_invalidates_by_rest_namespace` | `invalidateQueries` được gọi với `queryKey: ['api']` |

**Mutation-check 2/2 FAIL thật**: bỏ `markAllGraphQLStale()` → 1 test đỏ; bỏ
`invalidateQueries` → 2 test đỏ.

---

## 3. 3 test bắt buộc còn lại

| Gotcha | Test | Mutation-check |
|---|---|---|
| #2 lỗi GraphQL hiện đúng `message` | `src/routes/Review.test.ts::test_business_error_message_renders_unescaped_vietnamese` — mount thật màn Review, `recordReview` trả `ok:false` + `error.message` tiếng Việt, assert DOM chứa **nguyên văn** và **không** chứa `lỗi hệ thống` | Sửa `payloadError` trả message mặc định ⇒ **4 test đỏ / 21 file** |
| #2 (tầng data) | `src/srs/api.test.ts::test_api_error_throws_vietnamese_message` | cùng mutation trên |
| #3 `/api/backup` 501 → UI báo lỗi | `src/routes/CaiDat.test.ts` 3 test: bấm nút → gọi thật `/api/backup` → message tiếng Việt của server lên DOM; **không** render nhánh thành công; `/api/restore` 501 cũng vậy | Xoá dòng `NoticeBar` hiện `backup.error` ⇒ **1 test đỏ** |

Ngoài ra: `src/rest/client.test.ts` khẳng định 501 **không** bị nuốt thành
blob rỗng (bug mà app v1 có: `<a download>` tải về file JSON lỗi rồi báo xong).

---

## 4. Test: 105 → 134, không bớt test nào

Bảng đối chiếu từng file cũ → mới (giữ **đúng số** test, chỉ đổi chủ thể kiểm
từ URL REST sang `operationName` + `variables` GraphQL):

| Test file cũ | Số | Đi qua |
|---|---|---|
| `api.test.ts` | 2 | `rest/client.test.ts` (health) + `srs/api.test.ts` (dictSearch) |
| `audio.test.ts` | 5 | `rest/client.test.ts` (giữ nguyên 5) |
| `decks.test.ts` | 5 | `srs/api.test.ts` (giữ nguyên 5) |
| `sync.test.ts` | 4 | `player/api.test.ts` (4) |
| `release.test.ts` | 4 | `player/api.test.ts` (4) |
| `chinese/api.test.ts` | 9 | `chinese/api.test.ts` (9) |
| `english/api.test.ts` | 4 | `english/api.test.ts` (4) |
| `routes/ZhPinyin.test.ts` | 1 | `chinese/api.test.ts` (thành test ánh xạ pinyin→tone/pair) |
| `routes/ZhBingo.test.ts` | 4 | `chinese/api.test.ts` (3) + `chinese/strokes.test.ts` (1) |
| **12 file logic thuần** (bingo, pinyin, strokes, chunk, stress, thieu, abloop, diff, streak, router, EnStress, EnThieu) | 67 | giữ nguyên, chỉ sửa **kiểu** id `number`→`string` |

**+29 test mới** (không lấp chỗ trống nào):

| File | Số | Vì sao mới |
|---|---|---|
| `App.test.ts` | 16 | 2 test shell (14 link, active route) + **14 test `it.each`** mỗi path cũ 1 màn — đây là bằng chứng "bookmark cũ còn chạy" |
| `lib/afterMutation.test.ts` | 4 | bắt buộc (gotcha #1) |
| `routes/CaiDat.test.ts` | 3 | bắt buộc (gotcha #3) |
| `routes/Review.test.ts` | 3 | bắt buộc (gotcha #2) + 2 test hành vi ôn (lỗi giữ thẻ trong hàng đợi) |
| `rest/client.test.ts` | +1 | 501 backup |
| `srs/api.test.ts` | +2 | `AppError.code` là enum server |
| `chinese/api.test.ts` | +3 | ánh xạ pinyin→tone/pair, bingo 12 thẻ, `parseSyllable` trung tính → tone 5 |

Hạ tầng test mới: `src/test/harness.ts` (`mountScreen` + `makeTestRouter`, cài
đúng router memory + `VueQueryPlugin` + stub `RouterLink`) và
`src/test/graphqlMock.ts` (đọc `operationName` từ GET query-string **hoặc** POST
body — urql tự chọn, test không cần biết).

---

## 5. Cách dò query khi dev

> **Bắt buộc đọc** (gotcha #4 của gate M4): `docker-compose.yml` đặt
> `GIN_MODE: release` ⇒ `/playground` **404** và CORS chỉ nhận
> `http://localhost:5173`. Dùng Vite dev thì URL **phải là
> `http://localhost:5173`**.

```bash
cd api && docker compose --profile v2 up -d   # app-v2 ở :8081
cd ../web && pnpm dev                        # http://localhost:5173
```

`vite.config.ts` proxy `/query` + `/playground` + `/api` → `http://localhost:8081`
(đổi bằng env `LANGAPP_V2`). Nhờ proxy mà origin của trang là 5173 ⇒ không cần
header CORS. **Xác minh proxy còn sống** (không phải app hỏng):

```bash
curl -s -X POST http://localhost:5173/query \
  -H 'Content-Type: application/json' \
  -d '{"query":"{ decks { id name lang } }"}'        # kỳ vọng {"data":{"decks":[]}}
curl -s http://localhost:5173/api/health            # {"status":"ok"|"degraded"}
```

`502` từ 2 lệnh trên = `app-v2` chưa boot (Vite proxy không có upstream), **không**
phải lỗi của `web/`. Dò trực tiếp upstream khi cần:

```bash
curl -s -X POST http://localhost:8081/query -H 'Content-Type: application/json' -d '{"query":"{ decks { id } }"}'
```

Mở app: `http://localhost:5173/#/hoc` (hash router, 14 path cũ giữ nguyên).

---

## 6. Bảng màu — không phát minh, lấy từ đúng inline style của app v1

`src/styles/app.css` (do lane trước setup) đã định nghĩa token theo **vai trò**.
Mỗi giá trị bám 1 màu có sẵn trong app v1:

| Token | Giá trị | Ô trong app v1 |
|---|---|---|
| `--color-ink` | `#111111` | màu nét canvas (ZhStroke) |
| `--color-muted` | `#666666` | text phụ, pinyin, nguồn bài đọc |
| `--color-dim` | `#888888` | token "thừa" trong diff (Recorder) |
| `--color-line` / `--color-line-strong` | `#999999` / `#333333` | viền canvas nét · viền popup tra từ |
| `--color-err` | `#dc2626` | mọi `<p style={{color:'red'}}>` |
| `--color-ok` | `#16a34a` | `<p style={{color:'green'}}>` |
| `--color-warn` | `#aa6600` | nhãn "ngoại lệ" trọng âm, viền cảnh báo |
| `--color-remind` | `#fff3cd` | banner nhắc học 20:00 (Dashboard) |
| `--color-cell-ok` / `cell-bad` / `cell-free` | `#b7f0b1` / `#f5b5b5` / `#ffe9a8` | bàn bingo 3×3 |
| `--radius-card` | `4px` | bo góc nút/card |

Tailwind 4 không có `tailwind.config.js`; class dùng token (`text-err`,
`bg-cell-ok`, `rounded-card`, `border-line`) **đã được emit thật** vào
`dist/assets/index-*.css` — đã grep xác nhận.

Icon: `lucide-vue-next` (đã có sẵn trong `package.json`), dùng ở
`SrsCard`/`ToneCard`. **Không** kéo `shadcn-vue` (M6 mới).

---

## 7. Port fidelity — 3 thay đổi hành vi, đều bắt buộc

### 7.1 `DrillItem` do client dựng (không còn server trả)

App-v2 không có `/api/zh/drill`; server chỉ có `pinyin` (`"ni3 hao3"`). `tone`
(`"3 3"`) và `pair` (`"3-3"`) **suy ra bằng `chinese/pinyin.parseSyllable`** trong
`chinese/api.ts::toDrillItem`. Giữ nguyên **tên field của v1**
(`card_id`, `pinyin_marks`) để `chinese/bingo.ts` + `chinese/drill.ts` (thuần,
đã có 20 test) không phải đổi.

### 7.2 `postZhBingoScore` ghi SRS từng thẻ

App-v2 không có mutation bingo riêng. `postZhBingoScore` gọi `recordReview` cho
từng ô (đúng = grade 4, sai = grade 1) rồi trả về **đúng hình dạng kết quả của
v1** (`{deckId, total, correct, grades}`) nên UI không phải biết khác biệt.

### 7.3 `uploadSync(file)` không còn

App-v2 không nhận file peer qua HTTP (`syncinfra.NewSchemaLoader(nil)` chưa nối
peer — M4 §8). `runSync()` gọi mutation `sync` không tham số; Cài đặt **giữ
nút**, gọi để hiện lỗi tiếng Việt nguyên văn thay vì giấu. Ô chọn file peer
được bỏ kèm dòng giải thích.

### 7.4 Đổi kiểu do `ID` là `String`

`card_id` / `ThieuSession.id` / `progressKey(deckId)` / `findCardForHanzi` nhận
**chuỗi** (id DB là `bigint`; ép `Number` là mất chính xác). 5 test cũ sửa
kiểu theo — **không bỏ test nào**.

---

## 8. Lỗi thật phát hiện + sửa khi port (đáng đọc cho M6)

1. **`payloadError` trả về, không ném.** 17 call site gọi `payloadError(x)` rồi
   tiếp tục ⇒ payload `ok:false` lọt qua im lặng, màn hiện *"lỗi hệ thống"* thay
   vì message tiếng Việt thật — **vi phạm đúng gotcha #2**. Sửa: thêm
   `assertPayloadOk()` (gộp 2 bước thành 1) trong `graphql/errors.ts`; giữ
   `payloadError` nguyên trạng.
2. **`GraphQLRequest.key` của urql là `number`**, không phải `string` như
   `client.ts` khai. `Map<string, number>` vẫn chạy nhưng là kiểu sai; sửa thành
   `RequestKey = ReturnType<typeof createRequest>['key']`.
3. **`queryFn` của vue-query nhận 1 object `context`**, còn `fetchHealth(base?)`
   nhận `string` có default ⇒ TS không bắt, base nhận nhầm context. Bọc closure.
4. **`ref(() => x)` ≠ lazy.** `useState(() => …)` của app v1 lazy; `ref` của Vue
   **giữ hàm làm giá trị** ⇒ `scores.value` là 1 hàm, `chunks.value` là 1 hàm.
   2 chỗ này đã sửa thành dựng thẳng.
5. **Server giả trả payload theo `operationName` ⇒ im lặng hỏng.** Trả
   `{[name]: data[name]}` với `Decks`/`decks` lệch hoa/thường làm `data.x`
   thành `undefined`, render đỏ mà không có lỗi network. `App.test.ts` trả
   **một** `data` chứa toàn bộ field.

---

## 9. Verify (đã chạy, kết quả thật)

| Lệnh | Kết quả |
|---|---|
| `pnpm vitest run` | **21 file / 134 test PASS, 0 FAIL** |
| `pnpm typecheck` (`vue-tsc --noEmit`) | sạch |
| `pnpm build` | OK, 21 chunk lazy (mỗi màn 1 chunk) |
| `grep -rn "react" src package.json` | **0 hit** (exit 1) |
| `grep -rni "react" src package.json` | **0 hit** — kể cả `reactive` trong comment |
| `find src -name "*.tsx"` | **0 file** |
| `cd .. && docker compose config` | hợp lệ (exit 0) |
| `pnpm dev` + `curl :5173/query` | 502 (app-v2 chưa boot — proxy đúng, xem §5) |

Mutation-check 4/4 **FAIL thật** (đã revert): bỏ `markAllGraphQLStale` (1 đỏ) ·
bỏ `invalidateQueries` (2 đỏ) · `payloadError` không trả message server (4 đỏ) ·
xoá hiện lỗi backup 501 (1 đỏ).

---

## 10. SKIPPED (có chủ ý)

1. **Roadmap UI / bản đồ game / bookmark** — của **M6**. M5 không tạo route mới:
   `ROUTES` vẫn đúng 14 mục, không mục nào thêm.
2. **`shadcn-vue`** — plan §6 chỉ lấy form/modal/table cho roadmap; kéo ở M5 là
   chi phí mà không màn nào dùng tới.
3. **`/playground` trong dev** — `GIN_MODE: release` tắt. Dùng `curl` §5.
4. **`Reader.fetchReaderLevels`** gọi `readerArticles` rồi suy ra level thay vì
   endpoint riêng — app-v2 không có `readerLevels`; vẫn trả về `string[]` nên
   UI không đổi.
5. **`/api/backup` download bằng `<a download>`** — bản v1 tải file JSON lỗi 501
   rồi báo xong. M5 gọi thật qua vue-query và hiện lỗi; nhánh tải file chỉ hiện
   khi server **thành công** (M7).
6. **`api/**`** — không sửa 1 dòng. Toàn bộ khác biệt client-side.
7. **`Cargo`/`audio-service`** — không liên quan M5.

## 11. Nợ chuyển M6/M7

- **`@urql/vue` `provideClient` chưa ai dùng** — đã cài ở `main.ts` để M6 lấy
  `useClient` cùng 1 document cache với tầng thực thi.
- **`rest/queryClient.ts` khai `restKeys.restore()` nhưng `useRestoreMutation`
  không dùng key đó** (`useMutation` không có `queryKey`). Xoá khoá hoặc dùng
  cho `mutationKey` khi M6 thêm endpoint.
- **`fetchTTSEngine` huỷ body sau khi đọc header** — 2 request TTS mỗi lần bấm
  "Play TTS" ở Cài đặt (1 tải audio, 1 probe header). Gộp khi M6 có chỗ để lưu
  `X-Engine` dài hạn.
- **SineWAV nhân bản 3 lần** (M4 đã ghi) — `api/audio.go` của app v1 xoá được
  khi app v1 bị thay thế; **không** đụng ở M5.
- **`GetPath` còn N+1 theo stage** (nợ M4, chỉ còn test dùng) → M7 hoặc M6 khi
  dựng bản đồ.
