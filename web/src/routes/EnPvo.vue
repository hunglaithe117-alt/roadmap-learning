<script setup lang="ts">
// M5 — port từ `routes/EnPvo.tsx`: nạp seed PVO/TMRND rồi drill theo ngày.
import { ref } from 'vue';
import NoticeBar from '../components/NoticeBar.vue';
import SrsCard from '../components/SrsCard.vue';
import { postEnSeed } from '../english/api';
import { errorMessage } from '../graphql/errors';
import { fetchDecks, fetchDueCards, postReview, type Card } from '../srs/api';

const deckName = ref('PVO');
const queue = ref<Card[]>([]);
const showBack = ref(false);
const msg = ref('');
const err = ref('');

/** CHỈ chạy khi bấm nút — seed là mutation nạp dữ liệu, không tự chạy lúc mở màn. */
async function doSeed() {
  try {
    const r = await postEnSeed();
    msg.value = `Seed xong: en_dict=${r.enDict}, PVO +${r.pvoAdded}, TMRND +${r.tmrndAdded}`;
    err.value = '';
  } catch (e) {
    err.value = errorMessage(e);
  }
}

async function loadDue() {
  try {
    const decks = await fetchDecks();
    const deck = decks.find((d) => d.name === deckName.value && d.lang === 'en');
    if (!deck) {
      err.value = `Chưa có deck ${deckName.value} — bấm Seed trước.`;
      return;
    }
    queue.value = await fetchDueCards(deck.id);
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
    <h2 class="text-xl font-semibold">English — Drill PVO / TMRND</h2>
    <NoticeBar :text="err" />
    <NoticeBar v-if="msg" tone="ok" :text="msg" />
    <div class="mt-2 flex flex-wrap items-center gap-2">
      <button
        type="button"
        class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
        @click="doSeed"
      >
        Seed PVO + TMRND
      </button>
      <select
        v-model="deckName"
        class="rounded-card border border-line bg-white px-2 py-1"
        aria-label="deck PVO/TMRND"
      >
        <option value="PVO">PVO</option>
        <option value="TMRND">TMRND</option>
      </select>
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
  </div>
</template>
