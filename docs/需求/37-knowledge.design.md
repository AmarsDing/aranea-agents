# Knowledge 知识库模块 — 实现设计文档

> 对应需求：`37 knowledge.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> **2026-05-21 校准**：与实际代码对齐；补充 Reranker、Embedder Admin API、摄取 WS 事件、`ingest.go` 流水线拆分。

---

## 一、模块概述

RAG 知识库：文档导入、分块、向量化、检索增强。对标 trpc-agent-go `knowledge` 包，当前实现基于 Collection 模型的简化 RAG 流水线。

### 核心架构

```
文档上传(base64) → Chunker(分块) → Embedder(向量化) → pgvector(存储)
                                                              ↓
Agent 调用 knowledge_search ← Tool(搜索工具) ← Retriever(检索) ← pgvector(搜索)
```

### 实际代码结构

```
internal/
├── biz/
│   ├── knowledge.go
│   └── knowledge_embed_setting.go  # Embedder DB patch 合并（EP-KN-01）
├── data/knowledge.go             # KnowledgeRepo 实现（PostgreSQL + pgvector raw SQL）
├── service/
│   ├── knowledge.go              # KnowledgeService（Kratos 传输适配）
│   ├── knowledge_embedder.go     # Embedder Wire 工厂（EP-KN-01）
│   └── knowledge_retriever.go    # Retriever + env Reranker（KN-01）
├── knowledge/
│   ├── chunker.go                # 文本分块（char/token 策略）
│   ├── embedder.go               # 向量化（openai/ollama/gemini/huggingface + EmbedBatch）
│   ├── chunk_strategy.go         # trpc 高级分块桥接
│   ├── document_extract.go       # PDF/DOCX/HTML 文本提取
│   ├── ingest.go                 # 分块+向量化流水线
│   ├── retriever.go              # 检索器（embed + search + optional rerank）
│   └── reranker_factory.go       # env → trpc reranker（topk/cohere/infinity）
├── tools/knowledge/tool.go       # knowledge_search trpc 工具
└── agent/trpc_build.go           # Agent 装配（KnowledgeSearch 开关）
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

```go
type KnowledgeCollection struct {
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

type KnowledgeDocument struct {
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

type KnowledgeChunk struct {
    ID           string
    DocID        string
    CollectionID string
    Content      string
    Embedding    []float32
    MetadataJSON string
    ChunkIndex   int
    Score        float32     // 仅搜索结果
}

type KnowledgeSearchQuery struct {
    CollectionID     string
    Query            string
    TopK             int
    MinScore         float32
    FilterJSON       string      // JSONB 元数据过滤
    UseRerank        *bool       // nil = 全局 reranker 启用时使用
    RerankCandidates int         // 重排前向量候选上限
}
```

### 3.2 Repo 接口

```go
type KnowledgeRepo interface {
    CreateCollection(ctx context.Context, c KnowledgeCollection) (KnowledgeCollection, error)
    GetCollection(ctx context.Context, id string) (KnowledgeCollection, error)
    ListCollections(ctx context.Context, workspace string, limit, offset int) ([]KnowledgeCollection, int, error)
    DeleteCollection(ctx context.Context, id string) error
    UpdateCollectionCounts(ctx context.Context, id string, docDelta, chunkDelta int) error

    CreateDocument(ctx context.Context, d KnowledgeDocument) (KnowledgeDocument, error)
    GetDocument(ctx context.Context, id string) (KnowledgeDocument, error)
    UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error
    ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]KnowledgeDocument, int, error)
    DeleteDocument(ctx context.Context, id string) error

    InsertChunks(ctx context.Context, chunks []KnowledgeChunk) error
    DeleteChunksByDocument(ctx context.Context, docID string) error
    SearchChunks(ctx context.Context, q KnowledgeSearchQuery, queryEmbedding []float32) ([]KnowledgeChunk, error)
}
```

### 3.3 Usecase

