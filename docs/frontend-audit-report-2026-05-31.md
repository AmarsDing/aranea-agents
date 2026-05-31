# Aranea-Agents 前端深度审查报告

> **审计日期**：2026-05-31
> **审计范围**：`web/src` 全量代码（42 页面、45 Store、17 业务域、180+ 组件）
> **审计依据**：项目 15 条前端红线 + `aranea-frontend-review` SKILL
> **修复状态**：P0 全部修复 ✅ | P1 全部修复 ✅ | P2 大部分修复 ✅ | P3 待后续迭代

---

## 一、总览

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| 数据流合规 | 10 | 4 | 2 | 16 |
| 组件分层 | 15 | 22 | 10 | 47 |
| 业务逻辑归属 | 8 | 5 | 0 | 13 |
| 聊天消息分组 | 0 | 0 | 0 | **0 ✅** |
| UX 主题 | 3 | 7 | 0 | 10 |
| 业务功能深度 | 10 逻辑缺陷 | 9 功能缺失 | 16 设计问题 | 35 |
| 构建与回归 | 0 | 0 | 0 | **0 ✅** |
| **合计** | **46** | **47** | **28** | **121** |

> **P2 修复后更新**：数据流违规减少 6 项（L1-01~06 迁移 + L5-01~04 Store 创建 + AR-FD2-01 + AR-FD1-01），逻辑缺陷减少 5 项（C-01/C-02/C-07/C-08/C-09）。

---

## 二、🔴 阻断项（必须修复）

### 2.1 数据流违规 — Memory 域全面绕过 Store ✅ 已修复

4 个 Memory 面板组件直接调用 `features/memory/api.ts`，完全绕过 `useMemoryStore`。

| ID | 文件 | 问题 | 红线 | 状态 |
|----|------|------|------|------|
| D4-3 | `features/memory/MemoryDeadLetterPanel.vue` | 直接 `import { listMemoryDeadLetters } from "./api"` | #7 | ✅ 已修复：改为 `useMemoryStore().loadDeadLetters()` |
| D4-4 | `features/memory/MemoryGraphExplorer.vue` | 直接 `import { getMemoryNeighborhood } from "./api"` | #7 | ✅ 已修复：改为 `useMemoryStore().loadNeighborhood()` |
| D4-5 | `features/memory/MemoryRecallTesterPanel.vue` | 直接 `import { compositeSearchMemories, debugMemoryRecall } from "./api"` | #7 | ✅ 已修复：改为 `useMemoryStore().runDebugRecall()`/`runCompositeSearch()` |
| D4-6 | `features/memory/MemoryPlatformSettingsPanel.vue` | 直接 `import { getMemoryPlatformSettings, updateMemoryPlatformSettings } from "./api"` | #7 | ✅ 已修复：改为 `useMemoryStore().loadPlatformSettings()`/`savePlatformSettings()` |

**修复方案**：在 `stores/memory/index.ts` 中新增 10 个 action（`loadDeadLetters`、`replayDeadLetter`、`abandonDeadLetter`、`loadNeighborhood`、`loadWorkerStatus`、`loadPlatformSettings`、`savePlatformSettings`、`runDebugRecall`、`runCompositeSearch`、`clearRecallResults`）及对应状态，4 个面板组件改为从 Store 读取。

### 2.2 数据流违规 — Dialog 内部直接操作 Store ✅ 已修复

| ID | 文件 | 问题 | 红线 | 状态 |
|----|------|------|------|------|
| D4-1 | `features/mcp/McpUserCredentialDialog.vue` | Dialog 内部直接调 `mcpStore.fetchUserCredentials()`/`saveUserCredential()` | #4 | ✅ 已修复：改为 emit 模式 |
| D4-2 | `features/mcp/McpServerFormDialog.vue` | Dialog 内部直接调 `mcpStore.validate()`/`test()`/`editServer()`/`addServer()` | #4 | ✅ 已修复：改为 emit 模式 |

### 2.3 展示组件错放 features/ 目录 ✅ 已修复

