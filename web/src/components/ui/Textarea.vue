<!-- shadcn-vue `textarea`. Cao mặc định 5 dòng — form của M6 chủ yếu ghi
     mô tả/đường dẫn tài liệu, những đoạn dài, một dòng là chật. -->
<script setup lang="ts">
import { cn } from '../../lib/cn';

defineProps<{ class?: string; invalid?: boolean; id?: string }>();
</script>

<template>
  <textarea
    :id="id"
    :aria-invalid="invalid || undefined"
    :class="
      cn(
        'w-full rounded-md border border-input bg-surface px-2.5 py-2 text-sm shadow-sm transition-colors',
        'placeholder:text-dim focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        invalid ? 'border-destructive' : 'border-input hover:border-line-strong',
        $props.class,
      )
    "
  />
</template>
