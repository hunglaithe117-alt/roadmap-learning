// M6 — client `roadmap` phía cây 5 tầng + bản đồ game.
//
// Mọi hàm đọc 1 operation; mọi hàm ghi đều `await afterMutation()` BÊN TRONG
// (như `srs/api.ts` và `player/api.ts`) — không màn nào tự invalidate.
//
// ĐIỂM DUY NHẤT đáng chú ý: `fetchPathTree` SẮP LẠI `(position, id)` cho
// stages / topics / resources / milestones trước khi trả. Không phải thừa — M4
// từng có F8: `StageTreeByPathIDs` gom bằng `range` trên Go map nên
// `path.stages` trả thứ tự NGẪU NHIÊN mỗi request. Bản đồ vẽ node theo thứ tự
// mảng nên node nhảy chỗ giữa 2 lần tải, không có lỗi nào để phát hiện. Suy ra
// `LevelState` (DONE/CURRENT/LOCKED) server gán theo CÙNG thứ tự đó, nên thứ tự
// sai còn làm "màn hiện tại" rơi vào sai node. Xem `roadmap/order.ts`.
import { runMutation, runQuery } from '../graphql/client';
import { assertPayloadOk } from '../graphql/errors';
import {
  CreateMilestoneMutation,
  CreatePathMutation,
  CreateResourceMutation,
  CreateStageMutation,
  CreateTopicMutation,
  DeleteMilestoneMutation,
  DeletePathMutation,
  DeleteResourceMutation,
  DeleteStageMutation,
  DeleteTopicMutation,
  Paths,
  PathTree,
  SetStageStatusMutation,
  SetTopicStatusMutation,
  UpdateMilestoneMutation,
  UpdatePathMutation,
  UpdateResourceMutation,
  UpdateStageMutation,
  UpdateTopicMutation,
  type MapDirection,
  type MapTerrain,
  type NodeStatus,
  type PathRow,
  type ResourceKind,
  type RoadmapMilestone,
  type RoadmapPath,
  type RoadmapResource,
  type RoadmapStage,
  type RoadmapTopic,
} from '../graphql/operations';
import { afterMutation } from '../lib/afterMutation';
import { sortByPosition } from './order';

export type {
  MapDirection,
  MapTerrain,
  NodeStatus,
  PathRow,
  ResourceKind,
  RoadmapMilestone,
  RoadmapPath,
  RoadmapResource,
  RoadmapStage,
  RoadmapTopic,
};

// ── đọc ─────────────────────────────────────────────────────────────────────

/** Danh sách path + tiến độ rút gọn. Không sắp lại: server đã `ORDER BY` slug. */
export async function fetchPathRows(): Promise<PathRow[]> {
  return (await runQuery(Paths)).paths;
}

export async function fetchPath(slug: string): Promise<RoadmapPath | null> {
  const { path } = await runQuery(PathTree, { slug });
  if (!path) return null;
  return sortPathTree(path);
}

/** Sắp `(position, id)` cho cả 4 tầng có `position`. Không mutate input. */
export function sortPathTree(path: RoadmapPath): RoadmapPath {
  return {
    ...path,
    stages: sortByPosition(path.stages).map((stage) => ({
      ...stage,
      topics: sortByPosition(stage.topics).map((topic) => ({
        ...topic,
        resources: sortByPosition(topic.resources),
      })),
      milestones: sortByPosition(stage.milestones),
    })),
  };
}

// ── path ────────────────────────────────────────────────────────────────────

export interface NewPath {
  slug: string;
  title: string;
  overview?: string | null;
  language?: string | null;
}

export async function createPath(input: NewPath): Promise<void> {
  const data = await runMutation(CreatePathMutation, {
    input: {
      slug: input.slug,
      title: input.title,
      overview: input.overview ?? null,
      language: input.language ?? null,
    },
  });
  assertPayloadOk(data.createPath);
  await afterMutation();
}

export interface PathPatch {
  title?: string;
  overview?: string;
  language?: string;
}

export async function updatePath(slug: string, patch: PathPatch): Promise<void> {
  const data = await runMutation(UpdatePathMutation, { slug, patch });
  assertPayloadOk(data.updatePath);
  await afterMutation();
}

export async function deletePath(slug: string): Promise<void> {
  const data = await runMutation(DeletePathMutation, { slug });
  assertPayloadOk(data.deletePath);
  await afterMutation();
}

// ── stage ───────────────────────────────────────────────────────────────────

export interface NewStage {
  slug: string;
  title: string;
  goal?: string | null;
  position?: number | null;
  durationWeeks?: number | null;
  status?: NodeStatus | null;
  deckId?: string | null;
  terrain?: MapTerrain | null;
  direction?: MapDirection | null;
}

