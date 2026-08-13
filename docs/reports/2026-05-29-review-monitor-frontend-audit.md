# 监控页面深度审查报告

> 审查日期：2026-05-29
> 审查范围：MonitorPage 全部 6 个 Tab（Usage / Alerts / Audit / Events / Traces / Logs）及嵌套 Dialog

---

## 一、问题总览

| # | 严重级别 | 类别 | 问题摘要 |
|---|---------|------|---------|
| 1 | 🔴 严重 | 数据正确性 | `listMonitorTraceEvents` 不安全类型转换，Traces Tab 显示不存在的字段 |
| 2 | 🔴 严重 | 数据流 | `MonitorAlertRules.vue` 直接修改 props 对象属性，绕过 Store 流程 |
| 3 | 🟠 高 | 数据流 | `useMonitorPage` 创建与 Store 重复的本地状态，导致 Store 数据永远为空 |
| 4 | 🟠 高 | UX 主题 | AuditTable / RealtimeEvents Dialog 缺少 `app-glass-dialog` 毛玻璃样式 |
| 5 | 🟠 高 | UX 主题 | TraceList Dialog 未使用 `app-dialog-card`，无毛玻璃样式 |
| 6 | 🟠 高 | 国际化 | FlowTracePanel 空状态文本为英文，与全中文 UI 不一致 |
| 7 | 🟡 中 | 国际化 | Traces 表格列标题全英文（Agent / Token in / out / Latency / Cost / Time） |
| 8 | 🟡 中 | UI 缺陷 | Events 表格 "time" 列宽度值 `"10"` 疑为笔误，应为 `"10%"` |
| 9 | 🟡 中 | 数据缺失 | `RunnerMetricsSummary` 类型缺少 proto 定义的平均/分位延迟字段 |
| 10 | 🟡 中 | 功能缺失 | Alert Rules 无"新增规则"按钮 |
| 11 | 🟡 中 | 功能缺陷 | 自动刷新仅覆盖 audit/events/traces，不包含 usage 和 alerts |
| 12 | 🟡 中 | 代码质量 | `AuditTable` 接收 `total` prop 但从未使用 |
| 13 | 🟢 低 | 性能 | `TraceList` deep watch `props.rows` 可能导致性能问题 |
| 14 | 🟢 低 | UX 主题 | Alert Rules 保存按钮使用 `color="primary"` 而非 `--color-accent` |

---

## 二、问题详细说明

### 问题 1：`listMonitorTraceEvents` 不安全类型转换 🔴

