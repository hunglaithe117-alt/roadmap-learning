<script setup lang="ts">
/**
 * `/bookmarks` — kho link độc lập, KHÔNG nằm trong cây roadmap (M6a §5.6).
 *
 * Lọc `status` + `tag` gửi thẳng xuống server. Lọc tag khớp theo PHẦN TỬ
 * CSV, không phải chuỗi con: lọc "a" không ra link mang tag "ab". Ô tag là 1
 * `<select>` lấy từ chính danh sách đang hiện chứ không phải ô gõ tự do —
 * gõ "a" vào ô tự do sẽ lọc theo từng ký tự và nhảy kết quả liên tục.
 */
import { computed, onMounted, ref } from 'vue';
import NoticeBar from '../components/NoticeBar.vue';
import Badge from '../components/ui/Badge.vue';
import Button from '../components/ui/Button.vue';
import Select from '../components/ui/Select.vue';
import BookmarkForm from '../bookmarks/BookmarkForm.vue';
import {
  deleteBookmark,
  fetchBookmarks,
  setBookmarkStatus,
  type Bookmark,
  type BookmarkStatus,
} from '../bookmarks/api';
import { errorMessage } from '../graphql/errors';

const STATUSES: Array<{ value: BookmarkStatus; label: string }> = [
  { value: 'TO_READ', label: 'Chưa đọc' },
  { value: 'READING', label: 'Đang đọc' },
  { value: 'DONE', label: 'Xong' },
  { value: 'ARCHIVED', label: 'Cất đi' },
];

/** Màu badge theo trạng thái — 1 bảng ở đây thay vì điều kiện rải ở template. */
const STATUS_TONE: Record<BookmarkStatus, 'neutral' | 'amber' | 'jade' | 'locked'> = {
  TO_READ: 'neutral',
  READING: 'amber',
  DONE: 'jade',
  ARCHIVED: 'locked',
};

const items = ref<Bookmark[]>([]);
const err = ref('');
const busy = ref(true);
const status = ref<BookmarkStatus | ''>('');
const tag = ref('');
const formOpen = ref(false);
const editing = ref<Bookmark | null>(null);
const confirming = ref('');

onMounted(load);

/** Mọi tag đang có trong kết quả chưa lọc — nguồn của dropdown. */
const knownTags = ref<string[]>([]);

async function load(): Promise<void> {
  busy.value = true;
  try {
    items.value = await fetchBookmarks({
      status: status.value === '' ? null : status.value,
      tag: tag.value === '' ? null : tag.value,
    });
    err.value = '';
  } catch (e) {
    err.value = errorMessage(e);
  } finally {
    busy.value = false;
  }
}

/** Tag lấy từ 1 lần đọc không lọc, nên đổi bộ lọc không làm mất danh sách tag. */
async function loadTags(): Promise<void> {
  try {
    const all = await fetchBookmarks();
    const set = new Set<string>();
    for (const b of all) for (const t of b.tagList) set.add(t);
    knownTags.value = [...set].sort();
  } catch {
    knownTags.value = [];
  }
}

onMounted(loadTags);

function onFilterChange(): void {
  void load();
}

const counts = computed(() => {
  const by: Record<string, number> = {};
  for (const b of items.value) by[b.status] = (by[b.status] ?? 0) + 1;
  return by;
});

function statusLabel(s: BookmarkStatus): string {
  return STATUSES.find((x) => x.value === s)?.label ?? s;
}

async function changeStatus(b: Bookmark, next: BookmarkStatus): Promise<void> {
  err.value = '';
  try {
    await setBookmarkStatus(b.id, next);
    await load();
  } catch (e) {
    err.value = errorMessage(e);
  }
}

