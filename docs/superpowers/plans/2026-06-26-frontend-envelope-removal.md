# 前端 Envelope 彻底删除实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 删除前端 Envelope 类型系统、EnvelopeDispatcher、data_channel、WS replay 机制、onEnvelope/onReplayState 回调链；将 5 个 Spirit payload type 内联到 stores/spirit；将 WsDownstream/WsUpstream 内联到 ws-transport.ts。

**Architecture:** 纯死代码删除 + 类型迁移。后端 Phase 5 Blocker A-G 已全部完成，前端 Envelope 路径全部为死代码（EnvelopeDispatcher.dispatch 永不触发，backend 不发 envelope）。Activity-First 路径（onActivityEvent/onMonitorEvent 透传 + useActivityTimeline + inboundSyncRouting）完整保留。用户选择方案 1（单次大提交），所有变更一次性完成 + 单次 commit。

**Tech Stack:** Vue 3 + TypeScript + Pinia + Vitest + ESLint + Vite

**Spec:** [2026-06-26-frontend-envelope-removal-design.md](../specs/2026-06-26-frontend-envelope-removal-design.md)

---

## 文件结构总览

### 删除文件（4 个）

| 文件 | 行数 | 责任 |
|------|------|------|
| `web/src/realtime/envelope.ts` | 373 | Envelope 类型 + WsDownstream/WsUpstream + Spirit payloads |
| `web/src/realtime/dispatcher.ts` | 117 | EnvelopeDispatcher（dispatch 永不触发） |
| `web/src/realtime/data_channel.ts` | 74 | subscribeToDataChannel/subscribeToDataTypes（0 调用方） |
| `web/src/realtime/event_replay.ts` | 92 | RevisionTracker/requestSyncReplay（WS replay 已删除） |

### 修改文件（15 个）

| 文件 | 改动 |
|------|------|
| `web/src/realtime/ws-transport.ts` | 内联 WsDownstream/WsUpstream；删除 onEnvelope/onReplayState/revisionTracker/replay_*/hasConnectedBefore/msg.envelope 块 |
| `web/src/realtime/globalWsHub.ts` | 删除 GlobalWsConsumer.onEnvelope 字段 + ensureHubTransport onEnvelope 回调链 |
| `web/src/realtime/useEnvelopeStream.ts` | 删除 EnvelopeDispatcher 实例/onType/onChannel/onEnvelope/onReplayState 透传 |
| `web/src/features/chat/useEnvelopeStream.ts` | 删除 useChatStream/useTeamStream/useMonitorStream 3 个死函数 + onReplayState 透传 |
| `web/src/features/chat/composables/useChatStreamManager.ts` | 删除 subscribeSessionStream/onReplayState 消费者/EnvelopeType import |
| `web/src/features/chat/composables/useChatEventInspector.ts` | 删除 subscribe 字段/LIVE_TYPES/unsubLive 调用 |
| `web/src/features/chat/composables/useChatTraceAndArtifacts.ts` | 删除 traceStreamDeps.subscribe |
| `web/src/features/chat/composables/useChatSender.ts` | WsUpstream import 改从 realtime/ws-transport |
| `web/src/features/teams/api.ts` | 删除 stream.onType('error')/onError/onReplayState 参数 |
| `web/src/stores/teams/index.ts` | 删除 subscribeRunEvents 的 onError/onReplayState 参数 |
| `web/src/features/teams/useTeamsPage.ts` | 删除 runEventsReplaying/onError/onReplayState 回调 |
| `web/src/pages/TeamsPage.vue` | 删除 :live-replaying prop + runEventsReplaying 解构 |
| `web/src/components/teams/TeamRunsDialog.vue` | liveReplaying prop 改为可选 |
| `web/src/stores/spirit/index.ts` | 内联 5 个 Spirit payload type；删除 envelope import |
| `web/src/features/chat/streamEventTypes.ts` | 删除 JSDoc 中 EnvelopeError.code 提及 |

### 删除 no-op 消费者字段（3 处）

| 文件 | 改动 |
|------|------|
| `web/src/composables/useGlobalInboundNotifications.ts:128` | 删除 `onEnvelope: () => {}` |
| `web/src/features/chat/useChatBackgroundJobs.ts:122` | 删除 `onEnvelope: () => {}` |
| `web/src/features/chat/composables/useChatInboundSync.ts:421` | 删除 `onEnvelope: () => {}` |

---

## Task 1: 内联 Spirit payload types 到 stores/spirit/index.ts

**Files:**
- Modify: `web/src/stores/spirit/index.ts:22-28` (删除 envelope import)
- Modify: `web/src/stores/spirit/index.ts` (在 `import` 后添加 5 个内联类型)

- [ ] **Step 1: 替换 envelope import 为本地类型定义**

打开 `web/src/stores/spirit/index.ts`，将第 22-28 行的 import 块：

```typescript
import type {
  SpiritPlanCreatedPayload,
  SpiritAllocationCreatedPayload,
  SpiritOrchestrationStartedPayload,
  SpiritOrchestrationCheckpointPayload,
  SpiritOrchestrationInterruptedPayload,
} from '../../realtime/envelope';
```

替换为内联定义（放在 `import type { ActivityEvent }` 之后，`import { Notify }` 之前）：

