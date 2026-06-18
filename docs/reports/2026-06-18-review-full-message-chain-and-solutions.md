# 完整消息链设计与问题解决方案（无超时优化版）

> 生成日期：2026-06-18
> 报告类型：全链路设计与问题综合报告（基于 9 条需求约束优化）
> 合并来源：
> - `2026-06-13-activity-first-restructure-optimized-proposal.md`（Activity-First 重构方案）
> - `2026-06-15-review-chat-ui-unified-event-stream.md`（聊天 UI 统一事件流审查）
> - `2026-06-17-research-orchestration-longtask-memory-upgrade.md`（综合升级调研）
> - `2026-06-18-review-issues-and-solutions.md`（残留问题与根本性方案）
>
> **核对方式**：3 个子代理并行核对 50+ 核心文件，覆盖前端发送、后端响应、前端渲染、超时配置、重试机制、可观测性基础设施

---

## 一、概要

| 维度 | 数量 |
|------|------|
| 需求约束 | 9 条（无超时 / 持续连接 / 队列管理 / 动态加载 / 同机部署 / 可观测性 等） |
| 消息链阶段 | 4（UI 发送 → 后端响应 → LLM 响应 → 前端展示） |
| 代码核对发现问题 | 39（前端发送 15 + 后端响应 12 + 前端渲染 12） |
| 去重后独立问题 | 30（含已解决 12 + 未实施 18） |
| 长任务目标 | 完成用户指令（**无时间限制**，无 24h deadline） |
| 根本性方案族 | 6（A 无超时重试 / B 前端收敛 / C 记忆生产化 / D 通道分离 / E 可观测性 / F 动态加载） |
| 总工作量 | 82 人天 |

---

## 二、需求约束与设计原则

### 2.1 九条需求约束

| # | 需求 | 设计影响 |
|---|------|---------|
| 1 | 用户发送消息后不能有超时，要持续连接 | 移除 dispatch 30s / turn-ack 30s，WS 连接持久保持 |
| 2 | 无论简单还是长任务都不能超时，以完成任务为最终目的 | 移除所有任务级超时（first-byte/stall/stream/LLM 30min/24h deadline），LLM 断开/DB 超时自动重试 |
| 3 | 前后端通信设计是否合理 | 通道职责分离：HTTP 仅命令，WS 唯一数据通道（同机部署简化） |
| 4 | 执行中可发消息，可排队，可立即发送和删除 | **后端 API + 前端 UI 已完整实现**（核对确认），仅需文档化 |
| 5 | 前端动态加载大的折叠消息 | VirtualScroller + 大消息默认折叠 + 按需展开 |
| 6 | 移动端先不考虑 | 移除 `<1024px` 折叠逻辑，专注桌面端体验 |
| 7 | server 和 quasar-app 在同一台计算机 | WS 为本地连接（延迟 <1ms），无需 CDN/复杂重连策略 |
| 8 | 多租户先不考虑 | 移除租户隔离检查，简化授权 |
| 9 | 后端动作可观测性：任务计划、team 执行状态、graph 等 | 后端 API 齐全（Team Observatory + Graph Visualize），需补前端 dashboard + Task Plan 查询 API |

### 2.2 核心设计原则：无超时 + 自动重试

```
┌─────────────────────────────────────────────────────────────┐
│                    无超时原则（No-Timeout）                    │
├─────────────────────────────────────────────────────────────┤
│  任务执行：无任何时间上限，持续运行直到完成或用户取消            │
│  连接健康：仅保留心跳检测（WS ping/pong + run_heartbeat）      │
│  异常恢复：LLM 断开 / DB 超时 / 网络抖动 → 自动重试            │
│  用户控制：用户可随时取消任务（Cancel）或插入新消息（Interrupt）│
└─────────────────────────────────────────────────────────────┘
```

**超时分类与处理**：

| 超时类型 | 当前值 | 新方案 | 理由 |
|---------|--------|--------|------|
| dispatch 30s | 30,000ms | **移除** | 用户发送后持续等待，不超时 |
| turn-ack 30s | 30,000ms | **移除** | 同上 |
| first-byte 90s | 90,000ms | **移除** | 慢模型不应被中断 |
| stall 180s | 180,000ms | **改为通知**（不中断） | 仅提示"似乎停滞"，不取消任务 |
| stream 10min | 600,000ms | **移除** | 流式持续到完成 |
| LLM HTTP 30min | 1,800s | **移除** + 自动重试 | LLM 断开 → 指数退避重试 |
| DB 事务 30s | 30s | **保留**（SQLite 需要）+ 自动重试 | 30s 超时 → 回滚 → 重试 |
| 24h hard deadline | 24h | **移除** | 任务无时间限制 |
| WS TurnTimeout 5min | 300s | **移除** | WS turn 不超时 |
| WS ping/pong | 30s/60s | **保留** | 连接健康检测 |
| run_heartbeat 10s | 10s | **保留** | 进度感知（不触发超时） |

### 2.3 同机部署简化

由于 server 和 quasar-app 在同一台计算机：

| 维度 | 常规部署 | 同机部署简化 |
|------|---------|-------------|
| WS 延迟 | 50-200ms | <1ms |
| 重连策略 | 快速+慢速双模式 | 仅快速模式（即时重连） |
| CDN | 需要 | 不需要 |
| WS 重连次数上限 | 10 次后放弃 | **无上限**（持续重连） |
| HTTP fallback | 必要 | 仅作为 WS 完全不可用时的降级 |
| 跨域 | 需配置 CORS | 同源，无需 CORS |

---

## 三、完整消息链设计

### 3.1 消息链全景图（无超时版）

```
┌─────────────────────────────────────────────────────────────────────┐
│ 阶段 1：UI 发送数据                                                  │
│ ┌──────────┐    ┌──────────────┐    ┌─────────────┐               │
│ │ 用户输入  │ → │ useChatSender │ → │ WS 命令通道  │               │
│ │ 聊天框    │    │ .onSend()     │    │ 提交消息     │               │
│ └──────────┘    └──────────────┘    └──────┬──────┘               │
│                                             │                       │
│  pending-user 占位消息创建                   │                       │
│  无 dispatch 超时（持续等待）                │                       │
│  任务执行中 → 排队（ChatPendingQueue UI）    │                       │
└─────────────────────────────────────────────┼───────────────────────┘
                                              │
┌─────────────────────────────────────────────┼───────────────────────┐
│ 阶段 2：后端响应                              ▼                       │
│ ┌────────────────┐    ┌─────────────────┐    ┌──────────────┐       │
│ │ WS handler     │ → │ Turn            │ → │ Admission    │       │
│ │ handleUserMsg  │    │ Orchestrator    │    │ Gate         │       │
│ └────────────────┘    └─────────────────┘    └──────┬───────┘       │
│                                                     │               │
│  EARLY ACK（run_status=running）                    │               │
│  Heartbeat 启动（10s 间隔，仅进度感知）              │               │
│  无 24h deadline / 无 turn 超时                     │               │
│  LLM 失败 → 自动重试（指数退避）                     │               │
│  DB 超时 → 回滚 + 自动重试                           │               │
└─────────────────────────────────────────────┼───────────────────────┘
                                              │
┌─────────────────────────────────────────────┼───────────────────────┐
│ 阶段 3：LLM 响应                                     ▼               │
│ ┌──────────────┐    ┌─────────────────┐    ┌──────────────┐       │
│ │ BUILD +      │ → │ chatagent       │ → │ event.Bus    │       │
│ │ INTENT PASS  │    │ .RunTRPCUserTurn│    │ .Publish     │       │
│ └──────────────┘    └─────────────────┘    └──────┬───────┘       │
│                                                     │               │
│  无 LLM HTTP 超时（移除 30min）                      │               │
│  LLM 断开 → RetryTransport 自动重试（默认开启）      │               │
│  流式事件：reasoning_delta / text_delta / tool_call  │               │
│  WBPF：Critical 事件 WAL 持久化后才发布               │               │
└─────────────────────────────────────────────┼───────────────────────┘
                                              │
┌─────────────────────────────────────────────┼───────────────────────┐
│ 阶段 4：前端展示                                     ▼               │
│ ┌──────────────┐    ┌─────────────────┐    ┌──────────────┐       │
│ │ streamHandlers│ → │ ActivityTimeline│ → │ EventStream  │       │
│ │ 事件分发      │    │ + MessageStore  │    │ 渲染组件     │       │
│ └──────────────┘    └─────────────────┘    └──────────────┘       │
│                                                                     │
│  Activity-First 渲染（零推理）                                       │
│  pending-user 占位消息转换                                           │
│  无 turn-ack/first-byte/stall 超时（持续等待）                       │
│  VirtualScroller 动态加载 + 大消息折叠                               │
│  可观测性 Dashboard（任务计划/Team/Graph）                           │
└─────────────────────────────────────────────────────────────────────┘
```

