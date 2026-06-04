# 后端分层规范

> 来源：项目规则 + `aranea-coding-guide` SKILL 精简版。
> 架构上下文详见 [`architecture-blueprint.md`](./architecture-blueprint.md) §三。

---

## 一、依赖方向

**跨层只允许向内依赖。违反即停。**

```
api/**/*.proto → internal/service → internal/biz → internal/data
```

---

## 二、各层约束

### Server 层 (`internal/server/`)

| 规则 | 说明 |
|------|------|
| 只做传输注册 | `RegisterXxxHTTPServer` / `RegisterXxxServiceServer` |
| 中间件统一在此注册 | recovery → tracing → logging → auth → cors |
| 不得 new Runner | 不得 `runner.Runner` 或 `llmagent.New`（红线 #3） |
| 不得写业务路由 | 只做注册，不写逻辑（红线 #5） |

### Service 层 (`internal/service/`)

| 规则 | 说明 |
|------|------|
| proto ↔ biz 类型映射 | `toProtoXxx` / `fromProtoXxx` |
| Runner 装配唯一入口 | 唯一允许创建 Runner 的层（红线 #3） |
| 不得写业务逻辑 | Service 只做映射 + 编排（红线 #4） |
| 不得直接依赖 Repo | 通过 Usecase 层访问（红线 #13） |
| 错误映射用 `kerrors` | 禁止 `fmt.Errorf` |

当前共 38 个 Service struct（含 ChatService 实现 7 个 biz 端口接口）。

### Biz 层 (`internal/biz/`)

| 规则 | 说明 |
|------|------|
| 禁止 import `pkg/trpc-agent-go` | 框架交互通过 `internal/agent`/`internal/tools` 桥接（红线 #1） |
| 禁止 import `api/*/v1` | proto 映射只在 Service 层（红线 #2） |
| 定义 Repo 接口 | 接口在 biz 定义，data 层实现 |
| 定义跨模块端口接口 | 端口在 biz 定义，Wire 绑定在 service 层 |
| 错误用 `kerrors` | 禁止 `fmt.Errorf` 返回业务错误 |
| Repo 接口方法 ≤ 5 | 超过按职责域拆分子接口（红线 #15） |

当前共 36 个 Usecase struct，295 个接口定义分布在 100 个文件中。

### Data 层 (`internal/data/`)

| 规则 | 说明 |
|------|------|
| 仅通过 `d.Ent()` / `d.Postgres()` 访问 | 不得另开 SQLite 连接（红线 #11） |
| 编译期接口检查 | 65 条 `var _ biz.XxxRepo = (*xxxRepo)(nil)` 编译期接口检查，~60 个唯一 Repo/Adapter 实现 |
| 转换函数 | `entXxxToBiz` / `bizXxxToEnt` |

---

## 三、Agent 运行时集成铁律

| # | 铁律 | 正确做法 |
|---|------|---------|
| A1 | 所有 Agent 必须实现 `agent.Agent` 接口（5 方法） | `Run/Tools/Info/SubAgents/FindSubAgent` |
| A2 | 事件发射必须走 `agent.EmitEvent(ctx, inv, ch, evt)` | 禁止 `event.EmitEvent(context.Background(), ch, evt)` |
| A3 | Agent.Run() 内部不得发射 `ObjectTypeRunnerCompletion` | Runner 层统一发射 |
| A4 | 后台/定时 Agent 必须通过 `Runner.Run()` 调用 | 参考框架 `openclaw/internal/cron/service.go` |
| A5 | 工具构建使用 `function.NewFunctionTool[I, O]` | 禁止手动实现 `CallableTool` 接口 |
| A6 | 程序化 Agent 也必须走 Runner | Runner 管理 Session/Invocation/事件流生命周期 |
| A7 | 工具结果门控：`tool_result_gate` 控制 tool_result 事件是否推送给前端 | `ToolResultGate` 端口接口，Service 层实现 |

---

## 四、工具装配

新增工具流程：

1. `Registry()` 注册 `ToolRegistration`
2. `builtin_tools_seed.go` 添加种子
3. Chat/Team 共用同一 `BuildToolsets` 逻辑

装配顺序：Registry 注册 → 配置覆盖 → OpenAPI → workspace_exec → AgentTool → MCP ToolSet → MCP Broker → CustomTools

当前共 28 个注册工具 + ~37 个运行时注入工具（kanban/memory/subagent/modelsync/cli_admin/skills_butler/spirit_tools 等）。

