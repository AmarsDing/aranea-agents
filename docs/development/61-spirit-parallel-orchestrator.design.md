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
    TeamTimeoutSeconds int           `json:"team_timeout_seconds"`   // 秒数（JSON 序列化友好）
    AutoArchiveSeconds int           `json:"auto_archive_seconds"`   // 秒数（JSON 序列化友好）
    MaxSessionDepth    int           `json:"max_session_depth"`
}

func DefaultParallelConfig() ParallelConfig {
    return ParallelConfig{
        MaxConcurrentTeams: 3,
        MaxTeamConcurrency: 2,
        TeamTimeoutSeconds: 600,    // 10 分钟
        AutoArchiveSeconds: 3600,   // 1 小时
        MaxSessionDepth:    2,
    }
}

// 辅助方法：将秒数转换为 time.Duration
func (c ParallelConfig) TeamTimeout() time.Duration {
    return time.Duration(c.TeamTimeoutSeconds) * time.Second
}

func (c ParallelConfig) AutoArchiveAfter() time.Duration {
    return time.Duration(c.AutoArchiveSeconds) * time.Second
}
```

存储位置：`AgentRuntimeSettings.ExtraJSON` 中 `parallel_config` 键，精灵 Agent 种子数据中注入默认值。

### 2.4 SpiritTeamAssemblerPort 接口扩展

代码中拆分为 3 个小接口（遵循接口隔离原则）：

```go
// 团队组装端口
type SpiritTeamAssemblerPort interface {
    AssembleTeam(ctx context.Context, params biz.SpiritTeamParams) (biz.Team, biz.Session, error)
    SuggestTopology(ctx context.Context, taskDescription string) (string, bool)
}

// 团队查询端口
type SpiritTeamQueryPort interface {
    ListActiveTeams(ctx context.Context, spiritSessionID string) ([]biz.Team, error)
    ListAllTeams(ctx context.Context, spiritSessionID string) ([]biz.Team, error)
    GetMaxParallelTeams(ctx context.Context, spiritSessionID string) int
}

