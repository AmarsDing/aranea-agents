# Channel 管理页面 — 问题审查报告

> 审查日期：2026-05-29
> 审查范围：`web/src/pages/ChannelsPage.vue` 及其关联的所有组件、composable、store、api、types

---

## 一、页面 UI 功能总览

### 1.1 页面结构

| 区域 | 组件 | 功能 |
|------|------|------|
| 顶部 Hero | `ChannelHeroSection` | 页面标题 + "新增 Channel" / "刷新" 按钮 |
| 筛选栏 | `ChannelCatalogFilters` | 搜索框 + 平台类型下拉 + 状态下拉 + 重置/刷新 |
| 列表表格 | `ChannelsTable` | 渠道列表（名称/平台/外部ID/状态/启停/更新时间/操作） |
| 分页 | `AppRegistryPagination` | 分页控制 |
| 运维面板 | `ChannelTurnJobsPanel` + `ChannelDeliveriesPanel` | Turn Job / Delivery 运维数据 |
| 编辑对话框 | `ChannelEditorDialog` | 新增/编辑渠道（平台选择 + 配置表单 + 路由 + 凭据 + 高级 JSON） |

### 1.2 数据流追踪

```
ChannelsPage.vue
  └─ useChannelsPage() (composable)
       ├─ useChannelsStore() → channels[], catalog[], loading
       │    ├─ listChannels() → channelApi.ListChannels → GET /v1/channels
       │    ├─ listChannelCatalog() → channelApi.ListChannelCatalog → GET /v1/channels/catalog
       │    ├─ toggleChannel() → channelApi.ToggleChannel → POST /v1/channels/{id}/toggle
       │    ├─ testChannel() → channelApi.TestChannel → POST /v1/channels/{id}/test
       │    ├─ deleteChannel() → channelApi.DeleteChannel → DELETE /v1/channels/{id}
       │    ├─ listChannelCredentials() → channelApi.ListChannelCredentials → GET /v1/channels/{id}/credentials
       │    ├─ loadTurnJobs() → listChannelTurnJobs() → GET /v1/channels/{id}/turn-jobs (raw HTTP)
       │    └─ loadDeliveries() → channelApi.ListChannelDeliveries → GET /v1/channels/{id}/deliveries
       └─ 本地 ref: search, typeFilter, statusFilter, togglingId, testingId, editorOpen, editingRow, editingCredentials, opsChannelId
```

### 1.3 操作响应追踪

| 操作 | 触发 | 响应链 | 反馈 |
|------|------|--------|------|
| 新增 Channel | Hero "新增" 按钮 | openCreate() → editorOpen=true | 打开编辑对话框 |
| 编辑 Channel | 表格"编辑"按钮 | openEdit(row) → fetchCredentials → editorOpen=true | 打开编辑对话框并加载凭据 |
| 启停 Channel | 表格 Toggle | toggleRow() → store.toggle() → API | notify 正/负 |
| 测试连接 | 表格"测试"按钮 | testRow() → store.testConnection() → API | notify 正/负 + loadAll |
| 复制 Webhook | 表格"链接"按钮 | copyWebhook() → channelWebhookURL() → clipboard | notify 正/负 |
| 删除 Channel | 表格"删除"按钮 | confirmDelete() → $q.dialog → store.removeChannel() | notify 正 |
| 查看 Job/Delivery | 表格"运维"按钮 | openOps() → opsChannelId=id | 展开运维面板 |
| 保存编辑 | 编辑对话框"保存" | save() → persistChannel() → createChannel/updateChannel | notify + 关闭对话框 |
| 保存并测试 | 编辑对话框"保存并测试" | saveAndTest() → persistChannel() + testChannel() | notify + 关闭对话框 |

---

## 二、发现的问题

### BUG-1: `confirmDelete` 未处理 API 错误（严重度：高）

**位置**: `features/channels/useChannelsPage.ts` 第 142-155 行

**描述**: `confirmDelete` 的 `onOk` 回调中调用 `channelsStore.removeChannel(row.id)` 没有 try/catch。如果 API 调用失败，将产生未捕获的 Promise rejection，用户看不到任何错误反馈。

