import { fileURLToPath, URL } from 'node:url';
import { defineConfig } from 'vitest/config';
import vue from '@vitejs/plugin-vue';
import tailwindcss from '@tailwindcss/vite';

/**
 * M5 — Vue 3 + Tailwind 4 + urql + vue-query.
 *
 * `target` mặc định của app v2 (`app-v2`, port 8081) chứ không phải app v1
 * (8080): app v1 chỉ phục vụ ~30 endpoint JSON của v1, app v2 mới có
 * `/query` GraphQL + 5 endpoint nhị phân mà SPA này gọi.
 *
 * Proxy `/query` + `/playground` + `/api` về cùng 1 target để dev KHÔNG dính
 * CORS: origin của trang là `http://localhost:5173` (giống allow-list ở
 * `internal/transport/http` middleware.go) nhưng sau proxy thì trình duyệt
 * chỉ thấy same-origin ⇒ không cần header CORS cho các request này.
 */
const V2_TARGET = process.env.LANGAPP_V2 ?? 'http://localhost:8081';

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/query': V2_TARGET,
      '/playground': V2_TARGET,
      '/api': V2_TARGET,
    },
  },
  test: {
    environment: 'happy-dom',
    include: ['src/**/*.test.ts'],
    // Mỗi file test tự chủ `vi.stubGlobal` + `vi.unstubAllGlobals` (giữ nguyên
    // đúng convention của 105 test v1) nên không cần pool riêng.
    restoreMocks: true,
  },
});
