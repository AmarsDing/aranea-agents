/**
 * Knowledge 知识库：**`createKnowledgeService()`** → **`/v1/knowledge/...`**。
 *
 * 注意：Knowledge 后端依赖 Postgres + pgvector（EP-DATA-01 / EP-KN-01）。
 * 在未配置 Postgres 的环境下，接口会返回明确错误，前端应做 "服务不可用" 降级提示。
 */
import { createKnowledgeService } from '../../services';
import { kratosApi } from '../../services/axiosHandler';
import { asRecord, pickBool, pickI32, pickI64, pickNum, pickStr } from '../../shared/wireJson';
import type {
  BlockBacklink,
  CollectionGraph,
  CollectionGraphEdge,
  CollectionGraphNode,
  CreateCollectionInput,
  DanglingLink,
  EntityMergeSuggestion,
  IngestDocumentInput,
  KnowledgeChunk,
  KnowledgeCollection,
  KnowledgeDocument,
  KnowledgeDocumentContent,
  MergeEntitiesResult,
  KnowledgeLink,
  ListCollectionsResult,
  ListDocumentsResult,
  PromoteResult,
  SearchKnowledgeQuery,
  UnlinkedMention,
  EmbedderConfig,
  UpdateEmbedderConfigInput,
  VaultTreeNode,
} from './types';

const svc = createKnowledgeService();

function pickStrList(r: Record<string, unknown>, snake: string, camel: string): string[] {
  const raw = r[snake] ?? r[camel];
  return Array.isArray(raw) ? raw.filter((v): v is string => typeof v === 'string') : [];
}

function mapCollection(raw: unknown): KnowledgeCollection {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    name: pickStr(r, 'name', 'name'),
    description: pickStr(r, 'description', 'description'),
    embedding_model: pickStr(r, 'embedding_model', 'embeddingModel'),
    dim: pickI32(r, 'dim', 'dim'),
    status: pickStr(r, 'status', 'status'),
    document_count: pickI32(r, 'document_count', 'documentCount'),
    chunk_count: pickI32(r, 'chunk_count', 'chunkCount'),
    workspace: pickStr(r, 'workspace', 'workspace'),
    created_at: pickStr(r, 'created_at', 'createdAt'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
    root_path: pickStr(r, 'root_path', 'rootPath'),
    sync_state: pickStr(r, 'sync_state', 'syncState'),
    last_sync_at: pickStr(r, 'last_sync_at', 'lastSyncAt'),
    vault_backend: pickStr(r, 'vault_backend', 'vaultBackend'),
  };
}

function mapDocument(raw: unknown): KnowledgeDocument {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    collection_id: pickStr(r, 'collection_id', 'collectionId'),
    source: pickStr(r, 'source', 'source'),
    mime_type: pickStr(r, 'mime_type', 'mimeType'),
    size_bytes: pickI64(r, 'size_bytes', 'sizeBytes'),
    chunk_count: pickI32(r, 'chunk_count', 'chunkCount'),
    status: pickStr(r, 'status', 'status'),
    error_message: pickStr(r, 'error_message', 'errorMessage'),
    created_at: pickStr(r, 'created_at', 'createdAt'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
    extract_supported:
      r.extract_supported !== undefined || r.extractSupported !== undefined
        ? Boolean(r.extract_supported ?? r.extractSupported)
        : undefined,
    rel_path: pickStr(r, 'rel_path', 'relPath'),
    summary: pickStr(r, 'summary', 'summary'),
    tags: pickStrList(r, 'tags', 'tags'),
    doc_type: pickStr(r, 'doc_type', 'docType'),
  };
}

