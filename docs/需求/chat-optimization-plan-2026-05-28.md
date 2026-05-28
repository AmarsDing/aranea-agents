# Chat 模块全面优化实施计划

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

## 剩余项（未修复，需后续迭代）

| # | 优先级 | ID | 问题 | 类型 | 原因 |
|---|--------|-----|------|------|------|
| 1 | P2 | P2-4 | features/chat/components/ 下存在 2 个 .vue 文件 | 红线 #5 | 需要迁移组件路径，影响面较大 |
| 2 | P2 | P2-5 | messageStore 函数级硬依赖 sessionStore | 架构 | 需重构 Store 接口，影响面大 |
| 3 | P2 | P2-6 | runtimeStore 函数级硬依赖 sessionStore | 架构 | 同上 |
| 4 | P2 | P2-7 | useChatWorkspace 返回 80+ 属性，760+ 行 | 架构 | 需逐步拆分，影响面大 |
| 5 | P2 | P2-8 | useChatWorkspace/useChatProviderOptions 直接调 API | 红线 #11 | 需迁入 Store action |
| 6 | P2 | P2-9 | ChatMessagePanel 硬编码 hex `#4caf50` | UX | 需确认 `--color-success` 变量定义 |
| 7 | P2 | P2-10 | useChatStore facade 已废弃但仍保留 162 行 | 死代码 | 需迁移测试用例后删除 |
| 8 | P3 | P3-1 | ChatComposer 展示组件内 $q.notify | 规范 | 输入验证通知，可标注为例外 |
| 9 | P3 | P3-2 | ChatSessionSidebar import sessionSync | 规范 | 事件总线模块，边界模糊 |
| 10 | P3 | P3-3 | ChatMessagePanel 容器组件放在 components/ | 规范 | 需团队确认分类 |

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
| P2（红线/清理） | 3 | 7 | 30% |
| P3（规范） | 0 | 3 | 0% |
| **总计** | **12** | **10** | **55%** |

> **核心指标**：P0+P1 完成率 **100%**，所有阻断级红线违规已修复。剩余 10 项均为 P2/P3 预存技术债，需后续迭代处理。

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
