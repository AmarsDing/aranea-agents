# Multi-Agent Team 编排模块 — 实现设计文档

> 对应需求：`11 multi-agent.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Team 编排：Coordinator/Swarm 模式、成员 Agent 管理、Team Runner 执行、SSE 事件流。核心包 `internal/team`。

---

## 二、Proto 层

### 2.1 现有 Proto

文件：`api/kratos/team/v1/team.proto`

```protobuf
service TeamService {
  rpc ListTeams(ListTeamsRequest) returns (ListTeamsResponse) {
    option (google.api.http) = { get: "/v1/teams" };
  }
  rpc CreateTeam(CreateTeamRequest) returns (Team) {
    option (google.api.http) = { post: "/v1/teams" body: "*" };
  }
  rpc GetTeam(GetTeamRequest) returns (Team) {
    option (google.api.http) = { get: "/v1/teams/{id}" };
  }
  rpc UpdateTeam(UpdateTeamRequest) returns (Team) {
    option (google.api.http) = { patch: "/v1/teams/{id}" body: "*" };
  }
  rpc DeleteTeam(DeleteTeamRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/teams/{id}" };
  }
}
```

### 2.2 待新增

| RPC | 路径 | 用途 |
|-----|------|------|
| `RunTeamTurn` | `POST /v1/teams/{id}/run` | 手动触发 Team turn |
| `GetTeamRunStatus` | `GET /v1/teams/{id}/runs/{run_id}` | 运行状态 |
| `ListTeamRuns` | `GET /v1/teams/{id}/runs` | 运行历史 |

---

## 三、Biz 层

### 3.1 领域模型

```go
type Team struct {
    ID              string
    TeamKey         string
    DisplayName     string
    Description     string
    Mode            string  // "coordinator" | "swarm"
    CoordinatorID   string  // 协调者 Agent ID
    Status          string
    CreatedAt       string
    UpdatedAt       string
    Members         []TeamMember
}

type TeamMember struct {
    ID       string
    TeamID   string
    AgentID  string
    Role     string  // "coordinator" | "member"
    SortOrder int32
}

type TeamRun struct {
    ID        string
    TeamID    string
    SessionID string
    Status    string  // "running" | "completed" | "failed"
    StartedAt string
    EndedAt   string
    Steps     []TeamRunStep
}
```

### 3.2 Usecase

```go
type TeamUsecase struct {
    repo   TeamRepository
    broker *TeamRunEventBroker
}

func (uc *TeamUsecase) List(ctx, query) (TeamListResult, error)
func (uc *TeamUsecase) Create(ctx, t Team) (Team, error)
func (uc *TeamUsecase) Update(ctx, t Team) (Team, error)
func (uc *TeamUsecase) Delete(ctx, id) error
```

### 3.3 SSE 事件

```go
type TeamRunEventBroker struct {
    mu    sync.RWMutex
    chans map[string]chan *TeamRunEvent
}

type TeamRunEvent struct {
    TeamID    string
    RunID     string
    Type      string  // "agent_start" | "agent_done" | "tool_call" | "transfer" | "done" | "error"
    AgentID   string
    Content   string
    Timestamp string
}
```

---

## 四、Data 层

### 4.1 Ent Schema

- `internal/data/ent/schema/team.go` — Team 主表
- `internal/data/ent/schema/team_run.go` — Team 运行记录
- `internal/data/ent/schema/team_run_step.go` — 运行步骤

### 4.2 关键字段

**team**：
- `team_key` (TEXT, unique)
- `display_name` (TEXT)
- `mode` (TEXT, "coordinator"/"swarm")
- `coordinator_id` (TEXT, optional)

**team_run**：
- `team_id` (TEXT)
- `session_id` (TEXT)
- `status` (TEXT)

---

## 五、运行时层

### 5.1 Team Runner

```go
// internal/team/runner.go
type Runner struct {
    teams    biz.TeamRepository
    sessions *biz.SessionUsecase
    agents   biz.AgentRepository
    agentsUC *biz.AgentUsecase
    tools    biz.ToolRepo
    llm      *biz.LlmProviderModelUsecase
    broker   *biz.TeamRunEventBroker
    skills   *biz.SkillUsecase
    sys      biz.SystemSettingRepo
    rt       *runtimedeps.Runtime
    compress biz.NativeTurnCompressor
    logs     *biz.MonitorLogBroker
}

func (r *Runner) RunTurn(ctx, team, session, agents, msg) (<-chan agent.Event, error)
```

### 5.2 Team 构建

```go
// internal/team/trpc_build.go
func BuildTRPCTeam(ctx, team, members, deps) (agent.Agent, error)
func BuildWorkflowRoot(ctx, team, members, deps) (agent.Agent, error)
```

Coordinator 模式：协调者 Agent + 成员作为 `AgentTool`
Swarm 模式：成员间 `transfer_to_agent` 传递控制权

### 5.3 crossRequestTransfer（待实现）

```go
// internal/team/trpc_build.go
func WithCrossRequestTransfer(enabled bool) team.Option
```

---

## 六、Service 层

```go
func (s *TeamService) ListTeams(ctx, req) (*ListTeamsResponse, error)
func (s *TeamService) CreateTeam(ctx, req) (*Team, error)
func (s *TeamService) UpdateTeam(ctx, req) (*Team, error)
func (s *TeamService) DeleteTeam(ctx, req) (*emptypb.Empty, error)
```

---

## 七、Wire 注入

已有：
```
data.ProviderSet → NewTeamRepo
biz.ProviderSet → NewTeamUsecase, NewTeamRunEventBroker
service.ProviderSet → NewTeamService
team.ProviderSet → NewRunner
```

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/features/teams/
├── api.ts
├── types.ts
└── components/
    ├── TeamListPage.vue
    ├── TeamCreateDialog.vue
    ├── TeamSettingsPage.vue
    └── TeamMemberEditor.vue
```

### 8.2 组件设计

**TeamCreateDialog.vue**：

| 控件 | 绑定 | 说明 |
|------|------|------|
| `QInput` 名称 | `displayName` | 必填 |
| `QInput` 标识 | `teamKey` | 必填 |
| `QSelect` 模式 | `mode` | coordinator/swarm |
| `QSelect` 协调者 | `coordinatorId` | coordinator 模式必选 |
| Agent 列表 | `members` | 多选成员 |

**TeamSettingsPage.vue**：

| Tab | 内容 |
|-----|------|
| 成员 | 成员列表 + 添加/移除 + 拖拽排序 |
| 设置 | 模式切换 + 高级配置 |
| 运行历史 | TeamRun 列表 + 状态 |

### 8.3 API

```typescript
export async function listTeams(query: TeamListQuery): Promise<TeamListResult>
export async function createTeam(req: CreateTeamRequest): Promise<Team>
export async function updateTeam(id: string, req: UpdateTeamRequest): Promise<Team>
export async function deleteTeam(id: string): Promise<void>
```
