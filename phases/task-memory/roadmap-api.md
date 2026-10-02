# R1a — Roadmap API + schema + seed loader

> Phase: R1a (fixer) — XONG. Cổng review: R1c (oracle).
> Deepwork: `.slim/deepwork/roadmap-feature.md`.
> Ngày: 2026-09-28.

## Đã làm

1. **Schema** (`api/schema.sql`): thêm 5 bảng `roadmap_paths`, `roadmap_stages`,
   `roadmap_milestones`, `roadmap_topics`, `roadmap_resources` + index + 5 trigger
   `trg_roadmap_*_touch_updated`. Không đụng bảng cũ, không thêm bảng nào khác.
2. **API** (`api/roadmap.go`): `RegisterRoadmapRoutes` — 21 endpoint CRUD + status +
   progress (bảng endpoint ở dưới).
3. **Seed loader** (`api/roadmap_seed.go`): `//go:embed roadmap_seed` + parse mọi
   `*.json`, insert-only theo natural key, gọi 1 lần lúc boot sau `OpenDB`.
4. **Test** (`api/roadmap_test.go`): 10 test, tất cả xanh.

## Schema — 1 cột thêm so với spec

Spec chốt 5 bảng với cột `activities` **chưa** có trong danh sách cột của
`roadmap_topics`. Task R1a yêu cầu thêm và ghi rõ:

- `roadmap_topics.activities TEXT NOT NULL DEFAULT ''` — **JSON array string**
  (`json.Marshal([]string)`), rỗng thì lưu chuỗi rỗng. API đọc ra `[]string`
  (`activities` trong JSON response luôn là mảng, không null). Đọc lại chịu được
  cả dạng newline-joined (fallback) phòng row nhập tay.
- Ngoài ra mỗi bảng có `position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0)`
  và `duration_weeks ... CHECK (duration_weeks >= 0)`.
- `status` có `CHECK (status IN ('not_started','in_progress','done','skipped'))` ở
  cả `roadmap_stages` lẫn `roadmap_topics` (test xác nhận ghi thẳng SQL sai vẫn bị
  DB chặn).
- `roadmap_paths.slug` UNIQUE; `roadmap_stages` UNIQUE `(path_id, slug)`.
  Tombstone vẫn giữ slug → user xoá mềm rồi tạo lại cùng slug sẽ 409 (khác
  `cards` vốn hồi sinh tombstone — roadmap không cần, user tự đổi slug).
- `guid TEXT NOT NULL DEFAULT ''` + `CREATE UNIQUE INDEX ux_roadmap_*_guid` cho cả
  5 bảng, đồng bộ convention `decks`/`cards` (sync dựa vào đó).

## Endpoint (`RegisterRoadmapRoutes`, method-prefixed như D5)

| Method | Path | Ghi chú |
| --- | --- | --- |
| GET | `/api/roadmap/paths` | list path + progress (`stages`, `topics_total`, `topics_done`, `topics_in_progress`, `percent`) |
| POST | `/api/roadmap/paths` | 409 nếu slug trùng |
| GET | `/api/roadmap/paths/{slug}` | cây đầy đủ: stages > (milestones, topics > resources) |
| PATCH | `/api/roadmap/paths/{slug}` | title / overview / language |
| DELETE | `/api/roadmap/paths/{slug}` | xoá mềm lan xuống 4 tầng con |
| GET | `/api/roadmap/paths/{slug}/progress` | `{stages, topics_total, topics_done, topics_in_progress, percent}` |
| POST | `/api/roadmap/paths/{slug}/stages` | |
| PATCH / DELETE | `/api/roadmap/stages/{id}` | DELETE xoá mềm + con |
| PUT | `/api/roadmap/stages/{id}/status` | `{status, status_note}` |
| POST | `/api/roadmap/stages/{id}/milestones` | |
| PATCH / DELETE | `/api/roadmap/milestones/{id}` | |
| POST | `/api/roadmap/stages/{id}/topics` | nhận `activities: []string` |
| PATCH / DELETE | `/api/roadmap/topics/{id}` | DELETE xoá mềm + resources |
| PUT | `/api/roadmap/topics/{id}/status` | `{status, status_note}` |
| POST | `/api/roadmap/topics/{id}/resources` | `url` nil → lưu NULL |
| PATCH / DELETE | `/api/roadmap/resources/{id}` | PATCH nhận `"url": null` để xoá link |

Quy ước chung: mọi list/get lọc `deleted = 0`; DELETE là tombstone
(`deleted = 1`, trigger chạm `updated_at`); GET/PATCH/DELETE theo id → 404 nếu
đã xoá; mọi ghi trong 1 `sql.Tx`; 400 (message tiếng Việt) / 404 / 409.

## Validate

- `slug`: trim + lower, chỉ `[a-z0-9-]`, 1..64, không `-` đầu/cuối.
- `title`: bắt buộc không rỗng sau trim, ≤200 ký tự.
- `url`: `null`/rỗng → NULL; nếu có thì phải parse được, có host, scheme
  `http|https` (chặn `ftp:`, `javascript:`, `example.com` không scheme).
