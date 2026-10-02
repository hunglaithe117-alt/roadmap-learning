<script setup lang="ts">
/**
 * Form path. `isBuiltin` = seed tích hợp ⇒ server từ chối sửa/xoá; form cho sửa
 * tiêu đề/mô tả (đọc để hiểu) nhưng KHÔNG cho sửa slug, vì slug là natural key
 * của seed — đổi nó làm seed lần boot sau tạo path trùng.
 */
import { computed, ref, watch } from 'vue';
import Button from '../components/ui/Button.vue';
import Dialog from '../components/ui/Dialog.vue';
import Field from '../components/ui/Field.vue';
import Input from '../components/ui/Input.vue';
import Textarea from '../components/ui/Textarea.vue';
import { createPath, updatePath, type NewPath, type RoadmapPath } from './api';
import { errorMessage } from '../graphql/errors';

const props = defineProps<{ open: boolean; path: RoadmapPath | null }>();
const emit = defineEmits<{ 'update:open': [boolean]; done: [] }>();

const slug = ref('');
const title = ref('');
const overview = ref('');
const language = ref('zh');
const err = ref('');
const busy = ref(false);

const editing = computed(() => props.path !== null);

watch(
  () => [props.open, props.path] as const,
  () => {
    if (!props.open) return;
    slug.value = props.path?.slug ?? '';
    title.value = props.path?.title ?? '';
    overview.value = props.path?.overview ?? '';
    language.value = props.path?.language || 'zh';
    err.value = '';
  },
  { immediate: true },
);

async function save(): Promise<void> {
  if (!title.value.trim()) {
    err.value = 'Cần có tiêu đề path.';
    return;
  }
  if (!editing.value && !/^[a-z0-9-]{1,64}$/.test(slug.value.trim())) {
    err.value = 'Slug chỉ gồm chữ thường, số và dấu gạch ngang.';
    return;
  }
  busy.value = true;
  err.value = '';
  try {
    if (props.path) {
      await updatePath(props.path.slug, {
        title: title.value.trim(),
        overview: overview.value.trim(),
        language: language.value.trim(),
      });
    } else {
      const input: NewPath = {
        slug: slug.value.trim(),
        title: title.value.trim(),
        overview: overview.value.trim(),
        language: language.value.trim(),
      };
      await createPath(input);
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
  <Dialog :open="open" :title="editing ? 'Sửa path' : 'Thêm path'" @update:open="emit('update:open', $event)">
    <form class="grid gap-4 sm:grid-cols-2" @submit.prevent="save">
      <Field v-if="!editing" label="Slug" for="path-slug" required hint="chữ thường, số, dấu gạch ngang.">
        <Input id="path-slug" v-model="slug" placeholder="vi-beginner" />
      </Field>
      <Field label="Tiêu đề" for="path-title" required>
        <Input id="path-title" v-model="title" />
      </Field>
      <div class="sm:col-span-2">
        <Field label="Mô tả" for="path-overview">
          <Textarea id="path-overview" v-model="overview" rows="3" />
        </Field>
      </div>
      <Field label="Ngôn ngữ" for="path-lang" hint="Tối đa 16 ký tự. Trống = zh.">
        <Input id="path-lang" v-model="language" maxlength="16" />
      </Field>
      <p v-if="editing && path?.isBuiltin" class="self-end text-[13px] text-muted-foreground">
        Path có sẵn: không sửa được slug và không xoá được.
      </p>
      <p v-if="err" class="sm:col-span-2 text-[13px] text-destructive" role="alert">{{ err }}</p>
      <div class="sm:col-span-2 flex justify-end gap-2">
        <Button type="button" variant="outline" @click="emit('update:open', false)">Huỷ</Button>
        <Button type="submit" :disabled="busy">{{ busy ? 'Đang lưu…' : 'Lưu' }}</Button>
      </div>
    </form>
  </Dialog>
</template>
