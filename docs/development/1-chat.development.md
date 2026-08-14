# Chat 对话 — 开发计划

> **版本**：2026-07-16 | **状态**：✅ 端到端可用；**聊天渲染已 v2-only**（`v2_event` + Task/Turn/Step）；v1 `activity_event` / `ActivityBridgeEvent` 生产发布路径已退役  
> **Review**：[2026-05-23-Chat-Flow-Full-Review.md](../review/2026-05-23-Chat-Flow-Full-Review.md)  
> **M55 Cursor 对标**：[55-chat-channel-cursor-solution.md](./55-chat-channel-cursor-solution.md) · [55-chat-channel-cursor-development.md](./55-chat-channel-cursor-development.md)  
> **四层解耦（DECO）**：[0-module-decoupling-architecture.md §3.1](./0-module-decoupling-architecture.md#31-推荐目标架构channel--chat--agent) · 任务板 [17-channel-development.md §14](./17-channel-development.md#14-phase-deco--四层架构解耦deco)（DECO-06/13/14）
> **需求**：[1-chat.md](./1-chat.md) · **设计**：[1-chat.design.md](./1-chat.design.md)  
> **ADR-02 / ADR-03**：已归档；运行时真相源为 v2 EventBus + Sequencer（见设计文档 §二 / §三）  
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—
>
> **文档边界**：本文档包含模块定位、代码锚点、现状评估、差距/优化、阶段划分、任务清单（含状态）、验收标准、改动文件清单。用户故事、功能需求清单、验收标准见 [1-chat.md](./1-chat.md)；架构设计、代码分层、Proto/API 契约、数据模型、接口定义、状态机、序列图、前端组件设计、UX 规范见 [1-chat.design.md](./1-chat.design.md)。

---

## 1. 模块定位

Chat 是用户与 Agent/Team 交互的核心入口，负责 HTTP/WS 发起对话、WebSocket + **v2 EventBus**（下行 `v2_event` + `monitor_event`）、上下文管理、用量记录、停止生成、人工等待回复与待执行队列。

**代码锚点**（已校验存在，2026-07-16 v2-only）：
- `api/kratos/chat/v1/chat.proto` — Chat RPC（含 `EnqueueUserMessage` → `POST /v1/chat/enqueue`）
- `internal/runtime/run_registry.go` — 会话级 active run / cancel / run status
- `internal/runtime/pending_queue.go` — PendingMessageQueue FIFO
- `internal/server/ws.go` / `ws_v2_subscriber.go` — WebSocket（上行 `user_message`；下行 `v2_event`）
- `internal/server/ws_message_handler.go` — WS 上行消息分发
- `internal/biz/chat_usecase.go` — Follow-up Queue 编排
- `internal/biz/event.go` / `event_system.go` — v2 Event 接口 + `system.notice` / `system.run_status`
- `internal/service/chat.go` — ChatService 薄传输桥（`Close` 调用 `PlanExecutor.Stop`）
- `internal/service/plan_executor.go` — PlanExecutor.StartSubscription / Stop（PlanBoard 事件订阅与 DAG lease 生命周期）
- `internal/service/chat_orchestrator.go` — ChatOrchestrator 编排核心
- `internal/service/chat_runtime_tooling.go` — RuntimeTooling 按域分组（Knowledge/Skill/Plugin/Bridges/Sharing/Extensions，AS-COG-01）
- `internal/agent/stream_consumer.go` — turn 流消费
- `internal/agent/v2/projector.go` — **唯一**投影器：trpc event → Task/Turn/Step…
- `internal/agent/v2/sequencer.go` — FIFO 发布；`step.streaming` 仅 WS（flush 时分配会话级单调 `DeltaSeq`，前端按序号去重，替代原内容指纹方案）；终态 WBPF + outbox + dead-letter
- `internal/event/contract/monitor_event.go` — MonitorEvent（监控通道，与聊天分离）
- `web/src/features/chat/composables/useChatWorkspace.ts` — Chat 页编排（只消费 `v2_event`）
- `web/src/features/chat/composables/useChatEventRouter.ts` — v2 EventKind → activityV2Store
- `web/src/stores/chat/activityV2Store.ts` — v2 实体树 + `fetchSessionHistory`
- `web/src/features/session/v2Api.ts` — REST hydrate（tasks/turns/steps/…）
- `web/src/components/chat/ChatMessageList.vue` — **仅** SessionPanelV2（无 legacy message 回落）
- `web/src/components/chat/v2/SessionPanel.vue` / `TaskCard.vue` / `TurnContainer.vue` — 主渲染树
- `web/src/features/chat/composables/useActivityQueries.ts` — 展示层查询门面（组件不直连 Pinia）

> **已删除 / 退役（2026-07-16 v2-only + leftover cleanup）**：
> - WS `activity_event` pump、`ActivityBridgeEvent` / `activity.bridge`、notice↔Activity 兼容工厂
> - 聊天 Cancel/Confirm 写路径改为 `steps_v2`；Spirit 组装不再 dual-write `activities`
> - 前端 legacy message 回落、`AgentWorkPanel.vue`；非聊天消费者改 `onV2Event` / `system.notice` 适配
> - 文档锚点中的 `activity_projector.go` / `activity_event_sequencer.go` / `useActivityTimeline` / `ActivityStream`（已由 v2 取代）
> - **已 DROP（DDL 20261012）**：`activities` 表 + Ent Activity schema + `activity_repo.go`
> - Event Inspector 历史/实时均走 v2 steps（`listStepsV2` + `step.*`）；ListActivities RPC 仍保留兼容
> - Graph 流原生 `system.notice`（`onGraphNotice`）；`messageStore` 仅 revision/hydrate
> - Memory Path B `ListRecentMessages` 已迁 `tasks_v2` + `steps_v2`；ActivityRepo 接口已删除

> 代码分层、请求流转、Proto 契约、WebSocket 协议详见 [1-chat.design.md](./1-chat.design.md)。

---

## 2. 现状评估（2026-07-16 v2-only）

| 项 | 状态 | 证据 |
|----|------|------|
| WS 实时对话 | ✅ | `/v1/ws` + `user_message` + `v2_event`（Task/Turn/Step…） |
| HTTP unary 对话 | ✅ | `SendChatMessage` / `RunNativeTurnUnary` |
| Channel / Cron 入口 | ✅ | `lockSession` + `RunRegistry` |
| 停止 / 运行中追加 | ✅ | `StopGeneration` / WS `cancel`；`EnqueueUserMessage` |
| 待执行 / Follow-up Queue | ✅ | Pending FIFO；`system.run_status` / `system.notice` 刷新 |
| RunStatus + AwaitUserReply | ✅ | RPC + v2 `system.run_status` |
| Team / Graph UI | ✅ | typed `team_*` / `graph_*` / `member_session` v2 事件；Teams/Monitor 经 `teamRunEventFromV2Event` |
| 非聊天 WS 消费者 | ✅ | `system.notice` 适配（graph）或原生 `onV2Event`（orch/knowledge/jobs/inbound） |
| WS 控制消息 | ✅ | `connected`/`pong`/`server_shutdown`；重连 hydrate 走 v2 REST |
| 工具 / Reasoning UI | ✅ | StepKind `action` / `thinking` → ActionBlock / ThinkingBlock |
| WS 重连恢复 | ✅ | `activityV2Store.fetchSessionHistory`（v2 REST） |
| 聊天渲染真相源 | ✅ | **仅 v2**（无 legacy message / activity_event 时间线） |
| 模型选项 | 🟡 | Platform 优先 + `GetChatOptions("model")` 回退 |
| 附件 / Vision | ✅ | Artifact 上传 + 多 part 装配 |
| Session 父子树 | ✅ | `GetSessionTree` + SessionTreeSidebar |
| 事件持久化 | ✅ | v2 Sequencer：streaming 不落库；终态 WBPF + outbox；retry + dead-letter |

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
| P-V2LF | **v2 实体生命周期与状态级联修复** | ✅ | P1-P6 + P5 Task 延迟关闭 + Fixes 1-7（seq 注入/msID 诊断/Cancelled 事件/双写消除/GraphStage 去重等）+ P-DBLEXEC 双重执行止血与 PlanStep.AgentKeys 传递（2026-07-05） |
| 11 | **三种模式数据模型 + WS 协议设计文档** | ✅ | 需求文档 §1.7 + 设计文档 §5.4 + B.2.1（Phase T7） |
| P-MDINC | **流式 MD 增量渲染（块级冻结 + DOM 分段 + 高亮 memo）** | ✅ | 借鉴 xai-grok-markdown；设计详见 [1-chat.design.md §11.1.2](./1-chat.design.md#1112-md-渲染策略2026-07-20-更新流式增量渲染) |

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
| 59 | **T-ER.1** 后端 `handleTextDelta` 纯空白 delta 不创建 ReplyStep（防过早创建） | P1 | ✅ 2026-07-04 |
| 60 | **T-ER.2** 后端 `handleTextDone` 空 content 走 cancelled 路径（复用 `NewStepCompletedEvent` + `Status=cancelled`） | P1 | ✅ 2026-07-04 |
| 61 | **T-ER.3** 前端 `TurnContainer.visibleSteps` 兜底过滤空 reply step（非 running 且 Content trim 后为空） | P1 | ✅ 2026-07-04 |
| 62 | **T-ER.4** spec `2026-07-02-llm-activity-ordering-design.md` §3.2.1 图示更新为多轮模式 | P2 | ✅ 2026-07-04 |
| 63 | **T-ER.5** 设计文档同步 §12.8 v2 Step 模型 + 空 ReplyStep 过滤 | P2 | ✅ 2026-07-04 |
| 64 | **P-ORCH.1** 编排细粒度进度事件（后端 planner/allocator/factory 发布 orchestration_progress） | P0 | ✅ 2026-07-18 |
| 65 | **P-ORCH.2** 前端 loading 映射（observabilityConstants + useContextualLoadingMessage） | P0 | ✅ 2026-07-18 |
| 66 | **P-ORCH.3** Agent 创建确认（EnsureAgent 复用 tool_confirmation 模式：ReplyFunc + ActivityEmitter） | P1 | ✅ 2026-07-18 |
| 67 | **P-ORCH.4** 确认链路验证（ConfirmActivity RPC 复用 + session 状态流转 + 拒绝降级） | P1 | 🟡 单测覆盖，运行时端到端待验证 |
| 68 | **P-ORCH.5** Allocate 两阶段并行化（Phase A 并行匹配 + Phase B 串行 factory） | P2 | ✅ 2026-07-18 |
| 69 | **P-REPORT** 任务执行总结（2026-07-24 重构：移除报告卡片 UI，改为 system-push 触发精灵 LLM 输出 Markdown 总结回复 + 取消守卫 + CAS 防重 + 兜底 notice） | P1 | ✅ 2026-07-24（运行时端到端已验证） |
| 70 | **P0-5b** 收窄 Chat `RuntimeTooling`：24 平铺字段按域拆为 6 分组（Knowledge/Skill/Plugin/Bridges/Sharing/Extensions），薄 `RuntimeTooling` 字段数 = 6 | P0 | ✅ 2026-08-14 |

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
| `web/src/components/chat/v2/GraphStageBlock.vue` | Chat PlanDAG 流程图（v2）；v1 组件已删除 |
| `web/src/components/chat/TeamCard.vue` | 移除 border+background，改树形行式布局；新增 `autoExpand` prop 支持外部触发展开 |
| `web/src/components/chat/AgentCard.vue` | 移除 border+background，改树形行式布局；新增 `autoExpand` prop 支持外部触发展开 |
| `web/src/components/chat/ActivityStream.vue` | 透传 `autoExpandFor` prop 到 TeamCard/AgentCard；TeamCard `autoExpand` 支持任意成员 agentKey 匹配；递归子流同步透传 |
| `web/src/components/chat/ChatMessageList.vue` | 监听 `useScrollToActivity` 定位命令；自动展开父级卡片；`data-agent-key`/`data-team-id` 定位；黄色高亮动画 |
| `web/src/features/chat/composables/useScrollToActivity.ts` | 新建：模块级 ref 单例，跨组件传递定位命令 |
| `web/src/components/spirit/SynthesisResultCard.vue` | `team-summary` / `team-findings` 改为完整换行显示；`renderedContent` 内图片限制最大宽度 |
| `web/src/i18n/locales/zh-CN.ts` / `en-US.ts` | 新增 `chat.agentSidebar.settings` 等文案 |
| `web/src/features/chat/streamEventTypes.ts` | `TeamMemberStatus` 新增 `blocked` 状态 |
| `web/src/features/chat/composables/useActivityTimeline.ts` | members 映射逻辑新增 blocked 状态 |

### P-ER 空 ReplyStep 清理与多轮 Step 模型澄清（2026-07-04 新增）

> **目标**：修复 chat 模块"空回复块"显示问题；澄清 turn 内 step 模型为多轮模式（`thinking → action → thinking → action → ... → reply`）。
> **设计稿**：[docs/superpowers/specs/2026-07-04-empty-reply-step-cleanup-design.md](../superpowers/specs/2026-07-04-empty-reply-step-cleanup-design.md)
> **关联 spec**：[docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md §3.2.1](../superpowers/specs/2026-07-02-llm-activity-ordering-design.md)

#### 任务清单

- [x] T-ER.1 后端 `handleTextDelta` 纯空白 delta 不创建 ReplyStep（防过早创建）
- [x] T-ER.2 后端 `handleTextDone` 空 content 走 cancelled 路径（复用 `NewStepCompletedEvent` + `Status=cancelled`）
- [x] T-ER.3 前端 `TurnContainer.visibleSteps` 兜底过滤空 reply step
- [x] T-ER.4 spec `2026-07-02-llm-activity-ordering-design.md` §3.2.1 图示更新为多轮模式
- [x] T-ER.5 设计文档同步 §12.8 v2 Step 模型 + 空 ReplyStep 过滤

#### 改动文件清单

| 文件 | 改动 |
|------|------|
| `internal/agent/v2/projector.go` | `handleTextDelta` + `handleTextDone` 修改（防过早创建 + 空 content 走 cancelled） |
| `internal/agent/v2/projector_test.go` | 新增 4 个测试（whitespace no-create / empty cancelled / normal / no-op） |
| `web/src/components/chat/v2/TurnContainer.vue` | `visibleSteps` 兜底过滤空 reply step |
| `web/src/components/chat/__tests__/TurnContainer.spec.ts` | 新建 7 个测试 |
| `docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md` | §3.2.1 图示更新为多轮模式 + Turn 内 Step 模型说明 |
| `docs/superpowers/specs/2026-07-04-empty-reply-step-cleanup-design.md` | 新建设计稿 |
| `docs/development/1-chat.design.md` | 新增 §12.8 v2 Step 模型与空 ReplyStep 过滤 |
| `docs/development/1-chat.development.md` | 新增 P-ER Phase 块 + §6 任务清单追加 T-ER.1~T-ER.5 |

#### 验收标准

- [x] 后端 `go test ./internal/agent/v2/...` 通过（4 个新测试 + 全部历史测试）
- [x] 后端 `internal/agent/...` + `internal/biz/...` 全量回归通过
- [x] 前端 `TurnContainer.spec.ts` 7/7 通过
- [x] 前端 lint 0 errors（52 个 warning 均为预先存在）
- [x] 空 ReplyBlock 不再显示（completed/cancelled 状态的空 reply 被过滤）
- [x] 流式中的 reply step 仍正常显示（Status=running 不被过滤）

---

### P-V2LF v2 实体生命周期与状态级联修复（2026-07-04 新增）

> **目标**：修复 v2 实体（Task/TeamStage/TeamRun/MemberSession/PlanBoard）生命周期状态级联不关闭、刷新后数据丢失、团队名称为空等问题。
> **分析报告**：[docs/reports/2026-07-04-analysis-v2-entity-lifecycle-state-cascade-fix.md](../reports/2026-07-04-analysis-v2-entity-lifecycle-state-cascade-fix.md)
> **关联 spec**：[docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md](../superpowers/specs/2026-07-02-llm-activity-ordering-design.md)

#### 任务清单

- [x] P1.1 修复 pending queue RootTaskActivityID 丢失（C2）— `chat_orchestrator_turn_dispatch.go` + `chat_orchestrator_turn.go` + `team_turn_hooks.go`
- [x] P1.2 `publishV2TeamRunCompletion` 增加日志（C1）— 0 结果 Warn + member count Info
- [x] P2 修复 PlanBoard 状态转换（D2）— `plan_executor.go` 增加 `markPlanBoardExecuting`
- [x] P3 放宽 TaskCard.vue 过滤（C7）— planning 状态 PlanBoard 也显示
- [x] P4 修复 4 处绕过 seq.Publish（C5）— `chat_event_publisher.go` + `chat_orch_await.go` + `pre_planning_gate.go` + `chat_run_gateway.go`
- [x] P6 team_name 防御性 Warn（C3）— `spirit_team.go` `publishSpiritTeamAssembled`
- [x] P5 Task 状态延迟关闭（D1）— `ProjectorFactory.teamDispatched` 跟踪 + `OnTurnEnd` 延迟 task.completed，synthesis turn 触发关闭
- [x] P-FIX1 `GraphOrchestrationProjector` 注入 seq（高）— `graph_task_status.go` 优先 `seq.Publish`（持久化+WS），fallback eventBus
- [x] P-FIX2 `HandleTeamTurnResult` 入口 Warn（高）— `RootTaskActivityID` 为空时记录告警
- [x] P-FIX3 `publishV2TeamRunCompletion` msID 诊断日志（高）— 记录 msID + agentKey + DB ID
- [x] P-FIX4 Cancelled TeamRun 事件语义修正（中）— 改用 `NewTeamRunFailedEvent`
- [x] P-FIX5 GraphStage 重复创建修复（中）— 移除 `initGraphStage` 中冗余 `seq.Publish`，保留同步 Upsert 作为 crash recovery
- [x] P-FIX6 PlanStep 双写消除（低）— 移除 4 处直接 `repos.UpsertPlanStep`，统一由 `seq.Publish` → EventRouter 异步持久化
- [x] P-FIX7 PlanExecutor 注入 TeamDispatchMarker（高/P5 配套）— `dispatchStep` 标记 task 已派发 team
- [x] P-DBLEXEC 双重执行止血与 PlanStep.AgentKeys 传递（2026-07-05 新增）— 5 步修复：禁用 Path A team 创建 + PlanStep 增加 AgentKeys 字段 + PublishV2Board 移到 Phase 2 之后 + RealTeamOrchestrator 优先 step.AgentKeys + TaskOrchestratorImpl.Orchestrate 标记 Deprecated。详见设计文档 §B.10.8

#### 改动文件清单

| 文件 | 改动 |
|------|------|
| `internal/service/chat_orchestrator_turn_dispatch.go` | `processPendingQueue` 增加 `rootTaskID` 参数，注入 `loopCtx` |
| `internal/service/chat_orchestrator_turn.go` | 调用 `processPendingQueue` 传 `RootTaskActivityIDFromCtx(ctx)` |
| `internal/service/team_turn_hooks.go` | 调用 `processPendingQueue` 传 `RootTaskActivityIDFromCtx(ctx)` |
| `internal/service/spirit_team.go` | `publishV2TeamRunCompletion` 增加 0 结果 Warn + member count Info；`publishSpiritTeamAssembled` 增加 team.DisplayName 为空 Warn |
| `internal/service/plan_executor.go` | `Subscribe` 增加 `markPlanBoardExecuting`（planning → executing） |
| `internal/agent/v2/projector.go` | `ProjectorFactory` 增加 `Seq()` 方法暴露 seq |
| `internal/service/chat_orchestrator.go` | 增加 `v2Seq` 字段，从 `V2ProjectorFactory.Seq()` 提取 |
| `internal/service/chat_event_publisher.go` | `chatTurnEventPublisher` 增加 `seq` 字段，`PublishTurnFailure` 用 seq 优先 |
| `internal/service/chat_orch_await.go` | `chatAwaitCoordinator` 增加 `seq` 字段，`PublishAwaitResumed` 用 seq 优先 |
| `internal/service/pre_planning_gate.go` | `PrePlanningGate` 增加 `seq` 字段，`publishPlanningPhase` 用 seq 优先 |
| `internal/service/chat_run_gateway.go` | `chatEventPublisher` 增加 `seq` 字段，`publishMessageQueuedToBus` 用 seq 优先 |
| `internal/service/chat_orchestrator_turn_preplanning.go` | `NewPrePlanningGate` 调用传 `o.v2Seq` |
| `web/src/components/chat/v2/TaskCard.vue` | `planBoards` computed 移除过滤，planning 状态也显示 |
| `internal/service/pre_planning_gate_test.go` | 2 处 `NewPrePlanningGate` 调用增加 `nil` seq 参数 |
| `internal/service/turn_error_publish_test.go` | 3 处 `newChatTurnEventPublisher` 调用增加 `loggateway.NewNoop()` 参数 + import |
| `internal/service/graph_orchestration_projector.go` | 增加 `seq` 字段 + `SetSeq` 方法（P-FIX1） |
| `internal/service/graph_task_status.go` | `PublishGraphTaskStatus` 优先使用 `seq.Publish`，fallback `eventBus.Publish`（P-FIX1） |
| `internal/service/spirit_team.go` | `HandleTeamTurnResult` 入口 Warn（P-FIX2）；`publishV2TeamRunCompletion` msID 诊断日志（P-FIX3）；Cancelled TeamRun 改用 `NewTeamRunFailedEvent`（P-FIX4） |
| `internal/service/plan_executor.go` | 移除 `initGraphStage` 冗余 `seq.Publish`（P-FIX5）；移除 4 处直接 `repos.UpsertPlanStep`（P-FIX6）；新增 `TeamDispatchMarker` 接口 + `SetTeamDispatchMarker` + `dispatchStep` 标记（P-FIX7） |
| `internal/service/plan_executor_test.go` | `fakeSeq` 增加 `repos` 字段模拟 EventRouter 持久化行为（P-FIX6 测试修复） |
| `internal/agent/v2/projector.go` | `ProjectorFactory` 增加 `teamDispatched sync.Map` + `MarkTeamDispatched`/`HasTeamDispatch`/`ClearTeamDispatch`；`ActivityProjector` 增加 `factory` 字段；`OnTurnEnd` 检查 team 派发标记决定是否延迟 task.completed（P5） |
| `internal/service/chat_wire.go` | `ProvideChatService` 新增 `graphProj` 参数；注入 seq 到 GraphOrchestrationProjector（P-FIX1）；注入 ProjectorFactory 到 PlanExecutor 作为 TeamDispatchMarker（P-FIX7） |
| `cmd/admin/wire_gen.go` | `make wire` 自动重生成：`ProvideChatService` 调用增加 `graphOrchestrationProjector` 参数 |
| `internal/tools/spirit_tools.go` | P-DBLEXEC Step 1：`executeOrchestratePhase` 不再调用 `deps.orchestrator.Orchestrate`，返回 placeholder handle；Step 3：Phase 2 之后调用 `planner.PublishV2Board`，direct 路径传 nil allocPlan |
| `internal/biz/plan_step.go` | P-DBLEXEC Step 2：`PlanStep` 增加 `AgentKeys []string` 字段 |
| `internal/data/ent/schema/plan_step_v2.go` | P-DBLEXEC Step 2：Ent Schema 增加 `agent_keys` JSON 列 |
| `internal/data/plan_step_v2_repo.go` | P-DBLEXEC Step 2：4 处 ent↔biz 转换函数更新（Create/Update/Upsert/entPlanStepV2ToBiz） |
| `internal/data/sql/migrations/20261003_plan_step_agent_keys.sql` | P-DBLEXEC Step 2：DDL 迁移（ALTER TABLE plan_steps_v2 ADD COLUMN agent_keys TEXT） |
| `internal/data/ddl_migration_registry.go` | P-DBLEXEC Step 2：注册 20261003 迁移 |
| `internal/biz/task_planner.go` | P-DBLEXEC Step 3：`TaskPlannerPort` 接口新增 `PublishV2Board` 方法 |
| `internal/agent/task_planner_impl.go` | P-DBLEXEC Step 3：`publishV2PlanBoard` → 公开 `PublishV2Board(ctx, plan, allocPlan, chatSessionID)`；从 `publishPlanCreated` 移除调用；从 `allocPlan.Allocations` 填充 `PlanStep.AgentKeys` |
| `internal/service/team_orchestrator_real.go` | P-DBLEXEC Step 4：`Orchestrate` 优先 `step.AgentKeys`，fallback `resolveAgentKeys(ctx)`；新增 `agentKeysSource` 辅助日志 |
| `internal/agent/task_orchestrator_impl.go` | P-DBLEXEC Step 5：`Orchestrate` 标记 Deprecated（team 创建迁移到 PlanExecutor + RealTeamOrchestrator） |
| `internal/service/pre_planning_gate_test.go` | P-DBLEXEC Step 4：`fakePlanner` 增加 `PublishV2Board` stub 方法 |

#### 验收标准

- [x] `go build ./...` 通过
- [x] `go test ./internal/service/... ./internal/agent/... ./internal/biz/...` 通过（含 `plan_executor_test.go` 修复后全绿）
- [x] `pnpm lint` 0 errors（67 个 warning 均为预先存在）
- [x] `pnpm build` 通过
- [x] 设计文档同步：`1-chat.design.md` §B.4.3 补充 PlanBoard 状态机 + 显示规则
- [x] `make wire` 重生成（`ProvideChatService` 签名变更）
- [ ] **运行时验证**（待用户执行）：清空数据库 → 发起复杂任务 → 验证：
  - Task 状态在 team 派发后保持 Running，synthesis turn 完成才转 completed
  - 实体状态级联关闭（Task/Turn/TeamStage/TeamRun/MemberSession/PlanBoard/PlanStep）
  - 刷新后数据不丢失（ActivityBridgeEvent 经 seq 持久化）
  - team_name 非空
  - Cancelled TeamRun 状态正确（Failed 事件而非 Started 占位）
  - PlanStep 单写无冗余（仅 seq.Publish 路径）
  - **P-DBLEXEC**：每个 PlanStep 只创建一个 team（无双重执行）；不同 team 使用不同 agent（PlanStep.AgentKeys 来自 LLM 分配，而非全部使用同一批 DB active agent）；`Orchestrate` 日志 `source=plan_step` 表示使用 LLM 分配，`source=db_fallback` 表示 fallback

#### P5 Task 延迟关闭设计（已实现）

**问题**：`ActivityProjector.OnTurnEnd` 在 root turn 完成时立即发射 `task.completed`，但 team 可能仍在执行，导致 Task 显示"已完成"而团队未结束。

**方案**（system-push 模式配套）：
1. `ProjectorFactory` 增加 `teamDispatched sync.Map`（key: taskID）
2. `PlanExecutor.dispatchStep` 在 `Orchestrate` 成功后调用 `MarkTeamDispatched(taskID)`
3. `OnTurnEnd` root turn 完成时检查 `HasTeamDispatch(meta.TaskID)`：
   - 命中 → 跳过 `task.completed`，Task 保持 Running（等 synthesis turn）
   - 未命中 → 正常发射 `task.completed`（原行为）
4. synthesis continuation turn（`meta.ParentTaskID != ""`）完成时发射 `task.completed`（parent task）并 `ClearTeamDispatch(taskID)`

**依赖**：`PlanExecutor.SetTeamDispatchMarker` 由 `ProvideChatService` 注入 `ProjectorFactory`（后置构造注入，避免 wire 循环）。

---

### P-ORCH 编排实时反馈与 Agent 创建确认（2026-07-18 新增）

> **目标**：编排三阶段（plan → allocate → factory/orchestrate）增加细粒度实时反馈；Agent 自动创建改为用户审批制；Allocate 并行化降低冷启动耗时。
> **需求**：[1-chat.md §1.8](./1-chat.md#18-编排实时反馈与-agent-创建确认) US-ORCH-01/02
> **设计**：[1-chat.design.md §B.10.14](./1-chat.design.md#b1014-编排实时反馈与-agent-创建确认2026-07-18-新增)

#### 任务清单

- [x] P-ORCH.1 后端进度事件：planner `Plan()` 发 decomposing/decomposed（新增 v2 EventBus 注入）；allocator `Allocate()` 发 allocating/allocated；factory `EnsureAgent()` 发 creating_agent/agent_created（SystemNoticeEvent，WS-only）；`TaskProfile` 新增 `SpiritSessionID` 字段
- [x] P-ORCH.2 前端 loading 映射：`observabilityConstants.ts` 新增 `ORCHESTRATION_PROGRESS_MAP`（6 条 phase 条目）；`useContextualLoadingMessage.ts` 新增 `orchestration_progress` 分支（按 meta.phase + index/total/agentName 渲染）
- [x] P-ORCH.3 Agent 创建确认：`EnsureAgent` 复用 tool_confirmation 模式——ctx 取 `serviceawaitreply.ReplyFunc` + `biz.ActivityEmitter`，LLM 生成提案后 EmitConfirmRequest → 阻塞等待（5min 超时）→ EmitConfirmResult；nil-safe（无 ReplyFunc 直接创建）
- [x] P-ORCH.4 确认链路验证：ConfirmActivity RPC 零改动复用；确认期间 session awaiting_confirmation，确认后恢复；拒绝/超时返回错误走 allocator fallback（单测覆盖确认门禁逻辑；真实用户确认的运行时端到端验证待补）
- [x] P-ORCH.5 `Allocate()` 两阶段重构：Phase A 并行 Layer 0-3（errgroup + 索引写入，提取为 `runPhaseAMatch`）；Phase B 串行 factory（含确认）→ fallback；收尾串行 selectAdditionalMembers + 单次持久化
- [x] 验证：后端 `go build ./...` + `go test ./internal/agent/... -race` + `go test ./internal/biz/...`；前端 `pnpm vitest run useContextualLoadingMessage.spec`（43/43）
- [x] 代码审查（aranea-review SKILL）+ 修复（2026-07-18：修复 DOC-SYNC-5 状态同步；提取 runPhaseAMatch 收敛 Allocate 长度）

#### 改动文件清单

| 文件 | 改动 |
|------|------|
| `internal/biz/agent_factory.go` | `TaskProfile` 新增 `SpiritSessionID` 字段 |
| `internal/agent/task_planner_impl.go` | `Plan()` decomposeTask 前后发进度事件（新增 v2 EventBus 注入） |
| `internal/agent/agent_allocator_impl.go` | `Allocate()` 两阶段重构 + 进度事件 |
| `internal/agent/agent_factory.go` | 上下文确认流程（复用 tool_confirmation 模式）+ 进度事件 |
| `cmd/admin/wire.go` + `wire_gen.go` | `provideTaskPlanner` 新增 v2 EventBus 参数 |
| `web/src/features/spirit/observabilityConstants.ts` | 新增 `ORCHESTRATION_PROGRESS_MAP`（6 条 phase 映射） |
| `web/src/features/chat/composables/useContextualLoadingMessage.ts` | 新增 orchestration_progress 分支 |

#### 验收标准

- [ ] 编排期间用户可见细粒度进度（分解中 → 匹配 i/N → 创建 Agent 中）
- [ ] 4 层匹配失败时弹出确认卡片，批准创建/拒绝降级
- [ ] 确认期间 session 进入 awaiting_confirmation，确认后自动恢复
- [ ] 多 subtask 需创建 Agent 时逐个确认（串行）
- [ ] Allocate 并行化后单测全绿，无数据竞争（`go test -race`）
- [ ] 无 ReplyFunc 上下文时行为与旧版一致（直接创建）

### P-MDINC 流式 MD 增量渲染（2026-07-20）

> **分析来源**：[docs/reports/2026-07-19-analysis-grok-build-insights.md](../reports/2026-07-19-analysis-grok-build-insights.md)
> **设计**：[1-chat.design.md §11.1.2](./1-chat.design.md#1112-md-渲染策略2026-07-20-更新流式增量渲染)

#### 任务清单
- [x] T-MDINC.1 块级冻结边界检测（`computeFrozenPrefixEnd`：列表/缩进代码/fence/链接引用定义/EOF 生长规则）
- [x] T-MDINC.2 分段渲染接口（`renderChatMarkdownParts`：frozenSegments + frozenEpoch + finish() 全量兜底）
- [x] T-MDINC.3 DOM 分段渲染指令（`vSegmentedMarkdown`：冻结段 DOM 不回改、epoch 前缀隔离整体失效）
- [x] T-MDINC.4 组件集成（`ReplyBlock`/`ThinkingBlock`/`ChatReasoningDrawer` 替换 v-html）
- [x] T-MDINC.5 代码高亮 memo（`detectCodeLanguage.ts`：highlight/detectLanguage 双 LRU 各 100 条 + 32KB 上限 + 500 字符 sample key）
- [x] T-MDINC.6 流式等价性安全网（枚举码点二分/逐字符/变长 chunk/4 段组合/CRLF/代理对/finish 逐字节一致）+ 全量验证（pnpm lint 0 errors / 623 测试通过 / build 成功）

#### 改动文件清单
- `web/src/features/chat/chatMessageMarkdown.ts`（分段渲染 + finish() 兜底）
- `web/src/features/chat/vSegmentedMarkdown.ts`（DOM 分段渲染指令，新）
- `web/src/features/chat/lib/detectCodeLanguage.ts`（高亮/探测 LRU memo）
- `web/src/components/chat/ReplyBlock.vue`、`ThinkingBlock.vue`、`ChatReasoningDrawer.vue`（指令集成）
- `web/src/features/chat/__tests__/chatMessageMarkdown.spec.ts`（分段渲染 + 安全网测试）
- `web/src/features/chat/__tests__/vSegmentedMarkdown.spec.ts`（指令 DOM 测试，新）
- `web/src/features/chat/__tests__/detectCodeLanguage.spec.ts`（memo 测试）

---

### P-RECOVER 崩溃恢复与中断任务续跑（L1/L2/L3，2026-07-22）

> **目标**：进程突然关闭导致在途任务终止后，用户重开软件可继续对话/续跑任务。方案经用户确认为 L2+L3 全做；L3 续跑语义为「带完整执行轨迹重跑」（已完成的 step 不跳过，轨迹注入 prompt 供 agent 参考）。
> **设计**：[1-chat.design.md §B.10.16](./1-chat.design.md#b1016-崩溃恢复与中断任务续跑l2l3落地记录2026-07-22--已落地)

#### 任务清单

- [x] L1 孤态恢复语义调整：`V2RecoveryRepo.FailOrphanedInFlight` 把 in-flight task 终态化为 `interrupted`（新增 `TaskStatusInterrupted`，可续跑）而非 `failed`；其余实体（turn/step/team_stage/team_run/member_session）仍为 `failed`；返回 `[]InterruptedTaskRef` 供启动通知
- [x] L2 关机保护：`EscalateAllActiveToDurable` 把活跃 interactive run 批量升级为 durable（写 checkpoint + 标记自动恢复）；`SessionStatusGuard` 增加 `SessionRunDurableEscalator` 依赖并在关机路径调用；Wire 绑定 `SessionRunDurableEscalator → ChatService`
- [x] L3 显式续跑后端：`ChatService.ResumeInterruptedTask`（预检 → `TaskV2Repo.ResumeInterruptedTask` CAS `interrupted→running` → `BuildTaskResumeTrace` 组装轨迹 → 异步 `RunNativeTurn(ParentTaskID=taskID)` 重跑）；WS 上行 `resume_task`（`server.TaskResumer` 本地接口 + `SetTaskResumer` 接线）
- [x] 终态事件版本冲突修复：`task.completed`/`task.failed` 改走 `CompleteTaskTerminal`（version 自 DB +1，忽略事件 version）——解决续跑 CAS 推高 version 后 synthesis `OnTurnEnd` 硬编码 `Version=2` 被 `UpsertTask` 的 `VersionLT` guard 拒绝、task 永远 running 的问题
- [x] 启动通知：`SessionStatusGuard` 按 session 发布 `task_interrupted` 系统 notice（仅对存在可续跑任务的 session）
- [x] L3-F4 前端入口：`v2Types.ts` TaskStatus 加 `interrupted`；`TaskCard.vue` 中断提示条 + 「继续执行」按钮；事件冒泡链 `TaskCard → TaskList → SessionPanel → ChatMessageList → ChatMessagePanel → ChatPage`；`useChatWorkspace.resumeTask` 发送 WS 上行（无乐观更新，`task.updated` 驱动 UI）；i18n zh-CN/en-US 三键
- [x] 全量验证：后端 `go build ./...` exit 0 + `go test`（service/data/server/biz/agent/v2 全过，仅 3 例既有网络依赖失败与本次无关）；前端 `pnpm lint` 0 errors + `pnpm test` 651 全过 + `pnpm build` 成功
- [x] 文档同步：`1-chat.design.md` §B.10.16 + §5.1 上行消息表（`resume_task`）；`65-module-cross-reference-full.md` §1.6 Chat 卡片（新增实现接口 + 崩溃恢复开发注意）

#### 改动文件清单

| 文件 | 改动 |
|------|------|
| `internal/biz/task.go` | 新增 `TaskStatusInterrupted` |
| `internal/biz/repo_ports_v2.go` | `TaskV2Writer` 新增 `ResumeInterruptedTask`（CAS）+ `CompleteTaskTerminal` |
| `internal/biz/task_resume.go` | `BuildTaskResumeTrace` + `InterruptedResumeUserContent`（轨迹注入） |
| `internal/data/v2_recovery_repo.go` | 孤态恢复 task → `interrupted`，返回 `[]InterruptedTaskRef` |
| `internal/data/task_v2_repo.go` | `ResumeInterruptedTask` CAS + `CompleteTaskTerminal`（version 自 DB +1） |
| `internal/agent/v2/event_router.go` | 终态事件路由到 `CompleteTaskTerminal` |
| `internal/agent/v2/reposet_adapter.go` | RepoSet 适配 `CompleteTaskTerminal` |
| `internal/service/chat_task_resume.go` | `ChatService.ResumeInterruptedTask`（L3 主流程） |
| `internal/service/chat_task_resume_test.go` | L3 单测 |
| `internal/service/chat_durable_escalate_all.go` | `EscalateAllActiveToDurable`（L2 批量升级） |
| `internal/service/session_status_guard.go` | 新增 `SessionRunDurableEscalator` 接口 + 关机调用 + 启动 `task_interrupted` 通知 |
| `internal/server/ws.go` / `ws_message_handler.go` | `TaskResumer` 接口 + `resume_task` 上行处理 |
| `cmd/admin/wire.go` / `wire_gen.go` | `SessionRunDurableEscalator → ChatService` 绑定 + `SetTaskResumer` 接线 |
| `web/src/features/chat/v2Types.ts` | `TaskStatus` 新增 `'interrupted'` |
| `web/src/components/chat/v2/TaskCard.vue` | 中断提示条 + 「继续执行」按钮 + `resume-task` emit |
| `web/src/components/chat/v2/TaskList.vue` / `SessionPanel.vue` / `ChatMessageList.vue` / `ChatMessagePanel.vue` | `resume-task` 事件透传 |
| `web/src/pages/ChatPage.vue` | `@resume-task="session.resumeTask"` 绑定 |
| `web/src/features/chat/composables/useChatWorkspace.ts` | `resumeTask`（WS 上行 `resume_task`） |
| `web/src/i18n/locales/zh-CN.ts` / `en-US.ts` | `taskInterrupted`/`resumeTask`/`resumeTaskSent` |

#### 验收标准

- [x] 崩溃重启后 in-flight task 落库为 `interrupted`（可续跑），其余 v2 实体为 `failed`
- [x] 优雅退出时活跃 interactive run 批量升级 durable，重启后由 `SessionRunDurableWorker` 自动续跑
- [x] 任务卡片显示中断条 + 「继续执行」，点击后 CAS `interrupted→running` 并带轨迹重跑；重复点击/并发由 CAS 防住（409）
- [x] 续跑后 synthesis 终态事件经 `CompleteTaskTerminal` 正常落库（task 不停留在 running）
- [ ] **运行时端到端验证**（待用户执行）：运行中任务 →  kill 进程 → 重启 → 任务卡片显示「继续执行」→ 点击 → 任务带轨迹重跑至完成

---

### P-CLARIFY 需求澄清提问（Clarification Gate，2026-07-22）

> **目标**：用户输入存在阻塞性歧义时，系统在同一 turn 内先以分页卡片向用户提问澄清，作答（或留空按推荐）后带澄清上下文续跑，避免方向性返工。
> **需求**：[1-chat.md §1.10](./1-chat.md#110-需求澄清提问clarification)（US-CLARIFY-01）
> **设计**：[1-chat.design.md §B.10.18](./1-chat.design.md#b1018-需求澄清提问-clarification-gate)

#### 任务清单

- [x] TDD-biz：`StepKindClarify` + `StepStatusAwaitingInput` + `sessstatus.StatusReasonClarification` 枚举与序列化
- [x] TDD-intent：`intent.Artifact` 新增 `Clarifications`/`RiskFlags`；prompt 输出澄清契约（阻塞性歧义才触发，≤5 问）
- [x] TDD-service：ClarificationGate 判定矩阵（enabled × needs_clarification × questions 非空）；`publishClarifyStep`（UpsertTask 幂等 + orphan Step Kind=clarify/Status=awaiting_input/Version=1/StartedAt + seq.Publish）；Session → awaiting_confirmation(reason=clarification)；turn 挂起
- [x] TDD-service：提交端点 `POST /v1/chat/clarifications/{step_id}`（CAS awaiting_input→completed，重复提交 409）+ 同 turn 续跑注入澄清问答上下文
- [x] TDD-service：自由回复等价路径（`Execute` 统一入口拦截 `resolveClarificationFreeText`：pending+step 双判据 → 按推荐填充空作答 + 回写 `free_text` + 完成 step + 恢复 running + 输入重写为「澄清上下文+原始需求」；非等待态/失败透传；`chat_clarify_freetext_test.go` 4 例）
- [x] 前端：`ClarifyBlock.vue` 分页卡片（上一页/下一页/完成，无跳过；单选/多选；推荐项高亮；每页「其他」输入；留空可提交；提交后只读摘要）；`TaskCard.vue` orphan step 注册渲染；hydration 恢复；i18n zh-CN/en-US
- [x] TDD-data：Agent 设置 `clarification_enabled` Ent 持久化（schema 列 + DDL 迁移 20261108 + 双向映射 + 默认值 true）
- [x] TDD-service：重启恢复——信封持久化 `original_input`，提交时 `resolveResumeInput` 内存 pending 优先、缺失则惰性重建续跑输入（4 例）；自由回复路径重启后 pending 缺失降级为新 turn（设计 B.10.18.5 已载明）
- [x] 全量验证：`go build ./...` + `go test`（service 4.7s/biz 9.8s/agent 32.6s/data 21.0s）全过（2026-07-23 复跑）；前端 `pnpm lint` + `pnpm test`（699 例）+ `pnpm build` 全过
- [x] 文档同步：本块状态标记、§6 任务清单、65 交叉参考 Chat 卡片；B.10.18.2/3/5 信封字段与自由回复/重启恢复实现语义（2026-07-23）

#### 抗过度澄清增强（P0+P1，2026-08-09）

> **需求**：[1-chat.md §1.10](./1-chat.md#110-需求澄清提问clarification)（US-CLARIFY-02）
> **设计**：[1-chat.design.md §B.10.18.7](./1-chat.design.md#b10187-抗过度澄清增强as-built2026-08-09)

- [x] P0 TDD：门判定矩阵扩展——全部问题含推荐默认且无高风险标记 → `AutoResolved`（不挂起）；任一问题缺推荐或命中高风险标记（`HasHighRiskFlag`：touches_auth/migrations/sensitive_data/compliance/destructive/irreversible）→ 挂起（`chat_clarify_gate_test.go`：`TestRunClarificationGate_AutoResolvedWhenAllRecommended` / `TestRunClarificationGate_TriggeredWhenQuestionLacksRecommended`）
- [x] P0 biz：信封 `Resolution` 字段 + `ApplyRecommendedAnswers` + `ClarificationAllRecommended`（`step.go` / `step_clarify_test.go`）；`autoResolveClarification` 落 completed 审计卡（resolution=auto_default）+ `ResolvedInput` 注入澄清上下文 + Artifact 剥离澄清残留（防下游重问）；意图产物注入统一下移到澄清门之后
- [x] P1 TDD：`history_test.go` 4 例（无历史/含历史拼装/超 6 条截最旧/单条 200 runes 截断）+ prompt 纪律守卫（`TestIntentSystemPrompts_ClarificationDiscipline`：历史消歧优先、禁问已确立事实、推荐默认自主执行）
- [x] P1 历史注入：`intent/history.go`（HistoryMessage + MaxIntentHistoryMessages=6 + buildUserMessageContent）；`TurnDeps.MsgHistory`（`biz.SessionRecentMessageLister` 窄接口）+ `recentIntentHistory`（过滤角色/空内容、剔除同文当前输入、失败降级 nil）；chat 路径接线、team 成员 turn 传 nil；wire 两处 TurnDeps 接 `MsgHistory: sessions`
- [x] 验证：独立 GOCACHE 下 `go test ./internal/agent/intent/ ./internal/service/ ./internal/biz/... ./internal/team/ ./internal/runtime/... ./cmd/admin/` 全绿（排除 2 个已知 models.dev 网络受限测试）；`go build ./cmd/... ./internal/... ./pkg/...` + vet 干净；改动文件 gofmt 干净（2026-08-09）

#### 验收标准

- [ ] LLM 判定阻塞性歧义时发布 clarify 卡片并挂起 turn；轻微歧义不触发
- [ ] 卡片分页交互：上一页/下一页/完成；无跳过；可全部留空提交（按推荐执行）
- [ ] 提交后同一任务续跑，澄清问答注入上下文，不产生新任务卡片
- [ ] 重复提交被 CAS 拒绝（409）；刷新/重启后卡片与作答进度恢复
- [ ] `clarification_enabled=false` 时门透传走原流程
- [ ] 全部问题含推荐默认且无高风险标记时不挂起，落 completed 审计卡（resolution=auto_default）续跑；命中高风险标记仍挂起等答
- [ ] 追问指代（"它""这个"）能从近期对话解析时不触发澄清；对话中已确立的事实不被重问

---

### P-REPORT 任务执行总结（2026-07-22 → 2026-07-24 重构）

> **目标**：编排类任务全部执行完成（成功/部分失败/失败）后，精灵自动在聊天时间线给出任务总结；用户主动中断（存在 cancelled 团队）时不触发；同一编排只触发一次。
> **需求**：[1-chat.md §子模块：任务执行总结](./1-chat.md)
> **设计**：[1-chat.design.md §B.10.17](./1-chat.design.md#b1017-任务执行总结llm-总结回复落地设计2026-07-22-初版2026-07-24-重构)
> **演进**：2026-07-22 初版为 ExecutionReportCard 报告卡片（信封 JSON → notice step → 四板块组件）；2026-07-24 与用户确认重构——总结不需要专门 UI，改为 `TeamStarter` 向精灵会话注入 `synthesisSummaryTrigger`（system-push），精灵以普通 reply step 输出四节结构 Markdown 总结。报告卡片全链路已移除（R4 全局清理）。

#### 任务清单（2026-07-24 重构后现状）

- [x] biz 取消计数：`CheckAllTeamsCompleted` 将 cancelled 团队从 failed 中分离，`AllTeamsCompletedResult.CancelledTeams`（2026-07-22 引入，保留）
- [x] 取消守卫：`checkAllTeamsCompleted` 检测 `CancelledTeams > 0` 时跳过总结触发（用户主动中断不出总结）
- [x] CAS 防重：`synthesisTriggered.LoadOrStore(spiritSessionID)` 仅放行一次；session 重建时 Delete 复位
- [x] 总结触发：`synthesisSummaryTrigger` 常量（四节结构 prompt 契约）经 `turnGateway.ExecuteTurn` 注入精灵会话（system-push，不渲染用户气泡）；`ParentTaskID = resolveLatestUserTaskID` 附着最近用户任务
- [x] 兜底通知：`ExecuteTurn` 失败时 `publishAllTeamsCompletedFallbackNotice` 发布直发 notice「所有团队已完成」（挂同一任务下）
- [x] biz 清理：`SynthesisOutput` 移除 `Overview`/`Deliverables`/`Degraded`；删除 `SynthesisEventPublisher` 端口；`NewSynthesisUsecase` 构造简化
- [x] service 清理：`SpiritSynthesisService` 移除 `synthesisEventPublisher`/`executionReportEnvelope` 与报告发布逻辑；`service.go` 移除失效 wire.Bind
- [x] 前端清理：删除 `ExecutionReportCard.vue`、`executionReport.ts`、`chat.executionReport.*` i18n 键；`NoticeBlock.vue` 移除报告分支（统一默认 notice 渲染）；`useContextualLoadingMessage` 移除 `synthesis_completed` 映射；`CoreContainers.spec.ts` 移除报告用例
- [x] 测试：service `spirit_team_synthesis_turn_test.go`（触发注入/cancelled 守卫/CAS 防重/兜底 notice/TaskID 回退）；biz `spirit_synthesis_report_test.go` 适配（run stats enrich/engine 错误传播/cancelled 计数）
- [x] 全量验证：`go build ./...` exit 0 + `go test` 全过；前端 `pnpm lint` + `pnpm test` + `pnpm build` 全过
- [x] 运行时端到端验证（2026-07-24）：发起编排任务 → 团队并行执行 → 精灵输出四节 Markdown 总结回复 → 刷新后总结在原位置还原
- [x] 文档同步：`1-chat.md` 子模块需求 + `1-chat.design.md` §B.10.17 + 本块

#### 改动文件清单（2026-07-24 重构）

| 文件 | 改动 |
|------|------|
| `internal/service/spirit_team.go` | 移除 synthesisSvc 依赖；新增 `synthesisSummaryTrigger` 常量 + 直接总结 turn 触发 + `publishAllTeamsCompletedFallbackNotice` |
| `internal/biz/spirit_synthesis.go` | `SynthesisOutput` 移除 Overview/Deliverables/Degraded；删除 `SynthesisEventPublisher` 端口；构造简化 |
| `internal/service/spirit_synthesis.go` | 移除 publisher/信封与报告发布逻辑；构造简化 |
| `internal/service/service.go` | 移除失效 `wire.Bind`（synthesisResultService） |
| `internal/service/spirit_team_synthesis_turn_test.go` | 新建：总结 turn 触发路径单测（取代 `spirit_synthesis_report_test.go` service 侧） |
| `internal/biz/spirit_synthesis_report_test.go` | 适配：移除 overview/deliverables 装配断言，保留 run stats/cancelled 计数 |
| `web/src/components/chat/ExecutionReportCard.vue` | 删除 |
| `web/src/features/chat/executionReport.ts` | 删除 |
| `web/src/components/chat/NoticeBlock.vue` | 移除报告卡片分支，统一默认 notice 渲染 |
| `web/src/components/chat/v2/TaskCard.vue` | 注释更新（总结报告 = Spirit 总结 turn 的普通 reply，无专门 UI） |
| `web/src/features/chat/composables/useContextualLoadingMessage.ts` | 移除 `synthesis_completed` 阶段映射 |
| `web/src/i18n/locales/zh-CN.ts` / `en-US.ts` | 移除 `chat.executionReport.*` 键 |
| `web/src/components/chat/__tests__/ExecutionReportCard.spec.ts` | 删除 |
| `web/src/components/chat/v2/__tests__/CoreContainers.spec.ts` | 移除报告相关用例 |

#### 验收标准

- [x] 任务全部完成（成功/部分失败/失败）后，精灵输出四节结构 Markdown 总结回复（任务总结/各团队结果/综合结论/建议与后续行动）
- [x] 存在 cancelled 团队（用户主动中断）时不触发总结
- [x] 总结即普通 reply step 持久化，刷新页面后在原位置完整还原；无专门渲染组件
- [x] 同一 spirit session 一次编排只触发一次总结（CAS）
- [x] 总结触发注入失败时，时间线出现兜底「所有团队已完成」通知

#### 后续修复（2026-07-27，会话 d78029b9 排查）

> 根因链：lazy 建团下 PlanExecutor 按 DAG 逐 step AssembleTeam，首波团队全终态时后续 step 尚无团队记录，`CheckAllTeamsCompleted`（只查 teams 表）把「波次中点」误判为「编排终点」；且 `StartTeamTurn` 每次组团重置 CAS 守卫 → 每个波次触发一次 synthesis（总结 ×3）。同时 `ExecuteTurn` 每个 turn 都跑预规划门控（2 根 turn + 2 澄清续跑 + 3 总结 turn = 7 次），门控 notice 无 TaskID/TurnID 成 session 级孤儿步骤（前端 `getTaskOrphanSteps` 只认 TaskID，孤儿永不渲染但污染数据），共 14 条（7 对重复）。

- [x] 修复 A（门控）：`TeamStarter.checkAllTeamsCompleted` 在 CAS 守卫前新增 `planExecutor.HasActiveRunForSession(spiritSessionID)` 检查——本 session 存在活跃 dagRun（lazy 建团未完）即跳过波次中点的 synthesis；门控路径不占用 CAS 名额
- [x] 修复 A（终态唯一触发）：synthesis 触发点收口到 dagRun 终态——`PlanExecutor.SetCompletionNotifier` 注入 `AllTeamsCompletedNotifier`，`dagRun.releaseLeaseAndNotifyCompletion()` 释放 lease 后调 `NotifyAllTeamsCompleted`
- [x] 修复 B（门控跳过续跑）：`runPrePlanningGate` 对 `ParentTaskID` 非空的续跑 turn（synthesis/澄清续答）直接放行，与 `runClarificationGate` 同款防循环；同时避免 forcedPlanning 系统提示注入 synthesis turn 强制其再走规划路径
- [x] 修复 B（notice 挂接 Task）：`biz.PlanInput` 新增 `TaskID` 字段；`runPrePlanningGate` 从 ctx 取 `RootTaskActivityIDFromCtx`（turn 入口预解析）填入；`PrePlanningGate.publishPlanningPhase` 落 `Step.TaskID`——门控 notice 经 TaskCard orphanNoticeSteps 渲染为任务 footer，不再是孤儿
- [x] 存量清理：`cmd/cleanup_orphan_notices` 工具删除历史孤儿 notice（`kind='notice' AND turn_id='' AND task_id='' AND author_agent_key='pre-planning-gate'`），318 条清零
- [x] 测试：`pre_planning_gate_test.go` 新增续跑跳过 + TaskID 挂接用例
- [x] 验证：`go build ./...` exit 0 + `go test ./internal/service/ -run 'TestRunPrePlanningGate|TestPrePlanningGate|Synthesis'` 全 PASS

| 文件 | 改动 |
|------|------|
| `internal/service/spirit_team.go` | `checkAllTeamsCompleted` 新增活跃 dagRun 门控（CAS 守卫之前） |
| `internal/service/plan_executor.go` | 新增 `SetCompletionNotifier` + `HasActiveRunForSession`；dagRun 终态 `releaseLeaseAndNotifyCompletion` 唯一触发 synthesis |
| `internal/service/chat_orchestrator_turn_preplanning.go` | 签名改收 `biz.TurnInput`；`ParentTaskID` 非空跳过门控；`PlanInput.TaskID` 取自 ctx RootTaskActivityID |
| `internal/biz/task_planner.go` | `PlanInput` 新增 `TaskID` 字段 |
| `internal/service/pre_planning_gate.go` | `publishPlanningPhase` 增加 taskID 参数，落 `Step.TaskID` |
| `internal/service/pre_planning_gate_test.go` | 新增续跑跳过 + TaskID 挂接用例 |
| ~~`cmd/cleanup_orphan_notices/main.go`~~ | 存量孤儿 notice 清理工具（一次性，已执行 318 条清零；2026-08-14 随死代码清理删除） |

---

### P-LAZYLOAD 长会话历史懒加载（2026-07-23）

> **目标**：打开长会话秒级可交互——全部用户指令即时渲染，执行过程仅自动水合最后一轮 + 非终态 task，历史轮次折叠为 meta-bar 卡片按需水合（滚入视口 500ms / 点击），默认停在消息底部。
> **需求**：[1-chat.md §子模块：长会话历史懒加载](./1-chat.md#子模块长会话历史懒加载lazy-hydration)
> **设计**：[1-chat.design.md §B.10.19](./1-chat.design.md#b1019-长会话历史懒加载落地设计2026-07-23)
> **实施计划**：`docs/superpowers/plans/2026-07-23-chat-history-lazy-load.md`

#### 代码锚点

| 锚点 | 文件 |
|------|------|
| ListStepsV2 分页契约 | `api/kratos/session/v1/session.proto` ListStepsV2Request/Response |
| 分页 repo | `internal/data/step_v2_repo.go` ListStepsBySessionPaged |
| 索引迁移 | `internal/data/sql/migrations/20261109_steps_v2_session_seq.sql`（registry 20261109） |
| service 分发 | `internal/service/session_v2.go` ListSteps |
| 分阶段水合 + hydrateTask | `web/src/stores/chat/activityV2Store.ts` fetchSessionHistory / hydrateTask |
| 懒水合编排 | `web/src/features/chat/composables/useLazyTaskHydration.ts` |
| 折叠卡四态 | `web/src/components/chat/v2/TaskCard.vue` |
| 接线 | `web/src/components/chat/v2/TaskList.vue`、`web/src/components/chat/ChatMessageList.vue` |

#### 任务清单

- [x] proto：ListStepsV2Request +limit/before_seq，Response +has_more；`make api` 重新生成
- [x] biz：`StepListOptions{Limit, BeforeSeq}` + `StepV2Reader.ListStepsBySessionPaged`
- [x] data：分页查询实现（`WHERE session_id=? [AND seq<?] ORDER BY seq DESC LIMIT n+1` → hasMore → 反转 ASC；limit<=0 降级遗留全量）+ PG 分页语义测试 4 用例
- [x] data：`(session_id, seq)` 索引迁移 20261109（幂等）+ registry 注册
- [x] service：ListSteps 参数校验（负数 400）+ 分页分发（session 级且 limit>0）+ limit 上限钳制 500 + 5 单测
- [x] v2Api：`listStepsV2` 透传 limit/beforeSeq
- [x] store：`hydratedTaskIds`（跨 WS 重连保留）+ `taskHydration` 瞬态；`task.created` 事件标记水合
- [x] store：`hydrateTask`（幂等，turns/steps/team/plan/graph 并行 + 下钻）+ 分阶段 `fetchSessionHistory`（Phase 1 轻量全量 + steps limit=100 窗口；Phase 3 自动水合集 fire-and-forget）
- [x] composable：`useLazyTaskHydration`（IntersectionObserver + 500ms dwell + 滚出取消 + 滚动锚定 + 手动折叠态 + 卸载清理）
- [x] 门面：`useActivityQueries` +isTaskHydrated/taskHydrationState/hydrateTask；i18n `chat.v2.collapseExecution`/`loadFailedRetry`（zh-CN/en-US）
- [x] TaskCard 四态：折叠（用户面板原样 + meta-bar 状态徽章+耗时）/ 水合中（shimmer×3）/ 失败（meta-bar 重试）/ 水合（完整 + 底部收起按钮）；组件测试 8 用例
- [x] TaskList 接线 + ChatMessageList `provide(CHAT_SCROLL_EL_KEY)`（经门面过 layer 检查）
- [x] 全量验证：`go build ./...` exit 0 + `go test ./internal/service/... ./internal/biz/... ./internal/data/...` 全 PASS（PG 实测）；`pnpm lint` + `pnpm test`（727 用例）+ `pnpm build` 全过
- [x] 文档同步：`1-chat.md` 子模块 + `1-chat.design.md` §B.10.19 + 本块

#### 验收标准

- [x] 首屏仅拉 tasks + steps 最近窗口 + 自动水合集 per-task 请求（单测断言）
- [x] 折叠卡滚入视口 500ms 自动水合 / 点击立即水合 / 失败可重试（composable + 组件单测）
- [x] 手动收起仅 UI 态，再展开零请求（store 单测）
- [x] WS 重连 `hydratedTaskIds` 不清空；task.created 默认展开（store/router 单测）
- [x] **运行时端到端验证**（2026-07-23 已执行，15 轮/253 steps 会话实测）：首屏仅 tasks + steps?limit=100 + 最后 task 6 请求；停底部；折叠卡 dwell 500ms 自动水合；点击立即水合；锚定补偿（scrollTop 1500→10608）；收起后再展开零请求

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


### C.0 2026-07-16 实现锚点对齐（v2）

> **现状**：Chat 块主路径已迁至 v2 实体树；下列旧锚点仅作历史参考。

| 旧锚点 | 当前实现 |
|--------|----------|
| `ActivityStream.vue` | 已删除；由 `v2/SessionPanel.vue` + `TaskCard` 递归实体树替代 |
| `TeamCard.vue` / `TeamStageBlock.vue` | `v2/TeamRunCard.vue` + `v2/TeamStagePanel.vue` |
| `AgentCard.vue`（chat） | `v2/MemberSessionPanel.vue`（含 Mode B orphan） |
| `PlanBlock.vue` | `v2/PlanBoardCard.vue`；PlanBoard ID = `pb_` + plan.ID（同 turn 不重复） |
| `useActivityTimeline.ts` | `stores/chat/activityV2Store.ts` + `useActivityQueries.ts` |
| 排序 | StartedAt 主序 + Seq 次序；`ListStepsBySession` 按 `session_id` 列过滤（根/子懒加载） |


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
| ~~`internal/graph/topology_evolution.go`~~ | 拓扑演化事件（publishTopologyEvolvedEvent） | 🗑️ 已移除（S5：insight 生产者从未落地，TargetNode 恒空导致回调不可达，确认死代码后随 wire 装配一并下线） |
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
- [x] agent-card：简化版布局（`MemberSessionPanel`），含 stop/send 底栏；pause/inject 传 chat SessionID
- [x] 模式 B：orphan MemberSession（`subagents_spawn` → ModeBStartedHook）在 TaskCard 渲染
- [x] 模式 B 历史 hydrate：`ListOrphanMemberSessions`（`GET /v2/sessions/{id}/orphan_member_sessions`）→ `fetchSessionHistory` upsert
- [ ] team-card 布局：头部2:中部6:尾部2，符合设计
- [x] team-card 尾部：取消按钮 + 重试按钮（T3 用户决策：只实现 cancel + retry，不实现 pause/resume/inject）
- [ ] team-card 展开：显示成员列表，成员展开显示 thinking/action/reply
- [ ] plan 面板：固定位置，支持折叠，状态由 team_stage 事件驱动
- [ ] 进度计算：X/N 简单实现
- [ ] Team 失败：手动重试
- [x] 历史加载：v2 `fetchSessionHistory` + Mode B orphan；子 session steps 展开懒加载（`ensureMemberStepsLoaded`）
- [x] direct-publish 事件：SpiritSessionID 已填充（T1.2/T1.3/T1.3.1 完成）
- [ ] 排序：用 Timestamp，无全局 Seq
- [ ] MaxDepth：从 AgentRuntimeSetting.MaxSessionDepth 读取

---

## 子模块：Grok Build 借鉴（P0/P1 加固，2026-07-20）

> **状态**：✅ 已完成 | **对比分析**：[2026-07-19-analysis-grok-build-function-by-function-comparison.md](../reports/2026-07-19-analysis-grok-build-function-by-function-comparison.md) | **实施计划**：[2026-07-19-grok-insights-p0-p1-implementation.md](../superpowers/plans/2026-07-19-grok-insights-p0-p1-implementation.md)

### D.1 范围

Chat 域落地 3 项 Grok Build 借鉴改进（P0×2 + P1×1）：

| # | 功能 | 优先级 | 问题/收益 |
|---|------|--------|----------|
| D-1 | Tool-pair 安全切分 | P0 | 上下文压缩拆散 assistant tool_call 与 tool result 配对 → API 400 |
| D-2 | 双锚点 token 估算收口 | P0 | 统一 2.5 chars/token 比率不准确，多处独立估算 |
| D-3 | Doom Loop 检测 | P1 | 无 LLM 层重复输出循环检测 |

### D.2 代码锚点

| 文件 | 职责 | 状态 |
|------|------|------|
| `internal/agent/context_compression_inject.go` | `partitionMessagesForCompression` 边界吸附：跨边界的 tool_call/tool_result 配对整体保留在 keep 侧 | ✅ |
| `internal/agent/context_compression_inject_test.go` | Tool-pair 安全回归测试（含多 tool call / 异常顺序边界用例） | ✅ |
| `internal/llmcontext/token_estimator.go` | 统一双锚点估算器：模型权威值回填（RecordAuthoritative）+ 增量字节估算（RecordIncremental）；默认 2.5 chars/token 兜底 | ✅ |
| `internal/llmcontext/token_estimator_test.go` | 估算器单测 | ✅ |
| `internal/agent/prompt_snapshot.go` | `estTokensFromChars` 替换为统一估算器 | ✅ |
| `internal/agent/doom_loop_detector.go` | `DoomLoopDetector`：滑窗 + Jaccard 相似度，连续 N 条高相似输出判定 doom loop | ✅ |
| `internal/agent/doom_loop_detector_test.go` | 重复/近重复检测测试 | ✅ |

### D.3 验收

- [x] `go test ./internal/agent/ -run TestPartitionMessagesForCompression -count=1` 通过（Tool-pair 不拆分）
- [x] `go test ./internal/llmcontext/ -count=1` 通过（双锚点估算）
- [x] `go test ./internal/agent/ -run TestDoomLoopDetector -count=1` 通过
- [x] TDD 流程：失败测试 → 最小实现 → 回归测试
- [x] 向后兼容：压缩策略与快照行为默认值不变

---

## 子模块：使命驱动的任务匹配与团队配方复用（Mission-Driven Matching）

> **状态**：✅ 已实施（2026-07-25） | **需求**：[1-chat.md §子模块：使命驱动的任务匹配与团队配方复用](./1-chat.md)（MM.1-MM.4） | **设计**：[1-chat.design.md §B.10.21](./1-chat.design.md#b1021-使命驱动的任务匹配与团队配方复用2026-07-25-评审版) | **实施计划**：docs/superpowers/plans/2026-07-25-mission-driven-matching.md

### MM-D.1 模块定位

将编排匹配锚点从"单次任务文本"迁移到"使命（Mission）+ 领域路径（domain_path）+ 履历"，修复同类任务重复创建 Agent/Team 的四条根因（R1 Factory key 文本哈希 / R2 findReusableTeam 仅防重试 / R3 OrchestrationCache write-only / R4 中文任务无匹配通道）。改动全部集中在后端 agent/biz/data 三层，无前端改动。

### MM-D.2 代码锚点

| 文件 | 职责 | 状态 |
|------|------|------|
| `internal/agent/domain_lexicon.go` | 领域词表常量 + `NormalizeDomainPath` 归一化（新建） | ✅ |
| `internal/agent/domain_lexicon_test.go` | 词表归一化单测（新建） | ✅ |
| `internal/biz/task_plan.go:105` | `biz.SubTask` 新增 `DomainPath` 字段 | ✅ |
| `internal/biz/agent_capability.go:12` | `AgentCapability` 新增 `Mission`/`DomainPath` | ✅ |
| `internal/biz/agent_factory.go:37` | `TaskProfile` 新增 `DomainPath`/`Mission` | ✅ |
| `internal/biz/spirit_orchestration_cache.go:23` | `OrchestrationCacheEntry` 新增 `DomainPath`；新增 `BestRecipeForDomain`/`RecordDomainRecipe` | ✅ |
| `internal/biz/agent_types.go:148` | `biz.Agent` 新增 `MissionStatement`/`DomainPath` | ✅ |
| `internal/data/ent/schema/agent.go` | agents 表新增 `mission_statement`/`domain_path` 列 | ✅ |
| `internal/data/sql/migrations/20261110_agent_mission_domain.sql` | DDL 迁移（幂等加列，新建） | ✅ |
| `internal/data/ddl_migration_registry.go` | 注册 20261110 迁移 | ✅ |
| `internal/data/agent_repo.go` | 新列读写映射（ent→biz / create→ent） | ✅ |
| `internal/agent/task_planner_impl.go` | `buildDecompositionPrompt` 增加 domain_path 输出约束；`parseDecompositionOutput` 解析+归一化 | ✅ |
| `internal/agent/agent_allocator_impl.go` + `agent_domain_match.go` | `matchSubTask`/`matchWholePlan` 管线重构：新增 L0 domain_recipe / L1 mission 层 | ✅ |
| `internal/agent/agent_factory.go` | `buildDynamicAgentKey` 域派生 key；生成 prompt 输出 mission/domain_path；创建前同域相似复用 | ✅ |
| `internal/agent/task_orchestrator_impl.go` | `learnFromOrchestration` 域派生配方记录 + AgentPerformance TaskType 语义扩展 | ✅ |

### MM-D.3 任务清单

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| T1 | 领域词表 `domain_lexicon.go` + 单测（TDD） | — | ✅ |
| T2 | 数据模型：Ent schema 加列 → DDL 迁移注册 → biz 字段 → repo 映射 | — | ✅ |
| T3 | OrchestrationCache：`DomainPath` 字段 + `BestRecipeForDomain` + 单测 | T1 | ✅ |
| T4 | TaskPlanner：prompt 约束 + 解析 domain_path（顺带，零额外 LLM 调用） | T1 | ✅ |
| T5 | Allocator 管线：L0 domain_recipe + L1 mission 层插入 matchSubTask/matchWholePlan | T2,T3,T4 | ✅ |
| T6 | AgentFactory：key 修正 + definition 解析 mission/domain_path + 同域复用检查 | T1,T2 | ✅ |
| T7 | 学习闭环：learnFromOrchestration 域派生 taskPattern + AgentPerformance TaskType 语义扩展 | T1,T3 | ✅ |
| T8 | 全量验证：build + internal/agent/biz/data 测试 + 回归（DomainPath 空走旧管线） | T1-T7 | ✅ |

### MM-D.4 验收标准

- [x] 同一会话先后发送"写一首关于春天的诗"和"再写一首秋天的诗"：第二次复用第一次的 Agent/配方，不新建 Agent（`agent_domain_match_test.go`：L0 配方短路后续层、L1 同域使命复用）
- [x] 全新领域任务（无候选）：factory 创建的新 Agent 落库时含非空 mission_statement 与 domain_path（`agent_factory_test.go` 出生登记断言；mission 缺省回退 description）
- [x] embedder 未配置环境：匹配管线正常工作（降级路径），不报错（nil-embedder / embedder 错误 / 维度异常三组用例回退 TF-IDF）
- [x] OrchestrationCache 命中时日志出现 `domain_recipe` 匹配层（`MatchLayer: "domain_recipe"` + L0 命中日志）
- [x] 存量 Agent（无使命）参与匹配不崩溃、不阻塞编排（`missionMatchText` 回退 Description；空 DomainPath 跳过 L0/L1 走旧管线）
- [x] `go build ./...` 通过；`go test ./internal/agent/... ./internal/biz/... ./internal/data/...` 全部通过（2026-07-25 全量验证：agent 55.8s / biz 9.6s / data 173.5s / service 5.4s，`make lint` 全绿）

### MM-D.5 改动文件清单（实际）

新建：`internal/agent/domain_lexicon.go`、`internal/agent/domain_lexicon_test.go`、`internal/agent/agent_domain_match.go`、`internal/agent/agent_domain_match_test.go`、`internal/biz/spirit_orchestration_cache_test.go`、`internal/data/sql/migrations/20261110_agent_mission_domain.sql`
修改：`internal/biz/task_plan.go`、`internal/biz/agent_capability.go`、`internal/biz/agent_factory.go`、`internal/biz/agent_types.go`、`internal/biz/spirit_orchestration_cache.go`、`internal/data/ent/schema/agent.go`、`internal/data/ddl_migration_registry.go`、`internal/data/agent_repo.go`、`internal/agent/task_planner_impl.go`、`internal/agent/task_planner_impl_test.go`（domain 解析用例）、`internal/agent/agent_allocator_impl.go`、`internal/agent/agent_factory.go`、`internal/agent/agent_factory_test.go`、`internal/agent/task_orchestrator_impl.go`、`internal/agent/agent_capability_builder.go`、`cmd/admin/wire.go`、`cmd/admin/wire_gen.go`

> 注：learn 闭环（T7）由 `spirit_orchestration_cache_test.go`（配方记录/查询语义）+ `agent_domain_match_test.go`（L0 复用链路）覆盖，未单建 `task_orchestrator_learn_test.go`；repo 列映射（`agent_repo.go`）无独立测试文件，由 ent 生成代码 + 全量 data 包测试覆盖。

---

## 子模块：团队交付物可靠性增强（Deliverable Reliability）

> **状态**：✅ 已实施（2026-07-25） | **设计**：[1-chat.design.md §B.10.22](./1-chat.design.md#b1022-团队交付物可靠性增强2026-07-25-已实施) | **背景**：19:29 对话 Agent 间数据共享失败根因链（G1-G7）

### DR-D.1 模块定位

修复团队"完成≠有产出"的根本语义缺陷：真实产出只认 `set_deliverable` 写入的 graph state；无产出团队翻转 failed 并阻断下游调度；交付协议注入 + DAG 团队无条件开启交付通道；合成报告诚实化；planner 需求澄清优先；member_sessions ID 公式统一；`read_upstream_deliverable` 内容源重排为 state→信封→legacy reply。改动集中在 biz/service/agent/data 后端四层 + DECISION.md 提示层，无前端改动。

### DR-D.2 代码锚点

| 文件 | 职责 | 状态 |
|------|------|------|
| `internal/biz/spirit_team_usecase.go:2004` | `HasRealDeliverable`：真实产出判定（只认 set_deliverable state） | ✅ |
| `internal/biz/spirit_team_usecase.go:1844` | 哨兵错误 `ErrNoRealDeliverable` | ✅ |
| `internal/biz/spirit_team_usecase.go:2188` | `BuildTeamTurnInput`：交付协议注入（Fix 2b） | ✅ |
| `internal/biz/spirit_team_usecase.go:1279` | `buildSpiritTeamDefinitionJSON`：DAG 团队无条件 `enable_state_deliverable` | ✅ |
| `internal/biz/spirit_team_usecase.go:2372` | `resolveDeliverableFullContent`：全文内容源 state→信封→legacy reply（Fix 7） | ✅ |
| `internal/biz/spirit_team_usecase.go:1591` | `ListFailedTeamBriefs`：失败团队简报收集（Fix 3） | ✅ |
| `internal/biz/spirit_synthesis.go:86` | `BuildSynthesisSummaryTrigger`：诚实合成触发文本（Fix 3） | ✅ |
| `internal/biz/team_types.go` | `DeliverableRef` 注释同步（全文源变更） | ✅ |
| `internal/service/spirit_team.go` | `HandleTeamTurnResult` 交付物闸门（无产出→failed）；诚实触发文本/兜底通知接线 | ✅ |
| `internal/service/spirit_team.go:1006` | `SpiritTeamAssembler.BuildTeamTurnInput` 委托 | ✅ |
| `internal/scenario/system/prompts/DECISION.md` | mode 选择规则第 1 条：阻塞性歧义→先澄清禁止组队（Fix 4） | ✅ |
| `internal/scenario/system/embed_test.go` | DECISION.md 澄清优先内容守卫测试 | ✅ |
| `internal/agent/activity_context.go:110` | `NewMemberSessionActivityID`：ID 公式统一（Fix 5） | ✅ |
| `internal/data/member_session_v2_repo.go` | 基于 ID 幂等写入（Fix 5） | ✅ |

### DR-D.3 任务清单

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| T1 | Fix 1+2：`HasRealDeliverable` + service 交付物闸门 + 移除 reply 双兜底 | — | ✅ |
| T2 | Fix 2b：协议注入（`DeliverableProtocolSuffix`/`BuildTeamTurnInput`）+ 定义层无条件 enable | T1 | ✅ |
| T3 | Fix 3：`TeamFailureBrief`/`BuildSynthesisSummaryTrigger` + service 接线 + synthesis 诚实约束 | T1 | ✅ |
| T4 | Fix 4：DECISION.md 澄清优先规则 + 内容守卫测试 | — | ✅ |
| T5 | Fix 5：member_sessions ID 公式统一 + 幂等写入（只修生成逻辑） | — | ✅ |
| T6 | Fix 7：`resolveDeliverableFullContent` 三级内容源 + 用例 | T1 | ✅ |
| T7 | 全量验证 + 设计文档 B.10.22 同步 | T1-T6 | ✅ |

### DR-D.4 验收标准

- [x] DAG 团队 turn 完成但未调 `set_deliverable`：团队状态翻转 failed，下游团队不被调度（`TestHasRealDeliverable_*` 五组用例 + service 闸门测试）
- [x] DAG 单成员团队定义 JSON 必含 `enable_state_deliverable: true`（`buildSpiritTeamDefinitionJSON` requireDeliverable 分支）
- [x] DAG 团队首轮输入必含交付协议且声明上游契约（`TestBuildTeamTurnInput_*` 四组用例）
- [x] 全文读取返回 state 未截断内容（600 字 summary 完整 + 结构化 keys），永不返回 reply 文本；state 不可读时降级信封（`TestReadUpstreamDeliverable_PrefersGraphStateOverReply`/`_GraphStateEmpty_FallsBackToEnvelope`）
- [x] 存在失败团队时合成触发文本如实列出失败，禁止虚构"全部成功"（`TestListFailedTeamBriefs_*`）
- [x] DECISION.md 含澄清优先规则（`TestDecisionPrompt_RequiresClarificationBeforeTeaming` 守卫）
- [x] member session activity ID 与 v2 公式一致（`TestPublishTeamStepActivity_MemberSessionIDMatchesV2Formula`）
- [x] `go build ./...` 通过；`go test ./internal/biz/... ./internal/service/... ./internal/scenario/...` 全部通过（2026-07-25）

### DR-D.5 改动文件清单（实际）

修改：`internal/biz/spirit_team_usecase.go`、`internal/biz/spirit_synthesis.go`、`internal/biz/team_types.go`、`internal/biz/spirit_team_deliverable_test.go`、`internal/biz/spirit_synthesis_report_test.go`、`internal/biz/spirit_team_usecase_test.go`、`internal/service/spirit_team.go`、`internal/service/spirit_team_handle_result_test.go`、`internal/scenario/system/prompts/DECISION.md`、`internal/scenario/system/embed_test.go`、`internal/agent/activity_context.go`、`internal/agent/task_orchestrator_impl.go`、`internal/data/member_session_v2_repo.go`、`internal/team/runner_helpers_test.go`、`cmd/admin/wire.go`、`cmd/admin/wire_gen.go`

---

## 子模块：成员级交付契约（MDC）+ 交付确认（ack）+ MergeReducer

> **状态**：✅ 已实施（2026-07-28，TDD） | **设计**：[1-chat.design.md §B.10.20.9](./1-chat.design.md#b10209-成员级交付契约mdc交付确认ack_deliverablemergereducer2026-07-28-实施-) + [11-multi-agent.design.md §6.5](./11-multi-agent.design.md#65-set_deliverable--get_deliverable--ack_deliverable-工具) | **背景**：团队成员相互交付信息时缺少内容级校验、并行写丢产物、无交付确认语义

### MDC-D.1 模块定位

补全团队内成员间交付的三个缺口：① `deliverable` StateField Reducer Cover→Merge 修复并行 topic 写互相覆盖；② 成员级交付契约（MDC，`deliverable_contract` Definition 字段）对 topic 写做 `required_keys`/`schema_json` 写时强制校验 + `required` 完成时 advisory；③ `ack_deliverable` 工具提供 accepted/rejected 交付确认语义，确认记录经 `ack/<topic>` 顶层键入 state、桥接排除不泄漏进团队间信封。纯后端改动（biz/team/tools），无 DB/proto/前端变更。

### MDC-D.2 代码锚点

| 文件 | 职责 | 状态 |
|------|------|------|
| `internal/biz/member_deliverable_contract.go` | MDC 类型 + `ValidateTopicData`（required_keys + C2 schema 复用）+ `RequiredTopicsMissing` + `MemberContractViolationError`（LLM 可纠错） | ✅ |
| `internal/tools/deliverable/tool.go` | `SetDeliverableTool.contract` + 写时校验点 + `ToolsWithContract` | ✅ |
| `internal/tools/deliverable/ack.go` | `AckDeliverableTool`：Call 校验 + `ack/<topic>` 顶层键写 + StateDelta 从结果确定性重建 | ✅ |
| `internal/team/definition.go` | `Definition.DeliverableContract` 字段 + 空 entries 归一化 | ✅ |
| `internal/team/trpc_build.go` | `deliverableToolsForDef`（契约装配收口）+ `parallelDeliverableAdvisory` | ✅ |
| `internal/team/graph_compile.go` | `CompileToCompiledTeam` 入口 advisory Warn | ✅ |
| `internal/team/graph_runtime_config.go` | `ensureDeliverableStateField` Reducer Cover→Merge | ✅ |
| `internal/biz/spirit_team_usecase.go` | `marshalNonReservedStateKeys` 排除 `ack/` + `requiredTopicsMissingFromState` 完成时 Warn | ✅ |
| `internal/biz/graph.go` | `DeliverableStateKey` 注释同步 Merge 语义 | ✅ |

### MDC-D.3 任务清单（TDD 五步）

| # | 任务 | 状态 |
|---|------|------|
| T1 | biz MDC 类型 + 校验（`member_deliverable_contract_test.go` 9 用例） | ✅ |
| T2 | set 工具契约校验 + ack 工具（`ack_test.go` 10 用例） | ✅ |
| T3 | team Definition 解析 + 装配 + parallel Warn（`deliverable_contract_build_test.go` 6 用例） | ✅ |
| T4 | Reducer Cover→Merge + 并行 union/同 key 覆盖测试（`graph_runtime_options_test.go`） | ✅ |
| T5 | 桥接 `ack/` 排除 + required topic Warn（`member_contract_bridge_test.go`） | ✅ |

### MDC-D.4 验收标准

- [x] 契约违规 topic 写被拒且错误列出全部违规（`TestSetDeliverableTool_ContractViolation_ReturnsStructuredError`）
- [x] 无契约/未声明 topic/nil 契约行为不变（`TestSetDeliverableTool_NilContract_BackwardCompatible` 等）
- [x] 并行成员同 superstep 写不同 topic 均存活（`TestDeliverableMergeReducer_ParallelTopicUnion`）
- [x] ack 记录写 `ack/<topic>` 且不进团队间信封（`TestMarshalNonReservedStateKeys_ExcludesAckKeys`）
- [x] required topic 缺失时完成日志 Warn 不阻断（`TestRequiredTopicsMissingFromState`）
- [x] parallel + deliverable 编译期 advisory（`TestParallelDeliverableAdvisory`）
- [x] `go build ./...` exit 0；`go test ./internal/tools/deliverable/... ./internal/team/... ./internal/biz/... ./internal/graph/... ./internal/service/...` 全部通过；`go vet` 无告警（2026-07-28）

### MDC-D.5 改动文件清单（实际）

新增：`internal/biz/member_deliverable_contract.go`、`internal/biz/member_deliverable_contract_test.go`、`internal/biz/member_contract_bridge_test.go`、`internal/tools/deliverable/ack.go`、`internal/tools/deliverable/ack_test.go`、`internal/team/deliverable_contract_build_test.go`
修改：`internal/tools/deliverable/tool.go`、`internal/team/definition.go`、`internal/team/trpc_build.go`、`internal/team/graph_compile.go`、`internal/team/graph_runtime_config.go`、`internal/team/graph_runtime_options_test.go`、`internal/biz/spirit_team_usecase.go`、`internal/biz/graph.go`、`internal/graph/adapter/runtime_adapter.go`（注释同步）

---

## 子模块：GraphStageBlock 方案A 重写（富卡片 DAG + 视口 + 成员弹框）

> **状态**：✅ 已实施（2026-07-26；2026-07-27 指针捕获修复 + 弹框渲染/实时性修复 + 运行时验证） | **设计**：[1-chat.design.md §B.10.23](./1-chat.design.md#b1023-graphstageblock-方案a-重写2026-07-26-已实施2026-07-27-指针捕获修复)

### GA-D.1 模块定位

将 Graph 流程图从「SVG rect 节点 + TeamStagePanel 折叠列表 + TeamRunCard 成员面板」三层方案，重写为单组件「富卡片 DAG + 缩放/平移视口 + 成员行点击弹框」：节点卡片直接渲染成员行（状态点+名称+耗时），点击成员行弹 MemberSessionDialog 查看执行内容；单节点也始终渲染，UI 不再随节点数跳变。纯前端改动，无后端变更。

### GA-D.2 代码锚点

| 文件 | 职责 | 状态 |
|------|------|------|
| `web/src/components/chat/v2/GraphStageBlock.vue` | 容器：横向 DAG 布局、视口接线、边层 SVG、成员弹框、选中/hover 高亮、级联入场 | ✅ |
| `web/src/components/chat/v2/GraphTeamNode.vue` | 富卡片节点：头部（状态点+标题+徽章）、成员行（点击 emit select-member）、进度条 | ✅ |
| `web/src/components/chat/v2/graphTeamNodeUi.ts` | 尺寸常量（GTN_WIDTH=240 等）+ `graphTeamNodeHeight(n)` + 状态色调/耗时格式化纯函数 | ✅ |
| `web/src/components/chat/v2/MemberSessionDialog.vue` | 成员执行内容弹框：v-model:open 纯展示，内嵌 MemberSessionPanel embedded，事件透传 | ✅ |
| `web/src/components/chat/v2/MemberSessionPanel.vue` | 新增 `embedded` 模式（始终展开、无折叠开关）供弹框复用 | ✅ |
| `web/src/features/chat/composables/useGraphViewport.ts` | 视口状态：scale/tx/ty、zoomAt 锚点缩放、zoomFit、pan 阈值 3px、justPanned 抑制 | ✅ |
| `web/src/features/chat/composables/useGraphNodeTeam.ts` | GraphNode → TeamStage → TeamRun → MemberSession 解析（渲染与 heightOf 布局共用） | ✅ |
| `web/src/features/chat/composables/usePlanDAGLayout.ts` | 横向 DAG 布局支持 per-node `heightOf` 变高节点 | ✅ |
| `web/src/components/chat/v2/TaskCard.vue` | 移除 TeamStagePanel，仅保留 GraphStageBlock | ✅ |
| `web/src/css/app-global.sass` | `.chat-message-prose` / `.code-block` 系列样式改全局作用域（去除 `.chat-page` 前缀，弹框 teleport 后可用） | ✅ 2026-07-27 |
| `web/src/css/theme/_chat-message-panel.sass` | 移除嵌套于 `.chat-message-content` 的 `.code-block` 样式（迁至全局） | ✅ 2026-07-27 |
| `web/src/features/chat/composables/useActivityQueries.ts` | 新增 `memberSessions()` 只读访问器（弹框按 ID 实时查询） | ✅ 2026-07-27 |

已删除：`GraphNode.vue`（SVG rect 节点）、`TeamStagePanel.vue`、`TeamRunCard.vue`、`useLocateTeamStage.ts`、`TeamComponents.spec.ts`。

### GA-D.3 任务清单

| # | 任务 | 状态 |
|---|------|------|
| T1 | GraphTeamNode 富卡片 + graphTeamNodeUi 尺寸纯函数 | ✅ |
| T2 | useGraphViewport（缩放/平移/fit）+ GraphStageBlock 视口接线 | ✅ |
| T3 | usePlanDAGLayout per-node heightOf 变高布局 | ✅ |
| T4 | useGraphNodeTeam 成员解析链路（TeamStageID 优先、DagNodeID 兜底） | ✅ |
| T5 | MemberSessionDialog + MemberSessionPanel embedded 模式 | ✅ |
| T6 | 单节点始终渲染，移除 TeamStagePanel/TeamRunCard/GraphNode/useLocateTeamStage | ✅ |
| T7 | 指针捕获延迟修复（纯点击不 capture，成员弹框真实浏览器可开） | ✅ 2026-07-27 |
| T8 | 单测全套 + 真实浏览器运行时验证 + 文档同步 | ✅ 2026-07-27 |
| T9 | 弹框 Markdown 排版修复：`.chat-message-prose`/`.code-block` 样式改全局作用域 | ✅ 2026-07-27 |
| T10 | 弹框代码块交互修复：body 绑定 `useChatCodeCopy` 事件委托（复制/折叠） | ✅ 2026-07-27 |
| T11 | 弹框数据实时性修复：`activeMember` 改 store 实时查询（停止/输入栏与状态流转不再过期） | ✅ 2026-07-27 |

### GA-D.4 验收标准

- [x] 单节点 GraphStage 也渲染富卡片（`GraphStageBlock.spec.ts`：always renders）
- [x] 成员行渲染且高度驱动布局：b/c 同列时 c 的 top = b.top + graphTeamNodeHeight(3) + gap（spec: lays out nodes with per-node heights）
- [x] 缩放按钮 100%→115%→100%→87%、滚轮缩放 transform 生效（spec: zoom buttons / wheel zooms）
- [x] 拖拽平移 translate 跟随、拖拽后 click 抑制一次、下一轮干净点击恢复（spec: pointer drag pans）
- [x] 纯点击（位移 <3px）不调用 setPointerCapture，成员弹框可开；拖拽超阈值才 capture（spec: defers setPointerCapture）
- [x] 成员行点击开弹框，pause/inject 事件透传（spec: opens MemberSessionDialog）
- [x] 真实浏览器验证（2026-07-27，session `d78029b9-c305-4bc1-9583-ac9f743cdc60`）：5 节点 10 成员行渲染，合成 click 与真实鼠标 click 均打开弹框（标题「市场趋势研究员」）
- [x] 弹框内 Markdown 排版正常（h1-h6 字号、表格边框、代码块头栏/复制/折叠），与聊天消息一致（全局作用域样式）
- [x] 弹框内 running/paused 成员显示底部输入栏：空输入 + running → 停止按钮（pause-agent）；有文字 → 发送按钮（inject-agent），事件链路至 ChatPage → spiritStore.pauseAgent / runtimeStore.enqueue
- [x] 弹框打开期间成员状态流转实时反映（activeMember 实时查询 store，非点击时快照）
- [x] `npx vitest run` 相关 6 个 spec 全绿（GraphStageBlock 9 用例 + entrance + GraphTeamNode + MemberSessionDialog + useGraphViewport + usePlanDAGLayout）

### GA-D.5 改动文件清单（实际）

新建：`web/src/components/chat/v2/GraphTeamNode.vue`、`web/src/components/chat/v2/MemberSessionDialog.vue`、`web/src/components/chat/v2/graphTeamNodeUi.ts`、`web/src/features/chat/composables/useGraphViewport.ts`、`web/src/features/chat/composables/useGraphNodeTeam.ts`、`web/src/components/chat/v2/__tests__/GraphStageBlock.spec.ts`、`web/src/components/chat/v2/__tests__/GraphStageBlock.entrance.spec.ts`、`web/src/components/chat/v2/__tests__/GraphTeamNode.spec.ts`、`web/src/components/chat/v2/__tests__/MemberSessionDialog.spec.ts`、`web/src/features/chat/composables/__tests__/useGraphViewport.spec.ts`

修改：`web/src/components/chat/v2/GraphStageBlock.vue`（重写 + 指针捕获延迟 + activeMember 实时查询）、`web/src/components/chat/v2/MemberSessionDialog.vue`（代码块事件委托）、`web/src/components/chat/v2/MemberSessionPanel.vue`（embedded 模式）、`web/src/components/chat/v2/TaskCard.vue`（移除 TeamStagePanel）、`web/src/components/chat/v2/TurnContainer.vue`、`web/src/components/chat/v2/TurnList.vue`、`web/src/components/chat/v2/SessionPanel.vue`、`web/src/features/chat/composables/usePlanDAGLayout.ts`（heightOf）、`web/src/features/chat/composables/useActivityQueries.ts`（memberSessions 访问器）、`web/src/css/app-global.sass`（markdown/代码块样式全局化）、`web/src/css/theme/_chat-message-panel.sass`（移除嵌套代码块样式）、`web/src/features/chat/composables/__tests__/usePlanDAGLayout.spec.ts`、`web/src/i18n/locales/zh-CN.ts` / `en-US.ts`

删除：`web/src/components/chat/v2/GraphNode.vue`、`web/src/components/chat/v2/TeamStagePanel.vue`、`web/src/components/chat/v2/TeamRunCard.vue`、`web/src/features/chat/composables/useLocateTeamStage.ts`、`web/src/components/chat/v2/__tests__/TeamComponents.spec.ts`

---

## GA-E Graph/弹框/总结增强迭代（2026-07-27）

> **范围**：承接 GA-D（方案A 重写）后的深化迭代——成员弹框可用性（加大/终态注入/任务指令块）、用户输入超限治理、Graph 富卡片视觉/动效/状态感知三方向增强、精灵总结回复显著化。
> **需求**：[1-chat.md §A.4.2 / §A.4.4 / §4.4 / §R.3 FR-9](./1-chat.md)；**设计**：[1-chat.design.md §B.10.17.5 / §B.10.23.5 / §B.10.24](./1-chat.design.md)

### GA-E.1 任务清单

| # | 任务 | 状态 |
|---|------|------|
| T1 | 成员弹框加大（md→lg）+ 底部输入栏终态扩展（canInject 扩至 completed/failed，终态可补充再执行） | ✅ 2026-07-27 |
| T2 | 弹框任务指令块：`memberInstruction` 显示成员任务输入（长内容折叠）；`ensureMemberStepsLoaded` 同载 tasks | ✅ 2026-07-27 |
| T3 | 用户输入超限治理：前端 `USER_INPUT_HARD_LIMIT_CHARS=200000` 双端校验 + 后端 `ToolResultGate.CheckUserInput`（50K blob 转存 / 200K 硬上限拒绝） | ✅ 2026-07-27 |
| T4 | Graph 富卡片视觉层次：running/completed/failed 状态淡底 / 头部状态胶囊徽章 / 状态点光晕 | ✅ 2026-07-27 |
| T5 | Graph 富卡片动效：hover 上浮 / running dot 波纹扩散 / 进度条 shimmer / 成员行 hover 位移（prefers-reduced-motion 全降级） | ✅ 2026-07-27 |
| T6 | Graph 富卡片状态感知：状态行（`graphTeamNodeStatusText` 纯函数——failed Error 优先 / running 成员最新 step）；`GTN_STATUS_ROW_H` 固定一行入布局高度 | ✅ 2026-07-27 |
| T7 | 精灵总结显著化：`SynthesisAuthorAgentKey` 标记链路（TurnInput.Synthesis → ProjectMeta → stepAuthorAgentKey 仅覆盖 reply step）+ ReplyBlock「任务总结」徽章 | ✅ 2026-07-27 |
| T8 | Review（aranea-review 全栈清单）+ 文档同步（本章节 + design/需求对应更新） | ✅ 2026-07-27 |
| T9 | Review 修复①：`graphTeamNodeHeight` 漏算 `.gtn-status-row` margin-bottom 6px（`GTN_ROW_GAP`）——DAG 高度比实际渲染少 6px，`overflow:hidden` 会裁进度条 | ✅ 2026-07-27 |
| T10 | Review 修复②：chat 主链路 `gateTurnUserInput` 补 200K 硬上限拒绝（原仅 50K 转存，API/WS 绕过前端时 >200K 被静默转存）；改返回 `(string, error)` + `chat_orchestrator_turn_pipeline_test.go` | ✅ 2026-07-27 |
| T11 | Review 修复③（lint 阻断）：`SkillUploadPlaceholder.vue` keep-separate 按钮新增硬编码中文 → i18n `skillImport.keepSeparate*`（zh/en） | ✅ 2026-07-27 |

### GA-E.2 验收标准

- [x] 弹框 lg 尺寸，活动列表与输入栏同屏可操作；completed/failed 成员显示输入栏可补充再执行
- [x] 弹框顶部显示成员任务输入（i18n `chat.v2.memberInstruction`），长内容自动折叠
- [x] 输入超 200,000 字符前后端双端拒绝 + Toast 提示；50K–200K 走 blob 转存 + preview 注入（`tool_result_gate_test.go` 覆盖阈值/幂等/source）
- [x] 富卡片 running dot 波纹 / 状态行动作与错误摘要（`GraphTeamNode.spec.ts` 14 用例全绿，含 5 个新增：ripple/动作行/错误行 title 全文/兜底文案/布局常量）
- [x] synthesis turn 的 reply 渲染「任务总结」徽章（`StepBlocks.spec.ts`）；普通 reply 无徽章；synthesis thinking/action step 保持原 agent key（`projector_test.go`）
- [x] Review 修复回归：DAG 高度函数与 CSS 间距账一致（members→status-row 10px + status-row→progress 6px 均入 `graphTeamNodeHeight`）；`gateTurnUserInput` >200K 返回 `CodeBadRequest`（`chat_orchestrator_turn_pipeline_test.go`）
- [x] 回归：后端 `go build ./...` + `go vet` + agent/v2、biz、service 三包测试全绿；前端 lint（0 error，i18n 债务较基线 -16，无新增硬编码）+ 978 测试全绿 + build 通过

### GA-E.3 改动文件清单（实际）

后端：`internal/biz/step.go`（SynthesisAuthorAgentKey 常量）、`internal/biz/turn_input.go`（Synthesis 字段）、`internal/agent/v2/project_meta.go`（Synthesis 透传 + stepAuthorAgentKey）、`internal/service/spirit_team.go`（Synthesis: true 触发）、`internal/agent/v2/projector_test.go`（synthesis 分支测试）、`internal/biz/tool_result_gate.go` + `tool_result_gate_test.go`（CheckUserInput 治理，T3）、`internal/service/chat_orchestrator_turn_pipeline.go`（gateTurnUserInput 硬上限拒绝，T10）+ `chat_orchestrator_turn_pipeline_test.go`（新增）

前端：`web/src/components/chat/ReplyBlock.vue`（synthesis 徽章）、`web/src/components/chat/__tests__/StepBlocks.spec.ts`、`web/src/components/chat/v2/GraphTeamNode.vue`（状态行 + 视觉/动效）、`web/src/components/chat/v2/graphTeamNodeUi.ts`（GTN_STATUS_ROW_H + graphTeamNodeStatusText）、`web/src/components/chat/v2/__tests__/GraphTeamNode.spec.ts`（+5 用例）、`web/src/components/chat/v2/MemberSessionDialog.vue`（lg 尺寸）、`web/src/components/chat/v2/MemberSessionPanel.vue`（canInject 终态扩展 + 任务指令块）、`web/src/stores/chat/activityV2Store.ts`（ensureMemberStepsLoaded 同载 tasks）、`web/src/features/chat/composables/useChatSender.ts`（USER_INPUT_HARD_LIMIT_CHARS）、`web/src/pages/ChatPage.vue`（发送前校验）、`web/src/i18n/locales/zh-CN.ts` / `en-US.ts`（synthesisBadge / memberInstruction / inputTooLong / skillImport.keepSeparate*）、`web/src/components/skills/SkillUploadPlaceholder.vue`（keep-separate 按钮 i18n 化，T11）

文档：`docs/development/1-chat.md`（A.4.2 / A.4.4 / §4.4 / R.2 / R.3 FR-9）、`1-chat.design.md`（B.10.17.4-5 / B.10.23.1/3/4/5 / B.10.24）、`1-chat.development.md`（本章节）

---

## P0-5b RuntimeTooling 按域收窄（2026-08-14）

> **范围**：把 `RuntimeTooling` 从 24 平铺字段拆成 6 个具名域分组，满足 AS-COG-01（struct ≤15）。对外工具装配 / 开关 / biz 调用不变。不拆 `TeamOrchestrationDeps`、不拆 `ChatOrchestrator` 其余字段。
> **设计**：[1-chat.design.md §8.2 RuntimeTooling](./1-chat.design.md)

### 任务清单

| # | 任务 | 状态 |
|---|------|------|
| T1 | 按真实共注入/共 nil-check 聚类为 Knowledge/Skill/Plugin/Bridges/Sharing/Extensions | ✅ 2026-08-14 |
| T2 | 薄 `RuntimeTooling`（6 字段）+ 调用点改为 `rt.Knowledge.xxx`；`provideRuntimeTooling` 同步 | ✅ 2026-08-14 |
| T3 | 文档同步（design §8.2、本块、65 交叉参考 Chat 卡片） | ✅ 2026-08-14 |

### 改动文件清单

新建：`internal/service/chat_runtime_tooling.go`、`internal/service/chat_runtime_tooling_test.go`

修改：`internal/service/chat_orchestrator.go`、`chat_orch_agent_build.go`、`chat.go`、`a2a_endpoint.go`、`openai_compat.go`、`chat_orchestrator_turn_phases.go`、`chat_orchestrator_turn_pipeline.go`、`chat_orchestrator_turn_metrics.go`、`m71_tools.go`、`cmd/admin/wire.go`、`cmd/admin/wire_gen.go`、`docs/development/1-chat.design.md`、`1-chat.development.md`、`65-module-cross-reference-full.md`
