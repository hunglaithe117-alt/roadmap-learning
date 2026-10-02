// Hình học bản đồ: khung, đường đi, thứ bậc node.
//
// Giá trị kỳ vọng lấy từ hằng số layout ở `domain/roadmap.ComputeLayout`
// (viewBox 0 0 1000 2000, lề 120) chứ không chạy lại hàm dưới test — xem
// `phases/task-memory/stack-v2-m2.md`.
import { describe, expect, it } from 'vitest';
import {
  FRAME_PADDING,
  LEVEL_STYLE,
  fitFrame,
  isPlayable,
  stageProgress,
  trailPath,
} from './geometry';

describe('test_fit_frame_wraps_server_points', () => {
  it('test_frame_adds_padding_on_all_four_sides', () => {
    const frame = fitFrame([{ x: 400, y: 1600 }, { x: 600, y: 1800 }]);
    expect(frame.minX).toBe(400 - FRAME_PADDING);
    expect(frame.maxY).toBe(1800 + FRAME_PADDING);
    expect(frame.width).toBe(200 + FRAME_PADDING * 2);
  });

  it('test_empty_stage_still_yields_a_usable_frame', () => {
    const frame = fitFrame([]);
    // viewBox 0×0 sẽ làm mọi phép scale chia cho 0.
    expect(frame.width).toBeGreaterThan(0);
    expect(frame.height).toBeGreaterThan(0);
  });

  it('test_single_node_frame_is_not_degenerate', () => {
    const frame = fitFrame([{ x: 500, y: 1000 }]);
    expect(frame.width).toBe(FRAME_PADDING * 2 + 1);
  });
});

describe('test_trail_path_follows_server_order', () => {
  it('test_path_starts_at_first_point', () => {
    expect(trailPath([{ x: 100, y: 900 }, { x: 120, y: 700 }])).toMatch(/^M 100\.0 900\.0/);
  });

  it('test_path_is_deterministic_for_same_input', () => {
    const pts = [
      { x: 10, y: 90 },
      { x: 30, y: 70 },
      { x: 50, y: 50 },
    ];
    expect(trailPath(pts)).toBe(trailPath(pts));
  });

  it('test_reversed_points_produce_different_path', () => {
    const pts = [
      { x: 10, y: 90 },
      { x: 30, y: 70 },
      { x: 50, y: 50 },
    ];
    // Đường phải PHẢN ÁNH thứ tự node: vẽ ngược sẽ ra đường đi chạy lùi.
    expect(trailPath(pts)).not.toBe(trailPath([...pts].reverse()));
  });

  it('test_no_points_yields_empty_path', () => {
    expect(trailPath([])).toBe('');
  });
});

describe('test_node_level_visibility', () => {
  it('test_locked_node_is_not_playable', () => {
    expect(isPlayable('LOCKED')).toBe(false);
  });

  it('test_current_and_done_nodes_are_playable', () => {
    expect(isPlayable('CURRENT')).toBe(true);
    expect(isPlayable('DONE')).toBe(true);
  });

  it('test_only_current_node_has_animated_halo', () => {
    // Vòng hào động trên node khoá sẽ giật mà không mở được gì.
    expect(LEVEL_STYLE.CURRENT.halo).toBe(true);
    expect(LEVEL_STYLE.LOCKED.halo).toBe(false);
    expect(LEVEL_STYLE.DONE.halo).toBe(false);
  });
});

describe('test_stage_progress', () => {
  it('test_optional_topics_are_excluded_from_the_denominator', () => {
    const p = stageProgress([
      { status: 'DONE', isOptional: false },
      { status: 'DONE', isOptional: true },
      { status: 'NOT_STARTED', isOptional: true },
    ]);
    expect(p.total).toBe(1);
    expect(p.pct).toBe(1);
  });

  it('test_skipped_counts_as_done', () => {
    const p = stageProgress([
      { status: 'SKIPPED', isOptional: false },
      { status: 'NOT_STARTED', isOptional: false },
    ]);
    expect(p.done).toBe(1);
    expect(p.pct).toBe(0.5);
  });

  it('test_stage_with_only_optional_topics_reports_zero_not_nan', () => {
    expect(stageProgress([{ status: 'DONE', isOptional: true }]).pct).toBe(0);
  });
});
