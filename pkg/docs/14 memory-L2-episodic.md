# 14 L2 会话记忆 / 事件记忆（Episodic Memory）

本文档落地 5 层记忆架构中的 **L2：会话 / 事件记忆**。L2 记录当前会话内**所有交互历史**：消息流、工具/Skill/MCP 调用、决策依据、错误与修复、子任务完成情况、L1 任务的归档快照。这是 LLM Agent 的「情景记忆」（episodic memory），回答的问题是：**这次会话期间发生了什么？为什么这样决策？哪一步失败？**

aranea 当前已有 `messages`、`session_runs`（计划中）、`team_runs` / `team_run_steps`、`session_trace_spans`（计划中）、`monitor_events`、`monitor_traces`、`session_summaries` 等结构，本文档**不重新造轮子**，而是把它们组织成一套统一的 L2 视图，并补齐两个关键能力：

1. **`memory_episodes`**：从 L1 归档而来的「任务级 episode」，是 L2 → L3 巩固的最小单元。
2. **`memory_l2_index`**：可选的 BM25 / 向量倒排索引，让 L2 可被 LLM 在「memory_recall」时候选。

> 关联文档：`memery.md` §1～§6、`10 session.md`、`11 multi-agent.md`、`12 memory-L0-sensory.md`、`13 memory-L1-working.md`、`18 monitor.md`。

---

## 1. 心智模型与边界

### 1.1 L2 在 5 层中的位置

| 维度 | 描述 |
|------|------|
| 容量 | 当前会话周期全量记录，不进入 prompt（仅按需检索） |
| 持久性 | 会话期内（数小时～数天）；归档/删除策略由 retention job 决定 |
| 访问模式 | 时序过滤 + 关键字 / BM25 / 向量检索；按 turn / span / agent / event_type 过滤 |
| 与 ADK 对齐 | 对应 `Runner` 自动写入的 `events` + `Artifact` 持久化 |
| 与 Aranea 现状对齐 | 复用 `messages` / `team_runs` / `team_run_steps` / `monitor_events` / `monitor_traces` / `model_token_usage_events` |

### 1.2 与其它层的边界

| 边界 | 走向 | 说明 |
|------|------|------|
| L0 → L2 | L0 装配后写一条 `memory_recall` span（L0 文档定义） |
| L1 → L2 | task 结束时 SnapshotForEpisode → 写入 `memory_episodes` |
| L2 → L3 | 巩固 Job：从 episode 抽取「事实/偏好/规则」候选，去重后入 L3 |
| L2 → L4 | 巩固 Job：从 episode 抽取「实体-关系」候选，进入 L4 知识图谱 |
| L2 → L0（recall） | 通过 `memory_l2_index` 按当前查询取相关 episode 摘要，注入 prompt（可选） |

### 1.3 非目标

- 不替代 `messages`：L2 是消息+工具调用+task episode 的**统一视图**，事实仍在原表。
- 不做长期跨会话事实存储（属于 L3）。
- 不做实体关系图谱（属于 L4）。
- 不做实时全文搜索（仅按需异步建索引）。

---

## 2. 需求清单

### 2.1 功能需求

| # | 需求 | 必要性 |
|---|------|--------|
| F1 | 自动记录每条 message / 工具调用 / Skill 调用 / MCP 调用为 L2 事件，无需 Agent 显式操作 | 必须（已部分实现） |
| F2 | 提供「按 session 时间线」浏览所有事件，支持按类型 / 状态 / actor 过滤 | 必须 |
| F3 | 提供「按 turn 浏览」聚合视图：用户输入 → 模型推理 → 工具/技能/MCP → AI 回复 | 必须（依赖 §10 session.md `session_turns`） |
| F4 | 任务级 episode：L1 任务结束时写入 `memory_episodes`，承载 task_goal / 主要决策 / 结果摘要 | 必须 |
| F5 | 高价值事件标记：用户标星、Critic 评分高、Skill 完成度高的事件可被显式标记为「巩固候选」 | 必须 |
| F6 | 关键字 / BM25 检索：在单 session 内按关键字快速定位事件 | 必须 |
| F7 | 可选向量索引：episode 摘要 embedding，用于 L2 → L3 巩固管道与 L0 recall | 推荐 |
| F8 | 检查点与续传：会话异常中断后可从最后一个稳定 turn 继续 | 必须（依赖现有 trace_spans） |
| F9 | 归档/Retention：按时间 + 容量自动归档；删除时保留 audit 链路 | 必须 |
| F10 | 多 Agent 编排时按 actor 区分事件，避免「coordinator 输出被算到 worker 头上」 | 必须 |

### 2.2 非功能需求

| # | 需求 | 目标值 |
|---|------|--------|
| N1 | 写入吞吐 | 每秒 ≥ 200 事件 |
| N2 | 关键字检索 P99 延迟 | < 200 ms（单 session 范围） |
| N3 | 巩固 Job 处理量 | ≥ 1000 事件 / min |
| N4 | 索引滞后 | 事件落库后 ≤ 30 s 可被检索到 |
| N5 | 归档安全 | 归档不丢失任何 audit / cost 关联 |

