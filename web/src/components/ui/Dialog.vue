<!--
  shadcn-vue `dialog` (reka-ui). Cửa sổ dùng chung cho 6 form thêm/sửa của M6.

  Cố ý bỏ phần "chỉ mở bằng nút" — 2 bản: `open` điều khiển từ ngoài để màn
  quyết định lúc nào mở, `DialogRoot` render theo `v-if` để không tạo nội dung
  cho tới khi cần.
-->
<script setup lang="ts">
import {
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from 'reka-ui';
import Button from './Button.vue';
import { cn } from '../../lib/cn';

defineProps<{
  open: boolean;
  title: string;
  description?: string;
  /** Nhãn nút huỷ. Để trống thì hiện nút × ở góc. */
  cancelLabel?: string;
  wide?: boolean;
}>();

const emit = defineEmits<{ 'update:open': [value: boolean] }>();
</script>

<template>
  <DialogRoot :open="open" @update:open="emit('update:open', $event)">
    <DialogPortal>
      <DialogOverlay class="fixed inset-0 z-40 bg-atlas/45 backdrop-blur-[2px]" />
      <DialogContent
        class="fixed left-1/2 top-6 z-50 max-h-[88vh] w-[min(94vw,44rem)] -translate-x-1/2 overflow-y-auto rounded-lg border border-border bg-card p-5 shadow-lift"
        :class="cn(wide && 'w-[min(96vw,64rem)]')"
      >
        <header class="mb-4 flex items-start justify-between gap-4">
          <div>
            <DialogTitle class="font-display text-lg font-semibold text-foreground">
              {{ title }}
            </DialogTitle>
            <DialogDescription
              v-if="description"
              class="mt-0.5 text-[13px] text-muted-foreground"
            >
              {{ description }}
            </DialogDescription>
          </div>
          <DialogClose as-child>
            <Button variant="ghost" size="icon" aria-label="đóng">×</Button>
          </DialogClose>
        </header>
        <slot />
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>
