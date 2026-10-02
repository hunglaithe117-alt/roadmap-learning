// Tầng client `roadmap`. Trọng tâm là SẮP LẠI CÂY KHI NHẬN — đây là lưới an
// toàn chống tái diễn F8 của M4 (`StageTreeByPathIDs` gom bằng `range` trên Go
// map ⇒ thứ tự node ngẫu nhiên mỗi request, bản đồ nhảy chỗ, không lỗi nào).
import { afterEach, describe, expect, it, vi } from 'vitest';
import { resetStaleMarks, resetUrqlClient } from '../graphql/client';
import { queryClient } from '../rest/queryClient';
import { operationNameOf } from '../test/graphqlMock';
import { fetchPath, fetchPathRows, sortPathTree } from './api';
import type { RoadmapPath } from '../graphql/operations';

afterEach(() => {
  vi.unstubAllGlobals();
  resetUrqlClient();
  resetStaleMarks();
  queryClient.clear();
});

function topic(id: string, position: number, title = id) {
  return {
    id,
    stageId: '10',
    title,
    why: '',
    activityList: [],
    position,
    status: 'NOT_STARTED' as const,
    statusNote: '',
    completedAt: null,
    isOptional: false,
    mapX: null,
    mapY: null,
    level: 'LOCKED' as const,
    point: { x: 500, y: 1000 - position * 100 },
    mapPinned: false,
    resources: [],
  };
}

function stage(id: string, position: number) {
  return {
    id,
    pathId: '1',
    slug: id,
    title: id,
    goal: '',
    position,
    durationWeeks: 0,
    status: 'NOT_STARTED' as const,
    statusNote: '',
    completedAt: null,
    deckId: null,
    deck: null,
    terrain: 'MEADOW' as const,
    direction: 'UP' as const,
    topics: [],
    milestones: [],
  };
}

function path(stages: RoadmapPath['stages']): RoadmapPath {
  return {
    id: '1',
    guid: 'p',
    slug: 'trung',
    title: 'T',
    overview: '',
    language: 'zh',
    isBuiltin: true,
    createdAt: 'x',
    updatedAt: 'x',
    stages,
    progress: {
      stages: stages.length,
      topicsTotal: 0,
      topicsRequired: 0,
      topicsOptional: 0,
      topicsDone: 0,
      topicsInProgress: 0,
      topicsLocked: 0,
      percent: 0,
      lastCompletedAt: null,
      completedInRange: 0,
    },
  };
}

describe('test_sort_path_tree_orders_by_position_then_id', () => {
  it('test_stages_are_sorted_by_position', () => {
    const sorted = sortPathTree(path([stage('c', 2), stage('a', 0), stage('b', 1)]));
    expect(sorted.stages.map((s) => s.id)).toEqual(['a', 'b', 'c']);
  });

  it('test_equal_positions_break_tie_on_id_numerically', () => {
    // `position` có thể trùng (user nhập tay). Nếu tie-break so CHUỖI thì "10"
    // đứng trước "9" và node nhảy vị trí.
    const sorted = sortPathTree(path([stage('10', 0), stage('9', 0), stage('100', 0)]));
    expect(sorted.stages.map((s) => s.id)).toEqual(['9', '10', '100']);
  });

  it('test_topics_are_sorted_independently_of_stages', () => {
    const p = path([{ ...stage('a', 0), topics: [topic('t3', 3), topic('t1', 1), topic('t2', 2)] }]);
    expect(sortPathTree(p).stages[0].topics.map((t) => t.id)).toEqual(['t1', 't2', 't3']);
  });

  it('test_sorting_does_not_mutate_the_input', () => {
    const p = path([stage('c', 2), stage('a', 0)]);
    sortPathTree(p);
    expect(p.stages.map((s) => s.id)).toEqual(['c', 'a']);
  });
});

describe('test_fetch_path_sorts_before_returning', () => {
  it('test_shuffled_server_order_is_normalised_by_the_client', async () => {
    const shuffled = path([
      { ...stage('s2', 1), topics: [topic('t9', 1), topic('t8', 0)] },
      stage('s1', 0),
    ]);
    const fetchMock = vi.fn().mockImplementation(async () => new Response(JSON.stringify({ data: { path: shuffled } }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchPath('trung');

    // Đây là hành vi M4 đã vỡ: server trả `s2` trước `s1` thì node bản đồ vẽ
    // sai thứ tự mà KHÔNG có lỗi nào báo.
    expect(result?.stages.map((s) => s.id)).toEqual(['s1', 's2']);
    expect(result?.stages[1].topics.map((t) => t.id)).toEqual(['t8', 't9']);
  });

  it('test_fetch_path_returns_null_for_unknown_slug', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation(async () => new Response(JSON.stringify({ data: { path: null } }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      })),
    );
    expect(await fetchPath('khong-co')).toBeNull();
  });

  it('test_fetch_path_reads_the_full_tree_in_one_request', async () => {
    const fetchMock = vi.fn().mockImplementation(async () => new Response(JSON.stringify({ data: { path: path([]) } }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    }));
    vi.stubGlobal('fetch', fetchMock);

    await fetchPath('trung');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(operationNameOf(fetchMock)).toBe('PathTree');
  });
});

describe('test_fetch_path_rows', () => {
  it('test_reads_the_summary_list_query', async () => {
    const fetchMock = vi.fn().mockImplementation(async () => new Response(JSON.stringify({ data: { paths: [] } }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    }));
    vi.stubGlobal('fetch', fetchMock);

    expect(await fetchPathRows()).toEqual([]);
    expect(operationNameOf(fetchMock)).toBe('Paths');
  });
});
