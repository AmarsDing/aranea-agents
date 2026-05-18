# 监控（Monitor）

本文档描述控制台 **监控** 相关页面的信息架构、控件与数据契约，与 **运行日志**、**审计**、**实时事件**、**LLM / 模型调用追踪** 对齐。**前端实现采用 Quasar Framework（Vue 3）**：路由 `vue-router`，布局 **`QLayout` / `QPageContainer` / `QPage`**，主题跟随控制台统一主题（`Quasar Dark Plugin` / `$q.dark`）。

> 2026-05-18 更新：标注各功能模块实现状态，与当前代码对齐。

**术语区分**

| 概念 | 含义 |
|------|------|
| **业务 Channel** | **`17 channel.md`** 中的消息接入（飞书、微信等），在 Agent 高级设置中与 `channel_id` 绑定 |
| **传输 Channel（日志字段）** | 事件 JSON 中的 `channel`，如 `ws`（WebSocket），表示**会话传输方式**，与业务 Channel 表无直接外键关系 |

---

## 0. 需求明确性评审与实现边界

### 0.1 监控模块要回答的问题

| 用户问题 | 对应页面 / Tab | 数据来源 | 实现状态 |
|----------|----------------|----------|----------|
| 系统最近发生了哪些管理操作？ | 活动日志 Audit | `audit_logs` / `/api/v1/monitor/audit` | ✅ 已实现 |
| Team / Agent 运行时现在正在发生什么？ | 实时事件 Events | `team-run-events` SSE、`monitor_events` | ✅ 已实现 |
| 哪些模型调用慢、失败、成本高？ | Usage / Traces | `model_token_usage_events`、`model_token_usage_daily`、`monitor_traces` | ✅ 已实现 |
| 某次对话为什么失败？ | Trace 详情 | Trace + spans + error payload | ✅ 已实现 |
| Gateway / 后端进程是否有异常日志？ | Logs | WS 推送 + 内存快照 | ✅ 已实现 |

### 0.2 模块实现状态

| 模块 | 状态 | 说明 |
|------|------|------|
| Audit | ✅ 已实现 | 表格、刷新、分页（limit/offset）、事件类型/实体类型/操作者/关键字筛选、详情弹窗、扩展字段（actor/ip/user_agent/severity/metadata_json） |
| Events | ✅ 已实现 | 持久化事件列表 + SSE 实时运行事件、分类 Tab（全部/任务/消息/Agent/工具/系统）、JSON 详情、暂停/恢复/清除 |
| Traces | ✅ 已实现 | 列表与 JSON 详情、Span 树提取、Agent/Provider/Model/Status 过滤、分页 |
| Usage | ✅ 已实现 | 总览卡（请求数/成功率/Token/费用）、Top 模型、Top Agent、最近异常、时间范围选择 |
| Logs | ✅ 已实现 | WS 日志流、级别过滤、关键字搜索、实时状态指示、暂停/恢复/清除 |

### 0.3 非目标

- 不在监控页修改 Agent、Channel、Provider 配置；只跳转到对应管理页。
- 不存储或展示完整用户隐私内容，日志与事件 payload 默认截断 / 脱敏。
- 不把事件 JSON 里的 `channel: "ws"` 当作业务 `channel.id` 使用。

---

## 1. 日志（Logs）

面向运维与开发：展示 **Gateway / 运行时** 文本日志流（类终端），支持级别过滤与关键字过滤。

> 实现状态：✅ 已实现。通过 `LogStream.vue` 组件 + WebSocket 推送 + HTTP REST 快照。

### 1.1 页面结构

| 区域 | 内容 |
|------|------|
| **标题** | 「日志」 |
| **副标题** | 当前追踪级别说明，如「正在实时追踪 **info** 级别」 |
| **右上** | **实时** 状态指示（绿色）；**停止**（暂停拉流）；**清除**（清空当前视图缓冲） |
| **工具行** | 搜索框，占位「过滤日志…」；级别 **DEBUG** \| **INFO** \| **WARN** \| **ERROR**（可多选或单选高亮）；右侧 **已显示/匹配条数**（如 `101/101`） |
| **主体** | 等宽字体、深色背景、可横向滚动；自底向上或自顶向下追加，视产品定 |

