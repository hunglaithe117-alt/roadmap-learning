// BẮT BUỘC của gate M4 (gotcha #2): lỗi GraphQL phải hiện `error.message`
// tiếng Việt NGUYÊN VĂN của server. Đổi thành "lỗi hệ thống" hoặc dịch lại
// ở client là sai hợp đồng (M4 §6.1) và test này đỏ.
//
// Màn Review được chọn vì nó là màn ôn SRS đầu tiên người dùng gặp và nó
// render lỗi qua `NoticeBar` — kiểm được cả tầng data lẫn tầng UI.
import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushPromises } from '@vue/test-utils';
import Review from './Review.vue';
import { mountScreen, type MountedScreen } from '../test/harness';
import { resetStaleMarks, resetUrqlClient } from '../graphql/client';
import { queryClient } from '../rest/queryClient';

const VIETNAMESE = 'grade phải từ 1 đến 4 (1=Quên, 4=Dễ)';

afterEach(() => {
  vi.unstubAllGlobals();
  resetUrqlClient();
  resetStaleMarks();
  queryClient.clear();
});

const CARD = {
  id: '7',
  deckId: '1',
  front: '你好',
  back: 'xin chào',
  pinyin: 'nǐ hǎo',
  dueAt: 'x',
  state: 'new',
  tone: null,
  audioURL: null,
};

const OTHER_CARD = { ...CARD, id: '8', front: '再见', back: 'tạm biệt', pinyin: 'zài jiàn' };

const DECKS = [{ id: '1', guid: 'g', name: 'HSK1', lang: 'zh', createdAt: 'x' }];

/**
 * Server cho `?card=`. `Cards` trả cả 2 thẻ (dùng để `fetchCardById` dò tìm
 * theo id — schema không có `card(id:)`), `due` là nội dung hàng đợi đến hạn
 * mà test muốn.
 */