```typescript
// --- Spirit Orchestration Event Payloads (inlined from deleted envelope.ts) ---
// These are type views over ActivityEvent.activity.meta for the spirit orchestration
// channel. The backend publishes them as ActivityEvent with kind=spirit_*.
type SpiritPlanCreatedPayload = {
  plan_id: string;
  spirit_session_id: string;
  complexity_level: string;
  complexity_score: number;
  strategy: string;
  strategy_reason: string;
  topology_hint: string;
  subtask_count: number;
};

type SpiritAllocationCreatedPayload = {
  allocation_id: string;
  task_plan_id: string;
  spirit_session_id: string;
  allocation_count: number;
  status: string;
};

type SpiritOrchestrationStartedPayload = {
  orchestration_id: string;
  spirit_session_id: string;
  strategy: string;
  status: string;
  task_plan_id: string;
  allocation_id: string;
  team_ids?: string[];
  max_concurrent_teams?: number;
};

type SpiritOrchestrationCheckpointPayload = {
  orchestration_id: string;
  spirit_session_id: string;
  checkpoint_id: string;
  step: string;
  status: string;
};

type SpiritOrchestrationInterruptedPayload = {
  orchestration_id: string;
  spirit_session_id: string;
  status: string;
};
```

- [ ] **Step 2: 验证 Spirit store 编译通过**

Run: `cd web && pnpm exec tsc --noEmit -p tsconfig.json 2>&1 | grep -i spirit | head -20`
Expected: 无输出（无 Spirit 相关错误）

注：此时 `envelope.ts` 仍存在，所以不会破坏其他文件。本任务只是把 spirit 解耦出来，为后续删除 envelope.ts 铺路。

---

## Task 2: 内联 WsDownstream/WsUpstream 到 ws-transport.ts

**Files:**
- Modify: `web/src/realtime/ws-transport.ts:12` (删除 envelope import，内联 WsDownstream/WsUpstream)

- [ ] **Step 1: 替换 envelope import 为内联类型定义**

打开 `web/src/realtime/ws-transport.ts`，将第 12 行：

```typescript
import type { Envelope, WsDownstream, WsUpstream } from './envelope';
```

替换为：

```typescript
import type { ActivityEvent } from './activityEvent';
import type { MonitorEvent } from './monitorEvent';

/**
 * WS downstream message shape. The single source of truth for what the
 * backend sends over `/v1/ws`. Carries one of:
 * - control messages (connected/pong/server_shutdown/replay_*)
 * - business events (activity_event for chat/system, monitor_event for monitor)
 *
 * The legacy `envelope?` field has been removed (Phase 5 Blocker G frontend).
 */
export type WsDownstream = {
  direction: 'server_to_client';
  channel: string;
  type?: string;
  payload?: unknown;
  activity_event?: ActivityEvent;
  monitor_event?: MonitorEvent;
};

/**
 * WS upstream message shape. Sent by the client over `/v1/ws` for
 * user_message/cancel/subscribe/unsubscribe/ping/enable_log/sync_request.
 */
export type WsUpstream = {
  direction: 'client_to_server';
  channel: string;
  type: string;
  request_id?: string;
  payload?: unknown;
};
```

注意：原第 13-14 行的 `import type { ActivityEvent }` 和 `import type { MonitorEvent }` 已包含在新替换块中，需删除原 13-14 行（避免重复 import）。

- [ ] **Step 2: 验证 ws-transport 编译通过**

Run: `cd web && pnpm exec tsc --noEmit -p tsconfig.json 2>&1 | grep -i ws-transport | head -20`
Expected: 无输出

注：此时 `envelope.ts` 仍存在但已无外部引用此两类型；其他文件仍从 `./envelope` 导入 `WsDownstream`/`WsUpstream`（但 envelope.ts 自身只是 re-export 或原定义？需检查）。实际 envelope.ts 仍定义这两个类型——本步骤会让 ws-transport.ts 内联自己的副本，envelope.ts 中的定义稍后在 Task 3 删除整个文件时一并清除。**此时 TypeScript 会因为存在两个同名 type 而冲突吗？** 不会——它们在不同模块中定义，导入路径决定使用哪个。但为避免混乱，Task 3 删除 envelope.ts 时会清除。

---

## Task 3: 删除 4 个死文件

**Files:**
- Delete: `web/src/realtime/envelope.ts`
- Delete: `web/src/realtime/dispatcher.ts`
- Delete: `web/src/realtime/data_channel.ts`
- Delete: `web/src/realtime/event_replay.ts`

- [ ] **Step 1: 删除 envelope.ts**

```bash
rm web/src/realtime/envelope.ts
```

- [ ] **Step 2: 删除 dispatcher.ts**

```bash
rm web/src/realtime/dispatcher.ts
```

- [ ] **Step 3: 删除 data_channel.ts**

```bash
rm web/src/realtime/data_channel.ts
```

- [ ] **Step 4: 删除 event_replay.ts**

```bash
rm web/src/realtime/event_replay.ts
```

- [ ] **Step 5: 验证全局无残留 import（预期有编译错误）**

Run: `cd web && pnpm exec tsc --noEmit -p tsconfig.json 2>&1 | head -40`
Expected: 多个文件报 "Cannot find module './envelope'" / "'./dispatcher'" / "'./event_replay'" 等错误。这些错误将在后续 Task 4-12 中逐一修复。

---

## Task 4: 清理 ws-transport.ts（移除 envelope/replay 代码路径）

**Files:**
- Modify: `web/src/realtime/ws-transport.ts` (多处删除)

- [ ] **Step 1: 删除 event_replay import**

打开 `web/src/realtime/ws-transport.ts`，删除第 15 行：

```typescript
import { RevisionTracker, requestSyncReplay } from './event_replay';
```

- [ ] **Step 2: 删除 WsTransportOptions.onEnvelope 字段**

删除 `WsTransportOptions` 类型中的 `onEnvelope` 字段（约第 51 行）：

```typescript
  onEnvelope?: (env: Envelope) => void;
```

- [ ] **Step 3: 删除 WsTransportOptions.onReplayState 字段**

删除 `WsTransportOptions` 类型中的 `onReplayState` 字段及其 JSDoc（约第 69-70 行）：