---

## 3. 数据模型

L2 由 4 类既有事实表 + 2 张新表组成。

### 3.1 复用既有事实表

| 表 | L2 角色 | 关键字段 |
|----|--------|---------|
| `messages` | 用户/Assistant/Tool/Agent 消息事实源 | `session_id, turn_index, role, content_markdown, status` |
| `session_runs`（`10 session.md` §4.3 规划） | 一次用户请求的编排 run | `session_id, run_type, status, started_at, ended_at` |
| `session_run_steps`（同上 §4.4 规划） | step 编排单元 | `step_type, actor_type, actor_id, status` |
| `session_turns`（同上 §4.5 规划） | 每轮对话聚合视图 | `turn_index, status, model_call_count, tool_call_count` |
| `session_trace_spans`（同上 §4.6 规划） | 调用链 span 事实表 | `trace_id, parent_span_id, span_type, status, duration_ms` |
| `team_runs` / `team_run_steps`（已有） | Team 编排事件 | `team_id, run.status, step.role, step.status` |
| `model_token_usage_events`（已有） | 模型调用 token / cost / latency | `provider_code, model_api_id, total_tokens, total_cost_micro_usd` |
| `tool_invocations` / `tool_invocation_params`（已有） | 工具调用事实 | `tool_id, status, duration_ms` |
| `skill_invocation`（已有） | Skill 调用事实 | `skill_id, status` |
| `monitor_events`（已有） | 已持久化的运行时事件 | `event_key, severity, payload_json` |
| `monitor_traces`（已有） | 高级 trace 资源 | `trace_key, payload_json` |
| `audit_logs`（已有） | 管理面审计 | `action, resource, resource_id` |
| `session_summaries`（已有） | 上下文压缩摘要 | `from_turn, to_turn, summary_markdown` |

> 「事件」在 L2 是逻辑概念，物理上散布在以上多表中，由 `MemoryL2Service` 提供统一查询视图。

### 3.2 新增表：`memory_episodes`

任务级（L1 任务终止时）或里程碑级（用户显式 `/save-episode` 命令、Critic 评分通过）的高凝练单元。

```sql
CREATE TABLE IF NOT EXISTS memory_episodes (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  run_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  l1_task_id TEXT NOT NULL DEFAULT '',

  episode_kind TEXT NOT NULL DEFAULT 'task',
  -- task / milestone / failure_postmortem / user_marked / critic_pass
  title TEXT NOT NULL,
  goal TEXT NOT NULL DEFAULT '',
  outcome TEXT NOT NULL DEFAULT '',
  -- success / partial / failed / cancelled
  outcome_summary TEXT NOT NULL DEFAULT '',
  result_preview TEXT NOT NULL DEFAULT '',
  failure_reason TEXT NOT NULL DEFAULT '',

  importance REAL NOT NULL DEFAULT 0.5,
  -- 0~1，决定是否进入 L3 巩固队列
  confidence REAL NOT NULL DEFAULT 0.7,
  user_feedback TEXT NOT NULL DEFAULT '',
  -- positive / neutral / negative / unrated
  critic_score REAL NOT NULL DEFAULT -1,
  -- -1 表示无 Critic 评分

  span_count INTEGER NOT NULL DEFAULT 0,
  message_count INTEGER NOT NULL DEFAULT 0,
  tool_call_count INTEGER NOT NULL DEFAULT 0,
  skill_call_count INTEGER NOT NULL DEFAULT 0,
  mcp_call_count INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  duration_ms INTEGER NOT NULL DEFAULT 0,

  l1_snapshot_json TEXT NOT NULL DEFAULT '{}',
  -- L1 字段树整段快照（参考 13 文档 L1Episode）
  key_decisions_json TEXT NOT NULL DEFAULT '[]',
  -- [{decision, rationale, at, span_id}]
  key_artifacts_json TEXT NOT NULL DEFAULT '[]',
  -- [{kind, ref, preview}]：关联 messages / artifacts / chat_attachments

  embedding_status TEXT NOT NULL DEFAULT 'pending',
  -- pending / ready / failed / skipped
  embedding_model TEXT NOT NULL DEFAULT '',
  embedding_dim INTEGER NOT NULL DEFAULT 0,
  embedding_blob BLOB,
  embedding_norm REAL NOT NULL DEFAULT 0,

  consolidation_status TEXT NOT NULL DEFAULT 'pending',
  -- pending / processing / done / skipped / error
  consolidated_at TEXT NOT NULL DEFAULT '',
  consolidated_l3_count INTEGER NOT NULL DEFAULT 0,
  consolidated_l4_count INTEGER NOT NULL DEFAULT 0,

  metadata_json TEXT NOT NULL DEFAULT '{}',
  started_at TEXT NOT NULL DEFAULT '',
  ended_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  archived_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_memory_episodes_session
  ON memory_episodes(session_id, ended_at DESC);

CREATE INDEX IF NOT EXISTS idx_memory_episodes_agent
  ON memory_episodes(agent_id, importance DESC, ended_at DESC);

CREATE INDEX IF NOT EXISTS idx_memory_episodes_consolidation
  ON memory_episodes(consolidation_status, importance DESC, ended_at);

CREATE INDEX IF NOT EXISTS idx_memory_episodes_kind
  ON memory_episodes(episode_kind, ended_at DESC);
```

