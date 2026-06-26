# 前端 Envelope 彻底删除设计：WS 改为接收 ActivityEvent

> **状态**：设计批准，待写实施计划
> **优先级**：高（ADR-03 Phase 5 Blocker G 前端部分 + 分析报告 §11 Phase 3 Task 8）
> **策略**：单次大提交（用户选择方案 1）
> **来源**：
> - [2026-06-25-analysis-chat-module-refactor.md](../../reports/2026-06-25-analysis-chat-module-refactor.md) §11 Phase 3 Task 8
> - [2026-06-26-review-adr-unified-bus-architecture.md](../../reports/2026-06-26-review-adr-unified-bus-architecture.md) Phase 5 Blocker G
> - 后端 [internal/server/ws.go#L223-L227](file:///f:/aranea-agents/internal/server/ws.go#L223-L227) 明确声明 "WS replay path has been removed"

---

## 1. 背景与目标

### 1.1 现状

后端 Phase 5 Blocker A-G 已全部完成：
- 后端不再发送 `envelope` 消息，chat/system 事件改为 `activity_event`
- WS replay 路径已删除（Blocker A），客户端重连后通过 `ListActivities` RPC 获取历史
- `last_event_id` URL 参数仅在后端 `connected` payload 中回显，不触发 replay
- `replay_start`/`replay_end` 消息后端不再发送

前端残留大量 Envelope 死代码：
- `envelope.ts`（373 行类型定义）—— 仅 5 个 Spirit payload type 仍被 `stores/spirit/index.ts` 使用
- `dispatcher.ts`（117 行）—— `EnvelopeDispatcher.dispatch` 永不触发（后端不发 envelope）
- `data_channel.ts`（74 行）—— 0 外部调用方
- `event_replay.ts`（92 行）—— `RevisionTracker` 跟踪 `msg.envelope.session_revision`，WS replay 已删除
- `ws-transport.ts` 中 `onEnvelope`/`revisionTracker`/`replay_*`/`onReplayState` 全部死代码
- `globalWsHub.ts` 中 `GlobalWsConsumer.onEnvelope` 字段 + 3 个 no-op `onEnvelope: () => {}` 消费者
- `useEnvelopeStream.ts` 中 `EnvelopeDispatcher` 实例 + `onType`/`onChannel` API 死代码
- `features/chat/useEnvelopeStream.ts` 中 3 个死函数（`useChatStream`/`useTeamStream`/`useMonitorStream`）
- `onReplayState` 透传链贯穿 `useEnvelopeStream.ts`/`features/chat/useEnvelopeStream.ts`/`features/teams/api.ts`/`stores/teams/index.ts`/`features/chat/composables/useChatStreamManager.ts`

### 1.2 目标

- **删除** Envelope 类型系统、EnvelopeDispatcher、data_channel、WS replay 机制、onEnvelope/onReplayState 回调链
- **迁移** 5 个 Spirit payload type 到 `stores/spirit/index.ts` 内联
- **保留** 已是 Activity-First 实现的活路径（`createEnvelopeStream`/`useEnvelopeStream` 工厂函数、`onActivityEvent`/`onMonitorEvent` 透传、`inboundSyncRouting.ts`）
- **对齐** 分析报告 §2.2 "删除 Envelope 结构体" + "删除 RouteChannel，统一 chat"

### 1.3 非目标

- 不重命名文件（用户选择方案 B 而非方案 C）
- 不重构 `useEnvelopeStream.ts` 的活路径（仅删死代码）
- 不新增测试（纯死代码删除，无行为变更）

---

## 2. 设计决策

### 2.1 WsDownstream/WsUpstream 类型迁移

`envelope.ts` 删除后，`WsDownstream`/`WsUpstream` 类型需迁移。决策：**内联到 `ws-transport.ts`**。

理由：
- `WsDownstream` 是 WS 消息形状，`ws-transport.ts` 是其唯一消费者和真相源
- `WsUpstream` 同理，由 `ws-transport.ts` 的 `send`/`subscribe`/`cancel` 等函数构造
- 内联避免新建文件，符合 YAGNI

```typescript
// ws-transport.ts 内联定义
export type WsDownstream = {
  direction: 'server_to_client';
  channel: string;
  type?: string;
  payload?: unknown;
  activity_event?: ActivityEvent;
  monitor_event?: MonitorEvent;
  // envelope? 字段删除
};

export type WsUpstream = {
  direction: 'client_to_server';
  channel: string;
  type: string;
  request_id?: string;
  payload?: unknown;
};
```

`event_replay.ts` 删除后，`WsUpstream` import 改为从 `ws-transport.ts` 导入。

### 2.2 _lastEventId 跟踪简化

当前 `ws-transport.ts` 从两处更新 `_lastEventId`：
1. `msg.envelope.id`（line 178）—— 死代码（后端不发 envelope）
2. `connected` payload 的 `last_event_id`（line 138）—— 仍存活（后端回显）

决策：**保留 connected payload 的 `_lastEventId` 更新**，删除 envelope 路径。`_lastEventId` 仍传入 `buildWsUrl` 作为 URL 参数（后端回显用于关联，不触发 replay）。

### 2.3 Spirit payload 迁移

5 个 Spirit payload type 从 `envelope.ts` 迁移到 `stores/spirit/index.ts` 顶部内联定义：

```typescript
// stores/spirit/index.ts 顶部
type SpiritPlanCreatedPayload = { ... };
type SpiritAllocationCreatedPayload = { ... };
type SpiritOrchestrationStartedPayload = { ... };
type SpiritOrchestrationCheckpointPayload = { ... };
type SpiritOrchestrationInterruptedPayload = { ... };
```

理由：仅 `stores/spirit/index.ts` 一个消费者，作为 `ActivityEvent.activity.meta` 的类型视图。

---

## 3. 文件变更清单

### 3.1 删除文件（4 个）

| 文件 | 行数 | 死代码证据 |
|------|------|-----------|
| `web/src/realtime/envelope.ts` | 373 | Spirit payloads 迁出后全死；`resolveEnvelopeTurnId/Source/Revision` 0 调用方 |
| `web/src/realtime/dispatcher.ts` | 117 | `EnvelopeDispatcher.dispatch` 永不触发（后端不发 envelope） |
| `web/src/realtime/data_channel.ts` | 74 | 0 外部调用方（`subscribeToDataChannel`/`subscribeToDataTypes`） |
| `web/src/realtime/event_replay.ts` | 92 | `RevisionTracker` 跟踪 `msg.envelope.session_revision`；WS replay 已删除（后端 ws.go:223-227） |

### 3.2 修改文件（14 个）

#### `web/src/realtime/ws-transport.ts`
- 删除 `import type { Envelope, WsDownstream, WsUpstream } from './envelope'`
- 内联 `WsDownstream`（删 `envelope?` 字段）和 `WsUpstream` 类型
- 删除 `onEnvelope` 选项（`WsTransportOptions`）
- 删除 `onReplayState` 选项
- 删除 `revisionTracker` 变量及其所有引用
- 删除 `msg.envelope` 处理块（line 177-184）
- 删除 `replay_start`/`replay_end` 处理块（line 157-167）
- 删除 `import { RevisionTracker, requestSyncReplay } from './event_replay'`
- 删除 `hasConnectedBefore` 变量及 sync replay 逻辑（line 102, 145-151）—— `hasConnectedBefore` 仅用于触发 sync replay
- 保留 `_lastEventId` 从 connected payload 更新（line 138）

#### `web/src/realtime/globalWsHub.ts`
- 删除 `import type { Envelope } from './envelope'`
- 删除 `GlobalWsConsumer.onEnvelope` 字段
- 删除 `onEnvelope` 回调链（ensureHubTransport 内，line 64-70）
- 删除 `acquireGlobalWsConsumer` 中 `onEnvelope: opts.onEnvelope` 赋值（line 121）

#### `web/src/realtime/useEnvelopeStream.ts`
- 删除 `import { EnvelopeDispatcher } from './dispatcher'`
- 删除 `import type { Envelope, EnvelopeType } from './envelope'`
- 删除 `EnvelopeDispatcher` 实例创建
- 删除 `onType`/`onChannel` API（已无调用方——见下方 `useChatStreamManager`/`teams/api.ts` 改动）
- 删除 `onEnvelope: (env) => dispatcher.dispatch(env)` 回调（2 处，line 94 和 line 124）
- 删除 `onReplayState` 选项和透传
- 保留 `createEnvelopeStream`/`useEnvelopeStream` 工厂函数
- 保留 `onActivityEvent`/`onMonitorEvent` 透传
- `WsDownstream`/`WsUpstream` import 改为从 `./ws-transport` 导入

#### `web/src/features/chat/useEnvelopeStream.ts`
- 删除 `useChatStream`/`useTeamStream`/`useMonitorStream` 3 个死函数（0 调用方，含 10 处 `stream.onType` 调用）
- 删除 `onReplayState` 选项和透传
- 保留 `createChatStream`/`createTeamStream` 工厂函数
- 更新 import（`WsDownstream`/`WsUpstream` 改从 `realtime/ws-transport`）

#### `web/src/features/chat/composables/useChatStreamManager.ts`
- 删除 `subscribeSessionStream` 函数（调用 `stream.onType`，已被 event inspector 使用但实际死代码——backend 不发 envelope）
- 删除 `onReplayState` 消费者（2 处，line 74 和 line 102）
- 删除 `EnvelopeType` import（`WsUpstream` 改从 `realtime/ws-transport` 导入）

#### `web/src/features/chat/composables/useChatEventInspector.ts`
- 删除 `subscribe` 字段从 `ChatEventInspectorStreamDeps` 类型
- 删除 `LIVE_TYPES` 常量（EnvelopeType 字符串列表）
- 删除 `streamDeps?.subscribe` 调用和 `unsubLive` 变量（line 117-122, 87, 132-133）
- 保留 `events` ref、`upsertEvent`、`useEventFilter`（UI 基础设施，非 Envelope 特定）

#### `web/src/features/chat/composables/useChatTraceAndArtifacts.ts`
- 删除 `traceStreamDeps` 中的 `subscribe: streamManager.subscribeSessionStream`（line 65）

#### `web/src/features/chat/composables/useChatSender.ts`
- 更新 `WsUpstream` import（从 `realtime/ws-transport` 导入，line 13）

#### `web/src/features/teams/api.ts`
- 删除 `subscribeTeamRunEventsWs` 中的 `stream.onType('error', ...)` 处理器（line 284-286，死代码——backend 不发 envelope）
- 删除 `onError` 参数（仅被上述死处理器调用）
- 删除 `onReplayState` 参数透传（line 267, 275）
- 更新 import（`WsUpstream` 改从 `realtime/ws-transport` 导入）

#### `web/src/stores/teams/index.ts`
- 删除 `subscribeRunEvents` 的 `onError`/`onReplayState` 参数（line 182-183, 185）

#### `web/src/features/teams/useTeamsPage.ts`
- 删除 `onError` 回调（line 304-306，死代码——仅被 `stream.onType('error')` 触发，后者永不触发）
- 删除 `onReplayState` 回调（line 307-309，死代码——backend 不发 replay 消息）
- 删除 `runEventsReplaying` ref（line 81，仅被死 `onReplayState` 回调设置）
- 保留 `runEventsConnected` ref（活——由事件接收和 close 设置）

#### `web/src/pages/TeamsPage.vue`
- 删除 `:live-replaying="runEventsReplaying"` prop 传递（line 149）
- 删除 `runEventsReplaying` 从 useTeamsPage 解构（line 212）

#### `web/src/components/teams/TeamRunsDialog.vue`
- `liveReplaying` prop 改为可选（`liveReplaying?: boolean`），因为 `TeamsPage` 不再传递此 prop
- 保留 `v-if="liveReplaying"` banner 模板（防御性，永不显示）

#### `web/src/stores/spirit/index.ts`
- 删除 `import { SpiritPlanCreatedPayload, ... } from '../../realtime/envelope'`
- 在文件顶部内联定义 5 个 Spirit payload type

### 3.3 删除 no-op 消费者（3 处）

| 文件 | 行号 | 改动 |
|------|------|------|
| `web/src/composables/useGlobalInboundNotifications.ts` | 128 | 删除 `onEnvelope: () => {}` |
| `web/src/features/chat/useChatBackgroundJobs.ts` | 122 | 删除 `onEnvelope: () => {}` |
| `web/src/features/chat/composables/useChatInboundSync.ts` | 421 | 删除 `onEnvelope: () => {}` |

### 3.4 无需修改的文件

- `web/src/realtime/activityEvent.ts` —— 已独立
- `web/src/realtime/monitorEvent.ts` —— 已独立
- `web/src/features/chat/inboundSyncRouting.ts` —— 已是 Activity-First，不依赖 Envelope

---

## 4. 数据流（清理后）

```
后端 WS → WsDownstream { activity_event?, monitor_event?, type, payload }
  ↓ ws-transport.ts onmessage
  ├── msg.type === 'connected' → onConnected (含 last_event_id 回显，仅关联用)
  ├── msg.type === 'pong' → 忽略
  ├── msg.type === 'server_shutdown' → onServerShutdown
  ├── msg.activity_event → opts.onActivityEvent?.(ev)
  └── msg.monitor_event → opts.onMonitorEvent?.(event)
  ↓
globalWsHub → consumer.onActivityEvent?/onMonitorEvent?
  ↓
useEnvelopeStream (工厂函数保留) → opts.onActivityEvent/onMonitorEvent
  ↓
useActivityTimeline / inboundSyncRouting (已是 Activity-First)
```

---

## 5. 验证策略

### 5.1 必须通过的验证

```bash
cd web
pnpm lint    # ESLint + Prettier
pnpm test    # Vitest
pnpm build   # Vite production build
```

### 5.2 验证点

1. **import 链完整**：删除 `envelope.ts` 后，所有原 `import from './envelope'` 已迁移或删除
2. **活路径保留**：`createEnvelopeStream`/`useEnvelopeStream` 仍被 chat/teams/monitor/graph/orchestration/knowledge 使用
3. **`WsDownstream`/`WsUpstream` 迁移**：所有引用改为从 `ws-transport.ts` 导入
4. **Spirit payloads 内联**：`stores/spirit/index.ts` 不再依赖 `envelope.ts`
5. **无残留 `Envelope`/`EnvelopeDispatcher`/`EnvelopeType` 引用**

### 5.3 无新增测试

纯死代码删除，无行为变更。现有测试已覆盖活路径（Activity-First 渲染、Spirit store、Teams store）。

---

## 6. 风险与缓解

### 6.1 `useEnvelopeStream.ts` 部分存活

**风险**：误删活路径（`createEnvelopeStream`/`useEnvelopeStream` 工厂函数、`onActivityEvent`/`onMonitorEvent` 透传）。

**缓解**：仅删 `EnvelopeDispatcher` 实例、`onType`/`onChannel` API、`onEnvelope` 回调、`onReplayState` 透传。工厂函数和 Activity/Monitor 透传完整保留。

### 6.2 Event Inspector subscribe 链删除

**风险**：`subscribeSessionStream` 被事件 inspector 使用，删除后 events tab 不再接收实时事件。

**缓解**：该 subscribe 链已是死代码（`EnvelopeDispatcher.dispatch` 永不触发，backend 不发 envelope）。events tab 本就显示空白。inspector 仍通过 `listActivities` RPC + `onReconnect` 获取历史数据。`events` ref、`upsertEvent`、`useEventFilter` 保留（UI 基础设施，非 Envelope 特定）。

### 6.3 Teams Page `runEventsReplaying` 删除

**风险**：`runEventsReplaying` 在 `TeamsPage.vue` 中作为 `:live-replaying` prop 传递。

**缓解**：该 ref 仅被 `onReplayState` 回调设置，后者永不触发（backend 不发 `replay_start`/`replay_end`）。删除 `:live-replaying` prop 后，TeamsPage 组件需确认该 prop 是可选的或有默认值。

### 6.4 `WsDownstream`/`WsUpstream` 迁移

**风险**：多个文件 import 这两个类型，迁移后需同步更新（`ws-transport.ts`/`useEnvelopeStream.ts`/`features/chat/useEnvelopeStream.ts`/`useChatSender.ts`/`features/teams/api.ts`）。

**缓解**：迁移到 `ws-transport.ts` 后，全局搜索 `from.*envelope` 确保无残留。

### 6.5 单次大提交

**风险**：~21 文件变更，难部分回退。

**缓解**：用户明确选择方案 1；所有变更均为死代码删除或类型迁移，无行为变更，`pnpm lint && pnpm test && pnpm build` 全过即安全。

---

## 7. 完成标准

- [ ] 4 个文件删除：`envelope.ts`/`dispatcher.ts`/`data_channel.ts`/`event_replay.ts`
- [ ] 14 个文件修改（见 §3.2）
- [ ] 3 个 no-op 消费者删除
- [ ] `pnpm lint && pnpm test && pnpm build` 全过
- [ ] 全局搜索无残留 `Envelope`/`EnvelopeDispatcher`/`EnvelopeType` 引用（`monitorEvent.ts` 注释除外）
- [ ] 全局搜索无残留 `from.*envelope` import
- [ ] ADR-03 Blocker G 前端部分标记完成
- [ ] 分析报告 §11 Phase 3 Task 8 标记 ✅
