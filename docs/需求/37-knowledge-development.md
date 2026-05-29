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
