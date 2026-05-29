# Knowledge 知识库 — 建设方向与演进方案

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
