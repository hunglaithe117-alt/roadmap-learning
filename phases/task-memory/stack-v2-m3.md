# STACK-V2 — M3 (infrastructure + application: `content`, `practice`, `insight`, `sync`)

> Hoàn tất 2026-09-28. Gate của M3: oracle review logic migrate 1:1, không
> đổi hành vi.
>
> M1: `stack-v2-m1.md` + `stack-v2-m1-remediation.md`. M2: `stack-v2-m2.md`.
> M3 **không** sửa `api/*.go` cũ và **không** sửa `web/`.

## 0. `internal/platform/testdb` — làm TRƯỚC mọi thứ

File `api/internal/platform/testdb/testdb.go` (file `.go`, **không** phải
`_test.go` — hằng số trong `_test.go` không import được qua package khác).

| Export | Việc |
|---|---|
| `MigrateLockKey` | hằng `20260928`, giữ nguyên giá trị M1/M2 |
| `DSN()` | đọc `LANGAPP_TEST_POSTGRES_DSN` |
| `Acquire(t, ctx) *Session` | nắm khoá advisory trên `*sql.Conn` session-level, `t.Cleanup` buông **sau cùng** |
| `Open(t, ctx)` / `OpenSchema(t, ctx)` | 1 schema test đã migrate, tự dọn |
| `OpenBareSchema(t, ctx)` | schema CHƯA migrate — chỉ dùng cho test của chính `platform.Migrate` |
| `(*Session).OpenSchema(t, ctx)` | thêm schema thứ 2 **dùng chung khoá** — test 2 máy |
| `WithSearchPath(dsn, schemas)` | `search_path` nhét vào DSN, nhận nhiều schema phân tách dấu phẩy |

3 bản sao `TestMain` + `newTestDB` + hằng `migrateLockKey` ở `platform`,
`infrastructure/srs`, `infrastructure/roadmap` → **1 bản duy nhất**.

### 2 phát hiện mới khi làm M3 (chưa có ở M2)

**1. Nhiều schema trong 1 test cần khoá suốt test.** `pg_advisory_lock` KHÔNG
xếp chồng giữa 2 session — session thứ 2 chờ vô hạn. Nên `Open` (chỉ giữ khoá
tới hết lần gọi) không dùng được khi test cần 2 schema; phải `Acquire` 1 lần rồi
`session.OpenSchema` nhiều lần. Đây là lý do tồn tại `Session`.

**2. `gin_trgm_ops` chỉ tìm thấy được trong schema GIỮ extension.**
`CREATE EXTENSION pg_trgm` cài operator class vào schema **đầu tiên** của
`search_path` lúc chạy. Schema thứ hai (peer) không có extension ⇒
`CREATE INDEX ... gin_trgm_ops` fail `42704` — **kể cả khi chạy tuần tự**, tức
không phải race mà là giới hạn thật.

Cách xử lý: `search_path` của schema thứ 2 gồm `schema2,schema1`. Sửa **1 chỗ**
trong `testdb` (không phải 7 chỗ như phương án "tạo extension 1 lần ở `public`"
mà M2 đã đánh giá là tốn kém) — khớp với đánh giá của Oracle là giữ hướng
`pg_advisory_lock` + `search_path` trong DSN.

**3. Postgres HỦY CẢ TRANSACTION khi 1 câu lệnh vi phạm UNIQUE** (SQLite chỉ
trả lỗi cho câu đó). Mọi INSERT của merge phải `ON CONFLICT ... DO NOTHING` +
`RETURNING id`, và `id == 0` là tín hiệu "bị skip" (tương đương `INSERT OR IGNORE`
của SQLite v1). Chi tiết ở §4.4.

## 1. Context `content` — dict + FTS + tone + English

### Use case (16)

| nhóm | use case |
|---|---|
| tra | `SearchDict` `SearchEnglish` `LookupStress` `UpsertDictEntry` |
| HSK | `ImportHSK` `GetStrokes` |
| tone | `GradeTonePair` `SetTone` |
| English | `SeedEnglish` `Chunk` `ListThieuAxes` `AppendThieu` `ListThieu` `GetReaderArticles` |

### Port

`Repository` (dict / en_dict / notes THIEU|) · `DeckWriter` · `CardToneWriter` ·
`CardReader` · `StaticContent` (dữ liệu bundle). **Không** cần `audio.STTPort`
— content chỉ đọc dict, không gọi audio.

