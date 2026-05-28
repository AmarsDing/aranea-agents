# 神经记忆系统 — 设计

> **需求**：[`neural-memory.md`](./neural-memory.md) · **开发计划**：[`neural-memory-development.md`](./neural-memory-development.md)
> **规范**：[`AI-DEVELOPMENT-SPECIFICATION.md`](../../guides/AI-DEVELOPMENT-SPECIFICATION.md)
> **前置设计**：[`memory.design.md`](./memory.design.md) · [`L3.design.md`](./L3.design.md) · [`L4.design.md`](./L4.design.md)
> **学术依据**：[`memory-optimization-proposal.md`](../../review/memory-optimization-proposal.md)

---

## 一、设计立场

本设计是对 [`memory.design.md`](./memory.design.md) 的**增量扩展**，不改变现有五层架构的存储拓扑和双轨关系，仅在 L3/L4 之上叠加**时间感知、关联链接、联动更新、仿生生命周期**四个能力层。

核心立场：

1. **增量而非重构**：所有新增字段为 `ALTER TABLE ADD COLUMN`，不改变现有表的主键和唯一约束
2. **后台 Agent 是调度器，不是第六层**：Memory-Agent 统一调度现有 Worker，不维护独立存储
3. **联动更新走 Action Log**：所有联动操作写入 `memory_action_log`，可审计可回滚
4. **巩固是视图合并，不是数据删除**：去重/合并产生新版本记录，原始数据归档而非删除

---

## 二、数据模型

### 2.1 L3 Fact 新增字段

```sql
-- 语义时间线
ALTER TABLE memory_facts ADD COLUMN semantic_time_start TEXT NOT NULL DEFAULT '';
-- 事实实际发生的开始时间 (ISO 8601)
ALTER TABLE memory_facts ADD COLUMN semantic_time_end TEXT NOT NULL DEFAULT '';
-- 事实实际发生的结束时间 (''=持续有效)
ALTER TABLE memory_facts ADD COLUMN dialogue_time TEXT NOT NULL DEFAULT '';
-- 对话中提及的时间 (保留，作为提取时间)
ALTER TABLE memory_facts ADD COLUMN temporal_confidence REAL NOT NULL DEFAULT 0;
-- 时间提取的置信度 0-1

-- 波动性
ALTER TABLE memory_facts ADD COLUMN volatility TEXT NOT NULL DEFAULT 'moderate';
-- static / slow / moderate / fast / transient
-- static: 出生地、毕业院校 → 不衰减
-- slow: 偏好、习惯 → 慢衰减
-- moderate: 工作、居住 → 中速衰减 (默认)
-- fast: 当前项目、近期计划 → 快衰减
-- transient: 心情、今日安排 → 极快衰减

-- 印迹成熟
ALTER TABLE memory_facts ADD COLUMN access_count INTEGER NOT NULL DEFAULT 0;
-- 被召回次数 (已有 hit_count，新增 access_count 区分: hit_count=检索命中, access_count=实际进入 prompt)
ALTER TABLE memory_facts ADD COLUMN last_accessed_at TEXT NOT NULL DEFAULT '';
-- 最后被召回时间

-- 联动标记
ALTER TABLE memory_facts ADD COLUMN pending_validation INTEGER NOT NULL DEFAULT 0;
-- 1=待验证 (高波动性+长时间未验证)
ALTER TABLE memory_facts ADD COLUMN superseding_fact_id TEXT NOT NULL DEFAULT '';
-- 取代本条的新 fact id (与 superseded_by 互补方向)
```

**索引**：

```sql
CREATE INDEX IF NOT EXISTS idx_memory_facts_semantic_time
  ON memory_facts(scope_type, scope_id, semantic_time_end, volatility);

CREATE INDEX IF NOT EXISTS idx_memory_facts_pending_validation
  ON memory_facts(pending_validation, volatility, last_accessed_at);
```

### 2.2 L4 Relation 新增字段

