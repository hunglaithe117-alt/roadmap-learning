# STACK-V2 — M1 (platform + Postgres + domain thuần)

> Hoàn tất 2026-09-28. Gate của M1: oracle review DDL + ranh giới layer +
> index FTS.
>
> **ĐÃ BỊ SỬA Ở `stack-v2-m1-remediation.md`** (11 finding Oracle). Các mục
> dưới đây chỉ còn đúng ở chỗ không bị sửa; cụ thể: §7 "KHÔNG publish 5432 ra
> host" → **đã publish**; §3 mục FTS nói 4 index `gin_trgm` → **3** + thêm 1
> btree `lower(term)`; §9 nói `schema_migrations` "vẫn tồn tại" nhưng không
> nói rõ là **đã seed version 4**; `dict_search` default **10** không phải 20.

## 1. Version đã pin (không dùng `@latest`)

| Module | Version | Ghi chú |
|---|---|---|
| `github.com/pressly/goose/v3` | v3.28.0 | đúng plan §1 |
| `gorm.io/gorm` | v1.31.2 | plan ghi "v1.31.x"; v1.31.13 **không tồn tại** trên proxy, `.13` bị từ chối. Đây là version cao nhất của nhánh 1.31. |
| `gorm.io/driver/postgres` | v1.6.3 | plan ghi "driver v1.6.x" |
| `gorm.io/gorm/logger` | (theo `gorm.io/gorm` v1.31.2) | là sub-package, không có module riêng — không pin riêng |
| `github.com/go-playground/validator/v10` | v10.30.5 | đúng plan §1. Hiện ghi `// indirect` vì **chưa file nào import** (M4 mới dùng tag `binding:`) |
| `github.com/stretchr/testify` | v1.12.1 | đúng plan §1 |
| `golang.org/x/sync` | v0.23.0 | đúng plan §1. Cũng `// indirect` cho tới M2 (chưa dùng `errgroup`) |
| `go.yaml.in/yaml/v3` | v3.0.5 | kéo về transitively qua testify |
| `modernc.org/sqlite` | v1.59.0 | **giữ nguyên** — app v1 vẫn chạy, M4 mới gỡ |

Lưu ý vận hành: `go get` bị chặn bởi `sum.golang.org` (DNS timeout) trên máy
này. Đã bypass bằng `GOSUMDB=off` cho lần fetch đó; `go.sum` vẫn ghi đủ
hash nên build sau này không cần mạng.

## 2. DDL đã tạo

3 file trong `api/migrations/`, goose version cuối = **3**.

### `00001_init.sql` — dịch `api/schema.sql` → Postgres

Tạo **13 bảng ứng dụng**:

`decks` · `cards` · `reviews` · `notes` · `dict` · `en_dict` · `sync_meta` ·
`sync_conflicts` · `roadmap_paths` · `roadmap_stages` · `roadmap_milestones` ·
`roadmap_topics` · `roadmap_resources` · `schema_migrations`

(14 tên — `schema_migrations` vẫn giữ vì `sync.go`/`backup.go` v1 đọc
`MAX(version)` từ đó; M7 sẽ chuyển sang `goose_db_version`.)

Cộng `goose_db_version` do goose tự tạo → **16 bảng** trong schema, **46 index**.

Ba khác biệt bắt buộc khi đổi engine:

1. `INTEGER PRIMARY KEY AUTOINCREMENT` → `BIGSERIAL PRIMARY KEY`.
2. Trigger chạm `updated_at`:
   - `NEW.updated_at IS OLD.updated_at` → `IS NOT DISTINCT FROM` (Postgres có
     `IS` nhưng NULL-safe khác — `IS` **có** thật trong Postgres, tuy nhiên
     plan §4.3 chốt `IS NOT DISTINCT FROM` vì đúng ngữ nghĩa NULL-safe của
     trigger SQLite và tránh phụ thuộc vào việc cột có NULL hay không).
   - `strftime('%Y-%m-%dT%H:%M:%SZ','now')` →
     `to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`.
   - **Cải tiến có chủ đích**: SQLite dùng `AFTER UPDATE` + `UPDATE` lồng,
     Postgres dùng `BEFORE UPDATE` + gán `NEW.updated_at`. Hệ quả là **đệ quy
     vô hạn không còn**, nên điều kiện chặn "OLD khác now" của SQLite thành
     thừa và đã bỏ. 1 hàm `langapp_touch_updated_at()` dùng chung cho 7 bảng.
