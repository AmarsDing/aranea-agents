# Database Architecture — 开发计划

> **对应需求**：[66-database-architecture.md](./66-database-architecture.md)
> **对应设计**：[66-database-architecture.design.md](./66-database-architecture.design.md)
>
> **范围**：模块定位、代码锚点、现状评估、Phase 划分、任务清单与状态、验收标准、改动文件清单。

---

## 1. 模块定位

数据库架构是 Aranea-Agents 平台的数据持久化基座，为全部业务域（Agent/Team/Session/Message/Tool/Memory/Graph/Channel/Usage/Monitor 等）提供统一的数据访问能力。本模块包含连接管理、事务机制、迁移系统、Repo 模式、级联删除、向量存储、全文搜索等子能力。

**代码锚点**：

| 锚点 | 路径 | 说明 |
|------|------|------|
| Data 结构体 | `internal/data/data.go` | 连接管理、初始化、ProviderSet |
| 事务管理 | `internal/data/tx.go` | ExecInTx、PostgresExecInTx、事务传播 |
| 事务适配器 | `internal/data/spirit_transactor.go` | biz.SpiritTransactor 适配器 |
| 读写分离（Ent） | `internal/data/readwrite.go` | ReadWriteClient |
| 读写分离（SQL） | `internal/data/readwrite_db.go` | ReadWriteDB |
| 错误转换 | `internal/data/errors.go` | entErrToBizErr |
| 就绪门控 | `internal/data/readiness.go` | ReadinessGate |
| DDL 迁移注册表 | `internal/data/ddl_migration_registry.go` | 54 个 DDL 迁移 |
| 数据迁移 | `internal/data/schema_migrations.go` | 4 个数据迁移 |
| 级联删除 | `internal/data/cascade_delete.go` | Agent/Session/Team/Channel |
| 延迟种子 | `internal/data/lazy_seeder.go` | P1 种子数据 |
| SQLite 连接 | `internal/data/sqlite_db.go`, `internal/data/sqlite_path.go` | 连接辅助 |
| Ent Schema | `internal/data/ent/schema/*.go` | 82 个 Schema |
| 原生 SQL | `internal/data/sql/*.sql` | 13 个非迁移 SQL |
| 迁移 SQL | `internal/data/sql/migrations/*.sql` | 28 个版本化迁移 SQL |
| 向量存储 | `internal/data/vector/store.go`, `sqlite.go`, `pgvector.go` | VectorStore 双实现 |
| 旧版 pgvector | `internal/data/pgvector/` | 已废弃，待清理 |
| 记忆 Schema | `internal/data/memory_chain_schema.go` | L0-L4 表初始化 |
| FTS5 Schema | `internal/data/message_fts_schema.go` | 全文搜索初始化 |
| 分页/过滤 | `internal/biz/shared/shared.go` | PageToLimitOffset、ListOptions |
| 事务接口 | `internal/biz/spirit_team_usecase.go` | SpiritTransactor 接口 |
| 配置 Proto | `internal/conf/conf.proto` | Data 配置定义 |

---

## 2. 现状评估

### 2.1 已实现能力

| 能力 | 状态 | 说明 |
|------|------|------|
| SQLite 双连接池 | ✅ | 写=1, 读=2, WAL 模式, 5 个 PRAGMA |
| PostgreSQL 双连接池 | ✅ | 写=16, 读=32, 降级策略 |
| 事务管理（ExecInTx） | ✅ | 嵌套检测 + 分离上下文 + 30s 超时 + 调用者取消检测 + 双 key 注入 |
| PostgreSQL 事务 | ✅ | PostgresExecInTx 独立方法 |
| 事务抽象 | ✅ | SpiritTransactor 接口 + 适配器 |
| 读写分离（Ent） | ✅ | ReadWriteClient |
| 读写分离（SQL） | ✅ | ReadWriteDB |
| 错误转换 | ✅ | entErrToBizErr（含 Postgres 错误） |
| 就绪门控 | ✅ | ReadinessGate 三态 |
| Ent 自动迁移 | ✅ | 82 个 Schema |
| DDL 迁移注册表 | ✅ | 54 个迁移，版本化 + 幂等 |
| 数据迁移 | ✅ | 4 个数据迁移 |
| 级联删除 | ✅ | Agent/Session/Team/Channel |
| FTS5 全文搜索 | ✅ | 虚拟表 + 3 触发器 + LIKE 回退 |
| 向量存储双实现 | ✅ | SQLiteVectorStore + PgVectorStore |
| 记忆系统 L0-L4 | ✅ | 25 个原生 SQL 表 |
| 种子数据 | ✅ | lazySeeder + P1 种子 |
| Wire DI | ✅ | 80+ Provider |

