# 15 L3 语义记忆 / 知识记忆（Semantic Memory）

本文档落地 5 层记忆架构中的 **L3：语义/知识记忆**。L3 是跨会话持久化的**结构化事实知识库**：用户偏好、项目规范、编码风格、技术栈信息、领域知识片段、常见错误与修正方案。类似认知心理学的「语义记忆」（semantic memory），存储**抽象、事实性、可被检索的知识**。

L3 区别于 L2：L2 记录「发生了什么」，L3 记录「什么是真的」。L3 也区别于 L4：L4 是「实体-关系」的图结构，L3 是「断言（statement / fact）」的扁平知识库。

aranea 当前已有一个轻量级 `memory_items` 表，本文档**扩展并兼容**该表，引入完整的事实-反馈-巩固机制，使 L3 真正可用于生产。

> 关联文档：[Memory 知识体系（合并）](./memory.md)（下文 §0）、`12～14` 上层、`5 agent-setting.md`、`7 agent-evolution.md`、`30 ecosystem.md`。

---

## 0. 指导思想（与 Memory 统一思想对齐）

在 **Ledger / Views / Policy** 中，本层是实现 **语义检索型 Derived Views**（向量 / BM25 / 作用域过滤）的主力：facts **可有损压缩与近似**，但必须 **带回源链路**（episode / message / 工具证据），以满足梳理副本 §4「views 回指 Ledger」与 **§8「检索误差」可控**。

- **参与对决策分布的外部修正 Δ**（梳理副本 §6）：注入 L0 的片段即可能影响输出；须在工程上同时具备 **confidence 衰减、冲突检测、反馈 API、去重**——等价于压低 **views 近似**对 System 1 的污染面。
- **时序作为默认语义的一部分**（梳理副本 §13～§14）：产品规则上宜优先 **当前语境**可用的陈述；对「仅存历史曾真」的事实，应通过 **有效期、版本或显式标签**与时间策略区分（细化为 **time_scope**，可与现有 `updated_at`/版本列渐进演进）；避免仅按向量相似把**已废止**事实拉回 **current**。
- **Declarative vs Procedural**：L3 聚焦「是什么」的断言单元（梳理副本 §12）；「怎么做」的路径技能由 L4 / 工具与演化承载，与本层 complementary。
- **MemWeaver 式混合**：抽象 **Passage 级 fact**（可溯源段落）为未来与 **L4 图谱**并用的接口预留空间（梳理副本 §16）。

延伸阅读：[`memory.md`](./memory.md)。

---

## 1. 心智模型与边界

### 1.1 L3 在 5 层中的位置

| 维度 | 描述 |
|------|------|
| 容量 | 单条 ≤ 500 tokens；总数建议 500~5000 条 / 作用域 |
| 持久性 | 跨会话（数天～数月） |
| 访问模式 | 向量语义检索 + 元数据过滤 + BM25 兜底 |
| 与 ADK 对齐 | 对应 `MemoryService.add_session_to_memory` / `search_memory`（生产用 `VertexAiMemoryBankService`） |
| 与 Aranea 现状对齐 | 复用并扩展 `memory_items`；新增 `memory_facts`（结构化版本） + 周边表 |

### 1.2 与其它层的边界

| 边界 | 走向 | 说明 |
|------|------|------|
| L2 → L3 | 巩固管道：从 episode 抽取 facts，去重后 upsert（详见 `14 ...md` §10） |
| L3 → L0 | L0 装配按当前 user_input 检索 ≤ K 条片段，注入 prompt（默认开启） |
| L1 → L3 | L1 字段标记为「可升档」时，episode 巩固阶段把字段抽成 fact |
| L3 ↔ Feedback | 用户/Agent 在 prompt 中确认/反驳，调用 Feedback API 强化或衰减 |
| L3 → L4 | 高频共现的 fact 集会被 L4 抽象为「实体-关系」 |

### 1.3 三级作用域

参考 Claude Code 的 user/project/org 三级，但允许扩展为五级以匹配 aranea：

| scope_type | 说明 | 隔离级别 |
|------------|------|---------|
| `global` | 全平台共享（仅平台管理员可写） | 所有 user 可读 |
| `workspace` | 工作区共享 | workspace 内可读 |
| `user` | 单个 user 的偏好 | 仅 user 可读写 |
| `team` | 单个 Team 的共享知识 | team 成员可读 |
| `agent` | 单个 Agent 的私有事实 | 仅该 Agent 可读写 |

### 1.4 非目标

- 不存对话原文（属于 L2）。
- 不做实体关系图谱（属于 L4）。
- 不替代 RAG 文档库（外部知识由 Skill / MCP 接入）。
- 不存代码 / 二进制 / 大附件。

---

## 2. 需求清单

### 2.1 功能需求