// 团队控制端口
type SpiritTeamControllerPort interface {
    CancelTeam(ctx context.Context, teamID string) error
    CheckTeamProgress(ctx context.Context, spiritSessionID string) ([]biz.TeamProgress, error)
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
type TopologyType string

const (
    TopologyDirect      TopologyType = "direct"      // 单 Agent 直连（RouteTopology 不返回此值）
    TopologyParallel    TopologyType = "parallel"    // 所有节点无依赖
    TopologySequential  TopologyType = "sequential"  // 有依赖但宽度=1
    TopologyHybrid      TopologyType = "hybrid"      // 部分并行+部分串行
    TopologyCoordinator TopologyType = "coordinator" // 依赖链深度>3
)

func (d *TaskDAG) RouteTopology() TopologyType {
    if len(d.Nodes) == 0 {
        return TopologyCoordinator
    }
    if len(d.Roots) == len(d.Nodes) {
        return TopologyParallel
    }
    depth := d.computeDepth()
    width := d.computeMaxWidth()
    if depth > 3 {
        return TopologyCoordinator
    }
    if width > 1 {
        return TopologyHybrid
    }
    return TopologySequential
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

团队新增状态 `pending`（替代旧的 `waiting_deps`/`assembling`/`assembled`）：

```
pending → running → completed → archived
                 → failed → archived
                 → cancelled → archived
                 → interrupted → running
```

完整状态常量（`internal/biz/team_types.go`）：

| 常量 | 值 | 说明 |
|------|-----|------|
| `TeamStatusPending` | `"pending"` | 已创建，等待执行 |
| `TeamStatusRunning` | `"running"` | 正在执行 |
| `TeamStatusCompleted` | `"completed"` | 成功完成 |
| `TeamStatusFailed` | `"failed"` | 执行失败 |
| `TeamStatusCancelled` | `"cancelled"` | 已取消 |
| `TeamStatusInterrupted` | `"interrupted"` | 异常中断，可恢复 |
| `TeamStatusArchived` | `"archived"` | 自动归档 |
| `TeamStatusBlocked` | `"blocked"` | 虚拟状态，仅用于级联阻塞结果展示，不持久化 |

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
| 全部成功 + >= 3 团队 | 混合合成（模板 + Prompt 综合摘要） |
| 部分失败 | 混合合成（成功团队模板 + 失败团队标注 + Prompt 总结） |
| 全部失败 | 混合合成（分析失败原因 + 建议） |

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

---

## 七、Phase 4 设计：智能增强

> 基于 AI Agent 工作模式启发，补充 4 个关键增强：任务复杂度分级、Graph DAG 编排、自适应 Team 模式、验证门禁。

### 7.1 GAP-1：TaskComplexityClassifier — 任务复杂度分级器

**问题**：Spirit 的"简单问答直接回答"是 Prompt 级别软约束，LLM 可能对复杂度判断不一致。

**方案**：新增 `assess_complexity` 工具，形式化复杂度评估。

#### 7.1.1 工具定义

> **注意**：`assess_complexity` 工具已标记 DEPRECATED，委托给 `plan_and_execute` 三阶段流程。
> 当 `TaskPlannerPort` 可用时，优先委托给 `Plan()` 方法；否则回退到规则引擎。

```go
// internal/tools/spirit_tools.go
type AssessComplexityInput struct {
    UserMessage string `json:"user_message" jsonschema:"description=用户消息内容"`
}

type AssessComplexityOutput struct {
    Level          string   `json:"level"`            // "simple" | "moderate" | "complex"
    Reasoning      string   `json:"reasoning"`        // 复杂度判断理由
    SuggestedPath  string   `json:"suggested_path"`   // "direct_answer" | "single_butler" | "orchestrator"
    RequiredSkills []string `json:"required_skills"`  // 需要的能力标签
    AvailableTools []string `json:"available_tools"`  // 可用工具列表
}
```

#### 7.1.2 规则引擎

```go
// internal/tools/spirit_complexity.go
type ComplexityLevel string

const (
    ComplexitySimple   ComplexityLevel = "simple"
    ComplexityModerate ComplexityLevel = "moderate"
    ComplexityComplex  ComplexityLevel = "complex"
)

type ComplexityRuleEngine struct {
    mu            sync.Mutex
    lastReasoning string
}

func NewComplexityRuleEngine() *ComplexityRuleEngine {
    return &ComplexityRuleEngine{}
}

func (r *ComplexityRuleEngine) Assess(message string) ComplexityLevel {
    return r.AssessDetailed(message).Level
}

func (r *ComplexityRuleEngine) AssessDetailed(message string) ComplexityAssessment {
    r.mu.Lock()
    defer r.mu.Unlock()

    lower := strings.ToLower(message)

    // 1. 简单问答模式（最高优先级）
    for _, p := range lowerSimplePatterns {
        if strings.Contains(lower, p) {
            r.lastReasoning = "匹配简单问答模式: " + p
            return ComplexityAssessment{
                Level: ComplexitySimple, SuggestedPath: PathDirectAnswer,
                RequiredSkills: nil, Reasoning: r.lastReasoning,
            }
        }
    }

    // 2. 复杂任务指标
    complexHits := 0
    for _, p := range lowerComplexIndicators {
        if strings.Contains(lower, p) { complexHits++ }
    }
    if complexHits >= 2 {
        r.lastReasoning = fmt.Sprintf("匹配 %d 个复杂任务指标", complexHits)
        return ComplexityAssessment{
            Level: ComplexityComplex, SuggestedPath: PathOrchestrator,
            RequiredSkills: complexAvailableTools, Reasoning: r.lastReasoning,
        }
    }
    if complexHits == 1 {
        r.lastReasoning = "匹配 1 个复杂任务指标，但不足以确定，降级为 moderate"
        return ComplexityAssessment{
            Level: ComplexityModerate, SuggestedPath: PathSingleButler,
            RequiredSkills: moderateAvailableTools, Reasoning: r.lastReasoning,
        }
    }

    // 3. 中等任务指标
    moderateHits := 0
    for _, p := range moderateIndicators {
        if strings.Contains(lower, p) { moderateHits++ }
    }
    if moderateHits >= 1 {
        r.lastReasoning = "匹配中等任务指标"
        return ComplexityAssessment{
            Level: ComplexityModerate, SuggestedPath: PathSingleButler,
            RequiredSkills: moderateAvailableTools, Reasoning: r.lastReasoning,
        }
    }

    r.lastReasoning = "无法通过规则确定复杂度，使用安全默认值 moderate"
    return ComplexityAssessment{
        Level: ComplexityModerate, SuggestedPath: PathSingleButler,
        RequiredSkills: moderateAvailableTools, Reasoning: r.lastReasoning,
    }
}
```

关键词列表（`internal/tools/spirit_complexity.go`）：

| 类别 | 关键词 |
|------|--------|
| 简单问答 | "什么是"、"解释一下"、"帮我看看"、"怎么用"、"查询"、"查找"、"搜索"、"what is"、"explain"、"show me" |
| 复杂任务 | "分析"、"对比"、"编写"、"设计"、"规划"、"编排"、"重构"、"迁移"、"集成"、"部署"、"优化"、"analyze"、"compare"、"design"、"plan" |
| 中等任务 | "创建"、"修改"、"更新"、"删除"、"配置"、"修复"、"调试"、"测试"、"转换"、"create"、"modify"、"fix"、"debug"、"test" |

#### 7.1.3 Spirit Prompt 增强

```
## 决策规则（强制）
1. 收到用户消息后，先调用 assess_complexity 评估复杂度
2. 根据评估结果路由：
   - simple → 直接回答，不委派
   - moderate → 委派给最相关的单一管家
   - complex → 委派给 __orchestrator__
3. 禁止跳过 assess_complexity 直接委派
4. 禁止对 simple 级别任务委派给管家
```

### 7.2 GAP-2：GraphOrchestration — 编排管家生成 Graph DAG

**问题**：编排管家通过线性工具调用序列执行任务，无法表达并行/条件路由，无法利用 Graph 的检查点、中断、重试能力。

**方案**：新增 `build_orchestration_graph` 工具，动态生成 `GraphBuildConfig`。

#### 7.2.1 工具定义

```go
// internal/tools/orchestrator/build_graph.go
type AgentAssignment struct {
    AgentKey  string   `json:"agent_key" jsonschema:"description=Agent key"`
    Role      string   `json:"role" jsonschema:"description=Agent role in the task"`
    SubTask   string   `json:"sub_task" jsonschema:"description=Sub-task description for this agent"`
    DependsOn []string `json:"depends_on" jsonschema:"description=Agent keys this agent depends on"`
}

type BuildOrchestrationGraphInput struct {
    TaskDescription string            `json:"task_description" jsonschema:"description=Overall task description"`
    Agents          []AgentAssignment `json:"agents" jsonschema:"description=Agent assignments for the graph"`
    Mode            string            `json:"mode" jsonschema:"description=Graph mode: parallel|sequential|hybrid|coordinator"`
}

type BuildOrchestrationGraphOutput struct {
    GraphBuildConfig  biz.GraphBuildConfig `json:"graph_build_config"`
    GraphExecutionID  string               `json:"graph_execution_id,omitempty"`
    NodeCount         int                  `json:"node_count"`
    EdgeCount         int                  `json:"edge_count"`
    VerificationNodes []string             `json:"verification_nodes"`
}
```

#### 7.2.2 Graph 动态构建核心逻辑

```go
func BuildGraphConfig(input BuildOrchestrationGraphInput) biz.GraphBuildConfig {
    var nodes []biz.NodeDef
    var edges []biz.EdgeDef

    // 1. 入口节点
    entryNode := "entry"
    nodes = append(nodes, biz.NodeDef{ID: entryNode, Type: biz.NodeTypeFunction})

    // 2. 为每个 Agent 创建节点
    agentKeys := make(map[string]bool)
    for _, a := range input.Agents {
        agentKeys[a.AgentKey] = true
        nodes = append(nodes, biz.NodeDef{
            ID:          a.AgentKey,
            Type:        biz.NodeTypeAgent,
            AgentName:   a.AgentKey,
            Instruction: a.SubTask,
        })
    }

    // 3. 根据依赖关系生成边（悬空依赖跳过）
    for _, a := range input.Agents {
        if len(a.DependsOn) == 0 {
            edges = append(edges, biz.EdgeDef{From: entryNode, To: a.AgentKey})
            continue
        }
        hasValidDep := false
        for _, dep := range a.DependsOn {
            if !agentKeys[dep] { continue }
            edges = append(edges, biz.EdgeDef{From: dep, To: a.AgentKey})
            hasValidDep = true
        }
        if !hasValidDep {
            edges = append(edges, biz.EdgeDef{From: entryNode, To: a.AgentKey})
        }
    }

    // 4. 循环检测：DFS 三色标记法，检测到环时降级为顺序链
    if hasCycle(nodes, edges) {
        // 降级：清空边，重建为顺序链
        edges = edges[:0]
        for i, a := range input.Agents {
            if i == 0 {
                edges = append(edges, biz.EdgeDef{From: entryNode, To: a.AgentKey})
            } else {
                edges = append(edges, biz.EdgeDef{From: input.Agents[i-1].AgentKey, To: a.AgentKey})
            }
        }
    }

    // 5. 汇合节点：叶子 Agent（不被其他 Agent 依赖的）连接到 merge
    mergeNode := "merge_results"
    nodes = append(nodes, biz.NodeDef{ID: mergeNode, Type: biz.NodeTypeFunction})
    for _, a := range input.Agents {
        if isDependedOn(input.Agents, a.AgentKey) { continue }
        edges = append(edges, biz.EdgeDef{From: a.AgentKey, To: mergeNode})
    }

    // 6. 完成节点
    finishNode := "finish"
    nodes = append(nodes, biz.NodeDef{ID: finishNode, Type: biz.NodeTypeFunction})
    edges = append(edges, biz.EdgeDef{From: mergeNode, To: finishNode})

    return biz.GraphBuildConfig{
        Nodes:            nodes,
        Edges:            edges,
        EntryPoint:       entryNode,
        FinishPoint:      finishNode,
        EnableCheckpoint: true,
        ExecutionEngine:  biz.EngineDAG,
        StateFields: []biz.StateFieldDef{
            {Name: "task_description", Reducer: biz.ReducerDefault},
            {Name: "agent_results", Reducer: biz.ReducerMerge},
        },
    }
}
```

#### 7.2.3 与 `assemble_team` 的关系

P0 阶段两者共存，编排管家 LLM 根据任务复杂度选择：
- 简单任务（2-3 Agent，顺序执行）→ `assemble_team`
- 复杂任务（4+ Agent，有并行/条件路由）→ `build_orchestration_graph`

P1 阶段 `assemble_team` 内部重构为调用 `build_orchestration_graph`，统一走 Graph 引擎。

### 7.3 GAP-3：AdaptiveTeamMode — 自适应 Team 模式选择

**问题**：Spirit Team 固定使用 `ModeCoordinator`，不同任务特征适合不同模式。

**方案**：在 `buildSpiritTeam` 中根据 `assess_complexity` 的输出动态选择 Team 构建方式。

#### 7.3.1 Team 模式选择

```go
// internal/service/chat_orchestrator_spirit.go
type SpiritTeamMode string

const (
    SpiritModeCoordinator SpiritTeamMode = "coordinator"
    SpiritModeSwarm       SpiritTeamMode = "swarm"
    SpiritModeDirect      SpiritTeamMode = "direct"
)

func (o *ChatOrchestrator) buildSpiritTeam(
    ctx context.Context, spiritAg biz.Agent, deps chatagent.TRPCBuilderDeps,
    mode SpiritTeamMode,
) (agent.Agent, error) {
    spiritAgent, err := chatagent.BuildTRPCAgentCached(ctx, spiritAg, deps)
    if err != nil { return nil, err }

    butlers, err := o.loadSystemButlers(ctx, deps)
    if err != nil { return nil, err }

    switch mode {
    case SpiritModeCoordinator:
        return trpcteam.New(spiritAgent, butlers)
    case SpiritModeSwarm:
        return trpcteam.NewSwarm(
            "spirit_swarm", spiritAgent.Info().Name,
            append([]agent.Agent{spiritAgent}, butlers...),
        )
    case SpiritModeDirect:
        targetKey := ctx.Value(ctxKeyTargetButler).(string)
        for _, b := range butlers {
            if b.Info().Name == targetKey { return b, nil }
        }
        return spiritAgent, nil
    default:
        return trpcteam.New(spiritAgent, butlers)
    }
}
```

#### 7.3.2 模式选择逻辑

| assess_complexity 输出 | Team 模式 | 说明 |
|----------------------|----------|------|
| `direct_answer` | Direct | 不构建 Team |
| `single_butler` | Direct | 直接路由到目标管家 |
| `orchestrator` | Coordinator | 完整 Team 编排 |

P1 增强：基于历史编排记录的成功率自动优化模式选择。

### 7.4 GAP-4：VerificationGate — 编排验证门禁节点

**问题**：Graph 有 `interrupt_before`/`interrupt_after` 用于 HITL，但没有自动化验证门禁。

**方案**：在 Graph 中增加验证节点（Verification Node），利用 `NodeDef.FailureAction` + 自定义验证函数实现自动验证。

#### 7.4.1 验证节点类型

```go
// internal/tools/orchestrator/verification.go
type VerificationType string

const (
    VerifyOutputFormat   VerificationType = "output_format"
    VerifyTaskCompletion VerificationType = "task_completion"
    VerifyHumanApproval VerificationType = "human_approval"
)
```

#### 7.4.2 验证函数

```go
// internal/tools/orchestrator/verify_funcs.go
func verifyOutputFormat(ctx context.Context, state graph.State) (any, error) {
    results, ok := state["agent_results"]
    if !ok { return nil, fmt.Errorf("no agent results found") }

    agentResults, ok := results.(map[string]any)
    if !ok { return nil, fmt.Errorf("agent_results is not a map") }

    var issues []string
    for agentKey, result := range agentResults {
        resultStr, ok := result.(string)
        if !ok || resultStr == "" {
            issues = append(issues, fmt.Sprintf("agent %s returned empty result", agentKey))
        }
    }

    if len(issues) > 0 {
        return map[string]any{"verified": false, "issues": issues}, nil
    }
    return map[string]any{"verified": true}, nil
}
```

#### 7.4.3 验证节点注入

在 `buildGraphConfig` 中根据验证类型注入验证节点：

| 验证类型 | 注入位置 | FailureAction | 说明 |
|---------|---------|---------------|------|
| output_format | merge 后 | Skip | 格式验证失败不阻塞 |
| task_completion | merge 后 | RetryThenBlock | 完成度不足则重试 |
| human_approval | 关键节点前 | interrupt_before | 暂停等待人工确认 |

### 7.5 整体架构

```
用户消息
  │
  ▼
Spirit（总管家）
  ├── GAP-1: assess_complexity
  │     ├── simple → direct_answer
  │     ├── moderate → single_butler
  │     └── complex → orchestrator
  │
  └── GAP-3: AdaptiveTeamMode
        ├── direct_answer → 不构建 Team
        ├── single_butler → SpiritModeDirect
        └── orchestrator → SpiritModeCoordinator
              │
              ▼
        Orchestrator（编排管家）
          ├── 现有工具链：classify_industry → search_positions → find_agents
          │
          └── GAP-2: build_orchestration_graph
                │  动态生成 GraphBuildConfig
                │
                ▼
          Graph Engine
            entry → [Agent A] ─┐
                   [Agent B] ─┼→ merge → verify → finish
                   [Agent C] ─┘          ↑
                                  GAP-4: VerificationGate
```

### 7.6 新增文件清单

```
internal/
  tools/
    spirit_complexity.go           # GAP-1: 复杂度评估工具 + 规则引擎（已合并为单文件）
    spirit_tools.go                # GAP-1: assess_complexity 工具注册（DEPRECATED，委托 plan_and_execute）
    orchestrator/
      build_graph.go               # GAP-2: Graph 构建工具 + GraphBuilderPort 接口
      verification.go              # GAP-4: 验证节点类型定义 + injectVerificationNodes
      verify_funcs.go              # GAP-4: 验证函数实现
  service/
    chat_orchestrator_spirit.go    # GAP-3: Spirit Team 构建 + 模式选择
  scenario/system/prompts/
    spirit.md                      # 修改：增加强制决策规则
    orchestrator.md                # 修改：增加 Graph 编排决策规则
```

### 7.7 依赖关系

| 依赖 | 说明 |
|------|------|
| M59 精灵模式骨架 | 前置：Spirit 种子数据 + CustomTools 机制 |
| team-graph-optimization | 前置：GraphBuilderFactory 拆分后接口稳定 |
| Graph 引擎 interrupt 机制 | GAP-4 依赖：interrupt_before/interrupt_after |
| `team.New()` / `team.NewSwarm()` | GAP-3 依赖：框架 Team 构造 API |


---

## 子模块：Spirit Parallel Orchestrator Deep Design

> **版本**：2026-06-06
> **定位**：基于需求文档 [60-spirit-parallel-orchestrator.md](./60-spirit-parallel-orchestrator.md) 和现有骨架代码，分析每个验收标准的实现差距，给出深度业务实现的详细设计方案。
> **前置**：P1 + P2 骨架已完成，Wire 注入链已修复，集成问题已修复。
> **实现状态**：✅ P0/P1/P2 差距已全部修复 + 深度架构审查修复已完成（2026-06-06）

---

## 一、差距总览

### 1.1 验收标准差距矩阵

| 验收 ID | 摘要 | 骨架状态 | 差距级别 | 差距描述 |
|---------|------|---------|---------|---------|
| SPO-01 | 同一精灵 Session 支持多团队并行 | ✅ 骨架完成 | **P0** | 团队创建后不自动启动 Runner，处于"悬停"状态 |
| SPO-02 | 并行度可配置，超限拒绝 | ✅ 骨架完成 | P2 | TeamTimeout/AutoArchiveAfter/MaxSessionDepth 未实际使用 |
| SPO-03 | 团队进度实时监控 + 精灵主动通知 | ⚠️ 部分完成 | **P1** | 进度只有 0%/100%，无中间进度；CurrentStep 始终为空 |
| SPO-04 | 取消团队 + 释放配额 | ✅ 骨架完成 | — | 无差距 |
| SPO-05 | Task DAG 依赖调度 | ⚠️ 部分完成 | **P0** | 依赖团队激活后不自动启动 Runner；DependencyScheduler 死代码 |
| SPO-06 | 拓扑路由自动选择编排模式 | ✅ 骨架完成 | P1 | RouteTopology 简化版，computeDepth/computeWidth 未实现 |
| SPO-07 | Synthesis Engine 结果合成 | ⚠️ 部分完成 | **P0** | Summary/KeyFindings 始终为空；部分失败场景未处理 |
| SPO-08 | DQ Score 驱动编排缓存 | ⚠️ 部分完成 | **P1** | 缓存仅内存存储重启丢失；DQ Score 仅时间惩罚 |
| SPO-09 | 编排策略进化闭环 | ⚠️ 部分完成 | **P1** | DQ<0.5 优化建议未生成；进化护栏未接入 |

### 1.2 差距分级定义

| 级别 | 含义 | 影响 |
|------|------|------|
| **P0** | 业务断裂 — 核心流程无法走通 | 用户无法获得预期功能 |
| **P1** | 验收不通过 — 功能存在但不满足验收标准 | 验收失败 |
| **P2** | 功能不完善 — 有定义但未实际使用 | 用户体验不完整 |

---

## 二、P0 问题详细设计

### 2.1 P0-01：团队创建后无自动启动 Runner

**现状**：`AssembleTeam` 只创建 Team + Session 记录，不触发 Runner 执行。团队处于 `"active"` 状态但无实际运行。

**根因**：精灵 Agent 的 `assemble_team` 工具返回 `team_id` 和 `session_id`，但没有任何代码向团队 session 发送初始消息来触发 `RunTurnFromInput`。

**设计方案**：在 `SpiritTeamAssembler.AssembleTeam` 完成后，自动向团队 session 发送初始消息触发 Runner 执行。

#### 2.1.1 新增 `start_team` 精灵工具

```
工具名：start_team
输入：{ team_id: string, initial_message: string }
输出：{ team_id: string, status: string, session_id: string }
职责：向指定团队的 session 发送初始消息，触发 Runner 执行
```

**实现位置**：`internal/tools/spirit_tools.go`

```go
type StartTeamInput struct {
    TeamID         string `json:"team_id" jsonschema:"description=要启动的团队 ID"`
    InitialMessage string `json:"initial_message" jsonschema:"description=发送给团队的初始任务消息"`
}

type StartTeamOutput struct {
    TeamID    string `json:"team_id"`
    Status    string `json:"status"`
    SessionID string `json:"session_id"`
}
```

**端口扩展**：`SpiritTeamControllerPort` 新增方法：

```go
type SpiritTeamControllerPort interface {
    CancelTeam(ctx context.Context, teamID string) error
    CheckTeamProgress(ctx context.Context, spiritSessionID string) ([]biz.TeamProgress, error)
    StartTeam(ctx context.Context, teamID string, initialMessage string) (biz.Session, error)
}
```

#### 2.1.2 `SpiritTeamAssembler.StartTeam` 实现

**实现位置**：`internal/service/spirit_team.go`

```go
func (a *SpiritTeamAssembler) StartTeam(ctx context.Context, teamID string, initialMessage string) (biz.Session, error) {
    team, err := a.spiritUC.GetTeam(ctx, teamID)
    if err != nil {
        return biz.Session{}, err
    }
    if team.Status != "active" && team.Status != "assembled" {
        return biz.Session{}, kerrors.BadRequest("SPIRIT", "team must be active or assembled to start")
    }
    sessions, err := a.sessionUC.ListByTeamID(ctx, teamID)
    if err != nil || len(sessions) == 0 {
        return biz.Session{}, kerrors.NotFound("SPIRIT", "team session not found")
    }
    session := sessions[0]
    // 通过 ChatOrchestrator 发送消息触发 Runner
    // 需要 ChatOrchestrator 暴露一个内部方法
    return session, nil
}
```

**关键问题**：`StartTeam` 需要调用 `ChatOrchestrator.Execute` 来发送消息触发 Runner，但 `SpiritTeamAssembler` 不应直接依赖 `ChatOrchestrator`（循环依赖风险）。

**解决方案**：定义 `TeamStarterPort` 接口在 biz 层，由 `ChatOrchestrator` 实现：

```go
// internal/biz/spirit_team_usecase.go
type TeamStarterPort interface {
    StartTeamTurn(ctx context.Context, sessionID string, content string) error
    HandleTeamTurnResult(ctx context.Context, spiritSessionID, teamID, status, errMsg string)
}
```

`ChatOrchestrator` 实现此接口，`StartTeamTurn` 内部调用 `executeTeamTurnViaHooks`，`HandleTeamTurnResult` 处理团队 Turn 完成回调（取消超时定时器、触发依赖调度等）。

**Wire 绑定**：在 `cmd/admin/wire.go` 中将 `ChatOrchestrator` 作为 `TeamStarterPort` 注入 `SpiritTeamAssembler`。

#### 2.1.3 自动启动策略

**方案 A（推荐）**：`assemble_team` 工具返回后，精灵 Agent 自行决定是否调用 `start_team`。

- 优点：精灵 Agent 有控制权，可以在启动前做额外配置
- 缺点：依赖 LLM 正确调用 `start_team`，可能遗漏

**方案 B**：`assemble_team` 内部自动调用 `StartTeam`。

- 优点：流程简洁，不依赖 LLM 二次调用
- 缺点：无法在启动前做额外配置

**建议**：采用方案 B + 可选 `initial_message` 参数。`assemble_team` 的 `TaskPrompt` 字段自动作为初始消息发送给团队。

```go
// assemble_team 工具内部，AssembleTeam 成功后：
if params.TaskDescription != "" {
    starter.StartTeamTurn(ctx, session.ID, params.TaskDescription)
}
```

#### 2.1.4 依赖团队激活后自动启动

`scheduleDependentTeams` 在激活团队时，同步触发 Runner：

```go
// team_turn_hooks.go scheduleDependentTeams 中，Update 成功后：
if o.teamStarter != nil {
    sessions, _ := o.sessionUC.ListByTeamID(ctx, t.ID)
    if len(sessions) > 0 {
        go safego.Go(func() {
            o.teamStarter.StartTeamTurn(context.WithoutCancel(ctx), sessions[0].ID, t.TaskDescription)
        })
    }
}
```

---

### 2.2 P0-02：SynthesisResult 的 Summary/KeyFindings 始终为空

**现状**：`SpiritSynthesisService.SynthesizeResults` 构建 `TeamSynthesisResult` 时只填了 `TeamID/TeamName/TaskName/Status`，`Summary` 和 `KeyFindings` 始终为空。

**根因**：没有从团队执行结果中提取摘要和关键发现。

**设计方案**：从团队 Session 的最后一条 Assistant 消息中提取 Summary 和 KeyFindings。

#### 2.2.1 新增 `extractTeamOutput` 方法

**实现位置**：`internal/biz/spirit_team_usecase.go`

```go
func (u *SpiritTeamUsecase) ExtractTeamOutput(ctx context.Context, teamID string) (summary string, keyFindings string, err error) {
    sessions, err := u.sessionUC.ListByTeamID(ctx, teamID)
    if err != nil || len(sessions) == 0 {
        return "", "", nil
    }
    // 查询团队 session 的最后一条 assistant 消息
    messages, err := u.sessionUC.ListMessages(ctx, sessions[0].ID, 1)
    if err != nil || len(messages) == 0 {
        return "", "", nil
    }
    // 从最后一条 assistant 消息中提取
    for i := len(messages) - 1; i >= 0; i-- {
        if messages[i].Role == "assistant" {
            content := messages[i].ContentMarkdown
            summary = TruncateRunes(content, 500)
            keyFindings = extractKeyFindings(content)
            return summary, keyFindings, nil
        }
    }
    return "", "", nil
}
```

#### 2.2.2 `extractKeyFindings` 简单提取

```go
func extractKeyFindings(content string) string {
    // 提取 markdown 中的关键行（以 - / * / 1. / > 开头的行）
    var findings []string
    for _, line := range strings.Split(content, "\n") {
        trimmed := strings.TrimSpace(line)
        if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") ||
           strings.HasPrefix(trimmed, "> ") || regexp.MustCompile(`^\d+\.\s`).MatchString(trimmed) {
            if len(findings) < 5 {
                findings = append(findings, trimmed)
            }
        }
    }
    return strings.Join(findings, "\n")
}
```

#### 2.2.3 SynthesisService 集成

```go
// SpiritSynthesisService.SynthesizeResults 中：
for _, t := range teams {
    summary, keyFindings, _ := s.spiritUC.ExtractTeamOutput(ctx, t.ID)
    teamResults = append(teamResults, biz.TeamSynthesisResult{
        TeamID:      t.ID,
        TeamName:    t.DisplayName,
        TaskName:    t.TaskDescription,
        Status:      t.Status,
        Summary:     summary,
        KeyFindings: keyFindings,
    })
}
```

#### 2.2.4 部分失败合成

当前只合成 `completed` 的团队，`failed` 的被排除。需改为包含 `failed` 团队但标注原因：

```go
// 改为同时包含 completed 和 failed
for _, t := range teams {
    if t.Status != "completed" && t.Status != "failed" {
        continue
    }
    summary, keyFindings, _ := s.spiritUC.ExtractTeamOutput(ctx, t.ID)
    if t.Status == "failed" {
        summary = "[执行失败] " + summary
    }
    teamResults = append(teamResults, biz.TeamSynthesisResult{...})
}
```

**SynthesisEngine.inferStrategy** 同步更新：

```go
func (e *SynthesisEngine) inferStrategy(input SynthesisInput) SynthesisStrategy {
    if input.Template != "" {
        return SynthesisStrategyTemplate
    }
    hasFailed := false
    completedCount := 0
    for _, r := range input.TeamResults {
        if r.Status == "completed" {
            completedCount++
        }
        if r.Status == "failed" {
            hasFailed = true
        }
    }
    if hasFailed {
        return SynthesisStrategyHybrid
    }
    if completedCount == len(input.TeamResults) && len(input.TeamResults) <= 3 {
        return SynthesisStrategyTemplate
    }
    return SynthesisStrategyHybrid
}
```

**级联标注**（依赖链中断场景）：当 failed 团队的下游团队被阻塞时，在合成结果中标注：

```go
// 在 SynthesisService 中，检查 failed 团队的下游
for _, t := range teams {
    if t.Status == "failed" && t.DagNodeID != "" {
        for _, other := range teams {
            if containsString(other.DependsOn, t.DagNodeID) && other.Status == "waiting_deps" {
                teamResults = append(teamResults, biz.TeamSynthesisResult{
                    TeamID:   other.ID,
                    TeamName: other.DisplayName,
                    TaskName: other.TaskDescription,
                    Status:   "blocked",
                    Summary:  fmt.Sprintf("被失败团队 %s 阻塞", t.DisplayName),
                })
            }
        }
    }
}
```

---

### 2.3 P0-03：DependencyScheduler 已删除

**现状**：`DependencyScheduler`（`spirit_dependency_scheduler.go`）已在深度架构审查后删除。实际调度由 `team_turn_hooks.go` 中的 `scheduleDependentTeams` 实现。

**已完成**：
- 删除 `internal/biz/spirit_dependency_scheduler.go`
- 调度逻辑由 `SpiritTeamController.ScheduleDependentTeams` 承担
- `TeamAssemblerPort` 接口已移除，相关功能由拆分后的 3 个小接口承担

---

## 三、P1 问题详细设计

### 3.1 P1-01：团队进度只有 0%/100%，无中间进度

**现状**：`CheckTeamProgress` 只检查 Run 的 status，`success` 则 100%，否则 0%。`CurrentStep` 始终为空。

**设计方案**：基于团队 Session 的消息数量和工具调用进度计算中间进度。

#### 3.1.1 进度计算算法

```
进度 = (已完成步骤数 / 预估总步骤数) * 100

已完成步骤数 = 团队 session 中的 assistant 消息数
预估总步骤数 = max(已完成步骤数 + 1, ParallelConfig.MaxTeamConcurrency * 2)
```

更精确的方案：基于团队 Run 的 TurnInput/TurnResult 统计：

```go
func (u *SpiritTeamUsecase) CheckTeamProgress(ctx context.Context, spiritSessionID string) ([]TeamProgress, error) {
    // ... 查询所有团队 ...
    for i := range teams {
        tp := TeamProgress{...}
        runs, _ := u.teamUC.ListRuns(ctx, teams[i].ID, 10)
        if len(runs) > 0 {
            totalRuns := len(runs)
            completedRuns := 0
            for _, r := range runs {
                if r.Status == "success" {
                    completedRuns++
                }
                tp.DurationMs += int64(r.DurationMS)
            }
            // 进度 = 已完成 Run 数 / 总 Run 数
            if totalRuns > 0 {
                tp.ProgressPct = float64(completedRuns) / float64(totalRuns) * 100
            }
            // 当前步骤
            if teams[i].Status == "active" {
                tp.CurrentStep = fmt.Sprintf("执行中 (已完成 %d/%d 轮)", completedRuns, totalRuns)
            }
        }
        if teams[i].Status == "completed" {
            tp.ProgressPct = 100
            tp.CurrentStep = "已完成"
        }
        if teams[i].Status == "waiting_deps" {
            tp.ProgressPct = 0
            tp.CurrentStep = "等待依赖完成"
        }
        out = append(out, tp)
    }
    return out, nil
}
```

#### 3.1.2 实时进度事件

`spirit_team_progress` 事件当前只在 `scheduleDependentTeams` 和 `CancelTeam` 中发布。需在团队 Turn 完成时也发布：

```go
// team_turn_hooks.go executeTeamTurnViaHooks 中，RunTurnFromInput 完成后：
if sess.ParentSessionID != "" && strings.TrimSpace(sess.TeamID) != "" {
    // 发布进度更新事件
    progress := computeTeamProgress(ctx, o.team.TeamUC, sess.TeamID)
    env := event.NewEnvelope(event.EnvelopeTypeSpiritTeamProgress, "team-turn-hooks", sess.ParentSessionID)
    env.TeamID = strings.TrimSpace(sess.TeamID)
    env.Metadata = map[string]any{
        "team_id":      sess.TeamID,
        "progress_pct": progress.ProgressPct,
        "current_step": progress.CurrentStep,
    }
    o.td.Pipeline.Bus.Publish(ctx, env)
}
```

---

### 3.2 P1-02：团队列表排序未实现

**现状**：前端 `teams` 列表按添加顺序排列，未按状态排序。

**需求**：US-01 要求按状态排序：running → waiting → completed → failed。

**设计方案**：在前端 Store 中添加排序 computed。

```typescript
// stores/spirit/index.ts
const sortedTeams = computed(() => {
  const statusOrder: Record<string, number> = {
    running: 0, assembled: 0, assembling: 0,
    waiting_deps: 1,
    completed: 2,
    failed: 3,
    cancelled: 4,
  };
  return [...teams.value].sort((a, b) => {
    const orderA = statusOrder[a.status] ?? 99;
    const orderB = statusOrder[b.status] ?? 99;
    return orderA - orderB;
  });
});
```

---

### 3.3 P1-03：依赖图文本展示未实现

**现状**：US-04 要求"精灵回复中展示任务依赖图（文本形式）"，但当前 `assemble_team` 工具返回的 `AssembleTeamOutput` 不包含 DAG 文本表示。

**设计方案**：在 `TaskDAG` 上新增 `ToTextDiagram()` 方法，返回文本形式的依赖图。

#### 3.3.1 `TaskDAG.ToTextDiagram()`

```go
func (d *TaskDAG) ToTextDiagram() string {
    if d == nil || len(d.Nodes) == 0 {
        return ""
    }
    var sb strings.Builder
    sb.WriteString("📋 任务依赖图：\n")
    for _, node := range d.OrderedNodes() {
        prefix := "  "
        if len(node.DependsOn) == 0 {
            prefix = "▶ "
        } else {
            prefix = "⏳ "
        }
        sb.WriteString(fmt.Sprintf("%s%s: %s", prefix, node.ID, node.Description))
        if len(node.DependsOn) > 0 {
            sb.WriteString(fmt.Sprintf(" (依赖: %s)", strings.Join(taskNodeIDsToStrings(node.DependsOn), ", ")))
        }
        sb.WriteString("\n")
    }
    return sb.String()
}
```

#### 3.3.2 `assemble_team` 工具返回 DAG 文本

当 `TaskDAGJSON` 非空时，在 `AssembleTeamOutput` 中包含 DAG 文本：

```go
type AssembleTeamOutput struct {
    TeamID         string `json:"team_id"`
    SessionID      string `json:"session_id"`
    TeamName       string `json:"team_name"`
    TopologyReason string `json:"topology_reason,omitempty"`
    DAGDiagram     string `json:"dag_diagram,omitempty"`
}
```

在 `assembleDAGTeams` 返回时附带 DAG 文本。

---

### 3.4 P1-04：OrchestrationCache 持久化

**现状**：`OrchestrationCache` 使用内存 `map[string]*OrchestrationCacheEntry` 存储，服务重启后丢失。

**需求**：设计文档指定存储在 `AgentRuntimeSettings.ExtraJSON` 中 `orchestration_cache` 键。

**设计方案**：在 `OrchestrationCache` 初始化时从 `AgentRuntimeSettings` 加载，每次 `Put` 时持久化。

#### 3.4.1 新增 `OrchestrationCacheRepo` 接口

```go
// internal/biz/spirit_orchestration_cache.go
type OrchestrationCacheRepo interface {
    LoadCacheJSON(ctx context.Context) (string, error)
    SaveCacheJSON(ctx context.Context, jsonStr string) error
}
```

#### 3.4.2 Data 层实现

```go
// internal/data/orchestration_cache_repo.go
type orchestrationCacheRepo struct {
    data *Data
}

func (r *orchestrationCacheRepo) LoadCacheJSON(ctx context.Context) (string, error) {
    // 从 AgentRuntimeSettings.ExtraJSON 中读取 orchestration_cache 键
}

func (r *orchestrationCacheRepo) SaveCacheJSON(ctx context.Context, jsonStr string) error {
    // 写入 AgentRuntimeSettings.ExtraJSON 的 orchestration_cache 键
}
```

#### 3.4.3 OrchestrationCache 集成

```go
func NewOrchestrationCache(repo OrchestrationCacheRepo, lg loggateway.Logger) *OrchestrationCache {
    c := &OrchestrationCache{
        entries: make(map[string]*OrchestrationCacheEntry),
        repo:    repo,
        lg:      lg,
    }
    // 启动时加载
    ctx := context.Background()
    if jsonStr, err := repo.LoadCacheJSON(ctx); err == nil {
        c.LoadFromJSON(jsonStr)
    }
    return c
}

func (c *OrchestrationCache) Put(entry OrchestrationCacheEntry) {
    c.mu.Lock()
    defer c.mu.Unlock()
    entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
    c.entries[entry.TaskPattern] = &entry
    // 异步持久化
    go safego.Go(func() {
        if jsonStr, err := c.ToJSON(); err == nil {
            c.repo.SaveCacheJSON(context.Background(), jsonStr)
        }
    })
}
```

---

### 3.5 P1-05：DQ Score 三元分解

**现状**：`ComputeDQScore` 仅基于时间惩罚。

**需求**：设计文档要求 `DQ Score = Validity * 0.4 + Specificity * 0.3 + Correctness * 0.3`。

**设计方案**：

#### 3.5.1 DQ Score 三元分解

```go
type DQScoreBreakdown struct {
    Validity     float64 `json:"validity"`      // 结果有效性（0-1）
    Specificity  float64 `json:"specificity"`    // 结果具体性（0-1）
    Correctness  float64 `json:"correctness"`    // 结果正确性（0-1）
    Overall      float64 `json:"overall"`        // 加权总分
    DurationMs   int64   `json:"duration_ms"`    // 执行时长
}

func ComputeDQScoreBreakdown(teamResult TeamSynthesisResult, durationMs int64) DQScoreBreakdown {
    b := DQScoreBreakdown{DurationMs: durationMs}

    // Validity: 团队是否成功完成（二值判断）
    if teamResult.Status == "completed" {
        b.Validity = 1.0
    } else {
        b.Validity = 0.0
    }

    // Specificity: 结果摘要长度和结构化程度
    if teamResult.Summary != "" {
        b.Specificity = DQSpecificityBase   // 0.7
        if len(teamResult.Summary) > 100 {
            b.Specificity = DQSpecificityHigh  // 1.0
        } else if len(teamResult.Summary) > 50 {
            b.Specificity = DQSpecificityMedium // 0.85
        }
    }
    if teamResult.KeyFindings != "" {
        b.Specificity = min(b.Specificity+DQSpecificityFindings, 1.0) // +0.15
    }

    // Correctness: 基于时间效率的代理指标
    b.Correctness = 1.0
    if durationMs > 0 {
        timePenalty := float64(durationMs) / DQTimePenaltyDivisorMs // 60000ms
        if timePenalty > DQTimePenaltyMax {  // 5.0
            timePenalty = DQTimePenaltyMax
        }
        b.Correctness -= timePenalty * DQTimePenaltyFactor // 0.1
    }
    if b.Correctness < DQScoreMin {  // 0.1
        b.Correctness = DQScoreMin
    }

    return b
}
```

#### 3.5.2 数据来源

- **Validity**：基于团队最终状态（completed=1.0, 否则=0.0），二值判断
- **Specificity**：从团队 Summary 长度推断（>50 字符=0.85, >100 字符=1.0, 基准=0.7），KeyFindings 额外 +0.15
- **Correctness**：基于执行时长的代理指标（每分钟 -0.1，上限 5 分钟惩罚）

#### 3.5.3 调用入口

```go
// team_turn_hooks.go recordSpiritTeamCompletion 中：
dqBreakdown := biz.ComputeDQScoreBreakdown(biz.TeamSynthesisResult{...}, durationMs)
dqScore := dqBreakdown.Overall()
o.orchCache.RecordCompletion(ctx, taskPattern, topology, dqScore, 1, durationMs)
```

---

### 3.6 P1-06：编排优化建议生成

**现状**：DQ Score < 0.5 时没有生成优化建议。

**需求**：DQ Score < 0.5 时，进化系统生成编排优化建议。

**设计方案**：在 `recordSpiritTeamCompletion` 中，当 DQ Score < 0.5 时，调用 `EvolutionUsecase` 生成编排优化建议。

#### 3.6.1 复用 `EvolutionSuggestionRepo`

代码中未实现专用的 `EvolutionSuggestionPort`，而是复用通用的 `EvolutionSuggestionRepo`（`internal/biz/evolution.go`），通过 `WithEvolutionSuggestionRepo` 选项注入 `SpiritTeamUsecase`：

```go
// internal/biz/evolution.go 中已有接口
type EvolutionSuggestionRepo interface {
    ListByAgent(ctx context.Context, agentKey string) ([]EvolutionSuggestion, error)
    GetByID(ctx context.Context, id string) (EvolutionSuggestion, error)
    Create(ctx context.Context, suggestion EvolutionSuggestion) error
    UpdateStatus(ctx context.Context, id string, status string) error
}
```

`SpiritTeamUsecase` 在 `RecordTeamCompletion` 中直接调用 `u.evolutionSugg.Create()` 生成编排优化建议。拓扑替代建议由 `OrchestrationCache.SuggestBestAlternativeTopology()` 承担。

#### 3.6.2 EvolutionUsecase 实现

```go
func (u *EvolutionUsecase) CreateOrchestrationSuggestion(ctx context.Context, agentKey string, taskPattern string, currentTopology TopologyType, dqScore float64) error {
    // 生成拓扑优化建议
    suggestedTopology := u.suggestAlternativeTopology(currentTopology, dqScore)
    suggestion := EvolutionSuggestion{
        AgentKey:  agentKey,
        Type:      "orchestration",
        Title:     fmt.Sprintf("编排优化建议: %s → %s (DQ=%.2f)", currentTopology, suggestedTopology, dqScore),
        Content:   fmt.Sprintf("任务模式 '%s' 使用 %s 拓扑的 DQ Score 为 %.2f，建议尝试 %s 拓扑", taskPattern, currentTopology, dqScore, suggestedTopology),
        Status:    "pending",
    }
    return u.suggestionRepo.Create(ctx, suggestion)
}
```

#### 3.6.3 拓扑替代建议

```go
func (u *EvolutionUsecase) suggestAlternativeTopology(current TopologyType, dqScore float64) TopologyType {
    // 简单策略：按拓扑质量排序推荐
    alternatives := []TopologyType{
        TopologyCoordinator,
        TopologyHybrid,
        TopologyParallel,
        TopologySequential,
    }
    for _, alt := range alternatives {
        if alt != current {
            return alt
        }
    }
    return current
}
```

#### 3.6.4 进化护栏接入

```go
// 在 CreateOrchestrationSuggestion 中检查护栏
func (u *EvolutionUsecase) checkGuardrails(agent Agent, dqScore float64) bool {
    if agent.GuardrailMaxChangePerPeriod > 0 {
        // 检查本周期内已生成的建议数量
        recentCount := u.suggestionRepo.CountRecent(ctx, agent.Key, 24*time.Hour)
        maxChanges := int(float64(recentCount) * agent.GuardrailMaxChangePerPeriod)
        if recentCount >= maxChanges && maxChanges > 0 {
            return false // 护栏阻止
        }
    }
    // DQ Score < 0.3 时触发回滚
    if dqScore < 0.3 {
        u.lg.Warn("DQ Score 严重偏低，建议回滚到上一个稳定拓扑",
            loggateway.StepID("spirit.evolution.rollback"),
            loggateway.Float64("dq_score", dqScore),
        )
    }
    return true
}
```

---

### 3.7 P1-07：RouteTopology 算法完善

**现状**：`RouteTopology` 是简化版，`computeDepth` 和 `computeWidth` 未实现。

**设计方案**：实现完整的拓扑路由算法，对齐 AdaptOrch 论文。

```go
func (d *TaskDAG) RouteTopology() TopologyType {
    if len(d.Nodes) == 0 {
        return TopologyCoordinator
    }
    if len(d.Roots) == len(d.Nodes) {
        return TopologyParallel
    }
    // 计算深度和宽度
    depth := d.computeDepth()
    width := d.computeMaxWidth()

    // 依赖链深度 > 3 → coordinator
    if depth > 3 {
        return TopologyCoordinator
    }
    // 有依赖且宽度 > 1 → hybrid
    if width > 1 {
        return TopologyHybrid
    }
    // 有依赖但宽度 = 1 → sequential
    return TopologySequential
}

func (d *TaskDAG) computeDepth() int {
    depthMap := make(map[TaskNodeID]int, len(d.Nodes))
    var calcDepth func(id TaskNodeID) int
    calcDepth = func(id TaskNodeID) int {
        if dep, ok := depthMap[id]; ok {
            return dep
        }
        node := d.Nodes[id]
        if node == nil {
            return 0
        }
        maxDep := 0
        for _, depID := range node.DependsOn {
            if dep := calcDepth(depID); dep > maxDep {
                maxDep = dep
            }
        }
        depthMap[id] = maxDep + 1
        return depthMap[id]
    }
    maxDepth := 0
    for id := range d.Nodes {
        if dep := calcDepth(id); dep > maxDepth {
            maxDepth = dep
        }
    }
    return maxDepth
}

func (d *TaskDAG) computeMaxWidth() int {
    levelMap := make(map[int]int)
    for id := range d.Nodes {
        level := d.computeNodeLevel(id)
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

func (d *TaskDAG) computeNodeLevel(id TaskNodeID) int {
    node := d.Nodes[id]
    if node == nil || len(node.DependsOn) == 0 {
        return 0
    }
    maxLevel := 0
    for _, depID := range node.DependsOn {
        if level := d.computeNodeLevel(depID); level > maxLevel {
            maxLevel = level
        }
    }
    return maxLevel + 1
}
```

---

## 四、P2 问题详细设计

### 4.1 P2-01：团队超时未实现

**设计方案**：在 `AssembleTeam` 创建团队后，注册超时回调。

```go
func (u *SpiritTeamUsecase) AssembleTeam(ctx context.Context, params SpiritTeamParams) (SpiritTeamResult, error) {
    // ... 创建团队和 session ...

    // 注册超时回调
    cfg := u.resolveParallelConfig(ctx, params.SpiritSessionID)
    if cfg.TeamTimeoutSeconds > 0 {
        time.AfterFunc(cfg.TeamTimeout(), func() {
            safego.Go(func() {
                team, err := u.teamUC.Get(context.Background(), team.ID)
                if err != nil || team.Status == "completed" || team.Status == "failed" {
                    return
                }
                u.teamUC.Update(context.Background(), team.ID, Team{Status: "failed"})
                // 发布超时事件
            })
        })
    }

    return SpiritTeamResult{Team: team, Session: teamSession}, nil
}
```

### 4.2 P2-02：自动归档未实现

**设计方案**：团队完成超过 `AutoArchiveAfter` 后自动归档。

```go
// 在 CheckTeamProgress 或定时任务中检查
func (u *SpiritTeamUsecase) AutoArchiveCompleted(ctx context.Context, spiritSessionID string) {
    cfg := u.resolveParallelConfig(ctx, spiritSessionID)
    teams, _ := u.ListCompletedTeams(ctx, spiritSessionID)
    for _, t := range teams {
        if time.Since(t.UpdatedAt) > cfg.AutoArchiveAfter() {
            u.teamUC.Update(ctx, t.ID, Team{Status: "archived"})
        }
    }
}
```

### 4.3 P2-03：Session 树深度限制未实现

**设计方案**：在 `AssembleTeam` 创建 team session 时检查深度。

```go
func (u *SpiritTeamUsecase) AssembleTeam(ctx context.Context, params SpiritTeamParams) (SpiritTeamResult, error) {
    // ... 创建团队 ...

    cfg := u.resolveParallelConfig(ctx, params.SpiritSessionID)
    // 检查 Session 树深度
    parentSession, err := u.sessionUC.Get(ctx, spiritSessionID)
    if err != nil {
        return SpiritTeamResult{}, err
    }
    if parentSession.AgentDepth >= cfg.MaxSessionDepth {
        return SpiritTeamResult{}, kerrors.BadRequest("SPIRIT",
            fmt.Sprintf("session tree depth (%d) exceeds max (%d)", parentSession.AgentDepth, cfg.MaxSessionDepth))
    }

    teamSession, err := u.sessionUC.Create(ctx, Session{
        AgentDepth: parentSession.AgentDepth + 1,
        // ...
    })
}
```

---

## 五、前端深度实现设计

### 5.1 前端差距总览

| 组件 | 差距 | 优先级 |
|------|------|--------|
| `ParallelTeamOverview.vue` | 团队列表未排序 | P1 |
| `TeamProgressCard.vue` | 进度条只有 0%/100%；无耗时显示 | P1 |
| `SynthesisResultCard.vue` | teamResults 数据源不完整 | P0 |
| Store `handleSpiritEnvelope` | `spirit_team_progress` 缺少 progress_pct 更新 | P1 |
| 依赖图可视化 | 未实现文本形式 DAG 展示 | P1 |
| 并行配额实时更新 | 取消团队后配额未实时更新 | P2 |

### 5.2 Store 增强

#### 5.2.1 进度实时更新

```typescript
case "spirit_team_progress":
    if (teamId) {
        const pct = Number(md.progress_pct ?? 0);
        const step = String(md.current_step ?? "");
        updateTeamStatus(teamId, String(md.status ?? "running"));
        // 更新进度
        const team = teams.value.find(t => t.id === teamId);
        if (team && pct >= 0) {
            team.completedSteps = Math.round(pct * team.totalSteps / 100);
        }
    }
    break;
```

#### 5.2.2 团队列表排序

```typescript
const sortedTeams = computed(() => {
    const statusOrder: Record<string, number> = {
        assembling: 0, assembled: 0, running: 0,
        waiting_deps: 1,
        completed: 2,
        failed: 3,
        cancelled: 4,
    };
    return [...teams.value].sort((a, b) => (statusOrder[a.status] ?? 99) - (statusOrder[b.status] ?? 99));
});
```

### 5.3 TeamProgressCard 增强

#### 5.3.1 耗时显示

```vue
<div v-if="durationText" class="team-progress-card__duration text-caption text-grey-6">
    ⏱ {{ durationText }}
</div>
```

```typescript
const durationText = computed(() => {
    if (!props.team.durationMs) return "";
    const seconds = Math.floor(props.team.durationMs / 1000);
    if (seconds < 60) return `${seconds}s`;
    const minutes = Math.floor(seconds / 60);
    return `${minutes}m ${seconds % 60}s`;
});
```

需要在 `SpiritTeam` 类型中新增 `durationMs` 字段，并在 WS 事件中传递。

### 5.4 DAG 文本展示组件

新增 `DAGDiagramCard.vue`：

```vue
<template>
  <div v-if="diagram" class="dag-diagram-card">
    <div class="dag-diagram-card__title">任务依赖图</div>
    <pre class="dag-diagram-card__content">{{ diagram }}</pre>
  </div>
</template>
```

在 `ParallelTeamOverview.vue` 中，当存在 DAG 团队时显示。

---

## 六、实现优先级与任务拆分

### 6.1 Phase 3 — 深度业务实现

| 排序 | ID | 任务 | 差距级别 | 影响域 |
|------|-----|------|---------|--------|
| 1 | SPO-DP-01 | `TeamStarterPort` 接口 + `ChatOrchestrator` 实现 + Wire 绑定 | P0 | biz + service + wire |
| 2 | SPO-DP-02 | `start_team` 工具 + `assemble_team` 自动启动 | P0 | tools + service |
| 3 | SPO-DP-03 | `scheduleDependentTeams` 激活后自动启动 | P0 | service/team_turn_hooks |
| 4 | SPO-DP-04 | `ExtractTeamOutput` + Summary/KeyFindings 提取 | P0 | biz + service |
| 5 | SPO-DP-05 | 部分失败合成 + 级联标注 | P0 | biz + service |
| 6 | SPO-DP-06 | 删除 `DependencyScheduler` 死代码 | P0 | biz |
| 7 | SPO-DP-07 | 进度中间值计算 + CurrentStep | P1 | biz |
| 8 | SPO-DP-08 | 实时进度事件（Turn 完成时发布） | P1 | service |
| 9 | SPO-DP-09 | 前端团队列表排序 + 进度更新 | P1 | frontend |
| 10 | SPO-DP-10 | `TaskDAG.ToTextDiagram()` + DAG 文本展示 | P1 | biz + tools + frontend |
| 11 | SPO-DP-11 | `OrchestrationCache` 持久化 | P1 | biz + data |
| 12 | SPO-DP-12 | DQ Score 三元分解 | P1 | biz |
| 13 | SPO-DP-13 | 编排优化建议生成 + 进化护栏接入 | P1 | biz |
| 14 | SPO-DP-14 | `RouteTopology` 算法完善 | P1 | biz |
| 15 | SPO-DP-15 | 团队超时实现 | P2 | biz |
| 16 | SPO-DP-16 | 自动归档实现 | P2 | biz |
| 17 | SPO-DP-17 | Session 树深度限制 | P2 | biz |
| 18 | SPO-DP-18 | 前端 TeamProgressCard 耗时显示 | P2 | frontend |

### 6.2 依赖关系

```
SPO-DP-01 → SPO-DP-02 → SPO-DP-03 (TeamStarterPort → start_team → 依赖调度自动启动)
SPO-DP-04 → SPO-DP-05 (ExtractTeamOutput → 部分失败合成)
SPO-DP-07 → SPO-DP-08 (进度计算 → 实时事件)
SPO-DP-12 → SPO-DP-13 (DQ Score V2 → 优化建议)
SPO-DP-11 (缓存持久化独立)
SPO-DP-06 (死代码清理独立)
```

---

## 七、风险与缓解

| 风险 | 缓解 |
|------|------|
| `TeamStarterPort` 循环依赖 | 接口定义在 biz 层，ChatOrchestrator 在 service 层实现，Wire 绑定 |
| `StartTeamTurn` 异步启动失败 | 使用 `safego.Go` + 失败时发布 `spirit_team_failed` 事件 |
| DQ Score 三元分解数据不完整 | Validity 基于工具调用记录（已有），Specificity 基于内容长度（代理），Correctness 基于时间（代理） |
| OrchestrationCache 持久化性能 | 异步写入，不阻塞主流程 |
| 团队超时 `time.AfterFunc` 内存泄漏 | 使用 `sync.Map` 注册/取消超时回调，团队完成时取消 |
| 前端 WS 消息风暴 | 进度事件节流（500ms），按团队分组 |

---

## 八、验证计划

### 8.1 P0 验证

| 验证项 | 方法 |
|--------|------|
| 团队创建后自动启动 | 精灵对话中调用 `assemble_team`，验证团队 Runner 自动执行 |
| 依赖团队激活后自动启动 | DAG 场景中前置团队完成后，验证依赖团队自动开始执行 |
| SynthesisResult 包含 Summary | 调用 `synthesize_results`，验证返回结果包含非空 Summary |
| 部分失败合成 | 模拟部分团队失败，验证合成结果包含失败标注 |

### 8.2 P1 验证

| 验证项 | 方法 |
|--------|------|
| 进度中间值 | 团队执行中调用 `check_team_progress`，验证进度在 0-100% 之间 |
| 团队列表排序 | 前端验证团队按 running → waiting → completed → failed 排序 |
| DAG 文本展示 | 前端验证 DAG 团队展示文本依赖图 |
| 缓存持久化 | 重启服务后验证编排缓存仍存在 |
| DQ Score 三元分解 | 验证 DQ Score 包含 Validity/Specificity/Correctness 分项 |
| 优化建议生成 | DQ Score < 0.5 时验证生成 pending 建议 |

### 8.3 全量验证

```bash
# 后端
make api && make wire && make build && make test && make lint

# 前端
cd web && pnpm lint && pnpm test && pnpm build
```

---

## 九、Phase 4 差距与详细实现 — 智能增强

> 基于 AI Agent 工作模式启发，补充 4 个关键增强的详细实现设计。

### 9.1 Phase 4 差距矩阵

| 验收 ID | 摘要 | 骨架状态 | 差距级别 | 差距描述 |
|---------|------|---------|---------|---------|
| SPO-10 | 任务复杂度智能评估 | ❌ 不存在 | **P0** | 无形式化复杂度评估机制，Spirit 路由决策依赖 LLM 软约束 |
| SPO-11 | Graph DAG 动态编排 | ❌ 不存在 | **P1** | 编排管家只有线性工具调用，无法利用 Graph 并行/条件/检查点能力 |
| SPO-12 | 编排验证门禁节点 | ❌ 不存在 | **P1** | Graph 无自动化验证门禁，质量检查依赖最终合成 |

### 9.2 SPO-10 详细实现：TaskComplexityClassifier

#### 9.2.1 `assess_complexity` 工具实现

**实现位置**：`internal/tools/spirit/assess_complexity.go`

```go
type assessComplexityTool struct {
    rules *ComplexityRuleEngine
}

func (t *assessComplexityTool) Call(ctx context.Context, input AssessComplexityInput) (*AssessComplexityOutput, error) {
    level := t.rules.Assess(input.UserMessage)
    path := levelToPath(level)
    return &AssessComplexityOutput{
        Level:         string(level),
        Reasoning:     t.rules.LastReasoning(),
        SuggestedPath: path,
    }, nil
}

func levelToPath(level ComplexityLevel) string {
    switch level {
    case ComplexitySimple:
        return "direct_answer"
    case ComplexityModerate:
        return "single_butler"
    case ComplexityComplex:
        return "orchestrator"
    default:
        return "single_butler"
    }
}
```

#### 9.2.2 规则引擎实现

**实现位置**：`internal/tools/spirit/complexity_rules.go`

```go
type ComplexityLevel string

const (
    ComplexitySimple   ComplexityLevel = "simple"
    ComplexityModerate ComplexityLevel = "moderate"
    ComplexityComplex  ComplexityLevel = "complex"
)

type ComplexityRuleEngine struct {
    simplePatterns    []string
    complexIndicators []string
    lastReasoning     string
    mu                sync.Mutex
}

func NewComplexityRuleEngine() *ComplexityRuleEngine {
    return &ComplexityRuleEngine{
        simplePatterns: []string{
            "什么是", "解释一下", "帮我看看", "怎么用",
            "是什么意思", "告诉我", "列出", "显示",
            "what is", "explain", "show me", "how to use",
        },
        complexIndicators: []string{
            "分析", "对比", "编写", "设计", "规划", "编排",
            "多个", "跨行业", "团队", "协作", "流程",
            "analyze", "compare", "design", "plan", "orchestrate",
        },
    }
}

func (r *ComplexityRuleEngine) Assess(message string) ComplexityLevel {
    r.mu.Lock()
    defer r.mu.Unlock()

    lower := strings.ToLower(message)

    for _, p := range r.simplePatterns {
        if strings.Contains(lower, strings.ToLower(p)) {
            r.lastReasoning = fmt.Sprintf("匹配简单问答模式: %s", p)
            return ComplexitySimple
        }
    }

    complexHits := 0
    for _, p := range r.complexIndicators {
        if strings.Contains(lower, strings.ToLower(p)) {
            complexHits++
        }
    }
    if complexHits >= 2 {
        r.lastReasoning = fmt.Sprintf("匹配 %d 个复杂任务指标", complexHits)
        return ComplexityComplex
    }
    if complexHits == 1 {
        r.lastReasoning = "匹配 1 个复杂任务指标，但不足以确定，降级为 moderate"
        return ComplexityModerate
    }

    r.lastReasoning = "无法通过规则确定复杂度，使用安全默认值 moderate"
    return ComplexityModerate
}

func (r *ComplexityRuleEngine) LastReasoning() string {
    r.mu.Lock()
    defer r.mu.Unlock()
    return r.lastReasoning
}
```

#### 9.2.3 工具注册

**实现位置**：`internal/tools/spirit_tools.go` 中 `NewSpiritTools` 函数追加：

```go
tools = append(tools,
    NewAssessComplexityTool(NewComplexityRuleEngine()),
)
```

#### 9.2.4 Spirit Prompt 更新

**实现位置**：`internal/scenario/system/prompts/spirit.md`

在现有 Prompt 末尾追加：

```
## 决策规则（强制）
1. 收到用户消息后，先调用 assess_complexity 评估复杂度
2. 根据评估结果路由：
   - simple → 直接回答，不委派
   - moderate → 委派给最相关的单一管家
   - complex → 委派给 __orchestrator__
3. 禁止跳过 assess_complexity 直接委派
4. 禁止对 simple 级别任务委派给管家
```

### 9.3 SPO-11 详细实现：GraphOrchestration

#### 9.3.1 `build_orchestration_graph` 工具实现

**实现位置**：`internal/tools/orchestrator/build_graph.go`

```go
// GraphBuilderPort 定义 Graph 执行接口，替代原先的 OrchestratorGraphDeps 结构体。
// 接口注入模式比函数字段注入更利于测试和 Wire 绑定。
type GraphBuilderPort interface {
    BuildAndExecute(ctx context.Context, config biz.GraphBuildConfig, sessionID string) (executionID string, err error)
}

// NewBuildOrchestrationGraphTool 创建工具。
// 如果 builder 为 nil，工具仅生成 Graph 配置而不执行。
func NewBuildOrchestrationGraphTool(builder GraphBuilderPort) *trpcfunction.FunctionTool[BuildOrchestrationGraphInput, BuildOrchestrationGraphOutput] {
    return trpcfunction.NewFunctionTool(
        func(ctx context.Context, input BuildOrchestrationGraphInput) (BuildOrchestrationGraphOutput, error) {
            config := BuildGraphConfig(input)
            verificationNodes := injectVerificationNodes(&config, input.Mode)
            out := BuildOrchestrationGraphOutput{
                GraphBuildConfig:  config,
                NodeCount:         len(config.Nodes),
                EdgeCount:         len(config.Edges),
                VerificationNodes: verificationNodes,
            }
            if builder != nil {
                execID, err := builder.BuildAndExecute(ctx, config, "")
                if err != nil { return BuildOrchestrationGraphOutput{}, err }
                out.GraphExecutionID = execID
            }
            return out, nil
        },
        // ...
    )
}
```

#### 9.3.2 Graph 拓扑生成关键逻辑

`buildGraphConfig` 方法根据 `AgentAssignment.DependsOn` 自动生成 DAG：
- 无依赖的 Agent → 从 entry_point 直连（可并行）
- 有依赖的 Agent → 从依赖 Agent 连边
- 所有 Agent 完成后 → JoinEdge 到 merge_results
- merge_results → verify_results（验证门禁节点）

#### 9.3.3 与 `assemble_team` 共存策略

**P0 阶段**：两个工具共存，编排管家 Prompt 中增加决策规则：

```
## 编排方式选择
- 简单任务（2-3 Agent，顺序执行）→ 使用 assemble_team
- 复杂任务（4+ Agent，有并行/条件路由）→ 使用 build_orchestration_graph
```

**P1 阶段**：`assemble_team` 内部重构为调用 `build_orchestration_graph`。

#### 9.3.4 依赖注入路径

`GraphBuilderPort` 接口通过 `ChatOrchestratorDeps` 获取实现：

```go
func (o *ChatOrchestrator) orchestratorTools(ctx context.Context, ag biz.Agent) []trpctool.Tool {
    if ag.AgentKey != "__orchestrator__" { return nil }
    return orchestrator.RegisterAll(orchestrator.Deps{
        // ... 现有依赖
        GraphBuilder: orchestrator.GraphBuilderPort(/* 实现者 */),
    })
}
```

### 9.4 SPO-12 详细实现：VerificationGate

#### 9.4.1 验证节点类型定义

**实现位置**：`internal/tools/orchestrator/verification.go`

```go
type VerificationType string

const (
    VerifyTypeOutputFormat   VerificationType = "output_format"
    VerifyTypeTaskCompletion VerificationType = "task_completion"
    VerifyTypeHumanApproval  VerificationType = "human_approval"
)

type VerificationConfig struct {
    Type          VerificationType
    AfterNode     string // Insert verification after this node
    FailureAction string // "skip" | "retry_then_block" | "interrupt_before"
}
```

#### 9.4.2 验证函数实现

**实现位置**：`internal/tools/orchestrator/verify_funcs.go`

```go
func verifyOutputFormat(ctx context.Context, state graph.State) (any, error) {
    results, ok := state["agent_results"]
    if !ok { return nil, fmt.Errorf("no agent results found in state") }

    agentResults, ok := results.(map[string]any)
    if !ok { return nil, fmt.Errorf("agent_results is not a map") }

    var issues []string
    for agentKey, result := range agentResults {
        resultStr, ok := result.(string)
        if !ok || resultStr == "" {
            issues = append(issues, fmt.Sprintf("agent %s returned empty result", agentKey))
        }
    }

    if len(issues) > 0 {
        return map[string]any{"verified": false, "issues": issues}, nil
    }
    return map[string]any{"verified": true}, nil
}

func verifyTaskCompletion(ctx context.Context, state graph.State) (any, error) {
    results, ok := state["agent_results"]
    if !ok { return map[string]any{"verified": false, "reason": "no results"}, nil }

    agentResults, ok := results.(map[string]any)
    if !ok { return map[string]any{"verified": false, "reason": "invalid results"}, nil }

    completedCount := 0
    for _, result := range agentResults {
        if resultStr, ok := result.(string); ok && resultStr != "" {
            completedCount++
        }
    }

    completionRate := float64(completedCount) / float64(len(agentResults))
    if completionRate >= 0.8 {
        return map[string]any{"verified": true, "completion_rate": completionRate}, nil
    }
    return map[string]any{
        "verified":        false,
        "completion_rate": completionRate,
        "reason":          fmt.Sprintf("only %.0f%% agents completed", completionRate*100),
    }, nil
}
```

#### 9.4.3 验证节点注入到 Graph

在 `BuildGraphConfig` 后通过 `injectVerificationNodes` 独立注入验证节点：

```go
// injectVerificationNodes 在 Graph 配置中插入验证节点。
// 找到所有指向 AfterNode 的边，在源节点和 AfterNode 之间插入验证节点。
// 返回添加的验证节点 ID 列表。
func injectVerificationNodes(config *biz.GraphBuildConfig, mode string) []string {
    configs := DefaultVerificationConfigs(mode)
    var nodeIDs []string

    for i, vc := range configs {
        nodeID := fmt.Sprintf("verify_%s_%d", vc.Type, i)

        vNode := biz.NodeDef{
            ID:            nodeID,
            Type:          biz.NodeTypeFunction,
            Description:   fmt.Sprintf("Verification gate: %s", vc.Type),
            FailureAction: vc.FailureAction,
        }

        if vc.Type == VerifyTypeHumanApproval {
            vNode.InterruptBefore = true
        }

        // 重写边：source → verify_node → AfterNode
        var newEdges []biz.EdgeDef
        for _, edge := range config.Edges {
            if edge.To == vc.AfterNode {
                newEdges = append(newEdges, biz.EdgeDef{From: edge.From, To: nodeID})
            } else {
                newEdges = append(newEdges, edge)
            }
        }
        newEdges = append(newEdges, biz.EdgeDef{From: nodeID, To: vc.AfterNode})

        config.Edges = newEdges
        config.Nodes = append(config.Nodes, vNode)
        nodeIDs = append(nodeIDs, nodeID)
    }

    return nodeIDs
}
```

**默认验证配置**（`DefaultVerificationConfigs`）：

| 模式 | 验证节点 | FailureAction |
|------|---------|---------------|
| parallel/hybrid | output_format (merge 后) | Skip |
| parallel/hybrid | task_completion (merge 后) | RetryThenBlock |
| coordinator | output_format (merge 后) | Skip |

### 9.5 GAP-3 详细实现：AdaptiveTeamMode

#### 9.5.1 Spirit Team 构建逻辑

**实现位置**：`internal/service/chat_orchestrator_spirit.go`（新增文件）

```go
type SpiritTeamMode string

const (
    SpiritModeCoordinator SpiritTeamMode = "coordinator"
    SpiritModeSwarm       SpiritTeamMode = "swarm"
    SpiritModeDirect      SpiritTeamMode = "direct"
)

func (o *ChatOrchestrator) selectSpiritMode(
    complexityLevel string,
) SpiritTeamMode {
    switch complexityLevel {
    case "simple":
        return SpiritModeDirect
    case "moderate":
        return SpiritModeDirect
    case "complex":
        return SpiritModeCoordinator
    default:
        return SpiritModeCoordinator
    }
}

func (o *ChatOrchestrator) buildSpiritTeam(
    ctx context.Context, spiritAg biz.Agent, deps chatagent.TRPCBuilderDeps,
    mode SpiritTeamMode,
) (agent.Agent, error) {
    spiritAgent, err := chatagent.BuildTRPCAgentCached(ctx, spiritAg, deps)
    if err != nil { return nil, err }

    if mode == SpiritModeDirect {
        return spiritAgent, nil
    }

    butlers, err := o.loadSystemButlers(ctx, deps)
    if err != nil { return nil, err }

    switch mode {
    case SpiritModeCoordinator:
        return trpcteam.New(spiritAgent, butlers)
    case SpiritModeSwarm:
        return trpcteam.NewSwarm(
            "spirit_swarm", spiritAgent.Info().Name,
            append([]agent.Agent{spiritAgent}, butlers...),
        )
    default:
        return trpcteam.New(spiritAgent, butlers)
    }
}

func (o *ChatOrchestrator) loadSystemButlers(
    ctx context.Context, deps chatagent.TRPCBuilderDeps,
) ([]agent.Agent, error) {
    var butlerKeys = []string{"__orchestrator__", "__system_admin__", "__memory__", "__skills__", "__monitor__"}
    var members []agent.Agent
    for _, key := range butlerKeys {
        b, err := o.td.Catalog.Agents.GetAgentByAgentKey(ctx, key)
        if err != nil { continue }
        memberDeps := deps
        memberDeps.CustomTools = o.systemBuiltinTools(ctx, b)
        member, err := chatagent.BuildTRPCAgentCached(ctx, b, memberDeps)
        if err != nil { continue }
        members = append(members, member)
    }
    return members, nil
}
```

#### 9.5.2 与 `runSingleAgentViaTRPC` 集成

**修改位置**：`internal/service/chat_orchestrator_turn.go`

```go
// 在 __spirit__ 分支中：
if ag.AgentKey == "__spirit__" {
    mode := o.selectSpiritMode(complexityLevel) // 从 assess_complexity 结果获取
    root, err = o.buildSpiritTeam(ctx, ag, deps, mode)
} else {
    root, err = chatagent.BuildTRPCAgentCached(ctx, ag, deps)
}
```

### 9.6 Phase 4 实现优先级

| 排序 | ID | 任务 | 差距级别 | 影响域 |
|------|-----|------|---------|--------|
| 1 | SPO-P4-01 | `ComplexityRuleEngine` + `assess_complexity` 工具 | P0 | tools/spirit |
| 2 | SPO-P4-02 | Spirit Prompt 强制决策规则 | P0 | scenario/prompts |
| 3 | SPO-P4-03 | `chat_orchestrator_spirit.go` + Team 模式选择 | P0 | service |
| 4 | SPO-P4-04 | `runSingleAgentViaTRPC` 集成 Spirit 模式选择 | P0 | service |
| 5 | SPO-P4-05 | `build_orchestration_graph` 工具 | P1 | tools/orchestrator |
| 6 | SPO-P4-06 | `buildGraphConfig` DAG 生成逻辑 | P1 | tools/orchestrator |
| 7 | SPO-P4-07 | 验证节点类型定义 + 验证函数 | P1 | tools/orchestrator |
| 8 | SPO-P4-08 | 验证节点注入到 Graph | P1 | tools/orchestrator |
| 9 | SPO-P4-09 | `OrchestratorGraphDeps` 依赖注入 | P1 | service |
| 10 | SPO-P4-10 | 编排管家 Prompt Graph 编排决策规则 | P1 | scenario/prompts |

### 9.7 依赖关系

```
SPO-P4-01 → SPO-P4-02 (规则引擎 → Prompt 规则)
SPO-P4-01 → SPO-P4-03 (assess_complexity → Team 模式选择)
SPO-P4-03 → SPO-P4-04 (buildSpiritTeam → runSingleAgentViaTRPC 集成)
SPO-P4-05 → SPO-P4-06 (工具定义 → DAG 生成逻辑)
SPO-P4-06 → SPO-P4-07 (DAG 生成 → 验证节点)
SPO-P4-07 → SPO-P4-08 (验证函数 → 注入逻辑)
SPO-P4-05 → SPO-P4-09 (工具 → 依赖注入)
SPO-P4-06 → SPO-P4-10 (DAG 生成 → Prompt 规则)
```

### 9.8 风险与缓解

| 风险 | 缓解 |
|------|------|
| `assess_complexity` 规则引擎覆盖不全 | P0 使用安全默认值 moderate；P1 引入历史数据优化 |
| `build_orchestration_graph` 生成的 DAG 不合理 | P0 保留 assemble_team 作为回退；P1 增加模板缓存 |
| Graph 验证节点增加执行时间 | 验证节点使用 FailureAction=Skip，验证失败不阻塞 |
| Spirit Team 模式选择错误 | P0 默认 Coordinator（最安全）；P1 基于成功率自动优化 |
| `OrchestratorGraphDeps` 循环依赖 | 接口定义在 biz 层，实现注入在 service 层 |

### 9.9 验证计划

#### 9.9.1 P0 验证

| 验证项 | 方法 |
|--------|------|
| assess_complexity 规则引擎 | 单元测试覆盖 simple/moderate/complex 三级 |
| Spirit 强制决策规则 | 精灵对话中验证先调用 assess_complexity 再路由 |
| Team 模式选择 | 验证 simple→Direct, moderate→Direct, complex→Coordinator |

#### 9.9.2 P1 验证

| 验证项 | 方法 |
|--------|------|
| Graph DAG 生成 | 验证并行/串行/混合拓扑正确性 |
| 验证节点注入 | 验证 output_format/task_completion/human_approval 三种类型 |
| 验证函数逻辑 | 验证空结果检测、完成度计算 |
| HITL 中断 | 验证 human_approval 验证节点触发 interrupt |

#### 9.9.3 全量验证

```bash
make api && make wire && make build && make test && make lint
cd web && pnpm lint && pnpm test && pnpm build
```

---

## 十、深度架构审查修复记录

> 2026-06-06：对 Spirit Team 全链路进行深度架构审查，发现并修复 7 个严重问题 + 5 个中等问题 + 3 个轻微问题。

### 10.1 严重问题修复

| ID | 问题 | 修复方案 | 影响文件 |
|----|------|----------|----------|
| S3 | OrchestrationCache.ToJSON() 递归 RLock 导致死锁 | 提取 `listLocked()` 内部方法，`ToJSON()` 和 `List()` 共用 | `spirit_orchestration_cache.go` |
| S4 | 超时回调仅转换状态，不触发依赖调度/事件发布/AllDone 检查 | 新增 `TimeoutHandler` 接口（biz 层），`TeamStarter` 实现，`BeforeStart` 阶段注入 | `spirit_team_usecase.go`, `spirit_team.go`, `app.go` |
| S5 | `interrupted` 状态被 `CheckAllTeamsCompleted` 错误视为终态 | switch 增加 `TeamStatusInterrupted` case；`IsTeamStatusActive` 同步增加 | `spirit_team_usecase.go`, `team_types.go` |
| FS1 | 前后端 SpiritTeamMode 枚举不一致 | 对齐为两套独立系统：`SpiritTeamMode`（精灵路由层：coordinator/swarm/direct）+ `TeamMode*`（团队定义层：sequential/parallel/coordinator/critic_loop/swarm/adaptive）。前端 `SpiritTeamMode` 移除 `direct`（精灵路由层内部使用，不暴露给前端），`TeamMode*` 对齐为 6 值 | `types.ts`, `TeamTaskCard.vue`, `TeamAssemblyCard.vue`, `TeamProgressCard.vue` |
| FS2 | 前后端 SpiritTeamStatus 枚举不一致 | 对齐为 `pending/running/completed/failed/cancelled/interrupted/archived` | `types.ts`, `stores/spirit/index.ts` |
| FS3 | SynthesisResultCard 使用 v-html 渲染未净化内容 | 替换为 `renderChatMarkdown()`（已通过安全审计） | `SynthesisResultCard.vue` |
| FS4 | cancelTeam 成功后从列表移除团队 | 改为 `updateTeamStatus(teamId, 'cancelled')`，与后端行为一致 | `stores/spirit/index.ts` |

### 10.2 中等问题修复

| ID | 问题 | 修复方案 | 影响文件 |
|----|------|----------|----------|
| M11 | HandleTeamTurnResult failed/cancelled 路径不取消超时定时器 | 入口统一调用 `CancelTimeoutTimer` | `spirit_team.go` |
| M13 | BuildGraphConfig 无循环检测和依赖验证 | DFS 三色标记法循环检测 + 悬空依赖跳过 + 环时降级顺序链 | `build_graph.go` |
| M8 | 前端 spirit_team_progress status 来源混用导致状态回退 | 增加状态转换合法性校验，禁止 running→pending 回退 | `stores/spirit/index.ts` |
| M6 | mode 默认值 `??` 不覆盖空字符串 | 改为 `\|\|` 运算符 | `stores/spirit/index.ts` |
| M7 | synthesizedAt 显示原始 ISO 时间戳 | 使用 `toLocaleString()` 格式化 | `SynthesisResultCard.vue` |

### 10.3 轻微问题修复

| ID | 问题 | 修复方案 | 影响文件 |
|----|------|----------|----------|
| L11 | AutoArchiveCompletedTeams 静默忽略 TransitionStatus 错误 | 添加 Warn 日志 | `spirit_team_usecase.go` |
| L17 | checkAllTeamsCompleted 在循环内重复调用 | 移到循环外统一调用一次 | `spirit_team.go` |
| WIRE | provideFailurePatternSyncJob 接口注入 + 测试 stub 缺失 | 改为接收接口类型 + 补全 GetTeamByKey stub | `wire.go`, `*_test.go` |

### 10.4 aranea-review 审查结论

- **1 个阻断项**（R-01: IsTeamStatusActive 与 CheckAllTeamsCompleted 对 interrupted 语义不一致）→ 已修复
- **10 个建议项**（构造函数参数过多、魔法数字、函数超长等）→ 已全部修复（见 10.5）
- **1 个提示项**（NewOrchestrationCache 允许 repo 为 nil）→ 可接受
- **合规性清单**：依赖方向向内 ✅ | Runner 装配在 Service ✅ | goroutine 走 safego ✅ | 日志用 loggateway ✅ | 业务错误用 kerrors ✅ | 跨模块通过窄接口 ✅

### 10.5 迭代建议修复记录

> 2026-06-06：对 aranea-review 10 个建议项全部实施修复。

| ID | 问题 | 修复方案 | 影响文件 |
|----|------|----------|----------|
| S-01/S-03 | NewSpiritTeamUsecase 7参数过多 | Options 模式：`SpiritTeamUsecaseOption` + `WithSpiritTransactor`/`WithOrchestrationCache`/`WithEvolutionSuggestionRepo` | `spirit_team_usecase.go`, `wire.go`, `biz.go` |
| S-02 | TeamGraphSessionRepo 6方法超限 | 拆分为 `TeamGraphSessionReader`(2) + `TeamGraphSessionWriter`(4)，原接口嵌入组合 | `team_types.go` |
| S-04 | RecordCompletionWithAgents Get+Put 非原子 | 合并为单 Lock：直接操作 `c.entries`，避免 Get→Put 间锁释放 | `spirit_orchestration_cache.go` |
| S-05 | app.go 混用 Kratos logger 和 loggateway | 统一为 `lg`（loggateway.Logger），`logger` 仅用于 `kratos.Logger(logger)` | `app.go` |
| S-06 | SetTimeoutHandler 并发安全隐患 | `sync.Once` 保证只设置一次 | `spirit_team_usecase.go` |
| S-07 | ComputeDQScoreBreakdown 魔法数字 | 命名常量：`DQWeightValidity/Specificity/Correctness`、`DQScoreMin`、`DQSpecificityBase/Medium/High/Findings`、`DQTimePenaltyDivisorMs/Max/Factor`、`DQEvolutionThreshold` | `spirit_orchestration_cache.go`, `spirit_team_usecase.go` |
| S-08 | BuildGraphConfig 95行超限 | 拆分为 `buildAgentNodes`/`buildDependencyEdges`/`buildSequentialChainEdges`/`buildMergeEdges`/`isDependedOn` | `build_graph.go` |
| S-09 | buildSpiritTeamDefinitionJSON 魔法数字 | 命名常量：`SpiritTeamDefVersion`/`SpiritTeamDefaultTimeout`/`SpiritTeamDefaultMaxConc` | `spirit_team_usecase.go` |
| S-10 | AutoArchiveCompletedTeams 逐条DB | 新增 `BatchArchiveTeams` 方法（TeamWriter 接口 + data 层 Ent 批量 UPDATE），收集 ID 后一次性归档 | `team_usecase.go`, `team_repo.go`, `spirit_team_usecase.go`, 7个测试 stub |

### 10.6 二轮审查阻塞项修复记录

> 2026-06-06：对 aranea-review 二轮审查发现的 8 个阻塞项全部修复。

#### 后端阻塞项（3个）

| ID | 问题 | 修复方案 | 影响文件 |
|----|------|----------|----------|
| BR-R01 | TeamOrchestrationDeps 持有未使用的 `biz.TeamRepository` 上帝接口 | 移除 `Teams biz.TeamRepository` 字段，所有操作已通过 `TeamUC`/`SpiritUC`/`TeamsNative` | `chat_orchestrator.go`, `wire.go` |
| BR-R02 | `TeamRepository` 23方法上帝接口 | 添加 `Deprecated` 注释；迁移 `ExperienceAnalyticsUsecase`→`TeamReader+TeamRunReader`；迁移 `PackRepoAdapter`→`TeamReader+TeamWriter` | `team_usecase.go`, `experience_analytics.go`, `pack_repo.go`, `seed_pack.go` |
| BR-R03 | 超时回调 `context.Background()` 无超时控制 | 改为 `context.WithTimeout(context.Background(), 30*time.Second)` | `spirit_team_usecase.go` |

#### 前端阻塞项（5个）

| ID | 问题 | 修复方案 | 影响文件 |
|----|------|----------|----------|
| FE-R01 | TeamTaskCard.vue `as any` 类型逃逸 | 新增 `mappedStatus` 计算属性，映射 `SpiritTeamStatus→SessionStatus` | `TeamTaskCard.vue` |
| FE-R02 | TeamProgressCard 检查废弃状态值 | `'waiting_deps'→'pending'`，`'assembled'/'assembling'→'running'/'pending'` | `TeamProgressCard.vue` |
| FE-R03 | `TeamProgressView.status`/`TeamSynthesisResult.status` 为 `string` | 改为 `SpiritTeamStatus` 类型 | `types.ts` |
| FE-R04 | `updateTeamStatus` 参数为 `string` | 改为 `SpiritTeamStatus`；新增 `isValidTeamStatus` 类型守卫验证 WS 推送状态 | `stores/spirit/index.ts` |
| FE-R05 | `SpiritTeamMode` 包含后端不存在的 `'direct'` | 移除 `'direct'`，更新 modeLabel/modeToTopology 映射 | `types.ts`, `TeamTaskCard.vue`, `TeamProgressCard.vue` |

### 10.7 三轮审查修复记录

> 2026-06-06：三轮 aranea-review 发现前端子代理修复未实际写入，1 个阻断项 + 6 个建议项。

| ID | 问题 | 修复方案 | 影响文件 |
|----|------|----------|----------|
| SPO3-R01 | `isValidTeamStatus` 被引用但未定义，运行时必崩 | 添加 `VALID_TEAM_STATUSES` Set + `isValidTeamStatus` 类型守卫函数 | `stores/spirit/index.ts` |
| SPO3-S01/S02 | `TeamProgressView.status`/`TeamSynthesisResult.status` 仍为 `string` | 改为 `SpiritTeamStatus` | `types.ts` |
| SPO3-S03 | `updateTeamStatus` 参数仍为 `string` | 改为 `SpiritTeamStatus` | `stores/spirit/index.ts` |
| SPO3-S04/S05 | `direct` 死代码残留于 `modeToTopology`/`modeLabel` | 移除 `direct` 映射行 | `TeamProgressCard.vue`, `TeamAssemblyCard.vue` |
| SPO3-S06 | `TaskExecutionPanel.vue` 遗漏 `as any` | 添加 `mappedStatus` 计算属性（同 TeamTaskCard） | `TaskExecutionPanel.vue` |
| R3-05 | `experience_analytics.go` DQ 权重硬编码与常量不一致 | 替换 `0.4/0.3/0.3` 为 `DQWeightValidity/DQWeightSpecificity/DQWeightCorrectness` | `experience_analytics.go` |

#### 后端建议项修复（8个）

| ID | 问题 | 修复方案 | 影响文件 |
|----|------|----------|----------|
| R3-01 | `Runner` 依赖 Deprecated `TeamRepository` | 拆为 `TeamReader+TeamRunReader+TeamRunWriter+OrchestrationStepRepo+TaskDeadLetterRepo` 5个窄接口 | `runner.go`, `runner_team_trpc.go`, `runner_helpers.go`, `runner_team_turn.go`, `runner_team_observer.go`, `team_graph_run_finisher.go`, 4个测试文件 |
| R3-02 | `TeamRepository` 缺少迁移计划 | 添加 `TODO(debt)` 注释和 Issue 编号 | `team_usecase.go` |
| R3-03 | 超时回调 `context.Background()` 与优雅关闭冲突 | 添加 `Stop()` 方法取消所有 pending timers | `spirit_team_usecase.go` |
| R3-04 | `kerrors` 字符串拼接丢失错误链 | 改为 `.WithCause(err)` 保留错误链 | `spirit_team_usecase.go` 4处, `spirit_orchestration_cache.go` 2处 |
| R3-06 | `TruncateRunes` 截断长度魔法数字 | 提取 5 个命名常量（`MaxTeamDisplayNameLen` 等） | `spirit_team_usecase.go` |
| R3-07 | `AssembleTeam` ~115 行超限 | 提取 `registerTeamTimeout` 子方法 | `spirit_team_usecase.go` |
| R3-08 | `AnalyzeOrchestration` N+1 查询 | 新增 `ListTeamRunsByTeamIDs` 批量查询方法 | `team_usecase.go`, `team_repo.go`, `experience_analytics.go`, 12个测试 stub |
| R3-09 | `TeamOrchestrationDeps` 持有 `*biz.SpiritTeamUsecase` 具体类型 | 定义 `SpiritTeamController` 窄接口（5方法），`wire.Bind` 绑定 | `spirit_team_usecase.go`, `chat_orchestrator.go`, `wire.go` |
