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
internal/service/chat.go              ← ChatService 主结构 + SendChatMessage/GetChatOptions/StopGeneration/GetPendingMessages/CancelPendingMessage/UpdatePendingMessage/GetRunStatus/AwaitUserReply
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
| `member_message_start` / `member_delta` / `member_message_done` | team | 成员级实时消息；类型已定义，当前仍需在 Team Runner 稳定发射 |
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

- **上下文用量追踪**：每次 turn 后通过 `UpdateSessionContextFromLLMUsage` 更新 `context_used_tokens` / `context_used_ratio`
- **Context Window 默认值**：当 Agent 配置的 `context_window` ≤ 0 时，默认使用 128000 tokens
- **L0 压缩**：当 `context_used_ratio` 超过阈值（默认 0.6）时，`SessionCompressor.AfterNativeTurn()` 异步触发摘要压缩
- **记忆服务**：通过 `runtimedeps.Runtime.SessionMemory` 注入 SQLite 适配器，由 trpc Runner 自动管理 L0-L4

### 1.7 用量记录

- 每次对话后通过 `recordChatIngressUsage()` 写入 `model_token_usage_events` 表
- 支持流式/非流式两种记录路径
- 可通过 `CHAT_RECORD_USAGE_INGRESS=0` 禁用（用于双写过渡期）
- 记录字段包含：session_id、agent_key、team_id、model_api_id、input/output_tokens、latency_ms、tokens_per_second、stream_enabled、usage_kind、provider_code、prompt_mode

### 1.7.1 SessionTurn 持久化

- 单 Agent turn 完成后，`recordSessionTurn()` 写入 `session_turns` 表，记录 turn 索引、角色、模型、token 用量和耗时
- **当前边界**：~~Team turn 路径不调用 `recordSessionTurn`，导致 `session_turns` 表对 Team 会话无记录，需补齐~~ ✅ 已修复：新增 `recordTeamSessionTurn`，Team turn 成功后调用

### 1.7.2 Team Run 持久化

- Team turn 执行时，`CreateTeamRun()` 写入 `team_runs` 表，记录 team_id、session_id、状态和起止时间
- Team turn 中每个成员 Agent 执行步骤通过 `CreateTeamRunStep()` 写入 `team_run_steps` 表
- Team 定义可配置 `intent_anchor_agent_id`（指定意图识别使用的成员 Agent）和 `TurnDeadlineDuration`（turn 超时时间）

### 1.7.3 可观测性

- Chat turn 耗时通过 `arametrics.ChatTurnDuration` Prometheus 指标记录
- 意图识别超时为 45 秒

### 1.8 停止生成

- `ChatService.activeRuns sync.Map` 跟踪 `sessionID → trpcrunner.Runner`
- `runSingleAgentViaTRPC` 中 `Store(sessionID, runner)`，defer `Delete(sessionID)`
- `RunTRPCUserTurn` 传入 `trpcagent.WithRequestID(sessionID)` 使 Runner 用 sessionID 作为 requestID
- `StopGeneration` 优先尝试 `ManagedRunner.Cancel(sessionID)`，回退到 `Runner.Close()`
- **WS 取消路径**：前端可通过 WS `cancel` 上行或 HTTP `StopGeneration` 停止当前 run；二者共用 `activeRuns` / `pendingCancels`
- **连接管理**：`WSServer` 负责心跳、连接数限制、断线回放和 EventBus 订阅；模型事件不再直接写 HTTP 流
- **当前边界**：~~`activeRuns` 只在单 Agent `runSingleAgentViaTRPC` 中登记；Team turn 还未纳入同一套 cancel/pending 串行保护~~ ✅ 已修复：`teamRunGuard` 接入 `activeRuns`，`StopGeneration`/`CancelRun` 支持 Team cancel

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

### 1.9 待执行队列

- `ChatService.pendingQueue sync.Map` 跟踪 `sessionID → []pendingEntry`
- 单 Agent 执行中再次发送时，消息入队而非拒绝
- 当前 turn 完成后，`processPendingQueue` 自动从队列取下一条发送
- 前端可通过 `GetPendingMessages` 查看待执行消息
- 前端可通过 `CancelPendingMessage` 取消待执行消息
- 前端可通过 `UpdatePendingMessage` 编辑待执行消息内容
- `pendingEntry` 使用 UUID 生成唯一 ID
- `processPendingQueue` 设置 600 秒超时控制，并支持取消传播
- **容量限制**：每个 Session 最多 32 条待执行消息（`maxPendingPerSession`），超出时返回 `BadRequest` 错误
- **错误上报**：待执行消息执行失败时通过 EventBus/WS `error` Envelope 通知前端；`pending_id` 统一写入 `error.pending_id` 字段（✅ 已修复：metadata 双写已移除）
- **当前边界**：~~Team 会话尚未进入 `activeRuns`，待执行队列语义主要覆盖单 Agent turn；**Team turn 完成后不触发 `processPendingQueue`，队列中的消息永远不会被执行——这是功能性 Bug**~~ ✅ 已修复：Team turn defer 中调用 `processPendingQueue`，内部按 `OwnerType` 路由

