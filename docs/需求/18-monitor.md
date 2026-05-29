# 监控（Monitor）

本文档描述控制台 **监控** 相关页面的信息架构、控件与数据契约，与 **运行日志**、**审计**、**实时事件**、**LLM / 模型调用追踪** 对齐。**前端实现采用 Quasar Framework（Vue 3）**：路由 `vue-router`，布局 **`QLayout` / `QPageContainer` / `QPage`**，主题跟随控制台统一主题（`Quasar Dark Plugin` / `$q.dark`）。

> 2026-05-19 更新：监控实时事件口径统一为 WebSocket + EventBus。历史 `team-run-events` SSE / 独立 SSE Broker 已从当前主链路移除，后续不得作为新实现入口。

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
| Team / Agent 运行时现在正在发生什么？ | 实时事件 Events | `/v1/ws`（`team_run_*`、`alert.fired` 等） | ✅ 基础已实现；Phase 1d 收窄 Chat `runner.completion` 列表展示 |
| 刚才那轮对话是否成功结束？耗时/Token 多少？ | **Runs（Traces Tab）** | `model_token_usage_events`（`recordTurnUsage`） | ✅ 已实现；Phase 1d 增强关联与跳转 |
| Runner 窗口内成功率/错误率？ | Usage → **Runner 指标** | `GET /v1/monitor/runner-metrics` | ✅ 已实现；点击下钻 Runs；Latency P50/P95/P99 |
| 哪些模型调用慢、失败、成本高？ | Usage 总览 + **Runs** | `model_token_usage_events` 聚合 + 单次运行列表 | ✅ 已实现 |
| 错误率超阈如何告警？ | **Alerts** 规则 | `monitor_alert_rules` + `alert.fired` + Webhook/Channel | ✅ 已实现；冷却持久化 + 评估批量化 |
| 某次对话为什么失败？ | **Runs 详情**（原 Trace 详情） | Summary + Flow（trace_id）+ Waterfall + Span | ✅ 已实现 |
| 某次对话/Team 执行卡在哪一步？ | Logs → **流程日志** | WS `flow_log`（`TraceEmitter`） | ✅ 已实现；文件落盘 + gzip |
| Gateway / 插件底层 stderr 是否正常？ | Logs → **进程日志** | WS `log` + `enable_log` | ✅ 已实现 |
| 能否自动诊断问题根因？ | **AI 诊断** | `GenerateDiagnosticBundle` + `RootCauseEngine` | ✅ 已实现 |

### 0.2 模块实现状态

| 模块 | 状态 | 说明 |
|------|------|------|
| Audit | ✅ 已实现 | 表格、刷新、分页（limit/offset）、事件类型/实体类型/操作者/关键字筛选、详情弹窗、扩展字段（actor/ip/user_agent/severity/metadata_json） |
| Alerts | ✅ 已实现 | `MonitorAlertRules`：规则 CRUD、`runner.error_rate`、Webhook/Channel 出站、`cooldown_minutes`；MON-OPT-02 冷却持久化 + firing 状态机；MON-OPT-03 评估批量化 + RingBuffer；MON-OPT-06 告警注册表 |
| Events | ✅ 已实现 | WS 实时流 + `alert.fired`；**方案 C**：已关联 Runs 的 Chat `runner.completion` 默认不出现在主列表 |
| Runs（路由 Tab 名 `traces`，列表标题 Runs） | ✅ 单次运行真相源 | `ListUsageEvents` 列表 + 详情（Flow/Waterfall/Span/JSONL 导出）；「打开会话」+ `usage_event_id` 深链；MON-OPT-05 Trace 写入回路 + 历史回填 |
| Usage | ✅ 已实现 | `MonitorRunnerMetrics` + `MonitorUsageDashboardLink`（完整大盘在 `/overview`）；Runner 下钻 Traces；Latency P50/P95/P99 |
| Logs | ✅ 已实现 | **二级 Tab**：流程日志（默认连接）+ 进程日志（`process_log_enabled`）；共享一条 WS；流程 Tab 可暂停/清除；进程 Tab 切离丢弃入站、切回恢复；LOG-01 文件落盘 + gzip + 30 天清理 |
| AI 诊断 | ✅ 已实现 | DIAG-01 诊断包 + DIAG-02 根因分析引擎（5 条内置规则 + 置信度评分） |

