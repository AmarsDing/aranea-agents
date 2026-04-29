# 12 L0 瞬时记忆 / 感知记忆（Sensory Memory）

本文档落地 5 层记忆架构中的 **L0：瞬时记忆 / 感知记忆**。L0 是 LLM 上下文窗口内的即时信息，类似认知心理学 Atkinson-Shiffrin 模型中的「感官记忆」：容量极小、访问极快、不持久化，由模型原生注意力机制直接消费，超出窗口部分自动溢出。

L0 的设计原则是**轻量、可观测、可裁剪**：不在 SQL 中保存 L0 的内容（内容仍由 `messages` 表承载），而是把「上下文窗口构造、滑动窗口、压缩裁剪、窗口快照」这一整套行为变成 aranea 后端可以编排、监控、配置的能力，作为 L1～L4 的入口与出口。

> 关联文档：`memery.md` §1～§5、`10 session.md` §4.5/§4.7、`5 agent-setting.md`、`7 agent-evolution.md`。

---

## 1. 心智模型与边界

### 1.1 L0 在 5 层中的位置

| 维度 | 描述 |
|------|------|
| 容量 | LLM 上下文窗口（128K~2M tokens），按 `agent.context_window`、`session_model_summaries.max_context_window_tokens`、本次模型配置取最小值 |
| 持久性 | 不持久化；只保留**最近 N 轮**消息和**当前轮工具结果** |
| 时效 | 单轮对话内，请求结束即丢弃 |
| 访问模式 | LLM 原生注意力机制，由 ChatService / TeamRuntime 在调用前装配 |
| 与 ADK 对齐 | 对应 `Session.events` + `Session.state` 中的「即时上下文 + 滑动窗口」 |

### 1.2 与其它层的边界

| 边界 | 走向 | 说明 |
|------|------|------|
| L0 ↔ L1 | 任务级结构化字段（task goal / active constraints）由 L1 注入 prompt header；L0 不保存这些字段，只「使用」 |
| L0 → L2 | 滑动窗口溢出的 message + 本轮工具结果落入 L2 `messages` / `session_trace_spans`（已有） |
| L0 → 摘要 | 当 `context_used_ratio` ≥ 阈值时，触发 `SummaryService` 把老对话压成 `session_summaries`，写回 L0 prompt 头部 |
| L0 ← L3 | 通过 `MemoryRecall` 从 L3 检索 ≤ K 条片段，注入 prompt 中部「相关知识」段 |
| L0 ← L4 | 通过 `GraphRecall` 从 L4 检索因果链/实体路径，注入 prompt 中部「相关历史经验」段 |

### 1.3 非目标

- 不在 L0 层做长期事实存储（用 L3）。
- 不在 L0 层维护任务状态机（用 L1）。
- 不替换现有 `messages` / `session_summaries` 的事实源地位，L0 只**使用**它们。
- 不破坏 `sessions.context_used_ratio` 的现有写入路径。

---

## 2. 需求清单

### 2.1 功能需求

| # | 需求 | 必要性 |
|---|------|--------|
| F1 | 在每次模型调用前，按当前 session 的最近 N 轮 message 构造 LLM `messages` 数组 | 必须 |
| F2 | 支持按 token 上限滑动窗口裁剪：仅保留最近 K tokens；老消息按时间倒序丢弃 | 必须 |
| F3 | 当 `context_used_ratio ≥ summary_threshold` 时，自动触发 SummaryService 生成段摘要 | 必须 |
| F4 | 摘要写入 `session_summaries` 后作为 system message 注入 prompt 头 | 必须 |
| F5 | 支持注入「Skill 文件 / SOUL.md / AGENTS.md」等 prompt 锚点段，固定在 prompt 头 | 必须 |
| F6 | 支持注入 L1 工作记忆字段、L3 检索片段、L4 推理路径，按段独立可关闭 | 必须 |
| F7 | 每次模型调用后，写入 `session_context_snapshots`（已规划）记录消耗轨迹 | 必须 |
| F8 | 支持显示「下一次发送给模型的实际 prompt 草稿」预览，便于人工调试 | 推荐 |
| F9 | 支持每个 Agent 配置 `recent_window_turns`、`recent_window_tokens`、`summary_threshold` | 必须 |
| F10 | Team session 中按子 Agent 各自构造 L0，避免互相覆盖 | 必须 |

