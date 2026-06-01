# SQLite 读写业务逻辑深度分析与解决方案

> 分析日期：2026-06-02
> 分析范围：`internal/data/` 全部 Repo 实现 + `internal/biz/` 全部 Repo 接口 + 核心业务流
> 视角：架构 + 设计 + 业务场景 + 业务需求 四维一体

---

## 第一部分：业务场景与数据流全景

### 1.1 核心业务场景识别

Aranea-Agents 的数据访问围绕 **6 大核心业务场景** 展开：

| 场景 | 频率 | 一致性要求 | 延迟敏感度 | 涉及的表 |
|------|------|-----------|-----------|----------|
| **对话轮次执行** | 极高（每次对话） | 强一致 | 极高（用户等待） | sessions, messages, session_runs, session_turns |
| **Token 用量记录** | 极高（每次 LLM 调用） | 强一致 | 中（异步可接受） | model_token_usage_events, model_token_usage_daily/hourly, sessions |
| **记忆提取与同步** | 高（每轮对话后） | 最终一致 | 低（后台异步） | memory_facts, memory_entities, memory_relations, pgvector |
| **渠道消息投递** | 中（外部平台触发） | 幂等一致 | 中（用户等待） | channel_turn_jobs, channel_inbound_receipts, channel_runtime_lease |
| **监控与审计** | 高（全链路） | Best-effort | 低（后台异步） | audit_logs, monitor_events, monitor_traces, flow_log_events |
| **配置与元数据** | 低（管理操作） | 强一致 | 低 | agents, teams, tools, system_settings, llm_provider_models |

### 1.2 一次 Chat Turn 的完整数据写入时序

```
用户发送消息
    │
    ├─[同步·用户等待] Session 状态更新
    │   └─ sessions 表: status='running', updated_at
    │
    ├─[同步·用户等待] 消息写入
    │   └─ messages 表: INSERT user message
    │   └─ sessions 表: message_count += 1, 自动标题
    │
    Runner 执行中
    │
    ├─[异步·后台] 工具调用记录
    │   └─ tool_invocations 表: INSERT
    │
    ├─[异步·后台] 流程日志
    │   └─ flow_log_events 表: INSERT
    │
    ├─[同步·用户等待] Token 用量记录
    │   └─ [单事务] INSERT model_token_usage_events
    │   └─ [单事务] UPDATE sessions (聚合递增)
    │   └─ [单事务] UPSERT model_token_usage_daily
    │   └─ [单事务] UPSERT model_token_usage_hourly
    │
    Runner 完成
    │
    ├─[同步·用户等待] 消息写入
    │   └─ messages 表: INSERT assistant message
    │
    ├─[同步·用户等待] Session 状态更新
    │   └─ sessions 表: status='idle', context_used_ratio, session_revision += 1
    │
    ├─[同步·用户等待] Runner 完成监控
    │   └─ monitor_events 表: INSERT (幂等)
    │
    ├─[异步·后台] 记忆任务入队
    │   └─ → 记忆队列 → L3 Fact 提取 → L4 Graph 写入 → 索引双写
    │
    ├─[异步·后台] Webhook 派发
    │   └─ hook_deliveries 表: INSERT + 外部 HTTP
    │
    └─[异步·后台] 预算告警评估
        └─ budget_alerts 表: UPDATE
```

### 1.3 一致性保证体系

| 层级 | 机制 | 适用场景 | 当前实现 |
|------|------|----------|----------|
| **强一致** | SQLite 单事务多表写入 | Usage 4 表原子写入、Session 压缩 CAS | ✅ 已实现 |
| **幂等写入** | exists 检查 + patch | RunnerCompletion 去重 | ✅ 已实现 |
| **原子递增** | `UPDATE SET x = x + ?` | Session 聚合、日/小时汇总 | ✅ 已实现 |
| **进程内互斥** | `locker.Lock(sessionID)` | 同一 Session 消息串行化 | ✅ 已实现 |
| **竞态桥接** | `TurnCompletionBridge` | Usage 先于 Completion 到达 | ✅ 已实现 |
| **最终一致** | EventBus + asyncEnvelopeWorker | 工具调用、FlowLog、Webhook | ✅ 已实现 |
| **Best-effort** | 失败仅日志 | 监控事件、审计日志 | ✅ 已实现 |

---

## 第二部分：问题诊断（业务驱动视角）

### 问题 1：对话主路径写入放大 🔴

**业务场景**：一次 Chat Turn 在同步路径上需要写 sessions 表 **3~4 次**（状态→消息→用量聚合→上下文更新），加上异步路径的 tool_invocations、flow_log、monitor_events 等。

