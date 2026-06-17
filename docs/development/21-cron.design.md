# Cron 定时任务模块 — 实现设计文档

> 对应需求：[21 cron.md](./21%20cron.md)
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

> **内容边界**：本文档描述架构、代码分层、Proto/API 契约、数据模型、接口定义、技术选型、状态机、前端组件设计、UX 规范。
> - 用户故事 / 验收标准 / 交互规格 → 见 [21-cron.md](./21-cron.md)
> - 模块定位 / 代码锚点 / 现状评估 / 任务清单 / 改动文件 → 见 [21-cron.development.md](./21-cron.development.md)

---

## 一、模块概述

定时任务调度：Cron 表达式/间隔/单次触发 Agent/Team/ModelRegistrySync 执行、任务管理、执行日志。支持三种计划类型（interval/cron/once），通过 `config_json` 存储计划详情，通过 `metadata_json` 存储运行统计。

**核心设计决策**：
- 物理表 `cron_task` 仅保存通用资源字段（id/task_key/name/description/status/enabled/sort_order/agent_id/config_json/metadata_json/时间戳），调度详情与统计字段不提升为物理列，避免主表迁移成本
- 调度器在 Go 进程内解析 `metadata_json.next_run_at` 筛选到期任务（P4 待优化为 DB 层筛选）
- 执行路径复用 `ChatService.RunCronTurn`（EP-RT-07 in-process），保留 `CRON_CHAT_DISPATCH_ORIGIN` HTTP fallback

---

## 二、架构与代码分层

```
api/kratos/cron/v1/cron.proto          ← Proto 契约（真相源）
        ↓
internal/service/cron.go               ← CronService（传输桥点）
internal/service/cron_trigger_gateway_adapter.go  ← CronTriggerGateway 适配器
        ↓
internal/biz/cron.go                   ← 类型 alias 到 cron 子包
internal/biz/cron/cron.go              ← Usecase + Repo 接口 + 领域模型 + 校验
        ↓
internal/data/cron.go                  ← CronRepo（Ent 实现）
internal/data/ent/schema/cron_task.go  ← Ent Schema（cron_task 表）
internal/data/ent/schema/cron_task_run.go  ← Ent Schema（cron_task_run 表）
        ↓
internal/cronrunner/runner.go          ← Runner 主循环 + dispatch + TriggerTask
internal/cronrunner/execute.go         ← executeTask / finalizeRun / lockTask / recordScheduleFailure
internal/cronrunner/schedule.go        ← config_json / metadata_json 解析 + next_run_at 计算
internal/cronrunner/errors.go          ← schedule 解析错误
internal/cronrunner/seed_model_registry.go  ← 模型注册表同步种子任务
        ↓
cmd/admin/wire.go                      ← provideCronRunnerDeps + provideCronRunner
cmd/admin/workers.go                   ← CronRunner.Start 启动
```

**依赖方向**：service → biz → data；cronrunner 依赖 biz 接口（CronRepo/SessionUsecase/TeamReader/AgentRepository）和 service（ChatService 实现 CronChatRunner）。

---

## 三、Proto / API 契约

### 3.1 Proto 定义

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

message ListCronTasksResponse { repeated CronTask items = 1; }
message ListCronTaskRunsRequest { string cron_task_id = 1; string status = 2; int32 limit = 3; }
message ListCronTaskRunsResponse { repeated CronTaskRun items = 1; }

message CreateCronTaskRequest {
  string task_key = 1 [(google.api.field_behavior) = REQUIRED];
  string name = 2 [(google.api.field_behavior) = REQUIRED];
  string description = 3; string status = 4; bool enabled = 5;
  int32 sort_order = 6; string agent_id = 7;
  string config_json = 8; string metadata_json = 9;
}
message UpdateCronTaskRequest { string id = 1 [(google.api.field_behavior) = REQUIRED]; CronTask task = 2; }
message GetCronTaskRequest { string id = 1 [(google.api.field_behavior) = REQUIRED]; }
message DeleteCronTaskRequest { string id = 1 [(google.api.field_behavior) = REQUIRED]; }
message TriggerCronTaskRequest { string id = 1 [(google.api.field_behavior) = REQUIRED]; }
message ResetCronTaskFailuresRequest { string id = 1 [(google.api.field_behavior) = REQUIRED]; }

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
    option (google.api.http) = {post: "/v1/cron-tasks/{id}/trigger" body: "*"};
  }
  rpc ResetCronTaskFailures(ResetCronTaskFailuresRequest) returns (CronTask) {
    option (google.api.http) = {post: "/v1/cron-tasks/{id}/reset-failures" body: "*"};
  }
}
```

### 3.2 HTTP API 摘要

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/cron-tasks` | 列表（当前无搜索/分页参数，前端客户端过滤；P3 待实现服务端 search/page） |
| POST | `/v1/cron-tasks` | 创建；服务端校验计划字段互斥 |
| GET | `/v1/cron-tasks/:id` | 详情 |
| PATCH | `/v1/cron-tasks/:id` | 更新名称、计划、消息、`status` |
| DELETE | `/v1/cron-tasks/:id` | 软删 |
| POST | `/v1/cron-tasks/:id/trigger` | 立即执行一次（异步，返回 `pending` 状态的 `CronTaskRun`） |
| POST | `/v1/cron-tasks/:id/reset-failures` | 重置失败计数（清零 `failure_count`/`last_error`/`recent_failures`，恢复 `active`） |
| GET | `/v1/cron-task-runs?cron_task_id=&status=&limit=` | 执行历史列表；支持按任务、状态筛选 |

