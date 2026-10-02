// T2.1 — pinyin tone engine (client mirror của api/chinese.go).
// Không dùng pinyin-pro để giữ build offline xanh (cùng lý do như ts-fsrs
// ở T1.2): bảng dấu chuẩn phổ thông đủ cho HSK1-4.

const MARKS: Record<string, string[]> = {
  a: ['a', 'ā', 'á', 'ǎ', 'à'],
  e: ['e', 'ē', 'é', 'ě', 'è'],
  i: ['i', 'ī', 'í', 'ǐ', 'ì'],
  o: ['o', 'ō', 'ó', 'ǒ', 'ò'],
  u: ['u', 'ū', 'ú', 'ǔ', 'ù'],
  'ü': ['ü', 'ǖ', 'ǘ', 'ǚ', 'ǜ'],
};

function normalizeSyllable(s: string): string {
  return s.toLowerCase().trim().replace(/u:/g, 'ü').replace(/v/g, 'ü');
}

function isVowel(ch: string): boolean {
  return ch === 'a' || ch === 'e' || ch === 'i' || ch === 'o' || ch === 'u' || ch === 'ü';
}

export function parseSyllable(syl: string): { base: string; tone: number } {
  const s = normalizeSyllable(syl);
  if (s === '') return { base: '', tone: 5 };
  const last = s[s.length - 1];
  if (last >= '1' && last <= '5') {
    return { base: s.slice(0, -1), tone: Number(last) };
  }
  return { base: s, tone: 5 };
}

export function markSyllable(numbered: string): string {
  const { base, tone } = parseSyllable(numbered);
  if (!base || tone < 1 || tone > 4) return base;
  const rs = [...base];
  let target = -1;
  const ai = rs.indexOf('a');
  if (ai >= 0) {
    target = ai;
  } else {
    const ei = rs.indexOf('e');
    if (ei >= 0) {
      target = ei;
    } else {
      const ou = rs.join('').indexOf('ou');
      if (ou >= 0) {
        target = ou;
      } else {
        rs.forEach((r, i) => {
          if (isVowel(r)) target = i;
        });
      }
    }
  }
  if (target < 0) return base;
  const marks = MARKS[rs[target]];
  if (marks) rs[target] = marks[tone];
  return rs.join('');
}

/** "ni3 hao3" -> "nǐ hǎo". */
export function toMarks(numbered: string): string {
  return numbered.split(/\s+/).filter(Boolean).map(markSyllable).join(' ');
}

/** Tách dãy thanh điệu người dùng nhập ("3 3", "1-4", "hao3") -> [3,3]. */
export function parseToneInput(s: string): number[] | null {
  const toks = s.split(/[\s\-,，]+/).map((t) => t.trim()).filter(Boolean);
  if (toks.length === 0) return null;
  const out: number[] = [];
  for (const raw of toks) {
    const t = normalizeSyllable(raw);
    const last = t[t.length - 1];
    if (last < '1' || last > '5') return null;
    out.push(Number(last));
  }
  return out;
}

export function pairLabel(tones: number[]): string {
  return tones.join('-');
}

export interface PairGrade {
  grade: number;
  score: number;
  exact: boolean;
}

/** Mirror của GradeTonePair (Go): exact -> 4, >=1/2 -> 3, trúng ít -> 2, trượt hết -> 1. */
export function gradePair(expected: number[], answered: number[]): PairGrade {
  if (expected.length === 0 || expected.length !== answered.length) {
    throw new Error('số âm tiết không khớp');
  }
  let hit = 0;
  expected.forEach((t, i) => {
    if (t === answered[i]) hit += 1;
  });
  const score = hit / expected.length;
  const exact = hit === expected.length;
  let grade = 1;
  if (exact) grade = 4;
  else if (score >= 0.5) grade = 3;
  else if (hit > 0) grade = 2;
  return { grade, score, exact };
}
