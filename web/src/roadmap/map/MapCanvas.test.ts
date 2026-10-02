// 3 CỬ CHỈ TÁCH VỊ TRÍ — test quyết định của M6 (ROADMAP-MAP-IDEA §4).
//
// Ba cử chỉ phải không nuốt lẫn nhau:
//   · vuốt TRÊN bản đồ  → cuộn
//   · chạm node         → mở panel
//   · vuốt TRONG panel  → đánh dấu Xong
//
// Nếu 2 cử chỉ đầu trùng nhau thì vuốt để cuộn lại đánh dấu nhầm màn; nếu cử
// chỉ 3 gắn ra ngoài panel thì vuốt cuộn biến thành đánh dấu. Cả 2 đều là
// mất dữ liệu âm thầm, không báo lỗi — đúng loại lỗi mà test hành vi bắt được
// còn đọc code thì không.
import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import LevelPanel from './LevelPanel.vue';
import MapCanvas from './MapCanvas.vue';
import type { MapNode } from './MapCanvas.vue';
import type { RoadmapTopic } from '../api';

const NODES: MapNode[] = [
  { id: 'n1', title: 'Chào', point: { x: 500, y: 1800 }, level: 'DONE', isOptional: false, index: 1 },
  { id: 'n2', title: 'Số đếm', point: { x: 500, y: 1600 }, level: 'CURRENT', isOptional: false, index: 2 },
  { id: 'n3', title: 'Ăn uống', point: { x: 500, y: 1400 }, level: 'LOCKED', isOptional: false, index: 3 },
];

/** Chặng 2 — khác `id`, khác toạ độ, cùng `terrain`. */
const NODES_2: MapNode[] = [
  { id: 'm1', title: 'Mua bán', point: { x: 500, y: 1800 }, level: 'DONE', isOptional: false, index: 1 },
  { id: 'm2', title: 'Hỏi đường', point: { x: 500, y: 1600 }, level: 'CURRENT', isOptional: false, index: 2 },
  { id: 'm3', title: 'Thời gian', point: { x: 500, y: 1400 }, level: 'LOCKED', isOptional: false, index: 3 },
];

function topic(over: Partial<RoadmapTopic> = {}): RoadmapTopic {
  return {
    id: 'n2',
    stageId: '10',
    title: 'Số đếm',
    why: 'Đếm được tới 100',
    activityList: ['Nghe 10 câu', 'Đếm lại'],
    position: 1,
    status: 'IN_PROGRESS',
    statusNote: '',
    completedAt: null,
    isOptional: false,
    mapX: null,
    mapY: null,
    level: 'CURRENT',
    point: { x: 500, y: 1600 },
    mapPinned: false,
    resources: [],
    ...over,
  };
}

/**
 * Pointer event giả. `Event` không nhận `clientX`/`clientY` (chỉ `MouseEvent`
 * mới có) nên phải gán thủ công — dùng `new MouseEvent(name, …)` rồi ghi đè là
 * cách duy nhất cho đủ 3 phase mà happy-dom không có `PointerEvent`.
 */
async function swipe(el: Element, dx: number, dy: number) {
  const fire = async (name: string, x: number, y: number) => {
    const ev = new MouseEvent(name, { bubbles: true, clientX: x, clientY: y });
    Object.defineProperty(ev, 'pointerId', { value: 1 });
    Object.defineProperty(ev, 'pointerType', { value: 'touch' });
    await el.dispatchEvent(ev);
  };
  await fire('pointerdown', 100, 200);
  await fire('pointermove', 100 + dx, 200 + dy);
  await fire('pointerup', 100 + dx, 200 + dy);
}

/**
 * `LevelPanel` dùng `RouterLink`. Component CHƯA đăng ký thì `global.stubs`
 * của vue-test-utils KHÔNG bắt được (nó chỉ thay thế component đã resolve) —
 * `RouterLink` sẽ render thành thẻ rỗng `<routerlink>` không có `href`, mà
 * `href` chính là thứ test dưới đây kiểm. Vì vậy đăng ký nó như component
 * thật thay vì stub.
 */
const RouterLink = { props: ['to'], template: '<a :href="to"><slot /></a>' };
const GLOBAL = { global: { components: { RouterLink } } };

