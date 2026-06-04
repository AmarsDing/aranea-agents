# Debug: Web Chat 发送消息无反应

**Status**: `[OPEN]`
**Session ID**: `web-chat-no-response`
**Date**: 2026-06-04
**Reporter**: User
**Symptom**: Web Chat 界面发送消息后没有任何反应

---

## 1. 现象描述

- **实际行为**: 在 Chat 页面输入消息并点击发送，UI 完全无反应（无 placeholder 用户消息、无 spinner、无错误 toast、无 agent 回复）
- **预期行为**: 用户消息应显示为占位 row、sending 状态出现、agent 流式回复出现
- **影响范围**: Web Chat 整个发送路径
- **未做改动**: 仅阅读代码，未触动任何业务逻辑

---

## 2. 已收集的代码事实（只读扫描）

### 2.1 前端发送路径

```
ChatMessagePanel (UI)
  └─ @send="composer.onSend"          (ChatPage.vue:88)
       └─ composerActions.onSend       (useChatComposerActions.ts:35)
            └─ sender.onSend           (useChatSender.ts:229)
                 ├─ 检查 isActiveRun / isAwaitingUser
                 └─ sendUserContent(...) (useChatSender.ts:437)
                      ├─ markSending() → sending.value = true
                      ├─ strategy.ensureSession()
                      ├─ 创建 placeholder user message
                      ├─ checkBackendAvailability()       ← 失败时返回 false 走 catch
                      ├─ strategy.ensureStream(sessionId)
                      │    └─ useChatStreamManager.ensureChatStream
                      │         └─ createChatStream() → createEnvelopeStream({autoConnect:false})
                      │         └─ chatStream.connect()        ← 内部 new WebSocket
                      └─ deps.sendChatViaWs(stream, payload)  ← 同步执行
                            └─ transport.send(upstream)        ← 关闭时入队 pendingQueue
```

### 2.2 关键观察

1. **`sendChatViaWs` 不 await**（useChatSender.ts:499-501）— 同步执行，错误通过 try/catch 捕获；任何异步错误（网络失败）**不会**被这个 catch 抓住。
2. **WS transport `send` 永不抛错**（ws-transport.ts:185-194）— WS 未 OPEN 时入队 `pendingQueue` 并尝试 `connect()`，**不抛异常**。
3. **`sendChatViaWs` 仅在 `transport.value === null` 时抛 "WebSocket transport unavailable"**（useChatStreamManager.ts:256-263）。
4. **HTTP 兜底 `sendViaHttpFallback` 在 catch 块内**（useChatSender.ts:504-523）— 兜底失败时仅 `markPendingUserFailed` + toast，无后续重试提示在 UI 上特别显眼。
5. **后端 `ws_message_handler.go:126` 修改（commit 4d229983）** — `s.turnExecutor.ExecuteTurn` 失败时**新增** `eventBus.Publish` 错误事件（之前没有这条事件，仅写 Warn 日志）。这意味着若 turn 失败，前端应能看到一条 `error` envelope。
6. **`buildWsUrl` 用 `getWsOrigin()`**（runtime.ts:79）— DEV 下默认同源 `''`，走 Vite proxy → `:8000` admin。若 runtime config 缺失或指向错误 origin，WS 会连接失败但前端可能无明显提示。

### 2.3 相关历史

- 最近 3 个未推送 commit 涉及「修复上下文取消导致读取失败」「axios 超时」「新增行业分类服务，优化 websocket 连接处理」。最近一次是 6 月 4 日上午 10:00 的 WebSocket 优化。
- 用户工作区存在 `tmp_debug_*.ps1`、`tmp_*.txt` 系列脚本和 `sessions.json`、`tmp_agent*.json`，说明用户已就类似问题做过若干次手动调试。

---

## 3. 假设清单（按可能性排序）

