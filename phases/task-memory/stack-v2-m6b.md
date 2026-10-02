# STACK-V2 — M6b (designer: bản đồ game + kho link + 3 tính năng phục hồi)

> Hoàn tất 2026-09-29. M6b là phần **UI** của M6; M6a (`fix-27`) đã làm server.
> **Không sửa 1 dòng `api/**`** — 1 phát hiện về schema ghi ở §7, đề xuất ở đó.
>
> M5: `stack-v2-m5.md` + `m5-remediation.md`. M6a: `stack-v2-m6a.md`. File này
> **bổ sung** cho chúng.

## 0. Tóm tắt 1 dòng

`vitest` **170 → 342 test** (26 → 38 file), `vue-tsc` sạch, `vite build` OK,
`grep react` 0 hit · 0 file `.tsx` · **mutation-check 19/19 FAIL thật**.
2 màn mới + 1 màn chi tiết có tham số, 8 component mới, 6 shadcn-vue primitive,
3 tính năng mất lúc port đã phục hồi và có test chặn tái mất.

---

## 1. Hệ thị giác — "Atlas Ink"

Đây là phase đầu tiên M6 được phép phát minh diện mạo (M5 ghi rõ: màu lúc đó
lấy nguyên văn từ app v1, đổi là việc của M6). Lựa chọn cốt lõi: **phần đọc
trên giấy ấm, phần bản đồ đảo sang mực sâu** — cùng một chất liệu lặp ở đầu
bảng, panel và con trỏ nên đọc ra là một sản phẩm, không phải "14 màn cũ + 1 màn
mới".

### 1.1 Bảng chất liệu (ghi để M7 không phá)

| Vai trò | Token | Giá trị |
|---|---|---|
| Giấy | `--color-paper` | `#f6f2e9` |
| Mặt thẻ | `--color-surface` | `#fffdf8` |
| Mực | `--color-ink` | `#182031` |
| Mực phụ | `--color-muted` | `#6b6a63` |
| Nhấn duy nhất | `--color-ember` | `#c8502a` |
| Xong | `--color-jade` | `#2e6b57` |
| Hổ phách | `--color-amber` | `#d99b23` |
| Node khoá | `--color-locked` | `#b4ac99` |
| Mặt bản đồ | `--color-atlas` | `#1b2337` |

`--primary` (shadcn) **trùng Ember** — nhấn là 1 màu duy nhất trong sản phẩm,
nút chính và node `CURRENT` cùng họ. `--accent` trùng Jade nên badge "Xong" và
node `DONE` cũng cùng họ. Đổi 1 dòng trong `:root` là đổi cả hệ.

### 1.2 Lớp token HSL (nợ M5 đã ghi — đã đóng)

`styles/app.css` nay có **3 tầng**:

1. `@theme` — token vai trò (ink/muted/line/ok/bad/paper/ember/…) + thang
   spacing `--spacing: 0.25rem` + `--radius-card: 10px` + 2 shadow. 14 màn cũ dùng
   đúng tên này nên **không phải sửa màn nào**.
2. `:root` — lớp shadcn-vue, giá trị là **3 số HSL trần** (`42 33% 94%`) đúng
   như shadcn quy định.
3. `@theme inline` — nối tầng 2 vào engine Tailwind bằng `hsl(var(--x) / …)`.

Muốn đổi palette: sửa tầng 2. Muốn đổi tên vai trò: sửa tầng 1.

### 1.3 Chữ — KHÔNG nạp font từ mạng

App chạy 1 binary offline; Google Fonts là 1 request chết mỗi lần mở app và 1
phụ thuộc mạng. Nên 3 thang là **stack hệ thống có sẵn**:

| Token | Stack | Dùng cho |
|---|---|---|
| `--font-display` | Iowan Old Style / Palatino / P052 / Georgia / serif | `h1`–`h3`, số trên node |
| `--font-sans` | Avenir Next / Segoe UI / Roboto / Noto Sans | giao diện |
| `--font-mono` | SF Mono / JetBrains Mono / IBM Plex Mono | class `.tabular` — mọi số |

