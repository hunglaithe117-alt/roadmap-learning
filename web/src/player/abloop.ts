// T4.1 — helper thuần cho shadowing player A-B loop (HTMLAudio).
// Không chạm DOM: nhận số giây, trả quyết định seek. Dung sai ±0.1s cho A-B
// vì HTMLAudio seek kém chính xác. Tốc độ kẹp 0.5x–1.5x (server cũng validate).

export const MIN_RATE = 0.5;
export const MAX_RATE = 1.5;
/** Dung sai seek A-B (giây). */
export const AB_TOLERANCE = 0.1;

export interface ABLoop {
  a: number;
  b: number;
}

/** Kẹp tốc độ phát vào 0.5x–1.5x. */
export function clampRate(rate: number): number {
  if (!Number.isFinite(rate)) return 1;
  return Math.min(MAX_RATE, Math.max(MIN_RATE, rate));
}

/** Chuẩn hoá điểm A-B: đảo khi a>b, kẹp vào [0, duration], bỏ loop khi quá ngắn. */
export function normalizeAB(a: number, b: number, duration: number): ABLoop | null {
  if (!Number.isFinite(a) || !Number.isFinite(b) || !Number.isFinite(duration) || duration <= 0) {
    return null;
  }
  let lo = Math.min(a, b);
  let hi = Math.max(a, b);
  lo = Math.min(Math.max(lo, 0), duration);
  hi = Math.min(Math.max(hi, 0), duration);
  if (hi - lo < AB_TOLERANCE) return null;
  return { a: lo, b: hi };
}

/**
 * Quyết định seek cho 1 tick timeupdate: khi currentTime chạm/vượt b
 * (trừ dung sai) thì nhảy về a. Trả vị trí seek tới, null khi cứ phát tiếp.
 */
export function loopTick(currentTime: number, loop: ABLoop | null): number | null {
  if (!loop) return null;
  if (currentTime >= loop.b - AB_TOLERANCE) return loop.a;
  return null;
}

/**
 * Key lưu vị trí playlist (resume sau reload).
 *
 * `deckId` là **chuỗi** ở M5: `ID` của GraphQL là `String` còn cột DB là
 * `bigint`. Khoá có tiền tố `shadow-index:` nên đổi kiểu không phá dữ liệu
 * localStorage của phiên bản cũ (`"shadow-index:3"` vẫn đọc được).
 */
export function progressKey(deckId: string | number): string {
  return `shadow-index:${deckId}`;
}

/** Đọc index đã lưu, kẹp vào [0, total-1]; null khi chưa có gì lưu. */
export function restoreIndex(raw: string | null, total: number): number | null {
  if (raw == null || total <= 0) return null;
  const i = Number.parseInt(raw, 10);
  if (!Number.isFinite(i) || i < 0) return null;
  return Math.min(i, total - 1);
}
