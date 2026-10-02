<!--
  Panel chi tiết 1 màn + cử chỉ vuốt đánh dấu Xong.

  Đây là nơi DUY NHẤT cử chỉ "vuốt lên = Xong" tồn tại (ROADMAP-MAP-IDEA §4):
  nó chỉ gắn ở panel, không gắn ở `MapCanvas`, nên vuốt trên bản đồ (để cuộn)
  không bao giờ đánh dấu nhầm.

  Nút "Xong" LUÔN có (đường chính, dùng được bằng bàn phím). Vuốt là đường tắt.
-->
<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import Badge from '../../components/ui/Badge.vue';
import Button from '../../components/ui/Button.vue';
import { LEVEL_STYLE, type NodeLevel } from './geometry';
import { isSwipe } from './swipe';
import type { ResourceKind, RoadmapResource, RoadmapTopic } from '../api';

const props = defineProps<{
  topic: RoadmapTopic;
  /** Ẩn nút "vào /review" khi stage chưa gắn deck — không render nút chết. */
  deckId: string | null;
}>();

const emit = defineEmits<{
  close: [];
  mark: [status: 'DONE' | 'IN_PROGRESS' | 'NOT_STARTED' | 'SKIPPED'];
  swipeDone: [];
  openResource: [resource: RoadmapResource];
  edit: [topic: RoadmapTopic];
}>();

const KIND_LABEL: Record<ResourceKind, string> = {
  VIDEO: 'Video',
  ARTICLE: 'Bài đọc',
  TOOL: 'Công cụ',
  APP: 'Ứng dụng',
  BOOK: 'Sách',
  COURSE: 'Khoá học',
  SITE: 'Website',
  PODCAST: 'Podcast',
  CHANNEL: 'Kênh',
};

const style = computed(() => LEVEL_STYLE[props.topic.level as NodeLevel] ?? LEVEL_STYLE.LOCKED);
const done = computed(() => props.topic.status === 'DONE' || props.topic.status === 'SKIPPED');

/**
 * Vuốt LÊN trong panel đánh dấu Xong. `pointerdown→pointerup` delta > 80px,
 * đúng trục dọc, đúng chiều lên, đúng 1 lần cho mỗi lần mở panel — `armed`
 * khoá lại sau khi nổ để một lần vuốt dài không ghi 2 lần `DONE`.
 *
 * Vuốt ngang trong panel KHÔNG đánh dấu: đó là cử chỉ vuốt bản đồ mà lọt vào
 * (ngón tay trượt sang ngang khi panel vừa mở), và ăn mất thao tác hoàn thành.
 */
const armed = ref(true);
const dragStart = ref<{ x: number; y: number } | null>(null);

watch(
  () => props.topic.id,
  () => {
    armed.value = true;
    dragStart.value = null;
  },
);

function onPointerDown(e: PointerEvent): void {
  if (e.pointerType === 'mouse' && e.button !== 0) return;
  dragStart.value = { x: e.clientX, y: e.clientY };
}

function onPointerUp(e: PointerEvent): void {
  const start = dragStart.value;
  dragStart.value = null;
  if (!start || !armed.value || done.value) return;
  const dx = e.clientX - start.x;
  const dy = e.clientY - start.y;
  // `isSwipe(…, 'y')` đã bao gồm cả luật trục (trục ngang phải NHỎ HƠN trục
  // dọc) lẫn ngưỡng 80px — không cần thêm `dominantAxis` ở trên, vì nó lặp lại
  // đúng nửa đầu của cùng một phép so. Ở đây chỉ cần chặn chiều xuống.
  if (dy > 0) return;
  if (!isSwipe({ dx, dy }, 'y')) return;
  armed.value = false;
  emit('swipeDone');
}
</script>