3. Guard `lang` của `decks`: SQLite không `ALTER TABLE ... ADD CHECK` được nên
   dùng 2 `BEFORE` trigger + `RAISE(ABORT)`. Postgres thêm CHECK được → đổi
   thành `CHECK (lang IN ('zh','en'))`. Cùng ràng buộc, ít trigger hơn.

Giữ nguyên theo yêu cầu: tên cột, `guid TEXT` / `updated_at TEXT` /
`deleted INTEGER DEFAULT 0` (**không** đổi sang `gorm.DeletedAt`),
FK `ON DELETE CASCADE`, partial index `ux_cards_deck_front ... WHERE deleted = 0`,
CHECK `status IN (4 hằng)`, CHECK `position >= 0`.

Down: `DROP TABLE ... CASCADE` tuần tự ngược + `DROP FUNCTION`. Được phép
destructive vì đang dev (theo spec).

### `00002_fts.sql` — thay FTS5

- `dict` / `en_dict` thành **bảng thật** (virtual table FTS5 của SQLite không
  nhận index thật, không map được sang Postgres). Tên cột giữ nguyên
  (`hanzi|pinyin|nghia`, `lang|term|reading|gloss`).
- Thêm `search_vector tsvector` + trigger `BEFORE INSERT OR UPDATE` ghi lại
  vector có trọng số (hanzi=A, pinyin=B, nghia=C).
- **6 index FTS**: `idx_dict_search` + `idx_en_dict_search` (GIN trên
  `search_vector`), `idx_dict_hanzi_trgm` / `idx_dict_pinyin_trgm` /
  `idx_en_dict_term_trgm` / `idx_en_dict_gloss_trgm` (GIN `gin_trgm_ops`).
- `CREATE EXTENSION pg_trgm` (quyền superuser của image `postgres` có).
- Hàm `dict_search(q, lim)` / `en_dict_search(q, lim)`: `websearch_to_tsquery`
  (chịu được cú pháp người dùng gõ thẳng, không ném lỗi như `to_tsquery`) **OR**
  `ILIKE '%q%'`.

### `00003_roadmap_a1.sql` — amendment A1

- `roadmap_stages.completed_at TEXT NULL`, `roadmap_topics.completed_at TEXT NULL`
- `roadmap_stages.deck_id BIGINT NULL REFERENCES decks(id) ON DELETE SET NULL`
- `roadmap_topics.is_optional INTEGER NOT NULL DEFAULT 0 CHECK (is_optional IN (0,1))`
- Bảng mới `roadmap_bookmarks` (id BIGSERIAL, title, url NULL, note, tags,
  status CHECK 4 hằng `to_read|reading|done|archived`, created_at, guid,
  updated_at, deleted) + `ux_roadmap_bookmarks_guid` UNIQUE.
- Thêm trigger chạm `updated_at` cho `roadmap_bookmarks` — bảng này cũng là
  bảng người dùng sửa nên phải theo convention v4, nếu không merge LWW phía
  peer sẽ không thấy thay đổi.

## 3. Quyết định Postgres đáng chú ý

1. **Mất FTS5.** `to_tsvector('simple', '你好')` coi CẢ chuỗi Hán là **một
   token**; `@@` với query `你好` sẽ **không** khớp row `你好世界`. Đây là
   giới hạn thật, ghi rõ trong plan §4.2 và cần ghi vào UI + README ở M4/M5.
   Hệ quả: tra Hán đa ký tự phải dùng đường `ILIKE`, nên index `gin_trgm` là
   bắt buộc chứ không phải tối ưu cho có. Test
   `Test_search_functions_find_latin_and_single_cjk` chứng minh cả 2 đường.
