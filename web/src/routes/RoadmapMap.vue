<script setup lang="ts">
/**
 * `/roadmap/:slug` — bản đồ game + chế độ danh sách dự phòng.
 *
 * 2 chế độ, cùng dữ liệu:
 *   • `map`  — bản đồ SVG mỗi chặng 1 bản đồ. Mặc định lần đầu vào, và là lựa
 *     chọn được ghi lại (bản đồ đẹp nhưng không quét nhanh 51 màn).
 *   • `list` — chặng/màn dạng bảng dọc, quét nhanh toàn bộ nội dung.
 *
 * Lựa chọn chế độ và vị trí đang xem đều nằm ở `localStorage` (khoá
 * `roadmap:<slug>:<view>`, xem `map/viewPrefs.ts`).
 */
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import NoticeBar from '../components/NoticeBar.vue';
import Badge from '../components/ui/Badge.vue';
import Button from '../components/ui/Button.vue';
import ConfettiCanvas from '../roadmap/map/ConfettiCanvas.vue';
import LevelPanel from '../roadmap/map/LevelPanel.vue';
import MapCanvas, { type MapNode } from '../roadmap/map/MapCanvas.vue';
import MilestoneForm from '../roadmap/MilestoneForm.vue';
import ResourceForm from '../roadmap/ResourceForm.vue';
import StageForm from '../roadmap/StageForm.vue';
import TopicForm from '../roadmap/TopicForm.vue';
import {
  deleteMilestone,
  deleteStage,
  deleteTopic,
  fetchPath,
  setTopicStatus,
  type NodeStatus,
  type RoadmapMilestone,
  type RoadmapPath,
  type RoadmapResource,
  type RoadmapStage,
  type RoadmapTopic,
} from '../roadmap/api';
import { stageProgress } from '../roadmap/map/geometry';
import { terrainLabel, type Terrain } from '../roadmap/map/terrain';
import {
  readPosition,
  readView,
  writePosition,
  writeView,
  type ViewMode,
} from '../roadmap/map/viewPrefs';
import { errorMessage } from '../graphql/errors';

const route = useRoute();
const router = useRouter();

const path = ref<RoadmapPath | null>(null);
const err = ref('');
const busy = ref(true);
const view = ref<ViewMode>('map');
const stageId = ref('');
const openTopicId = ref<string | null>(null);
const celebrate = ref(0);

const stageForm = ref(false);
const editingStage = ref<RoadmapStage | null>(null);
const topicForm = ref(false);
const editingTopic = ref<RoadmapTopic | null>(null);
const resourceForm = ref(false);
const editingResource = ref<RoadmapResource | null>(null);
const milestoneForm = ref(false);
const editingMilestone = ref<RoadmapMilestone | null>(null);

/**
 * Khoá xác nhận xoá — 3 flow xoá ở màn này (chặng / màn / mốc) đều đi qua
 * đây, đúng pattern 2 màn anh em đã có (`RoadmapHome.vue` + `Bookmarks.vue`).
 * Trước đó 3 nút này gọi thẳng `remove*` ⇒ 1 chạm là xoá hẳn 1 chặng gồm mọi
 * màn, mốc và tài liệu bên trong, lệch với chính 2 flow xoá còn lại trong
 * sản phẩm. Xoá chặng là soft-delete nên server cứu được, nhưng "cứu được" là
 * lý do server, không phải lý do để bỏ bước xác nhận.
 *
 * Ràng chặt: chỉ 1 mục xác nhận mở tại 1 lần ⇒ đổi chặng hay bấm nút khác là
 * mất trạng thái chờ, tránh "Xoá thật" còn treo từ lần bấm 3 phút trước.
 * `null` (không phải `''`) vì `id` trong DB là chuỗi — `''` là giá trị hợp lệ
 * về hình thức và sẽ khớp nhầm với "không có".
 */
const confirming = ref<{ kind: 'stage' | 'topic' | 'milestone'; id: string } | null>(null);

function askDelete(kind: 'stage' | 'topic' | 'milestone', id: string): void {
  confirming.value = confirming.value?.kind === kind && confirming.value.id === id ? null : { kind, id };
}

function isConfirming(kind: 'stage' | 'topic' | 'milestone', id: string): boolean {
  return confirming.value?.kind === kind && confirming.value.id === id;
}

const slug = computed(() => String(route.params.slug ?? ''));
const stages = computed(() => path.value?.stages ?? []);
const stage = computed<RoadmapStage | null>(
  () => stages.value.find((s) => s.id === stageId.value) ?? stages.value[0] ?? null,
);
const openTopic = computed<RoadmapTopic | null>(
  () => stage.value?.topics.find((t) => t.id === openTopicId.value) ?? null,
);
const mapNodes = computed<MapNode[]>(() =>
  (stage.value?.topics ?? []).map((t, i) => ({
    id: t.id,
    title: t.title,
    point: t.point,
    level: t.level,
    isOptional: t.isOptional,
    index: i + 1,
  })),
);
const mapRef = ref<InstanceType<typeof MapCanvas> | null>(null);
const progress = computed(() => stageProgress(stage.value?.topics ?? []));

