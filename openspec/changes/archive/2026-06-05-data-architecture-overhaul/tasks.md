## Non-goals

- 不更换 SQLite 为 Postgres 作为主数据库
- 不重写前端
- 不引入分布式缓存（Redis 等）
- 不修改 Proto 定义
- 不在本次变更中实施完整的 Session 冷热分离

## 1. 基础设施：ReadWriteClient + 统一错误翻译 + DB 指标

- [x] 1.1 创建 `internal/data/readwrite.go`：实现 `ReadWriteClient` struct（持有 write/read `*ent.Client`），提供 `Read(ctx)` / `Write(ctx)` 方法，自动感知事务上下文
  - DoD: ✅ 单元测试覆盖 4 种场景（读/写 × 事务内/外），go build 通过，aranea-review 审查通过
- [x] 1.2 创建 `internal/data/readwrite_db.go`：实现 `ReadWriteDB` struct（持有 write/read `*sql.DB`），提供 `ReadDB(ctx)` / `WriteDB(ctx)` 方法，自动感知 `*sql.Tx` 上下文
  - DoD: ✅ 单元测试覆盖 4 种场景，go build 通过，aranea-review 审查通过
- [x] 1.3 修改 `Data` struct：新增 `rw *ReadWriteClient` 和 `rwDB *ReadWriteDB` 字段，在 `NewData` 中初始化；导出 `RW()` / `RWDB()` 方法
  - DoD: ✅ `go build ./internal/data/...` 和 `go build ./cmd/admin` 通过，aranea-review 审查通过
- [x] 1.4 创建 `internal/data/errors.go`：实现 `entErrToBizErr(err, domain, msg)` 函数，覆盖 NotFound / ConstraintError / NotLoaded / 默认 4 种翻译
  - DoD: ✅ 单元测试覆盖 4 种错误类型，go build 通过，aranea-review 审查通过
- [x] 1.5 创建 `internal/data/metrics.go`：定义 Prometheus histogram `aranea_db_query_duration_seconds`（labels: repo, operation, status）+ gauge `aranea_db_pool_*` 系列
  - DoD: ✅ `go build ./cmd/admin` 通过，aranea-review 审查通过
- [x] 1.6 创建 `internal/data/observe.go`：实现 `observeQuery(repo, operation string, fn func() error) error` 包装函数，记录 query latency + slow query 日志（>100ms）
  - DoD: ✅ 单元测试验证 latency 记录和 slow query 日志（4 测试通过），aranea-review 审查通过（无阻断项）

## 2. Store 收口：6 个 Memory Repo（Shim 阶段）

- [x] 2.1 创建 `internal/data/memory_shim_l0.go`：`l0SnapshotRepo` struct 持有 `*sessionmemory.Store`，实现 `biz.L0AdminStore` 接口（4 方法），委托到 Store
  - DoD: ✅ 编译期接口检查 `var _ biz.L0AdminStore = (*l0SnapshotRepo)(nil)` 通过，go build 通过，aranea-review 审查通过
- [x] 2.2 创建 `internal/data/memory_shim_l1.go`：`l1WorkingMemoryRepo` 实现 `biz.L1TaskWriter` + `biz.L1FieldWriter` + `biz.L1AdminReader` + `biz.L1IdleTaskReader`（10 方法），委托到 Store
  - DoD: ✅ 编译期接口检查通过（4 个接口），go build 通过，aranea-review 审查通过
- [x] 2.3 创建 `internal/data/memory_shim_l2.go`：`l2EpisodeRepo` 实现 `biz.L2EpisodeWriter` + `biz.L2ConsolidationStore` + `biz.L2RecallStore`（5 方法），委托到 Store
  - DoD: ✅ 编译期接口检查通过（3 个接口），go build 通过，aranea-review 审查通过
- [x] 2.4 创建 `internal/data/memory_shim_l3.go`：`l3FactRepo` 实现 `biz.L3FactReader` + `biz.L3FactWriter` + `biz.L3ConflictStore` + `biz.PIIReviewStore`（11 方法），委托到 Store（含 FactUpsert/DeleteFactByID 适配）
  - DoD: ✅ 编译期接口检查通过（4 个接口），go build 通过，aranea-review 审查通过