### 2.2 已知技术债务

| 编号 | 问题 | 位置 | 严重度 | 状态 |
|------|------|------|--------|------|
| DB-DEBT-01 | `AgentRuntimeSetting` Schema 约 140 个字段，严重超标 | `ent/schema/agent_runtime_setting.go` | 高 | 🟡 待重构 |
| DB-DEBT-02 | `SessionRepo` 方法数 40+，远超 5 | `biz/session/usecase.go` | 中 | 🟡 待重构 |
| DB-DEBT-03 | pgvector 旧版存储已废弃但仍保留 | `internal/data/pgvector/` | 中 | 🟡 待清理 |
| DB-DEBT-04 | 时间戳用 String 而非 Time | 全部 Schema | 低 | 📋 设计决策 |
| DB-DEBT-05 | 部分 Repo 方法过长 | `session_repo.go` 等 | 低 | 🟡 待重构 |
| DB-DEBT-06 | `memory_l2_index_meta` 表已在 20260620 迁移中删除 | `memory_chain.sql` | 低 | ✅ 已清理 |

---

## 3. Phase 划分与任务清单

### Phase 1: 基础设施搭建 ✅

**目标**：建立 Data 层核心框架、连接管理、事务机制

#### Task 1.1: Data 结构体与初始化 ✅

- [x] 实现 `Data` struct（SQLite 写连接 + 读连接 + PostgreSQL 双连接池）
- [x] 实现 `NewData()` 初始化流程
- [x] SQLite PRAGMA 设置（foreign_keys/WAL/busy_timeout/synchronous/wal_autocheckpoint）
- [x] PostgreSQL 双连接池初始化（写=16, 读=32，含降级逻辑）
- [x] Wire ProviderSet 定义

**改动文件**：`internal/data/data.go`, `internal/data/sqlite_db.go`, `internal/data/sqlite_path.go`

**验证**：`go build ./cmd/admin`

#### Task 1.2: 读写分离 ✅

- [x] 实现 `ReadWriteClient`（Ent 读写分离）
- [x] 实现 `ReadWriteDB`（原生 SQL 读写分离）
- [x] 事务感知路由（ctx 中有事务 → 返回事务客户端）

**改动文件**：`internal/data/readwrite.go`, `internal/data/readwrite_db.go`

**验证**：`go test ./internal/data/... -run TestReadWrite -count=1`

#### Task 1.3: 事务管理 ✅

- [x] 实现 `ExecInTx()`（嵌套检测 + 分离上下文 + 30s 超时 + 调用者取消检测 + 双 key 注入）
- [x] 实现 `PostgresExecInTx()`
- [x] 实现 `spiritTransactorAdapter`（biz 层事务抽象）
- [x] 事务传播机制（`txClientKey` + `rawTxKey` 双 context key）
- [x] 事务感知辅助函数（`EntClientFromCtx`、`TxExecerFromCtx`）

**改动文件**：`internal/data/tx.go`, `internal/data/spirit_transactor.go`

**验证**：`go test ./internal/data/... -run TestTx -count=1`

#### Task 1.4: 错误转换与就绪门控 ✅

- [x] 实现 `entErrToBizErr()`（NotFound→404, Constraint→409, NotLoaded→400, 其他→500，含 Postgres 错误）
- [x] 实现 `ReadinessGate`（Pending→Ready/Failed 三态门控）

**改动文件**：`internal/data/errors.go`, `internal/data/readiness.go`

**验证**：`go test ./internal/data/... -count=1`

---

### Phase 2: 核心 Repo 实现 ✅

**目标**：实现所有业务领域的 Repo 接口和实现

#### Task 2.1: Agent Repo ✅

- [x] 实现 `AgentRepository` 组合接口（AgentReader + AgentWriter + AgentAtomicWriter + RuntimeSettings + PromptFile + ReferenceChecker）
- [x] 实现 `NewAgentRepo()`
- [x] 软删除过滤 + 索引优化