**现状**:
```typescript
}).onOk(async () => {
  await channelsStore.removeChannel(row.id);  // 无错误处理
  if (opsChannelId.value === row.id) { closeOps(); }
  $q.notify({ type: "positive", message: t("channelsPage.deleteOk") });
});
```

**修复**: 添加 try/catch，失败时显示 negative notify。

---

### BUG-2: `onSaved` 直接修改 Store 状态（严重度：中）

**位置**: `features/channels/useChannelsPage.ts` 第 90-94 行

**描述**: `onSaved` 通过 `storeToRefs(channelsStore).channels` 获取的 ref 直接修改数组元素，绕过了 Pinia 的 action 模式。这违反了项目数据流铁律（Store 状态应只通过 action 修改），会导致 Pinia devtools 无法追踪变更。

**现状**:
```typescript
function onSaved(row: ChannelRow) {
  const index = rows.value.findIndex((item) => item.id === row.id);
  if (index >= 0) rows.value[index] = row;
  else rows.value.unshift(row);
}
```

**修复**: 在 Store 中新增 `upsertChannel` action，composable 调用 action 而非直接修改。

---

### BUG-3: `ChannelRow` 类型包含从未填充的字段（严重度：中）

**位置**: `features/channels/types.ts` 第 113 行

**描述**: `ChannelRow = PlatformResource`，而 `PlatformResource` 包含 `is_system: boolean` 和 `capabilities?` 字段。但 `kratosChannelToLegacy()` 从不设置这些字段，导致 `is_system` 为 `undefined`（类型声明为 `boolean`），`capabilities` 为 `undefined`。同样，`useChannelEditorForm.ts` 中 `copyWebhookPreview` 构造 `ChannelRow` 对象时也缺少这些字段。

**影响**: TypeScript 类型不安全；任何依赖 `is_system` 的逻辑会得到 `undefined` 而非 `false`。

**修复**: 在 `kratosChannelToLegacy` 中补充 `is_system: false` 默认值；`copyWebhookPreview` 同理。或为 Channel 创建独立类型而非复用 `PlatformResource`。

---

### BUG-4: `listChannelTurnJobs` 使用原始 HTTP 而非生成客户端（严重度：低，技术债）

**位置**: `features/channels/api.ts` 第 227-236 行

**描述**: `listChannelTurnJobs` 使用 `requestHandler` 直接发 HTTP 请求，绕过了已生成的 `channelApi.ListChannelTurnJobs` 客户端。代码注释标注了 `TECH-DEBT`，但生成的客户端已经包含此方法。`wireChannelTurnJob` 函数使用 `asRecord`/`pickStr` 处理 snake_case/camelCase 双映射，增加了不必要的复杂度。

**修复**: 改用 `channelApi.ListChannelTurnJobs`，与 `listChannelDeliveries` 保持一致的映射模式。

---

### BUG-5: `loadCatalog` 不参与 loading 状态（严重度：低）

**位置**: `stores/channels/index.ts` 第 36-38 行

**描述**: `loadCatalog()` 不设置 `loading` ref，而 `loadChannels()` 设置。`loadAll()` 并行调用两者，但 loading 指示器仅反映 channels 加载状态。如果 catalog 加载更慢，loading 指示器会在 catalog 尚未就绪时消失。

**修复**: 让 `loadCatalog` 也参与 `loading` 状态管理，或在 `loadAll` 中统一管理 loading。

---

### BUG-6: `useChannelEditorForm` 中硬编码中文字符串（严重度：中）

**位置**: `features/channels/useChannelEditorForm.ts` 第 458, 460, 476, 508, 510, 524 行

**描述**: 多处 `$q.notify` 使用硬编码中文字符串而非 i18n key，破坏了国际化支持。

| 行号 | 硬编码字符串 | 应改为 i18n key |
|------|-------------|----------------|
| 458 | `"Channel 已保存"` | `t("channelEditor.saved")` |
| 460 | `"保存失败"` | `t("channelEditor.saveFailed")` |
| 476 | `"保存或测试失败"` | `t("channelEditor.saveOrTestFailed")` |
| 508 | `` `已复制 Webhook URL：${url}` `` | `t("channelEditor.webhookCopied", { url })` |
| 510 | `"复制失败"` | `t("channelEditor.copyFailed")` |
| 524 | `"加载 Agent / Team 列表失败"` | `t("channelEditor.routingLoadFailed")` |