### 1.2 行格式（示例）

```text
20:50:51 [WARN] no channels enabled
20:50:51 [INFO] goclaw gateway starting agents=[] channels=[] protocol=3 tools=38 version=dev
```

| 片段 | 说明 |
|------|------|
| 时间戳 | `HH:mm:ss` 或 ISO8601 |
| 级别 | `DEBUG` / `INFO` / `WARN` / `ERROR` |
| 正文 | 自由文本；可含 `key=value` 片段便于检索 |

### 1.3 数据来源与行为

| 项 | 实现 |
|------|------|
| 传输 | WebSocket（`subscribeMonitorLogsWs`）+ HTTP REST（`getMonitorLogs` 快照） |
| 过滤 | 前端在已收流上按关键字子串过滤时，更新「匹配数/总数」 |
| 缓冲 | 前端最多保留最近 5,000 行；用户可清空视图但不删除后端日志 |

### 1.4 空态与异常态

| 场景 | 表现 |
|------|------|
| 后端未实现日志流 | 显示提示：「日志流尚未启用，请使用 Audit / Events / Traces 查看结构化监控」 |
| WS 断开 | 顶部实时状态变为灰色；显示「已断开，点击重连」按钮 |
| 大量日志 | 前端最多保留最近 5,000 行 |

### 1.5 Quasar 映射（日志）

| 区域 | Quasar 组件 |
|------|-------------|
| 页面骨架 | `QPage` + `QCard` |
| 实时 / 停止 / 清除 | `QBtn`；实时状态用 `QBadge` color=`positive` |
| 过滤行 | `QInput` `debounce="300"` + `clearable`；级别用 `QBtnToggle` |
| 计数 | `span.text-caption` |
| 日志主体 | `QScrollArea` + `<pre>` 按级别上色 |
| 长列表性能 | `QVirtualScroll` |

---

## 2. 活动日志（Audit / Activity Log）

面向管理员：**配置与管理层面的审计**，非逐行运行时日志。

> 实现状态：✅ 已实现。通过 `AuditTable.vue` 组件 + `MonitorService.ListAuditLogs` API。
> 增强项：分页（limit/offset）、事件类型/实体类型/操作者/关键字筛选、扩展字段（actor/ip/user_agent/severity/metadata_json）、详情弹窗。

### 2.1 页面结构

| 区域 | 内容 |
|------|------|
| **标题** | 「活动日志」 |
| **副标题** | 说明为管理/配置变更的审计记录 |
| **右上** | **刷新** |
| **筛选** | 下拉 **事件类型**（如：create/delete/update/toggle/credentials）；下拉 **实体类型**（如：agent/team/channel/provider/config/session）；搜索框（事件/资源/详情） |
| **表格** | 见下表 |
| **底栏** | 总条数；每页条数；翻页 |

### 2.2 表格列

| 列 | 说明 |
|------|------|
| **事件** | 彩色标签，如 `create.agent`、`update.team`、`credentials.update.channels` |
| **实体** | 对象类型 + 实体 ID |
| **操作者** | 用户 ID 或 `system`；下方显示 IP |
| **Request ID** | 请求追踪 ID |
| **详情** | 变更详情 |
| **时间** | 本地化时间 |

### 2.3 数据与 API

| 方法 | 说明 |
|------|------|
| GET | `/api/v1/monitor/audit?limit=200&offset=0&action=&resource=&actor=&keyword=` |
| 字段 | `id`、`action`、`resource`、`resource_id`、`request_id`、`detail`、`created_at`、`actor`、`ip`、`user_agent`、`severity`、`metadata_json` |
| 响应 | `{ items: AuditLog[], total: number }` |

### 2.4 Audit 详情弹窗

