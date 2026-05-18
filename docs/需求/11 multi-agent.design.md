# Multi-Agent Team 编排模块 — 实现设计文档

> 对应需求：`11 multi-agent.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> 更新时间：2026-05-18

---

## 一、模块概述

Team 多智能体编排系统：支持 Sequential、Parallel、Coordinator、Critic Loop、Swarm 五种编排模式。
基于 trpc-agent-go 框架，已完成 SwarmConfig 安全限制、CrossRequestTransfer 跨请求转移、
SwarmHandoffInputBuilder 自定义转移输入、MemberToolConfig 成员工具配置、
动态成员管理（UpdateSwarmMembers）以及结构导出（ExportTeamStructure）。

### 当前实现状态

| 模块 | 状态 | 说明 |
|------|------|------|
| Proto CRUD + DuplicateTeam | ✅ 已完成 | ListTeams / CreateTeam / GetTeam / UpdateTeam / DeleteTeam / DuplicateTeam |
| Proto Run 管理 | ✅ 已完成 | ListTeamRuns / GetTeamRun / CancelTeamRun / ListTeamRunSteps |
| Proto 高级功能 | ✅ 已完成 | UpdateSwarmMembers / ExportTeamStructure / RunTeamTest |
| 五种编排模式 | ✅ 已完成 | sequential / parallel / coordinator / critic_loop / swarm |
| Definition 解析与校验 | ✅ 已完成 | `internal/team/definition.go` |
| Team 运行时 | ✅ 已完成 | `internal/team/runner_team_trpc.go` |
| SwarmConfig 安全限制 | ✅ 已完成 | MaxHandoffs / NodeTimeout / RepetitiveHandoff |
| CrossRequestTransfer | ✅ 已完成 | 跨请求转移开关 |
| SwarmHandoffInputBuilder | ✅ 已完成 | 自定义转移输入构建 |
| MemberToolConfig | ✅ 已完成 | StreamInner / InnerTextMode / HistoryScope / SkipSummarization |
| 动态成员管理 | ✅ 已完成 | UpdateSwarmMembers API |
| 结构导出 | ✅ 已完成 | ExportTeamStructure API |
| escalationFunc 增强 | ✅ 已完成 | 支持 ScoreThreshold 结构化评分 |
| Usecase 层 | ✅ 已完成 | `internal/biz/team_usecase.go` |
| Data 层 | ✅ 已完成 | `internal/data/team_repo.go` |
| Service 层 | ✅ 已完成 | `internal/service/team.go` |
| SSE Broker | ✅ 已完成 | `internal/biz/team_run_events.go` |
| 前端 Team 管理页 | ✅ 已完成 | TeamCard / TeamToolbar / TeamEditorDialog / TeamRunsDialog |
| A2A call_agent 工具 | ❌ 缺失 | Agent 无法在 Team 中调用远程 Agent |
| member_* SSE 事件 | ❌ 缺失 | Team 对话不发射子 Agent 实时流事件 |
| Team 运行结果结构化汇总 | ❌ 缺失 | 无成员贡献度 / 工具调用统计 |
| RunTeamTest 端到端 | ⏳ 桩实现 | Service 层返回 Unimplemented |

### 核心架构

```
用户消息 → Team.Run()
             ↓
         switch mode:
           ┌─ ModeCoordinator → coordinator.Run() → 成员作为 AgentTool 调用
           ├─ ModeSwarm       → entryMember.Run() → transfer_to_agent 自由转移
           ├─ Sequential      → chainagent 顺序执行
           ├─ Parallel        → parallelagent 并行执行
           └─ Critic Loop     → cycleagent 迭代执行
             ↓
         事件流 → Runner 事件循环 → 持久化 + SSE 推送
