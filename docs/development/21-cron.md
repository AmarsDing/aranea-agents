# 定时任务（Quasar UI + 用户视角）

本文档描述 **定时任务** 列表页、**创建/编辑** 对话框、**执行历史** 弹窗的用户视角交互规格与功能需求。产品定位：**安排定期 Agent 任务**（与 Agent、通道等模块关联时在接口层携带 `agent_id` / `channel_id` 等）。

> **内容边界**：本文档只描述用户故事、功能需求、验收标准、非功能需求、交互规格。
> - 数据模型 / Proto 契约 / API 实现 / 代码分层 → 见 [21-cron.design.md](./21-cron.design.md)
> - 模块定位 / 代码锚点 / 现状评估 / 任务清单 / 改动文件 → 见 [21-cron.development.md](./21-cron.development.md)

---

## 1. 路由与页面骨架

| 路由 | 说明 |
|--------------|------|
| `/cron` | 定时任务管理主页（含执行历史弹窗入口） |

| 区域 | Quasar / 布局 |
|------|----------------|
| 页容器 | **`QPage`** `class` 与主题一致|
| 顶栏 | **`QToolbar`** 或 `div.row.items-center.q-mb-md`：左侧标题区，右侧按钮 |
| 标题 | **`div.text-h5`**「定时任务」 |
| 副标题 | **`div.text-caption text-grey`**「安排定期 Agent 任务」 |
| 刷新 | **`QBtn`** `outline` `icon="refresh"` 或 `icon="sym_o_refresh"`，文案「刷新」 |
| 新建 | **`QBtn`** `unelevated`  `icon="add"`「新建任务」 |
| 执行历史 | **`QBtn`** `flat` `icon="history"`「执行历史」→ 打开 §4 弹窗（不预选任务） |

---

## 2. 列表页

### 2.1 搜索与筛选

| 元素 | Quasar |
|------|--------|
| 搜索框 | **`QInput`** `outlined` `rounded` `dense` `debounce="300"`，`prepend` 插槽 **`QIcon`** `name="search"`，`placeholder="搜索定时任务..."`；`v-model` 绑定 `search`，`@update:model-value` 或 watch 触发列表 `reload` |
| 状态筛选 | **`QSelect`** `outlined` `dense` `clearable` `emit-value` `map-options`，选项 `active`/`paused`/`dead` |
| 重置 | **`QBtn`** `flat` `icon="restart_alt"`「重置」清空搜索与筛选 |

### 2.2 空状态（无数据时）

与截图一致：居中列布局。

| 元素 | 实现 |
|------|------|
| 容器 | **`div.column.flex.flex-center`**，上下 `q-pa-xl` |
| 图标 | **`QIcon`** `name="schedule"` 或 `alarm`，外包 **`QAvatar`** `size="80px"` `color="grey-9"` / `text-grey-5` |
| 主文案 | **`div.text-h6`**「暂无定时任务」 |
| 次文案 | **`div.text-body2 text-grey`**「创建定时任务以安排定期 Agent 任务。」 |
| 主按钮 | 同顶栏 **`QBtn`** orange「+ 新建任务」→ 打开 §3 对话框 |

有数据时：本项目实现采用 **`QTable`**，因为列包含执行次数、成功/失败、上/下次运行和行内操作，信息密度高；移动端后续可再拆成 `QList + QItem`。

### 2.3 表格列（有数据后）

| 列 | 说明 |
|----|------|
| **名称** | `name` 主行，副行 `task_key` |
| **描述** | `description`；单行截断，**`QTooltip`** 悬停展示全文；空则「—」 |
| **计划** | 根据 `schedule_type` 渲染：`每隔 N 分钟` / `cron: 0 * * * *` / `一次 · 2026-04-22 09:00` |
| **Agent** | `agent_id` 为空时显示「默认」；有则显示 **`agent_display_name`** |
| **执行次数** | `run_count`（累计触发次数，含成功+失败） |
| **成功次数** | `success_count`；可用 **`QBadge`** `color="positive"` 或默认正文 |
| **失败次数** | `failure_count`；`failure_count === 0` 时仅展示数字；`> 0` 时 **`class="text-negative"`** 或 `QBadge` `color="negative"`。**交互见下** |
| **状态** | **`QBadge`** / **`QChip`**：`active` 绿、`paused` 灰、`dead` 红 |
| **上次运行** | `last_run_at` 相对时间或本地时间 |
| **下次运行** | `next_run_at`；`schedule_type=once` 且已执行可为「—」 |
| **操作** | **启用/暂停** `QToggle`；**编辑** / **删除** `QBtn` `flat` `dense`；可选 **历史** `QBtn` `icon="history"` → 同失败次数点击跳转（见下）；`dead` 任务显示 **重置失败计数** `QBtn` `icon="restart_alt"` |

