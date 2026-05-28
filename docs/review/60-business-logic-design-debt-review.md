# 60 业务逻辑底层设计债 Review

> **评分**：72 / 100 | **风险等级**：P1
> **审查时间**：2026-05-28
> **触发**：前端 `turn_index` 设计问题已迁移修复（`turn_index → turn_id/turn_number/seq_in_turn`），审查全项目是否存在同类设计债
> **代码锚点**：`web/src/features/chat/` · `internal/biz/` · `internal/agent/options.go` · `internal/data/session_repo.go`

---

## 核心发现

`turn_index` 的问题本质：**用脆弱的隐式约定代替显式的、类型安全的、可查询的数据模型**。以下 7 个问题属于同一模式。

---

## 🔴 D-01：`options_json` — 用 JSON 字符串当万能口袋

**严重度**：🔴 P1 | **影响范围**：前后端全链路

### 现状

`Message.options_json`（`string` 字段）承载了 7+ 种完全不同语义的数据：

| 写入者 | 存的内容 | 读取者 |
|--------|---------|--------|
| 后端 `UserOptionsJSON` | `{dialog_mode, provider, model, agent, send_meta, team_member}` | 前端 UserBubble |
| 后端 `AssistantOptionsJSON` | `{agent, team_member}` | 前端 AssistantBubble |
| 后端 `MergeReasoningIntoAssistantOptionsJSON` | `{reasoning_content, reasoning_markdown}` | 前端 reasoning 折叠区 |
| 后端 `MergeSourceIntoUserOptionsJSON` | `{source, platform, channel_key}` | 前端 UserBubble 徽章 |
| 后端 activity persist | `{schema: "chat.activity/v1", tool_event: {...}}` | 前端 ToolCard |
| 前端 `streamContentPatch.ts` | `{reasoning_markdown}` | 前端 reasoning 折叠区 |
| 前端 `useChatWorkspace.ts` | `{attachments: [...]}` | 前端附件芯片 |
| 后端 feedback | `{feedback: {rating, comment}}` | 前端反馈 UI |

### 问题

1. **前端到处 `JSON.parse(options_json)`** — `mergeSessionMessages.ts:131`、`envelopeToolCall.ts:172`、`streamContentPatch.ts:18`、`useChatWorkspace.ts:424`
2. **无类型安全** — 任何方都可以往里面塞任意字段，无 schema 约束
3. **字段冲突** — `reasoning_markdown` vs `reasoning_content` 双写就是冲突产物
4. **查询不可能** — 想按 `source=channel` 过滤消息？只能全表扫 + JSON parse
5. **后端也一样** — `options.go` 里全是 `json.Marshal(map[string]any{})` 的无类型写入

### 改进方案

```
Phase 1（前端类型化）：
  - Message 类型拆分 options_json → typed 字段：
    source?: string, platform?: string, channel_key?: string,
    dialog_mode?: string, provider?: string, model?: string,
    agent_ref?: { id, key, name, icon },
    team_member?: { agent_id, name, role },
    tool_event?: ToolUseEvent,
    reasoning_markdown?: string,
    attachments?: Array<{ id }>,
    feedback?: { rating, comment }
  - wireInboundChatMessage 解析 options_json → typed 字段
  - 所有消费方改用 typed 字段，不再 JSON.parse

Phase 2（后端拆列）：
  - messages 表新增强类型列：source, platform, channel_key, dialog_mode, provider, model
  - 新增 message_agent_refs 关联表
  - tool_event 独立为 tool_invocations 行（已有）
  - reasoning 独立列
  - options_json 降级为仅存前端扩展字段，加 JSON Schema 校验
```

### 涉及文件

- `web/src/domain/types.ts` — Message 类型定义
- `web/src/features/chat/api.ts` — wireInboundChatMessage
- `web/src/features/chat/mergeSessionMessages.ts` — isActivityMessage
- `web/src/features/chat/envelopeToolCall.ts` — toolEventFromMessage
- `web/src/features/chat/streamContentPatch.ts` — parseMessageExtras
- `web/src/features/chat/composables/useChatWorkspace.ts` — 附件删除
- `internal/agent/options.go` — UserOptionsJSON / AssistantOptionsJSON / Merge*
- `internal/data/session_repo.go` — entMessageToBiz

---

## 🔴 D-02：消息 ID 前缀约定 — 隐式协议代替显式类型

**严重度**：🔴 P1 | **影响范围**：前端消息合并/分组全链路

### 现状

前端消息 `id` 字段用前缀区分来源和生命周期：

| 前缀 | 含义 | 判断代码 |
|------|------|---------|
| `pending-user-` | 本地乐观更新用户消息 | `id.startsWith("pending-user-")` |
| `ws-stream-` | WS 流式助手消息 | `id.startsWith("ws-stream-")` |
| `ws-team-stream-` | Team 流式消息 | `id.startsWith("ws-team-stream-")` |
| `member-` | Team 成员流式消息 | `id.startsWith("member-")` |
| `act-` | 工具活动卡片 | `activityMessageId()` |
| `tool-` | 工具消息 fallback | `activityMessageId()` |
| `turn-orphan-` | 无 user 的 turn block | `groupMessagesByTurn.ts:76` |