### 3.2 阶段 1：UI 发送数据

#### 3.2.1 用户交互信息

| 用户动作 | 系统响应 | 实现状态 |
|---------|---------|---------|
| 输入文本 | 实时字符计数 + 附件预览 | ✅ `useChatComposerActions.ts` |
| 点击发送 | 创建 pending-user 占位 + WS 提交（**无超时**） | 🟡 需移除 dispatch 30s |
| 任务执行中发送 | 排队消息 + ChatPendingQueue UI 展示 | ✅ `useChatSender.ts:340-373` |
| 排队消息立即发送 | 中断当前 turn + 提升优先级 | ✅ `ChatPendingQueue.vue` interrupt 按钮 |
| 排队消息删除 | 从队列移除 | ✅ `ChatPendingQueue.vue` cancel 按钮 |
| 排队消息编辑 | inline 编辑内容 | ✅ `ChatPendingQueue.vue` edit 功能 |
| 重试失败消息 | 重置 status + 复用 pendingId 重发 | 🟡 `useChatSender.ts:321-338` 需修复 |

#### 3.2.2 发送路径（无超时版）

**WS 路径（主，同机部署唯一路径）**：
```
useChatSender.onSend()
  → sendUserContent(entityKind, content)
    → strategy.ensureSession(title)
    → createPlaceholderMessage(pendingUserId)
    → strategy.ensureStream(sessionId)
    → deps.sendChatViaWs(stream, payload)
      → transport.send(upstream)  // WS OPEN 时立即发送
    → 等待 run_status=running（无超时，持续等待）
    → 收到 run_status → 占位消息匹配替换
```

**HTTP fallback 路径（仅 WS 完全不可用时）**：
```
sendChatViaWs 抛出异常（WS 连接失败）
  → sendViaHttpFallback(...)
    → runtime.send()  // HTTP 命令通道，仅入队 + 返回 ACK
    → 不接收任何流式数据（WS 恢复后通过 afterRevision 回放）
    → markSendingDone()  // 立即标记完成
```

**排队路径（任务执行中）**：
```
isActiveRun() = true
  → enqueueDuringRun(sessionId, text, pendingUserId)
    → runtime.enqueue(sessionId, content)
    → ChatPendingQueue.vue 展示排队消息
    → 用户可：编辑 / 立即发送（interrupt）/ 删除（cancel）
    → turn 结束后自动消费队列（FIFO + priority）
```

#### 3.2.3 消息队列管理（需求 4 — 已完整实现）

**后端 API**（`internal/runtime/pending_queue.go`）：

| API | 签名 | 用途 |
|-----|------|------|
| `List` | `List(sessionID) []PendingMessage` | 列出队列 |
| `Enqueue` | `Enqueue(sessionID, content) string` | 入队 |
| `Remove` | `Remove(sessionID, entryID) bool` | **删除队列项** |
| `Update` | `Update(sessionID, entryID, content) bool` | 编辑内容 |
| `PromoteToFront` | `PromoteToFront(sessionID, pendingID) error` | **移到队首（立即发送）** |
| `SetPriority` | `SetPriority(sessionID, pendingID, priority) error` | 设置优先级 |
| `InterruptAndSendMessage` | `PromoteToFront + SetPriority(1) + runs.Cancel` | **中断当前 + 立即发送** |

**HTTP API 端点**（`api/kratos/chat/v1/chat.proto`）：

| 端点 | RPC | 用途 |
|------|-----|------|
| `GET /v1/chat/pending` | GetPendingMessages | 列出队列 |
| `POST /v1/chat/pending/cancel` | CancelPendingMessage | 删除队列项 |
| `POST /v1/chat/pending/update` | UpdatePendingMessage | 编辑内容 |
| `POST /v1/chat/pending/interrupt-and-send` | InterruptAndSendMessage | 立即发送 |

**前端 UI**（`web/src/components/chat/ChatPendingQueue.vue`）：

| 功能 | 按钮 | 事件 | 状态 |
|------|------|------|------|
| 查看 | — | — | ✅ |
| 编辑 | `check` icon | `update-pending` | ✅ |
| 删除 | `cancel` icon | `cancel-pending` | ✅ |
| 立即发送 | `send` icon | `interrupt-pending` | ✅ |

**队列持久化**：Snapshot 已启用（10s 周期，`pending_queue.json`，2h cutoff），但 Wire 注入时传空 dir（问题 7）。

#### 3.2.4 pending-user 占位消息生命周期（无超时版）

```
创建 → status='ok'
  ├─ WS ack 收到 run_status=running → 等待 mergeSessionMessages 匹配替换
  ├─ error envelope 到达 → status='failed'
  ├─ HTTP fallback 成功 → dropPendingUserRow（移除）
  ├─ enqueue 成功 → dropPendingUserRow（移除）
  └─ retryFailedMessage → status='ok' + 重发
  （移除：turn-ack 30s 超时 → status='failed'）
```

### 3.3 阶段 2：后端响应

#### 3.3.1 后端事件和消息

| 事件 | 时机 | 可靠性 | 用途 |
|------|------|--------|------|
| `run_status=running` | Turn 开始（EARLY ACK） | Important | 通知前端任务已启动 |
| `run_heartbeat` | 每 10s | Informational | 前端进度感知（**不触发超时**） |
| `context_usage` | Context 加载后 | Important | 前端展示 token 用量 |
| `intent_pass` | 意图识别完成 | Important | 编排意图通知 |
| `activity_start` | Activity 开始 | Important | 创建 thinking/reply/action 消息 |
| `activity_delta` | Activity 流式增量 | Informational | 更新 content/reasoning |
| `activity_done` | Activity 完成 | Important | finalize 消息 |
| `tool_call` / `tool_result` | 工具调用 | Critical (WBPF) | 工具执行结果 |
| `runner_completion` | Turn 完成 | Critical (WBPF) | finalize + reload |
| `error` | 错误发生 | Critical (WBPF) | 标记失败 + reload |
| `llm_retry` | LLM 重试时 | Important | **新增**：通知前端"正在重试" |
| `db_retry` | DB 重试时 | Informational | **新增**：通知前端"数据库重试" |

#### 3.3.2 Turn Orchestrator 流程（无超时版）

