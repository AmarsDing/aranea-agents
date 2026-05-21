# 01 Chat 对话 Review

> **评分**：84 / 100 | **风险等级**：P1  
> **文档**：[1 chat.md](../需求/1%20chat.md) · [1 chat.design.md](../需求/1%20chat.design.md) · [1-chat-development.md](../需求/1-chat-development.md)  
> **代码锚点**：`internal/service/chat.go` · `internal/service/trpc_turn.go` · `internal/service/chat_enqueue.go` · `web/src/features/chat/`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 17 | 20 | WS 主链路、工具卡片、Reasoning 展示、RunStatus、AwaitUserReply 均已落地；Follow-up Queue 前端 UX（连续发送）待补 |
| 架构一致性 | 22 | 25 | ChatService 正确组装 Runner；EventBus + EventProjector 投影完整；PendingQueue 在 Service 层是架构漂移 |
| 后端实现质量 | 18 | 20 | Session 锁、RunRegistry 入队、quota 拦截、用量记录路径清晰；AwaitUserReply 跨进程 resume 已实现 |
| 前端实现质量 | 14 | 15 | Envelope 订阅、工具卡片、Reasoning 折叠、A2UI 组件树均已实现；`useChatWorkspace.ts` 约 1500 行，是主要质量风险 |
| 测试与验证 | 7 | 10 | `chat_await_*_test.go`、`chat_cancel_run_test.go` 已有；WS 集成测试缺失 |
| 文档一致性 | 6 | 10 | 开发计划（2026-05-21）与现状同步；`1 chat-execution-trace.md` 无独立开发计划文件（折入主开发计划） |

---

## 模块定位

Chat 是用户与 Agent/Team 交互的核心入口，负责：
- HTTP/WS 发起对话（`SendChatMessage` / `user_message`）
- WS 上行控制（cancel / enqueue_message）
- EventBus 实时事件投影推送
- 上下文管理、用量记录、停止生成
- AwaitUserReply 人工等待与待执行队列

---

## 主链路验收

### 单 Agent 对话

```
WS user_message → ChatService.handleUserMessage
    → lockSession + RunRegistry.CanEnqueue
    → BuildTRPCLLMAgentCached (internal/agent)
    → NewTRPCRunner (Session + Memory + Plugins)
    → Runner.Run → event.Event stream
    → ConsumeEventStream → EventProjector.ProjectAndPublish
    → EventBus → WS Envelope → 客户端
    → recordTurnUsage + appendChatMessage
```

**状态**：✅ 端到端可用

### Follow-up Queue (SteerableRunner)

```
POST /v1/chat/enqueue / WS enqueue_message
    → ChatUsecase.EnqueueUserMessage
    → PendingMessageQueue FIFO
    → publishMessageQueuedToBus
```

**状态**：✅ 后端；🟡 前端 UX（Cursor 式连续发送待补 Phase 1.5）

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| WS 实时对话 + Envelope | ✅ |
| HTTP unary 对话 | ✅ |
| StopGeneration + WS cancel | ✅ |
| RunStatus WS Envelope | ✅ 持久化到 state_json |
| AwaitUserReply 跨进程 resume | ✅ 新 turn + `await_resumed` |
| 工具卡片（工具调用/结果/耗时） | ✅ ChatToolCallCard |
| Reasoning 折叠展示 | ✅ |
| ReAct 步骤卡 + A2UI 组件树 | ✅ |
| 多模态附件 / Vision | ✅ |
| Team member_* 分栏 | ✅ |
| 事件回放 replay_start/end | ✅ |
| Follow-up Queue 后端 | ✅ |
| Follow-up Queue 前端 UX | 🟡 P1 待补 |
| WS 控制消息反馈 | 🟡 页面无回放提示 |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| CHAT-P1-01 | `useChatWorkspace.ts` 约 1500 行，包含 WS 流、Pending 队列、AwaitUserReply、ReAct 等多个关注点，可读性差、测试极难 | 拆分为 `useChatStream`、`useChatSender`、`useFollowUpQueue`、`useAwaitReply` 等独立 composable |
| CHAT-P1-02 | `PendingMessageQueue` 实现在 `internal/service/chat_pending.go`，Service 层承担了状态机职责 | 下沉到 `internal/runtime` 或 `biz.ChatUsecase` |
| CHAT-P1-03 | Follow-up Queue 前端 UX：运行中 Enter 应入队而非阻塞；依赖纯 3s 轮询刷新 Pending 列表 | 监听 `run_status.metadata.hint === "message_queued"` 立即刷新 |
| CHAT-P1-04 | 部分业务文案仍为硬编码中文（工具卡片、Reasoning 标签等） | 迁入 `web/src/locales/` i18n 键 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| CHAT-P2-01 | RunStatus 仍有 HTTP 快照轮询（会话切换时）— 应全量 WS 驱动 | 用 `state_delta` Envelope 替代 |
| CHAT-P2-02 | 模型选项双路径：Platform 优先 + `GetChatOptions` 回退 | 长期统一为单一真相源 |
| CHAT-P2-03 | WS 集成测试缺失（connect → message → disconnect 完整流程） | 补 WS 集成测试 |

---

## 前端分层评价

```
ChatPage.vue
    ↓
useChatWorkspace.ts (1500 行 — 需拆分)
    ↓
features/chat/ws-transport.ts (WS 客户端 — 正确隔离)
features/chat/useEnvelopeStream.ts (Envelope 订阅 — 正确)
stores/chat/index.ts (选中 Agent/会话等)
    ↓
features/chat/api.ts → services/kratos/chat/
```

**问题**：composable 层过重；`stores/chat` 与 `useChatWorkspace` 职责边界模糊。

---

## 建议优化路径

1. **高优**：拆分 `useChatWorkspace.ts`，按功能域创建独立 composable（流、发送、Pending、Await）。
2. **本迭代**：实现 WS 驱动的 Pending 刷新（`message_queued` hint 监听）。
3. **下迭代**：将 `PendingMessageQueue` 从 Service 下沉到 runtime。
4. **长期**：补全 WS 集成测试，覆盖 cancel/enqueue/replay 场景。
