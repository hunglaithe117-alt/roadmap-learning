// F3 (cổng Oracle M5) — `cacheExchange` của urql KHÔNG tự invalidate được nếu
// selection set thiếu `__typename`.
//
// Test này đo bằng **số request thật**, không đọc comment. Ma trận 2×2 đo thật
// trên `@urql/core` 6.0.3 cho kết quả DUY NHẤT (chỉ khi CẢ HAI vế có
// `__typename` thì cache mới tự làm mông):
//
//   đọc có   ghi có    → rereadHitNetwork = true
//   đọc có   ghi không → false
//   đọc không ghi có    → false
//   đọc không ghi không → false
//
// ⇒ `__typename` phải có ở CẢ selection set của query LẪN mutation. Đây là
// điều kiện để `cacheExchange` tự invalidate được; nhưng nó KHÔNG đủ thay
// `afterMutation()` (mutation trả entity KHÁC loại như `ReviewResult` không
// chạm vào `Card`) — `mutationInvalidation.test.ts` là nơi chứng minh điều đó.
import { describe, expect, it, vi } from 'vitest';
import { gql, Client, cacheExchange, fetchExchange, type TypedDocumentNode } from '@urql/core';
import { print } from 'graphql';
import { CARD_FIELDS, DECK_FIELDS, Decks, CreateDeckMutation } from './operations';
import { resetStaleMarks, resetUrqlClient, runMutation, runQuery } from './client';

/**
 * Nội dung câu query dạng chữ.
 *
 * KHÔNG dùng `doc.loc.source.body`: field đó rỗng với document viết 1 dòng
 * (đã kiểm chứng) ⇒ regex luôn `false` ⇒ test xanh nhầm. `print()` serialize
 * từ AST nên đúng với mọi hình thức viết.
 */
function queryText(d: unknown): string {
  return print(d as Parameters<typeof print>[0]);
}

const READ_NO_TN = gql`query Probe { deck { id name } }` as unknown as TypedDocumentNode<any, any>;
const READ_TN = gql`query ProbeT { deck { __typename id name } }` as unknown as TypedDocumentNode<any, any>;
const WRITE_NO_TN = gql`mutation ProbeW { createDeck { ok deck { id name } error { message code } } }` as unknown as TypedDocumentNode<any, any>;
const WRITE_TN = gql`mutation ProbeWT { createDeck { ok deck { __typename id name } error { message code } } }` as unknown as TypedDocumentNode<any, any>;

/**
 * warm cache → mutation → đọc lại `cache-first`. Trả về có ra mạng không (đếm
 * CHÍNH xác, trừ request của chính mutation) + giá trị đọc được.
 *
 * 2 điều kiện kỹ thuật, cả hai đều phải đúng thì cache mới tự làm mông:
 *   1. `operationName` đọc từ CẨ query string (GET) và body (POST) — urql chọn
 *      GET cho query, POST cho mutation. Chỉ đọc query string là khiến mutation
 *      bị coi như không có `__typename` và mọi kết luận về nó trở nên sai.
 *   2. Response của mutation phải chứa `__typename` ĐÚNG KHI mutation xin nó
 *      — `cacheExchange` đọc trường có sẵn trong payload, không đọc document.
 *      Nên mock bám theo document: có `__typename` trong câu mutation thì mới
 *      trả `__typename` trong response.
 */
async function probe(read: unknown, write: unknown, readTn: boolean) {
  const writeAsksTypename = /__typename/.test(queryText(write));
  const fetchMock = vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    let name = new URL(String(input), 'http://localhost').searchParams.get('operationName') ?? '';
    let isMutation = false;
    if (init?.body) {
      const body = JSON.parse(init.body as string) as { operationName?: string; query?: string };
      name = body.operationName ?? name;
      isMutation = /mutation/.test(body.query ?? '');
    }
    const deck: Record<string, unknown> = { id: '1', name: 'OLD' };
    if (readTn) deck.__typename = 'Deck';
    const written: Record<string, unknown> = { id: '1', name: 'NEW' };
    if (writeAsksTypename) written.__typename = 'Deck';
    const data = isMutation
      ? { createDeck: { ok: true, deck: written, error: null } }
      : { deck };
    return new Response(JSON.stringify({ data }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  });
  vi.stubGlobal('fetch', fetchMock);

  const client = new Client({ url: 'http://localhost/query', exchanges: [cacheExchange, fetchExchange] });
  await client.query(read as never, {}, { requestPolicy: 'network-only' }).toPromise();
  await client.mutation(write as never, {}, { requestPolicy: 'network-only' }).toPromise();
  const afterMutationCount = fetchMock.mock.calls.length;
  const reread = await client.query(read as never, {}, { requestPolicy: 'cache-first' }).toPromise();

  const result = {
    rereadHitNetwork: fetchMock.mock.calls.length > afterMutationCount,
    rereadValue: (reread.data as { deck?: { name: string } })?.deck?.name,
  };
  vi.unstubAllGlobals();
  return result;
}

