# ROADMAP MAP — Ý tưởng biến roadmap thành bản đồ game

> Bổ sung 2026-09-28 sau khi user muốn roadmap nhìn như Candy Crush: milestone = màn, màn
> trong 1 phần, mỗi phần 1 loại địa hình, vuốt dưới→trên hoặc trái→phải, user tự define.

## 1. Câu trả lời công nghệ

| Công nghệ | Chọn? | Lý do |
|---|---|---|
| **SVG** | ✅ Xương sống bản đồ | Hit-test dễ (`<circle>` + event), responsive, **test được trong vitest** (assert số node + toạ độ), ~15KB, không cần WebGL, chữ 2D luôn dễ đọc |
| **Canvas 2D** | ✅ Chỉ hiệu ứng | Confetti khi xong màn, bụi khi vuốt. Tắt khi `prefers-reduced-motion` |
| **Three.js** | ❌ | ~600KB, cần WebGL, tốn pin mobile, **vitest không dựng được WebGL context** → phải test bằng mắt. Chữ 3D khó đọc trong app text-heavy. 90% look mong muốn cho bằng SVG filter + gradient + CSS transform |
| **PixiJS** | ⏳ Dự phòng | Nếu sau này cần 2D engine thật sự (particle nặng, nhiều sprite). Nhẹ hơn Three.js nhiều. Chưa cần |
| **Canva (design tool)** | ✅ Offline | Tự design 4–6 tile địa hình, xuất SVG, ship vào `web/src/assets/terrain/`. Asset tĩnh, **không có runtime dependency** |

Lựa chọn sẵn có terrain (SVG, self-made): `meadow` (đồng cỏ), `desert` (sa mạc), `snow` (tuyết),
`volcano` (núi lửa), `ocean` (biển), `city` (thành phố).

## 2. Ánh xạ dữ liệu — KHÔNG cần bảng mới, chỉ thêm cột

Bản chất Candy Crush đã trùng khớp cấu trúc sẵn có:

```
stage  (G0…G4)  =  1 BẢN ĐỒ        ← thêm terrain + direction
  └─ topic        =  1 MÀN (level)  ← thêm map_x, map_y (NULL = auto-layout)
  └─ milestone    =  SAO / boss     ← dùng nguyên, thêm role nếu cần
```

Cột cần thêm (delta nhỏ, chưa đụng gì đã xong):

| Bảng | Cột | Kiểu | Ý nghĩa |
|---|---|---|---|
| `roadmap_stages` | `terrain` | `TEXT NOT NULL DEFAULT 'meadow' CHECK (terrain IN ('meadow','desert','snow','volcano','ocean','city'))` | Loại địa hình của bản đồ |
| `roadmap_stages` | `direction` | `TEXT NOT NULL DEFAULT 'up' CHECK (direction IN ('up','right'))` | `up` = vuốt dưới→trên · `right` = vuốt trái→phải |
| `roadmap_topics` | `map_x` | `REAL NULL` | NULL = tự layout. Set tay để đặt node tự do |
| `roadmap_topics` | `map_y` | `REAL NULL` | như trên |

`position` (đã có) giữ vai trò **thứ tự màn** (level 1, 2, 3…). `status` (đã có) là trạng thái
khoá / đã mở / đã xong. `duration_weeks` (đã có) hiện ở góc node dưới dạng "3 tuần".

**Layout tự động theo terrain + direction** — không bắt user kéo thả:
- `direction='up'`: node *i* đặt trên đường cong hình sin đi từ dưới lên, x dao động nhẹ để
  trông như đường núi.
- `direction='right'`: cùng đường cong nhưng xoát ngang trái→phải, y dao động.
- Hệ số dao động khác nhau theo terrain (`desert` phẳng hơn, `volcano` gấp thêm).
- Toạ độ tính từ `position` + `path(t)` → **deterministic, test được**: cùng `position` luôn
  ra cùng toạ độ, không phụ thuộc render.

## 3. Trạng thái màn (mở / khoá) — suy ra, không lưu

| Trạng thái node | Điều kiện | Hiển thị |
|---|---|---|
| `done` | `status='done'` | Đã qua, có ngôi sao, có thể chơi lại |
| `current` | màn trước `done` và màn này chưa `done` | Node sáng, có vòng hào động, có nút "vào" |
| `locked` | màn trước chưa `done` | Node mờ, icon ổ khoá, **không** bấm được |

Không lưu `current` vào DB → không phải xử lý lệch trạng thái. Màn `skipped` coi như `done`
(không khoá màn sau).

## 4. Cử chỉ — ĐÃ CHỐT 2026-09-28

Ba cử chỉ tách bạch theo vị trí, không kích hoạt nhầm:

