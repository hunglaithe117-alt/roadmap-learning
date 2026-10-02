import { describe, it, expect } from 'vitest';
import { computeStreakDays } from './streak';

const D = (s: string) => new Date(`${s}T12:00:00Z`);

describe('test_streak_consecutive_days', () => {
  it('test_streak_counts_back_from_today', () => {
    expect(computeStreakDays(['2026-09-21', '2026-09-22', '2026-09-23'], D('2026-09-23'))).toBe(3);
  });

  it('test_streak_keeps_yesterday_when_today_missing', () => {
    expect(computeStreakDays(['2026-09-21', '2026-09-22'], D('2026-09-23'))).toBe(2);
  });

  it('test_streak_breaks_after_gap', () => {
    expect(computeStreakDays(['2026-09-20', '2026-09-23'], D('2026-09-23'))).toBe(1);
    expect(computeStreakDays(['2026-09-20'], D('2026-09-23'))).toBe(0);
  });
});
