// G4-F：3D 知识图谱 composable 行为测试（V12.7）。
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { ref, nextTick } from 'vue';
import type { CollectionGraph, KnowledgeCollection } from '../types';

const mockStore = {
  loadVaultTree: vi.fn(),
};
const mockApi = {
  listCollectionGraph: vi.fn(),
};

vi.mock('../../../stores/knowledge', () => ({
  useKnowledgeStore: () => mockStore,
}));

vi.mock('../api', () => ({
  listCollectionGraph: (...args: unknown[]) => mockApi.listCollectionGraph(...args),
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
});
