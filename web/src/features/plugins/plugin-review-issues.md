# Plugin 管理页面 UI 审查问题清单

> 审查日期: 2026-05-29
> 审查范围: PluginsPage + PluginRunsPage 全链路（Page → Composable → Store → API → 后端）

---

## 问题总览

| # | 严重度 | 文件 | 问题摘要 | 状态 |
|---|--------|------|----------|------|
| 1 | 🔴 Bug | usePluginsPage.ts | 筛选变更时 page>1 触发双次 API 请求 | ✅ 已修复 |
| 2 | 🔴 Bug | usePluginRunsPage.ts | 同上，onFilterChange + watch([page,pageSize]) 双触发 | ✅ 已修复 |
| 3 | 🟡 Bug | PluginDetailDialog.vue | 作用域保存按钮无权限检查 | ✅ 已修复 |
| 4 | 🟡 Bug | usePluginsPage.ts | scope 保存时 agent 模式下空 Agent ID 未前端校验 | ✅ 已修复 |
| 5 | 🟡 UX | PluginConfigDialog.vue | 默认配置/Schema 参考区未用 prettyJSON 格式化 | ✅ 已修复 |
| 6 | 🟡 UX | PluginsPage.vue | Callback 筛选器为文本输入，与 PluginRunsPage 不一致 | ✅ 已修复 |
| 7 | 🟡 UX | PluginDetailDialog.vue | bumpSort 按钮无 loading 状态 | ✅ 已修复 |
| 8 | 🟢 UX | PluginDetailDialog.vue | "最近状态"指标无颜色标记 | ✅ 已修复 |
| 9 | 🟢 UX | usePluginRunsPage.ts | 筛选变更不同步到 URL query | ✅ 已修复 |
| 10 | 🟢 UX | usePluginRunsPage.ts | applyRouteQuery 未处理 from/to 参数 | ✅ 已修复 |
| 11 | 🔴 Bug | PluginSchemaForm.vue | 展示组件内使用 useQuasar() + $q.notify 违反红线 | 🔲 待修 |
| 12 | 🔴 Bug | ModelRouterRulesEditor.vue | 同上，展示组件内使用 useQuasar() + $q.notify | 🔲 待修 |
| 13 | 🟡 Bug | PluginRunsPage.vue | 运行详情 Dialog 缺少 app-glass-dialog 类 | 🔲 待修 |
| 14 | 🟡 Bug | PluginRunsPage.vue | 错误 Banner 用 bg-negative 而非 app-page-error-banner | 🔲 待修 |
| 15 | 🟡 Bug | PluginSchemaForm.vue | integer 类型字段用 Number() 转换会丢失精度且未取整 | 🔲 待修 |
| 16 | 🟡 Bug | usePluginsPage.ts | openDetail 解析 scope 逻辑不严谨 | 🔲 待修 |
| 17 | 🟡 Bug | PluginSchemaForm.vue | setValue 对 null/undefined 值处理不当 | 🔲 待修 |
| 18 | 🟢 UX | ModelRouterRulesEditor.vue | addRule 立即 emitRules 发出空规则 | 🔲 待修 |
| 19 | 🟢 UX | PluginDetailDialog.vue | Callback 空数组时 .length 访问可能报错 | 🔲 待修 |
| 20 | 🟢 UX | usePluginRunsPage.ts | from/to 日期格式不兼容 datetime-local input | 🔲 待修 |

---

## 第二轮深度审查发现的问题详情

### 问题 11: PluginSchemaForm 展示组件内使用 $q.notify（红线 D1/B4）

**文件**: `web/src/components/plugins/PluginSchemaForm.vue`

**现状**:
```ts
const $q = useQuasar();
// ...
function setJSONField(key: string, text: string) {
  try {
    const parsed = JSON.parse(text || "[]");
    setValue(key, parsed);
  } catch {
    $q.notify({ type: "warning", message: "JSON 格式错误" });  // ← 红线违规
  }
}
```

**违反红线**: D1（展示组件禁止使用 Store/API）+ B4（$q.notify 应在 Composable/Store）

**修复方案**: 移除 `$q.notify`，改为 emit 事件让父组件处理通知

---

### 问题 12: ModelRouterRulesEditor 展示组件内使用 $q.notify（红线 D1/B4）

**文件**: `web/src/components/plugins/ModelRouterRulesEditor.vue`

