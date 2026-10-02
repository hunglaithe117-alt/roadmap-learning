<script setup lang="ts">
// M5 — port từ `routes/ZhPinyin.tsx`: 1 vòng drill = nghe audio mẫu → gõ thanh
// điệu ("3 3") → `gradeTone` chấm → `recordReview` lưu SRS.
// M6 — phục hồi nút nạp HSK + chế độ chấm cả vòng (mất lúc port React→Vue).
import { computed, onMounted, ref } from 'vue';
import Button from '../components/ui/Button.vue';
import DeckSelect from '../components/DeckSelect.vue';
import NoticeBar from '../components/NoticeBar.vue';
import ToneCard from '../components/ToneCard.vue';
import {
  answerDrill,
  loadDrillRound,
  postZhDrillGrade,
  postZhImport,
  type DrillItem,
  type ImportReport,
} from '../chinese/api';
import { remainingItems, roundGrade, summarizeRound } from '../chinese/drill';
import { errorMessage } from '../graphql/errors';
import { fetchDecks, postReview, type Deck } from '../srs/api';

// ── chế độ chấm ────────────────────────────────────────────────────────────
//
// SRS hiện tại chấm TỪNG THẺ: mỗi câu trả lời là 1 lần `recordReview`, nên 1
// lần sai đủ kéo thẻ về hàng đợi ngay — đúng với nghiệp vụ "ôn riêng 1 thẻ"
// của app v1. Đó là MẶC ĐỊNH và giữ nguyên.
//
// Chấm CẢ VÒNG là chế độ thứ hai: cả vòng điểm mới quyết định grade, rồi ghi
// 1 lần cho mỗi thẻ. Nó dành cho lượt nghe một mạch không cần biết ngay thẻ nào
// hỏng. Chọn qua UI, KHÔNG thay mặc định — bật/tắt được và nhớ lựa chọn.
type GradeMode = 'card' | 'round';
const MODE_KEY = 'zh-pinyin:grade-mode';

function readMode(): GradeMode {
  try {
    return localStorage.getItem(MODE_KEY) === 'round' ? 'round' : 'card';
  } catch {
    return 'card';
  }
}

const decks = ref<Deck[]>([]);
const deckId = ref('');
const items = ref<DrillItem[]>([]);
const idx = ref(0);
const input = ref('');
const msg = ref('');
const err = ref('');
const done = ref<boolean[]>([]);
const mode = ref<GradeMode>(readMode());
/** Trong chế độ cả vòng: id các thẻ đã trả lời — nguồn cho `remainingItems`. */
const answeredIds = ref<string[]>([]);
const roundSaved = ref(false);
const importing = ref(false);
const importReport = ref<ImportReport | null>(null);
const importLevel = ref('HSK1');
/** Kho tên deck HSK đã nạp, để nút import không tạo deck trùng mỗi lần bấm. */
const deckNames = computed(() => new Set(decks.value.map((d) => d.name)));

onMounted(async () => {
  try {
    decks.value = (await fetchDecks()).filter((d) => d.lang === 'zh');
  } catch (e) {
    err.value = errorMessage(e);
  }
});

/** Nạp 1 level HSK vào deck cùng tên. Idempotent ở server (M4 `importHSK`). */
async function importHsk() {
  importing.value = true;
  err.value = '';
  try {
    importReport.value = await postZhImport(importLevel.value, importLevel.value);
    decks.value = (await fetchDecks()).filter((d) => d.lang === 'zh');
    msg.value = `Đã nạp ${importLevel.value}: +${importReport.value.cardsAdded} thẻ, +${importReport.value.dictAdded} mục từ điển.`;
  } catch (e) {
    err.value = errorMessage(e);
  } finally {
    importing.value = false;
  }
}

