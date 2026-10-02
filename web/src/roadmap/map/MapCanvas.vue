<!--
  Bản đồ 1 chặng (1 stage) — SVG là xương sống, Canvas 2D chỉ để vẽ confetti
  (ROADMAP-MAP-IDEA §1: không Three.js, không WebGL).

  Toạ độ node đến từ server (`Topic.point`). Ở đây KHÔNG tính lại layout:
  `fitFrame` chỉ bọc toạ độ đó vào khung vừa nội dung, `trailPath` chỉ nối chúng
  thành đường — xem `map/geometry.ts` để biết vì sao 2 việc đó tách khỏi việc
  sinh toạ độ.

  Cử chỉ (ROADMAP-MAP-IDEA §4), 3 cái tách theo vị trí:
    · vuốt TRÊN khung bản đồ → cuộn bản đồ theo `direction` (xử ở đây)
    · chạm node                → mở panel (emit `select`)
    · vuốt TRONG panel         → đánh dấu Xong (không ở đây — ở `LevelPanel`)
  Nhờ tách vị trí mà vuốt trên bản đồ không bao giờ đánh dấu nhầm.
-->
<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import {
  LEVEL_STYLE,
  MILESTONE_RADIUS,
  NODE_RADIUS,
  NODE_RADIUS_OPTIONAL,
  fitFrame,
  isPlayable,
  trailPath,
  type MapPoint,
  type NodeLevel,
} from './geometry';
import { TERRAIN_SKIN, isHorizontal, terrainBands, terrainLabel, type Direction, type Terrain } from './terrain';
import { dominantAxis } from './swipe';

export interface MapNode {
  id: string;
  title: string;
  point: MapPoint;
  level: NodeLevel;
  isOptional: boolean;
  /** Số thứ tự hiện trong node (1-based). */
  index: number;
}

const props = defineProps<{
  slug: string;
  terrain: Terrain;
  direction: Direction;
  nodes: readonly MapNode[];
  /** Mốc chặng: 1 hình thoi trên đường đi. */
  milestones?: readonly { id: string; text: string; position: number }[];
  selectedId?: string | null;
}>();

const emit = defineEmits<{ select: [id: string]; scrolled: [offset: number] }>();

/**
 * Kích thước canvas theo TRỤC CHÍNH, không đo DOM.
 *
 * Bản đồ dọc (`up`) cuộn theo Y nên bề rộng phải vừa khung nhìn; bản đồ ngang
 * (`right`) cuộn theo X nên bề cao phải vừa khung nhìn. Dùng hằng số thay vì
 * `getBoundingClientRect()` để cùng dữ liệu cho ra cùng kích thước ở mọi lần
 * render và ở cả test (`happy-dom` trả 0 cho mọi hình học) — cùng lý do các
 * hằng số layout ở M2 đóng băng thay vì dùng phép đo DOM.
 *
 * `Math.min(1, …)`: map hẹp (chặng có 5 node gần như thẳng hàng) không được phóng
 * to lên — phóng to làm node to bằng cả khung nhìn và nhãn chồng nhau.
 */
const CANVAS_WIDTH = 720;
const CANVAS_HEIGHT = 520;

const skin = computed(() => TERRAIN_SKIN[props.terrain] ?? TERRAIN_SKIN.MEADOW);
const horizontal = computed(() => isHorizontal(props.direction));
const frame = computed(() => fitFrame(props.nodes.map((n) => n.point)));
const viewBox = computed(
  () => `${frame.value.minX} ${frame.value.minY} ${frame.value.width} ${frame.value.height}`,
);

const pixels = computed(() => {
  const { width, height } = frame.value;
  if (horizontal.value) {
    const k = Math.min(1, CANVAS_HEIGHT / height);
    return { width: Math.round(width * k), height: Math.round(height * k) };
  }
  const k = Math.min(1, CANVAS_WIDTH / width);
  return { width: Math.round(width * k), height: Math.round(height * k) };
});