| ID | 文件 | 应迁至 | 状态 |
|----|------|--------|------|
| L1-01 | `features/channels/ChannelCatalogPicker.vue` | `components/channels/` | ✅ 已修复 |
| L1-02 | `features/memory/MemoryCascadePanel.vue` | `components/memory/` | ✅ 已修复 |
| L1-03 | `features/memory/MemoryFactDrawer.vue` | `components/memory/` | ✅ 已修复 |
| L1-04 | `features/memory/MemoryKnowledgePanel.vue` | `components/memory/` | ✅ 已修复 |
| L1-05 | `features/memory/MemorySessionsPanel.vue` | `components/memory/` | ✅ 已修复 |
| L1-06 | `features/memory/MemorySnapshotDrawer.vue` | `components/memory/` | ✅ 已修复 |

### 2.4 Composable 绕过 Store 直接调 API ✅ 大部分已修复

| ID | 文件 | 直接调用的 API | 状态 |
|----|------|---------------|------|
| L5-01 | `features/industries/useIndustryMarket.ts` | `listIndustries` | ✅ 已修复：改用 `useIndustryStore` |
| L5-02 | `features/industries/useIndustryDetail.ts` | `getIndustry`, `listDepartments`, `listPositions` | ✅ 已修复：改用 `useIndustryStore` |
| L5-03 | `features/industries/useIndustryWizard.ts` | 5 个 API 直接调用 | ✅ 已修复：改用 `useIndustryStore` |
| L5-04 | `features/platform/usePlatformResource.ts` | 4 个 CRUD API | ✅ 已修复：改用 `usePlatformStore` |
| L5-05 | `features/chat/composables/useChatSender.ts` | `sendMessage`（已标 TECH-DEBT） | ⏳ TECH-DEBT 标注保留 |
| L5-06 | `features/model-catalog/useModelCatalogPage.ts` | 10+ API 直接调用 | ✅ 已修复：改用 `useModelCatalogStore` |
| L5-12 | `features/memory/useMemoryCenterPage.ts` | 3 个 API（已标 TECH-DEBT） | ⏳ TECH-DEBT 标注保留 |

### 2.5 UX 主题 — 日间使用霓虹青/紫 ✅ 已修复

| ID | 文件 | 问题 | 状态 |
|----|------|------|------|
| U2-1 | `components/usage/CommandCenterStatusPanels.vue` | `linear-gradient(135deg, #00E5FF, #A78BFA)` | ✅ 已修复：改为 `var(--color-accent, #E9A23B)` 渐变 |
| U2-2 | `components/usage/CommandCenterHero.vue` | `background: #A78BFA` 裸紫色 | ✅ 已修复：改为 `var(--color-accent-indigo, #6D28D9)` |
| U2-3 | `components/usage/CommandCenterHero.vue` | `linear-gradient(135deg, #818CF8, #22D3EE)` 紫-青渐变 | ✅ 已修复：改为 `var(--color-accent-indigo)` + `var(--color-accent)` 渐变 |

### 2.6 跨 Store 直接依赖违反 sessionSync 红线 ✅ 已修复

| ID | 文件 | 问题 | 红线 | 状态 |
|----|------|------|------|------|
| X-01 | `stores/app.ts` | `useAppStore` 直接 import `useChatSessionStore` 和 `useChatMessageStore` | #11 | ✅ 已修复：改为 `emitSessionMutation({ type: "refresh" })` |

---

## 三、🟡 业务逻辑缺陷（逻辑 Bug）

