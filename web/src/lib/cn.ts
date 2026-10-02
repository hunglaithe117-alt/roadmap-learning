/**
 * Gộp class Tailwind — thứ tự sau thắng trước. `cva` ở `Button.vue` trả class
 * khác nhau theo biến thể; không có hàm này thì class của caller sẽ bị
 * `variant:default` của component đè mất (class trùng tên ở 2 chỗ, cái sau
 * cùng thắng theo thứ tự CSS chứ không theo ý muốn).
 */
import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
