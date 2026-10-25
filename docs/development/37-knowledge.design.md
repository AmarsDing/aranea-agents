# Knowledge 知识库模块 — 实现设计文档

## 2026-10-22：可靠性修复实现设计

- 增加 GetDocument RPC（GET /v1/knowledge/documents/{id}），与正文读取共用行可见性和库授权；用于跨库命中定位。KnowledgeDocument 增加 visibility、owner_user_id、shared_with 字段，分别对应当前文档可见性、创建者和专项 Agent 组织名单。
- UpdateDocumentContentResponse 增加 base_hash，表示本次保存版本；本地返回实际写入字节版本，team 返回所提交正文版本。前端跟踪请求快照与当前输入，禁止无条件写后 GET 采纳他人版本。冲突重载需要显式确认，重载期间的新输入仍保留。
- 原件 MIME 不随抽取改变；历史二进制格式按原始路径扩展拒绝文本写入。
- 本地删除/覆盖通过回收站、数据库事务和文件补偿保持可恢复；本地跨库移动未具备迁移事务前明确拒绝。
- 团队笔记正文采用 PG 行锁下的 CAS，正文变更、旧 chunks 删除与计数调整同事务，状态退回 pending；支持清空正文。索引提交携带正文版本，在删旧插新前复核，旧任务返回 Conflict，不把新稿标成 error。块索引重建读取最新正文并复核版本，失败保留后续修复资格。
- 治理列表按调用方工作区过滤；人工处置要求管理员或系统身份及目标库写授权，提案锁、正文处置、版本记录、旧 chunks 失效和提案终态同事务；派生索引与外部调用提交后重放。当前人工处置仅支持托管库，本地文件库明确拒绝，避免跨介质事务假承诺。
- 检索共用库授权、路径/元数据范围；时态查询的当前关系卡腿在接入时态谓词前关闭。非空 FilterJSON 查询停用缺少原块锚点的卡片腿，原始片段及图邻居片段仍使用范围谓词。reflect 的零命中为 insufficient，未接评估器为 evaluated=false、confidence=0。
- 图谱卡引用为派生证据，记录 evidence_kind、evidence_hash、内容快照与原 viewer 的工作区/用户/组织身份；回采重新检查当前卡片及来源文档 ACL，使用已记录快照，旧式无版本通知不回采。合成 ID 不更新真实 chunk 命中计数。
- 默认建库走现有 team 后端；服务器目录挂载位于高级选项。资源树浏览上下文与活动标签所属库分离，草稿按用户/工作区/文档隔离恢复，恢复前复核读取权限。托管库的目录由笔记路径生成，暂不支持空目录。
- embedding 配置合并优先级为显式环境变量 > 持久化 DB 配置 > 配置文件 > 默认值；未配置项保留未设置语义，避免默认维度覆盖已保存维度。
- 跨模块决策背景、后果与替代方案见 docs/reports/2026-10-21-review-adr-knowledge-reliability.md。

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

### 产品边界（2026-08-22）

Knowledge = 可引用工作区（文档 / Vault 笔记 / 团队词条）。Memory = 运行时状态。写回（G1/G2）是 Memory→Knowledge 的投影，对话默认以 Knowledge 为引用源、Memory 为「关于此人/此任务」的状态。

用户可见名词是「知识库」。`collection_id` 是内部键：入库/检索留空 = 默认库 / 全库联邦路由（US-14）。

### 文档状态机（AS-FSM-01）

`pending → indexing → indexed | error`；`error/indexed → indexing` 可重试；禁止 `pending → indexed`。摄取 / vault 同步 / 重嵌入 / 晋升重放的提交（删旧 chunks + 插入 + indexed + 计数，vault 另含 sync meta）走 `Usecase.CommitIndexedDocument` / `CommitIndexedDocumentMeta` 单事务，indexing→indexed 带 CAS。

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
  // G-12 分数分解：dense_score/sparse_score 回来源于侧的原始分数；
  // rrf_score 为 RRF 融合分（未重排时 score==rrf_score）；
  // rerank_score 仅重排器改写 score 时设置（重排前后序可对比）。
  float  dense_score = 9;
  float  sparse_score = 10;
  float  rrf_score = 11;
  float  rerank_score = 12;
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

// G-19 hit-test：collection_id REQUIRED；hybrid_search: auto|dense|sparse|rrf
// （panel 一次发三策略对比）；repeated 前/重排后序都可由 chunks 分数分解还原。
message DebugKnowledgeSearchRequest {
  string collection_id = 1 [(google.api.field_behavior) = REQUIRED];
  string query = 2 [(google.api.field_behavior) = REQUIRED];
  int32 top_k = 3;
  float min_score = 4;
  string filter_json = 5;
  optional bool use_rerank = 6;
  string hybrid_search = 7;
  string path_prefix = 8;
}

message DebugKnowledgeSearchResponse {
  repeated KnowledgeChunk chunks = 1;
  string mode_used = 2;   // 实际分数形态推断（"rrf"|"dense"|"sparse"|"none"），降级可见
  bool   reranked = 3;    // 重排器改写 score 时为 true（对比 rrf_score 与 score）
}

// G-11：local vault 全量重索引受理（文件系统真相源强制重建派生索引）。
message ReindexVaultRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];  // collection id
}