| ID | 域 | 文件 | 问题描述 | 严重度 | 状态 |
|----|-----|------|----------|--------|------|
| C-01 | Chat/WS | `realtime/ws-transport.ts` | WS 重连耗尽后仅一次 toast，无持久断连提示 | logic-bug | ✅ 已修复：添加 `disconnected` ref + `onReconnectFailed` 回调 |
| C-02 | Chat/WS | `realtime/ws-transport.ts` | `ws.onclose` 不区分正常/异常关闭 | logic-bug | ✅ 已修复：close code 1000/1001 判断为正常关闭 |
| C-03 | Chat/WS | `realtime/ws-transport.ts` | `ws.onmessage` JSON 解析错误被静默吞掉 | logic-bug | ✅ 已修复：添加 `console.warn` 日志 |
| C-04 | Chat/Stream | `features/chat/streamHandlers.ts` | `runner_completion` 中异常被静默吞掉 | logic-bug | ✅ 已修复：改为 `ctx.onErrorNotify()` |
| C-07 | Chat/Concur | `stores/chat/messageStore.ts` | `loadMessages` 无并发保护 | logic-bug | ✅ 已修复：版本号模式防竞态 |
| C-08 | Chat/Concur | `features/chat/composables/useChatSender.ts` | `onSend` 存在 TOCTOU 竞态 | logic-bug | ✅ 已修复：`sendGuard` 互斥锁 |
| C-09 | Chat/Session | `features/chat/composables/useChatWorkspace.ts` | Session 切换时旧 WS stream 未主动断开 | logic-bug | ✅ 已修复：`bindSessionView` 中断开旧 stream |
| C-10 | Chat/Session | `stores/chat/messageStore.ts` | `clearSessionMessages` 只清消息不清 revision | logic-bug | ✅ 已修复：同时 `delete sessionRevisionBySession[sessionId]` |
| A-05 | Agent | `stores/app.ts` | `updateSelectedAgent` undefined 覆盖有效值 | logic-bug | ✅ 已修复：过滤 undefined 字段 |
| S-01 | Session | `stores/session/index.ts` | `removeSession` 无错误处理 | logic-bug | ✅ 已修复：添加 try/catch + error ref |
| T-02 | Team | `features/teams/api.ts` | `findActiveTeamRun` 只查最近 50 条 | logic-bug | ⏳ 待修复 |
| T-03 | Orchestration | `features/orchestration/useOrchestrationStream.ts` | `connected` ref 虚假设置 | logic-bug | ✅ 已修复：改为 `onConnected`/`onDisconnected` 回调 |
| AU-02 | Auth | `stores/auth.ts` | `ensureSession` 只执行一次 | logic-bug | ⏳ 待修复 |
| E-07 | Error | `features/chat/api.ts` | `cancelChatBackgroundJob` 死代码 | logic-bug | ✅ 已修复：移除不可达 `return false` |

---

## 四、🟡 功能缺失

| ID | 域 | 描述 | 优先级 | 状态 |
|----|-----|------|--------|------|
| AU-01 | Auth | 无 Token 刷新机制 | 🔴 高 | ⏳ 待修复 |
| K-01 | Knowledge | 服务不可用无降级提示 | 🔴 高 | ⏳ 待修复 |
| K-02 | Knowledge | 文档状态无自动更新 | 🟡 中 | ⏳ 待修复 |
| K-04 | Knowledge | 缺少 `updateCollection` API | 🟡 中 | ⏳ 待修复 |
| A-01 | Agent | 缺少 Agent 批量操作 API | 🟡 中 | ⏳ 待修复 |
| T-01 | Team | 缺少 `getTeam(id)` 单条查询 | 🟡 中 | ⏳ 待修复 |
| T-04 | Orchestration | 缺少重连失败/服务端关闭处理 | 🟡 中 | ⏳ 待修复 |
| T-05 | Team | 编译 API 未在 Store 集成 | 🟡 中 | ⏳ 待修复 |
| S-03 | Session | 导出功能未集成自动下载 | 🟡 中 | ⏳ 待修复 |

---

## 五、🟡 设计问题

| ID | 域 | 描述 |
|----|-----|------|
| C-05 | Chat | 流式快照 `reasoning` 拼接存在增量/全量歧义 |
| C-06 | Chat | 两套流式处理逻辑追加/替换策略不一致 |
| A-03 | Agent | 两套序列化逻辑产生不同结构 |
| A-04 | Agent | `agents/detail.ts` 的 `patch` 方法无错误处理 |
| G-03 | Graph | `wireGraph` 使用 `as` 类型断言 |
| K-03 | Knowledge | `search` 无防抖/缓存/loading |
| E-06 | Chat | `stopGeneration` catch 中返回 `false` |
| X-05 | CrossStore | `onSessionMutation` 返回 unsubscribe 但从未调用 |

---

## 六、✅ 合规项（亮点）

| 维度 | 结论 |
|------|------|
| **聊天消息分组 (M1-M5)** | 🟢 全部合规 |
| **构建 (R1-R3)** | 🟢 `quasar build` 成功 |
| **展示组件 Store/API 隔离 (D1)** | 🟢 `components/**/*.vue` 无 Store/API import |
| **Page 不直接调 API (D2)** | 🟢 所有 Page 通过 composable/Store 访问数据 |
| **裸 URL/散装 axios (D6)** | 🟢 所有 HTTP 调用均经 `features/*/api.ts` 封装 |
| **backdrop-filter 成对 (U3)** | 🟢 50+ 处全部成对 |
| **裸 q-table (U9)** | 🟢 全部使用 `AppRegistryTable` 包装 |
| **第二全局 CSS 入口 (U5)** | 🟢 不存在 |
| **运行时改 quasar-variables (U6)** | 🟢 无 |

