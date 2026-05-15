# Chat 对话模块 — 需求规格

> 对话框是用户与 Agent/Team 交互的核心入口，负责 SSE 流式对话、上下文管理、用量记录、停止生成与待执行队列。

---

## 一、后端架构

### 1.1 代码分层（遵循 AI-DEVELOPMENT-SPECIFICATION.md）

```
api/kratos/chat/v1/chat.proto        ← 对话 API 契约（4 个 RPC + SSE 端点）
        ↓
internal/service/chat.go              ← ChatService 主结构 + SendChatMessage/GetChatOptions/StopGeneration/GetPendingMessages/CancelPendingMessage/UpdatePendingMessage
internal/service/chat_native.go       ← 原生对话入口（SSE + unary）+ streamWriter + hydratedAgent
internal/service/trpc_turn.go         ← trpc-agent-go 单 Agent turn 执行 + 事件流投影
internal/service/chat_usage_ingress.go ← 用量记录
internal/service/session_compress.go  ← L0 上下文压缩
internal/service/session_title_llm.go ← LLM 标题生成
        ↓
internal/agent/trpc_build.go         ← Agent 构建（BuildTRPCLLMAgent）
internal/agent/trpc_runtime.go       ← Runner 构建（NewTRPCRunner + RunTRPCUserTurn）
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
前端 POST /v1/chat/messages/stream (SSE)
  → ChatService.ProxyStream()
    → proxyNativeStream()
      → runNativeAgentTurn()
        → session.owner_type == "team"?
          → team.Runner.RunTurn() → BuildTRPCTeam → trpc Runner → SSE 事件流
        → session.owner_type == "agent"?
          → runSingleAgentViaTRPC()
            → BuildTRPCLLMAgent() → NewTRPCRunner() → RunTRPCUserTurn()
            → SSE 事件流（delta / tool.call / done / state_delta / extensions / branch / tag）
```

### 1.3 API 端点

| 方法 | 路径 | 协议 | 说明 |
|------|------|------|------|
| POST | `/v1/chat/messages` | unary | 非流式对话 |
| POST | `/v1/chat/messages/stream` | SSE | 流式对话（HTTP Server 层注册） |
| GET | `/v1/chat/options` | unary | 获取对话选项 |
| POST | `/v1/chat/stop` | unary | 停止生成 |
| GET | `/v1/chat/pending` | unary | 获取待执行消息列表 |
| POST | `/v1/chat/pending/cancel` | unary | 取消待执行消息 |
| POST | `/v1/chat/pending/update` | unary | 编辑待执行消息 |

### 1.4 SSE 事件协议

| 事件 | 方向 | 载荷 | 说明 |
|------|------|------|------|
| `user_message` | server→client | `{id, session_id, role, content_markdown, ...}` | 用户消息回显 |
| `delta` | server→client | `{content: "..."}` 或 `{reasoning_content: "..."}` | 流式增量文本 |
| `tool.call` | server→client | `{session_id, tool_name, tool_call_id}` | 工具调用通知（前端转换为 ToolUseEvent 渲染） |
| `done` | server→client | `{agent_message: {id, content_markdown, ...}}` | 生成完成 |
| `error` | server→client | `{message: "..."}` | 错误信息 |
| `state_delta` | server→client | `{session_id, state_delta: {...}}` | Session State 增量 |
| `extensions` | server→client | `{session_id, extensions: {...}}` | 事件扩展数据 |
| `branch` | server→client | `{session_id, branch: "..."}` | 分支标识 |
| `filter_key` | server→client | `{session_id, filter_key: "..."}` | 过滤键 |
| `tag` | server→client | `{session_id, tag: "..."}` | 事件标签 |
| `intent_pass` | server→client | `{outcome, duration_ms, intent_kind, ...}` | 意图识别结果 |
| `tool_event` | server→client | `{id, phase, status, tool_name, ...}` | 工具事件详情（预留，暂未发射） |
| `member_message_start` | server→client | `{id, role, content_markdown, ...}` | Team 成员消息开始（预留，暂未发射） |
| `member_delta` | server→client | `{message_id, content}` | Team 成员增量（预留，暂未发射） |
| `member_message_done` | server→client | `{agent_message: {...}}` | Team 成员消息完成（预留，暂未发射） |

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
- **L0 压缩**：当 `context_used_ratio` 超过阈值（默认 0.6）时，`SessionCompressor.AfterNativeTurn()` 异步触发摘要压缩
- **记忆服务**：通过 `runtimedeps.Runtime.SessionMemory` 注入 SQLite 适配器，由 trpc Runner 自动管理 L0-L4