- [x] 2.5 创建 `internal/data/memory_shim_l4.go`：`l4EntityRepo` 实现 `biz.L4EntityStore` + `biz.L4EvolutionStore`（9 方法），委托到 Store（含 EvolutionEventInsert 适配）
  - DoD: ✅ 编译期接口检查通过（2 个接口），go build 通过，aranea-review 审查通过
- [x] 2.6 创建 `internal/data/memory_shim_cascade.go`：`cascadeRepo` 实现 `biz.CascadeProposalStore` + `biz.CascadeGraphReader` + `biz.CascadeFactMutator` + `biz.CascadeSagaStore`（14 方法），委托到 Store（含 CascadeProposalInsert/CascadeSagaStep 适配）
  - DoD: ✅ 编译期接口检查通过（4 个接口），go build 通过，aranea-review 审查通过
- [~] 2.7 创建 data 层 DTO 类型：在 `internal/data/memory/dto.go` 中定义 `L0SnapshotInsert`、`L1TaskInsert`、`L1FieldInsert`、`L1ArchiveEpisodeInsert`、`ReinforcementSignal`、`L4DecayConfig` 等 data 层 DTO，替代 biz 层类型
  - DoD: 6 个 DTO struct 定义完成，与 biz 层对应类型字段一致
  - **不再需要**：Store 独立化（Task 9）已完成，6 个 Repo 已直接实现 biz 接口（方法签名使用 biz 类型），无需额外 data DTO 层。biz 接口本身就是 data 层的契约，直接接受 biz 类型参数
- [~] 2.8 各 Repo 方法参数改为 data 层 DTO：将 6 个 Repo 中接受 biz DTO 的方法参数替换为 data DTO，在 adapter 层做转换
  - DoD: `internal/data/memory/` 包不再 import `biz.L0AssemblySnapshotInsert` 等 6 个类型
  - **不再需要**：同 2.7，Repo 直接实现 biz 接口，方法签名必须匹配 biz 接口定义。引入 data DTO 会破坏接口契约满足
- [x] 2.9 Wire 适配器归位：将 `cmd/admin/wire_memory.go` 中的 `wireSessionAdminStoreAdapter` 和 `wireL3FactWriterAdapter` 移到 `internal/data/memory_admin_adapter.go` 和 `internal/data/memory_l3_fact_writer_adapter.go`
  - DoD: ✅ `cmd/admin/wire_memory.go` 不再包含 data 层适配器代码，wire.go 第681行替换为 data.NewL3FactWriterAdapter，wire_gen.go 重新生成，go build 通过
- [x] 2.10 消除 Store 直接满足 biz 接口：Store 当前直接满足 22 个 biz 接口（L0AdminStore, L1AdminReader, L1TaskWriter, L1FieldWriter, L1IdleTaskReader, L2EpisodeWriter, L2ConsolidationStore, L2RecallStore, L3FactReader, L3ConflictStore, PIIReviewStore, L4EntityStore, SessionL2RecallStore, SessionL3RecallStore, MemoryActionLogWriter, CascadeGraphReader, CascadeFactMutator, MemoryFactIndexMaintainer, MemoryEpisodeDecayer, MemoryFactDecayer, MemoryFactIndexCounter, L4DecayWriter）。为每个创建显式 adapter，替代 `*sessionmemory.Store` 直接作为实现
  - DoD: ✅ `biz` 层零直接引用 `sessionmemory.Store` 具体类型，所有 biz 接口通过 data 层适配器桥接
- [x] 2.11 更新 Wire 绑定：将所有 Store 相关的 Wire 绑定更新为使用新 Repo，`make wire && go build ./cmd/admin` 通过
  - DoD: ✅ 修复 PackRepoAdapter 重复绑定、auth.Middleware 缺失参数，wire 重新生成，`go build ./cmd/admin` 通过
- [x] 2.12 移除 Store.Client()：删除 `sessionmemory.Store.Client()` 方法，`memory_migrate.go` 改为接受 `*Data` 参数
  - DoD: ✅ `Store` struct 无 `Client()` 方法，`memory_migrate.go` 使用 `Data.ClientFromCtx`，`runPendingDataMigrations` 接受 `*Data` 完整参数（消除部分构造 Data 的 nil 风险），编译时接口检查 + nil guard 已添加，测试通过

## 3. 接口拆分补全