---

## 七、修复进度

### ✅ 已修复（本轮）

| # | ID | 修复内容 | 修改文件 |
|---|-----|---------|---------|
| 1 | C-10 | `clearSessionMessages` 同时清除 revision | `stores/chat/messageStore.ts` |
| 2 | C-03 | WS `onmessage` JSON 解析错误添加日志 | `realtime/ws-transport.ts` |
| 3 | C-04 | `runner_completion` 异常改为 `onErrorNotify` | `features/chat/streamHandlers.ts` |
| 4 | E-07 | 移除 `cancelChatBackgroundJob` 不可达代码 | `features/chat/api.ts` |
| 5 | T-03 | `connected` ref 改为异步回调设置 | `features/orchestration/useOrchestrationStream.ts` |
| 6 | X-01 | app store 改用 `emitSessionMutation` 事件总线 | `stores/app.ts` |
| 7 | D4-3~6 | Memory 4 面板 API 调用收敛到 Store | `stores/memory/index.ts` + 4 个面板组件 |
| 8 | U2-1~3 | 日间霓虹色替换为 CSS 变量 | `CommandCenterStatusPanels.vue` + `CommandCenterHero.vue` |
| 9 | S-01 | `removeSession` 添加 try/catch 错误处理 | `stores/session/index.ts` |
| 10 | A-05 | `updateSelectedAgent` 过滤 undefined 字段 | `stores/app.ts` |
| 11 | U4-1 | ToolEditorHelpDrawer 添加 `app-dialog-card` class | `components/tools/editor/ToolEditorHelpDrawer.vue` |
| 12 | AR-FD4-01/02 | MCP Dialog 改为 emit 模式 | `McpUserCredentialDialog.vue` + `McpServerFormDialog.vue` + `useMcpServersPage.ts` + `McpServersPage.vue` |
| 13 | AR-FL1-01 | ChannelCatalogPicker 迁移至 components/ | `components/channels/ChannelCatalogPicker.vue` |
| 14 | AR-FB4-01/02 | MCP Dialog $q.notify 上收到 composable | 同 #12 |
| 15 | AR-FU1-01 | CommandCenterHero 夜间 gradient fallback | `components/usage/CommandCenterHero.vue` |
| 16 | AR-FD3-01/02 | Memory 面板 loading/saving 迁入 Store | `stores/memory/index.ts` + 2 个面板组件 |
| 17 | AR-FU4-01 | MemoryDeadLetterPanel 改用 AppRegistryMarkupTable | `features/memory/MemoryDeadLetterPanel.vue` + `memoryTableUi.ts` |
| 18 | AR-FB6-01/02/03 | 表格列定义移至 *Ui.ts | `memoryTableUi.ts` + `usageTableUi.ts` + 3 个组件 |
| 19 | AR-BE4-01 | WebhookDialog 空 catch 修复 | `components/webhooks/WebhookDialog.vue` |
| 20 | AR-FD2-01 | SystemSettingsCatalogTab 迁入 composable | `useModelCatalogPage.ts` + `SystemSettingsCatalogTab.vue` |
| 21 | AR-FD1-01 | ProviderLogo 改为 loader prop 注入 | `components/platform/ProviderLogo.vue` + `ProviderModelsTable.vue` + `ResourceManagerPage.vue` |
| 22 | AR-FB4-03 | AIRefineButton $q.notify 改为 emit | `components/agents/AIRefineButton.vue` + 4 个父组件 |
| 23 | AR-FB4-04 | JsonCodeViewer $q.notify 改为 emit | `components/common/JsonCodeViewer.vue` + composable |
| 24 | TS-fix | MemoryDeadLetterPanel + MemoryGraphExplorer 类型断言 | 2 个 .vue 文件 |
| 25 | L1-02~06 | 5 个 Memory 展示组件迁移至 components/memory/ | `MemoryCascadePanel.vue` + `MemoryFactDrawer.vue` + `MemoryKnowledgePanel.vue` + `MemorySessionsPanel.vue` + `MemorySnapshotDrawer.vue` + `MemoryCenterPage.vue` |
| 26 | L5-01~03 | 创建 `useIndustryStore` + 3 个 composable 改用 Store | `stores/industry/index.ts` + `useIndustryMarket.ts` + `useIndustryDetail.ts` + `useIndustryWizard.ts` |
| 27 | L5-04 | `usePlatformResource` 改用 `usePlatformStore` | `features/platform/usePlatformResource.ts` |
| 28 | L5-06 | 创建 `useModelCatalogStore` + composable 改用 Store | `stores/model-catalog/index.ts` + `useModelCatalogPage.ts` + `ResourceManagerPage.vue` |
| 29 | C-01/C-02 | WS 重连持久提示 + close code 区分 | `realtime/ws-transport.ts` + `realtime/useEnvelopeStream.ts` |
| 30 | C-07 | `loadMessages` 版本号模式防竞态 | `stores/chat/messageStore.ts` |
| 31 | C-08 | `onSend` `sendGuard` 互斥锁 | `features/chat/composables/useChatSender.ts` |
| 32 | C-09 | Session 切换断开旧 WS stream | `features/chat/composables/useChatWorkspace.ts` |
| 33 | AR-FD1-01 | ProviderLogo 新增 `svg` prop 优先于 `loader` | `components/platform/ProviderLogo.vue` |
| 34 | Store 导出 | `useIndustryStore` + `useModelCatalogStore` 具名导出 | `stores/index.ts` |

