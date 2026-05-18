# Session 历史存储与编排设计

本文档设计 **会话历史存储（Session History）**，覆盖数据库表、后端 session 模块、前端展示界面，以及 session 在 **Agent / Team 编排**中的核心作用。目标不是只保存聊天记录，而是把一次用户任务从输入、编排、模型调用、上下文窗口消耗到最终结果都串成可回放、可分析、可治理的运行实例。

> **框架对齐**：本设计遵循 `AI-DEVELOPMENT-SPECIFICATION.md` 分层规范和 `plan.md` M5 Session 管理模块，逐步向 trpc-agent-go `session.Service` 对齐。当前阶段以 SQLite + Ent 为主存储，后续可桥接 trpc session.Service 多后端（Redis/PG/MySQL）。

---

## 0. 会话历史追踪弹窗（实施范围）

### 0.1 用户体验

在 Chat 右侧 Session 列表中，每条会话的更多菜单新增 **历史追踪**。点击后弹出 Quasar `QDialog`，内部使用 `QTimeline` 纵向时间轴展示整个会话链路：

| 区域 | 内容 |
|------|------|
| 顶部 | 会话标题、事件数量、消息/工具/Skill/MCP 统计 |
| 中轴 | `QTimeline` 时间轴，按事件发生时间排序 |
| 左侧 | 对话内容：用户消息、Agent 消息、Team 成员消息，默认折叠预览，点击展开完整 Markdown |
| 右侧 | 外部调用：Tool、Skill、MCP 等，显示调用标签、状态、耗时、输入输出摘要和错误 |

外部调用使用不同标签：

| 类型 | 标签 | 颜色建议 |
|------|------|----------|
| Tool | `Tool` | primary / info |
| Skill | `Skill` | deep-purple |
| MCP | `MCP` | teal |
| Message | `User` / `Agent` / `Team` | grey / primary |

### 0.2 后端 API

新增：

```http
GET /api/v1/sessions/{session_id}/timeline
```

返回：

```json
{
  "session_id": "xxx",
  "items": [
    {
      "id": "message-id",
      "kind": "message",
      "side": "left",
      "title": "用户消息",
      "subtitle": "user",
      "status": "ok",
      "occurred_at": "2026-04-26T09:00:00Z",
      "duration_ms": 0,
      "content_markdown": "消息内容",
      "preview": "消息内容摘要",
      "tags": ["User"]
    },
    {
      "id": "tool-inv-id",
      "kind": "tool",
      "side": "right",
      "title": "读取文件",
      "subtitle": "read_file",
      "status": "success",
      "occurred_at": "2026-04-26T09:00:01Z",
      "duration_ms": 34,
      "preview": "{\"path\":\"README.md\"}",
      "detail_json": "{\"input\":...,\"output\":...}",
      "tags": ["Tool"]
    }
  ],
  "summary": {
    "total": 8,
    "message_count": 4,
    "tool_count": 2,
    "skill_count": 1,
    "mcp_count": 1
  }
}
```

### 0.3 当前实施策略

第一版直接聚合已有表：

| 来源表 | 时间字段 | 映射 |
|--------|----------|------|
| `messages` | `created_at` | `kind=message`，左侧 |
| `tool_invocations` | `started_at / created_at` | `kind=tool` 或 `kind=mcp`，右侧 |
| `skill_invocation` | `started_at / created_at` | `kind=skill`，右侧 |

MCP 当前没有独立调用表时，先通过 `tool_invocations.source == "mcp"` 或 `tool_key` 包含 `mcp` 归类为 `kind=mcp`；后续如果增加 MCP invocation 表，只需在 timeline 聚合服务增加一个来源。

---

## 1. 核心目标

| 目标 | 说明 |
|------|------|
| 追踪历史会话 | 支持按时间、Agent、Team、状态、模型、上下文消耗比例检索历史 session |
| 查看会话属性 | 展示发生时间、最后活跃时间、消息数、调用次数、Token/费用、context window 消耗比例 |
| 完整追踪链路 | 每轮对话都能看到模型、工具、Skill、MCP 的调用名称、耗时、token、状态、输入输出与错误 |
| 区分 Team / 单 Agent | 单 Agent session 只绑定 `agent_id`；Team session 绑定 `team_id`，并记录参与编排的多个 Agent |
| 支撑 Agent 编排 | Session 是编排运行的主索引，所有 run、step、message、tool call、token usage、trace 都通过 `session_id` 串联 |
| 支持问题复盘 | 能回答「哪个 Agent 在什么时候消耗了多少上下文」「Team 编排卡在哪一步」「失败来自模型、工具还是子 Agent」 |
| 支持上下文治理 | 当 context 消耗过高时触发摘要、归档、裁剪、分支或新 session 建议 |

---

## 2. Session 在编排中的作用

Session 是一次持续交互或任务执行的 **上下文容器**，也是编排层的 **运行边界**。

| 角色 | 作用 |
|------|------|
| 上下文容器 | 保存用户输入、assistant 回复、系统摘要、附件、工具结果、子 Agent 输出 |
| 编排主索引 | Team 编排中的 planner、worker、reviewer、tool step 都挂在同一个 `session_id` 下 |
| 状态机 | 记录 `active`、`running`、`completed`、`failed`、`archived`、`deleted` 等状态 |
| 指标聚合点 | 聚合上下文窗口消耗、Token、费用、模型调用次数、延迟、错误率 |
| 权限与归属边界 | 区分 workspace、user、agent、team，避免 Team 会话与个人 Agent 会话混用 |
| 可回放单元 | 复盘时按 session 加载 run timeline、消息流、模型调用流水和事件轨迹 |

Team session 与单 Agent session 的主要差异：

| 维度 | 单 Agent Session | Team Session |
|------|------------------|--------------|
| 归属 | `owner_type = 'agent'`，`agent_id` 必填 | `owner_type = 'team'`，`team_id` 必填 |
| 参与者 | 一个主 Agent | 多个 Agent，可有 planner / executor / reviewer 等角色 |
| 消息流 | 用户与单 Agent 对话为主 | 用户、Team coordinator、子 Agent、工具事件混合 |
| 上下文窗口 | 主要看当前模型上下文 | 既看 Team 总上下文，也看每个参与 Agent 的上下文 |
| 编排记录 | 可没有 run/step，普通聊天即可 | 必须记录 run、step、agent handoff、tool call |

---

## 3. 数据模型总览

建议将 session 数据分为四层：

| 层级 | 表 | 说明 |
|------|----|------|
| 会话主表 | `sessions` | 一条 session 的归属、标题、状态、时间、上下文消耗摘要 |
| 内容层 | `messages`、`session_turns`、`chat_attachments` | 用户、assistant、system、tool、agent 产出的内容与每轮对话指标 |
| 编排层 | `session_runs`、`session_run_steps`、`session_trace_spans`、`session_participants` | Team / Agent 编排运行、步骤、调用链路、参与者 |
| 指标层 | `session_context_snapshots`、`session_model_summaries`、`model_token_usage_events` | 上下文窗口、多模型分布、Token、费用、延迟、错误等指标 |

现有代码中已经有 `sessions`、`messages`、`model_token_usage_events`，并且 `ChatService` 会更新 `context_used_ratio`。本设计是在现有基础上扩展，让 session 从聊天列表升级为编排历史中心。

> **trpc-agent-go 对齐路径**：trpc 框架提供 `session.Service` + 多后端（SQLite/Redis/PG/MySQL/ClickHouse）+ 内置摘要压缩。当前项目使用 Ent + SQLite 自管理 session 存储，后续通过适配器桥接到 trpc `session.Service` 接口，实现：
> 1. `internal/session/trpc/service.go` — 桥接 Ent session 到 trpc session.Service
> 2. `internal/session/trpc/redis.go` — 生产环境 Redis 后端
> 3. 逐步将 `internal/compress` 摘要逻辑迁移到 trpc 内置压缩

---

## 4. 数据库表设计

### 4.1 会话主表：`sessions`

`sessions` 是查询列表和详情页的入口。一条记录代表一次用户任务或持续对话。