async function start() {
  if (!deckId.value) return;
  try {
    const round = await loadDrillRound(deckId.value);
    items.value = round;
    idx.value = 0;
    done.value = [];
    answeredIds.value = [];
    roundSaved.value = false;
    msg.value =
      round.length === 0 ? 'Deck chưa có dữ liệu thanh điệu — nạp HSK1 ở nút bên cạnh trước.' : '';
    err.value = '';
  } catch (e) {
    err.value = errorMessage(e);
  }
}

function setMode(next: GradeMode) {
  mode.value = next;
  try {
    localStorage.setItem(MODE_KEY, next);
  } catch {
    /* storage unavailable */
  }
}

/** Chấm 1 câu nhưng CHƯA ghi SRS — cả 2 chế độ dùng chung bước này. */
async function answer() {
  const item = items.value[idx.value];
  if (!item || !input.value.trim()) return;
  try {
    if (mode.value === 'card') {
      const { grade } = await answerDrill(item, input.value.trim());
      done.value = [...done.value, grade.correct];
      msg.value = grade.correct
        ? `Đúng! ${item.hanzi} = ${item.pinyin_marks} (${item.pair}) — đã lưu SRS (grade ${grade.grade}).`
        : `Chưa đúng. ${item.hanzi} = ${item.pinyin_marks} (${item.pair}) — đã lưu SRS (grade ${grade.grade}).`;
    } else {
      const grade = await postZhDrillGrade(item.tone || item.pair, input.value.trim());
      done.value = [...done.value, grade.correct];
      answeredIds.value = [...answeredIds.value, item.card_id];
      msg.value = grade.correct
        ? `Đúng! ${item.hanzi} = ${item.pinyin_marks} (${item.pair}).`
        : `Chưa đúng. ${item.hanzi} = ${item.pinyin_marks} (${item.pair}).`;
    }
    input.value = '';
    idx.value += 1;
    err.value = '';
  } catch (e) {
    err.value = errorMessage(e);
  }
}

/**
 * Ghi điểm cả vòng: 1 grade duy nhất tính từ tỉ lệ đúng, rồi `recordReview` cho
 * TỪNG thẻ trong vòng. Thẻ chưa làm (người dùng bỏ dở) không ghi — `remainingItems`
 * lọc đúng các thẻ đó, nếu không sẽ ghi grade cho thẻ chưa từng trả lời.
 */
const savingRound = ref(false);

async function saveRound() {
  savingRound.value = true;
  err.value = '';
  try {
    const grade = roundGrade(summary.value.pct);
    // `remainingItems` là nơi duy nhất biết thẻ nào CHƯA làm. Chấm thẻ chưa
    // từng trả lời thì ghi SRS vô nghĩa, nên chỉ ghi phần đã làm.
    const done = new Set(remainingItems(items.value, answeredIds.value).map((i) => i.card_id));
    for (const item of items.value) {
      if (done.has(item.card_id)) continue;
      await postReview(item.card_id, grade);
    }
    roundSaved.value = true;
    msg.value = `Cả vòng: ${summary.value.correct}/${summary.value.total} đúng — mọi thẻ trong vòng lưu SRS với grade ${grade}.`;
  } catch (e) {
    err.value = errorMessage(e);
  } finally {
    savingRound.value = false;
  }
}

const current = computed(() => items.value[idx.value]);
const summary = computed(() => summarizeRound(done.value));
const finished = computed(() => items.value.length > 0 && idx.value >= items.value.length);
const needSave = computed(() => mode.value === 'round' && finished.value && !roundSaved.value);
const alreadyImported = computed(() => deckNames.value.has(importLevel.value));
</script>