调度器服务根据 `cron_task` 计算并回写 `next_run_at`，触发时消费 `message`（+ `payload`）启动 Agent 运行；每次运行结束更新 **`run_count` / `success_count` / `failure_count`**（或由异步任务根据 `cron_task_run` 汇总回写）。

---

## 四、数据模型

### 4.1 物理表 `cron_task`

**设计决策**：物理表只保存通用资源字段，调度详情与运行统计分别存入 `config_json` / `metadata_json`，避免主表迁移成本。后续若调度器需要高频查询，可再把 `next_run_at`、计数字段提升为物理列（P4）。

Ent Schema：`internal/data/ent/schema/cron_task.go`

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | string | Immutable, Unique, MaxLen 256 | 主键（24 位 hex 随机 ID） |
| `task_key` | string | Unique, MaxLen 512 | 业务 key（前端类型中为 `key`） |
| `name` | string | MaxLen 1024 | 展示名称 |
| `description` | text | default "" | 列表副行、备注 |
| `status` | string | default "active" | `active` / `paused` / `dead` / `deleted` |
| `enabled` | bool | default true | 启用开关 |
| `sort_order` | int | default 0 | 排序 |
| `agent_id` | string | default "", MaxLen 256 | 空表示「默认」Agent 策略由运行时解析 |
| `config_json` | text | default "" | 计划详情（见 §4.3） |
| `metadata_json` | text | default "" | 运行统计（见 §4.4） |
| `created_at` | string | default "" | RFC3339 |
| `updated_at` | string | default "" | RFC3339 |
| `deleted_at` | string | default "" | 软删标记，非空=已删 |

**索引**：`idx_cron_task_agent` (agent_id, deleted_at)

**校验规则（应用层）**：
- `schedule_type = 'cron'` → `cron_expression` 非空，`interval_seconds` / `run_at` 为空
- `schedule_type = 'interval'` → `interval_seconds` > 0
- `schedule_type = 'once'` → `run_at` 非空；执行完毕后 `status=paused`、`enabled=false`、`next_run_at=""`

### 4.2 物理表 `cron_task_run`

Ent Schema：`internal/data/ent/schema/cron_task_run.go`

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | string | Immutable, Unique, MaxLen 256 | UUID |
| `task_id` | string | MaxLen 256 | FK → cron_task.id |
| `status` | string | default "pending" | `success` / `failure` / `pending` / `skipped` |
| `started_at` | string | default "" | RFC3339 |
| `finished_at` | string | default "" | RFC3339 |
| `output_json` | text | default "" | trigger / session_id / user_message_id / agent_message_id / run_id |
| `error_message` | text | default "" | 失败摘要 |
| `created_at` | string | default "" | RFC3339 |

**索引**：`idx_cron_run_task` (task_id, created_at)

**说明**：列表 tooltip 所需的「最近失败」由 `cron_task.metadata_json.recent_failures` 提供（最多 5 条）；详细历史从 `cron_task_run` 查询。

### 4.3 `config_json` 结构

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
| `target_type` | string | `agent` / `team` / `model_registry_sync`（前端表单只暴露 agent/team，model_registry_sync 由种子任务注入） |
| `team_id` | string | target_type=team 时必填 |
| `schedule_type` | string | `interval` / `cron` / `once`，空默认 `interval` |
| `cron_expression` | string | schedule_type=cron 时必填，标准5段 |
| `interval_seconds` | int | schedule_type=interval 时必填，<=0 时默认 900 |
| `run_at` | string | schedule_type=once 时必填，ISO 时间 |
| `timezone` | string | 默认 `UTC` |
| `message` | string | 下发给 Agent/Team 的消息，必填 |
| `retry_max_attempts` | int (nullable) | 首次失败后的重试次数；`null`/未设置=默认 3；`0`=禁用；正整数=指定次数 |

### 4.4 `metadata_json` 结构

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

### 4.5 UI 字段与物理字段映射