function mapVaultTreeNode(raw: unknown): VaultTreeNode {
  const r = asRecord(raw);
  return {
    name: pickStr(r, 'name', 'name'),
    path: pickStr(r, 'path', 'path'),
    kind: pickStr(r, 'kind', 'kind'),
    doc_id: pickStr(r, 'doc_id', 'docId'),
    summary: pickStr(r, 'summary', 'summary'),
    tags: pickStrList(r, 'tags', 'tags'),
    doc_type: pickStr(r, 'doc_type', 'docType'),
    status: pickStr(r, 'status', 'status'),
    size_bytes: pickI64(r, 'size_bytes', 'sizeBytes'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
    error_message: pickStr(r, 'error_message', 'errorMessage'),
  };
}

function mapKnowledgeLink(raw: unknown): KnowledgeLink {
  const r = asRecord(raw);
  return {
    target_doc_id: pickStr(r, 'target_doc_id', 'targetDocId'),
    target_source: pickStr(r, 'target_source', 'targetSource'),
    target_rel_path: pickStr(r, 'target_rel_path', 'targetRelPath'),
    link_type: pickStr(r, 'link_type', 'linkType'),
    context: pickStr(r, 'context', 'context'),
    direction: pickStr(r, 'direction', 'direction'),
  };
}

function mapChunk(raw: unknown): KnowledgeChunk {
  const r = asRecord(raw);
  const embeddingRaw = r.embedding ?? r.Embedding;
  const embedding = Array.isArray(embeddingRaw)
    ? (embeddingRaw as unknown[]).map((v) => (typeof v === 'number' ? v : Number(v) || 0))
    : [];
  return {
    id: pickStr(r, 'id', 'id'),
    doc_id: pickStr(r, 'doc_id', 'docId'),
    collection_id: pickStr(r, 'collection_id', 'collectionId'),
    content: pickStr(r, 'content', 'content'),
    embedding,
    metadata_json: pickStr(r, 'metadata_json', 'metadataJson'),
    chunk_index: pickI32(r, 'chunk_index', 'chunkIndex'),
    score: pickNum(r, 'score', 'score'),
  };
}

// ---------- Collections ----------

export async function listCollections(
  params: { limit?: number; offset?: number } = {},
): Promise<ListCollectionsResult> {
  const res = asRecord(await svc.ListCollections({ limit: params.limit, offset: params.offset }));
  const itemsRaw = res.items ?? res.Items;
  const items = Array.isArray(itemsRaw) ? itemsRaw.map(mapCollection) : [];
  return { items, total: pickI32(res, 'total', 'total') || items.length };
}

export async function getCollection(id: string): Promise<KnowledgeCollection> {
  const raw = await svc.GetCollection({ id });
  return mapCollection(raw);
}

export async function createCollection(input: CreateCollectionInput): Promise<KnowledgeCollection> {
  const raw = await svc.CreateCollection({
    name: input.name,
    description: input.description ?? '',
    embeddingModel: input.embedding_model ?? '',
    rootPath: input.root_path,
  });
  return mapCollection(raw);
}

export async function deleteCollection(id: string): Promise<void> {
  await svc.DeleteCollection({ id });
}

// ---------- Documents ----------

export async function listDocuments(
  collectionId: string,
  params: { limit?: number; offset?: number } = {},
): Promise<ListDocumentsResult> {
  const res = asRecord(await svc.ListDocuments({ collectionId, limit: params.limit, offset: params.offset }));
  const itemsRaw = res.items ?? res.Items;
  const items = Array.isArray(itemsRaw) ? itemsRaw.map(mapDocument) : [];
  return { items, total: pickI32(res, 'total', 'total') || items.length };
}

export async function ingestDocument(input: IngestDocumentInput): Promise<KnowledgeDocument> {
  const raw = await svc.IngestDocument({
    collectionId: input.collection_id,
    source: input.source,
    mimeType: input.mime_type ?? '',
    contentBase64: input.content_base64,
    metadataJson: input.metadata_json ?? '{}',
    chunkSize: input.chunk_size ?? 0,
    chunkOverlap: input.chunk_overlap ?? 0,
    chunkStrategy: input.chunk_strategy ?? '',
    organizeToMarkdown: input.organize_to_markdown,
    targetDir: input.target_dir ?? '',
  });
  return mapDocument(raw);
}

export async function getDocumentContent(id: string): Promise<KnowledgeDocumentContent> {
  const r = asRecord(await svc.GetDocumentContent({ id }));
  return {
    id: pickStr(r, 'id', 'id'),
    content_text: pickStr(r, 'content_text', 'contentText'),
    organized: pickBool(r, 'organized', 'organized'),
    raw_content: pickStr(r, 'raw_content', 'rawContent'),
    base_hash: pickStr(r, 'base_hash', 'baseHash'),
  };
}

/** updateDocumentContent 编辑保存（G2-B5）：body 写回 vault 文件（frontmatter 保留），
 *  CAS 冲突仍写入并留双份（返回 conflict=true，前端提示重载）。 */
export async function updateDocumentContent(
  id: string,
  content: string,
  baseHash: string,
): Promise<{ document: KnowledgeDocument; conflict: boolean }> {
  const r = asRecord(await svc.UpdateDocumentContent({ id, content, baseHash }));
  return {
    document: mapDocument(r.document ?? r.Document),
    conflict: pickBool(r, 'conflict', 'conflict'),
  };
}

/** fetchDocumentAsset 原始文件流（G2-B6）：带 JWT 拉取 blob，供 <img>/<audio>/<video>
 *  object URL 渲染或 word 等原文下载。skipErrorNotify——失败由调用方降级处理。 */
export async function fetchDocumentAsset(id: string): Promise<{ blob: Blob; filename: string }> {
  const res = await kratosApi.get(`/v1/knowledge/documents/${encodeURIComponent(id)}/asset`, {
    responseType: 'blob',
    skipErrorNotify: true,
  });
  const cd = String(res.headers?.['content-disposition'] ?? '');
  const m = /filename="([^"]*)"/.exec(cd);
  return { blob: res.data as Blob, filename: m?.[1] ?? '' };
}

