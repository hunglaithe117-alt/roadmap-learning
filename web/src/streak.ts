// T5.2 — streak + nhắc học (thuần, không DOM).
// Streak ngày liên tiếp tính từ chuỗi ngày UTC đã học (server /api/stats
// trả streak từ reviews SQLite; hàm này để client hiển thị + test logic).

/** Đếm ngày liên tiếp tới today (YYYY-MM-DD); hôm nay chưa học vẫn giữ nếu hôm qua có. */
export function computeStreakDays(learnedDays: string[], today = new Date()): number {
  const set = new Set(learnedDays);
  const fmt = (d: Date) => d.toISOString().slice(0, 10);
  let cursor = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate()));
  if (!set.has(fmt(cursor))) {
    cursor = new Date(cursor.getTime() - 86400000);
    if (!set.has(fmt(cursor))) return 0;
  }
  let streak = 0;
  while (set.has(fmt(cursor))) {
    streak++;
    cursor = new Date(cursor.getTime() - 86400000);
  }
  return streak;
}

const LAST_VISIT_KEY = 'last-visit-day';
const REMINDER_HOUR = 20;

/** Hôm nay đã ghé chưa (để quyết định hiện badge nhắc trong-app). */
export function visitedToday(now = new Date()): boolean {
  try {
    return localStorage.getItem(LAST_VISIT_KEY) === now.toISOString().slice(0, 10);
  } catch {
    return false;
  }
}

export function markVisited(now = new Date()): void {
  try {
    localStorage.setItem(LAST_VISIT_KEY, now.toISOString().slice(0, 10));
  } catch {
    /* bỏ qua khi không có storage */
  }
}

/** Có nên nhắc học lúc này: sau 20:00 giờ địa phương và hôm nay chưa ghé. */
export function shouldRemind(now = new Date()): boolean {
  return now.getHours() >= REMINDER_HOUR && !visitedToday(now);
}

/** Xin quyền Notification đúng 1 lần, trả false khi browser không hỗ trợ. */
export async function ensureNotificationPermission(): Promise<boolean> {
  try {
    if (!('Notification' in window)) return false;
    if (Notification.permission === 'granted') return true;
    if (Notification.permission === 'denied') return false;
    return (await Notification.requestPermission()) === 'granted';
  } catch {
    return false;
  }
}
