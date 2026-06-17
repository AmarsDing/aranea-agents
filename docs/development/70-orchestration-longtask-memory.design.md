# 编排引擎 + 24h 长任务 + 领先记忆综合升级 — 设计文档

> 模块编号：70
> 关联需求：`70-orchestration-longtask-memory.md`
> 关联调研：`docs/reports/2026-06-17-research-orchestration-longtask-memory-upgrade.md`

---

## 一、架构总览

```
┌─────────────────────────────────────────────────────────────────┐
│                    前端（Vue 3 + Quasar）                         │
│  统一聊天 UI ──► WS 流式订阅 ──► 任务面板（短/长任务无感切换）      │
│  编排时间线 ──► 统一仪表盘 ──► 记忆侧边栏                        │
└────────────┬────────────────────────────────────┬───────────────┘
             │ WebSocket（流式事件）                │ HTTP（提交+控制）
             ▼                                     ▼
┌─────────────────────────────────────────────────────────────────┐
│              Kratos 传输层（HTTP/gRPC/WS）                        │
│  SendChatMessage → 提交到执行引擎 → 立即返回 run_id              │
│  StopGeneration / PauseRun / ResumeRun / GetRunProgress          │
└────────────┬────────────────────────────────────────────────────┘
             ▼
┌─────────────────────────────────────────────────────────────────┐
│     Layer 1: 强制任务规划（Always-On Planner）                    │
│     Intent Pass(强制) → 复杂度评估 → 任务分解 → 策略决策          │
│     预规划门控：Simple=Direct / Moderate+Complex=强制规划          │
├─────────────────────────────────────────────────────────────────┤
│     Layer 2: 动态 Agent 供给（Agent Factory）                     │
│     4层匹配(含向量) → 缺口检测 → AgentFactory(LLM生成)            │
│     先搜索关联现有 graph/team/agent/记忆，无则动态创建              │
├─────────────────────────────────────────────────────────────────┤
│     Layer 3: 自主 Graph 编排（Adaptive Graph Engine）             │
│     NL2Graph → 执行监控 → 运行时重规划 → 拓扑演化                 │
│     基于 trpc-agent-go Graph + CheckpointSaver 增强              │
├─────────────────────────────────────────────────────────────────┤
│     Layer 4: 全链路可观测（Unified Observability）                │
│     编排时间线 → 跨边界Trace → 统一仪表盘                         │
├─────────────────────────────────────────────────────────────────┤
│     统一执行引擎（基于 trpc-agent-go 增强）                        │
│     流式执行器(ManagedRunner) + 后台任务器(taskrun增强)            │
│     + 检查点管理器(Postgres CheckpointSaver)                      │
│     + 事件流总线(event.Bus + EventStore + WAL)                    │
│     + 并行工具执行器(Worktree + TransactionSandbox)               │
├─────────────────────────────────────────────────────────────────┤
│     领先记忆系统（L0-L4 + 5 项前沿增强）                          │
│     Bi-temporal + Ebbinghaus + Sleep-time + 主动召回 + 链接图     │
├─────────────────────────────────────────────────────────────────┤
│     Postgres（全量迁移，替代 SQLite 主库）                         │
│     主库(事务/FK/高并发写) + pgvector(向量) + 时序扩展             │
└─────────────────────────────────────────────────────────────────┘
```

---

## 二、Postgres 全量迁移设计

### 2.1 迁移策略

三阶段渐进迁移，降低风险：

| 阶段 | 范围 | 目标 |
|------|------|------|
| Phase 1 | EventStore/WAL/Checkpoint/Run/SessionRun/TeamRun/GraphExecution | 长任务可靠性优先 |
| Phase 2 | Memory(L2/L3/L4) + Usage | 记忆高频读写 + 向量检索 |
| Phase 3 | 其余 70+ 表 | 完成全量迁移 |

### 2.2 连接池配置

```go
// internal/data/data.go 改造
// 写连接池
writeDB.SetMaxOpenConns(16)
writeDB.SetMaxIdleConns(8)
writeDB.SetConnMaxIdleTime(5 * time.Minute)

// 读连接池
readDB.SetMaxOpenConns(32)
readDB.SetMaxIdleConns(16)
readDB.SetConnMaxIdleTime(5 * time.Minute)

// pgvector 独立连接池
pgDB.SetMaxOpenConns(8)
```

### 2.3 FK 与唯一约束补齐

```sql
-- INV-UNIQ-01: SessionRun 无 DB 唯一约束
CREATE UNIQUE INDEX idx_session_run_active ON session_runs (session_id)
  WHERE phase IN ('interactive', 'durable');

-- INV-UNIQ-02: TeamRun 无 DB 唯一约束
CREATE UNIQUE INDEX idx_team_run_active ON team_runs (team_id)
  WHERE status IN ('pending', 'running');

-- INV-REF-01: Message→Session 无 FK
ALTER TABLE messages ADD CONSTRAINT fk_messages_session
  FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE;

-- INV-REF-02: TeamRun 无 FK
ALTER TABLE team_runs ADD CONSTRAINT fk_team_runs_team
  FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;

-- INV-REF-03: GraphExecution 无 FK
ALTER TABLE graph_executions ADD CONSTRAINT fk_graph_executions_definition
  FOREIGN KEY (graph_definition_id) REFERENCES graph_definitions(id) ON DELETE CASCADE;
```

### 2.4 事务超时改造

```go
// internal/data/tx.go 改造
// 去掉 30s 硬超时，改为可配置
detached, detachedCancel := context.WithTimeout(
    context.Background(),
    d.TxTimeout(), // 从配置读取，默认 30s；SetTxTimeout(0) 禁用
)
```

**实现细节**：
- `Data` struct 新增 `txTimeout time.Duration` 字段
- `TxTimeout()` 方法返回当前超时配置
- `SetTxTimeout(d)` 支持运行时调整（如长运行 Postgres 操作可设为 0 禁用）
- 默认值 30s 保持向后兼容

### 2.5 错误翻译适配

