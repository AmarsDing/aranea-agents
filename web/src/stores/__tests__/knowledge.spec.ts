// knowledge store：B1 reembedDocuments action（受理计数透传 + api 入参校验）。
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useKnowledgeStore } from '../knowledge';

vi.mock('../../features/knowledge/api', () => ({
  createCollection: vi.fn(),
  createVaultDir: vi.fn(),
  createVaultDocument: vi.fn(),
  deleteCollection: vi.fn(),
  deleteDocument: vi.fn(),
  getCollection: vi.fn(),
  ingestDocument: vi.fn(),
  listBlockBacklinks: vi.fn(),
  listCollections: vi.fn(),
  listDanglingLinks: vi.fn(),
  listDocuments: vi.fn(),
  listDocumentLinks: vi.fn(),
  listVaultTree: vi.fn(),
  moveDocument: vi.fn(),
  moveDocumentToDir: vi.fn(),
  promoteDocuments: vi.fn(),
  reembedDocuments: vi.fn(),
  searchKnowledge: vi.fn(),
  getEmbedderConfig: vi.fn(),
  updateEmbedderConfig: vi.fn(),
}));

import { reembedDocuments } from '../../features/knowledge/api';

describe('useKnowledgeStore reembedDocuments（B1）', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it('reembedDocuments action 调用 api 并返回受理计数', async () => {
    vi.mocked(reembedDocuments).mockResolvedValueOnce({ accepted_count: 1, skipped_count: 0 });
    const store = useKnowledgeStore();
    const r = await store.reembedDocuments('col-1', ['doc-1']);
    expect(reembedDocuments).toHaveBeenCalledWith('col-1', ['doc-1'], undefined, undefined);
    expect(r).toEqual({ accepted_count: 1, skipped_count: 0 });
  });
});