<template>
  <div>
    <h2 class="text-xl font-semibold">Luyện thanh điệu (Pinyin Drill)</h2>
    <NoticeBar :text="err" />
    <div class="mt-2 flex flex-wrap items-center gap-3">
      <DeckSelect v-model="deckId" :decks="decks" label="Deck" :show-lang="false" />
      <button
        type="button"
        class="rounded-card border border-line px-3 py-1 hover:border-line-strong disabled:opacity-50"
        :disabled="!deckId"
        @click="start"
      >
        Bắt đầu 1 vòng
      </button>
    </div>
    <!-- Nút nạp HSK: `postZhImport` đã có hàm + test từ M5 nhưng KHÔNG màn
         nào gọi. Deck rỗng là ngõ cụt của màn này, và câu "nạp HSK1 trước"
         không kèm nút thì người dùng không đi được. -->
    <div class="mt-4 rounded-card border border-line bg-surface p-3">
      <h3 class="text-[13px] font-semibold text-ink">Chưa có dữ liệu? Nạp từ vựng HSK</h3>
      <p class="mt-0.5 text-[13px] text-muted-foreground">
        Nạp 1 level HSK vào deck cùng tên. Chạy lại không tạo bản trùng.
      </p>
      <div class="mt-2 flex flex-wrap items-center gap-2">
        <input
          v-model="importLevel"
          class="h-9 w-24 rounded-md border border-input bg-surface px-2 text-sm"
          aria-label="level HSK cần nạp"
          placeholder="HSK1"
        />
        <Button size="sm" :disabled="importing" @click="importHsk">
          {{ importing ? 'Đang nạp…' : `Nạp ${importLevel}` }}
        </Button>
        <span v-if="alreadyImported && !importing" class="text-[13px] text-ok">
          Deck {{ importLevel }} đã có.
        </span>
      </div>
      <p v-if="importReport" class="tabular mt-1.5 text-[13px] text-muted-foreground">
        Deck {{ importReport.deck }} · tổng {{ importReport.cardsTotal }} thẻ · từ điển +{{
          importReport.dictAdded
        }}
      </p>
    </div>

    <div class="mt-4 flex flex-wrap items-center gap-3 rounded-card border border-line bg-surface px-3 py-2">
      <span class="text-[13px] font-semibold text-ink">Chấm điểm:</span>
      <label class="inline-flex items-center gap-1.5 text-[13px]">
        <input
          type="radio"
          name="grade-mode"
          value="card"
          :checked="mode === 'card'"
          aria-label="chấm từng thẻ"
          @change="setMode('card')"
        />
        Từng thẻ
      </label>
      <label class="inline-flex items-center gap-1.5 text-[13px]">
        <input
          type="radio"
          name="grade-mode"
          value="round"
          :checked="mode === 'round'"
          aria-label="chấm cả vòng"
          @change="setMode('round')"
        />
        Cả vòng
      </label>
      <span class="text-[13px] text-muted-foreground">
        {{
          mode === 'card'
            ? 'Mỗi câu lưu SRS ngay — thẻ sai kéo về hàng đợi luôn.'
            : 'Xong cả vòng mới lưu, dùng 1 grade chung cho mọi thẻ.'
        }}
      </span>
    </div>

    <NoticeBar v-if="msg" tone="info" :text="msg" />
    <div v-if="!finished && current" class="mt-3">
      <ToneCard :item="current" />
      <p class="mt-2">
        Câu {{ idx + 1 }}/{{ items.length }} — Nghe rồi gõ thanh điệu (VD: 3 3):
      </p>
      <div class="mt-1 flex items-center gap-2">
        <input
          v-model="input"
          class="rounded-card border border-line px-2 py-1"
          placeholder="3 3"
          aria-label="thanh điệu bạn nghe"
        />
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="answer"
        >
          Chấm điểm
        </button>
      </div>
    </div>
    <div v-if="finished" class="mt-3">
      <p>
        Xong 1 vòng: đúng {{ summary.correct }}/{{ summary.total }}
        ({{ Math.round(summary.pct * 100) }}%).
        <template v-if="mode === 'card' || roundSaved">Điểm đã lưu SRS.</template>
      </p>
      <Button v-if="needSave" class="mt-2" :disabled="savingRound" @click="saveRound">
        {{ savingRound ? 'Đang lưu…' : 'Lưu điểm cả vòng' }}
      </Button>
    </div>
  </div>
</template>