| UI / 逻辑字段 | 保存位置 |
|---------------|----------|
| `name`（slug） | `cron_task.task_key`（前端类型中为 `key`） |
| 展示名称 | `cron_task.name` |
| `description` | `cron_task.description` |
| `agent_id` | `cron_task.agent_id`（目标为 Agent 时） |
| `target_type`、`team_id`、`schedule_type`、`cron_expression`、`interval_seconds`、`run_at`、`timezone`、`message`、`retry_max_attempts` | `config_json` |
| `run_count`、`success_count`、`failure_count`、`last_run_at`、`last_run_status`、`last_error`、`next_run_at`、`recent_failures[]` | `metadata_json`，执行历史页也可从 `cron_task_run` 查询 |
| 启用 / 暂停 | `enabled` + `status`（`active` / `paused`） |

---

## 五、Biz 层

### 5.1 类型 alias 文件

文件：`internal/biz/cron.go`

```go
type CronTriggerGateway interface {
    TriggerCronTask(ctx context.Context, taskID string) (CronTaskRun, error)
    GetTaskRun(ctx context.Context, id string) (CronTaskRun, error)
}

type (
    CronTask         = cron.Task
    CronTaskRun      = cron.TaskRun
    CronTaskRunQuery = cron.TaskRunQuery
    CronTaskRunInput = cron.TaskRunInput
    CronTaskPatch    = cron.TaskPatch
    CronRepo         = cron.Repo
    CronTaskTrigger  = cron.TaskTrigger
    CronUsecase      = cron.Usecase
)

var (
    NewCronUsecase           = cron.NewUsecase
    ResetCronFailureMetadata = cron.ResetFailureMetadata
    StrPtr                   = cron.StrPtr
    BoolPtr                  = cron.BoolPtr
    IntPtr                   = cron.IntPtr
    ErrCronRunnerDisabled    = cron.ErrRunnerDisabled
    ErrCronTaskDeleted       = cron.ErrTaskDeleted
    ErrCronSessionBusy       = cron.ErrSessionBusy
    ErrCronNotFound          = cron.ErrNotFound
)
```

### 5.2 领域模型

文件：`internal/biz/cron/cron.go`

```go
type Task struct {
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

type TaskRun struct {
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

type TaskRunQuery struct {
    TaskID string
    Status string
    Limit  int
}

type TaskRunInput struct {
    ID         string
    TaskID     string
    Status     string
    StartedAt  string
    OutputJSON string
    CreatedAt  string
}

type TaskPatch struct {
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

### 5.3 Repo 接口

```go
type Repo interface {
    ListCronTasks(ctx context.Context) ([]Task, error)
    GetCronTask(ctx context.Context, id string) (Task, error)
    CreateCronTask(ctx context.Context, t Task) (Task, error)
    UpdateCronTask(ctx context.Context, t Task) (Task, error)
    DeleteCronTask(ctx context.Context, id string) error
    GetCronTaskRun(ctx context.Context, id string) (TaskRun, error)
    ListCronTaskRuns(ctx context.Context, q TaskRunQuery) ([]TaskRun, error)
    InsertCronTaskRun(ctx context.Context, in TaskRunInput) error
    UpdateCronTaskRun(ctx context.Context, id, status, finishedAt, outputJSON, errorMessage string) error
}

type TaskTrigger interface {
    TriggerTask(ctx context.Context, taskID string) (TaskRun, error)
}
```

### 5.4 Usecase

```go
type Usecase struct {
    repo    Repo
    trigger TaskTrigger
}

func NewUsecase(repo Repo, trigger TaskTrigger) *Usecase

func (u *Usecase) ListTasks(ctx context.Context) ([]Task, error)
func (u *Usecase) GetTask(ctx context.Context, id string) (Task, error)
func (u *Usecase) CreateTask(ctx context.Context, in Task) (Task, error)
func (u *Usecase) UpdateTask(ctx context.Context, id string, patch TaskPatch) (Task, error)
func (u *Usecase) DeleteTask(ctx context.Context, id string) error
func (u *Usecase) ListTaskRuns(ctx context.Context, q TaskRunQuery) ([]TaskRun, error)
func (u *Usecase) GetTaskRun(ctx context.Context, id string) (TaskRun, error)
func (u *Usecase) TriggerTask(ctx context.Context, id string) (TaskRun, error)
func (u *Usecase) ResetTaskFailures(ctx context.Context, id string) (Task, error)
```

**CreateTask 校验规则**：
- `task_key` 和 `name` 必填
- ID 为空时自动生成 24 位 hex 随机 ID
- `status` 为空时默认 `active`
- 调用 `ValidateTaskConfig(in.ConfigJSON)` 校验 `target_type`（必须为 agent/team/model_registry_sync）和 `cron_expression`（非空字符串）

**UpdateTask 合并策略**：
- 读取当前记录，用 `TaskPatch` 指针非 nil 的字段覆盖
- `TaskKey`/`Name`/`Status` 仅在指针非 nil 且值非空时覆盖
- `Description`/`Enabled`/`SortOrder`/`AgentID`/`ConfigJSON`/`MetadataJSON` 指针非 nil 时覆盖
- 指针为 nil 的字段保持原值不变（解决 bool/int 零值歧义）
- 合并后调用 `ValidateTaskConfig(merged.ConfigJSON)` 校验

**ResetTaskFailures**：
- 调用 `ResetFailureMetadata(cur.MetadataJSON)` 清零 `failure_count`/`last_error`/`recent_failures`
- 通过 `UpdateTask` 设置 `enabled=true`、`status=active`、新 `metadata_json`

### 5.5 错误定义

```go
var (
    ErrRunnerDisabled = apierror.Unavailable("CRON", "cron runner disabled")
    ErrTaskDeleted    = apierror.NotFound("CRON", "cron task deleted")
    ErrSessionBusy    = apierror.Conflict("CRON", "cron session has active run")
    ErrNotFound       = apierror.NotFound("CRON", "cron task not found")
)
```

---

## 六、Data 层

### 6.1 Repo 实现

文件：`internal/data/cron.go`

```go
type cronRepo struct {
    data *Data
}