### 2.2 非功能需求

| # | 需求 | 目标值 |
|---|------|--------|
| N1 | L0 装配延迟 P99 | < 30 ms（不含 L3/L4 检索） |
| N2 | 滑动窗口 + 摘要不会让 prompt 超出 model context window | 100% |
| N3 | 摘要触发频次合理 | 单个 session 24h 内 ≤ 5 次 |
| N4 | 上下文比例计算准确率 | ≥ 95%（与模型 usage 返回值偏差） |
| N5 | 内部装配过程可追踪 | 每次调用写一条 `memory_recall` span（见 `10 session.md` §4.6） |

### 2.3 配置需求

复用并扩展 `agent_runtime_settings`（已有 `memory_*` 字段），新增 L0 子项；详见 §6.1。

---

## 3. 数据模型

L0 不引入大型存储表，仅扩展两类既有表 + 新增一张极轻量级的「装配快照」表（可选，仅当 `evolution_metrics_enabled = true` 时写入）。

### 3.1 扩展 `sessions`（已规划字段，本节明确语义）

`10 session.md` §4.1 已定义；本设计要求以下字段对 L0 必须实时维护：

| 字段 | L0 用途 |
|------|--------|
| `context_used_tokens` | 上一次模型调用使用的 prompt tokens |
| `context_used_ratio` | `prompt_tokens / context_window_tokens` |
| `max_context_used_ratio` | 历史最高比例，用于发现爆窗风险 |
| `context_status` | normal / warning / critical / exceeded |
| `last_context_window_tokens` | 上一次实际模型 context window 大小 |

### 3.2 扩展 `agent_runtime_settings`

`memory_*` 已存在（`memory_enabled`、`memory_max_chunk_length`、`memory_max_results`、`memory_min_score`），需要新增 L0 专属字段：

```sql
ALTER TABLE agent_runtime_settings ADD COLUMN l0_recent_window_turns INTEGER NOT NULL DEFAULT 12;
ALTER TABLE agent_runtime_settings ADD COLUMN l0_recent_window_tokens INTEGER NOT NULL DEFAULT 0;
-- 0 表示按 model context window 的 60% 自动计算
ALTER TABLE agent_runtime_settings ADD COLUMN l0_summary_threshold REAL NOT NULL DEFAULT 0.6;
ALTER TABLE agent_runtime_settings ADD COLUMN l0_summary_keep_turns INTEGER NOT NULL DEFAULT 4;
ALTER TABLE agent_runtime_settings ADD COLUMN l0_truncate_strategy TEXT NOT NULL DEFAULT 'summary';
-- summary / drop_oldest / drop_tool_results / hybrid
ALTER TABLE agent_runtime_settings ADD COLUMN l0_inject_l1 INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agent_runtime_settings ADD COLUMN l0_inject_l3 INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agent_runtime_settings ADD COLUMN l0_inject_l4 INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agent_runtime_settings ADD COLUMN l0_l3_max_chunks INTEGER NOT NULL DEFAULT 5;
ALTER TABLE agent_runtime_settings ADD COLUMN l0_l4_max_paths INTEGER NOT NULL DEFAULT 3;
```

### 3.3 新增表：`memory_l0_assembly_snapshots`（可选 / 低频）

仅当 `evolution_metrics_enabled = true` 或人工触发「调试」时写入。用于审计 L0 装配过程，前端「Prompt 调试器」、「Trace 详情 → memory_recall span」展开使用。

