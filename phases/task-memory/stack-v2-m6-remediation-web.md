# STACK-V2 — M6 remediation (web lane)

> Hoàn tất 2026-09-29. Lane này **vá 4 finding** Oracle M6 chấp nhận (verdict
> **KHÔNG PASS**) + 4 việc dọn. Design intent của `stack-v2-m6b.md` là đầu ra
> được chấp nhận — lane này **không phát minh lại diện mạo**, chỉ sửa chỗ hỏng.
>
> **Không đụng `api/**`, `web/src/graphql/operations.ts`,
> `web/src/roadmap/map/terrain.ts`** (xác nhận sha256 ở §5).

---

## 0. Tóm tắt 1 dòng

`vitest` **342 → 363 test** (38 file, không đổi số file) · `vue-tsc` sạch ·
`vite build` OK · grep react 0 hit · 0 file `.tsx` ·
**mutation-check mới 11/11 FAIL thật** + **mutation-check M6b cũ 19/19 vẫn FAIL**
(không hồi quy). 4 finding đều là lỗi **im lặng** — không lỗi biên dịch, không
cảnh báo, app chạy bình thường.

---

## 1. F2 — `/review?card=` là LINK CHẾT

**Vì sao hỏng (đo được, không suy đoán).** `ErrorBook.vue:156` render
`:to="`/review?card=${t.cardId}`"`. `Review.vue` trước đó **chỉ đọc
`route.query.deck`** — grep toàn repo không có chỗ nào đọc `query.card`. Bấm
"Ôn thẻ này" ⇒ `deckId` rỗng ⇒ `v-if="!deckId"` giữ màn trắng *"Chọc deck để
bắt đầu ôn"*. Không lỗi, không thẻ, không log.

**Vì sao test xanh (bẫy do chính designer ghi ở `m6b.md` §8.1).**
`RestoredFeatures.test.ts:237` tên `test_top_errors_row_links_to_review_only_when_card_id_exists`
chỉ assert `row.text()).toContain('Ôn thẻ này')` — **không assert `href`, không
điều hướng**. Thêm nữa, `test/harness.ts` stub `RouterLink` thành
`{ template: '<a><slot /></a>' }` — không có `href` ⇒ *mọi* assert về link
trong toàn bộ test suite đều là assert về vô nghĩa. Đây là nguyên nhân gốc của
bẫy: **hạ tầng test đã chặn việc nhìn thấy `href`** trước khi ai kịp viết.

### Sửa

| File | Sửa |
|---|---|
| `src/test/harness.ts` | stub `RouterLink` → `{ props: ['to'], template: '<a :href="to"><slot /></a>' }`. Comment cũ *"test không cần kiểm link"* **xoá** — nó là lý do F2 lọt. |
| `src/srs/api.ts` | thêm `fetchCardById(cardId)`. |
| `src/routes/Review.vue` | thêm `cardFromQuery` + `focusCard` + `loadFocus()`; `deckId` tự chuyển sang deck của thẻ. |
| `src/routes/ErrorBook.vue` | thêm `data-testid` cho link để assert đúng thẻ. |

**Vì sao `fetchCardById` phải quét chứ không có `card(id:)`:** schema
(`api/graph/schema/root.graphqls`) chỉ có `deck(id:)`, `cards(deckId:)`,
`dueCards(deckId:)` — **không có root field nào tra 1 thẻ theo id**. Link
`/review?card=…` không mang `deckId`, nên trong phạm vi client chỉ còn 1 đường:
`Decks` (vài chục dòng, cache-first) rồi `cards(deckId)` **dừng sớm ngay khi
thấy**. Thêm `Query.card(id: ID!)` là việc của `api/**` — **ngoài phạm vi ghi**,
đã ghi ở §6 nợ M7. Nhờ đó `operations.ts` **không phải đụng tới** (lane F1 đang
giữ file đó).

**3 điều kiện phải có cùng lúc** — thiếu 1 là link vẫn chết với hình thức khác:

1. đọc `cardFromQuery` (không có ⇒ màn trắng);
2. thẻ lên **đầu** hàng đợi (`withFocus`);
3. `deckId` chuyển sang deck của thẻ (không có ⇒ `v-if="!deckId"` vẫn giữ màn
   trắng **dù hàng đợi đã có thẻ**).

