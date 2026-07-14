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

**实现差异（Wave 1 落地）**：

1. **Controller 接口方法数**：实际 Controller 接口含 6 方法（Spawn/List/Get/Cancel/Wait/Events），超过 ≤5 建议。审查建议拆分为 `EventStreamer` 独立接口，但因涉及框架代码修改（用户明确禁止），保留现状。

2. **`eventChs` map 保护**：`eventChs map[string]chan *event.Event` 由 `s.mu` 保护。`forwardEvent` 在持有 `s.mu` 期间执行 non-blocking send（select+default），锁持有时间极短，防止与 `closeEventChannel` 竞态导致 send-on-closed-channel panic。

3. **drop policy 与 AS-EVT-01 对齐**：non-blocking send（select+default）静默丢弃满缓冲事件，符合 Informational 级别"尽力而为，丢弃不报错"的语义。

4. **`closeEventChannel` 幂等**：终态转换路径（finishRun/failPersistedRun/finalizeCanceledRun）均调用 `closeEventChannel`，第二次调用时 `ch = nil`（已从 map 删除），不会重复 close。

5. **单订阅者限制**：每个 run 仅允许一个订阅者（`if _, exists := s.eventChs[runID]; exists` 拒绝第二次订阅）。重复订阅错误用 `fmt.Errorf` 而非哨兵错误（框架豁免，不修改 trpc 代码）。

6. **`eventChannelBuffer = 64`**：命名常量，注释清晰说明 drop policy 语义。

### 3.3 跨进程事件流

```go
// internal/event/postgres_eventstore.go 新增
type PostgresEventStore struct {
    db *sql.DB        // Postgres 连接（来自 data.Data.Postgres()）
    lg loggateway.Logger
}

// Save 持久化事件到 Postgres（幂等：ON CONFLICT DO NOTHING）
func (s *PostgresEventStore) Save(ctx, envelope *Envelope) error

// Replay 从 Postgres 回放事件（WS 重连时），支持 afterEventID 游标 + limit 上限
func (s *PostgresEventStore) Replay(ctx, sessionID string, afterEventID string, limit int) ([]*Envelope, error)

// Cleanup 清理过期事件
func (s *PostgresEventStore) Cleanup(ctx, before time.Time) error

// EnsureSchema 幂等建表（构造时调用）
func (s *PostgresEventStore) EnsureSchema(ctx) error
```

**`CrossProcessStore` 窄接口**（`internal/event/contract/bus.go`，`Stability:evolving`）：

```go
type CrossProcessStore interface {
    Save(ctx context.Context, env *Envelope) error
    Replay(ctx context.Context, sessionID string, afterEventID string, limit int) ([]*Envelope, error)
}
```

> 上层（`EventBusConsumer`/`WSServer`）依赖 `contract.CrossProcessStore` 接口而非具体实现，避免分层违规。`PostgresEventStore` 实现该接口。

**双写路径**（`internal/biz/event_bus_consumer.go::handleEnvelope`）：

```go
// shouldPersistEnvelope(env) 为真时，best-effort 非阻塞写 Postgres
if c.crossProcessSink != nil && shouldPersistEnvelope(env) {
    if err := c.crossProcessSink.Save(ctx, &env); err != nil && c.logger != nil {
        c.logger.LogSessionWarn(ctx, env.SessionID, "event_store.cross_process",
            "跨进程事件持久化失败", ...)
    }
}
```

- 双写为 best-effort：失败仅 Warn 日志，不阻塞主流程（符合 Informational/Important 级别语义）
- `shouldPersistEnvelope` 过滤掉纯日志类事件，避免 Postgres 写入压力

**WS replay fallback**（`internal/server/ws_event.go::replayEvents`）：

```go
// 内存 buffer 为空时回退到 Postgres
if len(events) == 0 && s.crossProcessStore != nil {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    pgEvents, err := s.crossProcessStore.Replay(ctx, sessionID, lastEventID, 100)
    cancel()
    if err == nil {
        for _, env := range pgEvents {
            events = append(events, *env)
        }
    }
}
```

- 5s 超时防止 Postgres 慢查询阻塞 WS 重连
- 仅在内存 buffer 为空时触发（进程重启或跨实例重连场景）

**Wire 接线**（`cmd/admin/wire.go` + `cmd/admin/app.go`）：

- `providePostgresEventStore` 从 `data.Data.Postgres()` 取连接，构造时 `EnsureSchema` 幂等建表
- `startReadinessDependentServices` 在 `consumer.Start(ctx)` 前调用 `consumer.WithCrossProcessSink(pgEventStore)`
- `WSServer` 通过 `infra.CrossProcessStore` 注入（`NewInfra` 第三参数）

**错误域**：`apierror.DomainEventStore = "EVENT_STORE"`（`pkg/apierror/domains.go`），所有 PostgresEventStore 错误经 `apierror.Wrap(err, CodeInternal, DomainEventStore)` 翻译。

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

> P1-8 已落地（Wave 4）。实际实现与原设计的差异：
> - **窄接口替代全 Repo**：`staleRunLister`（仅 `FindStaleRuns`）替代 `RunRepo`，遵循接口窄化原则（DB-N3/AS-FIT-01）
> - **依赖替换**：`engine ExecutionEngine` 改为 `resumer biz.DurableResumeGateway`（仅暴露 `ResumeFromCheckpoint`，更窄）
> - **构造 nil 防御**：`NewRecoveryWorker` 任一依赖为 nil 即返回 nil（红线 #26）
> - **可配置轮询周期**：新增 `pollInterval time.Duration` 字段（默认 5min，测试可注入短周期）
> - **safego.Go 启动**：`Run` 内部循环由 `safego.Go` 包裹（红线 #13），通过 `workers.go::goAfterReady` 在 ReadinessGate 通过后启动
> - **Postgres CheckpointSaver**：`pkg/trpc-agent-go/graph/checkpoint/postgres/saver.go`，使用 `$N` 占位符 + `ON CONFLICT DO UPDATE`，`PutFull` 在单事务内删除+插入

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

