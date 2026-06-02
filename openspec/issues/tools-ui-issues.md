# Tools 管理页面 UI 问题清单

> 审查日期：2026-05-29
> 审查范围：ToolsPage、ToolRunsPage、ToolAuditsPage 及其所有子组件、Store、Composable、API 层

---

## P0 — 严重缺陷（功能不可用）

### P0-01: ToolEditorDialog 缺少 key/source/risk_level 必填字段

**位置**: `web/src/components/tools/ToolEditorDialog.vue` 第 65-117 行（基础信息 section）

**现象**: 新建 Tool 时，编辑器的基础信息区只有 `display_name`、`category`、`description` 三个字段。缺少以下必填/重要字段：
- `key`（proto 标记 REQUIRED，后端校验必填）
- `source`（影响工具来源标识）
- `risk_level`（影响策略与审计）

**影响**: 新建 Tool 时无法设置 key，提交空 key 会被后端拒绝，导致创建功能完全不可用。

**根因**: `ToolEditorDialog.vue` 使用内联 section 布局，但没有包含完整字段。而 `editor/ToolEditorBasicTab.vue` 已包含完整字段（key、display_name、description、category、source、risk_level），却未被引用。

**修复方案**: 在 ToolEditorDialog 的基础信息 section 中补充 key、source、risk_level 字段，参考 `ToolEditorBasicTab.vue` 的实现。

---

### P0-02: ToolEditorDialog 缺少运行策略开关（policy toggles）

**位置**: `web/src/components/tools/ToolEditorDialog.vue`

**现象**: 编辑器没有 `enabled`、`readonly`、`requires_confirmation`、`supports_streaming`、`supports_concurrency` 五个策略开关的 UI。

**影响**: 用户无法在创建/编辑时设置工具的运行策略，这些字段始终使用 `blankToolForm()` 的默认值。

**根因**: `editor/ToolEditorPolicyTab.vue` 已实现完整的策略开关 UI，但 `ToolEditorDialog.vue` 未引用。

**修复方案**: 在 ToolEditorDialog 中增加策略 section，引入 `ToolEditorPolicyTab.vue` 或内联实现策略开关。

---

### P0-03: ToolAuditsPage 筛选条件变更不自动刷新

**位置**: `web/src/features/tools/useToolAuditsPage.ts` 第 53-55 行

**现象**: `useToolAuditsPage` 只 watch 了 `page` 和 `pageSize`，没有 watch `toolKey`、`agentId`、`userId`、`status`。用户修改筛选条件后，列表不会自动刷新，必须手动点击"刷新"按钮。

**对比**: `useToolRunsPage.ts` 正确地 watch 了所有筛选条件。

**修复方案**: 添加 `watch([toolKey, agentId, userId, status], () => { page.value = 1; void loadRows(); });`

---

## P1 — 重要缺陷（数据/交互错误）

### P1-01: ToolsTable runtime_status 为 disabled 时 badge 颜色错误

**位置**: `web/src/components/tools/ToolsTable.vue` 第 50 行

**现象**: `runtime_status === 'disabled'` 时，badge 显示文字"禁用"（正确），但颜色为 positive（绿色），与"禁用"语义矛盾。

**代码**:
```html
<q-badge rounded :color="props.row.runtime_status === 'catalog_only' ? 'grey' : 'positive'">
```

**修复方案**: 改为三色逻辑：
```html
<q-badge rounded :color="runtimeStatusColor(props.row.runtime_status)">
```
```ts
function runtimeStatusColor(status?: string): string {
  if (status === 'disabled') return 'negative';
  if (status === 'catalog_only') return 'grey';
  return 'positive';
}
```

---

### P1-02: ToolDetailDrawer runtime_status 为 disabled 时 badge 颜色同样错误

**位置**: `web/src/components/tools/ToolDetailDrawer.vue` 第 51 行

**现象**: 同 P1-01，详情抽屉中的 runtime_status chip 对 disabled 状态也缺少专门的颜色处理。

**修复方案**: 同 P1-01，使用统一的 `runtimeStatusColor` 函数。

---

### P1-03: ToolDetailContent.vue 是死代码（未被任何组件引用）

**位置**: `web/src/components/tools/ToolDetailContent.vue`

**现象**: `ToolDetailContent.vue` 是一个完整的详情内容组件（含 overview/params/config/agents/runs 五个 Tab），但没有任何组件 import 或使用它。`ToolDetailDrawer.vue` 内联实现了所有详情内容。

