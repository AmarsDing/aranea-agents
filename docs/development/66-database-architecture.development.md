# Database Architecture — 开发计划

> **对应需求**：[66-database-architecture.md](./66-database-architecture.md)
> **对应设计**：[66-database-architecture.design.md](./66-database-architecture.design.md)
>
> **状态**：✅ 核心架构已落地（2026-06）

---

## 总览

数据库架构开发分为五大阶段：基础设施搭建、核心 Repo 实现、迁移系统建设、特殊特性实现、持续优化。

---

## Phase 1: 基础设施搭建

**目标**：建立 Data 层核心框架、连接管理、事务机制

### Task 1: Data 结构体与初始化

- [x] 实现 `Data` struct（SQLite 写连接 + 读连接 + PostgreSQL 连接）
- [x] 实现 `NewData()` 初始化流程
- [x] SQLite PRAGMA 设置（foreign_keys/WAL/busy_timeout/synchronous）
- [x] PostgreSQL 连接初始化（含降级逻辑）
- [x] Wire ProviderSet 定义

**Files**: `internal/data/data.go`, `internal/data/sqlite_db.go`, `internal/data/sqlite_path.go`

**验证**: `go build ./cmd/admin`

---

### Task 2: 读写分离

- [x] 实现 `ReadWriteClient`（Ent 读写分离）
- [x] 实现 `ReadWriteDB`（原生 SQL 读写分离）
- [x] 事务感知路由（ctx 中有事务 → 返回事务客户端）

**Files**: `internal/data/readwrite.go`, `internal/data/readwrite_db.go`

**验证**: `go test ./internal/data/... -run TestReadWrite -count=1`

---

### Task 3: 事务管理

- [x] 实现 `ExecInTx()`（嵌套检测 + 分离上下文 + 30s 超时 + 调用者取消检测）
- [x] 实现 `PostgresExecInTx()`
- [x] 实现 `spiritTransactorAdapter`（biz 层事务抽象）
- [x] 事务传播机制（`txClientKey` context key）

**Files**: `internal/data/tx.go`, `internal/data/spirit_transactor.go`

**验证**: `go test ./internal/data/... -run TestTx -count=1`

---

### Task 4: 错误转换与就绪门控

- [x] 实现 `entErrToBizErr()`（NotFound→404, Constraint→409, NotLoaded→400, 其他→500）
- [x] 实现 `ReadinessGate`（Pending→Ready/Failed 三态门控）

**Files**: `internal/data/errors.go`, `internal/data/readiness.go`

**验证**: `go test ./internal/data/... -count=1`

---

## Phase 2: 核心 Repo 实现

**目标**：实现所有业务领域的 Repo 接口和实现

### Task 5: Agent Repo

- [x] 实现 `AgentRepository` 组合接口（AgentReader + AgentWriter + AgentAtomicWriter + RuntimeSettings + PromptFile + ReferenceChecker）
- [x] 实现 `NewAgentRepo()`
- [x] 软删除过滤 + 索引优化

**Files**: `internal/data/agent_repo.go`, `internal/data/agent_runtime_patch.go`, `internal/data/agent_list_extras.go`, `internal/data/agent_performance_repo.go`, `internal/data/agent_template_repo.go`

---

### Task 6: Session Repo（最重）

- [x] 实现 `SessionRepo` 接口（会话 CRUD + 消息 CRUD + 搜索 + 统计）
- [x] 实现 `NewSessionRepo()`
- [x] 消息搜索（FTS5 + LIKE 回退）
- [x] Session 状态管理
- [x] Session 批量操作

**Files**: `internal/data/session_repo.go`, `internal/data/session_state_repo.go`, `internal/data/session_runtime_repo.go`, `internal/data/session_metrics_repo.go`, `internal/data/session_message_repo.go`, `internal/data/session_turn_repo.go`, `internal/data/session_participant_repo.go`, `internal/data/session_run_repo.go`, `internal/data/session_timeline.go`, `internal/data/session_repo_summaries.go`, `internal/data/session_repo_batch.go`, `internal/data/session_metrics_cache.go`, `internal/data/session_message_feedback.go`

---

### Task 7: Team Repo

- [x] 实现 `TeamRepository` 组合接口
- [x] 实现 `NewTeamRepo()`
- [x] CompiledTeam 管理

**Files**: `internal/data/team_repo.go`, `internal/data/compiled_team_repo.go`, `internal/data/team_graph_session_repo.go`

---

