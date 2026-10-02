// T2.2 — stroke order: lazy-load dữ liệu nét theo level + chấm đúng/sai cơ bản.
// Canvas ghi mỗi nét vẽ thành chuỗi điểm; classifyStroke đoán mã nét theo
// hướng chủ đạo (ngang/sổ/phẩy/mác/chấm). gradeStrokes so thứ tự mã nét.

export interface Point {
  x: number;
  y: number;
}

export interface StrokeStep {
  order: number;
  code: string;
  name: string;
}

export interface StrokeDetail {
  hanzi: string;
  pinyin_marks: string;
  level: string;
  stroke_count: number;
  strokes: StrokeStep[];
}

/** Đoán mã nét từ hướng chủ đạo của 1 nét vẽ. */
export function classifyStroke(points: Point[]): string {
  if (points.length < 2) return 'd';
  const first = points[0];
  const last = points[points.length - 1];
  const dx = last.x - first.x;
  const dy = last.y - first.y;
  const dist = Math.hypot(dx, dy);
  if (dist < 6) return 'd'; // chấm: gần như không di chuyển
  const adx = Math.abs(dx);
  const ady = Math.abs(dy);
  if (adx > ady * 2) return 'h'; // ngang
  if (ady > adx * 2) return 's'; // sổ
  if (dx < 0 && dy > 0) return 'p'; // phẩy (xuống-trái)
  return 'n'; // mác (xuống-phải)
}

export interface StrokeGrade {
  score: number;
  pass: boolean;
  matches: number;
  expected: number;
}

/** So thứ tự mã nét vẽ với chuẩn. pass khi khớp >=80% và đủ số nét. */
export function gradeStrokes(expected: string[], drawn: string[]): StrokeGrade {
  const matches = expected.filter((code, i) => drawn[i] === code).length;
  const score = expected.length === 0 ? 0 : matches / expected.length;
  const pass = drawn.length === expected.length && score >= 0.8;
  return { score, pass, matches, expected: expected.length };
}

/** Mã chuẩn rút gọn từ detail API. */
export function expectedCodes(detail: StrokeDetail): string[] {
  return detail.strokes.map((s) => s.code);
}

/**
 * Tìm `card_id` theo mặt chữ để lưu điểm viết vào SRS (màn nét chữ chấm đúng
 * thì ghi 1 lần ôn cho thẻ tương ứng). Id là chuỗi vì `ID` của GraphQL là
 * `String` còn cột DB là `bigint` — ép sang `number` là mất chính xác.
 */
export function findCardForHanzi(
  cards: ReadonlyArray<{ id: string; front: string }>,
  hanzi: string,
): string | null {
  const c = cards.find((x) => x.front === hanzi);
  return c ? c.id : null;
}
