// Helper cho test client GraphQL.
//
// urql chọn **GET** cho query và **POST** cho mutation: với GET, operation +
// variables nằm trong query string của URL; với POST chúng nằm trong body
// JSON. Hàm này đọc CẢ HAI vị trí để test không phải biết urql chọn gì — và để
// khi đổi `preferGetMethod` test không đỏ vì lý do vô nghĩa.
import { expect, vi } from 'vitest';

export interface GraphQLBody {
  query: string;
  variables: Record<string, unknown>;
  operationName?: string;
}

/** Body `JSON.parse` của một lần gọi `fetch` tới `/query`. */
export function bodyOf(fetchMock: ReturnType<typeof vi.fn>, index = 0): GraphQLBody {
  const [url, init] = fetchMock.mock.calls[index] as [string, RequestInit | undefined];
  if (init?.body) return JSON.parse(init.body as string) as GraphQLBody;

  const qs = new URL(url, 'http://localhost').searchParams;
  const variables = qs.get('variables');
  return {
    query: '',
    operationName: qs.get('operationName') ?? undefined,
    variables: variables ? (JSON.parse(variables) as Record<string, unknown>) : {},
  };
}

/** Tên operation mà server nhận — assert nó đúng là kiểm ĐÚNG câu query gửi đi. */
export function operationNameOf(fetchMock: ReturnType<typeof vi.fn>, index = 0): string {
  const body = bodyOf(fetchMock, index);
  if (body.operationName) return body.operationName;
  const m = /query\s+(\w+)|mutation\s+(\w+)/.exec(body.query);
  expect(m, 'không tìm thấy operation trong body').toBeTruthy();
  return m![1] ?? m![2];
}

/** Response GraphQL `{data}`; mỗi lần gọi trả Response MỚI (Response không đọc 2 lần được). */
export function mockData(...payloads: unknown[]): ReturnType<typeof vi.fn> {
  let i = 0;
  const fetchMock = vi.fn().mockImplementation(async () => {
    const body = payloads[Math.min(i, payloads.length - 1)];
    i += 1;
    return new Response(JSON.stringify({ data: body }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}
