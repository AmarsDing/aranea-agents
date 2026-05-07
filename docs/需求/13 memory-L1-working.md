# 13 L1 工作记忆 / 核心记忆（Working Memory）

本文档落地 5 层记忆架构中的 **L1：工作记忆 / 核心记忆**。L1 是当前任务关键信息的**结构化快照**：任务目标、活跃约束、子任务状态、中间结果、关键决策。容量严格受限、访问极快、由编排 Agent 主动读写，任务结束自动归档到 L2（事件记忆）。

L1 区别于 L0：L0 是**整段消息历史的滑动窗口**，L1 是**任务级的结构化字段**。L1 也区别于 L3：L3 是跨会话的**事实知识**，L1 只是当前任务**进行中**的状态。

> 关联文档：[Memory 知识体系（合并）](./memory.md)（下文 §0）、`12 memory-L0-sensory.md`、`10 session.md`、`11 multi-agent.md`、`5 agent-setting.md`。

---

## 0. 指导思想（与 Memory 统一思想对齐）

梳理副本 §2 **命题 B** 强调 **Policy** 必须把「何时读、读多少、何时写」**显性化**为可记录的动作；本层即为 **写入工作记忆状态的 Policy 承载面**：通过 API 与 **`working_memory.*` 工具**完成 ADD/UPDATE/DELETE，并把每次变更写入 **field history / audit**，满足 **provenance / rollback**（梳理副本 §4 结论链、§10）。

- **非「再造一份聊天记录」**：L1 存的是当前任务可用的**结构化决策态**（goal / constraints / decisions），对应 [`memory.md`](./memory.md) 中「把历史转成当前可用信息」的 **System 2 慢回路**一环；与只靠 prompt 暗示相比，更可治理、可回放。
- **Memory Algorithm Protocol 取向**：写入受 **schema、预算、`IfRevision`** 约束（UPDATE 有界、可冲突检测），而非无界覆盖；与 §5.2「写时校验」一致。
- **与 System 1 的交界**：`RenderForPrompt` 输出的块即注入 L0 的 **证据包**；检索噪声在低层已通过「仅结构化、短预览」减负，仍须注意 **并行多 Agent** 时的命名空间隔离（梳理副本 §5 交叉项：错状态会向上污染推理）。

延伸阅读：[`memory.md`](./memory.md)。

---

## 1. 心智模型与边界

### 1.1 L1 在 5 层中的位置

| 维度 | 描述 |
|------|------|
| 容量 | 单 task 严格受限：建议 2K~8K tokens；行数硬上限 100 字段 |
| 持久性 | 任务级（多轮）；任务结束（success/failed/cancelled/timeout）后归档到 L2，不再消费 |
| 访问 | 结构化字段 + JSON Schema；通过 Service API 显式读写；无向量检索 |
| 与 ADK 对齐 | 对应 ADK `Session.state`（魔法前缀去掉，保留结构化） |
| 与 ChatGPT/Claude 「Memory」 | 不同；ChatGPT 那套是 L3 跨会话偏好；L1 是「当前正在做的事」 |

### 1.2 与其它层的边界

| 边界 | 走向 | 说明 |
|------|------|------|
| L0 → L1 | L0 装配时把 L1 字段渲染成 system message 注入 prompt（见 `12 ...md` §5.2 step 4） |
| L1 → L0 | 反向；L1 不直接写 prompt，由 L0 渲染层负责 |
| L1 ↔ Tool | 工具可通过 `set_working_state` / `get_working_state` 工具显式读写 |
| L1 ↔ Agent | Agent prompt / Skill 自动绑定一组「期望出现的字段」，缺失时引导 LLM 填写 |
| L1 → L2 | 任务结束时整段 snapshot 写入 `memory_episodes`（L2 文档定义） |
| L1 → L3 | 高价值字段（如确认的用户偏好）由 L2→L3 巩固管道升档 |

### 1.3 非目标

- 不替代 `messages` 表；L1 不存对话内容。
- 不做跨任务的偏好持久化（属于 L3）。
- 不做长期审计（任务归档后 L1 表删除，审计在 L2/L3）。
- 不存 PII / 敏感长文本；只存结构化字段、引用 ID、摘要预览。

---

## 2. 需求清单

### 2.1 功能需求