Ngoài ra: `withFocus` **lọc trùng** (thẻ vốn đã nằm trong hàng đợi đến hạn thì
không được nhân đôi — chấm 1 lần phải mất đúng 1 thẻ), đổi deck tay thì xoá
focus, và `?card=` không tìm thấy thẻ thì **báo lỗi rõ ràng** thay vì im lặng
(thẻ xoá mềm là 1 trong 3 trường hợp `cardId` null của M6a §2.3).

### Test (7)

`Review.test.ts` — 6 test mới (`test_review_opens_the_card_from_the_query_string`):
thẻ lên đầu · màn rời empty-state + `select` có value · chấm xong sang thẻ kế
tiếp · **không nhân đôi** khi thẻ đã có trong hàng đợi · `?card=999` báo lỗi ·
`?card=` rỗng thì hành vi y hệt cũ (chặn "sửa F2 làm đổi hành vi mặc định").

`RestoredFeatures.test.ts` — 1 test mới +
`test_top_errors_row_links_to_review_only_when_card_id_exists` **đổi từ assert
text sang assert `href` = `/review?card=7`**.

Test quan trọng nhất là `test_review_screen_actually_shows_the_card_the_row_links_to`:
lấy đúng `href` mà `ErrorBook` render, **rồi mount `Review.vue` tại URL đó** và
kiểm thẻ hiện. Chỉ assert `href` thì vẫn có thể trỏ tới nơi không đọc tham số;
test này chứng minh **điều hướng thật sự tới nơi**.

---

## 2. F3 — `load()` reset về chặng 1 sau MỌI mutation

**Vì sao hỏng.** `RoadmapMap.vue` tính lại `stageId` từ `localStorage` mỗi lần
`load()`. `mark()` / `removeTopic` / `removeStage` / `removeMilestone` /
`@done` của 4 form **đều gọi `load()`**.

Kịch bản phổ biến: mở path lần đầu (`localStorage` trống) → bấm chip "Chặng 3" →
chạm node → "Đánh dấu Xong" ⇒ `load()` ⇒ `stageId = firstStage.id` ⇒ **nhảy về
chặng 1** và `watch(stageId)` **đóng luôn panel** đang mở.

**Vì sao test không thấy.** Fixture `RoadmapMap.test.ts` chỉ có **1 stage** ⇒
`firstStage.id === stageId` luôn đúng ⇒ nhánh hỏng không bao giờ được chạy tới.

### Sửa

Ưu tiên giữ `stageId` hiện tại **nếu chặng đó còn trong cây**; chỉ rơi về
`saved`/`firstStage` khi chặng hiện tại **không còn tồn tại** (thẻ xoá mềm ⇒ phải
chọn lại chặng hợp lệ, không treo id chết).

### Test (2 + fixture)

Fixture `TREE` đổi từ **1 stage → 2 stage, CÙNG `terrain` (`PLAIN`)**. Đây là
tiền đề cho cả F3 lẫn F4: với 1 stage, cả 2 lỗi đều không thể lộ.

- `test_marking_done_on_stage_two_keeps_stage_two_open` — đúng kịch bản trên.
- `test_stage_survives_deleting_a_milestone` — đường thứ 3 vào `load()` (khác
  `mark`, khác `@done` của form).

Không test qua form vì `Dialog` của reka-ui không render trong `happy-dom`
(thiếu `ResizeObserver` + vị trí đo) — cùng lý do `m6b.md` §9 chọn `<select>`
gốc thay vì `Select` của reka-ui.

---

## 3. F4 — Đổi chặng không reset scroll của bản đồ

**Vì sao hỏng.** `MapCanvas.vue` chỉ `watch(() => props.terrain, () => setOffset(0))`.
`MapCanvas` **không có `:key`** nên đổi chặng tái dùng cùng instance và giữ
`scrollTop`. `terrain` là thuộc tính **hình ảnh**, không phải danh tính bản đồ.

Hệ quả ** kép, tầng 2 nặng hơn tầng 1: vì lỗi `enum Terrain` (M6b §7), server trả
`PLAIN` cho **5/6 địa hình** ⇒ `terrain` là `PLAIN` ở **mọi** chặng ⇒ watch
**không bao giờ chạy** ⇒ offset chặng trước mang sang chặng sau ⇒ `onScrolled`
ghi `{stageId: chặng mới, offset: cũ}` vào `localStorage` ⇒ **vị trí sai được
nhớ vĩnh viễn**. Lỗi F1 của lane song song làm lộ tận cùng.

