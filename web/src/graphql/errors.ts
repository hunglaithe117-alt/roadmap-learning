// Ánh xạ lỗi của data layer sang thứ UI hiển thị.
//
// Hợp đồng đã đóng băng ở M4 (`api/graph/schema/common.graphqls` +
// `internal/transport/graphql/errors.go`):
//   - lỗi NGHIỆP VỤ nằm trong `payload.error { message code }`, `message` là
//     tiếng Việt NGUYÊN VĂN từ `*application/errors.Error` ⇒ hiển thị thẳng,
//     KHÔNG dịch lại ở client (dịch 2 lần là cách chắc chắn lệch);
//   - lỗi HỆ THỐNG server đã che sẵn thành `"lỗi hệ thống"` + `code: INTERNAL`
//     ⇒ client cũng chỉ hiện đúng chuỗi đó, không đoán bừa nội dung;
//   - lỗi GraphQL cấp protocol (validation của gqlgen, complexity limit 422)
//     nằm ở mảng `errors` ⇒ `CombinedError.graphQLErrors`, cũng hiện nguyên văn.
import { CombinedError } from '@urql/core';

/** Lỗi đã sẵn sàng hiển thị cho người dùng. */
export class AppError extends Error {
  /** Mã lỗi của server (`BAD_REQUEST`/`NOT_FOUND`/`CONFLICT`/`NOT_IMPLEMENTED`/
   * `INTERNAL`). `NOT_IMPLEMENTED` = tính năng chưa bật (`mutation.sync` khi
   * chưa có nguồn snapshot peer) — hiện "đang xây", KHÔNG phải "server hỏng",
   * nên đừng cho user bấm "thử lại". Cùng mã với REST `/api/backup` 501
   * (`rest/client.ts`). */
  readonly code: string;

  constructor(message: string, code = 'INTERNAL', options?: { cause?: unknown }) {
    super(message, options);
    this.name = 'AppError';
    this.code = code;
  }
}

/** Hình dạng `UserError` trong schema. */
export interface UserErrorLike {
  message: string;
  code: string;
}

/**
 * Payload `ok` + `error` mà M4 đặt cho MỌI mutation (và cho các query có thể
 * lỗi nghiệp vụ). Trả `AppError` khi payload báo lỗi, `null` khi thành công.
 */
export function payloadError(
  payload: { ok?: boolean; error?: UserErrorLike | null } | null | undefined,
): AppError | null {
  if (!payload || payload.ok !== false) return null;
  const message = payload.error?.message ?? 'lỗi hệ thống';
  const code = payload.error?.code ?? 'INTERNAL';
  return new AppError(message, code);
}

/**
 * Biến `payloadError` thành hàm NÉM — đây là cách 17 call site trong
 * `src/{srs,chinese,english,player}/api.ts` dùng nó.
 *
 * Vì sao không sửa `payloadError` thành ném thẳng: nó được viết để trả
 * `null` khi thành công, và "gọi mà quên `throw`" là lỗi **im lặng** — payload
 * `ok:false` lọt qua, màn hiện "lỗi hệ thống" thay vì message tiếng Việt thật
 * (vi phạm đúng gotcha #2 của gate M4). Hàm này gộp 2 bước thành 1 để không
 * còn chỗ quên được: `assertPayloadOk(p)`.
 */
export function assertPayloadOk(payload: {
  ok?: boolean;
  error?: UserErrorLike | null;
}): void {
  const err = payloadError(payload);
  if (err) throw err;
}

/**
 * `CombinedError` là lỗi ở TẦNG PROTOCOL, không phải trong payload:
 * mạng hỏng, CORS, query không hợp lệ, quá complexity limit.
 *
 * Ưu tiên `graphQLErrors[0].message` (server đã quyết định message này có an
 * toàn để trả — xem M4 §6.1). Không có thì mới rơi về thông điệp Việt cho
 * tình huống không có message nào từ server; nguyên nhân gốc giữ ở `cause` để
 * dev đọc được trong console thay vì phải đoán.
 */
export function fromCombinedError(err: CombinedError): AppError {
  const fromServer = err.graphQLErrors?.[0]?.message;
  if (fromServer) return new AppError(fromServer, 'INTERNAL', { cause: err });
  if (err.networkError) {
    return new AppError('không gọi được máy chủ', 'INTERNAL', { cause: err.networkError });
  }
  return new AppError('lỗi hệ thống', 'INTERNAL', { cause: err });
}

/** Thông điệp để đưa lên UI, bất kể lỗi là loại gì. */
export function errorMessage(e: unknown): string {
  if (e instanceof AppError) return e.message;
  if (e instanceof Error && e.message) return e.message;
  return String(e);
}