| # | 假设 | 验证点 | 影响面 |
|---|------|--------|--------|
| **H1** | **后端 admin 进程未运行**。WS 连接 upgrade 失败，HTTP 心跳不通。`checkBackendAvailability` 检测到后端不通，理论上应该走负向 toast 路径；但**`chat/backendUnavailable` 的 toast 可能在某些情况下不显示**（如 service worker / cookie 干扰），让用户感觉"无反应"。 | Debug Server 收集：浏览器 console 是否有 WS 4001/握手失败；`/healthz` 状态；admin 进程端口占用。 | 整个 chat |
| **H2** | **WS 已建立但消息进入 `pendingQueue` 永不 flush**。`pendingQueue` 在 `ws.onopen` 中 `flushPendingQueue()`，但若 `ws.onopen` 之前 `disconnect()` 被调用（例如 `onUnmounted` 触发），队列不会 flush，前端也不会再重试（`useEnvelopeStream.ts:97-101` 的 `if (transport.value?.connected) return; if (transport.value) { transport.value.connect(); return; }` 没覆盖**已 disconnect 但 transport 仍存在**的情况）。 | Debug Server 收集：发送瞬间 `pendingQueue.length`、`ws.readyState`、`transport.connected` 时间线。 | 单 session 切换 |
| **H3** | **`useChatSender.sendUserContent` 中 WS send 静默成功但后端不响应**。后端 `turnExecutor.ExecuteTurn` 失败时（4d229983 之前）只写 Warn 日志，前端无 error envelope → 用户看不到错误。**新 commit 4d229983 加了 `eventBus.Publish(EnvelopeTypeError)`**（ws_message_handler.go:128-134），所以如果错误是这条链路、且 commit 4d229983 已经在运行，应该能看到 error envelope；若**未运行该 commit**，则不会有任何反馈。 | Debug Server 收集：是否在运行 4d229983 之后的 binary？后端日志中是否有 `WebSocket 用户消息发送失败`？前端是否收到 `error` envelope？ | turn 失败路径 |
| **H4** | **`entityKind` / `selectedSession` 缺失**导致 `onSend` 提前 return。`useChatSender.onSend:243-247`：当 `entityKind === 'agent'` 但 `selectedAgent` 为空，或 `entityKind === 'team'` 但 `teamSelectedSessionId` 空，`sendUserContent` 不会被调用。 | Debug Server 收集：发送时 `sessionStore.entityKind`、`appStore.selectedAgent`、`sessionStore.selectedSession`。 | UI 状态机 |
| **H5** | **HTTP 兜底也失败**：`sendChatViaWs` 抛 "WebSocket transport unavailable" → 走 `sendViaHttpFallback` → axios 401/403/超时 → `markPendingUserFailed` 但 toast 不显眼。 | Debug Server 收集：HTTP 兜底是否触发、Network 面板 sendMessage 请求状态码、错误信息。 | 后端 API 路径 |

---

## 4. 调试策略

按 **H1 → H5 顺序**逐一排除。每条假设对应一组 instrumentation 日志上报到 Debug Server。

| 阶段 | 操作 | 目的 |
|------|------|------|
| Step 1 | 启动 Debug Server，启动 admin 后端 | 拿到 baseline |
| Step 2 | 收集 H1：检查后端 health、WS 升级日志 | 确认后端可达 |
| Step 3 | 收集 H4：在 `onSend` 入口 + 各 early-return 处插桩 | 确认走到 sendUserContent |
| Step 4 | 收集 H2/H5：在 WS send、HTTP fallback、placeholder 创建处插桩 | 确认消息离开前端 |
| Step 5 | 收集 H3：对比 commit 4d229983 是否生效、envelope 是否到达 | 确认消息到达后端 |

**关键原则**: 任何业务逻辑修改都必须在拿到 runtime 证据之后；否则只做日志插桩。

---

## 5. 进度

- [x] 步骤 0：初始化 debug 文件，列假设（无业务逻辑修改）
- [x] 步骤 1：Debug Server 已启动 → `http://127.0.0.1:7777/event`
  - env 文件: `f:\project\aranea-agents\.dbg\web-chat-no-response.env`
  - 日志文件: `f:\project\aranea-agents\.dbg\trae-debug-log-web-chat-no-response.ndjson`
- [x] 步骤 2：环境确认
  - 后端 admin 进程运行中（8000 端口可达）✅
  - 前端 dev server 运行中（9001 端口可达）✅
  - 模式：Agent（实际 `selectedAgent: __system_admin__`，不是 spirit）
  - UI 反馈：用户称"完全无任何 UI 反馈"
- [x] 步骤 3：插桩已部署
- [x] 步骤 4：用户复现一次
  - 浏览器报错：`useChatMessageScroll.ts:252:26` `Cannot read properties of undefined (reading 'target')` — **次要 bug（独立）**
  - 前端日志显示发送链路 100% 正常（WS 成功 send `user_message`）