**影响**: 代码维护混乱，两套实现容易产生不一致。

**修复方案**: 删除 `ToolDetailContent.vue` 死代码，或在 `ToolDetailDrawer.vue` 中引用它替代内联实现。

---

### P1-04: useToolDetailPanel.ts composable 是死代码

**位置**: `web/src/features/tools/useToolDetailPanel.ts`

**现象**: `useToolDetailPanel` composable 与 `stores/tools/toolDetail.ts` Store 功能完全重复。页面使用的是 Store，composable 未被引用。

**影响**: 代码冗余，维护时容易遗漏同步修改。

**修复方案**: 删除 `useToolDetailPanel.ts`，统一使用 Store。

---

### P1-05: useToolEditor.ts composable 与 toolEditor.ts Store 重复

**位置**: `web/src/features/tools/useToolEditor.ts`

**现象**: `useToolEditor` composable 与 `stores/tools/toolEditor.ts` Store 功能完全重复（blankToolForm、assignForm、openCreate、openEdit、save 等）。页面使用 Store，composable 仅 `useToolToggle` 被引用。

**影响**: 同 P1-04。

**修复方案**: 将 `useToolToggle` 提取为独立文件，删除 `useToolEditor` composable 的其余部分。

---

### P1-06: ToolDetailConfigPanel 中 schemaEditJson 编辑功能无保存出口

**位置**: `web/src/components/tools/ToolDetailConfigPanel.vue` 第 132-143 行

**现象**: `schemaEditJson` 和 `onSchemaEdit` 允许用户编辑 config_schema_json，但没有保存按钮或 emit 事件将修改传回父组件。编辑后无法持久化。

**修复方案**: 要么移除这个不可用的编辑功能，要么添加保存逻辑（emit 事件 + 父组件调用 updateTool API）。

---

### P1-07: ToolOverrideEditorDialog 缺少 config_override_json 的 JSON 校验

**位置**: `web/src/components/tools/ToolOverrideEditorDialog.vue` 第 42-49 行

**现象**: `config_override_json` 是自由文本输入，没有 JSON 格式校验。用户输入无效 JSON 后，保存时会被后端拒绝，但前端没有提前拦截。

**修复方案**: 在保存前添加 JSON.parse 校验，或在输入时添加实时校验提示。

---

## P2 — 一般缺陷（体验/一致性问题）

### P2-01: ToolRunsPage/ToolAuditsPage 使用 SkillPagination 而非 AppRegistryPagination

**位置**: `web/src/pages/ToolRunsPage.vue` 第 33 行, `web/src/pages/ToolAuditsPage.vue` 第 37 行

**现象**: 两个子页面使用 `SkillPagination`（来自 skills 域），而主页面 `ToolsPage` 使用 `AppRegistryPagination`。跨域组件引用且风格不一致。

**修复方案**: 统一使用 `AppRegistryPagination`。

---

### P2-02: ToolAuditsPage CSS 类名不一致

**位置**: `web/src/pages/ToolAuditsPage.vue` 第 2、9、28 行

**现象**: 审计页面使用了 `tool-runs-page`（runs 页面的类名）和 `tool-runs-outline-btn`（runs 页面的按钮类名），以及 `tools-error-banner`（与主页面 `app-page-error-banner` 不一致）。

**修复方案**: 审计页面应使用自己的语义类名，如 `tool-audits-page`、`tool-audits-outline-btn`、`app-page-error-banner`。

---

### P2-03: ToolRunsFilters 的"开始时间"使用纯文本 ISO 输入

**位置**: `web/src/components/tools/ToolRunsFilters.vue` 第 40-47 行

**现象**: "开始时间 ISO" 是一个纯文本输入框，要求用户手动输入 ISO 格式时间字符串，体验差。

**修复方案**: 改用 Quasar 的 `q-date` / `q-time` 组件或 `date` 类型 input。

---

### P2-04: ToolDetailDrawer Agent 覆盖列表显示 agent_id 而非 agent 名称

**位置**: `web/src/components/tools/ToolDetailDrawer.vue` 第 265 行

**现象**: 覆盖列表中 `<q-item-label>{{ o.agent_id }}</q-item-label>` 显示的是 Agent ID（UUID 格式），而非人类可读的 Agent 名称。`agentOptions` 已加载了名称映射但未在此处使用。

