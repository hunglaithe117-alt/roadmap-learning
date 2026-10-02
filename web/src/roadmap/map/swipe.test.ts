// Ranh giới cử chỉ (ROADMAP-MAP-IDEA §4) — thuần, không DOM.
//
// Ba hàm này là nơi quyết định 3 cử chỉ có tách vị trí hay không. Test ở đây
// khẳng định ranh giới đó bằng con số; test ở `MapCanvas.test.ts` xác nhận
// mỗi cử chỉ thật sự gắn vào đúng vị trí.
import { describe, expect, it } from 'vitest';
import { dominantAxis, isSwipe, swipeAxisFor } from './swipe';

describe('test_dominant_axis', () => {
  it('test_tiny_movement_is_not_an_axis', () => {
    expect(dominantAxis({ dx: 3, dy: -2 })).toBeNull();
  });

  it('test_vertical_dominant_movement_is_y', () => {
    expect(dominantAxis({ dx: 10, dy: -90 })).toBe('y');
  });

  it('test_horizontal_dominant_movement_is_x', () => {
    expect(dominantAxis({ dx: 90, dy: 10 })).toBe('x');
  });

  it('test_exact_tie_falls_back_to_vertical', () => {
    // Vuốt chéo 45° phải rơi về 1 trục xác định, không trả null — nếu null thì
    // cử chỉ bị bỏ âm thầm ở ranh giới giữa dọc và ngang.
    expect(dominantAxis({ dx: 80, dy: -80 })).toBe('y');
  });
});

describe('test_is_swipe', () => {
  it('test_swipe_up_beyond_80px_is_a_swipe', () => {
    expect(isSwipe({ dx: 0, dy: -81 }, 'y')).toBe(true);
  });

  it('test_movement_just_under_threshold_is_not_a_swipe', () => {
    expect(isSwipe({ dx: 0, dy: -79 }, 'y')).toBe(false);
  });

  it('test_horizontal_drag_does_not_count_as_vertical_swipe', () => {
    expect(isSwipe({ dx: 200, dy: -20 }, 'y')).toBe(false);
  });

  it('test_vertical_drag_does_not_count_as_horizontal_swipe', () => {
    expect(isSwipe({ dx: 20, dy: -200 }, 'x')).toBe(false);
  });
});

describe('test_swipe_axis_for', () => {
  it('test_up_map_swipes_on_vertical_axis', () => {
    expect(swipeAxisFor(false)).toBe('y');
  });

  it('test_right_map_swipes_on_horizontal_axis', () => {
    expect(swipeAxisFor(true)).toBe('x');
  });
});