```
handleUserMessage (ws_message_handler.go:93)
  → goroutine: safego.Go(appctx.Ctx(), "ws-user-message", ...)
    → connCtx 派生 ctx（无超时，或仅连接级超时）
    → turnExecutor.ExecuteTurn(ctx, input)
      → TurnPipeline.Run
        → ChatOrchestrator.Execute
          → RunNativeAgentTurnWithOutcome
            → checkTurnAdmission  // 有 active run 则排队
            → runSingleAgentViaTRPC
              ├─ ADMISSION: 生成 runID
              ├─ EARLY ACK: SetRunStatus("running")
              ├─ 无 24h hard deadline（移除）
              ├─ Heartbeat: Start(ctx, runID, progress)  // 仅感知
              ├─ BUILD + INTENT PASS (并行 errgroup)
              ├─ PRE-PLANNING GATE (硬门控)
              ├─ EXECUTE: executeTurn → invokeLLMCall
              │    └─ LLM 失败 → RetryTransport 自动重试（默认开启）
              │    └─ 重试耗尽 → 推送 error 事件，turn 失败
              ├─ PERSIST: persistTurn
              │    └─ DB 超时 → 回滚 → 自动重试（新增）
              ├─ POST-PROCESS: postProcessTurn
              └─ defer: processPendingQueue  // 消费下一条排队消息
```

#### 3.3.3 排队消息后端处理

**入队**：
```
EnqueueUserMessage(sessionID, content, mergeFollowup)
  ├─ 框架级 enqueue（tool boundary 注入）：trpcrunner.EnqueueUserMessage
  └─ pending queue（FIFO，每会话最多 32 条，Snapshot 持久化）
```

**消费时机**：Turn 结束时（非 tool boundary）
```
processPendingQueue(sessionID)  // 改为迭代式（while loop）
  → while queue.HasPending():
      → DequeuePendingMessage
      → if HasActive: 重新入队（保持原位置和优先级）+ break
      → else: runSingleAgentViaTRPC（启动新 turn）
```

**立即插入**（用户点击"立即发送"）：
```
InterruptAndSendMessage(sessionID, pendingEntryID)
  → PromoteToFront  // 移到队首
  → SetPriority(1)  // 高优先级
  → runs.Cancel     // 取消当前 turn
  → turn defer 触发 processPendingQueue → 消费队首
```

### 3.4 阶段 3：LLM 响应

#### 3.4.1 LLM 调用链路（无超时 + 自动重试）

```
invokeLLMCall (chat_orchestrator_turn_phases.go:357)
  → buildUserMessage（含附件）
  → chatagent.RunTRPCUserTurnMsg  // 调用 LLM
    → RetryTransport.RoundTrip  // 默认开启重试
      ├─ 成功 → 返回响应
      ├─ HTTP 5xx / 429 / 网络错误 → 指数退避重试
      │    ├─ 第 1 次：1s
      │    ├─ 第 2 次：2s
      │    ├─ 第 3 次：4s
      │    ├─ 第 4 次：8s
      │    ├─ 第 5 次：16s
      │    ├─ 第 6 次+：30s（封顶）
      │    └─ 无限重试（直到用户取消或成功）
      └─ 每次重试推送 llm_retry 事件（前端显示"正在重试..."）
  → consumeTurnStream
    → event.WrapFrameworkEventsWithOtel  // OTel 包装
    → 事件发布: event.Bus.Publish
      → Infra.Publish (WBPF)
        ├─ Critical 事件: WAL.WriteBeforePublish → publishToBuses
        └─ 其他事件: 直接 publishToBuses
```

#### 3.4.2 超时层次（无超时版）

| 层级 | 旧值 | 新值 | 说明 |
|------|-----|------|------|
| 首字节超时 | 30s | **移除** | 慢模型不应被中断 |
| 业务级 turn 超时 | 10min（软） | **移除** | 任务持续运行 |
| LLM HTTP 超时 | 30min | **移除** | LLM 调用无上限，断开自动重试 |
| 24h hard deadline | 24h | **移除** | 任务无时间限制 |
| DB 事务超时 | 30s | **保留**（SQLite 需要） | 超时回滚 + 自动重试 |
| WS TurnTimeout | 5min | **移除** | WS turn 不超时 |
| Heartbeat | 10s | **保留** | 仅进度感知，不触发超时 |
| WS ping/pong | 30s/60s | **保留** | 连接健康检测 |

### 3.5 阶段 4：前端展示

#### 3.5.1 前端渲染链路

```
WS onmessage → ws-transport.ts
  → EnvelopeDispatcher.dispatch
    → bindStreamHandlers (streamHandlers.ts)
      ├─ run_status → 创建/移除 thinking placeholder
      ├─ activity_start → 按 kind 创建消息（thinking/reply/action/error）
      ├─ activity_delta → 更新 content/reasoning
      ├─ activity_done → finalize 消息
      ├─ runner_completion → finalize + reload
      ├─ error → 标记失败 + reload
      ├─ run_heartbeat → 更新进度显示（不触发超时）
      ├─ llm_retry → 显示"正在重试..."提示（新增）
      └─ tool_call/tool_result → 更新 action 消息

messageStore.messages (Pinia ref)
  → useConversationTimeline.conversationTurns (computed)
    ├─ AF 路径: buildAllConversationTurnsFromActivities（零推理）
    └─ Legacy 路径: buildLegacyConversationTurn（待移除）
      → ChatMessageList.vue
        → VirtualScroller（动态加载，新增）
          → ConversationTurn.vue
            → AgentWorkPanel.vue
              → EventStream.vue
                → ThinkingBlock（默认折叠，按需展开）
                → ActionBlock / ReplyBlock / ErrorBlock
```

#### 3.5.2 Activity-First 渲染

**AF 路径触发条件**：`hasAfData = afActivities.length > 0 || afRawRecords.length > 0`

**AF 路径优势**：
- 13 层前端推理全部消除
- 后端语义直推（activity.kind 直接给出）
- 树形嵌套原生支持（parentActivityId）
- reasoning_as_display 流式阶段解决

**ActivityKind 渲染映射**：

| ActivityKind | 渲染组件 | 说明 |
|-------------|---------|------|
| `task` | TaskBlock | 任务描述 |
| `thinking` | ThinkingBlock | reasoning 内容（流式 + **默认折叠**） |
| `action` | ActionBlock | 工具调用（含结果 + 耗时） |
| `reply` | ReplyBlock | Agent 回复（最终答案） |
| `sub_task_board` | SubTaskBoard | 子任务看板（递归嵌套） |
| `error` | ErrorBlock | 错误信息（含重试按钮） |
| `delegate` | TeamAssemblyCard | 团队委派 |
| `notice` | NoticeBlock | 系统通知 |
| `end` | EndMarker | 任务完成标记 |

#### 3.5.3 动态加载与折叠（需求 5）

**VirtualScroller**：
- 组件：`vue-virtual-scroller` 的 `DynamicScroller`（支持动态高度）
- 触发条件：消息数 > 100 时启用
- 当前问题：`useVirtualMessageList: computed(() => false)` 硬编码禁用

**大消息折叠**：
- ThinkingBlock：默认折叠（reasoning 内容可能很长）
- ActionBlock：工具结果超过 500 字符时折叠
- ReplyBlock：始终展开（最终答案）
- 用户点击展开/折叠，状态记忆到 sessionStorage

---

## 四、长任务目标实现（无时间限制）

### 4.1 设计目标

**用户指令**：完成用户指令，**没有时间限制**。

**核心原则**：
- 任务持续运行直到完成或用户取消
- LLM 断开 → 自动重试（无限次，指数退避）
- DB 超时 → 回滚 + 自动重试
- 进程崩溃 → CheckpointSaver 恢复
- 用户可随时：查看进度（heartbeat）/ 取消任务 / 插入新消息

### 4.2 长任务支持链路（无 deadline 版）