describe('test_map_gesture_1_swipe_scrolls_only', () => {
  it('test_vertical_drag_moves_the_vertical_offset_on_an_up_map', async () => {
    const w = mount(MapCanvas, { ...GLOBAL, props: { slug: 'trung', terrain: 'MEADOW', direction: 'UP', nodes: NODES } });
    const scroller = w.get('[data-testid="map-scroll"]').element as HTMLElement;
    const before = scroller.scrollTop;

    await swipe(scroller, 0, -200);

    // Ngón tay kéo lên ⇒ nội dung dịch lên ⇒ scrollTop tăng.
    expect(scroller.scrollTop).toBeGreaterThan(before);
  });

  it('test_horizontal_drag_does_not_scroll_a_vertical_map', async () => {
    // `direction = up` khoá trục X. Vuốt ngang trên bản đồ dọc là cử chỉ rác:
    // cho nó cuộn được thì cuộn ngang sẽ làm người dùng lạc chặng.
    const w = mount(MapCanvas, { ...GLOBAL, props: { slug: 'trung', terrain: 'MEADOW', direction: 'UP', nodes: NODES } });
    const scroller = w.get('[data-testid="map-scroll"]').element as HTMLElement;

    await swipe(scroller, 300, 0);

    expect(scroller.scrollTop).toBe(0);
    expect(scroller.scrollLeft).toBe(0);
  });

  it('test_diagonal_drag_does_not_scroll_a_vertical_map', async () => {
    // Vuốt chéo: trục X chiếm ưu thế ⇒ KHÔNG phải cuộn dọc. Bỏ khoá trục
    // phụ thì vuốt chéo lại làm bản đồ trượt — mất kiểm soát vị trí đang xem.
    const w = mount(MapCanvas, { ...GLOBAL, props: { slug: 'trung', terrain: 'MEADOW', direction: 'UP', nodes: NODES } });
    const scroller = w.get('[data-testid="map-scroll"]').element as HTMLElement;

    await swipe(scroller, 200, -40);

    expect(scroller.scrollTop).toBe(0);
  });

  it('test_scrolling_a_map_emits_no_mark_event', async () => {
    const w = mount(MapCanvas, { ...GLOBAL, props: { slug: 'trung', terrain: 'MEADOW', direction: 'UP', nodes: NODES } });
    await swipe(w.get('[data-testid="map-scroll"]').element, 0, -200);
    expect(w.emitted('select')).toBeUndefined();
  });

  it('test_right_direction_map_scrolls_horizontally', async () => {
    const w = mount(MapCanvas, { ...GLOBAL, props: { slug: 'en', terrain: 'OCEAN', direction: 'RIGHT', nodes: NODES } });
    const scroller = w.get('[data-testid="map-scroll"]').element as HTMLElement;

    // Bản đồ đi trái→phải nên đi tiếp = kéo ngón tử sang TRÁI.
    await swipe(scroller, -300, 0);

    expect(scroller.scrollLeft).toBeGreaterThan(0);
  });

  it('test_drag_past_the_start_stays_at_zero', async () => {
    // Không khoá thì scroll âm là trạng thái không hợp lệ: kéo ngược lại bản
    // đồ nhảy vị trí.
    const w = mount(MapCanvas, { ...GLOBAL, props: { slug: 'en', terrain: 'OCEAN', direction: 'RIGHT', nodes: NODES } });
    const scroller = w.get('[data-testid="map-scroll"]').element as HTMLElement;

    await swipe(scroller, 300, 0);

    expect(scroller.scrollLeft).toBe(0);
  });
});

describe('test_map_offset_resets_when_the_map_changes', () => {
  it('test_two_stages_with_the_same_terrain_still_reset_the_offset', async () => {
    // F4 (M6 remediation): `MapCanvas` không có `:key` nên đổi chặng tái dùng
    // cùng instance. `watch(() => props.terrain)` KHÔNG chạy khi 2 chặng cùng
    // terrain (đúng tình trạng thật do lỗi `enum Terrain`: mọi chặng đều là
    // `PLAIN`) ⇒ offset chặng trước mang sang chặng sau, rồi `onScrolled` ghi
    // `{stageId: chặng mới, offset: cũ}` vào `localStorage` ⇒ vị trí sai được
    // nhớ vĩnh viễn. Fixture dùng CÙNG `terrain` để F4 vẫn lộ sau khi F1 sửa.
    const w = mount(MapCanvas, { ...GLOBAL, props: { slug: 'trung', terrain: 'MEADOW', direction: 'UP', nodes: NODES } });
    const scroller = w.get('[data-testid="map-scroll"]').element as HTMLElement;
    await swipe(scroller, 0, -200);
    expect(scroller.scrollTop).toBeGreaterThan(0);

    await w.setProps({ nodes: NODES_2 });

    expect(scroller.scrollTop, 'offset chặng trước mang sang chặng sau').toBe(0);
  });

  it('test_a_plain_reload_of_the_same_stage_keeps_the_offset', async () => {
    // Ngược lại: `load()` dựng lại mảng `nodes` MỚI sau mọi mutation. Nếu
    // watch bám tham chiếu mảng thì mỗi lần đánh dấu Xong lại cuộn về đầu.
    const w = mount(MapCanvas, { ...GLOBAL, props: { slug: 'trung', terrain: 'MEADOW', direction: 'UP', nodes: NODES } });
    const scroller = w.get('[data-testid="map-scroll"]').element as HTMLElement;
    await swipe(scroller, 0, -200);
    const kept = scroller.scrollTop;

    await w.setProps({ nodes: NODES.map((n) => ({ ...n })) });

    expect(scroller.scrollTop).toBe(kept);
  });

  it('test_switching_path_resets_the_offset', async () => {
    const w = mount(MapCanvas, { ...GLOBAL, props: { slug: 'trung', terrain: 'MEADOW', direction: 'UP', nodes: NODES } });
    const scroller = w.get('[data-testid="map-scroll"]').element as HTMLElement;
    await swipe(scroller, 0, -200);

    await w.setProps({ slug: 'toan' });

    expect(scroller.scrollTop).toBe(0);
  });
});