---

## 五、记忆系统

### 5.1 两种工具路径

| 路径 | 工具 | 注入方式 |
|------|------|---------|
| 框架记忆工具 | memory_add/search/delete 等 6 个 | `memory.Service.Tools()` → 过滤 → `AssemblyConfig.MemoryTools` + `llmagent.WithMemoryService(service)` |
| L1 工作记忆工具 | working_memory.read/write/list/patch/delete 5 个 | `working_memory.ToolSet` → `BeforeToolHook` 注入 L1TaskWriter/L1FieldWriter/L1AdminReader/sessionID/agentID |

### 5.2 写入红线

- 记忆写入经 broker/async 异步写（红线 #8）
- L1 工作记忆工具写入是同步的（Agent 主动调用，非 plugin 回调）

### 5.3 五层架构

| 层级 | 存储 | 核心接口 | 关键 Worker |
|------|------|----------|-------------|
| L0 会话快照 | SQLite Session | `L0AdminStore` | 无 |
| L1 工作记忆 | SQLite Memory | `L1TaskWriter`(4) + `L1FieldWriter`(4) + `L1AdminReader`(4) | `MemoryL1ArchiveWorker`(5min) |
| L2 会话事件 | SQLite + pgvector | `L2RecallStore` + `L2EpisodeWriter` + `L2ConsolidationStore`(2) | `MemoryL2ConsolidateWorker`(10min) + `MemoryL2DecayWorker` |
| L3 语义知识 | SQLite + pgvector | `L3FactAdminStore` + `L3ConflictStore`(2) + `PIIReviewStore`(3) | `MemoryL3DecayWorker` + `MemoryFactIndexReconciler`(6h) |
| L4 持久进化 | SQLite Memory | `L4EntityStore`(5) + `L4EvolutionStore`(4) | `MemoryL4DecayWorker` |

### 5.4 关键数据流

- **L1→L2 桥接**：`EndL1Task` → `archiveAndCreateEpisode` → `ArchiveL1Task` + `InsertL1ArchiveEpisode`
- **L2 Consolidation**：Episode pending → `MemoryL2ConsolidateWorker` → consolidated（实际 LLM 提取由 AutoMemoryWorker 完成）
- **L3 冲突检测**：`UpsertFactRow` → `DetectFactConflicts`（best-effort）→ `IncrementConflictCount`
- **L3 PII 审核**：`ListPIIFlaggedFacts` / `ApprovePIIFact` / `RejectPIIFact`
- **L3 5维评分**：keyword(0.25) + vector(0.30) + importance(0.20) + recency(0.15) + quality(0.10)

### 5.5 SessionAdminStore 迁移

`SessionAdminStore`（38 方法）是向后兼容的组合接口，已标记 Deprecated。新代码应依赖细粒度子接口：
- `L0AdminStore`、`L1AdminReader`、`L1TaskWriter`、`L1FieldWriter`、`L1IdleTaskReader`
- `L2RecallStore`、`L2EpisodeWriter`、`L2ConsolidationStore`
- `L3FactAdminStore`、`L3ConflictStore`、`PIIReviewStore`
- `L4EntityStore`、`L4EvolutionStore`

---

## 六、Provider 集成

- 厂商连接收口在 `internal/provider`
- 契约对齐以 `pkg/trpc-agent-go/model` 为准
- 7 种 Provider：OpenAI/Anthropic/Gemini/Ollama/Hunyuan/HuggingFace/Bedrock
- HA 策略：Failover / Hedge

---

## 七、横切约束

| # | 约束 | 说明 |
|---|------|------|
| 1 | 所有 `go func()` 必须走 `pkg/safego` | 禁止裸 `go func()` 不处理 panic（红线 #9） |
| 2 | 禁止 `log/slog` | 统一使用 `pkg/loggateway.Logger`（红线 #16，原 #10） |
| 3 | 跨模块调用通过 biz 级窄接口 | 禁止持有对方 Service 具体类型（红线 #7） |
| 4 | 异步事件通过 Broker 发布/订阅 | 禁止全局变量共享状态 |
| 5 | 框架 plugin 回调不得直接写数据库 | 经 broker/async 异步写（红线 #8） |
| 6 | 压缩操作 CAS + 事务 | `TryIncrementCompressVersion` + `CompressSessionInTx`（红线 #14） |
| 7 | 不得修改工具生成的代码 | protoc/wire/Ent 等，改源头 → 重新生成（红线 #6） |
| 8 | 不得新增已无调用者的 deprecated 方法 | 死代码即删（红线 #12） |

