# 监控（Monitor）

本文档描述控制台 **监控** 相关页面的信息架构、控件与数据契约，与 **运行日志**、**审计**、**实时事件**、**LLM / 模型调用追踪** 对齐。**前端实现采用 Quasar Framework（Vue 3）**：路由 `vue-router`，布局 **`QLayout` / `QPageContainer` / `QPage`**，主题跟随控制台统一主题（`Quasar Dark Plugin` / `$q.dark`）。各页 **Quasar 组件映射** 见 **§1.5、§2.4、§3.4、§4.5**；侧栏与路由见 **§5**。

当前需求的方向明确，但原文有三处需要收敛：一是“日志 / 实时事件 / Trace / 模型用量”的边界容易混淆；二是 API 路径需与 aranea 当前后端 `/api/v1/*` 对齐；三是前端首版应明确是单页多 Tab 还是多子路由。本版按 aranea 现状建议：**首版使用单个 `MonitorPage.vue` + Tabs 承载 Audit / Events / Traces / Usage，后续再拆子路由。**

**术语区分**

| 概念 | 含义 |
|------|------|
| **业务 Channel** | **`17 channel.md`** 中的消息接入（飞书、微信等），在 Agent 高级设置中与 `channel_id` 绑定 |
| **传输 Channel（日志字段）** | 事件 JSON 中的 `channel`，如 `ws`（WebSocket），表示**会话传输方式**，与业务 Channel 表无直接外键关系 |

---

## 0. 需求明确性评审与实现边界

### 0.1 监控模块要回答的问题

| 用户问题 | 对应页面 / Tab | 数据来源 |
|----------|----------------|----------|
| 系统最近发生了哪些管理操作？ | 活动日志 Audit | `audit_logs` / `/api/v1/monitor/audit` |
| Team / Agent 运行时现在正在发生什么？ | 实时事件 Events | `team-run-events` SSE、`monitor_events` |
| 哪些模型调用慢、失败、成本高？ | Usage / Traces | `model_token_usage_events`、`model_token_usage_daily`、`monitor_traces` |
| 某次对话为什么失败？ | Trace 详情 | Trace + spans + error payload |
| Gateway / 后端进程是否有异常日志？ | Logs | 后续接入日志流 API；首版可作为预留 Tab |

### 0.2 首版 MVP 范围

| 模块 | 首版是否实现 | 说明 |
|------|--------------|------|
| Audit | 是 | 当前后端已有 `/api/v1/monitor/audit`，首版做表格、刷新、分页、关键字筛选 |
| Events | 是 | 当前后端已有 `monitor-events` 平台资源与 `team-run-events` SSE，首版展示结构化事件列表 |
| Traces | 是 | 当前后端已有 `monitor-traces` 平台资源，首版展示列表与 JSON 详情；Span 树可先兼容空数据 |
| Usage | 是 | 当前后端已有 `/api/v1/model-usage/*`，建议加入总览卡、趋势、Top 模型/Agent、事件表 |
| Logs | 可后置 | 原文提到 Gateway 文本日志流，但当前 aranea 后端未见 `/monitor/logs/stream`；先保留设计，不阻塞 MVP |

### 0.3 非目标

- 不在监控页修改 Agent、Channel、Provider 配置；只跳转到对应管理页。
- 不存储或展示完整用户隐私内容，日志与事件 payload 默认截断 / 脱敏。
- 不把事件 JSON 里的 `channel: "ws"` 当作业务 `channel.id` 使用。

---

## 1. 日志（Logs）

面向运维与开发：展示 **Gateway / 运行时** 文本日志流（类终端），支持级别过滤与关键字过滤。

> 实现说明：当前 aranea 已有 Audit、monitor events、model usage 等结构化监控接口，但未看到稳定的文本日志流 API。Logs 页建议作为第二阶段能力；若首版必须展示，可先接后端本地内存 ring buffer 或进程 stdout tail，但需避免读取任意文件路径。

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