| # | 需求 | 必要性 |
|---|------|--------|
| F1 | 每个 session 内可创建一个或多个 task；每个 task 维护一份 L1 状态 | 必须 |
| F2 | L1 状态以**结构化字段（JSON Schema 校验）**保存：task_goal、active_constraints、subtasks、intermediate_results、key_decisions、open_questions | 必须 |
| F3 | 字段支持版本化（每次写入产生 revision），可回滚最近 N 个版本 | 必须 |
| F4 | 提供 `get / set / patch / delete` 显式 API，支持事务 | 必须 |
| F5 | 提供工具形式 `working_memory.read` / `working_memory.write` 注册到 Agent，便于 LLM 调用 | 必须 |
| F6 | Agent 可声明 `expected_fields_schema`，缺失时 L0 装配阶段提示模型补全 | 推荐 |
| F7 | Team session 中：每个子 Agent 各自一个 L1 命名空间；可设置共享字段（synthesizer 可读所有 worker） | 必须 |
| F8 | 任务终止（complete/abort/timeout）时自动 snapshot 整体 L1 → 写入 L2 `memory_episodes` | 必须 |
| F9 | L1 字段总 token 超限时自动拒写并返回 `OVERFLOW`，调用方负责精简 | 必须 |
| F10 | 支持「字段过期 TTL」：长时间未访问的字段自动降级为只读，避免污染 prompt | 推荐 |

### 2.2 非功能需求

| # | 需求 | 目标值 |
|---|------|--------|
| N1 | get 操作 P99 延迟 | < 5 ms |
| N2 | set 操作 P99 延迟 | < 15 ms |
| N3 | 单 task L1 总 token 上限 | 默认 8K（可配） |
| N4 | 单字段大小上限 | 默认 2K tokens |
| N5 | 字段写入冲突检测 | 基于 `revision` 乐观锁，冲突时返回 409 |

### 2.3 配置需求

复用 `agent_runtime_settings`，新增 L1 字段；详见 §6.1。

---

## 3. 数据模型

### 3.1 新增表：`memory_l1_tasks`

一份 L1 状态的容器；一个 session 可有多个并行 task（如 Team 编排中每个子 Agent 一个）。

```sql
CREATE TABLE IF NOT EXISTS memory_l1_tasks (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  run_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',

  task_key TEXT NOT NULL DEFAULT '',
  -- 任务命名空间：default / coordinator / worker:agent_xxx / synthesizer 等
  task_title TEXT NOT NULL DEFAULT '',
  task_goal TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  -- active / paused / completed / failed / cancelled / timeout / archived

  schema_version INTEGER NOT NULL DEFAULT 1,
  budget_tokens INTEGER NOT NULL DEFAULT 8192,
  used_tokens INTEGER NOT NULL DEFAULT 0,

  parent_task_id TEXT NOT NULL DEFAULT '',
  -- Team 子 Agent 的 task 可挂在 coordinator 的 task 下

  shared_with_json TEXT NOT NULL DEFAULT '[]',
  -- 字段级共享白名单：[{"field":"plan","read_by":["agent_xxx"]}]

  started_at TEXT NOT NULL,
  ended_at TEXT NOT NULL DEFAULT '',
  archived_at TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(session_id, task_key, agent_id)
);

CREATE INDEX IF NOT EXISTS idx_memory_l1_tasks_session
  ON memory_l1_tasks(session_id, status, updated_at);

CREATE INDEX IF NOT EXISTS idx_memory_l1_tasks_agent
  ON memory_l1_tasks(agent_id, status);
```

### 3.2 新增表：`memory_l1_fields`

每个 task 下的结构化字段。一行一个字段。

```sql
CREATE TABLE IF NOT EXISTS memory_l1_fields (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',

  field_path TEXT NOT NULL,
  -- 点路径：task_goal / subtasks.0.title / intermediate_results.api_payload
  field_kind TEXT NOT NULL DEFAULT 'string',
  -- string / number / boolean / json / reference / markdown
  visibility TEXT NOT NULL DEFAULT 'prompt',
  -- prompt / internal / shared
  pin_to_prompt INTEGER NOT NULL DEFAULT 1,
  -- 0 表示不渲染进 L0 prompt
  is_required INTEGER NOT NULL DEFAULT 0,

  value_text TEXT NOT NULL DEFAULT '',
  value_json TEXT NOT NULL DEFAULT '',
  value_ref TEXT NOT NULL DEFAULT '',
  -- 引用其它资源：message:msg_xxx / artifact:art_xxx / l3_fact:fact_xxx
  preview TEXT NOT NULL DEFAULT '',
  token_estimate INTEGER NOT NULL DEFAULT 0,

  source TEXT NOT NULL DEFAULT 'agent',
  -- agent / tool / user / l3_recall / l4_recall / system
  source_ref TEXT NOT NULL DEFAULT '',
  ttl_seconds INTEGER NOT NULL DEFAULT 0,
  -- 0 表示不过期
  expires_at TEXT NOT NULL DEFAULT '',

  revision INTEGER NOT NULL DEFAULT 1,
  last_read_at TEXT NOT NULL DEFAULT '',
  read_count INTEGER NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(task_id, field_path)
);

CREATE INDEX IF NOT EXISTS idx_memory_l1_fields_task
  ON memory_l1_fields(task_id, visibility, pin_to_prompt);

CREATE INDEX IF NOT EXISTS idx_memory_l1_fields_session
  ON memory_l1_fields(session_id, updated_at);
```