```typescript
  /** Fired when EventBuffer replay starts/ends (reconnect with last_event_id). */
  onReplayState?: (replaying: boolean, count?: number) => void;
```

- [ ] **Step 4: 删除 hasConnectedBefore 变量及其注释**

删除第 99-102 行的注释和变量声明：

```typescript
  // T3.4: Track whether we've ever connected before, to detect reconnects.
  // reconnectAttempts is reset to 0 in onopen, so we can't rely on it in the
  // connected message handler (which arrives after onopen).
  let hasConnectedBefore = false;
```

- [ ] **Step 5: 删除 revisionTracker 变量及其注释**

删除第 106-109 行的注释和变量声明：

```typescript
  // T3.4: Per-session revision tracker for sync_request replay after reconnect.
  // Updated on every envelope carrying session_revision; used on reconnect to
  // request replay of envelopes with revision > last known.
  const revisionTracker = new RevisionTracker();
```

- [ ] **Step 6: 删除 connected 处理器中的 sync replay 块**

在 `ws.onmessage` 的 `connected` 分支中，删除第 140-151 行的 sync replay 逻辑：

```typescript
          // T3.4: After reconnect, request revision-based sync replay.
          // The server replays envelopes with session_revision > last known.
          // This complements event-ID-based replay (via lastEventId in URL)
          // by ensuring message-level consistency for envelopes persisted
          // during the disconnection window.
          if (hasConnectedBefore) {
            const lastRevision = revisionTracker.get(opts.sessionId);
            if (lastRevision > 0) {
              requestSyncReplay(send, opts.sessionId, lastRevision);
            }
          }
          hasConnectedBefore = true;
```

保留前后代码：保留 `_lastEventId` 更新和 `opts.onConnected?.()` 调用。

- [ ] **Step 7: 删除 replay_start/replay_end 处理块**

删除第 157-167 行：

```typescript
        if (msg.type === 'replay_start') {
          const payload = msg.payload as Record<string, unknown> | undefined;
          const count = typeof payload?.count === 'number' ? payload.count : undefined;
          opts.onReplayState?.(true, count);
          return;
        }

        if (msg.type === 'replay_end') {
          opts.onReplayState?.(false);
          return;
        }
```

- [ ] **Step 8: 删除 msg.envelope 处理块**

删除第 177-184 行：

```typescript
        if (msg.envelope) {
          _lastEventId = msg.envelope.id;
          // T3.4: Track session_revision for sync_request replay.
          if (msg.envelope.session_revision && msg.envelope.session_revision > 0) {
            revisionTracker.update(opts.sessionId, msg.envelope.session_revision);
          }
          opts.onEnvelope?.(msg.envelope);
        }
```

保留后面 `msg.activity_event` 和 `msg.monitor_event` 处理块。

- [ ] **Step 9: 验证 ws-transport.ts 编译通过**

Run: `cd web && pnpm exec tsc --noEmit -p tsconfig.json 2>&1 | grep "ws-transport" | head -10`
Expected: 无输出（ws-transport 本身编译通过；其他文件可能仍报错）

---

## Task 5: 清理 globalWsHub.ts

**Files:**
- Modify: `web/src/realtime/globalWsHub.ts:12,20,64-70,121`

- [ ] **Step 1: 删除 Envelope import**

打开 `web/src/realtime/globalWsHub.ts`，删除第 12 行：

```typescript
import type { Envelope } from './envelope';
```

- [ ] **Step 2: 删除 GlobalWsConsumer.onEnvelope 字段**

删除 `GlobalWsConsumer` 类型中的 `onEnvelope` 字段（第 20 行）：

```typescript
  onEnvelope: (env: Envelope) => void;
```

- [ ] **Step 3: 删除 ensureHubTransport 中的 onEnvelope 回调链**

在 `ensureHubTransport` 函数中，删除第 64-70 行：

```typescript
    onEnvelope: (env) => {
      for (const c of consumers.values()) {
        if (env.channel && c.channels.has(env.channel)) {
          c.onEnvelope(env);
        }
      }
    },
```

保留 `onActivityEvent`、`onMonitorEvent`、`onConnected`、`onDisconnected`、`onServerShutdown` 等回调。

- [ ] **Step 4: 删除 acquireGlobalWsConsumer 中的 onEnvelope 赋值**

在 `acquireGlobalWsConsumer` 函数中，删除第 121 行：

```typescript
    onEnvelope: opts.onEnvelope,
```

- [ ] **Step 5: 验证 globalWsHub 编译通过**

Run: `cd web && pnpm exec tsc --noEmit -p tsconfig.json 2>&1 | grep "globalWsHub" | head -10`
Expected: 无输出

---

## Task 6: 清理 realtime/useEnvelopeStream.ts

**Files:**
- Modify: `web/src/realtime/useEnvelopeStream.ts` (多处删除)

- [ ] **Step 1: 删除 dispatcher/envelope import**

打开 `web/src/realtime/useEnvelopeStream.ts`，删除第 15-16 行：

```typescript
import { EnvelopeDispatcher } from './dispatcher';
import type { Envelope, EnvelopeType } from './envelope';
```

- [ ] **Step 2: 从 UseEnvelopeStreamReturn 删除 dispatcher/onType/onChannel 字段**

在 `UseEnvelopeStreamReturn` 类型中，删除以下字段（约第 63, 66, 67 行）：

```typescript
  dispatcher: EnvelopeDispatcher;
  onType: (type: EnvelopeType | EnvelopeType[], handler: (env: Envelope) => void) => () => void;
  onChannel: (channel: string | string[], handler: (env: Envelope) => void) => () => void;
```

- [ ] **Step 3: 删除 onReplayState 选项**

在 `UseEnvelopeStreamOptions` 类型中，删除第 43 行：