```

---

## 二、Proto 层

### 2.1 完整 Proto 定义

文件：`api/kratos/team/v1/team.proto`

核心消息：

| 消息 | 用途 |
|------|------|
| `Team` | Team 实体 |
| `TeamRun` | 运行记录 |
| `TeamRunStep` | 运行步骤 |
| `StructureNode` / `StructureEdge` / `StructureSurface` | 结构导出节点/边/面 |

核心 RPC：

| RPC | HTTP | 用途 |
|-----|------|------|
| `ListTeams` | `GET /v1/teams` | 列出所有未删除 Team |
| `CreateTeam` | `POST /v1/teams` | 创建 Team |
| `GetTeam` | `GET /v1/teams/{id}` | 获取单个 Team |
| `UpdateTeam` | `PATCH /v1/teams/{id}` | 更新 Team |
| `DeleteTeam` | `DELETE /v1/teams/{id}` | 软删除 Team |
| `DuplicateTeam` | `POST /v1/teams/{id}/duplicate` | 复制 Team |
| `ListTeamRuns` | `GET /v1/team-runs` | 列出 Team 运行记录 |
| `GetTeamRun` | `GET /v1/team-runs/{id}` | 获取单条运行详情 |
| `CancelTeamRun` | `POST /v1/team-runs/{id}/cancel` | 取消正在运行的 Team Run |
| `ListTeamRunSteps` | `GET /v1/team-runs/{run_id}/steps` | 列出运行步骤 |
| `UpdateSwarmMembers` | `POST /v1/teams/{team_id}/swarm-members` | Swarm 动态成员管理 |
| `ExportTeamStructure` | `GET /v1/teams/{team_id}/structure` | 导出 Team 结构快照 |
| `RunTeamTest` | `POST /v1/teams/{id}/run-test` | 手动触发 Team 测试运行 |

---

## 三、Biz 层

### 3.1 领域模型

文件：`internal/biz/team_types.go`

```go
type Team struct {
    ID             string
    TeamKey        string
    DisplayName    string
    Status         string   // "draft" | "active" | "archived" | "deleted"
    IsDefault      bool
    DefinitionJSON string
    ADKAppName     string
    CreatedAt      string
    UpdatedAt      string
    DeletedAt      string
}

type TeamRun struct {
    ID            string   `json:"id"`
    TeamID        string   `json:"team_id"`
    SessionID     string   `json:"session_id"`
    MessageID     string   `json:"message_id"`
    Mode          string   `json:"mode"`
    Status        string   `json:"status"`
    InputPreview  string   `json:"input_preview"`
    OutputPreview string   `json:"output_preview"`
    TokenIn       int      `json:"token_in"`
    TokenOut      int      `json:"token_out"`
    CostMicroUSD  int64    `json:"cost_micro_usd"`
    DurationMS    int      `json:"duration_ms"`
    ErrorMessage  string   `json:"error_message"`
    TopologyJSON  string   `json:"topology_json"`
    StartedAt     string   `json:"started_at"`
    FinishedAt    string   `json:"finished_at"`
    CreatedAt     string   `json:"created_at"`
    UpdatedAt     string   `json:"updated_at"`
}

type TeamRunStep struct {
    ID            string   `json:"id"`
    RunID         string   `json:"run_id"`
    TeamID        string   `json:"team_id"`
    AgentID       string   `json:"agent_id"`
    AgentKey      string   `json:"agent_key"`
    AgentName     string   `json:"agent_name"`
    Role          string   `json:"role"`
    SortOrder     int      `json:"sort_order"`
    Status        string   `json:"status"`
    InputPreview  string   `json:"input_preview"`
    OutputPreview string   `json:"output_preview"`
    TokenIn       int      `json:"token_in"`
    TokenOut      int      `json:"token_out"`
    CostMicroUSD  int64    `json:"cost_micro_usd"`
    DurationMS    int      `json:"duration_ms"`
    ErrorMessage  string   `json:"error_message"`
    StartedAt     string   `json:"started_at"`
    FinishedAt    string   `json:"finished_at"`
    CreatedAt     string   `json:"created_at"`
}

type TeamStructureSnapshot struct {
    EntryNodeID string
    Nodes       []StructureNode
    Edges       []StructureEdge
    Surfaces    []StructureSurface
}

type StructureNode struct {
    NodeID string
    Kind   string
    Name   string
}

type StructureEdge struct {
    FromNodeID string
    ToNodeID   string
}

type StructureSurface struct {
    NodeID string
    Name   string
}
```

### 3.2 Team Definition 结构

文件：`internal/team/definition.go`

```go
type Definition struct {
    Version             int               `json:"version"`
    Mode                string            `json:"mode"`
    SynthesizerAgentID  string            `json:"synthesizer_agent_id"`
    Members             []MemberDef       `json:"members"`
    MaxConcurrency      int               `json:"max_concurrency"`
    TimeoutSeconds      int               `json:"timeout_seconds"`
    LoopMaxIterations   int               `json:"loop_max_iterations,omitempty"`
    CriticLoop          *CriticLoopConfig `json:"critic_loop,omitempty"`
    IntentAnchorAgentID string            `json:"intent_anchor_agent_id,omitempty"`
    Swarm               *SwarmConfigDef   `json:"swarm,omitempty"`
    MemberTool          *MemberToolDef    `json:"member_tool_config,omitempty"`
}

