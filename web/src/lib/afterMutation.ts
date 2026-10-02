// M5 — helper DUY NHẤT để đồng bộ cache của 2 protocol sau mỗi mutation.
//
// VÌ SAO cần (STACK-V2 §3 — rủi ro lớn nhất còn lại của kiến trúc 2 protocol):
//   - dữ liệu JSON đi qua urql (GraphQL `/query`),
//   - dữ liệu nhị phân đi qua vue-query (`/api/tts`, `/api/stt`,
//     `/api/backup`, `/api/restore`, `/api/health`).
// Hai cache này **không biết nhau**. Ôn xong 1 thẻ (`recordReview`) mà chỉ
// invalidate urql thì `DueCards` vẫn trả hàng đợi cũ; ngược lại bấm "Check
// /api/health" xong mà chỉ invalidate vue-query thì cache GraphQL vẫn cũ.
// Rải lệnh `invalidate` ở từng màn là cách chắc chắn sót — nên mọi mutation ở
// `src/{srs,chinese,english,player}/api.ts` đều gọi đúng 1 hàm này.
//
// KHÔNG invalidate trực tiếp ở component: component không biết mình vừa làm
// thay đổi cái gì, và 14 màn × nhiều mutation = nhiều chỗ để quên.
import { markAllGraphQLStale } from '../graphql/client';
import { REST_NAMESPACE, queryClient } from '../rest/queryClient';

/**
 * Gọi sau MỌI mutation. Trả `Promise` vì `invalidateQueries` bất đồng bộ —
 * `await` nó để màn gọi tiếp (`await createDeck(); await fetchDecks()`) chắc
 * chắn đọc được dữ liệu mới thay vì đọc trúng cache vừa bị đánh dấu cũ.
 *
 * - urql: `markAllGraphQLStale()` tăng "kỷ nguyên" ⇒ key nào chưa lấy ở kỷ
 *   nguyên mới sẽ đi `network-only` ở lần đọc kế tiếp.
 *
 *   KHÔNG được cho rằng `cacheExchange` đã loại bỏ đúng entity mà mutation trả
 *   về nên không cần gọi hàm này. Cơ chế tự-invalidate của urql cần `__typename`
 *   + `id` ở CẢ selection set của query lẫn mutation (`operations.ts` đã viết
 *   tay vì gqlgen không chèn), và **vẫn không đủ** cho các mutation không trả
 *   entity của query đó: `recordReview` trả `ReviewResult` chứ không phải `Card`
 *   ⇒ `DueCards` giữ nguyên hàng đợi cũ. Đo thật: `graphql/__typename.test.ts`.
 *
 *   ⇒ Đây là cơ chế invalidate DUY NHẤT; `__typename` chỉ giảm request thừa.
 * - vue-query: invalidate theo namespace `['api']` — namespace này là nơi
 *   duy nhất mọi query REST đăng ký (`rest/queryClient.ts`).
 */
export async function afterMutation(): Promise<void> {
  markAllGraphQLStale();
  await queryClient.invalidateQueries({ queryKey: [...REST_NAMESPACE] });
}