```typescript
  onReplayState?: (replaying: boolean, count?: number) => void;
```

- [ ] **Step 4: 删除 EnvelopeDispatcher 实例创建**

在 `createEnvelopeStream` 函数中，删除第 80 行：

```typescript
  const dispatcher = new EnvelopeDispatcher();
```

- [ ] **Step 5: 删除 globalHub 回调中的 onEnvelope**

在 `acquireGlobalWsConsumer` 调用中，删除第 94 行：

```typescript
        onEnvelope: (env) => dispatcher.dispatch(env),
```

- [ ] **Step 6: 删除 transport onEnvelope 回调**

在 `createWsTransport` 调用中，删除第 124-126 行：

```typescript
      onEnvelope: (env) => {
        dispatcher.dispatch(env);
      },
```

- [ ] **Step 7: 删除 transport onReplayState 回调**

在 `createWsTransport` 调用中，删除第 149-152 行：

```typescript
      onReplayState: (replaying, count) => {
        wsReplaying.value = replaying;
        opts.onReplayState?.(replaying, count);
      },
```

注意：`wsReplaying` ref（第 77 行）保留，作为死状态（永远 false）——UI 消费者（ChatPage/ChatMessagePanel/useChatWorkspace/useChatInboundSync）仍引用它。

- [ ] **Step 8: 删除 disconnect 中的 dispatcher.clear() 调用**

删除第 164 行和第 170 行：

```typescript
      dispatcher.clear();
```

（共两处：一处 in globalHub 分支，一处 in transport 分支）

- [ ] **Step 9: 删除 onType/onChannel 函数定义**

删除第 173-179 行：

```typescript
  function onType(type: EnvelopeType | EnvelopeType[], handler: (env: Envelope) => void): () => void {
    return dispatcher.onType(type, handler);
  }

  function onChannel(channel: string | string[], handler: (env: Envelope) => void): () => void {
    return dispatcher.onChannel(channel, handler);
  }
```

- [ ] **Step 10: 从 return 对象删除 dispatcher/onType/onChannel**

在 `createEnvelopeStream` 的 return 语句中，删除以下三行（约第 218, 221, 222 行）：

```typescript
    dispatcher,
    onType,
    onChannel,
```

- [ ] **Step 11: 验证 useEnvelopeStream 编译通过**

Run: `cd web && pnpm exec tsc --noEmit -p tsconfig.json 2>&1 | grep "useEnvelopeStream" | head -10`
Expected: 无输出

---

## Task 7: 清理 features/chat/useEnvelopeStream.ts（删除 3 个死函数）

**Files:**
- Modify: `web/src/features/chat/useEnvelopeStream.ts` (大段删除)

- [ ] **Step 1: 删除 import 块中的 ref/UseEnvelopeStreamReturn**

打开 `web/src/features/chat/useEnvelopeStream.ts`，将第 28-31 行：

```typescript
import { ref } from 'vue';
import { createEnvelopeStream, useEnvelopeStream } from '../../realtime/useEnvelopeStream';
import { CHAT_ENVELOPE_LOG_LOCAL_MAX } from '../constants/queryLimits';
import type { UseEnvelopeStreamReturn } from '../../realtime/useEnvelopeStream';
```

替换为：

```typescript
import { createEnvelopeStream, useEnvelopeStream } from '../../realtime/useEnvelopeStream';
```

（删除 `ref`、`CHAT_ENVELOPE_LOG_LOCAL_MAX`、`UseEnvelopeStreamReturn` import——它们仅被死函数使用）

- [ ] **Step 2: 删除 ChatStreamFactoryOpts.onReplayState 字段**

在 `ChatStreamFactoryOpts` 类型中，删除第 39 行：

```typescript
  onReplayState?: (replaying: boolean, count?: number) => void;
```

- [ ] **Step 3: 删除 createChatStream 中的 onReplayState 透传**

在 `createChatStream` 函数中，删除第 57 行：

```typescript
    onReplayState: streamOpts?.onReplayState,
```

- [ ] **Step 4: 删除 useChatStream 函数（第 62-135 行整段）**

删除整个 `useChatStream` 函数（包括函数签名、函数体、stream.onType 调用、return 语句）。该函数 0 调用方，含 5 处 `stream.onType` 死代码。

- [ ] **Step 5: 删除 createTeamStream 的 onReplayState 参数和透传**

将 `createTeamStream` 函数（约第 137-155 行）的签名和函数体改为：

```typescript
export function createTeamStream(
  sessionId: string,
  streamOpts?: {
    onConnected?: () => void;
    onServerShutdown?: (reason: string) => void;
    onActivityEvent?: (ev: ActivityEvent) => void;
  },
): UseEnvelopeStreamReturn {
  return createEnvelopeStream({
    sessionId,
    channels: ['chat', 'team', 'system'],
    autoConnect: false,
    onConnected: () => streamOpts?.onConnected?.(),
    onServerShutdown: streamOpts?.onServerShutdown,
    onActivityEvent: streamOpts?.onActivityEvent,
  });
}
```

（删除 `onReplayState` 参数和 `onReplayState: streamOpts?.onReplayState` 透传）

- [ ] **Step 6: 删除 useTeamStream 函数（第 157-204 行整段）**

删除整个 `useTeamStream` 函数。该函数 0 调用方，含 4 处 `stream.onType` 死代码。

- [ ] **Step 7: 删除 useMonitorStream 函数（第 206-237 行整段）**

删除整个 `useMonitorStream` 函数。该函数 0 调用方，含 1 处 `stream.onType` 死代码。

- [ ] **Step 8: 删除 `import type { ActivityEvent }` 检查**

