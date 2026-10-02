// Kho link: chữ ký operation + lọc.
//
// Trọng tâm là 2 chỗ dễ sai đã được M6a ghi:
//   1. `BookmarkStatus` KHÁC `Status` của node roadmap — test này khẳng định
//      `setBookmarkStatus` gửi đúng giá trị enum của bookmark.
//   2. Lọc tag khớp theo PHẦN TỬ CSV, và UI chuẩn hoá tag TRƯỚC khi gửi vì
//      server đếm độ dài trước khi chuẩn hoá.
import { afterEach, describe, expect, it, vi } from 'vitest';
import { resetStaleMarks, resetUrqlClient } from '../graphql/client';
import { queryClient } from '../rest/queryClient';
import { bodyOf, operationNameOf } from '../test/graphqlMock';
import {
  createBookmark,
  deleteBookmark,
  fetchBookmarks,
  normalizeTags,
  setBookmarkStatus,
  updateBookmark,
} from './api';

afterEach(() => {
  vi.unstubAllGlobals();
  resetUrqlClient();
  resetStaleMarks();
  queryClient.clear();
});

const SAVED = {
  id: '55',
  guid: 'b55',
  title: 'Bài 1',
  url: null,
  note: '',
  tags: 'hsk3',
  tagList: ['hsk3'],
  status: 'TO_READ' as const,
  createdAt: 'x',
  updatedAt: 'x',
};

function stub(payload: unknown) {
  const fetchMock = vi.fn().mockImplementation(async () => new Response(JSON.stringify({ data: payload }), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  }));
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

interface SentVariables {
  id?: string;
  status?: string;
  input: { tags?: unknown; url?: unknown };
  patch: Record<string, unknown>;
}

function lastMutation(fetchMock: ReturnType<typeof vi.fn>): SentVariables {
  return bodyOf(fetchMock, fetchMock.mock.calls.length - 1).variables as unknown as SentVariables;
}

describe('test_normalize_tags', () => {
  it('test_trims_lowercases_and_dedupes', () => {
    expect(normalizeTags([' HSK3 ', 'hsk3', 'Ngữ Pháp'])).toEqual(['hsk3', 'ngữ pháp']);
  });

  it('test_blank_tags_are_dropped', () => {
    expect(normalizeTags(['', '   ', 'a'])).toEqual(['a']);
  });
});

describe('test_fetch_bookmarks_filters', () => {
  it('test_sends_status_and_tag_variables', async () => {
    const fetchMock = stub({ bookmarks: [SAVED] });
    await fetchBookmarks({ status: 'READING', tag: 'hsk3' });
    expect(operationNameOf(fetchMock)).toBe('Bookmarks');
    expect(bodyOf(fetchMock).variables).toEqual({ status: 'READING', tag: 'hsk3' });
  });

  it('test_no_filter_sends_explicit_nulls', async () => {
    // `null` = không lọc. Gửi `undefined` làm urql bỏ hẳn biến khỏi query
    // string, tức operation khác — cache key khác, invalidate không trúng.
    const fetchMock = stub({ bookmarks: [] });
    await fetchBookmarks();
    expect(bodyOf(fetchMock).variables).toEqual({ status: null, tag: null });
  });
});

describe('test_set_bookmark_status_uses_bookmark_enum', () => {
  it('test_sends_bookmark_status_not_roadmap_status', async () => {
    const fetchMock = stub({ setBookmarkStatus: { ok: true, bookmark: SAVED, error: null } });
    await setBookmarkStatus('55', 'DONE');
    const body = lastMutation(fetchMock);
    expect(operationNameOf(fetchMock, fetchMock.mock.calls.length - 1)).toBe('SetBookmarkStatus');
    // `NOT_STARTED`/`IN_PROGRESS` là giá trị của `Status` node roadmap, gửi cho
    // bookmark là 400.
    expect(body).toEqual({ id: '55', status: 'DONE' });
  });
});

describe('test_create_bookmark_normalises_before_sending', () => {
  it('test_tags_are_trimmed_and_lowercased_in_the_request', async () => {
    const fetchMock = stub({ createBookmark: { ok: true, bookmark: SAVED, error: null } });
    await createBookmark({ title: 'T', tags: [' HSK3 ', 'hsk3'] });
    expect(lastMutation(fetchMock).input.tags).toEqual(['hsk3']);
  });

  it('test_absent_url_is_sent_as_null_not_empty_string', async () => {
    const fetchMock = stub({ createBookmark: { ok: true, bookmark: SAVED, error: null } });
    await createBookmark({ title: 'T' });
    expect(lastMutation(fetchMock).input.url).toBeNull();
  });
});

describe('test_update_bookmark_url_contract', () => {
  it('test_clear_url_flag_is_sent_when_link_is_removed', async () => {
    const fetchMock = stub({ updateBookmark: { ok: true, bookmark: SAVED, error: null } });
    await updateBookmark('55', { title: 'T', clearUrl: true });
    expect(lastMutation(fetchMock).patch.clearUrl).toBe(true);
  });

  it('test_empty_tag_array_is_sent_to_remove_all_tags', async () => {
    const fetchMock = stub({ updateBookmark: { ok: true, bookmark: SAVED, error: null } });
    await updateBookmark('55', { tags: [] });
    // Không gửi `tags` = giữ nguyên; `[]` = xoá hết. Hai thứ khác nhau.
    expect(lastMutation(fetchMock).patch.tags).toEqual([]);
  });
});

describe('test_delete_bookmark', () => {
  it('test_calls_the_soft_delete_mutation', async () => {
    const fetchMock = stub({ deleteBookmark: { ok: true, error: null } });
    await deleteBookmark('55');
    expect(operationNameOf(fetchMock)).toBe('DeleteBookmark');
    expect(lastMutation(fetchMock)).toEqual({ id: '55' });
  });
});
