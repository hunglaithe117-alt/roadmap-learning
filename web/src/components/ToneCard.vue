<script setup lang="ts">
// M5 — thẻ 1 câu drill: chữ Hán, pinyin có dấu, audio mẫu + badge engine TTS.
import { computed, useTemplateRef } from 'vue';
import { Volume2 } from 'lucide-vue-next';
import { toMarks } from '../chinese/pinyin';
import type { DrillItem } from '../chinese/api';
import { buildTTSUrl } from '../rest/client';
import { useTtsEngineQuery } from '../rest/useRest';

const props = defineProps<{ item: DrillItem }>();

// Badge đọc header `X-Engine` (không tải audio về). Đổi chữ là đổi queryKey ⇒
// tự gọi lại, không cần `watch` thủ công như bản v1.
const text = computed(() => props.item.hanzi);
const { data: ttsEngine } = useTtsEngineQuery(text);

const sample = useTemplateRef<HTMLAudioElement>('sample');
</script>

<template>
  <section class="rounded-card border border-line bg-white p-4" style="max-width: 480px">
    <p class="text-5xl">{{ item.hanzi }}</p>
    <p class="text-muted">{{ item.pinyin_marks || toMarks(item.pinyin) }}</p>
    <audio ref="sample" preload="auto" :src="buildTTSUrl(item.hanzi)" />
    <div class="mt-3 flex flex-wrap items-center gap-3">
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-card border border-line px-3 py-1.5 hover:border-line-strong"
        @click="sample?.play().catch(() => {})"
      >
        <Volume2 :size="16" aria-hidden="true" /> Nghe mẫu (giọng máy)
      </button>
      <span
        v-if="ttsEngine"
        data-testid="engine-badge"
        title="Engine TTS đang chạy (đọc từ header X-Engine)"
        class="text-muted"
      >
        X-Engine: {{ ttsEngine }}
      </span>
    </div>
  </section>
</template>