```go
// internal/data/errors.go 扩展
func entErrToBizErr(err error, domain string) error {
    // ... 现有 SQLite 错误码处理 ...

    // 新增 Postgres 错误码（使用 errors.As 支持包装错误）
    var pgErr *pq.Error
    if errors.As(err, &pgErr) {
        switch pgErr.Code.Name() {
        case "unique_violation", "foreign_key_violation":  // 23505, 23503
            return apierror.Wrap(err, apierror.CodeConflict, domain)
        case "not_null_violation", "check_violation":      // 23502, 23514
            return apierror.Wrap(err, apierror.CodeBadRequest, domain)
        }
    }
    // ...
}
```

**实现细节**：
- 使用 `github.com/lib/pq` 驱动（非 `pgconn.PgError`），通过 `pgErr.Code.Name()` 获取 SQLSTATE 名称
- 类型断言改为 `errors.As(err, &pgErr)` 支持被 `fmt.Errorf("%w", err)` 包装的错误链遍历
- `isPostgresAlreadyExistsErr`（data.go）同样改用 `errors.As`，处理 SQLSTATE 42P07/42710/42701（duplicate_table/duplicate_object/duplicate_column）
- 错误翻译使用 `apierror.Wrap(err, code, domain)` 保留原始错误链（非 `apierror.NewXxx`）

---

## 三、统一执行引擎设计

### 3.1 核心改造：HTTP 请求只负责"提交 + 返回 + 订阅"

```
用户输入 → SendChatMessage → ExecutionEngine.Submit(input)
                              ↓
                    1. 创建 Run 记录（Postgres，状态=running）
                    2. 预规划门控（Layer 1）
                    3. 根据策略选择执行路径
                    4. 创建 ManagedRunner（WithDetachedCancel(true)）
                    5. go func() { runner.Run(ctx, ...) }()
                    6. EventProjector → event.Bus → WS 推送
                    7. 返回 run_id
```

### 3.2 taskrun 事件透传

```go
// pkg/trpc-agent-go/agent/taskrun/inprocess/service.go 增强
type Controller interface {
    // 现有方法
    Spawn(ctx, req) (string, error)
    Wait(ctx, id) (*Result, error)
    Cancel(ctx, id) error
    List(ctx) ([]*TaskInfo, error)

    // 新增：事件流透传
    Events(id string) (<-chan *event.Event, error)
}

// 实现：runChild 中同时消费事件流并转发到外部 channel
func (s *service) runChild(ctx, req) {
    events := runner.Run(ctx, ...)
    for evt := range events {
        result.consume(evt)
        progress.consume(evt)
        // 新增：转发到外部 channel
        if s.eventChs[id] != nil {
            select {
            case s.eventChs[id] <- evt:
            default: // drop if no consumer
            }
        }
    }
}
```

### 3.3 跨进程事件流

```go
// internal/event/postgres_eventstore.go 新增
type PostgresEventStore struct {
    db *pgxpool.Pool
}

// Save 持久化事件到 Postgres
func (s *PostgresEventStore) Save(ctx, envelope *Envelope) error

// Replay 从 Postgres 回放事件（WS 重连时）
func (s *PostgresEventStore) Replay(ctx, sessionID string, afterEventID string) ([]*Envelope, error)

// Cleanup 清理过期事件
func (s *PostgresEventStore) Cleanup(ctx, before time.Time) error
```

### 3.4 任务级心跳

```go
// internal/service/heartbeat.go 新增
type RunHeartbeatEmitter struct {
    interval time.Duration // 默认 10s
    bus      event.Bus
}

func (e *RunHeartbeatEmitter) Start(ctx, runID string, progress func() RunProgress) {
    ticker := time.NewTicker(e.interval)
    go func() {
        for {
            select {
            case <-ticker.C:
                p := progress()
                e.bus.Publish(ctx, &Envelope{
                    Type: EnvelopeTypeRunHeartbeat,
                    Content: &RunHeartbeatContent{
                        RunID:           runID,
                        ProgressPercent: p.Percent,
                        CurrentStep:     p.CurrentStep,
                        TotalSteps:      p.TotalSteps,
                        ETA:             p.ETA,
                    },
                })
            case <-ctx.Done():
                ticker.Stop()
                return
            }
        }
    }()
}
```

### 3.5 崩溃恢复

```go
// internal/service/recovery_worker.go 新增
type RecoveryWorker struct {
    runRepo       RunRepo
    checkpointSav graph.CheckpointSaver
    engine        ExecutionEngine
}

func (w *RecoveryWorker) Run(ctx) {
    // 扫描状态为 running 但无活跃心跳的 Run
    staleRuns := w.runRepo.FindStaleRuns(ctx, staleThreshold)
    for _, run := range staleRuns {
        // 从 checkpoint 恢复
        checkpoint, err := w.checkpointSav.Load(ctx, run.CheckpointID)
        if err != nil {
            w.runRepo.MarkFailed(ctx, run.ID, "recovery_failed")
            continue
        }
        // 重新提交执行
        w.engine.ResumeFromCheckpoint(ctx, run.ID, checkpoint)
    }
}
```

---

## 四、Layer 1：强制任务规划设计

### 4.1 预规划门控

```go
// internal/service/chat_orchestrator_turn.go 新增
func (o *ChatOrchestrator) prePlanningGate(ctx, input) (Strategy, error) {
    // 1. 强制 Intent Pass（1.5s 超时，失败非致命）
    intent, _ := o.intentPass.Run(ctx, input)

    // 2. 快速复杂度评估（纯计算，无 LLM）
    complexity := o.planner.QuickAssess(ctx, input, intent)

    // 3. 门控决策
    switch complexity.Level {
    case biz.ComplexitySimple:
        return biz.StrategyDirect, nil
    case biz.ComplexityModerate, biz.ComplexityComplex:
        // 强制走 plan_and_execute
        return o.planner.PlanAndExecute(ctx, input, intent, complexity)
    }
}
```

**实现差异说明**：

设计稿为概念草图，实际实现做了三处安全/效率改进：

1. **返回 `GateDecision` 而非 `Strategy`** — 实际 `PrePlanningGate.Evaluate` 返回结构化的 `GateDecision{Level, Score, ForcePlanning, Reason, IntentArtifact}`，携带复杂度分数和原因，便于日志和前端时间线展示。调用方（`chat_orchestrator_turn.go`）根据 `ForcePlanning` 标志决定是否注入强制规划 RunOption。