### 问题

1. **`isInFlightLocalRow`、`isEphemeralMessage`、`isTeamMemberStreamMessage`** 全靠 `id.startsWith()` 判断
2. **如果后端 UUID 恰好以这些前缀开头**，逻辑就会误判
3. **合并逻辑脆弱** — `isPendingUserMatch` 用 `session_id + content_markdown` 匹配，连续发两条相同内容会错配

### 改进方案

```typescript
type MessageOrigin =
  | { kind: "persisted" }
  | { kind: "pending_user"; localId: string }
  | { kind: "streaming"; sessionId: string }
  | { kind: "team_member"; agentKey: string }
  | { kind: "tool_activity"; toolEventId: string };

type Message = {
  id: string;          // 永远是服务端 ID 或 UUID
  origin: MessageOrigin; // 替代前缀约定
  // ...
};
```

### 涉及文件

- `web/src/domain/types.ts` — Message 类型
- `web/src/features/chat/mergeSessionMessages.ts` — isInFlightLocalRow / isEphemeralMessage
- `web/src/features/chat/groupMessagesByTurn.ts` — isTeamMemberStreamMessage
- `web/src/features/chat/envelopeToolCall.ts` — activityMessageId
- `web/src/features/chat/streamHandlers.ts` — createPlaceholderMessage / streamRowId

---

## 🟠 D-03：`AgentRuntimeSettings` — 80+ 字段上帝结构体 + config_json 双写

**严重度**：🟠 P2 | **影响范围**：后端 Agent CRUD + 运行时

### 现状

`AgentRuntimeSettings` 有 80+ 平铺字段，跨越 7 个领域（Identity、Reasoning、Memory、Tools、Skills、Evolution、Context）。

`syncConfigJSON` 每次保存都将 settings + files 序列化回 `agents.config_json`，形成**双重写入**：数据既在 `agent_runtime_settings` 表里，又在 `agents.config_json` 里。

### 问题

1. 写入方直接操作平铺字段，无领域边界
2. Ent schema 是一张大平表，字段增删都要改 schema + migration
3. config_json 双写 → 数据不一致风险

### 改进方案

- 去掉 config_json 双写，agent_runtime_settings 就是唯一真相源
- 如果保留 config_json 做兼容，让它变成只读的 materialized view

### 涉及文件

- `internal/biz/agent_types.go` — AgentRuntimeSettings
- `internal/biz/agent_usecase.go` — syncConfigJSON
- `internal/data/ent/schema/` — agent + agent_runtime_settings schema

---

## 🟠 D-04：Envelope `metadata` 滥用 — 又一个无类型 JSON 口袋

**严重度**：🟠 P2 | **影响范围**：前后端实时通信

### 现状

`Envelope.metadata: Record<string, unknown>` 被后端塞入 `turn_id`、`run_id`、`session_revision`、`source`、`status` 等数据，但 Envelope 已有 `turn_id`、`session_revision`、`source` 一级字段。

前端 `conversationEventDispatcher.ts:38-42` 有 5 层 fallback：

```typescript
const turnId =
  (env.turn_id ?? "").trim() ||
  stringValue(metadataValue(env, "turn_id")) ||
  stringValue(metadataValue(env, "run_id")) ||
  (env.request_id ?? "").trim() ||
  env.id;
```

### 问题

1. 后端有些路径只往 metadata 里塞，前端不得不两个地方都查
2. 5 层 fallback 链是 metadata 滥用的直接后果
3. 类型不安全，metadata 里的值可能是任意类型

### 改进方案

- 后端统一在 Envelope 一级字段写入 turn_id、session_revision、source
- metadata 只放真正的扩展数据（intent_kind、target_agent）
- 前端去掉 metadata fallback 链

### 涉及文件

- `web/src/realtime/envelope.ts` — Envelope 类型
- `web/src/features/chat/conversationEventDispatcher.ts` — turnId 5 层 fallback
- `web/src/features/chat/inboundSyncEnvelope.ts` — metadata 读取
- 后端 Envelope 构建代码

---

## 🟠 D-05：`groupMessagesByTurn` 忽略 turn_id — 有 FK 不用靠 role 猜

**严重度**：🟠 P2 | **影响范围**：前端 TurnBlock 分组

### 现状

注释说"turn_id is the authoritative FK"，但分组决策完全靠 `msg.role === "user"` 开新 block，turn_id 只是挂上去的标签。

### 问题

1. await_user 场景一个 turn 里两个 user 消息 → 前端错误拆成两个 block
2. Team 场景 assistant 的 turn_id 和 user 不同 → 不会归到同一个 turn

### 改进方案

用 turn_id 做 Map 分组 key，role 只决定 block 内的分布位置。

### 涉及文件

- `web/src/features/chat/groupMessagesByTurn.ts`

---

## 🟡 D-06：`status` 裸字符串 — 缺少类型安全枚举

**严重度**：🟡 P3 | **影响范围**：前端消息状态判断

### 现状

