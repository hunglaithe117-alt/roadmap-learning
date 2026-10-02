import { describe, it, expect } from 'vitest';
import { thieuAverage } from '../english/thieu';

/** Route-adjacent test for EnThieu: checklist scoring shown on save button. */
describe('test_enthieu_route_scoring', () => {
  it('test_enthieu_checklist_all3_average_is_3', () => {
    expect(thieuAverage({ A: 3, B: 3, C: 3, D: 3, E: 3, F: 3, G: 3, H: 3 })).toBe(3);
  });

  it('test_enthieu_checklist_mixed_average', () => {
    expect(thieuAverage({ A: 1, B: 5, C: 1, D: 5, E: 1, F: 5, G: 1, H: 5 })).toBe(3);
  });
});
