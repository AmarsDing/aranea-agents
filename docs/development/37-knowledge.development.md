# Knowledge 知识库 — 开发计划

> **版本**：2026-07-21 | **状态**：✅ Phase 1-9 已完成（Phase 9 多模态入库：图片经 VisionExtractor 异步提取为 MD；真实视觉模型端到端待环境就位后复验）；✅ Phase 11（US-14 免选择知识库）已完成；Phase 10（GraphRAG 旁路）可选
> **2026-07-25 新增**：§子模块 Vault 重设计 Phase 计划（P1~P6，含 P4a/P4b 拆分）——Vault 重设计经评审有条件通过，R-1~R-6 已合入设计文档 §V6，待启动实施。
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
| **P1** | Vault 基础 + 单向同步（文件→索引） | S-1 | 📋 |
| **P2** | 双向同步 + frontmatter 摘要卡 + 双轨关联 | R-1、R-2、R-3 | 📋 |
| **P3** | 资源管理器 UI（树/列表/详情/双区搜索） | R-3（来源标注） | 📋 |
| **P4a** | L0 强 BM25 栈（含 R-5 选型 spike） | R-5、S-5 | 📋 |
| **P4b** | L2 语义层插件化（含 R-4 契约变更） | R-4、S-3 | 📋 |
| **P5** | Agent 工具族 navigate/grep/write（含 R-6 安全契约） | R-6 | 📋 |
| **P6** | 迁移 Collection → Vault | S-2 | 📋 |

### P1：Vault 基础 + 单向同步

| # | 任务 |
|---|------|
| P1-1 | `knowledge_collections` 升级 Vault：`root_path`（唯一约束 + 规范化：resolve symlink/绝对路径/尾部斜杠归一，禁挂系统根目录 S-1）、`sync_state`、`sync_config`；DDL 迁移注册 — ✅ 已完成（2026-07-25，Schema 列+唯一索引已落；`internal/biz/knowledge/vault_usecase.go`：NormalizeRootPath + CreateVault，9 测试全绿） |
| P1-2 | VaultFiler：KB 侧唯一写文件出口（路径 sanitize 基础版）— ✅ 已完成（2026-07-25，`internal/biz/knowledge/vault_filer.go`：sanitize/原子写/覆盖备份/回收站，8 测试全绿） |
| P1-3 | SyncEngine 单向扫描：文件变更 → chunk → （可选 embed）→ 派生索引；轮询实现，fsnotify 留接口 — ✅ 已完成（2026-07-25，`internal/biz/knowledge/sync_engine.go` + `internal/knowledge/vault_sync.go`/`vault_sync_runner.go`：Scan/Diff/Apply/Run 三段解耦；prev 重启自 DB 重建；幂等短路 + 无语义层降级；Watcher 接口预留，P2 接 fsnotify；13 测试全绿） |
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
| P3-1 | KnowledgePage 重构为三栏：Vault 切换 + 文件夹树（懒加载）+ 文档列表 + 详情面板（hover 卡两级密度） |
| P3-2 | 统一搜索框双区：即时区（纯前端 fzf 式内存索引 <10k 文档）+ 语义区（走后端检索） |
| P3-3 | 关联区：双链/实体/语义三类关联展示并标注来源类型（R-3）；搜索意图分流规则与后端路由规则共享定义 |

验收：三栏可用；搜索双区意图分流正确；关联区来源标注完整。

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
| P4b-1 | **R-4 契约变更四条**：CreateVault EmbeddingModel 改可选（空=无语义层）；摄取流水线对无 embedding Vault 跳过向量写入；knowledge_search 无语义层自动降级 L0+L1；前端 embedding 配置改「可选增强」 |
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
