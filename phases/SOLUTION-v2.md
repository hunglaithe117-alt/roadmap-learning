# SOLUTION-v2 — Mở rộng sau v1 (real audio + content + sync)

## 7. Real audio (Piper TTS + Whisper STT thật)

### Option A: Piper `os/exec` trực tiếp (khuyến nghị)

- Engine OHF-Voice/piper1-gpl v1.6.1 (~32MB) + 2 voice `zh_CN-huayan-medium` +
  `en_US-lessac-medium` (~63MB/voice, tổng ~160MB, RAM infer <300MB).
- Env `PIPER_BIN`/`PIPER_MODEL` đã có sẵn trong code; Go `exec` theo lang,
  header `X-Engine: piper`, cache theo (text, lang, voice).
- STT: service `stt` image `onerahmet/openai-whisper-asr-webservice`
  (faster-whisper, small CPU int8 ~466MB disk / ~1.5GB RAM), Go adapter ~10 dòng
  `join(segments[].text)`; fix `language=zh/en` + `vad_filter` + `beam_size=1`.

### Option B: Wyoming sidecar cho Piper + FastAPI STT tự viết

- Wyoming thêm 1 hop HTTP + container, lợi là chuẩn protocol khi nhiều voice.
- FastAPI STT tự viết cho JSON đúng contract ngay từ đầu, khỏi adapter join.
- Giá: thêm service/compose, tốn RAM/disk hơn, code mới phải bảo trì.

### Trade-offs

| Tiêu chí | A: exec + adapter (khuyến nghị) | B: wyoming + FastAPI riêng |
|---|---|---|
| Số service | API + stt có sẵn | + wyoming/TＴS sidecar |
| Disk/RAM | ~160MB + ~466MB, RAM nhẹ | nặng hơn, nhiều layer |
| Code mới | Adapter ~10 dòng | FastAPI service mới |
| License | GPL mere aggregation + offer source | tương tự, thêm mặt bảo trì |

Khuyến nghị **Option A**: ít moving parts nhất mà vẫn ra giọng/transcript thật,
đúng thiết kế v1 (`os/exec` + sidecar HTTP).

## 8. Content (HSK seed + PVO expand)

### Option A: drkameleon chính + clem109 backup (khuyến nghị)

- Nguồn chính `drkameleon/complete-hsk-vocabulary` (MIT, JSON, bundle OK giữ
  copyright); backup `clem109/hsk-vocabulary` (MIT, CSV) để đối chiếu count.
- HSK 2.0 L1-L4 = 1200 từ; HSK 3.0 đầy đủ (`ivankra/hsk30`, ghi rõ nguồn) để sau.
- Import qua endpoint có sẵn (idempotent upsert `hanzi+level`); nghĩa Việt tự
  soạn + pinyin đối chiếu, KHÔNG copy tài liệu bản quyền.

### Option B: clem109 chính + scrape bổ sung

- CSV dễ đọc nhưng schema nghèo hơn JSON drkameleon, phải scrape thêm pinyin/ví dụ.
- Nguy cơ dính nội dung bản quyền khi scrape; đối chiếu 2 chiều mất công hơn.

### Trade-offs

| Tiêu chí | A: drkameleon chính (khuyến nghị) | B: clem109 chính + scrape |
|---|---|---|
| Schema | JSON đủ field, ít xử lý | CSV mỏng, cần enrich |
| License | MIT rõ, attribution nhẹ | MIT nhưng scrape thêm rủi ro |
| Công sức | Tải → enrich Việt → import | + scrape + làm sạch |

Khuyến nghị **Option A**. T8.2 mở rộng PVO/TMRND tự biên theo mẫu T3.2 (đã có
24+12), không copy nguồn ngoài.

## 9. Sync (peer file-carry + merge LWW)

### Option A: peer đối xứng file-carry (khuyến nghị, theo oracle đã duyệt)

- Flow: backup → chép tay (USB/chat) → `POST /api/sync` merge; giữ restore hủy
  diệt như đường riêng.
- Migration v3: decks/cards +`guid` UNIQUE/+`updated_at`/＋`deleted` tombstone;
  reviews/notes +`guid`; `sync_meta` + `sync_conflicts`; backfill `guid=uuid`;
  trigger chạm `updated_at`; `google/uuid` đã có indirect.
- Merge 1 tx ATTACH incoming: decks/cards LWW theo `updated_at` + tombstone thắng,
  reviews append dedupe guid rồi recompute reps + replay SRS từ review mới nhất,
  notes union guid, bỏ qua dict/seed, cảnh báo lệch đồng hồ, cập nhật `last_sync_at`.
- T9.2: nút Đồng bộ + xem log xung đột trong Cài đặt (read-only).

### Option B: 1 máy làm server (push/pull qua mạng)

- Một máy expose API, máy kia push/pull trực tiếp, khỏi chép file tay.
- Giá: cần 2 máy cùng mạng + auth + TLS + discovery; single-user offline-first
  của v1 bị phá vỡ; scope phình to.

### Trade-offs

| Tiêu chí | A: file-carry (khuyến nghị) | B: 1 máy server |
|---|---|---|
| Hạ tầng | Không cần mạng chung | Cần LAN + auth + TLS |
| Offline | Giữ offline-first | Phụ thuộc máy chủ online |
| Code | 1 endpoint merge + UI log | + server/discovery/auth |
| Rủi ro | Clock-skew + id số trùng (đã có mitigation guid) | + bảo mật + NAT |

Khuyến nghị **Option A**: khớp triết lý single-user SQLite local của DECISIONS.md,
rủi ro chỉ còn clock-skew (cảnh báo) + id số trùng (merge theo guid).

## Câu hỏi cho user — đã duyệt (2026-09-23)

1. Model STT: **small** (CPU int8, ~1.5GB RAM).
2. Bundle Piper GPL + voice vào image (kèm offer source + attribution).
3. Sync UI chỉ xem log, không resolve tay.
4. Chạy cả 3 phases (7 → 8 → 9, cổng review giữa phases).
