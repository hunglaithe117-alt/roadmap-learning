// Danh sách 14 màn + bảng hash của app. M5 giữ **nguyên 14 path cũ** để bookmark
// cũ (`#/review?deck=3`) vẫn chạy — `createWebHashHistory` giữ đúng cách so khớp
// mà `matchRoute` viết (bỏ `#`, cắt `?…`, bỏ `/` cuối, lạ ⇒ rơi về `/hoc`).
import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router';

/** `/roadmap/:slug` không nằm trong `Route` — có tham số, chỉ dùng bởi
 *  `RoadmapMap.vue`. `matchRoute` xử lý nó tách riêng (xem dưới). */
export type Route =
  | '/hoc'
  | '/review'
  | '/cai-dat'
  | '/zh-pinyin'
  | '/zh-stroke'
  | '/zh-bingo'
  | '/en-stress'
  | '/en-pvo'
  | '/en-thieu'
  | '/player'
  | '/recorder'
  | '/loi-sai'
  | '/reader'
  | '/dashboard'
  | '/roadmap'
  | '/bookmarks';

const SCREENS: Array<{ path: Route; label: string; load: () => Promise<unknown> }> = [
  { path: '/hoc', label: 'Học', load: () => import('./routes/Hoc.vue') },
  { path: '/player', label: 'Luyện nói', load: () => import('./player/ShadowPlayer.vue') },
  { path: '/recorder', label: 'Ghi âm', load: () => import('./routes/Recorder.vue') },
  { path: '/loi-sai', label: 'Sổ lỗi', load: () => import('./routes/ErrorBook.vue') },
  { path: '/reader', label: 'Đọc', load: () => import('./routes/Reader.vue') },
  { path: '/review', label: 'Review', load: () => import('./routes/Review.vue') },
  { path: '/dashboard', label: 'Dashboard', load: () => import('./routes/Dashboard.vue') },
  { path: '/roadmap', label: 'Lộ trình', load: () => import('./routes/RoadmapHome.vue') },
  { path: '/bookmarks', label: 'Kho link', load: () => import('./routes/Bookmarks.vue') },
  { path: '/zh-pinyin', label: 'Pinyin', load: () => import('./routes/ZhPinyin.vue') },
  { path: '/zh-stroke', label: 'Nét chữ', load: () => import('./routes/ZhStroke.vue') },
  { path: '/zh-bingo', label: 'Bingo', load: () => import('./routes/ZhBingo.vue') },
  { path: '/en-stress', label: 'Stress', load: () => import('./routes/EnStress.vue') },
  { path: '/en-pvo', label: 'PVO', load: () => import('./routes/EnPvo.vue') },
  { path: '/en-thieu', label: 'THIEU', load: () => import('./routes/EnThieu.vue') },
  { path: '/cai-dat', label: 'Cài đặt', load: () => import('./routes/CaiDat.vue') },
];

export const ROUTES: Array<{ path: Route; label: string }> = SCREENS.map(({ path, label }) => ({
  path,
  label,
}));

const KNOWN: Route[] = ROUTES.map((r) => r.path);

/**
 * `true` khi hash là `/roadmap/<slug>`. Route có tham số nên không nằm trong
 * `Route` (union của path tĩnh) — tách riêng thay vì ép slug vào `Route` rồi
 * phải so 2 mẫu ở mọi nơi dùng.
 */
export function matchDetailRoute(hash: string): string | null {
  const path = hash.replace(/^#/, '').split('?')[0].replace(/\/$/, '');
  const m = /^\/roadmap\/([^/]+)$/.exec(path);
  return m ? m[1] : null;
}

/** Pure hash -> route mapping (no DOM), so vitest can cover routing. */
export function matchRoute(hash: string): Route {
  const path = hash.replace(/^#/, '').split('?')[0].replace(/\/$/, '') || '/hoc';
  if ((KNOWN as string[]).includes(path)) return path as Route;
  if (matchDetailRoute(hash)) return '/roadmap';
  return '/hoc';
}

export function routeHref(route: Route): string {
  return `#${route}`;
}

/**
 * Bảng route của vue-router. Component nạp **lazy** (dynamic import) vì có 14
 * màn: nạp sẵn tất cả làm chunk đầu tiên nặng gấp nhiều lần, mà app v1 (1
 * file) không có vấn đề đó.
 *
 * `redirect` cho `/` và `/:pathMatch(.*)*` dùng lại `matchRoute` — 1 nơi duy
 * nhất quyết định "path lạ thì về đâu", không phải 2 bảng quy tắc song song.
 */
/**
 * Bảng route. Tách khỏi `createAppRouter` vì test cần dựng router với
 * `createMemoryHistory` (không đụng `window.location.hash`) mà vẫn phải dùng
 * ĐÚNG bảng route của app — trùng danh sách là 2 nơi chắc chắn lệch.
 */
export function appRoutes(): RouteRecordRaw[] {
  return [
    { path: '/', redirect: () => matchRoute('') },
    ...SCREENS.map(({ path, label, load }) => ({
      path,
      name: path,
      component: load,
      meta: { label },
    })),
    // Đặt SAU `/roadmap` để path tĩnh ăn trước; vue-router so theo thứ tự.
    { path: '/roadmap/:slug', name: 'roadmap-map', component: () => import('./routes/RoadmapMap.vue') },
    { path: '/:pathMatch(.*)*', redirect: () => matchRoute('') },
  ];
}

export function createAppRouter() {
  return createRouter({ history: createWebHashHistory(), routes: appRoutes() });
}
