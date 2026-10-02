# stack-v2 — typed-nil interface (`stack-v2-typednil`)

> Worker: fixer. Sửa 2 lỗi **typed-nil interface gây panic 500** do lane M7a
> phát hiện rồi revert kèm (`stack-v2-m7a.md` §3.1).
> Phạm vi ghi: **`api/**`** + file bàn giao này + `.slim/deepwork/stack-v2.md`.
> **KHÔNG** sửa `web/**`. **KHÔNG** thêm `pg_dump`/`pg_restore` — user đã chốt
> bỏ, 2 endpoint vẫn trả **501**.

## 0. Kết quả

| Việc | Trạng thái |
|---|---|
| **B2** — `mutation.sync` panic 500 | **XONG** — 2 lớp chặn + test |
| **B1** — `Container.Backup` nil trong interface | **XONG** — `backupDisabled` + test |
| Rà toàn bộ `api/internal/` | **XONG** — 14 chỗ, bảng ở §3 |
| 2 chỗ `*gorm.DB` nil **mới phát hiện khi rà** | **XONG** — 2 guard + 6 test |
| Verify + mutation-check | **XONG** — §4, §5 |

---

## 1. B2 — `mutation.sync` panic 500 (quan trọng hơn)

### 1.1 Bằng chứng trước khi sửa

Chạy test lần đầu in ra đúng chuỗi gọi:

```
gorm.io/gorm.(*DB).Session(0x0, ...)              ← db == nil
gorm.io/gorm.(*DB).WithContext(...)
syncinfra.(*SchemaLoader).guidByID(...)  snapshot.go:214
syncinfra.(*SchemaLoader).Load(...)      snapshot.go:177
syncapp.(*Service).Sync(...)             service.go:63
gqltransport.(*mutationResolver).Sync(...)  root.resolvers.go:492
```

Client nhận `{"message":"internal system error","path":["sync"]}` — mất
trọn vẹn thông báo "chưa cấu hình nguồn snapshot peer" mà `Service.Sync` đã
soạn sẵn.

### 1.2 Nguyên nhân

```go
// di.go (trước)
c.Sync = syncapp.NewService(syncRepo, syncinfra.NewUnitOfWork(db),
    syncinfra.NewSchemaLoader(nil), nil)
    └─ trả *SchemaLoader (CON TRỎ ≠ nil) bọc peer = nil
         └─ ép vào tham số SnapshotLoader (INTERFACE) → interface KHÔNG nil
              └─ service.go: `if s.loader == nil` → FALSE  ← chỗ đo sai
                   └─ gọi Load với peer nil → deref *gorm.DB nil → PANIC
```

### 1.3 Đã chọn phương án nào, và vì sao

Có 2 lựa chọn như đề bài nêu: (a) truyền đúng dependency, (b) bỏ hẳn tham số
+ xoá field. **Đã chọn (a) — truyền `nil` thật** — vì:

- App v1 **không có** nguồn snapshot peer. Peer chỉ tồn tại trong test 2 schema
  (`internal/infrastructure/sync/helper_test.go`) và sẽ là schema tạm sau
  `pg_restore` ở M7. Nên ở composition root, giá trị **đúng** của tham số là
  "không có" — và "không có" của interface là `nil` thật.
- Phương án (b) **xoá hợp đồng đang dùng**: `Merge` vẫn cần loader qua `Sync`,
  và test 2 máy vẫn phải truyền loader thật.

**Không dừng ở 1 dòng.** Sửa xong thì `== nil` ở tầng application lại đủ dùng,
nhưng bẫy vẫn còn nguyên cho lần refactor sau. Nên thêm **2 lớp chặn**:

| Lớp | Vị trí | Bắt được hình thức nào |
|---|---|---|
| 1 | `platform/di.go` | `NewSchemaLoader(nil)` không còn được gọi ở production |
| 2 | `application/sync/service.go` `typednil.Is(s.loader)` | `(*T)(nil)` ép vào interface — **bằng chứng B2** |
| 3 | `infrastructure/sync/snapshot.go` `l.peer == nil` | `NewSchemaLoader(nil)` — trả con trỏ **KHÔNG nil** bọc peer nil, mà lớp 2 **không bắt được** |

Lớp 3 là điểm dễ tưởng là thừa: `typednil.Is` soi con trỏ **ngoài cùng**, mà
`NewSchemaLoader(nil)` trả con trỏ ngoài cùng **không nil** ⇒ lớp 2 trả FALSE
⇒ chỉ lớp 3 mới nói thật. Bỏ lớp 3 thì B2 quay lại y nguyên dạng gốc.

### 1.4 Đổi status 500 → 409 (không nằm trong đề bài, nhưng BẮT BUỘC)

Cổng trả `StatusInternalServerError` khiến `toUserError` (`errors.go:80`) che
message thành `"lỗi hệ thống"` — tức **sửa xong panic thì user vẫn không đọc
được lý do**. Đề bài yêu cầu "trả lỗi nghiệp vụ có message tiếng Việt", nên đổi
`StatusConflict`. Test `Test_Sync_rejects_typed_nil_loader` gắn assertion này ở
tầng application để lỗi sai loại bị bắt sớm, không đợi tới transport.

---

## 2. B1 — `Container.Backup` nil vào interface

### 2.1 Hiện trạng sau revert

`Container.Backup` **đã bị gỡ** (M7a §3.1), `cmd/langapp/main.go` truyền
`Backup: nil` — nên **hôm nay không còn đường nổ**. Nhưng `Options.Backup` vẫn
là interface, và handler vẫn so `port == nil` ⇒ **đúng cái bẫy đã nổ 1 lần**.

### 2.2 Đã chọn phương án nào, và vì sao

Đề bài đưa 2 lựa chọn: (a) đổi kiểu ở `Container` sang interface, (b) thêm cờ
`HasBackup bool`. **Cả 2 đều không còn đúng chỗ** vì `Container.Backup` không
còn tồn tại. Đã chọn hướng **(a) — giữ kiểu interface, siết chỗ kiểm**:

- `Options.Backup` **đã là** `syncapp.BackupPort` (interface) — đúng như (a)
  muốn. Comment ở `router.go` + `main.go` ghi lại để M7 không "tiện tay" đổi
  thành `*pgBackup`.
- Handler dùng `backupDisabled` → `typednil.Is` thay cho `port == nil`.
- **Không dùng `HasBackup bool`**: cờ phải đồng bộ thủ công với giá trị port ở
  mọi nơi dựng `Options` (main, test, harness) ⇒ thêm 1 nguồn sự thật để quên,
  và quên cờ thì lại 501 giả khi M7 đã bật. `typednil.Is` soi thẳng giá trị nên
  **không có gì phải đồng bộ**.
- `Container.Backup` không được thêm lại (thuộc phần backup đã bỏ).

### 2.3 Message

`"chưa dùng được"` → **`"chưa bật"`**. Client cần từ này để hiện "tính năng
đang xây" thay vì báo lỗi. Giữ `M7` trong message (test cũ `router_test.go`
assert nó).

---

## 3. Rà toàn bộ `api/internal/` — 14 chỗ

Quét 3 hướng: (i) field kiểu interface trong struct, (ii) dependency `Wire`
truyền `nil` cố ý, (iii) chỗ so `x == nil` với biến vốn là interface.