| 项 | 建议 |
|------|------|
| 传输 | **SSE** 优先，建议路径 `GET /api/v1/monitor/logs/stream?level=info`；停止即断开或退订 |
| 过滤 | 前端在已收流上按关键字子串过滤时，更新「匹配数/总数」 |
| 与 Channel | 日志中可出现 `channels=[]`、`no channels enabled` 等，表示 **Gateway 当前加载的业务通道配置**，与 **`17 channel.md`** 中启用状态一致时需核对配置与进程重启 |

### 1.4 空态与异常态

| 场景 | 表现 |
|------|------|
| 后端未实现日志流 | 显示 `QBanner`：「日志流尚未启用，请使用 Audit / Events / Traces 查看结构化监控」 |
| SSE 断开 | 顶部实时状态变为灰色；显示「已断开，点击重连」按钮 |
| 大量日志 | 前端最多保留最近 5,000 行；用户可清空视图但不删除后端日志 |

### 1.5 Quasar 映射（日志）

| 区域 | Quasar 组件 / 说明 |
|------|---------------------|
| 页面骨架 | `QPage` + `QCard` 或扁平 `div` + `q-pa-md` |
| 标题行 | `QCardSection` 或 `div.row.items-center`；副标题用 `text-caption text-grey` |
| 实时 / 停止 / 清除 | `QBtn`（停止可用 `outline`/`flat`）；实时状态用 `QBadge` color=`positive` 或 `QChip` |
| 过滤行 | `QInput` `debounce="300"` + `clearable` + 前缀图标 `search`；级别用 **`QBtnToggle`** 或 **`QTabs` narrow** + `no-caps`，或 `QChip` 可点击多选 |
| 计数 | `span.text-caption` 或 `QBadge` |
| 日志主体 | **`QScrollArea`**（横向 `bar-style`）包一层；内部 `pre` / `div` + `text-green`/`text-orange` 等按级别上色；或 **`QMarkupTable`** 仅当按行拆表格时 |
| 长列表性能 | 超大量行时用 **`QVirtualScroll`**（`type="list"`）替代整块 `innerHTML` 拼接 |

---

## 2. 活动日志（Audit / Activity Log）

面向管理员：**配置与管理层面的审计**，非逐行运行时日志。

### 2.1 页面结构

| 区域 | 内容 |
|------|------|
| **标题** | 「活动日志」 |
| **副标题** | 说明为管理/配置变更的审计记录 |
| **右上** | **刷新** |
| **筛选** | 下拉 **事件类型**（如：全部、创建 Agent、更新 Agent、删除 Agent…）；第二只下拉可为 **实体类型** 或 **操作者**（截图中为「全部」） |
| **表格** | 见下表 |
| **底栏** | 总条数；每页条数（如 20）；**第 x/y 页**；翻页箭头 |

### 2.2 表格列

| 列 | 说明 |
|------|------|
| **事件** | 彩色标签，如 `agent.created`、`agent.updated`、`config.patched`、`session.deleted`、`provider.created`、`team.created` 等 |
| **操作者** | 如 `user:system` 或用户 ID |
| **实体** | 对象类型：`agent`、`config`、`session`、`provider`、`team`、`agent_link` 等 |
| **实体 ID** | UUID 或业务键（如 `gateway`） |
| **IP 地址** | 客户端 IP，可为 IPv6 `::1` 或带端口 |
| **时间** | 本地化时间，如 `Apr 13, 10:21:49 PM` |

### 2.3 数据与 API

| 方法 | 说明 |
|------|------|
| GET | `/api/v1/monitor/audit?limit=50` |
| 当前字段 | `id`、`action`、`resource`、`resource_id`、`request_id`、`detail`、`created_at` |
| 建议扩展字段 | `actor`、`ip`、`user_agent`、`severity`、`metadata_json` |

当前 aranea `AuditLog` 更接近「操作审计」而非传统安全审计，事件列应由 `action + resource` 组合展示，例如 `create.channels`、`update.team`、`credentials.update.channels`。

### 2.4 Quasar 映射（活动日志）