**修复**: 添加对应 i18n key 到 zh-CN.ts 和 en-US.ts，替换硬编码字符串。

---

### BUG-7: 面板组件中硬编码字符串（严重度：低）

**位置**: 多个文件

| 文件 | 行号 | 硬编码 | 应改为 |
|------|------|--------|--------|
| `ChannelTurnJobsPanel.vue` | 29 | `empty-label="暂无内容"` | `:empty-label="t('channelEditor.noContent')"` |
| `ChannelDeliveriesPanel.vue` | 39 | `empty-label="暂无内容"` | `:empty-label="t('channelEditor.noContent')"` |
| `ChannelDeliveriesPanel.vue` | 98 | `"load failed"` | `t("channelEditor.loadFailed")` |
| `useChannelTurnJobsPanel.ts` | 43 | `"load failed"` | `t("channelEditor.loadFailed")` |

**修复**: 添加 i18n key 并替换。

---

### BUG-8: 分页组件 label 硬编码中文（严重度：低）

**位置**: `pages/ChannelsPage.vue` 第 73 行

**描述**: `label="个 Channel"` 硬编码中文，应使用 i18n。

**修复**: 改为 `:label="t('channelsPage.paginationUnit')"`。

---

### BUG-9: `ChannelTurnJob` proto 缺少 `agent_id` / `graph_id` 字段（严重度：低，需后端配合）

**位置**: `api/kratos/channel/v1/channel.proto` 第 162-179 行

**描述**: 后端 `biz.ChannelTurnJob` struct 包含 `AgentID` 和 `GraphID` 字段，但 proto `ChannelTurnJob` message 未定义这些字段。`bizTurnJobToProto` 也不映射它们。前端 Turn Jobs 面板无法显示每个 Job 对应的 Agent，影响排障体验。

**修复**: 需要在 proto 中添加字段并重新生成，不在本次前端修复范围。

---

### BUG-10: `ChannelsTable` 中 q-tooltip 硬编码中文（严重度：中）

**位置**: `components/channels/ChannelsTable.vue` 第 23, 86, 96, 107, 110, 113 行

**描述**: 6 处 `<q-tooltip>` 使用硬编码中文，未走 i18n：

| 行号 | 硬编码 | 应改为 |
|------|--------|--------|
| 23 | `连接正常` | `t('channelsPage.statusConnected')` |
| 86 | `复制 Webhook URL` | `t('channelsPage.copyWebhook')` |
| 96 | `查看 Job / Delivery` | `t('channelsPage.viewOps')` |
| 107 | `测试连接` | `t('channelsPage.testConnection')` |
| 110 | `编辑` | `t('channelsPage.edit')` |
| 113 | `删除` | `t('channelsPage.delete')` |

**修复**: 添加 i18n key 并替换。注意 `ChannelsTable` 是展示组件，需通过 props 传入翻译后的字符串或使用 `useI18n()`。

---

### BUG-11: `channelUi.ts` 中列标题和错误消息硬编码中文（严重度：中）

**位置**: `components/channels/channelUi.ts` 第 115, 124-129, 134-138, 142-146 行

**描述**: `CHANNEL_TABLE_COLUMNS`、`CHANNEL_TURN_JOBS_TABLE_COLUMNS`、`CHANNEL_DELIVERIES_TABLE_COLUMNS` 的列标题全部硬编码中文（"名称"、"平台"、"外部 ID"、"连接状态"、"最近更新" 等）。`copyChannelWebhookURL` 抛出 `"Webhook URL 不可用"` 也是硬编码中文。

**影响**: 
1. 列标题不会随语言切换变化
2. 错误消息无法国际化

**修复**: 列标题改用 i18n key（在组件中动态构建 columns），错误消息改用 i18n。

---

### BUG-12: `channelPlatformFields.ts` 中大量硬编码中文（严重度：高）

**位置**: `features/channels/channelPlatformFields.ts` 多行

**描述**: 以下内容全部硬编码中文，未走 i18n：