| # | Vị trí | Biến | Loại | Reachable? | Xử lý |
|---|---|---|---|---|---|
| 1 | `platform/di.go` → `sync.Service.loader` | interface | typed-nil | **CÓ** — `mutation.sync` | **ĐÃ SỬA** (B2) 3 lớp |
| 2 | `transport/http` `Options.Backup` → handler | interface | typed-nil | **CÓ (tương lai)** — M7 | **ĐÃ SỬA** (B1) `backupDisabled` |
| 3 | `infrastructure/sync` `SchemaLoader.peer` | `*gorm.DB` | nil con trỏ | **CÓ** — `NewSchemaLoader(nil)` | **ĐÃ SỬA** (mới) |
| 4 | `infrastructure/sync` `Repository.peer` | `*gorm.DB` | nil con trỏ | **CÓ** — `NewPeerRepository(db, nil)` | **ĐÃ SỬA** (mới) |
| 5 | `cmd/langapp` `Options.DB` (`Pinger`) | interface | typed-nil `*sql.DB` | Không | Ghi nhận §3.1 |
| 6 | `cmd/langapp` `Options.TTS`/`STT` | interface | typed-nil | Không | Ghi nhận §3.1 |
| 7 | `application/roadmap` `Service.decks` | interface | typed-nil | Không | Ghi nhận §3.1 |
| 8 | `application/practice` `Service.stt` | interface | typed-nil | Không | Ghi nhận §3.1 |
| 9 | `application/practice` `Service.tts` | interface | typed-nil | Không | Ghi nhận §3.1 |
| 10 | `application/content` `Service.data` | interface | typed-nil | Không | Ghi nhận §3.1 |
| 11 | `application/content` `Service.tone` | interface | typed-nil | Không | Ghi nhận §3.1 |
| 12 | `application/content` `Service.decks`/`cards` | interface | **không có** `== nil` | Không | Ghi nhận §3.1 |
| 13 | `infrastructure/audio` `Engine.TTSSynth`/`STTSTT` | interface | typed-nil | Không | Ghi nhận §3.1 |
| 14 | `transport/graphql` `Loaders` qua context | type assert | — | Không | Ghi nhận §3.1 |

### 3.1 Vì sao 5–14 KHÔNG sửa (đã kiểm từng chỗ, không đoán)

- **#5, #6, #13**: `Engine` do `NewFromEnv` dựng, **luôn gán cả 2 interface** —
  stub khi `AUDIO_GRPC_ADDR` rỗng, adapter khi có. Không có đường nào để
  `TTSSynth` là con trỏ nil. `main.go` truyền `c.Audio.TTSSynth` (đã là
  interface) nên không có chỗ chuyển từ con trỏ sang nil.
- **#7–#12**: `Wire` truyền adapter **concrete** (`&roadmapDeckReader{...}`,
  `&sttPort{...}`, `srsAdapter`, `contentinfra.NewSeedContent()` trả `&SeedContent{}`).
  Không có `nil` nào đi vào. #12 đáng chú ý: `decks`/`cards` **không có** cổng
  `== nil` nào, nhưng vì luôn non-nil nên hiện chưa nổ.
- **#5**: `Container.SQLDB()` đã kiểm `c.DB == nil` và trả error; `gorm.DB()`
  trả `(nil, ErrInvalidDB)` chứ không phải `(nil, nil)` — đã đọc source gorm
  v1.31.2 để xác nhận.

**Vì sao để yên thay vì sửa hết**: sửa 10 chỗ không reachable = thêm 10 điều kiện
chạy mỗi request mà **không test nào chứng minh cần**, và tự mở đường cho việc
"chặn nhầm" (xem `Test_Sync_still_merges_when_loader_is_real`). Rủi ro thật đã
được chặn; phần còn lại là **nợ biết**, ghi ở §6 để M7 xử lý đúng lúc nó bật
backup/audio thật.

### 3.2 Ranh giới đã vạch: typed-nil ≠ nil-pointer

#3, #4 **không** thuộc lớp typed-nil (`*gorm.DB` là kiểu cụ thể, `== nil` của Go
là đúng). Chúng cùng **đường tới panic** nên sửa, nhưng tách riêng ở đây để
không ai nghĩ `typednil.Is` là liều thuốc cho mọi `nil`. Ghi rõ trong comment
`SchemaLoader.Load` và `Repository.PeerSchemaVersion`.

