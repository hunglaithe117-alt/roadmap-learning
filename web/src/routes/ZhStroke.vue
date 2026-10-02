<script setup lang="ts">
// M5 — port từ `routes/ZhStroke.tsx`: tập viết nét trên canvas.
// Dữ liệu nét lazy-load theo level (index nhẹ → chi tiết từng chữ). Chấm
// đúng/sai: hướng nét vẽ → mã nét → so thứ tự với chuẩn (`chinese/strokes`).
//
// Canvas giữ nguyên API 2D của bản v1: framework mới không thay được cách vẽ,
// chỉ thay cách gắn handler. `ref` bên dưới giữ trạng thái chỉ đọc trong
// handler (điểm đang vẽ, cờ đang vẽ) — không kích hoạt render nên `ref` là
// đủ và rẻ hơn.
import { onMounted, ref, useTemplateRef, watch } from 'vue';
import DeckSelect from '../components/DeckSelect.vue';
import NoticeBar from '../components/NoticeBar.vue';
import {
  loadStrokeDetail,
  loadStrokeIndex,
  type StrokeDetail,
} from '../chinese/api';
import {
  classifyStroke,
  expectedCodes,
  findCardForHanzi,
  gradeStrokes,
  type Point,
} from '../chinese/strokes';
import { errorMessage } from '../graphql/errors';
import { fetchCards, fetchDecks, postReview, type Deck } from '../srs/api';

const chars = ref<Array<{ hanzi: string; strokeCount: number }>>([]);
const hanzi = ref('人');
const detail = ref<StrokeDetail | null>(null);
const drawn = ref<string[]>([]);
const msg = ref('');
const err = ref('');
const decks = ref<Deck[]>([]);
const deckId = ref('');

const canvas = useTemplateRef<HTMLCanvasElement>('canvas');
const currentStroke = ref<Point[]>([]);
const drawing = ref(false);

function clearCanvas() {
  const cv = canvas.value;
  if (!cv) return;
  const ctx = cv.getContext('2d');
  if (!ctx) return;
  ctx.clearRect(0, 0, cv.width, cv.height);
  ctx.strokeStyle = '#eeeeee';
  ctx.strokeRect(4.5, 4.5, cv.width - 9, cv.height - 9);
}

function canvasPos(e: PointerEvent): Point {
  const rect = canvas.value!.getBoundingClientRect();
  return { x: e.clientX - rect.left, y: e.clientY - rect.top };
}

function onDown(e: PointerEvent) {
  drawing.value = true;
  currentStroke.value = [canvasPos(e)];
  (e.target as HTMLElement).setPointerCapture?.(e.pointerId);
}

function onMove(e: PointerEvent) {
  if (!drawing.value) return;
  const p = canvasPos(e);
  const stroke = currentStroke.value;
  const prev = stroke[stroke.length - 1];
  stroke.push(p);
  currentStroke.value = stroke;
  const ctx = canvas.value?.getContext('2d');
  if (ctx && prev) {
    ctx.strokeStyle = '#111111';
    ctx.lineWidth = 4;
    ctx.beginPath();
    ctx.moveTo(prev.x, prev.y);
    ctx.lineTo(p.x, p.y);
    ctx.stroke();
  }
}

function onUp() {
  if (!drawing.value) return;
  drawing.value = false;
  drawn.value = [...drawn.value, classifyStroke(currentStroke.value)];
  currentStroke.value = [];
}

onMounted(async () => {
  try {
    const r = await loadStrokeIndex();
    chars.value = r.chars;
    if (r.chars.length > 0) hanzi.value = r.chars[0].hanzi;
  } catch (e) {
    err.value = errorMessage(e);
  }
  try {
    decks.value = (await fetchDecks()).filter((d) => d.lang === 'zh');
  } catch (e) {
    err.value = errorMessage(e);
  }
});

watch(
  hanzi,
  async (h) => {
    if (!h) return;
    try {
      detail.value = await loadStrokeDetail(h);
      drawn.value = [];
      msg.value = '';
      clearCanvas();
    } catch (e) {
      err.value = errorMessage(e);
    }
  },
  { immediate: true },
);

async function grade() {
  const d = detail.value;
  if (!d) return;
  const g = gradeStrokes(expectedCodes(d), drawn.value);
  msg.value = g.pass
    ? `Đúng thứ tự nét! ${g.matches}/${g.expected} nét khớp (${Math.round(g.score * 100)}%).`
    : `Chưa đúng: khớp ${g.matches}/${g.expected} nét, bạn viết ${drawn.value.length} nét (${drawn.value.join(',') || '—'}), chuẩn (${expectedCodes(d).join(',')}). Viết lại nhé.`;
  if (g.pass && deckId.value) {
    try {
      const cards = await fetchCards(deckId.value);
      const cardId = findCardForHanzi(cards, hanzi.value);
      if (cardId != null) {
        await postReview(cardId, 4);
        msg.value += ' Đã lưu SRS.';
      }
    } catch (e) {
      err.value = errorMessage(e);
    }
  }
}
</script>

<template>
  <div>
    <h2 class="text-xl font-semibold">Tập viết nét chữ Hán</h2>
    <NoticeBar :text="err" />
    <div class="mt-2 flex flex-wrap items-center gap-3">
      <label class="inline-flex items-center gap-1">
        <span>Chữ:</span>
        <select
          v-model="hanzi"
          class="rounded-card border border-line bg-white px-2 py-1"
          aria-label="chọn chữ"
        >
          <option v-for="c in chars" :key="c.hanzi" :value="c.hanzi">
            {{ c.hanzi }} ({{ c.strokeCount }} nét)
          </option>
        </select>
      </label>
      <DeckSelect
        v-model="deckId"
        :decks="decks"
        label="Deck SRS"
        placeholder="— không lưu —"
        :show-lang="false"
      />
    </div>
    <p v-if="detail" class="mt-2">
      {{ detail.hanzi }} ({{ detail.pinyin_marks }}) — {{ detail.stroke_count }} nét:
      {{ detail.strokes.map((s) => s.name).join(' → ') }}
    </p>
    <canvas
      ref="canvas"
      width="280"
      height="280"
      class="border border-line"
      style="touch-action: none; cursor: crosshair"
      @pointerdown="onDown"
      @pointermove="onMove"
      @pointerup="onUp"
      @pointercancel="onUp"
    />
    <div class="mt-2 flex flex-wrap gap-2">
      <button
        type="button"
        class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
        @click="
          () => {
            drawn = [];
            clearCanvas();
            msg = '';
          }
        "
      >
        Viết lại
      </button>
      <button
        type="button"
        class="rounded-card border border-line px-3 py-1 hover:border-line-strong disabled:opacity-50"
        :disabled="!detail || drawn.length === 0"
        @click="grade"
      >
        Chấm ({{ drawn.length }} nét)
      </button>
    </div>
    <NoticeBar v-if="msg" tone="info" :text="msg" />
  </div>
</template>
