// CHẶN TÁI PHÁT F1 (cổng Oracle M5): `createDeck` là mutation DUY NHẤT trong 17
// call site mà quên gọi `afterMutation()` ⇒ tạo deck im lặng, không hiện, không
// lỗi. Nó lọt vì `afterMutation.test.ts` chỉ test **helper**, không test **bất
// kỳ hàm mutation nào** — nên test ở đây KHÔNG test lại helper một lần nữa.
//
// Cách chặn tái diễn cho M6 (sẽ thêm nhiều mutation mới): `MUTATIONS` ở đây là
// danh sách khai báo TẤT CẢ hàm mutation của app. Thêm mutation mới mà quên
// khai báo ⇒ `test_mutation_list_covers_every_exported_mutation` đỏ (buộc phải
// nghĩ tới `afterMutation`); khai báo mà quên gọi `afterMutation` ⇒ test
// `reread` của nó đỏ.
//
// Kỹ thuật đo: chạy mutation, đếm `fetch` TRƯỚC, đọc lại query liên quan, đếm
// `fetch` SAU. Không mock rồi assert lời gọi — đo số request thật qua
// `cache-first`, đó mới là cái quyết định người dùng thấy dữ liệu mới hay không.
import { afterEach, describe, expect, it, vi } from 'vitest';
import { resetStaleMarks, resetUrqlClient } from '../graphql/client';
import { queryClient } from '../rest/queryClient';
import { createCard, createDeck, fetchCards, fetchDecks, fetchDueCards, postReview } from '../srs/api';
import { answerDrill, loadDrillRound, postZhBingoScore, postZhImport, type DrillItem } from '../chinese/api';
import { fetchThieuHistory, postEnSeed, postThieu } from '../english/api';
import { fetchErrors, fetchProgress, postError, postProgress, runSync } from '../player/api';
import {
  createMilestone,
  createPath,
  createResource,
  createStage,
  createTopic,
  deleteMilestone,
  deletePath,
  deleteResource,
  deleteStage,
  deleteTopic,
  fetchPath,
  fetchPathRows,
  setStageStatus,
  setTopicStatus,
  updateMilestone,
  updatePath,
  updateResource,
  updateStage,
  updateTopic,
} from '../roadmap/api';
import {
  createBookmark,
  deleteBookmark,
  fetchBookmarks,
  setBookmarkStatus,
  updateBookmark,
} from '../bookmarks/api';
import * as srsApi from '../srs/api';
import * as chineseApi from '../chinese/api';
import * as englishApi from '../english/api';
import * as playerApi from '../player/api';
import * as roadmapApi from '../roadmap/api';
import * as bookmarksApi from '../bookmarks/api';

const CARD = {
  id: '7',
  deckId: '1',
  front: '你好',
  back: 'xin chào',
  pinyin: 'ni3 hao3',
  dueAt: 'x',
  state: 'new',
  tone: null,
  audioURL: null,
};

const REVIEW = {
  cardId: '7',
  dueAt: '2026-10-01',
  intervalDays: 3,
  stability: 1,
  difficulty: 1,
  reps: 1,
  fallback: true,
};

const MERGED = {
  decks: 1, cards: 2, reviews: 3, notes: 0,
  roadmapPaths: 0, roadmapStages: 0, roadmapMilestones: 0,
  roadmapTopics: 0, roadmapResources: 0, roadmapBookmarks: 0,
};

const ITEM: DrillItem = {
  card_id: '7',
  hanzi: '你好',
  pinyin: 'ni3 hao3',
  pinyin_marks: 'nǐ hǎo',
  tone: '3 3',
  pair: '3-3',
};