```
用户发送指令
  → Turn Orchestrator 启动 turn
    → 无 24h hard deadline（移除）
    → Heartbeat 每 10s 推送（前端进度感知，不触发超时）
    → CheckpointSaver 强制启用（崩溃恢复）
  → LLM 调用（无超时，断开自动重试）
  → 流式事件持续推送（无总时长限制）
  → LLM 断开 → RetryTransport 自动重试
    → 推送 llm_retry 事件 → 前端显示"正在重试...（第 N 次）"
    → 指数退避：1s/2s/4s/8s/16s/30s（封顶）
    → 无限重试，直到成功或用户取消
  → DB 超时 → 回滚 + 自动重试
    → 推送 db_retry 事件 → 前端显示"数据库重试中..."
  → 进程崩溃 → RecoveryWorker 恢复
    → 加载 checkpoint
    → ResumeDurableSessionRun
    → 复用 turn_id，WithDetachedCancel
  → 用户可主动升级 Durable 模式
    → EscalateToDurableByUser
    → 创建 durable checkpoint
    → 取消当前 runner
    → 后台继续执行
```

### 4.3 长任务阻断点与解决状态

| # | 阻断点 | 解决状态 | 解决方案 |
|---|--------|---------|---------|
| 1 | HTTP server timeout 600s | ✅ 已解决 | 改为 0s（不超时） |
| 2 | Turn 硬上限 2h | ✅ 已解决 | 提升到 24h → **本方案移除** |
| 3 | LLM 单调用 5min | 🟡 部分解决 | 提升到 30min → **本方案移除 + 自动重试** |
| 4 | DB 事务 30s 硬超时 | ✅ 已解决 | 改为可配置 → **本方案保留 + 自动重试** |
| 5 | 无通用 BackgroundJob Worker | ✅ 已解决 | Durable Worker + Recovery Worker |
| 6 | Interactive 阶段崩溃不可恢复 | ✅ 已解决 | CheckpointSaver 强制启用 |
| 7 | 无任务级心跳 | ✅ 已解决 | RunHeartbeatEmitter（10s） |
| 8 | WBPF 语义违规 | ✅ 已解决 | 修复 WBPF |
| 9 | 状态机被绕过 | ✅ 已解决 | GraphExecution FSM |
| 10 | SQLite 单写瓶颈 | 🔴 未解决 | Phase A Postgres 迁移（排除本文范围） |
| 11 | **24h hard deadline 限制** | 🔴 新增 | **移除 24h deadline**（本方案） |
| 12 | **LLM 断开无自动重试** | 🔴 新增 | **RetryTransport 默认开启 + 无限重试**（本方案） |
| 13 | **DB 超时无自动重试** | 🔴 新增 | **DB 操作重试包装器**（本方案） |

---

## 五、用户体验提升方案

### 5.1 已实现的体验优化

| 体验点 | 实现状态 | 位置 |
|--------|---------|------|
| ErrorBlock 内联重试 | ✅ | `ErrorBlock.vue` + `errorCodeHints.ts`（6 种 action） |
| WS 断连快速检测 | ✅ | 30s stale 检测 + recover 按钮 |
| WS 断连 UI 反馈 | ✅ | `wsReplaying` / `isStale` / `sessionLoading` banner |
| 长任务进度展示 | ✅ | heartbeat 10s + elapsed timer |
| **排队消息管理** | ✅ | `ChatPendingQueue.vue`（查看/编辑/取消/立即发送） |
| pending-user 占位 | ✅ | 即时视觉反馈 + 匹配替换 |
| 编排时间线视图 | ✅ | Plan→Allocate→Orchestrate→Delivery 四阶段 |
| i18n CI 检查 | ✅ | baseline 增量比对（458 文件技术债务） |
| **可观测性后端** | ✅ | OTel + Prometheus 30+ 指标 + flow_log 结构化查询 |
| **Team 执行 API** | ✅ | Observatory + Timeline + Summary |
| **Graph 可视化 API** | ✅ | VisualizeGraph（nodes + edges） |

### 5.2 待提升的体验点（基于 9 条需求）

| 体验点 | 当前问题 | 根本性方案 |
|--------|---------|-----------|
| **任务无超时感知** | stall 警告 180s 后用户焦虑 | 分级提示：30s"思考中" / 90s"仍在工作" / 180s"似乎停滞，可继续等待或取消"（**不中断**） |
| **LLM 重试无反馈** | LLM 断开后用户无感知 | 推送 `llm_retry` 事件 + 前端显示"正在重试...（第 N 次）" |
| **长会话滚动卡顿** | 无虚拟滚动，1000+ 消息全量渲染 | VirtualScroller（DynamicScroller 支持动态高度） |
| **大消息撑满屏幕** | ThinkingBlock 全展开 | 大消息默认折叠 + 按需展开 + 状态记忆 |
| **主题无跟随系统** | 需手动切换昼/夜 | ThemeManager auto 模式（prefers-color-scheme） |
| **HTTP fallback 消息混乱** | 双通道数据流竞态 | 通道职责分离（HTTP 仅命令，WS 仅数据） |
| **实时 Activity 不渲染** | AF 路径实时事件未接入 | 补充 onActivityEnvelope 回调 |
| **可观测性无前端 dashboard** | 后端 API 齐全但前端无展示 | 新增 Observability Dashboard（任务计划/Team/Graph） |
| **移动端折叠逻辑冗余** | `<1024px` 折叠（需求 6 不考虑移动端） | 移除移动端折叠逻辑，专注桌面端 |
| **WS 重连 10 次后放弃** | 同机部署不应放弃 | 无限重连（同机部署简化） |

### 5.3 可观测性 Dashboard 设计（需求 9）

**后端 API 就绪状态**：

| 可观测领域 | 后端 API | 前端 UI | 缺口 |
|-----------|---------|---------|------|
| 任务计划（Plan） | 🟡 仅 ConfirmPlan | ❌ 无 | 需补 List/Get Plan API + 前端展示 |
| Team 执行状态 | ✅ Observatory/Timeline/Summary | ❌ 无 | 需补前端 dashboard |
| Graph 执行状态 | ✅ Visualize/Get/List | ❌ 无 | 需补前端可视化 |
| LLM 调用 | ✅ Prometheus metrics | ❌ 无 | 需补 metrics dashboard |
| 系统健康 | ✅ /metrics 端点 | ❌ 无 | 需补系统监控面板 |

**前端 Dashboard 组件设计**：

```
web/src/features/observability/
├── ObservabilityDashboard.vue      # 主面板（Tab 切换）
├── TaskPlanView.vue                # 任务计划视图
│   ├─ PlanCard.vue                 # 单个计划卡片
│   └─ PlanStepList.vue             # 计划步骤列表
├── TeamRunView.vue                 # Team 执行视图
│   ├─ TeamMemberCard.vue           # 成员状态卡片
│   └─ TeamTimeline.vue             # 执行时间线
├── GraphExecutionView.vue          # Graph 执行视图
│   ├─ GraphVisualizer.vue          # 图可视化（nodes + edges）
│   └── NodeStatusList.vue          # 节点状态列表
├── MetricsPanel.vue                # 系统指标面板
│   ├─ LlmMetrics.vue               # LLM 调用指标
│   ├─ ToolMetrics.vue              # 工具调用指标
│   └─ EventBusMetrics.vue          # 事件总线指标
└── FlowLogViewer.vue               # 结构化日志查看器
```

**数据流**：
```
ObservabilityDashboard
  ├─ TaskPlanView → GET /v1/chat/plans?session_id=xxx（需新增 API）
  ├─ TeamRunView → GET /v1/team/runs/{id}/observatory
  ├─ GraphExecutionView → GET /v1/graph/executions/{id} + VisualizeGraph
  ├─ MetricsPanel → GET /metrics（Prometheus 格式解析）
  └─ FlowLogViewer → GET /v1/flowlog/list（需确认 API）
```

---

## 六、问题清单与解决方案

### 6.1 已解决问题（12 项）

> 综合升级方案（30/30 验收项）已解决的问题，不在本文修复范围。

