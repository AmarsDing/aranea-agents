# Overview 指挥中心 — 设计文档

> **当前版本**：V2「异常驱动运维台」（2026-10-22 重构落地）。V1（2026-05-29 Hero/状态面板版）设计已被推翻，原始内容见 git 历史。
> V2 决策依据：6 智能体现状审计（26 项准确性问题，6 大高危簇）+ 6 系统开源大屏调研 + 3 方案评审（Route A「异常驱动运维台」37/39/37.5 胜出，并入 B 的预算燃尽/未定价披露与 C 的口径/时区披露）。

## 1. 页面布局设计（V2：7 分区）

```text
┌──────────────────────────────────────────────────────────────┐
│ ① 裁决横幅（sticky）: ● 运行正常/需要关注/存在严重异常 + 4 栏计数 chips │
│    LIVE · 更新于 HH:mm:ss · [时间范围][Provider] · 🔄           │
├──────────────────────────────────────────────────────────────┤
│ ② 注意力队列（4 栏，永不掩埋）: 激活告警 | 失败运行 | 调用异常 | 积压与审批 │
├──────────────────────────────┬───────────────────────────────┤
│ ③ RED·模型调用（用量事件口径）   │ RED·Run 执行（runner 窗口口径）    │
│   调用数+环比+spark | 调用成功率  │ 执行数|Run合格率|P50|P95 [窗口切换]│
│   | 燃烧速率 $/h               │                                │
├──────────────────────────────┴───────────────────────────────┤
│ ④ 成本与预算: 区间花费+环比 | 预算燃尽条(80/95%变色)+月末预测      │
│    | 未定价 chip | 缓存命中率 chip  → 成本 Top5 条形             │
├──────────────────────────────┬───────────────────────────────┤
│ ⑤ 趋势（≤2 图）: 调用量+失败同图  │ 成本趋势                        │
│    x 轴本地时区渲染，UTC 切日在 caption 披露                      │
├──────────────────────────────────────────────────────────────┤
│ ⑥ 组织与编制（默认折叠 caption）: N 组织·M Team·A/T Agent 启用    │
│    + 展开 → 成功率最弱 Agent Top5 + 静态配置口径声明              │
├──────────────────────────────────────────────────────────────┤
│ ⑦ 新鲜度页脚: 各数据源更新时间 · UTC 切日披露 · 探针快照声明 · 未定价披露│
└──────────────────────────────────────────────────────────────┘
```

### 1.1 设计原则（调研结论固化为规则）

| # | 规则 | 落地 |
|---|------|------|
| P1 | 5 秒测试：第一眼必须是「系统好不好」 | ①裁决横幅只回答一个问题，顶部 sticky |
| P2 | 异常优先，注意力队列永不掩埋 | ②告警/失败/异常/积压 4 栏固定在 RED 指标之前 |
| P3 | KPI 三件套：值 + 环比基线 + sparkline | ③模型调用组「调用数」卡三件套齐全 |
| P4 | 颜色只承载语义（绿/黄/红），不作唯一信息载体 | 所有语义色同时配图标/文字 |
| P5 | 禁仪表盘/3D/彩虹色 | V1 的双仪表盘已删除 |
| P6 | 口径命名：成功率永远带定语 | 「调用成功率」（事件行口径）/「Run 合格率」（runner 窗口口径） |
| P7 | 分位数优于均值 | Run 延迟展示 P50/P95，不展示均值 |
| P8 | 新鲜度可见 | LIVE 指示 + ①更新时刻 + ⑦各数据源新鲜度页脚 |
| P9 | 每卡片 = 下钻门 | 可点击瓷贴带 hover 态与回车可达 |
| P10 | 视图数封顶：≤7 区 | 全页 7 个分区，趋势 ≤2 图 |

### 1.2 各分区契约

