# roadmap_seed — seed data cho learning path (R1b)

Thư mục này được nhúng vào binary. Có **2 loader** cùng đọc chung 2 file JSON:

| Loader | Dùng bởi | Cách đọc |
|---|---|---|
| `api/roadmap_seed.go` (app v1) | binary SQLite, `main.go` | `//go:embed roadmap_seed` — embed cả thư mục |
| `api/internal/infrastructure/roadmap/seed.go` (STACK-V2 M2) | binary Postgres | `package roadmapseed` trong chính thư mục này (`embed.go`), `//go:embed *.json` |

Cần package `embed` riêng vì Go `embed` không đi lên khỏi thư mục package được:
loader M2 nằm ở `internal/infrastructure/roadmap/` (sâu 3 cấp) nên không với
tới được thư mục này. Cách này giống hệt `api/migrations/embed.go` với file
`.sql`. Pattern `*.json` (không phải cả thư mục) để `README.md` này khỏi bị
nhúng vào binary.

Cả 2 loader đọc **mọi file `*.json`** trong thư mục, sort theo tên để thứ tự
insert ổn định, chạy 1 lần lúc boot, và nạp TẤT CẢ các file trong 1
transaction (1 file hỏng ⇒ rollback toàn bộ, không để lại cây nửa vời).

Thư mục không có file `*.json` → loader no-op, KHÔNG lỗi.

## Format

Mỗi file = 1 learning path. Tên file không bắt buộc theo quy ước gì cả —
`language` trong nội dung mới là nguồn sự thật.

```json
{
  "language": "zh",
  "title": "Tự học tiếng Trung giản thể từ số 0 → HSK 4",
  "overview": "Mô tả ngắn về path",
  "slug": "tu-hoc-tieng-trung",   // tuỳ chọn — bỏ trống thì tự sinh từ title
  "stages": [
    {
      "id": "zh-g0",
      "title": "G0 — Pinyin & thanh điệu",
      "goal": "Mục tiêu của stage",
      "duration_weeks": 3,
      "milestones": ["Viết được 50 chữ HSK1", "Nghe hiểu tên của người quen"],
      "topics": [
        {
          "title": "Nhóm âm đầu (bopomofo)",
          "why": "Vì sao cần học phần này",
          "activities": ["Nghe 10 phút mỗi ngày", "Viết tay 20 chữ"],
          "resources": [
            {
              "title": "Bài giảng bopomofo",
              "url": "https://example.com/real-link",
              "kind": "video",
              "note": "ghi chú tùy chọn"
            },
            { "title": "Từ vựng tự tra", "url": null, "kind": "tool", "note": "" }
          ]
        }
      ]
    }
  ]
}
```

## Ràng buộc validate (lúc boot, file hỏng → rollback toàn bộ tx, boot log lỗi)

- `language`: chỉ `zh` hoặc `en` (seed là contract; path user tạo qua API thì
  nhận chuỗi tự do tối đa 16 ký tự).
- `title`: **tiêu đề đọc được**, bắt buộc không rỗng — KHÔNG phải slug. Ví dụ
  `"Tự học tiếng Trung giản thể từ số 0 → HSK 4"`. **Slug path** lấy từ field
  `slug` nếu có, không thì loader tự `slugify(firstNonEmpty(slug, title))`:
  bỏ dấu tiếng Việt, ký tự ngoài `[a-z0-9]` gộp thành `-`, cắt còn ≤64 ký tự.
  Nên 2 file JSON hiện tại **không khai `slug`**, slug ra tự sinh từ title:
  - `roadmap_zh.json` → `tu-hoc-tieng-trung-gian-the-tu-so-0-hsk-4`
  - `roadmap_en.json` → `lo-trinh-tu-hoc-tieng-anh-methods-ngong-tap-thieu-tich-hop`
- `stages[].id`: slug của stage (`[a-z0-9-]`, nên prefix language như `zh-g0`).
  Nếu không phải slug (ví dụ `"G0 — Chữ viết"`), loader tự sinh `stage-<n>` theo
  thứ tự và log cảnh báo.
- `stages[].title`, `topics[].title`, `resources[].title`: bắt buộc không rỗng.
- `url`: `null` hoặc link `http`/`https` parse được → lưu NULL khi rỗng/không
  dùng được (kèm log cảnh báo, không fail cả file). **KHÔNG bịa link** — nguồn
  không có link thì để `null`.
- `kind`: rỗng hoặc 1 trong `video, article, tool, app, book, course, site,
  podcast, channel`. Sai thì log cảnh báo + lưu rỗng (không fail file).
- `duration_weeks`: âm → coi như 0.
- `activities[]`: lưu thành JSON array string ở `roadmap_topics.activities`.

## Quy tắc vàng: chỉ INSERT, không bao giờ UPDATE

Natural key để chống insert trùng:

| bảng               | natural key              |
| ------------------ | ------------------------ |
| `roadmap_paths`    | `slug`                   |
| `roadmap_stages`   | `(path_id, slug)`        |
| `roadmap_milestones` | `(stage_id, text)`      |
| `roadmap_topics`   | `(stage_id, title)`      |
| `roadmap_resources`| `(topic_id, title)`      |

Đã tồn tại → bỏ qua, giữ nguyên nội dung user. Nên user sửa tiêu đề path seed
rồi restart app, tiêu đề đó **không** bị seed ghi đè lại. Path seed đánh dấu
`is_builtin = 1` (user vẫn sửa/xoá được — xoá là xoá mềm `deleted = 1`).

## Bản đồ game: terrain + direction (STACK-V2 M2)

File JSON **không** khai terrain/direction. Loader M2 gán sẵn theo vị trí stage
trong path (0-based), lấy từ `domain/roadmap.MapDefaults` — cùng bảng mà
migration `00004_roadmap_map.sql` gán lại cho DB đã có sẵn stage:

| path | stage | terrain | direction |
|---|---|---|---|
| `zh` | G0 | `meadow` | `up` |
| `zh` | G1 | `meadow` | `up` |
| `zh` | G2 | `desert` | `up` |
| `zh` | G3 | `snow` | `up` |
| `zh` | G4 | `volcano` | `up` |
| `en` | G0 | `meadow` | `up` |
| `en` | G1 | `ocean` | `up` |
| `en` | G2 | `city` | `right` |
| `en` | G3 | `snow` | `right` |
| `en` | G4 | `volcano` | `right` |

Sửa bảng này = sửa 3 nơi: `MapDefaults`, migration `00004`, và test
`Test_seed_assigns_terrain_and_direction_per_roadmap_map_idea` (test sẽ đỏ
nếu lệch).
