// Toàn bộ operation GraphQL của SPA, viết tay (`gql` template) theo
// `api/graph/schema/*.graphqls` — KHÔNG có codegen client (STACK-V2 §1:
// `@urql/codegen` đã unpublish, 30 operation thì viết tay rẻ hơn).
//
// Kiểu TS đặt cạnh operation thay vì sinh tự động: mỗi `interface` ở đây là
// bản CHỌN LỌC đúng bằng selection set phía trên nó, không phải bản sao của
// type server. Sửa selection set mà quên sửa interface thì `tsc` báo đỏ —
// đó là cảnh báo duy nhất khi không có codegen.
//
// Danh sách dùng ở màn nào ghi ở comment từng query.

import { gql } from '@urql/core';

/** `UserError` — khai lại ở đây để không import type server sang client. */
export interface PayloadErrorField {
  message: string;
  code: string;
}

// ─────────────────────────────────────────────────────────────────────────────
// srs — deck / card / ôn
// ─────────────────────────────────────────────────────────────────────────────

export interface Deck {
  id: string;
  guid: string;
  name: string;
  lang: string;
  createdAt: string;
}

export interface Card {
  id: string;
  deckId: string;
  front: string;
  back: string;
  pinyin: string;
  dueAt: string;
  state: string;
  tone: string | null;
  audioURL: string | null;
}

export interface ReviewResult {
  cardId: string;
  dueAt: string;
  intervalDays: number;
  stability: number;
  difficulty: number;
  reps: number;
  fallback: boolean;
}

// `__typename` KHÔNG phải thừa: nó là ĐIỀU KIỆN để `cacheExchange` của urql tự
// loại entity mà mutation vừa ghi. Cơ chế: exchange đọc `__typename` + `id` để
// ghi entity vào document cache, rồi so với `__typename` + `id` của các query
// đang cache để quyết định cái nào phải làm mới.
//
// Đo thật trên `@urql/core` 6.0.3 (xem `graphql/__typename.test.ts`):
//   - có `__typename` ở cả query lẫn mutation → đọc lại `cache-first` RA MẠNG
//     (cache tự invalidate);
//   - thiếu `__typename`           → đọc lại `cache-first` TRẢ TỪ CACHE CŨ.
//
// gqlgen KHÔNG tự chèn `__typename` vào câu query, nên phải viết tay ở đây.
// Kiểu TS (`interface Deck`/`Card`) không khai `__typename` vì đó là field của
// GraphQL chứ không phải dữ liệu mà app dùng — `runQuery` trả object có field
// thừa, vô hại.
/** Export để `__typename.test.ts` kiểm được — xem comment `__typename` ở trên. */
export const DECK_FIELDS = gql`
  fragment DeckFields on Deck {
    __typename
    id
    guid
    name
    lang
    createdAt
  }
`;

/** Export để `__typename.test.ts` kiểm được — xem comment `__typename` ở trên. */
export const CARD_FIELDS = gql`
  fragment CardFields on Card {
    __typename
    id
    deckId
    front
    back
    pinyin
    dueAt
    state
    tone
    audioURL
  }
`;

/** Hoc, Review, ZhPinyin, ZhBingo, ZhStroke, EnStress, EnPvo, ShadowPlayer, Reader, Recorder. */
export const Decks = gql<{ decks: Deck[] }>`
  ${DECK_FIELDS}
  query Decks {
    decks {
      ...DeckFields
    }
  }
`;

/** ShadowPlayer (nguồn playlist), ZhPinyin/ZhBingo (nguồn drill — xem `chinese/api.ts`). */
export const Cards = gql<{ cards: Card[] }, { deckId: string }>`
  ${CARD_FIELDS}
  query Cards($deckId: ID!) {
    cards(deckId: $deckId) {
      ...CardFields
    }
  }
`;

/** Review, EnStress, EnPvo, ShadowPlayer. */
export const DueCards = gql<{ dueCards: Card[] }, { deckId: string }>`
  ${CARD_FIELDS}
  query DueCards($deckId: ID!) {
    dueCards(deckId: $deckId) {
      ...CardFields
    }
  }
`;

/** Hoc, Recorder. */
export const CreateDeckMutation = gql<
  { createDeck: { ok: boolean; deck: Deck | null; error: PayloadErrorField } },
  { name: string; lang: string }
>`
  ${DECK_FIELDS}
  mutation CreateDeck($name: String!, $lang: String) {
    createDeck(name: $name, lang: $lang) {
      ok
      deck {
        ...DeckFields
      }
      error {
        message
        code
      }
    }
  }
`;

/** Recorder (lưu từ sai thành thẻ), Reader (lưu từ mới). */
export const CreateCardMutation = gql<
  { createCard: { ok: boolean; card: Card | null; error: PayloadErrorField } },
  { deckId: string; input: { front: string; back: string; pinyin: string } }
>`
  ${CARD_FIELDS}
  mutation CreateCard($deckId: ID!, $input: CardInput!) {
    createCard(deckId: $deckId, input: $input) {
      ok
      card {
        ...CardFields
      }
      error {
        message
        code
      }
    }
  }
`;

