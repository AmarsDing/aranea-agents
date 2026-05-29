/**
 * Knowledge 知识库：**`createKnowledgeService()`** → **`/v1/knowledge/...`**。
 *
 * 注意：Knowledge 后端依赖 Postgres + pgvector（EP-DATA-01 / EP-KN-01）。
 * 在未配置 Postgres 的环境下，接口会返回明确错误，前端应做 "服务不可用" 降级提示。
 */
import { createKnowledgeService } from "../../services";
import { asRecord, pickBool, pickI32, pickNum, pickStr, pickStrArray } from "../../shared/wireJson";
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
  UpdateEmbedderConfigInput
} from "./types";

const svc = createKnowledgeService();

function mapCollection(raw: unknown): KnowledgeCollection {
  const r = asRecord(raw);
  return {
    id: pickStr(r, "id", "id"),
    name: pickStr(r, "name", "name"),
    description: pickStr(r, "description", "description"),
    embedding_model: pickStr(r, "embedding_model", "embeddingModel"),
    dim: pickI32(r, "dim", "dim"),
    status: pickStr(r, "status", "status"),
    document_count: pickI32(r, "document_count", "documentCount"),
    chunk_count: pickI32(r, "chunk_count", "chunkCount"),
    workspace: pickStr(r, "workspace", "workspace"),
    created_at: pickStr(r, "created_at", "createdAt"),
    updated_at: pickStr(r, "updated_at", "updatedAt")
  };
}

function mapDocument(raw: unknown): KnowledgeDocument {
  const r = asRecord(raw);
  return {
    id: pickStr(r, "id", "id"),
    collection_id: pickStr(r, "collection_id", "collectionId"),
    source: pickStr(r, "source", "source"),
    mime_type: pickStr(r, "mime_type", "mimeType"),
    size_bytes: pickNum(r, "size_bytes", "sizeBytes"),
    chunk_count: pickI32(r, "chunk_count", "chunkCount"),
    status: pickStr(r, "status", "status"),
    error_message: pickStr(r, "error_message", "errorMessage"),
    created_at: pickStr(r, "created_at", "createdAt"),
    updated_at: pickStr(r, "updated_at", "updatedAt")
  };
}

function mapChunk(raw: unknown): KnowledgeChunk {
  const r = asRecord(raw);
  const embeddingRaw = r.embedding ?? r.Embedding;
  const embedding = Array.isArray(embeddingRaw)
    ? (embeddingRaw as unknown[]).map((v) => (typeof v === "number" ? v : Number(v) || 0))
    : [];
  return {
    id: pickStr(r, "id", "id"),
    doc_id: pickStr(r, "doc_id", "docId"),
    collection_id: pickStr(r, "collection_id", "collectionId"),
    content: pickStr(r, "content", "content"),
    embedding,
    metadata_json: pickStr(r, "metadata_json", "metadataJson"),
    chunk_index: pickI32(r, "chunk_index", "chunkIndex"),
    score: pickNum(r, "score", "score")
  };
}

// ---------- Collections ----------

export async function listCollections(params: { limit?: number; offset?: number } = {}): Promise<ListCollectionsResult> {
  const res = asRecord(await svc.ListCollections({ limit: params.limit, offset: params.offset }));
  const itemsRaw = res.items ?? res.Items;
  const items = Array.isArray(itemsRaw) ? itemsRaw.map(mapCollection) : [];
  return { items, total: pickI32(res, "total", "total") || items.length };
}

export async function getCollection(id: string): Promise<KnowledgeCollection> {
  const raw = await svc.GetCollection({ id });
  return mapCollection(raw);
}

export async function createCollection(input: CreateCollectionInput): Promise<KnowledgeCollection> {
  const raw = await svc.CreateCollection({
    name: input.name,
    description: input.description ?? "",
    embeddingModel: input.embedding_model
  });
  return mapCollection(raw);
}

export async function deleteCollection(id: string): Promise<void> {
  await svc.DeleteCollection({ id });
}

// ---------- Documents ----------

export async function listDocuments(
  collectionId: string,
  params: { limit?: number; offset?: number } = {}
): Promise<ListDocumentsResult> {
  const res = asRecord(
    await svc.ListDocuments({ collectionId, limit: params.limit, offset: params.offset })
  );
  const itemsRaw = res.items ?? res.Items;
  const items = Array.isArray(itemsRaw) ? itemsRaw.map(mapDocument) : [];
  return { items, total: pickI32(res, "total", "total") || items.length };
}

export async function ingestDocument(input: IngestDocumentInput): Promise<KnowledgeDocument> {
  const raw = await svc.IngestDocument({
    collectionId: input.collection_id,
    source: input.source,
    mimeType: input.mime_type ?? "",
    contentBase64: input.content_base64,
    metadataJson: input.metadata_json ?? "{}",
    chunkSize: input.chunk_size ?? 0,
    chunkOverlap: input.chunk_overlap ?? 0,
    chunkStrategy: input.chunk_strategy ?? ""
  });
  return mapDocument(raw);
}

export async function deleteDocument(id: string): Promise<void> {
  await svc.DeleteDocument({ id });
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
      rewriteStrategy: query.rewrite_strategy ?? "",
      hybridSearch: query.hybrid_search ?? ""
    })
  );
  const chunksRaw = res.chunks ?? res.Chunks;
  return Array.isArray(chunksRaw) ? chunksRaw.map(mapChunk) : [];
}

export async function getEmbedderConfig(): Promise<EmbedderConfig> {
  const r = asRecord(await svc.GetEmbedderConfig({}));
  return {
    provider: pickStr(r, "provider", "provider"),
    base_url: pickStr(r, "base_url", "baseUrl"),
    model: pickStr(r, "model", "model"),
    dim: pickI32(r, "dim", "dim"),
    configured: pickBool(r, "configured", "configured"),
    has_api_key: pickBool(r, "has_api_key", "hasApiKey")
  };
}

export async function updateEmbedderConfig(input: UpdateEmbedderConfigInput): Promise<EmbedderConfig> {
  const raw = asRecord(
    await svc.UpdateEmbedderConfig({
      provider: input.provider ?? "",
      baseUrl: input.base_url ?? "",
      apiKey: input.api_key ?? "",
      model: input.model ?? "",
      dim: input.dim ?? 0
    })
  );
  const cfg = asRecord(raw.config ?? raw.Config);
  return {
    provider: pickStr(cfg, "provider", "provider"),
    base_url: pickStr(cfg, "base_url", "baseUrl"),
    model: pickStr(cfg, "model", "model"),
    dim: pickI32(cfg, "dim", "dim"),
    configured: pickBool(cfg, "configured", "configured"),
    has_api_key: pickBool(cfg, "has_api_key", "hasApiKey")
  };
}
