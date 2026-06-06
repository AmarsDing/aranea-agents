# Chat 对话模块 — 需求规格

> 对话框是用户与 Agent/Team 交互的核心入口，负责 HTTP/WS 发起对话、WebSocket + EventBus 实时事件、上下文管理、用量记录、停止生成、人工等待回复与待执行队列。
>
> **2026-05-17 现状对齐**：当前代码已移除独立 SSE `/v1/chat/messages/stream` 路由，实时事件以 `/v1/ws` WebSocket Envelope 为主通道；HTTP `SendChatMessage` 保留为非流式/后台入口，并被 WS 上行、Channel、Cron 等复用。

---

## 一、后端架构

### 1.1 代码分层（遵循 AI-DEVELOPMENT-SPECIFICATION.md）

```
api/kratos/chat/v1/chat.proto        ← 对话 API 契约（发送、选项、停止、待执行、RunStatus、AwaitUserReply）
        ↓
internal/server/ws.go                 ← WebSocket 实时通道（订阅、回放、取消、user_message 上行）
internal/service/chat.go              ← ChatService 桥接 + RunStatus/AwaitUserReply；入队/排队委托 chatUC
internal/biz/chat_usecase.go         ← Follow-up Queue 编排（EnqueueUserMessage / Pending CRUD）
internal/service/chat_pending.go       ← PendingMessageQueue 实现（待下沉 runtime）
internal/service/chat_native.go       ← 原生对话入口（HTTP unary + WS 上行复用）+ hydratedAgent
internal/service/trpc_turn.go         ← trpc-agent-go 单 Agent turn 执行 + EventBus 投影
internal/service/chat_usage_ingress.go ← 用量记录
internal/service/session_compress.go  ← L0 上下文压缩
internal/service/session_title_llm.go ← LLM 标题生成
        ↓
internal/agent/trpc_build.go         ← Agent 构建（BuildTRPCLLMAgent）
internal/agent/trpc_runtime.go       ← Runner 构建（NewTRPCRunner + RunTRPCUserTurn）
internal/agent/event_projector.go     ← trpc-agent-go event → EventBus Envelope
internal/agent/options.go            ← options_json 构建
internal/agent/intent/               ← 意图识别与消息增强
        ↓
internal/team/runner.go              ← Team Runner（Coordinator / Swarm）
internal/team/trpc_build.go          ← Team 构建（BuildTRPCTeam）
        ↓
internal/runtimedeps/deps.go         ← 运行时依赖注入 DTO（TurnDeps / Runtime）
internal/biz/session_usecase.go      ← Session Usecase（含标题自动生成）
internal/biz/session_title.go        ← SessionTitleGenerator 接口
internal/biz/agent.go                ← Agent Usecase
```

### 1.2 请求流转

```
前端 GET /v1/ws?session_id=... 建立 WebSocket
  → 上行 user_message / enqueue_message / cancel / ping / subscribe / unsubscribe / enable_log
    → WSServer.handleUserMessage()
      → ChatService.SendChatMessage()
        → runNativeAgentTurn()
          → session.owner_type == "team"?
            → team.Runner.RunTurn() → BuildTRPCTeam → trpc Runner → EventBus Envelope
          → session.owner_type == "agent"?
            → runSingleAgentViaTRPC()
              → BuildTRPCLLMAgentCached() → NewTRPCRunner() → RunTRPCUserTurn()
              → EventProjector → EventBus → WS 下行 Envelope

后台/非流式入口：
POST /v1/chat/messages
  → ChatService.SendChatMessage()
  → 同一 runNativeAgentTurn() 主链路

Channel 入口（飞书/Lark webhook 等）：
POST /v1/channel/{channel_type}/webhook
  → ChannelIngress.HandleWebhook()
    → ChatService.RunNativeTurnUnary()
      → 同一 runNativeAgentTurn() 主链路

Cron 入口：
Cron Scheduler
  → ChatService.RunCronTurn()
    → 同一 runNativeAgentTurn() 主链路
```

### 1.3 API 端点

| 方法 | 路径 | 协议 | 说明 |
|------|------|------|------|
| POST | `/v1/chat/messages` | unary | 非流式对话 |
| GET | `/v1/ws?session_id=...` | WebSocket | 实时事件主通道；支持订阅、回放、取消、user_message 上行 |
| GET | `/v1/chat/options` | unary | 获取对话选项 |
| POST | `/v1/chat/stop` | unary | 停止生成 |
| GET | `/v1/chat/pending` | unary | 获取待执行消息列表 |
| POST | `/v1/chat/pending/cancel` | unary | 取消待执行消息 |
| POST | `/v1/chat/pending/update` | unary | 编辑待执行消息 |
| GET | `/v1/chat/run-status` | unary | 查询当前/最近一次 Run 状态 |
| POST | `/v1/chat/await-reply` | unary | 提交人工等待回复 |

### 1.4 WebSocket 协议

#### 1.4.1 上行消息类型