---

## 4. Verify

| Lệnh | Kết quả |
|---|---|
| `go test -count=1 ./...` × **3 lần liên tiếp** | **27 package ok, 0 FAIL, 0 SKIP** × 3 |
| `go build ./...` | OK |
| `go vet ./...` | sạch |
| `gofmt -l .` | chỉ 2 file cũ được phép (`chinese_test.go`, `reader.go`) |
| grep luật DDD #1 (`domain/` × gorm/gin/grpc) | **0 file** |
| grep luật DDD #2 (`application/` × gorm/gin/grpc) | **0 file** |
| grep luật DDD #3 (`go list -deps -test`) | **0** |
| `go list -deps -test ./...` | sạch; `typednil` chỉ phụ thuộc `reflect` (package lá) |
| `go test .` (app cũ `api/*.go`) | **ok langapp** |
| `go generate ./...` | **idempotent** (md5 trước/sau khớp, 0 diff) |
| Test lại sau `go generate` | 0 FAIL, 0 SKIP |

**0 SKIP**: mọi test chạy thật với `LANGAPP_TEST_POSTGRES_DSN` trỏ `localhost:5433`.

### 4.1 10 test mới

| File | Test | Bắt gì |
|---|---|---|
| `typednil/typednil_test.go` | `Test_Is_detects_every_nil_shape` (+3 sub) | 3 hình nil; có **tiền đề** `x == nil` phải FALSE |
| | `Test_Is_keeps_usable_values` | không chặn nhầm giá trị dùng được |
| | `Test_Is_separates_nil_container_from_empty_container` | nil map ≠ map rỗng |
| | `Test_Is_pierces_pointer_to_nil_interface` | typed-nil 2 tầng |
| | `Test_Is_does_not_call_any_method` | hàm sinh chặn panic không được tự nổ |
| | `Test_Is_documented_contract` | `error` nil, `fmt.Stringer`, `time.Duration` |
| `application/sync/typednil_loader_test.go` | `Test_Sync_rejects_typed_nil_loader` | **B2 tầng application**; kèm assert status ≠ 500 |
| | `Test_Sync_rejects_plain_nil_loader` | cổng `== nil` cũ vẫn còn |
| | `Test_Sync_still_merges_when_loader_is_real` | loader con trỏ **không nil** không bị chặn nhầm |
| `transport/graphql/typednil_sync_test.go` | `Test_sync_mutation_returns_business_error_instead_of_panicking` | **B2 đầu-cuối** qua `graph/client` |
| | `Test_sync_mutation_full_payload_does_not_panic` | 3 field non-null của `MergeResult` |
| | `Test_sync_mutation_invalid_payload_does_not_panic` (6 sub) | **"payload sai"** — 6 dạng query hỏng |
| | `Test_sync_status_and_conflicts_still_work_after_sync_disabled` | sửa B2 không làm hỏng phần đọc |
| `transport/http/backup_typednil_test.go` | `Test_backup_and_restore_return_501_when_port_is_typed_nil` (2 sub) | **B1**: 501, không 500, không gọi method trên con trỏ nil |
| | `Test_backup_and_restore_return_501_for_plain_nil_port` (2 sub) | nhánh `nil` thật |
| | `Test_backup_disabled_recognises_every_nil_shape` | mọi hình nil shape |
| | `Test_not_implemented_message_is_vietnamese_and_says_not_enabled` | chữ "chưa bật" |
| `infrastructure/sync/typednil_peer_test.go` | `Test_SchemaLoader_Load_without_peer_returns_error_not_panic` | **#3** |
| | `Test_SchemaLoader_Load_on_nil_receiver_returns_error_not_panic` | receiver nil |
| | `Test_Repository_PeerSchemaVersion_without_peer_returns_error_not_panic` | **#4** |
| | `Test_Repository_PeerSchemaVersion_on_nil_receiver_returns_error_not_panic` | receiver nil |
| | `Test_Repository_with_peer_still_works` | **không chặn nhầm** chiều đọc |
| | `Test_SchemaLoader_with_peer_still_works` | **không chặn nhầm** chiều đọc |

