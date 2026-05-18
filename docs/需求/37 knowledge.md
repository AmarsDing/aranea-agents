# M13: Knowledge 知识库 — 详细需求

> 对标 `pkg/trpc-agent-go/knowledge` 包，实现 RAG 知识库能力。
>
> **2026-05-19 现状对齐**：
> - ✅ Collection/Document/Chunk CRUD + 语义搜索 API 已上线（HTTP + gRPC）。
> - ✅ `knowledge_search` 工具经 `buildToolsetsForAgent` + `ToolKeyKnowledgeSearch` 进入 Agent 装配链（需 Agent 工具开关启用）。
> - ✅ 前端 Knowledge Store + API 层已实现。
> - 🟡 **Postgres 依赖**：`NewKnowledgeRepoFromData` 无 PG 时返回 nil Repo；`NewData()` **未**调用 `EnsureKnowledgeSchema`（EP-DATA-01）；Embedder 需 conf/env 注入（EP-KN-01）。
> - 🟡 摄取流水线异步为主（`safego.Go`），但进度不可观测、前端无闭环（EP-KN-02）。
> - ❌ AgenticFilter / OCR / 多租户隔离 / Reranker 未实现。
>
> 进度以 `guides/execution-plan.md` 附录 A 为准。

---

## 1. 用户故事

### US-1：知识库管理员创建知识集合

**作为**知识库管理员，**我希望**创建一个命名的知识集合（Collection），指定嵌入模型和维度，**以便**将相关文档组织在一起并统一检索。

**验收标准**：
- 可通过 API 创建 Collection，指定 name、description、embedding_model。
- 创建后 Collection 状态为 `active`，维度默认 1536。
- name 和 embedding_model 为必填项。

### US-2：用户上传文档到知识集合

**作为**用户，**我希望**向知识集合上传文档（文本/Markdown），**以便**文档被自动分块、向量化并索引，供后续语义搜索使用。

**验收标准**：
- 可通过 API 上传文档，传入 base64 编码的文档内容和元信息。
- 文档创建后状态为 `pending`，后台异步完成分块和向量化。
- 成功后状态变为 `indexed`；失败则变为 `error` 并记录错误信息。
- 分块参数（chunk_size、chunk_overlap）可在请求中指定。

### US-3：用户搜索知识库

**作为**用户，**我希望**通过自然语言查询搜索知识集合，**以便**获取与查询语义相关的文档片段。

**验收标准**：
- 可通过 API 发起语义搜索，指定 collection_id、query、top_k、min_score。
- 返回按相似度排序的文档片段（Chunk），包含内容、分数、来源文档 ID。
- 支持通过 filter_json 进行元数据过滤。

### US-4：Agent 通过工具搜索知识库

**作为** Agent，**我希望**在对话中调用 `knowledge_search` 工具搜索知识集合，**以便**获取外部知识增强回答质量。

**验收标准**：
- Agent 启用 `knowledge_search` 工具开关后，工具自动装配到 Agent 工具集。
- 工具接收 collection_id、query、top_k、min_score 参数。
- 搜索结果以结构化 JSON 返回给模型上下文。

### US-5：用户管理知识集合和文档

**作为**用户，**我希望**列出、查看和删除知识集合及文档，**以便**管理知识库生命周期。

**验收标准**：
- 可列出所有 Collection（分页）。
- 可列出某 Collection 下的所有 Document（分页）。
- 删除 Collection 时级联删除其下所有 Document 和 Chunk。
- 删除 Document 时级联删除其下所有 Chunk。

### US-6：LLM 动态过滤搜索结果（AgenticFilter）

**作为** Agent，**我希望** LLM 根据查询意图动态决定过滤条件，**以便**精准检索相关知识片段。

**验收标准**：
- 启用 AgenticFilter 后，LLM 可在搜索时自动生成过滤参数。
- 过滤条件基于文档元数据字段。

### US-7：OCR 文档识别入库

**作为**用户，**我希望**上传图片或 PDF 文档后自动 OCR 提取文本并入库，**以便**非纯文本文档也能被语义搜索。

**验收标准**：
- 图片/PDF 上传后自动 OCR 提取文本。
- 提取的文本进入分块和向量化流水线。
- OCR 失败时文档状态标记为 `error`。

### US-8：多租户知识库隔离（超越层）