**改动文件**：`internal/data/agent_repo.go`, `internal/data/agent_runtime_patch.go`, `internal/data/agent_list_extras.go`, `internal/data/agent_performance_repo.go`, `internal/data/agent_template_repo.go`

#### Task 2.2: Session Repo（最重）✅

- [x] 实现 `SessionRepo` 接口（会话 CRUD + 消息 CRUD + 搜索 + 统计）
- [x] 实现 `NewSessionRepo()`
- [x] 消息搜索（FTS5 + LIKE 回退）
- [x] Session 状态管理
- [x] Session 批量操作

**改动文件**：`internal/data/session_repo.go`, `internal/data/session_state_repo.go`, `internal/data/session_runtime_repo.go`, `internal/data/session_metrics_repo.go`, `internal/data/session_message_repo.go`, `internal/data/session_turn_repo.go`, `internal/data/session_participant_repo.go`, `internal/data/session_run_repo.go`, `internal/data/session_timeline.go`, `internal/data/session_repo_summaries.go`, `internal/data/session_repo_batch.go`, `internal/data/session_metrics_cache.go`, `internal/data/session_message_feedback.go`, `internal/data/message_search.go`

#### Task 2.3: Team Repo ✅

- [x] 实现 `TeamRunRepo` 组合接口（TeamRunReader + TeamRunWriter）
- [x] 实现 `NewTeamRepo()`
- [x] CompiledTeam 管理

**改动文件**：`internal/data/team_repo.go`, `internal/data/compiled_team_repo.go`, `internal/data/team_graph_session_repo.go`

#### Task 2.4: Channel Repo ✅

- [x] 实现 Channel 相关 Repo（PlatformChannel + Credential + Delivery + PeerSession + InboundReceipt + TurnJob + RuntimeLease）

**改动文件**：`internal/data/channel.go`, `internal/data/channel_peer_session.go`, `internal/data/channel_inbound_receipt.go`, `internal/data/channel_turn_job.go`, `internal/data/channel_runtime_lease.go`

#### Task 2.5: Memory Repo ✅

- [x] 实现 Memory L0-L4 分层 Repo（memory_shim_l0 ~ memory_shim_l4）
- [x] 实现 `MemoryCompositeAdapter`
- [x] 实现 `MemoryAdminAdapter`
- [x] 实现 `MemoryMaintenanceAdapter`
- [x] 实现 Fact Index 同步
- [x] 实现 Episode 同步
- [x] 实现死信表管理

**改动文件**：`internal/data/memory.go`, `internal/data/memory_shim_l0.go` ~ `memory_shim_l4.go`, `internal/data/memory_composite_adapter.go`, `internal/data/memory_admin_adapter.go`, `internal/data/memory_maintenance_adapter.go`, `internal/data/memory_fact_index_sync.go`, `internal/data/memory_fact_reader.go`, `internal/data/memory_episode_sync.go`, `internal/data/memory_l3_scored_adapter.go`, `internal/data/memory_l4.go`, `internal/data/memory_migrate.go`, `internal/data/memory_chain_schema.go`, `internal/data/memory_job_deadletter.go`

#### Task 2.6: 其他 Repo ✅

- [x] Tool Repo（tool.go, tool_result_repo.go, tool_audit.go）
- [x] Skill Repo（skill.go, skill_dedup.go, skill_health.go, skill_intelligence.go, skill_invocation_stats.go, skill_evolution.go）
- [x] Usage Repo（usage.go, usage_write.go, usage_quota.go, usage_pricing.go, usage_hourly.go, usage_daily.go, usage_budget_alert.go）
- [x] Monitor Repo（monitor.go, monitor_trace.go, monitor_alert.go）
- [x] Graph Repo（graph.go）
- [x] Knowledge Repo（knowledge.go）
- [x] A2A Repo（a2a.go）
- [x] 其他（admin.go, avatar.go, cron.go, hook.go, plugin.go, evaluation.go, ecosystem.go, webhook.go, mcp_server.go, system_setting.go, llm_provider_model.go, pack_repo.go, organization_repo.go, activity_repo.go, borrow_request_repo.go, circuit_breaker_state_repo.go, evolution_suggestion_repo.go, evolution_metrics_repo.go, experience_report.go, failure_pattern_repo.go, heal_record_repo.go, self_check_report_repo.go, task_plan_repo.go, allocation_plan_repo.go, orchestration_repo.go, background_job.go, flow_log_repo.go, event_store_repo.go, learning_loop.go, plan.go, plugin_run.go, hook_delivery.go, unified_evolution.go）