```sql
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,

  -- 归属
  workspace_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',
  owner_type TEXT NOT NULL DEFAULT 'agent',
  -- agent / team
  agent_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',

  -- 展示
  title TEXT NOT NULL,
  summary TEXT NOT NULL DEFAULT '',
  tags_json TEXT NOT NULL DEFAULT '[]',

  -- 会话默认配置快照：用于新消息默认值，不代表整个会话只使用这一个模型
  dialog_mode TEXT NOT NULL DEFAULT '',
  default_provider TEXT NOT NULL DEFAULT '',
  default_model TEXT NOT NULL DEFAULT '',
  default_context_window_tokens INTEGER NOT NULL DEFAULT 0,

  -- 最近一次模型调用快照：用于列表快速展示，事实源仍是 usage / step 明细
  last_provider TEXT NOT NULL DEFAULT '',
  last_model TEXT NOT NULL DEFAULT '',
  last_context_window_tokens INTEGER NOT NULL DEFAULT 0,

  -- 当前状态
  status TEXT NOT NULL DEFAULT 'active',
  -- active / running / completed / failed / archived / deleted
  visibility TEXT NOT NULL DEFAULT 'private',
  -- private / team / workspace

  -- 聚合指标
  message_count INTEGER NOT NULL DEFAULT 0,
  run_count INTEGER NOT NULL DEFAULT 0,
  model_call_count INTEGER NOT NULL DEFAULT 0,
  tool_call_count INTEGER NOT NULL DEFAULT 0,
  skill_call_count INTEGER NOT NULL DEFAULT 0,
  mcp_call_count INTEGER NOT NULL DEFAULT 0,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  avg_latency_ms REAL NOT NULL DEFAULT 0,
  error_count INTEGER NOT NULL DEFAULT 0,

  -- 上下文窗口消耗
  context_used_tokens INTEGER NOT NULL DEFAULT 0,
  context_used_ratio REAL NOT NULL DEFAULT 0,
  max_context_used_ratio REAL NOT NULL DEFAULT 0,
  context_status TEXT NOT NULL DEFAULT 'normal',
  -- normal / warning / critical / exceeded

  -- 时间
  first_message_at TEXT NOT NULL DEFAULT '',
  last_message_at TEXT NOT NULL DEFAULT '',
  last_run_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  archived_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT '',

  -- Runner 快照：序列化的 trpc-agent-go Runner 会话状态（events + KV state），用于 Runner 恢复与压缩重写
  runner_snapshot_json TEXT NOT NULL DEFAULT '',

  -- 扩展
  metadata_json TEXT NOT NULL DEFAULT '{}',

  CHECK (
    (owner_type = 'agent' AND agent_id <> '' AND team_id = '')
    OR
    (owner_type = 'team' AND team_id <> '')
  )
);
```

推荐索引：

```sql
CREATE INDEX IF NOT EXISTS idx_sessions_owner_time
  ON sessions(owner_type, agent_id, team_id, deleted_at, COALESCE(NULLIF(last_message_at, ''), updated_at));

CREATE INDEX IF NOT EXISTS idx_sessions_workspace_time
  ON sessions(workspace_id, deleted_at, updated_at);

CREATE INDEX IF NOT EXISTS idx_sessions_context
  ON sessions(context_status, context_used_ratio);

CREATE INDEX IF NOT EXISTS idx_sessions_status_time
  ON sessions(status, updated_at);
```

字段说明：

| 字段 | 说明 |
|------|------|
| `owner_type` | 区分 `agent` 与 `team`，这是前后端筛选和权限判断的第一入口 |
| `default_provider` / `default_model` | 创建会话时的默认模型，只作为下一次发送消息的默认选择 |
| `last_provider` / `last_model` | 最近一次实际调用的模型，便于列表展示；完整模型历史从 `model_token_usage_events` 查询 |
| `context_used_ratio` | 当前上下文消耗比例，建议使用最近一次模型调用的 prompt tokens / context window |
| `max_context_used_ratio` | 会话历史最高消耗比例，用于发现曾经接近爆窗的 session |
| `context_status` | 前端可直接映射颜色：normal 绿、warning 橙、critical 红、exceeded 紫/红 |
| `summary` | 后端异步生成的会话摘要，用于历史列表和归档后快速理解 |
| `runner_snapshot_json` | trpc-agent-go Runner 会话序列化状态（events + KV state），用于 Runner 恢复和压缩重写；空值表示纯 native chat |
| `metadata_json` | 保存 UI 扩展、入口来源、编排策略版本等非核心字段 |

---

### 4.2 会话参与者：`session_participants`

Team session 中必须知道哪些 Agent 参与过，以及它们在编排中扮演什么角色。单 Agent session 也可以写一条参与者记录，便于统一查询。

```sql
CREATE TABLE IF NOT EXISTS session_participants (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,

  participant_type TEXT NOT NULL DEFAULT 'agent',
  -- user / agent / team / tool / system
  participant_id TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',

  role_in_session TEXT NOT NULL DEFAULT '',
  -- owner / coordinator / planner / executor / reviewer / observer / tool
  status TEXT NOT NULL DEFAULT 'active',
  -- active / left / archived

  first_active_at TEXT NOT NULL DEFAULT '',
  last_active_at TEXT NOT NULL DEFAULT '',
  message_count INTEGER NOT NULL DEFAULT 0,
  run_step_count INTEGER NOT NULL DEFAULT 0,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  context_used_ratio REAL NOT NULL DEFAULT 0,

  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,

  UNIQUE(session_id, participant_type, participant_id, role_in_session)
);
```

使用场景：

| 场景 | 说明 |
|------|------|
| Team 详情页 | 展示参与 Agent 列表、角色、贡献消息数、Token 消耗 |
| 编排复盘 | 判断 planner / executor / reviewer 哪个环节耗时或失败 |
| 成本分摊 | Team session 总成本可按参与 Agent 拆分 |

---

### 4.3 编排运行：`session_runs`

一次 session 可以有多轮 run。普通聊天通常一条用户消息对应一次 run；Team 编排可能一次 run 内包含多个 step 和多个 Agent。