| # | 需求 | 必要性 |
|---|------|--------|
| F1 | 结构化事实存储：每条 fact 包含 statement、scope、tags、confidence、source、版本 | 必须 |
| F2 | 向量语义检索：按 query embedding 取 top_k 相关 facts | 必须 |
| F3 | 元数据过滤：按 scope、tags、agent_id、time_range、min_confidence 过滤 | 必须 |
| F4 | 事实版本化：修改 fact 时保留历史，支持回滚 | 必须 |
| F5 | 用户反馈：confirm/reject API，自动调整 confidence | 必须 |
| F6 | 衰减与遗忘：周期 Job 按使用频率与时效衰减 confidence；低于阈值归档 | 必须 |
| F7 | 冲突检测：新 fact 与现有 fact 矛盾时标记冲突，等待仲裁 | 必须 |
| F8 | 去重：写入前按文本指纹 + embedding 相似度判断 | 必须 |
| F9 | 多源写入：episode 巩固 / 用户手动 / Plugin / 工具 / Skill | 必须 |
| F10 | L0 装配集成：`Recall(query)` 返回 ≤ K 条 fact 片段 | 必须 |
| F11 | 作用域继承：agent → team → workspace → global 自动联合检索 | 必须 |
| F12 | PII 过滤：写入前 PII 检测；高敏感 fact 仅作用域 user 不外泄 | 必须 |
| F13 | 与 ADK 适配：实现 `MemoryService` 接口，便于 ADK 替换 `InMemoryMemoryService` | 推荐 |

### 2.2 非功能需求

| # | 需求 | 目标值 |
|---|------|--------|
| N1 | 检索 P99 延迟 | < 200 ms（top_k=5，作用域内 10K facts） |
| N2 | 检索 Recall@5 | > 90% |
| N3 | 去重率 | > 85% |
| N4 | 写入延迟 P99 | < 80 ms（不含 embedding） |
| N5 | embedding 异步生成 SLA | < 30 s |

---

## 3. 数据模型

### 3.1 兼容并扩展 `memory_items`（已有）

`memory_items` 已经存在，作为快速兼容层：旧版接口仍读旧表；新版以 `memory_facts` 为事实源。在迁移完成前，新写入双写，旧表只读。

```sql
-- 已有
-- CREATE TABLE memory_items (
--   id TEXT PRIMARY KEY, scope_type, scope_id, content,
--   source_session_id, source_message_id, importance, metadata_json,
--   created_at, updated_at
-- );

-- 兼容字段补齐
ALTER TABLE memory_items ADD COLUMN scope_subtype TEXT NOT NULL DEFAULT '';
ALTER TABLE memory_items ADD COLUMN fact_id TEXT NOT NULL DEFAULT '';
-- 引用新表 memory_facts.id；旧版 memory_items 视作 deprecated 视图
```

### 3.2 新增表：`memory_facts`

```sql
CREATE TABLE IF NOT EXISTS memory_facts (
  id TEXT PRIMARY KEY,

  -- 作用域
  scope_type TEXT NOT NULL,
  -- global / workspace / user / team / agent
  scope_id TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',

  -- 内容
  statement TEXT NOT NULL,
  -- 一句话陈述：用户偏好、项目规范、技术栈信息等
  statement_normalized TEXT NOT NULL DEFAULT '',
  -- 标准化（小写 + 去标点 + trim）用于指纹去重
  fingerprint TEXT NOT NULL DEFAULT '',
  -- sha256(scope_type:scope_id:statement_normalized)
  details_markdown TEXT NOT NULL DEFAULT '',
  -- 详情说明，可选，最大 2K 字符
  fact_kind TEXT NOT NULL DEFAULT 'fact',
  -- fact / preference / rule / pattern / pitfall / glossary
  tags_json TEXT NOT NULL DEFAULT '[]',

  -- 信号
  confidence REAL NOT NULL DEFAULT 0.7,
  importance REAL NOT NULL DEFAULT 0.5,
  use_count INTEGER NOT NULL DEFAULT 0,
  hit_count INTEGER NOT NULL DEFAULT 0,
  -- 被检索命中且实际进入 prompt 的次数
  positive_feedback_count INTEGER NOT NULL DEFAULT 0,
  negative_feedback_count INTEGER NOT NULL DEFAULT 0,
  conflict_count INTEGER NOT NULL DEFAULT 0,

  -- 来源
  source_kind TEXT NOT NULL DEFAULT 'episode',
  -- episode / user / agent / plugin / tool / skill / consolidator
  source_episode_id TEXT NOT NULL DEFAULT '',
  source_session_id TEXT NOT NULL DEFAULT '',
  source_message_id TEXT NOT NULL DEFAULT '',
  source_external TEXT NOT NULL DEFAULT '',
  -- 外部源 URL / Plugin key

  -- 版本
  version INTEGER NOT NULL DEFAULT 1,
  status TEXT NOT NULL DEFAULT 'active',
  -- active / archived / disputed / deprecated / deleted
  superseded_by TEXT NOT NULL DEFAULT '',
  -- 被新版本替换时指向新 fact id

  -- Embedding
  embedding_status TEXT NOT NULL DEFAULT 'pending',
  embedding_model TEXT NOT NULL DEFAULT '',
  embedding_dim INTEGER NOT NULL DEFAULT 0,
  embedding_blob BLOB,
  embedding_norm REAL NOT NULL DEFAULT 0,

  -- 隐私
  pii_flag INTEGER NOT NULL DEFAULT 0,
  redacted_statement TEXT NOT NULL DEFAULT '',

  -- 生命周期
  ttl_days INTEGER NOT NULL DEFAULT 0,
  -- 0 = 永久；> 0 时配合衰减策略
  decay_factor REAL NOT NULL DEFAULT 0.98,
  -- 每次衰减乘数
  next_decay_at TEXT NOT NULL DEFAULT '',
  last_used_at TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL DEFAULT '',

  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  archived_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT '',

  UNIQUE(scope_type, scope_id, fingerprint)
);

CREATE INDEX IF NOT EXISTS idx_memory_facts_scope_status
  ON memory_facts(scope_type, scope_id, status, updated_at);

CREATE INDEX IF NOT EXISTS idx_memory_facts_workspace
  ON memory_facts(workspace_id, status, updated_at);

CREATE INDEX IF NOT EXISTS idx_memory_facts_agent
  ON memory_facts(agent_id, status, last_used_at);

CREATE INDEX IF NOT EXISTS idx_memory_facts_decay
  ON memory_facts(status, next_decay_at);

CREATE INDEX IF NOT EXISTS idx_memory_facts_kind
  ON memory_facts(fact_kind, scope_type, scope_id);
```

