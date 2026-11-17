// B2-T2：onTreeNodeAction('enable-semantic')——先弹确认（展示 embedder model/dim），确认后调 store 并刷新集合。
import { describe, it, expect, vi, beforeEach } from 'vitest';

const notifySpy = vi.fn();
// 确认框走 useAppConfirm 单例：捕获入参 + 手动放行。
const confirmCall = vi.hoisted(() => ({
  calls: [] as Record<string, unknown>[],
  current: null as null | (() => void),
}));

vi.mock('quasar', () => ({
  useQuasar: () => ({ notify: notifySpy }),
}));
vi.mock('../../../composables/useAppConfirm', () => ({
  confirm: (opts: Record<string, unknown>) => {
    confirmCall.calls.push(opts);
    return new Promise<boolean>((resolve) => {
      confirmCall.current = () => resolve(true);
    });
  },
}));

vi.mock('vue-i18n', async (importOriginal) => {
  // createI18n 需保留真实现：knowledgePageError → src/i18n 在模块加载期调用；
  // 只替换 useI18n 为恒等 t。
  const actual = await importOriginal<typeof import('vue-i18n')>();
  return { ...actual, useI18n: () => ({ t: (k: string) => k }) };
});

const lexicalCol = { id: 'col-lex', name: '词法库', embedding_model: '' };
const mockStore = {
  embedderConfig: {
    provider: 'openai',
    base_url: '',
    model: 'text-embedding-3-small',
    dim: 1536,
    configured: true,
    has_api_key: true,
  },
  collections: [lexicalCol],
  documentsByCollection: {},
  documentsTruncatedByCollection: {},
  loading: false,
  enableCollectionSemantic: vi
    .fn()
    .mockResolvedValue({ enqueued_docs: 7, embedding_model: 'text-embedding-3-small', dim: 1536 }),
  loadCollections: vi.fn().mockResolvedValue({ items: [lexicalCol], total: 1 }),
  loadDocuments: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  // useVaultExplorer 依赖（setup 期不触发，仅兜底）
  loadVaultTree: vi.fn(),
  invalidateTree: vi.fn(),
  loadDocumentLinks: vi.fn(),
  loadBlockBacklinks: vi.fn().mockResolvedValue([]),
  loadDanglingLinks: vi.fn().mockResolvedValue([]),
  search: vi.fn(),
  moveDocToDir: vi.fn(),
  invalidateLinkCaches: vi.fn(),
  loadDocumentContent: vi.fn(),
  saveDocumentContent: vi.fn(),
};

vi.mock('../../../stores/knowledge', () => ({ useKnowledgeStore: () => mockStore }));
vi.mock('../useKnowledgeIngestWs', () => ({ useKnowledgeIngestWs: () => {} }));
vi.mock('../api', () => ({
  fetchDocumentAsset: vi.fn(),
  getDocumentContent: vi.fn(),
  updateDocumentContent: vi.fn(),
}));

import { useKnowledgePage } from '../useKnowledgePage';
import { vaultNodeKey } from '../vaultTreeUi';
import type { VaultQTreeNode } from '../useVaultExplorer';

function vaultNode(vaultId: string): VaultQTreeNode {
  return { label: vaultId, key: vaultNodeKey(vaultId), icon: 'inventory_2', kind: 'vault', vaultId, prefix: '' };
}

describe('useKnowledgePage enable-semantic（B2-T2 确认对话框）', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    confirmCall.calls = [];
    confirmCall.current = null;
    mockStore.collections = [lexicalCol];
    mockStore.enableCollectionSemantic.mockResolvedValue({
      enqueued_docs: 7,
      embedding_model: 'text-embedding-3-small',
      dim: 1536,
    });
  });

  it('先弹确认对话框（标题/正文带 embedder model/dim），未确认不调 store', () => {
    const page = useKnowledgePage();
    page.onTreeNodeAction('enable-semantic', vaultNode('col-lex'));
    expect(confirmCall.calls).toHaveLength(1);
    const arg = confirmCall.calls[0] as { title: string; message: string; tone: string };
    expect(arg.title).toBe('knowledgePage.enableSemanticTitle');
    expect(arg.message).toBe('knowledgePage.enableSemanticBody');
    expect(arg.tone).toBe('danger');
    expect(mockStore.enableCollectionSemantic).not.toHaveBeenCalled();
  });

  it('确认后调 store 并刷新集合 + 受理计数 notify', async () => {
    const page = useKnowledgePage();
    page.onTreeNodeAction('enable-semantic', vaultNode('col-lex'));
    expect(confirmCall.current).toBeTruthy();
    confirmCall.current!();
    await vi.waitFor(() => expect(mockStore.enableCollectionSemantic).toHaveBeenCalledWith('col-lex'));
    await vi.waitFor(() => expect(mockStore.loadCollections).toHaveBeenCalled());
    await vi.waitFor(() => expect(notifySpy).toHaveBeenCalledWith(expect.objectContaining({ type: 'positive' })));
  });

  it('语义库（embedding_model 非空）或未知集合不弹对话框', () => {
    const page = useKnowledgePage();
    page.onTreeNodeAction('enable-semantic', vaultNode('col-unknown'));
    mockStore.collections = [{ id: 'col-sem', name: '语义库', embedding_model: 'm0' }];
    page.onTreeNodeAction('enable-semantic', vaultNode('col-sem'));
    expect(confirmCall.calls).toHaveLength(0);
  });
});
