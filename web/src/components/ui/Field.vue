<!--
  Cụm 1 ô form: nhãn ở trên, control, gợi ý/lỗi ở dưới. Lỗi luôn chiếm chỗ
  kể cả khi rỗng (`aria-live`) để form không nhảy layout khi báo lỗi — nhảy
  layout giữa lúc người dùng đang bấm nút "Lưu" làm bấm trượt.
-->
<script setup lang="ts">
import Label from './Label.vue';

defineProps<{ label: string; for?: string; error?: string; hint?: string; required?: boolean }>();
</script>

<template>
  <div class="flex flex-col gap-1">
    <Label :for="$props.for">
      {{ label }}<span v-if="required" class="text-ember"> *</span>
    </Label>
    <slot />
    <p v-if="$props.error" class="text-[13px] text-destructive" role="alert">{{ $props.error }}</p>
    <p v-else-if="$props.hint" class="text-[13px] text-muted-foreground">{{ $props.hint }}</p>
  </div>
</template>
