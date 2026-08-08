// G4-F：3D 知识图谱 composable 行为测试（V12.7）。
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { ref, nextTick } from 'vue';
import type { CollectionGraph, EntityMergeSuggestion, KnowledgeCollection } from '../types';

const mockStore = {
  loadVaultTree: vi.fn(),
};
const mockApi = {
  listCollectionGraph: vi.fn(),
  listEntityMergeSuggestions: vi.fn(),
  mergeKnowledgeEntities: vi.fn(),
};

vi.mock('../../../stores/knowledge', () => ({
  useKnowledgeStore: () => mockStore,
}));

vi.mock('../api', () => ({
  listCollectionGraph: (...args: unknown[]) => mockApi.listCollectionGraph(...args),
  listEntityMergeSuggestions: (...args: unknown[]) => mockApi.listEntityMergeSuggestions(...args),
  mergeKnowledgeEntities: (...args: unknown[]) => mockApi.mergeKnowledgeEntities(...args),
}));

import { useKnowledgeGraph } from '../useKnowledgeGraph';

function makeCollection(id: string): KnowledgeCollection {
  return {
    id,
    name: `Vault ${id}`,
    description: '',
    embedding_model: 'm',
    dim: 1536,
    status: 'active',
    document_count: 0,
    chunk_count: 0,
    workspace: '',
    created_at: '2026-07-30T00:00:00Z',
    updated_at: '2026-07-30T00:00:00Z',
    root_path: `/vault-${id}`,
    sync_state: 'active',
    last_sync_at: '',
  };
}

const GRAPH: CollectionGraph = {
  nodes: [
    { doc_id: 'd1', name: 'doc1', rel_path: 'notes/doc1.md', doc_type: 'note', degree: 2 },
    { doc_id: 'd2', name: 'doc2', rel_path: 'doc2.md', doc_type: 'note', degree: 1 },
  ],
  edges: [{ source: 'd1', target: 'd2', type: 'explicit' }],
};

const SUGGESTIONS: EntityMergeSuggestion[] = [
  { keeper_id: 1, keeper_name: 'AI', mergee_id: 2, mergee_name: 'ai', source: 'norm', similarity: 1, tier: 'auto' },
];

function setup(cols: string[] = ['c1']) {
  const collections = ref<KnowledgeCollection[]>(cols.map(makeCollection));
  const graph = useKnowledgeGraph({
    collections,
    friendlyError: (e) => (e instanceof Error ? e.message : String(e)),
  });
  return { graph, collections };
}

/** watch 触发的 loadGraph 是 async，等两个 tick 让 Promise 链落地。 */
async function settle() {
  await nextTick();
  await vi.waitFor(() => expect(mockApi.listCollectionGraph).toHaveBeenCalled());
  await nextTick();
}