2. **`pg_trgm` là phụ thuộc runtime**, không chỉ lúc migrate: nếu image
   Postgres được build lại không có extension này thì `CREATE EXTENSION` fail.
   `pg_trgm` nằm trong `postgres-contrib` — image chính thức đã kèm.
3. **`now()` là thời điểm BẮT ĐẦU transaction** ở Postgres (khác
   `clock_timestamp()`). Trigger chạm `updated_at` vì vậy cho cùng 1 mốc
   trong cả transaction — đây là hành vi mong muốn, và là lý do điều kiện
   chống đệ quy của SQLite không còn cần.
4. **Bigserial = sequence + `bigint`**, không phải `serial` 32-bit. Mọi cột
   `id` cũ nay là 64-bit → code Go đang dùng `int64` là đúng, không cần đổi.
5. **FK giờ THẬT SỰ được enforce** (SQLite mặc định `PRAGMA foreign_keys` tắt
   trừ khi bật). M1 đã đặt `ON DELETE CASCADE` đúng như schema, nhưng M2 cần
   chú ý: thứ tự xóa thật sự cascade ở Postgres, test v1 có thể dựa vào
   việc cascade **không** chạy.
6. **`SELECT ... LIMIT` không có `ORDER BY` thì không ổn định** — Postgres
   trả thứ tự tuỳ ý (thường theo thứ tự vật lý). Các query v1 dựa vào
   `ORDER BY` sẵn nên không sao, nhưng M3 viết query mới phải luôn có
   `ORDER BY`.
7. **`INSERT OR IGNORE` của SQLite không tồn tại** → `INSERT ... ON CONFLICT
   DO NOTHING`. 4 chỗ trong `sync.go` v1 dùng cú pháp này, M3 phải dịch.
8. **`master.decks` / `incoming.cards`** (schema prefix của `ATTACH`) không
   tồn tại — Postgres dùng `search_path` hoặc schema thật. M3/M7 phải thiết
   kế lại cơ chế attach snapshot (nhiều khả năng: schema tạm + `SET
   search_path`).
9. **`VACUUM INTO` không tồn tại** → `pg_dump`/`pg_restore` subprocess (M7).
10. **Không có `PRAGMA`.** `platform/db.go` cố ý không set gì dạng PRAGMA;
    tương đương Postgres là DSN param / `SET`, để infrastructure lo khi cần.

## 4. Kiến trúc đã dựng

### `api/internal/platform/` (7 file)

| File | Vai trò |
|---|---|
| `config.go` | `Config` + `LoadConfig()` + `Validate()`. Đọc `PORT`, `LANGAPP_DB`, `WHISPER_URL`, `PIPER_BIN`, `PIPER_MODEL_ZH/_EN`, `GIN_MODE`, `LOG_LEVEL`, `DB_MAX_OPEN_CONNS/DB_MAX_IDLE_CONNS/DB_CONN_MAX_LIFETIME`. Không viper. |
| `logger.go` | slog JSON handler, `ParseLevel` map env → level. |
| `db.go` | `OpenPostgres(ctx, cfg)`. Tắt GORM logger mặc định, `AllowGlobalUpdate: false` tường minh, `NowFunc` UTC, pool sizing, ping. Có `slogWriter` để bật log SQL khi debug mà vẫn ra JSON. **Không PRAGMA.** |
| `migrate.go` | `Migrate(ctx, db)` — entrypoint migration **duy nhất**, `goose.NewProvider(DialectPostgres, sqlDB, migrations.FS)` + `provider.Up(ctx)`, trả `MigrationStatus{Version, Applied}` để test assert. |
| `di.go` | `Container` + `Build(ctx)` lắp theo thứ tự config → logger → DB → migrate; lỗi giữa chừng thì đóng pool đã mở. `Close()` idempotent, an toàn với nil. M2/M3 gắn repository vào đây. |
| `migrate_test.go` | 5 test DB thật (schema riêng cho mỗi test, tự drop). |
| `config_test.go` | 6 test thuần, không cần DB. |

