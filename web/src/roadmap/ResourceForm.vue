<script setup lang="ts">
/**
 * Form tài liệu — dùng chung cho thêm và sửa. Khác `BookmarkForm` ở 1 điểm:
 * `url` NULL và `url` rỗng là 2 thứ khác nhau. Server (`ResourcePatch.clearUrl`)
 * cũng phân biệt vậy: gửi `url: ""` ghi chuỗi rỗng, xoá link phải gửi
 * `clearUrl: true`. Ô nhập vì thế luôn hiện, kèm 1 checkbox "xoá link" chỉ bật
 * được khi đang sửa — người dùng thấy link biến mất thay vì tự đoán.
 */
import { computed, ref, watch } from 'vue';
import Button from '../components/ui/Button.vue';
import Dialog from '../components/ui/Dialog.vue';
import Field from '../components/ui/Field.vue';
import Input from '../components/ui/Input.vue';
import Select from '../components/ui/Select.vue';
import Textarea from '../components/ui/Textarea.vue';
import {
  createResource,
  deleteResource,
  updateResource,
  type NewResource,
  type ResourcePatch,
  type RoadmapResource,
} from './api';
import type { ResourceKind } from '../graphql/operations';
import { errorMessage } from '../graphql/errors';

const props = defineProps<{ open: boolean; topicId: string; resource: RoadmapResource | null }>();
const emit = defineEmits<{ 'update:open': [boolean]; done: [] }>();

const KINDS: Array<{ value: ResourceKind; label: string }> = [
  { value: 'VIDEO', label: 'Video' },
  { value: 'ARTICLE', label: 'Bài đọc' },
  { value: 'TOOL', label: 'Công cụ' },
  { value: 'APP', label: 'Ứng dụng' },
  { value: 'BOOK', label: 'Sách' },
  { value: 'COURSE', label: 'Khoá học' },
  { value: 'SITE', label: 'Website' },
  { value: 'PODCAST', label: 'Podcast' },
  { value: 'CHANNEL', label: 'Kênh' },
];

const title = ref('');
const url = ref('');
const kind = ref<ResourceKind>('ARTICLE');
const note = ref('');
const clearUrl = ref(false);
const err = ref('');
const busy = ref(false);

const editing = computed(() => props.resource !== null);

watch(
  () => [props.open, props.resource] as const,
  () => {
    if (!props.open) return;
    title.value = props.resource?.title ?? '';
    url.value = props.resource?.url ?? '';
    kind.value = props.resource?.kind ?? 'ARTICLE';
    note.value = props.resource?.note ?? '';
    clearUrl.value = false;
    err.value = '';
  },
  { immediate: true },
);

async function save(): Promise<void> {
  if (!title.value.trim()) {
    err.value = 'Cần có tiêu đề tài liệu.';
    return;
  }
  busy.value = true;
  err.value = '';
  try {
    if (props.resource) {
      const patch: ResourcePatch = { title: title.value.trim(), kind: kind.value, note: note.value.trim() };
      if (clearUrl.value) patch.clearUrl = true;
      else if (url.value.trim()) patch.url = url.value.trim();
      await updateResource(props.resource.id, patch);
    } else {
      const input: NewResource = {
        title: title.value.trim(),
        kind: kind.value,
        note: note.value.trim(),
        url: url.value.trim() || null,
      };
      await createResource(props.topicId, input);
    }
    emit('done');
    emit('update:open', false);
  } catch (e) {
    err.value = errorMessage(e);
  } finally {
    busy.value = false;
  }
}

// Xoá đặt ở đây vì danh sách tài liệu chỉ hiện trong `LevelPanel` — không có
// hàng nào trong list view để đặt nút xoá cạnh, và tách nút ra panel thành
// emit thứ 5 chỉ để mang 1 hàm là vòng vèo.
async function remove(): Promise<void> {
  if (!props.resource) return;
  busy.value = true;
  err.value = '';
  try {
    await deleteResource(props.resource.id);
    emit('done');
    emit('update:open', false);
  } catch (e) {
    err.value = errorMessage(e);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <Dialog
    :open="open"
    :title="editing ? 'Sửa tài liệu' : 'Thêm tài liệu'"
    @update:open="emit('update:open', $event)"
  >
    <form class="grid gap-4 sm:grid-cols-2" @submit.prevent="save">
      <div class="sm:col-span-2">
        <Field label="Tiêu đề" for="res-title" required>
          <Input id="res-title" v-model="title" placeholder="Tên tài liệu" />
        </Field>
      </div>
      <Field label="Link" for="res-url" hint="http hoặc https. Bỏ trống = tài liệu không có link.">
        <Input id="res-url" v-model="url" placeholder="https://…" :disabled="clearUrl" />
        <label v-if="editing" class="mt-1 inline-flex items-center gap-1.5 text-[13px]">
          <input v-model="clearUrl" type="checkbox" :disabled="url.trim() !== ''" />
          Xoá link đã lưu
        </label>
      </Field>
      <Field label="Loại" for="res-kind">
        <Select id="res-kind" v-model="kind">
          <option v-for="k in KINDS" :key="k.value" :value="k.value">{{ k.label }}</option>
        </Select>
      </Field>
      <div class="sm:col-span-2">
        <Field label="Ghi chú" for="res-note">
          <Textarea id="res-note" v-model="note" rows="2" placeholder="Ghi chú ngắn" />
        </Field>
      </div>
      <p v-if="err" class="sm:col-span-2 text-[13px] text-destructive" role="alert">{{ err }}</p>
      <div class="sm:col-span-2 flex items-center justify-between gap-2">
        <Button v-if="editing" type="button" variant="danger" :disabled="busy" @click="remove">
          Xoá tài liệu
        </Button>
        <div class="ml-auto flex gap-2">
          <Button type="button" variant="outline" @click="emit('update:open', false)">Huỷ</Button>
          <Button type="submit" :disabled="busy">{{ busy ? 'Đang lưu…' : 'Lưu' }}</Button>
        </div>
      </div>
    </form>
  </Dialog>
</template>
