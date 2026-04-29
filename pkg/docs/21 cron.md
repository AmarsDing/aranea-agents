# 定时任务（Quasar UI + 数据库字段）

本文档描述 **定时任务** 列表页、**创建/编辑** 对话框的 Quasar 组件映射，以及建议的 **数据表字段** 与 API 契约。产品定位：**安排定期 Agent 任务**（与 Agent、通道等模块关联时在接口层携带 `agent_id` / `channel_id` 等）。

---

## 1. 路由与页面骨架

| 路由（示例） | 说明 |
|--------------|------|
| `/cron` 或 `/scheduled-tasks` | 定时任务管理主页 |
| `/cron/runs` 或 `/scheduled-tasks/runs` | **执行历史**（可按 `cron_task_id` 查询参数默认筛选） |

| 区域 | Quasar / 布局 |
|------|----------------|
| 页容器 | **`QPage`** `class` 与主题一致|
| 顶栏 | **`QToolbar`** 或 `div.row.items-center.q-mb-md`：左侧标题区，右侧按钮 |
| 标题 | **`div.text-h5`**「定时任务」 |
| 副标题 | **`div.text-caption text-grey`**「安排定期 Agent 任务」 |
| 刷新 | **`QBtn`** `outline` `icon="refresh"` 或 `icon="sym_o_refresh"`，文案「刷新」 |
| 新建 | **`QBtn`** `unelevated`  `icon="add"`「新建任务」 |

---

## 2. 列表页

### 2.1 搜索与筛选

| 元素 | Quasar |
|------|--------|
| 搜索框 | **`QInput`** `outlined` `rounded` `dense` `debounce="300"`，`prepend` 插槽 **`QIcon`** `name="search"`，`placeholder="搜索定时任务..."`；`v-model` 绑定 `search`，`@update:model-value` 或 watch 触发列表 `reload` |

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
| **名称** | `name` |
| **描述** | `description`；单行截断，**`QTooltip`** 悬停展示全文；空则「—」 |
| **计划** | 根据 `schedule_type` 渲染：`每隔 N 分钟` / `cron: 0 * * * *` / `一次 · 2026-04-22 09:00` |
| **Agent** | `agent_id` 为空时显示「默认」；有则显示 **`agent_display_name`** |
| **执行次数** | `run_count`（累计触发次数，含成功+失败；与 §4.1 字段一致） |
| **成功次数** | `success_count`；可用 **`QBadge`** `color="positive"` 或默认正文 |
| **失败次数** | `failure_count`；`failure_count === 0` 时仅展示数字；`> 0` 时 **`class="text-negative"`** 或 `QBadge` `color="negative"`。**交互见下** |
| **状态** | **`QBadge`** / **`QChip`**：`active` 绿、`paused` 灰、`failed` 红 |
| **上次运行** | `last_run_at` 相对时间或本地时间 |
| **下次运行** | `next_run_at`；`schedule_type=once` 且已执行可为「—」 |
| **操作** | **启用/暂停** `QToggle`；**编辑** / **删除** `QBtn` `flat` `dense`；可选 **历史** `QBtn` `icon="history"` → 同失败次数点击跳转（见下） |

**失败次数列的交互**（`failure_count > 0` 时）：

| 行为 | 实现 |
|------|------|
| **鼠标移入** | 使用 **`QTooltip`** `delay="300"` `max-width="320px"`：展示 **最近若干条失败记录**摘要（来自列表接口内嵌的 `recent_failures[]` 或悬停时 **`GET /cron-tasks/:id/runs?status=failure&limit=5`**）。每条建议一行：`started_at` + `error_message` 截断（多行可用 **`QList`** `dense` 放在 tooltip 插槽内）。无额外请求时优先用列表响应中的 `recent_failures` 以降低抖动。 |
| **点击** | **`router.push`**（或 **`QBtn`** `flat` `dense` 包一层）：目标 **`/cron/runs?cron_task_id=<id>`**（或项目统一 query 名），**执行历史页**加载时读取 query，**默认按该定时任务筛选**（`cron_task_id` 预填且可清除后查看全部）。 |

`failure_count === 0`：无 tooltip 或 tooltip 文案「暂无失败记录」；点击可禁用或仍跳转历史页（筛选该任务且结果为空）。

分页：服务端分页时 **`QTable`** `@request` + **`QPagination`**。

### 2.3.1 执行历史页（与失败点击联动）

| 区域 | 说明 |
|------|------|
| **入口** | 侧栏「执行历史」；或列表 **失败次数** / **操作 · 历史** |
| **默认筛选** | URL **`?cron_task_id=`** 时，筛选器 **`QSelect`** 或只读 Chip 显示当前任务名称，表格仅展示该任务产生的 **`cron_task_run`** |
| **筛选器** | 定时任务（可清空=全部）、结果 `success`/`failure`、时间范围 |
| **表格列** | 任务名称、`started_at`、`finished_at`、`status`、`error_message` 摘要、`trigger`；可 **跳转 Agent 运行**（若有 `run_id`） |