### 3.3 新增表：`memory_l2_index`

关键字 / BM25 / 向量倒排索引。SQLite 用 `FTS5` 虚拟表 + 向量字段；若用 PostgreSQL 可改 `pg_trgm` + `pgvector`。

```sql
-- FTS5 全文倒排
CREATE VIRTUAL TABLE IF NOT EXISTS memory_l2_index_fts
USING fts5(
  episode_id UNINDEXED,
  session_id UNINDEXED,
  agent_id UNINDEXED,
  text,
  tokenize = 'unicode61 remove_diacritics 2'
);

-- 元信息（可与 FTS 表分离便于维护）
CREATE TABLE IF NOT EXISTS memory_l2_index_meta (
  id TEXT PRIMARY KEY,
  episode_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  text_kind TEXT NOT NULL DEFAULT 'episode',
  -- episode / message / tool_call / decision
  text_preview TEXT NOT NULL DEFAULT '',
  token_estimate INTEGER NOT NULL DEFAULT 0,
  embedding_model TEXT NOT NULL DEFAULT '',
  embedding_dim INTEGER NOT NULL DEFAULT 0,
  embedding_blob BLOB,
  embedding_norm REAL NOT NULL DEFAULT 0,
  importance REAL NOT NULL DEFAULT 0.5,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(episode_id, text_kind)
);

CREATE INDEX IF NOT EXISTS idx_memory_l2_index_meta_episode
  ON memory_l2_index_meta(episode_id);

CREATE INDEX IF NOT EXISTS idx_memory_l2_index_meta_session_kind
  ON memory_l2_index_meta(session_id, text_kind);
```

> 向量字段在 SQLite 中以 BLOB 存储 float32 数组；检索时全表扫 + cosine 相似度（小规模 OK）。当数据量上升后改用外部向量库（Milvus / Qdrant / pgvector）。详见 §15.

### 3.4 新增表：`memory_event_marks`

让用户、Critic、Plugin 显式标记某些事件为「高价值」或「需要复盘」。

```sql
CREATE TABLE IF NOT EXISTS memory_event_marks (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  episode_id TEXT NOT NULL DEFAULT '',
  -- 二选一：mark 一个 episode 或单个事件
  ref_kind TEXT NOT NULL,
  -- message / span / step / turn / episode
  ref_id TEXT NOT NULL,

  mark_type TEXT NOT NULL,
  -- star / pin / consolidate / forget / postmortem / good_example / bad_example
  marked_by TEXT NOT NULL DEFAULT '',
  -- user:xxx / agent:xxx / plugin:xxx / critic
  reason TEXT NOT NULL DEFAULT '',
  weight REAL NOT NULL DEFAULT 1.0,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT '',
  UNIQUE(ref_kind, ref_id, mark_type, marked_by)
);

CREATE INDEX IF NOT EXISTS idx_memory_event_marks_session
  ON memory_event_marks(session_id, mark_type, created_at);
```

### 3.5 扩展 `agent_runtime_settings`

```sql
ALTER TABLE agent_runtime_settings ADD COLUMN l2_episode_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agent_runtime_settings ADD COLUMN l2_episode_min_importance REAL NOT NULL DEFAULT 0.3;
-- 低于该阈值不入巩固队列
ALTER TABLE agent_runtime_settings ADD COLUMN l2_index_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agent_runtime_settings ADD COLUMN l2_index_embedding_model TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_runtime_settings ADD COLUMN l2_recall_enabled INTEGER NOT NULL DEFAULT 0;
-- 是否在 L0 装配时回注 L2 episode
ALTER TABLE agent_runtime_settings ADD COLUMN l2_recall_max INTEGER NOT NULL DEFAULT 3;
ALTER TABLE agent_runtime_settings ADD COLUMN l2_retention_days INTEGER NOT NULL DEFAULT 90;
ALTER TABLE agent_runtime_settings ADD COLUMN l2_archive_after_days INTEGER NOT NULL DEFAULT 30;
```

---

## 4. Go 域模型与 Repository 接口

### 4.1 域模型 `internal/domain/memory_l2.go`