**实现差异（Wave 1 落地）**：

实际实现与上述设计草图有以下差异，已通过 aranea-review 审查确认：

1. **in-memory cosine 而非 pgvector SQL**：实际实现调用 `embedder.Embed()`（OpenAI/Gemini/Ollama text-embedding API）后在 Go 内存中计算 cosine 相似度（`cosineSimilarity32` helper），**未使用 pgvector SQL `<=>` 操作符，未访问 `vector_embeddings` 表，未走 `d.Postgres()`**。设计理由：allocator 是热路径，避免 Postgres 往返；pgvector SQL 仅用于 memory 域（`internal/data/vector/pgvector_fact.go`）的批量检索。

2. **批量 embedding**：将 task 文本 + 所有 agent capability 文本合并为单次 `Embed()` 调用（`allTexts []string`），减少 HTTP 往返。`vectors[0]` 为 task 向量，`vectors[1:]` 为 agent 向量。

3. **三级 fallback 链**：`matchLayer2` → `matchLayer2Embedding`（ok=false）→ `matchLayer2TFIDF`。embedding 失败/nil embedder/维度不匹配均安全降级，不向调用方抛硬错误。

4. **`cosineSimilarity32` 防御性编程**：处理空向量、零范数向量、维度不匹配三种边界，均返回 0 而非 panic。

5. **Embedder 接口来源**：复用 `internal/knowledge.Embedder` 接口（`Embed(ctx, []string) ([][]float32, error)` + `Dim() int`），Wire 绑定 `*knowledge.MultiProviderEmbedder`。

6. **whole-plan 路径未升级**：`matchLayer2ForPlan`（whole-plan 路径）仍使用 TF-IDF，仅 subtask 路径升级为 embedding。后续迭代可统一。

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
    template, tmplErr := f.selectClosestTemplate(ctx, profile)
    if tmplErr != nil {
        // CQ-3 修复：模板查询失败时 Warn 日志并降级为空模板，不中断生成流程
        f.lg.Warn("AgentFactory 模板查询失败，使用空模板",
            loggateway.StepID("agent_factory.template_query"),
            loggateway.Err(tmplErr),
        )
    }

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

**Allocator 集成**（`internal/agent/agent_allocator_impl.go`，P1-4 审查修复）：

4 层匹配失败时，allocator 优先调用 AgentFactory，再降级为 Spirit fallback。两条路径：

```go
// SubTask 路径
allocation, err := impl.matchSubTask(ctx, subTask, capabilities, traceID)
if err != nil {
    // P1-4: 4-layer matching failed → try AgentFactory before fallback.
    if factoryAlloc, ok := impl.tryAgentFactoryForSubTask(ctx, subTask, traceID); ok {
        allocations = append(allocations, factoryAlloc)
        continue
    }
    allocation = impl.fallbackAllocation(subTask, capabilities)
}

// Whole-plan 路径（无 subtasks 时）
allocation, err := impl.matchWholePlan(ctx, taskPlan, capabilities, traceID)
if err != nil {
    if factoryAlloc, ok := impl.tryAgentFactoryForPlan(ctx, taskPlan, traceID); ok {
        allocations = append(allocations, factoryAlloc)
    } else {
        allocation = impl.fallbackWholePlanAllocation(taskPlan, capabilities)
        allocations = append(allocations, allocation)
    }
}
```

`tryAgentFactoryForSubTask`/`tryAgentFactoryForPlan` 契约：
- AgentFactory 为 nil 或 `EnsureAgent` 失败 → 返回 `(zero, false)`，caller 继续降级
- 成功 → 返回 `TaskAllocation{MatchLayer: "factory", MatchReason: "AgentFactory 动态创建"}`，便于观测区分
- `TaskProfile` 构造：SubTask 路径用 `subTask.RequiredCapabilities` + `subTask.Description`（空则用 Name）；Whole-plan 路径用 `extractCapabilityHints(taskPlan.UserMessage)` + `taskPlan.UserMessage`

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

> P2-2 已落地（Wave 4）。实际实现与原设计的差异：
> - **4 种 ReplanType 对齐**：`ReplanRetry` / `ReplanReroute` / `ReplanInsertFallback` / `ReplanRebuildSubgraph` 与原设计一致
> - **规则化失败分析（无 LLM）**：`analyzeFailure` 基于错误类型规则匹配（transient/agent_incapable/subtask_invalid/blocked_downstream），YAGNI 原则——不引入 LLM 决策，避免运行时延迟与成本
> - **`sync.Mutex` 保护 attemptCount**：`map[string]int` 按 execID 计数，并发安全（红线 #21）
> - **`maxReplanAttempts` 上限**：单 execution 单节点最多重规划 3 次，超限返回 `apierror.CodeTooManyRequests`
> - **apierror 错误模型**：所有业务错误经 `apierror.New(...)` 返回（红线 #14），不使用 `fmt.Errorf`
> - **事件发布**：重规划决策通过 `ReplanBus.Publish` 发布 `ReplanEnvelope`，由 executor 订阅执行
> - **18 个测试**：含 `TestRuntimeReplanner_ConcurrentAccess` 并发测试（8 goroutine × maxReplanAttempts，`-race` 通过，BD5）

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
// internal/telemetry/turntrace/bridge.go 新增（P3-2 落地）
type Bridge struct {
    mu       sync.Mutex
    domain   Domain
    root     trace.Span
    llm      trace.Span
    tool     map[string]trace.Span
    plan     trace.Span // PhasePlan 阶段 span（root 的 child）
    alloc    trace.Span // PhaseAlloc 阶段 span
    orch     trace.Span // PhaseOrch 阶段 span
    finished bool
}

