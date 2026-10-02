// Port 1:1 từ `web/src/api.ts` (app v1) — đọc `apiBase` trong
// localStorage, rỗng = cùng origin. App v2 phục vụ luôn static SPA ở cùng
// origin nên rỗng là cấu hình mặc định đúng; giữ lại ô "API base" ở Cài đặt
// vì 1 người dùng hay chạy app ở máy khác trong cùng LAN.

/** API base URL, cấu hình được từ Cài đặt (lưu trong localStorage). */
export function getApiBase(): string {
  try {
    return localStorage.getItem('apiBase') ?? '';
  } catch {
    return '';
  }
}

export function setApiBase(base: string): void {
  try {
    const trimmed = base.trim();
    if (trimmed) localStorage.setItem('apiBase', trimmed);
    else localStorage.removeItem('apiBase');
  } catch {
    /* storage unavailable */
  }
}