```sql
CREATE TABLE IF NOT EXISTS memory_l0_assembly_snapshots (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  run_id TEXT NOT NULL DEFAULT '',
  turn_id TEXT NOT NULL DEFAULT '',
  span_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',

  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  context_window_tokens INTEGER NOT NULL DEFAULT 0,
  budget_tokens INTEGER NOT NULL DEFAULT 0,
  -- 本次允许写入 prompt 的 token 上限（model_window - reserved_for_output）

  recent_window_turns INTEGER NOT NULL DEFAULT 0,
  recent_window_tokens INTEGER NOT NULL DEFAULT 0,
  summary_token_estimate INTEGER NOT NULL DEFAULT 0,

  l1_field_count INTEGER NOT NULL DEFAULT 0,
  l1_token_estimate INTEGER NOT NULL DEFAULT 0,
  l3_chunk_count INTEGER NOT NULL DEFAULT 0,
  l3_token_estimate INTEGER NOT NULL DEFAULT 0,
  l4_path_count INTEGER NOT NULL DEFAULT 0,
  l4_token_estimate INTEGER NOT NULL DEFAULT 0,

  prompt_token_estimate INTEGER NOT NULL DEFAULT 0,
  prompt_token_actual INTEGER NOT NULL DEFAULT 0,
  used_ratio REAL NOT NULL DEFAULT 0,

  truncate_strategy TEXT NOT NULL DEFAULT '',
  truncated_message_count INTEGER NOT NULL DEFAULT 0,
  summarized_turn_from INTEGER NOT NULL DEFAULT 0,
  summarized_turn_to INTEGER NOT NULL DEFAULT 0,

  segments_json TEXT NOT NULL DEFAULT '[]',
  -- 装配后各段落的轻摘要：role/section/source/token/preview
  warning_codes_json TEXT NOT NULL DEFAULT '[]',
  -- ['exceeded','near_limit','no_summary_available'] 等
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memory_l0_snapshots_session
  ON memory_l0_assembly_snapshots(session_id, created_at);

CREATE INDEX IF NOT EXISTS idx_memory_l0_snapshots_span
  ON memory_l0_assembly_snapshots(span_id);
```

`segments_json` 示例：

```json
[
  {"section":"system.prompt", "role":"system", "source":"agent.prompt",        "tokens":820,  "preview":"You are an expert designer..."},
  {"section":"system.skill",  "role":"system", "source":"skill:design-md",     "tokens":1240, "preview":"Use semantic tokens for spacing"},
  {"section":"memory.l1",     "role":"system", "source":"l1:working_state",    "tokens":312,  "preview":"task_goal=...; active_constraints=..."},
  {"section":"memory.l3",     "role":"system", "source":"l3:semantic_recall",  "tokens":540,  "preview":"用户偏好：暗色主题；React + Quasar"},
  {"section":"memory.l4",     "role":"system", "source":"l4:graph_path",       "tokens":280,  "preview":"实体: SidebarBug -> 修复模式 -> ..."},
  {"section":"summary",       "role":"system", "source":"session_summaries:s_2", "tokens":420, "preview":"前 18 轮摘要..."},
  {"section":"history",       "role":"history","source":"messages[recent_12]", "tokens":3800, "preview":"u: ...; a: ..."},
  {"section":"user.input",    "role":"user",   "source":"messages[current]",   "tokens":160,  "preview":"实现 dark mode"}
]
```

> 写入策略：默认**仅在 `warning/critical/exceeded` 状态或开启「记忆调试」时写**。可通过 `agent_runtime_settings.l0_snapshot_mode = always | on_warning | off` 控制（建议作为 §3.2 的扩展项加入）。

### 3.4 复用既有表

| 表 | L0 角色 |
|----|--------|
| `messages` | 历史消息事实源；L0 滑动窗口直接从此表按 `session_id` + `turn_index DESC` 取最近 N 条 |
| `session_summaries` | 已有；L0 摘要写入此表，并按 `from_turn / to_turn` 替换历史段 |
| `session_context_snapshots`（`10 session.md` §4.7 规划） | L0 上下文消耗轨迹 |
| `session_trace_spans` | 每次 L0 装配产生一条 `span_type = memory_recall` 的 span，`metadata_json.l0_assembly_id` 引用 §3.3 |
| `model_token_usage_events` | L0 装配后实际消耗 token 的事实源，回填 `prompt_token_actual` |

