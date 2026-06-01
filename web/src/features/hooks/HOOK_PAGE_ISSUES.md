# HOOK/回调 页面问题清单

> 审查日期：2026-05-29
> 审查范围：HooksPage、HookDeliveriesPage、HooksTable、CallbackEditor、AgentHooksPanel 及关联 Store/API/Composable
> 第二轮：深入下沉所有 UI 交互路径

---

## 第一轮问题（已修复 ✅）

| # | 问题 | 严重度 | 状态 |
|---|------|--------|------|
| 1 | "运行记录"按钮路由错误 | 中 | ✅ 已修复 |
| 2 | HookDeliveriesPage 直接调用 API | 高 | ✅ 已修复 |
| 3 | HookDeliveriesPage 缺少 AppPageHero | 低 | ✅ 已修复 |
| 4 | HookDeliveriesPage 错误提示样式不一致 | 低 | ✅ 已修复 |
| 5 | HookDeliveriesPage 详情 Dialog 缺毛玻璃 | 中 | ✅ 已修复 |
| 6 | HooksPage 编辑 Dialog 缺毛玻璃 | 中 | ✅ 已修复 |
| 7 | toggleEnabled 直接修改 store 外对象 | 中 | ✅ 已修复 |
| 8 | AgentHooksPanel 编辑 Dialog 缺毛玻璃 | 中 | ✅ 已修复 |
| 9 | 投递页面文案与状态不一致 | 低 | ✅ 已修复 |
| 10 | 投递页面状态颜色未适配暗色 | 低 | ✅ 已修复 |
| 11 | log 类型显示 message 字段可能误导 | 低 | ✅ 已修复 |
| 12 | AgentHooksPanel 加载全部 Hook | 低 | 技术债，暂不修复 |

---

## 第二轮问题（深入下沉发现）

### 问题 13：HooksPage subtitle 中英文混用

**严重度**：低（一致性）

