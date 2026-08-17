# Knowledge 知识库模块 — 实现设计文档

> 对应需求：`37 knowledge.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> **2026-06-17 校准**：与实际代码对齐；修正 Embedder 接口/结构体（`Embedder` 为接口，`MultiProviderEmbedder` 为实现）、修正构造函数签名（补充 `lg loggateway.Logger` 参数）、修正 `knowledge_embed_setting.go` 引用（实际逻辑在 `knowledge/knowledge.go` 的 `ApplyEmbedPatch`）、补充 Reranker、Embedder Admin API、摄取 WS 事件、`ingest.go` 流水线拆分、Advanced RAG（查询重写/混合检索/自适应路由/检索评估）、Agentic RAG（联邦搜索/knowledge_reflect 工具）、OCR stub、BM25 双路检索、Biz 子包迁移、KnowledgeSearchDeps 聚合、GraphRAG/Skill Knowledge 待实现设计。
> **2026-07-20 升级**：统一摄取管线设计——Extractor 接口抽象（§5.2）收编文本提取、MarkdownOrganizer（§5.2b，LLM 整理为 MD）、VisionExtractor（§5.2c，Phase 2 多模态）、`knowledge_documents` 新增 content_text/organized/asset_uri 列、Proto 新增 `organize_to_markdown` 字段与 `GetDocumentContent` RPC、OOXML magic 二次判定修复、前端拖拽批量上传、GraphRAG 裁决为 Phase 3 旁路非侵入可选增强。
> **2026-07-21 校准（Phase 9 实现）**：图片改「先落文档 + 后台异步提取」（§5.2c/§7.2）——VisionExtractor 在摄取 goroutine 内执行，成功后经新增 `DocumentRepo.UpdateDocumentContent` 回写 content_text/organized；原图留存落地为 `AssetStore`（`internal/knowledge/asset_store.go`）；Wire 工厂 `NewKnowledgeExtractorRegistry`/`NewKnowledgeAssetStore` 就位（§7.3）。
> **2026-07-21 升级（US-14 免选择知识库，Phase 11）**：「存储可分类，使用免选」——上传免预选（默认知识库懒创建，§7.4）、Search/Ingest 的 collection_id 去 REQUIRED（§2.1）、knowledge_search/knowledge_reflect 工具 collection 参数改可选 + scoped 多库/全库智能路由（§6.1/§6.1b）、文档跨库移动 MoveDocument（§2.1/§3.2/§4.3）。
> **2026-07-25 升级（Vault 重设计，评审通过待实施）**：新增 §子模块 Vault 重设计——文件系统即真相源（Vault=本地文件夹，Markdown+frontmatter 摘要卡，PG/pgvector 降级为可重建派生索引）、三层检索（L0 强 BM25 精确层 / L1 导航层 / L2 语义层插件化，embedding 从必需降级为可选增强）、Agent 工具族（navigate/grep/write）、Collection→Vault 迁移；评审 R-1~R-6 已合入为实现契约（§V6）。依据：2026-07-25 评审报告 + 无 embedding 检索调研。

---

## 一、模块概述

RAG 知识库：文档导入、分块、向量化、检索增强。对标 trpc-agent-go `knowledge` 包，当前实现基于 Collection 模型的 RAG 流水线，已升级至 Advanced RAG + 部分 Agentic RAG。

### 核心架构

```
文档上传(base64) → Chunker(分块) → Embedder(向量化) → pgvector(存储)
                                                              ↓
Agent 调用 knowledge_search ← Tool(搜索工具) ← AdaptiveRouter ← HybridRetriever ← pgvector(搜索)
                                   ↓                    ↓              ↓
                              knowledge_reflect    QueryRewriter   RetrievalEvaluator
                                   ↓
                              FederatedRetriever ← 多 Collection 并行搜索
```

### 实际代码结构

```
internal/
├── biz/
│   ├── knowledge.go              # 类型别名转发（KnowledgeRepo = knowledge.Repo 等 + ApplyKnowledgeEmbedPatch 等）
│   └── knowledge/                # 领域子包
│       └── knowledge.go          # Collection/Document/Chunk 模型 + Repo/Usecase 接口 + EmbedSetting patch 合并
├── data/knowledge.go             # KnowledgeRepo 实现（PostgreSQL + pgvector + BM25 双路 raw SQL）
├── service/
│   ├── knowledge.go              # KnowledgeService（Kratos 传输适配，KnowledgeSearchDeps 聚合）
│   ├── knowledge_embedder.go     # Embedder Wire 工厂（EP-KN-01）
│   ├── knowledge_retriever.go    # Retriever + env Reranker（KN-01）
│   └── knowledge_advanced.go     # Advanced RAG 组件 Wire 工厂（6 个 Provider）
├── knowledge/
│   ├── chunker.go                # 文本分块（char/token 策略）
│   ├── embedder.go               # 向量化（openai/ollama/gemini/huggingface + EmbedBatch）
│   ├── chunk_strategy.go         # trpc 高级分块桥接（markdown/json/recursive）
│   ├── document_extract.go       # PDF/DOCX/HTML 文本提取
│   ├── ocr.go                    # OCR 提供者接口（stub，KNOWLEDGE_OCR 环境变量）
│   ├── html_text.go              # HTML 文本剥离（strip script/style）
│   ├── readers_import.go         # trpc document reader 注册
│   ├── ingest.go                 # 分块+向量化流水线（IngestParams.ApplyDefaults）
│   ├── retriever.go              # 检索器（embed + search + optional rerank + TaskTypeEmbedder）
│   ├── reranker_factory.go       # env → trpc reranker（topk/cohere/infinity）
│   ├── memory_rerank_adapter.go  # trpc reranker → biz.Reranker（Memory L2/L3；AH-04 从 data 上移）
│   ├── memory_rerank_factory.go  # KRATOS_MEMORY_RERANKER → NewMemoryReranker（Wire 注入 data）
│   ├── query_rewriter.go         # 查询重写（HyDE/Decomposition/MultiQuery）
│   ├── hybrid_retriever.go       # 混合检索（Dense+Sparse+RRF 融合）
│   ├── adaptive_router.go        # 自适应检索路由（查询复杂度分类）
│   ├── retrieval_evaluator.go    # 检索质量评估（CRAG 式自校验）
│   ├── federated_retriever.go    # 跨 Collection 联邦搜索（Broadcast + Route 策略）
│   ├── search_helpers.go         # 检索评估辅助（ChunkSearcher/ChunkAssessor/SearchWithEvaluation）
│   └── llm_resolver.go          # LLM 模型解析（Advanced RAG 共用）
├── tools/knowledge/tool.go       # knowledge_search + knowledge_reflect 工具
├── agent/knowledge_inject.go     # Plan-Then-Retrieve BeforeModel 钩子
└── agent/trpc_build.go           # Agent 装配（KnowledgeSearch/KnowledgeReflect 开关）
```

---

## 二、Proto 层

### 2.1 api/kratos/knowledge/v1/knowledge.proto

```protobuf
syntax = "proto3";

package kratos.knowledge.v1;

import "google/api/annotations.proto";
import "google/api/field_behavior.proto";
import "google/protobuf/empty.proto";

option go_package = "aranea-agents/api/kratos/knowledge/v1;v1";

// KnowledgeCollection — 命名向量存储，绑定固定嵌入模型。
message KnowledgeCollection {
  string id = 1;
  string name = 2;
  string description = 3;
  string embedding_model = 4;
  int32  dim = 5;
  string status = 6;          // active | indexing | error
  int32  document_count = 7;
  int32  chunk_count = 8;
  string workspace = 9;
  string created_at = 10;
  string updated_at = 11;
}

// KnowledgeDocument — 摄入集合的一个源文档。
message KnowledgeDocument {
  string id = 1;
  string collection_id = 2;
  string source = 3;          // filename, URL, or description
  string mime_type = 4;
  int64  size_bytes = 5;
  int32  chunk_count = 6;
  string status = 7;          // pending | indexing | indexed | error
  string error_message = 8;
  string created_at = 9;
  string updated_at = 10;
}

// KnowledgeChunk — 一个带嵌入向量的索引文本块。
message KnowledgeChunk {
  string id = 1;
  string doc_id = 2;
  string collection_id = 3;
  string content = 4;
  repeated float embedding = 5;
  string metadata_json = 6;
  int32  chunk_index = 7;
  float  score = 8;           // 相似度分数（仅搜索结果）
}

// --- Requests / Responses ---

message CreateCollectionRequest {
  string name = 1 [(google.api.field_behavior) = REQUIRED];
  string description = 2;
  string embedding_model = 3;   // V2 可选：留空 = 仅词法检索，不建语义层
  string root_path = 4;         // SP1-F 条件必填：backend=local 必填（本地文件夹绝对路径），team 必须为空
  string vault_backend = 5;     // SP1-F：local（缺省）| team
}

message GetCollectionRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ListCollectionsRequest {
  int32 limit = 1;
  int32 offset = 2;
}

message ListCollectionsResponse {
  repeated KnowledgeCollection items = 1;
  int32 total = 2;
}

message DeleteCollectionRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message IngestDocumentRequest {
  // US-14：可选。留空 = 自动落「默认知识库」（服务端懒创建），上传免预选。
  string collection_id = 1;
  string source = 2 [(google.api.field_behavior) = REQUIRED];
  string mime_type = 3;
  string content_base64 = 4 [(google.api.field_behavior) = REQUIRED];
  string metadata_json = 5;
  int32 chunk_size = 6;       // 0 = 服务端默认 512
  int32 chunk_overlap = 7;    // 0 = 服务端默认 64
  string chunk_strategy = 8;  // char|token|markdown|json|recursive
  optional bool organize_to_markdown = 9;  // unset/true = LLM 整理为 MD（默认开启，失败降级原文本）
}

// GetDocumentContent — 预览整理后的 Markdown 全文（content_text）。
message GetDocumentContentRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}
message DocumentContent {
  string id = 1;
  string content_text = 2;   // 整理后的 Markdown（未整理时为提取原文）
  bool   organized = 3;      // 是否经 LLM 整理
}

message ListDocumentsRequest {
  string collection_id = 1;
  int32 limit = 2;
  int32 offset = 3;
}

message ListDocumentsResponse {
  repeated KnowledgeDocument items = 1;
  int32 total = 2;
}

message DeleteDocumentRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

// MoveDocument — US-14 文档跨库移动（整理：默认库收件箱 → 分类库归档）。
// 文档连同 chunks 移至目标 Collection，两侧计数同步校正；目标库 dim 不一致时拒绝（向量维度不兼容）。
message MoveDocumentRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string target_collection_id = 2 [(google.api.field_behavior) = REQUIRED];
}

message SearchRequest {
  // US-14：可选。留空 = 全库智能路由（Route 策略：名称/描述匹配取 top N 广播 + 结果合并）。
  string collection_id = 1;
  string query = 2 [(google.api.field_behavior) = REQUIRED];
  int32 top_k = 3;            // default 5
  float min_score = 4;        // 最低相似度阈值（0 = 不过滤）
  string filter_json = 5;     // 可选元数据过滤（JSON）
  optional bool use_rerank = 6;       // unset = 使用全局 reranker（若已配置）
  int32 rerank_candidates = 7;        // 重排前向量候选数（0 = 默认 oversample）
  // --- Advanced RAG 字段 ---
  string rewrite_strategy = 10;       // hyde | decomposition | multi_query（空 = 不重写）
  string hybrid_search = 11;          // auto | dense | sparse | rrf（空 = auto）
}

message GetEmbedderConfigRequest {}
message EmbedderConfig { /* provider, base_url, model, dim, configured, has_api_key */ }
message UpdateEmbedderConfigRequest { /* provider, base_url, api_key, model, dim */ }
message UpdateEmbedderConfigResponse { EmbedderConfig config = 1; }

message SearchResponse {
  repeated KnowledgeChunk chunks = 1;
}

