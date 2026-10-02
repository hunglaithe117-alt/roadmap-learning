import { describe, it, expect } from 'vitest';
import { dealBoard, markCell, checkWin, boardScore, buildBingoResults } from './bingo';
import { summarizeRound, roundGrade, remainingItems } from './drill';
import type { DrillItem } from './api';

function sampleItems(n = 8): DrillItem[] {
  const hanzi = ['你', '好', '我', '他', '是', '不', '大', '小', '中', '水'];
  // `card_id` là chuỗi ở M5 (`ID` của GraphQL là `String`, cột DB là `bigint`).
  return hanzi.slice(0, n).map((h, i) => ({
    card_id: String(i + 1),
    hanzi: h,
    pinyin: 'x1',
    pinyin_marks: h,
    tone: '1',
    pair: '1',
  }));
}

describe('test_bingo_board_logic', () => {
  it('test_deal_board_needs_8_items', () => {
    expect(() => dealBoard(sampleItems(3))).toThrow('cần ít nhất 8 chữ');
  });

  it('test_deal_board_has_free_center', () => {
    const board = dealBoard(sampleItems());
    expect(board).toHaveLength(9);
    expect(board[4].free).toBe(true);
    expect(board[4].marked).toBe(true);
  });

  it('test_check_win_top_row_wins', () => {
    let board = dealBoard(sampleItems());
    board = markCell(board, 'c0', true);
    board = markCell(board, 'c1', true);
    board = markCell(board, 'c2', true);
    expect(checkWin(board)).toBe(true);
  });

  it('test_check_win_diagonal_through_free_wins', () => {
    let board = dealBoard(sampleItems());
    board = markCell(board, 'c0', true);
    board = markCell(board, 'c7', true);
    expect(checkWin(board)).toBe(true);
  });

  it('test_check_win_wrong_cell_blocks_win', () => {
    let board = dealBoard(sampleItems());
    board = markCell(board, 'c0', true);
    board = markCell(board, 'c1', false);
    board = markCell(board, 'c2', true);
    expect(checkWin(board)).toBe(false);
  });

  it('test_board_score_counts_answered_cells', () => {
    let board = dealBoard(sampleItems());
    board = markCell(board, 'c0', true);
    board = markCell(board, 'c1', false);
    expect(boardScore(board)).toEqual({ total: 2, correct: 1 });
  });

  it('test_build_bingo_results_skips_free_and_unanswered', () => {
    let board = dealBoard(sampleItems());
    board = markCell(board, 'c0', true);
    const results = buildBingoResults(board);
    expect(results).toEqual([{ card_id: '1', correct: true }]);
  });
});

describe('test_drill_round_logic', () => {
  it('test_summarize_round_computes_pct', () => {
    expect(summarizeRound([true, true, false])).toEqual({ total: 3, correct: 2, pct: 2 / 3 });
  });

  it('test_round_grade_maps_pct_to_srs', () => {
    expect(roundGrade(1)).toBe(4);
    expect(roundGrade(0.7)).toBe(3);
    expect(roundGrade(0.2)).toBe(2);
    expect(roundGrade(0)).toBe(1);
  });

  it('test_remaining_items_skips_done', () => {
    const items = sampleItems(3);
    expect(remainingItems(items, ['1'])).toHaveLength(2);
  });
});
