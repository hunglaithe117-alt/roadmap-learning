// Sort cây roadmap ở CLIENT — lưới an toàn thứ 2, KHÔNG phải nguồn sự thật.
//
// Vì sao cần dù server đã `ORDER BY position, id` rồi (M4 đã sửa F8:
// `StageTreeByPathIDs` từng gom bằng `range` trên Go map ⇒ `path.stages` trả
// thứ tự NGẪU NHIÊN mỗi request)?
//
//   1. Bản đồ roadmap (M6) vẽ node theo thứ tự mảng. Một lần server trả sai
//      thứ tự là node nhảy chỗ giữa 2 lần tải — mà KHÔNG có lỗi nào để client
//      phát hiện.
//   2. `LevelState` (DONE/CURRENT/LOCKED) do server gán theo THỨ TỰ topic trong
//      stage. Thứ tự sai thì "topic đầu mỗi stage = CURRENT" rơi vào sai
//      topic, im lặng.
//   3. Client là nơi cuối cùng còn kiểm soát được thứ tự trước khi render.
//
// Hàm này thuần, không đụng framework, và CỐ TÌNH không mutate input.

/** Đủ để sắp; GraphQL `ID` là `String` nên id phải so được cả số lẫn chuỗi. */
export interface Orderable {
  position: number;
  id: string;
}

/**
 * So id theo số khi cả hai vế là số thuần (id DB là `bigint`), ngược lại so
 * chuỗi. Cần TỔNG (total order) để `Array.sort` không trả kết quả phụ thuộc
 * thứ tự input — nếu không thì sort yếu vẫn "chạy được" nhưng lệch tùy engine.
 */
function compareId(a: string, b: string): number {
  const na = Number(a);
  const nb = Number(b);
  if (Number.isFinite(na) && Number.isFinite(nb) && a.trim() !== '' && b.trim() !== '') {
    return na === nb ? 0 : na < nb ? -1 : 1;
  }
  return a < b ? -1 : a > b ? 1 : 0;
}

/** `(position, id)` — đúng thứ tự `ORDER BY` của repository. */
export function sortByPosition<T extends Orderable>(rows: readonly T[]): T[] {
  return [...rows].sort((a, b) => a.position - b.position || compareId(a.id, b.id));
}

// Không có `sortStages`/`sortTopics`: cả 3 tầng dùng CHUNG 1 luật, và 2 wrapper
// trùng lặp chỉ tồn tại vì app v1 có 3 hàm gọi tên khác nhau. M6 vẽ bản đồ sẽ
// gọi `sortByPosition` cho cả path/stage/topic — nếu sau này tầng nào thật sự
// cần luật khác (ví dụ topic sắp theo `level` chứ không theo `position`) thì viết
// hàm riêng lúc đó, có test riêng, thay vì giữ 2 wrapper giả ngay từ đầu.