**失败次数列的交互**（`failure_count > 0` 时）：

| 行为 | 实现 |
|------|------|
| **鼠标移入** | 使用 **`QTooltip`** `delay="300"` `max-width="320px"`：展示 **最近若干条失败记录**摘要（来自列表响应内嵌的 `recent_failures[]`，最多 5 条）。每条建议一行：`started_at` + `error_message` 截断（多行可用 **`QList`** `dense` 放在 tooltip 插槽内）。 |
| **点击** | **`router.push`**（或 **`QBtn`** `flat` `dense` 包一层）：打开 §4 执行历史弹窗，**默认按该定时任务筛选**（`cron_task_id` 预填且可清除后查看全部）。 |

`failure_count === 0`：无 tooltip 或 tooltip 文案「暂无失败记录」；点击可禁用或仍打开历史弹窗（筛选该任务且结果为空）。

分页：当前为前端客户端分页与搜索（列表接口无 search/page 参数）。

---

## 3. 创建 / 编辑定时任务对话框

### 3.1 容器

| 元素 | Quasar |
|------|--------|
| 弹层 | **`QDialog`** `persistent`（可选，避免误关丢数据） |
| 卡片 | **`QCard`** 深色表面、`style="min-width: 420px; max-width: 560px"` |
| 标题栏 | **`QCardSection`** `row items-center`：左侧 **`div.text-h6`**「创建定时任务」/「编辑定时任务」，右侧 **`QBtn`** `flat` `round` `icon="close"` `v-close-popup` |

### 3.2 表单字段

| 字段 | 组件与行为 |
|------|------------|
| **名称** | **`QInput`** `outlined` `label="名称"` `hint="仅小写字母、数字和连字符"`；校验：`/^[a-z0-9]+(-[a-z0-9]+)*$/`；`placeholder="my-daily-task"` |
| **目标类型** | **`QBtnToggle`**：`agent`「Agent」/ `team`「Team」；决定下方选择器与后端执行分支 |
| **Agent ID（目标为 Agent）** | **`QSelect`** `outlined` `clearable` `emit-value` `map-options`；选项含 `{ label: '默认', value: null }` + `GET /agents` 列表；展示 Agent 显示名，值为 `agent_id` |
| **Team ID（目标为 Team）** | **`QSelect`** `outlined` `emit-value` `map-options`；选项来自 `GET /teams`；展示 Team 显示名，值为 `team_id` |
| **计划类型** | **`QBtnToggle`** `spread` `no-caps` `toggle-color="orange"` `color="grey-9"` `text-color="white"`；选项：`interval`「每隔」、`cron`「Cron」、`once`「一次」 |
| **每隔** | 当 `schedule_type === 'interval'`： **`QInput`** `type="number"` 或 **`QSelect`**（如 5/15/30/60 分钟）+ 单位文案「分钟」+ 可以自己填写数字 |
| **Cron 表达式** | 当 `schedule_type === 'cron'`：**`QInput`** `outlined` `label="Cron 表达式"` `hint="标准 5 字段 cron: 分 时 日 月 周"` `placeholder="0 * * * *"`；可选服务端校验 |
| **执行时间（一次）** | 当 `schedule_type === 'once'`：**`QInput`** + **`QPopupProxy`** 包 **`QDate`** + **`QTime`**，或项目统一日期时间组件；绑定 `run_at`（ISO 本地） |
| **描述（可选）** | **`QInput`** `outlined` `label="描述"` `autogrow` 或固定行数 |
| **消息** | **`QInput`** `type="textarea"` `autogrow` `label="消息"` `placeholder="Agent 应该做什么?"` |
| **最大重试次数** | **`QInput`** `type="number"` `outlined` `label="最大重试次数"` `hint="0=禁用重试，默认3"`；绑定 `config_json.retry_max_attempts` |
| **启用** | **`QToggle`** 默认启用 |

### 3.3 底部操作

| 按钮 | Quasar |
|------|--------|
| 取消 | **`QBtn`** `flat` `label="取消"` `v-close-popup` |
| 创建 | **`QBtn`** `unelevated` `color="orange"` `label="创建"` ；`:disable` 绑定表单 `invalid`；提交 `POST` / `PATCH` |

表单外层可用 **`QForm`** `@submit.prevent`，统一触发表单校验。

---

## 4. 执行历史弹窗

> **交互说明**：执行历史以弹窗（`CronRunsDialog`）形式嵌入定时任务管理页，而非独立路由页面。