const ENTRY = { lang: 'en', term: 'photograph', reading: '/ˈfəʊtəɡrɑːf/', gloss: 'bức ảnh' };
const THIEU_SESSION = {
  id: '1', session: '2026-09-23',
  scores: [{ axis: 'A', value: 3 }, { axis: 'B', value: 4 }, { axis: 'C', value: 3 },
    { axis: 'D', value: 4 }, { axis: 'E', value: 3 }, { axis: 'F', value: 4 },
    { axis: 'G', value: 3 }, { axis: 'H', value: 4 }],
  average: 3.4, note: '', createdAt: 'x',
};
const ERROR_ENTRY = {
  id: 'e1', cardId: '7', expected: 'ni hao', transcript: 'ni hau', wrong: ['hau'], createdAt: 'x',
};
const SHADOW = { cardId: '7', loops: 2, rate: 1, updatedAt: 'x' };
const DECK = { id: '2', guid: 'g2', name: 'NEW', lang: 'zh', createdAt: 'x' };

const ROADMAP_PROGRESS = {
  stages: 1, topicsTotal: 2, topicsRequired: 2, topicsOptional: 0, topicsDone: 1,
  topicsInProgress: 0, topicsLocked: 0, percent: 50, lastCompletedAt: 'x', completedInRange: 1,
};
const SUMMARY = {
  stages: 1, topicsTotal: 2, topicsRequired: 2, topicsDone: 1, topicsInProgress: 0, percent: 50,
};
const PATH = {
  id: '1', guid: 'p1', slug: 'trung', title: 'Tiếng Trung', overview: '', language: 'zh',
  isBuiltin: true, createdAt: 'x', updatedAt: 'x',
};
const TREE = {
  ...PATH,
  progress: ROADMAP_PROGRESS,
  stages: [
    {
      id: '10', guid: 's10', pathId: '1', slug: 'zh-g0', title: 'Chặng 0', goal: '',
      position: 0, durationWeeks: 4, status: 'IN_PROGRESS', statusNote: '', completedAt: null,
      deckId: null, deck: null, terrain: 'MEADOW', direction: 'UP',
      topics: [
        {
          id: '100', guid: 't100', stageId: '10', title: 'Chào', why: '', activityList: [],
          position: 0, status: 'DONE', statusNote: '', completedAt: 'x', isOptional: false,
          mapX: null, mapY: null, level: 'DONE', point: { x: 500, y: 1800 }, mapPinned: false,
          resources: [],
        },
        {
          id: '101', guid: 't101', stageId: '10', title: 'Số đếm', why: '', activityList: [],
          position: 1, status: 'NOT_STARTED', statusNote: '', completedAt: null, isOptional: false,
          mapX: null, mapY: null, level: 'CURRENT', point: { x: 500, y: 1600 }, mapPinned: false,
          resources: [],
        },
      ],
      milestones: [],
    },
  ],
};

const BOOKMARK = {
  id: '55', guid: 'b55', title: 'Bài học 1', url: 'https://example.com', note: '',
  tags: 'hsk3', tagList: ['hsk3'], status: 'TO_READ', createdAt: 'x', updatedAt: 'x',
};