2. **复用已有 Intent Pass 结果而非重新运行** — 设计稿中门控自己调 `o.intentPass.Run()`，实际实现复用 BUILD 阶段并行运行的 Intent Pass 产物（`*intent.Artifact`）。这避免了重复 LLM 调用（节省 0.5-3s），且 Intent Pass 已在 `eg.Wait()` 前完成，门控只需 <1ms 纯计算。

3. **注入 RunOption 而非直接调 `PlanAndExecute`** — 设计稿中门控直接调 `o.planner.PlanAndExecute()`，实际实现通过 `forcedPlanningRunOption()` 注入一条系统消息（`trpcagent.WithInjectedContextMessages`），指示 Spirit LLM 调用 `plan_and_execute` 工具。这保持了正常 turn 流程的完整性（BUILD → EXECUTE → PERSIST → POST-PROCESS），让 plan_and_execute 作为工具在框架内执行，而非绕过 turn 生命周期。

**事件可靠性分级（AS-EVT-01）**：规划时间线事件（`planning_phase_start/progress/done`）归类为 **Informational** 级别（尽力而为，不持久化），因为它们仅用于前端时间线可见性，不影响业务状态一致性。

**代码锚点**：
- `internal/service/pre_planning_gate.go` — `PrePlanningGate.Evaluate` + `publishPlanningPhase`
- `internal/service/chat_orchestrator_turn_preplanning.go` — `runPrePlanningGate` + `forcedPlanningRunOption` + `intentArtifactToBiz`
- `internal/service/chat_orchestrator_turn.go:354-360` — 门控调用点（eg.Wait 后）
- `internal/agent/task_planner_impl.go:259-284` — `QuickAssess` 纯计算实现
- `internal/biz/task_planner.go:14` — `TaskPlannerPort.QuickAssess` 接口（Stability: evolving）

### 4.2 Intent Pass 默认开启

```go
// internal/agent/intent/pass.go 改造
func IntentPassFromAgent(ag biz.Agent) bool {
    if ag.Settings != nil {
        return ag.Settings.IntentPassEnabled // 显式设置仍可关闭
    }
    return true // 默认开启（原来是默认 false）
}
```

**实现差异说明**：
- 设计稿原用 `*bool` 指针区分"未设置"与"显式 false"，实际代码 `IntentPassEnabled` 为 plain bool
- 采用双层默认 ON 策略替代指针语义：
  1. `IntentPassFromAgent`：`ag.Settings == nil`（无 settings 整体）→ 返回 `true`
  2. `DefaultAgentRuntimeSettings()`：`IntentPassEnabled: true`（新 Agent 默认 ON）
- 显式 `IntentPassEnabled=false` 仍被尊重（agent setting 可关闭）
- A2A Proxy Agent 在 `agent_usecase.go:411` 显式覆盖为 `false`，不受影响
- 现有 DB 中已持久化 `false` 的 Agent 保持 OFF（不追溯变更，避免改变现有行为契约）

### 4.3 规划时间线事件

```go
// internal/event/contract/envelope.go 新增
const (
    EnvelopeTypePlanningPhaseStart    = "planning_phase_start"
    EnvelopeTypePlanningPhaseProgress = "planning_phase_progress"
    EnvelopeTypePlanningPhaseDone     = "planning_phase_done"
)

type PlanningPhaseContent struct {
    Phase   string  `json:"phase"`   // intent/assess/decompose/strategy
    Message string  `json:"message"`
    Duration float64 `json:"duration_ms"`
}
```

---

## 五、Layer 2：动态 Agent 供给设计

### 5.1 语义匹配接入 pgvector

```go
// internal/agent/agent_allocator_impl.go 改造
// Layer 2 从 TF-IDF 升级为 pgvector embedding

func (a *AgentAllocatorImpl) matchLayer2(ctx, subTask, candidates) (*TaskAllocation, error) {
    // 旧：TF-IDF 关键词相似度
    // 新：pgvector embedding 余弦相似度
    queryEmbedding, err := a.embedder.Embed(ctx, subTask.Description)
    if err != nil {
        return nil, err // fallback to TF-IDF
    }

    results, err := a.memoryRepo.SearchByEmbedding(ctx, queryEmbedding, topK=5)
    // ... 计算相似度并匹配
}
```

### 5.2 AgentFactory

```go
// internal/agent/agent_factory.go 新增
type AgentFactoryImpl struct {
    llm         model.Model
    agentRepo   biz.AgentRepo
    templateRepo biz.AgentTemplateRepo
    eventBus    event.Bus
    lg          loggateway.Logger
}

type TaskProfile struct {
    RequiredCapabilities []string
    Domain              string
    TaskDescription     string
    PreferredTools      []string
    PreferredModel      string
}

func (f *AgentFactoryImpl) EnsureAgent(ctx, profile TaskProfile) (string, error) {
    // 1. 先搜索关联现有 graph/team/agent/记忆
    //    - 搜索 Agent catalog（4 层匹配）
    //    - 搜索 Graph 模板库
    //    - 搜索 Team 模板
    //    - 搜索记忆（L3 事实 + L4 实体图）
    // 2. 有匹配 → 返回 agentKey
    // 3. 无匹配 → LLM 生成 Agent 定义

    // LLM 生成 Agent 定义
    agentDef, err := f.generateAgentDefinition(ctx, profile)
    if err != nil {
        return "", err
    }

    // 持久化到 DB
    agent, err := f.agentRepo.Create(ctx, &biz.Agent{
        AgentKey:    agentDef.AgentKey,
        DisplayName: agentDef.DisplayName,
        Description: agentDef.Description,
        Provider:    agentDef.Provider,
        Model:       agentDef.Model,
        ConfigJSON:  agentDef.ConfigJSON,
        Status:      biz.AgentStatusActive,
        Source:      "system", // 标记动态创建（对齐 agent.go schema enum: user/system/imported）
    })

    // 发布事件
    f.eventBus.Publish(ctx, &Envelope{
        Type: EnvelopeTypeAgentCreated,
        Content: &AgentCreatedContent{
            AgentKey:    agent.AgentKey,
            DisplayName: agent.DisplayName,
            Source:      "system",
            Trigger:     profile.TaskDescription,
        },
    })

    return agent.AgentKey, nil
}

type GeneratedAgentDefinition struct {
    AgentKey    string `json:"agent_key"`
    DisplayName string `json:"display_name"`
    Description string `json:"description"`
    Provider    string `json:"provider"`
    Model       string `json:"model"`
    ConfigJSON  string `json:"config_json"` // 含 tools/skills/prompt
}

func (f *AgentFactoryImpl) generateAgentDefinition(ctx, profile TaskProfile) (*GeneratedAgentDefinition, error) {
    // 1. 从模板库选择最接近的模板作为基础
    template := f.selectClosestTemplate(ctx, profile)

    // 2. LLM 定制化
    prompt := fmt.Sprintf(`Based on the task profile, generate an agent definition:
Task: %s
Domain: %s
Required Capabilities: %v
Preferred Tools: %v
Base Template: %s

Generate a JSON agent definition with: agent_key, display_name, description, provider, model, config_json (including tools, skills, system_prompt).`,
        profile.TaskDescription, profile.Domain,
        profile.RequiredCapabilities, profile.PreferredTools,
        template.DisplayName)

    // LLM 调用生成定义
    result, err := f.llm.Generate(ctx, prompt)
    // ... 解析 JSON 并返回
}
```

---

## 六、Layer 3：自主 Graph 编排设计

### 6.1 NL2Graph

```go
// internal/graph/nl2graph.go 新增
type NL2GraphConverterImpl struct {
    llm           model.Model
    templateReg   *TemplateRegistry
    agentCapRepo  biz.AgentCapabilityRepo
}

func (c *NL2GraphConverterImpl) Convert(ctx, taskDesc string, availableAgents []biz.AgentCapability) (*biz.GraphBuildConfig, error) {
    // 1. LLM 分析任务，识别子任务和依赖
    analysis := c.analyzeTask(ctx, taskDesc, availableAgents)

    // 2. 从 6 Graph 模板 + 5 Team 模板匹配最优模板
    template := c.matchTemplate(ctx, analysis)

    // 3. 用可用 Agent 填充模板节点
    config := c.fillTemplate(ctx, template, analysis, availableAgents)

    // 4. 环检测 + DAG 验证（复用现有 build_orchestration_graph 逻辑）
    if err := validateDAG(config); err != nil {
        // 回退为 sequential pipeline
        config = c.fallbackSequential(analysis, availableAgents)
    }

    return config, nil
}
```

### 6.2 RuntimeReplanner

```go
// internal/graph/runtime_replanner.go 新增
type RuntimeReplannerImpl struct {
    llm          model.Model
    graphBuilder GraphBuilderFactory
    eventBus     event.Bus
}

type ReplanAction struct {
    Type       ReplanType // retry/reroute/insert_fallback/rebuild_subgraph
    NewNodes   []biz.NodeDef
    NewEdges   []biz.EdgeDef
    SkipNodes  []string
}

type ReplanType string

const (
    ReplanRetry           ReplanType = "retry"
    ReplanReroute         ReplanType = "reroute"
    ReplanInsertFallback  ReplanType = "insert_fallback"
    ReplanRebuildSubgraph ReplanType = "rebuild_subgraph"
)

func (r *RuntimeReplannerImpl) OnNodeFailure(ctx, exec *biz.GraphExecution, failedNode string, err error) (*ReplanAction, error) {
    // 1. 分析失败原因
    failureAnalysis := r.analyzeFailure(ctx, exec, failedNode, err)

    // 2. 决策重规划类型
    switch failureAnalysis.Severity {
    case "transient":
        return &ReplanAction{Type: ReplanRetry}, nil
    case "agent_incapable":
        // 插入 fallback 节点
        fallback := r.createFallbackNode(ctx, exec, failedNode, failureAnalysis)
        return &ReplanAction{
            Type:     ReplanInsertFallback,
            NewNodes: []biz.NodeDef{fallback},
            NewEdges: []biz.EdgeDef{
                {From: failedNode + "_prev", To: fallback.ID},
                {From: fallback.ID, To: failedNode + "_next"},
            },
        }, nil
    case "subtask_invalid":
        // 重建子图
        subgraph := r.rebuildSubgraph(ctx, exec, failedNode, failureAnalysis)
        return &ReplanAction{
            Type:     ReplanRebuildSubgraph,
            NewNodes: subgraph.Nodes,
            NewEdges: subgraph.Edges,
            SkipNodes: []string{failedNode},
        }, nil
    }
}
```

### 6.3 Graph 拓扑演化

```go
// internal/graph/topology_evolution.go 新增
type TopologyEvolver struct {
    llm       model.Model
    eventBus  event.Bus
}

// OnExecutionInsight 执行过程中发现新路径时触发
func (e *TopologyEvolver) OnExecutionInsight(ctx, exec *biz.GraphExecution, insight ExecutionInsight) error {
    // insight: 节点 A 执行结果发现需要节点 C 的能力（非预定义路径）
    // LLM 决策：是否添加 A→C 边
    shouldAdd := e.llmDecideEdge(ctx, exec, insight)
    if !shouldAdd {
        return nil
    }

    // 动态添加 transfer 边（通过 Graph 版本管理记录）
    newEdge := biz.EdgeDef{
        From: insight.SourceNode,
        To:   insight.TargetNode,
        Kind: "transfer",
    }

    // 发布拓扑演化事件
    e.eventBus.Publish(ctx, &Envelope{
        Type: EnvelopeTypeGraphTopologyEvolved,
        Content: &GraphTopologyEvolvedContent{
            ExecutionID: exec.ID,
            NewEdge:     newEdge,
            Reason:      insight.Reason,
        },
    })

    return nil
}
```

---

## 七、Layer 4：全链路可观测设计

### 7.1 编排时间线视图

```typescript
// web/src/features/orchestration/OrchestrationTimeline.vue 新增
interface TimelinePhase {
  phase: 'planning' | 'allocation' | 'orchestration' | 'delivery'
  startedAt: number
  durationMs: number
  steps: TimelineStep[]
  result?: string
}

interface TimelineStep {
  name: string
  startedAt: number
  durationMs: number
  status: 'running' | 'completed' | 'failed' | 'skipped'
  metadata?: Record<string, unknown>
}
```

### 7.2 跨边界 Trace 传播

