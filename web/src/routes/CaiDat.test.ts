// BẮT BUỘC của gate M4 (gotcha #3): `/api/backup` + `/api/restore` trả **501**
// tới M7 — UI phải báo lỗi rõ ràng, KHÔNG được trông như thành công.
//
// App v1 có `<a href="/api/backup" download>`: bấm là trình duyệt tải về 1 file
// JSON lỗi 501 rồi báo "xong". Test ở đây bắt đúng điều đó — xoá nhánh báo
// lỗi đi là đỏ.
import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushPromises } from '@vue/test-utils';
import CaiDat from './CaiDat.vue';
import { mountScreen, type MountedScreen } from '../test/harness';
import { resetStaleMarks, resetUrqlClient } from '../graphql/client';
import { queryClient } from '../rest/queryClient';

const NOT_IMPLEMENTED =
  'tính năng backup chưa dùng được: Postgres không có `VACUUM INTO`, cần `pg_dump`/`pg_restore` (M7)';

afterEach(() => {
  vi.unstubAllGlobals();
  resetUrqlClient();
  resetStaleMarks();
  queryClient.clear();
});

/** Trả 501 cho `/api/backup` và `/api/restore`, 200 cho mọi thứ khác. */
function stubServerWith501() {
  const fetchMock = vi.fn().mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.includes('/api/backup') || url.includes('/api/restore')) {
      return new Response(JSON.stringify({ error: NOT_IMPLEMENTED }), {
        status: 501,
        headers: { 'content-type': 'application/json' },
      });
    }
    return new Response(JSON.stringify({ data: {} }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function clickByText(wrapper: MountedScreen, text: string) {
  const btn = wrapper.findAll('button').find((b) => b.text().includes(text));
  expect(btn, `không thấy nút "${text}"`).toBeTruthy();
  return btn!.trigger('click');
}

describe('test_cai_dat_backup_501', () => {
  it('test_backup_button_calls_api_and_shows_server_message', async () => {
    const fetchMock = stubServerWith501();
    const wrapper = await mountScreen(CaiDat, { route: '/cai-dat' });
    await flushPromises();

    await clickByText(wrapper, 'Kiểm tra & tải backup');
    // Query on-demand: cần vài nhịp cho vue-query chạy xong rồi re-render.
    for (let i = 0; i < 12; i += 1) await flushPromises();

    expect(fetchMock.mock.calls.some((c) => String(c[0]).includes('/api/backup'))).toBe(true);
    // Message tiếng Việt NGUYÊN VĂN của server phải lên UI.
    expect(wrapper.text()).toContain(NOT_IMPLEMENTED);
  });

  it('test_backup_501_never_renders_success_state', async () => {
    stubServerWith501();
    const wrapper = await mountScreen(CaiDat, { route: '/cai-dat' });
    await flushPromises();

    await clickByText(wrapper, 'Kiểm tra & tải backup');
    for (let i = 0; i < 12; i += 1) await flushPromises();

    // Nút "Lưu file backup" chỉ hiện khi query THÀNH CÔNG — 501 thì không được.
    expect(wrapper.text()).not.toContain('Đã tải được backup');
    expect(wrapper.text()).not.toContain('Lưu file backup');
    // Trạng thái "đang tải" cũng phải tắt (không kẹt ở loading vô hạn).
    expect(wrapper.text()).not.toContain('đang kiểm tra…');
  });

  it('test_khong_con_the_a_download_troi_thang_bam_xuong_file_loi', async () => {
    // 5.1 (cổng Oracle M5): app v1 có `<a href="/api/backup" download>` — bấm là
    // trình duyệt tải về 1 file JSON báo 501 rồi báo "xong". M5 đã gỡ, nhưng dễ
    // thêm lại khi ai đó muốn "nút tải nhanh". Test này canh đúng thứ đó.
    const wrapper = await mountScreen(CaiDat, { route: '/cai-dat' });
    await flushPromises();

    const direct = wrapper.findAll('a').filter((a) => a.attributes('href')?.includes('/api/backup'));
    expect(direct, 'đã quay lại dùng <a download> tới /api/backup — bấm là tải file lỗi 501').toEqual([]);
  });

  it('test_restore_501_shows_server_message_not_success', async () => {
    const fetchMock = stubServerWith501();
    const wrapper = await mountScreen(CaiDat, { route: '/cai-dat' });
    await flushPromises();

    // happy-dom không cho đặt `files` thật ⇒ dựng mảng 1 phần tử giả (màn chỉ
    // đọc `files?.[0]`).
    //
    // Ghi chú: `File`/`FormData` của happy-dom khiến happy-dom in ra
    // `ECONNREFUSED 127.0.0.1:3000` (URL mặc định của chính nó) ra stderr. Đó là
    // TIẾNG LOG của môi trường test, KHÔNG phải request nào của app — đã dò
    // `fetch` + `net.connect` + `XMLHttpRequest`: cả 3 lần gọi đều về
    // `/query` và `/api/restore` đúng như thiết kế, và không test nào đỏ vì nó.
    const input = wrapper.find('input[aria-label="file backup để restore"]');
    const file = new Blob(['dump'], { type: 'application/octet-stream' });
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true });
    await input.trigger('change');
    for (let i = 0; i < 12; i += 1) await flushPromises();

    expect(fetchMock.mock.calls.some((c) => String(c[0]).includes('/api/restore'))).toBe(true);
    expect(wrapper.text()).toContain(NOT_IMPLEMENTED);
    expect(wrapper.text()).not.toContain('Restore xong');
  });
});
