# 评估管理页面问题清单

> 审查日期：2026-05-29（第二轮深入下沉审查）
> 审查范围：EvaluationPage 全链路（Page → Composable → Store → API → Proto → 后端 Service/Biz/Data）

---

## 第一轮问题（已修复）

| # | 严重级别 | 问题 | 状态 |
|---|----------|------|------|
| 1 | 🔴 Critical | `EvalResult` 类型不存在，编译失败 → 改为 `EvalCaseResult` | ✅ 已修复 |
| 2 | 🟠 High | 趋势 API 参数 `metric` 被静默丢弃 → Store 参数类型改为 `dataset_id` | ✅ 已修复 |
| 3 | 🟠 High | 趋势加载缺少 `dataset_id` 过滤 → Composable 传递 `selectedDatasetId` | ✅ 已修复 |
| 4 | 🟡 Medium | 对比选择未在数据集切换时重置 → watch `runs` 重置 `localSelected` | ✅ 已修复 |
| 5 | 🟡 Medium | 结果 Dialog 分页未重置 → watch `runId` 重置 `page = 1` | ✅ 已修复 |
| 6 | 🟡 Medium | Analytics Panel 分页未重置 → watch 各数据源重置分页 | ✅ 已修复 |

---

## 第二轮深入下沉问题

### 问题 7：人工标注"清除"操作无法发送到后端

**严重级别**：🟠 High（功能缺陷）

**文件**：[api.ts](file:///f:/aranea-agents/web/src/features/evaluation/api.ts#L92-L103)

**现象**：
- 用户在 `EvaluationResultsDialog` 中将 `human_pass` 从 Pass/Fail 切回 `-`（unset）
- `onPassChange` 将 `human_pass` 设为 `null`
- 但 `annotateCaseResult` API 中，`human_pass !== null` 条件跳过了 `null` 值
- 请求 body 中不包含 `human_pass` 字段，后端无法区分"未修改"和"清除"

**数据流追踪**：
```
EvaluationResultsDialog.onPassChange(row, 'unset')
  → row.human_pass = null
    → emit('update-row', next)
      → useEvaluationPage.saveAnnotation(row)
        → store.annotateResult({ human_pass: null })
          → api.annotateCaseResult({ human_pass: null })
            → body.human_pass 被跳过（null 检查）
              → 后端收到 {} → 不更新 human_pass
```

**修复方案**：将 `human_pass !== null` 改为 `human_pass !== undefined`，`null` 值显式发送

**状态**：✅ 已修复

---

### 问题 8：运行记录分页未在数据集切换时重置

**严重级别**：🟡 Medium（UX 缺陷）

**文件**：[EvaluationPage.vue](file:///f:/aranea-agents/web/src/pages/EvaluationPage.vue#L182-L188)

**现象**：
- `runsPage` 在 `EvaluationPage.vue` 中是本地 ref
- 用户在数据集 A 的运行记录第 3 页，切换到数据集 B
- 数据集 B 可能只有 1 页数据，但 `runsPage` 仍为 3
- 虽然 watch 会在 `runsPage > runsPageMax` 时修正，但用户看到的不是第 1 页

**修复方案**：watch `selectedDatasetId` 变化时重置 `runsPage = 1`

**状态**：✅ 已修复

---

### 问题 9：mappers.ts 与 api.ts 重复定义 mapDataset/mapRun

**严重级别**：🟢 Low（技术债务）

**文件**：
- [mappers.ts](file:///f:/aranea-agents/web/src/features/evaluation/mappers.ts) — 轻量版 mapper（4 字段/8 字段）
- [api.ts](file:///f:/aranea-agents/web/src/features/evaluation/api.ts) — 完整版 mapper（7 字段/19 字段）

**现象**：
- 两个文件都定义了 `mapDataset` 和 `mapRun` 函数
- `mappers.ts` 是轻量版，只映射列表行需要的字段
- `api.ts` 是完整版，映射所有字段
- 命名相同但返回类型不同，容易混淆

**影响**：
- 维护时需要同步修改两处
- 新开发者可能误用轻量版 mapper 处理完整数据

**修复方案**：标记为 TECH-DEBT，后续统一为单一 mapper 文件

**状态**：📝 记录为技术债务

---

## 修复文件清单

| 文件 | 修复内容 |
|------|----------|
| `evaluationTableUi.ts` | `EvalResult` → `EvalCaseResult` |
| `stores/evaluation/index.ts` | `loadAgentTrend` 参数 `metric` → `dataset_id` |
| `useEvaluationPage.ts` | `loadTrend()` 传递 `dataset_id` 而非 `metric` |
| `EvaluationAnalyticsPanel.vue` | 添加 watch 重置 `localSelected`、`comparePage`、`trendPage`、`comparisonPage` |
| `EvaluationResultsDialog.vue` | watch `runId` 重置 `page = 1` |
| `api.ts` | `annotateCaseResult` 允许发送 `null` 值（清除标注） |
| `EvaluationPage.vue` | watch `selectedDatasetId` 重置 `runsPage = 1` |