var _ bizcron.Repo = (*cronRepo)(nil)

func NewCronRepo(d *Data) biz.CronRepo
```

**关键查询**：

| 方法 | Ent ORM 查询 |
|------|-------------|
| `ListCronTasks` | `r.data.RW().Read(ctx).CronTask.Query().Where(DeletedAtEQ("")).Order(BySortOrder(), ByCreatedAt(Desc)).All(ctx)` |
| `GetCronTask` | `r.data.RW().Read(ctx).CronTask.Query().Where(IDEQ(id), DeletedAtEQ("")).Only(ctx)`；`ent.IsNotFound` → `biz.ErrCronNotFound` |
| `CreateCronTask` | `r.data.RW().Write(ctx).CronTask.Create().SetID/SetTaskKey/SetName/.../Save(ctx)` |
| `UpdateCronTask` | `r.data.RW().Write(ctx).CronTask.UpdateOneID(t.ID).SetTaskKey/SetName/.../Exec(ctx)` 后 `GetCronTask` 返回最新 |
| `DeleteCronTask` | `r.data.RW().Write(ctx).CronTask.UpdateOneID(id).SetDeletedAt(now).SetStatus("deleted").SetUpdatedAt(now).Exec(ctx)` — 软删 |
| `GetCronTaskRun` | `r.data.RW().Read(ctx).CronTaskRun.Query().Where(IDEQ(id)).Only(ctx)` + 关联查询 task 名称 |
| `ListCronTaskRuns` | `r.data.RW().Read(ctx).CronTaskRun.Query().Order(ByCreatedAt(Desc)).Limit(limit).All(ctx)` + 批量查询任务名称（按 task_id 去重后 `IDIn` 查询） |
| `InsertCronTaskRun` | `r.data.RW().Write(ctx).CronTaskRun.Create().SetID/SetTaskID/SetStatus/.../Save(ctx)` — 参数为 `CronTaskRunInput` 结构体 |
| `UpdateCronTaskRun` | `r.data.RW().Write(ctx).CronTaskRun.UpdateOneID(id).SetStatus/SetFinishedAt/SetOutputJSON/SetErrorMessage/Exec(ctx)` |

**Limit 边界**：`limit <= 0` → 100；`limit > 500` → 500

**类型转换函数**：
- `entToBizCronTask(e *ent.CronTask) biz.CronTask`
- `entToBizCronTaskRun(e *ent.CronTaskRun, taskName string) biz.CronTaskRun`
- `outputJSONExtras(outputJSON string) (trigger, runID string)` — 从 `output_json` 提取 `trigger` 和 `run_id`，trigger 空时默认 `schedule`

**已知技术债务**：错误处理直接返回 `biz.ErrCronNotFound`，未使用 `entErrToBizErr`（DB-DEBT-04）。

---

## 七、调度器（Cronrunner）

### 7.1 Runner 结构

文件：`internal/cronrunner/runner.go`

```go
type Runner struct {
    deps   Deps
    lg     loggateway.Logger
    mu     sync.Mutex      // TryLock 防止 tick 重入
    taskMu sync.Map        // per-task 互斥锁（key=taskID, value=*sync.Mutex）
}

type Deps struct {
    Cron              biz.CronRepo
    Session           *biz.SessionUsecase
    Teams             biz.TeamReader
    Agents            biz.AgentRepository
    EventBus          event.Bus
    Chat              CronChatRunner
    RegistrySyncAgent CronRegistrySyncAgent
}