### 3.3 新增表：`memory_fact_versions`

```sql
CREATE TABLE IF NOT EXISTS memory_fact_versions (
  id TEXT PRIMARY KEY,
  fact_id TEXT NOT NULL,
  version INTEGER NOT NULL,

  statement TEXT NOT NULL,
  details_markdown TEXT NOT NULL DEFAULT '',
  tags_json TEXT NOT NULL DEFAULT '[]',
  confidence REAL NOT NULL DEFAULT 0.7,
  status TEXT NOT NULL DEFAULT 'active',

  changed_by TEXT NOT NULL DEFAULT '',
  -- user:xxx / agent:xxx / consolidator / plugin:xxx
  change_reason TEXT NOT NULL DEFAULT '',
  -- create / update / merge / split / decay / dispute / supersede / restore
  diff_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  UNIQUE(fact_id, version)
);

CREATE INDEX IF NOT EXISTS idx_memory_fact_versions_fact
  ON memory_fact_versions(fact_id, version DESC);
```

### 3.4 新增表：`memory_fact_feedback`

```sql
CREATE TABLE IF NOT EXISTS memory_fact_feedback (
  id TEXT PRIMARY KEY,
  fact_id TEXT NOT NULL,
  session_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',

  feedback_type TEXT NOT NULL,
  -- confirm / reject / refine / ignore / used / not_used
  source TEXT NOT NULL,
  -- user:xxx / agent:xxx / critic / plugin:xxx / runtime_signal
  weight REAL NOT NULL DEFAULT 1.0,
  comment TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memory_fact_feedback_fact
  ON memory_fact_feedback(fact_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_memory_fact_feedback_session
  ON memory_fact_feedback(session_id, created_at DESC);
```

### 3.5 新增表：`memory_fact_conflicts`

```sql
CREATE TABLE IF NOT EXISTS memory_fact_conflicts (
  id TEXT PRIMARY KEY,
  fact_a_id TEXT NOT NULL,
  fact_b_id TEXT NOT NULL,
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL DEFAULT '',

  conflict_kind TEXT NOT NULL,
  -- contradiction / overlap / outdated / scope_mismatch
  similarity REAL NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'open',
  -- open / resolved / ignored / superseded

  detected_by TEXT NOT NULL DEFAULT '',
  -- consolidator / runtime / user
  resolution TEXT NOT NULL DEFAULT '',
  -- keep_a / keep_b / merge / mark_disputed / split_scope
  resolved_by TEXT NOT NULL DEFAULT '',
  resolved_at TEXT NOT NULL DEFAULT '',

  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(fact_a_id, fact_b_id)
);

CREATE INDEX IF NOT EXISTS idx_memory_fact_conflicts_status
  ON memory_fact_conflicts(status, created_at);
```

### 3.6 新增表：`memory_fact_index`

向量索引元表（与 `14 ...md` §3.3 风格一致；BLOB 存 float32）。

```sql
CREATE TABLE IF NOT EXISTS memory_fact_index (
  fact_id TEXT PRIMARY KEY,
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL DEFAULT '',
  embedding_model TEXT NOT NULL DEFAULT '',
  embedding_dim INTEGER NOT NULL DEFAULT 0,
  embedding_blob BLOB,
  embedding_norm REAL NOT NULL DEFAULT 0,
  importance REAL NOT NULL DEFAULT 0.5,
  confidence REAL NOT NULL DEFAULT 0.7,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memory_fact_index_scope
  ON memory_fact_index(scope_type, scope_id);
```

### 3.7 扩展 `agent_runtime_settings`

