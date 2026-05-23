# Chat 对话全链路复盘 Review

> **评分**：88 / 100 | **风险等级**：P2  
> **依据**：`docs/README.md` 工作流 · 代码现状（2026-05-23 两轮 P1–P2 收口后）  
> **关联**：[01-chat-review.md](./01-chat-review.md) · [changelog Round1](../changelog/2026-05-23-Chat-Flow-P1-P2-Optimizations.md) · [changelog Round2](../changelog/2026-05-23-Chat-Flow-P1-P2-Round2.md)  
> **需求**：[1 chat.md](../需求/1%20chat.md) · [51 消息机制.md](../需求/51%20消息机制.md)

---

## 1. 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 18 | 20 | WS/HTTP/Channel/Cron 共用 turn；Follow-up、Await、Reasoning、工具卡均已落地 |
| 架构一致性 | 23 | 25 | 分层清晰；Team turn 编排仍与 `trpc_turn` 部分重复 |
| 后端实现质量 | 20 | 20 | Admission/排队/首字节/queued sentinel 已收口 |
| 前端实现质量 | 15 | 15 | Composable 拆分完成；Await 提交、双 WS 消费已修 |
| 测试与验证 | 8 | 10 | agent/server 单测 + WS 协议；缺全链路 E2E |
| 文档一致性 | 4 | 10 | 本次同步；设计正文仍含 `StorePlaceholder` 等旧表述（已修 §1 chat.design） |

---

## 2. 全链路业务逻辑（复盘）

### 2.1 入口与统一 Turn 门控

所有对话 ingress **必须**经 `ChatService.runNativeAgentTurn`（`chat_native.go`）：

| 入口 | 路径 | 说明 |
|------|------|------|
| HTTP | `SendChatMessage` → `nativeSendChatMessage` | 同步返回 user/agent message map |
| WebSocket | `WSServer.handleUserMessage` → `SendChatMessage` | 异步 goroutine；`request_id` 关联错误 |
| Channel | `ChannelIngress.runChatTurnWithOutcome` → `RunNativeTurnUnary` | queued 经 `ErrTurnMessageQueued` |
| Cron / A2A | `RunNativeTurnUnary` / `RunCronTurn` | 同 admission |

**Admission 状态机（当前实现）**

```
lockSession
  → HasActive?
      → ActiveRunner? → EnqueueUserMessage → ErrTurnMessageQueued | reject
      → else (turn starting) → CHAT_TURN_BUSY (409)
  → team? → StoreCancelable + teamsNative.RunTurn
  → agent → StoreCancelable + runSingleAgentViaTRPC
```

要点：
- Agent 启动阶段 **不再** `Finish()` 误清 registry，避免并行 turn。
- `StoreCancelable` 使构建 Runner 期间 **StopGeneration/WS cancel** 可生效。
- 入队成功返回 **`ErrTurnMessageQueued`**，HTTP 空响应、Channel 显式 `Queued` outcome。

### 2.2 单 Agent Turn 执行

`runSingleAgentViaTRPC`（`trpc_turn.go`）职责链：

1. Turn timeout（仅当父 ctx 无 deadline）
2. BuildTRPCAgentCached → NewTurnRunner → `StoreRunner`
3. Intent pass（非 A2A proxy）
4. 持久化 user message → `RunTRPCUserTurnMsg`
5. `ConsumeEventStreamWithFirstByte` → `turnStreamConsumer` → `EventProjector` → EventBus
6. 空回复策略：`displayMarkdown = reply || reasoning`（与 Team 对齐）
7. 持久化 assistant + session turn + usage
8. defer：`Finish` + `processPendingQueue`

**Pending 队列**：defer 中 dequeue 一条；goroutine 内 **先 lockSession**；若 session 仍 active 则 **重新 Enqueue**，否则直接 `runSingleAgentViaTRPC`（不经 admission，避免嵌套锁）。

### 2.3 Team Turn

`teamsNative.RunTurn` → `runner_team_trpc.go`：与 Agent 共享 `turnStreamConsumer` + 首字节超时；member_* envelope 由 `EventProjector` 发射；step 持久化在 team 包内。

### 2.4 实时投影（WS / IM）

```
trpc events
  → turnStreamConsumer（聚合 Reply/Reasoning/Usage + tool 跟踪）
  → EventProjector.Project（ChoiceStreamContent 统一 delta）
  → event.Bus
      → WSServer.eventPump → 客户端 Envelope
      → TurnPreviewCoordinator（Channel IM，非 OnReplyDelta）
```

### 2.5 前端对话 UX

```
用户输入 → useChatSender
  → idle: user_message + pending-user 占位
  → active run: enqueue_message
  → awaiting: useAwaitReply.submit* → HTTP AwaitUserReply

WS Envelope → useChatStreamManager（session 级）
  → text_delta/done、tool_*、runner_completion
  → mergeSessionMessages + dropPendingUserPlaceholders

Global hub → useChatInboundSync
  → 当前 session 的 text 事件跳过（避免双 patch）
  → runner_completion 刷新 sessions / 通知
```

### 2.6 Cancel / RunStatus

