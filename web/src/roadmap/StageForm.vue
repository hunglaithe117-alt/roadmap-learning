<script setup lang="ts">
/**
 * Form chặng (stage) — chặn điều hướng + ĐỊA HÌNH của bản đồ.
 *
 * `terrain` và `direction` là 2 trường quyết định diện mạo bản đồ nên được
 * đặt cạnh nhau ở đầu form, kèm 1 ô xem trước nhỏ — người dùng đổi địa hình mà
 * không thấy gì đổi thì sẽ đoán là nút hỏng.
 *
 * Danh sách terrain lấy từ `map/terrain.ts` chứ không gõ tay ở đây: cùng 1
 * nguồn với bản đồ đang vẽ, thêm terrain mới không phải sửa 2 file.
 */
import { computed, ref, watch } from 'vue';
import Button from '../components/ui/Button.vue';
import Dialog from '../components/ui/Dialog.vue';
import Field from '../components/ui/Field.vue';
import Input from '../components/ui/Input.vue';
import Select from '../components/ui/Select.vue';
import Textarea from '../components/ui/Textarea.vue';
import { TERRAINS, terrainLabel, type Terrain } from './map/terrain';
import { createStage, updateStage, type NewStage, type RoadmapStage, type StagePatch } from './api';
import type { MapDirection } from '../graphql/operations';
import { errorMessage } from '../graphql/errors';

const props = defineProps<{ open: boolean; pathSlug: string; stage: RoadmapStage | null }>();
const emit = defineEmits<{ 'update:open': [boolean]; done: [] }>();

const slug = ref('');
const title = ref('');
const goal = ref('');
const position = ref('');
const durationWeeks = ref('');
const deckId = ref('');
const terrain = ref<Terrain>('MEADOW');
const direction = ref<MapDirection>('UP');
const err = ref('');
const busy = ref(false);

const editing = computed(() => props.stage !== null);
const skin = computed(() => terrain.value);

watch(
  () => [props.open, props.stage] as const,
  () => {
    if (!props.open) return;
    slug.value = props.stage?.slug ?? '';
    title.value = props.stage?.title ?? '';
    goal.value = props.stage?.goal ?? '';
    position.value = props.stage ? String(props.stage.position) : '';
    durationWeeks.value = props.stage ? String(props.stage.durationWeeks) : '';
    deckId.value = props.stage?.deckId ?? '';
    terrain.value = props.stage?.terrain ?? 'MEADOW';
    direction.value = props.stage?.direction ?? 'UP';
    err.value = '';
  },
  { immediate: true },
);

function toInt(raw: string): number | null {
  const v = raw.trim();
  if (v === '') return null;
  const n = Number(v);
  return Number.isFinite(n) ? Math.trunc(n) : null;
}

async function save(): Promise<void> {
  if (!title.value.trim()) {
    err.value = 'Cần có tiêu đề chặng.';
    return;
  }
  if (!editing.value && !/^[a-z0-9-]{1,64}$/.test(slug.value.trim())) {
    err.value = 'Slug chỉ gồm chữ thường, số và dấu gạch ngang.';
    return;
  }
  busy.value = true;
  err.value = '';
  try {
    if (props.stage) {
      const patch: StagePatch = {
        title: title.value.trim(),
        goal: goal.value.trim(),
        position: toInt(position.value) ?? undefined,
        durationWeeks: toInt(durationWeeks.value) ?? undefined,
        deckId: deckId.value.trim() || null,
        terrain: terrain.value,
        direction: direction.value,
      };
      await updateStage(props.stage.id, patch);
    } else {
      const input: NewStage = {
        slug: slug.value.trim(),
        title: title.value.trim(),
        goal: goal.value.trim(),
        position: toInt(position.value),
        durationWeeks: toInt(durationWeeks.value),
        deckId: deckId.value.trim() || null,
        terrain: terrain.value,
        direction: direction.value,
      };
      await createStage(props.pathSlug, input);
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
    :title="editing ? 'Sửa chặng' : 'Thêm chặng'"
    @update:open="emit('update:open', $event)"
  >
    <form class="grid gap-4 sm:grid-cols-2" @submit.prevent="save">
      <Field v-if="!editing" label="Slug" for="stage-slug" required hint="chữ thường, số, dấu gạch ngang.">
        <Input id="stage-slug" v-model="slug" placeholder="zh-g5" />
      </Field>
      <Field label="Tiêu đề" for="stage-title" required>
        <Input id="stage-title" v-model="title" />
      </Field>
      <div class="sm:col-span-2">
        <Field label="Mục tiêu" for="stage-goal">
          <Textarea id="stage-goal" v-model="goal" rows="2" />
        </Field>
      </div>

      <Field label="Địa hình" for="stage-terrain" hint="Quyết định màu nền và hình dạng đường đi.">
        <Select id="stage-terrain" v-model="terrain" :data-terrain="skin">
          <option v-for="t in TERRAINS" :key="t" :value="t">{{ terrainLabel(t) }}</option>
        </Select>
      </Field>
      <Field label="Chiều đi" for="stage-dir" hint="Vuốt dọc hay vuốt ngang để cuộn bản đồ.">
        <Select id="stage-dir" v-model="direction">
          <option value="UP">Dọc (dưới lên trên)</option>
          <option value="RIGHT">Ngang (trái sang phải)</option>
        </Select>
      </Field>

      <Field label="Thứ tự" for="stage-pos" hint="Bỏ trống = thêm vào cuối.">
        <Input id="stage-pos" v-model="position" inputmode="numeric" />
      </Field>
      <Field label="Số tuần" for="stage-weeks">
        <Input id="stage-weeks" v-model="durationWeeks" inputmode="numeric" />
      </Field>
      <div class="sm:col-span-2">
        <Field label="Deck ôn" for="stage-deck" hint="Có deck thì màn hiện nút vào /review. Để trống = chưa gắn.">
          <Input id="stage-deck" v-model="deckId" inputmode="numeric" placeholder="id deck" />
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