### 0.3 非目标

- **用量/成本大盘**在独立概览 `/overview`，见 [18 monitor-dashboard.md](./18%20monitor-dashboard.md)（非本页）。
- 不在监控页修改 Agent、Channel、Provider 配置；只跳转到对应管理页。
- 不存储或展示完整用户隐私内容，日志与事件 payload 默认截断 / 脱敏。
- 不把事件 JSON 里的 `channel: "ws"` 当作业务 `channel.id` 使用。

---

## 1. 日志（Logs）

Monitor **Logs** 一级 Tab 内拆为 **两个二级 Tab**，分别服务不同排障场景（详见 [52-flow-logger.md](./52-flow-logger.md) §2）：

| 二级 Tab | 面向 | Envelope | 默认行为 |
|----------|------|----------|----------|
| **流程日志** | 业务用户 / 产品运维：「这次对话卡在哪？」 | `flow_log` | 进入 Tab 即连接 WS，**无需手动开启** |
| **进程日志** | 开发 / SRE：Gateway、插件 stderr | `log` | **`server.monitor.process_log_enabled`**（默认 `true`）；进入进程 Tab 自动恢复接收；无 UI 开关 |

> 实现状态：✅ 已实现。`LogStreamPanel.vue` + 共享 `useLogStreamHub`（一条 `session_id=*` WS，全局上限 3 连接）。

### 1.1 页面结构（Logs 一级 Tab）

| 区域 | 内容 |
|------|------|
| **二级 Tab** | **流程日志**（默认） \| **进程日志** |
| **流程 Tab 工具行** | 状态 Badge；**暂停/恢复**（仅停本 Tab 缓冲写入）；关键字；级别；**清除** |
| **进程 Tab 工具行** | 状态 Badge；关键字；级别；按 source 筛选；清除（**无**开启/暂停按钮；切换 Tab 自动恢复） |
| **主体** | 等宽深色控制台；流程行展示 `title` + `message` + severity 色条 |

### 1.2 行格式

**流程日志（flow_log）示例：**

```text
12:00:01 [INFO] 调用语言模型 — 模型已返回，开始处理输出流 (3240ms)
12:00:05 [ERROR] 对话超时 — Turn 超过 5 分钟
```

**进程日志（log）示例：**

```text
12:00:01 [WARN][hook] execute failed tool=search error=timeout
```

### 1.3 数据来源与行为

| 项 | 流程日志 | 进程日志 |
|------|----------|----------|
| 传输 | WS `flow_log`，channel=`monitor` | WS `log`；服务端 `process_log_enabled` + 客户端 `enable_log` |
| HTTP 快照 | 无（Phase 2 `ListFlowLogs` 规划） | `GET /v1/monitor/logs` 返回 `enabled`（镜像 config）+ hint |
| 过滤 | trace_id / step_id / title / 关键字 | source / 级别 / 关键字 |
| 缓冲 | 各 Tab 独立，最多 5,000 行 | 同左 |
| 暂停 | 不断 WS，仅停止追加本 Tab 缓冲 | 离开进程 Tab 时 **丢弃** 入站行（不缓冲）；切回 Tab 自动恢复 |

### 1.4 连接状态

| 状态 | UI | 说明 |
|------|-----|------|
| `connecting` | 橙色「连接中」 | WS 握手中 |
| `connected` | 绿色「已连接」 | 收到 WS `connected` 帧，等待数据 |
| `live` | 绿色「实时」 | 已收到至少一条对应类型日志 |
| `paused` | 灰色「已暂停」 | 用户暂停本 Tab |
| `error` | 红色「连接异常」 | WS 错误或 429（全局连接满） |

### 1.5 空态与异常态

