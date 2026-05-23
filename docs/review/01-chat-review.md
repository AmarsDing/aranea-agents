# 01 Chat 对话 Review

> **评分**：88 / 100 | **风险等级**：P2  
> **全链路复盘**：[2026-05-23-Chat-Flow-Full-Review.md](./2026-05-23-Chat-Flow-Full-Review.md)（权威）  
> **文档**：[1 chat.md](../需求/1%20chat.md) · [1 chat.design.md](../需求/1%20chat.design.md) · [1-chat-development.md](../需求/1-chat-development.md)  
> **代码锚点**：`internal/service/chat_native.go` · `trpc_turn.go` · `internal/agent/stream_consumer.go` · `web/src/features/chat/composables/`  
> **审查时间**：2026-05-21 · **P1–P2 收口**：2026-05-23 · **全链路复盘**：2026-05-23

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 18 | 20 | WS/HTTP/Channel/Cron 共用 turn；Follow-up、Await、Reasoning、工具卡均已落地 |
| 架构一致性 | 23 | 25 | `ChoiceStreamContent` + `turnStreamConsumer`；Channel 经 TurnPreviewCoordinator |
| 后端实现质量 | 20 | 20 | Admission busy/queued、pending 加锁、首字节、reasoning-only 已收口 |
| 前端实现质量 | 15 | 15 | Composable 拆分；Await 提交、双 WS 消费、pending-user 清理 |
| 测试与验证 | 8 | 10 | agent/server WS 协议单测；全链路 E2E 仍缺 |
| 文档一致性 | 4 | 10 | 本次已同步 review/development/design；execution-plan 已标注 |

---

## 主链路（2026-05-23）

```
Ingress (HTTP/WS/Channel/Cron)
  → runNativeAgentTurn [lockSession + admission]
      → ActiveRunner? → Enqueue → ErrTurnMessageQueued
      → starting?     → CHAT_TURN_BUSY
      → team/agent turn
  → RunTRPCUserTurnMsg
  → turnStreamConsumer → EventProjector → EventBus → WS / IM
  → persist messages + defer processPendingQueue
```

详见 [全链路复盘 §2](./2026-05-23-Chat-Flow-Full-Review.md#2-全链路业务逻辑复盘)。

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| WS 实时对话 + Envelope | ✅ |
| HTTP unary 对话 | ✅ |
| StopGeneration + WS cancel（run_status only） | ✅ Round2 |
| RunStatus WS Envelope | ✅ |
| AwaitUserReply + 前端 submit | ✅ Round2 |
| 工具卡片 / Reasoning | ✅ |
| Team member_* | ✅ |
| Follow-up Queue 前后端 | ✅ |
| Admission 并发安全 | ✅ Round2 |
| Channel queued 显式 sentinel | ✅ Round2 |
| WS 回放顶栏提示 | ✅ |

---

## 主要风险（开放项）

### P2

| ID | 问题 | 建议 |
|----|------|------|
| CHAT-P2-01 | 会话切换 HTTP `getRunStatus` 快照 | 全量 WS `run_status` |
| CHAT-P2-02 | 模型选项双路径 | 长期单一真相源 |
| CHAT-P2-03 | 全链路 WS E2E 缺失 | connect → message → replay |
| CHAT-P2-04 | WS turn detached context | 断连 cancel 绑定 |

### P3

| ID | 问题 | 建议 |
|----|------|------|
| CHAT-P3-01 | Team/Agent turn 编排重复 | 共享 TurnExecutor |
| CHAT-P3-02 | ChatUsecase 死 API | 统一或删除 |
| CHAT-P3-03 | 双 enqueue UX（WS + HTTP 组件） | 统一入口 |

---

## 已关闭项（勿重复开 task）

| ID | 原问题 | 关闭版本 |
|----|--------|----------|
| CHAT-P1-01 | useChatWorkspace 1500 行 | ✅ composable 拆分 ~494 行 |
| CHAT-P1-02 | PendingQueue 在 service | ✅ `internal/runtime` |
| CHAT-P1-03 | Follow-up 连续发送 | ✅ enqueue_message + message_queued |
| FLOW-P1-* / FLOW-P2-* | 见 changelog Round1/2 | ✅ 2026-05-23 |

---

## 前端分层（当前）

```
ChatPage.vue
  → useChatWorkspace.ts (~494 行 orchestrator)
      → useChatStreamManager / useChatSender / useFollowUpQueue / useAwaitReply
      → useChatInboundSync（非当前 session 通知）
  → ws-transport.ts / useEnvelopeStream.ts
  → stores/app.ts + features/chat/api.ts
```

---

## 建议下一步

1. WS 全链路 E2E + admission 并发单测  
2. `run_status` 全 WS 驱动，减少 HTTP 快照  
3. Team/Agent shared turn executor（降低重复与回归面）