func NewRunner(deps Deps, lg loggateway.Logger) *Runner
func (r *Runner) Start(ctx context.Context, interval time.Duration)
func (r *Runner) TriggerTask(ctx context.Context, taskID string) (biz.CronTaskRun, error)
```

**接口定义**：

```go
// CronChatRunner dispatches a single cron-triggered chat turn via the in-process agent runner (EP-RT-07).
// Implemented by *service.ChatService.
type CronChatRunner interface {
    RunCronTurn(ctx context.Context, sessionID, content, teamID string) (userMsgID, agentMsgID string, err error)
}

// SessionRunControl is optionally implemented by CronChatRunner to respect active runs.
type SessionRunControl interface {
    HasActiveRun(sessionID string) bool
}

type CronRegistrySyncAgent interface {
    RunSync(ctx context.Context) error
}
```

### 7.2 调度执行流程

`Start(ctx, interval)` 阻塞运行，先执行一次 `runDue`，然后每 `interval` tick 一次。

1. `cmd/admin/main.go` 启动 `Runner.Start(ctx, interval)`；间隔由 `CRON_RUNNER_INTERVAL` 控制（**默认 1 分钟**）
2. 每 tick 用 `mu.TryLock` 防重入，加载全部未删 `cron_task`，筛选 `enabled=true`、`status=active`
3. 在 Go 进程内解析 `metadata_json.next_run_at`，仅调度 `next_run_at <= now` 的到期任务
4. 解析 `config_json` 与 schedule；**无效配置/空 message/到期计算错误** → 写入 `cron_task_run`（`failure`），递增 `failure_count`，**不 dispatch**（`recordScheduleFailure`）
5. 解析 `config_json.target_type`：`agent` 使用 `cron_task.agent_id`；`team` 使用 `config_json.team_id`；`model_registry_sync` 触发模型注册表同步
6. 写入 `cron_task_run`（`pending`），`dispatchWithRetry` 按 `retry_max_attempts` 退避（默认 3 次：30s/2m/10m；`0`=不重试）
7. 创建 Session（`dialog_mode=cron`），调用 **`ChatService.RunCronTurn`**（EP-RT-07 in-process）；`CRON_CHAT_DISPATCH_ORIGIN` 保留 HTTP fallback
8. 写回 `cron_task_run` + `metadata_json` 统计 + 重算 `next_run_at`；`once` 执行后自动暂停
9. 连续失败 ≥3 次 → `status=dead` + `cron.dead_letter` 事件 + Prometheus `aranea_cron_job_dead_total`

### 7.3 `cron_task_run.status` 语义

| status | 场景 | `failure_count` / dead letter | 写入 run 行 |
|--------|------|-------------------------------|-------------|
| `success` | dispatch 成功 | schedule 成功清零失败链 | 是 |
| `failure` | 配置/schedule 错误、dispatch 失败 | schedule 递增，可 dead | 是 |
| `skipped` | Session 忙（`ErrCronSessionBusy`，瞬时） | **不**递增 | 是 |
| `pending` | 手动 trigger 已提交、尚未完成 | — | 是 |

### 7.4 手动触发（`TriggerTask`）

`POST /v1/cron-tasks/{id}/trigger` → `Runner.TriggerTask`：
- 校验任务存在、未删除、`config_json.message` 非空
- 在 `lockTask` 内 `insertPendingRun` 写入 `cron_task_run`（`pending`），`output_json.trigger=manual`
- 立即返回 `pending` run；后台 `safego.Go` 异步执行 `runManualTask`
- **不**校验 schedule 到期；**不**推进 `next_run_at`；**不**因 manual 执行 pause `once` 任务
- manual 失败**不计入** dead letter 连续失败计数（仍写入 run 与 `recent_failures`）
- 同一 `task_id` 与调度 tick 共用 per-task 互斥锁，避免双跑

### 7.5 重置失败计数（`ResetTaskFailures`）

`POST /v1/cron-tasks/{id}/reset-failures` → `Usecase.ResetTaskFailures`：
- 调用 `ResetFailureMetadata` 清零 `metadata_json.failure_count/last_error/recent_failures`
- 通过 `UpdateTask` 设置 `enabled=true`、`status=active`

### 7.6 状态机

`cron_task.status` 状态机（满足 AS-FSM-01）：

```
                  创建
                   │
                   ▼
                ┌──────┐
        ┌───►   │active│   ◄─── ResetTaskFailures
        │       └──┬───┘
        │          │ toggle off / once 执行后
        │          ▼
        │       ┌──────┐
        │       │paused│
        │       └──┬───┘
        │          │ toggle on
        └──────────┘
                   │ 连续失败 ≥3 次（schedule）
                   ▼
                ┌──────┐
                │ dead │   ──► ResetTaskFailures ──► active
                └──────┘
                   │ DeleteTask
                   ▼
                ┌──────┐
                │deleted│  （软删，deleted_at 非空）
                └──────┘
