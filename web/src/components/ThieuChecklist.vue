<script setup lang="ts">
// M5 — checklist THIEU 8 trục (thang 1-5) + bảng tiến bộ.
// Tách 2 component khỏi bản v1 (`ThieuChecklist`, `ThieuChart` nằm chung file
// `EnThieu.tsx`) vì `<script setup>` chỉ xuất được 1 component mỗi file.
import { ref } from 'vue';
import { THIEU_CODES, thieuAverage, type ThieuAxis } from '../english/thieu';
import { postThieu } from '../english/api';
import { errorMessage } from '../graphql/errors';
import type { ThieuSession } from '../english/thieu';

const props = defineProps<{ axes: ThieuAxis[] }>();
const emit = defineEmits<{ saved: [session: ThieuSession] }>();

// Dựng thẳng object: 8 phần tử là rẻ, và `ref(() => …)` mang nghĩa KHÁC — hàm
// được giữ nguyên làm giá trị, nên viết lazy ở đây sẽ cho `scores.value` là 1
// hàm thay vì object điểm.
const scores = ref<Record<string, number>>(Object.fromEntries(THIEU_CODES.map((c) => [c, 3])));
const note = ref('');
const err = ref('');

async function submit() {
  try {
    const today = new Date().toISOString().slice(0, 10);
    const saved = await postThieu(today, scores.value, note.value);
    emit('saved', saved);
    err.value = '';
  } catch (e) {
    err.value = errorMessage(e);
  }
}
</script>

<template>
  <section>
    <h3 class="font-semibold">Checklist THIEU (thang 1–5)</h3>
    <p v-if="err" data-testid="notice" class="text-err">{{ err }}</p>
    <label v-for="a in props.axes" :key="a.code" class="mb-2 block">
      <strong>{{ a.code }}. {{ a.name }}</strong> — {{ a.desc }}
      <br />
      <select
        class="rounded-card border border-line bg-white px-2 py-1"
        :value="scores[a.code] ?? 3"
        :aria-label="`điểm trục ${a.code}`"
        @change="scores[a.code] = Number(($event.target as HTMLSelectElement).value)"
      >
        <option v-for="v in [1, 2, 3, 4, 5]" :key="v" :value="v">{{ v }}</option>
      </select>
    </label>
    <div class="mt-2 flex flex-wrap items-center gap-2">
      <input
        v-model="note"
        class="rounded-card border border-line px-2 py-1"
        placeholder="ghi chú buổi học"
        aria-label="ghi chú buổi học"
      />
      <button
        type="button"
        class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
        @click="submit"
      >
        Lưu buổi học (TB: {{ thieuAverage(scores).toFixed(2) }})
      </button>
    </div>
  </section>
</template>
