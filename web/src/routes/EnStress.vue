<script setup lang="ts">
// M5 — port từ `routes/EnStress.tsx`: tra trọng âm + tách chunk + drill SRS.
//
// `StressResult.source` ở client = `'dict' | 'rule'`, do `english/api.ts` map từ
// `fromDict` của schema. `Chunk.kind` server trả `CONTENT|FUNCTION`, client đổi
// sang `content|function` cho đúng kiểu của `english/chunk.ts`.
import { ref } from 'vue';
import NoticeBar from '../components/NoticeBar.vue';
import SrsCard from '../components/SrsCard.vue';
import { splitChunks, type Chunk } from '../english/chunk';
import { splitStressMarks, type StressResult } from '../english/stress';
import {
  fetchEnSearch,
  fetchEnStress,
  postEnChunks,
  type EnEntry,
} from '../english/api';
import { errorMessage } from '../graphql/errors';
import { buildTTSUrl } from '../rest/client';
import { fetchDecks, fetchDueCards, postReview, type Card } from '../srs/api';

const word = ref('photograph');
const entry = ref<StressResult | null>(null);
const dictHits = ref<EnEntry[]>([]);
const sentence = ref('I want to make progress');
// Giá trị khởi tạo tính ngay (8 từ là rẻ) — `ref(() => …)` của Vue giữ hàm làm
// giá trị chứ không lazy, nên viết callback ở đây sẽ cho `chunks.value` là hàm.
const chunks = ref<Chunk[]>(splitChunks('I want to make progress'));
const deckId = ref('');
const queue = ref<Card[]>([]);
const showBack = ref(false);
const err = ref('');

/** Tách 1 lần rồi render — v1 gọi `splitStressMarks` ngay trong JSX mỗi vòng lặp. */
const parts = () => splitStressMarks(entry.value?.stress || entry.value?.term || '');

async function doLookup() {
  try {
    entry.value = await fetchEnStress(word.value.trim());
    dictHits.value = await fetchEnSearch(word.value.trim());
    err.value = '';
  } catch (e) {
    err.value = errorMessage(e);
  }
}

async function doChunk() {
  try {
    chunks.value = splitChunks(sentence.value);
    chunks.value = (await postEnChunks(sentence.value)).chunks;
    err.value = '';
  } catch (e) {
    err.value = errorMessage(e);
  }
}

async function loadDue() {
  try {
    const decks = await fetchDecks();
    const en = decks.find((d) => d.lang === 'en' && d.id === deckId.value) ?? decks.find((d) => d.lang === 'en');
    const id = deckId.value || (en ? en.id : '');
    if (!id) {
      err.value = 'Chưa có deck tiếng Anh — qua tab PVO bấm Seed trước.';
      return;
    }
    deckId.value = id;
    queue.value = await fetchDueCards(id);
    showBack.value = false;
    err.value = '';
  } catch (e) {
    err.value = errorMessage(e);
  }
}

async function grade(g: number) {
  const cur = queue.value[0];
  if (!cur) return;
  try {
    await postReview(cur.id, g);
    queue.value = queue.value.slice(1);
    showBack.value = false;
  } catch (e) {
    err.value = errorMessage(e);
  }
}
</script>

<template>
  <div>
    <h2 class="text-xl font-semibold">English — Trọng âm &amp; Chunking</h2>
    <NoticeBar :text="err" />
    <section class="mt-4">
      <h3 class="font-semibold">Tra trọng âm</h3>
      <div class="mt-2 flex items-center gap-2">
        <input
          v-model="word"
          class="rounded-card border border-line px-2 py-1"
          aria-label="từ tiếng Anh"
        />
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="doLookup"
        >
          Tra
        </button>
      </div>
      <div
        v-if="entry"
        class="mt-2 rounded-card border border-line bg-white p-3"
        style="max-width: 480px"
      >
        <p class="text-2xl">
          <template v-for="(s, i) in parts()" :key="i">
            <span :class="s.stressed ? 'font-bold' : ''">
              {{ s.stressed ? s.text : s.text.toLowerCase() }}</span
            ><template v-if="i < parts().length - 1">·</template>
          </template>
        </p>
        <p v-if="entry.ipa" class="text-muted">{{ entry.ipa }}</p>
        <p v-if="entry.exception" class="text-warn">
          <strong>ngoại lệ</strong>
          <template v-if="entry.note"> — {{ entry.note }}</template>
        </p>
        <audio controls class="mt-2 w-full" :src="buildTTSUrl(entry.term)" />
      </div>
      <ul v-if="dictHits.length > 0" class="mt-2 list-disc pl-5">
        <li v-for="d in dictHits" :key="d.term">{{ d.term }} {{ d.reading }} — {{ d.gloss }}</li>
      </ul>
    </section>
    <section class="mt-4">
      <h3 class="font-semibold">Tách chunk</h3>
      <div class="mt-2 flex items-center gap-2">
        <input
          v-model="sentence"
          class="rounded-card border border-line px-2 py-1"
          aria-label="câu mẫu"
          style="width: 320px"
        />
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="doChunk"
        >
          Tách
        </button>
      </div>
      <p class="mt-2">
        <span
          v-for="(c, i) in chunks"
          :key="i"
          class="mr-1.5"
          :class="c.kind === 'content' ? 'font-bold' : 'text-dim opacity-60'"
        >
          {{ c.text }}
        </span>
      </p>
    </section>
    <section class="mt-4">
      <h3 class="font-semibold">Drill theo ngày (SRS)</h3>
      <div class="mt-2 flex items-center gap-2">
        <input
          v-model="deckId"
          class="rounded-card border border-line px-2 py-1"
          placeholder="deck id"
          aria-label="deck id"
        />
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="loadDue"
        >
          Tải thẻ đến hạn
        </button>
      </div>
      <p v-if="!queue[0]" class="mt-2 text-muted">Hết bài hoặc chưa tải.</p>
      <SrsCard
        v-if="queue[0]"
        class="mt-2"
        :card="queue[0]"
        :show-back="showBack"
        audio
        @reveal="showBack = true"
        @grade="grade"
      />
    </section>
  </div>
</template>
