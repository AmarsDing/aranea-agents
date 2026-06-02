# M60: Spirit Parallel Orchestrator — 实现设计

> 对应需求：[60-spirit-parallel-orchestrator.md](./60-spirit-parallel-orchestrator.md)
> 前置需求：[59-chat-spirit-mode.md](./59-chat-spirit-mode.md) · [59-chat-spirit-mode.design.md](./59-chat-spirit-mode.design.md)
> 遵循：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · [AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)
> **实现差距与迭代计划**以 [60-spirit-parallel-orchestrator-development.md](./60-spirit-parallel-orchestrator-development.md) 为准

---

## 一、模块概述

### 1.1 设计定位

Spirit Parallel Orchestrator (SPO) 在 M59 精灵模式基础上，实现多团队并行编排。核心架构为三层分离（对齐 APWA 论文 arXiv:2605.15132）：

- **Spirit = Manager**：高层规划，决定"做什么"和"谁来做"
- **Team = Worker**：任务委派，管理成员执行
- **Agent = Executor**：具体执行，独立上下文

设计融合三篇论文核心思想：
- **AdaptOrch** (arXiv:2602.16873)：拓扑路由算法，根据任务 DAG 自动选择最优编排拓扑
- **APWA** (arXiv:2605.15132)：Manager-Worker-Executor 三层分离
- **Maestro** (arXiv:2511.06134)：探索-合成分离，并行 Team 做发散探索，Spirit 做收敛合成

### 1.2 分层与依赖

```
api/kratos/team/v1/team.proto          ← Team 扩展字段（parallel_config 等）
api/kratos/session/v1/session.proto     ← Session 扩展字段（dag_snapshot 等）
        ↓
internal/service/
  spirit_team.go                        ← AssembleTeam 改造（多团队 + 并行度检查）
  spirit_synthesis.go                   ← Synthesis Engine（P2）
  chat_orchestrator_turn.go             ← 精灵 CustomTools 扩展
        ↓
internal/biz/
  spirit_team_usecase.go                ← ListActiveTeams / ParallelConfig
  spirit_task_dag.go                    ← Task DAG 模型 + 拓扑路由（P2）
  spirit_synthesis.go                   ← Synthesis Engine 逻辑（P2）
  spirit_parallel_config.go             ← 并行度配置
  evolution.go                          ← DQ Score 驱动编排缓存（P2）
        ↓
internal/tools/
  spirit_tools.go                       ← 精灵工具扩展（check_team_progress / cancel_team / synthesize_results）
        ↓
internal/event/contract/
  envelope.go                           ← 新增 EnvelopeType（spirit_team_progress 等）
        ↓
web/src/
  features/spirit/                      ← 精灵域扩展（并行团队 API + 类型）
  stores/spirit/                        ← useSpiritTeamStore 扩展
  components/spirit/                    ← 并行团队面板 + 合成卡片
```

**红线**：`internal/biz` 不 import `pkg/trpc-agent-go`；精灵构建仅在 `internal/service`；Team 编译仅在 `internal/team`。

### 1.3 影响域

| 包 | 变更类型 | 说明 |
|----|----------|------|
| `internal/biz` | 扩展 | ListActiveTeams / ParallelConfig / TaskDAG / SynthesisEngine |
| `internal/service` | 扩展 | spirit_team.go 改造 + spirit_synthesis.go 新增 |
| `internal/tools` | 扩展 | 精灵工具新增 3 个 |
| `internal/event` | 扩展 | 3 个新 EnvelopeType |
| `internal/data` | 扩展 | Team 查询扩展 + DAG 存储 |
| `web/src/features/spirit` | 扩展 | 并行团队 API + 类型 |
| `web/src/stores/spirit` | 扩展 | 并行团队状态管理 |
| `web/src/components/spirit` | 扩展 | 并行进度卡片 + 合成卡片 |

