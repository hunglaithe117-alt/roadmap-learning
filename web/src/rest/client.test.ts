// Port `audio.test.ts` (5) + phần `/api/health` của `api.test.ts` (1) sang
// client REST của app-v2 (`rest/client.ts`).
import { describe, it, expect, vi } from 'vitest';
import { buildTTSUrl, fetchHealth, fetchTTSEngine, uploadSTT } from './client';

describe('test_audio_tts_and_stt_helpers', () => {
  it('test_build_tts_url_encodes_text', () => {
    expect(buildTTSUrl('ni hao', '')).toBe('/api/tts?text=ni%20hao');
  });

  it('test_upload_stt_ok_returns_transcript', async () => {
    const sample = { transcript: 'ni hao', lang: 'zh', engine: 'stub' };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers(),
      json: async () => sample,
    });
    vi.stubGlobal('fetch', fetchMock);
    const r = await uploadSTT(new Blob(['fake-audio'], { type: 'audio/webm' }), '');
    expect(r).toEqual({ ...sample, engineHeader: 'stub' });
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/stt');
    expect(init.method).toBe('POST');
    expect(init.body).toBeInstanceOf(FormData);
    vi.unstubAllGlobals();
  });

  it('test_upload_stt_reads_x_engine_header', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'X-Engine': 'faster-whisper' }),
      json: async () => ({ transcript: 'hello', lang: 'en', engine: 'faster-whisper' }),
    });
    vi.stubGlobal('fetch', fetchMock);
    const r = await uploadSTT(new Blob(['x']), '');
    expect(r.engineHeader).toBe('faster-whisper');
    vi.unstubAllGlobals();
  });

  it('test_fetch_tts_engine_reads_header', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'X-Engine': 'piper' }),
      body: { cancel: async () => {} },
    });
    vi.stubGlobal('fetch', fetchMock);
    await expect(fetchTTSEngine('ni hao', '')).resolves.toBe('piper');
    expect(fetchMock).toHaveBeenCalledOnce();
    vi.unstubAllGlobals();
  });

  it('test_upload_stt_server_error_throws', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: false, status: 502, json: async () => null }),
    );
    await expect(uploadSTT(new Blob(['x']), '')).rejects.toThrow('stt 502');
    vi.unstubAllGlobals();
  });
});

describe('test_rest_health', () => {
  it('test_fetch_health_ok_returns_status', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => ({ status: 'ok' }) }),
    );
    const h = await fetchHealth('');
    expect(h.status).toBe('ok');
    vi.unstubAllGlobals();
  });
});
