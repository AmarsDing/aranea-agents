// B1-T5：confirmReembed 确认对话框——先弹确认（列出文档数 + 说明），确认后才调 store。
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

const mockStore = {
  embedderConfig: null,
  collections: [],
  documentsByCollection: {},
  documentsTruncatedByCollection: {},
  loading: false,
  loadCollections: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  loadDocuments: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  reembedDocuments: vi.fn().mockResolvedValue({ accepted_count: 1, skipped_count: 0 }),
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

describe('useKnowledgePage confirmReembed（B1-T5 确认对话框）', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    confirmCall.calls = [];
    confirmCall.current = null;
    mockStore.reembedDocuments.mockResolvedValue({ accepted_count: 1, skipped_count: 0 });
  });

  it('先弹确认对话框，未确认不调 store', () => {
    const page = useKnowledgePage();
    page.selectedId.value = 'col-1';
    void page.confirmReembed(['d1']);
    expect(confirmCall.calls).toHaveLength(1);
    const arg = confirmCall.calls[0] as { title: string; message: string; tone: string };
    expect(arg.title).toBe('knowledgePage.reembedConfirmTitle');
    expect(arg.message).toBe('knowledgePage.reembedConfirmBody');
    expect(arg.tone).toBe('danger');
    expect(mockStore.reembedDocuments).not.toHaveBeenCalled();
  });

  it('确认后调 store 并 notify 受理计数', async () => {
    const page = useKnowledgePage();
    page.selectedId.value = 'col-1';
    void page.confirmReembed(['d1', 'd2']);
    expect(confirmCall.current).toBeTruthy();
    confirmCall.current!();
    await vi.waitFor(() => expect(mockStore.reembedDocuments).toHaveBeenCalledWith('col-1', ['d1', 'd2']));
    await vi.waitFor(() => expect(notifySpy).toHaveBeenCalledWith(expect.objectContaining({ type: 'positive' })));
  });

  it('空 docIds 或未选集合时不弹对话框', () => {
    const page = useKnowledgePage();
    void page.confirmReembed(['d1']); // selectedId 空
    page.selectedId.value = 'col-1';
    void page.confirmReembed([]);
    expect(confirmCall.calls).toHaveLength(0);
  });
});
