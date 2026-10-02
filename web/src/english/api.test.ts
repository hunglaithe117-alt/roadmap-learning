// Port `english/api.test.ts` (4) sang client `content` phía Anh của app-v2.
// App-v2 gộp 5 endpoint `/api/en/*` vào GraphQL nên assert operation + variables.
import { afterEach, describe, expect, it, vi } from 'vitest';
import { resetStaleMarks, resetUrqlClient } from '../graphql/client';
import { bodyOf, mockData, operationNameOf } from '../test/graphqlMock';
import {
  fetchEnSearch,
  fetchEnStress,
  fetchThieuHistory,
  postEnChunks,
  postThieu,
} from './api';

afterEach(() => {
  vi.unstubAllGlobals();
  resetUrqlClient();
  resetStaleMarks();
});


describe('test_en_api', () => {
  it('test_fetch_en_search_returns_entries', async () => {
    const sample = [{ lang: 'en', term: 'photograph', reading: '/ˈfəʊtəɡrɑːf/', gloss: 'bức ảnh' }];
    const fetchMock = mockData({ englishSearch: sample });
    expect(await fetchEnSearch('photograph')).toEqual(sample);
    expect(operationNameOf(fetchMock)).toBe('EnglishSearch');
  });

  it('test_fetch_en_stress_exception_flagged', async () => {
    mockData({
      stress: {
        term: 'present',
        ipa: '…',
        stress: '',
        exception: true,
        note: 'ngoại lệ',
        fromDict: false,
      },
    });
    const r = await fetchEnStress('present');
    expect(r.exception).toBe(true);
    // `fromDict` ở schema map thành `source` của client — mất nó thì UI không
    // biết chỉ đoán bằng quy tắc hay tra từ điển.
    expect(r.source).toBe('rule');
  });

  it('test_post_en_chunks_returns_chunks', async () => {
    mockData({
      chunk: {
        ok: true,
        chunks: [
          { text: 'I', kind: 'FUNCTION' },
          { text: 'progress', kind: 'CONTENT' },
        ],
        error: null,
      },
    });
    const r = await postEnChunks('I progress');
    expect(r.chunks).toHaveLength(2);
    // Enum server viết HOA, module `chunk.ts` dùng chữ thường ⇒ map ở biên.
    expect(r.chunks[0].kind).toBe('function');
    expect(r.chunks[1].kind).toBe('content');
  });

  it('test_post_thieu_and_history_roundtrip_shape', async () => {
    const session = {
      id: '1',
      session: '2026-09-23',
      scores: [{ axis: 'A', value: 3 }],
      average: 3,
      note: '',
      createdAt: 'x',
    };
    const fetchMock = mockData({ appendThieu: { ok: true, session, error: null } });
    const saved = await postThieu('2026-09-23', { A: 3 });
    expect(saved.id).toBe('1');
    // Mảng `[{axis,value}]` ở schema ⇒ input phải gửi đúng dạng đó, không phải
    // object (server trả 400 nếu thiếu trục).
    expect(bodyOf(fetchMock).variables).toEqual({
      input: { session: '2026-09-23', scores: [{ axis: 'A', value: 3 }], note: '' },
    });

    const historyFetch = mockData({ thieuSessions: [session] });
    const list = await fetchThieuHistory();
    expect(list[0].session).toBe('2026-09-23');
    // Mảng server → object scores cho màn (mã trục A..H là duy nhất).
    expect(list[0].scores).toEqual({ A: 3 });
    expect(operationNameOf(historyFetch)).toBe('ThieuSessions');
  });
});
