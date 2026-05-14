# Knowledge 知识库模块 — 实现设计文档

> 对应需求：`37 knowledge.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

RAG 知识库：文档导入、分块、向量化、检索增强。对标 trpc-agent-go `knowledge` 包，完整实现 Knowledge 接口，包括文档管理、分块策略、向量化存储、语义搜索、Agentic Filter、OCR 识别、代码搜索、Reranker 重排序等能力。

### 核心架构

```
文档上传 → Extractor(格式转换) → Reader(解析) → Chunking(分块) → Embedder(向量化) → VectorStore(存储)
                                                                                                    ↓
Agent 调用 knowledge_search ← Tool(搜索工具) ← Reranker(重排序) ← Retriever(检索) ← VectorStore(搜索)
```

### trpc-agent-go knowledge 包结构

```
pkg/trpc-agent-go/knowledge/
├── knowledge.go              # Knowledge 接口：Search
├── default.go                # BuiltinKnowledge 实现（含 Load/AddSource/RemoveSource/ReloadSource）
├── default_options.go        # Option 函数（WithVectorStore/WithEmbedder/WithSources 等）
├── chunking/                 # 分块策略
│   ├── chunking.go           # Strategy 接口 + createChunk
│   ├── fixed.go              # 固定大小分块
│   ├── recursive.go          # 递归分块
│   ├── markdown.go           # Markdown 按标题分块
│   └── json.go               # JSON 结构分块
├── document/                 # 文档模型
│   ├── document.go           # Document 结构体
│   └── reader/               # 文档读取器
│       ├── reader.go         # Reader 接口
│       ├── registry.go       # ReaderRegistry 注册表
│       ├── text/             # 纯文本读取器
│       ├── markdown/         # Markdown 读取器
│       ├── json/             # JSON 读取器
│       ├── csv/              # CSV 读取器
│       ├── pdf/              # PDF 读取器
│       ├── docx/             # Word 读取器
│       ├── proto/            # Proto 读取器
│       └── golang/           # Go 源码读取器
├── embedder/                 # 向量化
│   ├── embedder.go           # Embedder 接口
│   ├── openai/               # OpenAI Embedding
│   ├── ollama/               # Ollama Embedding
│   ├── gemini/               # Gemini Embedding
│   └── huggingface/          # HuggingFace Embedding
├── extractor/                # 格式转换（PDF/图片 → 文本/Markdown）
│   ├── extractor.go          # Extractor 接口
│   └── docling/              # Docling 服务实现
├── ocr/                      # OCR 识别
│   ├── ocr.go                # Extractor 接口
│   └── tesseract/            # Tesseract 实现
├── query/                    # 查询增强
│   ├── query.go              # Enhancer 接口
│   └── passthrough.go        # 透传实现
├── reranker/                 # 重排序
│   ├── reranker.go           # Reranker 接口
│   ├── topk/                 # TopK 简单排序
│   ├── cohere/               # Cohere Reranker
│   └── infinity/             # Infinity Reranker
├── retriever/                # 检索器
│   ├── retriever.go          # Retriever 接口
│   └── default.go            # DefaultRetriever（完整 RAG 流水线）
├── searchfilter/             # 搜索过滤
│   ├── filter_condition.go   # UniversalFilterCondition
│   └── builder.go            # 条件构建器（Equal/And/Or/Like 等）
├── source/                   # 数据源
│   └── source.go             # Source 接口 + 元数据常量
├── tool/                     # 知识搜索工具
│   ├── searchtool.go         # knowledge_search 工具
│   └── codesearchtool.go     # code_search 工具
└── vectorstore/              # 向量存储
    ├── vectorstore.go        # VectorStore 接口
    ├── pgvector/             # PostgreSQL pgvector 实现
    ├── inmemory/             # 内存实现
    ├── elasticsearch/        # Elasticsearch 实现
    ├── milvus/               # Milvus 实现
    ├── qdrant/               # Qdrant 实现
    ├── sqlitevec/            # SQLite-vec 实现
    └── tcvector/             # TcVector 实现
```

---

## 二、Proto 层

### 2.1 api/knowledge/v1/knowledge.proto

```protobuf
syntax = "proto3";

package knowledge.v1;

option go_package = "aranea-agents/api/knowledge/v1;knowledgev1";

import "google/api/annotations.proto";
import "google/protobuf/empty.proto";
import "validate/validate.proto";

service KnowledgeService {
  // 知识库 CRUD
  rpc ListKnowledgeBases(ListKnowledgeBasesRequest) returns (ListKnowledgeBasesResponse) {
    option (google.api.http) = { get: "/v1/knowledge-bases" };
  }
  rpc GetKnowledgeBase(GetKnowledgeBaseRequest) returns (KnowledgeBase) {
    option (google.api.http) = { get: "/v1/knowledge-bases/{id}" };
  }
  rpc CreateKnowledgeBase(CreateKnowledgeBaseRequest) returns (KnowledgeBase) {
    option (google.api.http) = { post: "/v1/knowledge-bases" body: "*" };
  }
  rpc UpdateKnowledgeBase(UpdateKnowledgeBaseRequest) returns (KnowledgeBase) {
    option (google.api.http) = { put: "/v1/knowledge-bases/{id}" body: "*" };
  }
  rpc DeleteKnowledgeBase(DeleteKnowledgeBaseRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/knowledge-bases/{id}" };
  }

  // 文档管理
  rpc UploadDocument(UploadDocumentRequest) returns (Document) {
    option (google.api.http) = { post: "/v1/knowledge-bases/{knowledge_base_id}/documents" body: "*" };
  }
  rpc ListDocuments(ListDocumentsRequest) returns (ListDocumentsResponse) {
    option (google.api.http) = { get: "/v1/knowledge-bases/{knowledge_base_id}/documents" };
  }
  rpc GetDocument(GetDocumentRequest) returns (Document) {
    option (google.api.http) = { get: "/v1/knowledge-bases/{knowledge_base_id}/documents/{id}" };
  }
  rpc DeleteDocument(DeleteDocumentRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/knowledge-bases/{knowledge_base_id}/documents/{id}" };
  }
  rpc ReindexDocument(ReindexDocumentRequest) returns (Document) {
    option (google.api.http) = { post: "/v1/knowledge-bases/{knowledge_base_id}/documents/{id}/reindex" body: "*" };
  }

  // 知识搜索
  rpc SearchKnowledge(SearchKnowledgeRequest) returns (SearchKnowledgeResponse) {
    option (google.api.http) = { post: "/v1/knowledge-bases/{knowledge_base_id}/search" body: "*" };
  }

  // 知识库统计
  rpc GetKnowledgeBaseStats(GetKnowledgeBaseStatsRequest) returns (KnowledgeBaseStats) {
    option (google.api.http) = { get: "/v1/knowledge-bases/{id}/stats" };
  }
}

// --- 知识库 ---