| # | 问题 | 解决方案 | 验收 |
|---|------|---------|------|
| 1 | HTTP server timeout 600s | 改为 0s | ✅ |
| 2 | Turn 硬上限 2h | 提升到 24h（本方案进一步移除） | ✅ |
| 3 | LLM 单调用 5min | 提升到 30min（本方案进一步移除+重试） | ✅ |
| 4 | DB 事务 30s 硬超时 | 改为可配置 | ✅ |
| 5 | Interactive 阶段崩溃不可恢复 | CheckpointSaver 强制启用 | ✅ |
| 6 | 无任务级心跳 | RunHeartbeatEmitter | ✅ |
| 7 | WBPF 语义违规 | 修复 WBPF | ✅ |
| 8 | 状态机被绕过 | GraphExecution FSM | ✅ |
| 9 | 规划非强制 | 预规划门控（硬门控） | ✅ |
| 10 | 无动态 Agent 创建 | AgentFactory | ✅ |
| 11 | Graph 不可重构 | RuntimeReplanner | ✅ |
| 12 | 13 层前端推理（AF 路径） | Activity-First 重构 | ✅（AF 路径） |

### 6.2 未实施问题（18 项）

> 按消息链阶段分组，每个问题含根本性方案。

#### 6.2.1 阶段 1（UI 发送）问题

**问题 1：HTTP fallback 成功后未 markSendingDone** 🔴 阻断

| 项 | 内容 |
|----|------|
| **位置** | `useChatSender.ts:562-585` |
| **问题** | HTTP fallback 成功路径中 `markSendingDone()` 未调用，`sending.value` 保持 true 长达 30s，导致 `inputDisabled=true` 用户无法继续输入 |
| **根本性方案** | 通道职责分离（方案 D）：HTTP 仅命令通道，不承担数据传输，成功后立即 markSendingDone |
| **工作量** | 临时修复 0.5 人天 / 根本性方案 7 人天 |

**问题 2：loadMessages 失败误标记消息为 failed** 🔴 阻断

| 项 | 内容 |
|----|------|
| **位置** | `useChatSender.ts:576-584` |
| **问题** | HTTP fallback 中 `sendViaHttpFallback` 已成功（消息已到后端），但后续 `loadMessages` 失败时进入 catch 标记 pending 为 failed，用户看到"发送失败"但消息实际已发送，重试导致重复 |
| **根本性方案** | 通道职责分离（方案 D）：HTTP 不返回数据，无需 loadMessages |
| **工作量** | 临时修复 0.5 人天 / 根本性方案含在问题 1 中 |

**问题 3：WS pendingQueue 无界增长且重连失败后消息丢失** 🔴 阻断

| 项 | 内容 |
|----|------|
| **位置** | `ws-transport.ts:219-228` 和 `212-217` |
| **问题** | `pendingQueue` 无大小限制；10 次重连失败后 `pendingQueue` 不清空，消息永远无法发送；`flushPendingQueue` 无错误处理 |
| **根本性方案** | 同机部署改为无限重连 + `pendingQueue` 最大长度限制（100）+ `flushPendingQueue` 添加 try-catch |
| **工作量** | 1 人天 |

**问题 4：retryFailedMessage 在活跃 run 期间行为错误** 🟡 建议

| 项 | 内容 |
|----|------|
| **位置** | `useChatSender.ts:321-338` |
| **问题** | 用户意图是"重新发送失败消息"，但 `sendUserContent` 内部检查 `isActiveRun()`，若活跃则进入 `enqueueDuringRun` 路径，移除正在重试的消息，用户看不到重试反馈 |
| **根本性方案** | `retryFailedMessage` 检查 `isActiveRun()`，若活跃则提示"当前有任务执行中，已加入队列"并展示在 ChatPendingQueue |
| **工作量** | 0.5 人天 |

**问题 5：两套 enqueue 路径行为不一致** 🟡 建议

| 项 | 内容 |
|----|------|
| **位置** | `useChatSender.enqueueDuringRun` (line 340) vs `useChatWorkspace.onEnqueueWhileRunning` (line 691) |
| **问题** | 路径 A 创建占位消息（出现又消失），路径 B 不创建占位（无视觉反馈），UX 不一致 |
| **根本性方案** | 统一为一种路径，所有排队消息都展示在 ChatPendingQueue |
| **工作量** | 0.5 人天 |

#### 6.2.2 阶段 2（后端响应）问题

**问题 6：processPendingQueue 递归调用风险** 🔴 阻断

| 项 | 内容 |
|----|------|
| **位置** | `chat_orchestrator_turn_dispatch.go:154` + `chat_orchestrator_turn.go:457` |
| **问题** | `processPendingQueue` 调用 `runSingleAgentViaTRPC`，后者的 defer 又调用 `processPendingQueue`，形成递归 goroutine 链，无最大递归深度限制 |
| **根本性方案** | 改为迭代式消费（while loop + 深度计数器） |
| **工作量** | 1 人天 |

**问题 7：Pending Queue 未持久化** 🔴 阻断

| 项 | 内容 |
|----|------|
| **位置** | `cmd/admin/wire.go:425` |
| **问题** | `providePendingMessageQueue` 传入空 dir，`snapshot=false`，进程重启后 pending queue 消息全部丢失，与无时间限制长任务承诺不一致 |
| **根本性方案** | 启用持久化：`NewPendingMessageQueueWithDirAndLogger(dataDir, lg)` + 配置 `pending_queue_snapshot_dir` |
| **工作量** | 1 人天 |

**问题 8：processPendingQueue 竞态条件** 🟡 建议

| 项 | 内容 |
|----|------|
| **位置** | `chat_orchestrator_turn_dispatch.go:155-170` |
| **问题** | `DequeuePendingMessage` 和 re-lock 之间存在窗口，重新入队时丢失原来的 priority 和位置（追加到队尾） |
| **根本性方案** | 在 lock 内完成 dequeue + 检查 + 决策，或重新入队时保持原位置和优先级 |
| **工作量** | 1 人天 |

**问题 9：24h hard deadline 限制长任务** 🔴 阻断（新增）

| 项 | 内容 |
|----|------|
| **位置** | `internal/service/chat_orchestrator_turn.go:313-323` |
| **问题** | `longTaskHardDeadline = 24 * time.Hour` 硬编码，任务超过 24h 被强制取消，与"无时间限制"需求矛盾 |
| **根本性方案** | 移除 24h deadline（方案 A）：删除 `context.WithTimeout(ctx, longTaskHardDeadline)` 代码块，任务持续运行直到完成或用户取消 |
| **工作量** | 0.5 人天 |

**问题 10：LLM 调用无默认重试** 🔴 阻断（新增）

| 项 | 内容 |
|----|------|
| **位置** | `internal/provider/trpc_llm.go:301-311` + `internal/provider/retry_transport.go` |
| **问题** | `RetryTransport` 存在但仅在模型 config_json 显式配置 `Retry.MaxAttempts > 0` 时启用，默认无重试，LLM 断开直接导致 turn 失败 |
| **根本性方案** | 默认开启重试（方案 A）：`MaxAttempts = -1`（无限重试）+ 指数退避（1s/2s/4s/8s/16s/30s 封顶）+ 每次重试推送 `llm_retry` 事件 |
| **工作量** | 2 人天 |

**问题 11：DB 事务超时无重试** 🟡 建议（新增）

| 项 | 内容 |
|----|------|
| **位置** | `internal/data/data.go:163-219` + `internal/data/tx.go:17-100` |
| **问题** | DB 事务 30s 硬超时，超时后回滚返回错误，无自动重试，长任务中 DB 超时直接失败 |
| **根本性方案** | DB 操作重试包装器（方案 A）：`ExecInTxWithRetry(ctx, fn, maxRetries=3, backoff=1s/2s/4s)`，仅对 `CodeInternal` 和 busy 错误重试 |
| **工作量** | 2 人天 |