```sql
CREATE TABLE IF NOT EXISTS session_runs (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,

  run_type TEXT NOT NULL DEFAULT 'chat',
  -- chat / team_orchestration / tool_workflow / background_task / summary
  trigger_type TEXT NOT NULL DEFAULT 'user_message',
  -- user_message / retry / schedule / webhook / system
  trigger_message_id TEXT NOT NULL DEFAULT '',

  owner_type TEXT NOT NULL DEFAULT 'agent',
  agent_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  coordinator_agent_id TEXT NOT NULL DEFAULT '',

  status TEXT NOT NULL DEFAULT 'running',
  -- queued / running / success / failed / cancelled / partial
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',

  started_at TEXT NOT NULL,
  ended_at TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0,

  step_count INTEGER NOT NULL DEFAULT 0,
  model_call_count INTEGER NOT NULL DEFAULT 0,
  tool_call_count INTEGER NOT NULL DEFAULT 0,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,

  plan_json TEXT NOT NULL DEFAULT '{}',
  result_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

`session_runs` 解决的问题：

| 问题 | 通过 run 回答 |
|------|---------------|
| 这次用户请求触发了哪些编排动作？ | 查 `session_runs` + `session_run_steps` |
| Team 编排是否完成？ | 看 `status`、`ended_at`、`error_message` |
| 一次任务用了多少模型调用和工具调用？ | 看 `model_call_count`、`tool_call_count` |
| 是否由重试或定时任务触发？ | 看 `trigger_type` |

---

### 4.4 编排步骤：`session_run_steps`

每个 step 是可观察的最小编排单元，例如 planner 产出计划、executor 调用工具、reviewer 复核、某个子 Agent 生成中间结果。

```sql
CREATE TABLE IF NOT EXISTS session_run_steps (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  run_id TEXT NOT NULL,

  parent_step_id TEXT NOT NULL DEFAULT '',
  step_index INTEGER NOT NULL DEFAULT 0,
  depth INTEGER NOT NULL DEFAULT 0,

  step_type TEXT NOT NULL,
  -- model_call / tool_call / skill_call / mcp_call / agent_handoff / memory_recall / summary / guardrail / final_response
  actor_type TEXT NOT NULL DEFAULT 'agent',
  -- agent / team / tool / system
  actor_id TEXT NOT NULL DEFAULT '',
  actor_name TEXT NOT NULL DEFAULT '',

  title TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'running',
  -- queued / running / success / failed / skipped / cancelled

  input_message_id TEXT NOT NULL DEFAULT '',
  output_message_id TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  tool_type TEXT NOT NULL DEFAULT '',
  -- builtin_tool / skill / mcp / external_api
  tool_name TEXT NOT NULL DEFAULT '',
  mcp_server_name TEXT NOT NULL DEFAULT '',
  skill_name TEXT NOT NULL DEFAULT '',

  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  context_window_tokens INTEGER NOT NULL DEFAULT 0,
  context_used_tokens INTEGER NOT NULL DEFAULT 0,
  context_used_ratio REAL NOT NULL DEFAULT 0,
  latency_ms INTEGER NOT NULL DEFAULT 0,
  cost_micro_usd INTEGER NOT NULL DEFAULT 0,

  started_at TEXT NOT NULL,
  ended_at TEXT NOT NULL DEFAULT '',
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',

  input_json TEXT NOT NULL DEFAULT '{}',
  output_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

推荐索引：

```sql
CREATE INDEX IF NOT EXISTS idx_session_run_steps_run
  ON session_run_steps(run_id, step_index);

CREATE INDEX IF NOT EXISTS idx_session_run_steps_session
  ON session_run_steps(session_id, created_at);

CREATE INDEX IF NOT EXISTS idx_session_run_steps_actor
  ON session_run_steps(actor_type, actor_id, created_at);
```

---

### 4.5 每轮对话：`session_turns`

`session_turns` 表示用户视角的一轮对话：一次用户输入开始，到 AI 给出最终可见回复结束。它用于快速回答「这一轮对话花了多久、用了多少 token、调用了哪些工具/Skill/MCP、是否成功」。

```sql
CREATE TABLE IF NOT EXISTS session_turns (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  run_id TEXT NOT NULL DEFAULT '',

  turn_index INTEGER NOT NULL DEFAULT 0,
  user_message_id TEXT NOT NULL DEFAULT '',
  assistant_message_id TEXT NOT NULL DEFAULT '',

  owner_type TEXT NOT NULL DEFAULT 'agent',
  agent_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',

  status TEXT NOT NULL DEFAULT 'running',
  -- running / success / failed / cancelled / partial
  started_at TEXT NOT NULL,
  ended_at TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0,
  first_token_ms INTEGER NOT NULL DEFAULT 0,

  model_call_count INTEGER NOT NULL DEFAULT 0,
  tool_call_count INTEGER NOT NULL DEFAULT 0,
  skill_call_count INTEGER NOT NULL DEFAULT 0,
  mcp_call_count INTEGER NOT NULL DEFAULT 0,

  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,

  final_provider TEXT NOT NULL DEFAULT '',
  final_model TEXT NOT NULL DEFAULT '',
  final_content_preview TEXT NOT NULL DEFAULT '',

  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

推荐索引：

```sql
CREATE INDEX IF NOT EXISTS idx_session_turns_session
  ON session_turns(session_id, turn_index);

CREATE INDEX IF NOT EXISTS idx_session_turns_status_time
  ON session_turns(status, started_at);
```

写入规则：

| 时机 | 行为 |
|------|------|
| 用户消息落库后 | 创建 turn，记录 `started_at`、`user_message_id` |
| 首个模型 token 返回 | 更新 `first_token_ms` |
| AI 最终回复落库后 | 写入 `assistant_message_id`、`final_content_preview`、`ended_at` |
| 子调用完成后 | 从 trace span 聚合模型/工具/Skill/MCP 次数、token、费用、耗时 |
| 异常结束 | 标记 `failed/cancelled/partial`，保留错误信息和已完成子调用 |

---

### 4.6 完整追踪链路：`session_trace_spans`

`session_trace_spans` 是 session 追踪链路的事实表。每条 span 表示一次可观察调用或动作：用户输入、AI 模型调用、工具调用、Skill 执行、MCP tool 调用、Agent handoff、最终回复等。通过 `parent_span_id` 构成树，前端可展示完整调用链。

```sql
CREATE TABLE IF NOT EXISTS session_trace_spans (
  id TEXT PRIMARY KEY,
  trace_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  run_id TEXT NOT NULL DEFAULT '',
  turn_id TEXT NOT NULL DEFAULT '',

  parent_span_id TEXT NOT NULL DEFAULT '',
  span_index INTEGER NOT NULL DEFAULT 0,
  depth INTEGER NOT NULL DEFAULT 0,

  span_type TEXT NOT NULL,
  -- user_message / ai_response / model_call / tool_call / skill_call / mcp_call / agent_handoff / memory_recall / summary / guardrail
  name TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',

  actor_type TEXT NOT NULL DEFAULT '',
  -- user / agent / team / tool / skill / mcp / system
  actor_id TEXT NOT NULL DEFAULT '',
  actor_name TEXT NOT NULL DEFAULT '',

  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  tool_name TEXT NOT NULL DEFAULT '',
  skill_name TEXT NOT NULL DEFAULT '',
  mcp_server_name TEXT NOT NULL DEFAULT '',
  mcp_tool_name TEXT NOT NULL DEFAULT '',

  status TEXT NOT NULL DEFAULT 'running',
  -- running / success / failed / cancelled / skipped / timeout
  started_at TEXT NOT NULL,
  ended_at TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0,
  first_token_ms INTEGER NOT NULL DEFAULT 0,

  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  context_window_tokens INTEGER NOT NULL DEFAULT 0,
  context_used_tokens INTEGER NOT NULL DEFAULT 0,
  context_used_ratio REAL NOT NULL DEFAULT 0,
  cost_micro_usd INTEGER NOT NULL DEFAULT 0,

  input_message_id TEXT NOT NULL DEFAULT '',
  output_message_id TEXT NOT NULL DEFAULT '',
  input_preview TEXT NOT NULL DEFAULT '',
  output_preview TEXT NOT NULL DEFAULT '',

  input_json TEXT NOT NULL DEFAULT '{}',
  output_json TEXT NOT NULL DEFAULT '{}',
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

推荐索引：

```sql
CREATE INDEX IF NOT EXISTS idx_session_trace_spans_trace
  ON session_trace_spans(trace_id, span_index);

CREATE INDEX IF NOT EXISTS idx_session_trace_spans_session
  ON session_trace_spans(session_id, started_at);

CREATE INDEX IF NOT EXISTS idx_session_trace_spans_turn
  ON session_trace_spans(turn_id, span_index);

CREATE INDEX IF NOT EXISTS idx_session_trace_spans_type_status
  ON session_trace_spans(span_type, status, started_at);
```

Span 类型与记录内容：

| `span_type` | 必填名称字段 | 指标 | 输出 |
|-------------|--------------|------|------|
| `user_message` | `name = user.message` | `duration_ms` 通常为 0 | `output_message_id` 指向用户消息 |
| `model_call` | `provider`、`model` | input/output tokens、首 token 时间、总耗时、费用、context ratio | 模型原始输出或 assistant 草稿 |
| `ai_response` | `name = ai.response` | 最终生成耗时、token 聚合 | `output_message_id` 指向最终 AI 回复 |
| `tool_call` | `tool_name` | 工具耗时、状态、错误 | 工具结果摘要与完整 JSON |
| `skill_call` | `skill_name` | Skill 耗时、状态、错误 | Skill 输出摘要与完整 JSON |
| `mcp_call` | `mcp_server_name`、`mcp_tool_name` | MCP 调用耗时、状态、错误 | MCP 返回摘要与完整 JSON |
| `agent_handoff` | `actor_name` | handoff 耗时 | 交接原因、上下文摘要 |
| `summary` | `name = context.summary` | 摘要前后 token | 摘要内容 |

存储原则：

| 原则 | 说明 |
|------|------|
| 链路可重建 | 同一轮对话使用同一个 `trace_id`，通过 `parent_span_id` 还原树 |
| 明细可审计 | 每个工具/Skill/MCP 调用都记录名称、输入摘要、输出摘要、状态、耗时、错误 |
| 内容可查看 | AI 最终回复写入 `messages`，span 保存 `output_message_id` 和 preview |
| 大内容分层 | `input_preview/output_preview` 用于列表，完整内容放 `input_json/output_json` 或 message/attachment |
| 失败不丢链路 | 失败 span 仍落库，turn/run 标记 failed 或 partial |

---

### 4.7 上下文快照：`session_context_snapshots`

`sessions.context_used_ratio` 只保存当前摘要；如果要看趋势，需要单独记录快照。

```sql
CREATE TABLE IF NOT EXISTS session_context_snapshots (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  run_id TEXT NOT NULL DEFAULT '',
  step_id TEXT NOT NULL DEFAULT '',
  message_id TEXT NOT NULL DEFAULT '',

  owner_type TEXT NOT NULL DEFAULT 'agent',
  agent_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',

  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  context_window_tokens INTEGER NOT NULL DEFAULT 0,
  used_tokens INTEGER NOT NULL DEFAULT 0,
  remaining_tokens INTEGER NOT NULL DEFAULT 0,
  used_ratio REAL NOT NULL DEFAULT 0,

  source TEXT NOT NULL DEFAULT 'model_usage',
  -- model_usage / estimate / summary / manual_recalc
  strategy TEXT NOT NULL DEFAULT '',
  -- full_history / summarized_history / truncated_history

  created_at TEXT NOT NULL,
  metadata_json TEXT NOT NULL DEFAULT '{}'
);
```

快照写入时机：

| 时机 | 写入内容 |
|------|----------|
| 模型调用成功后 | 写入 prompt tokens / context window 的真实比例 |
| 模型调用失败但拿到估算 token | 写入 `source = 'estimate'` |
| 自动摘要后 | 写入摘要前后 used tokens，对比压缩效果 |
| 裁剪上下文后 | 写入 `strategy = 'truncated_history'` |

---

### 4.6 会话内模型汇总：`session_model_summaries`

一个会话中可能多次切换模型，例如先用低成本模型做探索，再切到长上下文模型整理，最后用高质量模型生成结果。因此 `sessions` 不能把 `provider/model` 当成唯一模型，只能保存默认模型和最近模型；真正的模型使用历史来自调用明细。

`session_model_summaries` 是可重算的聚合表，用于列表、详情页和筛选提速。

```sql
CREATE TABLE IF NOT EXISTS session_model_summaries (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,

  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  model_display_name TEXT NOT NULL DEFAULT '',

  first_used_at TEXT NOT NULL DEFAULT '',
  last_used_at TEXT NOT NULL DEFAULT '',
  call_count INTEGER NOT NULL DEFAULT 0,
  success_count INTEGER NOT NULL DEFAULT 0,
  failed_count INTEGER NOT NULL DEFAULT 0,

  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  avg_latency_ms REAL NOT NULL DEFAULT 0,

  max_context_window_tokens INTEGER NOT NULL DEFAULT 0,
  max_context_used_ratio REAL NOT NULL DEFAULT 0,

  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,

  UNIQUE(session_id, provider, model)
);
```

推荐索引：

```sql
CREATE INDEX IF NOT EXISTS idx_session_model_summaries_session
  ON session_model_summaries(session_id, total_tokens DESC);

CREATE INDEX IF NOT EXISTS idx_session_model_summaries_model
  ON session_model_summaries(provider, model, last_used_at);
```

更新规则：

| 时机 | 行为 |
|------|------|
| 每次模型调用完成 | 按 `session_id + provider + model` upsert 汇总 |
| Session 聚合重算 | 从 `model_token_usage_events` 重建该 session 的所有模型汇总 |
| 列表展示 | 展示 `last_provider/last_model`，并用「+N 模型」提示还有其他模型 |
| 详情页展示 | 展示模型分布：调用次数、Token、费用、最高 context ratio |

---

### 4.7 消息表：`messages`

现有 `messages` 已可承载基础聊天记录，建议补充字段以适配 Team 编排和工具消息。

```sql
-- 在现有 messages 基础上建议增加：
ALTER TABLE messages ADD COLUMN run_id TEXT NOT NULL DEFAULT '';
ALTER TABLE messages ADD COLUMN step_id TEXT NOT NULL DEFAULT '';
ALTER TABLE messages ADD COLUMN sender_type TEXT NOT NULL DEFAULT '';
-- user / agent / tool / system
ALTER TABLE messages ADD COLUMN sender_id TEXT NOT NULL DEFAULT '';
ALTER TABLE messages ADD COLUMN content_type TEXT NOT NULL DEFAULT 'markdown';
-- markdown / json / tool_result / summary / error
ALTER TABLE messages ADD COLUMN visibility TEXT NOT NULL DEFAULT 'visible';
-- visible / collapsed / hidden / internal
```

消息角色建议：

| `role` | 用途 |
|--------|------|
| `user` | 用户输入 |
| `assistant` | 最终给用户看的 Agent 回复 |
| `system` | 系统摘要、策略调整、状态提醒 |
| `tool` | 工具调用结果，可默认折叠 |
| `agent` | Team 内部子 Agent 中间输出，可根据 visibility 展示 |

---

## 5. 后端 Session 模块设计

### 5.1 模块边界（遵循 AI-DEVELOPMENT-SPECIFICATION.md 分层规范）

> **分层铁律**：`internal/biz` 不得 import `pkg/trpc-agent-go`；`internal/service` 是框架调用的唯一桥点；`internal/data` 仅通过 `Ent()`/`Postgres()` 访问数据库。

| 模块 | 职责 | 所在层 |
|------|------|--------|
| `SessionUsecase` | Session CRUD、timeline 聚合、上下文治理、摘要触发 | biz |
| `SessionRepository` | 读写 `sessions`、turns、participants、runs、steps、trace spans、snapshots | biz 接口 / data 实现 |
| `SessionService` | proto ↔ biz 映射、Runner 装配编排 | service |
| `ChatService` | 单 Agent 对话处理，写 message、turn、trace span、usage、context snapshot | service |
| `SessionCompressor` | 上下文接近阈值时生成摘要，写 system message 和 context snapshot | service |
| `UsageService` | 维护 `model_token_usage_events`，回填 session 聚合字段 | service |
| `sessionmemory.Store` | L0-L4 记忆链读写，Runner 会话实体同步 | data |
| `trpc session.Service` 适配器 | 桥接 Ent session 到 trpc session.Service 接口（后续实现） | internal/session/trpc |

当前 `SessionUsecase` 已实现 create/search/get/rename/archive/delete/restore/update/timeline/appendChatTurn/turns/state/titleGeneration 等基础能力，建议逐步扩展为 session 历史中心：

```go
// biz 层：SessionUsecase（不 import pkg/trpc-agent-go）
type SessionUsecase struct {
    sessions       SessionRepository
    agents         AgentRepository
    teams          TeamRepository
    titleGenerator SessionTitleGenerator
}

// SessionRepository 接口定义在 biz，实现在 data
type SessionRepository interface {
    SearchSessions(ctx context.Context, q SessionSearchQuery) (SessionListResult, error)
    CreateSession(ctx context.Context, s Session) (Session, error)
    GetSessionByID(ctx context.Context, id string) (Session, error)
    UpdateSessionTitle(ctx context.Context, id, title string) (Session, error)
    UpdateSession(ctx context.Context, id string, fields SessionUpdateFields) (Session, error)
    RestoreSession(ctx context.Context, id string) (Session, error)
    ArchiveSession(ctx context.Context, id string) error
    DeleteSession(ctx context.Context, id string) error
    DeleteSessionsByAgentID(ctx context.Context, agentID string) error
    ListMessagesBySession(ctx context.Context, sessionID string) ([]ChatMessage, error)
    ListToolInvocationsBySession(ctx context.Context, sessionID string, limit int) ([]ToolInvocationView, error)
    ListSkillInvocationsBySession(ctx context.Context, sessionID string, limit int) ([]SkillInvocationView, error)
    AppendChatTurn(ctx context.Context, sessionID string, user, assistant ChatMessage) error
    AppendChatMessage(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error
    UpdateRunnerSnapshotJSON(ctx context.Context, sessionID string, snapshotJSON string) error
    UpdateSessionContextFromLLMUsage(ctx context.Context, sessionID string, promptTokens, completionTokens, contextWindow int) error
    UpdateSessionContextAfterCompression(ctx context.Context, sessionID string, estimatedPromptTokens int, contextWindow int) error
    InsertSessionSummary(ctx context.Context, row SessionSummary) error
    MaxSessionSummaryToTurn(ctx context.Context, sessionID string) (int, error)
    ListSessionSummaries(ctx context.Context, sessionID string) ([]SessionSummary, error)
    LatestSessionSummaryTime(ctx context.Context, sessionID string) (string, error)
    UpdateSessionListSummary(ctx context.Context, sessionID, summary string) error
    GetSessionState(ctx context.Context, sessionID string) (map[string]string, error)
    SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error
    CreateSessionTurn(ctx context.Context, turn SessionTurn) (SessionTurn, error)
    UpdateSessionTurn(ctx context.Context, id string, fields SessionTurnUpdateFields) (SessionTurn, error)
    ListSessionTurns(ctx context.Context, sessionID string, limit, offset int) (SessionTurnListResult, error)
    GetSessionTurn(ctx context.Context, id string) (SessionTurn, error)
    // 后续扩展：runs / steps / trace_spans / participants / context_snapshots
}
```

```go
// service 层：SessionService（proto ↔ biz 映射 + Runner 装配）
type SessionService struct {
    v1.UnimplementedSessionServiceServer
    uc *biz.SessionUsecase
}
```

### 5.2 创建 Session

单 Agent：

```json
POST /api/v1/sessions
{
  "owner_type": "agent",
  "agent_id": "agent_xxx",
  "title": "设计登录流程",
  "dialog_mode": "chat",
  "default_provider": "anthropic",
  "default_model": "claude-4.6-sonnet"
}
```

Team：

```json
POST /api/v1/sessions
{
  "owner_type": "team",
  "team_id": "team_xxx",
  "title": "生成首页重构方案",
  "dialog_mode": "team_orchestration"
}
```

创建规则：

| 规则 | 说明 |
|------|------|
| `owner_type = agent` | `agent_id` 必填，`team_id` 为空 |
| `owner_type = team` | `team_id` 必填，可选 `coordinator_agent_id` |
| 默认模型快照 | 创建时保存 `default_provider/default_model/default_context_window_tokens`，作为新消息默认值 |
| 多模型历史 | 每次实际调用的 provider/model 写入 `model_token_usage_events` 和 `session_run_steps`，并聚合到 `session_model_summaries` |
| 默认参与者 | 单 Agent 写入一个 participant；Team 写入 team coordinator 和所有初始成员 |

### 5.3 发送消息与编排流程

单 Agent 流程：

1. `POST /chat/messages` 收到 `session_id` 与用户内容。
2. 校验 `session.owner_type = agent`，且 `session.agent_id` 与请求 Agent 匹配。
3. 创建 `session_run`，`run_type = chat`。
4. 创建 `session_turn`，生成本轮 `trace_id`。
5. 写入用户 `message`，并写入 `user_message` span。
6. 根据本次请求 options、session 默认模型、agent 默认模型解析出实际 provider/model。
7. 创建 `model_call` span，调用模型，记录首 token 时间、总耗时、token、状态、错误。
8. 如果模型触发工具、Skill、MCP 调用，为每次调用创建子 span，记录名称、输入/输出、耗时和状态。
9. 写入 assistant `message`，并写入 `ai_response` span，保存最终 AI 返回内容。
10. 写入 `model_token_usage_events`、`session_context_snapshots` 和 `session_model_summaries`。
11. 聚合回填 `session_turns`：耗时、token、模型/工具/Skill/MCP 次数、最终内容预览、状态。
12. 聚合回填 `sessions`：消息数、Token、费用、`context_used_ratio`、`last_provider/last_model`、`last_message_at`。
13. 结束 run。

Team 编排流程：

1. `POST /team-runs` 或 `POST /chat/messages` 以 `owner_type = team` 的 session 触发。
2. 创建 `session_run`，`run_type = team_orchestration`。
3. Coordinator / Planner 写入 plan step。
4. 每次子 Agent 执行同时写入 `session_run_steps` 和 `session_trace_spans`，`step_type/span_type = agent_handoff` 或 `model_call`。
5. 工具调用写入 `tool_call` span；Skill 调用写入 `skill_call` span；MCP 工具调用写入 `mcp_call` span。
6. 每个调用完成后记录名称、状态、耗时、输入输出摘要、错误、token/费用。
7. Reviewer 或 Coordinator 生成最终 assistant message，并写入 `ai_response` span。
8. 聚合所有 step/span 的 Token、费用、耗时、错误和模型分布。
9. 更新 participants 的贡献指标。
10. 结束 turn/run 并更新 session 状态。

### 5.4 Context Window 计算

核心公式：

```text
context_used_ratio = prompt_tokens / context_window_tokens
```

如果 provider 返回精确 token，使用返回值；否则使用本地估算。由于一个 session 可以切换多个模型，`context_window_tokens` 必须按「本次调用的实际模型」计算，而不是只看 session 主表。优先级：

1. 本次调用模型配置中的 `context_window_k * 1000`
2. 本次消息 options 指定模型的 context window
3. session 创建时保存的 `default_context_window_tokens`
4. agent 配置中的 `context_window`
5. provider preset 的默认值

状态阈值：

| 状态 | 条件 | UI |
|------|------|----|
| `normal` | `< 60%` | 绿色 |
| `warning` | `60% - 80%` | 橙色 |
| `critical` | `80% - 95%` | 红色 |
| `exceeded` | `>= 95%` 或模型报 context length exceeded | 紫/红 + 建议新建 session 或摘要 |

Team session 的 context 有两个口径：

| 口径 | 说明 |
|------|------|
| Team 总消耗 | 本 session 下所有模型调用的最大 `used_ratio` 或最近 run 的聚合值 |
| Agent 局部消耗 | 每个 participant / step 自己的 context ratio |

前端列表展示 Team 总消耗；详情页展示每个 Agent 的局部消耗。

### 5.5 聚合更新策略

写入消息、usage、step 后，统一调用聚合函数：

```sql
UPDATE sessions
SET
  message_count = (SELECT COUNT(*) FROM messages WHERE session_id = ?),
  model_call_count = (SELECT COALESCE(SUM(call_count), 0) FROM model_token_usage_events WHERE session_id = ?),
  tool_call_count = (SELECT COUNT(*) FROM session_trace_spans WHERE session_id = ? AND span_type = 'tool_call'),
  skill_call_count = (SELECT COUNT(*) FROM session_trace_spans WHERE session_id = ? AND span_type = 'skill_call'),
  mcp_call_count = (SELECT COUNT(*) FROM session_trace_spans WHERE session_id = ? AND span_type = 'mcp_call'),
  input_tokens = (SELECT COALESCE(SUM(input_tokens), 0) FROM model_token_usage_events WHERE session_id = ?),
  output_tokens = (SELECT COALESCE(SUM(output_tokens), 0) FROM model_token_usage_events WHERE session_id = ?),
  total_tokens = (SELECT COALESCE(SUM(total_tokens), 0) FROM model_token_usage_events WHERE session_id = ?),
  total_cost_micro_usd = (SELECT COALESCE(SUM(total_cost_micro_usd), 0) FROM model_token_usage_events WHERE session_id = ?),
  context_used_ratio = ?,
  max_context_used_ratio = MAX(max_context_used_ratio, ?),
  context_status = ?,
  updated_at = ?
WHERE id = ? AND deleted_at = '';
```

高频流式场景不要每个 delta 更新 session，只在以下时机更新：

| 时机 | 是否更新 session 聚合 |
|------|----------------------|
| 用户消息落库 | 创建 turn 和 `user_message` span，更新 `message_count`、`last_message_at` |
| assistant 最终消息落库 | 写入 `ai_response` span，更新消息、时间、最终内容预览 |
| 工具/Skill/MCP 调用完成 | 更新 span 状态、耗时、输入输出和错误；必要时增量更新 turn 统计 |
| 模型 usage 完成 | 更新 token、费用、context |
| run 结束 | 更新 run_count、状态、耗时 |

---

## 6. API 设计

### 6.1 Session 列表

```http
GET /api/v1/sessions?owner_type=team&team_id=xxx&status=active&context_status=warning&page=1&page_size=20
```

响应：

```json
{
  "items": [
    {
      "id": "sess_xxx",
      "owner_type": "team",
      "team_id": "team_xxx",
      "agent_id": "",
      "title": "生成首页重构方案",
      "summary": "Team 完成了首页信息架构与组件拆分建议。",
      "status": "completed",
      "context_used_ratio": 0.72,
      "context_status": "warning",
      "default_provider": "anthropic",
      "default_model": "claude-4.6-sonnet",
      "last_provider": "openrouter",
      "last_model": "google/gemini-2.5-pro",
      "model_count": 3,
      "message_count": 18,
      "run_count": 3,
      "model_call_count": 9,
      "tool_call_count": 12,
      "skill_call_count": 2,
      "mcp_call_count": 5,
      "total_tokens": 128000,
      "total_cost_micro_usd": 32000,
      "last_message_at": "2026-04-25T08:20:00Z",
      "created_at": "2026-04-25T08:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}
```

### 6.2 Session 详情

```http
GET /api/v1/sessions/{id}
```

返回：

```json
{
  "session": {},
  "participants": [],
  "turns": [],
  "messages": [],
  "runs": [],
  "trace_spans": [],
  "context_snapshots": [],
  "usage_summary": {}
}
```

### 6.3 Run Timeline

```http
GET /api/v1/sessions/{id}/runs
GET /api/v1/session-runs/{run_id}/steps
```

### 6.4 Trace 链路

```http
GET /api/v1/sessions/{id}/trace
GET /api/v1/sessions/{id}/turns
GET /api/v1/session-turns/{turn_id}/trace
```

`/trace` 返回完整 span 列表，前端按 `parent_span_id` 组装树；`/turns` 用于按每轮对话查看耗时、token、工具/Skill/MCP 调用数和最终状态。

Trace span 示例：

```json
{
  "id": "span_model_1",
  "trace_id": "trace_turn_1",
  "session_id": "sess_xxx",
  "turn_id": "turn_1",
  "parent_span_id": "",
  "span_type": "model_call",
  "name": "model.call",
  "provider": "anthropic",
  "model": "claude-4.6-sonnet",
  "status": "success",
  "duration_ms": 8200,
  "first_token_ms": 900,
  "input_tokens": 4200,
  "output_tokens": 1300,
  "total_tokens": 5500,
  "output_preview": "这是最终方案的摘要……"
}
```

MCP 调用 span 示例：

```json
{
  "id": "span_mcp_1",
  "trace_id": "trace_turn_1",
  "session_id": "sess_xxx",
  "turn_id": "turn_1",
  "parent_span_id": "span_model_1",
  "span_type": "mcp_call",
  "name": "mcp.call",
  "mcp_server_name": "plugin-notion-workspace-notion",
  "mcp_tool_name": "search",
  "status": "success",
  "duration_ms": 640,
  "input_preview": "query=session design",
  "output_preview": "命中 3 个页面"
}
```

### 6.5 Context 快照

```http
GET /api/v1/sessions/{id}/context-snapshots
```

用于详情页绘制 context ratio 趋势线。

### 6.6 归档 / 删除

```http
POST /api/v1/sessions/{id}/archive
DELETE /api/v1/sessions/{id}
```

删除建议仍使用软删除，保留 usage 明细用于统计。真正物理清理放到后台 retention job。

---

## 7. 前端界面设计（Quasar / Vue）

### 7.1 路由与信息架构

| 路由 | 页面 | 说明 |
|------|------|------|
| `/sessions` | Session 历史 | 全局历史列表，支持 Agent / Team 过滤 |
| `/agents/:agentId/sessions` | Agent 会话历史 | 某个 Agent 的单 Agent sessions |
| `/teams/:teamId/sessions` | Team 会话历史 | 某个 Team 的编排 sessions |
| `/sessions/:sessionId` | Session 详情 | 消息、属性、运行 timeline、context 趋势 |

### 7.2 历史列表页

| 区域 | 内容 |
|------|------|
| 标题 | 「会话历史」 |
| 副标题 | 展示当前作用域，如「全部会话 / Team: Design Agents / Agent: Writer」 |
| 顶部统计 | 总会话数、活跃会话、平均上下文消耗、近 7 日 Token |
| 筛选 | 类型 `全部 / Agent / Team`、状态、上下文状态、Agent、Team、模型、时间范围、关键字 |
| 主表 | `QTable` 服务端分页 |
| 行操作 | 打开详情、继续会话、归档、删除 |

表格列：

| 列 | UI 说明 |
|----|---------|
| 会话 | 主行 title，副行 summary / session id 短码 |
| 类型 | `QChip`：Agent / Team；Team 使用不同颜色 |
| 归属 | Agent 名称或 Team 名称 |
| 上下文 | `QLinearProgress` 或 `QCircularProgress`，显示百分比与状态色 |
| 时间 | 创建时间 + 最后消息时间 |
| 消耗 | Token、费用、模型/工具/Skill/MCP 调用次数 |
| 状态 | active / running / completed / failed / archived |
| 操作 | 详情、继续、归档、删除 |

Quasar 组件映射：

| 区域 | 组件 |
|------|------|
| 页面 | `QPage` + `QCard` |
| 筛选区 | `QInput`、`QSelect`、`QDate`、`QBtnToggle` |
| KPI | `QCard` 或 `QBanner` |
| 表格 | `QTable` + server-side pagination |
| 上下文进度 | `QLinearProgress` / `QCircularProgress` |
| 状态 | `QBadge` / `QChip` |
| 删除确认 | `QDialog` |

### 7.3 Session 详情页

详情页建议左右结构：

| 区域 | 内容 |
|------|------|
| 顶部 Header | title、类型 chip、状态、创建时间、最后活跃、继续会话按钮 |
| 左侧主区 | 消息流 / 对话轮次 / Trace 链路 / 编排 Timeline Tab |
| 右侧属性栏 | 会话属性、参与 Agent、上下文消耗、Token/费用、模型信息 |
| 底部或 Tab | Turn traces、Run steps、Context snapshots、Usage events、附件 |

Tab 建议：

| Tab | 内容 |
|-----|------|
| 消息 | 用户可见消息，内部消息默认折叠 |
| 对话轮次 | 每轮对话的耗时、token、状态、模型/工具/Skill/MCP 调用数 |
| Trace 链路 | 按树或瀑布图展示 `session_trace_spans` |
| 编排 | `session_runs` + `session_run_steps` 时间线 |
| 上下文 | context ratio 趋势、摘要/裁剪事件 |
| 消耗 | 模型调用明细、Token、费用、延迟、工具/Skill/MCP 耗时 |
| 附件 | 上传文件、工具产物 |

右侧属性栏字段：

| 字段 | 展示 |
|------|------|
| 类型 | Agent Session / Team Session |
| 归属 | Agent 或 Team 名称 |
| Provider / Model | 默认模型、最近调用模型、会话内模型数量 |
| Context | 当前比例、最高比例、窗口大小、剩余 tokens |
| 时间 | 创建、首次消息、最后消息、归档 |
| 指标 | 消息数、turn 数、run 数、step 数、模型/工具/Skill/MCP 调用数、费用 |

### 7.4 Trace 链路页

Trace 链路页用于清晰查看整个 session 的调用情况。推荐提供「树形视图」和「瀑布视图」两种模式。

| 区域 | 内容 |
|------|------|
| 顶部汇总 | 总耗时、模型调用数、工具调用数、Skill 调用数、MCP 调用数、总 token、失败数 |
| 轮次列表 | 左侧按 turn 展示用户问题、状态、耗时、token、最终模型 |
| Trace 树 | 右侧按父子层级展示 user message → model call → tool/skill/mcp → ai response |
| Span 详情抽屉 | 点击节点后展示输入、输出、错误、token、耗时、上下文比例 |
| 筛选 | span 类型、状态、Agent、模型、工具/Skill/MCP 名称、失败优先 |

Trace 节点展示规则：

| 类型 | 展示 |
|------|------|
| 用户消息 | 用户输入摘要、发生时间 |
| AI 响应 | 最终返回内容、耗时、成功/失败、输出 token |
| 模型调用 | provider/model、input/output tokens、首 token、总耗时、context ratio |
| 工具调用 | tool name、耗时、状态、输入/输出摘要 |
| Skill 调用 | skill name、版本、耗时、状态、输出摘要 |
| MCP 调用 | server/tool name、耗时、状态、返回摘要 |
| Agent handoff | 来源 Agent、目标 Agent、交接原因、上下文摘要 |

Quasar 组件映射：

| 区域 | 组件 |
|------|------|
| 轮次列表 | `QList` / `QItem` / `QBadge` |
| Trace 树 | `QTree` 或自定义递归组件 |
| 瀑布图 | `QMarkupTable` + 自定义 duration bar |
| Span 详情 | `QDrawer` 或 `QDialog` |
| 输入输出 | `QExpansionItem` + `QInput type="textarea"` readonly 或代码块 |
| 状态与类型 | `QChip` / `QBadge` |

交互要求：

| 行为 | 说明 |
|------|------|
| 点击某轮对话 | 加载该 turn 下的 trace tree |
| 点击失败筛选 | 只显示 failed/timeout/cancelled span，并保留父节点路径 |
| 点击 AI 响应 | 展示最终 `messages.content_markdown`，不是只看 preview |
| 点击工具/Skill/MCP | 展示调用名称、入参、返回、耗时、错误 |
| 点击模型调用 | 展示 provider/model、token、费用、上下文窗口、原始输出摘要 |

### 7.5 Team Session 专属展示

Team session 需要突出编排结构，而不是只显示聊天气泡。

| 组件 | 内容 |
|------|------|
| Participants Panel | 每个 Agent 的头像、角色、状态、Token、context ratio |
| Timeline | planner → executor → tool → reviewer → final response |
| Step Drawer | 点击 step 查看输入、输出、错误、模型和工具参数 |
| Handoff Badge | 展示 Agent A 交给 Agent B 的原因和上下文摘要 |
| Internal Message Toggle | 「显示内部消息」开关，默认关闭 |

Timeline 节点颜色：

| 状态 | 颜色 |
|------|------|
| success | positive |
| running | primary + spinner |
| failed | negative |
| skipped | grey |
| cancelled | warning |

### 7.6 Context 消耗可视化

列表页使用简洁进度条；详情页使用趋势线和分段说明。

| 展示 | 数据 |
|------|------|
| 当前消耗 | `sessions.context_used_ratio` |
| 最高消耗 | `sessions.max_context_used_ratio` |
| 趋势线 | `session_context_snapshots.used_ratio` |
| Agent 分布 | `session_participants.context_used_ratio` |
| 模型调用点 | `model_token_usage_events.provider_code/model_api_id/context_window_k` + tokens |
| 模型分布 | `session_model_summaries` 按模型展示调用数、Token、费用、最高 context ratio |

当进入 `critical` 或 `exceeded`：

| 建议动作 | UI |
|----------|----|
| 生成摘要 | `QBanner` + 「压缩上下文」按钮 |
| 新建延续会话 | 「从摘要继续」按钮 |
| 归档旧会话 | 「归档并创建新 session」 |
| 查看高消耗消息 | 跳转到上下文详情 Tab |

---

## 8. 前端状态与 API 类型

建议前端 `Session` 类型扩展：

```ts
export type SessionOwnerType = "agent" | "team";
export type SessionStatus = "active" | "running" | "completed" | "failed" | "archived" | "deleted";
export type ContextStatus = "normal" | "warning" | "critical" | "exceeded";

export type Session = {
  id: string;
  workspace_id: string;
  user_id: string;
  owner_type: SessionOwnerType;
  agent_id: string;
  team_id: string;
  title: string;
  summary: string;
  dialog_mode: string;
  default_provider: string;
  default_model: string;
  last_provider: string;
  last_model: string;
  model_count: number;
  status: SessionStatus;
  message_count: number;
  run_count: number;
  model_call_count: number;
  tool_call_count: number;
  skill_call_count: number;
  mcp_call_count: number;
  total_tokens: number;
  total_cost_micro_usd: number;
  default_context_window_tokens: number;
  last_context_window_tokens: number;
  context_used_tokens: number;
  context_used_ratio: number;
  max_context_used_ratio: number;
  context_status: ContextStatus;
  first_message_at: string;
  last_message_at: string;
  created_at: string;
  updated_at: string;
  archived_at: string;
  deleted_at: string;
};

export type SessionTurn = {
  id: string;
  session_id: string;
  run_id: string;
  turn_index: number;
  user_message_id: string;
  assistant_message_id: string;
  status: "running" | "success" | "failed" | "cancelled" | "partial";
  started_at: string;
  ended_at: string;
  duration_ms: number;
  first_token_ms: number;
  model_call_count: number;
  tool_call_count: number;
  skill_call_count: number;
  mcp_call_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  final_provider: string;
  final_model: string;
  final_content_preview: string;
  error_message: string;
};

export type SessionTraceSpan = {
  id: string;
  trace_id: string;
  session_id: string;
  run_id: string;
  turn_id: string;
  parent_span_id: string;
  span_index: number;
  depth: number;
  span_type: "user_message" | "ai_response" | "model_call" | "tool_call" | "skill_call" | "mcp_call" | "agent_handoff" | "memory_recall" | "summary" | "guardrail";
  name: string;
  display_name: string;
  actor_type: string;
  actor_id: string;
  actor_name: string;
  provider: string;
  model: string;
  tool_name: string;
  skill_name: string;
  mcp_server_name: string;
  mcp_tool_name: string;
  status: "running" | "success" | "failed" | "cancelled" | "skipped" | "timeout";
  started_at: string;
  ended_at: string;
  duration_ms: number;
  first_token_ms: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  context_used_ratio: number;
  cost_micro_usd: number;
  input_message_id: string;
  output_message_id: string;
  input_preview: string;
  output_preview: string;
  error_code: string;
  error_message: string;
};
```

Pinia store 可拆出独立 `sessionStore`，避免继续把 Agent 聊天、Team 编排、全局历史都塞进 `appStore`：

```ts
export const useSessionStore = defineStore("sessions", {
  state: () => ({
    query: {} as SessionSearchQuery,
    items: [] as Session[],
    total: 0,
    selected: null as SessionDetail | null,
    loading: false
  }),
  actions: {
    async search() {},
    async loadDetail(id: string) {},
    async archive(id: string) {},
    async remove(id: string) {}
  }
});
```

---

## 9. 与现有代码的落地顺序

建议分阶段实现，避免一次性重构聊天链路。

### Phase 0：代码清理与命名对齐（✅ 已完成）

| 工作 | 说明 | 状态 |
|------|------|------|
| 重命名 `AdkSnapshotJSON` → `RunnerSnapshotJSON` | biz 模型、Ent schema、data 层、service 层统一重命名 | ✅ |
| 重命名 `UpdateAdkSnapshotJSON` → `UpdateRunnerSnapshotJSON` | SessionRepository 接口和实现 | ✅ |
| 重命名 `DeleteADKSessionEventEntities` → `DeleteSessionEventEntities` | sessionmemory.Store | ✅ |
| 重命名 `ADKEventEntityParams` → `EventEntityParams`、`UpsertADKEventEntity` → `UpsertEventEntity` | sessionmemory.Store | ✅ |
| 重命名 `adkScopeTypeSession`/`adkEntityEvent`/`adkSourceSync` 常量 | sessionmemory | ✅ |
| 补充 Session 模型缺失字段 | workspace_id, user_id, tags_json, default_provider, default_model, default_context_window_tokens, last_provider, last_model, last_context_window_tokens, visibility, avg_latency_ms, error_count, first_message_at, last_run_at, metadata_json, state_json | ✅ |

### Phase 0a：Session 功能增强（✅ 已完成）

| 工作 | 说明 | 状态 |
|------|------|------|
| RestoreSession RPC | 恢复归档会话 | ✅ |
| UpdateSession 部分更新 | title/tags_json/visibility/metadata_json/dialog_mode/default_provider/default_model | ✅ |
| Session Turns | CreateTurn/UpdateTurn/ListTurns + session_turns 表 | ✅ |
| Session State KV | GetSessionState/SaveSessionState/ApplyStateDelta | ✅ |
| 自动标题生成 | LLMSessionTitleGenerator + 截取双策略 | ✅ |
| Timeline 增强 | kind_filter/sort_order/limit/offset 分页 | ✅ |
| ListSessionMessages 分页 | limit/offset | ✅ |
| SearchSessions 增强 | user_id/sort_by/sort_order 筛选 | ✅ |

### Phase 1：扩展 Session 主表与列表（部分完成）

| 工作 | 说明 | 状态 |
|------|------|------|
| 数据库 | 给 `sessions` 增加 summary、message_count、token、cost、context_status 等字段 | ✅ |
| Ent Schema | 同步新增字段定义，`go generate ./internal/data/ent` | ✅ |
| Proto | 更新 `session.proto`，添加新字段的 proto 定义，`make api` | ✅ |
| Repository | `ListSessions` 支持 owner_type、team_id、状态、分页 | ✅ |
| API | `GET /sessions` 从简单数组升级为分页查询 | ✅ |
| 前端 | 新增 Session 历史页和详情 Header，复用现有聊天列表数据 | ✅ |
| Session 置顶 | sessions 增加 `pinned_at` 字段 + PinSession/UnpinSession RPC | 待实现 |

### Phase 2：Context 快照与聚合

| 工作 | 说明 | 状态 |
|------|------|------|
| 新表 | `session_context_snapshots` | 待实现 |
| ChatService | 模型调用完成后写快照，更新 `context_status` | 待实现 |
| Usage 聚合 | 从 `model_token_usage_events` 回填 session token / cost | 待实现 |
| 前端 | 显示 context 趋势和 warning/critical 状态 | 待实现 |

### Phase 3：Run / Step 编排记录

| 工作 | 说明 | 状态 |
|------|------|------|
| 新表 | `session_runs`、`session_run_steps`、`session_trace_spans` | 待实现 |
| 单 Agent | 每次发送消息创建 chat run、turn 和 trace spans | 待实现（turns 已实现） |
| Team | 编排器每个 agent/tool/skill/mcp 动作写 step 和 span | 待实现 |
| 前端 | 详情页增加 Timeline Tab 和 Trace 链路 Tab | 待实现 |

### Phase 4：Team Participants 与复盘

| 工作 | 说明 | 状态 |
|------|------|------|
| 新表 | `session_participants` | 待实现 |
| Team 编排 | 自动维护参与 Agent、角色、贡献指标 | 待实现 |
| 前端 | Team session 展示参与者、handoff、内部消息开关 | 待实现 |

### Phase 5：trpc session.Service 对齐

| 工作 | 说明 | 状态 |
|------|------|------|
| 适配器 | `internal/session/trpc/service.go` 桥接 Ent session 到 trpc session.Service | 待实现 |
| Redis 后端 | `internal/session/trpc/redis.go` 生产环境 Redis 存储 | 待实现 |
| 摘要迁移 | 将 `internal/compress` 摘要逻辑迁移到 trpc 内置压缩 | 待实现 |
| 验证 | Runner 使用适配后的 SessionService 正常运行 | 待实现 |

---

## 10. 关键设计原则

1. **Session 是事实关联中心，不是所有事实本身**：消息、usage、run、step 各自独立落库，通过 `session_id` 关联。
2. **明细不可变，聚合可重算**：`model_token_usage_events`、`session_run_steps` 是事实源；`sessions` 上的 token、费用、context 是查询优化字段。
3. **Team 与 Agent 共用一套 session 模型**：通过 `owner_type` 区分，不要拆成两套历史系统。
4. **内部编排消息默认可折叠**：Team session 既要可复盘，也不能让用户被内部 step 淹没。
5. **Context ratio 必须可解释**：列表显示比例，详情能追到哪个 run/step/message 导致消耗升高。
6. **软删除优先**：session 删除不应破坏成本统计和审计链路。
7. **模型配置保存快照，但不假设唯一模型**：历史 session 保存默认模型和最近模型；每次实际调用的 provider/model/context window 以 usage 与 step 明细为准。
8. **框架对齐优先**：先查 trpc-agent-go 框架 API 再实现，不在 biz 重写运行时；`runner_snapshot_json` 是 Runner 状态的唯一持久化格式。
9. **分层铁律不可违反**：`internal/biz` 不得 import `pkg/trpc-agent-go`；框架运行时交互只在 `internal/service` 和 `internal/agent` 层进行。
10. **错误处理统一**：biz 层使用 `kerrors.BadRequest`/`kerrors.NotFound`/`kerrors.InternalServer`，不用 `fmt.Errorf` 或 `errors.New`。

---

## 11. 会话上下文压缩

> 本节整合自 `architecture/session-context-compression.md`，描述长会话如何用 LLM 生成可复用的压缩摘要，并在后续轮次仅向模型注入摘要 + 近期原文以降低 token 的行为契约与实现落点。

### 11.1 问题与目标

**问题**：单会话消息与 Runner 事件随轮次增长，反复把完整历史送入模型导致 prompt token 线性增长、成本与延迟上升，更易触碰上下文上限。

**目标**：

1. 当历史达到一定规模，将一段可追溯的对话区间交给「压缩模型」梳理为结构化摘要，要求不遗漏对后续决策关键的事实。
2. 后续用户继续对话时，模型侧上下文改为：**压缩摘要 + 最近若干轮原文 + 本轮输入**。
3. **账本不变**：`messages`（及必要的 trace）仍保留完整原文；压缩改变的是**送入模型的装配结果**，默认不物理删除历史消息。

**非目标**：不替代 L3/L4 长期记忆检索；不把「压缩」作为唯一降耗手段；首版不要求用户编辑摘要正文。

### 11.2 核心概念

| 术语 | 含义 |
|------|------|
| 滚动摘要（Rolling Summary） | 覆盖区间 `[from_turn, to_turn]` 的 Markdown 文本，存于 `session_summaries` |
| 当前有效摘要（Active Summary） | 某 session 在装配时刻用于头部的摘要集合 |
| 滑动窗口（Tail） | 摘要区间之后、尚未被摘要覆盖的最近 K 轮或最近 T tokens 的原始对话 |
| 压缩模型（Compressor Model） | 执行摘要生成的 LLM 调用；可与对话模型同厂商或降级为更小规格 |

### 11.3 触发条件

建议可组合配置（Agent / Team / Session 级覆盖），默认启用「soft + hard」双层：

| 策略 | 条件 | 说明 |
|------|------|------|
| 比例触发 | `context_used_ratio ≥ summary_threshold` | 与现有 `sessions.context_*` 字段对齐 |
| 轮次触发 | 自上次摘要以来新增 `Δturn ≥ compress_every_n_turns` | 防窗口很大但比例尚未告警时长对话不摘要 |
| Token 估算触发 | 未摘要前缀估算 token ≥ `compress_prefix_token_budget` | 与滑动窗口预算联动 |
| 手动触发 | UI「生成会话摘要」或 API | 便于调试与关键节点强制固化 |

**防抖**：同一 session 短时间窗口内（如 5～10 分钟）最多触发 N 次摘要任务。

**并发**：若上一轮摘要尚未完成，后续触发应合并区间或排队，避免交错写入两条重叠 `from_turn/to_turn`。

### 11.4 压缩任务的输入与输出

**输入（发给压缩模型）**：对选定区间，序列化可追溯的对话——角色与时间序、工具调用（工具名、参数摘要、结果摘要或截断后的关键字段）、锚点（session_id、区间、上轮摘要）。

**输出（摘要 schema）**——推荐固定章节：

1. 用户意图与目标
2. 已确认事实 / 结论（含数字、版本、路径、API 名等硬信息）
3. 约束与偏好（语言、风格、禁止项）
4. 未完成事项 / 待澄清问题
5. 重要工具结果摘录（表格或列表）
6. 术语与别名

输出写入 `session_summaries.summary_markdown`，并填写 `from_turn`、`to_turn`、`token_estimate`、`created_at`。可同时更新 `sessions.summary` 为列表页/会话卡片用的一句话摘要。

**提示词原则**：明确后续对话仅能看到本摘要 + 最近几轮，要求在无损前提下最大化密度；不得编造；保留可执行细节。

### 11.5 后续轮次装配顺序

在单次模型调用前，L0 装配器按段拼装：

1. 系统 / 开发者固定段（SOUL、策略等）
2. **`session_summaries` 合并摘要**（标记 `source: session_summaries:<id>`）
3. L1 工作记忆字段（若有）
4. **滑动窗口内原始 messages**（摘要区间之后）
5. L3/L4 检索段（若有）
6. 本轮 user 输入

被摘要覆盖的旧消息不再重复进入 prompt，但仍可从 DB 读取用于 UI 与合规。

### 11.6 多条 session_summaries 合并策略

- **A. 区间链式（首版推荐）**：保留多条记录，装配时按 `from_turn` 排序拼接为一块「历史摘要」文本。逻辑清晰且易于回放；metadata 中记录 `supersedes_id` 以备迁移。
- **B. 单条滚动（二期演进）**：每次压缩后把旧摘要 + 新区间对话一并输入，产出一条覆盖 `[0, current_to]` 的新摘要并 supersede 旧指针。token 更省，单次成本高。

### 11.7 与 ADK 会话持久态的关系

| 方案 | 做法 | 优点 | 风险 |
|------|------|------|------|
| **装配层优先（首版推荐）** | 不改变 `runner_snapshot_json` 内全量事件；仅在构造发往 LLM 的 messages 时应用摘要 + tail | 实现集中、可逆、与现有 messages 账本一致 | ADK 内部若独立推算上下文，需确认走同一装配入口 |
| 快照裁剪（可选） | 在摘要固化后，对 snapshot 中早于 `to_turn` 的模型事件做归档或删除 | 持久态更小 | 回放 Runner 历史不完整，需额外归档存储 |

### 11.8 一致性、失败与重试

- **幂等键**：`(session_id, from_turn, to_turn, prompt_hash)`；重复任务返回已有记录。
- **失败**：摘要失败时不阻断用户发消息；回退为「仅滑动窗口截断」并打 `context_status`/告警日志。
- **观测**：写入 `memory_l0_assembly_snapshots` 中的 `summarized_turn_from/to`、`summary_token_estimate`、`segments_json` 段来源。

### 11.9 Team 会话

- **隔离**：每个子 Agent 应有独立的摘要区间与 `session_summaries` 维度，避免 Host 与子 Agent 上下文串扰。
- **Host 摘要**：可仅摘要「路由级」对话；专家会话单独滚动摘要。

### 11.10 API / 配置 / UX

| 层次 | 建议 |
|------|------|
| 配置 | 扩展 `agent_runtime_settings`：`summary_threshold`、`compress_every_n_turns`、`recent_window_turns`、`recent_window_tokens`、`compressor_model_profile`；另设 `l0_compress_provider` / `l0_compress_model`（可选），指定专用压缩调用 |
| API | `SummaryService.SummarizeRange(session_id, from, to)` |
| UI | 会话详情展示「已摘要至第 N 轮」标签；可选展示摘要正文（只读）；手动触发按钮 |

### 11.11 落地里程碑

1. **M1**：实现 `SummarizeRange`，写入 `session_summaries`；L0 装配器读取并注入 summary 段；集成测试覆盖「摘要 + tail」token 低于「全量」。
2. **M2**：与 `context_used_ratio` / 轮次策略联动自动触发；观测写入 assembly snapshot。
3. **M3**：Team 维度与手动触发 / UI；可选迁移到单条滚动摘要策略 B。

---

## 12. trpc-agent-go 对齐需求（M5 Session 管理）

> 本节补充 `plan.md` M5 模块的对齐需求，确保 Session 管理完全复刻 trpc-agent-go `session.Service` 能力。

### 12.1 trpc session.Service 集成

**trpc 框架**：`session.Service` 提供统一的 Session 存储接口，支持多后端。

**需求**：
- 新建 `internal/session/trpc/service.go`，桥接 Ent session 到 trpc `session.Service` 接口
- 先实现 SQLite 后端（项目已有）
- 后续增加 Redis 后端用于生产环境
- 最终支持 PostgreSQL/MySQL/ClickHouse 后端

**涉及文件**：`internal/session/trpc/service.go`、`internal/session/trpc/sqlite.go`、`internal/session/trpc/redis.go`

**验收标准**：Session 通过 trpc `session.Service` 接口持久化到 SQLite

### 12.2 Event 分页

**trpc 框架**：`session.Service.ListEvents` 支持分页查询 Session 事件。

**需求**：
- 实现 `ListEvents(sessionID, pageSize, pageToken)` 方法
- 支持按时间正序/倒序
- 支持按事件类型过滤

**涉及文件**：`internal/session/trpc/service.go`

**验收标准**：Session 事件可分页查询

### 12.3 Session Track

**trpc 框架**：`session.Service` 支持 Track 操作，记录 Session 级别的元数据。

**需求**：
- 实现 `Track(sessionID, key, value)` 方法
- Track 数据存储在 `sessions.metadata_json` 中
- 前端可查询 Track 数据

**涉及文件**：`internal/session/trpc/service.go`

**验收标准**：Session 级别的元数据可记录和查询

### 12.4 Session Ingestor

**trpc 框架**：`session.Ingestor` 接口，Session 完成后自动摄入到外部平台。

**需求**：
- 新建 `internal/session/trpc/ingestor.go`
- 实现 `session.Ingestor` 接口
- 可对接 Mem0 等外部记忆平台
- Runner 完成后自动调用 Ingestor

**涉及文件**：`internal/session/trpc/ingestor.go`、`internal/agent/trpc_runtime.go`

**验收标准**：Session 完成后自动摄入到外部记忆平台

### 12.5 多后端支持

**trpc 框架**：`session/sqlite`、`session/redis`、`session/pg`、`session/mysql`、`session/clickhouse` 多后端。

**需求**：
- 配置文件增加 `session.backend` 字段
- 可选值：`sqlite`（默认）、`redis`、`postgresql`、`mysql`、`clickhouse`
- 按配置动态选择后端
- 提供迁移工具

**涉及文件**：`internal/session/trpc/factory.go`、`internal/conf/conf.proto`

**验收标准**：Session 可按配置选择不同存储后端

### 12.6 Runner Snapshot 集成

**trpc 框架**：Runner 执行状态序列化为 `runner_snapshot_json`，用于 Runner 恢复。

**需求**：
- Runner 每轮执行后更新 `sessions.runner_snapshot_json`
- Runner 恢复时从 `runner_snapshot_json` 加载状态
- 摘要压缩后同步更新 snapshot

**涉及文件**：`internal/agent/trpc_runtime.go`、`internal/service/chat_native.go`

**当前状态**：✅ 已实现（`UpdateRunnerSnapshotJSON` + SessionCompressor 同步更新）

**验收标准**：Runner 可从 snapshot 恢复执行状态

---

## 13. 当前实施状态总览

| 模块 | 功能 | 状态 |
|------|------|------|
| Session CRUD | Create/Get/Update/Delete/Search | ✅ |
| Session 归档/恢复 | ArchiveSession/RestoreSession | ✅ |
| Session 部分更新 | UpdateSession（title/tags/visibility/metadata/dialog_mode/provider/model） | ✅ |
| Timeline 聚合 | GetSessionTimeline + kind_filter/sort_order/limit/offset | ✅ |
| 消息列表 | ListSessionMessages + limit/offset 分页 | ✅ |
| 上下文压缩 | SessionCompressor.AfterNativeTurn | ✅ |
| 标题生成 | LLMSessionTitleGenerator + 截取双策略 | ✅ |
| Session Turns | CreateTurn/UpdateTurn/ListTurns | ✅ |
| Session State KV | GetSessionState/SaveSessionState/ApplyStateDelta | ✅ |
| Runner Snapshot | UpdateRunnerSnapshotJSON | ✅ |
| Session Summaries | InsertSessionSummary/ListSessionSummaries | ✅ |
| Session 置顶 | pinned_at + PinSession/UnpinSession | ❌ 待实现 |
| Session 导出 | ExportSession（Markdown/JSON） | ❌ 待实现 |
| 消息搜索 | SearchMessages（全文检索） | ❌ 待实现 |
| session_runs | 编排运行记录 | ❌ 待实现 |
| session_run_steps | 编排步骤记录 | ❌ 待实现 |
| session_participants | Team 参与者 | ❌ 待实现 |
| session_trace_spans | 完整追踪链路 | ❌ 待实现 |
| session_context_snapshots | Context 趋势 | ❌ 待实现 |
| session_model_summaries | 多模型分布 | ❌ 待实现 |
| trpc session.Service 适配器 | 桥接 trpc-agent-go | ❌ 待实现 |
| 多后端支持 | Redis/PG/MySQL/ClickHouse | ❌ 待实现 |
| Session Ingestor | 自动摄入外部记忆平台 | ❌ 待实现 |