### 2.4 Quasar 映射（列表）

| 区域 | 组件 |
|------|------|
| 列表主体 | `QTable` 或 `QList` |
| 描述列 | `body-cell-description`：`ellipsis` + `QTooltip` |
| 失败次数列 | `QBtn` 或 `span` + `cursor-pointer` + `QTooltip`；`@click` → `$router.push` |
| 空态 | `QIcon` + 文案 + `QBtn` |
| 加载 | `QInnerLoading` |

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
| **描述（可选）** | **`QInput`** `outlined` `label="描述"` `autogrow` 或固定行数；落库 `description` |
| **消息** | **`QInput`** `type="textarea"` `autogrow` `label="消息"` `placeholder="Agent 应该做什么?"` |

### 3.2.1 当前工程实现映射

当前后端已有通用资源接口与 SQLite 表 `cron_task`，物理表字段为 `task_key/name/description/status/enabled/agent_id/config_json/metadata_json`。因此前端表单字段按以下方式保存：

| UI / 逻辑字段 | 当前保存位置 |
|---------------|--------------|
| `name`（slug） | `cron_task.task_key`（前端类型中为 `key`） |
| 展示名称 | `cron_task.name` |
| `description` | `cron_task.description` |
| `agent_id` | `cron_task.agent_id`（目标为 Agent 时） |
| `target_type`、`team_id`、`schedule_type`、`cron_expression`、`interval_seconds`、`run_at`、`timezone`、`message` | `config_json` |
| `run_count`、`success_count`、`failure_count`、`last_run_at`、`last_run_status`、`last_error`、`next_run_at`、`recent_failures[]` | `metadata_json`，执行历史页也可从 `cron_task_run` 查询 |
| 启用 / 暂停 | `enabled` + `status`（`active` / `paused`） |

这使本期不需要迁移主表即可完成 UI 与 CRUD；后续若调度器需要高频查询，可再把 `next_run_at`、计数字段提升为物理列。

### 3.3 底部操作

| 按钮 | Quasar |
|------|--------|
| 取消 | **`QBtn`** `flat` `label="取消"` `v-close-popup` |
| 创建 | **`QBtn`** `unelevated` `color="orange"` `label="创建"` ；`:disable` 绑定表单 `invalid`；提交 `POST` / `PATCH` |

表单外层可用 **`QForm`** `@submit.prevent`，统一触发表单校验。

---

## 4. 数据库字段（建议表名：`cron_task` 或 `scheduled_task`）

### 4.1 主表 `cron_task`

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | uuid | PK | |
| `tenant_id` | uuid | 索引 | 多租户 |
| `name` | varchar(128) | UNIQUE(`tenant_id`, `name`) | 小写字母、数字、连字符；与 UI hint 一致 |
| `description` | text | nullable | 列表副行、备注 |
| `agent_id` | uuid | nullable, FK → agent | 空表示「默认」Agent 策略由运行时解析 |
| `schedule_type` | varchar(16) | NOT NULL | `interval` \| `cron` \| `once` |
| `cron_expression` | varchar(64) | nullable | `schedule_type=cron` 时必填；标准 5 段 |
| `interval_seconds` | int | nullable | `schedule_type=interval` 时必填；如 `900` = 15 分钟 |
| `run_at` | timestamptz | nullable | `schedule_type=once` 时必填；单次触发时间 |
| `timezone` | varchar(64) | default `'UTC'` 或 `'Asia/Shanghai'` | Cron/一次任务解释时区 |
| `message` | text | NOT NULL | 下发给 Agent 的指令/用户消息模板 |
| `status` | varchar(16) | NOT NULL | `active` \| `paused`；失败不删任务时可加 `failed` |
| `last_run_at` | timestamptz | nullable | 上次实际开始或结束时间（产品定一种） |
| `last_run_status` | varchar(16) | nullable | `success` \| `failure` \| `skipped` |
| `last_error` | text | nullable | 失败摘要 |
| `next_run_at` | timestamptz | nullable | 调度器维护，列表展示 |
| `run_count` | bigint | NOT NULL, default 0 | 累计执行次数（每次触发 +1） |
| `success_count` | bigint | NOT NULL, default 0 | 成功次数 |
| `failure_count` | bigint | NOT NULL, default 0 | 失败次数；与 `cron_task_run` 可定期对账 |
| `created_at` | timestamptz | | |
| `updated_at` | timestamptz | | |
| `deleted_at` | timestamptz | nullable | 软删 |

**校验规则（应用层或 DB check）**：

