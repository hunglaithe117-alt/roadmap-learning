<script setup lang="ts">
// M6 — port từ `routes/ErrorBook.tsx` + phục hồi BỘ LỌC THEO THẺ (mất khi port).
//
// `errors(cardId:, limit:)` đã hỗ trợ lọc theo thẻ từ M4 nhưng `ErrorBook` luôn
// gọi `fetchErrors(undefined, 50)` ⇒ không ai lọc được. Bộ lọc ở đây chỉ dùng
// danh sách thẻ client đang có (deck HSK) + ô nhập id thẻ tự do: lỗi gắn thẻ có
// thể thuộc bất kỳ deck nào, kể cả deck đã xoá, nên ô tự do là đường thoát
// duy nhất không giả định sai.
//
// `topErrors` đổi sang `insightTopErrors`: query cũ không có `cardId` nên dòng
// từ sai không nhảy được tới thẻ (M6a §5.7).
import { computed, onMounted, ref } from 'vue';
import NoticeBar from '../components/NoticeBar.vue';
import Badge from '../components/ui/Badge.vue';
import Button from '../components/ui/Button.vue';
import Select from '../components/ui/Select.vue';
import {
  fetchErrors,
  fetchSuggestErrors,
  type ErrorEntry,
  type SuggestCard,
} from '../player/api';
import { fetchInsightTopErrors, type TopErrorWithCard } from '../player/insight';
import { fetchDecks, fetchCards, type Card, type Deck } from '../srs/api';
import { errorMessage } from '../graphql/errors';

const entries = ref<ErrorEntry[]>([]);
const top = ref<TopErrorWithCard[]>([]);
const suggest = ref<SuggestCard[]>([]);
const err = ref('');
const busy = ref(true);

const decks = ref<Deck[]>([]);
const cards = ref<Card[]>([]);
const deckId = ref('');
const cardId = ref('');

const activeCardId = computed(() => (cardId.value.trim() === '' ? undefined : cardId.value.trim()));

onMounted(async () => {
  try {
    decks.value = await fetchDecks();
  } catch (e) {
    err.value = errorMessage(e);
  }
  await load();
});

async function load(): Promise<void> {
  busy.value = true;
  try {
    const [e, t, s] = await Promise.all([
      fetchErrors(activeCardId.value, 50),
      fetchInsightTopErrors(10),
      fetchSuggestErrors(10),
    ]);
    entries.value = e;
    top.value = t;
    suggest.value = s;
    err.value = '';
  } catch (e) {
    err.value = errorMessage(e);
  } finally {
    busy.value = false;
  }
}

async function onDeckChange(): Promise<void> {
  if (deckId.value === '') {
    cards.value = [];
    return;
  }
  try {
    cards.value = await fetchCards(deckId.value);
  } catch (e) {
    err.value = errorMessage(e);
  }
}

function applyFilter(): void {
  void load();
}

function clearFilter(): void {
  cardId.value = '';
  applyFilter();
}
</script>

<template>
  <div>
    <h2 class="font-display text-2xl font-semibold text-ink">Sổ lỗi</h2>
    <NoticeBar :text="err" />

    <!-- Bộ lọc theo thẻ. `cardId` null = không lọc, khớp `Query.errors`. -->
    <section class="mt-3 flex flex-wrap items-end gap-2 rounded-card border border-line bg-surface p-3">
      <label class="inline-flex flex-col gap-1 text-[13px] font-semibold text-ink">
        Deck
        <Select
          v-model="deckId"
          class="w-40"
          aria-label="deck để chọn thẻ"
          @change="onDeckChange"
        >
          <option value="">— không chọn —</option>
          <option v-for="d in decks" :key="d.id" :value="d.id">{{ d.name }} [{{ d.lang }}]</option>
        </Select>
      </label>
      <label class="inline-flex flex-col gap-1 text-[13px] font-semibold text-ink">
        Thẻ
        <Select v-model="cardId" class="w-56" aria-label="lọc lỗi theo thẻ" @change="applyFilter">
          <option value="">Tất cả thẻ</option>
          <option v-for="c in cards" :key="c.id" :value="c.id">{{ c.front }} — {{ c.back }}</option>
        </Select>
      </label>
      <label class="inline-flex flex-col gap-1 text-[13px] font-semibold text-ink">
        Nhập id thẻ
        <input
          v-model="cardId"
          class="h-9 w-32 rounded-md border border-input bg-surface px-2 text-sm"
          aria-label="lọc lỗi theo id thẻ"
          inputmode="numeric"
          placeholder="mọi deck"
          @keyup.enter="applyFilter"
        />
      </label>
      <Button size="sm" :disabled="busy" @click="applyFilter">Lọc</Button>
      <Button size="sm" variant="ghost" @click="clearFilter">Bỏ lọc</Button>
      <p class="text-[13px] text-muted-foreground">
        {{
          activeCardId
            ? `Đang lọc theo thẻ ${activeCardId}.`
            : 'Chưa lọc — xem lỗi của mọi thẻ.'
        }}
      </p>
    </section>

    <p v-if="busy" class="mt-3 text-muted">đang tải…</p>

    <template v-else>
      <section v-if="top.length > 0" class="mt-4">
        <h3 class="text-[16px] font-semibold text-ink">Từ sai nhiều nhất ({{ top.length }})</h3>
        <ul class="stagger mt-2 grid gap-1.5">
          <li
            v-for="t in top"
            :key="t.word"
            :data-testid="`top-error-${t.word}`"
            class="flex flex-wrap items-baseline gap-2"
          >
            <span class="font-medium text-ink">{{ t.word }}</span>
            <span class="tabular text-[13px] text-muted-foreground">sai {{ t.count }} lần</span>
            <!-- `cardId` NULL khi lỗi luyện tự do hoặc thẻ đã xoá mềm ⇒
                 dòng tĩnh, KHÔNG render nút nhảy tới thẻ không tồn tại. -->
            <template v-if="t.cardId">
              <span class="text-[13px] text-muted-foreground">mặt trước: {{ t.front }}</span>
              <RouterLink
                class="text-[13px] underline"
                :data-testid="`top-error-review-${t.word}`"
                :to="`/review?card=${t.cardId}`"
              >
                Ôn thẻ này
              </RouterLink>
            </template>
            <Badge v-else tone="locked">không gắn thẻ</Badge>
          </li>
        </ul>
      </section>

      <section v-if="suggest.length > 0" class="mt-4">
        <h3 class="text-[16px] font-semibold text-ink">Gợi ý ôn qua SRS</h3>
        <ul class="mt-2 space-y-1">
          <li v-for="s in suggest" :key="s.cardId" class="text-[14px]">
            {{ s.front }} — {{ s.back }} ({{ s.errors }} lỗi)
            <RouterLink class="underline" to="/review">ôn ngay</RouterLink>
          </li>
        </ul>
      </section>

      <section class="mt-4">
        <h3 class="text-[16px] font-semibold text-ink">Lịch sử lỗi ({{ entries.length }})</h3>
        <p v-if="entries.length === 0" class="mt-1 text-muted">
          Không có lỗi nào khớp bộ lọc.
        </p>
        <ul v-else class="stagger mt-2 space-y-1">
          <li v-for="e in entries" :key="e.id" :data-testid="`error-row-${e.id}`">
            mẫu: <strong>{{ e.expected }}</strong> → nghe: {{ e.transcript }}
            <template v-if="e.wrong.length > 0"> — sai: {{ e.wrong.join(', ') }}</template>
            <template v-if="e.cardId != null"> (thẻ {{ e.cardId }})</template>
          </li>
        </ul>
      </section>
    </template>
  </div>
</template>
