# DEPLOY — lang-learn-app (web, single-user, Docker)

> Stack v1 (SQLite + `net/http`) **đã bị gỡ ở M7c**: mã nguồn `api/*.go` cấp gốc,
> service `app` trong compose, volume `langapp-data` và binary `/app/langapp` đều
> đã xoá. Dưới đây chỉ còn stack v2 (Go + DDD + Gin + GraphQL + PostgreSQL).

## Chạy

```bash
# App + Postgres (đủ dùng)
docker compose --profile v2 up -d --build
# mở http://localhost:8081

# Kèm audio thật (gRPC tách process) + Whisper STT — lần đầu tải model ~466MB
docker compose --profile v2 --profile audio --profile stt up -d --build
```

Cổng host: app = `8081` (`V2_PORT` để đổi) · Postgres = `5433` (`POSTGRES_PORT`) ·
Whisper = `9000`. Cổng **trong mạng compose** luôn là 5432 — mọi DSN nội bộ dùng
`@postgres:5432`, đừng sửa nhầm.

Profile `v2` sẽ được gỡ ở milestone sau (cùng lúc đổi tên service `app-v2` →
`app`). Profile `audio` và `stt` vẫn còn vì `audio-service` của v2 đọc
`WHISPER_URL`.

## Kiểm tra còn sống không

```bash
curl localhost:8081/api/health
# {"status":"ok"}        — Postgres OK, audio engine thật
# {"status":"degraded"}  — Postgres OK, audio đang dùng stub (chạy được, không chết)
# HTTP 503               — Postgres chết, app không phục vụ được
```

`degraded` là trạng thái hợp lệ, không phải lỗi. Nó xuất hiện khi `AUDIO_GRPC_ADDR`
trỏ tới chỗ không có thật — hoặc cố ý bỏ profile `audio` cho dev nhanh.

## Reset dữ liệu — làm lại từ đầu

Đây là cách **mặc định khi đang dev**: xoá hết, seed lại. Không có backup/restore
(xem mục dưới).

```bash
cd /home/hung1/personal/lang-learn-app

# 1. Dừng hết, xoá volume Postgres
docker compose --profile v2 down -v

# 2. Bật lại — migration + seed chạy tự động lúc boot
docker compose --profile v2 up -d

# 3. Xác nhận seed đã nạp
curl -s localhost:8081/api/health
docker compose exec -T postgres psql -U langapp -d langapp -c \
  "SELECT (SELECT count(*) FROM roadmap_paths) AS paths,
          (SELECT count(*) FROM roadmap_topics) AS topics,
          (SELECT count(*) FROM roadmap_resources) AS resources,
          (SELECT count(*) FROM cards) AS cards;"
```

Kỳ vọng: `2` path (Trung, Anh) · `10` stage · `51` topic · `263` resource ·
`~1100` thẻ HSK. `down -v` xoá **mọi thứ bạn tự tạo**: bookmark đã lưu, deck tự
thêm, thẻ đã ôn, tiến độ roadmap đã đánh dấu. Seed chỉ nạp lại dữ liệu có sẵn
trong `api/roadmap_seed/*.json` + seed HSK/PVO/TMRND.

Nếu chỉ muốn xoá dữ liệu mà **không** xoá volume (nhanh hơn, nhưng chỉ khi schema
không đổi):

```bash
docker compose exec -T postgres psql -U langapp -d langapp \
  -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
docker compose --profile v2 restart app-v2
```

## Backup / restore — CHƯA CÓ (trả 501)

`GET /api/backup` và `POST /api/restore` trả **501 "chưa bật"**. Lý do: Postgres
cần `pg_dump`/`pg_restore` (chạy subprocess), mà máy này **không có** 2 binary đó.
Không có đường thay bằng SQL thuần — `VACUUM INTO` là của SQLite, Postgres không có.

UI route Cài đặt hiện báo lỗi rõ, không tải về file hỏng.

## Đồng bộ 2 máy (merge peer, 1 chiều)

Chưa bật được vì nó cũng đi qua file backup. Thiết kế đã có trong
`application/sync` + `domain/sync.Decide` (LWW theo `updated_at`, tombstone thắng,
reviews append dedupe `guid`, clock-skew chỉ cảnh báo). Khi nào có `pg_dump` thì bật
lại được — xem `phases/task-memory/stack-v2-m7a.md`.

## Env

| Biến | Mặc định | Ý nghĩa |
|---|---|---|
| `V2_PORT` | `8081` | Cổng host của app |
| `POSTGRES_PORT` | `5433` | Cổng **host** của Postgres (nội bộ luôn 5432) |
| `POSTGRES_PASSWORD` | `langapp` | Mật khẩu; đổi thì set cùng lúc ở app |
| `LANGAPP_TEST_POSTGRES_DSN` | `make test` tự set | DSN cho `go test` — trỏ database test, KHÔNG phải database app |
| `ALLOW_SKIP_DB_TESTS` | _(trống = không cho)_ | Set `1` để cho phép skip test DB khi thiếu DSN |
| `LANGAPP_POSTGRES_DSN` | _(compose set)_ | App đọc biến này |
| `AUDIO_GRPC_ADDR` | `audio-service:9090` | Rỗng ⇒ audio fallback stub |
| `PIPER_BIN` | trong image | Binary Piper TTS |
| `PIPER_MODEL_ZH` / `_EN` | trong image | 2 voice (~63MB mỗi cái) |
| `WHISPER_URL` | _(trống = stub)_ | Sidecar STT (`http://stt:9000/asr`) |
| `GIN_MODE` | `release` | Tắt introspection + `/playground` |