**根因**：Session 表被设计为"超级表"，既存储会话元数据，又承载实时聚合计数器（message_count, model_call_count, tool_call_count, input_tokens, total_cost_micro_usd 等），还存储运行时状态（status, context_used_ratio, runner_snapshot_json, state_json）。

**影响**：
- SQLite 单写连接下，所有 Session 更新串行化，高并发时成为瓶颈
- Usage 记录事务内同时写 4 张表 + 更新 sessions 聚合，事务持有时间长
- `context_used_ratio` 和 `status` 更新频率极高但与核心业务无关

**业务需求**：用户发送消息后应在 200ms 内收到响应，但 SQLite 写入串行化可能使单次 Turn 的同步写入耗时超过 100ms。

### 问题 2：双轨 Schema 管理导致演进困难 🔴

**业务场景**：新增渠道（如企业微信）需要 `channel_turn_jobs` 表增加 `retry_count` 列。当前需要：① 手写 ALTER TABLE ② 新建 `*_patch.go` ③ 在 `ensureSchemaDDL` 中注册 ④ 手动写 CRUD SQL。

**根因**：25 张表未纳入 Ent Schema，Schema 变更无法通过 `go generate` 统一管理。

**影响**：
- 新增字段的开发周期是 Ent Schema 表的 3~5 倍
- 20+ 个 `*_patch.go` 文件散落各处，`PRAGMA table_info` + `ALTER TABLE` 模式重复 15+ 次
- 无法生成类型安全的 CRUD 代码，Raw SQL 中字段名拼写错误只能在运行时发现
- 表结构文档（`docs/sql/`）与实际代码可能不同步

**业务需求**：快速迭代新功能（新渠道、新监控指标、新记忆层级）需要 Schema 变更的敏捷性。

### 问题 3：记忆子系统数据访问绕过标准架构 🟡

**业务场景**：记忆系统是 Agent "越用越懂你" 的核心能力，涉及 L0~L4 五个层级。当前 `sessionmemory.Store` 直接操作 18+ 张表，绕过了 biz 层的 Repo 接口。

**根因**：记忆子系统在项目早期作为独立模块开发，未遵循标准的 biz→data 分层模式。

**影响**：
- 无法在测试中 mock 记忆存储（必须启动真实 SQLite）
- 记忆子系统的数据访问逻辑散落在 `sessionmemory/` 子包中，biz 层无法控制
- 新增记忆层级（如 L5 长期规划记忆）需要在 `Store` 中增加方法，违反开闭原则

**业务需求**：记忆系统需要持续演进（新增层级、调整策略、A/B 测试），架构必须支持灵活扩展。

### 问题 4：接口拆分不彻底导致依赖污染 🟡

**业务场景**：Channel 模块只需要 `TeamRunRepo` 来查询 Team 运行状态，但当前必须依赖整个 `TeamRepository`（21 方法），引入了对 Team CRUD、OrchestrationStep、TaskDeadLetter 的不必要依赖。

**根因**：`TeamRepository`、`monitor.Repo`、`a2a.Repo` 等接口未按红线 #15 拆分。

**影响**：
- Wire 注入时构造了不需要的依赖
- 接口变更影响面大（改 Team CRUD 影响只读 TeamRun 的消费方）
- 测试时需要 mock 整个大接口

**业务需求**：模块间应松耦合，渠道模块不应因 Team CRUD 变更而重新编译。

### 问题 5：事务管理不统一导致跨 Repo 操作无法原子化 🟡

**业务场景**：Usage 记录需要在同一事务中写 `model_token_usage_events` + 更新 `sessions` 聚合 + upsert 日/小时汇总。当前通过 `r.ent().BeginTx()` 实现，但如果未来需要在同一事务中写入 `hook_deliveries`（Raw SQL 表），则无法协调。

**根因**：4 种事务模式并存（Ent ExecInTx、RawDB BeginTx、sqlRunner、ent BeginTx），无法跨模式共享事务。

**影响**：
- 跨 Ent 表和 Raw SQL 表的操作无法原子化
- 渠道场景中 `channel_turn_jobs`（Raw SQL）+ `sessions`（Ent）的状态更新不是原子的

**业务需求**：渠道消息的"任务创建→Session 创建→任务关联"应保证原子性，否则可能出现孤儿任务。

### 问题 6：Raw SQL 泛滥降低可维护性 🟡

**业务场景**：开发者新增一个 `monitor_trace_spans` 表的查询，需要手写 SQL、手写 Scan 逻辑、手写 WHERE 条件拼接、手写分页。Ent 表的同类操作只需 3 行代码。

**根因**：55% 的 Raw SQL 源于表未进 Ent，20% 源于 SQLite 特有语法需求。

**影响**：
- 代码量是 Ent 实现的 3~5 倍
- SQL 注入风险（动态 WHERE 拼接）
- 字段映射错误只能在运行时发现
- 重构时无法使用 IDE 的"查找引用"功能