- [x] 3.1 拆分 `biz.monitor.Repo`（20 方法）为 5 个子接口：`AuditRepo`（2）、`EventRepo`（4）、`TraceRepo`（6）、`AlertRepo`（3）、`RunnerCompletionRepo`（5）+ 组合接口 `MonitorRepo`（Deprecated）
  - DoD: ✅ 编译期接口检查通过，消费者按需依赖窄接口（TraceProjector→TraceRepo, MonitorTraceBackfillWorker→TraceRepo+RunnerCompletionRepo, DiagBundleGenerator→EventRepo+TraceRepo, RunnerErrorRateMetric→EventRepo），Wire 绑定更新，aranea-review 审查通过（无阻断项）
- [x] 3.2 拆分 `biz.a2a.Repo`（14 方法）为 4 个子接口：`CardRepo`（3）、`InvocationRepo`（2）、`AuditRepo`（2）、`RemoteAgentRepo`（7）+ 组合接口 `A2ARepo`（Deprecated）
  - DoD: ✅ 编译期接口检查通过，Usecase 持有 4 个子接口，Wire 绑定更新，aranea-review 审查通过（无阻断项）
- [x] 3.3 删除 `biz.CascadeGraphStore` 聚合接口和旧实现 `memory_cascade.go`，Wire 改用 `NewCascadeRepo`
  - DoD: ✅ `CascadeGraphStore` 已删除，`memory_cascade.go` 已删除，4 个子接口保留，消费者已全部迁移
- [x] 3.4 Delta 溢出安全阀：`MaxDeltaAge` 改为 5 分钟，`MaxDeltaCount` 改为 1000，超限强制 flush
  - DoD: ✅ 5 个单元测试覆盖（基本累积、计数溢出 flush、年龄溢出 flush、全量 flush、单条 flush）

## 4. Repo 读写分离全量迁移

- [x] 4.1 迁移 9 个 Ent Repo 到 ReadWriteClient：`sessionRepo`、`agentRepo`、`teamRepo`、`channelRepo`、`systemSettingRepo`、`taskRepo`、`llmProviderModelRepo`、`toolRepo`、`usageRepo` — 将 `r.data.Ent()` / `r.readClient(ctx)` / `r.txClient(ctx)` 替换为 `r.data.RW().Read(ctx)` / `r.data.RW().Write(ctx)`
  - DoD: ✅ 9 个 Ent Repo + 14 个额外 Repo 全部迁移到 RW() 模式，`r.data.Ent()` / `r.data.entClient` / `r.readClient(ctx)` / `r.txClient(ctx)` 调用已全部清除
- [x] 4.2 迁移 ~10 个写操作绕过 txClient 的 Repo（35+ 处调用）：`graphRepo`、`graphRunRepo`、`flowLogRepo`、`eventStoreRepo`、`avatarRepo`、`planRepo`、`seedVersionRepo`、`toolResultRepo`、`adminRepo`、`skillRepo`、`backgroundJobRepo`、`sessionTurnRepo`、`sessionRepoSummaries`、`sessionRepoBatch`、`sessionMessageFeedback` — 写操作改用 `r.data.RW().Write(ctx)`
  - DoD: ✅ 写操作在事务上下文中正确传播，辅助方法 `readClient`/`txClient`/`client`/`entClient` 已删除
- [x] 4.3 迁移 ~17 个 Raw SQL Repo 到 ReadWriteDB：`a2aRepo`、`evalRepo`、`ecosystemRepo`、`monitorRepo`、`learningLoop` 系列、`skillProposalRepo`、`memoryJobDeadLetterRepo`、`sessionParticipantRepo`、`channelRuntimeLeaseRepo`、`compiledTeamRepo`、`sessionRunRepo`、`teamGraphSessionRepo`、`sessionRunCheckpointRepo`、`hookDeliveryRepo`、`monitorAlertRepo`、`channelTurnJobRepo`、`messageSearch` — 读操作改用 `r.data.RWDB().ReadDB(ctx)`，写操作改用 `r.data.RWDB().WriteDB(ctx)`
  - DoD: ✅ 17 个 Repo 的读操作走 `ReadDB()`，写操作走 `WriteDB()`，`TxExecerFromCtx` fallback 改用 `WriteHandle()`，`QueryRowContext` 替换为 `queryRowScan`/`QueryContext`+`rows.Next()` 模式
