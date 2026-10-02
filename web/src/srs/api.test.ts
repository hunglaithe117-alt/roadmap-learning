// Port `decks.test.ts` (5) + phần tra từ điển của `api.test.ts` (1) sang client
// `srs` của app-v2. Khác bản v1: client nói chuyện GraphQL `/query` nên test
// kiểm **operation + variables trong body POST** thay vì kiểm URL — đó mới là
// thứ quyết định server trả đúng dữ liệu.
import { afterEach, describe, expect, it, vi } from 'vitest';
import { resetStaleMarks, resetUrqlClient } from '../graphql/client';
import { AppError } from '../graphql/errors';
import { bodyOf, mockData, operationNameOf } from '../test/graphqlMock';
import { createDeck, dictSearch, fetchDecks, fetchDueCards, postReview } from './api';

afterEach(() => {
  vi.unstubAllGlobals();
  resetUrqlClient();
  resetStaleMarks();
});

describe('test_deck_api_client', () => {
  it('test_fetch_decks_ok_returns_list', async () => {
    const sample = [{ id: '1', guid: 'g1', name: 'HSK1', lang: 'zh', createdAt: 'x' }];
    mockData({ decks: sample });
    await expect(fetchDecks()).resolves.toEqual(sample);
  });

  it('test_create_deck_sends_name_and_lang', async () => {
    const deck = { id: '2', guid: 'g2', name: 'PVO', lang: 'en', createdAt: 'x' };
    const fetchMock = mockData({ createDeck: { ok: true, deck, error: null } });
    const d = await createDeck('PVO', 'en');
    expect(d).toEqual(deck);
    const body = bodyOf(fetchMock);
    expect(operationNameOf(fetchMock)).toBe('CreateDeck');
    expect(body.variables).toEqual({ name: 'PVO', lang: 'en' });
  });

  it('test_fetch_due_cards_uses_due_cards_query_with_deck_id', async () => {
    const sample = [{ id: '7', deckId: '1', front: '你好', back: 'xin chào' }];
    const fetchMock = mockData({ dueCards: sample });
    await expect(fetchDueCards('1')).resolves.toEqual(sample);
    expect(operationNameOf(fetchMock)).toBe('DueCards');
    expect(bodyOf(fetchMock).variables).toEqual({ deckId: '1' });
  });

  it('test_post_review_returns_schedule', async () => {
    const review = {
      cardId: '7',
      dueAt: 'x',
      intervalDays: 3,
      stability: 1,
      difficulty: 4,
      reps: 2,
      fallback: true,
    };
    mockData({ recordReview: { ok: true, review, error: null } });
    const r = await postReview('7', 3);
    expect(r.intervalDays).toBe(3);
    expect(r.fallback).toBe(true);
  });

  it('test_api_error_throws_vietnamese_message', async () => {
    // `ok: false` + `error` ⇒ `payloadError` ném `AppError` mang message đó.
    mockData({
      recordReview: {
        ok: false,
        review: null,
        error: { message: 'grade phải từ 1 đến 4 (1=Quên, 4=Dễ)', code: 'BAD_REQUEST' },
      },
    });
    // Gotcha #2 của gate M4: message tiếng Việt của server hiện NGUYÊN VĂN.
    // Test sửa `payloadError` thành trả message mặc định ⇒ đỏ.
    await expect(postReview('1', 9)).rejects.toThrow('grade phải từ 1 đến 4 (1=Quên, 4=Dễ)');
  });
});

describe('test_dict_search', () => {
  it('test_search_ni_returns_entries', async () => {
    const sample = [{ hanzi: '你好', pinyin: 'ni hao', nghia: 'xin chào' }];
    const fetchMock = mockData({ dictSearch: sample });
    await expect(dictSearch('ni')).resolves.toEqual(sample);
    expect(bodyOf(fetchMock).variables).toEqual({ q: 'ni', limit: 10 });
  });

  it('test_payload_error_is_app_error_with_server_code', () => {
    // `AppError` là thứ UI đọc `message` + `code`; nếu đổi sang Error thường thì
    // `code` biến mất và các màn lọc theo mã lỗi hỏng.
    const e = new AppError('boom', 'CONFLICT');
    expect(e).toBeInstanceOf(Error);
    expect(e.code).toBe('CONFLICT');
  });
});