**不改动**：`internal/server` 直连 runtime；`internal/team` 编译/运行流程不变；`internal/agent` 构建流程不变。

---

## 二、Phase 1 设计：基础并行

### 2.1 移除单活跃团队限制

**当前**：[spirit_tools.go](../../internal/tools/spirit_tools.go) 中 `NewAssembleTeamTool` 调用 `GetActiveTeam()` 短路返回。

**改造**：

```go
func NewAssembleTeamTool(assembler SpiritTeamAssemblerPort) trpctool.Tool {
    return function.NewFunctionTool[AssembleTeamInput, AssembleTeamOutput](
        "assemble_team",
        "Assemble a new team for a specific subtask. Multiple teams can run in parallel under the same spirit session.",
        func(ctx context.Context, input AssembleTeamInput) (AssembleTeamOutput, error) {
            spiritSessionID := spiritSessionIDFromCtx(ctx)

            activeTeams, err := assembler.ListActiveTeams(ctx, spiritSessionID)
            if err != nil {
                return AssembleTeamOutput{}, err
            }
            maxParallel := assembler.GetMaxParallelTeams(ctx, spiritSessionID)
            if len(activeTeams) >= maxParallel {
                return AssembleTeamOutput{}, kerrors.BadRequest(
                    "SPIRIT",
                    fmt.Sprintf("max parallel teams (%d) reached, wait for existing teams to complete", maxParallel),
                )
            }

            result, err := assembler.AssembleTeam(ctx, SpiritTeamParams{
                SpiritSessionID: spiritSessionID,
                AgentIDs:       input.AgentKeys,
                Mode:           input.Mode,
                TaskPrompt:     input.TaskPrompt,
            })
            if err != nil {
                return AssembleTeamOutput{}, err
            }
            return AssembleTeamOutput{
                TeamID:   result.Team.ID,
                TeamName: result.Team.DisplayName,
            }, nil
        },
    )
}
```

### 2.2 TeamKey UUID 后缀

**当前**：`"spirit_" + spiritSessionID`，同一精灵 Session 第二次创建冲突。

**改造**：

```go
func (u *SpiritTeamUsecase) AssembleTeam(ctx context.Context, params SpiritTeamParams) (*SpiritTeamResult, error) {
    teamKey := fmt.Sprintf("spirit_%s_%s", params.SpiritSessionID, uuid.New().String()[:8])
    // ...
}
```

### 2.3 ParallelConfig 配置

```go
type ParallelConfig struct {
    MaxConcurrentTeams int           `json:"max_concurrent_teams"`
    MaxTeamConcurrency int           `json:"max_team_concurrency"`
    TeamTimeout        time.Duration `json:"team_timeout"`
    AutoArchiveAfter   time.Duration `json:"auto_archive_after"`
    MaxSessionDepth    int           `json:"max_session_depth"`
}

func DefaultParallelConfig() ParallelConfig {
    return ParallelConfig{
        MaxConcurrentTeams: 3,
        MaxTeamConcurrency: 2,
        TeamTimeout:        10 * time.Minute,
        AutoArchiveAfter:   1 * time.Hour,
        MaxSessionDepth:    2,
    }
}
```

存储位置：`AgentRuntimeSettings.ExtraJSON` 中 `parallel_config` 键，精灵 Agent 种子数据中注入默认值。

### 2.4 SpiritTeamAssemblerPort 接口扩展

```go
type SpiritTeamAssemblerPort interface {
    AssembleTeam(ctx context.Context, params SpiritTeamParams) (*SpiritTeamResult, error)
    GetActiveTeam(ctx context.Context, spiritSessionID string) (*biz.Team, error)
    ListActiveTeams(ctx context.Context, spiritSessionID string) ([]biz.Team, error)
    GetMaxParallelTeams(ctx context.Context, spiritSessionID string) int
    CancelTeam(ctx context.Context, teamID string) error
    CheckTeamProgress(ctx context.Context, spiritSessionID string) ([]TeamProgress, error)
}

type TeamProgress struct {
    TeamID      string  `json:"team_id"`
    TeamName    string  `json:"team_name"`
    Status      string  `json:"status"`
    ProgressPct float64 `json:"progress_pct"`
    CurrentStep string  `json:"current_step"`
    DurationMs  int64   `json:"duration_ms"`
}
```