第 32 行 `import type { ActivityEvent } from '../../realtime/activityEvent';` 在 Step 5 后仍被 `createTeamStream` 使用（`onActivityEvent?: (ev: ActivityEvent) => void`），保留不动。

- [ ] **Step 9: 验证 useEnvelopeStream.ts (chat 模块) 编译通过**

Run: `cd web && pnpm exec tsc --noEmit -p tsconfig.json 2>&1 | grep "features/chat/useEnvelopeStream" | head -10`
Expected: 无输出

---

## Task 8: 清理 chat composables（4 个文件）

**Files:**
- Modify: `web/src/features/chat/composables/useChatStreamManager.ts`
- Modify: `web/src/features/chat/composables/useChatEventInspector.ts`
- Modify: `web/src/features/chat/composables/useChatTraceAndArtifacts.ts`
- Modify: `web/src/features/chat/composables/useChatSender.ts`

- [ ] **Step 1: useChatStreamManager.ts - 更新 import**

打开 `web/src/features/chat/composables/useChatStreamManager.ts`，将第 8 行：

```typescript
import type { EnvelopeType, WsUpstream } from '../../../realtime/envelope';
```

替换为：

```typescript
import type { WsUpstream } from '../../../realtime/ws-transport';
```

- [ ] **Step 2: useChatStreamManager.ts - 删除 ensureChatStream 的 onReplayState**

删除 `ensureChatStream` 中的 `onReplayState` 字段（约第 74-76 行）：

```typescript
      onReplayState: (replaying) => {
        wsReplaying.value = replaying;
      },
```

注意：`wsReplaying` ref（第 34 行）保留作为死状态返回给 UI 消费者。

- [ ] **Step 3: useChatStreamManager.ts - 删除 ensureTeamStream 的 onReplayState**

删除 `ensureTeamStream` 中的 `onReplayState` 字段（约第 102-104 行）：

```typescript
      onReplayState: (replaying) => {
        wsReplaying.value = replaying;
      },
```

- [ ] **Step 4: useChatStreamManager.ts - 删除 subscribeSessionStream 函数**

删除整个 `subscribeSessionStream` 函数及其上方 JSDoc 注释（约第 166-186 行）：

```typescript
  /**
   * Subscribe to raw envelope types on the session stream. Used by the event
   * inspector, which consumes InspectorEvent (a minimal local type that
   * captures only the fields the inspector UI accesses). The underlying WS
   * stream still delivers Envelope objects, but they are structurally
   * compatible with InspectorEvent and adapted here.
   *
   * Phase 5 Blocker A: the legacy server-side replay (event.Buffer →
   * replayEvents → Envelope) has been removed. The inspector re-fetches
   * historical Activities via ListActivities RPC on reconnect (handled by
   * the caller via onReconnect).
   */
  function subscribeSessionStream(
    sessionId: string,
    ownerKind: 'agent' | 'team',
    types: string[],
    handler: (env: InspectorEvent) => void,
  ): () => void {
    const stream = ownerKind === 'team' ? ensureTeamStream(sessionId) : ensureChatStream(sessionId);
    return stream.onType(types as EnvelopeType[], (env) => handler(env));
  }
```

- [ ] **Step 5: useChatStreamManager.ts - 从 return 删除 subscribeSessionStream**

在 return 语句中删除 `subscribeSessionStream,` 行（约第 192 行）。

- [ ] **Step 6: useChatStreamManager.ts - 删除未使用 import**

删除第 10 行（如存在）：

```typescript
import type { InspectorEvent } from '../eventFilter';
```

实际查看文件，第 10 行是 `import type { InspectorEvent } from '../eventFilter';` —— 该 import 仅被 `subscribeSessionStream` 使用，删除后者后需删除前者。

- [ ] **Step 7: useChatEventInspector.ts - 删除 LIVE_TYPES 常量**

打开 `web/src/features/chat/composables/useChatEventInspector.ts`，删除第 7-36 行整个 `LIVE_TYPES` 常量定义。

- [ ] **Step 8: useChatEventInspector.ts - 删除 ChatEventInspectorStreamDeps.subscribe 字段**

在 `ChatEventInspectorStreamDeps` 类型中，删除 `subscribe` 字段（约第 42-47 行）：

```typescript
  subscribe?: (
    sessionId: string,
    ownerKind: 'agent' | 'team',
    types: string[],
    handler: (env: InspectorEvent) => void,
  ) => () => void;
```

保留 `ownerKind` 和 `onReconnect` 字段。

- [ ] **Step 9: useChatEventInspector.ts - 删除 connectStream 中的 subscribe 调用**

在 `connectStream` 函数中，删除第 117-122 行：

```typescript
    if (streamDeps?.subscribe) {
      unsubLive = streamDeps.subscribe(id, ownerKind, LIVE_TYPES, (env) => {
        if (paused.value) return;
        upsertEvent(env);
      });
    }
```

保留 `unsubReconnect` 赋值。

- [ ] **Step 10: useChatEventInspector.ts - 删除 unsubLive 变量和清理**

删除第 87 行：

```typescript
  let unsubLive: (() => void) | null = null;
```

在 `disconnectStream` 函数中，删除第 132-133 行：

```typescript
    unsubLive?.();
    unsubLive = null;
```

- [ ] **Step 11: useChatTraceAndArtifacts.ts - 删除 traceStreamDeps.subscribe**

打开 `web/src/features/chat/composables/useChatTraceAndArtifacts.ts`，在 `traceStreamDeps` computed 中，删除第 65 行：

```typescript
    subscribe: streamManager.subscribeSessionStream,
```

- [ ] **Step 12: useChatSender.ts - 更新 WsUpstream import**

打开 `web/src/features/chat/composables/useChatSender.ts`，将第 13 行：

```typescript
import type { WsUpstream } from '../../../realtime/envelope';
```