**位置**：[HooksPage.vue:6](file:///f:/aranea-agents/web/src/pages/HooksPage.vue#L6)

**现状**：
```html
subtitle="Configure lifecycle hooks for Agent, Model, Tool, and Runner events (log, notify, block, modify)."
```

**问题**：kicker 和 title 是中文（"回调规则"、"Hook / 回调规则"），但 subtitle 是英文。其他页面（如 HookDeliveriesPage）subtitle 是中文。中英文混用不一致。

**修复**：将 subtitle 改为中文。

---

### 问题 14：HooksPage confirmDelete 缺少错误处理

**严重度**：中（功能缺陷）

**位置**：[HooksPage.vue:238-248](file:///f:/aranea-agents/web/src/pages/HooksPage.vue#L238-L248)

**现状**：
```typescript
function confirmDelete(row: HookRow) {
  $q.dialog({
    title: "删除 Hook",
    message: `确定删除「${row.name}」？`,
    cancel: true,
    persistent: true
  }).onOk(async () => {
    await hooksStore.removeHook(row.id);
    await loadRows();
  });
}
```

**问题**：`onOk` 回调中 `hooksStore.removeHook` 和 `loadRows` 没有 try/catch。如果删除失败（如网络错误、后端校验失败），错误会被静默吞掉，用户不知道删除失败。

**修复**：添加 try/catch 并显示错误通知。

---

### 问题 15：HooksPage maximized Dialog 不应使用 app-glass-dialog

**严重度**：中（UX 规范）

**位置**：[HooksPage.vue:63-64](file:///f:/aranea-agents/web/src/pages/HooksPage.vue#L63-L64)

**现状**：
```html
<q-dialog v-model="editorOpen" persistent maximized>
  <q-card class="app-dialog-card app-glass-dialog">
```

**问题**：`maximized` 模式下 Dialog 占满全屏，没有背景可见，毛玻璃效果（`backdrop-filter`）无意义。同时 maximized 模式下内容没有宽度约束，表单字段会过度拉伸。应改用固定宽度（如 `app-dialog-card--xl`）+ `app-glass-dialog`。

**修复**：移除 `maximized`，改用 `app-dialog-card--xl` 宽度修饰符。

---

### 问题 16：AgentHooksPanel 错误提示使用 bg-negative text-white

**严重度**：低（UI 规范）

**位置**：[AgentHooksPanel.vue:10](file:///f:/aranea-agents/web/src/components/agents/AgentHooksPanel.vue#L10)

**现状**：
```html
<q-banner v-if="loadError" rounded class="bg-negative text-white">
```

**问题**：其他页面使用 `app-page-error-banner` class，此组件使用手写 `bg-negative text-white`，样式不一致。

**修复**：改用 `app-page-error-banner` class。

---

### 问题 17：AgentHooksPanel expansion-item 内卡片使用 flat bordered

**严重度**：低（UX 规范）

**位置**：[AgentHooksPanel.vue:25](file:///f:/aranea-agents/web/src/components/agents/AgentHooksPanel.vue#L25)

**现状**：
```html
<q-card flat bordered class="q-pa-md q-mt-sm">
```

**问题**：`flat bordered` 是实色边框卡片，不符合项目玻璃主题。应使用 `app-dialog-section` 或玻璃面板样式。

**修复**：改用 `app-dialog-section` class。

---

### 问题 18：HookDeliveriesPage detail dialog 中 text-grey-7 暗色模式不可读

**严重度**：中（可访问性）

**位置**：[HookDeliveriesPage.vue:95](file:///f:/aranea-agents/web/src/pages/HookDeliveriesPage.vue#L95)

**现状**：
```html
<div class="text-caption text-grey-7">{{ detailUrl }}</div>
```

**问题**：`text-grey-7` 在暗色模式下对比度极低，几乎不可读。应使用 CSS 变量 `var(--color-text-secondary)` 或 Quasar 的 `text-grey`（自动适配暗色模式）。

**修复**：将 `text-grey-7` 改为 `text-grey`。

---

### 问题 19：AgentHooksPanel createScopedHook 空标识生成无效 key

**严重度**：中（边界情况 bug）

**位置**：[useAgentHooksPanel.ts:61-62](file:///f:/aranea-agents/web/src/features/agents/useAgentHooksPanel.ts#L61-L62)

**现状**：
```typescript
const key = `${agentKey() || agentId()}-hook-${Date.now()}`.replace(/[^a-zA-Z0-9_-]/g, "_");
// name: `${agentKey() || agentId()} callback`
```

**问题**：如果 `agentKey()` 和 `agentId()` 都为空字符串，则 key 变为 `-hook-1234567890`（以连字符开头），name 变为 ` callback`（以空格开头）。这可能导致后端校验失败或显示异常。

**修复**：添加空值保护，当两者都为空时使用 `hook` 作为前缀。

---

### 问题 20：HooksPage saveHook 成功后多余的 loadRows 调用

**严重度**：低（性能）

**位置**：[HooksPage.vue:218](file:///f:/aranea-agents/web/src/pages/HooksPage.vue#L218)

**现状**：
```typescript
editorOpen.value = false;
await loadRows();
```

**问题**：`hooksStore.addHook` 和 `hooksStore.saveHook` 已经更新了 store 中的 hooks 数组（addHook 在头部插入，saveHook 用 map 替换），不需要再调用 `loadRows()` 重新从服务器加载。多余的 API 调用增加了延迟和服务器负担。

**修复**：移除 `saveHook` 成功后的 `await loadRows()` 调用。同样，`confirmDelete` 中 `removeHook` 已从 store 中移除，也不需要 `loadRows()`。

---

## 第二轮问题汇总

| # | 问题 | 严重度 | 类型 |
|---|------|--------|------|
| 13 | HooksPage subtitle 中英文混用 | 低 | 一致性 |
| 14 | confirmDelete 缺少错误处理 | 中 | 功能缺陷 |
| 15 | maximized Dialog 不应使用 app-glass-dialog | 中 | UX 规范 |
| 16 | AgentHooksPanel 错误提示样式不一致 | 低 | UI 规范 |
| 17 | AgentHooksPanel expansion-item 卡片样式不符合主题 | 低 | UX 规范 |
| 18 | detail dialog text-grey-7 暗色不可读 | 中 | 可访问性 |
| 19 | createScopedHook 空标识生成无效 key | 中 | 边界 bug |
| 20 | saveHook 成功后多余 loadRows | 低 | 性能 |