---

## 4. Go 域模型与 Repository 接口

### 4.1 域模型

新增到 `internal/domain/memory_l0.go`：

```go
package domain

type L0AssemblyRequest struct {
	SessionID         string
	RunID             string
	TurnID            string
	SpanID            string
	AgentID           string
	TeamID            string
	Provider          string
	Model             string
	ContextWindow     int
	ReservedForOutput int
	UserMessageID     string
	ExtraSystemBlocks []L0Segment
}

type L0Segment struct {
	Section string `json:"section"`
	Role    string `json:"role"`
	Source  string `json:"source"`
	Tokens  int    `json:"tokens"`
	Content string `json:"content"`
	Preview string `json:"preview"`
}

type L0AssemblyResult struct {
	Segments              []L0Segment
	PromptMessages        []ChatMessage // 给 LLM 的最终结构
	BudgetTokens          int
	PromptTokenEstimate   int
	UsedRatioEstimate     float64
	RecentWindowTurns     int
	RecentWindowTokens    int
	SummarizedTurnFrom    int
	SummarizedTurnTo      int
	TruncateStrategy      string
	TruncatedMessageCount int
	WarningCodes          []string
	SnapshotID            string // 对应 memory_l0_assembly_snapshots.id（若已写入）
}

type L0Settings struct {
	RecentWindowTurns  int
	RecentWindowTokens int
	SummaryThreshold   float64
	SummaryKeepTurns   int
	TruncateStrategy   string // summary / drop_oldest / drop_tool_results / hybrid
	InjectL1           bool
	InjectL3           bool
	InjectL4           bool
	L3MaxChunks        int
	L4MaxPaths         int
	SnapshotMode       string // always / on_warning / off
}

type L0AssemblySnapshot struct {
	ID                    string
	SessionID             string
	RunID                 string
	TurnID                string
	SpanID                string
	AgentID               string
	TeamID                string
	Provider              string
	Model                 string
	ContextWindowTokens   int
	BudgetTokens          int
	RecentWindowTurns     int
	RecentWindowTokens    int
	SummaryTokenEstimate  int
	L1FieldCount          int
	L1TokenEstimate       int
	L3ChunkCount          int
	L3TokenEstimate       int
	L4PathCount           int
	L4TokenEstimate       int
	PromptTokenEstimate   int
	PromptTokenActual     int
	UsedRatio             float64
	TruncateStrategy      string
	TruncatedMessageCount int
	SummarizedTurnFrom    int
	SummarizedTurnTo      int
	SegmentsJSON          string
	WarningCodesJSON      string
	MetadataJSON          string
	CreatedAt             string
}
```

### 4.2 Repository 接口

新增 `internal/repository/memory_l0.go`：

```go
type MemoryL0Repository interface {
	InsertSnapshot(ctx context.Context, snap domain.L0AssemblySnapshot) error
	UpdateActualTokens(ctx context.Context, snapshotID string, actualPromptTokens int, usedRatio float64) error
	GetSnapshotByID(ctx context.Context, id string) (domain.L0AssemblySnapshot, error)
	ListBySession(ctx context.Context, sessionID string, limit int) ([]domain.L0AssemblySnapshot, error)
	ListBySpan(ctx context.Context, spanID string) ([]domain.L0AssemblySnapshot, error)
}
```

实现位于 `internal/repository/sqlite_memory_l0.go`，与现有 `sqlite.go` 同风格的 prepared statements；查询固定按 `session_id, created_at DESC` 排序，分页限制最大 100 行。

### 4.3 与 `MessageRepository` 的协作

L0 不重复实现「取最近 N 条 message」的能力，复用 `MessageRepository`（已有 `ListBySession`），并扩展：

```go
type MessageRepository interface {
	// ...
	ListLatestByTokens(ctx context.Context, sessionID string, maxTokens int, hardCap int) ([]domain.Message, error)
}
```