---

## 八、Wire 依赖注入

- Wire ProviderSet：每层一个（`biz.go` / `data.go` / `service.go` / `server.go`）
  - biz.ProviderSet — 36 个 Usecase
  - data.ProviderSet — ~60 个 Repo/Adapter 实现
  - service.ProviderSet — 38 个 Service + 16 条 Wire 接口绑定
  - cmd/admin/wire.go 额外 19 条 wire.Bind（跨层绑定 + biz 子接口窄化绑定）。
- 构造函数参数：只接收接口或具体依赖，不接收"上帝对象"
- 禁止手动编辑 `wire_gen.go`，必须通过 `make wire` 生成
- 关键绑定详见 [`architecture-blueprint.md`](./architecture-blueprint.md) §八

---

## 九、错误处理

统一使用 `kerrors`，禁止 `fmt.Errorf` 返回业务错误。示例详见 [`architecture-blueprint.md`](./architecture-blueprint.md) §三"错误处理规范"。

---

## 十、验证命令

| 改动类型 | 最小验证 |
|---------|---------|
| 仅 Service + 单测 | `go test ./internal/service/... -run TestXxx -count=1` |
| 仅 Biz/Data | `go test ./internal/biz/... ./internal/data/... -count=1` |
| Proto 变更 | `make api && go build ./...` |
| Wire 注入 | `make wire && go build ./cmd/admin` |
| **提交前（全量）** | `make api && make wire && make build && make test && make lint` |

---

## memory-admin-interfaces (from data-architecture-overhaul)

### Requirement: SessionAdminStore deprecated interface migration
The `biz.SessionAdminStore` deprecated composite interface SHALL be replaced by its constituent sub-interfaces in all Wire bindings. Code that depends on `SessionAdminStore` SHALL be migrated to depend on the specific sub-interfaces it needs (e.g., `L0AdminStore`, `L1TaskWriter`, `L3FactReader`).

#### Scenario: Wire binding uses specific sub-interface
- **WHEN** a usecase needs L0 snapshot operations
- **THEN** it SHALL depend on `biz.L0AdminStore`, NOT `biz.SessionAdminStore`

#### Scenario: SessionAdminStore removed from Wire
- **WHEN** all consumers have been migrated to specific sub-interfaces
- **THEN** `biz.SessionAdminStore` SHALL be deleted

### Requirement: CascadeGraphStore split into sub-interfaces
The `biz.CascadeGraphStore` composite interface SHALL be split into `CascadeProposalRepo` and `CascadeSagaRepo`. Consumers SHALL depend on the specific sub-interface they need.

#### Scenario: Cascade proposal operations
- **WHEN** a usecase needs cascade proposal CRUD
- **THEN** it SHALL depend on `biz.CascadeProposalRepo` with methods: `InsertCascadeProposal`, `GetCascadeProposalRow`, `ListCascadeProposalRows`, `UpdateCascadeProposalStatus`

#### Scenario: Cascade saga operations
- **WHEN** a usecase needs cascade saga step management
- **THEN** it SHALL depend on `biz.CascadeSagaRepo` with methods: `InitCascadeSagaSteps`, `GetCascadeSagaSteps`, `UpdateSagaStepState`, `UpdateSagaStepResult`, `HasCascadeSaga`

#### Scenario: CascadeGraphStore removed
- **WHEN** all consumers have been migrated
- **THEN** `biz.CascadeGraphStore` SHALL be deleted

---

## memory-store-decomposition (from data-architecture-overhaul)

### Requirement: Store decomposition into independent Repos
The system SHALL decompose `sessionmemory.Store` (96 methods) into 6 independent Repo structs: `L0SnapshotRepo` (4 methods), `L1WorkingMemoryRepo` (8 methods), `L2EpisodeRepo` (12 methods), `L3FactRepo` (16 methods), `L4EntityRepo` (12 methods), `CascadeRepo` (14 methods). Each Repo SHALL hold `*Data` (not `*ent.Client`).

#### Scenario: Each Repo independently implements biz interfaces
- **WHEN** a biz layer usecase needs L3 fact operations
- **THEN** it SHALL depend on `biz.L3FactReader` / `biz.L3FactWriter` interfaces, implemented by `L3FactRepo`, without depending on other memory layer repos