**① 裁决横幅（OverviewVerdictBanner）**
- verdict 三态：`critical`（任一分栏有 critical 条目）/ `warning`（任一分栏计数>0）/ `ok`
- 横幅内容：verdict 图标脉冲点 + 标题 + 副标题（待处理总数）+ 4 栏计数 chips + LIVE 圆点 + 更新时间 + 刷新按钮 + filters slot（时间范围/Provider）
- 滞后检测：`stale` = 最新数据源更新时刻距今 >120s（30s ticker 驱动），LIVE 圆点转灰
- 加载失败：横幅上方出现 `q-banner` 错误条（ov-error）+ 重试按钮（usageStore.error 非空即显）

**② 注意力队列（OverviewAttentionQueue）**
- 4 个 lane，每栏最多 3 条 + 「共 N 项」footer；空栏显示「无待处理项」（不伪装成告警色）
- 条目：severity 点 + 标题 + 摘要（2 行截断）+ 时刻；有 `to` 的条目可点击下钻

| lane | 数据源 | severity 规则 | 下钻 |
|------|--------|---------------|------|
| 激活告警 | `ListMonitorAlertRules` × `ListAlertMetrics`，enabled 且 current_value ≥ threshold | 取 rule.severity（critical/high→critical，warning/medium→warning，其余 info） | `/monitor/logs?tab=alerts` |
| 失败运行 | `GetRunnerMetrics(24h)` error_runs 计数 + `ListCronTaskRuns(failure)` 明细 | warning | `/cron` |
| 调用异常 | usage overview `anomalies`，∈{failed,error,timeout} 且排除 cancelled | warning | `/usage/events?status=error` |
| 积压与审批 | `ListTaskDeadLetters(pending)` + `ListDecisionRecords(hitl_approval, outcome∈{'',pending,awaiting})` | 死信 critical / 审批 warning | 死信→`/team`；审批暂无落点（不渲染箭头） |

- 轮询：30s（页面可见时才发请求，`visibilitychange` 唤醒即刷）

**③ RED 指标带（OverviewRedStrip + OverviewStatTile）**
- 模型调用组（usage 窗口）：调用数（+vs 昨日 +sparkline）、调用成功率（+昨日基线 caption）、燃烧速率 $/h（窗口花费÷已过时间）
- Run 执行组（独立 runner 窗口 15m/1h/24h 可切换）：执行数、Run 合格率（≥99% 绿 / ≥95% 黄 / 其余红）、P50、P95
- 空窗诚实态：窗口内 0 调用/0 执行时显示「—」+「窗口内无调用/执行」，不显示会引发误报的 0.0%

**④ 成本与预算（OverviewCostBoard）**
- 区间花费 + vs 昨日环比（成本涨为坏事，delta 反向着色）
- 预算燃尽条：`quota_dashboard.max_utilization_ratio`，≥80% 黄、≥95% 红；月末线性预测；月初（<5 日）预测打「低置信」标
- 未配置预算 → 灰 hint，不伪装 0% 绿
- 未定价 chip：`top_models` 中 tokens>0 且 cost≈0 的模型计数；「全部已定价」时为绿底；有未定价时 chip 可点击下钻 `/models`
- 缓存命中率 chip：P50 口径（与 `llm.cache_hit_ratio_low` 告警同口径，低水位 0.5）；非管理员 403 时静默隐藏（cacheHitDenied）
- 成本 Top5 横向条形：未定价行用斜纹填充 + 「未定价」标注，与成本行可区分

**⑤ 趋势（OverviewTrendBoard）**
- 左：调用量（蓝柱）+ 失败（红柱）同图，粒度随窗口（今日→小时，其余→天）
- 右：成本趋势（金色柱）
- x 轴只标首尾，本地时区渲染；UTC 切月在 caption 披露「后端按 UTC 自然日切分」
- CSS 柱状图（无 echarts 依赖）；零值柱高 0，非零最低 2% 可见高度

**⑥ 组织与编制（OverviewOrgBoard）**
- 默认折叠：caption 一行「N 组织 · M Team · A/T Agent 启用」+「静态配置口径，不反映运行态」声明
- 展开：调用成功率最弱 Agent Top5 表（名称/调用/成功率，阈值同 Run 合格率）+ 全部 Agent 入口