**业务需求**：降低新功能开发成本，减少运行时 bug。

### 问题 7：SQLite 写瓶颈 🟢

**业务场景**：10 个并发用户同时对话，每个 Turn 触发 3~4 次同步 Session 写入 + 1 次 Usage 4 表事务。SQLite 单写连接下这些操作串行执行。

**根因**：SQLite WAL 模式允许并发读，但写仍然是单线程的。`MaxOpenConns=1` 确保了安全但限制了吞吐。

**影响**：
- 当前用户规模（<50 并发）下可接受
- 随着用户增长，写入延迟将成为瓶颈
- `retryOnBusy` 仅 3 次，极端情况下可能写入失败

**业务需求**：支持 100+ 并发用户的流畅对话体验。

---

## 第三部分：解决方案（四维一体）

### 维度一：架构层面 — 分离关注点

#### 方案 1.1：Session 表冷热分离

**解决问题**：对话主路径写入放大（问题 1）

**核心思路**：将 Session 表的"实时热字段"与"冷元数据"分离，减少同步路径的写入量。

```
当前 sessions 表（一表多用）：
├── 冷元数据（创建时写入，几乎不变）
│   id, workspace_id, user_id, agent_id, team_id, title, summary,
│   tags_json, dialog_mode, default_provider, default_model, ...
├── 实时聚合（每次 Turn 更新）
│   message_count, run_count, model_call_count, tool_call_count,
│   input_tokens, output_tokens, total_tokens, total_cost_micro_usd,
│   avg_latency_ms, error_count
├── 运行时状态（每次 Turn 更新 2~3 次）
│   status, status_reason, context_used_tokens, context_used_ratio,
│   max_context_used_ratio, context_status, runner_snapshot_json, state_json
└── 时间戳
    created_at, updated_at, last_message_at, last_run_at
```

**改造方案**：

```
sessions 表（冷元数据 + 时间戳）：
  id, workspace_id, user_id, agent_id, team_id, title, summary,
  tags_json, dialog_mode, default_provider, default_model,
  created_at, updated_at, first_message_at, last_message_at,
  last_run_at, archived_at, deleted_at, pinned_at, session_revision

session_metrics 表（实时聚合，每次 Turn 写 1 次）：
  session_id, message_count, run_count, model_call_count,
  tool_call_count, skill_call_count, mcp_call_count,
  input_tokens, output_tokens, total_tokens, total_cost_micro_usd,
  avg_latency_ms, error_count, updated_at

session_runtime_state 表（运行时状态，每次 Turn 写 2~3 次）：
  session_id, status, status_reason, context_used_tokens,
  context_used_ratio, max_context_used_ratio, context_status,
  runner_snapshot_json, state_json, compress_version, updated_at
```

**收益**：
- 对话主路径的同步写入从 sessions 表 3~4 次降为 1 次（runtime_state）+ 1 次（metrics）
- 每次写入的行更窄，SQLite 锁持有时间更短
- 聚合查询（如"列出所有会话的 Token 用量"）可独立读 `session_metrics`，不影响主表
- `session_runtime_state` 可考虑未来迁移到内存 + 定期快照模式

**实施路径**：

```
Phase 1: 新建 Ent Schema（session_metrics, session_runtime_state）
Phase 2: 数据迁移（从 sessions 复制字段到新表）
Phase 3: 读写切换（读走新表，写走新表）
Phase 4: 清理 sessions 表中的冗余字段
```

#### 方案 1.2：统一事务管理器

**解决问题**：跨 Repo 操作无法原子化（问题 5）

**核心思路**：建立统一的 `TransactionManager` 接口，通过 context 同时传播 Ent TX 和 Raw SQL TX。

```go
type TransactionManager interface {
    ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type rawTxKey struct{}

func (d *Data) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
    tx, err := d.entClient.Tx(ctx)
    if err != nil {
        return err
    }
    txCtx := context.WithValue(ctx, txClientKey{}, tx.Client())
    txCtx = context.WithValue(txCtx, rawTxKey{}, tx)
    if err := fn(txCtx); err != nil {
        _ = tx.Rollback()
        return err
    }
    return tx.Commit()
}

// Raw SQL Repo 从 context 获取共享事务
func (r *someRawSQLRepo) execerFromCtx(ctx context.Context) execer {
    if tx, ok := ctx.Value(rawTxKey{}).(*ent.Tx); ok {
        return tx.Client()
    }
    return r.data.RawDB()
}
```

**收益**：
- 渠道场景的"任务创建→Session 创建→任务关联"可原子化
- Usage 记录可在同一事务中写入 Ent 表和 Raw SQL 表
- 所有 Repo 共享同一事务语义

**实施路径**：