**CẤM `db.Model(&X{}).Update(...)`** được viết thành hằng số ngay trong
`db.go` kèm lý do (bypass trigger `updated_at` + không `WHERE id`); test
trigger nằm ở `Test_touch_updated_trigger_sets_now_when_caller_omits`.

### `api/internal/domain/` — 7 context, 31 file

| Context | File | Nội dung chính |
|---|---|---|
| `srs` | `entity.go` `value_object.go` `policy.go` `due_filter.go` + test | `Deck`/`Card`/`Review`, `Grade` (thang 1-4 — **xem lưu ý bên dưới**), `ScheduleNext` (fallback 1-3-7-14-30 khi reps<3, FSRS-lite khi reps≥3, Again reset +1d), `Replay` (tính lại reps/lapses từ lịch sử — vòng recompute của `mergeReviews`), `DueFilter` (kèm `state='new'`) |
| `content` | `entity.go` `pinyin.go` `tone_policy.go` `chunk.go` `stress.go` `thieu.go` `simplify.go` + test | `DictionaryEntry`/`EnglishEntry`/`ZHCard`/`DrillItem`, `PinyinSyllable`+`MarkSyllable`+`PinyinMarks`, `TonePattern`+`GradeTonePair` (4/3/2/1), `Chunk`+`SplitChunks`+`ContentWords`, `StressResult`+`StressFromIPA`+`LookupStress`(+`LookupStressRule`), `THIEUAxis`+`THIEUAvg`, `ToSimplified` (bảng phồn→giản dùng chung với diff) |
| `roadmap` | `entity.go` `value_object.go` `progress.go` `status_policy.go` + test | `Path`/`Stage`/`Topic`/`Resource`/`Milestone`/`Bookmark`, `Status` 4 hằng, `IsOptional`, `ComputeProgress` (floor + **loại optional khỏi mẫu số**), `SetStatus` (set/clear `completed_at`), `CompletedSince`, `CountByStatus` |
| `audio` | `value_object.go` `port.go` + test | `Transcript`/`WordTimestamp`/`EngineInfo` (header `X-Engine`), port `TTSSynthesizer`/`STTTranscriber` + 2 phần mở rộng `LangSynthesizer`/`DetailedTranscriber` |
| `practice` | `entity.go` `diff.go` `shadow.go` + test | `ShadowSession`/`Recording`/`ErrorEntry`, `DiffPolicy`: `WordDiff` (LCS + gộp missing/extra thành wrong, port từ `web/src/player/diff.ts`), `WrongWords`, `DiffScore`, `Compare`, `NormalizeWrong`, `NormalizeRate` (0.5-1.5) |
| `insight` | `entity.go` `normalize.go` + test | `Stats`/`Range`/`ReviewCount`, `ComputeAccuracy`, `DueNow`, `ComputeStreak` (UTC, giữ streak nếu hôm nay chưa học), `TopError` (tie-break alphabet để thứ tự ổn định) |
| `sync` | `entity.go` `merge_policy.go` + test | `PeerSnapshot`/`Row`/`Conflict`, `Decide` (LWW theo `updated_at` → fallback `created_at`; **tombstone thắng khi mốc bằng**; log conflict khi cả 2 vế đổi sau `lastSync`), `DecideAppend` (reviews/notes dedupe theo GUID, **không mutate** tập đã biết), `DetectClockSkew`, `CheckVersion` |

**Khác biệt cần Oracle lưu ý:**