// 阶段名常量
const (
    PhasePlan  = "plan"
    PhaseAlloc = "alloc"
    PhaseOrch  = "orch"
)

// Start 创建 turn root span 并通过 WithBridge 注入 ctx
func Start(ctx context.Context, cfg Config) (context.Context, *Bridge, trace.Span)

// StartPhase 开启阶段 span（root 的 child），返回带 span 的 ctx；nil-safe
func (b *Bridge) StartPhase(ctx context.Context, phase string, attrs ...attribute.KeyValue) (context.Context, trace.Span)

// EndPhase 结束阶段 span 并记录错误状态（nil-safe，safe for non-started phases）
func (b *Bridge) EndPhase(phase string, err error)
```

**错误传播修复（CQ-1）**：

`executePlanPhase`/`executeAllocatePhase`/`Orchestrate` 改为命名返回值 + `defer EndPhase`，确保 panic/early-return 路径下 span 也能正确记录错误状态：

```go
// internal/tools/spirit_tools.go
func (e *SpiritToolsExecutor) executePlanPhase(...) (plan *biz.TaskPlan, step biztypes.OrchestrationStepRecord, err error) {
    bridge := turntrace.FromContext(ctx)
    ctx, _ = bridge.StartPhase(ctx, turntrace.PhasePlan)
    defer func() { bridge.EndPhase(turntrace.PhasePlan, err) }()

    plan, err = e.planner.Plan(ctx, ...)  // 注意：用 = 而非 := 避免 shadowing 命名返回
    if err != nil { return nil, step, err }
    // ...
    return plan, step, nil
}
```

> 关键：`:=` 会 shadow 命名返回值，导致 `defer` 中的 `err` 永远为 nil。改为 `=` 赋值后，`defer EndPhase(phase, err)` 能正确捕获实际错误。

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

**实现差异（Wave 1 落地）**：

1. **指标命名前缀**：实际命名为 `aranea_spirit_plan_duration_seconds` 等（加 `aranea_` 前缀），符合 Prometheus 命名规范，与项目既有指标一致。

2. **buckets 调整（审查修复）**：`SpiritPlanDuration`/`SpiritAllocDuration` 从 `prometheus.DefBuckets`（max 10s）改为 `spiritPhaseBuckets = []float64{0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300}`（max 300s=5min）。理由：Plan/Alloc 阶段可能调用 LLM，耗时超过 10s，DefBuckets 无法覆盖；新 buckets 匹配"multi-minute"注释声明。`SpiritOrchDuration` 保留独立 buckets（1s-3600s）覆盖长任务子阶段。

3. **`spiritPhaseBuckets` 命名常量**：抽取为包级常量复用，避免重复定义。

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

> P2-4 已落地（Wave 4）。实际实现与原设计的差异：
> - **safego.Go 替代裸 goroutine**：`executeLayer` 内部用 `safego.Go(ctx, "parallel_tool.exec", fn)` 替代 `go func()`（红线 #13，保证 panic 恢复 + ctx 取消传播）
> - **信号量限流**：新增 `semaphore` 字段（`chan struct{}`，容量 `maxConcurrency`），在 goroutine 启动前 acquire、退出时 release，防止突发 layer 耗尽 goroutine
> - **预分配 results 切片**：`results := make([]ToolResult, len(calls))` 按索引写入，避免 `append` 竞态（无需 `sync.Mutex` 保护）
> - **ctx 取消传播**：`safego.Go` 内部检查 `ctx.Err()`，ctx 取消时未启动的 goroutine 跳过执行
> - **WorktreeIsolator 错误日志化**：审查修复（红线 #22）—— `mergeWorktree`/`removeWorktree` 中的 `runGit` 错误不再用 `_ =` 吞掉，改为 `lg.Warn` 记录（best-effort 清理，不阻断主流程）

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
    // 新增（指针类型，nil 表示未设置）
    ValidFrom  *time.Time `json:"valid_from,omitempty"`
    ValidUntil *time.Time `json:"valid_until,omitempty"`
}

type Entry struct {
    // ... 既有字段
    ValidFrom  *time.Time `json:"valid_from,omitempty"`
    ValidUntil *time.Time `json:"valid_until,omitempty"`
}
```

**SQLite 迁移**（`internal/data/sql/migrations/20260725_memory_bitemporal.sql`，版本 20260725）：

```sql
-- 实际表名为 memory_facts（非 memories），列类型为 TEXT（非 TIMESTAMP），
-- 与 SQLite 既有 schema 一致。空字符串表示未设置。
ALTER TABLE memory_facts ADD COLUMN valid_from TEXT NOT NULL DEFAULT '';
ALTER TABLE memory_facts ADD COLUMN valid_until TEXT NOT NULL DEFAULT '';

-- 部分索引：仅索引当前有效记录，加速 SearchMemories 默认过滤
CREATE INDEX IF NOT EXISTS idx_memory_facts_valid_until
  ON memory_facts(valid_until)
  WHERE valid_until = '';
```

