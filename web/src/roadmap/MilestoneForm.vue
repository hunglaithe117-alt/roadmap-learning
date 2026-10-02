<script setup lang="ts">
/**
 * Form mốc chặng (milestone). Mốc chặng không có `url`, không có `status` — nó là
 * 1 dòng chữ trên đường đi, nên form chỉ có 2 ô thật.
 */
import { ref, watch } from 'vue';
import Button from '../components/ui/Button.vue';
import Dialog from '../components/ui/Dialog.vue';
import Field from '../components/ui/Field.vue';
import Input from '../components/ui/Input.vue';
import { createMilestone, updateMilestone, type RoadmapMilestone } from './api';
import { errorMessage } from '../graphql/errors';

const props = defineProps<{ open: boolean; stageId: string; milestone: RoadmapMilestone | null }>();
const emit = defineEmits<{ 'update:open': [boolean]; done: [] }>();

const text = ref('');
const err = ref('');
const busy = ref(false);

watch(
  () => [props.open, props.milestone] as const,
  () => {
    if (!props.open) return;
    text.value = props.milestone?.text ?? '';
    err.value = '';
  },
  { immediate: true },
);

async function save(): Promise<void> {
  if (!text.value.trim()) {
    err.value = 'Cần có nội dung mốc chặng.';
    return;
  }
  busy.value = true;
  err.value = '';
  try {
    if (props.milestone) {
      await updateMilestone(props.milestone.id, { text: text.value.trim() });
    } else {
      await createMilestone(props.stageId, text.value.trim());
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
    :title="milestone ? 'Sửa mốc chặng' : 'Thêm mốc chặng'"
    @update:open="emit('update:open', $event)"
  >
    <form class="grid gap-4" @submit.prevent="save">
      <Field label="Nội dung" for="ms-text" required>
        <Input id="ms-text" v-model="text" placeholder="Nói được 30 câu giao tiếp" />
      </Field>
      <p v-if="err" class="text-[13px] text-destructive" role="alert">{{ err }}</p>
      <div class="flex justify-end gap-2">
        <Button type="button" variant="outline" @click="emit('update:open', false)">Huỷ</Button>
        <Button type="submit" :disabled="busy">{{ busy ? 'Đang lưu…' : 'Lưu' }}</Button>
      </div>
    </form>
  </Dialog>
</template>