```sql
-- 双时态模型
ALTER TABLE memory_relations ADD COLUMN valid_from TEXT NOT NULL DEFAULT '';
-- 关系生效时间
ALTER TABLE memory_relations ADD COLUMN valid_until TEXT NOT NULL DEFAULT '';
-- 关系失效时间 (''=持续有效)
ALTER TABLE memory_relations ADD COLUMN ingested_at TEXT NOT NULL DEFAULT '';
-- 系统录入时间
ALTER TABLE memory_relations ADD COLUMN invalidation_reason TEXT NOT NULL DEFAULT '';
-- 失效原因: conflict / superseded / manual / decay
ALTER TABLE memory_relations ADD COLUMN superseding_relation_id TEXT NOT NULL DEFAULT '';
-- 取代本条的新 relation id
```

**索引**：

```sql
CREATE INDEX IF NOT EXISTS idx_memory_relations_temporal
  ON memory_relations(source_entity_id, relation_type, valid_until);
```

### 2.3 新增表：`memory_fact_links`

L3 fact 之间的关联链接，参考 A-Mem Zettelkasten 机制。

```sql
CREATE TABLE IF NOT EXISTS memory_fact_links (
  id TEXT PRIMARY KEY,

  source_fact_id TEXT NOT NULL,
  target_fact_id TEXT NOT NULL,

  link_type TEXT NOT NULL,
  -- semantic: 语义关联 (同一主题)
  -- temporal: 时间关联 (同一时间段)
  -- causal: 因果关联 (A 导致 B)
  -- contradicts: 矛盾关联 (A 与 B 矛盾)
  -- supersedes: 取代关联 (A 取代 B)

  link_strength REAL NOT NULL DEFAULT 0.5,
  -- 0-1, 由 LLM 或 embedding 相似度决定

  auto_generated INTEGER NOT NULL DEFAULT 1,
  -- 1=系统自动生成, 0=人工/Agent 手动

  created_at TEXT NOT NULL,

  UNIQUE(source_fact_id, target_fact_id, link_type)
);

CREATE INDEX IF NOT EXISTS idx_memory_fact_links_source
  ON memory_fact_links(source_fact_id, link_type);

CREATE INDEX IF NOT EXISTS idx_memory_fact_links_target
  ON memory_fact_links(target_fact_id, link_type);
```

### 2.4 新增表：`memory_evolution_log`

联动更新和巩固操作的审计日志。

```sql
CREATE TABLE IF NOT EXISTS memory_evolution_log (
  id TEXT PRIMARY KEY,

  operation TEXT NOT NULL,
  -- linked_update: 联动更新
  -- consolidation_merge: 巩固合并
  -- consolidation_dedup: 巩固去重
  -- consolidation_durative: 持续性记忆构建
  -- consolidation_disambiguate: 实体消歧
  -- reconsolidation: 提取再巩固
  -- conflict_auto_resolve: 冲突自动解决

  trigger_fact_id TEXT NOT NULL DEFAULT '',
  -- 触发操作的 fact id
  affected_fact_ids_json TEXT NOT NULL DEFAULT '[]',
  -- 受影响的 fact id 列表
  affected_relation_ids_json TEXT NOT NULL DEFAULT '[]',
  -- 受影响的 relation id 列表

  depth INTEGER NOT NULL DEFAULT 0,
  -- 联动更新递归深度 (0=直接触发)

  risk_level TEXT NOT NULL DEFAULT 'low',
  -- low / medium / high

  auto_applied INTEGER NOT NULL DEFAULT 1,
  -- 1=自动执行, 0=需审批

  before_snapshot TEXT NOT NULL DEFAULT '',
  -- 变更前快照 (JSON)
  after_snapshot TEXT NOT NULL DEFAULT '',
  -- 变更后快照 (JSON)

  reason TEXT NOT NULL DEFAULT '',
  -- 操作原因 (LLM 判断结果)

  llm_tokens_used INTEGER NOT NULL DEFAULT 0,
  -- 本次操作消耗的 LLM token 数

  tenant_id TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',

  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memory_evolution_log_trigger
  ON memory_evolution_log(trigger_fact_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_memory_evolution_log_operation
  ON memory_evolution_log(operation, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_memory_evolution_log_tenant
  ON memory_evolution_log(tenant_id, created_at DESC);
```