**冲突检测与失效**（`internal/memory/trpc/sqlite_adapter.go`）：

- 写入时检测同 key 冲突 → 旧记录 `valid_until = now`（不删除，保留历史）→ 新记录 `valid_from = now` 写入
- `InvalidateFact(ctx, key)` 显式失效单条记录
- `SearchMemories` 默认过滤：`WHERE valid_until = '' OR valid_until > now()`（仅返回当前有效记忆）
- 历史重建查询：显式传入 `includeInvalidated=true` 跳过过滤，按 `valid_from`/`valid_until` 重建时间线

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

**实现差异（Wave 1 落地）**：

1. **队列解耦**：实际实现将 `EnqueueConsolidationJob`（入队）与 `Consolidate`（执行）解耦。`EnqueueConsolidationJob` 将 `ConsolidationJobRequest` 发送到 `ConsolidationQueue`（buffered channel），`Consolidate` 由 `MemorySleepTimeWorker` cron job 出队后执行。设计理由：避免阻塞调用方，支持后台异步整理。

2. **三阶段非原子**：`Consolidate` 的 read→LLM→execute 三阶段非原子。`executeOperations` 对每个 op 失败采用 best-effort（log + continue）而非 fail-fast，使单 op 失败不阻塞后续 op。文档明确"consolidation 是最终一致而非强一致"。

3. **未知 op 类型日志**：`executeOperations` 的 switch 增加 `default` 分支，记录未知 op 类型（`loggateway.Warn` + `loggateway.Str("op_type", op.Type)`），便于发现 LLM prompt/response 契约漂移。

4. **LLM 失败优雅降级**：`Consolidate` 对 LLM 失败（API 错误、malformed JSON、nil LLM）统一 `lg.Warn` + 返回 `nil`，不修改 memory。memory read 失败则返回错误（不降级），区分"基础设施失败"与"LLM 失败"。

5. **`AgentUserKeyLister` placeholder**：当前 `AgentUserKeyLister` 返回空列表（开发/测试用），生产启用前需实现从 `SessionRepo` 派生活跃用户。

6. **未完成项**：`MemorySleepTimeWorker` 未通过 Wire 绑定到 cronrunner 调度器（死代码）；失败 job 无重试无死信（对比 `AutoMemoryWorker.processWithRetry`）。生产启用前需补全。

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

> P3-11 已落地（Wave 4）。实际实现与原设计的差异：
> - **签名增加 UserKey**：`ProactiveRecall(ctx, userKey UserKey, convCtx ConversationContext) ([]*Entry, error)` —— 多租户场景必须区分 agent/user，原设计遗漏此参数
> - **biz 端口分离**：`internal/biz/memory_composite_recall.go` 定义 `ProactiveRecaller` 接口（用 `agentID/userID + ProactiveRecallContext`），避免 biz 直接依赖框架类型（红线 #2）
> - **适配器模式**：`ProactiveRecallAdapter` 桥接框架 `Service.ProactiveRecall` 与 biz `ProactiveRecaller.ProactiveRecall`（Go 不允许同类型同名方法，签名又不同）
> - **可选依赖 + Setter 注入**：`SetProactiveRecaller` 后置注入，避免破坏 `NewMemoryCompositeRecallUsecase` 现有签名（向后兼容）
> - **Bi-temporal 过滤集成**：跳过 `ValidUntil.Before(time.Now())` 的失效记忆（P3-8 联动）
> - **矛盾检测**：`extractKeywords` 分词 + `hasKeywordOverlap` 命中时 `Score += 0.1`，优先暴露潜在冲突
> - **查询上限**：`proactiveRecallMaxQueries=8` 防止 DB 过载
> - **错误降级**：单 query 搜索失败 `lg.Warn` + continue，不中断整体召回
> - **in-memory 框架实现**：`pkg/trpc-agent-go/memory/inmemory/service.go` 提供测试用简化版（无 Bi-temporal/矛盾检测）

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

> P3-4 已落地（Wave 4）。实际实现与原设计的差异：
> - **错误码映射集中化**：`web/src/features/chat/errorCodeHints.ts` 统一管理 17 个错误码（9 TurnErrorCode + 8 ApiErrorCode）到 `ErrorAction` 的映射，组件不内联硬编码
> - **6 个 emit 事件**：`retry` / `switch-model` / `rephrase` / `dismiss` / `report` / `ignore`，覆盖所有错误动作
> - **条件按钮渲染**：`getErrorAction()` 返回 action 后，按钮按 action 类型条件渲染（无 action 时隐藏整个 actions 区）
> - **i18n 完整覆盖**：`errorBlock.*` 命名空间 17 个 key（zh-CN + en-US 双语）
> - **未实现 §10.2 编排时间线视图**：原设计的 `OrchestrationTimeline.vue` 不在 Wave 4 范围内，留待后续

### 10.2 WS 断连快速检测

> P3-5 已落地（Wave 4）。原设计未单独描述 WS 断连检测，实际实现如下：
> - **`useChatStreamManager.ts`**：新增 `isStale` ref + `resetStaleTimer()` + `onHeartbeat()` / `onStale()` 回调；心跳超时（默认 30s）触发 `isStale = true`
> - **`recover()` 方法**：手动重连入口，重置 stale 状态 + 重新建立 WS 连接
> - **`ChatMessagePanel.vue`**：新增 `isStale` prop + `recover` emit + stale banner UI（顶部黄色提示条 + "重新连接" 按钮）
> - **`ChatPage.vue`**：`:is-stale` 绑定 + `@recover` 处理器 + `onRecover()` 调用 stream manager
> - **i18n**：`wsStale.*` 命名空间 4 个 key（title/message/recover/dismiss）

