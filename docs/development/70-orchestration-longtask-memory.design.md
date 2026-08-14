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

**R5 补充（2026-08-01，vector 基础设施错误处理）**：

记忆域 vector 调用（`internal/data/memory.go`）此前将 `internal/data/vector` 包的原始 `fmt.Errorf` 直接透传至 biz 层，违反 DB-R5。修复方案：

- **可用性门控**：`memoryRepo.storeFor` 先检查 `vector.IsPgvector()`，非 pgvector 环境（如 SQLite 测试）直接返回 `biz.ErrMemoryUnavailable`，不再尝试构建 store 后失败
- **统一翻译**：所有 vector store 调用（Insert/FindSimilar/FindSimilarWithUser/UpsertFactVector）的错误经 `entErrToBizErr(err, "MEMORY")` 翻译后返回，消除原始基础设施错误泄漏

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

### 3.5.1 Phase 1/2 规划恢复（P1-10，2026-08-15）

进程重启后 `SessionStatusGuard.OnStartup` → `TaskOrchestratorImpl.RecoverAllInterrupted` 原先只把 `OrchestrationHandle` 从 checkpoint 标回 running，**不装回**已持久化的 draft `TaskPlan` / `AllocationPlan`。当前 `plan_and_execute` 又不落库 handle（Phase 3 委托 PlanExecutor），长任务中断后下一次规划会重新跑 LLM 分解。

**恢复路径**（不改变未中断的 `Plan()` 语义）：

```
启动
  RecoverAllInterrupted (system workspace)
    ├─ List interrupted OrchestrationHandle → Recover(checkpoint) → 按 handle.TaskPlanID 装回 plan
    └─ ListByStatuses(draft/approved/confirmed/executing) 装回无 handle 的孤儿 draft
  缓存 keyed by spirit_session_id
续跑 plan_and_execute
  ConsumeRecoveredPlan(session, user_message) 命中 → 跳过 TaskPlanner.Plan /（若有）Allocate
  未命中 → 原路径 Plan()
```

**可恢复**：`task_plans` 行状态 ∈ {draft, approved, confirmed, executing}，且：
- `direct`（或空 strategy）：允许无 SubTasks
- 团队策略（parallel/dag/coordinator/single_agent）：必须已有 SubTasks（分解已完成）

**明确不恢复（skip + 结构化日志 `reason=`，不假装 recovered）**：

| reason | 范围 |
|--------|------|
| `incomplete_draft_no_subtasks` | 分解中途崩溃：draft 已落库但 SubTasks 为空，恢复需重跑 LLM，本能力不做 |
| `task_plan_missing` / `no_task_plan_id` | handle 无对应 plan 行 |
| `terminal_status` | completed/failed |
| `allocation_get_failed` | Phase 2 行缺失：plan 仍恢复，Allocate 在续跑时重做（可能有 cold-start LLM） |

**代码锚点**：`internal/agent/task_orchestrator_plan_recovery.go`、`RecoverAllInterrupted`、`biz.RecoveredPlanConsumer`、`internal/tools/spirit_tools.go` `consumeRecoveredPlan`

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

**行为契约补充（2026-07-27，会话 d78029b9 孤儿 notice 排查修复）**：

1. **续跑 turn 跳过门控** — `runPrePlanningGate` 对 `ParentTaskID` 非空的续跑 turn（synthesis 总结 turn / 澄清续答 turn）直接放行，与 `runClarificationGate` 同款防循环。复杂度在根 turn 已评估，续跑重评会重复发布门控 notice（单会话实测 7 对重复 = 2 根 turn + 2 澄清续跑 + 3 总结 turn），且 forcedPlanning 系统提示注入 synthesis turn 会强制其再走规划路径。
2. **门控 notice 挂接 Task** — `biz.PlanInput` 新增 `TaskID` 字段，由 `runPrePlanningGate` 从 ctx 取 `RootTaskActivityIDFromCtx`（turn 入口预解析的根 Task ID）填入；`publishPlanningPhase` 落 `Step.TaskID`。此前 notice 无 TaskID/TurnID 是 session 级孤儿步骤（前端 `getTaskOrphanSteps` 只认 TaskID，孤儿永不渲染且污染数据）；挂接后 notice 经 TaskCard orphanNoticeSteps 渲染为任务 footer。ctx 缺失时 TaskID 为空，退化为 session 级（行为同修复前，不阻断）。存量孤儿经 ~~`cmd/cleanup_orphan_notices`~~ 一次性清理（工具已执行完毕，2026-08-14 随死代码清理删除）。

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

