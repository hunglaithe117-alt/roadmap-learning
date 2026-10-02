// M6 — client kho link. Độc lập cây roadmap: bookmark không có `position` và
// không có `completedAt`, thứ tự là thứ tự thêm (server đã `ORDER BY id`).
//
// ⚠️ `BookmarkStatus` KHÁC `Status` của node roadmap. Gửi `NOT_STARTED` cho
// bookmark là 400 chứ không phải rơi về mặc định — xem
// `phases/task-memory/stack-v2-m6a.md` §5.2.
import { runMutation, runQuery } from '../graphql/client';
import { assertPayloadOk } from '../graphql/errors';
import {
  Bookmarks,
  CreateBookmarkMutation,
  DeleteBookmarkMutation,
  SetBookmarkStatusMutation,
  UpdateBookmarkMutation,
  type Bookmark,
  type BookmarkStatus,
} from '../graphql/operations';
import { afterMutation } from '../lib/afterMutation';

export type { Bookmark, BookmarkStatus };

export interface BookmarkFilter {
  status?: BookmarkStatus | null;
  tag?: string | null;
}

export async function fetchBookmarks(filter: BookmarkFilter = {}): Promise<Bookmark[]> {
  const { bookmarks } = await runQuery(Bookmarks, {
    status: filter.status ?? null,
    tag: filter.tag ?? null,
  });
  return bookmarks;
}

export interface NewBookmark {
  title: string;
  url?: string | null;
  note?: string | null;
  tags?: string[];
  status?: BookmarkStatus | null;
}

/**
 * Tag nhập tay chuẩn hoá TRƯỚC khi gửi: server cũng chuẩn hoá (trim + hạ chữ
 * thường + bỏ trùng) nhưng chỉ sau khi đã kiểm độ dài 40 ký tự/10 tag — nếu để
 * client gửi thẳng thì " HSK3 " sẽ bị đếm 6 ký tự và có thể vượt ngưỡng.
 */
export function normalizeTags(raw: string[]): string[] {
  const seen = new Set<string>();
  for (const t of raw) {
    const v = t.trim().toLowerCase();
    if (v !== '') seen.add(v);
  }
  return [...seen];
}

export async function createBookmark(input: NewBookmark): Promise<Bookmark> {
  const data = await runMutation(CreateBookmarkMutation, {
    input: {
      title: input.title,
      url: input.url ?? null,
      note: input.note ?? null,
      tags: input.tags ? normalizeTags(input.tags) : null,
      status: input.status ?? null,
    },
  });
  assertPayloadOk(data.createBookmark);
  if (!data.createBookmark.bookmark) throw new Error('lỗi hệ thống');
  await afterMutation();
  return data.createBookmark.bookmark;
}

export interface BookmarkPatch {
  title?: string;
  url?: string;
  note?: string;
  /** `[]` = XOÁ HẾT tag. Không truyền = giữ nguyên. */
  tags?: string[];
  clearUrl?: boolean;
}

export async function updateBookmark(id: string, patch: BookmarkPatch): Promise<Bookmark> {
  const data = await runMutation(UpdateBookmarkMutation, { id, patch });
  assertPayloadOk(data.updateBookmark);
  if (!data.updateBookmark.bookmark) throw new Error('lỗi hệ thống');
  await afterMutation();
  return data.updateBookmark.bookmark;
}

export async function deleteBookmark(id: string): Promise<void> {
  const data = await runMutation(DeleteBookmarkMutation, { id });
  assertPayloadOk(data.deleteBookmark);
  await afterMutation();
}

export async function setBookmarkStatus(id: string, status: BookmarkStatus): Promise<Bookmark> {
  const data = await runMutation(SetBookmarkStatusMutation, { id, status });
  assertPayloadOk(data.setBookmarkStatus);
  if (!data.setBookmarkStatus.bookmark) throw new Error('lỗi hệ thống');
  await afterMutation();
  return data.setBookmarkStatus.bookmark;
}