| 类别 | 行号 | 内容 |
|------|------|------|
| 飞书区域选项 | 53-54 | `"飞书（国内 open.feishu.cn）"` / `"Lark（国际 open.larksuite.com）"` |
| 字段 hint | 121 | `"开启后走客服 API 主动回复"` |
| 字段 hint | 127 | `"Socket Mode 必填"` |
| 字段 hint | 224-226 | `"允许发消息的用户 ID…"` / `"允许响应的群 chat_id…"` / `"群聊需 @ 机器人才响应…"` |
| 执行模式选项 | 232-234 | `"sync — 同步等待结果"` / `"auto — 按关键词自动路由"` / `"async — 提交后台任务"` |
| 进度模式选项 | 238-240 | `"off — 不展示进度"` / `"text — 文本进度/心跳"` / `"steps — Team 成员步骤摘要"` |
| 字段 placeholder | 250-251 | `"收到，正在处理…"` / `"仍在处理中… {{elapsed}}"` |
| Section hint | 265 | `"实例标识与平台凭据…"` |
| Section hint | 275 | `"Webhook / 长连接接入方式"` / `"长连接接入方式"` |
| Section hint | 283 | `"消息路由与访问控制"` |
| Section hint | 290 | `"长任务 ACK、超时、进度与 async 路由"` |
| 字段 hint | 297 | `"留空使用平台默认图标"` |

**影响**: 这些硬编码字符串直接渲染到 UI 上，切换到英文后仍显示中文。

**修复**: 将所有 label/hint/placeholder 改为 i18n key，在 `useChannelEditorLabels` 中解析。由于 `channelPlatformFields.ts` 是纯数据定义文件（非 Vue 组件），需要将翻译逻辑移到消费端（composable/组件）。

---

### BUG-13: `channelLongTaskPresets.ts` 中 preset label/description 硬编码中文（严重度：中）

**位置**: `features/channels/channelLongTaskPresets.ts` 第 28-94 行

**描述**: 所有 `CHANNEL_LONG_TASK_PRESETS` 的 `label` 和 `description` 字段都是硬编码中文，如 `"飞书 · IM Preview（推荐）"`、`"流式 transcript + 工具/MCP 有序展示"` 等。这些会直接显示在 Long Task Preset 下拉选项中。

**修复**: 改为 i18n key，在 `longTaskPresetOptions` computed 中解析。

---

### BUG-14: `channelLongTaskDefaults.ts` 中 select 选项 label 硬编码中文（严重度：中）

**位置**: `features/channels/channelLongTaskDefaults.ts` 第 39-58 行

**描述**: `TURN_TIMEOUT_OPTIONS`、`FIRST_BYTE_TIMEOUT_OPTIONS`、`PROGRESS_QUIET_OPTIONS` 的 label 全部硬编码中文，如 `"300 秒（5 分钟）"`、`"30 秒（默认）"` 等。

**修复**: 改为 i18n key，在消费端解析。

---

### BUG-15: `jsonError` 函数硬编码中文错误消息（严重度：低）

**位置**: `features/channels/useChannelEditorForm.ts` 第 566 行

**描述**: `jsonError` 函数的 fallback 消息 `"JSON 格式错误"` 是硬编码中文。

```typescript
function jsonError(value: string) {
  try { JSON.parse(value || "{}"); return ""; } catch (err) { return err instanceof Error ? err.message : "JSON 格式错误"; }
}
```

**修复**: 改为 `t("channelEditor.jsonFormatError")`。

---

### BUG-16: `ChannelEditorDialog` 中 `show_secrets` label 硬编码（严重度：低）

**位置**: `features/channels/ChannelEditorDialog.vue` 第 205 行

**描述**: `label="show_secrets"` 是原始字段名而非用户友好的 i18n 标签。

**修复**: 改为 `:label="t('channelEditor.showSecretsLabel')"`。

---

### BUG-17: `ChannelDeliveriesPanel` 刷新按钮使用错误的 i18n key（严重度：低）

**位置**: `features/channels/ChannelDeliveriesPanel.vue` 第 13 行

**描述**: 刷新按钮的 label 使用 `t('channelEditor.turnJobsRefresh')` 而非 deliveries 专用的 key。虽然功能上不影响（两者都是"刷新"），但语义不准确。

**修复**: 添加 `channelEditor.deliveriesRefresh` i18n key 并替换，或统一为 `channelEditor.refresh`。

---