```go
package domain

type EpisodeKind string

const (
	EpisodeTask           EpisodeKind = "task"
	EpisodeMilestone      EpisodeKind = "milestone"
	EpisodeFailurePM      EpisodeKind = "failure_postmortem"
	EpisodeUserMarked     EpisodeKind = "user_marked"
	EpisodeCriticPass     EpisodeKind = "critic_pass"
)

type MemoryEpisode struct {
	ID                   string
	SessionID            string
	RunID                string
	TeamID               string
	AgentID              string
	L1TaskID             string
	Kind                 EpisodeKind
	Title                string
	Goal                 string
	Outcome              string
	OutcomeSummary       string
	ResultPreview        string
	FailureReason        string
	Importance           float64
	Confidence           float64
	UserFeedback         string
	CriticScore          float64
	SpanCount            int
	MessageCount         int
	ToolCallCount        int
	SkillCallCount       int
	MCPCallCount         int
	TotalTokens          int
	TotalCostMicroUSD    int64
	DurationMS           int
	L1SnapshotJSON       string
	KeyDecisionsJSON     string
	KeyArtifactsJSON     string
	EmbeddingStatus      string
	EmbeddingModel       string
	EmbeddingDim         int
	EmbeddingBlob        []byte
	EmbeddingNorm        float64
	ConsolidationStatus  string
	ConsolidatedAt       string
	ConsolidatedL3Count  int
	ConsolidatedL4Count  int
	StartedAt            string
	EndedAt              string
	Metadata             map[string]any
	CreatedAt            string
	UpdatedAt            string
	ArchivedAt           string
	DeletedAt            string
}

type MemoryL2Event struct {
	// 跨表事件统一视图（read-only）
	ID         string
	Kind       string // message / model_call / tool_call / skill_call / mcp_call / agent_handoff / summary / mark
	SessionID  string
	RunID      string
	TurnID     string
	SpanID     string
	ActorType  string
	ActorID    string
	ActorName  string
	Status     string
	Title      string
	Preview    string
	OccurredAt string
	DurationMS int
	TokensIn   int
	TokensOut  int
	CostMicro  int64
	RefTable   string
	RefID      string
	Metadata   map[string]any
}

type MemoryL2EventQuery struct {
	SessionID    string
	TurnID       string
	SpanID       string
	Kinds        []string
	ActorIDs     []string
	StatusIn     []string
	StartTimeUTC string
	EndTimeUTC   string
	Keyword      string
	Limit        int
	Offset       int
}

type MemoryL2RecallQuery struct {
	SessionID     string
	AgentID       string
	Query         string
	QueryEmbedding []float32
	MinImportance float64
	TopK          int
	IncludeKinds  []EpisodeKind
}

type MemoryL2RecallResult struct {
	Episode   MemoryEpisode
	BM25Score float64
	VectorSim float64
	FinalRank float64
}

type MemoryEventMark struct {
	ID        string
	SessionID string
	EpisodeID string
	RefKind   string
	RefID     string
	MarkType  string
	MarkedBy  string
	Reason    string
	Weight    float64
	CreatedAt string
}
```

### 4.2 Repository 接口 `internal/repository/memory_l2.go`

```go
type MemoryL2Repository interface {
	// Episodes
	CreateEpisode(ctx context.Context, e domain.MemoryEpisode) error
	UpdateEpisode(ctx context.Context, e domain.MemoryEpisode) error
	GetEpisode(ctx context.Context, id string) (domain.MemoryEpisode, error)
	ListEpisodes(ctx context.Context, sessionID string, limit, offset int) ([]domain.MemoryEpisode, int, error)
	ListPendingConsolidation(ctx context.Context, minImportance float64, limit int) ([]domain.MemoryEpisode, error)
	UpdateConsolidationStatus(ctx context.Context, id string, status string, l3Count, l4Count int) error
	UpdateEmbedding(ctx context.Context, id string, model string, dim int, blob []byte, norm float64) error

	// Indexes
	UpsertIndex(ctx context.Context, episodeID string, kind string, text string, embedding []float32, importance float64) error
	DeleteIndex(ctx context.Context, episodeID string) error
	SearchBM25(ctx context.Context, sessionID, query string, limit int) ([]domain.MemoryL2RecallResult, error)
	SearchVector(ctx context.Context, sessionID string, query []float32, limit int) ([]domain.MemoryL2RecallResult, error)

	// Marks
	UpsertMark(ctx context.Context, m domain.MemoryEventMark) error
	ListMarks(ctx context.Context, sessionID string, markType string, limit int) ([]domain.MemoryEventMark, error)

	// 事件视图（跨表 UNION ALL；带 LIMIT/OFFSET）
	ListEvents(ctx context.Context, q domain.MemoryL2EventQuery) ([]domain.MemoryL2Event, int, error)

	// Retention
	ArchiveBeforeDate(ctx context.Context, sessionID string, before string) (int, error)
	DeleteArchivedBefore(ctx context.Context, before string) (int, error)
}
```

`ListEvents` 实现以 SQL UNION ALL 把 `messages`、`session_trace_spans`、`team_run_steps`、`monitor_events` 拼成统一行（按需挑列），按 `occurred_at DESC` 排序；详细 SQL 在 §16。

---

## 5. Service 层接口

### 5.1 `MemoryL2Service`