/** Review, EnStress, EnPvo, ZhPinyin (sau khi chấm drill), ZhStroke, bingo (chấm từng ô). */
export const RecordReviewMutation = gql<
  { recordReview: { ok: boolean; review: ReviewResult | null; error: PayloadErrorField } },
  { cardId: string; grade: number }
>`
  mutation RecordReview($cardId: ID!, $grade: Int!) {
    recordReview(input: { cardId: $cardId, grade: $grade }) {
      ok
      review {
        cardId
        dueAt
        intervalDays
        stability
        difficulty
        reps
        fallback
      }
      error {
        message
        code
      }
    }
  }
`;

// ─────────────────────────────────────────────────────────────────────────────
// content — tra từ, nét chữ, chunk, trọng âm, THIEU, reader, HSK
// ─────────────────────────────────────────────────────────────────────────────

export interface DictEntry {
  hanzi: string;
  pinyin: string;
  nghia: string;
}

export interface EnglishEntry {
  lang: string;
  term: string;
  reading: string;
  gloss: string;
}

export interface ToneGrade {
  grade: number;
  score: number;
  exact: boolean;
  hit: number;
}

export interface StressLookup {
  term: string;
  ipa: string;
  stress: string;
  exception: boolean;
  note: string;
  fromDict: boolean;
}

export interface StrokeIndexEntry {
  hanzi: string;
  strokeCount: number;
}

export interface StrokeStep {
  order: number;
  code: string;
  name: string;
}

export interface StrokeInfo {
  hanzi: string;
  pinyinMarks: string;
  level: string;
  strokeCount: number;
  steps: StrokeStep[];
}

export interface Chunk {
  text: string;
  kind: 'CONTENT' | 'FUNCTION';
}

export interface ThieuAxis {
  code: string;
  name: string;
  desc: string;
}

export interface ThieuScore {
  axis: string;
  value: number;
}

export interface ThieuSession {
  id: string;
  session: string;
  scores: ThieuScore[];
  average: number;
  note: string;
  createdAt: string;
}

export interface ReaderArticle {
  id: string;
  level: string;
  lang: string;
  title: string;
  text: string;
  source: string;
}

export interface ImportResult {
  deckId: string;
  deck: string;
  level: string;
  cardsAdded: number;
  cardsTotal: number;
  dictAdded: number;
}

export interface EnglishSeedResult {
  enDict: number;
  pvoAdded: number;
  tmrndAdded: number;
}

/** Hoc (tra từ điển), Reader (bấm từ Hán trong bài đọc). */
export const DictSearch = gql<{ dictSearch: DictEntry[] }, { q: string; limit: number }>`
  query DictSearch($q: String!, $limit: Int) {
    dictSearch(q: $q, limit: $limit) {
      hanzi
      pinyin
      nghia
    }
  }
`;

/** EnStress (tra trọng âm + từ điển Anh), Reader (tra từ trong bài đọc tiếng Anh). */
export const EnglishSearch = gql<{ englishSearch: EnglishEntry[] }, { q: string; limit: number }>`
  query EnglishSearch($q: String!, $limit: Int) {
    englishSearch(q: $q, limit: $limit) {
      lang
      term
      reading
      gloss
    }
  }
`;

/** ZhPinyin + ZhBingo (chấm ô). `exact` = đúng hết âm tiết = `correct` của app v1. */
export const GradeTone = gql<
  { gradeTone: { ok: boolean; grade: ToneGrade | null; error: PayloadErrorField } },
  { expected: string; answered: string }
>`
  query GradeTone($expected: String!, $answered: String!) {
    gradeTone(expected: $expected, answered: $answered) {
      ok
      grade {
        grade
        score
        exact
        hit
      }
      error {
        message
        code
      }
    }
  }
`;

/** EnStress (khóa dấu âm tiết trọng âm). */
export const Stress = gql<{ stress: StressLookup }, { word: string }>`
  query Stress($word: String!) {
    stress(word: $word) {
      term
      ipa
      stress
      exception
      note
      fromDict
    }
  }
`;

/** ZhStroke — index nhẹ 1 level khi `hanzi` null, chi tiết 1 chữ khi có `hanzi`. */
export const Strokes = gql<
  {
    strokes: {
      ok: boolean;
      index: StrokeIndexEntry[];
      info: StrokeInfo | null;
      error: PayloadErrorField;
    };
  },
  { level: string; hanzi: string | null }
>`
  query Strokes($level: String!, $hanzi: String) {
    strokes(level: $level, hanzi: $hanzi) {
      ok
      index {
        hanzi
        strokeCount
      }
      info {
        hanzi
        pinyinMarks
        level
        strokeCount
        steps {
          order
          code
          name
        }
      }
      error {
        message
        code
      }
    }
  }
`;

