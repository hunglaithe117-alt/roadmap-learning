// M5 — client `practice` + `insight` + `sync` (player, sổ lỗi, reader, dashboard,
// cài đặt). PORT từ `player/api.ts` của app v1.
//
// Ba điểm khác biệt so với v1, đều do app-v2 bỏ endpoint JSON:
//   - `uploadSync(file)` **KHÔNG CÒN**: app-v2 không nhận file peer qua HTTP
//     (`syncinfra.NewSchemaLoader(nil)` chưa nối peer — M4 §8 gọi đây là
//     "nối dây, chưa có peer"). Thay bằng mutation `sync` không tham số. Cài
//     đặt vẫn giữ nút, gọi để hiện lỗi tiếng Việt nguyên văn thay vì giấu.
//   - `fetchReaderLevels()` không có query riêng: app-v2 suy ra danh sách level
//     từ chính `readerArticles` (xem doc của `readerArticles`).
//   - Tên field theo camelCase của schema (`lastSyncAt`, `conflictCount`).
import { runMutation, runQuery } from '../graphql/client';
import { assertPayloadOk, errorMessage } from '../graphql/errors';
import {
  AppendErrorMutation,
  ErrorSuggestions,
  Errors,
  ReaderArticles,
  RecordShadowProgressMutation,
  ShadowProgressQuery,
  StatsQuery,
  SyncConflicts,
  SyncMutation,
  SyncStatusQuery,
  TopErrors,
  type CardErrorCount,
  type ErrorEntry,
  type MergeResult,
  type ReaderArticle,
  type ShadowProgress,
  type Stats,
  type SyncConflict,
  type SyncStatus,
  type TopErrorCount,
} from '../graphql/operations';
import { afterMutation } from '../lib/afterMutation';
import type { DiffToken } from './diff';

export type { CardErrorCount as SuggestCard, ErrorEntry, ReaderArticle, Stats, SyncConflict, SyncStatus, ShadowProgress, TopErrorCount as TopError };

export async function fetchProgress(cardId: string): Promise<ShadowProgress> {
  return (await runQuery(ShadowProgressQuery, { cardId })).shadowProgress;
}

export async function postProgress(cardId: string, loops: number, rate: number): Promise<ShadowProgress> {
  const data = await runMutation(RecordShadowProgressMutation, { input: { cardId, loops, rate } });
  assertPayloadOk(data.recordShadowProgress);
  if (!data.recordShadowProgress.progress) throw new Error('lỗi hệ thống');
  await afterMutation();
  return data.recordShadowProgress.progress;
}

export interface NewError {
  cardId?: string;
  expected: string;
  transcript: string;
  wrong: string[];
}

export async function postError(input: NewError): Promise<ErrorEntry> {
  const data = await runMutation(AppendErrorMutation, {
    input: {
      cardId: input.cardId ?? null,
      expected: input.expected,
      transcript: input.transcript,
      wrong: input.wrong,
    },
  });
  assertPayloadOk(data.appendError);
  if (!data.appendError.entry) throw new Error('lỗi hệ thống');
  await afterMutation();
  return data.appendError.entry;
}

export async function fetchErrors(cardId?: string, limit = 50): Promise<ErrorEntry[]> {
  return (await runQuery(Errors, { cardId: cardId ?? null, limit })).errors;
}

export async function fetchTopErrors(limit = 10): Promise<TopErrorCount[]> {
  return (await runQuery(TopErrors, { limit })).topErrors;
}

export async function fetchSuggestErrors(limit = 10): Promise<CardErrorCount[]> {
  return (await runQuery(ErrorSuggestions, { limit })).errorSuggestions;
}

export async function fetchReader(level?: string, id?: string): Promise<ReaderArticle[]> {
  const { readerArticles } = await runQuery(ReaderArticles, { level: level ?? null, id: id ?? null });
  return readerArticles;
}

/** Level suy ra từ chính danh sách bài (app-v2 không có query `/api/reader/levels`). */
export async function fetchReaderLevels(): Promise<string[]> {
  const articles = await fetchReader();
  return [...new Set(articles.map((a) => a.level))].sort();
}

export async function fetchStats(range: 'week' | 'month' = 'week'): Promise<Stats> {
  const data = await runQuery(StatsQuery, { range });
  assertPayloadOk(data.stats);
  if (!data.stats.stats) throw new Error('lỗi hệ thống');
  return data.stats.stats;
}

// ── sync ─────────────────────────────────────────────────────────────────────

/** Cờ UI thử nghiệm — còn giữ vì người dùng bật/tắt ở Cài đặt (hành vi v1). */
export function isSyncUiEnabled(): boolean {
  try {
    return localStorage.getItem('syncEnabled') === '1';
  } catch {
    return false;
  }
}

export function setSyncUiEnabled(on: boolean): void {
  try {
    if (on) localStorage.setItem('syncEnabled', '1');
    else localStorage.removeItem('syncEnabled');
  } catch {
    /* storage unavailable */
  }
}

export async function fetchSyncStatus(): Promise<SyncStatus> {
  const data = await runQuery(SyncStatusQuery);
  assertPayloadOk(data.syncStatus);
  if (!data.syncStatus.status) throw new Error('lỗi hệ thống');
  return data.syncStatus.status;
}

export async function fetchSyncConflicts(limit = 50): Promise<SyncConflict[]> {
  const data = await runQuery(SyncConflicts, { limit });
  assertPayloadOk(data.syncConflicts);
  return data.syncConflicts.conflicts;
}

/**
 * Chạy merge peer. App-v2 chưa nối peer ⇒ `sync` trả lỗi nghiệp vụ/lỗi hệ
 * thống; `assertPayloadOk` biến nó thành `AppError` mang message tiếng Việt để Cài
 * đặt hiện nguyên văn thay vì báo "đã đồng bộ".
 */
export async function runSync(): Promise<MergeResult> {
  const data = await runMutation(SyncMutation);
  assertPayloadOk(data.sync);
  await afterMutation();
  return data.sync;
}

/** Message để đưa lên UI, bất kể lỗi là loại gì (tái dùng từ tầng GraphQL). */
export { errorMessage };

export type { DiffToken };