## Chạy test

Dùng `make` — đó là entrypoint duy nhất:

```bash
make db-up        # dựng Postgres + database test `langapp_test` (idempotent)
make test         # go test ./... với DSN thật
make test-web     # pnpm vitest run + pnpm typecheck
make check        # test + test-web + go vet + go build  ← chạy cái này
```

### Vì sao phải qua `make` chứ không `go test ./...` trần

`go test` không có khả năng "fail vì chưa set biến môi trường", nên trước M7c
`go test ./...` không set DSN báo **"525 PASS / 219 SKIP / 0 FAIL"**. Con số
"0 FAIL" đó gần như vô nghĩa khi 1/3 suite im lặng bỏ qua — và đúng loại đó đã
giấu bug mất dữ liệu ở chiều ghi (F1) suốt 2 phase.

Nên bây giờ `TestMain` của mọi package test DB **FAIL** khi thiếu
`LANGAPP_TEST_POSTGRES_DSN`, trừ khi `ALLOW_SKIP_DB_TESTS=1` được set **tường
minh**:

```bash
# Chạy cố ý không có Postgres (chỉ test không cần DB):
ALLOW_SKIP_DB_TESTS=1 go test ./...
```

### Test dùng database RIÊNG, không đụng database app

`make test` trỏ DSN về `langapp_test`, KHÔNG phải `langapp`.

Lý do không phải lý thuyết: test dựng schema tạm rồi đặt `search_path` trỏ vào
đó, nhưng `search_path` buộc phải chứa schema giữ extension `pg_trgm` — mà
extension đó nằm ở `public`, cùng schema chứa bảng thật của app. Khi đó
`SELECT count(*) FROM dict` sau khi test đã `DROP TABLE dict` sẽ **rơi xuống
`public` và đọc dữ liệu thật của bạn** thay vì báo "relation does not exist" —
test xanh vì đọc nhầm bảng production. Test còn có thể *ghi* vào bảng thật.

`testdb.requireIsolatedDB` fail nếu DSN trỏ vào database có dữ liệu app, nên
quên đổi DSN sẽ ra lỗi tường minh thay vì kết quả sai.

```bash
make test-db-reset   # xoá sạch database test rồi dựng lại
```

Test migration tự tạo schema riêng (`t_test_*`) rồi `DROP SCHEMA … CASCADE` ở
cleanup.

> `pnpm vitest run` **không** typecheck. Thêm 1 giá trị hợp lệ vào 1 union type
> có thể làm test xanh trong khi `pnpm build` đỏ — vì vậy `make test-web` gọi
> `pnpm typecheck` luôn.

## Dò query khi dev

`GIN_MODE=release` (mặc định của compose) ⇒ `/playground` **trả 404** và
introspection **tắt**. Ba cách dò query:

1. Bỏ `GIN_MODE: release` trong `docker-compose.yml` (chỉ khi dev, app không có
   auth — đừng bật ở máy khác).
2. Dùng `graph/client` trong test Go — không cần server.
3. `curl -X POST localhost:8081/query -H 'content-type: application/json' -d '{"query":"…"}'`.

Khi chạy Vite dev (`:5173`) với app trong container, CORS chỉ nhận
`http://localhost:5173` — đừng đổi cổng dev.

## Ghi chú kỹ thuật

- **Múi giờ UTC**: mọi timestamp (review/due/streak) là UTC. Client hiển thị giờ
  địa phương nhưng không gửi ngày đã convert. Streak sẽ roll lúc 7h sáng VN.
- **Search**: Postgres `tsvector` + `pg_trgm`. Tra 1 ký tự Hán đi đường `ILIKE`
  (Postgres không có `zhparser`) — kém chính xác hơn, chấp nhận được.
- **Soft delete**: cột `deleted INTEGER` ở mọi bảng người dùng sửa được, **không**
  dùng `gorm.DeletedAt` — logic sync LWW phụ thuộc kiểu integer này.
- **Trigger `updated_at`**: Postgres trigger `BEFORE UPDATE` gán thẳng `NEW`.
  Vì vậy repository ghi thường **luôn Omit** `updated_at`; chỉ đường merge mới set
  tường minh. Sửa sai chỗ này là hỏng mốc LWW.
- **Single-user, không auth**: đừng expose `app-v2` ra ngoài localhost/trusted LAN.
- License: xem `THIRD-PARTY-LICENSES`.
