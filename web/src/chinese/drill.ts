// T2.1 — logic vòng drill tone-pair (thuần, test được không cần DOM).
import type { DrillItem } from './api';

export interface RoundResult {
  total: number;
  correct: number;
  pct: number;
}

/** Tóm tắt 1 vòng drill từ mảng đúng/sai. */
export function summarizeRound(results: boolean[]): RoundResult {
  const total = results.length;
  const correct = results.filter(Boolean).length;
  return { total, correct, pct: total === 0 ? 0 : correct / total };
}

/** Gợi ý grade SRS cho cả vòng (đủ tốt -> Được, xuất sắc -> Dễ). */
export function roundGrade(pct: number): number {
  if (pct >= 0.9) return 4;
  if (pct >= 0.6) return 3;
  if (pct > 0) return 2;
  return 1;
}

/** Lấy các item drill còn lại (bỏ item vừa làm).
 * GHI CHÚ (D4): không có helper chuẩn hoá input ở client — server
 * `gradeTone` (ParseToneSequence) là source-of-truth. */
export function remainingItems(items: DrillItem[], doneIds: string[]): DrillItem[] {
  const done = new Set(doneIds);
  return items.filter((it) => !done.has(it.card_id));
}