| Cử chỉ | Hành vi | Ở đâu | Ràng buộc |
|---|---|---|---|
| **Vuốt dọc / ngang** | Cuộn bản đồ theo `direction` của map | Toàn bản đồ | Không được nuốt cử chỉ khác |
| **Chạm node** | Mở panel chi tiết màn | Trên node | Chỉ node `current` hoặc `done` |
| **Vuốt lên trong panel** | Đánh dấu Xong + confetti | **Chỉ trong panel** | `pointerdown→pointerup` delta > 80px, cùng trục, 1 lần |

Luôn có nút bấm **"Xong"** song song với vuốt — swipe là đường tắt, nút là đường chính
(điều khiển bằng bàn phím cũng dùng nút này). Cảm giác chơi game vẫn còn, nhưng không bao giờ
mất công vì lỡ tay.

## 5. Ranh giới kỹ thuật

```
web/src/roadmap/map/
  terrain.ts        # 6 terrain: màu, path function, rung/hill amplitude
  layout.ts         # pure: (position, count, direction, terrain) → {x, y}[]
                    # ← test thuần, không DOM, assert toạ độ
  levelNode.ts      # trạng thái: (topics) → {done, current, locked}[]
  MapCanvas.vue     # SVG: terrain layer + path + nodes
  LevelPanel.vue    # panel chi tiết + vuốt đánh dấu
  ConfettiCanvas.vue# Canvas 2D, tắt khi prefers-reduced-motion
  swipe.ts          # pointer gesture → direction, threshold 80px
```

`layout.ts` + `levelNode.ts` là **pure function** — chiếm phần lớn test value, test được không
cần DOM. `MapCanvas.vue` chỉ render, assert được số `<circle>` và toạ độ trong DOM test.

## 6. Địa hình + hiệu ứng — ĐÃ CHỐT 2026-09-28

**6 loại địa hình, gán sẵn** (không cho user tự thêm ở v1 — giữ whitelist hẹp để layout
deterministic và test được): `meadow`, `desert`, `snow`, `volcano`, `ocean`, `city`.

Gán cho 10 stage seed sẵn có:
- Trung (5 stage, độ khó tăng dần): `meadow` → `meadow` → `desert` → `snow` → `volcano`
- Anh (5 stage): `meadow` → `ocean` → `city` → `snow` → `volcano`

**Nhớ vị trí đang xem** — `localStorage` khoá `roadmap:<slug>:<view>` → toạ độ scroll hiện tại,
khôi phục khi mở lại. Với `direction='up'` thì khoá theo trục Y, `'right'` thì theo trục X.

**Hiệu ứng** — confetti Canvas 2D khi hoàn thành màn. Bắt buộc:
- Tắt hoàn toàn khi `prefers-reduced-motion: reduce`.
- Có toggle trong Cài đặt để tắt thủ công.
- Không nhạc/âm thanh mặc định (chưa chốt) — để sau, không block.

## 7. Việc cần làm

1. Migration `00004_roadmap_map.sql` — 4 cột: `roadmap_stages.terrain` (CHECK 6 giá trị),
   `roadmap_stages.direction` (CHECK `up|right`), `roadmap_topics.map_x` REAL NULL,
   `roadmap_topics.map_y` REAL NULL + index nhỏ.
2. Backend: stage PATCH nhận `terrain`/`direction`; topic PATCH nhận `map_x`/`map_y`;
   validate bằng whitelist trả lỗi tiếng Việt; `GET /api/roadmap/paths/{slug}` trả kèm layout
   server-side (một nguồn sự thật cho web lẫn app sau này).
3. Seed: gán `terrain`/`direction` cho 10 stage theo bảng §6.
4. Frontend: `terrain.ts`, `layout.ts`, `levelNode.ts`, `swipe.ts` + test thuần; `MapCanvas.vue`
   (SVG), `LevelPanel.vue`, `ConfettiCanvas.vue`.
5. Asset terrain: 6 file SVG tự vẽ, đặt `web/src/assets/terrain/`. Nếu bạn muốn đẹp hơn thì
   dùng Canva thiết kế rồi export SVG thay vào — cùng tên file, không đổi code.
6. **Giữ list view** dạng roadmap.sh hiện có làm chế độ dự phòng (`?view=list`) — bản đồ game
   không thay thế được list khi cần quét nhanh toàn bộ nội dung. Mặc định `?view=map` khi user
   lần đầu vào, nhớ lựa chọn ở localStorage.

## 8. Câu hỏi đã giải quyết

| Câu hỏi | Chốt |
|---|---|
| Vuốt = cuộn hay = đánh dấu? | Cả 3, tách vị trí (§4) |
| Bao nhiêu loại địa hình? | 6, gán sẵn, whitelist hẹp (§6) |
| Nhớ vị trí + âm thanh? | Nhớ vị trí qua localStorage; confetti có toggle; **chưa làm nhạc** (§6) |
| Three.js / Canva? | SVG là xương sống, Canvas 2D chỉ hiệu ứng, **không Three.js**; Canva dùng offline làm asset (§1) |