实现思路：从 `messages` 按 `turn_index DESC` 取，累加 `token_in + token_out` 估算；超过 `maxTokens` 或 `hardCap` 行数即停止。注意必须保留**当前用户输入消息**且不计入裁剪。

---

## 5. Service 层接口

### 5.1 新增 `MemoryL0Service`

```go
type MemoryL0Service interface {
	// 装配 prompt：从 messages、summaries、L1/L3/L4 拼出最终 LLM messages
	Assemble(ctx context.Context, req domain.L0AssemblyRequest) (domain.L0AssemblyResult, error)

	// 模型调用完成后，回填实际 prompt token，更新 sessions 与 snapshot
	RecordActual(ctx context.Context, snapshotID string, actualPromptTokens int, contextWindow int) error

	// 列出最近 N 个装配快照（调试用）
	ListSnapshots(ctx context.Context, sessionID string, limit int) ([]domain.L0AssemblySnapshot, error)

	// 仅生成预览，不发出，调用方为「Prompt 调试器」
	Preview(ctx context.Context, req domain.L0AssemblyRequest) (domain.L0AssemblyResult, error)
}
```

### 5.2 装配流程（Assemble 内部步骤）

```text
1. 读 agent_runtime_settings 解析 L0Settings
2. 计算 budget_tokens = context_window - reserved_for_output - safety_margin
3. 取 system blocks：
     a. agent.prompt（agent_prompt_files）
     b. skill 文件（已绑定 skill）
     c. extra_system_blocks（来自调用方）
4. 若 inject_l1：调用 MemoryL1Service.GetWorkingState() 注入「memory.l1」段
5. 若 inject_l3：调用 MemoryL3Service.Recall(query=user_input) 取 ≤ l3_max_chunks
6. 若 inject_l4：调用 MemoryL4Service.GraphRecall(query=user_input) 取 ≤ l4_max_paths
7. 取 session_summaries（已有摘要段），按 turn_index 范围合并
8. 取 messages：MessageRepository.ListLatestByTokens(...) → 最近窗口
9. 估算总 token；若 > budget_tokens：
     - 若 truncate_strategy = summary 且 budget 超出来自 history：
         - 触发 SummaryService.Summarize(from_turn..to_turn)
         - 用新摘要替换被压缩的历史段
     - 若 truncate_strategy = drop_oldest：去掉最老的历史段
     - 若 truncate_strategy = drop_tool_results：仅去掉 tool 角色消息
     - 若 truncate_strategy = hybrid：先 drop_tool_results，再 summary，再 drop_oldest
10. 装配最终 prompt_messages
11. 估算 prompt_token_estimate
12. 计算 used_ratio_estimate = prompt_token_estimate / context_window
13. 若 used_ratio_estimate ≥ 1.0 → warning_codes += "exceeded"
14. 若 snapshot_mode = always 或 (on_warning 且 used_ratio_estimate ≥ 0.6)：
       - InsertSnapshot
       - SnapshotID 写回 result
       - SessionTraceSpan(memory_recall) 通过 SessionService.StartSpan 创建
15. 返回 L0AssemblyResult
```

### 5.3 与现有 ChatService / TeamRuntime 集成

`ChatService.SendMessage()` 当前调用 `ADKAdapter` 直接构造 prompt。改造点：

```go
// 旧
modelInput := buildModelInput(session, history, userMsg)

// 新
result, err := memoryL0.Assemble(ctx, domain.L0AssemblyRequest{
    SessionID: session.ID,
    RunID: run.ID,
    TurnID: turn.ID,
    SpanID: modelCallSpan.ID,
    AgentID: agent.ID,
    Provider: chosenProvider,
    Model: chosenModel,
    ContextWindow: chosenContextWindow,
    ReservedForOutput: maxOutputTokens,
    UserMessageID: userMsg.ID,
})
modelInput := result.PromptMessages
```

模型调用完成后：

