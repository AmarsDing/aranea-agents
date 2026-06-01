# 系统设置页面问题清单

> 审查日期：2026-05-29
> 审查范围：`SystemSettingsPage.vue` + `SystemSettingsCatalogTab.vue` 及其数据流全链路

---

## 问题总览

| # | 严重度 | 分类 | 问题摘要 |
|---|--------|------|----------|
| 1 | 🔴 高 | 数据正确性 | Knowledge Embed base_url 无法清空——后端 `ApplyEmbedPatch` 忽略空值 |
| 2 | 🔴 高 | i18n | 评估 LLM 区域全部硬编码中文，无 i18n key |
| 3 | 🟡 中 | i18n | 模型目录 Tab 几乎全部硬编码中文，无 i18n key |
| 4 | 🟡 中 | i18n | `settingsPage.kicker` / `settingsPage.subtitle` 无 i18n key，依赖 fallback |
| 5 | 🟡 中 | 数据流 | `evalLLMToPatch()` 是死代码——`api.ts` 内联映射且未 trim |
| 6 | 🟡 中 | 数据流 | `SystemSettingsCatalogTab` 直接 import API 函数，违反前端红线 #12 |
| 7 | 🟢 低 | 死代码 | Store `saveSettings()` 方法仅保存 4 字段，从未被调用 |
| 8 | 🟢 低 | 死代码 | `useSystemSettingsPage` composable 从未被页面使用 |

---

## 问题详情

### 问题 1：Knowledge Embed base_url 无法清空