### Vài điểm đáng ghi

- **`SearchDict`/`SearchEnglish` gọi HÀM SQL `dict_search`/`en_dict_search`**,
  không viết lại truy vấn. Hàm đó xử lý `websearch_to_tsquery` + ILIKE lưới vớ
  CJK + khử `%_\`.
- **`LookupEN` dùng `lower(term) = lower(?)`** → dùng được
  `idx_en_dict_term_lower`. **KHÔNG** `COLLATE NOCASE` (cú pháp SQLite, lỗi
  cú pháp ở Postgres). `api/english.go:274` vẫn còn lỗi này vì app cũ chạy
  SQLite — **không sửa**. Nợ #1 trong `.slim/deepwork/stack-v2.md` **đóng ở
  M3**.
- **Không map `search_vector`** vào struct GORM: cột do trigger sinh, map vào
  nghĩa là GORM thử ghi ngược.
- **`SrsAdapter`** (`infrastructure/content/srs_adapter.go`) hiện thực
  `DeckWriter`/`CardToneWriter`/`CardReader` trên bảng `decks`/`cards`. Nằm ở
  content chứ không ở `infrastructure/srs` vì M3 không được ghi vào đó (phạm vi
  ghi song song với lane đang sửa `repository.go` của srs). Interface vẫn khai ở
  `application/content` — đó mới là điểm quan trọng: `content` phụ thuộc `srs`
  qua interface, không đọc bảng của srs trực tiếp.
- **`DictHanziSet` + `CountDeckCards` nhận `tx`**: đọc ngoài transaction không
  thấy dòng vừa insert (chưa commit) → `cards_total` luôn 0 ở lần import đầu.
  Bắt buộc, không phải tối ưu.
- Seed data port **nguyên văn**: 36 từ HSK1 + 1043 từ HSK2-4, 26 headword
  en_dict, 48 thẻ PVO + 24 thẻ TMRND, 16 chữ bút thuận, 8 bài đọc.
- `LookupStressRule` vẫn luôn trả `exception=true` (giữ nguyên hành vi M1).

## 2. Context `practice` — shadowing + recorder + error book

### Use case (10)

`RecordShadowProgress` `GetShadowProgress` `TranscribeRecording`
`DiffAgainstSample` `SpeakSample` `AppendError` `ListErrors` `TopErrors`
`SuggestErrorsFromErrors` `MarkErrorResolved`

### Port

`Repository` (notes SHADOW|/ERR|) · `STTPort` · `TTSPort` ·
`UnitOfWork`. (`srs.CardReader` và `content.DictReader` cuối cùng **không cần**:
`AppendError` không bắt buộc kiểm tra thẻ — FK `notes.card_id` chặn sẵn, và
`DiffAgainstSample` lấy mẫu từ client chứ không tra từ điển. Bỏ 2 port đó
thay vì khai ra rồi không dùng.)

### Vài điểm đáng ghi

- **`DiffAgainstSample` chuẩn hóa phồn→giản TRƯỚC khi so** (`domain/practice.
  WordDiff` đã làm bên trong; tầng này chuẩn hóa thêm để transcript trả về
  client là bản đã chuẩn hóa — nếu không, UI hiện chữ phồn trong khi điểm đã
  chấm theo chữ giản).
- `TopErrors` quét **500 note** bất kể `limit` (giữ hằng v1): `limit` là số
  KẾT QUẢ trả về, không phải số note quét — quét 1 note sẽ ra số lần sai hoàn
  toàn sai.
- `SuggestErrorsFromErrors` dùng **1 JOIN** (`CountErrorNotesByCard`), có
  `c.deleted = 0` để thẻ xoá mềm tự rớt.
- `MarkErrorResolved` là use case **MỚI** (v1 không có): `notes` không có cột
  `deleted` nên "đánh dấu đã xử lý" = ghi 1 note MỚI cùng tham chiếu id, giữ
  tính append-only để sync union theo guid vẫn đúng. Xoá cứng note gốc thì máy
  peer không bao giờ nhận được trạng thái này.
- `IsResolvedNote` phân biệt 2 hình dạng JSON dùng chung prefix `ERR|`.

## 3. Context `insight` — stats + streak (read model)

### Use case (4)

`Stats(range)` `ComputeStreak` `TopErrors` `ProgressOverRange`

`Repository` **read-only**, không có `UnitOfWork`, không có method ghi.

### Vài điểm đáng ghi

- **Streak chuẩn UTC, giữ nguyên** — test `Test_streak_uses_utc_days_not_local_
  timezone` chứng minh 23:30 UTC hôm 27 + 00:30 UTC hôm 28 là 2 ngày UTC khác
  nhau (theo giờ VN +7 đều rơi vào 28/09). Không "cải thiện" thành local.
- `Stats.DueNow` = `deleted = 0 AND due_at <= now`, **KHÔNG** kéo `state='new'`
  (khác hàng đợi ôn `srs.DueCards` vốn luôn kéo thẻ mới). Đây là hành vi v1,
  giữ nguyên; ghi rõ trong doc comment để người đọc không "sửa nhầm".
- `TopErrors` quét toàn bộ lịch sử lỗi, `domain/insight.TopError` lo sắp xếp +
  tie-break alphabet.
- `insight` khai báo `ErrorEntry` cục bộ (M1) — không import `practice`.

## 4. Context `sync` — merge LWW (chiều sâu nhất)

### Use case (4)

`Sync` (nạp snapshot + merge) · `Merge(snapshot)` · `Status` · `Conflicts`

`BackupPort` khai trong `ports.go` nhưng **KHÔNG implement** — `pg_dump`/
`pg_restore` thuộc M7 (v1 dùng `VACUUM INTO` không tồn tại ở Postgres).

### 4.1 Bảng merge — roadmap đã vào danh sách

`decks` `cards` `reviews` `notes` `roadmap_paths` `roadmap_stages`
`roadmap_milestones` `roadmap_topics` `roadmap_resources`
`roadmap_bookmarks`

Bỏ qua (có hằng `SkipTables` + test khoá lại): `dict` `en_dict`
`schema_migrations` `goose_db_version`.

### 4.2 1 transaction cho MỌI bảng

Đây là khác biệt rõ nhất với v1: v1 chỉ merge `decks`/`cards`/`reviews`/`notes`
và bỏ sót toàn bộ `roadmap_*` (lỗ hổng ghi ở plan M7). Test
`Test_merge_rolls_back_roadmap_and_srs_together_on_midway_failure` chặn INSERT
`notes` bằng trigger tạm rồi assert deck + roadmap **cùng biến mất**.

### 4.3 `Omit("updated_at")` — vì sao có method `Merge*` riêng

Oracle đã đo `Omit("updated_at")` loại cột khỏi tập `SET` **kể cả khi map có
key tường minh**. Nếu merge gọi lại `srs.Repository.UpdateCard` thì mốc LWW từ
peer bị trigger `langapp_touch_updated_at` ghi đè bằng giờ máy local → 2 máy so
sai vĩnh viễn.

Vì vậy `internal/infrastructure/sync` có `UpsertDeck`/`UpsertCard`/`UpsertPath`/
… **riêng**, ghi thẳng SQL, set `updated_at` tường minh, **không** `Omit`.
Test `Test_merge_keeps_peer_timestamp_as_authoritative` assert mốc peer sống
sót sau merge.

### 4.4 3 bẫy Postgres mà SQLite v1 không có

1. **UNIQUE hủy cả transaction.** Mọi INSERT merge phải
   `ON CONFLICT ... DO NOTHING RETURNING id`; `id == 0` = bị skip.
2. **`ON CONFLICT DO NOTHING` (không chỉ định cột) KHÔNG bắt được lỗi từ
   PARTIAL unique index.** Postgres không suy ra được arbiter và vẫn ném
   `23505` → hủy cả merge. Vì vậy `cards` dùng
   `ON CONFLICT (deck_id, front) WHERE deleted = 0 DO NOTHING` (kèm index
   predicate), còn `decks` dùng `ON CONFLICT (guid) DO NOTHING`.
3. **Đọc ngoài transaction không thấy dòng vừa insert.** Mọi `*Rows` của
   repository merge nhận `tx`: merge ghi `decks` rồi cần map `guid→id` để ghi
   `cards`; đọc ngoài transaction sẽ **bỏ âm thầm toàn bộ card của peer**.

Ngoài ra: `anyToString` phải xử lý `int16`/`int32`/`int64` — pgx trả
`INTEGER` là `int32`; chỉ xử lý `int64` biến `deleted` thành chuỗi rỗng và
tombstone LWW biến mất không kêu.

### 4.5 `domain/sync.Decide` được dùng lại, không viết lại

Mọi quyết định LWW / tombstone / ghi log conflict gọi `domain/sync.Decide`;
tầng application chỉ thi hành `MergeDecision.Action`. Tương tự, replay lịch ôn
dùng `domain/srs.Replay` (CÙNG hàm `srs.Service.RecordReview` dùng — viết lại ở
đây là 2 bản lệch nhau sau vài tháng).

Bổ sung vào `domain/sync/entity.go` (M1 cho phép bổ sung): 6 hằng
`TableRoadmap*` + field `Row.ParentGUID` (GUID của cha theo bảng — tách khỏi
`DeckGUID` vì cùng 1 trường mang 2 ý nghĩa sẽ dễ gán nhầm khi đọc code).

### 4.6 Cơ chế "ATTACH" ở Postgres

SQLite có `ATTACH`; Postgres không (M1 §3.8 đã ghi bắt buộc thiết kế lại).
M3 dùng **2 schema trong cùng 1 database** qua `SnapshotLoader` interface.
M7 sẽ nạp `pg_restore` vào đúng chỗ đó — interface không đổi.

### 4.7 Test bắt buộc theo spec — đã có đủ

| yêu cầu | test |
|---|---|
| merge 2 snapshot cùng ôn offline (union, không mất dòng) | `Test_merge_unions_both_machines_offline_reviews` (4/4 review còn) |
| LWW theo `updated_at` | `Test_merge_applies_last_write_wins_by_timestamp` / `..._older_peer_...` |
| tombstone thắng khi mốc bằng | `Test_merge_tombstone_wins_when_timestamps_tie` |
| remap guid→id (không tin id số) | `Test_merge_resolves_foreign_keys_through_guid_not_numeric_id` (id 2 máy BẰNG NHAU) |
| clock-skew cảnh báo | `Test_merge_warns_about_clock_skew_without_blocking` |
| mốc peer sống sót | `Test_merge_keeps_peer_timestamp_as_authoritative` |
| merge chạm roadmap + decks/cards cùng 1 tx, rollback khi lỗi giữa chừng | `Test_merge_rolls_back_roadmap_and_srs_together_on_midway_failure` |
| reviews union + replay reps | `Test_merge_recomputes_reps_from_unified_review_history` |
| hồi sinh tombstone, giữ id (không nhân đôi) | `Test_merge_adopts_local_tombstone_when_peer_recreates_card` |
| idempotent khi merge 2 lần | `Test_merge_is_idempotent_on_second_run` |
| bỏ qua dict/en_dict/version books | `Test_merge_ignores_dict_and_version_tables_from_snapshot` |
| version lệch → từ chối | `Test_merge_rejects_peer_with_different_schema_version` |

## 5. Migration `00005_insight_index.sql` — CÓ

3 nhóm index, mỗi cái có 1 truy vấn cụ thể ghi trong comment:

| index | phục vụ truy vấn |
|---|---|
| `idx_reviews_reviewed_at` | `Stats`: `WHERE reviewed_at >= ?` — bảng chỉ tăng, không có index nào khác trên cột này |
| `idx_reviews_reviewed_day` (expression) | `ComputeStreak`: `DISTINCT substr(reviewed_at,1,10) ORDER BY … DESC` → index-only scan |
| `idx_notes_err` + `idx_notes_err_card` (partial) | sổ lỗi: `text LIKE 'ERR|%' AND text NOT LIKE 'THIEU|%'`, có cả đường lọc theo `card_id` |

**Không** index `text` bằng `pg_trgm`: prefix cố định 3 ký tự đã khớp btree của
partial index; trigram chỉ hữu ích khi tìm chuỗi con bất kỳ.

Goose version cuối = **5**.

## 6. Kết quả validate

| lệnh | kết quả |
|---|---|
| `docker compose up -d postgres` | healthy, `localhost:5432` mở |
| `LANGAPP_TEST_POSTGRES_DSN=… go test ./...` | **PASS** — 21 package, **549 test, 0 FAIL, 0 SKIP** |
| Chạy lại **3 lần** liên tiếp (`-count=1`) | **3/3 xanh** |
| `go vet ./...` | PASS, 0 cảnh báo |
| `gofmt -l internal migrations roadmap_seed` | sạch (0 file) |
| grep 1: `domain/` không `gorm.io\|gin-gonic\|database/sql\|net/http\|grpc` | **0 file** (exit 1) |
| grep 2: `application/` không `gorm.io` | **0 file** (exit 1) |
| grep 3: `domain`+`application` không import tầng hạ tầng | **0 file** (exit 1) |
| App cũ `ok langapp` | **PASS** (≈0.9s) |

Test count: 382 (M2) → **549** (toàn suite). Phần M3 thêm 167:

| package | test | M3 thêm |
|---|---|---|
| `internal/application/content` | 19 | mới |
| `internal/application/practice` | 23 | mới |
| `internal/application/insight` | 13 | mới |
| `internal/application/sync` | 23 | mới |
| `internal/infrastructure/content` | 24 | mới (Postgres thật) |
| `internal/infrastructure/practice` | 16 | mới (Postgres thật) |
| `internal/infrastructure/insight` | 8 | mới (Postgres thật) |
| `internal/infrastructure/sync` | 18 | mới (2 schema Postgres thật) |
| `internal/platform` | 11 | (không đổi số, chỉ cập nhật 4→5 migration) |

Cả 3 grep phải ra 0 **kể cả trong comment** — 3 comment của M3 ban đầu có chứa
chuỗi `internal/infrastructure` và đã viết lại (đúng như M2 đã làm).

### Verify bổ sung bằng dependency thật (không chỉ grep text)

`grep` bắt được cả chuỗi trong comment nên có thể **báo đỏ oan**. Kiểm tra chắc
chắn hơn là hỏi `go list -deps` — nó chỉ thấy import THẬT, không thấy comment:

```
$ for p in $(go list ./internal/application/... ./internal/domain/...); do
    go list -deps "$p" | grep -E 'gorm\.io|gin-gonic|google\.golang\.org/grpc|internal/infrastructure'
  done | sort -u
