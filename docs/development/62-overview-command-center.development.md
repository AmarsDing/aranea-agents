# Overview 指挥中心 — 开发计划

> **Goal:** 将 OverviewPage 从用量仪表盘升级为智能体指挥中心，新增 Hero 欢迎区、实时状态面板行、快捷入口区，保留现有用量分析功能。

> **Architecture:** 前端组件分层：Page → Composable → Store → API。新增 6 个展示组件 + 升级 1 个 composable，不新增后端 API。数据复用现有 Store（usage/platform/monitor）+ 直接 API 调用（listAgentsPaged/listTeams）。

> **Tech Stack:** Vue 3 + Quasar + Pinia + TypeScript

> **状态**：✅ 已完成开发（2026-05-29）

---

## 1. 模块定位

Overview 指挥中心是前端首页 `/overview` 的展示层升级，将原用量仪表盘升级为融合驾驶舱实时感 + 数据仪表盘分析力 + 工作台快捷入口的智能体指挥中心。本模块为**纯前端变更**，不涉及后端 API 新增或 Proto 变更。

## 2. 现状评估

### 2.1 实现调整说明

| 需求 ID | 原需求 | 实际实现 | 调整原因 |
|---------|--------|----------|----------|
| FR-1.2 | 显示系统运行时长（Uptime） | 暂缓实现 | 后端 `RunnerMetricsSummary` 类型无 `uptime_seconds` 字段（见 `web/src/features/monitor/types.ts:118-128`） |
| FR-2.4 | 系统负载面板（CPU/内存使用率） | 改为 Runner 运行面板（成功率/错误率/运行数） | 后端 `RunnerMetricsSummary` 无 `cpu_percent`/`memory_percent` 字段，改为复用现有 `total_runs`/`error_runs`/`success_rate`/`error_rate` 字段 |
| FR-1.3 | 3 个核心指标 | 扩展为 6 个核心指标 | 实现时增加 Provider 数、公司数、团队数，提升首屏信息密度 |
| FR-1.4/1.5 | Hero 内置"开始对话"/"查看告警"按钮 | 改为 `actions` slot 由父组件注入 | 解耦 Hero 与具体操作，提升复用性 |

### 2.2 已完成能力

- ✅ Hero 欢迎区：6 个核心指标卡片（可点击跳转）+ 时间显示 + i18n
- ✅ 快捷入口区：4 个卡片（新建对话/创建 Agent/定时任务/系统设置）
- ✅ 实时状态面板行：4 个面板（Agent/会话/Provider/Runner）
- ✅ 现有用量分析功能完整保留
- ✅ Dark mode 支持
- ✅ 响应式布局

## 3. 代码锚点

### 3.1 新增组件

| 文件路径 | 职责 |
|---------|------|
| `web/src/components/usage/CommandCenterHero.vue` | Hero 欢迎区（6 指标 + actions slot） |
| `web/src/components/usage/CommandCenterQuickActions.vue` | 快捷入口区（4 卡片） |
| `web/src/components/usage/CommandCenterStatusPanels.vue` | 状态面板行容器 |
| `web/src/components/usage/StatusPanelAgent.vue` | Agent 状态面板（环形进度条） |
| `web/src/components/usage/StatusPanelSession.vue` | 会话活跃面板（大数字 + Sparkline） |
| `web/src/components/usage/StatusPanelProvider.vue` | Provider 健康面板（状态灯） |
| `web/src/components/usage/StatusPanelRunner.vue` | Runner 运行面板（成功率/错误率仪表盘） |

### 3.2 修改文件

| 文件路径 | 变更内容 |
|---------|----------|
| `web/src/pages/OverviewPage.vue` | 集成新组件，移除 OverviewPageHero 引用 |
| `web/src/features/usage/useOverviewPage.ts` | 新增 agentStats/providerCount/categoryCount/teamCount/username/providerHealthSummary/sessionActiveCount/sessionSparkline/runnerStats |
| `web/src/i18n/locales/zh-CN.ts` | 新增 overviewPage 命名空间下的 statActiveAgents/statProviders/statCategories/statTeams/statTodayChats/statTodayTokens/btnAlerts/btnDetails 键 |
| `web/src/i18n/locales/en-US.ts` | 同步新增 i18n 键 |