```

**合法转换**：
- `active ↔ paused`：toggle enabled
- `active → dead`：schedule 连续失败 ≥3 次
- `dead → active`：ResetTaskFailures
- `active → paused`：once 任务执行后（schedule）
- `* → deleted`：DeleteTask（软删）

### 7.7 重试策略

`dispatchWithRetry` 按 `retry_max_attempts` 退避：

| 尝试 | 延迟 |
|------|------|
| 第 1 次重试 | 30 秒 |
| 第 2 次重试 | 2 分钟 |
| 第 3 次重试 | 10 分钟 |

- `retry_max_attempts` 未设置/null → 默认 3（完整 30s/2m/10m 退避）
- `retry_max_attempts = 0` → 禁用重试（仅 1 次尝试）
- `retry_max_attempts = N` (1 ≤ N < 3) → 取 `defaultRetryBackoff[:N]`
- Panic 通过 `dispatchSafe` 的 `recover()` 捕获，视为硬失败进入重试计划

定义在 `internal/cronrunner/runner.go`：

```go
var defaultRetryBackoff = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}
const maxDeadFailures = 3
```

`internal/cronrunner/schedule.go`：

```go
const defaultRetryMaxAttempts = 3

func effectiveRetryMaxAttempts(cfg cronTaskConfig) int  // nil → 3, 否则取 *RetryMaxAttempts
func retryPlan(maxRetries int) (attempts int, backoff []time.Duration)  // maxRetries<=0 → (1, nil)
```

### 7.8 死信状态

当一个任务在多次**调度**运行中累计 **3 次连续失败**时（manual 不计入），转入 `dead` 状态：

- `cron_task.status` 设为 `"dead"`
- `cron_task.enabled` 设为 `false`
- 内部事件总线发出 `cron.dead_letter` 管理告警事件，元数据：
  ```json
  { "job_id": "…", "task_key": "…", "name": "…" }
  ```
- Prometheus `aranea_cron_job_dead_total` 计数 +1

死信任务**不再调度**，直到手动重置（`ResetTaskFailures` 将 `enabled = true`、`status = "active"`、`failure_count = 0`）。

### 7.9 Prometheus 指标

| 指标 | 类型 | 标签 | 说明 |
|------|------|------|------|
| `aranea_cron_job_runs_total` | Counter | `job_id`, `status` | 按结果的总执行次数（`success`/`failure`/`skipped`） |
| `aranea_cron_job_duration_seconds` | Histogram | `job_id` | 每次执行的挂钟时间 |
| `aranea_cron_job_dead_total` | Counter | `job_id` | 任务进入死信状态的次数 |

`duration_seconds` 桶：`0.5s, 1s, 5s, 15s, 30s, 60s, 120s, 300s, 600s`

### 7.10 持久化到数据库的失败字段

| 数据库列 | 更新时机 |
|----------|----------|
| `cron_task.metadata_json.failure_count` | 每次 schedule 失败运行（manual 不递增） |
| `cron_task.metadata_json.last_error` | 最新错误消息 |
| `cron_task_run.status` | `"success"` / `"failure"` / `"skipped"`（每次运行） |
| `cron_task_run.error_message` | 错误文本 |
| `cron_task_run.finished_at` | 完成时间戳 |

### 7.11 文件拆分

| 文件 | 职责 |
|------|------|
| `runner.go` | Runner 结构、Start、runDue、TriggerTask、dispatchWithRetry、dispatchSafe、dispatchCronTask、resolveCronAgent、persistMetadata、postChat、sessionBusyErr、publishDeadLetterEvent、publishTeamCronMaybe、Prometheus 指标定义 |
| `execute.go` | lockTask、insertPendingRun、runDispatch、finalizeRun、finishTaskRun、executeTask、runManualTask、isSessionBusyErr、recordPreExecuteOutcome、recordScheduleFailure |
| `schedule.go` | cronTaskConfig / cronTaskMetadata / cronFailureSummary 结构、parseCronTaskConfig / parseCronTaskMetadata、cronTaskDueAt / nextCronRunAfter / parseCronRunAt / nextCronExpressionTime / cronFieldMatches / cronWeekdayMatches、cronTargetType、effectiveRetryMaxAttempts / retryPlan、mustMarshalJSON |
| `errors.go` | errRunAtRequired / errInvalidRunAt / errCronFields / errCronNoSlot |
| `seed_model_registry.go` | SeedModelRegistrySyncTask（注入模型注册表同步种子任务） |

---

## 八、Service 层

### 8.1 CronService

文件：`internal/service/cron.go`

```go
type CronService struct {
    v1.UnimplementedCronServiceServer
    uc *biz.CronUsecase
}

