import { describe, it, expect, vi, beforeEach } from 'vitest';

const notifySpy = vi.fn();
vi.mock('quasar', () => ({
  useQuasar: () => ({ notify: notifySpy, dialog: vi.fn(() => ({ onOk: vi.fn() })) }),
}));
vi.mock('vue-i18n', async (importOriginal) => {
  // createI18n 需保留真实现：knowledgePageError → src/i18n 在模块加载期调用；
  // 只替换 useI18n 为恒等 t。
  const actual = await importOriginal<typeof import('vue-i18n')>();
  return { ...actual, useI18n: () => ({ t: (k: string) => k }) };
});

const mockStore = {
  addCollection: vi.fn().mockResolvedValue({ id: 'managed-kb' }),
  embedderConfig: null,
  collections: [],
  documentsByCollection: {},
  documentsTruncatedByCollection: {},
  loading: false,
  loadCollections: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  loadDocuments: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  ingest: vi.fn().mockResolvedValue({ id: 'doc-1', collection_id: 'default-kb' }),
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

describe('useKnowledgePage submitIngest US-14', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockStore.ingest.mockResolvedValue({ id: 'doc-1', collection_id: 'default-kb' });
  });

  it('未选知识库时不传 collection_id 并选中返回的默认库', async () => {
    const page = useKnowledgePage();
    page.ingestForm.value.text = 'hello knowledge';
    await page.submitIngest();
    expect(mockStore.ingest).toHaveBeenCalledTimes(1);
    const arg = mockStore.ingest.mock.calls[0][0] as { collection_id?: string; source: string };
    expect(arg.collection_id).toBeUndefined();
    expect(arg.source).toBe('paste');
    expect(page.selectedId.value).toBe('default-kb');
  });

  it('已选知识库时仍传入该库', async () => {
    const page = useKnowledgePage();
    page.selectedId.value = 'col-1';
    page.ingestForm.value.text = 'hello';
    await page.submitIngest();
    const arg = mockStore.ingest.mock.calls[0][0] as { collection_id?: string };
    expect(arg.collection_id).toBe('col-1');
  });
});

it('defaults explicit creation to a managed team vault without a server path', async () => {
  const page = useKnowledgePage();
  page.openCreateCollection();
  page.createForm.value.name = 'My sources';
  await page.submitCreateCollection();
  expect(mockStore.addCollection).toHaveBeenCalledWith({
    name: 'My sources',
    description: '',
    embedding_model: '',
    root_path: '',
    vault_backend: 'team',
  });
  expect(page.selectedId.value).toBe('managed-kb');
});