- `schedule_type = 'cron'` → `cron_expression` 非空，`interval_seconds` / `run_at` 为空。
- `schedule_type = 'interval'` → `interval_seconds` > 0。
- `schedule_type = 'once'` → `run_at` 非空；执行完毕后可将 `status` 置 `paused` 或标记 `completed`（若增加该状态）。

### 4.2 可选：`cron_task_run`（执行历史）

便于与 **`18 monitor.md`** 活动日志互补。

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | uuid | PK |
| `cron_task_id` | uuid | FK |
| `started_at` | timestamptz | |
| `finished_at` | timestamptz | nullable |
| `status` | varchar(16) | `success` \| `failure` |
| `trigger` | varchar(32) | `schedule` \| `manual` |
| `run_id` | uuid | nullable | 关联 Agent  |
| `error_message` | text | nullable |

**说明**：列表 tooltip 所需的「最近失败」可由 **`GET /cron-tasks`** 每条内嵌 `recent_failures`（最多 5 条）返回；或在悬停时请求 **`GET /cron-task-runs?cron_task_id=&status=failure&limit=5`**。

---

## 5. API 摘要

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/cron-tasks?search=&status=&page=` | 列表；支持 `search` 匹配 `name` / `description`；每条含 `run_count`、`success_count`、`failure_count`，可选 `recent_failures[]` |
| POST | `/cron-tasks` | 创建；服务端校验计划字段互斥 |
| GET | `/cron-tasks/:id` | 详情 |
| PATCH | `/cron-tasks/:id` | 更新名称、计划、消息、`status` |
| DELETE | `/cron-tasks/:id` | 软删 |
| POST | `/cron-tasks/:id/trigger` | 立即执行一次（可选） |
| GET | `/cron-task-runs?cron_task_id=&status=&from=&to=&page=` | 执行历史列表；**从列表点击失败次数**进入时带 `cron_task_id` 做默认筛选 |

调度器服务根据 `cron_task` 计算并回写 `next_run_at`，触发时消费 `message`（+ `payload`）启动 Agent 运行；每次运行结束更新 **`run_count` / `success_count` / `failure_count`**（或由异步任务根据 `cron_task_run` 汇总回写）。

### 5.1 Cron 调动 Agent / Team 的执行设计

Cron 不直接实现 Agent 或 Team 的运行逻辑，而是复用现有 **`ChatService.Send`** 作为统一入口：

1. **调度器扫描任务**：后台 runner 定期读取 `cron_task`，筛选 `enabled=true`、`status=active`、`metadata_json.next_run_at <= now` 的任务。
2. **解析目标**：`config_json.target_type=agent` 时使用 `cron_task.agent_id`；`target_type=team` 时使用 `config_json.team_id`。
3. **创建执行记录**：先写入 `cron_task_run`，状态为 `pending`，`started_at=now`，`output_json.trigger=schedule`。
4. **创建 Session**：
   - Agent：`owner_type=agent`、`agent_id=<agent_id>`、`dialog_mode=cron`。
   - Team：`owner_type=team`、`team_id=<team_id>`、`dialog_mode=cron`。
5. **调用 ChatService**：
   - Agent：`SendMessageInput{SessionID, AgentKey, Content: message}`。
   - Team：`SendMessageInput{SessionID, TeamID, Content: message}`；`ChatService.Send` 会因 session owner 为 `team` 自动进入 `sendTeam(...)`。
6. **写回结果**：
   - 成功：`cron_task_run.status=success`，`output_json` 写入 `session_id` / `message_id`，`metadata_json.success_count++`。
   - 失败：`cron_task_run.status=failure`，写入 `error_message`，`metadata_json.failure_count++`、`last_error`、`recent_failures[]`。
   - 所有结果都更新 `run_count`、`last_run_at`、`last_run_status` 并重新计算 `next_run_at`；`once` 任务执行后自动暂停。

### 5.2 本期实施范围

- `/cron`：专用定时任务管理页，替代通用 `ResourceManagerPage`。
- `/cron/runs`：执行历史页，支持 `cron_task_id` 与 `status` 筛选。
- `GET /cron-task-runs`：读取已有 `cron_task_run` 表，返回最近运行记录。
- 创建 / 编辑 / 删除 / 启停：继续使用 `/cron-tasks` 通用资源 CRUD。
- 后端 Cron runner：定时扫描到期任务，调动 Agent 或 Team，并回写执行历史与统计字段。

---

## 6. 与设计稿对照

| 设计稿元素 | 文档章节 |
|------------|----------|
| 标题「定时任务」、副标题、刷新、橙色新建 | §1 |
| 圆角搜索框 | §2.1 |
| 时钟空态 + 文案 + 新建 | §2.2 |
| 创建弹窗：名称、Agent、计划类型三选一、Cron、消息、取消/创建 | §3 |

---

*文档版本：1.3 — 增加 Cron 调动 Agent / Team 的执行设计，并将后端 Cron runner 纳入本期实施。*
