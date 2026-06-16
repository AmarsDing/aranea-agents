# 知识库（Knowledge）— 框架对齐分析

> 模块路径：`pkg/trpc-agent-go/knowledge/`
> 项目实现路径：`internal/knowledge/`、`internal/biz/knowledge/`、`internal/data/knowledge.go`、`internal/tools/knowledge/`、`internal/service/knowledge*.go`
> 当前对齐度：★☆☆☆☆

---

## 一、框架能力全景

### 1.1 核心接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `knowledge.Knowledge` | `Search(ctx, *SearchRequest) (*SearchResult, error)` | 知识库搜索主接口 |
| `knowledge.GraphKnowledge` | `Search` + `Traverse(ctx, *TraverseQuery) (*TraverseResult, error)` + `FindPaths(ctx, *PathQuery) (*PathResult, error)` | 图知识库扩展接口 |
| `source.Source` | `ReadDocuments(ctx) ([]*Document, error)` + `Name()` + `Type()` + `GetMetadata()` | 知识源接口 |
| `source.GraphSource` | `ReadGraph(ctx, ...opts) (*graph.Data, error)` | 图知识源接口 |
| `vectorstore.VectorStore` | `Add` / `Get` / `Update` / `Delete` / `Search` / `DeleteByFilter` / `UpdateByFilter` / `Count` / `GetMetadata` / `Close` | 向量存储接口 |
| `embedder.Embedder` | `GetEmbedding(ctx, text) ([]float64, error)` + `GetEmbeddingWithUsage(ctx, text) (..., map[string]any, error)` + `GetDimensions() int` | 嵌入器接口 |
| `retriever.Retriever` | `Retrieve(ctx, *Query) (*Result, error)` + `Close()` | 检索器接口 |
| `query.Enhancer` | `EnhanceQuery(ctx, *Request) (*Enhanced, error)` | 查询增强器接口 |
| `reranker.Reranker` | `Rerank(ctx, *Query, []*Result) ([]*Result, error)` | 重排序器接口 |
| `graphstore.Store` | `AddNodes` / `AddEdges` / `Traverse` / `FindPaths` / `Close` | 图存储接口 |
| `chunking.Strategy` | `Chunk(*Document) ([]*Document, error)` | 分块策略接口 |
| `extractor.Extractor` | `Extract` / `ExtractFromReader` / `SupportedFormats` / `Close` | 文档提取器接口 |

### 1.2 关键类型

| 类型 | 说明 |
|------|------|
| `BuiltinKnowledge` | 框架默认 Knowledge 实现，编排 Embedder→VectorStore→Reranker 管线 |
| `BuiltinGraphKnowledge` | 框架默认 GraphKnowledge 实现 |
| `document.Document` | 文档模型（ID/Name/Content/EmbeddingText/Metadata/CreatedAt/UpdatedAt） |
| `SearchRequest` | 搜索请求（Query/History/UserID/SessionID/MaxResults/MinScore/SearchFilter/SearchMode） |
| `SearchResult` | 搜索结果（Document/Score/Text/Documents） |
| `SearchFilter` | 搜索过滤（DocumentIDs/Metadata/FilterCondition） |
| `UniversalFilterCondition` | 通用递归过滤条件（Field/Operator/Value，支持 and/or 嵌套） |
| `graph.Node` / `graph.Edge` / `graph.Data` | 图数据模型 |
| `graph.TraverseQuery` / `graph.PathQuery` | 图遍历/路径查询 |

### 1.3 扩展点

| 扩展点 | 机制 | 适用场景 |
|--------|------|---------|
| 自定义 Embedder | 实现 `embedder.Embedder` 接口 | 新增嵌入模型提供商 |
| 自定义 VectorStore | 实现 `vectorstore.VectorStore` 接口 | 新增向量数据库后端 |
| 自定义 Reranker | 实现 `reranker.Reranker` 接口 | 新增重排序算法/服务 |
| 自定义 QueryEnhancer | 实现 `query.Enhancer` 接口 | 自定义查询重写逻辑 |
| 自定义 Retriever | 实现 `retriever.Retriever` 接口 | 自定义检索管线 |
| 自定义 Source | 实现 `source.Source` 接口 | 新增知识源类型 |
| 自定义 GraphSource | 实现 `source.GraphSource` 接口 | 新增图知识源 |
| 自定义 GraphStore | 实现 `graphstore.Store` 接口 | 新增图存储后端 |
| 自定义 Chunking | 实现 `chunking.Strategy` 接口 | 自定义分块策略 |
| 自定义 Extractor | 实现 `extractor.Extractor` 接口 | 自定义文档提取 |
| ResultPostProcessor | `func(ctx, *KnowledgeSearchResponse) *KnowledgeSearchResponse` | 搜索结果后处理钩子 |

### 1.4 配置选项