### 2.5 新增精灵工具

**check_team_progress**：

```go
func NewCheckTeamProgressTool(assembler SpiritTeamAssemblerPort) trpctool.Tool {
    return function.NewFunctionTool[CheckTeamProgressInput, CheckTeamProgressOutput](
        "check_team_progress",
        "Check the progress of all active teams under the current spirit session.",
        func(ctx context.Context, _ CheckTeamProgressInput) (CheckTeamProgressOutput, error) {
            spiritSessionID := spiritSessionIDFromCtx(ctx)
            progress, err := assembler.CheckTeamProgress(ctx, spiritSessionID)
            if err != nil {
                return CheckTeamProgressOutput{}, err
            }
            return CheckTeamProgressOutput{Teams: progress}, nil
        },
    )
}
```

**cancel_team**：

```go
func NewCancelTeamTool(assembler SpiritTeamAssemblerPort) trpctool.Tool {
    return function.NewFunctionTool[CancelTeamInput, CancelTeamOutput](
        "cancel_team",
        "Cancel a running team by its ID. Releases the parallel team quota.",
        func(ctx context.Context, input CancelTeamInput) (CancelTeamOutput, error) {
            err := assembler.CancelTeam(ctx, input.TeamID)
            if err != nil {
                return CancelTeamOutput{}, err
            }
            return CancelTeamOutput{TeamID: input.TeamID, Status: "cancelled"}, nil
        },
    )
}
```

### 2.6 事件驱动：精灵 Observer

精灵 Session 注册为 Event Observer，订阅子团队完成/失败事件：

```go
func (o *ChatOrchestrator) onSpiritTeamEvent(ctx context.Context, env contract.Envelope) {
    if env.Type != contract.EnvelopeTypeSpiritTeamCompleted &&
       env.Type != contract.EnvelopeTypeSpiritTeamFailed {
        return
    }
    spiritSessionID, _ := env.Metadata["spirit_session_id"].(string)
    if spiritSessionID == "" {
        return
    }

    activeTeams, _ := o.spiritAssembler.ListActiveTeams(ctx, spiritSessionID)
    if len(activeTeams) == 0 {
        o.publishSpiritTeamsAllCompleted(ctx, spiritSessionID)
    }
}
```

新增 EnvelopeType：

```go
const (
    EnvelopeTypeSpiritTeamProgress       EnvelopeType = "spirit_team_progress"
    EnvelopeTypeSpiritTeamsAllCompleted  EnvelopeType = "spirit_teams_all_completed"
    EnvelopeTypeSpiritSynthesisCompleted EnvelopeType = "spirit_synthesis_completed"
)
```

### 2.7 前端并行团队面板

**useSpiritTeamStore 扩展**：

```typescript
interface SpiritTeamState {
  teams: SpiritTeam[]
  expandedTeamIds: Set<string>
  activePanelMode: 'spirit' | 'team' | 'member'
  activeTeamId: string | null
  activeMemberId: string | null
  loading: boolean
  parallelConfig: ParallelConfig
  synthesisResult: SynthesisResult | null
}

interface ParallelConfig {
  maxConcurrentTeams: number
  maxTeamConcurrency: number
  teamTimeoutMs: number
  autoArchiveAfterMs: number
  maxSessionDepth: number
}

interface SynthesisResult {
  taskResults: TaskSynthesis[]
  summary: string
  allSuccess: boolean
}
```

新增 actions：