`.tabular` = `font-variant-numeric: tabular-nums`; bọc mọi số liệu (tiến độ, id,
toạ độ, số câu) để chúng không nhảy cột khi đổi chữ số.

### 1.4 Thang nhịp

Tailwind mặc định (`--spacing: 0.25rem`) — 4/8/12/16/24/32/48. **Mọi khoảng cách
ở M6 là 1 bậc trong thang**, không có giá trị lẻ. Cột hiển thị dùng 13px (nhãn
phụ), 14px (nội dung), 18px (tiêu đề trong panel), 20/24px (tiêu đề màn).

### 1.5 Hệ phân cấp node (4 tầng, khác nhau cả hình lẫn kích thước)

| Tầng | Hình | Bán kính (viewBox) | Khi nào |
|---|---|---|---|
| Node bắt buộc | tròn đặc, viền 5 | 34 | mặc định |
| Node tham khảo | tròn **nét đứt**, viền 3 | 24 | `isOptional` |
| Vòng hào động | vòng tròn, `node-breathe` | r+14 | **chỉ** `CURRENT` |
| Mốc chặng | hình thoi 45° | 15 | `milestones` |

`LEVEL_STYLE` trong `map/geometry.ts` là **bảng duy nhất** ánh xạ
`DONE/CURRENT/LOCKED` → màu + có halo hay không. Test
`test_only_current_node_has_animated_halo` chặn việc thêm halo cho node khoá
(giật mà không mở được gì).

---

## 2. Chuyển động — ít nhưng đúng chỗ

| Chuyển động | Khi nào | Vì sao |
|---|---|---|
| `map-path` (vẽ đường 1.1s) | mở bản đồ | 1 chuyển động duy nhất đủ tạo cảm giác "đang đi tiếp" thay vì 1 bảng toạ độ rơi xuống |
| `node-breathe` (2.6s) | node `CURRENT` | chỉ 1 node sáng; 6 node cùng thở thì thành màu nhiễu |
| `stagger` (26ms/dòng, trần 10) | list view | 51 topic; không trần thì vòng dài phải chờ 1.3s |
| `rise-in` (0.42s) | panel, form | vào/ra có hướng |
| Confetti (Canvas 2D) | đánh dấu Xong | lớp vẽ pixel DUY NHẤT |

Tất cả bị tắt bằng 1 khối `@media (prefers-reduced-motion: reduce)` ở cuối
`app.css` — **1 chỗ**, không phải rải `if` trong từng component.

Lực confetti là hàm thuần trong `map/confetti.ts` (`burst`/`step`) nên kiểm
được số mảnh, hướng văng, màu mà không mở trình duyệt.

---

## 3. Bản đồ — ranh giới kỹ thuật

### 3.1 Chia tầng (đúng như ROADMAP-MAP-IDEA §5, có thêm 1 file)

```
src/roadmap/
  api.ts             client đầy đủ 21 use case (mọi mutation gọi afterMutation)
  PathForm · StageForm · TopicForm · ResourceForm · MilestoneForm
  map/
    terrain.ts       6 địa hình: màu + hình nền tất định (seeded)
    geometry.ts      fitFrame · trailPath · LEVEL_STYLE · stageProgress  (thuần)
    swipe.ts         dominantAxis · isSwipe                            (thuần)
    confetti.ts      burst · step                                      (thuần)
    viewPrefs.ts     localStorage + prefersReducedMotion
    MapCanvas.vue    SVG
    LevelPanel.vue   panel + vuốt đánh dấu
    ConfettiCanvas.vue
    ProgressRing.vue
```

**Toạ độ KHÔNG tính lại ở client.** `Topic.point` đến từ
`domain/roadmap.ComputeLayout`. `fitFrame` chỉ **bọc** toạ độ đó vào khung vừa
nội dung (chặng 5 node không nằm giữa trang trắng), `trailPath` chỉ **nối**.
Tính lại là 2 bản luật layout trôi khỏi nhau.

