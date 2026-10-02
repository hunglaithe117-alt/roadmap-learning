// M5 — client `content` phía Trung: vòng drill thanh điệu, nét chữ, import HSK.
//
// PORT từ `chinese/api.ts` của app v1. Ba điểm khác biệt so với v1, đều là hệ
// quả của việc app-v2 đã bỏ endpoint `/api/zh/*`:
//
//  1. `DrillItem` **không còn là dữ liệu server trả về** mà là hình dạng do
//     client dựng từ `Card` (`cards(deckId)`). Server chỉ có `pinyin` ("ni3
//     hao3"); `tone` ("3 3") và `pair` ("3-3") suy ra bằng
//     `chinese/pinyin.parseSyllable` — cùng quy tắc mà `pinyin-pro` không cần.
//     Giữ nguyên TÊN FIELD của v1 (`card_id`, `pinyin_marks`, …) để
//     `chinese/bingo.ts` + `chinese/drill.ts` (thuần, đã test) không phải đổi.
//  2. Chấm thanh là QUERY `gradeTone` (không phải `POST /api/zh/drill/grade`).
//  3. `postZhBingoScore` — app-v2 không có mutation bingo riêng. V1 chấm cả
//     vòng rồi POST 1 lần; ở đây ghi SRS từng thẻ bằng `recordReview`
//     (đúng nghĩa: "bấm bingo = ôn 1 thẻ") rồi trả về **đúng hình dạng kết quả
//     mà v1 trả** để UI không phải biết khác biệt.
import { runMutation, runQuery } from '../graphql/client';
import { assertPayloadOk } from '../graphql/errors';
import {
  Cards,
  GradeTone,
  ImportHSKMutation,
  RecordReviewMutation,
  Strokes,
  type Card,
  type ImportResult,
  type ReviewResult,
  type StrokeIndexEntry,
  type StrokeInfo,
  type StrokeStep,
} from '../graphql/operations';
import { afterMutation } from '../lib/afterMutation';
import { postReview } from '../srs/api';
import { parseSyllable, toMarks } from './pinyin';

/** Hình dạng drill của app v1 — `chinese/bingo.ts` + `chinese/drill.ts` dùng. */
export interface DrillItem {
  card_id: string;
  hanzi: string;
  pinyin: string;
  pinyin_marks: string;
  tone: string;
  pair: string;
}

export interface DrillGrade {
  correct: boolean;
  score: number;
  grade: number;
  pair: string;
}

export interface StrokeDetail {
  hanzi: string;
  pinyin_marks: string;
  level: string;
  stroke_count: number;
  strokes: StrokeStep[];
}

export interface StrokeIndex {
  level: string;
  chars: StrokeIndexEntry[];
}

/** "ni3 hao3" → "3 3". Âm tiết không đánh số vẫn ra 5 (giữ hành vi v1). */
function toneDigits(pinyin: string): string {
  return pinyin
    .split(/\s+/)
    .filter(Boolean)
    .map((syl) => String(parseSyllable(syl).tone))
    .join(' ');
}

/** Dựng 1 dòng drill từ thẻ của app-v2. */
export function toDrillItem(card: Card): DrillItem {
  const tone = toneDigits(card.pinyin);
  return {
    card_id: card.id,
    hanzi: card.front,
    pinyin: card.pinyin,
    pinyin_marks: toMarks(card.pinyin),
    tone,
    pair: tone.split(' ').join('-'),
  };
}

/**
 * Nguồn danh sách drill = `cards(deckId)` của deck. App-v2 không có
 * `/api/zh/drill` nữa nên "vòng drill" = `limit` thẻ đầu của deck; server vẫn
 * là nơi quyết định thẻ nào hợp lệ (mọi thẻ có `pinyin` đều dùng được).
 */
export async function loadDrillRound(deckId: string, limit = 10): Promise<DrillItem[]> {
  const { cards } = await runQuery(Cards, { deckId });
  return cards.slice(0, limit).map(toDrillItem);
}