**⑦ 新鲜度页脚（OverviewFreshnessFooter）**
- 各数据源最新更新时刻 + 三条诚实披露：UTC 切日 / Provider 状态为目录快照非实时探测 / 未定价不计成本（有未定价时才出现）

## 2. 数据流设计

```
Page(OverviewPage.vue 只做模板拼装与路由下钻，script ≤200 行)
 ├─ features/usage/useOverviewPage.ts（窗口=range/provider）
 ├─ features/usage/useOverviewView.ts（view-model：窗口数据+注意力队列 → 7 分区展示绑定；
 │     含 30s ticker 驱动的滞后检测，最新数据源更新距今 >120s 判定 stale）

 │   ├─ stores/usage        → overview/runner 无关用量（range/yesterday/month/trends/top_models/top_agents/anomalies/quota_dashboard/cacheHit）
 │   ├─ stores/monitor      → runnerMetrics（runner 窗口独立）
 │   ├─ stores/platform     → providerModels（Provider 筛选选项 + 编制口径组织树）
 │   └─ features/agents|teams api（limit=1 取 total 的编制计数）
 │       ※ TECH-DEBT: 直调 api 未过 store，后端 P1 org runtime-summary 上线后切换
 ├─ features/usage/useAttentionBoard.ts（30s 轮询，可见性守卫）
 │   ├─ features/monitor/api: listMonitorAlertRules / listAlertMetrics / getRunnerMetrics
 │   ├─ features/teams/api: listTaskDeadLetters
 │   ├─ features/decision/api: listDecisionRecords
 │   └─ features/cron/api: listCronTaskRuns
 │       ※ TECH-DEBT: 后端 P1 attention-summary 聚合端点上线后切换
 └─ components/overview/*（props/emits only，禁 import store/api；格式化仅 moneyFormat + ovTime）
```

数据流铁律保持：api.ts → Store → composable → Page → Component(props/emits)。两处跨域直读均有 `TECH-DEBT` 注释并登记 P1 后端任务（见 §7）。

## 3. 组件分层（components/overview/）

| 组件 | 职责 | 关键 props |
|------|------|-----------|
| `OverviewSparkline.vue` | 纯 SVG 折线（归一化，全等画中线） | `points: number[]` |
| `OverviewStatTile.vue` | KPI 瓷贴（label+值+delta chip+caption+spark） | `label/value/caption/delta/tone/spark/clickable` |
| `OverviewVerdictBanner.vue` | ①横幅 + filters slot | `verdict/headline/subline/chips/updatedAt/loading` |
| `OverviewAttentionQueue.vue` | ②四栏队列 | `lanes: AttentionLaneState[]`，emit `drill(item)` |
| `OverviewRedStrip.vue` | ③双 RED 组 + runner 窗口切换 | `modelTiles/runTiles/runWindow(runWindowOptions)` |
| `OverviewCostBoard.vue` | ④成本/预算/未定价/缓存/Top5 条形 | `costValueUsd/costDelta/budget/unpricedCount/topBars/cache` |
| `OverviewTrendBoard.vue` | ⑤双 CSS 柱状图 | `labels/calls/failures/costsUsd/granularity` |
| `OverviewOrgBoard.vue` | ⑥折叠编制 + 最弱 Agent 表 | `orgs/teams/agentsTotal/agentsEnabled/worstAgents` |
| `OverviewFreshnessFooter.vue` | ⑦页脚披露 | `sources/notes` |
| `ovTypes.ts` / `ovTime.ts` | 共享展示类型（SFC 不能 export type，故独立） / 绝对时间格式化 | — |

Page（`pages/OverviewPage.vue`，script ≤200 行）只负责路由跳转（`/usage/events`、`/models`、`/cron`、`/monitor/logs?tab=alerts`、`/team`、`/agents`）；全部展示绑定由 `features/usage/useOverviewView.ts` 组装。

## 4. CSS 设计

