# Cron 定时任务模块 — 实现设计文档

> 对应需求：`21 cron.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

定时任务调度：Cron 表达式触发 Agent/Team 执行、任务管理、执行日志。

---

## 二、Proto 层

### 2.1 待新增

```protobuf
service CronService {
  rpc ListCronJobs(ListCronJobsRequest) returns (ListCronJobsResponse) {
    option (google.api.http) = { get: "/v1/cron-jobs" };
  }
  rpc CreateCronJob(CreateCronJobRequest) returns (CronJob) {
    option (google.api.http) = { post: "/v1/cron-jobs" body: "*" };
  }
  rpc UpdateCronJob(UpdateCronJobRequest) returns (CronJob) {
    option (google.api.http) = { patch: "/v1/cron-jobs/{id}" body: "*" };
  }
  rpc DeleteCronJob(DeleteCronJobRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/cron-jobs/{id}" };
  }
  rpc ToggleCronJob(ToggleCronJobRequest) returns (CronJob) {
    option (google.api.http) = { patch: "/v1/cron-jobs/{id}/toggle" };
  }
  rpc ListCronRuns(ListCronRunsRequest) returns (ListCronRunsResponse) {
    option (google.api.http) = { get: "/v1/cron-jobs/{id}/runs" };
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type CronJob struct {
    ID          string
    Name        string
    CronExpr    string
    TargetType  string  // "agent"/"team"
    TargetID    string
    Payload     string  // 触发消息
    Enabled     bool
    LastRunAt   string
    NextRunAt   string
    CreatedAt   string
    UpdatedAt   string
}

type CronRun struct {
    ID        string
    JobID     string
    Status    string  // "running"/"completed"/"failed"
    StartedAt string
    EndedAt   string
    Result    string
}
```

### 3.2 Usecase

```go
type CronUsecase struct {
    repo    CronRepository
    scheduler *cron.Scheduler
}

func (uc *CronUsecase) List(ctx, query) (CronJobListResult, error)
func (uc *CronUsecase) Create(ctx, j CronJob) (CronJob, error)
func (uc *CronUsecase) Update(ctx, j CronJob) (CronJob, error)
func (uc *CronUsecase) Delete(ctx, id) error
func (uc *CronUsecase) Toggle(ctx, id) (CronJob, error)
```

---

## 四、Data 层

### 4.1 Ent Schema

- `internal/data/ent/schema/cron_job.go` — 任务表
- `internal/data/ent/schema/cron_run.go` — 执行记录表

### 4.2 调度器

```go
// internal/cron/scheduler.go
type Scheduler struct {
    cron    *cron.Cron
    usecase *biz.CronUsecase
}

func (s *Scheduler) Start(ctx) error
func (s *Scheduler) Stop() error
func (s *Scheduler) AddJob(j biz.CronJob) error
func (s *Scheduler) RemoveJob(id string) error
```

---

## 五、Service 层

```go
func (s *CronService) ListCronJobs(ctx, req) (*ListCronJobsResponse, error)
func (s *CronService) CreateCronJob(ctx, req) (*CronJob, error)
func (s *CronService) UpdateCronJob(ctx, req) (*CronJob, error)
func (s *CronService) DeleteCronJob(ctx, req) (*emptypb.Empty, error)
func (s *CronService) ToggleCronJob(ctx, req) (*CronJob, error)
```

---

## 六、Wire 注入

待新增：
```
data.ProviderSet → NewCronRepo
biz.ProviderSet → NewCronUsecase
service.ProviderSet → NewCronService
cron.ProviderSet → NewScheduler
```

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/features/cron/
├── api.ts
├── types.ts
├── CronJobEditorDialog.vue
├── CronRunList.vue
└── components/
    └── CronJobListPage.vue
```

### 7.2 组件设计

**CronJobEditorDialog.vue**：

| 控件 | 绑定 | 说明 |
|------|------|------|
| `QInput` 名称 | `name` | 必填 |
| `QInput` Cron 表达式 | `cronExpr` | 必填 + 预览下次执行时间 |
| `QSelect` 目标类型 | `targetType` | agent/team |
| `QSelect` 目标 | `targetID` | 依赖类型动态加载 |
| `QInput` 触发消息 | `payload` | 必填 |
| `QToggle` 启用 | `enabled` | 默认启用 |

### 7.3 API

```typescript
export async function listCronJobs(query: CronJobQuery): Promise<CronJobListResult>
export async function createCronJob(req: CreateCronJobRequest): Promise<CronJob>
export async function updateCronJob(id: string, req: UpdateCronJobRequest): Promise<CronJob>
export async function deleteCronJob(id: string): Promise<void>
export async function toggleCronJob(id: string): Promise<CronJob>
export async function listCronRuns(jobId: string): Promise<CronRunListResult>
```