function stubFocusServer(due: typeof CARD[] = [OTHER_CARD]) {
  const fetchMock = vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const name =
      new URL(String(input), 'http://localhost').searchParams.get('operationName') ??
      (init?.body ? (JSON.parse(init.body as string) as { operationName: string }).operationName : '');
    const data =
      name === 'DueCards'
        ? { decks: DECKS, dueCards: due }
        : name === 'Cards'
          ? { cards: [CARD, OTHER_CARD] }
          : name === 'RecordReview'
            ? {
                recordReview: {
                  ok: true,
                  review: { cardId: '7', dueAt: '2026-10-01', intervalDays: 3, stability: 1, difficulty: 1, reps: 1, fallback: true },
                  error: null,
                },
              }
            : { decks: DECKS };
    return new Response(JSON.stringify({ data }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

/** Server trả 1 thẻ đến hạn; `recordReview` thì theo `reviewPayload` truyền vào. */
function stubReviewServer(reviewPayload: Record<string, unknown>) {
  const fetchMock = vi.fn().mockImplementation(async (input: RequestInfo | URL) => {
    const name =
      new URL(String(input), 'http://localhost').searchParams.get('operationName') ?? '';
    const data =
      name === 'DueCards' ? { decks: DECKS, dueCards: [CARD] } : { decks: DECKS, ...reviewPayload };
    return new Response(JSON.stringify({ data }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

/** Bấm "Hiện đáp án" rồi bấm nút chấm `3.`. */
async function revealAndGrade(wrapper: MountedScreen) {
  const reveal = wrapper.findAll('button').find((b) => b.text().includes('Hiện đáp án'));
  expect(reveal, 'không thấy nút hiện đáp án').toBeTruthy();
  await reveal!.trigger('click');
  await flushPromises();
  const grade3 = wrapper.findAll('button').find((b) => b.text().startsWith('3.'));
  expect(grade3, 'không thấy nút chấm 3.').toBeTruthy();
  await grade3!.trigger('click');
  for (let i = 0; i < 12; i += 1) await flushPromises();
}

describe('test_review_shows_graphql_error_message_verbatim', () => {
  it('test_business_error_message_renders_unescaped_vietnamese', async () => {
    // `recordReview` là mutation CÓ payload lỗi (`RecordReviewPayload` mang
    // `ok` + `error { message code }`) — đúng hợp đồng M4 đã đóng băng.
    // `dueCards` là list thuần `[Card!]!` nên không mang được lỗi nghiệp vụ.
    stubReviewServer({
      recordReview: { ok: false, review: null, error: { message: VIETNAMESE, code: 'BAD_REQUEST' } },
    });

    // Mở thẳng `#/review?deck=3` — hash cũ của app v1.
    const wrapper = await mountScreen(Review, { route: '/review?deck=3' });
    for (let i = 0; i < 12; i += 1) await flushPromises();
    await revealAndGrade(wrapper);

    // Message tiếng Việt NGUYÊN VĂN của server phải lên UI.
    expect(wrapper.text()).toContain(VIETNAMESE);
    // KHÔNG được thay bằng thông điệp chung — đó là lỗi báo đỏ từng mắc ở M3.
    expect(wrapper.text()).not.toContain('lỗi hệ thống');
  });

  it('test_failed_grade_keeps_card_in_queue', async () => {
    // Lỗi thì KHÔNG bỏ thẻ khỏi hàng đợi — bỏ thẻ rồi mới hiện lỗi là mất
    // luôn thẻ đó vĩnh viễn trong phiên (app v1 cũng giữ nguyên hành vi này).
    stubReviewServer({
      recordReview: { ok: false, review: null, error: { message: VIETNAMESE, code: 'BAD_REQUEST' } },
    });
    const wrapper = await mountScreen(Review, { route: '/review?deck=3' });
    for (let i = 0; i < 12; i += 1) await flushPromises();
    await revealAndGrade(wrapper);

    expect(wrapper.text()).toContain('你好');
    expect(wrapper.text()).not.toContain('Hết bài hôm nay');
  });
});

describe('test_review_grade_flow', () => {
  it('test_grading_card_shows_server_schedule_and_advances_queue', async () => {
    stubReviewServer({
      recordReview: {
        ok: true,
        review: {
          cardId: '7',
          dueAt: '2026-10-01',
          intervalDays: 3,
          stability: 1,
          difficulty: 1,
          reps: 1,
          fallback: true,
        },
        error: null,
      },
    });

    const wrapper = await mountScreen(Review, { route: '/review?deck=3' });
    for (let i = 0; i < 12; i += 1) await flushPromises();

    expect(wrapper.text()).toContain('你好');
    expect(wrapper.text()).toContain('nǐ hǎo');
    await revealAndGrade(wrapper);

    // Lịch lần sau do SERVER tính, client chỉ hiện lại — không tự tính.
    expect(wrapper.text()).toContain('Đã lưu. Thẻ tiếp theo đến hạn sau 3 ngày (2026-10-01).');
    // Hết hàng đợi sau 1 thẻ.
    expect(wrapper.text()).toContain('Hết bài hôm nay');
  });
});

// F2 (M6 remediation) — `/review?card=` là link do `ErrorBook.vue` render cho
// từ sai nhiều nhất. Trước đó màn này **chỉ đọc `query.deck`** ⇒ bấm link ra
// màn trắng "Chọc deck để bắt đầu ôn", không lỗi, không thẻ.
describe('test_review_opens_the_card_from_the_query_string', () => {
  it('test_query_card_puts_that_card_at_the_head_of_the_queue', async () => {
    stubFocusServer();
    const wrapper = await mountScreen(Review, { route: '/review?card=7' });
    for (let i = 0; i < 12; i += 1) await flushPromises();

    // Thẻ 7 (`你好`) LÊN TRƯỚC thẻ 8 (`再见`) dù hàng đợi đến hạn chỉ có 8.
    expect(wrapper.text()).toContain('你好');
    expect(wrapper.text()).not.toContain('再见');
  });

  it('test_query_card_also_selects_the_deck_so_the_screen_leaves_its_empty_state', async () => {
    stubFocusServer();
    const wrapper = await mountScreen(Review, { route: '/review?card=7' });
    for (let i = 0; i < 12; i += 1) await flushPromises();

    // Không có bước này thì `deckId` vẫn rỗng và `v-if="!deckId"` giữ màn
    // trắng dù hàng đợi đã có thẻ.
    expect(wrapper.text()).not.toContain('Chọc deck để bắt đầu ôn');
    expect((wrapper.find('select').element as HTMLSelectElement).value).toBe('1');
  });

  it('test_query_card_survives_grading_and_hands_over_to_the_rest_of_the_queue', async () => {
    // Chấm xong thẻ `?card=` ⇒ phần còn lại của hàng đợi phải hiện tiếp, đừng
    // rơi vào "Hết bài hôm nay" dù vẫn còn thẻ đến hạn.
    stubFocusServer();
    const wrapper = await mountScreen(Review, { route: '/review?card=7' });
    for (let i = 0; i < 12; i += 1) await flushPromises();
    await revealAndGrade(wrapper);

    expect(wrapper.text()).toContain('再见');
    expect(wrapper.text()).not.toContain('Hết bài hôm nay');
  });

  it('test_query_card_does_not_duplicate_a_card_already_in_the_due_queue', async () => {
    // Thẻ 7 vốn ĐÃ nằm trong hàng đợi đến hạn ⇒ đặt lên đầu không được nhân
    // đôi (chấm 1 lần phải mất đúng 1 thẻ, không phải 2).
    stubFocusServer([CARD, OTHER_CARD]);
    const wrapper = await mountScreen(Review, { route: '/review?card=7' });
    for (let i = 0; i < 12; i += 1) await flushPromises();
    // Chấm thẻ 7 rồi phải ra thẻ 8 — nếu thẻ 7 bị nhân đôi, lần chấm kế vẫn là
    // thẻ 7 và màn quay lại `你好`.
    await revealAndGrade(wrapper);
    expect(wrapper.text()).toContain('再见');
  });

  it('test_unknown_card_says_so_instead_of_showing_a_blank_screen', async () => {
    // `cardId` NULL là 1 trong 3 trường hợp `TopErrorWithCard` (M6a §2.3), và
    // thẻ có thể bị xoá mềm sau khi link đã được gửi đi. Im lặng rơi về "Chọc
    // deck" là đúng loại lỗi mà F2 này sinh ra.
    stubFocusServer();
    const wrapper = await mountScreen(Review, { route: '/review?card=999' });
    for (let i = 0; i < 12; i += 1) await flushPromises();

    expect(wrapper.text()).toContain('Không tìm thấy thẻ này');
  });

  it('test_review_without_card_param_behaves_exactly_as_before', async () => {
    // `?card=` rỗng ⇒ `focusCard` null ⇒ hàng đợi y hệt cũ. Chặn "sửa F2
    // làm đổi hành vi mặc định".
    stubFocusServer();
    const wrapper = await mountScreen(Review, { route: '/review?deck=1' });
    for (let i = 0; i < 12; i += 1) await flushPromises();

    expect(wrapper.text()).toContain('再见');
    expect(wrapper.text()).not.toContain('你好');
  });
});