```go
memoryL0.RecordActual(ctx, result.SnapshotID, usage.PromptTokens, chosenContextWindow)
```

`TeamRuntime` 中每个子 Agent 调用前各自走一次 `Assemble`，传入 `team_id`、对应子 Agent 的 `agent_id`，保证 Team session 中各 Agent 的 L0 互不污染。

### 5.4 与 `SummaryService` 协作

`SummaryService` 已被规划写入 `session_summaries`。L0 在压缩时按以下规则触发：

| 触发条件 | 行为 |
|----------|------|
| `used_ratio_estimate ≥ summary_threshold` 且历史段还可压缩 | 调用 `SummaryService.SummarizeRange(session_id, from_turn, to_turn)`，产生 `session_summaries` 记录 |
| 摘要生成失败 | 回退到 `drop_oldest`，并加 `warning_codes += "summary_failed"` |
| 摘要后仍超额 | 继续 `drop_tool_results` → `drop_oldest` |

摘要保留最近 `l0_summary_keep_turns` 轮**不压缩**，保证模型能看到最新决策上下文。

---

## 6. HTTP API

### 6.1 配置 API（已有 settings 接口扩展）

复用现有 `PATCH /api/v1/agents/{id}/runtime-settings`，body 增加：

```json
{
  "l0_recent_window_turns": 12,
  "l0_recent_window_tokens": 8000,
  "l0_summary_threshold": 0.6,
  "l0_summary_keep_turns": 4,
  "l0_truncate_strategy": "summary",
  "l0_inject_l1": true,
  "l0_inject_l3": true,
  "l0_inject_l4": false,
  "l0_l3_max_chunks": 5,
  "l0_l4_max_paths": 3,
  "l0_snapshot_mode": "on_warning"
}
```

### 6.2 调试 / 观测 API

```http
GET /api/v1/sessions/{id}/l0/snapshots?limit=20
GET /api/v1/sessions/{id}/l0/snapshots/{snapshotId}
GET /api/v1/spans/{span_id}/l0/snapshots

POST /api/v1/sessions/{id}/l0/preview
{
  "user_message": "实现暗色模式",
  "agent_id": "...",
  "provider": "openrouter",
  "model": "google/gemini-2.5-pro",
  "context_window": 1048576,
  "reserved_for_output": 8192
}
```

`POST .../l0/preview` 响应即 `L0AssemblyResult` 序列化，但 `prompt_messages` 中只返回 `preview`（前 200 字）+ token 估算，避免泄露完整内容。仅管理员可见完整 content。

### 6.3 与 Trace 的集成

`session_trace_spans.span_type = memory_recall` 的 span 详情接口（`GET /api/v1/sessions/{id}/spans/{spanId}`）应在 `metadata_json.l0_assembly_id` 存在时，**联表查询并返回** `memory_l0_assembly_snapshots` 的 `segments_json`、`warning_codes_json`、`prompt_token_actual`。

---

## 7. 与现有 aranea 模块的对接

| 模块 | 改造点 |
|------|--------|
| `internal/runtime/adk_adapter.go` | 用 `MemoryL0Service.Assemble` 替换内部 message 拼装；不再直接读 messages |
| `internal/service/chat_service.go` | 在 `SendMessage` / `SendMessageStream` 调用 `Assemble`、`RecordActual` |
| `internal/service/team_runtime.go` | 每个子 step 单独 `Assemble`，并把 `result.PromptMessages` 传给 ADK |
| `internal/service/session_service.go` | 新增 `RecordContextSnapshot`（已规划），由 L0 驱动写入 |
| `internal/service/summary_service.go`（新） | `SummarizeRange` 接口；写 `session_summaries` |
| `internal/transport/sessions.go` | 暴露 §6.2 的 `/l0/snapshots` 和 `/l0/preview` 接口 |
| `internal/repository/sqlite.go` | 在 `Migrate()` 内执行 §3.2 与 §3.3 的 ALTER / CREATE，并在 `ensureLegacyColumns` 中补齐 |

---

## 8. 前端展示需求（Quasar / Vue）

