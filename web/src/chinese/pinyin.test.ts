import { describe, it, expect } from 'vitest';
import { toMarks, markSyllable, parseToneInput, pairLabel, gradePair } from './pinyin';

describe('test_pinyin_marks_render', () => {
  it('test_to_marks_nihao_returns_marks', () => {
    expect(toMarks('ni3 hao3')).toBe('nǐ hǎo');
  });

  it('test_mark_syllable_placement_rules', () => {
    expect(markSyllable('zhong1')).toBe('zhōng');
    expect(markSyllable('guo2')).toBe('guó');
    expect(markSyllable('xue2')).toBe('xué');
    expect(markSyllable('hao3')).toBe('hǎo');
    expect(markSyllable('mei2')).toBe('méi');
    expect(markSyllable('ou1')).toBe('ōu');
  });

  it('test_mark_syllable_iu_ui_marks_last_vowel', () => {
    expect(markSyllable('liu2')).toBe('liú');
    expect(markSyllable('gui4')).toBe('guì');
    expect(markSyllable('lüe4')).toBe('lüè');
  });

  it('test_mark_syllable_neutral_keeps_base', () => {
    expect(markSyllable('ma5')).toBe('ma');
    expect(toMarks('shen2 me5')).toBe('shén me');
  });
});

describe('test_tone_pair_scoring', () => {
  it('test_grade_pair_exact_returns_easy', () => {
    expect(gradePair([3, 3], [3, 3])).toEqual({ grade: 4, score: 1, exact: true });
  });

  it('test_grade_pair_half_returns_good', () => {
    expect(gradePair([1, 4], [1, 2])).toEqual({ grade: 3, score: 0.5, exact: false });
  });

  it('test_grade_pair_none_returns_again', () => {
    expect(gradePair([1, 4], [2, 3])).toEqual({ grade: 1, score: 0, exact: false });
  });

  it('test_grade_pair_mismatch_throws', () => {
    expect(() => gradePair([3, 3], [3])).toThrow();
  });

  it('test_parse_tone_input_accepts_variants', () => {
    expect(parseToneInput('3 3')).toEqual([3, 3]);
    expect(parseToneInput('1-4')).toEqual([1, 4]);
    expect(parseToneInput('hao3')).toEqual([3]);
    expect(parseToneInput('x')).toBeNull();
  });

  it('test_pair_label_joins_with_dash', () => {
    expect(pairLabel([3, 3])).toBe('3-3');
  });
});
