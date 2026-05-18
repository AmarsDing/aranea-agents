# Knowledge 知识库 — 开发计划

> **版本**：2026-05-19 | **状态**：✅ 核心端到端可用，工程化待补
> **需求**：[37 knowledge.md](./37%20knowledge.md) · **设计**：[37 knowledge.design.md](./37%20knowledge.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md)

---

## 1. 模块定位

Knowledge 知识库：管理 Agent 的知识来源，支持文档上传、分块、向量化、检索和注入。

**代码锚点**：
- `api/kratos/knowledge/v1/knowledge.proto` — Knowledge CRUD + Search RPC
- `internal/service/knowledge.go` — KnowledgeService
- `internal/biz/knowledge.go` — KnowledgeUsecase + KnowledgeRepo
- `internal/data/knowledge.go` — KnowledgeRepo（PostgreSQL + pgvector）
- `internal/knowledge/chunker.go` — 文档分块（char/token）
- `internal/knowledge/embedder.go` — 向量化（OpenAI/Ollama）
- `internal/knowledge/retriever.go` — 检索器
- `internal/tools/knowledge/tool.go` — knowledge_search 工具
- `internal/agent/trpc_build.go` — KnowledgeSearch 装配
- `web/src/features/knowledge/api.ts` — 前端 API
- `web/src/stores/knowledge/index.ts` — 前端 Store

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Collection CRUD | ✅ | Create/Get/List/Delete + HTTP+gRPC 注册 |
| Document CRUD | ✅ | Create/List/Delete + 异步摄取（safego.Go） |
| 文档分块 | ✅ | `chunker.go`（char/token 策略） |
| 向量化 | ✅ | `embedder.go`（OpenAI/Ollama） |
| 向量检索 | ✅ | pgvector 余弦相似度 + ivfflat 索引 |
| 元数据过滤 | ✅ | filter_json → JSONB `@>` 操作符 |
| knowledge_search 工具 | ✅ | `tools/knowledge/tool.go` + `buildToolsetsForAgent` |
| 前端 API + Store | ✅ | `features/knowledge/api.ts` + `stores/knowledge/` |
| EnsureKnowledgeSchema 调用 | 🟡 | 函数已定义，但 `NewData()` 未调用（EP-DATA-01） |
| Embedder 配置注入 | 🟡 | Wire 工厂硬编码空配置（EP-KN-01） |
| 摄取进度可观测 | 🟡 | 异步摄取完成但前端无进度闭环（EP-KN-02） |
| Markdown/JSON/递归分块 | ❌ | 仅 char/token |
| Gemini/HuggingFace Embedding | ❌ | 仅 OpenAI/Ollama |
| AgenticFilter | ❌ | 未实现 |
| OCR / Extractor | ❌ | 未实现 |
| Reranker | ❌ | 未实现 |
| 多租户隔离 | ❌ | 未实现 |
| code_search 工具 | ❌ | 未实现 |

---

## 3. 差距与优先级

### 3.1 工程化闭环（P0/P1）

| 编号 | 差距 | 优先级 | EP | 说明 |
|------|------|--------|-----|------|
| G1 | `EnsureKnowledgeSchema` 未在启动期调用 | P0 | EP-DATA-01 | `NewData()` 需在配置 Postgres 时调用；nil Repo 时 service 应 fail-fast |
| G2 | Embedder 配置硬编码 | P1 | EP-KN-01 | provider/baseURL/apiKey/model 应从 conf/env 注入 |
| G3 | 摄取进度前端不可观测 | P2 | EP-KN-02 | 异步摄取完成后前端需轮询或 SSE 获取状态 |

### 3.2 功能扩展（P2）

| 编号 | 差距 | 优先级 | 说明 |
|------|------|--------|------|
| G4 | Markdown 按标题分块 | P2 | 集成 trpc `chunking/markdown.go` |
| G5 | JSON 结构分块 | P2 | 集成 trpc `chunking/json.go` |
| G6 | 递归分块 | P2 | 集成 trpc `chunking/recursive.go` |
| G7 | PDF/Word/HTML 文档解析 | P2 | 集成 trpc `document/reader/` 或 Extractor |
| G8 | 本地 Embedding 模型 | P2 | Gemini/HuggingFace embedder |
| G9 | 检索结果 Reranker | P2 | TopK/Cohere/Infinity 重排序 |

### 3.3 超越层（P3）

| 编号 | 差距 | 优先级 | 说明 |
|------|------|--------|------|
| G10 | AgenticFilter | P3 | LLM 动态生成过滤条件 |
| G11 | OCR 识别 | P3 | Tesseract/Docling 图片→文本 |
| G12 | 多租户隔离 | P3 | tenant_id 分区 |
| G13 | code_search 工具 | P3 | 代码语义搜索 |
| G14 | SourceSync 增量同步 | P3 | 数据源自动增量更新 |

---

