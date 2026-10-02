<script setup lang="ts">
// M5 — port từ `routes/Hoc.tsx`: danh sách deck + tạo deck + tra từ điển.
// `fetchDecks`/`createDeck`/`dictSearch` giờ đi qua GraphQL (xem `srs/api.ts`).
import { onMounted, ref } from 'vue';
import NoticeBar from '../components/NoticeBar.vue';
import { errorMessage } from '../graphql/errors';
import type { Deck, DictEntry } from '../srs/api';
import { createDeck, dictSearch, fetchDecks } from '../srs/api';

const decks = ref<Deck[]>([]);
const name = ref('');
const lang = ref('zh');
const err = ref('');
const q = ref('ni');
const results = ref<DictEntry[]>([]);

async function load() {
  try {
    decks.value = await fetchDecks();
    err.value = '';
  } catch (e) {
    err.value = errorMessage(e);
  }
}

onMounted(load);

async function onCreate() {
  try {
    await createDeck(name.value.trim(), lang.value);
    name.value = '';
    await load();
  } catch (e) {
    err.value = errorMessage(e);
  }
}

async function doSearch() {
  try {
    results.value = await dictSearch(q.value);
    err.value = '';
  } catch (e) {
    err.value = errorMessage(e);
  }
}
</script>

<template>
  <div>
    <h2 class="text-xl font-semibold">Học</h2>
    <NoticeBar :text="err" />
    <section class="mt-4">
      <h3 class="font-semibold">Decks ({{ decks.length }})</h3>
      <ul class="mt-2 list-disc pl-5">
        <li v-for="d in decks" :key="d.id">
          {{ d.name }} [{{ d.lang }}] —
          <RouterLink class="underline" :to="`/review?deck=${d.id}`">ôn ngay</RouterLink>
        </li>
      </ul>
      <p v-if="decks.length === 0" class="text-muted">
        Chưa có deck nào. Tạo deck đầu tiên bên dưới.
      </p>
      <div class="mt-3 flex flex-wrap items-center gap-2">
        <input
          v-model="name"
          class="rounded-card border border-line px-2 py-1"
          placeholder="Tên deck (VD: HSK1)"
          aria-label="tên deck"
        />
        <select
          v-model="lang"
          class="rounded-card border border-line bg-white px-2 py-1"
          aria-label="ngôn ngữ"
        >
          <option value="zh">Trung (zh)</option>
          <option value="en">Anh (en)</option>
        </select>
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="onCreate"
        >
          Tạo deck
        </button>
      </div>
    </section>
    <section class="mt-6">
      <h3 class="font-semibold">Tra từ điển</h3>
      <div class="mt-2 flex flex-wrap items-center gap-2">
        <input
          v-model="q"
          class="rounded-card border border-line px-2 py-1"
          placeholder="q"
          aria-label="từ cần tra"
        />
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="doSearch"
        >
          Search
        </button>
      </div>
      <ul class="mt-2 list-disc pl-5">
        <li v-for="(r, i) in results" :key="i">
          {{ r.hanzi }} ({{ r.pinyin }}) — {{ r.nghia }}
        </li>
      </ul>
    </section>
  </div>
</template>
