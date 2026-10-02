export interface StressResult {
  term: string;
  ipa: string;
  stress: string;
  exception: boolean;
  note: string;
  source: 'dict' | 'rule';
}

/** Split a CAPS-marked pattern like "PHO-to-graph" into syllable parts. */
export function splitStressMarks(stress: string): Array<{ text: string; stressed: boolean }> {
  if (!stress) return [];
  return stress.split('-').map((s) => ({
    text: s,
    stressed: s.length > 0 && s === s.toUpperCase() && /[A-Z]/.test(s),
  }));
}