/** `seed` = độ dài slug + số node để mỗi bản đồ nền khác nhau mà vẫn tất định. */
const bands = computed(() =>
  terrainBands(
    props.terrain,
    props.slug.length + props.nodes.length,
    frame.value.width,
    frame.value.height,
  ),
);

const trail = computed(() => trailPath(props.nodes.map((n) => n.point)));

/** Mốc chặng nằm giữa 2 node kế cập, theo `position` — rẽ ra khỏi đường chính. */
const diamonds = computed(() => {
  const list = props.nodes;
  return (props.milestones ?? []).map((m) => {
    const at = Math.min(Math.max(m.position, 1), Math.max(list.length - 1, 1)) - 1;
    const a = list[at]?.point;
    const b = list[at + 1]?.point ?? a;
    if (!a) return { id: m.id, x: 0, y: 0, text: m.text };
    return { id: m.id, x: (a.x + b.x) / 2, y: (a.y + b.y) / 2, text: m.text };
  });
});

/**
 * Viewport theo slug — `Map` phẳng, cố ý KHÔNG bọc theo dõi thay đổi.
 *
 * Hàm `:ref` được gọi lại ở mỗi lần render; lưu phần tử vào nguồn theo dõi thì
 * bản thân việc gán trong hàm đó lại kích hoạt render tiếp ⇒ "Maximum recursive
 * updates exceeded". Ở đây phần tử chỉ được đọc bởi hàm mệnh lệnh (`restore`,
 * đọc offset), không dùng trong template, nên không cần theo dõi thay đổi.
 */
const viewports = new Map<string, HTMLElement | null>();

function setViewport(slug: string, el: unknown): void {
  viewports.set(slug, (el as HTMLElement | null) ?? null);
}

function restore(slug: string, offset: number): void {
  const el = viewports.get(slug);
  if (!el) return;
  if (horizontal.value) el.scrollLeft = Math.max(0, offset);
  else el.scrollTop = Math.max(0, offset);
}

defineExpose({ restore, viewport: () => viewports.get(props.slug) ?? null });

/**
 * Vuốt trên bản đồ = cuộn, khoá đúng trục của `direction`. Cử động chéo bị bỏ
 * qua nhờ `dominantAxis` để không giật bản đồ khi ngón tay chệch — cùng ngưỡng
 * nhiễu mà `swipe.ts` dùng cho cử chỉ.
 */
const drag = ref<{ x: number; y: number; from: number } | null>(null);

/** `scrollTop`/`scrollLeft` là số ≥ 0; chuẩn hoá để phép trừ bên dưới không nhận
 *  `undefined`/`NaN` từ phần tử chưa có nội dung. */
function currentOffset(): number {
  const el = viewports.get(props.slug);
  if (!el) return 0;
  const raw = horizontal.value ? el.scrollLeft : el.scrollTop;
  return Number.isFinite(raw) ? raw : 0;
}

function onPointerDown(e: PointerEvent): void {
  if (e.pointerType === 'mouse' && e.button !== 0) return;
  drag.value = { x: e.clientX, y: e.clientY, from: currentOffset() };
  (e.currentTarget as HTMLElement).setPointerCapture?.(e.pointerId);
}

function onPointerMove(e: PointerEvent): void {
  const start = drag.value;
  if (!start) return;
  const dx = e.clientX - start.x;
  const dy = e.clientY - start.y;
  const axis = dominantAxis({ dx, dy });
  if (axis === null) return;
  if (axis !== (horizontal.value ? 'x' : 'y')) return;
  setOffset(start.from - (horizontal.value ? dx : dy));
}

function onPointerUp(): void {
  drag.value = null;
  emit('scrolled', currentOffset());
}

function onScroll(): void {
  emit('scrolled', currentOffset());
}