`message.status` 是 `string`，`tool_running`、`tool_blocked`、`streaming`、`ok`、`failed`、`tool_failed`、`tool_cancelled` 散落各处。

### 改进方案

前端定义 `MessageStatus` 联合类型，所有消费方用类型安全的枚举值。

### 涉及文件

- `web/src/domain/types.ts`
- `web/src/features/chat/mergeSessionMessages.ts`
- `web/src/features/chat/envelopeToolCall.ts`

---

## 🟡 D-07：`NativeTurnGateway` vs `TurnGateway` vs `TurnExecutor` — 接口碎片化

**严重度**：🟡 P3 | **影响范围**：后端 biz 层

### 现状

对"执行一个 turn"有 3 个重叠接口：
1. `TurnGateway`（5 方法）+ `TurnControlGateway`（扩展 4 方法）
2. `NativeTurnGateway`（12 方法，包含上面两个的所有方法 + 更多）
3. `TurnExecutor`（3 方法，最精简）

`NativeTurnGateway` 有 12 个方法，违反接口隔离原则。

### 改进方案

- NativeTurnGateway 拆成 TurnGateway + TurnControlGateway + PendingMessageGateway
- 逐步迁移消费方到拆分后的接口

### 涉及文件

- `internal/biz/turn_gateway.go`
- `internal/biz/turn_input.go`
- `internal/biz/turn_executor.go`

---

## 修复优先级

| 序号 | ID | 修复内容 | 优先级 | 预计影响文件数 | 状态 |
|------|-----|---------|--------|--------------|------|
| 1 | D-01 | options_json → 前端 Message 类型化 | P1 | ~15 | ✅ 已修复 |
| 2 | D-02 | 消息 ID 前缀 → MessageOrigin 类型 | P1 | ~8 | ✅ 已修复 |
| 3 | D-05 | groupMessagesByTurn 用 turn_id 分组 | P2 | 1 | ✅ 已修复 |
| 4 | D-04 | Envelope metadata → 统一一級字段 | P2 | ~6 | ✅ 已修复（前端 resolve 函数） |
| 5 | D-06 | MessageStatus 联合类型 | P3 | ~5 | ✅ 已修复 |
| 6 | D-03 | AgentRuntimeSettings 去 config_json 双写 | P2 | ~5 | ✅ 已修复 |
| 7 | D-07 | NativeTurnGateway 拆分 | P3 | ~6 | ✅ 已修复 |

### 已修复项的变更摘要

**D-01**：新增 `parseMessageOptions.ts`，在 `wireInboundChatMessage` 中一次性解析 `options_json` → typed 字段（`agent_ref`、`team_member`、`source_meta`、`reasoning_markdown`、`dialog_mode`、`provider`、`model`、`attachments`、`tool_event`）。所有消费方优先使用 typed 字段，`JSON.parse(options_json)` 降级为 fallback。

**D-02**：新增 `messageOrigin.ts` + `MessageOrigin` 类型，替代 `id.startsWith()` 前缀约定。`mergeSessionMessages.ts`、`groupMessagesByTurn.ts`、`streamHandlers.ts`、`toolEventMarkdown.ts` 均已迁移。

**D-05**：`groupMessagesByTurn.ts` 重写为 turn_id-based 分组，`turn_id` 作为 Map key，`role` 仅决定 block 内分布位置。新增 `consolidateOrphanToolBlocks` 合并孤立工具块。

**D-04**：新增 `resolveEnvelopeTurnId`、`resolveEnvelopeSource`、`resolveEnvelopeRevision` 统一 resolve 函数，替代 `conversationEventDispatcher.ts` 中的 5 层 fallback 链和 `inboundSyncEnvelope.ts` 中的重复逻辑。

**D-06**：新增 `MessageStatus` 联合类型 + `MESSAGE_STATUS` 常量 + `isInFlightStatus`/`isToolStatus` 工具函数。`mergeSessionMessages.ts`、`envelopeToolCall.ts`、`toolEventMarkdown.ts`、`useChatMessageRow.ts` 均已迁移。

**D-03**：去掉 `syncConfigJSON` 双写（5 处调用全部移除），改为在 `hydrate` 中懒计算 `config_json`。`computeConfigJSON` 保留为纯计算函数（不写回 DB）。缓存指纹改用 `SettingsJSON`（有 settings 时）优先于 `ConfigJSON`。AfterTurn hook 新增 `AgentSettings` 字段，评估配置优先从 settings 读取。`mergeEvaluationFromLegacy` 保留 `evaluation` 字段从 legacy config_json 合并。

**D-07**：`NativeTurnGateway` 从 12 方法平铺接口重构为组合接口：`TurnGateway` + `TurnControlGateway` + `PendingQueueGateway`。新增 `PendingQueueGateway`（2 方法：`TryEnqueueUserMessage`、`SetSessionPendingMergeFollowup`）。`TurnGateway` 扩展加入 `RunNativeTurn` 和 `RunNativeTurnWithOutcome`。`NativeTurnGateway` 标记为 Deprecated，保持向后兼容。

---

*本文档由 AI 代码 Review 生成，2026-05-28。最后更新：2026-05-28。*
