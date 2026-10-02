<!--
  Thông báo 1 dòng dùng chung (lỗi / thành công / cảnh báo).

  Vì sao tách component: app v1 in-line `<p style={{color:'red'}}>` ở 11/14 màn —
  copy 11 lần cùng 1 class là chỗ dễ để sót 1 màn không bắt lỗi. Ở đây màu đi
  từ token vai trò trong `styles/app.css` (`err`/`ok`/`warn`) nên đổi bảng màu ở
  M6 chỉ sửa 1 chỗ.

  Không tự dịch lại message: `text` là nguyên văn server trả về (xem
  `graphql/errors.ts`).
-->
<script setup lang="ts">
withDefaults(
  defineProps<{
    text?: string;
    tone?: 'err' | 'ok' | 'warn' | 'info';
  }>(),
  { text: '', tone: 'err' },
);
</script>

<template>
  <p
    v-if="text"
    data-testid="notice"
    :class="{
      'text-err': tone === 'err',
      'text-ok': tone === 'ok',
      'text-warn': tone === 'warn',
      'text-muted': tone === 'info',
    }"
  >
    {{ text }}
  </p>
</template>