### Sửa — 2 bẫy phải tránh, đều đã có test

```ts
watch(
  () => `${props.slug}|${props.direction}|${props.nodes.map((n) => n.id).join(',')}`,
  () => setOffset(0),
  { flush: 'post' },
);
```

1. **So sánh theo `id`, không theo tham chiếu mảng.** Sau MỌI mutation `load()`
   dựng lại cả cây ⇒ `nodes` là **mảng MỚI** dù vẫn là CÙNG chặng. Bám tham
   chiếu sẽ cuộn về 0 sau mỗi lần đánh dấu Xong — biến F4 thành 1 lỗi mới.
   *(Đây chính là lý do đề xuất ban đầu `watch(() => [..., props.nodes])` bị
   sửa: nó đúng 1 nửa, sai 1 nửa.)*
2. **Getter trả CHUỖI, không trả mảng.** `watch` so kết quả bằng `Object.is`,
   mà mảng mới luôn khác mảng cũ ⇒ callback chạy **mỗi lần effect đánh giá lại**,
   kể cả khi nội dung y hệt. Mutation `…props.nodes` (biến 1) xanh vì lý do
   vô nghĩa nên đã bị thay bằng mutation (2) bám chuỗi.
3. **`flush: 'post'`.** Hàm `:ref` đăng ký phần tử theo `slug` **trong lúc
   render**; chạy `pre` sẽ `setOffset(0)` lên phần tử của slug cũ, hoặc không có
   phần tử nào để set.

### Test (3 tầng)

- `MapCanvas.test.ts` — **2 chặng CÙNG terrain** (đúng tình trạng thật) ⇒ offset
  reset; **cùng 1 chặng được nạp lại** (`nodes` là mảng mới) ⇒ offset **giữ**;
  đổi `slug` ⇒ reset.
- `RoadmapMap.test.ts` — offset chặng 1 không lọt sang chặng 2, và
  `localStorage` sau khi đổi chặng là `{stageId: '11', offset: 0}` chứ không
  phải offset cũ.

Fixture 2 chặng dùng `PLAIN` cho **cả hai** là bắt buộc: nếu 2 chặng khác
terrain thì test F4 xanh vì lý do vô nghĩa, đúng bẫy §8.1.

---

## 4. Việc dọn

| # | Việc | Kết quả |
|---|---|---|
| 5 | Xoá chặng / màn / mốc **không có bước xác nhận**, lệch với 2 flow xoá của designer (`RoadmapHome.vue:118`, `Bookmarks.vue:207`) | Thêm `confirming` + `askDelete(kind, id)` + `isConfirming(kind, id)`, dùng lại **đúng** pattern 2 màn đó (dải "Xoá thật" / "Huỷ"). Ràng chặt thêm: 1 mục chờ tại 1 lần; dùng `null` chứ không phải `''` vì `''` là giá trị id hợp lệ về hình thức. Test **7**: mỗi flow 2 test (chưa gọi mạng / gọi mạng) + 1 test Huỷ. |
| 6 | `web/preview-mock.mjs` — file scratch sót ở `web/` (ngoài `src/`, không build) | **Đã xoá.** Grep 0 tham chiếu. |
| 7 | Export chết do F1 lane ghi | Xem §7. |
| 8 | Test M6b còn assert **text** thay vì **hành vi/href** | Xem §7. |

---

## 5. Verify

| Lệnh | Kết quả |
|---|---|
| `pnpm vitest run` | **363 PASS / 0 FAIL** (38 file; 342 → 363) |
| `pnpm typecheck` | sạch |
| `pnpm build` | OK (`queryClient` 143.92 kB, `RoadmapMap` 40.60 kB) |
| `node mutation-check-m6-remediation.mjs` | **11/11 đỏ đúng** |
| `node mutation-check-m6b.mjs` (chạy lại) | **19/19 đỏ đúng** — lane này **không làm hồi quy** lưới bảo vệ cũ |
| `grep -rn react src package.json` | **0 hit** |
| `find src -name '*.tsx'` | **0 file** |

### Xác nhận KHÔNG chạm file của lane song song