- `kind`: rỗng hoặc ∈ {video, article, tool, app, book, course, site, podcast, channel}.
- `language`: seed chỉ `zh|en`; path tạo qua API nhận chuỗi tự do ≤16 ký tự
  (`zh-Hans`, `vi`...).
- `status`: đúng 4 hằng, chỉ trim **không** hạ chữ hoa → `"DONE"` là 400 để lỗi
  client lộ sớm.
- `position` / `duration_weeks`: âm → 400.
- Không set `updated_at` tường minh khi UPDATE → trigger chạm (LWW merge).

## Quyết định khác cần R2/R3 biết

- **`activities` lưu JSON array string** trong `roadmap_topics.activities`
  (không gộp vào `why`) — docs có nhiều hoạt động/tài liệu mỗi topic, gộp vào
  `why` làm mất cấu trúc. API trả `activities` là `[]string`, rỗng = `[]`.
- **`percent` làm tròn XUỐNG (floor)**: `floor(done*100/total)`. 2/3 topic done =
  **66**, 1/3 = 33, 3/3 = 100, chưa có topic = 0 (không chia 0). Kiểm ở
  `Test_roadmap_progress_percent_floors`.
- **`position` không gửi → `max+1`** (thêm vào cuối) thay vì 0. Gửi tường minh thì
  dùng để reorder.
- **Validate seed không fail cả file** vì 1 link/kind lỗi: URL không dùng được →
  lưu NULL + `log.Printf` cảnh báo; `kind` sai → lưu rỗng + cảnh báo. Lỗi cấu
  trúc (JSON hỏng, `language` sai, title rỗng, text quá dài) mới fail → rollback
  toàn bộ tx.
- **Slug path seed** lấy từ field `slug` (tuỳ chọn) nếu có, không thì
  `slugify(title)`. `slugify` bỏ dấu tiếng Việt bằng bảng rune tự chứa —
  **không dùng `unicode/norm`** vì GOROOT máy này (`go1.27.1` linuxbrew) bị cắt
  stdlib, không có package đó. Nếu deploy chỗ khác có stdlib đầy đủ thì vẫn
  chạy (không import gì thêm).
- **`//go:embed roadmap_seed`** (nhúng cả thư mục) chứ không phải
  `roadmap_seed/*.json` — pattern không khớp file nào thì **Go build fail**, mà
  R1a phải build/test được ở trạng thái chưa có seed. Loader lọc `*.json` lúc
  runtime nên 0 file = no-op, không lỗi. `api/roadmap_seed/README.md` là
  placeholder (mô tả format) để embed có file để nhúng; loader bỏ qua.
- `main.go`: chỉ thêm `RegisterRoadmapRoutes(mux, db)` + `SeedRoadmap(db)` sau
  `SeedDemo` (lỗi chỉ `log.Printf`, không chặn boot).

## Kiểm chứng

- `go vet ./...` — sạch.
- `go test -count=1 ./...` — **toàn bộ xanh** (regression 14 route cũ không hỏng).
- 10 test roadmap: roundtrip path→stage→milestone→topic→resource, 409 slug trùng
  (path + stage), 400 title rỗng / slug sai / url sai scheme / kind sai / position
  âm, 4 status hợp lệ + status sai (kể cả DB CHECK), soft-delete 404 + lọc khỏi
  list, progress 0→1/3→2/3→3/3, 404 cho 17 route parent-missing, seed loader no-op
  khi dir rỗng, seed idempotent + **không overwrite title user sửa** + không hồi
  sinh resource đã xoá mềm, 1 file hỏng → rollback toàn bộ.
- Smoke thật với seed R1b vừa ghi: `SeedRoadmap` chạy 2 lần cho **2 path / 10 stage
  / 40 milestone / 51 topic / 263 resource**, `bad kind = 0`, `is_builtin = 1`.

## Còn lại cho phase sau

- R1b: 2 file JSON seed (đã có `roadmap_zh.json`; `roadmap_en.json` chờ ghi) —
  loader đã parse và nạp được cả 2, không cần đổi code.
- Sync (`api/sync.go`) **chưa** gộp 5 bảng roadmap — row có `guid` + trigger
  `updated_at` sẵn để mở rộng, nhưng đó là việc phase riêng (R3 hoặc gate R1c).
- R2 (UI) dùng `GET /api/roadmap/paths` cho màn danh sách + `GET /api/roadmap/paths/{slug}`
  cho cây, `PUT .../status` để đánh dấu, `GET .../progress` cho vòng tiến độ.

## File

- Sửa: `api/schema.sql`, `api/main.go`.
- Tạo: `api/roadmap.go`, `api/roadmap_seed.go`, `api/roadmap_test.go`,
  `api/roadmap_seed/README.md` (placeholder embed + mô tả format), file này.
- **Không** đụng `api/roadmap_seed/roadmap_zh.json` / `roadmap_en.json` (lane R1b).
