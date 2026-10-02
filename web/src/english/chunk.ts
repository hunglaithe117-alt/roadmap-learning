export interface Chunk {
  text: string;
  kind: 'content' | 'function';
}

const FUNCTION_WORDS = new Set([
  'a', 'an', 'the',
  'i', 'you', 'he', 'she', 'it', 'we', 'they', 'me', 'him', 'her',
  'us', 'them', 'my', 'your', 'his', 'our', 'their', 'this', 'that',
  'these', 'those',
  'is', 'am', 'are', 'was', 'were', 'be', 'been', 'being', 'do', 'does',
  'did', 'have', 'has', 'had', 'will', 'would', 'can', 'could', 'shall',
  'should', 'may', 'might', 'must',
  'to', 'of', 'in', 'on', 'at', 'for', 'with', 'by', 'from', 'as',
  'and', 'or', 'but', 'so', 'if', 'because', 'when', 'while', 'there',
  'not', 'no', 'up', 'out', 'about',
]);

/** Split a sentence into content/function chunks (client helper cho highlight;
 * server POST /api/en/chunks là source-of-truth khi chấm nhịp câu). */
export function splitChunks(sentence: string): Chunk[] {
  return sentence
    .split(/\s+/)
    .filter(Boolean)
    .map((text) => {
      const key = text.toLowerCase().replace(/^[.,!?;:"'()]+|[.,!?;:"'()]+$/g, '');
      return { text, kind: (FUNCTION_WORDS.has(key) ? 'function' : 'content') as Chunk['kind'] };
    });
}
