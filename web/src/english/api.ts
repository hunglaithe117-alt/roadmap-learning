// M5 — client `content` phía Anh: tra từ điển, trọng âm, chunk, seed, THIEU.
//
// PORT từ `english/api.ts` của app v1; app-v2 đã gộp 5 endpoint `/api/en/*`
// vào GraphQL. Khác biệt đáng chú ý:
//   - `ThieuSession.scores` là MẢNG `[{axis, value}]` ở schema (không phải
//     object), nên 2 hàm bên dưới chuyển qua lại để màn vẫn dùng
//     `Record<string, number>` như v1 (mã trục A..H là duy nhất).
//   - `Chunk.kind` là enum `CONTENT|FUNCTION` ở schema, `content|function` ở
//     `english/chunk.ts` (lowercase) — map ở biên.
import { runMutation, runQuery } from '../graphql/client';
import { assertPayloadOk } from '../graphql/errors';
import {
  AppendThieuMutation,
  ChunkSentence,
  EnglishSearch,
  SeedEnglishMutation,
  Stress,
  ThieuAxes,
  ThieuSessions,
  type Chunk as ChunkRowT,
  type EnglishEntry,
  type EnglishSeedResult,
  type StressLookup,
  type ThieuAxis,
  type ThieuScore,
  type ThieuSession as ThieuSessionT,
} from '../graphql/operations';
import { afterMutation } from '../lib/afterMutation';
import type { Chunk } from './chunk';
import type { StressResult } from './stress';
import type { ThieuSession } from './thieu';

export type { EnglishEntry as EnEntry } from '../graphql/operations';

export async function fetchEnSearch(q: string, limit = 10): Promise<EnglishEntry[]> {
  return (await runQuery(EnglishSearch, { q, limit })).englishSearch;
}

export async function fetchEnStress(word: string): Promise<StressResult> {
  const lookup: StressLookup = (await runQuery(Stress, { word })).stress;
  return {
    term: lookup.term,
    ipa: lookup.ipa,
    stress: lookup.stress,
    exception: lookup.exception,
    note: lookup.note,
    source: lookup.fromDict ? 'dict' : 'rule',
  };
}

export async function postEnChunks(sentence: string): Promise<{ chunks: Chunk[] }> {
  const data = await runQuery(ChunkSentence, { sentence });
  assertPayloadOk(data.chunk);
  const chunks: ChunkRowT[] = data.chunk.chunks;
  return {
    chunks: chunks.map((c) => ({
      text: c.text,
      kind: c.kind === 'FUNCTION' ? 'function' : 'content',
    })),
  };
}

export async function postEnSeed(): Promise<EnglishSeedResult> {
  const data = await runMutation(SeedEnglishMutation);
  assertPayloadOk(data.seedEnglish);
  if (!data.seedEnglish.result) throw new Error('lỗi hệ thống');
  await afterMutation();
  return data.seedEnglish.result;
}

export async function fetchThieuAxes(): Promise<ThieuAxis[]> {
  return (await runQuery(ThieuAxes)).thieuAxes;
}

function toSession(s: ThieuSessionT): ThieuSession {
  const scores: Record<string, number> = {};
  for (const sc of s.scores) scores[sc.axis] = sc.value;
  return {
    id: s.id,
    session: s.session,
    scores,
    average: s.average,
    note: s.note,
    created_at: s.createdAt,
  };
}

export async function postThieu(
  session: string,
  scores: Record<string, number>,
  note = '',
): Promise<ThieuSession> {
  const input: ThieuScore[] = Object.entries(scores).map(([axis, value]) => ({ axis, value }));
  const data = await runMutation(AppendThieuMutation, { input: { session, scores: input, note } });
  assertPayloadOk(data.appendThieu);
  if (!data.appendThieu.session) throw new Error('lỗi hệ thống');
  await afterMutation();
  return toSession(data.appendThieu.session);
}

export async function fetchThieuHistory(): Promise<ThieuSession[]> {
  const { thieuSessions } = await runQuery(ThieuSessions);
  return thieuSessions.map(toSession);
}