/** EnStress (tách chunk). Server là source-of-truth; `english/chunk.ts` chỉ là bản mirror. */
export const ChunkSentence = gql<
  { chunk: { ok: boolean; chunks: Chunk[]; error: PayloadErrorField } },
  { sentence: string }
>`
  query ChunkSentence($sentence: String!) {
    chunk(sentence: $sentence) {
      ok
      chunks {
        text
        kind
      }
      error {
        message
        code
      }
    }
  }
`;

/** EnThieu (8 trục + rubric). */
export const ThieuAxes = gql<{ thieuAxes: ThieuAxis[] }>`
  query ThieuAxes {
    thieuAxes {
      code
      name
      desc
    }
  }
`;

/** EnThieu (bảng tiến bộ 8 trục). */
export const ThieuSessions = gql<{ thieuSessions: ThieuSession[] }>`
  query ThieuSessions {
    thieuSessions {
      id
      session
      scores {
        axis
        value
      }
      average
      note
      createdAt
    }
  }
`;

/** EnThieu (lưu buổi học). */
export const AppendThieuMutation = gql<
  { appendThieu: { ok: boolean; session: ThieuSession | null; error: PayloadErrorField } },
  { input: { session: string | null; scores: ThieuScore[]; note: string | null } }
>`
  mutation AppendThieu($input: ThieuInput!) {
    appendThieu(input: $input) {
      ok
      session {
        id
        session
        scores {
          axis
          value
        }
        average
        note
        createdAt
      }
      error {
        message
        code
      }
    }
  }
`;

/** EnPvo (nạp bộ PVO/TMRND). */
export const SeedEnglishMutation = gql<{
  seedEnglish: { ok: boolean; result: EnglishSeedResult | null; error: PayloadErrorField };
}>`
  mutation SeedEnglish {
    seedEnglish {
      ok
      result {
        enDict
        pvoAdded
        tmrndAdded
      }
      error {
        message
        code
      }
    }
  }
`;

/** Chưa có màn nào gọi: v1 cũng chỉ có client + test. Mở cho CaiDat/M6 dùng lại. */
export const ImportHSKMutation = gql<
  { importHSK: { ok: boolean; result: ImportResult | null; error: PayloadErrorField } },
  { input: { level: string | null; deck: string | null } }
>`
  mutation ImportHSK($input: ImportHSKInput) {
    importHSK(input: $input) {
      ok
      result {
        deckId
        deck
        level
        cardsAdded
        cardsTotal
        dictAdded
      }
      error {
        message
        code
      }
    }
  }
`;

/** Reader — `level`/`id` null = không lọc. Danh sách level suy ra từ chính response này. */
export const ReaderArticles = gql<
  { readerArticles: ReaderArticle[] },
  { level: string | null; id: string | null }
>`
  query ReaderArticles($level: String, $id: String) {
    readerArticles(level: $level, id: $id) {
      id
      level
      lang
      title
      text
      source
    }
  }
`;

// ─────────────────────────────────────────────────────────────────────────────
// practice — shadowing, sổ lỗi
// ─────────────────────────────────────────────────────────────────────────────

export interface ShadowProgress {
  cardId: string;
  loops: number;
  rate: number;
  updatedAt: string;
}

export interface ErrorEntry {
  id: string;
  cardId: string | null;
  expected: string;
  transcript: string;
  wrong: string[];
  createdAt: string;
}

export interface TopErrorCount {
  word: string;
  count: number;
}

/** `insight.topErrors` — `TopErrorCount` cộng thêm đường nhảy tới thẻ. */
export interface TopErrorWithCard extends TopErrorCount {
  cardId: string | null;
  front: string | null;
}

export interface CardErrorCount {
  cardId: string;
  front: string;
  back: string;
  errors: number;
}

/** ShadowPlayer (khôi phục số vòng + tốc độ khi mở lại 1 thẻ). */
export const ShadowProgressQuery = gql<{ shadowProgress: ShadowProgress }, { cardId: string }>`
  query ShadowProgress($cardId: ID!) {
    shadowProgress(cardId: $cardId) {
      cardId
      loops
      rate
      updatedAt
    }
  }
`;

/** ShadowPlayer (lưu số vòng khi sang thẻ / bấm "＋1 lượt & lưu"). */
export const RecordShadowProgressMutation = gql<
  {
    recordShadowProgress: {
      ok: boolean;
      progress: ShadowProgress | null;
      error: PayloadErrorField;
    };
  },
  { input: { cardId: string; loops: number; rate: number } }
>`
  mutation RecordShadowProgress($input: ShadowProgressInput!) {
    recordShadowProgress(input: $input) {
      ok
      progress {
        cardId
        loops
        rate
        updatedAt
      }
      error {
        message
        code
      }
    }
  }
`;

/** ErrorBook (lịch sử lỗi). */
export const Errors = gql<
  { errors: ErrorEntry[] },
  { cardId: string | null; limit: number }
>`
  query Errors($cardId: ID, $limit: Int) {
    errors(cardId: $cardId, limit: $limit) {
      id
      cardId
      expected
      transcript
      wrong
      createdAt
    }
  }
`;

