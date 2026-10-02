import { describe, it, expect } from 'vitest';
import { wordDiff, wrongWords, diffScore, toSimplified } from './diff';

describe('test_word_diff_highlight', () => {
  it('test_diff_identical_marks_all_ok', () => {
    const d = wordDiff('ni hao ma', 'ni hao ma');
    expect(d.every((t) => t.status === 'ok')).toBe(true);
  });

  it('test_diff_wrong_word_merges_missing_extra', () => {
    const d = wordDiff('ni hao ma', 'ni hao ba');
    expect(d).toEqual([
      { text: 'ni', status: 'ok' },
      { text: 'hao', status: 'ok' },
      { text: 'ma→ba', status: 'wrong' },
    ]);
    expect(wrongWords(d)).toEqual(['ma→ba']);
  });

  it('test_diff_case_and_punctuation_insensitive', () => {
    const d = wordDiff('Keep a promise.', 'keep A PROMISE');
    expect(d.every((t) => t.status === 'ok')).toBe(true);
  });

  it('test_diff_missing_and_extra_tokens', () => {
    const d = wordDiff('a b c', 'a c d');
    expect(d.map((t) => t.status)).toEqual(['ok', 'missing', 'ok', 'extra']);
    expect(diffScore('a b c', d)).toBeCloseTo(2 / 3);
  });

  it('test_diff_empty_expected_scores_zero', () => {
    expect(diffScore('', [])).toBe(0);
  });

  it('test_traditional_to_simplified', () => {
    expect(toSimplified('學習廣東話')).toBe('学习广东话');
    expect(toSimplified('他們說國語')).toBe('他们说国语');
    expect(toSimplified('你好 ni hao')).toBe('你好 ni hao');
  });

  it('test_diff_traditional_matches_simplified', () => {
    const d = wordDiff('学习', '學習');
    expect(d.every((t) => t.status === 'ok')).toBe(true);
  });
});