### 10.3 编排时间线视图

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

> 注：编排时间线视图不在 Wave 4 范围内，保留原设计待后续实现。

---

## 十一、数据模型变更

### 11.1 新增 Ent Schema

```go
// internal/data/ent/schema/agent.go 已有 source 字段
field.Enum("source").Values("user", "system", "imported").Default("user").Comment("agent source: origin tracking (user | system | imported), aligned with team.source"),
```

> 注：动态创建的 Agent 标记为 `source="system"`（非 "dynamic"），与现有 enum 值对齐。详见 `internal/biz/agent_types.go:121`。

### 11.2 Memory 表新增列

> 注：实际表名为 `memory_facts`（非 `memories`），Bi-temporal 列（P3-8）类型为 `TEXT NOT NULL DEFAULT ''`（非 `TIMESTAMP`），与 SQLite 既有 schema 一致。Ebbinghaus 相关列（P3-9）尚未实现，保留原设计。链接图相关列（P3-12）已落地，类型为 `TEXT NOT NULL DEFAULT '[]'`（SQLite 无原生 JSON 类型，用 TEXT 存储 JSON 字符串）。

```sql
-- P3-8 已落地（internal/data/sql/migrations/20260725_memory_bitemporal.sql）
ALTER TABLE memory_facts ADD COLUMN valid_from TEXT NOT NULL DEFAULT '';
ALTER TABLE memory_facts ADD COLUMN valid_until TEXT NOT NULL DEFAULT '';

-- P3-9 待落地（保留原设计，类型待 SQLite 适配）
ALTER TABLE memory_facts ADD COLUMN access_count INTEGER DEFAULT 0;
ALTER TABLE memory_facts ADD COLUMN last_accessed_at TIMESTAMP;
ALTER TABLE memory_facts ADD COLUMN decay_score FLOAT DEFAULT 1.0;

-- P3-12 已落地（internal/data/sql/migrations/20260726_memory_links.sql）
-- SQLite 无原生 JSON 类型，用 TEXT 存储 JSON 字符串
ALTER TABLE memory_facts ADD COLUMN links TEXT NOT NULL DEFAULT '[]';
ALTER TABLE memory_facts ADD COLUMN keywords TEXT NOT NULL DEFAULT '[]';
ALTER TABLE memory_facts ADD COLUMN tags TEXT NOT NULL DEFAULT '[]';
```

### 11.3 新增事件表

> P1-6 已落地（`internal/event/postgres_eventstore.go::EnsureSchema`），实际 schema 与原设计略有差异：增加 `run_id` 列、`event_id` 唯一索引、`created_at` 改为 `TIMESTAMPTZ`。

```sql
-- 实际实现（PostgresEventStore.EnsureSchema，幂等 CREATE IF NOT EXISTS）
CREATE TABLE IF NOT EXISTS event_store (
    id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(64) NOT NULL,
    session_id VARCHAR(64) NOT NULL,
    run_id VARCHAR(64),
    envelope_type VARCHAR(64) NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_event_store_event_id ON event_store (event_id);
CREATE INDEX IF NOT EXISTS idx_event_store_session_created ON event_store (session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_event_store_type ON event_store (envelope_type);
```

- `event_id` 唯一索引保证 `Save` 幂等（`ON CONFLICT DO NOTHING`）
- `run_id` 可选列，便于按 run 维度查询
- `created_at` 用 `TIMESTAMPTZ` 带时区，避免跨时区问题

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

**AS-EVT-01 可靠性分级（Wave 1 落地）**：

4 个 Wave 1 预注册事件类型已在 `internal/event/contract/reliability.go` 中完成 AS-EVT-01 分级注册：

| 事件类型 | 分级 | 理由 | 持久化 |
|---------|------|------|--------|
| `EnvelopeTypeRunHeartbeat` | Informational | 丢失仅降低进度可见性，不破坏状态 | 不持久化 |
| `EnvelopeTypeAgentCreated` | Informational | Agent 已落库，事件仅驱动 UI | 不持久化 |
| `EnvelopeTypeGraphReplanned` | Important | 拓扑漂移防护，不可静默丢弃 | 异步持久化 |
| `EnvelopeTypeGraphTopologyEvolved` | Important | 拓扑漂移防护，不可静默丢弃 | 异步持久化 |

**审查修复（Wave 1）**：初始实现中 `GraphReplanned`/`GraphTopologyEvolved` 的注释声明为 Important 但未在 `reliability.go` 注册，实际为 Informational（默认）。aranea-review 第一轮审查发现此阻断项（B1），已在 `reliability.go` 的 `RegisterBulk(reliability.Important, ...)` 中追加注册。新增 `TestReliabilityClassification` 测试验证分级一致性。

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

---

## 十五、记忆神经元模型增强设计

> 基于 DeepSeek 神经元式记忆数据库方案，渐进增强现有 L4 实体图谱。核心思路：**memory_entities = 神经元节点，memory_relations = 突触关系**，在现有表上增加激活值/共同激活计数字段，新增扩散激活检索引擎，不重构存储引擎。

### 15.1 现有表结构盘点

