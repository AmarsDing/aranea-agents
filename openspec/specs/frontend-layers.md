# 前端分层规范

> 来源：项目规则 + `aranea-frontend-guide` SKILL 精简版。

---

## 一、数据流方向

```
services/index.ts (createXxxService)
  → features/<域>/api.ts (HTTP 门面 + 类型归一化)
    → stores/<域>/index.ts (状态 + action 调 api)
      → features/<域>/useXxxPage.ts (composable 组合 Store)
        → pages/XxxPage.vue (布局 + 传参)
          → components/<域>/*.vue (纯展示：props in / emits out)
```

**数据流只允许从左到右。**

---

## 二、各层约束

### services/ (`services/index.ts`)

| 规则 | 说明 |
|------|------|
| HTTP 客户端工厂 | `createXxxService()` 或 `kratosApi` |
| 不写业务逻辑 | 只创建 HTTP 调用实例 |

### features/<域>/api.ts

| 规则 | 说明 |
|------|------|
| HTTP 门面 + 类型归一化 | 所有 HTTP 调用写在此文件（红线 #7） |
| 禁止裸 URL / 散装 axios | 必须经 `services/` 的工厂方法（红线 #7） |
| 共享类型放 `features/<域>/types.ts` | 组件只 import types（红线 #13） |

### stores/<域>/index.ts

| 规则 | 说明 |
|------|------|
| 状态 + action | action 内调 api.ts（红线 #2） |
| 新 Store 必须在 `stores/index.ts` 具名导出 | 不得删除 default export Pinia 工厂（红线 #6） |
| 跨 Store 同步走 `stores/sessionSync.ts` | 禁止直接 import 另一 Store 导致循环依赖（红线 #11） |

### composables / useXxxPage.ts

| 规则 | 说明 |
|------|------|
| 组合 Store | 多页面复用逻辑放 composables |
| 单页编排过重拆 composable | `features/<域>/useXxxPage.ts` 或 `useXxxPanel.ts` |

### pages/XxxPage.vue

| 规则 | 说明 |
|------|------|
| 布局 + composable 绑定 + 传参 | 不直接 import `features/*/api`（红线 #12） |
| `<script setup>` ≤ ~200 行 | 超过则拆 Dialog + composable + 子面板（红线 #14） |

### components/<域>/*.vue

| 规则 | 说明 |
|------|------|
| 纯展示：props in / emits out | 不得 import `useXxxStore` / `defineStore`（红线 #1） |
| 不得 import api / services / axios | 网络请求只在 Store action（红线 #2） |
| 不得 watch + fetch + ref 存共享数据 | 应进 Store（红线 #3） |
| Dialog/Drawer 不得直接调 API | `emit('submit', payload)`，由 Page 或 Store 调（红线 #4） |
| 展示组件放 `components/<域>/` | 禁止放在 `features/<域>/`（红线 #5） |

---

## 三、实时通信层

```
WebSocket Server (Go /v1/ws)
  → ws-transport.ts (原始 WS 连接、重连、心跳)
    → globalWsHub.ts (共享 session_id=* 连接)
      → useEnvelopeStream.ts (Composable 工厂)
        → dispatcher.ts (EnvelopeDispatcher: 按 type/channel/sessionId/teamId 发布订阅)
          → features/chat/composables/useChatStreamManager.ts
          → features/monitor/api.ts
          → features/teams/api.ts
          → features/graph/runtime/useGraphExecutionStream.ts
          → features/orchestration/useOrchestrationStream.ts
```

**47 种 Envelope 类型**：text_delta、tool_call、tool_result、runner_completion、context_usage、graph_node_start/end、team_run_started/finished、alert.notify、session.status_changed 等。

---

## 四、跨 Store 通信

| 机制 | 生产者 | 消费者 | 说明 |
|------|--------|--------|------|
| sessionSync 事件总线 | ChatSessionStore、SessionStore | ChatSessionStore | 会话变更通知（remove/update/archive/refresh/status_changed） |
| AppStore → ChatStore | AppStore | ChatSessionStore、ChatMessageStore | Agent 切换时重置 |
| InboundNotificationStore | WS 事件 | InboundNotificationBell | 通知铃铛 |

**跨 Store 同步必须通过 `stores/sessionSync.ts` 事件总线**（红线 #11）。

---

## 五、CSS 主题规范

| 规则 | 说明 |
|------|------|
| 单入口 | `app-theme.sass` 聚合 partial（红线 #10） |
| 昼夜切换 | CSS 变量 + Quasar Dark mode（红线 #9） |
| 新 token | 在 `theme/` 增加 partial，由 `app-theme.sass` 聚合 |
| 禁止运行时改 quasar-variables | 昼夜仅用 Dark + CSS 变量 + body 选择器（红线 #9） |
| 浮层视觉 | `backdrop-filter` + `-webkit-backdrop-filter` 成对；主按钮用 `--color-accent`（红线 #8） |
| 禁止日间用夜间霓虹青紫 | 默认强调色遵守 UX 规范（红线 #8） |

### CSS 变量文件

- 新增 CSS 变量：`css/theme/_css-vars-light.sass` + `_css-vars-dark.sass`

---

## 六、消息分组规范

**禁止使用 `turn_index` 做消息分组，必须使用堆栈模型**（红线 #15）。

`groupMessagesByTurn`：按 `role=user` 边界 + 时间顺序分组。

---

## 七、页面路由

| 路由 | 页面 | 核心功能 |
|------|------|---------|
| `/overview` | OverviewPage | 用量概览 |
| `/chat` | ChatPage | 聊天工作台 |
| `/sessions` | SessionsPage | 会话列表 |
| `/memory` | MemoryCenterPage | 记忆中心 |
| `/agents` | AgentsPage | Agent 列表 |
| `/agents/:id/settings` | AgentSettingsPage | Agent 设置 |
| `/team` | TeamsPage | Team 编排 |
| `/graphs` | GraphsPage | Graph 列表 |
| `/graphs/:id/edit` | GraphEditorPage | Graph 编辑器 |
| `/graphs/:id/run` | GraphRunPage | Graph 执行态 |
| `/models` | ResourceManagerPage | LLM 管理 |
| `/channels` | ChannelsPage | 渠道管理 |
| `/tools` | ToolsPage | 工具目录 |
| `/monitor/logs` | MonitorPage | 监控日志 |
| `/cron` | CronTasksPage | 定时任务 |
| `/hooks` | HooksPage | Webhook |
| `/knowledge` | KnowledgePage | 知识库 |
| `/plugins` | PluginsPage | 插件 |
| `/skills` | SkillsPage | 技能 |
| `/mcp-servers` | McpServersPage | MCP 服务器 |
| `/a2a` | A2APage | A2A 端点 |
| `/evaluation` | EvaluationPage | 评估 |
| `/settings` | SystemSettingsPage | 系统设置 |

---

## 八、验证命令

```bash
cd web && pnpm lint && pnpm test && pnpm build
```
