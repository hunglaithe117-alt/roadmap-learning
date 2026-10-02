export interface ThieuAxis {
  code: string;
  name: string;
  desc: string;
}

export interface ThieuSession {
  /** `ID` của GraphQL là chuỗi (id DB là `bigint`) — đổi sang `number` là mất chính xác. */
  id: string;
  session: string;
  scores: Record<string, number>;
  average: number;
  note: string;
  created_at: string;
}

export const THIEU_CODES = ['A', 'B', 'C', 'D', 'E', 'F', 'G', 'H'];

/** Mean of the 8 axis scores (missing axes are ignored). */
export function thieuAverage(scores: Record<string, number>): number {
  const vals = THIEU_CODES.map((c) => scores[c]).filter((v) => typeof v === 'number');
  if (vals.length === 0) return 0;
  return vals.reduce((a, b) => a + b, 0) / vals.length;
}

/** Per-axis series for the trend chart: {code -> scores oldest-first}. */
export function thieuTrend(sessions: ThieuSession[]): Record<string, number[]> {
  const ordered = [...sessions].reverse();
  const out: Record<string, number[]> = {};
  for (const c of THIEU_CODES) out[c] = ordered.map((s) => s.scores[c] ?? 0);
  return out;
}