**Kích thước canvas dùng hằng số, không đo DOM.** `CANVAS_WIDTH = 720` (map dọc)
· `CANVAS_HEIGHT = 520` (map ngang), `Math.min(1, …)` để map hẹp không bị phóng
to. Cùng lý do các hằng số layout ở M2 đóng băng: `happy-dom` trả 0 cho mọi
hình học, đo DOM thì test không có gì để kiểm.

### 3.2 Ba cử chỉ — cách tách VỊ TRÍ, không tách theo điều kiện

| Cử chỉ | Gắn ở đâu | Hành vi | Ràng buộc |
|---|---|---|---|
| Vuốt | `MapCanvas` khung scroll | cuộn theo `direction` | trục phụ **khoá**; vuốt chéo bị bỏ qua |
| Chạm | `<circle>` của node | mở panel | `LOCKED` không bấm được |
| Vuốt lên | `LevelPanel` `<aside>` | Xong + confetti | delta ≥ 80px, trục **dọc**, chiều **lên**, 1 lần/mở panel |

Nút **"Đánh dấu Xong"** luôn có — đường chính, dùng được bằng bàn phím. Vuốt là
đường tắt. Tắt hiệu ứng thì nút vẫn hoạt động (test riêng).

`isSwipe(…, 'y')` **đã bao gồm cả luật trục** (trục ngang phải nhỏ hơn trục dọc)
lẫn ngưỡng 80px — không thêm `dominantAxis` ở `LevelPanel` vì lặp lại đúng nửa
đầu của cùng phép so (mutation-check đã chứng minh nhánh đó chết).

### 3.3 Sắp lại cây khi nhận — lưới an toàn chốt F8

`fetchPath()` gọi `sortPathTree()` sắp `(position, id)` cho cả 4 tầng. Không
phải thừa: M4 từng có F8 (`StageTreeByPathIDs` gom bằng `range` trên Go map ⇒
thứ tự ngẫu nhiên mỗi request ⇒ node bản đồ nhảy chỗ, `LevelState` áp vào sai
node, **không lỗi nào báo**). Test
`test_shuffled_server_order_is_normalised_by_the_client` mô phỏng đúng dữ liệu
đó.

### 3.4 Nhớ vị trí & chế độ xem

- `roadmap:view` = `map | list`; chưa chọn ⇒ `map`. `?view=list` trong URL **thắng**
  lựa chọn đã nhớ (để gửi link cho người khác, và để test mở thẳng 1 chế độ).
- `roadmap:<slug>:<view>` = `{ stageId, offset }`. Ghi kèm `stageId` vì mỗi
  chặng là 1 bản đồ riêng — chặng nào đang mở cũng là thứ cần nhớ.
- `offset` là **trục chính** của map: `up` = Y, `right` = X. Khoá ở 0.
- `roadmap:confetti` = `0|1`, mặc định bật; Cài đặt có toggle.
- `zh-pinyin:grade-mode` = `card|round`, mặc định `card`.

---

## 4. 3 tính năng phục hồi (user xác nhận bản cũ ĐÃ CÓ)

Cả 3 đều là loại lỗi M5 đã ghi: **hàm có, test có, không màn nào gọi** — không
lỗi biên dịch, không cảnh báo.

1. **Nút nạp HSK ở màn Pinyin** — `postZhImport` có từ M5, không màn nào gọi.
   Nay có nút + ô nhập level + trạng thái "Đang nạp…" + báo số thẻ nạp được +
   lỗi hiện **nguyên văn** message server. Câu "nạp HSK1 trước" giờ trỏ được
   tới nút thật.
