# Knowledge 知识库 — 开发计划

> **版本**：2026-05-29 | **状态**：✅ Phase 5 Advanced RAG 完成，Phase 6 Agentic RAG 完成，Phase 7 质量优化完成，OCR/多租户待补
> **需求**：[37 knowledge.md](./37%20knowledge.md) · **设计**：[37 knowledge.design.md](./37%20knowledge.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md)

---

## 1. 模块定位

Knowledge 知识库：管理 Agent 的知识来源，支持文档上传、分块、向量化、检索和注入。

**代码锚点**：
- `api/kratos/knowledge/v1/knowledge.proto` — Knowledge CRUD + Search RPC（含 `rewrite_strategy` + `hybrid_search`）
- `internal/service/knowledge.go` — KnowledgeService
- `internal/service/knowledge_advanced.go` — Advanced RAG Wire 工厂
- `internal/biz/knowledge.go` — KnowledgeUsecase + KnowledgeRepo + SparseSearcher
- `internal/data/knowledge.go` — KnowledgeRepo（PostgreSQL + pgvector + BM25）
- `internal/knowledge/chunker.go` — 文档分块（char/token）
- `internal/knowledge/embedder.go` — 向量化（四 provider + EmbedBatch）
- `internal/knowledge/ingest.go` — 分块+向量化流水线
- `internal/knowledge/retriever.go` — 检索器
- `internal/knowledge/query_rewriter.go` — 查询重写（HyDE/Decomposition/MultiQuery）
- `internal/knowledge/hybrid_retriever.go` — 混合检索（Dense+Sparse+RRF）
- `internal/knowledge/adaptive_router.go` — 自适应检索路由
- `internal/knowledge/retrieval_evaluator.go` — 检索质量评估（CRAG）
- `internal/knowledge/federated_retriever.go` — 跨 Collection 联邦搜索
- `internal/knowledge/search_helpers.go` — 检索评估辅助
- `internal/tools/knowledge/tool.go` — knowledge_search + knowledge_reflect 工具
- `internal/agent/knowledge_inject.go` — Plan-Then-Retrieve BeforeModel 钩子
- `internal/agent/tool_assembly.go` — KnowledgeSearch/KnowledgeReflect 装配
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
| knowledge_reflect 工具 | ✅ | `tools/knowledge/tool.go` + `ToolKeyKnowledgeReflect` |
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
| 查询重写 | ✅ | `query_rewriter.go`（HyDE/Decomposition/MultiQuery） |
| 混合检索 | ✅ | `hybrid_retriever.go`（Dense+BM25+RRF） |
| BM25 全文检索 | ✅ | `data/knowledge.go` `SearchChunksBM25` + GIN 索引 |
| 自适应检索路由 | ✅ | `adaptive_router.go`（查询复杂度分类） |
| 检索质量评估 | ✅ | `retrieval_evaluator.go`（CRAG 式自校验） |
| 跨 Collection 联邦搜索 | ✅ | `federated_retriever.go`（并行广播 + Route 策略） |
| Plan-Then-Retrieve | ✅ | `agent/knowledge_inject.go`（BeforeModel 钩子注入 Collection 摘要） |
| 联邦搜索 Route 策略 | ✅ | `federated_retriever.go`（`SearchWithOptions` + `routeCollections`） |
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
| 17 | 查询重写（HyDE/Decomposition/MultiQuery） | P0 | — | 5 ✅ |
| 18 | BM25 全文检索（ts_vector + GIN 索引） | P0 | — | 5 ✅ |
| 19 | 混合检索（Dense+Sparse+RRF 融合） | P0 | — | 5 ✅ |
| 20 | 自适应检索路由（查询复杂度分类） | P1 | — | 5 ✅ |
| 21 | 检索质量评估（CRAG 式自校验） | P1 | — | 5 ✅ |
| 22 | knowledge_reflect 工具 | P0 | — | 6 ✅ |
| 23 | 跨 Collection 联邦搜索 | P1 | — | 6 ✅ |
| 24 | Plan-Then-Retrieve | P1 | — | 6 ✅ |
| 25 | 联邦搜索 Route 策略 | P2 | — | 6 ✅ |

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

### Phase 5 — ✅

- [x] 查询重写（HyDE）生成假设性回答后检索，提升语义召回
- [x] 查询分解（Decomposition）将复杂查询拆分为子问题
- [x] 多查询改写（MultiQuery）生成多角度查询变体
- [x] BM25 全文检索（PostgreSQL ts_vector + GIN 索引）
- [x] 混合检索 RRF 融合（Dense + Sparse + RRF，K=60）
- [x] 自适应路由根据查询复杂度自动选择检索模式
- [x] 检索质量评估（CRAG）在结果不足时自动补充检索
- [x] Search API 支持 `rewrite_strategy` 和 `hybrid_search` 参数
- [x] 前端搜索面板增加混合检索模式/查询重写策略/Rerank 控件

### Phase 6 — ✅

- [x] Agent 可调用 `knowledge_reflect` 工具评估检索质量
- [x] 跨 Collection 联邦搜索（多集合并行 + 结果合并去重）
- [x] FederatedRetriever/RetrievalEvaluator Context 注入链完整
- [x] knowledge_reflect 工具注册链完整（ToolKey + effective_config + tool_assembly + seed）
- [x] Plan-Then-Retrieve（BeforeModel 钩子注入 Collection 摘要到 Agent 系统提示）
- [x] 联邦搜索 Route 策略（基于 Collection 名称/描述相关性智能路由）

### Phase 7：质量优化 — ✅ 已完成

