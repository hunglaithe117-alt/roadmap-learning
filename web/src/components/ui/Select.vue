<!--
  `<select>` gốc, đóng khung kiểu shadcn.

  CỐ Ý không dùng `Select` của reka-ui: 9 kind tài liệu + 6 terrain + 4 trạng
  thái là danh sách ngắn, cần bàn phím và đọc bằng trình duyệt ngay. `<select>`
  gốc có sẵn cả ba, chạy được trong `happy-dom` (reka-ui cần `ResizeObserver` +
  vị trí đo) nên test form không phải dựng giả môi trường.

  PHẢI khai `modelValue` + emit `update:modelValue`: component render thẻ
  `<select>` mà không khai v-model thì `v-model` trên component rơi vào chỗ
  không, giá trị không bao giờ về — mọi bộ lọc trên màn sẽ hiện đúng rồi không
  bao giờ lọc. `Select.vue` của reka-ui có sẵn hợp đồng này nên đổi sang nó sau
  cũng không phải sửa chỗ gọi.
-->
<script setup lang="ts">
import { computed } from 'vue';
import { cn } from '../../lib/cn';

const props = defineProps<{ modelValue?: string; class?: string; id?: string; invalid?: boolean; disabled?: boolean }>();
const emit = defineEmits<{ 'update:modelValue': [value: string] }>();

const value = computed({
  get: () => props.modelValue ?? '',
  set: (v: string) => emit('update:modelValue', v),
});
</script>

<template>
  <select
    :id="id"
    :value="value"
    :disabled="disabled"
    :aria-invalid="invalid || undefined"
    :class="
      cn(
        'h-9 w-full rounded-md border border-input bg-surface px-2 text-sm shadow-sm transition-colors',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50',
        invalid ? 'border-destructive' : 'border-input hover:border-line-strong',
        $props.class,
      )
    "
    @change="value = ($event.target as HTMLSelectElement).value"
  >
    <slot />
  </select>
</template>