| 区域 | 说明 |
|------|------|
| **入口** | 顶栏「执行历史」按钮（不预选任务）/ 列表 **失败次数** / **操作 · 历史** 按钮（预选当前任务） |
| **默认筛选** | 从失败次数列打开时传入 `cron_task_id`，筛选器 **`QSelect`** 或只读 Chip 显示当前任务名称，表格仅展示该任务产生的运行记录 |
| **筛选器** | 定时任务（可清空=全部）、结果 `success`/`failure`/`pending`/`skipped` |
| **表格列** | 任务名称、`started_at`、`finished_at`、`status`、`error_message` 摘要、`trigger`；可 **跳转 Agent 运行**（若有 `run_id`） |
| **分页** | 前端分页（当前 `ListCronTaskRuns` 仅支持 `limit`，无 offset） |

---

## 5. 用户故事

### US-1：安排定期 Agent 任务
**作为** 平台管理员，
**我希望** 创建定时任务，按 interval/cron/once 三种计划自动触发 Agent 执行，
**以便** 无需人工干预即可完成日报、巡检、数据汇总等周期性工作。

**验收标准**：
- 三种计划类型可选且互斥
- `cron` 必填 cron 表达式（5 段）
- `interval` 必填间隔秒数 > 0
- `once` 必填执行时间
- 创建后立即可在列表看到，并按计划自动执行

### US-2：查看执行历史与失败原因
**作为** 平台管理员，
**我希望** 在列表中看到失败次数，悬停查看最近失败摘要，点击进入执行历史弹窗，
**以便** 快速定位失败原因。

**验收标准**：
- 失败次数 > 0 时显示红色，悬停展示最近 5 条失败摘要
- 点击失败次数打开历史弹窗，默认筛选该任务
- 历史弹窗可按状态、任务筛选
- 每条历史包含 started_at/finished_at/status/error_message/trigger

### US-3：手动触发任务
**作为** 平台管理员，
**我希望** 立即触发一次任务执行（不等待到下次计划时间），
**以便** 验证任务配置或临时执行。

**验收标准**：
- 列表操作列有「立即执行」按钮
- 点击后立即返回 `pending` 状态的 run，后台异步执行
- 手动触发不推进 `next_run_at`，不让 `once` 任务自动暂停
- 手动失败不计入死信连续失败计数

### US-4：失败重试与死信保护
**作为** 平台管理员，
**我希望** 任务失败时自动重试（指数退避），连续失败 3 次后进入死信状态停止调度，
**以便** 防止失控故障。

**验收标准**：
- 默认重试 3 次（30s/2m/10m 退避）
- `retry_max_attempts=0` 禁用重试
- 连续失败 ≥3 次 → `status=dead`、`enabled=false`、发出 `cron.dead_letter` 告警事件
- 死信任务不再调度，直到手动重置

### US-5：重置失败计数
**作为** 平台管理员，
**我希望** 对死信任务一键重置失败计数并恢复调度，
**以便** 在修复问题后重新启用任务。

**验收标准**：
- `dead` 状态任务操作列显示「重置失败计数」按钮
- 点击后清零 `failure_count`/`last_error`/`recent_failures`，`status=active`、`enabled=true`

### US-6：暂停与恢复
**作为** 平台管理员，
**我希望** 暂停任务（不删除）并随时恢复，
**以便** 临时停止调度。

**验收标准**：
- 列表操作列有启用/暂停 `QToggle`
- 暂停后 `status=paused`，调度器跳过该任务
- 恢复后 `status=active`，按计划继续调度

---

## 6. 非功能需求

| 项 | 要求 |
|----|------|
| **可用性** | 调度器单实例运行，进程重启后从 `metadata_json.next_run_at` 恢复调度 |
| **可观测性** | Prometheus 指标：`aranea_cron_job_runs_total`、`aranea_cron_job_duration_seconds`、`aranea_cron_job_dead_total`；死信事件 `cron.dead_letter` |
| **可配置性** | `CRON_RUNNER_INTERVAL`（默认 1m）、`CRON_RUNNER_DISABLED=1` 关闭调度、`CRON_CHAT_DISPATCH_ORIGIN` 保留 HTTP fallback |
| **并发安全** | 调度器 `TryLock` 防重入；per-task 互斥锁避免同任务双跑；Session 忙时返回 `skipped` 不计失败 |
| **数据一致性** | `metadata_json` 写入前 reload 任务避免 lost update；`once` 任务执行后自动暂停 |
| **多租户** | 当前未强制 `tenant_id` 隔离（P3 待补） |

---

## 7. 与设计稿对照

| 设计稿元素 | 文档章节 |
|------------|----------|
| 标题「定时任务」、副标题、刷新、橙色新建 | §1 |
| 圆角搜索框 | §2.1 |
| 时钟空态 + 文案 + 新建 | §2.2 |
| 创建弹窗：名称、Agent、计划类型三选一、Cron、消息、取消/创建 | §3 |
| 执行历史弹窗 | §4 |

---

*文档版本：2.0 — 按三件套内容边界重组：迁移数据模型/API 契约/执行设计/运维指南到设计文档，迁移实施范围到开发计划，本文档聚焦用户视角。*