```go
type KnowledgeUsecase struct {
    repo KnowledgeRepo
}

func (uc *KnowledgeUsecase) CreateCollection(ctx context.Context, in KnowledgeCollection) (KnowledgeCollection, error)
func (uc *KnowledgeUsecase) GetCollection(ctx context.Context, id string) (KnowledgeCollection, error)
func (uc *KnowledgeUsecase) ListCollections(ctx context.Context, workspace string, limit, offset int) ([]KnowledgeCollection, int, error)
func (uc *KnowledgeUsecase) DeleteCollection(ctx context.Context, id string) error
func (uc *KnowledgeUsecase) UpdateCollectionCounts(ctx context.Context, id string, docDelta, chunkDelta int) error

func (uc *KnowledgeUsecase) CreateDocument(ctx context.Context, d KnowledgeDocument) (KnowledgeDocument, error)
func (uc *KnowledgeUsecase) ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]KnowledgeDocument, int, error)
func (uc *KnowledgeUsecase) DeleteDocument(ctx context.Context, id string) error
func (uc *KnowledgeUsecase) UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error

func (uc *KnowledgeUsecase) InsertChunks(ctx context.Context, chunks []KnowledgeChunk) error
func (uc *KnowledgeUsecase) Search(ctx context.Context, q KnowledgeSearchQuery, queryEmbedding []float32) ([]KnowledgeChunk, error)
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

`knowledgeRepo` 使用 `*sql.DB` 直接操作 PostgreSQL：

| 方法 | SQL 要点 |
|------|----------|
| `CreateCollection` | `INSERT INTO knowledge_collections ... RETURNING ...` |
| `GetCollection` | `SELECT ... FROM knowledge_collections WHERE id = $1` |
| `ListCollections` | `WHERE workspace = $1 OR $1 = ''` + 分页 |
| `DeleteCollection` | `DELETE FROM knowledge_collections WHERE id = $1`（CASCADE 自动清理） |
| `UpdateCollectionCounts` | `UPDATE ... SET document_count = document_count + $2, chunk_count = chunk_count + $3` |
| `CreateDocument` | `INSERT INTO knowledge_documents ... RETURNING ...` |
| `UpdateDocumentStatus` | `UPDATE ... SET status, error_message, chunk_count, updated_at` |
| `InsertChunks` | 事务批量 `INSERT INTO knowledge_chunks`，使用 `pgvector.NewVector` |
| `SearchChunks` | `ORDER BY embedding <=> $1::vector LIMIT $3`，支持 `min_score` 和 `filter_json` |

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
type Embedder struct {
    Provider string    // openai | ollama | gemini | huggingface
    BaseURL  string
    APIKey   string
    Model    string    // 默认 "text-embedding-3-small"
    Dim      int       // 默认 1536
}

func NewEmbedder(provider, baseURL, apiKey, model string, dim int) *Embedder
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error)
func (e *Embedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
```

**设计决策**：
- `openai`：`POST /v1/embeddings`，`EmbedBatch` 单次最多 32 条 input。
- `ollama`：`POST /api/embeddings`，逐条调用。
- `gemini`：`google.golang.org/genai` `EmbedContent`，批量 contents。
- `huggingface`：TEI `POST /embed`，`inputs` 数组批量。
- Wire 工厂 `NewKnowledgeEmbedder(c, SystemSettingRepo)`：env → DB → provider 默认 key（EP-KN-01 ✅）。
- `PersistKnowledgeEmbed` / `UpdateEmbedderConfig` 写回 `system_settings`；`biz/knowledge_embed_setting.go` 负责 patch 合并。

### 5.4 Retriever（internal/knowledge/retriever.go）

```go
type Retriever struct {
    embedder QueryEmbedder
    repo     biz.KnowledgeRepo
    reranker reranker.Reranker  // 可选，来自 NewRerankerFromEnv
}

func NewRetriever(embedder QueryEmbedder, repo biz.KnowledgeRepo, rr reranker.Reranker) *Retriever
func (r *Retriever) Search(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error)
```

**设计决策**：
- Retriever 封装「嵌入查询 → 向量搜索 → 可选 Rerank」三步。
- Rerank 失败时 FlowLog 警告并回退向量排序（`knowledge.rerank.fallback`）。
- 通过 `knowledgetool.WithRetriever(ctx, retriever)` 注入到工具上下文。

### 5.5 Ingest 流水线（internal/knowledge/ingest.go）

```go
func BuildIndexedChunks(ctx context.Context, embedder QueryEmbedder, p IngestParams) ([]biz.KnowledgeChunk, error)
```

**设计决策**：
- Service：`ExtractDocumentText` → 异步 `BuildIndexedChunks` → Event Bus。
- `IngestParams.Strategy` 驱动 `SplitWithStrategy`；`BatchEmbedder.EmbedBatch` 批量向量化。

### 5.6 Reranker 工厂（internal/knowledge/reranker_factory.go）

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
- Retriever 通过 context 传递（`WithRetriever` / `RetrieverFromContext`），避免全局状态。
- 返回精简的 `chunkSummary`（不含 embedding 向量），减少 Token 消耗。

### 6.2 Agent 装配链

在 `buildToolsetsForAgent` 中：

```go
cfg.KnowledgeSearch = eff[biz.ToolKeyKnowledgeSearch]  // "knowledge_search"
```

当 Agent 工具配置中启用 `knowledge_search` 时，`Assemble` 会将 `knowledgepkg.NewSearchTool()` 加入 `customTools`。

### 6.3 工具开关

`ToolKeyKnowledgeSearch = "knowledge_search"` 定义在 `internal/biz/agent_mcp_effective.go`，通过 effective-tools 机制控制是否装配。

---

## 七、Service 层

### 7.1 KnowledgeService（internal/service/knowledge.go）