async function remove(b: Bookmark): Promise<void> {
  err.value = '';
  try {
    await deleteBookmark(b.id);
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
        <h2 class="font-display text-2xl font-semibold text-ink">Kho link</h2>
        <p class="mt-0.5 text-[14px] text-muted">
          Trang để đọc sau. Không nằm trong lộ trình.
        </p>
      </div>
      <Button @click="editing = null; formOpen = true">Thêm link</Button>
    </header>

    <section class="mt-4 flex flex-wrap items-end gap-2">
      <label class="inline-flex flex-col gap-1 text-[13px] font-semibold text-ink">
        Trạng thái
        <Select v-model="status" class="w-40" aria-label="lọc theo trạng thái" @change="onFilterChange">
          <option value="">Tất cả</option>
          <option v-for="s in STATUSES" :key="s.value" :value="s.value">{{ s.label }}</option>
        </Select>
      </label>
      <label class="inline-flex flex-col gap-1 text-[13px] font-semibold text-ink">
        Tag
        <Select v-model="tag" class="w-44" aria-label="lọc theo tag" @change="onFilterChange">
          <option value="">Tất cả</option>
          <option v-for="t in knownTags" :key="t" :value="t">{{ t }}</option>
        </Select>
      </label>
      <Button
        v-if="status !== '' || tag !== ''"
        size="sm"
        variant="ghost"
        @click="status = ''; tag = ''; onFilterChange()"
      >
        Bỏ lọc
      </Button>
    </section>

    <NoticeBar class="mt-3" :text="err" />
    <p v-if="busy" class="mt-3 text-muted">đang tải…</p>
    <p v-else-if="items.length === 0" class="mt-3 text-muted">
      Chưa có link nào khớp bộ lọc.
    </p>

    <p v-else class="tabular mt-3 text-[13px] text-muted-foreground">
      {{ items.length }} link ·
      <template v-for="s in STATUSES" :key="s.value">
        <template v-if="counts[s.value]"> {{ s.label }} {{ counts[s.value] }} ·</template>
      </template>
    </p>

    <ul v-if="!busy && items.length > 0" class="stagger mt-3 grid gap-2">
      <li
        v-for="b in items"
        :key="b.id"
        :data-testid="`bookmark-row-${b.id}`"
        class="rounded-card border border-line bg-surface p-3 shadow-plate"
      >
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-2">
              <a
                v-if="b.url"
                :href="b.url"
                target="_blank"
                rel="noopener noreferrer"
                class="font-medium text-ink underline decoration-dotted hover:decoration-solid"
                >{{ b.title }}</a
              >
              <span v-else class="font-medium text-ink">{{ b.title }}</span>
              <Badge v-if="!b.url" tone="locked">không có link</Badge>
              <Badge v-for="t in b.tagList" :key="t" tone="neutral">{{ t }}</Badge>
              <Badge :tone="STATUS_TONE[b.status]">{{ statusLabel(b.status) }}</Badge>
            </div>
            <p v-if="b.url" class="mt-0.5 truncate text-[13px] text-muted-foreground">{{ b.url }}</p>
            <p v-if="b.note" class="mt-1 text-[14px] text-muted">{{ b.note }}</p>
          </div>
          <div class="flex shrink-0 items-center gap-1.5">
            <Select
              class="w-32"
              :model-value="b.status"
              :aria-label="`đổi trạng thái ${b.title}`"
              @update:model-value="changeStatus(b, $event as BookmarkStatus)"
            >
              <option v-for="s in STATUSES" :key="s.value" :value="s.value">{{ s.label }}</option>
            </Select>
            <Button size="sm" variant="ghost" @click="editing = b; formOpen = true">Sửa</Button>
            <Button size="sm" variant="danger" @click="confirming = b.id">Xoá</Button>
          </div>
        </div>
        <div
          v-if="confirming === b.id"
          class="mt-2 flex flex-wrap items-center gap-2 rounded-md bg-secondary p-2"
        >
          <span class="text-[13px]">Xoá “{{ b.title }}”? Link sẽ mất hẳn sau khi đồng bộ.</span>
          <Button size="sm" variant="danger" @click="remove(b)">Xoá thật</Button>
          <Button size="sm" variant="ghost" @click="confirming = ''">Huỷ</Button>
        </div>
      </li>
    </ul>

    <BookmarkForm
      :open="formOpen"
      :bookmark="editing"
      @update:open="formOpen = $event"
      @done="load(); loadTags()"
    />
  </div>
</template>