```
Phase 1: 定义 TransactionManager 接口 + Data 实现
Phase 2: 逐步迁移 Raw SQL Repo 使用 execerFromCtx()
Phase 3: 废弃 Raw SQL Repo 中独立的 BeginTx() 调用
Phase 4: biz 层 Usecase 通过 TransactionManager 编排跨 Repo 事务
```

#### 方案 1.3：记忆子系统数据访问收口

**解决问题**：sessionmemory.Store 绕过标准架构（问题 3）

**核心思路**：将 `sessionmemory.Store` 拆分为独立 Repo，每个 Repo 实现 biz 层定义的窄接口。

```
当前：
  biz 层 → sessionmemory.Store（影子数据层）→ 18+ 张表

改造后：
  biz 层 → L0SnapshotRepo → memory_l0_assembly_snapshots 表
        → L2EpisodeRepo → memory_l2_episodes + memory_l2_episode_index 表
        → L3FactRepo → memory_l3_facts + memory_facts 表
        → L4EntityRepo → memory_entities + memory_relations 表
        → CascadeRepo → cascade_proposals + cascade_sagas 表
        → ActionLogRepo → memory_action_log 表
```

**收益**：
- 记忆子系统可独立测试（mock Repo 接口）
- 新增记忆层级只需新增 Repo 接口 + 实现，不影响已有层级
- biz 层 MemoryUsecase 只依赖窄接口，符合依赖倒置原则

**实施路径**：

```
Phase 1: 在 biz 层定义记忆子系统的 Repo 接口（部分已有）
Phase 2: 将 sessionmemory.Store 的方法拆分到独立 Repo struct
Phase 3: Wire 绑定新 Repo，替换 Store 的直接注入
Phase 4: 废弃 sessionmemory.Store 的聚合方法
```

---

### 维度二：设计层面 — 消除技术债务

#### 方案 2.1：野生表纳入 Ent Schema

**解决问题**：双轨 Schema 管理（问题 2）、Raw SQL 泛滥（问题 6）

**核心思路**：将 25 张野生表全部纳入 Ent Schema，对 Ent 不支持的特性用 Raw Query + Ent 类型映射保留。

**分批实施计划**：

**第一批（高频表，立即执行）**：

| 表 | 当前访问方式 | 迁移后 | 预期收益 |
|----|------------|--------|----------|
| `channel_turn_jobs` | 纯 Raw SQL | Ent + 条件 upsert 保留 Raw | 类型安全 + 可生成 |
| `channel_inbound_receipts` | 纯 Raw SQL | Ent CRUD | 消除手写 SQL |
| `session_runs` | 纯 Raw SQL | Ent + 乐观锁保留 Raw | 类型安全 |
| `session_run_checkpoints` | 纯 Raw SQL | Ent CRUD | 消除手写 SQL |
| `hook_deliveries` | 纯 Raw SQL | Ent + 乐观锁保留 Raw | 类型安全 |
| `monitor_alert_rules` | 纯 Raw SQL | Ent + BEGIN IMMEDIATE 保留 Raw | 类型安全 |

**第二批（中频表，短期执行）**：

| 表 | 当前访问方式 | 迁移后 |
|----|------------|--------|
| `audit_logs` | 纯 Raw SQL | Ent CRUD |
| `monitor_events` | 纯 Raw SQL | Ent + json_extract 保留 Raw |
| `monitor_traces` / `monitor_trace_spans` | 纯 Raw SQL | Ent + 生成列保留 Raw |
| `plugin_runs` | 纯 Raw SQL | Ent CRUD |
| `plugin_cost_guard_usage` | 纯 Raw SQL | Ent + ON CONFLICT 保留 Raw |
| `team_graph_sessions` | 纯 Raw SQL | Ent CRUD |

**第三批（低频表，中期执行）**：

| 表 | 当前访问方式 | 迁移后 |
|----|------------|--------|
| `a2a_agent_cards` 等 4 张 | 纯 Raw SQL | Ent CRUD |
| `eval_datasets` 等 4 张 | 纯 Raw SQL | Ent CRUD |
| `ecosystem_products` 等 2 张 | 纯 Raw SQL | Ent CRUD |
| `learning_observations` 等 3 张 | 纯 Raw SQL | Ent CRUD |
| `positions` / `industries` / `departments` | 纯 Raw SQL | Ent CRUD + JOIN 保留 Raw |

**Ent 不支持的特性保留策略**：