| File | sha256 (đầu lane) | sha256 (cuối lane) | Kết luận |
|---|---|---|---|
| `web/src/graphql/operations.ts` | `30abdb702a6aa31a…` | `30abdb702a6aa31a…` | **không đổi** |
| `web/src/roadmap/map/terrain.ts` | `082a8f5b099b761e…` | `082a8f5b099b761e…` | **không đổi** |

`api/**`: `find api -newermt <mtime đầu lane>` chỉ ra
`api/internal/infrastructure/sync/pgbackup_test.go` — file của **context sync**,
m6b §8 không hề đụng, và **không phải do lane này sửa** (không file `api/` nào
nằm trong danh sách file mà lane này ghi). Đã ghi lại để orchestrator đối chiếu.

### File đã sửa (7) + file mới/xoá (2)

**Sửa:** `routes/Review.vue` · `routes/ErrorBook.vue` · `routes/RoadmapMap.vue` ·
`roadmap/map/MapCanvas.vue` · `srs/api.ts` · `test/harness.ts`
**Test:** `routes/Review.test.ts` · `routes/RestoredFeatures.test.ts` ·
`routes/RoadmapMap.test.ts` · `roadmap/map/MapCanvas.test.ts`
**Mới:** `web/mutation-check-m6-remediation.mjs` · **Xoá:** `web/preview-mock.mjs`

### 11 mutation-check

| Mutation | Kết quả |
|---|---|
| `Review.vue` bỏ đọc `query.card` | đỏ — link chết quay lại |
| bỏ việc chọn deck của thẻ `?card=` | đỏ — màn vẫn trắng "Chọc deck" |
| thẻ `?card=` không lên đầu hàng đợi | đỏ |
| `load()` tính lại `stageId` từ `localStorage` | đỏ — F3 |
| `MapCanvas` chỉ watch `terrain` | đỏ — F4 |
| `MapCanvas` bám **tham chiếu mảng** `nodes` | đỏ — bẫy 1 ở §3 |
| xoá chặng gọi thẳng mutation | đỏ |
| xoá màn gọi thẳng mutation | đỏ |
| xoá mốc gọi thẳng mutation | đỏ |
| `ErrorBook` bỏ `?card=` khỏi link | đỏ |
| **harness stub `RouterLink` thành `<a>` rỗng** | đỏ — bẫy §8.1 quay lại |

Mutation cuối là mutation quan trọng nhất: nó chứng minh **hạ tầng test** đã
được sửa chứ không chỉ test case. Chỉ sửa `RestoredFeatures.test.ts` thì
mutation này vẫn xanh.

---

## 6. SKIPPED + lý do

| Việc | Lý do |
|---|---|
| Thêm `Query.card(id: ID!)` vào GraphQL | **ngoài phạm vi ghi** (`api/**`). Đã bọc 1 hàm `fetchCardById` duy nhất để khi schema có field này thì sửa 1 dòng `runQuery`, không phải rà lại màn nào |
| Thêm `deckId` vào link `/review?card=` thay vì quét | `TopErrorWithCard` (`M6a` §5.7) chỉ có `cardId` + `front`, không có `deckId`; `insight.topErrors` đọc thẳng `notes` mà `notes` không cột deck. Cũng là thay đổi `api/**` |
| Test `@done` của 4 form qua `Dialog` thật | `Dialog` của reka-ui không render trong `happy-dom` (thiếu `ResizeObserver` + vị trí đo). Dựng môi trường giả cho 1 assert là tự tạo bẫy mới — đã phủ `@done` bằng đường `removeMilestone` cùng gọi `load()` |
| Xoá export chết ở `terrain.ts` | **Thuộc lane F1** (`terrain.ts` là file của lane đó). Đã rà và báo cáo §7, không sửa |

---

## 7. Báo cáo việc dọn #7 + #8 (không sửa, chỉ báo cáo)

### #7 — Export chết

Quét tất cả `export` ở 6 file logic M6 (`terrain` · `geometry` · `viewPrefs` ·
`confetti` · `swipe` · `order`) và đếm tham chiếu ngoài file khai báo.