#### Scenario: No Store.Client() backdoor
- **WHEN** any code needs to execute raw SQL against memory tables
- **THEN** it SHALL use `Data.ExecInTx` / `Data.ClientFromCtx` / `ReadWriteDB`, NOT `Store.Client()`

### Requirement: Wire adapter relocation
All data-layer adapters currently in `cmd/admin/wire_memory.go` SHALL be relocated to `internal/data/`. The `wireSessionAdminStoreAdapter` and `wireL3FactWriterAdapter` SHALL become `internal/data/memory_admin_adapter.go` and `internal/data/memory_l3_fact_writer_adapter.go`.

#### Scenario: Adapter in data layer
- **WHEN** Wire assembles the dependency graph
- **THEN** all data-layer adapters SHALL be in `internal/data/` package, not in `cmd/admin/`

### Requirement: Eliminate Store satisfying biz interfaces directly
`*sessionmemory.Store` SHALL NOT directly implement any biz interface. All biz interface satisfaction SHALL go through explicit adapter structs in `internal/data/`.

#### Scenario: SessionL2RecallStore via adapter
- **WHEN** `biz.MemoryL2RecallUsecase` needs `SessionL2RecallStore`
- **THEN** it SHALL receive an explicit `l2RecallAdapter` struct, NOT `*sessionmemory.Store`

### Requirement: Store method parameters use data-layer DTOs
Store method parameters that currently accept `biz.L0AssemblySnapshotInsert`, `biz.L1TaskInsert`, `biz.L1FieldInsert`, `biz.L1ArchiveEpisodeInsert`, `biz.ReinforcementSignal`, `biz.L4DecayConfig` SHALL be replaced with data-layer DTOs. Conversion SHALL happen in the adapter layer.

#### Scenario: L1 task insert with data DTO
- **WHEN** `L1WorkingMemoryRepo.StartL1Task` is called
- **THEN** it SHALL accept a `data.L1TaskInsert` DTO, and the adapter SHALL convert from `biz.L1TaskInsert` to `data.L1TaskInsert`

### Requirement: Shim migration phase
During migration, each new Repo SHALL delegate to the existing Store methods (shim pattern). This allows incremental migration without breaking existing functionality.

#### Scenario: L3FactRepo delegates to Store
- **WHEN** `L3FactRepo.UpsertFactRow` is called during shim phase
- **THEN** it SHALL delegate to `Store.UpsertFactRow` internally

#### Scenario: Store removal after full migration
- **WHEN** all Store methods have been migrated to independent Repos
- **THEN** the `sessionmemory.Store` struct SHALL be deleted

---

## session-repo-interfaces (from data-architecture-overhaul)

### Requirement: Session repo interfaces for split tables
The `biz.SessionRepo` composite interface SHALL be updated to include new sub-interfaces for `session_metrics` and `session_runtime` tables. The existing `SessionReader`, `SessionWriter`, `ContextUpdater` interfaces SHALL be modified to reflect the table split.

#### Scenario: SessionMetricsReader interface
- **WHEN** a usecase needs to read session metrics
- **THEN** it SHALL depend on `biz.SessionMetricsReader` interface with methods: `GetSessionMetrics`, `BatchGetSessionMetrics`

#### Scenario: SessionMetricsWriter interface
- **WHEN** the delta flush mechanism writes metrics
- **THEN** it SHALL depend on `biz.SessionMetricsWriter` interface with method: `ApplyMetricsDelta`

#### Scenario: SessionRuntimeReader interface
- **WHEN** a usecase needs to read session runtime state
- **THEN** it SHALL depend on `biz.SessionRuntimeReader` interface with methods: `GetSessionRuntime`, `GetSessionRevision`

#### Scenario: SessionRuntimeWriter interface
- **WHEN** runtime state changes during a turn
- **THEN** it SHALL depend on `biz.SessionRuntimeWriter` interface with methods: `PatchSessionState`, `UpdateRunnerSnapshot`, `BumpSessionRevision`

#### Scenario: SessionRepo composite includes new sub-interfaces
- **WHEN** `biz.SessionRepo` is used for Wire binding
- **THEN** it SHALL embed `SessionMetricsReader` + `SessionMetricsWriter` + `SessionRuntimeReader` + `SessionRuntimeWriter` in addition to existing sub-interfaces

---

## wild-table-ent-migration (from data-architecture-overhaul)

