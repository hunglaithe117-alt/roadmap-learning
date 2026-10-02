// 3 TÍNH NĂNG MẤT KHI PORT React→Vue — phục hồi ở M6b.
//
// Cả 3 đều là loại lỗi M5 đã ghi trong handoff: hàm có, test có, **không màn
// nào gọi**. Không có lỗi biên dịch, không có cảnh báo — app chạy bình thường,
// người dùng chỉ thấy "import HSK1 trước" mà không có nút import, và không lọc
// được sổ lỗi theo thẻ. Ba test ở đây gắn lại đúng mối nối đó.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises } from '@vue/test-utils';
import { resetStaleMarks, resetUrqlClient } from '../graphql/client';
import { queryClient } from '../rest/queryClient';
import { bodyOf } from '../test/graphqlMock';
import { mountScreen, type MountedScreen } from '../test/harness';
import ErrorBook from './ErrorBook.vue';
import Review from './Review.vue';
import ZhPinyin from './ZhPinyin.vue';

const DECK = { id: '1', guid: 'g1', name: 'HSK1', lang: 'zh', createdAt: 'x' };

function drillCard(id: string, pinyin: string, front = '你好') {
  return {
    id,
    deckId: '1',
    front,
    back: 'xin chào',
    pinyin,
    dueAt: 'x',
    state: 'new',
    tone: null,
    audioURL: null,
  };
}

function payloadFor(name: string): Record<string, unknown> {
  switch (name) {
    case 'Decks':
      return { decks: [DECK] };
    case 'Cards':
      return { cards: [drillCard('7', 'ni3 hao3'), drillCard('8', 'shi4 ge4')] };
    // Cố ý CHỈ trả thẻ 8: nếu `?card=7` thật sự đưa thẻ 7 lên đầu hàng đợi
    // thì màn phải hiện `你好` (thẻ 7) chứ không phải `再见` (thẻ 8).
    case 'DueCards':
      return { dueCards: [drillCard('8', 'shi4 ge4', '再见')] };
    case 'ImportHSK':
      return {
        importHSK: {
          ok: true,
          result: { deckId: '1', deck: 'HSK1', level: 'HSK1', cardsAdded: 36, cardsTotal: 36, dictAdded: 30 },
          error: null,
        },
      };
    case 'GradeTone':
      return { gradeTone: { ok: true, grade: { grade: 4, score: 1, exact: true, hit: 2 }, error: null } };
    case 'RecordReview':
      return {
        recordReview: {
          ok: true,
          review: { cardId: '7', dueAt: 'd', intervalDays: 3, stability: 1, difficulty: 1, reps: 1, fallback: true },
          error: null,
        },
      };
    case 'Errors':
      return { errors: [{ id: 'e1', cardId: '7', expected: 'ni hao', transcript: 'ni hau', wrong: ['hau'], createdAt: 'x' }] };
    case 'InsightTopErrors':
      return { insightTopErrors: { ok: true, errors: [{ word: 'hau', count: 3, cardId: '7', front: '你好' }], error: null } };
    case 'ErrorSuggestions':
      return { errorSuggestions: [] };
    default:
      return {};
  }
}

let fetchMock: ReturnType<typeof vi.fn>;

