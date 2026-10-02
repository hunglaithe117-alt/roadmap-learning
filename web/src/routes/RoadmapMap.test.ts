// `/roadmap/:slug` — chế độ bản đồ / danh sách, và việc confetti tắt khi máy bảo
// giảm chuyển động (ROADMAP-MAP-IDEA §6 — điều kiện bắt buộc, không phải tuỳ thích).
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import { createRouter, createMemoryHistory } from 'vue-router';
import { resetStaleMarks, resetUrqlClient } from '../graphql/client';
import { queryClient } from '../rest/queryClient';
import { appRoutes } from '../router';
import RoadmapMap from './RoadmapMap.vue';
import ConfettiCanvas from '../roadmap/map/ConfettiCanvas.vue';
import type { RoadmapPath } from '../graphql/operations';

function stage(id: string, slug: string, position: number, over: Partial<RoadmapPath['stages'][number]> = {}) {
  return {
    id,
    pathId: '1',
    slug,
    title: `Chặng ${position}`,
    goal: 'Nói được câu ngắn',
    position,
    durationWeeks: 4,
    status: 'IN_PROGRESS' as const,
    statusNote: '',
    completedAt: null,
    deckId: null,
    deck: null,
    // Cố ý `PLAIN` cho MỌI chặng: đúng tình trạng thật do lỗi `enum Terrain`
    // ở server (M6b §7), và là điều kiện để F4 còn lộ được sau khi F1 sửa —
    // fixture khác terrain thì test F4 xanh vì lý do vô nghĩa.
    terrain: 'MEADOW' as const,
    direction: 'UP' as const,
    topics: [],
    milestones: [],
    ...over,
  };
}

function topic(
  id: string,
  title: string,
  position: number,
  level: 'DONE' | 'CURRENT' | 'LOCKED',
  stageId = '10',
) {
  return {
    id,
    stageId,
    title,
    why: '',
    activityList: [],
    position,
    status: level === 'DONE' ? ('DONE' as const) : ('NOT_STARTED' as const),
    statusNote: '',
    completedAt: null,
    isOptional: false,
    mapX: null,
    mapY: null,
    level,
    point: { x: 500, y: 1800 - position * 200 },
    mapPinned: false,
    resources: [],
  };
}

const TREE: RoadmapPath = {
  id: '1',
  guid: 'p1',
  slug: 'trung',
  title: 'Tiếng Trung',
  overview: 'Từ A1 tới B1',
  language: 'zh',
  isBuiltin: true,
  createdAt: 'x',
  updatedAt: 'x',
  progress: {
    stages: 2, topicsTotal: 6, topicsRequired: 6, topicsOptional: 0, topicsDone: 2,
    topicsInProgress: 0, topicsLocked: 2, percent: 33, lastCompletedAt: null, completedInRange: 0,
  },
  // HAI chặng, cùng `terrain` (`PLAIN`). Fixture M6b cũ chỉ có 1 chặng ⇒
  // `firstStage.id === stageId` LUÔN đúng ⇒ F3 (giữ chặng sau mutation) và F4
  // (reset offset khi đổi chặng) đều không thể lộ dù đã hỏng.
  stages: [
    stage('10', 'zh-g0', 0, {
      topics: [topic('100', 'Chào', 0, 'DONE'), topic('101', 'Số đếm', 1, 'CURRENT'), topic('102', 'Ăn uống', 2, 'LOCKED')],
      milestones: [{ id: 'm1', stageId: '10', text: 'Nói 20 câu', position: 1, createdAt: 'x', updatedAt: 'x' }],
    }),
    stage('11', 'zh-g1', 1, {
      topics: [topic('200', 'Mua bán', 0, 'DONE', '11'), topic('201', 'Hỏi đường', 1, 'CURRENT', '11'), topic('202', 'Thời gian', 2, 'LOCKED', '11')],
      milestones: [{ id: 'm2', stageId: '11', text: 'Mua được đồ', position: 1, createdAt: 'x', updatedAt: 'x' }],
    }),
  ],
};

