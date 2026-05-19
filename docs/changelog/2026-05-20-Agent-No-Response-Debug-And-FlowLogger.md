# Agent 无响应问题排查与 FlowLogger 流程日志系统

## 一、问题描述

在聊天界面中发送消息给 Agent 后，Agent 无任何响应。前端无回复内容，后端无有效错误提示，用户体验极差——既没有返回结果，也没有超时或错误提示。

## 二、问题原因

经过逐步排查，定位到以下三个核心原因：

### 2.1 SlogBridge 死锁（根本原因）

**问题链路：**

```
用户消息 → chat service → runner.Run → agent.RunWithPlugins → LLM 调用
    → 事件回调 → audit_log 插件 onEvent → slog.Info
    → SlogBridge.Handle → bus.Publish（同步，持锁）
    → 事件订阅者回调 → 再次 slog → SlogBridge.Handle（再次请求锁）
    → 死锁
```

**根因分析：**

- `SlogBridge.Handle` 使用 `bus.Publish` 同步发布事件到事件总线
- 事件总线内部使用 `RWMutex` 保护订阅者列表
- 如果订阅者在处理事件时调用了 `slog`（如 `EventBusConsumer`），会重新进入 `SlogBridge.Handle`
- 由于 `RWMutex` 不可重入，导致死锁

**具体触发场景：**

`audit_log` 插件的 `onEvent` 方法中使用了 `slog.Info` 记录审计日志，而 `slog` 的默认 handler 正是 `SlogBridge`，形成了以下调用环：

```
audit_log.onEvent → slog.Info → SlogBridge.Handle → bus.Publish → 订阅者 → slog → 死锁
```

### 2.2 事件流错误未传播

`ConsumeEventStream` 在消费 LLM 返回的事件流时，没有追踪错误状态和内容接收情况。当 LLM 调用失败或返回空内容时，上游无法感知，导致用户收到空响应而没有任何错误提示。

### 2.3 缺少超时保护

LLM 调用没有 turn 级别的超时保护。如果 LLM API 挂起（如网络问题、API 无响应），HTTP 请求会无限期阻塞，用户永远等不到结果。

### 2.4 调试信息不可见

排查过程中发现，所有调试信息仅输出到 `os.Stderr`，前端无法查看，运维人员也无法通过 monitor 面板了解流程走到了哪一步。`slog` 的输出被框架配置覆盖，终端中看不到任何日志。

## 三、解决方案

### 3.1 SlogBridge 死锁修复

**文件：** `internal/event/slog_bridge.go`

**方案：** 添加可重入保护 + 异步发布

```go
type SlogBridge struct {
    // ...
    inPublish sync.Mutex // 重入保护
}

func (b *SlogBridge) Handle(ctx context.Context, r slog.Record) error {
    if b.bus != nil && r.Level >= b.level {
        if b.inPublish.TryLock() {  // 非阻塞获取锁
            // ... 构建事件 ...
            go func() {
                defer b.inPublish.Unlock()
                bus.Publish(context.Background(), env)  // 异步发布
            }()
        }
        // TryLock 失败说明正在发布中，跳过避免死锁
    }
}
```

**关键点：**
- 使用 `TryLock` 替代 `Lock`，重入时直接跳过而非阻塞
- 使用 `go func()` 异步发布事件，避免在 `Handle` 中同步等待

### 3.2 audit_log 插件阻塞修复

**文件：** `internal/plugin/trpc/audit.go`

**方案：** 用 `os.Stderr` 直接输出替代 `slog.Info`，绕过 SlogBridge

```go
func (a *AuditLogPlugin) onEvent(...) (*trpcevent.Event, error) {
    fmt.Fprintf(os.Stderr, "[audit_log.on_event] plugin=%s session_id=%s ...\n", ...)
    os.Stderr.Sync()
    // ... 原有逻辑 ...
}
```

### 3.3 事件流错误传播

**文件：** `pkg/trpc-agent-go/agent/chat_agent.go`

**方案：** 增强 `EventStreamResult` 结构体，追踪错误和内容状态

```go
type EventStreamResult struct {
    Reply        strings.Builder
    Reasoning    strings.Builder
    HasError     bool   // 是否有错误
    HasContent   bool   // 是否收到内容
    LastError    string // 最后一个错误信息
    PromptTok    int
    CompletionTok int
}
```

在消费循环中追踪状态，并在上层检测空响应：

```go
if replyText == "" {
    fallback := "I received your message but was unable to generate a response."
    if result.HasError {
        fallback = fmt.Sprintf("AI service error: %s", result.LastError)
    } else if !result.HasContent {
        fallback = "The AI model did not produce any output."
    }
    return userMsg, biz.ChatMessage{}, kerrors.InternalServer("CHAT_AGENT", fallback)
}
```

### 3.4 Turn 级超时保护

**文件：** `internal/service/trpc_turn.go`

**方案：** 对所有 turn 统一应用 5 分钟超时