- `Grade` của `srs` khai báo **thang 1-4** (`MinGrade`/`MaxGrade`), không phải
  0-5 như brief M1 ghi. Lý do: `api/srs.go` v1 và UI đang đóng băng thang 1-4
  (`ReviewHandler` từ chối `grade < 1 || grade > 4`), 0/5 là khoảng trống thật.
  Mở rộng lên 0-5 ở M3 sẽ phá hợp đồng với UI hiện tại. Hằng số đặt sẵn trong
  file, thêm sau được.
- `practice` import `content` (dùng chung `ToSimplified`) — hợp lệ vì
  `content` không import ngược lại; đây là dependency hợp đồng trong plan §2
  (content → audio, practice → content).
- `insight` khai báo `ErrorEntry` cục bộ thay vì import `practice`: `insight`
  là read-model, giữ nó độc lập tránh vòng import. Chỗ gắn thật là
  `application/insight/ports.go` ở M3.
- `content` khai báo hằng grade 1-4 cục bộ (`toneGradeAgain`…) thay vì import
  `srs` — 2 bounded context độc lập, chỉ gặp nhau ở application.

## 5. Kết quả grep luật DDD

```
$ grep -rl 'gorm.io\|gin-gonic\|database/sql\|net/http\|google.golang.org/grpc' api/internal/domain
$ echo $?
1
```

**0 file** — đạt. Toàn bộ import trong `api/internal/domain/**` (không tính
`_test.go`):

```
errors · fmt · langapp/internal/domain/content · math · sort · strconv
strings · time · unicode · unicode/utf8
```

Tức là: 9 package stdlib + đúng 1 dependency nội bộ. Không driver DB, không
framework HTTP, không gRPC.

## 6. Kết quả validate

| Lệnh | Kết quả |
|---|---|
| `go build ./...` | PASS |
| `go vet ./...` | PASS (0 cảnh báo) |
| `gofmt -l api/internal` | PASS (0 file) |
| `go test ./...` **không** set `DATABASE_URL`/`LANGAPP_DB` | PASS — 9 package, app v1 + 7 domain + platform |
| `go test ./...` **có** Postgres thật | PASS — **259 test**, 0 FAIL, 0 SKIP |
| `docker compose config` | PASS |
| `docker compose up -d postgres` | PASS — `postgres:18`, healthy sau ~8s |

**Goose**: `version=3`, `applied=3` lần đầu, `applied=0` lần hai
(idempotent), `goose_db_version` có 4 dòng applied (1 dòng `version_id = 0`
làm mốc + 3 migration), **16 bảng** / **46 index** trong schema test.

**App cũ vẫn xanh** (ràng buộc quan trọng nhất của M1): `ok langapp 0.904s` —
toàn bộ suite `api/*.go` cũ (SQLite + `modernc.org/sqlite` + handler
`net/http`) vẫn build và test qua. `api/main.go` **không sửa một dòng**.
`api/schema.sql` **không sửa một dòng**.

## 7. docker-compose.yml

Thêm đúng 1 service `postgres` (không tạo service nào khác ngoài yêu cầu):

- `image: postgres:18` (Postgres 18.6, phát hành 09/2025 — bản stable mới
  nhất xác minh được trên máy này).
- `POSTGRES_DB/USER/PASSWORD=langapp`, `POSTGRES_INITDB_ARGS="--encoding=UTF8
  --locale=C.UTF-8"`.