function stub(tree: RoadmapPath = TREE) {
  const fetchMock = vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const name =
      new URL(String(input), 'http://localhost').searchParams.get('operationName') ??
      (init?.body ? (JSON.parse(init.body as string) as { operationName: string }).operationName : '');
    // Mutation trả `ok: true` để `assertPayloadOk` không ném; cây đọc lại từ
    // `path` vẫn là `tree` (mutation không đổi fixture trong test).
    const data =
      name === 'SetTopicStatus'
        ? { setTopicStatus: { ok: true, topic: null, error: null } }
        : { path: tree };
    return new Response(JSON.stringify({ data }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

/** Tên operation mà server nhận — assert nó đúng là kiểm ĐÚNG việc xoá. */
function opNamesOf(fetchMock: ReturnType<typeof vi.fn>): string[] {
  return fetchMock.mock.calls.map((c) => {
    const [url, init] = c as [string, RequestInit | undefined];
    if (init?.body) return (JSON.parse(init.body as string) as { operationName: string }).operationName;
    return new URL(url, 'http://localhost').searchParams.get('operationName') ?? '';
  });
}

async function mountScreenAt(route: string) {
  const router = createRouter({ history: createMemoryHistory(), routes: appRoutes() });
  await router.push(route);
  await router.isReady();
  const w = mount(RoadmapMap, { global: { plugins: [router] } });
  for (let i = 0; i < 8; i += 1) await flushPromises();
  return w;
}

function setReduceMotion(reduce: boolean): void {
  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockImplementation((q: string) => ({
      matches: reduce && q.includes('prefers-reduced-motion'),
      media: q,
      addEventListener: () => {},
      removeEventListener: () => {},
    })),
  );
}

beforeEach(() => {
  localStorage.clear();
  vi.unstubAllGlobals();
  resetUrqlClient();
  resetStaleMarks();
  queryClient.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('test_map_view_is_the_default', () => {
  it('test_first_visit_shows_the_map_not_the_list', async () => {
    stub();
    const w = await mountScreenAt('/roadmap/trung');
    expect(w.find('[data-testid="map-scroll"]').exists()).toBe(true);
    expect(w.find('[data-testid="list-view"]').exists()).toBe(false);
  });

  it('test_map_draws_one_svg_node_per_topic', async () => {
    stub();
    const w = await mountScreenAt('/roadmap/trung');
    for (const id of ['100', '101', '102']) {
      expect(w.find(`[data-testid="node-${id}"]`).exists(), `thiếu node ${id}`).toBe(true);
    }
  });

  it('test_milestone_is_drawn_on_the_trail', async () => {
    stub();
    const w = await mountScreenAt('/roadmap/trung');
    expect(w.findAll('[data-testid="map-milestone"]')).toHaveLength(1);
  });
});

describe('test_list_view_fallback', () => {
  it('test_query_view_list_renders_the_list', async () => {
    stub();
    const w = await mountScreenAt('/roadmap/trung?view=list');
    expect(w.find('[data-testid="list-view"]').exists()).toBe(true);
    expect(w.find('[data-testid="map-scroll"]').exists()).toBe(false);
  });

  it('test_list_shows_every_topic_of_the_stage', async () => {
    stub();
    const w = await mountScreenAt('/roadmap/trung?view=list');
    for (const id of ['100', '101', '102']) {
      expect(w.find(`[data-testid="list-topic-${id}"]`).exists(), `thiếu dòng ${id}`).toBe(true);
    }
  });

  it('test_switching_back_to_map_works', async () => {
    stub();
    const w = await mountScreenAt('/roadmap/trung?view=list');
    await w.get('[data-testid="view-map"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();
    expect(w.find('[data-testid="map-scroll"]').exists()).toBe(true);
  });

  it('test_chosen_view_is_remembered_for_next_visit', async () => {
    stub();
    const w = await mountScreenAt('/roadmap/trung');
    await w.get('[data-testid="view-list"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();
    expect(localStorage.getItem('roadmap:view')).toBe('list');
  });
});

describe('test_confetti_respects_reduced_motion', () => {
  it('test_no_canvas_when_the_system_asks_for_reduced_motion', () => {
    setReduceMotion(true);
    const w = mount(ConfettiCanvas, { props: { trigger: 1 } });
    // Canvas KHÔNG được tạo ⇒ không có phần tử để vẽ, không có vòng lặp
    // requestAnimationFrame chạy nền.
    expect(w.find('[data-testid="confetti"]').exists()).toBe(false);
  });

  it('test_canvas_exists_when_motion_is_allowed', () => {
    setReduceMotion(false);
    const w = mount(ConfettiCanvas, { props: { trigger: 1 } });
    expect(w.find('[data-testid="confetti"]').exists()).toBe(true);
  });

  it('test_no_canvas_when_the_user_turned_confetti_off', () => {
    localStorage.setItem('roadmap:confetti', '0');
    setReduceMotion(false);
    const w = mount(ConfettiCanvas, { props: { trigger: 1 } });
    expect(w.find('[data-testid="confetti"]').exists()).toBe(false);
  });

  it('test_marking_a_level_done_does_not_break_with_motion_reduced', async () => {
    setReduceMotion(true);
    stub();
    const w = await mountScreenAt('/roadmap/trung');
    await w.get('[data-testid="node-hit-101"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();

    const done = w.find('[data-testid="mark-done"]');
    expect(done.exists()).toBe(true);
    // Chức năng đánh dấu Xong độc lập với hiệu ứng: tắt confetti không được
    // tắt luôn nút.
    await done.trigger('click');
    for (let i = 0; i < 8; i += 1) await flushPromises();
    expect(w.find('[data-testid="confetti"]').exists()).toBe(false);
  });
});

describe('test_panel_opens_from_the_map', () => {
  it('test_tapping_a_node_opens_the_level_panel', async () => {
    stub();
    const w = await mountScreenAt('/roadmap/trung');
    await w.get('[data-testid="node-hit-101"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();
    expect(w.find('[data-testid="level-panel"]').exists()).toBe(true);
  });

  it('test_locked_node_never_opens_a_panel', async () => {
    stub();
    const w = await mountScreenAt('/roadmap/trung');
    await w.get('[data-testid="node-hit-102"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();
    expect(w.find('[data-testid="level-panel"]').exists()).toBe(false);
  });
});

describe('test_review_button_hidden_without_deck', () => {
  it('test_no_review_link_when_stage_has_no_deck', async () => {
    stub();
    const w = await mountScreenAt('/roadmap/trung');
    await w.get('[data-testid="node-hit-101"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();
    expect(w.find('[data-testid="goto-review"]').exists()).toBe(false);
  });

  it('test_review_link_appears_once_the_stage_has_a_deck', async () => {
    const withDeck: RoadmapPath = {
      ...TREE,
      stages: [
        {
          ...TREE.stages[0],
          deckId: '4',
          deck: { id: '4', name: 'HSK1', lang: 'zh' },
        },
      ],
    };
    stub(withDeck);
    const w = await mountScreenAt('/roadmap/trung');
    await w.get('[data-testid="node-hit-101"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();
    expect(w.get('[data-testid="goto-review"]').attributes('href')).toBe('/review?deck=4');
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// F3 (M6 remediation) — `load()` KHÔNG được ném người dùng về chặng đầu.
// ─────────────────────────────────────────────────────────────────────────────

describe('test_stage_survives_a_mutation', () => {
  it('test_marking_done_on_stage_two_keeps_stage_two_open', async () => {
    // Kịch bản phổ biến: mở path lần đầu (`localStorage` trống) → bấm chip
    // "Chặng 3" → chạm node → "Đánh dấu Xong". `load()` chạy lại sau mutation;
    // nếu `stageId` tính lại từ `localStorage` thì nhảy về chặng 1 và
    // `watch(stageId)` đóng panel.
    stub();
    const w = await mountScreenAt('/roadmap/trung');
    expect(w.get('[data-testid="stage-chip-zh-g1"]').attributes('data-testid')).toBeTruthy();

    await w.get('[data-testid="stage-chip-zh-g1"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();
    expect(w.find('[data-testid="node-hit-201"]').exists()).toBe(true);

    await w.get('[data-testid="node-hit-201"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();
    expect(w.find('[data-testid="mark-done"]').exists()).toBe(true);

    await w.get('[data-testid="mark-done"]').trigger('click');
    for (let i = 0; i < 8; i += 1) await flushPromises();

    // Vẫn ở chặng 2: node của chặng 2 còn trên bản đồ, node chặng 1 biến mất.
    expect(w.find('[data-testid="node-201"]').exists(), 'rớt khỏi chặng 2').toBe(true);
    expect(w.find('[data-testid="node-100"]').exists(), 'quay về chặng 1').toBe(false);
  });

  it('test_stage_survives_deleting_a_milestone', async () => {
    // `removeMilestone` cũng gọi `load()` — đây là đường thứ 3 (khác `mark`
    // và khác `@done` của 4 form) mà `stageId` từng bị ném về chặng đầu.
    // Không dùng form ở đây vì `Dialog` của reka-ui không render trong
    // `happy-dom` (thiếu `ResizeObserver` + vị trí đo — cùng lý do M6b §9
    // chọn `<select>` gốc thay vì `Select` của reka-ui).
    const fetchMock = stub();
    const w = await mountScreenAt('/roadmap/trung?view=list');
    await w.get('[data-testid="stage-chip-zh-g1"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();
    await w.get('[data-testid="ask-delete-milestone-m2"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();
    await w.get('[data-testid="delete-milestone-m2"]').trigger('click');
    for (let i = 0; i < 8; i += 1) await flushPromises();

    expect(opNamesOf(fetchMock)).toContain('DeleteMilestone');
    expect(w.find('[data-testid="list-topic-201"]').exists(), 'rớt khỏi chặng 2').toBe(true);
    expect(w.find('[data-testid="list-topic-101"]').exists(), 'quay về chặng 1').toBe(false);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// F4 (M6 remediation) — đổi chặng phải reset scroll, kể cả 2 chặng CÙNG terrain.
// ─────────────────────────────────────────────────────────────────────────────

describe('test_switching_stage_resets_the_map_offset', () => {
  it('test_offset_from_stage_one_does_not_leak_into_stage_two', async () => {
    stub();
    const w = await mountScreenAt('/roadmap/trung');
    const scroller = w.get('[data-testid="map-scroll"]').element as HTMLElement;

    // Cuộn chặng 1 (offset > 0) rồi chuyển sang chặng 2 CÙNG terrain `PLAIN`.
    scroller.scrollTop = 240;
    await w.get('[data-testid="stage-chip-zh-g1"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();

    // Không reset ⇒ `onScrolled` ghi `{stageId: chặng 2, offset: 240}` vào
    // `localStorage` ⇒ vị trí sai được nhớ vĩnh viễn.
    expect(scroller.scrollTop, 'offset chặng trước mang sang chặng sau').toBe(0);
  });

  it('test_changing_stage_does_not_leave_a_stale_offset_in_local_storage', async () => {
    stub();
    const w = await mountScreenAt('/roadmap/trung');
    const scroller = w.get('[data-testid="map-scroll"]').element as HTMLElement;
    scroller.scrollTop = 240;
    scroller.dispatchEvent(new Event('scroll'));

    await w.get('[data-testid="stage-chip-zh-g1"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();
    scroller.dispatchEvent(new Event('scroll'));

    const saved = JSON.parse(localStorage.getItem('roadmap:trung:map') ?? '{}') as { stageId?: string; offset?: number };
    expect(saved.stageId).toBe('11');
    expect(saved.offset).toBe(0);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Việc dọn #5 — 3 flow xoá ở màn này phải có bước xác nhận, đúng pattern
// `RoadmapHome.vue` + `Bookmarks.vue`.
// ─────────────────────────────────────────────────────────────────────────────

describe('test_every_delete_asks_for_confirmation_first', () => {
  it('test_deleting_a_stage_does_not_call_the_server_on_the_first_click', async () => {
    const fetchMock = stub();
    const w = await mountScreenAt('/roadmap/trung?view=list');
    await w.get('[data-testid="ask-delete-stage-10"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();

    expect(opNamesOf(fetchMock)).not.toContain('DeleteStage');
    expect(w.find('[data-testid="delete-stage-10"]').exists()).toBe(true);
  });

  it('test_deleting_a_stage_happens_on_the_second_click', async () => {
    const fetchMock = stub();
    const w = await mountScreenAt('/roadmap/trung?view=list');
    await w.get('[data-testid="ask-delete-stage-10"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();
    await w.get('[data-testid="delete-stage-10"]').trigger('click');
    for (let i = 0; i < 8; i += 1) await flushPromises();

    expect(opNamesOf(fetchMock)).toContain('DeleteStage');
  });

  it('test_deleting_a_topic_does_not_call_the_server_on_the_first_click', async () => {
    const fetchMock = stub();
    const w = await mountScreenAt('/roadmap/trung?view=list');
    await w.get('[data-testid="ask-delete-topic-101"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();

    expect(opNamesOf(fetchMock)).not.toContain('DeleteTopic');
    expect(w.find('[data-testid="delete-topic-101"]').exists()).toBe(true);
  });

  it('test_deleting_a_topic_happens_on_the_second_click', async () => {
    const fetchMock = stub();
    const w = await mountScreenAt('/roadmap/trung?view=list');
    await w.get('[data-testid="ask-delete-topic-101"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();
    await w.get('[data-testid="delete-topic-101"]').trigger('click');
    for (let i = 0; i < 8; i += 1) await flushPromises();

    expect(opNamesOf(fetchMock)).toContain('DeleteTopic');
  });

  it('test_deleting_a_milestone_does_not_call_the_server_on_the_first_click', async () => {
    const fetchMock = stub();
    const w = await mountScreenAt('/roadmap/trung?view=list');
    await w.get('[data-testid="ask-delete-milestone-m1"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();

    expect(opNamesOf(fetchMock)).not.toContain('DeleteMilestone');
    expect(w.find('[data-testid="delete-milestone-m1"]').exists()).toBe(true);
  });

  it('test_deleting_a_milestone_happens_on_the_second_click', async () => {
    const fetchMock = stub();
    const w = await mountScreenAt('/roadmap/trung?view=list');
    await w.get('[data-testid="ask-delete-milestone-m1"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();
    await w.get('[data-testid="delete-milestone-m1"]').trigger('click');
    for (let i = 0; i < 8; i += 1) await flushPromises();

    expect(opNamesOf(fetchMock)).toContain('DeleteMilestone');
  });

  it('test_cancel_drops_the_pending_confirmation', async () => {
    const fetchMock = stub();
    const w = await mountScreenAt('/roadmap/trung?view=list');
    await w.get('[data-testid="ask-delete-stage-10"]').trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();
    await w.findAll('button').find((b) => b.text() === 'Huỷ')!.trigger('click');
    for (let i = 0; i < 4; i += 1) await flushPromises();

    expect(w.find('[data-testid="delete-stage-10"]').exists()).toBe(false);
    expect(opNamesOf(fetchMock)).not.toContain('DeleteStage');
  });
});
