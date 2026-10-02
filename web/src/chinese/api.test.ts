// Port `chinese/api.test.ts` (9) + `routes/ZhPinyin.test.ts` (1) +
// `routes/ZhBingo.test.ts` (3) sang client `content` phía Trung của app-v2.
//
// Ba test cũ assert URL REST (`/api/zh/drill?deck_id=…`). App-v2 không còn
// endpoint đó, nên test assert **operation + variables** của GraphQL — đó là
// thứ quyết định server trả đúng dữ liệu, và là hợp đồng mới của client.
import { afterEach, describe, expect, it, vi } from 'vitest';
import { resetStaleMarks, resetUrqlClient } from '../graphql/client';
import { buildTTSUrl } from '../rest/client';
import { bodyOf, mockData, operationNameOf } from '../test/graphqlMock';

/** Trả lần lượt các `data` theo thứ tự request (2 use case của 2 context). */
const mockSequence = mockData;
import { dealBoard } from './bingo';
import {
  answerDrill,
  loadBingoItems,
  loadDrillRound,
  loadStrokeDetail,
  loadStrokeIndex,
  postZhBingoScore,
  postZhDrillGrade,
  postZhImport,
  toDrillItem,
  type DrillItem,
} from './api';

afterEach(() => {
  vi.unstubAllGlobals();
  resetUrqlClient();
  resetStaleMarks();
});

const CARD = {
  id: '1',
  deckId: '1',
  front: '你好',
  back: 'xin chào',
  pinyin: 'ni3 hao3',
  dueAt: 'x',
  state: 'new',
  tone: null,
  audioURL: null,
};

const ITEM: DrillItem = {
  card_id: '1',
  hanzi: '你好',
  pinyin: 'ni3 hao3',
  pinyin_marks: 'nǐ hǎo',
  tone: '3 3',
  pair: '3-3',
};

describe('test_zh_drill_api_client', () => {
  it('test_post_zh_drill_grade_exact_returns_grade4', async () => {
    mockData({ gradeTone: { ok: true, grade: { grade: 4, score: 1, exact: true, hit: 2 }, error: null } });
    const g = await postZhDrillGrade('3 3', '3 3');
    expect(g.grade).toBe(4);
    expect(g.correct).toBe(true);
  });

  it('test_load_drill_round_uses_cards_query_with_deck_id', async () => {
    const fetchMock = mockData({ cards: [CARD] });
    await loadDrillRound('1', 10);
    const body = bodyOf(fetchMock);
    expect(operationNameOf(fetchMock)).toBe('Cards');
    expect(body.variables).toEqual({ deckId: '1' });
  });

  it('test_load_drill_round_maps_pinyin_to_tone_and_pair', async () => {
    // App-v2 chỉ trả `pinyin`; `tone`/`pair` là thứ client dựng. Nếu đổi quy
    // tắc (`tone` dùng dấu thanh thay vì số) thì test này đỏ.
    mockData({ cards: [CARD] });
    const items = await loadDrillRound('1', 10);
    expect(items).toHaveLength(1);
    expect(items[0]).toEqual(ITEM);
  });

  it('test_load_bingo_items_asks_twelve_cards', async () => {
    // Bingo cần 8 chữ + 1 ô free; xin 12 như v1 để dự phòng.
    const cards = Array.from({ length: 12 }, (_, i) => ({ ...CARD, id: String(i + 1) }));
    mockData({ cards });
    const items = await loadBingoItems('1');
    expect(dealBoard(items)).toHaveLength(9);
    expect(items).toHaveLength(12);
  });

  it('test_answer_drill_grades_then_saves_review', async () => {
    const fetchMock = mockSequence(
      { gradeTone: { ok: true, grade: { grade: 4, score: 1, exact: true, hit: 2 }, error: null } },
      {
        recordReview: {
          ok: true,
          review: {
            cardId: '1',
            dueAt: 'x',
            intervalDays: 1,
            stability: 1,
            difficulty: 1,
            reps: 1,
            fallback: true,
          },
          error: null,
        },
      },
    );
    const { grade, review } = await answerDrill(ITEM, '3 3');
    expect(grade.grade).toBe(4);
    expect(review.cardId).toBe('1');
    // Thứ tự 2 use case của 2 context phải giữ: chấm trước, lưu SRS sau.
    expect(operationNameOf(fetchMock, 0)).toBe('GradeTone');
    expect(operationNameOf(fetchMock, 1)).toBe('RecordReview');
    expect(bodyOf(fetchMock, 1).variables).toEqual({ cardId: '1', grade: 4 });
  });

  it('test_post_zh_import_returns_counts', async () => {
    mockData({
      importHSK: {
        ok: true,
        result: { deckId: '1', deck: 'HSK1', level: 'HSK1', cardsAdded: 36, cardsTotal: 36, dictAdded: 30 },
        error: null,
      },
    });
    const r = await postZhImport('HSK1', 'HSK1');
    expect(r.cardsAdded).toBe(36);
  });

  it('test_zh_grade_error_throws_vietnamese_message', async () => {
    mockData({
      gradeTone: {
        ok: false,
        grade: null,
        error: { message: 'số âm tiết không khớp (mẫu 2, bạn nhập 1)', code: 'BAD_REQUEST' },
      },
    });
    await expect(postZhDrillGrade('3 3', '3')).rejects.toThrow('số âm tiết không khớp');
  });

  it('test_tts_url_points_to_api_tts', () => {
    expect(buildTTSUrl('你好', '')).toBe('/api/tts?text=' + encodeURIComponent('你好'));
  });
});