点击行打开 `QDialog`，展示：

| 区块 | 内容 |
|------|------|
| 摘要 | `action`、`resource`、`resource_id`、时间 |
| 操作者 | `actor`、`ip` |
| 严重级别 | `severity`（带颜色 Badge） |
| Detail | `detail` 原文；若是 JSON 字符串则格式化展示 |
| 操作 | 复制 JSON |

---

## 3. 实时事件（Real-time Events）

展示 **Team / Agent** 侧经 **SSE / WebSocket** 推送的**结构化事件流**。

> 实现状态：✅ 已实现。通过 `RealtimeEvents.vue` 组件 + `MonitorService.ListMonitorEvents` + WS 推送。
> 增强项：分类 Tab（全部/任务/消息/Agent/工具/系统）、事件类型/Agent ID/状态过滤、分页。

### 3.1 页面结构

| 区域 | 内容 |
|------|------|
| **标题** | 「实时事件」 |
| **右上** | **实时** 指示；**事件计数**；**暂停**；**清除** |
| **分类 Tab** | **全部** \| **任务** \| **消息** \| **Agent** \| **工具** \| **系统** |
| **列表** | 卡片流，每条含：时间、事件类型标签、摘要正文、元数据 |

### 3.2 事件分类映射

| 分类 | 匹配规则 |
|------|----------|
| 任务 | `run.*`、`team_run.*` |
| 消息 | `message.*`、`chat.*` |
| Agent | `agent.*`、`agent_link.*` |
| 工具 | `tool.*` |
| 系统 | `system.*`、`runtime.*` |

### 3.3 详情弹窗（JSON）

点击卡片打开 Modal：语法高亮 JSON，带复制、关闭。

### 3.4 实时连接状态

| 状态 | UI |
|------|----|
| connecting | 黄色点 +「连接中」 |
| live | 绿色点 +「实时」 |
| paused | 灰色点 +「已暂停」 |
| error | 红色点 + 错误摘要 +「重连」 |

---

## 4. 追踪（LLM Traces）与 Usage 总览

展示 **LLM 调用链** 与性能：**Span 树**、Token 进出、延迟；支持按 Agent、Provider、Model 筛选。

> 实现状态：✅ 已实现。
> - Usage 总览：通过 `UsageOverview.vue` + `UsageService` API
> - Trace 列表：通过 `TraceList.vue` + `UsageService.ListUsageEvents` + `MonitorService.ListMonitorTraces`

### 4.0 Usage 总览

| 区域 | 内容 |
|------|------|
| 指标卡 | 今日请求数、成功率、输入/输出 Token、总成本 |
| Top 模型 | 按成本/调用数排序，展示 provider、model、成本、成功率 |
| Top Agent | 按调用量/成本排序，展示 Agent、Token、成功率 |
| 最近异常 | 最近失败模型调用，显示时间、Agent、Provider、错误信息 |
| 时间范围 | 今日/7天/30天/本月 |

后端 API：

| API | 用途 |
|-----|------|
| `GET /api/v1/usage/overview` | 总览指标 |
| `GET /api/v1/usage/top-models` | Top 模型 |
| `GET /api/v1/usage/top-agents` | Top Agent |
| `GET /api/v1/usage/events` | 最近调用事件 |

### 4.1 Trace 列表

| 区域 | 内容 |
|------|------|
| **标题** | 「追踪」 |
| **副标题** | 「LLM 调用追踪和性能数据」 |
| **筛选** | Agent、Provider、Model、Status |
| **表格列** | 名称、令牌（in/out）、跨度、时间 + 延迟 |

### 4.2 追踪详情弹窗

| 区块 | 内容 |
|------|------|
| **摘要** | 名称、状态、耗时、Channel、Tokens、Span 统计 |
| **操作** | 复制追踪 ID |
| **错误** | 失败时红色区域展示完整日志 |
| **跨度树** | 嵌套：agent → llm_call；每节点含时间、耗时、状态、模型 |