export async function deleteDocument(id: string): Promise<void> {
  await svc.DeleteDocument({ id });
}

// US-14：文档跨库移动（默认库收件箱 → 分类库归档）；目标库 dim 不一致时后端拒绝。
export async function moveDocument(id: string, targetCollectionId: string): Promise<KnowledgeDocument> {
  const raw = await svc.MoveDocument({ id, targetCollectionId });
  return mapDocument(raw);
}

// G3-B4：库内跨目录移动（拖拽移动）；同名冲突 CodeConflict → 前端弹 覆盖(overwrite)/
// 保留两份(rename)/取消；文档身份/chunks/hash 保留，入链服务端重建。
export async function moveDocumentToDir(
  id: string,
  targetDir: string,
  conflictPolicy = '',
): Promise<KnowledgeDocument> {
  const raw = await svc.MoveDocumentToDir({ id, targetDir, conflictPolicy });
  return mapDocument(raw);
}

// ---------- Vault explorer（P3 资源管理器） ----------

/** listVaultTree 懒加载 vault 文件夹直接子节点（prefix 空 = 根层）。 */
export async function listVaultTree(collectionId: string, prefix = ''): Promise<VaultTreeNode[]> {
  const res = asRecord(await svc.ListVaultTree({ collectionId, prefix }));
  const itemsRaw = res.items ?? res.Items;
  return Array.isArray(itemsRaw) ? itemsRaw.map(mapVaultTreeNode) : [];
}

/** createVaultDir 树内新建目录（G1-B2；幂等，嵌套父级一并创建）。 */
export async function createVaultDir(collectionId: string, dirPath: string): Promise<void> {
  await svc.CreateVaultDir({ collectionId, dirPath });
}

/** createVaultDocument 树内新建模板 .md 并立即索引（G1-B2；同名 CodeConflict）。 */
export async function createVaultDocument(collectionId: string, relPath: string): Promise<KnowledgeDocument> {
  const raw = await svc.CreateVaultDocument({ collectionId, relPath });
  return mapDocument(raw);
}

/** listDocumentLinks 列出文档已解析关联（双向；linkType 空 = 全部三类，R-3 来源标注）。 */
export async function listDocumentLinks(docId: string, linkType = ''): Promise<KnowledgeLink[]> {
  const res = asRecord(await svc.ListDocumentLinks({ id: docId, linkType }));
  const itemsRaw = res.items ?? res.Items;
  return Array.isArray(itemsRaw) ? itemsRaw.map(mapKnowledgeLink) : [];
}