**问题 12：RunRegistry TOCTOU 风险** 🔴 阻断

| 项 | 内容 |
|----|------|
| **位置** | `internal/runtime/run_registry.go:129-141` `StoreRunner` |
| **问题** | `StoreRunner` 先 `load` 再 `store`，存在 TOCTOU 风险，两个并发 goroutine 可能同时 load 到旧值然后各自 store |
| **根本性方案** | 用 ManagedMap（内置锁 + 原子操作）替代裸 `sync.Map`，`LoadOrStore` 替代 `load+store`（方案 A） |
| **工作量** | 1 人天 |

**问题 13：前端 7 个超时模型需移除** 🔴 阻断（新增）

| 项 | 内容 |
|----|------|
| **位置** | `web/src/features/constants/timeouts.ts` |
| **问题** | 7 个独立超时（dispatch 30s / turn-ack 30s / first-byte 90s / stall 180s / run-stale 30s / heartbeat 25s / stream 10min），与"无超时"需求矛盾 |
| **根本性方案** | 移除任务级超时（方案 A）：删除 dispatch/turn-ack/first-byte/stream 超时常量；stall 改为通知（不中断）；保留 heartbeat（连接健康） |
| **工作量** | 1.5 人天 |

#### 6.2.3 阶段 3（LLM 响应）问题

**问题 14：WBPF Post-Publish Failure 幂等性隐式契约** 🟡 建议

| 项 | 内容 |
|----|------|
| **位置** | `internal/event/infra.go:167-180` |
| **问题** | Post-publish failure 时事件已发布但 mark failed，重启时 Recover 可能重放，要求订阅者幂等，但缺少统一保证机制 |
| **根本性方案** | 引入 EventStoreExistChecker 统一应用，订阅者通过 event_id 去重 |
| **工作量** | 2 人天 |

#### 6.2.4 阶段 4（前端展示）问题

**问题 15：实时 Activity 事件未接入 useActivityTimeline** 🔴 阻断

| 项 | 内容 |
|----|------|
| **位置** | `useChatStreamManager.ts:229-260` + `useChatInboundSync.ts:379-383` |
| **问题** | `bindStreamHandlers` 未设置 `ctx.onActivityEnvelope`，当前 session 的实时 activity 事件不更新 `useActivityTimeline`，实时新 turn 走 Legacy 路径显示空内容 |
| **根本性方案** | 补充 `onActivityEnvelope` 回调，将当前 session 的 activity 事件转发到 `activityTimeline.handleActivityStart/Delta/Done/ChildStart` |
| **工作量** | 1 人天 |

**问题 16：AF 与 Legacy 双路径技术债** 🔴 阻断

| 项 | 内容 |
|----|------|
| **位置** | `useConversationTimeline.ts:94-131`、`mergeSessionMessages.ts:210-213` |
| **问题** | AF 与 Legacy 路径并存，根据 `hasAfData`/`hasSnapshots` 切换，两条路径行为不一致，bug 修复易遗漏 |
| **根本性方案** | 完成 AF 迁移（方案 B1）：验证 AF 覆盖所有 Legacy 场景 → 移除 Legacy 函数 → 移除切换逻辑 |
| **工作量** | 5 人天 |

**问题 17：HTTP/WS 双通道职责不清** 🔴 阻断

| 项 | 内容 |
|----|------|
| **位置** | `useChatSender.ts:552-585`、后端 HTTP 消息接口 |
| **问题** | HTTP 和 WS 都承担数据传输职责，WS 失败回退 HTTP 时双通道数据流导致竞态、重复、占位残留 |
| **根本性方案** | 通道职责分离（方案 D）：HTTP 仅命令通道（fire-and-ack），WS 唯一数据通道 |
| **工作量** | 7 人天 |

**问题 18：消息列表无虚拟滚动 + 大消息不折叠** 🟡 建议

| 项 | 内容 |
|----|------|
| **位置** | `ChatMessageList.vue:29-48` + `ChatMessagePanel.vue:547-548` + ThinkingBlock/ActionBlock |
| **问题** | `useVirtualMessageList: computed(() => false)` 硬编码禁用虚拟滚动；ThinkingBlock 默认展开，长 reasoning 撑满屏幕 |
| **根本性方案** | 启用 VirtualScroller（DynamicScroller）+ ThinkingBlock/ActionBlock 默认折叠 + 按需展开 + 状态记忆（方案 F） |
| **工作量** | 6 人天 |

**问题 19：可观测性前端 Dashboard 缺失** 🟡 建议（新增）

| 项 | 内容 |
|----|------|
| **位置** | 前端无 observability 组件 |
| **问题** | 后端 API 齐全（Team Observatory / Graph Visualize / Prometheus metrics / flow_log），但前端无展示组件，用户无法查看任务计划、Team 执行状态、Graph 可视化 |
| **根本性方案** | 新增 Observability Dashboard（方案 E）：TaskPlanView + TeamRunView + GraphExecutionView + MetricsPanel + FlowLogViewer |
| **工作量** | 12 人天 |

**问题 20：Task Plan 查询 API 缺口** 🟡 建议（新增）

| 项 | 内容 |
|----|------|
| **位置** | `api/kratos/chat/v1/chat.proto:284-293` |
| **问题** | 仅暴露 `ConfirmPlan` RPC，未暴露 `ListPlans` / `GetPlan` 端点，前端无法查询任务计划 |
| **根本性方案** | 新增 `ListPlans` / `GetPlan` RPC + Service 实现（方案 E） |
| **工作量** | 2 人天 |

**问题 21：移动端折叠逻辑冗余** 🟢 清理（新增）

| 项 | 内容 |
|----|------|
| **位置** | 前端 `<1024px` 响应式逻辑 |
| **问题** | 需求 6 明确不考虑移动端，但代码中存在大量 `<1024px` 折叠逻辑，增加维护成本 |
| **根本性方案** | 移除移动端折叠逻辑，专注桌面端（≥1024px）体验 |
| **工作量** | 1 人天 |

---

## 七、根本性方案

### 7.1 方案 A：无超时重试框架（22 人天）

**覆盖问题**：问题 9（24h deadline）、问题 10（LLM 重试）、问题 11（DB 重试）、问题 12（TOCTOU）、问题 13（前端超时）+ 问题 6（递归）+ 问题 7（持久化）

**核心设计**：
```
internal/runtime/lifecycle/
├── manager.go          # LifecycleManager（统一注册/销毁）
├── goroutine_pool.go   # GoroutinePool（Go + GoBackground 多模式）
├── managed_map.go      # ManagedMap（带 TTL 或终态清理）
└── dead_letter.go      # DeadLetterQueue（内存缓冲 + DB 持久化）

internal/provider/
├── retry_transport.go      # 增强：默认开启 + 无限重试 + 事件推送
└── retry_policy.go         # 新增：RetryPolicy（指数退避 + 封顶）

internal/data/
└── tx_retry.go             # 新增：ExecInTxWithRetry（DB 操作重试包装）

internal/service/
└── chat_orchestrator_turn.go  # 修改：移除 24h deadline
```

