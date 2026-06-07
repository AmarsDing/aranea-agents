# Cron 定时任务模块 — 实现设计文档

> 对应需求：`21 cron.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

定时任务调度：Cron 表达式/间隔/单次触发 Agent/Team 执行、任务管理、执行日志。支持三种计划类型（interval/cron/once），通过 `config_json` 存储计划详情，通过 `metadata_json` 存储运行统计。

---

## 二、Proto 层

### 2.1 现有 Proto

文件：`api/kratos/cron/v1/cron.proto`

```protobuf
message CronTask {
  string id = 1;
  string task_key = 2;
  string name = 3;
  string description = 4;
  string status = 5;
  bool enabled = 6;
  int32 sort_order = 7;
  string agent_id = 8;
  string config_json = 9;
  string metadata_json = 10;
  string created_at = 11;
  string updated_at = 12;
  string deleted_at = 13;
}

message CronTaskRun {
  string id = 1;
  string task_id = 2;
  string task_name = 3;
  string status = 4;
  string started_at = 5;
  string finished_at = 6;
  string trigger = 7;
  string run_id = 8;
  string output_json = 9;
  string error_message = 10;
  string created_at = 11;
}

message ListCronTasksResponse {
  repeated CronTask items = 1;
}

message CreateCronTaskRequest {
  string task_key = 1 [(google.api.field_behavior) = REQUIRED];
  string name = 2 [(google.api.field_behavior) = REQUIRED];
  string description = 3;
  string status = 4;
  bool enabled = 5;
  int32 sort_order = 6;
  string agent_id = 7;
  string config_json = 8;
  string metadata_json = 9;
}

message UpdateCronTaskRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  CronTask task = 2;
}

message ListCronTaskRunsRequest {
  string cron_task_id = 1;
  string status = 2;
  int32 limit = 3;
}

message ListCronTaskRunsResponse {
  repeated CronTaskRun items = 1;
}

service CronService {
  rpc ListCronTasks(google.protobuf.Empty) returns (ListCronTasksResponse) {
    option (google.api.http) = {get: "/v1/cron-tasks"};
  }
  rpc CreateCronTask(CreateCronTaskRequest) returns (CronTask) {
    option (google.api.http) = {post: "/v1/cron-tasks" body: "*"};
  }
  rpc GetCronTask(GetCronTaskRequest) returns (CronTask) {
    option (google.api.http) = {get: "/v1/cron-tasks/{id}"};
  }
  rpc UpdateCronTask(UpdateCronTaskRequest) returns (CronTask) {
    option (google.api.http) = {patch: "/v1/cron-tasks/{id}" body: "task"};
  }
  rpc DeleteCronTask(DeleteCronTaskRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {delete: "/v1/cron-tasks/{id}"};
  }
  rpc ListCronTaskRuns(ListCronTaskRunsRequest) returns (ListCronTaskRunsResponse) {
    option (google.api.http) = {get: "/v1/cron-task-runs"};
  }
  rpc TriggerCronTask(TriggerCronTaskRequest) returns (CronTaskRun) {
    option (google.api.http) = {
      post: "/v1/cron-tasks/{id}/trigger"
      body: "*"
    };
  }
  rpc ResetCronTaskFailures(ResetCronTaskFailuresRequest) returns (CronTask) {
    option (google.api.http) = {
      post: "/v1/cron-tasks/{id}/reset-failures"
      body: "*"
    };
  }
}

message TriggerCronTaskRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ResetCronTaskFailuresRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}
```

### 2.2 config_json 结构

`config_json` 存储计划详情，前端写入、后端调度器读取：

```json
{
  "target_type": "agent",
  "team_id": "",
  "schedule_type": "cron",
  "cron_expression": "0 * * * *",
  "interval_seconds": 0,
  "run_at": "",
  "timezone": "Asia/Shanghai",
  "message": "执行每日数据汇总",
  "retry_max_attempts": 3
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `target_type` | string | `agent` / `team` / `model_registry_sync` |
| `team_id` | string | target_type=team 时必填 |
| `schedule_type` | string | `interval` / `cron` / `once` |
| `cron_expression` | string | schedule_type=cron 时必填，标准5段 |
| `interval_seconds` | int | schedule_type=interval 时必填 |
| `run_at` | string | schedule_type=once 时必填，ISO时间 |
| `timezone` | string | 默认 `Asia/Shanghai` |
| `message` | string | 下发给 Agent/Team 的消息 |
| `retry_max_attempts` | int | 首次失败后的重试次数；`0`=禁用；未设置默认 `3`（完整 30s/2m/10m 退避） |

### 2.3 metadata_json 结构

`metadata_json` 存储运行统计，后端调度器写入、前端读取：

```json
{
  "run_count": 10,
  "success_count": 8,
  "failure_count": 2,
  "last_run_at": "2026-05-14T10:00:00Z",
  "last_run_status": "success",
  "last_error": "",
  "next_run_at": "2026-05-14T11:00:00Z",
  "recent_failures": [
    {"started_at": "2026-05-13T09:00:00Z", "error_message": "agent not found"}
  ]
}
```

---

## 三、Biz 层

### 3.1 领域模型

文件：`internal/biz/cron.go`

```go
type CronTask struct {
    ID           string
    TaskKey      string
    Name         string
    Description  string
    Status       string  // "active" / "paused" / "dead" / "deleted"
    Enabled      bool
    SortOrder    int
    AgentID      string
    ConfigJSON   string
    MetadataJSON string
    CreatedAt    string
    UpdatedAt    string
    DeletedAt    string
}

type CronTaskRun struct {
    ID           string
    TaskID       string
    TaskName     string
    Status       string  // "success" / "failure" / "pending" / "skipped"
    StartedAt    string
    FinishedAt   string
    OutputJSON   string
    ErrorMessage string
    CreatedAt    string
    Trigger      string  // "schedule" / "manual"
    RunID        string
}

type CronTaskRunQuery struct {
    TaskID string
    Status string
    Limit  int
}

type CronTaskRunInput struct {
    ID         string
    TaskID     string
    Status     string
    StartedAt  string
    OutputJSON string
    CreatedAt  string
}

type CronTaskPatch struct {
    TaskKey      *string
    Name         *string
    Description  *string
    Status       *string
    Enabled      *bool
    SortOrder    *int
    AgentID      *string
    ConfigJSON   *string
    MetadataJSON *string
}
```

### 3.2 Repo 接口

```go
type CronRepo interface {
    ListCronTasks(ctx context.Context) ([]CronTask, error)
    GetCronTask(ctx context.Context, id string) (CronTask, error)
    CreateCronTask(ctx context.Context, t CronTask) (CronTask, error)
    UpdateCronTask(ctx context.Context, t CronTask) (CronTask, error)
    DeleteCronTask(ctx context.Context, id string) error
    GetCronTaskRun(ctx context.Context, id string) (CronTaskRun, error)
    ListCronTaskRuns(ctx context.Context, q CronTaskRunQuery) ([]CronTaskRun, error)
    InsertCronTaskRun(ctx context.Context, in CronTaskRunInput) error
    UpdateCronTaskRun(ctx context.Context, id, status, finishedAt, outputJSON, errorMessage string) error
}
```

### 3.3 Usecase

```go
type CronUsecase struct {
    repo CronRepo
}

func NewCronUsecase(repo CronRepo) *CronUsecase

func (u *CronUsecase) ListTasks(ctx context.Context) ([]CronTask, error)
func (u *CronUsecase) GetTask(ctx context.Context, id string) (CronTask, error)
func (u *CronUsecase) CreateTask(ctx context.Context, in CronTask) (CronTask, error)
func (u *CronUsecase) UpdateTask(ctx context.Context, id string, patch CronTaskPatch) (CronTask, error)
func (u *CronUsecase) DeleteTask(ctx context.Context, id string) error
func (u *CronUsecase) ListTaskRuns(ctx context.Context, q CronTaskRunQuery) ([]CronTaskRun, error)
func (u *CronUsecase) GetTaskRun(ctx context.Context, id string) (CronTaskRun, error)
func (u *CronUsecase) TriggerTask(ctx context.Context, id string) (CronTaskRun, error)
func (u *CronUsecase) ResetTaskFailures(ctx context.Context, id string) (CronTask, error)
```

**CronTaskTrigger**（由 `*cronrunner.Runner` 实现，Wire 注入 `CronUsecase`）：

```go
type CronTaskTrigger interface {
    TriggerTask(ctx context.Context, taskID string) (CronTaskRun, error)
}

**CreateTask 校验规则**：
- `task_key` 和 `name` 必填
- ID 为空时自动生成 24 位 hex 随机 ID
- `status` 为空时默认 `active`

**UpdateTask 合并策略**：
- 读取当前记录，用 `CronTaskPatch` 指针非 nil 的字段覆盖
- `TaskKey`/`Name`/`Status` 仅在指针非 nil 且值非空时覆盖
- `Description`/`Enabled`/`SortOrder`/`AgentID`/`ConfigJSON`/`MetadataJSON` 指针非 nil 时覆盖
- 指针为 nil 的字段保持原值不变（解决 bool/int 零值歧义）

---

## 四、Data 层

### 4.1 Ent Schema

- `internal/data/ent/schema/cron_task.go` — 任务表（字段：id, task_key, name, description, status, enabled, sort_order, agent_id, config_json, metadata_json, created_at, updated_at, deleted_at）
- `internal/data/ent/schema/cron_task_run.go` — 执行记录表（字段：id, task_id, status, started_at, finished_at, output_json, error_message, created_at）

### 4.2 Data 层实现

文件：`internal/data/cron.go`

```go
type cronRepo struct {
    data *Data
}

func NewCronRepo(d *Data) biz.CronRepo
```

**关键查询**：

| 方法 | Ent ORM 查询 |
|------|-------------|
| `ListCronTasks` | `CronTask.Query().Where(DeletedAtEQ("")).Order(BySortOrder(), ByCreatedAt(Desc)).All(ctx)` |
| `GetCronTask` | `CronTask.Query().Where(IDEQ(id), DeletedAtEQ("")).Only(ctx)` |
| `CreateCronTask` | `CronTask.Create().SetID/SetTaskKey/SetName/.../Save(ctx)` |
| `UpdateCronTask` | `CronTask.UpdateOneID(t.ID).SetTaskKey/SetName/.../Exec(ctx)` |
| `DeleteCronTask` | `CronTask.UpdateOneID(id).SetDeletedAt(now).SetStatus("deleted").Exec(ctx)` — 软删 |
| `ListCronTaskRuns` | `CronTaskRun.Query().Where(TaskIDEQ(tid), StatusEQ(st)).Order(ByCreatedAt(Desc)).Limit(limit).All(ctx)` + 批量查询任务名称 |
| `InsertCronTaskRun` | `CronTaskRun.Create().SetID/SetTaskID/SetStatus/.../Save(ctx)` — 参数为 `CronTaskRunInput` 结构体 |
| `UpdateCronTaskRun` | `CronTaskRun.UpdateOneID(id).SetStatus/SetFinishedAt/.../Exec(ctx)` |

**output_json 解析**：`outputJSONExtras` 函数从 `output_json` 提取 `trigger` 和 `run_id`，用于填充 `CronTaskRun.Trigger` 和 `CronTaskRun.RunID`。

### 4.3 调度器

```go
// internal/cronrunner/runner.go
type Runner struct {
    deps Deps
    lg   loggateway.Logger
    mu   sync.Mutex
    taskMu sync.Map
}

type Deps struct {
    Cron              biz.CronRepo
    Session           SessionUsecase
    Teams             TeamLister
    Agents            AgentLister
    Chat              CronChatRunner
    Event             EventPublisher
    RegistrySyncAgent RegistrySyncAgent
}

func NewRunner(deps Deps, lg loggateway.Logger) *Runner
func (r *Runner) Start(ctx context.Context, interval time.Duration) error
func (r *Runner) Stop()
func (r *Runner) TriggerTask(ctx context.Context, taskID string) (biz.CronTaskRun, error)
```

**调度执行流程**（`internal/cronrunner/runner.go`）：
1. `cmd/admin/main.go` 启动 `Runner.Start(ctx, interval)`；间隔由 `CRON_RUNNER_INTERVAL` 控制（**默认 1 分钟**，非 15 秒）
2. 每 tick 用 `TryLock` 防重入，加载全部未删 `cron_task`，筛选 `enabled=true`、`status=active`
3. 在 Go 进程内解析 `metadata_json.next_run_at`，仅调度 `next_run_at <= now` 的到期任务
4. 解析 `config_json` 与 schedule；**无效配置/空 message/到期计算错误** → 写入 `cron_task_run`（`failure`），递增 `failure_count`，**不 dispatch**（与 dispatch 失败共用 `finalizeRun` 路径）
5. 解析 `config_json.target_type`：`agent` 使用 `cron_task.agent_id`；`team` 使用 `config_json.team_id`；`model_registry_sync` 触发模型注册表同步
6. 写入 `cron_task_run`（`pending`），`dispatchWithRetry` 按 `retry_max_attempts` 退避（默认 3 次：30s/2m/10m；`0`=不重试）
7. 创建 Session（`dialog_mode=cron`），调用 **`ChatService.RunCronTurn`**（EP-RT-07 in-process）；`CRON_CHAT_DISPATCH_ORIGIN` 保留 HTTP fallback
8. 写回 `cron_task_run` + `metadata_json` 统计 + 重算 `next_run_at`；`once` 执行后自动暂停
9. 连续失败 ≥3 次 → `status=dead` + `cron.dead_letter` 事件 + Prometheus `aranea_cron_job_dead_total`

**`cron_task_run.status` 语义**：

| status | 场景 | `failure_count` / dead letter | 写入 run 行 |
|--------|------|-------------------------------|-------------|
| `success` | dispatch 成功 | schedule 成功清零失败链 | ✅ |
| `failure` | 配置/schedule 错误、dispatch 失败 | schedule 递增，可 dead | ✅ |
| `skipped` | Session 忙（`ErrCronSessionBusy`，瞬时） | **不**递增 | ✅ |
| `pending` | 手动 trigger 已提交、尚未完成 | — | ✅ |

**手动触发**（`POST /v1/cron-tasks/{id}/trigger`）：
- 立即写入 `cron_task_run`（`pending`）并返回；后台 `safego` 异步执行。
- `output_json.trigger=manual`。
- **不**校验 schedule 到期；**不**推进 `next_run_at`；**不**因 manual 执行 pause `once` 任务。
- manual 失败**不计入** dead letter 连续失败计数（仍写入 run 与 `recent_failures` 可选记录）。
- 同一 `task_id` 与调度 tick 共用 per-task 互斥锁，避免双跑。

**重置失败计数**（`POST /v1/cron-tasks/{id}/reset-failures`）：
- 清零 `metadata_json.failure_count/last_error/recent_failures`，`status=active`、`enabled=true`。

---

## 五、Service 层

文件：`internal/service/cron.go`

```go
type CronService struct {
    v1.UnimplementedCronServiceServer
    uc *biz.CronUsecase
}

func NewCronService(uc *biz.CronUsecase, runner *cronrunner.Runner) *CronService
```

| 方法 | 转换逻辑 |
|------|----------|
| `ListCronTasks` | `uc.ListTasks` → 逐条 `toProtoCronTask` |
| `CreateCronTask` | `req` → `biz.CronTask` → `uc.CreateTask` → `toProtoCronTask` |
| `GetCronTask` | `uc.GetTask` → `toProtoCronTask`；`sql.ErrNoRows` → `NotFound` |
| `UpdateCronTask` | `req.Task` → `patchFromProtoCronTask` → `uc.UpdateTask` → `toProtoCronTask` |
| `DeleteCronTask` | `uc.DeleteTask` → `emptypb.Empty` |
| `ListCronTaskRuns` | `req` → `biz.CronTaskRunQuery` → `uc.ListTaskRuns` → 逐条 `toProtoCronTaskRun` |
| `TriggerCronTask` | `runner.TriggerTask` → `toProtoCronTaskRun`；异步执行，立即返回 `pending` run |
| `ResetCronTaskFailures` | `uc.ResetTaskFailures` → `toProtoCronTask` |

**类型转换函数**：
- `toProtoCronTask(biz.CronTask) *v1.CronTask`
- `patchFromProtoCronTask(*v1.CronTask) biz.CronTaskPatch`
- `toProtoCronTaskRun(biz.CronTaskRun) *v1.CronTaskRun`

---

## 六、Wire 注入

已有注入：
```
data.ProviderSet → NewCronRepo
biz.ProviderSet → NewCronUsecase
service.ProviderSet → NewCronService
cronrunner.ProviderSet → NewRunner
```

Runner 在 **`cmd/admin/main.go`** 中通过 Wire 注入；`CRON_RUNNER_DISABLED=1` 时不创建 Runner。启动时 `go runner.Start(cronCtx, DefaultInterval())`，随进程 ctx 取消退出。

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/
├── features/cron/
│   ├── api.ts                          # API 调用与类型转换
│   └── types.ts                        # TypeScript 类型定义
├── components/cron/
│   ├── CronTaskFormDialog.vue          # 创建/编辑对话框
│   ├── CronTaskFormFields.vue          # 表单字段（主容器）
│   ├── CronTaskFormTargetFields.vue    # 目标类型选择（Agent/Team）
│   ├── CronTaskFormScheduleFields.vue  # 计划类型选择（interval/cron/once）
│   ├── CronRunsDialog.vue              # 执行历史弹窗
│   └── cronTaskUtils.ts               # 表单工具函数
└── pages/
    └── CronTasksPage.vue               # 定时任务管理主页（含执行历史弹窗入口）
```

### 7.2 页面路由

| 路由 | 页面 | 说明 |
|------|------|------|
| `/cron` | `CronTasksPage.vue` | 定时任务管理主页（含执行历史弹窗 `CronRunsDialog`） |

### 7.3 CronTasksPage.vue

| 区域 | 组件 | 说明 |
|------|------|------|
| 顶栏 | `QToolbar` | 标题「定时任务」+ 副标题「安排定期 Agent 任务」+ 刷新按钮 + 新建按钮 |
| 搜索 | `QInput` | `outlined rounded dense debounce="300"`，搜索 `name`/`description` |
| 空状态 | `QIcon` + `QAvatar` + `QBtn` | 图标 `schedule`，文案「暂无定时任务」，新建按钮 |
| 表格 | `QTable` | 客户端分页与搜索（当前 `ListCronTasks` 无 search/page 参数，全量返回后前端过滤） |

**表格列**：

| 列 | 字段 | UI |
|----|------|-----|
| 名称 | `name` | 主行名称，副行 `task_key` |
| 描述 | `description` | 单行截断 + `QTooltip` |
| 计划 | `config_json.schedule_type` | 根据 `schedule_type` 渲染：`每隔 N 分钟` / `cron: 0 * * * *` / `一次 · 2026-04-22 09:00` |
| Agent | `agent_id` | 空=「默认」，有=Agent 显示名 |
| 执行次数 | `metadata_json.run_count` | 数字 |
| 成功/失败 | `metadata_json.success_count` / `metadata_json.failure_count` | 失败>0 时 `text-negative` + `QTooltip` 展示最近失败 |
| 状态 | `status` + `enabled` | `QBadge`：active 绿、paused 灰、dead 红 |
| 上次运行 | `metadata_json.last_run_at` | 相对时间 |
| 下次运行 | `metadata_json.next_run_at` | once 已执行显示「—」 |
| 操作 | — | 启用/暂停 `QToggle` + 编辑/删除 `QBtn flat dense` + 历史 `QBtn icon="history"` + dead 任务「重置失败计数」`QBtn icon="restart_alt"` |

### 7.4 CronTaskFormDialog.vue

| 控件 | 绑定 | 说明 |
|------|------|------|
| `QInput` 名称 | `name` | 必填，hint="仅小写字母、数字和连字符" |
| `QBtnToggle` 目标类型 | `config.targetType` | agent / team |
| `QSelect` Agent | `agentId` | target_type=agent 时显示，选项含「默认」+ Agent 列表 |
| `QSelect` Team | `config.teamId` | target_type=team 时显示，Team 列表 |
| `QBtnToggle` 计划类型 | `config.scheduleType` | interval / cron / once |
| `QInput` 每隔 | `config.intervalSeconds` | schedule_type=interval 时显示 |
| `QInput` Cron 表达式 | `config.cronExpression` | schedule_type=cron 时显示，hint="标准5段" |
| `QInput` 执行时间 | `config.runAt` | schedule_type=once 时显示，`QPopupProxy` + `QDate` + `QTime` |
| `QInput` 描述 | `description` | `autogrow` |
| `QInput` 消息 | `config.message` | `type="textarea" autogrow`，placeholder="Agent 应该做什么?" |
| `QInput` 最大重试次数 | `config.retryMaxAttempts` | `type="number"`，hint="0=禁用重试，默认3" |
| `QToggle` 启用 | `enabled` | 默认启用 |

### 7.5 CronRunsDialog.vue

> 执行历史以弹窗形式嵌入 `CronTasksPage`，非独立路由页面。

| 区域 | 组件 | 说明 |
|------|------|------|
| 筛选器 | `QSelect` + `QInput` | 定时任务（可清空=全部）、状态 success/failure/pending/skipped |
| 表格 | `QTable` | 列：任务名称、started_at、finished_at、status、error_message 摘要、trigger、run_id；前端分页 |

### 7.6 API

```typescript
export async function listCronTasks(): Promise<CronTask[]>
export async function getCronTask(id: string): Promise<CronTask>
export async function createCronTask(payload: CreateCronTaskRequest): Promise<CronTask>
export async function updateCronTask(id: string, payload: CronTask): Promise<CronTask>
export async function deleteCronTask(id: string): Promise<void>
export async function triggerCronTask(id: string): Promise<CronTaskRun>
export async function resetCronTaskFailures(id: string): Promise<PlatformResource>
export async function listCronTaskRuns(query: { cron_task_id?: string; status?: string; limit?: number }): Promise<CronTaskRun[]>
export async function listCronAgents(): Promise<Agent[]>
export async function listCronTeams(): Promise<Team[]>
export function parseCronConfig(row: CronTask): CronTaskConfig
export function parseCronMetadata(row: CronTask): CronTaskMetadata
```

### 7.7 类型定义

```typescript
export interface CronTask {
  id: string
  task_key: string
  name: string
  description: string
  status: string
  enabled: boolean
  sort_order: number
  agent_id: string
  config_json: string
  metadata_json: string
  created_at: string
  updated_at: string
  deleted_at: string
}

export interface CronTaskRun {
  id: string
  task_id: string
  task_name: string
  status: string  // "success" | "failure" | "pending" | "skipped"
  started_at: string
  finished_at: string
  trigger: string  // "schedule" | "manual"
  run_id: string
  output_json: string
  error_message: string
  created_at: string
}

export interface CronTaskConfig {
  target_type: 'agent' | 'team' | 'model_registry_sync'
  team_id: string
  schedule_type: 'interval' | 'cron' | 'once'
  cron_expression: string
  interval_seconds: number
  run_at: string
  timezone: string
  message: string
  retry_max_attempts: number
}

export interface CronTaskMetadata {
  run_count: number
  success_count: number
  failure_count: number
  last_run_at: string
  last_run_status: string
  last_error: string
  next_run_at: string
  recent_failures: Array<{ started_at: string; error_message: string }>
}
```
