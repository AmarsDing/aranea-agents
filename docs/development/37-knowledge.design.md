# Knowledge 知识库模块 — 实现设计文档

> 对应需求：`37 knowledge.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> **2026-06-17 校准**：与实际代码对齐；修正 Embedder 接口/结构体（`Embedder` 为接口，`MultiProviderEmbedder` 为实现）、修正构造函数签名（补充 `lg loggateway.Logger` 参数）、修正 `knowledge_embed_setting.go` 引用（实际逻辑在 `knowledge/knowledge.go` 的 `ApplyEmbedPatch`）、补充 Reranker、Embedder Admin API、摄取 WS 事件、`ingest.go` 流水线拆分、Advanced RAG（查询重写/混合检索/自适应路由/检索评估）、Agentic RAG（联邦搜索/knowledge_reflect 工具）、OCR stub、BM25 双路检索、Biz 子包迁移、KnowledgeSearchDeps 聚合、GraphRAG/Skill Knowledge 待实现设计。

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
  string embedding_model = 3 [(google.api.field_behavior) = REQUIRED];
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
  string collection_id = 1 [(google.api.field_behavior) = REQUIRED];
  string source = 2 [(google.api.field_behavior) = REQUIRED];
  string mime_type = 3;
  string content_base64 = 4 [(google.api.field_behavior) = REQUIRED];
  string metadata_json = 5;
  int32 chunk_size = 6;       // 0 = 服务端默认 512
  int32 chunk_overlap = 7;    // 0 = 服务端默认 64
  string chunk_strategy = 8;  // char|token|markdown|json|recursive
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

message SearchRequest {
  string collection_id = 1 [(google.api.field_behavior) = REQUIRED];
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
  rpc DeleteDocument(DeleteDocumentRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/knowledge/documents/{id}" };
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
    ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]Document, int, error)
    DeleteDocument(ctx context.Context, id string) error
}

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

func (uc *Usecase) CreateDocument(ctx context.Context, d Document) (Document, error)
func (uc *Usecase) ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]Document, int, error)
func (uc *Usecase) DeleteDocument(ctx context.Context, id string) error
func (uc *Usecase) UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error

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
| `SearchChunks` | `ORDER BY embedding <=> $1::vector LIMIT $3`，支持 `min_score` 和 `filter_json` |
| `SearchChunksBM25` | 双路 BM25：tsvector 全文检索 + pg_trgm 模糊搜索，合并去重 |
| `DeleteDocument` | 事务删除 + 计数器修正 |

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

### 5.2 文档解析（internal/knowledge/document_extract.go）

```go
func ExtractDocumentText(raw []byte, source, mimeType string) (string, error)
```

- PDF / DOCX：trpc `document/reader`（`readers_import.go` 侧载注册）。
- HTML：`html_text.go` 剥离 script/style 后提取可见文本。
- 纯文本：UTF-8 直读。

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

---

## 六、Agent 集成

### 6.1 knowledge_search 工具（internal/tools/knowledge/tool.go）

```go
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

### 6.1b knowledge_reflect 工具（internal/tools/knowledge/tool.go）

```go
type reflectInput struct {
    CollectionIDs []string `json:"collection_ids" jsonschema:"description=List of collection IDs to search across,required"`
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
    bus           event.Bus
    systemSetting biz.SystemSettingRepo
    lg            loggateway.Logger
}
```

**关键设计**：

| 方法 | 说明 |
|------|------|
| `CreateCollection` | 参数校验 → `uc.CreateCollection` |
| `IngestDocument` | base64 解码 → 创建文档 → `safego.Go` → `BuildIndexedChunks` → 发布 `knowledge_ingest` 事件 |
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

```
IngestDocument(req)
  ├── base64.Decode → ExtractDocumentText(source/mime)
  ├── NormalizeMetadataJSON
  ├── uc.CreateDocument(status=pending)
  └── safego.Go → BuildIndexedChunks(strategy, EmbedBatch) → InsertChunks
```

**错误处理**：任何步骤失败 → `UpdateDocumentStatus(error, errMsg)` → goroutine 退出。

### 7.3 Wire 注入

```go
// internal/service/wire_providers.go — Chunker 默认 512/64 char
// internal/service/knowledge_embedder.go — NewKnowledgeEmbedder(c *conf.Data, sys, lg)
// internal/service/knowledge_retriever.go — NewKnowledgeRetriever(emb, repo, lg)
// internal/service/knowledge_advanced.go — Advanced RAG 组件工厂（6 个 Provider）
//   - NewKnowledgeHybridRetriever(retriever, sparse, lg)
//   - NewKnowledgeQueryRewriter(llm, sys, catalog, lg)
//   - NewKnowledgeAdaptiveRouter(hybrid, rewriter, lg)
//   - NewKnowledgeRetrievalEvaluator(llm, sys, catalog, lg)
//   - NewKnowledgeFederatedRetriever(router, retriever, uc, lg)
//   - ProvideKnowledgeSearchDeps(retriever, router, evaluator) → KnowledgeSearchDeps
```

**Wire 依赖链**：

```
Embedder + Repo → Retriever → HybridRetriever → AdaptiveRouter → FederatedRetriever
                              ↑                    ↑
                         SparseSearcher        QueryRewriter
                                              RetrievalEvaluator
```

---

## 八、前端集成

### 8.1 API 层（web/src/features/knowledge/api.ts）

| 函数 | 说明 |
|------|------|
| `listCollections` / `getCollection` / `createCollection` / `deleteCollection` | 集合 CRUD |
| `listDocuments` / `ingestDocument` / `deleteDocument` | 文档 CRUD |
| `searchKnowledge` | 语义搜索 |
| `getEmbedderConfig` / `updateEmbedderConfig` | Embedder 管理 |

### 8.2 Store 与页面

| 路径 | 说明 |
|------|------|
| `web/src/stores/knowledge/index.ts` | Pinia Store |
| `web/src/pages/KnowledgePage.vue` | 管理页（路由 `/knowledge`） |
| `web/src/components/knowledge/*` | 集合列表、文档、检索、Embedder、入库对话框 |
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

- OCR：`internal/knowledge/ocr.go` 已实现 `OCRProvider` 接口和工厂函数 `NewOCRProviderFromEnv()`。
  - 环境变量 `KNOWLEDGE_OCR`：`stub` / `placeholder` / `tesseract` / `docling`。
  - 当前所有值均回落到 `stubOCR`（返回占位文本），tesseract/docling 后端待接入。
  - `noopOCR`（默认）：静默返回空字符串。
  - `ExtractDocumentTextWithOCR` 支持注入自定义 OCR provider。
- Extractor：集成 `knowledge/extractor/docling`，PDF/图片 → Markdown（未实现）。

### 9.5 多租户隔离

SearchFilter 增加 `tenant_id`，向量存储按租户分区，API 层强制注入。

### 9.6 GraphRAG — 知识图谱增强

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
| `internal/knowledge/*.go` | 内部包：chunker/embedder/ingest/retriever/query_rewriter/hybrid_retriever/adaptive_router/retrieval_evaluator/federated_retriever/search_helpers/llm_resolver/ocr/html_text/chunk_strategy/document_extract/readers_import/reranker_factory |
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