> 打标点落地（2026-07-23，Graph Engineering 评审 Phase C）：`ToolCall.IsolationStrategy` 的统一分类点为 `tools.IsolationStrategyForTool(toolName)`——先经 `alias.RuntimeToolNameAliases` 归一化 UI 别名（`write_file→save_file`、`edit_file→diff_edit`），再匹配文件写工具集 `{save_file, diff_edit, patch_file, replace_content}` → `IsolationStrategyWorktree`；只读文件工具（`read_file`/`list_file`/`search_*`）与无关工具返回 `""` 直接执行。ToolCall 构造点统一经此函数打标，分类保持一致。E2E 验证 `TestBatchExecuteSpiritTools_ParallelWorktreeFileOps`：两个并发 `save_file` 各自在独立 worktree 提交不同文件，双双合并回主仓（首个 ff、次个 --no-ff）且 HEAD 前进。

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

### 9.6 召回透明度（memory_recalled notice）

> R4 修复（2026-08-01）：chat 场景下用户无法感知"本轮注入了哪些记忆"。设计目标——召回结果对用户可见，且不干扰聊天流。

**后端发射**（`internal/agent/memory_inject.go` BeforeModel 钩子）：

```go
// 召回结果注入 prompt 前，先发射 notice（best-effort，Informational 级）
ctx = emitMemoryRecalledNotice(ctx, result.RecallHits)

// notice payload（steps_v2, kind=notice, notice_type=memory_recalled）
{"hits":[{"layer":"L2","line":"...","score":0.5,"fact_id":"...","confidence":0.88,"version":3}]}
```

- **幂等去重**：ctx 标记 `memoryRecalledEmittedKey`，tool-loop 重入不重复发射
- **限额**：单 notice ≤10 条 hits，单条 line ≤120 runes（截断加省略号）
- **降级契约**：无 hits / 无 ActivityEmitter / 发射失败均静默跳过，绝不中断 turn（与 PinnedPreferenceCue 同级）

**前端渲染**：

| 组件 | 职责 |
|------|------|
| `web/src/features/chat/memoryRecall.ts` | `parseMemoryRecallHits` 解析 notice Content（JSON → `MemoryRecallHit[]`，非法输入返回空） |
| `activityV2Store.recallHitsByTurn` | `upsertStep` 时按 TurnID 索引 hits（created→completed 重放幂等覆盖） |
| `MemoryRecallChips.vue` | turn 顶部渲染 chips：层级 badge（L1-L4 配色）+ line + score%，tooltip 显示置信度/版本。**2026-08-08 修正**：从「turn 底部」移至「turn 顶部」——召回发生在 BeforeModel（turn 最开始），UI 顺序必须与实际执行顺序一致（召回 → 思考 → 行动 → 回复） |
| `noticeFilter.ts` | `memory_recalled` 加入 `SYSTEM_NOTICE_TYPES`，原始 JSON 不作为 NoticeBlock 渲染 |

**数据流**：BeforeModel 钩子 → `ActivityEmitter.EmitNotice` → steps_v2 落库 + WS 推送 → store 索引 → TurnContainer 顶部 chips（steps 之前）。

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

> **实现注记（2026-08-05）**：已按本节设计落地。实际实现较设计稿有两点收窄：① store 依赖从 `biz.L4EntityWriter`（Stable 不可扩展）收窄为新建的 `biz.L4ReconsolidationStore`（evolving，BoostActivation/IncrementUseCount 两方法）；② activation 饱和在 SQL 层原子完成（`CASE WHEN activation + delta > 1.0`），避免 Go 层 read-modify-write 竞态。触发侧由 `L4MemoryCue` 返回实际注入的实体 ID 集，`triggerL4Reconsolidation` 经 `safego.Go` 后台执行并以 ctx 标记保证每回合至多触发一次。

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

---

## 十六、记忆系统重设计（写入闭环）

