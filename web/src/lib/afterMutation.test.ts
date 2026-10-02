// BẮT BUỘC của gate M4: "sau mỗi mutation phải invalidate cả cache urql lẫn
// cache vue-query" — đây là nửa nguy hiểm nhất của kiến trúc 2 protocol
// (STACK-V2 §3), nên test KHÔNG mock 2 module rồi assert "đã gọi" (test đó xanh
// kể cả khi hàm gọi sai hàm). Ở đây test HÀNH VI thật: cache có thật, invalidate
// có thật, số request mạng đếm được.
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { afterMutation } from './afterMutation';
import { Decks } from '../graphql/operations';
import { resetStaleMarks, resetUrqlClient, runQuery } from '../graphql/client';
import { restKeys, queryClient } from '../rest/queryClient';

/** Response GraphQL tối thiểu mà `fetchExchange` của urql chấp nhận. */
function decksResponse() {
  return new Response(JSON.stringify({ data: { decks: [] } }), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  });
}

/** Mỗi lần gọi trả Response MỚI — Response không đọc 2 lần được. */
function countingFetch() {
  return vi.fn().mockImplementation(async () => decksResponse());
}

beforeEach(() => {
  resetUrqlClient();
  resetStaleMarks();
  queryClient.clear();
});

describe('test_after_mutation_invalidates_urql_cache', () => {
  it('test_second_read_same_generation_hits_cache_only', async () => {
    const fetchMock = countingFetch();
    vi.stubGlobal('fetch', fetchMock);

    await runQuery(Decks);
    await runQuery(Decks);

    // Cùng "kỷ nguyên" thì đọc lần 2 rẻ — đây là điều kiện để test dưới có ý
    // nghĩa: nếu runQuery lúc nào cũng gọi mạng thì test sau xanh nhầm.
    expect(fetchMock).toHaveBeenCalledTimes(1);
    vi.unstubAllGlobals();
  });

  it('test_read_after_after_mutation_goes_back_to_network', async () => {
    const fetchMock = countingFetch();
    vi.stubGlobal('fetch', fetchMock);

    await runQuery(Decks);
    await afterMutation();
    await runQuery(Decks);

    expect(fetchMock).toHaveBeenCalledTimes(2);
    vi.unstubAllGlobals();
  });
});

describe('test_after_mutation_invalidates_vue_query_cache', () => {
  it('test_rest_query_marked_invalidated_by_after_mutation', async () => {
    const key = restKeys.health();
    await queryClient.fetchQuery({ queryKey: key, queryFn: () => 'ok' });
    expect(queryClient.getQueryState(key)?.isInvalidated).toBe(false);

    await afterMutation();

    expect(queryClient.getQueryState(key)?.isInvalidated).toBe(true);
  });

  it('test_after_mutation_invalidates_by_rest_namespace', async () => {
    const spy = vi.spyOn(queryClient, 'invalidateQueries');
    await afterMutation();
    expect(spy).toHaveBeenCalledWith({ queryKey: ['api'] });
    spy.mockRestore();
  });
});