### 3.3 保留未引用文件

| 文件路径 | 说明 |
|---------|------|
| `web/src/components/usage/OverviewPageHero.vue` | 原 Hero 组件，被 CommandCenterHero 替代，文件保留但不再引用 |

### 3.4 关联依赖文件

| 文件路径 | 依赖关系 |
|---------|----------|
| `web/src/features/agents/api.ts` | `listAgentsPaged` 函数（Hero activeAgentCount 数据源） |
| `web/src/features/teams/api.ts` | `listTeams` 函数（Hero teamCount 数据源） |
| `web/src/features/monitor/types.ts` | `RunnerMetricsSummary` 类型（Runner 面板数据源） |
| `web/src/stores/usage/index.ts` | `useUsageStore`（overview/today 数据） |
| `web/src/stores/platform/index.ts` | `usePlatformStore`（providerModels/taxonomyTree） |
| `web/src/stores/monitor/index.ts` | `useMonitorStore`（runnerMetrics） |
| `web/src/features/usage/moneyFormat.ts` | `formatCount` 函数（Token 数格式化） |
| `web/src/router/routes.ts` | 路由定义（`/chat`、`/agents`、`/cron`、`/settings`、`/models`、`/settings/organization`、`/team`、`/usage/events`） |

## 4. Phase 划分与任务清单

### Phase 1: Hero 欢迎区（✅ 已完成）

- [x] **Task 1: CommandCenterHero 组件**
  - 创建 `web/src/components/usage/CommandCenterHero.vue`
  - 实现 6 个指标卡片（router-link 可点击）
  - 实现 actions slot
  - 实现 i18n 集成
  - 实现时间显示（60s 定时刷新）

### Phase 2: 状态面板行（✅ 已完成）

- [x] **Task 2: StatusPanelAgent 组件**
  - 创建 `web/src/components/usage/StatusPanelAgent.vue`
  - 实现环形进度条（SVG）+ 在线/离线统计
- [x] **Task 3: StatusPanelSession 组件**
  - 创建 `web/src/components/usage/StatusPanelSession.vue`
  - 实现大数字 + Sparkline（SVG polyline）
- [x] **Task 4: StatusPanelProvider 组件**
  - 创建 `web/src/components/usage/StatusPanelProvider.vue`
  - 实现活跃/降级/总计统计 + 状态灯
- [x] **Task 5: StatusPanelRunner 组件**（原计划为 StatusPanelSystem，因后端字段缺失调整）
  - 创建 `web/src/components/usage/StatusPanelRunner.vue`
  - 实现成功率/错误率仪表盘 + 运行数摘要
- [x] **Task 6: CommandCenterStatusPanels 容器组件**
  - 创建 `web/src/components/usage/CommandCenterStatusPanels.vue`
  - 组合 4 个子面板，响应式 grid 布局

### Phase 3: 快捷入口区（✅ 已完成）

- [x] **Task 7: CommandCenterQuickActions 组件**
  - 创建 `web/src/components/usage/CommandCenterQuickActions.vue`
  - 实现 4 个快捷入口卡片（router-link）

### Phase 4: Composable 升级与页面集成（✅ 已完成）

- [x] **Task 8: 升级 useOverviewPage composable**
  - 修改 `web/src/features/usage/useOverviewPage.ts`
  - 新增 agentStats（通过 listAgentsPaged 获取 total）
  - 新增 providerCount（去重统计）
  - 新增 categoryCount（通过 loadTaxonomyTree）
  - 新增 teamCount（通过 listTeams）
  - 新增 username（从 localStorage 解析）
  - 新增 providerHealthSummary/sessionActiveCount/sessionSparkline
  - 新增 runnerStats（从 runnerMetrics 转换百分比）
- [x] **Task 9: 重构 OverviewPage 集成新组件**
  - 修改 `web/src/pages/OverviewPage.vue`
  - 替换 Hero 为 CommandCenterHero
  - 新增 CommandCenterQuickActions（位于 Hero 与 StatusPanels 之间）
  - 新增 CommandCenterStatusPanels
  - 保留现有筛选栏与用量分析区
  - 实现 scrollToAlerts 与 onMetricNavigate

### Phase 5: 验证与收尾（✅ 已完成）

