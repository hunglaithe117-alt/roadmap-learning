/**
 * Nhận diện cử chỉ vuốt — thuần, không DOM, không chuẩn hoá sự kiện.
 *
 * Ba cử chỉ của bản đồ TÁCH theo vị trí (ROADMAP-MAP-IDEA §4), nên phần
 * "vuốt có phải vuốt không" phải quyết định được mà không cần biết vuốt ở
 * đâu — chỗ nào quyết định là việc của component.
 *
 *   vuốt trên bản đồ   → cuộn bản đồ theo `direction` của map
 *   chạm node          → mở panel (không phải vuốt)
 *   vuốt trong panel   → đánh dấu Xong
 *
 * Ngưỡng 80px lấy đúng con số §4. Ngưỡng trục 12px chặn cái run tay khi
 * bấm chuột: dưới ngưỡng đó coi như chạm, không phải vuốt.
 */

export type Axis = 'x' | 'y';

export interface DragSample {
  dx: number;
  dy: number;
}

/** Trục chiếm ưu thế, `null` khi cử động quá nhỏ để gọi là vuốt. */
export function dominantAxis({ dx, dy }: DragSample, noise = 12): Axis | null {
  const ax = Math.abs(dx);
  const ay = Math.abs(dy);
  if (Math.max(ax, ay) < noise) return null;
  return ax > ay ? 'x' : 'y';
}

/** Vuốt đúng trục đã chốt, quá ngưỡng, và dấu chưa đổi (không lưỡng tính). */
export function isSwipe(sample: DragSample, axis: Axis, threshold = 80): boolean {
  const delta = axis === 'x' ? sample.dx : sample.dy;
  const across = axis === 'x' ? sample.dy : sample.dx;
  if (Math.abs(across) > Math.abs(delta)) return false;
  return Math.abs(delta) >= threshold;
}

/**
 * Cử chỉ của 1 chặng bản đồ: trục chính là trục `direction` (vuốt dọc khi
 * `up`, vuốt ngang khi `right`), trục phụ bị khoá — vuốt sai chiều chỉ là
 * cử chỉ rác, không cuộn và cũng không đánh dấu.
 */
export function swipeAxisFor(horizontal: boolean): Axis {
  return horizontal ? 'x' : 'y';
}