```go
type MemoryL2Service interface {
	// 自动归档 L1 任务为 episode
	ArchiveL1Task(ctx context.Context, l1TaskID string) (domain.MemoryEpisode, error)
	// 用户/Critic/Plugin 显式创建 milestone episode
	CreateMilestoneEpisode(ctx context.Context, in CreateEpisodeInput) (domain.MemoryEpisode, error)

	// 事件视图
	ListEvents(ctx context.Context, q domain.MemoryL2EventQuery) (EventListResult, error)
	GetEventDetail(ctx context.Context, refKind, refID string) (EventDetail, error)

	// Episode 列表与详情
	ListEpisodes(ctx context.Context, sessionID string, limit, offset int) (EpisodeListResult, error)
	GetEpisode(ctx context.Context, id string) (EpisodeDetail, error)

	// Marks
	Mark(ctx context.Context, m domain.MemoryEventMark) error
	UnMark(ctx context.Context, id string) error
	ListMarks(ctx context.Context, sessionID, markType string, limit int) ([]domain.MemoryEventMark, error)

	// Recall（被 L0 调用 / 被 L3 巩固 Job 调用）
	RecallByQuery(ctx context.Context, q domain.MemoryL2RecallQuery) ([]domain.MemoryL2RecallResult, error)

	// 异步任务
	BuildIndexFor(ctx context.Context, episodeID string) error
	RunConsolidationBatch(ctx context.Context, batchSize int) (ConsolidationReport, error)

	// Retention
	ApplyRetention(ctx context.Context) (RetentionReport, error)
}

type CreateEpisodeInput struct {
	SessionID    string
	RunID        string
	TeamID       string
	AgentID      string
	Title        string
	Goal         string
	Outcome      string
	Importance   float64
	Kind         domain.EpisodeKind
	KeyDecisions []KeyDecision
	KeyArtifacts []KeyArtifact
	Metadata     map[string]any
}

type EpisodeListResult struct {
	Items  []domain.MemoryEpisode `json:"items"`
	Total  int                    `json:"total"`
	Limit  int                    `json:"limit"`
	Offset int                    `json:"offset"`
}

type EpisodeDetail struct {
	Episode    domain.MemoryEpisode
	Events     []domain.MemoryL2Event
	Marks      []domain.MemoryEventMark
	Summary    string // 来自 session_summaries 或 LLM 生成
}

type ConsolidationReport struct {
	ProcessedEpisodes int
	NewL3Facts        int
	NewL4Entities     int
	NewL4Relations    int
	Skipped           int
	Errors            int
}

type RetentionReport struct {
	ArchivedSessions int
	DeletedEvents    int
	DeletedSpans     int
}
```

### 5.2 `ArchiveL1Task` 流程

```text
1. 读 L1 task + 字段（MemoryL1Service.GetTask）
2. 聚合统计：从 session_run_steps / session_trace_spans / messages 计算 message_count / tool_call_count 等
3. 抽取 key_decisions：遍历 L1 task.key_decisions 字段 + Critic span 中评分 ≥ threshold 的 span
4. 抽取 key_artifacts：从 chat_attachments / messages.role=assistant 中按 user mark 或长度阈值挑选
5. 计算 importance：
     base = 0.3 + 0.2*has_critic_pass + 0.2*has_user_positive + 0.1*subtask_completion + 0.2*duration_factor
6. CreateEpisode（status=pending consolidation）
7. 异步触发 BuildIndexFor(episodeID)
```

### 5.3 `RecallByQuery` 流程

```text
1. 校验 agent 设置 l2_recall_enabled
2. 若 query.QueryEmbedding 为空且 l2_index_embedding_model 已设置：调 ProviderService 算 embedding
3. SearchBM25 + SearchVector 两路并行
4. 融合分数：final = 0.4 * bm25_norm + 0.5 * vector_sim + 0.1 * importance
5. 过滤 deleted/archived；按 final desc 取 TopK
6. 返回时 episode 仅返回 title/goal/outcome_summary/result_preview，避免 prompt 膨胀
```

### 5.4 与 ChatService / TeamRuntime 集成

| 事件 | L2 行为 |
|------|--------|
| 任意消息落库 | 不需要新动作，已由 `messages` 表承担 |
| L0 装配前 | 若 `l2_recall_enabled`：RecallByQuery → 注入 prompt（与 L3 段落分开） |
| L1 task EndTask | 异步队列 ArchiveL1Task → 写 episode |
| Run 结束 | 遍历未归档 L1 task 全部 ArchiveL1Task；若 run 失败，自动 mark `failure_postmortem` |
| 用户在 UI 标星 | `MemoryL2Service.Mark(mark_type=star)`；importance += 0.2（capped at 1.0） |
| Critic 评分 ≥ threshold | 自动 Mark(mark_type=consolidate)；importance += 0.15 |

### 5.5 巩固 Job（L2 → L3 / L4）

由后台 goroutine 周期调度（默认 5 分钟一次）。详见 §10。

```text
loop:
  episodes = repo.ListPendingConsolidation(min_importance=agent.l2_episode_min_importance, limit=batch_size)
  for each episode:
    facts   = LLM.ExtractFacts(episode.summary + l1_snapshot)
    entities, relations = LLM.ExtractGraph(episode)
    for f in facts: MemoryL3Service.UpsertFact(f, source=episode.id)
    for e,r in graph: MemoryL4Service.UpsertEntityRelation(e,r, source=episode.id)
    repo.UpdateConsolidationStatus(episode.id, status=done, l3_count, l4_count)
  emit metrics
```

---

## 6. HTTP API

### 6.1 配置 API

复用 `PATCH /api/v1/agents/{id}/runtime-settings`：