### 1.7 用量记录

- 每次对话后通过 `recordChatIngressUsage()` 写入 `model_token_usage_events` 表
- 支持流式/非流式两种记录路径
- 可通过 `CHAT_RECORD_USAGE_INGRESS=0` 禁用（用于双写过渡期）
- 记录字段包含：session_id、agent_key、team_id、model_api_id、input/output_tokens、latency_ms、tokens_per_second、stream_enabled、usage_kind、provider_code、prompt_mode

### 1.8 停止生成

- `ChatService.activeRuns sync.Map` 跟踪 `sessionID → trpcrunner.Runner`
- `runSingleAgentViaTRPC` 中 `Store(sessionID, runner)`，defer `Delete(sessionID)`
- `RunTRPCUserTurn` 传入 `trpcagent.WithRequestID(sessionID)` 使 Runner 用 sessionID 作为 requestID
- `StopGeneration` 优先尝试 `ManagedRunner.Cancel(sessionID)`，回退到 `Runner.Close()`

### 1.9 待执行队列

- `ChatService.pendingQueue sync.Map` 跟踪 `sessionID → []pendingEntry`
- 执行中再次发送时，消息入队而非拒绝
- 当前 turn 完成后，`processPendingQueue` 自动从队列取下一条发送
- 前端可通过 `GetPendingMessages` 查看待执行消息
- 前端可通过 `CancelPendingMessage` 取消待执行消息
- 前端可通过 `UpdatePendingMessage` 编辑待执行消息内容
- `pendingEntry` 使用 UUID 生成唯一 ID
- `processPendingQueue` 设置 600 秒超时控制，并支持取消传播

### 1.10 Session 标题自动生成

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
4. 工具事件消息可折叠（details/summary）
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

- [x] **暗黑模式可读性**：聊天记录在黑夜模式下保证正文、代码块、工具结果、时间戳等文本可读
- [x] **Agent/Team 标签栏暗黑模式**：使用明确的选中态、文字色和图标色
- [x] **Session 标题自动生成**：首次对话后由 LLM 生成标题，展示在 Session 列表和聊天顶部
- [x] **输入框键盘行为**：`Enter` 发送，`Shift + Enter` 换行
- [x] **Session 内切换模型**：后续发送使用当前选择的模型
- [x] **停止按钮**：模型回复或工具执行中，发送按钮切换为停止图标，点击可暂停/停止
- [x] **待执行队列**：执行中再次发送时进入"待执行"队列，可见，执行完成后按序发送
- [x] **取消待执行消息**：前端可取消待执行队列中的消息（`CancelPendingMessage` RPC 已实现，前端 UI 已添加取消按钮）

### 3.2 待实现

（当前无待实现功能）

### 3.3 已实现（本轮新增）

- [x] **编辑待执行消息**：前端可编辑待执行队列中的消息内容后重新发送（`UpdatePendingMessage` RPC + 前端编辑 UI）
- [x] **意图识别增强**：单 Agent 聊天通过 SSE 发送 `intent_pass` 事件；前端接收并展示意图类型；用户消息标签行显示 intent_kind
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
- [x] `chatHTTPPostBody` 合并到 SSE handler（`proxyNativeStream` 内联解析）
- [x] `NewChatService` 构造函数参数封装为 `ChatServiceDeps` struct
- [x] `memory_decode.go` 中通用 JSON 解码函数提取到 `pkg/jsonutil`
- [x] `compress_wire.go` 合并到 `session_compress.go`
- [x] `err == sql.ErrNoRows` 修正为 `errors.Is(err, sql.ErrNoRows)`
- [x] SSE body 缺少 attachments 字段修复
- [x] `hydratedAgent` 简化，移除冗余 AgentUsecase 分支
- [x] `runAgentTurn` 移除，直接调用 `runSingleAgentViaTRPC`
- [x] Session 标题 LLM 自动生成

### 4.2 待优化

（当前无待优化项）

### 4.3 已优化（本轮新增）

- [x] `legacychat` 包及 `LEGACY_REST_ORIGIN` 已废弃并移除，所有 Chat 请求直接由 admin 进程内 trpc-agent-go 运行时处理
