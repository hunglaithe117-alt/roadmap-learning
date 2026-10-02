import { describe, it, expect } from 'vitest';
import { clampRate, normalizeAB, loopTick, restoreIndex } from './abloop';

describe('test_abloop_rate_clamp', () => {
  it('test_clamp_rate_keeps_valid_speed', () => {
    expect(clampRate(0.8)).toBe(0.8);
  });

  it('test_clamp_rate_clips_out_of_range', () => {
    expect(clampRate(0.1)).toBe(0.5);
    expect(clampRate(2)).toBe(1.5);
    expect(clampRate(NaN)).toBe(1);
  });
});

describe('test_abloop_normalize', () => {
  it('test_normalize_ab_swaps_reversed_points', () => {
    expect(normalizeAB(3.4, 1.2, 10)).toEqual({ a: 1.2, b: 3.4 });
  });

  it('test_normalize_ab_clamps_to_duration', () => {
    expect(normalizeAB(-1, 99, 10)).toEqual({ a: 0, b: 10 });
  });

  it('test_normalize_ab_rejects_too_short_loop', () => {
    expect(normalizeAB(1.0, 1.05, 10)).toBeNull();
  });
});

describe('test_abloop_tick', () => {
  it('test_loop_tick_seeks_back_at_b', () => {
    expect(loopTick(3.35, { a: 1.2, b: 3.4 })).toBe(1.2);
  });

  it('test_loop_tick_continues_before_b', () => {
    expect(loopTick(2.0, { a: 1.2, b: 3.4 })).toBeNull();
    expect(loopTick(2.0, null)).toBeNull();
  });

  it('test_restore_index_clamps_saved_position', () => {
    expect(restoreIndex('7', 5)).toBe(4);
    expect(restoreIndex(null, 5)).toBeNull();
    expect(restoreIndex('abc', 5)).toBeNull();
  });
});