```json
{
  "l2_episode_enabled": true,
  "l2_episode_min_importance": 0.3,
  "l2_index_enabled": true,
  "l2_index_embedding_model": "text-embedding-3-small",
  "l2_recall_enabled": false,
  "l2_recall_max": 3,
  "l2_retention_days": 90,
  "l2_archive_after_days": 30
}
```

### 6.2 事件流 API

```http
GET  /api/v1/sessions/{sid}/l2/events?kinds=message,tool_call&actor_id=agent_xxx&keyword=login&limit=50&offset=0
GET  /api/v1/sessions/{sid}/l2/events/{ref_kind}/{ref_id}
```

返回 `MemoryL2Event` 列表 + total。

### 6.3 Episode

```http
GET    /api/v1/sessions/{sid}/l2/episodes?limit=20&offset=0&kind=task
GET    /api/v1/sessions/{sid}/l2/episodes/{episodeId}
POST   /api/v1/sessions/{sid}/l2/episodes                # 创建 milestone
PATCH  /api/v1/sessions/{sid}/l2/episodes/{episodeId}    # 修改 importance / outcome / metadata
DELETE /api/v1/sessions/{sid}/l2/episodes/{episodeId}
POST   /api/v1/sessions/{sid}/l2/episodes/{episodeId}/reindex
POST   /api/v1/sessions/{sid}/l2/episodes/{episodeId}/consolidate
```

### 6.4 Marks

```http
POST   /api/v1/sessions/{sid}/l2/marks
DELETE /api/v1/sessions/{sid}/l2/marks/{markId}
GET    /api/v1/sessions/{sid}/l2/marks?type=star&limit=50
```

请求示例（star 一条 message）：

```json
{
  "ref_kind": "message",
  "ref_id": "msg_xxx",
  "mark_type": "star",
  "reason": "好的回复模板",
  "weight": 1.0
}
```

### 6.5 Recall

```http
POST /api/v1/sessions/{sid}/l2/recall
{
  "query": "用户上次说要避免使用 jQuery",
  "top_k": 5,
  "min_importance": 0.3
}
```

### 6.6 巩固与 Retention（管理员）

```http
POST /api/v1/admin/memory/l2/consolidate?batch=50
POST /api/v1/admin/memory/l2/retention/run
GET  /api/v1/admin/memory/l2/consolidation/stats
```

---

## 7. 与现有 aranea 模块对接

| 模块 | 改造点 |
|------|--------|
| `internal/repository/sqlite.go` | `Migrate()` 中增加 §3.2、§3.3、§3.4 表；ALTER §3.5 |
| `internal/repository/sqlite_memory_l2.go`（新） | 实现 `MemoryL2Repository`，含跨表 UNION ALL 的 `ListEvents` |
| `internal/service/memory_l2_service.go`（新） | 实现 `MemoryL2Service` |
| `internal/service/team_runtime.go` | run 结束时调用 `ArchiveL1Task` 收尾 |
| `internal/service/chat_service.go` | turn 结束时聚合 L1 → 触发 ArchiveL1Task |
| `internal/service/memory_l0_service.go` | `Assemble` 步骤增加 l2_recall 段（独立于 l3/l4） |
| `internal/transport/sessions.go` | 暴露 §6.2~§6.5 |
| `internal/transport/admin.go`（如无则新增） | 暴露 §6.6 |
| `internal/runtime/adk_plugin_*.go` | Critic Plugin 在评分后写 `memory_event_marks(mark_type=consolidate)` |
| `cmd/server/main.go` | 启动时拉起两个 goroutine：consolidation worker + retention scheduler |

---

## 8. 前端展示需求（Quasar / Vue）

### 8.1 Session 详情 → 事件 Tab（统一时间线）

| 区域 | 内容 |
|------|------|
| 顶部 KPI | 消息数 / span 数 / tool 数 / skill 数 / mcp 数 / 总 token / 平均延迟 / 失败数 |
| 筛选栏 | `QSelect` Kinds 多选；`QSelect` Actor；`QInput` 关键字；`QDate` 起止；`QToggle` 仅失败 |
| 时间线 | `QTimeline` 或 `QVirtualScroll` + 自定义卡片：每条事件 [时间｜类型 chip｜actor｜title｜状态｜耗时] |
| 详情抽屉 | 点击打开 `QDrawer`，显示完整 ref 数据：message content / tool args+result / span tree |
| 标记 | 卡片右侧三点菜单：标星 / 加入巩固队列 / 标记需要复盘 / 标记好范例 / 标记坏范例 |

### 8.2 Session 详情 → Episode Tab

| 区域 | 内容 |
|------|------|
| 列表 | `QTable`：title / kind / outcome / importance（彩色 chip）/ duration / tokens / cost / consolidation_status |
| 详情 | 抽屉/页签：goal、outcome_summary、result_preview、key_decisions 列表、key_artifacts 卡片（可下载附件）、L1 snapshot 折叠 JSON、相关事件链路（链回 §8.1）|
| 操作 | 「重建索引」「立即巩固」「修改 importance」「删除」「标记类型」 |

### 8.3 Session 详情 → 标记 Tab