| 任务 | 状态 | 涉及文件 |
|------|------|----------|
| KnowledgeService 构造函数 8→6 参数 | ✅ | `service/knowledge.go`（`KnowledgeSearchDeps`） |
| `ProvideKnowledgeSearchDeps` Wire provider | ✅ | `service/knowledge_advanced.go` |
| IngestDocument 默认值逻辑下移 | ✅ | `knowledge/ingest.go`（`IngestParams.ApplyDefaults()`） |
| agent/knowledge_inject.go 编译验证 | ✅ | 无编译错误 |
| aranea-review 审查 | ✅ | 0 阻断、0 建议、1 提示 |

### Phase 8+（路线图）

> 完整方案见 [37-knowledge-evolution-roadmap.md](./37-knowledge-evolution-roadmap.md)

- [ ] **GraphRAG** — 知识图谱构建（实体/关系提取）+ 图增强检索 + 图查询工具
- [ ] **Skill Knowledge** — 技能知识库（三层技能层次）+ 知识导航工具 + 技能蒸馏管线

### Phase 5：Advanced RAG — ✅ 已完成

| 任务 | 状态 | 涉及文件 |
|------|------|----------|
| 查询重写（HyDE/Decomposition/MultiQuery） | ✅ | `query_rewriter.go`、`llm_resolver.go` |
| BM25 全文检索（PostgreSQL ts_vector） | ✅ | `data/knowledge.go`（`SearchChunksBM25` + GIN 索引） |
| 混合检索（Dense+Sparse+RRF 融合） | ✅ | `hybrid_retriever.go` |
| 自适应检索路由（查询复杂度分类） | ✅ | `adaptive_router.go` |
| 检索质量评估（CRAG 式自校验） | ✅ | `retrieval_evaluator.go`、`search_helpers.go` |
| Search API 集成（rewrite_strategy/hybrid_search 参数） | ✅ | `knowledge.proto`、`service/knowledge.go` |
| 前端搜索面板增加混合检索/查询重写控件 | ✅ | `KnowledgeSearchPanel.vue`、`useKnowledgePage.ts` |
| Wire 工厂（5 个新 Provider） | ✅ | `knowledge_advanced.go` |

### Phase 6：Agentic RAG — ✅ 已完成

| 任务 | 状态 | 涉及文件 |
|------|------|----------|
| knowledge_reflect 工具（Agent 自校验检索质量） | ✅ | `tools/knowledge/tool.go` |
| 跨 Collection 联邦搜索（FederatedRetriever） | ✅ | `federated_retriever.go` |
| Context 注入链（FederatedRetriever/Evaluator） | ✅ | `chat_orchestrator.go`、`chat_orchestrator_turn.go`、`runner.go`、`runner_team_trpc.go` |
| 工具注册链（ToolKey + effective_config + tool_assembly + seed） | ✅ | `tool.go`、`tool_catalog_runtime.go`、`effective_config.go`、`tool_assembly.go`、`builtin_tools_seed.go` |
| Plan-Then-Retrieve（L4 prompt 注入 Collection 摘要） | ✅ | `agent/knowledge_inject.go`、`builder_deps.go`、`callback_chain.go` |
| 联邦搜索 Route 策略（智能路由到最相关 Collection） | ✅ | `federated_retriever.go`（`SearchWithOptions` + `routeCollections`） |

---

## 7. 依赖与风险

| 依赖 | 说明 |
|------|------|
| PostgreSQL + pgvector | Knowledge 核心存储，无 PG 时模块不可用 |
| Embedding API | OpenAI 或 Ollama 端点必须可达 |
| PDF 解析 | 需引入第三方库（如 unidoc/unioffice）或 Docling 服务 |
| 本地 Embedding | 需 GPU 或大量 CPU 资源 |
| OCR | 需 Tesseract 或 Docling 服务部署 |
| LLM API | 查询重写和检索评估依赖 LLM 调用，无 LLM 时自动降级 |

---

## 8. 代码优化记录

> 详细优化记录见 [2026-05-29-Knowledge-Optimization.md](../../changelog/2026-05-29-Knowledge-Optimization.md)

### 第一轮：安全 + 可靠性修复（已完成）

| 修复项 | 说明 |
|--------|------|
| SQL 注入修复 | MinScore 参数化查询 |
| 错误吞没修复 | 5 处 `_ =` 异步 ingest + 查询重写错误 |
| LLM 评估假阳性修复 | Sufficient:true → false（安全降级） |
| HTTP 客户端超时 | http.DefaultClient → 60s 超时 |
| Service 层业务逻辑下移 | 提取到 `search_helpers.go` |
| resolveModel 代码重复消除 | 提取到 `llm_resolver.go` |
| Repo 接口拆分 | 11 方法 → CollectionRepo(5)/DocumentRepo(5)/ChunkRepo(3) |
| 常量提取 | DefaultChunkSize/DefaultChunkOverlap |

### 第二轮：错误处理规范化（已完成）

| 修复项 | 说明 |
|--------|------|
| fmt.Errorf → kerrors 全量替换 | 27 处替换（9 个文件），`fmt.Errorf` 已清零 |
| 错误分类修正 | nil 配置检查改用 ServiceUnavailable，环境变量错误改用 InternalServer |
| Service 层错误映射 | `InternalServer(err.Error())` → `FromError(err)` 保留原始错误类型 |
| 子查询全失败日志 | adaptive_router 所有子查询失败时记录 SysLogWarn |
| 静默降级日志 | search_helpers 评估/补充检索失败时记录 SysLogWarn |
| 缩进修正 | service/knowledge.go rewriteErr 块缩进 |

### 第三轮：OOP 优化 + 降级策略注释（已完成）