---

### Phase 3: 迁移系统建设 ✅

**目标**：建立三层迁移机制，确保 Schema 演进可控

#### Task 3.1: Ent 自动迁移 ✅

- [x] 实现 `migrateDev()`（开发模式，允许删索引）
- [x] 生产模式 `Schema.Create()`（只增不减）
- [x] 82 个 Ent Schema 定义

**改动文件**：`internal/data/ent/schema/*.go`, `internal/data/data.go`

#### Task 3.2: DDL 迁移注册表 ✅

- [x] 实现 `ddlMigration` 结构体
- [x] 实现 `ensureSchemaDDL()` 执行流程
- [x] 幂等设计（duplicate column/already exists 视为成功）
- [x] 注册 54 个迁移（版本号 20260601 ~ 20260724）

**改动文件**：`internal/data/ddl_migration_registry.go`, `internal/data/sql/migrations/*.sql`

#### Task 3.3: 数据迁移 ✅

- [x] 实现 `runPendingDataMigrations()`
- [x] LegacyTRPCMemoryFacts 迁移（20260524）
- [x] TurnIndexToTurnID 迁移（20260528）
- [x] SessionStatusIdle 迁移（20260531）
- [x] OrganizationRedesign 迁移

**改动文件**：`internal/data/schema_migrations.go`, `internal/data/turn_index_migrate.go`, `internal/data/session_status_migrate.go`, `internal/data/organization_redesign_migrate.go`, `internal/data/trpc_memory_facts_test.go`

#### Task 3.4: 级联删除 ✅

- [x] 实现 `cascadeDeleteByAgent()`
- [x] 实现 `cascadeDeleteBySession()`（事务内 14 个关联表）
- [x] 实现 `cascadeDeleteByTeam()`
- [x] 实现 `cascadeDeleteByChannel()`

**改动文件**：`internal/data/cascade_delete.go`

#### Task 3.5: 种子数据 ✅

- [x] 实现 `lazySeeder` 延迟种子数据
- [x] P1 种子数据（InitialAdmin + 系统默认配置）

**改动文件**：`internal/data/lazy_seeder.go`, `internal/data/seed_*.go`, `internal/data/bootstrap_*.go`

---

### Phase 4: 特殊特性实现 ✅

**目标**：实现全文搜索、向量存储等特殊数据库特性

#### Task 4.1: FTS5 全文搜索 ✅

- [x] 创建 `messages_fts` 虚拟表
- [x] 创建 INSERT/DELETE/UPDATE 触发器自动同步
- [x] 实现 `SearchMessages()`（FTS5 + snippet + bm25）
- [x] LIKE 回退策略

**改动文件**：`internal/data/sql/message_fts.sql`, `internal/data/message_fts_schema.go`, `internal/data/session_message_repo.go`, `internal/data/message_search.go`

#### Task 4.2: 向量存储 ✅

- [x] 定义 `VectorStore` 接口（Upsert/Search/Delete）
- [x] 实现 `SQLiteVectorStore`（JSON embedding + Go 侧余弦）
- [x] 实现 `PgVectorStore`（pgvector 扩展 + DB 侧余弦）
- [x] 选择逻辑（特性开关 + PostgreSQL 可用性）

**改动文件**：`internal/data/vector/store.go`, `internal/data/vector/sqlite.go`, `internal/data/vector/pgvector.go`

#### Task 4.3: 记忆系统原生 SQL 表 ✅

- [x] 创建 L0-L4 记忆表（session_summaries, memory_l0_assembly_snapshots, memory_items, memory_l1_*, memory_episodes, memory_facts, memory_entities 等）
- [x] embedding_blob 字段 + embedding_norm
- [x] 审计表（memory_action_log, memory_cascade_proposals）
- [x] 演化表（agent_identity, agent_strategy_profile, agent_evolution_events, agent_evolution_proposals, agent_skill_stats）

