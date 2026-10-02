// M5 — client `srs` (deck / card / ôn) + tra từ điển Trung qua GraphQL.
//
// Mọi hàm ở đây là "1 hàm = 1 operation" để màn gọi được thẳng
// (`await fetchDecks()`) mà không phải tự `runQuery` + tự kiểm `payload.ok`.
// `afterMutation()` được gọi ở đây, KHÔNG gọi ở component: xem
// `src/lib/afterMutation.ts`.
//
// LỖI: `assertPayloadOk()` ném `AppError` mang `error.message` NGUYÊN VĂN của
// server (tiếng Việt) — UI hiện thẳng, không dịch lần 2. Lỗi hệ thống server
// đã che thành `"lỗi hệ thống"` + `INTERNAL` nên client cũng không đoán bừa.
import { runMutation, runQuery } from '../graphql/client';
import { assertPayloadOk } from '../graphql/errors';
import {
  Cards,
  CreateCardMutation,
  CreateDeckMutation,
  Decks,
  DictSearch,
  DueCards,
  RecordReviewMutation,
  type Card,
  type Deck,
  type DictEntry,
  type ReviewResult,
} from '../graphql/operations';
import { afterMutation } from '../lib/afterMutation';

export type { Card, Deck, DictEntry, ReviewResult };

export async function fetchDecks(): Promise<Deck[]> {
  return (await runQuery(Decks)).decks;
}

export async function createDeck(name: string, lang = 'zh'): Promise<Deck> {
  const data = await runMutation(CreateDeckMutation, { name, lang });
  assertPayloadOk(data.createDeck);
  if (!data.createDeck.deck) throw new Error('lỗi hệ thống');
  // F1: THIẾU dòng này thì `Hoc.vue` + `Recorder.vue` gọi `createDeck()` rồi
  // `fetchDecks()` sẽ đọc cache cũ (`cache-first`) ⇒ deck vừa tạo không hiện,
  // không báo lỗi. `CreateDeck` trả `Deck` có `id` nhưng cacheExchange CHỈ tự
  // invalidate khi CẢ query lẫn mutation đều có `__typename` (xem
  // `graphql/client.ts`) ⇒ `afterMutation()` là cơ chế DUY NHẤT.
  // Test chặn tái diễn: `lib/mutationInvalidation.test.ts`.
  await afterMutation();
  return data.createDeck.deck;
}

export async function fetchCards(deckId: string): Promise<Card[]> {
  return (await runQuery(Cards, { deckId })).cards;
}

/** Hàng đợi ôn: quá hạn HOẶC chưa từng ôn (xem doc `dueCards` trong schema). */
export async function fetchDueCards(deckId: string): Promise<Card[]> {
  return (await runQuery(DueCards, { deckId })).dueCards;
}

/**
 * Tìm 1 thẻ theo id — phục vụ link `/review?card=<id>` mà `ErrorBook` render
 * (M6b §4 tính năng phục hồi #2: dòng từ sai phải nhảy được tới thẻ).
 *
 * **Vì sao phải quét thay vì 1 query:** schema KHÔNG có `card(id:)` — `Query`
 * chỉ có `cards(deckId:)` và `dueCards(deckId:)`, tức là cần `deckId` mà link
 * `/review?card=…` không mang theo. Nên đường đi duy nhất bên trong phạm vi
 * client là: `Decks` (luôn vài chục dòng) rồi `cards(deckId)` từng deck, **dừng
 * sớm ngay khi thấy**. `Decks` được cache-first của urql giữ, và mỗi
 * `cards(deckId)` cũng vậy ⇒ lần bấm lại gần như không tốn request.
 *
 * ⚠️ Nợ biết (ghi cho M7, KHÔNG tự vá ở đây vì cần đổi `api/**`): thêm
 * `Query.card(id: ID!)` thì cả hàm này còn 1 dòng `runQuery`. Đây là lý do
 * `M6-remediation F2` không thêm operation mới vào `operations.ts`.
 */
export async function fetchCardById(cardId: string): Promise<Card | null> {
  if (cardId === '') return null;
  const { decks } = await runQuery(Decks);
  for (const d of decks) {
    const hit = (await runQuery(Cards, { deckId: d.id })).cards.find((c) => c.id === cardId);
    if (hit) return hit;
  }
  return null;
}

export async function postReview(cardId: string, grade: number): Promise<ReviewResult> {
  const data = await runMutation(RecordReviewMutation, { cardId, grade });
  assertPayloadOk(data.recordReview);
  if (!data.recordReview.review) throw new Error('lỗi hệ thống');
  await afterMutation();
  return data.recordReview.review;
}

export interface NewCard {
  front: string;
  back: string;
  pinyin?: string;
}

export async function createCard(deckId: string, input: NewCard): Promise<Card> {
  const data = await runMutation(CreateCardMutation, {
    deckId,
    // `CardInput.pinyin` là `String!` (bắt buộc) — thẻ tạo từ Recorder/Reader
    // không có pinyin nên gửi chuỗi rỗng, đúng như app v1 để dùng.
    input: { front: input.front, back: input.back, pinyin: input.pinyin ?? '' },
  });
  assertPayloadOk(data.createCard);
  if (!data.createCard.card) throw new Error('lỗi hệ thống');
  await afterMutation();
  return data.createCard.card;
}

/** Tra từ điển Trung. `limit` null ở server = 10 (xem doc `dictSearch`). */
export async function dictSearch(q: string, limit = 10): Promise<DictEntry[]> {
  return (await runQuery(DictSearch, { q, limit })).dictSearch;
}
