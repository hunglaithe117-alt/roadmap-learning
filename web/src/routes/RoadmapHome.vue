<script setup lang="ts">
/**
 * `/roadmap` — danh sách path. Mỗi dòng là 1 lá bài: số chữ to, tiêu đề, mô tả,
 * vòng tiến độ, và con số "x/y màn". Bấm vào để vào bản đồ.
 *
 * `isBuiltin` = seed tích hợp: hiện nút "Sửa" (đọc tiêu đề/mô tả) nhưng KHÔNG
 * hiện nút xoá — server từ chối và nút chết chỉ làm người dùng bấm rồi thấy
 * lỗi.
 */
import { onMounted, ref } from 'vue';
import NoticeBar from '../components/NoticeBar.vue';
import Badge from '../components/ui/Badge.vue';
import Button from '../components/ui/Button.vue';
import ProgressRing from '../roadmap/map/ProgressRing.vue';
import PathForm from '../roadmap/PathForm.vue';
import { deletePath, fetchPathRows, type PathRow, type RoadmapPath } from '../roadmap/api';
import { errorMessage } from '../graphql/errors';

const rows = ref<PathRow[]>([]);
const err = ref('');
const busy = ref(true);
const formOpen = ref(false);
const editing = ref<RoadmapPath | null>(null);
const confirmDelete = ref('');

onMounted(load);

async function load(): Promise<void> {
  busy.value = true;
  try {
    rows.value = await fetchPathRows();
    err.value = '';
  } catch (e) {
    err.value = errorMessage(e);
  } finally {
    busy.value = false;
  }
}

function openCreate(): void {
  editing.value = null;
  formOpen.value = true;
}

function openEdit(path: RoadmapPath): void {
  editing.value = path;
  formOpen.value = true;
}

async function remove(path: RoadmapPath): Promise<void> {
  err.value = '';
  try {
    await deletePath(path.slug);
    await load();
  } catch (e) {
    err.value = errorMessage(e);
  }
}
</script>

<template>
  <div>
    <header class="flex flex-wrap items-end justify-between gap-3">
      <div>
        <h2 class="font-display text-2xl font-semibold text-ink">Lộ trình</h2>
        <p class="mt-0.5 text-[14px] text-muted">
          Mỗi path là 1 bản đồ. Bấm vào để mở.
        </p>
      </div>
      <Button @click="openCreate">Thêm path</Button>
    </header>

    <NoticeBar :text="err" />
    <p v-if="busy" class="mt-3 text-muted">đang tải…</p>
    <p v-else-if="rows.length === 0" class="mt-3 text-muted">Chưa có path nào.</p>

    <ul v-else class="stagger mt-4 grid gap-3">
      <li
        v-for="r in rows"
        :key="r.path.id"
        :data-testid="`path-row-${r.path.slug}`"
        class="rounded-card border border-line bg-surface p-4 shadow-plate transition-shadow hover:shadow-lift"
      >
        <div class="flex items-start gap-4">
          <ProgressRing :percent="r.summary.percent" />
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-2">
              <RouterLink
                class="font-display text-lg font-semibold text-ink hover:underline"
                :to="`/roadmap/${r.path.slug}`"
              >
                {{ r.path.title }}
              </RouterLink>
              <Badge tone="neutral">{{ r.path.language || 'zh' }}</Badge>
              <Badge v-if="r.path.isBuiltin" tone="amber">có sẵn</Badge>
            </div>
            <p v-if="r.path.overview" class="mt-1 text-[14px] text-muted">{{ r.path.overview }}</p>
            <p class="tabular mt-1.5 text-[13px] text-muted-foreground">
              {{ r.summary.topicsDone }}/{{ r.summary.topicsRequired }} màn · {{ r.summary.stages }} chặng
              <template v-if="r.summary.topicsInProgress > 0">
                · {{ r.summary.topicsInProgress }} đang học
              </template>
            </p>
          </div>
          <div class="flex shrink-0 flex-col gap-1.5">
            <Button size="sm" variant="outline" @click="openEdit(r.path)">Sửa</Button>
            <Button
              v-if="!r.path.isBuiltin"
              size="sm"
              variant="danger"
              @click="confirmDelete = confirmDelete === r.path.slug ? '' : r.path.slug"
            >
              Xoá
            </Button>
          </div>
        </div>

        <div v-if="confirmDelete === r.path.slug" class="mt-3 flex flex-wrap items-center gap-2 rounded-md bg-secondary p-2.5">
          <span class="text-[13px]">
            Xoá “{{ r.path.title }}” cùng toàn bộ chặng, màn và tài liệu bên trong?
          </span>
          <Button size="sm" variant="danger" @click="remove(r.path)">Xoá thật</Button>
          <Button size="sm" variant="ghost" @click="confirmDelete = ''">Huỷ</Button>
        </div>
      </li>
    </ul>

    <PathForm :open="formOpen" :path="editing" @update:open="formOpen = $event" @done="load" />
  </div>
</template>