### 8.1 Agent 设置 → 记忆 Tab → L0 子区

放在 `/agents/:id/settings/memory` 页 L0 子卡片：

| 控件 | 字段 | 类型 |
|------|------|------|
| 最近窗口轮数 | `l0_recent_window_turns` | `QInput` number 1-50 |
| 最近窗口 tokens | `l0_recent_window_tokens` | `QInput` number；0 = 自动 |
| 摘要触发阈值 | `l0_summary_threshold` | `QSlider` 0.3-0.95，步进 0.05 |
| 摘要后保留最近轮数 | `l0_summary_keep_turns` | `QInput` 1-20 |
| 裁剪策略 | `l0_truncate_strategy` | `QSelect` summary/drop_oldest/drop_tool_results/hybrid |
| 注入 L1 工作记忆 | `l0_inject_l1` | `QToggle` |
| 注入 L3 语义记忆 | `l0_inject_l3` | `QToggle` |
| 注入 L4 进化记忆 | `l0_inject_l4` | `QToggle` |
| L3 最大片段数 | `l0_l3_max_chunks` | `QInput` 0-20 |
| L4 最大路径数 | `l0_l4_max_paths` | `QInput` 0-10 |
| 快照模式 | `l0_snapshot_mode` | `QBtnToggle` always/on_warning/off |

### 8.2 Session 详情 → 上下文 Tab

- 顶部 KPI：当前 ratio、最高 ratio、本会话摘要次数、装配快照数。
- `QLinearProgress` 显示当前比例，颜色映射 `context_status`。
- 折线图（`Chart.js`）：`session_context_snapshots` 趋势 + 摘要事件标记。
- 列表：`memory_l0_assembly_snapshots`（最近 20 条），列：时间 / 模型 / 装配段数 / prompt token / used_ratio / warning。
- 点击任意行打开 `QDrawer` 展示 `segments_json` 段落，`QExpansionItem` 内 `<pre>` 显示 preview，「展开完整内容」需管理员权限。

### 8.3 Trace 详情 → memory_recall span 节点

当 span_type = `memory_recall` 时，`QDrawer` 增加：

- 段落统计：history / summary / l1 / l3 / l4 / system 各占多少 token；
- 触发 warning 列表（chip）；
- 「重放」按钮：用 §6.2 `preview` 接口重新装配并对比差异。

### 8.4 Prompt 调试器（可选 P2）

新增 `/agents/:id/debug/prompt` 页：

- 输入框：`user_message`、模型、context_window、reserved_for_output；
- 「装配预览」按钮调用 `POST .../l0/preview`；
- 右侧瀑布展示装配结果，每段 token 占比；
- 「保存为测试用例」可在 §3.3 表打个永久标记 `metadata.test_case = true`。

---

## 9. 写入与读取策略

| 场景 | 行为 |
|------|------|
| 普通对话 | 默认每次模型调用前 Assemble；`snapshot_mode = on_warning` 时仅触发 warning 才落库快照 |
| Team 编排 | 每个子 Agent step 各自 Assemble；snapshot 必须按子 Agent 区分 |
| 重试同一 turn | 复用已有 snapshot ID，更新 `prompt_token_actual` 和 `metadata.retry_count` |
| 流式输出 | Assemble 在 stream 开始前完成；`RecordActual` 在 stream 结束后调用 |
| 失败模型调用 | 仍写入 snapshot，`prompt_token_actual` 取本地估算，`metadata.error_code` 记录错误 |
| 上下文 exceeded | 触发 `context_status = exceeded`，前端弹出「自动压缩 / 建议新建会话」二选一 |

---

## 10. 观测与治理

- 每次 Assemble 在 OpenTelemetry 打 trace：`memory.l0.assemble`，attributes 含 segments 计数与 token 估算。
- Datadog 指标：`aranea.memory.l0.used_ratio`、`aranea.memory.l0.summary_triggered_total`、`aranea.memory.l0.exceeded_total`。
- 异常告警阈值（建议）：单 Agent `exceeded` 比例 24h > 5% → P2 告警；`summary_triggered` 同 session > 5 次/小时 → 检查阈值配置。
- 隐私：`segments_json.preview` 默认前 200 字；`POST .../l0/preview` 返回 preview 而非完整文本；完整内容仅 `audit_logs` 中记录访问。

