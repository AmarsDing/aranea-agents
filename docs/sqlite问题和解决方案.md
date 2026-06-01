# SQLite 读写业务逻辑深度分析报告

> 分析日期：2026-06-02
> 分析范围：`internal/data/` 全部 Repo 实现 + `internal/biz/` 全部 Repo 接口

---

## 一、全局概览

### 1.1 数据层规模

| 维度 | 数量 |
|------|------|
| Ent Schema | 54 个 |
| biz Repo 接口 | 117 个（含组合接口 17 个） |
| data Repo 实现 | 39 个 struct 实现 43 个接口 |
| 纯 Ent ORM 实现 | 18 个 (~46%) |
| 纯 Raw SQL 实现 | 10 个 (~26%) |
| Ent + Raw SQL 混合 | 2 个 (~5%) |
| sessionmemory.Store 适配器 | 6 个 (~15%) |
| Pgvector / PostgreSQL | 3 个 (~8%) |
| 文件系统 | 1 个 (~3%) |

### 1.2 双数据库架构

项目采用 **SQLite（主库）+ PostgreSQL（向量库）** 双数据库架构：

- **SQLite**：所有 CRUD 业务数据，通过 Ent ORM + 原生 SQL 访问
- **PostgreSQL**：仅用于 pgvector 向量存储（Agent 记忆、知识库向量搜索）
- **读写分离**：SQLite 有写连接（`entClient`, `MaxOpenConns=1`）和读连接（`readClient`, `MaxOpenConns=2`）

### 1.3 数据访问模式分布

| 模式 | 占比 | 典型模块 |
|------|------|----------|
| 纯 Ent ORM | ~46% | Agent、Admin、Channel(PeerSession)、Webhook、AgentCategory、MCPServer、EventStore |
| Ent + Raw SQL 混合 | ~5% | Session、Tool、SystemSetting、LlmProviderModel |
| 纯 Raw SQL | ~26% | Monitor、Channel(RuntimeLease/TurnJob/InboundReceipt)、A2A、Evaluation、Ecosystem、LearningLoop、Industry、Department、Position |
| sessionmemory.Store 适配器 | ~15% | L0/L2/L3/L4/Cascade/Composite 记忆子系统 |
| Pgvector | ~8% | Memory(向量)、Knowledge(向量+全文)、FactIndexSync |
| 文件系统 | ~3% | Artifact |

---

## 二、逐业务模块分析

### 2.1 Agent 领域 — 基本合规

**实现**：`internal/data/agent_repo.go` — 纯 Ent ORM

**接口**：`AgentRepository` = `AgentReader`(4) + `AgentWriter`(3) + `AgentRuntimeSettingsRepo`(2) + `AgentPromptFileRepo`(5) + 自有(3) = 17 方法

**业务逻辑正确性**：
- ✅ 读写分离：`readClient(ctx)` 读、`txClient(ctx)` 写
- ✅ 事务支持：`ExecInTx` 用于 Agent+RuntimeSettings 联合操作
- ✅ 接口拆分遵循红线 #15（子接口均 ≤5 方法）
- ⚠️ `normalizeJSONList` / `sanitizePromptFileID` 等辅助函数放在 data 层，属于业务逻辑应移至 biz 层

### 2.2 Session 领域 — 存在设计问题

**实现**：`internal/data/session_repo.go` — Ent + Raw SQL 混合

**接口**：`SessionRepo` = 17 个子接口组合 = **52 方法**（最大组合接口）

**业务逻辑正确性**：
- ✅ 读写分离做得好：`readClient(ctx)` / `txClient(ctx)`
- ✅ 压缩操作走 CAS + 事务（`TryIncrementCompressVersion` + `CompressSessionInTx`）
- ⚠️ `PatchSessionState` 使用 `json_set()`/`json_remove()` 原生 SQL，绕过 Ent 类型安全
- ⚠️ `UpdateSessionContextFromLLMUsage` 使用 `MAX()` + `CASE WHEN` 原生 SQL
- ⚠️ 52 方法的组合接口虽然子接口合规，但整体过于庞大

### 2.3 Memory 领域 — 架构最复杂

