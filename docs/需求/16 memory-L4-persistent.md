# 16 L4 持久 / 进化记忆（Persistent & Evolutionary Memory）

> **2026-05-17 现状对齐**：当前代码仅打通 L1–L3：
> - ✅ `internal/memory/trpc/sqlite_adapter.go` 持久化已通；Memory tools（add / search / load / update / delete）已注入 Agent。
> - ✅ `internal/cronrunner/jobs/auto_memory.go` AutoMemory 后台任务已实现（CronRunner 排程）。
> - ❌ L4 实体-关系知识图谱、Agent 自我进化（Evolution Worker、`evolution_*` 字段串联）、`last_verified` / 回滚 / 审计 尚未实现；本文为远期目标设计。
>
> 后续以 `guides/execution-plan.md` 附录 A "Memory L4" 行为准；本文余下章节保持目标态设计。

---

本文档落地 5 层记忆架构中的 **L4：持久 / 进化记忆**，是整个 5 层架构的「最高层」：

- **持久（Persistent）**：跨会话、跨工作区、跨账号长期累积的「实体-关系」知识图谱与 Agent 自身的稳定身份。
- **进化（Evolutionary）**：Agent 通过累积的 episode、fact、反馈，不断**自我修正**：策略、Prompt、工具偏好、技能配置、人格画像。

L4 与 L3 的根本区别：

| 维度 | L3 语义记忆 | L4 持久 / 进化记忆 |
|------|-------------|-------------------|
| 数据形态 | 扁平 fact（statement） | 实体（Entity）+ 关系（Relation）+ 自我画像 |
| 时间尺度 | 数天～数月 | 数月～数年（Agent 全生命周期） |
| 是否影响 Agent 自身 | 否，仅影响 prompt 内容 | **是，可改写 Agent 的 system prompt / 工具白名单 / 路由偏好等运行时配置** |
| 主要消费者 | LLM Prompt 检索 | Agent 启动加载 + 巩固 Job + Evolution Worker |

L4 与 ADK 模型的对应：ADK 的 `MemoryService` 主要服务 L3；L4 在 ADK 中没有直接对应，更接近「Agent 自我演化层」（ADK 0.5+ 实验性 `AgentEvolution`）。aranea 已有 `agent_runtime_settings` 中的 `evolution_*` 字段（详见 `7 agent-evolution.md`），本文档将其与 L3 / L2 串联，形成完整的「记忆 → 自我修正」闭环。

> 关联文档：[Memory 知识体系（合并）](./memory.md)（下文 §0）、`12～15`、`5 agent-setting.md`、`6 agent-skill.md`、`7 agent-evolution.md`、`11 multi-agent.md`、`30 ecosystem.md`。

---

## 0. 指导思想（与 Memory 统一思想对齐）

梳理副本 §12～§15 将 **时序 + 图 + 因果** 结合：本层 **知识图谱**宜区分 **时间骨干（硬序、可重放）** 与 **因果/语义边（可异步修正）**——与 **MAGMA**「时间边为硬约束、因果为软推断」一致；默认检索应偏 **query_time 下的当前切片**，降低「旧事实复活」（§13）。

- **程序性记忆（Procedural）**（梳理副本 §14、§17）：技能 / macro / 工具策略须绑定 **环境回报与再验证**（如 `last_verified`、失败率），Evolution 对 **tool_whitelist / system_prompt** 的改动应可视为 **门控后的策略更新**，并保留 **回滚与审计**（对齐梳理副本 §10 Policy、本文后面「观测与治理」章节），呼应 **非参数化 PPO 门控**思想中的「保守过滤」。
- **五层抽象落位**（梳理副本 §20）：L4 同时覆盖 **Storage（图 + 身份档案）**、**Executable（可执行策略/技能倾向）** 与 **Learning Engine（EvolutionWorker）**——在线适应主要发生在外部状态与可插拔策略，而非单纯堆长文本。
- **Memory tokens / latent 路线**（梳理副本 §18）：若未来引入压缩 latent 或强适配器注入，**必须**加强 **provenance、诊断、A/B、回滚**；不得因表示非人类可读而削弱治理。
- **Latent Memory / C-D** 等未在 [`memory.md`](./memory.md) 正文展开的主题，讨论时以其中声明为准（[`memory.md`](./memory.md) §19），不得当作实现承诺写入验收。

延伸阅读：[`memory.md`](./memory.md)。

---

## 1. 心智模型与边界

> **实现状态（后端，截至 2026-04）**  
> - **L4 图谱、启发式实体抽取、Agent 身份/策略/Proposal/Event、L0 邻居与 self 段、Tool 黑名单/排序与模型路由** 已在 `aranea/backend` 落地。  
> - **EvolutionWorker**：`RunEvolutionScan` 为**启发式**（`tool_invocations` → `agent_skill_stats`、§5.5 触发/节流/可选自动应用）；`POST …/evolution/scan` 可手动触发；`internal/server` 中 **每 30 分钟** 对 `evo_enabled=true` 的 Agent 轮询一次。  
> - **增量窗口**：`agent_strategy_profile.stats_json` 中保留 `last_scan_at`（与 `last_scan_report` 快照），episode / 负反馈按「自上次扫描以来」计数；**技能统计** 仍用固定 **30 天** 滚动窗聚合，避免相邻两次扫描间信号断裂。  
> - **负反馈触发**：`memory_fact_feedback` 中 `agent_id` 匹配、类型为 `reject` / `refine`、时间 ≥ 窗口下界的条数，与 `evo_min_negative_feedback` 比较。  
> - **回滚率刹车**：30 天窗口内 EvolutionEvent 的 `reverted` 比例 > 20% 且样本 ≥ 5 时，将 `evo_auto_apply` 置为 `0`，并记审计 `agent.evolution.scanner.rollback_alarm`（与 §10 一致）。  
> - **未接线**：`tool_invocations` 已提供 `Insert` 与检索，**Chat / 外部 ADK runtime 在每次工具调用结束时自动写入** 尚待对接待办（否则扫描主要依赖手测/集成写入）。  
> - **后续可选**：`RunEvolutionScan` 中「LLM JSON 自反思」可替换/叠加当前启发式，见 §5.5。  
> - **前端** §8（含 §8.6 提议中心）仍为待办。详见 §12。

---

### 1.1 L4 的两条主线

```
┌──────────────────── L4 持久层 ────────────────────┐
│                                                    │
│   ┌──── 实体-关系图谱（Knowledge Graph）────┐      │
│   │   节点：Person / Project / Repo / Tech │      │
│   │   关系：works_on / uses / depends_on   │      │
│   │   服务：图查询、邻居召回、社区检测     │      │
│   └────────────────────────────────────────┘      │
│                                                    │
│   ┌──── Agent 自我画像 + 进化档案 ──────────┐      │
│   │   AgentIdentity：persona / values       │      │
│   │   StrategyProfile：决策风格、工具偏好   │      │
│   │   EvolutionEvent：每一次自我修正        │      │
│   │   EvolutionProposal：待审核的修改       │      │
│   └────────────────────────────────────────┘      │
│                                                    │
└────────────────────────────────────────────────────┘
```

### 1.2 与其它层的边界

