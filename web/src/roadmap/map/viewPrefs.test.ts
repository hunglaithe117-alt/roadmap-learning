// Bộ nhớ cục bộ của bản đồ + điều kiện bật confetti.
//
// `confettiAllowed()` là yêu cầu BẮT BUỘC của ROADMAP-MAP-IDEA §6: tắt hoàn
// toàn khi `prefers-reduced-motion: reduce` **và** có toggle ở Cài đặt. Hai
// điều kiện là VÀ — chỉ kiểm một trong hai là nửa chừng.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  confettiAllowed,
  positionKey,
  readConfettiEnabled,
  readPosition,
  readView,
  writeConfettiEnabled,
  writePosition,
  writeView,
  type ViewMode,
} from './viewPrefs';

function setReduceMotion(reduce: boolean): void {
  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockImplementation((q: string) => ({
      matches: reduce && q.includes('prefers-reduced-motion'),
      media: q,
      addEventListener: () => {},
      removeEventListener: () => {},
    })),
  );
}

beforeEach(() => {
  localStorage.clear();
  vi.unstubAllGlobals();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('test_view_preference', () => {
  it('test_first_visit_defaults_to_map', () => {
    expect(readView()).toBe('map');
  });

  it('test_choice_is_remembered', () => {
    writeView('list');
    expect(readView()).toBe('list');
  });

  it('test_corrupt_storage_value_falls_back_to_map', () => {
    localStorage.setItem('roadmap:view', 'banana');
    expect(readView()).toBe('map');
  });
});

describe('test_scroll_position_memory', () => {
  it('test_position_key_separates_map_and_list', () => {
    expect(positionKey('trung', 'map')).toBe('roadmap:trung:map');
    expect(positionKey('trung', 'list')).not.toBe(positionKey('trung', 'map'));
  });

  it('test_position_round_trips_for_both_views', () => {
    for (const view of ['map', 'list'] as ViewMode[]) {
      writePosition('trung', view, { stageId: '10', offset: 640 });
      expect(readPosition('trung', view)).toEqual({ stageId: '10', offset: 640 });
    }
  });

  it('test_missing_position_reads_as_null', () => {
    expect(readPosition('en', 'map')).toBeNull();
  });

  it('test_corrupt_json_reads_as_null', () => {
    localStorage.setItem(positionKey('trung', 'map'), '{not json');
    expect(readPosition('trung', 'map')).toBeNull();
  });

  it('test_non_numeric_offset_falls_back_to_zero', () => {
    localStorage.setItem(positionKey('trung', 'map'), JSON.stringify({ stageId: '10', offset: 'x' }));
    expect(readPosition('trung', 'map')).toEqual({ stageId: '10', offset: 0 });
  });
});

describe('test_confetti_toggle', () => {
  it('test_confetti_is_on_by_default', () => {
    expect(readConfettiEnabled()).toBe(true);
  });

  it('test_user_can_turn_confetti_off_in_settings', () => {
    writeConfettiEnabled(false);
    expect(readConfettiEnabled()).toBe(false);
    expect(confettiAllowed()).toBe(false);
  });
});

describe('test_prefers_reduced_motion', () => {
  it('test_reduce_motion_disables_confetti_even_with_toggle_on', () => {
    writeConfettiEnabled(true);
    setReduceMotion(true);
    expect(confettiAllowed()).toBe(false);
  });

  it('test_confetti_runs_when_motion_allowed_and_toggle_on', () => {
    writeConfettiEnabled(true);
    setReduceMotion(false);
    expect(confettiAllowed()).toBe(true);
  });

  it('test_missing_matchmedia_is_treated_as_motion_allowed', () => {
    writeConfettiEnabled(true);
    vi.stubGlobal('matchMedia', undefined);
    expect(confettiAllowed()).toBe(true);
  });
});
