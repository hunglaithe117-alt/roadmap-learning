<script setup lang="ts">
/**
 * App shell: tiêu đề + thanh 14 màn + `<RouterView>`.
 *
 * `RouterLink` (không phải `<a href="#/...">` như v1) vì `createWebHashHistory`
 * tự sinh đúng `href="#/x"` — viết tay dễ quên `?` escape khi có query
 * (`#/review?deck=3`).
 */
import { ROUTES } from './router';

defineOptions({ name: 'App' });
</script>

<template>
  <main class="p-6 font-sans">
    <h1 class="text-2xl font-semibold">Lang Learn</h1>
    <nav class="my-4 flex flex-wrap gap-3" aria-label="các màn">
      <RouterLink
        v-for="r in ROUTES"
        :key="r.path"
        :to="r.path"
        class="hover:underline"
        active-class="font-bold"
      >
        {{ r.label }}
      </RouterLink>
    </nav>
    <hr class="mb-4 border-line-soft" />
    <RouterView />
  </main>
</template>