3 test `mustNotPanic` — `defer recover()` + `t.Errorf` (KHÔNG `t.Fatal`) để
panic thành **FAIL rõ ràng kèm stack** thay vì làm sập cả binary test làm hỏng
26 package khác.

### 4.2 "Payload hợp lệ / payload sai" — chính xác nghĩa là gì

`mutation.sync` **không có tham số nào** (`sync: MergeResult!` trong
`root.graphqls`). Nên diễn giải 2 nhánh thành:
- **hợp lệ**: query đúng, 2 dạng (tối giản + đầy đủ 9 field `merged` + 4 field
  conflict) ⇒ phải trả lỗi nghiệp vụ tiếng Việt, `ok=false`, không panic.
- **sai**: query sai hình dạng, 6 dạng (field lạ, đối số thừa, sub-field trên
  scalar, operation rỗng, field gốc sai tên, ngoặc không cân) ⇒ gqlgen phải
  chặn ở tầng validation, **không** chạm `Service.Sync`.

Đã thử 1 dạng sai trước (`mutation { sync { ok } }` — "thiếu sub-field") và
test **FAIL** vì GraphQL cho phép chọn subset. Đã bỏ dạng đó, thay bằng 6 dạng
sai thật sự ở trên. Ghi lại vì đây là loại test tưởng xanh nhầm dễ xảy ra.

---

## 5. Mutation-check — 5/5 FAIL thật

| # | Mutation | Test bắt | FAIL vì |
|---|---|---|---|
| 1 | `di.go` trả lại `NewSchemaLoader(nil)` | `Test_sync_mutation_returns_business_error_…` | `"lỗi hệ thống"` không chứa `"chưa cấu hình"` |
| 2 | `service.go` `typednil.Is` → `== nil` | `Test_Sync_rejects_typed_nil_loader` | PANIC nil deref |
| 3 | `backup.go` `typednil.Is` → `port == nil` | `Test_backup_and_restore_return_501_when_port_is_typed_nil` + `…recognises_every_nil_shape` | 500 thay vì 501 |
| 4 | Xoá guard `l.peer == nil` (`snapshot.go`) | `Test_SchemaLoader_Load_without_peer…` + `…on_nil_receiver…` | PANIC nil deref |
| 5 | Xoá guard `r.peer == nil` (`repository.go`) | `Test_Repository_PeerSchemaVersion_without_peer…` + `…on_nil_receiver…` | PANIC nil deref |
| 6 | `typednil.Is` → `v == nil` | 3 test trong `typednil` | `Should be true` |

**Mutation 1 đáng chú ý**: nó KHÔNG panic nữa (vì lớp 2 + 3 vẫn còn) mà FAIL ở
assertion **message**. Điều đó chứng minh tầng graphql test đúng thứ cần kiểm:
"không panic" là điều kiện tối thiểu, **"user đọc được lý do"** mới là hợp đồng.

---

## 6. File đã sửa

### Sửa
- `internal/platform/di.go` — `Wire` truyền `nil` thật cho `SnapshotLoader`.
- `internal/application/sync/service.go` — `typednil.Is(s.loader)`, status 500→409.
- `internal/infrastructure/sync/snapshot.go` — guard `l == nil || l.peer == nil`.
- `internal/infrastructure/sync/repository.go` — guard `r == nil || r.peer == nil`.
- `internal/transport/http/backup.go` — `backupDisabled()`, message "chưa bật".
- `internal/transport/http/router.go` — hợp đồng `Options.Backup` cho M7.
- `cmd/langapp/main.go` — comment: khai interface, không khai con trỏ.