func NewCronService(uc *biz.CronUsecase) *CronService
```

| 方法 | 转换逻辑 |
|------|----------|
| `ListCronTasks` | `uc.ListTasks` → 逐条 `toProtoCronTask` |
| `CreateCronTask` | `req` → `biz.CronTask` → `uc.CreateTask` → `toProtoCronTask` |
| `GetCronTask` | `uc.GetTask` → `toProtoCronTask`；`sql.ErrNoRows` → `NotFound` |
| `UpdateCronTask` | `req.Task` 为 nil → `BadRequest`；`patchFromProtoCronTask` → `uc.UpdateTask` → `toProtoCronTask` |
| `DeleteCronTask` | `uc.DeleteTask` → `emptypb.Empty` |
| `ListCronTaskRuns` | `req` → `biz.CronTaskRunQuery` → `uc.ListTaskRuns` → 逐条 `toProtoCronTaskRun` |
| `TriggerCronTask` | `uc.TriggerTask` → `toProtoCronTaskRun`；异步执行，立即返回 `pending` run |
| `ResetCronTaskFailures` | `uc.ResetTaskFailures` → `toProtoCronTask` |
| `GetTaskRun`（非 RPC） | `uc.GetTaskRun`，供内部异步完成观察者使用 |

**类型转换函数**：
- `toProtoCronTask(biz.CronTask) *v1.CronTask`
- `patchFromProtoCronTask(*v1.CronTask) biz.CronTaskPatch` — 所有字段都用 `StrPtr`/`BoolPtr`/`IntPtr` 包装（指针非 nil 即触发 patch）
- `toProtoCronTaskRun(biz.CronTaskRun) *v1.CronTaskRun`

### 8.2 CronTriggerGateway 适配器

文件：`internal/service/cron_trigger_gateway_adapter.go`

```go
type cronTriggerGatewayAdapter struct {
    svc *CronService
}

func NewCronTriggerGatewayAdapter(svc *CronService) biz.CronTriggerGateway
```

实现 `biz.CronTriggerGateway` 接口（`TriggerCronTask` / `GetTaskRun`），将 gateway 调用桥接到 `CronService.uc`。

---

## 九、Wire 注入

`cmd/admin/wire.go`：

```go
func provideCronRunnerDeps(
    cron biz.CronRepo,
    session *biz.SessionUsecase,
    teams biz.TeamReader,
    agents biz.AgentRepository,
    eventBus event.Bus,
    chat *service.ChatService,
    registrySyncAgent cronrunner.CronRegistrySyncAgent,
) cronrunner.Deps

func provideCronRunner(deps cronrunner.Deps, lg loggateway.Logger) *cronrunner.Runner {
    if strings.TrimSpace(os.Getenv("CRON_RUNNER_DISABLED")) == "1" {
        return nil
    }
    return cronrunner.NewRunner(deps, lg)
}
```

**ProviderSet**：
- `data.ProviderSet → NewCronRepo`
- `biz.ProviderSet → NewCronUsecase`
- `service.ProviderSet → NewCronService` + `NewCronTriggerGatewayAdapter`
- `cronrunner.ProviderSet → NewRunner`

**启动**：`cmd/admin/workers.go` 中 `goAfterReady("cron", func() { cfg.CronRunner.Start(ctx, interval) })`，`interval = cronrunner.DefaultInterval()`；随进程 ctx 取消退出。

---

## 十、Web 前端设计

### 10.1 文件结构

```
web/src/
├── features/cron/
│   ├── api.ts                          # API 调用与 wire 转换
│   ├── types.ts                        # TypeScript 类型定义
│   ├── useCronTasksPage.ts             # 列表页 Composable
│   └── cronTableUi.ts                  # 表格 UI 辅助
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

### 10.2 页面路由

`web/src/router/routes.ts`：

| 路由 | 页面 | 说明 |
|------|------|------|
| `/cron` | `CronTasksPage.vue` | 定时任务管理主页（含执行历史弹窗 `CronRunsDialog`） |

### 10.3 CronTasksPage.vue

| 区域 | 组件 | 说明 |
|------|------|------|
| Hero | `AppPageHero` | kicker="Scheduled tasks"、title="定时任务"、subtitle="安排定期 Agent 任务..."；actions 含「执行历史」「新建任务」 |
| 工具栏 | `AppPageToolbar` | 搜索 `QInput`、状态筛选 `QSelect`、计数提示、重置/刷新按钮 |
| 错误 | `QBanner` | `bg-negative` 显示错误，含「重试」按钮 |
| 空状态 | `QCard` + `QAvatar` + `QBtn` | 图标 `schedule`，文案「暂无定时任务」，新建按钮 |
| 表格 | `AppRegistryTable` | 客户端分页与搜索（当前 `ListCronTasks` 无 search/page 参数，全量返回后前端过滤） |

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
| 操作 | — | 启用/暂停 `QToggle` + 编辑/删除 `QBtn flat dense` + 历史 `QBtn icon="history"` + 立即执行 + dead 任务「重置失败计数」`QBtn icon="restart_alt"` |