| 区域 | Quasar 组件 / 说明 |
|------|---------------------|
| 刷新 | `QBtn` `icon="refresh"` + `round`/`flat` 或带文案「刷新」 |
| 筛选 | 两只 **`QSelect`**：`emit-value` `map-options` `clearable`，选项来自枚举 |
| 表格 | **`QTable`**：`columns` 定义 `name`/`field`/`align`；事件列用 **`QBadge`** 或 **`QChip`** `:color` 按 `event` 映射 |
| 分页底栏 | **`QPagination`** + `QSelect` 每页条数；或 `QTable` 自带 `pagination` + `@request` 服务端分页 |
| 空/加载 | `QInnerLoading` / `QSkeleton` |

### 2.5 Audit 详情抽屉

点击行打开右侧 `QDrawer` 或 `QDialog`，展示：

| 区块 | 内容 |
|------|------|
| 摘要 | `action`、`resource`、`resource_id`、时间 |
| 请求 | `request_id`，后续可扩展 IP、User-Agent |
| Detail | `detail` 原文；若是 JSON 字符串则格式化展示 |
| 操作 | 复制 JSON、跳转到相关资源（如 Channel / Agent / Team） |

---

## 3. 实时事件（Real-time Events）

展示 **Team / Agent** 侧经 **SSE / WebSocket** 推送的**结构化事件流**（`tool.call`、`run.failed` 等），与 §1 文本日志互补。

在 aranea 当前后端中，Team 运行事件已有 **SSE**：`GET /api/v1/team-run-events?team_id=`；监控事件资源已有通用平台资源：`GET /api/v1/monitor/events`。因此本页应分清两类来源：

| 来源 | 用途 | 更新方式 |
|------|------|----------|
| `team-run-events` SSE | 实时运行过程、工具调用、运行状态 | 实时追加 |
| `monitor-events` 平台资源 | 已持久化或配置化的监控事件 | 刷新加载 |

### 3.1 页面结构

| 区域 | 内容 |
|------|------|
| **标题** | 「实时事件」 |
| **副标题** | 说明为 Team 与 Agent 的实时事件 |
| **右上** | **实时** 指示；**事件计数**（如「20 个事件」）；**暂停**；**清除** |
| **分类 Tab** | **全部** \| **任务** \| **消息** \| **Agent** \| **Team管理** \| **Agent链接** |
| **下拉** | **所有用户**、**所有对话**（筛选作用域） |
| **列表** | 卡片流，每条含：时间（如相对时间 `7d ago`）、事件类型标签、摘要正文、元数据（`run` / `call` / `chat` 等） |
| **角标** | 可滚动到底部按钮（新事件在底部时） |

### 3.2 事件类型与标签色（示例）

| 类型 | 标签色（示例） | 说明 |
|------|----------------|------|
| `tool.call` / `tool.result` | 橙 | 工具调用与结果 |
| `run.started` | 蓝 | 一次运行开始 |
| `run.completed` | 绿 | 运行成功结束 |
| `run.failed` | 红 | 运行失败，正文含错误栈或 API 返回 JSON |
| `agent_link.created` 等 | 青 | 与 **`8 agent-title.md`**、编排中的 Agent 链接相关 |

建议补充事件分类映射：

| 分类 | 匹配规则 |
|------|----------|
| 任务 | `run.*`、`team_run.*` |
| 消息 | `message.*`、`chat.*` |
| Agent | `agent.*`、`agent_link.*` |
| 工具 | `tool.*` |
| 系统 | `system.*`、`runtime.*` |

### 3.3 详情弹窗（JSON）

点击卡片打开 **Modal**：标题可为分类名（如 `agent`）+ 时间；正文为 **语法高亮 JSON**，带 **复制**、**关闭**。

**载荷示例（`tool.call`）**：

