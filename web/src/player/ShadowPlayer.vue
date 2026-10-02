<script setup lang="ts">
// M5 — port từ `player/ShadowPlayer.tsx`: shadowing A-B loop dùng chung zh/en.
//
// Playlist = thẻ đến hạn của deck; vị trí lưu qua `postProgress` (server) và
// index trong localStorage (resume sau reload). Phím tắt giữ nguyên: Space
// phát/dừng, `[` đặt A, `]` đặt B.
import { computed, onBeforeUnmount, onMounted, ref, useTemplateRef, watch } from 'vue';
import DeckSelect from '../components/DeckSelect.vue';
import NoticeBar from '../components/NoticeBar.vue';
import { clampRate, loopTick, normalizeAB, progressKey, restoreIndex, type ABLoop } from './abloop';
import { errorMessage } from '../graphql/errors';
import { buildTTSUrl } from '../rest/client';
import { fetchProgress, postProgress } from './api';
import { fetchDecks, fetchDueCards, type Card, type Deck } from '../srs/api';

const decks = ref<Deck[]>([]);
const deckId = ref('');
const queue = ref<Card[]>([]);
const index = ref(0);
const rate = ref(1);
const loop = ref<ABLoop | null>(null);
const aText = ref('');
const bText = ref('');
const loops = ref(0);
const msg = ref('');
const err = ref('');

const audio = useTemplateRef<HTMLAudioElement>('audio');

onMounted(async () => {
  try {
    decks.value = await fetchDecks();
  } catch (e) {
    err.value = errorMessage(e);
  }
});

async function loadQueue(id: string) {
  try {
    const cards = await fetchDueCards(id);
    queue.value = cards;
    err.value = '';
    let start = 0;
    try {
      const saved = restoreIndex(localStorage.getItem(progressKey(id)), cards.length);
      if (saved != null) start = saved;
    } catch {
      /* bỏ qua khi không có storage */
    }
    index.value = cards.length > 0 ? Math.min(start, cards.length - 1) : 0;
    if (cards.length > 0) {
      const first = cards[Math.min(start, cards.length - 1)];
      try {
        const p = await fetchProgress(first.id);
        loops.value = p.loops;
        rate.value = clampRate(p.rate);
      } catch {
        /* chưa luyện thẻ này lần nào ⇒ giữ 0 lượt / tốc độ 1.0 */
      }
    }
  } catch (e) {
    err.value = errorMessage(e);
  }
}

watch(deckId, (id) => loadQueue(id));

const current = computed(() => queue.value[index.value]);

// Sang thẻ mới thì dừng audio, bỏ vòng lặp, xoá điểm A/B — giữ nguyên v1.
watch(index, () => {
  const el = audio.value;
  if (el) {
    el.playbackRate = clampRate(rate.value);
    el.pause();
  }
  loop.value = null;
  aText.value = '';
  bText.value = '';
});

function onTimeUpdate() {
  const el = audio.value;
  if (!el) return;
  const seek = loopTick(el.currentTime, loop.value);
  if (seek != null) {
    el.currentTime = seek;
    loops.value += 1;
  }
}

function applyLoop() {
  const dur = audio.value?.duration ?? 0;
  const l = normalizeAB(Number(aText.value), Number(bText.value), dur);
  if (!l) {
    err.value = 'A-B không hợp lệ (cần 2 điểm cách nhau ≥0.1s trong bài audio)';
    return;
  }
  loop.value = l;
  err.value = '';
}

function setPoint(which: 'a' | 'b') {
  const t = audio.value?.currentTime ?? 0;
  if (which === 'a') aText.value = t.toFixed(1);
  else bText.value = t.toFixed(1);
}

async function saveProgress(nextLoops = loops.value) {
  const card = current.value;
  if (!card) return;
  try {
    await postProgress(card.id, nextLoops, rate.value);
    msg.value = `Đã lưu tiến độ thẻ ${card.id} (${nextLoops} lượt, ${rate.value}x).`;
    try {
      localStorage.setItem(progressKey(deckId.value), String(index.value));
    } catch {
      /* bỏ qua */
    }
  } catch (e) {
    err.value = errorMessage(e);
  }
}