**实现**：涉及 6 个适配器 + `sessionmemory.Store` + `pgvector.Store`

**核心问题**：
- `sessionmemory.Store` 是一个 **"影子数据层"**，直接持有 `*ent.Client` 执行原生 SQL，绕过了标准 Repo 模式
- 记忆系统的 18+ 张表全部不在 Ent Schema 中，通过 `EnsurePatches` 手动 DDL 管理
- Schema 迁移散落在 `schema.go`、`store_cascade_saga.go`、`store_fact_embedding.go` 等多个文件
- `memoryFactIndexSync` 存在 **双写**（pgvector 向量 + SQLite embedding_blob），失败时标记 `stale`，但缺乏补偿机制

### 2.4 Usage 领域 — Raw SQL 比重过大

**实现**：`internal/data/usage_write.go` — 几乎全 Raw SQL

**核心问题**：
- `model_token_usage_events` 表有 50+ 列，用 Ent builder 极其冗长，因此全部走原生 SQL
- `upsertModelTokenUsageDaily`/`Hourly` 使用 `ON CONFLICT DO UPDATE` 加权平均计算，Ent 无法表达
- 事务内同时写 events + 更新 sessions 计数器 + upsert daily/hourly 聚合，**4 个写操作在一个事务中**，对 SQLite 单写连接压力大

### 2.5 Channel 领域 — 全 Raw SQL，无 Ent Schema

**实现**：4 个 Repo 中 3 个使用 `d.RawDB()` 原生 SQL

- `channelRuntimeLeaseRepo`：需要 `ON CONFLICT ... WHERE` 条件 upsert（分布式租约）
- `channelTurnJobRepo`：需要 `CASE WHEN` 条件 upsert
- `channelInboundReceiptRepo`：需要 `ON CONFLICT DO NOTHING`（幂等去重）
- `channelPeerSessionRepo`：纯 Ent（唯一走 Ent 的渠道 Repo）

**核心问题**：渠道核心表不在 Ent Schema 中，Schema 通过独立 `EnsureXxxSchema()` 函数管理

### 2.6 Monitor 领域 — 严重违规

**实现**：`internal/data/monitor.go` — 全 Raw SQL

**接口**：`monitor.Repo` = **20 方法**（未拆分，违反红线 #15）

**核心问题**：
- 3 张表（`audit_logs`、`monitor_events`、`monitor_traces`）不在 Ent Schema 中
- 使用 `json_extract()` 查询 JSON 字段内部值
- `monitor_alert.go` 使用 `BEGIN IMMEDIATE` 事务防并发
- `monitor_trace.go` 使用 `GENERATED ALWAYS AS ... VIRTUAL` 生成列
- **20 方法的单体接口未按红线 #15 拆分**

### 2.7 Team 领域 — 接口违规

**实现**：`internal/data/team_repo.go` — Ent + 部分 Raw SQL

**接口**：`TeamRepository` = **21 方法**（未拆分，违反红线 #15）

**核心问题**：
- 混合了 Team CRUD + TeamRun CRUD + TeamRunStep + OrchestrationStep + TaskDeadLetter 五个职责域
- 应拆分为 `TeamReader`/`TeamWriter`/`TeamRunRepo`/`OrchestrationStepRepo`/`TaskDeadLetterRepo`

### 2.8 A2A / Evaluation / Ecosystem / Learning Loop — 全 Raw SQL

这些模块完全绕过 Ent，直接持有 `*sql.DB`，自建 DDL 和 CRUD：
- A2A：4 张表，14 方法
- Evaluation：4 张表，含级联删除事务
- Ecosystem：2 张表
- Learning Loop：3 张表

---

## 三、架构和设计问题汇总

### 问题 1：双轨 Schema 管理（严重 🔴）

**现状**：54 张表走 Ent Schema 自动迁移，~25 张表走手动 DDL + `ALTER TABLE ADD COLUMN` 补丁。

**影响**：
- Schema 变更无法通过 `go generate` 统一管理
- 20+ 个 `*_patch.go` / `*_schema.go` 文件散落各处
- `PRAGMA table_info` 列检查 + `ALTER TABLE` 增量迁移模式重复出现 15+ 次
- 无法生成类型安全的 CRUD 代码

