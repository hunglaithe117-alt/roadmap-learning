// F2 (cổng Oracle M5) — chứng minh `provideClient` ở `main.ts` **không** làm
// được việc gì, và bản sửa `app.use(urqlVue, client)` **có** làm được.
//
// Vì sao cần test thật thay vì tin code: `provideClient()` gọi `provide()` của
// Vue; `provide()` chỉ chạy được trong `setup()`. Gọi ở top-level module KHÔNG
// throw — chỉ in warning rồi âm thầm không làm gì ⇒ app vẫn chạy, 14 màn vẫn
// render, và M6 chỉ vỡ khi viết màn đầu tiên dùng `useQuery`. Đó là loại lỗi
// im lặng đắt nhất.
//
// Ghi chú API: `@urql/vue` 2.1.1 **không export `useClient`** (chỉ có
// `useClientHandle`, còn `useQuery`/`useMutation` gọi `useClient()` bên trong).
// Vì vậy probe dùng `useClientHandle()` — đường công khai tương đương, và cho
// ta so sánh identity của client, thứ mà `useQuery` không để lộ.
import { afterEach, describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { createApp, defineComponent, h } from 'vue';
import { provideClient, useClientHandle, useQuery } from '@urql/vue';
import { gql } from '@urql/core';
import { resetUrqlClient, urqlClient } from './graphql/client';
import { installPlugins } from './main';

/** Kết quả setup của `Probe` cho lần mount gần nhất. */
let seen: { client?: unknown; usedUseQuery: boolean } = { usedUseQuery: false };

const Probe = defineComponent({
  name: 'UrqlProbe',
  setup() {
    const handle = useClientHandle();
    seen.client = handle.client;
    // `useQuery` với `pause: true` — vẫn đi qua `useClient()` nên chạy được ở
    // đây là bằng chứng client đã được provide, mà không bắn request ra mạng.
    try {
      useQuery({ query: gql`{ decks { id } }`, pause: true });
      seen.usedUseQuery = true;
    } catch {
      seen.usedUseQuery = false;
    }
    return () => h('span', { 'data-testid': 'ok' }, 'ok');
  },
});

/**
 * Mount `Probe` sau khi chạy đúng hàm `installPlugins` của `main.ts`.
 *
 * Vì sao phải đi vòng qua `installPlugins(app)` rồi lấy `app._context.provides`:
 * `mount(app)` CỦA VUE-UTILS không render (nó cần `Component`, không cần
 * `App`), và `global.plugins` thì tự dựng app khác — nên không thể dùng để kiểm
 * "app đã gắn plugin chưa". Đọc `provides` từ app vừa gắn là cách kiểm đúng
 * thứ mà `main.ts` thật sự làm.
 */
function mountAfterInstallPlugins(): ReturnType<typeof mount> {
  seen = { usedUseQuery: false };
  const app = createApp(Probe);
  installPlugins(app);
  // `provides` là nơi `app.use(plugin)` ghi vào — đây là hợp đồng của Vue,
  // không phải chi tiết riêng của urql.
  const provides = (app as unknown as { _context: { provides: Record<string, unknown> } })._context
    .provides;
  return mount(Probe, { global: { provide: provides } });
}

afterEach(() => {
  vi.unstubAllGlobals();
  resetUrqlClient();
});

describe('test_urql_client_is_provided_to_components', () => {
  it('test_installPlugins_cua_main_ts_that_su_provide_client', () => {
    // Test này gọi ĐÚNG hàm `installPlugins` mà `main.ts` chạy khi boot. Nhờ vậy
    // `main.ts` không thể quay lại `provideClient` ở top-level (F2) mà test vẫn
    // xanh — sửa `main.ts` là test này đỏ.
    const wrapper = mountAfterInstallPlugins();

    expect(wrapper.find('[data-testid="ok"]').exists()).toBe(true);
    expect(seen.usedUseQuery, 'useQuery phải chạy được trong component con').toBe(true);
    // PHẢI là ĐÚNG instance của `graphql/client.ts` — client thứ 2 là 2
    // document cache, tức là lệch cache (rủi ro STACK-V2 §3).
    expect(seen.client).toBe(urqlClient());
  });

  it('test_cach_plugin_cung_dung_client_khong_phai_client_thu_2', () => {
    // Ngược lại: nếu ai đó truyền `new Client({...})` mới vào `app.use` thì 2
    // document cache ⇒ lệch cache (STACK-V2 §3). Test này canh đúng việc đó.
    const app = createApp(Probe);
    installPlugins(app);
    const provides = (app as unknown as { _context: { provides: Record<string, unknown> } })._context
      .provides;
    const injected = provides['$urql'] as { value: unknown } | undefined;
    expect((injected?.value ?? injected), 'phải đúng instance urqlClient()').toBe(urqlClient());
  });

  it('test_provideClient_ở_top_level_module_là_noop', () => {
    // Cách CŨ: gọi ở top-level (ngoài setup). Vue cảnh báo và KHÔNG provide.
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    provideClient(urqlClient());
    const warned = warn.mock.calls.some((c) =>
      String(c[0]).includes('provide() can only be used inside setup()'),
    );
    warn.mockRestore();

    expect(
      warned,
      'Vue phải cảnh báo provide() ngoài setup — nếu không cảnh báo thì hành vi đã đổi và test này cần viết lại',
    ).toBe(true);

    // Và hậu quả quan trọng: component con KHÔNG lấy được client.
    seen = { usedUseQuery: false };
    let threw = '';
    try {
      mount(Probe);
    } catch (e) {
      threw = (e as Error).message;
    }
    expect(threw).toMatch(/No urql Client was provided/);
  });
});