/** Recorder (lưu vào sổ lỗi sau 1 lần ghi âm). */
export const AppendErrorMutation = gql<
  { appendError: { ok: boolean; entry: ErrorEntry | null; error: PayloadErrorField } },
  { input: { cardId: string | null; expected: string; transcript: string; wrong: string[] } }
>`
  mutation AppendError($input: AppendErrorInput!) {
    appendError(input: $input) {
      ok
      entry {
        id
        cardId
        expected
        transcript
        wrong
        createdAt
      }
      error {
        message
        code
      }
    }
  }
`;

/** ErrorBook + Dashboard (top từ sai lặp). */
export const TopErrors = gql<{ topErrors: TopErrorCount[] }, { limit: number }>`
  query TopErrors($limit: Int) {
    topErrors(limit: $limit) {
      word
      count
    }
  }
`;

/**
 * Đường THỨ HAI lấy từ lịch sử `notes` (M6a). Khác `topErrors` ở chỗ mang
 * `cardId`/`front` ⇒ mỗi dòng biết nhảy tới thẻ nào. Cả 2 trả `null` khi lỗi
 * không gắn thẻ (luyện nói tự do, hoặc thẻ đã xoá mềm) — UI kiểm
 * `cardId !== null` mới render nút nhảy.
 */
export const InsightTopErrors = gql<
  { insightTopErrors: { ok: boolean; errors: TopErrorWithCard[]; error: PayloadErrorField } },
  { limit: number }
>`
  query InsightTopErrors($limit: Int) {
    insightTopErrors(limit: $limit) {
      ok
      errors {
        word
        count
        cardId
        front
      }
      error {
        message
        code
      }
    }
  }
`;

/** ErrorBook + Recorder (gợi ý ôn lại thẻ nhiều lỗi). */
export const ErrorSuggestions = gql<{ errorSuggestions: CardErrorCount[] }, { limit: number }>`
  query ErrorSuggestions($limit: Int) {
    errorSuggestions(limit: $limit) {
      cardId
      front
      back
      errors
    }
  }
`;

// ─────────────────────────────────────────────────────────────────────────────
// roadmap — cây 5 tầng + bản đồ game
// ─────────────────────────────────────────────────────────────────────────────
//
// `__typename` ở fragment là ĐIỀU KIỆN để `cacheExchange` tự loại entity, viết
// tay vì gqlgen không chèn (xem `DECK_FIELDS` ở trên). `afterMutation()` vẫn là
// lớp bảo đảm duy nhất — `__typename` chỉ giảm request thừa.

/** Trạng thái node do server gán; KHÔNG lưu DB. Xem `domain/roadmap.LevelState`. */
export type NodeLevel = 'DONE' | 'CURRENT' | 'LOCKED';
/** Trạng thái 1 hàng roadmap — khác hẳn `BookmarkStatus` bên dưới. */
export type NodeStatus = 'NOT_STARTED' | 'IN_PROGRESS' | 'DONE' | 'SKIPPED';
/**
 * 6 địa hình của bản đồ — TÊN HẰNG của `enum Terrain` ở
 * `api/graph/schema/common.graphqls` (đã khớp whitelist
 * `meadow/desert/snow/volcano/ocean/city` của `domain/roadmap` + CHECK
 * migration `00004`).
 *
 * Đây là danh sách DUY NHẤT trong toàn bộ SPA. `roadmap/map/terrain.ts` dùng
 * `Record<MapTerrain, TerrainSkin>` nên thêm 1 địa hình mà quên khai ở đây thì
 * `vue-tsc` báo đỏ — trước đó có 2 danh sách độc lập (`MapTerrain` ở đây và
 * `TERRAINS` trong `terrain.ts`) nên lệch nhau mà không ai thấy.
 */
export type MapTerrain = 'MEADOW' | 'DESERT' | 'SNOW' | 'VOLCANO' | 'OCEAN' | 'CITY';
export type MapDirection = 'UP' | 'RIGHT';
export type ResourceKind =
  | 'VIDEO'
  | 'ARTICLE'
  | 'TOOL'
  | 'APP'
  | 'BOOK'
  | 'COURSE'
  | 'SITE'
  | 'PODCAST'
  | 'CHANNEL';

export interface MapPoint {
  x: number;
  y: number;
}

export interface DeckRef {
  id: string;
  name: string;
  lang: string;
}

export interface RoadmapResource {
  id: string;
  topicId: string;
  title: string;
  url: string | null;
  kind: ResourceKind;
  note: string;
  position: number;
  createdAt: string;
  updatedAt: string;
}

export interface RoadmapTopic {
  id: string;
  stageId: string;
  title: string;
  why: string;
  activityList: string[];
  position: number;
  status: NodeStatus;
  statusNote: string;
  completedAt: string | null;
  isOptional: boolean;
  mapX: number | null;
  mapY: number | null;
  level: NodeLevel;
  point: MapPoint;
  mapPinned: boolean;
  resources: RoadmapResource[];
}

