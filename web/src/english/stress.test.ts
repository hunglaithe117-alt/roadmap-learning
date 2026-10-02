import { describe, it, expect } from 'vitest';
import { splitStressMarks } from './stress';

describe('test_stress_marks', () => {
  it('test_split_stress_marks_caps_syllable_is_stressed', () => {
    const parts = splitStressMarks('PHO-to-graph');
    expect(parts[0]).toEqual({ text: 'PHO', stressed: true });
    expect(parts[1].stressed).toBe(false);
    expect(parts).toHaveLength(3);
  });

  it('test_split_stress_marks_empty_returns_empty', () => {
    expect(splitStressMarks('')).toEqual([]);
  });
});
