# Knowledge 知识库 — 开发计划

> **版本**：2026-07-21 | **状态**：✅ Phase 1-9 已完成（Phase 9 多模态入库：图片经 VisionExtractor 异步提取为 MD；真实视觉模型端到端待环境就位后复验）；✅ Phase 11（US-14 免选择知识库）已完成；Phase 10（GraphRAG 旁路）可选
> **2026-07-25 新增**：§子模块 Vault 重设计 Phase 计划（P1~P6，含 P4a/P4b 拆分）——Vault 重设计经评审有条件通过，R-1~R-6 已合入设计文档 §V6，待启动实施。
> **2026-08-07 新增**：§子模块 图谱深空版与实体治理（G5）Phase 计划（G5-A~G5-G）——调研评审通过（`docs/reports/2026-08-07-research-knowledge-graph-oss.md`），V12.8 设计已合入；**2026-08-08：G5-F 实体治理后端（B9~B12）✅ 完成**（详见该节 As-built）。
> **2026-08-08 更新**：G5-A~G5-E ✅ 完成；**G5-C 渲染层 v2 重写**（GPU 位置纹理管线替代 InstancedMesh/Raycaster——万级卡顿修复 + 全场景降亮度 + Obsidian 柔光点/细直线/流动光脉冲/HUD 瞄准具科幻视觉 + 自适应画质三档 governor），详见 G5-C As-built；G5-G 🟡（移除清单 G-2 ✅、治理前端 G-1 ✅、全量静态验证与浏览器运行时复验 G-4 ✅——含复验发现的标签可见性修复；性能基准 G-3 📋）。
> **2026-08-08 新增**：§子模块 双模块级知识内核（SP1）Phase 计划（SP1-A~SP1-I）——双模统一架构经评审通过（用户裁决：块级双链完整粒度；批准落档三件套），S1~S11 设计已合入设计文档，待启动实施。
> **2026-08-08 深入评审**：[SP1 三件套评审报告](../reports/2026-08-08-review-sp1-knowledge-blueprint.md)——B-1~B-4 阻断项修订全部合入下列任务；G1~G8 前瞻提案全部采纳，产品路线扩展为 SP1~SP7（见 SP1 节末预告表）。
> **2026-08-10 检索链路事故根修（TDD，全绿）**：① **向量维度对账**——`EnsureKnowledgeSchema` 尾部新增 `reconcileEmbeddingDim`（`data/knowledge.go`）：embedder 换模型后 PG 列 typmod 不随 `CREATE TABLE IF NOT EXISTS` 修正，新维度插入全被拒（"expected N dimensions"）而应用层按 `collections.dim` 校验反过，语义检索全灭极难定位；对账幂等四步（向量置 NULL + 文档 hash 重置回 pending + 语义层集合 dim 快照同步 + ALTER 列重建 ivfflat），vault 文档下轮 sync 自愈，UX验证库已愈合复验；② **无语义层集合前置降级**（§V5 降级矩阵 #3 落地）——`collectionLacksSemanticLayer`（`search_helpers.go`）判定 `embedding_model` 空时 Retriever/HybridRetriever 直接降级 BM25，消除 dense 恒空静默；③ **中文短查询词法失效根修**——trigram 路 `similarity(content,q)`+`%` 对 2-4 字中文查询相似度稀释永低于阈值，改 `word_similarity(q,content)`+`%>`（`data/knowledge.go`）；新增 `knowledge_dim_reconcile_test.go` + `TestKnowledgeRepo_SearchChunksBM25_ChineseShortQuery` 等回归。详见设计文档 §4.2 维度对账 / §4.3 trigram 选型注 / §5.4、§5.6 降级注。
> **2026-08-10 新增**：§子模块 编辑器与笔记体验（SP2）Phase 计划（SP2-1~SP2-9）——用户裁决：Obsidian 级笔记能力、UI 推翻 Tab 管理后台为深空液态玻璃工作台、编辑器选型 CodeMirror 6 Live Preview、A1+A2 一轮交付、纯前端重构后端零改动；设计已合入 [37-knowledge.design.md §SP2](./37-knowledge.design.md#sp2-编辑器与笔记体验深空液态玻璃工作台2026-08-10)。
> **需求**：[37-knowledge.md](./37-knowledge.md) · **设计**：[37-knowledge.design.md](./37-knowledge.design.md)

---

## 1. 模块定位

Knowledge 知识库：管理 Agent 的知识来源，支持文档上传、分块、向量化、检索和注入。

**代码锚点**：
- `api/kratos/knowledge/v1/knowledge.proto` — Knowledge CRUD + Search RPC（含 `rewrite_strategy` + `hybrid_search`）
- `internal/biz/knowledge.go` — 类型别名转发（KnowledgeRepo = knowledge.Repo 等 + ApplyKnowledgeEmbedPatch 等）
- `internal/biz/knowledge/knowledge.go` — 领域模型 + Repo/Usecase 接口（子接口拆分）+ EmbedSetting patch 合并
- `internal/data/knowledge.go` — KnowledgeRepo（PostgreSQL + pgvector + BM25 双路）
- `internal/service/knowledge.go` — KnowledgeService（KnowledgeSearchDeps 聚合）
- `internal/service/knowledge_advanced.go` — Advanced RAG Wire 工厂（6 个 Provider）
- `internal/knowledge/chunker.go` — 文档分块（char/token）
- `internal/knowledge/embedder.go` — 向量化（四 provider + EmbedBatch）
- `internal/knowledge/ingest.go` — 分块+向量化流水线（IngestParams.ApplyDefaults）
- `internal/knowledge/retriever.go` — 检索器（含 TaskTypeEmbedder）
- `internal/knowledge/query_rewriter.go` — 查询重写（HyDE/Decomposition/MultiQuery）
- `internal/knowledge/hybrid_retriever.go` — 混合检索（Dense+Sparse+RRF）
- `internal/knowledge/adaptive_router.go` — 自适应检索路由
- `internal/knowledge/retrieval_evaluator.go` — 检索质量评估（CRAG）
- `internal/knowledge/federated_retriever.go` — 跨 Collection 联邦搜索
- `internal/knowledge/search_helpers.go` — 检索评估辅助
- `internal/knowledge/llm_resolver.go` — LLM 模型解析
- `internal/knowledge/ocr.go` — OCR 提供者接口（⛔ 已废弃，由 Phase 9 VisionExtractor 取代）
- `internal/knowledge/html_text.go` — HTML 文本剥离
- `internal/knowledge/chunk_strategy.go` — trpc 高级分块桥接
- `internal/knowledge/document_extract.go` — PDF/DOCX/HTML 文本提取
- `internal/knowledge/readers_import.go` — trpc reader 注册
- `internal/knowledge/reranker_factory.go` — env Reranker（KN-01）
- `internal/service/knowledge_embedder.go` — Embedder Wire（env + DB）
- `api/kratos/system_setting/v1/system_setting.proto` — `KnowledgeEmbedSettings`
- `internal/service/knowledge_retriever.go` — Retriever Wire
- `internal/tools/knowledge/tool.go` — knowledge_search + knowledge_reflect 工具
- `internal/agent/knowledge_inject.go` — Plan-Then-Retrieve BeforeModel 钩子
- `internal/agent/tool_assembly.go` — KnowledgeSearch/KnowledgeReflect 装配
- `web/src/features/knowledge/api.ts` — 前端 API
- `web/src/stores/knowledge/index.ts` — 前端 Store
- `web/src/features/knowledge/useKnowledgeIngestWs.ts` — 入库 WS 进度

**Vault 重设计已新增（P1，✅ 已完成）**：
- `internal/biz/knowledge/vault_filer.go` — Vault 文件写唯一出口（sanitize/原子写/覆盖备份/回收站）
- `internal/biz/knowledge/vault_usecase.go` — Vault 用例（NormalizeRootPath/CreateVault/同步入口）
- `internal/biz/knowledge/sync_engine.go` — 单向扫描器（Scan + DiffSnapshots，mtime 预筛）
- `internal/knowledge/vault_sync.go` — VaultSyncApplier（事件 → chunk → 可选 embed → 派生索引）
- `internal/knowledge/vault_sync_runner.go` — VaultSyncRunner 轮询循环（prev 自 DB 重建；Watcher 接口预留）
- `internal/knowledge/vault_reindex.go` — ReindexVault 一键重建派生索引（P1-4）

**Phase 8 已新增（✅ 已完成）**：
- `internal/knowledge/extractor.go` — Extractor 接口 + ExtractorRegistry 路由 + TextExtractor
- `internal/knowledge/markdown_organizer.go` — LLM 整理为 Markdown（30s 超时，失败降级原文本）
- `internal/service/knowledge_advanced.go` — MarkdownOrganizer Wire 工厂（`NewKnowledgeMarkdownOrganizer`）
- `web/src/components/knowledge/KnowledgeDropZone.vue` — 拖拽上传区
- `web/src/components/knowledge/KnowledgeUploadQueue.vue` — 批量上传队列
- `web/src/components/knowledge/KnowledgeDocPreviewDialog.vue` — MD 全文预览

**Phase 9 已落地（✅ 2026-07-21）**：
- `internal/knowledge/vision_extractor.go` — 多模态 LLM 图片理解 → MD（60s 超时，无视觉模型时返回明确错误）
- `internal/knowledge/asset_store.go` — 原图留存（`KRATOS_KNOWLEDGE_ASSET_DIR` env > `./data/knowledge_assets`）

**G3 已新增（✅ 2026-07-30）**：
- `web/src/components/knowledge/KnowledgeMoveConflictDialog.vue` — 拖拽移动同名冲突策略弹窗（覆盖/保留两份/取消）

**G4 已新增（✅ 2026-07-30）**：
- `internal/biz/knowledge/graph.go` — `ListCollectionGraph` 单库全量图谱（节点过滤/边过滤/悬空边剔除/连接度）
- `internal/data/knowledge_links.go` — `ListCollectionLinks` 库级关联读取（B8 数据源）
- `web/src/features/knowledge/graphUi.ts` — 图谱纯函数（边/节点配色、连接度映射、渲染裁剪、排序/过滤/一跳邻居、径向 containment 力）
- `web/src/features/knowledge/useKnowledgeGraph.ts` — 图谱编排 composable（三维过滤/选中聚焦/范围迷你树）
- `web/src/components/knowledge/KnowledgeGraph3D.vue` — 图谱主面板（左 3D 画布 + 右操作台）
- `web/src/components/knowledge/KnowledgeGraphCanvas.vue` — 3d-force-graph 画布封装
- `web/src/components/knowledge/KnowledgeScopePicker.vue` — 搜索范围选择器（搜索框/图谱操作台复用）
- `web/src/components/knowledge/KnowledgeSearchPanel.vue` — ⛔ 已删除（检索 Tab 由图谱 Tab 取代）

**G5 计划新增（📋 待启动，设计 §V12.8）**：
- `web/src/features/knowledge/graph3d/` — 纯 TS 引擎（零 Vue/three 依赖）：`model.ts`（SoA 图模型 + 确定性播种）、`octree.ts`（typed-array 八叉树）、`forces.ts`（5 力物理引擎）、`protocol.ts`（Worker 协议）、`tiering.ts`（节点三层分级）、`palette.ts`（分组调色板）、`particleMath.ts`（粒子流纯数学）、`physics.worker.ts`（物理 Worker）
- `web/src/components/knowledge/graph3d/KnowledgeGraph3DCanvas.vue` + `render/`（NodeLayer/EdgeLayer/ParticleLayer/BackdropLayer/BloomPipeline/LabelLayer/Picker）— Vue/three 命令式壳
- 后端实体治理：DDL 迁移（`knowledge_entities.name_norm` + `knowledge_entity_aliases` 表）+ L3 Go 数据迁移（回填 + 冲突组合并）、`NormalizeEntityName`、`MergeKnowledgeEntities`/`ListEntityMergeSuggestions` RPC、`ReplaceDocEntities` 归一化接线、流程日志 step `knowledge.entity.merge`
- **G5 移除**：`3d-force-graph` 依赖（`three` 保留）、`KnowledgeGraphCanvas.vue`、`graphUi.ts graphContainmentForce`（自研引擎向心力 + maxStep 钳制原生覆盖）

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
| BM25 全文检索 | ✅ | `data/knowledge.go` `SearchChunksBM25`（tsvector + pg_trgm 双路） + GIN 索引 |
| 自适应检索路由 | ✅ | `adaptive_router.go`（查询复杂度分类） |
| 检索质量评估 | ✅ | `retrieval_evaluator.go`（CRAG 式自校验） |
| 跨 Collection 联邦搜索 | ✅ | `federated_retriever.go`（并行广播 + Route 策略） |
| Plan-Then-Retrieve | ✅ | `agent/knowledge_inject.go`（BeforeModel 钩子注入 Collection 摘要） |
| 联邦搜索 Route 策略 | ✅ | `federated_retriever.go`（`SearchWithOptions` + `routeCollections`） |
| AgenticFilter | ❌ | 未实现 |
| OCR / Extractor | ✅ Phase 8/9 | 2026-07-20 裁决：多模态 LLM 视觉路线取代 tesseract/docling；统一 Extractor 抽象（设计 §5.2/§9.4）。TextExtractor + ExtractorRegistry（`extractor.go`）+ VisionExtractor（`vision_extractor.go`）均已落地 |
| 拖拽批量上传 | ✅ | KnowledgeDropZone/UploadQueue 已实现，多文件逐任务状态展示 |
| 整理为 Markdown | ✅ | MarkdownOrganizer 已落地（设计 §5.2b），30s 超时 + 失败降级原文本 |
| content_text 预览 | ✅ | Schema 加列 + GetDocumentContent RPC + KnowledgeDocPreviewDialog 已实现 |
| OOXML 上传守卫 | ✅ 已修复 | `resolveIngestMIMEAllowed` 声明 MIME + 扩展名二次判定，DOCX/XLSX/PPTX 不再被 `application/zip` 误拒（设计 §7.2） |
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

### 3.2 功能扩展（P2）— 已完成

| 编号 | 差距 | 优先级 | 说明 | 状态 |
|------|------|--------|------|------|
| G5 | Markdown 按标题分块 | P2 | 集成 trpc `chunking/markdown.go` | ✅ |
| G6 | JSON 结构分块 | P2 | 集成 trpc `chunking/json.go` | ✅ |
| G7 | 递归分块 | P2 | 集成 trpc `chunking/recursive.go` | ✅ |
| G8 | PDF/Word/HTML 文档解析 | P2 | 集成 trpc `document/reader/` 或 Extractor | ✅ |
| G9 | 本地 Embedding 模型 | P2 | Gemini/HuggingFace embedder | ✅ |
| G10 | EmbedBatch 批量 API | P2 | 减少逐条 embed HTTP 往返 | ✅ |

### 3.3 超越层（P3）

| 编号 | 差距 | 优先级 | 说明 |
|------|------|--------|------|
| G11 | AgenticFilter | P3 | LLM 动态生成过滤条件 |
| G12 | OCR 识别 | — | 2026-07-20 并入 G16/G17：多模态 LLM 视觉路线，`ocr.go` stub 废弃 |
| G13 | 多租户隔离 | P3 | tenant_id 分区 |
| G14 | code_search 工具 | P3 | 代码语义搜索 |
| G15 | SourceSync 增量同步 | P3 | 数据源自动增量更新 |

### 3.4 统一摄取管线（2026-07-20 设计；Phase 8/9 已落地）

| 编号 | 差距 | 优先级 | 说明 | Phase |
|------|------|--------|------|-------|
| G16 | 文本类拖拽上传 + LLM 整理为 MD + content_text 预览 | P1 | Extractor 抽象 + MarkdownOrganizer + 前端 DropZone/Queue + OOXML 守卫修复 | 8 ✅ |
| G17 | 多模态图片入库 | P2 | VisionExtractor（多模态 LLM）+ image/* 白名单 + asset_uri 血缘 + 异步提取回写 | 9 ✅ |
| G18 | 知识关联图谱（GraphRAG） | P3 可选 | 旁路非侵入：异步从 chunks 构建实体/关系，不嵌入摄取主链路（用户裁决，设计 §9.6） | 10 ⏸ |

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

### Phase 3：高级检索 — ✅ Rerank 已完成，AgenticFilter 待补

| 任务 | 状态 | 涉及文件 |
|------|------|----------|
| TopK / Cohere / Infinity Reranker | ✅ | `reranker_factory.go`、`retriever.go` |
| AgenticFilter | ⏳ | 集成 trpc `searchfilter` |

### Phase 4：超越层 — 待实现

| 任务 | 涉及文件 | 状态 |
|------|----------|------|
| OCR 识别（tesseract/docling 后端） | `internal/knowledge/ocr.go`（stub 已就位） | ⛔ 已废弃（2026-07-20 裁决：由 Phase 9 VisionExtractor 取代） |
| 多租户隔离 | 修改搜索过滤 + 向量存储 | ❌ |
| code_search 工具 | 新建 `internal/tools/knowledge/code_search.go` | ❌ |
| SourceSync 增量同步 | 新建 `internal/knowledge/sync.go` | ❌ |

### Phase 8：统一摄取管线（文本类）— ✅ 已完成

> 设计：[37-knowledge.design.md §5.2/§5.2b/§7.2](./37-knowledge.design.md) | 需求：US-12

| 任务 | 涉及文件 | 状态 |
|------|----------|------|
| Extractor 接口 + ExtractorRegistry 路由；TextExtractor 收编现有提取逻辑（摘除图片分支） | `internal/knowledge/extractor.go`（新增）、`document_extract.go`、`ocr.go`（废弃 stub） | ✅ |
| MarkdownOrganizer（LLM 整理为 MD，30s 超时，失败降级原文本） | `internal/knowledge/markdown_organizer.go`（新增） | ✅ |
| Proto：`organize_to_markdown` 字段 + `GetDocumentContent` RPC | `api/kratos/knowledge/v1/knowledge.proto` + `make api` | ✅ |
| Schema：`content_text`/`organized`/`asset_uri` 列（幂等 ALTER） | `internal/data/knowledge.go`（EnsureKnowledgeSchema） | ✅ |
| Biz：Document 模型加字段（CreateDocument 写入 / GetDocument 读出） | `internal/biz/knowledge/knowledge.go`、`internal/data/knowledge.go` | ✅ |
| Service：IngestDocument 接线 Extract+Organize；OOXML 守卫二次判定；GetDocumentContent 实现 | `internal/service/knowledge.go` | ✅ |
| Wire：MarkdownOrganizer 工厂（`NewKnowledgeMarkdownOrganizer`） | `internal/service/knowledge_advanced.go`、`cmd/admin/wire.go` | ✅ |
| 前端：DropZone 拖拽区 + UploadQueue 批量队列 + api.ts 参数 | `KnowledgeDropZone.vue`/`KnowledgeUploadQueue.vue`（新增）、`KnowledgeDocumentsPanel.vue`、`api.ts` | ✅ |
| 前端：MD 全文预览对话框 | `KnowledgeDocPreviewDialog.vue`（新增）、`api.ts`（getDocumentContent） | ✅ |

### Phase 9：多模态入库（图片）— ✅ 已完成（2026-07-21）

> 设计：[37-knowledge.design.md §5.2c](./37-knowledge.design.md) | 需求：US-13

| 任务 | 涉及文件 | 状态 |
|------|----------|------|
| VisionExtractor（多模态 LLM 图片 → MD 描述，实现 Extractor 接口） | `internal/knowledge/vision_extractor.go`（新增） | ✅ |
| 白名单放开 image/png、image/jpeg、image/webp | `internal/service/knowledge.go` | ✅ |
| 原图留存 + asset_uri 血缘 | `internal/knowledge/asset_store.go`（新增）、`internal/data/knowledge.go` | ✅ |
| metadata 写入 modality/extractor 标记 | `internal/service/knowledge.go`（mergeIngestMetadata） | ✅ |
| 图片异步提取：先落文档 + 后台 VisionExtractor + UpdateDocumentContent 回写（NFR-12 失败 status=error） | `internal/service/knowledge.go`、`internal/biz/knowledge/knowledge.go`、`internal/data/knowledge.go` | ✅ |
| Wire：ExtractorRegistry + AssetStore 工厂 | `internal/service/knowledge_advanced.go`、`cmd/admin/wire_gen.go` | ✅ |
| 前端：放开图片上传（accept/MIME 白名单/i18n） | `useKnowledgePage.ts`、`KnowledgeDropZone.vue`、`KnowledgeIngestDialog.vue`、locales | ✅ |
| 前端：图片预览回链（可选增强） | `KnowledgeDocumentsPanel.vue` | 📋 可选 |

> **架构决策（2026-07-21）**：图片入库采用「先落文档 + 后台异步提取」而非同步提取——视觉 LLM 调用最长 60s，同步会阻塞 HTTP；失败置 `status=error`（NFR-12），与 indexing → indexed/error 状态流一致。新增 `UpdateDocumentContent` Repo/Usecase 接口用于提取完成后回写 `content_text`/`organized`。

### Phase 10：GraphRAG 旁路（可选）— 暂缓

> 裁决（2026-07-20 用户确认）：Phase 3 可选增强；旁路非侵入，绝不嵌入摄取主链路。详见设计 §9.6 与 Roadmap 子模块 Phase 3。

### Phase 11：免选择知识库（US-14）— ✅ 已完成（2026-07-21）

> 设计：[37-knowledge.design.md §7.4](./37-knowledge.design.md) | 需求：US-14
> 核心理念：**存储可分类，使用免选**——Collection 是收纳工具，不是使用门槛。

| 任务 | 涉及文件 | 状态 |
|------|----------|------|
| Proto：Ingest/Search collection_id 去 REQUIRED + MoveDocument RPC | `api/kratos/knowledge/v1/knowledge.proto` + `make api` | ✅ |
| Usecase：EnsureDefaultCollection 懒创建（按 name 查找复用） | `internal/biz/knowledge/knowledge.go` | ✅ |
| Service：IngestDocument 留空落默认库；Search 留空走 FederatedRetriever Route 全库 | `internal/service/knowledge.go` | ✅ |
| 工具：collection_id/collection_ids 改可选；scoped 多库路由；无 scoped 全库路由；零库返回空结果 | `internal/tools/knowledge/tool.go` | ✅ |
| MoveDocument：Repo 事务（documents+chunks 随迁 + 计数校正）+ dim 兼容校验 | `internal/data/knowledge.go`、`internal/biz/knowledge/knowledge.go`、`internal/service/knowledge.go` | ✅ |
| 前端：上传免预选（不静默丢弃 + 落默认库提示） | `useKnowledgePage.ts`、locales | ✅ |
| 前端：搜索面板默认「全部知识库」 | `useKnowledgePage.ts` / 搜索面板组件 | ✅ |
| 前端：文档「移动到…」（dim 不兼容禁用提示） | `KnowledgeDocumentsPanel.vue`、`api.ts`、locales | ✅ |
| 前端：Agent 编辑器 knowledge_bases 折叠到高级配置 | Agent 编辑组件 | ✅ |

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
| 26 | Extractor 接口 + Registry + TextExtractor 收编 | P1 | — | 8 ✅ |
| 27 | MarkdownOrganizer（LLM 整理为 MD + 降级） | P1 | — | 8 ✅ |
| 28 | Proto `organize_to_markdown` + `GetDocumentContent` RPC | P1 | — | 8 ✅ |
| 29 | Schema content_text/organized/asset_uri 列 | P1 | — | 8 ✅ |
| 30 | IngestDocument 接线 + OOXML 守卫二次判定修复 | P1 | — | 8 ✅ |
| 31 | 摄取管线 Wire 工厂 | P1 | — | 8 ✅ |
| 32 | 前端拖拽区 + 批量上传队列 | P1 | — | 8 ✅ |
| 33 | 前端 MD 全文预览 | P2 | — | 8 ✅ |
| 34 | VisionExtractor（多模态 LLM 图片 → MD） | P2 | — | 9 ✅ |
| 35 | image/* 白名单放开 + 原图 asset_uri 血缘 | P2 | — | 9 ✅ |
| 35b | 图片异步提取 + UpdateDocumentContent 回写 | P2 | — | 9 ✅ |
| 36 | GraphRAG 旁路构建（可选，非侵入） | P3 | — | 10 ⏸ |
| 37 | 免选择知识库：默认库懒创建 + 工具/Search 全库智能路由 + MoveDocument + 前端配套 | P1 | — | 11 ✅ |

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

### Phase 3 — ✅ Rerank / ⏳ AgenticFilter

- [x] 检索结果经 Reranker 重排序（env 配置 + Search 请求覆盖）
- [ ] AgenticFilter 启用后 LLM 可动态生成过滤条件

### Phase 4 — 待实现

- [ ] ~~图片/PDF 文档可 OCR 识别入库~~（⛔ 2026-07-20 废弃，由 Phase 9 多模态视觉路线取代）
- [ ] 不同租户搜索不到彼此的知识
- [ ] Agent 可调用 code_search 工具

### Phase 8 — ✅ 已完成（统一摄取管线·文本类）

- [x] 前端拖拽区域可一次拖入多个文本/Office 文件，逐文件生成上传任务并实时展示状态
- [x] txt/md/json/csv/html/xml/yaml + pdf/doc/docx/xlsx/pptx 均可正确提取文本（DOCX/XLSX/PPTX 不再被 `application/zip` 误拒）
- [x] 默认开启整理为 Markdown：提取文本经 LLM 结构化后按 markdown 策略分块入库
- [x] LLM 不可用/整理失败时降级原文本入库，文档正常 indexed
- [x] 整理后 MD 全文写入 `content_text`，`GetDocumentContent` RPC + 前端预览可用
- [x] 拖入图片返回明确错误提示（多模态未上线），不静默失败

### Phase 9 — ✅ 已完成（多模态入库）

- [x] png/jpg/jpeg/webp 图片拖入后经多模态 LLM 输出 MD 描述入库，可被语义搜索命中（代码链路完成 + 单测覆盖；真实视觉模型端到端命中待环境就位后复验）
- [x] 原始图片留存，Document `asset_uri` 血缘可追（已运行时验证：`data/knowledge_assets/{docID}.{ext}` 落盘）
- [x] 文档 metadata 含 modality/extractor 标记，检索结果与文本文档无差别
- [x] 未配置多模态 LLM 时图片上传返回明确错误（status=error）（已运行时验证：`no vision-capable LLM available for vision extract; enable a vision model in the catalog or configure DefaultRefineLLM`）

> **运行时验证记录（2026-07-21）**：① 图片上传 13ms 异步返回（非阻塞）✅；② 无视觉模型时文档 status=error + 明确错误信息 ✅；③ 原图留存落盘 ✅；④ 文本类回归（.md 提取 + content_text 预览）无回归 ✅。验证环境无视觉模型与 embedder，「图片 → MD → 向量命中」全链路需在配置视觉模型（如 gpt-4o / qwen-vl）+ embedder 后复验。

### Phase 10 — ⏸ 可选（GraphRAG 旁路）

- [ ] 图谱构建为异步旁路，摄取主链路无侵入（对照设计 §9.6 架构纪律）
- [ ] 多跳/实体关系查询可由图遍历增强检索回答

### Phase 11 — ✅ 免选择知识库（US-14）

- [x] 上传不预选 Collection：留空自动落「默认知识库」（懒创建），前端不静默丢弃文件
- [x] `knowledge_search`/`knowledge_reflect` 留空 collection：scoped>1 库内路由、无 scoped 全库路由，不再报 required 错误
- [x] Search API 留空 collection_id：全库智能路由（Route 策略 + 无匹配降级 Broadcast）
- [x] 系统无任何 Collection 时工具返回空结果（不阻塞会话）
- [x] MoveDocument：文档连 chunks 跨库移动、计数同步、dim 不兼容拒绝（CodeConflict）
- [x] 调试搜索面板默认「全部知识库」；Agent 编辑器知识库绑定折叠到高级配置
- [x] 兼容性：已绑定 knowledge_bases 的 Agent 与显式传 collection_id 的调用行为不变

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

- [ ] **GraphRAG** — 知识图谱构建（实体/关系提取）+ 图增强检索 + 图查询工具
- [ ] **Skill Knowledge** — 技能知识库（三层技能层次）+ 知识导航工具 + 技能蒸馏管线

---

## 7. 依赖与风险

| 依赖 | 说明 |
|------|------|
| PostgreSQL + pgvector | Knowledge 核心存储，无 PG 时模块不可用 |
| Embedding API | OpenAI 或 Ollama 端点必须可达 |
| PDF 解析 | 需引入第三方库（如 unidoc/unioffice）或 Docling 服务 |
| 本地 Embedding | 需 GPU 或大量 CPU 资源 |
| OCR | 需 Tesseract 或 Docling 服务部署（当前为 stub） |
| LLM API | 查询重写和检索评估依赖 LLM 调用，无 LLM 时自动降级 |

---

## 8. 代码优化记录

> 优化记录汇总（原始 changelog 文件已归档）。

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
| ~~P1~~ | ~~统一摄取管线 Phase 8~~ | ✅ 已完成：Extractor 抽象 + MarkdownOrganizer + 拖拽上传 + OOXML 守卫修复（G16） |
| ~~P2~~ | ~~多模态入库 Phase 9~~ | ✅ 已完成（2026-07-21）：VisionExtractor 多模态 LLM 路线（G17）+ 异步提取 + 原图 asset_uri 血缘；原 tesseract/docling OCR 方案废弃 |
| P2 | ListChunks/ReindexDocument/UpdateDocument RPC | 运维调试不便（KB-17）；content_text 列已为 Reindex 铺路 |
| P3 | AgenticFilter | 集成 trpc `searchfilter` |
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

> **版本**：2026-06-17 | **状态**：Phase 1（Advanced RAG）✅ 已实现，Phase 2（Agentic RAG）✅ 已实现
> **前置**：[37-knowledge.md](./37-knowledge.md) · [37-knowledge.design.md](./37-knowledge.design.md)
> **学术参考**：见附录 A

---

## 一、现状评估

### 1.1 已实现能力（Naive RAG 完整管线）

| 能力 | 状态 | 代码锚点 |
|------|------|----------|
| Collection/Document/Chunk 三级数据模型 | ✅ | `internal/biz/knowledge/knowledge.go` |
| 多 Provider Embedder（OpenAI/Ollama/Gemini/HuggingFace） | ✅ | `internal/knowledge/embedder.go` |
| 多格式文档提取（PDF/DOCX/XLSX/PPTX/HTML/OCR stub） | ✅ | `internal/knowledge/document_extract.go` + `ocr.go` |
| 多分块策略（char/token/markdown/json/recursive） | ✅ | `internal/knowledge/chunker.go` + `chunk_strategy.go` |
| pgvector 向量存储 + 余弦相似度搜索 | ✅ | `internal/data/knowledge.go` |
| 可选 Rerank（topk/cohere/infinity） | ✅ | `internal/knowledge/retriever.go` |
| Agent 运行时 `knowledge_search` 工具 | ✅ | `internal/tools/knowledge/tool.go` |
| 异步入库 + WebSocket 进度推送 | ✅ | `internal/service/knowledge.go` + `useKnowledgeIngestWs.ts` |
| Collection 级别权限隔离 | ✅ | `WithKnowledgeCollections` context 限定 |
| Embedder 运行时热更新 | ✅ | `internal/service/knowledge_embedder.go` |
| 查询重写（HyDE/Decomposition/MultiQuery） | ✅ | `internal/knowledge/query_rewriter.go` |
| 混合检索（BM25+向量 RRF 融合） | ✅ | `internal/knowledge/hybrid_retriever.go` |
| BM25 全文检索（PostgreSQL ts_vector + pg_trgm 双路） | ✅ | `internal/data/knowledge.go` |
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
                     Phase 1 ✅          Phase 2 ✅
                     已完成               已完成
```

**当前 Aranea 知识库处于 Agentic RAG 阶段**（已具备查询重写、混合检索、自适应路由、检索评估、联邦搜索、Agent 自校验工具、Plan-Then-Retrieve），正向 GraphRAG 阶段演进（知识图谱构建待实现）。

---

## 二、知识库在 Aranea 中的定位

> 三层知识模型（L1 文档知识 / L2 关系知识 / L3 技能知识）的架构设计详见 [37-knowledge.design.md §9.7.1](./37-knowledge.design.md#971-三层知识模型)。

Knowledge 与 Memory、Skill 构成 Agent 认知三角：
- **Memory**：Agent 自身经验的内化（"我做过什么"）
- **Knowledge**：外部知识的摄入和组织（"世界是什么样"）
- **Skill**：从经验/文档中蒸馏的可复用操作流程（"该怎么做"）

当前实现 L1 文档知识（向量检索），L2 关系知识（GraphRAG）和 L3 技能知识（Skill Knowledge）待实现。

---

## 三、四阶段演进路线

### Phase 1：Advanced RAG — 夯实检索质量 ✅ 已实现

> **实现日期**：2026-05-28 | **预期收益**：检索精度提升 20-30%
> 架构设计详见 [37-knowledge.design.md §5.5-5.9](./37-knowledge.design.md#55-queryrewriterinternalknowledgequery_rewritergo)

| 子任务 | 状态 | 代码锚点 |
|--------|------|----------|
| 1.1 查询重写与分解（HyDE/Decomposition/MultiQuery） | ✅ | `internal/knowledge/query_rewriter.go` |
| 1.2 混合检索（Dense+BM25+RRF 融合） | ✅ | `internal/knowledge/hybrid_retriever.go` + `internal/data/knowledge.go` (`SearchChunksBM25`) |
| 1.3 自适应检索路由（查询复杂度分类） | ✅ | `internal/knowledge/adaptive_router.go` |
| 1.4 检索质量评估（CRAG 式自校验） | ✅ | `internal/knowledge/retrieval_evaluator.go` |

---

### Phase 2：Agentic RAG — 让 Agent 主动检索 ✅ 已实现

> **实现日期**：2026-05-29 | **预期收益**：复杂查询检索质量提升 40-50%
> 架构设计详见 [37-knowledge.design.md §5.10](./37-knowledge.design.md#510-federatedretrieverinternalknowledgefederated_retrievergo) 和 [§6.1b](./37-knowledge.design.md#61b-knowledge_reflect-工具internaltoolsknowledgetoolgo)

| 子任务 | 状态 | 代码锚点 |
|--------|------|----------|
| 2.1 多轮迭代检索工具（knowledge_reflect） | ✅ | `internal/tools/knowledge/tool.go` |
| 2.2 跨 Collection 联邦搜索（Broadcast + Route） | ✅ | `internal/knowledge/federated_retriever.go` |
| 2.3 Plan-Then-Retrieve（BeforeModel 钩子） | ✅ | `internal/agent/knowledge_inject.go` |

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

---

### Phase 3：GraphRAG — 知识图谱增强

> **2026-07-20 定位裁决（用户确认）**：Phase 3 可选增强，工程暂缓。**旁路非侵入纪律**：图谱从已入库 chunks 异步构建，绝不嵌入统一摄取管线（Phase 8/9）主链路；检索增强以 GraphAugmentedRetriever 包裹现有 Router 外层。详见 [37-knowledge.design.md §9.6](./37-knowledge.design.md#96-graphrag--知识图谱增强)。

> **预期收益**：多跳/实体关系查询准确率提升 60-70%
> 架构设计详见 [37-knowledge.design.md §9.6](./37-knowledge.design.md#96-graphrag--知识图谱增强)

| 子任务 | 状态 | 代码锚点（计划） |
|--------|------|------------------|
| 3.1 知识图谱构建（实体/关系提取 + 存储） | ❌ | `internal/biz/knowledge/graph.go`（新增）、`internal/data/knowledge_graph.go`（新增） |
| 3.2 图增强检索（向量 + 图遍历融合） | ❌ | `internal/knowledge/graph_augmented_retriever.go`（新增） |
| 3.3 图查询工具（knowledge_graph_search/traverse） | ❌ | `internal/tools/knowledge/graph_tool.go`（新增） |

---

### Phase 4：Skill Knowledge — 技能知识库

> **预期收益**：Agent 任务执行效率提升 80%+（通过技能复用减少重复推理）
> 架构设计详见 [37-knowledge.design.md §9.7](./37-knowledge.design.md#97-skill-knowledge--技能知识库)

| 子任务 | 状态 | 代码锚点（计划） |
|--------|------|------------------|
| 4.1 技能知识库构建（三层技能层次） | ❌ | `internal/biz/knowledge/skill_knowledge.go`（新增） |
| 4.2 知识导航工具（knowledge_navigate/drill） | ❌ | `internal/tools/knowledge/navigate_tool.go`（新增） |
| 4.3 技能蒸馏管线（轨迹 → 技能提取） | ❌ | `internal/knowledge/skill_distiller.go`（新增） |

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
| 6 | 日志统一 | 使用 `loggateway.Logger`（构造注入），禁止 `log/slog`、`loggateway.Global()` |
| 7 | Proto 契约 | 新增 API 先写 proto，`make api` 生成，不手写 |
| 8 | 向后兼容 | 每个 Phase 向后兼容，不破坏现有 API |

---

## 五、演进总览

| 维度 | Phase 1 ✅ | Phase 2 ✅ | Phase 3 | Phase 4 |
|------|-----------|---------|---------|---------|
| 检索模式 | 混合检索+查询重写 | 多轮迭代+自校验+Plan-Then-Retrieve+Route策略 | 图+向量融合 | 层次导航 |
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
| `internal/service/knowledge_advanced.go` | 新增 | Service 层 Wire 工厂（6 个 Provider） |
| `internal/biz/knowledge/knowledge.go` | 修改 | 新增 `SparseSearcher` 接口 + 子接口拆分 |
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

---

## 子模块：Vault 重设计 Phase 计划

> **设计契约**：[37-knowledge.design.md §子模块 Vault 重设计](./37-knowledge.design.md#子模块vault-重设计)（V1~V11，含 R-1~R-6 实现契约）
> **评审结论**（2026-07-25）：方向与架构通过；R-1~R-6 已合入设计文档 §V6，本计划按评审 §9 微调：**P4 拆为 P4a（L0 强 BM25 栈，含 R-5 选型 spike）+ P4b（L2 语义层插件化，含 R-4 契约变更）；R-6 安全契约并入 P5**。
> **状态**：📋 待启动 | 与上文 Roadmap 子模块（Phase 1-4 RAG 演进）为独立演进线，编号互不影响。

### 总览

| Phase | 内容 | 关联契约 | 状态 |
|-------|------|---------|------|
| **P1** | Vault 基础 + 单向同步（文件→索引） | S-1 | ✅ |
| **P2** | 双向同步 + frontmatter 摘要卡 + 双轨关联 | R-1、R-2、R-3 | ✅ |
| **P3** | 资源管理器 UI（树/列表/详情/双区搜索） | R-3（来源标注） | ✅ |
| **P4a** | L0 强 BM25 栈（含 R-5 选型 spike） | R-5、S-5 | 📋 |
| **P4b** | L2 语义层插件化（含 R-4 契约变更） | R-4、S-3 | 📋 |
| **P5** | Agent 工具族 navigate/grep/write（含 R-6 安全契约） | R-6 | 📋 |
| **P6** | 迁移 Collection → Vault | S-2 | 📋 |

### P1：Vault 基础 + 单向同步

| # | 任务 |
|---|------|
| P1-1 | `knowledge_collections` 升级 Vault：`root_path`（唯一约束 + 规范化：resolve symlink/绝对路径/尾部斜杠归一，禁挂系统根目录 S-1）、`sync_state`、`sync_config`；DDL 迁移注册 — ✅ 已完成（2026-07-25，Schema 列+唯一索引已落；`internal/biz/knowledge/vault_usecase.go`：NormalizeRootPath + CreateVault，9 测试全绿；2026-07-29：Proto/CLI `root_path` 改必填（V2 不兼容旧模式）、`embedding_model` 改可选留空=无语义层词法库，service 层 CreateCollection 改走 CreateVault） |
| P1-2 | VaultFiler：KB 侧唯一写文件出口（路径 sanitize 基础版）— ✅ 已完成（2026-07-25，`internal/biz/knowledge/vault_filer.go`：sanitize/原子写/覆盖备份/回收站，8 测试全绿） |
| P1-3 | SyncEngine 单向扫描：文件变更 → chunk → （可选 embed）→ 派生索引；轮询实现，fsnotify 留接口 — ✅ 已完成（2026-07-25，`internal/biz/knowledge/sync_engine.go` + `internal/knowledge/vault_sync.go`/`vault_sync_runner.go`：Scan/Diff/Apply/Run 三段解耦；prev 重启自 DB 重建；幂等短路 + 无语义层降级；Watcher 接口预留，P2 接 fsnotify；13 测试全绿；2026-07-29 生产装配补齐：`internal/knowledge/vault_sync_supervisor.go` 每 vault 一 RunVault 生命周期管理（StartAll/StartVault/StopVault/Stop，幂等），wire `provideVaultSyncSupervisor` 装配 + app.go BeforeStart 注入 service / readiness 后 StartAll / AfterStop Stop，service CreateCollection/DeleteCollection 钩子；修复 data 层 `GetDocumentByRelPath`/`GetDocument` 裸 `sql.ErrNoRows` 未翻译（DB-R5）导致 applier 无法走创建路径；5 测试全绿） |
| P1-4 | 派生索引纪律落地：全部索引无状态可重建，reindex 一键化；禁止与业务表触发器耦合 — ✅ 已完成（2026-07-25，links/entities 表按派生纪律落 Schema；`internal/knowledge/vault_reindex.go`：ReindexVault 一键重建，ApplyEventsForced 绕过幂等短路强制重建 chunks，4 测试全绿） |

验收：挂载一个本地文件夹为 Vault，新增/修改 .md 后索引自动更新；删 Vault 只删索引不动文件。

### P2：双向同步 + 摘要卡 + 双轨关联

| # | 任务 |
|---|------|
| P2-1 | frontmatter 受管字段分区实现（R-1）：KB 独占 `id/summary/tags/type/summary_hash/source/created`；写入前重读 hash，冲突留双份备份 — ✅ 已完成（2026-07-26，`internal/biz/knowledge/vault_filer.go`：`ReadDocWithHash` 返回内容 hash + `WriteDocCAS` 写入前重读比对，三类冲突（hash 不匹配/期望不存在但存在/期望存在但消失）均留双份——磁盘当前版本进 trash + KB 版本写入，12 测试全绿） |
| P2-2 | 摘要卡生成：LLM 预生成摘要写入 frontmatter；`summary_hash` 比对标记 stale，异步重生成 — ✅ 已完成（2026-07-26，`internal/knowledge/vault_summary.go`：VaultSummaryGenerator——stale 仅基于 Body 判定（防写回自触发循环）、合并写回不回滚并发外部编辑、JSON 容错解析、5min 失败节流、无 LLM/超时/解析失败全降级 nil error；`internal/biz/knowledge/vault_filer.go`：SummaryStale；`internal/knowledge/vault_sync.go`：SetSummaryHook 索引成功后 stale 触发；12 测试全绿） |
| P2-3 | KB 自写文件打标 + watcher 回环防护（R-2）；外部删除一律进 `.aranea/trash` — ✅ 已完成（2026-07-27，`internal/biz/knowledge/vault_filer.go`：自写标记管理（markSelfWrite/ConsumeSelfWrite 一次性消费）+ WriteTrashFromMirror 镜像抢救；`internal/knowledge/vault_sync.go`：deleteDoc 接线外部删除进 trash；4 新测试全绿） |
| P2-4 | `knowledge_links` 表 + explicit（`[[]]` 双链解析）/ entity（LLM 实体共现，停用词+频次过滤 R-3）双轨；semantic 轨待 P4b — ✅ 已完成（2026-07-27，explicit：`internal/biz/knowledge/link_parser.go` ParseWikiLinks/ResolveLinkRefs + `vault_sync.go` rebuildExplicitLinks；entity：`internal/knowledge/vault_entity.go` VaultEntityExtractor——LLM 抽取、内置停用词+过短过滤、R-3 频次过滤（先落库再查共现，频次含当前文档）、docID+contentHash 幂等 + 5min 失败节流、无 LLM/超时/解析失败全降级；`vault_sync.go` SetEntityHook 索引成功触发；data 层 `internal/data/knowledge_links.go`：ReplaceLinks/ListLinks/ReplaceDocEntities（含孤儿实体清理）/FindEntityCooccurrences；7+5 测试全绿） |

验收：KB 写入与用户外部编辑双向不丢不乱（回环防护生效）；摘要卡随内容过期自动重生成；关联区数据可按来源类型过滤。

### P3：资源管理器 UI

| # | 任务 |
|---|------|
| P3-1 | KnowledgePage 重构为三栏：Vault 切换 + 文件夹树（懒加载）+ 文档列表 + 详情面板（hover 卡两级密度）— ✅ 已完成（2026-07-26，组件 `web/src/components/knowledge/KnowledgeVaultTree.vue`（Vault 切换头+sync 状态徽标+根目录行+q-tree 懒加载）/`KnowledgeDocList.vue`（面包屑+文件列表+hover 摘要卡）/`KnowledgeDocDetail.vue`（一级摘要卡/二级正文预览+操作）；编排 `web/src/features/knowledge/useVaultExplorer.ts`（选中态/树缓存经 store/平铺降级/文档签名变化自动刷新树）；`KnowledgePage.vue` documents tab 收敛为 explorer 三栏（260px/1fr/360px 响应式网格），旧 `KnowledgeCollectionList/KnowledgeDocumentsPanel/KnowledgeDocPreviewDialog` 保留供回退；`useKnowledgePage.ts` 挂 explorer + 文档索引上限放宽 2000） |
| P3-2 | 统一搜索框双区：即时区（纯前端 fzf 式内存索引 <10k 文档）+ 语义区（走后端检索）— ✅ 已完成（2026-07-26，`web/src/components/knowledge/KnowledgeSearchDual.vue` 双区面板；`web/src/features/knowledge/instantMatch.ts` fzf 子序列匹配+多词 AND+连续/边界加分（11 单测）；即时区过滤 source/rel_path/summary/tags/doc_type，语义区回车走当前 vault 后端 Search top_k=8，命中跳转定位 prefix+选中文档） |
| P3-3 | 关联区：双链/实体/语义三类关联展示并标注来源类型（R-3）；搜索意图分流规则与后端路由规则共享定义 — ✅ 已完成（2026-07-26，关联区在 `KnowledgeDocDetail.vue` 二级：explicit=显式双链(primary)/entity=实体共现(deep-purple)/semantic=语义近邻(teal) 三类来源徽标 + out=本文引用/in=被引用于方向标注 + context 展示 + 点击跳转目标文档；意图分流共享定义 `web/src/features/knowledge/searchIntent.ts` ↔ `internal/knowledge/search_intent.go` 同一规则表（注释互指），前端中文疑问词以码点构造正则规避 check-i18n CJK 限制，两侧各 6/单测全绿） |

验收：三栏可用；搜索双区意图分流正确；关联区来源标注完整。

> **运行时验证记录（2026-07-29）**：① 中栏重设计为资源管理器式表格（`KnowledgeDocList.vue`：名称/修改日期/类型/大小/状态五列，列头点击升降序排序，目录行下钻 + 面包屑返回，目录优先排序）浏览器实测通过；② 新建对话框（`KnowledgeCreateDialog.vue`：名称/根目录必填 + 描述/Embedding 模型可选）实测：不存在路径返回 warning 通知 `root_path not found: ...`，合法路径创建成功并自动同步入库；③ Vault 切换器名称无重复、显示文档/分块计数与同步状态；④ 详情面板选中文件显示摘要卡 + 展开正文预览（已整理 MD 徽标）+ 关联区；⑤ 双区搜索即时区命中文件名、语义区回车提示；⑥ P1-3 生产装配运行时验证：存量 vault 启动即扫入库、UI 新建 vault 立即同步 2 文档、45s 轮询捕获新增文件（documentCount/syncState/lastSyncAt 正确）。已知小项：无 root_path 的历史 collection 在切换器中显示「同步正常」（语义不准，低优）；目录行修改日期列为空（tree API 目录无 updatedAt，装饰性）；`KnowledgeEmbedderFields` defineModel('form') 触发 dev 编译器 const 警告（既有模式，行为正确）。

> **UI 修复 + 语义降级验证记录（2026-07-29 晚）**：全面 UI 测试发现配色与功能问题后按 F1~F5 修复——F1 中栏表格重写（`KnowledgeDocList.vue`：`table-layout: fixed` + `name-wrap/name-text` ellipsis 修复名称列文字不可见；列宽 40%/20%/14%/10%/16%）；F2 四处组件 19 处 `--q-grey-*` 颜色变量替换为主题 token（`--color-text-primary/secondary`、`--color-surface-soft`、`--color-border-soft`）；F3 DropZone 浏览态折叠为紧凑单行按钮（`compact` prop + `dropzoneCompactTitle` i18n）；F4 语义检索错误内联展示（`useVaultExplorer.semanticError` + `KnowledgeSearchDual` error 区，兜底文案 i18n 化 `knowledgePage.searchSemanticError`，不弹红 toast）；F5 后端 embedder 未配置/不可用时 BM25 降级（`retriever.go`/`hybrid_retriever.go`：`embedder==nil` 或 `CodeUnavailable` 时走 `searchSparseFallback`/`searchSparse`，`retriever_sparse_fallback_test.go` 覆盖 nil embedder/UNAVAILABLE/无 BM25 能力三场景）。验证：`pnpm lint` 0 errors（修复 1 处新硬编码中文入 i18n）、`pnpm test` 147 文件 1059 测试全绿、`pnpm build` 成功、`go build ./...` + `go test ./internal/knowledge/... ./internal/service -run Knowledge` 全绿；浏览器双主题复验：亮色/暗色下表头、面包屑、名称列、选中态、详情面板配色均正常，guides 下钻 + setup.md 详情 + 即时搜索命中正常；语义检索实测 embedder 未配置场景：后端日志确认 `knowledge.hybrid.sparse_fallback`（reason=embed failed, UNAVAILABLE），前端不再 500 红错，语义区显示「无语义结果」（该 vault 文档无 chunks 故 BM25 空结果，数据层面预期）。已知非阻塞项：WS 重连 429 限流（admin 重启瞬态，自行恢复）；`internal/service` 全量测试中 chat 域 ModelCatalog panic 为既有无关问题。

> **G1 资源管理器 V2 树骨架完成记录（2026-07-30，设计 §V12.1~V12.3 B1~B3）**：① 库融树——树一级节点=库（`vaultTreeUi.ts` key 编解码 `v|<cid>`/`d|<cid>|<prefix>`），选中态单一事实源 `{collectionId, prefix}`，树底部 `[+ 新建库]` 融入（页头新建按钮已删）；② 树节点 hover 菜单——库/目录节点：新建目录、新建文档、上传文件到此，库节点另有刷新/删除库（`KnowledgeVaultTree.vue`）；③ 科幻图标——库=cyan、目录=violet、md=teal、图片=magenta、音视频=orange、error=red 脉冲（`vaultTreeUi.ts vaultNodeVisual` + `app-global.sass` `.kv-icon--*` 双主题色板，树/中栏列表共用）；④ 后端 B1 扫文件系统（`vault_explorer.go` ListVaultTree = `VaultFiler.ListSubdirs` ∪ 索引聚合，空目录可见+目录 mtime）、B2 `CreateVaultDir`/`CreateVaultDocument`（Mkdir/WriteDoc 模板 + `VaultSyncApplier.ApplyOne` 立即索引，不等 45s 轮询）、B3 `IngestDocumentRequest.target_dir`（`WriteVaultUpload` 落盘 + content_hash 视为已应用 + CreateDocument 失败补偿删除，target_dir="/" 根目录特例）；⑤ DropZone 已删，上传走树 hover 菜单 → 隐藏 input 接线（`pendingUploadTarget` watch 触发 click）；⑥ q-tree 懒加载缓存修复——`useVaultExplorer.reloadExpandedNodes` 递归重拉展开节点子树原地修补（done() 标记不重置导致刷新不生效）。**降级一致性修复**：IngestDocument 词法库（embedding_model 空）传 nil embedder + `BuildIndexedChunks` 对 `ErrEmbedderNotConfigured` 降级空向量（与 vault `buildChunks` R-4 一致，ingest_test 3 新用例）——修复前上传词法库报「embedder not configured」置 error，修复后实测立即 indexed。验证：前端 lint 0 error / 知识域 28 测试全绿 / build 成功；Go `internal/knowledge` + `service -run Knowledge|Vault` 全绿；浏览器实测：三栏布局、树图标配色、hover 菜单三项、新建目录（pw-test-dir）、新建文档（pw-test-doc.md 立即 indexed+自动选中）、上传到此（g1-upload-test2.md 立即 indexed）、中栏五列排序/下钻/面包屑、error 红色脉冲、console 无 error。

> **G2 详情面板改版完成记录（2026-07-30，设计 §V12.3 B5/B6 + §V12.4）**：① B5 编辑保存——proto 新增 `rpc UpdateDocumentContent(id, content, base_hash)`（PUT `/v1/knowledge/documents/{id}/content`）+ `DocumentContent.raw_content/base_hash` 字段；biz `vault_write.go` 新增 `GetVaultDocumentRaw`（body 原文 + 文件 sha1，供编辑器数据源与 CAS 凭证）与 `UpdateVaultDocumentContent`（读出现有 frontmatter（受管字段+用户 Extra）仅替换 body，`WriteDocCAS` 原子写入——外部已改/已删/并发创建时留双份（磁盘版备份进 trash）仍写入并返 `conflict=true` 保守不丢数据，写后 `VaultSyncApplier.ApplyOne` 立即重索引）；service 接线 + 冲突/失败进程日志（`knowledge.document.save_fail`/`save_conflict`）；`vault_filer.go` strVal 修复 time.Time 序列化（frontmatter created 字段 RFC3339 回写，否则编辑保存后时间戳解析失败）。② B6 原始文件流——`GET /v1/knowledge/documents/{id}/asset` 经 custom route 注册（`http.go` registerCustomRoutes，先于 proto 通配，同 artifact 下载模式，标准 auth 过滤器鉴权）；biz `ResolveDocumentAsset`（vault 文档读 collection root+rel_path，历史非 vault 文档经 AssetStore 解析 asset_uri）；service `ServeDocumentAsset`（doc→collection 租户门禁 NotFound 防泄漏、mime 判定、image/audio/video/pdf inline 其余 attachment、`http.ServeContent` 支持 Range 媒体拖动）。③ 前端 V12.4——`KnowledgeDocDetail.vue` 重构：第一行摘要一行省略 + hover 360px 大号浮层卡（完整摘要+tags+大小/更新时间/路径/类型元信息），名称/路径缩小为副标题；第二行关联计数 chips（explicit/entity/semantic 聚合，点击锚滚关联区）+ 操作按钮（编辑/下载/移动/删除）；正文/媒体区固定高 420px（img/audio/video 走 B6 流内联渲染、md/txt 编辑态等宽 textarea 保存/取消、word 原文下载、其余解析文本预览）。`knowledgeUi.ts` 新增媒体分类（`knowledgeMediaKind`/`knowledgeMediaNeedsAsset`/`knowledgeMediaEditable`，按扩展名 image/audio/video/word/markdown/text/other）；`useVaultExplorer.ts` 选中即加载详情（content_text + raw_content/base_hash 编辑凭证 + 关联 force 刷新），asset blob→objectURL（watcher 竞态丢弃、scope dispose revoke），编辑保存返回 saved/conflict/error 由页面层提示（conflict 黄条提示留双份重载）。验证：Go `biz/knowledge` + `internal/knowledge` + `service -run Knowledge|Vault` 全绿（含 `vault_write_test.go` + `knowledge_asset_test.go` 新用例）；前端 lint 0 error / 1121 测试全绿 / build 成功；干净 GOCACHE（test/gocache-g2）复跑 `go build ./...` 通过。**浏览器运行时复验（2026-07-30，重启最新 admin 后）**：选中即加载详情正常；摘要 hover 大卡弹出正常——**修复 1 处主题 token 错误**：hover 卡 `background: var(--color-surface)` 与滚动条 `var(--color-border)` 引用了不存在的 token（主题为 `--color-surface-solid/soft/elevated`、`--color-border-soft`），导致 hover 卡背景透明内容重叠难读，已修正（滚动条 hover 用 `var(--color-primary, var(--q-primary))` 兜底链，`--color-primary` 全项目未定义为既有债务）；B5 编辑保存实测 readme.md 追加/恢复往返成功（保存→立即重索引→预览刷新，磁盘文件 frontmatter title/tags 完整保留）；B6 asset 端点实测 200 + `Accept-Ranges: bytes` + `Content-Type: text/markdown` + .md 正确走 attachment + 响应字节与磁盘文件一致；console 0 error（1 条 WS probe 警告为 admin 重启瞬态，自行恢复）。

> **G3 拖拽移动 + 搜索范围选择器完成记录（2026-07-30，设计 §V12.5/V12.6 B4/B7）**：① B4 后端——proto `rpc MoveDocumentToDir(id, target_dir, conflict_policy)`（POST `/v1/knowledge/documents/{id}/move_to_dir`）；biz `vault_filer.go` 新增 `Move`（文件系统原子 rename + 冲突策略：默认已存在则报错 / `overwrite` 目标旧版本入 trash / `rename` 自动生成唯一名 + 自写标记防 watcher 回环）；biz `vault_write.go` 新增 `MoveVaultDocumentToDir`（VaultFiler.Move → `UpdateDocumentRelPath` 保留文档身份/chunks/content_hash 不重索引 → `rebuildMovedDocInboundLinks` 重建引用本文档的其他文档 explicit 出链——精确路径引用断链诚实反映失效）；`link.go` `RebuildExplicitLinks` 收编为 biz 公共方法（同步索引与移动入链重建复用，消除重复）；service 接线 + 租户门禁；TDD 覆盖基本移动/同目录幂等/三冲突策略/入链重建/跨库与非 vault 拒绝。② B7 后端——proto `SearchRequest.path_prefix`（field 12）；biz `SearchQuery.PathPrefix` 透传；data 层 chunks 查询 SQL 增加 `rel_path LIKE <prefix>%`（`%`/`_` 通配符转义 + 目录边界语义，首尾斜杠容忍）；service 映射 `strings.TrimSpace`；`TestKnowledgeRepo_SearchChunks_PathPrefix`（修复测试 schema search_path 使 pgvector `vector` 类型可见）+ `TestSearch_PathPrefixPlumbed` 全绿。③ G3-F 前端拖拽移动（V12.5，HTML5 DnD 不引库）——`vaultTreeUi.ts` 新增 `DragFileRef`/`DropTargetRef`/`isValidDropTarget`（同 vault 且非原地才合法，跨库本期禁止）；`useVaultExplorer.ts` 拖拽状态（`dragFile`/`pendingMove`）+ `dragStartFile`/`dragEnd`/`dropOnTarget`/`resolveMoveConflict`/`dismissMoveConflict`（409 经 `parseKratosApiError` 判定返 conflict 暂存待决，成功强制重载中栏）；`KnowledgeDocList.vue` 文件行 `draggable` + 离屏幽灵卡 `setDragImage` + 面包屑段兼作 drop 目标（合法发光高亮 + hover 500ms 下钻）；`KnowledgeVaultTree.vue` 树目录/库节点 drop 高亮 + 非法禁用；`KnowledgeMoveConflictDialog.vue`（新增）冲突策略弹窗 覆盖（旧版入回收站）/ 保留两份（自动改名）/ 取消；`KnowledgePage.vue` 接线 + 移动成功 notify。④ G3-F 搜索范围选择器（V12.6）——`KnowledgeSearchDual.vue` 搜索框前缀「范围」按钮弹出迷你目录树（仅目录单选，复用 vault key 编解码，懒加载走同一 `onLazyLoad`），选中后 `searchScopePrefix` 持续生效（切库自动复位）；即时区前端按 `rel_path.startsWith(scope)` 过滤，语义区经 B7 `path_prefix` 走后端；`stores/knowledge` `moveDocToDir` action（移动成功失效树缓存）+ `api.ts` `moveDocumentToDir`/`search` path_prefix 参数；i18n 补 10 key（searchScope×3 + moveConflict×6 + moveToDirSuccess）。验证：前端 lint 0 error（改动文件 prettier 已 fix）/ vitest 156 文件 1139 测试全绿（含 `vaultTreeUi.spec.ts` G3-F1 套件 5 用例 + `useVaultExplorer.g3.spec.ts` 13 用例）/ build 成功；Go `biz/knowledge` + `internal/data` + `service -run Knowledge` 全绿（B4/B7 TDD 用例）。

> **G4 3D 知识图谱完成记录（2026-07-30，设计 §V12.7 B8）**：① B8 后端——proto `rpc ListCollectionGraph(collection_id, link_types[], path_prefix)`；biz `graph.go` `ListCollectionGraph`（库内文档过滤 + 关联过滤 + 端点校验剔除悬空边 + 连接度计算）；data `knowledge_links.go` `ListCollectionLinks`（库级关联读取）；TDD 覆盖节点/边/度/过滤/悬空边。② G4-F 前端——**检索 Tab 按 V12.1 替换为图谱 Tab**：`graphUi.ts` 纯函数（边配色 explicit 蓝/entity 紫/semantic 青、doc_type 调色板稳定哈希、连接度 sqrt 压缩映射、`buildRenderGraph` >2k 节点默认隐藏孤立节点 + 开关放开、连接度降序排序、名称/路径/类型子串过滤、一跳邻居集）；`useKnowledgeGraph.ts` 编排（库/边类型/目录前缀三维过滤 → B8 一次性全量，全选/全不选语义 = 传空数组；切库重置范围与选中；节点搜索/列表/选中/聚焦信号；范围迷你树复用 V12.6 懒加载仅目录）；`KnowledgeGraphCanvas.vue`（3d-force-graph 封装：hover 高亮一跳邻居淡化其余经 accessor 重挂、选中描边色、聚焦相机飞行、代际变化 zoomToFit、ResizeObserver 尺寸跟随、析构 `_destructor`）；`KnowledgeGraph3D.vue` 主面板（左画布 + 右操作台：库 select/边类型 chips/范围选择器 + 孤立节点开关/节点搜索/节点列表徽标度数/选中节点卡「在浏览中打开」）；`KnowledgeScopePicker.vue` 抽取独立组件（搜索框与图谱操作台复用）；「在浏览中打开」跨库先切 `selectedId`（explorer `flush:'sync'` watch 立即复位 prefix）再 `navigateToDocument` 定位。③ 死代码清理——`KnowledgeSearchPanel.vue` 删除；`useKnowledgePage.ts` 移除 search tab 专属状态（`searchQuery/searchTopK/searchMinScore/searchHybridMode/searchRewriteStrategy/searchUseRerank/searchResults/searchLoading/searchRan/searchScopeId/searchScopeOptions/runSearch`）；孤儿 i18n key 清理（`tabSearch/searchScopeAll/searchScopeLabel/searchHintHybrid/searchHintRewrite/searchHintRerank` ×2 语言）。④ 关键修复——模板组件标签 `knowledge-graph-3d` 无法匹配 `KnowledgeGraph3D`（`3D` 的 `D` 需独立分段为 `knowledge-graph-3-d`，否则运行时组件解析失败，eslint `no-unused-vars` 暴露）。验证：前端 eslint 0 problems（改动文件 prettier --fix）/ vitest 7 文件 73 测试全绿（含 `graphUi.spec.ts` 15 用例 + `useKnowledgeGraph.spec.ts` 8 用例）/ `pnpm build` 成功。i18n 补 15 key（tabGraph/graphVaultLabel/graphLinkTypes/graphShowIsolated/graphNodeSearchPlaceholder/graphNodeList/graphNodesEmpty/graphStats/graphIsolatedHidden/graphEmpty/graphLoading/graphSelectedDegree/graphOpenInExplorer/graphSelectedEmpty/linkTypeExplicit 等 ×2 语言）。

> **G4 浏览器运行时复验记录（2026-07-31）**：图谱 Tab 全链路实测——B8 端点 200（7 节点 7 边）、3D 画布渲染、节点列表度数徽标、选中节点卡（连接度 +「在浏览中打开」）、边类型 chips 过滤、节点列表点击聚焦飞行均正常；「在浏览中打开」实测自动切回浏览 Tab + guides 目录定位 + 详情面板（摘要/关联 chips/正文预览）联动正确。**发现并修复 1 处运行时 bug（TDD）**：边类型全部过滤后（0 边数据），断链节点在纯电荷斥力下无界飞散，`zoomToFit` 框住巨大 bbox 把相机拉到无穷远 → 画布空白/渲染伪影（巨大灰色多边形）；`graphUi.ts` 新增 `graphContainmentForce` 径向 containment 力（距离×强度×alpha 拉向原点，强度 0.03 与默认电荷 -30 在半径约 30 处平衡，不压缩连通簇；免 d3-force-3d 依赖，ForceFn 协议自带 initialize），`KnowledgeGraphCanvas.vue` `d3Force('contain', ...)` 注册；复验 0 边状态 3s/13s 布局稳定不再飞散、恢复边后簇正常重连无伪影。验证：前端 lint 0 errors / vitest 158 文件 1165 测试全绿（graphUi.spec.ts 新增 3 用例共 18）/ build 成功。已知非阻塞项：WS `/v1/ws` 握手失败重试（admin 重启后浏览器端会话瞬态，不影响 HTTP 数据链路）。

> **UX 体验整改完成记录（2026-08-05）**：以 dogfood 方式从用户使用角度体验知识库页面（G2 详情面板 / G3 拖拽移动+范围选择器 / G4 图谱），发现 7 个 UX 问题（0 critical / 0 high / 2 medium / 5 low）并全部修复，完整报告见 `test/dogfood-knowledge/report.md`。① ISSUE-001 点击关联 chips 整页下跳丢失上下文（medium）——`KnowledgePage.vue` 左右列 `position: sticky; top: 76px; max-height: calc(100vh-92px)` 列内独立滚动，关联区锚滚不再触发整页滚动；② ISSUE-002 搜索范围选择器二次打开目录树收起（low）——`KnowledgeScopePicker.vue` `onScopeMenuShow` 每次打开强制将库根 key 写入 `scopeExpanded`，q-tree 重新评估 lazy 节点；③ ISSUE-003 内容/关联请求失败后详情面板卡在误导性「解析中」占位且无重试入口（medium）——`useVaultExplorer.ts` 新增 `previewError`/`linksError` 错误态 + `reloadDetail()` 重试（重复点击已选中文档且上次失败时自动重拉），`KnowledgeDocDetail.vue` 内联错误 +「重试」按钮；④ ISSUE-004 目录树加载失败降级后不自动恢复、横幅无重试（low）——`KnowledgeVaultTree.vue` 降级横幅内嵌「重试」按钮触发 `refreshExplorerTree`；⑤ ISSUE-005 新建文档 409 冲突 toast 英文原文且对话框关闭丢输入（low）——`useKnowledgePage.ts` 捕获 409 → 中文 toast「同名文档已存在：{name}」+ 重开对话框保留输入；⑥ ISSUE-006 零计数关联 chip 可点击但无反馈（low）——`KnowledgeDocDetail.vue` chips `:clickable="c.count > 0"` 禁用；⑦ ISSUE-007 关联列表同一文档按方向重复出现、chip 计数与列表行数口径不一（low）——`useVaultExplorer.ts` `linkCounts` 改按 `target_doc_id` 聚合去重（计数 = 不重复文档数），`KnowledgeDocDetail.vue` 关联列表按文档合并方向（互引/本文引用/被引用于，新增 i18n `linkDirBoth`）。i18n 补 5 key（linkDirBoth/retry/contentLoadError/linksLoadError/docAlreadyExists ×2 语言）。验证：前端 `pnpm lint` 0 errors / vitest 161 文件 1189 测试全绿（含新增 `useVaultExplorer.ux.spec.ts` 错误态置位/通知/重试清除 + 聚合计数用例）/ `pnpm build` 成功；浏览器运行时复验——sticky 计算样式生效且点 chip 后 `window.scrollY` 保持 0、范围选择器首次/二次打开均自动展开库根并懒加载子目录、零计数 chip `cursor: auto` 非零 `cursor: pointer`、readme.md 关联列表 3 行不重复文档（setup.md 合并为「互引」单条）且 chip 计数 3 与列表行数一致，console 0 error。**顺带修复阻断性缺陷**：`sql/migrations/20261125_memory_fact_three_counters.sql` 注释内含分号被 `splitDDLStatements`（comment-unaware）截断 → P1 迁移失败 → ReadinessGate 不打开 → 全站 503；注释分号改逗号并加注警示后迁移成功、healthz 200。设计契约同步：设计 §V12.4（计数口径/聚合/错误态/粘性布局）+ §V12.6（每次打开自动展开库根）。

### P4a：L0 强 BM25 栈（含 R-5 选型 spike）

| # | 任务 |
|---|------|
| P4a-1 | **R-5 spike（前置，禁止跳过）**：Bleve vs FTS5(trigram) vs 自研倒排三方各 200 行验证 + 真实语料质量对比，产出选型结论 |
| P4a-2 | 按选型实施：CJK bigram+unigram 分词、字段加权（title×20/tags×5/body×1）、文件名索引 |
| P4a-3 | 查询扩展（RM3 伪相关反馈 / LLM 关键词扩展） |
| P4a-4 | S-5 测试基线：50 条中英查询金标准，BM25 召回质量纳入回归 |
| P4a-5 | 替换/旁路现有失效的 PG `ts_rank` 中文检索路径（设计 §V11-1） |

验收：spike 报告落档；中文查询召回达到金标准基线；索引可 DROP/REBUILD 无状态重建。

### P4b：L2 语义层插件化（含 R-4 契约变更）

| # | 任务 |
|---|------|
| P4b-1 | **R-4 契约变更四条**：CreateVault EmbeddingModel 改可选（空=无语义层）— ✅ 已落地（2026-07-29，Proto/CLI/CreateVault 全链路）；knowledge_search 无语义层自动降级 L0 — ✅ 已落地（2026-07-29 晚 F5，`retriever.go`/`hybrid_retriever.go` BM25 降级，见上方验证记录）；摄取流水线对无 embedding Vault 跳过向量写入 — ✅ 已落地（2026-07-30，`IngestDocument` 词法库传 nil embedder + `BuildIndexedChunks` 对 `ErrEmbedderNotConfigured` 降级空向量，见上方 G1 记录）；前端 embedding 配置改「可选增强」— ⏳ |
| P4b-2 | Embedder 接口抽象整理：本地模型（ONNX/Ollama）/ model2vec 静态查表 / 远程 API 三实现可插拔 |
| P4b-3 | model2vec 静态查表后端（自实现查表，不依赖社区移植；向量按 `(model,dim)` 命名空间隔离 S-3） |
| P4b-4 | semantic 关联轨落地；无语义层时「相关推荐」降级为同文件夹+共享标签+双链共现 |
| P4b-5 | 有/无语义层双套测试矩阵 |

验收：无 embedding 的 Vault 全功能可用（L0+L1）；配置 model2vec 后语义检索生效；`ErrEmbeddingModelRequired` 契约回归测试通过。

### P5：Agent 工具族（含 R-6 安全契约）

| # | 任务 |
|---|------|
| P5-1 | `knowledge_navigate`：tree（≤1k token）→ card（≤200 token）→ read（分页全文）三级下钻 + token 预算 + 超限截断提示 |
| P5-2 | `knowledge_grep`：内容正则/字面搜索（只读） |
| P5-3 | **R-6 安全契约**：knowledge_write 路径 sanitize（禁 `..`/symlink 逃逸/限制 vault root 内）+ 覆盖前自动备份 `.aranea/trash` + 审计日志（who/when/what 结构化入 activities）；watcher 对 KB 自写文件打标防回环重复摄取 |
| P5-4 | 工具经 `internal/tools/` Registry 注册（ToolKeyKnowledge* 常量）+ agent 提示词同步 |

验收：navigate/grep 只读可用；write 越权路径被拒、覆盖有备份、审计可查；agent 写入不触发回环重复摄取。

### P6：迁移 Collection → Vault

| # | 任务 |
|---|------|
| P6-1 | 迁移器：content_text 导出 .md（文件名清洗 + 冲突处理），幂等可重入 + 失败单条跳过 + `schema_migrations` 门控 |
| P6-2 | 迁移期 Vault `migrating` 态：检索走旧索引、写入排队（S-2） |
| P6-3 | 旧表只读兜底一个版本周期；迁移代码完成后标记废弃 |

验收：存量 Collection 可完整迁移为 Vault，迁移期检索不中断，重复执行幂等。

### 依赖关系

```
P1（Vault 基础）──→ P2（双向同步+摘要卡+关联）──→ P3（UI）
                  └──→ P4a（L0 强 BM25，spike 前置）──→ P4b（L2 插件化）──→ P6（迁移）
                  └──→ P5（Agent 工具族，R-6 前置）可与 P3/P4 并行
```

---

## 子模块：图谱深空版与实体治理（G5）Phase 计划

> **设计契约**：[37-knowledge.design.md §V12.8](./37-knowledge.design.md#v128-图谱深空版渲染层--实体治理g52026-08-07-评审通过)
> **需求**：[37-knowledge.md §子模块：图谱深空版与实体治理（G5 需求）](./37-knowledge.md#子模块图谱深空版与实体治理g5-需求2026-08-07)（US-21~US-23、FR-G5-1~11、NFR-G5-1~5、验收 30~37）
> **调研依据**：[2026-08-07-research-knowledge-graph-oss.md](../reports/2026-08-07-research-knowledge-graph-oss.md)（fast-graph 主蓝本 / orrery 视觉蓝本 / jarvis-ui HUD·分级蓝本 / Simple Graph Builder 消歧蓝本）
> **状态**：📋 待启动 | 用户已裁决：自研渲染层；深空星河 + HUD 操作台；粒子流完整复刻（时变彩虹色保留）；`3d-force-graph` 依赖完全移除；实体消歧与前端一轮做完。

### 总览

| Phase | 内容 | 关联契约 | 状态 |
|-------|------|---------|------|
| **G5-A** | 引擎内核（纯 TS，零 Vue/three 依赖，TDD） | FR-G5-1/2/5、NFR-G5-2/3 | ✅ |
| **G5-B** | 物理执行层（Worker 协议 + 主线程兜底） | FR-G5-1、NFR-G5-5 | ✅ |
| **G5-C** | 渲染层（Node/Edge/Backdrop/Bloom/Label/Picker + Canvas 装配） | FR-G5-1/3/6 | ✅（v2 重写，见 As-built） |
| **G5-D** | 交互全集 + 粒子流 | FR-G5-4/7 | ✅ |
| **G5-E** | HUD 操作台换肤 + 新控件 | FR-G5-8、NFR-G5-4 | ✅ |
| **G5-F** | 实体治理后端（B9~B12） | FR-G5-9/10/11 | ✅ |
| **G5-G** | 治理前端 + 移除清单 + 全量验证 | FR-G5-10、验收 30~37 | 🟡（G-1/G-2/G-4 ✅；G-3 性能基准 📋，见该节 As-built） |

### G5-A：引擎内核（纯 TS）

| # | 任务 | 涉及文件 |
|---|------|----------|
| A-1 | SoA 图模型：positions/velocities Float32Array(3N)、degree Uint16、groupId Uint16、edges Int32Array(2E)；docId↔index 双射；确定性播种 mulberry32 + 球内体采样 `r=(cbrt(N)*20+1)*cbrt(rand)` | `web/src/features/knowledge/graph3d/model.ts`（新增） |
| A-2 | typed-array 八叉树池（Float32 8/cell + Int32 9/cell，容量 16N 倍增，显式栈迭代，质心除法延迟到查询） | `graph3d/octree.ts`（新增） |
| A-3 | 物理引擎：BH 斥力（repulsion=30,theta=0.8) + 弹簧（0.05/30) + 簇凝聚（0.08) + 簇分离（100·count/d²) + 向心力（0.011)；显式 Euler damping=0.9；**maxStep 位移钳制（≤linkDistance)**；alphaDecay=0.0228，alphaMin=0.005 | `graph3d/forces.ts`（新增） |
| A-4 | 节点三层分级：supernode=degree≥15；ultranode=连接≥4 个不同 supernode；尺寸倍率 1.0/1.5/2.5；分层 charge(-120/-200/-350) | `graph3d/tiering.ts`（新增） |
| A-5 | 分组调色板：doc_type 稳定哈希取色（沿用 G4 graphUi 调色板语义） | `graph3d/palette.ts`（新增） |
| A-6 | 粒子流纯数学：相位均布 `prog[i]=i/n`、easeInOutQuad、时变 HSL `hue=0.5+0.32·sin((t·0.6+p·2.2+i·0.12)·π)` | `graph3d/particleMath.ts`（新增） |

验收：全部纯函数单测覆盖（确定性播种同数据同布局；maxStep 钳制不发散；八叉树查询正确性；分级阈值；粒子相位均布）。

### G5-B：物理执行层

| # | 任务 | 涉及文件 |
|---|------|----------|
| B-1 | Worker 消息协议：init(slice 后 transfer)/setParams/pin/unpin/reheat/stop ↔ tick{positions,alpha}/stopped/error | `graph3d/protocol.ts`（新增） |
| B-2 | 物理 Worker（16ms tick，alpha<alphaMin 自停发 stopped；Vite `?worker` 引入） | `graph3d/physics.worker.ts`（新增） |
| B-3 | 主线程兜底客户端：Worker 创建失败 → RAF 跑同一 `forces.ts` 引擎（NFR-G5-5） | `graph3d/engine.ts`（新增，纯 TS 装配） |

验收：协议消息单测；Worker/主线程两路径同一引擎行为一致（同输入同 positions 序列）。

### G5-C：渲染层

| # | 任务 | 涉及文件 |
|---|------|----------|
| C-1 | NodeLayer：InstancedMesh 低模球（6,4)+MeshBasicMaterial（加法混合）；instanceColor + baseColors 缓存 + lerp(white,0.5) 高亮；大小=base+sqrt(degree)·scale × 分级倍率 | `components/knowledge/graph3d/render/NodeLayer.ts`（新增） |
| C-2 | EdgeLayer：微弯 Bezier 边（QuadraticBezierCurve3，bow 0.3·len，垂直轴 hash01("s->t")·2π 定向，6 段）；单 LineSegments + vertexColors（rest=边类型色×0.32，hover 关联边=×0.9 瞬时换色） | `render/EdgeLayer.ts`（新增） |
| C-3 | BackdropLayer：FBM 星云反转球（3-octave，colA 紫（0.12,0.06,0.22)/colB 青（0.05,0.17,0.21)，bright=0.5，pow(fbm,2.2) 压在 bloom 阈值下）+ 三档星空（dim 2400/med 4800/bright 800，球面均匀，sizeAttenuation:false，64px 柔光 dotTexture）+ 核雾（520 颗加法 Points，布局收敛后锚定度数最大 hub） | `render/BackdropLayer.ts`（新增） |
| C-4 | BloomPipeline：EffectComposer（RenderPass + UnrealBloomPass strength≈1.2/radius=0.5/threshold=0.28，半分辨率 w/2×h/2，nMips=3）；ACESFilmicToneMapping exposure=1.2；**不透明深空底 #050810**（bloom 与透明背景不兼容）；strength=0 时整 pass enabled=false | `render/BloomPipeline.ts`（新增） |
| C-5 | LabelLayer：three-spritetext 标签（挂节点子对象，LABEL_HEIGHT/r 抵消父缩放）；相机距离/节点度数双阈值，密图不糊屏 | `render/LabelLayer.ts`（新增） |
| C-6 | Picker：Raycaster 逐实例求交 + mousemove 去抖（同时防粒子相位重置） | `render/Picker.ts`（新增） |
| C-7 | Canvas 装配：Renderer+Worker 客户端+交互桥；**lazy-render**（needsRender \|\| particles.active \|\| autoRotate 才 composer.render()）；IntersectionObserver 离屏 pauseAnimation | `components/knowledge/graph3d/KnowledgeGraph3DCanvas.vue`（新增） |

验收：渲染层组件级快照/像素断言可行部分单测；收敛静置后 RAF 不再过 GPU（lazy-render 计数断言）；WebGL 不可用友好占位。

> **As-built（2026-08-08，渲染管线 v2 —— 万级性能 + 降亮度 + 科幻视觉）**：用户反馈「卡顿/太亮/学习 Obsidian 更科幻」，C-1/C-2/C-6 全量重写为 GPU 位置纹理管线，新增 C-8/C-9/C-10：
>
> | # | 落地 | 偏差 |
> |---|------|------|
> | C-1 v2 | `render/NodeLayer.ts`：InstancedMesh → `THREE.Points`（1 节点=1 顶点，gl_VertexID→texelFetch 取位）；Obsidian 柔光点（core+halo 径向衰减、亚像素 vFade）；**普通混合替代加法混合**（重叠不烧白）；动态属性仅 aEmph | 原设计 InstancedMesh 每 tick 重组矩阵上传 640KB，万级不可行 |
> | C-2 v2 | `render/EdgeLayer.ts`：6 段贝塞尔 → **Obsidian 细直线**（每边 2 顶点，顶点量 ÷6）；rest α=0.16；hover 关联边提亮 + 流动光脉冲（sin 沿边跑动数据流） | 用户裁决：直线（Obsidian 风） |
> | C-3 v2 | `render/BackdropLayer.ts`：星云**烘焙** 1024×512 equirect RT（弃每帧全屏 FBM ≈60M hash/帧）；bright 0.5→0.34、星空 ×0.65、核雾 0.2→0.1（全压 bloom 阈值 0.55 下） | 降亮度需求驱动 |
> | C-4 v2 | `render/BloomPipeline.ts`：threshold 0.28→0.55、strength 1.2→0.9（只有真高亮冒辉光） | 降亮度需求驱动 |
> | C-6 v2 | `render/Picker.ts` + `features/knowledge/graph3d/pickMath.ts`：弃 Raycaster 逐实例矩阵求逆 → 自研射线-球 O(N) 纯循环；slack 随距离放大保远节点可点；最近 t 保遮挡 | 原方案每实例矩阵求逆 = 万级 hover 卡顿元凶 |
> | C-8 新增 | `render/PositionTexture.ts` + `textureLayout.ts`：RGBA32F DataTexture（⌈√N⌉² 取整）；每 tick 一次 memcpy + 纹理上传（万级 ≈0.3ms），节点/边/瞄准具共享取位 | v2 管线核心基础设施 |
> | C-9 新增 | `render/ReticleLayer.ts`：HUD 瞄准具（hover 圆环 + 选中六边形，视空间 billboard，uniform 索引驱动） | 科幻增强用户裁决「全套」 |
> | C-10 新增 | `features/knowledge/graph3d/qualityTiers.ts`：自适应画质 HIGH/MID/LOW（初始按节点数 ≥2500/≥8000 分级；FPS EMA governor 连续 90 帧 <45fps 降档 / 600 帧 >57fps 升档不超初始档顶）；档规格驱动 bloom/pixelRatio/标签候选（200/100/40）；HUD 档位指示 | 自适应降级用户裁决「允许」 |
>
> C-5/C-7 按原设计落地（LabelLayer 候选池上限改由画质档驱动）。测试：graph3d 域 15 文件 106 用例全绿（新增 textureLayout/pickMath/qualityTiers/ReticleLayer 4 套）；全量 `pnpm lint`（0 错误）+ `pnpm test`（199 文件/1489 用例）+ `pnpm build` 通过。设计同步：[37-knowledge.design.md §V12.8-1 As-built](./37-knowledge.design.md#v128-图谱深空版渲染层--实体治理g52026-08-07-评审通过)。

### G5-D：交互全集 + 粒子流

| # | 任务 | 涉及文件 |
|---|------|----------|
| D-1 | hover 一跳邻居：全边表 O(E) 扫描 + 去抖；邻居集驱动 NodeLayer 提亮、其余压暗（lerp 0.08）、EdgeLayer 关联边换色、ParticleLayer 发射 | `graph3d/engine.ts`、render 各层 |
| D-2 | pin 拖拽：pin-and-move（拖拽平面法线朝相机 + grabOffset 防跳变）；拖拽中挂起 controls/autoRotate；fx/fy/fz 写 Worker pin 消息；只重写关联边顶点 | `KnowledgeGraph3DCanvas.vue`、`protocol.ts` |
| D-3 | zoom-to-cursor：滚轮射线 ∩ 过 target 面向相机平面求 pivot，相机+target 同步缩放 `0.95^(-ΔY·0.01)` | `KnowledgeGraph3DCanvas.vue` |
| D-4 | 交互状态机：shown(hover)/selected（单击锁定） 分离；位移 <5px 区分拖拽与点击；双击 =「在浏览中打开」（沿用 G4 跨 tab 定位链路） | `graph3d/engine.ts`、`KnowledgeGraph3D.vue` |
| D-5 | 局部图谱：N 跳 BFS（1-4）→ 子图重建（新 GraphModel + 重播种），**groupId 沿原图复制保持跨视图颜色一致**；「返回全局」恢复 | `graph3d/model.ts`、`useKnowledgeGraph.ts` |
| D-6 | ParticleLayer：粒子流 1:1 复刻（MAX=80、SPEED=0.45/s、PointsMaterial size=8 + 64px 径向渐变 glowTexture + vertexColors + depthWrite:false；时变彩虹色） | `render/ParticleLayer.ts`（新增） |

验收：交互状态机单测；局部图谱颜色与全局一致断言；pin 后位置持久；双击触发「在浏览中打开」。

### G5-E：HUD 操作台换肤

| # | 任务 | 涉及文件 |
|---|------|----------|
| E-1 | `.kg-hud` 作用域皮肤（**不改全局主题 token**，NFR-G5-4）：等宽字体（'JetBrains Mono', Consolas, monospace）、主青 #00d4ff、边色 #1a3a4a、letter-spacing:0.08em、面板 rgba(5,8,16,0.88) + box-shadow:0 0 15px #00d4ff22 + 1px 青色描边、`[ ON ]/[ OFF ]` 括号式开关、边类型图例发光色块 | `KnowledgeGraph3D.vue`（改造）、样式 |
| E-2 | 新增 HUD 控件：auto-rotate 开关、标签开关、「聚焦邻域」（跳数 1-4 步进）+「返回全局」；画布左下统计（节点/边数）、右上浮动工具条（适应视图/图例/auto-rotate/标签/返回全局） | `KnowledgeGraph3D.vue`、`KnowledgeGraph3DCanvas.vue` |
| E-3 | 保留 G4 全部控件与 `KnowledgeScopePicker` 复用，功能不回归；全部文案入 locale（check-i18n 红线） | `KnowledgeGraph3D.vue`、locales |

验收：亮色/暗色主题下图谱 Tab 均为深空风；全局主题 token 零改动；G4 控件功能不回归。

### G5-F：实体治理后端（B9~B12）

| # | 任务 | 涉及文件 |
|---|------|----------|
| F-1 | `NormalizeEntityName` 纯函数：Unicode NFC + case-fold（unicode.CaseRanges 语义无损小写）+ 内部空白折叠单空格 + 去首尾 | `internal/biz/knowledge/`（新增，TDD） |
| F-2 | DDL 迁移：`knowledge_entities` 加 `name_norm TEXT` + `UNIQUE(collection_id, name_norm)`；新建 `knowledge_entity_aliases(collection_id, alias_norm, entity_id)` 表；L3 Go 数据迁移回填 name_norm（PG 无 NFC，不可 DB 侧回填），冲突组按 id 最小者为 keeper 自动合并并落别名；迁移幂等（IF NOT EXISTS） | `internal/data/sql/migrations/`（新增）、`ddl_migration_registry.go`、L3 数据迁移 |
| F-3 | `ReplaceDocEntities` 按 name_norm 查/建字典条目（name 保留首见写法作展示名）+ 别名命中 keeper；解析管线：归一化 → 精确 name_norm → 别名 → (embedding ≥0.90 自动合并入别名表） → 新建；`FindEntityCooccurrences` 改按 entity_id 关联 | `internal/data/knowledge_links.go`、`internal/knowledge/vault_entity.go` |
| F-4 | Proto：`rpc MergeKnowledgeEntities(collection_id, keeper_id, mergee_ids[])` + `rpc ListEntityMergeSuggestions(collection_id)` + `make api` | `api/kratos/knowledge/v1/knowledge.proto` |
| F-5 | MergeKnowledgeEntities：`Data.ExecInTx` 重写 mergee 的 `knowledge_doc_entities`（mentions 求和，冲突合并）+ `knowledge_links` entity 轨 context 引用重写 + mergee 条目删除 + name/name_norm 落 keeper 别名；返回 `{rewritten_mentions, rewritten_links}`；流程日志 step `knowledge.entity.merge`（登记 `internal/event/flow_log.go` stepTitleRegistry + 同步 `docs/development/52-flow-logger.design.md` §5.1 步骤注册表） | `internal/biz/knowledge/`、`internal/data/knowledge_links.go`、`internal/event/flow_log.go` |
| F-6 | ListEntityMergeSuggestions：归一化冲突组（name 不同 name_norm 相同）+ 配置 embedding 时高相似对（余弦 ≥0.90 标 auto、0.80-0.90 标 suggest）；embedding 未配置仅返回 norm 组（对齐 NFR-15）；实时计算不落队列表（YAGNI） | `internal/biz/knowledge/`、`internal/service/knowledge.go` |

验收："AI"/"ai"/"ＡＩ" 聚合为同一实体；合并事务重写条数正确返回；合并后别名命中跨同步持久；无 embedding 时归一化与手动合并全功能可用。

> **As-built（2026-08-08，全部验收通过）**：F-1 `internal/biz/knowledge/entity_norm.go`(+test)；F-2 `internal/data/sql/migrations/20261129_knowledge_entity_governance.sql` + `ddl_migration_registry.go` + L3 回填/冲突组合并（与 B10 共享 helper）；F-3 `internal/data/knowledge_links.go`（`ReplaceDocEntities` 归一化/别名/同批求和/孤儿清理、`FindEntityCooccurrences` 按 entity_id）+ `internal/knowledge/vault_entity.go` 适配；F-4 `api/kratos/knowledge/v1/knowledge.proto`（`MergeKnowledgeEntities` POST `/v1/knowledge/vaults/{collection_id}/entity-merges`、`ListEntityMergeSuggestions` GET `/v1/knowledge/vaults/{collection_id}/entity-merge-suggestions`）；F-5 `internal/data/knowledge_entities.go`（`MergeEntities`/`ListEntities` + 共享 helper）+ `internal/service/knowledge_governance.go` + step `knowledge.entity.merge` 登记（`internal/event/flow_log.go` + 52-flow-logger §5.1）；F-6 `internal/biz/knowledge/entity_suggest.go`(+test) + `knowledge_governance.go` handler（embedder 经构造函数注入，nil/失败降级 norm-only）。测试：data PG 实测 4 项、service 6 项、biz 归一化/建议矩阵全绿；`go build ./...` + 定向 vet 通过（干净 GOCACHE）。设计偏差：解析管线「embedding ≥0.90 自动合并」步骤未接线（YAGNI，仅走 B11 建议由用户确认），详见 [37-knowledge.design.md §V12.8-3 As-built](./37-knowledge.design.md#v12-图谱深空版渲染层--实体治理g52026-08-07-评审通过)。

### G5-G：治理前端 + 移除清单 + 全量验证

| # | 任务 | 状态 | 涉及文件 |
|---|------|------|----------|
| G-1 | HUD「实体治理」分区：合并建议列表（保留名 ← 候选名、来源徽标 norm/embedding、相似度）+ 一键合并 + 重写条数内联反馈；api.ts + store 接线 | ✅ | `KnowledgeGraph3D.vue`、`api.ts`、`useKnowledgeGraph.ts`、`types.ts` |
| G-2 | 移除清单执行：`3d-force-graph` 从 package.json 移除（新增 `three-spritetext`）；`KnowledgeGraphCanvas.vue` 删除；`graphUi.ts graphContainmentForce` 删除（配色/排序/过滤/一跳邻居纯函数保留复用）；Grep 全局搜索确认零残留引用 | ✅ | `web/package.json`、`KnowledgeGraphCanvas.vue`、`graphUi.ts` |
| G-3 | 性能基准：2 万节点/5 万边合成数据集交互帧率记录入测试文档；布局收敛静置 CPU/GPU 零占用断言 | 📋 | `docs/testing/`（性能基准记录） |
| G-4 | 全量验证：前端 `pnpm lint && pnpm test && pnpm build`；后端 `make api && make wire && make build && make test && make lint`（干净 GOCACHE）；浏览器运行时复验（深空视觉/hover 粒子流/pin/局部图谱/合并治理全链路，对照验收 30~37） | ✅ | — |

验收：验收标准 30~37 全部通过；移除清单零残留；性能基准落档。

> **As-built（2026-08-08，G-1/G-2/G-4 完成）**：
>
> - **G-1 治理前端**：`KnowledgeGraph3D.vue` 新增「实体治理」分区（建议列表限高滚动、norm/embedding 来源徽标、embedding 对相似度两位小数、合并按钮防重入、`merged: 重写 N 处提及 · M 条关联` 内联反馈）；`api.ts` 新增 `listEntityMergeSuggestions`/`mergeKnowledgeEntities`（snake/camel 双键映射对齐既有风格）；`useKnowledgeGraph.ts` 新增 `mergeSuggestions`/`merging`/`lastMergeResult` 状态与 `mergeEntities()`——建议只随库加载（与边类型/目录过滤无关），拉取失败降级空列表不置主 error（辅助数据不阻断图谱），合并成功并行重拉图谱与建议；`types.ts` 新增 `EntityMergeSuggestion`/`MergeEntitiesResult`；i18n 键 `knowledgePage.graphEntityGovernance`/`mergeSource.*`/`mergeAction`/`mergeNoSuggestions`/`mergeFeedback`（zh+en）。单测：`useKnowledgeGraph.spec.ts` 新增 2 用例（建议随库加载+失败降级；合并→重拉→反馈置位）。
> - **G-4 浏览器复验修复（标签可见性）**：复验发现两标签缺陷并修复——① `maxDistance = fitDist × 0.85` 致适应视图后候选标签全隐藏，修为 `fitDist + 半径`（适应视图即全候选可达，拉远按距离渐进隐藏）；② 固定 `minDegree=4` 致小图（最大度数 < 4）无任何标签，新增纯函数 `effectiveMinDegree(maxDegree, base)` 降档到图最大度数（全孤立图钳 1，孤立节点不出标签）；hover/选中 forced 标签豁免距离/度数/开关三重阈值（labels OFF 时悬停/选中仍出标签，已浏览器实证）。`LabelLayer.spec.ts` 新增 `effectiveMinDegree` 3 用例。
> - **G-4 复验记录**：合并全链路（造数 norm 冲突三元组 OpenAI/OPENAI/" OpenAI " → 建议出现 → 一键合并 → 反馈「重写 1 处提及 · 0 条关联」→ 建议清空 → DB 验 keeper/别名/提及重写与唯一索引完好，测试数据已清理）；标签修复前后对照；labels 开关切换；hover 瞄准具+forced 标签；选中六边形；浅色主题渲染；边类型 chips。对照验收 30~37：30/31/32/33/35/36/37 通过，34 的性能基准部分随 G-3 落档后关闭。
> - **G-3 遗留**：2 万节点/5 万边合成数据集性能基准与静置零占用断言未做（G5-C v2 管线已实测 1 万节点/2 万边 ≈100FPS、Worker 物理 48ms tick 不阻塞主线程，可作基准起点参考）。

### 依赖关系

```
G5-A（引擎内核）──→ G5-B（Worker）──→ G5-C（渲染层）──→ G5-D（交互+粒子流）──→ G5-E（HUD）──→ G5-G（验证）
G5-F（后端治理，独立可并行）──────────────────────────────────────────────────────↗
```

> **反模式红线**（调研 §8，实施时必须规避）：① bloom 必须不透明深空底 clear（alpha:false）；② 星云/核雾亮度压在 bloom threshold 下防糊屏；③ 力布局必须 maxStep 钳制防 hub 发散；④ Worker init 先 slice 再 transfer；⑤ 拖拽挂起 controls/autoRotate，恢复时 hoverId 指向刚放下节点；⑥ 子图重建 groupId 沿原图复制保色。

---

## 子模块：双模块级知识内核（SP1）Phase 计划

> **设计契约**：[37-knowledge.design.md §子模块：双模块级知识内核（SP1）](./37-knowledge.design.md#子模块双模块级知识内核sp12026-08-08-评审通过)（S1~S11）
> **需求**：[37-knowledge.md §子模块：双模块级知识内核（SP1）](./37-knowledge.md#子模块双模块级知识内核sp12026-08-08-评审通过)（US-24~US-29、F-SP1-1~11、NFR-SP1-1~4、验收 38~44）
> **调研依据**：[2026-08-08-research-pkm-obsidian-blueprint.md](../reports/2026-08-08-research-pkm-obsidian-blueprint.md)（学术 × 开源 × Obsidian 逆向 × SiYuan 源码四路调研；SiYuan 证据锚点 `test/pkm-research/D-siyuan-kernel.md` §10）
> **状态**：🟡 进行中（SP1-A/B/C/D/E/F 已建成） | 用户已裁决：SP1 做块级双链完整粒度；批准双模统一架构 + SP1 范围落档三件套。2026-08-08 深入评审通过：B-1~B-4 修订已合入下列任务（C-1/C-3/D-1/D-2/F-2/G-2）。
> **License 纪律**：SiYuan（AGPL）/Logseq（AGPL）仅借鉴思路，全部逐行自研；解析器用 goldmark（MIT）扩展。

### 总览

| Phase | 内容 | 关联契约 | 状态 |
|-------|------|---------|------|
| **SP1-A** | 块解析管线 blockparse 包（纯函数，TDD） | F-SP1-1/2、NFR-SP1-1 | ✅ |
| **SP1-B** | 块/refs 物化表（DDL 迁移 + ReplaceDocBlocks Repo） | F-SP1-3、NFR-SP1-2 | ✅ |
| **SP1-C** | Resolver + 写路径接线 + explicit 轨投影 | F-SP1-3/8、S3/S4 | ✅ |
| **SP1-D** | 统一链接索引 LinkIndex + WS 增量 | F-SP1-4、NFR-SP1-4、S5 | ✅ |
| **SP1-E** | 块级反链 API（含 dangling 语义） | F-SP1-5、S8 | ✅ |
| **SP1-F** | 团队库后端（vault_backend 维度） | F-SP1-6、S6 | ✅ |
| **SP1-G** | 晋升（复制式）+ 删除同步 | F-SP1-7/8、S7 | ✅ |
| **SP1-H** | RebuildIndex + 惰性锚点回填 | F-SP1-9/10、NFR-SP1-3、S9 | ✅ |
| **SP1-I** | 前端（反链分组/dangling 灰显/晋升 UI/WS 订阅） | 验收 38~44、交互规格 | ✅ |

### SP1-A：块解析管线 blockparse 包（纯函数）

| # | 任务 | 涉及文件 |
|---|------|----------|
| A-1 | goldmark wikilink inline parser 扩展（spike 定案：AST 扩展 vs 前置正则切分，倾向 AST 扩展保位置准确；goldmark 不原生支持 wikilink） | `internal/knowledge/blockparse/`（新增，TDD） |
| A-2 | AST 块切分 → `BlockRow`：heading/paragraph/list_item/code_block/blockquote/table/math；frontmatter 不切块；heading 块产 `heading_path`；每块 `content_hash` + `text_excerpt`（前 200 字符）+ ordinal | `blockparse/blocks.go`（新增） |
| A-3 | 四语法 + `\|别名` 变体产 `RefRow`：`raw_target`（原文）、`context`（±50 字符）、`syntax`、`edge_type`（ref/embed） | `blockparse/refs.go`（新增） |
| A-4 | `Parse(docKey, markdown []byte) (blocks, refs, err)` 纯函数契约收口（无 IO、无全局态） | `blockparse/parse.go`（新增） |

验收：语法矩阵单测全覆盖（NFR-SP1-1）；同输入同输出（纯函数确定性）；dangling 目标产 raw_target 不报错。

> **2026-08-08 完成**：spike 定案——goldmark AST 负责块结构与 CodeSpan 排除，wikilink 扫描器在 AST 定位的连续文本 run 上扫描（保证源码位置准确，Context ±50 rune 直接从源文本取窗口）。实现合并为单文件 `internal/knowledge/blockparse/parse.go`（A-1~A-4 同文件，包内内聚优于拆分）。验证：`go test ./internal/knowledge/blockparse/ -count=1 -v` 13 测试函数 + 25 语法矩阵子用例全绿；`go vet` 零告警。

### SP1-B：块/refs 物化表

| # | 任务 | 涉及文件 |
|---|------|----------|
| B-1 | ~~Ent Schema `KnowledgeBlock`~~（**2026-08-08 裁决改道**：弃 Ent，Raw SQL 域内一致——knowledge 域全 TEXT id Raw SQL，uuid FK 物理不可建；派生索引表批量写 Ent 无收益；裁决 D1~D6 全文见设计文档 S2 修订注记） | `internal/data/sql/migrations/20261203_knowledge_blocks.sql`（新增） |
| B-2 | DDL 迁移：`knowledge_blocks` 部分唯一索引 UNIQUE(collection_id, anchor) WHERE anchor IS NOT NULL；`knowledge_block_refs` 表（src_block_id FK ON DELETE CASCADE、dst_block_id/dst_doc_id FK ON DELETE SET NULL 转 dangling、索引 (dst_block_id)/(dst_doc_id)/(collection_id, raw_target)/(src_block_id)）；注册 `ddl_migration_registry.go`（Version 20261203）；幂等（IF NOT EXISTS） | 同上 + `ddl_migration_registry.go` |
| B-3 | Repo `ReplaceDocBlocks`：`PostgresExecInTx` 内整文档删了重插（删块 FK 级联清出向边；指向旧块的入向边 FK 自动置 NULL 保 raw_target 转 dangling；插新块 → 插新 refs）；refs 不做 diff；块 content_hash diff 仅供跳过 embedding/FTS 重算。块 ID 规则：显式锚定后 = `^anchor`，未锚块存储层生成随机 hex | `internal/data/knowledge_blocks.go`（新增）、`internal/biz/knowledge/block_index.go`（窄接口 `BlockIndexRepo`，Stability:evolving） |

验收：整文档重建后零孤儿边（NFR-SP1-2）；同一文档重复重建结果一致（幂等）；旧块 dst 引用正确转 dangling。

> **2026-08-08 完成（TDD）**：5 个 PG 集成测试全绿——Basic（锚块 ID=anchor、refs ordinal 映射、dangling 初态 dst 全 NULL）、Rebuild（重复重建不累积、内容变更旧边清零）、DstDangling（目标块重建删除后 dst 置 NULL 保 raw_target）、AnchorUniquePerCollection（库级锚冲突映射 CodeConflict、无锚块不受影响）、DocDeleteCascade（文档删除级联清块、入向边转 dangling）。验证：`go test ./internal/data/ -run TestKnowledgeBlocks -count=1` 5/5 PASS；`go build ./...` + `go vet` + gofmt 全净。

### SP1-C：Resolver + 写路径接线

| # | 任务 | 涉及文件 |
|---|------|----------|
| C-1 | Resolver：`raw_target` → `dst_doc_id/dst_block_id`（文档键最短路径唯一匹配——rel_path 无扩展名/标题/别名；`#heading` 按 heading_path 定位；`#^anchor` 按 anchor 定位；查不到 → dst NULL + dangling）；两阶段分离（目标后创建重跑 Resolver 即复活，不重解析源文档）；**跨库规则（B-1）**：同库优先 → 可见库最短路径 → 多义按（collection 创建序、路径字典序）取首记 `ambiguous=true`；不可见库不参与匹配 | `internal/biz/knowledge/link_resolver.go`（新增，TDD） |
| C-2 | 写路径接线：VaultSyncApplier 事件 → blockparse → ReplaceDocBlocks；「粘贴文本入库」与 agent knowledge_write 路径同接（local/team 同管线） | `internal/knowledge/vault_sync.go`、`internal/biz/knowledge/`（写路径） |
| C-3 | explicit 轨投影：块级 refs 同文档去重聚合 → `knowledge_links` explicit 轨（G5 图谱/关联区消费方式不变；entity/semantic 轨不动）；**权重规则（N-3）**：同文档对聚合为一条文档边，`meta.weight=块边数` | `internal/data/knowledge_links.go`（适配） |

验收：Vault 文件写入后块/边落库正确；标题改名后 `[[doc#标题]]` 按最新解析不存死 ID；G5 图谱/关联区数据语义不回归。

> **2026-08-08 完成（TDD）**：
> - **C-1 Resolver**：`LinkResolver.ResolveRefs`（`internal/biz/knowledge/link_resolver.go`）——同库优先 → 可见库最短路径 → 多义按（collection created_at、路径字典序）取首记 `Ambiguous=true`；不可见库由调用方传入的 visibleCollectionIDs 裁剪（防文档名泄漏）；`#^anchor`/`#heading` 块级定位经 `ResolveIndex` 端口；查不到 → dst 留空转 dangling 保 raw_target；自文档引用走内存态块产 `DstSelfOrdinal`；15 个测试函数全绿（同库优先/最短路径/多义确定性/可见性/title+alias/大小写/embed 资产/heading/anchor/块失文档存/文档失 dangling/自引用锚/自引用标题/端口故障上抛/纯自引用零查询）。
> - **C-2 写路径接线**：`Usecase.RebuildBlockIndex`（`biz/knowledge/block_pipeline.go`）统一 parse→resolve→persist→explicit 投影四步；vault 同步（`vault_sync.go`）、移动入链修复（`vault_write.go`）、粘贴文本摄取（`service/knowledge.go` IngestDocument）同走此入口；失败降级记日志不回滚主流程。旧 `link_parser.go`（正则 explicit 重建）已删除。Wire 经 `ProvideKnowledgeUsecase` 类型断言装配（`BlockIndexRepo` 与 `ResolveIndex` 同一 data repo 实现）。
> - **C-3 explicit 投影**：`projectExplicitLinks` 同 (src_doc, dst_doc) 块边聚合为一条文档边、`Weight=块边数`；dangling 与文档级自环不投影。`knowledge_links.weight` + `knowledge_documents.title/aliases` 落列（DDL 迁移 20261204，`EnsureKnowledgeSchema` fresh 形态同步）；`ReplaceLinks` 改 ON CONFLICT 刷新 weight/context；`UpdateDocLinkKeys` 物化 frontmatter 解析键。
> - **自引用存储契约**：Resolver 对自文档引用只产 `DstSelfOrdinal`，`ReplaceDocBlocks` 事务内按本次插入 ordinal→ID 映射回填 `dst_block_id`；ordinal 越界 → CodeBadRequest 整事务回滚（`TestKnowledgeBlocksReplace_SelfReference` 覆盖）。
> - **可见集合推导**：后台索引无「当前用户」，取文档所在 workspace 全部集合作为可见集（`visibleCollectionIDs`）。
>
> 验证：`go test ./internal/biz/knowledge/ -run TestResolve -count=1` 15/15 PASS；`go test ./internal/data/ -run TestKnowledgeBlocks -count=1` 6/6 PASS（含 SelfReference）；`vault_sync_test`/`vault_write_test` 经 memBlockIndex 内存端口覆盖写路径全链路；`go build ./...` + `go vet` + gofmt 全净。

### SP1-D：统一链接索引 LinkIndex + WS 增量

| # | 任务 | 涉及文件 |
|---|------|----------|
| D-1 | `LinkIndex` 进程内内存图：正/反邻接表 + 单调递增版本号；启动时从 `knowledge_block_refs` 全量构建（万级边毫秒级）；解析事务提交后 apply 增量（add/remove 边，版本号+1）；**单进程约束（N-1）**：多副本化需改事件广播，另立 ADR | `internal/biz/knowledge/link_index.go`（新增，TDD） |
| D-2 | `knowledge.graph.delta` WS 事件（`{added, removed, version}`，经 `event.Bus` 复用既有 WS 链路）；边带 scope（src/dst 块所属 collection backend + 租户推导），查询按当前用户可见 collection 集过滤（SP1 仅 scope 字段 + 基础过滤，完整片段级权限属 SP5）；**事件分级（N-2）**：按 AS-EVT-01 登记 Informational | `internal/event/`（事件定义）、`internal/biz/knowledge/link_index.go` |
| D-3 | 内存基准：10 万边 < 100MB（NFR-SP1-4），基准测试落档 | `link_index_bench_test.go`（新增） |

> **2026-08-08 完成（TDD，D-1/D-2）**：
> - **D-1 LinkIndex**：`internal/biz/knowledge/link_index.go`——五索引同增同删（bySrc/byDstBlk/incoming/bySrcDoc/danglingByColl），边单分配多索引共享指针；`ApplyDocDelta`（摘旧出边 → 入向块边转文档级镜像 FK SET NULL → 加新边，multiset 集合差收敛，空 delta 不递增版本）、`RemoveDoc`（出边级联清除、入边转 dangling 保 raw_target）、`LoadAll`（重置语义 version 归零）；可见性过滤按边源集合（B-1 防泄漏延伸到图谱读侧）。**时序契约**：ApplyDocDelta 不区分初次/重建，目标文档每次 apply 均转换入向块边（与 ReplaceDocBlocks 先删后插 FK 语义一致）。
> - **D-2 WS 增量**：`knowledge.graph.delta` SystemNoticeEvent（WS-only；AS-EVT-01 登记 Informational）——`GraphDeltaPublisher` biz 端口 + service 适配器（`internal/service/knowledge_graph_delta.go`，meta 携带 version + added/removed 边负载）；`NewKnowledgeService` 构造期创建 LinkIndex 并 `uc.SetLinkIndex` 接线（共享 uc，serve 前单线程）；启动全量加载 `Usecase.LoadLinkIndex`（LinkEdgeLoader 端口，data `ListAllRefEdges` JOIN 推导 SrcDocID/DstCollectionID）经 app.go readiness 门控后后台触发（`startup.knowledge_link_index`，失败仅降级不阻塞）；`DeleteDocument` 接 `RemoveDoc` 同步删除语义（集合删除的图同步归 SP1-G）。
> - **测试**：biz 12 个用例（增删/空 delta 不推/块级与文档级反链/dangling 聚合/目标重建转换/RemoveDoc/增量 vs 全量重放一致性/LoadAll 重置/并发 -race/启动加载四分支/删除同步含失败路径）+ service 发布器负载断言全绿。
>
> 验证：`go test ./internal/biz/knowledge/ -run 'TestLinkIndex|TestUsecase_LoadLinkIndex|TestUsecase_DeleteDocument' -count=1 -race` PASS；`go test ./internal/service/ -run 'Knowledge' -count=1` PASS；独立 GOCACHE 全新编译 `go build ./internal/... ./cmd/admin` + `go vet` 全净。（注：本次独立缓存重编发现上轮「全绿」为默认缓存幻影，两个测试期望与 FK 语义矛盾已修正。）

验收：增量 apply 后内存图与 DB 重放结果一致（一致性单测 ✅）；WS delta 事件到达前端（SP1-I 订阅接线后复验）；内存基准达标（✅ 见下）。

> **2026-08-08 完成（D-3 内存基准）**：`link_index_bench_test.go`——`TestLinkIndex_MemoryFootprint100K`（NFR-SP1-4 门控，常驻测试）：10 万仿真边（块 ID ~20 字符、raw_target ~24、context ~60，含 10% dangling + 10% embed）LoadAll 后 HeapAlloc 增量 **51.3 MB < 100 MB** ✅。基准（i9-12900K）：`LoadAll` 10 万边 **~49ms**（设计「万级边毫秒级」达标）；`ApplyDocDelta` 单文档 10 出边（图内已有 10 万边）**~112µs**。

### SP1-E：块级反链 API

| # | 任务 | 涉及文件 |
|---|------|----------|
| E-1 | Proto：`rpc ListBlockBacklinks`（GET `/v1/knowledge/blocks/{block_id}/backlinks`，支持 `doc_id` 参数聚合文档全部块反链）+ `rpc ListDanglingLinks`（GET `/v1/knowledge/collections/{id}/dangling-links`）+ `make api` | `api/kratos/knowledge/v1/knowledge.proto` |
| E-2 | biz/service 实现：块级反链读内存图（O（度数）），落库查询兜底；dangling 聚合（raw_target 分组 + 引用计数，「未创建笔记」视图语义） | `internal/biz/knowledge/`、`internal/service/knowledge.go` |

验收：反链精确到块级含上下文片段；文档反链 = 全部块反链聚合；dangling 出现在目标名反链语义中（验收 39）。

> **2026-08-08 完成（TDD，E-1/E-2）**：
> - **E-1 Proto**：`BlockBacklink`（src_block/src_doc/src_collection/src_doc_name/raw_target/edge_type/context/ambiguous）+ `DanglingLink`（raw_target/ref_count/refs）+ 双 RPC（`ListBlockBacklinks` 支持 block_id 路径与 doc_id 绑定双路由，doc_id 优先；`ListDanglingLinks` 按集合聚合）。
> - **E-2 biz 读路径**（`internal/biz/knowledge/backlink.go`）：读路由 `linkIndex.Loaded()` 已加载 → 直读内存图（O(度数)）；启动窗口未加载 → `BlockLinkReader` 落库兜底；双端口未接线 → 空降级。**loaded 门**（`LinkIndex.Loaded()`，LoadAll 置位）修复启动窗口读空图误判无反链。`DocNameReader` 批量解析源文档显示名（rel_path 优先、source 兜底），失败/缺失留空不阻塞主查询。输出确定性：反链按 (SrcDocID, SrcBlockID) 字典序；dangling 组间 ref_count 降序 + raw_target 字典序，组内同反链序。`ResolveBlockOwnerDoc` 支撑块级路径的 service 权限断言前置。
> - **E-2 data 兜底**：`knowledgeBlockRepo` 实现 `BlockLinkReader`（`ListBacklinksByBlock/ByDoc/ListDanglingEdges/GetBlockOwnerDoc`，复用 `refEdgeSelect` 公共 SELECT+JOIN）；主 repo 实现 `ListDocumentNames`（`COALESCE(NULLIF(rel_path,''), source)`，空入参短路）。
> - **E-2 service**：`internal/service/knowledge_backlink.go`——双 RPC + proto 映射；块级路径经 `ResolveBlockOwnerDoc → GetDocument → GetCollection → assertCollectionAccess` 权限链（与 ListDocumentLinks 同款 C-01 跨租户断言）；装配 `ProvideKnowledgeUsecase` 类型断言自动接线（BlockLinkReader ← blockIndex repo，DocNameReader ← 主 repo）。
> - **测试**：biz 7 用例（内存图/doc 聚合/DB 兜底/参数校验/名字降级/dangling 聚合排序/兜底+校验）+ data PG 集成 3 用例（三查询/空结果/名字解析）+ service 3 用例（块路径含 NotFound/BadRequest、doc 绑定、dangling 聚合+未知集合）全绿。
>
> 验证：`go test ./internal/biz/... ./internal/data/ ./internal/service/ ./internal/knowledge/... -count=1` 全绿（service 包 2 个 models.dev 网络依赖失败为已登记环境受限项，与本改动无关）；`go vet` + `gofmt` 全净。

### SP1-F：团队库后端（vault_backend 维度）

| # | 任务 | 涉及文件 |
|---|------|----------|
| F-1 | `knowledge_collections` 加 `vault_backend TEXT NOT NULL DEFAULT 'local'`（DDL 迁移 + ~~Ent Schema~~（B-1 裁决：knowledge 域纯 Raw SQL，无 Ent，实际为 `EnsureKnowledgeSchema` fresh 形态））；约束调整：backend=local 时 root_path 必填唯一，team 时为空 | `internal/data/knowledge.go`、`sql/migrations/20261205_knowledge_vault_backend.sql`（新增）、`ddl_migration_registry.go` |
| F-2 | team 库文档本体复用 PG `knowledge_documents.content_text` 路径；同管线解析接线（content_text → blockparse → 同套块/边表）；team 库无 SyncEngine（PG 即真相源）；**写路径契约（B-2）**：SP1 单写者语义 + activities 审计，并发冲突协议（版本/etag）后置 SP2；Proto `KnowledgeCollection` + `vault_backend` 字段 | `internal/biz/knowledge/`、`internal/service/knowledge.go`、`knowledge.proto` |

验收：两种库走同一条解析管线产出同构索引（US-26-2）；图谱/检索对个人边与团队边统一查询按可见性过滤（US-26-3）。

> **2026-08-08 完成（TDD，F-1/F-2）**：
> - **F-1 DDL**：迁移 `20261205_knowledge_vault_backend.sql`（幂等 ALTER，存量行默认 `local` 与历史语义一致无需回填）+ `EnsureKnowledgeSchema` fresh 形态补列 + registry 登记（Version 20261205）。root_path 部分唯一索引（`WHERE root_path <> ''`）对 team 行天然不生效，无需调整。
> - **F-1 Proto/模型**：`KnowledgeCollection.vault_backend=15` + `CreateCollectionRequest.vault_backend=5`（root_path 由 REQUIRED 注解改条件必填：local 必填 / team 必须为空）；biz `Collection.VaultBackend` + `VaultBackendLocal/Team` 常量；data 层 INSERT/Get/List/scanCollection 全链读写。
> - **F-2 约束（biz）**：`CreateVault` 按 backend 分支——local（缺省归一）走 `NormalizeRootPath`；team 禁 root_path（`ErrTeamRootPathForbidden`）且跳过路径规范化；未知值 `ErrInvalidVaultBackend`。
> - **F-2 接线（service）**：`CreateCollection` root_path 必填仅对 local 生效；team 创建后不启动同步循环（`StartVault` 门控）；`toProtoCollection` 映射新字段。
> - **F-2 同管线确认**：team 库写路径 = 既有 IngestDocument（粘贴文本/agent write），SP1-C 已在 `knowledge.go` ingest 尾部对任意 collection 调 `RebuildBlockIndex`（content_text → blockparse → 同套 blocks/refs 表），backend 无关无需新接线；`VaultSyncSupervisor.StartAll` 显式跳过 team（纵深防御，root_path 空本已排除）；图谱/检索可见性沿用 C-01 workspace 过滤，team 边与个人边同路径查询（US-26-3）。
> - **测试**：biz 5 用例（缺省归一 local/team 成功/team 禁 root_path/local 显式空路径报错/未知 backend）+ service 4 用例（team 成功且不启动同步/team 禁 root_path/local 启动同步/未知 backend）全绿。
>
> 验证：独立 GOCACHE 全新 `go build ./internal/... ./cmd/admin` ✅；`go test ./internal/biz/knowledge/ ./internal/service(-run Knowledge) ./internal/data(-run Knowledge,PG 集成) ./internal/knowledge/... -count=1` 全绿（service 包 2 个 models.dev 网络依赖失败为已登记环境受限项；chat durable-resume panic 属并行会话 self_improvement WIP，与本改动无关）；`go vet` 全净。

### SP1-G：晋升 + 删除同步

| # | 任务 | 涉及文件 |
|---|------|----------|
| G-1 | Proto：`rpc PromoteBlocks`（POST `/v1/knowledge/blocks/promote`，body `{block_ids[], target_collection_id}`；返回 created_blocks/cascade_candidates/lineage）+ `make api` | `knowledge.proto` |
| G-2 | `PromoteBlocks` usecase（`Data.ExecInTx`）：校验目标 backend=team + 写权限；团队库克隆（目标文档按 rel_path 同名查找或新建 + 新块 ID + `promoted_from`；源块回写 `promoted_to`）；**目标文档派生索引同事务重放（B-3）**：chunk→embed（有语义层时）→FTS，晋升即可检索；引用私有块的返回 cascade_candidates，未一并晋升的团队侧落 raw_target + dangling + `meta.private_external=true`；activities 表审计（对齐 knowledge_write R-6 惯例） | `internal/biz/knowledge/promote.go`（新增，TDD） |
| G-3 | 删除同步：team 块删除 → dst FK SET NULL 转 dangling；src 块删除 → 边显式同事务 DELETE（区分「边消失」与「转 dangling」）；WS 增量携带两类变更 | `internal/data/knowledge_blocks.go`、`link_index.go` |

验收：晋升产生谱系对 + 审计（验收 41）；team 块删除后边级联清除、WS 增量到达、私有侧 dangling 化（验收 42）。

> **2026-08-08 完成（TDD，G-1/G-2/G-3）**：
> - **G-1 Proto**：`PromoteBlocks`（POST `/v1/knowledge/blocks/promote`）——req `{block_ids[], target_collection_id}`；resp `created_blocks[]`（谱系对 src_block_id/new_block_id/new_doc_id）+ `cascade_candidates[]`（引用私有块的级联提示）+ 目标文档统计。
> - **G-2 晋升**（`biz/knowledge/promote.go` + `service/knowledge_promote.go`）：权限链——目标库 `assertCollectionMutateAccess` + 源块逐库 `assertPromoteSourceAccess` + biz 侧 backend=team 校验（`ErrPromoteTargetNotTeam`）；块克隆按源文档分单元 `promoteDocBlocks`（提取块全文 → 目标文档 find-or-create → 尾部追加 → 块级索引重放 → 尾部 N 块按序对应回写谱系）；谱系对经 `PromoteLineageWriter.WritePromoteLineage` 单事务回写（新块 `promoted_from` / 源块 `promoted_to`）；目标文档 chunk/FTS 重放走 service 层 `replayPromotedDocChunks`（晋升完成即可检索；单文档失败降级 status=error 不回滚，最终一致）。**对 S7 的四点 as-built 偏差**（设计文档已同步注记）：① 块克隆经「全文追加 + 目标文档重解析」而非直接 INSERT 块行（blocks 是派生索引，直插行会在下次重放丢失）；② 无单一大事务，逐目标文档原子（embed 走外部 API 不可入库事务）；③ `meta.private_external` 列不建——引用私有块的目标经可见性过滤自然落 dangling（raw_target 保留即占位语义）；④ 审计走 `knowledgeFlow` 流程日志 K1/K2（start/error/done 带 target_collection_id/block_count/created_blocks/replay 统计），非 activities 表。
> - **G-3 删除同步**：两类变更三层覆盖齐备——DB 侧 FK（src 块删 → `src_block_id ON DELETE CASCADE` 边消失；dst 块/文档/集合删 → `dst_* ON DELETE SET NULL` 转 dangling 保 raw_target；「显式同事务 DELETE」由 FK 同事务级联等价实现）；内存图侧 `RemoveDoc`（SP1-D）+ 新增 `RemoveCollection`（补齐 SP1-D 遗留的集合删除图同步：源边消失 + 外部入边转 dangling，镜像 FK 语义）；WS 侧 `DeleteDocument`/`DeleteCollection` 均发布非空 delta（removed 携带两类原形态、added 携带 dangling 新形态，前端按摘除/灰显分别处理）。
> - **测试**：biz——promote 4 用例（新建目标文档谱系 / 追加既有文档 / cascade 候选 / 校验矩阵）+ `TestLinkIndex_RemoveCollection`（库内边/跨库出边消失、外部入边转 dangling、不相关边不动、delta 两类齐备）+ `EmptyNoVersion`（空 delta 不递增版本）+ `TestUsecase_DeleteCollection_LinkIndex`（接线 + 失败路径不动图）；data PG 集成——`TestKnowledgeBlocks_CollectionDeleteCascade`（集合删除：本集合块/refs 消失、外部集合入边 dst 全 NULL 保 raw_target）；service——promote 4 用例（happy path 谱系+cascade / 追加既有文档 chunk 重放 / 跨租户权限矩阵 / 参数校验）。流程日志 step `knowledge.block.promote` 已双登记（`flow_log.go` stepTitleRegistry + 52-flow-logger.design.md §5.1）。
>
> 验证：独立 GOCACHE 全新 `go build ./internal/... ./cmd/admin` ✅；`go test ./internal/biz/knowledge/ ./internal/knowledge/... -count=1` ✅；`go test ./internal/data/ -run Knowledge`（PG 集成 7/7 含新级联用例）✅；`go test ./internal/service/ -run Knowledge` ✅；`go vet` + gofmt 全净。

### SP1-H：RebuildIndex + 惰性锚点回填

| # | 任务 | 涉及文件 |
|---|------|----------|
| H-1 | Proto `RebuildKnowledgeIndex`（POST `/v1/knowledge/collections/{id}/rebuild-index`）；流式重建：按文档分批（每批一事务，删块→重解析→重插），`sync_state` 加 `rebuilding` 态（期间检索走旧 chunks/FTS 降级可用）；幂等（中断重跑继续）；进度事件复用 EP-KN-02 模式 | `knowledge.proto`、`internal/biz/knowledge/`（新增 rebuild usecase） |
| H-2 | 惰性锚点回填：写路径发现引用指向未锚块时，经 VaultFiler（local）/团队内容写路径（team）向源文本行尾追加 ` ^<uuid7>`；幂等（已有锚点跳过）；同一文件多次解析锚点稳定不漂移 | `internal/biz/knowledge/vault_filer.go`（扩展）、`internal/biz/knowledge/`（team 写路径） |

验收：RebuildIndex 幂等可重入、重建期检索降级可用、重建后 refs 无孤儿边（验收 43）；锚点回填幂等稳定（验收 44）。

> **2026-08-09 完成（TDD，H-1/H-2）**：
> - **H-1 RebuildIndex**（`biz/knowledge/rebuild_index.go` + `service/knowledge_rebuild.go` + Proto `RebuildKnowledgeIndex`）：`RebuildCollectionBlockIndex` 流式逐文档重建——可见集合集整批提升一次（`visible` 预解析避免逐文档现查）→ 逐文档调 `rebuildBlockIndex(allowBackfill=false)`（重建是索引修复不改源文本）；幂等可重入（删块→重解析→重插整文档重放，中断重跑继续）；`sync_state` 置 `rebuilding` 期间检索走旧 chunks/FTS 降级可用，完成后恢复 `active`；service 层异步任务执行 + 进度经 WS 事件推送（复用 EP-KN-02 模式）+ `sync.Map` 防同库并发重建冲突（409）。
> - **H-2 惰性锚点回填**（`blockparse/anchor_backfill.go` + `biz/knowledge/anchor_backfill.go`）：三段式——① `AppendHeadingAnchor` 纯函数（goldmark AST 定位首个命中 heading 块，行尾 ATX 闭合符前插 ` ^<uuid7>`；已锚/未命中/空路径幂等跳过；frontmatter 原样保留；变更时统一 LF 换行）；② Resolver 检出——`ResolveIndex.FindBlockByHeadingPath` 增返回 `anchored` 标记（SQL `(anchor IS NOT NULL AND anchor <> '')`），`ResolveRefs` 收集未锚命中产 `AnchorBackfillRequest`（按 (doc,path) 去重；显式锚引用/已锚块/自文档锚块均不产请求）；③ 执行侧 `backfillAnchors`——team 库走 PG `UpdateDocumentContent`（content_text 即真相源），local 库走 `VaultFiler` CAS 写文件 + 镜像正文/hash 同步（hash 同步后下轮轮询幂等短路，**不触发 chunks/embedding 重建**）；回填自触发目标重索引 `allowBackfill=false`（**一跳即止不级联**）；best-effort——单请求失败仅记 Warn（K3），不回滚主流程、不重试（目标文档下次写路径自愈；源文档指向旧未锚块 ID 的边经 FK SET NULL 转块级 dangling，源文档下次写路径重解析愈合，最终一致 SP1-ADR-3）。
> - **对 S9 的 as-built 偏差**：回填锚点 ID 直接复用目标块 ID（`^<uuid7>`），不另建锚→块映射——库内唯一由 `knowledge_blocks` 部分唯一索引保证，与设计「块 ID 即锚文本」语义一致。
> - **测试**：blockparse 8 用例（基本回填/幂等/重复标题取首/ATX 闭合符/Setext/frontmatter 保留/未命中/空 heading）；biz——backfill 4 用例（team 落锚+幂等/不级联/best-effort 失败不阻塞/local CAS+镜像同步）+ Resolver 回填请求 5 用例（远端未锚产请求/已锚跳过/显式锚跳过/自文档未锚/(doc,path) 去重）+ `TestRebuildCollectionBlockIndex_NoBackfill`（全量重建不改源文本）；端到端 `TestVaultSync_AnchorBackfill_LocalEndToEnd`（A 引用 B 未锚标题 → B 文件落锚 + 镜像 hash 同步 + chunks 不重建 + 块索引锚定 + A 重解析边愈合到锚块 ID + 幂等稳定锚点不漂移）。
>
> **2026-08-09 事故根修（H-1 补丁）**：`RebuildCollectionBlockIndex` 误用 `ListDocuments` 摘要投影（SELECT 不含 `content_text`）当正文源——重建解析恒 0 块 0 边，`ReplaceDocBlocks` 删旧插空，**全库块索引静默清空**（UX 验证时发现反链全丢）。修复：分页仅取 ID，逐文档 `GetDocument` 回源完整正文再重建；`rebuildFixture` 改为「list 投影无正文 + docGetFn 回源」防 mock 假绿，新增 `TestRebuildCollectionBlockIndex_LoadsContentViaGetDocument` 回归。
>
> 验证：独立 GOCACHE `go build ./cmd/... ./internal/... ./pkg/...` ✅；`go test ./internal/biz/knowledge/... ./internal/knowledge/... -count=1` ✅ 全绿；`go test ./internal/service/ -run "TestKnowledge|TestPromote|TestRebuild"` ✅；`go vet` 全净。

### SP1-I：前端

| # | 任务 | 涉及文件 |
|---|------|----------|
| I-1 | 文档详情关联区新增「反向链接」分组（与既有显式/实体/语义分区并列），块级粒度 + 上下文片段 | `web/src/components/knowledge/KnowledgeDocDetail.vue`（改造）、`api.ts`、store |
| I-2 | dangling 灰显 + hover 提示「目标未创建」；库选择器 team 库「团队」徽标 | 浏览视图组件、库选择器组件、locales |
| I-3 | 晋升 UI：文档/块操作菜单「晋升到团队库…」→ 目标库选择 + 级联提示清单 → 确认执行 → 结果反馈（新建块数/谱系链接） | 晋升对话框组件（新增）、`api.ts`、store |
| I-4 | `knowledge.graph.delta` WS 订阅接线：图谱/反链视图增量更新（摘除边 / 节点灰显两类变更分别处理） | `web/src/features/knowledge/`（新增 composable）、图谱/反链消费方 |

验收：对照验收 38~44 全链路浏览器复验；i18n 键全入 locale（check-i18n 红线）。

> **2026-08-09 完成（TDD）**：
> - **I-1 块级反链分组**：`KnowledgeDocDetail.vue` 关联区下方新增「反向链接」分组（来源文档 + 上下文片段 + embed 徽标，点击导航来源文档）；`api.ts listBlockBacklinks` + store `backlinksByDoc` 缓存 + explorer `reloadDetail` 并行加载。
> - **I-2 dangling 灰显 + team 徽标**：`knowledgeUi.splitDanglingPreview` 纯函数（wikilink 分段，别名取 `|` 前、`#heading`/`#^anchor` 保留口径与 blockparse 一致），浏览视图命中 dangling 目标灰显 + 虚线 underline + hover 提示；`KnowledgeVaultTree.vue` / `KnowledgeGraph3D.vue` 库选择器 team 库「团队」徽标（`VaultQTreeNode.vaultBackend` 维度）。
> - **I-3 晋升 UI**：`KnowledgePromoteDialog.vue`（新增）两阶段——目标团队库选择（`promoteTargetOptions` 纯函数过滤 team 库且排除源库）→ 结果反馈（新建块数 + 级联提示清单：未一并晋升的私有引用 raw_target 列表 + 悬空复活提示）；`KnowledgeDocDetail` 操作行晋升按钮（`promotable`：非 team 库 + 已选中文件）；`api.ts promoteDocuments`（doc_ids 文档级入口，SP1-G 后端 `PromoteDocuments`）+ store `promoteDocs`（目标库树缓存失效）。
> - **I-4 graph.delta WS 订阅**：`graphDelta.ts`（新增，`parseGraphDeltaMeta`/`graphDeltaAffected` 纯函数，snake_case Meta 与 service `knowledge_graph_delta.go` 对齐）+ `useKnowledgeGraphDeltaWs.ts`（新增，SystemNotice 通道过滤 `knowledge.graph.delta`，页面级单订阅）；`useKnowledgePage.applyGraphDelta`——失效受影响文档反链/关联缓存 + 集合悬空链缓存（store `invalidateLinkCaches`），当前详情受影响时 `reloadDetail`（灰显/反链刷新）；当前图谱集合受影响时整图 `loadGraph`（文档级投影，增量边无法直接映射，重载即「摘除边消失 / dangling 节点灰显」两类变更的统一呈现）。
> - **修 I-1/I-2 遗留断线**：`KnowledgePage.vue` 补 `:dangling-targets` 透传（I-2 灰显此前未生效）；`useVaultExplorer.ux.spec.ts` mock 补 `loadDanglingLinks`（缺失致 3 个 UX 测试失败）；store 补 `moveDocumentToDir` 导入（此前遗漏，build 会断）。
> - **测试**：`knowledgeUi.spec.ts` +12 用例（splitDanglingPreview 4 + promoteTargetOptions 4，TDD RED→GREEN）；`graphDelta.spec.ts` 7 用例（Meta 解析/受影响面提取）。
>
> 验证：`npx vitest run src/features/knowledge src/stores` 382 全绿；`npx eslint --fix`（全部改动文件）零告警；i18n 键 zh-CN/en-US 双侧同步（promote* 13 键 + danglingTargetHint/vaultTeamBadge）。
>
> **as-built 偏差**：晋升 UI 仅文档级入口（doc_ids）——浏览视图无块选择能力，块级晋升（block_ids）留待 SP2 编辑器；图谱增量为整图重载而非逐边应用（文档级投影粒度，SP3 图谱 2.0 再评估逐边增量）。

### 依赖关系

```
SP1-A（解析）──→ SP1-B（物化）──→ SP1-C（Resolver/接线）──→ SP1-D（LinkIndex/WS）──→ SP1-E（反链 API）──→ SP1-I（前端）
                                    │                          ↑
SP1-F（team 后端，依赖 B/C）──────────┘                          │
SP1-G（晋升/删除，依赖 D/F）─────────────────────────────────────┘
SP1-H（重建/回填，依赖 B/C，可与 D~G 并行）
```

> **实施红线**：① TDD 铁律——blockparse/Resolver/LinkIndex/PromoteBlocks 全部先写失败测试；② refs 一律整文档重插不做 diff（一致性优先）；③ dangling 与边消失必须区分（SET NULL vs DELETE）；④ SiYuan/Logseq 仅借鉴思路逐行自研，禁止抄代码（License 纪律）；⑤ SP1 实施完成后同步 `65-module-cross-reference-full.md` knowledge 模块卡片（新增 blocks/refs 表、LinkIndex、blockparse 包）；⑥ 新增流程日志 step 须登记 `internal/event/flow_log.go` stepTitleRegistry + 同步 52-flow-logger.design.md §5.1。

### SP2~SP7 路线预告（G1~G8，2026-08-08 评审合入）

> 前瞻提案的产品化映射，学术依据见[评审报告 §五](../reports/2026-08-08-review-sp1-knowledge-blueprint.md)；各 SP 均须各自走 需求→设计→TDD 流程立项，本表仅为路线图锚点。

| # | 子项目 | 关键任务域 | 依赖 | 状态 |
|---|--------|-----------|------|------|
| SP2 | 编辑器与笔记体验 | CodeMirror 6 Live Preview + 自研 wikilink 扩展；反链面板；深空液态玻璃工作台（2026-08-10 用户裁决定稿） | SP1 | **已立项（2026-08-10）→ 见下文 §子模块：编辑器与笔记体验（SP2）Phase 计划** |
| SP3 | 图谱 2.0 | sigma.js 2D + three.js 3D（复用 G5-C v2 GPU 纹理管线）；Worker FA2；Obsidian 四区控制台；局部图谱；**时间轴回放**（消费 G4 valid_from/to） | SP1 | 待立项 |
| SP4 | 多模态摄取管线 | 文本代理路线（Whisper ASR + 关键帧 OCR + 场景描述，时间戳回链原片）；Office（docconv/excelize/pdfium 思路）；模态分库 + 查询路由（G5） | SP1 | 待立项 |
| SP5 | 双模权限与成熟度 | 片段级权限（Collaborative Memory 模型）；成熟度状态机（草稿→共享→精炼→规范，AS-FSM-01 显式建模）；AI 去私人化 super note 综合（G3） | SP1 | 待立项 |
| SP6 | AI 知识伙伴 | 写入即自动链接/标签建议（Notemd 式批量限流）；会话决策点主动唤回（复用 BeforeModel 钩子）；摄入即综合（交叉引用 + 矛盾检测）（G6） | SP1 | 待立项 |
| **SP7（新）** | 知识-记忆同基底 + 写回飞轮 | 记忆 L3/L4 投影为 agent vault 块（G1）；会话/TeamRun → LLM 抽取 → 团队库（验证门 + provenance，G2）；专家定位聚合（G7）；知识健康度指标（G8） | SP1+SP5 | 待立项 |

---

## 子模块：编辑器与笔记体验（SP2）Phase 计划

> **状态**：🟡 实施中（2026-08-10 立项） | **需求**：US-24~US-36 / FR-SP2-1~17（[37-knowledge.md](./37-knowledge.md#子模块编辑器与笔记体验sp2-需求2026-08-10)） | **设计**：[37-knowledge.design.md §SP2](./37-knowledge.design.md#sp2-编辑器与笔记体验深空液态玻璃工作台2026-08-10)（SP2-1~SP2-18）
> **用户裁决（2026-08-10）**：Obsidian 级笔记能力；UI 推翻 Tab 管理后台；编辑器 CodeMirror 6 Live Preview；A1+A2 一轮交付；深空液态玻璃视觉全套。
> **范围纪律**：纯前端重构，**后端零改动**；全部数据复用既有 `features/knowledge/api.ts`。
> **增强轮（2026-08-11，SP2-10）**：用户反馈（配色脱节/液态玻璃不足/对标功能差距）驱动——P0 视觉同源（P0-1 令牌消费全局变量、P0-2 三层液态玻璃）→ P1 功能补齐（P1-3 全库搜索、P1-4 标签页管理）→ P2 体验增强（P2-5 wikilink 标题段、P2-6 命令面板 MRU/别名、P2-7 未链接提及）。P2-7 为 SP2 唯一后端新增（1 个只读 RPC，无 Schema 变更）。

### 代码锚点（目标态）

| 路径 | 说明 |
|------|------|
| `web/src/css/deep-space.sass` | 深空液态玻璃设计令牌（`.kb-workbench` 作用域隔离） |
| `web/src/features/knowledge/useKnowledgeWorkbench.ts` | 工作台状态机（tabs/激活/脏标记/CAS 保存） |
| `web/src/features/knowledge/{wikilink,outline,frontmatter,commands}.ts` | 纯函数层（可单测） |
| `web/src/components/knowledge/effects/*` | GlassPanel / ParticleField / TiltCard / GlowButton / RingCarousel |
| `web/src/components/knowledge/workbench/*` | KnowledgeWorkbench / TopBar / Tabs / NoteEditor / QuickSwitcher / CommandPalette / SearchPanel（P1-3） |
| `web/src/components/knowledge/panels/*` | PanelBacklinks（含 P2-7 未链接提及分组）/ Outlinks / Outline / Properties / LocalGraph |
| `web/src/pages/KnowledgePage.vue` | 重写为薄壳 |
| `internal/biz/knowledge/mention.go` + `internal/data/knowledge_mentions.go` + `internal/service/knowledge_mention.go` | P2-7 未链接提及（端口 + ILIKE 预筛实现 + RPC 装配） |

### Phase 划分

| Phase | 内容 | 关联契约 | 状态 |
|-------|------|---------|------|
| SP2-1 | 视觉令牌 + 5 特效组件 | 设计 §SP2-2/§SP2-3；FR-SP2-9/10 | ✅（2026-08-10：deep-space.sass + Glass/Particle/Tilt/Glow/Ring + useReducedMotion；9 测试绿 + eslint/stylelint 干净） |
| SP2-2 | `useKnowledgeWorkbench` 状态机（TDD：tabs 开闭/去重激活/脏标记/CAS 冲突/删除联动） | 设计 §SP2-4；FR-SP2-2 | ✅（2026-08-10） |
| SP2-3 | Workbench 骨架 + TopBar + 三栏装配（树复用换肤、空态占位） | 设计 §SP2-1；FR-SP2-1 | ✅（2026-08-10：workbench.spec smoke 5 测试绿） |
| SP2-4 | CM6 编辑器：高亮 + 深空主题 + 保存接线 → Live Preview 行级装饰 → `[[` 补全 + 芯片 + 跳链 | 设计 §SP2-5/§SP2-6；FR-SP2-3/4；验收 31/32 | ✅（2026-08-10：NoteEditor.vue + wikilink.ts + note-editor.spec/wikilink.spec） |
| SP2-5 | 右栏五面板（反链/出链/大纲/属性/局部图谱）联动 | 设计 §SP2-8；FR-SP2-7；验收 30/35 | ✅（2026-08-10：panels/* + WorkbenchSidePanels + outline/frontmatter/localGraphLayout 纯函数层） |
| SP2-6 | ⌘O 快速切换 + ⌘K 命令面板 + 全局快捷键 | 设计 §SP2-7；FR-SP2-5/6；验收 33 | ✅（2026-08-10：QuickSwitcher/CommandPalette/PaletteModal + commands.ts + commands.spec 10 命令） |
| SP2-7 | 新建笔记/文件夹双入口 + 空态 RingCarousel | 设计 §SP2-3/§SP2-7；FR-SP2-8；验收 34 | ✅（2026-08-10：Sidebar 头部双按钮 + 命名弹窗落盘） |
| SP2-8 | KnowledgePage 重写接入 + 图谱全屏覆盖 + 设置浮层 + 旧组件退役清理 | 设计 §SP2-9/§SP2-11；验收 36 | ✅（2026-08-11：KnowledgePage 薄壳重写 + 图谱全屏 overlay + kb-portal 设置浮层 + 文件行操作菜单/拖拽移动/下载；旧组件 KnowledgeDocumentsPanel/DocList/DocDetail/SearchDual/DocPreviewDialog 已删除，knowledgeUi 死导出与 i18n 死 key 已清理；知识域 267 测试绿 + eslint 0 err） |
| SP2-9 | i18n + `pnpm lint/test/build` 全绿 + 浏览器运行时复验 + review | 验收 37/38 | 🟡（2026-08-11：i18n 双语已补 + lint/test 绿；待 build + 浏览器复验 + review） |
| SP2-10 | 增强轮：P0 视觉同源（P0-1 令牌消费全局变量 / P0-2 三层液态玻璃）→ P1 功能（P1-3 全库搜索 SearchPanel / P1-4 标签页拖拽重排+中键关闭）→ P2 增强（P2-5 wikilink 标题段补全+定位 / P2-6 命令面板 MRU+别名 / P2-7 未链接提及后端 RPC+面板分组） | 设计 §SP2-2/7/8/12~17；FR-SP2-11~17；US-31~36；验收 39~44 | ✅（2026-08-11：全部落地；mention biz/data/service + 前端 6 文件；wikilink/commands/workbench/api 单测绿） |

### 实施红线

1. **TDD**：状态机与纯函数层（wikilink/outline/frontmatter/命令过滤）先写失败测试再实现；组件级以 smoke 测试兜底
2. **视觉令牌作用域隔离**：深空样式一律限定 `.kb-workbench` 根类名下，禁止污染全局 Quasar 明暗主题（SP2-ADR-6）
3. **降级契约**：`prefers-reduced-motion` 全动效关闭 + 粒子按设备分级（FR-SP2-10），每个特效组件自带降级分支
4. **CAS 语义不变**：保存复用 `updateDocumentContent` 既有冲突行为（留双份 + 警告），禁止引入自动保存（SP2-ADR-3）
5. **退役组件处理**：~~KnowledgeDocumentsPanel/DocList/DocDetail/SearchDual 在 SP2-8 验收前保留文件，验收后删除并全局 grep 清理引用（R4）~~ ✅ 已完成（2026-08-11：含 KnowledgeDocPreviewDialog 一并删除，knowledgeUi 死导出/i18n 死 key 同步清理）
6. **小步快跑**：每 Phase 独立可验证（lint+test 绿），一次只改一个明确问题（R5）

### 验收标准（SP2 映射）

对应需求文档验收 30~38：三栏联动与标签页（30）、Live Preview + CAS（31）、wikilink 写作（32）、⌘O/⌘K 键盘全可操作（33）、新建双入口（34）、局部图谱联动（35）、图谱全屏 + 设置浮层（36）、视觉全套 + reduced-motion（37）、lint/test/build 全绿 + 运行时复验（38）。
