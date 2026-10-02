<script setup lang="ts">
// M5 — port từ `routes/Recorder.tsx`: MediaRecorder → `POST /api/stt` → diff với
// câu mẫu (tô đỏ từ sai) → lưu sổ lỗi hoặc tạo thẻ SRS.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import DeckSelect from '../components/DeckSelect.vue';
import NoticeBar from '../components/NoticeBar.vue';
import { diffScore, wordDiff, wrongWords, type DiffToken } from '../player/diff';
import { errorMessage } from '../graphql/errors';
import { buildTTSUrl, uploadSTT, type STTResult } from '../rest/client';
import { fetchSuggestErrors, postError, type SuggestCard } from '../player/api';
import { createCard, createDeck, fetchDecks, type Deck } from '../srs/api';

const MAX_SEC = 60;

/** Màu 1 token diff — cùng bảng màu của app v1 (đỏ/cam/xám/đen). */
function statusClass(s: DiffToken['status']): string {
  switch (s) {
    case 'ok':
      return 'text-ink';
    case 'wrong':
      return 'text-err';
    case 'missing':
      return 'text-warn';
    case 'extra':
      return 'text-dim';
  }
}

const expected = ref('ni hao ma');
const cardId = ref('');
const recording = ref(false);
const secs = ref(0);
const stt = ref<STTResult | null>(null);
const err = ref('');
const msg = ref('');
const decks = ref<Deck[]>([]);
const deckId = ref('');
const suggest = ref<SuggestCard[]>([]);

let rec: MediaRecorder | null = null;
let chunks: Blob[] = [];
let timer: number | null = null;

onMounted(async () => {
  try {
    decks.value = await fetchDecks();
  } catch (e) {
    err.value = errorMessage(e);
  }
  try {
    suggest.value = await fetchSuggestErrors(10);
  } catch {
    /* gợi ý là phụ — lỗi không chặn màn ghi âm */
  }
});

onBeforeUnmount(() => {
  if (timer != null) window.clearInterval(timer);
});

function stop() {
  if (timer != null) {
    window.clearInterval(timer);
    timer = null;
  }
  recording.value = false;
  rec?.stop();
}

async function start() {
  err.value = '';
  msg.value = '';
  stt.value = null;
  let stream: MediaStream;
  try {
    stream = await navigator.mediaDevices.getUserMedia({ audio: true });
  } catch {
    err.value = 'Không có quyền micro. Hãy cho phép micro trong browser rồi thử lại.';
    return;
  }
  const mime = MediaRecorder.isTypeSupported('audio/webm') ? 'audio/webm' : '';
  const r = mime ? new MediaRecorder(stream, { mimeType: mime }) : new MediaRecorder(stream);
  chunks = [];
  r.ondataavailable = (e) => {
    if (e.data.size > 0) chunks.push(e.data);
  };
  r.onstop = async () => {
    stream.getTracks().forEach((t) => t.stop());
    const blob = new Blob(chunks, { type: mime || 'audio/webm' });
    try {
      msg.value = 'Đang nhận dạng...';
      stt.value = await uploadSTT(blob);
      msg.value = '';
    } catch (e) {
      err.value = `STT lỗi: ${errorMessage(e)} (có thể bỏ qua và thử lại)`;
    }
  };
  rec = r;
  r.start();
  recording.value = true;
  secs.value = 0;
  timer = window.setInterval(() => {
    if (secs.value + 1 >= MAX_SEC) stop();
    secs.value += 1;
  }, 1000);
}

const diff = computed(() => (stt.value ? wordDiff(expected.value, stt.value.transcript) : []));
const wrong = computed(() => wrongWords(diff.value));

function replay(text: string) {
  new Audio(buildTTSUrl(text)).play().catch(() => {});
}

async function saveError() {
  if (!stt.value) return;
  try {
    await postError({
      cardId: cardId.value || undefined,
      expected: expected.value,
      transcript: stt.value.transcript,
      wrong: wrong.value,
    });
    msg.value = 'Đã lưu vào sổ lỗi. Lỗi lặp sẽ được gợi ý ôn lại qua SRS.';
    try {
      suggest.value = await fetchSuggestErrors(10);
    } catch {
      /* không chặn thông báo đã lưu */
    }
  } catch (e) {
    err.value = errorMessage(e);
  }
}

