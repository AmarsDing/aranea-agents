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
- [ ] 2.7 创建 data 层 DTO 类型：在 `internal/data/memory/dto.go` 中定义 `L0SnapshotInsert`、`L1TaskInsert`、`L1FieldInsert`、`L1ArchiveEpisodeInsert`、`ReinforcementSignal`、`L4DecayConfig` 等 data 层 DTO，替代 biz 层类型
  - DoD: 6 个 DTO struct 定义完成，与 biz 层对应类型字段一致
- [ ] 2.8 各 Repo 方法参数改为 data 层 DTO：将 6 个 Repo 中接受 biz DTO 的方法参数替换为 data DTO，在 adapter 层做转换
  - DoD: `internal/data/memory/` 包不再 import `biz.L0AssemblySnapshotInsert` 等 6 个类型
- [ ] 2.9 Wire 适配器归位：将 `cmd/admin/wire_memory.go` 中的 `wireSessionAdminStoreAdapter` 和 `wireL3FactWriterAdapter` 移到 `internal/data/memory_admin_adapter.go` 和 `internal/data/memory_l3_fact_writer_adapter.go`
  - DoD: `cmd/admin/wire_memory.go` 不再包含 data 层适配器代码
- [ ] 2.10 消除 Store 直接满足 biz 接口：Store 当前直接满足 22 个 biz 接口（L0AdminStore, L1AdminReader, L1TaskWriter, L1FieldWriter, L1IdleTaskReader, L2EpisodeWriter, L2ConsolidationStore, L2RecallStore, L3FactReader, L3ConflictStore, PIIReviewStore, L4EntityStore, SessionL2RecallStore, SessionL3RecallStore, MemoryActionLogWriter, CascadeGraphReader, CascadeFactMutator, MemoryFactIndexMaintainer, MemoryEpisodeDecayer, MemoryFactDecayer, MemoryFactIndexCounter, L4DecayWriter）。为每个创建显式 adapter，替代 `*sessionmemory.Store` 直接作为实现
  - DoD: `biz` 层零直接引用 `sessionmemory.Store` 具体类型
- [ ] 2.11 更新 Wire 绑定：将所有 Store 相关的 Wire 绑定更新为使用新 Repo，`make wire && go build ./cmd/admin` 通过
  - DoD: `make wire && make build` 通过
- [ ] 2.12 移除 Store.Client()：删除 `sessionmemory.Store.Client()` 方法，`memory_migrate.go` 改为接受 `*Data` 参数
  - DoD: `Store` struct 无 `Client()` 方法，`memory_migrate.go` 使用 `Data.ClientFromCtx`

## 3. 接口拆分补全

- [ ] 3.1 拆分 `biz.monitor.Repo`（19 方法）为 4 个子接口：`MonitorEventRepo`（5）、`MonitorTraceRepo`（5）、`MonitorAlertRepo`（4）、`MonitorAuditRepo`（3）+ 组合接口 `MonitorRepo`
  - DoD: 编译期接口检查通过，消费者按需依赖窄接口
- [ ] 3.2 拆分 `biz.a2a.Repo`（14 方法）为 3 个子接口：`A2ACardRepo`（4）、`A2AInvocationRepo`（3）、`A2ARemoteRepo`（5）+ 组合接口 `A2ARepo`
  - DoD: 编译期接口检查通过
- [ ] 3.3 拆分 `biz.CascadeGraphStore` 为 `CascadeProposalRepo` + `CascadeSagaRepo`
  - DoD: 所有消费者迁移到子接口，`CascadeGraphStore` 标记 Deprecated
- [ ] 3.4 Delta 溢出安全阀：在 `SessionMetricsDelta` 中实现 `maxDeltaAge`（5 分钟）和 `maxDeltaCount`（1000）限制，超限强制 flush
  - DoD: 单元测试验证超限 flush 触发

## 4. Repo 读写分离全量迁移

- [ ] 4.1 迁移 9 个 Ent Repo 到 ReadWriteClient：`sessionRepo`、`agentRepo`、`teamRepo`、`channelRepo`、`systemSettingRepo`、`taskRepo`、`llmProviderModelRepo`、`toolRepo`、`usageRepo` — 将 `r.data.Ent()` / `r.readClient(ctx)` / `r.txClient(ctx)` 替换为 `r.rw.Read(ctx)` / `r.rw.Write(ctx)`
  - DoD: 9 个 Repo 不再包含 `r.data.Ent()` / `r.data.ReadEnt()` 调用
- [ ] 4.2 迁移 ~10 个写操作绕过 txClient 的 Repo（35+ 处调用）：`toolRepo`、`flowLogRepo`、`eventStoreRepo`、`avatarRepo`、`planRepo`、`graphRepo`、`graphRunRepo`、`taskRepo`、`channelRepo`、`seedVersionRepo`、`toolAuditRepo`、`taskLinkRepo` — 写操作改用 `r.rw.Write(ctx)`
  - DoD: 写操作在事务上下文中正确传播
