<script setup lang="ts">
// M5 — bảng tiến bộ 8 trục + chuỗi điểm mỗi trục (oldest-first, như v1).
import { THIEU_CODES, thieuTrend, type ThieuSession } from '../english/thieu';

const props = defineProps<{ history: ThieuSession[] }>();
</script>

<template>
  <p v-if="props.history.length === 0">Chưa có lịch sử checklist.</p>
  <section v-else>
    <h3 class="font-semibold">Tiến bộ 8 trục</h3>
    <table class="mt-2 border-collapse text-sm">
      <thead>
        <tr>
          <th class="border border-line-soft px-2 py-1">Buổi</th>
          <th v-for="c in THIEU_CODES" :key="c" class="border border-line-soft px-2 py-1">
            {{ c }}
          </th>
          <th class="border border-line-soft px-2 py-1">TB</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="s in props.history" :key="s.id">
          <td class="border border-line-soft px-2 py-1">{{ s.session }}</td>
          <td
            v-for="c in THIEU_CODES"
            :key="c"
            class="border border-line-soft px-2 py-1"
          >
            {{ s.scores[c] ?? '–' }}
          </td>
          <td class="border border-line-soft px-2 py-1">{{ s.average.toFixed(2) }}</td>
        </tr>
      </tbody>
    </table>
    <p v-for="c in THIEU_CODES" :key="c">Trục {{ c }}: {{ thieuTrend(props.history)[c].join(' → ') }}</p>
  </section>
</template>
