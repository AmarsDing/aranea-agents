# Multi-Agent Team 编排模块 — 实现设计文档

> 对应需求：`11 multi-agent.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Team 编排：五种编排模式（sequential / parallel / coordinator / critic_loop / swarm）、成员 Agent 管理、Team Runner 执行、SSE 事件流、运行轨迹可观测。核心包 `internal/team`。

**当前实现状态**：
- ✅ Proto CRUD + DuplicateTeam + ListTeamRuns + ListTeamRunSteps 已实现
- ✅ `internal/team/trpc_build.go` 五种编排模式全部实现
- ✅ `internal/team/definition.go` Definition 解析与校验
- ✅ `internal/team/runner_team_trpc.go` Team 运行时
- ✅ `internal/biz/team_usecase.go` Usecase 完整实现
- ✅ `internal/data/team_repo.go` Repo 完整实现
- ✅ `internal/service/team.go` Service 完整实现
- ✅ `internal/biz/team_run_events.go` SSE Broker 实现
- ✅ 前端 Team 管理页组件化（TeamCard / TeamToolbar / TeamEditorDialog / TeamRunsDialog / teamUtils）
- ⏳ escalationFunc 增强：当前仅检查 "approved" 关键词，需支持 `CriticLoopConfig.ScoreThreshold` 结构化评分
- ⏳ sequential 上下文传递验证
- ⏳ parallel synthesizer 结果汇总验证
- ⏳ 实时 step event SSE 增强（step_started / 进度百分比 / 事件回放）
- ⏳ Team 模板后端库 / 自定义模板保存
- ⏳ 图工作流拖拽编辑 / 条件分支执行 / 后端 DAG 调度
- ⏳ A2A 跨框架协议握手 / 能力发现 / 消息持久化

---

## 二、Proto 层

### 2.1 完整 Proto 定义

文件：`api/kratos/team/v1/team.proto`

```protobuf
syntax = "proto3";

package kratos.team.v1;

import "google/api/annotations.proto";
import "google/api/field_behavior.proto";
import "google/protobuf/empty.proto";

option go_package = "aranea-agents/api/kratos/team/v1;v1";

message Team {
  string id = 1;
  string team_key = 2;
  string display_name = 3;
  string status = 4;
  bool is_default = 5;
  string definition_json = 6;
  string adk_app_name = 7;
  string created_at = 8;
  string updated_at = 9;
  string deleted_at = 10;
}

message TeamRun {
  string id = 1;
  string team_id = 2;
  string session_id = 3;
  string message_id = 4;
  string mode = 5;
  string status = 6;
  string input_preview = 7;
  string output_preview = 8;
  int32 token_in = 9;
  int32 token_out = 10;
  int64 cost_micro_usd = 11;
  int32 duration_ms = 12;
  string error_message = 13;
  string topology_json = 14;
  string started_at = 15;
  string finished_at = 16;
  string created_at = 17;
  string updated_at = 18;
}

message TeamRunStep {
  string id = 1;
  string run_id = 2;
  string team_id = 3;
  string agent_id = 4;
  string agent_key = 5;
  string agent_name = 6;
  string role = 7;
  int32 sort_order = 8;
  string status = 9;
  string input_preview = 10;
  string output_preview = 11;
  int32 token_in = 12;
  int32 token_out = 13;
  int64 cost_micro_usd = 14;
  int32 duration_ms = 15;
  string error_message = 16;
  string started_at = 17;
  string finished_at = 18;
  string created_at = 19;
}

message ListTeamsRequest {}

message ListTeamsResponse {
  repeated Team items = 1;
}

message CreateTeamRequest {
  string team_key = 1 [(google.api.field_behavior) = REQUIRED];
  string display_name = 2 [(google.api.field_behavior) = REQUIRED];
  string status = 3;
  string definition_json = 4;
  string adk_app_name = 5;
}

message GetTeamRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message UpdateTeamRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  Team team = 2;
}

message DeleteTeamRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message DuplicateTeamRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ListTeamRunsRequest {
  string team_id = 1;
  int32 limit = 2;
}

message ListTeamRunsResponse {
  repeated TeamRun items = 1;
}

