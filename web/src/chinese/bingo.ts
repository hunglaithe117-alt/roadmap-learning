// T2.2 — logic bàn bingo tone 3x3 (8 ô chữ + 1 ô free giữa).
// Thuần, test được không cần DOM. Điểm vòng lưu qua POST /api/zh/bingo/score.
import type { DrillItem } from './api';

export interface BingoCell {
  key: string;
  card_id: string | null;
  hanzi: string;
  pair: string;
  free: boolean;
  marked: boolean | null; // null = chưa trả lời, true/false = đúng/sai
}

export function dealBoard(items: DrillItem[]): BingoCell[] {
  if (items.length < 8) {
    throw new Error('cần ít nhất 8 chữ để chia bàn bingo');
  }
  const picked = items.slice(0, 8);
  const cells: BingoCell[] = picked.slice(0, 4).map((it, i) => ({
    key: `c${i}`,
    card_id: it.card_id,
    hanzi: it.hanzi,
    pair: it.pair,
    free: false,
    marked: null,
  }));
  cells.push({ key: 'free', card_id: null, hanzi: '★', pair: '', free: true, marked: true });
  picked.slice(4).forEach((it, i) => {
    cells.push({
      key: `c${i + 4}`,
      card_id: it.card_id,
      hanzi: it.hanzi,
      pair: it.pair,
      free: false,
      marked: null,
    });
  });
  return cells;
}

export function markCell(board: BingoCell[], key: string, correct: boolean): BingoCell[] {
  return board.map((c) => (c.key === key && !c.free ? { ...c, marked: correct } : c));
}

const LINES: number[][] = [
  [0, 1, 2],
  [3, 4, 5],
  [6, 7, 8],
  [0, 3, 6],
  [1, 4, 7],
  [2, 5, 8],
  [0, 4, 8],
  [2, 4, 6],
];

/** Thắng khi có 1 hàng/cột/chéo toàn ô đúng (ô free tính là đúng). */
export function checkWin(board: BingoCell[]): boolean {
  return LINES.some((line) => line.every((i) => board[i]?.marked === true));
}

export function boardScore(board: BingoCell[]): { total: number; correct: number } {
  const played = board.filter((c) => !c.free && c.marked !== null);
  return { total: played.length, correct: played.filter((c) => c.marked === true).length };
}

/** Kết quả gửi lên /api/zh/bingo/score (bỏ ô free + ô chưa trả lời). */
export function buildBingoResults(board: BingoCell[]): Array<{ card_id: string; correct: boolean }> {
  return board
    .filter((c) => !c.free && c.card_id !== null && c.marked !== null)
    .map((c) => ({ card_id: c.card_id as string, correct: c.marked === true }));
}