describe('test_zh_bingo_api_client', () => {
  it('test_post_zh_bingo_score_writes_one_review_per_result', async () => {
    const fetchMock = mockSequence({
      recordReview: {
        ok: true,
        review: {
          cardId: '5',
          dueAt: 'x',
          intervalDays: 1,
          stability: 1,
          difficulty: 1,
          reps: 1,
          fallback: true,
        },
        error: null,
      },
    });
    const r = await postZhBingoScore('1', [
      { card_id: '5', correct: true },
      { card_id: '6', correct: false },
    ]);
    // Giữ nguyên hình dạng kết quả của v1 để UI không phải biết khác biệt.
    expect(r.correct).toBe(1);
    expect(r.total).toBe(2);
    expect(r.grades).toEqual({ '5': 4, '6': 1 });
    // 2 thẻ ⇒ 2 mutation; grade 4 cho đúng, 1 cho sai.
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(bodyOf(fetchMock, 0).variables).toEqual({ cardId: '5', grade: 4 });
    expect(bodyOf(fetchMock, 1).variables).toEqual({ cardId: '6', grade: 1 });
  });
});

describe('test_zh_stroke_lazy_load', () => {
  it('test_load_stroke_index_queries_level_without_hanzi', async () => {
    const fetchMock = mockData({
      strokes: {
        ok: true,
        index: [{ hanzi: '人', strokeCount: 2 }],
        info: null,
        error: null,
      },
    });
    const r = await loadStrokeIndex('HSK1');
    expect(r.chars).toEqual([{ hanzi: '人', strokeCount: 2 }]);
    const body = bodyOf(fetchMock);
    expect(operationNameOf(fetchMock)).toBe('Strokes');
    // `hanzi: null` = chỉ xin index nhẹ, KHÔNG tải chi tiết 1 chữ.
    expect(body.variables).toEqual({ level: 'HSK1', hanzi: null });
  });

  it('test_load_stroke_detail_returns_steps', async () => {
    const fetchMock = mockData({
      strokes: {
        ok: true,
        index: [],
        info: {
          hanzi: '人',
          pinyinMarks: 'rén',
          level: 'HSK1',
          strokeCount: 2,
          steps: [
            { order: 1, code: 'p', name: 'phẩy' },
            { order: 2, code: 'n', name: 'nà' },
          ],
        },
        error: null,
      },
    });
    const d = await loadStrokeDetail('人', 'HSK1');
    expect(d.stroke_count).toBe(2);
    // Tên field đổi camelCase→snake_case ở biên để module thuần `strokes.ts` giữ
    // nguyên hình dạng của v1.
    expect(d).toEqual({
      hanzi: '人',
      pinyin_marks: 'rén',
      level: 'HSK1',
      stroke_count: 2,
      strokes: [
        { order: 1, code: 'p', name: 'phẩy' },
        { order: 2, code: 'n', name: 'nà' },
      ],
    });
    expect(bodyOf(fetchMock).variables).toEqual({ level: 'HSK1', hanzi: '人' });
  });
});

describe('test_to_drill_item', () => {
  it('test_to_drill_item_keeps_neutral_tone_as_5', () => {
    // `ma` không đánh số thanh ⇒ `parseSyllable` cho tone 5 (giống `pinyin.ts`).
    const item = toDrillItem({ ...CARD, pinyin: 'ma' });
    expect(item.tone).toBe('5');
    expect(item.pair).toBe('5');
  });
});