```go
type KnowledgeService struct {
    v1.UnimplementedKnowledgeServiceServer
    uc         *biz.KnowledgeUsecase
    chunker    *knowledge.Chunker
    embedder   *knowledge.Embedder
    retriever  *knowledge.Retriever
    bus        event.Bus
}
```

**关键设计**：

| 方法 | 说明 |
|------|------|
| `CreateCollection` | 参数校验 → `uc.CreateCollection` |
| `IngestDocument` | base64 解码 → 创建文档 → `safego.Go` → `BuildIndexedChunks` → 发布 `knowledge_ingest` 事件 |
| `Search` | `retriever.Search`（含可选 rerank）→ Prometheus 计时 |
| `GetEmbedderConfig` / `UpdateEmbedderConfig` | 脱敏读取 / 运行时更新 Embedder（EP-KN-01） |
| `DeleteCollection` | 级联删除（数据库 CASCADE） |
| `DeleteDocument` | 级联删除（数据库 CASCADE） |

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
// internal/service/knowledge_embedder.go — NewKnowledgeEmbedder(c *conf.Data)
// internal/service/knowledge_retriever.go — NewKnowledgeRetriever(emb, repo)
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

---

## 九、待实现设计

以下为对标 trpc-agent-go `knowledge` 包但尚未实现的能力，列出设计方向供后续迭代参考。

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

### 9.3 高级分块策略

| 策略 | trpc-agent-go 对应 | 说明 |
|------|---------------------|------|
| Markdown 按标题 | `chunking/markdown.go` | 按标题层级分块 |
| JSON 结构 | `chunking/json.go` | 按 JSON 结构分块 |
| 递归分块 | `chunking/recursive.go` | 递归字符分割 |

### 9.4 Reranker

✅ 已实现：`reranker_factory.go` + Retriever 集成（KN-01）。以下为扩展方向：

```go
type Reranker interface {
    Rerank(ctx context.Context, query string, documents []string) ([]ScoredDocument, error)
}
```

可选扩展：请求级 rerank 策略选择、FlowLog 指标化。

### 9.5 AgenticFilter

集成 trpc-agent-go `searchfilter` 包，LLM 根据查询动态生成 `UniversalFilterCondition`。

### 9.6 OCR / Extractor

- OCR：集成 `knowledge/ocr/tesseract`，图片/PDF → 文本。
- Extractor：集成 `knowledge/extractor/docling`，PDF/图片 → Markdown。

### 9.7 多租户隔离

SearchFilter 增加 `tenant_id`，向量存储按租户分区，API 层强制注入。

---

## 十、涉及文件

| 文件 | 操作 | 说明 |
|------|------|------|
| `api/kratos/knowledge/v1/knowledge.proto` | ✅ 已实现 | Proto 定义 |
| `internal/biz/knowledge.go` | ✅ 已实现 | Knowledge Usecase + Repo 接口 |
| `internal/data/knowledge.go` | ✅ 已实现 | PostgreSQL + pgvector Repo |
| `internal/service/knowledge.go` | ✅ 已实现 | Knowledge Service |
| `internal/knowledge/chunker.go` | ✅ 已实现 | 文本分块 |
| `internal/knowledge/embedder.go` | ✅ 已实现 | 向量化 |
| `internal/knowledge/ingest.go` | ✅ 已实现 | 分块+向量化流水线 |
| `internal/knowledge/retriever.go` | ✅ 已实现 | 检索 + 可选 rerank |
| `internal/knowledge/reranker_factory.go` | ✅ 已实现 | env Reranker 工厂 |
| `internal/service/knowledge_embedder.go` | ✅ 已实现 | Embedder Wire + DB 回落（EP-KN-01） |
| `internal/biz/knowledge_embed_setting.go` | ✅ 已实现 | Embedder patch 合并 |
| `api/kratos/system_setting/v1/system_setting.proto` | ✅ 已实现 | `KnowledgeEmbedSettings` |
| `internal/service/knowledge_retriever.go` | ✅ 已实现 | Retriever Wire（KN-01） |
| `internal/tools/knowledge/tool.go` | ✅ 已实现 | knowledge_search 工具 |
| `internal/agent/trpc_build.go` | ✅ 已修改 | KnowledgeSearch 开关 |
| `internal/biz/agent_mcp_effective.go` | ✅ 已修改 | ToolKeyKnowledgeSearch |
| `internal/tools/trpc/toolsets.go` | ✅ 已修改 | KnowledgeSearch 装配 |
| `internal/service/wire_providers.go` | ✅ 已修改 | Chunker/Embedder 工厂 |
| `internal/data/data.go` | ✅ 已修改 | NewKnowledgeRepoFromData |
| `internal/server/http.go` | ✅ 已修改 | HTTP 注册 |
| `internal/server/grpc.go` | ✅ 已修改 | gRPC 注册 |
| `web/src/features/knowledge/api.ts` | ✅ 已实现 | 前端 API |
| `web/src/stores/knowledge/index.ts` | ✅ 已实现 | 前端 Store |