替换为：

```typescript
import type { WsUpstream } from '../../../realtime/ws-transport';
```

- [ ] **Step 13: 验证 4 个 chat composables 编译通过**

Run: `cd web && pnpm exec tsc --noEmit -p tsconfig.json 2>&1 | grep -E "useChatStreamManager|useChatEventInspector|useChatTraceAndArtifacts|useChatSender" | head -20`
Expected: 无输出

---

## Task 9: 清理 teams chain（5 个文件）

**Files:**
- Modify: `web/src/features/teams/api.ts`
- Modify: `web/src/stores/teams/index.ts`
- Modify: `web/src/features/teams/useTeamsPage.ts`
- Modify: `web/src/pages/TeamsPage.vue`
- Modify: `web/src/components/teams/TeamRunsDialog.vue`

- [ ] **Step 1: api.ts - 删除 subscribeTeamRunEventsWs 的 onError/onReplayState 参数**

打开 `web/src/features/teams/api.ts`，找到 `subscribeTeamRunEventsWs` 函数（约第 262-294 行），替换为：

```typescript
export function subscribeTeamRunEventsWs(
  sessionId: string,
  teamID: string,
  onEvent: (event: TeamRunEvent) => void,
) {
  const effectiveSession =
    sessionId.trim() === '' || sessionId === TEAM_MONITOR_SESSION_ALIAS ? GLOBAL_WS_SESSION_ID : sessionId;
  const stream = createEnvelopeStream({
    sessionId: effectiveSession,
    channels: ['team', 'monitor', 'system'],
    autoConnect: false,
    onActivityEvent: (ev) => {
      const mapped = teamRunEventFromActivityEvent(ev, teamID);
      if (mapped) {
        onEvent(mapped);
      }
    },
  });

  stream.connect();

  return {
    close: () => stream.disconnect(),
    connected: stream.connected,
  };
}
```

（删除 `onError?`、`onReplayState?` 参数；删除 `onReplayState: (replaying) => onReplayState?.(replaying)` 透传；删除 `stream.onType('error', ...)` 处理器）

- [ ] **Step 2: api.ts - 检查是否有其他 Envelope import 需更新**

Run: `cd web && grep -n "from.*envelope" web/src/features/teams/api.ts`
Expected: 无输出（已确认无 EnvelopeType/WsUpstream import）

注：如本步骤发现遗漏的 import，需一并迁移到 `realtime/ws-transport`。

- [ ] **Step 3: stores/teams/index.ts - 删除 subscribeRunEvents 的 onError/onReplayState 参数**

打开 `web/src/stores/teams/index.ts`，将 `subscribeRunEvents` 函数（约第 178-186 行）替换为：

```typescript
  function subscribeRunEvents(
    sessionId: string,
    teamID: string,
    onEvent: (event: TeamRunEvent) => void,
  ) {
    return subscribeTeamRunEventsWs(sessionId, teamID, onEvent);
  }
```

- [ ] **Step 4: useTeamsPage.ts - 删除 runEventsReplaying ref**

打开 `web/src/features/teams/useTeamsPage.ts`，删除第 81 行：

```typescript
  const runEventsReplaying = ref(false);
```

保留第 80 行 `const runEventsConnected = ref(false);`。

- [ ] **Step 5: useTeamsPage.ts - 简化 openRunEvents 调用**

将 `openRunEvents` 函数（约第 295-311 行）替换为：

```typescript
  function openRunEvents(teamID: string) {
    closeRunEvents();
    runEventsSource = store.subscribeRunEvents(
      GLOBAL_WS_SESSION_ID,
      teamID,
      (event) => {
        runEventsConnected.value = true;
        applyRunEvent(event);
      },
    );
  }
```

（删除 `() => { runEventsConnected.value = false; }` onError 回调和 `(replaying) => { runEventsReplaying.value = replaying; }` onReplayState 回调）

- [ ] **Step 6: useTeamsPage.ts - 简化 closeRunEvents**

将 `closeRunEvents` 函数（约第 313-318 行）替换为：

```typescript
  function closeRunEvents() {
    runEventsSource?.close();
    runEventsSource = null;
    runEventsConnected.value = false;
  }
```

（删除 `runEventsReplaying.value = false;`）

- [ ] **Step 7: useTeamsPage.ts - 从 return 删除 runEventsReplaying**

在 return 语句中，删除第 410 行：

```typescript
    runEventsReplaying,
```

- [ ] **Step 8: TeamsPage.vue - 删除 :live-replaying prop 传递**

打开 `web/src/pages/TeamsPage.vue`，删除第 149 行：

```vue
      :live-replaying="runEventsReplaying"
```

- [ ] **Step 9: TeamsPage.vue - 从 useTeamsPage 解构删除 runEventsReplaying**

在 `<script setup>` 中找到 `useTeamsPage()` 解构，删除 `runEventsReplaying`。该解构位置需读取确认，但根据 useTeamsPage 的 return 结构，应在 TeamsPage.vue 的 script 中有类似：

```typescript
const { runEventsConnected, runEventsReplaying, ... } = useTeamsPage(...);
```

删除 `runEventsReplaying`。

- [ ] **Step 10: TeamRunsDialog.vue - 将 liveReplaying prop 改为可选**

打开 `web/src/components/teams/TeamRunsDialog.vue`，找到第 124 行：

```typescript
  liveReplaying: boolean;
```

替换为：

```typescript
  liveReplaying?: boolean;
```

保留 `<q-banner v-if="liveReplaying">` 模板——`liveReplaying` 现在是 `boolean | undefined`，`v-if` 会将其视为 false，banner 永不显示（防御性保留）。

- [ ] **Step 11: 验证 teams chain 编译通过**

