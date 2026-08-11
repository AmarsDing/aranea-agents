# 模型管理页面问题清单

> 审查日期：2026-05-29（第一轮）、2026-05-29（第二轮深入审查）、2026-05-29（第三轮 UI 功能审查）、2026-05-29（第四轮深度下沉审查）
> 审查范围：ResourceManagerPage.vue 及其所有子组件、composable、store、API 层、后端合并逻辑

---

## 第一轮审查（问题 1-6）

### 问题 1（严重）：ProviderHAConfig v-model 导致高可用配置丢失 ✅ 已修复

**文件**：[ResourceManagerPage.vue](file:///f:/aranea-agents/web/src/pages/ResourceManagerPage.vue#L507-L510)

**现象**：用户在编辑对话框的「高可用」步骤中修改 HA 模式、候选模型或 Hedge 延迟后，点击保存，这些修改不会被写入 config_json。

**根因**：`providerHAForm` 是 `reactive()` 对象，`ProviderHAConfig` 通过 `emit('update:modelValue', {...})` 发出新普通对象，`v-model` 替换了引用，composable 内部持有的仍是旧 reactive 引用。

**修复**：改为 `:model-value` + `@update:model-value` + `updateHAForm` 函数用 `Object.assign` 合并。

---

### 问题 2（中等）：hasPricingConfigured 两个版本字段不一致 ✅ 已修复

**修复**：在 `pricingWarning.ts` 中补充 `embeddingPrice` 和 `cacheWritePrice` 参数。

---

### 问题 3（中等）：showPricingWarning 不必要的单位转换 ✅ 已修复

**修复**：移除 `effectiveMicroPrice` 中间函数，直接传 USD/1M 值。

---

### 问题 4（低）：providerForm 中 micro 价格字段为死代码 ✅ 已修复

**修复**：从 4 个文件中移除 5 个 micro 字段。

---

### 问题 5（低）：缺少 cache_write_usd_per_1m 的 UI 输入 ✅ 已修复

**修复**：添加输入框并在 `hasPricingConfigured` 中补充。

---

### 问题 6（低）：ProviderHAConfig 候选模型 Provider 类型选项语义混淆 ✅ 已修复

**文件**：[useProviderWizard.ts](file:///f:/aranea-agents/web/src/features/platform/useProviderWizard.ts)

**现象**：HA 候选模型的 Provider 类型下拉框使用 `providerTypeFilterOptions`，与列表页过滤共享同一变量名，语义混淆。

**修复**：新增 `haCandidateProviderTypeOptions` 独立变量，HA 配置组件使用新变量，列表过滤继续使用 `providerTypeFilterOptions`。

---

## 第二轮深入审查（问题 7-9）

### 问题 7（高）：toggleEnabled 展开整行导致 config_json 被旧值覆盖 ✅ 已修复

**文件**：[useProviderList.ts](file:///f:/aranea-agents/web/src/features/platform/useProviderList.ts#L121-L129)

**现象**：在模型列表中切换「启用/禁用」开关时，如果后端在此期间更新了 config_json（如用量统计回写 `usage_call_count_30d`、`model_hotness_score` 等），toggle 操作会用页面加载时的旧 config_json 覆盖新值。

**根因**：`{ ...row, enabled }` 将整行数据（含 `config_json`）作为 payload 发送，旧 config_json 优先于服务器当前值。

**修复**：只发送 `{ enabled }` 字段，让 `mergeProviderModel` 从服务器当前状态取其余字段。

---

### 问题 8（高）：ProviderForm 类型在 3 个文件中重复定义 ✅ 已修复

**修复**：将 `ProviderForm` 提取到 [types.ts](file:///f:/aranea-agents/web/src/features/platform/types.ts)，三个文件统一 `import type { ProviderForm } from "./types"`。

---

### 问题 9（低）：TECH-DEBT — useProviderCatalog 直接调 API 绕过 Store ✅ 已标注

**修复**：添加 `TECH-DEBT` 标注注释，说明当前保留原因和长期方案。

---

## 第三轮 UI 功能审查（问题 10-16）

### 问题 10（高）：removeRow 无错误处理 ✅ 已修复

**修复**：添加 try/catch，失败时显示 `$q.notify({ type: "negative", ... })`。

---

### 问题 11（高）：loadRows 无错误处理 ✅ 已修复

**修复**：添加 catch 块，失败时显示 `$q.notify({ type: "negative", ... })`。

---

### 问题 12（高）：toggleEnabled 无错误通知 ✅ 已修复

**修复**：添加 catch 块，失败时显示 `$q.notify({ type: "negative", ... })`。

---

### 问题 13（中）：saveProviderRow 无 catch ✅ 已修复

**修复**：添加 catch 块 + `errorMessage` 导入。

---

### 问题 14（中）：saveRow（非 Provider）无 catch ✅ 已修复

**修复**：添加 catch 块 + `errorMessage` 导入。

---

### 问题 15（低）：openEdit 无错误处理 ✅ 已修复

**修复**：添加 try/catch，失败时显示 warning 通知但仍打开对话框。

---

### 问题 16（低）：HA 候选 API Key 无可见性切换 ✅ 已修复

**修复**：为每个 HA 候选 API Key 输入框添加 visibility 切换按钮。

---

## 第四轮深度下沉审查（问题 17-19）

### 问题 17（高）：useProviderTrendDialog.loadOverview 无错误处理 ✅ 已修复

**文件**：[useProviderTrendDialog.ts](file:///f:/aranea-agents/web/src/features/usage/useProviderTrendDialog.ts)

**现象**：趋势弹窗加载用量数据 API 失败时，`loadOverview` 只有 `try/finally` 无 `catch`。用户看到加载动画消失，弹窗内容为空，但没有任何错误通知。

**根因**：`loadOverview` 的 `try/finally` 结构不包含 catch 块，`usageStore.fetchOverview` 抛出的错误成为 unhandled rejection。

**数据追踪**：
```
ProviderTrendDialog 打开 → watch(open, row) → loadOverview()
  → usageStore.fetchOverview({ range, provider_code, model_api_id })
  → 失败 → unhandled rejection → loading 重置 → 弹窗空内容无提示
```

**修复**：添加 catch 块 + `$q.notify({ type: "negative", ... })`。新增 `useQuasar` 和 `errorMessage` 导入。

---

### 问题 18（高）：ProviderHAConfig.removeCandidate 可见性状态索引错位 ✅ 已修复

**文件**：[ProviderHAConfig.vue](file:///f:/aranea-agents/web/src/components/platform/ProviderHAConfig.vue)

**现象**：当 HA 候选列表有 3 个以上条目时，删除中间的候选后，后续候选的 API Key 可见性状态与实际不匹配。例如：候选 [A, B, C]，B 的 Key 可见（`visibleCandidateKeys[1] = true`），删除 A 后列表变为 [B, C]，B 现在在索引 0 但 `visibleCandidateKeys[0]` 为 undefined（不可见），C 在索引 1 但 `visibleCandidateKeys[1]` 仍为 true（错误可见）。

**根因**：`removeCandidate` 只删除了被移除索引的可见性状态（`delete visibleCandidateKeys[idx]`），但候选数组移位后，后续索引的可见性状态没有跟随调整。

**修复**：删除候选时清空所有可见性状态（`Object.keys(visibleCandidateKeys).forEach(k => delete visibleCandidateKeys[Number(k)])`）。用户可重新切换可见性，避免复杂的索引重映射逻辑。

---

### 问题 19（低）：ProviderTrendDialog 缺少 app-glass-dialog + scroll wrapper ✅ 已修复

**文件**：[ProviderTrendDialog.vue](file:///f:/aranea-agents/web/src/components/platform/ProviderTrendDialog.vue)

**现象**：趋势弹窗的 `q-card` 只有 `app-dialog-card` 没有 `app-glass-dialog` class，且 body 内容缺少 `app-glass-dialog__scroll` wrapper。根据 UX 规范，有头 + 可滚动内容 + 底栏结构的弹窗应使用 `app-glass-dialog` + `app-glass-dialog__scroll`。

**影响**：弹窗缺少毛玻璃效果，长内容可能无法正确滚动。

**修复**：
1. 添加 `app-glass-dialog` class 到 `q-card`
2. 在 `q-separator` 和 `q-card-section` 之间添加 `<div class="app-glass-dialog__scroll">` wrapper

---

## 深入审查确认项（无问题）

以下功能经深入追踪确认数据流正确、操作响应正确：

| 功能模块 | 审查结果 |
|----------|----------|
| 向导步骤1：目录选择模式（供应商搜索、选择、模型选择） | ✅ 数据流正确 |
| 向导步骤1：自定义模式（预设选择、手动配置） | ✅ 数据流正确 |
| 向导步骤1：运行时类型锁定/解锁逻辑 | ✅ 条件渲染正确 |
| 向导步骤1：API 密钥显示/隐藏 + 服务端凭据揭示 | ✅ 数据流正确 |
| 向导步骤1：Provider ID 验证规则 | ✅ 规则生效 |
| 向导步骤1：身份变更检测 + 检查按钮 | ✅ 逻辑正确 |
| 向导步骤2：模型分类、规格、评级 | ✅ 双向绑定正确 |
| 向导步骤2：目录能力标签（只读展示） | ✅ 数据来源正确 |
| 向导步骤2：6 个价格输入 + 2 个定价警告 banner | ✅ 数据和条件正确 |
| 向导步骤3：HA 模式/候选/延迟 | ✅ emit + Object.assign 正确 |
| 向导步骤3：HA 候选 API Key 可见性切换 | ✅ 已修复（问题16+18） |
| 向导步骤4：条件 Toggle（按 provider_type/variant） | ✅ 条件渲染正确 |
| 向导步骤4：条件字段值在切换 provider_type 后保留 | ⚠️ 已知行为，非 bug |
| 向导导航：步骤跳转（允许跳到任意步骤） | ✅ UX 选择 |
| 向导保存：canSubmitNewProviderModel 守卫 | ✅ 逻辑完整 |
| 向导保存：buildProviderPayload 构建 | ✅ 字段映射正确 |
| 向导保存：后端 mergeConfigJSONForUpdate 保留凭据 | ✅ 后端合并逻辑正确 |
| 向导保存：API 失败错误通知 | ✅ 已修复（问题13-14） |
| 趋势弹窗：KPI 卡片 + 图表 + 详情 | ✅ 数据来源正确 |
| 趋势弹窗：指标切换触发重新加载 | ✅ watch 触发正确 |
| 趋势弹窗：API 失败错误通知 | ✅ 已修复（问题17） |
| 趋势弹窗：毛玻璃 + 滚动 wrapper | ✅ 已修复（问题19） |
| 列表表格：所有列的数据来源和格式化 | ✅ 数据正确 |
| 列表表格：密钥揭示/隐藏 | ✅ 状态管理正确 |
| 列表操作：toggleEnabled 错误通知 | ✅ 已修复（问题12） |
| 列表操作：删除错误通知 | ✅ 已修复（问题10） |
| 列表加载：错误通知 | ✅ 已修复（问题11） |
| 列表过滤：关键词 + Provider 类型 | ✅ 过滤逻辑正确 |
| 分页：页码/每页条数/总数 | ✅ 计算正确 |
| 编辑对话框：openEdit 错误处理 | ✅ 已修复（问题15） |
| 非Provider资源：简单表单 CRUD | ✅ 数据流正确 |
| 凭据加密状态 banner | ✅ 条件判断正确 |
| ProviderModelsTable：AppRegistryTable + registryCol | ✅ 规范合规 |
| ProviderModelsTable：纯 props/emits | ✅ 无 Store/API import |
| ProviderHAConfig：纯 props/emits | ✅ 无 Store/API import |
| ProviderTrendDialog：纯 props/emits | ✅ 无 Store/API import |
| ResourceManagerPage script 行数 | ✅ ~107 行（<200） |

---

## 第五轮 Dogfood 实测审查（问题 20-22 + 观察项）

> 审查日期：2026-08-11。方式：Playwright 浏览器实测模型注册表页面（搜索/筛选/开关/编辑弹窗）。

### 问题 20（高）：搜索框输入后不过滤 ✅ 已修复

**文件**：[llm_provider_model.proto](file:///f:/aranea-agents/api/kratos/llm_provider_model/v1/llm_provider_model.proto)

**现象**：列表搜索框输入关键词后结果不变，请求是不带任何 query 的裸 GET。

**根因**：proto 中 `ListProviderModels` 声明为 `google.protobuf.Empty` 入参，生成的 TS 客户端丢弃 `page/pageSize/search`，服务端只能走 `searchQueryFromContext` 旁路且永远读不到值。

**修复**：proto 新增 `ListProviderModelsRequest{page, page_size, search}`，`make api` 重生成 Go + TS；service 改为从请求消息读取参数（空分页参数时保留 legacy 全量路径供选择器/健康检查）。回归测试：`internal/service/llm_provider_model_list_test.go`、`web/src/features/platform/__tests__/llmProviderModelListQuery.spec.ts`。

---

### 问题 21（低）：「Provider 类型」筛选下拉 label 截断 ✅ 已修复

**文件**：[_form-layout.sass](file:///f:/aranea-agents/web/src/css/theme/_form-layout.sass)

**现象**：工具栏 Provider 类型筛选下拉在空选时内容塌缩，通用 `min-width: 120px` 截断 label。

**修复**：追加 `> .q-select.provider-control { min-width: 168px }` 规则。

---

### 问题 22（高）：切换启用开关后 30 天用量统计列清零 ✅ 已修复

**文件**：[llm_provider_model.go](file:///f:/aranea-agents/internal/biz/llm_provider_model.go)

**现象**：toggle 启用/禁用后，「30 天调用/热度」等统计列变成「—」，刷新页面才恢复。

**根因**：前端用 PATCH 响应整行替换列表行；`List`/`ListPaged` 会经 `statsInjector` 注入 `usage_*_30d` 统计到 config_json，但 `Update` 返回的是未装饰数据，统计字段被抹掉。

**修复**：`Usecase.Update` 返回前同样经 `statsInjector.InjectStats` 装饰（statsInjector 为 nil 时跳过，保持兼容）。回归测试：`internal/biz/llm_provider_model_update_stats_test.go`。

---

### 观察项：价格输入框显示 float32 加宽噪声长小数 ✅ 已修复

**现象**：编辑弹窗价格偶发显示 `0.14000000059604645`。

**根因**：上游 models.dev 价格为 float32，加宽到 float64 后产生尾数噪声。

**修复**：价格粒度为 micro-USD/1K（USD/1M 下 6 位小数以外恒为噪声），两侧统一归一化——后端 `normalizeUSDPer1M`（catalog 同步写入 config_json 前，`internal/modelregistry/pricing.go`），前端 `normalizeUsdPer1M`（加载进编辑表单前，`providerRuntimeOverlay.ts` + `useProviderCredentials.ts`）。各有单测覆盖。

---

## 已知限制（暂不修复）

1. **HA 候选 API Key 无服务端 `api_key_set` 标记**：编辑已有 HA 候选 API Key 的 Provider 时，Key 字段为空且无 "••••••" 占位符或图标提示 Key 已存在。后端 `mergeHACandidateSecrets` 按 name 匹配正确保留 Key，但用户无法感知 Key 是否已配置。需后端在 API 响应中增加 `api_key_set` 标记。

2. **HA 候选 name 变更风险**：如果用户修改了 HA 候选的 `name` 字段，后端 `mergeHACandidateSecrets` 的 name 匹配会失败，回退到索引匹配可能导致 Key 分配到错误候选。建议后端增加候选 ID 字段。

3. **条件 Toggle 值跨 provider_type 保留**：切换 provider_type 后，被隐藏的 Toggle 值（如 `optimize_for_cache`）仍保留在 form 中，保存时写入 config_json。后端运行时应忽略不相关字段。