### 10.4 CronTaskFormDialog.vue

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

### 10.5 CronRunsDialog.vue

> 执行历史以弹窗形式嵌入 `CronTasksPage`，非独立路由页面。

| 区域 | 组件 | 说明 |
|------|------|------|
| 筛选器 | `QSelect` + `QInput` | 定时任务（可清空=全部）、状态 success/failure/pending/skipped |
| 表格 | `QTable` | 列：任务名称、started_at、finished_at、status、error_message 摘要、trigger、run_id；前端分页 |

### 10.6 API

文件：`web/src/features/cron/api.ts`

```typescript
export async function listCronTasks(): Promise<PlatformResource[]>
export async function createCronTask(payload: PlatformResourceInput): Promise<PlatformResource>
export async function updateCronTask(id: string, payload: Partial<PlatformResourceInput>): Promise<PlatformResource>
export async function deleteCronTask(id: string): Promise<void>
export async function triggerCronTask(id: string): Promise<CronTaskRun>
export async function resetCronTaskFailures(id: string): Promise<PlatformResource>
export async function listCronTaskRuns(query: CronTaskRunQuery): Promise<CronTaskRun[]>
export async function listCronAgents(): Promise<Agent[]>
export async function listCronTeams(): Promise<Team[]>
export function parseCronConfig(row: PlatformResource): CronTaskConfig
export function parseCronMetadata(row: PlatformResource): CronTaskMetadata
```

**设计说明**：前端使用 `PlatformResource` 作为统一资源类型（与平台资源管理器共用），通过 `wireCronTask` / `wireCronTaskRun` 将 Proto wire 类型转换为前端类型。

### 10.7 类型定义

文件：`web/src/features/cron/types.ts`

```typescript
export type CronScheduleType = 'interval' | 'cron' | 'once';
export type CronTaskStatus = 'active' | 'paused' | 'dead' | string;
export type CronRunStatus = 'pending' | 'success' | 'failure' | 'skipped' | string;

export type CronTaskConfig = {
  target_type?: 'agent' | 'team';  // 前端表单只暴露 agent/team；model_registry_sync 由后端种子任务注入
  team_id?: string;
  schedule_type?: CronScheduleType;
  cron_expression?: string;
  interval_seconds?: number;
  run_at?: string;
  timezone?: string;
  message?: string;
  retry_max_attempts?: number;
};

export type CronFailureSummary = { started_at?: string; error_message?: string };

export type CronTaskMetadata = {
  run_count?: number;
  success_count?: number;
  failure_count?: number;
  last_run_at?: string;
  last_run_status?: CronRunStatus;
  last_error?: string;
  next_run_at?: string;
  recent_failures?: CronFailureSummary[];
  [key: string]: unknown;
};

export type CronTaskFormValue = {
  name: string; display_name: string; description: string;
  target_type: 'agent' | 'team'; agent_id: string; team_id: string;
  schedule_type: CronScheduleType;
  interval_minutes: number; cron_expression: string;
  run_at_date: string; run_at_time: string; timezone: string;
  message: string; retry_max_attempts: number; enabled: boolean;
};

export type CronTaskRow = PlatformResource;

export type CronTaskRun = {
  id: string; task_id: string; task_name: string;
  status: CronRunStatus;
  started_at: string; finished_at: string;
  trigger: string; run_id: string;
  output_json: string; error_message: string; created_at: string;
};

export type CronTaskRunQuery = { cron_task_id?: string; status?: string; limit?: number };
```

---

## 十一、UX 规范

- **主题**：遵循项目统一主题（`app-registry-page` / `app-empty-state-center` 等类名）
- **空状态**：居中列布局，`QAvatar` 80px + `text-h6` 主文案 + `text-body2 text-grey-7` 次文案 + 主按钮
- **表格**：`AppRegistryTable` 客户端分页，`hide-pagination` + `pagination="{ rowsPerPage: 0 }"`
- **失败次数交互**：`failure_count > 0` 时 `text-negative` + `QTooltip` 展示最近失败摘要，点击打开 `CronRunsDialog`
- **dead 任务**：操作列额外显示 `restart_alt` 图标的「重置失败计数」按钮
- **对话框**：`QDialog` + `QCard`，`min-width: 420px; max-width: 560px`，标题栏含关闭按钮
- **表单校验**：`QForm @submit.prevent` 统一触发，名称校验 `/^[a-z0-9]+(-[a-z0-9]+)*$/`

---

*文档版本：2.0 — 按三件套内容边界重组：合并需求文档迁移来的数据模型/API 契约/执行设计/运维指南，对齐代码真实状态（biz/cron 子包、NewUsecase 签名、Deps 字段名、文件拆分、CronTriggerGateway 适配器）。*