### 2.5 新增表：`memory_consolidation_state`

巩固任务状态追踪。

```sql
CREATE TABLE IF NOT EXISTS memory_consolidation_state (
  id TEXT PRIMARY KEY,

  tenant_id TEXT NOT NULL,
  consolidation_type TEXT NOT NULL,
  -- dedup / merge / durative / disambiguate / link_completion / episode_refine

  status TEXT NOT NULL DEFAULT 'pending',
  -- pending / running / completed / failed / timeout

  scope_type TEXT NOT NULL DEFAULT '',
  scope_id TEXT NOT NULL DEFAULT '',

  facts_processed INTEGER NOT NULL DEFAULT 0,
  facts_merged INTEGER NOT NULL DEFAULT 0,
  facts_archived INTEGER NOT NULL DEFAULT 0,
  links_created INTEGER NOT NULL DEFAULT 0,

  started_at TEXT NOT NULL DEFAULT '',
  completed_at TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0,

  error_message TEXT NOT NULL DEFAULT '',

  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memory_consolidation_state_tenant
  ON memory_consolidation_state(tenant_id, status, created_at DESC);
```

---

## 三、核心接口设计

### 3.1 Biz 层新增接口

#### 3.1.1 时间感知

```go
// internal/biz/memory_temporal.go

type FactTemporalExt struct {
    SemanticTimeStart string
    SemanticTimeEnd   string
    DialogueTime      string
    TemporalConfidence float64
    Volatility        string
}

type TemporalExtractor interface {
    ExtractTemporal(statement string, dialogueContext string) (*FactTemporalExt, error)
}

type VolatilityClassifier interface {
    Classify(statement string, factKind string) string
}
```

#### 3.1.2 关联链接

```go
// internal/biz/memory_links.go

type FactLink struct {
    ID             string
    SourceFactID   string
    TargetFactID   string
    LinkType       string
    LinkStrength   float64
    AutoGenerated  bool
    CreatedAt      time.Time
}

type FactLinkStore interface {
    InsertLink(ctx context.Context, link *FactLink) error
    ListLinksBySource(ctx context.Context, factID string) ([]*FactLink, error)
    ListLinksByTarget(ctx context.Context, factID string) ([]*FactLink, error)
    DeleteLink(ctx context.Context, id string) error
}

type FactLinkGenerator interface {
    GenerateLinks(ctx context.Context, factID string, statement string, scopeType string, scopeID string) ([]*FactLink, error)
}
```

#### 3.1.3 联动更新

```go
// internal/biz/memory_linked_update.go

type LinkedUpdateTrigger struct {
    FactID      string
    ChangeType  string
    ChangedBy   string
    Reason      string
}

type LinkedUpdateResult struct {
    EvaluatedCount int
    UpdatedCount   int
    ProposedCount int
    SkippedCount  int
    MaxDepth      int
    TokensUsed    int
}

type LinkedUpdateEngine interface {
    EvaluateLinkedUpdates(ctx context.Context, trigger LinkedUpdateTrigger) (*LinkedUpdateResult, error)
}
```

#### 3.1.4 巩固