| 区域 | 内容 |
|------|------|
| 标签 | `QTabs`: 标星 / 巩固候选 / 复盘 / 好范例 / 坏范例 / 已忘记 |
| 列表 | 来源 (user/agent/critic) + ref 摘要 + 时间 + 删除/编辑 |

### 8.4 Trace 详情联动

`session_trace_spans` 详情抽屉新增「事件来源」标签：跳转到 §8.1 事件 Tab 并定位到对应 span。

### 8.5 Recall 调试器（仅开发者 / 管理员）

`/admin/memory/l2/recall-tester`：

- 输入 query + session_id + top_k；
- 调用 `POST /l2/recall`；
- 表格展示 BM25 / vector / final 分数，支持调整融合权重。

---

## 9. 写入与读取策略

| 场景 | 行为 |
|------|------|
| 写入事件 | 不直接写 L2，事件由各业务模块写各自表 |
| 写入 episode | L1 EndTask 后异步 ArchiveL1Task；user / critic / plugin 触发 milestone |
| 写入索引 | episode 创建后 ≤ 30s 内异步 BuildIndexFor |
| 读取事件流 | API §6.2，按 session 全表扫，分页 |
| 读取 episode | API §6.3，按 importance / time 排序 |
| Recall | API §6.5；融合 BM25 + vector；只返回摘要，不返回完整内容 |
| 删除 | 软删除：`deleted_at`；事件流过滤；保留 audit 链路 |
| 归档 | `ArchiveBeforeDate`：把 episode 从 active 池移到 archived（仅设置 archived_at） |

---

## 10. 巩固管道（Consolidation Worker）

| 步骤 | 详细 |
|------|------|
| 调度 | `cmd/server/main.go` 启 goroutine：`time.Tick(5 * time.Minute)` → `MemoryL2Service.RunConsolidationBatch(ctx, 50)` |
| 选择候选 | `repo.ListPendingConsolidation(min_importance, 50)` ORDER BY importance DESC, ended_at ASC |
| Fact 抽取 | 调用「Consolidator Agent」（系统级 Agent，配置在 `agents` 表中由系统创建）：input = episode.title+goal+outcome_summary+l1_snapshot；output 为 JSON `[{statement, scope, confidence, tags}]` |
| 去重 | 与 L3 既有 facts 比较：相似度 ≥ 0.92 → 视为重复，更新 last_used_at；< 0.92 → upsert |
| Graph 抽取 | input 同上；output `{entities:[{name,type}], relations:[{src,dst,type,confidence}]}` |
| 入库 | MemoryL3Service.UpsertFact + MemoryL4Service.UpsertEntityRelation |
| 状态更新 | `repo.UpdateConsolidationStatus(done, l3_count, l4_count)`；写 `audit_logs(action=l2.consolidate)` |
| 失败重试 | 单 episode 失败 ≤ 3 次；超出后 status=error，metadata.last_error 记录 |
| 指标 | `aranea.memory.l2.consolidation.processed_total`、`new_l3_facts_total`、`new_l4_entities_total`、`errors_total` |

---

## 11. 观测与治理

- **Datadog 指标**：
  - `aranea.memory.l2.events_listed_total{kind}`
  - `aranea.memory.l2.episode_created_total{kind, agent}`
  - `aranea.memory.l2.recall_latency_ms` P50/P95/P99
  - `aranea.memory.l2.recall_hit_ratio`（recall 后被 LLM 实际引用的比例）
  - `aranea.memory.l2.index_lag_seconds`（事件创建 → 索引可用）
- **Trace**：每次 RunConsolidationBatch 一个 root span，每条 episode 一个子 span。
- **Audit**：episode CRUD、mark CRUD、巩固运行均写 `audit_logs`。
- **隐私**：episode 摘要默认按 PII 规则脱敏；admin 才能看到原始 `l1_snapshot_json`。

---

## 12. 落地实施阶段

### Phase 1（episode 落库 + 事件流统一视图，1～2 周）

- [ ] §3.2、§3.4 表落库；§3.5 ALTER。
- [ ] `MemoryL2Service.{ArchiveL1Task, ListEvents, GetEpisode, ListEpisodes, Mark}`。
- [ ] §6.2、§6.3、§6.4 接口。
- [ ] 前端 §8.1、§8.2、§8.3。

### Phase 2（索引 + Recall，1～2 周）

- [ ] §3.3 FTS5 + meta 表。
- [ ] `BuildIndexFor` 异步 worker（队列：内存 channel，先简单实现）。
- [ ] `RecallByQuery`（BM25-only 先上线）。
- [ ] `MemoryL0Service.Assemble` 接 l2_recall 段（默认关闭）。

### Phase 3（巩固管道，2 周，依赖 L3/L4 文档完成）

- [ ] 系统级 Consolidator Agent 配置（写入 `agents` 表 + prompt 文件）。
- [ ] `RunConsolidationBatch` worker。
- [ ] L3 / L4 接口对接。
- [ ] §8.5 Recall 调试器。

### Phase 4（向量与扩展）