describe('useKnowledgeGraph', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApi.listCollectionGraph.mockResolvedValue(GRAPH);
    mockApi.listEntityMergeSuggestions.mockResolvedValue(SUGGESTIONS);
    mockApi.mergeKnowledgeEntities.mockResolvedValue({ rewritten_mentions: 3, rewritten_links: 2, merged_entities: 1 });
    mockStore.loadVaultTree.mockResolvedValue([]);
  });

  it('collections 就绪后默认选中首库并自动加载（全类型 + 全库）', async () => {
    const { graph } = setup();
    await settle();
    expect(graph.collectionId.value).toBe('c1');
    expect(mockApi.listCollectionGraph).toHaveBeenCalledWith('c1', [], '');
    expect(graph.nodes.value).toHaveLength(2);
    expect(graph.generation.value).toBe(1);
  });

  it('无库时不发起加载', async () => {
    const { graph } = setup([]);
    await nextTick();
    expect(graph.collectionId.value).toBe('');
    expect(mockApi.listCollectionGraph).not.toHaveBeenCalled();
  });

  it('边类型 chips：全选/全不选传空数组，部分选传选中集', async () => {
    const { graph } = setup();
    await settle();

    // 部分选：去掉 explicit → 传 ['entity', 'semantic']。
    graph.toggleLinkType('explicit');
    await vi.waitFor(() =>
      expect(mockApi.listCollectionGraph).toHaveBeenLastCalledWith('c1', ['entity', 'semantic'], ''),
    );

    // 全不选 = 全部（空数组）。
    graph.toggleLinkType('entity');
    graph.toggleLinkType('semantic');
    await vi.waitFor(() => expect(mockApi.listCollectionGraph).toHaveBeenLastCalledWith('c1', [], ''));
  });

  it('目录前缀过滤：setPathPrefix 带前缀重新加载；切库重置范围与选中', async () => {
    const { graph, collections } = setup(['c1', 'c2']);
    await settle();

    graph.setPathPrefix('notes/');
    await vi.waitFor(() => expect(mockApi.listCollectionGraph).toHaveBeenLastCalledWith('c1', [], 'notes/'));
    graph.selectNode('d1');

    // 切库：范围与选中复位，按新库重新加载。
    void collections; // collections 已就绪，直接切库。
    graph.selectCollection('c2');
    await vi.waitFor(() => expect(mockApi.listCollectionGraph).toHaveBeenLastCalledWith('c2', [], ''));
    expect(graph.pathPrefix.value).toBe('');
    expect(graph.selectedNodeId.value).toBe('');
  });

  it('focusNode：选中 + 聚焦信号递增；selectNode 仅选中', async () => {
    const { graph } = setup();
    await settle();

    graph.selectNode('d1');
    expect(graph.selectedNodeId.value).toBe('d1');
    expect(graph.focusSignal.value).toBe(0);

    graph.focusNode('d2');
    expect(graph.selectedNodeId.value).toBe('d2');
    expect(graph.focusSignal.value).toBe(1);
    expect(graph.selectedNode.value?.name).toBe('doc2');
  });

  it('加载失败：error 置位且节点清空', async () => {
    mockApi.listCollectionGraph.mockRejectedValue(new Error('boom'));
    const { graph } = setup();
    await vi.waitFor(() => expect(graph.error.value).toBe('boom'));
    expect(graph.nodes.value).toHaveLength(0);
    expect(graph.loading.value).toBe(false);
  });

  it('节点列表：连接度降序 + 搜索过滤', async () => {
    const { graph } = setup();
    await settle();
    expect(graph.nodeList.value.map((n) => n.doc_id)).toEqual(['d1', 'd2']);

    graph.nodeQuery.value = 'doc2';
    expect(graph.nodeList.value.map((n) => n.doc_id)).toEqual(['d2']);
  });

  it('范围迷你树懒加载：仅目录进树', async () => {
    mockStore.loadVaultTree.mockResolvedValue([
      { name: 'guides', path: 'guides/', kind: 'dir' },
      { name: 'doc.md', path: 'doc.md', kind: 'file', doc_id: 'd9' },
    ]);
    const { graph } = setup();
    await settle();

    const done = vi.fn();
    const fail = vi.fn();
    await graph.onScopeLazyLoad({ key: 'v|c1', done, fail });
    expect(fail).not.toHaveBeenCalled();
    expect(done).toHaveBeenCalledWith([
      expect.objectContaining({ kind: 'dir', prefix: 'guides/', vaultId: 'c1' }),
    ]);
  });

  // ---------- G5-G G-1：实体治理（合并建议 + 一键合并） ----------

  it('合并建议随库加载；建议拉取失败降级空列表不污染图谱 error', async () => {
    const { graph } = setup();
    await settle();
    await vi.waitFor(() => expect(mockApi.listEntityMergeSuggestions).toHaveBeenCalledWith('c1'));
    expect(graph.mergeSuggestions.value).toHaveLength(1);
    expect(graph.mergeSuggestions.value[0].keeper_name).toBe('AI');

    // 拉取失败：降级空列表，主 error 不置位（辅助数据不阻断图谱）。
    mockApi.listEntityMergeSuggestions.mockRejectedValue(new Error('suggest boom'));
    graph.selectCollection('c1'); // 同库不触发；改库才重拉
    await graph.loadMergeSuggestions();
    expect(graph.mergeSuggestions.value).toEqual([]);
    expect(graph.error.value).toBe('');
  });

  it('mergeEntities：调合并 RPC → 重拉图谱与建议 → 内联反馈置位', async () => {
    const { graph } = setup();
    await settle();
    await vi.waitFor(() => expect(graph.mergeSuggestions.value).toHaveLength(1));
    const graphCalls = mockApi.listCollectionGraph.mock.calls.length;

    await graph.mergeEntities(1, 2);

    expect(mockApi.mergeKnowledgeEntities).toHaveBeenCalledWith({ collectionId: 'c1', keeperId: 1, mergeeIds: [2] });
    expect(mockApi.listCollectionGraph.mock.calls.length).toBe(graphCalls + 1);
    expect(mockApi.listEntityMergeSuggestions.mock.calls.length).toBeGreaterThanOrEqual(2);
    expect(graph.lastMergeResult.value).toEqual({ rewritten_mentions: 3, rewritten_links: 2, merged_entities: 1 });
    expect(graph.merging.value).toBe(false);
  });

  it('mergeEntities 失败：error 置位且 merging 复位，无反馈残留', async () => {
    mockApi.mergeKnowledgeEntities.mockRejectedValue(new Error('merge boom'));
    const { graph } = setup();
    await settle();

    await graph.mergeEntities(1, 2);

    expect(graph.error.value).toBe('merge boom');
    expect(graph.merging.value).toBe(false);
    expect(graph.lastMergeResult.value).toBeNull();
  });

  it('切库清空合并反馈', async () => {
    const { graph } = setup(['c1', 'c2']);
    await settle();
    await graph.mergeEntities(1, 2);
    expect(graph.lastMergeResult.value).not.toBeNull();

    graph.selectCollection('c2');
    expect(graph.lastMergeResult.value).toBeNull();
  });
});
