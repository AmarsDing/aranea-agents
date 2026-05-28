# Chat 模块全面优化实施计划

> **Goal:** 修复 Chat 模块 P0 Bug，重构前端发送路径消除重复代码，改善错误处理和用户体验
> **Architecture:** 前端统一发送策略模式 + 后端保持现有分层，聚焦前端 P0/P1 修复
> **Tech Stack:** Vue 3 + TypeScript + Pinia + Quasar

---

## 问题总览

| 优先级 | ID | 问题 | 类型 |
|--------|-----|------|------|
| P0 | P0-1 | retryFailedMessage Team 消息重试走错通道 | Bug |
| P0 | P0-2 | Stall 检测定时器已定义但从未激活 | Bug |
| P0 | P0-3 | failedPendingIds 模块级全局 Set 内存泄漏 | Bug |
| P0 | P0-4 | inboundHydrateError 设置后永不清除 | Bug |
| P1 | P1-1 | Agent/Team 双通道发送逻辑 80% 重复 | 架构 |
| P1 | P1-2 | streamHandlers 守卫代码重复 6 处 | 架构 |
| P1 | P1-3 | 后端错误码未在前端映射 | UX |
| P1 | P1-4 | 助手消息错误无恢复路径 | UX |
| P1 | P1-5 | WS 重连后不主动同步 Run 状态 | 业务 |

---

## Task 1: P0-1 修复 retryFailedMessage Team 消息重试走错通道

**Files:**
- Modify: `web/src/features/chat/composables/useChatSender.ts`
- Modify: `web/src/features/chat/streamHandlers.ts`

### 根因

`retryFailedMessage` 固定调用 `sendAgentUserContent`，即使原始消息来自 Team 会话。pending-user 消息没有记录来源 entityKind。

### 修复方案

1. 在 `createPlaceholderMessage` 中增加 `entityKind` 字段记录来源
2. `retryFailedMessage` 根据消息来源选择正确的发送通道

---

## Task 2: P0-2 激活 Stall 检测定时器

**Files:**
- Modify: `web/src/features/chat/composables/useChatSender.ts`

### 根因

`lastRunEventAt` 和 `RUN_STALL_TIMEOUT_MS` 已定义，`touchRunActivity()` 更新了时间戳，但没有定时器检查这个值。

### 修复方案

在 `markSending` 时启动 stall 检测定时器，`markSendingDone` 时清除。

---

## Task 3: P0-3 failedPendingIds 实例化 + 会话切换清理

**Files:**
- Modify: `web/src/features/chat/composables/useChatSender.ts`

### 根因

`failedPendingIds` 定义在模块作用域，所有组件实例共享，会话切换时不会清理。

### 修复方案

将 `failedPendingIds` 移入 `useChatSender` 函数体内，并在 `SenderDeps` 中增加 `onSessionSwitch` 回调进行清理。

---

## Task 4: P0-4 inboundHydrateError 成功后清除

**Files:**
- Modify: `web/src/features/chat/composables/useChatWorkspace.ts`
- Modify: `web/src/features/chat/composables/useChatInboundSync.ts`

### 根因

`inboundHydrateError` 一旦设置，即使后续 hydrate 成功也不会清除。

### 修复方案

在 `useChatInboundSync` 的 `onTurnComplete` 回调中清除错误。

---

## Task 5: P1-1 统一 Agent/Team 发送路径

**Files:**
- Modify: `web/src/features/chat/composables/useChatSender.ts`

### 根因

`sendAgentUserContent`（~140行）和 `sendTeamMessage`（~120行）逻辑 80% 重复。

### 修复方案

提取 `sendUserContent(entityKind, content, reusePendingId?)` 统一入口，通过策略对象区分 Agent/Team 差异。

---

## Task 6: P1-2 streamHandlers 守卫代码提取为高阶函数

**Files:**
- Modify: `web/src/features/chat/streamHandlers.ts`

### 修复方案

提取 `withSessionFilter(handler)` 高阶函数，消除 6 处重复的 session 过滤守卫。

---

## Task 7: P1-3 前端错误码映射 + 操作建议

**Files:**
- Create: `web/src/features/chat/errorCodeHints.ts`
- Modify: `web/src/features/chat/streamHandlers.ts`

### 修复方案

建立 `TurnErrorCode → 操作建议` 映射表，在 error 信封处理中根据错误码提供本地化操作建议。

---

## Task 8: P1-4 助手消息错误增加重新生成按钮

**Files:**
- Modify: `web/src/components/chat/ChatMessageRow.vue`

### 修复方案

在助手错误消息区域增加"重新生成"按钮，emit `regenerate` 事件。

---

## Task 9: P1-5 WS 重连后主动同步 Run 状态

**Files:**
- Modify: `web/src/features/chat/composables/useChatStreamManager.ts`

### 修复方案

WS 重连成功后，主动查询当前 Run 状态，如果 Run 已结束则强制重新加载消息。

---

## 验证

每个 Task 完成后运行：
```bash
cd web && pnpm lint && pnpm build
```