- [ ] 4.3 迁移 ~13 个 Raw SQL Repo 到 ReadWriteDB（56 处读操作走写连接）：`a2aRepo`、`evalRepo`、`ecosystemRepo`、`monitorRepo`、`learningLoop` 系列、`skillProposalRepo`、`memoryJobDeadLetterRepo`、`sessionParticipantRepo`、`channelRuntimeLeaseRepo`、`compiledTeamRepo`、`sessionRunRepo`、`teamGraphSessionRepo`、`sessionRunCheckpointRepo` — 读操作改用 `r.rwDB.ReadDB(ctx)`
  - DoD: 15 个 Repo 的读操作走 `ReadDB()`，写操作走 `WriteDB()`
- [ ] 4.4 迁移 sessionmemory 子包到 ReadWriteClient/ReadWriteDB：Store 持有 `*Data` 而非 `*ent.Client`，通过 `Data.RW()` / `Data.RWDB()` 访问连接
  - DoD: Store 不再持有 `*ent.Client` 字段
- [ ] 4.5 全量验证：`make wire && make build && make test` 通过，所有 Repo 读写分离合规
  - DoD: `grep -r "r.data.Ent()" internal/data/` 返回零结果（除 Data struct 本身）

## 5. Session 表拆分

- [ ] 5.1 创建 Ent Schema `internal/data/ent/schema/session_metrics.go`：定义 `session_metrics` 表（session_id PK + 16 个聚合字段 + updated_at）
  - DoD: `go generate ./internal/data/ent` 成功
- [ ] 5.2 创建 Ent Schema `internal/data/ent/schema/session_runtime.go`：定义 `session_runtime` 表（session_id PK + revision + state_json + runner_snapshot + context 字段 + updated_at）
  - DoD: `go generate ./internal/data/ent` 成功
- [ ] 5.3 创建 DDL migration：`sql/migrations/YYYYMMDD_session_table_split.sql` — CREATE TABLE + 从 sessions 回填数据到 session_metrics 和 session_runtime
  - DoD: 在已有数据库上执行迁移后，新表数据与旧字段一致
- [ ] 5.4 注册 DDL migration 到 `ddl_migration_registry.go`
  - DoD: 新安装和升级安装均能正确创建新表
- [ ] 5.5 新增 biz 端口接口：`biz.SessionMetricsReader`、`biz.SessionMetricsWriter`、`biz.SessionRuntimeReader`、`biz.SessionRuntimeWriter`
  - DoD: 编译期接口检查通过
- [ ] 5.6 实现 data 层 Repo：`sessionMetricsRepo`（实现 SessionMetricsReader + SessionMetricsWriter）和 `sessionRuntimeRepo`（实现 SessionRuntimeReader + SessionRuntimeWriter）
  - DoD: 编译期接口检查通过
- [ ] 5.7 实现 SessionMetricsCache：LRU 缓存（容量 500，TTL 30s），包装 SessionMetricsReader，flush 时失效
  - DoD: 单元测试覆盖缓存命中/未命中/失效场景
- [ ] 5.8 Feature flag 实现：在 `internal/conf/` 中添加 `session_table_mode` 配置（legacy / dual_write / new_table），Data 层根据 flag 选择写入路径
  - DoD: 三种模式均可通过配置切换
- [ ] 5.9 修改 `sessionRepo`：在 dual_write 模式下同时写入旧字段和新表，在 new_table 模式下仅写入新表
  - DoD: `make test ./internal/data/... -run TestSession` 通过
- [ ] 5.10 修改 `toProtoSession`：从 sessions + session_metrics LEFT JOIN 聚合数据（或从缓存获取 metrics）
  - DoD: API 返回的 Session 对象包含完整 metrics 字段
- [ ] 5.11 新增 `EnvelopeTypeMetricsUpdated` 事件：在 `ApplyMetricsDelta` 完成后发布，包含 session_id 和更新的 metrics 字段
  - DoD: 前端 WebSocket 收到 MetricsUpdated 事件
- [ ] 5.12 前端适配：处理 `MetricsUpdated` 事件，更新本地 session metrics 状态；修复 `reconcilePatchFromServer` 旧值覆盖问题
  - DoD: 前端 `pnpm build` 通过，metrics 更新实时可见
- [ ] 5.13 数据一致性验证脚本：对比 sessions 旧字段与 session_metrics/session_runtime 新表数据，输出差异报告
  - DoD: 在 dual_write 模式下运行 24 小时后，差异为零

## 6. VectorStore 策略模式

- [ ] 6.1 创建 `internal/data/vector/store.go`：定义 `VectorStore` 接口（Upsert / Search / Delete）+ `VectorHit` struct
  - DoD: 接口定义完成
- [ ] 6.2 创建 `internal/data/vector/sqlite.go`：实现 `SQLiteVectorStore`，使用 SQLite JSON 列存储 + Go 侧余弦相似度
  - DoD: 单元测试覆盖 Upsert / Search / Delete