export interface RoadmapMilestone {
  id: string;
  stageId: string;
  text: string;
  position: number;
  createdAt: string;
  updatedAt: string;
}

export interface RoadmapStage {
  id: string;
  pathId: string;
  slug: string;
  title: string;
  goal: string;
  position: number;
  durationWeeks: number;
  status: NodeStatus;
  statusNote: string;
  completedAt: string | null;
  deckId: string | null;
  deck: DeckRef | null;
  terrain: MapTerrain;
  direction: MapDirection;
  topics: RoadmapTopic[];
  milestones: RoadmapMilestone[];
}

export interface Progress {
  stages: number;
  topicsTotal: number;
  topicsRequired: number;
  topicsOptional: number;
  topicsDone: number;
  topicsInProgress: number;
  topicsLocked: number;
  percent: number;
  lastCompletedAt: string | null;
  completedInRange: number;
}

export interface RoadmapPath {
  id: string;
  guid: string;
  slug: string;
  title: string;
  overview: string;
  language: string;
  isBuiltin: boolean;
  createdAt: string;
  updatedAt: string;
  stages: RoadmapStage[];
  progress: Progress;
}

export interface ProgressSummary {
  stages: number;
  topicsTotal: number;
  topicsRequired: number;
  topicsDone: number;
  topicsInProgress: number;
  percent: number;
}

export interface PathRow {
  path: RoadmapPath;
  summary: ProgressSummary;
}

/** `paths` — danh sách 1 dòng mỗi path kèm tiến độ (màn `/roadmap`). */
export const Paths = gql<{ paths: PathRow[] }>`
  query Paths {
    paths {
      path {
        __typename
        id
        guid
        slug
        title
        overview
        language
        isBuiltin
        createdAt
        updatedAt
      }
      summary {
        stages
        topicsTotal
        topicsRequired
        topicsDone
        topicsInProgress
        percent
      }
    }
  }
`;

/** `path(slug:)` — cây 5 tầng đầy đủ, 1 request đủ để vẽ bản đồ. */
export const PathTree = gql<{ path: RoadmapPath | null }, { slug: string }>`
  query PathTree($slug: String!) {
    path(slug: $slug) {
      __typename
      id
      guid
      slug
      title
      overview
      language
      isBuiltin
      createdAt
      updatedAt
      progress {
        stages
        topicsTotal
        topicsRequired
        topicsOptional
        topicsDone
        topicsInProgress
        topicsLocked
        percent
        lastCompletedAt
        completedInRange
      }
      stages {
        __typename
        id
        guid
        pathId
        slug
        title
        goal
        position
        durationWeeks
        status
        statusNote
        completedAt
        deckId
        terrain
        direction
        deck {
          id
          name
          lang
        }
        topics {
          __typename
          id
          guid
          stageId
          title
          why
          activityList
          position
          status
          statusNote
          completedAt
          isOptional
          mapX
          mapY
          level
          point {
            x
            y
          }
          mapPinned
          resources {
            __typename
            id
            guid
            topicId
            title
            url
            kind
            note
            position
            createdAt
            updatedAt
          }
        }
        milestones {
          __typename
          id
          guid
          stageId
          text
          position
          createdAt
          updatedAt
        }
      }
    }
  }
`;

export const CreatePathMutation = gql<
  { createPath: { ok: boolean; path: RoadmapPath | null; error: PayloadErrorField } },
  { input: { slug: string; title: string; overview?: string | null; language?: string | null } }
>`
  mutation CreatePath($input: PathInput!) {
    createPath(input: $input) {
      ok
      path {
        __typename
        id
        guid
        slug
        title
        overview
        language
        isBuiltin
        createdAt
        updatedAt
      }
      error {
        message
        code
      }
    }
  }
`;

export const UpdatePathMutation = gql<
  { updatePath: { ok: boolean; path: RoadmapPath | null; error: PayloadErrorField } },
  { slug: string; patch: { title?: string | null; overview?: string | null; language?: string | null } }
>`
  mutation UpdatePath($slug: String!, $patch: PathPatch!) {
    updatePath(slug: $slug, patch: $patch) {
      ok
      path {
        __typename
        id
        guid
        slug
        title
        overview
        language
        isBuiltin
        createdAt
        updatedAt
      }
      error {
        message
        code
      }
    }
  }
`;

export const DeletePathMutation = gql<
  { deletePath: { ok: boolean; error: PayloadErrorField } },
  { slug: string }
>`
  mutation DeletePath($slug: String!) {
    deletePath(slug: $slug) {
      ok
      error {
        message
        code
      }
    }
  }
`;

export const CreateStageMutation = gql<
  { createStage: { ok: boolean; stage: RoadmapStage | null; error: PayloadErrorField } },
  {
    pathSlug: string;
    input: {
      slug: string;
      title: string;
      goal?: string | null;
      position?: number | null;
      durationWeeks?: number | null;
      status?: NodeStatus | null;
      deckId?: string | null;
      terrain?: MapTerrain | null;
      direction?: MapDirection | null;
    };
  }
