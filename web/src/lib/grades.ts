/**
 * Thang chấm SRS 1-4 — hợp đồng đóng băng từ v1 (cả `domain/srs.Grade` ở server
 * lẫn UI). Nằm ở file .ts riêng vì cả 3 màn (Review, EnStress, EnPvo) và test
 * đều cần đọc, còn `<script setup>` không export được hằng cho module khác.
 */
export const GRADES = [
  { value: 1, label: 'Quên' },
  { value: 2, label: 'Khó' },
  { value: 3, label: 'Được' },
  { value: 4, label: 'Dễ' },
] as const;