# (rỗng)
```

Cả 3 luật đều giữ đúng ở mức dependency: `domain` và `application` không import
driver, framework, hay tầng hạ tầng. Interface nằm ở `application/<ctx>/ports.go`,
hiện thực nằm ở `infrastructure/<ctx>` — ví dụ `application/content/ports.go`
khai `StaticContent` + `DeckWriter` + `CardToneWriter` + `CardReader`,
`infrastructure/content` hiện thực bằng `SeedContent` + `SrsAdapter`.

## 6b. Tương thích với F1 của lane song song (đã verify)

Lane song song sửa `application/roadmap`: `UpdateStageStatus`/`UpdateTopicStatus`
nay nhận `*Stage`/`*Topic` (tự làm mới `s` sau khi ghi) thay vì nhận rời
`(id, status, note, completedAt)`; `domain/roadmap/entity.go` +
`ApplyStatus`/`CollectTopics`/`CountByStatus` đã bị xoá.

M3 **không** gọi 2 method này và **không** import `domain/roadmap` ở đâu —
`infrastructure/sync` tự viết SQL merge của cây roadmap (xem §4.3: không được
dùng lại `Update*` của repository khác vì `Omit("updated_at")`). Nên đổi chữ ký
không ảnh hưởng M3; đã chạy lại `go build`, `go vet` và `go test ./...` 3 lần
sau khi lane merge — vẫn 21/21 package xanh.


## 7. File đã sửa NGOÀI phạm vi ghi (nêu rõ)

1. **`internal/platform/migrate_test.go`** — chuyển từ `package platform`
   (in-package) sang `package platform_test` (external).
   **Bắt buộc**: `testdb` import `platform`; nếu test ở trong package `platform`
   thì đó là import cycle (Go cấm in-package test import package phụ thuộc vào
   nó). File này chỉ dùng identifier export nên chuyển được.
   Đồng thời sửa 4 con số migration 3→4→**5** và thay toàn bộ `TestMain`/`newTestDB`
   bằng `testdb`. **Không sửa dòng logic production nào của `platform`**
   (`config.go` `db.go` `di.go` `migrate.go` `logger.go` nguyên vẹn).
2. **`internal/infrastructure/srs/helper_test.go`** và
   **`internal/infrastructure/roadmap/helper_test.go`** — chỉ xoá `TestMain` +
   `migrateLockKey` + `connectLock` + `newTestDB` cũ, thay bằng `testdb.Open`.
   **KHÔNG** sửa `repository.go`/`model.go`/`seed.go` của 2 context đó (lane song
   song đang sửa chúng).
3. **`internal/domain/sync/entity.go`** — thêm 6 hằng `TableRoadmap*` + field
   `Row.ParentGUID`. Nằm trong phạm vi ghi của M3.

## 8. SKIPPED + lý do

| Việc | Trạng thái | Lý do |
|---|---|---|
| `BackupPort` (`pg_dump`/`pg_restore`) | **KHAI, KHÔNG implement** | Thuộc M7 (plan §4.6). Khai interface để chỗ gọi đã có hợp đồng; subprocess trong application sẽ phá luật tầng. |
| Sửa `api/english.go:274` (`COLLATE NOCASE`) | **SKIPPED có chủ đích** | App cũ chạy SQLite nên câu đó hợp lệ ở đó. Code M3 đã viết đúng (`lower(term) = lower(?)`) và có test EXPLAIN chứng minh dùng index. Nợ #1 đóng. |
| Sửa `api/*.go` cũ (`srs.go` `chinese.go` `english.go` `player.go` `errorbook.go` `stats.go` `reader.go` `sync.go` `backup.go` `simplify.go` `chinese_hsk.go`) | **KHÔNG sửa 1 dòng** | Ràng buộc M3; app cũ phải xanh. |
| Wire repository vào `platform.Container` | **PENDING (M4)** | `di.go` ngoài phạm vi ghi. Mọi service đã sẵn sàng nhận qua `NewService(...)`; test đã dựng đúng như cách M4 sẽ wire. |
| `insight` đọc qua port của `srs`/`practice` | **PENDING (M4)** | M3 cho `insight` repository read-only riêng đọc thẳng 4 bảng: đây là read model, 1 SQL mỗi câu là rẻ hơn N port + N query. M4 nếu tách repository theo context thì thay bằng port, câu SQL không đổi. |
| `srs.CardReader` / `content.DictReader` trong `application/practice` | **KHÔNG khai** | Đã cân nhắc rồi bỏ: `notes.card_id` FK chặn sẵn (không cần check thẻ tồn tại) và `DiffAgainstSample` lấy mẫu từ client. Khai port không dùng là nợ. Xem §2. |
| `errgroup` / `validator` thành direct dep | **PENDING (M4)** | Vẫn chưa file nào import. |

## 9. Ghi chú cho M4/M7

- **Wire DI**: `contentinfra.NewRepository` + `NewUnitOfWork` + `NewSrsAdapter` +
  `NewSeedContent`; `practiceinfra.NewRepository` + `NewUnitOfWork` (cần bind
  `STTPort`/`TTSPort` — sẽ là client gRPC M4); `insightinfra.NewRepository`;
  `syncinfra.NewPeerRepository(local, peer)` + `NewSchemaLoader(peer)`.
  Test M3 đã dựng đúng các wiring này, copy được.
- **`Merge(snapshot)` chưa có đường vào từ HTTP**: M4 nhận multipart rồi
  `pg_restore` vào schema tạm (M7) rồi `SchemaLoader` đọc — interface không đổi.
- **Streak UTC không được "cải thiện" thành local.** Đã có test chứng minh hành vi
  khác nhau; đổi sẽ làm hỏng test và hồi quy hợp đồng v1.
- **`domain/sync.Row.Values` là `map[string]string`** nên NULL bị ép thành chuỗi
  rỗng. `completed_at`/`map_x`/`url` phải đi qua `snapshot_rows.go` để giữ
  `*string`/`*float64` — thêm bảng/cột nullable mới vào merge thì nhớ bổ sung ở
  đó, nếu không NULL sẽ thành `''` và CHECK/FK sai.
- **`testdb.Session`**: test nào cần > 1 schema phải `Acquire` 1 lần rồi mở
  nhiều schema. `Acquire` 2 lần sẽ treo vô hạn (advisory lock không xếp chồng).
- `schema_migrations` vẫn = version **4** (bảng app v1 đọc, không theo goose);
  M7 dọn.
