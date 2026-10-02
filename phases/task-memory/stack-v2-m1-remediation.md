# STACK-V2 — M1 remediation (sau cổng Oracle)

> 2026-09-28. Sửa đúng 11 finding Oracle, không mở rộng scope. Bản gốc:
> `phases/task-memory/stack-v2-m1.md` (đã bị ghi đè ở các mục nêu dưới).

## 11/11 finding đã xử lý

| # | Mức | Finding | Sửa ở đâu |
|---|---|---|---|
| F1 | CHẶN | compose set `LANGAPP_POSTGRES_DSN` nhưng `LoadConfig` chỉ đọc `LANGAPP_DB` → M4 thay binary là boot-crash ở `Validate()` | `internal/platform/config.go`: `DSN: envString("LANGAPP_POSTGRES_DSN", envString("LANGAPP_DB", DefaultDSN))` + comment giải thích vì sao 2 biến phải tách. Test: `Test_load_config_prefers_postgres_dsn_over_langapp_db`, `Test_load_config_falls_back_to_langapp_db` |
| F2 | CHẶN | Postgres không publish ra host + `LANGAPP_TEST_POSTGRES_DSN` không có trong docs → 6 test luôn SKIP | `docker-compose.yml`: bỏ comment `ports`, thật sự publish `${POSTGRES_PORT:-5432}:5432`. `DEPLOY.md`: thêm mục "## Postgres (STACK-V2 M1…)" với lệnh bật test + bảng 4 biến |
| F3 | HIGH | `SET search_path` chỉ áp cho 1 conn của pool (MaxOpen=5); goose lấy conn khác → 15 bảng rơi vào `public`, `DROP SCHEMA` là no-op | `migrate_test.go`: `openTestDB` ghép `search_path` vào **DSN** qua helper `withSearchPath`, xoá hẳn `SET search_path` (cả ở cleanup) |
| F4 | HIGH | `schema_migrations` tạo rồi bỏ rơi → `MAX(version)` NULL → `CheckVersion(0,0)` pass vacuously | `migrations/00001_init.sql`: cuối file `INSERT … VALUES (4, …) ON CONFLICT (version) DO NOTHING`. Test `Test_schema_migrations_seeded_with_version_4` (assert MAX=4 + không nhân bản) |
| F5 | HIGH | `q` nội suy thô vào `ILIKE` → user gõ `%`/`_` thành wildcard, trả nguyên bảng | `migrations/00002_fts.sql`: cả 2 hàm `dict_search`/`en_dict_search` đổi `LANGUAGE sql` → `plpgsql`, đầu hàm `qc := regexp_replace(q, '[%_\\]', '', 'g')` + `IF qc IS NULL OR btrim(qc) = '' THEN RETURN`. Test `Test_search_functions_reject_like_wildcards` (7 input độc: `%`, `_`, `%_`, `''`, `'   '`, `%%%`, `\`) |
| F6 | MED | `api/english.go:274` dùng `COLLATE NOCASE` — syntax error ở Postgres | `migrations/00002_fts.sql`: `CREATE INDEX idx_en_dict_term_lower ON en_dict (lower(term))` + comment yêu cầu M3 port `lower(term) = lower(?)`. **Không sửa `api/english.go`** (app cũ chạy SQLite) |
| F7 | LOW | `dict_search` default `lim=20` lệch `LIMIT 10` của `main.go:36` | `migrations/00002_fts.sql`: default đổi thành 10 |
| F8 | LOW | `idx_en_dict_gloss_trgm` là index chết (hàm search không ILIKE `gloss`) | `migrations/00002_fts.sql`: xoá dòng tạo index (Up lẫn Down) |
| F9 | LOW | `reviews.grade` chưa có CHECK, lệch lập luận đã dùng cho `decks.lang` | `migrations/00001_init.sql`: `grade INTEGER NOT NULL CHECK (grade BETWEEN 1 AND 4)`. Test `Test_reviews_grade_check_rejects_out_of_range` (1-4 OK, 0/5/-1 lỗi) |
| F10 | LOW | comment `db.go` mô tả sai cơ chế ("GORM loại bỏ `WHERE id = ?`") | `internal/platform/db.go`: viết lại — `AllowGlobalUpdate: false` khiến GORM **trả `ErrMissingWhereClause` chứ không chạy câu lệnh**; hậu quả chỉ xảy ra nếu ai đó bật `AllowGlobalUpdate: true` |
| F11 | LOW | `roadmap.Stage/Topic.CompletedAt` là `string` cho cột NULL, lệch `srs.Card.Tone` đã dùng `*string` | `internal/domain/roadmap/entity.go`: 2 field → `*string` + comment "repository M2 phải ghi SQL NULL chứ không `''`". `status_policy.go`: `ApplyStatus` dịch `""` → `nil`, `CompletedSince` nil-check. `policy_test.go`: thêm helper `ts()`, `Test_apply_status_writes_topic_and_stage` assert `Nil` thay vì `Empty` |

## Kết quả validate

| Lệnh | Kết quả |
|---|---|
| `docker compose up -d postgres` | healthy ~8s, `localhost:5432` **mở** (đã publish) |
| `cd api && go test ./...` **có** `LANGAPP_TEST_POSTGRES_DSN` | PASS — **264 test**, **0 FAIL**, **0 SKIP** (M1 gốc: 259 / 0 / 0) |
| `cd api && go test ./internal/platform/...` **không** set DSN | 8 test DB SKIP có chủ đích (hành vi cũ giữ nguyên), 8 test config PASS |
| `cd api && go vet ./...` | PASS, 0 cảnh báo |
| `gofmt -l api/internal api/migrations` | sạch (0 file) |
| `docker compose config` | PASS |
| App cũ `ok langapp` | PASS (0.926s) — `api/main.go` + `api/schema.sql` vẫn không sửa dòng nào |
| `grep -rl 'gorm.io\|gin-gonic\|database/sql\|net/http\|google.golang.org/grpc' api/internal/domain` | **0 hit** (exit 1) |

### Bằng chứng riêng cho F3 / F8

```
$ psql -tAc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'"
0                       # trước fix: 15 bảng test rơi vào public
$ psql -tAc "SELECT count(*) FROM pg_indexes WHERE indexname='idx_en_dict_gloss_trgm'"
0
```

### Trạng thái schema sau remediation

`goose version=3 applied=3`, **16 bảng**, **46 index** (F8 −1, F6 +1 nên tổng
không đổi), `schema_migrations MAX(version) = 4`.

6 index FTS: `idx_dict_search` (GIN tsvector) · `idx_dict_hanzi_trgm` ·
`idx_dict_pinyin_trgm` · `idx_en_dict_search` (GIN tsvector) ·
`idx_en_dict_term_trgm` · `idx_en_dict_term_lower` (btree `lower(term)`).

Roundtrip `DownTo(0)` → còn đúng 1 bảng (`goose_db_version`), `Up()` lại
thành công — các function `plpgsql` mới rollback sạch.

## Ghi chú cho lane sau

- **`reviews.guid` UNIQUE trên cột `NOT NULL DEFAULT ''`**: 2 review cùng guid
  rỗng là đụng nhau. Hành vi này giữ nguyên từ `schema.sql` v1, **không** phải
  hồi vấn đề mới — nhưng M2 phải luôn sinh guid (v1 `ReviewHandler` đã làm vậy).
  Test `Test_reviews_grade_check_rejects_out_of_range` đã phải sinh guid khác
  nhau cho mỗi dòng; comment tại chỗ ghi rõ.
- **F5 đổi 2 hàm search từ `LANGUAGE sql` sang `plpgsql`**: giao diện
  (`RETURNS TABLE`, tham số, `DEFAULT`) không đổi nên không có call site nào
  phải sửa. Đổi lại: thêm 1 lần gọi `regexp_replace` mỗi lần tra — vô hại vì
  hàm `STABLE`.
- **F1 đặt tiền tử thứ tự biến**: `LANGAPP_POSTGRES_DSN` thắng `LANGAPP_DB`.
  Nếu đổi binary mà set nhầm `LANGAPP_DB=/data/langapp.db`, `Validate()` vẫn
  chặn (không phải `postgres://`) → fail sớm, có thông báo rõ.