>`
  mutation CreateStage($pathSlug: String!, $input: StageInput!) {
    createStage(pathSlug: $pathSlug, input: $input) {
      ok
      stage {
        __typename
        id
        guid
        pathId
        slug
        title
        goal
        position
        durationWeeks
        status
        statusNote
        completedAt
        deckId
        terrain
        direction
      }
      error {
        message
        code
      }
    }
  }
`;

export const UpdateStageMutation = gql<
  { updateStage: { ok: boolean; stage: RoadmapStage | null; error: PayloadErrorField } },
  {
    id: string;
    patch: {
      title?: string | null;
      goal?: string | null;
      position?: number | null;
      durationWeeks?: number | null;
      deckId?: string | null;
      terrain?: MapTerrain | null;
      direction?: MapDirection | null;
    };
  }
>`
  mutation UpdateStage($id: ID!, $patch: StagePatch!) {
    updateStage(id: $id, patch: $patch) {
      ok
      stage {
        __typename
        id
        guid
        pathId
        slug
        title
        goal
        position
        durationWeeks
        status
        statusNote
        completedAt
        deckId
        terrain
        direction
      }
      error {
        message
        code
      }
    }
  }
`;

export const DeleteStageMutation = gql<
  { deleteStage: { ok: boolean; error: PayloadErrorField } },
  { id: string }
>`
  mutation DeleteStage($id: ID!) {
    deleteStage(id: $id) {
      ok
      error {
        message
        code
      }
    }
  }
`;

export const SetStageStatusMutation = gql<
  { setStageStatus: { ok: boolean; stage: RoadmapStage | null; error: PayloadErrorField } },
  { id: string; input: { status: NodeStatus; statusNote?: string | null } }
>`
  mutation SetStageStatus($id: ID!, $input: StatusInput!) {
    setStageStatus(id: $id, input: $input) {
      ok
      stage {
        __typename
        id
        status
        statusNote
        completedAt
      }
      error {
        message
        code
      }
    }
  }
`;

export const CreateTopicMutation = gql<
  { createTopic: { ok: boolean; topic: RoadmapTopic | null; error: PayloadErrorField } },
  {
    stageId: string;
    input: {
      title: string;
      why?: string | null;
      activities?: string[] | null;
      position?: number | null;
      isOptional?: number | null;
      mapX?: number | null;
      mapY?: number | null;
    };
  }
>`
  mutation CreateTopic($stageId: ID!, $input: TopicInput!) {
    createTopic(stageId: $stageId, input: $input) {
      ok
      topic {
        __typename
        id
        guid
        stageId
        title
        why
        activityList
        position
        status
        statusNote
        completedAt
        isOptional
        mapX
        mapY
        level
        point {
          x
          y
        }
        mapPinned
      }
      error {
        message
        code
      }
    }
  }
`;

export const UpdateTopicMutation = gql<
  { updateTopic: { ok: boolean; topic: RoadmapTopic | null; error: PayloadErrorField } },
  {
    id: string;
    patch: {
      title?: string | null;
      why?: string | null;
      activities?: string[] | null;
      position?: number | null;
      isOptional?: number | null;
      mapX?: number | null;
      mapY?: number | null;
      clearMap?: boolean | null;
    };
  }
>`
  mutation UpdateTopic($id: ID!, $patch: TopicPatch!) {
    updateTopic(id: $id, patch: $patch) {
      ok
      topic {
        __typename
        id
        guid
        stageId
        title
        why
        activityList
        position
        status
        statusNote
        completedAt
        isOptional
        mapX
        mapY
        level
        point {
          x
          y
        }
        mapPinned
      }
      error {
        message
        code
      }
    }
  }
`;

export const DeleteTopicMutation = gql<
  { deleteTopic: { ok: boolean; error: PayloadErrorField } },
  { id: string }
>`
  mutation DeleteTopic($id: ID!) {
    deleteTopic(id: $id) {
      ok
      error {
        message
        code
      }
    }
  }
`;

export const SetTopicStatusMutation = gql<
  { setTopicStatus: { ok: boolean; topic: RoadmapTopic | null; error: PayloadErrorField } },
  { id: string; input: { status: NodeStatus; statusNote?: string | null } }
>`
  mutation SetTopicStatus($id: ID!, $input: StatusInput!) {
    setTopicStatus(id: $id, input: $input) {
      ok
      topic {
        __typename
        id
        status
        statusNote
        completedAt
        level
      }
      error {
        message
        code
      }
    }
  }
`;

export const CreateResourceMutation = gql<
  { createResource: { ok: boolean; resource: RoadmapResource | null; error: PayloadErrorField } },
  { topicId: string; input: { title: string; url?: string | null; kind?: ResourceKind | null; note?: string | null; position?: number | null } }
>`
  mutation CreateResource($topicId: ID!, $input: ResourceInput!) {
    createResource(topicId: $topicId, input: $input) {
      ok
      resource {
        __typename
        id
        guid
        topicId
        title
        url
        kind
        note
        position
        createdAt
        updatedAt
      }
      error {
        message
        code
      }
    }
  }
`;