```go
// internal/service/turn_trace.go 扩展
type OrchestrationSpan struct {
    TraceID   string
    PlanSpan  trace.Span  // 规划阶段
    AllocSpan trace.Span  // 分配阶段（child of PlanSpan）
    OrchSpan  trace.Span  // 编排阶段（child of AllocSpan）
    NodeSpans []trace.Span // Graph 节点（children of OrchSpan）
}

// 通过 context 传播 TraceID
func WithOrchestrationTrace(ctx context.Context, span OrchestrationSpan) context.Context {
    return context.WithValue(ctx, orchestrationTraceKey{}, span)
}
```

### 7.3 Spirit 编排阶段 Metrics

```go
// internal/metrics/vars.go 新增
var (
    SpiritPlanDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "spirit_plan_duration_seconds",
        Help:    "Duration of Spirit planning phase",
        Buckets: prometheus.DefBuckets,
    })
    SpiritAllocDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "spirit_alloc_duration_seconds",
        Help:    "Duration of Spirit allocation phase",
        Buckets: prometheus.DefBuckets,
    })
    SpiritOrchDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "spirit_orch_duration_seconds",
        Help:    "Duration of Spirit orchestration phase",
        Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600},
    })
    AgentFactoryCreated = promauto.NewCounter(prometheus.CounterOpts{
        Name:    "agent_factory_created_total",
        Help:    "Total number of dynamically created agents",
    })
    GraphReplanTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name:    "graph_replan_total",
        Help:    "Total number of graph replans",
    }, []string{"type"}) // retry/reroute/insert_fallback/rebuild_subgraph
)
```

---

## 八、Cursor 级并行工具执行设计

### 8.1 ParallelToolExecutor

```go
// internal/tools/parallel_executor.go 新增
type ParallelToolExecutor struct {
    depAnalyzer    *DependencyAnalyzer
    worktreeIso    *WorktreeIsolator
    txSandbox      *TransactionSandbox
    maxConcurrency int // 默认 GOMAXPROCS
    eventBus       event.Bus
}

func (e *ParallelToolExecutor) Execute(ctx, toolCalls []ToolCall) ([]ToolResult, error) {
    // 1. 依赖分析：构建 DAG
    dag := e.depAnalyzer.Analyze(toolCalls)

    // 2. 按拓扑层级并行执行
    var results []ToolResult
    for _, layer := range dag.TopologicalLayers() {
        layerResults := e.executeLayer(ctx, layer)
        results = append(results, layerResults...)
    }

    return results, nil
}

func (e *ParallelToolExecutor) executeLayer(ctx, calls []ToolCall) []ToolResult {
    var wg sync.WaitGroup
    results := make([]ToolResult, len(calls))

    for i, call := range calls {
        wg.Add(1)
        go func(idx int, tc ToolCall) {
            defer wg.Done()

            // 根据工具类型选择隔离策略
            switch tc.IsolationStrategy {
            case "worktree":
                results[idx] = e.worktreeIso.Execute(ctx, tc)
            case "transaction":
                results[idx] = e.txSandbox.Execute(ctx, tc)
            default:
                results[idx] = e.executeDirect(ctx, tc)
            }
        }(i, call)
    }
    wg.Wait()
    return results
}
```

### 8.2 WorktreeIsolator

```go
// internal/tools/worktree_isolator.go 新增
type WorktreeIsolator struct {
    gitRepo string
}

func (i *WorktreeIsolator) Execute(ctx, call ToolCall) ToolResult {
    // 1. 创建 worktree
    worktree := i.createWorktree(ctx, call.ID)

    // 2. 在 worktree 中执行工具
    result := i.executeInWorktree(ctx, worktree, call)

    // 3. 成功 → 合并回主分支；失败 → 删除 worktree
    if result.Success {
        i.mergeWorktree(ctx, worktree)
    } else {
        i.removeWorktree(ctx, worktree)
    }

    return result
}
```

### 8.3 TransactionSandbox

```go
// internal/tools/transaction_sandbox.go 新增
type TransactionSandbox struct {
    data *Data
}

func (s *TransactionSandbox) Execute(ctx, call ToolCall) ToolResult {
    err := s.data.ExecInTx(ctx, func(txCtx context.Context) error {
        // 在事务中执行工具
        result := executeToolInTx(txCtx, call)
        if !result.Success {
            return fmt.Errorf("tool failed: %s", result.Error)
        }
        return nil
    })
    // 事务失败自动回滚
    // ...
}
```

---

## 九、领先记忆系统设计

### 9.1 Bi-temporal 失效标记

```go
// pkg/trpc-agent-go/memory/memory.go 扩展
type Memory struct {
    ID        string
    Content   string
    Topics    []string
    Kind      Kind
    Metadata  Metadata
    // 新增
    ValidFrom  *time.Time `json:"valid_from,omitempty"`
    ValidUntil *time.Time `json:"valid_until,omitempty"`
}

// SearchMemories 默认过滤
// WHERE valid_until IS NULL OR valid_until > now()
```

### 9.2 Ebbinghaus 衰减评分

```go
// internal/memory/ebbinghaus.go 新增
type EbbinghausDecayWorker struct {
    memoryRepo biz.MemoryRepo
    interval   time.Duration // 默认 1h
}

func (w *EbbinghausDecayWorker) computeDecayScore(m *biz.Memory) float64 {
    // R_t = exp(-n_t / S_t)
    // n_t = time since last access
    // S_t = U_t + F_t + ε·T
    //   U_t = time since creation
    //   F_t = access frequency factor
    //   T = total lifetime
    n_t := time.Since(m.LastAccessedAt).Hours()
    S_t := m.CreationAgeHours() + m.AccessCount*24 + 0.001*m.CreationAgeHours()
    return math.Exp(-n_t / S_t)
}
```

### 9.3 Sleep-time Agent 异步整理

```go
// internal/memory/sleep_time.go 新增
// 复用 EnqueueAutoMemoryJob 管道架构
func (s *SleepTimeService) EnqueueConsolidationJob(ctx, sess session.Service) error {
    // 1. 读取近期记忆
    memories := s.memoryRepo.ReadRecent(ctx, userID, limit=50)

    // 2. LLM 分析：合并重复、提取反思、更新 core memory
    consolidation := s.llmConsolidate(ctx, memories)

    // 3. 执行操作
    for _, op := range consolidation.Operations {
        switch op.Type {
        case "merge":
            s.memoryRepo.Update(ctx, op.TargetID, op.MergedContent)
            s.memoryRepo.Delete(ctx, op.SourceIDs...)
        case "reflect":
            s.memoryRepo.Add(ctx, op.Reflection)
        case "update_core":
            s.updateCoreMemory(ctx, op.Updates)
        }
    }
}
```

