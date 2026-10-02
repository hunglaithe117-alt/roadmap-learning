<!--
  `<select>` chọn deck — dùng ở 6 màn (Học / Review / Pinyin / Nét chữ / Bingo /
  Luyện nói / Recorder / Reader). App v1 copy nguyên khối `<label>Deck: <select>`
  6 lần; ở đây 1 component.

  `modelValue` là **chuỗi** (rỗng = chưa chọn) vì `ID` của GraphQL là `String`
  — giữ chuỗi suốt pipeline thay vì ép `Number` rồi lại stringify (ép `Number`
  trên `bigint` là mất chính xác).
-->
<script setup lang="ts">
import type { Deck } from '../graphql/operations';

withDefaults(
  defineProps<{
    decks: readonly Deck[];
    modelValue: string;
    label?: string;
    placeholder?: string;
    /** Bỏ trống = không dùng placeholder (dùng cho select bắt buộc chọn). */
    optional?: boolean;
    showLang?: boolean;
  }>(),
  { label: 'Deck', placeholder: '— chọn deck —', optional: true, showLang: true },
);

const emit = defineEmits<{ 'update:modelValue': [value: string] }>();
</script>

<template>
  <label class="inline-flex items-center gap-1">
    <span>{{ label }}:</span>
    <select
      class="rounded-card border border-line bg-white px-2 py-1"
      :value="modelValue"
      :aria-label="`chọn ${label.toLowerCase()}`"
      @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
    >
      <option v-if="optional" value="">{{ placeholder }}</option>
      <option v-for="d in decks" :key="d.id" :value="d.id">
        {{ d.name }}<template v-if="showLang"> [{{ d.lang }}]</template>
      </option>
    </select>
  </label>
</template>