### Requirement: Batch 1 wild tables into Ent Schema
The system SHALL create Ent Schema definitions for the following 6 high-frequency tables: `session_runs`, `session_participants`, `session_run_checkpoints`, `channel_inbound_receipts`, `channel_turn_jobs`, `channel_runtime_lease`. These tables SHALL be managed by Ent's `Schema.Create` for new installations and DDL migration for existing installations.

#### Scenario: New installation creates tables via Ent
- **WHEN** a fresh database is initialized
- **THEN** Ent `Schema.Create` SHALL create these 6 tables with correct columns and indexes

#### Scenario: Existing installation migrates via DDL registry
- **WHEN** an existing database is upgraded
- **THEN** the DDL migration registry SHALL detect missing columns and add them via ALTER TABLE

### Requirement: Batch 2 memory tables into Ent Schema
The system SHALL create Ent Schema definitions for the following 6 memory tables: `memory_facts`, `memory_entities`, `memory_relations`, `memory_episodes`, `memory_l1_tasks`, `memory_l1_fields`. Complex queries (vector search, cascade, JSON aggregation) MAY remain as Raw SQL.

#### Scenario: Memory table schema defined in Ent
- **WHEN** a new column is added to `memory_facts`
- **THEN** the Ent Schema SHALL be the single source of truth for the column definition

#### Scenario: Complex queries remain Raw SQL
- **WHEN** a vector similarity search is needed
- **THEN** the Repo MAY use Raw SQL via `ReadWriteDB`, but the table structure SHALL be defined in Ent Schema

### Requirement: memory_chain.sql deduplication
The system SHALL remove table definitions from `memory_chain.sql` that overlap with Ent Schema definitions (23 tables). `memory_chain.sql` SHALL only contain the 34 Memory-specific tables not managed by Ent.

#### Scenario: Overlapping table removed from SQL file
- **WHEN** a table is defined in both Ent Schema and `memory_chain.sql`
- **THEN** the `memory_chain.sql` definition SHALL be removed, and Ent Schema SHALL be the single source of truth

### Requirement: DDL migration system SQL file support
The `ddl_migration_registry` SHALL support registering SQL file paths (embedded via `go:embed`) in addition to Go functions. This reduces inline SQL strings in Go code.

#### Scenario: Migration from SQL file
- **WHEN** a DDL migration is registered with a `SQL` field pointing to an embedded SQL file
- **THEN** the migration system SHALL read and execute the SQL file contents

### Requirement: Zero wild tables target
The long-term target SHALL be 0 wild tables (all 34 pure-wild tables managed by Ent Schema). Batch 3 (remaining ~28 tables after Batch 1 and 2) SHALL be migrated incrementally after Batch 1 and 2 are stable.

#### Scenario: Wild table count tracking
- **WHEN** a new table is added to the system
- **THEN** it MUST be defined in Ent Schema first, with no raw SQL CREATE TABLE allowed

---

## architecture (from team-graph-optimization)

### Requirement: GraphBuildConfig field count
The `GraphBuildConfig` struct SHALL contain 11 fields (down from 13). The `FailurePolicy *TeamFailurePolicy` and `ParallelBranchIDs []string` fields SHALL be removed.

#### Scenario: GraphBuildConfig has no Team domain concepts
- **WHEN** `GraphBuildConfig` is defined in `internal/biz/graph.go`
- **THEN** it SHALL NOT contain any field that references Team domain types (`TeamFailurePolicy`, `ParallelBranchIDs`)

#### Scenario: Graph runtime consumes only universal NodeDef fields
- **WHEN** Graph runtime processes a node failure
- **THEN** it SHALL read `NodeDef.FailureAction`, `NodeDef.FallbackAgent`, `NodeDef.RetryMaxAttempts` — NOT `GraphBuildConfig.FailurePolicy`

### Requirement: NodeDef field count
The `NodeDef` struct SHALL contain 20 fields (down from 28). The 8 Task metadata fields SHALL be moved to `NodeTaskMeta`.

#### Scenario: NodeDef contains only graph topology and universal failure fields
- **WHEN** `NodeDef` is defined in `internal/biz/graph.go`
- **THEN** it SHALL NOT contain: `RequiredRole`, `AssignmentMode`, `AssignmentStrategy`, `ReviewerAgent`, `ReviewRules`, `TimeoutSeconds`, `HeartbeatIntervalSeconds`, `EnableLeaseExtension`