| 特性 | 保留方式 | 示例 |
|------|---------|------|
| `ON CONFLICT DO UPDATE WHERE` | `ent.Client.ExecContext()` + Ent 类型映射 | 渠道租约 |
| `INSERT OR IGNORE` | Ent `OnConflictColumns` + 不更新 | 幂等去重 |
| `json_set()`/`json_remove()` | 封装为 Repo 方法，内部 Raw SQL | Session State Patch |
| FTS5 `MATCH`/`bm25()` | 保留 Raw SQL（Ent 不支持 FTS5） | 消息全文搜索 |
| pgvector 向量搜索 | 保留 Raw SQL（Ent 不支持向量） | 记忆向量检索 |
| `BEGIN IMMEDIATE` | 保留 Raw SQL | 告警规则替换 |
| `GENERATED ALWAYS AS` | Ent Schema `Annotations` + Raw DDL | Trace Span 索引 |
| 50+ 列 INSERT | Ent `SetXxx()` 链式调用 | Usage 事件 |

#### 方案 2.2：接口拆分合规化

**解决问题**：接口拆分不彻底（问题 4）

**拆分方案**：

```go
// ─── TeamRepository 拆分 ───

type TeamReader interface {
    ListTeams(ctx context.Context) ([]Team, error)
    GetTeamByID(ctx context.Context, id string) (Team, error)
    ListBySpiritSessionID(ctx context.Context, spiritSessionID string) ([]Team, error)
}

type TeamWriter interface {
    CreateTeam(ctx context.Context, t Team) (Team, error)
    UpdateTeam(ctx context.Context, t Team) (Team, error)
    DeleteTeam(ctx context.Context, id string) error
}

type TeamRunReader interface {
    ListTeamRuns(ctx context.Context, teamID string, limit int) ([]TeamRun, error)
    HasActiveTeamRun(ctx context.Context, teamID string) (bool, error)
    GetTeamRunByID(ctx context.Context, id string) (TeamRun, error)
}

type TeamRunWriter interface {
    CreateTeamRun(ctx context.Context, r TeamRun) (TeamRun, error)
    UpdateTeamRun(ctx context.Context, r TeamRun) error
    UpdateTeamRunGraphExecutionID(ctx context.Context, runID, graphExecutionID string) error
    UpdateTeamRunTraceID(ctx context.Context, runID, traceID string) error
    UpdateTeamRunSummaryJSON(ctx context.Context, runID, summaryJSON string) error
}

type OrchestrationStepRepo interface {
    BatchCreateOrchestrationSteps(ctx context.Context, steps []OrchestrationStep) error
    ListOrchestrationSteps(ctx context.Context, teamRunID, nodeID string, limit int) ([]OrchestrationStep, error)
}

type TaskDeadLetterRepo interface {
    CreateTaskDeadLetter(ctx context.Context, dl TaskDeadLetter) error
    ListTaskDeadLetters(ctx context.Context, filter TaskDeadLetterListFilter) ([]TaskDeadLetter, error)
    ResolveTaskDeadLetter(ctx context.Context, id string) (TaskDeadLetter, error)
}

// TeamRepository = 上述所有子接口组合
// Wire 绑定时按需注入窄接口

// ─── monitor.Repo 拆分 ───

type AuditLogReader interface {
    ListAuditLogs(ctx context.Context, query AuditQuery) (AuditListResult, error)
}
type AuditLogWriter interface {
    InsertAuditLog(ctx context.Context, entry AuditLog) error
}
type MonitorEventReader interface {
    ListMonitorEvents(ctx context.Context, query EventsQuery) (ListResult, error)
    GetMonitorEvent(ctx context.Context, id string) (PlatformRow, error)
    CountMonitorEventsSince(ctx context.Context, eventKey, status, since, until string) (int32, error)
}
type MonitorEventWriter interface {
    InsertMonitorEvent(ctx context.Context, ev EventWrite) error
}
type MonitorTraceReader interface {
    ListMonitorTraces(ctx context.Context, query TracesQuery) (ListResult, error)
    GetMonitorTrace(ctx context.Context, id string) (PlatformRow, error)
}
type MonitorTraceWriter interface {
    InsertMonitorTrace(ctx context.Context, tw TraceWrite) error
    UpsertMonitorTraceSpan(ctx context.Context, sw TraceSpanWrite) error
    UpdateMonitorTraceCompletion(ctx context.Context, traceID string, status string, durationMs int64, spanCount, errorCount int, totalTokens int64, totalCostUsd float64) error
}
type AlertRuleRepo interface {
    ListAlertRules(ctx context.Context) ([]AlertRule, error)
    ReplaceAlertRules(ctx context.Context, rules []AlertRule) error
    UpdateAlertFiringState(ctx context.Context, id string, state AlertFiringState, lastFiredAt *time.Time, lastFiredValue float64, recoveredAt *time.Time) error
}
type MonitorCompletionRepo interface {
    AvgRunnerCompletionDurationMsSince(ctx context.Context, sinceRFC3339 string) (float64, error)
    LatencyPercentilesSince(ctx context.Context, sinceRFC3339 string) (p50, p95, p99 float64, err error)
    ExistsRunnerCompletion(ctx context.Context, sessionID, invocationID string) (bool, error)
    PatchRunnerCompletionMetadata(ctx context.Context, sessionID, runID, invocationID, patchJSON string) (bool, error)
    ListRecentRunnerCompletions(ctx context.Context, since time.Duration, limit int) ([]RunnerCompletionRow, error)
}

// ─── a2a.Repo 拆分 ───

type A2ACardRepo interface {
    UpsertAgentCard(ctx context.Context, card AgentCard) (AgentCard, error)
    GetAgentCard(ctx context.Context, agentID string) (AgentCard, error)
    ListEnabledCards(ctx context.Context, workspace, capability string) ([]AgentCard, error)
    MapEndpointEnabled(ctx context.Context, agentIDs []string) (map[string]bool, error)
}
type A2AInvocationRepo interface {
    CreateInvocation(ctx context.Context, inv Invocation) (Invocation, error)
    UpdateInvocation(ctx context.Context, inv Invocation) error
}
type A2AAuditRepo interface {
    InsertAudit(ctx context.Context, entry AuditEntry) error
    ListAudit(ctx context.Context, callerID, calleeID string, limit, offset int) ([]AuditEntry, int, error)
}
type A2ARemoteRepo interface {
    CreateRemoteAgent(ctx context.Context, agent RemoteAgent) (RemoteAgent, error)
    ListRemoteAgents(ctx context.Context, workspace string) ([]RemoteAgent, error)
    DeleteRemoteAgent(ctx context.Context, id string) error
    GetRemoteAgent(ctx context.Context, id string) (RemoteAgent, error)
    DiscoverRemoteCard(ctx context.Context, in RemoteCardDiscoverInput) (AgentCard, error)
    UpdateRemoteAgentHealth(ctx context.Context, id string, ok bool, errMsg string) error
}
```