**涉及文件**：

| 文件 | 操作的表 | 迁移类型 |
|------|---------|----------|
| `sessionmemory/schema.go` | `tool_invocation_params` | ALTER TABLE ADD COLUMN |
| `memory_facts_index_patch.go` | `memory_facts` | ALTER TABLE + CREATE INDEX |
| `memory_l4_reinforcements_patch.go` | L4 增强表 | ALTER TABLE |
| `agent_runtime_patch.go` | `agent_runtime_settings` | ALTER TABLE |
| `hook_delivery_patch.go` | `hook_deliveries` | ALTER TABLE |
| `cascade_saga_patch.go` | `cascade_sagas` | ALTER TABLE |
| `pricing_patch.go` | `model_pricing_rules` | ALTER TABLE |
| `llm_provider_model_patch.go` | `llm_provider_models` | ALTER TABLE |
| `a2a_remote_patch.go` | `a2a_remote_agents` | ALTER TABLE |
| `system_setting_patch.go` | `system_settings` | ALTER TABLE |
| `team_run_patch.go` | `team_runs` | ALTER TABLE |
| `session_patch.go` | `sessions` | ALTER TABLE |
| `session_run_schema.go` | `session_runs` | CREATE TABLE |
| `session_run_schema_patch.go` | `session_runs` | ALTER TABLE |
| `session_participant_schema.go` | `session_participants` | CREATE TABLE |
| `team_graph_session_schema.go` | `team_graph_sessions` | CREATE TABLE |
| `channel_turn_job_schema.go` | `channel_turn_job` | CREATE TABLE |
| `channel_inbound_schema.go` | `channel_inbound_receipt` | CREATE TABLE |
| `message_fts_schema.go` | `messages_fts` | CREATE TABLE (FTS5) |
| `memory_chain_schema.go` | 记忆链表 | CREATE TABLE |

### 问题 2：Raw SQL 泛滥（严重 🔴）

**现状**：~40 个文件使用原生 SQL，占 data 层文件的 ~60%。

**根因分类**：

| 根因 | 占比 | 说明 |
|------|------|------|
| 表不在 Ent Schema | ~55% | 25 张表未进 Ent |
| SQLite 特有语法 | ~20% | `ON CONFLICT`、`json_set()`、`INSERT OR IGNORE` 等 |
| 复杂 SQL 表达式 | ~15% | 聚合、CASE WHEN、动态 IN |
| 大表（50+ 列） | ~10% | Ent builder 过于冗长 |

**Raw SQL 使用场景详细分类**：

| 场景 | 涉及文件数 | 典型语法 |
|------|-----------|----------|
| 非 Ent 表的完整 CRUD | ~25 | 全部 SQL 操作 |
| `ON CONFLICT DO UPDATE WHERE` | 3 | 条件 upsert（租约、TurnJob） |
| `INSERT OR IGNORE` / `INSERT OR REPLACE` | 4 | 幂等写入 |
| `json_set()` / `json_remove()` | 2 | 原子 JSON Patch |
| `json_extract()` | 3 | JSON 字段查询 |
| FTS5 `MATCH` / `bm25()` / `snippet()` | 1 | 全文搜索 |
| `BEGIN IMMEDIATE` | 1 | 事务隔离级别 |
| `GENERATED ALWAYS AS ... VIRTUAL` | 1 | 生成列 |
| `PRAGMA table_info` | 5 | Schema 检查 |
| 聚合 + 加权平均 | 3 | `COALESCE`、`NULLIF`、`GROUP BY` |
| 动态 WHERE 条件拼接 | 8 | 字符串拼接 SQL |
| 跨表批量 UPDATE | 2 | Provider 迁移 |
| 50+ 列 INSERT | 2 | `model_token_usage_events`、`agent_runtime_settings` |

### 问题 3：接口拆分不彻底（中等 🟡）

**违反红线 #15 的接口**：

