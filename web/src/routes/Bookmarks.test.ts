// `/bookmarks` — kho link.
//
// Trọng tâm là bộ lọc: `status` + `tag` gửi thẳng xuống server, và lọc tag là
// so PHẦN TỬ CSV chứ không phải so chuỗi con (M6a §1.2). Case viền `a` vs `ab`
// chạy ở CẢ 2 tầng: server đã test trên SQL thật, test ở đây bảo đảm UI không
// tự lọc bằng `LIKE`/`includes` ở client — lọc sai thì người dùng thấy kết quả
// sai mà không có cách nào phát hiện.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises } from '@vue/test-utils';
import { resetStaleMarks, resetUrqlClient } from '../graphql/client';
import { queryClient } from '../rest/queryClient';
import { bodyOf, operationNameOf } from '../test/graphqlMock';
import { mountScreen, type MountedScreen } from '../test/harness';
import Bookmarks from './Bookmarks.vue';
import type { Bookmark, BookmarkStatus } from '../bookmarks/api';

function bm(id: string, over: Partial<Bookmark> = {}): Bookmark {
  return {
    id,
    guid: `g${id}`,
    title: `Trang ${id}`,
    url: `https://example.com/${id}`,
    note: '',
    tags: '',
    tagList: [],
    status: 'TO_READ',
    createdAt: 'x',
    updatedAt: 'x',
    ...over,
  };
}

const ALL = [
  bm('1', { tagList: ['a'], tags: 'a', title: 'Chữ a' }),
  bm('2', { tagList: ['ab'], tags: 'ab', title: 'Chữ ab' }),
  bm('3', { tagList: ['abc'], tags: 'abc', title: 'Chữ abc' }),
  bm('4', { tagList: ['a', 'ba'], tags: 'a,ba', title: 'Cả hai', status: 'DONE' }),
];

/** Lọc đúng luật server: so phần tử CSV sau khi hạ chữ thường + trim. */
function serverFilter(rows: Bookmark[], status: BookmarkStatus | null, tag: string | null): Bookmark[] {
  const want = (tag ?? '').trim().toLowerCase();
  return rows.filter((b) => {
    if (status !== null && b.status !== status) return false;
    if (want === '') return true;
    return b.tagList.includes(want);
  });
}

let fetchMock: ReturnType<typeof vi.fn>;

/**
 * Stub server thật hiện hành vi server: lọc theo phần tử CSV + AND 2 điều kiện.
 * Đọc `variables` qua `bodyOf` vì urql gửi QUERY bằng GET và gói biến vào 1
 * tham số JSON `?variables=…`, không phải 2 tham số rời.
 */