// ---------- Block backlinks（SP1-E/I-1 块级反链） ----------

function mapBlockBacklink(raw: unknown): BlockBacklink {
  const r = asRecord(raw);
  return {
    src_block_id: pickStr(r, 'src_block_id', 'srcBlockId'),
    src_doc_id: pickStr(r, 'src_doc_id', 'srcDocId'),
    src_collection_id: pickStr(r, 'src_collection_id', 'srcCollectionId'),
    src_doc_name: pickStr(r, 'src_doc_name', 'srcDocName'),
    raw_target: pickStr(r, 'raw_target', 'rawTarget'),
    edge_type: pickStr(r, 'edge_type', 'edgeType'),
    context: pickStr(r, 'context', 'context'),
    ambiguous: pickBool(r, 'ambiguous', 'ambiguous'),
  };
}

/** listBlockBacklinks 列出文档的块级反向链接（SP1-E：按文档聚合所有块的入边）。
 *  生成客户端只实现主绑定 blocks/{block_id} 路径（block_id 必填），doc 级聚合走
 *  additional_binding GET /v1/knowledge/documents/{doc_id}/block-backlinks 直连。
 *  注意：kratosApi.get 裸调返回 AxiosResponse，载荷在 .data（requestHandler 才解包）。 */
export async function listBlockBacklinks(docId: string): Promise<BlockBacklink[]> {
  const res = asRecord(
    (await kratosApi.get(`/v1/knowledge/documents/${encodeURIComponent(docId)}/block-backlinks`)).data,
  );
  const itemsRaw = res.items ?? res.Items;
  return Array.isArray(itemsRaw) ? itemsRaw.map(mapBlockBacklink) : [];
}

// ---------- Dangling links（SP1-E/I-2 悬空链） ----------

function mapDanglingLink(raw: unknown): DanglingLink {
  const r = asRecord(raw);
  const refsRaw = r.refs ?? r.Refs;
  return {
    raw_target: pickStr(r, 'raw_target', 'rawTarget'),
    ref_count: pickI32(r, 'ref_count', 'refCount'),
    refs: Array.isArray(refsRaw) ? refsRaw.map(mapBlockBacklink) : [],
  };
}

/** listDanglingLinks 悬空链列表（SP1-E：raw_target 聚合 + 引用计数，「未创建笔记」视图）。 */
export async function listDanglingLinks(collectionId: string): Promise<DanglingLink[]> {
  const res = asRecord(await svc.ListDanglingLinks({ id: collectionId }));
  const itemsRaw = res.items ?? res.Items;
  return Array.isArray(itemsRaw) ? itemsRaw.map(mapDanglingLink) : [];
}

// ---------- Unlinked mentions（P2-7 未链接提及） ----------

function mapUnlinkedMention(raw: unknown): UnlinkedMention {
  const r = asRecord(raw);
  return {
    src_doc_id: pickStr(r, 'src_doc_id', 'srcDocId'),
    src_doc_name: pickStr(r, 'src_doc_name', 'srcDocName'),
    count: pickI32(r, 'count', 'count'),
    snippet: pickStr(r, 'snippet', 'snippet'),
  };
}

/** listUnlinkedMentions 未链接提及（P2-7）：本文档名在他文档正文中的纯文本出现。
 *  GET /v1/knowledge/documents/{doc_id}/unlinked-mentions 直连（kratosApi.get 裸调
 *  返回 AxiosResponse，载荷在 .data）。 */
export async function listUnlinkedMentions(docId: string): Promise<UnlinkedMention[]> {
  const res = asRecord(
    (await kratosApi.get(`/v1/knowledge/documents/${encodeURIComponent(docId)}/unlinked-mentions`)).data,
  );
  const itemsRaw = res.items ?? res.Items;
  return Array.isArray(itemsRaw) ? itemsRaw.map(mapUnlinkedMention) : [];
}