/** Payload `data` trả về cho mọi operation (mọi query trả list rỗng hợp lệ). */
function dataFor(name: string): Record<string, unknown> {
  switch (name) {
    case 'Decks': return { decks: [{ id: '1', guid: 'g', name: 'OLD', lang: 'zh', createdAt: 'x' }] };
    case 'DueCards':
    case 'Cards': return { dueCards: [CARD], cards: [CARD] };
    case 'CreateDeck': return { createDeck: { ok: true, deck: DECK, error: null } };
    case 'CreateCard': return { createCard: { ok: true, card: CARD, error: null } };
    case 'RecordReview': return { recordReview: { ok: true, review: REVIEW, error: null } };
    case 'GradeTone': return { gradeTone: { ok: true, grade: { grade: 4, score: 1, exact: true, hit: 2 }, error: null } };
    case 'ImportHSK':
      return { importHSK: { ok: true, result: { deckId: '1', deck: 'HSK1', level: 'HSK1', cardsAdded: 36, cardsTotal: 36, dictAdded: 30 }, error: null } };
    case 'SeedEnglish': return { seedEnglish: { ok: true, result: { enDict: 1, pvoAdded: 2, tmrndAdded: 3 }, error: null } };
    case 'AppendThieu': return { appendThieu: { ok: true, session: THIEU_SESSION, error: null } };
    case 'ThieuSessions': return { thieuSessions: [THIEU_SESSION] };
    case 'ThieuAxes': return { thieuAxes: [] };
    case 'Chunk': return { chunk: { ok: true, chunks: [{ text: 'I', kind: 'FUNCTION' }], error: null } };
    case 'Stress': return { stress: { term: 'a', ipa: '', stress: '', exception: false, note: '', fromDict: true } };
    case 'EnglishSearch': return { englishSearch: [ENTRY] };
    case 'RecordShadowProgress': return { recordShadowProgress: { ok: true, progress: SHADOW, error: null } };
    case 'ShadowProgress': return { shadowProgress: SHADOW };
    case 'AppendError': return { appendError: { ok: true, entry: ERROR_ENTRY, error: null } };
    case 'Errors': return { errors: [ERROR_ENTRY] };
    case 'TopErrors': return { topErrors: [{ word: 'ni', count: 3 }] };
    case 'ErrorSuggestions': return { errorSuggestions: [{ cardId: '7', front: 'a', back: 'b', errors: 2 }] };
    case 'Sync': return { sync: { ok: true, merged: MERGED, conflicts: [], warnings: [], lastSyncAt: 'x', error: null } };
    case 'InsightTopErrors': return { insightTopErrors: { ok: true, errors: [{ word: 'ni', count: 3, cardId: '7', front: '你' }], error: null } };
    case 'Paths': return { paths: [{ path: PATH, summary: SUMMARY }] };
    case 'PathTree': return { path: TREE };
    case 'Bookmarks': return { bookmarks: [BOOKMARK] };
    default: return payloadForMutation(name);
  }
}

// ── payload mutation roadmap + bookmark ────────────────────────────────────
//
// Mọi mutation đều bọc `ok` + payload riêng. Trả payload `null` là hợp lệ: test
// này chỉ đo "mutation có làm mông cache không", còn hình dạng payload được
// `roadmap/api.test.ts` và `bookmarks/api.test.ts` kiểm riêng.
function payloadForMutation(name: string): Record<string, unknown> {
  const fields: Record<string, string> = {
    CreatePath: 'path',
    UpdatePath: 'path',
    CreateStage: 'stage',
    UpdateStage: 'stage',
    SetStageStatus: 'stage',
    CreateTopic: 'topic',
    UpdateTopic: 'topic',
    SetTopicStatus: 'topic',
    CreateResource: 'resource',
    UpdateResource: 'resource',
    CreateMilestone: 'milestone',
    UpdateMilestone: 'milestone',
  };
  // KHÓA CỦA `data` LÀ TÊN FIELD, KHÔNG PHẢI TÊN OPERATION: operation
  // `UpdateBookmark` trả `{ data: { updateBookmark: … } }`. Nhầm 2 thứ này là
  // `data.updateBookmark` = undefined ⇒ client đọc `undefined.bookmark` và ném
  // TypeError, tưởng là lỗi cache.
  const field = `${name.charAt(0).toLowerCase()}${name.slice(1)}`;

  // Bookmark trả chính nó về; các hàm client đó ném "lỗi hệ thống" khi payload
  // `ok` mà entity null, nên stub phải trả entity thật.
  if (field === 'createBookmark' || field === 'updateBookmark' || field === 'setBookmarkStatus') {
    return { [field]: { ok: true, bookmark: BOOKMARK, error: null } };
  }
  const key = fields[name];
  if (key) return { [field]: { ok: true, [key]: null, error: null } };
  if (name.startsWith('Delete')) return { [field]: { ok: true, error: null } };
  return {};
}