- `checkTeamProgress()` — 调用 `check_team_progress` 工具查询进度
- `cancelTeam(teamId)` — 调用 `cancel_team` 工具取消团队
- `synthesizeResults()` — 调用 `synthesize_results` 工具合成结果（P2）

新增组件：

- `ParallelTeamOverview.vue` — 并行团队总览卡片（精灵对话中）
- `TeamProgressCard.vue` — 单团队进度卡片

---

## 三、Phase 2 设计：智能编排

### 3.1 Task DAG 数据模型

```go
type TaskNode struct {
    ID           string   `json:"id"`
    TaskPrompt   string   `json:"task_prompt"`
    AgentIDs     []string `json:"agent_ids"`
    Mode         string   `json:"mode"`
    Dependencies []string `json:"dependencies"`
    Priority     int      `json:"priority"`
}

type TaskDAG struct {
    Nodes           []*TaskNode `json:"nodes"`
    SpiritSessionID string      `json:"spirit_session_id"`
}

func (d *TaskDAG) Validate() error {
    if len(d.Nodes) == 0 {
        return kerrors.BadRequest("SPIRIT", "DAG must have at least one node")
    }
    nodeMap := make(map[string]*TaskNode, len(d.Nodes))
    for _, n := range d.Nodes {
        if _, exists := nodeMap[n.ID]; exists {
            return kerrors.BadRequest("SPIRIT", fmt.Sprintf("duplicate node ID: %s", n.ID))
        }
        nodeMap[n.ID] = n
    }
    for _, n := range d.Nodes {
        for _, dep := range n.Dependencies {
            if _, exists := nodeMap[dep]; !exists {
                return kerrors.BadRequest("SPIRIT", fmt.Sprintf("dependency %s not found", dep))
            }
        }
    }
    if d.hasCycle() {
        return kerrors.BadRequest("SPIRIT", "DAG contains cycle")
    }
    return nil
}

func (d *TaskDAG) hasCycle() bool {
    visited := make(map[string]bool)
    inStack := make(map[string]bool)
    for _, n := range d.Nodes {
        if d.dfs(n.ID, visited, inStack) {
            return true
        }
    }
    return false
}
```

### 3.2 拓扑路由算法

对齐 AdaptOrch (arXiv:2602.16873) 的 O(|V|+|E|) 拓扑路由：

```go
func (d *TaskDAG) RouteTopology() string {
    if len(d.Nodes) == 1 {
        return "sequential"
    }

    allIndependent := true
    hasDeps := false
    maxDepth := 0
    maxWidth := 0

    for _, n := range d.Nodes {
        if len(n.Dependencies) > 0 {
            hasDeps = true
            allIndependent = false
        }
    }

    if allIndependent {
        return "parallel"
    }

    depth := d.computeDepth()
    width := d.computeWidth()

    if depth > 3 {
        return "coordinator"
    }
    if width > 1 {
        return "hybrid"
    }
    return "sequential"
}

func (d *TaskDAG) computeDepth() int {
    depthMap := make(map[string]int)
    var calcDepth func(id string) int
    calcDepth = func(id string) int {
        if d, ok := depthMap[id]; ok {
            return d
        }
        node := d.nodeByID(id)
        maxDep := 0
        for _, dep := range node.Dependencies {
            if d := calcDepth(dep); d > maxDep {
                maxDep = d
            }
        }
        depthMap[id] = maxDep + 1
        return depthMap[id]
    }
    maxDepth := 0
    for _, n := range d.Nodes {
        if d := calcDepth(n.ID); d > maxDepth {
            maxDepth = d
        }
    }
    return maxDepth
}

func (d *TaskDAG) computeWidth() int {
    levelMap := make(map[int]int)
    for _, n := range d.Nodes {
        level := d.computeDepth()
        levelMap[level]++
    }
    maxWidth := 0
    for _, w := range levelMap {
        if w > maxWidth {
            maxWidth = w
        }
    }
    return maxWidth
}
```

### 3.3 依赖感知调度