```sql
ALTER TABLE agent_runtime_settings ADD COLUMN l3_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agent_runtime_settings ADD COLUMN l3_recall_top_k INTEGER NOT NULL DEFAULT 5;
ALTER TABLE agent_runtime_settings ADD COLUMN l3_recall_min_score REAL NOT NULL DEFAULT 0.55;
ALTER TABLE agent_runtime_settings ADD COLUMN l3_recall_scopes_json TEXT NOT NULL DEFAULT '["agent","user","team","workspace"]';
ALTER TABLE agent_runtime_settings ADD COLUMN l3_embedding_model TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_runtime_settings ADD COLUMN l3_decay_interval_hours INTEGER NOT NULL DEFAULT 24;
ALTER TABLE agent_runtime_settings ADD COLUMN l3_archive_threshold REAL NOT NULL DEFAULT 0.2;
ALTER TABLE agent_runtime_settings ADD COLUMN l3_max_per_recall_chars INTEGER NOT NULL DEFAULT 1500;
```

> 现有 `memory_max_chunk_length` / `memory_max_results` / `memory_min_score` 字段视为 L3 的兼容别名，新字段优先。

---

## 4. Go 域模型与 Repository 接口

### 4.1 域模型 `internal/domain/memory_l3.go`

```go
package domain

type ScopeType string

const (
	ScopeGlobal    ScopeType = "global"
	ScopeWorkspace ScopeType = "workspace"
	ScopeUser      ScopeType = "user"
	ScopeTeam      ScopeType = "team"
	ScopeAgent     ScopeType = "agent"
)

type FactKind string

const (
	FactPreference FactKind = "preference"
	FactRule       FactKind = "rule"
	FactPattern    FactKind = "pattern"
	FactPitfall    FactKind = "pitfall"
	FactGlossary   FactKind = "glossary"
	FactGeneric    FactKind = "fact"
)

type MemoryFact struct {
	ID                    string
	ScopeType             ScopeType
	ScopeID               string
	WorkspaceID           string
	UserID                string
	TeamID                string
	AgentID               string

	Statement             string
	StatementNormalized   string
	Fingerprint           string
	DetailsMarkdown       string
	Kind                  FactKind
	Tags                  []string

	Confidence            float64
	Importance            float64
	UseCount              int
	HitCount              int
	PositiveFeedbackCount int
	NegativeFeedbackCount int
	ConflictCount         int

	SourceKind            string
	SourceEpisodeID       string
	SourceSessionID       string
	SourceMessageID       string
	SourceExternal        string

	Version               int
	Status                string
	SupersededBy          string

	EmbeddingStatus       string
	EmbeddingModel        string
	EmbeddingDim          int
	EmbeddingBlob         []byte
	EmbeddingNorm         float64

	PIIFlag               bool
	RedactedStatement     string

	TTLDays               int
	DecayFactor           float64
	NextDecayAt           string
	LastUsedAt            string
	ExpiresAt             string

	Metadata              map[string]any
	CreatedAt             string
	UpdatedAt             string
	ArchivedAt            string
	DeletedAt             string
}

type FactUpsertInput struct {
	ScopeType       ScopeType
	ScopeID         string
	WorkspaceID     string
	UserID          string
	TeamID          string
	AgentID         string
	Statement       string
	DetailsMarkdown string
	Kind            FactKind
	Tags            []string
	Confidence      float64
	Importance      float64
	SourceKind      string
	SourceEpisodeID string
	SourceSessionID string
	SourceMessageID string
	TTLDays         int
	Metadata        map[string]any
}

type FactRecallQuery struct {
	WorkspaceID     string
	UserID          string
	TeamID          string
	AgentID         string
	IncludeScopes   []ScopeType
	Query           string
	QueryEmbedding  []float32
	Tags            []string
	Kinds           []FactKind
	TopK            int
	MinScore        float64
	MaxChars        int
}

type FactRecallHit struct {
	Fact         MemoryFact
	VectorScore  float64
	BM25Score    float64
	FinalScore   float64
	ScopeWeight  float64
	Reason       string
}

type FactFeedback struct {
	FactID       string
	SessionID    string
	AgentID      string
	Type         string
	Source       string
	Weight       float64
	Comment      string
	Metadata     map[string]any
}

type FactConflict struct {
	ID         string
	FactAID    string
	FactBID    string
	ScopeType  ScopeType
	ScopeID    string
	Kind       string
	Similarity float64
	Status     string
	DetectedBy string
	Resolution string
	ResolvedBy string
	ResolvedAt string
}
```

### 4.2 Repository 接口 `internal/repository/memory_l3.go`