| 接口 | 方法数 | 应拆分为 |
|------|--------|----------|
| `TeamRepository` | 21 | `TeamReader` + `TeamWriter` + `TeamRunRepo` + `OrchestrationStepRepo` + `TaskDeadLetterRepo` |
| `monitor.Repo` | 20 | `AuditReader` + `AuditWriter` + `EventReader` + `EventWriter` + `TraceReader` + `TraceWriter` + `AlertRepo` |
| `SessionAdminStore` | 20 | `L0AdminStore` + `L1AdminReader` + `L2RecallStore` + `L3FactAdminStore` + `L4GraphAdminStore`（已拆分子接口但组合接口仍暴露） |
| `a2a.Repo` | 14 | `CardRepo` + `InvocationRepo` + `AuditRepo` + `RemoteAgentRepo` |
| `SystemSettingRepo` | 11 | `SettingReader` + `SettingWriter` + `CredentialKeyRepo` |

### 问题 4：事务管理不统一（中等 🟡）

**现状**：4 种事务模式并存

| 模式 | 使用位置 | 机制 |
|------|---------|------|
| `d.ExecInTx()` | Ent Repo | context 传播 `txClientKey{}` → `tx.Client()` |
| `d.RawDB().BeginTx()` | Raw SQL Repo | 独立 `*sql.Tx` |
| `sqlRunner` 接口 | sessionmemory.Store | 第三种事务抽象 |
| `r.ent().BeginTx()` | usage_write.go | 第四种模式 |

**影响**：跨 Repo 事务无法协调，同一事务内 Ent 操作和 Raw SQL 操作无法原子提交

### 问题 5：读写分离不一致（轻微 🟢）

**现状**：
- Ent Repo 有 `readClient(ctx)` / `txClient(ctx)` 双 client
- Raw SQL Repo 直接用 `d.RawDB()`，无读副本
- `monitorRepo` 用 `d.RawDB()` 写、`d.RawDB()` 读，无读写分离

### 问题 6：sessionmemory.Store 架构定位模糊（严重 🔴）

**现状**：`sessionmemory.Store` 是一个 1000+ 行的"影子数据层"，直接操作 18+ 张表，绕过了 biz 层的 Repo 接口定义。

**影响**：
- 记忆系统的数据访问没有经过 biz 层接口抽象
- 无法替换存储后端（如测试时 mock）
- 与标准 Repo 模式不一致，增加理解成本

**涉及的表**：

| 表名 | 用途 |
|------|------|
| `memory_l0_assembly_snapshots` | L0 上下文装配快照 |
| `memory_l2_episodes` | L2 情景记忆 |
| `memory_l2_episode_index` | L2 情景嵌入索引 |
| `memory_l3_facts` | L3 事实记忆 |
| `memory_entities` | L4 实体 |
| `memory_relations` | L4 关系 |
| `memory_action_log` | 记忆操作日志 |
| `memory_facts` | 事实存储（与 Ent schema 重叠） |
| `cascade_proposals` | 级联提案 |
| `cascade_sagas` | 级联 Saga |
| `memory_l4_reinforcement_paths` | L4 增强路径 |

### 问题 7：SQLite 写瓶颈（架构性 🟡）

**现状**：写连接 `MaxOpenConns=1`，所有写操作串行化。

**影响**：
- Usage 写入（4 个写操作/事务）在高并发下成为瓶颈
- `retryOnBusy` 仅重试 3 次，间隔 100-300ms，可能不够
- 渠道租约、Hook 投递等高频写操作互相阻塞

---

## 四、完整改进方案

### 阶段一：统一 Schema 管理（优先级：P0）

**目标**：将所有 25 张"野生"表纳入 Ent Schema 管理。

**方案**：