describe('test_cacheExchange_needs_typename_on_both_sides', () => {
  it('test_read_va_write_che_typename_thi_cache_tu_lam_mong', async () => {
    const r = await probe(READ_TN, WRITE_TN, true);
    expect(r.rereadHitNetwork, 'có `__typename` ở cả hai vế ⇒ cache tự làm mông').toBe(true);
  });

  it('test_read_co_typename_nhung_write_khong_thi_van_bi_tra_cache_cu', async () => {
    // Nhánh này loại bỏ giả thuyết "chỉ cần `__typename` ở query" — đó là điều
    // dễ tin nhầm nhất vì nghe có vẻ hợp lý.
    const r = await probe(READ_TN, WRITE_NO_TN, true);
    expect(r.rereadHitNetwork).toBe(false);
    expect(r.rereadValue).toBe('OLD');
  });

  it('test_ca_hai_thieu_typename_thi_doc_lai_bi_tra_cache_cu', async () => {
    const r = await probe(READ_NO_TN, WRITE_NO_TN, false);
    expect(r.rereadHitNetwork).toBe(false);
    expect(r.rereadValue).toBe('OLD');
  });
});

describe('test_operations_actually_request_typename', () => {
  it('test_DECK_FIELDS_va_CARD_FIELDS_co_typename', () => {
    // Nhánh 3 của bằng chứng: app thật phải ở nhánh "có `__typename` ở cả hai
    // vế". Xoá `__typename` khỏi `operations.ts` là test này đỏ (đo được).
    expect(queryText(DECK_FIELDS)).toMatch(/__typename/);
    expect(queryText(CARD_FIELDS)).toMatch(/__typename/);
    // Fragment phải xin `__typename` ở CẢ HAI vị trí dùng nó, và mutation
    // `CreateDeck` trả `Deck` cũng phải xin — nếu không thì nhánh "cache tự làm
    // mông" ở trên không áp dụng cho app.
    expect(queryText(CreateDeckMutation)).toMatch(/__typename/);
  });

  it('test_tao_deck_roi_doc_lai_Decks_thay_vi_thay_cache_cu', async () => {
    // Hành vi thật của app: `createDeck` (có `__typename`) rồi `fetchDecks`.
    // Lớp 2 (`__typename`) đang hoạt động — nhưng lớp bảo đảm DUY NHẤT vẫn là
    // `afterMutation()`, chứng minh đầy đủ ở `mutationInvalidation.test.ts`.
    const fetchMock = vi.fn().mockImplementation(async (input: RequestInfo | URL) => {
      const name = new URL(String(input), 'http://localhost').searchParams.get('operationName') ?? '';
      const deck = { __typename: 'Deck', id: '2', guid: 'g2', name: 'NEW', lang: 'zh', createdAt: 'x' };
      const data = name === 'Decks' ? { decks: [deck] } : { createDeck: { ok: true, deck, error: null } };
      return new Response(JSON.stringify({ data }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    resetUrqlClient();
    resetStaleMarks();

    await runQuery(Decks);
    await runMutation(CreateDeckMutation, { name: 'NEW', lang: 'zh' });
    const before = fetchMock.mock.calls.length;
    const reread = await runQuery(Decks);

    expect(fetchMock.mock.calls.length).toBeGreaterThan(before);
    expect(reread.decks[0]?.name).toBe('NEW');
    // `__typename` đi kèm ra mạng — server trả cái này, không phải client bịa.
    expect(reread.decks[0]).toHaveProperty('__typename', 'Deck');

    vi.unstubAllGlobals();
  });
});