```go
type MemoryL3Repository interface {
	// CRUD
	UpsertFact(ctx context.Context, f domain.MemoryFact, version domain.MemoryFact) error
	GetFact(ctx context.Context, id string) (domain.MemoryFact, error)
	GetByFingerprint(ctx context.Context, scopeType domain.ScopeType, scopeID, fp string) (domain.MemoryFact, error)
	ListFacts(ctx context.Context, q FactListQuery) ([]domain.MemoryFact, int, error)
	UpdateConfidence(ctx context.Context, id string, delta float64, reason string) error
	UpdateStatus(ctx context.Context, id, status, supersededBy string) error
	BumpUseStat(ctx context.Context, id string, hit bool, atISO string) error

	// Versions
	InsertVersion(ctx context.Context, fv FactVersion) error
	ListVersions(ctx context.Context, factID string, limit int) ([]FactVersion, error)
	RollbackToVersion(ctx context.Context, factID string, version int, by string) (domain.MemoryFact, error)

	// Feedback
	InsertFeedback(ctx context.Context, fb domain.FactFeedback) error
	ListFeedback(ctx context.Context, factID string, limit int) ([]domain.FactFeedback, error)

	// Conflicts
	UpsertConflict(ctx context.Context, c domain.FactConflict) error
	ListOpenConflicts(ctx context.Context, scope domain.ScopeType, scopeID string, limit int) ([]domain.FactConflict, error)
	UpdateConflictResolution(ctx context.Context, id, status, resolution, by string) error

	// Index
	UpsertEmbedding(ctx context.Context, id string, model string, dim int, blob []byte, norm float64) error
	SearchVector(ctx context.Context, scopes []domain.ScopeType, scopeIDs []string, q []float32, topK int) ([]domain.FactRecallHit, error)
	SearchBM25(ctx context.Context, scopes []domain.ScopeType, scopeIDs []string, query string, topK int) ([]domain.FactRecallHit, error)

	// Decay
	ListDueForDecay(ctx context.Context, before string, limit int) ([]domain.MemoryFact, error)
	ApplyDecay(ctx context.Context, factID string, factor float64, nextAt string) error
	ArchiveBelowConfidence(ctx context.Context, threshold float64, limit int) (int, error)
}

type FactListQuery struct {
	ScopeType ScopeType
	ScopeID   string
	Status    string
	Kind      FactKind
	Tags      []string
	Keyword   string
	Limit     int
	Offset    int
}

type FactVersion struct {
	ID            string
	FactID        string
	Version       int
	Statement     string
	Details       string
	Tags          []string
	Confidence    float64
	Status        string
	ChangedBy     string
	ChangeReason  string
	DiffJSON      string
	CreatedAt     string
}
```

---

## 5. Service 层接口

### 5.1 `MemoryL3Service`

```go
type MemoryL3Service interface {
	// 写入
	UpsertFact(ctx context.Context, in domain.FactUpsertInput) (domain.MemoryFact, error)
	BulkUpsert(ctx context.Context, ins []domain.FactUpsertInput) (BulkUpsertReport, error)
	UpdateFact(ctx context.Context, id string, patch FactPatch) (domain.MemoryFact, error)
	DeleteFact(ctx context.Context, id string, by string) error
	RollbackFact(ctx context.Context, id string, toVersion int, by string) (domain.MemoryFact, error)

	// 读取与检索
	Get(ctx context.Context, id string) (domain.MemoryFact, error)
	List(ctx context.Context, q FactListQuery) (FactListResult, error)
	Recall(ctx context.Context, q domain.FactRecallQuery) ([]domain.FactRecallHit, error)

	// 反馈
	Feedback(ctx context.Context, fb domain.FactFeedback) error

	// 冲突
	DetectConflicts(ctx context.Context, factID string) ([]domain.FactConflict, error)
	ResolveConflict(ctx context.Context, conflictID, resolution, by string) error
	ListOpenConflicts(ctx context.Context, scope domain.ScopeType, scopeID string) ([]domain.FactConflict, error)

	// 异步任务
	BuildEmbedding(ctx context.Context, factID string) error
	RunDecayBatch(ctx context.Context) (DecayReport, error)

	// L0 渲染
	RenderForPrompt(ctx context.Context, hits []domain.FactRecallHit, maxChars int) (PromptBlock, error)
}

type FactPatch struct {
	Statement       *string
	DetailsMarkdown *string
	Tags            *[]string
	Kind            *domain.FactKind
	Confidence      *float64
	Importance      *float64
	Status          *string
	TTLDays         *int
	By              string
	Reason          string
}

type FactListResult struct {
	Items  []domain.MemoryFact `json:"items"`
	Total  int                 `json:"total"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
}

type BulkUpsertReport struct {
	Created    int
	Updated    int
	Duplicated int
	Errors     int
	Conflicts  int
}

type DecayReport struct {
	Processed       int
	Archived        int
	ConfidenceDrop  float64
}

type PromptBlock struct {
	Section string // memory.l3
	Role    string // system
	Tokens  int
	Content string
	Items   []domain.FactRecallHit
}
```

### 5.2 UpsertFact 流程

```text
1. 校验 scope 与 ACL：
   - agent scope 仅该 agent / admin 可写
   - team scope 需 team member
   - workspace scope 需 workspace 写权限
   - global scope 仅平台 admin
2. PII 检测：扫描 statement + details；命中则强制 scope=user 或 scope=agent，并写 redacted_statement
3. 标准化：lower / strip punctuation / 折叠空白 → statement_normalized
4. 计算 fingerprint = sha256(scope_type:scope_id:statement_normalized)
5. GetByFingerprint：
   - 命中 → 视为 update：bumping confidence (+0.05), version+1, change_reason=update
   - 未命中：
     5a. 用 query embedding 检索同 scope 内 ≥ 0.92 相似度的 fact
     5b. 若命中 → 视为 update（merge details + tags 并集）
     5c. 否则 → 新建