message ReindexVaultResponse {
  string status = 1;  // "reindexing"（任务受理）
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
  // G-19 hit-test：单库「为什么（没）命中」自助排查。同 Search 检索路径
  // （router/hybrid/rerank/路由规则全保留），差异：collection_id 必填（面板按库）；
  // 不写 access_log（调试流量不得污染底层激活/热度信号）；无 RetrievalEvaluator
  // LLM 环节。逐 chunk 分数经 KnowledgeChunk（G-12）携带，面板可对比重排前后序。
  rpc DebugKnowledgeSearch(DebugKnowledgeSearchRequest) returns (DebugKnowledgeSearchResponse) {
    option (google.api.http) = { post: "/v1/knowledge/search/debug" body: "*" };
  }
  // G-11：local vault 全量重索引（SCAN→Deleted/强制 Created→ApplyEventsForced）；
  // 异步幂等，同库在途 409（job key knowledge-reindex:<id>）；team/无 root_path 拒绝。
  rpc ReindexVault(ReindexVaultRequest) returns (ReindexVaultResponse) {
    option (google.api.http) = { post: "/v1/knowledge/vaults/{id}/reindex" body: "*" };
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

Wave 3（AS-COG-01）：`Usecase` 是 Wire 装配根，字段分簇为 `vaultFields` / `graphFields` / `writeBackFields` / `curateFields`。方法仍挂在 Usecase 上以保持 nil-safe 兼容；新调用方走域门面 `Vault()` / `Retrieve()` / `Graph()` / `WriteBack()` / `Curate()`（`internal/biz/knowledge/domains.go`）。

Wave 4：`GetDocumentByContentHash` 在同一知识库内按正文 hash 去重重复入库；无 LLM 时 `DeriveSummaryCard` 写一行摘要/类型（frontmatter 受管字段，不覆盖用户正文）。用户创建知识库时 embedding/dim 放在「高级选项」，日常界面不强调向量维度。

```go
type Usecase struct {
    vaultFields
    graphFields
    writeBackFields
    curateFields
    lg loggateway.Logger
}

func (uc *Usecase) CreateCollection(ctx context.Context, in Collection) (Collection, error) // embedding_model 可选；空 = 词法库
func (uc *Usecase) GetCollection(ctx context.Context, id string) (Collection, error)
func (uc *Usecase) ListCollections(ctx context.Context, workspace string, limit, offset int) ([]Collection, int, error)
func (uc *Usecase) DeleteCollection(ctx context.Context, id string) error
func (uc *Usecase) UpdateCollectionCounts(ctx context.Context, id string, docDelta, chunkDelta int) error
// EnsureDefaultCollection — US-14：返回「默认知识库」，不存在则懒创建（name=默认知识库，
// team Vault；embedding_model/dim 可空 = 词法收件箱）。上传免预选的兜底出口。
func (uc *Usecase) EnsureDefaultCollection(ctx context.Context, embeddingModel string, dim int, ws string) (Collection, error)
func (repo CollectionRepo) GetCollectionByName(ctx context.Context, workspace, name string) (Collection, error)

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

CREATE INDEX IF NOT EXISTS knowledge_chunks_embedding_hnsw
    ON knowledge_chunks USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);
CREATE INDEX IF NOT EXISTS knowledge_chunks_collection_idx
    ON knowledge_chunks(collection_id);

-- 20261283 父子分块（G-06，EnsureKnowledgeSchema 幂等 ALTER 同步）：
ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS parent_chunk_id TEXT NOT NULL DEFAULT '';   -- 子块→父块行 ID（<docID>-p-<pidx>）；'' = 独立块/父块自身；无自引用 FK（应用层同一事务维护）
ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS chunk_role      TEXT NOT NULL DEFAULT 'child'; -- child|parent；父块仅语境回填，不入任何召回通路
CREATE INDEX IF NOT EXISTS knowledge_chunks_parent_idx ON knowledge_chunks (parent_chunk_id) WHERE parent_chunk_id <> '';
ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS chunk_strategy TEXT NOT NULL DEFAULT '';    -- 摄取时生效策略（reembed 保形用）
ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS chunk_size     INT  NOT NULL DEFAULT 0;
ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS chunk_overlap  INT  NOT NULL DEFAULT 0;
```

#### 维度对账（2026-08-10 事故根修）

`knowledge_chunks.embedding vector(N)` 的 N 在**首次建表**时固定为当时的 `vector_dim` 配置；embedder 换模型（维度变化）后 `CREATE TABLE IF NOT EXISTS` 不会修正存量列，新维度向量插入全部被 PG 拒绝（`expected N dimensions`），而应用层按 `knowledge_collections.dim` 的校验反而通过，故障极难定位（2026-08-08 配置切 bge-m3/1024 后语义检索全灭）。

`EnsureKnowledgeSchema` 尾部执行 `reconcileEmbeddingDim`（幂等；列 typmod == 配置维度时零动作）：

1. 存量向量全部置 NULL 作废（旧维度向量对新 embedder 无检索意义）；
2. 受影响文档 `content_hash='' + status='pending'`——vault 文档下轮 sync 自动重嵌入自愈；UI 上传文档（rel_path 空）无 sync 循环，需人工重传；
3. 有语义层集合（`embedding_model <> ''`）的 `dim` 快照同步为新维度——单全局 embedder 架构下不同步 = 应用层校验永远拒绝新插入（死库）；`embedding_model` 名不动（Ensure 拿不到模型名，仅展示用）；
4. `ALTER COLUMN embedding TYPE vector(N)` + 重建 HNSW 索引（先 DROP 旧 ivfflat/`knowledge_chunks_embedding_hnsw` 再 ALTER，避免 PG 级联重建沿用旧维度）。查询侧 `SearchChunks` 在事务内 `SET LOCAL hnsw.ef_search`（默认 40，可用 `KRATOS_KNOWLEDGE_HNSW_EF_SEARCH` 覆盖；禁止无 LOCAL 的 SET，以免污染连接池）。

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
    ChunkByChar        ChunkStrategy = "char"
    ChunkByToken       ChunkStrategy = "token"
    ChunkByMarkdown    ChunkStrategy = "markdown"    // trpc MarkdownChunking
    ChunkByJSON        ChunkStrategy = "json"        // trpc JSONChunking
    ChunkByRecursive   ChunkStrategy = "recursive"   // trpc RecursiveChunking
    ChunkByParentChild ChunkStrategy = "parent_child" // G-06 小块召回 + 父块语境回填
)

func ParseChunkStrategy(raw string) ChunkStrategy
func SplitWithStrategy(strategy ChunkStrategy, text string, size, overlap int) ([]Chunk, error)
func SplitParentChild(text string, size, overlap, parentSize int) ([]Chunk, error) // 父块默认 4×size

type Chunk struct {
    Content     string
    ChunkIndex  int
    IsParent    bool // 父块（语境行）：不入 embed/召回，ID <docID>-p-<pidx>
    ParentIndex int  // 子块所属父块序号；-1 = 无父块（SplitWithStrategy 非父子策略统一归一 -1）
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
- `token` 策略（G-06 修正）：cl100k_base BPE 真 token 计数（tokenizer.go 包级单例），窗口按 token 前缀和换算原文边界切片；Encode 失败降级 rune 窗口。原空格分词实现已废弃（CJK 长无空格段塌缩成单块的存量 bug）。
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
    Strategy        FederationStrategy
    RouteTopN       int
    RouteMinScore   float32
    EnumerateLimit  int // SearchAll 枚举上限，默认 64
    BroadcastCap    int // 广播/降级扇出上限，默认 8
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
- Wave 2 预算：`SearchAll` 枚举最多 64 库；无 Route 命中时 Broadcast 最多 8 库（按 List 顺序，通常为最近创建）。扇出前做一次查询重写（API `rewrite_strategy` 或复杂查询自动 MultiQuery），避免每库各打一次 LLM。
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
| `evaluatorKey{}` | 默认不注入；Search `use_eval=true` 时走 Service 层评估 | RetrievalEvaluator |
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
| `Search` | 查询重写 → AdaptiveRouter/Retriever 检索 → **仅 `use_eval=true` 时** RetrievalEvaluator 评估 → Prometheus 计时 |
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
  └── use_eval == true ?
      └── SearchWithEvaluation(retriever, evaluator, query, q, chunks)
          ├── evaluator.Evaluate → RetrievalAssessment
          ├── !sufficient && supplementQuery != "" → retriever.Search(supplementQ)
          └── MergeSearchResults(chunks, supplementChunks, topK)
```

> **2026-08-21**：Search API 与对话路径默认关闭 RetrievalEvaluator（评估 LLM ~1.4s）。管理端显式 `use_eval=true` 才补充检索。GraphExpander 已由 AdaptiveRouter.Search 接线，一跳扩展在混合检索之后。

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

> **2026-10-19 复核校准（F5/F6 竞态闭环）**：①内容哈希去重命中 `error`（或 stale>15min 的 `indexing/pending` 尸体）时，经可选能力接口 `DocumentRetryClaimer.ClaimDocumentRetry` 守卫式原子 UPDATE 认领后重跑管线——check-then-spawn 双管线竞态收口（fake fallback 退化为 error-only 读判定）；②摄取 goroutine panic 一律捕获并置 `status=error`（15min 尸体的制造源封口）；③`CommitIndexedDocument` 失败时重读文档终态：已被对端管线抢为 `indexed` 则静默收敛（胜者终态即两上传共用结果），不再写回错误。

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
        ├── Route 无匹配（全部 < 阈值）→ 降级 Broadcast，最多 8 库
        └── 部分库失败 → FlowLog 警告，返回成功库结果（§5.10 现状）
```

**关键设计决策**：

- **默认知识库懒创建**：首个免选上传时创建（`name="默认知识库"`，`vault_backend=team`，PG 即真相源）。`embedding_model`/`dim` 取当前 Embedder 配置，**未配置则建词法收件箱**；`GetCollectionByName(workspace, name)` 精确复用，`(workspace, name)` 唯一索引防并发双建；不引入 is_default 标记列。语义层是后续 `EnableCollectionSemantic` 升级，不是创建门槛。
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
3. **embedding 可选化（R-4，2026-08-22 已落地）**：`CreateCollection` / `CreateVault` / `EnsureDefaultCollection` 均允许空 `embedding_model`（词法库）。`ErrEmbeddingModelRequired` 仅保留兼容别名，创建路径不再返回。语义层通过 `EnableCollectionSemantic` 升级。

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

**部署约束（N-1 / [ADR-KN-LINKINDEX](../reports/2026-08-22-review-adr-knowledge-linkindex-replica.md)）**：`LinkIndex` 是本进程可选缓存；跨进程反链/dangling 以 `knowledge_block_refs` 为准（SP1-E 未加载即落库）。禁止副本间广播内存图；`knowledge.graph.delta` 保持 Informational，只服务本进程 WS 客户端。

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
  → maybeWriteBack（失败 → knowledge_writeback_outbox 持久化重试队列，G-10）
       FilterWriteBackFacts（kind 白名单 ∩ confidence≥0.85 ∩ ≥8 rune）
       → 团队库当日日记追加 provenance 块
       → Autolink + RebuildBlockIndex
       → KnowledgeService 重放 chunk/FTS
```

- 端口：`knowledge.SessionWriteBack`（biz 别名 `KnowledgeWriteBack`），生产实现 `KnowledgeService`。
- 日记路径：`inbox/writeback-YYYY-MM-DD.md`。集合：同名「团队知识收件箱」→ 否则工作区第一个 team 库 → 否则 `CreateVault` 懒创建（词法、无 embedding）。
- **不**在 watcher/外部编辑路径写回；**不**把 L2 episode 原文整段入库。

### G-10 写回 outbox 持久化（2026-10-19）

> AS-EVT-01 对齐：写回失败属 Important 事件（应异步持久化），修复前仅存一条
> Warn（重启/日志轮转即永久丢失）。模式复用 `decision_record_outbox`（20261251）。

- **表** `knowledge_writeback_outbox`（DDL migration `20261280`，Ent schema 契约文档 + raw-SQL 仓储）：`batch_key`(UNIQUE 幂等键) / `workspace_id` / `payload`(codec-stable JSON) / `status`(pending|published|dead) / `attempts` / `last_error` / `created_at` / `published_at`。
- **幂等键** `WriteBackBatchKey` = `wb:{sessionID}:{sha256(规范化 facts)[:16]}`——同会话同批事实反复失败归并一行（BuildInsertOrIgnore），不堆行。
- **端口** `knowledge.WriteBackOutboxRepo`（biz 别名 `KnowledgeWriteBackOutboxRepo`）：`EnqueueWriteBackOutbox` / `ListPendingWriteBackOutbox`(attempts<10) / `MarkWriteBackOutboxAttempt`（触 10 同语句翻 dead）/ `MarkWriteBackOutboxPublished` / `MarkWriteBackOutboxDead`（poison 专用——不可解码行记 published 是审计造假）。
- **payload 编解码** `EncodeWriteBackOutboxPayload` / `DecodeWriteBackOutboxPayload`：codec-stable JSON；`EntryTitle`（执行期回填字段）不入队，重放由写回路径重新计算。
- **生产侧** `AutoMemoryWorker.maybeWriteBack`：`WriteBackSessionFacts` 失败 → Warn + 入队；入队自身再败才 dead-letter Warn（与 decision replay 同语义）。
- **消费侧** `KnowledgeWriteBackSweeperWorker`（`internal/cronrunner/jobs/knowledge_writeback_sweeper.go`，默认 2min，`cmd/admin/workers.go` 注册 `knowledge_writeback_sweeper`）：扫 pending → 解码（失败→dead）→ 重投 `WriteBackSessionFacts`（成功→published；失败→attempt+1）；每轮 Info 汇总 scanned/published/dead（积压观测口径）。

### 不做什么

- 不做 LLM 二次抽取 / 三元组表
- 自动路径仍无弹窗（≥0.85 直接日记）；0.60–0.84 进 pending（V12.13 US-44）
- G1 投影见 V12.13（覆盖写，不是往 Usecase 加字段）

### G-15 access_log 审计足迹扩列（2026-10-19）

> 调研报告 G-15：access_log 之前仅 collection_id/doc_id/query_hash 三列支撑
> base-level 激活分与 Hebbian 共激活；session_id 列在 20261220 建表时已存在但
> 从不被写。扩列 + 检索命中路径透传请求上下文，为审计查询、「我的足迹」视图
> 与个性化排序铺数据基础（维度即契约，先落库再谈消费）。

- **扩列**（DDL migration `20261282`；`EnsureKnowledgeSchema` 同型 + Ent schema 契约文档）：`user_id` / `agent_id` / `session_id`（接通既有死列）/ `rank`(INT，文档首个 chunk 在最终排序中的 1-based 位置) / `score`(FLOAT8，该 chunk 最终排序分——G-12 已统一 `Score==最终排序键`，无需分解链) / `latency_ms`(INT，检索总时延)；审计索引 `knowledge_access_log_user_idx(user_id, accessed_at DESC)`。幂等：duplicate-column 按 DB-N6 视作成功；`"rank"` 为窗口函数关键字，双方言统一加引号。
- **端口** `bizknowledge.AccessLogEntry` 增 `SessionID/UserID/AgentID/Rank/Score/LatencyMs` 六字段（语义注释在结构体上）；data 层 `LogAccess` UNNEST 扩为 9 列（PG-only 路径不变）。
- **透传** `bizknowledge.SearchQuery` 增 `SessionID/AgentID/UserID`（仅记账用途，不影响检索语义）：Agent 工具回调路径由 `working_memory` ctx 注入 session/agent（tool.go）；cue 预检索由 `trpcagent.Invocation` 透传（knowledge_inject.go）；`UserID` 缺省时由检索层从请求 ctx 认证身份回填（`ctxuser.FromContext`，HTTP 服务调用自动覆盖）。
- **计算** `accessEntriesFromChunks`（retriever.go，Retriever/Router 共用）：同 doc 多 chunk 记首开位置与最终分；latency 由 `Retriever.Search`/`AdaptiveRouter.Search` 入口计时随条目落库。G-19 `SkipAccessLog` 旁路语义不变（两路径互斥无双记）。
- 基建幂等注记：20261281 为 twinmonitor 种子重编号（原 20261276 与 DDL `steps_v2_updated_at` 撞号，fresh 库曾按「已应用」跳过种子——`seed_versions.go` 注释立案）。

### G-30 治理中心仪表盘（2026-10-20）

> 调研报告 G-30：治理提案之前只能等 dream 周期 cron 或 memory_butler 工具间接触达；
> 提案列表 cap=200 无翻页（同库 >200 条 pending 时审不到尾部）。落地「手动触发 +
> offset 分页」治理仪表盘：复核面板的提案队列可翻页扫尾，预演/实跑双入口当面闭环。

- **API**：`ListGovernanceProposalsRequest` 增 `offset=4`；`ListGovernanceProposalsResponse` 增 `total=2`（同 WHERE 口径 COUNT，独立于 LIMIT/OFFSET）——前端据此渲染翻页而不再依赖「items 长度 == 总数」。新增 RPC `CurateKnowledgeNow{ collection_id REQUIRED, dry_run } → { status }`，HTTP `POST /v1/knowledge/vaults/{collection_id}/curate`。
- **分层**：biz `KnowledgeCurateRepo.ListGovernanceProposals` 签名增 `offset` 与 `total` 返回值（usecase 钳 limit≤0→50 / >200→200、offset<0→0）；data 层 items（`ORDER BY id DESC LIMIT … OFFSET …`）与 total（同 WHERE COUNT）两查询。`memory_butler` 治理工具 input 增可选 `offset`，输出透传 `total`。
- **手动触发**（`internal/service/knowledge_curate.go`）：镜像 G-11 ReindexVault 模式——GetCollection → `assertCollectionMutateAccess` → `acquireJob`（job key `knowledge-curate:<id>`，PG advisory lock + 本进程 `curateRuns` sync.Map 双层门，与 cron 同键互斥——同库在途返回 409，防止重入双倍提案）→ safego 后台跑 `uc.CurateKnowledge({CollectionID, Workspace, DryRun})` → WS 广播 `knowledge_curate`（curating/done/error，EP-KN-02 SystemNoticeEvent）。`dry_run=true` 仅探测不应用（decay/置位/提案均不落库）。语义刻意与 cron `CurateAllTeamKnowledge` 单库路径一致：显式 CollectionID 跳过 distill（homeCol 为空 → VaultBackend≠team 短路）。
- **前端**：`listGovernanceProposals(collectionId, status, {limit, offset})` 返回 `{items, total}`；新增 `curateKnowledge(collectionId, dryRun)`。`GovernanceReviewDialog` 升级治理仪表盘：分页控件（`total/page/pageSize` props + `page` emit）+ 「预演治理 / 立即治理」按钮（`curate` emit，受理期 `curating` 防重入）；`useWorkbenchCommands` 持有 `govTotal/govPage/govCurating` 状态，`onGovPageChange` 换页、`onCurateGovernance` 触发；`useReviewBadge` 治理角标改用 `total` 口径（不再被页大小截断）。提案全部批完当前页自动回退上一页继续二审。
- **未做的**（保持范围）：distill 仍只对团队库 home 库蒸馏（显式 CollectionID 语义不变）；提案队列为分页列表而非 DOM 虚拟滚动（单页 20 条五页内翻页 < 虚拟列表阈值）；健康卡为既有 `getCollectionHealth` 复用，无新后端。

### G-13 金标快照 + CI 门禁（2026-10-20）

> 调研报告 G-13：`EvaluateRetrievalMetrics` 只在包内单测被调用；既有金标断言
> （`gold_recall_test.go`）只校验「expander 全中」的绝对阈值，无历史快照比对——
> 检索层排序/扩展/成链的渐进式劣化无门禁可拦。落地「驱动命令 + 快照 JSON 入仓 +
> CI before/after 跌幅报警」。

- **评测基建**：`internal/knowledge/eval` 既有纯库（`Runner`/`DiffReports`/`RegressionThreshold`=0.02 chunk recall 跌幅）补齐持久化与出口——`store.go` 增 `LoadRunReport`/`SaveRunReport`；`report.go` 增 `RenderDiffMarkdown`（CI job summary 用的 before/after 报表）。
- **确定性检索器**（`eval/fixture_search.go`）：CI 无 PG/pgvector/embedder，真实 BM25/向量召回在 data 层不可复现，故 fixture 复刻 `gold_recall_test.go` 的最坏假设——每查询锚点种子当混合命中，其余走**全生产组件**：`AutolinkWikiMentions` 成链 → `Chunker` 切分 → `ClassifySearchIntent` 分流（Instant 跳过扩展）→ `GraphExpander` 一跳 → `MergeSearchResults` 合并截断。语料（6 文档中英混排）与 12 条查询（zh/en/混合）入仓 `testdata/gold-corpus.json` + `gold-set.json`；快照 `gold-snapshot.json`（topK=8 与 gold 单测同口径）同 PR 入仓。
- **驱动命令**：`internal/knowledge/eval/cmd/goldeval`（`-corpus/-set/-baseline/-topk/-update/-out/-markdown`）——退出码 0 无回归 / 1 运行错误 / 2 门禁违规（recall/MRR 回归、检索失败、集漂移、快照过期、基线降级）；`-update` 重写基线前强制完美运行校验（等价本地入口 `go test ./internal/knowledge/eval/ -run TestGoldSnapshot -gold.update`，测试二进制免 auth env）。注意二进制经 `internal/knowledge` 传递依赖 `pkg/auth`，非 dev/CI 环境需 `DEPLOY_ENV=dev`。
- **门禁判定单一事实源**（`eval/gate.go`，测试与 goldeval 共用，禁手抄分支）：`CheckGoldGate` 枚举违规——快照过期（set name/topK 不一致）、入库基线降级（任何用例带 Err 或 chunk/doc recall/MRR 非满分，防垃圾快照囤库后假绿）、用例检索失败、集漂移、chunk recall 跌幅 > `RegressionThreshold`(0.02)、chunk MRR 跌幅 > `MRRRegressionThreshold`(0.02)（纯排序/打分回归，recall 不变也能红——正是 GraphExpander 邻居打分与 MergeSearchResults 排序的守护对象；`DiffReports.Regression` 保持 recall-only 库契约不动）；`ValidateGoldBaseline` 为 `-update`/`-gold.update` 写盘守门，基线必须是完美运行；`ValidateGoldExpectations` 拒绝空期望用例（vacuous-success 恒满分 = 免疫一切回归，弱化期望还会把指标「改好」）。评测集严格解析（`LoadEvalSet` 未知字段报错，字段拼写错误不再静默吞期望）。金标按构造满血：期望 = 种子∪一跳全集、topK=8 容纳得下，正确实现必然全满分——有意变更构造时须显式改守门并随 PR 说明。
- **CI 门禁**：`ci.yml` test-go 增「Knowledge gold recall gate」步——`go build -o bin/goldeval` 后执行（保留语义化退出码），markdown diff 写 `$GITHUB_STEP_SUMMARY`；同逻辑亦含于 `go test ./...`（`TestGoldSnapshotRegression`）双保险。
- **未做的**（保持范围）：不引 PG 金标（需 pgvector 服务，CI 代价不成比例）；不覆盖 LLM rerank/向量质量（确定性替身无法代表，留待 G-06/G-38 线上评测层）；doc-basis 指标只报不卡（该金标探针灵敏度口径在 chunk 粒度）。

### G-06 父子分块 + 真 Tokenizer（2026-10-20）

> 调研报告 G-06：① `token` 策略用空格分词模拟 token 计数——中文整段无空格时
> 「512 词窗口」恒为全文一块，token 策略对 CJK 实质失效；② 召回粒度与语境宽度
> 不可兼得：小块命中准但语境残缺（相邻段落须分开召回），大块语境全但命中率低、
> 噪声入窗。落地「真 cl100k tokenizer + 父子分块（小块召回、父块回填语境）」。

**ADR-KN-20261020：数据驱动回填，不设馆藏级 context_mode 开关**
勘察曾建议仿 vault_backend 加 `knowledge_collections.context_mode` 列 + proto 16 号位。
放弃理由：回填需求完全由行数据决定（子块带 parent_chunk_id 才需要回填），馆藏开关
会引入「模式与数据不一致」的第二真相源（切模式未重灌、混策略同库等歧义态），且
proto/前端馆藏面正被 peer 会话在途占用。维持「父块存在即回填」数据驱动语义——零配置、
恒一致；代价是每次检索引一次条件批量查询（有父块才发，无父块零成本）。

- **真 Tokenizer**（`internal/knowledge/tokenizer.go`）：`splitByToken` 弃用空格分词，
  改 cl100k_base BPE——包级单例 `sync.OnceValue(tokenizer.Get(Cl100kBase))`（BPE 加载
  昂贵，禁每请求 ForModel；直接引用 `github.com/tiktoken-go/tokenizer`（原经 trpc wrapper
  间接引入，go.mod 转直接依赖））；一次 Encode 后用 token 串前缀和换算原文切片边界
  （不重 join、保留原始空白/CJK）。codec 初始化/编码失败降级为 rune 窗口（Warn 一次，
  与 trpc_build.go 既有降级模式同型）。窗口语义从「词数」修正为「BPE token 数」——
  中文长无空格段不再塌缩为一块；默认 512/64 数值不变（单位即日起为真实 token）。
- **父子分块策略** `parent_child`（chunker.go / chunk_strategy.go）：父块 = 固定 token
  窗口（`parent_chunk_size`，0=默认 4×chunk_size，规范化下限 2×chunk_size；父块间不重叠），
  子块 = 父块内 token 窗口（chunk_size/chunk_overlap 原语义，最后子块可越父块尾直到
  父块末——按父块边界钳制）。`Chunk` 增 `IsParent`（父块标记，仅摄取写路径用）与
  `ParentIndex`（-1=无父块；`SplitWithStrategy` 对非父子策略统一归一 -1——单一归一点，
  既有构造零改动）。父块行 ID `<docID>-p-<pidx>`、子块行保持 `<docID>-ch-<cidx>`
  （cidx 全文连续递增）——retryDocEmbeddings 精确 ID 寻址与 citation 锚点命名空间
  零碰撞。markdown 标题结构感知的父块切分留 G-07（blockparse BlockRow.HeadingPath；
  trpc MarkdownChunking 的 merge/overlap 会破坏节原子性，不可用）。
- **schema**（DDL migration `20261283` + EnsureKnowledgeSchema 同型幂等 ALTER + ent
  schema 契约同步）：`knowledge_chunks` 增 `parent_chunk_id TEXT NOT NULL DEFAULT ''`
  （子块→父块行 ID；'' = 独立块/父块自身；**不加自引用 FK**——应用层维护，同一
  DELETE+INSERT 事务内父子一致）+ `chunk_role TEXT NOT NULL DEFAULT 'child'`
  （'child'|'parent'；排除谓词用 `chunk_role='child'` 而非 ID 前缀匹配，既有全量行
  天然落在默认侧）+ 部分索引 `knowledge_chunks_parent_idx(parent_chunk_id)
  WHERE parent_chunk_id <> ''`；`knowledge_documents` 增
  `chunk_strategy/chunk_size/chunk_overlap`（摄取时落库生效值——顺带修复 reembed
  恒 char+512/64 丢策略的存量 bug，重嵌保形）。
- **摄取**：`BuildIndexedChunks` 父子分支——父块行不进 embed 批（Embedding=NULL，
  assertChunkDimensions 跳过父块行；误带向量也无害：dense 恒有 role 谓词挡）、
  子块照常；`IngestParams.ParentChunkSize`（proto `IngestDocumentRequest.parent_chunk_size`=11）；
  文档行落规范化后的 strategy/size/overlap（service 在 CreateDocument 时写入请求归一值）。
  `CommitIndexedDocument` 计数口径 = 子块：documents.chunk_count 与 collections
  chunk_count delta 均只计 `chunk_role='child'` 行。
- **召回排除**：父块行绝不进入任何召回通路——dense 天然免疫（`WHERE embedding IS NOT NULL`
  已挡），BM25 四支（tsvector/trigram/substring/bigram）与 GraphExpander 邻居通道
  `ListChunksByDocuments` 统一增 `AND chunk_role='child'`；所有 chunk SELECT 增扫
  `parent_chunk_id`（回填旁路输入）。
- **语境回填**（AdaptiveRouter.Search 尾段，排序完全落定之后、返回之前）：收集返回集
  非空 ParentChunkID 去重 → 一次 `ListChunksByIDs`（data 端口新方法；复用
  docChunkVisibilityClause 走同一可见性 ACL）→ 仅置 `Chunk.ParentContent`（不改序、
  不参分）。失败降级 Warn 跳过（与图扩展同语义）。端口为 knowledge 包局部能力接口
  `ParentContextResolver`（biz 契约面零变更，同 NeighborChunkLister 先例），service 装配
  与 AccessLogRepo/CoActivationRepo 同款 capability-assert 接线；未接线的 retriever 直通
  通路无回填、行为如旧。
- **锚定不变**：`[n]` 引用、citation ledger、k-gram 事实匹配、cited_count 全部仍以
  子块 id + 子块 Content 为契约（`loadChunkContents` 从 DB 取子块原文，零改动）；
  `ParentContent` 只影响「LLM 看到的文本」。展示规则单点 `Chunk.DisplayContent()`
  （biz/knowledge）：ParentContent 非空取之、否则 Content——inject cue 与
  knowledge_search 工具摘要统一经此；cue 单条截断预算父块 768 rune（子块沿用现值），
  总预算与块/条目边界丢弃语义不变。因子块⊂父块，k-gram 命中只偏保守不偏假。
- **reembed 修复**（knowledge_reembed.go）：策略 = 请求 `chunk_strategy`（proto 增
  `ReembedDocumentsRequest.chunk_strategy`=5，空=用文档落库值）> 文档 chunk_strategy >
  char；size/overlap 同序取 请求 > 文档 > 默认。父子文档重嵌后形状守恒。
- **proto**：`KnowledgeChunk.parent_content`=13（命中调试/追溯面板可直接消费）；
  增量编译续编（knowledge.proto 在途未提交改动均为本域历轮落地）。
- **未做的**（保持范围）：不设 collection `context_mode` 列与 proto 16（见上 ADR）；不做
  vault 同步链策略切换（vault 恒 markdown，ID 命名空间隔离）；不变 GraphExpander 一跳
  邻居取子块口径；不做父块嵌入/父块参与 rerank（回填在打分落定后，纯展示）；前端策略
  下拉与 parent_content 面板接入留前端会话（proto 字段已备）；agent 侧 TokenCounter 与
  本计数标准的收敛留独立 ADR；内容哈希命中重试路径不刷新文档落库策略（reembed 可用
  请求覆盖）；`parent_chunk_size` 不落 documents 列——摄入一次性生效，reembed 恒归一为
  4×chunk_size（proto 11 注释同步注明）。
- **对抗审查修正**（2026-10-20，0 refuted）：回收面字段保全——`rrfMerge` chunkMap 字面量与
  `mergeScoreBreakdown` 重排并回均须带 `ParentChunkID`（回填触发键，字面量重建易漏已钉注释）；
  `use_eval` 补充检索走裸 Retriever 不经 Router，补充块经 `BackfillParentContext` 在合并终态
  统一补一次父块语境（幂等）；裸 `Chunker` 下 `parent_child` 显式退化 token 窗（入 `ChunkByToken`
  case），真父子双层仅 `SplitParentChild` 入口。

### G-07 原件留存 + 页码/标题锚定（2026-10-20）

> 调研报告 G-07：答案引用无法锚定到原文页/段——① 非图片原件不落 AssetStore（仅图片
> 分支写 asset_uri），引用页码无原件可回跳；② chunk 无 page/heading 元数据，对标
> NotebookLM 句级引用点击跳原文高亮。落地「AssetStore 推广到全部可提取源 + chunk 落
> page_no/heading_path」。

**ADR-KN-20261020-B：锚点走 metadata_json，不加列不加 proto**
背景：页号/标题路径是「有锚文档才有」的稀疏元数据（PDF 提取产 `<!-- page:N -->`，
markdown 有标题栈；txt/代码/Office 多数无页概念）。**决策**：锚并入 chunk
`metadata_json`（键 `page_no` int>0 / `heading_path` []string；空锚不落键、无锚文档
字节不动）——零 DDL、零 proto 变更（`KnowledgeChunk.metadata_json`=6 已贯通 Search 与
debug search 透传面）。**后果**：检索/调试与命中追溯面板立即可消费；无锚存量行为不变；
代价是锚不可索引/谓词过滤（现无此需求——页码过滤属 PageIndex/时态域）。**替代方案
（弃）**：`knowledge_chunks` 增 `page_no INT` + `heading_path TEXT[]` 列——为稀疏字段
动全量 DDL + proto + 前端馆藏面（peer 会话在途占用）不成比例；若日后需谓词查询再随
PageIndex（G-38 区）迁列。

**ADR-KN-20261020-C：起点锚（start-anchor）语义**
**决策**：块锚 = 块**首字节**所在页/标题——跨页窗起点落前页即归前页（引用起点即指向
起点，NotebookLM 回跳口径）；`ByteStart` 与分块同基准（所有分块入口先 TrimSpace，
锚索引同源）。**替代（弃）**：多数决或全覆盖区间锚（块跨页 N-M）——元数据膨胀且引用
端无法单一回跳，宁缺不假。

- **坐标基**（chunker.go / parent_child.go / chunk_strategy.go）：`Chunk` 增 `ByteStart`
  （源文本 TrimSpace 基准的起始字节偏移；定位失败 -1 = 锚定跳过）。splitByChar 单遍
  rune→byte 表；token/父子窗沿 trim-shift 公式（`offs[i]+len(raw)-len(TrimLeft)`）；
  trpc 策略产物按内容顺序回定位（cursor+Index，失序回退全文首个命中——宁近似不丢；
  拼接式产出 -1）。
- **锚索引**（`internal/knowledge/anchors.go`）：`newDocAnchors(text)`——页锚经
  `parser.PageAnchorSpans`（复用 PDFToMarkdown `<!-- page:N -->` 约定，无需重解析）；
  标题栈经 blockparse.Parse 的 `KindHeading`+`HeadingPath`（ATX/setext、code fence 免疫、
  frontmatter 剥离）；`anchorAt` 取起点不大于块偏移的最后一锚（锚前内容 PageNo=0
  不落键）；`mergeChunkAnchorMetadata` 无锚字节级不动、有锚保留文档级键、块锚覆盖同名键。
- **对抗审查修正**（2026-10-20，11 findings / 7 confirmed / 4 refuted，去重 5 项全修）：
  ①F1/F4/F8（major，同根因）标题锚子串重定位在「代码 fence/行内 prose 含同名标题行」时
  抢占锚位——改取真源偏移：blockparse `BlockRow` 导出 `ByteStart`（body 坐标系，
  `blockLineStart` 与 rawTextOf 同口径行首回溯）+ `BodyOf` 导出 frontmatter 剥离正文
  （skip = len(src)-len(body) 补偿），anchors.go 删除全部 strings.Index 重定位与单调
  守卫；②F2（minor）CRLF 多行 setext 标题（归一后 b.Text 含 \n）失信——锚索引整体改
  norm 坐标系（ReplaceAll \r\n→\n），docAnchors 记 \r 偏移表，anchorAt original→norm
  换算（span 起点恒为行首字节，起点落 \r 按「起点即归属」归前区间）；③F9（minor）
  asset 落盘后 CreateDocument 失败留孤儿文件——CreateDocument 失败分支补偿
  `os.Remove(assets.Resolve(uri))`（与 vault 补偿同型）；④F10（major）asset 测试未钉
  doc.ID 派生——两连传共存 + `uri == docID+ext` 断言（缺 doc.ID 提前生成时相互覆盖
  必红）；⑤F11（minor）anchorAt 等号边界零覆盖——`TestDocAnchors_EqualityEdge` 直测
  「起点恰落锚行首字节归该锚、前一字节归前区间」。回归测试：
  `TestDocAnchors_FencedDuplicateHeading`（fence 伪锚+行内提及不抢锚）、
  `TestDocAnchors_CRLFAnchors`（多行 setext+页锚 CRLF 全锚）、EqualityEdge、
  `TestIngestDocument_AssetCompensatedOnCreateFailure`（failingCreateRepo 注入失败）。
  refuted：非对象 metadata_json 不可达（mergeIngestMetadata 早在 API 边界拒非对象）、
  invalid UTF-8 不可达（PG 写入先行拒收）、setext 口径被夸大（单行 setext CRLF 无恙，
  残余多行情形已并入 F2 修复）。
- **原件留存推广**（service/knowledge.go）：`assetEligible = isImage ||
  extractors.Supports(source, mime)`，vault 上传（rel_path 已持原件镜像）跳过不双份；
  doc.ID 提前生成（asset 文件名依赖 docID）；留存失败 Warn 降级不阻断入库（与原图片
  分支同语义）。存量无 asset 文档不回填（重传即得）。
- **未做的**（保持范围）：引用点击跳原文高亮 UI 留前端会话（锚与 asset_uri 经
  metadata_json/DTO 已备）；PDF bbox 级锚（RAGFlow 式，需解析器坐标输出，属 G-38
  连接器/PDF 解析升级域）；Office 页/片锚（docx/xlsx 无页概念，pptx 按 slide 留后续）；
  vault 同步链 chunk 不另锚（rel_path + block 锚已在 promoteDocBlocks）；粘贴文本 raw
  不落 asset（与上传字节不可区分，接受）；markdown 组织器丢 `<!-- page:N -->` 注释时
  页锚自然退化为无（标题锚仍可得）。

### G-08 部门级可见性（shared_with，2026-10-20）

> 调研报告 G-08：可见性仅 collection/private 两档 + workspace 整库物理隔离，无共享名单/
> 部门/角色粒度。对标 Azure AI Search security filter / Dify 元数据过滤（部门级隔离为最小
> 经济方案，优于拆库）。落地「documents 增 `shared_with`（部门/岗位数组）+ 谓词扩三档 +
> 检索各读路径同步过滤」，与 M78 部门边界天然对应。设计侦察：workflow wf_f2a3dbdd-c44
> （4 路 reader / 425k tokens——读路径 19 项清单、Agent→org 解析链、API 面、DDL/测试惯例）。

**ADR-KN-20261020-D：三档谓词与匹配主体——`shared` 面向专项 Agent，人类用户仅 owner 可见**
背景：org 树（organizations：company→department→position）上挂的是 **Agent**（agent.position_id），
人类用户 schema 无任何 org 绑定（auth.Auth 仅 UserID/Access/WorkspaceID；admin 表无 dept 列——
侦察逐项证伪）。**决策**：`knowledge_documents.shared_with TEXT[]` 存 org 节点 ID（department 或
position 节点）；第三档 `visibility='shared'` 谓词 = `owner 命中 OR (viewer 的 org scope ∩
shared_with ≠ ∅)`。org scope = {本岗位 ID, 本部门 ID}；人类检索者无 org scope → shared 档对其
仅 owner 可见（与 private 同形，fail-closed）。控制台编辑共享名单不受此限（写侧为 G-03 门）。
**后果**：专项 Agent 检索自动获得本部门/本岗位共享语料（热路径零配置）；团队 Run 的 scope 取
**全体成员的并集**（借调成员带入其部门读取权——与 M78 借调语义一致）；shared 档天然不能跨
workspace 泄漏（C-25 collection JOIN 先行裁剪租户，org scope 只可能在同树内相交）。**替代方案
（弃）**：① 给人类用户建 user→org 成员表——无产品需求支撑且触发 M67 铁律（员工不做成组织
成员，组织树只挂编制）；② 共享名单存 user ID 数组——每单粒度过细、违背「部门级最小经济
方案」定位且人员流动维护爆炸。

**ADR-KN-20261020-E：`shared_with` 用 TEXT[] + GIN + `&&` 重叠，不用 JSONB/关联表**
**决策**：列型 `TEXT[] NOT NULL DEFAULT '{}'`，查询 `shared_with && $n::text[]`（pq.Array），
GIN 索引 `knowledge_documents_shared_with_gin`。**依据**：knowledge 域为 PG-only
（EnsureKnowledgeSchema 依赖 pgvector/trgm，SQLite/CLI 不建知识表——JSON-in-TEXT 双方言
惯例（teams.cross_dept_member_ids）不适用于本域）；域内数组先例 = `knowledge_blocks.
heading_path TEXT[]` + pq.Array 读写；`= ANY($n)` 惯用法全仓普及。**替代（弃）**：① JSONB
`?|`——无双方言需求，操作符生疏且无域内先例；② 关联表 knowledge_document_shares——谓词
需 JOIN，写侧多表事务，为重度稀疏字段（多数文档不共享）付出不成比例代价。

**ADR-KN-20261020-F：org scope 在 runCtx 装配期解析注入 ctx，数据层只读 ctx 键**
背景：数据层谓词现读 `auth.FromContext`（viewerUserID）；但 ① 在跑 Agent 的身份不在 auth
里；② 异步/后台 turn 重建 ctx 时丢 auth（仅回填 ctxuser）；③ 若数据层现场解析
（agent→position→dept）则每次检索 N+1 且需向 data 层引入 org 依赖。**决策**：仿
`knowledge.SearchDeps` 注入链先例（prepareRunContext/buildTeamRunContext 装配 runCtx 时已
持 Agent 对象），**在装配期一次性解析** `scope = {positionID, parentDeptID}`（岗位→部门走
既有两级 GetOrgNode 模式，dept_lead.go:587 同型；每 turn 至多 1 次 org 查询），经
`biz/knowledge` 新 ctx 键 `WithOrgScope/OrgScopeFromContext` 注入；数据层
`docVisibilityPredicate` 与 biz `DocumentVisibleTo` 同读此键（ctx 键宿主包 =
`internal/biz/knowledge`，data/service/tools 均已有其依赖，分层无回头）。无键/空 scope →
shared 腿不生成（fail-closed，现状语义不变）。**替代（弃）**：① 扩展 working_memory
BeforeTool hook（G-15 已在用 AgentIDFromCtx 读同一组键，但该 hook 仅对 `working_memory_*`
前缀工具生效——侦察证实对 knowledge_search **恒空**，是 G-15 在途缺陷，另票记录不并入本轮）；
② 数据层惰性解析（N+1 + 反向依赖，见上）。

- **DDL（配对纪律）**：EnsureKnowledgeSchema 内联 `ADD COLUMN IF NOT EXISTS shared_with
  TEXT[] NOT NULL DEFAULT '{}'` + `CREATE INDEX IF NOT EXISTS ... USING GIN (shared_with)`；
  同步注册迁移 `20261284_knowledge_shared_visibility.sql`（离线工具 RunDDLMigrationsExternal
  只走 registry，两者必须同形；版本尾号以实施时 registry 实尾为准）。
- **写侧**：`DocumentACLStore.UpdateDocumentVisibility(ctx, id, visibility, owner, sharedWith)`
  签名扩参（单端口单实现，原子写）；biz 白名单扩 `shared`，`shared` 要求名单非空且经新端口
  `OrgNodeValidator` 校验：每个 ID 须为存在、未软删、level ∈ {department, position} 的 org
  节点（company 级共享等同于 collection，拒收）；G-03 写门不变（owner/admin/system，IDOR
  404 遮蔽，admin 保留原 owner）；`shared` 切换非空校验失败 → BadRequest。HTTP 面 =
  既有自定义 JSON 路由 `GET/POST /v1/knowledge/documents/{id}/visibility` 增 `shared_with`
  字段——**不动 proto**（peer 会话 G-06/G-11/G-12/G-19/G-30 在途占用 knowledge.pb.go 全文件
  再生成面，proto 增列留前端会话协调时一并做）。
- **读侧（中央扩点，一处改全链路继承）**：
  ① `docVisibilityPredicate` 增 shared 腿（`OR (vis='shared' AND shared_with && $n)`，参数
  按 uid/scope 在否裁剪）；chunk 四路径（dense/BM25/父子回填/图邻居）、文档列表、explorer
  树、links 三查询、双端点谓词全部自动继承；② `DocumentVisibleTo`（deny-by-default，必须
  同步扩否则 shared 文档全灭）增 scope 命中分支；③ `GetDocument`/`GetByRelPath`/
  `GetByContentHash`/pending SELECT 列与 `scanDocument` 增 `shared_with`，biz Document 增
  `SharedWith []string`（GET visibility JSON 回显名单所需）。
- **顺手闭环的既有泄漏面**（同谓词一次性收口，均属「读路径同步过滤」）：
  `SearchDocContentMentions`（autolink 预览把他人 private 文档全文 ILIKE 返回）与
  `ListDocumentNames`（反链/悬挂链泄漏 private 文档名，SP5 遗留）补 `docVisibilityPredicate`
  行级谓词；`ListDocumentNeighborhood` 根节点补 `requireDocumentRead`（404 一致性）；
  评审 C1 追加 `ListResolveCandidates`（autolink 针候选集——他人 private 文档 title/aliases
  不得借 `[[wikilink]]` 拼接扩散进他人正文持久落盘）同谓词收口。
- **未做的**（保持范围）：citation 回采聚合读者（计数无内容输出，维持无过滤）；修复/重嵌
  工人读面**保持无子句**（root ctx 匿名，加子句会让 private 文档停止自愈合——侦察明确警告）；
  proto 字段/列表徽章（scanDocumentSummary 现连 visibility 都不选，随 proto 协调轮一并）；
  部门管理员代管他人共享名单（无需求）；frontend 共享编辑对话框接线留前端会话（GET/POST
  JSON 契约已备）；跨 workspace 共享（谓词+C-25 天然禁止，保持）。
- **测试策略**：DB-free 子句单测沿用现型（SQL 子串 + 精确 args：uid+scope / 仅 scope /
  仅 uid / 皆无 / system）；biz 谓词与写门单测（fake validator）；PG 集成沿用
  knowledge_graph_test.go 全 schema 型（本地 pgvector 环境债红、CI 绿为验收腿）；另加一例
  最小自建表 PG 测试直验 `&&` 重叠绑定（本地可绿，对标 g30 自建表先例）。
- **对抗审查修正**（2026-10-20，6 维 finder × 2 refuter/发现，44 agents / 19 findings /
  3 confirmed / 16 refuted，confirmed 全修 + 回归）：C1[high] `ListResolveCandidates`
  未挂谓词（见上泄漏面追加）；C2[low] `resolveTeamOrgScope` 同部门成员 parent 查询无去重 →
  节点级 `nodeCache` memo（失败不缓存，fail-closed 语义不变）；C3[high] `SetOrgNodeValidator`
  手写进生成文件 wire_gen.go（再生成即静默覆灭）→ wire.go 标记 provider
  `knowledgeOrgValidatorWiring` + wire 正式再生成，生成文件零手改。16 refuted：$n 占位符
  错位、pq nil 数组写 NULL、扫列错位、COALESCE 与 Go 侧语义不齐、team 并集覆盖 spirit
  scope 违 ADR、异步 turn 丢 stamp、其他二进制绕过滤、软删节点存量名单等——均被既有守卫、
  设计意图（ADR-D fail-closed/并集覆盖语义、worker 豁免）或接线事实逐项证伪。

### G-33 实体/谓词级向量召回（graph card 第三腿，2026-10-20）

> 调研报告 G-33：typed edges 只服务图扩展不进召回。对标 LightRAG 双层检索，取其最短借鉴路径——
> 把「词条文/谓词文」嵌入为独立向量档并作为第三腿进 RRF。设计侦察 workflow wf_2cca9f8c-586
> （4 路 reader / 571k tokens）：检索管线五入口收敛链、实体/边文本资产 DDL、向量基建 A/B 方案
> 事实面、金标/契约扩展面全量落地。

**ADR-KN-20261020-G：最小嵌入单元是「卡片文」——词条文一档、谓词文一档，不重嵌既有正文**
**决策**：新增两级向量文档。（a）**词条文**（实体名片，kind='entity'，每实体一卡），
文本 = `展示名` + `（entity_type 非空时追加）` + 别名表（`alias_norm` 升序去重，`\n别名：a, b`）——
全部现货于 knowledge_entities/knowledge_entity_aliases，与库类型无关（本地 vault 刻意不写
词条页，名片档恰补上「名字即锚」）；team 库词条页正文已经 chunk 管线嵌入，不重复向量化。
（b）**谓词文**（边陈述，kind='edge'，每 active semantic 边一卡），文本 =
`主语展示名 <relation> 宾语展示名 — <evidence>`（≤240 runes，沿用 relationEdgeContext 截断
纪律），仅取 `link_type='semantic' AND valid_to IS NULL`——谓词核心闭集仅 8 个、单独嵌无
区分度，必须落到边级证据句；entity/explicit/co_activated 三轨 context 语义分别为展示名串/
wikilink 片段/未核实，一律不入卡。**依据**：嵌入模型对短唯一名与完整陈述句的检索区分度
最好（LightRAG entity/relation 双层即此粒度）；closed 边按 bitemporal 语义本就该退出召回。
**替代（弃）**：① 嵌 core-8 谓词词表本身——8 个词无区分度，且 promoted 档提升管线未落地；
② 嵌词条页正文——team 库已进 chunks，且本地 vault 无页；③ 共激活边入卡——context 语义
未核实，G-33 边界外。

**ADR-KN-20261020-H：独立表 `knowledge_graph_embeddings` + 宿主文档投影，哨兵行方案弃**
**决策**：新表（Raw SQL + TEXT 时代惯例 + DDL Registry 双轨镜像，域内先例 docs 三件套
明文；不入 ent）：

```sql
knowledge_graph_embeddings(
  kind TEXT NOT NULL CHECK (kind IN ('entity','edge')),
  ref_id BIGINT NOT NULL,              -- entity=kg_entities.id / edge=knowledge_links.id
  collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
  text TEXT NOT NULL,                  -- ADR-G 卡片文（审计/重嵌复用）
  source_hash TEXT NOT NULL DEFAULT '',-- 卡片文 sha1（对账漂移检出）
  embedding vector(%d),                -- 与 knowledge_chunks 同一向量维度（启动对账同步）
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (kind, ref_id)
) + knowledge_graph_embeddings_embedding_hnsw HNSW(m=16, ef_construction=64) vector_cosine_ops
+ (collection_id, kind) B-tree
```

ref_id **不建 FK**（kind 复用两宿主表无法表达单一 FK）；孤儿行（实体合并硬删、边闭合）
由对账工人回收。**哨兵行方案（复用 knowledge_chunks）弃——四条机制性硬伤**：doc_id
NOT NULL FK 无单一宿主（实体跨文档多对多）；recency JOIN knowledge_documents 内连接吞掉
无宿主行；CommitIndexedDocument DELETE+INSERT 级联误删哨兵 + MoveDocument 迁移污染 +
chunk_count/LisPendingReembed 计数语义错乱；chunk_role='child' 谓词并入既有 dense/BM25
谓词却无来源腿字段。**可见性（红线）**：卡片表自身无 visibility 列，查询一律经宿主
投影下推 G-08 中央谓词——entity 卡谓词 = `EXISTS (SELECT 1 FROM
knowledge_doc_entities de JOIN knowledge_documents d ON d.id=de.doc_id
WHERE de.entity_id=t.ref_id AND de.collection_id=t.collection_id AND <docVisibilityPredicate(d)>)`
（至少一个宿主文档可见）；edge 卡谓词 = doc_id 与 target_doc_id 两端各一条同一谓词的
EXISTS（沿用 links 双端点纪律）。**宿主文档投影**：卡命中→候选 chunk 粒度化——
synthetic ID `g33e/<entityID>/<docID>`、`g33r/<edgeID>/<docID>`（r=relation edge，
避让摄取命名空间 `<doc>-ch-<i>`/`<doc>-p-<i>`）；content=卡片文；DocID/CollectionID 为
真实值——父回填（ParentChunkID 空=跳过）、applyBaseLevelBoost/access_log/Hebbian 共激活/
GraphExpander 种子按真实 doc_id 自然继承零回归。entity 卡出题宿主 ≤3 文档、edge 卡出
两端 2 文档，卡命中各 ≤8（锚数沿用 dual.go ≤8 先例），每查询新增候选 ≤40 硬上界。
**维度治理**：嵌入列与 knowledge_chunks 同 `vector(%d)`；`reconcileEmbeddingDim` 启动对账
序列扩进本表（**按表独立对账**：chunks 侧 DROP 索引→置 NULL→documents 置 pending→ALTER
TYPE→重建 HNSW；卡片侧 DROP HNSW→墓碑式 DELETE 全表→ALTER TYPE→重建，两维度各自判
漂移——卡片 source_hash 由卡片文推导、embedder 换维不漂移，对账工人无法自愈，必须空表
重建）。**替代（弃）**：① per-dim sidecar
（memory TableNameForDimension 型）——G-39 大题另轮，本轮不与 knowledge 域原地 ALTER
惯例分叉；② 卡片表加 doc_id 列建 FK——实体多宿主语义冲突，见上。

**ADR-KN-20261020-I：第三腿进 `rrfMergeN`——不唤醒 DualLevelSearch，窄端口默认 nil=关**
**决策**：`HybridRetriever.searchRRF` 内在 dense+sparse 之外以同模式 safego 并发跑第三腿
`GraphCardSearcher`（新窄端口，照 SparseSearcher 先例：独立接口、装配层注入、nil=关闭）。
融合改 `rrfMergeN(k int, lists ...[]biz.KnowledgeChunk)` 变参核心，`rrfMerge(dense,sparse,60)`
保留为两列包装（TestRRFMerge_ScoreBreakdown 等三处精确分值/签名钉死测试零改动）；卡片腿
贡献同权 1/(60+rank+1)（v1 不加权，金标裁决再调）。**不唤醒 graphrag.DualLevelSearch**：
六端口生产全缺、FindEntities 无实现、其扩展阶段与 GraphExpander（spread 2 跳 ×0.5/类型权）
双重扩展必然双计；其低层「实体锚投射」思想已由本设计的卡片→宿主→rrfMergeN 直接承载。
**候选携带标记（零 proto）**：biz.Chunk 增 `EntityScore float32`（卡片相似度，贡献者取最大，
Go-only 第五分解字段，rrfMergeN 字面量重建处同步抄录——G-06 钉）；metadata_json 注入
`{"g33":"entity"|"edge","g33_ref":<id>}` 标记（Search/chips/hit-test 可读，G-08 metadata
side-channel 先例）；mode_used 因卡候选天然带 RRFScore 保持 "rrf" 不出 "none" 盲区。
**降级矩阵**：无 `embedding_model` 词法库 → 实体卡退化为 `name_norm 精确/前缀 ∪ alias_norm
精确` 词法命中（同一 repo 端口内部换实现，可见性谓词不变），edge 卡腿静默关闭（边陈述
无结构词法匹配是噪声源，listed 非目标）；dense-only（Simple/即时意图）模式不启用本腿
（成本与噪声控制，v1 只进 RRF 模式）；腿自身失败 → 回退 dense+sparse 融合（warn 日志，
照 dense/sparse 单路失败先例）。**记账**：卡片嵌入走既有 `recordUsage(aux_embedding)`，
`metadata.task_type` 新值 `graph_card` 便于拆分观测（RETRIEVAL_QUERY/'' 并列第三值）。

- **DDL（配对纪律）**：EnsureKnowledgeSchema 内联建表 + 双索引（注释「存量库由迁移
  20261286 补」），同步注册 `20261286_knowledge_graph_embeddings.sql`（原拟 20261285
  与并行会话 tool_invocations_workspace 撞号重编号）；registry 追加禁 gofmt 该文件（既有钉）。
- **写侧——对账投影工人 `KnowledgeGraphProjectionWorker`**（cron 5m；每轮集合上限/全局
  预算 256 行，embed 批 32 沿用 defaultEmbedBatchSize）：仅处理 `embedding_model<>''`
  的集合（词法库无卡可嵌，工人空转）；三类 reconcile——(1) 缺卡：实体/active semantic 边
  存在而卡不存在 → 构造卡片文+嵌入 upsert；(2) 漂移：`source_hash != sha1(text)`（更名/别名
  过户/mergee 归并 keeper 后文本自愈同式）→ 重嵌入；(3) 孤儿/闭合：实体行消失或边
  valid_to 非空 → 墓碑 DELETE。全部按 (kind,ref_id) 幂等，失败单条跳过记 warn 下轮重试。
  冷实体覆盖：实体/边本身由 EntityPipeline/relation extract 热度闸产生，卡工人只投影不
  扩产（零新增 LLM 成本面）。**即时钩子不做**：EntityPipeline/ReplaceSemanticLinks 后即时
  投影属优化，v1 以工人统一承接（5 分钟级延迟可接受，避免双写点纪律分裂）。
- **读侧——`knowledgeGraphCardSearcher`**（internal/knowledge 层，实现 GraphCardSearcher
  端口）：embedSearchQuery 复用（queryCache TTL10m 自动继承）；两条 ANN（kind 过滤各 ≤8）
  → 宿主投影 → 候选构造 → tagEntityScores(EntityScore=卡片相似度 max)；三个独立窄 repo
  接口入 data（SearchGraphCards / SearchEntityCardsLexical / 宿主 docs 查询——不扩既有
  SparseSearcher 签名，不碰 SearchChunksBM25 四分支 SQL，BM25 门禁零风险）。
- **未做的**：唤醒 DualLevelSearch（六端口 + 双扩展冲突，G-34 一并评估）；共激活边入卡；
  本地 vault 词条页（刻意）；edge 腿词法降级；access_log 命中来源腿列（G-15）；entity 独立
  proto 分解字段（metadata side-channel 已备，留前端协调轮）；GraphExpander 与卡片腿的
  种子加权调参（留金标实证轮）；前端 hit-test 面板读取 g33 标记（契约已备留前端会话）。
- **测试策略**：卡片文构造纯函数单测（别名排序/去重/类型空档、evidence 240 runes 边界、
  主语名在边 context 旧名不漂白——重写纪律只服 entity 轨）；rrfMergeN 精确分值三列版
  （照 ScoreBreakdown 同款断言 + 未命中腿为 0 + EntityScore/MetadataJSON 过融合字段清单）；
  searchRRF 三腿并发启动钉（照 RRFStartsDenseAndSparseConcurrently 同款）+ 腿失败回退；
  data 层：host 投影可见性谓词最小自建表 PG 例（私密宿主不可见/双端点一端私密整条不见/
  词法回退分支，照 G-08 pg_test 先例，本地可绿）；全 schema PG 集成照 knowledge_graph_test.go
  先例（CI 腿，本地 pgvector 环境债同 G-08 豁免口径）；金标——gold-corpus.json 增可选
  卡片声明（非 strict Unmarshal 零破坏）+ fixture GraphCardSearcher 桩 + 2–3 条 entity-anchored
  探针用例（期望=种子∪实体腿可达文档且 topK=8 容纳，满分纪律）+ 同 PR 刷快照（GateDrift）
  + 消融对照（装配 nil 关腿复现基线）；对账工人三态（缺卡/漂移/孤儿）+ 预算闸单测。

- **对抗审查修正**（2026-10-20，workflow wf_1fd59d73-51a：4 维 finder × 2 refuter/发现，
  12 agents / 804k tokens / 4 findings / **3 confirmed 全修** / 1 killed；另 1 项自查先修）：
  ① `searchRRF` 降级链是两腿时代写死的早退（dense 失败仅回 sparse、sparse 失败仅回 dense、
  双挂坍缩 Internal），健康卡腿候选被静默丢弃——修正为**任一腿失败回退剩余腿 RRF 融合**，
  双挂退卡腿单路；卡腿无产出（nil/err/空）时保持 legacy 双腿 tag 路径，既有钉分测试零改动
  （钉：`TestHybridRetriever_RRFCardLegSurvivesClassicLegFailure` / `...CardOnlyWhenBothClassicFail`）。
  ② `rrfMergeN` 同分组（三腿并列最多 3 项 1/61）次序取自 map 随机迭代序，trimChunks 截断
  口随机挤出卡腿唯一宿主文档——修正为 `sortChunksByRRFScoreDesc` 同分按 ID 升序破平
  （钉：`TestRRFMergeN_TieBreakDeterministic` 20 轮次序恒定）。
  ③ 对账工人按 `len(upserts)+len(deletes)` 预扣全局 256 预算——持续失败行（重进 plan 无
  backoff）每轮白吃预算，前序集合（ORDER BY id）永久饿死后续集合第三腿——修正为**预算按
  成功行数后扣**（钉：失败墓碑批不占预算、后续集合同轮承接）。
  ④（自查先修，finder 未报）`reconcileEmbeddingDim` 原只读 chunks typmod：两次 ALTER 无
  事务，崩溃窗口会让卡片列永久滞留旧维度——修正为 chunks/卡片**按表独立对账**（见 ADR-H
  维度治理段），对账日志补 `card_from_dim`。
  ⑤ killed（记档不追逐）：词法腿 `name_norm LIKE $2||'%'` 未转义通配符——双 refuter 均认定
  v1 装配不可达（卡腿仅 searchRRF 喂 vec、embedder 空向量必转 error、词法分支为防御路径）；
  仍补 `ESCAPE '\'` + 复用 `escapeLikePattern` 作纵深防御（独立调用面不假设上游过滤）。

---

### G-05-asof 双时态 as-of 查询接口（2026-10-20）

> 差距锚点：调研报告 §22（bitemporal 只写不读）/§31（双时态 schema + GiST tstzrange
> 已落地，「缺的只是查询接口」）/§168（Month 1–2：G-05 as-of 时态查询含 recorded_at
> 第二轴 + 前端演化时间轴）。侦察 wf_ba36bbc4-c92（写侧真相源/读路径盘点/API 契约面/
> 金色回归 4 维并行，本文锚点均出自其核验）。
> 「G-05 起步」为 PDF 版式解析器（报告 §67，已闭环），与本项仅编号撞车，语义无关。

**ADR-KN-20261020-J：as-of 语义口径——边图双轴保真，属性与历史破口显式契约化**
**决策**：as-of 接口只时序化 **`knowledge_links` 边图的存在性/谓词结构**（doc↔target
端点、link_type、relation、valid_from/valid_to 区间），**不**宣称属性双时态：
weight_f/confidence/context 一律返回当前值——写侧原位变异已被测试钉死（ReplaceLinks
未变化边 UPDATE :94-99、ReplaceSemanticLinks :82-87、decay 权重原位乘
knowledge_curate.go:35-38、ON CONFLICT DO UPDATE :105-110），推写侧版本化=重写
decay/Hebbian 热路径，超出本轮范围。R 轴（recorded_at）语义 = **existence-as-known-at-R**
：recorded_at 记录行 incarnation 的插入时刻（全库 Go 代码零 UPDATE——红线由「无代码
写它」守住，须补回归钉），原位变异不升迁；valid_to 恒等于关闭时刻 NOW()，兼作
「记录端」——R 谓词 = `tstzrange(recorded_at, COALESCE(valid_to,'infinity')) @> $R`。
**组合语义矩阵（四象限，接口契约固定）**：
① T=0,R=0 = current 快报：原 SQL 文本不动（`valid_to IS NULL`，partial index 热路径）；
② T≠0,R=0 = T-only：彼时有效、以当前已知信息（R 谓词缺席）；
③ T=0,R≠0 = R-only 知识快照：系统于 R 时已知且当时仍有效；
④ T≠0,R≠0 = 全双轴交：`@> T` ∧ `@> R`。

**已知失真——四类破口显式契约化（接口文档必载，不宣称完整双时态快照）**：
a. **co_activated 边排除在 as-of 谓词外**（`link_type <> 'co_activated'`）：其衰减关闭
   区间被原位复活抹除（StrengthenCoActivations `valid_to=NULL` 同 record，
   knowledge_access_log.go:109/121；knowledge_access_log_test.go:156-171 钉死）——
   如实回放只剩 explicit/entity/semantic 三类；current 快报不受影响。
   **替代（弃）**：复活改新 incarnation——先决条件是推翻钉死测试的复活语义决策，另轮。
b. **orphan 词条删除=边历史物理丢失**（applied→DeleteDocument→FK CASCADE，
   curate.go:543 + schema.go:135-136）——本轮接受并文档化；删除前快照边集/墓碑化
   软删列「未做的」。
c. **closed-on-arrival 空区间不可见**（valid_from==valid_to 低置信语义边留痕，
   knowledge_relation.go:30/97-98）——点区间 range-containment 谓词天然不回放；
   时间轴治理事件视图走边界枚举（valid_from/valid_to 端点序列），非本接口职责。
d. **节点集与可见性=当前态**：documents 无时态、visibleDocIDsSubquery 当前口径——
   接口语义=「边时态 × 当前节点集 × 当前权限」；历史已删文档的边被端点裁剪吞掉。
   附注：supersedes 裁决只改正文+落 fact_version（正交单轴 best-effort 链，
   writeback_entry.go:314-326），边关闭滞后到下次重抽取——图时态粒度=最后一次
   重抽取/治理时刻，非正文变更时刻。

**端口形状（AS-STA-01：新增窄口，既有签名零改动）**：新增**两条窄口**（均
Stability:evolving，定义于 `internal/biz/knowledge/temporal.go`）镜像两热读者——
`TemporalActiveLinkReader.ListActiveLinksAsOf`（扩散逐跳，镜像 ListActiveLinks
投影+ACL）与 `TemporalCollectionLinkReader.ListCollectionLinksAsOf`（图 RPC，
镜像 ListCollectionLinks 投影+ACL+linkTypes 过滤），各加 `span TemporalSpan`
参数；data 侧 knowledgeRepo 同实例实现两接口（断言照既有 `var _ bizknowledge.X =
(*knowledgeRepo)(nil)` 惯例，见 internal/data/knowledge_links_asof.go）；装配=ok
断言接线（knowledge_advanced.go:64-71——`SetActiveLinks` 紧随其后
`SetTemporalActiveLinks`）。既有 ActiveLinkReader/CollectionLinkReader/LinkRepo
签名与存量 fake 零扰动。`TemporalSpan{ValidAt, RecordedAt time.Time}` +
`Any()/ValidOn()/RecordedOn()` 定义于 biz 层（零值=该轴关闭）。

**双 SQL 文本红线**：current 分支原 WHERE 文本逐字节不动（`valid_to IS NULL` 必须继续
匹配 knowledge_links_active_idx partial index——改写成 `(valid_to IS NULL OR
valid_to > $T)` 即失配）；as-of 分支独立文本：T=`tstzrange(valid_from,
COALESCE(valid_to,'infinity')) @> $T`（knowledge_links_valid_idx GiST 承载）、R 谓词
同构、`AND link_type <> 'co_activated'`。四象限由 data 层按 span 零值组合 choose 文本，
current 象限必须落原函数。

**检索/图链载体**：`biz.SearchQuery`（internal/biz/knowledge/knowledge.go:149；
KnowledgeSearchQuery 别名 internal/biz/knowledge.go:12）增 `Temporal TemporalSpan`
字段——struct 值拷贝全链路自动携带（PathPrefix :159 先例），AdaptiveRouter→
GraphExpander.Expand(:69) 签名不动；as-of 仅消费于扩散激活：spreadExpand 两跳共用
同一 span（时态一致性天然），reader 按 span 选择旧/新端口（零值=旧端口，未接线
时态端口回退当前快照记 warn）；AdaptiveRouter 热度链路（applyBaseLevelBoost/
triggerHebbian）以 `!q.Temporal.Any()` 守卫——as-of 检索对运营热度零读写副作用。
**降噪边界（不宣称时态的部分照跑不误）**：G-33 卡片腿是「上一轮投影轮粒度的当前态
物化」，dense/sparse 腿是「当前内容检索」——as-of 语义恒定「仅边扩展腿时序化」，
三腿照常运行、契约文档化（卡片表无 T/R 列、entities 无时态，皆当前态成立的边界内）；
检索历史态需求若扩到 chunk/正文 → G-32 已裁决落地（ADR-KN-20261022-I，2026-10-22
拍板：本轮交付全量时态）：化身区间经 `document_versions`/`chunk_archive` 推导，
citation 证据回落+as-of 时态端口（RecallChunksAsOf/ListChunksAsOf，独立 SQL 变体）
接入 `SearchQuery.Temporal`；current 读面逐字节不动；卡片腿与块/refs 仍 current-only。
**治理读显式排除**：ListStaleEntries 的全历史聚合（closed_ratio 口径）、
decay/orphan/contradicts/hub/density 读全部 current-only 且刻意如此——as-of 谓词
只经新端口逐读者接入，禁止横切 WHERE 注入。

**前端现状红线**：ListLinks(:131)/ListCollectionLinks(:163)/ListResolvedLinks
（knowledge_explorer.go:48）三读者当前**不过滤 valid_to**——已关闭历史边今天已漏进
3D 图谱与关联区。v1 契约：无时态参数=当前行为逐字节保持（不得静默改变 3D 图谱观感）；
(T,R) 非零才进 as-of 分支。默认口径收拢（closed 边隐藏）属行为变化评审，留前端轮。

**ADR-KN-20261020-K**：as-of 读面**新增两条 custom-HTTP 端点**（proto 红线下生成
handler 无法收新字段；US-39 autolink-preview :241-244 注释「custom JSON，避免 make
api」同宗先例）。中央登记于 internal/server/http.go :177-193 注释块（注册必须先于
proto :107-110）；鉴权=默认 JWT + WorkspaceFilter（:74-79），handler 内
`assertCollectionAccess`（knowledge.go:412）；错误映射复用 `writeKnowledgeJSONError`
（NotFound→404 / BadRequest→400 / **Forbidden→404 防 IDOR** / Conflict→409，
knowledge_http.go:22-35）。

**决策**：`POST /v1/knowledge/search/asof`——body 复用 search 现字段集
（collection_id 可空=联邦/query/top_k/min_score/filter_json/use_rerank/
rerank_candidates/hybrid_search/path_prefix）+ `valid_at`/`recorded_at`（RFC3339，
空串=零轴，解析失败→BadRequest）；SearchQuery 在**新 handler 内构造**
（knowledge.go:1004-1014 主构造点与 debug-hit-test/tool×2/cue 预检索/G-13 fixture
五个存量构造点全部零改动——零 span=current 快报，值拷贝自动成立）；热度守卫与扩散
时态腿已内置。响应复用 SearchResponse `{"chunks":[...]}`。
`GET /v1/knowledge/vaults/{collection_id}/graph/asof`——query：`valid_at?`/
`recorded_at?`（RFC3339）/`link_types?`（逗号）/`path_prefix?`；biz 新增
`ListCollectionGraphAsOf`（镜像 graph.go:61，节点集/degree/裁剪逻辑复用，边源走
TemporalCollectionLinkReader；邻域经 graph.go:139 内部复用自动覆盖）+
`SetTemporalGraphRepo`（镜像 SetGraphRepo :41，AS-STA-01 新增窄口装配）。
响应复用 ListCollectionGraphResponse `{"nodes","edges"}`。
**前端契约（TK-G05-01，交前端会话）**：新增并列 client 函数
`searchKnowledgeAsOf`/`listCollectionGraphAsOf`（api.ts:646-657/736-754 现函数
签名与序列化一字不动）；as-of 图**不写** graphsByCollection 共享缓存
（stores/knowledge/index.ts:88-95——否则污染 byte-level 现状共享缓存）；T≠0 视图期间
暂停应用 WS 增量（useKnowledgeGraphDeltaWs.ts/graphDelta.ts——现状红线派生约束）；
UI 挂载首选 KnowledgeGraph3D.vue HUD（邻域 slider 先例）+ useKnowledgeGraph.ts
temporal 状态与 linkTypes 并列。
**排除**：整条「演化时间轴」事件流端点（valid_from/valid_to 端点序列回放）不属本轮
——ADR-J(c) 边界已划；治理读 current-only 显式排除（curate.go:152-176 签名族佐证）。

**时态金色探针（G-13 扩张，eval-gates 侦察 wf_ba36bbc4-c92 已收官）**：金色门禁
现状锚点——`GoldenCase`（internal/knowledge/eval/golden.go:21，无 span 字段）、
`Searcher`（runner.go:11，Search(ctx,query,topK) 无 span 通道）、`FixtureCorpus`
（fixture_search.go:51，edges 仅 Weight=1 无性态字段）、fixture 的 GraphExpander
未接时态端口（span 查询会静默降级 current 误判）、门禁单一实源 `gate.go:14,57`
（GateStale 键=SetName+TopK 耦合、GateDrift、MRRRegressionThreshold=0.02）。
**决策**：(a) `GoldenCase` 增可选 `valid_at`/`recorded_at`（RFC3339，omitempty）；
(b) corpus 增声明式 `edges` 数组（doc/target/link_type/relation/valid_from/valid_to/
recorded_at——co_activated 与空区间可摆）；(c) `buildFixtureSearcher` 接线
`fixtureTemporalLinkReader` 进 GraphExpander；(d) runner 对 `TemporalSearcher` 窄口
机会断言（既有 12+2 探针 Search 通道签名零改动）；(e) **独立第二 EvalSet 独立门禁**
（gold-set-temporal.json + 专属 SetName/topK——直改 gold-set.json 会因 SetName 键
耦合炸 GateDrift/GateStale 全体；探针形态「t0 应含后被关的边 / t2 排除之」「R0 应含
/ Rmid 排除」，expected_ids 回看 case span 生效）；(f) CI 第二 goldeval 腿需动
.github/workflows（本会话红线禁触 → 先以 go test 目标承载，CI 腿另待协调窗）。

- **测试策略**（全部已落地）：
  - **PG-only 门**：as-of 谓词依赖 tstzrange/GiST——`internal/data/knowledge_links_asof_test.go`
    走 `setupKnowledgeSearchRepo`（testhelper.SetupTestPGRaw，本地 PG 不在即 t.Skipf）；
    sqlite 双注册（chat_turn_submit_claim_test.go:10）阻塞解除后与 TK-G33-02 GraphCard
    三例同批补跑。
  - **fixture 摆位纪律**：直写 timestamp 列（knowledge_bitemporal_test.go 模式）；fixture
    必须只摆生产可达形态——closed-on-arrival 是同事务 NOW() 三点重合
    （valid_from==valid_to==recorded_at），合成 `recorded_at < valid_to` 状态物理不可达。
  - **四象限矩阵**：active 八象限（current/T=T1/T=T2/R=R0/R=Rmid/dual×3）+
    collection 七象限（linkTypes 叠加、ORDER BY id 稳序钉、zero-span 全量 legacy
    语义钉——不做 valid_to 过滤的前端现状红线）。
  - **三边界钉**：recorded_at 零更新回归（ReplaceLinks 原位更新+关边两路径，
    R 轴精确性前提）/ closed-on-arrival 空区间任何象限不可见 / co_activated
    唯 current 象限可见。
  - **current 字节级等价钉**：biz TemporalSpan 语义矩阵与 SearchQuery 载体钉；
    knowledge 层 `NoSpanIgnoresTemporalPort`（时态端口零调用）与
    `NoSpanKeepsHeatSideEffects`（boost/日志/Hebbian 照旧）双向回归护栏。

- **落地状态**（2026-10-20，全部腿 GREEN 收官）：biz 时态端口与 `TemporalSpan`
  （internal/biz/knowledge/temporal.go）；data 双 SQL 谓词四象限（internal/data/
  knowledge_links_asof.go）；扩散/路由 span 传播与热度守卫（knowledge/graph_expander.go、
  adaptive_router.go）；服务双端点（internal/service/knowledge_asof.go，
  internal/server/http.go 自定义路由注册——见上表尾部两行）；时态金色探针决策
  (a)–(e) 全落（eval/golden.go + runner.go + fixture_search.go 三改 +
  gold-{corpus,set,snapshot}-temporal.json 三件套，既有 14 用例金集字节零改动、
  快照实测 closed↔future 翻转成立）；goldeval CLI temporal 腿 `cases=6 violations=0`；
  (f) CI 第二 goldeval 腿留协调窗（红线禁触 .github/workflows）。PG 运行时测试
  （data 层四象限 + 三边界钉）因 sqlite 双注册测试门阻塞，门开与 TK-G33-02 同批补跑；
  recorded_at/vstzrange 索引补强是否需独立迁移条目待 PG 实测与索引维度独立复审结论。
- **对抗审查修正**（2026-10-20，专项 workflow w8ptxy558：4 维 finder × 2 refuter/发现，
  13 findings / **6 confirmed 全修全钉** / 7 refuted；**索引维度独立复审**另立
  standalone 重查：1 major 确认见下➌➐条外条目）：
  ① F1[major] 热度守卫漏第三写入方——`Retriever.logAccess`（Router 缺席的 service
  else 分支与 FederatedRetriever 三退化点均可达）未受 `!q.Temporal.Any()` 守卫，as-of
  查询会回放污染访问热度——补 retriever.go logAccess 守卫（钉：`TestRetriever_AccessLog`
  `_SpanSkipsHeatWrite`：span 零写、零 span 照写双向钉）。② F2 as-of 图未接线时态端口
  静默回退 current——补 warn（`knowledge.graph.temporal_fallback`，graph.go）。③ F3
  服务头注释枚举 ADR-K 排除面不齐（仅 use_eval，漏 rewrite_strategy 与 SearchQuery 侧
  五字段）——注释精确化。④ F4 补「400 先于存储读」钉（InvalidSpan 用例 lastSearchQ
  零触）。⑤⑥ F5/F6 fixture edges 校验补空白边 ID 拒收与重复边 ID 钉（Validation 表
  9 变异项）。7 refuted：MoveDocument 集合改写属 ADR-J(d) 已声明失真类、colID trim
  drift 为刻意口径、1MB LimitReader 截断由 unmarshal 拒绝兜住等——逐项被既有守卫与
  决策文本证伪。
- **索引维度复审修正**（2026-10-20，standalone 重查：1 major / 其余维度全验过——
  零 span 委托字节安全、T-only 象限 GiST 表达式索引精确命中、逐跳 reader 四象限由
  既有非 partial btree 全覆盖）：F1[major] **`ListCollectionLinksAsOf` R-only/双轴
  谓词零索引路径**——`recorded_at <= $R` 无 btree、`valid_to > $R` OR 分支与 GiST
  表达式索引凑不起 BitmapOr → 全库 as-of 图读退化全表 seq scan，双时态表历史行只增
  不删代价线性膨胀（每次图/时间轴交互均触发）。修正：`knowledge_links` 增
  **`(collection_id, recorded_at)` 组合 btree**（两读者均以 collection_id 前置等值 +
  recorded_at 驱动范围，单索引同服务 R-only 与双轴两 SQL 变体）——迁移
  `20261287_knowledge_links_recorded_idx.sql` + EnsureKnowledgeSchema 双轨镜像 +
  `TestMigration20261287_RecordIndex` 幂等钉（皆随 sqlite 门开与四象限矩阵同批跑）。

### G-34 MOC 自动摘要词条 + 全局汇总档（2026-10-21）

> 差距锚点：调研报告 §111（G-34：AdaptiveRouter 只有复杂度路由，「本部门对 X 的
> 全部立场」类问题无路径，GraphRAG DRIFT 可借 moc_emerge 摘要词条零重写落地）/
> §112（G-35：moc_emerge 只产提案不产摘要词条，hub 簇→高风险提案→人工手动建 MOC
> 就断，applied 后零落地）/§168（Month 1–2：G-34 MOC 自动摘要 + 全局汇总档）。
> 设计侦察 workflow wf_f06dfa27-e1f（moc_emerge 真相源/router 检索链/载体与金标/
> 前端契约 4 维并行，52 facts 附锚）；本设计锚点全部出自其核验。
> 关键事实：applyGovernanceDisposal switch 无 ProposalKindMOCEmerge case（:536-578，
> applied 纯状态翻转零产物）；runMOCEmergeTask 不调 LLM（payload=纯统计字段）；
> EnsureEntityWikiPages 提供成熟的词条幂等路径（entity_wiki.go:89-131）。
> 账户分层：MOC 自动摘要词条 = 闭环 G-35 断点；全局汇总档 =（a）摘要词条本身即
> 语料级语义单元（自然进检索与被引用）+（b）语料级路由档/DRIFT primer→follow-up
> 新腿为 v1 显式不做项（Month 2+ 与 G-36 并评）。

**ADR-KN-20261021-A：MOC 载体 = `moc/<slug>.md` 普通词条页（doc_type='moc'），applied
同步落骨架——「MOC 是否参与默认检索」开放问题收案为「参与」**
**决策**：applyGovernanceDisposal 增 ProposalKindMOCEmerge case；applied 时**同步**落
骨架 MOC 页（零 LLM）——取 payload 的 suggested_title/suggested_path/members/density/
degree，复用 entries 词条幂等模式（entity_wiki.go:89-131）：GetDocumentByRelPath
get-or-create → 正文纯函数构造（`buildMOCSkeletonBody`：frontmatter title、簇统计段、
`## 摘要` 占位段、`## 簇成员` wikilink 清单——成员链用 rel_path 全路径形
`[[<rel 去 .md>]]`，与 entityWikiWikilink 同式）→ CreateDocument（RelPath、MimeType
text/markdown、Organized:true、Status:'pending'、**DocType:'moc' 显式落列**——
CreateDocument 直写 doc_type（knowledge_documents.go:39），绕开 inferDocType 的 note
默认；前端图谱调色板分组/summary chip 白拿，零渲染分支成本）→ RebuildBlockIndex →
writeBackReplay 钩子（PromoteTouchedDoc，与 ensureOneEntityWiki 完全同型）。
**检索面零改动**：MOC↔成员关系经正文 wikilink 由块索引物化为 explicit 边（与
entries 词条款同源机制，无需 ReplaceLinks 手工写边）；`moc/*` 不在
SearchQuery.ExcludePathPrefixes（排除面只含 writeback inbox 前缀，
agent/knowledge_inject.go:206 与 tools/knowledge/tool.go:176/:417），故 MOC 页
**自动参与 BM25/向量召回与被引用**——curate.go:760「MOC 是否参与默认检索」的
开放问题就此收案为「参与」（治理提案本身 gating 了落地，参与默认检索是提案被
采纳的语义后果）。
**隔离保证**：ListHubClusters 两端 `rel_path LIKE 'entries/%'` 硬约束
（knowledge_curate.go:196-197）→ `moc/*` 页永不成为下一轮 hub，无自我摘要循环；
治理支部队限定（curate 只扫 team 库），disposal 内复 Gate VaultBackendTeam 防御深
（与 EnsureEntityWikiPages :29 同纪律）。冲突处理：suggested_path 已存在（与
title 无关的既有页）→ entrySlug(title)+短后缀重试；正文已含同名 ## 簇成员段时
幂等跳过（不再写）。实体/关系抽取轨与 chunks 走 entries 同款 replay 管线，
不新增 moc/ 过滤（entries 先例证明可接受）。
**替代（弃）**：(c) 新表——要自建整条检索/可见性/嵌入管线，零收益；(d)
knowledge_graph_embeddings 扩 kind——CHECK 约束+三窄接口宿主投影全要发明，
且「摘要词条本质是文档+chunks 不是图卡」（语义错位）；(e) 新 mode 档/第四列
primer 腿随本轮上线——动 classify/selectMode 三档结构要 rerun 全部金标钉，
语料级路由是独立大题（见「检索升级边界」）。

**ADR-KN-20261021-B：去重键加簇成员签名——簇结构变化可重提，解除三态死信**
**决策**：moc_emerge 提案 dedup_key 由 `'moc:'+hubDocID` 改为
`'moc:'+hubDocID+':'+memberSig`，memberSig=成员 rel_path 排序后 hash 截断
（纯函数 `mocMemberSignature`）。旧键语义是三态永久禁言：人工 applied 但未真建
（断链期历史提案）→ 该 hub 永不重提；真建后簇演化（新成员涌现）也不再触发——
侦察确认的死信与刷新缺失。新键下 hub 身份不变、成员集合变更即新提案自然到达；
同键三态不重复语义不变（HasProposal 机制零改动——dedup 是提案侧计算量）。
存量旧键提案不受回溯（新提案自然新键；历史 payload 无需迁移）。
处置幂等双保险：提案层 dedup（第一道闸）+ 文档层 collection_id+rel_path
get-or-create（最终幂等担保，与 entries 先例一致）。

**ADR-KN-20261021-C：摘要 LLM 增强 = curate 轮内对账子任务——窄端口
MOCSummarizer + aux_moc_summary 记账 + 预算闸**
**决策**：applied 只落骨架（同步路径不引 LLM——ResolveGovernanceProposal 是 RPC
同步链，LLM 延迟/失败面不该进提案处置）；摘要由 KnowledgeCurateWorker 轮内新增
`runMOCSummaryTask` 对账补齐（默认 6h 周期 + CurateKnowledgeNow 手动即时触发——
治理对话框「立即治理」按钮已成链）：扫描本库 `moc/` 前缀下 `## 摘要` 段仍为占位文的
→ 读页内 `## 簇成员` wikilink（页即真相源，人手工补链也纳入）反查成员文档
（GetDocumentByRelPath）→ 聚合 hub/成员 Summary+正文截断输入摘要 → 生成后
`ensureMOCSummarySection` 幂等替换 `## 摘要` 段（H2 段精确替换，仿
ensureEntityWikiBody 幂等纪律）+ UpdateDocumentContent + RebuildBlockIndex +
writeBackReplay。可调失败/端口未接线 = 占位段保留，下轮自然重试（无死信）。
端口形状（AS-STA-01 新窄口，既有签名零改动）：biz `MOCSummarizer`（
`SummarizeMOC(ctx, MOCSummaryInput{Title, HubSummary, MemberSummaries[]}) (string, error)`
，Stability:evolving）+ `SetMOCSummarizer` 装配注入、nil=跳过本任务（与
SetWriteBackArbiter :105-108 先例同型）。生产实现放 internal/knowledge
（resolveModel→ResolveLLM 链 + Prompt 蒸馏「本簇主题与成员立场」200-300 字）。
**预算与记账**：每轮每库摘要上限 mocSummaryBudgetPerRound=4 页（防第一次烧
token 的 cron 无闸——CostGuard 插件不拦 cron 上下文）；记账走 RecordAuxLLMUsage +
新 usage kind `aux_moc_summary`（usage_types.go 常量 + biz/usage.go 镜像两行
模板，scheduleBudgetAlerts 告警免费获得）；输入截断（hub ≤800 runes、成员各
≤400 runes、总预算 ≤4000 runes）。
**DRIFT primer 语义单元兑现**：经此任务 `moc/*` 页获得「簇主题+成员立场」蒸馏
摘要段——检索侧 dense/sparse 命中 MOC 页时命中的是摘要语义而非骨架；这正是
GraphRAG DRIFT primer 需要的物化资产，primer→follow-up 路由升级（v1 不做）
届时直接消费。

- **检索升级边界（v1 不做项，显式挂账）**：语料级意图维度（classify 第四信号/
  新 mode 档）、DRIFT primer→follow-up 新腿与 rrfMergeN 第四列、SummaryScore
  分解字段、「命中来源腿」观测列——Month 2+ 与 G-36 PPR 一并评估；
  HasProposal 的 payload->>'dedup_key' 表达式索引（提案量级不足以触发，每类
  ≤50/轮）；butler proposals 2048B members 截断前 8 沿用（D19-b 教训的既定
  纪律）；chips MOC 徽章（chunk 级标记无通道，挂前端票）。
- **前端最小特化（本会话同 PR）**：governance.ts 的 kindLabel/proposalSummary/
  proposalCaption 增 moc_emerge 分支（显示 suggested_title、簇规模 n、density、
  members 前 8 + k_total 标注——与 butler 截断口径一致）；applied 决策按钮
  文案「建 MOC」。零新端点（resolve 复用 ResolveGovernanceProposal RPC）、
  零新渲染组件（doc_type='moc' 白拿图谱分组）、零新 WS noticeType（MOC 页
  wikilink 边经 RebuildBlockIndex 自然触发 graph.delta 工作台失效）。
- **测试策略**：
  - biz：buildMOCSkeletonBody/ensureMOCSkeletonBody 纯函数钉（页头/统计/占位段/
    成员清单；空成员/单成员/多成员；幂等重放零改动）；disposal moc case fake
    repo 钉（CreateDocument 行形态+DocType+Organized+pending；replay hook 被调；
    既有页 Update 路径；rel_path 冲突后缀化；非 team 库 Gate）；提案 Guards 钉
    keep_old/keep_new 不变。
  - biz：mocMemberSignature 钉（成员无序等价/成员变更新键/截断长度）；dedup
    三态语义不变回归（curate_test 现有钉全绿）。
  - curate 对账：MOCSummarizer nil 跳过钉；stub 摘要→H2 段精确替换钉；失败
    Warn 占位保留钉；预算截断 ≤4 钉；usage kind 落录钉（stub recorder 断言
    aux_moc_summary）；DryRun 全跳过钉。
  - 金标：第三档 gold-{corpus,set,snapshot}-moc.json——**fixture 代码零扩展**
    （MOC↔成员=explicit 边复用 G-05 edges 声明机制）：探针=「全局问题 seed
    锚定 MOC 页 recall 1.0 且 MRR 1.0（MOC 命中后 explicit 扩散成员可达）×2
    + 零集对照 1」；既有 14+6 用例金集字节零改动；`-gold.update` 同轮刷
    snapshot；消融用 nil Summarizer fixture 等价路径（探针不依赖摘要文——
    seed 语义即「最坏假设只命中锚点」，摘要段命中与否不进门禁断言）。
  - 前端：governance.ts 三分支的 unit 钉（vitest 既存文件同款）。

**落地纪要（2026-10-21 同 PR 闭环）**：端口形状实测收口为
`MOCSummarizer.SummarizeMOC(ctx, MOCSummaryRequest{CollectionID, DocID,
RelPath, Title, Hub/Members MOCSummaryExcerpt, Truncated})
(MOCSummaryResult, error)`（设计稿 Input 结构升级：DocID/RelPath 供 usage
metadata 与日志锚、Truncated 供 prompt 降级提示；`MOCSummaryExcerpt` 承载
截断后节选）。代码锚：biz `internal/biz/knowledge/moc_summary.go`（对账 +
mocMemberLinks/mocTitleFromBody/ensureMOCSummarySection/mocExcerpt 纯函数）、
生产 `internal/knowledge/moc_summarizer.go`（镜像 WriteBackArbiter
ResolveLLM purpose "moc summary" + 60s mocSummarizeTimeout +
KNOWLEDGE_MOC_SUMMARIZER_DISABLED）、usage adapter
`internal/service/knowledge_moc_usage.go`（mocUsageRecorder 镜像
embedUsageRecorder，metadata_json{task_type:moc_summary}）、data 扫描口
`KnowledgeCurateRepo.ListPendingMOCSummaries`（占位文案过滤 + ORDER BY d.id
稳定序）、wire 手合三镜像 `knowledgeMOCSummarizerWiring`（wire_part2.go ↔
wire.go ↔ wire_gen.go）。对抗评审（54 agent workflow wf_0132085c-3bf，
6 维找茬 16 条/3 透镜证伪确认 14 条）后加固落码（钉入源码注释 F 系列）：
**F1/F4** 落笔一律以 LLM 返回后重读的新鲜正文为基+重跑占位门（快照盲写窗口
60s→毫秒，窗口内人工编辑/删除让位人工作主）；**F2** 总预算耗尽整段丢弃
成员同样置 Truncated；**F5** 结构性不可逆失败（缺簇成员段/wikilink 空/hub
已删）占位行内改写为「（摘要自动补齐暂停：…）」标记退出待补集（不自动
恢复、防扫描槽饥饿）；**F6** 成员读取仅 NotFound 跳过、瞬时错误整页留下轮；
**F7** LLM 连续失败 3 连熔断本库本轮（防故障期 32 页×60s 串停）；**F12**
零 token 成功行不记账（对齐 title/memory/embedder 三先例）；**F13** 金标
第三档注释勘正（门禁锚接线，摘要文不进门禁断言）；**F14** 前端
parseProposalPayload 数组 JSON.stringify 保形 + mocMembers JSON.parse 优先
（成员路径含逗号不再碎成幻影成员）。5 条「提交边界」找茬经逐条核实为
不提交红线下的多轮共仓假象（G-05 时态金标注册/G-33 projection provider/
agentSidebar+spirit status i18n/domains.go temporalGraphLinks 均为已闭环
前轮留守的未提交产物，diff vs HEAD 必然内含），无拆出空间、如实挂账。

### G-37 实体消解分层管线（2026-10-21）

> 调研锚：`docs/reports/2026-10-19-research-knowledge-base-sota-gap-analysis.md`
> §114（G-37 现状 embedding 建议 O(N²)+截断 500+人工 RPC；KGGen/DistillER：
> ANN/倒排预对齐→LLM 只裁灰区→裁决+证据进提案二审）/§168（Month 1–2 路线图）。

**缺口（五路侦察 wf_abf46429-54d 钉钉：63 confirmed / 7 corrected / 0 rejected）**：

- **实时算 + O(N²) + 零持久化**：`Usecase.ListEntityMergeSuggestions`
  （`entity_suggest.go:60`）RPC 内整库 `ListEntities` → Go 侧截 500
  （`embeddingSuggestionMaxEntities` :29）→ 整批 embed 展示名 → 双层循环全对
  枚举余弦（`embeddingPairSuggestions` :116）；阈值 0.80/0.90 与上限全是 Go
  常量，建议不落表/不缓存/每次请求重复付费 embedding；生产唯一点 =
  `knowledge_governance.go:72` HTTP handler。
- **裁决直执绕开提案体系**：`MergeKnowledgeEntities` RPC（`knowledge_governance.go:20`）
  → `MergeEntities` 立即执行；`applyGovernanceDisposal` switch 只有
  orphan/conflict/moc_emerge 三 case（`curate.go:544`），`evolution.go:59`
  注释预留 "merge" 无常量/无生产者/无消费者；人工二审通道
  （ListGovernanceProposals/ResolveGovernanceProposal）与实体建议零接线。
- **LLM 完全缺席知识实体消解**：词法归一化组纯字符串比对；LLM-dedup 先例
  只在 memory sleep-time（`sleep_time.go:436` case "merge"，prompt
  `sleep_time_part2.go:12`）——协议模板可借、代码不可复用。

**关键结构抉择**：候选对齐复用 G-33 实体卡片档（`knowledge_graph_embeddings`
kind=entity，卡片文含别名集，合并后 source_hash 漂移自愈重嵌）——ANN/词法两
窄口均已物化带宿主投影下推 G-08 谓词，零新表零新索引零新 proto 零新迁移；
合并施加复用既有 `EntityRepo.MergeEntities` 四层事务（`link.go:93`
提及重指/撞名别名删/其余过户/name_norm 落 keeper 别名，幂等）。实时预览
RPC 与直执 RPC 均不动（proto 红线），新管线走提案二审轨道并行运转。

- **ADR-KN-20261021-D（裁决提案化 + 处置施加）**：新增
  `ProposalKindEntityMerge="entity_merge"`；curate 轮内 `runEntityMergeTask`
  （注册位 moc_summary 后）产高风险 pending 提案；`applyGovernanceDisposal`
  增 case：payload 平铺字段（keeper_id/keeper_name/mergee_id/mergee_name/
  similarity/sources/llm_reason/evidence{shared_doc_ids≤8,mergee_alias≤8}/
  dedup_key——agent 出口 2048 cap 纪律：摘要+锚点进 payload，全文不进）→
  `u.entities.MergeEntities(colID, keeperID, []int64{mergeeID})`；缺字段降级
  no-op（mocParamsFromPayload 先例）、keeper NotFound 视为幂等成功（orphan
  DeleteDocument NotFound 先例）、mergee 不存在端口自身跳过。rejected 静默
  关闭；keep_old/keep_new 守卫不放宽（仍仅 conflict 合法，`curate.go:538`）。
  dedup_key = `entity_merge:<keeperID>:<mergeeID>:<sig>`，sig =
  sha1(round2(sim) ∪ sources 排序 ∪ llm_verdict) 截 10 hex
  （mocMemberSignature `moc.go:51` 先例）；三态全含（pending/applied/rejected）
  ——同签名拒绝即沉默，证据演进（sim 漂移/LLM 裁定翻案/别名集变化）改签名
  可重提（ADR-KN-20261021-B 先例）。施加边界：G-33 卡孤儿交既有 5m 投影对账
  墓碑（单写点纪律 `graph_projection.go:13-15`，零新增删除点）；词条页收敛
  与 wikilink 漂白不做（TK-G37-01——与直执 RPC 现状同语义，无回归）。
- **ADR-KN-20261021-E（ANN/词法预对齐候选腿，替 O(N²)）**：每轮
  `ORDER BY md5((id::text || :daySalt))` 日轮转扫描 ≤`entityMergeScanLimit`=32
  实体（同日确定序镜像 SQL 扣减次序，跨日轮转防少数顽固对独占槽位——无
  游标状态的确定性轮换）；对扫描批整批 embed 展示名（1 次 Embed 调用）→
  每实体 `GraphCardVectorSearcher.SearchGraphCards`（biz 窄口 G-33 已定义，
  `graph_card_search.go:31`）kind='entity' limit=8 过取 1 结果侧滤自身 ref_id；
  norm 冲突组按 entity_suggest 同规则并入（sim=1.0 确定档、对级去重 norm 优先）。
  ANN score=`1-(embedding<=>$q)` 即余弦免二次比对；merge 判阈只认向量余弦
  （词法前缀分不作 merge 阈值——选型纪律）。卡片消融/词法库无卡窗口 ANN 空
  → 该实体仅 norm 候选降级不报错（镜像 embedder nil 先例）。候选对总量按 sim
  降序封顶 ≤64/轮。每实体 ANN 上限沿用 GraphCardSearchKindLimit=8 常量不动，
  G-33 装配链（GraphCardSearchKindLimit 钉死处）不受新调用方影响。
- **ADR-KN-20261021-F（LLM 只裁灰区 + 护栏纪律全镜像）**：sim ≥0.90 免 LLM
  直接提案（tier=auto 证据自证）；[0.80,0.90) → `EntityMergeAdjudicator`
  （LLM 单轮 JSON `{"verdict":"yes|no","reason":"…≤120r"}` 严解析，非 yes/no
  或解析失败 fail-close → no）；no 静默不落提案零噪音；<0.80 丢弃。负面裁定
  零备忘——md5 日轮转保底每对至多 1 次/日 LLM 调用 + 预算闸双重封顶，观测到
  thrash 再上指纹表（TK-G37-04，不为理论问题建表）。端口三件套镜像
  MOCSummarizer：biz `EntityMergeAdjudicator`、`SetEntityMergeAdjudicator`、
  `SetEntityMergeCandidateRepos(ann GraphCardVectorSearcher)`（embedder 复用
  既有 EntityEmbedder 窄口 `entity_suggest.go:53`，wire 已有生产件）；生产
  `internal/knowledge/entity_merge_adjudicator.go`（ResolveLLM purpose
  "entity merge adjudication" + 60s 超时 + `KNOWLEDGE_ENTITY_MERGE_
  ADJUDICATOR_DISABLED`）；usage 记账 `aux_entity_merge` 双镜像
  （`internal/biz/usage/usage_types.go` ↔ `internal/biz/usage.go`）+ service
  `entityMergeUsageRecorder`（metadata_json{task_type:"entity_merge_adjudication",
  keeper_id,mergee_id}）——零 token 成功行不记账（F12 纪律）。预算/断流：
  `entityMergeLLMBudgetPerRound=4` 成功后扣、3 连失败熔断（tripped 本库本轮）、
  结局四态枚举（emSkip/emRetry/emLLMFail/emOK——mocFillOutcome `moc_summary.go:54-62`
  先例）；DryRun/非团队库/embedder 或 adjudicator 未接线静默跳过；actions 追加
  "entity_merge" 仅 filled>0；`rep.EntityMergeProposals` 进 Info 聚合日志。
- **前端最小特化**：`proposalSummary('entity_merge')` = `{keeper_name} ←
  {mergee_name}`；`decisionsForProposal` 仅 applied/rejected（复用默认分支零
  改动）；caption = `相似度 {similarity} · 裁定 {llm_reason}`（≤一行截断 + tooltip）；
  decisionLabel applied=「合并实体/Merge entity」；i18n 双语
  govKindEntityMerge/govApplyEntityMerge 两键。

**边界（v1 不做项，显式挂账）**：

- TK-G37-01 合并后词条页收敛（`entries/<mergee>.md` 归并/跳转）+ wikilink 漂白
  （LinkResolver 只读 documents title/aliases JSONB，实体别名表不参与解析——
  [[mergee]] 在合并后仍解析旧页；与直执 RPC 现状同缺陷，无回归但属已知洞）。
- TK-G37-02 实体卡同步删除进合并事务（现 5m+预算排队墓碑窗口；需先与投影
  工人单写点 ADR 纪律对表，不擅自开第二删除点）。
- TK-G37-03 实时建议 RPC（500 截断/O(N²) 预览）退役或分流读 pending 提案
  ——proto 变更窗口期再动。
- TK-G37-04 LLM 负面裁定指纹备忘表（md5 日轮转观测到 thrash 再上）。
- TK-G37-05 trgm 倒排候选腿（CJK 模糊/子串对齐；20261242 memory_facts
  gin_trgm_ops 先例形制；CJK 场景 trgm 优于 tsvector 'simple'）。
- TK-G37-06 keeper 卡 source_hash 自愈链观测（合并→别名集变→重嵌已闭环，
  仅落观测计数，无机制新增）。

**测试策略**：

- biz：`entity_merge.go` 套件——候选构造（norm 优先/ANN 自身过滤/对级去重/
  sim 降序截 64/ANN 空降级 norm-only）、分层裁定（≥0.90 免 LLM/灰区 LLM
  yes→提案 no→静默/<0.80 丢弃）、预算后扣+3 连断流+日轮转序稳定、dedup_key
  证据演进重提 vs 拒绝沉默、disposal case（applied→MergeEntities 端口调用/
  keeper NotFound 幂等/缺字段 no-op/rejected 零调用）、DryRun/非团队库/未接线闸。
- 生产 adjudicator：JSON 解析（含 ```json 包裹剥除）/非 yes|no fail-close/
  超时与错误上抛/usage 成败记账/零 token 成功跳账。
- 前端：summary/caption/decision 分支 + i18n 键（governance.spec.ts 同款）。
- 金标：不进门禁（merge 是治理施加非检索接线——F13 勘正口径沿用）。

**落地纪要（2026-10-21，实施与设计的差分 + 对抗评审处置）**：

- **Go 侧日轮转替代 SQL md5 排序**（设计 ADR-E `ORDER BY md5(id‖daySalt)` →
  实现 `rotatedEntityOrder` sha1(id‖UTC 日盐) 纯 Go 排序）：候选扫描复用
  `EntityRepo.ListEntities` 既有窄口，零碰 internal/data 红线；语义等价
  （同日序确定、跨日换新、入参不可变），测试 `EntityMergeRotation` 三点钉。
- **ANN 候选 limit 9 替代设计 8**（过取 +1 补偿自身卡过滤——自身 top-1 必命中，
  `RefID==id` 滤）；norm 组 leader-fanout 全库参与、ANN 腿仅日轮转抽样 ≤32。
- **预算后扣为「裁决成行成败同价」**：LLM 否决（no）也消耗一格预算（防拒答
  刷免），`filled++` 置 `Adj.` 成功返回之后；连败 3 熔断是本库本轮粒度。
- **dedup 签名含 verdict**：auto（≥0.90）与 yes（灰区）同对签名分离——sim
  跨档漂移自动变签可重提；三态全含（rejected→沉默），`HasProposal` 精确键。
- **提案函数计数单账**（评审 EM-D3）：`rep.EntityMergeProposals` 由主循环以
  任务返回值为唯一事实源，落笔函数只递增 `ProposalsPending`（绝不覆盖）。
  `recordEntityMergeProposal` 收束为 void 无 DryRun 分支——入口门槛已担保
  DryRun 静默返回（评审 EM-D1）。
- **nil 语义不对称钉死**（评审 cand-1）：embedder nil=整任务静默跳过（未
  接线口径）≠ ann nil=仅关 ANN 腿（运行时失败才按腿降级）；
  `SetEntityMergeCandidateRepos` 注释已按此口径重写。
- **行数不齐降级带证据**（评审 cand-2）：批量 embed 行数 mismatch（err==nil）
  Warn 增 `expected/got` 字段（`Err(nil)=zap.Skip` 不再吞现场）。
- **emPayloadID 类型容忍** int64/int/float64/string >0；float64 小数截取系
  脏载荷形态按 no-op 幂等覆盖（评审 split-vote 驳回，见
  `test/g37-review/closure-notes.md`）。
- wire 三镜像：`knowledgeEntityMergeWiring` anchor（curate worker 依赖位）；
  ANN 断言失败仅降级 embedder+adjudicator 仍接线（norm 腿可用），不整体 nil。

---

## G-39 多维度向量共存（2026-10-21，设计定稿 v1）

> 来源：SOTA gap 分析 G-39（`docs/reports/2026-10-19-research-knowledge-base-sota-gap-analysis.md` §四）；
> Month 2+ 定序 G-39→G-32→G-38→G-40 第一棒（与 G-32 共享 `CommitIndexedDocument`/`insertChunksTx`/
> `knowledge_schema.go` 承重面，schema 先行）。用户拍板默认方向：侧表形态 / Matryoshka 仅 spike /
> G-40 DELETE 只到 edge/fact 关闭层 / 前端配套独立收尾轮。

### ADR-KN-20261021-G：`chunk_embeddings` 侧表（undecorated vector）+ 按 dim partial HNSW——换模型不再清库

- **背景**：`knowledge_chunks.embedding` 列类型在首建时固化为 `vector(N)`（typmod），换模型后
  `reconcileEmbeddingDim`（`knowledge_schema.go:356-419`，08-10「expected N dimensions」事故根修）
  走「全列置 NULL + content_hash 清空 + ALTER typmod + 重建 HNSW」核清路径——vault 文档靠 sync
  自愈、UI 上传文档靠 reembed 修复追，窗口期 dense 整库归零且 UI 文档一度需人工重传。
- **决策**：向量迁出主表落独立侧表；`embedding` 列**不装饰维度**（`vector` 裸型，异 dim 同列合法
  存储）；每个 dim 一条 partial HNSW（`WHERE dim = N`）；查询恒按 embedder 当前 dim 过滤
  （`ce.dim = $dim`），异 dim 永不互比；`reconcileEmbeddingDim` 原子退役。
- **后果**：08-10 事故类根因永久消除（无 typmod 固化点）；换模型=切 embedder 配置，存量向量原地
  留存可瞬时回滚（回切配置即恢复旧 dim 召回）；代价是异 dim 同列的索引纪律（partial per-dim 纯
  增量注册）与 `collections.dim` 语义弱化为账本；旧主表列 soak 一版本后 DROP（TK-G39-01）。
- **替代方案（被否）**：按 dim 分列（新模型注册=一次 ALTER+索引重建 DDL 发布面，运营剧本退化回
  原事故形态）；LIST 分区子表（同 DDL 成本且查询路由复杂度外溢到每条检索 SQL）；JSONB float
  数组+表达式索引（失去 pgvector 原生 casts/ops，体积膨胀 ≈3×）。Spike 挂账：多 embedder
  注册表跨 dim 并行检索（TK-G39-03）、Matryoshka 统一维度查询侧裁剪（TK-G39-04）。

### 设计要点

1. **表形（迁移 20261290 + Ensure 兜底 fresh 库）**：

   ```sql
   CREATE TABLE IF NOT EXISTS chunk_embeddings (
     chunk_id      TEXT NOT NULL REFERENCES knowledge_chunks(id) ON DELETE CASCADE,
     dim           INT  NOT NULL CHECK (dim > 0),
     collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
     doc_id        TEXT NOT NULL,
     model         TEXT NOT NULL DEFAULT '',
     embedding     vector NOT NULL,              -- 不装饰维度：异 dim 同列合法
     created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
     PRIMARY KEY (chunk_id, dim)
   );
   CREATE INDEX IF NOT EXISTS chunk_embeddings_collection_dim_idx
     ON chunk_embeddings (collection_id, dim);
   CREATE INDEX IF NOT EXISTS chunk_embeddings_doc_idx ON chunk_embeddings (doc_id);
   -- 每 dim 一条 partial HNSW（distinct dim ∪ 当前 embedder dim，幂等注册）。
   -- 索引必须建在「cast 表达式」上，不能直接建裸型列（pgvector HNSW opclass 要求
   -- 列带维度 typmod，裸型 vector 报 22023 `column does not have dimensions`——
   -- 官方 halfvec/量化索引同款模式）；partial WHERE 把索引切片限定在单维桶，
   -- 桶内 cast 恒成功：
   CREATE INDEX IF NOT EXISTS chunk_embeddings_hnsw_d<N> ON chunk_embeddings
     USING hnsw ((embedding::vector(<N>)) vector_cosine_ops) WITH (m=16, ef_construction=64)
     WHERE dim = <N>;
   ```

   - `doc_id` 冗余列：`UpdateChunkEmbeddings` 归属守卫与文档级清扫免 join
     （`knowledge_embed.go:59-83` 现语义）。
   - backfill：`INSERT ... SELECT c.id, kc.dim, c.collection_id, c.doc_id, kc.embedding_model,
     c.embedding FROM knowledge_chunks c JOIN knowledge_collections kc ON kc.id = c.collection_id
     WHERE c.embedding IS NOT NULL ON CONFLICT (chunk_id, dim) DO NOTHING`（DB-N6 幂等重跑安全）。
   - 索引注册器 `ensureChunkEmbeddingDimIndexes(ctx, db, dim)`：`SELECT DISTINCT dim FROM
     chunk_embeddings` ∪ {embedder.Dim()} 逐条 `CREATE INDEX IF NOT EXISTS`——纯增量，永不 DROP
     异 dim 索引（旧 dim 退役由 TK-G39-02 与 TK-G39-01 同窗处置，禁运行时删索引引发写阻塞）。
2. **`collections.dim` 语义降级为「最近写入账本 dim」**：Ensure 检测到 embedder.Dim() 变化时仅
   UPDATE 账本 + Warn，**不作废任何向量**；写路径不再按集合查 dim
   （`knowledge_chunks.go:111-129` 现实现有一次 collections RTT——新语义免查库）。
3. **assert 新语义 = 批内等长（零端口签名改动）**：`assertChunkDimensions` 改为「同一提交内全部
   child 块 `len(embedding)` 全等且 >0」。向量由调用方 embedder 产出，批内必然同 dim；错 dim 只能
   来自混入异源批次（编码错误），DB 侧 PK `(chunk_id, dim)` 兜底。生命周期回归钉
   （`knowledge_lifecycle_regression_test.go:17`）改写为「批内 2 维+3 维混入 → 拒」。
   `CommitIndexedDocument` 4 调用方（`service/knowledge.go:790`/`knowledge_reembed.go:312`/
   `knowledge_promote.go:202`、`vault_sync.go:265`）签名不动——dim 取自 `len(embedding)`。
4. **读取路径**：dense SQL（`knowledge_search.go` denseANN 内层 ANN 排序
   与外层 recency 包装、min_score 阈值子句）主表自含向量改为
   `JOIN chunk_embeddings ce ON ce.chunk_id = c.id AND ce.dim = <N>`，相似度算子改指
   `ce.embedding`。**查询 dim 恒 = embedder 当前 dim**（单 embedder 现状）→ 全库广播
   （`federated_retriever.go:256-311`）无需按 dim 分组；Understand 轮风险「异 dim 22P02 被
   firstErr 吞」在本裁剪下不成立。TK-G39-03 注册表落地时必须补回「按 dim 分组 embed + 逐 dim
   查询」并复活该风险的测试钉。
   **落地两处实现级偏差（记档，不改语义）**：
   ① dim 以 Go 侧 `len(queryEmbedding)` 的 int 字面值 `Sprintf` 内联进 JOIN/卡片 WHERE，
   不做 SQL `vector_dims($1::vector)`（注入安全：纯 int；且 partial 谓词在 generic plan 下
   可证，方能稳定命中 per-dim 索引）；② 内层排序必须用与索引同形的 cast 表达式
   `ORDER BY ce.embedding::vector(<N>) <=> $1::vector`（索引建在表达式上，裸列排序不命中）；
   MinScore 阈值与外层相似度表达式保留裸 `<=>`——比较只发生在 `ce.dim=<N>` 桶内，
   跨维度报错（22P02）结构性不可能。
5. **写路径**：
   - `insertChunksTx`：主表 embedding 恒写 NULL（列留存 soak），child 且有向量 → side 行 upsert
     （`ON CONFLICT (chunk_id, dim) DO UPDATE SET embedding=EXCLUDED.embedding, model=EXCLUDED.model`）；
     parent 行恒无 side 行（G-06 三钉语义不变）。
   - `UpdateChunkEmbeddings`（重嵌修复回填）：纯侧表 upsert，主表列不动（soak 期恒 NULL）——
     `INSERT INTO chunk_embeddings ... SELECT c.id, $dim, c.collection_id, c.doc_id, $model, $vec
     FROM knowledge_chunks c WHERE c.id = $chunk AND c.doc_id = $doc
     ON CONFLICT (chunk_id,dim) DO UPDATE ...`（doc 归属守卫经 WHERE 保持，0 行静默同现语义；
     model 经 doc JOIN collections 台账一次事务内预查）。
   - `ListDocumentsPendingReembed`（`knowledge_embed.go:91-`）：待嵌口径补「缺当前 dim side 行」——
     `NOT EXISTS (SELECT 1 FROM chunk_embeddings ce WHERE ce.chunk_id = c.id AND ce.dim = $dim)`；
     端口签名 +dim 入参（biz 经 `embedder.Dim()` 传；接口标注 Evolving，AS-STA-01）。
   - `knowledge_move.go` 跨库搬移：side 行 `UPDATE chunk_embeddings SET collection_id=$2
     WHERE doc_id=$1` 与主表同事务。`DeleteChunksByDocument`：FK CASCADE 零代码清 side 行。
6. **reconcileEmbeddingDim 原子退役**：`EnsureKnowledgeSchema` 停止调用（`knowledge_schema.go:353`），
   函数体+卡片 typmod 双轨对账（:385-419）删除，替换为 `ensureChunkEmbeddingDimIndexes`。退役后
   系统内不再存在任何「换 dim=向量作废」代码路径。proto 注释残留（`knowledge.proto:510/728`
   提及 reconcile）属 proto 红线豁免，记档不改。
7. **卡片表同轮（ADR-KN-20261020-H 挂账兑现）**：`knowledge_graph_embeddings`
   `ADD COLUMN IF NOT EXISTS dim INT NOT NULL DEFAULT 0` + backfill（行级
   `dim = vector_dims(embedding)` 真值回填）+ `ALTER COLUMN embedding TYPE vector`（裸型；
   卡片行基数小，typmod 变更可接受）+ DROP 旧 `knowledge_graph_embeddings_embedding_hnsw` →
   per-dim partial HNSW（同注册器，同 cast 表达式形态——见要点 1，pgvector 裸型列建 opclass
   报 22023）。`UpsertGraphCards`（`knowledge_graph_projection.go`）落 `dim=len(vec)`；
   第三腿查询 `AND dim = <N>`（Go int 字面值内联，同要点 4 偏差①）+ `ORDER BY
   t.embedding::vector(<N>) <=> $3::vector`（cast 表达式，同偏差②）。
8. **换模型运营剧本（新）**：Update 新 embedder → collections.dim 账本跟新（向量/文档状态全不动）→
   过渡期 dense 腿新 dim 覆盖率低，库级自然降级 BM25 双路（F5 先例，V12.14 §检索决策）→
   `ListDocumentsPendingReembed` 按新 dim 增量追平（UI 上传文档免人工重传——旧核清路径最大痛点
   消项）→ 追平后召回满血；回滚=配置回切旧 embedder（旧 dim 侧行原地，瞬时恢复）。
9. **范围裁剪（显式挂账）**：多 embedder 注册表+跨 dim 并行检索=TK-G39-03（spike 结论回本 ADR
   补录再决）；Matryoshka 查询侧裁剪=TK-G39-04；旧主表列 DROP=TK-G39-01（soak ≥1 版本，拟与
   G-32 迁移同窗）；旧 dim 侧行空间清理=TK-G39-02；前端「dim 徽章/追平进度」=独立前端收尾轮。

### 测试策略

- **PG data 层**（`ARANEA_TEST_PG_DSN`）：① 双 dim 共存钉——同集合两批 dim 写入，按各 dim 检索
  各自命中互不污染；② 主表 embedding 恒 NULL + side 行齐全（insertChunksTx 契约钉）；③ assert
  批内混入异长向量拒绝（生命周期 :17 反例新语义）；④ 待重嵌口径按 dim 区分（dim A 已嵌/缺 dim B
  命中待嵌）；⑤ 删文档级联清 side 行、move 后 side 行 collection_id 跟新；⑥ 卡片表 dim backfill
  与第三腿按 dim 命中；⑦ 迁移 20261290 双跑幂等 + backfill 行数等值断言。
- **biz 层**：`CommitIndexedDocument` 批内一致性 assert（`knowledge_more_test:104-161` 不回归）；
  dim 取自 `len(embedding)` 的零签名改动编译钉。
- **回归门**：G-06 父子块三钉、`knowledge_team_note_test`、8 项存量失败基线对账
  （curate×3/parent_child 夹具×2/MCP 夹具/MigrationRegistryCovers/EntRuntimeToBizFieldCoverage）；
  `go build ./...` + archlint；wire 零改动（zero-wire-touch 达成，wire_gen 手工并合面无新债）。

### 预计改动文件

`internal/data/knowledge_schema.go`（退役+索引注册器）、`internal/data/knowledge_chunks.go`
（insert/assert）、`internal/data/knowledge_embed.go`（upsert+待嵌口径）、
`internal/data/knowledge_search.go`（dense JOIN）、`internal/data/knowledge_move.go`、
`internal/data/knowledge_graph_projection.go`（卡片 dim）、
`internal/data/ddl_migration_table_late.go` + `internal/data/sql/migrations/20261290_chunk_embeddings.sql`、
`internal/data/knowledge_lifecycle_regression_test.go`（:17 语义改写）+ 新 PG 测试；
`internal/biz/knowledge/knowledge.go`（待嵌端口传 dim）。

### ADR 落地纪要（2026-10-22，T1–T7 实施 + 对抗评审复核）

> T7 对抗评审（4 维 finder 舰队 + 第 5 维 inline）产出 17 项发现：3 blocker 中 2 项代码侧已修、
> 1 项接受+缓解入档（F1-B1）；9 major/minor 已修；6 项存量/并行会话归属记档。以下为设计补正，
> 凡与上文 v1 冲突处以本节为准。

1. **dim 真相源契约（F4-B1，blocker 已修）**：OpenAI-compat/Ollama provider 不回传 `dimensions`，
   配置 dim 与原生输出可分叉——pending 判定必须以「新写入真实维度」为口径，否则缺维腿永不命中、
   全集合无限重嵌。落地：`MultiProviderEmbedder.ProbeDim(ctx)`（一次真实 Embed 探测，
   `atomic.Int32` 缓存，`Update()` 清零）；service 统一 `currentEmbedderDim(ctx)`——prober 接口
   断言优先，probe 失败/零值回退配置 `Dim()`，embedder nil 回 0（判定腿安全关闭）。
   ReembedDocuments / RepairPendingKnowledgeIndexes / EnableCollectionSemantic / 卡片 reconcile
   worker 四处同源。配表台账 `collections.dim` 语义不变（账本，不参与判定）。
2. **启动期索引建设改 CONCURRENTLY + 失效自愈（F1-B2）**：per-dim partial HNSW 由
   `ensureIndexConcurrent` 落地——pg_class/pg_index 探针三分支（健康 no-op / `indisvalid=false`
   DROP+重建 / 缺失创建）。CONCURRENTLY 不可入事务，Ensure 本就独立 Exec 面（安全）。代价：单
   语句失败不再整批回滚（Ensure 本就非事务）；CI/首启 DDL 耗时窗口拉长（可接受，后台索引不伤写）。
3. **卡片 dim 感知对账（F3-B1，blocker 已修）**：卡片表单行/键（kind,ref_id），多 dim 非真共存；
   原 hash-only reconcile 使 dim 漂移卡永久退出 ANN 腿。落地：`GraphCardRow.Dim` 出账、
   `ListGraphCards SELECT dim`、`PlanGraphCardReconcile` 第四参 `currentDim`——
   `hash 不变且 (currentDim<=0 或 row.Dim==currentDim)` 方视为健康；worker 经 prober 断言取
   currentDim（无探测能力/探测失败退化 hash-only，Warn 记 `knowledge.graph_projection.probe_dim`）。
4. **卡片表回滚边界 + 修复 runbook（F1-B1，接受+缓解）**：卡维度化需要 DROP 旧 typmod HNSW +
   `ALTER COLUMN embedding TYPE vector`（裸型）。**旧二进制回滚后启动 Ensure 会在裸型列建
   typmod HNSW → 22023 拒绝，readiness 失败（响亮失败，与 08-10 静默归零相反）**。接受因由：
   卡片为 entities/links/doc_entities 的派生廉价数据，worker 全量 reconcile 可完整再生；chunks
   主表 soak 期不受影响（旧代码主路径无碍）。**回滚修复 runbook**（回切旧二进制前执行）：
   ① `DROP INDEX IF EXISTS knowledge_graph_embeddings_hnsw_d*`（全部 per-dim partial）；
   ② 跨 dim 脏行先清 `UPDATE knowledge_graph_embeddings SET embedding=NULL WHERE dim <> <D>`
   （D=回滚目标 embedder dim；或全列置 NULL 交由 worker 重嵌）；
   ③ `ALTER TABLE knowledge_graph_embeddings ALTER COLUMN embedding TYPE vector(<D>) USING embedding::vector(<D>)`；
   ④ `ALTER TABLE knowledge_graph_embeddings DROP COLUMN IF EXISTS dim`（可选，旧 SELECT 不含 dim
   本无碍，DROP 更干净）；⑤ 启动旧二进制，Ensure 重建 typmod HNSW，projection worker 下轮全量
   重嵌回补。
5. **读路径脏行硬化（F3-B2/F3-B3）**：dense 内层补 `AND ce.embedding IS NOT NULL`——侧表无
   NOT NULL 约束（裸型列可存 NULL），一行脏 NULL 会使整桶 ANN 腿 NULL-scan 静默归零（卡侧
   对称防线本就有）；`UpsertGraphCards` 竞争防线改条件赋值——`dim/embedding` 仅
   `EXCLUDED.dim>0 / EXCLUDED.embedding IS NOT NULL` 时覆盖，空向量 upsert 只推进 text/hash，
   不抹健康向量（缺维由卡侧 dim 口径召回重嵌）。
6. **写路径竞态与归属（F2-B3/B4/B5/B6）**：`UpdateChunkEmbeddings` 预查改
   `SELECT ... FOR UPDATE OF c`，与 MoveDocument 的 chunks UPDATE 串行化（防旧集合行
   写入新集合侧行）；`insertChunksTx` 侧写闸 `len>0 && !IsParent`（父块带向量属带外构造，
   同口径拦截防 per-dim 索引污染）；侧行 model 改批内 per-collection 懒查表（不再 chunks[0]
   单点溯源）；model 台账查库失败静默空串改 Warn（`knowledge.chunk_side_model_skip`）。
7. **backfill 孤儿哨兵**：存量 chunks 悬空 `collection_id`（FK 外历史数据）在
   `INSERT...SELECT ... JOIN knowledge_collections` 下静默不进侧表——Ensure 后按
   `NOT EXISTS(JOIN collections)` 计数 Warn（`data.schema.knowledge_backfill_orphan`），
   只诊断不阻断。
8. **记档归属（不改本轮代码）**：F2-B1/B2（UpdateChunkEmbeddings 陈旧覆写收敛性、MoveDocument
   陈旧计数）存量竞态窗口，重嵌幂等可收敛；F4-B2（fail()→Conflict 卡死 indexing）、F4-B4
   （pending 查询 `OR status='pending'` 腿）、F4-B5（Enable 台账快照漂移）为并行会话 hunk/存量
   语义，转各归属轮处置；proto 残留注释属红线豁免不改。并行会话在
   `service/knowledge_reembed.go` 新增的 `RebuildCurrentBlockIndex` 调用与存量断言
   「重嵌入禁止触发 RebuildBlockIndex」（knowledge_reembed_test.go:282）冲突——该测试当前红，
   归属该企业并会话，本轮不代办。

---

## G-32 内容版本链+增量重建（2026-10-22，设计定稿 v1）

> **来源**：SOTA gap 分析 G-32（report :107「document_versions 表 + 新旧 chunk diff 只重建
> 变更块」）；Month 2+ 定序第二棒（G-39→G-32→G-38→G-40）。设计流程：理解轮×2（9 agent
> 事实侦察，`test/g32-design/factpack.md` 归档）→ 设计轮（3 独立设计 mvp/full/delivery ×
> 契约/风险/交付 3 评审视角，12 agent，全票 adopt-with-grafts）→ inline 合成；评审
> killer flaws 的处置逐条编入决策条款。质量门=TDD 后对抗评审硬门。
> **共享承重面**：G-39 已入库契约（ADR-KN-20261021-G）不可侵；b8 在飞面（CAS hunk、
> ReplaceDocumentPath、RebuildCurrentBlockIndex、治理 Tx）只读，交付序=案 B 并行错峰+
> 末段吸收（决策 11/TK-G32-02）。

### ADR-KN-20261022-I：主表恒 current + 历史侧表化 + 单事务 diff-commit

**拍板记录（2026-10-22，用户）**：①cited_count=化身语义（对 R2「命名位置累计」的
逆转，确认）；②content_key 列 P0 直落（确认）；③b8 交接=案 B 错峰+末段吸收（确认）；
④as-of=**本轮交付 chunk 全量时态检索**（推翻初稿窄口径，TK-G32-06 取消，scope 依此
扩，新增决策 12——向量处置随之反转，初稿「向量不归档」作废）。

**背景**：`CommitIndexedDocument`（data/knowledge_chunks.go:32-97）恒「DELETE all + 重插」，
任何微改整篇重建 chunks 与向量；chunk ID 位置派生（`<docID>-ch-<idx>`，ingest.go:170-176），
净块数变化即全量漂移；citation 账本（knowledge_chunk_citations 无 FK）随重嵌悬空+位次 ID
复用错绑（旧 turn 的 k-gram 拿新正文判定），cited_count 随旧行蒸发且无回补
（knowledge_citation.go:425-428 黑洞为存量债）；G-05-asof 降噪边界（:3185 上一段）已把
chunk/正文历史显式划给本轮裁决。

**决策**（评审 killer flaws 逐条并入骨骼）：

1. **架构形态=archive 侧表，不开同表 gating**。新增 `chunk_archive`（DELETE 同事务物化
   归档，无 FK）与 `document_versions`（轻版本账本）账外表；`knowledge_chunks` 恒
   current 唯一事实源，current 读面 SQL 逐字节不动（G-05 条款①），「见历史」走独立
   窄端口独立 SQL（条款②），治理读显式 current-only 并于本文显式声明：
   ListStaleEntries/decay/治理回放 current-only；chunk 级 ANN/召回的历史态=决策 12
   全量时态端口（2026-10-22 拍板由窄口径升级为全量，取代原「另轮」）。
2. **稳定键=content_key 列 P0 直落**（合成 delivery/full）：`knowledge_chunks` 新增
   `content_key TEXT NOT NULL DEFAULT ''` + `(doc_id, content_key)` 索引；口径登记=
   HashContent（'sha1:'+hex 裸字节，vault_filer.go:684-688；**不是** blockparse 的
   sha256 bare、**不是** relation-extract 的裸 sha1——迁移注释+本章双登记）。配对权威
   永远在 Go 侧按现载 content 现算（multiset 按 chunk_index 升序贪心，重复内容块确定性
   规则；键含 `chunk_role`；子块保留当且仅当其父也配对——父死子随）；列只服务 SQL
   寻址/审计与幂等回填，`''` 行按现算键等价处理，回填未完成不构成配对阻塞。否决
   (heading_path, ordinal) 复合键：无标题/纯文本退化、改标题全灭、图片 LLM 重述双死。
3. **版本账本字段与写入时机**：`document_versions(doc_id, version_no, content_hash,
   asset_hash, change_kind TEXT, chunk_stats JSONB, producer TEXT, created_at,
   PK(doc_id,version_no))`；change_kind 值空间 `seed|create|edit|rename|supersede|tombstone`
   一次入列，producer 渐进到齐（seed/create/edit 本轮，rename=b8 TK-G32-02，
   supersede=G-40，tombstone=DeleteDocument 本轮）。version_no=同 tx
   `COALESCE(MAX(version_no),0)+1`（doc 行 FOR UPDATE 已串行化）。**写入双条件**：head
   行缺失 OR head.content_hash≠tx 现载正文 HashContent OR chunk 多重集变更
   （added>0∧deleted>0）——head.content_hash=''（种子未知档）视为相等防一次性伪版本；
   同 body 重嵌/幂等重放不产行。**不设 documents.version_no 守卫列、守卫行不加
   content_hash/version_no 谓词**（评审指正：资产类 content_hash=资产字节与正文口径
   混装会楔死谓词；A→B→A 已由「diff 基线=tx 现载行集」天然收敛）。`asset_hash` 血缘
   列承接图片/编译类（当前=documents.content_hash 的资产字节口径）。**不设
   FK/FOV**——tombstone 可回放要求账行不随 documents 行物理删除级联抹除（化身区间
   闭端全失）；孤儿台账清理归 TK-G32-03 GC（与 chunk_archive 无 FK 同源）。
4. **图片/编译非确定性源=第一次即冻结**：首个抽取 md 落库即 canonical，链头=首版正文
   hash，资产字节 hash 入 `asset_hash` 血缘；后台恒重抽（service/knowledge.go:743-772）
   其产物经 diff-commit 管道 CAS 概率性升格；relation-extract 幂等闸锚资产字节
   （relation_extract.go:132-146）语义不动；不建抽取产物缓存表（成本超收益）；跳过
   版本链则 citation 历史失声=否决。
5. **diff-commit 单事务形态**（改造 data `CommitIndexedDocument`，骨架/b8 CAS hunk
   不动）：①doc 行 FOR UPDATE（既有）→②快短路：**added=0∧deleted=0∧body hash=head
   时零 chunk 写/零版本行，但守卫 UPDATE 仍执行**（评审指正：跳过则 doc 滞留
   indexing、终态帧不发、前端 240s 假超时回归；zero-diff 钉范围仅 chunks 行集不变，
   documents.updated_at 变化属既有 churn 不入钉）→③现载旧行（id/content/chunk_index/
   parent_chunk_id/chunk_role/cited_count/metadata）→④`PairChunksForDiff`（biz 新纯
   函数）→⑤**归档先于删除**：孤儿（含父死孤子）INSERT…SELECT 物化 chunk_archive
   **向量先镜档**：孤儿 side 行 INSERT…SELECT 入 chunk_embeddings_archive（决策 12②）
   再按 id 删孤儿（主侧行 FK CASCADE 回收，20261290:11）→⑦保留行 **compare-and-skip
   UPDATE**（仅 chunk_index/parent_chunk_id/metadata 实变才写；保留子行父指针重指到
   新父 ID）→⑧added 子集走 `insertChunksTx` 原函零改动（新行 ID 代际命名空间
   `<docID>-v<version_no>-ch-<idx>`/`-p-<idx>`，旧位置派生格式仅役存量 legacy 行；位次
   ID 不再复用→citation 错绑类 bug 结构性灭绝）+ **缺 dim 补齐条件 upsert**：kept 行
   带新向量且 `NOT EXISTS (chunk_id,dim)` 时侧表 upsert（评审 killer：G-39 换 dim 主
   通道 reembedOneDocument 同 content 重嵌→added=∅，无此步则新向量静默丢弃、pending
   永不收敛为死循环）→⑨版本行（双条件）+chunk_stats{added,deleted,kept,strategy,
   model,dim}→⑩守卫 UPDATE（`id + status='indexing'` 谓词逐字不动）→⑪计数 oldCount
   现算/delta GREATEST 原口径→⑫ syncMeta 原样→⑬ `DiffStats` 透出（repo 新增方法返回
   (DiffStats, error)，AS-STA-01 Evolving 标注；旧签名单薄包装留一版）。**归档/删除
   全部排在 b8 CAS hunk 之后**，b8 验收钉「陈旧 commit Conflict 先于删块、旧块保活到
   新块提交」由构造保持。
6. **embed 经济化=核心收益兑现点**（评审 killer：tx 内 diff 不省 embed 计算——四条
   调用链 BuildIndexedChunks 皆 commit 前全量算向量）：新增 repo 窄端口
   `ListChunksForPairing`（Evolving）+ 服务层/摄取层预配对，仅对非配对（新增/变质）
   chunk 发 embed；配对失败最坏后果=该 chunk 进 pending-reembed 口径自愈（经济 hint
   与 tx 权威配对允许不一致）。四调用方（ingest :791、reembed :344-348、promote :202、
   vault :265-274）同窗改造。
7. **寻址切换与 diff-commit 同窗强制**（评审 killer：namespace 先落、retry 按位置
   派生 ID 静默 no-op+熔断假性复位窗）：`retryDocEmbeddings`
   （internal/knowledge/vault_sync_part2.go:44-79——评审更正锚点：该函在 internal/
   knowledge 包，非 internal/data）切到「现载行→同一配对函数→解析存活 id」再调
   `UpdateChunkEmbeddings`（data knowledge_embed.go:64-106 函数体与归属守卫零改动）；
   **namespace 变更×寻址切换必须同 commit，禁拆阶段**。
8. **并发纪律**：串行原语=doc 行 FOR UPDATE 唯一（**不外加 advisory 锁**——锁空间
   收敛挂 TK-G32-01）；ctx 戳沿用 b8 DocumentContentVersion 体系（其落线后 T4 为
   vault upsertDoc 补戳+Conflict→重读 latest body→重算 diff→重试≤3 收敛环，
   RebuildCurrentBlockIndex 对型）；治理 DeferReplay 不增新机制（回放 commit 走同一
   diff 管道、producer='governance'）；fail 分支不得覆盖胜者 indexed 终态（既有
   document_state_machine.go:51-62 坑位声明纪律+钉，不冒充修复）；
   UpdateChunkEmbeddings 的 chunk 行 `FOR UPDATE OF c` 与 diff-commit 锁序
   =doc→chunk 单向无环记档。
9. **WS 帧面**：publishKnowledgeIngest 终态帧（indexed/error，service/knowledge.go
   :1206-1222 meta）逐字节不动；新增 `event_type=knowledge_chunks_diff`（doc_id/
   added/deleted/kept/chunk_count/producer）经 SystemNoticeEvent meta 自由结构免
   proto 红线，与 knowledge_ingest 并存不冒充；promote/replay 通道（当前 :162-224
   原静默）自此有帧；repo 不支持 DiffStats 时零发布；帧契约钉用键集+逐值相等
   （评审指正：map 键序断言天然 flaky）；回放支线终态帧衔接/at-least-once 语义
   在 T5 钉齐。
10. **迁移 20261294 单迁移**（初稿占用 20261292，2026-10-22 当日被并行会话 C3 连占
    20261292/20261293——撞号警报后整体顺延；canonical 铁律：SQL 文件名、注册行、
    硬编码引用同改同 commit）：`20261294_document_versions_chunk_archive.sql`（三表——
    document_versions/chunk_archive/chunk_embeddings_archive+种子+chunks.content_key 列+
    (doc_id,content_key) 索引+化身区间 VIEW）+ ddl_migration_table_late.go 注册行 +
    Ensure 双轨同 commit（canonical 命名铁律）；种子=迁移内一次性纯 SQL
    （content_hash='' 未知档+asset_hash=doc.content_hash+chunk_stats={chunk_count}，
    `ON CONFLICT DO NOTHING`），**Ensure 不挂种子循环**（评审指正：启动全表扫幂等
    空转不可接受）；Go 启动回填 runner 幂等分批跑 content_key 与文本 doc 版本行
    body-hash 富化（sha1 进不了 SQL=禁引 pgcrypto，PG 无先例）。并行面纪律：A11
    （20261291）与 C3（20261292/93）均已落线，提交前先 merge 当前 dev 吸收占用
    （禁 rebase/force），以当时最大值顺延复核；带外 export 提交态验证收口（memory
    铁律）。**TK-G39-01 DROP 顺延 ≥20261295**（soak≥1 版本未满；且评审发现 Ensure
    清理枚举除 knowledge_schema.go:125/:131/:428-437 三处外还有第 4 处
    ddl_migration_hooks_index.go:466 的 20261272 钩子调用——四处必须同窗同 commit）。

11. **b8 交接序=案 B 错峰+末段吸收（拍板 2026-10-22）**：T1–T3/T3b 与 b8 在飞面
    零~极小重叠先行（迁移、配对引擎、archive 写入器、citation 回落、时态端口）；T4
    diff-commit 主刀必须叠在 b8 的 CAS hunk（knowledge_chunks.go:41-50、biz :700-702）
    之上——待 b8 commit 后 merge（禁 rebase/force）吸收基线再动刀；b8 三锚点归档
    补锚挂 TK-G32-02；全程多会话纪律（只 add 显式文件名、A11 注册行 hunk 级排除）。
12. **全量时态检索本轮交付（拍板 2026-10-22，推翻初稿窄口径，TK-G32-06 取消）**：
    ① 化身区间由 document_versions 相邻版本 created_at 推导（版本 i 区间=
    [v_i.created_at, v_{i+1}.created_at)；current 化身 valid_to=+∞），不落区间冗余列，
    迁移内建派生视图 `knowledge_chunk_incarnations`（namespace 携带出生版本号；
    G-32 前存量的位置 ID 行出生归 seed 版本档）；
    ② **向量随归档镜档**：新增 `chunk_embeddings_archive`（(chunk_id, dim, model,
    embedding, archived_at)，与 chunk_embeddings 同型）承载死化身向量——diff-archive
    路径在删孤儿前 INSERT…SELECT 镜档；namespace 下死化身行永不 JOIN current 读面
    （无同名活 chunk_id），条款①零横切保持；G-39 主侧表契约（FK CASCADE、恒
    NULL、upsert 口径）逐字不动；
    ③ 时态读=新窄端口（条款②对型）：`RecallChunksAsOf`（dense=当前侧行腿∪档案腿
    各 ANN+化身区间过滤后 top-k 合并；sparse 对型）与 `ListChunksAsOf`，复用
    `SearchQuery.Temporal`（G-05 已通边扩展腿）同一 span，零值=current 行为逐字节
    保持；DeleteDocument tombstone 后文本档案与向量档案保留（授权用当下、内容用
    历史），物理硬清属 GC；
    ④ 档案体积/HNSW 膨胀并入 TK-G32-03 GC 预算（区间闭合化身整批可回收；
    chunk_embeddings_archive 按 dim partial HNSW 镜像 G-39 索引形态）；
    ⑤ 块/refs/卡片腿仍 current-only（65 护栏与 G-33 边界不动）。

**对抗评审落地收紧**（2026-10-22，wf_e2eac60f-cc5 triage 修复批；19 confirmed 处置
全录 `test/g32-review-triage/findings.md`，设计条款不变处仅录实现口径）：
决策 2——配对现载投影按决策 5③ 全列实施（`cited_count/metadata` 随行，DIS-2 修正
投影缺列防止 T4 compare-and-skip 契约断裂）；`content_key` 计算直调
`bizknowledge.HashContent`（CON-4，禁公式双登记漂移）。决策 3——零账本 doc 直落
tombstone 的区间坍缩护栏：`tombstoneDocumentTx` ver==1 时先以 doc.created_at mint
seed 版本行（content_hash='' 未知档+asset_hash=doc.content_hash，镜像迁移种子，
producer='g32-tombstone-mint'），tombstone 退至 v2（DIS-5）。决策 10——迁移内
`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS content_hash` 预种子块
（CON-9：42703 静默容忍+applied 记账=老库种子永久断供类）；版本行 body-hash 回填
改批量 tx 内逐项 `SELECT 1 ... FOR SHARE` 核身锁行+满批零落盘 3 连熔断（DIS-3：
`UPDATE...FROM` 普通连接行无 row mark，READ-COMMITTED EPQ 不重取——谓词守护须
显式 FOR 锁）；`ensureIndexConcurrent` 并发 42P07 竞态复查-重试容忍（CON-10）；
`knowledge_chunks(content_key='')` partial 回填加速索引（CON-15）。决策 12②——
镜档 upsert 口径 latest-wins（`ON CONFLICT (chunk_id,dim) DO UPDATE`，回滚窗 id
复活二次死亡防御，CON-12）。决策 12①——视图归档腿
`valid_to=GREATEST(死亡版时点, 出生时点)` clamp 防时钟回拨负区间（CON-13，迁移
SQL 与 Ensure 双轨逐字符一致）；同 tick [t,t) 坍缩根治挂 T4 写入侧 created_at
clamp（`documentVersionRow.CreatedAt` 通道已铺）。决策 12③——dense 重构为
**per-leg KNN**（双腿各自存活谓词裁剪后 `ORDER BY embedding::vector(<D>) <=>`
+LIMIT，dim 以 Go 字面量内联保持 per-dim partial HNSW 可达，再 DISTINCT ON 合并，
CON-8/14）；sparse 对型落地=四分支词法内核（tsvector/trigram/substring/CJK
bigram）在化身视图存活谓词下各跑 top-k，分支原生得分直出不套 recency，RRF 融合
复用服务层 `mergeBM25Rankings` 同轨（CON-2，端口增 `SparseRecallChunksAsOf`）。

**cited_count 语义裁决（对 R2 推荐的显式逆转，记档）**：P0=**化身语义**——
archive.cited_count 快照列+新化身从 0 起算；累计口径 Σ(archive)+current 挂 P2
（TK-G32-04）。理由：跨化身写入改数违反最小写面（RecordChunkCitations :410-433
零改动、四钉 knowledge_citation_test.go:82-122 零改写）；R2 的「命名位置累计」是
读面聚合叙事，不等于写入期交接。knowledge_citation.go:425-428 UPDATE 无
RowsAffected 校验的存量黑洞不放大不冒充修复，记档 TK-G32-04。

**ChunkArchiveReader 窄端口**：loadChunkContents（knowledge_citation.go:263-290）
主表 miss→archive 回落（独立 SQL 变体，不横切 WHERE）；既钉
TestKnowledgeCitationTraceReader_DropsMissingChunks（:186-215）语义演进为「主表
miss 且 archive miss 才剔除」；卡片腿既有「evidence 快照+当前授权」混合语义
（:292-294）不回档案；archive 回落授权谓词=collection 级同口径（一句话裁决：授权
用当下、内容用历史）。revision 选择规则=按 chunk_id 精确（代际命名空间下所见 ID
恒定位唯一化身，无需 MAX/按 turn 推段）。

**被否方案**：①同表 gating（is_current/valid_to 列）——条款①违反、8 读面谓词横切、
位次 ID 复用语义冲突；②document_versions 全正文快照链——存储无界增长且 citation
要的是 chunk 级粒度（归档表承担）；③稳定键 (heading_path,ordinal)——雪崩面论证见
决策 2；④位置 ID+对齐配对（零迁移）——评审后合成仍采纳 content_key 列 P0（加速+
审计+G-40 接口面），配对算法本身两案同构；⑤advisory 锁 P0 前置——与 b8 锁空间
混排风险大于收益，FOR UPDATE 已足。

**老二进制回滚 runbook**：新表/新列对旧代码全透明（旧 INSERT 不列新增 DEFAULT 列可
运行）；回滚后旧代码恢复全删全插但不写 archive/版本行（回滚窗内 citation 历史断档
与本版之前行为等价，无回归）；升级回切=重新部署新二进制即可，无需数据修复。APP 级
force-full 旁路：配置开关 `KNOWLEDGE_DIFF_COMMIT_DISABLED=1` 时配对跳过=全量替换但
仍归档（回滚窗一个版本）。

**挂账**：TK-G32-01 advisory 锁收敛+13+ 裸奔 UpdateDocumentContent 写者/replay 族
无锁/MoveDocument 无 doc 锁/Cron curate 绕锁的锁空间收敛（P1）；TK-G32-02 b8 三锚点
（ReplaceTeamDocumentContent team_note.go:39、ReplaceDocumentPath
knowledge_documents.go:146 两站点）归档补锚——补锚前团队文档正文替换的场景回放仍
断档（边界明示，b8 落线后随 merge 吸收）；TK-G32-03 archive/向量档案 GC 与版本、化身体积增长预算（决策 12④）；
TK-G32-04 cited_count 累计读+RowsAffected 黑洞修复窗口；TK-G32-05 前端 diff chip/
版本徽章/命名空间解析审计（独立前端收尾轮，G-39 先例）；TK-G32-06 已取消——
chunk 全量时态检索随本轮交付（拍板 2026-10-22，决策 12）；TK-G32-07 G-40 仲裁主键裁决备忘（账本键 (doc_id,version_no)/
(chunk_id,revision_no) 为准，content_key 仅寻址加速——双键并存期不返工）；
TK-G39-01 顺延 ≥20261295（决策 10）。

**实施阶段**（同 commit set 同发布，无跨阶段部署窗；每阶段独立 revert 口）：T1 迁移
20261294（三表+区间 VIEW SQL+注册行+Ensure 双轨+种子+Go 回填 runner）→T2 配对引擎
biz 纯函数+夹具矩阵（四策略×编辑形态、重复块贪心、角色/父约束）→T3 chunk_archive+
chunk_embeddings_archive 写入器+ChunkArchiveReader+tombstone 组合器新文件内核（citation
回落与 DeleteDocument 锚点两处接线因外源脏面转入 T4 一并吸收——文件头接线清单四条
为合同；tombstone 版本行同事务语义不变）→T3b 时态端口（化身区间视图+
RecallChunksAsOf/ListChunksAsOf/sparse 对型，G-05 Temporal span 复用；内核先落、biz
端口声明已立，Usecase/装配接线并入 T4 清单④）→
T4 diff-commit 主刀+寻址切换+embed 经济化+vault 收敛环（b8 commit 后
基线吸收，merge 不 rebase；**namespace×寻址同窗强制**）→T5 WS 帧+缺 dim 收敛 e2e 钉+
时态 recall e2e 钉+文档三件套同步→T6=TK-G39-01（顺延，不随本发布）。65 手册 :588「块/refs 写路径一律
整文档重插」红句修订带护栏：**仅 chunk 级放开 diff，块/refs 整放纪律不变**
（ReplaceDocBlocks/KnowledgeBlock 范围外）。

**测试矩阵**（TDD 锚，含评审 grafts）：①迁移护栏 merge dev 吸收后
GloballyUnique/RegistryCoversSQLFiles+带外 export 提交态验证；②配对单测：四种分块策略×文首/
中部/尾部插入、重复内容块、父子约束、角色误配；③diff-commit PG：微改只重建邻域块
（added/deleted/kept 断言）、保留行向量零重写、缺 dim 条件 upsert 收敛 e2e
（pending→reembed→侧表补齐→出列）、同 body 重嵌零版本行、归档物化+回落、
DeleteDocument tombstone、force-full 旁路、Conflict 先于删块（b8 钉保持）；
④citation：回落语义演进钉、化身 cited_count 快照、RecordChunkCitations 四钉
零改写过；⑤帧：diff 帧键集值等+终态帧逐字不动；⑥并发钉：双 commit 竞态终态
正确、vault 撞入重试环收敛；⑦G-39/现有回归矩阵全绿；⑧前端 namespace 解析
挂账前由 citation 派生判定（parseGraphCitationID derived-only）钉兜底；⑨时态：
as-of recall 命中编辑前化身内容、current 召回零泄漏（档案向量不入 current 结果）、
区间边界（created_at 等值归入新区间）、当前+档案两腿 top-k 合并排序钉、tombstone
后 as-of 仍可回放而 current 查无。

---

```text
RebuildKnowledgeIndex
  → RebuildCollectionBlockIndex（allowBackfill=false，不改源）

POST .../autolink-backfill
  → BackfillOutgoingAutolinks（显式写路径成链）
  → RebuildCollectionBlockIndex
  （与重建共用本进程 rebuildRuns + PG advisory lock `knowledge-rebuild:{id}` + knowledge_rebuild_index 进度）

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
| POST | `/v1/knowledge/search/asof` | 集合读（留空走全库智能路由）；body = search 现字段集 + `valid_at`/`recorded_at`（G-05-asof，ADR-KN-20261020-K） |
| GET | `/v1/knowledge/vaults/{id}/graph/asof` | 集合读；query `valid_at?`/`recorded_at?`/`link_types?`/`path_prefix?`（G-05-asof，ADR-KN-20261020-K） |

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

## G-38 SourceSync 外部连接器（2026-10-23，设计 v0 待拍板）

> 锚点：SOTA :117「组织知识停在本仓自控源。CocoIndex/LlamaIndex 增量模式：Connector（walk/checkpoint）+ 指纹表 + 删除墓碑级联。先落 Notion/共享目录最简 connector。」
> 证据链：六路理解轮 wf_869b63fa-09d（factpack=`test/g38-design/factpack.md`）→ 设计轮 wf_50f854f4-66a（mvp/full/delivery 三哲学 × 契约/风险/交付三透镜，**九审全 adopt-with-grafts**，汇合 `test/g38-design/review-grafts.md`；三案全文 `test/g38-design/design-{mvp,full,delivery}.md`）。
> 锁序：G-32 → G-38 → G-40；消费 G-32 T4 后接口（13 步 diff-commit tx + tombstoneDocumentTx，内核已 commit 于 4f33e11af）。
> 状态：**v0 草案待用户拍板**（11 项拍板清单+任务切片在 37-knowledge.development.md G-38 kickoff；下文按综合推荐值落地成文）。

### ① 综合裁决（设计轮三案收敛点+分歧裁决）

三案一致收敛：connector 配置独立新表（非 cron_task.config_json——红线：checkpoint 禁放 metadata_json）＋ 单例 fan-out worker＋per-connector advisory lock（KnowledgeJobLocker key 家族 fail-closed，不引入新选举）＋ 聚合帧 `knowledge_source_sync`（禁 per-doc 帧风暴）＋ 凭据走 biz.CredentialCrypto 唯一通道＋RequireKey 级拒写闸＋recordAudit 禁记 secret＋无 Reveal 端点＋`env:VAR` 备选 scheme ＋ 更新唯走 `CommitIndexedDocumentMeta`（复刻 vault_sync.go:225-271 提交序列）＋ 删除唯走 `Usecase.DeleteDocument`（T4 后 FOR UPDATE+tombstoneDocumentTx 锚）＋ 专用 team-backend collection 绑库（写闸 fail-closed，IngestDocument/target_dir 拒入）。

分歧裁决：

- **骨架＝运维案为底**：删除链路按失效模式完备化（`ConfirmDeleted` 三态＋auth 整轮闸＋walk_empty_abort 通用闸＋`max_deletes_per_run` 爆炸半径截断＋runs 历史自带保留 100 轮）；首发 scope 按运营排期＝shared_dir 首发（SyncEngine.Scan 薄适配，G-32 T4 窗口期可安全灰度）＋ notion 第二（全库同步嵌入账单挂 T4 接线信号）；shared_dir connector 与既有 `vault_backend=local` 目录源**并存不迁移**（用户可见分工写进需求档）。
- **身份寻址＝items 表唯一真相**：connector 文档 `rel_path=''`（免疫 vault diff 与扩展名族 switch 全部误判面——前提：walker 显式提交 MIME；title 落 source 前剥末尾类扩展名片段）；`(connector_id, external_id)→document_id` 是更新/删除唯一寻址与管辖权边界；`GetDocumentByContentHash` 命中即 **adopt**（own_lifecycle=false，登记 item 但不接管生命周期、不 tombstone）——红线 3「同库同 hash 不重复建档」契约完整保留，同时管辖权互斥。mvp 案 rel_path 命名空间案的六雷区（编辑器判定/CAS/下载名/Move 等）整类消失。
- **凭据＝行表**（channel_credential 契约1 同构）：credential_key 维度+软删重建+OAuth per-user 升级不卡 shape；mvp 行内 config_json 案的 merge/refresh 落点弱点被评审点名且升级需改列。

### ② 抽象形态（签名级）

- 包落位：`internal/biz/knowledge/sourcesync/model.go`＝唯一 biz 落点（Config/Item/Run/DeletionVerdict/Kind 类型+校验）；Walker/Runner/Applier 实现落 `internal/knowledge/sourcesync/`（与 vault_sync.go 同层，BuildIndexedChunks/EmbedCircuitAllow 同包直调，消除 biz→knowledge 反向依赖边）。全部构造注入 `loggateway.Logger`（禁 Global()）。
- Walker：`Kind()`；`Walk(ctx, cfg, prev CursorState, emit func(Snapshot) error) (CursorState, error)`——emit 出错即中止本轮、游标不动（失败不存 checkpoint，对齐 vault runner 不存 prev 契约）；整轮 walk error＝当轮 abort 零删除零指纹变更。`Fetch(ctx, cfg, externalID) (*Payload, error)`——Payload 含显式 MIME 交 ExtractorRegistry 归一化。`ConfirmDeleted(ctx, cfg, externalID) DeletionVerdict`——三态 Unknown/Confirmed/StillExists；Notion 走 pages.retrieve（404/archived=Confirmed，余=Unknown）；shared_dir 文件系统 walk-absent 即真相。
- Applier：`Upsert/Delete` 薄封装 vault_sync 提交序列（幂等短路 → CreateDocument（adopt 判定在前）→ indexing CAS → BuildIndexedChunks → CommitIndexedDocumentMeta → 图谱四 hook）；fetch/远端 5xx 失败**不进** embed 熔断计数（熔断只包 embed 调用点，防串味）。
- Runner：`SyncOnce(ctx, connectorID)`——租约 → 保留集载入 → Walk+diff → 预算内 Fetch+Upsert → 删除确认级联 → run 行 finalize（含保留 DELETE）+ 游标推进 + 聚合帧。
- shared_dir walker＝SyncEngine.Scan 薄适配（IsIngestibleExt/32MB 白名单下沉实现侧，walk 与 filter 已分层）；Notion walker＝search API **页级指纹**（仅 last_edited_time，禁 walk 期逐页拉 blocks，成本模型钉）＋429 尊重 Retry-After＋cursor-invalid 回退清 cursor 全量 walk（item 表仍是删除对账唯一全集）。

### ③ 数据模型（四表，迁移占位 20261296–20261299，开工重锚）

版本重锚铁律：20261295 已被 `board_run_leases`（b91072a7c）占用；W1 开工第一步重查 ddl max Version 与 sql/migrations 目录，四表顺占当时下一空号段，SQL 文件名=版本号名且与注册行**同 commit**。

1. `knowledge_source_connectors`：id PK｜kind CHECK('shared_dir','notion')｜name｜workspace_id｜collection_id（软删感知部分唯一）｜config_json TEXT（仅非密配置）｜interval_seconds（创建/PATCH 校验 ≥900，due 初值=id-hash 撒点）｜enabled｜sync_state('idle|running|auth_paused|error')｜cursor TEXT（游标列，非 metadata_json）｜next_run_at/last_run_at/last_success_at｜consecutive_failures｜error_code TEXT（字节级稳定枚举，纯函数钉遍历断言无路径/hash/页 ID 动态片段）｜error_message(≤512 截断)｜预算三列 max_items_per_run=500/max_deletes_per_run=200/max_fetch_bytes_per_run｜run_token（当前轮认领 CAS；崩溃回收=lease_ttl 过期置 idle）｜created/updated/deleted_at。索引：due 拾取部分索引 `(next_run_at) WHERE enabled AND deleted_at=''`（20261293 先例）＋`(workspace_id)`。退避：失败 `next_run_at=now()+2^consecutive_failures` 分钟 cap 60（memory_fact sweeper 先例）；auth 类连 5 次→`auth_paused`＋recordAudit。
2. `knowledge_source_connector_items`：`(connector_id, external_id)` PK｜document_id｜fingerprint（远端 rev）｜content_hash（提交成功才回填=checkpoint 推进点，对齐 vault 落库即 checkpoint）｜own_lifecycle BOOL（adopt=false）｜status('active|missing|deleted')｜miss_count｜last_seen_run_id｜first_seen_at/last_seen_at/deleted_at。索引：PK＋部分索引 `(connector_id) WHERE status='missing'`（删除候选拾取）＋`(document_id)`。Upsert 语义钉：出席即 miss_count=0＋清 error_message——flap 边界（缺席回归再缺席）不触发删除。
3. `knowledge_source_connector_credential`：id PK｜connector_id｜credential_key｜status｜secret_ref（仅存 enc: 信封，Sensitive）｜metadata_json（workspace_id/bot_id 等非密元数据）｜软删三列＋部分唯一 `(connector_id, credential_key) WHERE deleted_at=''`（`''::text` 谓词规范，mcp 先例）。
4. `knowledge_source_connector_runs`：id PK｜connector_id｜started_at/finished_at｜status('success|partial|failed|skipped')｜六计数 created/updated/unchanged/fetch_failed/deleted/miss_unknown｜bytes_pulled｜error_code/error_message。保留纪律：finalize 同事务单行 DELETE 保每 connector 最近 **100** 轮（cron_task_run 无 GC 教训的对冲设计）。

- 双轨：`EnsureSourceSyncSchema`（CREATE TABLE/INDEX IF NOT EXISTS 镜像，列集合与 SQL 字节级一致）＋ddl 注册行同 commit；索引走 CONCURRENTLY 注册轨先例。
- 持久化载体（拍板项 P6，推荐 hand-repo 裸 SQL）：四表 CRUD 扁平+免 ent codegen 单写者窗口税；先例边界写明＝channel_delivery_claim.go 实为 ent repo 内嵌裸 SQL CTE。复审触发＝需要 ent 查询构造面时再开窗口。

### ④ 凭据模型

选项：A 行表（契约1，channel_credential 同构）——**推荐**；B 行内 config_json `token_enc`（契约2：merge/保留语义复杂＋无 credential_key 维度＋OAuth refresh token 无落点）；C 仅 `env:VAR` 引用（K8s 注入备选，不唯一承载）。

铁律链：biz 新增 `CredentialCrypto.RequireKeyForSecret(ctx)`（仅校验 aesKey 可解析 32B）作为 UpsertCredential 前置硬错——无 key 拒写、不静默明文降级；读侧 API 只返 `configured`+`masked_preview`（sanitizeMCPUserCredential 模板），永不返明文、不建 Reveal 端点；运行时 walker 拉取前 `DecryptChannelSecretRef` 注密，解密失败→`sync_state='auth_paused'`＋`error_code='credential_decrypt_failed'`（稳定字节）＋告警，**不重试烧配额**；PUT credential 写成功即自动清 auth_paused/consecutive_failures 唤醒；Upsert/Delete 各 `recordAudit(AuditVerbCredentials, "source_connector")`，Summary 仅 credential_key、严禁 secret。

### ⑤ 调度与运行

接入面 B 定型：单例 fan-out worker（`internal/cronrunner/jobs/source_sync.go`，KnowledgeCurate 模板）＋ due 拾取走 `next_run_at` 部分索引 SQL（非 Go 全表内存筛）＋ per-connector 跨副本排他 `KnowledgeJobLocker(key='knowledge-sourcesync:<id>')`（**fail-closed**：外部 API 有配额副作用，PG 抖动宁停不重——与 cron lease fail-open 取值相反且理由写明）＋ 进程内 single-flight＋ workers.go goAfterReady 注册＋ env kill-switch（`SOURCE_SYNC_DISABLED`/`SOURCE_SYNC_<KIND>_DISABLED`）。

退避/停轮分级表：凭据/解密/分页不完整＝直置 auth_paused 或 error 停轮不重试；walk 5xx/超时＝JobRunner 30s/2m/10m＋consecutive_failures 退避；429＝尊重 Retry-After；死信＝DeadLetterSinkAdapter→memory_job_deadletter（不开新表）。宕机对齐 skipped_missed：逾期 connector 合并仅补一轮（walk 全量对账天然幂等，禁 backlog 追跑）。手动 trigger＝`next_run_at=now()` 入队语义＋在途 409（run_token 由 scheduler 认领时签发；scheduler 未上线窗口禁返伪造 202）。

### ⑥ 删除墓碑级联（八步序＋防误删双闸）

1. **保留集载入**：run 开始载入本 connector 全部 active item（管辖域；UI 上传/adopt 天然域外）。
2. **walk 标记**：emit 时 upsert item（last_seen_run_id=run_token, fingerprint, status='active', miss_count=0）。
3. **候选圈定**：`active ∧ last_seen_run_id<run_token` → status='missing', miss_count+=1。
4. **auth 整轮闸**：本 walk 任一 401/403 → `error_code='auth_failed'`，删除阶段整体跳过＋sync_state='auth_paused'＋告警（token 吊销→全量 404→误 tombstone 全库＝连接器头号事故模式）；**walk_empty_abort 通用闸**：空 walk ∧ 上轮 active>0 → 跳过删除＋告警（挂载瞬断/过滤配错同判）。
5. **ConfirmDeleted 三态**逐条确认；Unknown 保留 missing 不降级删除；miss_unknown 连 3 轮>0 走 MonitorBus 告警。
6. **预算截断**：Confirmed 超 `max_deletes_per_run` 部分留 miss 下轮（限单次误删爆炸半径）。
7. **级联唯一正道**：own_lifecycle=true → `Usecase.DeleteDocument`（prepareLocalDocumentDelete 对 `RootPath=''` 直通、不空转 trash；T4 后 SELECT FOR UPDATE+tombstoneDocumentTx 锚＋零账本 mint 护栏 DIS-5 自动生效）；false → 仅 item status='deleted' 不动文档。
8. **收尾**：item deleted_at；计数校正/GREATEST 由 DeleteDocument 内核承担；图谱卡片孤儿归 GraphProjectionWorker（禁手工清卡）；archive/版本行硬清归 TK-G32-03＋`chunk_archive`/`document_versions` 行数 gauge 进监控阈值告警。

notion 灰度期（T4 未接线窗口）默认 `tombstone_enabled=false`＝additive-only 防误删。

### ⑦ API 契约（custom JSON＋ADR 封尾，拍板项 P4）

推荐 custom JSON：`registerCustomRoutes` knowledge 块集中登记＋justification 注释＋专篇 ADR（占位 `ADR-KN-20261023-G38-API` 同 commit 落字）；晋升条件＝第二 connector kind GA 或 G-40 启动，一次性迁 proto（pb+TS 生成物同批）。理由：迭代期字段不稳＋ADR-KN-20261020-K 先例＋knowledge.proto 并行脏区叠加 make api 生成物合并成本。反案＝full 设计：新建 `api/kratos/knowledge/connector/v1/connector.proto`（§8.3 契约完整性优先）。

端点：GET/POST `/v1/knowledge/connectors`；GET/PATCH/DELETE `/v1/knowledge/connectors/{id}`；PUT/GET/DELETE `/{id}/credential`（GET 仅 configured+masked_preview）；`POST /{id}/sync`→202/409；`GET /{id}/runs?limit`；`GET /{id}/items?status=missing`。

权限：CreateConnector 从 ctx 盖 workspace_id（CreateDocument owner 盖章先例）；绑定共享库 ws='' 仅 system ctx 放行＋先于 assertCollectionMutateAccess 判定（404 反枚举）；写闸 assertCollectionMutateAccess（目标库）；读闸 assertCollectionAccess；config_json 出库前 sanitize 密文键。DELETE 默认软删 connector＋级联软删 credential（同笔 recordAudit）＋**保留已摄取文档**；`purge=1`→202 异步入队（同 advisory key 防与轮巡并发），逐条走 ⑥ 序，deleted_documents 汇入聚合帧——禁请求内开环跑 tombstone 事务。

### ⑧ WS 帧契约

`event_type='knowledge_source_sync'`，SystemNoticeEvent WS-only（Informational，AS-EVT-01 分级表不动）：meta=`{connector_id, run_token, collection_id, kind, phase:'started|progress|done|auth_paused', status, created, updated, unchanged, fetch_failed, deleted, miss_unknown, error_code, duration_ms}`。聚合纪律：每轮 ≤1 started＋节流 progress（≥15s 或计数变化>20%）＋≤1 done；auth_paused 状态迁移必发 1 帧（抑制 5min 重发）；变更集>50 时 per-doc 帧全压制并入终帧计数；手动触发∧变更≤10 豁免走既有 publishKnowledgeIngest（前端零改）；错误明细落 runs/connector 列不进帧。G-32 决策 9 producer 值空间登记 `connector_source_*` 前缀（tombstone 组合），与 T5 `knowledge_chunks_diff` 不双发（DOC-SYNC 项）。

### ⑨ 前端组件

KnowledgePage 设置区新增 `KnowledgeConnectorsPanel.vue`（KnowledgeEmbedderPanel 明线先例，禁命令面板暗线）：列表卡（名称/kind 徽章/目标库/sync-dot 圆点+tooltip 复用 KnowledgeVaultTree 渲染核/last_success_at/六计数摘要/手动同步 409 禁用态）；编辑抽屉（config 表单化/interval ≥900 下限/预算三值/enabled/凭据区 masked_preview+覆盖写+无 key 时禁保存并提示 ARANEA_CREDENTIAL_KEY）；runs 表＋items missing 运营视图（删除待决列表）；visibility 档选项默认 collection＋「导入内容对本工作区全库可见」提示文案（Notion 页级权限不映射的权衡进需求档）。api 走 features/knowledge/api.ts 手映射双通道（kratosApi 裸调＋asRecord/pickStr 先例）＋types.ts ConnectorDTO 镜像；WS 订阅复用 useKnowledgeIngestWs 模式（NoticeType 过滤＋meta.collection_id＋800ms 尾触发防抖）。connector 文档前端禁编辑入口（徽章判定）；仅 console.warn/info。

### ⑩ G-32 接口消费关系与错峰门

消费面＝`CommitIndexedDocument(Meta)`/`DeleteDocument` 签名面（已 commit 于 4f33e11af）。更新管道片（Applier 起）与 notion GA 均绑定「13 步 diff-commit tx 已接线」信号（internal/data/knowledge_chunk_archive.go:19-51 清单消化状态）；未接线窗口**禁**「旧全量重插临时发货」（embed 经济化反转即成本放大器）。例外面＝shared_dir 低频灰度可加性上线（旧路径代价有界：interval≥900＋撒点），删除阶段窗口期默认 tombstone_enabled=false。

### ⑪ 设计死区（不做什么）

不新增 current 查询横切谓词（G-05 零横切）；不旁路直删/全量重建 chunks；不新增文档 FSM 迁移路径；不给 knowledge_documents DDL 加列（拍板 P3 若反拍加 source_kind/connector_id 列则单开迁移评估）；不引入新选举机制/新死信表/新事件可靠性档；不碰 cron target_type 白名单四件套（欠账=无 cron UI/逐轮 cron_task_run 历史，runs 表弥补）；不接 fsnotify；不做 Notion 页级权限映射（首批）；不做 web/URL connector（crawl 是纯库未接线，需先补 checkpoint 基建，G-38 二期）；不做「连接器插件化」运行时注册（进程内编译期装配）。

### ⑫ 设计轮评审收敛账

九审全 adopt-with-grafts：mvp 84/84/79、full 88/82/76、delivery 86/85/78。**violations 三处全闭环**：迁移号撞 20261295→重锚程序化（③）；mvp「同库同 hash 各建各档」触红线 3→adopt 管辖语义消解（①）；mvp 三件套堆末片触 DOC-SYNC-1 字面→文档同步打散进每片验收（kickoff 切片纪律）。**grafts 22 项 P0 归宿**：401 fail-fast/auth_paused 闸（④⑤⑥）、walk_empty_abort 通用化（⑥）、miss_count 列（③-2）、due 部分索引＋interval≥900 校验（③-1）、错误分级表（⑤）、max_items/max_deletes 预算闸（③-1/⑥）、runs 自带保留（③-4）、Notion 页级指纹成本模型（②）、external_id 归一/sanitize 防撞钉（W3 验收）、error_code 字节稳定纯函数钉（③-1）、Upsert miss 归零 flap 语义（③-2）、cursor 失效回退（②）、sourcesync 落位消反向依赖（②）、purge 异步入队（⑦）、trigger 202 语义钉（⑤）、RequireKeyForSecret 签名化（④）、visibility 默认+文案（⑨）、DeleteConnector 级联软删 audit（⑦）、producer 值空间 DOC-SYNC（⑧）、TK-G32-03 可观测化（⑥-8）、文档同步打散（kickoff）。**显式挂账欠账目**：cron UI/历史（P4 晋升一并解决）、web connector（crawl checkpoint 基建）、OAuth per-user（凭据行表 metadata_json 已预留 refresh 落点）、archive 硬清 GC（TK-G32-03）、vault 与 fsdir connector 归一（AS 级重构不在本批）、adopt 档内容冻结不随源更新（已知限制，治理面提示挂 kickoff）。

### ⑬ 待拍板清单（11 项）

首项＝kickoff 预留的 Notion 凭据模型。逐项 topic/选项/推荐/理由见 37-knowledge.development.md G-38 kickoff；本设计按推荐值落地成文，拍板反转的受影响面：P1 凭据（④+W5）、P2 scope（W3/W6 排序）、P3 寻址（③+⑦）、P4 API（⑦+W5/W8）、P5 删除宽限（⑥+W7）、P6 载体（③）、P8 visibility（⑨）、P9 vault 关系（需求档分工）、P10 run 保留（③-4）、P11 权限映射（需求档+⑨文案）。

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

- 预检索命中以 `{chunks:[{n,chunk_id,doc_id,score,line}]}` 进入活动时间线；`n` 与 cue `[n]` 同序。前端按 `chunk_id` 合并为 chips，回答里的 `[n]` 渲染为脚注按钮，有 `doc_id` 则路由 `/knowledge?doc=`。工具路径 notice 不加 `n`。
- `config_json.knowledge.grounded_only`：Create/Update Agent 与 `evaluation` 一并 overlay，禁止整表清空 `config_json`。
- `formatKnowledgeCue(..., toolsEnabled, groundedOnly)` 返回实际写入的 cited chunks，供 numbered notice；空正文/空 ID 不占 `[n]`。
  - grounded + 无命中 + 关工具 → 硬拒答、禁止世界知识；
  - grounded + 无命中 + 开工具 → 只许 `knowledge_search`；
  - grounded + 有命中 → 只用段落，不足则说没有。
- cited 回采：回复含 `[n]` 即记该 chunk（兼原 ID/k-gram 启发式）。

### 实体词条（US-56）

- `EntityPipeline.SetWikiWriter` → `Usecase.EnsureEntityWikiPages`。
- 仅 team vault；`entries/<slug>.md`；最多 8 实体；幂等追加来源 wikilink。
- 写入成功后调用既有 `SetWriteBackReplay` 立刻重建 chunks/FTS（与写回词条同一钩子）；失败 Warn，索引自愈可补。
- 失败 Warn（`knowledge.entity.wiki`），实体轨已提交不回滚。

### 文档可见性（US-57）

- 列：`visibility TEXT DEFAULT 'collection'`、`owner_user_id TEXT DEFAULT ''`；索引 `(collection_id, visibility, owner_user_id)`。
- 读：`DocumentVisibleTo`；system 全见；无 user 只见 collection；有 user 见 collection 或 owner。
- 写：custom `GET/POST /v1/knowledge/documents/{id}/visibility`；标 private 时 `owner_user_id` = 当前用户。
- `ListDocuments` / `ListDocumentPaths` / `SearchChunks` / `SearchChunksBM25` 加 ACL 子句。直取内容走 `requireDocumentRead`（不可见 = NotFound）。
- 图扩展：`ListChunksByDocuments`、`ListLinks`、`ListActiveLinks` 两端可见性与检索一致，避免公开种子把私密邻居扩进 Agent。
- 默认上传仍为 collection。不做飞书/SharePoint 连接器。

---

## SP3. 知识库 UI 重设计：深空退役与 Tech Night 并轨（2026-08-23 用户批准）

> 动机（用户四条全选）：视觉太重太炫、功能多但散乱、风格与全站割裂想换、特效性能问题。约束：**功能零删减，只重排重组**。决策过程：brainstorming 可视化对比三方向（A 并入全站 Tech Night / B 克制版深空 / C Linear 式扁平工具风），用户批准「A 为骨架 + 中央编辑区吸收 C + 深空气质仅保留 3D 图谱全屏」；布局四决策点（搜索三合一/右栏默认收起/审查徽章可见化/删除性能模式）用户全选。

### SP3.1 视觉语言

- **框架面板并入全站 Tech Night 令牌**：页底 `var(--gradient-page)`、面板 `--glass-surface` / `--glass-border` / blur 18px 标准玻璃，与 Agents/Skills/Memory 等页同一语言；知识库不再是独立视觉体系。
- **中央编辑区扁平纯净**（吸收 C 方向）：纯色底、1px hairline 分隔、零发光零模糊，长时间阅读/编辑不晃眼；排版走 `--text-*` 阶梯。
- **深空气质仅保留在 3D 图谱全屏模式**（`KnowledgeGraph3D` + `graph3d/` 渲染管线不动）——该场景是按需打开的展示型场景。
- 明暗双主题自动继承 `body.body--dark` 令牌体系（旧深空为暗色单主题，重设计后天然获得浅色主题）；全部颜色/间距走既有 CSS 变量，禁止新增散落 hex。

### SP3.2 布局与信息架构

- **顶栏收敛**：库切换 + 当前路径面包屑｜⌘K 统一搜索框｜🔔 审查徽章（有待办时亮起）+ 图谱 + 设置。删除性能模式开关。
- **搜索三合一**：`QuickSwitcher`(⌘O) + `CommandPalette`(⌘K) + `SearchPanel`(Ctrl+Shift+F) 合并为新 `CommandCenter.vue`（复用 `PaletteModal`），浮层内分「文档 / 命令 / 内容」三段；⌘O、Ctrl+Shift+F 保留为快捷键别名，打开后自动定位对应分段。
- **右栏五面板 → 可收纳图标轨**：默认收起为 32px 细图标轨常显，点击展开 300px 面板；反链+出链合并为「链接」→ 大纲/链接/属性/图谱 4 个标签页（局部图谱面板保留「展开全屏」入口）。
- **审查待办可见化**：顶栏 🔔 徽章显示待办计数（写回 N + 治理 M，复用现有 `writeback-pending` / governance API，进入页面拉一次 + 审查后刷新），点击打开对应审查对话框；无待办时徽章隐藏。原 ⌘K 命令保留（归入命令分段）。
- **左栏不变**：库树 + 当前目录文件列表 + 上传队列（有任务时出现），仅视觉并轨。

### SP3.3 组件处置清单

| 处置 | 对象 | 说明 |
|---|---|---|
| 删除 | `effects/ParticleField.vue`、`LiquidGlassDefs.vue`、`TiltCard.vue`（仅测试引用）、`RingCarousel.vue`、`GlowButton.vue`（仅空态引用） | 空态改为标准空态（图标 + 近期文档平铺列表） |
| 删除 | `css/deep-space.sass`（`style.sass` 摘除 import）、`kb-portal` 机制 | 对话框/菜单/气泡 teleport 后不再需要重挂深空令牌，统一走全站 `_glass-dialog.sass` |
| 删除 | 性能模式（`performanceMode` 状态、顶栏开关、`kb-workbench--perf` 分支） | 特效清零后开关失去意义 |
| 合并 | `QuickSwitcher.vue` + `CommandPalette.vue` + `SearchPanel.vue` → `CommandCenter.vue` | 三分段；`PaletteModal.vue` 保留复用 |
| 改造 | `KnowledgeWorkbench.vue`、`WorkbenchTopBar.vue`（重排 + 🔔 徽章）、`WorkbenchSidebar.vue`（去 kb-portal/深空类）、`WorkbenchTabs.vue`（空态改造 + 去 RingCarousel/GlowButton）、`WorkbenchSidePanels.vue`（图标轨 + 4 标签页） | 布局与令牌并轨 |
| 改造 | 9 个对话框 + `KnowledgeUploadQueue` + 各 `panels/*.vue` | 仅换皮肤（去 `kb-portal`/GlassPanel → 全站玻璃对话框），功能逻辑不动 |
| 不动 | `graph3d/` 全部、`KnowledgeGraph3D.vue`、`NoteEditor.vue` 内核 | 3D 图谱保留深空；编辑器仅外层容器换扁平皮肤 |

### SP3.4 技术要点与验收

- **纯前端改动**：零 Proto/API/DB 变更；🔔 徽章数据复用 `GET /v1/knowledge/collections/{id}/writeback-pending` 与治理提案接口。
- **性能目标**：知识库页 0 个 rAF 循环、0 个 SVG 折射滤镜、0 个常驻 canvas 粒子（图谱全屏除外）；移除 deep-space.sass 整块样式；编辑区无 backdrop-filter 重绘。
- **测试**：`effects.spec.ts` 随组件删除；`workbench.spec.ts`/`WorkbenchSidebar.spec.ts` 等按新结构改断言；新增 CommandCenter 三分段与快捷键别名、图标轨展开收起、🔔 徽章计数测试。验收 `pnpm lint && pnpm test && pnpm build`；运行时 Docker 后端 + quasar dev(9301) 人工核验（R3）。
