<script setup lang="ts">
/**
 * Form kho link. Tách `url` rỗng (NULL) và `url` có: server phân biệt bằng
 * `clearUrl` vì `BookmarkPatch.url: String` không phân biệt được "không gửi" với
 * "gửi rỗng". Ô xoá link chỉ bật khi đang sửa — người dùng thấy link biến mất
 * thay vì tự đoán.
 *
 * Tag nhập tay cách nhau bởi dấu phẩy, chuẩn hoá lúc gửi (`normalizeTags`) vì
 * server đếm độ dài TRƯỚC khi chuẩn hoá — " HSK3 " sẽ bị đếm 6 ký tự.
 */
import { computed, ref, watch } from 'vue';
import Button from '../components/ui/Button.vue';
import Dialog from '../components/ui/Dialog.vue';
import Field from '../components/ui/Field.vue';
import Input from '../components/ui/Input.vue';
import Select from '../components/ui/Select.vue';
import Textarea from '../components/ui/Textarea.vue';
import {
  createBookmark,
  updateBookmark,
  type Bookmark,
  type BookmarkPatch,
  type BookmarkStatus,
} from './api';
import { errorMessage } from '../graphql/errors';

const props = defineProps<{ open: boolean; bookmark: Bookmark | null }>();
const emit = defineEmits<{ 'update:open': [boolean]; done: [] }>();

/** Nhãn tiếng Việt + 4 giá trị `BookmarkStatus` — KHÁC `Status` của node map. */
const STATUSES: Array<{ value: BookmarkStatus; label: string }> = [
  { value: 'TO_READ', label: 'Chưa đọc' },
  { value: 'READING', label: 'Đang đọc' },
  { value: 'DONE', label: 'Xong' },
  { value: 'ARCHIVED', label: 'Cất đi' },
];

const title = ref('');
const url = ref('');
const note = ref('');
const tags = ref('');
const status = ref<BookmarkStatus>('TO_READ');
const clearUrl = ref(false);
const err = ref('');
const busy = ref(false);

const editing = computed(() => props.bookmark !== null);

watch(
  () => [props.open, props.bookmark] as const,
  () => {
    if (!props.open) return;
    title.value = props.bookmark?.title ?? '';
    url.value = props.bookmark?.url ?? '';
    note.value = props.bookmark?.note ?? '';
    tags.value = (props.bookmark?.tagList ?? []).join(', ');
    status.value = props.bookmark?.status ?? 'TO_READ';
    clearUrl.value = false;
    err.value = '';
  },
  { immediate: true },
);

async function save(): Promise<void> {
  if (!title.value.trim()) {
    err.value = 'Cần có tiêu đề.';
    return;
  }
  busy.value = true;
  err.value = '';
  try {
    const tagList = tags.value
      .split(',')
      .map((t) => t.trim())
      .filter(Boolean);
    if (props.bookmark) {
      const patch: BookmarkPatch = {
        title: title.value.trim(),
        note: note.value.trim(),
        // Luôn gửi `tags`: `[]` = xoá hết, không gửi = giữ nguyên. Ô tag luôn
        // hiện nên "không gửi" không bao giờ là điều người dùng ngầm ý.
        tags: tagList,
      };
      if (clearUrl.value) patch.clearUrl = true;
      else if (url.value.trim()) patch.url = url.value.trim();
      await updateBookmark(props.bookmark.id, patch);
    } else {
      await createBookmark({
        title: title.value.trim(),
        url: url.value.trim() || null,
        note: note.value.trim(),
        tags: tagList,
        status: status.value,
      });
    }
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
    :title="editing ? 'Sửa link' : 'Thêm link'"
    @update:open="emit('update:open', $event)"
  >
    <form class="grid gap-4 sm:grid-cols-2" @submit.prevent="save">
      <div class="sm:col-span-2">
        <Field label="Tiêu đề" for="bm-title" required>
          <Input id="bm-title" v-model="title" placeholder="Tên trang" />
        </Field>
      </div>
      <div class="sm:col-span-2">
        <Field label="Link" for="bm-url" hint="http hoặc https. Bỏ trống = ghi chú không cần link.">
          <Input id="bm-url" v-model="url" placeholder="https://…" :disabled="clearUrl" />
          <label v-if="editing" class="mt-1 inline-flex items-center gap-1.5 text-[13px]">
            <input v-model="clearUrl" type="checkbox" :disabled="url.trim() !== ''" />
            Xoá link đã lưu
          </label>
        </Field>
      </div>
      <Field label="Trạng thái" for="bm-status" hint="Trạng thái của kho link, không phải của node bản đồ.">
        <Select id="bm-status" v-model="status" :disabled="editing">
          <option v-for="s in STATUSES" :key="s.value" :value="s.value">{{ s.label }}</option>
        </Select>
        <p v-if="editing" class="text-[13px] text-muted-foreground">
          Đổi trạng thái bằng nút ở cột trạng thái, không sửa ở đây.
        </p>
      </Field>
      <Field label="Tag" for="bm-tags" hint="Cách nhau bởi dấu phẩy. Tối đa 10 tag.">
        <Input id="bm-tags" v-model="tags" placeholder="hsk3, ngữ pháp" />
      </Field>
      <div class="sm:col-span-2">
        <Field label="Ghi chú" for="bm-note">
          <Textarea id="bm-note" v-model="note" rows="3" />
        </Field>
      </div>
      <p v-if="err" class="sm:col-span-2 text-[13px] text-destructive" role="alert">{{ err }}</p>
      <div class="sm:col-span-2 flex justify-end gap-2">
        <Button type="button" variant="outline" @click="emit('update:open', false)">Huỷ</Button>
        <Button type="submit" :disabled="busy">{{ busy ? 'Đang lưu…' : 'Lưu' }}</Button>
      </div>
    </form>
  </Dialog>
</template>