| 修复项 | 说明 |
|--------|------|
| RetrievalEvaluator 降级策略注释 | 三种降级策略均添加注释说明设计意图 |
| Usecase 子接口字段拆分 | `Usecase.repo` 拆分为 `collections`/`documents`/`chunks` 三个子接口字段 |
| requireRepo() 更新 | 检查三个子接口字段而非单一 repo 字段 |
| data/knowledge.go 编译期接口检查 | 新增 `var _ biz.KnowledgeRepo = (*knowledgeRepo)(nil)` |

### 剩余优化项

| 优先级 | 问题 | 说明 |
|--------|------|------|
| ~~P2~~ | ~~KnowledgeService 构造函数 8 参数~~ | ✅ Phase 7：引入 `KnowledgeSearchDeps` 聚合检索依赖，8→6 参数 |
| ~~P2~~ | ~~NewUsecase 仍接收 Repo 组合接口~~ | 保持向后兼容，内部已拆分为 collections/documents/chunks 子接口 |
| ~~P2~~ | ~~IngestDocument 默认值逻辑可下移~~ | ✅ Phase 7：`IngestParams.ApplyDefaults()` 下移到 knowledge 包 |
| ~~P3~~ | ~~agent/knowledge_inject.go 编译错误~~ | ✅ 已验证无编译错误 |
| ~~P2~~ | ~~tools/knowledge 13 处 fmt.Errorf~~ | ✅ Round 1：全部替换为 kerrors.BadRequest/InternalServer |
| ~~P0~~ | ~~上传大小/解码/MIME magic 校验~~ | ✅ Round 3：32MB 限制 + MIME magic + 白名单（KB-02） |
| ~~P0~~ | ~~嵌入维度强校验~~ | ✅ Round 2：InsertChunks 事务前校验维度（KB-03） |
| ~~P1~~ | ~~CreateCollection embedding_model 绑定校验~~ | ✅ Round 3：校验与当前 embedder 配置一致（KB-05） |
| ~~P1~~ | ~~Memory 与 Knowledge Embedder 解耦~~ | ✅ Round 4：MemoryEmbeddingAdapter 适配器（KB-06） |
| ~~P1~~ | ~~Team Runner 注入 KnowledgeBases~~ | ✅ Round 4：WithKnowledgeCollections 注入（KB-07） |
| P1 | OCR tesseract/docling 实现 | 仍返回 stub（KB-09） |
| ~~P1~~ | ~~Gemini ingest/query 分 task type~~ | ✅ Round 4：TaskTypeEmbedder + RETRIEVAL_QUERY（KB-10） |
| ~~P2~~ | ~~rerank chunk_index 类型断言~~ | ✅ Round 2：改为 `.(float64)` + `int(v)`（KB-12） |
| ~~P2~~ | ~~chunk index 用 metadata 而非循环 i~~ | ✅ Round 3：从 trpc Metadata 读取 MetaChunkIndex（KB-13/14） |
| ~~P2~~ | ~~异步 ingest context 传递~~ | ✅ Round 2：传递请求 ctx 到 safego.Go（KB-15） |
| ~~P2~~ | ~~KnowledgeService.chunker 死代码~~ | ✅ Round 2：字段、构造函数参数、Wire provider 全部清理（KB-16） |
| ~~P2~~ | ~~MinScore SQL 参数化~~ | ✅ Round 2：提取 `hasMinScore` 布尔变量（KB-18） |
| ~~P2~~ | ~~knowledge_search 暴露 filter_json/use_rerank~~ | ✅ Round 3：searchInput 新增两个字段（KB-19） |
| ~~P2~~ | ~~HTTP embedder timeout 配置~~ | ✅ Round 3：`KRATOS_KNOWLEDGE_EMBED_TIMEOUT_SEC` 环境变量可配（KB-11） |
| ~~P2~~ | ~~IVFFlat lists=100 写死~~ | ✅ Round 4：`ivfflatLists(dim)` 动态计算 + 环境变量覆盖（KB-20） |
| P2 | ListChunks/ReindexDocument/UpdateDocument RPC | 运维调试不便（KB-17） |
| P3 | AgenticFilter | 集成 trpc `searchfilter` |
| P3 | OCR 识别 | Tesseract/Docling 图片→文本 |
| P3 | 多租户知识库隔离 | tenant_id 分区 |
| P3 | code_search 工具 | 代码语义搜索 |
| P3 | SourceSync 增量同步 | 数据源自动增量更新 |

### 第四轮：构造函数优化 + 默认值下移（已完成）

| 修复项 | 说明 |
|--------|------|
| KnowledgeService 构造函数 8→6 参数 | 引入 `KnowledgeSearchDeps` 聚合检索依赖（Retriever/Router/Evaluator） |
| `ProvideKnowledgeSearchDeps` Wire provider | service.go ProviderSet 新增 |
| IngestDocument 默认值逻辑下移 | `IngestParams.ApplyDefaults()` 方法，Service 层不再硬编码默认值 |
| agent/knowledge_inject.go 编译验证 | 已确认无编译错误 |

### 第五轮：安全 + 数据正确性 + 功能补全（已完成）

| 修复项 | 说明 |
|--------|------|
| KB-02：上传守卫 | 32MB 大小限制 + `http.DetectContentType` MIME magic + `allowedIngestMIMEs` 白名单 |
| KB-05：embedding_model 绑定校验 | CreateCollection 时校验与当前 embedder 配置一致 |
| KB-11：embedder timeout 可配 | `KRATOS_KNOWLEDGE_EMBED_TIMEOUT_SEC` 环境变量，默认 60s |
| KB-13：chunk index 从 metadata 读取 | `trpcDocsToChunks` 优先读 `MetaChunkIndex`，回退到循环 i |
| KB-14：chunk ID 用 ChunkIndex | `fmt.Sprintf("%s-ch-%d", p.DocID, ch.ChunkIndex)` 替代循环 i |
| KB-19：knowledge_search 参数补全 | 新增 `filter_json` + `use_rerank` 字段 |