**作为**系统管理员，**我希望**不同租户的知识库完全隔离，**以便**租户 A 搜索不到租户 B 的知识。

**验收标准**：
- 搜索请求自动注入租户 ID。
- 向量存储按租户分区。
- 跨租户搜索返回空结果。

---

## 2. 功能规格

### 2.1 知识集合管理

| 功能 | 说明 | 状态 |
|------|------|------|
| 创建集合 | 指定 name、description、embedding_model | ✅ |
| 列出集合 | 分页查询，支持 workspace 过滤 | ✅ |
| 获取集合 | 按 ID 获取单个集合详情 | ✅ |
| 删除集合 | 级联删除文档和向量块 | ✅ |

### 2.2 文档管理

| 功能 | 说明 | 状态 |
|------|------|------|
| 上传文档 | base64 编码内容，异步分块+向量化 | ✅ |
| 列出文档 | 按集合分页查询 | ✅ |
| 删除文档 | 级联删除向量块 | ✅ |
| 文档状态 | pending → indexing → indexed / error | ✅ |
| 进度可观测 | 前端可查询摄取进度 | 🟡 |

### 2.3 语义搜索

| 功能 | 说明 | 状态 |
|------|------|------|
| 向量搜索 | 余弦相似度，ivfflat 索引 | ✅ |
| 元数据过滤 | filter_json 通过 JSONB `@>` 操作符 | ✅ |
| 最低分数过滤 | min_score 阈值 | ✅ |
| TopK 限制 | 默认 5 | ✅ |

### 2.4 分块策略

| 策略 | 键 | 说明 | 状态 |
|------|-----|------|------|
| 按字符 | `char` | 按 N 字符窗口分割，含重叠 | ✅ |
| 按 Token | `token` | 空格分词，近似 Token 计数 | ✅ |
| Markdown 按标题 | — | 按标题层级分块 | ❌ |
| JSON 结构 | — | 按 JSON 结构分块 | ❌ |
| 递归分块 | — | 递归字符分割 | ❌ |

### 2.5 嵌入提供者

| 提供者 | 说明 | 状态 |
|--------|------|------|
| OpenAI 兼容 | `/v1/embeddings` 端点 | ✅ |
| Ollama | `/api/embeddings` 端点 | ✅ |
| Gemini | — | ❌ |
| HuggingFace | — | ❌ |

### 2.6 Agent 集成

| 功能 | 说明 | 状态 |
|------|------|------|
| knowledge_search 工具 | Agent 可调用搜索知识库 | ✅ |
| 工具开关 | Agent 工具配置中启用/禁用 | ✅ |
| AgenticFilter | LLM 动态生成过滤条件 | ❌ |
| code_search 工具 | 代码语义搜索 | ❌ |

### 2.7 高级功能

| 功能 | 说明 | 状态 |
|------|------|------|
| OCR 识别 | 图片/PDF 自动提取文本 | ❌ |
| Reranker | 检索结果重排序（TopK/Cohere/Infinity） | ❌ |
| SourceSync | 数据源增量同步 | ❌ |
| 多租户隔离 | 租户间知识库完全隔离 | ❌ |
| Extractor | 格式转换（PDF/图片 → 文本/Markdown） | ❌ |

---

## 3. trpc 框架参照

