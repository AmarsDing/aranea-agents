# M13: Knowledge 知识库 — 详细需求

> 对标 `pkg/trpc-agent-go/knowledge` 包，实现 RAG 知识库能力。
>
> **2026-05-17 现状对齐**：以下"现状分析"已被代码反超。当前实现状态：
> - ✅ `internal/biz/knowledge.go` / `internal/data/knowledge.go` / `internal/service/knowledge.go` 已通；`internal/knowledge/{chunker,embedder,retriever}.go` 已实现；`internal/tools/knowledge/tool.go` 已包装为 `knowledge_search` 工具。
> - ❌ **未在 Agent 装配链注入 knowledge tool**（`internal/agent/trpc_build.go` 没有把 KnowledgeUsecase 编织进 tool list）；Agent 实际跑时仍无 RAG。
> - ❌ Embedder 默认 stub，pgvector 后端未启用。
>
> 后续以 `guides/execution-plan.md` §3 EP-BIZ-02 为准；运维要点见 `guides/knowledge.md`。

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
