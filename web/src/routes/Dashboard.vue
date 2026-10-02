<script setup lang="ts">
// M5 — port từ `routes/Dashboard.tsx`.
// `Stats` đổi tên field camelCase theo schema (`doneWindow`, `totalAll`,
// `dueNow`) — số liệu và cách tính giữ nguyên hành vi v1.
import { onMounted, ref, watch } from 'vue';
import NoticeBar from '../components/NoticeBar.vue';
import { errorMessage } from '../graphql/errors';
import { markVisited, shouldRemind, ensureNotificationPermission } from '../streak';
import type { Stats, TopError } from '../player/api';
import { fetchStats, fetchTopErrors } from '../player/api';

const range = ref<'week' | 'month'>('week');
const stats = ref<Stats | null>(null);
const top = ref<TopError[]>([]);
const err = ref('');
const remind = ref(false);
const notifState = ref('');

onMounted(() => {
  // F4: THỨ TỰ QUAN TRỌNG. `shouldRemind()` = `giờ >= 20 && !visitedToday()`.
  // Gọi `markVisited()` TRƯỚC sẽ đánh dấu "hôm nay đã ghé" rồi `shouldRemind()`
  // đọc lại chính dấu đó ⇒ luôn `false` ⇒ banner nhắc học 20:00 chết vĩnh viễn
  // (`<p v-if="remind">` là code chết). Phải HỎI trước, ĐÁNH DẤU sau.
  remind.value = shouldRemind();
  markVisited();
});

watch(
  range,
  async () => {
    try {
      const [s, t] = await Promise.all([fetchStats(range.value), fetchTopErrors(10)]);
      stats.value = s;
      top.value = t;
    } catch (e) {
      err.value = errorMessage(e);
    }
  },
  { immediate: true },
);

async function enableReminder() {
  const ok = await ensureNotificationPermission();
  if (ok) {
    notifState.value = 'Đã bật nhắc học lúc 20:00.';
    try {
      new Notification('Lang Learn', { body: 'Nhắc học đã bật — hẹn gặp lúc 20:00!' });
    } catch {
      /* có quyền nhưng tạo thất bại thì vẫn còn badge trong-app */
    }
  } else {
    notifState.value = 'Browser chặn/không hỗ trợ Notification — sẽ nhắc bằng badge trong-app.';
  }
}
</script>

<template>
  <div>
    <h2 class="text-xl font-semibold">Dashboard</h2>
    <NoticeBar :text="err" />
    <p
      v-if="remind"
      class="mt-3 rounded-card border border-warn bg-remind px-2 py-2"
    >
      ⏰ Hôm nay chưa ôn SRS — dành ít phút ôn bài nhé! (fallback nhắc trong-app)
    </p>
    <div class="mt-3 flex flex-wrap items-center gap-3">
      <button
        type="button"
        class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
        :class="{ 'font-bold': range === 'week' }"
        @click="range = 'week'"
      >
        Tuần
      </button>
      <button
        type="button"
        class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
        :class="{ 'font-bold': range === 'month' }"
        @click="range = 'month'"
      >
        Tháng
      </button>
      <span>🔥 Streak: {{ stats ? `${stats.streak} ngày` : '…' }}</span>
      <button
        type="button"
        class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
        @click="enableReminder"
      >
        Bật nhắc học 20:00
      </button>
      <span v-if="notifState" class="text-muted">{{ notifState }}</span>
    </div>
    <section v-if="stats" class="mt-3 rounded-card border border-line p-4" style="max-width: 560px">
      <h3 class="font-semibold">
        Tiến độ {{ stats.range === 'week' ? 'tuần' : 'tháng' }} ({{ stats.days }} ngày)
      </h3>
      <ul class="mt-2 list-disc pl-5">
        <li>Đã ôn (tuần/tháng này): {{ stats.doneWindow }}/{{ stats.totalAll }} thẻ</li>
        <li>Đến hạn ngay: {{ stats.dueNow }} thẻ</li>
        <li>Độ chính xác: {{ Math.round(stats.accuracy * 100) }}%</li>
      </ul>
    </section>
    <section v-if="top.length > 0" class="mt-3">
      <h3 class="font-semibold">Top lỗi lặp</h3>
      <ol class="mt-2 list-decimal pl-5">
        <li v-for="t in top" :key="t.word">{{ t.word }} — sai {{ t.count }} lần</li>
      </ol>
    </section>
    <p class="mt-3">
      <RouterLink class="underline" to="/review">Ôn ngay →</RouterLink>
    </p>
  </div>
</template>