service KnowledgeService {
  // Collections
  rpc CreateCollection(CreateCollectionRequest) returns (KnowledgeCollection) {
    option (google.api.http) = { post: "/v1/knowledge/collections" body: "*" };
  }
  rpc GetCollection(GetCollectionRequest) returns (KnowledgeCollection) {
    option (google.api.http) = { get: "/v1/knowledge/collections/{id}" };
  }
  rpc ListCollections(ListCollectionsRequest) returns (ListCollectionsResponse) {
    option (google.api.http) = { get: "/v1/knowledge/collections" };
  }
  rpc DeleteCollection(DeleteCollectionRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/knowledge/collections/{id}" };
  }

  // Documents
  rpc IngestDocument(IngestDocumentRequest) returns (KnowledgeDocument) {
    option (google.api.http) = { post: "/v1/knowledge/documents" body: "*" };
  }
  rpc ListDocuments(ListDocumentsRequest) returns (ListDocumentsResponse) {
    option (google.api.http) = { get: "/v1/knowledge/documents" };
  }
  rpc GetDocumentContent(GetDocumentContentRequest) returns (DocumentContent) {
    option (google.api.http) = { get: "/v1/knowledge/documents/{id}/content" };
  }
  rpc DeleteDocument(DeleteDocumentRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/knowledge/documents/{id}" };
  }
  rpc MoveDocument(MoveDocumentRequest) returns (KnowledgeDocument) {
    option (google.api.http) = { post: "/v1/knowledge/documents/{id}/move" body: "*" };
  }

  // Search
  rpc Search(SearchRequest) returns (SearchResponse) {
    option (google.api.http) = { post: "/v1/knowledge/search" body: "*" };
  }
  rpc GetEmbedderConfig(GetEmbedderConfigRequest) returns (EmbedderConfig) {
    option (google.api.http) = { get: "/v1/knowledge/embedder-config" };
  }
  rpc UpdateEmbedderConfig(UpdateEmbedderConfigRequest) returns (UpdateEmbedderConfigResponse) {
    option (google.api.http) = { put: "/v1/knowledge/embedder-config" body: "*" };
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

> 领域模型定义在 `internal/biz/knowledge/knowledge.go`，`internal/biz/knowledge.go` 通过类型别名转发。

```go
// internal/biz/knowledge/knowledge.go

type Collection struct {
    ID             string
    Name           string
    Description    string
    EmbeddingModel string
    Dim            int
    Status         string    // "active" | "indexing" | "error"
    DocumentCount  int
    ChunkCount     int
    Workspace      string
    CreatedAt      string
    UpdatedAt      string
}

type Document struct {
    ID           string
    CollectionID string
    Source       string
    MimeType     string
    SizeBytes    int64
    ChunkCount   int
    Status       string        // "pending" | "indexing" | "indexed" | "error"
    ErrorMessage string
    ContentText  string        // 整理后 MD 全文（未整理时为提取原文）
    Organized    bool          // 是否经 LLM 整理
    AssetURI     string        // 原始文件留存路径（Phase 2 图片血缘）
    CreatedAt    string
    UpdatedAt    string
}

type Chunk struct {
    ID           string
    DocID        string
    CollectionID string
    Content      string
    Embedding    []float32
    MetadataJSON string
    ChunkIndex   int
    Score        float32     // 仅搜索结果
}

type SearchQuery struct {
    CollectionID     string
    Query            string
    TopK             int
    MinScore         float32
    FilterJSON       string      // JSONB 元数据过滤
    UseRerank        *bool       // nil = 全局 reranker 启用时使用
    RerankCandidates int         // 重排前向量候选上限
    RewriteStrategy  string      // hyde | decomposition | multi_query（空 = 不重写）
    HybridSearch     string      // auto | dense | sparse | rrf（空 = auto）
}
```

### 3.2 Repo 接口

> 接口定义在 `internal/biz/knowledge/knowledge.go`，`internal/biz/knowledge.go` 通过类型别名转发（`type KnowledgeRepo = knowledge.Repo`）。

```go
// 子接口拆分
type CollectionRepo interface {
    CreateCollection(ctx context.Context, c Collection) (Collection, error)
    GetCollection(ctx context.Context, id string) (Collection, error)
    ListCollections(ctx context.Context, workspace string, limit, offset int) ([]Collection, int, error)
    DeleteCollection(ctx context.Context, id string) error
    UpdateCollectionCounts(ctx context.Context, id string, docDelta, chunkDelta int) error
}

type DocumentRepo interface {
    CreateDocument(ctx context.Context, d Document) (Document, error)
    GetDocument(ctx context.Context, id string) (Document, error)
    UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error
    // UpdateDocumentContent 回写文档正文与整理标记（Phase 9 图片异步提取完成后调用）。
    UpdateDocumentContent(ctx context.Context, id, contentText string, organized bool) error
    ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]Document, int, error)
    DeleteDocument(ctx context.Context, id string) error
    // MoveDocument 文档连同 chunks 移至目标 Collection（US-14），事务内完成 + 两侧计数校正。
    // 目标库 dim 与源库不一致时返回错误（向量维度不兼容，需重建索引）。
    MoveDocument(ctx context.Context, id, targetCollectionID string) (Document, error)
}

// 注：content_text/organized/asset_uri 随 CreateDocument 写入、GetDocument 读出（预览）；
// 图片异步提取完成后经 UpdateDocumentContent 回写 content_text/organized（2026-07-21 新增）。

type ChunkRepo interface {
    InsertChunks(ctx context.Context, chunks []Chunk) error
    DeleteChunksByDocument(ctx context.Context, docID string) error
    SearchChunks(ctx context.Context, q SearchQuery, queryEmbedding []float32) ([]Chunk, error)
}

// 组合接口（向后兼容）
type Repo interface {
    CollectionRepo
    DocumentRepo
    ChunkRepo
}
```

### 3.3 Usecase

```go
type Usecase struct {
    collections CollectionRepo
    documents   DocumentRepo
    chunks      ChunkRepo
}

func (uc *Usecase) CreateCollection(ctx context.Context, in Collection) (Collection, error)
func (uc *Usecase) GetCollection(ctx context.Context, id string) (Collection, error)
func (uc *Usecase) ListCollections(ctx context.Context, workspace string, limit, offset int) ([]Collection, int, error)
func (uc *Usecase) DeleteCollection(ctx context.Context, id string) error
func (uc *Usecase) UpdateCollectionCounts(ctx context.Context, id string, docDelta, chunkDelta int) error
// EnsureDefaultCollection — US-14：返回「默认知识库」，不存在则懒创建（name=默认知识库，
// embedding_model/dim 取当前 Embedder 配置）。上传免预选的兜底出口。
func (uc *Usecase) EnsureDefaultCollection(ctx context.Context, embeddingModel string, dim int) (Collection, error)

func (uc *Usecase) CreateDocument(ctx context.Context, d Document) (Document, error)
func (uc *Usecase) ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]Document, int, error)
func (uc *Usecase) DeleteDocument(ctx context.Context, id string) error
func (uc *Usecase) UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error
// MoveDocument — US-14 文档跨库移动（校验目标库存在且 dim 兼容后委托 Repo 事务）。
func (uc *Usecase) MoveDocument(ctx context.Context, id, targetCollectionID string) (Document, error)

func (uc *Usecase) InsertChunks(ctx context.Context, chunks []Chunk) error
func (uc *Usecase) Search(ctx context.Context, q SearchQuery, queryEmbedding []float32) ([]Chunk, error)
```

---

## 四、Data 层

### 4.1 存储选型

使用 **PostgreSQL + pgvector** raw SQL，不使用 Ent ORM。原因：
- 向量列（`vector(N)`）需要 pgvector 扩展，Ent 不原生支持。
- 向量搜索需要 `embedding <=> $1::vector` 专用操作符，raw SQL 更直接。
- Schema 由 `EnsureKnowledgeSchema` 在启动期创建。

### 4.2 数据库 Schema

由 `data.EnsureKnowledgeSchema(ctx, db, dim)` 创建：

```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS knowledge_collections (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    embedding_model TEXT NOT NULL,
    dim             INT  NOT NULL DEFAULT 1536,
    status          TEXT NOT NULL DEFAULT 'active',
    document_count  INT  NOT NULL DEFAULT 0,
    chunk_count     INT  NOT NULL DEFAULT 0,
    workspace       TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS knowledge_documents (
    id            TEXT PRIMARY KEY,
    collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
    source        TEXT NOT NULL,
    mime_type     TEXT NOT NULL DEFAULT '',
    size_bytes    BIGINT NOT NULL DEFAULT 0,
    chunk_count   INT    NOT NULL DEFAULT 0,
    status        TEXT   NOT NULL DEFAULT 'pending',
    error_message TEXT   NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2026-07-20 统一摄取管线升级（EnsureKnowledgeSchema 幂等 ALTER）：
ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS content_text TEXT NOT NULL DEFAULT '';   -- 整理后 MD 全文（未整理时为提取原文），供预览/血缘/Reindex
ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS organized    BOOLEAN NOT NULL DEFAULT FALSE; -- 是否经 LLM 整理
ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS asset_uri    TEXT NOT NULL DEFAULT '';       -- 原始文件留存路径（Phase 2 图片血缘）

CREATE TABLE IF NOT EXISTS knowledge_chunks (
    id            TEXT PRIMARY KEY,
    doc_id        TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
    collection_id TEXT NOT NULL,
    content       TEXT NOT NULL,
    embedding     vector(N),
    metadata      JSONB NOT NULL DEFAULT '{}',
    chunk_index   INT   NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS knowledge_chunks_embedding_idx
    ON knowledge_chunks USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
CREATE INDEX IF NOT EXISTS knowledge_chunks_collection_idx
    ON knowledge_chunks(collection_id);
```

#### 维度对账（2026-08-10 事故根修）

`knowledge_chunks.embedding vector(N)` 的 N 在**首次建表**时固定为当时的 `vector_dim` 配置；embedder 换模型（维度变化）后 `CREATE TABLE IF NOT EXISTS` 不会修正存量列，新维度向量插入全部被 PG 拒绝（`expected N dimensions`），而应用层按 `knowledge_collections.dim` 的校验反而通过，故障极难定位（2026-08-08 配置切 bge-m3/1024 后语义检索全灭）。

`EnsureKnowledgeSchema` 尾部执行 `reconcileEmbeddingDim`（幂等；列 typmod == 配置维度时零动作）：

1. 存量向量全部置 NULL 作废（旧维度向量对新 embedder 无检索意义）；
2. 受影响文档 `content_hash='' + status='pending'`——vault 文档下轮 sync 自动重嵌入自愈；UI 上传文档（rel_path 空）无 sync 循环，需人工重传；
3. 有语义层集合（`embedding_model <> ''`）的 `dim` 快照同步为新维度——单全局 embedder 架构下不同步 = 应用层校验永远拒绝新插入（死库）；`embedding_model` 名不动（Ensure 拿不到模型名，仅展示用）；
4. `ALTER COLUMN embedding TYPE vector(N)` + 重建 ivfflat 索引（lists 随新维度重算；先 DROP 再 ALTER，避免 PG 级联重建沿用旧 lists 参数）。

对账事件以 Error 级进程日志记录（`data.schema.knowledge_dim_reconcile`，含 from_dim/to_dim/pending_docs）。另：`SearchChunks` dense 查询排除 `embedding IS NULL` 行（对账后重嵌入窗口期的无向量行不参与 dense 检索）。

### 4.3 Repo 实现

`knowledgeRepo` 使用 `*sql.DB` 直接操作 PostgreSQL，同时实现 `biz.KnowledgeRepo`、`biz.KnowledgeSparseSearcher`、`bizknowledge.Repo` 三个接口：

```go
var (
    _ biz.KnowledgeRepo           = (*knowledgeRepo)(nil)
    _ biz.KnowledgeSparseSearcher = (*knowledgeRepo)(nil)
    _ bizknowledge.Repo           = (*knowledgeRepo)(nil)
)
```

| 方法 | SQL 要点 |
|------|----------|
| `CreateCollection` | `INSERT INTO knowledge_collections ... RETURNING ...` |
| `GetCollection` | `SELECT ... FROM knowledge_collections WHERE id = $1` |
| `ListCollections` | `WHERE workspace = $1 OR $1 = ''` + 分页 |
| `DeleteCollection` | `DELETE FROM knowledge_collections WHERE id = $1`（CASCADE 自动清理） |
| `UpdateCollectionCounts` | `UPDATE ... SET document_count = document_count + $2, chunk_count = chunk_count + $3` |
| `CreateDocument` | `INSERT INTO knowledge_documents ... RETURNING ...` |
| `UpdateDocumentStatus` | `UPDATE ... SET status, error_message, chunk_count, updated_at` |
| `InsertChunks` | 事务批量 `INSERT INTO knowledge_chunks`，使用 `pgvector.NewVector`，含维度校验 |
| `SearchChunks` | `ORDER BY embedding <=> $1::vector LIMIT $3`，排除 `embedding IS NULL` 行，支持 `min_score` 和 `filter_json` |
| `SearchChunksBM25` | 双路 BM25：tsvector 全文检索 + pg_trgm `word_similarity`（`%>` 操作符）模糊搜索，合并去重 |
| `DeleteDocument` | 事务删除 + 计数器修正 |
| `MoveDocument` | 单事务：documents/chunks collection_id 更新 + 源/目标库计数校正（US-14） |

> **trigram 路操作符选型（2026-08-10 中文短查询根修）**：旧实现 `similarity(content, q)` + `%` 的分母含双方 trigram 总数，中文 2-4 字短查询对长文档的相似度被稀释到 0.3 阈值以下（实测"斑马线"=0.064）永不命中，无语义层集合的词法降级对中文实质不可用。改为 `word_similarity(q, content)` + `%>`（查询 trigram 集 vs 文本连续区间的最大相似度，阈值 `pg_trgm.word_similarity_threshold`=0.6；中文子串处 0.67~1.0 命中），`gin_trgm_ops` 索引对 `%>` 同样可用。tsvector 路对 CJK 的局限（`simple` 分词整串单 token）见 §V6 1725 行论述，不在本次修复范围。

### 4.4 搜索过滤

`filter_json` 通过 JSONB `@>` 操作符实现元数据匹配：

```sql
AND metadata @> $4::jsonb
```

示例：`filter_json = '{"category": "policy"}'` 将匹配 `metadata` 中包含 `category: policy` 的 Chunk。

### 4.5 降级策略

`NewKnowledgeRepoFromData(d *Data)` 在无 Postgres 时返回 nil：

```go
func NewKnowledgeRepoFromData(d *Data) biz.KnowledgeRepo {
    if d == nil || d.Postgres() == nil {
        return nil
    }
    return NewKnowledgeRepo(d.Postgres())
}
```

---

## 五、Knowledge 内部包

### 5.1 Chunker（internal/knowledge/chunker.go）

```go
type ChunkStrategy string

const (
    ChunkByChar      ChunkStrategy = "char"
    ChunkByToken     ChunkStrategy = "token"
    ChunkByMarkdown  ChunkStrategy = "markdown"   // trpc MarkdownChunking
    ChunkByJSON      ChunkStrategy = "json"       // trpc JSONChunking
    ChunkByRecursive ChunkStrategy = "recursive"  // trpc RecursiveChunking
)

func ParseChunkStrategy(raw string) ChunkStrategy
func SplitWithStrategy(strategy ChunkStrategy, text string, size, overlap int) ([]Chunk, error)

type Chunk struct {
    Content    string
    ChunkIndex int
}

type Chunker struct {
    ChunkSize    int            // 默认 512
    ChunkOverlap int            // 默认 64
    Strategy     ChunkStrategy  // 默认 char
}

func NewChunker(size, overlap int, strategy ChunkStrategy) *Chunker
func (c *Chunker) Split(text string) []Chunk
```

**设计决策**：
- `char` 策略：按 rune 窗口滑动，适合中文等多字节文本。
- `token` 策略：按空格分词后按词数窗口，近似真实 Token 计数。
- 两者均支持重叠窗口，step = chunk_size - chunk_overlap。

- `char` / `token`：本地 `chunker.go` 实现。
- `markdown` / `json` / `recursive`：桥接 trpc `chunking/*`（`chunk_strategy.go`）。

### 5.2 文档解析与 Extractor 统一抽象（internal/knowledge/extractor.go + document_extract.go）

> **2026-07-20 升级**：为统一「文本类」与「多模态」两条入库路径，引入 Extractor 接口抽象。任何模态提取后归一为 Markdown 文本（NFR-13），下游 Organize/Chunk/Embed/检索完全无模态差异。

```go
// Extractor 将上传字节按模态提取为文本（Markdown 优先）。
type Extractor interface {
    // Supports 判定是否可处理该来源（按扩展名/MIME 路由）。
    Supports(ext, mimeType string) bool
    // Extract 提取文本；图片等多模态实现直接输出结构化 Markdown。
    Extract(ctx context.Context, raw []byte, source, mimeType string) (string, error)
}

// ExtractorRegistry 按优先级路由到首个 Supports 的实现。
type ExtractorRegistry struct{ extractors []Extractor }

func (r *ExtractorRegistry) Extract(ctx context.Context, raw []byte, source, mimeType string) (string, error)
```

**实现矩阵**：

| 实现 | 覆盖范围 | 阶段 |
|------|----------|------|
| `TextExtractor` | 文本直读（txt/md/json/csv/html/xml/yaml）+ trpc `document/reader`（pdf/doc/docx/xlsx/pptx）+ HTML 剥离 | Phase 1 |
| `VisionExtractor` | 图片（png/jpg/jpeg/webp）→ 多模态 LLM 输出结构化 MD 描述 | Phase 2 |

**TextExtractor**（收编现有 `ExtractDocumentText`）：

```go
func ExtractDocumentText(raw []byte, source, mimeType string) (string, error)
```

- PDF / DOCX：trpc `document/reader`（`readers_import.go` 侧载注册）。
- HTML：`html_text.go` 剥离 script/style 后提取可见文本。
- 纯文本：UTF-8 直读。
- 图片分支从 `ExtractDocumentText` 摘除，移交 `VisionExtractor`（Phase 2）；`ocr.go` 的 stub 接口废弃，由 VisionExtractor 取代。

### 5.2b MarkdownOrganizer（internal/knowledge/markdown_organizer.go，新增）

> Phase 1 核心新增：提取文本 → LLM 结构化整理为 Markdown。

```go
// MarkdownOrganizer 将提取出的原始文本整理为结构化 Markdown。
type MarkdownOrganizer struct {
    llm     biz.LLMCaller
    sys     *biz.SystemSettingUsecase
    catalog *biz.LlmProviderModelUsecase
    lg      loggateway.Logger
}

func NewMarkdownOrganizer(llm biz.LLMCaller, sys *biz.SystemSettingUsecase, catalog *biz.LlmProviderModelUsecase, lg loggateway.Logger) *MarkdownOrganizer

// Organize 输入提取文本与来源信息，输出结构化 Markdown。
// LLM 不可用 / 超时 / 解析失败 → 返回原文本 + organized=false（降级不阻塞，NFR-11）。
func (o *MarkdownOrganizer) Organize(ctx context.Context, text, source, mimeType string) (md string, organized bool, err error)
```

**设计决策**：
- LLM 模型经 `llm_resolver.go` 的 `ResolveLLM` 统一解析（与 QueryRewriter/RetrievalEvaluator 同构）。
- Prompt 约束：保留全部事实、不增删内容、重建标题层级/列表/表格、输出纯 Markdown。
- 超时 30s；输入超长时按 chunk_size 窗口分段整理后拼接（窗口内整理，保证标题层级连续）。
- `organized=false` 时调用方使用原文本继续分块，FlowLog 记录降级原因。
- Wire 工厂位于 `internal/service/knowledge_advanced.go`（第 7 个 Advanced 组件 Provider）。

### 5.2c VisionExtractor（internal/knowledge/vision_extractor.go，Phase 2 新增）

```go
// VisionExtractor 使用多模态 LLM 将图片理解结果输出为结构化 Markdown。
type VisionExtractor struct {
    llm     biz.LLMCaller
    sys     *biz.SystemSettingUsecase
    catalog *biz.LlmProviderModelUsecase
    lg      loggateway.Logger
}

func (v *VisionExtractor) Supports(ext, mimeType string) bool // image/* 或 .png/.jpg/.jpeg/.webp
func (v *VisionExtractor) Extract(ctx context.Context, raw []byte, source, mimeType string) (string, error)
```

**设计决策**：
- 实现 Extractor 接口，注册到 ExtractorRegistry（优先级高于 TextExtractor）。
- 多模态 LLM（gpt-4o / gemini 视觉等）输入图片 base64 + prompt，直接输出 MD（图中文字、表格、图表含义），产出物与文本类同构。
- 输出已是 Markdown 时可跳过 MarkdownOrganizer（流水线开关控制）。
- 未配置多模态模型时返回明确错误（NFR-12），文档状态 `error`。
- 原始图片留存：写入 asset 存储（本地目录），Document 记录 `asset_uri` 血缘。
- 替代原 `ocr.go` stub 路线（tesseract/docling 不再作为 OCR 依赖；docling 仅留作 PDF 版面保真的后续独立增强）。

**异步提取流程（2026-07-21 实现裁决）**：
- 图片走「先落文档 + 后台提取」：HTTP 立即返回 `status=pending` 的文档记录（含 `asset_uri` 血缘），视觉 LLM 提取在摄取 goroutine 内执行——与文本类的 chunk/embed 同一异步上下文，不阻塞请求（视觉调用最长 60s）。
- 提取成功后先经 `DocumentRepo.UpdateDocumentContent(id, contentText, organized=true)` 回写正文（`GetDocumentContent` 预览可用），再走统一 chunk/embed 流程；下游与文本类完全同构。
- 提取失败（无视觉模型 / LLM 调用失败 / 空响应）置 `status=error` + 明确 `error_message`（NFR-12），与 indexing → indexed/error 状态流一致。
- 视觉模型解析顺序：catalog 中声明 Vision 能力的启用模型 → 回退 `DefaultRefineLLM`；两者皆无则返回明确错误。
- 原图留存：创建文档前写入 asset 存储（`KRATOS_KNOWLEDGE_ASSET_DIR` env > `./data/knowledge_assets/{docID}.{ext}`），失败仅降级跳过血缘不阻塞入库。

### 5.3 Embedder（internal/knowledge/embedder.go）

```go
// Embedder 是向量化接口，MultiProviderEmbedder 为默认实现。
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// EmbedderAdmin 扩展运行时配置能力。
type EmbedderAdmin interface {
    Embedder
    UpdateConfig(provider, baseURL, apiKey, model string, dim int) error
}

type MultiProviderEmbedder struct {
    Provider string    // openai | ollama | gemini | huggingface
    BaseURL  string
    APIKey   string
    Model    string    // 默认 "text-embedding-3-small"
    Dim      int       // 默认 1536
    // ... lg loggateway.Logger
}

func NewMultiProviderEmbedder(provider, baseURL, apiKey, model string, dim int, lg loggateway.Logger) *MultiProviderEmbedder
func (e *MultiProviderEmbedder) Embed(ctx context.Context, text string) ([]float32, error)
func (e *MultiProviderEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
func (e *MultiProviderEmbedder) EmbedWithTaskType(ctx context.Context, text string, taskType string) ([]float32, error)
func (e *MultiProviderEmbedder) EmbedBatchWithTaskType(ctx context.Context, texts []string, taskType string) ([][]float32, error)
```

**设计决策**：
- `Embedder` 为接口，`MultiProviderEmbedder` 为多 Provider 统一实现（KB-06 解耦）。
- `openai`：`POST /v1/embeddings`，`EmbedBatch` 单次最多 32 条 input（`defaultEmbedBatchSize`）。
- `ollama`：`POST /api/embeddings`，逐条调用。
- `gemini`：`google.golang.org/genai` `EmbedContent`，批量 contents。
- `huggingface`：TEI `POST /embed`，`inputs` 数组批量。
- `TaskTypeEmbedder` 接口扩展 `EmbedWithTaskType`/`EmbedBatchWithTaskType`，Gemini 用 `RETRIEVAL_QUERY` task type 分离入库/查询（KB-10）。
- Wire 工厂 `NewKnowledgeEmbedder(c, SystemSettingRepo, lg)`：env → DB → provider 默认 key（EP-KN-01）。
- `PersistKnowledgeEmbed` / `UpdateEmbedderConfig` 写回 `system_settings`；patch 合并逻辑在 `internal/biz/knowledge/knowledge.go` 的 `ApplyEmbedPatch` 函数中（`internal/biz/knowledge.go` 通过类型别名 `ApplyKnowledgeEmbedPatch = knowledge.ApplyEmbedPatch` 转发）。
- Embedder 超时可通过 `KRATOS_KNOWLEDGE_EMBED_TIMEOUT_SEC` 环境变量配置（默认 60s，KB-11）。

**Embedder 配置优先级**（EP-KN-01，高 → 低）：

| 来源 | 说明 |
|------|------|
| 环境变量 | `KRATOS_KNOWLEDGE_EMBED_PROVIDER` / `_BASE_URL` / `_API_KEY` / `_MODEL` / `_DIM` |
| 系统设置 DB | `system_settings.knowledge_embed_*`；`GET/PUT /v1/system-settings` 字段 `knowledge_embed` |
| Knowledge Admin API | `GET/PUT /v1/knowledge/embedder-config`（运行时 + 写回 DB） |
| 前端 | `KnowledgeEmbedderPanel.vue`；系统设置页可写 `knowledge_embed` |

| Provider | 典型 model | base_url / key |
|----------|------------|----------------|
| `openai` | `text-embedding-3-small` | `OPENAI_API_KEY` |
| `ollama` | `nomic-embed-text` | 默认 `http://localhost:11434` |
| `gemini` | `gemini-embedding-001` | `GOOGLE_API_KEY` 或 DB `knowledge_embed_api_key` |
| `huggingface` | — | TEI `http://localhost:8080`（`knowledge_embed_base_url`） |

### 5.4 Retriever（internal/knowledge/retriever.go）

```go
type TaskTypeEmbedder interface {
    QueryEmbedder
    EmbedWithTaskType(ctx context.Context, text string, taskType string) ([]float32, error)
}

type Retriever struct {
    embedder QueryEmbedder
    repo     biz.KnowledgeRepo
    reranker reranker.Reranker  // 可选，来自 NewRerankerFromEnv
    lg       loggateway.Logger
}

func NewRetriever(embedder QueryEmbedder, repo biz.KnowledgeRepo, rr reranker.Reranker, lg loggateway.Logger) *Retriever
func (r *Retriever) Search(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error)
```

**设计决策**：
- Retriever 封装「嵌入查询 → 向量搜索 → 可选 Rerank」三步。
- `embedQuery` 私有方法：优先使用 `TaskTypeEmbedder`（Gemini 用 `RETRIEVAL_QUERY` task type），否则走标准 `Embed`。
- Rerank 失败时 FlowLog 警告并回退向量排序（`knowledge.rerank.fallback`）。
- 通过 `knowledgetool.WithRetriever(ctx, retriever)` 注入到工具上下文。
- **无语义层集合前置降级（2026-08-10 根修，§V5 降级矩阵 #3 落地）**：embed 之前经 `collectionLacksSemanticLayer`（search_helpers.go）判定目标集合 `embedding_model` 为空时直接走 BM25 词法检索（`knowledge.retriever.sparse_fallback` Warn 日志）——此类集合 chunks 无向量，dense 路径恒空且静默无感知；判定前置避免浪费一次查询 embed。embedder 为 nil 或 embed 失败（CodeUnavailable）同样降级 BM25。

### 5.5 QueryRewriter（internal/knowledge/query_rewriter.go）

```go
type RewriteStrategy string

const (
    RewriteNone          RewriteStrategy = ""
    RewriteHyDE          RewriteStrategy = "hyde"
    RewriteDecomposition RewriteStrategy = "decomposition"
    RewriteMultiQuery    RewriteStrategy = "multi_query"
)

type QueryRewriteResult struct {
    Queries []string
    Used    RewriteStrategy
}

type QueryRewriter struct {
    llm     biz.LLMCaller
    sys     *biz.SystemSettingUsecase
    catalog *biz.LlmProviderModelUsecase
    lg      loggateway.Logger
}

func NewQueryRewriter(llm biz.LLMCaller, sys *biz.SystemSettingUsecase, catalog *biz.LlmProviderModelUsecase, lg loggateway.Logger) *QueryRewriter
func (r *QueryRewriter) Rewrite(ctx context.Context, query string, strategy RewriteStrategy) (*QueryRewriteResult, error)
```

**设计决策**：
- 三种重写策略：HyDE（假设性回答）、Decomposition（查询分解）、MultiQuery（多查询变体）。
- LLM 不可用时自动降级为透传原始查询。
- 重写超时 15s，失败时 FlowLog 警告并回退。
- LLM 模型通过 `llm_resolver.go` 的 `ResolveLLM` 统一解析。
- HyDE：原始查询 + 假设性回答同时检索，提升语义召回。
- Decomposition：复杂查询分解为 2-4 个子问题，分别检索后合并。
- MultiQuery：生成 3 个不同角度的改写版本，合并检索结果。

### 5.6 HybridRetriever（internal/knowledge/hybrid_retriever.go）

```go
type HybridSearchMode string

const (
    HybridAuto   HybridSearchMode = "auto"
    HybridDense  HybridSearchMode = "dense"
    HybridSparse HybridSearchMode = "sparse"
    HybridRRF    HybridSearchMode = "rrf"
)

type HybridRetriever struct {
    embedder QueryEmbedder
    dense    biz.KnowledgeRepo
    sparse   SparseSearcher
    reranker rerankerForHybrid
    rrfK     int
    lg       loggateway.Logger
}

func NewHybridRetriever(retriever *Retriever, sparse SparseSearcher, lg loggateway.Logger) *HybridRetriever
func (h *HybridRetriever) Search(ctx context.Context, q biz.KnowledgeSearchQuery, mode HybridSearchMode) ([]biz.KnowledgeChunk, error)
```

**设计决策**：
- 四种检索模式：auto（自适应）、dense（纯向量）、sparse（纯 BM25）、rrf（混合融合）。
- RRF（Reciprocal Rank Fusion）融合 Dense 和 Sparse 结果，K=60。
- Sparse 检索使用 PostgreSQL `ts_vector` + GIN 索引（`SearchChunksBM25`）。
- 无 Sparse 配置时 auto 降级为 dense。
- Dense 或 Sparse 单路失败时自动回退到另一路。
- RRF overfetch = topK×3（上限 50），保证融合后有足够候选。
- **无语义层集合前置降级（2026-08-10 根修）**：与 Retriever 同一判定（`collectionLacksSemanticLayer`），在 mode 选择之前执行——无语义层集合 dense/RRF 密集侧恒空，直接降级 BM25（`knowledge.hybrid.sparse_fallback` Warn 日志）。

### 5.7 AdaptiveRouter（internal/knowledge/adaptive_router.go）

```go
type QueryComplexity int

const (
    QuerySimple    QueryComplexity = iota
    QueryModerate
    QueryComplex
)

type AdaptiveRouter struct {
    hybrid   *HybridRetriever
    rewriter *QueryRewriter
}

func NewAdaptiveRouter(hybrid *HybridRetriever, rewriter *QueryRewriter) *AdaptiveRouter
func (a *AdaptiveRouter) Search(ctx context.Context, q biz.KnowledgeSearchQuery, rewriteResult *QueryRewriteResult, modeOverride HybridSearchMode) ([]biz.KnowledgeChunk, error)
```

**设计决策**：
- 查询复杂度分类基于启发式规则：词数、问号数、连接词、Decomposition 标记、TopK 大小。
- modeOverride 非空且非 auto 时跳过分类，使用用户指定模式。
- MultiQuery 结果走 `searchMultiQuery`：每个子查询独立检索，结果按分数去重合并。
- 简单查询 → Dense（低延迟），中等查询 → RRF，复杂查询 → RRF。
- 子查询检索失败时 FlowLog 警告并跳过，不阻塞整体结果。

### 5.8 RetrievalEvaluator（internal/knowledge/retrieval_evaluator.go）

```go
type RetrievalAssessment struct {
    Sufficient      bool
    Confidence      float32
    SupplementQuery string
}

type RetrievalEvaluator struct {
    llm     biz.LLMCaller
    sys     *biz.SystemSettingUsecase
    catalog *biz.LlmProviderModelUsecase
    lg      loggateway.Logger
}

func NewRetrievalEvaluator(llm biz.LLMCaller, sys *biz.SystemSettingUsecase, catalog *biz.LlmProviderModelUsecase, lg loggateway.Logger) *RetrievalEvaluator
func (e *RetrievalEvaluator) Evaluate(ctx context.Context, query string, chunks []biz.KnowledgeChunk) (*RetrievalAssessment, error)
```

**设计决策**：
- CRAG（Corrective RAG）思路：检索后评估质量，不足时生成补充查询。
- 评估超时 10s，LLM 不可用时降级为 `Sufficient=true, Confidence=0.5`。
- 评估维度：sufficient（是否充分）、confidence（置信度 0-1）、supplement_query（补充查询）。
- LLM 返回 JSON 解析容错：`parseJSONLoose` 提取第一个 `{...}` 块。
- Chunks 摘要截断 2000 字符，单片段截断 200 字符。

### 5.9 SearchHelpers（internal/knowledge/search_helpers.go）

```go
type ChunkSearcher interface {
    Search(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error)
}

type ChunkAssessor interface {
    Evaluate(ctx context.Context, query string, chunks []biz.KnowledgeChunk) (*RetrievalAssessment, error)
}

func SearchWithEvaluation(ctx context.Context, searcher ChunkSearcher, assessor ChunkAssessor, query string, q biz.KnowledgeSearchQuery, chunks []biz.KnowledgeChunk) ([]biz.KnowledgeChunk, error)
```

**设计决策**：
- `SearchWithEvaluation` 封装「评估 → 不充分则补充检索 → 合并」流程。
- 评估失败或结果已充分时直接返回原始 chunks。
- 补充检索结果通过 `MergeSearchResults` 去重合并。
- Service 层 Search 方法直接调用此函数，将 CRAG 逻辑与传输层解耦。

### 5.10 FederatedRetriever（internal/knowledge/federated_retriever.go）

```go
type FederationStrategy int

const (
    FederationBroadcast FederationStrategy = iota
    FederationRoute
)

type CollectionMetaFetcher interface {
    ListCollections(ctx context.Context, workspace string, limit, offset int) ([]biz.KnowledgeCollection, int, error)
}

type FederatedSearchOptions struct {
    Strategy       FederationStrategy
    RouteTopN      int
    RouteMinScore  float32
}

type FederatedRetriever struct {
    router    *AdaptiveRouter
    retriever *Retriever
    meta      CollectionMetaFetcher
    lg        loggateway.Logger
}

func NewFederatedRetriever(router *AdaptiveRouter, retriever *Retriever, lg loggateway.Logger) *FederatedRetriever
func NewFederatedRetrieverWithMeta(router *AdaptiveRouter, retriever *Retriever, meta CollectionMetaFetcher, lg loggateway.Logger) *FederatedRetriever
func (f *FederatedRetriever) Search(ctx context.Context, collectionIDs []string, q biz.KnowledgeSearchQuery, rewriteResult *QueryRewriteResult, modeOverride HybridSearchMode) ([]biz.KnowledgeChunk, error)
func (f *FederatedRetriever) SearchWithOptions(ctx context.Context, collectionIDs []string, q biz.KnowledgeSearchQuery, rewriteResult *QueryRewriteResult, modeOverride HybridSearchMode, opts FederatedSearchOptions) ([]biz.KnowledgeChunk, error)
```

**设计决策**：
- 两种联邦策略：Broadcast（默认，向所有 Collection 并行广播）和 Route（基于相关性评分筛选 TopN Collection）。
- `CollectionMetaFetcher` 接口由 `biz.KnowledgeUsecase` 实现，提供 Collection 元数据。
- Route 策略：`collectionRelevanceScore` 基于 Collection 名称/描述与查询词的匹配度评分，按评分排序取 TopN（默认 3），最低分数阈值（默认 0.3）。
- 路由失败时自动降级为 Broadcast。
- `Search` 方法默认 Broadcast，`SearchWithOptions` 支持指定策略。
- 单 Collection 时自动降级为 AdaptiveRouter 或 Retriever 直接搜索。
- 多 Collection 并行：使用 `safego.Go` + `sync.WaitGroup`，部分集合失败时 FlowLog 警告，返回成功集合的结果。

### 5.11 Ingest 流水线（internal/knowledge/ingest.go）

```go
func BuildIndexedChunks(ctx context.Context, embedder QueryEmbedder, p IngestParams) ([]biz.KnowledgeChunk, error)
```

**设计决策**：
- Service：`ExtractDocumentText` → 异步 `BuildIndexedChunks` → Event Bus。
- `IngestParams.Strategy` 驱动 `SplitWithStrategy`；`BatchEmbedder.EmbedBatch` 批量向量化。
- `IngestParams.ApplyDefaults()` 下移默认值逻辑（ChunkSize=512, ChunkOverlap=64），Service 层不再硬编码。
- `BuildIndexedChunks` 使用 `QueryEmbedder` 接口（而非 `Embedder` 具体类型），支持 `BatchEmbedder` 优化路径。

### 5.12 Reranker 工厂（internal/knowledge/reranker_factory.go）

环境变量 `KRATOS_KNOWLEDGE_RERANKER`：`off` | `topk` | `cohere` | `infinity`。
Wire 经 `NewKnowledgeRetriever` 装配；配置错误时 SysLog 警告并禁用 rerank。

**Memory 召回桥接（AH-04）**：`NewMemoryReranker`（`memory_rerank_factory.go`）按 `KRATOS_MEMORY_RERANKER=cohere|infinity` 包装同一工厂为 `biz.Reranker`（`KnowledgeRerankerAdapter`）。Wire 注入 `data.NewData`；data 只持有接口、不 import trpc knowledge/reranker。默认仍为 `biz.CrossEncoderReranker`（bigram Jaccard）。出错回退保持 bigram-only Jaccard（与 Knowledge 检索 rerank 降级路径独立）。

---

## 六、Agent 集成

### 6.1 knowledge_search 工具（internal/tools/knowledge/tool.go）

```go
// US-14：CollectionID 可选（无 jsonschema required）。留空 = 自动路由。
type searchInput struct {
    CollectionID string  `json:"collection_id"`
    Query        string  `json:"query"`
    TopK         int     `json:"top_k,omitempty"`
    MinScore     float32 `json:"min_score,omitempty"`
    FilterJSON   string  `json:"filter_json,omitempty"`
    UseRerank    *bool   `json:"use_rerank,omitempty"`
}

type searchOutput struct {
    Chunks []chunkSummary `json:"chunks"`
}

type chunkSummary struct {
    ID      string  `json:"id"`
    Content string  `json:"content"`
    Score   float32 `json:"score"`
    DocID   string  `json:"doc_id"`
}
```

**设计决策**：
- 工具声明名 `knowledge_search`，与 trpc-agent-go 框架一致。
- Retriever/AdaptiveRouter 通过 context 传递（`WithRetriever` / `WithAdaptiveRouter`），避免全局状态。
- 返回精简的 `chunkSummary`（不含 embedding 向量），减少 Token 消耗。
- 优先使用 AdaptiveRouter（混合检索 + 自适应路由），不可用时降级为 Retriever。
- **US-14（2026-07-21）**：`collection_id` 改可选，消除「LLM 不知有哪些库却被迫填 UUID」的死局。留空时按序解析：
  1. scoped == 1 → 直用该库（现状不变）；
  2. scoped > 1 → FederatedRetriever Route 策略在 scoped 内智能路由（不再报 "multiple knowledge_bases are scoped" 错误）；
  3. scoped == 0 → 列出全部 Collection 后 FederatedRetriever Route 全库路由；系统无任何 Collection 时返回空结果（chunks=[]），不报错。
- 显式传 collection_id 时仍校验必须在 scoped 内（越权防护不变）。

### 6.1b knowledge_reflect 工具（internal/tools/knowledge/tool.go）

```go
type reflectInput struct {
    // US-14：可选（jsonschema 不再 required）。留空 = scoped 内路由；无 scoped 时全库智能路由。
    CollectionIDs []string `json:"collection_ids"`
    Query         string   `json:"query" jsonschema:"description=The original user query to reflect on,required"`
    TopK          int      `json:"top_k,omitempty" jsonschema:"description=Maximum number of results to return per collection"`
}

type reflectOutput struct {
    Sufficient       bool           `json:"sufficient"`
    Confidence       float32        `json:"confidence"`
    SupplementQuery  string         `json:"supplement_query,omitempty"`
    Chunks           []chunkSummary `json:"chunks"`
}
```

**设计决策**：
- 工具声明名 `knowledge_reflect`，让 Agent 主动评估检索质量。
- 接收 `collection_ids`（复数），支持跨 Collection 搜索。
- 优先使用 FederatedRetriever（多 Collection 并行），不可用时降级为 AdaptiveRouter/Retriever（仅单 Collection）。
- 当 RetrievalEvaluator 可用时，自动评估检索质量并返回 `sufficient`/`confidence`/`supplement_query`。
- 评估失败时 FlowLog 警告，降级为 `sufficient=true, confidence=1.0`。
- Collection 权限校验：`WithKnowledgeCollections` context 限定可访问的集合。
- **US-14（2026-07-21）**：`collection_ids` 改可选。留空时：scoped 非空 → 在 scoped 内联邦路由；scoped 为空 → 全库智能路由（不再报 "collection_ids is required"）。

### 6.2 Agent 装配链

在 `buildToolsetsForAgent` 中：

```go
cfg.KnowledgeSearch = eff[biz.ToolKeyKnowledgeSearch]   // "knowledge_search"
cfg.KnowledgeReflect = eff[biz.ToolKeyKnowledgeReflect]  // "knowledge_reflect"
```

当 Agent 工具配置中启用对应开关时，`Assemble` 会将工具加入 `customTools`。

### 6.3 工具开关

| 工具键 | 常量 | 说明 |
|--------|------|------|
| `knowledge_search` | `ToolKeyKnowledgeSearch` | Agent 搜索知识库 |
| `knowledge_reflect` | `ToolKeyKnowledgeReflect` | Agent 评估检索质量 + 跨 Collection 搜索 |

两个工具均通过 effective-tools 机制控制是否装配，均属于 `sessionBoundToolKeys`。

### 6.4 Context 注入链

| Context Key | 注入位置 | 说明 |
|-------------|----------|------|
| `contextKey{}` | `chat_orchestrator_turn.go` | Retriever |
| `routerKey{}` | `chat_orchestrator_turn.go` | AdaptiveRouter |
| `federatedKey{}` | `chat_orchestrator_turn.go` | FederatedRetriever |
| `evaluatorKey{}` | `chat_orchestrator_turn.go` | RetrievalEvaluator |
| `collectionsKey{}` | `chat_orchestrator_turn.go` | 可访问 Collection IDs |

Team Runner 同样注入以上 context（`runner_team_trpc.go`）。

### 6.5 Plan-Then-Retrieve（internal/agent/knowledge_inject.go）

```go
func newKnowledgeCueBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback
func buildKnowledgeCue(ctx context.Context, uc *biz.KnowledgeUsecase) string
```

**设计决策**：
- BeforeModel 钩子（优先级 6），在每次模型调用前注入 Collection 摘要到系统提示。
- 仅注入 Agent 关联的 Collection（通过 `KnowledgeCollectionsFromContext` 读取 context 中的 scoped IDs），避免泄露其他 Collection。
- 摘要内容：Collection 名称、ID、描述（≤120 字符）、文档数、块数 + 搜索策略提示。
- 截断保护：总摘要 ≤1500 字符，最多 10 个 Collection。
- KnowledgeUsecase 为 nil 或无 Collection 时自动跳过。
- 列表失败时 FlowLog 警告，不阻塞模型调用。
- 注册位置：`callback_chain.go` 的 `productCallbackChain` 中。

---

## 七、Service 层

### 7.1 KnowledgeService（internal/service/knowledge.go）

```go
type KnowledgeSearchDeps struct {
    Retriever *knowledge.Retriever
    Router    *knowledge.AdaptiveRouter
    Evaluator *knowledge.RetrievalEvaluator
}

type KnowledgeService struct {
    v1.UnimplementedKnowledgeServiceServer
    uc            *biz.KnowledgeUsecase
    embedder      *knowledge.Embedder
    search        KnowledgeSearchDeps
    extractors    *knowledge.ExtractorRegistry    // 统一摄取管线：模态路由
    organizer     *knowledge.MarkdownOrganizer    // LLM 整理为 MD（nil 时跳过整理）
    bus           event.Bus
    systemSetting biz.SystemSettingRepo
    lg            loggateway.Logger
}
```

**关键设计**：

| 方法 | 说明 |
|------|------|
| `CreateCollection` | 参数校验 → `uc.CreateCollection` |
| `IngestDocument` | base64 解码 → 守卫（OOXML 二次判定）→ ExtractorRegistry.Extract → 可选 MarkdownOrganizer.Organize → 创建文档（含 content_text）→ `safego.Go` → `BuildIndexedChunks` → 发布 `knowledge_ingest` 事件 |
| `GetDocumentContent` | 读取 content_text/organized 供前端预览 |
| `Search` | 查询重写 → AdaptiveRouter/Retriever 检索 → RetrievalEvaluator 评估 → Prometheus 计时 |
| `GetEmbedderConfig` / `UpdateEmbedderConfig` | 脱敏读取 / 运行时更新 Embedder（EP-KN-01） |
| `DeleteCollection` | 级联删除（数据库 CASCADE） |
| `DeleteDocument` | 级联删除（数据库 CASCADE） |

**Search 方法流程**：

```
Search(req)
  ├── router != nil ?
  │   ├── rewrite_strategy != none ? → QueryRewriter.Rewrite()
  │   ├── hybrid_search → ParseHybridSearchMode → modeOverride
  │   └── AdaptiveRouter.Search(q, rewriteResult, modeOverride)
  │       ├── classify(q) → QueryComplexity → selectMode
  │       └── HybridRetriever.Search(q, mode)
  │           ├── Dense: embedder.Embed → repo.SearchChunks
  │           ├── Sparse: sparse.SearchChunksBM25
  │           └── RRF: rrfMerge(dense, sparse)
  ├── router == nil → Retriever.Search(q)
  └── SearchWithEvaluation(retriever, evaluator, query, q, chunks)
      ├── evaluator.Evaluate → RetrievalAssessment
      ├── !sufficient && supplementQuery != "" → retriever.Search(supplementQ)
      └── MergeSearchResults(chunks, supplementChunks, topK)
```

### 7.2 异步摄取流程

> **2026-07-20 升级**：统一摄取管线（模态无关主干）。Extract 经 ExtractorRegistry 路由，文本类与多模态归一为 Markdown 后共用下游。
> **2026-07-21 校准**：图片提取从同步前移到摄取 goroutine 内（视觉 LLM 最长 60s，同步会阻塞 HTTP），详见 §5.2c「异步提取流程」。

```
IngestDocument(req)
  ├── base64.Decode → 大小/MIME 守卫（OOXML 二次判定）
  ├── NormalizeMetadataJSON（合并 modality/extractor 标记）
  ├── 文本类：ExtractorRegistry.Extract（同步，本地解析快，失败即 400 不落孤儿文档）
  │   └── organize_to_markdown != false ?
  │       └── MarkdownOrganizer.Organize() → md + organized   ← 失败降级原文本（NFR-11）
  ├── 图片：跳过同步提取；AssetStore.Save 原图留存（asset_uri 血缘，失败仅降级跳过）
  ├── uc.CreateDocument(status=pending) + content_text/organized 持久化
  └── safego.Go → [图片：VisionExtractor.Extract → UpdateDocumentContent 回写；失败 status=error（NFR-12）]
                → BuildIndexedChunks(strategy=markdown, EmbedBatch) → InsertChunks
```

**设计决策**：
- 整理成功（`organized=true`）时强制 `ChunkByMarkdown` 分块（按标题层级），检索质量最优；降级时沿用请求策略。
- `content_text` 在 `CreateDocument` 时同步写入（摄取失败也保留提取结果，便于诊断）；`GetDocumentContent` RPC 供前端预览。
- 图片 `content_text` 初始为空，视觉提取成功后经 `UpdateDocumentContent` 回写（§5.2c）；提取失败置 `status=error` 不回写。
- OOXML 守卫：`http.DetectContentType` 对 DOCX/XLSX/PPTX 返回 `application/zip`，白名单命中 `application/zip` 时按请求 `mime_type`/扩展名二次判定，避免 Office 文件被误拒。

**错误处理**：任何步骤失败 → `UpdateDocumentStatus(error, errMsg)` → goroutine 退出。

### 7.3 Wire 注入

```go
// internal/service/wire_providers.go — Chunker 默认 512/64 char
// internal/service/knowledge_embedder.go — NewKnowledgeEmbedder(c *conf.Data, sys, lg)
// internal/service/knowledge_retriever.go — NewKnowledgeRetriever(emb, repo, lg)
// internal/service/knowledge_advanced.go — Advanced RAG 组件工厂
//   - NewKnowledgeHybridRetriever(retriever, sparse, lg)
//   - NewKnowledgeQueryRewriter(llm, sys, catalog, lg)
//   - NewKnowledgeAdaptiveRouter(hybrid, rewriter, lg)
//   - NewKnowledgeRetrievalEvaluator(llm, sys, catalog, lg)
//   - NewKnowledgeFederatedRetriever(router, retriever, uc, lg)
//   - ProvideKnowledgeSearchDeps(retriever, router, evaluator) → KnowledgeSearchDeps
//   - NewKnowledgeMarkdownOrganizer(llm, sys, catalog, lg) → MarkdownOrganizer（LLM 不可用时返回 nil）
//   - NewKnowledgeExtractorRegistry(llm, sys, catalog, lg) → ExtractorRegistry（VisionExtractor 优先 + TextExtractor）
//   - NewKnowledgeAssetStore(lg) → AssetStore（KRATOS_KNOWLEDGE_ASSET_DIR env > ./data/knowledge_assets）
```

**Wire 依赖链**：

```
Embedder + Repo → Retriever → HybridRetriever → AdaptiveRouter → FederatedRetriever
                              ↑                    ↑
                         SparseSearcher        QueryRewriter
                                              RetrievalEvaluator
```

### 7.4 免选择知识库（US-14，2026-07-21）

> 核心理念：**存储可分类，使用免选**。Collection 是收纳分类工具（文件夹），不是使用门槛。

**四条规则**：

| # | 规则 | 实现位置 |
|---|------|---------|
| 1 | 上传免预选：`collection_id` 留空 → `EnsureDefaultCollection` 懒创建「默认知识库」后落入；前端不再静默丢弃文件 | Service `IngestDocument` |
| 2 | 检索免选择：Search API / 工具 collection 留空 → 全库智能路由 | Service `Search`、`tools/knowledge` |
| 3 | 智能路由策略：Route（名称/描述与 query 匹配度取 top N=3，阈值 0.3）→ 路由失败/无匹配降级 Broadcast；复用 `FederatedRetriever.SearchWithOptions`（§5.10） | FederatedRetriever |
| 4 | 文档可归档：MoveDocument 跨库移动（默认库收件箱 → 分类库），chunks 随迁 + 计数校正 | `MoveDocument` RPC |

**全库路由解析顺序**（工具与 Search API 共用）：

```
collection 留空
  ├── scoped（Agent 绑定 knowledge_bases）== 1 → 单库直搜（现状）
  ├── scoped > 1 → FederatedRetriever Route（scoped 内）
  └── scoped == 0 → ListCollections 全量 → FederatedRetriever Route（全库）
        ├── 无 Collection → 返回空结果（不报错）
        ├── Route 无匹配（全部 < 阈值）→ 降级 Broadcast 全库并行
        └── 部分库失败 → FlowLog 警告，返回成功库结果（§5.10 现状）
```

**关键设计决策**：

- **默认知识库懒创建**：首个免选上传时创建（`name="默认知识库"`，`embedding_model`/`dim` 取当前 Embedder 配置）；按 name 查找复用，不引入 is_default 标记列（避免 Schema 变更 + 多默认库歧义）。
- **MoveDocument dim 校验**：目标库 `dim` 与源库不一致时拒绝移动（`CodeConflict`）——pgvector 列维度固定，跨 dim 移动会导致向量不可检索；用户需删除后重新入库。同 dim 移动保留原向量，无需重 embedding。
- **MoveDocument 事务**：单事务内 `UPDATE knowledge_documents.collection_id` + `UPDATE knowledge_chunks.collection_id` + 源库计数 `-1/-chunkCount` + 目标库计数 `+1/+chunkCount`，失败整体回滚。
- **工具零库行为**：系统无任何 Collection 时工具返回 `chunks=[]` 空结果而非错误——LLM 可继续无知识回答，不阻塞会话。
- **scoped 语义不变**：Agent 绑定 knowledge_bases 仍作为范围限定（越权校验保留）；未绑定 = 全库可搜，这是 US-14 的默认路径。
- **兼容性**：proto 仅去除 REQUIRED 标注（field number 不变），已绑定 Agent 与显式传 collection_id 的调用行为完全不变。

**前端配套**：

- 上传：未选中 Collection 时照常上传（不传 collection_id），队列项标注「默认知识库」；上传完成后自动选中该库刷新列表。
- 搜索面板：Collection 下拉首项「全部知识库」（值为空），默认选中。
- 文档列表：行内「移动到…」菜单 → 对话框选目标库（过滤当前库 + dim 不兼容库禁用并提示）→ MoveDocument。
- Agent 编辑器：knowledge_bases 绑定从基础配置折叠到「高级配置」分区，默认空（全库可搜）。

---

## 八、前端集成

### 8.1 API 层（web/src/features/knowledge/api.ts）

| 函数 | 说明 |
|------|------|
| `listCollections` / `getCollection` / `createCollection` / `deleteCollection` | 集合 CRUD |
| `listDocuments` / `ingestDocument` / `deleteDocument` | 文档 CRUD（`ingestDocument` 支持 `organize_to_markdown` 参数） |
| `getDocumentContent` | 预览整理后 MD 全文（content_text） |
| `searchKnowledge` | 语义搜索 |
| `getEmbedderConfig` / `updateEmbedderConfig` | Embedder 管理 |

### 8.2 Store 与页面

页面结构：`KnowledgePage.vue` 为页面级三 Tab 布局（**文档 | 检索 | 设置**）。文档 Tab 左侧集合列表、右侧操作区自上而下为拖拽上传区（常驻）→ 上传队列 → 集合详情卡 + 文档面板；检索 Tab 为调试搜索面板（高级控件附行内说明）；Embedder 配置收纳于设置 Tab（低频操作，与系统设置页共享配置源，面板内附跳转链接）。文档入库对话框仅保留「粘贴文本」模式，文件上传统一走拖拽区（双入口能力重叠已消除）。

| 路径 | 说明 |
|------|------|
| `web/src/stores/knowledge/index.ts` | Pinia Store |
| `web/src/pages/KnowledgePage.vue` | 管理页（路由 `/knowledge`，三 Tab：文档 / 检索 / 设置） |
| `web/src/components/knowledge/*` | 集合列表、文档、检索、Embedder、入库对话框（粘贴文本） |
| `web/src/components/knowledge/KnowledgeDropZone.vue` | 拖拽上传区（文件统一入口：多文件批量、自动推断 source/mime、默认整理为 MD） |
| `web/src/components/knowledge/KnowledgeUploadQueue.vue` | 上传队列（逐文件状态卡片，复用 WS 事件刷新） |
| `web/src/components/knowledge/KnowledgeDocPreviewDialog.vue` | MD 全文预览对话框 |
| `web/src/features/knowledge/useKnowledgeIngestWs.ts` | WS 入库进度（EP-KN-02） |

### 8.3 摄取进度 WS 事件（EP-KN-02）

异步摄取经 Event Bus 发布 `knowledge_ingest` 信封（`EnvelopeTypeKnowledgeIngest`），前端 `useKnowledgeIngestWs` 订阅 `/v1/ws` 频道 `knowledge` 并刷新文档列表。

### 8.4 Reranker 环境变量（KN-01）

| 环境变量 | 说明 |
|----------|------|
| `KRATOS_KNOWLEDGE_RERANKER` | `off` \| `topk` \| `cohere` \| `infinity` |
| `KRATOS_KNOWLEDGE_RERANK_TOP_K` | 重排后保留条数（topk 模式） |
| `COHERE_*` / `INFINITY_*` | 第三方 Rerank 端点与密钥 |

Search RPC 可选 `use_rerank`、`rerank_candidates` 覆盖单次请求行为。

### 8.5 Prometheus 指标

| 指标 | 类型 | 说明 |
|------|------|------|
| `aranea_knowledge_ingest_documents_total` | Counter | 成功索引的文档数 |
| `aranea_knowledge_search_duration_seconds` | Histogram | 搜索延迟 |

### 8.6 降级与限制

- 需要 pgvector；当 Postgres 未配置时 Repo 为 nil，API 返回 `ErrKnowledgeUnavailable`。
- 嵌入维度每个集合固定；更改需重建集合。
- 文档内容必须可文本解码；图片/PDF 需 OCR 提取（当前 OCR 为 stub，`KNOWLEDGE_OCR` 环境变量配置）。
- 文档级 `metadata_json` 写入每个 Chunk 的 JSONB 列，供 `filter_json` 检索过滤。
- 查询重写和检索评估依赖 LLM 调用，无可用 LLM 时自动降级（透传原始查询 / 跳过评估）。
- 联邦搜索支持 Broadcast 和 Route 两种策略，Route 策略基于 Collection 名称/描述相关性评分。
- Plan-Then-Retrieve 通过 BeforeModel 钩子注入 Collection 摘要，高频场景下可能增加延迟。

---

## 九、待实现设计

以下为对标 trpc-agent-go `knowledge` 包但尚未实现的能力，列出设计方向供后续迭代参考。

> 实现状态与任务追踪详见 [37-knowledge.development.md §子模块：Knowledge Evolution Roadmap](./37-knowledge.development.md#子模块knowledge-evolution-roadmap)。

### 9.1 KnowledgeBaseFactory

```go
type KnowledgeBaseFactory interface {
    BuildKnowledge(ctx context.Context, kb *KnowledgeBase) (knowledge.Knowledge, error)
}
```

Factory 负责根据配置构建 `knowledge.Knowledge` 实例：创建 Embedder、VectorStore、Reranker、BuiltinKnowledge。

### 9.2 DocumentPipeline

完整 RAG 流水线：Extractor(格式转换) → Reader(解析) → Chunking(分块) → Embedder(向量化) → VectorStore(存储)。

当前实现跳过 Extractor/Reader，直接对 base64 解码后的原始文本分块。

### 9.3 AgenticFilter

集成 trpc-agent-go `searchfilter` 包，LLM 根据查询动态生成 `UniversalFilterCondition`。

### 9.4 OCR / Extractor

> **2026-07-20 裁决**：OCR 路线由「多模态 LLM 视觉理解」取代 tesseract/docling 依赖。统一摄取管线分两阶段落地：
> - **Phase 1（文本类）**：Extractor 接口抽象（§5.2）+ TextExtractor + MarkdownOrganizer（§5.2b）+ 拖拽批量上传 + content_text 预览。
> - **Phase 2（多模态）**：VisionExtractor（§5.2c）+ 白名单放开 image/* + asset_uri 原图血缘。
> - 原 `ocr.go` stub 废弃；docling 仅留作 PDF 版面保真的后续独立增强（可选）。

### 9.5 多租户隔离

SearchFilter 增加 `tenant_id`，向量存储按租户分区，API 层强制注入。

### 9.6 GraphRAG — 知识图谱增强

> **2026-07-20 定位裁决（用户确认）**：GraphRAG 为 **Phase 3 可选增强**，工程上暂缓。
> **架构纪律：旁路非侵入**——图谱从已入库的 chunks 异步构建（实体/关系提取复用 LLMResolver），写入独立表，检索时以 `GraphAugmentedRetriever` 包裹现有 Router 外层。**绝不嵌入摄取主链路**，统一摄取管线（Phase 1/2）不依赖图谱的任何部分；Chunk metadata 已有的 `doc_id`/`collection_id` 即为图谱回链所需的全部预留。

> 目标：引入知识图谱层，支撑多跳推理和实体关系查询。

#### 9.6.1 知识图谱构建

在文档入库时增加实体和关系提取步骤：

```
文档入库管线（升级）：
  ExtractDocumentText → SplitWithStrategy → EmbedTexts
    + ExtractEntities → ExtractRelations → BuildKnowledgeGraph
```

```go
// internal/biz/knowledge/graph.go（新增）

type Entity struct {
    ID           string
    Name         string
    Type         string
    Properties   map[string]any
    CollectionID string
    DocID        string
}

type Relation struct {
    ID           string
    SourceID     string
    TargetID     string
    Type         string
    Properties   map[string]any
    CollectionID string
}

type GraphRepo interface {
    UpsertEntities(ctx context.Context, entities []Entity) error
    UpsertRelations(ctx context.Context, relations []Relation) error
    SearchSubgraph(ctx context.Context, query GraphQuery) (Subgraph, error)
    Traverse(ctx context.Context, startEntityID string, depth int) (Subgraph, error)
}
```

- **实体提取**：LLM-based NER（利用已有 Provider 集成）
- **关系提取**：LLM-based 关系三元组提取
- **存储**：PostgreSQL 关系表（`knowledge_entities`、`knowledge_relations`），未来可扩展 Neo4j
- **架构位置**：`internal/biz/knowledge/graph.go`（新增），`internal/data/knowledge_graph.go`（新增）

**数据库 Schema**：

```sql
CREATE TABLE IF NOT EXISTS knowledge_entities (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    type          TEXT NOT NULL DEFAULT '',
    properties    JSONB NOT NULL DEFAULT '{}',
    collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
    doc_id        TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS knowledge_relations (
    id            TEXT PRIMARY KEY,
    source_id     TEXT NOT NULL REFERENCES knowledge_entities(id) ON DELETE CASCADE,
    target_id     TEXT NOT NULL REFERENCES knowledge_entities(id) ON DELETE CASCADE,
    type          TEXT NOT NULL DEFAULT '',
    properties    JSONB NOT NULL DEFAULT '{}',
    collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ke_collection ON knowledge_entities(collection_id);
CREATE INDEX idx_ke_name_type  ON knowledge_entities(name, type);
CREATE INDEX idx_kr_source     ON knowledge_relations(source_id);
CREATE INDEX idx_kr_target     ON knowledge_relations(target_id);
CREATE INDEX idx_kr_type       ON knowledge_relations(type);
```

#### 9.6.2 图增强检索

向量检索 + 图遍历融合：

```go
// internal/knowledge/graph_augmented_retriever.go（新增）

type GraphAugmentedRetriever struct {
    vectorRetriever *Retriever
    graphRepo       biz.KnowledgeGraphRepo
}

func (r *GraphAugmentedRetriever) Search(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
    // 1. 向量检索获取初始 chunks
    chunks, _ := r.vectorRetriever.Search(ctx, q)
    // 2. 从 chunks 中提取实体
    entities := extractEntitiesFromChunks(chunks)
    // 3. 图遍历获取关联实体和文档
    subgraph := r.graphRepo.Traverse(ctx, entities, depth=2)
    // 4. 融合向量结果和图结果
    return mergeResults(chunks, subgraphChunks), nil
}
```

- 向量检索负责语义相似度
- 图遍历负责关系推理和多跳连接
- 融合策略：加权合并或 RRF

#### 9.6.3 图查询工具

```go
// internal/tools/knowledge/graph_tool.go（新增）

func NewGraphSearchTool() trpctool.CallableTool {
    // knowledge_graph_search: 搜索知识图谱中的实体和关系
    // 输入: collection_id, entity_name, relation_type, depth
    // 输出: entities[], relations[]
}

func NewGraphTraverseTool() trpctool.CallableTool {
    // knowledge_graph_traverse: 从指定实体出发遍历关系图
    // 输入: entity_id, depth, relation_type_filter
    // 输出: subgraph
}
```

**Proto 扩展**：

```protobuf
message GraphSearchRequest {
    string collection_id = 1;
    string entity_name = 2;
    string relation_type = 3;
    int32 depth = 4;
}

message GraphSearchResponse {
    repeated KnowledgeEntity entities = 1;
    repeated KnowledgeRelation relations = 2;
}
```

### 9.7 Skill Knowledge — 技能知识库

> 目标：从文档知识库演进为技能知识库，与 Aranea 的 Skill 体系深度融合。

#### 9.7.1 三层知识模型

| 层 | 类型 | 存储形式 | 检索方式 |
|----|------|----------|----------|
| L1 文档知识 | "知道什么" | Chunk + Embedding | 向量相似度（已实现） |
| L2 关系知识 | "谁关联谁" | 实体 + 关系（知识图谱） | 图遍历 + 子图检索（GraphRAG） |
| L3 技能知识 | "如何做" | 技能描述 + 执行轨迹 | 语义匹配 + 层次导航 |

#### 9.7.2 技能知识库构建

借鉴 SkillX 和 CORPUS2SKILL，构建三层技能层次：

```go
// internal/biz/knowledge/skill_knowledge.go（新增）

type SkillKnowledge struct {
    ID             string
    Name           string
    Description    string
    Level          SkillLevel
    ParentID       string
    CollectionID   string
    Procedure      string
    Tools          []string
    Preconditions  string
    Postconditions string
    Embedding      []float32
}

type SkillLevel int

const (
    SkillPlanning  SkillLevel = iota  // 高层任务规划
    SkillFunctional                    // 可复用功能子程序
    SkillAtomic                        // 原子操作模式
)
```

- **离线蒸馏**：从 Agent 执行轨迹中提取技能（与 Memory 的压缩机制协同）
- **层次导航**：Agent 获得技能目录鸟瞰图 → 逐级钻入 → 获取具体操作步骤
- **架构位置**：`internal/biz/knowledge/skill_knowledge.go`（新增），与 `internal/biz/skill` 协同

#### 9.7.3 知识导航工具

```go
// internal/tools/knowledge/navigate_tool.go（新增）

func NewKnowledgeNavigateTool() trpctool.CallableTool {
    // knowledge_navigate: 浏览知识库的层次结构
    // 输入: collection_id, path (可选，如 "/技术/后端/Go")
    // 输出: 当前层级的摘要 + 子主题列表
}

func NewKnowledgeDrillTool() trpctool.CallableTool {
    // knowledge_drill: 钻入特定知识分支
    // 输入: collection_id, topic_id
    // 输出: 更细粒度的摘要 + 文档列表
}
```

Agent 不再"盲目检索"，而是"有地图地导航"。

#### 9.7.4 技能蒸馏管线

从 Agent 执行轨迹中自动提取技能：

```
Agent 执行轨迹
  → 轨迹分析（LLM）
    → 提取 Planning Skills（高层任务组织）
    → 提取 Functional Skills（可复用功能子程序）
    → 提取 Atomic Skills（原子操作模式）
  → 技能去重 + 合并
  → 写入技能知识库
```

- 与 Memory 压缩机制协同：Memory 压缩后的轨迹作为技能蒸馏输入
- 与 Skill 体系协同：蒸馏出的技能可注册为 Agent 可用技能
- 架构位置：`internal/knowledge/skill_distiller.go`（新增）

---

## 十、涉及文件

> 完整文件清单（含实现状态标记）详见 [37-knowledge.development.md §1 代码锚点](./37-knowledge.development.md#1-模块定位) 和 [§附录 B：新增文件清单](./37-knowledge.development.md#附录-b新增文件清单)。

### 10.1 后端核心文件

| 文件 | 说明 |
|------|------|
| `api/kratos/knowledge/v1/knowledge.proto` | Proto 定义（含 `rewrite_strategy` + `hybrid_search` 字段） |
| `internal/biz/knowledge.go` | 类型别名转发（KnowledgeRepo = knowledge.Repo 等） |
| `internal/biz/knowledge/knowledge.go` | 领域模型 + Repo/Usecase 接口（子接口拆分）+ EmbedSetting patch 合并 |
| `internal/data/knowledge.go` | PostgreSQL + pgvector Repo + `SearchChunksBM25` |
| `internal/service/knowledge.go` | KnowledgeService（KnowledgeSearchDeps 聚合） |
| `internal/service/knowledge_advanced.go` | Advanced RAG 组件 Wire 工厂（6 个 Provider） |
| `internal/service/knowledge_embedder.go` | Embedder Wire + DB 回落（EP-KN-01） |
| `internal/service/knowledge_retriever.go` | Retriever Wire（KN-01） |
| `internal/knowledge/*.go` | 内部包：chunker/embedder/ingest/retriever/query_rewriter/hybrid_retriever/adaptive_router/retrieval_evaluator/federated_retriever/search_helpers/llm_resolver/ocr/html_text/chunk_strategy/document_extract/readers_import/reranker_factory/memory_rerank_adapter/memory_rerank_factory |
| `internal/tools/knowledge/tool.go` | knowledge_search + knowledge_reflect 工具 |
| `internal/agent/knowledge_inject.go` | Plan-Then-Retrieve BeforeModel 钩子 |
| `internal/agent/tool_assembly.go` | KnowledgeSearch/KnowledgeReflect 装配 |
| `api/kratos/system_setting/v1/system_setting.proto` | `KnowledgeEmbedSettings` |

### 10.2 前端文件

| 文件 | 说明 |
|------|------|
| `web/src/features/knowledge/api.ts` | 前端 API（含 rewrite_strategy/hybrid_search 参数） |
| `web/src/features/knowledge/useKnowledgeIngestWs.ts` | WS 入库进度（EP-KN-02） |
| `web/src/stores/knowledge/index.ts` | Pinia Store |
| `web/src/pages/KnowledgePage.vue` | 管理页（路由 `/knowledge`） |
| `web/src/components/knowledge/*` | 集合列表、文档、检索、Embedder、入库对话框 |

---

## 子模块：Vault 重设计

> **来源**：2026-07-25 评审**有条件通过**（[评审报告](../reports/2026-07-25-review-knowledge-vault-redesign.md)），方向与架构获批，R-1~R-6 必须修改项已作为实现契约合入本节（§V6）。
> **决策依据**：[无 Embedding 检索可行性调研](../reports/2026-07-25-research-embedding-free-retrieval.md)（三条无向量路线背书：PageIndex 推理导航 / Claude Code agentic 词法检索 / BM25+rerank 与最佳稠密模型打平）。
> **状态**：设计已批准，待实施。Phase 计划见 [37-knowledge.development.md §子模块 Vault 重设计 Phase 计划](./37-knowledge.development.md#子模块vault-重设计-phase-计划)。
> **说明**：原方案文档（research-knowledge-vault-redesign）未落盘，本节为 Vault 重设计的**权威落档**，取代该缺失文件。

### V1. 定位与核心转向

把知识库从「Collection + chunks + 向量」重设计为「**Vault（本地文件夹）+ Markdown 文档 + frontmatter 元数据**」：

- **文件系统即真相源**（D1）：用户知识以通用 `.md` 存在本地路径，可被 Git/编辑器/同步盘自由使用；Postgres/pgvector 降级为**可重建的派生索引**
- **embedding 从「必需组件」降级为「可选增强插件」**（D5）：三层检索中 L0/L1 完全无向量，L2 语义层可插拔（本地模型 / model2vec 静态查表 / 远程 API 三选一，缺省时系统完整可用）——与 NFR-11「无 LLM 降级」哲学一脉相承
- **检索引擎零重写**：现有 Retriever/Hybrid/Federated 全家桶保留，改动集中在文档来源层（VaultFiler/SyncEngine）与呈现层

一句话：**把「检索引擎选型焦虑」转化为「文档来源与组织革命」**——瓶颈不在 ANN 算法，而在知识的存在形态。

### V2. 核心概念模型

| 概念 | 定义 | 要点 |
|------|------|------|
| **Vault** | 一个本地文件夹路径即一个知识库（多 Vault，每库一路径） | Collection 平滑升级，Agent 绑定 `knowledge_bases` 语义不变；`root_path` 唯一约束防重复挂载；删 Vault 只删索引不动文件 |
| **文档** | Vault 内的 `.md` 文件（Markdown + YAML frontmatter） | 路径即 ID 的一部分；用户可直接编辑 |
| **frontmatter 摘要卡** | LLM 预生成的文档摘要，写入 frontmatter | 人可读、机可查、agent 低 token 消费（card ≤200 token）；含 `summary_hash`（被摘要内容的 hash），比对即知过期 |
| **双轨关联** | links 表三类型：`explicit`（`[[]]` 双链）/ `entity`（LLM 实体共现）/ `semantic`（向量近邻） | 显式优先；无语义层时 semantic 轨降级为「同文件夹 + 共享标签 + 双链共现」 |
| **派生索引** | chunks 向量（PG）、FTS5/BM25 词法索引、links 表 | 全部无状态可重建，reindex 一键化；与业务表零触发器耦合 |

### V3. 架构与数据流

```
用户文件夹（真相源）
   ↑↓ 双向同步（SyncEngine，轮询扫描；fsnotify 为可选加速）
VaultFiler（KB 侧唯一写文件出口）
   ↓ 变更事件
派生索引管道：chunk → embed(可选) → 向量(PG) / 词法索引(FTS5或Bleve) / links 表
   ↓
查询意图路由（规则，<5ms）
  ├── 搜文件/路径/精确短语 ──→ L0 精确层（无向量）：文件名索引 + 强 BM25
  ├── 浏览/导航/关联追问 ────→ L1 导航层（无向量）：knowledge_navigate 树导航 + 摘要卡 + 双链/实体图遍历 + knowledge_grep
  └── 概念/模糊/跨语言 ──────→ L2 语义层（可选插件，缺省时 L0+L1 完整可用）
```

关键组件：
- **VaultFiler**：KB 写文件的唯一出口（agent 写入、摄取落盘、frontmatter 更新都经它）
- **SyncEngine**：文件变更同步；对 KB 自写文件打标防 watcher 回环（R-2）；外部删除一律进 `.aranea/trash`
- **knowledge_navigate** 三级工具：tree（缩进树 ≤1k token）→ card（摘要卡 ≤200 token）→ read（分页全文）= PageIndex 推理导航模式的本地化实现，且真实文件夹树比 PageIndex 的 AI 生成 ToC 更可靠

### V4. 数据模型变化

| 项 | 变化 |
|----|------|
| `knowledge_collections` | 升级为 Vault：新增 `root_path`（唯一、规范化：resolve symlink + 绝对路径 + 尾部斜杠归一）、`sync_state`（含 `migrating` 态，S-2）、`sync_config` |
| frontmatter 受管字段分区（R-1） | KB 独占：`id/summary/tags/type/summary_hash/source/created`；其余归用户自由使用；写入前重读 hash，冲突备份 |
| `knowledge_links`（新增） | `(vault_id, src_path, dst_path, link_type[explicit/entity/semantic], meta)`；随文档增删级联，可全量重扫重建 |
| 词法索引 | SQLite FTS5（trigram）或 Bleve（R-5 选型后定），**纯派生索引**：无触发器耦合、无业务表依赖、DROP/REBUILD 无状态（吸取 messages_fts 被连根拔除的教训） |
| 向量命名空间（S-3） | 按 `(model, dim)` 隔离（现以 Collection 粒度绑定 `embedding_model+dim`，chunks 无 model 列；跨模型共存按 Collection 隔离，或后续扩展维度列），为 model2vec→bge 升级预留 reindex 能力 |
| EmbeddingModel 契约（R-4） | Collection/Vault 的 EmbeddingModel 从必填改**可选**（空 = 无语义层），放开 `ErrEmbeddingModelRequired` 校验（现状无 dim 合法性校验，`Dim<=0` 时缺省 1536） |

### V5. 三层检索设计（D5/D6/D7）

| 层 | 技术 | 路由意图 | 备注 |
|----|------|---------|------|
| **L0 精确层**（无向量） | 文件名索引 + 强 BM25（CJK bigram+unigram 分词、字段加权 title×20/tags×5/body×1、RM3/LLM 查询扩展） | 搜文件/路径/精确短语 | 分词学术定论：bigram+unigram 与最优中文词切分效果相当（Nie et al.）；查询扩展是收益最大一步（+10%~30%） |
| **L1 导航层**（无向量） | knowledge_navigate 树导航 + 摘要卡 + 双链/实体图遍历 + knowledge_grep | 浏览/导航/关联追问 | PageIndex 模式；真实文件夹树零自愈成本 |
| **L2 语义层**（可选插件） | Embedder 接口三实现：本地开源模型（bge-m3/bge-small-zh，ONNX/Ollama）/ model2vec 静态查表（~50MB，纯 Go 查表+均值池化，比 teacher 快 500 倍，potion-base-32M 质量达 MiniLM 的 93.2%）/ 远程 API | 概念/模糊/跨语言 | model2vec 是「自研 embedding」现实最优解：一次性蒸馏为本地资产，无模型推理依赖；定位为语义层零依赖默认实现而非唯一实现 |

**降级矩阵**（R-4 四条契约变更，§V6 引用）：
1. CreateVault 时 EmbeddingModel 改可选（空 = 无语义层）
2. 摄取流水线对无 embedding 的 Vault 跳过向量写入
3. knowledge_search 对无语义层 Vault 自动降级 L0+L1
4. 前端设置页 embedding 配置改「可选增强」

**向量仍不可替代的场景**（诚实边界）：跨语言检索、同义/模糊概念查询（查询扩展只能部分弥补）、「以文找文」相似推荐（无语义层时降级为同文件夹+共享标签+双链共现）。

**路由规则**：规则表配置化（<5ms，避免 LLM 路由反模式），前后端共享同一份定义，随 badcase 积累迭代；测试矩阵有/无语义层两套。

### V6. 实现契约（R-1~R-6，评审前置条件，实施必须遵守）

| # | 契约 | 归属 |
|---|------|------|
| **R-1** | frontmatter 受管字段分区：KB 独占 `id/summary/tags/type/summary_hash/source/created`，其余归用户；**写入前重读 hash，冲突备份**（保守默认：冲突留双份） | D1/D3 |
| **R-2** | SyncEngine 对 KB 自写文件打标（watcher 回环防护）；**外部删除一律进 `.aranea/trash`**，不物理删除 | D1 |
| **R-3** | entity 抽取必须做**停用词/频次过滤**（停用实体表 + 频次阈值）；关联区 UI 必须**标注来源类型**（显式/实体/语义），避免用户误以为全是可靠关联 | D4 |
| **R-4** | Embedder 接口抽象 + CreateVault 的 EmbeddingModel 改可选 + §V5 降级矩阵四条全部落地 | D5 |
| **R-5** | L0 选型 **spike 前置**：Bleve vs FTS5(trigram) vs 自研倒排，各 200 行验证 + 真实语料质量对比后再定；**禁止直接默认自研** | D6 |
| **R-6** | knowledge_write 安全契约：路径 sanitize（禁 `..`、禁 symlink 逃逸、限制 vault root 内）+ 覆盖前自动备份到 `.aranea/trash` + 每次写入记审计日志（who/when/what，结构化入 activities）；navigate/grep 只读；watcher 对 KB 自写文件打标防回环重复摄取 | D8 |

**建议项（S 系列，不阻塞）**：S-1 root_path 规范化 + 禁挂系统根目录校验；S-2 迁移期 `migrating` 态期间检索走旧索引；S-3 向量按 `(model,dim)` 命名空间隔离；S-4 目录级 README 自动摘要（二期候选）；S-5 「BM25 召回质量」纳入测试基线（50 条中英查询金标准，防分词器回归）。

### V7. Agent 工具族（D8）

| 工具 | 能力 | 约束 |
|------|------|------|
| `knowledge_navigate` | tree（≤1k token）→ card（≤200 token）→ read（分页全文）三级下钻，token 预算 + 超限截断提示 | 只读 |
| `knowledge_grep` | 内容正则/字面搜索（ripgrep 式），补精确内容搜索 | 只读 |
| `knowledge_write` | 创建/追加（Letta 式自编辑：agent 自主决定记什么） | **R-6 安全契约前置**；升级路径：创建/追加 → frontmatter 字段级编辑 → 批量整理（agent 图书管理员） |

工具 schema 演进需与 agent 提示词同步；write 审计日志为永久维护项。

### V8. 迁移设计（D9：Collection → Vault）

- 幂等可重入 + 失败单条跳过 + `schema_migrations` 门控（符合项目 L3 数据迁移惯例）
- content_text 导出 `.md`（文件名清洗 source→合法文件名，冲突处理）；chunks 重建期间检索可用性下降——迁移期 Vault 状态机加 `migrating` 态，期间检索走旧索引、写入排队（S-2）
- 旧「粘贴文本入库」入口保留降低断裂感；旧表只读兜底一个版本周期；迁移代码完成后可标记废弃

### V9. UI 设计（D10：资源管理器）

- 三栏布局：Vault 切换 + 文件夹树（懒加载）+ 文档列表 + 详情面板（hover 卡 + 详情两级密度）
- **统一搜索框双区**：即时区（纯前端毫秒，<10k 文档 fzf 式内存索引，多 vault 切换需重建）+ 语义区（亚秒）——正确分离「搜文件/搜知识」两种意图
- 关联区展示双链/实体/语义三类关联并**标注来源类型**（R-3）
- 图谱视图放二期（避免 MVP 膨胀）；搜索意图分流规则与后端路由规则共享定义（两处维护）

### V9.1 P3 实施契约（API + 组件）

**Proto 扩展**（`api/kratos/knowledge/v1/knowledge.proto`）：

| 项 | 变化 |
|----|------|
| `KnowledgeCollection` | + `root_path`/`sync_state`/`last_sync_at`（Vault 切换栏展示同步状态）；SP1-F + `vault_backend`（local/team） |
| `KnowledgeDocument` | + `rel_path`/`summary`/`tags`/`doc_type`（列表与 hover 卡一级密度直接可用，无需二次请求） |
| `rpc ListVaultTree` | `GET /v1/knowledge/vaults/{collection_id}/tree?prefix=`；懒加载：返回 prefix 直接子节点（目录+文件各一条），节点 `{name, path, kind(dir/file), doc_id, summary, tags, doc_type, status, size_bytes, updated_at}`；中栏文档列表复用文件节点（不再给 ListDocuments 加 prefix 过滤，YAGNI） |
| `rpc ListDocumentLinks` | `GET /v1/knowledge/documents/{id}/links`；返回 `{target_doc_id, target_source, target_rel_path, link_type, context, direction(out/in)}`，data 层 JOIN knowledge_documents 一次取回（禁 N+1） |

**Biz 层**：

- `ListVaultTree(ctx, collectionID, prefix)`：经新增窄接口 `ListDocumentPaths(ctx, collectionID) ([]DocumentPath, error)` 取全量轻量路径行（id/rel_path/source/summary/tags/doc_type/status/size_bytes/updated_at），在内存中聚合 prefix 直接子节点（目录去重排序在前，文件按名称排序在后）；非 vault 文档（rel_path 空）归入虚拟根层
- `ResolvedLink`（biz 类型）：Link + TargetSource/TargetRelPath/Direction；新增窄接口方法 `ListResolvedLinks(ctx, collectionID, docID, linkType)` 委托 data 层 JOIN 查询；`linkType` 空 = 全部三类

**前端组件**（三栏，遵守数据流铁律：api.ts → store → composable → page → 展示组件）：

| 组件 | 路径 | 职责 |
|------|------|------|
| `KnowledgeVaultTree.vue` | `components/knowledge/` | 左栏：Vault 切换头 + q-tree 懒加载文件夹树（emit `select(prefix)`） |
| `KnowledgeDocList.vue` | `components/knowledge/` | 中栏：资源管理器式表格——名称/修改日期/类型/大小/状态五列，列头点击升降序排序（目录优先），目录行点击下钻 + 面包屑返回；props/emits 纯展示 |
| `KnowledgeDocDetail.vue` | `components/knowledge/` | 右栏：两级密度——一级（摘要卡：summary/tags/type/时间）+ 二级（展开正文预览 + 关联区）；关联区三类来源徽标（explicit=显式双链 / entity=实体共现 / semantic=语义近邻，R-3） |
| `KnowledgeSearchDual.vue` | `components/knowledge/` | 统一搜索框双区：即时区（fzf 式前端过滤树节点+列表，<10k）+ 语义区（回车走后端 Search，意图分流规则与后端共享定义，见下） |

- 编排：`features/knowledge/useVaultExplorer.ts`（选中 vault/prefix/doc、树节点缓存、links 加载）；Page 只做布局与事件绑定
- **意图分流共享定义**：`web/src/features/knowledge/searchIntent.ts`（前端）与 `internal/knowledge/search_intent.go`（后端）维护同一规则表——命中路径/扩展名/引号短语 → 即时区；自然语言问句/概念词 → 语义区。两处维护，注释互指
- 旧三 Tab（documents/search/settings）收敛：documents 由三栏取代；settings（embedder 配置）保留为独立 Tab；search Tab 已由 3D 图谱取代（§V12.7），`KnowledgeSearchPanel` 随 G4 移除（高级检索选项 top_k/min_score/hybrid/rewrite/rerank 不再暴露 UI，检索能力由双区搜索语义区承载）；旧 `KnowledgeDocumentsPanel/KnowledgeDocPreviewDialog` 保留供迁移期回退

### V10. 技术升级路径（每层相互独立，接口隔离：Embedder / Retriever / LinkResolver）

```
L0 精确层:  自研bigram/FTS5 ──→ Bleve ──→ pg_textsearch/Tantivy ──→ SPLADE(远期)
L1 导航层:  真实文件夹树+摘要卡 ──→ 目录README自动摘要 ──→ 长文档AI ToC(PageIndex式) ──→ vault摘要树(RAPTOR式)
L2 语义层:  model2vec静态查表 ──→ bge-m3本地(ONNX/Ollama) ──→ 远程API ──→ HNSW(halfvec)索引升级
关联:      双链+实体共现 ──→ GraphRAG完整版(社区摘要,§9.6设计已备) ──→ KAG式符号推理(远期)
agent:     navigate/grep/write ──→ frontmatter字段级编辑 ──→ agent图书管理员(批量整理)
UI:        树+列表+详情+双区搜索 ──→ 局部图谱(二期) ──→ Q&A模式(三期)
```

### V11. 现状缺陷修正背景（评审代码核验发现）

1. **现有中文全文检索实际已失效**：[knowledge.go:514-517](../../internal/data/knowledge.go#L514-L517) 的 `ts_rank(to_tsvector('simple', ...))` 双重问题——`ts_rank` 无 IDF/无 TF 饱和（弱 BM25）；更严重的是 PG `simple` 分词对 CJK 不切分（需 zhparser/pg_jieba），连续中文归为单一 token，中文查询几乎只能整串精确命中。**强 BM25 自研栈不是「优化」而是「修复」**。
2. **FTS5 前车之鉴**：项目曾有 `messages_fts`（SQLite FTS5），因虚拟表+触发器与核心业务表耦合，演进时被整体移除（[20260902_drop_messages_subsystem.sql](../../internal/data/sql/migrations/20260902_drop_messages_subsystem.sql)）。知识库词法索引必须设计为完全独立的派生索引。
3. **embedding 可选化破坏现有契约**：当前 CreateCollection 校验 `ErrEmbeddingModelRequired`（EmbeddingModel 必填；`Dim<=0` 时缺省 1536，无 dim 合法性校验），必须按 §V5 降级矩阵四条变更（R-4）。

### V12. 资源管理器 V2 改版（G1~G4，2026-07-29 评审通过）

> 用户需求 8 条（树内新建/库融树/拖拽移动/详情面板改版/删 DropZone/搜索范围/删新建合集按钮/3D 图谱），评审后落此设计。分期：G1 树骨架 → G2 详情面板 → G3 拖拽+搜索范围 → G4 3D 图谱。

#### V12.1 页面结构（改版后）

- **浏览 tab**：左栏树（**一级节点=库**，懒加载目录；树底部 `[+ 新建库]` 融入原页头按钮）+ 中栏文件列表 + 右栏详情；页头删除「新建集合」；中栏顶部 DropZone 删除（上传入口移入树节点 hover 菜单）。
- **检索 tab**：改为 3D 知识图谱（左：3D 力导向图；右：操作台）。
- **选中态单一事实源**：`{collectionId, prefix}` 取代独立 vault 切换器状态；选中库节点=浏览根目录。

#### V12.2 树节点 hover 操作与科幻图标

| 节点类型 | hover 操作 |
|---|---|
| 库节点 | 新建目录、新建文档、上传文件、刷新、删除库；右侧保留同步状态徽标 |
| 目录节点 | 新建子目录、新建文档、上传文件（落此目录）、删除空目录 |
| 根级底部 | `[+ 新建库]`（复用 KnowledgeCreateDialog） |

图标配色：库=cyan、目录=violet、md=teal、图片=magenta、音视频=orange、error=red 脉冲；选中态 `drop-shadow` 光晕；全部走 CSS 变量双主题适配。

#### V12.3 后端契约变更（B1~B8）

| # | 契约 | 说明 |
|---|---|---|
| B1 | `ListVaultTree` 改实现 | **扫文件系统目录** + 联文档索引（替代纯 rel_path 聚合）：空目录可见、目录节点带 mtime；文件节点仍来自索引（summary/tags/status 等不变） |
| B2 | `rpc CreateVaultDir(collection_id, dir_path)` / `rpc CreateVaultDocument(collection_id, rel_path)` | 经 VaultFiler 建目录/写模板 md（frontmatter+空标题）；写后立即触发单文档 apply（不等 45s 轮询） |
| B3 | `IngestDocumentRequest` + `target_dir`（可选） | 上传到指定子目录：VaultFiler 落盘 → 同步入库（Vault 模式文件系统为真相源）；空 = 现有行为 |
| B4 | `rpc MoveDocumentToDir(id, target_dir, conflict_policy)` | 库内跨目录移动：VaultFiler 原子 move + `UpdateDocumentRelPath`（保留文档身份/chunks/hash）+ 入链重建；同名冲突：默认 → CodeConflict（前端弹 覆盖/保留两份/取消），`overwrite` = 目标旧版本入 trash，`rename` = 自动生成唯一名 |
| B5 | `rpc UpdateDocumentContent(id, content, base_hash)` | 编辑保存：VaultFiler.WriteDocCAS（冲突留双份返 CodeConflict）→ 触发重索引 |
| B6 | `GET /v1/knowledge/documents/{id}/asset` | asset_uri 原始文件流式输出（图片/音频/视频内联渲染，word 下载） |
| B7 | `SearchRequest` + `path_prefix`（可选） | BM25/向量 SQL 增加 `rel_path LIKE prefix%` 过滤 |
| B8 | `rpc ListCollectionGraph(collection_id, link_types[], path_prefix)` | 返回 `{nodes:[{doc_id,name,rel_path,doc_type,degree}], edges:[{source,target,type}]}`，一次性全量（<2k 节点） |

#### V12.4 详情面板改版（G2）

- 第一行：**摘要**（一行省略，名称/路径缩小为副标题）；hover 显示 360px 大号浮层卡（完整摘要+元信息）。
- 第二行：关联计数 chips（显式/实体/语义），点击锚滚关联区。**计数口径 = 不重复目标文档数**（按 `target_doc_id` 聚合去重，与关联列表行数一致）；零计数 chip 禁用点击（UX 整改 2026-08-05）。
- 关联列表：**按目标文档聚合**（一篇文档一行），方向合并标注（双向 =「互引」，单向 =「本文引用」/「被引用于」）；同文档多方向不重复成行（UX 整改 2026-08-05）。
- 正文/媒体区：固定高 420px，主题化滚动条（`::-webkit-scrollbar` 用 `--color-border`/`--color-primary`）；md/txt 可编辑（编辑态等宽 textarea → B5 保存，CAS 冲突提示重载）；图片 `<img>`、音频/视频原生播放器（B6 流）、word 显示解析后 md + 原文下载。
- **错误态**：正文/关联加载失败显示内联错误 +「重试」按钮（复用「解析中」占位属误导）；重复点击已选中文档且上次失败时自动重新拉取（UX 整改 2026-08-05）。
- **布局**：三栏中左右列 `position: sticky` + 视口高度限制，列内独立滚动——关联区锚滚不触发整页下跳（UX 整改 2026-08-05）。

#### V12.5 拖拽移动（G3）

- HTML5 DnD（不引库）：中栏文件行 `draggable`，合法目标=树目录节点/库节点（=根）/面包屑段；拖动幽灵卡+目标发光高亮+非法禁用；面包屑 hover 500ms 展开。
- 同名冲突弹确认：覆盖 / 保留两份（`name (2).md`）/ 取消；失败回滚提示。
- 跨库拖拽本期禁止（维度冲突风险，保留按钮式跨库 MoveDocument）。

#### V12.6 搜索范围选择器（G3）

- 搜索框左侧「范围」按钮：弹出迷你目录树（仅目录单选），选中后即时区前端 prefix 过滤 + 语义区走 B7；再选「全库」或 × 清除。
- **每次打开菜单自动展开库根节点**并触发懒加载（而非仅首次；菜单关闭期间节点对象可能随父级重建，需重新赋展开数组让 q-tree 重新评估 lazy 节点）（UX 整改 2026-08-05）。

#### V12.7 3D 知识图谱（G4）

- **选型**：`3d-force-graph`（three.js 封装，力导向+交互开箱，~60KB gzip）；不手写 three.js。
- **节点**：doc_type 着色、大小=连接度；**边**：explicit=primary / entity=紫 / semantic=青（预留，P4b 前两类先行），带方向箭头。
- **交互**：左键旋转/右键平移/滚轮缩放；hover 高亮一跳邻居淡化其余；点击选中联动操作台。
- **操作台**：库 select、边类型过滤 chips、目录前缀过滤（复用 V12.6 组件）、节点搜索定位、节点列表（连接度排序，点击聚焦）、选中节点卡（「在浏览中打开」→ 切浏览 tab 定位选中）。
- **规模**：>2k 节点默认只渲染有连接节点 + 「显示孤立节点」开关。

#### V12.8 图谱深空版渲染层 + 实体治理（G5，2026-08-07 评审通过）

> 调研依据：`docs/reports/2026-08-07-research-knowledge-graph-oss.md`（三个 Obsidian 图谱开源仓库逐行精读）。**V12.7 选型条款作废**：`3d-force-graph` 每节点一个 Object3D，万级节点不可行，且辉光/粒子流/星河背景等视觉不可控；G5 起自研渲染层，`3d-force-graph` 依赖完全移除（`three` 保留）。数据契约 B8（`ListCollectionGraph`）不变——G5 是纯渲染层替换 + 实体轨增强，无图谱数据 API 破坏。

##### V12.8-1 渲染层架构（前端）

```
web/src/features/knowledge/graph3d/        ← 纯 TS 引擎（零 Vue/three 依赖，全部可单测）
  ├── model.ts         SoA 图模型：positions/velocities Float32Array(3N)、degree Uint16、
  │                    groupId Uint16、edges Int32Array(2E)；docId↔index 双射；
  │                    确定性播种 mulberry32 + 球内体采样 r=(cbrt(N)*20+1)*cbrt(rand)
  ├── octree.ts        typed-array 八叉树池（Float32 8/cell + Int32 9/cell，容量 16N 倍增，
  │                    显式栈迭代，质心除法延迟到查询）
  ├── forces.ts        物理引擎（主线程/Worker 共用同一代码）：
  │                    BH 斥力(repulsion=30,theta=0.8) + 弹簧(0.05/30) + 簇凝聚(0.08)
  │                    + 簇分离(100·count/d²) + 向心力(0.011)；显式 Euler damping=0.9；
  │                    maxStep 位移钳制(≤linkDistance)；alphaDecay=0.0228，alphaMin=0.005
  ├── protocol.ts      Worker 消息协议：init(slice 后 transfer)/setParams/pin/unpin/reheat/
  │                    stop ↔ tick{positions,alpha}/stopped/error
  ├── tiering.ts       节点三层分级：supernode=degree≥15；ultranode=连接≥4 个不同 supernode；
  │                    尺寸倍率 1.0/1.5/2.5；分层 charge(-120/-200/-350)
  ├── palette.ts       分组调色板（doc_type 稳定哈希取色，沿用 G4 graphUi 调色板语义）
  ├── particleMath.ts  粒子流纯数学：相位均布 prog[i]=i/n、easeInOutQuad、
  │                    时变 HSL hue=0.5+0.32·sin((t·0.6+p·2.2+i·0.12)·π)
  ├── engine.ts        纯 TS 装配（模型+Worker 客户端+交互状态机），被 Canvas 组件持有
  └── physics.worker.ts  物理 Worker（16ms tick，alpha<alphaMin 自停发 stopped）

web/src/components/knowledge/graph3d/      ← Vue/three.js 命令式壳
  ├── KnowledgeGraph3DCanvas.vue   装配：Renderer+Worker 客户端+交互桥；IntersectionObserver
  │                                离屏 pauseAnimation；Worker 失败主线程 RAF 兜底
  └── render/
      ├── NodeLayer.ts     InstancedMesh 低模球(6,4)+MeshBasicMaterial(加法混合)；
      │                    instanceColor + baseColors 缓存 + lerp(white,0.5) 高亮；
      │                    大小=base+sqrt(degree)·scale × 分级倍率
      ├── EdgeLayer.ts     微弯 Bezier 边（QuadraticBezierCurve3，bow 0.3·len，垂直轴
      │                    hash01("s->t")·2π 定向，6 段）；单 LineSegments + vertexColors
      │                    （rest=边类型色×0.32，hover 关联边=×0.9 瞬时换色）
      ├── ParticleLayer.ts 粒子流（MAX=80、SPEED=0.45/s、PointsMaterial size=8 +
      │                    64px 径向渐变 glowTexture + vertexColors + depthWrite:false）
      ├── BackdropLayer.ts FBM 星云反转球(3-octave，colA 紫(0.12,0.06,0.22)/colB 青
      │                    (0.05,0.17,0.21)，bright=0.5，pow(fbm,2.2) 压在 bloom 阈值下)
      │                    + 三档星空(dim 2400/med 4800/bright 800，球面均匀，
      │                    sizeAttenuation:false，64px 柔光 dotTexture)
      │                    + 核雾(520 颗加法 Points，布局收敛后锚定度数最大 hub)
      ├── BloomPipeline.ts EffectComposer：RenderPass + UnrealBloomPass(strength≈1.2,
      │                    radius=0.5, threshold=0.28，半分辨率 w/2×h/2，nMips=3)；
      │                    ACESFilmicToneMapping exposure=1.2；不透明深空底 #050810
      │                   （bloom 与透明背景不兼容）；strength=0 时整 pass enabled=false
      ├── LabelLayer.ts    three-spritetext 标签（挂节点子对象，LABEL_HEIGHT/r 抵消父缩放）；
      │                    显示阈值：相机距离/节点度数双阈值，密图不糊屏
      └── Picker.ts        Raycaster 逐实例求交 + mousemove 去抖（同时防粒子相位重置）
```

**关键机制**：
- **lazy-render**：`needsRender || particles.active || autoRotate` 才调 `composer.render()`；物理 tick 到达/交互/hover 各事件置 needsRender。收敛静置后 GPU 零占用。
- **Worker 客户端**：init 时 slice 复制 buffer 再 transfer；tick 回传 positions 直接作 BufferAttribute 源；Worker 创建失败 → 主线程 RAF 跑同一 `forces.ts` 引擎（NFR-G5-5）。
- **hover 一跳邻居**：全边表 O(E) 扫描 + 去抖（低频不建邻接表）；邻居集驱动 NodeLayer 提亮、其余节点 instanceColor 压暗（向底色 lerp 0.08）、EdgeLayer 关联边换色、ParticleLayer 发射。
- **pin 拖拽**：自研 pin-and-move——拖拽平面（过节点、法线朝相机）+ grabOffset 防跳变；拖拽中挂起 controls/autoRotate；位置写 fx/fy/fz（Worker pin 消息）；只重写关联边顶点。
- **zoom-to-cursor**：滚轮射线 ∩ 过 target 面向相机平面求 pivot，相机+target 同步缩放 `0.95^(-ΔY·0.01)`。
- **局部图谱**：N 跳 BFS（1-4）→ 子图重建（新 GraphModel + 重播种），**groupId 沿原图复制保持跨视图颜色一致**；「返回全局」恢复。
- **交互状态机**：`shown`(hover)/`selected`(单击锁定) 分离；位移 <5px 区分拖拽与点击；双击 =「在浏览中打开」（沿用 G4 跨 tab 定位链路）。

**数据流合规**：组件不直接调 api/store——`useKnowledgeGraph.ts` 编排不变（B8 数据/三维过滤/选中/聚焦信号），新增 `graph3d/engine.ts` 纯 TS 装配被 Canvas 组件持有；Worker 文件经 Vite `?worker` 引入。

> **As-built（2026-08-08，渲染管线 v2 —— 万级性能 + 降亮度 + 科幻视觉，替代原 InstancedMesh 方案）**：
>
> 原设计（InstancedMesh + 微弯贝塞尔 + Raycaster 逐实例求交）在万级规模不可行：每实例矩阵求逆致 hover 卡顿、每 tick CPU 重组 instanceMatrix 上传 640KB、加法混合重叠烧白。v2 全量重写渲染层为 **GPU 位置纹理管线**：
>
> - **PositionTexture.ts（新增）**：RGBA32F DataTexture（尺寸 = ⌈√N⌉² 向上取整，`textureLayout.ts` 纯函数可单测）；物理 tick 回传的 positions 一次 memcpy 写入 + `needsUpdate`，万级 ≈0.3ms/tick。节点/边/瞄准具顶点着色器统一 `texelFetch(uPosTex, ivec2(idx % texW, idx / texW))` 取世界坐标——每 tick 零 CPU 几何计算。
> - **NodeLayer.ts（重写）**：InstancedMesh → `THREE.Points`（1 节点 = 1 顶点，`gl_VertexID` 取位）；Obsidian 签名柔光点（core 亮核 + halo 外晕径向衰减，`gl_PointSize` 距离缩放 + 亚像素 vFade 淡出防抖）；**普通混合替代加法混合**（重叠不烧白，降亮度核心手段）；静态属性 aColor/aSize，动态属性仅 aEmph（邻居 1.6 / 压暗 0.15 / 常态 1.0，rest 增益压在 bloom 阈值下）。
> - **EdgeLayer.ts（重写）**：6 段贝塞尔 → **Obsidian 细直线**（每边 2 顶点，顶点量 ÷6，密图视觉更净）；rest α=0.16 细线；hover 关联边提亮 + **流动光脉冲**（`sin(uTime·7 − vT·16)` 沿边跑动的数据流光效，科幻感来源）；动态属性仅 aHi per-edge。
> - **ReticleLayer.ts（新增）**：HUD 风瞄准具——hover 圆环 + 选中六边形，视空间 billboard（`mv.xy += corner·size`），uHoverIndex/uSelIndex uniform 驱动，-1 隐藏；科幻交互反馈核心。
> - **Picker.ts + pickMath.ts（重写）**：弃 three Raycaster（每实例矩阵求逆 = 万级 hover 卡顿元凶）→ 自研**射线-球纯循环 O(N)**（无矩阵运算）；阈值 = max(节点半径, t·worldPerPixel·slackPx)（slack 随距离放大保证远节点可点），最近 t 获胜保遮挡语义。
> - **qualityTiers.ts（新增）**：自适应画质三档 HIGH/MID/LOW——初始按节点数分级（≥2500 MID、≥8000 LOW，万级 LOW 起步保帧率）；运行期 governor（FPS EMA，连续 90 帧 <45fps 降档 / 连续 600 帧 >57fps 升档不超初始档顶防振荡）；档规格驱动 bloom 开关/分辨率、pixelRatio 上限、标签候选数（200/100/40）；HUD 档位指示。
> - **BackdropLayer.ts（降亮度 v2）**：星云改**烘焙**——FBM 着色器一次性渲入 1024×512 equirect RT（弃每帧全屏 FBM ≈60M hash/帧），球体改贴图采样；bright 0.5→0.34、星空不透明度 ×0.65、核雾 0.2→0.1，全部压在 bloom 阈值 0.55 下。
> - **BloomPipeline.ts**：threshold 0.28→0.55、strength 1.2→0.9（只有真高亮节点冒辉光）；曝光收敛。
> - **测试**：新增 `textureLayout.spec.ts`（尺寸取整/边界）、`pickMath.spec.ts`（射线-球/遮挡/slack）、`qualityTiers.spec.ts`（初始分级/governor 升降档/防振荡）、`ReticleLayer.spec.ts`；NodeLayer/EdgeLayer 全量重写用例。graph3d 域 15 文件 106 用例全绿。
> - **性能预期**：万级节点每 tick GPU 侧一次纹理上传（≈0.3ms）+ 顶点着色器取位，CPU 渲染线程零几何计算；LOW 档关 bloom + pixelRatio=1 + 标签 40 候选，交互帧率优先。
> - **标签可见性语义（2026-08-08 G5-G 复验修订，替代原「距离/度数双阈值」描述）**：候选标签可见性 = `dist ≤ maxDistance && labelsEnabled && degree ≥ minDegree`，hover/选中 forced 标签豁免全部三重阈值（labels OFF 时悬停/选中仍出标签）。`maxDistance = fitDist + 半径`（适应视图即全候选可达，拉远按距离渐进隐藏——原 `fitDist×0.85` 在适应视图即全隐藏，已修复）；`minDegree = effectiveMinDegree(图最大度数, 基准 4)`——小图（最大度数 < 4）降档到最大度数保证 hub 标签可见，全孤立图钳 1（孤立节点不出标签）。`LabelLayer.spec.ts` 覆盖 `effectiveMinDegree` 边界。

##### V12.8-2 HUD 操作台（前端）

- 保留 G4 全部控件与 `KnowledgeScopePicker` 复用；视觉换肤限定 `.kg-hud` 作用域类，**不改全局主题 token**（NFR-G5-4）：等宽字体（'JetBrains Mono', Consolas, monospace）、主青 `#00d4ff`、边色 `#1a3a4a`、`letter-spacing:0.08em`、面板 `rgba(5,8,16,0.88)` + `box-shadow:0 0 15px #00d4ff22` + 1px 青色描边、开关为 `[ ON ]/[ OFF ]` 括号式、边类型图例发光色块。
- 新增「实体治理」分区（B10/B11 数据）：合并建议列表（保留名 ← 候选名、来源徽标 norm/embedding、相似度）、一键合并按钮、合并结果重写条数内联反馈。
- 新增 HUD 控件：auto-rotate 开关、标签开关、「聚焦邻域」（跳数 1-4 步进）+「返回全局」。
- i18n：全部文案入 locale 文件（check-i18n 红线）。

> **As-built（2026-08-08，G5-G 治理前端完成）**：
>
> - **治理分区数据流**：`useKnowledgeGraph.ts` 持有 `mergeSuggestions`/`merging`/`lastMergeResult` 三状态——建议只随 collectionId 加载（watch immediate；与边类型/目录过滤无关），拉取失败降级空列表且不置主 error（辅助数据不阻断图谱主流程）；`mergeEntities(keeperId, mergeeId)` 防重入，成功后并行重拉图谱（边可能变化）与建议列表，`lastMergeResult` 内联展示重写条数、切库清空。组件经 props/emit 接线（`merge-suggestions`/`merging`/`last-merge-result` 入，`merge-entities` 出），不直接调 api——数据流铁律合规。
> - **API 映射**：`listEntityMergeSuggestions`/`mergeKnowledgeEntities` 走既有 `svc` 客户端，snake/camel 双键容错映射（`pickI64/pickStr/pickNum`）对齐模块既有风格。
> - **UI 契约**：建议列表限高 148px 滚动防挤压节点列表；来源徽标 `norm`（归一化冲突）/`embedding`（语义相似，附相似度两位小数）；合并按钮 `kg-hud__switch--accent` 皮肤 + `merging` 禁用防重入；反馈行 `合并完成：重写 {mentions} 处提及 · {links} 条关联`（positive 色）。
> - **i18n 键**：`knowledgePage.graphEntityGovernance`/`mergeSource.norm`/`mergeSource.embedding`/`mergeAction`/`mergeNoSuggestions`/`mergeFeedback`（zh-CN + en-US）。
> - **测试**：`useKnowledgeGraph.spec.ts` 新增 2 用例（建议随库加载 + 拉取失败降级空列表不污染 error；mergeEntities 调 RPC → 重拉图谱与建议 → 反馈置位）。浏览器复验：norm 冲突三元组造数 → 建议出现 → 一键合并 → 反馈与 DB 重写一致 → 建议清空。

##### V12.8-3 实体消歧（后端）

现状：`knowledge_entities(collection_id, name, entity_type)` UNIQUE(collection_id, name)；`knowledge_doc_entities(doc_id, entity_id, mentions)`；抽取仅 TrimSpace + 停用词。G5 增强（遵循项目字典模式约定：字典表存规范值+治理元数据，改名/合并走事务重写引用并返回重写条数）：

| # | 契约 | 说明 |
|---|------|------|
| B9 | 实体归一化 | `NormalizeEntityName`：Unicode NFC + case-fold（strings.ToLowerSpecial 用 unicode.CaseRanges 语义无损小写）+ 内部空白折叠为单空格 + 去首尾；DDL 迁移：`knowledge_entities` 加 `name_norm TEXT` + 回填 + `UNIQUE(collection_id, name_norm)`；`ReplaceDocEntities` 按 name_norm 查/建字典条目，name 保留首见写法作展示名 |
| B10 | `rpc MergeKnowledgeEntities(collection_id, keeper_id, mergee_ids[])` | `Data.ExecInTx`：mergee 的 `knowledge_doc_entities` 重写指向 keeper（mentions 求和，(doc_id,entity_id) 冲突时合并）、`knowledge_links` entity 轨 context 中的实体名引用重写、mergee 条目删除；返回 `{rewritten_mentions, rewritten_links}`；写流程日志（step 登记 `knowledge.entity.merge`） |
| B11 | `rpc ListEntityMergeSuggestions(collection_id)` | 归一化冲突组（name 不同 name_norm 相同——迁移期可能存在）+ 配置 embedding 时高相似对（实体名 embedding 余弦：≥0.90 标 auto 候选、0.80-0.90 标 suggest；embedding 未配置仅返回 norm 组，对齐 NFR-15）；orphan 语义不适用（实体全部来自字典表） |
| B12 | `knowledge_entity_aliases` 表 | `(collection_id, alias_norm, entity_id)`：合并时 mergee 的 name/name_norm 落为 keeper 别名；后续抽取先精确 name_norm → 再别名命中 keeper——合并效果跨同步持久 |

- **解析管线**（~~`vault_entity.go`~~ 抽取落库路径——该文件为 TEST_ONLY 僵尸实现，2026-08-14 已删除；以下管线契约保留，重新实现抽取器时遵循）：归一化 → 精确 name_norm → 别名 → （embedding ≥0.90 自动合并入别名表）→ 新建条目。0.80-0.90 不自动动数据，仅入建议（B11 实时计算，不落队列表——YAGNI）。
- **触点收窄**：归一化只在 `ReplaceDocEntities` 入口与 B10 合并写路径生效；`FindEntityCooccurrences` 查询改按 entity_id 关联（已无 name 字符串比对）。
- **迁移幂等**：DDL 迁移 SQL 幂等（IF NOT EXISTS）；回填 `name_norm` 用 DB 侧 `lower(nfc)` 不可行（PG 无 NFC）——回填走 Go 数据迁移（L3 数据迁移体系），冲突组按 id 最小者为 keeper 自动合并并落别名。

> **As-built（2026-08-08，G5-F 完成）**：
> - **B9**：`internal/biz/knowledge/entity_norm.go`（`NormalizeEntityName`：NFC + case-fold + 内部空白折叠单空格 + 去首尾）；DDL 迁移 `internal/data/sql/migrations/20261129_knowledge_entity_governance.sql`（`name_norm` 列 + `UNIQUE(collection_id, name_norm)` + `knowledge_entity_aliases` 表，注册于 `ddl_migration_registry.go`）；L3 Go 数据迁移回填 + 冲突组自动合并（keeper=组内 id 最小者）落别名，与 B10 共享 `mergeKnowledgeEntityRows`/`rewriteEntityLinkContexts` helper 防逻辑漂移。
> - **B10**：`internal/data/knowledge_entities.go` `MergeEntities`（`PostgresExecInTx`；幂等——不存在的 mergee 跳过、keeper 缺失返回 NotFound、keeper 混入 mergeeIDs 防御剔除）；biz `Usecase.MergeEntities`（未接线 EntityRepo 显式报错）；service `internal/service/knowledge_governance.go`（参数校验 + 跨租户 NotFound + 流程日志 K1 入口/出口 + K2 错误路径）。
> - **B11**：`internal/biz/knowledge/entity_suggest.go` `ListEntityMergeSuggestions`——norm 冲突组在前（keeper=组内最小 id，Similarity=1.0/tier=auto），embedding 对按相似度降序追加（`EntityEmbedder` 窄端口，生产实现=`MultiProviderEmbedder` 方法子集结构满足；O(N²) 余弦，N≤500 按 id 序截断；id 对去重 norm 优先）；embedder nil 或调用失败降级 norm-only 不报错；service handler 只读不发流程日志。
> - **B12**：`knowledge_entity_aliases(collection_id, alias_norm, entity_id)`；`ReplaceDocEntities` 解析管线按设计落地为：归一化 → 精确 name_norm → 别名命中 keeper → 新建（同批归一化撞车 mentions 求和；孤儿实体清理级联别名）；设计中「embedding ≥0.90 自动合并入别名表」步骤未接线（YAGNI——自动动数据风险高，当前仅走 B11 建议由用户确认合并）。
> - **测试**：`entity_norm_test.go`（归一化矩阵）、`internal/data/knowledge_entity_resolution_test.go`（PG 实测：norm 聚合/别名路由/合并全契约/幂等重跑/跨同步持久）、`entity_suggest_test.go`（norm 组/embedding 对/去重/降级）、`internal/service/knowledge_governance_test.go`（参数校验/跨租户/proto 映射/nil embedder 降级）。

##### V12.8-4 移除清单

- 依赖：`3d-force-graph` 从 `web/package.json` 移除；新增 `three-spritetext`（标签）。
- 组件：`KnowledgeGraphCanvas.vue` 删除，由 `graph3d/KnowledgeGraph3DCanvas.vue` 取代；`KnowledgeGraph3D.vue` 面板改造（HUD 皮肤 + 新工具条 + 治理分区）。
- 纯函数：`graphUi.ts` 中 `graphContainmentForce`（d3 协议专用）随旧画布删除；配色/排序/过滤/一跳邻居等纯函数保留复用。
- G4 已知问题条款（containment 防飞散）由自研引擎的向心力 + maxStep 钳制原生覆盖。

#### V12.9 知识库全面升级（M1~M5 + B1/B2，2026-08-12）

> 方案档案：`docs/superpowers/plans/2026-08-12-knowledge-galaxy-liquid-glass.md`（已实施完成；spec：`docs/reports/2026-08-12-plan-knowledge-galaxy-liquid-glass.md`）。
> 性质：G5 图谱深空版延续——前端纯增量增强既有引擎（GPU 位置纹理管线 / Worker 物理 / lazy-render 均不动），零新 npm 依赖；B1/B2 复用既有摄取管线。**数据表无变更**（无 Ent Schema 变更、无 DDL 迁移）；Proto 新增 2 个 RPC（契约见 V12.9-8）。
> 需求：[37-knowledge.md §子模块：知识库全面升级（V4 需求）](./37-knowledge.md#子模块知识库全面升级v4-需求2026-08-12)（US-26~US-31、FR-V4-1~7、验收 45~51）。

##### V12.9-1 M1 Liquid Glass 真折射

- `components/knowledge/effects/LiquidGlassDefs.vue`（新增）：SVG filter 单例集中管理——`kb-liquid-refract`（装饰光纹，原 GlassPanel 内联迁移至此，消除多实例重复 id）+ `kb-liquid-bg`（背景真折射：feTurbulence + feDisplacementMap 低频细腻位移）；SVG 本体 0 尺寸、`aria-hidden`。`KnowledgeWorkbench.vue` 根挂载一次，全部 GlassPanel 共享。
- `GlassPanel.vue` 新增 `refract` prop → `kb-glass-panel--refract` 修饰类；`css/deep-space.sass` 真折射类以 `backdrop-filter: url(#kb-liquid-bg)` 实现，`@supports` 探测失败回退既有 `blur+saturate`（零回归）。仅显式 `refract` 的浮层启用（三个既有浮层：全库搜索/命令面板/设置）。
- 测试：`LiquidGlassDefs.spec.ts`（双 filter 单例 / feDisplacementMap 存在 / 不可见不占布局）、`GlassPanel.spec.ts`（refract 修饰类 / 防重复 id 回归 / 装饰层保留）。

##### V12.9-2 M2 星系盘物理 + 布局切换

- `graph3d/forces.ts` 新增三力（并入 ForceEngine 同一 tick，三力全 0 时零开销不启用）：`coreGravity`（核心引力 `k/(1+r·0.02)`）、`discFlatten`（Y 轴压向 XZ 盘面）、`spiralSwirl`（绕核涡旋，带包络衰减）；`FORCE_DEFAULTS` 三力为 0（力导向行为不变），`GALAXY_FORCE_PARAMS`（0.08 / 0.12 / 0.02）为星系盘预设。
- `graph3d/engine.ts` 新增 `setLayout('force' | 'galaxy')`：`setParams(预设)` + reheat（alpha 再加热）——布局切换为物理 morph（节点连续流动重组），**非坐标插值**；Worker 与主线程兜底两路径同一引擎、行为一致。
- `render/EdgeLayer.ts`：星系盘下边为曲线（segments + curvature uniform）；力导向保持 Obsidian 细直线不回归。
- `KnowledgeGraph3DCanvas.vue` 新增 `layout` prop；`KnowledgeGraph3D.vue` 顶栏 HUD 布局切换按钮（力导向/星系盘）+ `localStorage('kg3d-layout')` 持久化。
- 测试：`forces.spec.ts`（三力方向 / 零值不启用）、`engine.spec.ts`（setLayout 预设 + reheat）、EdgeLayer 相关 spec。

##### V12.9-3 M3 电影感镜头（cameraDirector，AS-FSM-01 合规）

- `graph3d/cameraDirector.ts`（新增）：显式状态机——`CameraState = idle | flying | orbiting | cruising | genesis`，`canTransition(from, event)` 合法转换表 + 转换校验（非法转换拒绝返回 false）；genesis 开场运镜驱动 NodeLayer 节点依次显现（reveal）；user-interrupt 事件随时打断运镜回 idle（用户接管镜头）。
- Canvas 集成：装载后进入 genesis，完成后回 idle；拖拽/缩放等交互接线打断。
- 测试：`cameraDirector.spec.ts`（状态枚举 / 转换表 / 非法拒绝 / 打断）、NodeLayer reveal 测试。

##### V12.9-4 M4 聚焦模式 + 节点卡

- `graph3d/interaction.ts` 扩展：`nHop(edges, edgeCount, root, n)` BFS（返回节点集 + 两端点均在集内的边集；n=0 仅根节点）+ 聚焦锁定状态。
- `components/knowledge/graph3d/FocusCard.vue`（新增）：真折射玻璃节点信息卡（GlassPanel refract）；「在浏览器打开」（沿用跨 tab 定位链路）+「重新向量化」按钮（= B1 入口②，见 V12.9-6）；卡片拖动由 pointer 事件实现。
- Canvas：单击节点 = 锁定聚焦（nHop 邻域保持正常亮度、其余压暗 dim）；点击空白 = 解除（`clearFocus` 经 defineExpose 暴露）。
- 测试：`interaction.spec.ts`（nHop 矩阵 / 边集语义）、`FocusCard.spec.ts`。

##### V12.9-5 M5 过滤图例 + 透镜

- `graph3d/model.ts` 新增 `filterGraphByGroups(model, hiddenGroups)`：按 doc_type 组过滤，边级联排除（端点被滤则边排除）；**空集合零开销——引用相等直接返回原模型**（过滤管线性能守卫）。
- `components/knowledge/graph3d/GraphLegend.vue`（新增）：颜色点 + 组名 + 计数；点击 `toggle-group` 切换隐藏（隐藏组行置灰斜体）；悬停发 `lens-enter`/`lens-leave` 透镜事件；空 docType 回退显示 i18n `graphLegendUntyped`（「未分类」）。
- `KnowledgeGraph3DCanvas.vue` 新增 `setLens(docType | null)`（defineExpose）：组内节点 emph 1.6 / 组外 0.15 压暗；优先级：聚焦锁定（M4）> 透镜 > hover。
- `KnowledgeGraph3D.vue` 挂载 GraphLegend——**v-if 键定未过滤 legendNodes**（全组隐藏时图例仍保留，提供恢复路径；空态覆盖层同改）；`KnowledgePage.vue` 持有 `graphHiddenGroups` + `localStorage('kg3d-hidden-groups')` 持久化 + `graphRenderNodes/Edges` 过滤管线。
- 测试：`model.spec.ts` 追加 describe（组过滤 / 边级联 / 空集合引用相等）、`GraphLegend.spec.ts`（5 用例：渲染 / 未分类回退 / toggle / 置灰斜体 / 透镜事件）、`KnowledgeGraph3D.spec.ts`（新建 2 用例：全组隐藏图例不消失陷阱回归）。

##### V12.9-6 B1 文档重嵌入（ReembedDocuments）

- **分层**：Proto 新增 RPC（契约见 V12.9-8）；data 层 `ListDocumentsPendingReembed`（`internal/data/knowledge.go`，chunks embedding IS NULL 或无 chunks 的待重嵌入文档）；biz `DocumentRepo`/Usecase 扩展；service `internal/service/knowledge_reembed.go`（串行重嵌入管线）。
- **管线**：从已存 `content_text` 重新分块 + 嵌入（复用既有摄取管线 `BuildIndexedChunks` + `DeleteChunksByDocument`），单 goroutine 串行执行，WS 进度复用摄取通道；跳过并计数：content_text 空 / 正在 indexing / Vault 文档（走 vault_sync 自愈）。
- **流程日志**：step 登记 `knowledge.reembed.start` / `knowledge.reembed.done`（`internal/event/flow_log.go` stepTitleRegistry，同步 `docs/development/52-flow-logger.design.md` §5.1）。
- **前端**：`features/knowledge/api.ts` / `stores/knowledge` `reembedDocuments`；入口① `WorkbenchSidebar.vue` 文件行右键菜单「重新向量化」（词法库置灰 + `reembedNoSemantic` 提示）；入口② FocusCard 按钮；确认对话框（`reembedConfirmTitle`/`reembedConfirmBody`）+ 受理通知 `reembedAccepted`（「已受理 {n} 篇重嵌入」）。

##### V12.9-7 B2 集合语义层启用（EnableCollectionSemantic）

- **语义**：单向启用（空 → 启用，不可逆不覆盖）——守卫 UPDATE 绑定当前全局 embedder 模型/dim；已启用返回 CodeConflict；embedder 未配置返回 CodeBadRequest；绑定成功后全部内容文档入 B1 重嵌入队列（复用 V12.9-6 管线）。
- **流程日志**：step 登记 `knowledge.collection.enable_semantic`（`internal/event/flow_log.go`）。
- **前端**：`KnowledgeVaultTree.vue` vault 根菜单「启用语义检索」（`data-test="vault-enable-semantic"`，仅词法库 `v-if` 渲染）；`useKnowledgePage.ts onTreeNodeAction` 新增 `enable-semantic` 分支 + 确认对话框（展示将绑定的模型名/dim：`enableSemanticTitle`/`enableSemanticBody`）+ 受理通知 `enableSemanticAccepted` + `loadCollections` 刷新；`KnowledgePage.vue` 向树传 `lexical-vault-ids`（collections 中 `embedding_model === ''` 的 id 集）。

##### V12.9-8 API 端点契约（新增 RPC，与 `api/kratos/knowledge/v1/knowledge.proto` 一致）

| RPC | HTTP | 说明 |
|-----|------|------|
| `ReembedDocuments` | `POST /v1/knowledge/collections/{collection_id}/documents:reembed` | body：`{doc_ids[], chunk_size, chunk_overlap}`（`doc_ids` 空 = 全集合待重嵌入文档）；返回 `{accepted_count, skipped_count}`（skipped = content_text 空 / indexing 中 / vault 文档走 sync 自愈） |
| `EnableCollectionSemantic` | `POST /v1/knowledge/collections/{collection_id}:enable-semantic` | 单向启用语义层：绑定当前全局 embedder（已启用 CodeConflict；embedder 未配置 CodeBadRequest）；返回 `{enqueued_docs, embedding_model, dim}`，全部内容文档入 B1 重嵌入队列 |

**数据表变更**：无（无 Ent Schema 变更、无 DDL 迁移）。

---

## 子模块：双模块级知识内核（SP1，2026-08-08 评审通过）

> **蓝图来源**：[2026-08-08 调研报告](../reports/2026-08-08-research-pkm-obsidian-blueprint.md)；SiYuan 源码证据锚点见 `test/pkm-research/D-siyuan-kernel.md` §10。
> **License 纪律**：SiYuan（AGPL）/Logseq（AGPL）仅借鉴思路，全部逐行自研；解析器用 goldmark（MIT）扩展。
> **与 Vault 重设计的关系（B-4，评审修订）**：「文件系统即真相源」仅适用 backend=local；SP1 引入 backend 维度后，§V 子模块契约（R-1~R-6：frontmatter 分区/trash/SyncEngine 等）按 backend 分别适用——team 库无文件概念，PG 即真相源。
> **2026-08-08 深入评审**：[评审报告](../reports/2026-08-08-review-sp1-knowledge-blueprint.md)——B-1 合入 §S3、B-2 合入 §S6、B-3 合入 §S7、N-1~N-3 合入 §S4/§S5。

### S1. 架构总览

```
个人 Vault（local，文件真相源）          团队库（team，PG 真相源）
  .md + frontmatter                       knowledge_documents.content_text
        │ 同一条解析管线                            │
        ▼                                          ▼
┌──────────────────────────────────────────────────────────┐
│ 块解析管线 blockparse（纯函数：markdown → AST → []Row）    │
│   产出：BlockRow[]（块） + RefRow[]（引用边） + 词法/向量物料 │
└──────────────────────────────────────────────────────────┘
        │ 整文档事务（删了重插，零孤儿边）
        ▼
PG 统一索引层：knowledge_blocks / knowledge_block_refs（+ 既有 chunks/FTS/links）
        │
        ▼
统一链接索引服务（内存图 + 版本号）── WS 增量推送 ──→ 前端各视图（图谱/反链/关联区）
```

**核心哲学（SiYuan 借鉴）**：源数据（文件/PG 内容）与派生索引严格分离，索引可全量重放重建；解析管线为**纯函数**（tree→[]Row），一次遍历产出全部索引行。

### S2. 块模型

| 概念 | 定义 |
|------|------|
| Block | Markdown 文档经 AST 切分的最小可引用单元：heading / paragraph / list_item / code_block / blockquote / table / math / frontmatter 不切块（属文档元数据） |
| 块 ID | 显式锚点 `^uuid`（行尾，Obsidian 语法）；未锚块使用**派生定位**（见下） |
| 派生定位 | heading 块 = 标题路径（`h1/h2/h3` 文本序列，改名即重解析，不存死 ID）；非标题未锚块 = 文档内 ordinal + content_hash 前缀（仅内部索引用，不可被 `[[#^]]` 引用） |
| 惰性锚点回填 | 当写入路径发现引用指向未锚块时，经 VaultFiler（local）/团队内容写路径（team）向源文本行尾追加 ` ^<uuid7>`；幂等（已有锚点跳过）；同一文件多次解析锚点稳定 |

**`knowledge_blocks` 表**（新，Raw SQL DDL 迁移 20261203——2026-08-08 修订：弃 Ent Schema，见下）：

| 列 | 类型 | 说明 |
|----|------|------|
| id | TEXT | 块 ID（显式锚定后 = 文本中 `^anchor`；未锚块存储层生成随机 hex） |
| collection_id | TEXT FK | 所属库（REFERENCES knowledge_collections，域内 TEXT id 一致） |
| doc_id | TEXT FK | 所属文档（ON DELETE CASCADE） |
| ordinal | int | 文档内序号（同文档唯一） |
| kind | text | heading/paragraph/list_item/code_block/blockquote/table/math |
| anchor | text nullable | 显式 `^anchor`（未锚为 NULL） |
| heading_path | text[] nullable | heading 块的标题路径 |
| content_hash | text | 块内容 hash（索引 diff 用） |
| text_excerpt | text | 前 200 字符（反链上下文/图谱标签用） |
| promoted_from / promoted_to | text nullable | 晋升谱系对（US-27，无 FK：源可删除） |

UNIQUE(doc_id, ordinal)；UNIQUE(collection_id, anchor) WHERE anchor IS NOT NULL（部分唯一索引，DDL 迁移建）。

> **2026-08-08 实施修订（SP1-B 裁决 D1~D6）**：本表与 `knowledge_block_refs` 弃用 Ent Schema + uuid 的原表述，改为 **Raw SQL + TEXT id + DDL Migration Registry 版本化迁移**（`sql/migrations/20261203_knowledge_blocks.sql`）。理由：① knowledge 域（collections/documents/chunks/links）全为 TEXT id Raw SQL，uuid FK 类型不匹配物理不可建；② blocks/refs 是派生索引表（可全量重放、整文档删了重插的批量写），Ent 类型安全 CRUD 无用武之地；③ 部分唯一索引/FK 级联语义超出 Ent 表达能力；④ 版本化 DDL（DB-R4）比 EnsureKnowledgeSchema 隐式演进更可控，SP1-G/H 重建与晋升直接受益。

### S3. 双链解析管线（blockparse 包）

**包**：`internal/knowledge/blockparse/`（新）。goldmark AST 扩展解析 `[[...]]`/`![[...]]`（goldmark 不原生支持 wikilink——实现 inline parser 扩展，或用前置正则切分 + AST 定位两种方案 spike 后定，倾向 AST 扩展保证位置准确）。

**语法矩阵**（全部产 RefRow）：

| 语法 | 边类型 | 目标解析 |
|------|--------|---------|
| `[[doc]]` / `[[doc\|alias]]` | ref | 文档键（rel_path 无扩展名 / 标题 / 别名，按 Obsidian 最短路径唯一匹配） |
| `[[doc#heading]]` | ref | 文档 + heading_path 定位 heading 块 |
| `[[doc#^anchor]]` | ref | 文档 + anchor 定位块；未锚块被引用触发回填 |
| `![[...]]` | embed | 同左，边类型 embed（嵌入语义） |

**纯函数契约**：`Parse(docKey, markdown []byte) (blocks []BlockRow, refs []RefRow, err error)`——无 IO、无全局态；RefRow 含 `raw_target`（原始目标文本，dangling 解析必需）、`context`（引用上下文 ±50 字符）、`syntax`（四语法之一）。

**解析为两阶段**：① `Parse` 产出行（纯函数）；② **Resolver** 将 `raw_target` 解析为 `dst_doc_id/dst_block_id`（查统一索引；查不到 → dst NULL + dangling 标记）。两阶段分离保证：目标后创建时重跑 Resolver 即可复活 dangling，无需重解析源文档。

**跨 Collection 解析规则（B-1，评审修订）**：`[[doc]]` 文档键匹配按确定次序——① 同 collection 命中优先；② 当前用户**可见** collection 内最短路径唯一匹配；③ 多义时按（collection 创建序、路径字典序）取首并在 RefRow 记 `ambiguous=true`；**不可见 collection 不参与匹配**（防文档名泄漏）。显式跨库链接语法（如 `[[库名/路径]]`）后置到 SP2 编辑器定案。

### S4. refs 物化与整文档重建

**`knowledge_block_refs` 表**（新）：

| 列 | 类型 | 说明 |
|----|------|------|
| id | bigserial | |
| collection_id | TEXT FK | （REFERENCES knowledge_collections ON DELETE CASCADE） |
| src_block_id | TEXT FK | 引用源块（REFERENCES knowledge_blocks ON DELETE CASCADE） |
| dst_doc_id | TEXT nullable | 解析后的目标文档（ON DELETE SET NULL） |
| dst_block_id | TEXT nullable | 解析后的目标块（ON DELETE SET NULL → dangling） |
| raw_target | text | 原始目标文本（dangling 时唯一线索） |
| alias | text | 引用别名（`\|alias`，无别名为空串） |
| edge_type | text | ref / embed |
| context | text | 引用上下文片段 |
| ambiguous | bool | 跨库多义取首时 Resolver 记 true（B-1 评审修订） |
| created_at | timestamptz | |

索引：`(dst_block_id)`、`(dst_doc_id)`、`(collection_id, raw_target)`（dangling 聚合/复活扫描）。

**整文档重建**（SiYuan `deleteRefsByPathTx` 语义）：文档变更时 `PostgresExecInTx`（knowledge 域 Raw SQL 事务入口）内——按 doc 删除全部 blocks（FK 级联删 src refs；指向旧块的 dst 引用由 `ON DELETE SET NULL` 自动转 dangling 保 raw_target）→ 插入新块行 → 重跑 Resolver 插入新 refs。块级 content_hash diff 仅供跳过 embedding/FTS 重算，**refs 不做 diff 一律重插**（一致性优先，写放大小）。实现：`internal/data/knowledge_blocks.go` `ReplaceDocBlocks`（biz 端口 `biz/knowledge.BlockIndexRepo`，Stability:evolving）。

**与既有 `knowledge_links`（文档级三轨）关系**：SP1 并存——显式 explicit 轨改由块级 refs **投影**生成（同文档去重聚合），保持 G5 图谱/关联区消费不变；entity/semantic 轨不动。SP3 图谱再迁移到块级直接投影。**投影权重规则（N-3，评审修订）**：同（src_doc, dst_doc）对的多条块边聚合为一条 explicit 文档边，`meta.weight = 块边数`（G5 图谱 size ∝ 被引数直接受益）。

> **2026-08-08 实施修订（SP1-C 建成态）**：
> ① **Resolver 文档键物化**：`knowledge_documents` 加 `title TEXT NOT NULL DEFAULT ''` / `aliases JSONB` 两列（DDL 迁移 20261204；`EnsureKnowledgeSchema` fresh 形态同步），由 `blockparse.ParseDocMeta` 从 frontmatter 提取后随块索引管线经 `BlockIndexRepo.UpdateDocLinkKeys` 回写——Resolver 候选查询（`ResolveIndex.ListResolveCandidates`）直接读物化列，不重复解析 frontmatter。
> ② **权重落实列**：N-3 的 `meta.weight` 落成 `knowledge_links.weight INT NOT NULL DEFAULT 1` 实列（同迁移）；`ReplaceLinks` 改写语义为 ON CONFLICT (doc_id, target_doc_id, link_type) 刷新 weight/context，存量行默认 1 与迁移前「一条边一票」一致无需回填。
> ③ **自引用存储契约**：Resolver 对自文档引用（`[[#^a]]`/`[[#H]]` 及按名引用回自身）只产 `DstSelfOrdinal`（目标块未落库无 ID），`ReplaceDocBlocks` 事务内按本次插入的 ordinal→ID 映射回填 `dst_block_id`；ordinal 越界属契约违例 → CodeBadRequest 整事务回滚。
> ④ **写路径编排收口**：`Usecase.RebuildBlockIndex`（`biz/knowledge/block_pipeline.go`）统一 parse→resolve→persist→explicit 投影四步，vault 同步 / 移动入链修复 / 粘贴文本与 agent knowledge_write 摄取同走此入口；失败降级记日志不回滚主流程（最终一致）。旧 `link_parser.go`（正则 explicit 重建）已删除。
> ⑤ **可见集合推导**：后台索引无「当前用户」，取文档所在 workspace 的全部集合作为可见集（`visibleCollectionIDs`）；读侧片段级权限属 SP5。

### S5. 统一链接索引服务 + WS 增量

**`internal/biz/knowledge` 新增 `LinkIndex`**（进程内内存图）：

- 结构：邻接表（`map[blockID][]edge` 正向 + 反向）+ 版本号（单调递增）
- 加载：启动时从 `knowledge_block_refs` 全量构建；万级边毫秒级
- 更新：解析管线事务提交后 apply 增量（add/remove 边），版本号 +1
- 推送：经 `event.Bus` 发 `knowledge.graph.delta` 事件（`{added: [...], removed: [...], version}`），前端图谱/反链订阅增量更新（复用既有 WS 链路）
- 消费方：块级反链查询直接读内存图（O（度数）），落库查询为兜底

**可见性投影**：边带 scope（由 src/dst 块所属 collection 的 backend + 租户推导）；查询时按当前用户可见 collection 集过滤。SP1 仅落地 scope 字段与基础过滤，完整片段级权限属 SP5。

**部署约束（N-1，评审修订）**：LinkIndex 为单进程内存图（当前 admin 单进程部署满足）；多副本化时需改事件广播保持副本一致，届时另立 ADR。

**事件分级（N-2，评审修订）**：`knowledge.graph.delta` 按 AS-EVT-01 登记为 Informational（可从 `knowledge_block_refs` 全量重放重建，丢失可容忍）。

> **As-built（2026-08-08，D-1/D-2）**：事件载体为 v2 `SystemNoticeEvent`（noticeType=`knowledge.graph.delta`，meta={event_type, version, added, removed}，边负载含 collection/src/dst/raw_target/edge_type/context/ambiguous），service 适配器 `knowledgeGraphDeltaPublisher` 经 v2Bus 广播；`NewKnowledgeService` 构造期创建 LinkIndex 并接线共享 uc；启动全量加载经 app.go readiness 门控后后台 goroutine（失败降级不阻塞）；`DeleteDocument` 接 `RemoveDoc`（集合删除归 SP1-G）。时序契约：`ApplyDocDelta` 不区分初次/重建，目标文档每次 apply 均将入向块边转文档级（严格镜像 `ReplaceDocBlocks` 先删后插的 FK SET NULL 语义）。
>
> **As-built（2026-08-08，SP1-E 读路径）**：`biz/knowledge/backlink.go` 落地 S5 消费方——读路由 `LinkIndex.Loaded()` 已加载 → 直读内存图（O(度数)），启动窗口未加载 → `BlockLinkReader` 落库兜底（data `refEdgeSelect` 公共 SELECT+JOIN：`ListBacklinksByBlock/ByDoc/ListDanglingEdges` + `GetBlockOwnerDoc` 支撑块级路径权限断言），双端口未接线 → 空降级；`Loaded` 门（LoadAll 置位）修复启动窗口读空图误判。源文档显示名经 `DocNameReader.ListDocumentNames` 批量解析（rel_path 优先、source 兜底，失败留空降级不阻塞）。输出确定性契约：反链按 (SrcDocID, SrcBlockID) 字典序；dangling 组间 ref_count 降序 + raw_target 字典序。service 双 RPC（`knowledge_backlink.go`）沿用 C-01 `assertCollectionAccess` 跨租户断言；装配经 `ProvideKnowledgeUsecase` 类型断言自动接线。可见性：当前 API 无用户上下文，visible=nil（片段级权限属 SP5）。

### S6. 团队库（team 后端）

| 项 | 设计 |
|----|------|
| Schema | `knowledge_collections` 加 `vault_backend TEXT NOT NULL DEFAULT 'local'`（local/team），DDL 迁移 |
| 文档本体 | team 库复用既有 PG 内容存储（`knowledge_documents.content_text`，Collection 时代既有路径）；无 `root_path`（约束调整为：backend=local 时 root_path 必填唯一，team 时为空） |
| 解析 | 与 local 同管线：content_text → blockparse → 块/边落同一套表 |
| 写入 | SP1 仅经既有「粘贴文本入库」/agent write 路径；协同编辑器属 SP2 |
| 写路径契约（B-2，评审修订） | SP1 阶段为单写者语义（无并发编辑）；审计沿用 activities（who/when/what，对齐 R-6 惯例）；多人并发编辑的冲突协议（行版本/etag）**后置 SP2 编辑器定案，SP1 不实现** |
| 同步/监听 | team 库无 SyncEngine（PG 即真相源，无文件监听） |

### S7. 晋升（US-27）与删除同步（US-28）

**`PromoteBlocks(ctx, blockIDs, targetTeamCollectionID)`**（biz usecase，`Data.ExecInTx`）：

1. 校验目标库 backend=team 且当前用户有写权限
2. 为每个源块在团队库克隆：目标文档（按 rel_path 同名查找或新建）+ 块行（**新块 ID**，`promoted_from=源块ID`；源块回写 `promoted_to=新块ID`）；**目标文档派生索引同事务重放**（chunk→embed（目标库有语义层时）→FTS，晋升完成即可检索——B-3 评审修订）
3. 源块 refs 处理：引用私有块的 → 返回级联提示清单（`cascade_candidates`）；未一并晋升的目标在团队侧 refs 落 `raw_target` + dangling + `meta.private_external=true`
4. 审计：activities 表记 who/when/what（对齐 knowledge_write R-6 审计惯例）
5. 返回 `{created_blocks, cascade_candidates, lineage}`

**删除同步**：team 块删除 → `knowledge_block_refs.dst_block_id` FK `ON DELETE SET NULL`（保 raw_target 转 dangling）；指向它的边删除走显式同事务 DELETE（区分「边消失」与「转 dangling」：src 块删除 = 边级联删除；dst 块删除 = 边转 dangling）。WS 增量事件携带两类变更，前端分别处理（摘除边 / 节点灰显）。

> **2026-08-08 As-built 修订注记（SP1-G 落地偏差）**：
> 1. **块克隆机制**：不直接 INSERT 块行，改为「目标文档全文尾部追加 + 块级索引重放」——`knowledge_blocks` 是派生索引（整文档删了重插语义），直插块行会在下次重放丢失；追加原文后重放让新块自然生成，谱系按「尾部 N 块按序对应」回写（`promoted_from`/`promoted_to` 单事务）。
> 2. **事务边界**：无 `Data.ExecInTx` 单一大事务，逐目标文档原子（文档写 + 块重放各自事务）；chunk→embed→FTS 重放在 service 层事务后执行（embed 走外部 API 不可入库事务），单文档失败降级 `status=error` 不回滚晋升——最终一致，重建索引自愈。
> 3. **`meta.private_external` 不建**：引用私有块的目标经可见性过滤自然落 dangling（`raw_target` 保留即占位语义），无需额外标记列。
> 4. **审计通道**：走 `knowledgeFlow` 流程日志（step `knowledge.block.promote`，K1 start/done + K2 error），非 activities 表。
> 5. **「显式同事务 DELETE」实现形态**：由 FK 约束同事务级联等价实现（`src_block_id ON DELETE CASCADE` = 边消失；`dst_* ON DELETE SET NULL` = 转 dangling），语义不变。
> 6. **集合删除图同步（扩展）**：`LinkIndex.RemoveCollection` 补齐整库删除的内存图同步（源边消失 + 外部入边转 dangling），`DeleteCollection` 发布 WS 增量——原文仅述块/文档级，此处扩展到集合级。

### S8. Proto 契约（`api/kratos/knowledge/v1/knowledge.proto` 增补）

| RPC | 路径 | 说明 |
|-----|------|------|
| `ListBlockBacklinks` | `GET /v1/knowledge/blocks/{block_id}/backlinks` | 块级反链（含 context、src 文档/块信息、direction）；另支持 `doc_id` 参数聚合文档全部块反链 |
| `ListDanglingLinks` | `GET /v1/knowledge/collections/{id}/dangling-links` | 悬空链列表（raw_target 聚合 + 引用计数，「未创建笔记」视图） |
| `PromoteBlocks` | `POST /v1/knowledge/blocks/promote` | body：`{block_ids[], target_collection_id}`；返回 created_blocks/cascade_candidates/lineage |
| `RebuildKnowledgeIndex` | `POST /v1/knowledge/collections/{id}/rebuild-index` | 幂等；异步任务，进度走既有摄取进度 WS 事件模式（EP-KN-02） |

`KnowledgeCollection` proto + `vault_backend` 字段。

### S9. RebuildIndex（US-29）

流式重建：按文档分批（每批一事务，删块→重解析→重插），`sync_state` 加 `rebuilding` 态（期间检索走旧 chunks/FTS——旧数据最后一批才清，降级可用）；幂等（中断重跑继续）；进度事件复用 EP-KN-02 模式。

> **2026-08-09 事故根修**：`ListDocuments` 是摘要投影（SELECT 不含 `content_text`），`RebuildCollectionBlockIndex` 直接拿分页结果重建会解析出 0 块 0 边，`ReplaceDocBlocks` 删旧插空 = **全库块索引静默清空**。修复：分页仅取 ID 列表，逐文档 `GetDocument` 取回完整正文再进重建管线（`rebuild_index.go`）；测试 fixture 同步改为摘要投影 + `docGetFn` 回源，防 mock 假绿回归。

### S10. 关键决策记录（ADR 摘要）

| # | 决策 | 理由 | 放弃方案 |
|---|------|------|---------|
| SP1-ADR-1 | 惰性锚点 + heading 路径派生定位 | Obsidian 语义兼容，不侵入未引用文本 | SiYuan 全块持久 ID（写入侵入大） |
| SP1-ADR-2 | 解析/解析目标两阶段分离（Parse 纯函数 + Resolver） | dangling 复活无需重解析源文档 | 单阶段（目标后创建需全量重解析） |
| SP1-ADR-3 | refs 整文档重插不做 diff | 零孤儿边、一致性极简；块 hash diff 仅用于跳 embedding/FTS | 行级 diff（边界 case 多） |
| SP1-ADR-4 | 客户端不存团队图谱副本，走 WS 订阅 | 避免 CRDT/离线合并全复杂度；离线副本后置评估 | Colanode 式本地副本 |
| SP1-ADR-5 | 晋升=复制非移动 + 谱系对 | provenance 不可变（Collaborative Memory 学术模型）；个人源不消失 | 移动式（破坏个人上下文） |

### S11. 影响面（改 SP1 必须同步谁）

- **G5 图谱**：数据源仍走 `knowledge_links`（块级投影生成，SP1 不变消费方式）；WS `knowledge.graph.delta` 为增量新通道（SP3 接入）
- **关联区（KnowledgeDocDetail）**：explicit 轨改由块级投影——数据语义不变，组件不改
- **Agent 工具族**：knowledge_write 写入路径接 blockparse；navigate/grep 不受影响
- **实体共现/语义轨**：不动
- **65-module-cross-reference-full.md**：SP1 实施后同步 knowledge 模块卡片（新增 blocks/refs 表、LinkIndex、blockparse 包）

---

## SP2. 编辑器与笔记体验（深空液态玻璃工作台，2026-08-10）

> **需求**：见 [37-knowledge.md §子模块：编辑器与笔记体验（SP2 需求）](./37-knowledge.md#子模块编辑器与笔记体验sp2-需求2026-08-10)（US-24~US-30、FR-SP2-1~10）。
> **用户裁决（2026-08-10）**：Obsidian 级笔记能力为目标；UI 推翻 Tab 管理后台为 Obsidian 工作台；编辑器选型 **CodeMirror 6 Live Preview**（弃 TipTap——源文本即真相源，与 SP1「文件/PG 内容为权威源」哲学一致）；A1（骨架+编辑器+双链）+A2（视觉全套）一轮交付；视觉=液态玻璃+科幻+炫酷。
> **范围纪律**：**纯前端重构，后端零改动**——全部数据走既有 API（`features/knowledge/api.ts`：tree/content/CAS 保存/反链/出链/dangling/图谱/新建目录文档均已就绪）。

### SP2-1. 架构总览

```
KnowledgePage.vue（重写，薄壳）
  └─ KnowledgeWorkbench.vue（工作台根，装配三栏 + 浮层 + 全屏图谱）
       ├─ WorkbenchTopBar        顶栏：Vault 切换 / ⌘O / ⌘K / 图谱 / 设置浮层入口
       ├─ WorkbenchSidebar（左）  Vault 树（复用 KnowledgeVaultTree 内核，玻璃换肤）
       ├─ WorkbenchTabs（中）     笔记标签页条 + 活动笔记编辑器/预览/空态（RingCarousel + 流体 GlowButton 主 CTA「新建笔记」，emit `create-note` 由父级弹命名对话框）
       │    └─ NoteEditor.vue     CM6 编辑器（Live Preview + wikilink 补全/芯片）
       ├─ WorkbenchSidePanels（右）五面板：反链/出链/大纲/属性/局部图谱
       ├─ QuickSwitcher.vue       ⌘O 快速切换（液态玻璃浮层）
       ├─ CommandPalette.vue      ⌘K 命令面板（液态玻璃浮层）
       └─ KnowledgeGraph3D        全屏覆盖模式（既有组件，顶栏进入、ESC 退出）
```

**数据流**：`useKnowledgeWorkbench`（composable，唯一状态机）持有——Vault 列表/当前 Vault、树选中、打开标签页数组 `tabs[]`（docId/relPath/title/dirty/saving/baseHash）、活动 tabId、右栏联动数据。组件全部经 props/emits 与该 composable 交互，不各自拉数（Obsidian MetadataCache「单一真相源」哲学的前端映射）。右栏五面板输入仅为 `activeTab`，活动标签切换/保存完成时统一联动刷新。

### SP2-2. 视觉设计令牌（深空液态玻璃）

新增 SCSS 令牌层（`web/src/css/deep-space.sass`，sass 缩进语法对齐项目惯例），仅作用于知识库工作台作用域（`.kb-workbench` 根类名隔离），不污染全局 Quasar 主题。

> **2026-08-11 增强轮（P0-1 色彩同源）**：初版硬编码色值已废弃，kb 令牌全部改为消费全局 Tech Night 变量（`app-theme.sass` / `theme/_css-vars-dark.sass`），系统主题切换时工作台自动跟随。下表为当前生效映射：

| 令牌 | 值（同源映射） | 用途 |
|------|-----|------|
| `--kb-bg-deep` | `var(--canvas-base)` | 深空底 |
| `--kb-bg-glass` | `var(--glass-surface)` | 玻璃面板底色 |
| `--kb-bg-glass-strong` | `var(--glass-elevated)` | 强玻璃底（浮层/确认框） |
| `--kb-glass-border` | `var(--glass-border)` | 高光描边 |
| `--kb-glass-border-strong` | `var(--glass-border-hover)` | 悬停描边 |
| `--kb-glass-highlight` | `rgba(255, 255, 255, 0.08)` | 顶部高光带 |
| `--kb-accent-cyan` | `var(--color-accent)` | 主霓虹（青） |
| `--kb-accent-violet` | `var(--color-neon-violet)` | 辅霓虹（紫，极光光斑） |
| `--kb-accent-cyan-dim` | `color-mix(in srgb, var(--color-accent) 35%, transparent)` | 主霓虹淡化 |
| `--kb-text-primary` | `var(--tech-text-primary)` | 主文本 |
| `--kb-text-dim` | `var(--tech-text-muted)` | 次文本 |
| `--kb-danger` / `--kb-warn` | `var(--color-danger)` / `var(--color-warning)` | 危险/警告 |
| `--kb-radius-glass` | `14px` | 玻璃圆角（全局无对应，本地保留） |
| `--kb-blur` | `var(--glass-blur-default)` | backdrop blur |

玻璃质感实现：`background: var(--kb-bg-glass); backdrop-filter: blur(var(--kb-blur)) saturate(1.4); border: 1px solid var(--kb-glass-border);` + 镜面断面（box-shadow 顶缘反光 + 底缘暗线，见 §SP2-12 三层液态效果）。极光光斑 = 三个固定定位径向渐变色团（cyan/violet/teal，视口相对 46/40/34vw，`filter: blur(120px)`，慢速漂移，不参与交互）。

> **2026-08-11 美化提升轮（U1~U3，调研 `docs/reports/2026-08-11-research-ui-liquid-glass-visual.md`）**：
>
> - **U1 色彩阶梯**：新增透明度阶梯令牌——文本 `--kb-text-secondary/faint/disabled`（68%/42%/28%）、边框 `--kb-line-hairline/emphasis`（7%/16%）、行态 `--kb-bg-hover/active`、辉光 `--kb-accent-glow`，全部 color-mix 派生自同源令牌、跟随主题；`.kb-portal` 菜单行 hover/active 改亮度阶梯。
> - **U1 渐变边缘光**：`kb-gradient-edge` mixin（顶亮 18% → 中暗 5% → 底缘微青 14% 的 1px 渐变环，mask-composite 合成只画边缘），`.kb-glass::before` 承载并取代原顶部高光线，平边框降为 `--kb-line-hairline` 兜底；`kb-glass-surface` 补左缘反光 inset（三向受光）。
> - **U1 焦点环/选区**：双环焦点（`0 0 0 2px 背景 + 0 0 0 4px accent 60%`，`:where()` 零特指度、`:focus-visible` 仅键盘）；`::selection` accent 32% 透明底，两作用域共享。
> - **U2 噪点纹理**：`.kb-workbench::after` feTurbulence 单色颗粒（data-URI SVG，160px 平铺，`mix-blend-mode: overlay` + opacity 0.04，z-index 2 盖内容上方）——防色带、掩 blur 廉价感；浮层 PaletteModal z-index 30 不受影响。
> - **U2 光晕增强**：极光由 480px 双团升级为视口相对三团 + `kb-aurora-drift` 慢速漂移（26/32/38s 交错，仅 transform，reduced-motion 统一降级）。
> - **U3 wikilink 芯片**（`NoteEditor.vue` kbTheme）：6px 圆角 + accent 10% 底；hover 底色升 20% + accent 微光 + 150ms 过渡；断链虚线边 + opacity 0.6 降级，dangling hover 独立态（无辉光、opacity 0.8）。

### SP2-3. 特效组件（5 个自研，`components/knowledge/effects/`）

| 组件 | 职责 | 关键技术 |
|------|------|---------|
| `GlassPanel.vue` | 液态玻璃容器（面板/浮层基座） | slot 包装 + 上述令牌 + 可选 `glow` 呼吸辉光；**P0-2 三层液态效果**（§SP2-12）：SVG 折射光纹 + 镜面断面 + 指针追随高光 |
| `ParticleField.vue` | 深空粒子背景 | Canvas 2D；粒子数按设备分级（`navigator.hardwareConcurrency`/`deviceMemory`：高 120 / 中 60 / 低 0）；鼠标 120px 斥力 + 近距连线（<90px 透明度渐隐）；`requestAnimationFrame` 自循环，页面不可见（`document.hidden`）时停帧。**2026-08-11 V2 增强**：星光闪烁（透明度正弦振荡 [0.25,0.85]，~3.9s 周期）+ 流星（4~8s 随机间隔，300ms 生命周期，淡入淡出渐变尾迹，屏上至多 1 颗）+ 视差双层（远层 depth 0.35 小且慢 / 近层 depth 1，鼠标归一化位移 × 深度反向偏移，满幅 18px）；纯函数层抽至 `features/knowledge/particles.ts`（seedField/twinkleAlpha/createMeteor/meteorProgress/meteorHead/parallaxOffset/nextMeteorDelay，全单测） |
| `TiltCard.vue` | 3D 倾斜卡片 | mousemove 求相对中心偏移 → `rotateX/Y`（±8° 上限）+ 移动高光（radial-gradient 跟随）；hover 跟随 120ms ease-out；mouseleave 弹簧回正 420ms overshoot 缓动 `cubic-bezier(0.34,1.56,0.64,1)`（V3，回弹感）；零尺寸守卫（jsdom/隐藏态不倾斜） |
| `GlowButton.vue` | 辉光磁吸主按钮 | hover 辉光晕（box-shadow 双层 cyan）+ 鼠标磁吸位移（≤6px，transform translate）；active 涟漪；**V3**：非 ghost 变体注入低透明度流体色团（`kb-fluid-blobs`，opacity 0.22，不抢 label 可读性） |
| `RingCarousel.vue` | 空态 3D 聚焦环 | N 张卡片 `rotateY(i*θ) translateZ(r)` 环形排布；**2026-08-11 V1 重写**：弃 CSS keyframes，改 JS rAF 帧循环驱动（旋转态自持，不走 Vue 响应式）——逐帧计算各卡与正面角差输出 `--focus`（cos 衰减），联动 opacity/blur/scale 衰减 + 聚焦卡旋转渐变光环（`@property --kb-halo-angle`）；交互全集：拖拽（0.35°/px）/滚轮（0.12°/px）/惯性（阻尼 2.2/s）/最近卡吸附（10/s）/hover 暂停/click 抑制（拖拽 >6px）；卡片为流体玻璃卡（`kb-fluid-blobs`：cyan/violet 双径向渐变反向漂移 + 42px 自模糊漫射光，宿主 `overflow:hidden` 裁剪）；点击进入笔记 |

> **V1~V3 增强轮（2026-08-11，抖音流体卡/粒子/3D 旋转调研落地）**：调研方案 `docs/reports/2026-08-11-research-ui-fluid-3d-particle-plan.md`。流体色团 mixin（`kb-fluid-blobs`）宿主纪律：仅装饰性玻璃卡使用，宿主必须 `overflow:hidden`；叠序 元素玻璃底 < 流体层(::after, z-index -1) < 内容 < 渐变边缘(::before)。

**降级契约（FR-SP2-10）**：`prefers-reduced-motion: reduce` 时——粒子不渲染、TiltCard 退化为静态卡、RingCarousel 退化为纵向列表、GlowButton 仅保留颜色 hover。统一经 `useReducedMotion()`（matchMedia 封装）判定，各组件消费。

### SP2-4. `useKnowledgeWorkbench` 状态机

`web/src/features/knowledge/useKnowledgeWorkbench.ts`（composable 单例模式——模块级状态 + 工厂函数返回共享实例）：

```ts
type WorkbenchTab = {
  docId: string; relPath: string; title: string;
  mode: 'edit' | 'preview';          // 非 markdown 文档恒 preview
  dirty: boolean; saving: boolean;
  baseHash: string;                  // CAS（UpdateDocumentContent）
  content: string;                   // 编辑缓冲区
};
```

| 动作 | 语义 |
|------|------|
| `openDoc(doc)` | 已在 tabs → 激活；否则 `getDocumentContent` 取回（content + base_hash）后推入 tabs 并激活；非 markdown MIME → mode=preview 锁死 |
| `closeTab(id)` | dirty 时弹确认（保存/放弃/取消）；关闭后激活相邻 tab |
| `saveTab(id)` | `updateDocumentContent({id, content, baseHash})`；CodeConflict → 既有「留双份 + 警告」语义（重新拉取远端 hash 刷新 baseHash，提示冲突副本已留存）；成功刷新 baseHash、清 dirty |
| `onDocRemoved(docId)` | 树/列表删除事件 → 关闭对应 tab（无确认，数据已删） |
| `onDocRenamed(doc)` | 同步 tab 的 relPath/title |
| `activeTab` / `sidePanelData` | computed；活动切换或保存成功后触发右栏五面板并行拉取 |

树与搜索沿用既有 composable 数据源（tree 懒加载/即时搜索），workbench 只订阅其选中事件调 `openDoc`。

### SP2-5. CodeMirror 6 编辑器（`NoteEditor.vue`）

**依赖新增**：`codemirror`（metapackage）、`@codemirror/lang-markdown`、`@codemirror/state`、`@codemirror/view`。

**装配**：
- `EditorState`：`markdown()` + 高亮 + 深空主题（`EditorView.theme` 定制：背景透明、光标 cyan、选区 rgba cyan 0.15、语法色 token 映射调色板）+ 下列扩展。
- 双向同步：`updateListener` 文档变更 → 写回 `tab.content` + `dirty=true`（300ms 防抖标脏，避免每 keystroke 触发响应式全链）；外部重建（如切换 tab）时 `dispatch(replace)` 注入。
- 保存接线：`Ctrl/Cmd+S` → `saveTab`；自动保存不做（CAS 语义下显式保存更安全，与 Obsidian 差异为有意决策——避免无 hash 守卫的后台写覆盖冲突窗口）。

**Live Preview（行级装饰，`ViewPlugin` + `Decoration`）**：
- 规则集：非光标行——ATX 标题按级别放大/加粗并淡化 `#` 标记；`**bold**`→粗体、`*em*`→斜体、`~~del**`→删除线（语法标记透明度 0.35）；代码块行底色 + 圆角背景；引用行左边框 cyan；列表 marker 替换为 `•`/`◦`。
- 光标行判定：以 `selection.main` 所在行集合为准，`selectionSet` 或 `docChanged` 时重算装饰（增量：`DocChanged` 时仅重映射受影响区间，Obsidian 同款策略）。
- 只读预览模式：`EditorState.readOnly` + 全文装饰（无光标行回退）。

### SP2-6. wikilink 写作（`[[` 补全 + 芯片 + 跳链）

**补全**：`@codemirror/autocomplete` 自定义 source——检测 `[[` 前缀（正则 `\[\[([^\]|#]*?)(?:#([^\]|]*))?$`），150ms 防抖后调即时搜索数据源（复用统一搜索即时区 `searchVaultFiles` 同一路径，当前库文件名候选）；候选项展示文档名 + 所在目录 dim 后缀；Enter/Tab 确认插入 `[[name]]`（已有 `\|alias`/`#heading` 后缀片段保留）。**P2-5 标题段补全**：`[[target#partial` 时切换为标题候选（`getHeadings(target)` 由容器提供——已打开 tab 的大纲解析），确认插入完整 `[[target#heading]]`；跳链时 heading 随 `open-doc` 事件上抛，容器打开文档后按标题文本匹配滚动定位（§SP2-15）。

**链接芯片（Decoration.replace + Widget）**：`[[target]]`/`[[target|alias]]`/`[[target#heading]]` 在**非光标行**替换为 `WikiLinkWidget`（chip 样式：胶囊玻璃底 + cyan 文本 + 链接图标；展示文本 = alias ?? target）：
- `resolveWikiTarget(target)`：查当前 tabs 缓存 + 即时搜索数据源判定存在性；不存在 → chip 加 `dangling` 类（灰显 + 虚线边框 + tooltip「目标未创建」），与浏览视图 dangling 视觉一致。
- 点击行为：widget `mousedown`——Ctrl/Cmd+点击（编辑态）或单击（预览态）→ `openDoc` 跳链；目标不存在时点击 = 经 `createVaultDocument` 新建并打开（Obsidian 语义）。

### SP2-7. ⌘O 快速切换 / ⌘K 命令面板 / Ctrl+Shift+F 全库搜索

两浮层共用 `PaletteModal`（`GlassPanel` + 居中模态结构，背景遮罩 `backdrop-filter: blur(6px) brightness(0.6)`）：

| 浮层 | 数据源 | 行为 |
|------|--------|------|
| `QuickSwitcher.vue`（⌘O/Ctrl+O） | 即时搜索（文件名，150ms 防抖，复用浏览视图同一函数） | ↑↓ 导航、Enter 打开（`openDoc`）、ESC 关闭；结果项 = 文件名 + 路径 + 库名徽标 |
| `CommandPalette.vue`（⌘K/Ctrl+K） | 命令注册表（静态数组，见下） | 模糊过滤（子序列匹配 + 打分排序：前缀 > 连续子串 > 散列）+ **P2-6 别名/MRU**（§SP2-16）；↑↓/Enter/ESC；命令行右侧展示快捷键提示 |
| `SearchPanel.vue`（Ctrl+Shift+F，**P1-3 新增**） | `SearchKnowledge` RPC（内容检索，300ms 防抖 + seq 竞态守卫） | 结果项 = 文档名 + 路径 + 相关度分数 + 命中片段（匹配词居中截取 160 字符）；↑↓/Enter/ESC；关闭时清零状态（§SP2-13） |

**命令注册表**（初版 9 条 + SP2-8 粘贴文本入库，注册表模式便于扩展）：新建笔记 / 新建文件夹 / 保存当前笔记（Ctrl+S）/ 切换编辑预览（Ctrl+E）/ 打开图谱全屏（Ctrl+G）/ 切换 Vault（子列表二级选择）/ 重建当前库索引（调既有 `RebuildKnowledgeIndex` RPC）/ 粘贴文本入库 / 晋升到团队库（打开既有 KnowledgePromoteDialog）/ 关闭当前标签（Ctrl+W）。**P2-6**：每条命令注册英文/拼音别名（`aliases` 字段），空查询时 MRU（至多 3 条，localStorage `kb.command.mru` 持久化）置顶并显示 history 角标。

快捷键全局接线：workbench 挂载期注册 `keydown`（capture），输入框聚焦时 ⌘O/⌘K/Ctrl+Shift+F 仍可唤起、其余命令键放行。三浮层互斥（开任一关其余）。

### SP2-8. 右栏五面板（`components/knowledge/panels/`）

| 面板 | 数据源（除 P2-7 外全部既有 API） | 交互 |
|------|----------------------|------|
| `PanelBacklinks.vue` | `listBlockBacklinks(docId)` + `listUnlinkedMentions(docId)`（**P2-7**） | 块级分组（来源文档 → 上下文片段列表）；点击来源 → `openDoc`；dangling 组展示 raw_target + 计数；**未链接提及分组**（来源文档名 + 次数，点击跳转，§SP2-17） |
| `PanelOutlinks.vue` | `listDocumentLinks(docId)` | 显式/实体/语义分区（沿用既有关联区口径）；点击 → `openDoc` |
| `PanelOutline.vue` | 当前 tab `content` 前端解析（正则 ATX heading 树，H1-H6 缩进） | 点击 → 编辑器滚动定位（`EditorView.dispatch` 选区 + `scrollIntoView`）；内容变更 300ms 防抖重解析 |
| `PanelProperties.vue` | `content` frontmatter 区段（`^---\n...\n---`）YAML 键值只读解析（title/aliases/tags 高亮，其余通用键值） | 只读；无 frontmatter → 「无属性」空态 |
| `PanelLocalGraph.vue` | `listCollectionGraph` 拉全图后前端以当前 docId 为根 BFS（1-5 跳滑块） | 迷你 2D 力导向（轻量自研 canvas，≤200 节点；非 G5 3D 管线复用——迷你图求快不求炫）；点击节点 `openDoc`；「展开全屏」按钮进图谱覆盖模式并定位该节点 |

五面板折叠态持久化到 `localStorage`（`kb-panels-collapsed`）。无活动 tab → 整栏玻璃空态（图标 + 文案，非报错）。

### SP2-9. 文件结构（新增/改动清单）

```
web/src/
  css/deep-space.sass                          [新] 设计令牌
  features/knowledge/
    useKnowledgeWorkbench.ts                   [新] 状态机
    useReducedMotion.ts                        [新] 降级判定
    wikilink.ts                                [新] 补全 source / widget / resolve
    outline.ts                                 [新] 大纲解析（纯函数）
    frontmatter.ts                             [新] frontmatter 解析（纯函数）
    commands.ts                                [新] 命令注册表
    particles.ts                               [新，V2] ParticleField 纯函数层（闪烁/流星/视差/双层种子）
  components/knowledge/
    effects/GlassPanel.vue                     [新]
    effects/ParticleField.vue                  [新]
    effects/TiltCard.vue                       [新]
    effects/GlowButton.vue                     [新]
    effects/RingCarousel.vue                   [新]
    workbench/KnowledgeWorkbench.vue           [新] 工作台根
    workbench/WorkbenchTopBar.vue              [新]
    workbench/WorkbenchTabs.vue                [新]
    workbench/NoteEditor.vue                   [新] CM6 装配
    workbench/QuickSwitcher.vue                [新]
    workbench/CommandPalette.vue               [新]
    workbench/SearchPanel.vue                  [新，P1-3] Ctrl+Shift+F 全库搜索浮层
    panels/PanelBacklinks.vue                  [新；P2-7 增未链接提及分组]
    panels/PanelOutlinks.vue                   [新]
    panels/PanelOutline.vue                    [新]
    panels/PanelProperties.vue                 [新]
    panels/PanelLocalGraph.vue                 [新]
  pages/KnowledgePage.vue                      [重写] 薄壳 → KnowledgeWorkbench
  components/knowledge/KnowledgeVaultTree.vue  [改] 外观令牌适配（逻辑不动）
  components/knowledge/KnowledgeGraph3D.vue    [改] 支持全屏覆盖模式（v-model:fullscreen）
  components/knowledge/KnowledgeEmbedderPanel.vue [改] 包入设置浮层（逻辑不动）
退役：KnowledgeDocumentsPanel / KnowledgeDocList / KnowledgeDocDetail / KnowledgeSearchDual
  ——树 + 工作台 + 五面板取代其职责；保留文件至验收通过后再删（切片 8 处理）

2026-08-11 增强轮新增/改动：
  api/kratos/knowledge/v1/knowledge.proto       [改，P2-7] ListUnlinkedMentions RPC
  internal/biz/knowledge/mention.go             [新，P2-7] 未链接提及领域逻辑 + DocContentSearcher 端口
  internal/data/knowledge_mentions.go           [新，P2-7] 端口实现（content_text ILIKE 预筛）
  internal/service/knowledge_mention.go         [新，P2-7] RPC 装配
  web/src/features/knowledge/commands.ts        [改，P2-6] aliases 字段 + pushMru + filterCommands MRU
  web/src/features/knowledge/wikilink.ts        [改，P2-5] #heading 标题补全分支
  web/src/features/knowledge/useKnowledgeWorkbench.ts [改，P1-4] reorderTabs
  web/src/components/knowledge/workbench/WorkbenchTabs.vue [改，P1-4] 原生 DnD + 中键关闭
```

**分层纪律**：纯函数解析（outline/frontmatter/wikilink 目标匹配）放 `features/knowledge/` 并可单测；组件只消费；API 层不动。

### SP2-10. 关键决策记录（ADR 摘要）

| # | 决策 | 理由 | 放弃方案 |
|---|------|------|---------|
| SP2-ADR-1 | CodeMirror 6 Live Preview | 源文本即真相源，与 SP1 哲学一致；装饰管线精确到行；License MIT | TipTap（富文本中间态与 md 双向映射有损）/BlockSuite（块模型与现有文档粒度不匹配） |
| SP2-ADR-2 | 单 composable 状态机 + props/emits | Obsidian MetadataCache「单一真相源」前端映射；五面板联动只需订阅 activeTab | Pinia store（跨页面共享无必要，composable 单例已够）/ 组件各自拉数（联动地狱） |
| SP2-ADR-3 | 显式保存（Ctrl+S）+ CAS，不做自动保存 | CAS 语义下自动保存放大冲突窗口；保存状态可见性更强 | 防抖自动保存（Obsidian 行为，但其无并发写者） |
| SP2-ADR-4 | 特效组件自研（非引库） | 5 个组件总代码量 <600 行，零依赖风险；液态玻璃为 CSS 技巧非库能力 | 引入 UI 特效库（体积 + 协议 + 主题割裂） |
| SP2-ADR-5 | 局部迷你图谱自研轻量 2D canvas | 迷你图 ≤200 节点求启动快；复用 G5 3D 管线加载成本高且视觉过载 | 嵌入 G5 3D 实例（重）/ sigma.js（新增依赖只为迷你图不值） |
| SP2-ADR-6 | 视觉令牌作用域隔离 `.kb-workbench` | 深空风仅知识库沉浸区；不污染全局明暗双主题体系 | 全局主题改造（破坏面大，违背 NFR-G5-4 同原则） |
| SP2-ADR-7 | kb 令牌消费全局 Tech Night 变量（P0-1） | 色彩同源消除主题漂移；作用域隔离不变（ADR-6），只是值的来源从硬编码改为 `var(--*)` | 保留独立硬编码调色板（主题切换时工作台颜色脱节，2026-08-11 用户反馈根因） |
| SP2-ADR-8 | 液态玻璃三层效果纯 CSS+SVG 实现（P0-2） | SVG 位移滤镜提供折射光纹，镜面断面/指针高光为 CSS 渐变与阴影；零 JS 动画循环、零依赖 | WebGL/canvas 流体模拟（性能与复杂度远超收益）/ 引入 liquid-glass 库（协议与主题割裂，同 ADR-4） |
| SP2-ADR-9 | 全库搜索复用 SearchKnowledge（P1-3） | 既有 RPC 已支持 collection 限定 + top_k；容器侧防抖 + seq 竞态守卫即可满足体验 | 新增专用全文搜索 RPC（重复能力，维护两套检索入口） |
| SP2-ADR-10 | 标签页拖拽用原生 HTML5 DnD（P1-4） | 标签条为扁平数组重排，原生 dragstart/dragover/drop 足够；零新增依赖 | vue-draggable 等库（为一个列表引入依赖不值，同 ADR-5 哲学） |
| SP2-ADR-11 | 未链接提及做端口 + 可选降级（P2-7） | `DocContentSearcher` 为 evolving 端口，SQL ILIKE 预筛 + biz 层剔除 `[[...]]` 内命中；未接线返回空不阻断反链 | 全量扫描无预筛（大库性能不可控）/ 纯前端扫描（拉全库正文不可行） |

### SP2-12. 真液态玻璃三层效果（P0-2，`GlassPanel.vue`）

初版玻璃仅「半透明 + blur + 1px 高光线」，无液态质感。增强后三层叠加（均为装饰层，`pointer-events: none`，`z-index` 低于内容）：

| 层 | 实现 | 效果 |
|----|------|------|
| 1 折射光纹 | SVG `<filter id="kb-liquid-refract">`：`feTurbulence`（fractalNoise，baseFrequency 0.012/0.028，2 octave）→ `feDisplacementMap`（scale 10）作用于对角线性渐变光泽 | 光纹被噪声位移扭曲，产生液态折射感；静态无动画 |
| 2 镜面断面 | `::after` 伪元素 inset box-shadow：顶缘 1px 白色反光（0.12）+ 左缘弱反光（0.05）+ 底缘暗线（0.3） | 玻璃厚度/断面感；全局玻璃表面在 `deep-space.sass` 统一注入同组阴影 |
| 3 指针追随高光 | `pointermove` 写入 `--kb-mx/--kb-my` CSS 变量 → radial-gradient 240px 跟随圆斑；hover 淡入（0.35s opacity 过渡） | 光照随指针流动 |

边缘羽化：层 1/3 均加 `mask-image: radial-gradient(130% 130% at 50% 50%, black 62%, transparent 100%)`，光效在角落淡出无硬边。降级契约不变（FR-SP2-10）。

### SP2-13. 全库内容搜索（P1-3，`SearchPanel.vue` + 容器检索）

**职责划分**（数据流纪律）：`SearchPanel` 纯受控组件（渲染结果 + 键盘导航）；检索逻辑在容器 `KnowledgeWorkbench`——

- **触发**：Ctrl+Shift+F / 顶栏搜索按钮；与 ⌘O/⌘K 互斥（`openSearch` 关闭另两者）。
- **检索**：`searchQuery` watch → 300ms 防抖 → `searchKnowledge({collection_id: 当前 Vault, query, top_k: 12})`；`searchSeq` 单调递增守卫，慢响应落地前比对序号，过期结果直接丢弃。
- **片段**：`buildSnippet` 以首个匹配词为中心前后各扩 48 字符、总长 160，两端越界加省略号（Obsidian 语义）。
- **结果项**：文档名（rel_path basename）+ 路径 + 分数徽标（tabular-nums）+ 双行截断片段；Enter/点击 → `openDoc`。
- **关闭清理**：取消防抖定时器 + 序号失效 + 清空 query/items/loading，下次打开从零开始。

### SP2-14. 标签页管理增强（P1-4，`WorkbenchTabs.vue` + `reorderTabs`）

- **拖拽重排**：原生 HTML5 DnD——tab `draggable`，`dragstart` 记录源索引，`dragover` 阻止默认使落点合法，`drop` 上抛 `reorder(from, to)`；状态机 `reorderTabs` 做 splice 移动，激活态 docId 不变。拖动中 `kb-tabs__tab--dragging` 半透明反馈。
- **中键关闭**：`@mousedown.middle.prevent` 直接走 `close` 事件（脏标签仍进保存确认弹窗，逻辑复用）。
- **边界**：from/to 越界或相等时状态机直接返回；纯前端数组变更，无持久化（标签序是会话态，Obsidian 同）。

### SP2-15. wikilink 标题段补全与定位（P2-5）

- **补全分支**：`wikiLinkCompletionSource` 正则升级为 `\[\[([^\]|#]*?)(?:#([^\]|]*))?$`；检测到 `#` 时——目标名为 `#` 前段，候选 = `getHeadings(target)`（容器提供：已打开 tab 的 `parseOutline` 标题文本列表），过滤后插入点从 `#` 后起算。未提供 `getHeadings` 时该分支返回 null（降级为仅文档名补全）。
- **装配**：`KnowledgeWorkbench.getHeadingsFor(target)` 按名归一化匹配已打开 tab → `parseOutline(tab.content)` 取标题文本；经 `WorkbenchTabs → NoteEditor` props 传递。
- **跳链定位**：`WikiLinkWidget` 持有 `heading` 字段；`open-doc` 事件载荷 `(target, heading?)`——无 heading 保持单参数（不破坏既有测试/下游契约）。容器 `openDocByName` 打开后 `nextTick` 在目标文档大纲中大小写不敏感匹配标题，命中则 `EditorView.dispatch` 选区 + `scrollIntoView`。

### SP2-16. 命令面板 MRU 与别名（P2-6，`commands.ts`）

- **别名**：`CommandDef.aliases` 注册英文/拼音关键词（如 save → `['save','write','baocun']`）；`filterCommands` 键入查询时 `instantFilter` 同时匹配标题 + 别名，打分排序不变。
- **MRU**：`pushMru(mru, id, keep=3)` 纯函数——id 置顶、去重、截断；空查询时 MRU 项置顶（保持近→远顺序）并显示 history 角标，其余按注册顺序。
- **持久化**：容器 `KnowledgeWorkbench` 持有 `commandMru` ref，命令执行后 `pushMru` + 写 localStorage `kb.command.mru`；读写均 try/catch，隐私模式写失败静默降级为会话内 MRU。

### SP2-17. 未链接提及（P2-7，SP2 唯一后端新增）

**定义**（Obsidian "Unlinked mentions"）：目标文档显示名（rel_path/source 取 basename 去扩展名）在同库其他文档正文中以**纯文本**出现——`[[wikilink]]` 整段剔除后计数。

**链路**：

```
proto: GET /v1/knowledge/documents/{doc_id}/unlinked-mentions
  → service/knowledge_mention.go（薄装配）
  → biz/knowledge/mention.go ListUnlinkedMentions
      ① 取目标文档 → mentionNeedle 提取显示名
      ② needle < 2 字符或端口未接线 → 返回空（降级不阻断）
      ③ DocContentSearcher.SearchDocContentMentions（端口，evolving）
         = data/knowledge_mentions.go：content_text ILIKE %needle% 预筛（排除自身，≤200 候选）
      ④ biz 精确化：wikiLinkSpanRe 剔除 [[...]] → 小写计数 + 首次出现片段（前后 48 rune）
      ⑤ Count 降序 + SrcDocID 字典序，≤50 条输出
  → PanelBacklinks 未链接提及分组（来源文档名 + 计数徽标，点击 openDocId）
```

**设计要点**：ILIKE 只作预筛（可能命中全在 `[[...]]` 内），精确判定必须在 biz 层剔除链接段后重算；snippet 用 `strings.ToLower` 对本域字符集（CJK/ASCII）等长的性质直接索引原文取片段；单字符名噪声过大直接降级。

### SP2-18. 影响面（改 SP2 必须同步谁）

- **KnowledgePage.vue**：整页重写为薄壳；路由 `/knowledge` 不变；i18n key 前缀 `knowledgePage.*` 新增 workbench 段
- **KnowledgeGraph3D**：新增 `fullscreen` props 模式（覆盖层定位 + ESC 退出），G5 画布/HUD 逻辑零改动
- **KnowledgeVaultTree**：仅样式适配深空令牌（CSS 变量注入点），交互逻辑/事件签名不变
- **退役组件**：KnowledgeDocumentsPanel/DocList/DocDetail/SearchDual 被工作台吸收——验收前保留文件，验收后切片 8 删除并全局 grep 清理引用（R4）
- **WS 摄取进度**：`useKnowledgeIngestWs` 继续工作（上传队列收纳进左栏底部），事件消费不变
- **后端**：初版零改动；**2026-08-11 增强轮 P2-7 新增 1 个只读 RPC** `ListUnlinkedMentions`（proto + biz 端口 + data 实现 + service 装配，见 §SP2-17），无 Schema/迁移变更
- **测试**：纯函数（outline/frontmatter/wikilink resolve/命令过滤/pushMru）+ 状态机（tabs 开闭/脏标记/CAS 冲突/reorderTabs）单测；组件级 smoke（挂载不炸）；`pnpm lint + test + build` 全绿 + 浏览器运行时复验（验收 38）

---

## V12.10 Lazy GraphRAG + Retrieve-Then-Generate（2026-08-15）

> 对应需求 US-32/US-33。Microsoft LazyGraphRAG 思路的工程落地：**不在入库期抽实体图谱**（§9.6 全量 GraphRAG 仍可选暂缓），查询期复用已物化的 `knowledge_links`。

### 检索侧

```
AdaptiveRouter.Search
  → (可选) 复杂查询自动 MultiQuery
  → HybridRetriever（dense / BM25 / RRF）
  → GraphExpander.Expand          ← 新增，nil 则跳过
       seed docs (top 5)
       → ListLinks 一跳邻居（explicit×3 > entity×2 > semantic×1，cap 8）
       → ListChunksByDocuments（每文档 chunk_index 前 2 块）
       → MergeSearchResults(seeds, neighbors×0.72, topK)
```

- `GraphExpander` 定义在 `internal/knowledge/graph_expander.go`（消费方接口 `NeighborLinkReader` / `NeighborChunkLister`）。
- 生产实现：`data.knowledgeRepo.ListLinks` + 新增 `ListChunksByDocuments`（**不**扩 `ChunkRepo` 接口，避免所有 mock 连锁修改；Wire 对 `KnowledgeRepo` 动态断言，未实现则 expander=nil）。
- 即时意图（`ClassifySearchIntent == instant`）与 `collection_id` 为空时不扩展（联邦已按库调用 Router）。
- 扩展预算 800ms；失败 Warn 后返回种子。

### Agent 注入侧

`internal/agent/knowledge_inject.go` 不再只拼 Collection 目录：

1. `lastUserQuery`：最新非 system 消息必须是 user，否则视为工具循环，跳过预检索。
2. 2s 超时：优先 FederatedRetriever（全库/多库），否则 AdaptiveRouter/Retriever 单库。
3. cue 结构：`## Retrieved Knowledge`（[n] 编号段落）+ 缩短的 `## Available Knowledge Bases`。

前缀缓存契约不变：动态 cue 仍 append 在消息列表末尾。

### 复杂查询自动重写

`pickAutoRewriteStrategy`：仅 `QueryComplex && rewrite_strategy 未指定` → `multi_query`。HyDE/Decomposition 仍由 API 显式参数触发。

### 不做什么（本轮边界）

- 不新建 `knowledge_entities` / `knowledge_relations` 表，不跑入库期 NER。
- 不改 Proto（Search 行为增强，契约字段不变）。
- 不做社区摘要 / 全局 GraphRAG 2.0 四模式。
- Skill Knowledge（Phase 4）与知识-记忆同基底（SP7）仍未实施。

---

## V12.11 编译期 Wiki 成链（2026-08-15）

> 对应需求 US-34。综合方案见 [2026-08-15-research-knowledge-synthesis.md](../reports/2026-08-15-research-knowledge-synthesis.md)。
> 四层架构中的**写路径**一层：Karpathy/llm-wiki 式 compile-time wiki + Obsidian 社区插件「Link Unlinked Mentions」规则，接到已有 `RebuildBlockIndex` / Lazy GraphRAG。

```
Write (ingest / vault save)
  → AutolinkWikiMentions(content, titles)
  → 落盘或 content_text
  → RebuildBlockIndex → knowledge_links (explicit)
Query
  → Hybrid + GraphExpander 一跳（V12.10）
Agent
  → Retrieve-Then-Generate（V12.10）
Write-back
  → 会话写回飞轮（V12.12 / SP7 G2）
```

### 规则（对标 Obsidian 插件，不抄 AGPL 代码）

| 规则 | 行为 |
|------|------|
| 标题来源 | `mentionNeedle(rel_path, source)`：basename 去扩展名 |
| 最长优先 | 先匹配更长标题，占用区间后短标题不再切 |
| 词边界 | ASCII 整词（`A-Za-z0-9_`）；CJK 子串允许 |
| 最短长度 | CJK ≥2 rune；ASCII ≥3 rune（抑制 Go/AI） |
| 歧义 | 同一 needle 对应多个文档 → 整针跳过 |
| 自链 | 当前文档标题不包装 |
| 保护区间 | YAML frontmatter、```/~~~ 围栏、行内 `` ` ``、`[[...]]`、`[text](url)`、`http(s)://` |
| 大小写 | 匹配不敏感，包装用库内规范标题 |
| 失败 | `ListDocuments` 错误 → 原文；不新增 Usecase 字段 |

### 接线

- 纯函数：`internal/biz/knowledge/autolink.go`
- 摄取：`IngestDocument` 整理后调用 `Usecase.MaybeAutolinkOutgoing`；图片视觉提取后同样成链再回写
- 保存：`UpdateVaultDocumentContent` 在 `WriteDocCAS` 之前成链
- **不**在 vault watcher `ApplyOne` / `UpdateDocumentContent` 持久化入口成链，避免外部编辑器保存被静默改写、以及 PG 与文件分叉
- 不改 Proto

### 不做什么

- 摄取/保存路径仍自动成链（无弹窗）；确认弹窗只服务「本页编译」与历史回填的显式动作（见 V12.13）
- 不读 frontmatter aliases 入库（v1 只用文件名标题）
- 历史回填不挂在 watcher 上（见 V12.13 US-38/US-45：显式 POST autolink-backfill，不挂 RebuildKnowledgeIndex）

---

## V12.12 会话写回飞轮（SP7 G2，2026-08-15）

> 对应需求 US-37。G1/G7/G8 与确认过门见 V12.13。

```
AutoMemoryWorker.extract
  → FactWritePipeline.Apply（L3 已有门 0.6）
  → maybeWriteBack（失败只 Warn）
       FilterWriteBackFacts（kind 白名单 ∩ confidence≥0.85 ∩ ≥8 rune）
       → 团队库当日日记追加 provenance 块
       → Autolink + RebuildBlockIndex
       → KnowledgeService 重放 chunk/FTS
```

- 端口：`knowledge.SessionWriteBack`（biz 别名 `KnowledgeWriteBack`），生产实现 `KnowledgeService`。
- 日记路径：`inbox/writeback-YYYY-MM-DD.md`。集合：同名「团队知识收件箱」→ 否则工作区第一个 team 库 → 否则 `CreateVault` 懒创建（词法、无 embedding）。
- **不**在 watcher/外部编辑路径写回；**不**把 L2 episode 原文整段入库。

### 不做什么

- 不做 LLM 二次抽取 / 三元组表
- 自动路径仍无弹窗（≥0.85 直接日记）；0.60–0.84 进 pending（V12.13 US-44）
- G1 投影见 V12.13（覆盖写，不是往 Usecase 加字段）

---

## V12.13 成链回填、金标与 SP7 收口（2026-08-15）

> 对应需求 US-38~US-48。不改 Proto（custom HTTP，与 document asset 同鉴权过滤器）。

```
RebuildKnowledgeIndex
  → RebuildCollectionBlockIndex（allowBackfill=false，不改源）

POST .../autolink-backfill
  → BackfillOutgoingAutolinks（显式写路径成链）
  → RebuildCollectionBlockIndex
  （与重建共用 rebuildRuns 互斥门 + knowledge_rebuild_index 进度）

工作台
  → GET autolink-preview → 确认 → POST autolink
  → GET writeback-home（只解析）→ experts / writeback-pending 打落点库
  → WritebackReviewDialog 勾选 → POST writeback-pending/apply（fact_ids）
  → GET health 仍打当前打开的库

AutoMemoryWorker.extract
  → maybeWriteBack（≥0.85）
  → maybeEnqueueReview（0.60–0.84 白名单）
  → maybeProjectMemory（覆盖 agents/{id}.md）
```

### 自定义路由（`internal/server/http.go`）

| 方法 | 路径 | 权限 |
|------|------|------|
| GET | `/v1/knowledge/documents/{id}/autolink-preview` | 集合读 |
| POST | `/v1/knowledge/documents/{id}/autolink` | 集合写 |
| POST | `/v1/knowledge/collections/{id}/autolink-backfill` | 集合写；改 Markdown |
| GET | `/v1/knowledge/writeback-home` | 工作区读；不创建集合 |
| GET | `/v1/knowledge/collections/{id}/health` | 集合读 |
| GET | `/v1/knowledge/collections/{id}/experts` | 集合读 |
| GET | `/v1/knowledge/collections/{id}/writeback-pending` | 集合读 |
| POST | `/v1/knowledge/collections/{id}/writeback-pending/apply` | 集合写；body `fact_ids` |

### 结构约束

- G1 用独立 `AgentMemoryProjector`（AS-COG-01），经 `KnowledgeService.SetAgentMemoryProjector` 接线。
- 健康度只读聚合，GET 不写 vault 文件。
- `LookupWriteBackHome` 与 `resolveWriteBackCollection` 共用扫描规则；后者仅在写路径懒创建。
- 一跳金标在 `internal/knowledge/gold_recall_test.go`（假 GraphExpander，不连 PG）。
- 词法金标在 `internal/data/knowledge_gold_bm25_test.go`（真实 `SearchChunksBM25`，需 `aranea_test`）。

### 不做什么

- 不上 PPR / 社区摘要 / 入库期 NER。
- 不把 ListChunksByDocuments 加进 `ChunkRepo`。
- 不做视频 RAG、时间知识图谱、成熟度 FSM（SP5）、JITAI 伙伴（SP6）。
- 不做 50 条 BM25 愿望清单；US-47 是 12 条生产路径查询。

---

## V12.14 长期供粮核心可靠性（2026-08-17）

> 对应需求 US-49。目标是修复检索热路径和派生索引闭环，不引入新的知识真相源。

### ADR-KN-20260817：正确性闭环优先于继续增加检索算法

- **背景**：系统已有 dense/BM25/RRF/rerank/图扩展，但 RRF 串行且不重排，数据库上传不触发图谱，派生 chunks 失败后依赖人工修复。
- **决策**：本轮先统一现有分支语义、并行独立召回并补自动修复工人；所有新后台动作保持有界、幂等、可降级。
- **后果**：热路径延迟不再叠加 dense+sparse+access-log；上传与索引故障具备最终一致闭环。代价是进程内缓存非跨实例共享，短查询 substring 分支仍需在线数据验证成本。
- **替代方案**：暂不引入外部缓存、独立搜索引擎或新图数据库；现阶段数据量与质量基线不足以证明额外基础设施收益。

### 检索决策

1. `HybridRetriever` 的 RRF 分支在 query embedding 完成后并行执行 dense / sparse；单路失败退化到另一条。
2. RRF 先合并 overfetch 候选，再调用与 dense 路相同的可选 reranker，最后裁剪 topK。
3. `AdaptiveRouter` 对路径、引号短语、错误码、带分隔符标识符和全大写术语选择 sparse；普通短问句仍选择 dense。
4. `SearchChunksBM25` 并行执行 tsvector / trigram；≤4 字符查询增加 exact-substring 分支，三个排名用 RRF 融合，不再把 tsvector 列表无条件放在 trigram 前。
5. `MultiProviderEmbedder` 对查询侧单条向量使用 10 分钟、512 项的进程内有界缓存和 singleflight；缓存键含 provider/baseURL/model/dim/task type 的哈希，运行时配置更新清空缓存。
6. 检索命中日志通过 `safego` 异步持久化，保留 context values、去掉取消传播并设置 2 秒写超时。阶段直方图覆盖 embed/dense/sparse/total。

### 摄取与自动修复

- 数据库上传在 chunks 与块索引完成后调用统一图谱钩子，异步执行实体共现和 typed relation 抽取；钩子按 content hash 幂等。
- typed relation 的 evidence 必须经归一化后能在当前正文中找到；找不到的三元组不登记谓词、不发布边。
- `knowledge_links_unique` 改为仅约束 `valid_to IS NULL` 的 active 行。显式/entity/semantic 替换均原位刷新未变边、关闭消失边、插入新增版本，历史不再被 `DELETE` 抹除。
- `KnowledgeIndexRepairWorker` 启动即执行、之后每 5 分钟扫描；每轮最多修复 20 篇，避免打爆 embedding provider。
- 待修复判定区分词法库与语义库：无 chunks 始终待修复；仅语义库的 NULL embedding 待修复。词法库正常 NULL embedding 不进入循环。

### 安全边界

- 同一别名命中多个词条时禁止任取一个自动归并；事实回退 provenance 日记并记录结构化告警。
- 本轮不声称解决全量事实断言的稳定 key、跨文档冲突真值或抽取置信度校准；这些需要独立金标，不能通过热路径优化替代。

---

## V12.15 质量门禁与事实演化连续性（2026-08-17）

> 对应需求 US-50。本轮不增加新的检索算法，而是把真实问法、标准指标和事实冲突安全边界固化为可回归契约。

### 检索评测

- `internal/knowledge/retrieval_metrics.go` 提供纯函数文档级评测：结果先按 `doc_id` 保序去重，再计算 Recall@K、HitRate@K、MRR、nDCG@K 和 Abstention Accuracy。
- 排名指标只统计有相关文档的 case；`abstain=true` 的 case 只进入拒答准确率，避免负例把 Recall 分母污染。
- `internal/data/knowledge_retrieval_baseline_test.go` 复用同一语料。词法发布门禁打每条金标的最长 `expected_keywords`（HitRate@5 ≥ 0.90）和标识符切片（HitRate@5 ≥ 0.80）；把全部关键词 AND 拼接会误杀短中文+数字组合。自然语言问法的词法硬门见 §V12.16。
- `internal/knowledge/retrieval_gold.go` 将 30 条问法扩展为 10 种检索前缀改写（≥300 条）+ 语料内标识符查询 + 拒答切片。改写集检验问法扰动下的词法稳定性，不是 300 条独立标注的新信息需求。
- 30 条是首个可执行真实问法基线；独立信息需求的下一步仍是按章节/同义改写人工扩容，并分语言、精确标识符、时态和拒答切片报告。

### 事实演化

1. 同 `fact_id` 的确定性更新语义不变。
2. 仲裁器高置信判定 `supersedes` 时，目标段原 `fact_id` 保留为 lineage identity；新提取 ID 写入 `source_id`，不接管演化身份。
3. `knowledge_fact_version.fact_id` 继续记录该 lineage identity，旧段和新段均保存。
4. 事实级 conflict 的 `dedup_key` 为 `conflict:fact:{doc}:{targetFact}:{incomingFactOrHash}`；pending/applied/rejected 都参与抑制，防止同一冲突反复打扰。
5. 事实级 conflict 禁止裸 `applied`。`keep_old` 删除新 H2 段；`keep_new` 删除旧段、保留新陈述，并把 `fact_id` 收敛到旧 lineage（新 ID 记 `source_id`）。两种决定落库终态均为 `applied`。文档级 conflict 仍关闭双向 active `contradicts` 边后 applied。
6. HTTP `ResolveGovernanceProposal.decision` 与记忆管家 `memory_butler_governance_resolve` 接受 `applied | rejected | keep_old | keep_new`。

### 并发验证

- 联邦广播测试 repo 的搜索记录使用互斥锁和快照访问器。
- Vault sync 测试 repo 的 chunk 读取返回锁内副本，后台 runner 断言通过锁内计数访问器轮询。
- 生产联邦结果槽位和 Vault 单库 runner 未发现对应竞态；不因测试夹具问题修改生产并发模型。

## V12.16 自然语言问句词法规范化（2026-08-17）

> 对应需求 US-51。不引入新检索算法、不调用 LLM。目标是让 Agent 的完整中文问句在纯 BM25 路径上可召回。

### 根因

PostgreSQL `simple` 配置下，无空格的中文问句会被 `plainto_tsquery` 当成极少几个大 lexeme 做 AND；问句套话（是什么/多久/请问）并不出现在正文，整句 `word_similarity` 也达不到 `%>` 阈值。短关键词能命中，完整问法 HitRate@5 实测为 0。

### 设计

1. `biz/knowledge.LexicalSearchQueries` 判断问句（问号、套话、≥8 个汉字）后，去掉套话与标点，按助词切开，抽取 3–8 字 CJK 针和 ASCII 标识符，最多 7 条变体。
2. `SearchChunksBM25` 对每个变体并行跑 tsvector + trigram；原查询维持 ≤4 字 substring，内容针放宽到 ≤8 字 substring；各路 RRF 融合。
3. 短关键词、工单号、设备名不走针扩展，避免 n-gram 稀释排序。
4. 30 条自然语言问法升为 BM25 硬门：HitRate@5 ≥ 0.90。拒答切片仍不得误中。

### 非目标

不把 300 条前缀改写当成 300 条独立事实；不上新 embedding 模型。工作台 keep_old/keep_new 见 §V12.17。

## V12.17 治理提案工作台（2026-08-17）

> 对应需求 US-52。后端 `keep_old`/`keep_new` 已通；本轮只补工作台二审入口，不改处置语义。

### 交互

- 命令面板「审核治理提案」（`review-governance`）。
- 默认拉当前库 `status=pending`；若为空则回退写回收件箱，并显示「结果来自某库」横幅。
- 事实段 conflict（payload 含 `target_fact_id`）：按钮仅为保留旧陈述 / 采用新陈述 / 驳回。
- 文档级 conflict：关闭矛盾关联（`applied`）/ 驳回。
- 孤儿词条：删除孤儿词条（`applied`，会删文档）/ 驳回。文案必须写明删除。

### 分层

- `features/knowledge/governance.ts` 纯函数决定按钮集。
- `features/knowledge/api.ts` 封装 List/Resolve。
- `GovernanceReviewDialog.vue` 仅 props/emits；`KnowledgeWorkbench` 调 API。

## V12.18 问句自动混合检索（2026-08-17）

> 对应需求 US-53。US-51 修好了 `SearchChunksBM25` 问句针，但 AdaptiveRouter 把带问号的中文问句判成 QuerySimple → dense-only，生产预检索用不上词法针。

### 路由

1. 路径 / 引号短语 / 工单号 / 全大写错误码 → `sparse`
2. 搜索意图 semantic，或 `LooksLikeNaturalLanguageQuery`（问号、套话、≥8 汉字）→ `rrf`
3. 其余仍按复杂度：simple=`dense`，moderate/complex=`rrf`
4. 问句不抬 classify，避免默认 LLM MultiQuery

Agent 预检索与 HTTP Search 的 auto 模式都走 AdaptiveRouter，因此词法库与语义库上的完整问句都会带上 BM25 针。

## V12.19 治理定时实跑与入库自关联（2026-08-17）

> 对应需求 US-54。体检结论：治理只挂在 `dream_cycle` 且默认 dry_run；vault 冷文档不抽 typed 关系；未链接提及要人点「编译双链」。本轮把低风险治理和入库关联收成后台闭环，高风险仍人工二审。

### 治理工人

- `KnowledgeCurateWorker`：`CurateAllTeamKnowledge(DryRun=false)`，默认 6h，启动时跑一轮。
- 低风险实写：Hebbian decay、谓词 promote、stale 标记、distill。
- 高风险只产 pending：contradicts、orphan、moc_emerge。工作台 `keep_old`/`keep_new` 语义不变。
- 无团队库 `NOT_FOUND` 静默；`KNOWLEDGE_CURATE_DISABLED=1` 不装配。

### 入库图谱

- Vault `SetRelationHook` 与 `SetEntityHook` 同点触发 `ExtractDoc`（幂等）。热度工人仍扫热文档作补网。
- `RebuildBlockIndex` 先 `compileOutgoingMentions` 再 parse/投影 explicit 边；不写文件、不写 `content_text`。US-45「重建不改源」仍成立。
- 把提及写进 Markdown 仍只有 `ApplyOutgoingAutolinks` / `autolink-backfill`。

## V12.20 可信回答、实体词条与文档可见性（2026-08-17）

> 对应需求 US-55 / US-56 / US-57。不对齐 NotebookLM 插件生态或 Glean 连接器，只补三块产品缺口：回答可核对、团队库会长词条、共享库能藏草稿。

### 可信回答（US-55）

- 预检索命中以 `{chunks:[{chunk_id,doc_id,score,line}]}` 进入活动时间线；前端按 `chunk_id` 合并为 chips，有 `doc_id` 则路由 `/knowledge?doc=`。
- `config_json.knowledge.grounded_only`：Create/Update Agent 与 `evaluation` 一并 overlay，禁止整表清空 `config_json`。
- `formatKnowledgeCue(..., toolsEnabled, groundedOnly)`：
  - grounded + 无命中 + 关工具 → 硬拒答、禁止世界知识；
  - grounded + 无命中 + 开工具 → 只许 `knowledge_search`；
  - grounded + 有命中 → 只用段落，不足则说没有。

### 实体词条（US-56）

- `EntityPipeline.SetWikiWriter` → `Usecase.EnsureEntityWikiPages`。
- 仅 team vault；`entries/<slug>.md`；最多 8 实体；幂等追加来源 wikilink。
- 失败 Warn（`knowledge.entity.wiki`），实体轨已提交不回滚。

### 文档可见性（US-57）

- 列：`visibility TEXT DEFAULT 'collection'`、`owner_user_id TEXT DEFAULT ''`；索引 `(collection_id, visibility, owner_user_id)`。
- 读：`DocumentVisibleTo`；system 全见；无 user 只见 collection；有 user 见 collection 或 owner。
- 写：custom `GET/POST /v1/knowledge/documents/{id}/visibility`；标 private 时 `owner_user_id` = 当前用户。
- `ListDocuments` / `ListDocumentPaths` / `SearchChunks` / `SearchChunksBM25` 加 ACL 子句。直取内容走 `requireDocumentRead`（不可见 = NotFound）。
- 默认上传仍为 collection。不做飞书/SharePoint 连接器。


