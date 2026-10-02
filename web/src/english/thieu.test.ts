import { describe, it, expect } from 'vitest';
import { thieuAverage, thieuTrend, type ThieuSession } from './thieu';

// `id` là chuỗi ở M5: `ID` của GraphQL là `String` còn cột DB là `bigint` —
// ép sang `number` là mất chính xác.
function session(id: string, sess: string, scores: Record<string, number>): ThieuSession {
  return { id, session: sess, scores, average: thieuAverage(scores), note: '', created_at: sess };
}

describe('test_thieu_scoring', () => {
  it('test_thieu_average_8_axes_expected_mean', () => {
    const avg = thieuAverage({ A: 3, B: 4, C: 2, D: 3, E: 4, F: 3, G: 2, H: 5 });
    expect(avg).toBeCloseTo(3.25);
  });

  it('test_thieu_average_empty_scores_returns_zero', () => {
    expect(thieuAverage({})).toBe(0);
  });

  it('test_thieu_trend_orders_oldest_first', () => {
    const newer = session('2', '2026-09-23', { A: 4, B: 4, C: 4, D: 4, E: 4, F: 4, G: 4, H: 4 });
    const older = session('1', '2026-09-16', { A: 2, B: 2, C: 2, D: 2, E: 2, F: 2, G: 2, H: 2 });
    const trend = thieuTrend([newer, older]);
    expect(trend['A']).toEqual([2, 4]);
  });
});