message KnowledgeBase {
  string id = 1;
  string name = 2;
  string description = 3;
  string embedding_provider = 4;
  string embedding_model = 5;
  int32 embedding_dimension = 6;
  string chunk_strategy = 7;       // "fixed"/"recursive"/"markdown"/"json"
  int32 chunk_size = 8;
  int32 chunk_overlap = 9;
  string vector_store_type = 10;   // "pgvector"/"inmemory"
  string reranker_type = 11;       // "topk"/"cohere"/"infinity"/""
  bool enable_source_sync = 12;
  bool enable_agentic_filter = 13;
  string agent_id = 14;            // 绑定 Agent
  int32 doc_count = 15;
  int32 total_chunks = 16;
  string status = 17;              // "active"/"inactive"/"error"
  string created_at = 18;
  string updated_at = 19;
}

message ListKnowledgeBasesRequest {
  string keyword = 1;
  int32 page = 2;
  int32 page_size = 3;
  string agent_id = 4;
}

message ListKnowledgeBasesResponse {
  repeated KnowledgeBase items = 1;
  int32 total = 2;
}

message GetKnowledgeBaseRequest {
  string id = 1;
}

message CreateKnowledgeBaseRequest {
  string name = 1 [(validate.rules).string.min_len = 1];
  string description = 2;
  string embedding_provider = 3 [(validate.rules).string.min_len = 1];
  string embedding_model = 4 [(validate.rules).string.min_len = 1];
  int32 embedding_dimension = 5;
  string chunk_strategy = 6;
  int32 chunk_size = 7;
  int32 chunk_overlap = 8;
  string vector_store_type = 9;
  string reranker_type = 10;
  bool enable_source_sync = 11;
  bool enable_agentic_filter = 12;
  string agent_id = 13;
}

message UpdateKnowledgeBaseRequest {
  string id = 1;
  string name = 2;
  string description = 3;
  string chunk_strategy = 4;
  int32 chunk_size = 5;
  int32 chunk_overlap = 6;
  string reranker_type = 7;
  bool enable_source_sync = 8;
  bool enable_agentic_filter = 9;
  string agent_id = 10;
  string status = 11;
}

message DeleteKnowledgeBaseRequest {
  string id = 1;
}

// --- 文档 ---

message Document {
  string id = 1;
  string knowledge_base_id = 2;
  string name = 3;
  string mime_type = 4;
  int64 size = 5;
  int32 chunk_count = 6;
  string source_type = 7;          // "file"/"url"/"text"
  string source_uri = 8;
  string status = 9;               // "pending"/"processing"/"ready"/"error"
  string error_message = 10;
  string created_at = 11;
  string updated_at = 12;
}

message UploadDocumentRequest {
  string knowledge_base_id = 1;
  string name = 2;
  string content_type = 3;         // MIME type
  bytes content = 4;               // 文件内容
  string source_type = 5;          // "file"/"url"/"text"
  string source_uri = 6;
  string text_content = 7;         // 纯文本内容（source_type=text 时使用）
  map<string, string> metadata = 8;
}

message ListDocumentsRequest {
  string knowledge_base_id = 1;
  string keyword = 2;
  string status = 3;
  int32 page = 4;
  int32 page_size = 5;
}

message ListDocumentsResponse {
  repeated Document items = 1;
  int32 total = 2;
}

message GetDocumentRequest {
  string knowledge_base_id = 1;
  string id = 2;
}

message DeleteDocumentRequest {
  string knowledge_base_id = 1;
  string id = 2;
}

message ReindexDocumentRequest {
  string knowledge_base_id = 1;
  string id = 2;
}

// --- 搜索 ---

message SearchKnowledgeRequest {
  string knowledge_base_id = 1;
  string query = 2;
  int32 max_results = 3;
  double min_score = 4;
  int32 search_mode = 5;           // 0=hybrid, 1=vector, 2=keyword, 3=filter
  UniversalFilterCondition filter = 6;
  repeated ConversationMessage history = 7;
}

message ConversationMessage {
  string role = 1;
  string content = 2;
  int64 timestamp = 3;
}

message UniversalFilterCondition {
  string field = 1;
  string operator = 2;             // eq/ne/gt/gte/lt/lte/in/not in/like/not like/between/and/or
  google.protobuf.Any value = 3;   // 比较值或子条件数组
}

message SearchKnowledgeResponse {
  repeated SearchHit hits = 1;
  string enhanced_query = 2;
}

message SearchHit {
  string document_id = 1;
  string document_name = 2;
  string content = 3;
  double score = 4;
  map<string, string> metadata = 5;
  int32 chunk_index = 6;
}

// --- 统计 ---

message GetKnowledgeBaseStatsRequest {
  string id = 1;
}