type SwarmConfigDef struct {
    MaxHandoffs               int  `json:"max_handoffs"`
    NodeTimeoutSeconds        int  `json:"node_timeout_seconds"`
    RepetitiveHandoffWindow   int  `json:"repetitive_handoff_window"`
    RepetitiveHandoffMinUnique int `json:"repetitive_handoff_min_unique"`
    CrossRequestTransfer      bool `json:"cross_request_transfer"`
}

type MemberToolDef struct {
    StreamInner       bool   `json:"stream_inner"`
    InnerTextMode     string `json:"inner_text_mode"`
    SkipSummarization bool   `json:"skip_summarization"`
    HistoryScope      string `json:"history_scope"`
    ToolSetName       string `json:"tool_set_name"`
}

type CriticLoopConfig struct {
    MaxIterations  int     `json:"max_iterations"`
    ScoreThreshold float64 `json:"score_threshold"`
}
```

### 3.3 Repo 接口

文件：`internal/biz/team_usecase.go`

```go
type TeamRepository interface {
    ListTeams(ctx context.Context) ([]Team, error)
    GetTeamByID(ctx context.Context, id string) (Team, error)
    CreateTeam(ctx context.Context, t Team) (Team, error)
    UpdateTeam(ctx context.Context, t Team) (Team, error)
    DeleteTeam(ctx context.Context, id string) error
    ListTeamRuns(ctx context.Context, teamID string, limit int) ([]TeamRun, error)
    GetTeamRunByID(ctx context.Context, id string) (TeamRun, error)
    ListTeamRunSteps(ctx context.Context, runID string) ([]TeamRunStep, error)
    CreateTeamRun(ctx context.Context, r TeamRun) (TeamRun, error)
    UpdateTeamRun(ctx context.Context, r TeamRun) error
    CreateTeamRunStep(ctx context.Context, s TeamRunStep) (TeamRunStep, error)
}
```

### 3.4 Usecase 方法

```go
type TeamUsecase struct {
    repo TeamRepository
}

func NewTeamUsecase(repo TeamRepository) *TeamUsecase