```go
// internal/biz/memory_consolidation.go

type ConsolidationTask struct {
    ID               string
    TenantID         string
    Type             string
    ScopeType        string
    ScopeID          string
    FactsProcessed   int
    FactsMerged      int
    FactsArchived    int
    LinksCreated     int
}

type ConsolidationEngine interface {
    RunDedup(ctx context.Context, tenantID string, scopeType string, scopeID string) (*ConsolidationTask, error)
    RunMerge(ctx context.Context, tenantID string, scopeType string, scopeID string) (*ConsolidationTask, error)
    RunDurativeBuild(ctx context.Context, tenantID string, scopeType string, scopeID string) (*ConsolidationTask, error)
    RunDisambiguate(ctx context.Context, tenantID string) (*ConsolidationTask, error)
    RunLinkCompletion(ctx context.Context, tenantID string, scopeType string, scopeID string) (*ConsolidationTask, error)
    RunEpisodeRefine(ctx context.Context, tenantID string) (*ConsolidationTask, error)
}
```

#### 3.1.5 仿生衰减

```go
// internal/biz/memory_bio_decay.go

type BioDecayInput struct {
    FactID           string
    Volatility       string
    AccessCount      int
    LastAccessedAt   time.Time
    InterferenceScore float64
}

type BioDecayResult struct {
    NewImportance float64
    DecayReason   string
}

type BioDecayEngine interface {
    ComputeDecay(ctx context.Context, input *BioDecayInput) (*BioDecayResult, error)
    ApplyBioDecayBatch(ctx context.Context, tenantID string, scopeType string, scopeID string) (int, error)
}
```

#### 3.1.6 提取再巩固

```go
// internal/biz/memory_reconsolidation.go

type ReconsolidationCheck struct {
    FactID          string
    IsOutdated      bool
    HasConflict     bool
    NeedsValidation bool
    ConflictFactIDs []string
}

type ReconsolidationEngine interface {
    CheckOnRecall(ctx context.Context, factID string) (*ReconsolidationCheck, error)
    MarkPendingValidations(ctx context.Context, tenantID string) (int, error)
}
```

### 3.2 Memory-Agent 调度接口

```go
// internal/biz/memory_agent.go

type MemoryAgentConfig struct {
    Enabled                       bool
    ConsolidationEnabled          bool
    ConsolidationMinIntervalMin   int
    LinkedUpdateEnabled           bool
    LinkedUpdateMaxDepth          int
    LinkedUpdateAutoApplyRisk     string
    InterferenceForgettingEnabled bool
    EngramMaturationEnabled       bool
    ReconsolidationOnRecall       bool
    AgentMemoryToolsEnabled       bool
}

type MemoryAgent interface {
    Start(ctx context.Context) error
    Stop()
    GetStatus() *MemoryAgentStatus
}

type MemoryAgentStatus struct {
    Running              bool
    LastConsolidationAt  string
    LastAuditAt          string
    PendingProposals     int
    PendingValidations   int
    ConsolidationStats   *ConsolidationStats
}
```

### 3.3 Agent Memory Tool 定义

```go
// internal/agent/memory_tools.go

// Tool 注册到 trpc-agent-go 的 Tool 系统
// 仅当 agent_memory_tools_enabled=true 时装配

var MemoryToolDefinitions = []ToolDef{
    {
        Name:        "memory_fact_add",
        Description: "主动添加一条长期记忆事实",
        Parameters:  map[string]interface{}{"statement": "", "scope": "", "confidence": 0.8},
    },
    {
        Name:        "memory_fact_update",
        Description: "更新已有长期记忆事实的内容",
        Parameters:  map[string]interface{}{"fact_id": "", "statement": "", "reason": ""},
    },
    {
        Name:        "memory_fact_search",
        Description: "主动检索长期记忆事实（非注入场景）",
        Parameters:  map[string]interface{}{"query": "", "top_k": 5},
    },
    {
        Name:        "memory_entity_link",
        Description: "在知识图谱中建立实体关系",
        Parameters:  map[string]interface{}{"source_entity": "", "relation": "", "target_entity": ""},
    },
}
```

---

## 四、核心流程

### 4.1 时间感知提取流程