```json
{
  "type": "tool.call",
  "agentId": "fox-spirit",
  "runId": "eb9b0d66-dd70-4b93-9f71-e283ee8effa3",
  "payload": {
    "arguments": { "query": "whoami about identity" },
    "id": "call_ec1ee0fdb95ecc1018761031b3c584acc2d",
    "name": "skill_search"
  },
  "userId": "system",
  "channel": "ws",
  "sessionKey": "agent:fox-spirit:ws:direct:79c85637-55ff-4ed0-82ef-21b0b4dd82ca"
}
```

| 字段 | 含义 |
|------|------|
| `channel` | **传输通道**（如 `ws`），非 **`channel` 表 ID** |
| `sessionKey` | 会话复合键，可解析出 agent、传输方式、直连/chat 等 |
| `run.failed` | `payload.error` 可含 LLM 厂商返回的 JSON（如 DeepSeek `tool_calls` 与 tool 消息顺序错误） |

### 3.4 Quasar 映射（实时事件）

| 区域 | Quasar 组件 / 说明 |
|------|---------------------|
| 顶栏按钮 | `QBtn`：暂停、清除；计数用 `QBadge` 或文案插槽 |
| 分类 Tab | **`QTabs` + `QTab` + `QTabPanels` + `QTabPanel`**，`align="left"` `no-caps`；或 `QRouteTab` 若需深链 |
| 下拉筛选 | **`QSelect`** ×2（用户、对话），可 `use-input` + `input-debounce` 远程搜索 |
| 事件列表 | **`QList` + `QItem` + `QItemSection`** 卡片感；类型标签用 **`QBadge`** / **`QChip`** `:color` 绑定 §3.2 |
| 滚动到底 | `QPageSticky` + `QBtn` `fab` `icon="keyboard_arrow_down"`，或列表 `ref` 调 `scrollTo` |
| JSON 详情 | **`QDialog`** + **`QCard`**；正文 **`QMarkupTable`** 不适用 JSON，用 **`QScrollArea`** + `<pre>` 或引入 **`prismjs`/`shiki`** 高亮；顶部 **`QBtn`**「复制」`@click` 写剪贴板 + **`Notify`** |
| 复制成功 | `Notify.create({ message: '已复制', position: 'top' })` |

### 3.5 实时连接状态

| 状态 | UI |
|------|----|
| connecting | 黄色点 +「连接中」 |
| live | 绿色点 +「实时」 |
| paused | 灰色点 +「已暂停」 |
| error | 红色点 + 错误摘要 +「重连」 |

暂停只停止前端追加；若使用 SSE，暂停可直接关闭连接，再次点击恢复时重新订阅。

---

## 4. 追踪（LLM Traces）

展示 **LLM 调用链** 与性能：**Span 树**、Token 进出、延迟；支持按 Agent、**业务渠道**筛选（与 **`17 channel.md`** 展示名/类型对齐的筛选器）。

建议将本节拆成两个 Tab：**Usage 总览** 与 **Trace 列表**。Usage 回答「整体成本/性能如何」，Trace 回答「某一次调用为什么这样」。

### 4.0 Usage 总览（建议新增）

| 区域 | 内容 |
|------|------|
| 指标卡 | 今日请求数、成功率、输入/输出 Token、总成本、平均延迟 |
| 趋势图 | 近 7 / 30 天调用次数、Token、成本、错误数 |
| Top 模型 | 按成本 / 调用数排序，展示 provider、model、成本、成功率 |
| Top Agent | 按调用量 / 成本排序，展示 Agent、Token、失败率 |
| 最近异常 | 最近失败模型调用，显示时间、Agent、Provider、错误码 |

当前后端可用 API：

| API | 用途 |
|-----|------|
| `GET /api/v1/model-usage/overview` | 总览指标 |
| `GET /api/v1/model-usage/trends` | 趋势 |
| `GET /api/v1/model-usage/top-models` | Top 模型 |
| `GET /api/v1/model-usage/top-agents` | Top Agent |
| `GET /api/v1/model-usage/events` | 最近调用事件 |

### 4.1 列表页