### 1.10 RunStatus 与 AwaitUserReply

- `GetRunStatus` 返回 `idle | pending | running | awaiting_user | completed | failed | cancelled`
- `runStatuses sync.Map` 记录当前/最近一次 run 状态、run_id、错误信息和更新时间
- `makeAwaitReplyFunc` 注入 service await-reply tool，工具阻塞时将状态置为 `awaiting_user`
- `AwaitUserReply` 向 `awaitChans` 投递人工回复，恢复正在等待的 run
- 当前 `AwaitHook` 注入在单 Agent builder 路径；~~Team builder 还未接入~~ ✅ 已修复：Team Runner 通过 `SetAwaitHookProvider` 注入 `makeAwaitReplyFunc`；`runCtx` 注入 `serviceawaitreply.WithReplyFunc`
- 前端已有 API/composable 基础（`useRunStatus` 轮询 run 状态 + `submitReply` 提交回复；`useEnvelopeStream` 消费 WS 事件；`useChatStream`/`useTeamStream` 分别处理 Agent/Team 事件流；`EnvelopeDispatcher` 分发事件；`WsTransport` 管理 WS 连接），但 Chat 页仍需补完整 awaiting_user 展示与回复 UI
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
5. 流式消息显示打字动画
6. 底部输入区域：初始高度 100px，autogrow，最高 400px
7. 输入框底部工具条：
   - 左侧：对话模式 `QSelect`、模型提供商 `QSelect`、上下文使用量 `QCircularProgress`
   - 右侧：文件导入、发送/停止按钮
8. 文件导入时，输入框上方显示文件方框（进度、名称、关闭按钮）
9. 待执行消息列表显示在消息区底部
10. 滚动到底部按钮

---

## 三、交互需求

### 3.1 已实现

- [x] **WS/EventBus 主通道**：Chat 实时事件通过 `/v1/ws` 下发 Envelope；前端通过 `useEnvelopeStream` 消费
- [x] **暗黑模式可读性**：聊天记录在黑夜模式下保证正文、代码块、工具结果、时间戳等文本可读
- [x] **Agent/Team 标签栏暗黑模式**：使用明确的选中态、文字色和图标色
- [x] **Session 标题自动生成**：首次对话后由 LLM 生成标题，展示在 Session 列表和聊天顶部
- [x] **输入框键盘行为**：`Enter` 发送，`Shift + Enter` 换行
- [x] **Session 内切换模型**：后续发送使用当前选择的模型
- [x] **停止按钮（单 Agent）**：模型回复或工具执行中，发送按钮切换为停止图标，点击可暂停/停止；Team 停止需补 active run 接入
- [x] **待执行队列（单 Agent）**：执行中再次发送时进入"待执行"队列，可见，执行完成后按序发送；Team 串行保护需补齐
- [x] **取消待执行消息**：前端可取消待执行队列中的消息（`CancelPendingMessage` RPC 已实现，前端 UI 已添加取消按钮）
- [x] **RunStatus / AwaitUserReply 后端基础**：支持查询 run 状态与提交人工等待回复；Team 路径与 Chat 页 UI 待补

### 3.2 待实现

- [ ] **Team 成员级实时流**：稳定发射并消费 `member_message_start/member_delta/member_message_done`
- [x] **Team 停止与待执行队列**：Team turn 接入 active run/cancel/pending 语义；Team turn 完成后触发 `processPendingQueue` ✅
- [x] **AwaitUserReply 后端**：Team 注入 AwaitHook ✅；Chat 页展示 awaiting_user 并提交人工回复（前端待闭环）
- [x] **pending_id 错误关联**：统一 pending 失败的 `pending_id` 字段位置（消除 metadata 双写，统一到 `error.pending_id`）✅
- [ ] **结构化工具事件卡片**：基于 `tool_call/tool_result` 展示参数、结果、耗时、错误和长任务状态（`is_long_running`）
- [ ] **多模态附件闭环**：上传、持久化、权限校验、对象存储与 LLM Vision 输入
- [ ] **模型选项来源统一**：Chat 前端应明确使用 `GetChatOptions("provider"|"model")` 或 Platform Resource，避免双口径
- [ ] **RunStatus/AwaitUserReply 可恢复性**：避免进程重启导致等待态丢失
- [ ] **Reasoning 展示规格**：定义 `content.reasoning` 的前端展示方式（折叠/内联/独立区域）
- [x] **Channel/Cron 并发保护**：Channel webhook 并发请求受 `lockSession` per-session 互斥锁 + `runPlaceholder` 原子占位保护 ✅
- [x] **SessionTurn 一致性**：Team turn 路径补齐 `recordTeamSessionTurn` 调用 ✅
- [ ] **WS 控制消息前端消费**：`connected`/`pong`/`replay_start`/`replay_end`/`server_shutdown` 需在前端协议层正确处理
- [x] **EventBuffer 清理策略优化**：TTL 30min 自动过期 + 5min eviction ticker ✅

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

