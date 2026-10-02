<script setup lang="ts">
/**
 * Form màn (topic) — thêm và sửa. Giữ `isOptional` dạng checkbox vì schema nhận
 * `Int` 0/1 chứ không phải Boolean (xem `TopicInput.isOptional`), và giữ luật
 * "node tham khảo không nằm trong mẫu số phần trăm" hiện ngay dưới ô.
 *
 * `mapX`/`mapY` để trống = để server layout. Ghi tay toạ độ thì phải nhập CẢ
 * hai: một node chỉ có `x` là node treo lơ lửng ngoài đường đi, mà người dùng
 * không có cách kéo lại (form này không có drag-drop — xem handoff M6b).
 */
import { computed, ref, watch } from 'vue';
import Button from '../components/ui/Button.vue';
import Dialog from '../components/ui/Dialog.vue';
import Field from '../components/ui/Field.vue';
import Input from '../components/ui/Input.vue';
import Textarea from '../components/ui/Textarea.vue';
import { createTopic, updateTopic, type NewTopic, type RoadmapStage, type RoadmapTopic, type TopicPatch } from './api';
import { errorMessage } from '../graphql/errors';

const props = defineProps<{ open: boolean; stage: RoadmapStage; topic: RoadmapTopic | null }>();
const emit = defineEmits<{ 'update:open': [boolean]; done: [] }>();

const title = ref('');
const why = ref('');
const activities = ref('');
const isOptional = ref(false);
const mapX = ref('');
const mapY = ref('');
const err = ref('');
const busy = ref(false);

const editing = computed(() => props.topic !== null);

watch(
  () => [props.open, props.topic] as const,
  () => {
    if (!props.open) return;
    title.value = props.topic?.title ?? '';
    why.value = props.topic?.why ?? '';
    activities.value = (props.topic?.activityList ?? []).join('\n');
    isOptional.value = props.topic?.isOptional ?? false;
    mapX.value = props.topic?.mapX == null ? '' : String(props.topic.mapX);
    mapY.value = props.topic?.mapY == null ? '' : String(props.topic.mapY);
    err.value = '';
  },
  { immediate: true },
);

async function save(): Promise<void> {
  if (!title.value.trim()) {
    err.value = 'Cần có tiêu đề màn.';
    return;
  }
  const hasX = mapX.value.trim() !== '';
  const hasY = mapY.value.trim() !== '';
  if (hasX !== hasY) {
    err.value = 'Đặt toạ độ thì nhập cả x và y, hoặc để trống cả hai để bản đồ tự xếp.';
    return;
  }
  busy.value = true;
  err.value = '';
  try {
    const lines = activities.value
      .split('\n')
      .map((l) => l.trim())
      .filter(Boolean);
    if (props.topic) {
      const patch: TopicPatch = {
        title: title.value.trim(),
        why: why.value.trim(),
        activities: lines,
        isOptional: isOptional.value ? 1 : 0,
      };
      if (hasX) {
        patch.mapX = Number(mapX.value);
        patch.mapY = Number(mapY.value);
      } else if (props.topic.mapPinned) {
        // Đã có toạ độ đặt tay mà người dùng xoá cả hai ô ⇒ chủ động đưa node
        // về auto-layout. `clearMap` thắng mọi giá trị `mapX/mapY` cùng patch.
        patch.clearMap = true;
      }
      await updateTopic(props.topic.id, patch);
    } else {
      const input: NewTopic = {
        title: title.value.trim(),
        why: why.value.trim(),
        activities: lines,
        isOptional: isOptional.value ? 1 : 0,
      };
      if (hasX) {
        input.mapX = Number(mapX.value);
        input.mapY = Number(mapY.value);
      }
      await createTopic(props.stage.id, input);
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
    :title="editing ? 'Sửa màn' : 'Thêm màn'"
    :description="editing ? undefined : 'Màn mới thêm vào cuối chặng, bản đồ tự xếp lại.'"
    @update:open="emit('update:open', $event)"
  >
    <form class="grid gap-4 sm:grid-cols-2" @submit.prevent="save">
      <div class="sm:col-span-2">
        <Field label="Tiêu đề" for="topic-title" required>
          <Input id="topic-title" v-model="title" placeholder="Tên màn" />
        </Field>
      </div>
      <div class="sm:col-span-2">
        <Field label="Vì sao học màn này" for="topic-why">
          <Textarea id="topic-why" v-model="why" rows="2" />
        </Field>
      </div>
      <div class="sm:col-span-2">
        <Field label="Hoạt động" for="topic-acts" hint="Mỗi dòng là 1 hoạt động.">
          <Textarea id="topic-acts" v-model="activities" rows="4" placeholder="Nghe 10 câu&#10;Điền âm điệu" />
        </Field>
      </div>
      <div class="sm:col-span-2">
        <label class="inline-flex items-center gap-2 text-[14px]">
          <input v-model="isOptional" type="checkbox" />
          Màn tham khảo (không tính vào phần trăm)
        </label>
      </div>
      <Field label="Toạ độ x" for="topic-x" hint="Để trống cả x và y để bản đồ tự xếp.">
        <Input id="topic-x" v-model="mapX" inputmode="decimal" placeholder="tự xếp" />
      </Field>
      <Field label="Toạ độ y" for="topic-y">
        <Input id="topic-y" v-model="mapY" inputmode="decimal" placeholder="tự xếp" />
      </Field>
      <p v-if="err" class="sm:col-span-2 text-[13px] text-destructive" role="alert">{{ err }}</p>
      <div class="sm:col-span-2 flex justify-end gap-2">
        <Button type="button" variant="outline" @click="emit('update:open', false)">Huỷ</Button>
        <Button type="submit" :disabled="busy">{{ busy ? 'Đang lưu…' : 'Lưu' }}</Button>
      </div>
    </form>
  </Dialog>
</template>