### Task 8: Channel Repo

- [x] 实现 Channel 相关 Repo（PlatformChannel + Credential + Delivery + PeerSession + InboundReceipt + TurnJob + RuntimeLease）

**Files**: `internal/data/channel.go`, `internal/data/channel_peer_session.go`, `internal/data/channel_inbound_receipt.go`, `internal/data/channel_turn_job.go`, `internal/data/channel_runtime_lease.go`

---

### Task 9: Memory Repo

- [x] 实现 Memory L0-L4 分层 Repo（memory_shim_l0 ~ memory_shim_l4）
- [x] 实现 `MemoryCompositeAdapter`
- [x] 实现 `MemoryAdminAdapter`
- [x] 实现 `MemoryMaintenanceAdapter`
- [x] 实现 Fact Index 同步
- [x] 实现 Episode 同步
- [x] 实现死信表管理

**Files**: `internal/data/memory.go`, `internal/data/memory_shim_l0.go` ~ `memory_shim_l4.go`, `internal/data/memory_composite_adapter.go`, `internal/data/memory_admin_adapter.go`, `internal/data/memory_maintenance_adapter.go`, `internal/data/memory_fact_index_sync.go`, `internal/data/memory_fact_reader.go`, `internal/data/memory_episode_sync.go`, `internal/data/memory_l3_scored_adapter.go`, `internal/data/memory_l4.go`, `internal/data/memory_migrate.go`, `internal/data/memory_chain_schema.go`, `internal/data/memory_job_deadletter.go`

---

### Task 10: 其他 Repo

- [x] Tool Repo（tool.go, tool_result_repo.go, tool_audit.go）
- [x] Skill Repo（skill.go, skill_dedup.go, skill_health.go, skill_intelligence.go, skill_invocation_stats.go, skill_evolution.go）
- [x] Usage Repo（usage.go, usage_write.go, usage_quota.go, usage_pricing.go, usage_hourly.go, usage_daily.go, usage_budget_alert.go）
- [x] Monitor Repo（monitor.go, monitor_trace.go, monitor_alert.go）
- [x] Graph Repo（graph.go）
- [x] Knowledge Repo（knowledge.go）
- [x] A2A Repo（a2a.go）
- [x] 其他（admin.go, avatar.go, cron.go, hook.go, plugin.go, evaluation.go, ecosystem.go, webhook.go, mcp_server.go, system_setting.go, llm_provider_model.go, pack_repo.go）

---

## Phase 3: 迁移系统建设

**目标**：建立三层迁移机制，确保 Schema 演进可控

### Task 11: Ent 自动迁移

- [x] 实现 `migrateDev()`（开发模式，允许删索引）
- [x] 生产模式 `Schema.Create()`（只增不减）
- [x] 67 个 Ent Schema 定义

**Files**: `internal/data/ent/schema/*.go`, `internal/data/data.go`

---

### Task 12: DDL 迁移注册表

- [x] 实现 `ddlMigration` 结构体
- [x] 实现 `ensureSchemaDDL()` 执行流程
- [x] 幂等设计（duplicate column/already exists 视为成功）
- [x] 注册 30+ 个迁移

**Files**: `internal/data/ddl_migration_registry.go`

---

### Task 13: 数据迁移

- [x] 实现 `runPendingDataMigrations()`
- [x] LegacyTRPCMemoryFacts 迁移（20260524）
- [x] TurnIndexToTurnID 迁移（20260528）
- [x] SessionStatusIdle 迁移（20260531）

**Files**: `internal/data/schema_migrations.go`

---

### Task 14: 级联删除

- [x] 实现 `cascadeDeleteByAgent()`
- [x] 实现 `cascadeDeleteBySession()`（事务内 14 个关联表）
- [x] 实现 `cascadeDeleteByTeam()`
- [x] 实现 `cascadeDeleteByChannel()`

**Files**: `internal/data/cascade_delete.go`

---

### Task 15: 种子数据

- [x] 实现 `lazySeeder` 延迟种子数据
- [x] P1 种子数据（InitialAdmin + 系统默认配置）

**Files**: `internal/data/lazy_seeder.go`, `internal/data/seed_*.go`, `internal/data/bootstrap_*.go`

---

## Phase 4: 特殊特性实现

**目标**：实现全文搜索、向量存储等特殊数据库特性

### Task 16: FTS5 全文搜索