**文件**：[api.ts:276-279](file:///f:/aranea-agents/web/src/features/monitor/api.ts#L276-L279)

**现状**：
```typescript
export async function listMonitorTraceEvents(query: ModelUsageQuery = {}): Promise<MonitorTraceEvent[]> {
  const rows = await listModelUsageEvents(query);
  return rows as MonitorTraceEvent[];
}
```

**问题**：`MonitorTraceEvent` 扩展了 `ModelTokenUsageEvent` 并添加了 `metadata_json`、`error_code`、`retry_count`、`time_to_first_token_ms` 等字段。但 `listModelUsageEvents` 返回的 `ModelTokenUsageEvent` 不包含这些字段，`as` 强转后这些字段始终为 `undefined`。

**影响**：
- TraceList 表格中 `metadata_json` 为空，导致 Trace 详情的 Span 树/瀑布图永远无数据
- `error_code`、`error_message` 等字段为空，搜索和筛选功能失效
- `formatMoney(props.row.total_cost_micro_usd)` 等格式化函数对 `undefined` 值处理不当

**修复方案**：调用后端 `ListMonitorTraces` API（proto 已定义），将 `MonitorPlatformRow` 映射为 `MonitorTraceEvent`，包含完整字段。

---

### 问题 2：`MonitorAlertRules.vue` 直接修改 props 对象属性 🔴

**文件**：[MonitorAlertRules.vue:18-34](file:///f:/aranea-agents/web/src/components/monitor/MonitorAlertRules.vue#L18-L34)

**现状**：
```vue
<q-input v-model="rule.name" dense outlined label="名称" />
<q-input v-model="rule.metric_key" dense outlined label="指标键" />
```

**问题**：`rules` prop 来自 `useMonitorAlertRules` → `storeToRefs(monitorStore).alertRules`，通过引用传递。`v-model` 直接修改对象属性等同于直接修改 Store 状态，绕过了 Pinia 的响应式流程。

**影响**：
- 编辑即生效，"保存"按钮形同虚设（Store 已被修改）
- 保存失败时无法回滚（Store 已是编辑后的值）
- 无法实现"取消编辑"功能

**修复方案**：组件内维护 `editableRules` 深拷贝，编辑操作作用于拷贝；保存时 emit 事件，由 composable 调 Store action。

---

### 问题 3：`useMonitorPage` 创建与 Store 重复的本地状态 🟠

**文件**：[useMonitorPage.ts:15-21](file:///f:/aranea-agents/web/src/features/monitor/useMonitorPage.ts#L15-L21)

**现状**：
```typescript
const auditRows = ref<AuditLog[]>([]);
const auditTotal = ref(0);
const events = ref<PlatformResource[]>([]);
const traces = ref<MonitorTraceEvent[]>([]);
```

**问题**：Store 已有 `auditLogs`、`auditTotal`、`events` 等状态，但 composable 使用 `fetchAuditPage()` / `fetchMonitorEvents()` 等只返回数据不更新 Store 的方法，导致：
- `store.auditLogs` 始终为 `[]`
- `store.events` 始终为 `[]`
- 其他组件若通过 `storeToRefs` 读取 Store 会拿到空数据

**修复方案**：composable 应使用 `storeToRefs` 读取 Store 状态，调用 Store 的 `loadAuditLogs()` / `loadEvents()` 等 action 更新 Store。

---

### 问题 4：AuditTable / RealtimeEvents Dialog 缺少毛玻璃样式 🟠

**文件**：
- [AuditTable.vue:109](file:///f:/aranea-agents/web/src/components/monitor/AuditTable.vue#L109)
- [RealtimeEvents.vue:102](file:///f:/aranea-agents/web/src/components/monitor/RealtimeEvents.vue#L102)

**现状**：
```vue
<q-card class="monitor-detail-card app-dialog-card app-dialog-card--lg">
```

**问题**：根据 UX 规范（aranea-frontend-guide 第七章），Dialog 应使用 `app-dialog-card` + `app-glass-dialog` 组合。当前缺少 `app-glass-dialog` 类。

**修复方案**：添加 `app-glass-dialog` 类，并按规范调整内部结构（`app-glass-dialog__head` / `__scroll` / `__body` / `__actions`）。

---

### 问题 5：TraceList Dialog 未使用 `app-dialog-card` 🟠

**文件**：[TraceList.vue:86](file:///f:/aranea-agents/web/src/components/monitor/TraceList.vue#L86)

**现状**：
```vue
<q-dialog v-model="detailOpen" maximized @hide="stopFlowStream">
  <q-card class="monitor-trace-dialog">
```

**问题**：maximized Dialog 完全没有 `app-dialog-card` 和 `app-glass-dialog` 样式，与项目其他 Dialog 风格不一致。

**修复方案**：添加 `app-dialog-card` 类；maximized 模式下可选择性添加 `app-glass-dialog`。

---

### 问题 6：FlowTracePanel 空状态英文 🟠

**文件**：[FlowTracePanel.vue:4](file:///f:/aranea-agents/web/src/components/monitor/FlowTracePanel.vue#L4)

**现状**：
```vue
No flow logs yet. Keep this detail open and run a chat turn, or use the Logs tab for the global stream.
```

**修复方案**：改为中文"暂无流程日志。保持详情打开并运行一次对话，或在日志 Tab 查看全局流。"

---

### 问题 7：Traces 表格列标题全英文 🟡

**文件**：[monitorTableUi.ts:28-35](file:///f:/aranea-agents/web/src/components/monitor/monitorTableUi.ts#L28-L35)

**现状**：
```typescript
registryCol<MonitorTraceEvent>("name", "Agent", ...),
registryCol<MonitorTraceEvent>("tokens", "Token in / out", ...),
registryCol<MonitorTraceEvent>("latency", "Latency", ...),
registryCol<MonitorTraceEvent>("cost", "Cost", ...),
registryCol<MonitorTraceEvent>("time", "Time", ...),
```

**修复方案**：统一为中文列标题。

---

### 问题 8：Events 表格 "time" 列宽度值疑为笔误 🟡

**文件**：[monitorTableUi.ts:23](file:///f:/aranea-agents/web/src/components/monitor/monitorTableUi.ts#L23)

**现状**：
```typescript
registryCol<MonitorViewEvent>("time", "时间", "time", "left", "10"),
```

**问题**：`"10"` 不是有效的 CSS 宽度值，应为 `"10%"` 或使用 `REGISTRY_COL_W` 常量。

**修复方案**：改为 `"10%"` 或 `REGISTRY_COL_W.time`。

---

### 问题 9：`RunnerMetricsSummary` 类型缺少 proto 字段 🟡

**文件**：
- [types.ts:104-110](file:///f:/aranea-agents/web/src/features/monitor/types.ts#L104-L110)
- [api.ts:324-334](file:///f:/aranea-agents/web/src/features/monitor/api.ts#L324-L334)

**现状**：Proto 定义了 `avgDurationMs`、`p50DurationMs`、`p95DurationMs`、`p99DurationMs`，但前端类型和映射函数均未包含。

**修复方案**：扩展类型定义和映射函数，在 RunnerMetricsPanel 中展示延迟分位数据。

---

### 问题 10：Alert Rules 无"新增规则"按钮 🟡

**文件**：[MonitorAlertRules.vue](file:///f:/aranea-agents/web/src/components/monitor/MonitorAlertRules.vue)

**问题**：只能编辑/保存已有规则，无法新增。用户需通过 API 手动添加。

**修复方案**：添加"新增规则"按钮，在列表末尾追加空规则模板。

---

### 问题 11：自动刷新不覆盖 usage / alerts Tab 🟡

**文件**：[useMonitorPage.ts:41-45](file:///f:/aranea-agents/web/src/features/monitor/useMonitorPage.ts#L41-L45)

**现状**：
```typescript
function refreshActiveTab() {
  if (tab.value === "audit") void loadAudit();
  else if (tab.value === "events") void loadEvents();
  else if (tab.value === "traces") void loadTraces();
}
```

**修复方案**：增加 usage tab 的 Runner Metrics 刷新。

---

### 问题 12：`AuditTable` 的 `total` prop 未使用 🟡

**文件**：[AuditTable.vue:157-161](file:///f:/aranea-agents/web/src/components/monitor/AuditTable.vue#L157-L161)

**现状**：`total` prop 被声明但从未在模板或脚本中使用。分页使用 `filteredRows.length`。

**修复方案**：移除 `total` prop，或改为服务端分页时使用。

---

### 问题 13：`TraceList` deep watch `props.rows` 🟢

**文件**：[TraceList.vue:288](file:///f:/aranea-agents/web/src/components/monitor/TraceList.vue#L288)

**现状**：
```typescript
watch(() => props.rows, tryOpenHighlightedRun, { deep: true });
```

**修复方案**：改为 watch `props.rows.length` 或 `props.highlightUsageEventId` 即可。

---

### 问题 14：Alert Rules 保存按钮颜色 🟢

**文件**：[MonitorAlertRules.vue:8](file:///f:/aranea-agents/web/src/components/monitor/MonitorAlertRules.vue#L8)

**现状**：`color="primary"` — 根据项目 UX 规范，主按钮应使用 `--color-accent`。

**修复方案**：使用 `app-accent-btn` 类或自定义样式引用 `--color-accent`。

---

## 三、修复优先级

| 批次 | 问题编号 | 说明 |
|------|---------|------|
| P0（立即修复） | 1, 2, 3 | 数据正确性和数据流问题 |
| P1（本轮修复） | 4, 5, 6, 7, 8 | UX 一致性和国际化 |
| P2（后续优化） | 9, 10, 11, 12, 13, 14 | 功能增强和代码质量 |

---

## 四、修复记录

| # | 状态 | 修复内容 | 修改文件 |
|---|------|---------|---------|
| 1 | ✅ 已修复 | 补充 `tokenEventFromUnknown` 缺失的字段映射（metadata_json、error_code、retry_count 等 20+ 字段） | `features/usage/api.ts` |
| 2 | ✅ 已修复 | 改为 `editableRules` 深拷贝 + `emit('save', rules)` 传参；新增"新增规则"和"删除"按钮 | `components/monitor/MonitorAlertRules.vue`、`features/monitor/useMonitorAlertRules.ts`、`pages/MonitorPage.vue` |
| 3 | ✅ 已修复 | 改用 `storeToRefs` 读取 Store 状态，调用 `loadAuditLogs()`/`loadEvents()` 更新 Store | `features/monitor/useMonitorPage.ts`、`pages/MonitorPage.vue` |
| 4 | ✅ 已修复 | 添加 `app-glass-dialog` + 规范内部结构（`__head`/`__scroll`/`__body`/`__actions`） | `components/monitor/AuditTable.vue`、`components/monitor/RealtimeEvents.vue` |
| 5 | ✅ 已修复 | 添加 `app-dialog-card` + `app-glass-dialog` + 规范内部结构 | `components/monitor/TraceList.vue` |
| 6 | ✅ 已修复 | 英文空状态改为中文 | `components/monitor/FlowTracePanel.vue` |
| 7 | ✅ 已修复 | 列标题改为中文（Token 入/出、延迟、费用、时间） | `components/monitor/monitorTableUi.ts` |
| 8 | ✅ 已修复 | `"10"` → `"10%"` | `components/monitor/monitorTableUi.ts` |
| 9 | ✅ 已修复 | 扩展类型 + API 映射 + RunnerMetricsPanel 展示 P50/P95/P99/平均延迟 | `features/monitor/types.ts`、`features/monitor/api.ts`、`components/monitor/RunnerMetricsPanel.vue` |
| 10 | ✅ 已修复 | 同问题 2 修复，新增"新增规则"按钮 | `components/monitor/MonitorAlertRules.vue` |
| 11 | ⏳ 待后续 | 需要协调 Runner Metrics composable 的刷新逻辑 | — |
| 12 | ✅ 已修复 | 移除未使用的 `total` prop | `components/monitor/AuditTable.vue`、`pages/MonitorPage.vue` |
| 13 | ✅ 已修复 | `deep: true` → watch `props.rows.length` | `components/monitor/TraceList.vue` |
| 14 | ✅ 已修复 | `color="primary"` → `class="app-accent-btn"` | `components/monitor/MonitorAlertRules.vue` |

---

## 五、第二轮深度审查 — 新发现问题

| # | 级别 | 类别 | 问题摘要 |
|---|------|------|---------|
| N1 | 🔴 严重 | 红线 #4 | `FlowLogExportButton.vue` 展示组件直接调用 `Notify.create` |
| N2 | 🔴 严重 | 红线 #4 | `AuditTable.vue` 展示组件直接调用 `Notify.create` |
| N3 | 🔴 严重 | 红线 #4 | `TraceList.vue` 展示组件直接调用 `Notify.create` |
| N4 | 🟠 高 | 红线 #4 | `RealtimeEvents.vue` 使用 `$q.dialog` 确认清除，应 emit 由 Page 处理 |
| N5 | 🟠 高 | 红线 #4 | `FlowLogStream.vue` 使用 `$q.dialog` 确认清除，应 emit 由 Page 处理 |
| N6 | 🟠 高 | UX 主题 | `MonitorUsageDashboardLink.vue` "打开概览"按钮使用 `color="primary"` 而非 `--color-accent` |
| N7 | 🟠 高 | 国际化 | `TraceList.vue` 详情面板 "Live capture" 提示仍为英文 |
| N8 | 🟡 中 | 代码质量 | `useMonitorPage` 仍返回 `auditTotal` 但 Page 不再使用 |
| N9 | 🟡 中 | 代码质量 | `listMonitorTraceEvents` 仍使用 `as` 强转（已安全但需注释说明） |

### 第二轮修复记录

| # | 状态 | 修复内容 | 修改文件 |
|---|------|---------|---------|
| N1 | ✅ 已修复 | 改为 `emit('export')`，导出逻辑移入 TraceList composable | `FlowLogExportButton.vue` |
| N2 | ✅ 已修复 | 改为 `emit('notify', payload)`，通知逻辑上收到 Page | `AuditTable.vue`、`MonitorPage.vue` |
| N3 | ✅ 已修复 | 改为 `emit('notify', payload)`，通知逻辑上收到 Page | `TraceList.vue`、`MonitorPage.vue` |
| N4 | ✅ 已修复 | 改为 `emit('clear')`，确认框移入 Page 层 `$q.dialog` | `RealtimeEvents.vue`、`MonitorPage.vue` |
| N5 | ✅ 已修复 | 改为 `emit('clear')`，确认框移入 Page 层 `$q.dialog` | `FlowLogStream.vue`、`LogStreamPanel.vue`、`MonitorPage.vue` |
| N6 | ✅ 已修复 | `color="primary"` → `class="app-accent-btn"` | `MonitorUsageDashboardLink.vue` |
| N7 | ✅ 已修复 | "Live capture…" → "实时捕获（按 trace_id 过滤，详情打开期间持续接收）" | `TraceList.vue` |
| N8 | ✅ 已修复 | 移除 `auditTotal` 返回值 | `useMonitorPage.ts` |
| N9 | ✅ 已修复 | 添加安全强转注释说明 | `api.ts` |

---

## 六、aranea-frontend-review 最终审查报告

### 概要

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| 数据流合规 | 0 | 0 | 0 | 0 |
| 组件分层 | 0 | 0 | 0 | 0 |
| 业务逻辑归属 | 0 | 1 | 0 | 1 |
| 聊天消息分组 | 0 | 0 | 0 | 0 |
| UX 主题 | 0 | 0 | 0 | 0 |
| 构建与回归 | 0 | 0 | 0 | 0 |

### 阻断项（必须修复）

无

### 建议项（推荐修复）

| ID | 维度 | 文件 | 问题描述 | 修复建议 |
|----|------|------|----------|----------|
| S1 | 业务逻辑 | `AuditTable.vue` / `TraceList.vue` | 客户端筛选/分页逻辑在组件 computed 中 | 大数据量时考虑移入 composable |

### 亮点

- 所有展示组件零 Store/API/Notify import，数据流完全合规
- 所有 Dialog 统一使用 `app-glass-dialog` 规范结构
- 所有确认框（清除事件/清除日志）正确使用 `emit` → Page `$q.dialog` 模式
- 所有通知使用 `emit('notify', payload)` → Page `$q.notify` 模式
- `MonitorAlertRules.vue` 正确使用 `structuredClone` 深拷贝 + `emit('save', rules)` 模式
- `useMonitorPage` 正确使用 `storeToRefs` + Store actions，消除了双状态问题
- 表格列定义全部使用 `registryCol()` + `REGISTRY_COL_W`，符合 Registry 表格规范
- 主按钮统一使用 `app-accent-btn`，符合 UX 主题规范
