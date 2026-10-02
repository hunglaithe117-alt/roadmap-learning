<!--
  Thẻ ôn SRS: mặt trước → bấm hiện đáp án → 4 nút chấm.

  Dùng chung cho Review / EnStress / EnPvo (app v1 copy 3 lần cùng 1 khối).
  `showBack` do màn sở hữu vì sau mỗi lần chấm phải tự ẩn đáp án.
-->
<script setup lang="ts">
import { Volume2 } from 'lucide-vue-next';
import GradeRow from './GradeRow.vue';
import { buildTTSUrl } from '../rest/client';
import type { Card } from '../graphql/operations';

defineProps<{
  card: Card;
  showBack: boolean;
  /** Phát TTS mặt trước (có ở drill Anh, không có ở Review). */
  audio?: boolean;
}>();

const emit = defineEmits<{ reveal: []; grade: [value: number] }>();
</script>

<template>
  <div class="rounded-card border border-line bg-white p-4" style="max-width: 480px">
    <p class="text-3xl">{{ card.front }}</p>
    <p v-if="card.pinyin" class="text-muted">{{ card.pinyin }}</p>
    <audio
      v-if="audio"
      controls
      class="mt-2 w-full"
      :src="buildTTSUrl(card.front)"
    />
    <button
      v-if="!showBack"
      type="button"
      class="mt-3 inline-flex items-center gap-1.5 rounded-card border border-line px-3 py-1.5 hover:border-line-strong"
      @click="emit('reveal')"
    >
      <Volume2 :size="16" aria-hidden="true" /> Hiện đáp án
    </button>
    <div v-else class="mt-3">
      <p class="text-xl">{{ card.back }}</p>
      <GradeRow class="mt-2" @grade="(v: number) => emit('grade', v)" />
    </div>
  </div>
</template>
