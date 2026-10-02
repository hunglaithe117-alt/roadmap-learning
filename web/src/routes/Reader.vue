<script setup lang="ts">
// M5 — port từ `routes/Reader.tsx`: bấm từ trong bài đọc để tra (zh qua
// `dictSearch`, en qua `englishSearch`), lưu từ mới thành card SRS 1 chạm.
//
// Popup neo theo toạ độ chuột như v1 (`position: fixed`), kẹp trong khung nhìn.
//
// 5.8 (cổng Oracle M5): comment cũ nói popup "neo lại khi resize", nhưng
// `window.innerWidth/innerHeight` KHÔNG được Vue theo dõi nên điều đó không xảy
// ra. Đã nói thẳng ở đây: hiện popup chỉ neo 1 lần khi mở, đúng như bản v1. Muốn
// neo theo resize thì phải `onMounted` + listener `resize` — việc của người sửa,
// không phải việc ngầm của `computed`.
import { computed, onMounted, ref } from 'vue';
import DeckSelect from '../components/DeckSelect.vue';
import NoticeBar from '../components/NoticeBar.vue';
import { fetchEnSearch } from '../english/api';
import { errorMessage } from '../graphql/errors';
import { fetchReader, fetchReaderLevels, type ReaderArticle } from '../player/api';
import { dictSearch, createCard, fetchDecks, type Deck } from '../srs/api';

interface Popup {
  word: string;
  lines: string[];
  ms: number;
  x: number;
  y: number;
}

/** Tiếng Anh tách theo từ (kèm khoảng trắng), tiếng Trung tách theo KÝ TỰ. */
function tokenize(text: string, lang: string): string[] {
  if (lang === 'en') return text.split(/(\s+)/);
  return text.split('');
}

const levels = ref<string[]>([]);
const level = ref('');
const articles = ref<ReaderArticle[]>([]);
const current = ref<ReaderArticle | null>(null);
const popup = ref<Popup | null>(null);
const decks = ref<Deck[]>([]);
const deckId = ref('');
const msg = ref('');
const err = ref('');

onMounted(async () => {
  try {
    levels.value = await fetchReaderLevels();
  } catch (e) {
    err.value = errorMessage(e);
  }
  try {
    decks.value = await fetchDecks();
  } catch (e) {
    err.value = errorMessage(e);
  }
});

async function loadArticles(lv: string) {
  level.value = lv;
  try {
    const list = await fetchReader(lv || undefined);
    articles.value = list;
    current.value = list[0] ?? null;
    popup.value = null;
  } catch (e) {
    err.value = errorMessage(e);
  }
}

async function lookup(word: string, e: MouseEvent) {
  const w = word.trim();
  const article = current.value;
  if (!w || !article) return;
  const t0 = performance.now();
  try {
    let lines: string[];
    if (article.lang === 'en') {
      const r = await fetchEnSearch(w.replace(/^[.,!?;:"'()]+|[.,!?;:"'()]+$/g, ''));
      lines = r.slice(0, 3).map((x) => `${x.term} [${x.reading}] — ${x.gloss}`);
      if (lines.length === 0) lines = ['(chưa có trong en_dict)'];
    } else {
      const r = await dictSearch(w);
      lines = r.slice(0, 3).map((x) => `${x.hanzi} (${x.pinyin}) — ${x.nghia}`);
      if (lines.length === 0) lines = ['(chưa có trong dict)'];
    }
    popup.value = { word: w, lines, ms: performance.now() - t0, x: e.clientX, y: e.clientY };
  } catch (e) {
    err.value = errorMessage(e);
  }
}

async function saveWord() {
  const p = popup.value;
  if (!p || !deckId.value) {
    err.value = 'Chọn deck để lưu từ mới thành card SRS.';
    return;
  }
  try {
    await createCard(deckId.value, { front: p.word, back: p.lines[0] ?? '' });
    msg.value = `Đã lưu "${p.word}" — card mới due ngày mai.`;
  } catch (e) {
    err.value = errorMessage(e);
  }
}

/**
 * Vị trí popup. `computed` chứ không phải hàm thường: template gọi lại mỗi lần
 * render là rẻ, còn hàm thường thì giá trị không ai theo dõi.
 *
 * KHÔNG tự neo lại khi resize (xem comment đầu file) — `window.innerWidth`/
 * `innerHeight` không được Vue theo dõi, cần `onMounted` + listener `resize` mới
 * làm được. Giữ hành vi v1: neo 1 lần khi mở popup.
 */
const popupStyle = computed(() => {
  const p = popup.value;
  if (!p) return {};
  return {
    left: `${Math.min(p.x, window.innerWidth - 280)}px`,
    top: `${Math.min(p.y + 12, window.innerHeight - 160)}px`,
  };
});
</script>

<template>
  <div>
    <h2 class="text-xl font-semibold">Đọc hiểu (Graded reader)</h2>
    <NoticeBar :text="err" />
    <NoticeBar v-if="msg" tone="ok" :text="msg" />
    <div class="mt-2 flex flex-wrap gap-2">
      <button
        type="button"
        class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
        :class="{ 'font-bold': level === '' }"
        @click="loadArticles('')"
      >
        Tất cả
      </button>
      <button
        v-for="lv in levels"
        :key="lv"
        type="button"
        class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
        :class="{ 'font-bold': level === lv }"
        @click="loadArticles(lv)"
      >
        {{ lv }}
      </button>
    </div>
    <div v-if="articles.length > 0" class="mt-2">
      <label class="inline-flex items-center gap-1">
        <span>Bài:</span>
        <select
          class="rounded-card border border-line bg-white px-2 py-1"
          :value="current?.id ?? ''"
          aria-label="chọn bài đọc"
          @change="
            current = articles.find((a) => a.id === ($event.target as HTMLSelectElement).value) ?? null
          "
        >
          <option v-for="a in articles" :key="a.id" :value="a.id">
            [{{ a.level }}] {{ a.title }}
          </option>
        </select>
      </label>
    </div>
    <article
      v-if="current"
      class="mt-3 rounded-card border border-line bg-white p-4"
      style="max-width: 640px; font-size: 20px; line-height: 2"
    >
      <h3 class="font-semibold">{{ current.title }}</h3>
      <p>
        <template v-for="(tok, i) in tokenize(current.text, current.lang)" :key="i">
          <span v-if="tok.trim() === ''">{{ tok }}</span>
          <span
            v-else
            class="cursor-pointer border-b border-dotted border-line"
            title="bấm để tra từ"
            @click="lookup(tok, $event)"
            >{{ tok }}</span
          >
        </template>
      </p>
      <p class="text-[12px] text-muted">Nguồn: {{ current.source }}</p>
    </article>
    <div
      v-if="popup"
      data-testid="lookup-popup"
      class="fixed z-10 rounded-card border border-line-strong bg-white p-3"
      style="max-width: 260px"
      :style="popupStyle"
    >
      <p>
        <strong>{{ popup.word }}</strong>
        <span class="text-muted">({{ popup.ms.toFixed(0) }}ms)</span>
      </p>
      <ul class="mt-1 list-disc pl-4">
        <li v-for="(l, i) in popup.lines" :key="i">{{ l }}</li>
      </ul>
      <div class="mt-2 flex flex-wrap items-center gap-2">
        <DeckSelect
          v-model="deckId"
          :decks="decks"
          label="deck lưu từ"
          placeholder="— deck —"
        />
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="saveWord"
        >
          Lưu SRS
        </button>
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="popup = null"
        >
          Đóng
        </button>
      </div>
    </div>
  </div>
</template>
