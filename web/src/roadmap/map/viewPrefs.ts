/**
 * Bộ nhớ cục bộ của bản đồ: chế độ xem, vị trí đang xem, bật/tắt confetti.
 *
 * Vì sao phải nhớ: bản đồ 1 chặng dài hàng nghìn đơn vị viewBox, mỗi lần
 * vào lại phải cuộn từ đầu thì không ai dùng lâu. Khoá theo
 * `roadmap:<slug>:<view>` (ROADMAP-MAP-IDEA §6) + ghi kèm `stageId` vì mỗi
 * chặng là 1 bản đồ riêng, chặng nào đang mở cũng là thứ cần nhớ.
 *
 * Mọi hàm đều nuốt lỗi: `localStorage` hỏng (chế độ riêng tư, quota) thì bản
 * đồ vẫn phải chạy, chỉ mất tiện.
 */

const VIEW_KEY = 'roadmap:view';
const POSITION_PREFIX = 'roadmap:';
const CONFETTI_KEY = 'roadmap:confetti';

export type ViewMode = 'map' | 'list';

export interface MapPosition {
  stageId: string;
  /** Dịch chuyển trên TRỤC CHÍNH của map (`up` = Y, `right` = X), px. */
  offset: number;
}

function read(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function write(key: string, value: string): void {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* storage unavailable */
  }
}

/**
 * Chế độ xem đã chọn. Chưa chọn lần nào ⇒ `map` (lần đầu vào phải thấy bản
 * đồ, không phải bảng — ROADMAP-MAP-IDEA §7.6). Giá trị rác trong storage
 * cũng rơi về `map` thay vì làm trắng màn.
 */
export function readView(): ViewMode {
  return read(VIEW_KEY) === 'list' ? 'list' : 'map';
}

export function writeView(mode: ViewMode): void {
  write(VIEW_KEY, mode);
}

export function positionKey(slug: string, view: ViewMode): string {
  return `${POSITION_PREFIX}${slug}:${view}`;
}

export function readPosition(slug: string, view: ViewMode): MapPosition | null {
  const raw = read(positionKey(slug, view));
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw) as Partial<MapPosition>;
    if (typeof parsed.stageId !== 'string') return null;
    const offset = typeof parsed.offset === 'number' && Number.isFinite(parsed.offset) ? parsed.offset : 0;
    return { stageId: parsed.stageId, offset };
  } catch {
    return null;
  }
}

export function writePosition(slug: string, view: ViewMode, value: MapPosition): void {
  write(positionKey(slug, view), JSON.stringify(value));
}

/** Confetti mặc định bật; người dùng tắt ở Cài đặt thì tắt hẳn. */
export function readConfettiEnabled(): boolean {
  return read(CONFETTI_KEY) !== '0';
}

export function writeConfettiEnabled(on: boolean): void {
  write(CONFETTI_KEY, on ? '1' : '0');
}

/**
 * `prefers-reduced-motion: reduce` của hệ điều hành. Không có `matchMedia`
 * (môi trường test cũ) ⇒ coi như KHÔNG giảm chuyển động — nhưng vòng lặp
 * confetti vẫn tự dừng, nên hậu quả chỉ là nhiều khung hình vô nghĩa.
 */
export function prefersReducedMotion(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false;
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/** Confetti có chạy không: cả 2 điều kiện phải cho phép (ROADMAP-MAP-IDEA §6). */
export function confettiAllowed(): boolean {
  return readConfettiEnabled() && !prefersReducedMotion();
}
