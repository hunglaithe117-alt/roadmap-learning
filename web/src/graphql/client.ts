// Client GraphQL (urql) + tầng thực thi bất đồng bộ cho 14 màn.
//
// VÌ SAO không dùng `useQuery`/`useMutation` của `@urql/vue` ở từng màn:
// 105 test v1 kiểm tra hành vi client (URL/operation/variables/message lỗi),
// và các màn v1 đã tự quản lý state trong component. Giữ nguyên hình dạng
// `await fetchDecks()` ⇒ port 1:1 không phải viết lại logic màn, và test vẫn
// kiểm được ĐÚNG operation gửi đi thay vì chỉ kiểm component render.
// `provideClient` vẫn được gắn ở `main.ts` để component nào cần `useClient`
// (M6) thì dùng được cùng 1 cache.
import {
  Client,
  cacheExchange,
  createRequest,
  fetchExchange,
  type AnyVariables,
  type OperationResult,
  type RequestPolicy,
  type TypedDocumentNode,
} from '@urql/core';
import { getApiBase } from '../lib/apiBase';
import { AppError, fromCombinedError } from './errors';

/** Đường dẫn GraphQL. KHÔNG phải `/api/graphql` — xem STACK-V2 §3. */
export const GRAPHQL_PATH = '/query';

function newClient(): Client {
  return new Client({
    url: `${getApiBase()}${GRAPHQL_PATH}`,
    exchanges: [cacheExchange, fetchExchange],
    requestPolicy: 'cache-first',
  });
}

let client: Client = newClient();

/** Client urql dùng chung cho cả `provideClient` lẫn tầng thực thi bên dưới. */
export function urqlClient(): Client {
  return client;
}

/** Dựng lại client (Cài đặt đổi API base, hoặc test cần cache sạch). */
export function resetUrqlClient(): Client {
  client = newClient();
  resetStaleMarks();
  return client;
}

// ─────────────────────────────────────────────────────────────────────────────
// Lạc thời cache — 1 nửa của `afterMutation()`
// ─────────────────────────────────────────────────────────────────────────────
//
// `cacheExchange` của urql CHỈ tự loại bỏ entry khi mutation trả về entity mà
// **cả query lẫn mutation đều có `__typename` + `id`** trong selection set
// (đo thật ở `graphql/__typename.test.ts`: thiếu `__typename` thì đọc lại
// `cache-first` trả cache cũ). gqlgen không tự chèn `__typename` nên ta viết
// tay trong `operations.ts`.
//
// Nhưng `__typename` KHÔNG đủ, và đây là lý do `afterMutation()` tồn tại:
//   - `recordReview` trả `ReviewResult` — KHÔNG phải `Card` ⇒ không có entity
//     nào để so ⇒ cache `DueCards` vẫn giữ hàng đợi cũ sau khi đã ôn;
//   - `appendError` trả `ErrorEntry` mới, không sửa thẻ nào;
//   - `gradeTone` là QUERY (không phải mutation) nên không tự làm mông cache.
//
// Cách xử lý: đánh dấu "kỷ nguyên" hiện tại + nhớ kỷ nguyên của từng key đã
// lấy từ mạng. `afterMutation()` tăng kỷ nguyên ⇒ key nào chưa được lấy ở kỷ
// nguyên mới thì `requestPolicy: 'network-only'`, còn lại `cache-first`.
// Query nào đã bị đánh dấu thì `cacheExchange` tự ghi lại kết quả mới vào
// document cache ⇒ lần đọc sau đó lại rẻ.
//
// ⇒ `afterMutation()` là lớp bảo đảm DUY NHẤT, `__typename` chỉ là tối ưu
// giảm số request thừa. Đừng tin `__typename` để bỏ `afterMutation()`.

let generation = 0;
// `GraphQLRequest.key` của urql là **số** (hash DJB2 của query + variables),
// không phải chuỗi — khai sai kiểu ở đây khiến 2 request khác nhau có thể trùng
// tên và `requestPolicy` rơi về `cache-first` oan.
type RequestKey = ReturnType<typeof createRequest>['key'];

/** key của operation → kỷ nguyên đã lấy từ mạng. */
const fetchedAt = new Map<RequestKey, number>();

function requestPolicyFor(key: RequestKey): RequestPolicy {
  return fetchedAt.get(key) === generation ? 'cache-first' : 'network-only';
}

function markFetchedFromNetwork(key: RequestKey): void {
  fetchedAt.set(key, generation);
}

/** Mọi query đã lấy ở kỷ nguyên cũ coi như chưa lấy ⇒ lần đọc tới sẽ gọi mạng. */
export function markAllGraphQLStale(): void {
  generation += 1;
  fetchedAt.clear();
}

/** Chỉ dùng trong test. */
export function resetStaleMarks(): void {
  generation = 0;
  fetchedAt.clear();
}

// ─────────────────────────────────────────────────────────────────────────────
// Tầng thực thi
// ─────────────────────────────────────────────────────────────────────────────

/** Ném `AppError` nếu operation lỗi; ngược lại trả `data` đã đóng băng kiểu. */
function unwrap<T>(result: OperationResult<T>): T {
  if (result.error) throw fromCombinedError(result.error);
  if (result.data == null) {
    // Server luôn trả `data` cho operation hợp lệ; `null` kèm không `errors`
    // là dạng hỏng client không dự đoán được nội dung.
    throw new AppError('lỗi hệ thống', 'INTERNAL');
  }
  return result.data;
}

export async function runQuery<TData, TVariables extends AnyVariables = AnyVariables>(
  document: TypedDocumentNode<TData, TVariables>,
  variables?: TVariables,
): Promise<TData> {
  const vars = (variables ?? {}) as TVariables;
  const key = createRequest(document, vars).key;
  const result = await client
    .query(document, vars, { requestPolicy: requestPolicyFor(key) })
    .toPromise();
  // `stale === false` nghĩa là dữ liệu từ mạng (cacheExchange đánh dấu sau khi
  // trả từ `fetchExchange`), tức key này đã "sạch" ở kỷ nguyên hiện tại.
  if (!result.stale) markFetchedFromNetwork(key);
  return unwrap(result);
}

export async function runMutation<TData, TVariables extends AnyVariables = AnyVariables>(
  document: TypedDocumentNode<TData, TVariables>,
  variables?: TVariables,
): Promise<TData> {
  // Mutation luôn `network-only` (đó là mặc định của urql) nên không ghi kỷ nguyên.
  const result = await client
    .mutation(document, (variables ?? {}) as TVariables, { requestPolicy: 'network-only' })
    .toPromise();
  return unwrap(result);
}