export const UpdateResourceMutation = gql<
  { updateResource: { ok: boolean; resource: RoadmapResource | null; error: PayloadErrorField } },
  { id: string; patch: { title?: string | null; url?: string | null; kind?: ResourceKind | null; note?: string | null; position?: number | null; clearUrl?: boolean | null } }
>`
  mutation UpdateResource($id: ID!, $patch: ResourcePatch!) {
    updateResource(id: $id, patch: $patch) {
      ok
      resource {
        __typename
        id
        guid
        topicId
        title
        url
        kind
        note
        position
        createdAt
        updatedAt
      }
      error {
        message
        code
      }
    }
  }
`;

export const DeleteResourceMutation = gql<
  { deleteResource: { ok: boolean; error: PayloadErrorField } },
  { id: string }
>`
  mutation DeleteResource($id: ID!) {
    deleteResource(id: $id) {
      ok
      error {
        message
        code
      }
    }
  }
`;

export const CreateMilestoneMutation = gql<
  { createMilestone: { ok: boolean; milestone: RoadmapMilestone | null; error: PayloadErrorField } },
  { stageId: string; input: { text: string; position: number | null } }
>`
  mutation CreateMilestone($stageId: ID!, $input: MilestoneInput!) {
    createMilestone(stageId: $stageId, input: $input) {
      ok
      milestone {
        __typename
        id
        guid
        stageId
        text
        position
        createdAt
        updatedAt
      }
      error {
        message
        code
      }
    }
  }
`;

export const UpdateMilestoneMutation = gql<
  { updateMilestone: { ok: boolean; milestone: RoadmapMilestone | null; error: PayloadErrorField } },
  { id: string; patch: { text?: string | null; position?: number | null } }
>`
  mutation UpdateMilestone($id: ID!, $patch: MilestonePatch!) {
    updateMilestone(id: $id, patch: $patch) {
      ok
      milestone {
        __typename
        id
        guid
        stageId
        text
        position
        createdAt
        updatedAt
      }
      error {
        message
        code
      }
    }
  }
`;

export const DeleteMilestoneMutation = gql<
  { deleteMilestone: { ok: boolean; error: PayloadErrorField } },
  { id: string }
>`
  mutation DeleteMilestone($id: ID!) {
    deleteMilestone(id: $id) {
      ok
      error {
        message
        code
      }
    }
  }
`;

// ─────────────────────────────────────────────────────────────────────────────
// bookmark — kho link độc lập
// ─────────────────────────────────────────────────────────────────────────────
//
// ⚠️ `BookmarkStatus` CỐ Ý KHÁC `Status` của node roadmap. Truyền
// `NOT_STARTED`/`IN_PROGRESS` cho bookmark là 400, không phải rơi về mặc định
// — xem `phases/task-memory/stack-v2-m6a.md` §5.2.

export type BookmarkStatus = 'TO_READ' | 'READING' | 'DONE' | 'ARCHIVED';

export const BOOKMARK_STATUSES: readonly BookmarkStatus[] = [
  'TO_READ',
  'READING',
  'DONE',
  'ARCHIVED',
];

export interface Bookmark {
  id: string;
  guid: string;
  title: string;
  url: string | null;
  note: string;
  /** CSV thô như lưu DB — chỉ để đối chiếu, UI dùng `tagList`. */
  tags: string;
  tagList: string[];
  status: BookmarkStatus;
  createdAt: string;
  updatedAt: string;
}

const BOOKMARK_FIELDS = gql`
  fragment BookmarkFields on Bookmark {
    __typename
    id
    guid
    title
    url
    note
    tags
    tagList
    status
    createdAt
    updatedAt
  }
`;

/** Màn `/bookmarks`. `status`/`tag` null = không lọc; lọc tag khớp theo PHẦN TỬ CSV. */
export const Bookmarks = gql<
  { bookmarks: Bookmark[] },
  { status: BookmarkStatus | null; tag: string | null }
>`
  ${BOOKMARK_FIELDS}
  query Bookmarks($status: BookmarkStatus, $tag: String) {
    bookmarks(status: $status, tag: $tag) {
      ...BookmarkFields
    }
  }
`;

export const CreateBookmarkMutation = gql<
  { createBookmark: { ok: boolean; bookmark: Bookmark | null; error: PayloadErrorField } },
  {
    input: {
      title: string;
      url?: string | null;
      note?: string | null;
      tags?: string[] | null;
      status?: BookmarkStatus | null;
    };
  }
>`
  ${BOOKMARK_FIELDS}
  mutation CreateBookmark($input: BookmarkInput!) {
    createBookmark(input: $input) {
      ok
      bookmark {
        ...BookmarkFields
      }
      error {
        message
        code
      }
    }
  }
`;

