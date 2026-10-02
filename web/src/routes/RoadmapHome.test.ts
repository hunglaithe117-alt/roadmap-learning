// `/roadmap` — danh sách path + vòng tiến độ.
//
// 2 chi tiết đáng test: `isBuiltin` quyết định có nút xoá hay không (server từ
// chối xoá path seed, nên nút xoá là nút chết), và vòng tiến độ lấy đúng phần
// trăm từ `summary.percent` chứ không tự chia lại.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import { createMemoryHistory, createRouter } from 'vue-router';
import { resetStaleMarks, resetUrqlClient } from '../graphql/client';
import { queryClient } from '../rest/queryClient';
import { appRoutes } from '../router';
import { mountScreen } from '../test/harness';
import RoadmapHome from './RoadmapHome.vue';

const path = (slug: string, isBuiltin: boolean) => ({
  __typename: 'Path',
  id: slug,
  guid: `g${slug}`,
  slug,
  title: slug === 'trung' ? 'Tiếng Trung từ A1' : 'Đường tự tạo',
  overview: slug === 'trung' ? 'Từ chào hỏi tới kể chuyện.' : '',
  language: 'zh',
  isBuiltin,
  createdAt: 'x',
  updatedAt: 'x',
});

const summary = { stages: 5, topicsTotal: 26, topicsRequired: 24, topicsDone: 12, topicsInProgress: 2, percent: 50 };

function stub() {
  const fetchMock = vi.fn().mockImplementation(async () =>
    new Response(
      JSON.stringify({
        data: {
          paths: [
            { path: path('trung', true), summary },
            { path: path('toan', false), summary: { ...summary, percent: 0, topicsDone: 0 } },
          ],
        },
      }),
      { status: 200, headers: { 'content-type': 'application/json' } },
    ),
  );
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function buttonsOf(w: Awaited<ReturnType<typeof mountScreen>>, slug: string): string[] {
  return w.get(`[data-testid="path-row-${slug}"]`)
    .findAll('button')
    .map((b) => b.text());
}

beforeEach(() => {
  vi.unstubAllGlobals();
  resetUrqlClient();
  resetStaleMarks();
  queryClient.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('test_roadmap_home', () => {
  it('test_lists_every_path_with_its_progress', async () => {
    stub();
    const w = await mountScreen(RoadmapHome, { route: '/roadmap' });
    await flushPromises();

    expect(w.find('[data-testid="path-row-trung"]').exists()).toBe(true);
    expect(w.get('[data-testid="path-row-trung"]').text()).toContain('12/24 màn');
  });

  it('test_progress_ring_reports_the_summary_percent', async () => {
    stub();
    const w = await mountScreen(RoadmapHome, { route: '/roadmap' });
    await flushPromises();

    const ring = w.get('[data-testid="path-row-trung"] svg');
    expect(ring.attributes('aria-label')).toBe('tiến độ 50%');
  });

  it('test_builtin_path_has_no_delete_button', async () => {
    // `isBuiltin` = seed tích hợp, server từ chối xoá. Nút xoá ở đây là nút
    // chết: bấm xong thấy lỗi tiếng Việt, không xoá được gì.
    stub();
    const w = await mountScreen(RoadmapHome, { route: '/roadmap' });
    await flushPromises();
    expect(buttonsOf(w, 'trung')).not.toContain('Xoá');
  });

  it('test_user_created_path_can_be_deleted', async () => {
    stub();
    const w = await mountScreen(RoadmapHome, { route: '/roadmap' });
    await flushPromises();
    expect(buttonsOf(w, 'toan')).toContain('Xoá');
  });

  it('test_delete_asks_before_calling_the_mutation', async () => {
    const fetchMock = stub();
    const w = await mountScreen(RoadmapHome, { route: '/roadmap' });
    await flushPromises();

    await w.get('[data-testid="path-row-toan"]').findAll('button').find((b) => b.text() === 'Xoá')!.trigger('click');
    await flushPromises();
    expect(fetchMock.mock.calls.some((c) => c[1]?.body && (c[1].body as string).includes('DeletePath'))).toBe(false);

    await w.get('[data-testid="path-row-toan"]').findAll('button').find((b) => b.text() === 'Xoá thật')!.trigger('click');
    for (let i = 0; i < 8; i += 1) await flushPromises();
    expect(fetchMock.mock.calls.some((c) => c[1]?.body && (c[1].body as string).includes('DeletePath'))).toBe(true);
  });

  it('test_path_title_links_to_the_map', async () => {
    stub();
    // Mount với router THẬT: harness stub `RouterLink` thành `<a>` rỗng, mà
    // `href` mới là thứ người dùng bấm. Toạ độ URL là hợp đồng thật sự.
    const router = createRouter({ history: createMemoryHistory(), routes: appRoutes() });
    await router.push('/roadmap');
    await router.isReady();
    const w = mount(RoadmapHome, { global: { plugins: [router] } });
    for (let i = 0; i < 6; i += 1) await flushPromises();

    expect(w.get('[data-testid="path-row-trung"]').find('a').attributes('href')).toBe('/roadmap/trung');
  });

  it('test_empty_list_says_so_instead_of_a_blank_page', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation(async () =>
        new Response(JSON.stringify({ data: { paths: [] } }), {
          status: 200,
          headers: { 'content-type': 'application/json' },
        }),
      ),
    );
    const w = await mountScreen(RoadmapHome, { route: '/roadmap' });
    await flushPromises();
    expect(w.text()).toContain('Chưa có path nào');
  });
});