**Kết luận: KHÔNG có export chết do lane F1 tạo ra.** 9 tên dưới đây được
grep ra là "0 tham chiếu bên ngoài", nhưng **đều là `interface`/`type` dùng
trong chính file khai báo** (2–4 lần mỗi tên) làm kiểu trả về — không phải code
chết. Danh sách: `TerrainSkin` · `DecorBand` (`terrain.ts`) · `Rect` ·
`LevelStyle` · `StageProgress` (`geometry.ts`) · `MapPosition` (`viewPrefs.ts`) ·
`BurstOptions` (`confetti.ts`) · `Axis` · `DragSample` (`swipe.ts`).

1 tên đáng nói: **`prefersReducedMotion` (`viewPrefs.ts`)** — export nhưng chỉ
`confettiAllowed` trong cùng file gọi; test cũng không gọi trực tiếp. Không phải
lỗi (đã có test `confettiAllowed` bảo vệ hành vi), chỉ là export thừa 1 dòng.

⇒ **Không có chỗ nào ngoài 2 file của lane F1 cần sửa.** Đề nghị lane F1 tự quyết
có bỏ `prefersReducedMotion` ra khỏi `export` không (1 dòng, không ảnh hưởng test).

### #8 — Test M6b còn assert text thay vì hành vi

Đã rà toàn bộ test M6b (`RoadmapMap` · `RestoredFeatures` · `RoadmapHome` ·
`Bookmarks` · `MapCanvas`). Kết quả:

| Kiểu assert | Còn lại | Đánh giá |
|---|---|---|
| Assert **text thay vì hành vi** | **0** trong `RoadmapMap.test.ts` và `MapCanvas.test.ts` | sạch sau lane này |
| Assert text ở `RestoredFeatures.test.ts` | 5 chỗ | **hợp lệ** — đều là *contract nội dung* đã đóng băng: `+36` (số thẻ nạp), message lỗi tiếng Việt nguyên văn (gate M4), `'Đang lọc theo thẻ 42'`, `'Chưa lọc'`, `'Xong 1 vòng'`. Không cái nào là hành vi có thể kiểm bằng `href`/DOM |
| Assert text ở `Bookmarks.test.ts` | 6 chỗ | **hợp lệ** — `'không có link'` + tên tài liệu là *quyết định hiển thị* được nêu ở `m6b.md` §5 ("link không có URL vẫn hiện tên + badge"); `'Chữ ab'`/`'Chữ abc'` là kết luận của bộ lọc |
| Assert text ở `RoadmapHome.test.ts` | 3 chỗ | **hợp lệ** — `'Chưa có path nào'` là empty-state; 2 chỗ `'Xoá'` là assert **nút tồn tại / không tồn tại**, tức chính là hành vi |

**1 chỗ đã sửa:** `test_top_errors_row_links_to_review_only_when_card_id_exists`
— chuyển từ `row.text()).toContain('Ôn thẻ này')` sang assert `href`, và thêm
test đóng vòng điều hướng.

**1 chỗ phát hiện thêm, chưa sửa (ngoài phạm vi, báo cáo cho M7):**
`RoadmapHome.test.ts:114` `test_path_title_links_to_the_map` dựng router thật
riêng chỉ để lấy `href` — việc này **nay không còn cần**, vì `mountScreen` đã
render `href`. Có thể gộp về `mountScreen` cho gọn (không sửa ở đây: thuộc
`RoadmapHome`, ngoài 4 finding + 4 việc dọn của lane này).

---

## 8. Nợ chuyển M7

1. **`Query.card(id: ID!)`** — làm `fetchCardById` còn 1 dòng `runQuery` và bỏ
   được vòng quét `Decks` → `cards(deckId)`. Cùng bounded context `srs`.
2. **`enum Terrain`** (lane F1 đang sửa) — sau khi F1 xong, `terrain` mới phân
   biệt được giữa 2 chặng. **Test F4 cố ý dùng 2 chặng CÙNG `PLAIN`** nên vẫn
   bảo vệ đúng sau khi F1 sửa xong; đừng "làm cho test xanh hơn" bằng cách đổi
   fixture.
3. **`prefersReducedMotion` export thừa** (`viewPrefs.ts`) — 1 dòng, cosmetic.
4. **`RoadmapHome.test.ts:114`** dựng router thật thủ công — gộp về `mountScreen`
   được vì `harness.ts` nay render `href`.
5. `enum Terrain` ở GraphQL (`m6b.md` §7) và các nợ M6b §12 **không đổi**.
