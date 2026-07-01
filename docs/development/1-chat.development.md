# Chat 对话 — 开发计划

> **版本**：2026-05-23 | **状态**：✅ 端到端可用；Follow-up / Await / Admission 两轮 P1–P2 已收口；ADR-02 + ADR-03 Activity-First 架构迁移已完成  
> **Review**：[2026-05-23-Chat-Flow-Full-Review.md](../review/2026-05-23-Chat-Flow-Full-Review.md)  
> **M55 Cursor 对标**：[55-chat-channel-cursor-solution.md](./55-chat-channel-cursor-solution.md) · [55-chat-channel-cursor-development.md](./55-chat-channel-cursor-development.md)  
> **四层解耦（DECO）**：[0-module-decoupling-architecture.md §3.1](./0-module-decoupling-architecture.md#31-推荐目标架构channel--chat--agent) · 任务板 [17-channel-development.md §14](./17-channel-development.md#14-phase-deco--四层架构解耦deco)（DECO-06/13/14）
> **需求**：[1-chat.md](./1-chat.md) · **设计**：[1-chat.design.md](./1-chat.design.md)  
> **ADR-02**：Activity-First 事件持久化（已归档，设计内容已并入 1-chat.design.md / 34-event-system.design.md）  
> **ADR-03**：统一总线架构（已归档，设计内容已并入 1-chat.design.md / 34-event-system.design.md）  
> **执行卡片 v2**：[1-chat-execution-trace.md](./1-chat-execution-trace.md) · [1-chat-execution-trace.design.md](./1-chat-execution-trace.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—
>
> **文档边界**：本文档包含模块定位、代码锚点、现状评估、差距/优化、阶段划分、任务清单（含状态）、验收标准、改动文件清单。用户故事、功能需求清单、验收标准见 [1-chat.md](./1-chat.md)；架构设计、代码分层、Proto/API 契约、数据模型、接口定义、状态机、序列图、前端组件设计、UX 规范见 [1-chat.design.md](./1-chat.design.md)。

---

## 1. 模块定位

Chat 是用户与 Agent/Team 交互的核心入口，负责 HTTP/WS 发起对话、WebSocket + ActivityEventBus 实时事件（ADR-03 后 Envelope 已删除，统一为 ActivityEvent + MonitorEvent）、上下文管理、用量记录、停止生成、人工等待回复与待执行队列。

**代码锚点**（已校验存在，2026-06-27 ADR-02/03 后）：
- `api/kratos/chat/v1/chat.proto` — Chat RPC（含 `EnqueueUserMessage` → `POST /v1/chat/enqueue`）
- `internal/runtime/run_registry.go` — 会话级 active run / cancel / run status
- `internal/runtime/pending_queue.go` — PendingMessageQueue FIFO（T1.4: 支持 snapshot 持久化，`NewPendingMessageQueueWithDirAndLogger`）
- `internal/server/ws.go` — WebSocket（`user_message` / `cancel` / `enqueue_message`）
- `internal/server/ws_message_handler.go` — WS 上行消息分发（user_message/enqueue_message/cancel/ping/subscribe/unsubscribe/enable_log）
- `internal/biz/chat_usecase.go` — Follow-up Queue 编排（EnqueueUserMessage / Pending CRUD）
- `internal/biz/activity.go` — Activity 模型 + ActivityKind(10) + ActivityEventType(7) + ToolCategory(10)
- `internal/biz/llm_context_builder.go` — LLM 上下文构建（从 Activity 表，替代原 Message 查询）
- `internal/service/chat.go` — ChatService 薄传输桥（委托 ChatOrchestrator）
- `internal/service/chat_orchestrator.go` — ChatOrchestrator 编排核心
- `internal/service/chat_orchestrator_turn.go` — Turn 编排
- `internal/service/chat_native.go` — 原生对话入口（admission + team/agent 路由）
- `internal/service/chat_run_gateway.go` — Biz 适配器 + `publishMessageQueuedToBus`
- `internal/service/turn_outcome.go` — `ErrTurnMessageQueued` / `CHAT_TURN_BUSY`
- `internal/service/turn_pipeline.go` · `turn_pipeline_chat.go` — Turn Pipeline（PersistentTurnService + chatTurnExecutor）
- `internal/agent/choice_stream.go` · `stream_consumer.go` — 流式 delta 与 turn 消费
- `internal/agent/activity_projector.go` — 唯一投影器：trpc event → ActivityEvent（含 Team `member_agent_key`）
- `internal/agent/activity_event_sequencer.go` — ActivityEvent 序列化（并行异步持久化 + WS 推送 + dead-letter + retry）
- `internal/agent/tool_category.go` — 工具类型识别（ToolCategorizer：注册表 + 前缀兜底）
- `internal/agent/activity_meta.go` — ActivityKind 分类
- `internal/event/activity_event.go` — ActivityEvent 类型定义（Event/Activity/Domain）
- `internal/event/activityevent/bus.go` — ActivityEventBus（传输 biz.ActivityEvent，chat+system 事件）
- `internal/event/contract/monitor_event.go` — MonitorEvent 类型 + MonitorBus 接口（log/flow_log/mcp/alert）
- `internal/event/contract/envelope_types.go` — 活类型提取（EnvelopeError/EnvelopeTokenUsage + 5 个 ErrorCode 常量）
- `web/src/features/chat/composables/useChatWorkspace.ts` — Chat 页编排
- `web/src/features/chat/composables/useChatStreamManager.ts` · `useChatSender.ts` · `useFollowUpQueue.ts` · `useAwaitReply.ts`
- `web/src/features/chat/composables/useActivityTimeline.ts` — Activity 时间线（activitiesBySession Map + ensureActivitiesLoaded 缓存）
- `web/src/features/chat/composables/useSystemEventNotification.ts` — Domain=system 事件通知处理
- `web/src/features/chat/ws-transport.ts` — WS 客户端（心跳、重连、控制消息）
- `web/src/components/chat/ActivityStream.vue` — Activity 统一渲染器（按 kind 分发 Block；Phase A 起为递归组件，消费 `activityTree: ActivityTreeNode[]`，按 `children` 嵌套渲染）
- `web/src/components/chat/ActionBlock.vue` — 工具调用块（按 tool_category 细分子组件）
- `web/src/components/chat/SessionTreeSidebar.vue` · `SessionTreeNode.vue` — Session 父子树侧栏（递归）

> **已删除代码锚点**（ADR-02 + ADR-03）：
> - `internal/agent/event_projector.go`（已废弃，被 `activity_projector.go` 取代）
> - `internal/agent/activity_persist.go`（`ChatMessageFromToolActivity` 转换，WBPF 策略废弃）
> - `internal/event/contract/envelope.go`（活类型提取到 `envelope_types.go`）
> - `internal/event/buffer.go`（WS replay Buffer 死代码）
> - `internal/event/bus.go`（SessionBus 死代码）
> - `internal/event/wal.go` · `internal/biz/event_persist_handler.go` · `internal/biz/event_store.go` · `internal/data/message_repo.go`
> - `messages` / `event_store` / `event_wal` 表（已 DROP）
> - 前端：`ConversationTurn.vue`、`TeamPanel.vue`、`useConversationTimeline.ts`、`streamHandlers.ts`、`envelope.ts`、`dispatcher.ts`、`envelopeToolCall.ts`、`envelopeRunStatus.ts`、`inboundSyncEnvelope.ts`

> 代码分层、请求流转、Proto 契约、WebSocket 协议、Biz/Data/Service/运行时层详细设计详见 [1-chat.design.md §二~§七](./1-chat.design.md#二代码分层遵循-ai-development-specificationmd)。

---

## 2. 现状评估（2026-06-27 ADR-02/03 后）

| 项 | 状态 | 证据 |
|----|------|------|
| WS 实时对话 | ✅ | `/v1/ws` + `user_message` + ActivityEventBus（activity_event? + monitor_event?） |
| HTTP unary 对话 | ✅ | `SendChatMessage` / `RunNativeTurnUnary` |
| Channel / Cron 入口 | ✅ | `lockSession` + `RunRegistry` |
| 停止 / 运行中追加 | ✅ | `StopGeneration` / WS `cancel`；`EnqueueUserMessage` |
| 待执行 / Follow-up Queue | ✅ | Steerable + Pending FIFO；WS `enqueue_message` + ActivityEvent（Domain=system）刷新 |
| RunStatus + AwaitUserReply | ✅ | RPC + WS ActivityEvent；`useAwaitReply` submit（Round2） |
| Team member_agent_key Activity | ✅ | `ActivityProjector` + `useActivityTimeline` 成员流 |
| WS 控制消息 | ✅ | `ws-transport`：`connected`/`pong`/`server_shutdown`；replay 协议已删除（改用 ListActivities RPC） |
| 工具事件 UI | ✅ | `ActivityStream` → `ActionBlock`（按 tool_category 细分子组件，10 种） |
| Reasoning UI | ✅ | 默认折叠 `<details>` 展示 `activity.reasoning`（Kind=thinking） |
| RunStatus | ✅ | WS ActivityEvent（Kind=notice/task）驱动；会话切换时 HTTP 快照一次 |
| WS 重连恢复 | ✅ | `ListActivities?since={updated_at}` 拉取增量 → 顶栏「正在同步历史 Activity…」 |
| Team 成员流 UX | ✅ | `activity.member_agent_key` + 左侧色条分栏 |
| 模型选项 | 🟡 | Platform 优先 + `GetChatOptions("model")` 回退 |
| 附件 / Vision | ✅ | Artifact 上传 + `buildUserMessage` 多 part 装配 |
| RunStatus 持久化 | ✅ | `state_json` + trpc `PendingAwaitUserReplyRoute`；resume 同步清状态 + `resumeInFlight` 防双 turn |
| Activity-First 架构 | ✅ | ADR-02 + ADR-03 全部完成；Envelope 已删除，2 bus/2 pump |
| Session 父子树 | ✅ | 9 个新字段 + `GetSessionTree` RPC + `SessionTreeSidebar`/`SessionTreeNode` |
| 工具类别细分 | ✅ | `ToolCategorizer` + 10 种 ToolCategory + 10 种 ActionBlock 子组件 |
| 并行异步持久化 | ✅ | `ActivityEventSequencer`：persist fire-and-forget + publish 同步 + retry 5 次 + dead-letter 512 FIFO |

> API 端点清单、Proto 定义、RunStatus 字段详见 [1-chat.design.md §四](./1-chat.design.md#四proto-层)。

---

## 3. 差距与优化（按优先级）

### P1 — Follow-up Queue UX（Cursor 对齐）

1. ~~**运行中连续发送**~~ ✅ `useChatSender` → `enqueue_message`（Round1）
2. ~~**WS 驱动 Pending 刷新**~~ ✅ `messageQueuedFromEnvelope`（Round1）
3. **（P3）** 可选 `pending_enqueued` ActivityEvent 携带 `pending_id` + 内容预览。

### P1 — Admission / 并发（2026-05-23 收口）

1. ~~Turn starting 并行 turn~~ ✅ `CHAT_TURN_BUSY` + `StoreCancelable`
2. ~~Channel queued 启发式~~ ✅ `ErrTurnMessageQueued`
3. ~~`processPendingQueue` 竞态~~ ✅ lock + 重新入队（T1.3 进一步改为迭代式循环 + inPendingLoop 标志，消除 goroutine 递归）

详见 [Chat Flow Full Review](../review/2026-05-23-Chat-Flow-Full-Review.md)

### P1 — 体验闭环（原）

1. **工具事件结构化卡片**：`tool_call`/`tool_result` → 参数 JSON、结果、耗时、`is_long_running` 折叠面板（`ChatMessagePanel` + `toolEventMarkdown` 扩展）。✅ 已由 `ChatExecutionCard` v2 实现
2. **Reasoning 展示规格**：产品定稿（折叠/内联/侧栏）并在助手气泡渲染 `content.reasoning`。✅ 已实现

### P2 — 实时与 Team UX

3. **RunStatus WS 驱动**：用 `state_delta` 或专用 ActivityEvent 替代 2s `getRunStatus` 轮询。✅ 已实现
4. **Team 成员分栏**：`member-{agent_key}` 消息增加头像、角色标签、独立气泡样式。✅ 已实现
5. **WS 回放 UX**：`ws-transport.onReplayState` → Chat 顶栏「同步历史事件…」提示。✅ 已实现

### P3 — 平台级

6. **多模态附件**：上传 API、对象存储、Vision 输入装配。✅ 已实现
7. **RunStatus 可恢复**：`awaiting_user` 持久化或 EventBuffer 恢复策略。✅ 已实现
8. **模型选项单一真相源**：长期统一为 `GetChatOptions` 或 Platform 之一（当前为 Platform 优先 + 回退）。🟡 回退已实现

### P0 — Cursor 对标（M55，2026-05-23）

> 详案：[55-chat-channel-cursor-development.md](./55-chat-channel-cursor-development.md) Phase C/E · [UX Backlog changelog](../changelog/2026-05-23-M55-Feishu-Rebind-UX-Backlog.md)

1. **ConversationTurn UI**：一轮 = User → ToolStrip（折叠）→ Assistant — **骨架 ✅**（`ConversationTurn.vue`）
2. **Channel 会话同步**：`session_revision` 增量 hydrate — **✅**
3. **滚动锚点**：最后一轮正文 — **✅**
4. **思考/ReAct UX**：互斥呈现、空 reasoning 不展示 — **✅** IM preview sanitize ✅（CH-BOR-12）；Web CC-C-UX-* ✅
5. **（P2）@ 上下文引用**、**diff Apply 卡片** — **📋**

---

## 4. 技术债务与优化方向

> 本节汇总从需求文档与设计文档迁移的技术债务条目，作为后续迭代的输入。

### 4.1 已知技术债务

| 编号 | 问题 | 位置 | 严重度 | 状态 |
|------|------|------|--------|------|
| TD-01 | `ChatService` 历史曾承担过多职责（teams/usage/plugin/skill/webhooks 等） | `internal/service/chat.go` | 中 | ✅ 已重构为薄桥（仅 orch/turnPipeline/lg） |
| TD-02 | 工具卡片曾用 `ChatToolCallCard` 命名，upsert 键易冲突 | 前端 | 中 | ✅ 已升级为 `ChatExecutionCard` v2（`act-{tool_call_id}` 稳定 upsert） |
| TD-03 | `TurnBlock.vue` 组件命名与 Cursor 对标文档不一致 | 前端 | 低 | ✅ 已重命名为 `ConversationTurn.vue` |
| TD-04 | 模型选项双源（Platform + GetChatOptions 回退） | `useChatProviderOptions` | 低 | 🟡 回退已实现，长期统一为单一来源 |
| TD-05 | WS 控制消息页面回放提示缺失 | `ws-transport` | 低 | ✅ 已实现 `replay_start/end` 顶栏提示 |

### 4.2 优化方向（从设计文档 §九 迁移）

| 方向 | 描述 | 优先级 | 状态 |
|------|------|--------|------|
| Activity-First 架构 | 删除 Envelope，统一为 ActivityEvent + MonitorEvent 双类型协议 | P0 | ✅ ADR-03 完成 |
| 并行异步持久化 | persist fire-and-forget + publish 同步 + retry + dead-letter | P0 | ✅ ADR-02 D1-D2 完成 |
| Session 父子树 | 9 个新字段 + GetSessionTree RPC + SessionTreeSidebar | P1 | ✅ 完成 |
| 工具类别细分 | ToolCategorizer + 10 种 ToolCategory + 10 种 ActionBlock 子组件 | P1 | ✅ 完成 |
| Running 态落库 | 执行卡片 running 状态持久化到 activities 表 | P2 | ✅ Activity upsert + StopGeneration 取消落库 |
| catalog display_name 查表 | `ActivityMetaResolver` + ToolUC 统一能力名查表 | P1 | ✅ 已实现 |
| Team 成员标识 | `activity.member_agent_key` + UI 展示执行成员 Agent | P1 | ✅ 已实现 |
| activities 持久化 schema | `activities` 表为唯一真相源；messages/event_store/event_wal 表已 DROP | P1 | ✅ 已实现 |

> 执行卡片 v2 详细设计、状态机、实施分期详见 [1-chat.design.md 子模块](./1-chat.design.md#子模块chat-执行过程卡片--技术设计)。Activity-First 架构设计详见 [1-chat.design.md §十二](./1-chat.design.md#十二activity-first-架构设计adr-02--adr-03-综合)。

---

## 5. 开发阶段

| Phase | 主题 | 状态 |
|-------|------|------|
| 1 | 文档与 WS/EventBus 主通道 | ✅ |
| 2 | Team active run / pending / cancel | ✅ |
| 3 | AwaitUserReply 后端 + Chat UI | ✅ |
| 4 | 数据一致性（pending_id、session_turns、Channel 互斥） | ✅ |
| 5 | Team `member_*` + 成员流消费 | ✅ 协议通；UX 待增强 |
| 6 | 工具可观测 UI（基础卡片） | ✅ |
| 6b | 执行过程卡片 v2（Skill/MCP/默认折叠/持久化 schema） | ✅ P0 |
| 7 | Reasoning 展示 | ✅ | 折叠 UI + 空壳/双轨（ReAct vs reasoning）已收口 CC-C-UX-01~03 |
| 8 | 附件 / RunStatus 持久化 | ✅ |
| 9 | M55 ConversationTurn + Channel 同步 | ✅ | Phase A–D ✅；UX 收口 CC-C-UX-* ✅ |
| 10 | **ADR-02 + ADR-03 Activity-First 架构迁移** | ✅ | Envelope 删除 + 统一总线 + 并行异步持久化 + Session 父子树 + 工具类别 |
| P-N | **流式渲染与活动排序修复** | ✅ | 统一 MD 渲染路径 + seq On* 入口预分配 + sequencer v2 单 publish worker（ADR-06） |
| 11 | **三种模式数据模型 + WS 协议设计文档** | ✅ | 需求文档 §1.7 + 设计文档 §5.4 + B.2.1（Phase T7） |

### P-N 流式渲染与活动排序修复（2026-06-27）

> **ADR-06**：单 publish worker 重设计（详见 [ADR-06](../reports/2026-06-27-review-adr-activity-event-sequencer-redesign.md)）
> **Plan**：[docs/superpowers/plans/2026-06-27-chat-ui-streaming-fix.md](../superpowers/plans/2026-06-27-chat-ui-streaming-fix.md)

#### 任务清单
- [x] T-N.1 删除 `renderStreamingChatMarkdown` 简化路径（Plan Task 1-3）
- [x] T-N.2 seq 在 `OnXxx` 入口主流程分配（Plan Tasks 4-6）
- [x] T-N.3 重写 sequencer 为单 publish worker（Plan Tasks 7-11）
- [x] T-N.4 端到端验证（Plan Task 12 — 用户人工验证）

#### 改动文件清单
- `web/src/features/chat/chatMessageMarkdown.ts`（统一 MD 渲染路径）
- `web/src/features/chat/__tests__/chatMessageMarkdown.spec.ts`（流式 MD 测试，新）
- `internal/agent/activity_projector.go`（seq 在 On* 入口预分配）
- `internal/agent/activity_event_sequencer.go`（单 publish worker 重写）
- `internal/agent/activity_projector_seq_test.go`（seq 预分配不变量测试，新）
- `internal/agent/activity_event_sequencer_v2_test.go`（v2 sequencer 测试，新）
- `internal/agent/activity_cross_order_e2e_test.go`（跨 activity 顺序 E2E 测试，新）
- `internal/agent/activity_event_sequencer_bench_test.go`（v2 sequencer 吞吐量基准，新）
- `docs/superpowers/specs/2026-06-27-chat-ui-streaming-fix-design.md`（设计文档，新）
- `docs/superpowers/plans/2026-06-27-chat-ui-streaming-fix.md`（实施计划，新）
- `docs/reports/2026-06-27-review-adr-activity-event-sequencer-redesign.md`（ADR-06，新）
- `docs/notes/2026-06-27-known-test-flakes.md`（已知测试 flake 记录，新）

### 5.1 Phase 10 ADR-02 + ADR-03 迁移任务块

> **来源**：ADR-02 + ADR-03

#### ADR-02 Activity 事件持久化（全部 ✅）

| 决策 | 描述 | 状态 |
|------|------|------|
| D1 | 并行异步持久化（persist fire-and-forget + publish 同步，替代 WBPF） | ✅ |
| D2 | 重试预算（5 次指数退避 100/200/400/800/1600ms）+ 死信环形缓冲（512 FIFO + activityID 去重）+ API backfill | ✅ |
| D3 | OnError 语义（删除 ActivityKindError，失败统一通过 task.failed 表达） | ✅ |
| D4 | legacy ActivityKind 清理（删除 ActivityKindError/ActivityKindMember 等） | ✅ |

#### ADR-03 统一总线架构（Phase 1-5 全部 ✅）

| Phase | Blocker | 描述 | 状态 |
|-------|---------|------|------|
| 1 | — | ActivityEvent 新增 Domain 字段 + MonitorEvent 类型拆分 | ✅ |
| 1 | — | 80+ publisher 迁移到 ActivityEventBus/MonitorBus | ✅ |
| 1 | — | WSServer 3 bus/3 pump → 2 bus/2 pump | ✅ |
| 1 | — | ActivityProjector.EmitSystemEvent（Domain=system 不持久化） | ✅ |
| 1 | — | 前端统一到 ActivityEvent + MonitorEvent + 本地类型解耦 | ✅ |
| 5 | A: WS Replay 路径 | 删除 EventBuffer replay，改用 ListActivities RPC 拉取增量 | ✅ |
| 5 | B: 4 个 side consumer | 迁移到 ActivityEventBus/MonitorBus 订阅 | ✅ |
| 5 | C: DomainEvent bridge | 迁移到 ActivityEventBus + ActivityDomainSystem | ✅ |
| 5 | D: vestigial bus 字段 | 删除 3 个死发布者（EmitProgress/LogError/PublishSessionRevisionEnvelope） | ✅ |
| 5 | E: EventPipeline | 删除 EventPipeline.Bus/Buffer | ✅ |
| 5 | F: Wire DI | 移除 ProvideSessionBus + 8 个 EventBusConsumer 迁移 | ✅ |
| 5 | G: contract/envelope.go | 活类型提取到 envelope_types.go，删除死代码 | ✅ |

#### Phase 10 后续清理（全部 ✅）

| 任务 | 描述 | 状态 |
|------|------|------|
| Session 7 字段断层补全 | api/kratos/session/v1/session.proto + toProtoSession + 前端 types.ts/api.ts + SessionTreeNode.vue | ✅ |
| 子 Session Activity 懒加载缓存 | useActivityTimeline.ensureActivitiesLoaded（缓存命中跳过 RPC） | ✅ |
| 前端 Envelope import 清理 | envelope.ts/dispatcher.ts/data_channel.ts/event_replay.ts/features/chat/dispatcher.ts 删除 | ✅ |
| 后端 Envelope 死代码清理 | transcript.go/tool_display.go/parity_run_test.go 死方法清理 | ✅ |

---

## 6. 任务清单

| # | 任务 | 优先级 | 状态 |
|---|------|--------|------|
| 1 | 文档移除 SSE 主路径 | P1 | ✅ |
| 2 | Team turn active run / pending | P1 | ✅ |
| 3 | Team defer `processPendingQueue` | P1 | ✅ |
| 4 | AwaitUserReply Chat UI | P1 | ✅ |
| 5 | `Activity.PendingID` 统一（原 EnvelopeError.PendingID） | P1 | ✅ |
| 6 | WS 控制消息协议 + transport | P1 | ✅ |
| 7 | `recordTeamSessionTurn` | P2 | ✅ |
| 8 | Channel/Cron 互斥 | P2 | ✅ |
| 9 | Team `member_agent_key` 发射与消费 | P2 | ✅ |
| 10 | 工具事件结构化卡片 | P1 | ✅ |
| 11 | Reasoning 展示规格与实现 | P1 | ✅ |
| 12 | ~~EventBuffer TTL~~ → ListActivities RPC 重连 | P2 | ✅ ADR-03 Blocker A |
| 13 | RunStatus WS 事件驱动 | P2 | ✅ |
| 14 | 多模态附件后端 | P3 | ✅ |
| 15 | 模型选项单一来源（长期） | P3 | 🟡 回退已实现 |
| 16 | RunStatus 持久化 | P3 | ✅ `state_json` + `PendingAwaitUserReplyRoute` |
| 17 | ChatService / WS 单测 | P1 | ⏳ |
| 18 | RunRegistry + EnqueueUserMessage | P0 | ✅ |
| 19 | 执行过程卡片 v2（EnvelopeToolCall v2 → Activity.ToolCall + tool_category） | P0 | ✅ |
| 20 | 执行卡片持久化 + catalog 名 + Team 标识 + 流式修复 | P1 | ✅ |
| 21 | **T1.1** 移除 24h hard deadline + Team/pending/resumeAwait 路径统一 WithCancel（No-Timeout 原则） | P0 | ✅ Sprint 1 |
| 22 | **T1.2** LLM 无限重试 + llm_retry 事件 + RetryCallback 回调模式 | P0 | ✅ Sprint 1 |
| 23 | **T1.3** processPendingQueue 迭代式循环（替代 goroutine 递归）+ inPendingLoop 标志 | P0 | ✅ Sprint 1 |
| 24 | **T1.4** PendingMessageQueue snapshot 持久化（进程重启恢复） | P0 | ✅ Sprint 1 |
| 25 | **ADR-02 D1** ActivityEventSequencer 并行异步持久化 | P0 | ✅ Phase 10 |
| 26 | **ADR-02 D2** 重试 5 次 + 死信 512 FIFO + activityID 去重 + API backfill | P0 | ✅ Phase 10 |
| 27 | **ADR-02 D3** OnError 语义（删除 ActivityKindError，task.failed 表达） | P0 | ✅ Phase 10 |
| 28 | **ADR-02 D4** legacy ActivityKind 清理 | P0 | ✅ Phase 10 |
| 29 | **ADR-03 D1** ActivityEvent Domain 字段（chat/system） | P0 | ✅ Phase 10 |
| 30 | **ADR-03 D2** MonitorEvent 类型拆分 + MonitorBus 接口 | P0 | ✅ Phase 10 |
| 31 | **ADR-03 D3** 80+ publisher 迁移 | P0 | ✅ Phase 10 |
| 32 | **ADR-03 D4** ActivityProjector.EmitSystemEvent | P0 | ✅ Phase 10 |
| 33 | **ADR-03 D5** WSServer 2 bus/2 pump | P0 | ✅ Phase 10 |
| 34 | **ADR-03 D6** 前端统一 ActivityEvent + MonitorEvent + 本地类型解耦 | P0 | ✅ Phase 10 |
| 35 | **ADR-03 Blocker A** WS Replay 删除 + ListActivities RPC 重连 | P0 | ✅ Phase 5 |
| 36 | **ADR-03 Blocker B** 4 个 side consumer 迁移 | P0 | ✅ Phase 5 |
| 37 | **ADR-03 Blocker C** DomainEvent bridge 迁移 | P0 | ✅ Phase 5 |
| 38 | **ADR-03 Blocker D** vestigial bus 字段删除 | P0 | ✅ Phase 5 |
| 39 | **ADR-03 Blocker E** EventPipeline.Bus/Buffer 删除 | P0 | ✅ Phase 5 |
| 40 | **ADR-03 Blocker F** Wire DI SessionBus 移除 | P0 | ✅ Phase 5 |
| 41 | **ADR-03 Blocker G** contract/envelope.go 活类型提取 + 死代码删除 | P0 | ✅ Phase 5 |
| 42 | Session 7 字段断层补全（proto + toProtoSession + 前端 + SessionTreeNode） | P1 | ✅ |
| 43 | 子 Session Activity 懒加载缓存（ensureActivitiesLoaded） | P1 | ✅ |
| 44 | 前端 Envelope import 清理（5 文件删除） | P0 | ✅ |
| 45 | 后端 Envelope 死代码清理（transcript/tool_display/parity_run_test） | P0 | ✅ |
| 46 | **T7.1** 需求文档新增 §1.7 三种对话模式（Activity 树结构 + 对比表 + 统一数据模型） | P1 | ✅ |
| 47 | **T7.2** 设计文档新增 §5.4 WebSocket 通信协议设计 | P1 | ✅ |
| 48 | **T7.3** 设计文档 B.2.1 三种模式渲染规则更新 | P1 | ✅ |
| 49 | **T7.4** 需求文档子模块 A.3 用户故事更新 | P1 | ✅ |
| 50 | **T7.5** 开发计划文档新增 T7 | P1 | ✅ |
| 51 | **T8.1** 左侧面板宽度 280→330px（`_css-vars-light.sass` + `_css-vars-dark.sass`） | P1 | ✅ |
| 52 | **T8.2** 左侧面板改为精灵下方 Agent 卡片列表（不分组折叠，按创建顺序） | P1 | ✅ |
| 53 | **T8.3** Agent 卡片：头像+名称+agentKey+团队名+状态动画+暂停/恢复按钮 | P1 | ✅ |
| 54 | **T8.4** 阻塞状态检测 composable（`useBlockedStatus`，4 种阻塞类型） | P1 | ✅ |
| 55 | **T8.5** 中间活动流去除深框嵌套（GraphStageBlock/TeamCard/AgentCard 改树形缩进+连接线） | P1 | ✅ |
| 56 | **T8.6** 点击 Agent 卡片定位到中间面板会话+高亮闪烁 | P1 | ✅ |
| 57 | **T8.7** 执行中转圈动画 + 已完成绿色标签 + 阻塞黄色高亮 | P1 | ✅ |
| 58 | **T8.8** 设计文档同步 B.8.3 阻塞定义 + B.9 树形重构设计 | P1 | ✅ |

### T8 UI 树形重构（2026-07-01 新增）

> **目标**：消除深框嵌套感，改用树形缩进+连接线；左侧面板改为 Agent 卡片列表；阻塞状态精确定义。

#### 改动文件清单

| 文件 | 改动 |
|------|------|
| `web/src/css/theme/_css-vars-light.sass` | `--chat-side-left-width: 280px → 330px` |
| `web/src/css/theme/_css-vars-dark.sass` | `--chat-side-left-width: 280px → 330px` |
| `web/src/css/app-global.sass` | `chat-message-prose img` 限制 `max-width: 100%`，防止大图撑破布局 |
| `web/src/components/chat/ChatEntitySidebar.vue` | 新增 Agent 卡片列表区域（精灵下方），移除原分组折叠逻辑；转发 agents 与 settings 事件 |
| `web/src/components/chat/AgentSidebarCard.vue` | 新建：左侧面板 Agent 卡片组件；隐藏 `agentKey`；从 Agent 库补充头像；常驻设置按钮；优化间距字号 |
| `web/src/features/chat/composables/useBlockedStatus.ts` | 新建：阻塞状态检测 composable；LLM 阻塞判定需 `meta.streaming === false`；支持 agentKey 继承 |
| `web/src/components/chat/GraphStageBlock.vue` | 移除 border+background，改树形行式布局 |
| `web/src/components/chat/TeamCard.vue` | 移除 border+background，改树形行式布局；新增 `autoExpand` prop 支持外部触发展开 |
| `web/src/components/chat/AgentCard.vue` | 移除 border+background，改树形行式布局；新增 `autoExpand` prop 支持外部触发展开 |
| `web/src/components/chat/ActivityStream.vue` | 透传 `autoExpandFor` prop 到 TeamCard/AgentCard；TeamCard `autoExpand` 支持任意成员 agentKey 匹配；递归子流同步透传 |
| `web/src/components/chat/ChatMessageList.vue` | 监听 `useScrollToActivity` 定位命令；自动展开父级卡片；`data-agent-key`/`data-team-id` 定位；黄色高亮动画 |
| `web/src/features/chat/composables/useScrollToActivity.ts` | 新建：模块级 ref 单例，跨组件传递定位命令 |
| `web/src/components/spirit/SynthesisResultCard.vue` | `team-summary` / `team-findings` 改为完整换行显示；`renderedContent` 内图片限制最大宽度 |
| `web/src/i18n/locales/zh-CN.ts` / `en-US.ts` | 新增 `chat.agentSidebar.settings` 等文案 |
| `web/src/features/chat/streamEventTypes.ts` | `TeamMemberStatus` 新增 `blocked` 状态 |
| `web/src/features/chat/composables/useActivityTimeline.ts` | members 映射逻辑新增 blocked 状态 |

---

## 7. 验收标准

### 7.1 核心对话（历史）

- [x] 无 `/v1/chat/messages/stream` 当前端点表述
- [x] WS 控制消息在需求/设计文档中完整描述
- [x] Team 停止/待执行与单 Agent 一致
- [x] AwaitUserReply：后端 + Chat 页可提交回复
- [x] `activity.pending_id` 前端可消费（原 `error.pending_id`）
- [x] `session_turns` Agent + Team 均有记录
- [x] Channel/Cron 不绕过 active run 互斥
- [x] Team `member_agent_key` 后端发射 + 前端增量展示
- [x] 工具执行结构化卡片（参数/结果/耗时/`is_long_running`）
- [x] Reasoning 折叠/展示符合产品规格
- [x] RunStatus 与 WS ActivityEvent 一致（切换会话时 HTTP 校准）
- [x] WS 重连后用户可见「同步中」状态（ListActivities RPC 拉取增量）
- [x] `go test ./internal/service/... -run TestChat` 通过

### 7.2 Activity-First 架构（ADR-02 + ADR-03）

- [x] Envelope 通用信封已删除，WS 下行为 `activity_event?` + `monitor_event?` 双类型协议
- [x] 10 种 ActivityKind + 7 种 ActivityEventType + ActivityDomain 全覆盖
- [x] `ActivityStream.vue` 统一渲染器按 `activity.kind` 分发到 10 种 Block 组件
- [x] 并行异步持久化：persist fire-and-forget + publish 同步 + retry 5 次 + dead-letter 512 FIFO
- [x] `select` on done channel（Close 不阻塞）
- [x] `messages`/`event_store`/`event_wal` 表已 DROP，`activities` 表为唯一真相源
- [x] WSServer 2 bus/2 pump（ActivityEventBus + MonitorBus）
- [x] 前端 Envelope import 清理（5 文件删除）
- [x] 后端 Envelope 死代码清理（transcript/tool_display/parity_run_test）

### 7.3 Session 父子树 + 工具类别

- [x] Session 表含 9 个父子树字段 + `GetSessionTree` RPC
- [x] 深度受 `subagents_max_generation_depth`（默认 3）+ `max_session_depth`（默认 5）限制
- [x] 前端 `SessionTreeSidebar.vue` + `SessionTreeNode.vue`（递归）渲染
- [x] 子 Session Activity 懒加载缓存（`ensureActivitiesLoaded` 命中跳过 RPC）
- [x] 10 种 ToolCategory + `ToolCategorizer`（精确匹配 + 前缀兜底）
- [x] `ActionBlock.vue` 按 `tool_category` 动态渲染 10 种差异化子组件
- [x] `team_stage` / `graph_stage` / `session` Activity + 对应 Block 组件

### 7.4 Activity 树嵌套渲染（Phase A）

> 2026-06-27：完成「主聊天流嵌套渲染 + 统一渲染器对齐 + 父子场景 bug 修复」一轮 P1/P2 修复。设计依据：[1-chat.design.md §聊天消息分组](./1-chat.design.md) + [1-chat.design.md §子模块：Team 团队历史显示设计](./1-chat.design.md)。

| ID | 类型 | 改动 | 状态 |
|----|------|------|------|
| B-04 | P1 架构 | `ActivityStream.vue` 改为递归组件（`defineOptions({ name: 'ActivityStream' })`），prop 从 `activities: Activity[]` 改为 `activityTree: ActivityTreeNode[]`；删除 L143 `activityToStreamEvent({ ...activity, children: [] })` 中 `children: []` 覆盖（根因：剥离 children 破坏树结构，递归渲染无法看到子节点）；`PlanBlock.vue` 不再渲染子 Activity（交给 ActivityStream 树层）；`ChatMessageList`/`ChatMessagePanel`/`ChatPage` props 改为 `activityTree`；新增 `B-04` 嵌套渲染单测 | ✅ |
| B-05 | P1 bug | `TeamStageBlock.vue` `expand-member` emit 增补 `teamId`（来源 `activity.teamId`）；`ChatPage.onExpandMember` 接收 `teamId` 优先从 payload 取 team，缺失回退 `spiritStore.activeTeam` | ✅ |
| B-06 | P1 bug | `SessionStageBlock.vue` 加 `enter-session` emit + `canEnter` 计算属性 + `@click="onEnter"` + `--clickable` 悬停样式 + 右侧 chevron 图标；`ChatPage.onEnterSession` 调用 `useActivityTimeline.setCurrentSession` + `ensureActivitiesLoaded` 进入子 Session | ✅ |
| B-07 | P2 bug（后端） | `internal/agent/activity_projector.go` `processGraphNodeStart` 创建 plan Activity 时增加 `ParentActivityID: p.rootActivityID`（根因：原代码未设置导致 plan 成为 task 的兄弟根，破坏 Activity 树父子层级，前端递归渲染失效） | ✅ |
| B-08 | P2 bug | `useActivityTimeline.ts` `case 'created'` 由 `map.set(snapshot.id, snapshot)` 改为 merge：`map.set(snapshot.id, existing ? { ...snapshot, ...existing } : snapshot)`（根因：WS 事件乱序时 `streaming` 可能先于 `created` 到达，覆盖会丢失已累积的 content/reasoning） | ✅ |
| GAP-04/05 | P1 UX | `SessionTreeNode.vue` 图标映射：`agent`→`smart_toy`（机器人，agent 非人类用户）；`standalone`→`chat`（单对话，forum 暗含多人）；fallback 同步 | ✅ |

**验证**：
- 后端：`go build ./internal/agent/...` ✅；`go test ./internal/agent/... -run TestActivityProjector` ✅（0.362s，全通过）
- 前端：`pnpm lint` 0 errors；`pnpm test` 540 tests pass（新增 1 个 B-04 嵌套渲染测试）；`pnpm build` ✅

**未做（YAGNI）**：
- B-09/GAP-03 depth 错误码 + i18n 文案：当前深度限制由后端 `subagents_max_generation_depth` / `max_session_depth` 已守门，前端仅依靠错误回退展示，无明确 UX 缺陷反馈。**待用户反馈具体场景再补**，避免为单一场景预抽 i18n key 与错误码层级（红线 #15 / CS-F9）。

### 7.5 T8 UI 树形重构验收（2026-07-01 新增）

- [x] 左侧面板宽度 330px，Spirit 入口下方平铺 Agent 卡片，按创建顺序排列，不分组折叠
- [x] Agent 卡片展示头像、显示名称（隐藏 `agentKey` / `__memory__` 等原始标识）、团队名、状态动画与操作
- [x] Agent 头像优先使用后端 `avatarUrl`，缺失时从用户 Agent 库按 `agentKey` 补充
- [x] Agent 卡片常驻设置按钮（⚙），点击可打开对应 Agent 设置弹窗
- [x] 中间活动流移除深框嵌套，`GraphStageBlock`/`TeamCard`/`AgentCard` 改用树形缩进 + 左侧连接线
- [x] 执行中显示青色转圈动画，阻塞显示黄色脉冲 + ⚠ 标签，已完成显示绿色 ✓ 标签
- [x] 阻塞状态基于后端状态机判定，LLM 阻塞需满足 `meta.streaming === false`，避免正常流式被误判
- [x] 子活动未携带 `agentKey` 时继承父节点 `agentKey`，左侧 Agent 卡片可精确高亮
- [x] 点击左侧 Agent 卡片 → 中间面板自动滚动并黄色高亮闪烁 3 次
- [x] 点击定位时若父级卡片折叠，自动展开 `TeamCard`/`AgentCard` 确保目标可见；多成员团队下任意成员均可触发父级 `TeamCard` 展开
- [x] 定位使用模块级 ref 单例 `useScrollToActivity`，通过 `data-agent-key`/`data-team-id` 查找 DOM 并做 `CSS.escape` 转义
- [x] `SynthesisResultCard` 团队总结与关键发现完整换行显示，不截断；`chat-message-prose` 与卡片内容区图片限制最大宽度，防止撑破布局

**验证**：
- 前端：`pnpm lint` 通过（0 errors，剩余 17 warnings 为历史技术债：测试文件 one-component-per-file、未使用变量 upsertEvent）；`pnpm test` 570 passed；`pnpm build` ✅
- 运行截图验证：左侧 Agent 卡片列表、树形缩进+连接线、阻塞黄色高亮、点击定位高亮均符合 B 方案设计

---

## 8. 依赖与风险

- Team 成员 UX 依赖 `MemberAgentKey` 在 turn 元数据中完整传递
- 工具卡片依赖 `ActivityProjector` 对 `duration_ms` / `is_long_running` / `tool_category` 的稳定填充
- RunStatus 持久化可能依赖 M2 多租户 Session 改造
- 附件闭环依赖 Artifact / 对象存储与 LlmProvider Vision 能力
- Domain 字段语义负担：publisher 必须正确声明 Domain，错误声明会导致 system 事件被持久化或 chat 事件被丢弃


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
- Modify: `web/src/components/chat/ChatMessagePanel.vue`
- Modify: `web/src/pages/ChatPage.vue`
- Modify: `web/src/features/chat/composables/useChatWorkspace.ts`

### 根因

`ChatMessageAttachments` 展示组件内 `import { useArtifactStore }` 并在 `onDownload`/`onDelete` 中直接调用 Store 方法 + `$q.notify`，违反红线 #1 和 #3。

### 修复方案

1. 移除 `useArtifactStore` / `useQuasar` import
2. `onDownload` 改为 `emit('download', meta)`，`onDelete` 改为 `emit('deleted', id)`
3. `ChatMessagePanel` → ChatPage 逐层转发 `download-artifact` 事件
4. `useChatWorkspace` 新增 `downloadArtifact` 方法，调用 `useArtifactStore` + 处理通知

> 注：原方案提及 `ChatMessageRow.vue` 作为中间转发层，但该组件在当前代码中不存在；实际转发链路为 `ChatMessageAttachments` → `ChatMessagePanel` → `ChatPage`。

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
| 5 | 🟠 高 | ConversationTurn 缺少 `@retry`/`@dismiss-failed` 事件转发 | 添加事件转发 |
| 6 | 🟡 中 | `regenerateMessage` 未停止活跃 Run | 重生前先 cancel + stop |
| 7 | 🟢 低 | `isFailedPendingMessage` 导出但无调用者 | 删除死代码 |

### 第二轮 Review（3 个问题）

| # | 严重度 | 问题 | 修复 |
|---|--------|------|------|
| 1 | 🟡 建议 | ConversationTurn 成员消息组件缺少事件转发 | 添加 `@retry`/`@dismiss-failed`/`@regenerate` |
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
| `web/src/components/chat/ChatMessagePanel.vue` | 修改（事件转发 + download-artifact） |
| `web/src/components/chat/ConversationTurn.vue` | 修改（事件转发 retry/dismiss-failed/regenerate） |
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
| `web/src/components/chat/ChatReasoningPeek.vue` | 修改（新增 thinkingOnly prop，thinking-only 模式渲染） | CC-C-UX-02 |

> 注：原方案提及 `ChatMessageRow.vue` 的 reasoning v-if 修复（CC-C-UX-01/02），但该组件在当前代码中不存在；实际 reasoning 展示逻辑由 `ChatReasoningPeek.vue` 承载。

---

## 剩余工作（超出本计划范围）

以下工作属于 M55/M56 更广泛的优化计划，不在本优化计划范围内：

| 来源 | ID | 问题 | 状态 |
|------|-----|------|------|
| M55 Phase E | CC-E-01 | @ 引用上下文注入 | 未启动 |
| M55 Phase E | CC-E-02 | diff Apply 卡片 | 未启动 |
| M55 Phase F | CC-F-01 | 24h Durable Job（Worker deadline） | ✅ T1.1 移除（No-Timeout 原则：任务无时间上限） |
| M55 Phase F | CC-F-02 | invocation restore | 未启动 |
| M56 BLO-1 | BLO-1 | Intent-Aware Admission | 需求草案 |
| M56 BLO-2 | BLO-2 | Multi-Signal Escalation | 需求草案 |
| M56 BLO-3 | BLO-3 | Channel Trigger Rules | 需求草案 |
| M56 BLO-4 | BLO-4 | Non-Blocking HITL | 需求草案 |
| M56 BLO-5 | BLO-5 | Unified BackgroundJob | 需求草案 |
| 架构 | TurnExecutor | Agent/Team 公共骨架提取 | 未启动 |
| 架构 | listChatOptions | runtimeStore 中 listChatOptions 语义归属 | 记录备忘 |

---

## 子模块：Team 团队历史显示开发计划

> **状态**：2026-06-28 新增 | **需求**：详见 [1-chat.md §子模块：Team 团队历史显示需求](./1-chat.md) | **设计**：详见 [1-chat.design.md §子模块：Team 团队历史显示设计](./1-chat.design.md)

### C.1 模块定位

Team 团队历史显示是 Chat UI 的核心子模块，负责在精灵对话中展示任务计划、Graph 流程图、Team 任务栏和子 Agent 卡片，并支持历史恢复。

**核心原则**：Agent 统一性——所有 agent（精灵+子agent）本质相同，UI 展示和后端逻辑统一，team 只是组合外衣。

### C.2 代码锚点

#### 后端

| 文件 | 职责 | 状态 |
|------|------|------|
| `internal/biz/activity_seq.go` | GlobalSeqAllocator（全局 Seq 分配） | ❌ 待移除 |
| `internal/agent/activity_event_sequencer.go` | ActivityEvent 序列化（单 publish worker） | ✅ 保留 |
| `internal/service/spirit_team.go` | Spirit 生成事件（SpiritSessionID 已填） | ✅ 已正确 |
| `internal/service/pre_planning_gate.go` | Spirit 生成 plan 事件（SpiritSessionID = SessionID） | ✅ 已修复 |
| `internal/team/runner_team_trpc_phases.go` | Team 阶段事件生成（含 deriveSpiritSessionID 辅助函数、createInitialTeamRun 设置字段） | ✅ 已修复 |
| `internal/team/team_graph_run_coordinator.go` | Team Graph 协调器（team_stage 事件） | ✅ 已修复 |
| `internal/team/team_graph_run_finisher.go` | Team Graph 完成事件（含 run.SpiritSessionID 回填） | ✅ 已修复 |
| `internal/team/runner_team_turn.go` | Team turn 事件 | ✅ 已修复 |
| `internal/team/runner_helpers.go` | publishTeamRunFailedActivity（team_stage/failed）+ publishTeamStepActivity（session/executing|completed，携带 AgentName + meta.child_session_id，Phase T6.3+T6.4 修复） | ✅ 已修复 |
| `internal/team/summary.go` | TeamSummaryActivityEvent 读取 run.SpiritSessionID | ✅ 已修复 |
| `internal/team/status_projector.go` | OrchestrationProjectorConfig 新增 SpiritSessionID 字段 | ✅ 已修复 |
| `internal/team/runner_team_observer.go` | startObservers 设置 SpiritSessionID | ✅ 已修复 |
| `internal/team/team_graph_run_context.go` | GraphRunStepContext 新增 SpiritSessionID 字段 | ✅ 已修复 |
| `internal/team/runner_mediator.go` | TeamGraphCoordAccess 接口签名扩展 | ✅ 已修复 |
| `internal/team/runner_team_compiler.go` | RegisterTeamGraphExecution 调用方传入 spiritSessionID | ✅ 已修复 |
| `internal/biz/team_types.go` | TeamRun 新增 SpiritSessionID 字段（`json:"-"`） | ✅ 已修复 |
| `internal/biz/graph_runtime.go` | GraphRunnerFactory 接口签名扩展 | ✅ 已修复 |
| `internal/biz/graph_execution_usecase.go` | RegisterTeamGraphExecution/ExecuteGraph 等设置 exec.SpiritSessionID | ✅ 已修复 |
| `internal/biz/graph.go` | GraphUsecase.RegisterTeamGraphExecution 委托方法签名扩展 | ✅ 已修复 |
| `internal/graph/trpc/event_bridge.go` | Graph 事件桥接（graph_stage 事件，EventBridge 新增 spiritSessionID 字段） | ✅ 已修复 |
| `internal/graph/runtime_replanner.go` | Graph 重规划事件 | ✅ 已修复 |
| `internal/graph/topology_evolution.go` | 拓扑演化事件（publishTopologyEvolvedEvent） | ✅ 已修复 |
| `internal/graph/adapter/runtime_adapter.go` | trpcGraphRuntime 新增 spiritSessionID 字段，签名扩展 | ✅ 已修复 |
| `internal/event/activityevent/bus.go` | ActivityEvent Bus（SessionID 兜底规范化） | ✅ 保留 |
| `internal/data/ent/schema/agent_runtime_setting.go` | AgentRuntimeSetting（含 MaxSessionDepth） | ✅ 已存在 |
| `internal/data/ent/schema/activity.go` | Activity Schema（含 seq 字段） | ⚠️ 保留字段，停止赋值 |
| `internal/data/activity_repo.go` | Activity Repo（查询排序） | ❌ 待修改排序逻辑 |
| `internal/service/session.go` | ListBySession RPC（历史加载入口） | ✅ 保留 |

#### 前端

| 文件 | 职责 | 状态 |
|------|------|------|
| `web/src/components/chat/ActivityStream.vue` | Activity 统一渲染器（含 silent tool 过滤、空 thinking 过滤） | ✅ 已修复（P0/P1） |
| `web/src/components/chat/TeamStageBlock.vue` | 现有 team_stage 渲染 | ⚠️ 待替换为 TeamCard.vue |
| `web/src/components/chat/AgentCard.vue` | AgentCard 组件（系统 agentKey 显示系统状态、按钮可见性控制） | ✅ 已修复（P0） |
| `web/src/components/chat/TeamCard.vue` | TeamCard 组件（成员名 display_name 查询、按钮可见性控制） | ✅ 已修复（P0） |
| `web/src/components/chat/PlanBlock.vue` | PlanBlock 组件 | ⚠️ 需增加折叠/初始渲染摘要 |
| `web/src/features/chat/composables/useActivityTimeline.ts` | Activity 时间线 + compareActivities（kind 优先级排序） | ✅ 已修复（P1） |
| `web/src/features/chat/composables/useChatMessageScroll.ts` | 自动滚动（rAF 节流 + final reply 触发） | ✅ 已修复（P1） |
| `web/src/stores/spirit/index.ts` | Spirit Store（含 spiritTeam 状态） | ⚠️ 需适配新设计 |
| `web/src/features/chat/api.ts` | API 调用 | ⚠️ 需新增 inject/pause/resume/retry API |

### C.3 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Activity-First 架构 | ✅ | ActivityStream.vue 递归渲染，按 kind 分发 |
| 单 publish worker | ✅ | activity_event_sequencer.go 保证顺序 + 单调 Version |
| GlobalSeqAllocator | ❌ 过度设计 | 需移除，用 Timestamp 排序 |
| Team/Graph 事件 SpiritSessionID | ✅ 已修复 | T1.2/T1.3/T1.3.1 完成，包含 helper/summary/projector/topology_evolution/pre_planning_gate 站点 |
| TeamCard 组件 | ✅ 已修复 | 成员名 display_name 查询、按钮可见性控制（P0） |
| AgentCard 组件 | ✅ 已修复 | 系统 agentKey 显示系统状态、按钮可见性控制（P0） |
| PlanBlock 折叠 | ❌ 未实现 | 需新增 |
| 子 session 懒加载 | ⚠️ 部分 | useActivityTimeline 有 ensureActivitiesLoaded，需适配 |
| MaxSessionDepth 配置 | ✅ 已存在 | AgentRuntimeSetting.MaxSessionDepth |
| ToolCategorizer 注入 | ✅ 已修复 | 生产路径注入 NewToolCategorizerFromCatalog（P0） |
| RunRegistry.Cancel 竞态 | ✅ 已修复 | cancelMu 串行化 + double-check active run（P0） |
| 系统 status 事件 kind | ✅ 已修复 | run_status_publish/run_heartbeat 改为 ActivityKindNotice（P0）；orchestration_started/checkpoint/interrupted、teams_all_completed、synthesis_completed、message_queued、await_resumed、graph checkpoint 事件从 ActivityKindSession 改为 ActivityKindNotice（P0-5） |
| AgentCard isSystemAgentKey | ✅ 已修复 | 移除 agent___ 前缀误判（agent___ 是团队成员正常前缀，非系统 agent）（P0-7） |
| TeamCard/AgentCard 取消按钮 | ✅ 已修复 | 增加 childSessionId/teamId 存在性检查，避免无目标取消（P0-8） |
| 自动滚动 | ✅ 已修复 | useChatMessageScroll rAF 节流 + final reply 触发（P1） |
| TeamRun 显式状态机 | ✅ 已新增 | team_run_state_machine.go（AS-FSM-01，P2） |

### C.4 Phase 划分

#### Phase T1: 后端修复（P0）

**目标**：修复跨 session 聚合盲区，移除过度设计的全局 Seq

| 任务 | 文件 | 状态 |
|------|------|------|
| T1.1 移除 GlobalSeqAllocator | `internal/biz/activity_seq.go` + 引用处 | ✅ |
| T1.2 修复 Team 事件 SpiritSessionID 填充 | `internal/team/runner_team_trpc_phases.go`、`internal/team/team_graph_run_coordinator.go`、`internal/team/team_graph_run_finisher.go`、`internal/team/runner_team_turn.go`、`internal/team/runner_helpers.go`、`internal/team/summary.go`、`internal/team/status_projector.go`、`internal/team/runner_team_observer.go`、`internal/biz/team_types.go` | ✅ |
| T1.3 修复 Graph 事件 SpiritSessionID 填充 | `internal/graph/trpc/event_bridge.go`、`internal/graph/runtime_replanner.go`、`internal/graph/topology_evolution.go`、`internal/graph/adapter/runtime_adapter.go`、`internal/biz/graph_runtime.go`、`internal/biz/graph_execution_usecase.go`、`internal/biz/graph.go`、`internal/team/runner_mediator.go`、`internal/team/runner_team_compiler.go`、`internal/team/team_graph_run_context.go` | ✅ |
| T1.3.1 Spirit 生成 plan 事件 SpiritSessionID 填充（Spirit 直接运行场景，SpiritSessionID = SessionID） | `internal/service/pre_planning_gate.go` | ✅ |
| T1.4 Activity Schema seq 字段保留但停止赋值（不删除字段，避免数据迁移风险；新事件 seq=0） | `internal/data/ent/schema/activity.go` | ✅ |
| T1.5 查询排序改为 ORDER BY turn_id, parent_activity_id, timestamp | `internal/data/activity_repo.go` | ✅ |

**验收标准**：
- [x] GlobalSeqAllocator 已移除，编译通过（`internal/biz/activity_seq.go` 已删除，`go build ./...` 通过）
- [x] Team 事件 SpiritSessionID 已填充（T1.2 完成，包含 helper/summary/projector 站点）
- [x] Graph 事件 SpiritSessionID 已填充（T1.3 完成，包含 topology_evolution/pre_planning_gate 站点）
- [x] 历史加载按 Timestamp 排序正确（`activity_repo.go` 5 处查询均使用 `Order(turnID, parentActivityID, timestamp)`）

#### Phase T2: 前端 TeamCard/AgentCard 组件（P0）

**目标**：实现 team-card 和 agent-card 组件

| 任务 | 文件 | 状态 |
|------|------|------|
| T2.1 新增 TeamCard.vue 组件 | `web/src/components/chat/TeamCard.vue` | ✅ |
| T2.2 新增 AgentCard.vue 组件 | `web/src/components/chat/AgentCard.vue` | ✅ |
| T2.3 ActivityStream 增加 team_stage → TeamCard 分支 | `web/src/components/chat/ActivityStream.vue` | ✅ |
| T2.4 ActivityStream 增加 session → AgentCard 分支 | `web/src/components/chat/ActivityStream.vue` | ✅ |
| T2.5 移除 compareActivities 中的 seq 排序逻辑 | `web/src/features/chat/composables/useActivityTimeline.ts` | ✅ |
| T2.6 实现排序改为 Timestamp + parentActivityId | `web/src/features/chat/composables/useActivityTimeline.ts` | ✅ |

**验收标准**：
- [x] TeamCard 按 布局设计渲染（头部2:中部6:尾部2）
- [x] AgentCard 简化版渲染（含补充输入框）
- [x] team-card 展开/折叠正常（T5.2 新增 `expand` emit）
- [x] agent-card 展开/折叠正常（T5.3 新增 `expand` emit）
- [x] 排序按 Timestamp 正确（`compareActivities` 使用 `timestamp` 排序，`parentActivityId` 用于树构建）

#### Phase T3: 交互能力（P1）

**目标**：实现 team/agent 的取消/重试交互能力（用户决策：只实现 cancel + retry，不实现 pause/resume/inject）

**业务语义**：
- **取消**：执行中发现卡死或任务不符合预期，主动终止（终态，不可恢复）
- **重试**：执行中出现卡死/中断/工具失败，重新恢复任务（保留原 plan）

| 任务 | 文件 | 状态 |
|------|------|------|
| T3.1 后端新增 RetrySession RPC（cancel 复用现有 StopGeneration） | `api/kratos/chat/v1/chat.proto` + `internal/service/chat.go` + `internal/biz/` | ✅ |
| T3.2 前端 spirit/api.ts 新增 cancelAgentSession/retryAgentSession 封装 | `web/src/features/spirit/api.ts` | ✅ |
| T3.3 TeamCard.vue 移除 inject 框，按钮改为 cancel+retry | `web/src/components/chat/TeamCard.vue` | ✅ |
| T3.4 AgentCard.vue 移除 inject 框，按钮改为 cancel+retry（emit childSessionId） | `web/src/components/chat/AgentCard.vue` | ✅ |
| T3.5 ActivityStream/ChatMessageList/ChatMessagePanel/ChatPage 接线 | 多文件 | ✅ |

**API 现状**：
- Team: cancel ✅（`cancelSpiritTeam`）+ retry ✅（`retrySpiritTeam`）— 已有，直接复用
- Agent cancel: ✅ 复用现有 `POST /v1/chat/stop`（StopGeneration RPC，body: session_id=childSessionId）
- Agent retry: ✅ 已新增 `POST /v1/chat/sessions/{session_id}/retry`（RetrySession RPC，[chat.go:146](../../../internal/service/chat.go#L146)）

**验收标准**：
- [x] Team running 状态显示"取消"按钮，点击触发 cancelSpiritTeam
- [x] Team failed/interrupted 状态显示"重试"按钮，点击触发 retrySpiritTeam
- [x] Agent running 状态显示"取消"按钮，点击触发 agent cancel API
- [x] Agent failed/interrupted 状态显示"重试"按钮，点击触发 agent retry API
- [x] completed/cancelled 状态隐藏按钮
- [x] 移除 inject 输入框（team-card + agent-card）

**已知后端缺口**（不在 T3 范围，记录备忘）：
- ~~AgentCard 的 `enter-session`/`cancel-agent`/`retry-agent` 依赖 `childSessionId`，但后端尚未发射 `child_session_id` meta（B.7.3 后端修复范围之外的影响，需后端补全 AgentCard 子 session 创建逻辑后才能生效）~~ ✅ 已于 Phase T6 修复（`publishTeamStepActivity` 改发 `Kind=Session` 携带 `meta.child_session_id = run.SessionID`，cancel/retry 复用共享 team session）

#### Phase T4: PlanBlock 折叠与状态更新（P1）

**目标**：实现任务计划面板的折叠和状态更新

| 任务 | 文件 | 状态 |
|------|------|------|
| T4.1 PlanBlock 折叠/展开交互 | `web/src/components/chat/PlanBlock.vue` | ✅ |
| T4.2 plan 状态由 team_stage 事件驱动更新 | `web/src/features/chat/composables/useActivityTimeline.ts` | ✅ |
| T4.3 初始渲染时若所有 plan item 已完成则自动折叠为摘要（X/N）；运行中变为全部完成不触发自动折叠（用户意图优先） | `web/src/components/chat/PlanBlock.vue` | ✅ |
| T4.4 计划变更直接更新 plan 内容（替换 items 列表），不引入 diff 标记 | `web/src/components/chat/PlanBlock.vue` | ✅ |

**验收标准**：
- [x] plan 面板支持折叠/展开
- [x] 初始渲染时若全部完成则自动折叠为摘要（X/N）
- [x] 运行中变为全部完成不触发自动折叠（用户意图优先）
- [x] plan 状态由 team_stage 事件驱动更新
- [x] 计划变更直接更新 plan 内容（无 diff 标记）

#### Phase T5: 历史加载懒加载（P1）

**目标**：实现只加载 spirit 根 session，子 session 懒加载

| 任务 | 文件 | 状态 |
|------|------|------|
| T5.1 历史加载只加载 spirit 根 session | `web/src/features/chat/composables/useActivityTimeline.ts` | ✅ |
| T5.2 team-card 展开时懒加载子 session | `web/src/components/chat/TeamCard.vue` | ✅ |
| T5.3 agent-card 展开时懒加载子 session | `web/src/components/chat/AgentCard.vue` | ✅ |
| T5.4 已加载子 session 缓存 | `web/src/features/chat/composables/useActivityTimeline.ts` | ✅ |

**验收标准**：
- [x] 历史加载只加载 spirit 根 session
- [x] team-card 展开时懒加载子 session
- [x] agent-card 展开时懒加载子 session
- [x] 已加载子 session 缓存，不重复加载

**实施说明**：
- T5.1：`loadActivitiesFromAPI` 仅加载指定 session 的 activities（backend `ListBySession` RPC 按 `session_id` 索引查询，只返回该 session 直接持久化的 activities，不含子 session）。Caller (`ChatPage.onEnterSession` / 初始 session 加载) 只对 spirit 根 session 调用一次。
- T5.2：`TeamCard.vue` 新增 `expand: [sessionIds: string[]]` emit，在 `toggleExpand` 从 false→true 时 emit 成员 `session_id` 列表。事件经 `ActivityStream → ChatMessageList → ChatMessagePanel → ChatPage.onExpandChildren` 透传到 `useActivityTimeline.ensureActivitiesLoaded`。
- T5.3：`AgentCard.vue` 对称新增 `expand: [sessionIds: string[]]` emit，在 `toggleExpand` 从 false→true 且 `childSessionId` 存在时 emit `[childSessionId]`。透传路径同 T5.2。
- T5.4：`ensureActivitiesLoaded` 已有缓存（`if (activitiesBySession.value.has(sessionId)) return;`），重复展开同一 team/agent card 不会触发重复 API 调用。失败不写缓存，下次调用自动重试。

#### Phase T6: Chat UX 8 项问题修复（P0）

**目标**：修复 2026-06-29 用户反馈的 8 项 Chat UI 体验问题（指令滞后/成员名丑/AgentCard 失效/取消无效/进度淹没/顺序错乱/输出卡顿/文档同步）。

| 任务 | 文件 | 状态 |
|------|------|------|
| T6.1 notice 不再独立渲染，附加到 task meta（方案 B） | `web/src/components/chat/ActivityStream.vue`、`web/src/features/chat/composables/useActivityTimeline.ts` | ✅ |
| T6.2 TeamCard 成员名解析：SpiritTeamAssembler 注入 AgentReader，批量查询 DisplayName | `internal/service/spirit_team.go`、`cmd/admin/wire_gen.go` | ✅ |
| T6.3 AgentCard 显示成员名 + cancel/retry 生效：publishTeamStepActivity 改发 Kind=Session 携带 child_session_id（选项 B：复用 session 事件 + 共享 team session） | `internal/team/runner_helpers.go`、`internal/biz/orchestration_status.go` | ✅ |
| T6.4 cancel 按钮无效修复（同 T6.3，共享 team session） | 同 T6.3 | ✅ |
| T6.5 check_progress 标记 silent tool，不再生成 action Activity（方案 A） | `internal/agent/activity_projector.go` | ✅ |
| T6.6 思考/act/回复顺序：DECISION.md 强制中间回复规则 + AfterTool→BeforeModel 状态注入提示（方案 A+B 组合） | `internal/scenario/system/prompts/DECISION.md`、`internal/agent/reply_reminder_inject.go`、`internal/agent/callback_chain.go` | ✅ |
| T6.7 输出越来越卡：按 activityId 细粒度响应式重构（方案 B） | `web/src/features/chat/composables/useActivityTimeline.ts` | ✅ |
| T6.8 文档同步（本节） | `docs/development/1-chat.development.md`、`docs/development/1-chat.design.md` | ✅ |

**验收标准**：
- [x] 指令 notice 不再独立占位，附加到对应 task 的 meta
- [x] TeamCard 成员名显示 DisplayName（如"深度研究员"），未解析时回退 agent_key
- [x] AgentCard 显示成员 DisplayName，不再显示"成员"占位
- [x] AgentCard cancel 按钮可取消整个 team run（共享 team session 语义）
- [x] AgentCard retry 按钮可重新入队最近用户消息
- [x] check_progress 工具调用不污染 Activity 流
- [x] Agent 执行工具后立即给出中间回复，指明下一步
- [x] 流式输出不再随消息数线性卡顿
- [x] 文档与代码同步

**实施说明**：
- T6.1：`useActivityTimeline.ts` 中 notice 事件不再独立插入 timeline；`ActivityStream.vue` 的 `renderItems` computed 过滤无 `parentActivityId` 的 notice，匹配到时间戳最近的后续 task activity 并附加到其 `meta.notices`。Orphan notice（无匹配 task）回退为独立项以防丢失。
- T6.2：`SpiritTeamAssembler` 结构体新增 `agentReader biz.AgentReader` 字段，构造函数注入；`publishSpiritTeamAssembled` 循环调用 `GetAgentByAgentKey` 解析 DisplayName，失败回退 agent_key（`shared.ErrNotFound` 静默，其他错误记 warn 日志）。`make wire` 自动重新生成 `wire_gen.go` 注入 `agentRepository`。
- T6.3+T6.4：`publishTeamStepActivity` 签名新增 `agentName string` 参数；`Kind` 从 `ActivityKindTeamStage` 改为 `ActivityKindSession`；`Meta` 新增 `child_session_id = run.SessionID`。`persistStep` 调用处传入 `step.AgentName`/`saved.AgentName`。`OrchestrationStatusStore.ApplyActivityEvent` 扩展 `ActivityKindSession` 分支处理 `Stage=executing/completed`（保留原 `checkpoint` 分支），确保 orchestration status projection 不被破坏。
- T6.5：`ActivityProjector` 维护 `silentToolNames` 集合（含 `check_progress`），`OnToolCall` 对 silent 工具早返回不创建 action Activity。
- T6.6：`DECISION.md` 新增"中间回复规则"章节，强制 agent 执行工具后立即给出回复（指明下一步 + 提高可观测性）；`reply_reminder_inject.go` 实现 AfterTool→BeforeModel 状态注入：AfterTool hook 写 invocation state，BeforeModel hook 读 state 并注入 system message 提醒 agent 给出中间回复。
- T6.7：`useActivityTimeline.ts` 中 Activity 对象用 `reactive()` 包裹（替代 `ref([])` + `triggerRef`），streaming 事件直接修改 reactive 对象字段，Vue 自动按 `activityId` 细粒度触发更新，避免全量 re-render。
- T6.8：本节同步更新开发计划文档；设计文档补充 per-member step 改用 `Kind=Session` 的事件映射说明。

**回归测试**：
- `internal/service/spirit_team_assembler_test.go::TestPublishSpiritTeamAssembled_UsesDisplayName` — 守卫 DisplayName 解析 + 未知 key 回退
- `internal/team/runner_helpers_test.go::TestPersistStep_EmitsStartedAndFinished` — 更新断言：`Kind=Session`、`AgentName`、`meta.child_session_id`
- `internal/agent/activity_projector_test.go` — silent tool 行为守卫
- `internal/biz/orchestration_status_test.go` — 验证 Kind=Session 的 executing/completed 分支不破坏 status projection

#### Phase T7: 三种模式数据模型 + WS 协议设计文档（P1）

**目标**：文档化三种对话模式的 Activity 树结构、统一数据模型和 WebSocket 通信协议设计（需求文档 + 设计文档）。

| 任务 | 文件 | 状态 |
|------|------|------|
| T7.1 需求文档新增 §1.7 三种对话模式（Activity 树结构 + 对比表 + 统一数据模型） | `docs/development/1-chat.md` | ✅ |
| T7.2 设计文档新增 §5.4 WebSocket 通信协议设计（通用信封 + Client/Server 消息 + 交互时序 + 断线重连 + SSE vs WS 对比） | `docs/development/1-chat.design.md` | ✅ |
| T7.3 设计文档 B.2.1 三种模式渲染规则更新（细化树结构 + 组件映射 + Kind 标注） | `docs/development/1-chat.design.md` | ✅ |
| T7.4 需求文档子模块 A.3 用户故事更新（引用 §1.7 + 补充三种模式详细描述） | `docs/development/1-chat.md` | ✅ |
| T7.5 开发计划文档新增 T7（本文档） | `docs/development/1-chat.development.md` | ✅ |

**验收标准**：
- [x] 需求文档 §1.7 含三种模式完整 Activity 树结构、对比表、统一数据模型
- [x] 设计文档 §5.4 含 WS 通用信封、Client/Server 消息定义、完整交互时序、取消流程、断线重连流程、SSE vs WS 对比
- [x] 设计文档 B.2.1 含三种模式详细树结构 + Kind 标注 + 组件映射
- [x] 需求文档子模块 A.3 引用 §1.7，补充模式 A/B/C 用户故事
- [x] 文档三件套内容边界合规（需求文档无代码分层/API 细节，设计文档无用户故事/开发进度）

### C.5 改动文件清单

#### 后端

| 文件 | 改动类型 | 说明 |
|------|---------|------|
| `internal/biz/activity_seq.go` | 删除 | 移除 GlobalSeqAllocator |
| `internal/agent/activity_projector.go` | 修改 | 移除 GlobalSeqAllocator 引用，不再为事件分配 seq |
| `internal/event/activityevent/bus.go` | 修改 | 移除 GlobalSeqAllocator 引用，direct-publish 事件不再分配 seq |
| `internal/team/runner_team_trpc_phases.go` | 修改 | Team 事件填充 SpiritSessionID（含 deriveSpiritSessionID 辅助函数、createInitialTeamRun 设置字段） |
| `internal/team/team_graph_run_coordinator.go` | 修改 | Team Graph 协调器事件填充 SpiritSessionID |
| `internal/team/team_graph_run_finisher.go` | 修改 | Team Graph 完成事件填充 SpiritSessionID（含 GetTeamRunByID 后回填 run.SpiritSessionID） |
| `internal/team/runner_team_turn.go` | 修改 | Team turn 事件填充 SpiritSessionID |
| `internal/team/runner_helpers.go` | 修改 | publishTeamRunFailedActivity（team_stage/failed）读取 run.SpiritSessionID；publishTeamStepActivity 改发 Kind=Session 携带 AgentName + meta.child_session_id（Phase T6.3+T6.4） |
| `internal/team/summary.go` | 修改 | TeamSummaryActivityEvent 读取 run.SpiritSessionID |
| `internal/team/status_projector.go` | 修改 | OrchestrationProjectorConfig 新增 SpiritSessionID 字段，publishOrchestrationStatus 读取 cfg.SpiritSessionID |
| `internal/team/runner_team_observer.go` | 修改 | startObservers 设置 SpiritSessionID: deriveSpiritSessionID(sess) |
| `internal/team/team_graph_run_context.go` | 修改 | GraphRunStepContext 新增 SpiritSessionID 字段，buildGraphRunStepContext 签名扩展 |
| `internal/team/runner_mediator.go` | 修改 | TeamGraphCoordAccess 接口和 RegisterTeamGraphExecution 方法签名扩展 |
| `internal/team/runner_team_compiler.go` | 修改 | RegisterTeamGraphExecution 调用方传入 deriveSpiritSessionID(sess) |
| `internal/biz/team_types.go` | 修改 | TeamRun 新增 SpiritSessionID 字段（`json:"-"` 非持久化运行时元数据） |
| `internal/biz/graph_runtime.go` | 修改 | GraphRunnerFactory 接口三个方法签名扩展（添加 spiritSessionID 参数） |
| `internal/biz/graph_execution_usecase.go` | 修改 | RegisterTeamGraphExecution/ExecuteGraph/ExecuteGraphBuildConfig/ensureCheckpointRuntime/ResumeExecution 设置 exec.SpiritSessionID |
| `internal/biz/graph.go` | 修改 | GraphUsecase.RegisterTeamGraphExecution 委托方法签名扩展 |
| `internal/graph/trpc/event_bridge.go` | 修改 | EventBridge 新增 spiritSessionID 字段，convertEvent 填充 SpiritSessionID |
| `internal/graph/runtime_replanner.go` | 修改 | publishReplanEvent 填充 SpiritSessionID: exec.SpiritSessionID |
| `internal/graph/topology_evolution.go` | 修改 | publishTopologyEvolvedEvent 填充 SpiritSessionID: exec.SpiritSessionID |
| `internal/graph/adapter/runtime_adapter.go` | 修改 | trpcGraphRuntime 新增 spiritSessionID 字段，buildRuntime/buildNodeCallbacks/BuildRuntime/BuildAndRun/BuildAndResume 签名扩展 |
| `internal/service/pre_planning_gate.go` | 修改 | publishPlanningPhase 填充 SpiritSessionID: sessionID（Spirit 直接运行场景） |
| `internal/data/activity_repo.go` | 修改 | 查询排序改为 ORDER BY turn_id, parent_activity_id, timestamp |
| `internal/data/ent/schema/activity.go` | 不修改 | seq 字段保留（停止赋值，不删除字段） |
| `internal/service/spirit_team.go` | 修改 | Phase T6.2：SpiritTeamAssembler 注入 `biz.AgentReader`，`publishSpiritTeamAssembled` 批量解析 DisplayName |
| `internal/service/spirit_team_assembler_test.go` | 新增 | Phase T6.2：回归守卫，验证 DisplayName 解析 + 未知 key 回退 |
| `internal/team/runner_helpers.go` | 修改 | Phase T6.3+T6.4：`publishTeamStepActivity` 改发 `Kind=Session` 携带 `AgentName` + `meta.child_session_id`；`persistStep` 调用处传入 `step.AgentName`/`saved.AgentName` |
| `internal/team/runner_helpers_test.go` | 修改 | Phase T6.3+T6.4：更新断言为 `Kind=Session`，新增 `AgentName`/`child_session_id` 校验 |
| `internal/biz/orchestration_status.go` | 修改 | Phase T6.3+T6.4：`ApplyActivityEvent` 扩展 `ActivityKindSession` 分支处理 `Stage=executing/completed`（保留原 `checkpoint` 分支），避免破坏 orchestration status projection |
| `internal/agent/activity_projector.go` | 修改 | Phase T6.5：维护 `silentToolNames` 集合（含 `check_progress`），`OnToolCall` 早返回不创建 action Activity |
| `internal/agent/activity_projector_test.go` | 新增/修改 | Phase T6.5：silent tool 行为守卫 |
| `internal/agent/reply_reminder_inject.go` | 新增 | Phase T6.6：AfterTool→BeforeModel 状态注入，提醒 agent 给出中间回复 |
| `internal/agent/callback_chain.go` | 修改 | Phase T6.6：注册 reply_reminder hook |
| `internal/scenario/system/prompts/DECISION.md` | 修改 | Phase T6.6：新增"中间回复规则"章节，强制 agent 执行工具后立即给出回复 |
| `cmd/admin/wire_gen.go` | 重新生成 | Phase T6.2：`make wire` 自动注入 `agentRepository` 到 `NewSpiritTeamAssembler` |

#### 前端

| 文件 | 改动类型 | 说明 |
|------|---------|------|
| `web/src/components/chat/TeamCard.vue` | 新增 | team-card 组件（替换 TeamStageBlock.vue） |
| `web/src/components/chat/AgentCard.vue` | 新增 | agent-card 组件 |
| `web/src/components/chat/ActivityStream.vue` | 修改 | team_stage 分支指向 TeamCard；session 分支指向 AgentCard |
| `web/src/components/chat/TeamStageBlock.vue` | 删除 | 被 TeamCard.vue 完全替代 |
| `web/src/components/chat/PlanBlock.vue` | 修改 | 增加折叠/展开交互、初始渲染自动折叠摘要 |
| `web/src/features/chat/composables/useActivityTimeline.ts` | 修改 | 移除 seq 排序，改用 timestamp + parentActivityId；Phase T6.1：notice 不再独立插入 timeline；Phase T6.7：Activity 用 `reactive()` 包裹，streaming 事件直接修改字段（按 activityId 细粒度响应式） |
| `web/src/features/spirit/api.ts` | 修改 | 新增 cancelAgentSession（复用 StopGeneration）/ retryAgentSession（新增 RetrySession RPC）封装 |
| `web/src/stores/spirit/index.ts` | 修改 | 适配新设计（如有需要） |
| `web/src/components/chat/ActivityStream.vue` | 修改 | Phase T6.1：`renderItems` computed 过滤无 `parentActivityId` 的 notice，匹配到时间戳最近的后续 task activity 并附加到其 `meta.notices` |

### C.6 已知技术债务

| 编号 | 问题 | 严重度 | 处理方式 |
|------|------|--------|---------|
| TD-T1 | 卡住检测未实现 | 中 | 先记录场景，后续迭代设计心跳检测 |
| TD-T2 | 历史 sub_task_board Activity 数据 | 低 | 提供 legacy 兼容渲染或迁移 |
| TD-T3 | ~~59-chat-ui-optimization.md 引用失效~~（已解决） | 低 | ✅ 旧文档已删除，内容合并到 1-chat 三件套子模块 |

### C.7 验收标准（整体）

- [ ] 简单对话模式：精灵直接 thinking + reply，无 plan/graph/team
- [ ] Team 模式：plan → graph → team-card 顺序显示
- [ ] agent-card：简化版布局，含补充输入框
- [ ] team-card 布局：头部2:中部6:尾部2，符合设计
- [x] team-card 尾部：取消按钮 + 重试按钮（T3 用户决策：只实现 cancel + retry，不实现 pause/resume/inject）
- [ ] team-card 展开：显示成员列表，成员展开显示 thinking/action/reply
- [ ] plan 面板：固定位置，支持折叠，状态由 team_stage 事件驱动
- [ ] 进度计算：X/N 简单实现
- [ ] Team 失败：手动重试
- [ ] 历史加载：只加载 spirit 根 session，子 session 懒加载
- [x] direct-publish 事件：SpiritSessionID 已填充（T1.2/T1.3/T1.3.1 完成）
- [ ] 排序：用 Timestamp，无全局 Seq
- [ ] MaxDepth：从 AgentRuntimeSetting.MaxSessionDepth 读取