| 场景 | 表现 |
|------|------|
| 流程 Tab 无数据 | 「已连接。发起一次对话后可看到流程日志。」 |
| 进程 Tab config 关闭 | 「进程日志已在 config.yaml 中关闭（server.monitor.process_log_enabled: false）。」 |
| 进程 Tab 已暂停（非当前 Tab） | 「已暂停接收（切换到本 Tab 后自动恢复）。」 |
| WS 429 | 提示「全局监控连接已达上限(3)，请关闭其他 Monitor/Chat 页签」 |
| 大量日志 | 各 Tab 缓冲最多 5,000 行 |

### 1.6 进程日志配置

| 项 | 说明 |
|----|------|
| 配置项 | `configs/config.yaml` → `server.monitor.process_log_enabled` |
| 默认值 | `true`（省略 `monitor` 块时同 true） |
| HTTP | `GET /v1/monitor/logs` 的 `enabled` 字段镜像该配置 |
| WS | globalMode（`session_id=*`）连接时，若 config 为 true 则自动 `logEnabled`；客户端 `enable_log(true)` 在 config 为 false 时被服务端忽略 |
| UI | 无「开启进程日志」按钮；进程 Tab 切离时暂停（丢弃入站），切回自动恢复 |

### 1.7 Quasar 映射

| 区域 | Quasar 组件 |
|------|-------------|
| 二级 Tab | `QTabs` + `QTabPanels`（嵌在 Logs 一级 Tab 内） |
| 状态 | `QBadge` |
| 工具行 | `QInput`、`QBtnToggle`、`QBtn` |
| 日志主体 | `QCard` + 等宽行列表（流程行带 severity class） |

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

展示 **Team 编排实时动态**、**告警触发** 及 **无 Runs 记录时的运行结束降级提示**；**不**作为 Chat 单次对话排障的主入口（见 §4 Runs）。