- [x] **Task 10: 验证与收尾**
  - 前端 lint 通过
  - 前端 build 通过
  - 手动验证 6 项验收标准
  - 文档同步更新

## 5. 验收标准

| 验收 ID | 验收标准 | 状态 |
|---------|----------|------|
| AC-1 | 访问 `/overview`，Hero 欢迎区显示用户名、时间、6 个核心指标 | ✅ |
| AC-2 | 4 个状态面板正确展示数据（Agent/会话/Provider/Runner） | ✅ |
| AC-3 | 快捷入口可点击跳转到 `/chat`、`/agents`、`/cron`、`/settings` | ✅ |
| AC-4 | 筛选栏和用量分析区功能不受影响 | ✅ |
| AC-5 | 切换 Dark mode，所有新组件样式正确 | ✅ |
| AC-6 | 缩小浏览器窗口，响应式布局正确（6/4 列 → 3/2 列 → 2/1 列） | ✅ |

## 6. 改动文件清单

### 6.1 新增文件（7 个）

```
web/src/components/usage/CommandCenterHero.vue
web/src/components/usage/CommandCenterQuickActions.vue
web/src/components/usage/CommandCenterStatusPanels.vue
web/src/components/usage/StatusPanelAgent.vue
web/src/components/usage/StatusPanelSession.vue
web/src/components/usage/StatusPanelProvider.vue
web/src/components/usage/StatusPanelRunner.vue
```

### 6.2 修改文件（4 个）

```
web/src/pages/OverviewPage.vue
web/src/features/usage/useOverviewPage.ts
web/src/i18n/locales/zh-CN.ts
web/src/i18n/locales/en-US.ts
```

## 7. 差距与优化（未来迭代）

| 编号 | 差距/优化项 | 优先级 | 说明 |
|------|------------|--------|------|
| OPT-1 | FR-1.2 系统运行时长（Uptime） | P2 | 需后端 `RunnerMetricsSummary` 新增 `uptime_seconds` 字段后实现 |
| OPT-2 | FR-2.4 系统负载面板（CPU/内存） | P2 | 需后端新增系统指标 API 后实现，当前以 Runner 面板替代 |
| OPT-3 | WebSocket 实时推送 | P3 | 当前依赖 Store 数据复用，未来可接入 WebSocket 提升实时性 |
| OPT-4 | OverviewPageHero.vue 清理 | P3 | 原 Hero 组件已不再引用，可考虑删除以减少死代码 |

---

# V2 重构：异常驱动运维台（2026-10-22 ✅ 已完成）

> **Goal:** 推翻 V1（Hero/状态面板/仪表盘版）全面重设计。依据：26 项准确性审计（UTC 切日、三成功率口径、双今日调用、未定价显示 $0.00、假 Provider 健康、配置态「活跃 Agent」）+ 6 开源监控大屏调研 + 3 方案评审（Route A 37/39/37.5 胜出）。
> **状态**：✅ 已完成（lint / 2391 单测 / 生产构建三绿）

## V2.1 任务清单

- [x] **P1 数据层**：`features/decision/{types,api}.ts` 新增；`services/index.ts` 接 `createDecisionRecordServiceClient`；`features/usage/types.ts` 追加 Attention 类型
- [x] **P2 composables**：`useAttentionBoard.ts`（4 栏注意力队列，30s 可见性轮询）；`useOverviewPage.ts` 重写为 V2（窗口派生/delta/燃烧速率/未定价/Top5/最弱 Agent/编制计数）
- [x] **P3 组件**：`components/overview/` 9 组件 + `ovTypes.ts`/`ovTime.ts`（props/emits only）
- [x] **P4 页面**：`pages/OverviewPage.vue` 重写（7 分区组合，script ≤200 行）
- [x] **P5 i18n**：`overviewPage.v2.*` × 2 locale（纯追加）
- [x] **P6 样式**：`app-global.sass` V1 概览样式块（~720 行）替换为 `ov-*` 区段（~760 行），其余组件仍用的 `overview-*` 类收敛为共享兼容层
- [x] **P7 清理**：删除 V1 组件 25 文件 + 孤儿 features 5 文件
- [x] **P8 验证**：vue-tsc（本任务文件零新增错误）/ eslint / vitest 2391 / quasar build