2. **Bộ lọc lỗi theo thẻ ở Sổ lỗi** — `ErrorBook` luôn gọi
   `fetchErrors(undefined, 50)`. Nay có: dropdown thẻ theo deck + ô nhập id tự
   do. Ô tự do **không phải thừa**: lỗi gắn thẻ có thể thuộc deck đã xoá, không
   có deck nào trong danh sách chứa nó. Kèm chuyển `topErrors` →
   `insightTopErrors` để dòng từ sai **nhảy được** tới thẻ (`cardId` null thì
   dòng tĩnh + badge, không render nút chết).
3. **Chấm điểm cả vòng drill** — `roundGrade`/`remainingItems` chỉ được test dùng.
   Nay có 2 chế độ, **mặc định giữ nguyên "từng thẻ"** (đúng nghiệp vụ SRS ôn
   riêng lẻ: 1 lần sai kéo thẻ về hàng đợi ngay). "Cả vòng" là lựa chọn thứ hai,
   nhớ ở `localStorage`. Chỉ ghi cho thẻ **đã trả lời** (`remainingItems` là nơi
   biết thẻ nào chưa làm).

---

## 5. `/bookmarks`

Kho link độc lập, KHÔNG nằm trong cây roadmap (M6a §5.6).

- ⚠️ `BookmarkStatus` = `TO_READ/READING/DONE/ARCHIVED` — **KHÁC** `Status` của
  node. Gửi nhầm là 400. Test `test_sends_bookmark_status_not_roadmap_status`.
- Lọc `status` + `tag` gửi thẳng xuống server. Ô tag là **dropdown lấy từ 1 lần
  đọc không lọc**, không phải ô gõ: gõ "a" vào ô tự do sẽ lọc theo từng ký tự
  và nhảy kết quả liên tục.
- Case viền `a` vs `ab` chạy ở **cả 3 tầng**: `domain.TagsContain` (M6a) · SQL
  thật (M6a) · UI (M6b, `test_filtering_by_a_excludes_ab_and_abc` — stub server
  mô phỏng đúng luật `position(',' || ? || ',')`). Mutation "đổi sang so chuỗi
  con ở client" làm test **đỏ**.
- Link không có URL vẫn hiện tên + badge "không có link" (không giấu) — người
  dùng còn cần biết có mục đó. `url` rỗng ≠ `url` null: xoá link dùng
  `clearUrl`, có checkbox riêng chỉ bật khi đang sửa.
- `normalizeTags()` chuẩn hoá **trước khi gửi** vì server đếm độ dài 40 ký
  tự TRƯỚC khi chuẩn hoá — `" HSK3 "` sẽ bị đếm 6 ký tự.

---

## 6. File mới / sửa

**Màn (3):** `routes/RoadmapHome.vue` · `routes/RoadmapMap.vue` (`/roadmap/:slug`) ·
`routes/Bookmarks.vue`

**Component (13):** `components/ui/{Button,Input,Textarea,Select,Label,Field,Dialog,Badge}.vue`
(8 primitive shadcn-vue) · `roadmap/map/{MapCanvas,LevelPanel,ConfettiCanvas,ProgressRing}.vue` ·
`roadmap/{PathForm,StageForm,TopicForm,ResourceForm,MilestoneForm}.vue` ·
`bookmarks/BookmarkForm.vue`

**Logic (7):** `roadmap/api.ts` · `bookmarks/api.ts` · `player/insight.ts` ·
`roadmap/map/{terrain,geometry,swipe,confetti,viewPrefs}.ts` · `lib/cn.ts`

**Sửa:** `graphql/operations.ts` (+21 operation roadmap, +5 bookmark, +1
insight) · `router.ts` (+2 route, +`matchDetailRoute`) · `routes/ZhPinyin.vue` ·
`routes/ErrorBook.vue` · `routes/CaiDat.vue` (toggle confetti) ·
`styles/app.css` (hệ 3 tầng) · `lib/mutationInvalidation.test.ts` (+2 module,
+21 mutation) · `package.json` (+4 lib)

**Ngoài `web/src/` (2, đều bắt buộc):**
- `web/package.json`: `reka-ui@2.10.5` · `class-variance-authority` · `clsx` ·
  `tailwind-merge`. Không có codegen, không có registry shadcn — component viết
  tay đúng hợp đồng shadcn.