/** `patch` rỗng là 400 "không có gì để cập nhật" — luôn gửi ít nhất 1 field. */
export const UpdateBookmarkMutation = gql<
  { updateBookmark: { ok: boolean; bookmark: Bookmark | null; error: PayloadErrorField } },
  {
    id: string;
    patch: {
      title?: string;
      url?: string;
      note?: string;
      tags?: string[];
      clearUrl?: boolean;
    };
  }
>`
  ${BOOKMARK_FIELDS}
  mutation UpdateBookmark($id: ID!, $patch: BookmarkPatch!) {
    updateBookmark(id: $id, patch: $patch) {
      ok
      bookmark {
        ...BookmarkFields
      }
      error {
        message
        code
      }
    }
  }
`;

export const DeleteBookmarkMutation = gql<
  { deleteBookmark: { ok: boolean; error: PayloadErrorField } },
  { id: string }
>`
  mutation DeleteBookmark($id: ID!) {
    deleteBookmark(id: $id) {
      ok
      error {
        message
        code
      }
    }
  }
`;

/** Đổi trạng thái KHÔNG đi qua `updateBookmark` — schema cố ý tách riêng. */
export const SetBookmarkStatusMutation = gql<
  { setBookmarkStatus: { ok: boolean; bookmark: Bookmark | null; error: PayloadErrorField } },
  { id: string; status: BookmarkStatus }
>`
  ${BOOKMARK_FIELDS}
  mutation SetBookmarkStatus($id: ID!, $status: BookmarkStatus!) {
    setBookmarkStatus(id: $id, status: $status) {
      ok
      bookmark {
        ...BookmarkFields
      }
      error {
        message
        code
      }
    }
  }
`;

// ─────────────────────────────────────────────────────────────────────────────
// insight — dashboard
// ─────────────────────────────────────────────────────────────────────────────

export interface Stats {
  range: string;
  days: number;
  doneWindow: number;
  totalAll: number;
  dueNow: number;
  accuracy: number;
  streak: number;
  timezone: string;
}

/** Dashboard (tiến độ tuần/tháng). */
export const StatsQuery = gql<{ stats: { ok: boolean; stats: Stats | null; error: PayloadErrorField } }, { range: string }>`
  query Stats($range: String) {
    stats(range: $range) {
      ok
      stats {
        range
        days
        doneWindow
        totalAll
        dueNow
        accuracy
        streak
        timezone
      }
      error {
        message
        code
      }
    }
  }
`;

// ─────────────────────────────────────────────────────────────────────────────
// sync — trạng thái + log xung đột (Cài đặt)
// ─────────────────────────────────────────────────────────────────────────────

export interface SyncConflict {
  guid: string;
  table: string;
  field: string;
  local: string;
  incoming: string;
  winner: string;
  detail: string;
  resolvedAt: string;
}

export interface Merged {
  decks: number;
  cards: number;
  reviews: number;
  notes: number;
  roadmapPaths: number;
  roadmapStages: number;
  roadmapMilestones: number;
  roadmapTopics: number;
  roadmapResources: number;
  roadmapBookmarks: number;
}

export interface SyncStatus {
  enabled: boolean;
  strategy: string;
  lastSyncAt: string;
  conflictCount: number;
}

export interface MergeResult {
  ok: boolean;
  merged: Merged;
  conflicts: SyncConflict[];
  warnings: string[];
  lastSyncAt: string;
  error: PayloadErrorField;
}

export const SyncStatusQuery = gql<{
  syncStatus: { ok: boolean; status: SyncStatus | null; error: PayloadErrorField };
}>`
  query SyncStatus {
    syncStatus {
      ok
      status {
        enabled
        strategy
        lastSyncAt
        conflictCount
      }
      error {
        message
        code
      }
    }
  }
`;

export const SyncConflicts = gql<
  { syncConflicts: { ok: boolean; conflicts: SyncConflict[]; error: PayloadErrorField } },
  { limit: number }
>`
  query SyncConflicts($limit: Int) {
    syncConflicts(limit: $limit) {
      ok
      conflicts {
        guid
        table
        field
        local
        incoming
        winner
        detail
        resolvedAt
      }
      error {
        message
        code
      }
    }
  }
`;

/**
 * App v2 KHÔNG nhận file peer qua HTTP: mutation `sync` không có tham số
 * (xem `root.graphqls`) vì `syncinfra.NewSchemaLoader(nil)` chưa nối peer —
 * M4 §8 ghi rõ đây là "nối dây, chưa có peer". CaiDat vẫn gọi để hiện lỗi
 * tiếng Việt nguyên văn thay vì giấu nút.
 */
export const SyncMutation = gql<{ sync: MergeResult }>`
  mutation Sync {
    sync {
      ok
      merged {
        decks
        cards
        reviews
        notes
        roadmapPaths
        roadmapStages
        roadmapMilestones
        roadmapTopics
        roadmapResources
        roadmapBookmarks
      }
      conflicts {
        guid
        table
        field
        local
        incoming
        winner
        detail
        resolvedAt
      }
      warnings
      lastSyncAt
      error {
        message
        code
      }
    }
  }
`;