#### BuiltinKnowledge Option

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithVectorStore(vs)` | 设置向量存储 | 无（必填） |
| `WithEmbedder(e)` | 设置嵌入器 | 无（必填） |
| `WithEnableSourceSync(enable)` | 启用增量同步 | `false` |
| `WithQueryEnhancer(qe)` | 设置查询增强器 | `PassthroughEnhancer` |
| `WithReranker(r)` | 设置重排序器 | `topk.New()` |
| `WithRetriever(r)` | 设置自定义检索器 | `DefaultRetriever` |
| `WithSources(sources)` | 设置知识源列表 | `nil` |

#### Load Option

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithShowProgress(show)` | 显示进度日志 | `true` |
| `WithProgressStepSize(step)` | 进度步长 | `10` |
| `WithShowStats(show)` | 显示统计 | `true` |
| `WithSourceConcurrency(n)` | 源级并发数 | `min(4, len(sources))` |
| `WithDocConcurrency(n)` | 文档级并发数 | `runtime.NumCPU()` |
| `WithRecreate(recreate)` | 重建向量库 | `false` |
| `WithLoadProgressCallback(cb)` | 进度回调 | `nil` |

#### SearchTool Option

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithToolName(name)` | 工具名 | `"knowledge_search"` |
| `WithToolDescription(desc)` | 工具描述 | 默认描述 |
| `WithFilter(filter)` | 静态元数据过滤 | `nil` |
| `WithConditionedFilter(cond)` | 复杂过滤条件 | `nil` |
| `WithMaxResults(n)` | 最大返回数 | `10` |
| `WithMinScore(score)` | 最低分数阈值 | `0.0` |
| `WithExcludeMetadataKeys(keys...)` | 排除元数据键 | 无 |
| `WithResultPostProcessor(p)` | 结果后处理器 | `nil` |

### 1.5 框架内置实现

#### VectorStore 实现

| 实现 | 路径 | 说明 |
|------|------|------|
| InMemory | `vectorstore/inmemory` | 内存存储，开发/测试用 |
| PGVector | `vectorstore/pgvector` | PostgreSQL + pgvector |
| Elasticsearch | `vectorstore/elasticsearch` | Elasticsearch |
| Milvus | `vectorstore/milvus` | Milvus 向量数据库 |
| Qdrant | `vectorstore/qdrant` | Qdrant 向量数据库 |
| TcVector | `vectorstore/tcvector` | 腾讯云向量数据库 |
| SQLiteVec | `vectorstore/sqlitevec` | SQLite 向量扩展 |

#### Embedder 实现

| 实现 | 路径 | 说明 |
|------|------|------|
| OpenAI | `embedder/openai` | OpenAI 兼容接口 |
| Gemini | `embedder/gemini` | Google Gemini |
| Ollama | `embedder/ollama` | Ollama 本地模型 |
| HuggingFace | `embedder/huggingface` | HuggingFace |

#### Reranker 实现

| 实现 | 路径 | 说明 |
|------|------|------|
| TopK | `reranker/topk` | 简单截断（默认） |
| Cohere | `reranker/cohere` | Cohere SaaS |
| Infinity | `reranker/infinity` | Infinity/TEI 标准 API |

#### Query Enhancer 实现

| 实现 | 路径 | 说明 |
|------|------|------|
| PassthroughEnhancer | `query/passthrough` | 透传（默认） |
| LLMEnhancer | `query/llm` | LLM 查询重写 |

#### Source 实现

| 实现 | 路径 | 说明 |
|------|------|------|
| File | `source/file` | 文件源 |
| Dir | `source/dir` | 目录源 |
| URL | `source/url` | URL 源 |
| Auto | `source/auto` | 自动检测源类型 |
| Repo | `source/repo` | 代码仓库源（同时实现 Source 和 GraphSource） |

#### GraphStore 实现

| 实现 | 路径 | 说明 |
|------|------|------|
| AGE | `graphstore/age` | Apache AGE (PostgreSQL 图扩展) |

#### Chunking 实现

| 实现 | 路径 | 说明 |
|------|------|------|
| Fixed | `chunking/fixed` | 固定大小分块 |
| Recursive | `chunking/recursive` | 递归分块 |
| Markdown | `chunking/markdown` | Markdown 按标题分块 |
| JSON | `chunking/json` | JSON 分块 |

#### Document Reader 实现

| 实现 | 路径 | 支持格式 |
|------|------|---------|
| Text | `document/reader/text` | .txt |
| Markdown | `document/reader/markdown` | .md |
| JSON | `document/reader/json` | .json |
| CSV | `document/reader/csv` | .csv |
| PDF | `document/reader/pdf` | .pdf |
| Docx | `document/reader/docx` | .docx |
| Go | `document/reader/golang` | .go (AST) |
| Proto | `document/reader/proto` | .proto (AST) |

#### 搜索工具

| 工具 | 路径 | 说明 |
|------|------|------|
| `NewKnowledgeSearchTool` | `knowledge/tool/searchtool.go` | 基础语义搜索工具 |
| `NewAgenticFilterSearchTool` | `knowledge/tool/searchtool.go` | 智能过滤搜索工具（LLM 自动构建 filter） |
| `NewCodeSearchTool` | `knowledge/tool/codesearchtool.go` | 代码搜索工具（AST 元数据 + 去重） |
| `NewCodeGraphSearchTool` | `knowledge/tool/graphtool.go` | 代码图搜索工具集（search/traverse/find_paths） |
| `NewGraphToolSet` | `knowledge/tool/graphtool.go` | 通用图工具集 |

---

## 二、项目实现现状

### 2.1 框架接口实现情况

| 框架接口/功能 | 项目实现 | 合规性 | 说明 |
|--------------|---------|--------|------|
| `knowledge.Knowledge` 接口 | ❌ 未实现 | ❌ | 项目未使用框架 Knowledge 接口 |
| `BuiltinKnowledge` | ❌ 未使用 | ❌ | 完全自建 RAG 管线 |
| `vectorstore.VectorStore` | ❌ 未使用 | ❌ | 自建 pgvector Raw SQL 实现 |
| `embedder.Embedder` | ❌ 未使用 | ❌ | 自建 `MultiProviderEmbedder` |
| `retriever.Retriever` | ❌ 未使用 | ❌ | 自建 `internal/knowledge.Retriever` |
| `query.Enhancer` | ❌ 未使用 | ❌ | 自建 `QueryRewriter`（HyDE/Decomposition/MultiQuery） |
| `reranker.Reranker` | ✅ 使用 | ✅ | 通过 `NewRerankerFromEnv()` 使用框架 reranker 包 |
| `chunking.Strategy` | ⚠️ 部分使用 | ⚠️ | 高级策略（markdown/json/recursive）委托框架 chunking，基础策略（char/token）自建 |
| `document.Reader` | ✅ 使用 | ✅ | 通过 `reader.GetReader()` 使用框架文档解析器 |
| `source.Source` | ❌ 未使用 | ❌ | 文档通过 API 上传，不使用框架 Source |
| `knowledge.KnowledgeSearchTool` | ❌ 未使用 | ❌ | 自建 `knowledge_search` + `knowledge_reflect` 工具 |
| `knowledge.WithKnowledge()` | ❌ 未使用 | ❌ | Agent 不通过框架 WithKnowledge 集成 |

### 2.2 自建功能清单

| 自建功能 | 实现位置 | 替代框架功能 | 自建原因 |
|---------|---------|-------------|---------|
| Collection/Document/Chunk 数据模型 | `internal/biz/knowledge/knowledge.go` | 框架无对应概念 | 项目需要多集合管理和 CRUD API |
| pgvector 向量存储（Raw SQL） | `internal/data/knowledge.go` | `vectorstore/pgvector` | 需要与项目 Postgres 连接池集成，需自定义 Schema |
| BM25 稀疏搜索 | `internal/data/knowledge.go` | 框架无对应功能 | 框架 VectorStore 无 BM25 接口 |
| RRF 融合排序 | `internal/knowledge/hybrid_retriever.go` | 框架无对应功能 | 框架无混合检索支持 |
| `MultiProviderEmbedder` | `internal/knowledge/embedder.go` | `embedder/openai` 等 | 需要运行时切换提供商 + Admin UI 配置 |
| `Retriever`（embed→search→rerank） | `internal/knowledge/retriever.go` | `retriever/default` | 需要使用项目 biz 层 Repo 接口 |
| `HybridRetriever`（Dense/Sparse/RRF） | `internal/knowledge/hybrid_retriever.go` | 框架无对应功能 | 框架无混合检索 |
| `AdaptiveRouter`（查询复杂度→模式选择） | `internal/knowledge/adaptive_router.go` | 框架无对应功能 | 框架无自适应路由 |
| `QueryRewriter`（HyDE/Decomposition/MultiQuery） | `internal/knowledge/query_rewriter.go` | `query/llm` | 项目需要 3 种策略 + 中文优化 Prompt |
| `FederatedRetriever`（跨集合联邦检索） | `internal/knowledge/federated_retriever.go` | 框架无对应功能 | 框架无联邦检索 |
| `RetrievalEvaluator`（LLM 评估检索质量） | `internal/knowledge/retrieval_evaluator.go` | 框架无对应功能 | 框架无检索评估 |
| `knowledge_search` 工具 | `internal/tools/knowledge/tool.go` | `knowledge/tool.NewKnowledgeSearchTool` | 需要 context 注入依赖 + 集合权限控制 |
| `knowledge_reflect` 工具 | `internal/tools/knowledge/tool.go` | 框架无对应功能 | 框架无 reflect 工具 |
| 基础分块器（char/token） | `internal/knowledge/chunker.go` | `chunking/fixed` | 项目先于框架分块功能开发 |
| 文档摄入管线（异步+事件） | `internal/service/knowledge.go` | `BuiltinKnowledge.Load()` | 需要 API 驱动、异步处理、WebSocket 状态推送 |
| 知识库 Cue 注入 | `internal/agent/knowledge_inject.go` | 框架无对应功能 | 项目自定义 BeforeModelHook |
| EmbedSetting 持久化 | `internal/biz/knowledge/knowledge.go` | 框架无对应功能 | 需要 Admin UI 运行时配置嵌入器 |
| OCR 接口 | `internal/knowledge/ocr.go` | 框架无 OCR | 框架暂无 OCR 支持（接口已定义，当前 noop） |

### 2.3 未使用的框架功能

| 框架功能 | 未使用原因 | 是否需要启用 |
|---------|-----------|-------------|
| `knowledge.Knowledge` 接口 | 项目自建完整 RAG 管线，不使用框架顶层接口 | 评估中 |
| `BuiltinKnowledge` | 项目需要多集合管理、API 驱动摄入、运行时配置，与框架 Load 模式不匹配 | 评估中 |
| `vectorstore/pgvector` | 项目自建 Raw SQL 实现，需与项目 Postgres 连接池集成 | 评估中 |
| `embedder/openai` 等 | 项目需要运行时切换提供商和 Admin UI 配置 | 评估中 |
| `query/llm` Enhancer | 项目自建 QueryRewriter，支持 3 种策略 | 否（项目更完善） |
| `source.*` 系列 | 项目通过 API 上传文档，不使用文件/URL/目录源 | 否（场景不同） |
| `NewKnowledgeSearchTool` | 项目自建工具，需要 context 注入 + 集合权限控制 | 评估中 |
| `NewAgenticFilterSearchTool` | 项目未使用智能过滤 | 评估中 |
| `NewCodeSearchTool` / `NewCodeGraphSearchTool` | 项目无代码搜索场景 | 否 |
| `NewGraphToolSet` | 项目无图知识库场景 | 否 |
| `WithKnowledge()` Agent 集成 | 项目通过 context 注入 + BeforeModelHook 集成 | 评估中 |
| 增量同步（`WithEnableSourceSync`） | 项目通过 API 驱动摄入，不需要 Source 同步 | 否 |
| 进度回调（`WithLoadProgressCallback`） | 项目使用 WebSocket 推送摄入状态 | 否 |

---

## 三、对比分析

### 3.1 框架优势（项目应采纳的）

| # | 框架优势 | 项目现状 | 对齐收益 |
|---|---------|---------|---------|
| 1 | **VectorStore 接口标准化**：6 种后端可插拔，统一 `Add/Search/Delete` 接口 | 项目自建 pgvector Raw SQL，仅支持 Postgres | 可获得 Milvus/Qdrant/Elasticsearch 等后端支持，减少约 200 行 Raw SQL |
| 2 | **Embedder 接口标准化**：4 种嵌入器可插拔，统一 `GetEmbedding/GetDimensions` 接口 | 项目自建 `MultiProviderEmbedder`，4 种提供商硬编码 HTTP 调用 | 可减少约 250 行嵌入器代码，获得框架维护的 API 兼容性 |
| 3 | **SearchTool 标准化**：`NewKnowledgeSearchTool` 自动注入 History/UserID/SessionID，支持静态过滤 + 智能过滤 | 项目自建工具，手动通过 context 注入依赖 | 可减少约 100 行工具代码，获得框架的自动上下文注入 |
| 4 | **Agent 集成标准化**：`WithKnowledge()` 一行集成，自动注册搜索工具 | 项目需要手动 context 注入 + BeforeModelHook | 简化 Agent 构建代码 |
| 5 | **增量同步机制**：`WithEnableSourceSync` 自动检测文档变更，仅处理增量 | 项目无增量同步，每次摄入全量处理 | 减少重复嵌入开销 |
| 6 | **过滤系统**：`UniversalFilterCondition` 支持递归嵌套 and/or，`AgenticFilterSearchTool` 让 LLM 自动构建 filter | 项目仅支持简单 JSON metadata filter | 增强搜索灵活性 |

### 3.2 项目优势（框架缺失的）

| # | 项目优势 | 框架现状 | 建议处理 |
|---|---------|---------|---------|
| 1 | **多集合管理**：Collection/Document/Chunk 三层模型 + CRUD API + 权限控制 | 框架无集合概念，Knowledge 实例绑定单一 Source 列表 | 贡献回框架（Collection 抽象） |
| 2 | **混合检索**：Dense + Sparse(BM25) + RRF 融合 | 框架仅支持 Dense 检索（VectorStore.Search） | 贡献回框架（HybridRetriever） |
| 3 | **自适应路由**：查询复杂度分类 → 自动选择检索模式 | 框架无对应功能 | 贡献回框架（AdaptiveRouter） |
| 4 | **查询重写**：HyDE/Decomposition/MultiQuery 三种策略 | 框架仅有 `query/llm` Enhancer（单一策略） | 贡献回框架（扩展 QueryEnhancer） |
| 5 | **联邦检索**：跨集合 Broadcast/Route 策略 | 框架无对应功能 | 贡献回框架（FederatedRetriever） |
| 6 | **检索评估**：LLM 评估检索质量 + 补充检索 | 框架无对应功能 | 贡献回框架（RetrievalEvaluator） |
| 7 | **knowledge_reflect 工具**：跨集合搜索 + 质量评估 | 框架仅有 `knowledge_search` | 贡献回框架 |
| 8 | **运行时配置**：Admin UI 动态切换嵌入器提供商/模型/维度 | 框架 Embedder 在初始化时固定 | 贡献回框架（EmbedderAdmin 接口） |
| 9 | **API 驱动摄入**：异步文档摄入 + WebSocket 状态推送 | 框架 `Load()` 是同步批量加载 | 保持自建（场景差异大） |
| 10 | **OCR 接口**：预留 OCR 扩展点 | 框架无 OCR 支持 | 贡献回框架 |

### 3.3 差异根因分析

| 差异点 | 根因 | 影响范围 |
|--------|------|---------|
| 未使用 `knowledge.Knowledge` 接口 | **架构决策**：项目需要多集合管理、API 驱动摄入、运行时配置，与框架的"单实例+Load"模式不匹配 | 全模块 |
| 自建 VectorStore | **功能缺失**：框架 `vectorstore/pgvector` 不支持 BM25、RRF 融合、项目 Postgres 连接池集成 | 检索层 |
| 自建 Embedder | **功能缺失**：框架 Embedder 不支持运行时切换提供商和 Admin UI 配置 | 嵌入层 |
| 自建 Retriever | **架构决策**：需要使用项目 biz 层 Repo 接口，不使用框架 VectorStore | 检索层 |
| 自建 QueryRewriter | **功能缺失**：框架 `query/llm` 仅支持单一策略，项目需要 3 种 + 中文优化 | 查询层 |
| 自建 SearchTool | **架构决策**：需要 context 注入依赖 + 集合权限控制，框架工具不支持 | 工具层 |
| 自建摄入管线 | **架构决策**：API 驱动 + 异步 + WebSocket 推送，与框架同步 Load 模式不同 | 摄入层 |
| 自建 HybridRetriever/AdaptiveRouter/FederatedRetriever/Evaluator | **功能缺失**：框架无混合检索、自适应路由、联邦检索、检索评估 | 检索层 |

---

## 四、对齐方案

### 4.1 对齐项清单

| # | 对齐项 | 类型 | 优先级 | 影响范围 | 预期收益 |
|---|--------|------|--------|---------|---------|
| 1 | 实现 `vectorstore.VectorStore` 适配层 | 新增适配层 | P1 | `internal/knowledge/` | 统一存储接口，未来可切换后端 |
| 2 | 实现 `embedder.Embedder` 适配层 | 新增适配层 | P1 | `internal/knowledge/embedder.go` | 减少约 250 行自建嵌入器代码 |
| 3 | 实现 `knowledge.Knowledge` 接口 | 新增适配层 | P2 | `internal/knowledge/` | 统一顶层接口，获得框架生态 |
| 4 | 使用框架 `NewKnowledgeSearchTool` 替换自建工具 | 替换自建实现 | P2 | `internal/tools/knowledge/` | 减少约 100 行工具代码 |
| 5 | 贡献 HybridRetriever 回框架 | 贡献回框架 | P2 | `pkg/trpc-agent-go/knowledge/` | 框架获得混合检索能力 |
| 6 | 贡献 AdaptiveRouter 回框架 | 贡献回框架 | P2 | `pkg/trpc-agent-go/knowledge/` | 框架获得自适应路由 |
| 7 | 贡献 QueryRewriter 多策略回框架 | 贡献回框架 | P3 | `pkg/trpc-agent-go/knowledge/query/` | 框架查询增强更完善 |
| 8 | 贡献 FederatedRetriever 回框架 | 贡献回框架 | P3 | `pkg/trpc-agent-go/knowledge/` | 框架获得联邦检索 |
| 9 | 贡献 RetrievalEvaluator 回框架 | 贡献回框架 | P3 | `pkg/trpc-agent-go/knowledge/` | 框架获得检索评估 |
| 10 | 贡献 Collection 抽象回框架 | 贡献回框架 | P3 | `pkg/trpc-agent-go/knowledge/` | 框架获得多集合管理 |

### 4.2 对齐项详情

#### 对齐项 #1：实现 VectorStore 适配层

**类型**：新增适配层

**现状**：
- 项目当前：自建 pgvector Raw SQL 实现（`internal/data/knowledge.go`），包含 `SearchChunks`/`InsertChunks`/`DeleteChunksByDocument` 等方法，直接操作 Postgres 连接
- 框架提供：`vectorstore.VectorStore` 接口 + `vectorstore/pgvector` 实现

**对齐方案**：
1. 创建 `internal/adapter/knowledge_vectorstore.go`，实现 `vectorstore.VectorStore` 接口
2. 适配层内部委托给项目现有的 `knowledgeRepo`（`internal/data/knowledge.go`）
3. 将 `SearchChunks`/`InsertChunks` 等方法映射到 VectorStore 的 `Search`/`Add`/`Delete` 方法
4. 保留项目特有的 BM25/RRF 功能在适配层之外

**代码变更范围**：
- 新增：`internal/adapter/knowledge_vectorstore.go`（约 150 行）
- 修改：`internal/knowledge/retriever.go`（可选，使用 VectorStore 接口替代 biz.KnowledgeRepo）
- 删除：无（渐进对齐，保留原有实现）

**兼容性风险**：
- 框架 `VectorStore.Search` 返回 `*vectorstore.SearchResult`，项目使用 `[]biz.KnowledgeChunk`，需要类型转换
- 框架 pgvector 实现可能使用不同的表结构，适配层需要桥接

**回退方案**：
- 适配层是可选的，不影响现有代码路径

**验证方法**：
- 现有 `internal/knowledge/retriever_test.go` 全部通过
- 新增适配层单元测试，验证 VectorStore 接口合规性

**预期收益**：
- 代码减少：约 0 行（适配层新增，但为后续替换铺路）
- 性能影响：无（适配层仅做类型转换）
- 维护成本：减少框架升级时的适配工作量
- 功能增强：未来可切换到 Milvus/Qdrant/Elasticsearch 等后端

---

#### 对齐项 #2：实现 Embedder 适配层

**类型**：新增适配层

**现状**：
- 项目当前：自建 `MultiProviderEmbedder`（约 400 行），硬编码 OpenAI/Ollama/Gemini/HuggingFace 四种提供商的 HTTP 调用
- 框架提供：`embedder.Embedder` 接口 + `embedder/openai`/`embedder/gemini`/`embedder/ollama`/`embedder/huggingface` 四种实现

**对齐方案**：
1. 创建 `internal/adapter/knowledge_embedder.go`，实现 `internal/knowledge.QueryEmbedder` + `TaskTypeEmbedder` 接口
2. 适配层内部使用框架 `embedder.Embedder` 实现
3. 保留 `EmbedderAdmin` 接口（运行时配置切换），框架不支持此功能
4. 当运行时配置变更时，重新创建框架 Embedder 实例

**代码变更范围**：
- 新增：`internal/adapter/knowledge_embedder.go`（约 120 行）
- 修改：`internal/service/knowledge_embedder.go`（使用适配层替代 `MultiProviderEmbedder`）
- 删除：`internal/knowledge/embedder.go` 中的 HTTP 调用代码（约 250 行）

**兼容性风险**：
- 框架 `embedder.Embedder` 返回 `[]float64`，项目使用 `[]float32`，需要类型转换
- 框架 Embedder 不支持运行时切换提供商，需要适配层在配置变更时重建实例
- 框架 Gemini Embedder 使用 `genai` SDK，项目也使用 `genai`，但版本可能不同

**回退方案**：
- 保留 `MultiProviderEmbedder` 作为 fallback，适配层失败时回退

**验证方法**：
- 现有 `internal/knowledge/ingest_test.go` 全部通过
- 新增适配层单元测试，验证各提供商嵌入结果一致性

**预期收益**：
- 代码减少：约 250 行（删除自建 HTTP 调用代码）
- 性能影响：无（框架 Embedder 也是 HTTP 调用）
- 维护成本：减少 API 变更时的适配工作量（框架维护 API 兼容性）
- 功能增强：获得框架新增的嵌入器支持

---

#### 对齐项 #3：实现 Knowledge 接口

**类型**：新增适配层

**现状**：
- 项目当前：不使用框架 `knowledge.Knowledge` 接口，自建多层检索编排
- 框架提供：`knowledge.Knowledge` 接口（`Search` 方法），`BuiltinKnowledge` 默认实现

**对齐方案**：
1. 创建 `internal/adapter/knowledge_service.go`，实现 `knowledge.Knowledge` 接口
2. 适配层内部委托给项目的 `AdaptiveRouter.Search()` 方法
3. 将项目的 `biz.KnowledgeSearchQuery` 映射到框架的 `knowledge.SearchRequest`
4. 将项目的 `[]biz.KnowledgeChunk` 映射到框架的 `knowledge.SearchResult`
5. 支持 `knowledge.WithKnowledge()` Agent 集成方式

**代码变更范围**：
- 新增：`internal/adapter/knowledge_service.go`（约 100 行）
- 修改：`internal/agent/builder_deps.go`（可选，增加 Knowledge 接口依赖）
- 删除：无

**兼容性风险**：
- 框架 `SearchRequest` 包含 `History`/`UserID`/`SessionID`，项目通过 context 传递，需要适配
- 框架 `SearchResult` 结构与项目 `[]biz.KnowledgeChunk` 不同，需要映射
- 项目多集合搜索需要通过 `SearchFilter.DocumentIDs` 适配

**回退方案**：
- 适配层是可选的，不影响现有代码路径

**验证方法**：
- 框架 `knowledge.Knowledge` 接口合规性测试
- 现有搜索功能回归测试

**预期收益**：
- 代码减少：约 0 行（适配层新增）
- 性能影响：无
- 维护成本：统一接口，减少框架升级适配
- 功能增强：可使用 `WithKnowledge()` 一行集成 Agent，获得框架生态

---

#### 对齐项 #4：使用框架 SearchTool 替换自建工具

**类型**：替换自建实现

**现状**：
- 项目当前：自建 `knowledge_search` + `knowledge_reflect` 两个工具（约 200 行），通过 context 注入 Retriever/Router/Evaluator
- 框架提供：`NewKnowledgeSearchTool`（自动注入 History/UserID/SessionID）+ `NewAgenticFilterSearchTool`（智能过滤）

**对齐方案**：
1. 使用框架 `NewKnowledgeSearchTool` 替换自建 `knowledge_search`
2. 通过 `WithFilter` 传入集合 ID 过滤条件
3. 保留 `knowledge_reflect` 工具（框架无对应功能）
4. 在 Agent 构建时使用 `WithKnowledge()` 替代手动 context 注入

**代码变更范围**：
- 修改：`internal/agent/tool_assembly.go`（使用框架工具注册方式）
- 修改：`internal/agent/chat_orchestrator_turn_phases.go`（移除手动 context 注入）
- 保留：`internal/tools/knowledge/tool.go` 中的 `knowledge_reflect` 和 context 辅助函数
- 删除：`internal/tools/knowledge/tool.go` 中的 `NewSearchTool()` 函数（约 80 行）

**兼容性风险**：
- 框架工具的参数 schema 与项目不同（框架用 `query` 字符串，项目用 `collection_id` + `query`）
- 框架工具通过 `WithFilter` 限制集合范围，项目通过 context 传递 `KnowledgeBases`
- 框架工具自动注入 History，项目当前未使用此功能
- **高风险**：框架工具参数 schema 变更会导致 Agent 行为变化

**回退方案**：
- 保留自建工具，框架工具作为可选路径

**验证方法**：
- Agent 端到端测试：验证知识库搜索功能正常
- 对比框架工具与自建工具的搜索结果

**预期收益**：
- 代码减少：约 80 行
- 性能影响：无
- 维护成本：减少工具维护负担
- 功能增强：获得框架的智能过滤、自动上下文注入

---

#### 对齐项 #5：贡献 HybridRetriever 回框架

**类型**：贡献回框架

**现状**：
- 项目当前：`HybridRetriever` 支持 Dense/Sparse/RRF 三种模式，RRF 融合排序
- 框架现状：仅支持 Dense 检索（VectorStore.Search），无混合检索

**对齐方案**：
1. 将 `internal/knowledge/hybrid_retriever.go` 重构为框架 `knowledge/retriever/hybrid/` 包
2. 适配框架接口：使用 `vectorstore.VectorStore` 替代 `biz.KnowledgeRepo`，使用框架 `reranker.Reranker`
3. 抽象 `SparseSearcher` 为框架接口（`vectorstore.TextSearcher` 或类似）
4. 在框架 `retriever/default` 中增加混合检索模式选项

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/knowledge/retriever/hybrid/`（约 200 行）
- 修改：`pkg/trpc-agent-go/knowledge/retriever/default/`（增加混合模式选项）
- 修改：`pkg/trpc-agent-go/knowledge/vectorstore/vectorstore.go`（增加 TextSearch 接口）

