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
    const existing = documentsByCollection.value[input.collection_id] ?? [];
    documentsByCollection.value[input.collection_id] = [doc, ...existing];
    return doc;
  }

  async function removeDocument(id: string, collectionId: string): Promise<void> {
    await deleteDocument(id);
    if (documentsByCollection.value[collectionId]) {
      documentsByCollection.value[collectionId] = documentsByCollection.value[collectionId].filter((d) => d.id !== id);
    }
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
    search,
    loadEmbedderConfig,
    saveEmbedderConfig,
  };
});