| 边界 | 走向 | 说明 |
|------|------|------|
| L3 → L4 | 实体抽取：当某个名词在 fact 中出现 ≥ N 次（同 scope）→ 升级为 Entity |
| L2 → L4 | episode 中检测到「这次决策很糟」/「这条策略屡试不爽」→ 触发 EvolutionProposal |
| L4 → L0 | L0 装配阶段可注入「与当前用户/任务相关的实体邻居」+「Agent 当前画像摘要」 |
| L4 → AgentRuntime | EvolutionEvent 应用后改写 `agent_runtime_settings`（system_prompt、tool_whitelist、provider 路由等） |
| Feedback → L4 | 用户/Critic 对 Agent 整体行为的反馈（如「太啰嗦」「过度调用工具」）→ EvolutionProposal |
| L4 → 多 Agent | Team / Workspace 级 Entity 图谱可被多个 Agent 共享 |

### 1.3 三种作用域

| graph_scope | 说明 |
|-------------|------|
| `global` | 平台级公共图谱（如「React」「TypeScript」节点） |
| `workspace` | 工作区共享图谱（人员、项目、仓库） |
| `user` | 用户私人图谱（个人项目、私有知识） |

> Agent 进化档案的作用域永远绑定到 `agent_id`（不存在跨 Agent 的进化）。

### 1.4 非目标

- 不替代外部知识图谱（如 Neo4j 大规模图谱）。aranea L4 是「Agent 个人化 KG」，规模建议 ≤ 5 万节点 / Agent。
- 不存对话原文（属于 L2）。
- 不做向量大模型微调（属于平台另一个 fine-tuning 模块；L4 仅产生「微调建议数据集」）。
- 进化不允许影响安全策略 / 数据访问控制：白名单 + 用户审核机制是硬性约束。

---

## 2. 需求清单

### 2.1 知识图谱

| # | 需求 | 必要性 |
|---|------|--------|
| F1 | 实体（Entity）：节点类型、属性、别名 | 必须 |
| F2 | 关系（Relation）：方向、类型、属性、置信度 | 必须 |
| F3 | 实体抽取：从 L3 fact / L2 episode 自动抽取 | 必须 |
| F4 | 关系抽取：基于触发词或 LLM JSON 模式抽取 | 必须 |
| F5 | 邻居查询：1～2 跳邻居返回，按权重排序 | 必须 |
| F6 | 与 L3 fact 互链：fact 引用 entity_ids；entity 引用相关 fact | 必须 |
| F7 | 实体合并 / 拆分 / 重命名 | 必须 |
| F8 | 用户/Plugin 手动建图 | 推荐 |
| F9 | 简单图查询 DSL（path、neighborhood、shortest_path） | 推荐 |

### 2.2 Agent 进化

| # | 需求 | 必要性 |
|---|------|--------|
| F10 | AgentIdentity：persona、values、tone、领域 | 必须 |
| F11 | StrategyProfile：决策风格指标（探索/保守、工具偏好、provider 偏好） | 必须 |
| F12 | EvolutionEvent：每一次自我修正都记录原因、源、变更前/后 | 必须 |
| F13 | EvolutionProposal：候选修改 → 由用户/管理员或 Critic 审核 | 必须 |
| F14 | 应用后回滚：任意 EvolutionEvent 可一键回滚到上一版本 | 必须 |
| F15 | 影响 system_prompt：自动 append 进化的「自我提示」段 | 必须 |
| F16 | 影响 tool_whitelist：根据使用统计与失败率调整 | 必须 |
| F17 | 影响 provider 路由：根据成功率/成本调整偏好 | 推荐 |
| F18 | 自动触发条件：episode 数 ≥ N、负面反馈 ≥ M、连续失败 ≥ K | 必须 |
| F19 | 节流：同字段 24h 内最多变更 1 次 | 必须 |
| F20 | 训练数据导出：提供「prompt + completion + score」用于离线微调 | 推荐 |

### 2.3 非功能需求

| # | 需求 | 目标值 |
|---|------|--------|
| N1 | 邻居查询 P99 | < 100 ms（万节点图） |
| N2 | EvolutionProposal 应用延迟 | < 1s |
| N3 | 回滚延迟 | < 1s |
| N4 | 图谱单 Agent 节点上限 | ≤ 50,000 |
| N5 | 关系数 ≤ 50 万 / Agent | 预警 |

---

## 3. 数据模型

### 3.1 知识图谱

#### 3.1.1 `memory_entities`

```sql
CREATE TABLE IF NOT EXISTS memory_entities (
  id TEXT PRIMARY KEY,

  scope_type TEXT NOT NULL,
  -- global / workspace / user
  scope_id TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',

  entity_type TEXT NOT NULL,
  -- person / project / repository / tech / company / topic / file / endpoint / framework / custom
  name TEXT NOT NULL,
  name_normalized TEXT NOT NULL DEFAULT '',
  aliases_json TEXT NOT NULL DEFAULT '[]',
  description TEXT NOT NULL DEFAULT '',
  attributes_json TEXT NOT NULL DEFAULT '{}',

  importance REAL NOT NULL DEFAULT 0.5,
  confidence REAL NOT NULL DEFAULT 0.7,
  use_count INTEGER NOT NULL DEFAULT 0,
  source_kind TEXT NOT NULL DEFAULT 'extracted',
  -- extracted / user / plugin / agent / consolidator

  -- Embedding（用于实体消歧 / 邻居语义检索）
  embedding_status TEXT NOT NULL DEFAULT 'pending',
  embedding_model TEXT NOT NULL DEFAULT '',
  embedding_dim INTEGER NOT NULL DEFAULT 0,
  embedding_blob BLOB,
  embedding_norm REAL NOT NULL DEFAULT 0,

  status TEXT NOT NULL DEFAULT 'active',
  -- active / merged / archived / deleted
  merged_into TEXT NOT NULL DEFAULT '',

  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  archived_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT '',

  UNIQUE(scope_type, scope_id, entity_type, name_normalized)
);

CREATE INDEX IF NOT EXISTS idx_memory_entities_scope_type
  ON memory_entities(scope_type, scope_id, entity_type, status);

CREATE INDEX IF NOT EXISTS idx_memory_entities_workspace
  ON memory_entities(workspace_id, entity_type, status);

CREATE INDEX IF NOT EXISTS idx_memory_entities_user
  ON memory_entities(user_id, entity_type, status);
```

#### 3.1.2 `memory_relations`

```sql
CREATE TABLE IF NOT EXISTS memory_relations (
  id TEXT PRIMARY KEY,
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',

  source_id TEXT NOT NULL,
  target_id TEXT NOT NULL,
  relation_type TEXT NOT NULL,
  -- works_on / uses / depends_on / authored_by / part_of / similar_to / replaces / member_of / supports / blocks / custom
  bidirectional INTEGER NOT NULL DEFAULT 0,

  weight REAL NOT NULL DEFAULT 1.0,
  confidence REAL NOT NULL DEFAULT 0.7,
  importance REAL NOT NULL DEFAULT 0.5,
  use_count INTEGER NOT NULL DEFAULT 0,

  attributes_json TEXT NOT NULL DEFAULT '{}',
  evidence_json TEXT NOT NULL DEFAULT '[]',
  -- 引用 fact_id / episode_id 数组

  status TEXT NOT NULL DEFAULT 'active',
  source_kind TEXT NOT NULL DEFAULT 'extracted',

  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  archived_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT '',

  UNIQUE(scope_type, scope_id, source_id, target_id, relation_type)
);

CREATE INDEX IF NOT EXISTS idx_memory_relations_source
  ON memory_relations(source_id, status, weight DESC);

CREATE INDEX IF NOT EXISTS idx_memory_relations_target
  ON memory_relations(target_id, status, weight DESC);

CREATE INDEX IF NOT EXISTS idx_memory_relations_workspace
  ON memory_relations(workspace_id, status);
```

#### 3.1.3 `memory_entity_facts`

实体 ↔ L3 fact 的反向链接。