### 第六轮：架构解耦 + 搜索质量 + 运维参数（已完成）

| 修复项 | 说明 |
|--------|------|
| KB-06：Embedder 解耦 | `MemoryEmbeddingAdapter` 封装 Knowledge Embedder，Wire 绑定独立 |
| KB-07：Team KnowledgeBases 注入 | `runTeamTRPCFromInput` 注入 `input.Options.KnowledgeBases` |
| KB-10：Gemini task type 分离 | `TaskTypeEmbedder` 接口 + `embedQuery` 方法，搜索用 `RETRIEVAL_QUERY` |
| KB-20：IVFFlat lists 参数化 | `ivfflatLists(dim)` 动态计算 + `KRATOS_KNOWLEDGE_IVFFLAT_LISTS` 环境变量 |
| SKILL-P2-03：slugify 唯一 | `slugify("")` 改用 `newID()[:8]` 生成唯一后缀 |
| SKILL-P2-06：watch.Runner 窄接口 | `SkillReader`(3) + `SkillWriter`(3) 替代 `*biz.SkillUsecase` |


---

## 子模块：Knowledge Evolution Roadmap

> **版本**：2026-05-29 | **状态**：Phase 1（Advanced RAG）✅ 已实现，Phase 2（Agentic RAG）✅ 已实现
> **前置**：[37 knowledge.md](./37-knowledge.md) · [37 knowledge.design.md](./37-knowledge.design.md) · [37-knowledge-development.md](./37-knowledge-development.md)
> **学术参考**：见附录 A

---

## 一、现状评估

### 1.1 已实现能力（Naive RAG 完整管线）

| 能力 | 状态 | 代码锚点 |
|------|------|----------|
| Collection/Document/Chunk 三级数据模型 | ✅ | `internal/biz/knowledge/knowledge.go` |
| 多 Provider Embedder（OpenAI/Ollama/Gemini/HuggingFace） | ✅ | `internal/knowledge/embedder.go` |
| 多格式文档提取（PDF/DOCX/XLSX/PPTX/HTML/OCR stub） | ✅ | `internal/knowledge/document_extract.go` |
| 多分块策略（char/token/markdown/json/recursive） | ✅ | `internal/knowledge/chunker.go` + `chunk_strategy.go` |
| pgvector 向量存储 + 余弦相似度搜索 | ✅ | `internal/data/knowledge.go` |
| 可选 Rerank（topk/cohere/infinity） | ✅ | `internal/knowledge/retriever.go` |
| Agent 运行时 `knowledge_search` 工具 | ✅ | `internal/tools/knowledge/tool.go` |
| 异步入库 + WebSocket 进度推送 | ✅ | `internal/service/knowledge.go` + `useKnowledgeIngestWs.ts` |
| Collection 级别权限隔离 | ✅ | `WithKnowledgeCollections` context 限定 |
| Embedder 运行时热更新 | ✅ | `internal/service/knowledge_embedder.go` |
| 查询重写（HyDE/Decomposition/MultiQuery） | ✅ | `internal/knowledge/query_rewriter.go` |
| 混合检索（BM25+向量 RRF 融合） | ✅ | `internal/knowledge/hybrid_retriever.go` |
| BM25 全文检索（PostgreSQL ts_vector） | ✅ | `internal/data/knowledge.go` |
| 自适应检索路由（查询复杂度分类） | ✅ | `internal/knowledge/adaptive_router.go` |
| 检索质量评估（CRAG 式自校验） | ✅ | `internal/knowledge/retrieval_evaluator.go` |
| knowledge_reflect 工具（Agent 自校验） | ✅ | `internal/tools/knowledge/tool.go` |
| 跨 Collection 联邦搜索 | ✅ | `internal/knowledge/federated_retriever.go` |

### 1.2 核心局限

| # | 局限 | 影响 | 对标学术 | 状态 |
|---|------|------|----------|------|
| L1 | **单次检索**：Agent 只能被动接收 topK chunks | 无法迭代精炼检索结果 | Agentic RAG (SoK 2026) | ✅ knowledge_reflect + Plan-Then-Retrieve 已解决 |
| L2 | **无查询重写**：原始 query 直接做嵌入 | 复杂/模糊查询召回率低 | HyDE, Query Decomposition | ✅ 已解决 |
| L3 | **纯向量检索**：缺少 BM25 稀疏检索 | 专业术语精确匹配差 | Hybrid Retrieval | ✅ 已解决 |
| L4 | **无知识结构**：扁平 chunks，无层次/图谱 | 多跳推理无法支撑 | GraphRAG, CORPUS2SKILL | ❌ Phase 3 |
| L5 | **无自适应检索**：所有查询走同一管线 | 简单查询浪费资源，复杂查询检索不足 | Adaptive RAG | ✅ 已解决 |
| L6 | **无检索质量评估**：检索结果直接返回 | 低质量结果无法自纠 | CRAG, Self-RAG | ✅ 已解决 |
| L7 | **无跨 Collection 联邦搜索** | 多知识源协同检索受限 | Federated Retrieval | ✅ 已解决 |
| L8 | **Chunk 粒度固定** | 同 Collection 内文档共享分块参数 | Granularity-Aware Retrieval | ❌ 未排期 |
| L9 | **无技能知识**：仅存储文档，不存储操作流程 | Agent 无法复用"如何做"知识 | SkillX, CORPUS2SKILL | ❌ Phase 4 |

### 1.3 RAG 成熟度定位

