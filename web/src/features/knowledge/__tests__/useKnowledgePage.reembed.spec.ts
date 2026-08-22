// B1-T5：confirmReembed 确认对话框——先弹确认（列出文档数 + 说明），确认后才调 store。
import { describe, it, expect, vi, beforeEach } from 'vitest';

const notifySpy = vi.fn();
let onOkCb: (() => void) | null = null;
const dialogSpy = vi.fn(() => ({
  onOk: (cb: () => void) => {
    onOkCb = cb;
  },
}));

vi.mock('quasar', () => ({
  useQuasar: () => ({ notify: notifySpy, dialog: dialogSpy }),
}));

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}));

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
    onOkCb = null;
    mockStore.reembedDocuments.mockResolvedValue({ accepted_count: 1, skipped_count: 0 });
  });

  it('先弹确认对话框，未确认不调 store', () => {
    const page = useKnowledgePage();
    page.selectedId.value = 'col-1';
    page.confirmReembed(['d1']);
    expect(dialogSpy).toHaveBeenCalledTimes(1);
    const arg = dialogSpy.mock.calls[0][0] as { title: string; message: string; cancel: boolean };
    expect(arg.title).toBe('knowledgePage.reembedConfirmTitle');
    expect(arg.message).toBe('knowledgePage.reembedConfirmBody');
    expect(arg.cancel).toBe(true);
    expect(mockStore.reembedDocuments).not.toHaveBeenCalled();
  });

  it('确认后调 store 并 notify 受理计数', async () => {
    const page = useKnowledgePage();
    page.selectedId.value = 'col-1';
    page.confirmReembed(['d1', 'd2']);
    expect(onOkCb).toBeTruthy();
    onOkCb!();
    await vi.waitFor(() => expect(mockStore.reembedDocuments).toHaveBeenCalledWith('col-1', ['d1', 'd2']));
    await vi.waitFor(() => expect(notifySpy).toHaveBeenCalledWith(expect.objectContaining({ type: 'positive' })));
  });

  it('空 docIds 或未选集合时不弹对话框', () => {
    const page = useKnowledgePage();
    page.confirmReembed(['d1']); // selectedId 空
    page.selectedId.value = 'col-1';
    page.confirmReembed([]);
    expect(dialogSpy).not.toHaveBeenCalled();
  });
});