- WS `cancel` → `CancelRun` → `cancelActiveRun` → `run_status=cancelled`（**不再**额外 error envelope）
- `StopGeneration` HTTP 同路径

---

## 3. 架构质量

### 3.1 分层（符合 AGENT_RUNTIME_BOUNDARY）

| 层 | 职责 | 评价 |
|----|------|------|
| `internal/server` | WS 传输、replay | ✅ 薄 |
| `internal/service` | 桥点、Runner 装配、持久化 | ⚠️ 仍偏大（ChatService 聚合多 concern） |
| `internal/agent` | 投影、流消费、Runner 封装 | ✅ 不 import biz |
| `internal/team` | Team turn 编排 | ⚠️ 与 service 重复模式 |
| `internal/biz` | Enqueue 编排、Pending CRUD | ✅ |
| `internal/runtime` | RunRegistry + PendingMessageQueue | ✅ |

### 3.2 单一职责（SRP）

| 模块 | 现状 | 建议 |
|------|------|------|
| `runNativeAgentTurn` | admission + 路由 team/agent | 可抽 `admitTurn()` |
| `runSingleAgentViaTRPC` | 全 turn 编排 ~340 行 | 长期抽 shared `TurnExecutor` |
| `EventProjector` | envelope + tool cache + member 路由 | 可接受；tool cache 可独立 |
| `useChatWorkspace` | ~494 行 orchestrator | 已拆子 composable；可再拆 trace/artifacts |
| `useChatStreamManager` / `useChatInboundSync` | 分工已清晰 | ✅ Round2 去掉双 patch |

### 3.3 影响域矩阵

| 变更点 | 直接影响 | 回归关注点 |
|--------|----------|------------|
| `runNativeAgentTurn` admission | 全部 ingress | 并发发送、Channel queued |
| `ChoiceStreamContent` / `EventProjector` | WS 文本、IM preview、DB 持久化 | 空格/Reasoning |
| `RunRegistry` | Cancel、Enqueue、StopGeneration | Team/Agent 一致 |
| `processPendingQueue` | Follow-up 顺序 | 与手动新 turn 竞态 |
| `mergeSessionMessages` | reload 后 UI | pending-user、streaming 行 |
| WS `cancel` 语义 | 前端 cancel 处理 | 仅听 run_status |

---

## 4. 代码质量要点

### 4.1 已收口（Round1 + Round2）

- Delta 投影/聚合一致（`choice_stream.go`）
- Team 首字节超时
- `turnStreamConsumer` 职责拆分 + `countsAsFirstByte`
- Follow-up 前端 `enqueue_message`
- WS `request_id` 错误关联
- Admission busy / queued sentinel
- Await 前端提交、`pending-user` 清理
- Channel queued 显式判定

### 4.2 剩余风险（P2/P3）

| ID | 问题 | 优先级 | 建议 |
|----|------|--------|------|
| CHAT-R2-01 | WS turn 使用 detached `context.Background()`，断连不 cancel | P2 | 绑定 conn lifecycle 或 RunRegistry |
| CHAT-R2-02 | `ChatUsecase.SetRunStatus` 等 API 未被 Service 调用 | P3 | 统一经 Usecase 或删除死 API |
| CHAT-R2-03 | Team/Agent turn 编排重复 | P3 | 共享 executor 抽象 |
| CHAT-R2-04 | 会话切换仍 HTTP `getRunStatus` 快照 | P2 | 全量 WS `run_status` |
| CHAT-R2-05 | 双 enqueue 路径（WS + HTTP `ChatEnqueueMessage`） | P3 | UX 统一 |
| CHAT-R2-06 | 全链路 E2E 测试缺失 | P2 | handshake → message → envelope → replay |
| CHAT-R2-07 | `EventProjector` / team 包单测覆盖薄 | P3 | 补 queued/admission 集成测 |

---

## 5. 测试现状

| 区域 | 覆盖 |
|------|------|
| `internal/agent` | choice_stream、turn_helpers、stream first-byte 行为 |
| `internal/server` | WS ping/cancel/enqueue/user_message+request_id |
| `internal/service` | turn_outcome、cancel run_status、channel preview 部分 |
| `web/features/chat` | mergeSessionMessages、envelopeToolCall |
| 缺失 | admission 并发、processPendingQueue 集成、前端 composable E2E |

---

## 6. 文档同步清单

| 文档 | 动作 |
|------|------|
| `01-chat-review.md` | 评分与风险表更新 |
| `1-chat-development.md` | 锚点、Follow-up/Await 状态 |
| `1 chat.design.md` | RunRegistry admission 描述 |
| `execution-plan.md` | Chat Flow Round2 里程碑 |
| 本文件 | 全链路复盘权威 |

---

## 7. 结论

经过两轮 P1–P2 优化，对话链路在 **业务正确性**（admission、queued、reasoning-only、pending 顺序）和 **实时一致性**（投影/聚合、双 WS 消费）上已达到可生产基线。架构主骨架 **Ingress → Admission → Runner → Projector → Bus → 持久化** 稳定且符合双框架分工。

后续迭代应聚焦：**WS 全链路 E2E**、**run_status 全 WS 驱动**、**Team/Agent turn 编排收敛**，以及 **WS 断连 cancel** 绑定。