export async function createStage(pathSlug: string, input: NewStage): Promise<void> {
  const data = await runMutation(CreateStageMutation, { pathSlug, input });
  assertPayloadOk(data.createStage);
  await afterMutation();
}

export interface StagePatch {
  title?: string;
  goal?: string;
  position?: number;
  durationWeeks?: number;
  deckId?: string | null;
  terrain?: MapTerrain;
  direction?: MapDirection;
}

export async function updateStage(id: string, patch: StagePatch): Promise<void> {
  const data = await runMutation(UpdateStageMutation, { id, patch });
  assertPayloadOk(data.updateStage);
  await afterMutation();
}

export async function deleteStage(id: string): Promise<void> {
  const data = await runMutation(DeleteStageMutation, { id });
  assertPayloadOk(data.deleteStage);
  await afterMutation();
}

export async function setStageStatus(
  id: string,
  status: NodeStatus,
  statusNote: string | null = null,
): Promise<void> {
  const data = await runMutation(SetStageStatusMutation, { id, input: { status, statusNote } });
  assertPayloadOk(data.setStageStatus);
  await afterMutation();
}

// ── topic (màn của bản đồ) ─────────────────────────────────────────────────

export interface NewTopic {
  title: string;
  why?: string | null;
  activities?: string[] | null;
  position?: number | null;
  isOptional?: number | null;
  mapX?: number | null;
  mapY?: number | null;
}

export async function createTopic(stageId: string, input: NewTopic): Promise<void> {
  const data = await runMutation(CreateTopicMutation, { stageId, input });
  assertPayloadOk(data.createTopic);
  await afterMutation();
}

export interface TopicPatch {
  title?: string;
  why?: string;
  activities?: string[];
  position?: number;
  isOptional?: number;
  mapX?: number;
  mapY?: number;
  clearMap?: boolean;
}

export async function updateTopic(id: string, patch: TopicPatch): Promise<void> {
  const data = await runMutation(UpdateTopicMutation, { id, patch });
  assertPayloadOk(data.updateTopic);
  await afterMutation();
}

export async function deleteTopic(id: string): Promise<void> {
  const data = await runMutation(DeleteTopicMutation, { id });
  assertPayloadOk(data.deleteTopic);
  await afterMutation();
}

export async function setTopicStatus(
  id: string,
  status: NodeStatus,
  statusNote: string | null = null,
): Promise<void> {
  const data = await runMutation(SetTopicStatusMutation, { id, input: { status, statusNote } });
  assertPayloadOk(data.setTopicStatus);
  await afterMutation();
}

// ── resource (tài liệu của 1 màn) ───────────────────────────────────────────

export interface NewResource {
  title: string;
  url?: string | null;
  kind?: ResourceKind | null;
  note?: string | null;
  position?: number | null;
}

export async function createResource(topicId: string, input: NewResource): Promise<void> {
  const data = await runMutation(CreateResourceMutation, { topicId, input });
  assertPayloadOk(data.createResource);
  await afterMutation();
}

export interface ResourcePatch {
  title?: string;
  url?: string;
  kind?: ResourceKind;
  note?: string;
  position?: number;
  clearUrl?: boolean;
}

export async function updateResource(id: string, patch: ResourcePatch): Promise<void> {
  const data = await runMutation(UpdateResourceMutation, { id, patch });
  assertPayloadOk(data.updateResource);
  await afterMutation();
}

export async function deleteResource(id: string): Promise<void> {
  const data = await runMutation(DeleteResourceMutation, { id });
  assertPayloadOk(data.deleteResource);
  await afterMutation();
}

// ── milestone (mốc chặng trên bản đồ) ───────────────────────────────────────

export async function createMilestone(stageId: string, text: string, position?: number | null): Promise<void> {
  const data = await runMutation(CreateMilestoneMutation, {
    stageId,
    input: { text, position: position ?? null },
  });
  assertPayloadOk(data.createMilestone);
  await afterMutation();
}

export interface MilestonePatch {
  text?: string;
  position?: number;
}

export async function updateMilestone(id: string, patch: MilestonePatch): Promise<void> {
  const data = await runMutation(UpdateMilestoneMutation, { id, patch });
  assertPayloadOk(data.updateMilestone);
  await afterMutation();
}

export async function deleteMilestone(id: string): Promise<void> {
  const data = await runMutation(DeleteMilestoneMutation, { id });
  assertPayloadOk(data.deleteMilestone);
  await afterMutation();
}
