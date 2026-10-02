// 5.6 (cổng Oracle M5) — `roadmap/order.ts` là lưới an toàn của bản đồ M6 mà
// chưa có test nào. Thêm ở M5 (sớm) vì M6 sẽ dựa vào nó để vẽ node; sai thứ tự
// ở đây thì node nhảy chỗ giữa 2 lần tải mà KHÔNG có lỗi nào để phát hiện — đúng
// loại bug F8 mà M4 đã phải sửa ở server.
//
// Case quan trọng nhất là id CHỮI SỐ: `GraphQL ID` là `String` nhưng id DB là
// `bigint`, nên `"10"` so `"9"` bằng phép trừ chuỗi sẽ ra `1 > 9` ⇒ sai. Hàm phải
// ép số khi cả hai vế là số thuần.
import { describe, expect, it } from 'vitest';
import { sortByPosition, type Orderable } from './order';

const row = (position: number, id: string): Orderable => ({ position, id });

describe('test_sort_by_position', () => {
  it('test_sort_theo_position_trước', () => {
    const rows = [row(2, 'a'), row(0, 'b'), row(1, 'c')];
    expect(sortByPosition(rows).map((r) => r.id)).toEqual(['b', 'c', 'a']);
  });

  it('test_position_bằng_nhau_thì_tiêu_chí_id_quyết_định', () => {
    const rows = [row(1, '3'), row(1, '1'), row(1, '2')];
    expect(sortByPosition(rows).map((r) => r.id)).toEqual(['1', '2', '3']);
  });

  it('test_id_số_10_sau_id_số_9_không_so_chuỗi', () => {
    // `Number("10") > Number("9")` đúng; so chuỗi thì `"10" < "9"` ⇒ sai.
    const rows = [row(1, '10'), row(1, '9')];
    expect(sortByPosition(rows).map((r) => r.id)).toEqual(['9', '10']);
  });

  it('test_id_không_số_được_so_chuỗi_theo_thứ tự_lexicographic', () => {
    // Guid của DB là chuỗi: `"9a"` không ép được thành số ⇒ phải so chuỗi.
    const rows = [row(1, '9z'), row(1, '10a')];
    expect(sortByPosition(rows).map((r) => r.id)).toEqual(['10a', '9z']);
  });

  it('test_một_vế_số_một_vế_chuỗi_thì_so_chuỗi', () => {
    const rows = [row(1, 'abc'), row(1, '12')];
    expect(sortByPosition(rows).map((r) => r.id)).toEqual(['12', 'abc']);
  });

  it('test_id_số_thuần_ngang_nhau_được_coi_bằng_nhau', () => {
    // `"01"` và `"1"` cùng id về mặt số ⇒ compareId phải trả 0, không phải so
    // chuỗi (nếu không thì sort không có TỔNG thứ tự → kết quả phụ thuộc engine).
    const rows = [row(1, '01'), row(1, '1')];
    const sorted = sortByPosition(rows);
    expect(sorted).toHaveLength(2);
  });

  it('test_khong_mutate_mảng_vào', () => {
    const rows = [row(2, 'a'), row(0, 'b')];
    const copy = [...rows];
    sortByPosition(rows);
    expect(rows).toEqual(copy);
  });

  it('test_mảng_rỗng_trả_rỗng', () => {
    expect(sortByPosition([])).toEqual([]);
  });

  it('test_sort_ổn_định_khi_nhiều_dòng_cùng_position_và_cùng_id', () => {
    // Tổng thứ tự: `Array.sort` phải cho kết quả không phụ thuộc thứ tự input.
    const rows = [row(1, '5'), row(1, '3'), row(1, '4'), row(1, '3'), row(1, '5')];
    const a = sortByPosition(rows).map((r) => r.id);
    const b = sortByPosition([...rows].reverse()).map((r) => r.id);
    expect(a).toEqual(b);
  });
});