### 3.3 新增表：`memory_l1_field_history`

字段版本化（默认保留最近 10 个 revision，可配）。

```sql
CREATE TABLE IF NOT EXISTS memory_l1_field_history (
  id TEXT PRIMARY KEY,
  field_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  revision INTEGER NOT NULL,

  value_text TEXT NOT NULL DEFAULT '',
  value_json TEXT NOT NULL DEFAULT '',
  value_ref TEXT NOT NULL DEFAULT '',
  preview TEXT NOT NULL DEFAULT '',
  token_estimate INTEGER NOT NULL DEFAULT 0,

  changed_by TEXT NOT NULL DEFAULT '',
  -- agent_id / tool_name / user_id / system
  change_reason TEXT NOT NULL DEFAULT '',
  -- create / update / patch / merge / rollback / expire / overflow_trim
  diff_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  UNIQUE(field_id, revision)
);

CREATE INDEX IF NOT EXISTS idx_memory_l1_field_history_field
  ON memory_l1_field_history(field_id, revision DESC);
```

### 3.4 新增表：`memory_l1_schemas`（Agent / Skill 声明的期望字段）

```sql
CREATE TABLE IF NOT EXISTS memory_l1_schemas (
  id TEXT PRIMARY KEY,
  scope_type TEXT NOT NULL,
  -- agent / skill / team / global
  scope_id TEXT NOT NULL DEFAULT '',
  schema_key TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1,
  schema_json TEXT NOT NULL,
  -- JSON Schema：properties.task_goal{type:string, required:true}, ...
  description TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(scope_type, scope_id, schema_key, schema_version)
);
```

`schema_json` 示例：

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "task_goal": { "type": "string", "maxLength": 800, "required": true },
    "active_constraints": {
      "type": "array",
      "items": { "type": "string", "maxLength": 200 },
      "maxItems": 8
    },
    "subtasks": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id": { "type": "string" },
          "title": { "type": "string" },
          "status": { "type": "string", "enum": ["pending","running","done","failed"] }
        }
      },
      "maxItems": 16
    },
    "key_decisions": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "decision": { "type": "string" },
          "rationale": { "type": "string" },
          "at": { "type": "string", "format": "date-time" }
        }
      }
    },
    "open_questions": { "type": "array", "items": { "type": "string" } }
  },
  "required": ["task_goal"]
}
```

### 3.5 扩展 `agent_runtime_settings`

```sql
ALTER TABLE agent_runtime_settings ADD COLUMN l1_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agent_runtime_settings ADD COLUMN l1_budget_tokens INTEGER NOT NULL DEFAULT 8192;
ALTER TABLE agent_runtime_settings ADD COLUMN l1_field_max_tokens INTEGER NOT NULL DEFAULT 2048;
ALTER TABLE agent_runtime_settings ADD COLUMN l1_history_keep_revisions INTEGER NOT NULL DEFAULT 10;
ALTER TABLE agent_runtime_settings ADD COLUMN l1_default_schema_id TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_runtime_settings ADD COLUMN l1_archive_on_idle_minutes INTEGER NOT NULL DEFAULT 60;
```

---

## 4. Go 域模型与 Repository 接口

### 4.1 域模型 `internal/domain/memory_l1.go`

```go
package domain

type L1TaskStatus string

const (
	L1TaskActive    L1TaskStatus = "active"
	L1TaskPaused    L1TaskStatus = "paused"
	L1TaskCompleted L1TaskStatus = "completed"
	L1TaskFailed    L1TaskStatus = "failed"
	L1TaskCancelled L1TaskStatus = "cancelled"
	L1TaskTimeout   L1TaskStatus = "timeout"
	L1TaskArchived  L1TaskStatus = "archived"
)

