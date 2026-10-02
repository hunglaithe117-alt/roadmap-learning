<!-- shadcn-vue `input`. `aria-invalid` để trình duyệt báo ô sai, và màu viền
     đổi theo — lỗi validate phải thấy được, không chỉ đọc bằng màu chữ. -->
<script setup lang="ts">
import { cn } from '../../lib/cn';

defineProps<{ class?: string; invalid?: boolean; id?: string }>();
</script>

<template>
  <input
    :id="id"
    :aria-invalid="invalid || undefined"
    :class="
      cn(
        'h-9 w-full rounded-md border border-input bg-surface px-2.5 text-sm shadow-sm transition-colors',
        'placeholder:text-dim focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        'disabled:opacity-50',
        invalid ? 'border-destructive' : 'border-input hover:border-line-strong',
        $props.class,
      )
    "
  />
</template>