- `web/mutation-check-m6b.mjs`: chạy 19 mutation, backup trước / khôi phục sau
  (bản shell đầu tiên hỏng file vì chuỗi rỗng được chèn vào mọi ký tự — đã
  ghi lại vì sao bản này copy file thay vì thay chuỗi).

---

## 7. Phát hiện về SERVER (KHÔNG sửa — ngoài phạm vi ghi)

### 🔴 `enum Terrain` trong GraphQL KHÔNG khớp whitelist ở DB

| Nơi | Giá trị |
|---|---|
| `migrations/00004_roadmap_map.sql` (CHECK) | `meadow desert snow volcano ocean city` |
| `domain/roadmap/map_value_object.go` | `meadow desert snow volcano ocean city` |
| **`api/graph/schema/common.graphqls`** | **`PLAIN FOREST HILL MOUNTAIN WATER DESERT`** |
| `convert.go:terrainOf` | map 6 giá trị DB **→ 6 giá trị enum cũ** |

Hệ quả đo được, không phải suy đoán: `terrainOf("meadow")` rơi về `default`
⇒ `PLAIN`. Cùng vậy `snow`/`volcano`/`ocean`/`city` đều ra `PLAIN`. **Server
đang trả `PLAIN` cho 5/6 địa hình**, và `terrainText()` chỉ ghi được `DESERT`
(các giá trị khác ghi `""` rồi validate gán lại default). Tức bộ chọn địa hình
trong UI **không lưu được 5/6 lựa chọn**.

M6b **không vá** (ngoài phạm vi ghi). Thay vào đó `roadmap/map/terrain.ts` giữ
1 bảng 6 dòng `PLAIN/FOREST/HILL/MOUNTAIN/WATER/DESERT → 6 chất liệu hình ảnh`,
nên UI dùng được ngay hôm nay; **khi schema sửa lại về whitelist M2 thì chỉ sửa
cột giá trị trong bảng đó, 6 dòng, không đụng chỗ nào khác**. Việc này nên vào
M6a-remediation hoặc M7 (cùng bounded context `roadmap`).

### Nợ khác (nhỏ)

- `EnStress.vue` vẫn dùng ô nhập `deck id` tự do thay `DeckSelect` (M5 đã ghi,
  M6 không chuẩn hoá vì ngoài phạm vi 3 màn này).
- `mapX`/`mapY` chỉ sửa được qua form, **không có drag-drop**. Cần kéo thì phải
  làm thêm; `clearMap` đã có nút tương ứng trong `TopicForm`.
- `Query.bookmarks` không có `limit`/phân trang (M6a ghi). Danh sách là dữ liệu
  cá nhân nên chưa cần.

---

## 8. Test — 170 → 342