**改动文件**：`internal/data/sql/memory_chain.sql`, `internal/data/memory_chain_schema.go`

#### Task 4.4: 其他原生 SQL 表 ✅

- [x] flow_log 相关表（flow_log_events）
- [x] plugin_run 相关表（plugin_runs）
- [x] monitor_alert 相关表（monitor_alert_rules）
- [x] learning_loop 相关表（learning_observations, learning_patterns, learning_proposals）
- [x] skill_evolution 相关表（skill_proposals）
- [x] plan 相关表（plans）
- [x] hook_delivery 相关表（hook_deliveries）
- [x] ecosystem_product 相关表（ecosystem_products, ecosystem_installs）
- [x] 死信表（memory_job_deadletter）
- [x] 告警触发状态列（ALTER TABLE 补丁）
- [x] 统一演化建议表（unified_evolution_suggestions）
- [x] 实体强化表（entity_reinforcements）
- [x] 级联 Saga 步骤表（cascade_saga_steps）
- [x] Postgres Phase 1 表（event_wal, event_store, session_run_checkpoints）
- [x] 用量事件表（model_token_usage_events, model_token_usage_daily）
- [x] 活动表（activities）

**改动文件**：`internal/data/sql/*.sql`, `internal/data/*_schema.go`

---

### Phase 5: 持续优化 🟡

**目标**：性能优化、索引补全、代码质量提升

#### Task 5.1: 索引优化 ✅

- [x] 补缺失索引（DDL 迁移 20260716）
- [x] Session 关键索引（agent 维度、team 维度、last_message_at、deleted_at+user_id、deleted_at+status、parent/root session）
- [x] Message 关键索引（session_id, session_id+turn_id, session_id+turn_number, session_id+status）
- [x] ToolInvocation 关键索引（tool_key+started_at, agent_id+started_at, session_id, status, deleted_at）
- [x] Agent 关键索引（deleted_at, deleted_at+status, position_key+agent_variant UNIQUE）

#### Task 5.2: Session 表拆分 ✅

- [x] Session 主表拆分为 session + session_runtime + session_metrics
- [x] 减少单表字段数
- [x] DDL 迁移 20260708

**改动文件**：`internal/data/session_state_repo.go`, `internal/data/session_runtime_repo.go`, `internal/data/session_metrics_repo.go`

#### Task 5.3: 代码质量 🟡

- [x] 错误转换统一（`entErrToBizErr`，含 Postgres 错误）
- [x] Wire ProviderSet 完整性
- [x] Repo 接口组合模式规范化
- [x] 事务双 key 注入（txClientKey + rawTxKey）
- [ ] Session Repo 方法拆分（待重构，DB-DEBT-02）
- [ ] pgvector 旧版存储清理（待清理，DB-DEBT-03）
- [ ] AgentRuntimeSetting Schema 拆分（待重构，DB-DEBT-01）

---

## 4. 验收标准

### 4.1 红线合规

- [x] `grep -r "ent\." internal/biz/` 为零（红线 #DB-1：biz 不直接依赖 ent）
- [x] `grep -r "\.Edge(" internal/data/ent/schema/` 为零（红线 #DB-3：无 Ent Edge，仅 Eval 域例外）
- [x] DDL 迁移均注册到 `ddlMigrations`（红线 #DB-4：无野生迁移）
- [x] 所有数据库错误经 `entErrToBizErr` 翻译（红线 DB-R5）
- [x] 事务均通过 `ExecInTx` 执行（红线 #DB-5，PostgreSQL 除外）
- [x] 未使用已废弃的连接访问器（红线 DB-R6：RawDB/ReadDB/ReadEnt/ReadClient）

### 4.2 功能验证

- [x] `go test ./internal/data/... -count=1` 通过
- [x] `make wire && go build ./cmd/admin` 通过
- [x] SQLite 读写分离正常
- [x] PostgreSQL 双连接池正常
- [x] PostgreSQL 降级正常
- [x] FTS5 全文搜索正常
- [x] 向量搜索双实现正常
- [x] 级联删除正常（Agent/Session/Team/Channel）
- [x] 三层迁移机制正常（Ent + DDL + 数据迁移）
- [x] ReadinessGate 三态门控正常

### 4.3 性能验证