// ---------- Promote（SP1-G/I-3 晋升到团队库） ----------

function mapPromoteResult(raw: unknown): PromoteResult {
  const r = asRecord(raw);
  const createdRaw = r.created_blocks ?? r.createdBlocks;
  const cascadeRaw = r.cascade_candidates ?? r.cascadeCandidates;
  return {
    created_blocks: Array.isArray(createdRaw)
      ? createdRaw.map((v) => {
          const e = asRecord(v);
          return {
            src_block_id: pickStr(e, 'src_block_id', 'srcBlockId'),
            new_block_id: pickStr(e, 'new_block_id', 'newBlockId'),
            target_doc_id: pickStr(e, 'target_doc_id', 'targetDocId'),
          };
        })
      : [],
    cascade_candidates: Array.isArray(cascadeRaw)
      ? cascadeRaw.map((v) => {
          const e = asRecord(v);
          return {
            src_block_id: pickStr(e, 'src_block_id', 'srcBlockId'),
            raw_target: pickStr(e, 'raw_target', 'rawTarget'),
            dst_doc_id: pickStr(e, 'dst_doc_id', 'dstDocId'),
            dst_collection_id: pickStr(e, 'dst_collection_id', 'dstCollectionId'),
          };
        })
      : [],
  };
}

/** promoteDocuments 文档级晋升（SP1-I）：后端解析整文档全部块走同一晋升管线
 *  （谱系 + 级联提示 + 目标文档 chunk 重放）。目标库必须 vault_backend=team。 */
export async function promoteDocuments(docIds: string[], targetCollectionId: string): Promise<PromoteResult> {
  const raw = await svc.PromoteBlocks({ docIds, targetCollectionId });
  return mapPromoteResult(raw);
}

// ---------- Collection graph（G4-B8 3D 知识图谱） ----------

function mapGraphNode(raw: unknown): CollectionGraphNode {
  const r = asRecord(raw);
  return {
    doc_id: pickStr(r, 'doc_id', 'docId'),
    name: pickStr(r, 'name', 'name'),
    rel_path: pickStr(r, 'rel_path', 'relPath'),
    doc_type: pickStr(r, 'doc_type', 'docType'),
    degree: pickI32(r, 'degree', 'degree'),
  };
}

function mapGraphEdge(raw: unknown): CollectionGraphEdge {
  const r = asRecord(raw);
  return {
    source: pickStr(r, 'source', 'source'),
    target: pickStr(r, 'target', 'target'),
    type: pickStr(r, 'type', 'type'),
  };
}

/** listCollectionGraph 单库全量图谱（G4-B8）：linkTypes 空 = 全部类型；
 *  pathPrefix 目录前缀过滤（空 = 全库）；一次性返回，无分页。 */
export async function listCollectionGraph(
  collectionId: string,
  linkTypes: string[] = [],
  pathPrefix = '',
): Promise<CollectionGraph> {
  const res = asRecord(await svc.ListCollectionGraph({ collectionId, linkTypes, pathPrefix }));
  const nodesRaw = res.nodes ?? res.Nodes;
  const edgesRaw = res.edges ?? res.Edges;
  return {
    nodes: Array.isArray(nodesRaw) ? nodesRaw.map(mapGraphNode) : [],
    edges: Array.isArray(edgesRaw) ? edgesRaw.map(mapGraphEdge) : [],
  };
}

// ---------- 索引重建（SP2-6 命令面板） ----------

/** rebuildKnowledgeIndex 触发块级派生索引（blocks/refs）流式重建；异步执行，立即返回受理结果。 */
export async function rebuildKnowledgeIndex(collectionId: string): Promise<void> {
  await svc.RebuildKnowledgeIndex({ id: collectionId });
}

// ---------- 实体治理（G5-F/G5-G） ----------

