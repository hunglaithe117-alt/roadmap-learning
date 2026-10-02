---
po_docs_branch: local-snapshot
po_docs_commit: non-git-2026-09-23
plan_revision: 4
last_updated: 2026-09-23
stack: web-v1
---

# Lang Learn App — Kế hoạch phân phase (WEB)

App học tiếng Anh + Trung cho người mới, chạy trên browser, offline-first ở mức dữ liệu học.

## Stack web-v1 (đã chốt)

- `web-v1: TypeScript Vite SPA + Go backend (net/http) + SQLite FTS5 modernc.org/sqlite`
- TTS Piper/Kokoro gọi qua backend (`os/exec` sidecar, stream audio về browser)
- STT faster-whisper sidecar (Python) sau backend, browser chỉ thu âm + upload
- Chạy browser nên không cần Wails/Tauri packaging; phân phối bằng Docker + static hosting
- SRS: ts-fsrs ở frontend, fallback lịch 1-3-7-14-30; pinyin: pinyin-pro

## Cấu trúc phases

| Phase | Thư mục | Mục tiêu (web) |
|---|---|---|
| 0 | `phases/phase-0-precheck/` | Spike Vite+Go API+SQLite, audio sidecar qua backend, license từ điển |
| 1 | `phases/phase-1-foundation/` | Web shell + API + SQLite (deck/card/note, FTS5, SRS) |
| 2 | `phases/phase-2-chinese/` | Tone engine + nét chữ + tone bingo trên web |
| 3 | `phases/phase-3-english/` | Stress engine + PVO/TMRND + checklist THIEU trên web |
| 4 | `phases/phase-4-player/` | Shadow player + recorder + error book (MediaRecorder + API) |
| 5 | `phases/phase-5-review/` | Reader + dict + dashboard + streak trên web |
| 6 | `phases/phase-6-release/` | Docker + deploy (thay packaging macOS/Linux), sync optional |
| 7 | `phases/phase-7-realaudio/` | TTS Piper thật (os/exec) + STT faster-whisper thật |
| 8 | `phases/phase-8-content/` | Seed HSK 2.0 L1-L4 (1200 từ) + mở rộng PVO/TMRND |
| 9 | `phases/phase-9-sync/` | Sync peer file-carry (migration v3 + merge LWW + UI log) |

## Tài liệu điều phối

- `phases/SOLUTION.md` — so sánh 2 options, khuyến nghị TS SPA + Go backend
- `phases/DECISIONS.md` — quyết định đã chốt (gồm pivot desktop → web 2026-09-23)
- `phases/USER_FEEDBACK.md` — phản hồi user
- `phases/OPEN_QUESTIONS.md` — câu hỏi mở
- `phases/task-memory/` — memory từng task sau khi chạy

## Nguồn học

- Trung: G0 pinyin/thanh điệu 4-6w, G1 HSK1 150 từ, G2 HSK2 300 từ, G3 HSK3 600 từ + graded reader + viết, G4 HSK4 1200 từ; daily 10p shadowing + 10p tone-pair + 10p minimal + SRS 1-3-7-14-30/FSRS.
- Anh: G0 METHODS, G1 NGONG S01-S24, G2 TAP (PVO 300-500, TMRND, WRITE WAY), G3 THIEU (8 trục A-H), G4 video+viết; daily shadowing+chunking+ghi âm+self-correction.
- Core chung: SRS + Recorder + ErrorBook + Review dashboard (weekly 30p / monthly 90p, streak, lỗi lặp).
# roadmap-learning
