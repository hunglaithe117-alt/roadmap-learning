// F4 (cổng Oracle M5) — banner nhắc học 20:00 ở Dashboard không bao giờ hiện.
//
// Nguyên nhân: `onMounted` gọi `markVisited()` TRƯỚC `shouldRemind()`, mà
// `shouldRemind()` = `giờ >= 20 && !visitedToday()` ⇒ vừa đánh dấu "đã ghé" thì
// hỏi lại thì luôn `false`. `<p v-if="remind">` là code chết — không lỗi biên
// dịch, không cảnh báo, chỉ im lặng.
//
// Test ở đây mount component THẬT (không chỉ test `streak.ts` — `streak.test.ts`
// chỉ phủ `computeStreakDays` nên không bắt được lỗi thứ tự này).
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises } from '@vue/test-utils';
import Dashboard from './Dashboard.vue';
import { mountScreen, type MountedScreen } from '../test/harness';
import { resetStaleMarks, resetUrqlClient } from '../graphql/client';
import { queryClient } from '../rest/queryClient';
import { LAST_VISIT_KEY_FOR_TEST } from './dashboardReminderProbe';

const REMIND_TEXT = 'Hôm nay chưa ôn SRS';

/** Đặt giờ hệ thống về 21:00 một ngày cố định (sau mốc 20:00 của `shouldRemind`). */
function setClockTo21h() {
  const fixed = new Date(2026, 8, 28, 21, 0, 0);
  vi.useFakeTimers({ shouldAdvanceTime: true, now: fixed.getTime() });
  return fixed;
}

beforeEach(() => {
  resetUrqlClient();
  resetStaleMarks();
  queryClient.clear();
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

function stubServer() {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation(
      async () =>
        new Response(JSON.stringify({ data: { stats: null, topErrors: [] } }), {
          status: 200,
          headers: { 'content-type': 'application/json' },
        }),
    ),
  );
}

async function mountDashboard(): Promise<MountedScreen> {
  const wrapper = await mountScreen(Dashboard, { route: '/dashboard' });
  for (let i = 0; i < 6; i += 1) await flushPromises();
  return wrapper;
}

describe('test_dashboard_reminder_banner', () => {
  it('test_banner_hien_khi_giou_20h_va_hom_nay_chua_ghe', async () => {
    setClockTo21h();
    stubServer();
    // localStorage sạch ⇒ hôm nay CHƯA ghé.
    const store = new Map<string, string>();
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => (store.has(k) ? store.get(k)! : null),
      setItem: (k: string, v: string) => void store.set(k, v),
      removeItem: (k: string) => void store.delete(k),
    });

    const wrapper = await mountDashboard();

    expect(wrapper.text(), 'banner nhắc học 20:00 phải hiện lúc 21:00 khi hôm nay chưa học').toContain(REMIND_TEXT);
  });

  it('test_hien_banner_roi_moi_danh_dau_da_ghe', async () => {
    setClockTo21h();
    stubServer();
    const store = new Map<string, string>();
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => (store.has(k) ? store.get(k)! : null),
      setItem: (k: string, v: string) => void store.set(k, v),
      removeItem: (k: string) => void store.delete(k),
    });

    await mountDashboard();

    // Sau khi hỏi xong thì mới đánh dấu — nếu đảo, dấu sẽ có trước lúc hỏi và
    // `shouldRemind()` không bao giờ thấy "chưa ghé".
    expect(store.get(LAST_VISIT_KEY_FOR_TEST)).toBe('2026-09-28');
  });

  it('test_banner_khong_hien_lai_trong_cung_ngay', async () => {
    setClockTo21h();
    stubServer();
    const store = new Map<string, string>();
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => (store.has(k) ? store.get(k)! : null),
      setItem: (k: string, v: string) => void store.set(k, v),
      removeItem: (k: string) => void store.delete(k),
    });

    await mountDashboard();
    const wrapper = await mountDashboard(); // mount lần 2 trong cùng ngày

    // Đã ghé hôm nay ⇒ không nhắc lại (banner 1 lần/ngày, không spam).
    expect(wrapper.text()).not.toContain(REMIND_TEXT);
  });

  it('test_banner_khong_hien_truocc_20h', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true, now: new Date(2026, 8, 28, 9, 0, 0).getTime() });
    stubServer();
    const store = new Map<string, string>();
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => (store.has(k) ? store.get(k)! : null),
      setItem: (k: string, v: string) => void store.set(k, v),
      removeItem: (k: string) => void store.delete(k),
    });

    const wrapper = await mountDashboard();

    expect(wrapper.text()).not.toContain(REMIND_TEXT);
  });
});