function mapMergeSuggestion(raw: unknown): EntityMergeSuggestion {
  const r = asRecord(raw);
  return {
    keeper_id: pickI64(r, 'keeper_id', 'keeperId'),
    keeper_name: pickStr(r, 'keeper_name', 'keeperName'),
    mergee_id: pickI64(r, 'mergee_id', 'mergeeId'),
    mergee_name: pickStr(r, 'mergee_name', 'mergeeName'),
    source: pickStr(r, 'source', 'source'),
    similarity: pickNum(r, 'similarity', 'similarity'),
    tier: pickStr(r, 'tier', 'tier'),
  };
}

/** listEntityMergeSuggestions 合并建议列表（G5-F B11）：norm 冲突组在前，
 *  embedding 高相似对按相似度降序追加；未配置 embedding 时仅 norm 组。 */
export async function listEntityMergeSuggestions(collectionId: string): Promise<EntityMergeSuggestion[]> {
  const res = asRecord(await svc.ListEntityMergeSuggestions({ collectionId }));
  const itemsRaw = res.items ?? res.Items;
  return Array.isArray(itemsRaw) ? itemsRaw.map(mapMergeSuggestion) : [];
}

/** mergeKnowledgeEntities 一键合并（G5-F B10）：事务重写 mergee 的文档提及与
 *  关联引用指向 keeper，mergee 名落 keeper 别名（跨同步持久）；返回重写条数。 */
export async function mergeKnowledgeEntities(input: {
  collectionId: string;
  keeperId: number;
  mergeeIds: number[];
}): Promise<MergeEntitiesResult> {
  const res = asRecord(await svc.MergeKnowledgeEntities(input));
  return {
    rewritten_mentions: pickI64(res, 'rewritten_mentions', 'rewrittenMentions'),
    rewritten_links: pickI64(res, 'rewritten_links', 'rewrittenLinks'),
    merged_entities: pickI64(res, 'merged_entities', 'mergedEntities'),
  };
}

// ---------- Search ----------

export async function searchKnowledge(query: SearchKnowledgeQuery): Promise<KnowledgeChunk[]> {
  const res = asRecord(
    await svc.Search({
      collectionId: query.collection_id,
      query: query.query,
      topK: query.top_k,
      minScore: query.min_score,
      filterJson: query.filter_json,
      useRerank: query.use_rerank,
      rerankCandidates: query.rerank_candidates,
      rewriteStrategy: query.rewrite_strategy ?? '',
      hybridSearch: query.hybrid_search ?? '',
      // G3-B7：搜索范围选择器（vault 相对目录前缀；空 = 全库）。
      pathPrefix: query.path_prefix ?? '',
    }),
  );
  const chunksRaw = res.chunks ?? res.Chunks;
  return Array.isArray(chunksRaw) ? chunksRaw.map(mapChunk) : [];
}

export async function getEmbedderConfig(): Promise<EmbedderConfig> {
  const r = asRecord(await svc.GetEmbedderConfig({}));
  return {
    provider: pickStr(r, 'provider', 'provider'),
    base_url: pickStr(r, 'base_url', 'baseUrl'),
    model: pickStr(r, 'model', 'model'),
    dim: pickI32(r, 'dim', 'dim'),
    configured: pickBool(r, 'configured', 'configured'),
    has_api_key: pickBool(r, 'has_api_key', 'hasApiKey'),
  };
}

export async function updateEmbedderConfig(input: UpdateEmbedderConfigInput): Promise<EmbedderConfig> {
  const raw = asRecord(
    await svc.UpdateEmbedderConfig({
      provider: input.provider ?? '',
      baseUrl: input.base_url ?? '',
      apiKey: input.api_key ?? '',
      model: input.model ?? '',
      dim: input.dim ?? 0,
    }),
  );
  const cfg = asRecord(raw.config ?? raw.Config);
  return {
    provider: pickStr(cfg, 'provider', 'provider'),
    base_url: pickStr(cfg, 'base_url', 'baseUrl'),
    model: pickStr(cfg, 'model', 'model'),
    dim: pickI32(cfg, 'dim', 'dim'),
    configured: pickBool(cfg, 'configured', 'configured'),
    has_api_key: pickBool(cfg, 'has_api_key', 'hasApiKey'),
  };
}