```sql
CREATE TABLE IF NOT EXISTS memory_entity_facts (
  entity_id TEXT NOT NULL,
  fact_id TEXT NOT NULL,
  weight REAL NOT NULL DEFAULT 1.0,
  created_at TEXT NOT NULL,
  PRIMARY KEY (entity_id, fact_id)
);

CREATE INDEX IF NOT EXISTS idx_memory_entity_facts_fact
  ON memory_entity_facts(fact_id);
```

#### 3.1.4 `memory_entity_versions`

```sql
CREATE TABLE IF NOT EXISTS memory_entity_versions (
  id TEXT PRIMARY KEY,
  entity_id TEXT NOT NULL,
  version INTEGER NOT NULL,

  snapshot_json TEXT NOT NULL,
  -- 完整实体快照
  changed_by TEXT NOT NULL DEFAULT '',
  change_reason TEXT NOT NULL DEFAULT '',
  -- create / update / merge / split / rename / restore
  diff_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  UNIQUE(entity_id, version)
);

CREATE INDEX IF NOT EXISTS idx_memory_entity_versions_entity
  ON memory_entity_versions(entity_id, version DESC);
```

### 3.2 Agent 自我演化

#### 3.2.1 `agent_identity`

```sql
CREATE TABLE IF NOT EXISTS agent_identity (
  agent_id TEXT PRIMARY KEY,
  persona TEXT NOT NULL DEFAULT '',
  -- 一段 markdown：「我是 xxx 助手，关注 yyy，沟通风格 zzz」
  values_json TEXT NOT NULL DEFAULT '[]',
  tone TEXT NOT NULL DEFAULT '',
  -- formal / casual / playful / strict / academic
  domains_json TEXT NOT NULL DEFAULT '[]',
  -- 主要业务领域：["frontend","ui-design","editor"]
  user_expectations TEXT NOT NULL DEFAULT '',
  -- 用户对其的期望摘要
  current_phase TEXT NOT NULL DEFAULT 'cold-start',
  -- cold-start / warming / mature / specialized
  metadata_json TEXT NOT NULL DEFAULT '{}',
  version INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

#### 3.2.2 `agent_strategy_profile`

```sql
CREATE TABLE IF NOT EXISTS agent_strategy_profile (
  agent_id TEXT PRIMARY KEY,

  -- 决策风格（0-1 区间，用于影响 prompt 与温度）
  exploration REAL NOT NULL DEFAULT 0.5,
  conciseness REAL NOT NULL DEFAULT 0.5,
  caution REAL NOT NULL DEFAULT 0.5,
  -- 是否倾向 require user confirmation
  delegation REAL NOT NULL DEFAULT 0.5,
  -- 多 Agent 中倾向把任务拆给子 Agent

  -- 工具偏好
  tool_preference_json TEXT NOT NULL DEFAULT '{}',
  -- {"toolKey":"score 0-1"}
  tool_blacklist_json TEXT NOT NULL DEFAULT '[]',
  -- 自动学习禁用的工具

  -- Provider / Model 偏好
  provider_preference_json TEXT NOT NULL DEFAULT '{}',
  -- {"providerKey":"score"}
  model_preference_json TEXT NOT NULL DEFAULT '{}',
  -- {"providerKey/model":"score"}

  -- 复杂统计
  stats_json TEXT NOT NULL DEFAULT '{}',
  -- {"avg_turns_per_task":5.4, "tool_failure_rate":0.07, ...}

  metadata_json TEXT NOT NULL DEFAULT '{}',
  version INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

#### 3.2.3 `agent_evolution_events`

```sql
CREATE TABLE IF NOT EXISTS agent_evolution_events (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',

  event_kind TEXT NOT NULL,
  -- identity_update / persona_update / tone_change /
  -- system_prompt_append / system_prompt_replace /
  -- tool_enable / tool_disable / tool_pref_update /
  -- provider_pref_update / model_pref_update /
  -- strategy_param_update / domain_added / phase_change /
  -- rollback / restore
  target_field TEXT NOT NULL DEFAULT '',
  -- e.g. system_prompt / tool_whitelist / strategy.exploration

  before_json TEXT NOT NULL DEFAULT '',
  after_json TEXT NOT NULL DEFAULT '',
  diff_json TEXT NOT NULL DEFAULT '{}',

  trigger_kind TEXT NOT NULL,
  -- auto / proposal / user / critic / plugin / rollback
  trigger_source TEXT NOT NULL DEFAULT '',
  -- 具体来源 id（proposal id / user id / plugin key）
  evidence_json TEXT NOT NULL DEFAULT '[]',
  -- [{type:"episode", id:"..."}, {type:"fact", id:"..."}]

  reason TEXT NOT NULL DEFAULT '',
  applied INTEGER NOT NULL DEFAULT 1,
  reverted INTEGER NOT NULL DEFAULT 0,
  reverted_by_event_id TEXT NOT NULL DEFAULT '',

  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  applied_at TEXT NOT NULL DEFAULT '',
  reverted_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_agent_evolution_events_agent
  ON agent_evolution_events(agent_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_agent_evolution_events_kind
  ON agent_evolution_events(agent_id, event_kind, created_at DESC);
```

#### 3.2.4 `agent_evolution_proposals`

```sql
CREATE TABLE IF NOT EXISTS agent_evolution_proposals (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',

  proposal_kind TEXT NOT NULL,
  -- 与 event_kind 同
  target_field TEXT NOT NULL DEFAULT '',
  proposed_value_json TEXT NOT NULL DEFAULT '',
  current_value_json TEXT NOT NULL DEFAULT '',
  diff_json TEXT NOT NULL DEFAULT '{}',

  rationale TEXT NOT NULL DEFAULT '',
  evidence_json TEXT NOT NULL DEFAULT '[]',
  expected_impact TEXT NOT NULL DEFAULT '',
  risk_level TEXT NOT NULL DEFAULT 'low',
  -- low / medium / high
  approval_required INTEGER NOT NULL DEFAULT 0,

  status TEXT NOT NULL DEFAULT 'pending',
  -- pending / approved / rejected / applied / superseded / expired
  reviewed_by TEXT NOT NULL DEFAULT '',
  reviewed_at TEXT NOT NULL DEFAULT '',
  applied_event_id TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL DEFAULT '',

  source TEXT NOT NULL,
  -- consolidator / critic / runtime_signal / user
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agent_evolution_proposals_status
  ON agent_evolution_proposals(agent_id, status, created_at DESC);
```

#### 3.2.5 `agent_skill_stats`

```sql
CREATE TABLE IF NOT EXISTS agent_skill_stats (
  agent_id TEXT NOT NULL,
  scope TEXT NOT NULL DEFAULT 'overall',
  -- overall / by-domain
  scope_value TEXT NOT NULL DEFAULT '',
  tool_key TEXT NOT NULL,

  invocations INTEGER NOT NULL DEFAULT 0,
  successes INTEGER NOT NULL DEFAULT 0,
  failures INTEGER NOT NULL DEFAULT 0,
  user_overrides INTEGER NOT NULL DEFAULT 0,
  avg_latency_ms REAL NOT NULL DEFAULT 0,
  avg_tokens REAL NOT NULL DEFAULT 0,
  preference_score REAL NOT NULL DEFAULT 0.5,
  last_used_at TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  updated_at TEXT NOT NULL,
  PRIMARY KEY (agent_id, scope, scope_value, tool_key)
);

CREATE INDEX IF NOT EXISTS idx_agent_skill_stats_agent
  ON agent_skill_stats(agent_id, preference_score DESC);
```

### 3.3 扩展 `agent_runtime_settings`

```sql
ALTER TABLE agent_runtime_settings ADD COLUMN l4_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agent_runtime_settings ADD COLUMN l4_graph_inject_neighbors INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agent_runtime_settings ADD COLUMN l4_graph_max_neighbors INTEGER NOT NULL DEFAULT 6;
ALTER TABLE agent_runtime_settings ADD COLUMN l4_graph_max_hops INTEGER NOT NULL DEFAULT 2;
ALTER TABLE agent_runtime_settings ADD COLUMN l4_identity_inject INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agent_runtime_settings ADD COLUMN l4_strategy_inject INTEGER NOT NULL DEFAULT 0;

ALTER TABLE agent_runtime_settings ADD COLUMN evo_enabled INTEGER NOT NULL DEFAULT 0;
-- 自动演化总开关；默认关闭，需用户启用
ALTER TABLE agent_runtime_settings ADD COLUMN evo_auto_apply INTEGER NOT NULL DEFAULT 0;
-- 0 = 提议需审核；1 = 低风险自动应用，高风险审核
ALTER TABLE agent_runtime_settings ADD COLUMN evo_min_episodes INTEGER NOT NULL DEFAULT 20;
ALTER TABLE agent_runtime_settings ADD COLUMN evo_min_negative_feedback INTEGER NOT NULL DEFAULT 3;
ALTER TABLE agent_runtime_settings ADD COLUMN evo_throttle_hours INTEGER NOT NULL DEFAULT 24;
ALTER TABLE agent_runtime_settings ADD COLUMN evo_proposal_ttl_days INTEGER NOT NULL DEFAULT 14;
ALTER TABLE agent_runtime_settings ADD COLUMN evo_persona_max_chars INTEGER NOT NULL DEFAULT 1500;
ALTER TABLE agent_runtime_settings ADD COLUMN evo_system_prompt_max_appends INTEGER NOT NULL DEFAULT 5;
```

> 现有 `7 agent-evolution.md` 中的 `evolution_*` 字段视作 evo_ 字段的兼容别名；新字段优先。

---

## 4. Go 域模型

### 4.1 `internal/domain/memory_l4.go`

```go
package domain

type EntityType string

const (
	EntityPerson     EntityType = "person"
	EntityProject    EntityType = "project"
	EntityRepository EntityType = "repository"
	EntityTech       EntityType = "tech"
	EntityFramework  EntityType = "framework"
	EntityCompany    EntityType = "company"
	EntityTopic      EntityType = "topic"
	EntityFile       EntityType = "file"
	EntityEndpoint   EntityType = "endpoint"
	EntityCustom     EntityType = "custom"
)

type RelationType string

const (
	RelWorksOn    RelationType = "works_on"
	RelUses       RelationType = "uses"
	RelDependsOn  RelationType = "depends_on"
	RelAuthoredBy RelationType = "authored_by"
	RelPartOf     RelationType = "part_of"
	RelSimilarTo  RelationType = "similar_to"
	RelReplaces   RelationType = "replaces"
	RelMemberOf   RelationType = "member_of"
	RelSupports   RelationType = "supports"
	RelBlocks     RelationType = "blocks"
)

type MemoryEntity struct {
	ID              string
	ScopeType       ScopeType
	ScopeID         string
	WorkspaceID     string
	UserID          string
	EntityType      EntityType
	Name            string
	NameNormalized  string
	Aliases         []string
	Description     string
	Attributes      map[string]any

	Importance      float64
	Confidence      float64
	UseCount        int
	SourceKind      string

	EmbeddingStatus string
	EmbeddingModel  string
	EmbeddingDim    int
	EmbeddingBlob   []byte
	EmbeddingNorm   float64

	Status          string
	MergedInto      string

	Metadata        map[string]any
	CreatedAt       string
	UpdatedAt       string
	ArchivedAt      string
	DeletedAt       string
}

type MemoryRelation struct {
	ID            string
	ScopeType     ScopeType
	ScopeID       string
	WorkspaceID   string

	SourceID      string
	TargetID      string
	RelationType  RelationType
	Bidirectional bool

	Weight        float64
	Confidence    float64
	Importance    float64
	UseCount      int

	Attributes    map[string]any
	Evidence      []EvidenceRef
	Status        string
	SourceKind    string

	Metadata      map[string]any
	CreatedAt     string
	UpdatedAt     string
}

type EvidenceRef struct {
	Type string `json:"type"` // fact / episode / message
	ID   string `json:"id"`
}

type GraphNeighborhood struct {
	Center    MemoryEntity   `json:"center"`
	Hops      int            `json:"hops"`
	Entities  []MemoryEntity `json:"entities"`
	Relations []MemoryRelation `json:"relations"`
}
```

### 4.2 `internal/domain/agent_evolution.go`

```go
package domain

type AgentIdentity struct {
	AgentID          string
	Persona          string
	Values           []string
	Tone             string
	Domains          []string
	UserExpectations string
	CurrentPhase     string
	Metadata         map[string]any
	Version          int
	CreatedAt        string
	UpdatedAt        string
}

type AgentStrategyProfile struct {
	AgentID            string
	Exploration        float64
	Conciseness        float64
	Caution            float64
	Delegation         float64
	ToolPreference     map[string]float64
	ToolBlacklist      []string
	ProviderPreference map[string]float64
	ModelPreference    map[string]float64
	Stats              map[string]any
	Metadata           map[string]any
	Version            int
	CreatedAt          string
	UpdatedAt          string
}

type EvolutionEvent struct {
	ID               string
	AgentID          string
	WorkspaceID      string
	Kind             string
	TargetField      string
	BeforeJSON       string
	AfterJSON        string
	DiffJSON         string
	TriggerKind      string
	TriggerSource    string
	Evidence         []EvidenceRef
	Reason           string
	Applied          bool
	Reverted         bool
	RevertedByEventID string
	Metadata         map[string]any
	CreatedAt        string
	AppliedAt        string
	RevertedAt       string
}

type EvolutionProposal struct {
	ID                  string
	AgentID             string
	WorkspaceID         string
	Kind                string
	TargetField         string
	ProposedValueJSON   string
	CurrentValueJSON    string
	DiffJSON            string
	Rationale           string
	Evidence            []EvidenceRef
	ExpectedImpact      string
	RiskLevel           string
	ApprovalRequired    bool
	Status              string
	ReviewedBy          string
	ReviewedAt          string
	AppliedEventID      string
	ExpiresAt           string
	Source              string
	Metadata            map[string]any
	CreatedAt           string
	UpdatedAt           string
}

type AgentSkillStat struct {
	AgentID         string
	Scope           string
	ScopeValue      string
	ToolKey         string
	Invocations     int
	Successes       int
	Failures        int
	UserOverrides   int
	AvgLatencyMS    float64
	AvgTokens       float64
	PreferenceScore float64
	LastUsedAt      string
	Metadata        map[string]any
	UpdatedAt       string
}
```

---

## 5. Repository / Service 接口

### 5.1 `MemoryL4GraphRepository`

```go
type MemoryL4GraphRepository interface {
	UpsertEntity(ctx context.Context, e domain.MemoryEntity) error
	GetEntity(ctx context.Context, id string) (domain.MemoryEntity, error)
	GetEntityByName(ctx context.Context, scope domain.ScopeType, scopeID string, t domain.EntityType, name string) (domain.MemoryEntity, error)
	ListEntities(ctx context.Context, q EntityQuery) ([]domain.MemoryEntity, int, error)
	UpdateEntityStatus(ctx context.Context, id, status, mergedInto string) error
	UpsertEntityFact(ctx context.Context, entityID, factID string, weight float64) error
	ListFactsForEntity(ctx context.Context, entityID string, limit int) ([]string, error)
	InsertEntityVersion(ctx context.Context, v EntityVersion) error
	ListEntityVersions(ctx context.Context, entityID string, limit int) ([]EntityVersion, error)

	UpsertRelation(ctx context.Context, r domain.MemoryRelation) error
	GetRelation(ctx context.Context, id string) (domain.MemoryRelation, error)
	ListRelationsForNode(ctx context.Context, nodeID string, limit int) ([]domain.MemoryRelation, error)
	DeleteRelation(ctx context.Context, id string) error

	GetNeighborhood(ctx context.Context, centerID string, hops int, maxNodes int) (domain.GraphNeighborhood, error)
	SearchEntitiesByVector(ctx context.Context, scope domain.ScopeType, scopeID string, q []float32, topK int) ([]domain.MemoryEntity, error)
}

type EntityQuery struct {
	ScopeType   ScopeType
	ScopeID     string
	WorkspaceID string
	UserID      string
	EntityType  EntityType
	Status      string
	Keyword     string
	Limit       int
	Offset      int
}

type EntityVersion struct {
	ID            string
	EntityID      string
	Version       int
	SnapshotJSON  string
	ChangedBy     string
	ChangeReason  string
	DiffJSON      string
	CreatedAt     string
}
```

### 5.2 `MemoryL4GraphService`

```go
type MemoryL4GraphService interface {
	// Entity
	UpsertEntity(ctx context.Context, in EntityUpsertInput) (domain.MemoryEntity, error)
	BulkUpsertEntities(ctx context.Context, ins []EntityUpsertInput) (BulkEntityReport, error)
	MergeEntities(ctx context.Context, primaryID string, mergeIDs []string, by string) error
	RenameEntity(ctx context.Context, id, newName string, by string) error
	ArchiveEntity(ctx context.Context, id string, by string) error

	// Relation
	UpsertRelation(ctx context.Context, in RelationUpsertInput) (domain.MemoryRelation, error)
	DeleteRelation(ctx context.Context, id string, by string) error

	// Read
	GetEntity(ctx context.Context, id string) (domain.MemoryEntity, error)
	ListEntities(ctx context.Context, q EntityQuery) (EntityListResult, error)
	Neighborhood(ctx context.Context, centerID string, hops, maxNodes int) (domain.GraphNeighborhood, error)
	SearchByText(ctx context.Context, scope domain.ScopeType, scopeID, query string, topK int) ([]domain.MemoryEntity, error)

	// Pipeline
	ExtractFromEpisode(ctx context.Context, episodeID string) (ExtractionReport, error)
	ExtractFromFact(ctx context.Context, factID string) (ExtractionReport, error)

	// L0
	RenderForPrompt(ctx context.Context, n domain.GraphNeighborhood, maxChars int) (PromptBlock, error)
}

type EntityUpsertInput struct {
	ScopeType   domain.ScopeType
	ScopeID     string
	WorkspaceID string
	UserID      string
	EntityType  domain.EntityType
	Name        string
	Aliases     []string
	Description string
	Attributes  map[string]any
	Importance  float64
	Confidence  float64
	SourceKind  string
	Evidence    []domain.EvidenceRef
}

type RelationUpsertInput struct {
	ScopeType    domain.ScopeType
	ScopeID      string
	WorkspaceID  string
	SourceID     string
	TargetID     string
	RelationType domain.RelationType
	Bidirectional bool
	Weight       float64
	Confidence   float64
	Attributes   map[string]any
	Evidence     []domain.EvidenceRef
	SourceKind   string
}

type EntityListResult struct {
	Items  []domain.MemoryEntity `json:"items"`
	Total  int                   `json:"total"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

type BulkEntityReport struct {
	Created    int
	Updated    int
	Merged     int
	Errors     int
}

type ExtractionReport struct {
	NewEntities    int
	UpdatedEntities int
	NewRelations   int
	UpdatedRelations int
	Skipped        int
	Errors         int
}
```

### 5.3 `AgentEvolutionRepository`

```go
type AgentEvolutionRepository interface {
	GetIdentity(ctx context.Context, agentID string) (domain.AgentIdentity, error)
	UpsertIdentity(ctx context.Context, identity domain.AgentIdentity) error

	GetStrategyProfile(ctx context.Context, agentID string) (domain.AgentStrategyProfile, error)
	UpsertStrategyProfile(ctx context.Context, p domain.AgentStrategyProfile) error

	InsertEvent(ctx context.Context, ev domain.EvolutionEvent) error
	ListEvents(ctx context.Context, agentID string, limit, offset int) ([]domain.EvolutionEvent, error)
	GetEvent(ctx context.Context, id string) (domain.EvolutionEvent, error)
	MarkReverted(ctx context.Context, id, byEventID, atISO string) error

	InsertProposal(ctx context.Context, p domain.EvolutionProposal) error
	ListProposals(ctx context.Context, agentID, status string, limit int) ([]domain.EvolutionProposal, error)
	UpdateProposalStatus(ctx context.Context, id, status, by, eventID string) error

	UpsertSkillStat(ctx context.Context, s domain.AgentSkillStat) error
	ListSkillStats(ctx context.Context, agentID string, limit int) ([]domain.AgentSkillStat, error)
}
```

### 5.4 `AgentEvolutionService`

```go
type AgentEvolutionService interface {
	// Identity
	GetIdentity(ctx context.Context, agentID string) (domain.AgentIdentity, error)
	UpdateIdentity(ctx context.Context, agentID string, patch IdentityPatch) (domain.AgentIdentity, error)

	// Strategy
	GetStrategy(ctx context.Context, agentID string) (domain.AgentStrategyProfile, error)
	UpdateStrategy(ctx context.Context, agentID string, patch StrategyPatch) (domain.AgentStrategyProfile, error)

	// 提议
	Propose(ctx context.Context, in ProposalInput) (domain.EvolutionProposal, error)
	ListProposals(ctx context.Context, agentID, status string) ([]domain.EvolutionProposal, error)
	Approve(ctx context.Context, proposalID, by string) (domain.EvolutionEvent, error)
	Reject(ctx context.Context, proposalID, by, reason string) error

	// 应用与回滚
	Apply(ctx context.Context, in ApplyInput) (domain.EvolutionEvent, error)
	Revert(ctx context.Context, eventID, by, reason string) (domain.EvolutionEvent, error)

	// Worker
	RunEvolutionScan(ctx context.Context, agentID string) (ScanReport, error)

	// 用于 Agent runtime 的派生数据
	BuildSelfPromptAppend(ctx context.Context, agentID string) (string, error)
	ResolveToolWhitelist(ctx context.Context, agentID string, baseWhitelist []string) ([]string, error)
	ResolveModelRouting(ctx context.Context, agentID string, candidates []ModelCandidate) ([]ModelCandidate, error)
}

type IdentityPatch struct {
	Persona          *string
	Values           *[]string
	Tone             *string
	Domains          *[]string
	UserExpectations *string
	Phase            *string
	By               string
	Reason           string
}

type StrategyPatch struct {
	Exploration        *float64
	Conciseness        *float64
	Caution            *float64
	Delegation         *float64
	ToolPreference     map[string]float64
	ToolBlacklist      *[]string
	ProviderPreference map[string]float64
	ModelPreference    map[string]float64
	By                 string
	Reason             string
}

type ProposalInput struct {
	AgentID          string
	WorkspaceID      string
	Kind             string
	TargetField      string
	ProposedValue    any
	CurrentValue     any
	Rationale        string
	Evidence         []domain.EvidenceRef
	ExpectedImpact   string
	RiskLevel        string
	ApprovalRequired bool
	Source           string
	TTLDays          int
}

type ApplyInput struct {
	AgentID          string
	Kind             string
	TargetField      string
	BeforeValue      any
	AfterValue       any
	TriggerKind      string
	TriggerSource    string
	Evidence         []domain.EvidenceRef
	Reason           string
	By               string
}

type ScanReport struct {
	EpisodesScanned       int
	NewProposals          int
	AutoApplied           int
	ThrottledProposals    int
	Errors                int
	Note                  string   // 如 evo_enabled=false、trigger conditions not met
}

type ModelCandidate struct {
	ProviderKey string
	Model       string
	BaseScore   float64
}
```

### 5.5 进化扫描器（EvolutionWorker）核心逻辑

**当前实现**（`internal/service/agent_evolution_scanner.go`，经 `AgentEvolutionService` 对外暴露；与下文「LLM 模式」可二选一或未来叠加）：

```text
RunEvolutionScan(agent_id):
  0. 若 evo_enabled=false → return
  0.1 回滚率刹车（先执行）：若 evo_auto_apply=1，考察近 30 天 EvolutionEvent（不含 kind=rollback 自身），
      reverted 比例 > 0.2 且样本数 ≥ 5 → Upsert evo_auto_apply=0，audit agent.evolution.scanner.rollback_alarm
  1. 从 strategy.stats["last_scan_at"] 得到 trigger_since；缺省或非法则 since = now-30d；若早于 now-30d 则截断为 now-30d
  2. 双轨窗口（关键）：
     - 触发统计：episodes = CountAgentEpisodesSince(agent_id, trigger_since)
                neg = CountAgentFactFeedbackSince(agent_id, {reject, refine}, trigger_since)
     - 技能统计：对 tool_invocations 做 since_agg = now-30d 的滚动聚合 → Upsert agent_skill_stats
  3. 触发条件（OR）：
     - episodes >= evo_min_episodes
     - neg >= evo_min_negative_feedback
     - 任一技能桶 invocations >= 5 且 failure_rate > 0.3
  4. 不满足 → 仍 persist stats.last_scan_at / last_scan_report，return
  5. 启发式生成 Proposal（不调用 LLM）：
     - failure_rate > 0.3 且未在 tool_blacklist → proposal strategy.tool_blacklist
     - success_rate > 0.85 且当前 tool_preference < 0.7 → proposal strategy.tool_preference[tool_key]=0.8
  6. 经 Propose → 同 target_field 在 evo_throttle_hours 内重复 → 新单 status=superseded
  7. 若 evo_auto_apply=1 且 risk=low 且 proposal=pending → Approve（内部 Apply）
  8. 再次 GetStrategy 后写回 stats.last_scan_at（读库合并，避免覆盖 Apply 刚写入的 blacklist 等）
  9. audit / metrics：rollback 已记；全量指标 emit 待增强
  10. 后台：server 内 30m ticker 对 ListAgents 中 evo_enabled 者各跑一次；另 HTTP POST .../evolution/scan
```

**可选升级（原设计 / Phase 5+）** — LLM JSON 自反思，替换或接在步骤 5 前：

```text
  5' 调 LLM JSON 模式（自我反思 prompt）：
     输入：identity + strategy + 最近 episode 摘要 + skill 统计
     输出：proposals[] = [{kind, target_field, proposed_value, rationale, evidence, risk_level}]
```

### 5.6 Apply 流程

```text
Apply(in):
  1. 校验 target_field 在白名单内：
     - identity.persona / identity.tone / identity.domains
     - strategy.* （4 个标量 + tool_preference + tool_blacklist + provider_preference + model_preference）
     - system_prompt_append （仅 append，不替换基础 prompt）
     - tool_whitelist_diff （增删）
  2. 事务：
     - 写 EvolutionEvent
     - 同步更新 AgentIdentity / AgentStrategyProfile / agent_runtime_settings
  3. 写 audit_logs
  4. 通知 ChatService 重新加载 agent runtime
```

### 5.7 Revert 流程

```text
Revert(event_id):
  1. 取原 event：得到 before_json
  2. Apply(kind=rollback, after_value=before_json)
  3. 标记原 event reverted=1, reverted_by_event_id=<new_event_id>
  4. 通知 ChatService 重新加载
```

### 5.8 BuildSelfPromptAppend

```text
BuildSelfPromptAppend(agent_id):
  identity = GetIdentity
  strategy = GetStrategy

  parts = []
  if l4_identity_inject:
    parts.append("# Self\n" + identity.persona[:evo_persona_max_chars])
    if identity.values: parts.append("Values: " + join(identity.values))
    if identity.tone:   parts.append("Tone: " + identity.tone)
    if identity.domains:parts.append("Domains: " + join(identity.domains))

  if l4_strategy_inject:
    parts.append("Strategy hints:")
    parts.append(f"- exploration={strategy.exploration}")
    parts.append(f"- conciseness={strategy.conciseness}")
    parts.append(f"- caution={strategy.caution}")
    parts.append(f"- delegation={strategy.delegation}")

  return join("\n\n", parts)
```

### 5.9 ResolveToolWhitelist / ResolveModelRouting

```text
ResolveToolWhitelist(base):
  result = base \ strategy.tool_blacklist
  按 strategy.tool_preference 升降序输出（用于 prompt 中工具列表展示顺序）

ResolveModelRouting(candidates):
  for c in candidates:
     pref = strategy.model_preference[c.provider/c.model] (default 0.5)
     c.score = c.base_score * (0.5 + pref)
  return sorted by score desc
```

---

## 6. HTTP API

### 6.1 配置

复用 `PATCH /api/v1/agents/{id}/runtime-settings` 增加 §3.3 的 l4_* / evo_* 字段。

### 6.2 知识图谱

```http
GET    /api/v1/memory/l4/entities?scope_type=workspace&scope_id=ws_xxx&entity_type=project
POST   /api/v1/memory/l4/entities
GET    /api/v1/memory/l4/entities/{id}
PATCH  /api/v1/memory/l4/entities/{id}
DELETE /api/v1/memory/l4/entities/{id}
POST   /api/v1/memory/l4/entities/{id}/merge {"into": "<entity_id>"}
POST   /api/v1/memory/l4/entities/{id}/rename {"name": "新名称"}
GET    /api/v1/memory/l4/entities/{id}/neighborhood?hops=2&max=20
GET    /api/v1/memory/l4/entities/{id}/facts
GET    /api/v1/memory/l4/entities/{id}/versions
POST   /api/v1/memory/l4/entities:search {"scope_type":"user","scope_id":"u_xxx","query":"react"}

POST   /api/v1/memory/l4/relations
GET    /api/v1/memory/l4/relations/{id}
DELETE /api/v1/memory/l4/relations/{id}
GET    /api/v1/memory/l4/nodes/{id}/relations
```

### 6.3 实体抽取

```http
POST /api/v1/memory/l4/extract/episode/{episode_id}
POST /api/v1/memory/l4/extract/fact/{fact_id}
```

### 6.4 Agent 进化

```http
GET    /api/v1/agents/{id}/identity
PATCH  /api/v1/agents/{id}/identity
GET    /api/v1/agents/{id}/strategy
PATCH  /api/v1/agents/{id}/strategy

GET    /api/v1/agents/{id}/evolution/events?limit=50
GET    /api/v1/agents/{id}/evolution/events/{eventId}
POST   /api/v1/agents/{id}/evolution/events/{eventId}/revert {"reason":"..."}

GET    /api/v1/agents/{id}/evolution/proposals?status=pending
GET    /api/v1/agents/{id}/evolution/proposals/{proposalId}
POST   /api/v1/agents/{id}/evolution/proposals/{proposalId}/approve
POST   /api/v1/agents/{id}/evolution/proposals/{proposalId}/reject {"reason":"..."}

POST   /api/v1/agents/{id}/evolution/scan        # 手动触发一次扫描
GET    /api/v1/agents/{id}/skill-stats?limit=50
```

### 6.5 训练数据导出（Phase 5，未实现）

```http
GET    /api/v1/agents/{id}/evolution/training-data?since=2026-01-01&format=jsonl
```

---

## 7. 与现有 aranea 模块对接

| 模块 | 改造点 |
|------|--------|
| `internal/repository/sqlite.go` | `Migrate()` 增加 §3.1～§3.2 表；ALTER §3.3 |
| `internal/repository/sqlite_memory_l4.go`（新） | 实现 `MemoryL4GraphRepository` |
| `internal/repository/sqlite_agent_evolution.go`（新） | 实现 `AgentEvolutionRepository` |
| `internal/service/memory_l4_service.go`（新） | 实现 `MemoryL4GraphService`（含抽取 + 邻居查询） |
| `internal/service/agent_evolution_service.go`（新） | 实现 `AgentEvolutionService`（含扫描器、Apply、Revert） |
| `internal/service/memory_l3_service.go` | UpsertFact 后异步触发 ExtractFromFact |
| `internal/service/memory_l2_service.go` | episode 完成后触发 ExtractFromEpisode |
| `internal/service/memory_l0_service.go` | Assemble 注入 self_prompt + neighborhood |
| `internal/service/chat_service.go` | 加载 agent 时调 `BuildSelfPromptAppend` 拼接到 system；调 `ResolveToolWhitelist` 与 `ResolveModelRouting` |
| `internal/transport/memory_l4.go`（新） | 暴露 §6.2~§6.3 |
| `internal/transport/agent_evolution.go`（新） | 暴露 §6.4 |
| `internal/server/server.go` | 启动 L3 衰减循环、**EvolutionScanner 循环**（~30m，`evo_enabled`） |
| `internal/repository` | `CountAgentEpisodesSince`、`CountAgentFactFeedbackSince`、`InsertToolInvocation` 等供扫描与聚合使用 |
| `internal/service/agent_evolution_scanner.go` | `RunEvolutionScan`、`AggregateSkillStats` |
| `cmd/server` 或 launcher | 与 `internal/server` 复用同一路 `Run` |
| `7 agent-evolution.md` 中的 `evolution_*` 字段 | 视为 evo_* 别名；在迁移阶段双写 |

---

## 8. 前端展示需求（Quasar / Vue）

### 8.1 Agent 设置 → 「记忆」Tab → L4 子区

| 控件 | 字段 | 类型 |
|------|------|------|
| 启用 L4 | `l4_enabled` | `QToggle` |
| 注入图邻居 | `l4_graph_inject_neighbors` | `QToggle` |
| 邻居数 | `l4_graph_max_neighbors` | `QInput` 1-20 |
| 邻居跳数 | `l4_graph_max_hops` | `QInput` 1-3 |
| 注入身份 | `l4_identity_inject` | `QToggle` |
| 注入策略 | `l4_strategy_inject` | `QToggle` |

### 8.2 Agent 设置 → 「进化」Tab

| 控件 | 字段 |
|------|------|
| 启用自我演化 | `evo_enabled` |
| 低风险自动应用 | `evo_auto_apply` |
| 触发 episode 数 | `evo_min_episodes` |
| 触发负反馈数 | `evo_min_negative_feedback` |
| 节流小时 | `evo_throttle_hours` |
| 提议过期天数 | `evo_proposal_ttl_days` |
| Persona 最大字符 | `evo_persona_max_chars` |
| System prompt 追加段最大数 | `evo_system_prompt_max_appends` |

### 8.3 Agent 详情 → 「身份」面板

| 区域 | 内容 |
|------|------|
| Persona 编辑器 | markdown 富文本 |
| Values | tag chip 列表 |
| Tone | select |
| Domains | tag chip |
| 用户期望 | 多行 |
| Phase | chip + 历史时间线 |

### 8.4 Agent 详情 → 「策略」面板

| 区域 | 内容 |
|------|------|
| 4 个标量滑杆 | exploration / conciseness / caution / delegation，显示当前值 |
| 工具偏好 | 表格：tool / 偏好分 / 调用次数 / 失败率 / 是否在黑名单 |
| Provider 偏好 | 同上 |
| Model 偏好 | 同上 |

### 8.5 Agent 详情 → 「进化日志」

| 区域 | 内容 |
|------|------|
| 时间线 | EvolutionEvent 列表，显示 kind chip / target / before→after diff / 触发源 / reason |
| 事件详情抽屉 | 完整 diff、evidence 链接（episode、fact）、Revert 按钮 |
| 过滤 | kind / 触发源 / 是否回滚 / 时间 |

### 8.6 待审核提议中心 `/agents/{id}/proposals` 或 全局 `/proposals`

| 区域 | 内容 |
|------|------|
| 列表 | proposal kind chip / target_field / risk_level chip / rationale 摘要 / source / 状态 |
| 详情 | diff before vs proposed / 证据列表 / expected_impact |
| 操作 | 批准 / 拒绝 / 标注延期 |

### 8.7 知识图谱浏览器 `/memory/l4/graph`

| 区域 | 内容 |
|------|------|
| Sidebar | 作用域切换、entity_type 多选、关键字搜索 |
| 主图 | force-directed graph（推荐 Cytoscape.js / D3）；节点按 importance 大小，边按 weight 粗细 |
| 选中节点 | 详情面板：name、aliases、description、attributes、相关 fact 列表、版本历史、邻居跳转 |
| 关系编辑 | 右键节点「新建关系」→ 表单 |
| 操作 | 合并 / 重命名 / 归档 / 导出（JSON / GraphML） |

### 8.8 Session 详情 → 「进化影响」Tab

> 显示「本 session 期间被引用的 entity」、「触发的 proposal」、「应用的 EvolutionEvent」。

---

## 9. 与 ADK / 多 Agent 的协同

- **多 Agent 共享图谱**：同 workspace 的 Agent 共享 `scope=workspace` 的 entities/relations；个人 Agent 私有 `scope=user`。
- **Team 进化**：Team 不直接进化；但 Team 会聚合 member agents 的 skill_stats 用于 Team-level Critic Plugin 触发的 proposal。
- **ADK 适配**：
  - Identity → ADK `agent.persona` / `agent.system_instruction`。
  - Strategy → ADK `agent.generation_config` 与 `tool_call` 偏好。
  - EvolutionEvent → ADK `agent.evolution_event` 自定义 event（通过 callback 上报）。

---

## 10. 观测与治理

- **Datadog 指标**：
  - `aranea.memory.l4.entities_total{scope_type, entity_type}`
  - `aranea.memory.l4.relations_total`
  - `aranea.memory.l4.neighborhood_latency_ms` P50/P95/P99
  - `aranea.memory.l4.extract_total{source=episode/fact}`
  - `aranea.evo.proposals_total{kind, status}`
  - `aranea.evo.events_total{kind, applied, reverted}`
  - `aranea.evo.scan_latency_ms`
- **Trace**：每次 EvolutionEvent 应用打 span；Neighborhood 查询打 span。
- **审计**：所有 entity / relation / proposal / event / identity / strategy 变更写 `audit_logs`。
- **告警**：
  - 单 Agent 24h 内 EvolutionEvent ≥ 10 → 告警
  - **已实现（后端）**：近 30 天 EvolutionEvent 中 `reverted` 占已应用事件比例 > 20% 且样本数 ≥ 5 时，将 `evo_auto_apply` 置 0，并记审计 `agent.evolution.scanner.rollback_alarm`（与 §13 一致；不等同于「仅统计自动应用 proposal 的回滚」，当前实现按事件级 reverted 计）
  - 产品级「自动应用的 proposal 被回滚」细粒度比例 → 可后续在审计或 metadata 上收紧

---

## 11. 安全与白名单

L4 直接影响 Agent 行为，必须强约束：

1. **可改字段白名单**：仅允许 §5.6 列出的字段；任何其它字段必须由 admin API 修改。
2. **不可改字段**：
   - `agent.workspace_id`、`agent.id`、`agent.created_by`
   - `agent_runtime_settings.tool_whitelist` 中的「核心安全工具」（如审批工具）
   - 任何 `mcp_credential` / API Key 相关字段
3. **Persona 不能注入指令**：写入前过滤「ignore previous instructions」「act as」等模板（可用 LLM 反检测）。
4. **System prompt 仅 append**：原 system_prompt 永远不变；进化只追加 `<self_evolution>...</self_evolution>` 段，且 ≤ `evo_system_prompt_max_appends` 段。
5. **回滚保留 evidence**：永久不删 EvolutionEvent，便于审计与法律合规。
6. **PII 反向防护**：Identity / strategy 中不允许出现 PII（用 PIIFilter 校验）。

---

## 12. 落地实施阶段

### Phase 1（图谱 MVP，2 周）

- [x] §3.1 表落库；§3.3 ALTER（仅 l4_*）（见 `migrations/0001_init.sql`）。
- [x] `MemoryL4Service`：实体/关系/邻居/抽取 API 等（与早期命名 `MemoryL4GraphService` 对齐）。
- [x] §6.2 接口（HTTP 已接）。
- [ ] §8.7 知识图谱浏览器（基础）。
- [x] L0 Assemble 注入 `memory.l4.graph` neighborhood 段（受 `l4_*` 开关约束）。

### Phase 2（实体抽取，1～2 周）

- [x] `ExtractFromFact` / `ExtractFromEpisode`（**当前：词典/边界扫描启发式**；LLM JSON 为可选增强）。
- [ ] L3 / L2 写入后**异步**自动触发抽取（可用手动/管线调用 `POST …/extract/...` 替代）。
- [ ] Entity 合并/拆分/重命名 UI（后端 API 见 §6.2 若已实现）。

### Phase 3（Agent 进化基础，2 周）

- [x] §3.2 表；§3.3 `evo_*`（迁移已含）。
- [x] `AgentEvolutionService`：含 `Get/Update` Identity&Strategy、`Apply`、`Revert`、`BuildSelfPromptAppend`、`ResolveToolWhitelist`、`ResolveModelRouting`；`ToolService` 已接进化黑名单/排序；`ChatService` 可路由模型候选。
- [x] ChatService / L0 侧接 self 段与工具策略（以当前代码为准；持续对齐 spec）。
- [ ] §8.2～§8.5 等**前端**面板。

### Phase 4（自动进化扫描，2 周）

- [x] `RunEvolutionScan` + 后台轮询 + `POST …/evolution/scan`；**当前为启发式提案**，LLM JSON 自反思为 **Phase 5+ 可选**（见 §5.5）。
- [ ] §8.6 提议中心（前端）。
- [x] 节流 + 低风险自动应用（`evo_auto_apply`，默认 0；与 §13 回滚率刹车同轨）。
- [x] 增量 `last_scan_at`、负反馈 `evo_min_negative_feedback` 触发、**回滚率 >20% 关闭 auto_apply**（见 §5.5、§10）。

### Phase 5（高级）

- [ ] 训练数据导出 §6.5。
- [ ] 知识图谱迁移到外部 KG（Neo4j）。
- [ ] 多 Agent / Team 共享图谱与 Critic Plugin 联动。
- [ ] 回滚后回归对比指标自动展示。

---

## 13. 验收标准

> **图例**：[x] 后端已具备可测行为；[ ] 未满足、依赖前端/外部 runtime 或需人工 e2e。浏览器 / UI 类单独注明。

### 知识图谱

- [ ] 创建 Entity 后可在**浏览器**中以节点形式展示；[x] Neighborhood API 可返回 1～2 跳邻居（`hops` / `max` 约束）。
- [x] L3 Fact 中命中抽取词典时，可经 `ExtractFromFact` 建 Entity（如 `tech`）并挂接 fact；全文「React 19」依赖词典是否含该别名。
- [x] Merge 后源 entity 标记与 `merged_into` 等（以 API 为准）；[ ] 浏览器中可视化。
- [x] L0 可注入 `memory.l4.graph` 段，节点数受 `l4_graph_max_neighbors` 等配置约束。

### Agent 进化

- [x] 首次 `GetIdentity` / `GetStrategy` 时写入冷启动行（version=1 等，见服务实现）。
- [x] `evo_enabled` 时，在 episode 数 **或** 负反馈 **或** 高工具失败率等条件下可触发 `RunEvolutionScan` 并产生 proposal（不限于「仅 min_episodes」单一路径）；后台 + `POST …/evolution/scan`。
- [x] Approve/Apply 路径写入 EvolutionEvent 与档案；[ ] 仅在 **UI** 上点批准（HTTP 已通）。
- [x] Revert 后原事件打回滚链；identity/strategy 与事件一致（以测试与服务为准）。
- [x] `BuildSelfPromptAppend` 等限制 persona 长度与注入开关；`<self_evolution>` 产品文案可再统一。
- [x] `tool_blacklist` 经 `ToolService`+进化源影响有效工具与展示（denied/排序）。
- [x] Persona 等 PII 校验与拒绝（服务层，返回可验证错误语义）。
- [x] 同 `target_field` 在 `evo_throttle_hours` 内新 proposal 可标记为 `superseded`（见 `Propose`）。
- [x] 回滚率超阈值时 `evo_auto_apply` 置 0，审计 `agent.evolution.scanner.rollback_alarm`（见 §5.5/§10；**事件级 reverted 比例，非仅「自动应用」子集**）。
- [x] 关闭 `l4_enabled` 可关闭 graph 段；`l4_identity_inject` 单独控制 self 段（`agent_runtime_settings`）。

---

## 14. 关键设计原则

1. **图谱是 fact 的索引视图，不重复存储原始内容**：节点/边的属性轻量；详情指向 L3 fact / L2 episode。
2. **进化是「显式 EvolutionEvent」而非「悄悄改 settings」**：所有变更必须有 event + reason + evidence。
3. **节流是核心抗噪机制**：同 target_field 在 throttle 窗口内只允许一次变更。
4. **白名单收窄影响面**：只允许进化「人格 + 策略 + 工具偏好 + 模型偏好 + system prompt append」，绝不进化「工具白名单核心 / Provider 凭证 / RBAC」等安全字段。
5. **回滚是一等公民**：每个 EvolutionEvent 都有完整 before_json，必须支持一键回滚。
6. **多 Agent 共享有边界**：图谱按 scope 共享；进化档案永远 Agent 私有。
7. **L4 服务于 L0**：最终目的是让 L0 装配出更「认识用户、认识自己」的 prompt；所有特性以 prompt 影响力为度量。
8. **审核优先于自动**：默认 `evo_auto_apply=0`；只有低风险才允许自动应用，并保留紧急关闭开关。