**严重度**：🔴 高
**分类**：数据正确性
**影响文件**：
- 后端：[knowledge.go:310-329](file:///f:/aranea-agents/internal/biz/knowledge/knowledge.go#L310-L329)（`ApplyEmbedPatch`）
- 前端：[knowledge-embed.ts](file:///f:/aranea-agents/web/src/features/system-settings/knowledge-embed.ts)

**现象**：
用户在 Knowledge Embedder 区域清空 Base URL 字段后保存，旧值依然存在，无法清除。

**根因**：
后端 `ApplyEmbedPatch` 使用"非零覆盖"策略：
```go
if b := strings.TrimRight(strings.TrimSpace(baseURL), "/"); b != "" {
    out.BaseURL = b
}
```
当 `baseURL` 为空字符串时，条件不满足，旧值被保留。同样 `model` 字段也有此问题（但 model 有默认值，实际不会为空）。

**修复方案**：
前端 `knowledgeEmbedToPatch` 增加 `clearBaseUrl` / `clearModel` 标记，或后端改为"始终覆盖"策略（与 `HTTPProxy` 一致）。推荐后端方案——让 `baseURL` 和 `model` 始终覆盖（空值表示清空），因为这两个字段语义上允许为空。

---

### 问题 2：评估 LLM 区域硬编码中文

**严重度**：🔴 高
**分类**：i18n
**影响文件**：[SystemSettingsPage.vue:143-161](file:///f:/aranea-agents/web/src/pages/SystemSettingsPage.vue#L143-L161)

**现象**：
评估 LLM 区域的标题、提示文字、字段标签、状态文案全部硬编码中文，切换英文后仍显示中文。

**硬编码字符串列表**：
- `评估 LLM（UserSim / Judge）`（标题）
- `持久化到 system_settings；运行时 env（KRATOS_EVAL_SIM_* / KRATOS_EVAL_JUDGE_*）优先。Judge 未填时回退 Sim。`（提示）
- `UserSim Provider` / `UserSim Model` / `Judge Provider（可选）` / `Judge Model（可选）`（标签）
- `评估 LLM 已配置`（状态文案）

**修复方案**：
在 `zh-CN.ts` 和 `en-US.ts` 的 `settingsPage` 下新增 `evalLLM` 子对象，包含上述所有字符串的 i18n key，页面改用 `t()` 调用。

---

### 问题 3：模型目录 Tab 硬编码中文

**严重度**：🟡 中
**分类**：i18n
**影响文件**：[SystemSettingsCatalogTab.vue](file:///f:/aranea-agents/web/src/pages/SystemSettingsCatalogTab.vue)

**现象**：
`SystemSettingsCatalogTab` 中几乎所有用户可见文字都是硬编码中文，包括：
- 区域标题：`模型目录（models.dev）`、`更新策略`、`Provider 命名对齐`、`Catalog 浏览`、`Catalog JSON 浏览`、`更新日志`
- 状态标签：`状态`、`已加载`、`未加载`、`上次同步`、`本地路径`
- 按钮标签：`保存策略`、`立即同步`、`Dry Run`、`刷新`、`预览影响`、`立即对齐`
- 提示文案：`无待迁移绑定`、`无匹配 Provider`、`暂无同步日志` 等
- 字段标签：`数据源 URL`、`同步策略`、`间隔（小时）`、`自动应用到 DB`
- 下拉选项：`关闭`、`定时`、`仅更新本地 JSON`、`元数据 + 定价` 等

**修复方案**：
在 i18n 文件中新增 `catalogTab` 子对象，所有硬编码字符串改用 `t()` 调用。

---

### 问题 4：settingsPage.kicker / subtitle 无 i18n key

**严重度**：🟡 中
**分类**：i18n
**影响文件**：[SystemSettingsPage.vue:6-8](file:///f:/aranea-agents/web/src/pages/SystemSettingsPage.vue#L6-L8)

**现象**：
```vue
:kicker="t('settingsPage.kicker', 'System')"
:subtitle="t('settingsPage.subtitle', '全局路径、A2A、配额与嵌入模型配置。')"
```
`settingsPage.kicker` 和 `settingsPage.subtitle` 在 i18n 文件中不存在，始终使用 fallback 默认值。切换英文时 subtitle 仍显示中文。

**修复方案**：
在 `zh-CN.ts` 和 `en-US.ts` 的 `settingsPage` 下新增 `kicker` 和 `subtitle` key。

---

### 问题 5：evalLLMToPatch() 是死代码

**严重度**：🟡 中
**分类**：数据流
**影响文件**：
- [eval-llm.ts:26-32](file:///f:/aranea-agents/web/src/features/system-settings/eval-llm.ts#L26-L32)（死代码）
- [api.ts:83-86](file:///f:/aranea-agents/web/src/features/system-settings/api.ts#L83-L86)（内联映射）
- [SystemSettingsPage.vue:328](file:///f:/aranea-agents/web/src/pages/SystemSettingsPage.vue#L328)（调用处）

**现象**：
`evalLLMToPatch()` 函数已定义但从未被调用。`api.ts` 的 `updateSystemSettings` 直接内联映射 `evalLLM?.simProvider ?? ""` 等，且未做 `.trim()`。而 `knowledgeEmbedToPatch` 和 `webResearchToPatch` 都被正确使用。

此外 `evalLLMToPatch` 返回类型为 `{ evalSimProvider, evalSimModel, ... }`（proto 风格），与 `EvalLLMForm`（`{ simProvider, simModel, ... }`）不兼容，无法直接传入 `saveAll`。

**修复方案**：
1. 删除 `evalLLMToPatch` 函数（类型不兼容且从未使用）
2. 在 `api.ts` 的 `updateSystemSettings` 中为 evalLLM 字段添加 `.trim()` 处理，保持与 `knowledgeEmbedToPatch` / `webResearchToPatch` 一致

---

### 问题 6：SystemSettingsCatalogTab 直接 import API 函数

**严重度**：🟡 中
**分类**：数据流（违反前端红线 #12）
**影响文件**：[SystemSettingsCatalogTab.vue:237-249](file:///f:/aranea-agents/web/src/pages/SystemSettingsCatalogTab.vue#L237-L249)

**现象**：
`SystemSettingsCatalogTab` 直接 import 了 `features/model-catalog/api.ts` 中的 10+ 个 API 函数并直接调用，违反前端红线 #12："Page 不得直接 `import` `features/*/api`"。

**修复方案**：
创建 `stores/model-catalog/index.ts` Store，将 API 调用收敛到 Store actions 中，页面通过 Store 间接访问。此为架构改进，优先级可降后。

---

### 问题 7：Store saveSettings() 是死代码

**严重度**：🟢 低
**分类**：死代码
**影响文件**：[stores/system-settings/index.ts:24-31](file:///f:/aranea-agents/web/src/stores/system-settings/index.ts#L24-L31)

**现象**：
`saveSettings(rootDirectory, workDirectory)` 方法仅保存 4 个字段（rootDirectory、workDirectory、globalMonthlyMicroUsd、a2aPublicBaseUrl），缺少 mcpAllowAdhocHttp、knowledgeEmbed、evalLLM、webResearch。且从未被任何页面调用（页面使用 `saveAll`）。

**修复方案**：
删除 `saveSettings` 方法（死代码即删，红线 #12）。

---

### 问题 8：useSystemSettingsPage composable 是死代码

**严重度**：🟢 低
**分类**：死代码
**影响文件**：[useSystemSettingsPage.ts](file:///f:/aranea-agents/web/src/features/system-settings/useSystemSettingsPage.ts)

**现象**：
`useSystemSettingsPage()` composable 仅返回 `{ settingsStore, a2aStore }`，但 `SystemSettingsPage.vue` 直接 import 了两个 Store，从未使用此 composable。

**修复方案**：
删除 `useSystemSettingsPage.ts` 文件（死代码即删）。

---

## 修复优先级

1. **问题 1**（🔴 数据正确性）→ 后端 `ApplyEmbedPatch` 改为始终覆盖
2. **问题 2**（🔴 i18n）→ 评估 LLM 区域 i18n 化
3. **问题 4**（🟡 i18n）→ 补充 kicker/subtitle i18n key
4. **问题 5**（🟡 数据流）→ 删除死代码 + api.ts 添加 trim
5. **问题 7**（🟢 死代码）→ 删除 `saveSettings`
6. **问题 8**（🟢 死代码）→ 删除 `useSystemSettingsPage.ts`
7. **问题 3**（🟡 i18n）→ 模型目录 Tab i18n 化（工作量较大）
8. **问题 6**（🟡 架构）→ 创建 model-catalog Store（架构改进，可后续处理）