onMounted(async () => {
  // `?view=list` trong URL thắng lựa chọn đã nhớ — dùng để gửi link tới đúng
  // chế độ cho người khác, và để test mở thẳng 1 chế độ mà không phải ghi
  // `localStorage` trước.
  view.value = route.query.view === 'list' ? 'list' : route.query.view === 'map' ? 'map' : readView();
  await load();
});

async function load(): Promise<void> {
  busy.value = true;
  try {
    const p = await fetchPath(slug.value);
    if (!p) {
      err.value = 'Không tìm thấy path này.';
      path.value = null;
      return;
    }
    path.value = p;
    err.value = '';
    const saved = readPosition(slug.value, view.value);
    const firstStage = p.stages[0];
    // ƯU TIÊN giữ chặng đang mở. `load()` chạy lại sau MỌI mutation
    // (`mark` / `removeTopic` / `removeStage` / `removeMilestone` / `@done` của
    // 4 form); nếu luôn tính lại từ `localStorage` thì mỗi lần đánh dấu Xong
    // sẽ nhảy về chặng đầu tiên và `watch(stageId)` đóng panel đang mở.
    // Chỉ khi chặng hiện tại KHÔNG còn trong cây mới rơi về `saved`/đầu cây
    // (thẻ xoá mềm ⇒ phải chọn lại chặng hợp lệ, không treo id chết).
    stageId.value =
      (p.stages.some((s) => s.id === stageId.value) ? stageId.value : '') ||
      (saved && p.stages.some((s) => s.id === saved.stageId) ? saved.stageId : '') ||
      firstStage?.id ||
      '';
    if (saved && saved.offset > 0) {
      // `nextTick` để `<MapCanvas>` đã render rồi mới cuộn — set ngay trong
      // `load()` thì `scrollTop` ghi vào phần tử chưa có nội dung.
      requestAnimationFrame(() => mapRef.value?.restore(slug.value, saved.offset));
    }
  } catch (e) {
    err.value = errorMessage(e);
  } finally {
    busy.value = false;
  }
}

function setView(next: ViewMode): void {
  view.value = next;
  writeView(next);
  const q = { ...route.query };
  if (next === 'list') q.view = 'list';
  else delete q.view;
  router.replace({ path: route.path, query: q });
}

function onScrolled(offset: number): void {
  if (!stage.value) return;
  writePosition(slug.value, view.value, { stageId: stage.value.id, offset });
}

async function mark(status: NodeStatus): Promise<void> {
  if (!openTopic.value) return;
  err.value = '';
  try {
    await setTopicStatus(openTopic.value.id, status);
    if (status === 'DONE') celebrate.value += 1;
    await load();
  } catch (e) {
    err.value = errorMessage(e);
  }
}

async function onSwipeDone(): Promise<void> {
  await mark('DONE');
}

async function removeTopic(t: RoadmapTopic): Promise<void> {
  confirming.value = null;
  err.value = '';
  try {
    await deleteTopic(t.id);
    openTopicId.value = null;
    await load();
  } catch (e) {
    err.value = errorMessage(e);
  }
}

async function removeStage(s: RoadmapStage): Promise<void> {
  confirming.value = null;
  err.value = '';
  try {
    await deleteStage(s.id);
    await load();
  } catch (e) {
    err.value = errorMessage(e);
  }
}

async function removeMilestone(m: RoadmapMilestone): Promise<void> {
  confirming.value = null;
  err.value = '';
  try {
    await deleteMilestone(m.id);
    await load();
  } catch (e) {
    err.value = errorMessage(e);
  }
}

watch(stageId, () => {
  openTopicId.value = null;
});

const statusLabel: Record<NodeStatus, string> = {
  NOT_STARTED: 'Chưa học',
  IN_PROGRESS: 'Đang học',
  DONE: 'Xong',
  SKIPPED: 'Bỏ qua',
};
</script>