function setOffset(next: number): void {
  const el = viewports.get(props.slug);
  if (!el) return;
  // Khoá ở 0: `from - delta` âm khi vuốt quá đầu trang, và ghi scroll âm là
  // trạng thái không hợp lệ — ngón tay kéo ngược lại sẽ nhảy vị trí.
  const clamped = Math.max(0, next);
  if (horizontal.value) el.scrollLeft = clamped;
  else el.scrollTop = clamped;
}

/**
 * Cuộn về 0 khi **bản đồ** đổi, không phải khi `terrain` đổi.
 *
 * Vì sao không bám `terrain`: `MapCanvas` không có `:key` nên đổi chặng tái
 * dùng cùng instance và giữ nguyên `scrollTop`. `terrain` là thuộc tính HÌNH
 * ẢNH, không phải danh tính bản đồ — 2 chặng cùng `terrain` rất hay (với lỗi
 * `enum Terrain` ở server — trước M7a nó rơi về 1 giá trị duy nhất nên **mọi**
 * chặng cùng terrain), lúc đó watch
 * không chạy ⇒ offset chặng trước mang sang chặng sau ⇒ `onScrolled` ghi
 * `{stageId: chặng mới, offset: cũ}` vào `localStorage` ⇒ vị trí sai được
 * nhớ **vĩnh viễn**.
 *
 * Bám đúng 3 thứ quyết định "đây là bản đồ khác": `slug` (path khác),
 * `direction` (trục cuộn khác ⇒ `scrollTop` cũ vô nghĩa), và **danh sách id
 * node** (chặng khác). So sánh theo `id` chứ không theo tham chiếu mảng: sau
 * MỌI mutation `load()` dựng lại cả cây nên `nodes` là mảng MỚI dù vẫn là
 * CÙNG chặng — bám tham chiếu sẽ cuộn về 0 sau mỗi lần đánh dấu Xong.
 *
 * ⚠️ Getter trả về **chuỗi**, không trả mảng: `watch` so kết quả bằng
 * `Object.is`, mà mảng mới luôn khác mảng cũ ⇒ callback chạy mỗi lần effect
 * đánh giá lại, kể cả khi nội dung y hệt. `flush: 'post'` để chạy SAU khi
 * render: hàm `:ref` đăng ký phần tử theo `slug` trong lúc render, chạy
 * `pre` sẽ `setOffset(0)` lên phần tử của slug cũ (hoặc chưa có phần tử nào).
 */
watch(
  () => `${props.slug}|${props.direction}|${props.nodes.map((n) => n.id).join(',')}`,
  () => setOffset(0),
  { flush: 'post' },
);
</script>

