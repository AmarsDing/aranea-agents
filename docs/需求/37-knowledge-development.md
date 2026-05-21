# Knowledge 知识库 — 开发计划

> **版本**：2026-05-21 | **状态**：✅ 核心端到端可用，OCR/多租户待补
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
- `internal/knowledge/embedder.go` — 向量化（四 provider + EmbedBatch）
- `internal/knowledge/ingest.go` — 分块+向量化流水线
- `internal/knowledge/retriever.go` — 检索器
- `internal/tools/knowledge/tool.go` — knowledge_search 工具
- `internal/agent/trpc_build.go` — KnowledgeSearch 装配
- `internal/knowledge/chunk_strategy.go` — trpc 高级分块桥接
- `internal/knowledge/document_extract.go` — PDF/DOCX/HTML 文本提取
- `internal/knowledge/readers_import.go` — trpc reader 注册
- `internal/knowledge/reranker_factory.go` — env Reranker（KN-01）
- `internal/service/knowledge_embedder.go` — Embedder Wire（env + DB）
- `internal/biz/knowledge_embed_setting.go` — Embedder patch 合并
- `api/kratos/system_setting/v1/system_setting.proto` — `KnowledgeEmbedSettings`
- `internal/service/knowledge_retriever.go` — Retriever Wire
- `web/src/features/knowledge/api.ts` — 前端 API
- `web/src/stores/knowledge/index.ts` — 前端 Store
- `web/src/features/knowledge/useKnowledgeIngestWs.ts` — 入库 WS 进度

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Collection CRUD | ✅ | Create/Get/List/Delete + HTTP+gRPC 注册 |
| Document CRUD | ✅ | Create/List/Delete + 异步摄取（safego.Go） |
| 文档分块 | ✅ | `chunker.go`（char/token 策略） |
| 向量化 | ✅ | `embedder.go`（四 provider + EmbedBatch） |
| 向量检索 | ✅ | pgvector 余弦相似度 + ivfflat 索引 |
| 元数据过滤 | ✅ | filter_json → JSONB `@>` 操作符 |
| knowledge_search 工具 | ✅ | `tools/knowledge/tool.go` + `buildToolsetsForAgent` |
| 前端 API + Store | ✅ | `features/knowledge/api.ts` + `stores/knowledge/` |
| 管理页面 | ✅ | `web/src/pages/KnowledgePage.vue`；路由 `/knowledge`；侧栏 `menu.knowledge` |
| EnsureKnowledgeSchema 调用 | ✅ | `NewData()` 在 Postgres 就绪后调用（EP-DATA-01） |
| nil Repo fail-fast | ✅ | `ErrKnowledgeUnavailable`（EP-DATA-01） |
| Embedder 配置注入 | ✅ | env + `system_settings` + Knowledge Admin API（EP-KN-01） |
| 摄取进度可观测 | ✅ | Event Bus `knowledge_ingest` + `useKnowledgeIngestWs`（EP-KN-02） |
| 文档 metadata_json → Chunk | ✅ | `ingest.go` `NormalizeMetadataJSON` + `BuildIndexedChunks` |
| Markdown/JSON/递归分块 | ✅ | `chunk_strategy.go` → trpc `chunking/*` |
| PDF/Word/HTML 文档解析 | ✅ | `document_extract.go` + trpc `document/reader/*` |
| EmbedBatch 批量向量化 | ✅ | `embedder.go` OpenAI/Gemini/HF batch |
| Gemini/HuggingFace Embedder | ✅ | env + `system_settings` + `embedder.go` |
| AgenticFilter | ❌ | 未实现 |
| OCR / Extractor | ❌ | 未实现 |
| Reranker | ✅ | `KRATOS_KNOWLEDGE_RERANKER`（topk/cohere/infinity） |
| 多租户隔离 | ❌ | 未实现 |
| code_search 工具 | ❌ | 未实现 |

---

## 3. 差距与优先级

### 3.1 工程化闭环（P0/P1）— 已完成

| 编号 | 项 | 状态 | EP |
|------|-----|------|-----|
| G1 | `EnsureKnowledgeSchema` 启动期调用 + nil Repo fail-fast | ✅ | EP-DATA-01 |
| G2 | Embedder env + Admin 运行时配置 | ✅ | EP-KN-01 |
| G3 | 摄取进度 WS + 文档 status | ✅ | EP-KN-02 |
| G4 | Embedder 写入 system_settings | ✅ | EP-KN-01 |
| G5 | 文档 metadata_json → Chunk | ✅ | — |

### 3.2 功能扩展（P2）

| 编号 | 差距 | 优先级 | 说明 |
|------|------|--------|------|
| G5 | Markdown 按标题分块 | P2 | 集成 trpc `chunking/markdown.go` |
| G6 | JSON 结构分块 | P2 | 集成 trpc `chunking/json.go` |
| G7 | 递归分块 | P2 | 集成 trpc `chunking/recursive.go` |
| G8 | PDF/Word/HTML 文档解析 | P2 | 集成 trpc `document/reader/` 或 Extractor |
| G9 | 本地 Embedding 模型 | P2 | Gemini/HuggingFace embedder |
| G10 | EmbedBatch 批量 API | P2 | 减少逐条 embed HTTP 往返 |