6. 事务：UpsertFact + InsertVersion(diff)
7. 异步：BuildEmbedding（如未生成）
8. 异步：DetectConflicts（与同 scope 高相似但 ≠ 上面 5a 的 fact 比较矛盾）
9. 写 audit_logs
```

### 5.3 Recall 流程

```text
1. 校验 agent.l3_enabled
2. 解析 scopes（默认 [agent,user,team,workspace,global]，按 agent 设置过滤）
3. 若 q.QueryEmbedding 为空 → 调 ProviderService.Embed(query)
4. SearchVector + SearchBM25 并行；BM25 用作召回兜底（embedding 失败时的备份）
5. 每 hit 计算 final_score：
   final = 0.65 * vector_sim
         + 0.15 * bm25_norm
         + 0.10 * confidence
         + 0.05 * recency_boost
         + 0.05 * scope_weight
   scope_weight: agent=1.0, user=0.95, team=0.9, workspace=0.85, global=0.8
6. 过滤 final_score < q.MinScore
7. 截断 top_k；按 max_chars 二次裁剪
8. 异步：每 hit BumpUseStat(hit=true)
9. 返回（含命中字段、来源、置信度）
```

### 5.4 Feedback 流程

```text
1. InsertFeedback
2. 调整 confidence:
   confirm:    +0.10 * weight，capped at 1.0
   reject:     -0.20 * weight，floor at 0.0
   refine:     no confidence change，但 importance + 0.05
   used:       +0.02
   not_used:   -0.01
3. 累加 positive_feedback_count / negative_feedback_count
4. 若 confidence < archive_threshold：UpdateStatus(archived)
5. 若连续 3 次 reject → 自动 InsertConflict(kind=contradiction, status=open)
6. 写 InsertVersion（reason=feedback）
7. 写 audit_logs
```

### 5.5 衰减 Job

```text
loop every l3_decay_interval_hours:
  facts = repo.ListDueForDecay(now, batch=200)
  for f in facts:
    days_since_use = (now - f.last_used_at) / 1 day
    decay_factor = base_factor (e.g. 0.98) ^ days_since_use
    new_conf = f.confidence * decay_factor
    if new_conf < archive_threshold:
       repo.UpdateStatus(f.id, archived, '')
    else:
       repo.UpdateConfidence(...) + InsertVersion(reason=decay)
       next_at = now + l3_decay_interval_hours
       repo.ApplyDecay(f.id, decay_factor, next_at)
  emit metrics
