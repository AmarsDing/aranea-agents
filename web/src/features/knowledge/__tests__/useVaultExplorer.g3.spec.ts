// G3-F：拖拽移动（V12.5）+ 搜索范围选择器（V12.6）composable 行为测试。
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { ref } from 'vue';
import type { KnowledgeCollection, KnowledgeDocument, VaultTreeNode } from '../types';

const mockStore = {
  loadVaultTree: vi.fn(),
  invalidateTree: vi.fn(),
  loadDocumentLinks: vi.fn(),
  search: vi.fn(),
  moveDocToDir: vi.fn(),
  loadDocumentContent: vi.fn(),
  saveDocumentContent: vi.fn(),
};

vi.mock('../../../stores/knowledge', () => ({
  useKnowledgeStore: () => mockStore,
}));

vi.mock('../api', () => ({
  fetchDocumentAsset: vi.fn(),
  getDocumentContent: vi.fn(),
  updateDocumentContent: vi.fn(),
}));

import { useVaultExplorer } from '../useVaultExplorer';

function makeDoc(id: string, relPath: string): KnowledgeDocument {
  const name = relPath.split('/').pop() ?? relPath;
  return {
    id,
    collection_id: 'c1',
    source: name,
    mime_type: 'text/markdown',
    size_bytes: 10,
    chunk_count: 1,
    status: 'indexed',
    error_message: '',
    created_at: '2026-07-30T00:00:00Z',
    updated_at: '2026-07-30T00:00:00Z',
    rel_path: relPath,
    summary: '',
    tags: [],
    doc_type: 'note',
  };
}

function makeFileNode(doc: KnowledgeDocument): VaultTreeNode {
  return {
    name: doc.source,
    path: doc.rel_path,
    kind: 'file',
    doc_id: doc.id,
    summary: '',
    tags: [],
    doc_type: 'note',
    status: 'indexed',
    size_bytes: 10,
    updated_at: doc.updated_at,
    error_message: '',
  };
}

function conflictError() {
  // axios.isAxiosError 仅校验 isAxiosError === true。
  return {
    isAxiosError: true,
    response: { status: 409, data: { message: 'name clash' } },
    message: 'Request failed with status code 409',
  };
}

function setup(docs: KnowledgeDocument[]) {
  const selectedId = ref('c1');
  const collections = ref<KnowledgeCollection[]>([
    {
      id: 'c1',
      name: 'Vault 1',
      description: '',
      embedding_model: 'm',
      dim: 1536,
      status: 'active',
      document_count: docs.length,
      chunk_count: docs.length,
      workspace: '',
      created_at: '2026-07-30T00:00:00Z',
      updated_at: '2026-07-30T00:00:00Z',
      root_path: '/vault',
      sync_state: 'active',
      last_sync_at: '',
    },
  ]);
  const documents = ref(docs);
  const notifyError = vi.fn();
  const ex = useVaultExplorer({
    selectedId,
    collections,
    documents,
    friendlyError: (e) => (e instanceof Error ? e.message : String(e)),
    notifyError,
    semanticErrorFallback: () => 'semantic failed',
  });
  return { ex, notifyError, selectedId, collections };
}

describe('G3-F2 搜索范围选择器', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockStore.loadVaultTree.mockResolvedValue([]);
    mockStore.search.mockResolvedValue([]);
  });

  it('默认全库：即时区不额外过滤，语义区 path_prefix 为空', async () => {
    const docs = [makeDoc('d1', 'notes/doc1.md'), makeDoc('d2', 'policies/doc2.md')];
    const { ex } = setup(docs);

    ex.searchQuery.value = 'doc';
    expect(ex.searchScopePrefix.value).toBe('');
    expect(ex.instantResults.value.map((d) => d.id).sort()).toEqual(['d1', 'd2']);

    await ex.runSemanticSearch();
    expect(mockStore.search).toHaveBeenCalledWith(expect.objectContaining({ collection_id: 'c1', path_prefix: '' }));
  });

  it('选中目录范围：即时区前端 prefix 过滤', () => {
    const docs = [makeDoc('d1', 'notes/doc1.md'), makeDoc('d2', 'policies/doc2.md'), makeDoc('d3', 'doc3.md')];
    const { ex } = setup(docs);

    ex.setSearchScope('notes/');
    ex.searchQuery.value = 'doc';
    expect(ex.instantResults.value.map((d) => d.id)).toEqual(['d1']);
  });

  it('选中目录范围：语义区带 path_prefix', async () => {
    const { ex } = setup([makeDoc('d1', 'notes/doc1.md')]);

    ex.setSearchScope('notes/');
    ex.searchQuery.value = '季度报告';
    await ex.runSemanticSearch();
    expect(mockStore.search).toHaveBeenCalledWith(expect.objectContaining({ path_prefix: 'notes/' }));
  });

  it('清除范围恢复全库', async () => {
    const docs = [makeDoc('d1', 'notes/doc1.md'), makeDoc('d2', 'policies/doc2.md')];
    const { ex } = setup(docs);

    ex.setSearchScope('notes/');
    ex.clearSearchScope();
    ex.searchQuery.value = 'doc';
    expect(ex.searchScopePrefix.value).toBe('');
    expect(ex.instantResults.value.map((d) => d.id).sort()).toEqual(['d1', 'd2']);
  });

  it('范围迷你树：根节点为当前 vault，选中 key 与 prefix 互转', () => {
    const { ex } = setup([]);
    expect(ex.scopeRootNodes.value).toHaveLength(1);
    expect(ex.scopeRootNodes.value[0].kind).toBe('vault');
    expect(ex.scopeRootNodes.value[0].vaultId).toBe('c1');

    // 全库（vault 节点 key）→ prefix ''
    expect(ex.scopeSelectedKey.value).toBe('v|c1');
    ex.setSearchScope('guides/');
    expect(ex.scopeSelectedKey.value).toBe('d|c1|guides/');
  });

  it('vault 切换复位范围', async () => {
    const docs = [makeDoc('d1', 'notes/doc1.md')];
    const { ex, selectedId, collections } = setup(docs);
    ex.setSearchScope('notes/');

    // 切到另一个库（watch flush:'sync' 立即复位范围与选中态）。
    collections.value = [
      ...collections.value,
      {
        id: 'c2',
        name: 'Vault 2',
        description: '',
        embedding_model: 'm',
        dim: 1536,
        status: 'active',
        document_count: 0,
        chunk_count: 0,
        workspace: '',
        created_at: '2026-07-30T00:00:00Z',
        updated_at: '2026-07-30T00:00:00Z',
        root_path: '/vault2',
        sync_state: 'active',
        last_sync_at: '',
      },
    ];
    selectedId.value = 'c2';
    expect(ex.searchScopePrefix.value).toBe('');
  });
});