- [ ] 6.3 创建 `internal/data/vector/pgvector.go`：实现 `PgVectorStore`，使用 pgvector 扩展
  - DoD: 单元测试覆盖 Upsert / Search / Delete（需 Postgres 环境）
- [ ] 6.4 修改 `Data` struct：根据配置选择 VectorStore 实现，导出 `VectorStore() VectorStore` 方法
  - DoD: 无 Postgres 时自动降级到 SQLiteVectorStore
- [ ] 6.5 修改 memory_facts 表：DDL migration 添加 `embedding_ref TEXT` 列，后续版本移除 `embedding_blob` / `embedding_norm` 列
  - DoD: 新安装使用 `embedding_ref`，旧安装通过 migration 添加列
- [ ] 6.6 迁移 L3FactRepo / L2EpisodeRepo 的向量操作到 VectorStore
  - DoD: 向量搜索通过 VectorStore 接口执行，不再直接操作 embedding_blob

## 7. 野生表纳入 Ent（Batch 1）

- [ ] 7.1 创建 Ent Schema `session_runs.go`：定义 session_runs 表结构，注册到 `go generate`
  - DoD: `go generate ./internal/data/ent` 成功
- [ ] 7.2 创建 Ent Schema `session_participants.go`
  - DoD: 同上
- [ ] 7.3 创建 Ent Schema `session_run_checkpoints.go`
  - DoD: 同上
- [ ] 7.4 创建 Ent Schema `channel_inbound_receipts.go`
  - DoD: 同上
- [ ] 7.5 创建 Ent Schema `channel_turn_jobs.go`
  - DoD: 同上
- [ ] 7.6 创建 Ent Schema `channel_runtime_lease.go`
  - DoD: 同上
- [ ] 7.7 迁移 6 个 Raw SQL Repo 到 Ent API：`sessionRunRepo`、`sessionParticipantRepo`、`sessionRunCheckpointRepo`、`channelInboundReceiptRepo`、`channelTurnJobRepo`、`channelRuntimeLeaseRepo` — 优先使用 Ent CRUD API，仅保留特殊 UPSERT 逻辑为 Raw SQL
  - DoD: 6 个 Repo 使用 `ReadWriteClient`，简单 CRUD 走 Ent API
- [ ] 7.8 更新 Wire 绑定和 `data.ProviderSet`
  - DoD: `make wire && make build && make test` 通过

## 8. Schema 真相源统一

- [ ] 8.1 增强 `ddl_migration_registry.go`：支持 `SQL` 字段（嵌入 SQL 文件路径），在执行时读取并执行 SQL 文件内容
  - DoD: 单元测试验证 SQL 文件 migration 执行
- [ ] 8.2 从 `memory_chain.sql` 中删除与 Ent Schema 重叠的 23 张表定义
  - DoD: `memory_chain.sql` 仅包含 Memory 专属表（L0-L4 + cascade + action_log）
- [ ] 8.3 将 12 个 `*_patch.go` 中的 ALTER TABLE 逻辑迁移到 DDL migration SQL 文件
  - DoD: `internal/data/` 中无 `*_patch.go` 文件，所有 schema 变更通过 DDL migration 执行
- [ ] 8.4 创建迁移测试 helper：`internal/data/testhelper/migration.go`，提供 `SetupTestDB(t)` 函数，自动执行所有 DDL migration
  - DoD: 所有 data 层测试使用 `SetupTestDB(t)` 初始化，无需手动创建表

## 9. Store 独立化（删除 Store）

- [ ] 9.1 将 L0SnapshotRepo 的 shim 委托替换为直接实现：Raw SQL 通过 `ReadWriteDB` 执行
  - DoD: L0SnapshotRepo 不再引用 `sessionmemory.Store`
- [ ] 9.2 将 L1WorkingMemoryRepo 的 shim 委托替换为直接实现
  - DoD: L1WorkingMemoryRepo 不再引用 Store
- [ ] 9.3 将 L2EpisodeRepo 的 shim 委托替换为直接实现
  - DoD: L2EpisodeRepo 不再引用 Store
- [ ] 9.4 将 L3FactRepo 的 shim 委托替换为直接实现
  - DoD: L3FactRepo 不再引用 Store
- [ ] 9.5 将 L4EntityRepo 的 shim 委托替换为直接实现
  - DoD: L4EntityRepo 不再引用 Store
- [ ] 9.6 将 CascadeRepo 的 shim 委托替换为直接实现
  - DoD: CascadeRepo 不再引用 Store
- [ ] 9.7 删除 `internal/data/sessionmemory/` 包：所有代码迁移到 `internal/data/memory/` 后，删除旧包
  - DoD: `grep -r "sessionmemory" internal/` 返回零结果，`make build && make test` 通过
- [ ] 9.8 全量验证：`make api && make wire && make build && make test && make lint` 通过
  - DoD: 全部测试通过，无 lint 错误