```
Naive RAG (2023)    Advanced RAG (2024)    Agentic RAG (2025-2026)
     │                    │                       │
     ▼                    ▼                       ▼
  检索→生成           查询重写+Rerank         Agent 主动规划
  单向管线            Self-RAG/CRAG           迭代检索+自校验
  固定 topK           混合检索                多源融合+图推理
     │                    │                       │
                          │                  ▲
                     当前位置 ◄─── Phase 1 ✅  │
                     Phase 1 ✅          Phase 2 🔄 部分实现
```

**当前 Aranea 知识库处于 Agentic RAG 阶段**（已具备查询重写、混合检索、自适应路由、检索评估、联邦搜索、Agent 自校验工具、Plan-Then-Retrieve），正向 GraphRAG 阶段演进（知识图谱构建待实现）。

---

## 二、知识库在 Aranea 中的定位

### 2.1 Agent 认知三角

```
┌─────────────────────────────────────────────────────┐
│                  Aranea Agent 运行时                  │
│                                                       │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────┐   │
│  │  Memory   │  │  Tools   │  │   Knowledge      │   │
│  │ (会话记忆) │  │ (工具集)  │  │   (认知基础设施)  │   │
│  │           │  │          │  │                   │   │
│  │ • 短期记忆 │  │ • 内置工具│  │ • 文档知识(What) │   │
│  │ • 长期记忆 │  │ • MCP工具 │  │ • 技能知识(How)  │   │
│  │ • 事实记忆 │  │ • 自定义  │  │ • 关系知识(Who)  │   │
│  └──────────┘  └──────────┘  └──────────────────┘   │
│                                                       │
│              ↑ 三者协同，构成 Agent 认知三角 ↑          │
└─────────────────────────────────────────────────────┘
```

- **Memory**：Agent 自身经验的内化（"我做过什么"）
- **Knowledge**：外部知识的摄入和组织（"世界是什么样"）
- **Skill**：从经验/文档中蒸馏的可复用操作流程（"该怎么做"）

### 2.2 三层知识模型

| 层 | 类型 | 存储形式 | 检索方式 | 对应当前实现 |
|----|------|----------|----------|-------------|
| **L1 文档知识** | "知道什么" | Chunk + Embedding | 向量相似度 | ✅ 已实现 |
| **L2 关系知识** | "谁关联谁" | 实体 + 关系（知识图谱） | 图遍历 + 子图检索 | ❌ 未实现 |
| **L3 技能知识** | "如何做" | 技能描述 + 执行轨迹 | 语义匹配 + 层次导航 | ❌ 未实现 |

### 2.3 与现有模块的关系

```
Memory (记忆)                    Knowledge (知识)
├── 短期：当前会话上下文           ├── L1：文档向量检索
├── 长期：用户偏好/历史摘要        ├── L2：实体关系图谱
└── 事实：结构化事实向量           └── L3：可复用技能库
     ↑                                ↑
     └── 已有 pgvector 存储 ──────────┘── 共用向量基础设施
```

---

## 三、四阶段演进路线

### Phase 1：Advanced RAG — 夯实检索质量 ✅ 已实现

> 目标：从 Naive RAG 升级到 Advanced RAG，提升检索精度和召回率。
> 预期收益：检索精度提升 20-30%
> **实现日期**：2026-05-28

#### 1.1 查询重写与分解

**现状**：用户原始 query 直接做 embedding，无任何优化。

**方案**：在 `Retriever.Search` 前插入查询理解层。

```go
// internal/knowledge/query_rewriter.go（新增）

type QueryRewriter interface {
    Rewrite(ctx context.Context, query string) ([]string, error)
}

// HyDE：先用 LLM 生成假设性回答，再用假设回答做 embedding 检索
type HyDERewriter struct {
    llm LLM
}

// Query Decomposition：将复杂查询分解为子问题，分别检索后合并
type DecompositionRewriter struct {
    llm LLM
}

// Multi-Query：生成多个查询变体，合并检索结果（RRF 倒排融合）
type MultiQueryRewriter struct {
    llm LLM
}
```

**Proto 扩展**：

```protobuf
message SearchRequest {
    // ... 现有字段
    bool enable_query_rewrite = 10;
    string rewrite_strategy = 11; // hyde | decomposition | multi_query
}
```

**架构位置**：`internal/knowledge/query_rewriter.go`（新增），在 Retriever.Search 前调用。

#### 1.2 混合检索

**现状**：纯 pgvector 余弦相似度搜索。

**方案**：增加 BM25 稀疏检索路径，与向量检索融合。

```go
// internal/knowledge/hybrid_retriever.go（新增）

type HybridRetriever struct {
    dense  DenseRetriever
    sparse SparseRetriever
    fusion FusionStrategy
}

type FusionStrategy interface {
    Merge(dense, sparse []ScoredChunk) []ScoredChunk
}

// RRF 融合：Reciprocal Rank Fusion
type RRFFusion struct {
    K int
}
```

- **BM25**：PostgreSQL `ts_vector` 全文检索，与 pgvector 向量检索并行
- **RRF 融合**：合并两路结果，兼顾语义相似度和关键词精确匹配
- **架构位置**：`internal/knowledge/hybrid_retriever.go`（新增），data 层增加 `SearchChunksBM25`

**Data 层扩展**：

```go
// internal/data/knowledge.go 扩展

func (r *knowledgeRepo) SearchChunksBM25(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
    // PostgreSQL ts_vector 全文检索
    // SELECT ... FROM knowledge_chunks
    // WHERE collection_id = $1 AND to_tsvector('simple', content) @@ plainto_tsquery('simple', $2)
    // ORDER BY ts_rank DESC LIMIT $3
}
```

**Proto 扩展**：

```protobuf
message SearchRequest {
    // ... 现有字段
    bool enable_hybrid_search = 12;
}
```

#### 1.3 自适应检索

**现状**：所有查询走同一管线。

**方案**：增加查询复杂度分类器，动态选择检索策略。