function stubServer() {
  fetchMock = vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    let name = new URL(String(input), 'http://x').searchParams.get('operationName') ?? '';
    if (!name && init?.body) name = (JSON.parse(init.body as string).operationName as string) ?? '';
    return new Response(JSON.stringify({ data: payloadFor(name) }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

/** urql gửi QUERY bằng GET (operation trong query string) và MUTATION bằng POST
 *  (operation trong body). Đọc cả hai — xem `test/graphqlMock.ts` (M5 đã vấp
 *  đúng chỗ này: mock chỉ đọc query string thì kết luận về POST hoàn toàn sai). */
function opNameOf(call: unknown): string {
  const [url, init] = call as [string, RequestInit | undefined];
  if (init?.body) return (JSON.parse(init.body as string) as { operationName: string }).operationName;
  return new URL(url, 'http://x').searchParams.get('operationName') ?? '';
}

function callsOf(op: string): Array<Record<string, unknown>> {
  const out: Array<Record<string, unknown>> = [];
  fetchMock.mock.calls.forEach((c, i) => {
    if (opNameOf(c) === op) out.push(bodyOf(fetchMock, i).variables);
  });
  return out;
}

beforeEach(() => {
  localStorage.clear();
  vi.unstubAllGlobals();
  resetUrqlClient();
  resetStaleMarks();
  queryClient.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

// ─────────────────────────────────────────────────────────────────────────────
// 1. Nút nạp HSK ở màn Pinyin
// ─────────────────────────────────────────────────────────────────────────────

describe('test_restored_1_hsk_import_button', () => {
  it('test_pinyin_screen_offers_an_import_button', async () => {
    stubServer();
    const w = await mountScreen(ZhPinyin, { route: '/zh-pinyin' });
    await flushPromises();
    const btn = w.findAll('button').find((b) => b.text().includes('Nạp HSK1'));
    expect(btn, 'không có nút nạp HSK ở màn Pinyin').toBeTruthy();
  });

  it('test_clicking_import_calls_import_hsk_mutation', async () => {
    stubServer();
    const w = await mountScreen(ZhPinyin, { route: '/zh-pinyin' });
    await flushPromises();

    await w.findAll('button').find((b) => b.text().includes('Nạp HSK1'))!.trigger('click');
    for (let i = 0; i < 6; i += 1) await flushPromises();

    const sent = callsOf('ImportHSK');
    expect(sent).toHaveLength(1);
    expect(sent[0].input).toMatchObject({ deck: 'HSK1', level: 'HSK1' });
  });

  it('test_import_reports_how_many_cards_were_added', async () => {
    stubServer();
    const w = await mountScreen(ZhPinyin, { route: '/zh-pinyin' });
    await flushPromises();

    await w.findAll('button').find((b) => b.text().includes('Nạp HSK1'))!.trigger('click');
    for (let i = 0; i < 6; i += 1) await flushPromises();

    expect(w.text()).toContain('+36');
  });

  it('test_import_failure_shows_the_vietnamese_server_message', async () => {
    // `ImportHSK` chỉ có `ok/error` trong payload; lỗi phải lên UI nguyên văn.
    const m = vi.fn().mockImplementation(async (_input: RequestInfo | URL, init?: RequestInit) => {
      const name = init?.body ? (JSON.parse(init.body as string) as { operationName: string }).operationName : '';
      const data =
        name === 'ImportHSK'
          ? { importHSK: { ok: false, result: null, error: { message: 'level HSK không có trong seed', code: 'BAD_REQUEST' } } }
          : payloadFor(name);
      return new Response(JSON.stringify({ data }), { status: 200, headers: { 'content-type': 'application/json' } });
    });
    vi.stubGlobal('fetch', m);

    const w = await mountScreen(ZhPinyin, { route: '/zh-pinyin' });
    await flushPromises();
    await w.findAll('button').find((b) => b.text().includes('Nạp HSK1'))!.trigger('click');
    for (let i = 0; i < 6; i += 1) await flushPromises();

    expect(w.text()).toContain('level HSK không có trong seed');
  });

  it('test_empty_deck_message_points_at_the_import_button', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
        const name = init?.body
          ? (JSON.parse(init.body as string) as { operationName: string }).operationName
          : (new URL(String(input), 'http://x').searchParams.get('operationName') ?? '');
        const data = name === 'Cards' ? { cards: [] } : name === 'Decks' ? { decks: [DECK] } : {};
        return new Response(JSON.stringify({ data }), { status: 200, headers: { 'content-type': 'application/json' } });
      }),
    );
    const w = await mountScreen(ZhPinyin, { route: '/zh-pinyin' });
    await flushPromises();

    await w.find('select').setValue('1');
    await w.findAll('button').find((b) => b.text().includes('Bắt đầu 1 vòng'))!.trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();

    expect(w.text()).toContain('nạp HSK1');
    expect(w.findAll('button').some((b) => b.text().includes('Nạp HSK1'))).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// 2. Bộ lọc lỗi theo thẻ ở Sổ lỗi
// ─────────────────────────────────────────────────────────────────────────────

describe('test_restored_2_error_book_card_filter', () => {
  it('test_errors_are_fetched_without_a_card_filter_by_default', async () => {
    stubServer();
    await mountScreen(ErrorBook, { route: '/loi-sai' });
    await flushPromises();
    expect(callsOf('Errors')[0]).toEqual({ cardId: null, limit: 50 });
  });

  it('test_typing_a_card_id_sends_it_to_the_query', async () => {
    stubServer();
    const w = await mountScreen(ErrorBook, { route: '/loi-sai' });
    await flushPromises();

    await w.find('input[aria-label="lọc lỗi theo id thẻ"]').setValue('42');
    await w.findAll('button').find((b) => b.text() === 'Lọc')!.trigger('click');
    for (let i = 0; i < 6; i += 1) await flushPromises();

    const sent = callsOf('Errors');
    expect(sent.at(-1)).toEqual({ cardId: '42', limit: 50 });
  });

  it('test_clearing_the_filter_goes_back_to_all_cards', async () => {
    stubServer();
    const w = await mountScreen(ErrorBook, { route: '/loi-sai' });
    await flushPromises();

    await w.find('input[aria-label="lọc lỗi theo id thẻ"]').setValue('42');
    await w.findAll('button').find((b) => b.text() === 'Lọc')!.trigger('click');
    for (let i = 0; i < 6; i += 1) await flushPromises();
    expect(w.text()).toContain('Đang lọc theo thẻ 42');

    await w.findAll('button').find((b) => b.text() === 'Bỏ lọc')!.trigger('click');
    for (let i = 0; i < 6; i += 1) await flushPromises();

    // Query `cardId: null` đã nằm trong cache nên urql KHÔNG ra mạng lần nữa —
    // đúng hành vi, nên test này khẳng định trạng thái hiển thị chứ không đếm
    // request (số request không phải hợp đồng của bộ lọc).
    expect(w.text()).toContain('Chưa lọc');
    expect((w.find('input[aria-label="lọc lỗi theo id thẻ"]').element as HTMLInputElement).value).toBe('');
  });

  it('test_top_errors_row_links_to_review_only_when_card_id_exists', async () => {
    stubServer();
    const w = await mountScreen(ErrorBook, { route: '/loi-sai' });
    await flushPromises();
    const row = w.get('[data-testid="top-error-hau"]');
    // F2 (M6 remediation): assert **HREF**, không assert text. Bản cũ chỉ
    // kiểm `row.text()).toContain('Ôn thẻ này')` nên xanh trong khi
    // `/review?card=…` không ai đọc — link chết vẫn xanh (bẫy #1 §8.1).
    const link = row.get('[data-testid="top-error-review-hau"]');
    expect(link.attributes('href')).toBe('/review?card=7');
  });

  it('test_review_screen_actually_shows_the_card_the_row_links_to', async () => {
    // Đóng vòng: lấy đúng `href` mà ErrorBook render rồi mở `Review.vue` tại
    // URL đó. Chỉ assert `href` thì vẫn có thể trỏ tới nơi không đọc tham số;
    // test này chứng minh điều hướng thật sự tới nơi và thẻ hiện lên.
    stubServer();
    const book = await mountScreen(ErrorBook, { route: '/loi-sai' });
    await flushPromises();
    const href = book.get('[data-testid="top-error-review-hau"]').attributes('href');
    expect(href).toBe('/review?card=7');

    const review = await mountScreen(Review, { route: href! });
    for (let i = 0; i < 12; i += 1) await flushPromises();

    // Thẻ 7 lên ĐẦU hàng đợi và màn KHÔNG rơi về "Chọc deck để bắt đầu ôn"
    // (trước đó `deckId` rỗng nên `v-if="!deckId"` giữ màn trắng).
    expect(review.text()).toContain('你好');
    expect(review.text()).not.toContain('Chọc deck để bắt đầu ôn');
  });

  it('test_top_error_without_card_id_renders_a_static_row', async () => {
    // Lỗi luyện tự do / thẻ đã xoá mềm ⇒ `cardId` null. Render nút nhảy review
    // là nút chết.
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
        const name = init?.body
          ? (JSON.parse(init.body as string) as { operationName: string }).operationName
          : (new URL(String(input), 'http://x').searchParams.get('operationName') ?? '');
        const data =
          name === 'InsightTopErrors'
            ? { insightTopErrors: { ok: true, errors: [{ word: 'hau', count: 2, cardId: null, front: null }], error: null } }
            : name === 'Decks'
              ? { decks: [] }
              : payloadFor(name);
        return new Response(JSON.stringify({ data }), { status: 200, headers: { 'content-type': 'application/json' } });
      }),
    );
    const w = await mountScreen(ErrorBook, { route: '/loi-sai' });
    await flushPromises();

    const row = w.get('[data-testid="top-error-hau"]');
    expect(row.text()).toContain('không gắn thẻ');
    expect(row.find('a').exists()).toBe(false);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// 3. Chấm điểm cả vòng drill
// ─────────────────────────────────────────────────────────────────────────────

/** Radio đang được chọn — `<input type=radio>` không có `v-model`, nên phải đọc
 *  `checked` trực tiếp thay vì tin vào `wrapper.find('…:checked')`. */
function checkedMode(w: MountedScreen): string {
  const on = w.findAll('input[type="radio"]').filter((r) => (r.element as HTMLInputElement).checked);
  expect(on, 'phải có đúng 1 chế độ chấm được chọn').toHaveLength(1);
  return (on[0].element as HTMLInputElement).value;
}

async function startRound(mode: 'card' | 'round'): Promise<MountedScreen> {
  localStorage.setItem('zh-pinyin:grade-mode', mode);
  stubServer();
  const w = await mountScreen(ZhPinyin, { route: '/zh-pinyin' });
  await flushPromises();
  await w.find('select').setValue('1');
  await w.findAll('button').find((b) => b.text().includes('Bắt đầu 1 vòng'))!.trigger('click');
  for (let i = 0; i < 4; i += 1) await flushPromises();
  return w;
}

async function answer(w: MountedScreen, tone: string): Promise<void> {
  await w.find('input[aria-label="thanh điệu bạn nghe"]').setValue(tone);
  await w.findAll('button').find((b) => b.text() === 'Chấm điểm')!.trigger('click');
  for (let i = 0; i < 4; i += 1) await flushPromises();
}

describe('test_restored_3_round_grading_mode', () => {
  it('test_per_card_is_the_default_mode', async () => {
    const w = await startRound('card');
    expect(checkedMode(w)).toBe('card');
  });

  it('test_per_card_mode_saves_review_for_every_answer', async () => {
    const w = await startRound('card');
    await answer(w, '3 3');
    expect(callsOf('RecordReview').length).toBe(1);
  });

  it('test_round_mode_defers_saving_until_the_round_ends', async () => {
    const w = await startRound('round');
    await answer(w, '3 3');
    // Chế độ cả vòng: 1 câu đúng KHÔNG được ghi SRS ngay — ghi sớm là chấm
    // từng thẻ mà mất vòng. Vòng stub có 2 thẻ nên câu 1 vẫn chưa xong.
    expect(callsOf('RecordReview')).toHaveLength(0);
    expect(w.text()).not.toContain('Lưu điểm cả vòng');

    await answer(w, '4 4');
    expect(w.text()).toContain('Xong 1 vòng');
    expect(w.findAll('button').some((b) => b.text() === 'Lưu điểm cả vòng')).toBe(true);
    expect(callsOf('RecordReview')).toHaveLength(0);
  });

  it('test_saving_the_round_writes_one_review_per_answered_card', async () => {
    const w = await startRound('round');
    await answer(w, '3 3');
    await answer(w, '4 4');

    await w.findAll('button').find((b) => b.text() === 'Lưu điểm cả vòng')!.trigger('click');
    for (let i = 0; i < 8; i += 1) await flushPromises();

    const reviews = callsOf('RecordReview');
    expect(reviews).toHaveLength(2);
    expect(reviews.map((r) => r.cardId).sort()).toEqual(['7', '8']);
  });

  it('test_round_grade_is_shared_across_the_round', async () => {
    const w = await startRound('round');
    await answer(w, '3 3');
    await answer(w, '4 4');
    await w.findAll('button').find((b) => b.text() === 'Lưu điểm cả vòng')!.trigger('click');
    for (let i = 0; i < 8; i += 1) await flushPromises();

    // Cả 2 câu đúng ⇒ `roundGrade(1)` = 4 cho mọi thẻ. `RecordReview` gửi
    // `cardId`/`grade` ở top level (mutation viết input inline trong selection).
    const grades = callsOf('RecordReview').map((r) => r.grade);
    expect(new Set(grades)).toEqual(new Set([4]));
  });

  it('test_switching_modes_is_possible_from_the_screen', async () => {
    localStorage.clear();
    stubServer();
    const w = await mountScreen(ZhPinyin, { route: '/zh-pinyin' });
    await flushPromises();

    await w.find('input[aria-label="chấm cả vòng"]').setValue(true);
    await flushPromises();

    expect(localStorage.getItem('zh-pinyin:grade-mode')).toBe('round');
  });

  it('test_saved_mode_survives_reopening_the_screen', async () => {
    localStorage.setItem('zh-pinyin:grade-mode', 'round');
    stubServer();
    const w = await mountScreen(ZhPinyin, { route: '/zh-pinyin' });
    await flushPromises();

    expect(checkedMode(w)).toBe('round');
  });
});