**现状**:
```ts
const $q = useQuasar();
// ...
function emitRules() {
  if (rules.value.some((rule) => regexError(rule))) {
    $q.notify({ type: "warning", message: "正则表达式有误，规则未保存" });  // ← 红线违规
    return;
  }
  // ...
}
```

**违反红线**: 同问题 11

**修复方案**: 移除 `$q.notify`，正则校验失败时不 emit，由父组件检测变化来提示

---

### 问题 13: 运行详情 Dialog 缺少 app-glass-dialog 类

**文件**: `web/src/pages/PluginRunsPage.vue`

**现状**:
```html
<q-card class="app-dialog-card app-dialog-card--sm">
```

PluginDetailDialog 和 PluginConfigDialog 都有 `app-glass-dialog` 类，运行详情 Dialog 缺失

**修复方案**: 补充 `app-glass-dialog` 类

---

### 问题 14: 错误 Banner 样式不一致

**文件**: `web/src/pages/PluginRunsPage.vue`

**现状**:
```html
<q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
```

PluginsPage 使用 `app-page-error-banner` 公共类，PluginRunsPage 使用内联样式

**修复方案**: 统一使用 `app-page-error-banner` 类

---

### 问题 15: integer 类型字段用 Number() 转换会丢失精度

**文件**: `web/src/components/plugins/PluginSchemaForm.vue`

**现状**:
```html
<q-input
  v-else-if="def.type === 'number' || def.type === 'integer'"
  type="number"
  @update:model-value="setValue(key, Number($event))"
/>
```

`def.type === 'integer'` 时应使用 `Math.round(Number($event))` 保证整数

**修复方案**: integer 类型取整后再 setValue

---

### 问题 16: openDetail 解析 scope 逻辑不严谨

**文件**: `web/src/features/plugins/usePluginsPage.ts`

**现状**:
```ts
function openDetail(plugin: Plugin) {
  scopeMode.value = plugin.scope && plugin.scope !== "global" ? "agent" : "global";
  scopeAgentId.value = scopeMode.value === "agent" ? plugin.scope : "";
}
```

`plugin.scope` 为空字符串时 `plugin.scope && plugin.scope !== "global"` 结果为 `""` (falsy)，虽然最终结果正确但逻辑不清晰

**修复方案**: 使用更明确的判断 `const isGlobal = !plugin.scope || plugin.scope === "global";`

---

### 问题 17: setValue 对 null/undefined 值处理不当

**文件**: `web/src/components/plugins/PluginSchemaForm.vue`

**现状**:
```ts
function setValue(key: string, val: unknown) {
  const next = { ...data.value, [key]: val };
  emit("update:modelValue", JSON.stringify(next, null, 2));
}
```

当 `val` 为 `undefined` 时，`JSON.stringify` 会跳过该 key，导致字段从配置中消失而非设为 null

**修复方案**: 将 undefined 转为 null 再设置

---

### 问题 18: addRule 立即 emitRules 发出空规则

**文件**: `web/src/components/plugins/ModelRouterRulesEditor.vue`

**现状**:
```ts
function addRule() {
  rules.value.push({ id: newRuleId(), model: "", contains: [], regex: "", min_chars: 0, priority: 0 });
  emitRules();  // ← 立即发出空规则到父组件
}
```

添加空规则后立即 emit，父组件 configText 中会写入空规则对象

**修复方案**: 添加规则时不立即 emit，等用户填写内容后再 emit

---

### 问题 19: Callback 空数组时 .length 访问可能报错

**文件**: `web/src/components/plugins/PluginDetailDialog.vue`

**现状**:
```html
<span v-if="!target.callback_points.length" class="text-grey-7">暂无 Callback</span>
```

`target.callback_points` 可能为 undefined（虽然类型定义为 `string[]`，但 API 返回可能缺失），访问 `.length` 会 TypeError

**修复方案**: 使用 `!target.callback_points?.length` 可选链

---

### 问题 20: from/to 日期格式不兼容 datetime-local input

**文件**: `web/src/features/plugins/usePluginRunsPage.ts`

**现状**: `from`/`to` 绑定 `<q-input type="datetime-local">`，该 input 需要 `YYYY-MM-DDTHH:mm` 格式。但 `applyRouteQuery` 从 URL 恢复时，存的是 ISO 字符串（`2026-05-29T15:38:00.000Z`），格式不兼容

**修复方案**: `applyRouteQuery` 中将 ISO 字符串转为 `datetime-local` 兼容格式（截取前 16 字符）
