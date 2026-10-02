<script setup lang="ts">
// M5 — port từ `routes/ZhBingo.tsx`: bàn 3x3 (8 chữ + ô free). Bấm ô → nghe
// audio → gõ thanh điệu → `gradeTone` chấm → đủ hàng/cột/chéo là Bingo →
// ghi điểm vòng bằng `postZhBingoScore` (ghi SRS từng thẻ, xem `chinese/api.ts`).
import { computed, onMounted, ref } from 'vue';
import DeckSelect from '../components/DeckSelect.vue';
import NoticeBar from '../components/NoticeBar.vue';
import { loadBingoItems, postZhBingoScore, postZhDrillGrade } from '../chinese/api';
import { buildBingoResults, boardScore, checkWin, dealBoard, markCell, type BingoCell } from '../chinese/bingo';
import { errorMessage } from '../graphql/errors';
import { buildTTSUrl } from '../rest/client';
import { fetchDecks, type Deck } from '../srs/api';

const decks = ref<Deck[]>([]);
const deckId = ref('');
const board = ref<BingoCell[]>([]);
const active = ref<string | null>(null);
const input = ref('');
const msg = ref('');
const err = ref('');
const won = ref(false);

onMounted(async () => {
  try {
    decks.value = (await fetchDecks()).filter((d) => d.lang === 'zh');
  } catch (e) {
    err.value = errorMessage(e);
  }
});

async function start() {
  if (!deckId.value) return;
  try {
    const items = await loadBingoItems(deckId.value);
    board.value = dealBoard(items);
    msg.value = '';
    won.value = false;
    active.value = null;
    err.value = '';
  } catch (e) {
    err.value = errorMessage(e);
  }
}

const activeCell = computed(() =>
  active.value === null ? null : (board.value.find((c) => c.key === active.value) ?? null),
);

async function answer() {
  const cell = activeCell.value;
  if (!cell || cell.free || !input.value.trim() || !deckId.value) return;
  try {
    const g = await postZhDrillGrade(cell.pair, input.value.trim());
    const next = markCell(board.value, active.value!, g.correct);
    board.value = next;
    input.value = '';
    active.value = null;
    if (checkWin(next)) {
      won.value = true;
      const saved = await postZhBingoScore(deckId.value, buildBingoResults(next));
      msg.value = `BINGO! Đúng ${saved.correct}/${saved.total} ô — điểm vòng đã lưu SRS.`;
    } else {
      const s = boardScore(next);
      msg.value = g.correct ? `Đúng! (${s.correct}/${s.total} ô)` : `Sai rồi (đáp án ${cell.pair}).`;
    }
    err.value = '';
  } catch (e) {
    err.value = errorMessage(e);
  }
}

/** Màu ô theo trạng thái — token `cell-ok` / `cell-bad` / `cell-free` của app.css. */
function cellClass(c: BingoCell): string {
  if (c.free) return 'bg-cell-free';
  if (c.marked === true) return 'bg-cell-ok';
  if (c.marked === false) return 'bg-cell-bad';
  return 'bg-white';
}
</script>

<template>
  <div>
    <h2 class="text-xl font-semibold">Tone Bingo</h2>
    <NoticeBar :text="err" />
    <div class="mt-2 flex flex-wrap items-center gap-3">
      <DeckSelect v-model="deckId" :decks="decks" label="Deck" :show-lang="false" />
      <button
        type="button"
        class="rounded-card border border-line px-3 py-1 hover:border-line-strong disabled:opacity-50"
        :disabled="!deckId"
        @click="start"
      >
        Chia bàn mới
      </button>
    </div>
    <NoticeBar v-if="msg" tone="info" :text="msg" />
    <div
      v-if="board.length > 0"
      class="mt-3 grid gap-2"
      style="grid-template-columns: repeat(3, 96px)"
    >
      <button
        v-for="c in board"
        :key="c.key"
        type="button"
        class="rounded-card border border-line text-3xl"
        style="height: 96px"
        :class="cellClass(c)"
        :disabled="c.free || c.marked !== null || won"
        :aria-label="c.free ? 'ô free' : `ô ${c.hanzi}`"
        @click="!c.free && (active = c.key)"
      >
        {{ c.hanzi }}
      </button>
    </div>
    <div v-if="activeCell" class="mt-3">
      <audio preload="auto" controls :src="buildTTSUrl(activeCell.hanzi)" />
      <p>
        Nghe “{{ activeCell.hanzi }}” rồi gõ thanh điệu (VD:
        {{ activeCell.pair.replace(/-/g, ' ') }}):
      </p>
      <div class="mt-1 flex items-center gap-2">
        <input
          v-model="input"
          class="rounded-card border border-line px-2 py-1"
          placeholder="3 3"
          aria-label="thanh điệu ô bingo"
        />
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="answer"
        >
          Chấm ô này
        </button>
      </div>
    </div>
  </div>
</template>