| File | Số | Vì sao đáng có |
|---|---|---|
| `roadmap/map/swipe.test.ts` | 10 | ranh giới 80px + trục + ngưỡng nhiễu 12px |
| `roadmap/map/geometry.test.ts` | 12 | `fitFrame` lùi đủ 4 cạnh · khung rỗng không 0×0 · `trailPath` phản ánh thứ tự server · `LEVEL_STYLE` halo · `stageProgress` bỏ node tham khảo khỏi mẫu số |
| `roadmap/map/terrain.test.ts` | 13 | 6 terrain có màu riêng + 6 nhãn tiếng Việt · nền **tất định** (không `Math.random`) · terrain lạ rơi về mặc định |
| `roadmap/map/viewPrefs.test.ts` | 11 | chế độ xem · vị trí scroll (JSON hỏng → null) · **confetti tắt khi `prefers-reduced-motion` DÙ toggle bật** |
| `roadmap/map/confetti.test.ts` | 8 | mảnh văng **lên** chứ không rơi · trọng lực · hết đời thì loại · không mutate input |
| `roadmap/map/MapCanvas.test.ts` | 24 | **3 cử chỉ tách vị trí** · trục phụ khoá (kể cả vuốt chéo) · nút Xong không cần vuốt · `/review` ẩn khi `deckId` null · tài liệu không có link vẫn hiện |
| `roadmap/api.test.ts` | 8 | **sắp `(position, id)` khi nhận cây** · tie-break `"9" < "10" < "100"` · 1 request cho cả cây |
| `bookmarks/api.test.ts` | 10 | `BookmarkStatus` ≠ `Status` · `tag` null tường minh · chuẩn hoá tag · `clearUrl` · `tags: []` |
| `routes/RestoredFeatures.test.ts` | 17 | **3 tính năng phục hồi** (nút HSK + lọc thẻ + 2 chế độ chấm) |
| `routes/RoadmapMap.test.ts` | 15 | bản đồ mặc định · **list view dự phòng** · **confetti không chạy khi giảm chuyển động** · node khoá không mở panel |
| `routes/RoadmapHome.test.ts` | 7 | `isBuiltin` không có nút xoá (nút chết) · vòng tiến độ đọc `summary.percent` |
| `routes/Bookmarks.test.ts` | 11 | lọc status · lọc tag **viền `a`/`ab`/`abc`** · 2 điều kiện AND · xoá có xác nhận |
| `lib/mutationInvalidation.test.ts` | +21 | 21 mutation mới · `roadmap/api` + `bookmarks/api` vào danh sách chặn |

**3 bẫy gặp khi viết test (đáng nhớ)**

1. **Nút chặn `global.stubs` không bắt được component CHƯA đăng ký.**
   `RouterLink` không có router thì render thành `<routerlink>` rỗng, và test
   `href` là undefined — tưởng link sai. Phải `global.components` hoặc router thật.
2. **urql gửi QUERY bằng GET, gói biến trong 1 tham số `?variables={…}`.** Mock
   đọc `?tag=x&status=y` sẽ luôn thấy `null` ⇒ test "lọc" xanh với dữ liệu sai.
3. **`Event` không có `clientX`.** Phải `new MouseEvent(name, {clientX})` rồi
   định nghĩa thêm `pointerId`/`pointerType` — nếu không, `dx/dy` = NaN và mọi
   test cử chỉ xanh vì lý do vô nghĩa.

### 8.1 Mutation-check 19/19 FAIL thật

`node mutation-check-m6b.mjs` — sửa ngược, test **phải đỏ**, khôi phục. Đáng kể:

| Mutation | Kết quả |
|---|---|
| `fetchPath` bỏ `sortPathTree` | đỏ — F8 của M4 quay lại |
| `LevelPanel` bỏ luật trục | đỏ — vuốt ngang ăn mất thao tác hoàn thành |
| `MapCanvas` bỏ khoá trục phụ | đỏ — vuốt chéo làm bản đồ trượt |
| `confettiAllowed` bỏ `prefers-reduced-motion` | đỏ |
| ẩn nút "Đánh dấu Xong" | đỏ — vuốt thành đường duy nhất, mất bàn phím |
| bỏ `v-if="deckId"` ở nút `/review` | đỏ — nút chết |
| bỏ chế độ danh sách (`?view=list`) | đỏ |
| lọc tag thành `includes` ở client | đỏ — `a` khớp `ab` |
| nút nạp HSK không gọi gì | đỏ |
| `ErrorBook` bỏ `cardId` | đỏ |
| chế độ cả vòng ghi từng câu | đỏ |
| node `locked` bấm được | đỏ |
| `createBookmark` quên `afterMutation()` | đỏ |
| `setTopicStatus` quên `afterMutation()` | đỏ — đánh dấu Xong không lên UI |
| `fitFrame` trả `maxX/maxY` thô | đỏ — cắt mất nhãn node cuối |
| nền terrain dùng `Date.now()` | đỏ — nhấp nháy mỗi render |
| bỏ khoá scroll ở 0 | đỏ — kéo quá đầu trang nhảy vị trí |
| path seed có nút Xoá | đỏ — nút chết |
| vòng tiến độ tự chia lại | đỏ |