## V2.2 改动文件清单

新增（15）：`components/overview/` ×11（9 组件 + ovTypes + ovTime）、`features/decision/types.ts`、`features/decision/api.ts`、`features/usage/useAttentionBoard.ts`（另：`features/usage/useOverviewPage.ts` 全量重写）

修改（6）：`pages/OverviewPage.vue`、`services/index.ts`、`features/usage/types.ts`、`i18n/locales/zh-CN.ts`、`i18n/locales/en-US.ts`、`css/app-global.sass`

删除（30）：`components/usage/` 下 V1 全部 25 文件；`features/usage/` 下 `useAllModelsBreakdown.ts`/`usageDonutOption.ts`/`usageBreakdownSlices.ts`/`usageGranularityLinkage.ts`（孤儿）+ `__tests__` 同步删除

## V2.3 验收标准

| 验收 ID | 验收标准 | 状态 |
|---------|----------|------|
| AV2-1 | 首屏 sticky 横幅 5 秒内可判定系统状态（三态 verdict + 4 栏计数） | ✅ |
| AV2-2 | 注意力队列 4 栏常驻 ②位，30s 轮询，tab 隐藏停轮询 | ✅ |
| AV2-3 | 成功率永远带口径定语（调用成功率 / Run 合格率） | ✅ |
| AV2-4 | 空窗显示「—」+「窗口内无调用/执行」，无伪 0.0% 告警 | ✅ |
| AV2-5 | 未定价模型：chip 计数 + Top5 斜纹 + 页脚披露 | ✅ |
| AV2-6 | UTC 切日/Provider 快照/未定价 3 条披露在页脚可见 | ✅ |
| AV2-7 | 昼/夜主题下全部组件语义色正确（仅 --color-* token） | ✅ |
| AV2-8 | lint / test / build 三绿 | ✅ |

## V2.4 差距与优化（P1 后端，见设计文档 §7）

| 编号 | 项 | 说明 |
|------|----|------|
| OV2-1 | attention-summary 聚合端点 | 替代 4 路直读（前端 TECH-DEBT 已标注） |
| OV2-2 | 告警 ack API | 当前队列只读 |
| OV2-3 | overview tz_offset_minutes | 当前 UTC 口径仅披露 |
| OV2-4 | unpriced_call_count | 当前仅 top_models 行 |
| OV2-5 | cron/health、org runtime-summary、provider error_class、runner points[] | 均为诚实降级展示 |

## V2.5 收尾 polish（2026-10-22 ✅ 已完成）

V2 交付后的复查清单收口：

- [x] **Page ≤200 行红线达标**：script 234→75 行。新增 `features/usage/useOverviewView.ts` view-model 层（7 分区展示绑定 + tone/口径判定 + 文案），Page 只做模板拼装与路由下钻
- [x] **LIVE 滞后检测落地**：横幅 `:stale="false"` 占位 → 30s ticker 驱动的真实判定（最新数据源更新距今 >120s 转灰）
- [x] **加载失败兜底**：usageStore.error 非空时横幅上方 q-banner 错误条 + 重试（V1 errorBanner 缺口补齐）
- [x] **未定价 chip 下钻**：可点击查看 → `/models`（P9 每卡片=下钻门）
- [x] **composable 死导出清理**：删除 `providerHealthDisclaimer`/`costSparkline`/`monthSummary`（页面未消费；monthSummary 定义保留供月末预测内部使用）
- [x] **decision api 类型修复**：protogen 请求类型为「必带键、值 undefined」非 optional，显式补全 7 字段 undefined（TS2345）
- [x] **验证三绿**：eslint 0 问题 / vue-tsc 本任务文件零错误 / vitest 304 文件 2391 用例全过 / quasar build 成功
- [x] **腐坏扫描**：全部新改文件无 U+201C/201D 智能引号字节（locale 内既有中文弯引号文案为正常字符串，非 watcher 腐坏）

> 既有树红不改的任务归属：zh-CN.ts:425/en-US.ts:432 `stop` 重复键（HEAD 已提交态即有）与 evaluation/graph/hooks 的 vue-tsc 错误均为 dev 提交态破裂（见 dev-branch-broken-build），不在本任务范围。