| 上行 type | 说明 |
|-----------|------|
| `user_message` | 发送用户消息，触发 ChatService.SendChatMessage |
| `enqueue_message` | 发送用户消息，若当前有 run 则入队 pendingQueue |
| `cancel` | 取消当前 run（等同于 HTTP StopGeneration） |
| `ping` | 心跳探测，服务端回复 `pong` |
| `subscribe` | 订阅指定 session/team 的 EventBus 事件 |
| `unsubscribe` | 取消订阅 |
| `enable_log` | 开启运行日志推送（monitor channel） |

#### 1.4.2 下行消息类型

| 下行 type | Channel | 说明 |
|-----------|---------|------|
| `connected` | system | WS 连接建立成功，携带 session_id 和连接元信息 |
| `pong` | system | 心跳回复 |
| `replay_start` | system | EventBuffer 回放开始标记 |
| `replay_end` | system | EventBuffer 回放结束标记 |
| `server_shutdown` | system | 服务端即将关闭通知 |
| `text_delta` / `text_done` | chat/team | 模型增量文本与最终文本 |
| `tool_call` / `tool_result` | chat/team | 工具调用与工具结果 |
| `state_delta` | chat/team | Runner State 增量 |
| `runner_completion` | chat/team | 一轮 Runner 完成，携带 usage |
| `error` | chat/system | 错误信息；待执行失败时携带 `pending_id`（见 §1.9） |
| `intent_pass` | chat/team | 意图识别结果 |
| `transfer` | team | Team/Swarm 转交 |
| `team_run_started` / `team_run_finished` / `team_run_failed` | team | Team run 生命周期 |
| `member_message_start` / `member_delta` / `member_message_done` | team | 成员级实时消息；`EventProjector` 在 `MemberAgentKeys` 下投影，前端 `useChatWorkspace` 消费 |
| `run_status` | chat/team | 运行生命周期；`metadata.hint=message_queued` 表示 Follow-up 入队成功 |
| `log` | monitor | 运行日志，需客户端通过 `enable_log` 上行开启订阅 |

#### 1.4.3 下行 Envelope 结构

```json
{
  "direction": "server_to_client",
  "channel": "chat|team|monitor|graph|system",
  "envelope": {
    "id": "...",
    "type": "text_delta",
    "session_id": "...",
    "content": {"text": "...", "reasoning": "...", "is_partial": true},
    "tool_call": {"id": "...", "name": "...", "arguments_json": "...", "status": "...", "is_long_running": false},
    "state_delta": {"operation": "...", "path": "...", "value_json": "..."},
    "transfer": {"from_agent": "...", "to_agent": "..."},
    "error": {"type": "...", "message": "...", "pending_id": "..."},
    "usage": {"prompt_tokens": 0, "completion_tokens": 0},
    "tag": "...",
    "filter_key": "...",
    "branch": "...",
    "version": 0,
    "extensions": {"skip_summarization": false},
    "actions": {"skip_summarization": false},
    "trace": {"agent_name": "...", "invocation_id": "...", "step_count": 0, "duration_ms": 0}
  }
}
```

> **字段说明**：`content`/`tool_call`/`state_delta`/`transfer`/`error`/`usage` 为载荷字段，按 `type` 选择性填充；`tag`/`filter_key`/`branch`/`version`/`extensions`/`actions`/`trace` 为元数据字段，所有类型均可能携带。

### 1.5 对话选项

通过 `GET /v1/chat/options` 获取，当前支持：

| 类型 | Key | 标签 | 说明 |
|------|-----|------|------|
| dialog_mode | default | 标准对话 | 默认模式 |
| dialog_mode | plan | 深思考 | 启用 BuiltinPlanner |
| dialog_mode | code | 仅代码 | 代码模式 |
| provider | — | — | 动态从 LLM Catalog 获取可用 Provider 列表 |
| model | — | — | 动态从 LLM Catalog 获取可用 Model 列表 |

### 1.6 上下文管理

- **上下文用量追踪**：每次 turn 后通过 `UpdateSessionContextFromLLMUsage` 更新 `context_used_tokens` / `context_used_ratio`（`prompt_tokens / context_window`）
- **Context Window 解析**：`llmcontext.ResolveWindow`（provider model `context_window_k` → session default → agent → 128000）；ChatOrchestrator 在 turn 结束与 `runner_completion` 投影时使用
- **L0 压缩**：当 `context_used_ratio` 超过阈值（默认 0.6）时，`SessionCompressor.AfterNativeTurn()` 异步触发摘要压缩；完成后 WS 推送带新 ratio 的 `system.session.compress` 通知
- **实时 UI**：`context_usage`（ReAct 子步）与 `runner_completion.usage`（含 `context_prompt_tokens`、`max_tokens`、`turn_total_tokens`）及压缩 notice 经 `sessionContextPatch` 乐观更新 Composer；Composer 副行合并展示 ctx/in/out/Σ/费用（与 Usage 大盘口径一致）
- **记忆服务**：通过 `runtimedeps.Runtime.SessionMemory` 注入 SQLite 适配器，由 trpc Runner 自动管理 L0-L4