#### 方案 2.3：Schema 迁移框架化

**解决问题**：散落的 `*_patch.go` 模式（问题 2 的一部分）

**核心思路**：建立统一迁移框架，替代 20+ 个 `*_patch.go` 文件。

```go
type Migration struct {
    Version      int
    Name         string
    Up           func(ctx context.Context, client *ent.Client) error
    Dependencies []int
}

var registry = []Migration{
    {Version: 20260601, Name: "add_channel_turn_job_retry_count",
     Up: func(ctx context.Context, c *ent.Client) error {
         if has, _ := sqliteColumnExists(ctx, c, "channel_turn_jobs", "retry_count"); has {
             return nil
         }
         _, err := c.ExecContext(ctx, "ALTER TABLE channel_turn_jobs ADD COLUMN retry_count INTEGER NOT NULL DEFAULT 0")
         return err
     }},
    // ... 所有 *_patch.go 中的迁移纳入此处
}

func RunPendingMigrations(ctx context.Context, client *ent.Client, lg loggateway.Logger) error {
    for _, m := range registry {
        applied, err := isMigrationApplied(ctx, client, m.Version, lg)
        if err != nil {
            return err
        }
        if applied {
            continue
        }
        if err := m.Up(ctx, client); err != nil {
            return fmt.Errorf("migration %d (%s): %w", m.Version, m.Name, err)
        }
        if err := recordMigrationApplied(ctx, client, m.Version, m.Name, lg); err != nil {
            return err
        }
    }
    return nil
}
```

**收益**：
- 所有 Schema 变更有版本号、有依赖顺序
- 新增字段只需在 `registry` 中添加一条记录
- 可生成迁移报告（哪些已应用、哪些待应用）
- 消除 20+ 个 `*_patch.go` 文件

---

### 维度三：业务场景层面 — 优化关键路径

#### 方案 3.1：对话主路径写入优化

**解决问题**：对话主路径写入放大（问题 1）

**核心思路**：减少同步路径的写入次数，将非关键写入推迟到异步。

**当前同步路径写入（4~5 次/Turn）**：

```
1. sessions: status='running'           ← 必须同步
2. messages: INSERT user message         ← 必须同步
3. sessions: message_count += 1          ← 可异步
4. [Usage 事务: 4 表写入]                ← 可异步
5. messages: INSERT assistant message    ← 必须同步
6. sessions: status='idle', context_*    ← 必须同步
7. sessions: session_revision += 1       ← 必须同步
```

**优化后同步路径写入（3 次/Turn）**：

```
1. messages: INSERT user message         ← 必须同步
2. messages: INSERT assistant message    ← 必须同步
3. session_runtime_state: status + context + revision ← 必须同步（窄行）

异步路径：
- session_metrics: 聚合递增              ← 异步（EventBus 消费者）
- model_token_usage_events + 聚合表      ← 异步（EventBus 消费者）
- sessions.title 自动标题                ← 异步（已有机制）
```

