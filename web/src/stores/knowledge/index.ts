import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  createCollection,
  createVaultDir,
  createVaultDocument,
  deleteCollection,
  deleteDocument,
  getCollection,
  ingestDocument,
  listBlockBacklinks,
  listCollectionGraph,
  listCollections,
  listDanglingLinks,
  listDocuments,
  listDocumentLinks,
  listDocumentNeighborhood,
  listUnlinkedMentions as listUnlinkedMentionsApi,
  listVaultTree,
  moveDocument,
  moveDocumentToDir,
  promoteDocuments,
  reembedDocuments as reembedDocumentsApi,
  enableCollectionSemantic as enableCollectionSemanticApi,
  searchKnowledge,
  getEmbedderConfig,
  updateEmbedderConfig,
  updateDocumentVisibility,
} from '../../features/knowledge/api';
import type {
  BlockBacklink,
  CollectionGraph,
  CreateCollectionInput,
  DanglingLink,
  IngestDocumentInput,
  KnowledgeChunk,
  KnowledgeCollection,
  KnowledgeDocument,
  KnowledgeLink,
  ListCollectionsResult,
  ListDocumentsResult,
  PromoteResult,
  SearchKnowledgeQuery,
  UnlinkedMention,
  EmbedderConfig,
  EnableSemanticResult,
  UpdateEmbedderConfigInput,
  VaultTreeNode,
} from '../../features/knowledge/types';

