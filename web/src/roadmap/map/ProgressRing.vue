<script setup lang="ts">
/**
 * Vòng tiến độ. Dùng `stroke-dasharray` trên 1 `<circle>` thay vì thư viện vẽ
 * chart: 1 con số, không cần thêm 15KB, và test được bằng cách đọc thuộc
 * tính `stroke-dashoffset` trong DOM.
 */
import { computed } from 'vue';

const props = withDefaults(
  defineProps<{ percent: number; size?: number; stroke?: number; label?: string }>(),
  { size: 56, stroke: 6, label: '' },
);

const R = computed(() => (props.size - props.stroke) / 2);
const C = computed(() => 2 * Math.PI * R.value);
const clamped = computed(() => Math.max(0, Math.min(100, props.percent)));
const offset = computed(() => C.value * (1 - clamped.value / 100));
</script>

<template>
  <div class="relative inline-flex items-center justify-center" :style="{ width: `${size}px`, height: `${size}px` }">
    <svg :width="size" :height="size" class="-rotate-90" role="img" :aria-label="`tiến độ ${clamped}%`">
      <circle
        :cx="size / 2"
        :cy="size / 2"
        :r="R"
        fill="none"
        stroke="var(--color-line-soft)"
        :stroke-width="stroke"
      />
      <circle
        data-testid="ring-arc"
        :cx="size / 2"
        :cy="size / 2"
        :r="R"
        fill="none"
        stroke="var(--color-ember)"
        :stroke-width="stroke"
        stroke-linecap="round"
        :stroke-dasharray="C"
        :stroke-dashoffset="offset"
        class="transition-[stroke-dashoffset] duration-700 ease-out"
      />
    </svg>
    <span class="tabular absolute text-[13px] font-semibold text-ink">
      {{ label || `${clamped}%` }}
    </span>
  </div>
</template>