type MemoryL1Task struct {
	ID            string
	SessionID     string
	RunID         string
	TeamID        string
	AgentID       string
	TaskKey       string
	TaskTitle     string
	TaskGoal      string
	Status        L1TaskStatus
	SchemaVersion int
	BudgetTokens  int
	UsedTokens    int
	ParentTaskID  string
	SharedWith    []L1FieldShare
	StartedAt     string
	EndedAt       string
	ArchivedAt    string
	Metadata      map[string]any
}

type L1FieldShare struct {
	Field  string   `json:"field"`
	ReadBy []string `json:"read_by"`
}

type MemoryL1Field struct {
	ID            string
	TaskID        string
	SessionID     string
	AgentID       string
	FieldPath     string
	FieldKind     string
	Visibility    string
	PinToPrompt   bool
	IsRequired    bool
	ValueText     string
	ValueJSON     string
	ValueRef      string
	Preview       string
	TokenEstimate int
	Source        string
	SourceRef     string
	TTLSeconds    int
	ExpiresAt     string
	Revision      int
	LastReadAt    string
	ReadCount     int
	Metadata      map[string]any
	CreatedAt     string
	UpdatedAt     string
}

type MemoryL1FieldHistory struct {
	ID            string
	FieldID       string
	TaskID        string
	Revision      int
	ValueText     string
	ValueJSON     string
	ValueRef      string
	Preview       string
	TokenEstimate int
	ChangedBy     string
	ChangeReason  string
	DiffJSON      string
	CreatedAt     string
}

type MemoryL1Schema struct {
	ID            string
	ScopeType     string
	ScopeID       string
	SchemaKey     string
	SchemaVersion int
	SchemaJSON    string
	Description   string
	Enabled       bool
}

type L1FieldPatch struct {
	FieldPath    string
	FieldKind    string
	Value        any
	ValueRef     string
	Visibility   string
	PinToPrompt  *bool
	TTLSeconds   *int
	Source       string
	SourceRef    string
	ChangedBy    string
	ChangeReason string
	IfRevision   *int // 乐观锁
}
```

### 4.2 Repository 接口 `internal/repository/memory_l1.go`

```go
type MemoryL1Repository interface {
	// Tasks
	CreateTask(ctx context.Context, t domain.MemoryL1Task) error
	UpdateTaskStatus(ctx context.Context, taskID string, status domain.L1TaskStatus, endedAt string) error
	GetTask(ctx context.Context, taskID string) (domain.MemoryL1Task, error)
	GetTaskByKey(ctx context.Context, sessionID, taskKey, agentID string) (domain.MemoryL1Task, error)
	ListActiveTasksBySession(ctx context.Context, sessionID string) ([]domain.MemoryL1Task, error)
	ArchiveIdleTasks(ctx context.Context, before string) (int, error)
	UpdateTaskUsedTokens(ctx context.Context, taskID string, usedTokens int) error

	// Fields
	UpsertField(ctx context.Context, f domain.MemoryL1Field, history domain.MemoryL1FieldHistory) error
	GetField(ctx context.Context, taskID, fieldPath string) (domain.MemoryL1Field, error)
	ListFieldsByTask(ctx context.Context, taskID string, includeInternal bool) ([]domain.MemoryL1Field, error)
	DeleteField(ctx context.Context, fieldID string) error
	BumpReadStat(ctx context.Context, fieldID string, atISO string) error

	// History
	ListFieldHistory(ctx context.Context, fieldID string, limit int) ([]domain.MemoryL1FieldHistory, error)
	TrimFieldHistory(ctx context.Context, fieldID string, keep int) error
	RollbackField(ctx context.Context, fieldID string, toRevision int, changedBy string) (domain.MemoryL1Field, error)

	// Schemas
	UpsertSchema(ctx context.Context, s domain.MemoryL1Schema) error
	ListSchemas(ctx context.Context, scopeType, scopeID string) ([]domain.MemoryL1Schema, error)
	GetSchema(ctx context.Context, id string) (domain.MemoryL1Schema, error)
}
```

实现要求：

- `UpsertField` 必须在事务中：写 `memory_l1_fields` + 追加 `memory_l1_field_history`，并按 `agent_runtime_settings.l1_history_keep_revisions` 截断旧 revision。
- 写入前估算新 token：`new_used = task.used_tokens - old_field.token_estimate + new_field.token_estimate`，若 > `task.budget_tokens` 返回 `ErrL1Overflow`。

---

## 5. Service 层接口

### 5.1 `MemoryL1Service`

```go
type MemoryL1Service interface {
	// Task lifecycle
	StartTask(ctx context.Context, in StartL1TaskInput) (domain.MemoryL1Task, error)
	EndTask(ctx context.Context, taskID string, status domain.L1TaskStatus) error
	GetTask(ctx context.Context, taskID string) (L1TaskView, error)
	GetTaskByKey(ctx context.Context, sessionID, taskKey, agentID string) (L1TaskView, error)
	ListActive(ctx context.Context, sessionID string) ([]L1TaskView, error)

	// Field CRUD
	GetField(ctx context.Context, taskID, fieldPath string) (domain.MemoryL1Field, error)
	SetField(ctx context.Context, taskID string, patch domain.L1FieldPatch) (domain.MemoryL1Field, error)
	PatchFields(ctx context.Context, taskID string, patches []domain.L1FieldPatch) ([]domain.MemoryL1Field, error)
	DeleteField(ctx context.Context, taskID, fieldPath string) error
	RollbackField(ctx context.Context, taskID, fieldPath string, toRevision int, by string) (domain.MemoryL1Field, error)

	// Prompt 渲染（被 L0 调用）
	RenderForPrompt(ctx context.Context, taskID string, viewerAgentID string) (L1PromptBlock, error)

	// 归档（任务结束时）
	SnapshotForEpisode(ctx context.Context, taskID string) (L1Episode, error)

	// 兜底
	ArchiveIdle(ctx context.Context, before string) (int, error)
}

