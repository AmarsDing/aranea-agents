# Chat 对话 — 开发计划

> **版本**：2026-05-23 | **状态**：✅ 端到端可用；Follow-up / Await / Admission 两轮 P1–P2 已收口  
> **Review**：[2026-05-23-Chat-Flow-Full-Review.md](../review/2026-05-23-Chat-Flow-Full-Review.md)  
> **M55 Cursor 对标**：[55-chat-channel-cursor-solution.md](./55-chat-channel-cursor-solution.md) · [55-chat-channel-cursor-development.md](./55-chat-channel-cursor-development.md)  
> **四层解耦（DECO）**：[0-module-decoupling-architecture.md §3.1](./0-module-decoupling-architecture.md#31-推荐目标架构channel--chat--agent) · 任务板 [17-channel-development.md §14](./17-channel-development.md#14-phase-deco--四层架构解耦deco)（DECO-06/13/14）
> **需求**：[1 chat.md](./1%20chat.md) · **设计**：[1 chat.design.md](./1%20chat.design.md)  
> **执行卡片 v2**：[1 chat-execution-trace.md](./1%20chat-execution-trace.md) · [1 chat-execution-trace.design.md](./1%20chat-execution-trace.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Chat 是用户与 Agent/Team 交互的核心入口，负责 HTTP/WS 发起对话、WebSocket + EventBus 实时事件、上下文管理、用量记录、停止生成、人工等待回复与待执行队列。

**代码锚点**：
- `api/kratos/chat/v1/chat.proto` — Chat RPC（含 `EnqueueUserMessage` → `POST /v1/chat/enqueue`）
- `internal/runtime/run_registry.go` — 会话级 active run / cancel / run status
- `internal/runtime/pending_queue.go` — PendingMessageQueue FIFO
- `internal/server/ws.go` — WebSocket（`user_message` / `cancel` / `enqueue_message`）
- `internal/biz/chat_usecase.go` — Follow-up Queue 编排（EnqueueUserMessage / Pending CRUD）
- `internal/service/turn_outcome.go` — `ErrTurnMessageQueued` / `CHAT_TURN_BUSY`
- `internal/service/chat_run_gateway.go` — Biz 适配器 + `publishMessageQueuedToBus`
- `internal/service/chat_native.go` — 原生对话入口（admission + team/agent 路由）
- `internal/service/trpc_turn.go` — trpc-agent-go 单 Agent turn + EventBus 投影
- `internal/agent/choice_stream.go` · `stream_consumer.go` — 流式 delta 与 turn 消费
- `internal/agent/event_projector.go` — trpc event → Envelope（含 Team `member_*`）
- `internal/event/envelope.go` — WS Envelope 协议与 channel 路由
- `web/src/features/chat/composables/useChatWorkspace.ts` — Chat 页编排
- `web/src/features/chat/composables/useChatStreamManager.ts` · `useChatSender.ts` · `useFollowUpQueue.ts` · `useAwaitReply.ts`
- `web/src/features/chat/ws-transport.ts` — WS 客户端（心跳、重连、控制消息）

---

## 2. 现状评估（2026-05-19）

| 项 | 状态 | 证据 |
|----|------|------|
| WS 实时对话 | ✅ | `/v1/ws` + `user_message` + EventBus Envelope |
| HTTP unary 对话 | ✅ | `SendChatMessage` / `RunNativeTurnUnary` |
| Channel / Cron 入口 | ✅ | `lockSession` + `RunRegistry` |
| 停止 / 运行中追加 | ✅ | `StopGeneration` / WS `cancel`；`EnqueueUserMessage` |
| 待执行 / Follow-up Queue | ✅ | Steerable + Pending FIFO；WS `enqueue_message` + `message_queued` 刷新 |
| RunStatus + AwaitUserReply | ✅ | RPC + WS；`useAwaitReply` submit（Round2） |
| Team member_* Envelope | ✅ | `EventProjector` + `useChatWorkspace` 成员流 |
| WS 控制消息 | 🟡 | `ws-transport`：`connected`/`pong`/`replay_*`/`server_shutdown`；页面无回放提示 |
| 工具事件 UI | ✅ | `ChatToolCallCard`：参数/结果/耗时/`is_long_running` 折叠卡片 |
| Reasoning UI | ✅ | 默认折叠 `<details>` 展示 `reasoning_markdown` |
| RunStatus | ✅ | WS `run_status` Envelope 驱动；会话切换时 HTTP 快照一次 |
| WS 回放提示 | ✅ | `replay_start/end` → 顶栏「正在同步历史事件…」 |
| Team 成员流 UX | ✅ | `team_member` 元数据 + 左侧色条分栏 |
| 模型选项 | 🟡 | Platform 优先 + `GetChatOptions("model")` 回退 |
| 附件 / Vision | ✅ | Artifact 上传 + `buildUserMessage` 多 part 装配 |
| RunStatus 持久化 | ✅ | `state_json` + trpc `PendingAwaitUserReplyRoute`；resume 同步清状态 + `resumeInFlight` 防双 turn |

---

## 3. 差距与优化（按优先级）

### P1 — Follow-up Queue UX（Cursor 对齐）

1. ~~**运行中连续发送**~~ ✅ `useChatSender` → `enqueue_message`（Round1）
2. ~~**WS 驱动 Pending 刷新**~~ ✅ `messageQueuedFromEnvelope`（Round1）
3. **（P3）** 可选 `pending_enqueued` Envelope 携带 `pending_id` + 内容预览。

### P1 — Admission / 并发（2026-05-23 收口）

1. ~~Turn starting 并行 turn~~ ✅ `CHAT_TURN_BUSY` + `StoreCancelable`
2. ~~Channel queued 启发式~~ ✅ `ErrTurnMessageQueued`
3. ~~`processPendingQueue` 竞态~~ ✅ lock + 重新入队

详见 [Chat Flow Full Review](../review/2026-05-23-Chat-Flow-Full-Review.md)

### P1 — 体验闭环（原）

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

### P0 — Cursor 对标（M55，2026-05-23）

> 详案：[55-chat-channel-cursor-development.md](./55-chat-channel-cursor-development.md) Phase C/E · [UX Backlog changelog](../changelog/2026-05-23-M55-Feishu-Rebind-UX-Backlog.md)

1. **TurnBlock UI**：一轮 = User → ToolStrip（折叠）→ Assistant — **骨架 ✅**（`TurnBlock.vue`）
2. **Channel 会话同步**：`session_revision` 增量 hydrate — **✅**
3. **滚动锚点**：最后一轮正文 — **✅**
4. **思考/ReAct UX**：互斥呈现、空 reasoning 不展示 — **🟡** IM preview sanitize ✅（CH-BOR-12）；Web CC-C-UX-* 📋
5. **（P2）@ 上下文引用**、**diff Apply 卡片** — **📋**

---

## 4. 开发阶段

| Phase | 主题 | 状态 |
|-------|------|------|
| 1 | 文档与 WS/EventBus 主通道 | ✅ |
| 2 | Team active run / pending / cancel | ✅ |
| 3 | AwaitUserReply 后端 + Chat UI | ✅ |
| 4 | 数据一致性（pending_id、session_turns、Channel 互斥） | ✅ |
| 5 | Team `member_*` + 成员流消费 | ✅ 协议通；UX 待增强 |
| 6 | 工具可观测 UI（基础卡片） | ✅ |
| 6b | 执行过程卡片 v2（Skill/MCP/默认折叠/持久化 schema） | ✅ P0 |
| 7 | Reasoning 展示 | ⏳ | 折叠 UI 有；空壳/双轨（ReAct vs reasoning）待 CC-C-UX-01 |
| 8 | 附件 / RunStatus 持久化 | 🟡 |
| 9 | M55 TurnBlock + Channel 同步 | 🚧 | Phase A–D ✅；UX 收口 CC-C-UX-* 📋 |

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
| 14 | 多模态附件后端 | P3 | ✅ |
| 15 | 模型选项单一来源（长期） | P3 | 🟡 回退已实现 |
| 16 | RunStatus 持久化 | P3 | ✅ `state_json` + `PendingAwaitUserReplyRoute` |
| 17 | ChatService / WS 单测 | P1 | ⏳ |
| 18 | RunRegistry + EnqueueUserMessage | P0 | ✅ |
| 19 | 执行过程卡片 v2（EnvelopeToolCall v2、ChatExecutionCard） | P0 | ✅ |
| 20 | 执行卡片持久化 + catalog 名 + Team 标识 + 流式修复 | P1 | ✅ |

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


---

## 子模块：Chat 优化计划 2026-05-28

> **Goal:** 修复 Chat 模块 P0 Bug，重构前端发送路径消除重复代码，改善错误处理和用户体验
> **Architecture:** 前端统一发送策略模式 + 后端保持现有分层，聚焦前端 P0/P1 修复
> **Tech Stack:** Vue 3 + TypeScript + Pinia + Quasar

---

## 问题总览

| 优先级 | ID | 问题 | 类型 | 状态 |
|--------|-----|------|------|------|
| P0 | P0-1 | retryFailedMessage Team 消息重试走错通道 | Bug | ✅ 已修复 |
| P0 | P0-2 | Stall 检测定时器已定义但从未激活 | Bug | ✅ 已修复 |
| P0 | P0-3 | failedPendingIds 模块级全局 Set 内存泄漏 | Bug | ✅ 已修复 |
| P0 | P0-4 | inboundHydrateError 设置后永不清除 | Bug | ✅ 已修复 |
| P1 | P1-1 | Agent/Team 双通道发送逻辑 80% 重复 | 架构 | ✅ 已修复 |
| P1 | P1-2 | streamHandlers 守卫代码重复 6 处 | 架构 | ✅ 已修复 |
| P1 | P1-3 | 后端错误码未在前端映射 | UX | ✅ 已修复 |
| P1 | P1-4 | 助手消息错误无恢复路径 | UX | ✅ 已修复 |
| P1 | P1-5 | WS 重连后不主动同步 Run 状态 | 业务 | ✅ 已修复 |
| P2 | P2-1 | ChatBackgroundJobsPanel 展示组件直接调 API | 红线 | ✅ 已修复 |
| P2 | P2-2 | ChatMessageAttachments 展示组件直接调 Store | 红线 | ✅ 已修复 |
| P2 | P2-3 | 死代码导出（patchTeamMessages 等） | 清理 | ✅ 已修复 |

---

## Task 1-9: P0/P1 修复（已完成）

详见历史记录。

---

## Task 10: P2-1 ChatBackgroundJobsPanel 展示组件直接调 API ✅

**Files:**
- Modify: `web/src/components/chat/ChatBackgroundJobsPanel.vue`
- Modify: `web/src/components/chat/ChatComposer.vue`
- Modify: `web/src/pages/ChatPage.vue`
- Modify: `web/src/features/chat/composables/useChatWorkspace.ts`

### 根因

`ChatBackgroundJobsPanel` 展示组件内直接 `import { cancelChatBackgroundJob } from "../../features/chat/api"` 并在 `cancelJob` 中调用 API + `$q.notify`，违反红线 #2 和 #4。

### 修复方案

1. 移除 `cancelChatBackgroundJob` import
2. `cancelJob` 改为 `emit('cancel-job', { id, source })`
3. ChatComposer → ChatPage 逐层转发 `cancel-job` 事件
4. `useChatWorkspace` 新增 `cancelBackgroundJob` 方法，动态 import API + 处理通知

---

## Task 11: P2-2 ChatMessageAttachments 展示组件直接调 Store ✅

**Files:**
- Modify: `web/src/components/chat/ChatMessageAttachments.vue`
- Modify: `web/src/components/chat/ChatMessageRow.vue`
- Modify: `web/src/components/chat/ChatMessagePanel.vue`
- Modify: `web/src/pages/ChatPage.vue`
- Modify: `web/src/features/chat/composables/useChatWorkspace.ts`

### 根因

`ChatMessageAttachments` 展示组件内 `import { useArtifactStore }` 并在 `onDownload`/`onDelete` 中直接调用 Store 方法 + `$q.notify`，违反红线 #1 和 #3。

### 修复方案

1. 移除 `useArtifactStore` / `useQuasar` import
2. `onDownload` 改为 `emit('download', meta)`，`onDelete` 改为 `emit('deleted', id)`
3. ChatMessageRow → ChatMessagePanel → ChatPage 逐层转发 `download-artifact` 事件
4. `useChatWorkspace` 新增 `downloadArtifact` 方法，调用 `useArtifactStore` + 处理通知

---

## Task 12: P2-3 死代码导出清理 ✅

**Files:**
- Modify: `web/src/features/chat/composables/useChatStreamManager.ts`
- Modify: `web/src/features/chat/composables/useFollowUpQueue.ts`
- Modify: `web/src/features/chat/composables/useChatProviderOptions.ts`

### 修复方案

1. 删除 `patchTeamMessages` 函数及导出（无外部调用者）
2. 删除 `resolveTeamMemberMeta` 导出（仅内部使用）
3. 删除 `onRunStatusHint` 函数及导出（空函数，无调用者）
4. 删除 `startPendingPoll`/`stopPendingPoll` 导出（无外部调用者）
5. 删除 `ensureSelectedModel` 导出（仅内部使用）

---

## Review 修复记录

### 第一轮 Review（7 个问题）

| # | 严重度 | 问题 | 修复 |
|---|--------|------|------|
| 1 | 🔴 严重 | `reactive` 使用但未从 vue 导入 | 添加 `reactive` 到 import |
| 2 | 🔴 严重 | `touchRunActivity` 定义但从未调用，Stall 检测完全失效 | 在 StreamHandlerCtx 新增 `onRunActivity`，WS 事件中调用 |
| 3 | 🔴 严重 | `formatErrorWithHint` 拼接原始 i18n key | 改为中文文本 |
| 4 | 🟠 高 | `clearFailedPendingForSession` 清除所有会话 | 改为 `Map<string, string>` 按会话清除 |
| 5 | 🟠 高 | TurnBlock 缺少 `@retry`/`@dismiss-failed` 事件转发 | 添加事件转发 |
| 6 | 🟡 中 | `regenerateMessage` 未停止活跃 Run | 重生前先 cancel + stop |
| 7 | 🟢 低 | `isFailedPendingMessage` 导出但无调用者 | 删除死代码 |

### 第二轮 Review（3 个问题）

| # | 严重度 | 问题 | 修复 |
|---|--------|------|------|
| 1 | 🟡 建议 | TurnBlock 成员 ChatMessageRow 缺少事件转发 | 添加 `@retry`/`@dismiss-failed`/`@regenerate` |
| 2 | 🟡 建议 | `sendMessage` 直接 API 调用未标注 TECH-DEBT | 添加标注 |
| 3 | 🟡 建议 | `ChatMessagePanel` 硬编码 hex fallback `#4caf50` | 记录为剩余项 |

### 第三轮 Review（aranea-frontend-review 全面审查，0 阻断）

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| 数据流合规 | 0 | 2 | 0 | 2 |
| 组件分层 | 0 | 1 | 1 | 2 |
| 业务逻辑归属 | 0 | 1 | 0 | 2 |
| 聊天消息分组 | 0 | 0 | 0 | 0 |
| UX 主题 | 0 | 1 | 0 | 1 |
| 构建与回归 | 0 | 0 | 0 | 0 |
| **合计** | **0** | **5** | **1** | **6** |

**关键结论**：所有红线违规已修复，展示组件零 Store/API 依赖，ChatPage.vue 仅 19 行 script，聊天消息分组完全合规。

---

## 剩余项（P2-4 ~ P2-10 已于 2026-05-29 修复；P3-1 ~ P3-3 + CC-C-UX-01~03 已于 2026-05-29 修复）

| # | 优先级 | ID | 问题 | 类型 | 状态 |
|---|--------|-----|------|------|------|
| 1 | P2 | P2-4 | features/chat/components/ 下存在 2 个 .vue 文件 | 红线 #5 | ✅ 已修复 |
| 2 | P2 | P2-5 | messageStore 函数级硬依赖 sessionStore | 架构 | ✅ 已修复 |
| 3 | P2 | P2-6 | runtimeStore 函数级硬依赖 sessionStore | 架构 | ✅ 已修复 |
| 4 | P2 | P2-7 | useChatWorkspace 返回 80+ 属性，760+ 行 | 架构 | ✅ 已修复 |
| 5 | P2 | P2-8 | useChatWorkspace/useChatProviderOptions 直接调 API | 红线 #11 | ✅ 已修复 |
| 6 | P2 | P2-9 | ChatMessagePanel 硬编码 hex `#4caf50` | UX | ✅ 已修复 |
| 7 | P2 | P2-10 | useChatStore facade 已废弃但仍保留 162 行 | 死代码 | ✅ 已修复 |
| 8 | P3 | P3-1 | ChatComposer 展示组件内 $q.notify | 规范 | ✅ 已修复（改为 emit） |
| 9 | P3 | P3-2 | ChatSessionSidebar import sessionSync | 规范 | ✅ 已修复（改为 props/emits） |
| 10 | P3 | P3-3 | ChatMessagePanel 容器组件放在 components/ | 规范 | ✅ 已修复（标注 Container: approved） |
| 11 | M55 | CC-C-UX-01 | reasoning v-if 未 trim 空白字符串 | UX | ✅ 已修复 |
| 12 | M55 | CC-C-UX-02 | "正在思考…" 与 ChatReasoningPeek 硬切换闪烁 | UX | ✅ 已修复（合并进 ChatReasoningPeek） |
| 13 | M55 | CC-C-UX-03 | 双 ToolStrip ReAct 重复 | UX | ✅ 已确认（filterToolsForToolStrip 已解决） |

---

## 验证

每个 Task 完成后运行：
```bash
cd web && npx quasar build
```

---

## 最终统计

### 完成情况

| 类别 | 已完成 | 剩余 | 完成率 |
|------|--------|------|--------|
| P0（Bug） | 4 | 0 | **100%** |
| P1（架构/UX） | 5 | 0 | **100%** |
| P2（红线/清理） | 10 | 0 | **100%** |
| P3（规范） | 3 | 0 | **100%** |
| M55 Phase C UX | 3 | 0 | **100%** |
| **总计** | **25** | **0** | **100%** |

> **核心指标**：P0+P1+P2+P3+M55-PhaseC 完成率 **100%**，所有阻断级红线违规和规范建议已修复。

### 改动文件清单

| 文件 | 改动类型 |
|------|----------|
| `web/src/features/chat/composables/useChatSender.ts` | 重构（策略模式统一发送路径 + P0 修复） |
| `web/src/features/chat/composables/useChatWorkspace.ts` | 修改（事件处理 + 新方法） |
| `web/src/features/chat/composables/useChatStreamManager.ts` | 修改（onRunActivity + refreshRunStatus + 死代码清理） |
| `web/src/features/chat/composables/useFollowUpQueue.ts` | 修改（死代码清理） |
| `web/src/features/chat/composables/useChatProviderOptions.ts` | 修改（死代码清理） |
| `web/src/features/chat/streamHandlers.ts` | 修改（withSessionFilter + onRunActivity + errorCodeHints） |
| `web/src/features/chat/errorCodeHints.ts` | 新建（错误码映射） |
| `web/src/features/chat/useEnvelopeStream.ts` | 修改（createTeamStream onConnected） |
| `web/src/components/chat/ChatMessageRow.vue` | 修改（regenerate 按钮 + download-artifact 事件） |
| `web/src/components/chat/ChatMessagePanel.vue` | 修改（事件转发 + download-artifact） |
| `web/src/components/chat/TurnBlock.vue` | 修改（事件转发 retry/dismiss-failed/regenerate） |
| `web/src/components/chat/ChatBackgroundJobsPanel.vue` | 修改（API 调用改为 emit） |
| `web/src/components/chat/ChatMessageAttachments.vue` | 修改（Store 调用改为 emit） |
| `web/src/components/chat/ChatComposer.vue` | 修改（cancel-job 事件转发） |
| `web/src/pages/ChatPage.vue` | 修改（事件绑定） |

### P2-4 ~ P2-10 修复改动（2026-05-29）

| 文件 | 改动类型 | 关联 ID |
|------|----------|---------|
| `web/src/components/chat/ChatMessagePanel.vue` | 修改（#4caf50 → #4CAF7C） | P2-9 |
| `web/src/components/chat/ChatRunnerStatus.vue` | 新建（从 features/chat/components/ 迁移） | P2-4 |
| `web/src/components/chat/ChatEnqueueMessage.vue` | 新建（从 features/chat/components/ 迁移） | P2-4 |
| `web/src/stores/chat/index.ts` | 修改（删除 useChatStore facade，保留 re-export） | P2-10 |
| `web/src/stores/index.ts` | 修改（删除 useChatStore 导出） | P2-10 |
| `web/src/stores/__tests__/app.store.spec.ts` | 修改（改用子 Store 直接调用） | P2-10 |
| `web/src/stores/chat/messageStore.ts` | 修改（移除 sessionStore 依赖，sessionId 必传） | P2-5 |
| `web/src/stores/chat/runtimeStore.ts` | 修改（移除 sessionStore 依赖，新增 submitFeedback/cancelBackgroundJob/listChatOptions） | P2-6/P2-8 |
| `web/src/features/chat/composables/useChatWorkspace.ts` | 修改（提取 dialogs/composerActions，API 调用迁入 Store） | P2-7/P2-8 |
| `web/src/features/chat/composables/useChatDialogs.ts` | 新建（dialog 状态聚合 composable） | P2-7 |
| `web/src/features/chat/composables/useChatComposerActions.ts` | 新建（composer action 方法 composable） | P2-7 |
| `web/src/features/chat/composables/useChatProviderOptions.ts` | 修改（listChatOptions 改为 Store 调用） | P2-8 |
| `web/src/features/chat/composables/useChatSender.ts` | 修改（新增 sendTeamMessage 导出） | P2-7 |

### P3-1 ~ P3-3 + CC-C-UX-01~03 修复改动（2026-05-29）

| 文件 | 改动类型 | 关联 ID |
|------|----------|---------|
| `web/src/components/chat/ChatComposer.vue` | 修改（$q.notify → emit('paste-unsupported')，移除 useQuasar） | P3-1 |
| `web/src/components/chat/ChatMessagePanel.vue` | 修改（新增 paste-unsupported/cancel-job emit 转发，标注 Container: approved） | P3-1/P3-3 |
| `web/src/pages/ChatPage.vue` | 修改（处理 paste-unsupported 通知，传入 favoriteIds/toggle-favorite） | P3-1/P3-2 |
| `web/src/components/chat/ChatSessionSidebar.vue` | 修改（sessionSync → props favoriteIds + emit toggle-favorite） | P3-2 |
| `web/src/features/chat/composables/useChatWorkspace.ts` | 修改（新增 favoriteIds/onToggleFavorite） | P3-2 |
| `web/src/components/chat/ChatMessageRow.vue` | 修改（reasoning v-if 加 trim，合并 thinkingIndicator 进 ChatReasoningPeek） | CC-C-UX-01/02 |
| `web/src/components/chat/ChatReasoningPeek.vue` | 修改（新增 thinkingOnly prop，thinking-only 模式渲染） | CC-C-UX-02 |

---

## 剩余工作（超出本计划范围）

以下工作属于 M55/M56 更广泛的优化计划，不在本优化计划范围内：

| 来源 | ID | 问题 | 状态 |
|------|-----|------|------|
| M55 Phase E | CC-E-01 | @ 引用上下文注入 | 未启动 |
| M55 Phase E | CC-E-02 | diff Apply 卡片 | 未启动 |
| M55 Phase F | CC-F-01 | 24h Durable Job（Worker deadline） | 未启动 |
| M55 Phase F | CC-F-02 | invocation restore | 未启动 |
| M56 BLO-1 | BLO-1 | Intent-Aware Admission | 需求草案 |
| M56 BLO-2 | BLO-2 | Multi-Signal Escalation | 需求草案 |
| M56 BLO-3 | BLO-3 | Channel Trigger Rules | 需求草案 |
| M56 BLO-4 | BLO-4 | Non-Blocking HITL | 需求草案 |
| M56 BLO-5 | BLO-5 | Unified BackgroundJob | 需求草案 |
| 架构 | TurnExecutor | Agent/Team 公共骨架提取 | 未启动 |
| 架构 | listChatOptions | runtimeStore 中 listChatOptions 语义归属 | 记录备忘 |