message KnowledgeBaseStats {
  int32 document_count = 1;
  int32 total_chunks = 2;
  int64 total_size_bytes = 3;
  int32 error_count = 4;
  string last_indexed_at = 5;
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type KnowledgeBase struct {
    ID                string
    Name              string
    Description       string
    EmbeddingProvider string
    EmbeddingModel    string
    EmbeddingDimension int
    ChunkStrategy     string
    ChunkSize         int
    ChunkOverlap      int
    VectorStoreType   string
    RerankerType      string
    EnableSourceSync  bool
    EnableAgenticFilter bool
    AgentID           string
    DocCount          int
    TotalChunks       int
    Status            string
    CreatedAt         time.Time
    UpdatedAt         time.Time
}

type KnowledgeDocument struct {
    ID              string
    KnowledgeBaseID string
    Name            string
    MimeType        string
    Size            int64
    ChunkCount      int
    SourceType      string
    SourceURI       string
    Status          string
    ErrorMessage    string
    Metadata        map[string]string
    CreatedAt       time.Time
    UpdatedAt       time.Time
}

type SearchHit struct {
    DocumentID   string
    DocumentName string
    Content      string
    Score        float64
    Metadata     map[string]string
    ChunkIndex   int
}

type SearchParams struct {
    Query       string
    MaxResults  int
    MinScore    float64
    SearchMode  int
    Filter      *FilterCondition
    History     []ConversationMessage
}

type ConversationMessage struct {
    Role      string
    Content   string
    Timestamp int64
}

type FilterCondition struct {
    Field    string
    Operator string
    Value    any
}
```

### 3.2 Repo 接口

```go
type KnowledgeBaseRepo interface {
    Create(ctx context.Context, kb *KnowledgeBase) (*KnowledgeBase, error)
    Get(ctx context.Context, id string) (*KnowledgeBase, error)
    List(ctx context.Context, keyword, agentID string, page, pageSize int) ([]*KnowledgeBase, int, error)
    Update(ctx context.Context, kb *KnowledgeBase) (*KnowledgeBase, error)
    Delete(ctx context.Context, id string) error
}

type KnowledgeDocumentRepo interface {
    Create(ctx context.Context, doc *KnowledgeDocument) (*KnowledgeDocument, error)
    Get(ctx context.Context, kbID, id string) (*KnowledgeDocument, error)
    List(ctx context.Context, kbID, keyword, status string, page, pageSize int) ([]*KnowledgeDocument, int, error)
    Update(ctx context.Context, doc *KnowledgeDocument) (*KnowledgeDocument, error)
    Delete(ctx context.Context, kbID, id string) error
    UpdateStatus(ctx context.Context, kbID, id, status, errMsg string) error
    IncrementChunkCount(ctx context.Context, kbID, id string, delta int) error
}
```

### 3.3 Usecase

```go
type KnowledgeUsecase struct {
    kbRepo     KnowledgeBaseRepo
    docRepo    KnowledgeDocumentRepo
    kbFactory  KnowledgeBaseFactory
    log        *log.Helper
}

func (uc *KnowledgeUsecase) CreateKB(ctx context.Context, kb *KnowledgeBase) (*KnowledgeBase, error)
func (uc *KnowledgeUsecase) GetKB(ctx context.Context, id string) (*KnowledgeBase, error)
func (uc *KnowledgeUsecase) ListKBs(ctx context.Context, keyword, agentID string, page, pageSize int) ([]*KnowledgeBase, int, error)
func (uc *KnowledgeUsecase) UpdateKB(ctx context.Context, kb *KnowledgeBase) (*KnowledgeBase, error)
func (uc *KnowledgeUsecase) DeleteKB(ctx context.Context, id string) error

func (uc *KnowledgeUsecase) UploadDocument(ctx context.Context, kbID string, doc *KnowledgeDocument, content []byte) (*KnowledgeDocument, error)
func (uc *KnowledgeUsecase) GetDocument(ctx context.Context, kbID, id string) (*KnowledgeDocument, error)
func (uc *KnowledgeUsecase) ListDocuments(ctx context.Context, kbID, keyword, status string, page, pageSize int) ([]*KnowledgeDocument, int, error)
func (uc *KnowledgeUsecase) DeleteDocument(ctx context.Context, kbID, id string) error
func (uc *KnowledgeUsecase) ReindexDocument(ctx context.Context, kbID, id string) (*KnowledgeDocument, error)

func (uc *KnowledgeUsecase) Search(ctx context.Context, kbID string, params *SearchParams) ([]*SearchHit, string, error)
func (uc *KnowledgeUsecase) GetStats(ctx context.Context, kbID string) (*KnowledgeBaseStats, error)
```

### 3.4 KnowledgeBaseFactory

```go
type KnowledgeBaseFactory interface {
    BuildKnowledge(ctx context.Context, kb *KnowledgeBase) (knowledge.Knowledge, error)
}
```

Factory 负责根据 KnowledgeBase 配置构建 `knowledge.Knowledge` 实例：
- 创建 Embedder（基于 provider/model 配置）
- 创建 VectorStore（基于 vector_store_type 配置）
- 创建 Reranker（基于 reranker_type 配置）
- 创建 BuiltinKnowledge 实例

---

## 四、Data 层

### 4.1 Ent Schema

**internal/data/ent/schema/knowledge_base.go**

```go
package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/dialect/entsql"
    "entgo.io/ent/schema"
    "entgo.io/ent/schema/field"
    "entgo.io/ent/schema/index"
)

type KnowledgeBase struct {
    ent.Schema
}

func (KnowledgeBase) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "knowledge_bases"},
    }
}

func (KnowledgeBase) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(64),
        field.String("name").MaxLen(256),
        field.Text("description").Default(""),
        field.String("embedding_provider").MaxLen(128),
        field.String("embedding_model").MaxLen(256),
        field.Int("embedding_dimension").Default(1536),
        field.String("chunk_strategy").Default("fixed"),
        field.Int("chunk_size").Default(1024),
        field.Int("chunk_overlap").Default(128),
        field.String("vector_store_type").Default("pgvector"),
        field.String("reranker_type").Default("topk"),
        field.Bool("enable_source_sync").Default(false),
        field.Bool("enable_agentic_filter").Default(false),
        field.String("agent_id").Default("").MaxLen(64),
        field.Int("doc_count").Default(0),
        field.Int("total_chunks").Default(0),
        field.String("status").Default("active").MaxLen(32),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
    }
}

func (KnowledgeBase) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("agent_id"),
        index.Fields("status"),
    }
}
```

**internal/data/ent/schema/knowledge_document.go**

```go
package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/dialect/entsql"
    "entgo.io/ent/schema"
    "entgo.io/ent/schema/field"
    "entgo.io/ent/schema/index"
)

type KnowledgeDocument struct {
    ent.Schema
}

func (KnowledgeDocument) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "knowledge_documents"},
    }
}

func (KnowledgeDocument) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(64),
        field.String("knowledge_base_id").MaxLen(64),
        field.String("name").MaxLen(512),
        field.String("mime_type").Default("").MaxLen(128),
        field.Int64("size").Default(0),
        field.Int("chunk_count").Default(0),
        field.String("source_type").Default("file").MaxLen(32),
        field.Text("source_uri").Default(""),
        field.String("status").Default("pending").MaxLen(32),
        field.Text("error_message").Default(""),
        field.Text("metadata_json").Default("{}"),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
    }
}

func (KnowledgeDocument) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("knowledge_base_id"),
        index.Fields("knowledge_base_id", "status"),
    }
}
```

### 4.2 向量存储扩展

**internal/data/pgvector/knowledge.go** — 扩展现有 pgvector Store 以支持 Knowledge 向量存储

```go
package pgvector

type KnowledgeVectorStore struct {
    db  *sql.DB
    dim int
}

func NewKnowledgeVectorStore(db *sql.DB, dim int) *KnowledgeVectorStore

func (s *KnowledgeVectorStore) CreateTable(ctx context.Context, kbID string) error
func (s *KnowledgeVectorStore) DropTable(ctx context.Context, kbID string) error
func (s *KnowledgeVectorStore) InsertChunks(ctx context.Context, kbID string, chunks []*ChunkRecord) error
func (s *KnowledgeVectorStore) DeleteByDocumentID(ctx context.Context, kbID, documentID string) error
func (s *KnowledgeVectorStore) DeleteByFilter(ctx context.Context, kbID string, filter map[string]any) error
func (s *KnowledgeVectorStore) Search(ctx context.Context, kbID string, embedding []float32, topK int, minScore float64, filter map[string]any) ([]*SearchResult, error)
func (s *KnowledgeVectorStore) Count(ctx context.Context, kbID string) (int, error)
func (s *KnowledgeVectorStore) GetMetadata(ctx context.Context, kbID string, filter map[string]any) (map[string]map[string]any, error)

type ChunkRecord struct {
    ID         string
    DocumentID string
    Content    string
    Embedding  []float32
    Metadata   map[string]any
    ChunkIndex int
    CreatedAt  time.Time
}