- [x] 4.4 迁移 sessionmemory 子包到 ReadWriteClient/ReadWriteDB：Store 持有 `*Data` 而非 `*ent.Client`，通过 `Data.RW()` / `Data.RWDB()` 访问连接
  - DoD: ✅ `sessionmemory` 包已完全删除（Task 9.7），所有 Memory Repo 已直接使用 `Data.RWDB()` 执行 Raw SQL
- [x] 4.5 全量验证：`go build ./internal/data/...` + `go test ./internal/data/...` 通过，所有 Repo 读写分离合规
  - DoD: ✅ `grep -r "r.data.Ent()" internal/data/` 返回零结果，`grep -r "r.data.entClient" internal/data/` 返回零结果，`grep -r "r.data.RawDB()" internal/data/` 仅剩测试文件和种子数据

## 5. Session 表拆分

- [x] 5.1 创建 Ent Schema `internal/data/ent/schema/session_metrics.go`：定义 `session_metrics` 表（session_id PK + 19 个字段 + updated_at）
  - DoD: ✅ `go generate ./internal/data/ent` 成功，StorageKey("session_id") 映射
- [x] 5.2 创建 Ent Schema `internal/data/ent/schema/session_runtime.go`：定义 `session_runtime` 表（session_id PK + revision + state_json + runner_snapshot + metadata_json + compress_version + updated_at）
  - DoD: ✅ `go generate ./internal/data/ent` 成功
- [x] 5.3 创建 DDL migration：`sql/migrations/20260708_session_table_split.sql` — CREATE TABLE + 从 sessions 回填数据
  - DoD: ✅ SQL 包含 CREATE TABLE + INSERT OR IGNORE 回填
- [x] 5.4 注册 DDL migration 到 `ddl_migration_registry.go`
  - DoD: ✅ Version 20260708 已注册，`EnsureSessionTableSplit` 函数执行 SQL 文件
- [x] 5.5 新增 biz 端口接口：`SessionMetricsReader`（2方法）、`SessionMetricsWriter`（2方法）、`SessionRuntimeReader`（1方法）、`SessionRuntimeWriter`（1方法）+ `SessionMetrics`/`SessionRuntime` 数据模型
  - DoD: ✅ 编译期接口检查通过，类型别名已添加到 biz 包
- [x] 5.6 实现 data 层 Repo：`sessionMetricsRepo`（实现 SessionMetricsReader + SessionMetricsWriter）和 `sessionRuntimeRepo`（实现 SessionRuntimeReader + SessionRuntimeWriter）
  - DoD: ✅ 编译期接口检查通过，使用 Ent OnConflict upsert
- [x] 5.7 实现 SessionMetricsCache：sync.Map + TTL（30s），容量 500，包装 SessionMetricsReader，flush 时失效
  - DoD: ✅ 编译期接口检查通过，Invalidate/InvalidateAll 方法
- [x] 5.8 Feature flag 实现：`conf.DAOSessionMetricsTable()` / `conf.DAOSessionDualWrite()` / `conf.DAOSessionRuntimeTable()`，环境变量驱动
  - DoD: ✅ 三种模式均可通过配置切换（默认 false）
- [x] 5.9 修改 `sessionRepo`：在 dual_write 模式下同时写入旧字段和新表，在 DAOSessionMetricsTable 模式下仅写入新表
  - DoD: ✅ `ApplyMetricsDelta`/`UpdateSessionContextFromLLMUsage`/`UpdateSessionContextAfterCompression` 三种模式均已实现
- [x] 5.10 修改 `toProtoSession`：接受可选 `*biz.SessionMetrics` 参数，有 metrics 时用新表数据覆盖
  - DoD: ✅ 7 个调用点已更新，`getSessionMetrics` 辅助方法仅在 flag 启用时查询
- [x] 5.11 新增 `EnvelopeTypeMetricsUpdated` 事件：在 metrics flush 成功后发布，路由到 "chat" channel
  - DoD: ✅ `MetricsUpdatedPublisher` 接口 + `metricsUpdatedPublisher` 实现，`WireSessionStatusPublisher` 同时注入
- [x] 5.12 前端适配：处理 `MetricsUpdated` 事件，更新本地 session metrics 状态；修复 `reconcilePatchFromServer` 旧值覆盖问题
  - DoD: ✅ 前端 `envelope.ts` 已注册 `metrics_updated` 事件类型，`useChatInboundSync.ts` 收到事件后调用 `fetchAndReconcileSession`；`reconcilePatchFromServer` 使用 `Math.max` 策略防止旧值覆盖