### ⏳ 待修复（P3 后续迭代）

| # | ID | 修复内容 |
|---|-----|---------|
| 1 | L5-05 | `useChatSender` sendMessage TECH-DEBT 迁入 Store |
| 2 | L5-12 | `useMemoryCenterPage` 3 个 API TECH-DEBT 迁入 Store |
| 3 | AU-01/AU-02 | Token 刷新 + session 健康检查 |
| 4 | K-01~K-04 | Knowledge 服务降级 + 状态更新 |
| 5 | FL2 | 4 个 features/ 下 .vue 缺少 `Container: approved` 注释（`McpServerFormDialog`、`McpUserCredentialDialog`、`ArtifactList`、`ArtifactPreview`） |

---

## 八、aranea-review SKILL 代码审查报告

> **审查日期**：2026-05-31（第二轮）
> **审查范围**：本轮 P2 修复涉及的 15 个前端文件
> **审查依据**：`aranea-review` SKILL（第七~十一节 + 第十二节构建回归）

### 8.1 审查概要

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **前端 — 数据流合规 (FD)** | 0 | 0 | 0 | 0 |
| **前端 — 组件分层 (FL)** | 0 | 1 | 0 | 1 |
| **前端 — 业务逻辑归属 (FB)** | 0 | 0 | 0 | 0 |
| **前端 — 聊天消息分组 (FM)** | 0 | 0 | 0 | 0 |
| **前端 — UX 主题 (FU)** | 0 | 0 | 0 | 0 |
| **构建与回归 (FR)** | 0 | 0 | 0 | 0 |
| **合计** | **0** | **1** | **0** | **1** |

### 8.2 🔴 阻断项（必须修复）

> 全部已修复 ✅

| ID | 维度 | 文件 | 问题描述 | 修复状态 |
|----|------|------|----------|----------|
| AR-FD4-01 | FD4 | `features/mcp/McpUserCredentialDialog.vue` | Dialog 内部直接调用 `mcpStore` 操作 | ✅ 已修复：改为 emit 模式，由 composable 处理 |
| AR-FD4-02 | FD4 | `features/mcp/McpServerFormDialog.vue` | Dialog 内部直接调用 `mcpStore` 操作 | ✅ 已修复：改为 emit 模式，由 composable 处理 |

### 8.3 🟡 建议项（推荐修复）

> 第一轮全部已修复 ✅

