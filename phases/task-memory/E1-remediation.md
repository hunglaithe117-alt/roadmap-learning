# E1-remediation — 6 fix sau cổng E1 (2026-09-23)

1. `docker-compose.yml`: service `stt` sang `profiles: ["stt"]`, app bỏ `depends_on` cứng; `WHISPER_URL` mặc định rỗng (`${WHISPER_URL:-}`) → stub, muốn STT thật chạy `WHISPER_URL=http://stt:9000/asr docker compose --profile stt up`.
2. Pin: image stt `onerahmet/openai-whisper-asr-webservice:latest@sha256:0616c3d5fc6924e9d59aa8a7a3a944fbe8f3a5d701e5fc1fa2ce5b933dfecb73` (multi-arch index tag latest, pushed 2026-08-09); 2 voice Piper pin HF commit `c10ece1aade47bb51c153c893d14e5bf8e5b7117` qua `ARG PIPER_VOICES_COMMIT` (thay `resolve/main`).
3. Phồn→giản: `api/simplify.go` (bảng map tối thiểu pure-Go, KHÔNG cgo/không dep mới), adapter `TranscribeDetailed` convert text + từng segment trước join + words, `STTHandler` normalize mọi engine; mirror `toSimplified` trong `web/src/player/diff.ts` (wordDiff normalize 2 vế). Test: `e1_remediation_test.go` (phồn→giản text/segments/words/handler) + `diff.test.ts` (toSimplified + diff phồn-giản khớp).
4. TTS cache: chọn **LRU đuổi cũ nhất** (giữ cap 200, `Get` chạm key, `Put` key mới đầy thì đuổi `order[0]`), thay drop-all cũ; giữ `Cache-Control: no-store`. Test: `Test_e1_tts_cache_lru_evicts_oldest` + `..._update_keeps_size`.
5. Badge X-Engine (đọc header sẵn có, không thêm API): `uploadSTT` capture `X-Engine` → `engineHeader`, helper mới `fetchTTSEngine`; badge hiện ở drill STT (`Recorder.tsx`), drill pinyin (`ToneCard` trong `ZhPinyin.tsx`) và Cài đặt (`CaiDat.tsx` TTS + STT).
6. `THIRD-PARTY-LICENSES`: mục 3 thêm digest sidecar đã pin + link repo onerahmet; mục 4 mới ghi license `torch` (BSD-3-Clause) + `pyannote.audio` (MIT) trong image sidecar.

Validation: `go test -count=1 ./...` pass, `pnpm vitest run` 20 files/101 tests pass, `tsc --noEmit` pass, `docker compose [--profile stt] config` hợp lệ.