> 源自《记忆系统整体设计评审与重设计方案》（[2026-07-29-review-memory-system-redesign.md](../reports/2026-07-29-review-memory-system-redesign.md)）。本节落地报告 §6.3 写入管线与 §6.7 P0/P1 的工程设计。实现状态见 [development.md §十八](./70-orchestration-longtask-memory.development.md#十八phase-m记忆系统重设计闭环p0-止血--p1-闭环重构)。

### 16.1 统一写入管线（P1-3）

**问题**：重设计前存在多条独立事实写入路径（auto_memory 内联冲突治理、episode consolidator 直写），操作语义与降噪策略各自为政，垃圾与重复事实无闸门。

**设计**：所有自动事实写入源归一化为 `FactWriteCandidate`，汇入单一管线 `FactWritePipeline.Apply`：

```
候选（candidate）
  → ① 降噪闸口（纯函数）：非空 statement + kind 白名单 + 置信度 ≥0.6
  → ② 邻域召回：embedding 相似邻居（≥0.80，上限 10），kind/statement 经 fact 行 enrichment
  → ③ 决策分流：
       非争议 → 启发式（同 kind 邻居 ≥0.92 → noop-merge；否则 add）
       争议   → 批次 LLM 裁决（邻居 ∈[0.80,0.92) 或跨 kind ≥0.92）
  → ④ 双时态写入执行
  → ⑤ 全量审计（memory_action_log）
```

**操作语义**（`FactWriteOperation`，对齐 Mem0 ADD/UPDATE/DELETE/NOOP）：

| 操作 | 执行 | 说明 |
|------|------|------|
| `add` | `UpsertFactRow` | 全新事实插入 |
| `update` | `InvalidateAndUpsertFactTx` | 取代旧事实：旧行 `valid_until=now` 失效 + 新行插入，**原子事务** |
| `delete` | `InvalidateFact` | 仅失效不替代——永不物理删除，双时态历史保留 |
| `noop` | 不写 / `IncrementFactAccessCount` | 两子类：LLM 宣告 noop（无存储价值）；去重合并（≥0.92 同 kind 邻居已承载，仅访问计数 +1） |

**降噪三闸**（`GateFactWriteCandidate` + 邻域判定）：

| 闸 | 规则 | 依据 |
|---|------|------|
| ① kind 白名单 | 仅 preference/profile/goal/constraint/decision/relationship 六类耐久事实入 L3；event/knowledge/fact 丢弃（「用户做了什么」归 L2 episode） | 认知分层语义边界 |
| ② 置信度下限 | `FactWriteMinConfidence = 0.6`；启发式正则提取器赋恰好 0.6 使高精度匹配通过 | Mem0 #4573 教训：无下限垃圾持续累积 |
| ③ 去重合并 | 同 kind 邻居余弦相似度 ≥0.92（`FactWriteMergeScore`）→ 合并不重复插入 | 与既有冲突 supersede 阈值一致 |

**争议带与 LLM 裁决**：邻居 ∈[`FactWriteContestedScore`=0.80, 0.92) 或跨 kind ≥0.92 的候选为「争议」，整批一次 LLM 调用裁决（`FactWriteAdjudicator.AdjudicateFactWrites`）。防御设计：
- 裁决目标必须 ∈ 该候选的邻居集合，否则降级 `add`（防幻觉/过期 id）
- 裁决器不可用/出错/漏判 → 回退启发式（争议在启发式下恒为 `add`，只有 LLM 可发出 update/delete）
- 邻域召回任何失败（embed/search/行读取）降级为无邻居 → `add`

**溯源与审计**：每个决策写一条 action log（`fact_write.{drop,add,update,delete,merge,noop}`，含 drop 原因/target/kind/statement 截断 80 runes）；candidate 携带 `source_kind`/`source_episode_id`/`source_session_id`/`source_message_id` 全链溯源。

**依赖装配**（`FactWritePipelineDeps`，恰好 8 个）：`Searcher`（邻域向量搜索）、`Embedder`、`Reader`（kind/statement enrichment）、`Writer`（双时态写）、`Access`（合并计数）、`Adjudicator`（可选）、`ActionLog`（可选）、`LG`。可选依赖为 nil 时按上述降级路径运行。

**接入点**：
- `AutoMemoryWorker.extract` / `extractFeedback`（会话提取 + 反馈提取均走管线；原内联冲突治理移除）
- `EpisodeConsolidator.persistViaPipeline`（sleep-time episode → fact 提取走管线；管线未注入时保留 legacy 直写兜底）

**subject_type → fact_kind 映射**（`FactKindForSubjectType`）：V3 提取 schema 的 `person|preference|constraint|goal|decision|relationship` 映射到白名单对应 kind（person→profile）；`event|concept` 映射为 event/knowledge（将被闸①丢弃）；未知值回退 `fact`（同样被闸①丢弃）。

### 16.2 Scratchpad→Episode 归档原子化（P1-2）

**问题**：L1 闲置 60min 即置 `cancelled` 且未归档（数据丢失）；归档失败后逃出扫描集合，失败静默永久化。

**设计**：
- **原子归档**：`EndL1Task` + `ArchiveAndCreateEpisodeTx` 同事务（archived_at 标记 + L2 episode 创建）
- **双分支扫描**（`MemoryL1ArchiveWorker`）：
  - 闲置 active 分支：updated_at < idle cutoff → end + 归档
  - 已结束未归档分支：archived_at='' 且 ended_at < now-2min → 仅重试归档事务（2min cutoff 防止与 `EndL1Task` 同步归档路径竞争）
- **死信告警**：同一任务连续归档失败 ≥3 次发流程日志告警（`system.memory_l1_archive.failed`），此后每 +10 次重发限频；任务永不离开扫描集合——失败可重试、持续失败有告警，消灭静默永久失败
- **显式完成**：`working_memory_complete` 工具让 agent 任务完成时主动触发归档，不再依赖闲置超时

### 16.3 Sleep-time LLM 接线（P1-1）

**问题**：Sleep-time 依赖静态 env 配置 LLM，未配置即每周期 nil LLM 空转跳过。

**设计**：`LLMResolver` 接口（`ResolveLLM(ctx, UserKey) trpcmodel.Model`）按 consolidation 目标从 agent 设置经 ModelCatalog 解析模型，优先级 **MemoryWorker → L0Compress → agent 默认模型**；`SleepTimeService` 与 `EpisodeConsolidator` 均经 `SetLLMResolver` 接线；resolver 缺失或解析失败回退静态 LLM。同一解析策略被 P1-3 的 `MemoryFactWriteAdjudicator` 复用。

### 16.4 记忆金丝雀（P0 闭环断言）

**设计动机**：评审根因——链路无人端到端拥有、失败全部静默降级、观测指标定义错误（`use_count` 召回即 +1 制造运行假象）。金丝雀是「写入的 fact 必须能召回」的代码级断言。

**设计**（`MemoryCanaryWorker`，默认 30min 周期）：

```
write   合成 fact（scope_type=canary）经生产 consolidation upsert 路径写入
recall  以严格哨兵阈值 minScore=0.55 经生产 L3 召回路径召回 → 断言命中
        （Bug A 回归捕获点：Scores.Total=0 将使召回阶段失败。
         注：P0-4 起生产默认 l3_recall_min_score 已降为 0.35，金丝雀刻意
         保留 0.55 严格门——合成 fact 构造得分远高于此，严格门不影响
         金丝雀通过率且保留回归捕获力，与 eval set min_score=0.55 同策略）
archive 双时态失效（valid_until=now）→ 断言从后续召回消失
```

任一阶段失败：记录 `biz.MemoryCanaryStatus`（经 `AlertMetricRegistry` 暴露为告警指标）+ 流程日志告警。金丝雀使用独立 canary scope，不污染业务记忆空间。

### 16.5 计数器三段化（FR-12.6）

**问题**：`use_count` 语义错误——召回路径返回结果即 +1，与该 fact 是否真正写入 prompt 无关。它制造「记忆在被使用」的假象（评审根因之一：观测指标定义错误），且无法度量真实注入率与引用率。

**设计**：三段漏斗分离，每段有唯一写入点，语义互不重叠：

| 计数器 | 语义 | 写入点 |
|--------|------|--------|
| `recalled_count` | 进入召回结果集 | 数据层召回路径：`l3ScoredAdapter` 在召回结果返回前批量 +1（`IncrementFactRecalledCount`）；历史 `use_count` 一次性回填 |
| `injected_count` | 通过过滤与预算、实际写入 prompt（用户视角唯一有意义的「使用」计数） | before-model hook：`memory_inject` 收集本轮实际注入的 factIDs，turn 完成后经 `bumpFactInjectedCounts` 异步批量 +1（`context.Background()` 脱离请求生命周期，不阻塞流式响应） |
| `cited_count` | 被助手回复显式引用 | `MemoryCitationBackfillWorker` 后台扫描回填（见下） |

**Schema 迁移**（`20261125_memory_fact_three_counters.sql`）：`memory_facts` 增加三列（`INTEGER NOT NULL DEFAULT 0`）；`UPDATE ... SET recalled_count = use_count WHERE recalled_count = 0 AND use_count > 0` 一次性回填（WHERE 守卫保证幂等）；新建 `memory_fact_citations(fact_id, turn_id, created_at)` 主键去重账本。`use_count` 列保留但不再维护（历史数据不丢）。

**引用回填 worker**（`MemoryCitationBackfillWorker`，默认 10min 周期、1h 滑窗、单批 200 条 notice）：

```
扫描 steps_v2 中 memory_recalled notice（= 本轮注入的 fact 集合）
  → join 该 turn 的最终 reply（kind='reply' AND is_final，seq 最大一条）
  → 对每条注入 fact 做双启发式判定：
      ① ID 引用：回复含 fact 短 provenance ID（prompt 以 "[id:abc12345, ...]" 渲染，
         LLM 复述该引用即确凿 citation）
      ② 文本重叠：statement 的 8-rune 滑窗 k-gram ≥50% 出现在回复中
         （CJK 无词边界无法分词，k-gram 包含容忍改写；statement <4 runes 跳过）
  → 命中写 (fact_id, turn_id) 去重账本 + cited_count +1
```

重叠窗口经账本幂等，重复扫描不会双计。全部失败 best-effort：本轮失败记录日志，下一 tick 重试；`MEMORY_CITATION_BACKFILL_DISABLED` 环境变量可整体关闭。

**前端可观测**：`MemoryFact` proto 增加 `recalled_count`/`injected_count`/`cited_count`（field 30/31/32）→ 记忆中心 L3 facts 表新增紧凑「召/注/引」列，fact 详情抽屉增加使用统计区（三枚 chip + 漏斗说明文案），中英 i18n。

### 16.6 Profile 常驻卡（FR-12.7）

**问题**：事实召回依赖向量打分，低分事实不进 prompt——用户最常用的画像/偏好可能长期不得注入，感知不到记忆存在。常驻卡是「用户感到记忆存在」的最短路径（Letta core memory 模式）：100% 注入率，不经召回打分。

**数据模型**（`20261126_memory_profile_cards.sql`）：`memory_profile_cards` 每 `(agent_id, user_id)` 一张卡（唯一索引），字段 `content`（蒸馏正文）/`fact_count`（源事实数）/`version`（upsert 递增）/`updated_at`。

**Sleep-time Phase 3**（`ProfileCardDistiller`，在 Phase 1 trpcmemory 整理与 Phase 2 episode→fact 提取之后运行，使用最新事实集）：

```
读取 active 画像类事实（kinds = profile/preference/goal/constraint，
  user ∪ agent scope，importance 排序，上限 50 条，单条截断 200 runes）
  → 零源事实 → 删除过期卡（卡不得比事实活得更久，如治理清空偏好后）
  → LLM 蒸馏（45s 超时；系统提示要求按语义分组合并、冲突以更新更具体为准、
     ≤800 字、不编造源事实之外信息）
  → 输出净化（剥 markdown 围栏/标题，硬上限 1000 runes）
  → upsert（version+1，fact_count=源事实数）
```

LLM 复用 §16.3 的 `LLMResolver` 按目标解析（MemoryWorker → L0Compress → agent 默认），nil LLM 或蒸馏失败 → 保留旧卡优雅降级。整个 Phase 3 best-effort 永不返回错误——JobRunner 的重试决策只由 Phase 1 驱动，避免 Phase 3 失败导致整个 job 重试而重复 Phase 1/2 变更。

**注入**（`ProfileCardCue`）：L3 注入开启时，卡片无条件渲染在记忆块**首位**（`## 用户档案（长期记忆摘要，始终生效）` 标题 + 正文，hook 侧硬上限 1200 runes 安全网）；无卡/读卡失败返回空串，best-effort 永不打断 turn。

**依赖装配**（wire）：`NewProfileCardDistiller(data.NewMemoryPreferenceLister, data.NewMemoryProfileCardStore)` → `SetLLMResolver(resolver)` → `SleepTimeService.SetProfileCardDistiller`；注入侧 `TRPCBuilderDeps.MemoryProfileCardReader` = `data.NewMemoryProfileCardStore`。

### 16.7 召回质量升级（P2）

> 对应需求 FR-13。FR-12/§16.1-16.6 修复「写入→可召回」闭环后，本节提升召回侧质量：混合检索补齐向量盲区、预算档位约束注入体量、L2 默认开启恢复会话上下文、离线评测集提供回归门。

#### 16.7.1 L2 召回默认开启（FR-13.1）

- **Schema 默认**：`agent_runtime_setting.l2_recall_enabled` 默认值改 `true`（`ent/schema/agent_runtime_setting.go`），`biz.DefaultAgentRuntimeSettings` 同步 `L2RecallEnabled: true`——新 agent 开箱即有会话事件召回。
- **存量迁移**：数据迁移 `20261127 memory_l2_recall_default_on`（`memory_l2_recall_default_migrate.go`，经 `schema_migrations` 表门控幂等）将全部存量行的 `l2_recall_enabled` false→true。历史上该功能因默认关闭实质不可用，不存在「显式 opt-out」语义，故整体翻转安全。

#### 16.7.2 召回 token 预算档位（FR-13.2）

- **档位常量**（`biz/agent_memory_runtime_policy.go`）：`MemoryRecallBudgetCompact=400 / Standard=800（默认）/ Generous=1600`，策略解析 nil/越界值回落 Standard。
- **配置链路**：proto `AgentRuntimeSetting.l3_recall_budget_tokens`（field 132）→ agent settings → runtime policy → 召回打包器；前端表单字段经 proto 再生成后暴露。
- **打包器**（`agent/recall_budget.go` `recallLinePacker`）：候选行按分数降序贪心装入——高分行优先，装不下当前大行时允许后续小行填充剩余预算；**标题行显式计入预算**（`packer.allow(header)`），杜绝「内容达标但标题溢出」；token 按 rune 估算。
- **接入点**：`composite_prompt.go`（L2+L3 复合块）、`l2_prompt.go`、`l3_prompt.go` 统一经 packer 输出。
- **计数准确性**：`injected_count` 只统计实际通过预算装入的行对应 fact ID——未注入不计数，保持 recalled ≥ injected ≥ cited 漏斗语义（FR-12.6）。

#### 16.7.3 L3 混合召回：pgvector + FTS 经 RRF 融合（FR-13.3/13.4）

**问题**：纯向量召回对关键词强项事实（编号、代号、精确名称等字母数字 token）相似度天然偏低，minScore 阈值直接漏召。

**FTS 通道**（`data/memory_l3_fts.go`）：

- DDL 迁移 `20261128 memory_facts_fts_index`（Postgres-only，registry Func 门控，SQLite CLI/测试跳过）：`memory_facts` 上建 GIN 索引 `to_tsvector('simple', statement || ' ' || COALESCE(details_markdown, ''))`。
- `'simple'` 配置不分词 CJK（连续中文成单 token），故 FTS 定位为**字母数字 token 的补充信号**；中文关键词匹配仍由 Go 子串通道（`keywordOverlapScore`）承担。
- `searchL3FTS` 按 `ts_rank` 排序返回候选 ID（遵循 active/未删除/未失效过滤与 scope/user 过滤）；空查询/非 Postgres 返回 nil，调用方自然降级。

**RRF 融合**：`rrfFuseRanked(k=60)` 合并向量排名与 FTS 排名——`fused(id) = Σ 1/(k+rank)`，取并集按融合分降序，截断到候选池后经既有 `scoreFactRow` 校准打分（minScore 语义不变）；融合分仅写入 `recallScoreBreakdown.RRF` 做可观测性注解，不参与 Total。

**三条召回路径全覆盖**（`data/memory_shim_l3.go`）：

| 路径 | 触发 | FTS 接入方式 |
|------|------|-------------|
| 暴力扫描（小数据集 ≤ 阈值或无 embedding） | `recallL3FactsBruteForce` | FTS 候选注入扫描池（`ftsExtraCandidates`），不错过关键词命中 |
| recency 池（无向量结果降级） | `recallL3Facts` | 同上 |
| pgvector 主路径 | `recallL3WithVectorStore` | 向量候选 ∪ FTS 候选 RRF 融合 |

**主聊天链路修复**：`memory_l3_scored_adapter.go` 的 `RecallL3Hits` 原传 `nil` VectorStore（融合路径永不激活），已改为 `a.data.VectorStore()`。

**降级可观测**（FR-13.4）：vector store 搜索失败不再静默回退——`Warn` 进程日志（step `memory.l3_vector_search`）后降级为 FTS/recency 召回，召回本身不失败。

#### 16.7.4 离线评测集（FR-13.5）

- **数据集**：`data/testdata/memory_l3_eval_set.json` 50 条，覆盖五能力——信息抽取 / 多跳推理 / 时序感知 / 知识更新（旧事实应被新事实取代）/ 拒答（无关查询应空召回）。每条含 `seed_facts`、`expect_hits`、`expect_absent`、`expect_empty`，确定性构造（控制关键词重叠/ recency / importance 变量）。
- **执行器**：`memory_l3_eval_set_test.go` `TestMemoryL3OfflineEvalSet` 对真实 Postgres schema 跑全量 50 条，通过率门 **≥90%**，纳入 `go test ./internal/data` 回归。

#### 16.7.5 配套 DDL/数据迁移一览

| 版本 | 名称 | 类型 | 说明 |
|------|------|------|------|
| 20261125 | memory_fact_three_counters | DDL | FR-12.6 recalled/injected/cited 三列 |
| 20261126 | memory_profile_cards | DDL | FR-12.7 常驻卡存储 |
| 20261127 | memory_l2_recall_default_on | 数据迁移 | FR-13.1 存量 L2 召回翻转 |
| 20261128 | memory_facts_fts_index | DDL（Func 门控） | FR-13.3 FTS GIN 索引（Postgres-only） |

#### 16.7.6 组合召回分层路径与 L2 跨会话候选池（P2-R1/R2 回归修复）

> M-P2 验收后运行时冒烟发现的两处召回回归，2026-08-05 修复。实现记录见 [development.md §18.7 M-P2-5](./70-orchestration-longtask-memory.development.md)。

**P2-R1：组合召回直接组合 fused 分层用例**。主聊天注入路径的 `MemoryCompositeRecallUsecase` 原走降级 legacy store 路径（`CompositeSearchMemories` raw repo 查询）——无 embedding、无 pgvector/FTS RRF、以 importance 冒充分数、`recalled_count` 不自增，M-P2-3 的融合成果在主链路实质失效。修复后经 `SetLayerRecallers(l2, l3)` 注入两个 fused 召回用例（Wire 装配）：

```
RecallComposite
  → L2 RecallEpisodes（embedding + 向量/CE 重排 + annotateEpisodeScores）
  → L3 RecallFactsFused（embedding + pgvector/FTS RRF + 校准分 + recalled_count 自增）
  → 按 scores.total 归并排序（L2 行经 annotateEpisodeScores 注入同一 scores 契约）
  → MMRRerankTexts 多样性重排（λ 偏向相关性，Jaccard 词集冗余度）
  → limit 截断
```

MMR 实现由 data 层上移至 biz（`biz/memory_mmr.go` `MMRRerankTexts`），data 层保留包装供 legacy 调用方。l3 为 nil 时回退 legacy store 路径（向后兼容）。

> **P0-C 补记（2026-08-11）**：layered 路径查询向量改为**每轮共享单 embed**——`MemoryCompositeRecallUsecase.SetEmbedder`（Wire 装配于 `wire_memory.go`）注入后，RecallComposite 先以 3s 超时 Embed 一次，向量经 `L2RecallQuery`/`L3FusedRecallQuery` 的 `QueryEmbedding`/`EmbedAttempted` 字段下传，L2/L3 不再各自对同一 query 独立 embed；embed 失败置 EmbedAttempted=true +  nil 向量，层内直接降级非向量检索（不再重复尝试）。未 SetEmbedder 时行为与此前完全一致（各层自行 embed，3s 超时降级）。

**P2-R2：L2 召回候选池 agent 全域化**。`recallL2Episodes`（无向量暴力路径）原将 `sessionID` 作为 SQL 过滤条件构造候选池——跨会话 episode 在该路径永不可达（向量路径 `recallL2WithVectorStore` 本就只按 agent_id 过滤，两条路径语义不一致）。修复后候选池与向量路径对齐为 agent 全域；`sessionID` 仅保留于打分的连续性加分（`l2ScoreWeightSession=0.05` 的 sessionBoost）——同会话 episode 在同等相关度下优先，但不再排除跨会话记忆。
