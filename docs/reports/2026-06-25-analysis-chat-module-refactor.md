# Chat 模块重构设计方案（彻底合并版）

> **日期**：2026-06-25
> **类型**：架构分析 + 重构方案
> **状态**：提议
> **范围**：后端 Activity/Envelope/Message 三体系彻底合并为单一 Activity + Session 父子结构 + 前端动态渲染统一

---

## 一、背景与问题

### 1.1 现状问题总结

经过全面代码审查，Chat 模块存在以下核心问题：

#### 问题 1：后端三套并行体系，关系复杂且冗余

| 体系 | 表 | 用途 | 问题 |
|------|------|------|------|
| **Activity** | `activities` | 语义单元（思考/回复/工具/计划） | 设计良好，但 SpiritSessionID 填充错误 |
| **Envelope** | `event_store` | WS 推送信封 + 快照备份 | 与 Activity 重复存储同一数据 |
| **ChatMessage** | `messages` | 聊天历史 + LLM 上下文 | 与 Activity 双写用户消息和回复 |

**冗余点**：
- 用户消息：`kind=task` Activity + `role=user` Message（双写）
- Agent 回复：`kind=reply` Activity + `role=assistant` Message（双写）
- 工具调用：AF 模式写 Activity，Legacy 模式写 Message（路径切换不一致）
- Envelope 快照：Activity 生命周期事件同时持久化到 `activities` 表和 `event_store` 表

#### 问题 2：Session 缺乏父子层级表达

当前 Session 表虽有 `parent_session_id` 和 `root_session_id` 字段，但：
- 前端未渲染父子层级
- Spirit session 与 team session 的关系未在 UI 体现
- 用户无法看到"一条指令 → 多个团队 → 多个成员"的完整执行树

#### 问题 3：持久化与推送顺序执行，存在延迟