> 实现状态：✅ 已实现（`RealtimeEvents.vue` + `ListMonitorEvents` + WS + `runCorrelation.ts`）。
> **方案 C**（Phase 1d ✅）：Events 与 Runs 分工、correlation 落库、统一 Runs 详情；见 [18 monitor.design.md §九](./18%20monitor.design.md#九方案-cruns--events--runnercompletion) · [changelog](../changelog/2026-05-20-Monitor-Phase1d-PlanC.md)。

### 3.0 产品定位（方案 C：Runs 列表 + Events 实时流）

| Tab | 回答的问题 | 不应承担 |
|-----|------------|----------|
| **Runs（Traces）** | 单次运行：成败、Token、延迟、Provider/Model、Flow/Span | Team 实时步骤流、管理审计 |
| **Events** | Team WS（`team_run_*`）、`alert.fired`、**无 Usage 行时的** completion 降级 | Chat 完整排障详情（与 Runs 重复） |
| **Logs → 流程** | 卡在哪一步 | 聚合错误率 |
| **Usage** | 窗口内统计、Top 排行 | 单次运行逐步日志 |

**`runner.completion`（后端仍落库）**：用于 `runner.error_rate` 告警、`RunnerMetricsPanel` 计数、Memory Worker；Chat 主路径已有 `recordTurnUsage` → Runs 行，**Events 主列表默认隐藏** persisted `runner.completion`（有 `usage_event_id` / `trace_id` 关联时提示「在 Runs 中查看」）。

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
| 任务 | `run.*`、`team_run.*`（不含 `runner.completion`） |
| 消息 | `message.*`、`chat.*` |
| Agent | `agent.*`、`agent_link.*` |
| 工具 | `tool.*`、含 `step` 的 team 步骤 |
| 系统 | `system.*`、`runtime.*`、`alert.*`、`runner.completion`（仅降级卡片） |

### 3.3 列表过滤（`runner.completion`）

| 场景 | Events 列表行为 |
|------|-----------------|
| Chat 且 metadata 含 `usage_event_id` 或可对上 Runs 行 | **不展示** persisted `runner.completion`（避免与 Runs 重复） |
| Team/Cron/无 Usage 行的 completion | 展示降级卡片：「运行已结束（无用量记录）」+ 会话链接 |
| 有 `usage_event_id` 的降级场景 | 主操作：**在 Runs 中查看**（打开 Trace 详情） |
| WS `runner_completion` | 可与 persisted 去重；不单独做第二套详情弹窗 |

### 3.4 详情弹窗（Events 非 Runs 事件）

- **Team / 告警 / 降级 completion**：保留 JSON 详情 + 复制；completion 降级卡片可增加「在 Runs 中查看」。
- **Chat 完整排障**：不在 Events 建平行详情；统一使用 **§4 Runs 详情**（已有 Summary / Flow / Waterfall / Span）。

### 3.5 用户故事与验收（Phase 1d · 方案 C）

| ID | 用户故事 | 验收标准 |
|----|----------|----------|
| RUN-01 | 作为运维，Chat 结束后在 Runs 看到这次运行 | 发一条 Chat 后 Runs（Traces）列表出现一行，含 Agent/Token/延迟/status |
| RUN-02 | 作为运维，从 Runs 详情排障 | 详情含 Flow（trace_id 过滤）、Waterfall、Span；可 **打开会话** |
| RUN-03 | 作为运维，Events 不重复刷屏 | Events 主列表不出现与 Runs 重复的 Chat `runner.completion` |
| RUN-04 | 作为运维，告警与 Runner 指标仍准确 | `runner.error_rate`、`RunnerMetricsPanel` 仍基于 `monitor_events` 计数 |
| RUN-05 | 作为运维，无 Usage 时仍有信号 | 无 Runs 行时 Events 显示降级 completion + 会话链接 |
| RUN-06 | 数据质量 | 同一 `session_id`+`invocation_id` 不重复插入 completion；metadata 含 `trace_id` / `usage_event_id` |

### 3.6 实时连接状态

| 状态 | UI | 说明 |
|------|-----|------|
| connecting | 橙色「连接中」 | WS 握手中 |
| connected | 绿色「已连接」 | 已握手，尚无新事件 |
| live | 绿色「实时」 | 已收到 WS 运行时事件 |
| paused | 灰色「已暂停」 | 用户暂停 |
| error | 红色「连接异常」 | WS 失败或全局连接满（3） |

---

## 4. Runs（单次运行 · UI 标签 Traces）与 Usage 总览

**方案 C 真相源**：单次 Agent/Team **运行**以本 Tab 为主（数据源 `model_token_usage_events`，`trpc_turn` → `recordTurnUsage`）。与 Events 中 `runner.completion` 的关系为 **关联键 + 告警计数**，非平行详情页。

> 实现状态：✅ 已实现。
> - Usage Tab：Runner + 跳转概览；用量大盘见 `/overview`（[18 monitor-dashboard.md](./18%20monitor-dashboard.md)）
> - Runs 列表：`TraceList.vue` + `ListUsageEvents`（即 `/v1/usage/events`）
> - Phase 1d：Runner 指标下钻、详情「打开会话」、`runner.completion` metadata 关联

### 4.0 Runs 与 Events 分工（方案 C）

| 维度 | Runs（Traces） | Events |
|------|----------------|--------|
| 主数据 | `model_token_usage_events` | WS + `monitor_events`（告警、降级 completion） |
| Chat 排障 | ✅ 默认入口 | ❌ 不重复列表 |
| 详情壳 | `TraceList` 最大化对话框（Flow/Waterfall/Span） | 仅非 Runs 类事件 |
| `runner.completion` | 通过 `trace_id` / `usage_event_id` 关联 | 落库但不主显 |

**默认用户路径**：发起对话 → Monitor → **Traces（Runs）** → 打开行详情 → Flow / Waterfall。

### 4.0 Usage Tab（Runner + 跳转概览）

**Usage Tab** 自上而下：`MonitorRunnerMetrics` → `MonitorUsageDashboardLink`。

| 区域 | 内容 |
|------|------|
| **Runner 指标** | 滑动窗口（15 分～24 小时）；`useRunnerMetrics` → Store → `GET /v1/monitor/runner-metrics`；点击下钻 `?tab=traces` |
| **跳转** | 「打开概览」→ `/overview?range=`（与页面顶栏 `filters.range` 一致）；「查看明细」→ `/usage/events` |

完整用量/趋势/Top/占比见 [18 monitor-dashboard.md](./18%20monitor-dashboard.md)（`/overview`）。

### 4.1 Runs 列表（路由 Tab：`traces`）

| 区域 | 内容 |
|------|------|
| **标题** | 「Runs」（侧栏/路由 Tab 标签仍为 Traces） |
| **副标题** | 单次对话运行真相源（Token + Flow / Waterfall / Span） |
| **筛选** | 关键字搜索；数据来自 `listMonitorTraceEvents` → `UsageService.ListUsageEvents` |
| **表格列** | Agent/Provider/Model、令牌 in/out、延迟、成本、错误、时间、详情操作 |

### 4.2 Runs 详情弹窗（最大化对话框）

| 区块 | 内容 |
|------|------|
| **摘要** | Status、Agent、Provider/Model、Tokens、延迟、成本、`trace_id` / `run_id` |
| **操作** | **打开会话**（有 `session_id` 时）；Flow JSONL 导出；复制 JSON |
| **Flow** | `FlowTracePanel`：按 `trace_id` 过滤 WS `flow_log` 缓冲 |
| **Waterfall** | `TraceWaterfall.vue`：`metadata_json.spans` / `turn_spans` |
| **Span 树** | 嵌套 agent → llm_call（有 spans 时） |
| **错误** | 失败时展示 `error_message` |

### 4.3 数据与 API

| 方法 | 说明 |
|------|------|
| GET | `/api/v1/monitor/traces?limit=100&offset=0&agent_id=&provider=&model=&status=` |
| GET | `/api/v1/monitor/traces/:traceId` | 详情 + spans 树 |
| GET | `/api/v1/usage/events?agent_id=&provider=&model=&limit=` | 模型调用事件表 |
| GET | `/api/v1/monitor/runner-metrics?window_minutes=` | Runner 窗口指标（`MonitorService`） |

### 4.5 告警规则（Alerts Tab）

面向运维：配置 `runner.error_rate` 等规则，超阈后写入 `alert.fired` 事件并可选出站通知。

> 实现状态：✅ 已实现。`MonitorAlertRules.vue` + `ListMonitorAlertRules` / `PutMonitorAlertRules`。

| 区域 | 内容 |
|------|------|
| **规则行** | 名称、指标键（如 `runner.error_rate`）、阈值、窗口（分钟）、启用、严重级别 |
| **通知** | Webhook URL；通知 Channel（下拉，来自 Channel 列表） |
| **冷却** | `cooldown_minutes`（默认 60）；同规则冷却期内不重复出站 |
| **操作** | 刷新、保存 |

| API | 说明 |
|-----|------|
| GET | `/api/v1/monitor/alert-rules` |
| PUT | `/api/v1/monitor/alert-rules`（body: `items[]`） |

评估时机：`runner.completion` 落库后 `MonitorUsecase.EvaluateAlerts`；出站见 `internal/service/monitor_notify.go`（`alert.notify`）。

---

## 5. 路由与导航

### 5.1 当前路由

| 路径 | 页面 |
|------|------|
| `/monitor/logs` | `MonitorPage.vue`，内部 **6 Tab**：Usage / Alerts / Audit / Events / Traces / Logs |

Tab 与深链 query（刷新可保留）：

| Query | 说明 |
|-------|------|
| `tab` | `usage` \| `alerts` \| `audit` \| `events` \| `traces` \| `logs`（默认 `usage`） |
| `usage_event_id` | 打开 Runs（Traces）Tab 并高亮/打开对应 usage 行详情 |
| `session` | 由 `useMonitorRunNavigation` 跳转 Chat（`/chat?session=…`） |

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
| `StreamState` | `connecting` / `connected` / `live` / `paused` / `error` |

### 6.2 前端模块

| 文件 | 职责 |
|------|------|
| `pages/MonitorPage.vue` | 页面壳、6 Tab；`tab` / `usage_event_id` query 同步 |
| `components/monitor/MonitorRunnerMetrics.vue` | 容器：`useRunnerMetrics` + `RunnerMetricsPanel` |
| `components/monitor/RunnerMetricsPanel.vue` | Runner 指标纯展示（props/emits） |
| `components/monitor/MonitorUsageDashboardLink.vue` | 跳转 `/overview`、`/usage/events` |
| `components/monitor/MonitorAlertRules.vue` | 告警规则编辑与保存 |
| `features/monitor/useRunnerMetrics.ts` | Runner 指标 composable（调 Store） |
| `components/monitor/AuditTable.vue` | 活动日志表格（筛选 + 分页） |
| `components/monitor/RealtimeEvents.vue` | WS 事件流 + 方案 C completion 过滤 |
| `components/monitor/TraceList.vue` | Runs 列表与详情（Flow/Waterfall/Span） |
| `components/monitor/TraceWaterfall.vue` | 详情瀑布图 |
| `components/monitor/FlowTracePanel.vue` | 详情流程 Tab（`flow_log` 过滤） |
| `components/monitor/FlowLogExportButton.vue` | Flow JSONL 导出 |
| `components/monitor/LogStreamPanel.vue` | Logs 二级 Tab 容器 + 共享 WS Hub |
| `components/monitor/FlowLogStream.vue` | 流程日志流 |
| `components/monitor/ProcessLogStream.vue` | 进程日志流 |
| `features/monitor/api.ts` | Monitor API（audit/events/traces/logs/alerts/runner-metrics） |
| `features/monitor/runCorrelation.ts` | 方案 C：completion 过滤与 Runs 关联 |
| `features/monitor/useMonitorRunNavigation.ts` | 会话 / Runs Tab / 详情深链 |
| `features/monitor/useLogStreamHub.ts` | 共享 Logs WS Hub |
| `features/monitor/types.ts` | 类型定义（含 AuditQuery/PaginatedResult/MonitorAlertRule） |
| `features/monitor/utils.ts` | 格式化工具 |
| `features/usage/api.ts` | Usage API |
| `features/usage/types.ts` | Usage 类型 |
| `stores/monitor/index.ts` | Pinia Store |

### 6.3 数据保留与脱敏

- JSON 详情默认折叠大字段，单字段超过 2,000 字符时显示「展开」。
- 密钥、Token、Authorization、Cookie、API Key 等字段统一用 `******` 脱敏。
- WS 前端缓冲默认最多 1,000 条事件；Logs 默认最多 5,000 行。

---

## 7. 验收要点

- [x] 页面进入 `/monitor/logs` 后能正常加载 Audit / Events / Traces / Usage 数据，失败时显示可读错误。
- [x] 活动日志：表格列与 API 字段一致；支持刷新、分页、事件类型/实体类型/操作者/关键字筛选、详情查看。
- [x] 实时事件：WS 连接状态清晰；支持暂停、恢复、清除、JSON 详情；分类 Tab。
- [x] Usage：Runner 指标 + 总览卡、Top 模型、Top Agent、最近异常能从 `/api/v1/usage/*` 与 `/api/v1/monitor/runner-metrics` 加载。
- [x] Alerts：规则可加载/保存；超阈产生 `alert.fired`；Webhook/Channel 出站（冷却生效）。
- [x] 方案 C（Phase 1d）：Runs 主排障；Events 不重复已关联 Chat `runner.completion`；Runs 详情可打开会话。
- [x] 追踪：列表与详情能展示 Token、耗时、状态、错误信息；存在 spans 时展示 Span 树。
- [x] 日志流：流程/进程二级 Tab 独立缓冲；流程默认连接；进程 `enable_log` 开关；级别/关键字过滤；连接状态含 `connected`。
- [x] 所有 JSON 详情支持复制，复制成功有 `Notify`。
- [x] 大量数据场景不明显卡顿：长列表使用分页、虚拟滚动或限制前端缓冲。
- [x] 敏感字段已脱敏，不展示明文密钥、Token、Cookie。

---

*文档版本：2026-05-29 — 对齐代码：6 Tab（含 Alerts）、Runner 指标、方案 C Phase 1d ✅、Logs 流程/进程二级 Tab、MON-OPT-01~06 ✅、LOG-01/TRACE-01/DIAG-01/02 ✅、Latency P50/P95/P99 ✅、LOG-03 P0/P1/P2 ✅、REDLINE ✅、QUALITY ✅。实现差距见 [18-monitor-development.md](./18-monitor-development.md)。*