**修复方案**: 通过 `agentOptions` 查找 agent_id 对应的 label 显示名称。

---

### P2-05: fetchToolAgentBindingSummary 存在 N+1 API 调用性能问题

**位置**: `web/src/features/tools/toolAgentBindingSummary.ts` 第 38-64 行

**现象**: 对每个 Agent 单独调用 `getAgentEffectiveTools(agentId)`，200 个 Agent 会产生 200 次 API 调用。

**影响**: 打开详情抽屉的 Agent Tab 时，加载时间随 Agent 数量线性增长。

**修复方案**: 后端提供批量查询接口（如 `BatchGetAgentEffectiveTools`），或前端改为懒加载/分页。

---

### P2-06: ToolDetailDrawer 成功率计算未排除 blocked 调用

**位置**: `web/src/components/tools/ToolDetailDrawer.vue` 第 390-394 行

**现象**: `successRate = success_count / invoke_count`，但 `invoke_count` 包含 blocked 调用。blocked 是策略拦截，不应计入成功率分母。

**修复方案**: 改为 `success_count / (success_count + failure_count)`，或在 UI 上单独展示 blocked 数。

---

### P2-07: editor/ 目录下的 Tab 组件未被使用

**位置**: `web/src/components/tools/editor/ToolEditorBasicTab.vue`, `ToolEditorSchemaTab.vue`, `ToolEditorPolicyTab.vue`, `ToolEditorAdvancedTab.vue`

**现象**: 四个编辑器 Tab 组件已实现完整功能，但 `ToolEditorDialog.vue` 使用内联 section 布局，未引用任何 Tab 组件。

**影响**: 代码冗余，且 Tab 组件中的完整字段（如 key、source、risk_level、policy toggles）在 Dialog 中缺失。

**修复方案**: 重构 `ToolEditorDialog.vue` 引用 Tab 组件，或删除未使用的 Tab 组件并将字段补充到 Dialog 内联实现中。

---

## 修复优先级排序

| 优先级 | 编号 | 简述 | 状态 |
|--------|------|------|------|
| P0 | P0-01 | 编辑器缺少 key/source/risk_level 必填字段 | ✅ 已修复 |
| P0 | P0-02 | 编辑器缺少策略开关 | ✅ 已修复 |
| P0 | P0-03 | 审计页筛选不自动刷新 | ✅ 已修复 |
| P1 | P1-01 | 表格 disabled 状态 badge 颜色错误 | ✅ 已修复 |
| P1 | P1-02 | 详情抽屉 disabled 状态 badge 颜色错误 | ✅ 已修复 |
| P1 | P1-03 | ToolDetailContent.vue 死代码 | ✅ 已删除 |
| P1 | P1-04 | useToolDetailPanel.ts 死代码 | ✅ 已删除 |
| P1 | P1-05 | useToolEditor.ts 重复代码 | ✅ 已清理（提取 useToolToggle.ts，删除 useToolEditor.ts） |
| P1 | P1-06 | ConfigPanel schema 编辑无保存出口 | ✅ 已修复（完整事件链 + saveConfigSchema） |
| P1 | P1-07 | Override 编辑器缺 JSON 校验 | ✅ 已修复 |
| P2 | P2-01 | 分页组件跨域引用不一致 | ✅ 已修复 |
| P2 | P2-02 | 审计页 CSS 类名不一致 | ✅ 已修复 |
| P2 | P2-03 | 时间筛选使用纯文本输入 | ⏳ 待修复（低优先级） |
| P2 | P2-04 | 覆盖列表显示 agent_id 而非名称 | ✅ 已修复 |
| P2 | P2-05 | Agent 绑定摘要 N+1 调用 | ⏳ 待修复（需后端配合） |
| P2 | P2-06 | 成功率计算含 blocked | ✅ 已修复 |
| P2 | P2-07 | 编辑器 Tab 组件未使用 | ✅ 已删除 |

---

## 审查修复记录

### aranea-review 审查（2026-05-29）

审查范围：本次修复涉及的所有前端文件。

**审查结论**：0 个 🔴 阻断项，3 个 🟡 建议项，1 个 🟢 提示项。

**建议项修复**：
- S2/S3: `toolToUpsertInput` 映射逻辑重复 → 提取为 `features/tools/toolFormPatch.ts` 中的 `toolToUpsertInput(tool, overrides?)` 工具函数，`ToolsPage.vue` 和 `toolDetail.ts` 共用