### 1.7 用量记录

- 每次对话后通过 `recordTurnUsage()`（`trpc_turn` defer）写入 `model_token_usage_events`；`recordChatIngressUsage` 仅 `CHAT_RECORD_USAGE_INGRESS=1` 时备用
- 支持流式/非流式两种记录路径
- 可通过 `CHAT_RECORD_USAGE_INGRESS=0` 禁用（用于双写过渡期）
- 记录字段包含：session_id、agent_key、team_id、model_api_id、input/output_tokens、latency_ms、tokens_per_second、stream_enabled、usage_kind、provider_code、prompt_mode

### 1.7.1 SessionTurn 持久化

- 单 Agent turn 完成后，`recordSessionTurn()` 写入 `session_turns` 表，记录 turn 索引、角色、模型、token 用量和耗时
- Team turn 完成后，`recordTeamSessionTurn()` 写入 `session_turns` 表，与单 Agent 行为一致

### 1.7.2 Team Run 持久化

- Team turn 执行时，`CreateTeamRun()` 写入 `team_runs` 表，记录 team_id、session_id、状态和起止时间
- Team turn 中每个成员 Agent 执行步骤通过 `CreateTeamRunStep()` 写入 `team_run_steps` 表
- Team 定义可配置 `intent_anchor_agent_id`（指定意图识别使用的成员 Agent）和 `TurnDeadlineDuration`（turn 超时时间）

### 1.7.3 可观测性

- Chat turn 耗时通过 `arametrics.ChatTurnDuration` Prometheus 指标记录
- 意图识别超时为 45 秒

### 1.8 停止生成与运行中追加消息

- **`internal/runtime.RunRegistry`** 跟踪每 session 的 active run（`trpcrunner.Runner` 或 Team `context.CancelFunc` 或占位符）、pending 处理 cancel、run status
- `runSingleAgentViaTRPC`：`RunRegistry.StoreRunner`；defer `Finish` + `runner.Close` + `processPendingQueue`
- `RunTRPCUserTurn` 使用 `trpcagent.WithRequestID(sessionID)`，与 `ManagedRunner.Cancel(sessionID)` 对齐
- **停止**：HTTP `StopGeneration` 或 WS `cancel` → `RunRegistry.Cancel`（含 pending 后台 turn 的 cancel）
- **运行中追加（Follow-up Queue）**：见 **§1.9**；HTTP `POST /v1/chat/enqueue`、`EnqueueUserMessage` RPC，或 WS `user_message` / `enqueue_message`；`SendChatMessage` 在 active run 时自动入队而非拒绝
- **Team cancel**：`RunRegistry.StoreCancelable` 登记 Team turn，与单 Agent 停止行为一致
- **连接管理**：`WSServer` 负责心跳、连接数限制、断线回放和 EventBus 订阅

### 1.8.1 EventBuffer 回放

- WS 连接断线重连时，客户端携带 `last_event_id` 请求回放
- `EventBuffer` 为 ring buffer，容量 200 条/Session，基于事件 ID 匹配回放起始位置
- 回放期间依次发送 `replay_start` → 事件序列 → `replay_end` 控制消息
- **清理策略**：TTL 30min 自动过期 + 5min eviction ticker + `Close()` 优雅停止；`lastAcc` 追踪最后访问时间

### 1.8.2 EventBus 订阅与背压

- `EventBus.Subscribe()` 支持 `SubscribeOptions`：SessionID / TeamID / Channel / FilterKey / EventTypes / LevelFilter
- `FilterKey` 采用前缀匹配规则（`MatchFilterKey`）
- 当订阅者消费速度落后时，EventBus 丢弃非关键事件；关键事件类型（不丢弃）包括：`tool_result`、`error`、`runner_completion`、`graph_node_end`、`team_run_finished`、`team_run_failed`
- WS 全局监控连接（`globalMode`，sessionId=`*`）可订阅所有 Session 的事件流

### 1.8.3 Agent Settings Variables 注入

- Agent 配置中的 `variables_json` 字段存储自定义变量
- `runSingleAgentViaTRPC` 执行时通过 `ParseVariablesJSON` → `MergeRuntimeState` 将变量注入 Runner State
- 变量可在 System Prompt 中通过占位符引用

### 1.9 对话阶段连续发送（Follow-up Queue / 待发送队列）