<template>
  <div
    :data-testid="`map-${slug}`"
    class="grain relative overflow-hidden rounded-lg border border-line shadow-plate"
    :class="horizontal ? 'h-[520px]' : 'h-[620px]'"
  >
    <div
      :ref="(el) => setViewport(slug, el)"
      data-testid="map-scroll"
      class="h-full w-full overscroll-contain"
      :class="
        horizontal
          ? 'touch-pan-x overflow-x-auto overflow-y-hidden'
          : 'touch-pan-y overflow-y-auto overflow-x-hidden'
      "
      @pointerdown="onPointerDown"
      @pointermove="onPointerMove"
      @pointerup="onPointerUp"
      @pointercancel="onPointerUp"
      @scroll.passive="onScroll"
    >
      <svg
        :viewBox="viewBox"
        :width="pixels.width"
        :height="pixels.height"
        class="block"
        :class="horizontal ? '' : 'mx-auto'"
        role="img"
        :aria-label="`bản đồ ${terrainLabel(terrain)}`"
      >
        <defs>
          <linearGradient :id="`sky-${slug}`" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" :stop-color="skin.sky" />
            <stop offset="100%" :stop-color="skin.ground" />
          </linearGradient>
        </defs>

        <rect
          :x="frame.minX"
          :y="frame.minY"
          :width="frame.width"
          :height="frame.height"
          :fill="`url(#sky-${slug})`"
        />
        <g :transform="`translate(${frame.minX} ${frame.minY})`">
          <path
            v-for="(band, i) in bands"
            :key="`band-${i}`"
            :d="band.d"
            fill="var(--color-atlas)"
            :fill-opacity="band.opacity"
          />
        </g>

        <path
          :d="trail"
          fill="none"
          :stroke="skin.path"
          stroke-width="14"
          stroke-linecap="round"
          class="map-path"
          :style="{ '--path-length': 2400 }"
        />
        <path
          :d="trail"
          fill="none"
          :stroke="skin.rim"
          stroke-width="3"
          stroke-dasharray="1 26"
          stroke-linecap="round"
          opacity="0.6"
        />

        <g
          v-for="m in diamonds"
          :key="`m-${m.id}`"
          :transform="`translate(${m.x} ${m.y})`"
          data-testid="map-milestone"
        >
          <title>{{ m.text }}</title>
          <rect
            :x="-MILESTONE_RADIUS"
            :y="-MILESTONE_RADIUS"
            :width="MILESTONE_RADIUS * 2"
            :height="MILESTONE_RADIUS * 2"
            transform="rotate(45)"
            :fill="skin.path"
            :stroke="skin.rim"
            stroke-width="3"
          />
        </g>

        <g
          v-for="n in nodes"
          :key="`n-${n.id}`"
          :transform="`translate(${n.point.x} ${n.point.y})`"
          :data-testid="`node-${n.id}`"
          :data-level="n.level"
        >
          <circle
            v-if="LEVEL_STYLE[n.level].halo"
            class="node-breathe"
            :r="(n.isOptional ? NODE_RADIUS_OPTIONAL : NODE_RADIUS) + 14"
            fill="none"
            :stroke="skin.rim"
            stroke-width="4"
          />
          <circle
            v-if="isPlayable(n.level)"
            :r="(n.isOptional ? NODE_RADIUS_OPTIONAL : NODE_RADIUS) + 9"
            :fill="LEVEL_STYLE[n.level].fill"
            fill-opacity="0.18"
          />
          <circle
            :r="n.isOptional ? NODE_RADIUS_OPTIONAL : NODE_RADIUS"
            :fill="LEVEL_STYLE[n.level].fill"
            :stroke="n.isOptional ? skin.path : LEVEL_STYLE[n.level].stroke"
            :stroke-width="n.isOptional ? 3 : 5"
            :stroke-dasharray="n.isOptional ? '7 7' : undefined"
            :data-testid="`node-hit-${n.id}`"
            :class="isPlayable(n.level) ? 'cursor-pointer' : 'cursor-not-allowed'"
            @click.stop="isPlayable(n.level) && emit('select', n.id)"
          />
          <text
            v-if="n.level === 'DONE'"
            y="9"
            text-anchor="middle"
            :fill="LEVEL_STYLE.DONE.text"
            font-size="26"
            pointer-events="none"
          >
            ★
          </text>
          <text
            v-else
            y="10"
            text-anchor="middle"
            :fill="LEVEL_STYLE[n.level].text"
            font-size="28"
            font-family="var(--font-display)"
            pointer-events="none"
          >
            {{ n.index }}
          </text>
          <text
            v-if="n.level === 'LOCKED'"
            y="-9"
            text-anchor="middle"
            :fill="LEVEL_STYLE.LOCKED.text"
            font-size="20"
            pointer-events="none"
          >
            🔒
          </text>
          <text
            :y="(n.isOptional ? NODE_RADIUS_OPTIONAL : NODE_RADIUS) + 26"
            text-anchor="middle"
            :fill="skin.rim"
            font-size="20"
            font-family="var(--font-sans)"
            pointer-events="none"
          >
            {{ n.title.length > 16 ? `${n.title.slice(0, 15)}…` : n.title }}
          </text>
        </g>
      </svg>
    </div>
  </div>
</template>