Run: `cd web && pnpm exec tsc --noEmit -p tsconfig.json 2>&1 | grep -E "teams/api|stores/teams|useTeamsPage|TeamsPage|TeamRunsDialog" | head -20`
Expected: 无输出

---

## Task 10: 清理 3 个 no-op 消费者

**Files:**
- Modify: `web/src/composables/useGlobalInboundNotifications.ts:128`
- Modify: `web/src/features/chat/useChatBackgroundJobs.ts:122`
- Modify: `web/src/features/chat/composables/useChatInboundSync.ts:421`

- [ ] **Step 1: useGlobalInboundNotifications.ts - 删除 onEnvelope 字段**

打开 `web/src/composables/useGlobalInboundNotifications.ts`，删除第 127-128 行：

```typescript
      // AF: Legacy Envelope path removed — no-op.
      onEnvelope: () => {},
```

- [ ] **Step 2: useChatBackgroundJobs.ts - 删除 onEnvelope 字段**

打开 `web/src/features/chat/useChatBackgroundJobs.ts`，删除第 120-122 行：

```typescript
      // Backend no longer sends envelopes; onEnvelope is required by the type
      // but never fires. ActivityEvent is the live path.
      onEnvelope: () => {},
```

- [ ] **Step 3: useChatInboundSync.ts - 删除 onEnvelope 字段**

打开 `web/src/features/chat/composables/useChatInboundSync.ts`，删除第 418-421 行：

```typescript
      // AF: Legacy Envelope path removed. onEnvelope is required by
      // GlobalWsConsumer but is now a no-op — all chat traffic arrives as
      // ActivityEvent via onActivityEvent.
      onEnvelope: () => {},
```

- [ ] **Step 4: 验证 3 个 no-op 消费者编译通过**

Run: `cd web && pnpm exec tsc --noEmit -p tsconfig.json 2>&1 | grep -E "useGlobalInboundNotifications|useChatBackgroundJobs|useChatInboundSync" | head -20`
Expected: 无输出

---

## Task 11: 清理 streamEventTypes.ts JSDoc 注释

**Files:**
- Modify: `web/src/features/chat/streamEventTypes.ts:113-120` (ErrorEvent.errorCode JSDoc)

- [ ] **Step 1: 删除 EnvelopeError.code 提及**

打开 `web/src/features/chat/streamEventTypes.ts`，将 `ErrorEvent.errorCode` 字段的 JSDoc（约第 113-121 行）：

```typescript
  /**
   * Stable machine-readable error code used to drive inline action hints
   * (retry / switch_model / rephrase / …). May originate from:
   *   - `Activity.toolErrorCode` (turn-level codes like `LLM_CALL_FAILED`)
   *   - `Activity.meta.error_code` (backend `apierror.Code` like `NOT_FOUND`)
   *   - `EnvelopeError.code` (raw WS error envelopes)
   *
   * See `features/chat/errorCodeHints.ts` for the full code → action map.
   */
```

替换为：

```typescript
  /**
   * Stable machine-readable error code used to drive inline action hints
   * (retry / switch_model / rephrase / …). May originate from:
   *   - `Activity.toolErrorCode` (turn-level codes like `LLM_CALL_FAILED`)
   *   - `Activity.meta.error_code` (backend `apierror.Code` like `NOT_FOUND`)
   *
   * See `features/chat/errorCodeHints.ts` for the full code → action map.
   */
```

（删除 `EnvelopeError.code` 一行——EnvelopeError 类型已不存在）

---

## Task 12: 全量验证 + 单次提交

**Files:** 无文件改动，纯验证 + commit

- [ ] **Step 1: 全量 TypeScript 编译检查**

Run: `cd web && pnpm exec tsc --noEmit -p tsconfig.json 2>&1 | head -40`
Expected: 无输出（无类型错误）

如有错误：根据错误信息定位遗漏的文件，补充修改后重跑。

- [ ] **Step 2: ESLint + Prettier 检查**

Run: `cd web && pnpm lint`
Expected: PASS（无 lint 错误）

如有错误：根据提示修复（通常是 unused import 或 trailing whitespace）。

- [ ] **Step 3: Vitest 单元测试**

Run: `cd web && pnpm test`
Expected: 全部 PASS

如有失败：确认是否为 Envelope 死代码删除引起的预期失败（例如测试直接 mock 了已删除的 API）；若测试本身测试死代码路径，则删除该测试。

- [ ] **Step 4: Vite 生产构建**

Run: `cd web && pnpm build`
Expected: 构建成功，无 warning 关于 Envelope

- [ ] **Step 5: 全局搜索验证无残留**

```bash
cd web && grep -rn "EnvelopeDispatcher\|EnvelopeType\|onEnvelope\|onReplayState\|RevisionTracker\|requestSyncReplay\|subscribeToDataChannel\|subscribeToDataTypes\|resolveEnvelopeTurnId\|resolveEnvelopeSource\|resolveEnvelopeRevision" src/ --include="*.ts" --include="*.vue"
```

Expected: 无输出（或仅 `monitorEvent.ts` 注释中的提及，spec §7 允许）

```bash
cd web && grep -rn "from ['\"].*realtime/envelope['\"]" src/ --include="*.ts" --include="*.vue"
```

Expected: 无输出

```bash
cd web && grep -rn "from ['\"]\\./envelope['\"]" src/ --include="*.ts" --include="*.vue"
```

Expected: 无输出

- [ ] **Step 6: 确认活路径保留**

```bash
cd web && grep -rn "createEnvelopeStream\|useEnvelopeStream" src/ --include="*.ts" --include="*.vue" | wc -l
```

Expected: 输出 ≥ 10（chat/teams/monitor/graph/orchestration/knowledge 等多个 feature 仍在使用工厂函数）