| 区域 | 内容 |
|------|------|
| **标题** | 「追踪」 |
| **副标题** | 「LLM 调用追踪和性能数据」 |
| **右上** | **刷新** |
| **筛选** | **所有 Agent**；**所有渠道**（此处「渠道」为业务或会话来源维度，可与 Web/会话标签及 **`channel` 配置**联合展示） |
| **表格列** | 见下 |

### 4.2 表格列

| 列 | 说明 |
|------|------|
| **名称** | Agent 展示名 + 头像；下方 **渠道标签**：如 **网页**（Web）、**ws** 及会话/别名（如 `www`、`111`），用于对应一次对话入口 |
| **令牌** | `in / out` Token；失败或未记录时可显示 **0 / 0** 与错误态图标 |
| **跨度** | Span 数量（整数） |
| **时间** | 发生时间 + **延迟**（如 `178ms`） |

### 4.3 追踪详情弹窗

| 区块 | 内容 |
|------|------|
| **标题** | 「追踪详情」 |
| **摘要** | 名称（如 `chat fox-spirit`）、**状态**（`ok` / `error`）、**耗时**、**Channel**（如 `ws`）、**Tokens**、**Span 统计**（含「x LLM 调用, y 工具」）、起止时间 |
| **操作** | **复制追踪 ID**、**导出追踪** |
| **输入** | 用户输入原文（如 `www`） |
| **错误** | 失败时红色区域展示完整日志（如 DeepSeek **401** API Key 无效、**400** tool 消息顺序等） |
| **跨度树** | 嵌套：**agent**（如 `fox-spirit`）→ **llm_call**（`provider/model #n`）；每节点含时间、耗时、状态、模型、输入摘要 |

### 4.4 数据与 API

| 方法 | 说明 |
|------|------|
| GET | `/api/v1/monitor/traces?agent_id=&channel_id=&page=`（`channel_id` 若存在则与 **`channel` 主表**关联；否则仅按会话维度过滤） |
| GET | `/api/v1/monitor/traces/:traceId` | 详情 + spans 树；当前可先用 `PlatformResource` 详情承载 |
| GET | `/api/v1/model-usage/events?agent_id=&provider=&model=&limit=` | 模型调用事件表，可作为 Trace 列表的临时数据源 |

### 4.5 Quasar 映射（追踪）

| 区域 | Quasar 组件 / 说明 |
|------|---------------------|
| 筛选 | **`QSelect`** Agent、`QSelect` 渠道；`clearable`，选项来自 `GET` |
| 列表 | **`QTable`**；**名称**列用 `slot body-cell-name`：`QAvatar` + `div` + 子行 **`QChip`**/`QIcon` 表示网页、ws |
| 令牌列 | 失败态：`QIcon` `name="cancel"` color=`negative` + 文案 `0 / 0` |
| 详情弹窗 | **`QDialog` full-width / maximized** 或 `QCard` 宽度 `min-width`；摘要区 `QList`/`QItem`；错误块 `QBanner` class `bg-negative` text-white |
| Span 树 | **`QTree`**（节点 `children` 映射 spans）或手风琴 **`QExpansionItem`** 递归；节点内 `QBadge` 状态 |
| 导出/复制 | `QBtn` `outline`；导出可生成 JSON 文件触发下载 |

### 4.6 Trace 与 Usage 的数据关系

| 数据 | 表 / 资源 | 说明 |
|------|-----------|------|
| 模型调用事件 | `model_token_usage_events` | 每次模型请求的 token、成本、延迟、错误 |
| 日聚合 | `model_token_usage_daily` | 趋势和统计 |
| Trace 资源 | `monitor_traces` | 可保存复杂调用链、Span 树、诊断 metadata |
| Team Run Steps | `team_run_steps` | 多 Agent 编排中的步骤、耗时、成本 |

首版可以先把 `model_token_usage_events` 作为 Trace 列表的主要来源；当 `monitor_traces.config_json` 内有 spans 时再展示 Span 树。

---

## 5. 路由与导航建议

### 5.1 首版路由

| 路径 | 页面 |
|------|------|
| `/monitor/logs` | `MonitorPage.vue`，内部使用 Tabs 展示 Audit / Events / Traces / Usage / Logs |