- [x] 5.13 数据一致性验证脚本：对比 sessions 旧字段与 session_metrics/session_runtime 新表数据，输出差异报告
  - DoD: ✅ `cmd/session-consistency-check/main.go` 已实现，对比 17 个 metrics 字段 + 5 个 runtime 字段，检测缺失/孤立记录，差异为零时 exit 0

## 6. VectorStore 策略模式

- [x] 6.1 创建 `internal/data/vector/store.go`：定义 `VectorStore` 接口（Upsert / Search / Delete）+ `VectorHit` struct
  - DoD: ✅ 接口定义完成，`VectorHit` 包含 ID/Score/Meta 字段
- [x] 6.2 创建 `internal/data/vector/sqlite.go`：实现 `SQLiteVectorStore`，使用 SQLite JSON 列存储 + Go 侧余弦相似度
  - DoD: ✅ 单元测试覆盖 Upsert / Search / Delete，`sqlite_test.go` 存在
- [x] 6.3 创建 `internal/data/vector/pgvector.go`：实现 `PgVectorStore`，使用 pgvector 扩展
  - DoD: ✅ 实现 via build tag `pgvector`，stub 版本 `pgvector_stub.go` 在非 pgvector 构建时返回错误
- [x] 6.4 修改 `Data` struct：根据配置选择 VectorStore 实现，导出 `VectorStore() VectorStore` 方法
  - DoD: ✅ `DAOVectorPgVector()` 配置驱动，无 Postgres 时自动降级到 SQLiteVectorStore
- [x] 6.5 修改 memory_facts 表：DDL migration 添加 `embedding_ref TEXT` 列，后续版本移除 `embedding_blob` / `embedding_norm` 列
  - DoD: ✅ DDL migration `20260709_vector_embedding_ref.sql` 已添加 `embedding_ref TEXT DEFAULT ''` 列
- [x] 6.6 迁移 L3FactRepo / L2EpisodeRepo 的向量操作到 VectorStore
  - DoD: ✅ `l2EpisodeRepo` 和 `l3FactRepo` 持有 `vectorStore` 字段，recall 方法优先使用 VectorStore.Search，fallback 到本地 embedding_blob

## 7. 野生表纳入 Ent（Batch 1）

- [x] 7.1 创建 Ent Schema `session_runs.go`：定义 session_runs 表结构，注册到 `go generate`
  - DoD: ✅ `internal/data/ent/schema/session_run.go` 已创建，`go generate` 成功
- [x] 7.2 创建 Ent Schema `session_participants.go`
  - DoD: ✅ `internal/data/ent/schema/session_participant.go` 已创建
- [x] 7.3 创建 Ent Schema `session_run_checkpoints.go`
  - DoD: ✅ `internal/data/ent/schema/session_run_checkpoint.go` 已创建
- [x] 7.4 创建 Ent Schema `channel_inbound_receipts.go`
  - DoD: ✅ `internal/data/ent/schema/channel_inbound_receipt.go` 已创建
- [x] 7.5 创建 Ent Schema `channel_turn_jobs.go`
  - DoD: ✅ `internal/data/ent/schema/channel_turn_job.go` 已创建
- [x] 7.6 创建 Ent Schema `channel_runtime_lease.go`
  - DoD: ✅ `internal/data/ent/schema/channel_runtime_lease.go` 已创建
- [~] 7.7 迁移 6 个 Raw SQL Repo 到 Ent API：`sessionRunRepo`、`sessionParticipantRepo`、`sessionRunCheckpointRepo`、`channelInboundReceiptRepo`、`channelTurnJobRepo`、`channelRuntimeLeaseRepo` — 优先使用 Ent CRUD API，仅保留特殊 UPSERT 逻辑为 Raw SQL
  - DoD: 6 个 Repo 使用 `ReadWriteClient`，简单 CRUD 走 Ent API
  - **部分完成**：6 个 Repo 已使用 `ReadWriteClient`/`ReadWriteDB`（`r.data.RW().Read(ctx)`/`r.data.RWDB().ReadDB(ctx)`），但部分 Repo 仍混合使用 Ent API 和 Raw SQL（如 `sessionRunRepo` 的 `writeDB` 辅助方法用于特殊 UPSERT）。Ent Schema 已定义，Ent 生成代码已存在，但 CRUD 操作尚未完全迁移到 Ent API