```
AutoMemoryWorker.extract()
    │
    ├─ 现有: LLM 提取 fact statement
    │
    ├─ 新增: TemporalExtractor.ExtractTemporal(statement, dialogueContext)
    │   ├─ LLM 从对话中提取语义时间 ("上周" → 具体日期)
    │   ├─ 计算 temporal_confidence
    │   └─ 返回 FactTemporalExt
    │
    ├─ 新增: VolatilityClassifier.Classify(statement, factKind)
    │   ├─ 规则版本: 基于 fact_kind + 关键词映射
    │   │   fact_kind=preference → "slow"
    │   │   fact_kind=fact + 含"出生"/"毕业" → "static"
    │   │   fact_kind=fact + 含"工作于"/"住在" → "moderate"
    │   │   fact_kind=pattern + 含"今天"/"现在" → "transient"
    │   └─ 后续: 学习版本 (embedding → 分类器)
    │
    └─ 写入 memory_facts (含新增字段)
```

### 4.2 L4 双时态冲突自动失效

```
L4 WriteFromUserText → UpsertRelation
    │
    ├─ 检查同 (source_entity_id, relation_type) 的现有 relation
    │
    ├─ 如果存在且语义冲突:
    │   ├─ 旧 relation: valid_until = now(), invalidation_reason = 'conflict'
    │   ├─ 新 relation: valid_from = now()
    │   └─ 触发联动更新评估 (优化项 B)
    │
    ├─ 如果存在且不冲突 (补充信息):
    │   └─ 新 relation: valid_from = now(), 旧 relation 保持
    │
    └─ 如果不存在:
        └─ 新 relation: valid_from = now()
```

### 4.3 联动更新流程

```
触发: fact F 被变更 (内容修改 / 时间线变更 / 状态变更)
    │
    ▼
Step 1: 查询关联
    SELECT * FROM memory_fact_links WHERE source_fact_id = F.id
    UNION
    SELECT * FROM memory_fact_links WHERE target_fact_id = F.id
    │
    ▼
Step 2: 逐条评估 (depth=0)
    对每条关联 fact F':
    ├─ LLM 判断: F 的变更是否影响 F' 的有效性?
    │   Prompt: "事实A从'{旧内容}'变为'{新内容}'，事实B是'{F'.statement}'，B是否需要更新?"
    │   输出: {needs_update: bool, update_type: "content"/"importance"/"temporal"/"status", reason: string}
    │
    ├─ needs_update=true:
    │   ├─ risk_level = assessRisk(F', update_type)
    │   │   低风险: importance 调整、temporal 标记
    │   │   中风险: 内容修改
    │   │   高风险: 状态变更 (active→archived)
    │   │
    │   ├─ risk_level <= auto_apply_risk_threshold:
    │   │   └─ 自动执行 + 写入 evolution_log
    │   │
    │   └─ risk_level > auto_apply_risk_threshold:
    │       └─ 生成 CascadeProposal (复用现有级联提案机制)
    │
    └─ needs_update=false: 跳过
    │
    ▼
Step 3: 递归 (depth=1,2,...,max_depth)
    对被更新的 F'，重复 Step 1-2
    每层递归 depth+1，超过 max_depth 停止
    │
    ▼
Step 4: 审计
    所有操作写入 memory_evolution_log
    包含: trigger_fact_id, affected_fact_ids, depth, risk_level, before/after snapshot
```

### 4.4 睡眠期巩固流程

