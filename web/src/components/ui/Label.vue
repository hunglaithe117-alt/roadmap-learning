<!--
  Nhãn form: chữ nhỏ đậm, khoảng cách dưới gắn với control. `Field.vue` bọc
  cả cụm (nhãn + control + lỗi) — 6 form của M6 đều dùng đúng cụm này.
-->
<script setup lang="ts">
import { cn } from '../../lib/cn';

defineProps<{ class?: string; for?: string }>();
</script>

<template>
  <label :for="$props.for" :class="cn('text-[13px] font-semibold text-ink', $props.class)">
    <slot />
  </label>
</template>
