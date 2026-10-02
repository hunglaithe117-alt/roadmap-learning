// Port `sync.test.ts` (4) + `release.test.ts` (4) sang client `practice`/`sync`
// của app-v2.
//
// Điểm port đáng ghi: app-v2 **không nhận file peer qua HTTP** — mutation
// `sync` không có tham số (M4 §8: "nối dây, chưa có peer"). 2 test cũ
// `uploadSync(blob)` vì vậy chuyển thành `runSync()`: vẫn kiểm "merge trả về
// số dòng đã ghi" và "lỗi server nổi lên", chỉ khác là không còn form multipart.
import { afterEach, describe, expect, it, vi } from 'vitest';
import { resetStaleMarks, resetUrqlClient } from '../graphql/client';
import { bodyOf, mockData, operationNameOf } from '../test/graphqlMock';
import {
  fetchSuggestErrors,
  fetchSyncConflicts,
  fetchSyncStatus,
  isSyncUiEnabled,
  runSync,
  setSyncUiEnabled,
} from './api';

afterEach(() => {
  vi.unstubAllGlobals();
  resetUrqlClient();
  resetStaleMarks();
});


const MERGED = {
  decks: 1,
  cards: 2,
  reviews: 3,
  notes: 0,
  roadmapPaths: 0,
  roadmapStages: 0,
  roadmapMilestones: 0,
  roadmapTopics: 0,
  roadmapResources: 0,
  roadmapBookmarks: 0,
};

describe('test_sync_mutation', () => {
  it('test_sync_returns_merged_counts', async () => {
    const fetchMock = mockData({
      sync: {
        ok: true,
        merged: MERGED,
        conflicts: [],
        warnings: [],
        lastSyncAt: '2026-09-24T00:00:00Z',
        error: null,
      },
    });
    const r = await runSync();
    expect(r.merged.cards).toBe(2);
    expect(r.lastSyncAt).toBe('2026-09-24T00:00:00Z');
    const body = bodyOf(fetchMock);
    expect(operationNameOf(fetchMock)).toBe('Sync');
    // Không tham số — app-v2 chưa nối peer nên không nhận file.
    expect(body.variables).toEqual({});
  });

  it('test_sync_surfaces_vietnamese_server_error', async () => {
    mockData({
      sync: {
        ok: false,
        merged: MERGED,
        conflicts: [],
        warnings: [],
        lastSyncAt: '',
        error: { message: 'chưa cấu hình peer', code: 'INTERNAL' },
      },
    });
    await expect(runSync()).rejects.toThrow('chưa cấu hình peer');
  });
});

describe('test_sync_conflicts_read_only', () => {
  it('test_fetch_sync_conflicts_lists_rows', async () => {
    const fetchMock = mockData({
      syncConflicts: {
        ok: true,
        conflicts: [
          {
            guid: 'abc',
            table: 'cards',
            field: 'front',
            local: 'a',
            incoming: 'b',
            winner: 'incoming',
            detail: 'both-changed',
            resolvedAt: '2026-09-24T00:00:00Z',
          },
        ],
        error: null,
      },
    });
    const rows = await fetchSyncConflicts();
    expect(rows).toHaveLength(1);
    expect(rows[0].table).toBe('cards');
    expect(operationNameOf(fetchMock)).toBe('SyncConflicts');
  });

  it('test_fetch_sync_status_carries_last_sync_at', async () => {
    mockData({
      syncStatus: {
        ok: true,
        status: {
          enabled: true,
          strategy: 'last-write-win',
          lastSyncAt: '2026-09-24T00:00:00Z',
          conflictCount: 2,
        },
        error: null,
      },
    });
    const s = await fetchSyncStatus();
    expect(s.enabled).toBe(true);
    expect(s.lastSyncAt).toBe('2026-09-24T00:00:00Z');
    expect(s.conflictCount).toBe(2);
  });
});

describe('test_release_suggest_and_sync_toggle', () => {
  it('test_fetch_suggest_errors_uses_error_suggestions_query', async () => {
    const sample = [{ cardId: '1', front: 'a', back: 'b', errors: 2 }];
    const fetchMock = mockData({ errorSuggestions: sample });
    const r = await fetchSuggestErrors(5);
    expect(r).toEqual(sample);
    expect(operationNameOf(fetchMock)).toBe('ErrorSuggestions');
    expect(bodyOf(fetchMock).variables).toEqual({ limit: 5 });
  });

  it('test_fetch_sync_status_off_by_default', async () => {
    mockData({
      syncStatus: {
        ok: true,
        status: { enabled: false, strategy: 'last-write-win', lastSyncAt: '', conflictCount: 0 },
        error: null,
      },
    });
    const s = await fetchSyncStatus();
    expect(s.enabled).toBe(false);
    expect(s.strategy).toBe('last-write-win');
  });

  it('test_sync_ui_toggle_roundtrip', () => {
    const store = new Map<string, string>();
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => (store.has(k) ? store.get(k)! : null),
      setItem: (k: string, v: string) => void store.set(k, v),
      removeItem: (k: string) => void store.delete(k),
    });
    expect(isSyncUiEnabled()).toBe(false);
    setSyncUiEnabled(true);
    expect(isSyncUiEnabled()).toBe(true);
    setSyncUiEnabled(false);
    expect(isSyncUiEnabled()).toBe(false);
    vi.unstubAllGlobals();
  });

  it('test_sync_ui_off_when_storage_missing', () => {
    vi.stubGlobal('localStorage', {
      getItem: () => {
        throw new Error('storage unavailable');
      },
      setItem: () => {},
      removeItem: () => {},
    });
    expect(isSyncUiEnabled()).toBe(false);
    vi.unstubAllGlobals();
  });
});