```
1. 为每张野生表创建 Ent Schema：
   internal/data/ent/schema/
   ├── a2a_agent_card.go          ← 新增
   ├── a2a_invocation.go          ← 新增
   ├── a2a_audit.go               ← 新增
   ├── a2a_remote_agent.go        ← 新增
   ├── audit_log.go               ← 新增
   ├── monitor_event.go           ← 新增
   ├── monitor_trace.go           ← 新增
   ├── monitor_trace_span.go      ← 新增
   ├── monitor_alert_rule.go      ← 新增
   ├── channel_runtime_lease.go   ← 新增
   ├── channel_turn_job.go        ← 新增
   ├── channel_inbound_receipt.go ← 新增
   ├── session_run.go             ← 新增
   ├── session_run_checkpoint.go  ← 新增
   ├── session_participant.go     ← 新增
   ├── team_graph_session.go      ← 新增
   ├── eval_dataset.go            ← 新增
   ├── ecosystem_product.go       ← 新增
   ├── learning_observation.go    ← 新增
   ├── plugin_run.go              ← 新增
   ├── plugin_cost_guard_usage.go ← 新增
   ├── hook_delivery.go           ← 新增
   ├── memory_job_deadletter.go   ← 新增
   ├── position.go                ← 新增
   └── industry.go / department.go ← 新增

2. 运行 go generate ./internal/data/ent 生成类型安全代码

3. 逐步迁移 Raw SQL Repo → Ent Repo：
   - 简单 CRUD：直接替换为 Ent API
   - 复杂查询：保留 ent.Client.QueryContext()，但用 Ent 生成的类型做结果映射
   - SQLite 特有语法：通过 Ent 的 Raw Query + 类型映射保留
```

**对于 Ent 无法覆盖的场景**，采用以下策略：

| 场景 | 方案 |
|------|------|
| `ON CONFLICT DO UPDATE WHERE` | Ent 的 `OnConflictColumns` + `UpdateSet` |
| `INSERT OR IGNORE` | Ent 的 `OnConflictColumns` + 不更新 |
| `json_set()`/`json_remove()` | 保留 Raw SQL，但封装为 Repo 方法 |
| FTS5 全文搜索 | 保留 Raw SQL（Ent 不支持 FTS5） |
| pgvector 向量搜索 | 保留 Raw SQL（Ent 不支持向量） |
| `BEGIN IMMEDIATE` | 保留 Raw SQL（Ent 不支持事务隔离级别） |
| 50+ 列大表 | Ent 生成后用 `SetXxx()` 链式调用 |

### 阶段二：接口拆分合规化（优先级：P0）

**目标**：所有 Repo 接口方法数 ≤ 5。

**拆分方案**：

