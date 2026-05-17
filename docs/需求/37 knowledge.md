# M13: Knowledge 知识库 — 详细需求

> 对标 `pkg/trpc-agent-go/knowledge` 包，实现 RAG 知识库能力。
>
> **2026-05-17 现状对齐**（2026-05-17 二批复核）：
> - ✅ Biz / Service / **HTTP+gRPC 已注册**；`knowledge_search` 经 `buildToolsetsForAgent` + `ToolKeyKnowledgeSearch` 进入装配链（需 Agent 工具开关启用）。
> - 🟡 **Postgres 依赖**：`NewKnowledgeRepoFromData` 无 PG 时返回 nil Repo；`NewData()` **未**调用 `EnsureKnowledgeSchema`（EP-DATA-01）；Embedder 需 conf/env 注入（EP-KN-01）。
> - 🟡 摄取流水线同步为主，异步进度与前端闭环待 EP-KN-02。
>
> 进度以 `guides/execution-plan.md` 附录 A 为准。运维要点见 §6。

---

## 1. 现状分析（已过期，保留参考）

项目无 Knowledge 知识库能力。当前 Agent 仅能通过 Memory 记忆和 Skill 技能获取上下文，无法检索外部知识。

---

## 2. trpc 框架参照

```
pkg/trpc-agent-go/knowledge/
├── knowledge.go          # Knowledge 接口：Search
├── default.go            # DefaultKnowledge 实现
├── default_options.go    # 默认选项
├── chunking/             # 分块策略
│   ├── fixed.go          # 固定大小分块
│   └── json.go           # JSON 结构分块
├── ocr/                  # OCR 识别
│   └── ocr.go
├── query/                # 查询处理
│   └── query.go
├── source/               # 数据源
│   └── source.go
├── tool/                 # 知识搜索工具
│   ├── searchtool.go     # knowledge_search 工具
│   └── code_dedup.go     # 代码去重
└── searchfilter/         # 搜索过滤
    └── ...
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

## 3. 需求清单

### 3.1 Knowledge Service 适配器

**需求**：桥接 trpc `knowledge.Knowledge` 接口

**实现要点**：
- 新建 `internal/knowledge/trpc/service.go`
- 实现 `knowledge.Knowledge` 接口
- 底层使用 pgvector 进行向量搜索

**验收标准**：Knowledge Search 返回相关文档

### 3.2 文档分块

**需求**：支持多种分块策略

**实现要点**：
- 集成 trpc `knowledge/chunking` 包
- 支持固定大小分块（Fixed）
- 支持 JSON 结构分块
- 分块参数可配置（chunk_size, overlap）

**验收标准**：文档按配置策略正确分块

### 3.3 向量存储

**需求**：文档向量化和存储

**实现要点**：
- 使用 pgvector 存储文档向量
- 集成项目的 `data/pgvector/store.go`
- 支持多种 Embedding 模型

**验收标准**：文档向量化后存储到 pgvector，可进行相似度搜索

### 3.4 知识搜索工具

**需求**：Agent 可通过工具搜索知识库

**实现要点**：
- 集成 trpc `knowledge/tool/searchtool.go`
- 在 `BuildTRPCLLMAgent` 中通过 `WithKnowledge` 注入
- 搜索结果自动注入 Agent 上下文

**验收标准**：Agent 可调用 `knowledge_search` 工具搜索知识库

### 3.5 AgenticFilter

**需求**：LLM 动态决定是否传递过滤参数

**实现要点**：
- 集成 trpc `knowledge/searchfilter` 包
- `WithEnableKnowledgeAgenticFilter(true)` 启用
- LLM 可根据查询动态选择过滤条件

**验收标准**：知识搜索支持动态过滤

### 3.6 OCR 识别

**需求**：支持图片/PDF 文档的 OCR 识别

**实现要点**：
- 集成 trpc `knowledge/ocr` 包
- 图片/PDF 上传后自动 OCR 提取文本
- 提取的文本进入知识库

**验收标准**：图片/PDF 文档可被 OCR 识别并入库

### 3.7 多租户知识库隔离（超越层）

**需求**：不同租户的知识库完全隔离

**实现要点**：
- SearchFilter 中增加 `tenant_id` 字段
- 向量存储按租户分区
- API 层强制注入租户 ID

**验收标准**：不同租户搜索不到彼此的知识

### 3.8 知识库管理 API

**需求**：通过 API 管理知识库

**实现要点**：
- `POST /knowledge/documents` — 上传文档
- `GET /knowledge/documents` — 列出文档
- `DELETE /knowledge/documents/:id` — 删除文档
- `POST /knowledge/search` — 搜索知识库

**验收标准**：通过 API 可管理知识库的完整生命周期

---

## 4. 涉及文件

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/knowledge/trpc/service.go` | 新建 | Knowledge Service 适配器 |
| `internal/knowledge/trpc/chunking.go` | 新建 | 分块策略 |
| `internal/knowledge/trpc/embedding.go` | 新建 | 向量化 |
| `internal/agent/trpc_build.go` | 修改 | 注入 Knowledge |
| `internal/data/pgvector/store.go` | 修改 | 扩展向量搜索 |
| `internal/service/knowledge.go` | 新建 | Knowledge 服务层 |
| `internal/server/register_knowledge.go` | 新建 | Knowledge HTTP 端点 |
| `web/src/features/knowledge/` | 新建 | 前端知识库管理 |

---

## 5. 验收标准总览

1. Knowledge Search 返回相关文档
2. 文档按配置策略正确分块
3. 文档向量化后存储到 pgvector
4. Agent 可调用 `knowledge_search` 工具
5. 知识搜索支持动态过滤
6. 图片/PDF 文档可 OCR 识别入库
7. 多租户知识库隔离（超越层）

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

### 6.3 数据库 Schema

由 `data.EnsureKnowledgeSchema(ctx, db, dim)` 创建：

```sql
knowledge_collections   (id, name, embedding_model, dim, status, ...)
knowledge_documents     (id, collection_id, source, status, chunk_count, ...)
knowledge_chunks        (id, doc_id, collection_id, content, embedding vector(N), metadata jsonb, ...)
```

索引：`ivfflat` on `knowledge_chunks.embedding` 用于余弦相似度。

**要求**：PostgreSQL + `pgvector` 扩展。

### 6.4 API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/v1/knowledge/collections` | 创建集合 |
| GET | `/v1/knowledge/collections` | 列出所有集合 |
| GET | `/v1/knowledge/collections/{id}` | 获取单个集合 |
| DELETE | `/v1/knowledge/collections/{id}` | 删除集合 + 所有数据 |
| POST | `/v1/knowledge/documents` | 摄入文档（异步索引） |
| GET | `/v1/knowledge/documents` | 列出文档 |
| DELETE | `/v1/knowledge/documents/{id}` | 删除文档 + 块 |
| POST | `/v1/knowledge/search` | 语义搜索 |

#### 摄入流程

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

将 `Retriever` 附加到 Agent 上下文：

```go
ctx = knowledgetool.WithRetriever(ctx, retriever)
```

然后向 trpc runner 注册 `knowledgetool.NewSearchTool()`。

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

- 需要 pgvector；当 `db == nil` 时 repo 优雅降级（仅 schema 调用）。
- 嵌入维度每个集合固定；更改需重建集合。
- 文档内容必须可文本解码（PDF/图像提取不在 S6 范围内）。