| ID | 维度 | 文件 | 问题描述 | 修复状态 |
|----|------|------|----------|----------|
| AR-FL1-01 | FL1 | `features/channels/ChannelCatalogPicker.vue` | 展示组件放在 `features/channels/` | ✅ 已修复：迁移至 `components/channels/` |
| AR-FL1-02 | FL1 | Memory 容器组件 | Memory 面板组件放在 `features/memory/` | ⏭️ 跳过：均为 `Container: approved` 容器组件，合规 |
| AR-FB4-01 | FB4 | `features/mcp/McpUserCredentialDialog.vue` | Dialog 内 `$q.notify` | ✅ 已修复：通知逻辑上收到 composable |
| AR-FB4-02 | FB4 | `features/mcp/McpServerFormDialog.vue` | Dialog 内多处 `$q.notify` | ✅ 已修复：通知逻辑上收到 composable |
| AR-FU1-01 | FU1 | `components/usage/CommandCenterHero.vue` | 夜间 gradient fallback `#00E5FF` | ✅ 已修复：改为 `#4DD8E8` |
| AR-FD3-01 | FD3 | `features/memory/MemoryRecallTesterPanel.vue` | `loadingComposite` 本地 ref | ✅ 已修复：迁入 Store `compositeLoading` |
| AR-FD3-02 | FD3 | `features/memory/MemoryPlatformSettingsPanel.vue` | `saving` 本地 ref | ✅ 已修复：迁入 Store `platformSettingsSaving` |

> 第二轮新增建议项

| ID | 维度 | 文件 | 问题描述 | 修复状态 |
|----|------|------|----------|----------|
| AR2-FL2-01 | FL2 | `features/mcp/McpServerFormDialog.vue` | 缺少 `Container: approved` 注释 | ⏳ 待修复 |
| AR2-FL2-02 | FL2 | `features/mcp/McpUserCredentialDialog.vue` | 缺少 `Container: approved` 注释 | ⏳ 待修复 |
| AR2-FL2-03 | FL2 | `features/artifact/ArtifactList.vue` | 缺少 `Container: approved` 注释 | ⏳ 待修复 |
| AR2-FL2-04 | FL2 | `features/artifact/ArtifactPreview.vue` | 缺少 `Container: approved` 注释 | ⏳ 待修复 |

### 8.4 🟢 提示项（记录备忘）

| ID | 维度 | 文件 | 描述 | 修复状态 |
|----|------|------|------|----------|
| AR-FU4-01 | FU4 | `features/memory/MemoryDeadLetterPanel.vue` | 使用 `q-markup-table` 而非 `AppRegistryMarkupTable` | ✅ 已修复：改用 `AppRegistryMarkupTable` + `DEAD_LETTER_COLUMNS` |
| AR-FB6-01 | FB6 | `features/memory/MemoryGraphExplorer.vue` | 表格列定义在 .vue 内 | ✅ 已修复：移至 `memoryTableUi.ts` `GRAPH_RELATION_COLUMNS` |
| AR-FB6-02 | FB6 | `components/memory/RecallHitTable.vue` | 表格列定义在 .vue 内 | ✅ 已修复：移至 `memoryTableUi.ts` `RECALL_HIT_COLUMNS` |
| AR-FB6-03 | FB6 | `components/usage/UsageAnomalyList.vue` | 表格列定义在 .vue 内 | ✅ 已修复：移至 `usageTableUi.ts` `USAGE_ANOMALY_TABLE_COLUMNS` |
| AR-BE4-01 | BE4 | `components/webhooks/WebhookDialog.vue` | 空 `catch {}` 吞掉 JSON 解析错误 | ✅ 已修复：`catch { headers = {}; }` |

### 8.5 🔴 遗留阻断项（历史债务，非本轮引入）

> 全部已修复 ✅

| ID | 维度 | 文件 | 问题描述 | 修复状态 |
|----|------|------|----------|----------|
| AR-FD2-01 | FD2 | `pages/SystemSettingsCatalogTab.vue` | Page 直接 import 10+ 函数从 `features/model-catalog/api`，违反红线 #11 | ✅ 已修复：创建 `useModelCatalogPage` composable，Page 瘦身至 ~50 行 |
| AR-FD1-01 | FD1 | `components/platform/ProviderLogo.vue` | 展示组件 import `fetchProviderLogoSvg`，该函数内部调用 `kratosApi`/`createModelCatalogService`，违反红线 #1/#2 | ✅ 已修复：改为 `loader` prop 注入，由 Page 层传入 `fetchProviderLogoSvg` |

### 8.6 🟡 遗留建议项（历史债务）

> 全部已修复 ✅

| ID | 维度 | 文件 | 问题描述 | 修复状态 |
|----|------|------|----------|----------|
| AR-FB4-03 | FB4 | `components/agents/AIRefineButton.vue` | 展示组件内 `$q.notify` | ✅ 已修复：改为 `emit('error', msg)`，4 个父组件添加 `@error` 处理 |
| AR-FB4-04 | FB4 | `components/common/JsonCodeViewer.vue` | 展示组件内 `$q.notify`（复制成功/失败） | ✅ 已修复：改为 `emit('copy', ok)`，composable 添加 `@copy` 处理 |