当 Task DAG 包含依赖时，依赖团队在前置团队完成后自动启动：

```go
type DependencyScheduler struct {
    teamUC     TeamUsecasePort
    sessionUC  SessionTreeReader
    spiritUC   SpiritTeamAssemblerPort
}

func (s *DependencyScheduler) OnTeamCompleted(ctx context.Context, teamID string) error {
    team, _ := s.teamUC.Get(ctx, teamID)
    spiritSessionID := team.SpiritSessionID

    teams, _ := s.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
    for _, t := range teams {
        if t.Status != "waiting_deps" {
            continue
        }
        deps := s.parseDependencies(t)
        if s.allDepsCompleted(deps, teams) {
            s.spiritUC.StartTeam(ctx, t.ID)
        }
    }
    return nil
}
```

团队新增状态 `waiting_deps`：

```
assembling → waiting_deps → running → completed
                                  → failed
                                  → cancelled
```

### 3.4 Synthesis Engine

对齐 Maestro (arXiv:2511.06134) 的探索-合成分离：

```go
type SynthesisEngine struct {
    teamUC    TeamUsecasePort
    sessionUC SessionTreeReader
    lg        loggateway.Logger
}

type SynthesisResult struct {
    TaskResults []TaskSynthesis `json:"task_results"`
    Summary     string          `json:"summary"`
    AllSuccess  bool            `json:"all_success"`
}

type TaskSynthesis struct {
    TeamID     string `json:"team_id"`
    TeamName   string `json:"team_name"`
    TaskPrompt string `json:"task_prompt"`
    Status     string `json:"status"`
    Output     string `json:"output"`
    DurationMs int64  `json:"duration_ms"`
}

func (e *SynthesisEngine) Synthesize(ctx context.Context, spiritSessionID string) (*SynthesisResult, error) {
    teams, err := e.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
    if err != nil {
        return nil, err
    }

    var results []TaskSynthesis
    allSuccess := true
    for _, t := range teams {
        if t.Status != "completed" && t.Status != "failed" {
            continue
        }
        output := e.extractTeamOutput(ctx, t.ID)
        results = append(results, TaskSynthesis{
            TeamID:     t.ID,
            TeamName:   t.DisplayName,
            TaskPrompt: t.TaskDescription,
            Status:     t.Status,
            Output:     output,
        })
        if t.Status == "failed" {
            allSuccess = false
        }
    }

    summary := e.generateSummary(ctx, results)
    return &SynthesisResult{
        TaskResults: results,
        Summary:     summary,
        AllSuccess:  allSuccess,
    }, nil
}
```

**合成策略**：

| 场景 | 策略 |
|------|------|
| 全部成功 + < 3 团队 | 模板合成（拼接各团队摘要） |
| 全部成功 + >= 3 团队 | LLM 合成（调用精灵 LLM 生成综合摘要） |
| 部分失败 | 混合合成（成功团队模板 + 失败团队标注 + LLM 总结） |
| 全部失败 | LLM 合成（分析失败原因 + 建议） |

### 3.5 synthesize_results 工具

```go
func NewSynthesizeResultsTool(synthesisPort SynthesisPort) trpctool.Tool {
    return function.NewFunctionTool[SynthesizeResultsInput, SynthesizeResultsOutput](
        "synthesize_results",
        "Synthesize the results of all completed teams under the current spirit session into a unified summary.",
        func(ctx context.Context, _ SynthesizeResultsInput) (SynthesizeResultsOutput, error) {
            spiritSessionID := spiritSessionIDFromCtx(ctx)
            result, err := synthesisPort.Synthesize(ctx, spiritSessionID)
            if err != nil {
                return SynthesizeResultsOutput{}, err
            }
            return SynthesizeResultsOutput{
                Summary:     result.Summary,
                AllSuccess:  result.AllSuccess,
                TaskResults: result.TaskResults,
            }, nil
        },
    )
}
```

