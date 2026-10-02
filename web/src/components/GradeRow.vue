<!--
  4 nút chấm SRS (1 Quên · 2 Khó · 3 Được · 4 Dễ).

  App v1 copy mảng `GRADES` ở 3 màn (Review / EnStress / EnPvo) — 3 bản phải
  khớp nhau thì mới chấm đúng; ở đây 1 nguồn (`lib/grades.ts`) + 1 component.
-->
<script setup lang="ts">
import { GRADES } from '../lib/grades';

defineProps<{ disabled?: boolean }>();
const emit = defineEmits<{ grade: [value: number] }>();
</script>

<template>
  <div class="flex flex-wrap gap-2">
    <button
      v-for="g in GRADES"
      :key="g.value"
      type="button"
      class="rounded-card border border-line px-3 py-1.5 hover:border-line-strong disabled:opacity-50"
      :disabled="disabled"
      @click="emit('grade', g.value)"
    >
      {{ g.value }}. {{ g.label }}
    </button>
  </div>
</template>
