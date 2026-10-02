<script setup lang="ts">
// M5 — port từ `routes/EnThieu.tsx`: tải 8 trục + lịch sử, checklist lưu buổi học.
import { onMounted, ref } from 'vue';
import NoticeBar from '../components/NoticeBar.vue';
import ThieuChart from '../components/ThieuChart.vue';
import ThieuChecklist from '../components/ThieuChecklist.vue';
import { fetchThieuAxes, fetchThieuHistory } from '../english/api';
import type { ThieuAxis, ThieuSession } from '../english/thieu';
import { errorMessage } from '../graphql/errors';

const axes = ref<ThieuAxis[]>([]);
const history = ref<ThieuSession[]>([]);
const err = ref('');

onMounted(async () => {
  try {
    const [a, h] = await Promise.all([fetchThieuAxes(), fetchThieuHistory()]);
    axes.value = a;
    history.value = h;
    err.value = '';
  } catch (e) {
    err.value = errorMessage(e);
  }
});
</script>

<template>
  <div>
    <h2 class="text-xl font-semibold">English — Checklist THIEU 8 trục</h2>
    <NoticeBar :text="err" />
    <ThieuChecklist class="mt-4" :axes="axes" @saved="(s) => (history = [s, ...history])" />
    <ThieuChart class="mt-6" :history="history" />
  </div>
</template>