```go
// ─── TeamRepository 拆分 ───

type TeamReader interface {
    ListTeams(ctx) ([]Team, error)
    GetTeamByID(ctx, id) (Team, error)
    ListBySpiritSessionID(ctx, sid) ([]Team, error)
}

type TeamWriter interface {
    CreateTeam(ctx, Team) (Team, error)
    UpdateTeam(ctx, Team) (Team, error)
    DeleteTeam(ctx, id) error
}

type TeamRunRepo interface {
    ListTeamRuns(ctx, teamID, limit) ([]TeamRun, error)
    HasActiveTeamRun(ctx, teamID) (bool, error)
    GetTeamRunByID(ctx, id) (TeamRun, error)
    CreateTeamRun(ctx, TeamRun) (TeamRun, error)
    UpdateTeamRun(ctx, TeamRun) error
}

type OrchestrationStepRepo interface {
    BatchCreateOrchestrationSteps(ctx, []OrchestrationStep) error
    ListOrchestrationSteps(ctx, teamRunID, nodeID, limit) ([]OrchestrationStep, error)
}

type TaskDeadLetterRepo interface {
    CreateTaskDeadLetter(ctx, TaskDeadLetter) error
    ListTaskDeadLetters(ctx, filter) ([]TaskDeadLetter, error)
    ResolveTaskDeadLetter(ctx, id) (TaskDeadLetter, error)
}

// TeamRepository = TeamReader + TeamWriter + TeamRunRepo
//                   + OrchestrationStepRepo + TaskDeadLetterRepo
// Wire 绑定时按需注入窄接口

// ─── monitor.Repo 拆分 ───

type AuditLogReader interface { /* ListAuditLogs */ }
type AuditLogWriter interface { /* InsertAuditLog */ }
type MonitorEventReader interface { /* ListMonitorEvents, GetMonitorEvent */ }
type MonitorEventWriter interface { /* InsertMonitorEvent */ }
type MonitorTraceReader interface { /* ListMonitorTraces, GetMonitorTrace */ }
type MonitorTraceWriter interface { /* InsertMonitorTrace, UpsertMonitorTraceSpan, UpdateMonitorTraceCompletion */ }
type AlertRuleRepo interface { /* ListAlertRules, ReplaceAlertRules, UpdateAlertFiringState */ }
type MonitorCompletionRepo interface { /* CountMonitorEventsSince, AvgRunnerCompletionDurationMsSince, ... */ }

// monitor.Repo = 上述所有子接口组合

// ─── a2a.Repo 拆分 ───

type A2ACardRepo interface {
    UpsertAgentCard(ctx, AgentCard) (AgentCard, error)
    GetAgentCard(ctx, agentID) (AgentCard, error)
    ListEnabledCards(ctx, workspace, capability) ([]AgentCard, error)
    MapEndpointEnabled(ctx, agentIDs) (map[string]bool, error)
}

type A2AInvocationRepo interface {
    CreateInvocation(ctx, Invocation) (Invocation, error)
    UpdateInvocation(ctx, Invocation) error
}

type A2AAuditRepo interface {
    InsertAudit(ctx, AuditEntry) error
    ListAudit(ctx, callerID, calleeID, limit, offset) ([]AuditEntry, int, error)
}

type A2ARemoteRepo interface {
    CreateRemoteAgent(ctx, RemoteAgent) (RemoteAgent, error)
    ListRemoteAgents(ctx, workspace) ([]RemoteAgent, error)
    DeleteRemoteAgent(ctx, id) error
    GetRemoteAgent(ctx, id) (RemoteAgent, error)
    DiscoverRemoteCard(ctx, RemoteCardDiscoverInput) (AgentCard, error)
    UpdateRemoteAgentHealth(ctx, id, ok, errMsg) error
}
```

### 阶段三：统一事务管理（优先级：P1）

**目标**：一套事务机制覆盖 Ent + Raw SQL。

**方案**：

```go
// 统一事务接口
type TransactionManager interface {
    ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Data 实现 TransactionManager
func (d *Data) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
    tx, err := d.entClient.Tx(ctx)
    if err != nil {
        return err
    }
    txCtx := context.WithValue(ctx, txClientKey{}, tx.Client())
    txCtx = context.WithValue(txCtx, rawTxKey{}, tx)  // 同时传播 raw TX
    if err := fn(txCtx); err != nil {
        _ = tx.Rollback()
        return err
    }
    return tx.Commit()
}

// Raw SQL Repo 从 ctx 获取事务
func (r *someRawSQLRepo) dbFromCtx(ctx context.Context) DBOperations {
    if tx, ok := ctx.Value(rawTxKey{}).(*ent.Tx); ok {
        return tx.Client()  // 共享同一事务
    }
    return r.data.RawDB()
}
```

### 阶段四：sessionmemory.Store 重构（优先级：P1）

**目标**：将 `sessionmemory.Store` 的数据访问收口到 biz 层 Repo 接口。

**方案**：

```
1. 在 biz 层定义记忆子系统的 Repo 接口（已有部分）：
   - L0SnapshotRepo
   - L2EpisodeRepo
   - L3FactRepo
   - L4EntityRepo / L4RelationRepo
   - CascadeProposalRepo / CascadeSagaRepo
   - ActionLogRepo

2. sessionmemory.Store 拆分为多个独立 Repo 实现：
   internal/data/sessionmemory/
   ├── l0_snapshot_repo.go     ← 实现 biz.L0SnapshotRepo
   ├── l2_episode_repo.go      ← 实现 biz.L2EpisodeRepo
   ├── l3_fact_repo.go         ← 实现 biz.L3FactRepo
   ├── l4_entity_repo.go       ← 实现 biz.L4EntityRepo
   ├── cascade_repo.go         ← 实现 biz.CascadeProposalRepo + CascadeSagaRepo
   └── schema.go               ← 保留 DDL 管理

3. biz 层 MemoryUsecase 只依赖窄接口，不直接依赖 Store
```