describe('G3-F1 拖拽移动', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockStore.loadVaultTree.mockResolvedValue([]);
    mockStore.moveDocToDir.mockImplementation((id: string) => Promise.resolve(makeDoc(id, 'archive/doc1.md')));
  });

  it('dragStartFile 记录拖拽源（fromPrefix 由文件路径推导）', () => {
    const { ex } = setup([makeDoc('d1', 'notes/doc1.md')]);
    ex.dragStartFile(makeFileNode(makeDoc('d1', 'notes/doc1.md')));
    expect(ex.dragFile.value).toEqual({ docId: 'd1', name: 'doc1.md', fromPrefix: 'notes/', vaultId: 'c1' });

    ex.dragEnd();
    expect(ex.dragFile.value).toBeNull();
  });

  it('合法 drop：调 moveDocToDir（默认策略）并强制重载中栏，返回 moved', async () => {
    const { ex } = setup([makeDoc('d1', 'notes/doc1.md')]);
    ex.dragStartFile(makeFileNode(makeDoc('d1', 'notes/doc1.md')));

    const res = await ex.dropOnTarget({ vaultId: 'c1', prefix: 'archive/' });
    expect(res).toBe('moved');
    expect(mockStore.moveDocToDir).toHaveBeenCalledWith('d1', 'archive/', '');
    // 中栏强制重载（invalidateTree 由 store action 内部负责，mock 不可见）。
    expect(mockStore.loadVaultTree).toHaveBeenCalledWith('c1', '', true);
    expect(ex.dragFile.value).toBeNull();
  });

  it('原地/跨库 drop = noop，不调后端', async () => {
    const { ex } = setup([makeDoc('d1', 'notes/doc1.md')]);

    ex.dragStartFile(makeFileNode(makeDoc('d1', 'notes/doc1.md')));
    expect(await ex.dropOnTarget({ vaultId: 'c1', prefix: 'notes/' })).toBe('noop');
    expect(mockStore.moveDocToDir).not.toHaveBeenCalled();

    ex.dragStartFile(makeFileNode(makeDoc('d1', 'notes/doc1.md')));
    expect(await ex.dropOnTarget({ vaultId: 'c2', prefix: 'x/' })).toBe('noop');
    expect(mockStore.moveDocToDir).not.toHaveBeenCalled();
  });

  it('无拖拽直接 drop = noop', async () => {
    const { ex } = setup([]);
    expect(await ex.dropOnTarget({ vaultId: 'c1', prefix: 'archive/' })).toBe('noop');
  });

  it('同名冲突（409）：返回 conflict 并暂存待决，resolveMoveConflict 带策略重试', async () => {
    mockStore.moveDocToDir.mockRejectedValueOnce(conflictError());
    const { ex } = setup([makeDoc('d1', 'notes/doc1.md')]);
    ex.dragStartFile(makeFileNode(makeDoc('d1', 'notes/doc1.md')));

    expect(await ex.dropOnTarget({ vaultId: 'c1', prefix: 'archive/' })).toBe('conflict');
    expect(mockStore.moveDocToDir).toHaveBeenCalledWith('d1', 'archive/', '');

    // 用户选「覆盖」→ 带 overwrite 重试。
    expect(await ex.resolveMoveConflict('overwrite')).toBe('moved');
    expect(mockStore.moveDocToDir).toHaveBeenLastCalledWith('d1', 'archive/', 'overwrite');
  });

  it('冲突重试「保留两份」→ rename 策略', async () => {
    mockStore.moveDocToDir.mockRejectedValueOnce(conflictError());
    const { ex } = setup([makeDoc('d1', 'notes/doc1.md')]);
    ex.dragStartFile(makeFileNode(makeDoc('d1', 'notes/doc1.md')));

    expect(await ex.dropOnTarget({ vaultId: 'c1', prefix: 'archive/' })).toBe('conflict');
    expect(await ex.resolveMoveConflict('rename')).toBe('moved');
    expect(mockStore.moveDocToDir).toHaveBeenLastCalledWith('d1', 'archive/', 'rename');
  });

  it('非冲突错误：返回 error 并通知，不暂存冲突', async () => {
    mockStore.moveDocToDir.mockRejectedValueOnce(new Error('disk full'));
    const { ex, notifyError } = setup([makeDoc('d1', 'notes/doc1.md')]);
    ex.dragStartFile(makeFileNode(makeDoc('d1', 'notes/doc1.md')));

    expect(await ex.dropOnTarget({ vaultId: 'c1', prefix: 'archive/' })).toBe('error');
    expect(notifyError).toHaveBeenCalled();
    expect(await ex.resolveMoveConflict('overwrite')).toBe('noop');
  });
});
