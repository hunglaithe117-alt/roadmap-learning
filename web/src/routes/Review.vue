<script setup lang="ts">
// M5 — port từ `routes/Review.tsx`.
// `#/review?deck=<id>` vẫn chạy y như v1: `useRoute().query.deck` đọc đúng
// query string đó, và `watch` để đổi deck qua link "ôn ngay" ở màn khác có
// đổi hàng đợi ngay (v1 chỉ đọc hash 1 lần lúc mount).
//
// M6 remediation (F2) — `?card=<id>`: `ErrorBook` render link
// `/review?card=${cardId}` cho từ sai nhiều nhất, nhưng màn này trước đó
// **chỉ đọc `query.deck`** ⇒ bấm "Ôn thẻ này" ra màn trắng "Chọc deck để bắt
// đầu ôn": không lỗi, không thẻ. Nay `cardFromQuery` được đọc, thẻ đó được
// đặt LÊN ĐẦU hàng đợi, và `deckId` tự chuyển sang deck của nó (không có
// bước này thì `v-if="!deckId"` vẫn giữ màn trắng dù đã có thẻ).
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import DeckSelect from '../components/DeckSelect.vue';
import NoticeBar from '../components/NoticeBar.vue';
import SrsCard from '../components/SrsCard.vue';
import { errorMessage } from '../graphql/errors';
import type { Card, Deck, ReviewResult } from '../srs/api';
import { fetchCardById, fetchDecks, fetchDueCards, postReview } from '../srs/api';

const route = useRoute();

/** `?deck=3` → `'3'`; thiếu/tham số rác → `''` (tương đương `null` của v1). */
const deckFromQuery = computed(() => singleParam(route.query.deck));

/**
 * `?card=7` → `'7'`. Cùng hình dạng với `deckFromQuery` — `vue-router` cho
 * `query` là `string | string[] | null`, mà link do `ErrorBook` dựng luôn là
 * 1 chuỗi; xử luôn cả mảng để query tay gõ không làm hỏng màn.
 */
const cardFromQuery = computed(() => singleParam(route.query.card));

function singleParam(raw: unknown): string {
  const v = Array.isArray(raw) ? raw[0] : raw;
  return typeof v === 'string' && v !== '' ? v : '';
}

const decks = ref<Deck[]>([]);
const deckId = ref(deckFromQuery.value);
const queue = ref<Card[]>([]);
const showBack = ref(false);
const last = ref<ReviewResult | null>(null);
const err = ref('');

/** Thẻ `?card=` — đặt lên ĐẦU hàng đợi, kể cả khi nó không đến hạn. */
const focusCard = ref<Card | null>(null);

watch(deckFromQuery, (v) => {
  deckId.value = v;
});

async function loadDecks() {
  try {
    decks.value = await fetchDecks();
  } catch (e) {
    err.value = errorMessage(e);
  }
}

/** `?card=` lên đầu, bỏ trùng nếu nó vốn đã nằm trong hàng đợi đến hạn. */
function withFocus(due: Card[]): Card[] {
  const f = focusCard.value;
  if (!f) return due;
  return [f, ...due.filter((c) => c.id !== f.id)];
}

async function loadDue(id: string) {
  if (!id) return;
  // Đổi deck tay ⇒ thẻ `?card=` thuộc deck cũ không còn ý nghĩa ở đầu hàng
  // đợi. Kiểm ở đây (trong `loadDue`) chứ không ở `watch(deckId)` để nhánh
  // `loadFocus` tự set `deckId` không tự xoá chính thẻ nó vừa tìm.
  if (focusCard.value && focusCard.value.deckId !== id) focusCard.value = null;
  try {
    queue.value = withFocus(await fetchDueCards(id));
    showBack.value = false;
    last.value = null;
    err.value = '';
  } catch (e) {
    err.value = errorMessage(e);
  }
}

/**
 * Nạp thẻ của `?card=`. Thẻ đã bị xoá mềm (hoặc id rác) ⇒ báo lỗi rõ ràng
 * thay vì im lặng rơi về "Chọc deck" — đó chính là loại lỗi im lặng khiến
 * `ErrorBook` trông như có tính năng nhưng không chạy.
 */
async function loadFocus(id: string): Promise<void> {
  if (!id) {
    focusCard.value = null;
    return;
  }
  try {
    const card = await fetchCardById(id);
    if (!card) {
      focusCard.value = null;
      err.value = 'Không tìm thấy thẻ này — có thể thẻ đã bị xoá.';
      return;
    }
    focusCard.value = card;
    showBack.value = false;
    last.value = null;
    if (card.deckId === deckId.value) {
      // Deck đã đúng: không gọi lại `fetchDueCards` (cache-first cũng rẻ, nhưng
      // 2 lần ghi `queue` là 2 lần lấy đè không cần). Chỉ đặt lại thẻ lên đầu.
      queue.value = withFocus(queue.value);
      err.value = '';
    } else {
      // Khác ⇒ gán `deckId` để `watch(deckId)` tự nạp hàng đợi (giữ focus).
      deckId.value = card.deckId;
    }
  } catch (e) {
    err.value = errorMessage(e);
  }
}

// `immediate` nạp hàng đợi ngay khi hash đã có `?deck=` (đúng hành vi v1: mount
// rồi tự tải). `loadDecks` để trong `onMounted` chứ không gọi ở thân setup —
// `mount` chạy setup trước nên gọi trực tiếp sẽ chạy ngoài vòng đời component.
watch(deckId, (v) => loadDue(v), { immediate: true });
watch(cardFromQuery, (v) => void loadFocus(v), { immediate: true });
onMounted(loadDecks);

const current = computed(() => queue.value[0]);

async function grade(g: number) {
  const card = current.value;
  if (!card) return;
  try {
    last.value = await postReview(card.id, g);
    queue.value = queue.value.slice(1);
    showBack.value = false;
  } catch (e) {
    err.value = errorMessage(e);
  }
}
</script>

<template>
  <div>
    <h2 class="text-xl font-semibold">Review</h2>
    <NoticeBar :text="err" />
    <div class="mt-2 flex flex-wrap items-center gap-3">
      <DeckSelect v-model="deckId" :decks="decks" label="Deck" />
      <button
        v-if="deckId"
        type="button"
        class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
        @click="loadDue(deckId)"
      >
        Tải thẻ đến hạn
      </button>
    </div>
    <p v-if="!deckId" class="mt-3 text-muted">Chọn deck để bắt đầu ôn.</p>
    <p v-else-if="!current" class="mt-3 text-muted">Hết bài hôm nay 🎉</p>
    <SrsCard
      v-if="current"
      class="mt-3"
      :card="current"
      :show-back="showBack"
      @reveal="showBack = true"
      @grade="grade"
    />
    <NoticeBar
      v-if="last"
      tone="ok"
      :text="`Đã lưu. Thẻ tiếp theo đến hạn sau ${last.intervalDays} ngày (${last.dueAt}).`"
    />
  </div>
</template>
