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
    d.txTimeout(), // 从配置读取，默认 60s
)
```

### 2.5 错误翻译适配

```go
// internal/data/ent_err.go 扩展
func entErrToBizErr(err error, domain string) error {
    // ... 现有 SQLite 错误码处理 ...

    // 新增 Postgres 错误码
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) {
        switch pgErr.Code {
        case "23505": // unique_violation
            return apierror.NewConflict(domain, pgErr.Detail)
        case "23503": // foreign_key_violation
            return apierror.NewBadRequest(domain, pgErr.Detail)
        case "23502": // not_null_violation
            return apierror.NewBadRequest(domain, pgErr.Detail)
        case "40001": // serialization_failure
            return apierror.NewConflict(domain, "concurrent modification")
        }
    }
    // ...
}
```

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

### 4.2 Intent Pass 默认开启

```go
// internal/agent/intent/pass.go 改造
func (p *Pass) PassEffective(ag *biz.Agent, content string) bool {
    // 改为默认开启
    if ag.IntentPassEnabled != nil {
        return *ag.IntentPassEnabled // 仍可通过 agent setting 关闭
    }
    return true // 默认开启（原来是默认 false）
}
```

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
        Source:      "dynamic", // 标记动态创建
    })

    // 发布事件
    f.eventBus.Publish(ctx, &Envelope{
        Type: EnvelopeTypeAgentCreated,
        Content: &AgentCreatedContent{
            AgentKey:    agent.AgentKey,
            DisplayName: agent.DisplayName,
            Source:      "dynamic",
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
// internal/data/ent/schema/agent.go 新增字段
field.String("source").Default("manual").Comment("创建来源：manual/dynamic"),
```

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

```protobuf
// api/kratos/chat/v1/chat.proto 新增
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