```
pkg/trpc-agent-go/knowledge/
├── knowledge.go          # Knowledge 接口：Search
├── default.go            # BuiltinKnowledge 实现（Load/AddSource/RemoveSource/ReloadSource）
├── default_options.go    # Option 函数
├── chunking/             # 分块策略
│   ├── chunking.go       # Strategy 接口 + createChunk
│   ├── fixed.go          # 固定大小分块
│   ├── recursive.go      # 递归分块
│   ├── markdown.go       # Markdown 按标题分块
│   └── json.go           # JSON 结构分块
├── document/             # 文档模型 + 读取器
│   ├── document.go       # Document 结构体
│   └── reader/           # Reader 接口 + 注册表（text/markdown/json/csv/pdf/docx/proto/golang）
├── embedder/             # 向量化
│   ├── embedder.go       # Embedder 接口
│   ├── openai/           # OpenAI Embedding
│   ├── ollama/           # Ollama Embedding
│   ├── gemini/           # Gemini Embedding
│   └── huggingface/      # HuggingFace Embedding
├── extractor/            # 格式转换（PDF/图片 → 文本/Markdown）
│   ├── extractor.go      # Extractor 接口
│   └── docling/          # Docling 服务实现
├── ocr/                  # OCR 识别
│   ├── ocr.go            # Extractor 接口
│   └── tesseract/        # Tesseract 实现
├── query/                # 查询增强
│   ├── query.go          # Enhancer 接口
│   └── passthrough.go    # 透传实现
├── reranker/             # 重排序
│   ├── reranker.go       # Reranker 接口
│   ├── topk/             # TopK 简单排序
│   ├── cohere/           # Cohere Reranker
│   └── infinity/         # Infinity Reranker
├── retriever/            # 检索器
│   ├── retriever.go      # Retriever 接口
│   └── default.go        # DefaultRetriever（完整 RAG 流水线）
├── searchfilter/         # 搜索过滤
│   ├── filter_condition.go # UniversalFilterCondition
│   └── builder.go        # 条件构建器
├── source/               # 数据源
│   └── source.go         # Source 接口 + 元数据常量
├── tool/                 # 知识搜索工具
│   ├── searchtool.go     # knowledge_search 工具
│   └── codesearchtool.go # code_search 工具
└── vectorstore/          # 向量存储
    ├── vectorstore.go    # VectorStore 接口
    ├── pgvector/         # PostgreSQL pgvector 实现
    ├── inmemory/         # 内存实现
    ├── elasticsearch/    # Elasticsearch 实现
    ├── milvus/           # Milvus 实现
    ├── qdrant/           # Qdrant 实现
    ├── sqlitevec/        # SQLite-vec 实现
    └── tcvector/         # TcVector 实现
```

### Knowledge 接口

```go
type Knowledge interface {
    Search(ctx context.Context, req *SearchRequest) (*SearchResult, error)
}

type SearchRequest struct {
    Query       string
    History     []ConversationMessage
    UserID      string
    SessionID   string
    MaxResults  int
    MinScore    float64
    SearchFilter *SearchFilter
    SearchMode  int
}
```

### LLMAgent 集成

```go
llmagent.New("agent",
    llmagent.WithKnowledge(knowledge),
    llmagent.WithKnowledgeFilter(filter),
    llmagent.WithEnableKnowledgeAgenticFilter(true),
)
```

---

## 4. API 端点

| 方法 | 路径 | 说明 | 状态 |
|------|------|------|------|
| POST | `/v1/knowledge/collections` | 创建集合 | ✅ |
| GET | `/v1/knowledge/collections` | 列出所有集合 | ✅ |
| GET | `/v1/knowledge/collections/{id}` | 获取单个集合 | ✅ |
| DELETE | `/v1/knowledge/collections/{id}` | 删除集合 + 所有数据 | ✅ |
| POST | `/v1/knowledge/documents` | 摄入文档（异步索引） | ✅ |
| GET | `/v1/knowledge/documents` | 列出文档 | ✅ |
| DELETE | `/v1/knowledge/documents/{id}` | 删除文档 + 块 | ✅ |
| POST | `/v1/knowledge/search` | 语义搜索 | ✅ |

---

## 5. 验收标准总览

| # | 验收标准 | 状态 |
|---|----------|------|
| 1 | 可创建/列出/获取/删除知识集合 | ✅ |
| 2 | 可上传文档并异步完成分块和向量化 | ✅ |
| 3 | 文档向量化后存储到 pgvector，可进行相似度搜索 | ✅ |
| 4 | Agent 可调用 `knowledge_search` 工具搜索知识库 | ✅ |
| 5 | 支持元数据过滤（filter_json） | ✅ |
| 6 | 知识搜索支持动态过滤（AgenticFilter） | ❌ |
| 7 | 图片/PDF 文档可 OCR 识别入库 | ❌ |
| 8 | 多租户知识库隔离 | ❌ |
| 9 | 摄取进度前端可观测 | 🟡 |
| 10 | Embedder 配置从 conf/env 注入 | 🟡 |

---

## 6. 运维指南

> 原 `guides/knowledge.md` 内容，2026-05-17 合入。

Knowledge 模块添加基于 pgvector 的 RAG（检索增强生成）管道。Agent 可上传文档、自动索引为向量嵌入，然后在查询时通过 `knowledge_search` 工具进行语义搜索。

### 6.1 架构