### 4.2 待优化

- [ ] 历史 SSE 文档口径需完全收敛到 WS/EventBus；不得再把 `/v1/chat/messages/stream` 写作当前端点
- [x] ~~Team turn 应纳入 `ChatService.activeRuns` 或等价会话级 run registry，保证停止与排队一致~~ ✅ `teamRunGuard` + `lockSession` per-session 互斥锁
- [x] ~~**Team turn 完成后需触发 `processPendingQueue`**，否则队列中的消息永远不会被执行（功能性 Bug）~~ ✅
- [x] ~~AwaitHook 应从单 Agent 扩展到 Team Builder，并在 Chat 页接入 `useRunStatus`~~ ✅ 后端已注入；前端 Chat 页 UI 待闭环
- [x] ~~pending 失败事件应统一 `pending_id` 字段位置，消除 metadata 与 `EnvelopeError.PendingID` 双写，统一到 `error.pending_id`~~ ✅
- [ ] Team Runner 应补成员级事件投影，避免前端只能看到聚合 `text_delta/text_done`
- [ ] 工具事件需要从简化文本升级为结构化 UI 与可观测字段（含 `is_long_running` 长任务标识）
- [ ] 附件 UI 目前只生成本地占位 ID，后端需补真正上传和 LLM 输入装配
- [ ] Chat 模型选择来源需统一，避免后端 Chat Options 与前端 Platform Resource 两套数据口径并存
- [ ] RunStatus/AwaitUserReply 当前为进程内状态，生产级长任务需持久化或恢复策略
- [x] ~~Channel/Cron 并发入口需受 activeRuns 互斥保护~~ ✅ `lockSession` per-session 互斥锁 + `runPlaceholder` 原子占位
- [x] ~~Team turn 路径需补齐 `recordSessionTurn` 调用，保证 `session_turns` 表对 Team 会话有记录~~ ✅ 新增 `recordTeamSessionTurn`
- [ ] WS 控制消息（`connected`/`pong`/`replay_*`/`server_shutdown`）需文档化并确保前端正确消费
- [x] ~~EventBuffer 需增加 TTL 自动过期清理策略，避免长时间运行 Session 积累事件~~ ✅ TTL 30min + 5min eviction
- [ ] 前端 `useRunStatus` 应从 HTTP 轮询改为 WS 事件驱动（`state_delta` 或专用事件）
- [ ] Reasoning 展示规格需定义（折叠/内联/独立区域）

### 4.3 已优化（本轮新增）

- [x] `legacychat` 包及 `LEGACY_REST_ORIGIN` 已废弃并移除，所有 Chat 请求直接由 admin 进程内 trpc-agent-go 运行时处理
- [x] **WS/EventBus 主通道接入**：实时事件由 `EventProjector` 投影到 EventBus，并通过 `/v1/ws` 下发；WS 支持连接数限制、心跳、回放、订阅、取消和上行消息
- [x] **pendingQueue 大小限制**：`enqueuePending` 增加 `maxPendingPerSession=32` 上限，超出时返回空 ID 并返回 `BadRequest` 错误，防止内存泄漏
- [x] **processPendingQueue 错误上报**：待执行消息执行失败时通过 WS `error` Envelope 通知前端；`pending_id` 关联字段仍需统一
- [x] **toolEventMessage 重复定义消除**：提取 `toolEventToMessage` 到 `toolEventMarkdown.ts` 共享模块，`stores/app.ts` 和 `useChatWorkspace.ts` 统一使用
- [x] **WS 错误事件处理**：`useEnvelopeStream` / `useChatWorkspace` 监听 `error` Envelope 并展示错误通知
- [x] **state_delta/extensions 投影**：后端 Envelope 支持 `state_delta` 与 `extensions` 字段，前端类型已覆盖
