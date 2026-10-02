import { describe, it, expect } from 'vitest';
import { splitChunks } from '../english/chunk';
import { splitStressMarks } from '../english/stress';

/** Route-adjacent test for EnStress: render helpers used by StressCard/ChunkRow. */
describe('test_enstress_route_helpers', () => {
  it('test_enstress_chunkrow_content_bold_function_dim', () => {
    const chunks = splitChunks('I want to make progress');
    const content = chunks.filter((c) => c.kind === 'content').map((c) => c.text);
    const fn = chunks.filter((c) => c.kind === 'function').map((c) => c.text);
    expect(content).toEqual(['want', 'make', 'progress']);
    expect(fn).toEqual(['I', 'to']);
  });

  it('test_enstress_stresscard_exception_label_source', () => {
    const parts = splitStressMarks('PRE-sent');
    expect(parts[0].stressed).toBe(true);
    expect(parts[1].stressed).toBe(false);
  });
});