type StartL1TaskInput struct {
	SessionID    string
	RunID        string
	TeamID       string
	AgentID      string
	TaskKey      string
	TaskTitle    string
	TaskGoal     string
	ParentTaskID string
	SchemaID     string
	BudgetTokens int
}

type L1TaskView struct {
	Task    domain.MemoryL1Task
	Fields  []domain.MemoryL1Field
	Schema  *domain.MemoryL1Schema
}

type L1PromptBlock struct {
	Section       string // memory.l1
	Role          string // system
	Source        string // l1:task_xxx
	Tokens        int
	Content       string // 渲染后的 markdown / yaml 文本
	Preview       string
	MissingFields []string
}

type L1Episode struct {
	TaskID   string
	Snapshot map[string]any // 完整字段树
	Schema   *domain.MemoryL1Schema
	Stats    map[string]int // {fields, revisions, tokens}
}
```

### 5.2 字段写入流程（SetField）

```text
1. GetTask(taskID)；若 status ≠ active/paused 返回错误
2. 验证 patch.FieldPath 合法（^[a-zA-Z_][a-zA-Z0-9_.]*$，长度 ≤ 256）
3. 若任务绑定 schema：用 JSON Schema 校验 patch 合法性
4. 估算 new_token = tokenize(patch.value or value_ref)
5. 若 new_token > l1_field_max_tokens 返回 ErrFieldTooLarge
6. 旧字段存在则取出 old_field；revision=old.revision+1
   - 若 patch.IfRevision != nil 且 != old.revision 返回 ErrRevisionConflict（409）
7. 计算 task_used_tokens 变化
8. 若 new_used > task.budget_tokens：
     - 若 visibility=internal 仍允许（不进 prompt）
     - 否则返回 ErrL1Overflow（建议调用方先精简 / DeleteField）
9. 事务：
     - upsert memory_l1_fields
     - insert memory_l1_field_history（reason=create/update/patch）
     - update memory_l1_tasks.used_tokens, updated_at
     - trim memory_l1_field_history 保留最近 N 个 revision
