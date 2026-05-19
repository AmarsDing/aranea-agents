# Chat 对话 — 开发计划

> **版本**：2026-05-19 | **状态**：✅ 端到端可用（WS/EventBus 主通道；RunRegistry + EnqueueUserMessage；AwaitUserReply UI 已接入）
> **需求**：[1 chat.md](./1%20chat.md) · **设计**：[1 chat.design.md](./1%20chat.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Chat 是用户与 Agent/Team 交互的核心入口，负责 HTTP/WS 发起对话、WebSocket + EventBus 实时事件、上下文管理、用量记录、停止生成、人工等待回复与待执行队列。

**代码锚点**：
- `api/kratos/chat/v1/chat.proto` — Chat RPC（含 `EnqueueUserMessage` → `POST /v1/chat/enqueue`）
- `internal/runtime/run_registry.go` — 会话级 active run / cancel / run status
- `internal/server/ws.go` — WebSocket（`user_message` / `cancel` / `enqueue_message`）
- `internal/service/chat.go` — ChatService 桥接 + awaitChans / EnqueueUserMessage
- `internal/service/chat_native.go` — 原生对话入口（HTTP unary + WS 上行复用）
- `internal/service/trpc_turn.go` — trpc-agent-go 单 Agent turn + EventBus 投影
- `internal/agent/event_projector.go` — trpc event → Envelope（含 Team `member_*`）
- `internal/event/envelope.go` — WS Envelope 协议与 channel 路由
- `web/src/features/chat/ws-transport.ts` — WS 客户端（心跳、重连、控制消息）
- `web/src/features/chat/composables/useChatWorkspace.ts` — Chat 页编排（WS 流、待执行、AwaitUserReply）
- `web/src/features/chat/useEnvelopeStream.ts` — Envelope 订阅与 `useChatStream` / `useTeamStream`

---

## 2. 现状评估（2026-05-19）

| 项 | 状态 | 证据 |
|----|------|------|
| WS 实时对话 | ✅ | `/v1/ws` + `user_message` + EventBus Envelope |
| HTTP unary 对话 | ✅ | `SendChatMessage` / `RunNativeTurnUnary` |
| Channel / Cron 入口 | ✅ | `lockSession` + `RunRegistry` |
| 停止 / 运行中追加 | ✅ | `StopGeneration` / WS `cancel`；`EnqueueUserMessage` |
| 待执行队列 | ✅ | `pendingQueue` + UI 取消/编辑 |
| RunStatus + AwaitUserReply | ✅ | RPC + Chat 页横幅与提交（`useChatWorkspace` 轮询） |
| Team member_* Envelope | ✅ | `EventProjector` + `useChatWorkspace` 成员流 |
| WS 控制消息 | 🟡 | `ws-transport`：`connected`/`pong`/`replay_*`/`server_shutdown`；页面无回放提示 |
| 工具事件 UI | ✅ | `ChatToolCallCard`：参数/结果/耗时/`is_long_running` 折叠卡片 |
| Reasoning UI | ✅ | 默认折叠 `<details>` 展示 `reasoning_markdown` |
| RunStatus | ✅ | WS `run_status` Envelope 驱动；会话切换时 HTTP 快照一次 |
| WS 回放提示 | ✅ | `replay_start/end` → 顶栏「正在同步历史事件…」 |
| Team 成员流 UX | ✅ | `team_member` 元数据 + 左侧色条分栏 |
| 模型选项 | 🟡 | Platform 优先 + `GetChatOptions("model")` 回退 |
| 附件 / Vision | ❌ | 前端占位 |
| RunStatus 持久化 | ❌ | 进程内状态 |

---

## 3. 差距与优化（按优先级）

### P1 — 体验闭环

1. **工具事件结构化卡片**：`tool_call`/`tool_result` → 参数 JSON、结果、耗时、`is_long_running` 折叠面板（`ChatMessagePanel` + `toolEventMarkdown` 扩展）。
2. **Reasoning 展示规格**：产品定稿（折叠/内联/侧栏）并在助手气泡渲染 `content.reasoning`。

### P2 — 实时与 Team UX

3. **RunStatus WS 驱动**：用 `state_delta` 或专用 Envelope 替代 2s `getRunStatus` 轮询。
4. **Team 成员分栏**：`member-{agent_key}` 消息增加头像、角色标签、独立气泡样式。
5. **WS 回放 UX**：`ws-transport.onReplayState` → Chat 顶栏「同步历史事件…」提示。

### P3 — 平台级

6. **多模态附件**：上传 API、对象存储、Vision 输入装配。
7. **RunStatus 可恢复**：`awaiting_user` 持久化或 EventBuffer 恢复策略。
8. **模型选项单一真相源**：长期统一为 `GetChatOptions` 或 Platform 之一（当前为 Platform 优先 + 回退）。

---

## 4. 开发阶段

| Phase | 主题 | 状态 |
|-------|------|------|
| 1 | 文档与 WS/EventBus 主通道 | ✅ |
| 2 | Team active run / pending / cancel | ✅ |
| 3 | AwaitUserReply 后端 + Chat UI | ✅ |
| 4 | 数据一致性（pending_id、session_turns、Channel 互斥） | ✅ |
| 5 | Team `member_*` + 成员流消费 | ✅ 协议通；UX 待增强 |
| 6 | 工具可观测 UI | ⏳ |
| 7 | Reasoning 展示 | ⏳ |
| 8 | 附件 / RunStatus 持久化 | ⏳ |

---

## 5. 任务清单

| # | 任务 | 优先级 | 状态 |
|---|------|--------|------|
| 1 | 文档移除 SSE 主路径 | P1 | ✅ |
| 2 | Team turn active run / pending | P1 | ✅ |
| 3 | Team defer `processPendingQueue` | P1 | ✅ |
| 4 | AwaitUserReply Chat UI | P1 | ✅ |
| 5 | `EnvelopeError.PendingID` 统一 | P1 | ✅ |
| 6 | WS 控制消息协议 + transport | P1 | ✅ |
| 7 | `recordTeamSessionTurn` | P2 | ✅ |
| 8 | Channel/Cron 互斥 | P2 | ✅ |
| 9 | Team `member_*` 发射与消费 | P2 | ✅ |
| 10 | 工具事件结构化卡片 | P1 | ✅ |
| 11 | Reasoning 展示规格与实现 | P1 | ✅ |
| 12 | EventBuffer TTL | P2 | ✅ |
| 13 | RunStatus WS 事件驱动 | P2 | ✅ |
| 14 | 多模态附件后端 | P3 | ⏳ |
| 15 | 模型选项单一来源（长期） | P3 | 🟡 回退已实现 |
| 16 | RunStatus 持久化 | P3 | ⏳ |
| 17 | ChatService / WS 单测 | P1 | ⏳ |
| 18 | RunRegistry + EnqueueUserMessage | P0 | ✅ |

---

## 6. 验收标准

- [x] 无 `/v1/chat/messages/stream` 当前端点表述
- [x] WS 控制消息在需求/设计文档中完整描述
- [x] Team 停止/待执行与单 Agent 一致
- [x] AwaitUserReply：后端 + Chat 页可提交回复
- [x] `error.pending_id` 前端可消费
- [x] `session_turns` Agent + Team 均有记录
- [x] Channel/Cron 不绕过 active run 互斥
- [x] Team `member_*` 后端发射 + 前端增量展示
- [x] 工具执行结构化卡片（参数/结果/耗时/`is_long_running`）
- [x] Reasoning 折叠/展示符合产品规格
- [x] RunStatus 与 WS `run_status` 一致（切换会话时 HTTP 校准）
- [x] WS 重连回放时用户可见「同步中」状态
- [x] `go test ./internal/service/... -run TestChat` 通过

---

## 7. 依赖与风险

- Team 成员 UX 依赖 `MemberAgentKeys` 在 turn 元数据中完整传递
- 工具卡片依赖 `EventProjector` 对 `duration_ms` / `is_long_running` 的稳定填充
- RunStatus 持久化可能依赖 M2 多租户 Session 改造
- 附件闭环依赖 Artifact / 对象存储与 LlmProvider Vision 能力