---

## 11. 落地实施阶段

### Phase 1（基础装配 / 与现有路径打平，1～2 周）

- [ ] 新增 `MemoryL0Service` + `MemoryL0Repository`，落库 §3.3 表。
- [ ] `agent_runtime_settings` ALTER（§3.2）。
- [ ] `MessageRepository.ListLatestByTokens` 实现。
- [ ] `ChatService.SendMessage` / `SendMessageStream` 接入 `Assemble` + `RecordActual`。
- [ ] `TeamRuntime` 每个 step 接入 `Assemble`。
- [ ] 暴露 §6.1、§6.2 的 GET 接口。
- [ ] 单元测试覆盖：滑动窗口、超额裁剪、摘要触发、snapshot 写入。

### Phase 2（摘要与可观测，1 周）

- [ ] 新增 `SummaryService.SummarizeRange`，接入 `session_summaries`。
- [ ] `truncate_strategy = hybrid` 实现（drop_tool_results → summary → drop_oldest）。
- [ ] `session_trace_spans.memory_recall` span 写入与详情联查。
- [ ] 前端 §8.2 上下文 Tab。

### Phase 3（前端调试与扩展，1 周）

- [ ] §8.1 设置面板。
- [ ] §8.3 Trace 详情 memory_recall 节点。
- [ ] §8.4 Prompt 调试器（P2）。

### Phase 4（与 L1/L3/L4 联调，跨阶段）

- 在 L1 文档 §6 接入 `inject_l1`；
- 在 L3 文档 §6 接入 `inject_l3`；
- 在 L4 文档 §6 接入 `inject_l4`。

---

## 12. 验收标准

- [ ] 任意 session 在发送消息时，后端日志可见一条 `memory.l0.assemble` trace。
- [ ] 当对话 turn 数超过 `l0_recent_window_turns` 时，最早的消息不会出现在下一次 prompt 中。
- [ ] 当 `used_ratio_estimate ≥ summary_threshold` 时，`session_summaries` 出现一条新记录，且下一次 prompt 中包含摘要 system message。
- [ ] `sessions.context_used_ratio` 与模型实际返回 `prompt_tokens / context_window` 偏差 ≤ 5%。
- [ ] Team session 中 3 个子 Agent 同时运行时，每个 step 的 `memory_l0_assembly_snapshots` 行可按 `agent_id` 区分。
- [ ] 前端「上下文」Tab 能展示 ratio 趋势、摘要事件、装配快照列表。
- [ ] `POST .../l0/preview` 返回的 `prompt_token_estimate` 与实际调用后的 `prompt_token_actual` 偏差 ≤ 5%。
- [ ] 关闭 `inject_l3` 后，装配段落 `segments_json` 中不出现 `memory.l3` section。
- [ ] 非管理员调用 `/l0/snapshots/{id}` 时返回的 `segments` 仅含 preview，不含完整 content。

---

## 13. 关键设计原则

1. **L0 不是存储层，是装配层**：内容仍来自 `messages` / `session_summaries` / L1 / L3 / L4，L0 只负责按预算拼装。
2. **快照可观测，不必每次写**：默认 `on_warning`，避免高频对话压垮 SQLite。
3. **token 预算优先于行数预算**：行数 fallback 仅作为安全兜底。
4. **摘要是裁剪的最后一步前**：先去掉低价值 tool_results，再压缩，再抛弃，避免无谓信息丢失。
5. **每次装配独立**：Team 中各子 Agent 的 L0 互不影响，避免「一个长输出污染所有人」。
6. **预览即调试**：`/l0/preview` 是产品的「眼睛」，让运营和工程能在不发模型的前提下确认 prompt 形态。
