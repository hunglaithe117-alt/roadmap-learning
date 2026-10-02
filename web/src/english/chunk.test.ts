import { describe, it, expect } from 'vitest';
import { splitChunks } from './chunk';

describe('test_split_chunks', () => {
  it('test_split_chunks_content_function_expected_kinds', () => {
    const kinds = Object.fromEntries(splitChunks('I want to make progress').map((c) => [c.text.toLowerCase(), c.kind]));
    expect(kinds['i']).toBe('function');
    expect(kinds['to']).toBe('function');
    expect(kinds['want']).toBe('content');
    expect(kinds['make']).toBe('content');
    expect(kinds['progress']).toBe('content');
  });

  it('test_split_chunks_empty_sentence_returns_empty', () => {
    expect(splitChunks('   ')).toEqual([]);
  });

  it('test_split_chunks_punctuation_still_function', () => {
    const kinds = Object.fromEntries(splitChunks('Progress, and joy.').map((c) => [c.text, c.kind]));
    expect(kinds['and']).toBe('function');
    expect(kinds['Progress,']).toBe('content');
    expect(kinds['joy.']).toBe('content');
  });
});