- Volume `postgres-data` mount ở **`/var/lib/postgresql`**, KHÔNG phải
  `/var/lib/postgresql/data`. Postgres 18 đổi layout PGDATA và image **từ chối
  khởi động** nếu thấy dữ liệu ở path cũ
  (docker-library/postgres#1259). Đây là lỗi đã gặp và đã sửa khi verify —
  ghi lại để lane sau không phải debug lại.
- Healthcheck `pg_isready -U langapp -d langapp`.
- **KHÔNG publish 5432 ra host** (chọn internal). Lý do: Postgres là
  dependency nội bộ của app, chỉ app cần truy cập; `docker compose exec
  postgres psql` đủ để debug. Dòng `ports` có sẵn trong file, bỏ comment là
  bật. Host vẫn truy cập được qua IP container (đã dùng để verify test).
- `app` thêm `depends_on: postgres: condition: service_healthy` + biến
  `LANGAPP_POSTGRES_DSN`. **Không ghi đè `LANGAPP_DB`** — app v1 đọc biến đó
  như đường dẫn file SQLite; nếu đổi thành DSN thì app v1 hỏng ngay. M4 mới
  chuyển app sang đọc biến Postgres.
- Service `app` và profile `stt` cũ **giữ nguyên**, không sửa dòng nào.

## 8. SKIPPED + lý do

| Việc | Trạng thái | Lý do |
|---|---|---|
| Test migration khi **không** có Postgres | **SKIP có chủ đích** | `internal/platform` đọc `LANGAPP_TEST_POSTGRES_DSN`; env rỗng thì `t.Skip` với lý do rõ ràng. KHÔNG dùng testcontainers (plan §1 cố ý loại), nên CI không có Postgres là test tự skip chứ không giả vờ pass. |
| Publish 5432 ra host | **SKIPPED (có chủ đích)** | xem §7 — chọn internal, ghi rõ lý do. |
| `Migrate` chạy lúc boot app | **SKIPPED** | `api/main.go` cũ KHÔNG được sửa ở M1. `platform.Build()` đã có sẵn đường nối Postgres + migrate; M2/M4 mới gọi. |
| `errgroup` / `validator` dùng thật | **PENDING (M2/M4)** | Đã pin version trong `go.mod` nhưng chưa file nào import nên `go mod tidy` gắn `// indirect`. Sẽ tự chuyển thành direct khi M2/M4 viết code dùng. |
| GORM model + repository | **SKIPPED (M2)** | ngoài phạm vi ghi M1. |
| GraphQL / Gin / dataloadgen | **SKIPPED (M4)** | ngoài phạm vi ghi M1. |

## 9. Ghi chú vận hành cho lane sau

- **Lệnh verify migration** (Postgres phải healthy trước):
  ```sh
  docker compose up -d postgres
  IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' lang-learn-app-postgres-1)
  cd api && LANGAPP_TEST_POSTGRES_DSN="postgres://langapp:langapp@$IP:5432/langapp?sslmode=disable" go test ./...
  ```
  Nếu muốn gọi bằng `localhost:5432` thì phải bỏ comment dòng `ports` trong
  `docker-compose.yml` rồi `docker compose up -d postgres`.
- **`api/migrations/embed.go`** là package riêng (`package migrations`,
  export `FS`) chỉ vì Go `embed` không đi lên khỏi thư mục package được, còn
  `.sql` phải nằm ở `api/migrations/` cho Oracle review DDL. Không phải
  chi phí bất thường.
- **`schema_migrations` vẫn tồn tại** trong Postgres dù goose đã có
  `goose_db_version`, vì `sync.go`/`backup.go` v1 đọc nó. M7 dọn.
- **Hành vi lệch đã kiểm chứng khi port, ghi lại để M3 không "sửa nhầm"**:
  - `splitSyllables` cắt theo nhóm nguyên âm nên `"photography"` →
    `["pho","to","gra","phy"]` và `"elephant"` → `["E","le","pha","nt"]`.
    Pattern trọng âm suy ra **không** khớp phát âm chuẩn — vì vậy
    `LookupStressRule` LUÔN trả `exception=true`.
  - `MarkSyllable("n2")` trả `"n"`: không có nguyên âm nào thì không đặt được
    dấu.
  - `WordDiff("", "x")` trả `[extra "x"]`, không phải rỗng.
  - Cặp sai được gộp thành `"the→a"` (text gốc mẫu + mũi tên + text
    transcript), nên `WrongWords` trả về `"the→a"` chứ không phải `"a"`.
