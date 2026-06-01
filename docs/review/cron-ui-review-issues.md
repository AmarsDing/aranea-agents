# Cron 管理页面审查问题清单

> 审查日期：2026-05-29
> 审查范围：CronTasksPage、CronRunsPage 及其关联的 Store / API / Composable / Components

---

## 一、页面 UI 功能清单与数据来源追踪

### 1.1 CronTasksPage（定时任务）

| UI 组件 | 显示内容 | 数据来源 | 是否正确 |
|---------|---------|---------|---------|
| AppPageHero | 标题"定时任务" | 静态文本 | ✅ |
| 新建任务按钮 | 触发 openCreate | composable | ✅ |
| 搜索框 | 搜索过滤 | composable.search | ✅ |
| 状态筛选 | active/paused/dead | composable.statusFilter | ⚠️ 过滤逻辑有误（见 P1-3） |
| 任务计数 | "共 X 个任务，Y 个启用" | composable.filteredRows/activeCount | ✅ |
| 任务名+Key | name + key | row.name / row.key | ✅ |
| 调度 | scheduleLabel | config_json → parseCronConfig | ✅ |
| 目标 | targetLabel | config_json.target_type + row.agent_id / config.team_id | ✅ |
| 统计 | run_count / success_count / failure_count | metadata_json → parseCronMetadata | ✅ |
| 状态 Chip | enabled ? status : "paused" | row.enabled / row.status | ✅ |
| 执行时间 | last_run_at / next_run_at | metadata_json → parseCronMetadata | ✅ |
| 启用开关 | toggle | row.enabled | ⚠️ 切换时发送全行数据（见 P1-1） |
| 编辑按钮 | 打开编辑 Dialog | composable.openEdit | ✅ |
| 执行历史 | 跳转 CronRunsPage | composable.openRuns | ✅ |
| 立即执行 | triggerCronTask | cronStore.triggerTask | ✅ |
| 重置失败计数 | resetCronTaskFailures | cronStore.resetFailures | ✅ |
| 删除 | confirmDelete | cronStore.removeTask | ⚠️ 无错误处理（见 P1-2） |

### 1.2 CronRunsPage（执行历史）

| UI 组件 | 显示内容 | 数据来源 | 是否正确 |
|---------|---------|---------|---------|
| 任务筛选 | taskId → taskOptions | cronStore.loadTasks | ✅ |
| 结果筛选 | success/failure/skipped/pending | composable.status | ✅ |
| 任务名称+ID | task_name / task_id | WireCronTaskRun.taskName / taskId | ✅ |
| 时间 | started_at / finished_at | WireCronTaskRun | ✅ |
| 结果 Badge | status | WireCronTaskRun.status | ✅ |
| 触发 | trigger | outputJSON → outputJSONExtras | ✅ |
| Agent 运行 | run_id | outputJSON → outputJSONExtras | ✅ |
| 筛选变更 | syncQueryAndLoad | ⚠️ 双重加载（见 P1-4） |

---

## 二、问题列表

### P1（高优先级 - 功能 Bug）

#### P1-1：toggleRow 发送全行数据导致潜在的服务端数据覆盖

**文件**：`web/src/features/cron/useCronTasksPage.ts` → `toggleRow()`

**问题**：`toggleRow` 将 `{ ...row, enabled, status }` 作为 payload 传给 `editTask`，展开整个 `PlatformResource` 对象。`updateCronTask` 中 `payload.config_json ?? cur.configJson` 和 `payload.metadata_json ?? cur.metadataJson` 会使用 payload 中的值（非 null/undefined），覆盖服务端可能已更新的 `metadata_json`（如 run_count、next_run_at 等）。

**影响**：启用/暂停操作可能将服务端已更新的 metadata 覆盖为页面加载时的旧值，导致统计数据丢失或回退。

**修复**：`toggleRow` 只发送变更字段 `{ enabled, status }`，不展开整行。

```typescript
// 修复前
const updated = await cronStore.editTask(row.id, { ...row, enabled, status: enabled ? "active" : "paused" });

// 修复后
const updated = await cronStore.editTask(row.id, { enabled, status: enabled ? "active" : "paused" });
```

---

#### P1-2：confirmDelete 缺少错误处理

**文件**：`web/src/features/cron/useCronTasksPage.ts` → `confirmDelete()`

**问题**：`cronStore.removeTask(row.id)` 的 `await` 在 `.onOk` 回调中，但没有 try/catch。如果删除失败（网络错误、服务端错误），异常未被捕获，用户看不到任何错误提示，且 `rows.value` 也不会被错误地更新。

**影响**：删除失败时用户无反馈，可能误以为删除成功。

**修复**：添加 try/catch 并显示错误通知。

---

#### P1-3：statusFilter 过滤逻辑不正确

**文件**：`web/src/features/cron/useCronTasksPage.ts` → `filteredRows`

**问题**：当前过滤逻辑：
- `"active"` 筛选 `!row.enabled` → 排除未启用的，但 **包含 enabled=true 且 status="dead" 的任务**
- `"paused"` 筛选 `row.enabled` → 排除启用的，但 **排除 enabled=false 且 status="dead" 的任务**

正确逻辑应为：
- `"active"` → `row.enabled && row.status !== "dead"`
- `"paused"` → `!row.enabled && row.status !== "dead"`
- `"dead"` → `row.status === "dead"`（当前逻辑正确）

**影响**：筛选"运行中"时会显示已死亡（dead）但 enabled=true 的任务；筛选"已暂停"时会遗漏已死亡且 disabled 的任务。

**修复**：修正 active/paused 的过滤条件。

---

#### P1-4：CronRunsPage 筛选变更导致双重 API 加载

