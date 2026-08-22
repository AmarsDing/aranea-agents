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
  enableCollectionSemantic: vi.fn(),
  searchKnowledge: vi.fn(),
  getEmbedderConfig: vi.fn(),
  getDocumentContent: vi.fn(),
  updateDocumentContent: vi.fn(),
  updateEmbedderConfig: vi.fn(),
}));

import { reembedDocuments, enableCollectionSemantic, listDocuments, ingestDocument } from '../../features/knowledge/api';

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

describe('useKnowledgeStore enableCollectionSemantic（B2）', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it('enableCollectionSemantic action 调用 api 并返回受理结果', async () => {
    vi.mocked(enableCollectionSemantic).mockResolvedValueOnce({
      enqueued_docs: 7,
      embedding_model: 'text-embedding-3-small',
      dim: 1536,
    });
    const store = useKnowledgeStore();
    const r = await store.enableCollectionSemantic('col-lex');
    expect(enableCollectionSemantic).toHaveBeenCalledWith('col-lex');
    expect(r).toEqual({ enqueued_docs: 7, embedding_model: 'text-embedding-3-small', dim: 1536 });
  });
});

describe('useKnowledgeStore documents + ingest', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it('loadDocuments marks truncated when total exceeds page', async () => {
    vi.mocked(listDocuments).mockResolvedValueOnce({
      items: [{ id: 'd1', collection_id: 'c1' } as never],
      total: 3,
    });
    const store = useKnowledgeStore();
    await store.loadDocuments('c1', { limit: 1 });
    expect(store.documentsTruncatedByCollection.c1).toBe(true);
  });

  it('loadDocuments append merges pages and clears truncated when complete', async () => {
    vi.mocked(listDocuments)
      .mockResolvedValueOnce({ items: [{ id: 'd1', collection_id: 'c1' } as never], total: 2 })
      .mockResolvedValueOnce({ items: [{ id: 'd2', collection_id: 'c1' } as never], total: 2 });
    const store = useKnowledgeStore();
    await store.loadDocuments('c1', { limit: 1 });
    await store.loadDocuments('c1', { limit: 1, offset: 1, append: true });
    expect(store.documentsByCollection.c1.map((d) => d.id)).toEqual(['d1', 'd2']);
    expect(store.documentsTruncatedByCollection.c1).toBe(false);
  });

  it('ingest upserts the same document id instead of duplicating', async () => {
    vi.mocked(ingestDocument).mockResolvedValue({
      id: 'd1',
      collection_id: 'c1',
      source: 'a.md',
      summary: 'updated',
    } as never);
    const store = useKnowledgeStore();
    store.documentsByCollection.c1 = [{ id: 'd1', collection_id: 'c1', source: 'a.md', summary: 'old' } as never];
    await store.ingest({ collection_id: 'c1', source: 'a.md', content_base64: 'eA==' });
    expect(store.documentsByCollection.c1).toHaveLength(1);
    expect(store.documentsByCollection.c1[0].summary).toBe('updated');
  });
});
