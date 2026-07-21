import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  createCollection,
  deleteCollection,
  deleteDocument,
  getCollection,
  ingestDocument,
  listCollections,
  listDocuments,
  moveDocument,
  searchKnowledge,
  getEmbedderConfig,
  updateEmbedderConfig,
} from '../../features/knowledge/api';
import type {
  CreateCollectionInput,
  IngestDocumentInput,
  KnowledgeChunk,
  KnowledgeCollection,
  KnowledgeDocument,
  ListCollectionsResult,
  ListDocumentsResult,
  SearchKnowledgeQuery,
  EmbedderConfig,
  UpdateEmbedderConfigInput,
} from '../../features/knowledge/types';

export const useKnowledgeStore = defineStore('knowledge', () => {
  const collections = ref<KnowledgeCollection[]>([]);
  const collectionsTotal = ref(0);
  /** Documents keyed by collection_id */
  // TECH-DEBT: no TTL or invalidation — long-running sessions may serve stale data.
  // Consider adding a per-key expiry or a "lastFetchedAt" timestamp.
  const documentsByCollection = ref<Record<string, KnowledgeDocument[]>>({});
  const loading = ref(false);
  const embedderConfig = ref<EmbedderConfig | null>(null);

  async function loadCollections(params: { limit?: number; offset?: number } = {}): Promise<ListCollectionsResult> {
    loading.value = true;
    try {
      const result = await listCollections(params);
      collections.value = result.items;
      collectionsTotal.value = result.total;
      return result;
    } finally {
      loading.value = false;
    }
  }

  async function addCollection(input: CreateCollectionInput): Promise<KnowledgeCollection> {
    const created = await createCollection(input);
    collections.value.push(created);
    collectionsTotal.value += 1;
    return created;
  }

  async function removeCollection(id: string): Promise<void> {
    await deleteCollection(id);
    collections.value = collections.value.filter((c) => c.id !== id);
    delete documentsByCollection.value[id];
    collectionsTotal.value = Math.max(0, collectionsTotal.value - 1);
  }

  async function refreshCollection(id: string): Promise<KnowledgeCollection> {
    const updated = await getCollection(id);
    collections.value = collections.value.map((c) => (c.id === id ? updated : c));
    return updated;
  }

  async function loadDocuments(
    collectionId: string,
    params: { limit?: number; offset?: number } = {},
  ): Promise<ListDocumentsResult> {
    const result = await listDocuments(collectionId, params);
    documentsByCollection.value[collectionId] = result.items;
    return result;
  }

  async function ingest(input: IngestDocumentInput): Promise<KnowledgeDocument> {
    const doc = await ingestDocument(input);
    // US-14：collection_id 留空时由后端落入默认知识库，缓存键以返回文档的实际归属为准。
    const key = doc.collection_id || input.collection_id;
    const existing = documentsByCollection.value[key] ?? [];
    documentsByCollection.value[key] = [doc, ...existing];
    return doc;
  }

  async function removeDocument(id: string, collectionId: string): Promise<void> {
    await deleteDocument(id);
    if (documentsByCollection.value[collectionId]) {
      documentsByCollection.value[collectionId] = documentsByCollection.value[collectionId].filter((d) => d.id !== id);
    }
  }

  // US-14：跨库移动文档；本地缓存由页面侧 reload 刷新（源/目标库计数均变化）。
  async function moveDoc(id: string, targetCollectionId: string): Promise<KnowledgeDocument> {
    return moveDocument(id, targetCollectionId);
  }

  async function search(query: SearchKnowledgeQuery): Promise<KnowledgeChunk[]> {
    return searchKnowledge(query);
  }

  async function loadEmbedderConfig(): Promise<EmbedderConfig> {
    const cfg = await getEmbedderConfig();
    embedderConfig.value = cfg;
    return cfg;
  }

  async function saveEmbedderConfig(input: UpdateEmbedderConfigInput): Promise<EmbedderConfig> {
    const cfg = await updateEmbedderConfig(input);
    embedderConfig.value = cfg;
    return cfg;
  }

  return {
    collections,
    collectionsTotal,
    documentsByCollection,
    loading,
    embedderConfig,
    loadCollections,
    addCollection,
    removeCollection,
    refreshCollection,
    loadDocuments,
    ingest,
    removeDocument,
    moveDoc,
    search,
    loadEmbedderConfig,
    saveEmbedderConfig,
  };
});