describe('test_map_gesture_2_tap_node_opens_panel', () => {
  it('test_tapping_a_current_node_selects_it', async () => {
    const w = mount(MapCanvas, { ...GLOBAL, props: { slug: 'trung', terrain: 'MEADOW', direction: 'UP', nodes: NODES } });
    await w.get('[data-testid="node-hit-n2"]').trigger('click');
    expect(w.emitted('select')).toEqual([['n2']]);
  });

  it('test_tapping_a_done_node_reopens_it', async () => {
    const w = mount(MapCanvas, { ...GLOBAL, props: { slug: 'trung', terrain: 'MEADOW', direction: 'UP', nodes: NODES } });
    await w.get('[data-testid="node-hit-n1"]').trigger('click');
    expect(w.emitted('select')).toEqual([['n1']]);
  });

  it('test_locked_node_cannot_be_tapped', async () => {
    const w = mount(MapCanvas, { ...GLOBAL, props: { slug: 'trung', terrain: 'MEADOW', direction: 'UP', nodes: NODES } });
    const locked = w.get('[data-testid="node-hit-n3"]');
    await locked.trigger('click');
    expect(w.emitted('select')).toBeUndefined();
    expect(locked.classes()).toContain('cursor-not-allowed');
  });

  it('test_node_state_is_readable_from_the_dom', () => {
    const w = mount(MapCanvas, { ...GLOBAL, props: { slug: 'trung', terrain: 'MEADOW', direction: 'UP', nodes: NODES } });
    expect(w.get('[data-testid="node-n3"]').attributes('data-level')).toBe('LOCKED');
    expect(w.get('[data-testid="node-n1"]').text()).toContain('★');
  });
});

describe('test_map_gesture_3_swipe_inside_panel_marks_done', () => {
  it('test_swipe_up_inside_the_panel_emits_swipe_done', async () => {
    const w = mount(LevelPanel, { ...GLOBAL, props: { topic: topic(), deckId: null } });
    await swipe(w.get('[data-testid="level-panel"]').element, 0, -150);
    expect(w.emitted('swipeDone')).toHaveLength(1);
  });

  it('test_panel_swipe_does_not_repeat_on_one_long_drag', async () => {
    // 1 lần cho mỗi lần mở panel: vuốt dài 300px là 1 cử chỉ, không phải 4.
    const w = mount(LevelPanel, { ...GLOBAL, props: { topic: topic(), deckId: null } });
    await swipe(w.get('[data-testid="level-panel"]').element, 0, -300);
    expect(w.emitted('swipeDone')).toHaveLength(1);
  });

  it('test_small_drag_inside_panel_does_not_mark_done', async () => {
    const w = mount(LevelPanel, { ...GLOBAL, props: { topic: topic(), deckId: null } });
    await swipe(w.get('[data-testid="level-panel"]').element, 0, -40);
    expect(w.emitted('swipeDone')).toBeUndefined();
  });

  it('test_swipe_down_inside_panel_does_not_mark_done', async () => {
    // Vuốt lên = Xong. Vuốt xuống là đóng panel, không phải hoàn thành.
    const w = mount(LevelPanel, { ...GLOBAL, props: { topic: topic(), deckId: null } });
    await swipe(w.get('[data-testid="level-panel"]').element, 0, 150);
    expect(w.emitted('swipeDone')).toBeUndefined();
  });

  it('test_horizontal_swipe_inside_panel_does_not_mark_done', async () => {
    // Vuốt ngang là cử chỉ cuộn bản đồ lọt vào panel. Ăn nó ở đây thì mất thao
    // tác hoàn thành mà người dùng không hiểu vì sao.
    const w = mount(LevelPanel, { ...GLOBAL, props: { topic: topic(), deckId: null } });
    await swipe(w.get('[data-testid="level-panel"]').element, 300, 0);
    expect(w.emitted('swipeDone')).toBeUndefined();
  });

  it('test_diagonal_swipe_leans_on_vertical_axis', async () => {
    // Vuốt lên kèm trượt ngang vẫn là vuốt lên: trục dọc vẫn chiếm ưu thế.
    const w = mount(LevelPanel, { ...GLOBAL, props: { topic: topic(), deckId: null } });
    await swipe(w.get('[data-testid="level-panel"]').element, 45, -200);
    expect(w.emitted('swipeDone')).toHaveLength(1);
  });

  it('test_swipe_on_an_already_done_topic_does_not_re_mark', async () => {
    const w = mount(LevelPanel, {
      ...GLOBAL,
      props: { topic: topic({ status: 'DONE', completedAt: '2026-09-28T00:00:00Z' }), deckId: null },
    });
    await swipe(w.get('[data-testid="level-panel"]').element, 0, -150);
    expect(w.emitted('swipeDone')).toBeUndefined();
  });
});