### 4.3 数据与 API

| 方法 | 说明 |
|------|------|
| GET | `/api/v1/monitor/traces?limit=100&offset=0&agent_id=&provider=&model=&status=` |
| GET | `/api/v1/monitor/traces/:traceId` | 详情 + spans 树 |
| GET | `/api/v1/usage/events?agent_id=&provider=&model=&limit=` | 模型调用事件表 |

---

## 5. 路由与导航

### 5.1 当前路由

| 路径 | 页面 |
|------|------|
| `/monitor/logs` | `MonitorPage.vue`，内部使用 5 Tabs 展示 Usage / Audit / Events / Traces / Logs |

Tab 状态同步到 query：`/monitor/logs?tab=audit`，便于刷新后保留当前视图。

### 5.2 后续可拆分路由

| 路径（建议） | 页面 |
|--------------|------|
| `/monitor/audit` | 活动日志 |
| `/monitor/events` | 实时事件 |
| `/monitor/usage` | Usage 总览 |
| `/monitor/traces` | 追踪 |
| `/monitor/logs` | 日志 |

---

## 6. 统一数据契约与前端状态

### 6.1 API 返回格式

| 类型 | 字段 |
|------|------|
| `PaginatedResult<T>` | `items: T[]`、`total: number` |
| `LoadState` | `idle` / `loading` / `success` / `empty` / `error` |
| `StreamState` | `connecting` / `live` / `paused` / `error` |

### 6.2 前端模块

| 文件 | 职责 |
|------|------|
| `pages/MonitorPage.vue` | 页面壳、5 Tab（Usage/Audit/Events/Traces/Logs） |
| `components/monitor/UsageOverview.vue` | 模型用量总览 |
| `components/monitor/AuditTable.vue` | 活动日志表格（筛选 + 分页） |
| `components/monitor/RealtimeEvents.vue` | SSE 事件流 |
| `components/monitor/TraceList.vue` | Trace 列表与详情 |
| `components/monitor/LogStream.vue` | 日志流 |
| `features/monitor/api.ts` | Monitor API（含分页/过滤参数） |
| `features/monitor/types.ts` | 类型定义（含 AuditQuery/PaginatedResult） |
| `features/monitor/utils.ts` | 格式化工具 |
| `features/usage/api.ts` | Usage API |
| `features/usage/types.ts` | Usage 类型 |
| `stores/monitor/index.ts` | Pinia Store |

### 6.3 数据保留与脱敏

- JSON 详情默认折叠大字段，单字段超过 2,000 字符时显示「展开」。
- 密钥、Token、Authorization、Cookie、API Key 等字段统一用 `******` 脱敏。
- SSE 前端缓冲默认最多 1,000 条事件；Logs 默认最多 5,000 行。

---

## 7. 验收要点

- [x] 页面进入 `/monitor/logs` 后能正常加载 Audit / Events / Traces / Usage 数据，失败时显示可读错误。
- [x] 活动日志：表格列与 API 字段一致；支持刷新、分页、事件类型/实体类型/操作者/关键字筛选、详情查看。
- [x] 实时事件：SSE 连接状态清晰；支持暂停、恢复、清除、JSON 详情；分类 Tab。
- [x] Usage：总览卡、Top 模型、Top Agent、最近异常能从 `/api/v1/usage/*` 加载。
- [x] 追踪：列表与详情能展示 Token、耗时、状态、错误信息；存在 spans 时展示 Span 树。
- [x] 日志流：支持开始/停止、级别过滤、关键字过滤、计数。
- [x] 所有 JSON 详情支持复制，复制成功有 `Notify`。
- [x] 大量数据场景不明显卡顿：长列表使用分页、虚拟滚动或限制前端缓冲。
- [x] 敏感字段已脱敏，不展示明文密钥、Token、Cookie。

---

*文档版本：2026-05-18 — 与当前代码实现完全对齐，标注各模块实现状态，补充分页/过滤/Usage 整合等设计。*