async function saveAsCard() {
  if (!deckId.value) {
    err.value = 'Chọn deck để lưu từ sai thành card SRS.';
    return;
  }
  try {
    const front = wrong.value[0] ?? expected.value;
    await createCard(deckId.value, {
      front,
      back: `sai: ${stt.value?.transcript ?? ''} (mẫu: ${expected.value})`,
    });
    msg.value = `Đã tạo card SRS "${front}" — sẽ due ngày mai.`;
  } catch (e) {
    err.value = errorMessage(e);
  }
}

async function newDeck() {
  try {
    await createDeck(`Lỗi-${new Date().toISOString().slice(0, 10)}`, 'zh');
    decks.value = await fetchDecks();
  } catch (e) {
    err.value = errorMessage(e);
  }
}
</script>

<template>
  <div>
    <h2 class="text-xl font-semibold">Ghi âm &amp; Sổ lỗi</h2>
    <NoticeBar :text="err" />
    <NoticeBar v-if="msg" tone="ok" :text="msg" />
    <div class="mt-2 flex flex-wrap items-center gap-3">
      <label class="inline-flex items-center gap-1">
        <span>Câu mẫu:</span>
        <input
          v-model="expected"
          class="rounded-card border border-line px-2 py-1"
          aria-label="câu mẫu"
          style="width: 280px"
        />
      </label>
      <label class="inline-flex items-center gap-1">
        <span>card_id (tùy chọn):</span>
        <input
          v-model="cardId"
          class="rounded-card border border-line px-2 py-1"
          aria-label="card id"
          style="width: 80px"
        />
      </label>
    </div>
    <div class="mt-2">
      <button
        v-if="!recording"
        type="button"
        class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
        @click="start"
      >
        ● Ghi âm (tối đa {{ MAX_SEC }}s)
      </button>
      <button
        v-else
        type="button"
        class="rounded-card border border-err px-3 py-1"
        @click="stop"
      >
        ■ Dừng ({{ secs }}s)
      </button>
    </div>
    <section
      v-if="stt"
      class="mt-3 rounded-card border border-line bg-white p-4"
      style="max-width: 640px"
    >
      <p>
        Nghe được: <strong>{{ stt.transcript }}</strong> [{{ stt.lang }}/{{ stt.engine }}]
        <span
          data-testid="engine-badge"
          title="Engine STT đang chạy (đọc từ header X-Engine)"
          class="text-muted"
        >
          X-Engine: {{ stt.engineHeader ?? stt.engine }}
        </span>
        <template v-if="stt.confidence != null">
          (độ tin cậy {{ stt.confidence.toFixed(2) }})</template
        >
      </p>
      <p class="mt-1">
        So với mẫu (đúng {{ Math.round(diffScore(expected, diff) * 100) }}%):
        <span
          v-for="(t, i) in diff"
          :key="i"
          :class="[statusClass(t.status), t.status === 'ok' ? '' : 'font-bold']"
        >
          {{ t.text }} </span
        >
      </p>
      <p class="mt-1 text-[13px] text-muted">
        Đỏ = đọc sai, cam = thiếu, xám = thừa. Bấm từ sai để nghe lại mẫu:
      </p>
      <div class="mt-1 flex flex-wrap gap-2">
        <button
          v-for="(w, i) in wrong"
          :key="i"
          type="button"
          class="rounded-card border border-err px-2 py-1 text-err"
          @click="replay(w.split('→')[0])"
        >
          🔊 {{ w }}
        </button>
      </div>
      <div class="mt-2 flex flex-wrap items-center gap-2">
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="saveError"
        >
          Lưu vào sổ lỗi
        </button>
        <DeckSelect
          v-model="deckId"
          :decks="decks"
          label="deck lưu card"
          placeholder="— deck lưu card —"
        />
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="saveAsCard"
        >
          Tạo card SRS từ lỗi
        </button>
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="newDeck"
        >
          + Deck mới
        </button>
      </div>
    </section>
    <section v-if="suggest.length > 0" class="mt-4">
      <h3 class="font-semibold">Gợi ý ôn lại (lỗi lặp nhiều nhất)</h3>
      <ul class="mt-2 list-disc pl-5">
        <li v-for="s in suggest" :key="s.cardId">
          {{ s.front }} — {{ s.back }} ({{ s.errors }} lỗi)
          <RouterLink class="underline" to="/review">ôn ngay</RouterLink>
        </li>
      </ul>
    </section>
  </div>
</template>
