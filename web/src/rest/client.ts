// M5 — client cho 5 endpoint REST còn lại. KHÔNG có endpoint JSON nào khác:
// mọi thứ JSON đã sang GraphQL (xem `internal/transport/http/routes.go` của
// app-v2 — cố ý không có để client không quay lại gọi tuần tự).
//
//   GET  /api/health    ping DB + trạng thái engine audio
//   GET  /api/tts       stream WAV + header `X-Engine`
//   POST /api/stt       multipart `audio` → JSON transcript
//   GET  /api/backup    501 cho tới M7 (`pg_dump` chưa có trong image)
//   POST /api/restore   501 cho tới M7
//
// Hợp đồng lỗi của cả 5 endpoint là `{"error": "<tiếng Việt>"}` (xem
// `internal/transport/http/errors.go` — `writeJSONError`), giống hệt app v1 ⇒
// client đọc lỗi y hệt cũ, không phải dịch lại.
import { AppError } from '../graphql/errors';
import { getApiBase } from '../lib/apiBase';

/** Kết quả 1 lần nhận dạng giọng nói — 5 field của `sttResponse` ở app-v2. */
export interface STTResult {
  transcript: string;
  lang: string;
  engine: string;
  /** Header `X-Engine` đọc thẳng từ response (badge E1, không thêm request nào). */
  engineHeader?: string;
  words?: Array<{ word: string; start?: number; end?: number; confidence?: number }>;
  confidence?: number;
}

export interface HealthResult {
  status: string;
}

/**
 * Mã lỗi cho `AppError.code`. Ở tầng HTTP không có enum `ErrorCode` của
 * GraphQL nên chỉ giữ vài mã có ý nghĩa; **message** mới là thứ hiển thị lên UI.
 */
function httpCode(status: number): string {
  if (status === 400) return 'BAD_REQUEST';
  if (status === 501) return 'NOT_IMPLEMENTED';
  if (status === 404) return 'NOT_FOUND';
  return 'INTERNAL';
}

/**
 * Ném `AppError` mang message NGUYÊN VĂN của server khi response lỗi.
 * `fallback` chỉ dùng khi server KHÔNG trả lời được (mạng hỏng, body hỏng) —
 * lúc đó mới rơi về thông điệp của client. Đây là cùng nguyên tắc với
 * `graphql/errors.ts`: đoán bừa nội dung lỗi là cách chắc chắn hiện sai.
 */
async function throwIfNotOk(res: Response, fallback: string): Promise<void> {
  if (res.ok) return;
  const body = await res.json().catch(() => null);
  const fromServer = body && typeof body.error === 'string' ? body.error : null;
  throw new AppError(fromServer ?? fallback, httpCode(res.status), { cause: body });
}

export function buildTTSUrl(text: string, base = getApiBase()): string {
  return `${base}/api/tts?text=${encodeURIComponent(text)}`;
}

export function buildBackupUrl(base = getApiBase()): string {
  return `${base}/api/backup`;
}

export async function fetchHealth(base = getApiBase()): Promise<HealthResult> {
  const res = await fetch(`${base}/api/health`);
  await throwIfNotOk(res, `health ${res.status}`);
  return (await res.json()) as HealthResult;
}

/** `X-Engine` đọc từ header, không tải audio về (badge drill / Cài đặt). */
export async function fetchTTSEngine(text: string, base = getApiBase()): Promise<string> {
  const res = await fetch(buildTTSUrl(text, base));
  const header = res.headers?.get?.('X-Engine') ?? null;
  try {
    await res.body?.cancel?.();
  } catch {
    /* bỏ qua — huỷ body chỉ là tối ưu, lỗi ở đây không đáng báo */
  }
  return header ?? 'unknown';
}

export async function uploadSTT(audio: Blob, base = getApiBase()): Promise<STTResult> {
  const form = new FormData();
  form.append('audio', audio, 'recording.webm');
  const res = await fetch(`${base}/api/stt`, { method: 'POST', body: form });
  await throwIfNotOk(res, `stt ${res.status}`);
  const data = (await res.json()) as STTResult;
  const header = res.headers?.get?.('X-Engine') ?? null;
  return { ...data, engineHeader: header ?? data.engine };
}

/**
 * Tải backup. **Trả 501 tới M7** — app-v2 chưa implement `BackupPort` (Postgres
 * không có `VACUUM INTO`, cần `pg_dump`/`pg_restore` thuộc M7). Hàm này ném
 * `AppError` mang message tiếng Việt của server; UI phải hiện đúng message đó
 * chứ không được báo "đã tải xong".
 */
export async function downloadBackup(base = getApiBase()): Promise<Blob> {
  const res = await fetch(buildBackupUrl(base));
  await throwIfNotOk(res, `backup ${res.status}`);
  return res.blob();
}

export interface RestoreResult {
  ok: boolean;
  restored?: number;
  restored_tables?: string[];
}

/** Nạp lại snapshot. **Trả 501 tới M7** — xem `downloadBackup`. */
export async function uploadRestore(file: File | Blob, base = getApiBase()): Promise<RestoreResult> {
  const form = new FormData();
  form.append('file', file, 'langapp-backup.dump');
  const res = await fetch(`${base}/api/restore`, { method: 'POST', body: form });
  await throwIfNotOk(res, `restore ${res.status}`);
  return (await res.json()) as RestoreResult;
}