- [x] SQLite 读写分离正常（写=1, 读=2）
- [x] PostgreSQL 双连接池正常（写=16, 读=32）
- [x] busy_timeout=30s 生效，无 SQLITE_BUSY 错误
- [x] wal_autocheckpoint=500 生效
- [x] FTS5 搜索性能优于 LIKE
- [x] 关键查询路径有索引覆盖

### 4.4 迁移验证

- [x] Ent 自动迁移正常（82 个 Schema）
- [x] DDL 迁移幂等（54 个迁移）
- [x] 数据迁移正常（4 个迁移）
- [x] ReadinessGate 正常

---

## 5. 改动文件清单

### 5.1 核心框架文件

| 文件 | 作用 |
|------|------|
| `internal/data/data.go` | Data 结构体、初始化、ProviderSet、Postgres 双连接池 |
| `internal/data/tx.go` | 事务管理、嵌套检测、上下文分离、双 key 注入 |
| `internal/data/spirit_transactor.go` | biz.SpiritTransactor 适配器 |
| `internal/data/readwrite.go` | ReadWriteClient（Ent 读写分离） |
| `internal/data/readwrite_db.go` | ReadWriteDB（原生 SQL 读写分离） |
| `internal/data/errors.go` | entErrToBizErr 错误转换 |
| `internal/data/readiness.go` | ReadinessGate 三态门控 |
| `internal/data/sqlite_db.go` | SQLite 连接辅助 |
| `internal/data/sqlite_path.go` | SQLite 路径解析 |

### 5.2 迁移系统文件

| 文件 | 作用 |
|------|------|
| `internal/data/ddl_migration_registry.go` | DDL 迁移注册表（54 个迁移） |
| `internal/data/schema_migrations.go` | 数据迁移门控 |
| `internal/data/turn_index_migrate.go` | turn_index → turn_id 迁移 |
| `internal/data/session_status_migrate.go` | session status active → idle 迁移 |
| `internal/data/organization_redesign_migrate.go` | 组织架构重设计迁移 |
| `internal/data/sql/migrations/*.sql` | 28 个版本化迁移 SQL 文件 |

### 5.3 级联删除与种子

| 文件 | 作用 |
|------|------|
| `internal/data/cascade_delete.go` | 级联删除（Agent/Session/Team/Channel） |
| `internal/data/lazy_seeder.go` | 延迟种子数据 |
| `internal/data/seed_*.go` | 种子数据实现 |
| `internal/data/bootstrap_*.go` | 启动引导数据 |

### 5.4 Ent Schema（82 个）

位于 `internal/data/ent/schema/`，按业务域组织：
- Agent 域：5 个（Agent, AgentPerformance, AgentPromptFile, AgentRuntimeSetting, AgentTemplate）
- Team 域：4 个（Team, CompiledTeam, TeamRun, TeamRunStep）
- Session 域：7 个（Session, SessionRun, SessionRunCheckpoint, SessionRuntime, SessionMetrics, SessionParticipant, SessionTurn）
- Message 域：1 个（Message）
- Tool 域：6 个（ToolInvocation, ToolInvocationAudit, ToolInvocationParam, ToolResultBlob, ToolResultReplacement, ToolAgentOverride）
- Channel 域：7 个（PlatformChannel, PlatformChannelCredential, PlatformChannelDelivery, PlatformChannelPeerSession, ChannelInboundReceipt, ChannelRuntimeLease, ChannelTurnJob）
- Graph 域：8 个（GraphDefinition, GraphExecution, GraphTask, GraphTaskComment, GraphTaskEvent, GraphTaskLink, GraphTaskLog, GraphTaskRun）
- Cron 域：2 个（CronTask, CronTaskRun）
- Memory 域：1 个（EventStore）
- Monitor 域：2 个（FlowLogEvent, SelfCheckReport）
- Usage 域：4 个（ModelPricingRule, ModelTokenUsageHourly, UsageQuota, BudgetAlert）
- Plugin 域：2 个（Plugin, Hook）
- Skill 域：5 个（PlatformSkill, SkillImportJob, SkillInvocation, SkillVersion, SkillEvolutionSuggestion）
- Ecosystem 域：4 个（PlatformTool, PlatformMcpServer, PlatformMcpUserCredential, AvatarAsset）
- Eval 域：4 个（EvalDataset, EvalCase, EvalCaseResult, EvalRun）
- System 域：20 个（Admin, SystemSetting, SchemaMigration, BackgroundJob, Orchestration, OrchestrationStep, AllocationPlan, TaskDeadLetter, TaskPlan, GatewayWebhook, HealRecord, EvolutionSuggestion, ExperienceReport, FailurePattern, UserEmbeddingSetting, LlmProviderModel, Organization, BorrowRequest, Activity, CircuitBreakerState）