### BUG-18: `ChannelRoutingRulesEditor` 中 targetType 选项硬编码英文（严重度：低）

**位置**: `components/channels/ChannelRoutingRulesEditor.vue` 第 102-105 行

**描述**: `targetTypeOptions` 的 label 硬编码为 `"Agent"` / `"Team"`，未走 i18n。虽然 Agent/Team 是专有名词，但为一致性应走 i18n。

**修复**: 改用 i18n key。

---

## 三、修复优先级

| 优先级 | 问题编号 | 说明 |
|--------|---------|------|
| P0 | BUG-1 | ✅ 已修复：删除操作无错误处理 |
| P1 | BUG-2 | ✅ 已修复：违反数据流铁律 |
| P1 | BUG-3 | ✅ 已修复：类型不安全 |
| P1 | BUG-6 | ✅ 已修复：编辑器硬编码中文 |
| P1 | BUG-12 | ✅ 已修复：channelPlatformFields 大量硬编码中文 |
| P2 | BUG-4 | ✅ 已修复：技术债 |
| P2 | BUG-5 | ✅ 已修复：loading 状态 |
| P2 | BUG-7 | ✅ 已修复：面板组件硬编码 |
| P2 | BUG-8 | ✅ 已修复：分页 label |
| P2 | BUG-10 | ✅ 已修复：ChannelsTable tooltip 硬编码 |
| P2 | BUG-11 | ✅ 已修复：channelUi.ts 列标题硬编码 |
| P2 | BUG-13 | ✅ 已修复：Long Task Preset label 硬编码 |
| P2 | BUG-14 | ✅ 已修复：Long Task Defaults select label 硬编码 |
| P2 | BUG-15 | ✅ 已修复：jsonError 硬编码中文 |
| P2 | BUG-16 | ✅ 已修复：show_secrets label 硬编码 |
| P3 | BUG-9 | ⏳ 需后端配合，本次不修 |
| P3 | BUG-17 | ✅ 已修复：Deliveries 刷新按钮 i18n key 不准确 |
| P3 | BUG-18 | ✅ 已修复：RoutingRules targetType 选项硬编码 |

---

## 四、二次审查新增问题（2026-05-29）

### NEW-1: `AgentChannelRefsSection.vue` 硬编码中文（P1）
**位置**: `pages/agent-settings/AgentChannelRefsSection.vue` 第 14, 19 行
**修复**: ✅ 已修复 — 改用 `t('agentSettings.retry')` 和 `t('agentSettings.noChannelRefs')`

### NEW-2: `useAgentChannelRefs.ts` 硬编码中文（P1）
**位置**: `features/agents/useAgentChannelRefs.ts` 第 23 行
**修复**: ✅ 已修复 — 改用 `t('agentSettings.loadChannelsFailed')`

### NEW-3: `ChannelDeliveriesPanel` agent_id 列显示原始 UUID（P1）
**位置**: `features/channels/ChannelDeliveriesPanel.vue` 第 34 行
**描述**: Deliveries 表格的 agent_id 列直接显示 UUID，用户无法识别对应哪个 Agent
**修复**: ✅ 已修复 — 添加 `agentNameById` 函数，从 Store 加载 agent 列表解析名称

### NEW-4: `copyWebhookPreview` fallback 路径丢失 publicWebhookOrigin（P2）
**位置**: `features/channels/useChannelEditorForm.ts` 第 490-521 行
**描述**: `copyWebhookPreview` 构造临时 `ChannelRow` 对象时 `metadata_json="{}"`，丢失 `publicWebhookOrigin`；且 fallback 路径违反数据流（features→components 依赖）
**修复**: ✅ 已修复 — 移除 fallback 路径，直接使用 `webhookPreview` computed；移除 `channelWebhookURL` import

### NEW-5: `useChannelEditorForm` 违反分层规则 import components 层（P2，技术债）
**位置**: `features/channels/useChannelEditorForm.ts` 第 33 行（已移除）
**描述**: composable import `channelUi.ts`（components 层），违反 features→components 依赖方向
**修复**: ✅ 已修复 — 移除 `channelWebhookURL` import，改用同层 `buildChannelWebhookURL`
**备注**: `useChannelsPage.ts` 和 `useChannelTurnJobsPanel.ts` 仍有同类问题，留作后续迭代
