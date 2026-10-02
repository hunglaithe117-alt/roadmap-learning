// Client vue-query — CHỈ dùng cho 5 endpoint REST nhị phân còn lại
// (`/api/tts`, `/api/stt`, `/api/backup`, `/api/restore`, `/api/health`).
// Mọi thứ JSON đã sang GraphQL/urql (STACK-V2 §3).
import { QueryClient } from '@tanstack/vue-query';

/**
 * Client dùng chung cho `VueQueryPlugin` (main.ts) VÀ cho `afterMutation()`
 * — 2 chỗ này phải trỏ CÙNG 1 instance, nếu không invalidate sẽ chạy trên
 * client không ai mount.
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // App single-user chạy local: retry lặp 3 lần chỉ làm user chờ lâu hơn
      // khi app-v2 chưa boot. Không tự refetch khi cửa sổ focus — app v1 cũng
      // vậy (mọi màn tự tải lại khi mount).
      retry: false,
      refetchOnWindowFocus: false,
      staleTime: 0,
    },
  },
});

/**
 * Namespace chung cho MỌI query vue-query. `afterMutation()` invalidate theo
 * namespace này nên không bao giờ sót endpoint REST nào thêm sau này.
 */
export const REST_NAMESPACE = ['api'] as const;

/**
 * Khoá query của từng endpoint REST — 1 nơi để không rải chuỗi.
 *
 * KHÔNG có khoá cho `restore`: `POST /api/restore` chạy qua `useMutation` mà
 * `useMutation` không nhận `queryKey` (chỉ `mutationKey`), nên khoá khai ra sẽ
 * không ai dùng và tạo cảm giác "đã cache" trong khi không có gì được cache.
 * Thành bỏ hơn khai 1 khoả trống — sẽ thêm lại kèm test khi thật sự cần.
 */
export const restKeys = {
  health: () => [...REST_NAMESPACE, 'health'] as const,
  ttsEngine: (text: string) => [...REST_NAMESPACE, 'tts-engine', text] as const,
  backup: () => [...REST_NAMESPACE, 'backup'] as const,
};