**关键变更**：
- Session 聚合字段（message_count, token 统计等）从同步写入改为异步更新
- Usage 记录从同步事务改为异步 EventBus 消费者
- `session_revision` 仍同步递增（前端增量同步依赖）

**一致性影响**：
- 聚合字段有短暂不一致（异步延迟 ~100ms），对业务可接受
- 前端通过 `session_revision` 做增量同步，revision 变更后触发聚合字段刷新

#### 方案 3.2：Usage 记录异步化

**解决问题**：Usage 4 表事务持有 SQLite 写锁时间过长（问题 1、问题 7）

**核心思路**：将 Usage 记录从同步路径移到异步 EventBus 消费者。

```
当前：
  Chat Turn 同步路径 → RecordTokenUsageEvent() → [SQLite 事务: 4 表写入]

改造后：
  Chat Turn 同步路径 → EventBus.Publish(EnvelopeTypeTokenUsage, event)
                     → asyncConsumer → RecordTokenUsageEvent() → [SQLite 事务: 4 表写入]
```

**关键保证**：
- EventBus 使用 `Reliable: true` 注册，保证事件不丢失
- Usage 事件包含完整的 `sessionID` + `messageID`，可事后关联
- `TurnCompletionBridge` 机制保留，用于关联 Usage 和 Completion

**降级策略**：
- 异步消费者队列满时，回退到同步写入（保证用量不丢失）
- 监控异步写入延迟，超过阈值时告警

#### 方案 3.3：渠道场景原子化

**解决问题**：渠道消息的"任务创建→Session 创建→任务关联"非原子（问题 5）

**核心思路**：使用统一事务管理器，将渠道场景的多步操作包装在单个事务中。

```go
func (uc *ChannelTurnJobUsecase) CreateAndBindSession(ctx context.Context, ...) error {
    return uc.txManager.ExecInTx(ctx, func(txCtx context.Context) error {
        // 1. 创建渠道任务（Raw SQL 表）
        job, err := uc.turnJobRepo.CreateAccepted(txCtx, ...)
        if err != nil {
            return err
        }
        // 2. 创建 Session（Ent 表）
        sess, err := uc.sessionRepo.CreateSession(txCtx, ...)
        if err != nil {
            return err
        }
        // 3. 关联任务与 Session（Raw SQL 表）
        return uc.turnJobRepo.UpdateAsyncTarget(txCtx, job.ID, "session", sess.ID)
    })
}
```

---

### 维度四：业务需求层面 — 面向未来演进

#### 方案 4.1：记忆系统可扩展架构

**业务需求**：记忆系统需要持续演进（新增 L5 长期规划记忆、调整策略、A/B 测试记忆层级效果）。

**当前问题**：新增记忆层级需要在 `sessionmemory.Store` 中增加方法，违反开闭原则。

**解决方案**：基于 Repo 接口的插件化记忆架构。

```go
// biz 层定义记忆层级端口
type MemoryLayer interface {
    LayerName() string
    OnTurnComplete(ctx context.Context, input MemoryTurnInput) error
    Recall(ctx context.Context, query MemoryRecallQuery) ([]MemoryRecallResult, error)
}

// 具体层级实现
type L3FactLayer struct { repo L3FactRepo; consolidator MemoryConsolidator }
type L4GraphLayer struct { repo L4EntityRepo; decayer L4Decayer }
type L5PlanningLayer struct { repo L5PlanRepo }  // 未来新增

// MemoryUsecase 持有层级列表
type MemoryUsecase struct {
    layers []MemoryLayer  // 按优先级排序
    ...
}
```

**收益**：
- 新增记忆层级只需实现 `MemoryLayer` 接口 + 注册到 `layers`
- A/B 测试：动态调整 `layers` 列表，观察不同层级组合的效果
- 可独立禁用某个层级（如 L4 出问题时只保留 L3）

#### 方案 4.2：多租户数据隔离

**业务需求**：企业客户要求数据隔离（不同 workspace 的数据不可互访）。

**当前问题**：部分 Raw SQL 查询缺少 `workspace_id` 过滤条件，存在数据泄漏风险。

**解决方案**：

1. **Ent 层**：通过 Ent 的 `Privacy Policy` 自动注入 `workspace_id` 过滤
2. **Raw SQL 层**：建立 `WorkspaceScope` 中间件，自动在 WHERE 条件中追加 `workspace_id = ?`
3. **审计**：定期扫描 Raw SQL 查询，确保所有涉及 workspace 数据的查询都有过滤

#### 方案 4.3：主库迁移 PostgreSQL 评估

**业务需求**：支持 1000+ 并发用户的流畅对话体验。

**当前瓶颈**：SQLite 单写连接在 100+ 并发时成为瓶颈。

**评估维度**：

