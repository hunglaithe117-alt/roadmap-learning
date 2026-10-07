// M5 — composable vue-query cho 3 endpoint REST. Đây là **nửa thứ hai** của
// kiến trúc 2 protocol: GraphQL đi qua urql, 3 endpoint này đi qua vue-query.
//
// Vì sao `useQuery` chứ không tự `ref` + `await`: các endpoint này được bấm
// thủ công (bấm "Check /api/health", bấm "Play TTS") chứ không tự chạy — nên
// chúng là **on-demand query** (`enabled` theo 1 ref). Đây cũng là nơi duy
// nhất trong app đọc `error` từ vue-query, để khi M6 thêm endpoint mới chỉ cần
// đăng ký key trong `rest/queryClient.ts` là được invalidate tự động bởi
// `afterMutation()`.
//
// Options của `useQuery` (vue-query) bám từng entry của `queryKey` vào nguồn
// của nó nên truyền `ref` cho `text` là tự refetch khi đổi chữ — đó là lý do
// các hàm dưới nhận `MaybeRef` chứ không nhận string.
import { useQuery } from '@tanstack/vue-query';
import { toValue, type MaybeRef } from 'vue';
import { fetchHealth, fetchTTSEngine } from './client';
import { restKeys } from './queryClient';

/**
 * `GET /api/health` — bấm tay ở Cài đặt. `enabled` mặc định `false` vì
 * healthcheck của Docker đã lo việc này; đây chỉ là nút "xem trạng thái".
 */
export function useHealthQuery(enabled: MaybeRef<boolean>) {
  // Bọc `fetchHealth` thành closure không tham số: vue-query truyền 1 object
  // context vào `queryFn`, còn `fetchHealth(base?)` nhận string — truyền thẳng
  // là base nhận nhầm context (và `base` có default nên TS không bắt được).
  return useQuery({ queryKey: restKeys.health(), queryFn: () => fetchHealth(), enabled });
}

/**
 * Header `X-Engine` của `/api/tts` — badge "engine nào đang chạy". Gọi rồi huỷ
 * body (xem `fetchTTSEngine`) nên không tải audio về.
 */
export function useTtsEngineQuery(text: MaybeRef<string>, enabled: MaybeRef<boolean> = true) {
  return useQuery({
    queryKey: restKeys.ttsEngine(toValue(text)),
    queryFn: () => fetchTTSEngine(toValue(text)),
    enabled,
    retry: false,
  });
}
