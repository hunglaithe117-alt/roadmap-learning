import { describe, it, expect } from 'vitest';
import {
  classifyStroke,
  expectedCodes,
  findCardForHanzi,
  gradeStrokes,
  type StrokeDetail,
} from './strokes';

describe('test_stroke_classification', () => {
  it('test_classify_horizontal_returns_h', () => {
    expect(classifyStroke([{ x: 0, y: 0 }, { x: 50, y: 2 }])).toBe('h');
  });

  it('test_classify_vertical_returns_s', () => {
    expect(classifyStroke([{ x: 0, y: 0 }, { x: 2, y: 50 }])).toBe('s');
  });

  it('test_classify_down_left_returns_p', () => {
    expect(classifyStroke([{ x: 50, y: 0 }, { x: 0, y: 50 }])).toBe('p');
  });

  it('test_classify_down_right_returns_n', () => {
    expect(classifyStroke([{ x: 0, y: 0 }, { x: 50, y: 50 }])).toBe('n');
  });

  it('test_classify_dot_returns_d', () => {
    expect(classifyStroke([{ x: 10, y: 10 }, { x: 12, y: 11 }])).toBe('d');
  });
});

describe('test_stroke_order_grading', () => {
  const ren: StrokeDetail = {
    hanzi: '人',
    pinyin_marks: 'rén',
    level: 'HSK1',
    stroke_count: 2,
    strokes: [
      { order: 1, code: 'p', name: 'phẩy' },
      { order: 2, code: 'n', name: 'mác' },
    ],
  };

  it('test_grade_correct_order_passes', () => {
    const g = gradeStrokes(expectedCodes(ren), ['p', 'n']);
    expect(g.pass).toBe(true);
    expect(g.score).toBe(1);
  });

  it('test_grade_wrong_order_fails', () => {
    const g = gradeStrokes(expectedCodes(ren), ['n', 'p']);
    expect(g.pass).toBe(false);
    expect(g.score).toBe(0);
  });

  it('test_grade_missing_stroke_fails', () => {
    const g = gradeStrokes(expectedCodes(ren), ['p']);
    expect(g.pass).toBe(false);
  });
});

describe('test_find_card_for_hanzi', () => {
  it('test_find_card_for_hanzi_matches_front', () => {
    // Id là chuỗi (xem `chinese/strokes.ts`); màn nét chữ dùng hàm này để biết
    // thẻ nào cần ghi điểm SRS sau khi chấm đúng.
    const cards = [
      { id: '7', front: '你好' },
      { id: '8', front: '谢谢' },
    ];
    expect(findCardForHanzi(cards, '谢谢')).toBe('8');
    expect(findCardForHanzi(cards, '没有')).toBeNull();
  });
});