<template>
  <aside
    data-testid="level-panel"
    class="grain relative overflow-hidden rounded-lg border border-line bg-card p-4 shadow-lift"
    @pointerdown="onPointerDown"
    @pointerup="onPointerUp"
    @pointercancel="dragStart = null"
  >
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <Badge :tone="done ? 'jade' : topic.level === 'CURRENT' ? 'ember' : 'locked'">
            {{ style.label }}
          </Badge>
          <Badge v-if="topic.isOptional" tone="neutral">Tham khảo</Badge>
          <Badge v-if="topic.mapPinned" tone="amber">Đã đặt tay</Badge>
        </div>
        <h3 class="mt-1.5 font-display text-xl font-semibold text-foreground">{{ topic.title }}</h3>
        <p v-if="topic.why" class="mt-1 text-[14px] text-muted-foreground">{{ topic.why }}</p>
        <p v-if="topic.completedAt" class="mt-1 text-[13px] text-muted-foreground">
          Xong lúc {{ topic.completedAt }}
        </p>
        <p v-if="topic.statusNote" class="mt-1 text-[13px] text-muted-foreground">
          Ghi chú: {{ topic.statusNote }}
        </p>
      </div>
      <Button variant="ghost" size="icon" aria-label="đóng panel" @click="emit('close')">×</Button>
    </div>

    <section v-if="topic.activityList.length > 0" class="mt-4">
      <h4 class="text-[13px] font-semibold uppercase tracking-wide text-muted-foreground">Hoạt động</h4>
      <ul class="mt-1.5 space-y-1">
        <li v-for="(a, i) in topic.activityList" :key="i" class="text-[14px]">
          <span class="tabular mr-1.5 text-muted-foreground">{{ i + 1 }}.</span>{{ a }}
        </li>
      </ul>
    </section>

    <section class="mt-4">
      <h4 class="text-[13px] font-semibold uppercase tracking-wide text-muted-foreground">
        Tài liệu ({{ topic.resources.length }})
      </h4>
      <p v-if="topic.resources.length === 0" class="mt-1 text-[14px] text-muted-foreground">
        Màn này chưa có tài liệu.
      </p>
      <ul v-else class="mt-1.5 space-y-1.5">
        <li
          v-for="r in topic.resources"
          :key="r.id"
          data-testid="resource-row"
          class="flex flex-wrap items-baseline gap-x-2 gap-y-0.5"
        >
          <Badge tone="neutral">{{ KIND_LABEL[r.kind] }}</Badge>
          <a
            v-if="r.url"
            :href="r.url"
            target="_blank"
            rel="noopener noreferrer"
            class="underline decoration-dotted hover:decoration-solid"
            >{{ r.title }}</a
          >
          <!-- Tài liệu không có link vẫn hiện tên + badge, KHÔNG giấu: người dùng
               còn cần biết màn này có tài liệu nào để tự tìm. -->
          <span v-else class="text-muted-foreground">{{ r.title }}</span>
          <Badge v-if="!r.url" tone="locked">chưa có link</Badge>
          <span v-if="r.note" class="text-[13px] text-muted-foreground">{{ r.note }}</span>
          <Button variant="ghost" size="sm" @click="emit('openResource', r)">Sửa</Button>
        </li>
      </ul>
    </section>

    <section class="mt-5 flex flex-wrap items-center gap-2 border-t border-border pt-4">
      <Button v-if="!done" data-testid="mark-done" @click="emit('mark', 'DONE')">
        Đánh dấu Xong
      </Button>
      <Button v-else variant="outline" data-testid="mark-reopen" @click="emit('mark', 'NOT_STARTED')">
        Mở lại
      </Button>
      <Button
        v-if="topic.status === 'NOT_STARTED'"
        variant="outline"
        @click="emit('mark', 'IN_PROGRESS')"
      >
        Đang học
      </Button>
      <Button
        v-if="!done"
        variant="ghost"
        @click="emit('mark', 'SKIPPED')"
      >
        Bỏ qua
      </Button>
      <Button variant="ghost" @click="emit('edit', topic)">Sửa màn</Button>
      <RouterLink
        v-if="deckId"
        data-testid="goto-review"
        class="ml-auto inline-flex h-9 items-center rounded-md bg-primary px-3.5 text-sm font-medium text-primary-foreground hover:brightness-110"
        :to="`/review?deck=${deckId}`"
      >
        Ôn bằng deck này
      </RouterLink>
    </section>

    <p class="mt-3 text-[13px] text-muted-foreground">
      Vuốt lên trong khung này cũng đánh dấu Xong. Vuốt bên ngoài chỉ cuộn bản đồ.
    </p>
  </aside>
</template>
