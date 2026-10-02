// BẮT BUỘC của gate M5: "20 route render đúng, không còn framework cũ" + "giữ
// nguyên path cũ để bookmark cũ còn chạy". Test 2 mảng: (1) shell render đủ 14
// link với đúng hash, (2) mỗi hash cũ vẫn mount được màn tương ứng.
//
// Vì sao phải test cả 14 chứ không chỉ 1: `router.ts` khai bảng route thủ
// tụng — thiếu 1 dòng là màn đó biến mất mà không có lỗi biên dịch nào báo.
import { describe, it, expect, vi, afterEach } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import { VueQueryPlugin } from '@tanstack/vue-query';
import App from './App.vue';
import { ROUTES, type Route } from './router';
import { makeTestRouter } from './test/harness';
import { queryClient } from './rest/queryClient';
import { resetStaleMarks, resetUrqlClient } from './graphql/client';

afterEach(() => {
  vi.unstubAllGlobals();
  resetUrqlClient();
  resetStaleMarks();
  queryClient.clear();
});

/**
 * Server giả: mọi query trả payload RỖNG hợp lệ.
 *
 * Trả một object `data` chứa TOÀN BỘ field của mọi operation thay vì chỉ field
 * của operation vừa gọi — GraphQL không phạt field thừa, urql chỉ đọc đúng
 * phần câu query xin. Cách này không phụ thuộc việc đoán tên field (query
 * `Decks` đọc field `decks` còn bản chữ hoa khác) — dạng sai đó im lặng biến
 * `data.x` thành `undefined` và làm hỏng render mà không có lỗi network nào.
 */
const EMPTY_PAYLOAD = {
  decks: [],
  dueCards: [],
  cards: [],
  shadowProgress: { cardId: '1', loops: 0, rate: 1, updatedAt: '' },
  errors: [],
  topErrors: [],
  errorSuggestions: [],
  readerArticles: [],
  stress: { term: '', ipa: '', stress: '', exception: false, note: '', fromDict: false },
  dictSearch: [],
  englishSearch: [],
  chunk: { ok: true, chunks: [], error: null },
  gradeTone: { ok: true, grade: { grade: 3, score: 0.5, exact: false, hit: 1 }, error: null },
  strokes: { ok: true, index: [], info: null, error: null },
  thieuAxes: [],
  thieuSessions: [],
  syncConflicts: { ok: true, conflicts: [], error: null },
  syncStatus: { ok: true, status: null, error: { message: 'lỗi hệ thống', code: 'INTERNAL' } },
  stats: { ok: true, stats: null, error: { message: 'lỗi hệ thống', code: 'INTERNAL' } },
};

function stubEmptyServer() {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation(
      async () =>
        new Response(JSON.stringify({ data: EMPTY_PAYLOAD }), {
          status: 200,
          headers: { 'content-type': 'application/json' },
        }),
    ),
  );
}

/**
 * Mount app shell đúng như `main.ts` làm: router + vue-query. Thiếu
 * `VueQueryPlugin` thì màn Cài đặt ném "No 'queryClient' found" — đây là
 * chính là lý do `harness.ts` tồn tại.
 */
async function mountApp(route: string) {
  stubEmptyServer();
  const router = await makeTestRouter(route);
  const wrapper = mount(App, {
    global: { plugins: [router, [VueQueryPlugin, { queryClient }]] },
  });
  // `<RouterView>` render component lazy: flush nhiều nhịp cho tới khi màn có mặt.
  for (let i = 0; i < 12; i += 1) await flushPromises();
  return wrapper;
}

/**
 * 14 path của app v1. Đây là hợp đồng "bookmark cũ còn chạy" — nên assert
 * TỪNG PATH còn tồn tại, KHÔNG assert `ROUTES.length === 14`: M6 sẽ thêm
 * `/roadmap` + `/bookmarks`, và một assert đếm số sẽ đỏ đúng lúc M6 thêm
 * tính năng mới (tức là test cản đúng việc M6 cần làm).
 */
const LEGACY_PATHS: Route[] = [
  '/hoc', '/player', '/recorder', '/loi-sai', '/reader', '/review', '/dashboard',
  '/zh-pinyin', '/zh-stroke', '/zh-bingo', '/en-stress', '/en-pvo', '/en-thieu', '/cai-dat',
];

describe('test_app_shell', () => {
  it('test_app_van_render_link_cho_moi_path_cu', async () => {
    const wrapper = await mountApp('/hoc');
    const hrefs = wrapper.findAll('nav a').map((a) => a.attributes('href') ?? '');
    // Test router dùng `createMemoryHistory` (không đụng `window.location.hash`)
    // nên `href` là `/hoc`, KHÔNG phải `#/hoc` như hash history thật. Vì vậy so
    // bằng cách bỏ tiền tố `#` thay vì gắn cứng — hash thật đã được cover ở
    // `router.test.ts` (`routeHref`).
    const normalized = hrefs.map((h) => h.replace(/^#/, ''));
    for (const p of LEGACY_PATHS) {
      expect(normalized, `thiếu link cho path cũ ${p}`).toContain(p);
    }
  });

  it('test_bang_route_khong_trung_duoc_path_cu', async () => {
    const declared = ROUTES.map((r) => r.path);
    for (const p of LEGACY_PATHS) {
      expect(declared, `router.ts mất path cũ ${p}`).toContain(p);
    }
  });

  it('test_app_marks_active_route_with_bold_class', async () => {
    const wrapper = await mountApp('/review');
    const active = wrapper.findAll('nav a').filter((a) => a.classes().includes('font-bold'));
    expect(active).toHaveLength(1);
    expect(active[0].text()).toBe('Review');
  });
});

describe('test_legacy_hash_paths_still_resolve', () => {
  // 14 path của app v1 — mỗi path 1 màn, không mất màn nào khi đổi framework.
  const SCREENS: Array<[string, string]> = [
    ['/hoc', 'Học'],
    ['/review', 'Review'],
    ['/cai-dat', 'Cài đặt'],
    ['/zh-pinyin', 'Luyện thanh điệu (Pinyin Drill)'],
    ['/zh-stroke', 'Tập viết nét chữ Hán'],
    ['/zh-bingo', 'Tone Bingo'],
    ['/en-stress', 'English — Trọng âm & Chunking'],
    ['/en-pvo', 'English — Drill PVO / TMRND'],
    ['/en-thieu', 'English — Checklist THIEU 8 trục'],
    ['/player', 'Luyện nói (Shadowing)'],
    ['/recorder', 'Ghi âm & Sổ lỗi'],
    ['/loi-sai', 'Sổ lỗi'],
    ['/reader', 'Đọc hiểu (Graded reader)'],
    ['/dashboard', 'Dashboard'],
  ];

  it.each(SCREENS)('test_legacy_path_%s_renders_its_screen', async (path, heading) => {
    const wrapper = await mountApp(path);
    expect(wrapper.text()).toContain(heading);
  });
});