function stub() {
  fetchMock = vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const [url, req] = [String(input), init];
    const vars = (req?.body
      ? (JSON.parse(req.body as string) as { variables: Record<string, unknown> }).variables
      : (new URL(url, 'http://x').searchParams.get('variables')
          ? (JSON.parse(new URL(url, 'http://x').searchParams.get('variables') as string) as Record<string, unknown>)
          : {})) as { status?: BookmarkStatus | null; tag?: string | null };
    const data = { bookmarks: serverFilter(ALL, vars.status ?? null, vars.tag ?? null) };
    return new Response(JSON.stringify({ data }), { status: 200, headers: { 'content-type': 'application/json' } });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

async function lastQueryVars(): Promise<Record<string, unknown>> {
  const i = fetchMock.mock.calls.length - 1;
  return bodyOf(fetchMock, i).variables;
}

function rowTitles(w: MountedScreen): string[] {
  return w.findAll('[data-testid^="bookmark-row-"]').map((r) => r.text());
}

beforeEach(() => {
  vi.unstubAllGlobals();
  resetUrqlClient();
  resetStaleMarks();
  queryClient.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('test_bookmark_list', () => {
  it('test_reads_bookmarks_without_a_filter_by_default', async () => {
    stub();
    const w = await mountScreen(Bookmarks, { route: '/bookmarks' });
    await flushPromises();
    expect(operationNameOf(fetchMock)).toBe('Bookmarks');
    expect(rowTitles(w)).toHaveLength(4);
  });

  it('test_link_without_url_is_shown_not_hidden', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation(async () =>
        new Response(JSON.stringify({ data: { bookmarks: [bm('9', { url: null, title: 'Ghi chú nhớ' })] } }), {
          status: 200,
          headers: { 'content-type': 'application/json' },
        }),
      ),
    );
    const w = await mountScreen(Bookmarks, { route: '/bookmarks' });
    await flushPromises();
    const row = w.get('[data-testid="bookmark-row-9"]');
    expect(row.text()).toContain('Ghi chú nhớ');
    expect(row.text()).toContain('không có link');
    expect(row.find('a').exists()).toBe(false);
  });
});

describe('test_filter_by_status', () => {
  it('test_choosing_a_status_sends_that_enum_value', async () => {
    stub();
    const w = await mountScreen(Bookmarks, { route: '/bookmarks' });
    await flushPromises();

    await w.get('select[aria-label="lọc theo trạng thái"]').setValue('DONE');
    for (let i = 0; i < 8; i += 1) await flushPromises();

    expect((await lastQueryVars()).status).toBe('DONE');
  });

  it('test_status_filter_narrows_the_list', async () => {
    stub();
    const w = await mountScreen(Bookmarks, { route: '/bookmarks' });
    await flushPromises();

    await w.get('select[aria-label="lọc theo trạng thái"]').setValue('DONE');
    for (let i = 0; i < 8; i += 1) await flushPromises();

    expect(rowTitles(w)).toHaveLength(1);
    expect(rowTitles(w)[0]).toContain('Cả hai');
  });

  it('test_clearing_the_status_filter_restores_everything', async () => {
    stub();
    const w = await mountScreen(Bookmarks, { route: '/bookmarks' });
    await flushPromises();

    await w.get('select[aria-label="lọc theo trạng thái"]').setValue('DONE');
    for (let i = 0; i < 8; i += 1) await flushPromises();
    await w.findAll('button').find((b) => b.text() === 'Bỏ lọc')!.trigger('click');
    for (let i = 0; i < 8; i += 1) await flushPromises();

    expect(rowTitles(w)).toHaveLength(4);
  });
});

describe('test_filter_by_tag_uses_whole_elements', () => {
  it('test_tag_dropdown_is_built_from_existing_tags', async () => {
    stub();
    const w = await mountScreen(Bookmarks, { route: '/bookmarks' });
    await flushPromises();
    const options = w.get('select[aria-label="lọc theo tag"]').findAll('option').map((o) => o.element.value);
    expect(options).toEqual(expect.arrayContaining(['', 'a', 'ab', 'abc', 'ba']));
  });

  it('test_filtering_by_a_excludes_ab_and_abc', async () => {
    // Case viền: `LIKE '%a%'` hay `includes('a')` đều ra 4 dòng, trong khi
    // server chỉ trả 2 (tag `a` và `a,ba`).
    stub();
    const w = await mountScreen(Bookmarks, { route: '/bookmarks' });
    await flushPromises();

    await w.get('select[aria-label="lọc theo tag"]').setValue('a');
    for (let i = 0; i < 8; i += 1) await flushPromises();

    expect((await lastQueryVars()).tag).toBe('a');
    const titles = rowTitles(w);
    expect(titles).toHaveLength(2);
    expect(titles.join(' ')).not.toContain('Chữ ab');
    expect(titles.join(' ')).not.toContain('Chữ abc');
  });

  it('test_filtering_by_ab_returns_only_ab', async () => {
    stub();
    const w = await mountScreen(Bookmarks, { route: '/bookmarks' });
    await flushPromises();

    await w.get('select[aria-label="lọc theo tag"]').setValue('ab');
    for (let i = 0; i < 8; i += 1) await flushPromises();

    const titles = rowTitles(w);
    expect(titles).toHaveLength(1);
    expect(titles[0]).toContain('Chữ ab');
  });

  it('test_status_and_tag_filters_combine_with_and', async () => {
    stub();
    const w = await mountScreen(Bookmarks, { route: '/bookmarks' });
    await flushPromises();

    await w.get('select[aria-label="lọc theo trạng thái"]').setValue('DONE');
    for (let i = 0; i < 8; i += 1) await flushPromises();
    await w.get('select[aria-label="lọc theo tag"]').setValue('a');
    for (let i = 0; i < 8; i += 1) await flushPromises();

    const vars = await lastQueryVars();
    expect(vars).toMatchObject({ status: 'DONE', tag: 'a' });
    expect(rowTitles(w)).toHaveLength(1);
  });
});

describe('test_bookmark_actions', () => {
  it('test_status_change_uses_the_dedicated_mutation', async () => {
    stub();
    const w = await mountScreen(Bookmarks, { route: '/bookmarks' });
    await flushPromises();

    const perRow = w.findAll('select[aria-label^="đổi trạng thái"]');
    expect(perRow.length, 'mỗi dòng phải có 1 bộ đổi trạng thái').toBe(4);
    await perRow[0].setValue('ARCHIVED');
    for (let i = 0; i < 8; i += 1) await flushPromises();

    // `updateBookmark` không có `status` trong patch — gửi status vào đó là 400.
    const bodies = fetchMock.mock.calls
      .map((c) => (c[1]?.body ? JSON.parse(c[1].body as string) as { operationName: string; variables: Record<string, unknown> } : null))
      .filter((b): b is { operationName: string; variables: Record<string, unknown> } => b !== null);
    expect(bodies.map((b) => b.operationName)).toContain('SetBookmarkStatus');
    const call = bodies.find((b) => b.operationName === 'SetBookmarkStatus')!;
    expect(call.variables).toEqual({ id: '1', status: 'ARCHIVED' });
  });

  it('test_delete_asks_for_confirmation_before_calling', async () => {
    stub();
    const w = await mountScreen(Bookmarks, { route: '/bookmarks' });
    await flushPromises();

    await w.findAll('button').find((b) => b.text() === 'Xoá')!.trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();
    expect(fetchMock.mock.calls.some((c) => c[1]?.body && (c[1].body as string).includes('DeleteBookmark'))).toBe(false);

    await w.findAll('button').find((b) => b.text() === 'Xoá thật')!.trigger('click');
    for (let i = 0; i < 8; i += 1) await flushPromises();
    expect(fetchMock.mock.calls.some((c) => c[1]?.body && (c[1].body as string).includes('DeleteBookmark'))).toBe(true);
  });
});