- [x] 7.8 更新 Wire 绑定和 `data.ProviderSet`
  - DoD: ✅ Wire 绑定已更新，`data.ProviderSet` 包含 `NewSessionRunRepo`/`NewSessionParticipantRepo` 等

## 8. Schema 真相源统一

- [x] 8.1 增强 `ddl_migration_registry.go`：支持 `SQL` 字段（嵌入 SQL 文件路径），在执行时读取并执行 SQL 文件内容
  - DoD: ✅ `ddlMigration` struct 已包含 `SQL string` 字段，`executeSQLFile` 函数通过 `go:embed sql/migrations/*.sql` 读取并执行 SQL 文件，`splitDDLStatements` 分割语句，`isColumnExistsErr` 处理幂等性
- [x] 8.2 从 `memory_chain.sql` 中删除与 Ent Schema 重叠的 23 张表定义
  - DoD: ✅ `memory_chain.sql` 仅包含 26 张 Memory 专属表（L0-L4 + cascade + action_log + evolution），与 68 个 Ent Schema 无重叠
- [x] 8.3 将 12 个 `*_patch.go` 中的 ALTER TABLE 逻辑迁移到 DDL migration SQL 文件
  - DoD: ✅ 所有 ALTER TABLE 逻辑已迁移到 DDL migration（Go Func 或 SQL 文件）。`agent_runtime_patch.go` 仍存在但仅包含通用工具函数（`isColumnExistsErr`/`sqliteTableExists`/`sqliteColumnExists`/`sqliteIndexExists`），不含 ALTER TABLE 逻辑
- [x] 8.4 创建迁移测试 helper：`internal/data/testhelper/migration.go`，提供 `SetupTestDB(t)` 函数，自动执行所有 DDL migration
  - DoD: ✅ `SetupTestDB(t)` 已实现，创建内存 SQLite + Ent auto-migration + DDL，返回 `(*ent.Client, *sql.DB)`，注册 `t.Cleanup` 自动关闭

## 9. Store 独立化（删除 Store）

- [x] 9.1 将 L0SnapshotRepo 的 shim 委托替换为直接实现：Raw SQL 通过 `ReadWriteDB` 执行
  - DoD: ✅ `l0SnapshotRepo` 持有 `*Data`，通过 `r.data.RWDB()` 执行 Raw SQL，不再引用 `sessionmemory.Store`
- [x] 9.2 将 L1WorkingMemoryRepo 的 shim 委托替换为直接实现
  - DoD: ✅ `l1WorkingMemoryRepo` 持有 `*Data`，通过 `r.data.RWDB()` 执行 Raw SQL，不再引用 Store
- [x] 9.3 将 L2EpisodeRepo 的 shim 委托替换为直接实现
  - DoD: ✅ `l2EpisodeRepo` 持有 `*Data` + `vectorStore`，通过 `r.data.RWDB()` 执行 Raw SQL，recall 优先使用 VectorStore
- [x] 9.4 将 L3FactRepo 的 shim 委托替换为直接实现
  - DoD: ✅ `l3FactRepo` 持有 `*Data` + `vectorStore`，通过 `r.data.RWDB()` 执行 Raw SQL，recall 优先使用 VectorStore
- [x] 9.5 将 L4EntityRepo 的 shim 委托替换为直接实现
  - DoD: ✅ `l4EntityRepo` 持有 `*Data`，通过 `r.data.RWDB()` 执行 Raw SQL，不再引用 Store
- [x] 9.6 将 CascadeRepo 的 shim 委托替换为直接实现
  - DoD: ✅ `cascadeRepo` 持有 `*Data`，通过 `r.data.RWDB()` 执行 Raw SQL，不再引用 Store
- [x] 9.7 删除 `internal/data/sessionmemory/` 包：所有代码迁移到 `internal/data/` 后，删除旧包
  - DoD: ✅ `internal/data/sessionmemory/` 目录已不存在，`grep -r "sessionmemory" internal/data/` 仅剩注释引用（`memory_helpers.go` 中的 StepID 和注释），`sessionmemory` 包 import 已完全消除
- [x] 9.8 全量验证：`make api && make wire && make build && make test && make lint` 通过
  - DoD: 全部测试通过，无 lint 错误（注：lint 仅有 R10 main.go 行数超限的历史遗留问题，非本次变更引入）