```go
// internal/knowledge/adaptive_router.go（新增）

type QueryComplexity int

const (
    QuerySimple    QueryComplexity = iota
    QueryModerate
    QueryComplex
)

type AdaptiveRouter struct {
    classifier QueryClassifier
    simple     Retriever   // 纯向量检索
    advanced   Retriever   // 混合检索 + Rerank
}
```

- 简单查询：直接向量检索（低延迟 ~50ms）
- 中等查询：向量 + BM25 混合检索
- 复杂查询：查询分解 + 多轮迭代检索 + Rerank

**架构位置**：`internal/knowledge/adaptive_router.go`（新增）

#### 1.4 检索质量评估（CRAG 思路）

**现状**：检索结果直接返回，无质量评估。

**方案**：在检索后增加评估环节，不满足阈值时触发补充检索。

```go
// internal/knowledge/retrieval_evaluator.go（新增）

type RetrievalEvaluator interface {
    Evaluate(ctx context.Context, query string, chunks []biz.KnowledgeChunk) RetrievalAssessment
}

type RetrievalAssessment struct {
    Sufficient      bool
    Confidence      float32
    SupplementQuery string
}

type LLMEvaluator struct {
    llm LLM
}
```

- 评估维度：相关性、完整性、一致性
- 不满足时：生成补充查询，触发二次检索
- 架构位置：`internal/knowledge/retrieval_evaluator.go`（新增）

---

### Phase 2：Agentic RAG — 让 Agent 主动检索 ✅ 已实现

> 目标：从被动检索升级为 Agent 主动规划、迭代检索、自校验。
> 预期收益：复杂查询检索质量提升 40-50%
> **实现日期**：2026-05-29

#### 2.1 多轮迭代检索工具

**现状**：`knowledge_search` 是单次调用工具。

**方案**：升级为支持多轮对话的迭代检索工具集。

```go
// internal/tools/knowledge/tool.go（已实现 knowledge_reflect）

// knowledge_reflect：让 Agent 评估当前检索结果是否充分
func NewReflectTool() trpctool.CallableTool { ... }

// knowledge_search 保持兼容，支持 AdaptiveRouter 自动路由
func NewSearchTool() trpctool.CallableTool { ... }
```

**迭代检索流程**（已实现）：

```
Agent 调用 knowledge_search(query, collection_id)
  → 返回 topK chunks（经 AdaptiveRouter 自动路由）
Agent 调用 knowledge_reflect(query, collection_ids)
  → FederatedRetriever 跨 Collection 搜索
  → RetrievalEvaluator 评估质量
  → 返回评估：sufficient=false, supplement_query="..."
Agent 调用 knowledge_search(supplement_query, collection_id)
  → 返回补充 chunks
Agent 调用 knowledge_reflect(query, collection_ids)
  → 返回评估：sufficient=true
Agent 生成最终回答
```

**架构位置**：`internal/tools/knowledge/tool.go`（已实现）

#### 2.2 跨 Collection 联邦搜索 ✅ 已实现

**现状**：每次搜索限定单一 Collection。

**方案**：增加联邦搜索能力，支持跨 Collection 检索。

```go
// internal/knowledge/federated_retriever.go（已实现）

type FederatedRetriever struct {
    router    *AdaptiveRouter
    retriever *Retriever
}

func NewFederatedRetriever(router *AdaptiveRouter, retriever *Retriever) *FederatedRetriever
func (f *FederatedRetriever) Search(ctx context.Context, collectionIDs []string, q biz.KnowledgeSearchQuery, rewriteResult *QueryRewriteResult, modeOverride HybridSearchMode) ([]biz.KnowledgeChunk, error)
```

- **Broadcast**（已实现）：向所有指定 Collection 并行广播查询，结果合并去重
- **Route**（待实现）：先路由到最相关的 Collection，再检索

**架构位置**：`internal/knowledge/federated_retriever.go`（已实现）

#### 2.3 Plan-Then-Retrieve 模式 ✅ 已实现

**现状**：Agent 不知道有哪些知识库可用，无法规划检索路径。

**方案**：在 Agent 系统提示中注入 Collection 摘要，让 Agent 先规划再检索。

```go
// internal/agent/knowledge_inject.go（已实现）

func newKnowledgeCueBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback
func buildKnowledgeCue(ctx context.Context, uc *biz.KnowledgeUsecase) string
```

- BeforeModel 钩子（优先级 6），在每次模型调用前注入 Collection 摘要
- 仅注入 Agent 关联的 Collection（通过 `KnowledgeCollectionsFromContext` 读取 scoped IDs）
- 摘要包含：Collection 名称、ID、描述、文档数、块数 + 搜索策略提示
- 截断保护：单个描述 ≤120 字符，总摘要 ≤1500 字符，最多 10 个 Collection
- KnowledgeUsecase 为 nil 或无 Collection 时自动跳过

---

### Phase 3：GraphRAG — 知识图谱增强

> 目标：引入知识图谱层，支撑多跳推理和实体关系查询。
> 预期收益：多跳/实体关系查询准确率提升 60-70%

#### 3.1 知识图谱构建

**方案**：在文档入库时，增加实体和关系提取步骤。

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
-- docs/sql/knowledge_graph.sql（新增）

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

#### 3.2 图增强检索

**方案**：向量检索 + 图遍历融合。

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

#### 3.3 图查询工具

**方案**：为 Agent 增加图查询工具。

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

**架构位置**：`internal/tools/knowledge/graph_tool.go`（新增）

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

---

### Phase 4：Skill Knowledge — 技能知识库

> 目标：从文档知识库演进为技能知识库，与 Aranea 的 Skill 体系深度融合。
> 预期收益：Agent 任务执行效率提升 80%+（通过技能复用减少重复推理）