### 5.5 原生 SQL 文件

| 文件 | 作用 |
|------|------|
| `internal/data/sql/memory_chain.sql` | 记忆系统 L0-L4 表（25 个表） |
| `internal/data/sql/message_fts.sql` | FTS5 全文搜索 |
| `internal/data/sql/flow_log.sql` | 流日志 |
| `internal/data/sql/plugin_run.sql` | 插件运行 |
| `internal/data/sql/monitor_alert.sql` | 监控告警 |
| `internal/data/sql/monitor_alert_firing_state.sql` | 告警触发状态列补丁 |
| `internal/data/sql/learning_loop.sql` | 学习循环 |
| `internal/data/sql/skill_evolution.sql` | 技能演化 |
| `internal/data/sql/plan.sql` | 计划管理 |
| `internal/data/sql/hook_delivery.sql` | Hook 投递 |
| `internal/data/sql/ecosystem_product.sql` | 生态产品 |
| `internal/data/sql/memory_job_deadletter.sql` | 记忆死信 |
| `internal/data/sql/unified_evolution.sql` | 统一演化 |

### 5.6 向量存储文件

| 文件 | 作用 |
|------|------|
| `internal/data/vector/store.go` | VectorStore 接口 |
| `internal/data/vector/sqlite.go` | SQLite 向量实现 |
| `internal/data/vector/pgvector.go` | PgVector 向量实现 |
| `internal/data/vector/pgvector_fact.go` | PgVector Fact 实现 |
| `internal/data/vector/pgvector_stub.go` | PgVector Stub（无 PG 时） |
| `internal/data/vector/pgvector_fact_stub.go` | PgVector Fact Stub |
| `internal/data/vector/parse.go` | 向量解析 |
| `internal/data/pgvector/` | 旧版 pgvector 存储（已废弃，待清理） |

### 5.7 Schema 初始化文件

| 文件 | 作用 |
|------|------|
| `internal/data/memory_chain_schema.go` | 记忆系统 Schema |
| `internal/data/message_fts_schema.go` | FTS5 Schema |
| `internal/data/flow_log_schema.go` | 流日志 Schema |
| `internal/data/plugin_run_schema.go` | 插件运行 Schema |
| `internal/data/compiled_team_schema.go` | 编译 Team Schema |
| `internal/data/session_run_schema.go` | Session 运行 Schema |
| `internal/data/session_participant_schema.go` | Session 参与者 Schema |
| `internal/data/channel_inbound_schema.go` | 渠道入站 Schema |
| `internal/data/channel_turn_job_schema.go` | 渠道轮次任务 Schema |
| `internal/data/skill_evolution_schema.go` | 技能演化 Schema |
| `internal/data/learning_loop_schema.go` | 学习循环 Schema |
| `internal/data/plan_schema.go` | 计划 Schema |
| `internal/data/self_check_report_schema.go` | 自检报告 Schema |
| `internal/data/team_graph_session_schema.go` | Team Graph Session Schema |
| `internal/data/unified_evolution_schema.go` | 统一演化 Schema |

---

## 6. 已知偏差跟踪

| 编号 | 描述 | 状态 |
|------|------|------|
| DB-DEBT-01 | `AgentRuntimeSetting` Schema 约 140 字段，严重超标 | 🟡 待重构 |
| DB-DEBT-02 | `SessionRepo` 方法数 40+，远超 5 | 🟡 待重构 |
| DB-DEBT-03 | pgvector 旧版存储待清理 | 🟡 待清理 |
| DB-DEBT-04 | 时间戳用 String 而非 Time | 📋 设计决策 |
| DB-DEBT-05 | 部分 Repo 方法过长 | 🟡 待重构 |
| DB-DEBT-06 | `memory_l2_index_meta` 表已删除 | ✅ 已清理 |
