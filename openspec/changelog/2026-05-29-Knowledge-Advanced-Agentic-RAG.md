# Knowledge — Advanced RAG + Agentic RAG 实现

> 日期：2026-05-29
> 模块：Knowledge 知识库
> 关联：[37-knowledge.md](../需求/37-knowledge.md) · [37-knowledge-evolution-roadmap.md](../需求/37-knowledge-evolution-roadmap.md)

---

## 变更概要

Knowledge 知识库从 Naive RAG 升级至 Advanced RAG + 部分 Agentic RAG，实现查询重写、混合检索、自适应路由、检索质量评估、联邦搜索和 Agent 自校验工具。

---

## Phase 5：Advanced RAG — ✅ 已完成

### 新增组件

| 组件 | 文件 | 说明 |
|------|------|------|
| QueryRewriter | `internal/knowledge/query_rewriter.go` | 查询重写（HyDE/Decomposition/MultiQuery） |
| HybridRetriever | `internal/knowledge/hybrid_retriever.go` | 混合检索（Dense+Sparse+RRF 融合） |
| AdaptiveRouter | `internal/knowledge/adaptive_router.go` | 自适应检索路由（查询复杂度分类） |
| RetrievalEvaluator | `internal/knowledge/retrieval_evaluator.go` | 检索质量评估（CRAG 式自校验） |
| SearchHelpers | `internal/knowledge/search_helpers.go` | 检索评估辅助（ChunkSearcher/ChunkAssessor） |
| LLMResolver | `internal/knowledge/llm_resolver.go` | LLM 模型解析（Advanced RAG 共用） |
| Advanced Wire | `internal/service/knowledge_advanced.go` | 5 个 Wire Provider 工厂 |

### Proto 扩展

- `SearchRequest.rewrite_strategy`（field 10）：hyde | decomposition | multi_query
- `SearchRequest.hybrid_search`（field 11）：auto | dense | sparse | rrf

### Data 层扩展

- `SearchChunksBM25`：PostgreSQL ts_vector 全文检索 + GIN 索引
- `NewKnowledgeSparseSearcherFromData`：稀疏检索 Provider

### 前端扩展

- `KnowledgeSearchPanel.vue`：混合检索模式选择器、查询重写策略选择器、Rerank 开关
- `useKnowledgePage.ts`：新增 `searchHybridMode`、`searchRewriteStrategy`、`searchUseRerank` ref
- `api.ts`：`searchKnowledge` 传递 `rewrite_strategy`、`hybrid_search`、`use_rerank` 参数

### 设计决策

- 查询重写 LLM 不可用时自动降级（透传原始查询）
- 混合检索 RRF K=60，overfetch = topK×3（上限 50）
- 自适应路由基于启发式规则（词数/问号/连接词/Decomposition 标记/TopK）
- 检索评估超时 10s，失败时降级为 `Sufficient=true, Confidence=0.5`
- CRAG 补充检索结果通过 `MergeSearchResults` 去重合并

---

## Phase 6：Agentic RAG — ✅ 已完成

### 新增组件

| 组件 | 文件 | 说明 |
|------|------|------|
| FederatedRetriever | `internal/knowledge/federated_retriever.go` | 跨 Collection 联邦搜索（Broadcast + Route 策略） |
| knowledge_reflect | `internal/tools/knowledge/tool.go` | Agent 自校验检索质量工具 |
| Plan-Then-Retrieve | `internal/agent/knowledge_inject.go` | BeforeModel 钩子注入 Collection 摘要 |

### 联邦搜索 Route 策略

- `CollectionMetaFetcher` 接口：由 `biz.KnowledgeUsecase` 实现
- `FederationStrategy`：Broadcast（默认）/ Route（基于相关性评分）
- `FederatedSearchOptions`：策略 + RouteTopN + RouteMinScore
- `collectionRelevanceScore`：基于 Collection 名称/描述与查询词匹配度评分
- `routeCollections`：按评分排序取 TopN，路由失败自动降级 Broadcast
- `NewFederatedRetrieverWithMeta`：Wire 工厂，注入 KnowledgeUsecase 作为 meta fetcher