```
                  ┌─────────────────────┐
                  │  KnowledgeService   │  ← Kratos HTTP/gRPC
                  └────────┬────────────┘
                           │ biz.KnowledgeUsecase
              ┌────────────┴────────────┐
              │                         │
     chunker.go                   knowledge.go (pgvector)
     embedder.go                  ├── knowledge_collections
     retriever.go                 ├── knowledge_documents
              │                   └── knowledge_chunks (vector)
              └────────────────────────►
```

### 6.2 组件

| 组件 | 路径 | 用途 |
|------|------|------|
| Proto | `api/kratos/knowledge/v1/knowledge.proto` | HTTP + gRPC API |
| Biz | `internal/biz/knowledge.go` | 领域逻辑 + `KnowledgeRepo` 接口 |
| Data | `internal/data/knowledge.go` | PostgreSQL + pgvector raw SQL |
| Chunker | `internal/knowledge/chunker.go` | 文本分割（字符/Token 策略） |
| Embedder | `internal/knowledge/embedder.go` | OpenAI 兼容 + Ollama 嵌入 API |
| Retriever | `internal/knowledge/retriever.go` | 嵌入查询 → 调用 `SearchChunks` |
| Tool | `internal/tools/knowledge/tool.go` | `knowledge_search` trpc 工具 |
| Service | `internal/service/knowledge.go` | Kratos 服务适配器 |
| Wire | `internal/service/wire_providers.go` | Chunker/Embedder 工厂 |
| 前端 API | `web/src/features/knowledge/api.ts` | Knowledge API 调用 |
| 前端 Store | `web/src/stores/knowledge/index.ts` | Knowledge Pinia Store |

### 6.3 数据库 Schema

由 `data.EnsureKnowledgeSchema(ctx, db, dim)` 创建：

```sql
knowledge_collections   (id, name, embedding_model, dim, status, document_count, chunk_count, workspace, ...)
knowledge_documents     (id, collection_id, source, mime_type, size_bytes, chunk_count, status, error_message, ...)
knowledge_chunks        (id, doc_id, collection_id, content, embedding vector(N), metadata jsonb, chunk_index, ...)
```

索引：`ivfflat` on `knowledge_chunks.embedding` 用于余弦相似度。

**要求**：PostgreSQL + `pgvector` 扩展。

### 6.4 摄取流程

1. 客户端调用 `POST /v1/knowledge/documents`，传入 `content_base64`（文档负载）。
2. 服务创建文档记录（`status=pending`）并立即返回。
3. 后台 goroutine（`safego.Go`）分块文本、嵌入每个块、插入 `knowledge_chunks`。
4. 完成后文档状态更新为 `indexed`；失败则变为 `error`。

### 6.5 分块策略

| 策略 | 键 | 说明 |
|------|-----|------|
| 按字符 | `char` | 默认。按 N 字符窗口分割，含重叠。 |
| 按 Token | `token` | 空格分词单词；近似真实 Token 计数。 |

参数：
- `chunk_size` — 窗口大小（默认 512）
- `chunk_overlap` — 连续块之间的重叠（默认 64）

### 6.6 嵌入提供者

`Embedder` 支持两种后端，通过 `provider` 选择：

| 提供者 | 端点 | 说明 |
|--------|------|------|
| `openai`（默认） | `POST /v1/embeddings` | 兼容任何 OpenAI-API 服务器 |
| `ollama` | `POST /api/embeddings` | 本地 Ollama 实例 |

### 6.7 Agent 工具：`knowledge_search`

工具通过 `buildToolsetsForAgent` 装配链注入，需 Agent 工具配置中启用 `knowledge_search` 开关。

模型可调用：

```json
{ "collection_id": "abc123", "query": "What is the refund policy?", "top_k": 5 }
```

### 6.8 Prometheus 指标

| 指标 | 类型 | 说明 |
|------|------|------|
| `aranea_knowledge_ingest_documents_total` | Counter | 成功索引的文档数 |
| `aranea_knowledge_search_duration_seconds` | Histogram | 搜索延迟 |

### 6.9 限制

- 需要 pgvector；当 `db == nil` 时 repo 优雅降级（返回 nil）。
- 嵌入维度每个集合固定；更改需重建集合。
- 文档内容必须可文本解码（PDF/图像提取不在当前范围）。
- `EnsureKnowledgeSchema` 尚未在 `NewData()` 中调用（EP-DATA-01）。
- Embedder 配置目前硬编码默认值，需从 conf/env 注入（EP-KN-01）。