type SearchResult struct {
    ID         string
    DocumentID string
    Content    string
    Score      float64
    Metadata   map[string]any
    ChunkIndex int
}
```

每个 KnowledgeBase 对应一张 pgvector 表 `knowledge_{kbID}_{dim}`，表结构：

```sql
CREATE TABLE IF NOT EXISTS knowledge_{kbID}_{dim} (
    id TEXT PRIMARY KEY,
    document_id TEXT NOT NULL,
    content TEXT NOT NULL,
    embedding vector({dim}) NOT NULL,
    metadata JSONB DEFAULT '{}',
    chunk_index INT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_knowledge_{kbID}_doc ON knowledge_{kbID}_{dim} (document_id);
```

### 4.3 KnowledgeBaseFactory 实现

**internal/knowledge/factory.go**

```go
package knowledge

type BuiltinKnowledgeBaseFactory struct {
    providerRepo biz.LLMProviderModelRepo
    pgDB         *sql.DB
}

func NewKnowledgeBaseFactory(providerRepo biz.LLMProviderModelRepo, pgDB *sql.DB) *BuiltinKnowledgeBaseFactory

func (f *BuiltinKnowledgeBaseFactory) BuildKnowledge(ctx context.Context, kb *biz.KnowledgeBase) (knowledge.Knowledge, error) {
    // 1. 根据 kb.EmbeddingProvider + kb.EmbeddingModel 创建 Embedder
    embedder := f.buildEmbedder(ctx, kb)

    // 2. 创建 VectorStore（pgvector）
    vs := f.buildVectorStore(ctx, kb)

    // 3. 创建 Reranker
    reranker := f.buildReranker(ctx, kb)

    // 4. 构建 BuiltinKnowledge
    k := knowledge.New(
        knowledge.WithVectorStore(vs),
        knowledge.WithEmbedder(embedder),
        knowledge.WithReranker(reranker),
        knowledge.WithEnableSourceSync(kb.EnableSourceSync),
    )

    return k, nil
}

func (f *BuiltinKnowledgeBaseFactory) buildEmbedder(ctx context.Context, kb *biz.KnowledgeBase) embedder.Embedder {
    // 根据 provider 选择 embedder 实现
    switch kb.EmbeddingProvider {
    case "openai":
        return openaiembedder.New(...)
    case "ollama":
        return ollamaembedder.New(...)
    case "gemini":
        return geminiembedder.New(...)
    default:
        return openaiembedder.New(...)
    }
}

func (f *BuiltinKnowledgeBaseFactory) buildVectorStore(ctx context.Context, kb *biz.KnowledgeBase) vectorstore.VectorStore {
    switch kb.VectorStoreType {
    case "pgvector":
        return pgvectorvs.New(...)
    case "inmemory":
        return inmemoryvs.New()
    default:
        return pgvectorvs.New(...)
    }
}

func (f *BuiltinKnowledgeBaseFactory) buildReranker(ctx context.Context, kb *biz.KnowledgeBase) reranker.Reranker {
    switch kb.RerankerType {
    case "topk":
        return topk.New()
    case "cohere":
        return cohere.New(...)
    case "infinity":
        return infinity.New(...)
    default:
        return topk.New()
    }
}
```

### 4.4 文档处理流水线

**internal/knowledge/pipeline.go**

```go
package knowledge

type DocumentPipeline struct {
    extractor  extractor.Extractor
    reader     documentreader.Reader
    chunker    chunking.Strategy
    embedder   embedder.Embedder
    vectorStore vectorstore.VectorStore
}

func NewDocumentPipeline(
    extractor extractor.Extractor,
    reader documentreader.Reader,
    chunker chunking.Strategy,
    embedder embedder.Embedder,
    vs vectorstore.VectorStore,
) *DocumentPipeline

func (p *DocumentPipeline) Process(ctx context.Context, doc *biz.KnowledgeDocument, content []byte) error {
    // 1. 格式转换（PDF/图片 → Markdown/Text）
    var reader io.Reader
    format := "text"
    if p.extractor != nil && len(content) > 0 {
        result, err := p.extractor.Extract(ctx, content)
        if err != nil {
            return fmt.Errorf("extract: %w", err)
        }
        reader = result.Reader
        format = result.Format
    } else {
        reader = bytes.NewReader(content)
    }

    // 2. 文档解析 → []*document.Document
    docs, err := p.reader.ReadFromReader(ctx, reader, documentreader.WithFormat(format))
    if err != nil {
        return fmt.Errorf("read: %w", err)
    }

    // 3. 分块
    var allChunks []*document.Document
    for _, d := range docs {
        chunks, err := p.chunker.Chunk(d)
        if err != nil {
            return fmt.Errorf("chunk: %w", err)
        }
        allChunks = append(allChunks, chunks...)
    }

    // 4. 向量化 + 存储
    for _, chunk := range allChunks {
        embedding, err := p.embedder.GetEmbedding(ctx, chunk.Content)
        if err != nil {
            return fmt.Errorf("embed: %w", err)
        }
        if len(embedding) == 0 {
            continue
        }
        if err := p.vectorStore.Add(ctx, chunk, embedding); err != nil {
            return fmt.Errorf("store: %w", err)
        }
    }

    return nil
}
```

### 4.5 Knowledge Search Tool 集成

**internal/knowledge/tool_adapter.go**

```go
package knowledge

type KnowledgeToolAdapter struct {
    factory KnowledgeBaseFactory
}

func NewKnowledgeToolAdapter(factory KnowledgeBaseFactory) *KnowledgeToolAdapter

func (a *KnowledgeToolAdapter) BuildSearchTool(ctx context.Context, kb *biz.KnowledgeBase) (tool.Tool, error) {
    k, err := a.factory.BuildKnowledge(ctx, kb)
    if err != nil {
        return nil, err
    }

    opts := []knowledgetool.Option{
        knowledgetool.WithMaxResults(10),
        knowledgetool.WithMinScore(0.0),
    }

    if kb.EnableAgenticFilter {
        opts = append(opts, knowledgetool.WithToolName("knowledge_search_with_agentic_filter"))
    }

    searchTool, err := knowledgetool.NewKnowledgeSearchTool(k, opts...)
    if err != nil {
        return nil, err
    }

    return searchTool, nil
}

func (a *KnowledgeToolAdapter) BuildCodeSearchTool(ctx context.Context, kb *biz.KnowledgeBase) (tool.Tool, error) {
    k, err := a.factory.BuildKnowledge(ctx, kb)
    if err != nil {
        return nil, err
    }

    codeTool, err := knowledgetool.NewCodeSearchTool(k)
    if err != nil {
        return nil, err
    }

    return codeTool, nil
}
```

### 4.6 Agent 集成

**修改 internal/agent/trpc_build.go**

```go
func BuildTRPCLLMAgent(ctx context.Context, cfg *AgentConfig, ...) (*llmagent.LLMAgent, error) {
    // ... 现有构建逻辑 ...

    // 注入 Knowledge
    if cfg.KnowledgeBaseID != "" {
        kb, err := knowledgeUsecase.GetKB(ctx, cfg.KnowledgeBaseID)
        if err != nil {
            return nil, fmt.Errorf("get knowledge base: %w", err)
        }

        k, err := factory.BuildKnowledge(ctx, kb)
        if err != nil {
            return nil, fmt.Errorf("build knowledge: %w", err)
        }

        opts = append(opts,
            llmagent.WithKnowledge(k),
        )

        if kb.EnableAgenticFilter {
            opts = append(opts,
                llmagent.WithEnableKnowledgeAgenticFilter(true),
            )
        }

        // 添加 knowledge_search 工具
        searchTool, err := toolAdapter.BuildSearchTool(ctx, kb)
        if err != nil {
            return nil, fmt.Errorf("build search tool: %w", err)
        }
        tools = append(tools, searchTool)
    }

    // ... 继续构建 ...
}
```

### 4.7 Repo 实现

**internal/data/knowledge_repo.go**

```go
package data

type knowledgeBaseRepo struct {
    data *Data
}

func NewKnowledgeBaseRepo(data *Data) biz.KnowledgeBaseRepo {
    return &knowledgeBaseRepo{data: data}
}

func (r *knowledgeBaseRepo) Create(ctx context.Context, kb *biz.KnowledgeBase) (*biz.KnowledgeBase, error) {
    now := time.Now().Format(time.RFC3339)
    build := r.data.entClient.KnowledgeBase.Create().
        SetID(kb.ID).
        SetName(kb.Name).
        SetDescription(kb.Description).
        SetEmbeddingProvider(kb.EmbeddingProvider).
        SetEmbeddingModel(kb.EmbeddingModel).
        SetEmbeddingDimension(kb.EmbeddingDimension).
        SetChunkStrategy(kb.ChunkStrategy).
        SetChunkSize(kb.ChunkSize).
        SetChunkOverlap(kb.ChunkOverlap).
        SetVectorStoreType(kb.VectorStoreType).
        SetRerankerType(kb.RerankerType).
        SetEnableSourceSync(kb.EnableSourceSync).
        SetEnableAgenticFilter(kb.EnableAgenticFilter).
        SetAgentID(kb.AgentID).
        SetStatus(kb.Status).
        SetCreatedAt(now).
        SetUpdatedAt(now)
    saved, err := build.Save(ctx)
    if err != nil {
        return nil, err
    }
    return entKBToBiz(saved), nil
}

func (r *knowledgeBaseRepo) Get(ctx context.Context, id string) (*biz.KnowledgeBase, error) {
    entKB, err := r.data.entClient.KnowledgeBase.Get(ctx, id)
    if err != nil {
        return nil, err
    }
    return entKBToBiz(entKB), nil
}

func (r *knowledgeBaseRepo) List(ctx context.Context, keyword, agentID string, page, pageSize int) ([]*biz.KnowledgeBase, int, error) {
    query := r.data.entClient.KnowledgeBase.Query()
    if keyword != "" {
        query = query.Where(knowledgebase.Or(
            knowledgebase.NameContains(keyword),
            knowledgebase.DescriptionContains(keyword),
        ))
    }
    if agentID != "" {
        query = query.Where(knowledgebase.AgentID(agentID))
    }
    total, err := query.Count(ctx)
    if err != nil {
        return nil, 0, err
    }
    items, err := query.
        Offset((page - 1) * pageSize).
        Limit(pageSize).
        Order(ent.Desc(knowledgebase.FieldCreatedAt)).
        All(ctx)
    if err != nil {
        return nil, 0, err
    }
    result := make([]*biz.KnowledgeBase, 0, len(items))
    for _, item := range items {
        result = append(result, entKBToBiz(item))
    }
    return result, total, nil
}

func (r *knowledgeBaseRepo) Update(ctx context.Context, kb *biz.KnowledgeBase) (*biz.KnowledgeBase, error) {
    build := r.data.entClient.KnowledgeBase.UpdateOneID(kb.ID).
        SetName(kb.Name).
        SetDescription(kb.Description).
        SetChunkStrategy(kb.ChunkStrategy).
        SetChunkSize(kb.ChunkSize).
        SetChunkOverlap(kb.ChunkOverlap).
        SetRerankerType(kb.RerankerType).
        SetEnableSourceSync(kb.EnableSourceSync).
        SetEnableAgenticFilter(kb.EnableAgenticFilter).
        SetAgentID(kb.AgentID).
        SetStatus(kb.Status).
        SetUpdatedAt(time.Now().Format(time.RFC3339))
    saved, err := build.Save(ctx)
    if err != nil {
        return nil, err
    }
    return entKBToBiz(saved), nil
}

func (r *knowledgeBaseRepo) Delete(ctx context.Context, id string) error {
    return r.data.entClient.KnowledgeBase.DeleteOneID(id).Exec(ctx)
}
```

**internal/data/knowledge_document_repo.go**

```go
package data

type knowledgeDocumentRepo struct {
    data *Data
}

func NewKnowledgeDocumentRepo(data *Data) biz.KnowledgeDocumentRepo {
    return &knowledgeDocumentRepo{data: data}
}

func (r *knowledgeDocumentRepo) Create(ctx context.Context, doc *biz.KnowledgeDocument) (*biz.KnowledgeDocument, error) {
    metadataJSON, _ := json.Marshal(doc.Metadata)
    now := time.Now().Format(time.RFC3339)
    build := r.data.entClient.KnowledgeDocument.Create().
        SetID(doc.ID).
        SetKnowledgeBaseID(doc.KnowledgeBaseID).
        SetName(doc.Name).
        SetMimeType(doc.MimeType).
        SetSize(doc.Size).
        SetSourceType(doc.SourceType).
        SetSourceURI(doc.SourceURI).
        SetStatus(doc.Status).
        SetMetadataJSON(string(metadataJSON)).
        SetCreatedAt(now).
        SetUpdatedAt(now)
    saved, err := build.Save(ctx)
    if err != nil {
        return nil, err
    }
    return entDocToBiz(saved), nil
}

func (r *knowledgeDocumentRepo) UpdateStatus(ctx context.Context, kbID, id, status, errMsg string) error {
    build := r.data.entClient.KnowledgeDocument.UpdateOneID(id).
        SetStatus(status).
        SetUpdatedAt(time.Now().Format(time.RFC3339))
    if errMsg != "" {
        build = build.SetErrorMessage(errMsg)
    }
    _, err := build.Save(ctx)
    return err
}

func (r *knowledgeDocumentRepo) IncrementChunkCount(ctx context.Context, kbID, id string, delta int) error {
    doc, err := r.data.entClient.KnowledgeDocument.Get(ctx, id)
    if err != nil {
        return err
    }
    _, err = r.data.entClient.KnowledgeDocument.UpdateOneID(id).
        SetChunkCount(doc.ChunkCount + delta).
        SetUpdatedAt(time.Now().Format(time.RFC3339)).
        Save(ctx)
    return err
}
```

---

## 五、Service 层

```go
type KnowledgeService struct {
    v1.UnimplementedKnowledgeServiceServer
    uc *biz.KnowledgeUsecase
}

func NewKnowledgeService(uc *biz.KnowledgeUsecase) *KnowledgeService {
    return &KnowledgeService{uc: uc}
}

func (s *KnowledgeService) CreateKnowledgeBase(ctx context.Context, req *v1.CreateKnowledgeBaseRequest) (*v1.KnowledgeBase, error) {
    kb := &biz.KnowledgeBase{
        ID:                uuid.New().String(),
        Name:              req.Name,
        Description:       req.Description,
        EmbeddingProvider: req.EmbeddingProvider,
        EmbeddingModel:    req.EmbeddingModel,
        EmbeddingDimension: int(req.EmbeddingDimension),
        ChunkStrategy:     req.ChunkStrategy,
        ChunkSize:         int(req.ChunkSize),
        ChunkOverlap:      int(req.ChunkOverlap),
        VectorStoreType:   req.VectorStoreType,
        RerankerType:      req.RerankerType,
        EnableSourceSync:  req.EnableSourceSync,
        EnableAgenticFilter: req.EnableAgenticFilter,
        AgentID:           req.AgentId,
        Status:            "active",
    }
    result, err := s.uc.CreateKB(ctx, kb)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return toProtoKnowledgeBase(result), nil
}

func (s *KnowledgeService) UploadDocument(ctx context.Context, req *v1.UploadDocumentRequest) (*v1.Document, error) {
    doc := &biz.KnowledgeDocument{
        ID:              uuid.New().String(),
        KnowledgeBaseID: req.KnowledgeBaseId,
        Name:            req.Name,
        MimeType:        req.ContentType,
        SourceType:      req.SourceType,
        SourceURI:       req.SourceUri,
        Status:          "pending",
    }
    result, err := s.uc.UploadDocument(ctx, req.KnowledgeBaseId, doc, req.Content)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return toProtoDocument(result), nil
}

func (s *KnowledgeService) SearchKnowledge(ctx context.Context, req *v1.SearchKnowledgeRequest) (*v1.SearchKnowledgeResponse, error) {
    params := &biz.SearchParams{
        Query:      req.Query,
        MaxResults: int(req.MaxResults),
        MinScore:   req.MinScore,
        SearchMode: int(req.SearchMode),
    }
    if req.Filter != nil {
        params.Filter = &biz.FilterCondition{
            Field:    req.Filter.Field,
            Operator: req.Filter.Operator,
            Value:    req.Filter.Value,
        }
    }
    for _, msg := range req.History {
        params.History = append(params.History, biz.ConversationMessage{
            Role:      msg.Role,
            Content:   msg.Content,
            Timestamp: msg.Timestamp,
        })
    }
    hits, enhancedQuery, err := s.uc.Search(ctx, req.KnowledgeBaseId, params)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    resp := &v1.SearchKnowledgeResponse{EnhancedQuery: enhancedQuery}
    for _, hit := range hits {
        resp.Hits = append(resp.Hits, toProtoSearchHit(hit))
    }
    return resp, nil
}

func (s *KnowledgeService) DeleteKnowledgeBase(ctx context.Context, req *v1.DeleteKnowledgeBaseRequest) (*emptypb.Empty, error) {
    if err := s.uc.DeleteKB(ctx, req.Id); err != nil {
        return nil, kerrors.FromError(err)
    }
    return &emptypb.Empty{}, nil
}
```

---

## 六、Wire 注入

```go
// internal/data/data.go — ProviderSet 新增
var ProviderSet = wire.NewSet(
    // ... 现有 providers ...
    NewKnowledgeBaseRepo,
    NewKnowledgeDocumentRepo,
    NewKnowledgeVectorStore,
)

// internal/biz/biz.go — ProviderSet 新增
var ProviderSet = wire.NewSet(
    // ... 现有 providers ...
    NewKnowledgeUsecase,
)

// internal/service/service.go — ProviderSet 新增
var ProviderSet = wire.NewSet(
    // ... 现有 providers ...
    NewKnowledgeService,
)

// internal/knowledge/knowledge.go — ProviderSet 新增
var ProviderSet = wire.NewSet(
    NewKnowledgeBaseFactory,
    NewKnowledgeToolAdapter,
    NewDocumentPipeline,
)
```

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/features/knowledge/
├── api.ts
├── types.ts
├── stores/
│   └── knowledgeStore.ts
├── components/
│   ├── KnowledgeListPage.vue
│   ├── KnowledgeDetailPage.vue
│   ├── KnowledgeBaseForm.vue
│   ├── DocumentUploadZone.vue
│   ├── DocumentList.vue
│   ├── DocumentStatusBadge.vue
│   ├── KnowledgeSearchDialog.vue
│   ├── SearchHitCard.vue
│   ├── FilterConditionBuilder.vue
│   └── KnowledgeBaseStats.vue
└── routes.ts
```

### 7.2 类型定义

**types.ts**

```typescript
export interface KnowledgeBase {
  id: string
  name: string
  description: string
  embedding_provider: string
  embedding_model: string
  embedding_dimension: number
  chunk_strategy: 'fixed' | 'recursive' | 'markdown' | 'json'
  chunk_size: number
  chunk_overlap: number
  vector_store_type: 'pgvector' | 'inmemory'
  reranker_type: 'topk' | 'cohere' | 'infinity' | ''
  enable_source_sync: boolean
  enable_agentic_filter: boolean
  agent_id: string
  doc_count: number
  total_chunks: number
  status: 'active' | 'inactive' | 'error'
  created_at: string
  updated_at: string
}

export interface KnowledgeDocument {
  id: string
  knowledge_base_id: string
  name: string
  mime_type: string
  size: number
  chunk_count: number
  source_type: 'file' | 'url' | 'text'
  source_uri: string
  status: 'pending' | 'processing' | 'ready' | 'error'
  error_message: string
  created_at: string
  updated_at: string
}

export interface SearchHit {
  document_id: string
  document_name: string
  content: string
  score: number
  metadata: Record<string, string>
  chunk_index: number
}

export interface FilterCondition {
  field: string
  operator: string
  value: any
}

export interface KnowledgeBaseStats {
  document_count: number
  total_chunks: number
  total_size_bytes: number
  error_count: number
  last_indexed_at: string
}
```

### 7.3 API

**api.ts**

```typescript
import axios from 'axios'
import type {
  KnowledgeBase, KnowledgeDocument, SearchHit,
  FilterCondition, KnowledgeBaseStats
} from './types'

const BASE = '/v1/knowledge-bases'

export async function listKnowledgeBases(params: {
  keyword?: string; agent_id?: string; page?: number; page_size?: number
}): Promise<{ items: KnowledgeBase[]; total: number }> {
  const { data } = await axios.get(BASE, { params })
  return data
}

export async function getKnowledgeBase(id: string): Promise<KnowledgeBase> {
  const { data } = await axios.get(`${BASE}/${id}`)
  return data
}

export async function createKnowledgeBase(req: Partial<KnowledgeBase>): Promise<KnowledgeBase> {
  const { data } = await axios.post(BASE, req)
  return data
}

export async function updateKnowledgeBase(id: string, req: Partial<KnowledgeBase>): Promise<KnowledgeBase> {
  const { data } = await axios.put(`${BASE}/${id}`, req)
  return data
}

export async function deleteKnowledgeBase(id: string): Promise<void> {
  await axios.delete(`${BASE}/${id}`)
}

export async function uploadDocument(kbId: string, file: File, metadata?: Record<string, string>): Promise<KnowledgeDocument> {
  const formData = new FormData()
  formData.append('content', file)
  formData.append('name', file.name)
  formData.append('content_type', file.type)
  formData.append('source_type', 'file')
  if (metadata) {
    for (const [k, v] of Object.entries(metadata)) {
      formData.append(`metadata[${k}]`, v)
    }
  }
  const { data } = await axios.post(`${BASE}/${kbId}/documents`, formData, {
    headers: { 'Content-Type': 'multipart/form-data' }
  })
  return data
}

export async function uploadTextDocument(kbId: string, name: string, text: string, metadata?: Record<string, string>): Promise<KnowledgeDocument> {
  const { data } = await axios.post(`${BASE}/${kbId}/documents`, {
    name,
    content_type: 'text/plain',
    source_type: 'text',
    text_content: text,
    metadata
  })
  return data
}

export async function uploadURLDocument(kbId: string, name: string, url: string, metadata?: Record<string, string>): Promise<KnowledgeDocument> {
  const { data } = await axios.post(`${BASE}/${kbId}/documents`, {
    name,
    source_type: 'url',
    source_uri: url,
    metadata
  })
  return data
}

export async function listDocuments(kbId: string, params: {
  keyword?: string; status?: string; page?: number; page_size?: number
}): Promise<{ items: KnowledgeDocument[]; total: number }> {
  const { data } = await axios.get(`${BASE}/${kbId}/documents`, { params })
  return data
}

export async function deleteDocument(kbId: string, docId: string): Promise<void> {
  await axios.delete(`${BASE}/${kbId}/documents/${docId}`)
}

export async function reindexDocument(kbId: string, docId: string): Promise<KnowledgeDocument> {
  const { data } = await axios.post(`${BASE}/${kbId}/documents/${docId}/reindex`)
  return data
}

export async function searchKnowledge(kbId: string, params: {
  query: string; max_results?: number; min_score?: number;
  search_mode?: number; filter?: FilterCondition
}): Promise<{ hits: SearchHit[]; enhanced_query: string }> {
  const { data } = await axios.post(`${BASE}/${kbId}/search`, params)
  return data
}

export async function getKnowledgeBaseStats(kbId: string): Promise<KnowledgeBaseStats> {
  const { data } = await axios.get(`${BASE}/${kbId}/stats`)
  return data
}
```

### 7.4 组件设计

**KnowledgeListPage.vue**

| 区域 | 组件 | 说明 |
|------|------|------|
| 搜索栏 | `QInput` | 按名称/描述搜索 |
| 筛选 | `QSelect` | 按 Agent、状态筛选 |
| 列表 | `QCard` 列表 | 知识库卡片（名称/描述/文档数/状态/绑定Agent） |
| 操作 | `QBtn` | 新建/编辑/删除/搜索测试 |
| 分页 | `QPagination` | 分页 |

**KnowledgeDetailPage.vue**

| 区域 | 组件 | 说明 |
|------|------|------|
| 信息 | `QCard` | 名称/描述/嵌入模型/分块策略/状态 |
| 统计 | `KnowledgeBaseStats` | 文档数/分块数/总大小/错误数 |
| 文档 | `DocumentList` | 文档列表 + 上传 |
| 上传 | `DocumentUploadZone` | 拖拽上传/URL导入/文本导入 |
| 搜索 | `KnowledgeSearchDialog` | 测试检索 |
| 绑定 | `QSelect` | 绑定 Agent |
| 配置 | `QToggle` | 启用 SourceSync / AgenticFilter |

**KnowledgeBaseForm.vue**

| 字段 | 组件 | 说明 |
|------|------|------|
| 名称 | `QInput` | 必填 |
| 描述 | `QInput` type="textarea" | 可选 |
| 嵌入模型 | `QSelect` | Provider + Model 联动选择 |
| 嵌入维度 | `QInput` type="number" | 根据模型自动填充 |
| 分块策略 | `QSelect` | fixed/recursive/markdown/json |
| 分块大小 | `QInput` type="number" | 默认 1024 |
| 分块重叠 | `QInput` type="number" | 默认 128 |
| 向量存储 | `QSelect` | pgvector/inmemory |
| 重排序器 | `QSelect` | topk/cohere/infinity |
| SourceSync | `QToggle` | 启用增量同步 |
| AgenticFilter | `QToggle` | 启用智能过滤 |
| 绑定Agent | `QSelect` | 从 Agent 列表选择 |

**DocumentUploadZone.vue**

| 功能 | 组件 | 说明 |
|------|------|------|
| 拖拽上传 | `QUploader` | 支持 PDF/TXT/MD/DOCX/CSV/JSON/Go/Proto |
| URL导入 | `QInput` + `QBtn` | 输入 URL 自动抓取 |
| 文本导入 | `QInput` type="textarea" + `QBtn` | 直接粘贴文本 |
| 进度 | `QLinearProgress` | 上传 + 处理进度 |
| 元数据 | `QInput` 键值对 | 自定义元数据 |

**DocumentList.vue**

| 列 | 说明 |
|------|------|
| 名称 | 文档名 |
| 类型 | MIME type 图标 |
| 大小 | 格式化文件大小 |
| 分块数 | chunk_count |
| 状态 | `DocumentStatusBadge` |
| 操作 | 删除/重建索引 |

**DocumentStatusBadge.vue**

| 状态 | 颜色 | 图标 |
|------|------|------|
| pending | grey | schedule |
| processing | blue | sync |
| ready | green | check_circle |
| error | red | error |

**KnowledgeSearchDialog.vue**

| 区域 | 组件 | 说明 |
|------|------|------|
| 查询 | `QInput` | 搜索文本 |
| 搜索模式 | `QBtnToggle` | hybrid/vector/keyword/filter |
| 最大结果数 | `QInput` type="number" | 默认 10 |
| 最低分数 | `QSlider` | 0.0 ~ 1.0 |
| 过滤条件 | `FilterConditionBuilder` | 构建过滤条件 |
| 结果列表 | `SearchHitCard` 列表 | 搜索结果卡片 |
| 增强查询 | `QBadge` | 显示增强后的查询文本 |

**FilterConditionBuilder.vue**

| 字段 | 组件 | 说明 |
|------|------|------|
| 字段名 | `QInput` | metadata.field 或 content |
| 操作符 | `QSelect` | eq/ne/gt/gte/lt/lte/in/like/between |
| 值 | `QInput` | 比较值 |
| 逻辑 | `QBtn` | AND/OR 组合条件 |
| 添加 | `QBtn` | 添加子条件 |

**SearchHitCard.vue**

| 区域 | 组件 | 说明 |
|------|------|------|
| 内容 | `QCardSection` | 高亮显示匹配文本 |
| 分数 | `QBadge` | 相似度分数 |
| 来源 | `QChip` | 文档名 + chunk_index |
| 元数据 | `QChip` 列表 | metadata 键值对 |

**KnowledgeBaseStats.vue**

| 指标 | 组件 | 说明 |
|------|------|------|
| 文档数 | `QStat` | document_count |
| 分块数 | `QStat` | total_chunks |
| 总大小 | `QStat` | 格式化 total_size_bytes |
| 错误数 | `QStat` | error_count（红色高亮） |
| 最后索引 | `QStat` | last_indexed_at |

### 7.5 Pinia Store

**stores/knowledgeStore.ts**

```typescript
import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as api from '../api'
import type { KnowledgeBase, KnowledgeDocument, SearchHit, KnowledgeBaseStats } from '../types'

export const useKnowledgeStore = defineStore('knowledge', () => {
  const knowledgeBases = ref<KnowledgeBase[]>([])
  const currentKB = ref<KnowledgeBase | null>(null)
  const documents = ref<KnowledgeDocument[]>([])
  const stats = ref<KnowledgeBaseStats | null>(null)
  const searchResults = ref<SearchHit[]>([])
  const loading = ref(false)
  const totalKBs = ref(0)
  const totalDocs = ref(0)

  async function fetchKnowledgeBases(params?: { keyword?: string; agent_id?: string; page?: number; page_size?: number }) {
    loading.value = true
    try {
      const result = await api.listKnowledgeBases(params || {})
      knowledgeBases.value = result.items
      totalKBs.value = result.total
    } finally {
      loading.value = false
    }
  }

  async function fetchKnowledgeBase(id: string) {
    loading.value = true
    try {
      currentKB.value = await api.getKnowledgeBase(id)
    } finally {
      loading.value = false
    }
  }

  async function createKnowledgeBase(req: Partial<KnowledgeBase>) {
    return await api.createKnowledgeBase(req)
  }

  async function updateKnowledgeBase(id: string, req: Partial<KnowledgeBase>) {
    return await api.updateKnowledgeBase(id, req)
  }

  async function removeKnowledgeBase(id: string) {
    await api.deleteKnowledgeBase(id)
  }

  async function fetchDocuments(kbId: string, params?: { keyword?: string; status?: string; page?: number; page_size?: number }) {
    loading.value = true
    try {
      const result = await api.listDocuments(kbId, params || {})
      documents.value = result.items
      totalDocs.value = result.total
    } finally {
      loading.value = false
    }
  }

  async function uploadDocument(kbId: string, file: File, metadata?: Record<string, string>) {
    return await api.uploadDocument(kbId, file, metadata)
  }

  async function removeDocument(kbId: string, docId: string) {
    await api.deleteDocument(kbId, docId)
  }

  async function reindexDocument(kbId: string, docId: string) {
    return await api.reindexDocument(kbId, docId)
  }

  async function search(kbId: string, params: { query: string; max_results?: number; min_score?: number; search_mode?: number }) {
    loading.value = true
    try {
      const result = await api.searchKnowledge(kbId, params)
      searchResults.value = result.hits
      return result
    } finally {
      loading.value = false
    }
  }

  async function fetchStats(kbId: string) {
    stats.value = await api.getKnowledgeBaseStats(kbId)
  }

  return {
    knowledgeBases, currentKB, documents, stats, searchResults,
    loading, totalKBs, totalDocs,
    fetchKnowledgeBases, fetchKnowledgeBase, createKnowledgeBase,
    updateKnowledgeBase, removeKnowledgeBase,
    fetchDocuments, uploadDocument, removeDocument, reindexDocument,
    search, fetchStats
  }
})
```

### 7.6 路由

**routes.ts**

```typescript
export default [
  { path: '/knowledge', name: 'KnowledgeList', component: () => import('./components/KnowledgeListPage.vue') },
  { path: '/knowledge/:id', name: 'KnowledgeDetail', component: () => import('./components/KnowledgeDetailPage.vue') },
]
```

---

## 八、实现阶段

### Phase 1：基础框架（知识库 CRUD + 文档上传 + pgvector 存储）

1. 创建 Proto 文件并生成代码
2. 创建 Ent Schema（knowledge_base, knowledge_document）
3. 实现 Biz 层（KnowledgeUsecase + Repo 接口）
4. 实现 Data 层（KnowledgeBaseRepo + KnowledgeDocumentRepo + KnowledgeVectorStore）
5. 实现 Service 层
6. Wire 注入
7. 前端知识库列表 + 创建 + 文档上传

### Phase 2：文档处理流水线（分块 + 向量化 + 搜索）

1. 实现 DocumentPipeline（Extractor → Reader → Chunking → Embedding → VectorStore）
2. 集成 trpc-agent-go chunking 包（Fixed/Recursive/Markdown/JSON）
3. 集成 trpc-agent-go embedder 包（OpenAI/Ollama）
4. 实现 KnowledgeBaseFactory
5. 实现搜索功能（Retriever → Reranker）
6. 前端搜索对话框 + 结果展示

### Phase 3：Agent 集成 + 高级功能

1. 实现 KnowledgeToolAdapter（knowledge_search + code_search）
2. 修改 BuildTRPCLLMAgent 注入 Knowledge
3. 实现 AgenticFilter
4. 实现 OCR 识别（Tesseract/Docling）
5. 实现 SourceSync 增量同步
6. 前端 FilterConditionBuilder + AgenticFilter 配置

### Phase 4：超越层（多租户隔离 + 高级 Reranker + 监控）

1. 多租户知识库隔离（SearchFilter 增加 tenant_id）
2. 集成 Cohere/Infinity Reranker
3. 知识库使用统计 + Token 消耗追踪
4. 文档处理进度实时推送（SSE）
5. 前端统计面板 + 进度条

---

## 九、涉及文件

| 文件 | 操作 | 说明 |
|------|------|------|
| `api/knowledge/v1/knowledge.proto` | 新建 | Proto 定义 |
| `internal/data/ent/schema/knowledge_base.go` | 新建 | 知识库 Ent Schema |
| `internal/data/ent/schema/knowledge_document.go` | 新建 | 文档 Ent Schema |
| `internal/data/knowledge_repo.go` | 新建 | 知识库 Repo 实现 |
| `internal/data/knowledge_document_repo.go` | 新建 | 文档 Repo 实现 |
| `internal/data/pgvector/knowledge.go` | 新建 | Knowledge 向量存储 |
| `internal/biz/knowledge.go` | 新建 | Knowledge Usecase + Repo 接口 |
| `internal/service/knowledge.go` | 新建 | Knowledge Service |
| `internal/knowledge/factory.go` | 新建 | KnowledgeBaseFactory |
| `internal/knowledge/pipeline.go` | 新建 | DocumentPipeline |
| `internal/knowledge/tool_adapter.go` | 新建 | Knowledge Search Tool 适配器 |
| `internal/agent/trpc_build.go` | 修改 | 注入 Knowledge |
| `internal/server/register_knowledge.go` | 新建 | Knowledge HTTP 端点注册 |
| `web/src/features/knowledge/` | 新建 | 前端知识库模块 |