func (u *TeamUsecase) List(ctx context.Context) ([]Team, error)
func (u *TeamUsecase) Get(ctx context.Context, id string) (Team, error)
func (u *TeamUsecase) Create(ctx context.Context, in Team) (Team, error)
func (u *TeamUsecase) Update(ctx context.Context, id string, patch Team) (Team, error)
func (u *TeamUsecase) Delete(ctx context.Context, id string) error
func (u *TeamUsecase) Duplicate(ctx context.Context, id string) (Team, error)
func (u *TeamUsecase) ListRuns(ctx context.Context, teamID string, limit int) ([]TeamRun, error)
func (u *TeamUsecase) GetRun(ctx context.Context, id string) (TeamRun, error)
func (u *TeamUsecase) ListRunSteps(ctx context.Context, runID string) ([]TeamRunStep, error)
func (u *TeamUsecase) UpdateSwarmMembers(ctx context.Context, teamID string, addIDs []string, removeIDs []string) (bool, error)
func (u *TeamUsecase) ExportStructure(ctx context.Context, teamID string) (*TeamStructureSnapshot, error)
```

### 3.5 校验规则

**validateTeamDefinition**：

- `mode` 必须为 `sequential` / `parallel` / `coordinator` / `critic_loop` / `swarm` / `adaptive`
- 至少一个 enabled member
- `parallel` 模式必须有 `synthesizer` 成员或 `synthesizer_agent_id`
- `critic_loop` 模式必须有 `generator` 和 `critic` 成员
- 每个 member 的 `agent_id` 非空

**Create 校验**：

- `team_key` 和 `display_name` 必填
- `definition_json` 为空时填充默认值
- 调用 `validateTeamDefinition` 校验 JSON 结构

**Update 合并**：

- `TeamKey` / `DisplayName` / `Status` / `DefinitionJSON`：空值不覆盖
- `ADKAppName`：空值回退到 `TeamKey`
- 合并后再次调用 `validateTeamDefinition`

**Delete 规则**：

- 默认 Team（`IsDefault=true`）不允许删除，返回 `kerrors.Conflict`
- 非默认 Team 执行软删除

**UpdateSwarmMembers 规则**：

- 仅 `swarm` 和 `adaptive` 模式支持
- 解析 `definition_json`，移除 `remove_agent_ids` 中的成员，追加 `add_agent_ids` 为 worker 角色
- 重新校验更新后的 definition

**ExportStructure 规则**：

- 根据 mode 生成不同拓扑：
  - `coordinator`：星形拓扑，第一个成员为 coordinator，其余为 worker
  - `swarm` / `adaptive`：全连接拓扑，成员间可互相转移
  - 其他模式：线性拓扑，team 节点连接所有成员

---

## 四、Data 层

### 4.1 Ent Schema

#### teams

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string (PK) | 唯一标识 |
| team_key | string (Unique) | 唯一 Key |
| display_name | string | 显示名称 |
| status | string | 状态 |
| is_default | bool | 是否默认 |
| definition_json | text | 编排定义 JSON |
| adk_app_name | string | 运行时应用名 |
| created_at | string | 创建时间 |
| updated_at | string | 更新时间 |
| deleted_at | string | 软删除时间 |

#### team_runs

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string (PK) | 唯一标识 |
| team_id | string | 关联 Team |
| session_id | string | 关联 Session |
| message_id | string | 关联 Message |
| mode | string | 编排模式 |
| status | string | 运行状态 |
| input_preview | text | 输入预览 |
| output_preview | text | 输出预览 |
| token_in | int32 | 输入 Token |
| token_out | int32 | 输出 Token |
| cost_micro_usd | int64 | 成本 |
| duration_ms | int32 | 耗时 |
| error_message | text | 错误信息 |
| topology_json | text | 拓扑 JSON |
| started_at | string | 开始时间 |
| finished_at | string | 结束时间 |
| created_at | string | 创建时间 |
| updated_at | string | 更新时间 |

#### team_run_steps

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string (PK) | 唯一标识 |
| run_id | string | 关联 Run |
| team_id | string | 关联 Team |
| agent_id | string | 关联 Agent |
| agent_key | string | Agent Key |
| agent_name | string | Agent 名称 |
| role | string | 角色 |
| sort_order | int32 | 排序 |
| status | string | 步骤状态 |
| input_preview | text | 输入预览 |
| output_preview | text | 输出预览 |
| token_in | int32 | 输入 Token |
| token_out | int32 | 输出 Token |
| cost_micro_usd | int64 | 成本 |
| duration_ms | int32 | 耗时 |
| error_message | text | 错误信息 |
| started_at | string | 开始时间 |
| finished_at | string | 结束时间 |
| created_at | string | 创建时间 |

### 4.2 Repo 实现

文件：`internal/data/team_repo.go`

关键方法：

- `GetTeamRunByID`：按 ID 获取单条 TeamRun，支持 NotFound 错误映射
- `ListTeamRuns`：支持 teamID 过滤和 limit 分页
- `CreateTeamRun` / `UpdateTeamRun`：创建和更新运行记录
- `CreateTeamRunStep`：创建运行步骤

---

## 五、Service 层

文件：`internal/service/team.go`

### 5.1 方法实现

| 方法 | 实现说明 |
|------|---------|
| `ListTeams` | 调用 `uc.List` |
| `CreateTeam` | 调用 `uc.Create` |
| `GetTeam` | 调用 `uc.Get`，映射 NotFound |
| `UpdateTeam` | 调用 `uc.Update`，校验 team body 非空 |
| `DeleteTeam` | 调用 `uc.Delete` |
| `DuplicateTeam` | 调用 `uc.Duplicate` |
| `ListTeamRuns` | 调用 `uc.ListRuns` |
| `GetTeamRun` | 调用 `uc.GetRun`，映射 NotFound |
| `CancelTeamRun` | 校验状态为 running/pending，设置 cancelled |
| `RunTeamTest` | 桩实现，返回 501 Unimplemented |
| `ListTeamRunSteps` | 调用 `uc.ListRunSteps` |
| `UpdateSwarmMembers` | 调用 `uc.UpdateSwarmMembers` |
| `ExportTeamStructure` | 调用 `uc.ExportStructure`，映射为 Proto 结构 |

### 5.2 错误映射

```go
func mapTeamErr(err error) error {
    if stderrors.Is(err, sql.ErrNoRows) {
        return kerrors.NotFound("TEAM", "team not found")
    }
    return err
}
```

---

## 六、Team 运行时

### 6.1 编排模式映射

| 编排模式 | trpc 框架组件 | 配置选项 |
|---------|-------------|---------|
| `sequential` | `chainagent.New` | `WithSubAgents(memberAgents)` |
| `parallel` | `parallelagent.New` | `WithSubAgents(workerAgents)` |
| `coordinator` | `trpcteam.New` | `WithCoordinatorOptions(...)` |
| `critic_loop` | `cycleagent.New` | `WithMaxIterations`, `WithEscalationFunc` |
| `swarm` | `trpcteam.NewSwarm` | `WithSwarmConfig`, `WithCrossRequestTransfer`, `WithSwarmHandoffInputBuilder` |

### 6.2 Swarm 配置构建

文件：`internal/team/trpc_build.go`

```go
func buildSwarmOptions(def Definition) []trpcteam.Option {
    cfg := trpcteam.DefaultSwarmConfig()
    crossTransfer := true
    if sc := def.Swarm; sc != nil {
        if sc.MaxHandoffs > 0 {
            cfg.MaxHandoffs = sc.MaxHandoffs
        }
        if sc.NodeTimeoutSeconds > 0 {
            cfg.NodeTimeout = time.Duration(sc.NodeTimeoutSeconds) * time.Second
        }
        if sc.RepetitiveHandoffWindow > 0 {
            cfg.RepetitiveHandoffWindow = sc.RepetitiveHandoffWindow
        }
        if sc.RepetitiveHandoffMinUnique > 0 {
            cfg.RepetitiveHandoffMinUnique = sc.RepetitiveHandoffMinUnique
        }
        crossTransfer = sc.CrossRequestTransfer
    }
    return []trpcteam.Option{
        trpcteam.WithSwarmConfig(cfg),
        trpcteam.WithCrossRequestTransfer(crossTransfer),
        trpcteam.WithSwarmHandoffInputBuilder(defaultSwarmHandoffInput),
    }
}
```

### 6.3 Escalation 函数

```go
func buildEscalationFunc(clc *CriticLoopConfig) func(ev *trpcevent.Event) bool {
    if clc == nil || clc.ScoreThreshold <= 0 {
        return defaultEscalationFunc
    }
    threshold := clc.ScoreThreshold
    return func(ev *trpcevent.Event) bool {
        if ev == nil || ev.Response == nil {
            return false
        }
        for _, ch := range ev.Choices {
            content := strings.ToLower(ch.Message.Content)
            if strings.Contains(content, "approved") {
                return true
            }
            score := extractScore(content)
            if score > 0 && score >= threshold {
                return true
            }
        }
        return false
    }
}
```

`extractScore` 从 critic 响应中提取数值评分，支持以下格式：
- `score: 0.85`
- `评分: 85`
- `rating: 8.5/10`

### 6.4 Coordinator 配置构建

```go
func buildCoordinatorOptions(def Definition) []trpcteam.Option {
    var opts []trpcteam.Option
    if mt := def.MemberTool; mt != nil {
        mtc := trpcteam.MemberToolConfig{
            StreamInner:      mt.StreamInner,
            InnerTextMode:    mt.InnerTextMode,
            SkipSummarization: mt.SkipSummarization,
            HistoryScope:     mt.HistoryScope,
        }
        if mt.ToolSetName != "" {
            mtc.ToolSetName = mt.ToolSetName
        }
        opts = append(opts, trpcteam.WithMemberToolConfig(mtc))
    }
    return opts
}
```

---

## 七、SSE 事件模型

文件：`internal/biz/team_run_events.go`

```go
type TeamRunEvent struct {
    Type      string         `json:"type"`
    TeamID    string         `json:"team_id"`
    RunID     string         `json:"run_id"`
    SessionID string         `json:"session_id,omitempty"`
    Run       *TeamRun       `json:"run,omitempty"`
    Step      *TeamRunStep   `json:"step,omitempty"`
    Payload   map[string]any `json:"payload,omitempty"`
}
```

事件类型：
- `run_started`：Team 运行开始
- `step_finished`：子 Agent 步骤完成
- `run_finished`：Team 运行结束
- `intent_pass`：意图传递

---

## 八、待实现功能

### 8.1 P0 — RunTeamTest 端到端

当前 Service 层为桩实现，需要：
- 创建临时 Session
- 调用 Team Runtime 执行
- 返回 TeamRun 和回复内容

### 8.2 P1 — A2A call_agent 工具注入

在 `internal/agent/trpc_build.go` 中注入 `call_agent` 工具到 Agent 工具集，使 Agent 可在 Team 中调用远程 Agent。

### 8.3 P2 — member_* SSE 事件

在 `chat_native.go` 的 Team turn 中发射 `member_message_start` / `member_message_delta` / `member_message_done` 事件，使前端可展示子 Agent 实时流。

### 8.4 P3 — Team 运行结果结构化汇总

新增 API 返回各成员贡献度、工具调用统计等结构化汇总数据。