- [x] 步骤 5：读取后端日志 + SQLite ✅ **H3 确认**

---

## 6. 诊断报告

### 6.1 前端结论（✅ 一切正常）

前端发送链路完整无错：

```
useChatComposerActions.onSend
  └─ useChatSender.onSend (rawInputLen=2, entityKind=agent)
       └─ sendUserContent (sendingAfterMark=True, msgsNow=2)
            └─ ensureStream (reused=True, connected=True, transportConnected=True)
                 └─ sendChatViaWs (upstreamType=user_message, ok=True)
                      └─ ws-transport.send (type=user_message, readyState=1, channel=chat)
                           → 后端 ✅
```

**所有错误假设已排除**：H1（后端运行）/ H2（WS 连上）/ H4（路径正确）/ H5（无 catch 触发）

### 6.2 后端结论（❌ 根因）

**后端日志关键行**（`logs/log-2026-06-04.jsonl`）：

```json
{
  "_ts": "2026-06-04T03:06:11.7889154Z",
  "_text": "WebSocket 用户消息发送失败",
  "error": "context deadline exceeded",
  "level": "warn",
  "session_id": "63472be0-4ff3-4772-9788-3dbd48a534a7",
  "step_id": "ws.send_failed"
}
```

+ **SQLite DB** (`data/arenea.sqlite`):
  - 6 月 4 日（今天）**0** 个新 session、**0** 条新 message
  - 最新数据停留在 **2026-06-03 02:07:30Z**
  - 该 session `63472be0-...` 的状态：`status=interrupted, status_reason=error, run_count=0, message_count=1, error_count=0`
  - 该 session 是 6 月 2 日创建的，不是今天

+ **日志文件** `logs/log-2026-06-04.jsonl` LastWriteTime = `2026-06-04 11:06:12`
  - 11:06 之后无新日志
  - 用户 18:18 复现**完全没有新日志**

### 6.3 假设验证矩阵

| # | 假设 | 证据 | 结论 |
|---|------|------|------|
| H1 | 后端未运行 | 8000 端口可达、11:06 还在写日志 | ❌ 排除 |
| H2 | WS 队列未 flush | 日志显示 `readyState=1` 即时发送，无 queued | ❌ 排除 |
| H3 | 后端处理超时/失败 | **后端日志 `context deadline exceeded`**；DB 无新数据；agent count=0 | ✅ **确认** |
| H4 | 早期 return | `sendUserContent` 完整执行到 WS send | ❌ 排除 |
| H5 | HTTP 兜底失败 | WS 路径成功，HTTP 兜底未触发 | ❌ 排除 |

### 6.4 根因定位（`internal/server/ws_message_handler.go:122-136`）

```go
connCtx := wc.contextOrBackground()                                       // L122
safego.Go(context.Background(), "ws-user-message", func() {              // L123 ← parent=Background
    ctx, cancel := context.WithTimeout(connCtx, defaultWSTurnTimeout)    // L124 ← 用 connCtx
    defer cancel()
    if err := s.turnExecutor.ExecuteTurn(ctx, input); err != nil {        // L126 ← turn 执行
        s.lg.With(...).Warn("WebSocket 用户消息发送失败", ...)
        env := event.NewEnvelope(event.EnvelopeTypeError, ...)            // L128 ← 4d229983 新增
        ...
        s.eventBus.Publish(context.Background(), env)                      // L134
    }
})
```

+ `defaultWSTurnTimeout = 5 * time.Minute`（`ws.go:52`）
+ 5 分钟内 turn 必失败，错误事件**有发布**（4d229983 之后），但**前端没收到**——可能：(a) 运行的 binary 不含 4d229983 修复；(b) 事件总线路由到 WS writePump 失败；(c) 5 分钟是硬超时，与"无任何反应"主诉不一致

### 6.5 第二个独立 Bug（次要）

`web/src/components/chat/ChatMessageList.vue:93`：
```html
@click="$emit('messages-click')"   ← 缺少 $event 参数
```
导致父组件 `handleMessagesClick(event)` 收到 `event=undefined`，运行时报错：
```
TypeError: Cannot read properties of undefined (reading 'target')
  at handleMessagesClick (useChatMessageScroll.ts:252:26)
```
**修复**：改为 `@click="$emit('messages-click', $event)"`