```
MemoryConsolidationWorker (Cron, 低峰期)
    │
    ▼
调度: 系统负载 < 阈值 且 距上次巩固 > min_interval
    │
    ▼
按 tenant 顺序执行 (互不干扰):
    │
    ├─ 1. 去重 (Dedup)
    │   SELECT fact pairs WHERE scope_type=X, scope_id=Y
    │     AND embedding_cosine_similarity > 0.92
    │   LLM 判断: 是否语义重复?
    │   合并: 保留更完整的 statement + 合并 metadata
    │   归档: 被合并的 fact status → 'archived'
    │
    ├─ 2. 持续性记忆构建 (Durative Build)
    │   SELECT facts WHERE scope_type=X, scope_id=Y
    │     AND semantic_time_start != ''
    │     AND fact_kind 相同
    │     AND 时间区间相邻或重叠
    │   合并为: "2023-03 至 2025-05 住在北京"
    │
    ├─ 3. Episode→Fact 二次提炼
    │   SELECT episodes WHERE importance < 0.3
    │     AND NOT EXISTS (fact from this episode)
    │   LLM 提取遗漏的 fact
    │
    ├─ 4. 实体消歧 (Disambiguate)
    │   SELECT entity pairs WHERE name 不同
    │     BUT embedding_similarity > 0.85
    │     OR shared_relations > 2
    │   LLM 判断: 是否同一实体?
    │   合并实体 + 更新所有关联 relation
    │
    └─ 5. 链接补全 (Link Completion)
        SELECT facts WHERE NOT EXISTS (link from this fact)
          AND scope 内 fact 数 > 1
        批量生成关联链接
```

### 4.5 干扰性遗忘流程

```
BioDecayWorker (替代现有 L2/L3/L4 DecayWorker)
    │
    ▼
对每条 fact:
    │
    ├─ 计算 base_decay (由 volatility 决定)
    │   static → 1.0 (不衰减)
    │   slow → 0.99
    │   moderate → 0.97
    │   fast → 0.93
    │   transient → 0.85
    │
    ├─ 计算 access_boost (由 access_count 决定)
    │   access_boost = min(0.3, 0.05 × log2(access_count + 1))
    │
    ├─ 计算 interference_factor (由语义冲突决定)
    │   检查同 scope 内是否有 superseding_fact_id != '' 的事实
    │   有 → interference_factor = 0.5 (加速遗忘)
    │   无 → interference_factor = 1.0
    │
    └─ new_importance = importance × base_decay × (1 + access_boost) × interference_factor
```

### 4.6 提取再巩固流程

```
BeforeModelHook → buildRuntimeMemoryCue → L3 RecallFactsFused
    │
    ▼ (每条被召回的 fact)
ReconsolidationEngine.CheckOnRecall(factID)
    │
    ├─ 检查 semantic_time_end != '' → 标记为"历史事实"，检索降权
    │
    ├─ 检查同 scope 内冲突 → 触发联动更新评估
    │
    ├─ 刷新 access_count += 1, last_accessed_at = now() (印迹成熟)
    │
    └─ 如果 volatility ∈ {fast, transient} 且 last_accessed_at 距今 > 30 天
        → pending_validation = 1 (待验证)
```

---

## 五、Proto 扩展

### 5.1 新增 RPC

| RPC | HTTP | 说明 |
|-----|------|------|
| `ListFactLinks` | GET /v1/memory/l3/facts/{fact_id}/links | 查询 fact 关联链接 |
| `CreateFactLink` | POST /v1/memory/l3/facts/links | 手动创建关联链接 |
| `DeleteFactLink` | DELETE /v1/memory/l3/facts/links/{id} | 删除关联链接 |
| `GetLinkedUpdatePreview` | POST /v1/memory/l3/facts/{fact_id}/linked-preview | 预览联动更新影响范围 |
| `ListEvolutionLog` | GET /v1/memory/evolution/log | 查询联动更新/巩固审计日志 |
| `GetConsolidationStatus` | GET /v1/memory/consolidation/status | 查询巩固任务状态 |
| `TriggerConsolidation` | POST /v1/memory/consolidation/trigger | 手动触发巩固 |
| `GetMemoryAgentStatus` | GET /v1/memory/agent/status | 查询 Memory-Agent 状态 |
| `UpdateMemoryAgentConfig` | PUT /v1/memory/agent/config | 更新 Memory-Agent 配置 |
| `AgentMemoryFactAdd` | POST /v1/memory/agent/facts | Agent 自主添加 fact |
| `AgentMemoryFactUpdate` | PUT /v1/memory/agent/facts/{id} | Agent 自主更新 fact |
| `AgentMemoryFactSearch` | POST /v1/memory/agent/facts/search | Agent 自主检索 fact |

