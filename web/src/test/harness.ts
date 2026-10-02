// Hạ tầng test component: mount màn Vue với đúng plugin mà `main.ts` cài.
//
// VÌ SAO cần: 3 màn dùng `useRoute()` (vue-router), 2 màn dùng
// `useQuery`/`useMutation` (vue-query). Mount thiếu plugin ⇒ `useRoute()` trả
// `undefined` và vue-query ném "No QueryClient set". Sai lỗi này chỉ ra ở test,
// không ra ở app.
//
// `RouterLink` được stub nhưng **PHẢI render `href`** (M6 remediation F2).
// Stub `<a><slot/></a>` rỗng khiến mọi assert về link là assert về vô nghĩa:
// `RestoredFeatures.test.ts` từng xanh vì chỉ kiểm text "Ôn thẻ này" trong khi
// `/review?card=…` không ai đọc — link chết vẫn xanh. Đây là bẫy #1 mà
// `stack-v2-m6b.md` §8.1 tự ghi.
import { mount, type VueWrapper } from '@vue/test-utils';
import { VueQueryPlugin } from '@tanstack/vue-query';
import type { Component } from 'vue';
import { createMemoryHistory, createRouter, type Router } from 'vue-router';
import { appRoutes } from '../router';
import { queryClient } from '../rest/queryClient';

export interface MountScreenOptions {
  /** Hash đầu vào, ví dụ `/review?deck=3`. Mặc định `/hoc`. */
  route?: string;
  props?: Record<string, unknown>;
}

/** Dựng router memory + trả về, để test dùng lại cho `router.push` nữa. */
export async function makeTestRouter(route = '/hoc'): Promise<Router> {
  const router = createRouter({ history: createMemoryHistory(), routes: appRoutes() });
  await router.push(route);
  await router.isReady();
  return router;
}

/** Wrapper đã mount — là kiểu mà test dùng (`wrapper.text()`, `wrapper.find`). */
export type MountedScreen = VueWrapper;

/**
 * Mount 1 màn với đúng plugin của app.
 *
 * Tham số để kiểu `Component` (không phải `ComponentMountingOptions`): generic
 * của `mount` cần suy ra `ComponentProps` để kiểm `props`, mà ở đây mọi màn
 * đều không có props. Truyền thẳng component vào làm generic khớp với chính nó
 * ⇒ không còn ép kiểu, và lỗi props sai vẫn bị bắt.
 */
export async function mountScreen(component: Component, opts: MountScreenOptions = {}) {
  const router = await makeTestRouter(opts.route ?? '/hoc');
  return mount(component, {
    props: opts.props,
    global: {
      plugins: [router, [VueQueryPlugin, { queryClient }]],
      stubs: { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } },
    },
  });
}