### 9.4 主动召回触发器

```go
// pkg/trpc-agent-go/memory/memory.go 接口扩展
type Service interface {
    // 现有方法...

    // 新增：主动召回
    ProactiveRecall(ctx context.Context, convCtx ConversationContext) ([]*Entry, error)
}

type ConversationContext struct {
    MentionedEntities []string // 提及的人/地/主题
    CurrentTopic      string
    UserStatement     string   // 用户陈述（可能与存储矛盾）
}

// 实现：基于提及实体检索关联记忆，无需显式 query
```

### 9.5 记忆链接图 Evolution

```go
// pkg/trpc-agent-go/memory/memory.go Entry 扩展
type Entry struct {
    ID      string
    Content string
    Score   float64
    // 新增
    Links    []string `json:"links,omitempty"`    // 关联记忆 ID
    Keywords []string `json:"keywords,omitempty"` // LLM 生成关键词
    Tags     []string `json:"tags,omitempty"`     // LLM 生成标签
}

// AddMemory 后异步触发 link generation
// 新笔记 → cosine 相似度检索历史 → LLM 判断是否建立链接 → 更新历史记忆的 keywords/tags
```

---

## 十、体验设计

### 10.1 ErrorBlock 内联重试

```vue
<!-- web/src/components/chat/ErrorBlock.vue 改造 -->
<template>
  <div class="error-block">
    <div class="error-content">
      <q-icon :name="statusIcon" />
      <span>{{ message }}</span>
      <span class="error-hint">({{ actionLabel }})</span>
    </div>
    <div class="error-actions">
      <q-btn v-if="action === 'retry'" flat label="重试" @click="$emit('retry')" />
      <q-btn v-if="action === 'switch_model'" flat label="切换模型" @click="$emit('switch-model')" />
      <q-btn v-if="action === 'rephrase'" flat label="重新表述" @click="$emit('rephrase')" />
    </div>
  </div>
</template>
```

### 10.2 编排时间线视图

```vue
<!-- web/src/features/orchestration/OrchestrationTimeline.vue -->
<template>
  <div class="orchestration-timeline">
    <div v-for="phase in phases" :key="phase.phase" class="timeline-phase">
      <div class="phase-header">
        <q-icon :name="phaseIcon(phase.phase)" />
        <span>{{ phaseLabel(phase.phase) }}</span>
        <span class="duration">{{ formatDuration(phase.durationMs) }}</span>
      </div>
      <div v-for="step in phase.steps" :key="step.name" class="timeline-step">
        <span :class="step.status">{{ step.name }}</span>
        <span class="duration">{{ formatDuration(step.durationMs) }}</span>
      </div>
    </div>
  </div>
</template>
```

---

## 十一、数据模型变更

### 11.1 新增 Ent Schema

```go
// internal/data/ent/schema/agent.go 已有 source 字段
field.Enum("source").Values("user", "system", "imported").Default("user").Comment("agent source: origin tracking (user | system | imported), aligned with team.source"),
```

> 注：动态创建的 Agent 标记为 `source="system"`（非 "dynamic"），与现有 enum 值对齐。详见 `internal/biz/agent_types.go:121`。

### 11.2 Memory 表新增列

```sql
ALTER TABLE memories ADD COLUMN valid_from TIMESTAMP;
ALTER TABLE memories ADD COLUMN valid_until TIMESTAMP;
ALTER TABLE memories ADD COLUMN access_count INTEGER DEFAULT 0;
ALTER TABLE memories ADD COLUMN last_accessed_at TIMESTAMP;
ALTER TABLE memories ADD COLUMN decay_score FLOAT DEFAULT 1.0;
ALTER TABLE memories ADD COLUMN links JSONB DEFAULT '[]';
ALTER TABLE memories ADD COLUMN keywords JSONB DEFAULT '[]';
ALTER TABLE memories ADD COLUMN tags JSONB DEFAULT '[]';
```

### 11.3 新增事件表

```sql
CREATE TABLE event_store (
    id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(64) NOT NULL,
    session_id VARCHAR(64) NOT NULL,
    envelope_type VARCHAR(64) NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    INDEX idx_event_store_session_created (session_id, created_at),
    INDEX idx_event_store_type (envelope_type)
);
```

---

## 十二、Proto/API 契约

### 12.1 新增 RPC

> 以下 RPC 为 Phase 1-2 规划新增，当前 `api/kratos/chat/v1/chat.proto` 尚未包含。现有 ChatService 已有 `StopGeneration`/`GetRunStatus`/`AwaitUserReply` 等运行控制 RPC。

```protobuf
// api/kratos/chat/v1/chat.proto 新增（规划中）
rpc PauseRun(PauseRunRequest) returns (PauseRunResponse);
rpc ResumeRun(ResumeRunRequest) returns (ResumeRunResponse);
rpc GetRunProgress(GetRunProgressRequest) returns (GetRunProgressResponse);

message PauseRunRequest { string run_id = 1; }
message PauseRunResponse {}
message ResumeRunRequest { string run_id = 1; bytes resume_value = 2; }
message ResumeRunResponse {}
message GetRunProgressRequest { string run_id = 1; }
message GetRunProgressResponse {
    string run_id = 1;
    float progress_percent = 2;
    string current_step = 3;
    int32 total_steps = 4;
    int64 eta_seconds = 5;
}
```

### 12.2 新增事件类型