async function nextCard() {
  await saveProgress();
  loops.value = 0;
  index.value = Math.min(index.value + 1, Math.max(queue.value.length - 1, 0));
}

function onKey(e: KeyboardEvent) {
  const tag = (e.target as HTMLElement)?.tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
  if (e.code === 'Space') {
    e.preventDefault();
    const el = audio.value;
    if (!el) return;
    if (el.paused) el.play().catch(() => {});
    else el.pause();
  } else if (e.key === '[') {
    setPoint('a');
  } else if (e.key === ']') {
    setPoint('b');
  }
}

onMounted(() => window.addEventListener('keydown', onKey));
onBeforeUnmount(() => window.removeEventListener('keydown', onKey));

/** Bấm "＋1 lượt & lưu" — nâng số lượt rồi ghi, giữ nguyên v1. */
function bumpAndSave() {
  loops.value += 1;
  saveProgress(loops.value);
}
</script>

<template>
  <div>
    <h2 class="text-xl font-semibold">Luyện nói (Shadowing)</h2>
    <NoticeBar :text="err" />
    <NoticeBar v-if="msg" tone="ok" :text="msg" />
    <div class="mt-2 flex items-center gap-3">
      <DeckSelect v-model="deckId" :decks="decks" label="Deck" />
    </div>
    <p v-if="!current && deckId" class="mt-3 text-muted">Hết bài hôm nay 🎉</p>
    <section
      v-if="current"
      class="mt-3 rounded-card border border-line bg-white p-4"
      style="max-width: 560px"
    >
      <p>Thẻ {{ index + 1 }}/{{ queue.length }} — {{ current.front }}</p>
      <p class="text-3xl">{{ current.front }}</p>
      <p v-if="current.pinyin" class="text-muted">{{ current.pinyin }}</p>
      <audio
        ref="audio"
        controls
        preload="auto"
        class="mt-2 w-full"
        :src="buildTTSUrl(current.front)"
        @timeupdate="onTimeUpdate"
      />
      <div class="mt-2">
        <label class="inline-flex items-center gap-2">
          <span>Tốc độ {{ rate.toFixed(1) }}x</span>
          <input
            type="range"
            min="0.5"
            max="1.5"
            step="0.1"
            :value="rate"
            aria-label="tốc độ phát"
            @input="rate = clampRate(Number(($event.target as HTMLInputElement).value))"
          />
        </label>
      </div>
      <div class="mt-2 flex flex-wrap items-center gap-2">
        <input
          v-model="aText"
          class="rounded-card border border-line px-2 py-1"
          placeholder="A (giây)"
          aria-label="điểm A"
          style="width: 90px"
        />
        <input
          v-model="bText"
          class="rounded-card border border-line px-2 py-1"
          placeholder="B (giây)"
          aria-label="điểm B"
          style="width: 90px"
        />
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="setPoint('a')"
        >
          [ = A
        </button>
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="setPoint('b')"
        >
          ] = B
        </button>
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="applyLoop"
        >
          Lặp A-B
        </button>
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="loop = null"
        >
          Hủy lặp
        </button>
      </div>
      <p v-if="loop" class="mt-2">
        Đang lặp {{ loop.a.toFixed(1) }}s → {{ loop.b.toFixed(1) }}s — {{ loops }} lượt.
      </p>
      <div class="mt-2 flex flex-wrap gap-2">
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="bumpAndSave"
        >
          ＋1 lượt &amp; lưu
        </button>
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="nextCard"
        >
          Thẻ tiếp →
        </button>
      </div>
      <p class="mt-2 text-[13px] text-muted">
        Phím tắt: Space phát/dừng, [ đặt A, ] đặt B.
      </p>
    </section>
  </div>
</template>
