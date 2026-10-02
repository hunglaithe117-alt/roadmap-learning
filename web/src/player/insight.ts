// M6 — 1 hàm đọc `insightTopErrors`.
//
// Tách khỏi `player/api.ts` vì hàm này là ĐỌC trên query có payload, không ghi
// gì: đặt cạnh `postError` sẽ dễ khiến người đọc tưởng nó phải gọi
// `afterMutation()` (điều mà `mutationInvalidation.test.ts` cũngng kiểm).
//
// Vì sao không dùng `topErrors` (đang được `player/api.ts` dùng): query đó
// trả `{word, count}` — không có `cardId` nên dòng từ sai không nhảy được tới
// thẻ. `insight.topErrors` đọc thẳng lịch sử `notes` và mang `cardId`/`front`
// (M6a §5.7). `cardId` NULL khi lỗi luyện tự do hoặc thẻ đã xoá mềm.
import { runQuery } from '../graphql/client';
import { assertPayloadOk } from '../graphql/errors';
import { InsightTopErrors, type TopErrorWithCard } from '../graphql/operations';

export type { TopErrorWithCard };

export async function fetchInsightTopErrors(limit = 10): Promise<TopErrorWithCard[]> {
  const data = await runQuery(InsightTopErrors, { limit });
  assertPayloadOk(data.insightTopErrors);
  return data.insightTopErrors.errors;
}
