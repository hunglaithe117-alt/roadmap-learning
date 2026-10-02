/**
 * Hình học bản đồ: khung, đường đi qua các node, bán kính node.
 *
 * Toạ độ node ĐẾN TỪ SERVER (`Topic.point` = `MapPoint{x,y}`, tính bởi
 * `domain/roadmap.ComputeLayout` trong viewBox 0 0 1000 2000). File này KHÔNG
 * sinh lại toạ độ — nó chỉ làm 3 việc hiển thị: (1) bọc toạ độ đó vào khung
 * vừa nội dung, (2) nối thành đường, (3) chọn bán kính theo trạng thái.
 *
 * Bọc khung theo nội dung (thay vì cứng 1000×2000) là vì một chặng seed có 5
 * node còn chặng cuối có 8; cứng viewBox thì chặng 5 node nằm giữa trang trắng
 * và người dùng phải cuộn tìm. Tỉ lệ giữa 2 cạnh KHÔNG đổi, chỉ phóng cả khung.
 */

export interface MapPoint {
  x: number;
  y: number;
}

export interface Rect {
  minX: number;
  minY: number;
  maxX: number;
  maxY: number;
  width: number;
  height: number;
}

/** Lề an toàn quanh node ngoài cùng — chừa chỗ cho nhãn và vòng hào động. */
export const FRAME_PADDING = 96;

/** Bán kính node (đơn vị viewBox). Node phụ nhỏ hơn để thứ bậc thị giác rõ. */
export const NODE_RADIUS = 34;
export const NODE_RADIUS_OPTIONAL = 24;
export const MILESTONE_RADIUS = 15;

/**
 * Khung vừa nội dung. `Math.max(…, 1)` chặn stage không có node trả về khung
 * 0×0 ⇒ `viewBox` vô nghĩa và mọi phép scale chia cho 0.
 */
export function fitFrame(points: readonly MapPoint[], padding = FRAME_PADDING): Rect {
  if (points.length === 0) {
    return { minX: 0, minY: 0, maxX: 1, maxY: 1, width: 1, height: 1 };
  }
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (const p of points) {
    minX = Math.min(minX, p.x);
    minY = Math.min(minY, p.y);
    maxX = Math.max(maxX, p.x);
    maxY = Math.max(maxY, p.y);
  }
  // Cả 4 cạnh đều lùi vào lề — `maxX`/`maxY` là góc ĐÃ lùi, không phải toạ độ
  // node. Trả toạ độ thô ở đây là cái bẫy: `<svg viewBox>` lấy min + width, nên
  // max sai là khung lệch và node cuối bị cắt mất nhãn.
  return {
    minX: minX - padding,
    minY: minY - padding,
    maxX: maxX + padding,
    maxY: maxY + padding,
    width: Math.max(maxX - minX, 1) + padding * 2,
    height: Math.max(maxY - minY, 1) + padding * 2,
  };
}

/**
 * Đường đi qua các node — Catmull-Rom chuyển sang cubic Bézier.
 *
 * Vì sao bo cong chứ không nối thẳng: đường thẳng giữa 2 node là gấp góc,
 * mà bản đồ có cảm giác đi theo con đường. Cùng toạ độ phải ra cùng `d` —
 * hàm thuần, không đọc trạng thái render.
 */
export function trailPath(points: readonly MapPoint[]): string {
  if (points.length === 0) return '';
  if (points.length === 1) {
    const p = points[0];
    return `M ${fmt(p.x)} ${fmt(p.y)} L ${fmt(p.x + 0.01)} ${fmt(p.y)}`;
  }
  if (points.length === 2) {
    return `M ${fmt(points[0].x)} ${fmt(points[0].y)} L ${fmt(points[1].x)} ${fmt(points[1].y)}`;
  }
  let d = `M ${fmt(points[0].x)} ${fmt(points[0].y)}`;
  for (let i = 0; i < points.length - 1; i += 1) {
    const p0 = points[i - 1] ?? points[i];
    const p1 = points[i];
    const p2 = points[i + 1];
    const p3 = points[i + 2] ?? p2;
    d += ` C ${fmt(p1.x + (p2.x - p0.x) / 6)} ${fmt(p1.y + (p2.y - p0.y) / 6)}` +
      ` ${fmt(p2.x - (p3.x - p1.x) / 6)} ${fmt(p2.y - (p3.y - p1.y) / 6)}` +
      ` ${fmt(p2.x)} ${fmt(p2.y)}`;
  }
  return d;
}

function fmt(n: number): string {
  return Number.isFinite(n) ? n.toFixed(1) : '0';
}

/** Trạng thái node do server gán (`LevelState`), dùng nguyên văn. */
export type NodeLevel = 'DONE' | 'CURRENT' | 'LOCKED';

/** Node `locked` không bấm được (ROADMAP-MAP-IDEA §3). */
export function isPlayable(level: NodeLevel): boolean {
  return level !== 'LOCKED';
}

export interface LevelStyle {
  fill: string;
  stroke: string;
  text: string;
  /** Node sáng có vòng hào động quanh mình. */
  halo: boolean;
  label: string;
}

export const LEVEL_STYLE: Record<NodeLevel, LevelStyle> = {
  DONE: { fill: 'var(--color-jade)', stroke: '#9fd6c1', text: '#f2fbf7', halo: false, label: 'Đã xong' },
  CURRENT: { fill: 'var(--color-ember)', stroke: '#ffd0b8', text: '#fff6f1', halo: true, label: 'Đang học' },
  LOCKED: { fill: 'var(--color-locked)', stroke: '#d9d2c0', text: '#4c4a43', halo: false, label: 'Chưa mở' },
};

/** Tiến độ 1 chặng: node bắt buộc là mẫu số, node tham khảo không tính. */
export interface StageProgress {
  done: number;
  total: number;
  pct: number;
}

export function stageProgress(
  topics: ReadonlyArray<{ status: string; isOptional: boolean }>,
): StageProgress {
  const required = topics.filter((t) => !t.isOptional);
  const done = required.filter((t) => t.status === 'DONE' || t.status === 'SKIPPED').length;
  return {
    done,
    total: required.length,
    pct: required.length === 0 ? 0 : done / required.length,
  };
}