**关键点**：
1. **移除 24h deadline**：删除 `chat_orchestrator_turn.go:313-323` 的 `context.WithTimeout(ctx, longTaskHardDeadline)` 代码块
2. **LLM 默认重试**：`RetryTransport` 默认 `MaxAttempts = -1`（无限），指数退避 1s/2s/4s/8s/16s/30s（封顶），每次重试推送 `llm_retry` 事件
3. **DB 重试包装**：`ExecInTxWithRetry(ctx, fn, maxRetries=3, backoff)`，仅对 `CodeInternal` 和 busy 错误重试
4. **前端超时移除**：删除 `timeouts.ts` 中的 dispatch/turn-ack/first-byte/stream 超时常量；stall 改为通知（不中断）
5. **GoroutinePool**：支持多模式 `Go(ctx, ...)` 请求级 + `GoBackground(name, ...)` 进程级
6. **ManagedMap**：替代裸 `sync.Map`/`map`，原子操作根治 TOCTOU
7. **pending queue 持久化**：`NewPendingMessageQueueWithDirAndLogger(dataDir, lg)`
8. **processPendingQueue 迭代式**：while loop + 深度计数器

### 7.2 方案 B：前端架构收敛与体验统一（29 人天）

**覆盖问题**：问题 15（实时 AF）、问题 16（双路径）、问题 18（虚拟滚动+折叠）+ 5 个其他

**核心设计**：
```
web/src/realtime/
├── command_channel.ts       # HTTP 命令通道（仅发送，返回 ACK）
├── data_channel.ts          # WS 数据通道（唯一数据流）
├── event_replay.ts          # 事件回放（afterRevision 增量同步）
├── timeout_model.ts         # 连接健康模型（仅 ping/pong + heartbeat）
└── reconnect_strategy.ts    # 重连策略（同机部署：无限重连）

web/src/features/chat/
├── conversation_timeline_af.ts  # AF 路径（唯一真相源）
└── legacy/                      # 待删除的 Legacy 路径

web/src/components/common/
├── VirtualScroller.vue      # 虚拟滚动组件（DynamicScroller）
└── ThemeManager.ts          # 主题管理（含 auto 模式）

web/src/components/chat/
├── ThinkingBlock.vue        # 修改：默认折叠 + 按需展开
└── ActionBlock.vue          # 修改：长结果折叠
```

**关键点**：
- 补充 `onActivityEnvelope` 回调，实时 AF 路径生效
- 完成 AF 迁移，移除 Legacy 路径（灰度发布 + 监控）
- VirtualScroller 用 DynamicScroller 支持动态高度
- ThinkingBlock/ActionBlock 默认折叠，状态记忆到 sessionStorage
- 超时统一为连接健康模型（仅 ping/pong + heartbeat，无任务级超时）

### 7.3 方案 C：记忆系统持久化与可靠性（9 人天）

**覆盖问题**：3 个记忆系统简化方案待生产化

**核心设计**：
```
internal/cronrunner/
├── job_framework.go        # 统一 Job 框架（重试 + 熔断 + 死信）
├── jobs/
│   ├── memory_ebbinghaus_decay.go  # 增强：DB 读写
│   ├── memory_sleep_time.go        # 增强：重试 + 死信
│   └── memory_link_evolution.go    # 增强：事务包裹
└── dead_letter_repo.go     # 死信持久化（与方案 A 统一）
```

### 7.4 方案 D：通道职责分离（含在方案 B 中，7 人天）

**覆盖问题**：问题 1（markSendingDone）、问题 2（loadMessages 误标记）、问题 17（双通道）

**核心设计**：

```
前端：
┌─────────────────────────────────────────────────┐
│  useChatSender                                  │
│  ├─ sendViaWS(message) → WS 提交 + 等待 ack     │
│  └─ sendViaHTTP(message) → 仅提交，返回 ACK     │
│      └─ 返回 {messageId, turnId, status:"queued"}│
│      └─ 不接收任何流式数据                       │
│      └─ markSendingDone() 立即调用              │
│                                                  │
│  所有消息/状态/流式 → WS 数据通道推送            │
│  WS 断连 → 事件持久化 → 重连后 afterRevision 回放│
│  同机部署 → 无限重连（不放弃）                   │
└─────────────────────────────────────────────────┘

后端：
┌─────────────────────────────────────────────────┐
│  HTTP /messages（命令通道）                     │
│  └─ 仅入队 + 返回 {messageId, turnId}           │
│      不启动 turn，不返回流                       │
│                                                  │
│  WS handleUserMessage（命令通道 + 数据通道）     │
│  └─ 入队 + 启动 turn + 流式推送                 │
│                                                  │
│  Turn Orchestrator                              │
│  └─ 无论消息来自 HTTP 还是 WS，                 │
│      turn 执行结果都通过 WS 推送                │
│  └─ WS 未连接时，事件持久化到 EventStore        │
└─────────────────────────────────────────────────┘
```

**关键设计点**：
1. HTTP 接口语义改变：POST /messages → 仅入队 + 返回 ACK，不启动 turn，不返回流
2. WS 作为唯一数据通道：所有事件都通过 WS 推送
3. 统一入队机制：HTTP 和 WS 都放入同一个 pending queue
4. WS 断连处理：turn 继续执行（durable），事件持久化到 EventStore，重连后 `afterRevision` 回放
5. pending-user 转换：由 WS 推送的 `message.persisted` 事件自动驱动，无需手动 hydrate
6. 同机部署简化：WS 重连无次数上限，即时重连

### 7.5 方案 E：可观测性 Dashboard（14 人天）

**覆盖问题**：问题 19（前端 Dashboard）、问题 20（Task Plan API）

**核心设计**：
```
后端 API 补充：
api/kratos/chat/v1/chat.proto
├── rpc ListPlans(ListPlansRequest) returns (ListPlansResponse)  # 新增
└── rpc GetPlan(GetPlanRequest) returns (GetPlanResponse)        # 新增

internal/service/
└── chat_plan_query.go    # 新增：ListPlans + GetPlan 实现

前端 Dashboard：
web/src/features/observability/
├── ObservabilityDashboard.vue      # 主面板（Tab 切换）
├── TaskPlanView.vue                # 任务计划视图
├── TeamRunView.vue                 # Team 执行视图
├── GraphExecutionView.vue          # Graph 执行视图
├── MetricsPanel.vue                # 系统指标面板
└── FlowLogViewer.vue               # 结构化日志查看器
```

**数据源映射**：

| Dashboard 组件 | 后端 API | 数据内容 |
|---------------|---------|---------|
| TaskPlanView | `GET /v1/chat/plans`（新增） | 计划步骤、复杂度、策略、状态 |
| TeamRunView | `GET /v1/team/runs/{id}/observatory` | 成员状态、token 用量、执行时间 |
| GraphExecutionView | `GET /v1/graph/executions/{id}` + `VisualizeGraph` | 节点状态、边关系、当前节点 |
| MetricsPanel | `GET /metrics`（Prometheus 解析） | TTFT、turn 时长、事件总线、工具调用 |
| FlowLogViewer | `GET /v1/flowlog/list` | 按 TraceID/SessionID/RunID 查询日志 |

**Graph 可视化**：
- 使用 `vis-network` 或 `d3-graphviz` 渲染 nodes + edges
- 节点颜色映射状态：running（蓝）/ completed（绿）/ failed（红）/ waiting_human（黄）
- 点击节点展开详情（输入/输出状态、错误信息）

### 7.6 方案 F：动态加载与折叠（6 人天，含在方案 B 中）

**覆盖问题**：问题 18（虚拟滚动 + 折叠）

**核心设计**：
```
web/src/components/chat/
├── ChatMessageList.vue          # 修改：启用 VirtualScroller
├── VirtualScroller.vue          # 新增：DynamicScroller 封装
├── ThinkingBlock.vue            # 修改：默认折叠 + 展开记忆
└── ActionBlock.vue              # 修改：长结果折叠

web/src/composables/
└── useCollapseState.ts          # 新增：折叠状态管理（sessionStorage）
```

**关键点**：
- VirtualScroller：消息数 > 100 时启用，DynamicScroller 支持动态高度
- ThinkingBlock：默认折叠（reasoning 可能很长），点击展开，状态记忆
- ActionBlock：工具结果 > 500 字符时折叠，点击展开
- 状态记忆：折叠/展开状态存入 sessionStorage，刷新后恢复