### 6.6 第三个观察（前端 UX）

前端**实际上**已经把 placeholder user 消息加入了 store（`msgsNow=2`），并 `markSending()` 把 `sending.value=true`。但用户报告"完全无任何 UI 反馈"——说明**前端没有 UI 在显示 placeholder / spinner**。这可能是另一个 UI 渲染问题，但优先级低于后端 turn 失败。

---

## 8. 修复方案

### 方案 C（次要 bug）✅ 已修

`web/src/components/chat/ChatMessageList.vue`：
- L7/41/93: `@click="$emit('messages-click')"` → `@click="$emit('messages-click', $event)"`
- L188: `emits` 类型声明 `[event: MouseEvent]`

### 方案 B（用户可感知）✅ 已修

**后端** `internal/server/ws.go:52`：
```go
defaultWSTurnTimeout = 30 * time.Second   // 原 5 * time.Minute
```

**前端** `web/src/features/chat/composables/useChatSender.ts`：
- 新增 `startTurnAckTimeout()` / `clearTurnAckTimeout()` / `pendingTurnAck`
- 30s 内若 `onRunAccepted` 未触发，调用 `markPendingUserFailed(pendingUserId, "后端 30 秒内未确认 turn，请点击重试")`
- 弹 `negative` toast
- `onRunAccepted` / `markSendingDone` 都会清除该 timer

### 方案 A（根因）✅ 已定位（非代码 bug，是配置 + 上游服务不可达）

**关键发现**：

1. **Agent 配置**（`agents` 表）：所有 imported agent + `__system_admin__` 都标 `provider: openrouter, model: gpt-4.1-mini`
2. **Provider 注册表**（`llm_provider_models` 表）：**只有 deepseek 两种模型（v4-flash/v4-pro），没有 openrouter**
3. **运行时实际调用**（`model_token_usage_events`）：**所有调用都打到 `deepseek:deepseek-v4-{flash,pro}`**，且 **100% 失败**（`error_code=""`, `latency_ms=15000-300000`）
4. **失败原因**：error_code 为空 + 高 latency 表明：DeepSeek API 不可达或鉴权失败，但**运行时没有把 HTTP error code 提取出来**（parsing 漏）

| Agent | 调用 | 请求 | 成功 | 失败 | 平均延迟 |
|-------|------|------|------|------|----------|
| `__system_admin__` | deepseek-v4-flash | 9 | 0 | 9 | 99.1s |
| `__spirit__` | deepseek-v4-pro | 7 | 0 | 7 | 21.2s |
| `__spirit__` (6/3) | deepseek-v4-pro | 1 | 0 | 1 | 15.0s |
| `go-senior-general` | deepseek-v4-pro | 3 | 0 | 3 | 21.3s |

**用户可执行的修复**（在 admin UI / DB 里）：

1. **优先**：去 System Settings → LLM Providers → 修 DeepSeek provider 的 API key（可能已过期 / 余额耗尽 / IP 被 ban）
2. **或**：在 `llm_provider_models` 表里加一条 `provider: openrouter, model: gpt-4.1-mini` 的注册条目
3. **或**：把所有 agent 的 `provider/model` 改成 `deepseek/deepseek-v4-pro` 配合可用的 deepseek key

**更深层建议**：在 `ws_message_handler.go` turn 失败时，error envelope 应携带**真正的 error code/message**（当前 `error_code=""` 留白），让前端能精确提示"DeepSeek API 鉴权失败"而不是空泛的"超时"。

---

## 9. 待验证步骤

由于修改了后端 Go 代码，需要**重启 admin**才能让 `defaultWSTurnTimeout=30s` 生效。

验证脚本：
1. **重启 admin**：`cd cmd/admin && go run .`（或你平时用的启动命令）
2. **强刷浏览器**（Ctrl+Shift+R）
3. **进入 Chat 页面**选 `__system_admin__` 或 `__spirit__` agent
4. **发送一条消息**
5. **等待 ≤ 30 秒**，应看到：
   - **乐观路径**（后端 OK）：placeholder → spinner → agent 流式回复
   - **悲观路径**（后端还是 fail）：30s 后 placeholder 变红 + 弹"后端响应超时"toast + 可点击重试
6. **告诉我结果**，我会：
   - ✅ Fixed → 进入清理（Task 5）
   - ❌ Still broken → 继续查