### 3.3 超越层（P3）

| 编号 | 差距 | 优先级 | 说明 |
|------|------|--------|------|
| G11 | AgenticFilter | P3 | LLM 动态生成过滤条件 |
| G12 | OCR 识别 | P3 | Tesseract/Docling 图片→文本 |
| G13 | 多租户隔离 | P3 | tenant_id 分区 |
| G14 | code_search 工具 | P3 | 代码语义搜索 |
| G15 | SourceSync 增量同步 | P3 | 数据源自动增量更新 |

---

## 4. 开发阶段

### Phase 1：工程化闭环 — ✅ 已完成

| 任务 | EP | 状态 | 涉及文件 |
|------|-----|------|----------|
| `KnowledgePage.vue` 管理界面 | — | ✅ | `web/src/pages/KnowledgePage.vue` |
| `NewData()` 调用 `EnsureKnowledgeSchema` | EP-DATA-01 | ✅ | `internal/data/data.go` |
| nil Repo fail-fast | EP-DATA-01 | ✅ | `internal/biz/knowledge.go` |
| Embedder env + Admin API/UI | EP-KN-01 | ✅ | `knowledge_embedder.go`、`KnowledgeEmbedderPanel.vue` |
| 摄取 WS 进度 | EP-KN-02 | ✅ | `useKnowledgeIngestWs.ts`、Event Bus |
| metadata_json → Chunk | — | ✅ | `internal/knowledge/ingest.go` |
| Reranker env 集成 | KN-01 | ✅ | `reranker_factory.go`、`retriever.go` |

### Phase 2：高级分块 + 文档解析 — ✅ 已完成

| 任务 | 状态 | 涉及文件 |
|------|------|----------|
| Markdown / JSON / 递归分块 | ✅ | `chunk_strategy.go` |
| PDF / DOCX / HTML 解析 | ✅ | `document_extract.go`、`readers_import.go` |
| EmbedBatch | ✅ | `embedder.go` |
| Gemini / HuggingFace Embedder | ✅ | `embedder.go`、`knowledge_embedder.go` |

### Phase 3：高级检索 — Rerank ✅，AgenticFilter 待补

| 任务 | 状态 | 涉及文件 |
|------|------|----------|
| TopK / Cohere / Infinity Reranker | ✅ | `reranker_factory.go`、`retriever.go` |
| AgenticFilter | ⏳ | 集成 trpc `searchfilter` |

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
| 3 | Embedder env + Admin 配置 | P1 | EP-KN-01 | 1 ✅ |
| 4 | 摄取进度 WS | P2 | EP-KN-02 | 1 ✅ |
| 4b | metadata_json 写入 Chunk | P1 | — | 1 ✅ |
| 10 | Reranker env 集成 | P2 | KN-01 | 3 ✅ |
| 5 | Markdown 按标题分块 | P2 | — | 2 ✅ |
| 6 | JSON 结构分块 | P2 | — | 2 ✅ |
| 7 | 递归分块 | P2 | — | 2 ✅ |
| 8 | PDF/Word/HTML 文档解析 | P2 | — | 2 ✅ |
| 9 | Gemini/HuggingFace + EmbedBatch | P2 | — | 2 ✅ |
| 10 | TopK Reranker | P2 | KN-01 | 3 ✅ |
| 11 | Cohere/Infinity Reranker | P2 | KN-01 | 3 ✅ |
| 12 | AgenticFilter | P3 | — | 3 |
| 13 | OCR 识别（Tesseract/Docling） | P3 | — | 4 |
| 14 | 多租户知识库隔离 | P3 | — | 4 |
| 15 | code_search 工具 | P3 | — | 4 |
| 16 | SourceSync 增量同步 | P3 | — | 4 |

---

## 6. 验收标准

### Phase 1 — ✅

- [x] 配置 Postgres 时 `EnsureKnowledgeSchema` 在启动期自动调用
- [x] 无 Postgres 时 Knowledge API 返回明确 "服务不可用" 错误
- [x] Embedder provider/baseURL/apiKey/model 从 env 注入 + Admin 运行时更新
- [x] 前端 WS 订阅摄取进度 + 文档 status 展示
- [x] 文档 `metadata_json` 写入 Chunk 供 filter_json 过滤

### Phase 2 — ✅

- [x] Markdown 文档按标题层级正确分块（`chunk_strategy=markdown`）
- [x] JSON 文档按结构正确分块（`chunk_strategy=json`）
- [x] 可上传 PDF/DOCX/HTML 并提取文本（mime/扩展名驱动）
- [x] EmbedBatch 减少 OpenAI/Gemini/TEI HTTP 往返
- [x] Gemini / HuggingFace TEI embedder 可用

### Phase 3

- [x] 检索结果经 Reranker 重排序（env 配置 + Search 请求覆盖）
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
