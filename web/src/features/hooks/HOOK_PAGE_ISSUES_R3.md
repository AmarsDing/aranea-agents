# HOOK/回调 页面问题清单（第三轮）

> 审查日期：2026-05-29
> 审查范围：HooksPage、HookDeliveriesPage、CallbackEditor、HooksTable、AgentHooksPanel 及关联 Store/API/Composable/常量
> 前两轮问题（1-20）均已修复 ✅

---

## 问题总览

| # | 严重度 | 分类 | 问题摘要 |
|---|--------|------|----------|
| 21 | 🔴 高 | i18n | HooksPage 全部硬编码中文，无 i18n key |
| 22 | 🔴 高 | i18n | HookDeliveriesPage 全部硬编码中文，无 i18n key |
| 23 | 🔴 高 | i18n | CallbackEditor 全部硬编码中文，无 i18n key |
| 24 | 🟡 中 | i18n | HooksTable 工具提示硬编码中文 |
| 25 | 🟡 中 | i18n | AgentHooksPanel 硬编码中文 |
| 26 | 🟡 中 | i18n | hookTableUi.ts 列标签硬编码中文 |
| 27 | 🟡 中 | i18n | callbackEditorUi.ts 提示/错误消息硬编码中文 |
| 28 | 🟡 中 | 一致性 | CALLBACK_POINT_OPTIONS / ACTION_TYPE_OPTIONS 英文标签 vs 中文页面混用 |
| 29 | 🟡 中 | 一致性 | LOG_LEVEL_OPTIONS 英文标签 vs 中文页面混用 |
| 30 | 🟡 中 | 一致性 | HookDeliveriesPage statusOptions 内联定义，未共享 |
| 31 | 🟡 中 | i18n | callback/constants.ts PLUGIN_RUN_STATUS_OPTIONS / PLUGIN_RUN_KEY_PRESETS 硬编码中文 |
| 32 | 🟢 低 | 数据流 | api.ts updateHook GET+merge+PUT 存在竞态（已知技术债） |

---

## 问题详情

### 问题 21-27：i18n 全面缺失

**严重度**：🔴 高（影响英文模式可用性）

**影响文件**：
- [HooksPage.vue](file:///f:/aranea-agents/web/src/pages/HooksPage.vue) — kicker/title/subtitle/按钮/Dialog标题/表单标签/通知消息/筛选标签/分页标签
- [HookDeliveriesPage.vue](file:///f:/aranea-agents/web/src/pages/HookDeliveriesPage.vue) — kicker/title/subtitle/按钮/筛选标签/Dialog标题/分页标签/错误消息
- [CallbackEditor.vue](file:///f:/aranea-agents/web/src/components/hooks/CallbackEditor.vue) — 区域标题/字段标签/提示/展开项标签
- [HooksTable.vue](file:///f:/aranea-agents/web/src/components/hooks/HooksTable.vue) — 工具提示
- [AgentHooksPanel.vue](file:///f:/aranea-agents/web/src/components/agents/AgentHooksPanel.vue) — 提示文字/按钮标签/Dialog标题
- [hookTableUi.ts](file:///f:/aranea-agents/web/src/components/hooks/hookTableUi.ts) — 列标签
- [callbackEditorUi.ts](file:///f:/aranea-agents/web/src/components/hooks/callbackEditorUi.ts) — MODIFY_PATCH_HINT/错误消息

**修复方案**：在 i18n 文件中新增 `hooksPage` 子对象，所有硬编码字符串改用 `t()` 调用。

### 问题 28-29：选项标签语言不一致

**严重度**：🟡 中

**现象**：
- `CALLBACK_POINT_OPTIONS`（"Before Agent"等）和 `ACTION_TYPE_OPTIONS`（"Log"/"Notify (Webhook)"等）使用英文标签
- `LOG_LEVEL_OPTIONS`（"debug"/"info"等）使用英文标签
- 但页面其他部分（标题、表单标签、按钮）全部中文
- 切换语言时这些选项标签不会变化

**修复方案**：将选项标签改为 i18n key，在组件中使用 `computed` 动态生成带 `t()` 的选项列表。

### 问题 30：statusOptions 内联定义

**严重度**：🟡 中

**位置**：[HookDeliveriesPage.vue:140-144](file:///f:/aranea-agents/web/src/pages/HookDeliveriesPage.vue#L140-L144)

**修复方案**：移至 `features/hooks/deliveries.ts` 或 `callback/constants.ts` 共享。

### 问题 31：constants.ts 硬编码中文

**严重度**：🟡 中

**位置**：[callback/constants.ts:14-18](file:///f:/aranea-agents/web/src/features/callback/constants.ts#L14-L18)

**现象**：`PLUGIN_RUN_STATUS_OPTIONS`（"成功"/"阻断"/"错误"）和 `PLUGIN_RUN_KEY_PRESETS`（"Hook 规则 (hook:*)"）硬编码中文。

**修复方案**：改为 i18n key 或移至使用方组件内动态生成。

---

## 修复优先级

1. 问题 21-27：i18n 全面化（最大工作量，最高影响）
2. 问题 28-29：选项标签一致性
3. 问题 30-31：常量共享与 i18n 化
4. 问题 32：竞态条件（技术债，暂不修复）