> 当前 `internal/event/contract/envelope.go` 包含 `EnvelopeTypePlanningPhaseStart/Progress/Done`（P1-2 预规划门控使用）。以下完整列表包含规划新增的 4 个事件类型（RunHeartbeat/AgentCreated/GraphTopologyEvolved/GraphReplanned）。任务实现状态详见 [70-orchestration-longtask-memory.development.md §七](./70-orchestration-longtask-memory.development.md#七验收标准)。

```go
// internal/event/contract/envelope.go 新增
const (
    EnvelopeTypeRunHeartbeat          = "run_heartbeat"
    EnvelopeTypeAgentCreated          = "agent_created"
    EnvelopeTypePlanningPhaseStart    = "planning_phase_start"
    EnvelopeTypePlanningPhaseProgress = "planning_phase_progress"
    EnvelopeTypePlanningPhaseDone     = "planning_phase_done"
    EnvelopeTypeGraphTopologyEvolved  = "graph_topology_evolved"
    EnvelopeTypeGraphReplanned        = "graph_replanned"
)
```

---

## 十三、技术选型

| 组件 | 选型 | 理由 |
|------|------|------|
| 数据库 | Postgres 16 + pgvector | 全量迁移，突破 SQLite 单写瓶颈 |
| 连接池 | pgxpool | Go 标准 Postgres 驱动 |
| 事务隔离 | Read Committed（默认） | 平衡一致性与并发 |
| 向量检索 | pgvector ivfflat | 已在用，无需新增依赖 |
| Git 隔离 | go-git + worktree | 纯 Go 实现，无 CGO 依赖 |
| Trace | OTel + W3C TraceContext | 标准 propagation 格式 |
| Metrics | Prometheus | 已在用 |
| 记忆衰减 | Ebbinghaus 曲线 | 论文验证，OBLIVION 2026 |
| 记忆冲突 | Bi-temporal model | Zep/Graphiti 验证 |

---

## 十四、Phase 0 设计参考

> Phase 0 聚焦 P0 阻断修复 + Postgres Phase 1 迁移，为后续 Phase 1-3 奠定基础。本章节记录 Phase 0 的核心设计决策，作为后续 Phase 的参考基线。任务清单与验收状态详见 [70-orchestration-longtask-memory.development.md §三](./70-orchestration-longtask-memory.development.md#三phase-0基础夯实p0-阻断修复--postgres-phase-1)。

### 14.1 P0-1：WBPF 语义修复（AS-EVT-01）

**问题**：原 `event.Infra.Publish` 对 Critical 事件 WAL 写入失败时仍发布事件，违反 AS-EVT-01 "Critical 事件 WBPF + 重试" 要求。

**设计**：区分 pre-publish 与 post-publish 失败语义：

```
Critical 事件发布流程：
  1. serialize envelope
  2. WAL.Insert(envelope)         ← pre-publish 阶段
  3. publishToBuses(envelope)     ← 发布阶段
  4. WAL.markPublished(envelopeID) ← post-publish 阶段
```

**失败处理矩阵**：

| 失败阶段 | 行为 | 日志级别 | 日志消息 | 事件可见性 |
|---------|------|---------|---------|-----------|
| pre-publish（serialize/insert） | 不发布事件 | Error | "WAL insert failed, event dropped" | 不可见（正确） |
| post-publish（markPublished） | 事件已发布，仅记录 | Warn | "event published but markPublished failed, may republish on restart" | 可见（可能重复） |

**实现要点**：
- `contract.IsCriticalWBPFType(envelope.Type)` 判定 Critical 事件
- 非 Critical 事件直接走 `publishToBuses`，无 WAL 开销
- WAL 失败不阻塞非 Critical 事件路径

> 改动文件与验收状态详见 [70-orchestration-longtask-memory.development.md §3.1](./70-orchestration-longtask-memory.development.md#31-p0-1修复-wbpf-语义违规)。

### 14.2 P0-2：GraphExecution 状态机（AS-FSM-01）

**问题**：GraphExecution 状态变更通过 `exec.Status =` 直接赋值，绕过状态机校验，违反 AS-FSM-01 ">3 状态实体必须定义显式状态机" 要求。

**设计**：定义 5 状态 + 7 转换的显式状态机，采用 authoritative 模式（非法转换拒绝并保留原状态）。

**状态枚举**：

```go
const (
    GraphExecRunning      GraphExecutionState = "running"
    GraphExecCompleted    GraphExecutionState = "completed"
    GraphExecFailed       GraphExecutionState = "failed"
    GraphExecCancelled    GraphExecutionState = "cancelled"
    GraphExecWaitingHuman GraphExecutionState = "waiting_human"
)
```

**转换规则表**（7 条）：

| From | Event | To | 场景 |
|------|-------|----|------|
| Running | complete | Completed | 正常完成 |
| Running | fail | Failed | 执行失败 |
| Running | cancel | Cancelled | 用户取消 |
| Running | interrupt | WaitingHuman | HITL 暂停 |
| WaitingHuman | resume | Running | HITL 恢复 |
| WaitingHuman | cancel | Cancelled | HITL 期间取消 |
| WaitingHuman | fail | Failed | HITL 期间节点错误（新增） |

**接口设计**：

```go
// internal/biz/graph_execution_state_machine.go
// GraphExecutionStateMachine 为 struct（非 interface），包装通用状态机
type GraphExecutionStateMachine struct {
    inner *shared.GenericStateMachine[GraphExecutionState, GraphExecutionEvent]
}

// Transition 校验并执行状态转换，非法转换返回 error
func (sm *GraphExecutionStateMachine) Transition(from GraphExecutionState, event GraphExecutionEvent) (GraphExecutionState, error)

// CanTransition 校验 from→to 是否合法（注意：参数为 from/to，非 from/event）
func (sm *GraphExecutionStateMachine) CanTransition(from, to GraphExecutionState) bool

// internal/biz/graph_execution_usecase.go
func (uc *GraphExecutionUsecase) applyExecTransition(exec *GraphExecution, event GraphExecutionEvent) error {
    // authoritative 模式：非法转换拒绝并保留原状态
    from := ParseGraphExecutionState(exec.Status)
    newState, err := uc.sm.Transition(from, event)
    if err != nil {
        uc.lg.Warn("graph: illegal state transition rejected by FSM",
            loggateway.StepID("graph.fsm_rejected"),
            loggateway.Str("execution_id", exec.ID),
            loggateway.Str("from", string(from)),
            loggateway.Str("event", string(event)),
            loggateway.Err(err))
        return err
    }
    exec.Status = string(newState)
    return nil
}
```

**集成点**：
- 所有 `exec.Status =` 直接赋值改为 `applyExecTransition(ctx, exec, event)`
- `ResumeExecution` 复用 `uc.sm` 而非创建新实例
- `team_graph_run_coordinator.go` 的 `CanTransition` 调用已校验，fallback 路径注释更新

**测试覆盖**（6 个用例）：
- 合法转换：Running→Completed/Failed/Cancelled/WaitingHuman
- 非法转换：Completed→Running 被拒绝，状态保留
- 终态无出口：Completed/Failed/Cancelled 无后续转换
- HITL 转换：WaitingHuman→Running/Cancelled/Failed

> 改动文件清单与验收状态详见 [70-orchestration-longtask-memory.development.md §3.2](./70-orchestration-longtask-memory.development.md#32-p0-2接入状态机)。

### 14.3 P0-3：Postgres Phase 1 迁移

**目标**：将 WAL/EventStore/Checkpoint 关键表迁移到 Postgres 原生 schema，突破 SQLite 单写瓶颈。

**架构设计**：

```
┌─────────────────────────────────────────────────────────────┐
│                    EventWAL 双后端选择                        │
│                                                             │
│  NewEventWAL(pgDB, sqliteDB)                                │
│       │                                                     │
│       ├─ pgDB != nil → PostgresWALStorage（Phase 1 优先）    │
│       │                                                     │
│       └─ pgDB == nil → SQLiteWALStorage（回退）              │
└─────────────────────────────────────────────────────────────┘
```

**PostgresWALStorage 设计**：

```go
// internal/event/postgres_wal_storage.go
type PostgresWALStorage struct {
    db *sql.DB // Postgres 连接
}

// Insert 使用 ON CONFLICT DO NOTHING 实现幂等
// markPublished 使用 UPDATE ... WHERE published = false
// LoadPending 使用 SELECT ... WHERE published = false ORDER BY created_at
// 使用 TIMESTAMPTZ（非 TIMESTAMP）+ $N 占位符（非 ?）
```

**Wire DI 类型歧义解决**：

```go
// cmd/admin/wire.go
// 问题：*sql.DB 在 Data 中有多个实例（entClient/rawDB/pg），Wire 无法区分
// 解决：provideEventWAL 从 *data.Data 提取双 DB 句柄
func provideEventWAL(d *data.Data, lg loggateway.Logger) *event.EventWAL {
    if d == nil {
        return nil
    }
    sqliteDB := d.RWDB().WriteHandle()  // SQLite 回退
    pgDB := d.Postgres()                // Postgres 优先（可能为 nil）
    return event.ProvideEventWAL(sqliteDB, pgDB, lg)
}
```

**幂等迁移设计**：

```sql
-- internal/data/sql/migrations/20260617_postgres_phase1.sql
-- 表/索引用 IF NOT EXISTS；FK 约束用 DO $$ ... IF NOT EXISTS ... END $$ 检查 pg_constraint

CREATE TABLE IF NOT EXISTS event_wal (...);  -- 幂等
CREATE UNIQUE INDEX IF NOT EXISTS idx_session_runs_active_unique
    ON session_runs(session_id) WHERE phase NOT IN ('completed', 'failed', 'cancelled');

-- FK 约束：检查 pg_constraint 后添加（非 EXCEPTION WHEN duplicate_table）
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_session_run_checkpoints_run') THEN
        ALTER TABLE session_run_checkpoints
            ADD CONSTRAINT fk_session_run_checkpoints_run
            FOREIGN KEY (session_run_id) REFERENCES session_runs(id) ON DELETE CASCADE;
    END IF;
END $$;
```

> 注：实际实现的 INV-REF 约束与设计稿 §2.3 不同。设计稿原列 Message→Session / TeamRun→Team / GraphExecution→GraphDefinition，实际迁移文件实现的是 session_run_checkpoints→session_runs / event_store→sessions / session_run_checkpoints→sessions。详见 `internal/data/sql/migrations/20260617_postgres_phase1.sql`。

**SQLSTATE 错误处理**：

| SQLSTATE | 名称 | 处理 |
|----------|------|------|
| 42P07 | duplicate_table | 迁移幂等，跳过 |
| 42710 | duplicate_object | 迁移幂等，跳过 |
| 42701 | duplicate_column | 迁移幂等，跳过 |
| 23505 | unique_violation | 翻译为 Conflict |
| 23503 | foreign_key_violation | 翻译为 Conflict |
| 23502 | not_null_violation | 翻译为 BadRequest |
| 23514 | check_violation | 翻译为 BadRequest |

> 注：`40001 serialization_failure` 在设计稿中曾列为 Conflict 翻译，但实际 `internal/data/errors.go` 未实现此 case。如需支持可后续扩展。

> 改动文件清单与验收状态详见 [70-orchestration-longtask-memory.development.md §3.3](./70-orchestration-longtask-memory.development.md#33-p0-3postgres-phase-1-迁移)。

### 14.4 P0-4：DB-R5 错误翻译修复

**问题**：多个 Repo 文件中 `return err` 直接返回 Ent/Raw SQL 错误，违反 DB-R5 "禁止 Repo 方法直接返回 Ent 错误" 要求。

**设计**：所有 DB 错误必须经 `entErrToBizErr(err, domain)` 翻译为 `apierror.Error`。

**翻译策略**：

| 错误来源 | 处理方式 |
|---------|---------|
| Ent 操作（Create/Update/Delete/Query） | `entErrToBizErr(err, "DOMAIN")` |
| Raw SQL 操作（ExecContext/QueryContext） | `entErrToBizErr(err, "DOMAIN")` |
| 同文件函数调用 | pass-through（`entErrToBizErr` 对 `apierror.Error` 透传，避免双重包装） |
| 非 DB 错误（json.Unmarshal 等） | 不翻译，直接返回 |
| 跨 Repo 调用返回的错误 | 必须翻译（如 `metricsWriter.ApplyMetricsDelta()` 返回的错误） |

**Domain 命名规范**：使用大写下划线格式，如 `SESSION`、`SESSION_RUN`、`SESSION_METRICS`、`AGENT`、`TOOL`、`MONITOR`、`MEMORY_L1`、`MODEL_REGISTRY`、`AGENT_PERFORMANCE`、`BORROW_REQUEST`。

> 改动文件清单与验收状态详见 [70-orchestration-longtask-memory.development.md §3.4](./70-orchestration-longtask-memory.development.md#34-p0-4修复-db-r5-错误翻译)。