function stubServer() {
  const fetchMock = vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    let name = new URL(String(input), 'http://localhost').searchParams.get('operationName') ?? '';
    if (!name && init?.body) name = (JSON.parse(init.body as string).operationName as string) ?? '';
    return new Response(JSON.stringify({ data: dataFor(name) }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

afterEach(() => {
  vi.unstubAllGlobals();
  resetUrqlClient();
  resetStaleMarks();
  queryClient.clear();
});

interface MutationCase {
  name: string;
  /** Chạy mutation. */
  run: () => Promise<unknown>;
  /** Đọc lại query liên quan — phải RA MẠNG sau mutation. */
  reread: () => Promise<unknown>;
  /** Operation của query dùng để reread (để payload đúng). */
  rereadOp: string;
}

/**
 * DANH SÁCH MUTATION — nguồn duy nhất cho test này và cho
 * `test_mutation_list_covers_every_exported_mutation` bên dưới.
 */
const MUTATIONS: MutationCase[] = [
  { name: 'srs/api/createDeck', run: () => createDeck('NEW'), reread: fetchDecks, rereadOp: 'Decks' },
  { name: 'srs/api/postReview', run: () => postReview('7', 3), reread: () => fetchDueCards('1'), rereadOp: 'DueCards' },
  { name: 'srs/api/createCard', run: () => createCard('1', { front: 'a', back: 'b' }), reread: () => fetchCards('1'), rereadOp: 'Cards' },
  { name: 'chinese/api/answerDrill', run: () => answerDrill(ITEM, '3 3'), reread: () => loadDrillRound('1'), rereadOp: 'Cards' },
  { name: 'chinese/api/postZhBingoScore', run: () => postZhBingoScore('1', [{ card_id: '7', correct: true }]), reread: () => loadDrillRound('1'), rereadOp: 'Cards' },
  { name: 'chinese/api/postZhImport', run: () => postZhImport('HSK1', 'HSK1'), reread: fetchDecks, rereadOp: 'Decks' },
  { name: 'english/api/postEnSeed', run: () => postEnSeed(), reread: fetchDecks, rereadOp: 'Decks' },
  { name: 'english/api/postThieu', run: () => postThieu('2026-09-23', { A: 3 }), reread: fetchThieuHistory, rereadOp: 'ThieuSessions' },
  { name: 'player/api/postProgress', run: () => postProgress('7', 2, 1), reread: () => fetchProgress('7'), rereadOp: 'ShadowProgress' },
  { name: 'player/api/postError', run: () => postError({ expected: 'ni hao', transcript: 'ni hau', wrong: ['hau'] }), reread: () => fetchErrors(undefined, 50), rereadOp: 'Errors' },
  { name: 'player/api/runSync', run: () => runSync(), reread: fetchDecks, rereadOp: 'Decks' },

  // ── M6: roadmap. Tất cả đọc lại `PathTree` vì đó là cây roadmap; `Paths` là
  // list rút gọn, `Bookmarks` là kho link — mỗi mutation chỉ cần chứng minh
  // cache của nó đã mông, đọc 1 query liên quan là đủ.
  { name: 'roadmap/api/createPath', run: () => createPath({ slug: 'vi', title: 'VI' }), reread: fetchPathRows, rereadOp: 'Paths' },
  { name: 'roadmap/api/updatePath', run: () => updatePath('trung', { title: 'T' }), reread: fetchPathRows, rereadOp: 'Paths' },
  { name: 'roadmap/api/deletePath', run: () => deletePath('trung'), reread: fetchPathRows, rereadOp: 'Paths' },
  { name: 'roadmap/api/createStage', run: () => createStage('trung', { slug: 'zh-g5', title: 'G5' }), reread: () => fetchPath('trung'), rereadOp: 'PathTree' },
  { name: 'roadmap/api/updateStage', run: () => updateStage('10', { terrain: 'SNOW' as never }), reread: () => fetchPath('trung'), rereadOp: 'PathTree' },
  { name: 'roadmap/api/deleteStage', run: () => deleteStage('10'), reread: () => fetchPath('trung'), rereadOp: 'PathTree' },
  { name: 'roadmap/api/setStageStatus', run: () => setStageStatus('10', 'DONE'), reread: () => fetchPath('trung'), rereadOp: 'PathTree' },
  { name: 'roadmap/api/createTopic', run: () => createTopic('10', { title: 'Mới' }), reread: () => fetchPath('trung'), rereadOp: 'PathTree' },
  { name: 'roadmap/api/updateTopic', run: () => updateTopic('100', { title: 'Sửa' }), reread: () => fetchPath('trung'), rereadOp: 'PathTree' },
  { name: 'roadmap/api/deleteTopic', run: () => deleteTopic('100'), reread: () => fetchPath('trung'), rereadOp: 'PathTree' },
  { name: 'roadmap/api/setTopicStatus', run: () => setTopicStatus('101', 'DONE'), reread: () => fetchPath('trung'), rereadOp: 'PathTree' },
  { name: 'roadmap/api/createResource', run: () => createResource('100', { title: 'T' }), reread: () => fetchPath('trung'), rereadOp: 'PathTree' },
  { name: 'roadmap/api/updateResource', run: () => updateResource('200', { title: 'T' }), reread: () => fetchPath('trung'), rereadOp: 'PathTree' },
  { name: 'roadmap/api/deleteResource', run: () => deleteResource('200'), reread: () => fetchPath('trung'), rereadOp: 'PathTree' },
  { name: 'roadmap/api/createMilestone', run: () => createMilestone('10', 'Mốc'), reread: () => fetchPath('trung'), rereadOp: 'PathTree' },
  { name: 'roadmap/api/updateMilestone', run: () => updateMilestone('300', { text: 'M' }), reread: () => fetchPath('trung'), rereadOp: 'PathTree' },
  { name: 'roadmap/api/deleteMilestone', run: () => deleteMilestone('300'), reread: () => fetchPath('trung'), rereadOp: 'PathTree' },

  // ── M6: bookmark. Cả 4 đọc lại `Bookmarks` — kho link là 1 list phẳng.
  { name: 'bookmarks/api/createBookmark', run: () => createBookmark({ title: 'L' }), reread: () => fetchBookmarks(), rereadOp: 'Bookmarks' },
  { name: 'bookmarks/api/updateBookmark', run: () => updateBookmark('55', { title: 'Sửa' }), reread: () => fetchBookmarks(), rereadOp: 'Bookmarks' },
  { name: 'bookmarks/api/deleteBookmark', run: () => deleteBookmark('55'), reread: () => fetchBookmarks(), rereadOp: 'Bookmarks' },
  { name: 'bookmarks/api/setBookmarkStatus', run: () => setBookmarkStatus('55', 'DONE'), reread: () => fetchBookmarks(), rereadOp: 'Bookmarks' },
];

describe('test_every_mutation_invalidates_urql_cache', () => {
  it.each(MUTATIONS)('$name: đọc lại sau mutation phải RA MẠNG', async (m) => {
    const fetchMock = stubServer();

    // 1) Warm cache: đọc query liên quan TRƯỚC khi mutation.
    await m.reread();
    expect(dataFor(m.rereadOp), 'payload reread phải khác rỗng').not.toEqual({});

    // 2) Chạy mutation.
    await m.run();
    const afterMutation = fetchMock.mock.calls.length;
    const mutationCalls = fetchMock.mock.calls
      .map((c) => (c[1]?.body ? (JSON.parse(c[1].body as string).operationName as string) : ''))
      .filter((n) => n && n !== m.rereadOp);
    expect(mutationCalls.length, 'mutation phải thật sự ra mạng').toBeGreaterThan(0);

    // 3) Đọc lại. `cache-first` — nếu cache còn "sạch" thì KHÔNG ra mạng ⇒ đọc
    //    trúng dữ liệu cũ ⇒ đúng bug F1.
    await m.reread();
    expect(
      fetchMock.mock.calls.length,
      `${m.name} không invalidate cache: đọc lại ${m.rereadOp} bị trả từ cache cũ`,
    ).toBeGreaterThan(afterMutation);
  });
});

describe('test_mutation_list_covers_every_exported_mutation', () => {
  // Mọi hàm `async` export từ 4 module client đều là mutation (hàm đọc thì
  // tên `fetch*`). Liệt kê thủ công ở đây: thêm hàm mutation mà quên thêm vào
  // `MUTATIONS` thì test này đỏ — buộc phải nghĩ tới `afterMutation`.
  //
  // Kiểu là `Record<string, unknown>` chứ không phải kiểu module: giao kiểu
  // module thật khiến `vue-tsc` bắt lỗi phân phối khi 4 module có tập export
  // khác nhau (intersection khác rỗng) — lỗi kiểu, không phải lỗi logic.
  const MODULES: Array<[string, Record<string, unknown>]> = [
    ['srs/api', srsApi],
    ['chinese/api', chineseApi],
    ['english/api', englishApi],
    ['player/api', playerApi],
    ['roadmap/api', roadmapApi],
    ['bookmarks/api', bookmarksApi],
  ];
  const MUTATING_FNS: Array<[string, string]> = [
    ['srs/api', 'createDeck'],
    ['srs/api', 'postReview'],
    ['srs/api', 'createCard'],
    ['chinese/api', 'answerDrill'],
    ['chinese/api', 'postZhBingoScore'],
    ['chinese/api', 'postZhImport'],
    ['english/api', 'postEnSeed'],
    ['english/api', 'postThieu'],
    ['player/api', 'postProgress'],
    ['player/api', 'postError'],
    ['player/api', 'runSync'],
  ];

  it('mọi hàm mutation export đều có trong danh sách kiểm', () => {
    const declared = new Set(MUTATIONS.map((m) => m.name));
    const namespaces = new Map(MODULES);

    const missing = MUTATING_FNS.filter(([mod, fn]) => {
      const ns = namespaces.get(mod);
      expect(ns, `không có namespace cho ${mod}`).toBeTruthy();
      expect(typeof ns![fn], `${mod} không export ${fn}`).toBe('function');
      return !declared.has(`${mod}/${fn}`);
    });
    expect(missing.map(([mod, fn]) => `${mod}/${fn}`), 'thiếu mutation trong danh sách test').toEqual([]);
  });

  it('không có hàm mutation nào bị bỏ sót khỏi module', () => {
    // Chiều ngược: mọi export có TÊN kiểu mutation đều phải nằm trong danh sách
    // — đây là chỗ bắt M6 khi thêm mutation mới mà quên nghĩ tới `afterMutation`.
    //
    // Tên `post*` KHÔNG đáng tin: app v1 gọi chấm thanh / tách chunk qua REST
    // `POST`, nên tên giữ lại `postZhDrillGrade` / `postEnChunks`, nhưng ở
    // app-v2 chúng là **query** (`gradeTone`, `chunk`) — server trả kết quả thuần
    // và không có `ok/error` payload. Đưa chúng vào `MUTATIONS` sẽ khẳng định
    // sai (đòi `afterMutation` sau một thao tác không ghi gì). Liệt kê tường minh
    // kèm lý do, thay vì nới regex đến mức không bắt được gì.
    const NAMED_LIKE_MUTATION_BUT_IS_QUERY = new Set([
      'chinese/api/postZhDrillGrade', // → query `gradeTone`
      'english/api/postEnChunks', // → query `chunk`
      // Không chạm server: bật/tắt cờ thử nghiệm trong localStorage.
      'player/api/setSyncUiEnabled',
    ]);

    const declared = new Set(MUTATIONS.map((m) => m.name));
    // Động từ hành động của GraphQL: đủ rộng để bắt `delete*`/`update*`/`set*`
    // (M6 chắc chắn sẽ thêm) chứ không chỉ `post*`/`create*`. Đã kiểm chứng: thêm
    // `deleteDeck` mà không khai báo thì test này đỏ.
    const VERB = /^(post|create|run|answer|record|append|seed|delete|update|set|import|upsert|add)/;

    const unlisted: string[] = [];
    for (const [mod, ns] of MODULES) {
      for (const name of Object.keys(ns)) {
        if (!VERB.test(name)) continue;
        const key = `${mod}/${name}`;
        if (NAMED_LIKE_MUTATION_BUT_IS_QUERY.has(key)) continue;
        if (!declared.has(key)) unlisted.push(key);
      }
    }
    expect(unlisted, 'hàm mutation mới chưa được đưa vào danh sách test').toEqual([]);
  });
});