### 8.7 ✅ 亮点

- ✅ **跨 Store 同步合规**：`stores/app.ts` 不再直接 import 其他 Store，改用 `emitSessionMutation` 事件总线（FD8 通过）
- ✅ **Memory 域数据流合规**：4 个 Memory 面板组件已从直接 API 调用改为通过 `useMemoryStore` 获取数据（FD1 通过）
- ✅ **聊天消息分组合规**：无 `turn_index` 使用，堆栈模型正确（FM1-FM5 全通过）
- ✅ **backdrop-filter 成对**：所有浮层 CSS 均成对使用 `backdrop-filter` + `-webkit-backdrop-filter`（FU3 通过）
- ✅ **Dialog 使用 app-dialog-card**：`ToolEditorHelpDrawer.vue` 已添加 `app-dialog-card` class（FU4 通过）
- ✅ **UX 主题日间安全**：`CommandCenterStatusPanels.vue` gradient 已改用 CSS 变量 + 日间安全 fallback（FU2 通过）
- ✅ **错误处理改善**：`ws-transport.ts` JSON 解析错误不再静默吞掉；`streamHandlers.ts` runner_completion reload 失败有通知
- ✅ **死代码清理**：`api.ts` `cancelChatBackgroundJob` 不再有不可达的 `return false`
- ✅ **Store 具名导出**：`stores/index.ts` `useMemoryStore` 已正确具名导出，default export Pinia 工厂保留（FD7/FR9 通过）
- ✅ **sessionRevision 清理**：`messageStore.ts` `clearSessionMessages` 已同步删除 revision 跟踪
- ✅ **WS 重连持久提示**：`ws-transport.ts` 添加 `disconnected` ref + `onReconnectFailed` 回调 + `reconnect()` 方法，UI 层可持久显示断连状态
- ✅ **WS close code 区分**：`ws.onclose` 区分正常关闭（1000/1001）和异常关闭，正常关闭不触发自动重连
- ✅ **消息加载竞态保护**：`messageStore.ts` `loadMessages` 使用版本号模式，后发请求覆盖先发请求结果时自动丢弃
- ✅ **发送互斥锁**：`useChatSender.ts` `onSend` 使用 `sendGuard` 互斥锁，防止 TOCTOU 竞态导致重复发送
- ✅ **Session 切换断开旧 stream**：`useChatWorkspace.ts` `bindSessionView` 在切换 session 时先断开旧 WS stream
- ✅ **Industry Store 创建**：`useIndustryStore` 封装 6 个 API action，3 个 composable 改用 Store
- ✅ **Model Catalog Store 创建**：`useModelCatalogStore` 封装 14 个 API action，composable + Page 改用 Store
- ✅ **ProviderLogo svg prop**：展示组件不再直接调 API，通过 `svg` prop 优先注入，`loader` prop 次之
- ✅ **展示组件迁移**：5 个 Memory 展示组件从 `features/` 迁移至 `components/memory/`（FL1 通过）
- ✅ **分层检查通过**：`pnpm check:layer` 验证 components/ 无 Store/API import

### 8.8 前端合规性清单

- [x] 展示组件无 Store/API import（✅ ProviderLogo.vue 已改为 loader prop 注入）
- [x] Page 无直接 API import（✅ SystemSettingsCatalogTab.vue 已迁入 useModelCatalogPage composable）
- [x] Dialog/浮层 emit 而非内部调 API（MCP 2 个 Dialog 已改为 emit 模式）
- [x] 新 HTTP 调用在 api.ts
- [x] 跨 Store 同步走 sessionSync 事件总线
- [x] 聊天消息分组用堆栈模型（非 turn_index）
- [x] 浮层 backdrop-filter 成对
- [x] 主按钮用 --color-accent
- [x] Dialog 用 app-dialog-card
- [x] Registry 表格用 AppRegistryTable + registryCol()
- [x] 表格列定义在 *Ui.ts（MemoryGraphExplorer + MemoryDeadLetterPanel + RecallHitTable + UsageAnomalyList 列定义已移至 Ui.ts）
- [x] Page script ≤~200 行