#### 4.1 技能知识库构建

**方案**：借鉴 SkillX 和 CORPUS2SKILL，构建三层技能层次。

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

#### 4.2 知识导航工具

**方案**：借鉴 CORPUS2SKILL，增加知识导航工具。

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

- Agent 不再"盲目检索"，而是"有地图地导航"
- 架构位置：`internal/tools/knowledge/navigate_tool.go`（新增）

#### 4.3 技能蒸馏管线

**方案**：从 Agent 执行轨迹中自动提取技能。

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

## 四、实施优先级与依赖关系

### 4.1 优先级排序

| 优先级 | Phase | 核心价值 | 预期收益 | 依赖 |
|--------|-------|----------|----------|------|
| **P0** | Phase 1.1-1.2 | 查询重写 + 混合检索 | 检索精度提升 20-30% | 无 |
| **P0** | Phase 2.1 | 多轮迭代检索工具 | Agent 检索能力质变 | Phase 1 |
| **P1** | Phase 1.3-1.4 | 自适应检索 + 质量评估 | 检索效率 + 可靠性 | Phase 1.1 |
| **P1** | Phase 2.2-2.3 | 联邦搜索 + Plan-Retrieve | 多知识源协同 | Phase 2.1 |
| **P2** | Phase 3 | GraphRAG | 多跳推理能力 | Phase 1 |
| **P3** | Phase 4 | Skill Knowledge | 技能复用 + 导航 | Phase 2+3 |

### 4.2 依赖关系图

```
Phase 1.1 查询重写 ─────┐
Phase 1.2 混合检索 ─────┤
                         ├──→ Phase 2.1 迭代检索 ──→ Phase 2.2 联邦搜索
Phase 1.3 自适应检索 ────┤                         Phase 2.3 Plan-Retrieve
Phase 1.4 质量评估 ──────┘
                              │
                              ├──→ Phase 3 GraphRAG ──→ Phase 4 Skill Knowledge
                              │
                              └──→ Phase 4 Skill Knowledge（部分可独立推进）
```

### 4.3 架构约束（遵循项目铁律）

| # | 约束 | 说明 |
|---|------|------|
| 1 | 依赖方向 | 所有新增代码遵循 `biz → data` 单向依赖，`internal/knowledge` 不 import `pkg/trpc-agent-go` |
| 2 | 框架真相源 | 向量存储、Embedder、Reranker 等框架能力优先使用 `pkg/trpc-agent-go/knowledge/` 已有实现 |
| 3 | 工具注册 | 新工具通过 `internal/tools/` 的 Registry 注册，走 `ToolKeyKnowledge*` 常量 |
| 4 | Wire 注入 | 新依赖通过 Wire ProviderSet 注入，不手动 new |
| 5 | 并发安全 | 所有 `go func()` 走 `pkg/safego` |
| 6 | 日志统一 | 使用 `internal/event` 的 `FlowLog`，禁止 `log/slog` |
| 7 | Proto 契约 | 新增 API 先写 proto，`make api` 生成，不手写 |
| 8 | 向后兼容 | 每个 Phase 向后兼容，不破坏现有 API |

---

## 五、演进总览

| 维度 | Phase 1 ✅ | Phase 2 ✅ | Phase 3 | Phase 4 |
|------|-----------|---------|---------|---------|
| 检索模式 | 混合检索+查询重写 | 多轮迭代+自校验+Plan-Then-Retrieve | 图+向量融合 | 层次导航 |
| Agent 角色 | 被动消费者 | 主动检索者 | 主动推理者 | 主动导航者 |
| 知识结构 | 扁平 chunks | 扁平 chunks | 实体关系图谱 | 技能层次树 |
| 检索质量 | +20-30% | +40-50% | +60-70% | +80%+ |
| 复杂查询 | 部分 | ✅ | ✅ | ✅ |
| 多跳推理 | ❌ | ❌ | ✅ | ✅ |
| 技能复用 | ❌ | ❌ | ❌ | ✅ |

---

## 附录 A：学术参考

| 论文 | 年份 | 核心贡献 | 对 Aranea 的启示 |
|------|------|----------|-----------------|
| CORPUS2SKILL (arXiv 2604.14572) | 2026.04 | 文档语料→层次化技能目录，Agent 主动导航 | Phase 4 知识导航工具设计 |
| SoK: Agentic RAG (arXiv 2603.07379) | 2026.03 | Agentic RAG 形式化分类体系，6 种设计模式 | Phase 2 迭代检索架构设计 |
| MMOA-RAG (NeurIPS 2025) | 2025 | 多模块联合优化，RAG 管线各组件视为多 Agent | 未来 RL 联合调优方向 |
| Agentic GraphRAG (arXiv 2605.18770) | 2026.04 | Neo4j 知识图谱 + 分析型 Agent | Phase 3 GraphRAG 架构参考 |
| SkillX (arXiv 2604.04804) | 2026.04 | 三层技能知识库自动构建（Planning/Functional/Atomic） | Phase 4 技能知识库设计 |
| RAG-Reasoning Survey (EMNLP 2025) | 2025 | 检索与推理双向增强 | 知识库与 Agent 推理链深度耦合 |
| Is Agentic RAG worth it? (arXiv 2601.07711) | 2026.01 | Enhanced RAG vs Agentic RAG 实验对比 | Phase 1-2 成本/收益权衡参考 |
| LazyGraphRAG (Microsoft) | 2025 | 索引成本降至 0.1%，查询成本降 700x | Phase 3 图构建成本优化参考 |
| GraphRAG 2.0 (Microsoft) | 2025 | 四模式检索全家福 + 增量更新 | Phase 3 图检索模式设计参考 |
| Agent Skills Survey (arXiv 2605.07358) | 2026.05 | Agent 技能全生命周期分类（表示/获取/检索/演化） | Phase 4 技能生命周期管理 |

