<!--
  Nhãn nhỏ để dán lên node/số liệu: trạng thái roadmap, kind tài liệu, trạng
  thái bookmark. Màu đến từ `tone` để 1 chỗ quyết định bảng màu, không rải
  điều kiện màu ở từng màn.
-->
<script setup lang="ts">
import { cn } from '../../lib/cn';

type Tone = 'neutral' | 'ember' | 'jade' | 'amber' | 'locked' | 'danger';

const TONE: Record<Tone, string> = {
  neutral: 'bg-secondary text-secondary-foreground border-line-soft',
  ember: 'bg-primary/10 text-primary border-primary/25',
  jade: 'bg-accent/12 text-accent border-accent/30',
  amber: 'bg-amber/15 text-[#7a5205] border-amber/40',
  locked: 'bg-locked/18 text-muted border-locked/40',
  danger: 'bg-destructive/10 text-destructive border-destructive/25',
};

withDefaults(defineProps<{ tone?: Tone; class?: string }>(), { tone: 'neutral' });
</script>

<template>
  <span
    class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[12px] font-medium whitespace-nowrap"
    :class="cn(TONE[$props.tone], $props.class)"
  >
    <slot />
  </span>
</template>