message ListTeamRunStepsRequest {
  string run_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ListTeamRunStepsResponse {
  repeated TeamRunStep items = 1;
}

service TeamService {
  rpc ListTeams(ListTeamsRequest) returns (ListTeamsResponse) {
    option (google.api.http) = {get: "/v1/teams"};
  }
  rpc CreateTeam(CreateTeamRequest) returns (Team) {
    option (google.api.http) = {post: "/v1/teams" body: "*"};
  }
  rpc GetTeam(GetTeamRequest) returns (Team) {
    option (google.api.http) = {get: "/v1/teams/{id}"};
  }
  rpc UpdateTeam(UpdateTeamRequest) returns (Team) {
    option (google.api.http) = {patch: "/v1/teams/{id}" body: "team"};
  }
  rpc DeleteTeam(DeleteTeamRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {delete: "/v1/teams/{id}"};
  }
  rpc DuplicateTeam(DuplicateTeamRequest) returns (Team) {
    option (google.api.http) = {post: "/v1/teams/{id}/duplicate" body: "*"};
  }
  rpc ListTeamRuns(ListTeamRunsRequest) returns (ListTeamRunsResponse) {
    option (google.api.http) = {get: "/v1/team-runs"};
  }
  rpc ListTeamRunSteps(ListTeamRunStepsRequest) returns (ListTeamRunStepsResponse) {
    option (google.api.http) = {get: "/v1/team-runs/{run_id}/steps"};
  }
}
```

### 2.2 API 路由汇总

| RPC | HTTP | 用途 |
|-----|------|------|
| `ListTeams` | `GET /v1/teams` | 列出所有未删除 Team |
| `CreateTeam` | `POST /v1/teams` | 创建 Team |
| `GetTeam` | `GET /v1/teams/{id}` | 获取单个 Team |
| `UpdateTeam` | `PATCH /v1/teams/{id}` | 更新 Team |
| `DeleteTeam` | `DELETE /v1/teams/{id}` | 软删除 Team |
| `DuplicateTeam` | `POST /v1/teams/{id}/duplicate` | 复制 Team |
| `ListTeamRuns` | `GET /v1/team-runs` | 列出 Team 运行记录 |
| `ListTeamRunSteps` | `GET /v1/team-runs/{run_id}/steps` | 列出运行步骤 |

### 2.3 待新增 Proto（P0）

| RPC | HTTP | 用途 |
|-----|------|------|
| `RunTeamTest` | `POST /v1/teams/{id}/run-test` | 手动触发 Team 测试运行 |
| `GetTeamRun` | `GET /v1/team-runs/{id}` | 获取单条运行详情（含 steps） |
| `CancelTeamRun` | `POST /v1/team-runs/{id}/cancel` | 取消正在运行的 Team Run |

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
    Mode          string   `json:"mode"`            // "sequential" | "parallel" | "coordinator" | "critic_loop" | "swarm"
    Status        string   `json:"status"`           // "running" | "success" | "failed" | "timeout" | "cancelled"
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
    Role          string   `json:"role"`             // "coordinator" | "worker" | "synthesizer" | "critic" | "generator"
    SortOrder     int      `json:"sort_order"`
    Status        string   `json:"status"`           // "success" | "failed" | "skipped" | "timeout"
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
```

### 3.2 SSE 事件模型

文件：`internal/biz/team_run_events.go`

```go
type TeamRunEvent struct {
    Type      string         `json:"type"`        // "run_started" | "step_finished" | "run_finished" | "intent_pass"
    TeamID    string         `json:"team_id"`
    RunID     string         `json:"run_id"`
    SessionID string         `json:"session_id,omitempty"`
    Run       *TeamRun       `json:"run,omitempty"`
    Step      *TeamRunStep   `json:"step,omitempty"`
    Payload   map[string]any `json:"payload,omitempty"`
}

type TeamRunEventBroker struct {
    mu          sync.RWMutex
    subscribers map[chan TeamRunEvent]string   // channel → filterTeamID
}

func NewTeamRunEventBroker() *TeamRunEventBroker
func (b *TeamRunEventBroker) Subscribe(filterTeamID string) (chan TeamRunEvent, func())
func (b *TeamRunEventBroker) Publish(event TeamRunEvent)
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
    ListTeamRunSteps(ctx context.Context, runID string) ([]TeamRunStep, error)
    CreateTeamRun(ctx context.Context, r TeamRun) (TeamRun, error)
    UpdateTeamRun(ctx context.Context, r TeamRun) error
    CreateTeamRunStep(ctx context.Context, s TeamRunStep) (TeamRunStep, error)
}
```

### 3.4 Usecase

文件：`internal/biz/team_usecase.go`

```go
type TeamUsecase struct {
    repo TeamRepository
    runs *TeamRunEventBroker
}

func NewTeamUsecase(repo TeamRepository, runs *TeamRunEventBroker) *TeamUsecase

func (u *TeamUsecase) List(ctx context.Context) ([]Team, error)
func (u *TeamUsecase) Get(ctx context.Context, id string) (Team, error)
func (u *TeamUsecase) Create(ctx context.Context, in Team) (Team, error)
func (u *TeamUsecase) Update(ctx context.Context, id string, patch Team) (Team, error)
func (u *TeamUsecase) Delete(ctx context.Context, id string) error
func (u *TeamUsecase) Duplicate(ctx context.Context, id string) (Team, error)
func (u *TeamUsecase) ListRuns(ctx context.Context, teamID string, limit int) ([]TeamRun, error)
func (u *TeamUsecase) ListRunSteps(ctx context.Context, runID string) ([]TeamRunStep, error)
func (u *TeamUsecase) PublishTeamRunEvent(ev TeamRunEvent)
```

**Create 校验规则**：
- `team_key` 和 `display_name` 必填
- `definition_json` 为空时填充默认值 `{"version":1,"mode":"sequential","members":[],"max_concurrency":2,"timeout_seconds":600}`
- 调用 `validateTeamDefinition` 校验 JSON 结构

**Update 合并规则**：
- `TeamKey` / `DisplayName` / `Status` / `DefinitionJSON`：空值不覆盖
- `ADKAppName`：空值回退到 `TeamKey`
- 合并后再次调用 `validateTeamDefinition`

**Delete 规则**：
- 默认 Team（`IsDefault=true`）不允许删除，返回 `kerrors.Conflict`
- 非默认 Team 执行软删除

**Duplicate 规则**：
- 复制 `DefinitionJSON`，生成新 ID
- `TeamKey` 追加 `-copy-{suffix}`
- `DisplayName` 追加 ` Copy`
- `IsDefault` 置为 false

**validateTeamDefinition 校验**：
- `mode` 必须为 `sequential` / `parallel` / `coordinator` / `critic_loop` / `adaptive`
- 至少一个 enabled member
- `parallel` 模式必须有 `synthesizer` 成员或 `synthesizer_agent_id`
- `critic_loop` 模式必须有 `generator` 和 `critic` 成员
- 每个 member 的 `agent_id` 非空

---

## 四、Data 层

### 4.1 Ent Schema

#### team（`internal/data/ent/schema/team.go`）

```go
type Team struct {
    ent.Schema
}

func (Team) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "teams"},
    }
}

func (Team) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("team_key").Unique().MaxLen(512),
        field.String("display_name").MaxLen(1024),
        field.String("status").Default("active"),
        field.Bool("is_default").Default(false),
        field.Text("definition_json").Default(""),
        field.String("adk_app_name").Default(""),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
        field.String("deleted_at").Default(""),
    }
}
```

#### team_run（`internal/data/ent/schema/team_run.go`）

```go
type TeamRun struct {
    ent.Schema
}

func (TeamRun) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "team_runs"},
    }
}

func (TeamRun) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("team_id").MaxLen(256),
        field.String("session_id").Default(""),
        field.String("message_id").Default(""),
        field.String("mode").Default(""),
        field.String("status").Default("running"),
        field.Text("input_preview").Default(""),
        field.Text("output_preview").Default(""),
        field.Int("token_in").Default(0),
        field.Int("token_out").Default(0),
        field.Int64("cost_micro_usd").Default(0),
        field.Int("duration_ms").Default(0),
        field.Text("error_message").Default(""),
        field.Text("topology_json").Default("{}"),
        field.String("started_at").Default(""),
        field.String("finished_at").Default(""),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
    }
}
```

#### team_run_step（`internal/data/ent/schema/team_run_step.go`）

```go
type TeamRunStep struct {
    ent.Schema
}

func (TeamRunStep) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "team_run_steps"},
    }
}

func (TeamRunStep) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("run_id").MaxLen(256),
        field.String("team_id").MaxLen(256),
        field.String("agent_id").Default(""),
        field.String("agent_key").Default(""),
        field.String("agent_name").Default(""),
        field.String("role").Default(""),
        field.Int("sort_order").Default(0),
        field.String("status").Default("success"),
        field.Text("input_preview").Default(""),
        field.Text("output_preview").Default(""),
        field.Int("token_in").Default(0),
        field.Int("token_out").Default(0),
        field.Int64("cost_micro_usd").Default(0),
        field.Int("duration_ms").Default(0),
        field.Text("error_message").Default(""),
        field.String("started_at").Default(""),
        field.String("finished_at").Default(""),
        field.String("created_at").Default(""),
    }
}
```

### 4.2 Repo 实现

文件：`internal/data/team_repo.go`

```go
type teamRepo struct {
    data *Data
}

func NewTeamRepo(d *Data) biz.TeamRepository
```

**Ent 转换函数**：

```go
func entTeamToBiz(e *ent.Team) biz.Team
func entTeamRunToBiz(e *ent.TeamRun) biz.TeamRun
func entTeamRunStepToBiz(e *ent.TeamRunStep) biz.TeamRunStep
```

**关键查询**：

| 方法 | Ent 查询 |
|------|----------|
| `ListTeams` | `Team.Query().Where(DeletedAtEQ("")).Order(ByIsDefault(Desc), ByCreatedAt(Desc))` |
| `GetTeamByID` | `Team.Query().Where(IDEQ(id), DeletedAtEQ("")).Only(ctx)` |
| `CreateTeam` | `Team.Create().SetID/SetTeamKey/SetDisplayName/.../Save(ctx)` → `GetTeamByID` 回读 |
| `UpdateTeam` | `Team.UpdateOneID(id).SetTeamKey/SetDisplayName/.../Save(ctx)` → `GetTeamByID` 回读 |
| `DeleteTeam` | `Team.UpdateOneID(id).SetDeletedAt(now).SetStatus("deleted").SetUpdatedAt(now).Save(ctx)` |
| `ListTeamRuns` | `TeamRun.Query().Order(ByCreatedAt(Desc)).Where(TeamIDEQ(teamID)).Limit(limit)` |
| `ListTeamRunSteps` | `TeamRunStep.Query().Where(RunIDEQ(runID)).Order(BySortOrder(Asc), ByCreatedAt(Asc))` |
| `CreateTeamRun` | `TeamRun.Create().SetID/SetTeamID/SetSessionID/.../Save(ctx)` → 回读 |
| `UpdateTeamRun` | `TeamRun.UpdateOneID(id).SetStatus/SetOutputPreview/.../Save(ctx)` |
| `CreateTeamRunStep` | `TeamRunStep.Create().SetID/SetRunID/SetTeamID/.../Save(ctx)` → 回读 |

---

## 五、运行时层

### 5.1 Team Definition

文件：`internal/team/definition.go`

```go
type Definition struct {
    Version            int               `json:"version"`
    Mode               string            `json:"mode"`                          // "sequential" | "parallel" | "coordinator" | "critic_loop" | "adaptive"
    SynthesizerAgentID string            `json:"synthesizer_agent_id"`
    Members            []MemberDef       `json:"members"`
    MaxConcurrency     int               `json:"max_concurrency"`
    TimeoutSeconds     int               `json:"timeout_seconds"`
    LoopMaxIterations  int               `json:"loop_max_iterations,omitempty"` // coordinator/adaptive 外圈迭代
    CriticLoop         *CriticLoopConfig `json:"critic_loop,omitempty"`
    IntentAnchorAgentID string           `json:"intent_anchor_agent_id,omitempty"`
}

type CriticLoopConfig struct {
    MaxIterations  int     `json:"max_iterations"`
    ScoreThreshold float64 `json:"score_threshold"`
}

type MemberDef struct {
    AgentID   string `json:"agent_id"`
    Role      string `json:"role"`       // "coordinator" | "worker" | "synthesizer" | "critic" | "generator"
    Enabled   *bool  `json:"enabled"`
    SortOrder int    `json:"sort_order"`
    Name      string `json:"name"`
}
```

**核心函数**：

```go
func ParseDefinition(raw string) (Definition, error)
func EnabledMembers(d Definition) []MemberDef       // 过滤 enabled 且 agent_id 非空，按 sort_order 排序
func SynthesizerAgentID(d Definition) string         // 从 definition 或 member role=synthesizer 解析
func ParallelWorkers(d Definition) []MemberDef       // 排除 synthesizer 的 worker 列表
func TurnDeadlineDuration(d Definition) time.Duration // 120s~7200s 范围限制
```

### 5.2 Team 构建

文件：`internal/team/trpc_build.go`

```go
type TRPCTeamBuilderDeps struct {
    BuilderDeps chatagent.TRPCBuilderDeps
}

func BuildTRPCTeam(ctx context.Context, def Definition, deps TRPCTeamBuilderDeps, catalogAgent func(ctx context.Context, id string) (biz.Agent, error)) (trpcagent.Agent, error)
```

**编排模式映射**：

| 编排模式 | trpc 框架组件 | 构建逻辑 |
|---------|-------------|----------|
| `sequential` | `chainagent.New` | `chainagent.New("team-sequential", chainagent.WithSubAgents(memberAgents))` |
| `parallel` | `parallelagent.New` | `parallelagent.New("team-parallel", parallelagent.WithSubAgents(memberAgents))` |
| `critic_loop` | `cycleagent.New` | `cycleagent.New("team-critic-loop", WithSubAgents, WithMaxIterations, WithEscalationFunc(defaultEscalationFunc))` |
| `swarm` | `trpcteam.NewSwarm` | `trpcteam.NewSwarm("team", entryName, memberAgents)` |
| 默认(coordinator) | `trpcteam.New` | `trpcteam.New(coordinator, rest)` — 第一个成员为 coordinator，其余为 workers |

**defaultEscalationFunc（当前实现）**：

```go
func defaultEscalationFunc(ev *trpcevent.Event) bool {
    if ev == nil || ev.Response == nil {
        return false
    }
    for _, ch := range ev.Choices {
        if strings.Contains(strings.ToLower(ch.Message.Content), "approved") {
            return true
        }
    }
    return false
}
```

**待实现 P0 — escalationFunc 增强**：

```go
func scoreBasedEscalationFunc(threshold float64) cycleagent.EscalationFunc {
    return func(ev *trpcevent.Event) bool {
        if ev == nil || ev.Response == nil {
            return false
        }
        for _, ch := range ev.Choices {
            content := strings.TrimSpace(ch.Message.Content)
            if score, err := extractCriticScore(content); err == nil && score >= threshold {
                return true
            }
        }
        return false
    }
}

func extractCriticScore(content string) (float64, error) {
    // 尝试从 JSON 结构中提取 score 字段
    // 尝试从文本中匹配 "score: 0.85" / "评分: 85" 等模式
    // 兜底：检查 "approved" 关键词
}
```

### 5.3 Team Runner

文件：`internal/team/runner_team_trpc.go`

```go
func (r *Runner) runTeamTRPC(ctx context.Context, sess biz.Session, req *chatv1.SendChatMessageRequest, teamRow biz.Team, def Definition, mode string, stream agent.StreamEmitter) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error)
```

**执行流程**：

1. **参数提取**：从 `req` 提取 `content`、`dialogMode`、`provOpt`、`modOpt`、`attN`
2. **创建 TeamRun**：生成 `run.ID`，设置 `status=running`，持久化到 `team_runs` 表
3. **发布 SSE 事件**：`TeamRunEvent{Type: "run_started"}`
4. **确定锚点成员**：`IntentAnchorAgentID` 或首位 enabled member
5. **构建 BuilderDeps**：组装 `TRPCBuilderDeps`（Catalog / AgentUC / Agents / RT / SkillUC / Sys / Provider / Model / DialogMode）
6. **构建 Team Agent**：`BuildTRPCTeam(ctx, def, teamDeps, r.catalogAgent)`
7. **构建 Runner**：`agent.NewTRPCRunner(root, runnerDeps)` — 含 MemoryService（SQLite 适配器）
8. **Intent 预处理**：`intent.Run(ctx, ...)` → 合并到 `UserOptionsJSON`
9. **追加用户消息**：`Sessions.AppendChatMessage(ctx, sess.ID, userMsg, false)`
10. **执行超时控制**：`TurnDeadlineDuration(def)` 创建 `context.WithTimeout`
11. **运行 Agent**：`agent.RunTRPCUserTurn(runCtx, runner, uid, sess.ID, sendText)`
12. **事件流处理**：遍历 events channel，收集 `reply` / `reasoning` / `promptTok` / `completionTok`，流式发射 delta
13. **构建助手消息**：`assistantMsg` 含 `ContentMarkdown` / `ModelName` / `TokenIn` / `TokenOut` / `OptionsJSON`
14. **持久化步骤**：为每个 member 调用 `persistStep`
15. **更新 TeamRun**：`status=success` / `duration_ms` / `output_preview` / `token_in` / `token_out`
16. **上下文压缩**：`Compress.AfterNativeTurn`
17. **发布 SSE 事件**：`TeamRunEvent{Type: "run_finished"}`
18. **SSE 提示**：`biz.HintTeamRunSSE` 推送最新 run 列表

**错误处理**：

- `finishRunErr`：设置 `run.Status="failed"` / `ErrorMessage` / `DurationMS` / `FinishedAt`，更新数据库
- 超时：`runCtx.Err() == context.DeadlineExceeded` → `finishRunErr`
- 取消：`runCtx.Err() == context.Canceled` → `finishRunErr`
- 无输出：`displayMarkdown == ""` → 返回 `InternalServer` 错误

---

## 六、Service 层

文件：`internal/service/team.go`

```go
type TeamService struct {
    v1.UnimplementedTeamServiceServer
    uc *biz.TeamUsecase
}

func NewTeamService(uc *biz.TeamUsecase) *TeamService
```

**类型转换函数**：

```go
func toProtoTeam(t biz.Team) *v1.Team
func toProtoTeamRun(r biz.TeamRun) *v1.TeamRun
func toProtoTeamRunStep(s biz.TeamRunStep) *v1.TeamRunStep
func teamFromProto(pb *v1.Team) biz.Team
func mapTeamErr(err error) error   // sql.ErrNoRows → kerrors.NotFound
```

**RPC 实现**：

| RPC | 实现 |
|-----|------|
| `ListTeams` | `uc.List(ctx)` → 遍历 `toProtoTeam` |
| `CreateTeam` | `uc.Create(ctx, biz.Team{...})` → `toProtoTeam` |
| `GetTeam` | `uc.Get(ctx, req.GetId())` → `mapTeamErr` → `toProtoTeam` |
| `UpdateTeam` | `teamFromProto(req.GetTeam())` → `uc.Update(ctx, req.GetId(), patch)` → `mapTeamErr` → `toProtoTeam` |
| `DeleteTeam` | `uc.Delete(ctx, req.GetId())` → `mapTeamErr` → `emptypb.Empty` |
| `DuplicateTeam` | `uc.Duplicate(ctx, req.GetId())` → `mapTeamErr` → `toProtoTeam` |
| `ListTeamRuns` | `uc.ListRuns(ctx, req.GetTeamId(), int(req.GetLimit()))` → 遍历 `toProtoTeamRun` |
| `ListTeamRunSteps` | `uc.ListRunSteps(ctx, req.GetRunId())` → 遍历 `toProtoTeamRunStep` |

---

## 七、Wire 注入

```
data.ProviderSet  → NewTeamRepo
biz.ProviderSet   → NewTeamUsecase, NewTeamRunEventBroker
service.ProviderSet → NewTeamService
team.ProviderSet  → NewRunner
```

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/features/teams/
├── api.ts                              # API 调用 + wire 类型转换
└── types.ts                            # TypeScript 类型定义

web/src/components/teams/
├── TeamCard.vue                        # Team 卡片（列表项）
├── TeamToolbar.vue                     # 搜索/筛选工具栏
├── TeamEditorDialog.vue                # Team 编辑弹窗（新增/编辑）
├── TeamRunsDialog.vue                  # 运行轨迹抽屉
└── teamUtils.ts                        # 纯函数工具（模板/解析/格式化）
```

### 8.2 TypeScript 类型

文件：`web/src/features/teams/types.ts`

```typescript
export type Team = {
  id: string;
  team_key: string;
  display_name: string;
  status: string;
  is_default: boolean;
  definition_json: string;
  adk_app_name: string;
  created_at: string;
  updated_at: string;
  deleted_at: string;
};

export type TeamDefinitionMember = {
  agent_id: string;
  role: "coordinator" | "worker" | "synthesizer" | "critic" | "generator" | string;
  name: string;
  enabled: boolean;
  sort_order: number;
};

export type TeamDefinitionGraphNode = {
  id: string;
  type: "start" | "agent" | "join" | "end" | string;
  label: string;
  agent_id?: string;
  role?: string;
  x?: number;
  y?: number;
};

export type TeamDefinitionGraphEdge = {
  id: string;
  source: string;
  target: string;
  label?: string;
  condition?: string;
};

export type TeamDefinition = {
  version: number;
  description?: string;
  mode: "sequential" | "parallel" | "coordinator" | "critic_loop" | "adaptive" | string;
  max_concurrency?: number;
  timeout_seconds?: number;
  loop_max_iterations?: number;
  intent_anchor_agent_id?: string;
  members: TeamDefinitionMember[];
  a2a?: {
    enabled?: boolean;
    envelope_version?: string;
    message_format?: "markdown_json" | "plain" | string;
    include_trace?: boolean;
    max_payload_chars?: number;
  };
  graph?: {
    version?: number;
    layout?: "linear" | "parallel" | "loop" | "coordinator" | string;
    nodes: TeamDefinitionGraphNode[];
    edges: TeamDefinitionGraphEdge[];
  };
  synthesizer_agent_id?: string;
  critic_loop?: {
    max_iterations?: number;
    score_threshold?: number;
  };
};

export type TeamRun = {
  id: string;
  team_id: string;
  session_id: string;
  message_id: string;
  mode: string;
  status: string;
  input_preview: string;
  output_preview: string;
  token_in: number;
  token_out: number;
  cost_micro_usd: number;
  duration_ms: number;
  error_message: string;
  topology_json: string;
  started_at: string;
  finished_at: string;
  created_at: string;
  updated_at: string;
};

export type TeamRunStep = {
  id: string;
  run_id: string;
  team_id: string;
  agent_id: string;
  agent_key: string;
  agent_name: string;
  role: string;
  sort_order: number;
  status: string;
  input_preview: string;
  output_preview: string;
  token_in: number;
  token_out: number;
  cost_micro_usd: number;
  duration_ms: number;
  error_message: string;
  started_at: string;
  finished_at: string;
  created_at: string;
};

export type TeamRunEvent = {
  type: string;
  team_id: string;
  run_id: string;
  session_id?: string;
  run?: TeamRun;
  step?: TeamRunStep;
  payload?: Record<string, unknown>;
};
```

### 8.3 API 层

文件：`web/src/features/teams/api.ts`

| 函数 | 说明 |
|------|------|
| `listTeams()` | `GET /v1/teams` → `Team[]` |
| `createTeam(payload)` | `POST /v1/teams` → `Team` |
| `updateTeam(id, payload)` | `PATCH /v1/teams/{id}` → `Team` |
| `duplicateTeam(id)` | `POST /v1/teams/{id}/duplicate` → `Team` |
| `deleteTeam(id)` | `DELETE /v1/teams/{id}` → `void` |
| `listTeamRuns(teamID?, limit?)` | `GET /v1/team-runs` → `TeamRun[]` |
| `listTeamRunSteps(runID)` | `GET /v1/team-runs/{run_id}/steps` → `TeamRunStep[]` |
| `subscribeTeamRunEvents(teamID, onEvent, onError?)` | SSE 订阅 `/sse/team-run-events?team_id=` |

**wire 类型转换**：`wireTeam` / `wireRun` / `wireStep` / `patchToWire` — Kratos 生成类型（camelCase）↔ 业务类型（snake_case）

### 8.4 组件设计

#### TeamCard.vue

**Props**：`team: Team` / `agents: Agent[]` / `isDark: boolean`

**Emits**：`copyKey` / `openRuns` / `duplicate` / `edit` / `remove`

**布局**：
- 头部：`display_name` + mode chip + is_default chip + status badge
- 拓扑条：`topologyNodesFromDefinition(definition)` 渲染图标+标签
- 成员列表：每行 `avatar` + `name` + `role` + `agent_name` + enabled badge
- 底部：成员数 + 更新时间 + 操作按钮（Chat测试 / 运行轨迹 / 复制 / 编辑 / 删除）

**样式**：玻璃态卡片（`backdrop-filter: blur(16px)`），暗色模式 `.is-dark` 覆盖

#### TeamToolbar.vue

**Props**：`search` / `modeFilter` / `statusFilter` / `loading` / `isDark`

**Emits**：`update:search` / `update:modeFilter` / `update:statusFilter` / `refresh`

**控件**：
- `QInput` 搜索（debounce 250ms）
- `QSelect` 编排模式筛选（modeOptions）
- `QSelect` 状态筛选（statusOptions）
- `QBtn` 刷新

#### TeamEditorDialog.vue

**Props**：`modelValue` / `selectedTemplateKey` / `editingId` / `form` / `definition` / `definitionJSON` / `agentOptions` / `saving` / `canSave` / `isDark`

**Emits**：`update:modelValue` / `update:selectedTemplateKey` / `addMember` / `removeMember` / `applyTemplate` / `save`

**布局**：
1. **模板选择区**：`QSelect` 选择内置模板（sequential / parallel_experts / critic_loop / coordinator）
2. **基础信息**：
   - `QInput` Team 名称（必填）
   - `QInput` Team Key（必填，小写字母/数字/连字符）
   - `QSelect` 编排模式（5 种）
   - `QSelect` 状态（draft / active / archived）
   - 条件字段：
     - `parallel`：并行批大小 `max_concurrency`
     - `coordinator`/`adaptive`：外圈循环迭代 `loop_max_iterations`
     - `critic_loop`：评审迭代次数 `critic_loop.max_iterations`
   - `QInput` Team 说明
3. **高级配置**：
   - `QInput` 单次运行超时（0~7200s）
   - `QSelect` Intent 锚定成员
4. **A2A 协议**（`QExpansionItem`）：
   - `QToggle` 启用 A2A 信封
   - `QInput` Envelope Version
   - `QSelect` 消息格式（markdown_json / plain）
   - `QInput` 最大载荷字符
   - `QToggle` 包含 trace metadata
5. **成员 Agent 列表**：
   - 每行：`QSelect` Agent / `QSelect` 角色 / `QInput` 名称 / `QInput` 顺序 / `QToggle` 启用 / `QBtn` 删除
   - 底部：添加成员按钮
6. **图工作流 / JSON**（`QExpansionItem`）：
   - 拓扑预览节点
   - 图边和节点可视化
   - JSON 预览

**模板定义**（`teamUtils.ts`）：

| 模板 Key | 模式 | 成员角色 | 说明 |
|----------|------|---------|------|
| `sequential` | sequential | 2×worker | 顺序接力 |
| `parallel_experts` | parallel | 3×worker + synthesizer | 并行专家组 |
| `critic_loop` | critic_loop | generator + critic | 生成评审 |
| `coordinator` | coordinator | coordinator + 2×worker | 主控分派 |

#### TeamRunsDialog.vue

**Props**：`modelValue` / `selectedTeam` / `runs` / `stepsByRun` / `stepsLoading` / `agents` / `loading` / `error` / `liveConnected` / `isDark`

**Emits**：`update:modelValue` / `refresh` / `showSteps`

**布局**：
- 头部：运行轨迹标题 + Team 名称 + SSE 连接状态 badge
- 刷新按钮
- 运行列表（`QExpansionItem`）：
  - 头部：status avatar + mode + duration_ms + created_at + token_in/out + cost
  - 展开内容：
    - 输入预览
    - 输出预览 / 错误信息
    - Step 列表：agent_name + role + duration_ms + token_in/out + cost + output_preview / error_message

**SSE 实时连接**：`subscribeTeamRunEvents(teamID, onEvent)` 监听 `run_started` / `step_finished` / `run_finished` / `intent_pass`

### 8.5 工具函数

文件：`web/src/components/teams/teamUtils.ts`

| 函数 | 说明 |
|------|------|
| `defaultDefinition()` | 生成默认 TeamDefinition |
| `definitionFromTemplate(template, agents)` | 按模板生成 Definition |
| `parseDefinition(team)` | 从 `definition_json` 解析 TeamDefinition |
| `topologyNodesFromDefinition(def)` | 根据模式返回拓扑节点数组 |
| `buildGraphFromDefinition(def)` | 构建 graph.nodes / graph.edges |
| `withGraph(definition)` | 确保 definition 有 graph |
| `agentName(agents, id)` | 从 Agent 列表查找 display_name |
| `memberIcon(role)` | 角色图标映射 |
| `formatDate(value)` | 日期格式化 |
| `defaultA2AConfig()` | 默认 A2A 配置 |

**常量**：
- `modeOptions`：5 种编排模式选项
- `statusOptions`：draft / active / archived
- `roleOptions`：worker / coordinator / synthesizer / generator / critic
- `teamTemplateOptions`：4 种内置模板

---

## 九、待实现功能（P0）

### 9.1 escalationFunc 增强

**当前**：`defaultEscalationFunc` 仅检查 `"approved"` 关键词

**目标**：支持 `CriticLoopConfig.ScoreThreshold` 结构化评分判断

**实现方案**：

1. 在 `internal/team/trpc_build.go` 中新增 `scoreBasedEscalationFunc(threshold float64)`
2. `BuildTRPCTeam` 中 `critic_loop` 分支改为：
   ```go
   if def.CriticLoop != nil && def.CriticLoop.ScoreThreshold > 0 {
       escalationFunc = scoreBasedEscalationFunc(def.CriticLoop.ScoreThreshold)
   } else {
       escalationFunc = defaultEscalationFunc
   }
   ```
3. `extractCriticScore` 支持多种格式：
   - JSON `{"score": 0.85}`
   - 文本 `Score: 85` / `评分: 85`
   - 百分比 `85%`
   - 兜底 `"approved"` 关键词

### 9.2 sequential 上下文传递验证

**当前**：`chainagent` 内部自动传递事件流

**需验证**：前一个 Agent 的输出是否正确作为下一个 Agent 的输入

**验证方案**：
1. 创建 2-Agent sequential Team
2. 第一个 Agent 输出特定标记
3. 第二个 Agent 的 prompt 要求引用第一个 Agent 的输出
4. 确认第二个 Agent 能正确获取前序输出

### 9.3 parallel synthesizer 验证

**当前**：`parallelagent` 并行执行后事件流合并

**需验证**：synthesizer Agent 是否能正确汇总并行结果

**验证方案**：
1. 创建 3-worker + 1-synthesizer parallel Team
2. 每个 worker 输出不同标记
3. synthesizer 的 prompt 要求汇总所有 worker 输出
4. 确认最终输出包含所有 worker 的结果

### 9.4 实时 step event SSE 增强

**当前**：仅 `run_started` / `step_finished` / `run_finished` / `intent_pass`

**目标**：增加 `step_started` / 进度百分比 / 事件回放

**实现方案**：

1. 在 `runner_team_trpc.go` 的事件循环中，每个 member Agent 开始执行时发布 `step_started`
2. `TeamRunEvent` 新增 `Progress` 字段（`float64`，0~1）
3. 前端 `TeamRunsDialog` 展示实时进度条
4. SSE 断线续传：前端重连时请求最近 N 条事件

### 9.5 RunTeamTest / GetTeamRun / CancelTeamRun

**Proto 新增**：

```protobuf
message RunTeamTestRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string content = 2;
}