> **产品定位**：对标 Cursor 等 IDE 聊天窗口——Agent 正在生成回复或执行工具时，用户仍可连续按 Enter 发送多条后续消息；消息不会丢失，也不会打断当前 turn，而是进入**对话队列**按策略处理。
>
> **编排归属**：Gateway 运行编排层（`ChatUsecase` + `RunRegistry` + `PendingMessageQueue`）；Chat 模块负责传输入口与 UI 展示。设计细节见 [35 gateway.design.md §3.3](./35%20gateway.design.md#33-follow-up-queue对话阶段连续发送)。

#### 1.9.1 用户故事

- 作为用户，当 Agent 正在思考/流式输出/调用工具时，我可以继续输入并发送多条消息，而不必等待当前回复结束。
- 作为用户，我可以在消息区底部看到**待发送队列**中的消息，并取消或编辑尚未执行的内容。
- 作为用户，当队列已满或运行已结束时，我应收到明确错误提示（`CHAT_QUEUE_FULL` / `CHAT_RUN_ENDED`），而不是静默失败。

#### 1.9.2 双路径入队策略

| 路径 | 条件 | 行为 | 队列可见性 |
|------|------|------|------------|
| **Steerable 直注** | Runner 实现 `SteerableRunner` 且 `EnqueueUserMessage` 成功 | 消息注入当前活跃 turn，由框架在合适时机消费 | 不进入 Pending 列表（仅本地占位气泡） |
| **Pending FIFO 降级** | Steerable 不支持或返回 `ErrQueuedUserMessageUnsupported` | 写入 `PendingMessageQueue`，当前 turn 结束后 `processPendingQueue` 取下一条发起新 turn | 出现在「待执行队列」UI + `GetPendingMessages` |

编排入口：`ChatUsecase.EnqueueUserMessage`（`internal/biz/chat_usecase.go`），由 `chat_native` / `EnqueueUserMessage` RPC / WS 上行触发。

#### 1.9.3 WS 通知（`message_queued`）

入队成功后，`ChatEventPublisher.PublishMessageQueued` 发布 `run_status` Envelope：

```json
{
  "type": "run_status",
  "metadata": { "status": "queued", "hint": "message_queued" }
}
```

前端应监听 `hint === "message_queued"` 刷新待发送列表（替代纯轮询）。  
`ChatService.publishMessageQueued` 已废弃，逻辑收敛至 `ChatUsecase` + `publishMessageQueuedToBus`。

#### 1.9.4 队列 CRUD 与容量

| API | 说明 |
|-----|------|
| `GET /v1/chat/pending` | 列出当前 session 的 Pending FIFO 条目 |
| `POST /v1/chat/pending/cancel` | 取消指定 `pending_id` |
| `POST /v1/chat/pending/update` | 编辑指定 `pending_id` 内容 |
| `POST /v1/chat/enqueue` | 显式入队（WS `enqueue_message` 等价） |

- **容量**：每 session 最多 **32** 条 Pending（`maxPendingPerSession`）；超出返回 `CHAT_QUEUE_FULL`。
- **持久化**：可选磁盘快照（`PendingMessageQueueWithDir`），重启恢复 2h 内条目。
- **执行**：turn 完成/失败后 defer 调用 `processPendingQueue`；单 Agent 与 Team 路径一致；失败时 `error.pending_id` 关联条目。
- **超时**：待执行 turn 处理 600s 超时，可被 `StopGeneration` 取消。

#### 1.9.5 与「停止生成」的关系

- **StopGeneration / WS cancel**：取消当前 run；**不自动清空** Pending FIFO（已排队消息保留，下一空闲窗口继续处理，除非用户手动取消）。
- 用户可在 Agent 运行时连续入队；也可随时停止当前生成，队列中未执行消息仍可见。

#### 1.9.6 前端交互规格（Cursor 对齐）

| 行为 | 期望 | 当前实现 |
|------|------|----------|
| 运行中可连续发送 | Enter 不阻塞，输入框在 `running` 时仍可用 | 🟡 `sending` 标志在首条发送后置 true，可能阻塞连续发送 |
| 待发送列表 | 消息区底部展示 Pending 条目，支持编辑/取消 | ✅ `ChatMessagePanel` + 3s 轮询 |
| 入队即时反馈 | WS `message_queued` 触发列表刷新 | 🟡 未监听 hint，依赖轮询 |
| Steerable 直注 | 消息立即出现在对话流（本地占位） | ✅ `pending-user-*` 占位气泡 |
| 队列满/运行结束 | Toast + 错误码 | ✅ `CHAT_QUEUE_FULL` / `CHAT_RUN_ENDED` |

> 前端优化项见 [1-chat-development.md](./1-chat-development.md) §3 与 [35-gateway-development.md](./35-gateway-development.md) Phase 1.5。

### 1.10 RunStatus 与 AwaitUserReply

- `GetRunStatus` 返回 `idle | pending | running | awaiting_user | completed | failed | cancelled`
- `runStatuses sync.Map` 记录当前/最近一次 run 状态、run_id、错误信息和更新时间
- `makeAwaitReplyFunc` 注入 service await-reply tool，工具阻塞时将状态置为 `awaiting_user`
- `AwaitUserReply` 向 `awaitChans` 投递人工回复，恢复正在等待的 run
- 单 Agent 和 Team 路径均通过 `makeAwaitReplyFunc` 注入 AwaitHook；Team Runner 通过 `SetAwaitHookProvider` 注入，`runCtx` 注入 `serviceawaitreply.WithReplyFunc`
- 前端：`useEnvelopeStream` / `useChatStream` / `useTeamStream` 消费 WS；`useChatWorkspace` 轮询 `GetRunStatus` 并在 `awaiting_user` 时展示提交回复横幅（`ChatMessagePanel` + `AwaitUserReply` RPC）
- 当前状态与等待通道为进程内内存结构；服务重启后不可恢复，后续应持久化或接入 EventBuffer 恢复

### 1.11 Session 标题自动生成

- 首次对话时，`maybeAutoTitleFromUserMessage` 触发标题生成
- 先用截取方式快速设置标题（用户消息前 22 字符，即时反馈）
- 异步调用 `LLMSessionTitleGenerator` 生成高质量标题（15s 超时，失败静默）
- `LLMSessionTitleGenerator` 优先选择轻量模型（mini/flash/lite/small）

---

## 二、前端布局和详情

### 2.1 左侧 Agent/Team 列表（ChatEntitySidebar）

1. 宽度 120px，高度 100%；Agent 和 Team 分组显示
2. Agent 按分类树（PlatformResourceTreeNode）分组显示
3. Team 按分组显示
4. 默认 Agent 显示在 Agent 组最上方，不可拖拽调序
5. 默认 Team 显示在 Team 组最上方，不可拖拽调序
6. 顶部搜索框：按名称搜索 Agent 和 Team，带输入提示和过滤
7. 条目：左侧显示工作状态指示灯（bolt/task_alt）和名称，右侧设置和删除按钮
8. 设置按钮：弹框显示 Agent/Team 设置界面（ChatSettingsDialog）
9. 删除按钮：弹出确认对话框（ChatDeleteDialog），工作中不可删除，删除需填写名称
10. 选中时背景高亮，右侧显示 Session 历史，中间显示最近 Session 内容
11. 首次进入默认选中默认 Agent
12. 列表右侧中间折叠按钮，带动画

### 2.2 右侧 Session 历史栏（ChatSessionSidebar）

1. 宽度 120px，高度 100%
2. 按时间线分组显示（置顶、今天、昨天、7天内、30天内、更早）
3. 每条：左侧圆环显示上下文额度比，右侧显示 Session 名称和时间
4. 支持置顶、收藏、重命名、历史追踪操作
5. 底部：左侧新建 Session，右侧一键删除历史 Session
6. 列表左侧中间折叠按钮，带动画

### 2.3 中间对话区域（ChatMessagePanel）

1. 顶部：Session 标题 + 上下文使用比例
2. 对话内容区：使用 `q-chat-message` 显示头像、时间、内容
3. 消息气泡区分用户/助手/工具事件/Team 成员
4. 助手消息中的 `reasoning` 内容需展示（当前前端已消费 `content.reasoning` 字段），展示方式待定：折叠区域 / 内联 / 独立面板
5. 工具事件消息可折叠（details/summary）
6. 流式消息显示打字动画
7. 底部输入区域：初始高度 100px，autogrow，最高 400px
8. 输入框底部工具条：
   - 左侧：对话模式 `QSelect`、模型提供商 `QSelect`、上下文使用量 `QCircularProgress`
   - 右侧：文件导入、发送/停止按钮
9. 文件导入时，输入框上方显示文件方框（进度、名称、关闭按钮）
10. 待执行消息列表显示在消息区底部
11. 滚动到底部按钮

---

## 三、交互需求

### 3.1 已实现

- [x] **WS/EventBus 主通道**：Chat 实时事件通过 `/v1/ws` 下发 Envelope；前端通过 `useEnvelopeStream` 消费
- [x] **暗黑模式可读性**：聊天记录在黑夜模式下保证正文、代码块、工具结果、时间戳等文本可读
- [x] **Agent/Team 标签栏暗黑模式**：使用明确的选中态、文字色和图标色
- [x] **Session 标题自动生成**：首次对话后由 LLM 生成标题，展示在 Session 列表和聊天顶部
- [x] **输入框键盘行为**：`Enter` 发送，`Shift + Enter` 换行
- [x] **Session 内切换模型**：后续发送使用当前选择的模型
- [x] **停止按钮**：模型回复或工具执行中，发送按钮切换为停止图标，点击可暂停/停止；单 Agent 和 Team 均支持
- [x] **Follow-up Queue（对话阶段连续发送）**：运行中再次发送自动入队（Steerable 直注或 Pending FIFO）；待发送列表可见、可取消/编辑；单 Agent 与 Team 均支持
- [x] **取消待执行消息**：前端可取消待执行队列中的消息（`CancelPendingMessage` RPC 已实现，前端 UI 已添加取消按钮）
- [x] **RunStatus / AwaitUserReply**：后端 RPC + Chat 页 `awaiting_user` 横幅与提交回复（`useChatWorkspace` 轮询 + `ChatMessagePanel`）

### 3.2 待优化（按优先级）

| 优先级 | 项 | 状态 | 说明 |
|--------|-----|------|------|
| P1 | 结构化工具事件卡片 | ✅ | `ChatToolCallCard`：参数 JSON、结果、耗时、`is_long_running` 徽章 |
| P1 | Reasoning 展示 | ✅ | `ChatReasoningPeek`：思考/正文分离；默认 **live tail 最后两行**；单击/滚轮/双击展开（见 [R-UX changelog](../changelog/2026-05-23-M55-Phase-R-UX-Channel-Format-Reasoning.md)） |
| P2 | Follow-up Queue UX（Cursor 对齐） | 待做 | 运行中解除 `sending` 阻塞；监听 `message_queued` 刷新队列；可选 `pending_enqueued` Envelope |
| P2 | RunStatus WS 驱动 | ✅ | 后端 `run_status` Envelope；前端监听 WS（切换会话 HTTP 校准） |
| P2 | Team 成员流 UX | ✅ | `team_member` 元数据 + 成员色条分栏 |
| P2 | WS 回放 UX | ✅ | 顶栏「正在同步历史事件…」 |
| P3 | 多模态附件 | 待做 | 前端占位 ID；后端无持久化/Vision 装配 |
| P3 | RunStatus 可恢复性 | 待做 | `awaitChans` / RunRegistry 进程内；重启后 `awaiting_user` 不可恢复 |
| P3 | 模型选项单一来源 | 部分 | 优先 `llm-provider-models` Platform；空列表时回退 `GetChatOptions("model")` |

### 3.3 已完成（历史项归档）

- [x] **Team 成员级实时流（协议+后端）**：`EventProjector.projectMemberText` + `MemberAgentKeys`
- [x] **Team 停止与待执行队列**
- [x] **pending_id 错误关联**
- [x] **Channel/Cron 并发保护**
- [x] **SessionTurn 一致性**
- [x] **WS 控制消息（协议层）**：`connected`/`pong`/`server_shutdown`；`replay_start`/`replay_end` 回调
- [x] **EventBuffer 清理策略**

### 3.3 已实现（本轮新增）

- [x] **编辑待执行消息**：前端可编辑待执行队列中的消息内容后重新发送（`UpdatePendingMessage` RPC + 前端编辑 UI）
- [x] **意图识别增强**：单 Agent/Team 聊天通过 EventBus/WS 发送 `intent_pass` 事件；前端接收并展示意图类型；用户消息标签行显示 intent_kind
- [x] **GetChatOptions 动态化**：Provider 和 Model 选项从 LLM Catalog 动态获取
- [x] **pendingEntry UUID**：使用 `github.com/google/uuid` 替代 `time.Now().UnixNano()` 生成 ID
- [x] **processPendingQueue 超时控制**：600 秒超时 + 取消传播（StopGeneration 可取消待执行队列处理）

---

## 四、技术债务与优化方向

### 4.1 已完成

- [x] ADK 残留代码清理（native_tools.go、tool_sse.go、turn_mount.go、adk.go、catalog/*）
- [x] `adkdeps` 包重命名为 `runtimedeps`，字段 `ADK` 重命名为 `RT`
- [x] `team/trpc_build.go` 错误处理从 `fmt.Errorf` 迁移到 `kerrors`
- [x] 重复 `sliceToSet` 函数统一到 `pkg/strutil.SliceToSet`
- [x] `firstNonEmpty` / `firstNonEmptyStr` / `firstNonEmptyString` 统一到 `pkg/strutil.FirstNonEmpty`
- [x] 历史 `chatHTTPPostBody` / 独立 SSE handler 已被 WS/EventBus 主通道替代
- [x] `NewChatService` 构造函数参数封装为 `ChatServiceDeps` struct
- [x] `memory_decode.go` 中通用 JSON 解码函数提取到 `pkg/jsonutil`
- [x] `compress_wire.go` 合并到 `session_compress.go`
- [x] `err == sql.ErrNoRows` 修正为 `errors.Is(err, sql.ErrNoRows)`
- [x] WS 上行 `buildChatOptions` 支持 attachments 引用
- [x] `hydratedAgent` 简化，移除冗余 AgentUsecase 分支
- [x] `runAgentTurn` 移除，直接调用 `runSingleAgentViaTRPC`
- [x] Session 标题 LLM 自动生成

### 4.2 待优化（2026-05-19 复核）

| 项 | 优先级 | 说明 |
|----|--------|------|
| 工具事件结构化 UI | P1 | Envelope 已有；Chat 气泡仍为 `🔧 name (status)` 简写 |
| Reasoning 展示规格 | P1 | 产品定稿 + `ChatMessagePanel` 折叠区 |
| RunStatus WS 驱动 | P2 | 替代 `useChatWorkspace` 2s 轮询 |
| Team 成员分栏展示 | P2 | `member_*` 已通；增强 UX（头像、角色标签） |
| 回放中 UI 提示 | P2 | `onReplayState` 已接入 transport；页面未展示 |
| 附件持久化 + Vision | P3 | 仅本地占位 |
| RunStatus 持久化/恢复 | P3 | 进程内 `awaitChans` |
| 模型列表单一真相源 | P3 | Platform 优先 + `GetChatOptions("model")` 回退（已实现） |

**已关闭**：SSE 主路径移除；RunRegistry；EnqueueUserMessage；processPendingQueue；AwaitHook Team；pending_id；Channel/Cron 互斥；recordTeamSessionTurn；EventBuffer TTL；member_* 后端投影；AwaitUserReply Chat UI；WS 控制消息协议层。

### 4.3 已优化（本轮新增）

- [x] `legacychat` 包及 `LEGACY_REST_ORIGIN` 已废弃并移除，所有 Chat 请求直接由 admin 进程内 trpc-agent-go 运行时处理
- [x] **WS/EventBus 主通道接入**：实时事件由 `EventProjector` 投影到 EventBus，并通过 `/v1/ws` 下发；WS 支持连接数限制、心跳、回放、订阅、取消和上行消息
- [x] **pendingQueue 大小限制**：`enqueuePending` 增加 `maxPendingPerSession=32` 上限，超出时返回空 ID 并返回 `BadRequest` 错误，防止内存泄漏
- [x] **processPendingQueue 错误上报**：待执行消息执行失败时通过 WS `error` Envelope 通知前端；`pending_id` 关联字段仍需统一
- [x] **toolEventMessage 重复定义消除**：提取 `toolEventToMessage` 到 `toolEventMarkdown.ts` 共享模块，`stores/app.ts` 和 `useChatWorkspace.ts` 统一使用
- [x] **WS 错误事件处理**：`useEnvelopeStream` / `useChatWorkspace` 监听 `error` Envelope 并展示错误通知
- [x] **state_delta/extensions 投影**：后端 Envelope 支持 `state_delta` 与 `extensions` 字段，前端类型已覆盖

---

## 子模块：Chat 执行过程卡片

> **模块**：Chat 对话 · 执行可见性
> **上级模块**：[1 chat.md](./1%20chat.md)
> **技术设计**：[1 chat-execution-trace.design.md](./1%20chat-execution-trace.design.md)
> **遵循**：[guides/AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · [guides/frontend-guide.md](../guides/frontend-guide.md)

---

## 1. 背景与目标

用户在 Chat 中与 Agent 对话时，需要**实时看到 Agent 正在做什么**（调用工具、加载/运行 Skill、通过 MCP 访问外部能力等），而不是仅看到最终文本回复或零散的 Markdown 提示。

本需求在**聊天消息流**中插入可折叠的**执行过程卡片（Execution Activity Card）**：卡片以图标 + 名称 + 状态为主信息，默认折叠详情，展开后可查看参数与结果。

> **与 Monitor 的边界**：Monitor → Logs / Traces 面向运维排障；Chat 执行卡片面向**终端用户理解 Agent 行为**，二者可共享同一条 `trace_id` / Span，但**不在 Chat 中展示 FlowLog 原始步骤**。

---

## 2. 用户故事

| ID | 角色 | 故事 | 优先级 |
|----|------|------|--------|
| U1 | 对话用户 | 发送消息后，我能看到 Agent 依次调用了哪些工具/Skill/MCP，以及当前是否仍在执行 | P0 |
| U2 | 对话用户 | 每张卡片标题显示**能力名称**（如 `read_file`、`skill_run`、`mcp_call`），执行中显示「正在执行」态，完成后显示**耗时** | P0 |
| U3 | 对话用户 | 卡片根据**成功 / 失败 / 阻塞（需确认）** 显示不同颜色与图标，失败时我能看到错误摘要 | P0 |
| U4 | 对话用户 | 卡片**默认折叠**，不占用阅读空间；需要时点击展开查看参数 JSON、结果摘要或 stderr | P0 |
| U5 | Team 用户 | 在 Team 会话中，卡片标明**执行成员 Agent**（author / agent_key） | P1 |
| U6 | 对话用户 | 刷新页面或 WS 重连后，仍能从历史消息中恢复已完成的执行卡片（与当轮持久化一致） | P1 |
| U7 | 管理员 | 敏感参数（API Key、token）在卡片详情中**脱敏**，不泄露明文 | P0 |

---

## 3. 功能范围

### 3.1 纳入范围（P0）

| 能力类型 | 典型名称 | 说明 |
|----------|----------|------|
| 平台工具 | `read_file`、`save_file`、`exec_command`、`todo_write` 等 | catalog / runtime 工具 |
| Skill | `skill_load`、`skill_run`、`skill_search` | 框架 Skill 工具族 |
| MCP | `mcp_call`、`mcp_list_tools` 及 MCP ToolSet 挂载工具 | 含 server_key / tool 名 |
| 内置能力 | `knowledge_search`、`call_agent`、`await_user_reply` | 随 Agent 策略挂载 |

### 3.2 纳入范围（P1）

| 能力类型 | 说明 |
|----------|------|
| 子 Agent | `transfer_to_agent`、`spawn_subagent` |
| Memory | `load_memory`、`preload_memory` |
| Team 步骤 | 与 `team_step_*` 事件对齐的成员级卡片（可选与成员气泡并列） |

### 3.3 不纳入范围

- Monitor FlowLog / Process Log 的 UI 迁移（仍在 Monitor → Logs）
- Graph 节点执行卡片（归属 Graph 执行页，见 `graphs` 模块）
- 修改 Agent 框架 `pkg/trpc-agent-go` 内部 Tool 语义（Aranea 仅在投影层扩展）

---

## 4. 交互规格

### 4.1 卡片布局（折叠态 — 默认）

```
┌─────────────────────────────────────────────────────────┐
│ [图标]  read_file                    正在执行…  ⏳      │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│ [图标]  skill_run · planning-and-task    1.2s  ✓      │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│ [图标]  mcp_call · github/list_issues    320ms  ✗      │
└─────────────────────────────────────────────────────────┘
```

| 区域 | 规则 |
|------|------|
| 图标 | 按 `activity_kind` 映射（tool / skill / mcp / …），见设计文档 §6 |
| 主标题 | `display_label`：优先 catalog 中文名，其次 runtime 名 |
| 副标题（可选） | 单行摘要：路径、skill 名、MCP server:tool，**不显示完整 JSON** |
| 右侧状态 | 执行中：`正在执行` + 动画；完成：耗时（≥1s 保留 1 位小数 + `s`，否则 `ms`） |
| 状态标记 | 成功 ✓ / 失败 ✗ / 阻塞 ⚠ / 长任务 ⏱ |

### 4.2 卡片布局（展开态）

点击卡片头部或 chevron 展开：

- **参数**：格式化 JSON（可滚动，最大高度约 280px）
- **结果**：stdout / JSON / 错误信息分区展示
- **元数据**（可选折叠）：`trace_id`、`run_id`、`invocation_id`、`duration_ms`

### 4.3 时间线位置

- 卡片插入在**当轮助手回复流**中，与 `text_delta` 交错，顺序等于 Agent **实际执行顺序**
- 同一 `activity_id` 仅对应**一张卡片**：`running` → `success|failed` 为**原地更新**，不新增重复行
- 不替代最终助手 Markdown 气泡；卡片与文本气泡均为 `role=assistant` 时间线上的独立条目

### 4.4 状态文案

| status | 折叠态标签 | 颜色语义 |
|--------|------------|----------|
| `running` | 正在执行 | warning / accent（UX `--color-warning`） |
| `success` | （仅耗时） | success（`--color-success`） |
| `failed` | 失败 + 耗时 | danger（`--color-danger`） |
| `blocked` | 待确认 | warning |
| `cancelled` | 已取消 | muted |

---

## 5. 非功能需求

| 项 | 要求 |
|----|------|
| 实时性 | WS 到达后 **200ms 内** UI 反映 running；完成态随 `tool_result` 即时更新 |
| 性能 | 单轮 ≥50 张卡片时列表仍流畅（虚拟滚动或增量 DOM，见设计 §8） |
| 可访问性 | 卡片 header 为 `button` 或带 `aria-expanded`；状态不仅依赖颜色 |
| 国际化 | 文案走 `vue-i18n`（`chat.activity.*`） |
| 安全 | 密钥字段脱敏；详情默认可复制但带审计提示（P2） |

---

## 6. 验收标准

- [x] 单 Agent 对话：调用任意已挂载工具时，Chat 中出现对应卡片；执行中显示「正在执行」，完成后显示耗时与成功/失败态
- [x] Skill：`skill_load` / `skill_run` 显示 Skill 图标与 skill 名称摘要
- [x] MCP：`mcp_call` 显示 server 与 tool 名摘要
- [x] 卡片**默认折叠**；展开后可见参数与结果
- [x] 同一工具调用不产生 duplicate 卡片（id 稳定 upsert）
- [x] WS 断线重连 + `last_event_id` 回放后，卡片状态与线上一致
- [x] 刷新会话历史：已完成轮次的卡片从 `messages` 只读还原（持久化命中）
- [x] Team 会话：卡片展示成员 Agent 标识（P1）
- [x] 失败工具调用：卡片红色边框 + 错误摘要，助手正文仍可继续输出

---

## 7. 关联文档

| 文档 | 关系 |
|------|------|
| [1 chat.md](./1%20chat.md) | Chat 主需求、WS Envelope 总览 |
| [1 chat.design.md](./1%20chat.design.md) | 现有 `tool_call` / `tool_result` 投影 |
| [52-flow-logger.design.md](./52-flow-logger.design.md) | Span / trace_id 同源；Chat 不展示 flow_log |
| [23 tools.md](./23%20tools.md) | 工具 catalog 与 risk_level |
| [20 skill.md](./20%20skill.md) | Skill 运行时 |
| `aranea-frontend-guide` SKILL §6 | 玻璃卡片视觉 token |