| 维度 | SQLite | PostgreSQL |
|------|--------|------------|
| 并发写入 | 单线程串行 | 多连接并行 |
| 事务隔离 | BEGIN IMMEDIATE | READ COMMITTED / SERIALIZABLE |
| 运维复杂度 | 零（文件数据库） | 中（需独立部署） |
| 部署成本 | 零 | 中（云 RDS 或自建） |
| 向量搜索 | 不支持 | pgvector 原生支持 |
| 全文搜索 | FTS5 | tsvector + GIN 索引 |
| JSON 操作 | json_extract/json_set | jsonb 操作符 |
| 备份恢复 | 文件复制 | pg_dump / PITR |

**迁移策略**：

```
Phase 0（当前）: SQLite 主库 + PostgreSQL 向量库
Phase 1: 高频写入表迁移到 PostgreSQL（usage, monitor, hook_delivery）
Phase 2: 核心业务表迁移（sessions, messages, agents）
Phase 3: 全量迁移，SQLite 仅用于单机开发/测试
```

**Phase 1 的优先迁移表**（写入频率高、对并发敏感）：

| 表 | 写入频率 | 迁移收益 |
|----|---------|----------|
| `model_token_usage_events` | 每次 LLM 调用 | 消除 4 表事务瓶颈 |
| `model_token_usage_daily/hourly` | 每次 LLM 调用 | 并行 UPSERT |
| `monitor_events` | 每轮对话 | 并行 INSERT |
| `hook_deliveries` | 每轮对话 | 并行 INSERT + 乐观锁 |
| `flow_log_events` | 每轮对话 | 并行 INSERT |

---

## 第四部分：实施路线图

### 总体优先级

```
P0（立即·1~2 周）────────────────────────────────────
  ├─ 接口拆分：TeamRepository → 5 个子接口
  ├─ 接口拆分：monitor.Repo → 7 个子接口
  ├─ 接口拆分：a2a.Repo → 4 个子接口
  └─ 统一事务管理器：定义接口 + Data 实现

P1（短期·2~4 周）────────────────────────────────────
  ├─ 第一批野生表纳入 Ent Schema（6 张高频表）
  ├─ Session 表冷热分离（新建 session_metrics + session_runtime_state）
  ├─ sessionmemory.Store 拆分为独立 Repo
  └─ Schema 迁移框架化

P2（中期·1~2 月）────────────────────────────────────
  ├─ 第二批野生表纳入 Ent Schema（6 张中频表）
  ├─ Usage 记录异步化
  ├─ 对话主路径写入优化（聚合字段异步更新）
  └─ Raw SQL Repo 读写分离

P3（长期·2~3 月）────────────────────────────────────
  ├─ 第三批野生表纳入 Ent Schema（7 张低频表）
  ├─ 记忆系统插件化架构
  ├─ 多租户数据隔离审计
  └─ 评估高频表迁移 PostgreSQL
```

### 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| Session 冷热分离导致查询性能回退 | 前端列表页变慢 | 保留 sessions 表的冗余聚合字段，异步同步到 session_metrics |
| 接口拆分导致 Wire 绑定变更量大 | 编译错误 | 逐模块拆分，每拆一个模块跑全量测试 |
| 野生表纳入 Ent 后 DDL 行为变化 | 数据丢失 | Ent Schema 使用 `WithDropIndex(true)` + 人工 review 生成代码 |
| Usage 异步化导致聚合延迟 | 前端显示不一致 | 前端通过 session_revision 轮询刷新，延迟 <500ms |
| sessionmemory.Store 拆分影响记忆管线 | 记忆提取失败 | 先并行运行新旧实现，对比结果一致后再切换 |

---

## 第五部分：核心设计原则

1. **业务驱动**：每个架构变更必须有明确的业务场景支撑，不为技术而技术。

2. **渐进演进**：不搞大重构，每个 Phase 独立可交付，可回滚。

3. **单一 Schema 真相源**：所有表必须进 Ent Schema，`go generate` 是唯一的 Schema 演进方式。对 Ent 不支持的特性，在 Ent Schema 中标注 `Annotations`，用 Raw Query 补充但不另建表。

4. **接口隔离到方法级**：每个 Repo 接口 ≤5 方法，按读写/职责域拆分。Wire 绑定时按需注入窄接口，消费方只看到自己需要的方法。

5. **同步路径最小化**：对话主路径只做必须同步的写入（消息 + 状态），聚合、监控、用量等全部异步化。

6. **最终一致优先**：除核心业务（消息、状态、用量）外，监控、审计、记忆等采用最终一致性，通过 EventBus + 死信队列保证可靠投递。

7. **数据访问收口**：所有数据访问必须经过 biz 层 Repo 接口，禁止"影子数据层"直接操作数据库。