message RunTeamTestResponse {
  TeamRun run = 1;
}

message GetTeamRunRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message GetTeamRunResponse {
  TeamRun run = 1;
  repeated TeamRunStep steps = 2;
}

message CancelTeamRunRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}
```

**Biz 层新增**：

```go
func (u *TeamUsecase) GetRun(ctx context.Context, id string) (TeamRun, []TeamRunStep, error)
func (u *TeamUsecase) CancelRun(ctx context.Context, id string) error
```

**Service 层新增**：

```go
func (s *TeamService) RunTeamTest(ctx, req) (*RunTeamTestResponse, error)
func (s *TeamService) GetTeamRun(ctx, req) (*GetTeamRunResponse, error)
func (s *TeamService) CancelTeamRun(ctx, req) (*emptypb.Empty, error)
```

---

## 十、待实现功能（P1）

### 10.1 Team 模板后端库

**当前**：前端 `teamUtils.ts` 内置 4 种模板

**目标**：支持后端模板库、自定义模板保存、模板权限

**实现方案**：

1. 新增 `team_templates` 表（Ent Schema）
2. 新增 `TeamTemplateUsecase` / `TeamTemplateRepository`
3. Proto 新增 `ListTeamTemplates` / `CreateTeamTemplate` / `DeleteTeamTemplate`
4. 前端 `TeamEditorDialog` 从后端加载模板列表

### 10.2 图工作流拖拽编辑

**当前**：`definition_json` 支持 `graph.nodes/edges`，前端有基础预览

**目标**：拖拽编辑、条件分支执行、后端按 graph DAG 调度

**实现方案**：

1. 前端引入图编辑库（如 Vue Flow / dagre）
2. `TeamEditorDialog` 新增图编辑 Tab
3. 后端 `BuildTRPCTeam` 新增 `graph` 模式分支
4. 使用 `trpc-agent-go` 的 `graph` 包构建 DAG 工作流

### 10.3 A2A 跨框架协议

**当前**：内部信封基础版（`a2a` 配置在 `definition_json`）

**目标**：跨进程 Agent 地址、外部 A2A 协议握手、能力发现、消息持久化

**实现方案**：

1. 新增 `a2a_agent_card` 表存储 Agent Card
2. 实现 A2A Server 端点（`/.well-known/agent.json`）
3. 实现 A2A Client 发现和握手
4. 消息持久化到 `a2a_messages` 表