export const useKnowledgeStore = defineStore('knowledge', () => {
  const collections = ref<KnowledgeCollection[]>([]);
  const collectionsTotal = ref(0);
  /** Documents keyed by collection_id */
  // TECH-DEBT: no TTL or invalidation — long-running sessions may serve stale data.
  // Consider adding a per-key expiry or a "lastFetchedAt" timestamp.
  const documentsByCollection = ref<Record<string, KnowledgeDocument[]>>({});
  /** 文档列表是否被 limit 截断（total > 已加载），键 collection_id；截断时前端即时搜索覆盖率不全。 */
  const documentsTruncatedByCollection = ref<Record<string, boolean>>({});
  const loading = ref(false);
  const embedderConfig = ref<EmbedderConfig | null>(null);
  /** P3 资源管理器：文件夹直接子节点缓存，键 `${collectionId}|${prefix}`。 */
  const treeChildren = ref<Record<string, VaultTreeNode[]>>({});
  /** P3 关联区：文档已解析关联缓存，键 doc_id。 */
  const linksByDoc = ref<Record<string, KnowledgeLink[]>>({});
  /** SP1-I：块级反链缓存，键 doc_id。 */
  const backlinksByDoc = ref<Record<string, BlockBacklink[]>>({});
  /** SP2-5：未链接提及缓存，键 doc_id（右栏反链面板与反链同源失效）。 */
  const unlinkedMentionsByDoc = ref<Record<string, UnlinkedMention[]>>({});
  /** SP1-I：悬空链缓存，键 collection_id。 */
  const danglingByCollection = ref<Record<string, DanglingLink[]>>({});
  /** 全库图谱共享缓存，键 collection_id（仅缓存无过滤全量图：linkTypes=[] + pathPrefix=''）。
   *  全屏 3D 图谱专用；右栏局部图走 loadDocumentNeighborhood（服务端 BFS 小载荷）。 */
  const graphsByCollection = ref<Record<string, CollectionGraph>>({});
  /** SP2-8 右栏局部图：文档 N 跳邻域缓存，键 `${docId}|${hops}`（小载荷，按跳数分档）。 */
  const neighborhoodsByKey = ref<Record<string, CollectionGraph>>({});

  /** 加载库图谱：无过滤（全量）结果进共享缓存；带过滤条件的查询透传不污染缓存。 */
  async function loadCollectionGraph(
    collectionId: string,
    linkTypes: string[] = [],
    pathPrefix = '',
    force = false,
  ): Promise<CollectionGraph> {
    const cacheable = linkTypes.length === 0 && pathPrefix === '';
    if (!force && cacheable && graphsByCollection.value[collectionId]) {
      return graphsByCollection.value[collectionId];
    }
    const g = await listCollectionGraph(collectionId, linkTypes, pathPrefix);
    if (cacheable) {
      graphsByCollection.value[collectionId] = g;
    }
    return g;
  }

  function treeKey(collectionId: string, prefix: string): string {
    return `${collectionId}|${prefix}`;
  }

  /** 文档结构变更后失效该 vault 的树缓存（ingest/delete/move/同步）。 */
  function invalidateTree(collectionId: string) {
    for (const key of Object.keys(treeChildren.value)) {
      if (key.startsWith(`${collectionId}|`)) {
        delete treeChildren.value[key];
      }
    }
  }

  async function loadVaultTree(collectionId: string, prefix = '', force = false): Promise<VaultTreeNode[]> {
    const key = treeKey(collectionId, prefix);
    if (!force && treeChildren.value[key]) {
      return treeChildren.value[key];
    }
    const items = await listVaultTree(collectionId, prefix);
    treeChildren.value[key] = items;
    return items;
  }

  async function loadDocumentLinks(docId: string, linkType = '', force = false): Promise<KnowledgeLink[]> {
    if (!force && !linkType && linksByDoc.value[docId]) {
      return linksByDoc.value[docId];
    }
    const items = await listDocumentLinks(docId, linkType);
    if (!linkType) {
      linksByDoc.value[docId] = items;
    }
    return items;
  }

  /** SP1-I：加载文档的块级反链（缓存按 doc_id）。 */
  async function loadBlockBacklinks(docId: string, force = false): Promise<BlockBacklink[]> {
    if (!force && backlinksByDoc.value[docId]) {
      return backlinksByDoc.value[docId];
    }
    const items = await listBlockBacklinks(docId);
    backlinksByDoc.value[docId] = items;
    return items;
  }

  /** SP2-5：加载文档的未链接提及（缓存按 doc_id；连边增量时随反链一并失效）。 */
  async function loadUnlinkedMentions(docId: string, force = false): Promise<UnlinkedMention[]> {
    if (!force && unlinkedMentionsByDoc.value[docId]) {
      return unlinkedMentionsByDoc.value[docId];
    }
    const items = await listUnlinkedMentionsApi(docId);
    unlinkedMentionsByDoc.value[docId] = items;
    return items;
  }

  /** SP1-I：加载集合悬空链（缓存按 collection_id）。 */
  async function loadDanglingLinks(collectionId: string, force = false): Promise<DanglingLink[]> {
    if (!force && danglingByCollection.value[collectionId]) {
      return danglingByCollection.value[collectionId];
    }
    const items = await listDanglingLinks(collectionId);
    danglingByCollection.value[collectionId] = items;
    return items;
  }

  /** SP2-8：加载文档 N 跳邻域子图（服务端 BFS）；按 `${docId}|${hops}` 缓存。 */
  async function loadDocumentNeighborhood(docId: string, hops = 2, force = false): Promise<CollectionGraph> {
    const key = `${docId}|${hops}`;
    if (!force && neighborhoodsByKey.value[key]) {
      return neighborhoodsByKey.value[key];
    }
    const g = await listDocumentNeighborhood(docId, hops);
    neighborhoodsByKey.value[key] = g;
    return g;
  }

  /** SP1-I：图谱增量（I-4）后失效相关缓存——反链（受影响文档）与悬空链（受影响集合）。
   *  同步失效对应集合的图谱缓存（ graphsByCollection ），避免全屏/右栏拉取到脏图。 */
  function invalidateLinkCaches(docIds: string[], collectionIds: string[]) {
    for (const id of docIds) {
      delete linksByDoc.value[id];
      delete backlinksByDoc.value[id];
      delete unlinkedMentionsByDoc.value[id];
    }
    for (const id of collectionIds) {
      delete danglingByCollection.value[id];
      delete graphsByCollection.value[id];
    }
    // 邻域子图随任一连边增量整体失效（小载荷重拉廉价，不追踪跨文档归属）。
    if (docIds.length || collectionIds.length) {
      neighborhoodsByKey.value = {};
    }
  }

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
    delete danglingByCollection.value[id];
    delete graphsByCollection.value[id];
    neighborhoodsByKey.value = {};
    invalidateTree(id);
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
    documentsTruncatedByCollection.value[collectionId] = result.total > result.items.length;
    return result;
  }

  async function ingest(input: IngestDocumentInput): Promise<KnowledgeDocument> {
    const doc = await ingestDocument(input);
    // US-14：collection_id 留空时由后端落入默认知识库，缓存键以返回文档的实际归属为准。
    const key = doc.collection_id || input.collection_id;
    const existing = documentsByCollection.value[key] ?? [];
    documentsByCollection.value[key] = [doc, ...existing];
    invalidateTree(key);
    return doc;
  }

  // G1-B2：树内新建目录（幂等）；失效该 vault 树缓存供重载。
  async function addVaultDir(collectionId: string, dirPath: string): Promise<void> {
    await createVaultDir(collectionId, dirPath);
    invalidateTree(collectionId);
  }

  // G1-B2：树内新建模板 .md（立即索引）；入库缓存 + 失效树。
  async function addVaultDocument(collectionId: string, relPath: string): Promise<KnowledgeDocument> {
    const doc = await createVaultDocument(collectionId, relPath);
    const existing = documentsByCollection.value[collectionId] ?? [];
    documentsByCollection.value[collectionId] = [doc, ...existing];
    invalidateTree(collectionId);
    return doc;
  }

  async function removeDocument(id: string, collectionId: string): Promise<void> {
    await deleteDocument(id);
    if (documentsByCollection.value[collectionId]) {
      documentsByCollection.value[collectionId] = documentsByCollection.value[collectionId].filter((d) => d.id !== id);
    }
    delete linksByDoc.value[id];
    delete backlinksByDoc.value[id];
    delete unlinkedMentionsByDoc.value[id];
    // 文档删除改变所在集合连边拓扑：邻域缓存整体失效（小载荷重拉廉价）。
    neighborhoodsByKey.value = {};
    invalidateTree(collectionId);
  }

  // B1：文档重嵌入受理（从已存正文重建向量，无需原文件）；docIds 空 = 全库待重建文档。
  // 受理后文档状态经摄取 WS 实时刷新，此处不触碰本地缓存。
  async function reembedDocuments(
    collectionId: string,
    docIds?: string[],
    chunkSize?: number,
    chunkOverlap?: number,
  ): Promise<{ accepted_count: number; skipped_count: number }> {
    return reembedDocumentsApi(collectionId, docIds, chunkSize, chunkOverlap);
  }

  // B2：词法库启用语义层（单向绑定全局 embedder）；集合字段由页面侧 loadCollections 刷新。
  async function enableCollectionSemantic(collectionId: string): Promise<EnableSemanticResult> {
    return enableCollectionSemanticApi(collectionId);
  }

  // US-14：跨库移动文档；本地缓存由页面侧 reload 刷新（源/目标库计数均变化）。
  async function moveDoc(id: string, targetCollectionId: string): Promise<KnowledgeDocument> {
    return moveDocument(id, targetCollectionId);
  }

  // G3-B4：库内跨目录移动；文档身份保留（更新缓存 rel_path/source），失效树缓存供重载。
  async function moveDocToDir(id: string, targetDir: string, conflictPolicy = ''): Promise<KnowledgeDocument> {
    const doc = await moveDocumentToDir(id, targetDir, conflictPolicy);
    const list = documentsByCollection.value[doc.collection_id];
    if (list) {
      const i = list.findIndex((d) => d.id === doc.id);
      if (i >= 0) list[i] = doc;
      else list.push(doc);
    }
    invalidateTree(doc.collection_id);
    return doc;
  }

  async function setDocumentVisibility(
    id: string,
    visibility: 'collection' | 'private',
  ): Promise<{ id: string; visibility: string; owner_user_id: string }> {
    const got = await updateDocumentVisibility(id, visibility);
    for (const list of Object.values(documentsByCollection.value)) {
      const i = list.findIndex((d) => d.id === id);
      if (i >= 0) {
        list[i] = { ...list[i], visibility: got.visibility, owner_user_id: got.owner_user_id };
      }
    }
    return got;
  }

  async function search(query: SearchKnowledgeQuery): Promise<KnowledgeChunk[]> {
    return searchKnowledge(query);
  }

  // SP1-G/I-3：文档级晋升到团队库；目标库文档结构变化（失效树缓存供重载），
  // 集合计数由页面侧 reload 刷新。
  async function promoteDocs(docIds: string[], targetCollectionId: string): Promise<PromoteResult> {
    const result = await promoteDocuments(docIds, targetCollectionId);
    invalidateTree(targetCollectionId);
    return result;
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
    documentsTruncatedByCollection,
    loading,
    embedderConfig,
    treeChildren,
    linksByDoc,
    backlinksByDoc,
    unlinkedMentionsByDoc,
    danglingByCollection,
    graphsByCollection,
    neighborhoodsByKey,
    invalidateTree,
    invalidateLinkCaches,
    loadVaultTree,
    loadDocumentLinks,
    loadBlockBacklinks,
    loadUnlinkedMentions,
    loadDanglingLinks,
    loadCollectionGraph,
    loadDocumentNeighborhood,
    loadCollections,
    addCollection,
    removeCollection,
    refreshCollection,
    loadDocuments,
    ingest,
    addVaultDir,
    addVaultDocument,
    removeDocument,
    reembedDocuments,
    enableCollectionSemantic,
    moveDoc,
    moveDocToDir,
    setDocumentVisibility,
    promoteDocs,
    search,
    loadEmbedderConfig,
    saveEmbedderConfig,
  };
});