- 类名空间 `ov-*`（与 V1 的 `overview-*` 完全隔离）；页面/分区样式集中在 `css/app-global.sass` 的「概览页 V2」区段
- 仅依赖语义 token：`--color-success|--color-warning|--color-danger|--color-info(--soft)`、`--color-text-heading|--color-text-secondary`、`--glass-surface|--glass-border`、`--color-accent(--blue)`；昼夜主题由 `body.body--dark` 上的 token 覆盖自动切换，无手写暗色变体
- `overview-panel`/`overview-stat-card` 等 V1 类仍被 monitor/self-improvement/skills 组件引用 → 以「共享兼容层」区段保留在 app-global.sass（迁移后删除）
- 响应式：注意力队列 4→2→1 列；RED 组 2→1 列；成本格 4→2→1 列

## 5. 与现有组件的关系

V1 组件已全部删除：`CommandCenterHero/QuickActions/StatusPanels`、`StatusPanel×4`、`OverviewMonitorQuickLinks/ProviderHealth/RunnerMetrics`、`UsageMetricCards/AnomalyList/FallbackEvents/CacheHitRatio/InefficientModels/TopModels/TopAgents/TrendChart/ModelCostPie/ProviderCostPie/TokenComposition`、`AllModelsBreakdownTable`、`TokenTrendDialog`、`allModelsBreakdownTableUi.ts`、`usageAnomalyTableUi.ts`、`usageGranularityLinkage.ts`、`usageDonutOption.ts`、`usageBreakdownSlices.ts`、`useAllModelsBreakdown.ts`。

保留并复用：`moneyFormat.ts`（格式化）、`pricingWarning.ts`（未定价判定）、`usageTableUi.ts`（UsageEventsPage）、`usageEcharts.ts`/`useUsageChart.ts`/`usageTrendMetrics.ts`（chat/evaluation/platform/skills 图表）、`useProviderTrendDialog.ts`（platform）。

## 6. 准确性口径（审计结论固化为契约）

| 编号 | V1 问题 | V2 处置 |
|------|---------|---------|
| ACC-1 | 「今日/昨日」UTC 自然日与本地日错位（+8h 下午看「今日」居多半天） | 页脚显式披露；范围筛选保留 UTC 后端口径不重采样 |
| ACC-2 | 三种成功率口径混用（事件行 / runner 窗口 / 配额化） | 分组命名展示，禁止裸「成功率」 |
| ACC-3 | 两个「今日调用」数字不一致（KV 卡 vs 会话面板） | 只保留一个「调用数」入口 |
| ACC-4 | 未定价模型显示 $0.00 被当免费 | 未定价 chip + Top5 斜纹行 + 页脚披露 |
| ACC-5 | Provider「健康」= 目录 status 快照 | 不再进 RED/横幅；仅在页脚披露口径 |
| ACC-6 | 「活跃 Agent」= 配置开关而非运行态 | 编制区显式「静态配置口径」声明 |
| ACC-7 | 双仪表盘冗余读数 | 删除仪表盘，改 KPI 三件套 |
| ACC-8 | 0.0% 空态看似告警 | 空窗显示「—」+ 无调用披露 |

## 7. P1 后端依赖（当前前端诚实降级）

| 端点 | 解决的降级 |
|------|-----------|
| `GET /v1/monitor/attention-summary` | 注意力队列 4 路直读 → 单聚合端点；useAttentionBoard 的 TECH-DEBT 切换 |
| 告警确认（ack）API | 注意力队列只读，无确认/静默操作 |
| overview `tz_offset_minutes` 参数 | UTC 切日披露 → 可选本地日切分 |
| overview `unpriced_call_count` | 未定价目前只看 top_models 行 |
| `GET /v1/cron/health` | cron 失败只来自 cron-task-runs 明细 |
| org runtime-summary | 编制区只有配置态计数，无运行态 |
| provider error_class | Provider 健康仅目录快照，无错误分类 |
| runner `points[]` | Run 组无趋势 spark |

## 8. 性能考虑

- 趋势/Top 数据复用一次 `loadOverview`；注意力队列 30s 独立轮询（tab 隐藏即停）
- WCAG：语义色均带图标/文字双编码；可点击瓷贴支持键盘回车；`aria-live` 标注注意力队列
- 每分区为独立 flex/grid 区块，首屏 ①②③ 即够决策，⑤⑥ 后置