10. 写一条 audit_logs：action=l1.set_field
```

### 5.3 PromptBlock 渲染（被 L0 调用）

`RenderForPrompt(taskID, viewerAgentID)`：

1. ListFieldsByTask(taskID)，过滤：`pin_to_prompt = true` 且（`visibility = prompt` 或 `viewerAgentID` 在 `shared_with` 中）。
2. 按 `field_path` 分组组装为 markdown / yaml：
   ```markdown
   ## Working Memory（任务：实现 dark mode）
   - **task_goal**: 让首页支持暗色主题
   - **active_constraints**: 1) 不破坏现有 token；2) 通过 a11y 对比度
   - **subtasks**:
     - sub_1 设计 token 切换 [done]
     - sub_2 改造组件 [running]
   - **key_decisions**:
     - 用 CSS 变量而非 class 切换（rationale: 性能更好）
   - **open_questions**:
     - 是否同步移动端
   ```
3. 计算 token；若 > budget 触发自动折叠 internal 字段或截断 preview。
4. 若 schema 标记 required 但缺失，返回 `MissingFields`，L0 装配时可加一段 `system: 请补充字段 [task_goal, ...]`。

### 5.4 工具适配（提供给 LLM 调用）

注册到 `tools` 表的内置工具：

| tool_key | 行为 |
|----------|------|
| `working_memory.read` | 入参 `field_path?`；不传返回整个 task；传则返回单字段 |
| `working_memory.write` | 入参 `field_path`、`value`、`reason?`；调用 `SetField` |
| `working_memory.patch` | 入参 `patches: [{field_path, value, reason}]`；批量 |
| `working_memory.delete` | 入参 `field_path` |
| `working_memory.list` | 列出所有字段（path + preview + token） |

工具执行权限受 `tool.requires_confirmation` 与 `tool.risk_level` 控制；默认 `low` 风险，无需确认。

### 5.5 与 ChatService / TeamRuntime 集成

| 事件 | L1 行为 |
|------|--------|
| 单 Agent session 第一条用户消息 | StartTask(task_key='default', agent_id=session.agent_id, schema_id=agent.l1_default_schema_id) |
| Team session 编排开始 | 为 coordinator 创建 task（task_key='coordinator'）；为每个 worker 创建子 task（task_key='worker:{agent_id}', parent_task_id=coordinator） |
| 用户消息中含 `/reset` 或新主题 | EndTask(完成上一个) → StartTask（新） |
| 子 Agent step 结束 | 不立即 EndTask；任务自然结束或 idle 超时再归档 |
| Run 结束 | 遍历该 run 下所有 active task，EndTask(status=completed/failed) → SnapshotForEpisode → 写 L2 |
| Session archive | EndTask 全部，归档到 L2 |

---

## 6. HTTP API

### 6.1 配置 API（扩展 `agent_runtime_settings`）

复用 `PATCH /api/v1/agents/{id}/runtime-settings`：

```json
{
  "l1_enabled": true,
  "l1_budget_tokens": 8192,
  "l1_field_max_tokens": 2048,
  "l1_history_keep_revisions": 10,
  "l1_default_schema_id": "schema_xxx",
  "l1_archive_on_idle_minutes": 60
}
```

### 6.2 Schema 管理

```http
GET    /api/v1/memory/l1/schemas?scope_type=agent&scope_id=agent_xxx
POST   /api/v1/memory/l1/schemas
PATCH  /api/v1/memory/l1/schemas/{id}
DELETE /api/v1/memory/l1/schemas/{id}
GET    /api/v1/memory/l1/schemas/{id}
POST   /api/v1/memory/l1/schemas/{id}/duplicate
```

### 6.3 Task 与字段

```http
GET   /api/v1/sessions/{sid}/l1/tasks
POST  /api/v1/sessions/{sid}/l1/tasks
GET   /api/v1/sessions/{sid}/l1/tasks/{taskId}
PATCH /api/v1/sessions/{sid}/l1/tasks/{taskId}        # 修改 status / shared_with / budget
DELETE /api/v1/sessions/{sid}/l1/tasks/{taskId}       # 软归档

GET   /api/v1/sessions/{sid}/l1/tasks/{taskId}/fields
GET   /api/v1/sessions/{sid}/l1/tasks/{taskId}/fields/{path}
PUT   /api/v1/sessions/{sid}/l1/tasks/{taskId}/fields/{path}    # set
PATCH /api/v1/sessions/{sid}/l1/tasks/{taskId}/fields           # batch patch
DELETE /api/v1/sessions/{sid}/l1/tasks/{taskId}/fields/{path}

GET   /api/v1/sessions/{sid}/l1/tasks/{taskId}/fields/{path}/history?limit=20
POST  /api/v1/sessions/{sid}/l1/tasks/{taskId}/fields/{path}/rollback {"to_revision": 7}