- [ ] 向量索引落地（pgvector / Milvus 二选一，详见 §15）。
- [ ] BM25 + vector 融合 ranker 调参。
- [ ] Retention scheduler。

---

## 13. 验收标准

- [ ] L1 task EndTask 后 ≤ 60s 内出现一条 `memory_episodes` 记录，consolidation_status=pending。
- [ ] 事件 Tab 能展示同一 session 下的 messages / tool / skill / mcp / model_call 事件，时间线倒序。
- [ ] 关键字检索 `keyword=xxx` 在 < 200ms 返回（单 session ≤ 5 万事件）。
- [ ] 用户标星某事件后，`memory_event_marks` 出现一行；对应 episode importance += 0.2 且 ≤ 1.0。
- [ ] 调用 `POST /l2/recall` 返回 ≤ top_k 条 episode，按 final_score 倒序，每条仅含摘要字段。
- [ ] 启动 RunConsolidationBatch 后，pending 状态的 episode 转为 done，并出现 L3 / L4 的新条目。
- [ ] 删除 episode 后，事件 Tab 仍可看到原始事件（事件源仍在），仅 episode 列表过滤掉。
- [ ] Trace 详情抽屉「事件来源」可跳转回事件 Tab 并定位到对应 span。
- [ ] Datadog 指标可见 `episode_created_total / consolidation.processed_total / recall_latency_ms`。
- [ ] 关闭 `l2_recall_enabled` 后，下一次 prompt 中不出现 memory.l2 段。

---

## 14. 关键设计原则

1. **事件复用既有事实表，不再造表**：messages / spans / steps / monitor_events 已经记录所有事实，L2 只补 episode 与索引。
2. **Episode 是 L2→L3/L4 的最小巩固单元**：所有从会话中升档的知识必须先经过 episode，避免散乱事件直接进入长期库。
3. **重要性是巩固调度的核心信号**：importance 综合 critic / user feedback / 完成度，决定是否进入 L3/L4。
4. **Recall 只返回摘要**：避免「整个会话被回灌」，保护 prompt 与隐私。
5. **索引是异步副本，不是事实源**：FTS / 向量索引重建任何时候都不破坏事实。
6. **Mark 是统一干预入口**：用户 / Critic / Plugin 都通过 marks 表对 L2 施加影响，便于审计。
7. **Retention 软删除优先**：默认 90 天保留 + 30 天归档；硬删除仅按合规请求触发。

---

## 15. 向量索引选型说明

| 阶段 | 方案 | 理由 |
|------|------|------|
| Phase 1-2 | SQLite FTS5（BM25）+ float32 BLOB（cosine 全表扫） | 与现有 SQLite 同库，零运维 |
| Phase 3 | pgvector 或 Qdrant，按部署模式二选一 | 数据量超过单 session 5 万事件 / 单 agent 1 万 episode 时性能不够 |
| Phase 4 | Milvus 集群（云上） | 跨租户、跨 workspace 全局检索 |

切换时，`MemoryL2Repository.SearchVector` 接口形态不变，仅替换底层实现。

---

## 16. 跨表事件视图 SQL（参考）

`ListEvents` 的 UNION ALL 形态（仅示意，实际需按 §3.1 真实字段微调）：

```sql
SELECT
  m.id                               AS id,
  'message'                          AS kind,
  m.session_id                       AS session_id,
  ''                                 AS run_id,
  ''                                 AS turn_id,
  ''                                 AS span_id,
  m.role                             AS actor_type,
  ''                                 AS actor_id,
  m.role                             AS actor_name,
  m.status                           AS status,
  ''                                 AS title,
  substr(m.content_markdown, 1, 200) AS preview,
  m.created_at                       AS occurred_at,
  m.latency_ms                       AS duration_ms,
  m.token_in                         AS tokens_in,
  m.token_out                        AS tokens_out,
  0                                  AS cost_micro,
  'messages'                         AS ref_table,
  m.id                               AS ref_id
FROM messages m
WHERE m.session_id = ?

UNION ALL

SELECT
  s.id, 'tool_call', s.session_id, s.run_id, s.turn_id, s.id,
  s.actor_type, s.actor_id, s.actor_name, s.status,
  s.tool_name, s.input_preview, s.started_at, s.duration_ms,
  s.input_tokens, s.output_tokens, s.cost_micro_usd,
  'session_trace_spans', s.id
FROM session_trace_spans s
WHERE s.session_id = ? AND s.span_type = 'tool_call'

UNION ALL

SELECT
  s.id, 'model_call', s.session_id, s.run_id, s.turn_id, s.id,
  s.actor_type, s.actor_id, s.actor_name, s.status,
  s.model, s.output_preview, s.started_at, s.duration_ms,
  s.input_tokens, s.output_tokens, s.cost_micro_usd,
  'session_trace_spans', s.id
FROM session_trace_spans s
WHERE s.session_id = ? AND s.span_type = 'model_call'

-- … skill_call / mcp_call / agent_handoff 同上扩展
ORDER BY occurred_at DESC
LIMIT ? OFFSET ?;
```

> 高 QPS 时把热门 SELECT 换成视图（VIEW）+ Redis 缓存；当前阶段直查即可。
