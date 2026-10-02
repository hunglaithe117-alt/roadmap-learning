# DECISIONS

| Ngày | Quyết định | Lý do |
|---|---|---|
| 2026-09-23 | Stack v1: Go + Wails v2.12 + TS Vite | Dev nhanh, pure-Go SQLite |
| 2026-09-23 | SQLite FTS5 qua `modernc.org/sqlite` | Offline-first, không cgo |
| 2026-09-23 | SRS dùng ts-fsrs, fallback lịch 1-3-7-14-30 | Khớp routine Trung/Anh |
| 2026-09-23 | Pinyin dùng pinyin-pro | Không cần backend riêng |
| 2026-09-23 | TTS Piper/Kokoro, STT faster-whisper/whisper.cpp sidecar | Offline hoàn toàn |
| 2026-09-23 | Fallback Rust + Tauri v2.11 | Ghi nhận, chưa triển khai |
| 2026-09-23 | Pivot desktop (Go+Wails) → web (TS Vite SPA + Go backend net/http) | Chạy browser, bỏ packaging Wails/Tauri; deploy Docker, TTS/STT qua backend sidecar |
| 2026-09-23 | TTS v1: Piper (Kokoro optional) | Nhẹ <50MB/voice, CPU realtime, sidecar đơn giản; user ủy quyền tự chốt |
| 2026-09-23 | STT v1: faster-whisper sidecar HTTP riêng (whisper.cpp fallback) | INT8 CPU, backend Go gọi qua HTTP; user ủy quyền tự chốt |
| 2026-09-23 | Auth/sync v1: không — single-user SQLite local | Export/import + sync để phase-6 optional; user ủy quyền tự chốt |
| 2026-09-23 | v2 duyệt cả 3 phases (7 audio thật, 8 nội dung, 9 sync) | User duyệt qua question gate sau SOLUTION-v2 |
| 2026-09-23 | STT model: small (CPU int8) | WER Trung tốt, ~1.5GB RAM; user chọn |
| 2026-09-23 | Bundle Piper GPL + voice vào image | 1 lệnh offline, kèm offer source + attribution; user chọn |
| 2026-09-23 | Sync UI chỉ xem log | Merge tự động, không nút resolve tay; user chọn |