```

### 5.6 与 ChatService / TeamRuntime 集成

- L0 装配（`12 ...md` §5.2 step 5）调用 `Recall` → `RenderForPrompt`，返回的 PromptBlock 注入 `memory.l3` 段。
- ChatService 在每个 turn 结束后，对本 turn 引用的 fact 写一条 `feedback(used)`。
- 用户在 UI 显式 confirm/reject → 写 feedback。
- L2 Consolidation Worker 调用 `BulkUpsert`。
- `MemoryL4Service` 在抽取实体时可调用 `UpsertFact(kind=glossary)` 把核心实体定义入 L3。

### 5.7 ADK MemoryService 适配（推荐）

实现 ADK MemoryService 协议（HTTP / gRPC 形式），暴露：

```http
POST /api/v1/adk/memory/{app_name}/{user_id}/sessions:add
POST /api/v1/adk/memory/{app_name}/{user_id}/memories:search
```

将 `app_name` 映射为 `workspace_id`，`user_id` 映射 `user_id`，body 转换为 `FactUpsertInput` / `FactRecallQuery`。这样 ADK Python / Go SDK 直接以「另一个 MemoryService 实现」的方式接入。

---

## 6. HTTP API

### 6.1 配置 API

复用 `PATCH /api/v1/agents/{id}/runtime-settings` 增加：

```json
{
  "l3_enabled": true,
  "l3_recall_top_k": 5,
  "l3_recall_min_score": 0.55,
  "l3_recall_scopes": ["agent","user","team","workspace"],
  "l3_embedding_model": "text-embedding-3-small",
  "l3_decay_interval_hours": 24,
  "l3_archive_threshold": 0.2,
  "l3_max_per_recall_chars": 1500
}
```

### 6.2 Fact CRUD

```http
GET    /api/v1/memory/l3/facts?scope_type=user&scope_id=u_xxx&kind=preference&limit=20&offset=0
POST   /api/v1/memory/l3/facts
GET    /api/v1/memory/l3/facts/{id}
PATCH  /api/v1/memory/l3/facts/{id}
DELETE /api/v1/memory/l3/facts/{id}
POST   /api/v1/memory/l3/facts/{id}/rollback {"to_version": 3}
GET    /api/v1/memory/l3/facts/{id}/versions
GET    /api/v1/memory/l3/facts/{id}/feedback
POST   /api/v1/memory/l3/facts:bulk-upsert
```

### 6.3 Recall

```http
POST /api/v1/memory/l3/recall
{
  "agent_id": "agent_xxx",
  "user_id": "u_xxx",
  "team_id": "team_xxx",
  "workspace_id": "ws_xxx",
  "include_scopes": ["agent","user","team","workspace"],
  "query": "是否使用 React 而不是 Vue？",
  "tags": ["frontend"],
  "kinds": ["preference","rule"],
  "top_k": 5,
  "min_score": 0.55,
  "max_chars": 1500
}
```

### 6.4 Feedback

```http
POST /api/v1/memory/l3/facts/{id}/feedback
{
  "session_id": "sess_xxx",
  "agent_id": "agent_xxx",
  "type": "confirm",
  "source": "user:u_xxx",
  "weight": 1.0,
  "comment": "保留这个偏好"
}
```

### 6.5 冲突管理

```http
GET   /api/v1/memory/l3/conflicts?scope_type=user&scope_id=u_xxx&status=open
POST  /api/v1/memory/l3/conflicts/{id}/resolve {"resolution":"keep_a","by":"u_xxx"}
```

### 6.6 管理员

```http
POST /api/v1/admin/memory/l3/decay/run
POST /api/v1/admin/memory/l3/embedding/rebuild
GET  /api/v1/admin/memory/l3/stats
```

---

## 7. 与现有 aranea 模块对接

| 模块 | 改造点 |
|------|--------|
| `internal/repository/sqlite.go` | `Migrate()` 增加 §3.2~§3.6 表；ALTER §3.7；`memory_items` 标记 deprecated |
| `internal/repository/sqlite_memory_l3.go`（新） | 实现 `MemoryL3Repository` |
| `internal/service/memory_l3_service.go`（新） | 实现 `MemoryL3Service` |
| `internal/service/embedding_service.go`（新） | 包装 ProviderService.Embed，支持多 model |
| `internal/service/pii_filter.go`（新） | 简单正则 + 可插拔 NER；命中即 redacted |
| `internal/service/memory_l0_service.go` | `Assemble` 中调 Recall + RenderForPrompt 注入 memory.l3 段 |
| `internal/service/memory_l2_service.go` | Consolidation worker 调 `BulkUpsert` |
| `internal/transport/memory_l3.go`（新） | 暴露 §6.2~§6.6 |
| `internal/transport/adk_memory.go`（新） | §5.7 ADK MemoryService 兼容层 |
| `cmd/server/main.go` | 启动衰减 worker + embedding worker |

---

## 8. 前端展示需求（Quasar / Vue）

### 8.1 Agent 设置 → 记忆 Tab → L3 子区

| 控件 | 字段 | 类型 |
|------|------|------|
| 启用 L3 | `l3_enabled` | `QToggle` |
| 检索 TopK | `l3_recall_top_k` | `QInput` 1-20 |
| 最小分数 | `l3_recall_min_score` | `QSlider` 0.3-0.95 |
| 检索作用域 | `l3_recall_scopes` | `QOptionGroup` checkbox 多选 agent/user/team/workspace/global |
| Embedding 模型 | `l3_embedding_model` | `QSelect` 来自 LLM provider models |
| 衰减间隔小时 | `l3_decay_interval_hours` | `QInput` 1-168 |
| 归档阈值 | `l3_archive_threshold` | `QSlider` 0.05-0.5 |
| Recall 最大字符 | `l3_max_per_recall_chars` | `QInput` 200-5000 |

### 8.2 知识库管理页 `/memory/l3`

| 区域 | 内容 |
|------|------|
| 顶栏 | 作用域 chip 切换：global/workspace/user/team/agent；右侧「新增 Fact」 |
| 统计卡 | 总数、活跃数、已归档、近 7 日命中率、平均 confidence、冲突数 |
| 筛选 | kind / tags / status / 关键字 / min_confidence / 时间范围 |
| 列表 | `QTable`：statement preview / kind chip / tags / confidence 进度条 / use_count / hit_count / 来源 / 最近使用 |
| 详情抽屉 | statement 全文、details markdown、tags、source、版本时间线、feedback 列表、相关冲突 |
| 操作 | 编辑 / 回滚 / 删除 / 标记归档 / 标记 disputed / 复制 statement |

### 8.3 Recall 调试器 `/admin/memory/l3/recall-tester`

| 区域 | 内容 |
|------|------|
| 输入 | query 输入框、scope 多选、top_k、min_score、tags、kinds |
| 结果 | 表格列：statement preview / vector_score / bm25_score / final_score / scope / confidence；可调权重 |
| 模拟注入 | 显示「将注入 prompt」的最终 markdown 文本 + token 计数 |

### 8.4 Session 详情 → 记忆使用 Tab

| 区域 | 内容 |
|------|------|
| 本会话引用的 facts | 列表 + 引用次数 + 用户反馈状态 |
| 写入的新 facts | 来自巩固管道的新 fact 列表（episode -> fact） |
| 反馈快捷按钮 | confirm / reject / refine 一键操作 |

### 8.5 冲突待办 `/memory/l3/conflicts`

| 区域 | 内容 |
|------|------|
| 列表 | conflict_kind chip + fact_a vs fact_b 摘要 + similarity + 时间 |
| 仲裁 | 「保留 A」「保留 B」「合并」「标记为存疑」「拆分作用域」 |

---

## 9. 写入与读取策略

| 场景 | 行为 |
|------|------|
| Episode 巩固 | `BulkUpsert`，去重 + 冲突检测 |
| 用户手动添加 | 通过 §6.2 POST；走 PII 检测 |
| Agent / Skill 工具写入 | 调 `memory_l3.write` 工具，权限受 `tool.requires_confirmation` 控制 |
| LLM 在 prompt 中的「记住」指令 | Plugin 监听 → 触发 `BulkUpsert` |
| Recall | L0 装配阶段；返回 ≤ K 条；max_chars 二次裁剪 |
| Feedback | 用户 UI / Critic 自动 / runtime「未引用」信号 |
| 衰减 | 周期 Job（默认 24h） |
| 归档 | confidence < threshold 自动 archived |
| 删除 | 软删除；硬删除仅合规；保留 audit |
| 冲突 | 发生时 status=open；UI 提示用户或团队管理员 |

---

## 10. 观测与治理

- **Datadog 指标**：
  - `aranea.memory.l3.facts_total{scope_type, status}`
  - `aranea.memory.l3.upsert_total{source_kind}`
  - `aranea.memory.l3.recall_latency_ms` P50/P95/P99
  - `aranea.memory.l3.recall_recall_at_k`（与人工标注对比）
  - `aranea.memory.l3.feedback_total{type}`
  - `aranea.memory.l3.conflict_open_total`
  - `aranea.memory.l3.decay_archived_total`
- **Trace**：每次 Recall / UpsertFact 都打 span；标注 final_score、scope_weight。
- **Audit**：所有 fact 写入、状态变更、冲突仲裁均写 `audit_logs`。
- **隐私**：PII 命中时强制降级 scope；redacted_statement 用于审计。

---

## 11. 落地实施阶段

### Phase 1（基础事实库 + Recall，2 周）

- [ ] §3.2 ~ §3.6 表落库；§3.7 ALTER。
- [ ] `MemoryL3Service.{UpsertFact, Get, List, Update, Delete, RollbackFact}`。
- [ ] EmbeddingService 包装 + 异步 worker。
- [ ] `Recall`（vector + BM25 融合）。
- [ ] `MemoryL0Service.Assemble` 接入 `memory.l3` 段。
- [ ] PIIFilter 简版（正则 + 可配置规则）。
- [ ] §6.2、§6.3 接口。
- [ ] §8.2 知识库管理页。

### Phase 2（反馈 + 衰减 + 冲突，1～2 周）

- [ ] `Feedback` API + 自动 confidence 调整。
- [ ] `RunDecayBatch` worker。
- [ ] `DetectConflicts` + UI 仲裁。
- [ ] §8.5 冲突待办。
- [ ] §8.4 Session 记忆使用 Tab。

### Phase 3（巩固管道联调，依赖 L2 Phase 3）

- [ ] L2 Consolidation Worker 调 `BulkUpsert`。
- [ ] 与 L1 字段升档联调（L1 可标 `升档候选` 字段）。
- [ ] §8.3 Recall 调试器。

### Phase 4（ADK 兼容 + 高级）

- [ ] §5.7 ADK MemoryService HTTP 兼容层。
- [ ] 多 embedding 模型并存（迁移时双写）。
- [ ] 向量索引迁移到 pgvector / Milvus（与 `14 ...md` §15 一致）。
- [ ] PII NER 替换。

---

## 12. 验收标准

- [ ] 成功创建一条 fact，`memory_facts` 写入；同时 `memory_fact_versions` 出现 v1。
- [ ] 重复 statement（同 scope）再次 upsert 时，version 自增并合并 tags。
- [ ] 调 Recall 返回 ≤ top_k，按 final_score 倒序，max_chars 内。
- [ ] L0 prompt 中出现 `memory.l3` 段，内容来自 Recall 返回。
- [ ] 用户 confirm 后 confidence + 0.10；reject 后 - 0.20。
- [ ] 连续 3 次 reject 后自动产生 conflict 记录。
- [ ] 衰减 Job 运行后，长时间未使用的 fact confidence 下降；< archive_threshold 转为 archived。
- [ ] PII 命中（如手机号）的 fact 强制 scope=user 或 agent，并显示 `[REDACTED]`。
- [ ] 高敏感 scope=user 的 fact 不会出现在 scope=workspace 检索结果。
- [ ] 通过 `POST /api/v1/adk/memory/.../memories:search` 也能拿到结果，结构与 ADK 兼容。
- [ ] 关闭 `l3_enabled` 后，下一次 prompt 中不出现 memory.l3 段。
- [ ] Session 记忆使用 Tab 能展示「本会话引用的 facts」与「本会话产生的 facts」。

---

## 13. 关键设计原则

1. **Scope 是访问控制的第一原则**：写入与检索都先按 scope 过滤，再融合。
2. **指纹去重，embedding 复合判重**：先精确，再语义。
3. **版本化是审计与回滚的基础**：所有写入产生 version，禁止覆盖。
4. **反馈即信号，不是另一份数据**：所有 feedback 直接驱动 confidence；不再额外做「评分系统」。
5. **冲突暴露而非自动消除**：自动检测，UI 仲裁；除非 user/team 显式允许 auto-merge。
6. **Recall 输出可解释**：每个 hit 给出 vector / bm25 / scope 分项，便于排查「记错」与「未召回」。
7. **PII 走 redacted 不走 hard delete**：保留链路便于审计与机器学习反馈。
8. **L3 是 L4 的素材，不是终点**：高频共现的 fact 会被 L4 抽象为实体-关系。