### 3.6 编排进化闭环

DQ Score 驱动的编排缓存与策略优化：

```go
type OrchestrationCache struct {
    TopologyHash string  `json:"topology_hash"`
    Mode         string  `json:"mode"`
    DQScore      float64 `json:"dq_score"`
    TaskPattern  string  `json:"task_pattern"`
    HitCount     int     `json:"hit_count"`
}

func (c *OrchestrationCache) ShouldUse(dag *TaskDAG) (string, bool) {
    pattern := dag.TaskPatternHash()
    if cached, ok := c.Lookup(pattern); ok && cached.DQScore > 0.7 {
        return cached.Mode, true
    }
    return "", false
}
```

存储位置：`AgentRuntimeSettings.ExtraJSON` 中 `orchestration_cache` 键。

进化闭环集成：

```
团队执行完成
  → 计算 DQ Score (Validity * 0.4 + Specificity * 0.3 + Correctness * 0.3)
  → DQ Score > 0.7 → 缓存编排拓扑
  → DQ Score < 0.5 → EvolutionUsecase 生成编排优化建议
  → 下次 assemble_team → 先查缓存，命中则复用
```

---

## 四、API 扩展

### 4.1 Team Proto

```protobuf
message Team {
  // 已有字段...
  string spirit_session_id = 20;
  string task_description = 21;
  bool auto_created = 22;
  string dag_node_id = 23;          // P2: DAG 节点 ID
  repeated string depends_on = 24;  // P2: 依赖的团队 ID 列表
  string parallel_config_json = 25; // 并行配置 JSON
}
```

### 4.2 精灵并行查询 API

```protobuf
message ListActiveTeamsRequest {
  string spirit_session_id = 1;
}

message TeamProgressView {
  string team_id = 1;
  string team_name = 2;
  string status = 3;
  double progress_pct = 4;
  string current_step = 5;
  int64 duration_ms = 6;
}

message SynthesizeResultsRequest {
  string spirit_session_id = 1;
}

message SynthesizeResultsResponse {
  string summary = 1;
  bool all_success = 2;
  repeated TaskSynthesisView task_results = 3;
}
```

---

## 五、测试策略

| 层 | 文件 | 覆盖 | 阶段 |
|----|------|------|------|
| Biz | `spirit_parallel_config_test.go` | ParallelConfig 默认值、校验 | P1 |
| Biz | `spirit_team_parallel_test.go` | ListActiveTeams、并行度检查 | P1 |
| Biz | `spirit_task_dag_test.go` | DAG 校验、环检测、拓扑路由 | P2 |
| Biz | `spirit_synthesis_test.go` | 合成策略（模板/LLM/混合） | P2 |
| Biz | `orchestration_cache_test.go` | 缓存命中、DQ Score 阈值 | P2 |
| Service | `spirit_team_test.go` | 多团队 AssembleTeam、CancelTeam | P1 |
| Service | `spirit_synthesis_test.go` | Synthesize 流程 | P2 |
| 前端 | `useSpiritTeamStore.spec.ts` | 并行团队状态、进度查询 | P1 |
| 前端 | `ParallelTeamOverview.spec.ts` | 并行团队总览卡片 | P1 |

---

## 六、与关联模块

| 模块 | 关系 |
|------|------|
| M59 Chat 管家模式 | 前置依赖：精灵模式基础骨架 |
| 11 Team | 团队创建扩展、TeamKey UUID、依赖调度 |
| 53 Orchestration | Task DAG 拓扑路由、Graph 引擎复用 |
| 10 Session | Session 树深度限制 |
| 7 Agent Evolution | DQ Score 驱动编排缓存、进化闭环 |
| 39 Planner | 远期：A2UI Planner 生成结构化执行计划 |
| superpowers Builtin Agents | 精灵工具扩展（3 个新工具） |
| superpowers Learning Loop | 编排策略 Pattern 检测 → Proposal |