### 5.2 现有 RPC 扩展

| RPC | 扩展内容 |
|-----|----------|
| `ListMemoryFacts` | 响应增加 `semantic_time_start/end`、`volatility`、`access_count`、`pending_validation` 字段 |
| `UpsertMemoryFact` | 请求增加 `semantic_time_start/end`、`volatility` 字段 |
| `ListMemoryEntities` | 响应增加 `valid_from/until` 字段 |
| `GetMemoryNeighborhood` | 响应增加 relation 的 `valid_from/until` 字段 |
| `DebugMemoryRecall` | 响应增加再巩固检查结果 |

---

## 六、层间数据流（扩展后）

```
用户消息 → messages (Ledger)
    → L0 装配 ← L1 RenderForPrompt
              ← L3 Recall(facts + 时间过滤 + 再巩固检查)
              ← L4 GraphRecall(neighbors + 时间过滤)
Turn 结束 → MemoryWorker
    → LLM 提取 fact + 语义时间 + 波动性分类
    → 写入 L3 (含 temporal 字段)
    → 生成关联链接 (FactLinkGenerator)
    → 写入 L4 (含双时态 relation)
    → 触发联动更新评估 (LinkedUpdateEngine)
    → 写入 evolution_log

低峰期 → MemoryConsolidationWorker
    → 去重 / 合并 / 持续性记忆 / 消歧 / 链接补全
    → 写入 consolidation_state + evolution_log

衰减 → BioDecayWorker
    → 按 volatility + access_count + interference 计算 new_importance
    → 更新 memory_facts.importance

召回 → BeforeModelHook
    → L3 Recall → 再巩固检查 → 印迹成熟 (access_count++)
    → L4 Recall → 时间过滤 (valid_until IS NULL)
```

---

## 七、与现有架构的对接点

| 对接点 | 现有代码 | 扩展方式 |
|--------|----------|----------|
| 提取管道 | `internal/cronrunner/jobs/auto_memory.go` | extract() 末尾增加 TemporalExtractor + VolatilityClassifier + FactLinkGenerator 调用 |
| L4 写入 | `internal/biz/memory_l4_usecase.go` | WriteFromUserText 增加 valid_from/valid_until + 冲突自动失效 |
| L3 召回 | `internal/biz/memory_l3_fused_recall.go` | RecallFactsFused 增加时间过滤 + 再巩固检查 + access_count++ |
| L4 召回 | `internal/biz/memory_l4_usecase.go` | GraphRecall 增加 valid_until 过滤 |
| 衰减 Worker | `internal/cronrunner/jobs/memory_l3_decay.go` | 替换为 BioDecayWorker |
| 级联提案 | `internal/biz/memory_l4_cascade.go` | 复用 CascadeProposal 机制，联动更新高风险变更走同一审批流 |
| MemorySet | `internal/runtime/memory_set.go` | 增加 MemoryAgent + LinkedUpdateEngine + ConsolidationEngine |
| Agent 装配 | `internal/agent/trpc_build.go` | 条件装配 Memory Tools |
| Proto | `api/kratos/memory/v1/memory.proto` | 新增 RPC + 扩展现有 message |

---

## 八、关键设计原则

1. **时间线是可选的**：`semantic_time_start/end` 默认为空，不强制提取；有则增强，无则不退化
2. **联动更新是异步的**：fact 变更后联动评估不阻塞写入路径，通过事件触发
3. **巩固是增量的**：每次只处理自上次以来的增量数据，不全量扫描
4. **衰减是可配置的**：不同 agent 可配置不同波动性映射和衰减参数
5. **审计是强制的**：所有联动更新和巩固操作必须写入 evolution_log，不可关闭
6. **递归是受限的**：联动更新最大深度 3，单次联动 LLM 调用上限 20
7. **开关是渐进的**：`neural_memory_enabled` 默认 false，逐 tenant 开启