```go
const defaultTurnTimeout = 5 * time.Minute
if deadline, hasDeadline := ctx.Deadline(); !hasDeadline || time.Until(deadline) > defaultTurnTimeout {
    var cancel context.CancelFunc
    ctx, cancel = context.WithTimeout(ctx, defaultTurnTimeout)
    defer cancel()
}
```

### 3.5 FlowLogger 流程日志系统

**文件：** `internal/event/flow_logger.go`

**目标：** 将聊天流程中每个关键步骤的结构化日志输出到前端 monitor 面板，方便实时查看流程进度和排查问题。

**核心设计：**

```go
type FlowPhase string

const (
    FlowPhaseStart FlowPhase = "start"  // 步骤开始
    FlowPhaseDone  FlowPhase = "done"   // 步骤完成
    FlowPhaseError FlowPhase = "error"  // 步骤出错
    FlowPhaseSkip  FlowPhase = "skip"   // 步骤跳过
)

type FlowLogger struct {
    bus       Bus      // 事件总线，发布到 monitor 通道
    sessionID string   // 会话ID
    agentKey  string   // Agent标识
    mu     sync.Mutex
    timers map[string]time.Time  // 步骤计时器
}
```

**日志格式：** 通过事件总线 monitor 通道发布，前端可直接订阅显示

```json
{
  "type": "flow_log",
  "step": "chat.llm_call",
  "phase": "done",
  "message": "LLM调用返回，开始消费事件流",
  "session_id": "xxx",
  "agent_key": "xxx",
  "duration_ms": 3200,
  "extra": {
    "prompt_tok": 150,
    "completion_tok": 80
  }
}
```

## 四、聊天流程步骤标记

以下为完整聊天流程中所有 FlowLogger 标记的步骤，前端可通过 monitor 面板实时查看：

| 序号 | 步骤标记 | 阶段 | 说明 | 关键附加信息 |
|------|---------|------|------|------------|
| 1 | `chat.receive` | start | 收到用户消息 | content_len |
| 2 | `chat.active_check` | done | 检查是否有活跃运行 | has_active |
| 3 | `chat.session_fetch` | done/error | 获取会话信息 | owner_type, agent_id |
| 4 | `chat.agent_hydrate` | done/error | 加载Agent配置 | agent_key, provider, model |
| 5 | `chat.provider_resolve` | done | Provider/Model解析完成 | provider, model, dialog_mode |
| 6 | `chat.turn_enter` | start | 进入Agent Turn执行 | dialog_mode, provider, model |
| 7 | `chat.agent_build` | done/error | 构建Agent实例 | provider, model |
| 8 | `chat.plugins_load` | done | 插件加载完成 | plugin_count |
| 9 | `chat.user_msg_persist` | done | 用户消息已持久化 | — |
| 10 | `chat.llm_call` | start/done/error | 调用LLM模型 | duration_ms |
| 11 | `chat.stream_consume` | done | 事件流消费完成 | reply_len, has_error, prompt_tok, completion_tok |
| 12 | `chat.assistant_msg_persist` | done | 助手消息已持久化 | reply_len |
| 13 | `chat.turn_execute` | done | Turn执行完成 | run_id, reply_len, prompt_tok, completion_tok |
| 14 | `chat.turn_timeout` | error | 请求超时 | timeout |
| 15 | `chat.empty_reply` | error | Agent未产生响应 | has_error, last_error, has_content |

## 五、调试探针清理

将 `internal/` 目录下所有 `fmt.Fprintf(os.Stderr, "[DEBUG:agent-no-response]...")` 调试探针替换为结构化日志：

| 文件 | 替换方式 |
|------|---------|
| `internal/service/chat_native.go` | FlowLogger |
| `internal/service/trpc_turn.go` | FlowLogger |
| `internal/agent/trpc_runtime.go` | 删除冗余探针（上层已有覆盖） |
| `internal/provider/trpc_llm.go` | slog 结构化日志 |
| `internal/plugin/trpc/event_bridge.go` | 删除调试探针（保留 metrics） |
| `internal/plugin/trpc/hook_events.go` | 删除调试探针（保留核心逻辑） |

**框架目录（`pkg/`）保持不变**，按需求不跟踪框架内部代码。

## 六、经验总结

1. **日志系统重入是常见死锁源**：当日志 handler 触发的事件回调中再次使用日志时，极易形成死锁。解决方案是使用 `TryLock` + 异步发布。

2. **关键路径必须有超时保护**：对外部 API（如 LLM）的调用必须设置超时，避免无限阻塞。

3. **错误必须传播到用户层**：内部错误不应被静默吞掉，空响应场景需要明确的用户提示。

4. **调试信息应可观测**：仅输出到 stderr 的调试信息对前端和运维不可见，需要通过事件总线等机制输送到前端 monitor 面板。

5. **结构化日志优于格式化字符串**：`FlowLogger` 的 step/phase/extra 结构比 `fmt.Fprintf` 的自由文本更易于前端解析和展示。