### 阶段五：Schema 迁移框架化（优先级：P2）

**目标**：替代散落的 `*_patch.go` 模式，建立统一迁移框架。

**方案**：

```go
// internal/data/migration/registry.go
type Migration struct {
    Version      int
    Name         string
    Up           func(ctx context.Context, client *ent.Client) error
    Down         func(ctx context.Context, client *ent.Client) error  // 可选
    Dependencies []int  // 依赖的迁移版本
}

var registry = []Migration{
    {Version: 20260524, Name: "legacy_trpc_memory_facts", Up: migrateLegacyTRPCMemoryFacts},
    {Version: 20260528, Name: "turn_index_to_turn_id", Up: migrateTurnIndexToTurnID},
    {Version: 20260531, Name: "session_status_active_to_idle", Up: migrateSessionStatusIdle},
    // 将所有 *_patch.go 中的 ALTER TABLE 迁移纳入此处
}

func RunMigrations(ctx context.Context, client *ent.Client, lg loggateway.Logger) error {
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

### 阶段六：读写分离增强（优先级：P2）

**目标**：Raw SQL Repo 也支持读写分离。

**方案**：

```go
// Data 增加读 DB 访问方法（已有 ReadDB()）
// Raw SQL Repo 统一模式
type someRawSQLRepo struct {
    writeDB *sql.DB  // d.RawDB()
    readDB  *sql.DB  // d.ReadDB()
}

func (r *someRawSQLRepo) ListXxx(ctx) {
    // 读操作用 readDB
    r.readDB.QueryContext(ctx, ...)
}

func (r *someRawSQLRepo) CreateXxx(ctx) {
    // 写操作用 writeDB
    r.writeDB.ExecContext(ctx, ...)
}
```

### 阶段七：SQLite 写性能优化（优先级：P3）

**目标**：缓解单写连接瓶颈。

**方案**：

```
1. 批量写入优化：
   - Usage 事件写入改为批量 INSERT（攒批后一次写入）
   - Hook 投递记录批量插入

2. 异步写入扩展：
   - 已有 Broker/EventBus 异步模式（用于记忆写入）
   - 扩展到 Usage 事件写入：先写内存队列 → 异步批量入库

3. WAL 模式调优：
   - 当前 PRAGMA: journal_mode=WAL, synchronous=NORMAL
   - 考虑 PRAGMA wal_autocheckpoint=1000 减少检查点频率

4. 长期：评估是否需要迁移到 PostgreSQL 作为主库
```

---

## 五、实施优先级路线图

```
P0（立即）────────────────────────────────────────
  ├─ 接口拆分：TeamRepository / monitor.Repo / a2a.Repo
  ├─ 野生表纳入 Ent Schema（先做高频使用的表）
  └─ 统一事务管理接口

P1（短期）────────────────────────────────────────
  ├─ sessionmemory.Store 拆分为独立 Repo
  ├─ 剩余野生表纳入 Ent Schema
  └─ Raw SQL Repo 读写分离

P2（中期）────────────────────────────────────────
  ├─ Schema 迁移框架化
  ├─ 消除 data 层的业务逻辑函数
  └─ 统一错误处理模式

P3（长期）────────────────────────────────────────
  ├─ SQLite 写性能优化（批量写入、异步化）
  └─ 评估主库迁移 PostgreSQL