POST  /api/v1/sessions/{sid}/l1/tasks/{taskId}/render-prompt?viewer_agent_id=agent_xxx
```

### 6.4 与 Trace 的关系

每次工具触发的 L1 写入都会产生：

- `session_trace_spans` 中 `span_type = tool_call` 子节点（`tool_name = working_memory.write`）；
- 同时 `metadata_json.l1_field_id` 引用本次写入的 field。

---

## 7. 与现有 aranea 模块对接

| 模块 | 改造点 |
|------|--------|
| `internal/domain/models.go` | 增加 §4.1 类型 |
| `internal/repository/sqlite.go` | `Migrate()` 中执行 §3.1～§3.5 DDL；`ensureLegacyColumns` 补 ALTER |
| `internal/repository/sqlite_memory_l1.go`（新） | 实现 §4.2 Repository |
| `internal/service/memory_l1_service.go`（新） | 实现 §5.1 Service |
| `internal/service/chat_service.go` | StartTask / EndTask 钩子 |
| `internal/service/team_runtime.go` | 多子 Agent 各自 StartTask + EndTask |
| `internal/runtime/adk_builtin_plugins.go`（已有） | 注册 `working_memory.*` 工具，Agent 默认装载 |
| `internal/transport/sessions.go` | 暴露 §6.3 接口 |
| `internal/transport/agents.go` | 暴露 §6.2 schema 管理接口 |
| `internal/service/memory_l0_service.go` | RenderForPrompt 注入装配段（见 `12 ...md` §5.2 step 4） |

---

## 8. 前端展示需求（Quasar / Vue）

### 8.1 Agent 设置 → 记忆 Tab → L1 子区

| 控件 | 字段 | 类型 |
|------|------|------|
| 启用 L1 | `l1_enabled` | `QToggle` |
| 任务总预算 tokens | `l1_budget_tokens` | `QInput` 1024-32768 |
| 单字段上限 tokens | `l1_field_max_tokens` | `QInput` 256-8192 |
| 历史版本保留数 | `l1_history_keep_revisions` | `QInput` 1-50 |
| 默认 Schema | `l1_default_schema_id` | `QSelect` 来自 §6.2 |
| 闲置归档分钟 | `l1_archive_on_idle_minutes` | `QInput` 5-1440 |

### 8.2 Session 详情 → 工作记忆 Tab

布局：左侧 task 列表，右侧字段编辑器。

| 区域 | Quasar 组件 |
|------|------------|
| Task 列表 | `QList` + `QItem`：title、status chip、token 进度条、agent 头像 |
| 字段树 | `QTree` 按 `field_path` 点路径分组；叶子节点显示 preview、token、source chip |
| 字段编辑 | `QInput` / `QInput type="textarea"` / `QInput type="number"` / `QSelect`（按 schema 类型）|
| Schema 提示 | 顶部 `QBanner` 显示缺失 required 字段 |
| 历史版本 | `QExpansionItem` 展开 revision 列表，diff 用 `<pre>` 双栏显示 |
| Token 预算 | `QLinearProgress`，颜色：< 60% 绿 / 60-80% 橙 / > 80% 红 |
| 操作 | 「保存」、「批量 patch」、「回滚到 vN」、「导出 JSON」、「归档任务」 |

### 8.3 Schema 管理页 `/memory/l1/schemas`

| 区域 | 内容 |
|------|------|
| 列表 | 按 scope_type 分组 (agent/skill/team/global)；显示 name、scope、字段数、最近修改 |
| 编辑器 | Monaco / `QInput textarea` 编辑 JSON Schema；右侧实时预览渲染样式 |
| 测试 | 「测试」按钮：粘贴一份样例 JSON，验证是否符合 schema |

### 8.4 Trace 详情中的 L1

`session_trace_spans.tool_call` 节点若 `tool_name` 以 `working_memory.` 开头，节点详情显示：

- 操作类型（write/patch/delete）；
- 字段路径；
- 旧值 → 新值 diff；
- 跳转「字段历史」按钮，回到 §8.2 历史版本视图。

---

## 9. 写入与读取策略

| 场景 | 行为 |
|------|------|
| LLM 工具调用写入 | `working_memory.write` 走 `MemoryL1Service.SetField`，自动写历史 |
| 用户在前端手动编辑 | 同上，但 `changed_by = user:xxx`，`reason = manual_edit` |
| L0 装配读取 | `RenderForPrompt`，按 `pin_to_prompt + visibility + shared_with` 过滤 |
| Team coordinator 共享计划 | 在 task `shared_with_json` 中加入 `[{"field":"plan","read_by":["agent_worker_a","agent_worker_b"]}]` |
| 字段过期 | `expires_at < now`：标记 `visibility=internal`，写一条 reason=expire 的 history |
| 任务结束 | `EndTask` → `SnapshotForEpisode` → L2 `memory_episodes`（见 14 文档） |
| Overflow | 写入失败返回 409 + ErrL1Overflow；前端弹窗提示「请精简或归档」 |

---

## 10. 观测与治理

- 每次 `SetField` 写 `audit_logs`：action=l1.set_field、resource=memory_l1_field、resource_id=field.id、detail={agent,task,path,reason,old_rev,new_rev}。
- 每次 task EndTask 写 `audit_logs`：action=l1.end_task、detail={status,used_tokens,fields,revisions}。
- Datadog 指标：
  - `aranea.memory.l1.field_writes_total{agent,reason}`
  - `aranea.memory.l1.task_active_count{session}`
  - `aranea.memory.l1.overflow_total`
  - `aranea.memory.l1.render_latency_ms` P50/P95/P99
- 隐私：默认所有字段均按 PII 规则过滤后写入；含 PII 的字段必须 `visibility=internal`，否则前端禁止 `pin_to_prompt`。

---

## 11. 落地实施阶段

### Phase 1（最小可用，1～2 周）

- [ ] §3.1～§3.3 三表落库 + ensureLegacyColumns 兼容。
- [ ] `MemoryL1Service.{StartTask, EndTask, GetField, SetField, ListFieldsByTask}`。
- [ ] `working_memory.read/write/list` 工具注册。
- [ ] `ChatService` 在用户首条消息时自动 StartTask。
- [ ] L0 `RenderForPrompt` 接入。
- [ ] 单元测试：overflow / revision 冲突 / 渲染。

### Phase 2（Schema 与 Team，1 周）

- [ ] §3.4 schemas 表 + §6.2 schema 管理接口。
- [ ] JSON Schema 校验集成（`github.com/santhosh-tekuri/jsonschema/v5`）。
- [ ] `TeamRuntime` 多子 task 创建与 shared_with。
- [ ] 缺失字段提示注入 prompt。

### Phase 3（前端与历史，1 周）

- [ ] §8.2 工作记忆 Tab。
- [ ] 字段历史 + 回滚。
- [ ] §8.3 Schema 管理页。
- [ ] §8.4 Trace 详情 L1 节点。

### Phase 4（治理与扩展）

- [ ] TTL 过期定时任务（cron 1 分钟一次）。
- [ ] Idle 任务归档（cron 10 分钟一次）。
- [ ] 与 L2 episode 落库联调。
- [ ] L1 字段升档 L3 的策略（在 L3 文档定义）。

---

## 12. 验收标准

- [ ] Agent 在第一条用户消息时自动创建 `memory_l1_tasks` 记录，状态 `active`。
- [ ] LLM 调用 `working_memory.write` 工具后，`memory_l1_fields` 出现新字段，`memory_l1_field_history` 出现 revision=1。
- [ ] 重复写同一 path 时，revision 自增，旧版本可在「历史版本」查看。
- [ ] 写入超过 `l1_field_max_tokens` 时，API 返回 422 `ErrFieldTooLarge`。
- [ ] 写入超过 `l1_budget_tokens` 时，API 返回 409 `ErrL1Overflow`。
- [ ] 下一次模型调用 prompt 中包含 L1 渲染段（system role），段落 token ≤ task.used_tokens。
- [ ] 关闭 `pin_to_prompt` 的字段不出现在 prompt 中，但仍可被工具读取。
- [ ] Team 编排时，子 Agent 创建独立 task，相互不可读取（除非在 `shared_with` 中）。
- [ ] Run 结束时，task 状态变为 `completed`，并出现一条 L2 `memory_episodes` 引用。
- [ ] 字段 `expires_at < now` 后，下一次渲染时该字段不出现在 prompt 中，并产生一条 reason=expire 的 history。
- [ ] 前端 Session 详情 → 工作记忆 Tab 能编辑、回滚、导出。

---

## 13. 关键设计原则

1. **结构化优于自由文本**：L1 字段必须有 path / kind，避免「再造一份对话日志」。
2. **任务粒度而非会话粒度**：一个 session 可以并行多个任务，多个 task 互不污染。
3. **写时校验，读时不校验**：schema 校验放写入路径，读取尽量快。
4. **乐观锁，禁止覆盖丢失**：`If-Revision` 头确保多 Agent 并发写时检测冲突。
5. **字段共享显式声明**：默认私有，避免「coordinator 写错被 worker 跟错」。
6. **任务结束即归档，不残留**：active task 不能无限累加；Idle 超时强制 archive。
7. **L1 是 L2 的入口**：所有进入 L2 episode 的「任务级摘要」必须先经过 L1，避免重复造轮子。