export async function postZhDrillGrade(expected: string, answered: string): Promise<DrillGrade> {
  const data = await runQuery(GradeTone, { expected, answered });
  assertPayloadOk(data.gradeTone);
  const g = data.gradeTone.grade;
  if (!g) throw new Error('lỗi hệ thống');
  return { correct: g.exact, score: g.score, grade: g.grade, pair: expected.replace(/ /g, '-') };
}

/** Chấm 1 câu drill rồi lưu SRS — 2 use case của 2 context, giữ như v1. */
export async function answerDrill(
  item: DrillItem,
  answered: string,
): Promise<{ grade: DrillGrade; review: ReviewResult }> {
  const grade = await postZhDrillGrade(item.tone || item.pair, answered);
  const review = await postReview(item.card_id, grade.grade);
  return { grade, review };
}

/** Bingo lấy 8 chữ + 1 ô free ⇒ xin 12 thẻ cho dư (v1 xin 12). */
export async function loadBingoItems(deckId: string): Promise<DrillItem[]> {
  return loadDrillRound(deckId, 12);
}

export interface BingoScore {
  deckId: string;
  total: number;
  correct: number;
  /** card_id → grade đã ghi, giữ đúng shape v1 để UI hiện kết quả vòng. */
  grades: Record<string, number>;
}

/**
 * Ghi điểm vòng bingo. `correct` ⇒ grade 4 (Dễ), sai ⇒ grade 1 (Quên) — cùng
 * thang 1-4 đóng băng ở v1. `afterMutation()` gọi 1 lần sau vòng lặp thay vì
 * 1 lần mỗi thẻ (invalidate là idempotent, gọi 8 lần chỉ tốn thêm microtask).
 */
export async function postZhBingoScore(
  deckId: string,
  results: Array<{ card_id: string; correct: boolean }>,
): Promise<BingoScore> {
  const grades: Record<string, number> = {};
  let correct = 0;
  for (const r of results) {
    const grade = r.correct ? 4 : 1;
    if (r.correct) correct += 1;
    grades[r.card_id] = grade;
    const data = await runMutation(RecordReviewMutation, { cardId: r.card_id, grade });
    assertPayloadOk(data.recordReview);
  }
  await afterMutation();
  return { deckId, total: results.length, correct, grades };
}

export interface ImportReport {
  deckId: string;
  deck: string;
  level: string;
  cardsAdded: number;
  cardsTotal: number;
  dictAdded: number;
}

export async function postZhImport(
  deck = 'HSK1',
  level = 'HSK1',
): Promise<ImportReport> {
  const data = await runMutation(ImportHSKMutation, { input: { deck, level } });
  assertPayloadOk(data.importHSK);
  const r: ImportResult | null = data.importHSK.result;
  if (!r) throw new Error('lỗi hệ thống');
  await afterMutation();
  return r;
}

function toStep(s: { order: number; code: string; name: string }): StrokeStep {
  return { order: s.order, code: s.code, name: s.name };
}


/** Index nhẹ 1 level — `hanzi` null ⇒ chỉ trả index, không tải chi tiết. */
export async function loadStrokeIndex(level = 'HSK1'): Promise<StrokeIndex> {
  const data = await runQuery(Strokes, { level, hanzi: null });
  assertPayloadOk(data.strokes);
  return { level, chars: data.strokes.index };
}

/** Chi tiết 1 chữ (kèm thứ tự nét) — lazy-load như v1. */
export async function loadStrokeDetail(hanzi: string, level = 'HSK1'): Promise<StrokeDetail> {
  const data = await runQuery(Strokes, { level, hanzi });
  assertPayloadOk(data.strokes);
  const info: StrokeInfo | null = data.strokes.info;
  if (!info) throw new Error('lỗi hệ thống');
  return {
    hanzi: info.hanzi,
    pinyin_marks: info.pinyinMarks,
    level: info.level,
    stroke_count: info.strokeCount,
    strokes: info.steps.map(toStep),
  };
}