```

---

## 六、核心设计原则

如果重新设计，核心原则是：

1. **单一 Schema 真相源**：所有表必须进 Ent Schema，`go generate` 是唯一的 Schema 演进方式。对 Ent 不支持的特性（FTS5、pgvector、`BEGIN IMMEDIATE`），在 Ent Schema 中标注 `Annotations`，用 Raw Query 补充但不另建表。

2. **接口隔离到方法级**：每个 Repo 接口 ≤5 方法，按读写/职责域拆分。Wire 绑定时按需注入窄接口，消费方只看到自己需要的方法。

3. **统一事务传播**：一套 `TransactionManager` 覆盖 Ent + Raw SQL，通过 context 传播事务对象。

4. **数据访问收口**：`sessionmemory.Store` 拆分为独立 Repo，每个 Repo 只负责一张表或一个紧密关联的表组。biz 层定义接口，data 层实现。

5. **读写分离一致**：所有 Repo（Ent 和 Raw SQL）统一使用 `readDB`/`writeDB` 分离读操作和写操作。

6. **迁移框架化**：所有 Schema 变更（包括 `ALTER TABLE ADD COLUMN`）纳入统一迁移框架，有版本号、有依赖顺序、可回滚。

---

## 附录：Raw SQL 使用全景图

### A.1 直接 Raw SQL 文件（完全绕过 Ent）

| 文件 | 操作的表 | 使用原因 |
|------|---------|----------|
| `a2a.go` | `a2a_agent_cards` 等 4 张 | 非 Ent 表 |
| `evaluation.go` | `eval_datasets` 等 4 张 | 非 Ent 表 |
| `knowledge.go` | `knowledge_collections` 等 3 张 | PostgreSQL + pgvector |
| `pgvector/store.go` | `agent_memory_<dim>` | 动态表名 + 向量 |
| `channel_runtime_lease.go` | `channel_runtime_lease` | 条件 upsert |
| `position_repo.go` | `positions` | 非 Ent 表 + 多表 JOIN |
| `industry_repo.go` | `industries` | 非 Ent 表 |
| `department_repo.go` | `departments` | 非 Ent 表 |
| `monitor.go` | `audit_logs` 等 3 张 | 非 Ent 表 + json_extract |
| `monitor_alert.go` | `monitor_alert_rules` | BEGIN IMMEDIATE |
| `monitor_trace.go` | `monitor_traces` 等 2 张 | 生成列 |
| `memory_job_deadletter.go` | `memory_job_deadletter` | 条件 upsert |
| `memory_fact_reader.go` | `memory_facts` | 简单 SELECT |
| `hook_delivery.go` | `hook_deliveries` | 乐观锁 |
| `learning_loop.go` | 3 张学习循环表 | 非 Ent 表 |
| `model_registry_apply.go` | 多张 Ent 表 | 跨表批量 UPDATE |
| `session_run_repo.go` | `session_runs` | 条件 UPDATE + 乐观锁 |
| `session_participant_repo.go` | `session_participants` | 全量替换 |
| `session_run_checkpoint_repo.go` | `session_run_checkpoints` | 非 Ent 表 |
| `team_graph_session_repo.go` | `team_graph_sessions` | INSERT OR REPLACE |
| `channel_turn_job.go` | `channel_turn_job` | 条件 upsert |
| `channel_inbound_receipt.go` | `channel_inbound_receipt` | 幂等去重 |
| `ecosystem.go` | 2 张生态表 | 非 Ent 表 |
| `plugin_run.go` | `plugin_runs` | 非 Ent 表 |
| `plugin_cost_guard_usage.go` | `plugin_cost_guard_usage` | 原子累加 |
| `message_search.go` | `messages_fts` | FTS5 全文搜索 |

### A.2 Ent 代理 Raw SQL 文件（通过 ent.Client 执行）

| 文件 | 操作的表 | 使用原因 |
|------|---------|----------|
| `usage_write.go` | `model_token_usage_events` 等 | 50+ 列 + 加权平均 |
| `usage_daily.go` | `model_token_usage_daily` | 复杂聚合 |
| `usage_quota.go` | `model_token_usage_events` | 批量费用汇总 |
| `session_repo.go` (部分) | `sessions` | MAX + CASE WHEN |
| `session_state_repo.go` | `sessions` | json_set/json_remove |
| `session_repo_summaries.go` | `sessions` | 特定 SQL 构造 |
| `session_timeline.go` | 多张 | 时间线查询 |
| `tool_audit.go` | `tool_invocation_audit` | 非 Ent 表 |
| `tool.go` (部分) | 多张 | 复杂联表查询 |
| `team_repo.go` (部分) | 多张 | 批量删除 |
| `system_setting.go` (部分) | `system_settings` | 特定查询 |
| `sessionmemory/` (18 文件) | 18+ 记忆表 | 全部原生 SQL |