<template>
  <div>
    <header class="flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0">
        <RouterLink to="/roadmap" class="text-[13px] text-muted hover:underline">← Danh sách path</RouterLink>
        <h2 class="mt-0.5 font-display text-2xl font-semibold text-ink">
          {{ path?.title ?? 'Lộ trình' }}
        </h2>
        <p v-if="path?.overview" class="mt-0.5 max-w-prose text-[14px] text-muted">{{ path.overview }}</p>
      </div>
      <div class="flex shrink-0 gap-2">
        <Button
          size="sm"
          :variant="view === 'map' ? 'default' : 'outline'"
          data-testid="view-map"
          @click="setView('map')"
        >
          Bản đồ
        </Button>
        <Button
          size="sm"
          :variant="view === 'list' ? 'default' : 'outline'"
          data-testid="view-list"
          @click="setView('list')"
        >
          Danh sách
        </Button>
      </div>
    </header>

    <NoticeBar :text="err" />
    <p v-if="busy" class="mt-3 text-muted">đang tải…</p>

    <template v-else-if="path && stages.length > 0">
      <!-- Chặn = 1 bản đồ. `terrain` + `direction` quyết định cả hình lẫn
           chiều vuốt, nên chip chặng mang luôn 2 thông tin đó. -->
      <nav class="mt-4 flex flex-wrap gap-2" aria-label="các chặng">
        <button
          v-for="s in stages"
          :key="s.id"
          type="button"
          class="rounded-full border px-3 py-1 text-[13px] transition-colors"
          :class="
            s.id === stage?.id
              ? 'border-transparent bg-atlas text-paper'
              : 'border-line bg-surface text-ink hover:border-line-strong'
          "
          :data-testid="`stage-chip-${s.slug}`"
          @click="stageId = s.id"
        >
          <span class="tabular mr-1.5">{{ s.position }}</span>{{ s.title }}
        </button>
        <button
          type="button"
          class="rounded-full border border-dashed border-line px-3 py-1 text-[13px] text-muted hover:border-line-strong"
          @click="editingStage = null; stageForm = true"
        >
          + Chặng
        </button>
      </nav>

      <section v-if="stage" class="mt-4">
        <div class="flex flex-wrap items-center gap-2">
          <Badge tone="ember">{{ terrainLabel(stage.terrain as Terrain) }}</Badge>
          <Badge tone="neutral">{{ stage.direction === 'RIGHT' ? 'vuốt ngang' : 'vuốt dọc' }}</Badge>
          <Badge tone="neutral">{{ progress.done }}/{{ progress.total }} màn</Badge>
          <Badge v-if="stage.deck" tone="jade">deck: {{ stage.deck.name }}</Badge>
          <span v-if="stage.durationWeeks > 0" class="tabular text-[13px] text-muted-foreground">
            {{ stage.durationWeeks }} tuần
          </span>
          <div class="ml-auto flex gap-1.5">
            <Button size="sm" variant="ghost" @click="editingStage = stage; stageForm = true">Sửa chặng</Button>
            <Button size="sm" variant="ghost" @click="editingStage = null; topicForm = true">+ Màn</Button>
            <Button size="sm" variant="ghost" @click="editingMilestone = null; milestoneForm = true">
              + Mốc chặng
            </Button>
          </div>
        </div>
        <p v-if="stage.goal" class="mt-1.5 max-w-prose text-[14px] text-muted">{{ stage.goal }}</p>

        <!-- ── chế độ bản đồ ────────────────────────────────────────── -->
        <div v-if="view === 'map'" class="relative mt-3">
          <MapCanvas
            ref="mapRef"
            :slug="slug"
            :terrain="stage.terrain as Terrain"
            :direction="stage.direction"
            :nodes="mapNodes"
            :milestones="stage.milestones"
            :selected-id="openTopicId"
            @select="openTopicId = $event"
            @scrolled="onScrolled"
          />
          <ConfettiCanvas :trigger="celebrate" :seed="stage.position" />
        </div>

        <!-- ── chế độ danh sách (dự phòng) ──────────────────────────── -->
        <div v-else data-testid="list-view" class="mt-3">
          <section v-if="stage.milestones.length > 0" class="mb-3">
            <h4 class="text-[13px] font-semibold uppercase tracking-wide text-muted-foreground">
              Mốc chặng ({{ stage.milestones.length }})
            </h4>
            <ul class="stagger mt-1 space-y-1">
              <li v-for="m in stage.milestones" :key="m.id" :data-testid="`milestone-row-${m.id}`" class="text-[14px]">
                <div class="flex items-center gap-2">
                  <span aria-hidden="true">◆</span>
                  <span>{{ m.text }}</span>
                  <Button size="sm" variant="ghost" @click="editingMilestone = m; milestoneForm = true">Sửa</Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    :data-testid="`ask-delete-milestone-${m.id}`"
                    @click="askDelete('milestone', m.id)"
                  >
                    Xoá
                  </Button>
                </div>
                <div
                  v-if="isConfirming('milestone', m.id)"
                  class="mt-1.5 flex flex-wrap items-center gap-2 rounded-md bg-secondary p-2"
                >
                  <span class="text-[13px]">Xoá mốc “{{ m.text }}”?</span>
                  <Button
                    size="sm"
                    variant="danger"
                    :data-testid="`delete-milestone-${m.id}`"
                    @click="removeMilestone(m)"
                  >
                    Xoá thật
                  </Button>
                  <Button size="sm" variant="ghost" @click="confirming = null">Huỷ</Button>
                </div>
              </li>
            </ul>
          </section>

          <ul class="stagger divide-y divide-[color:var(--color-line-soft)] rounded-card border border-line bg-surface">
            <li
              v-for="t in stage.topics"
              :key="t.id"
              :data-testid="`list-topic-${t.id}`"
              class="px-3 py-2.5"
            >
              <div class="flex flex-wrap items-baseline gap-x-3 gap-y-1">
                <span class="tabular w-7 shrink-0 text-[13px] text-muted-foreground">{{ t.position }}</span>
                <button
                  type="button"
                  class="min-w-0 flex-1 text-left font-medium text-ink hover:underline"
                  @click="openTopicId = t.id"
                >
                  {{ t.title }}
                </button>
                <Badge :tone="t.status === 'DONE' || t.status === 'SKIPPED' ? 'jade' : t.level === 'CURRENT' ? 'ember' : 'locked'">
                  {{ statusLabel[t.status] }}
                </Badge>
                <Badge v-if="t.isOptional" tone="neutral">tham khảo</Badge>
                <span class="tabular text-[13px] text-muted-foreground">{{ t.resources.length }} tài liệu</span>
                <Button size="sm" variant="ghost" @click="editingTopic = t; topicForm = true">Sửa</Button>
                <Button
                  size="sm"
                  variant="ghost"
                  :data-testid="`ask-delete-topic-${t.id}`"
                  @click="askDelete('topic', t.id)"
                >
                  Xoá
                </Button>
              </div>
              <div
                v-if="isConfirming('topic', t.id)"
                class="mt-1.5 flex flex-wrap items-center gap-2 rounded-md bg-secondary p-2"
              >
                <span class="text-[13px]">Xoá màn “{{ t.title }}” cùng tài liệu bên trong?</span>
                <Button
                  size="sm"
                  variant="danger"
                  :data-testid="`delete-topic-${t.id}`"
                  @click="removeTopic(t)"
                >
                  Xoá thật
                </Button>
                <Button size="sm" variant="ghost" @click="confirming = null">Huỷ</Button>
              </div>
            </li>
          </ul>
        </div>
      </section>

      <!-- Panel chi tiết: đặt NGOÀI khung bản đồ để vuốt lên trong panel
           không bao giờ bị `MapCanvas` nhận nhầm. -->
      <LevelPanel
        v-if="openTopic && stage"
        class="mt-4"
        :topic="openTopic"
        :deck-id="stage.deckId"
        @close="openTopicId = null"
        @mark="mark"
        @swipe-done="onSwipeDone"
        @open-resource="editingResource = $event; resourceForm = true"
        @edit="editingTopic = $event; topicForm = true"
      />

      <div v-if="stage" class="mt-4 border-t border-border pt-3">
        <div class="flex flex-wrap gap-2">
          <Button
            size="sm"
            variant="danger"
            :data-testid="`ask-delete-stage-${stage.id}`"
            @click="askDelete('stage', stage.id)"
          >
            Xoá chặng này
          </Button>
        </div>
        <div
          v-if="isConfirming('stage', stage.id)"
          class="mt-2 flex flex-wrap items-center gap-2 rounded-md bg-secondary p-2.5"
        >
          <span class="text-[13px]">
            Xoá “{{ stage.title }}” cùng toàn bộ màn, mốc chặng và tài liệu bên trong?
          </span>
          <Button
            size="sm"
            variant="danger"
            :data-testid="`delete-stage-${stage.id}`"
            @click="removeStage(stage)"
          >
            Xoá thật
          </Button>
          <Button size="sm" variant="ghost" @click="confirming = null">Huỷ</Button>
        </div>
      </div>

      <StageForm
        :open="stageForm"
        :path-slug="slug"
        :stage="editingStage"
        @update:open="stageForm = $event"
        @done="load"
      />
      <TopicForm
        :open="topicForm"
        :stage="stage!"
        :topic="editingTopic"
        @update:open="topicForm = $event"
        @done="load"
      />
      <ResourceForm
        :open="resourceForm"
        :topic-id="editingResource?.topicId ?? openTopic?.id ?? ''"
        :resource="editingResource"
        @update:open="resourceForm = $event"
        @done="load"
      />
      <MilestoneForm
        :open="milestoneForm"
        :stage-id="stage?.id ?? ''"
        :milestone="editingMilestone"
        @update:open="milestoneForm = $event"
        @done="load"
      />
    </template>

    <p v-else-if="!busy" class="mt-3 text-muted">Path này chưa có chặng nào.</p>
  </div>
</template>