### Plan-Then-Retrieve

- BeforeModel 钩子（优先级 6），在每次模型调用前注入 Collection 摘要
- 仅注入 Agent 关联的 Collection（通过 `KnowledgeCollectionsFromContext` 读取 scoped IDs）
- 摘要内容：Collection 名称、ID、描述、文档数、块数 + 搜索策略提示
- 截断保护：单个描述 ≤120 字符，总摘要 ≤1500 字符，最多 10 个 Collection
- KnowledgeUsecase 为 nil 或无 Collection 时自动跳过

### 工具注册链

- `ToolKeyKnowledgeReflect = "knowledge_reflect"` 常量
- `tool_catalog_runtime.go`：KnowledgeReflect 加入 `sessionBoundToolKeys`
- `effective_config.go`：`cfg.KnowledgeReflect` 映射
- `tool_assembly.go`：`cfg.KnowledgeReflect = eff[biz.ToolKeyKnowledgeReflect]`
- `toolsets.go`：`knowledgepkg.NewReflectTool()` 装配
- `builtin_tools_seed.go`：knowledge_reflect 种子

### Context 注入链

- `chat_orchestrator.go`：RuntimeTooling 增加 FederatedRetriever/Evaluator
- `chat_orchestrator_turn.go`：Context 注入 FederatedRetriever/Evaluator
- `runner.go`：Team Runner 增加 SetKnowledgeFederatedRetriever/SetKnowledgeEvaluator
- `runner_team_trpc.go`：Team context 注入
- `cmd/admin/wire.go`：provideRuntimeTooling 参数

### knowledge_reflect 工具

输入：
```json
{ "collection_ids": ["abc123", "def456"], "query": "What is X?", "top_k": 5 }
```

输出：
```json
{ "sufficient": false, "confidence": 0.6, "supplement_query": "补充查询", "chunks": [...] }
```

### 设计决策

- FederatedRetriever 使用 `safego.Go` + `sync.WaitGroup` 并行搜索
- 单 Collection 自动降级为 AdaptiveRouter/Retriever
- 部分集合失败时返回成功集合结果，全部失败返回错误
- 评估失败时 FlowLog 警告，降级为 `sufficient=true, confidence=1.0`
- Collection 权限校验：`WithKnowledgeCollections` context 限定

### 待实现

- ~~Plan-Then-Retrieve（L4 prompt 注入 Collection 摘要）~~ ✅ 已实现
- ~~联邦搜索 Route 策略（智能路由到最相关 Collection）~~ ✅ 已实现
- AgenticFilter（LLM 动态生成过滤条件）
- OCR / Extractor（图片/PDF 自动提取文本）
- 多租户知识库隔离
- code_search 工具

---

## 测试覆盖

| 测试文件 | 说明 |
|----------|------|
| `query_rewriter_test.go` | 查询重写策略解析、HyDE/Decomposition/MultiQuery |
| `hybrid_retriever_test.go` | 混合检索模式选择、RRF 融合 |
| `adaptive_router_test.go` | 查询复杂度分类、模式选择 |
| `retrieval_evaluator_test.go` | 评估解析、JSON 容错、截断 |
| `federated_retriever_test.go` | 单/多 Collection 搜索、合并顺序、Route 策略、相关性评分 |

---

## 文档更新

| 文档 | 更新内容 |
|------|----------|
| `37-knowledge.md` | 新增 US-9/10/11 用户故事、搜索功能表、验收标准 11-18、架构图、组件表、Plan-Then-Retrieve |
| `37-knowledge.design.md` | 新增 §5.5-5.10 设计、§6.1b knowledge_reflect、§6.4 Context 注入链、§6.5 Plan-Then-Retrieve、Search 流程图、Wire 依赖链 |
| `37-knowledge-development.md` | Phase 5/6 任务表（全部 ✅）、验收清单、任务 #17-25 |
| `37-knowledge-evolution-roadmap.md` | Phase 2 ✅ 已实现、局限表更新、RAG 成熟度定位更新（Agentic RAG） |