---

## 八、实施建议

### 8.1 实施顺序

```
阶段 1（P0，立即）：8 个阻断项
  ├─ 问题 9 移除 24h deadline（0.5 人天）— 最高 ROI
  ├─ 问题 10 LLM 默认重试（2 人天）— 核心需求
  ├─ 问题 13 前端超时移除（1.5 人天）— 核心需求
  ├─ 问题 15 实时 AF 接入（1 人天）
  ├─ 问题 1+2 HTTP fallback 临时修复（1 人天）
  ├─ 问题 3 WS pendingQueue 保护（1 人天）
  ├─ 问题 6 processPendingQueue 递归（1 人天）
  └─ 问题 7 Pending Queue 持久化（1 人天）
  → 9 人天

阶段 2（P1，本迭代）：通道分离 + 可观测性 + DB 重试
  ├─ 问题 17 通道职责分离（7 人天）— 含问题 1+2 根本性解决
  ├─ 问题 19 可观测性 Dashboard（12 人天）— 需求 9
  ├─ 问题 20 Task Plan API（2 人天）— 需求 9
  ├─ 问题 11 DB 重试包装（2 人天）
  ├─ 问题 12 RunRegistry TOCTOU（1 人天）
  └─ 方案 C 记忆生产化（4.5 人天）
  → 28.5 人天

阶段 3（P2，下迭代）：体验与性能
  ├─ 问题 16 AF/Legacy 双路径收敛（5 人天）
  ├─ 问题 18 虚拟滚动 + 折叠（6 人天）— 需求 5
  ├─ 问题 8 竞态条件（1 人天）
  ├─ 问题 4 retryFailedMessage（0.5 人天）
  ├─ 问题 5 enqueue 路径统一（0.5 人天）
  ├─ 问题 14 WBPF 幂等性（2 人天）
  └─ 问题 21 移动端逻辑清理（1 人天）— 需求 6
  → 16 人天

阶段 4（P3，机会性）：i18n baseline 持续清理
  → 持续
```

### 8.2 工作量汇总

| 方案 | 工作量 | 覆盖问题 |
|------|--------|---------|
| 方案 A（无超时重试） | 22 人天 | 7 |
| 方案 B（前端收敛） | 29 人天 | 9 |
| 方案 C（记忆生产化） | 9 人天 | 3 |
| 方案 D（通道分离） | 含在 B 中 | 3 |
| 方案 E（可观测性） | 14 人天 | 2 |
| 方案 F（动态加载） | 含在 B 中 | 1 |
| 临时修复（阶段 1） | 9 人天 | 8 |
| **合计** | **82 人天** | **18+12 已解决** |

### 8.3 系统级风险防范

| 风险 | 缓解措施 |
|------|---------|
| 无超时导致资源泄漏 | GoroutinePool 统一管理 + 用户取消 + CheckpointSaver 崩溃恢复 |
| LLM 无限重试导致成本失控 | 每次重试推送事件 + 前端显示重试次数 + 用户可随时取消 |
| DB 无限重试导致死锁 | DB 重试上限 3 次 + 仅对 busy/internal 错误重试 |
| 改造期间稳定性下降 | 分阶段实施 + 每阶段独立验证 + 灰度发布 |
| 回归测试覆盖不足 | 改造前建立性能基准 + 关键路径 E2E 测试 + `goleak` 检测 |
| AF 收敛遗漏 Legacy 场景 | 灰度发布 + 监控 Legacy fallback 触发次数 |
| 通道分离接口变更 | 版本化 + 双写期 + 灰度 |
| 文档同步滞后 | 每个 PR 必须同步文档（DOC-SYNC-1） |

---

## 九、结论

### 9.1 需求达成评估

| 需求 | 达成状态 | 关键方案 |
|------|---------|---------|
| 1. 发送后无超时持续连接 | 🟡 方案就绪 | 移除 dispatch/turn-ack 超时（方案 A） |
| 2. 任务无超时 + 自动重试 | 🟡 方案就绪 | 移除 24h deadline + LLM/DB 自动重试（方案 A） |
| 3. 通信设计合理 | 🟡 方案就绪 | 通道职责分离（方案 D） |
| 4. 队列管理（排队/立即发送/删除） | ✅ 已实现 | 后端 API + 前端 UI 完整 |
| 5. 动态加载大折叠消息 | 🟡 方案就绪 | VirtualScroller + 折叠（方案 F） |
| 6. 不考虑移动端 | 🟡 待清理 | 移除移动端逻辑（问题 21） |
| 7. 同机部署 | ✅ 已满足 | WS 本地连接，简化重连 |
| 8. 不考虑多租户 | ✅ 已满足 | 无租户隔离 |
| 9. 可观测性 | 🟡 后端就绪 | 后端 API 齐全 + 前端 Dashboard（方案 E） |

### 9.2 消息链完整性评估

| 阶段 | 完整性 | 关键问题 |
|------|--------|---------|
| 阶段 1 UI 发送 | 🟡 85% | HTTP fallback 状态管理不完整（问题 1+2） |
| 阶段 2 后端响应 | 🟡 70% | 24h deadline + LLM 无重试 + DB 无重试（问题 9+10+11） |
| 阶段 3 LLM 响应 | 🟡 75% | LLM 无默认重试（问题 10） |
| 阶段 4 前端展示 | 🟡 70% | 实时 AF 未接入 + 双通道竞态 + 无虚拟滚动（问题 15+17+18） |

### 9.3 长任务目标评估

**当前能力**：能支撑"用户主动转 Durable 后的长任务"，但存在以下限制：
- 24h hard deadline 限制（问题 9）→ **本方案移除**
- LLM 单调用无重试，断开即失败（问题 10）→ **本方案默认重试**
- DB 超时无重试（问题 11）→ **本方案增加重试**
- pending queue 进程重启丢失（问题 7）→ **本方案启用持久化**
- SQLite 单写瓶颈（Phase A 未实施，排除本文范围）

**目标达成路径**：
1. 修复问题 9+10+11（无超时 + 自动重试）→ 长任务可靠性提升
2. 实施方案 D（通道分离）→ 消除双通道竞态，长任务期间用户可继续交互
3. 实施方案 E（可观测性）→ 用户可查看任务进度、Team 状态、Graph 执行
4. 实施 Phase A（Postgres 迁移，排除本文）→ 突破 SQLite 单写瓶颈

### 9.4 用户体验评估

**当前水平**：🟡 7/10
- 实时性良好（heartbeat 10s）
- 错误处理完整（6 种 action + 17 种错误码）
- 队列管理完整（查看/编辑/取消/立即发送）
- 但存在 7 个体验痛点（见 §5.2）

**目标水平**：🟢 9/10
- 修复 7 个体验痛点
- 无超时 + 自动重试（用户无焦虑）
- 通道分离消除双通道竞态
- VirtualScroller + 折叠提升长会话体验
- 可观测性 Dashboard 提升透明度

### 9.5 最终建议

**优先修复阶段 1 的 8 个阻断项**（9 人天），立即实现"无超时 + 自动重试"核心需求。然后按阶段 2→3 推进根本性方案，最终达成"完成用户指令（无时间限制）+ 绝对友好体验 + 全链路可观测"的目标。

排除数据库迁移后，本报告共列出 **18 个未实施问题**，预估总修复工作量约 **82 人天**（含临时修复 9 人天 + 根本性方案 73 人天）。

**核心设计哲学转变**：从"超时保护"到"无超时 + 自动重试 + 用户可控"。任务持续运行直到完成或用户取消，LLM/DB 异常自动恢复，用户通过可观测性 Dashboard 全程感知进度。