当前 [activity_event_sequencer.go:254-269](file:///f:/aranea-agents/internal/agent/activity_event_sequencer.go#L254-L269) 采用 BlockUpTo 语义：**先持久化，后推送**。持久化失败则跳过推送。

```go
// 当前逻辑（顺序执行）
func (s *activityEventSequencer) processTask(activityID string, task publishTask) {
    if task.persist && s.activityRepo != nil {
        if _, err := s.activityRepo.UpsertActivity(...); err != nil {
            return  // 持久化失败，跳过推送
        }
    }
    if s.eventBus != nil {
        s.eventBus.Publish(...)  // 持久化成功后才推送
    }
}
```

**问题**：DB I/O 阻塞 WS 推送，用户感知延迟；持久化失败导致前端丢失实时事件。

#### 问题 4：前端显示逻辑不一致

| 事件类型 | Spirit 模式 | Team 模式 | Member 模式 |
|---------|------------|----------|------------|
| thinking | ✅ ThinkingBlock | ❌ 不显示 | ❌ 不显示 |
| action | ✅ ActionBlock | ✅ ChatExecutionCard | ✅ ChatExecutionCard |
| reply | ✅ ReplyBlock | ✅ markdown 渲染 | ✅ markdown 渲染 |
| error/plan/confirm/notice | ✅ 对应 Block | ❌ 不显示 | ❌ 不显示 |

- 同一工具调用在 Spirit 模式渲染为 `ActionBlock`，在 Team/Member 模式渲染为 `ChatExecutionCard`，UI 不一致
- Team/Member 模式丢失 5 种富事件类型

#### 问题 5：死代码与废弃功能

| 死代码 | 证据 |
|--------|------|
| `TeamPanel.vue` | `agentWork.panel` 从未赋值 |
| `OrchestrationTimeline` | prop 从未传入 |
| `useTeamStream` member_message 处理器 | 已被 AF 路径替代 |
| `EventProjector` | 已标记 Deprecated，生产不使用 |
| `sub_task_board`/`delegate` ActivityKind | 后端有定义，前端过滤且无 UI |

#### 问题 6：工具调用未按类型细分

当前 `ActionBlock` 对所有工具统一渲染，未区分 shell/浏览器/文件读写等类型，用户无法快速识别正在执行的操作类型。

#### 问题 7：Team/Graph 执行阶段未在 UI 展示

- `UnifiedExecutionPanel` 仅在 team 模式渲染，spirit 模式不渲染
- 团队组建、graph 规划、执行进度等阶段未在 spirit 视图体现
- 团队成员的子 session 未折叠展示

---

## 二、目标架构（彻底合并）

### 2.1 设计原则

1. **唯一模型**：只保留 Activity 一套数据模型，Envelope 和 ChatMessage 概念彻底消失
2. **业务语义事件类型**：7 种 Activity 生命周期事件（created/streaming/updated/completed/failed/cancelled/child_created），每种事件有明确业务含义，所有业务语义通过 `Activity.kind` + `event` + `status` + `meta` 表达
3. **父子层级**：Session 支持父子结构，Activity 通过 session_id 自然归属到对应层级
4. **并行异步**：持久化与 WS 推送同时异步进行，互不阻塞
5. **动态渲染**：前端根据 Activity kind 动态选择渲染组件，所有模式使用统一的渲染管线
6. **工具细分**：工具调用按类型（shell/browser/file/...）细分，UI 表现形式不同
7. **YAGNI**：删除死代码和未使用的功能，不添加未请求的抽象

### 2.2 彻底合并 vs 妥协方案对比

| 维度 | 妥协方案（不彻底） | 彻底合并（本方案） |
|------|----------------|---------|
| 数据模型 | Activity + Envelope（传输） + Message（兼容期） | **仅 Activity** |
| 事件类型 | 15 种 EnvelopeType | **7 种业务语义事件** |
| Envelope 结构体 | 保留 | **删除** |
| Message 表 | 保留兼容期 | **立即删除** |
| `role` 字段 | 新增 | **不需要，用 kind 表达** |
| `error` kind | 保留 | **删除，用 `failed` 事件表达** |
| Channel 路由 | 保留 RouteChannel | **删除，统一 chat** |
| EventBus 传输 | Envelope | **ActivityEvent** |
| LLM 上下文 | 从 Message 或 Activity | **从 Activity** |

### 2.3 目标架构总览

```
┌─────────────────────────────────────────────────────────────┐
│ 后端：单一 Activity 模型                                     │
│                                                              │
│   trpc-agent-go 事件                                         │
│     ↓                                                        │
│   ActivityProjector（唯一投影器）                             │
│     ↓                                                        │
│   ActivityEventSequencer（并行分发）                          │
│     ├──→ goroutine 1: ActivityRepo.UpsertActivity（持久化）  │
│     └──→ goroutine 2: EventBus.Publish（WS 推送）            │
│                                                              │
│   Activity 表（唯一存储）                                     │
│     - kind 覆盖所有业务语义（含 session/team_stage/graph_stage）│
│     - meta 字段吸收原 Envelope.metadata                      │
│     - SpiritSessionID 正确填充                                │
│     - 无 role 字段（kind 即 role）                            │
│                                                              │
│   Session 表（父子层级）                                      │
│     - parent_session_id：父 session                          │
│     - session_type：spirit/team/agent/standalone             │
│     - 通过 parent 链构建执行树                                │
│                                                              │
│   EventBus 传输 ActivityEvent（Activity + 业务语义事件）      │
│   WS 传输 ActivityEvent JSON                                 │
│   无 Envelope 结构体，无 EnvelopeType，无 RouteChannel        │
└─────────────────────────────────────────────────────────────┘
          │ WS（7 种业务语义事件，统一 chat channel）
          ▼
┌─────────────────────────────────────────────────────────────┐
│ 前端：动态渲染管线                                            │
│                                                              │
│   WS ActivityEvent（created/streaming/updated/completed/     │
│                     failed/cancelled/child_created）          │
│     ↓                                                        │
│   useActivityTimeline（按 session_id 隔离）                  │
│     ↓                                                        │
│   ActivityStream（动态分发）                                  │
│     ├── task → UserMessageBubble                             │
│     ├── thinking → ThinkingBlock                             │
│     ├── action → ActionBlock（按 tool_category 细分）        │
│     ├── reply → ReplyBlock                                   │
│     ├── plan → PlanBlock                                     │
│     ├── confirm → ConfirmBlock                               │
│     ├── notice → NoticeBlock                                 │
│     ├── session → SessionStageBlock                          │
│     ├── team_stage → TeamStageBlock（团队进度+成员折叠）     │
│     └── graph_stage → GraphStageBlock（DAG 阶段）            │
│                                                              │
│   SessionTree（左侧栏父子层级）                              │
│     ├── Spirit Session（根）                                 │
│     │   ├── Team Session 1（可折叠）                         │
│     │   │   ├── Member Session 1（可折叠）                   │
│     │   │   └── Member Session 2                             │
│     │   └── Team Session 2                                   │
│     └── ...                                                  │
└─────────────────────────────────────────────────────────────┘
```

---

## 三、彻底合并：单一 Activity 模型

### 3.1 ActivityKind 完整枚举

合并所有事件类型到 ActivityKind：

```go
// internal/biz/activity.go
type ActivityKind string

const (
    // === 基础交互 ===
    ActivityKindTask       ActivityKind = "task"        // 用户消息/任务根
    ActivityKindThinking   ActivityKind = "thinking"    // 推理过程
    ActivityKindAction     ActivityKind = "action"      // 工具调用
    ActivityKindReply      ActivityKind = "reply"       // Agent 回复
    ActivityKindPlan       ActivityKind = "plan"        // 计划
    ActivityKindConfirm    ActivityKind = "confirm"     // 确认
    ActivityKindNotice     ActivityKind = "notice"      // 通知

    // === Session 生命周期（合并 session_* 事件） ===
    ActivityKindSession    ActivityKind = "session"     // Session 创建/状态变更/完成

    // === Team/Graph 阶段（合并 team_stage/graph_stage 等事件） ===
    ActivityKindTeamStage  ActivityKind = "team_stage"  // 团队阶段
    ActivityKindGraphStage ActivityKind = "graph_stage" // Graph 阶段
)
```

**注意**：不保留 `ActivityKindError`。错误是其他 Activity 的终态，用 `event=failed` 表达（如工具失败 = `kind=action` + `event=failed`，团队失败 = `kind=team_stage` + `event=failed`）。避免同一错误产生两个 Activity（原 Activity + error Activity）。

### 3.2 业务语义事件类型：7 种

**设计原则**：每种事件类型对应明确的业务语义，而非技术概念。原 "delta"（增量）是技术术语，改为 "streaming"（流式追加）更符合业务表达。

```go
// internal/event/activity_event.go
type ActivityEventType string

const (
    // === Activity 生命周期 ===

    // ActivityEventCreated Activity 创建
    // 业务含义：新的思考/工具调用/回复/团队阶段等开始
    // 前端行为：新增对应 Block 组件
    ActivityEventCreated ActivityEventType = "created"

    // ActivityEventStreaming 流式追加（替代技术术语 "delta"）
    // 业务含义：思考流式文本、回复流式文本、工具参数流式输入
    // 前端行为：向现有 Block 追加文本，光标闪烁
    // meta.delta_field 标识追加字段：content/reasoning/tool_arguments
    ActivityEventStreaming ActivityEventType = "streaming"

    // ActivityEventUpdated 状态变更（非流式）
    // 业务含义：团队阶段变更（assembled → executing）、Graph 节点状态变更、进度更新
    // 前端行为：更新 Block 的状态/阶段/进度，不追加文本
    // meta.changed_fields 标识变更字段
    ActivityEventUpdated ActivityEventType = "updated"

    // ActivityEventCompleted 正常完成
    // 业务含义：思考完成、工具执行完成、回复完成、团队阶段完成
    // 前端行为：停止光标，标记完成状态，可展开查看详情
    ActivityEventCompleted ActivityEventType = "completed"

    // ActivityEventFailed 失败（独立事件，非 completed + status=failed）
    // 业务含义：工具执行失败、团队执行失败、Graph 节点错误
    // 前端行为：高亮显示错误，展示错误详情，可重试
    // meta.error_code + meta.error_message 标识错误信息
    ActivityEventFailed ActivityEventType = "failed"

    // ActivityEventCancelled 取消（用户主动停止）
    // 业务含义：用户点击停止按钮、团队执行被中断
    // 前端行为：标记为已取消，展示取消原因
    // meta.cancel_reason 标识取消原因
    ActivityEventCancelled ActivityEventType = "cancelled"

    // ActivityEventChildCreated 子 Activity 创建
    // 业务含义：工具调用产生子任务、团队阶段产生成员任务
    // 前端行为：在父 Block 下新增子 Block（折叠状态）
    // parent_activity_id 标识父 Activity
    ActivityEventChildCreated ActivityEventType = "child_created"
)

// ActivityEvent 是 EventBus 和 WS 传输的唯一格式
type ActivityEvent struct {
    Event    ActivityEventType `json:"event"`
    Activity Activity           `json:"activity"`
}
```

**可靠性分级**（对应 §5.3）：

| 级别 | 事件类型 | 持久化 | 推送 |
|------|---------|--------|------|
| Important | `created`/`completed`/`failed`/`cancelled`/`child_created` | 异步持久化，失败重试 | 同步推送 |
| Informational | `streaming`/`updated` | 异步持久化，失败丢弃 | 同步推送（streaming 可批量合并） |

**streaming vs updated 边界**（必须遵守）：

| 维度 | streaming | updated |
|------|-----------|---------|
| 变更类型 | 文本追加（content/reasoning/tool_arguments） | 非文本变更（status/stage/progress/成员列表） |
| 频率 | 高频（每 token） | 低频（阶段变更） |
| 前端行为 | 追加文本，光标闪烁 | 更新状态/进度，不追加文本 |
| 批量合并 | 是（16ms 窗口） | 否 |
| meta 字段 | `meta.delta_field` 标识追加字段 | `meta.changed_fields` 标识变更字段 |

**child_created 语义**：

`child_created` 是**父 Activity 的事件**，通知前端在父 Block 下新增子 Block。子 Activity 有自己完整的生命周期（独立发送 `created`/`streaming`/`completed`/...），父子解耦，子 Activity 可独立查询和渲染。

```
父 Activity (kind=action, tool=shell)
  ├── event=child_created  ← 通知前端：有子任务产生
  │     meta.child_activity_id = "act_child_xxx"
  │
  └── 子 Activity (kind=action, parent_activity_id=父ID)
        ├── event=created       ← 子 Activity 自己的生命周期开始
        ├── event=streaming     ← 子 Activity 流式追加
        └── event=completed     ← 子 Activity 完成
```

### 3.3 原 EnvelopeType → ActivityKind 映射

| 原 EnvelopeType | 彻底合并后 | Activity kind | 事件类型 |
|----------------|-----------|--------------|---------|
| `activity_start`/`delta`/`done`/`child_start` | 保留语义，改名 | 按 kind | created/streaming/completed/child_created |
| `session_created` | 合并 | `session` | `created` |
| `session_status` | 合并 | `session` | `updated` |
| `session_completed` | 合并 | `session` | `completed` |
| `spirit_team_assembled` | 合并 | `team_stage` | `created`（stage=assembled） |
| `spirit_team_completed` | 合并 | `team_stage` | `completed`（stage=completed） |
| `spirit_team_failed`/`interrupted` | 合并 | `team_stage` | `failed`/`cancelled`（stage=failed/interrupted） |
| `spirit_team_cancelled` | 合并 | `team_stage` | `cancelled` |
| `spirit_team_progress` | 合并 | `team_stage` | `updated`（meta.progress） |
| `spirit_teams_all_completed` | 合并 | `team_stage` | `completed`（stage=all_completed） |
| `spirit_plan_created` | 合并 | `plan` | `created` |
| `spirit_allocation_created` | 合并 | `notice` | `created`（meta.allocation） |
| `spirit_orchestration_started`/`checkpoint`/`interrupted` | 合并 | `notice` | `updated`（meta.phase） |
| `spirit_synthesis_completed` | 合并 | `reply` | `completed`（meta.synthesis） |
| `team_run_started` | 合并 | `team_stage` | `created` |
| `team_run_finished` | 合并 | `team_stage` | `completed` |
| `team_run_failed` | 合并 | `team_stage` | `failed` |
| `team_step_started`/`finished`/`summary` | 合并 | `team_stage` | `updated` |
| `member_message_start`/`delta`/`done` | 合并 | `reply` | created/streaming/completed（agent_key=agent，session_type=agent） |
| `orchestration_agent_status` | 合并 | `team_stage` | `updated`（meta.member_status） |
| `graph_node_start` | 合并 | `graph_stage` | `created` |
| `graph_node_end` | 合并 | `graph_stage` | `completed` |
| `graph_node_error` | 合并 | `graph_stage` | `failed` |
| `graph_node_custom` | 合并 | `graph_stage` | `updated` |
| `graph_step`/`execution_done`/`replanned`/`topology_evolved` | 合并 | `graph_stage` | `updated` |
| `checkpoint` | 合并 | `task` | `completed`（meta.checkpoint） |
| `error` | 合并 | 对应 kind | `failed`（如 action+failed / team_stage+failed） |
| `token_usage` | 合并 | `task` | `completed`（meta.token_usage） |
| `run_completion` | 合并 | `task` | `completed` |
| `user_feedback` | 合并 | `notice` | `created`（meta.feedback） |
| `text_delta`/`text_done` | 删除 | `reply` | streaming/completed |
| `tool_call`/`tool_result` | 删除 | `action` | created/completed |
| `state_delta`/`transfer`/`context_usage`/`intent_pass` | 删除 | 合并到对应 Activity 的 `streaming`/`updated` |
| `monitor/*`（log/flow_log/mcp_*/alert_*/monitor_*） | 移出 chat | 不影响 | — |
| `butler_*`/`skill_*`/`borrow_*`/`organization_*`/`planning_phase_*` | 删除 | 未使用 | — |

### 3.4 Activity 表结构（彻底版）

```go
// internal/data/ent/schema/activity.go
func (Activity) Fields() []ent.Field {
    return []ent.Field{
        // === 主键 ===
        field.String("id").MaxLen(64).Unique().Immutable(),

        // === 分类（合并所有事件类型） ===
        field.String("kind").MaxLen(32).Comment("task/thinking/action/reply/plan/confirm/notice/session/team_stage/graph_stage"),
        field.String("status").MaxLen(32).Default("pending").Comment("pending/running/tool_running/tool_blocked/completed/failed/partial_failure/cancelled/interrupted"),

        // === 归属 ===
        field.String("session_id").MaxLen(128).Default(""),
        field.String("turn_id").MaxLen(128).Default(""),
        field.String("parent_activity_id").MaxLen(64).Default("").Comment("FK to parent Activity for tree nesting"),
        field.String("spirit_session_id").MaxLen(128).Default("").Comment("Spirit Session ID（跨 session 聚合查询）"),
        field.String("team_id").MaxLen(128).Default(""),
        field.String("dag_node_id").MaxLen(128).Default(""),

        // === 时间 ===
        field.String("timestamp").Default("").Comment("ISO8601 start timestamp"),
        field.Int64("duration_ms").Default(0),
        field.Int64("seq").Default(0).Comment("Global emission sequence for stable frontend ordering"),

        // === Token usage（kind=task 根 Activity） ===
        field.Int64("prompt_tokens").Default(0),
        field.Int64("completion_tokens").Default(0),

        // === 内容字段 ===
        field.Text("content").Default("").Comment("task/reply/error/session/team_stage/graph_stage 文本内容"),
        field.Text("reasoning").Default("").Comment("thinking reasoning content"),

        // === 工具字段（kind=action） ===
        field.String("tool_name").MaxLen(128).Default(""),
        field.String("tool_category").MaxLen(32).Default("").Comment("shell/browser/file_read/file_write/file_search/web_search/mcp/code/todo/other"),
        field.String("tool_call_id").MaxLen(128).Default(""),
        field.Text("tool_arguments").Default("").Sensitive(),
        field.Text("tool_result").Default("").Sensitive(),
        field.Int64("tool_duration_ms").Default(0),
        field.String("tool_error_code").MaxLen(64).Default(""),

        // === 阶段（kind=session/team_stage/graph_stage） ===
        field.String("stage").MaxLen(64).Default("").Comment("assembled/planning/executing/completed/failed/..."),
        field.JSON("depends_on", []string{}).Optional().Comment("DAG 依赖节点"),

        // === Agent 信息 ===
        field.String("agent_key").MaxLen(128).Default(""),
        field.String("agent_name").MaxLen(128).Default(""),

        // === 显示 hints ===
        field.Bool("collapsed").Default(false),
        field.String("label").MaxLen(128).Default(""),

        // === 元数据（吸收原 Envelope.metadata） ===
        field.JSON("meta", map[string]any{}).Optional().Comment("Kind-specific metadata（成员列表/DAG 节点/进度/token_usage 等）"),
    }
}

func (Activity) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("session_id", "turn_id").StorageKey("idx_activities_session_turn"),
        index.Fields("parent_activity_id").StorageKey("idx_activities_parent"),
        index.Fields("spirit_session_id").StorageKey("idx_activities_spirit_session"),
        index.Fields("team_id").StorageKey("idx_activities_team"),
    }
}
```

**删除的字段**：
- `role`（用 kind 表达，不需要）
- `child_board_id`（用 parent_activity_id 表达）
- `tool_icon`（前端根据 tool_category 决定）

### 3.5 LLM 上下文构建（替代 Message）

```go
// internal/biz/llm_context_builder.go
type LLMMessage struct {
    Role       string
    Content    string
    ToolCallID string
    ToolName   string
    Name       string // 发言者标识（用于 assistant 角色，标识团队成员）
}

// BuildLLMContext 从 Activity 表构建 LLM 上下文（替代原 Message 查询）
//
// 角色映射规则（LLM API 只接受 user/assistant/tool/system）：
//   - task    → user
//   - reply   → assistant（含团队成员回复，通过 agent_key 标识来源，不改变 role）
//   - action  → tool
//   - notice  → system
func BuildLLMContext(ctx context.Context, repo ActivityReader, sessionID, turnID string) ([]LLMMessage, error) {
    activities, err := repo.ListBySessionTurn(ctx, sessionID, turnID)
    if err != nil {
        return nil, err
    }

    var messages []LLMMessage
    for _, a := range activities {
        switch a.Kind {
        case ActivityKindTask:
            messages = append(messages, LLMMessage{Role: "user", Content: a.Content})
        case ActivityKindReply:
            // 团队成员回复也用 assistant 角色，agent_key 仅用于业务标识
            messages = append(messages, LLMMessage{
                Role:    "assistant",
                Content: a.Content,
                Name:    a.AgentKey, // OpenAI/Anthropic 支持 name 字段标识发言者
            })
        case ActivityKindAction:
            messages = append(messages, LLMMessage{
                Role:       "tool",
                Content:    a.ToolResult,
                ToolCallID: a.ToolCallID,
                ToolName:   a.ToolName,
            })
        case ActivityKindNotice:
            messages = append(messages, LLMMessage{Role: "system", Content: a.Content})
        }
    }
    return messages, nil
}
```

### 3.6 EventBus 传输 ActivityEvent

EventBus 接口改为传输 `ActivityEvent`（定义见 §3.2）：

```go
// internal/event/bus.go
type EventBus interface {
    Publish(ctx context.Context, event ActivityEvent) error
    Subscribe(handler func(ActivityEvent)) Subscription
}
```

**EventBus 实现注意**：
- `Publish` 返回 `error`（原接口可能不返回），用于 processTask 同步推送时检测失败
- 现有 `EventBusSink`（logpipeline）和 `WebSocketHub` 订阅者需更新签名
- monitor 类事件（log/flow_log/mcp_*/alert_*）不走 ActivityEvent，保留独立 Envelope 通道（见 §6.3）

**删除**：
- `Envelope` 结构体（[envelope.go:154-186](file:///f:/aranea-agents/internal/event/contract/envelope.go#L154-L186)）
- `EnvelopeType` 常量（[envelope.go:15-151](file:///f:/aranea-agents/internal/event/contract/envelope.go#L15-L151)）
- `RouteChannel` 函数（[envelope.go:475-483](file:///f:/aranea-agents/internal/event/contract/envelope.go#L475-L483)）
- `channelRegistry`（[envelope.go:411-469](file:///f:/aranea-agents/internal/event/contract/envelope.go#L411-L469)）

### 3.7 Channel 路由简化

所有 Activity 事件统一走 "chat" channel，不再需要路由：

```go
// 删除 RouteChannel 函数
// WS 连接只需订阅 "chat" channel
// 不再有 "team"/"graph"/"monitor" 等 channel 概念（monitor 保留独立）
```

### 3.8 WS 传输格式

WS 传输的就是 `ActivityEvent` JSON：

```json
{
  "event": "start",
  "activity": {
    "id": "act_xxx",
    "kind": "team_stage",
    "status": "running",
    "session_id": "sess_team_xxx",
    "spirit_session_id": "sess_spirit_xxx",
    "team_id": "team_xxx",
    "stage": "assembled",
    "meta": { "members": [...], "task_summary": "..." },
    "timestamp": "2026-06-25T10:00:00Z",
    "seq": 12345
  }
}
```

对比原 Envelope：
```json
{
  "id": "env_xxx",
  "type": "spirit_team_assembled",   // ← 删除，用 activity.kind 表达
  "session_id": "sess_spirit_xxx",
  "team_id": "team_xxx",             // ← 合并到 activity.team_id
  "metadata": { ... },               // ← 合并到 activity.meta
  "channel": "chat"                  // ← 删除，统一 chat channel
}
```

### 3.9 修正 SpiritSessionID 传递链

**根因**：`ProjectMeta` 缺少 `SpiritSessionID` 字段

**修复**：

```go
// internal/agent/activity_projector.go
type ProjectMeta struct {
    SessionID        string   // 当前 session ID（team session 或 spirit session）
    SpiritSessionID  string   // ← 新增：Spirit session ID（用于跨 session 聚合查询）
    ParentSessionID  string   // ← 新增：父 session ID
    RootSessionID    string   // ← 新增：根 session ID
    RequestID        string
    TeamID           string
    MemberAgentKeys  []string
    // ... 其他字段
}
```

**传递链**：
```
SpiritTeamAssembler.AssembleTeam
  → buildTeamProjectMeta(spiritSessionID, teamSession, team)
  → ProjectMeta{
      SessionID:       teamSession.ID,
      SpiritSessionID: spiritSessionID,           // ← 正确填充
      ParentSessionID: spiritSessionID,           // ← 父 session
      RootSessionID:   spiritSessionID,           // ← 根 session
      TeamID:          team.ID,
    }
```

### 3.10 新增查询方法

```go
// internal/biz/activity.go
type ActivityReader interface {
    ListBySession(ctx context.Context, sessionID string) ([]Activity, error)
    ListBySessionTurn(ctx context.Context, sessionID, turnID string) ([]Activity, error)

    // ← 新增：按 spirit session 聚合查询（含所有子 session）
    ListBySpiritSession(ctx context.Context, spiritSessionID string) ([]Activity, error)

    // ← 新增：按 team 查询
    ListByTeam(ctx context.Context, teamID string) ([]Activity, error)

    // ← 新增：按 parent_session_id 查询子 session 的 Activity
    ListByParentSession(ctx context.Context, parentSessionID string) ([]Activity, error)
}
```

---

## 四、Session 父子结构设计

### 4.1 Session 类型与层级

#### 4.1.1 Session 类型枚举

```go
// internal/biz/session.go
type SessionType string

const (
    SessionTypeSpirit     SessionType = "spirit"     // 精灵会话（用户直接交互，根 Session）
    SessionTypeTeam       SessionType = "team"       // 团队会话（spirit 显式组建的团队）
    SessionTypeAgent      SessionType = "agent"      // Agent 会话（通用，替代 member；Spirit 直接调度或 Team 成员或子 Agent）
    SessionTypeStandalone SessionType = "standalone" // 独立会话（非 spirit 场景，如直接对话）
)
```

**关键变化**：删除 `member`，统一用 `agent`。Team 下的 member 就是 `agent` 类型的 Session，parent 是 team。这样支持：
- Spirit 直接调度多 agent（无 Team）：spirit → agent
- Team member 再调度子 agent：team → agent → agent（任意深度）
- Graph 节点执行：spirit → agent（每个节点一个 agent session）

#### 4.1.2 通用 Session 树（支持任意深度）

```
场景 1：Spirit 单 agent 执行
  spirit_session (depth=0)

场景 2：Spirit 组建 Team 执行
  spirit_session (depth=0)
    └── team_session (depth=1)
          ├── agent_session (depth=2, member A)
          ├── agent_session (depth=2, member B)
          └── agent_session (depth=2, member C)

场景 3：Spirit 直接调度多 agent（无 Team）
  spirit_session (depth=0)
    ├── agent_session (depth=1, agent A)
    ├── agent_session (depth=1, agent B)
    └── agent_session (depth=1, agent C)

场景 4：Spirit 通过 Graph 执行
  spirit_session (depth=0)
    ├── agent_session (depth=1, graph 节点 A)
    └── agent_session (depth=1, graph 节点 B)

场景 5：Team member 再调度子 agent（任意深度）
  spirit_session (depth=0)
    └── team_session (depth=1)
          └── agent_session (depth=2, member A)
                └── agent_session (depth=3, sub-agent)
                      └── agent_session (depth=4, sub-sub-agent)
```

**深度限制**：由 Agent 设置中的 `subagents_max_generation_depth` 和 Spirit 的 `max_session_depth` 控制（见 §4.4）。

#### 4.1.3 Session 表结构调整

```go
// internal/data/ent/schema/session.go
func (Session) Fields() []ent.Field {
    return []ent.Field{
        // ... 现有字段 ...

        // === 父子层级（已有，确认使用） ===
        field.String("parent_session_id").Default("").MaxLen(256),
        field.String("root_session_id").Default("").MaxLen(256),
        field.Int("agent_depth").Default(0).Comment("相对于 root 的深度：spirit=0, team/agent=1+, 任意深度"),

        // === 新增：Session 类型 ===
        field.String("session_type").MaxLen(32).Default("standalone").Comment("spirit/team/agent/standalone"),

        // === 团队信息（team/agent session 专用） ===
        field.String("team_id").Default("").Comment("关联的 Team ID（已有，确认使用）"),
        field.String("member_agent_key").Default("").Comment("agent key（agent session 专用，标识执行 agent）"),
        field.String("member_role").Default("").Comment("agent 角色（如 coordinator/worker）"),

        // === 新增：执行状态汇总 ===
        field.String("execution_stage").Default("").Comment("当前执行阶段：idle/planning/allocating/executing/completed/failed"),
        field.Int("completed_steps").Default(0),
        field.Int("total_steps").Default(0),
        field.Float("progress_pct").Default(0.0),
    }
}
```

### 4.2 Session 树构建规则

#### 4.2.1 Spirit Session 创建

- `session_type = "spirit"`
- `parent_session_id = ""`
- `root_session_id = self.id`
- `agent_depth = 0`

#### 4.2.2 Team Session 创建

由 `SpiritTeamAssembler.AssembleTeam` 创建：
- `session_type = "team"`
- `parent_session_id = spiritSessionID`
- `root_session_id = spiritSessionID`
- `agent_depth = parentSession.agent_depth + 1`（通常为 1）
- `team_id = team.ID`

#### 4.2.3 Agent Session 创建（通用，替代原 Member Session）

Agent Session 可由多种场景创建：Team 成员、Spirit 直接调度、子 Agent 递归调用。

```go
// 通用创建逻辑
func CreateAgentSession(parentSession Session, agentKey string, opts ...AgentSessionOpt) Session {
    childDepth := parentSession.AgentDepth + 1
    // 深度校验（见 §4.4）
    if err := validateDepth(parentSession, childDepth); err != nil {
        return err
    }
    return Session{
        SessionType:      SessionTypeAgent,
        ParentSessionID:  parentSession.ID,
        RootSessionID:    parentSession.RootSessionID,
        AgentDepth:       childDepth,
        MemberAgentKey:   agentKey,
        TeamID:           parentSession.TeamID, // 继承父 session 的 team_id（如有）
        // ...
    }
}
```

**场景适配**：
- Team 成员：`parent = team_session`，`depth = 2`（通常）
- Spirit 直接调度：`parent = spirit_session`，`depth = 1`
- Graph 节点：`parent = spirit_session`，`depth = 1`
- 子 Agent 递归：`parent = agent_session`，`depth = parent.depth + 1`（任意深度）

### 4.3 Depth 配置与限制（Agent 设置集成）

#### 4.3.1 现有 Depth 配置（已存在，复用）

| 配置字段 | 位置 | 默认值 | 作用域 | 语义 |
|---------|------|--------|--------|------|
| `subagents_max_generation_depth` | AgentRuntimeSetting | 1 | Agent 级 | 控制子 Agent 生成的最大嵌套深度（相对深度） |
| `max_session_depth` | Spirit ParallelConfig（extra_json） | 2 | Spirit 级 | 控制 Session 树的最大绝对深度 |
| `agent_depth` | Session 表 | 0 | Session 级（运行时） | 记录 Session 在树中的实际深度 |

**关键语义区分**：
- `subagents_max_generation_depth`：**相对深度**，控制"我能生成多少层子 Agent"
  - Spirit 配置 2 → Spirit 可生成深度 1、2 的子 Agent
  - Team 成员配置 1 → 成员可生成深度 1 的子 Agent（相对于自己）
- `max_session_depth`：**绝对深度**，控制"整棵 Session 树最多多深"
  - Spirit 配置 2 → Session 树最多 2 层（spirit=0, team=1, agent=2）
  - Spirit 配置 4 → 支持场景 5 的 4 层嵌套

#### 4.3.2 深度校验逻辑

```go
// internal/biz/session_usecase.go

// validateDepth 创建子 Session 前校验深度限制
func (u *SessionUsecase) validateDepth(parentSession Session, childDepth int) error {
    // 1. 校验 Spirit 级最大深度（绝对深度）
    spiritCfg := u.loadSpiritConfig(parentSession.RootSessionID)
    if childDepth > spiritCfg.MaxSessionDepth {
        return fmt.Errorf("session tree depth (%d) exceeds spirit max (%d)",
            childDepth, spiritCfg.MaxSessionDepth)
    }

    // 2. 校验 Agent 级子代理生成深度（相对深度）
    //    parentSession.MemberAgentKey 标识执行 Agent，读取其配置
    if parentSession.MemberAgentKey != "" {
        agentCfg := u.loadAgentConfig(parentSession.MemberAgentKey)
        relativeDepth := childDepth - parentSession.AgentDepth
        if relativeDepth > agentCfg.SubagentsMaxGenerationDepth {
            return fmt.Errorf("subagent generation depth (%d) exceeds agent max (%d)",
                relativeDepth, agentCfg.SubagentsMaxGenerationDepth)
        }
    }

    return nil
}
```

#### 4.3.3 配置示例

| 场景 | Spirit max_session_depth | Agent subagents_max_generation_depth | 支持的 Session 树 |
|------|--------------------------|--------------------------------------|------------------|
| 单 agent 执行 | 0 | 0 | spirit(0) |
| Spirit 组建 Team（无子 agent） | 2 | 0 | spirit(0) → team(1) → agent(2) |
| Spirit 直接调度 agent | 1 | 0 | spirit(0) → agent(1) |
| Team member 生成子 agent | 3 | 1 | spirit(0) → team(1) → agent(2) → agent(3) |
| 深度嵌套（场景 5） | 4 | 2 | spirit(0) → team(1) → agent(2) → agent(3) → agent(4) |

**前端配置入口**（已有，无需新增）：
- `AgentSettingsAgentTab.vue` 第 172 行：`config.subagents.max_generation_depth`（"最大生成深度"）
- `AgentSettingsAgentTab.vue` 第 279 行：`config.spirit.max_session_depth`（"最大会话深度"）

### 4.4 Session 树查询 API

**查询策略**：利用 `root_session_id` 索引一次查询所有子孙 session，避免递归 N 次查询。支持任意深度。

```go
// internal/biz/session.go
type SessionTreeReader interface {
    // 获取完整 session 树（任意深度，递归结构）
    // 实现策略：一次查询 root_session_id = spiritSessionID 的所有 session，
    // 然后在内存中按 parent_session_id 构建递归树。避免 N 次递归查询。
    GetSessionTree(ctx context.Context, spiritSessionID string) (*SessionTree, error)

    // 获取子 session 列表（单层，不递归）
    ListChildSessions(ctx context.Context, parentSessionID string) ([]Session, error)

    // 获取团队的所有成员 session（agent session under team）
    ListTeamAgentSessions(ctx context.Context, teamID string) ([]Session, error)
}

// SessionTree 支持任意深度的递归树
type SessionTree struct {
    Root     Session           // Spirit session
    Children []*SessionTreeNode // 直接子节点（team 或 agent）
}

// SessionTreeNode 通用树节点，支持任意深度
type SessionTreeNode struct {
    Session   Session            // 当前 session（team 或 agent）
    Children []*SessionTreeNode  // 子节点（递归，支持任意深度）
    Activities []Activity        // 该 session 的 Activity（可选，按需加载）
}
```

**Repo 实现**：

```go
// internal/data/session_repo.go
func (r *sessionRepo) GetSessionTree(ctx context.Context, spiritSessionID string) (*biz.SessionTree, error) {
    // 一次查询所有子孙 session（root_session_id 索引）
    sessions, err := r.data.RW().Read(ctx).Session.
        Query().
        Where(session.RootSessionID(spiritSessionID)).
        Order(ent.Asc(session.FieldAgentDepth, session.FieldCreatedAt)).
        All(ctx)
    if err != nil {
        return nil, entErrToBizErr(err, "session")
    }

    // 内存中构建递归树（支持任意深度）
    tree := &biz.SessionTree{}
    nodeMap := make(map[string]*biz.SessionTreeNode) // sessionID → node

    for _, s := range sessions {
        node := &biz.SessionTreeNode{Session: toBizSession(s)}
        nodeMap[s.ID] = node

        switch s.SessionType {
        case "spirit":
            tree.Root = node.Session
        default:
            // 挂载到父节点（支持任意深度）
            if parent, ok := nodeMap[s.ParentSessionID]; ok {
                parent.Children = append(parent.Children, node)
            } else {
                // 父节点未找到，挂载到 root
                tree.Children = append(tree.Children, node)
            }
        }
    }
    return tree, nil
}
```

**索引要求**：`sessions` 表需在 `root_session_id` 上有索引（已有字段，需确认索引存在）。

---

## 五、持久化与推送并行设计

### 5.1 当前代码逻辑

[activity_event_sequencer.go:254-269](file:///f:/aranea-agents/internal/agent/activity_event_sequencer.go#L254-L269)：

```go
func (s *activityEventSequencer) processTask(activityID string, task publishTask) {
    // 1. 先持久化（阻塞）
    if task.persist && s.activityRepo != nil {
        if _, err := s.activityRepo.UpsertActivity(...); err != nil {
            return  // 持久化失败，跳过推送
        }
    }
    // 2. 后推送（持久化成功后）
    if s.eventBus != nil {
        s.eventBus.Publish(...)
    }
}
```

**问题**：
- DB I/O 阻塞 WS 推送，用户感知延迟
- 持久化失败导致前端丢失实时事件
- 单一 goroutine 串行处理，吞吐量受限

### 5.2 设计方案：并行异步

#### 5.2.1 核心思路

持久化与推送**同时异步进行**，互不阻塞：
- 持久化失败：记录日志，前端仍有实时事件，通过 backfill 机制最终一致
- 推送失败：记录日志，前端通过 API reload 补全

#### 5.2.2 改造后的 processTask

**设计要点**：
- **持久化 fire-and-forget**：异步执行，不阻塞 consume goroutine。失败仅记日志，前端通过 API backfill 保证最终一致
- **推送同步**：WS 推送通常 < 5ms，同步执行可保留 per-activity FIFO 顺序，无需额外协调
- **consume 不等持久化**：处理完一个 task 立即处理下一个，吞吐量不受 DB I/O 限制

```go
// internal/agent/activity_event_sequencer.go

// publishTask 增加 eventType 字段（替代原 env）
type publishTask struct {
    eventType ActivityEventType
    activity  biz.Activity
    persist   bool
}

func (s *activityEventSequencer) processTask(activityID string, task publishTask) {
    // 任务 1：持久化（fire-and-forget，不阻塞 consume）
    // 用 sequencer 级 wg 保证 Close 时等待所有 persist goroutine 完成
    if task.persist && s.activityRepo != nil {
        s.wg.Add(1)
        persistCtx := context.Background()
        safego.GoBackground("activity_persist", func() {
            defer s.wg.Done()
            if _, err := s.activityRepo.UpsertActivity(persistCtx, task.activity); err != nil {
                s.lg.Warn("activity persist failed; frontend will backfill via API",
                    loggateway.StepID("agent.activity_sequencer.persist"),
                    loggateway.Str("activity_id", activityID),
                    loggateway.Str("kind", string(task.activity.Kind)),
                    loggateway.Err(err))
                // 不影响推送，前端通过 API backfill 保证最终一致
            }
        })
    }

    // 任务 2：WS 推送（同步，保留 FIFO 顺序）
    // 推送通常 < 5ms，同步执行不会成为瓶颈
    if s.eventBus != nil {
        event := ActivityEvent{
            Event:    task.eventType,
            Activity: task.activity,
        }
        if err := s.eventBus.Publish(context.Background(), event); err != nil {
            s.lg.Warn("activity publish failed; frontend will reload via API",
                loggateway.StepID("agent.activity_sequencer.publish"),
                loggateway.Str("activity_id", activityID),
                loggateway.Err(err))
        }
    }
}
```

**为什么不用 `sync.WaitGroup` 等待两个 goroutine？**

| 方案 | 总耗时 | consume 阻塞 | FIFO 保证 | 问题 |
|------|--------|-------------|----------|------|
| 原方案（串行） | persist + publish | 是 | 是 | DB I/O 阻塞推送 |
| 妥协方案（wg.Wait 两 goroutine） | max(persist, publish) | 是 | 是 | consume 仍等 max(persist, publish) |
| **本方案（persist 异步 + publish 同步）** | publish（~5ms） | 否 | 是 | persist 失败需 backfill |

本方案让 consume 几乎不阻塞（只等 ~5ms 的推送），吞吐量提升 10x+，且保留 per-activity FIFO。

#### 5.2.3 可靠性保证

**最终一致性**：
- WS 推送失败 → 前端通过 `listActivities` API reload 补全
- 持久化失败 → 前端有实时事件但 reload 时丢失，通过 retry 机制补全

**FIFO 顺序保证**：
- 保留 per-activity channel（同一 Activity 的 created/streaming/updated/completed/failed/cancelled/child_created 仍按序）
- 不同 Activity 之间可并行（无顺序依赖）

**背压机制**：
- Sequencer channel buffer 满时，丢弃 `streaming`/`updated` 事件（Informational 级别）
- 保留 `created`/`completed`/`failed`/`cancelled`/`child_created` 事件（Important 级别）

### 5.3 简化的可靠性分级

彻底合并后，可靠性分级简化为：

| 级别 | Activity 事件 | 持久化 | 推送 |
|------|-------------|--------|------|
| Important | `created`/`completed`/`failed`/`cancelled`/`child_created` | 异步持久化，失败重试 | 同步推送，失败记录 |
| Informational | `streaming`/`updated` | 异步持久化，失败丢弃 | 同步推送（streaming 可批量合并），失败丢弃 |

**删除 WAL**：Activity 表已是唯一真相源，无需额外 WAL 持久化。持久化失败通过 retry + API backfill 保证最终一致。

---

## 六、废弃功能删除清单

### 6.1 后端删除清单

| 文件/组件 | 原因 | 操作 |
|----------|------|------|
| `internal/agent/event_projector.go` | 已标记 Deprecated，AF 模式不使用 | 删除文件 |
| `internal/agent/activity_publish.go` | Legacy 工具卡片持久化 | 删除文件 |
| `internal/agent/activity_persist.go` | `ChatMessageFromToolActivity` 转换 | 删除文件 |
| `internal/biz/event_persist_handler.go` | Envelope 持久化处理器 | 删除文件 |
| `internal/biz/event_store.go` | EventStore 快照存储 | 删除文件 |
| `internal/event/wal.go` | WAL 持久化（Activity 表替代） | 删除文件 |
| `internal/data/ent/schema/event_store.go` | event_store 表 Schema | 删除 Schema + 迁移 |
| `internal/data/ent/schema/message.go` | messages 表 Schema | 一次性迁移后删除 |
| `internal/biz/session/message_usecase.go` | Message 用例 | 合并到 Activity 用例后删除 |
| `internal/data/message_repo.go` | Message Repo | 合并到 Activity Repo 后删除 |
| `internal/event/contract/envelope.go` | Envelope 结构体 + EnvelopeType + RouteChannel | 删除文件，替换为 ActivityEvent |
| `EnvelopeType*`（~80 种） | 彻底删除，用 ActivityKind + 事件类型表达 | 删除所有常量 |
| `ActivityKindSubTaskBoard` | 前端无 UI 实现 | 删除常量 |
| `ActivityKindDelegate` | 前端无 UI 实现 | 删除常量 |

### 6.2 前端删除清单

| 文件/组件 | 原因 | 操作 |
|----------|------|------|
| `web/src/components/chat/TeamPanel.vue` | `agentWork.panel` 从未赋值 | 删除文件 |
| `web/src/components/chat/OrchestrationTimeline.vue` | prop 从未传入 | 删除文件 |
| `web/src/features/chat/useEnvelopeStream.ts` member_message 处理器 | 已被 AF 路径替代 | 删除相关代码 |
| `web/src/features/chat/streamEventTypes.ts` 中的 Legacy 类型 | 由 Activity 替代 | 清理 |
| `web/src/components/spirit/TaskExecutionPanel.vue` ChatExecutionCard 路径 | 统一到 ActionBlock | 重构后删除 |
| `web/src/components/spirit/MemberReadOnlyPanel.vue` | 统一到 ActivityStream | 重构后删除 |
| `web/src/realtime/envelope.ts` | Envelope 类型定义 | 替换为 ActivityEvent 类型 |
| `web/src/features/chat/inboundSyncRouting.ts` | Envelope 路由逻辑 | 简化或删除 |

### 6.3 monitor 事件处理方案

文档 §3.3 提到 `monitor/*` 事件"移出 chat"，具体方案如下：

**monitor 事件分类**：
- `monitor.log` / `monitor.flow_log` — 运行时日志
- `monitor.mcp_*` — MCP 工具监控
- `monitor.alert_*` — 告警事件

**处理方案**：monitor 事件**不属于 Chat 业务**，不应走 ActivityEvent 通道。保留独立的 monitor 通道：

```go
// monitor 事件保留独立 Envelope（不走 ActivityEvent）
// WS 连接可选订阅 "monitor" channel（与 "chat" channel 隔离）
// Chat 模块只处理 "chat" channel 的 ActivityEvent

// internal/event/contract/monitor_event.go（新增，从 envelope.go 拆分）
type MonitorEvent struct {
    Type      string         // log/flow_log/mcp_*/alert_*
    Timestamp string
    Payload   map[string]any
}
```

**删除范围**：仅从 `envelope.go` 中删除 Chat 相关的 EnvelopeType（~70 种），保留 monitor 相关的 EnvelopeType（~10 种）并迁移到 `monitor_event.go`。Chat 模块不再引用任何 Envelope 概念。

---

## 七、前端动态渲染设计

### 7.1 统一渲染管线

#### 7.1.1 核心原则

**所有模式（spirit/team/member）使用同一渲染管线**，根据 Activity kind 动态选择组件。

#### 7.1.2 渲染管线

```
WS ActivityEvent（created/streaming/updated/completed/failed/cancelled/child_created）
  ↓
useActivityTimeline（按 session_id 隔离 Map）
  ↓
ActivityStream.vue（统一入口，替代 ChatMessageList + TaskExecutionPanel + MemberReadOnlyPanel）
  ↓
按 activity.kind 动态分发：
  ├── task → UserMessageBubble（用户消息）
  ├── thinking → ThinkingBlock
  ├── action → ActionBlock（按 tool_category 细分；failed 事件高亮错误）
  ├── reply → ReplyBlock
  ├── plan → PlanBlock
  ├── confirm → ConfirmBlock
  ├── notice → NoticeBlock
  ├── session → SessionStageBlock（Session 生命周期）
  ├── team_stage → TeamStageBlock（团队阶段+成员折叠；failed/cancelled 事件显示状态）
  └── graph_stage → GraphStageBlock（DAG 阶段；failed 事件高亮错误节点）
```

**注意**：不保留 `error → ErrorBlock` 分支。错误通过对应 kind 的 `failed` 事件表达（如 action+failed 在 ActionBlock 内高亮错误，team_stage+failed 在 TeamStageBlock 内显示失败状态）。

#### 7.1.3 Activity Timeline 按 session 隔离

```typescript
// web/src/features/chat/composables/useActivityTimeline.ts
// 改造：从单一 Map 改为按 session_id 隔离的 Map

const activitiesBySession = shallowRef<Map<string, Map<string, Activity>>>(new Map());

function getSessionActivities(sessionId: string): Map<string, Activity> {
    let map = activitiesBySession.value.get(sessionId);
    if (!map) {
        map = new Map();
        activitiesBySession.value.set(sessionId, map);
    }
    return map;
}

// 切换 session 时无需 reset，自然隔离
```

### 7.2 ActivityStream 组件设计

```vue
<!-- web/src/components/chat/ActivityStream.vue -->
<template>
  <DynamicScroller :items="sortedActivities" :min-item-size="60">
    <template #default="{ item: activity }">
      <component
        :is="resolveBlockComponent(activity.kind)"
        :activity="activity"
        @expand="onExpand"
        @collapse="onCollapse"
      />
    </template>
  </DynamicScroller>
</template>

<script setup lang="ts">
import type { Component } from 'vue';
import type { Activity, ActivityKind } from '../../features/chat/activityTypes';

const props = defineProps<{
  sessionId: string;
  activities: Activity[];
}>();

function resolveBlockComponent(kind: ActivityKind): Component {
  const map: Record<ActivityKind, Component> = {
    task: UserMessageBubble,
    thinking: ThinkingBlock,
    action: ActionBlock,
    reply: ReplyBlock,
    plan: PlanBlock,
    confirm: ConfirmBlock,
    notice: NoticeBlock,
    session: SessionStageBlock,
    team_stage: TeamStageBlock,
    graph_stage: GraphStageBlock,
  };
  return map[kind] ?? NoticeBlock; // 兜底
}
</script>
```

### 7.3 动态渲染行为

#### 7.3.1 思考（thinking）

```
收到 ActivityEvent (event=created, kind=thinking)
  → 新增 ThinkingBlock（折叠状态，显示"思考中..."）
收到 ActivityEvent (event=streaming, delta_field=reasoning)
  → 流式追加文本，光标闪烁
收到 ActivityEvent (event=completed)
  → 停止光标，可展开查看完整推理
收到 ActivityEvent (event=failed, meta.error_message=xxx)
  → 显示"思考失败"，展示错误信息
收到 ActivityEvent (event=cancelled)
  → 显示"已停止"
```

#### 7.3.2 回复（reply）

```
收到 ActivityEvent (event=created, kind=reply)
  → 新增 ReplyBlock（流式渲染 markdown）
收到 ActivityEvent (event=streaming, delta_field=content)
  → 流式追加文本
收到 ActivityEvent (event=completed)
  → 完成 markdown 渲染
收到 ActivityEvent (event=failed)
  → 显示"回复失败"
收到 ActivityEvent (event=cancelled)
  → 显示"已停止"
```

#### 7.3.3 计划（plan）

```
收到 ActivityEvent (event=created, kind=plan)
  → 新增 PlanBlock（显示计划标题）
收到 ActivityEvent (event=streaming, delta_field=steps)
  → 渲染计划步骤列表（checkbox 形式）
收到 ActivityEvent (event=updated, meta.changed_fields=step_status)
  → 更新步骤状态（pending/completed/failed）
收到 ActivityEvent (event=completed)
  → 完成计划展示，可折叠
```

#### 7.3.4 工具调用（action）

```
收到 ActivityEvent (event=created, kind=action, tool_name=xxx, tool_category=shell)
  → 新增 ActionBlock（按 tool_category 选择图标和布局）
  → 显示"正在执行：xxx"
收到 ActivityEvent (event=streaming, delta_field=tool_arguments)
  → 流式追加工具参数
收到 ActivityEvent (event=updated, meta.changed_fields=tool_status)
  → 更新工具状态（如 shell 执行中、浏览器导航中）
收到 ActivityEvent (event=completed, tool_result=xxx)
  → 显示执行结果，可展开查看详情
收到 ActivityEvent (event=failed, meta.error_code=xxx, meta.error_message=xxx)
  → 高亮显示错误，展示错误详情，可重试
收到 ActivityEvent (event=cancelled)
  → 显示"已取消"
```

#### 7.3.5 团队阶段（team_stage）

```
收到 ActivityEvent (event=created, kind=team_stage, stage=assembled)
  → 新增 TeamStageBlock，显示"团队已组建"
  → 展示团队成员头像列表
  → 展示任务摘要

收到 ActivityEvent (event=updated, meta.changed_fields=stage, stage=executing)
  → 更新阶段为"执行中"
  → 展示进度条（completed_steps/total_steps）
  → 展示停止/恢复按钮

收到 ActivityEvent (event=updated, meta.changed_fields=progress)
  → 更新进度条

收到 ActivityEvent (event=completed, stage=completed)
  → 更新阶段为"已完成"
  → 展示最终结果摘要
  → 展示 DQ 评分

收到 ActivityEvent (event=failed, stage=failed, meta.error_message=xxx)
  → 更新阶段为"失败"
  → 展示失败原因

收到 ActivityEvent (event=cancelled, meta.cancel_reason=user_interrupted)
  → 更新阶段为"已取消"
  → 展示取消原因

收到 ActivityEvent (event=child_created, meta.child_activity_id=xxx, meta.member_agent_key=yyy)
  → 在成员列表新增成员 Block（折叠状态）
```

#### 7.3.6 Graph 阶段（graph_stage）

```
收到 ActivityEvent (event=created, kind=graph_stage, stage=planned)
  → 新增 GraphStageBlock，显示"Graph 已规划"
  → 展示 DAG 节点列表（依赖关系）

收到 ActivityEvent (event=updated, meta.changed_fields=current_node, meta.current_node=xxx)
  → 高亮当前执行节点
  → 展示已完成/进行中/待执行节点状态

收到 ActivityEvent (event=completed, stage=completed)
  → 所有节点标记完成
  → 展示最终结果

收到 ActivityEvent (event=failed, meta.error_node=xxx, meta.error_message=xxx)
  → 高亮错误节点
  → 展示错误详情

收到 ActivityEvent (event=child_created, meta.child_activity_id=xxx)
  → 在 DAG 中新增子节点
```

---

## 八、工具类型细分设计

### 8.1 工具类型枚举

```go
// internal/biz/activity.go
type ToolCategory string

const (
    ToolCategoryShell       ToolCategory = "shell"        // Shell 命令执行
    ToolCategoryBrowser     ToolCategory = "browser"      // 浏览器操作
    ToolCategoryFileRead    ToolCategory = "file_read"    // 文件读取
    ToolCategoryFileWrite   ToolCategory = "file_write"   // 文件写入
    ToolCategoryFileSearch  ToolCategory = "file_search"  // 文件查找
    ToolCategoryWebSearch   ToolCategory = "web_search"   // 网络搜索
    ToolCategoryMCP         ToolCategory = "mcp"          // MCP 工具
    ToolCategoryCode        ToolCategory = "code"         // 代码执行
    ToolCategoryTodo        ToolCategory = "todo"         // Todo 管理
    ToolCategoryOther       ToolCategory = "other"        // 其他
)
```

### 8.2 工具类型识别规则

**识别策略**：优先查询工具注册表（准确），前缀/名称匹配作为兜底（覆盖未注册工具）。

```go
// internal/agent/tool_category.go

// ToolCategorizer 工具类型识别器（可注入，便于测试和扩展）
type ToolCategorizer interface {
    Categorize(toolName string) ToolCategory
}

// defaultToolCategorizer 默认实现：注册表查询 + 前缀匹配兜底
type defaultToolCategorizer struct {
    // toolRegistry 工具注册表（可选）。key: toolName, value: ToolCategory
    // 由 ToolService 在启动时填充，包含所有已注册工具的准确分类
    toolRegistry map[string]ToolCategory
}

func NewToolCategorizer(toolRegistry map[string]ToolCategory) ToolCategorizer {
    if toolRegistry == nil {
        toolRegistry = make(map[string]ToolCategory)
    }
    return &defaultToolCategorizer{toolRegistry: toolRegistry}
}

func (c *defaultToolCategorizer) Categorize(toolName string) ToolCategory {
    // 1. 优先查注册表（准确）
    if cat, ok := c.toolRegistry[toolName]; ok {
        return cat
    }

    // 2. 前缀/名称匹配兜底（覆盖未注册工具）
    switch {
    case strings.HasPrefix(toolName, "shell") || strings.HasPrefix(toolName, "bash"):
        return ToolCategoryShell
    case strings.HasPrefix(toolName, "browser") || strings.HasPrefix(toolName, "playwright"):
        return ToolCategoryBrowser
    case toolName == "read_file" || toolName == "cat" || toolName == "head":
        return ToolCategoryFileRead
    case toolName == "write_file" || toolName == "edit_file" || toolName == "patch":
        return ToolCategoryFileWrite
    case toolName == "find" || toolName == "grep" || toolName == "glob":
        return ToolCategoryFileSearch
    case toolName == "web_search" || toolName == "search":
        return ToolCategoryWebSearch
    case strings.HasPrefix(toolName, "mcp_"):
        return ToolCategoryMCP
    case toolName == "execute_code" || toolName == "python":
        return ToolCategoryCode
    case toolName == "todo_write" || toolName == "todo_read":
        return ToolCategoryTodo
    default:
        return ToolCategoryOther
    }
}
```

**注册表填充**：ToolService 启动时遍历所有已注册工具，根据工具元数据（如 `tool.Metadata.Category`）填充 `toolRegistry`。未提供元数据的工具走前缀匹配兜底。

**注入方式**：`ActivityProjector` 通过构造函数注入 `ToolCategorizer`，便于测试时 mock。

### 8.3 前端 UI 表现

| 工具类型 | 图标 | 布局 | 折叠时显示 | 展开时显示 |
|---------|------|------|-----------|-----------|
| shell | `$` | 终端样式 | 命令摘要 | 完整命令 + stdout/stderr |
| browser | 🌐 | 网页卡片 | URL + 操作类型 | 截图 + DOM 操作详情 |
| file_read | 📖 | 文件卡片 | 文件路径 | 文件内容片段 |
| file_write | ✏️ | 文件卡片 | 文件路径 + 变更行数 | diff 视图 |
| file_search | 🔍 | 搜索结果 | 搜索条件 + 命中数 | 结果列表 |
| web_search | 🔎 | 搜索卡片 | 查询词 + 结果数 | 结果摘要列表 |
| mcp | 🔌 | 通用卡片 | MCP 服务名 + 方法 | 参数 + 结果 |
| code | 💻 | 代码块 | 语言 + 执行状态 | 代码 + 输出 |
| todo | ✅ | 看板卡片 | 进度条 | 任务列表 |
| other | 🔧 | 通用卡片 | 工具名 | 参数 + 结果 |

### 8.4 ActionBlock 改造

```vue
<!-- web/src/components/chat/ActionBlock.vue -->
<template>
  <div class="action-block" :class="`tool-${category}`">
    <div class="action-header" @click="toggleExpand">
      <ToolIcon :category="category" :name="activity.tool_name" />
      <span class="action-title">{{ actionTitle }}</span>
      <StatusBadge :status="activity.status" />
      <DurationBadge v-if="activity.duration_ms" :ms="activity.duration_ms" />
    </div>

    <div v-if="expanded" class="action-detail">
      <!-- 按类型渲染不同详情 -->
      <component
        :is="detailComponent"
        :activity="activity"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { Component } from 'vue';
import type { Activity } from '../../features/chat/activityTypes';

const props = defineProps<{ activity: Activity }>();

const category = computed(() => props.activity.tool_category || 'other');

const detailComponent = computed(() => {
  const map: Record<string, Component> = {
    shell: ShellToolDetail,
    browser: BrowserToolDetail,
    file_read: FileReadToolDetail,
    file_write: FileWriteToolDetail,
    file_search: FileSearchToolDetail,
    web_search: WebSearchToolDetail,
    mcp: McpToolDetail,
    code: CodeToolDetail,
    todo: TodoToolDetail,
    other: GenericToolDetail,
  };
  return map[category.value] ?? GenericToolDetail;
});
</script>
```

---

## 九、Team/Graph UI 展示设计

### 9.1 Team 执行展示

#### 9.1.1 TeamStageBlock 组件

新增 `ActivityKind = "team_stage"` 用于团队阶段变更：

```
收到 ActivityEvent (event=created, kind=team_stage, stage=assembled)
  → 新增 TeamStageBlock，显示"团队已组建"
  → 展示团队成员头像列表
  → 展示任务摘要

收到 ActivityEvent (event=updated, meta.changed_fields=stage, stage=executing)
  → 更新阶段为"执行中"
  → 展示进度条（completed_steps/total_steps）
  → 展示停止/恢复按钮

收到 ActivityEvent (event=updated, meta.changed_fields=progress)
  → 更新进度条

收到 ActivityEvent (event=completed, stage=completed)
  → 更新阶段为"已完成"
  → 展示最终结果摘要
  → 展示 DQ 评分

收到 ActivityEvent (event=failed, stage=failed)
  → 更新阶段为"失败"，展示失败原因

收到 ActivityEvent (event=cancelled, meta.cancel_reason=xxx)
  → 更新阶段为"已取消"

收到 ActivityEvent (event=child_created, meta.child_activity_id=xxx)
  → 在成员列表新增成员 Block
```

#### 9.1.2 团队成员折叠展示

```
TeamStageBlock
  ├── 团队头部（阶段/进度/控制按钮）
  ├── 成员列表（可折叠）
  │   ├── Member 1（点击展开）
  │   │   └── 子 Activity 流（该成员的 thinking/action/reply）
  │   ├── Member 2
  │   └── Member 3
  └── 团队结果摘要
```

#### 9.1.3 子 session Activity 加载

点击成员展开时，懒加载该成员 session 的 Activity：

```typescript
async function expandMember(memberSessionId: string) {
  if (!activitiesBySession.value.has(memberSessionId)) {
    const activities = await listActivities(memberSessionId);
    activitiesBySession.value.set(memberSessionId, new Map(activities.map(a => [a.id, a])));
  }
  expandedMemberSessions.value.add(memberSessionId);
}
```

> **实现状态（Phase E）**：`useActivityTimeline.ensureActivitiesLoaded(sessionId)` 实现了上述缓存跳过语义——缓存命中（含空 Map）时跳过 API 调用，失败时不写缓存以便下次自动重试，WS replay 负责重连后补齐缺失事件。`bindSessionView` 改用 `ensureActivitiesLoaded` 替代 `loadActivitiesFromAPI`，成员切换瞬时响应。3 个单测覆盖（skip/load/retry-after-fail）。

### 9.2 Graph 阶段展示

#### 9.2.1 GraphStageBlock 组件

新增 `ActivityKind = "graph_stage"` 用于 Graph 阶段变更：

```
收到 ActivityEvent (event=created, kind=graph_stage, stage=planned)
  → 新增 GraphStageBlock，显示"Graph 已规划"
  → 展示 DAG 节点列表（依赖关系）

收到 ActivityEvent (event=updated, meta.changed_fields=current_node, meta.current_node=xxx)
  → 高亮当前执行节点
  → 展示已完成/进行中/待执行节点状态

收到 ActivityEvent (event=completed, stage=completed)
  → 所有节点标记完成
  → 展示最终结果

收到 ActivityEvent (event=failed, meta.error_node=xxx)
  → 高亮错误节点，展示错误详情

收到 ActivityEvent (event=child_created, meta.child_activity_id=xxx)
  → 在 DAG 中新增子节点
```

#### 9.2.2 DAG 渲染

```vue
<!-- web/src/components/chat/GraphStageBlock.vue -->
<template>
  <div class="graph-stage-block">
    <div class="graph-header">
      <GraphIcon />
      <span>{{ stageTitle }}</span>
      <ProgressIndicator :completed="completedNodes" :total="totalNodes" />
    </div>

    <div v-if="expanded" class="graph-detail">
      <DagView
        :nodes="dagNodes"
        :current-node="currentNode"
        :completed-nodes="completedNodeIds"
      />
    </div>
  </div>
</template>
```

### 9.3 编排阶段展示

Spirit 视图顶部展示编排阶段进度条：

```
[规划] → [分配] → [执行] → [完成]
  ✅       ✅       🔄       ⏳
```

通过 `notice` kind + `meta.phase` 的 Activity 事件驱动：

```typescript
const phases = ['planning', 'allocating', 'orchestrating', 'completed'];
const currentPhaseIndex = computed(() => phases.indexOf(spiritStore.orchestrationPhase));
```

---

## 十、Session 树 UI 展示设计

### 10.1 左侧栏 Session 树（支持任意深度）

```
┌─────────────────────────────────┐
│ 💬 Spirit Sessions              │
├─────────────────────────────────┤
│ ▼ 帮我重构 chat 模块            │ ← Spirit Session（根，depth=0）
│   ├─ 🔄 团队 1：后端重构        │ ← Team Session（depth=1）
│   │   ├─ 👤 agent-go            │ ← Agent Session（depth=2，Team 成员）
│   │   │   └─ 👤 sub-agent       │ ← Agent Session（depth=3，子 Agent）
│   │   ├─ 👤 agent-ent           │ ← Agent Session（depth=2）
│   │   └─ 👤 agent-test          │ ← Agent Session（depth=2）
│   ├─ 👤 agent-direct-A          │ ← Agent Session（depth=1，Spirit 直接调度，无 Team）
│   ├─ 👤 agent-direct-B          │ ← Agent Session（depth=1，Spirit 直接调度）
│   └─ ✅ 团队 2：前端重构        │ ← Team Session（depth=1）
│       ├─ � agent-vue           │ ← Agent Session（depth=2）
│       └─ 👤 agent-style         │ ← Agent Session（depth=2）
│                                 │
│ ▶ 另一个 Spirit Session         │
└─────────────────────────────────┘
```

**特点**：
- 支持任意深度递归（受 `max_session_depth` 限制）
- Team Session 和 Agent Session 都可作为 Spirit 的直接子节点
- Agent Session 可嵌套（子 Agent 递归调用）

### 10.2 SessionTreeSidebar 组件

```vue
<!-- web/src/components/chat/SessionTreeSidebar.vue -->
<template>
  <div class="session-tree-sidebar">
    <div class="spirit-sessions">
      <SessionTreeNode
        v-for="spiritNode in spiritTreeNodes"
        :key="spiritNode.session.id"
        :node="spiritNode"
        :active-session-id="activeSessionId"
        @select="onSelectSession"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { SessionTree, SessionTreeNode as SessionTreeNodeType } from '../../features/session/types';

const props = defineProps<{
  spiritTreeNodes: SessionTreeNodeType[]; // 已构建好的递归树
  activeSessionId: string;
}>();
</script>
```

### 10.3 SessionTreeNode 递归组件（支持任意深度）

```vue
<!-- web/src/components/chat/SessionTreeNode.vue -->
<template>
  <div class="session-tree-node" :class="`depth-${node.session.agent_depth}`">
    <div
      class="node-header"
      :class="{ active: isActive }"
      @click="onSelect"
    >
      <SessionTypeIcon :type="node.session.session_type" :stage="node.session.execution_stage" />
      <span class="node-title">{{ node.session.title }}</span>
      <DepthBadge v-if="node.session.agent_depth > 0" :depth="node.session.agent_depth" />
      <StageBadge v-if="node.session.execution_stage" :stage="node.session.execution_stage" />
      <ProgressMini v-if="node.session.total_steps > 0" :completed="node.session.completed_steps" :total="node.session.total_steps" />
    </div>

    <!-- 递归渲染子节点（支持任意深度） -->
    <div v-if="expanded && node.children.length > 0" class="node-children">
      <SessionTreeNode
        v-for="child in node.children"
        :key="child.session.id"
        :node="child"
        :active-session-id="activeSessionId"
        @select="onSelect"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { SessionTreeNode as SessionTreeNodeType } from '../../features/session/types';

defineProps<{
  node: SessionTreeNodeType; // 递归节点：{ session, children: SessionTreeNodeType[] }
  activeSessionId: string;
}>();
</script>
```

**前端类型定义**：

```typescript
// web/src/features/session/types.ts
interface SessionTreeNode {
  session: Session;
  children: SessionTreeNode[]; // 递归，支持任意深度
  activities?: Activity[]; // 按需加载
}
```

> **实现状态（Phase D）**：`SessionTreeNode.vue` 已实现上述设计，但 `SessionTypeIcon`/`DepthBadge`/`StageBadge`/`ProgressMini` 内联为 computed 属性 + `q-badge` 元素（而非独立组件文件），避免过度拆分。`session_type` 驱动图标选择（spirit→`auto_awesome`/team→`groups`/agent→`person`/standalone→`forum`），`execution_stage` 驱动阶段徽章颜色（planning/allocating→blue、executing→orange、completed→green、failed→red），i18n key `session.executionStage.*` 覆盖中英文。

---

## 十一、重构路线图

### Phase 1a：Activity 模型扩展（非破坏性）

**目标**：扩展 Activity 表和枚举，修正 SpiritSessionID 传递链。**不删除任何旧代码**，可与旧体系并存。

**任务**：
1. Activity 表新增 `tool_category`/`stage` 字段（保留 `role`/`child_board_id`/`tool_icon` 暂不删）
2. 新增 `ActivityKindSession`/`ActivityKindTeamStage`/`ActivityKindGraphStage` 枚举
3. 新增 `ActivityEvent` 类型（`ActivityEventType` + `Activity`）
4. `ProjectMeta` 新增 `SpiritSessionID`/`ParentSessionID`/`RootSessionID` 字段
5. 修正 `buildTeamProjectMeta` 传入正确的 spirit session ID
6. 实现 `ListBySpiritSession`/`ListByTeam`/`ListByParentSession` Repo 方法
7. 工具类型识别函数 `CategorizeTool`
8. 实现 `BuildLLMContext` 从 Activity 构建 LLM 上下文（与 Message 并存，灰度切换）

**验证**：
- 单元测试：SpiritSessionID 正确填充
- 集成测试：Spirit session 能查询到 team session 的 Activity
- LLM 上下文构建从 Activity 正确读取（与原 Message 查询结果对比）

**回滚**：直接 revert，无破坏性变更。

### Phase 1b：并行异步 + EventBus 切换

**目标**：改造 processTask 为并行异步，EventBus 改为传输 ActivityEvent。

**任务**：
1. 改造 `processTask`：persist fire-and-forget + publish 同步
2. `publishTask` 结构体改为 `{eventType, activity, persist}`
3. `EventBus.Publish` 签名改为接收 `ActivityEvent`
4. `WebSocketHub` 订阅者更新为接收 `ActivityEvent` 并转发到 "chat" channel
5. 前端 WS 处理新增 `ActivityEvent` 解析路径（与原 Envelope 路径并存）

**验证**：
- 单元测试：processTask 持久化失败不阻塞推送
- 压测：WS 推送延迟 < 50ms（不受 DB I/O 阻塞）
- 集成测试：前端能接收并渲染 ActivityEvent

**回滚**：恢复 processTask 串行逻辑 + EventBus 原 Envelope 签名。

### Phase 1c：删除旧体系

> **工作量修正（2026-06-26）**：原方案估计"删文件 + 数据迁移"即可，实际"删除 Envelope"牵涉 **6 个级联 Blocker**（WS replay 路径、4 个 side consumer、DomainEvent bridge、vestigial bus 字段、EventPipeline Bus/Buffer、Wire DI SessionBus 绑定），需多 session 推进。
> 详见 [ADR-03](./2026-06-26-review-adr-unified-bus-architecture.md) §"Phase 5 剩余路线图"。
> Blocker 依赖链：B（side consumer，独立）→ C（DomainEvent bridge，独立）→ D（vestigial 字段，依赖 B/C）→ A（WS replay，依赖前端改造）→ E（EventPipeline，依赖 A）→ F（Wire DI，依赖 E）→ 删除 envelope.go。
>
> **当前完成度**：
> - ✅ 已删：event_projector.go / activity_publish.go / activity_persist.go / event_persist_handler.go / event_store.go / wal.go / event_store schema / message schema / message_repo.go / messages 表（DROP TABLE 已执行）
> - ❌ 未删：envelope.go（含 67 个 EnvelopeType + RouteChannel）、message_usecase.go（仍承载完整业务逻辑，TECH-DEBT 标注已加）
> - ⚠️ 部分删：activity schema 的 role/tool_icon 已删（优于原方案），child_board_id 保留

**目标**：确认 Phase 1a/1b 稳定后，删除 Envelope 和 Message。

**任务**：
1. 数据迁移：`messages` 表数据迁移到 `activities` 表（见 §14.2）
2. 删除 `event_store` 表 + `EventPersistHandler` + `EventStoreUsecase`
3. 删除 `wal.go`（WAL 持久化）
4. 删除 `EventProjector`（已废弃）
5. 删除 `Envelope` 结构体 + `EnvelopeType` 常量 + `RouteChannel`
6. 删除 `messages` 表 + `message_repo.go` + `message_usecase.go`
7. 删除 Activity 表的 `role`/`child_board_id`/`tool_icon` 字段
8. 删除前端 Envelope 类型定义和路由逻辑

**验证**：
- 全量回归测试：所有功能正常
- LLM 上下文构建无 Message 表依赖
- 前端统一使用 ActivityEvent

**回滚**：从备份恢复 `messages` 和 `event_store` 表（迁移前必须备份）。

### Phase 2：Session 父子结构

**目标**：Session 表增加类型字段，构建父子层级

**任务**：
1. Session 表新增 `session_type`/`member_agent_key`/`execution_stage` 等字段
2. `SpiritTeamAssembler` 创建 team session 时设置 `session_type=team`/`parent_session_id`
3. 团队成员启动时创建 agent session（`session_type=agent`，`parent_session_id=teamSessionID`）
4. 实现 `GetSessionTree`/`ListChildSessions` API
5. 前端 `SessionStore` 加载 session 树

**验证**：
- Spirit 发送指令后，左侧栏显示 session 树
- 团队组建后，team session 出现在树中
- 成员启动后，member session 出现在 team 下

### Phase 3：前端统一渲染管线

**目标**：所有模式使用 ActivityStream 统一渲染

> **实际状态（2026-06-26 更新）**：任务 1-7 + 删除清单中的 `TaskExecutionPanel.vue`/`MemberReadOnlyPanel.vue` 已完成（Phase A + Phase B）。Phase D 补全 Session 7 字段断层（proto/service/前端类型/SessionTreeNode UI 增强），Phase E 实现 `ensureActivitiesLoaded` 缓存跳过优化（§9.1.3）。任务 8 及剩余删除项归因于 Envelope 删除的级联依赖，详见 [ADR-03](./2026-06-26-review-adr-unified-bus-architecture.md) Phase 5 路线图（Blocker A-G 全部完成）。

**任务**：
1. ✅ 实现 `ActivityStream.vue` 统一入口（Phase A）
2. ✅ `useActivityTimeline` 改造为按 session_id 隔离（Phase A）
3. ✅ 新增 `SessionStageBlock`/`TeamStageBlock`/`GraphStageBlock` 组件
4. ✅ `ActionBlock` 改造为按 `tool_category` 细分
5. ✅ 实现各工具类型详情组件（ShellToolDetail/BrowserToolDetail/...）— `ActionBlock` 按 `tool_category` 动态分发到 10 个详情组件 + `ToolCategorizer` 后端分类器（10 类别前缀匹配 + 注册表覆盖），含单测覆盖（`tool_category_test.go` 29 子测试 + `ActionBlock.spec.ts` 30 测试）
6. ✅ 实现 `SessionTreeSidebar` + `SessionTreeNode` 递归组件 + `useSessionTree` composable + `GetSessionTree` RPC 暴露（Phase B-1/B-2）。Phase D 补全 Session 7 字段断层：proto `Session` 新增 `session_type`/`member_agent_key`/`member_role`/`execution_stage`/`completed_steps`/`total_steps`/`progress_pct`（编号 53-59），`service.toProtoSession` 映射，前端 `types.ts`/`api.ts` 补全，`SessionTreeNode.vue` UI 增强（`session_type` 图标 / `L{depth}` 深度徽章 / `execution_stage` 阶段徽章带颜色映射 / `{completed}/{total}` 进度）。
7. ✅ 团队成员折叠展示 + 子 session Activity 懒加载（Phase B-3/B-4：`TeamStageBlock` 成员行可点击 → `expand-member` 事件链 → `spiritStore.selectMember` → `useChatWorkspace` panelMode watcher 通过 `sessionTree.findMemberSessionId` 解析并 `bindSessionView`）。Phase E 实现 `ensureActivitiesLoaded` 缓存跳过（§9.1.3）：`useActivityTimeline` 新增 `ensureActivitiesLoaded(sessionId)`——缓存命中时跳过 API 调用，失败时不写缓存以便下次自动重试，WS replay 负责重连后补齐缺失事件；`bindSessionView` 改用它替代 `loadActivitiesFromAPI`，成员切换瞬时响应。
8. 🟡 前端 WS 处理改为接收 `ActivityEvent`（替代 Envelope）— 后端 ADR-03 Phase 5 Blocker A-G 全部完成（`contract/envelope.go` 已删、`ProvideSessionBus` 已删、`RouteChannel` 已删、8 个 consumer 全部迁移到 `ActivityEventBus`/`MonitorBus`）；**前端部分未完成**：`web/src/realtime/envelope.ts` 仍存在，23 个前端文件、30 处引用待迁移到 `ActivityEvent` 类型

**删除**：
- ✅ `TeamPanel.vue`（死代码）
- ✅ `OrchestrationTimeline.vue`（死代码）
- ✅ `TaskExecutionPanel.vue` 的 ChatExecutionCard 路径（Phase B-5 删除整个组件文件）
- ✅ `MemberReadOnlyPanel.vue`（Phase B-5 删除）
- 🟡 `envelope.ts` 类型定义 — 后端 `contract/envelope.go` 已删（Blocker G ✅）；**前端** `web/src/realtime/envelope.ts` 仍存在，属任务 8 前端迁移范围
- 🟡 `inboundSyncRouting.ts` Envelope 路由逻辑 — 前端文件仍存在，属任务 8 前端迁移范围

**验证**：
- Spirit 视图显示完整 Activity 流（思考/计划/工具/团队/Graph）
- 点击团队展开显示成员子 session Activity
- 工具调用按类型显示不同 UI

---

## 十二、验收标准

### 12.1 功能验收

| 场景 | 验收标准 |
|------|---------|
| Spirit 发送指令 | 左侧栏显示 Spirit Session，工具/思考/回复正确渲染 |
| 组建团队 | 团队 Session 出现在 Spirit 下，TeamStageBlock 显示阶段 |
| 团队执行 | 成员 Session 出现在 Team 下，进度条更新，停止/恢复按钮可用 |
| 成员展开 | 点击成员显示其子 session 的 Activity 流 |
| 工具调用 | 按 tool_category 显示不同 UI（shell/browser/file/...） |
| Graph 规划 | GraphStageBlock 显示 DAG 节点和执行进度 |
| 切换 Session | Activity Timeline 按 session 隔离，无串扰 |
| 持久化失败 | 前端仍有实时事件，reload 时通过 API backfill |
| LLM 上下文 | 从 Activity 表正确构建，无 Message 表依赖 |

### 12.2 架构验收

> **实际状态评估（2026-06-27 更新）**：14 项验收中 **12 项达成 / 2 项部分达成 / 0 项未达成**。
> Phase B 推进了"前端渲染管线"从部分达成 → 达成（ActivityStream 为三模式唯一渲染器）。后端 Envelope 删除的级联依赖已由 [ADR-03](./2026-06-26-review-adr-unified-bus-architecture.md) Phase 5 全部完成（Blocker A-G：B→C→D→A→E→F→G）。唯一剩余项是前端 Envelope → ActivityEvent 类型迁移（任务 8 前端部分）。

| 指标 | 目标 | 实际状态 | 评估 |
|------|------|---------|------|
| 后端数据模型 | 1 套（Activity） | Activity 表是唯一真相源，messages 表已 DROP | ✅ 达成 |
| 事件类型 | 7 种业务语义事件（created/streaming/updated/completed/failed/cancelled/child_created） | ActivityEventType 已定义 7 种 | ✅ 达成 |
| ActivityKind | 10 种（task/thinking/action/reply/plan/confirm/notice/session/team_stage/graph_stage，无 error） | 10 种，无 ActivityKindError | ✅ 达成 |
| Envelope 结构体 | 删除 | 后端 `contract/envelope.go` 已删（Blocker G ✅，活类型提取到 `envelope_types.go`）；前端 `realtime/envelope.ts` 仍存在 | 🟡 后端达成 / 前端未达成 |
| Message 表 | 删除 | DROP TABLE 已执行（迁移 20260902） | ✅ 达成 |
| `role` 字段 | 不存在（用 kind 表达） | 不存在 | ✅ 达成 |
| `error` kind | 不存在（用 `failed` 事件表达） | 不存在 | ✅ 达成 |
| Channel 路由 | 删除 RouteChannel，统一 chat | `RouteChannel` 已随 `contract/envelope.go` 删除（Blocker G ✅） | ✅ 达成 |
| EventBus 传输 | ActivityEvent | `ActivityEventBus`（biz.ActivityEvent）+ `MonitorEventBus`（contract.MonitorEvent），legacy Envelope Bus / SessionBus / MonitorBus 全部删除 | ✅ 达成 |
| 死代码（后端） | 0 | `contract/envelope.go`/`buffer.go`/`reliability.go`/`bus.go` 等 10+ 文件已删（Blocker F/G ✅） | ✅ 达成 |
| 死代码（前端） | 0 | `TeamPanel`/`OrchestrationTimeline`/`TaskExecutionPanel`/`MemberReadOnlyPanel` 已删；剩余 `envelope.ts` + `inboundSyncRouting.ts` 待任务 8 前端迁移 | 🟡 部分达成 |
| 前端渲染管线 | 1 套（ActivityStream） | ✅ Phase B 后 ActivityStream 为 spirit/team/member 三模式唯一渲染器（panelMode watcher 同步 currentSessionId + bindSessionView streamKind 覆盖） | ✅ 达成 |
| Session 隔离 | 按 session_id 自然隔离，无手动 reset | 已实现 | ✅ 达成 |
| 持久化与推送 | 并行异步，互不阻塞 | processTask fire-and-forget + publish 同步 | ✅ 达成 |

**未达成项的后续推进计划**：后端 ADR-03 Phase 5 Blocker A-G 全部完成（`contract/envelope.go` 已删、`ProvideSessionBus` 已删、8 个 consumer 全部迁移到 `ActivityEventBus`/`MonitorBus`）。**唯一剩余项**是前端 Envelope → ActivityEvent 迁移（任务 8 前端部分）：23 个前端文件、30 处引用 `realtime/envelope`，需将前端 WS 处理从 `Envelope` 类型改为 `ActivityEvent` 类型，完成后可删除 `web/src/realtime/envelope.ts` 和 `web/src/features/chat/inboundSyncRouting.ts`。

### 12.3 性能验收

| 指标 | 目标 |
|------|------|
| WS 推送延迟 | < 50ms（不受 DB I/O 阻塞） |
| Activity 持久化 | 异步，不阻塞用户感知 |
| 前端渲染 | 60fps（虚拟滚动 + 按需展开） |

---

## 十三、风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| Message 表一次性删除影响 LLM 上下文 | 高 | Phase 1a 先实现 `BuildLLMContext` 与 Message 并存，灰度对比验证；Phase 1c 删除前必须备份 |
| Envelope 删除影响面大 | 中 | `ActivityEvent` 接口设计兼容，EventBus 接口不变（仅签名参数类型变） |
| 并行持久化导致数据不一致 | 中 | backfill 机制 + retry + 最终一致性；前端 reload 时 API 是真相源 |
| Session 树深度过大影响性能 | 低 | 限制最大深度（spirit=0, team=1, member=2） |
| 工具类型识别不准 | 低 | 兜底 `other` 类型，可配置规则 |
| 前端统一渲染管线改造量大 | 中 | Phase 3 在 Phase 1/2 之后，后端已稳定 |
| Phase 1c 数据迁移失败 | 高 | 迁移前全量备份；迁移脚本幂等；失败可从备份恢复 |

---

## 十四、回滚方案

### 14.1 回滚原则

- **Phase 1a/1b 可独立回滚**：非破坏性变更，直接 revert 即可
- **Phase 1c 回滚需从备份恢复**：删除表操作不可逆，必须提前备份
- **Phase 2/3 可独立回滚**：前端和 Session 结构变更不影响后端 Activity 模型

### 14.2 各 Phase 回滚策略

| Phase | 回滚方式 | 数据影响 | 预计耗时 |
|-------|---------|---------|---------|
| 1a | git revert | 无（新增字段不影响旧代码） | < 1h |
| 1b | git revert + 恢复 EventBus 签名 | 无（Activity 表数据保留） | < 2h |
| 1c | 从备份恢复 `messages`/`event_store` 表 | 丢失 1c 后的新 Activity 数据 | < 4h |
| 2 | git revert + 恢复 Session 表结构 | 无（新增字段不影响旧代码） | < 1h |
| 3 | git revert 前端代码 | 无 | < 2h |

### 14.3 备份要求

**Phase 1c 执行前必须备份**：
```bash
# 备份 PostgreSQL 数据库（使用 pg_dump）
PGDATABASE=aranea pg_dump -h $PG_HOST -U $PG_USER -F c -f /data/aranea.backup.$(date +%Y%m%d).dump

# 验证备份完整性（恢复到临时库检查）
PGDATABASE=aranea_verify pg_restore -h $PG_HOST -U $PG_USER -d aranea_verify /data/aranea.backup.$(date +%Y%m%d).dump
psql -h $PG_HOST -U $PG_USER -d aranea_verify -c "SELECT count(*) FROM messages;"
psql -h $PG_HOST -U $PG_USER -d aranea_verify -c "SELECT count(*) FROM event_store;"
```

---

## 十五、监控指标

### 15.1 重构后必须监控的指标

| 指标 | 目标 | 监控方式 | 告警阈值 |
|------|------|---------|---------|
| WS 推送延迟 P99 | < 50ms | processTask 计时 | > 200ms |
| Activity 持久化失败率 | < 0.1% | persist goroutine 错误计数 | > 1% |
| Activity 持久化延迟 P99 | < 100ms | UpsertActivity 计时 | > 500ms |
| 前端 backfill 触发率 | < 5% | 前端 reload 时 Activity 数 vs WS 接收数 | > 20% |
| Session 树查询延迟 P99 | < 50ms | GetSessionTree 计时 | > 200ms |
| LLM 上下文构建延迟 P99 | < 30ms | BuildLLMContext 计时 | > 100ms |

### 15.2 关键日志字段

```go
// processTask 推送计时
s.lg.Info("activity published",
    loggateway.StepID("agent.activity_sequencer.publish"),
    loggateway.Str("activity_id", activityID),
    loggateway.Str("kind", string(task.activity.Kind)),
    loggateway.Int64("publish_ms", publishMs))

// persist 失败
s.lg.Warn("activity persist failed",
    loggateway.StepID("agent.activity_sequencer.persist"),
    loggateway.Str("activity_id", activityID),
    loggateway.Str("kind", string(task.activity.Kind)),
    loggateway.Err(err))
```

### 15.3 验证指标的方式

- **WS 推送延迟**：在 processTask 的 publish 前后计时
- **持久化失败率**：统计 persist goroutine 的错误日志
- **backfill 触发率**：前端记录 WS 接收的 Activity ID 集合，reload 后对比 API 返回的 Activity ID 集合，差集比例即为 backfill 率

---

## 十六、附录

### 16.1 关键文件清单

**后端新增**：
- `internal/agent/tool_category.go` — 工具类型识别
- `internal/biz/session_tree.go` — Session 树构建
- `internal/biz/llm_context_builder.go` — LLM 上下文构建（替代 Message）
- `internal/event/activity_event.go` — ActivityEvent 类型定义

**后端修改**：
- `internal/agent/activity_projector.go` — ProjectMeta 新增字段 + SpiritSessionID 修正
- `internal/agent/activity_event_sequencer.go` — processTask 并行化
- `internal/biz/activity.go` — 新增 ActivityKind + 查询方法 + ToolCategory 枚举
- `internal/data/activity_repo.go` — 实现新查询方法
- `internal/data/ent/schema/activity.go` — 新增字段，删除冗余字段
- `internal/data/ent/schema/session.go` — 新增 session_type 等字段
- `internal/event/bus.go` — EventBus 传输 ActivityEvent

**后端删除**：
- `internal/agent/event_projector.go`
- `internal/agent/activity_publish.go`
- `internal/agent/activity_persist.go`
- `internal/biz/event_persist_handler.go`
- `internal/biz/event_store.go`
- `internal/event/wal.go`
- `internal/event/contract/envelope.go`（替换为 activity_event.go）
- `internal/data/ent/schema/event_store.go`
- `internal/data/ent/schema/message.go`
- `internal/biz/session/message_usecase.go`
- `internal/data/message_repo.go`

**前端新增**：
- `web/src/components/chat/ActivityStream.vue` — 统一渲染入口
- `web/src/components/chat/SessionStageBlock.vue` — Session 生命周期 Block
- `web/src/components/chat/TeamStageBlock.vue` — 团队阶段 Block
- `web/src/components/chat/GraphStageBlock.vue` — Graph 阶段 Block
- `web/src/components/chat/SessionTreeSidebar.vue` — Session 树侧栏
- `web/src/components/chat/SessionTreeNode.vue` — 递归树节点
- `web/src/components/chat/tools/ShellToolDetail.vue` — Shell 工具详情
- `web/src/components/chat/tools/BrowserToolDetail.vue` — 浏览器工具详情
- `web/src/components/chat/tools/FileReadToolDetail.vue` — 文件读取详情
- `web/src/components/chat/tools/FileWriteToolDetail.vue` — 文件写入详情
- `web/src/components/chat/tools/FileSearchToolDetail.vue` — 文件搜索详情
- `web/src/components/chat/tools/WebSearchToolDetail.vue` — 网络搜索详情
- `web/src/components/chat/tools/GenericToolDetail.vue` — 通用工具详情

**前端删除**：
- `web/src/components/chat/TeamPanel.vue`
- `web/src/components/chat/OrchestrationTimeline.vue`
- `web/src/components/spirit/TaskExecutionPanel.vue`（重构后）
- `web/src/components/spirit/MemberReadOnlyPanel.vue`（重构后）
- `web/src/realtime/envelope.ts`（替换为 activityEvent.ts）
- `web/src/features/chat/inboundSyncRouting.ts`

### 16.2 数据迁移

**数据库**：PostgreSQL（SQLite 已弃用）。以下 SQL 兼容 PostgreSQL 语法。

**执行方式**：通过 DDL Migration Registry 注册（见 `internal/data/ddl_migration_registry.go`），纳入 L2 迁移体系，幂等执行。

```sql
-- 1. Activity 表新增字段（IF NOT EXISTS 保证幂等）
ALTER TABLE activities ADD COLUMN IF NOT EXISTS tool_category TEXT DEFAULT '';
ALTER TABLE activities ADD COLUMN IF NOT EXISTS stage TEXT DEFAULT '';

-- 2. Activity 表删除冗余字段（兼容期后，PostgreSQL 支持 DROP COLUMN）
-- ALTER TABLE activities DROP COLUMN IF EXISTS child_board_id;
-- ALTER TABLE activities DROP COLUMN IF EXISTS tool_icon;

-- 3. Session 表新增字段
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS session_type TEXT DEFAULT 'standalone';
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS member_agent_key TEXT DEFAULT '';
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS member_role TEXT DEFAULT '';
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS execution_stage TEXT DEFAULT '';
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS completed_steps INTEGER DEFAULT 0;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS total_steps INTEGER DEFAULT 0;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS progress_pct DOUBLE PRECISION DEFAULT 0.0;

-- 4. 回填 session_type（含 member → agent 迁移）
UPDATE sessions SET session_type = 'spirit'
WHERE parent_session_id = ''
  AND agent_id IN (SELECT id FROM agents WHERE agent_key = '__spirit__');
UPDATE sessions SET session_type = 'team'
WHERE parent_session_id != '' AND team_id != ''
  AND member_agent_key = '';
-- member session 统一迁移为 agent session（替代原 member 类型）
UPDATE sessions SET session_type = 'agent'
WHERE parent_session_id != '' AND member_agent_key != '';

-- 5. 回填 activity.tool_category（根据 tool_name）
UPDATE activities SET tool_category = 'shell' WHERE tool_name LIKE 'shell%' OR tool_name LIKE 'bash%';
UPDATE activities SET tool_category = 'file_read' WHERE tool_name IN ('read_file', 'cat', 'head');
UPDATE activities SET tool_category = 'file_write' WHERE tool_name IN ('write_file', 'edit_file', 'patch');
UPDATE activities SET tool_category = 'file_search' WHERE tool_name IN ('find', 'grep', 'glob');
UPDATE activities SET tool_category = 'web_search' WHERE tool_name IN ('web_search', 'search');
UPDATE activities SET tool_category = 'todo' WHERE tool_name IN ('todo_write', 'todo_read');

-- 6. 修正 spirit_session_id（从 session 表关联）
UPDATE activities
SET spirit_session_id = (
    SELECT s.root_session_id FROM sessions s WHERE s.id = activities.session_id
)
WHERE spirit_session_id = '' OR spirit_session_id = session_id;

-- 7. 迁移 Message 数据到 Activity（一次性）
INSERT INTO activities (id, kind, status, session_id, turn_id, content, timestamp, agent_key)
SELECT
    'migrated_' || m.id,
    CASE m.role
        WHEN 'user' THEN 'task'
        WHEN 'assistant' THEN 'reply'
        WHEN 'tool' THEN 'action'
        WHEN 'system' THEN 'notice'
        ELSE 'notice'
    END,
    'completed',
    m.session_id,
    m.turn_id,
    m.content,
    m.created_at,
    m.agent_id
FROM messages m
WHERE NOT EXISTS (
    SELECT 1 FROM activities a
    WHERE a.session_id = m.session_id
      AND a.turn_id = m.turn_id
      AND a.content = m.content
);

-- 8. 删除 event_store 表
DROP TABLE IF EXISTS event_store;

-- 9. 删除 messages 表（Phase 1 末尾，确认无依赖后）
-- DROP TABLE IF EXISTS messages;
```

---

## 十七、总结

本次重构的核心目标是**彻底统一前后端逻辑，消除所有冗余，让 Chat 模块变得清晰、简洁、正确响应业务**。

**彻底合并的核心收益**：
1. **唯一模型**：只有 Activity 一个数据模型，Envelope 和 ChatMessage 概念彻底消失
2. **业务语义事件**：7 种业务语义事件（created/streaming/updated/completed/failed/cancelled/child_created），每种有明确业务含义，所有业务语义通过 `kind` + `event` + `status` + `meta` 表达
3. **无技术债务**：不保留兼容期，不新增 `role` 字段，不保留 Envelope 作为传输协议，不保留 `error` kind（用 `failed` 事件表达）
4. **父子层级**：Session 树清晰表达 Spirit → Team → Member 的执行关系
5. **并行异步**：持久化与推送互不阻塞，降低用户感知延迟
6. **动态渲染**：前端根据 Activity kind 动态选择组件，所有模式统一
7. **工具细分**：按类型显示不同 UI，提升用户认知效率
8. **死代码清理**：删除 TeamPanel/OrchestrationTimeline/EventProjector/Envelope/Message 等所有废弃代码

**关键约束**：
- 不引入新的抽象层，不添加未请求的功能
- 保持 YAGNI 原则，工具类型细分仅覆盖实际使用的工具
- 重构分 3 个 Phase 渐进推进，每个 Phase 可独立验证

**与妥协方案的根本区别**：
- 妥协方案保留 Envelope 作为传输协议 → 本方案彻底删除 Envelope
- 妥协方案保留 Message 兼容期 → 本方案一次性迁移
- 妥协方案新增 role 字段 → 本方案用 kind 表达
- 妥协方案保留 `error` kind → 本方案用 `failed` 事件表达
- 妥协方案保留 15 种 EnvelopeType → 本方案只有 7 种业务语义事件
- 妥协方案保留 RouteChannel → 本方案统一 chat channel
