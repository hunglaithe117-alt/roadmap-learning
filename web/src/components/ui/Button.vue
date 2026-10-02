<!--
  shadcn-vue `button` — nền của mọi nút trong app. Biến thể bám theo chất
  liệu: `default` dùng nhấn Ember, `ghost` cho thao tác phụ, `outline` cho
  hành động thứ hai cạnh hành động chính, `danger` cho xoá.
-->
<script setup lang="ts">
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '../../lib/cn';

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-45',
  {
    variants: {
      variant: {
        default: 'bg-primary text-primary-foreground hover:brightness-110',
        outline: 'border border-line bg-surface hover:border-line-strong hover:bg-secondary',
        ghost: 'hover:bg-secondary',
        danger: 'border border-destructive/40 text-destructive hover:bg-destructive/10',
      },
      size: {
        sm: 'h-7 px-2.5 text-[13px]',
        md: 'h-9 px-3.5',
        lg: 'h-11 px-6 text-base',
        icon: 'h-9 w-9',
      },
    },
    defaultVariants: { variant: 'default', size: 'md' },
  },
);

export type ButtonVariants = VariantProps<typeof buttonVariants>;

defineProps<{ class?: string; variant?: ButtonVariants['variant']; size?: ButtonVariants['size'] }>();
</script>

<template>
  <button :class="cn(buttonVariants({ variant, size }), $props.class)">
    <slot />
  </button>
</template>