首版保留当前 aranea 路由结构，避免侧栏和面包屑一次性扩散。Tab 状态建议同步到 query：`/monitor/logs?tab=audit`，便于刷新后保留当前视图。

### 5.2 后续可拆分路由

| 路径（建议） | 页面 |
|--------------|------|
| `/monitor/audit` | §2 活动日志 |
| `/monitor/events` | §3 实时事件 |
| `/monitor/usage` | §4.0 Usage 总览 |
| `/monitor/traces` | §4 追踪 |
| `/monitor/logs` | §1 日志 |

侧栏统一归入 **「监控」** 分组，子项：日志、实时事件、活动日志、模型用量、追踪。

**Quasar 侧栏**：父级 **`QDrawer`** + **`QList`** **`QItem`** **`QItemSection`**，子路由用 **`QItem` + `to`** 或 **`QRouteLink`**；当前项 `active-class`。

---

## 6. 统一数据契约与前端状态

### 6.1 API 返回格式

当前 aranea 列表 API 多使用：

```json
{
  "items": []
}
```

监控页前端应统一封装：

| 类型 | 字段 |
|------|------|
| `ListResponse<T>` | `items: T[]` |
| `LoadState` | `idle` / `loading` / `success` / `empty` / `error` |
| `StreamState` | `connecting` / `live` / `paused` / `error` |

### 6.2 前端模块建议

| 文件 | 职责 |
|------|------|
| `src/pages/MonitorPage.vue` | 页面壳、Tab、全局刷新 |
| `src/features/monitor/api.ts` | Audit / Events / Traces / Usage API |
| `src/features/monitor/types.ts` | 统一类型 |
| `src/features/monitor/AuditTable.vue` | 活动日志表格 |
| `src/features/monitor/RealtimeEvents.vue` | SSE 事件流 |
| `src/features/monitor/UsageOverview.vue` | 模型用量总览 |
| `src/features/monitor/TraceList.vue` | Trace 列表与详情 |
| `src/features/monitor/LogStream.vue` | 日志流预留组件 |

### 6.3 数据保留与脱敏

- JSON 详情默认折叠大字段，单字段超过 2,000 字符时显示「展开」。
- 密钥、Token、Authorization、Cookie、API Key 等字段统一用 `******` 脱敏。
- SSE 前端缓冲默认最多 1,000 条事件；Logs 默认最多 5,000 行。

---

## 7. 验收要点

- [ ] 页面进入 `/monitor/logs` 后能正常加载 Audit / Events / Traces / Usage 数据，失败时显示可读错误。
- [ ] 活动日志：表格列与 `/api/v1/monitor/audit` 字段一致；支持刷新、分页、关键字筛选、详情查看。
- [ ] 实时事件：SSE 连接状态清晰；支持暂停、恢复、清除、JSON 详情；`channel` 字段含义与 §术语区分一致。
- [ ] Usage：总览、趋势、Top 模型、Top Agent、最近调用事件能从 `/api/v1/model-usage/*` 加载。
- [ ] 追踪：列表与详情能展示 Token、耗时、状态、错误信息；存在 spans 时展示 Span 树，不存在时显示空态。
- [ ] 日志流：若后端未实现，显示明确空态；若实现，支持开始/停止、级别过滤、关键字过滤、计数。
- [ ] 所有 JSON 详情支持复制，复制成功有 `Notify`。
- [ ] 大量数据场景不明显卡顿：长列表使用分页、虚拟滚动或限制前端缓冲。
- [ ] 敏感字段已脱敏，不展示明文密钥、Token、Cookie。

---

## 8. 参考截图（本地）

产品/UI 对照图已保存在工作区 `assets/` 下，文件名含 `image-*.png`，可与本文各节对照迭代。

---

*文档版本：明确 aranea 监控 MVP、统一 API 路径、Usage/Trace 边界、实时事件状态与验收标准；后端字段以实际 OpenAPI 为准。*