**文件**：`web/src/features/cron/useCronRunsPage.ts` → `syncQueryAndLoad()`

**问题**：`syncQueryAndLoad` 同时调用 `router.replace()` 和 `loadRuns()`。`router.replace` 更新 URL 后触发 route query watcher，watcher 再次调用 `loadRuns()`。导致每次筛选变更触发两次 API 请求。

**影响**：不必要的网络请求，可能导致列表闪烁。

**修复**：移除 `syncQueryAndLoad` 中的直接 `loadRuns()` 调用，仅依赖 route query watcher 触发加载。

---

### P2（中优先级 - 设计/一致性问题）

#### P2-1：CronTaskFormDialog 未遵循毛玻璃 Dialog 规范

**文件**：`web/src/components/cron/CronTaskFormDialog.vue`

**问题**：Dialog 使用 `app-dialog-card` 但缺少 `app-glass-dialog` class 及其推荐的 DOM 结构（`app-glass-dialog__head`、`app-glass-dialog__scroll`、`app-glass-dialog__body`、`app-glass-dialog__actions`、`app-actions-bar`）。

**影响**：与项目其他 Dialog 风格不一致，缺少毛玻璃效果和标准化的头部/底部布局。

**修复**：按照前端指南 §7 重构 Dialog 结构。

---

#### P2-2：CronTaskFormDialog 手动处理暗色模式

**文件**：`web/src/components/cron/CronTaskFormDialog.vue`

**问题**：Dialog 使用 `:class="{ 'is-dark': $q.dark.isActive }"` 手动切换暗色模式。根据 UX 规范，暗色模式应通过 CSS 变量 + `body.body--dark` 选择器处理，不应手动添加 class。

**影响**：与项目暗色模式规范不一致。

**修复**：移除手动暗色 class，改用 CSS `body.body--dark` 选择器。

---

#### P2-3：wireCronTask 缺少 is_system 字段映射

**文件**：`web/src/features/cron/api.ts` → `wireCronTask()`

**问题**：`PlatformResource` 类型定义了 `is_system: boolean`，但 `wireCronTask` 未设置该字段，导致值为 `undefined`。

**影响**：如果有组件依赖 `is_system` 字段，会出现类型不匹配。

**修复**：在 `wireCronTask` 中添加 `is_system: false`。

---

#### P2-4：CronRunsPage 分页逻辑在页面而非 composable 中

**文件**：`web/src/pages/CronRunsPage.vue`

**问题**：`CronRunsPage` 的分页逻辑（page、pageSize、pageMax、pagedRuns）直接定义在页面的 `<script setup>` 中，而 `CronTasksPage` 的分页逻辑在 composable 中。两者不一致。

**影响**：代码组织不一致，CronRunsPage 的页面脚本偏重。

**修复**：将分页逻辑移入 `useCronRunsPage` composable。

---

#### P2-5：Composable 与 Store 存在双重状态

**文件**：`web/src/features/cron/useCronTasksPage.ts`、`useCronRunsPage.ts`

**问题**：
- `useCronTasksPage` 有本地 `rows` ref，与 `cronStore.tasks` 重复
- `useCronRunsPage` 有本地 `runs` ref 和 `tasks` ref，与 `cronStore.runs` 和 `cronStore.tasks` 重复

两份数据需要手动同步（通过 `onSaved` 等），容易遗漏导致不一致。

**影响**：数据同步脆弱，维护成本高。

**修复**：composable 直接使用 `storeToRefs(cronStore)` 引用 store 状态，消除本地副本。

---

### P3（低优先级 - 小改进）

#### P3-1：表单"名称"字段标签与实际用途不匹配

**文件**：`web/src/components/cron/CronTaskFormFields.vue`

**问题**：`form.name` 字段标签为"名称"，但实际映射为 `payload.key`（任务标识/Key），且有 slug 格式校验。用户可能困惑为什么"名称"只能用小写字母和连字符。

**修复**：将标签改为"标识 *"或"Key *"，更准确反映其用途。

---

#### P3-2：runNow 后 pending 状态不刷新任务列表

**文件**：`web/src/features/cron/useCronTasksPage.ts` → `runNow()`

**问题**：当 `run.status === "pending"` 时，直接跳转到执行历史页面，不刷新当前任务列表。任务列表的 metadata（如 next_run_at）可能已更新但未反映。

**影响**：轻微的数据陈旧，用户返回后需手动刷新。

---

#### P3-3：CronRunsPage 错误消息悬停提示位置不直观

**文件**：`web/src/pages/CronRunsPage.vue`

**问题**：`AppRegistryHoverTip` 包裹任务名称列显示 `error_message`，但用户更可能在状态列寻找错误信息。

**影响**：UX 不够直观，但不影响功能。

---

## 三、修复优先级

| 优先级 | 问题 ID | 修复难度 | 说明 |
|--------|---------|---------|------|
| P1 | P1-1 | 低 | 仅修改 toggleRow payload |
| P1 | P1-2 | 低 | 添加 try/catch |
| P1 | P1-3 | 低 | 修正过滤条件 |
| P1 | P1-4 | 低 | 移除冗余 loadRuns 调用 |
| P2 | P2-1 | 中 | 重构 Dialog 结构 |
| P2 | P2-2 | 低 | 移除手动暗色 class |
| P2 | P2-3 | 低 | 添加字段映射 |
| P2 | P2-4 | 中 | 移动分页逻辑到 composable |
| P2 | P2-5 | 中 | 消除双重状态 |
| P3 | P3-1 | 低 | 修改标签文本 |
| P3 | P3-2 | 低 | 添加 loadAll 调用 |
| P3 | P3-3 | 低 | 调整 HoverTip 位置 |
