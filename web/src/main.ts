// M5 — entrypoint. 3 thứ phải cài, đúng thứ tự:
//
//   1. `VueQueryPlugin` với ĐÚNG 1 `queryClient` — instance trong
//      `rest/queryClient.ts` cũng là instance `afterMutation()` invalidate.
//      Tạo client thứ 2 ở đây là invalidate chạy trên client không ai mount
//      ⇒ mọi query REST kẹt cache (rủi ro lệch cache 2 protocol, STACK-V2 §3).
//   2. `app.use(urqlVue, client)` — cùng client với tầng thực thi
//      `graphql/client.ts`, để component nào cần `useQuery`/`useClient` (M6)
//      dùng chung 1 document cache.
//   3. Router hash (`createWebHashHistory` trong `router.ts`) — giữ 14 path cũ.
//
// F2 (cổng Oracle M5): trước đây gọi `provideClient(urqlClient())` ở **top-level
// module**. `provideClient` gọi `provide()` của Vue mà `provide()` **chỉ chạy
// được trong setup()** ⇒ Vue in warning và KHÔNG provide được gì ⇒ M6 gọi
// `useClient()` là throw `No urql Client was provided`. Dùng `app.use(plugin,
// client)` — đúng API của `@urql/vue` 2.1.1: `install(app, opts)` gọi
// `app.provide('$urql', …)` nên hoạt động ở cấp app, không cần instance.
import { createApp, type App as VueApp } from 'vue';
import { VueQueryPlugin } from '@tanstack/vue-query';
import urqlVue from '@urql/vue';
import App from './App.vue';
import { createAppRouter } from './router';
import { urqlClient } from './graphql/client';
import { queryClient } from './rest/queryClient';
import './styles/app.css';

/**
 * Gắn 3 plugin vào app.
 *
 * Tách ra khỏi `main.ts` để **test được**: nếu gắn thẳng ở module top-level thì
 * import `main.ts` vào test sẽ mount app thật vào DOM, và test sẽ không phân
 * biệt được "plugin gắn đúng" với "gắn sai nhưng vẫn chạy" — đúng cái lỗi F2
 * (provideClient top-level không ném lỗi, chỉ im lặng không làm gì).
 */
export function installPlugins(app: VueApp): void {
  // ĐÚNG 1 `queryClient`: instance này cũng là instance `afterMutation()`
  // invalidate. Tạo client thứ 2 ở đây là invalidate chạy trên client không ai
  // mount ⇒ mọi query REST kẹt cache (rủi ro lệch cache 2 protocol, STACK-V2 §3).
  app.use(VueQueryPlugin, { queryClient });
  app.use(urqlVue, urqlClient());
  app.use(createAppRouter());
}

// Chỉ mount khi module này là entrypoint thật. `import.meta.env.VITEST` chặn
// lại nhánh mount khi test import `installPlugins` — không có `#app` trong DOM
// của test nên `mount` sẽ ném, và lỗi đó che mất việc test định kiểm.
if (!import.meta.env.VITEST) {
  const app = createApp(App);
  installPlugins(app);
  app.mount('#app');
}