- [x] 创建 `messages_fts` 虚拟表
- [x] 创建 INSERT/DELETE/UPDATE 触发器自动同步
- [x] 实现 `SearchMessages()`（FTS5 + snippet + bm25）
- [x] LIKE 回退策略

**Files**: `internal/data/sql/message_fts.sql`, `internal/data/message_fts_schema.go`, `internal/data/session_message_repo.go`

---

### Task 17: 向量存储

- [x] 定义 `VectorStore` 接口（Upsert/Search/Delete）
- [x] 实现 `SQLiteVectorStore`（JSON embedding + Go 侧余弦）
- [x] 实现 `PgVectorStore`（pgvector 扩展 + DB 侧余弦）
- [x] 选择逻辑（特性开关 + PostgreSQL 可用性）

**Files**: `internal/data/vector/store.go`, `internal/data/vector/sqlite.go`, `internal/data/vector/pgvector.go`

---

### Task 18: 记忆系统原生 SQL 表

- [x] 创建 L0-L4 记忆表（session_summaries, memory_l1_tasks, memory_episodes, memory_facts, memory_entities 等）
- [x] embedding_blob 字段 + embedding_norm
- [x] 审计表（memory_action_log, memory_cascade_proposals）
- [x] 演化表（agent_evolution_events, agent_evolution_proposals, agent_skill_stats）

**Files**: `internal/data/sql/memory_chain.sql`, `internal/data/memory_chain_schema.go`

---

### Task 19: 其他原生 SQL 表

- [x] flow_log 相关表
- [x] plugin_run 相关表
- [x] monitor_alert 相关表
- [x] learning_loop 相关表
- [x] skill_evolution 相关表
- [x] plan 相关表
- [x] hook_delivery 相关表
- [x] ecosystem_product 相关表
- [x] 死信表
- [x] 告警触发状态表

**Files**: `internal/data/sql/*.sql`, `internal/data/*_schema.go`

---

## Phase 5: 持续优化

**目标**：性能优化、索引补全、代码质量提升

### Task 20: 索引优化

- [x] 补缺失索引（DDL 迁移 20260716）
- [x] Session 关键索引（agent 维度、team 维度、last_message_at、deleted_at+user_id、deleted_at+status、parent/root session）
- [x] Message 关键索引（session_id, session_id+turn_id, session_id+turn_number, session_id+status）
- [x] ToolInvocation 关键索引（tool_key+started_at, agent_id+started_at, session_id, status, deleted_at）
- [x] Agent 关键索引（deleted_at, deleted_at+status, position_key+agent_variant UNIQUE）

---

### Task 21: Session 表拆分

- [x] Session 主表拆分为 session + session_runtime + session_metrics
- [x] 减少单表字段数（40+ → 核心字段）
- [x] DDL 迁移 20260708

**Files**: `internal/data/session_state_repo.go`, `internal/data/session_runtime_repo.go`, `internal/data/session_metrics_repo.go`

---

### Task 22: 代码质量

- [x] 错误转换统一（`entErrToBizErr`）
- [x] Wire ProviderSet 完整性
- [x] Repo 接口组合模式规范化
- [ ] Session Repo 方法拆分（待重构）
- [ ] pgvector 旧版存储清理（待清理）

---

## 验证清单

### 红线合规

- [x] `grep -r "ent\." internal/biz/` 为零（红线 #DB-1）
- [x] `grep -r "\.Edge(" internal/data/ent/schema/` 为零（红线 #DB-3）
- [x] DDL 迁移均注册到 `ddlMigrationRegistry`（红线 #DB-4）

### 功能验证

- [x] `go test ./internal/data/... -count=1` 通过
- [x] `make wire && go build ./cmd/admin` 通过
- [x] SQLite 读写分离正常
- [x] PostgreSQL 降级正常
- [x] FTS5 全文搜索正常
- [x] 向量搜索双实现正常

### 迁移验证

- [x] Ent 自动迁移正常
- [x] DDL 迁移幂等
- [x] 数据迁移正常
- [x] ReadinessGate 正常

---

## 已知偏差跟踪

| 编号 | 描述 | 状态 |
|------|------|------|
| DB-1 | Ent Schema 无 Edge，级联删除应用层实现 | 设计决策 |
| DB-2 | 时间戳用 String 而非 Time | 历史遗留 |
| DB-3 | pgvector 旧版存储待清理 | 待清理 |
| DB-4 | SQLite 写连接池 MaxOpen=1 | 设计决策 |
| DB-5 | Session Repo 方法过长 | 待重构 |
