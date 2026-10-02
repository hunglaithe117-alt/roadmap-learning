// Lực confetti — thuần, kiểm được không cần mở trình duyệt.
import { describe, expect, it } from 'vitest';
import { burst, step } from './confetti';

describe('test_burst', () => {
  it('test_burst_produces_requested_piece_count', () => {
    expect(burst({ count: 20, width: 400, height: 300 })).toHaveLength(20);
  });

  it('test_burst_starts_at_the_centre', () => {
    for (const p of burst({ count: 10, width: 400, height: 300 })) {
      expect(p.x).toBe(200);
      expect(p.y).toBe(150);
    }
  });

  it('test_pieces_fly_upwards_not_downwards', () => {
    // Vụ nổ pháo giấy: `vy` âm = đi lên. Nếu có mảnh đi xuống, hiệu ứng trông
    // như mảnh rơi khỏi bản đồ.
    for (const p of burst({ count: 40, width: 400, height: 300 })) {
      expect(p.vy).toBeLessThan(0);
    }
  });

  it('test_same_seed_produces_identical_burst', () => {
    const a = burst({ count: 12, seed: 5, width: 400, height: 300 });
    const b = burst({ count: 12, seed: 5, width: 400, height: 300 });
    expect(a).toEqual(b);
  });

  it('test_different_seeds_produce_different_burst', () => {
    const a = burst({ count: 12, seed: 1, width: 400, height: 300 });
    const b = burst({ count: 12, seed: 2, width: 400, height: 300 });
    expect(a).not.toEqual(b);
  });
});

describe('test_step', () => {
  it('test_gravity_increases_downward_velocity', () => {
    const [p] = burst({ count: 1, width: 400, height: 300 });
    const [next] = step([p], 0.1);
    // Mảnh bắn lên nên `vy` âm; trọng lực làm nó dồn dần về 0 rồi dương.
    // Kiểm `vy` tăng chứ không kiểm `y` vì 0.1s đầu mảnh vẫn đang bay lên.
    expect(p.vy).toBeLessThan(0);
    expect(next.vy).toBeGreaterThan(p.vy);
  });

  it('test_horizontal_velocity_is_unchanged_by_gravity', () => {
    const [p] = burst({ count: 1, width: 400, height: 300 });
    expect(step([p], 0.3)[0].vx).toBe(p.vx);
  });

  it('test_expired_pieces_are_dropped', () => {
    const [p] = burst({ count: 1, width: 400, height: 300 });
    expect(step([p], p.life + 0.1)).toEqual([]);
  });

  it('test_step_does_not_mutate_input', () => {
    const pieces = burst({ count: 3, width: 400, height: 300 });
    const snapshot = JSON.parse(JSON.stringify(pieces));
    step(pieces, 0.2);
    expect(pieces).toEqual(snapshot);
  });
});