**memory_entities**（[memory_chain.sql:415](../../internal/data/sql/memory_chain.sql#L415)）已有字段：

| 字段 | 类型 | 神经元语义 |
|------|------|-----------|
| id | TEXT PK | 神经元唯一标识 |
| entity_type | TEXT | 神经元类型（person/place/preference/event/concept） |
| importance | REAL | 重要性（现有） |
| confidence | REAL | 置信度（现有） |
| use_count | INTEGER | 访问次数（现有，≈ access_count） |
| source_kind | TEXT | 来源（'extracted'，现有） |
| embedding_blob | BLOB | 文本向量（现有） |
| status | TEXT | 状态（active/archived/deleted） |

**memory_relations**（[memory_chain.sql:446](../../internal/data/sql/memory_chain.sql#L446)）已有字段：

| 字段 | 类型 | 突触语义 |
|------|------|---------|
| source_id | TEXT | 突触前神经元 |
| target_id | TEXT | 突触后神经元 |
| relation_type | TEXT | 关系类型（现有，自由值） |
| bidirectional | INTEGER | 是否双向 |
| **weight** | **REAL DEFAULT 1.0** | **连接强度（现有！）** |
| confidence | REAL | 关系置信度 |
| use_count | INTEGER | 使用次数 |
| valid_from / valid_to | TEXT | Bi-temporal 有效区间 |

**关键发现**：`weight` 字段已存在（默认 1.0），`relation_type` 已为自由值 TEXT，`use_count` 已有。**无需重构存储**，只需增量加字段。

### 15.2 神经元字段增强（DDL 迁移）

```sql
-- internal/data/sql/migrations/20261005_memory_neuron_enhancement.sql
-- 版本 20261005：神经元模型字段增强（20260728 已被 memory_job_deadletter_schema 占用）

-- memory_entities 新增激活值与来源类型
ALTER TABLE memory_entities ADD COLUMN activation REAL NOT NULL DEFAULT 0;
ALTER TABLE memory_entities ADD COLUMN activation_updated_at TEXT NOT NULL DEFAULT '';
ALTER TABLE memory_entities ADD COLUMN source_type TEXT NOT NULL DEFAULT '';
  -- source_type: perception/inference/told/knowledge（区分现有 source_kind='extracted'）
ALTER TABLE memory_entities ADD COLUMN valence REAL NOT NULL DEFAULT 0;   -- 预留：情感效价
ALTER TABLE memory_entities ADD COLUMN arousal REAL NOT NULL DEFAULT 0;   -- 预留：情感唤醒度

-- memory_relations 新增共同激活计数与强化时间
ALTER TABLE memory_relations ADD COLUMN co_activation_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE memory_relations ADD COLUMN last_reinforced_at TEXT NOT NULL DEFAULT '';
ALTER TABLE memory_relations ADD COLUMN context_note TEXT NOT NULL DEFAULT '';

-- 扩散激活索引：按激活值降序检索 Top-K
CREATE INDEX IF NOT EXISTS idx_memory_entities_activation
  ON memory_entities(scope_type, scope_id, status, activation DESC);

-- 关系联合索引：图遍历加速
CREATE INDEX IF NOT EXISTS idx_memory_relations_graph
  ON memory_relations(source_id, target_id, status, weight DESC);
```

**迁移注册**：在 `internal/data/ddl_migration_registry.go` 注册版本 20261005（最新可用版本号，20260728 已被占用）。

### 15.3 关系类型扩展

现有 `relation_type` 为自由值 TEXT（无枚举约束），扩展类型定义在应用层：

```go
// internal/biz/memory_l4.go 扩展
const (
    RelationRelatedTo   = "RELATED_TO"     // 现有：双向关联
    RelationEvolvedFrom = "EVOLVED_FROM"   // 现有：有向，新记忆替代旧记忆
    RelationCausal      = "CAUSAL"         // 新增：有向，因果关系（A 导致 B）
    RelationTemporalNext = "TEMPORAL_NEXT" // 新增：有向，时序关系（B 在 A 之后）
    RelationInhibit     = "INHIBIT"        // 新增：有向，抑制关系（冲突消解）
)

// relationTypeProps 定义每种关系类型的属性（方向性、是否强化目标、是否抑制）
var relationTypeProps = map[string]RelationTypeProp{
    RelationRelatedTo:    {Bidirectional: true,  ReinforcesTarget: true,  InhibitsTarget: false},
    RelationEvolvedFrom:  {Bidirectional: false, ReinforcesTarget: false, InhibitsTarget: true},
    RelationCausal:       {Bidirectional: false, ReinforcesTarget: true,  InhibitsTarget: false},
    RelationTemporalNext: {Bidirectional: false, ReinforcesTarget: false, InhibitsTarget: false},
    RelationInhibit:      {Bidirectional: false, ReinforcesTarget: false, InhibitsTarget: true},
}
```

### 15.4 递归 CTE 图遍历（替换现有应用层 BFS）

**现有实现**（[memory_shim_l4.go:99-232](../../internal/data/memory_shim_l4.go#L99)）：应用层逐跳 BFS，每跳 2 次 SQL 查询（relations + entities），N 跳 = 2N 次往返。

**新设计**：单次递归 CTE 完成全图遍历，应用层只做加权传播计算。

```sql
-- PostgreSQL 递归 CTE（Postgres 环境）
WITH RECURSIVE graph_traverse AS (
  -- 种子：中心节点
  SELECT source_id AS node_id, 0 AS hop, 1.0 AS activation
  FROM memory_relations
  WHERE source_id = $1 AND status = 'active'
  UNION ALL
  -- 递归：沿关系传播
  SELECT
    CASE WHEN r.source_id = gt.node_id THEN r.target_id ELSE r.source_id END AS node_id,
    gt.hop + 1 AS hop,
    gt.activation * r.weight AS activation
  FROM graph_traverse gt
  JOIN memory_relations r ON
    (r.source_id = gt.node_id OR r.target_id = gt.node_id)
    AND r.status = 'active'
  WHERE gt.hop < $2  -- 最大跳数
)
SELECT DISTINCT ON (node_id) node_id, hop, activation
FROM graph_traverse
ORDER BY node_id, activation DESC
LIMIT $3;  -- Top-K
```

**SQLite 兼容**：SQLite 同样支持 `WITH RECURSIVE` 语法，但占位符用 `?` 而非 `$N`。通过 `Dialect().RenumberPlaceholders()` 适配（现有模式）。

**性能对比**：

| 指标 | 现有应用层 BFS | 递归 CTE |
|------|--------------|---------|
| SQL 往返（3 跳） | 6 次 | 1 次 |
| 网络延迟（3 跳） | 6×RTT | 1×RTT |
| 应用层逻辑 | 逐跳循环+去重 | 纯结果处理 |
| 加权传播 | 无法在 SQL 做 | SQL 内完成 |

### 15.5 扩散激活算法（Go 应用层）

递归 CTE 返回图结构后，Go 应用层执行加权传播 + Top-K 剪枝：

```go
// internal/memory/spreading_activation.go 新增
type SpreadingActivationEngine struct {
    l4Repo  biz.L4EntityStore
    lg      loggateway.Logger
}

type ActivationResult struct {
    NodeID       string
    Activation   float64
    HopCount     int
    ActivationPath []PathStep // 可解释激活路径
}

type PathStep struct {
    FromNodeID string
    ToNodeID   string
    EdgeWeight float64
    RelationType string
}

// SpreadingActivation 执行扩散激活检索
// centerID: 线索神经元 ID
// hops: 最大传播跳数（默认 3）
// topK: 每跳保留的最大节点数（默认 20）
func (e *SpreadingActivationEngine) SpreadingActivation(
    ctx context.Context, centerID string, hops, topK int,
) ([]ActivationResult, error) {
    // 1. 递归 CTE 获取 N 跳内所有节点+边
    graph, err := e.l4Repo.GraphTraverseCTE(ctx, centerID, hops, topK*2)
    if err != nil {
        return nil, entErrToBizErr(err, "MEMORY_L4")
    }

    // 2. 初始化：center 节点 activation = 1.0
    activations := map[string]float64{centerID: 1.0}
    paths := map[string][]PathStep{} // 每个节点的激活路径

    // 3. 逐跳传播
    for hop := 1; hop <= hops; hop++ {
        nextActivations := map[string]float64{}
        for nodeID, activation := range activations {
            if activation < 0.01 {
                continue // 低于阈值的节点不传播
            }
            // 获取该节点的邻居
            neighbors := graph.Neighbors(nodeID)
            for _, edge := range neighbors {
                propagated := activation * edge.Weight * decayFactor(hop)
                target := edge.TargetID
                if target == nodeID {
                    target = edge.SourceID
                }
                // 累加激活值（多条路径汇聚）
                nextActivations[target] += propagated
                // 记录激活路径
                paths[target] = append(paths[target], PathStep{
                    FromNodeID: nodeID, ToNodeID: target,
                    EdgeWeight: edge.Weight, RelationType: edge.RelationType,
                })
            }
        }
        // Top-K 剪枝：每跳只保留激活值最高的 K 个节点
        activations = topKFilter(nextActivations, topK)
    }

    // 4. 构建结果（按激活值降序）
    results := buildActivationResults(activations, paths)
    return results, nil
}

// decayFactor 跳数衰减因子（模拟信号衰减）
func decayFactor(hop int) float64 {
    return math.Pow(0.7, float64(hop-1)) // hop1=1.0, hop2=0.7, hop3=0.49
}
```

### 15.6 赫布权重更新规则

"一起放电，连在一起"——同时激活的神经元之间增强连接权重：

```go
// internal/memory/hebbian_update.go 新增
type HebbianUpdater struct {
    l4Repo biz.L4EntityWriter
    lg     loggateway.Logger
}

// ReinforceConnection 赫布规则强化连接
// 当两个神经元在同一上下文中被同时激活时调用
func (u *HebbianUpdater) ReinforceConnection(
    ctx context.Context, nodeA, nodeB string, relationType string,
) error {
    // 1. 查找现有关系
    rel, err := u.l4Repo.FindRelation(ctx, nodeA, nodeB, relationType)
    if err != nil {
        return err
    }

    // 2. 赫布规则：Δw = η * pre_activation * post_activation
    //    η = learning_rate（默认 0.1）
    //    pre/post_activation 从 memory_entities.activation 读取
    learningRate := 0.1
    newWeight := rel.Weight + learningRate*rel.SourceActivation*rel.TargetActivation

    // 3. 权重饱和保护（0~1）
    if newWeight > 1.0 {
        newWeight = 1.0
    }

    // 4. 更新 weight + co_activation_count + last_reinforced_at
    return u.l4Repo.UpdateRelationWeight(ctx, rel.ID, newWeight,
        rel.CoActivationCount+1, time.Now().UTC().Format(time.RFC3339Nano))
}

// DecayUnused 长期未使用的连接权重衰减（配合 Ebbinghaus）
func (u *HebbianUpdater) DecayUnused(ctx context.Context, threshold time.Duration) error {
    // 查找 last_reinforced_at < threshold 的关系
    // weight *= 0.95（衰减因子）
    // weight < 0.1 时标记 status = 'archived'（弱连接归档）
}
```

### 15.7 记忆重巩固机制

回忆时神经元属性可被更新（模拟人脑记忆重巩固）：

```go
// internal/memory/reconsolidation.go 新增
type ReconsolidationService struct {
    l4Repo      biz.L4EntityWriter
    hebbian     *HebbianUpdater
    lg          loggateway.Logger
}

// OnRecall 当神经元被召回时触发重巩固
func (s *ReconsolidationService) OnRecall(
    ctx context.Context, nodeID string, recalledWith []string,
) error {
    // 1. 提升被召回神经元的 activation
    newActivation := math.Min(1.0, currentActivation+0.2)
    s.l4Repo.UpdateActivation(ctx, nodeID, newActivation, time.Now())

    // 2. 增加访问计数（use_count + 1）
    s.l4Repo.IncrementUseCount(ctx, nodeID)

    // 3. 对同时召回的神经元执行赫布强化
    for _, otherID := range recalledWith {
        s.hebbian.ReinforceConnection(ctx, nodeID, otherID, RelationRelatedTo)
    }

    return nil
}
```

**集成点**：在 [memory_inject.go](../../internal/agent/memory_inject.go) 的 BeforeModel 钩子中，召回记忆后异步触发 `OnRecall`（不阻塞模型调用）。

### 15.8 冲突消解策略

新记忆与旧记忆矛盾时建立 INHIBIT 抑制关系：

```go
// internal/memory/conflict_resolver.go 新增
type ConflictResolver struct {
    l4Repo  biz.L4EntityWriter
    lg      loggateway.Logger
}

// ResolveConflict 当检测到新记忆与旧记忆矛盾时调用
func (r *ConflictResolver) ResolveConflict(
    ctx context.Context, newEntityID, oldEntityID string, conflictReason string,
) error {
    // 1. 建立 INHIBIT 关系（新记忆 → 抑制 → 旧记忆）
    err := r.l4Repo.CreateRelation(ctx, biz.RelationInsert{
        SourceID:    newEntityID,
        TargetID:    oldEntityID,
        RelationType: RelationInhibit,
        Weight:      0.8, // 强抑制
        ContextNote: conflictReason,
        ValidFrom:   time.Now().UTC().Format(time.RFC3339Nano),
    })
    if err != nil {
        return err
    }

    // 2. 降低旧记忆的 confidence（不删除，保留历史）
    return r.l4Repo.AdjustConfidence(ctx, oldEntityID, -0.3)
}

// 检索时过滤被抑制的记忆
// SpreadingActivationEngine 中，遇到 INHIBIT 关系时：
//   - 不传播激活值到被抑制的节点
//   - 或传播负向激活值（降低目标节点激活值）
```

### 15.9 知识库协同设计

知识库（37 号模块）与记忆（70 号模块）的协同关系：

```go
// memory_entities 新增可选字段（通过 metadata_json 存储，不改表结构）
// metadata_json.source_collection_id: 标记实体来源的知识库 Collection

// 知识库检索结果触发记忆强化（FR-11.7）
func (s *KnowledgeMemoryBridge) OnKnowledgeConfirmed(
    ctx context.Context, collectionID string, query string, confirmed bool,
) error {
    // 用户确认的知识库检索结果 → 提升 L4 相关实体的 confidence
    entities := s.l4Repo.FindBySourceCollection(ctx, collectionID)
    for _, entity := range entities {
        if confirmed {
            s.l4Repo.AdjustConfidence(ctx, entity.ID, +0.1)
        } else {
            s.l4Repo.AdjustConfidence(ctx, entity.ID, -0.1)
        }
    }
    return nil
}
```

**边界约束**：
- 知识库块（knowledge_chunks）**不进入** memory_entities
- 记忆事实（memory_facts）**不进入** knowledge_chunks
- 协同仅通过 `source_collection_id` 引用 + confidence 调整

### 15.10 迁移路径（何时切换图数据库）

**触发条件**（满足任一即可评估迁移）：

| 条件 | 阈值 | 理由 |
|------|------|------|
| 单 Agent 图谱节点数 | >10万 | 递归 CTE 性能下降 |
| 图遍历深度 | >5 跳 | 递归 CTE 指数级膨胀 |
| 图算法需求 | PageRank/社区发现 | SQL 无法高效实现 |
| 跨 Agent 全局推理 | 需要 | 需要全局图视图 |

**迁移目标**：
- **Neo4j**：原生图存储，亿级节点，Cypher 查询，适合大规模
- **Apache AGE**：PG 图扩展，保持单 PG 部署，适合中等规模（注意 Windows 支持）

**兼容性**：本期数据模型（memory_entities 节点 + memory_relations 边）与图数据库的节点/边模型完全兼容，迁移时只需导出数据 + 转换为 Cypher 导入格式。

### 15.11 性能预算

| 操作 | 目标延迟 | 实现方式 |
|------|---------|---------|
| 扩散激活 1 跳 | <20ms | 递归 CTE + Top-20 剪枝 |
| 扩散激活 3 跳 | <100ms | 递归 CTE + Top-20 剪枝 |
| 赫布权重更新 | <10ms | 单行 UPDATE |
| 记忆重巩固 | <20ms | 2 行 UPDATE（activation + use_count） |
| 冲突消解 | <30ms | 1 INSERT + 1 UPDATE |

**容量假设**：单 Agent 图谱 ≤10万节点、≤50万边。超过此规模触发图数据库迁移评估。