describe('test_done_button_works_without_any_swipe', () => {
  it('test_mark_done_button_emits_done', async () => {
    const w = mount(LevelPanel, { ...GLOBAL, props: { topic: topic(), deckId: null } });
    await w.get('[data-testid="mark-done"]').trigger('click');
    expect(w.emitted('mark')).toEqual([['DONE']]);
  });

  it('test_reopen_button_is_shown_only_after_completion', async () => {
    const w = mount(LevelPanel, { ...GLOBAL, props: { topic: topic({ status: 'DONE' }), deckId: null } });
    expect(w.find('[data-testid="mark-done"]').exists()).toBe(false);
    await w.get('[data-testid="mark-reopen"]').trigger('click');
    expect(w.emitted('mark')).toEqual([['NOT_STARTED']]);
  });

  it('test_fresh_level_offers_start_and_skip_alongside_done', () => {
    const w = mount(LevelPanel, {
      ...GLOBAL,
      props: { topic: topic({ status: 'NOT_STARTED' }), deckId: null },
    });
    const labels = w.findAll('button').map((b) => b.text());
    expect(labels.some((l) => l.includes('Đánh dấu Xong'))).toBe(true);
    expect(labels.some((l) => l.includes('Đang học'))).toBe(true);
    expect(labels.some((l) => l.includes('Bỏ qua'))).toBe(true);
  });

  it('test_in_progress_level_offers_done_skip_and_reopen', () => {
    const w = mount(LevelPanel, { ...GLOBAL, props: { topic: topic({ status: 'IN_PROGRESS' }), deckId: null } });
    const labels = w.findAll('button').map((b) => b.text());
    expect(labels.some((l) => l.includes('Đánh dấu Xong'))).toBe(true);
    expect(labels.some((l) => l.includes('Bỏ qua'))).toBe(true);
  });
});

describe('test_review_button_depends_on_deck', () => {
  it('test_review_link_hidden_when_stage_has_no_deck', () => {
    // `deckId` nullable — render nút chết là nút bấm không đi đâu.
    const w = mount(LevelPanel, { ...GLOBAL, props: { topic: topic(), deckId: null } });
    expect(w.find('[data-testid="goto-review"]').exists()).toBe(false);
  });

  it('test_review_link_shown_when_stage_has_a_deck', () => {
    const w = mount(LevelPanel, { ...GLOBAL, props: { topic: topic(), deckId: '4' } });
    const link = w.get('[data-testid="goto-review"]');
    expect(link.attributes('href')).toBe('/review?deck=4');
  });
});

describe('test_resource_list', () => {
  it('test_resource_without_url_is_shown_with_a_badge_not_hidden', () => {
    const w = mount(LevelPanel, {
      ...GLOBAL,
      props: {
        topic: topic({
          resources: [
            {
              id: 'r1', topicId: 'n2', title: 'Giáo trình', url: null, kind: 'BOOK',
              note: '', position: 0, createdAt: 'x', updatedAt: 'x',
            },
          ],
        }),
        deckId: null,
      },
    });
    expect(w.text()).toContain('Giáo trình');
    expect(w.text()).toContain('chưa có link');
  });

  it('test_resource_with_url_opens_in_a_new_tab', () => {
    const w = mount(LevelPanel, {
      ...GLOBAL,
      props: {
        topic: topic({
          resources: [
            {
              id: 'r1', topicId: 'n2', title: 'Bài giảng', url: 'https://example.com', kind: 'VIDEO',
              note: '', position: 0, createdAt: 'x', updatedAt: 'x',
            },
          ],
        }),
        deckId: null,
      },
    });
    const a = w.get('a');
    expect(a.attributes('href')).toBe('https://example.com');
    expect(a.attributes('target')).toBe('_blank');
    expect(a.attributes('rel')).toContain('noopener');
  });
});