**1 mutation lần đầu XANH** (LevelPanel "nhận cả vuốt ngang") đã dẫn tới 1
phát hiện thật: `isSwipe(…, 'y')` **đã bao gồm sẵn** luật trục, nên nhánh
`dominantAxis(...) !== 'y'` là **code chết**. Đã xoá, thay bằng mutation thật
sự (bỏ luật trục khỏi `isSwipe`) — nay đỏ. Đây là lần thứ 2 trong M6 mà
mutation-check dạy được điều code review bình thường không thấy.

---

## 9. shadcn-vue — lựa chọn và giới hạn

Dùng `reka-ui` 2.10.5 (lõi của shadcn-vue) cho **Dialog**. **Cố ý KHÔNG dùng**
`Select` của reka-ui: 9 kind + 6 terrain + 4 trạng thái là danh sách ngắn, cần
bàn phím và đọc bằng trình duyệt ngay; `<select>` gốc có sẵn cả hai, chạy được
trong `happy-dom` (reka-ui cần `ResizeObserver` + vị trí đo) nên test form không
phải dựng giả môi trường.

⚠️ `ui/Select.vue` **phải khai `modelValue` + emit `update:modelValue`**.
Component render `<select>` mà không khai v-model thì `v-model` trên component
rơi vào chỗ không — giá trị không bao giờ về, **mọi bộ lọc hiện đúng rồi không
bao giờ lọc**. Đã trải qua đúng lỗi đó (7 test đỏ) trước khi sửa.

---

## 10. Verify

| Lệnh | Kết quả |
|---|---|
| `pnpm vitest run` | **342 PASS / 0 FAIL** (38 file; 170 → 342) |
| `pnpm typecheck` | sạch |
| `pnpm build` | OK (chunk lớn nhất `queryClient` 143KB, `Button` 29KB, `RoadmapMap` 38KB) |
| `node mutation-check-m6b.mjs` | **19/19 đỏ đúng** |
| `grep -rn "react" src package.json` | **0 hit** (kể cả chữ "reactive" trong comment — M5 chặn vậy) |
| `find src -name '*.tsx'` | **0 file** |
| `grep -rn api/**` | 1 dòng `api/**` không sửa |

---

## 11. SKIPPED + lý do

| Việc | Lý do |
|---|---|
| Sửa `enum Terrain` ở schema | **ngoài phạm vi ghi** (`api/**`). Đã báo §7 + đã bọc 1 bảng 6 dòng để sửa sau chỉ tốn 6 dòng |
| Kéo thả node trên bản đồ | `mapX/mapY` sửa được qua form + có `clearMap`. Drag-drop là 1 phase riêng; làm vội sẽ sinh 2 nguồn sự thật cho vị trí node |
| Nhạc/âm thanh khi hoàn thành | ROADMAP-MAP-IDEA §6 để sau, không block |
| Server render `prefers-reduced-motion` | Không có SSR; `matchMedia` ở client là nguồn duy nhất |
| Chuẩn hoá form 12 màn cũ sang shadcn | Ngoài 3 màn trong phạm vi M6. M5 đã ghi nợ: 10/14 màn chưa có test DOM — chuẩn hoá hàng loạt **trước** khi có lưới an toàn là đổi UI không kiểm được |
| `limit`/phân trang cho `bookmarks` | M6a §8 đã ghi: kho link là dữ liệu cá nhân vài chục dòng |

## 12. Nợ chuyển M7

1. **`enum Terrain` ở GraphQL** (§7) — nên sửa cùng M7 hoặc mở remediation M6a.
2. `EnStress.vue` dùng ô nhập `deck id` tự do — chuẩn hoá khi đủ test DOM.
3. Kéo thả node bản đồ (nếu muốn) — phải dùng `mapPinned` làm điều kiện để không
   sinh 2 nguồn sự thật cho vị trí node.