```bash
cd web && grep -rn "onActivityEvent\|onMonitorEvent" src/ --include="*.ts" --include="*.vue" | wc -l
```

Expected: 输出 ≥ 8（Activity-First 透传链完整）

- [ ] **Step 7: 单次大提交**

```bash
cd f:/aranea-agents
git add web/src/realtime/envelope.ts web/src/realtime/dispatcher.ts web/src/realtime/data_channel.ts web/src/realtime/event_replay.ts
git add web/src/realtime/ws-transport.ts web/src/realtime/globalWsHub.ts web/src/realtime/useEnvelopeStream.ts
git add web/src/features/chat/useEnvelopeStream.ts web/src/features/chat/composables/useChatStreamManager.ts web/src/features/chat/composables/useChatEventInspector.ts web/src/features/chat/composables/useChatTraceAndArtifacts.ts web/src/features/chat/composables/useChatSender.ts
git add web/src/features/teams/api.ts web/src/stores/teams/index.ts web/src/features/teams/useTeamsPage.ts web/src/pages/TeamsPage.vue web/src/components/teams/TeamRunsDialog.vue
git add web/src/stores/spirit/index.ts web/src/features/chat/streamEventTypes.ts
git add web/src/composables/useGlobalInboundNotifications.ts web/src/features/chat/useChatBackgroundJobs.ts web/src/features/chat/composables/useChatInboundSync.ts
git status
git commit -m "$(cat <<'EOF'
refactor(web): remove frontend Envelope system (Phase 5 Blocker G frontend)

Delete the entire Envelope type system, EnvelopeDispatcher, data_channel,
and WS replay mechanism from the frontend. The backend Phase 5 Blocker
A-G is complete — the server no longer sends envelopes; chat/system
events arrive as ActivityEvent, monitor events as MonitorEvent.

Changes:
- Delete 4 files (envelope.ts, dispatcher.ts, data_channel.ts,
  event_replay.ts) — all dead code (EnvelopeDispatcher.dispatch never
  fires, RevisionTracker tracks removed WS replay path).
- Inline WsDownstream/WsUpstream into ws-transport.ts (single source
  of truth for WS message shape, no envelope? field).
- Inline 5 Spirit payload types into stores/spirit/index.ts (the only
  consumer; type views over ActivityEvent.activity.meta).
- Remove onEnvelope/onReplayState callback chains from ws-transport,
  globalWsHub, useEnvelopeStream, chat composables, teams chain.
- Delete 3 dead functions (useChatStream/useTeamStream/useMonitorStream)
  in features/chat/useEnvelopeStream.ts (0 callers, 10 stream.onType
  dead calls).
- Delete subscribeSessionStream + LIVE_TYPES subscribe chain from
  chat event inspector (already dead — EnvelopeDispatcher never fires).
- Delete stream.onType('error') in teams api.ts (already dead —
  backend doesn't send envelope errors).
- Make TeamRunsDialog.vue liveReplaying prop optional (TeamsPage
  no longer passes it; banner kept as defensive dead code).
- Clean 3 no-op onEnvelope: () => {} consumers.
- Clean streamEventTypes.ts JSDoc (remove EnvelopeError.code mention).

Preserved (Activity-First live paths):
- createEnvelopeStream/useEnvelopeStream factory functions
- onActivityEvent/onMonitorEvent transparent pass-through
- wsReplaying ref as dead state (always false) — UI consumers
  (ChatPage/ChatMessagePanel/useChatWorkspace/useChatInboundSync)
  still reference it; deletion would expand blast radius unnecessarily.
- inboundSyncRouting.ts (already Activity-First, no Envelope dep).

Verification: pnpm lint && pnpm test && pnpm build all pass.

Refs: ADR-03 Phase 5 Blocker G (frontend portion),
analysis-chat-module-refactor.md §11 Phase 3 Task 8.
EOF
)"
```

- [ ] **Step 8: 验证 commit 成功**

Run: `cd f:/aranea-agents && git log -1 --stat | head -30`
Expected: 最新 commit 显示约 22 个文件变更（4 deletions + 18 modifications）

Run: `cd f:/aranea-agents && git status`
Expected: 工作区干净

---

## Self-Review Checklist

- [x] **Spec coverage**: Spec §3.1 (4 文件删除) → Task 3；§3.2 (14 文件修改) → Task 1-11；§3.3 (3 no-op 消费者) → Task 10；§7 完成标准 8 项 → Task 12 Step 1-8。
- [x] **Placeholder scan**: 所有 step 含具体代码或具体命令，无 "TODO/TBD"。
- [x] **Type consistency**: `WsDownstream`/`WsUpstream` 内联定义在 Task 2，所有后续 Task 引用 `realtime/ws-transport`；`SpiritPlanCreatedPayload` 等 5 类型在 Task 1 内联，后续无再次定义。
- [x] **Bite-sized tasks**: 每个 step 是 2-5 分钟的单一 action（删一段代码 / 改一行 import / 跑一次验证）。
- [x] **DRY**: 不重复定义同一类型；wsReplaying ref 作为死状态保留的决策避免在 4 个 UI 消费者中重复修改。
- [x] **YAGNI**: 不新建文件；不重命名 useEnvelopeStream.ts（用户选择方案 B 而非方案 C）；不新增测试（纯死代码删除）。
- [x] **TDD 豁免**: 本计划是纯死代码删除 + 类型迁移，无行为变更，spec §5.3 明确不新增测试。现有 Vitest 测试覆盖活路径。
- [x] **Frequent commits**: 用户选择方案 1（单次大提交），故所有变更在 Task 12 一次性 commit。如执行中选择分段提交，可在每个 Task 末尾追加 commit step。

---

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-06-26-frontend-envelope-removal.md`. Two execution options:**

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