**兼容性风险**：
- 框架 VectorStore 接口需要扩展 BM25 搜索方法，可能影响现有实现
- RRF 融合算法的 K 参数需要标准化

**回退方案**：
- 贡献不成功则保持自建

**验证方法**：
- 框架 retriever 测试套件
- 项目混合检索回归测试

**预期收益**：
- 代码减少：约 200 行（项目自建代码迁移到框架）
- 性能影响：无
- 维护成本：框架维护混合检索逻辑
- 功能增强：框架生态获得混合检索能力

---

#### 对齐项 #6：贡献 AdaptiveRouter 回框架

**类型**：贡献回框架

**现状**：
- 项目当前：`AdaptiveRouter` 根据查询复杂度（Simple/Moderate/Complex）自动选择检索模式，支持 MultiQuery 并行检索
- 框架现状：无自适应路由

**对齐方案**：
1. 将 `internal/knowledge/adaptive_router.go` 重构为框架 `knowledge/retriever/adaptive/` 包
2. 依赖 HybridRetriever（对齐项 #5）
3. 复杂度分类逻辑可配置化

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/knowledge/retriever/adaptive/`（约 150 行）

**兼容性风险**：
- 依赖 HybridRetriever 贡献成功

**回退方案**：
- 贡献不成功则保持自建

**验证方法**：
- 框架 retriever 测试套件
- 项目自适应路由回归测试

**预期收益**：
- 代码减少：约 150 行
- 维护成本：框架维护自适应路由逻辑

---

#### 对齐项 #7~10：贡献 QueryRewriter/FederatedRetriever/Evaluator/Collection 回框架

**类型**：贡献回框架

**现状**：
- 项目有 4 个框架缺失的高级功能，每个都是独立可贡献的模块
- 依赖关系：FederatedRetriever 依赖 AdaptiveRouter（#6），其余独立

**对齐方案**：
- QueryRewriter：扩展框架 `query/llm`，增加 HyDE/Decomposition/MultiQuery 策略选择
- FederatedRetriever：新增 `knowledge/retriever/federated/` 包
- RetrievalEvaluator：新增 `knowledge/retriever/evaluator/` 包
- Collection：在框架 Knowledge 层增加 Collection 抽象

**代码变更范围**：
- 每个贡献约 100-200 行新增框架代码

**预期收益**：
- 代码减少：总计约 500 行项目自建代码可迁移到框架
- 维护成本：框架统一维护高级检索功能

---

## 五、实施路线

### 5.1 阶段规划

| 阶段 | 对齐项 | 前置依赖 | 预计工作量 |
|------|--------|---------|-----------|
| Phase 1 | #1 VectorStore 适配层, #2 Embedder 适配层 | 无 | 中 |
| Phase 2 | #3 Knowledge 接口实现, #4 SearchTool 替换 | Phase 1 | 中 |
| Phase 3 | #5 HybridRetriever 贡献, #6 AdaptiveRouter 贡献 | Phase 2 | 大 |
| Phase 4 | #7~10 高级功能贡献 | Phase 3 | 大 |

### 5.2 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| 框架 VectorStore 接口不支持 BM25，适配层无法覆盖混合检索 | 高 | 高 | 适配层仅覆盖 Dense 检索，BM25 保持自建；同时向框架提议扩展接口 |
| 框架 Embedder 返回 `[]float64`，项目使用 `[]float32`，精度损失 | 低 | 低 | 适配层做类型转换，float64→float32 精度损失可忽略 |
| 框架 SearchTool 参数 schema 与项目不兼容 | 高 | 高 | 优先使用适配层包装框架工具，保持项目 schema 不变 |
| 贡献回框架的 PR 被拒绝或长期未合并 | 中 | 中 | 保持自建实现，适配层作为可选路径 |
| 运行时配置切换（EmbedderAdmin）框架不支持 | 高 | 中 | 适配层内部重建框架 Embedder 实例，Admin 接口保持自建 |
| 对齐过程中搜索结果回归 | 中 | 高 | 每个对齐项都有回归测试 + 回退方案 |

---

## 六、附录

### A. 框架示例代码参考（必填）

| 示例 | 路径 | 关键 API | 初始化模式 | 与项目实现差异 |
|------|------|---------|-----------|--------------|
| Knowledge Basic | `examples/knowledge/basic/main.go` | `knowledge.New` + `WithVectorStore` + `WithEmbedder` + `WithSources` + `kb.Load` + `knowledgetool.NewKnowledgeSearchTool` | 1. 创建 Source → 2. 创建 VectorStore → 3. 创建 Embedder → 4. `knowledge.New(opts...)` → 5. `kb.Load(ctx)` → 6. `NewKnowledgeSearchTool(kb)` → 7. Agent `WithTools` | 项目不使用 `knowledge.New`，不使用 `kb.Load`（API 驱动摄入），不使用框架 SearchTool |
| Skill (Knowledge Search) | `examples/skill/main.go` + `helper.go` | `llmagent.WithSkills` + `llmagent.WithSkillToolProfile` | Skill Repository + CodeExecutor → Agent `WithSkills` | 项目不使用 Skill 方式集成知识库，而是直接注册 knowledge_search 工具 |

**对齐目标状态**：
- Phase 1-2 完成后，项目应能通过 `knowledge.New(WithVectorStore(adapter), WithEmbedder(adapter))` 创建框架 Knowledge 实例
- Phase 2 完成后，Agent 可通过 `WithKnowledge(kb)` 一行集成知识库搜索
- 框架示例的初始化模式（Source→VectorStore→Embedder→Knowledge.New→Load→SearchTool→Agent）将作为对齐的目标架构

### B. 框架文档参考

| 文档 | 路径 |
|------|------|
| Knowledge 中文文档 | `docs/mkdocs/zh/knowledge/index.md` |
| Knowledge 英文文档 | `docs/mkdocs/en/knowledge/index.md` |