### Thêm
- `internal/typednil/typednil.go` — package lá, 1 hàm `Is`, chỉ import `reflect`.
- `internal/typednil/typednil_test.go`
- `internal/application/sync/typednil_loader_test.go`
- `internal/transport/graphql/typednil_sync_test.go`
- `internal/transport/http/backup_typednil_test.go`
- `internal/infrastructure/sync/typednil_peer_test.go`

**KHÔNG** thêm `pg_dump`/`pg_restore`/bất kỳ phần backup nào.

`grep -rn "PgDump\|PgRestore\|pgbackup\|pg_dump\|pg_restore" api/` → 23 hit,
**toàn bộ nằm trong comment** (giải thích vì sao endpoint trả 501 + ghi chú
M7). Đã kiểm thêm 2 mốc:

- `grep -rn 'func.*Dump(ctx' api/` → **0 hit**: không có hiện thực `BackupPort`
  nào (type nào đều không thoả interface ⇒ handler luôn 501).
- `grep -rn '"os/exec"' api/` → 2 hit, **cả 2 đều có sẵn từ trước và không liên
  quan backup**: `api/audio.go` (app v1 chạy Piper TTS) và
  `api/services/audio-service/synthesizer.go`. Không hit nào trong
  `internal/transport/http/backup.go` hay `internal/infrastructure/sync/`.

`/api/backup` + `/api/restore` vẫn trả **501**.

---

## 7. Nợ còn lại

| # | Việc | Vì sao | Khi nào |
|---|---|---|---|
| 1 | `Options.DB` (`Pinger`) dùng `typednil.Is` | #5 — chưa reachable, `SQLDB()` đã chặn | M7 |
| 2 | `Options.TTS`/`STT` dùng `typednil.Is` | #6, #13 — `NewFromEnv` luôn gán cả 2 | khi bật audio thật |
| 3 | `roadmap.Service.decks`, `practice.Service.stt`/`tts`, `content.Service.data`/`tone` | #7–#11 — `Wire` truyền adapter concrete | M7 |
| 4 | `content.Service.decks`/`cards` **chưa có** cổng `== nil` | #12 — luôn non-nil nên chưa nổ | M7 |
| 5 | `pg_dump`/`pg_restore` | user đã chốt bỏ | M7 (nếu đổi ý) |
| 6 | Xoá 2 file scratch `zz_repro_attempt2*.go` | ngoài phạm vi; `attempt2b` đã bị xoá ngoài ý muốn trong lúc làm việc | lần dọn sau |
| 7 | `roadmap_resources.kind` chưa có CHECK ở DB | `stack-v2-m7a.md` §1.2 | M7b |
| 8 | `DEPLOY.md` + `THIRD-PARTY-LICENSES` | chưa đụng tới | M7 |

---

## 8. Bài học rút ra (cho lane sau)

1. **`x == nil` trên interface trả lời SAI câu hỏi.** Nó hỏi "interface này có
   chứa con trỏ nil không", không phải "có dùng được không". Dùng `typednil.Is`.
2. **Sửa 1 dòng ở composition root không đủ** nếu bẫy còn ở tầng dưới: người
   sau gọi `NewSchemaLoader(nil)` lại làm lại y hệt. B2 cần **3 lớp** vì 3 lớp
   bắt 3 hình thức khác nhau (§1.3).
3. **Test "không panic" là điều kiện tối thiểu.** Mutation 1 cho thấy 1 fix còn
   có thể FAIL vì user đọc không được message. Phải assert **nội dung** lỗi.
4. **Mọi test "phải chặn" cần test đối trọng "không chặn nhầm".** Không có nó
   thì `typednil.Is` soi quá tay và `/api/backup` 501 vĩnh viễn — lỗi mà test
   nào cũng xanh vì "vẫn trả 501".
5. **"Đã revert" không phải "đã hết".** B1 hôm nay không nổ, nhưng bẫy vẫn nằm
   trong `Options.Backup`. Sửa cổng 501 tốn 1 dòng; trả lại về panic thì tốn
   1 incident.