## 4. 开发阶段

### Phase 1：工程化闭环（当前优先）

完成 EP-DATA-01、EP-KN-01、EP-KN-02，确保 Knowledge 模块在生产环境可用。

| 任务 | EP | 涉及文件 |
|------|-----|----------|
| `NewData()` 调用 `EnsureKnowledgeSchema` | EP-DATA-01 | `internal/data/data.go` |
| nil Repo 时 service 返回明确错误 | EP-DATA-01 | `internal/service/knowledge.go`、`internal/biz/knowledge.go` |
| Embedder 配置从 conf/env 注入 | EP-KN-01 | `internal/service/wire_providers.go`、conf |
| 摄取进度可观测（轮询/SSE） | EP-KN-02 | `internal/service/knowledge.go`、前端 |

### Phase 2：高级分块 + 文档解析

扩展分块策略和文档格式支持。

| 任务 | 涉及文件 |
|------|----------|
| Markdown 按标题分块 | `internal/knowledge/chunker.go` |
| JSON 结构分块 | `internal/knowledge/chunker.go` |
| 递归分块 | `internal/knowledge/chunker.go` |
| PDF/Word/HTML 文档解析 | 新建 `internal/knowledge/extractor.go` 或集成 trpc reader |

### Phase 3：Reranker + 高级检索

提升检索质量。

| 任务 | 涉及文件 |
|------|----------|
| TopK Reranker | 新建 `internal/knowledge/reranker.go` |
| Cohere/Infinity Reranker | 扩展 reranker |
| AgenticFilter | 集成 trpc `searchfilter` |

### Phase 4：超越层

| 任务 | 涉及文件 |
|------|----------|
| OCR 识别 | 新建 `internal/knowledge/ocr.go` |
| 多租户隔离 | 修改搜索过滤 + 向量存储 |
| code_search 工具 | 新建 `internal/tools/knowledge/code_search.go` |
| SourceSync 增量同步 | 新建 `internal/knowledge/sync.go` |

---

## 5. 任务清单

| # | 任务 | 优先级 | EP | Phase |
|---|------|--------|-----|-------|
| 1 | `NewData()` 调用 `EnsureKnowledgeSchema` | P0 | EP-DATA-01 | 1 |
| 2 | nil Repo 时 service/knowledge fail-fast | P0 | EP-DATA-01 | 1 |
| 3 | Embedder 配置从 conf/env 注入 | P1 | EP-KN-01 | 1 |
| 4 | 摄取进度前端可观测 | P2 | EP-KN-02 | 1 |
| 5 | Markdown 按标题分块 | P2 | — | 2 |
| 6 | JSON 结构分块 | P2 | — | 2 |
| 7 | 递归分块 | P2 | — | 2 |
| 8 | PDF/Word/HTML 文档解析 | P2 | — | 2 |
| 9 | 本地 Embedding 模型（Gemini/HuggingFace） | P2 | — | 2 |
| 10 | TopK Reranker | P2 | — | 3 |
| 11 | Cohere/Infinity Reranker | P2 | — | 3 |
| 12 | AgenticFilter | P3 | — | 3 |
| 13 | OCR 识别（Tesseract/Docling） | P3 | — | 4 |
| 14 | 多租户知识库隔离 | P3 | — | 4 |
| 15 | code_search 工具 | P3 | — | 4 |
| 16 | SourceSync 增量同步 | P3 | — | 4 |

---

## 6. 验收标准

### Phase 1

- [ ] 配置 Postgres 时 `EnsureKnowledgeSchema` 在启动期自动调用
- [ ] 无 Postgres 时 Knowledge API 返回明确 "服务不可用" 错误，不 panic
- [ ] Embedder provider/baseURL/apiKey/model 从配置文件或环境变量注入
- [ ] 前端可查询文档摄取进度（轮询文档状态或 SSE 推送）

### Phase 2

- [ ] Markdown 文档按标题层级正确分块
- [ ] JSON 文档按结构正确分块
- [ ] 可上传 PDF/Word/HTML 文档并正确解析

### Phase 3

- [ ] 检索结果经 Reranker 重排序后相关性提升
- [ ] AgenticFilter 启用后 LLM 可动态生成过滤条件

### Phase 4

- [ ] 图片/PDF 文档可 OCR 识别入库
- [ ] 不同租户搜索不到彼此的知识
- [ ] Agent 可调用 code_search 工具

---

## 7. 依赖与风险

| 依赖 | 说明 |
|------|------|
| PostgreSQL + pgvector | Knowledge 核心存储，无 PG 时模块不可用 |
| Embedding API | OpenAI 或 Ollama 端点必须可达 |
| PDF 解析 | 需引入第三方库（如 unidoc/unioffice）或 Docling 服务 |
| 本地 Embedding | 需 GPU 或大量 CPU 资源 |
| OCR | 需 Tesseract 或 Docling 服务部署 |