---

## 附录 B：新增文件清单

### Phase 1 ✅ 已实现

| 文件 | 类型 | 说明 |
|------|------|------|
| `internal/knowledge/query_rewriter.go` | 新增 | 查询重写接口 + HyDE/Decomposition/MultiQuery 实现 |
| `internal/knowledge/hybrid_retriever.go` | 新增 | 混合检索器（Dense+Sparse+RRF 融合） |
| `internal/knowledge/adaptive_router.go` | 新增 | 自适应检索路由器 |
| `internal/knowledge/retrieval_evaluator.go` | 新增 | 检索质量评估器 |
| `internal/knowledge/query_rewriter_test.go` | 新增 | 查询重写单测 |
| `internal/knowledge/hybrid_retriever_test.go` | 新增 | 混合检索单测 |
| `internal/knowledge/adaptive_router_test.go` | 新增 | 自适应路由单测 |
| `internal/service/knowledge_advanced.go` | 新增 | Service 层 Wire 工厂（4 个新 Provider） |
| `internal/biz/knowledge/knowledge.go` | 修改 | 新增 `SparseSearcher` 接口 |
| `internal/biz/knowledge.go` | 修改 | 导出 `KnowledgeSparseSearcher` 类型别名 |
| `internal/data/knowledge.go` | 修改 | 新增 `SearchChunksBM25` + GIN tsvector 索引 |
| `internal/data/data.go` | 修改 | 新增 `NewKnowledgeSparseSearcherFromData` Provider |
| `internal/service/knowledge.go` | 修改 | Search 方法集成 AdaptiveRouter + RetrievalEvaluator |
| `internal/service/service.go` | 修改 | ProviderSet 增加 4 个新 Provider |
| `api/kratos/knowledge/v1/knowledge.proto` | 修改 | SearchRequest 增加 `rewrite_strategy` + `hybrid_search` 字段 |

### Phase 2 ✅ 已实现

| 文件 | 类型 | 说明 |
|------|------|------|
| `internal/knowledge/federated_retriever.go` | ✅ 新增 | 联邦检索器（Broadcast + Route 策略） |
| `internal/knowledge/federated_retriever_test.go` | ✅ 新增 | 联邦检索单测（含 Route 策略测试） |
| `internal/tools/knowledge/tool.go` | ✅ 修改 | knowledge_reflect 工具 + context 注入 + KnowledgeCollectionsFromContext 导出 |
| `internal/biz/tool/tool.go` | ✅ 修改 | ToolKeyKnowledgeReflect 常量 |
| `internal/biz/agent_mcp_effective.go` | ✅ 修改 | ToolKeyKnowledgeReflect 导出 |
| `internal/biz/tool/tool_catalog_runtime.go` | ✅ 修改 | KnowledgeReflect 加入 sessionBoundToolKeys |
| `internal/tools/trpc/effective_config.go` | ✅ 修改 | KnowledgeReflect 配置映射 |
| `internal/tools/trpc/toolsets.go` | ✅ 修改 | KnowledgeReflect 装配 |
| `internal/agent/tool_assembly.go` | ✅ 修改 | KnowledgeReflect 开关 |
| `internal/agent/knowledge_inject.go` | ✅ 新增 | Plan-Then-Retrieve BeforeModel 钩子 |
| `internal/agent/builder_deps.go` | ✅ 修改 | KnowledgeUsecase 加入 TRPCBuilderDeps |
| `internal/agent/callback_chain.go` | ✅ 修改 | 注册 knowledgeCueBeforeHook |
| `internal/service/knowledge_advanced.go` | ✅ 修改 | NewFederatedRetrieverWithMeta 工厂 |
| `internal/service/chat_orchestrator.go` | ✅ 修改 | RuntimeTooling 增加 KnowledgeUC |
| `internal/service/chat_orchestrator_turn.go` | ✅ 修改 | KnowledgeUsecase 传入 BuilderDeps |
| `internal/service/a2a_endpoint.go` | ✅ 修改 | A2A endpoint KnowledgeUsecase 传入 |
| `internal/team/runner.go` | ✅ 修改 | Team Runner 增加 FederatedRetriever/Evaluator |
| `internal/team/runner_team_trpc.go` | ✅ 修改 | Team context 注入 |
| `internal/data/builtin_tools_seed.go` | ✅ 修改 | knowledge_reflect 种子 |
| `cmd/admin/wire.go` | ✅ 修改 | provideRuntimeTooling 增加 KnowledgeUC |

### Phase 3

| 文件 | 类型 | 说明 |
|------|------|------|
| `internal/biz/knowledge/graph.go` | 新增 | Entity/Relation 领域模型 + GraphRepo 接口 |
| `internal/data/knowledge_graph.go` | 新增 | GraphRepo PostgreSQL 实现 |
| `internal/knowledge/graph_augmented_retriever.go` | 新增 | 图增强检索器 |
| `internal/knowledge/entity_extractor.go` | 新增 | LLM 实体/关系提取 |
| `internal/tools/knowledge/graph_tool.go` | 新增 | 图查询工具 |
| `docs/sql/knowledge_graph.sql` | 新增 | 知识图谱数据库 Schema |

### Phase 4

| 文件 | 类型 | 说明 |
|------|------|------|
| `internal/biz/knowledge/skill_knowledge.go` | 新增 | 技能知识领域模型 |
| `internal/knowledge/skill_distiller.go` | 新增 | 技能蒸馏管线 |
| `internal/tools/knowledge/navigate_tool.go` | 新增 | 知识导航工具 |
| `internal/tools/knowledge/drill_tool.go` | 新增 | 知识钻入工具 |
