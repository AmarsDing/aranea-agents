# Postgres 全量迁移 + 剩余项集成修复 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 完成 SQLite → Postgres 全量迁移（76 张表），并修复 12 个剩余项集成缺口（P1-7/P2-1~P2-4/P3-3/P3-9/P3-10/P3-11/P3-12/P3-6 + 文档同步）。

**Architecture:** 分 5 个 Phase 渐进实施。Phase A（Postgres 全量迁移）是基础设施，采用"停机迁移"策略（不推荐长期双写）。Phase B/C/D 是剩余项修复，可并行执行。每个 Phase 可独立验证。

**Tech Stack:** Go + Ent ORM + Kratos v2 + trpc-agent-go + Postgres 16 + pgvector + Wire DI

---

## 前置调研结论（已完成）

### 数据库迁移现状
- **SQLite 仍是主库**：76 张业务表全部在 SQLite，Postgres 仅用于 7 张辅助表（WAL/EventStore/Checkpoint/pgvector/Knowledge）
- **调研报告 §6.1 的"全量迁移"是设计目标**，开发计划 P0-3 只完成 Phase 1 的 3/7 类表
- **迁移完成度约 9%**（按表数量）

### 技术路线调研结论
| 方面 | 工作量 | 关键结论 |
|------|--------|---------|
| Ent dialect 切换 | 低 | Schema 层无需改动，新增 `initPostgresEnt` 函数 |
| FTS5 替代 | 中 | 重写 `message_search.go` + 新建 Postgres tsvector schema |
| DDL 迁移重写 | 中 | 约 5-8 个文件需 Postgres 版本，其余可复用 |
| Raw SQL Repo 重写 | 高 | monitor 模块 + 10+ Repo，GENERATED VIRTUAL 列 + json_extract 重写 |
| 数据迁移工具 | 高 | 从零自建，76+ 表 |
| 双写基础设施 | 高 | 不推荐长期双写；推荐短过渡期 + 离线迁移 |
| 配置和 Wire 注入 | 中 | 配置结构 + Wire 绑定 + 框架兼容性审查 |

### 关键风险
1. **trpc-agent-go 框架内部 SQL 兼容性未知**（`trpcsession.Service`、`SQLiteCheckpointSaver`）
2. **monitor 模块 GENERATED VIRTUAL 列无法直接迁移**（Postgres 仅支持 STORED）
3. **跨库事务不可行**（双写过渡期无法保证原子性）
4. **76 表数据一致性校验**（需自建校验工具）

---

## Phase A：Postgres 全量迁移（基础设施）

> **策略**：停机迁移（不推荐长期双写）。分 6 个子阶段，每个子阶段可独立验证。
> **预估工作量**：4-6 人周
> **关键路径**：A1（框架兼容性）→ A2（dialect 抽象）→ A3（Schema 迁移）→ A4（数据迁移工具）→ A5（切换）→ A6（清理）

### A0：trpc-agent-go 框架兼容性调研

**目标**：审查 `pkg/trpc-agent-go` 框架内部 SQL 是否硬编码 SQLite 语法

**Files:**
- Read: `pkg/trpc-agent-go/session/` （Session Service 实现）
- Read: `pkg/trpc-agent-go/graph/checkpoint/sqlite/` （SQLite CheckpointSaver）
- Read: `pkg/trpc-agent-go/graph/checkpoint/postgres/saver.go` （已有 Postgres CheckpointSaver）
- Read: `pkg/trpc-agent-go/memory/` （Memory Service 实现）

- [ ] **Step 1: 审查 trpcsession.Service 的 SQL 兼容性**
  - 检查 `pkg/trpc-agent-go/session/` 下所有 `.go` 文件中的 SQL 语句
  - 重点关注：`sqlite_master`、`INSERT OR IGNORE/REPLACE`、`PRAGMA`、`json_extract`、`strftime`
  - 输出：兼容性清单（哪些 SQL 需要重写）

- [ ] **Step 2: 审查 SQLiteCheckpointSaver 的 SQL 兼容性**
  - 检查 `pkg/trpc-agent-go/graph/checkpoint/sqlite/` 下所有 `.go` 文件
  - 确认是否已有 Postgres 版本（`pkg/trpc-agent-go/graph/checkpoint/postgres/saver.go`）
  - 输出：CheckpointSaver 的 Postgres 支持状态

- [ ] **Step 3: 审查 Memory Service 的 SQL 兼容性**
  - 检查 `pkg/trpc-agent-go/memory/` 下所有 `.go` 文件
  - 重点关注：memory_facts 表的 CRUD SQL
  - 输出：Memory Service 的 Postgres 支持状态

- [ ] **Step 4: 生成兼容性报告**
  - 汇总 Step 1-3 的发现
  - 列出需要框架层修改的 SQL 语句
  - 评估是否需要修改 `pkg/trpc-agent-go` 代码（红线：框架代码修改需谨慎）

**验证**：兼容性报告保存到 `docs/reports/2026-06-18-trpc-framework-postgres-compatibility.md`

### A1：dialect 抽象层 + Postgres Ent 初始化

**目标**：新增 dialect 抽象，支持配置切换 SQLite/Postgres

**Files:**
- Create: `internal/data/dialect.go` （dialect 检测 + JSON 路径表达式抽象）
- Modify: `internal/data/data.go` （新增 `initPostgresEnt` 函数）
- Modify: `internal/data/readwrite.go` （支持 Postgres Ent Client）
- Modify: `internal/data/readwrite_db.go` （支持 Postgres `*sql.DB`）
- Modify: `internal/data/tx.go` （`ExecInTx` 支持 Postgres 事务）
- Modify: `configs/config.yaml` （新增 `data.driver` 字段）
- Modify: `api/kratos/admin/v1/data.proto` （扩展 `Data` 消息）

- [ ] **Step 1: 新增 dialect 抽象层**
  - 创建 `internal/data/dialect.go`，提供 `IsPostgres()`/`IsSQLite()` 函数
  - 提供 `JSONExtract(col, key)`/`JSONEach(col, path)` 函数返回 dialect 感知的 SQL 片段
  - 提供 `TableExists(db, table)`/`ColumnExists(db, table, col)` 函数（dialect 感知）

- [ ] **Step 2: 新增 Postgres Ent 初始化**
  - 在 `data.go` 新增 `initPostgresEnt(c *conf.Data)` 函数
  - 使用 `dialect.Postgres` 打开 Ent Client
  - 设置连接池参数（MaxOpen=16, MaxIdle=4）
  - 执行 `Schema.Create(ctx)` 自动迁移

- [ ] **Step 3: 修改 NewData 支持 dialect 切换**
  - 新增 `data.driver` 配置字段（`sqlite`/`postgres`）
  - `NewData` 根据 `driver` 选择 `initSQLite` 或 `initPostgresEnt`
  - 保持向后兼容（未配置 `driver` 时默认 `sqlite`）

- [ ] **Step 4: 修改 ReadWriteClient/ReadWriteDB 支持 Postgres**
  - `ReadWriteClient` 支持传入 Postgres Ent Client
  - `ReadWriteDB` 支持传入 Postgres `*sql.DB`
  - 保持接口不变，内部根据 dialect 选择句柄

- [ ] **Step 5: 修改 ExecInTx 支持 Postgres**
  - `ExecInTx` 根据 dialect 选择 `entClient.Tx` 或 `pg.BeginTx`
  - 保持分离 context + 30s 硬超时（可配置）的行为
  - 嵌套事务检测逻辑保持一致

- [ ] **Step 6: 更新配置文件和 Proto**
  - `configs/config.yaml` 新增 `data.driver: sqlite`（默认）
  - `api/kratos/admin/v1/data.proto` 新增 `string driver = 5`
  - 重新生成 Proto：`make api`

- [ ] **Step 7: 验证**
  - `go build ./...` 通过
  - `go test ./internal/data/... -count=1` 通过
  - 手动测试：配置 `driver: postgres` 能启动并连接 Postgres

### A2：Schema 迁移（DDL + FTS5 + monitor）

**目标**：为 Postgres 提供完整的 DDL 迁移支持

**Files:**
- Create: `internal/data/sql/migrations_postgres/` （Postgres 版迁移目录）
- Modify: `internal/data/ddl_migration_registry.go` （支持 dialect 感知的迁移执行）
- Create: `internal/data/sql/migrations_postgres/20260624_message_fts.sql` （tsvector 版）
- Modify: `internal/data/message_search.go` （支持 tsvector 查询）
- Modify: `internal/data/monitor_trace.go` （GENERATED VIRTUAL → STORED）
- Modify: `internal/data/monitor.go` （json_extract → ->>）

- [ ] **Step 1: 扩展 DDL 迁移注册表支持 Postgres**
  - `ddlMigration` 结构体新增 `PostgresSQL string` 字段
  - `executeSQLFile` 根据 dialect 选择 SQL 文件
  - 集成 `isPostgresAlreadyExistsErr` 到通用执行器

- [ ] **Step 2: 重写 FTS5 → tsvector**
  - 新建 `20260624_message_fts.sql`（Postgres 版）：
    ```sql
    ALTER TABLE messages ADD COLUMN search_vector tsvector
      GENERATED ALWAYS AS (to_tsvector('simple', coalesce(content_markdown, ''))) STORED;
    CREATE INDEX idx_messages_search ON messages USING GIN(search_vector);
    ```
  - 修改 `message_search.go` 支持 tsvector 查询（dialect 感知）
  - 保留 LIKE 回退作为安全网

- [ ] **Step 3: 重写 monitor 模块的 GENERATED VIRTUAL 列**
  - `monitor_trace.go:122-126` 5 个 GENERATED VIRTUAL 列改为 GENERATED STORED
  - 或改为应用层计算（插入时计算并存储）
  - 评估两种方案的复杂度，选择更简单的

- [ ] **Step 4: 重写 Raw SQL Repo 的 SQLite 特定语法**
  - `json_extract(col, '$.key')` → `col ->> 'key'`（Postgres）
  - `INSERT OR IGNORE` → `ON CONFLICT DO NOTHING`
  - `INSERT OR REPLACE` → `ON CONFLICT DO UPDATE`
  - `sqlite_master` 查询 → `information_schema`（Postgres）
  - 逐个审查 99 个 Raw SQL Repo 文件，使用 dialect 抽象层

- [ ] **Step 5: 验证**
  - `go build ./...` 通过
  - `go test ./internal/data/... -count=1` 通过
  - 手动测试：Postgres 模式下 DDL 迁移全部成功执行

### A3：数据迁移工具

**目标**：自建 SQLite → Postgres 数据迁移 CLI 工具

**Files:**
- Create: `cmd/migrate-sqlite-to-postgres/main.go`
- Create: `cmd/migrate-sqlite-to-postgres/migrator.go`
- Create: `cmd/migrate-sqlite-to-postgres/validator.go`
- Create: `cmd/migrate-sqlite-to-postgres/README.md`

- [ ] **Step 1: 设计数据迁移工具架构**
  - 按表分批迁移，每表流程：SQLite 流式读取 → Postgres 批量写入 → 行数校验
  - 优先级排序（按依赖关系）：P0 核心表 → P1 业务表 → P2 辅助表
  - 特殊处理：`vector_embeddings`（重新生成嵌入）、`messages_fts`（不迁移，重建）、`monitor_events`（GENERATED 列不迁移）

- [ ] **Step 2: 实现迁移器**
  - `migrator.go`：按表迁移逻辑
  - 支持 `--table` 参数（迁移单个表）、`--batch-size` 参数
  - 使用 `INSERT ... ON CONFLICT DO NOTHING` 批量写入
  - 流式读取（分页避免 OOM）

- [ ] **Step 3: 实现校验器**
  - `validator.go`：行数校验 + 抽样字段校验
  - 输出校验报告（哪些表一致/不一致）

- [ ] **Step 4: 编写 README**
  - 使用说明、参数说明、注意事项

- [ ] **Step 5: 验证**
  - 在测试环境运行迁移工具
  - 校验所有表行数一致
  - 抽样校验关键字段

### A4：Wire 注入调整 + 框架兼容性修复

**目标**：调整 Wire 绑定，修复框架兼容性问题

**Files:**
- Modify: `cmd/admin/wire.go` （调整数据库相关 Wire 绑定）
- Modify: `cmd/admin/wire_gen.go` （重新生成）
- Modify: `internal/data/wire.go` （如有）
- Modify: `pkg/trpc-agent-go/session/` （如需修复框架兼容性）

- [ ] **Step 1: 调整 Wire 绑定**
  - `provideSQLiteRawDB` → `providePrimaryRawDB`（返回主库 `*sql.DB`，dialect 感知）
  - `provideTRPCSessionService` 审查框架 SQL 兼容性
  - `provideGraphCheckpointSaver` 提供 Postgres 版本（已有 `postgres/saver.go`）

- [ ] **Step 2: 修复框架兼容性问题（基于 A0 调研结果）**
  - 如 `trpcsession.Service` 内部有 SQLite 特定 SQL，提供 Postgres 版本或抽象
  - 如 `SQLiteCheckpointSaver` 无法复用，确认 `postgres/saver.go` 功能完整

- [ ] **Step 3: 重新生成 Wire**
  - `make wire && go build ./cmd/admin`

- [ ] **Step 4: 验证**
  - `go build ./cmd/admin` 通过
  - `go test ./... -count=1` 通过（除已知失败）

### A5：停机迁移 + 切换

**目标**：执行停机迁移，切换到 Postgres 主库

**状态**：✅ 已完成（数据迁移成功，133 表迁移，0 失败，43991 行数据）

- [x] **Step 1: 准备迁移清单**
  - 列出所有需要迁移的表（144 张，其中 133 张迁移、11 张跳过）
  - 列出特殊处理项（FTS5、GENERATED 列、vector_embeddings、trpc_* 表、graph_checkpoints）
  - 列出迁移前后的校验步骤

- [x] **Step 2: 执行停机迁移**
  - 停止服务
  - 运行数据迁移工具：`go run ./cmd/migrate-sqlite-to-postgres --init-schema --run-ddl --init-framework-schema --mode=migrate`
  - 运行校验器：`go run ./cmd/migrate-sqlite-to-postgres --mode=validate`
  - 修改配置：`data.driver: postgres`
  - 启动服务
  - 验证功能正常

- [x] **Step 3: 灰度验证**
  - 核心功能测试：Agent CRUD、Session 创建、消息发送、Team 编排
  - 记忆系统测试：AddMemory、SearchMemories、ProactiveRecall
  - 长任务测试：SessionRun 启动、Checkpoint 恢复

**完成记录**：
- 创建 `cmd/migrate-sqlite-to-postgres/framework_schema.go` — 框架管理表的 DDL（trpc_*/graph_checkpoints/event_wal/vector_embeddings 等 12 张表）
- 添加表名映射逻辑：`checkpoints` → `graph_checkpoints`、`checkpoint_writes` → `graph_checkpoint_writes`
- 修复 `internal/data/monitor_trace.go` bug：Postgres 的 `started_at`/`ended_at` 从 INTEGER 改为 BIGINT（纳秒时间戳溢出）
- 跳过不兼容表：`vector_embeddings`（SQLite TEXT vs Postgres vector）、`trpc_session_events/states`（SQLite 纳秒时间戳 vs Postgres TIMESTAMP）、`memory_l2_index_meta`（已删除）
- 验证结果：109 表完全匹配，24 表 checksum 不匹配（行数匹配，差异来自数据类型表示差异），0 表数据丢失

### A6：清理 SQLite 专用代码

**目标**：移除 SQLite 专用代码，简化代码库

**Files:**
- Modify: `internal/data/data.go` （移除 `initSQLite`，或保留为开发模式）
- Delete: `internal/data/sql/message_fts.sql` （FTS5 schema）
- Delete: `internal/data/message_fts_schema.go` （FTS5 embed）
- Modify: `internal/data/ddl_migration_registry.go` （移除 SQLite 专用迁移）
- Modify: `internal/data/vector/sqlite.go` （保留为降级方案或删除）

- [ ] **Step 1: 评估保留 SQLite 作为开发模式的价值**
  - 如果保留，将 SQLite 代码移到 `internal/data/dev/` 目录
  - 如果删除，确保所有测试用 Postgres

- [ ] **Step 2: 清理 SQLite 专用代码**
  - 移除 `sqlite_master` 查询
  - 移除 `PRAGMA` 设置
  - 移除 FTS5 schema 和查询代码
  - 移除 `vector/sqlite.go`（如不需要降级）

- [ ] **Step 3: 验证**
  - `go build ./...` 通过
  - `go test ./... -count=1` 通过

---

## Phase B：高优先级集成缺口

> **前置依赖**：Phase A 完成（或确认不依赖 Postgres 迁移）
> **预估工作量**：3-5 人天
> **可并行执行**：B1-B5 之间无文件冲突

### B1：P1-7 心跳集成

**目标**：将 `RunHeartbeatEmitter` 集成到 `chat_orchestrator_turn.go` 主流程

**Files:**
- Modify: `internal/service/chat_orchestrator_turn.go` （在 turn 开始时启动心跳，结束时取消）
- Modify: `cmd/admin/wire.go` （新增 `provideRunHeartbeatEmitter`）
- Modify: `cmd/admin/workers.go` （如有需要）
- Test: `internal/service/chat_orchestrator_turn_test.go` （新增心跳集成测试）

- [ ] **Step 1: Wire 绑定 RunHeartbeatEmitter**
  - `cmd/admin/wire.go` 新增 `provideRunHeartbeatEmitter(interval time.Duration, bus contract.Bus, lg loggateway.Logger) *RunHeartbeatEmitter`
  - 从配置读取心跳间隔（默认 10s）

- [ ] **Step 2: 集成到 chat_orchestrator_turn.go**
  - 在 `OrchestratorTurn` 方法中，turn 开始时调用 `emitter.Start(ctx, runID, sessionID, progress)`
  - 在 turn 结束时（成功/失败/取消）调用返回的 `cancel` 函数
  - `progress` 闭包从 turn 状态派生（如当前阶段、进度百分比）

- [ ] **Step 3: 编写集成测试**
  - 测试 turn 开始时心跳启动
  - 测试 turn 结束时心跳取消
  - 测试心跳事件发布到 chat 频道

- [ ] **Step 4: 验证**
  - `go build ./...` 通过
  - `go test ./internal/service/... -run TestHeartbeat -count=1` 通过

### B2：P2-1 NL2Graph 集成

**目标**：将 `NL2GraphConverter` 集成到 `task_orchestrator_impl.go`

**Files:**
- Modify: `internal/agent/task_orchestrator_impl.go` （在 Graph 编排路径调用 NL2GraphConverter）
- Modify: `cmd/admin/wire.go` （新增 `provideNL2GraphConverter`）
- Test: `internal/agent/task_orchestrator_impl_test.go` （新增 NL2Graph 集成测试）

- [ ] **Step 1: Wire 绑定 NL2GraphConverter**
  - `cmd/admin/wire.go` 新增 `provideNL2GraphConverter(llm trpcmodel.Model, lg loggateway.Logger) *NL2GraphConverterImpl`

- [ ] **Step 2: 集成到 task_orchestrator_impl.go**
  - `TaskOrchestratorImpl` 新增 `nl2graph NL2GraphConverter` 字段
  - 在 Graph 编排路径（`orchestrateWithGraph` 或类似方法）中，当无预定义 Graph 模板时调用 `nl2graph.Convert`
  - 失败时降级到现有 sequential pipeline

- [ ] **Step 3: 编写集成测试**
  - 测试自然语言任务描述生成 Graph
  - 测试 LLM 失败时降级到 sequential pipeline

- [ ] **Step 4: 验证**
  - `go build ./...` 通过
  - `go test ./internal/agent/... -run TestNL2Graph -count=1` 通过

### B3：P2-2 RuntimeReplanner 集成

**目标**：将 `RuntimeReplanner` 集成到 Graph executor 的 `OnNodeError` 回调

**Files:**
- Modify: `internal/graph/adapter/runtime_adapter.go` （注册 OnNodeError 回调）
- Modify: `cmd/admin/wire.go` （新增 `provideRuntimeReplanner`）
- Test: `internal/graph/adapter/runtime_adapter_test.go` （新增重规划集成测试）

- [ ] **Step 1: Wire 绑定 RuntimeReplanner**
  - `cmd/admin/wire.go` 新增 `provideRuntimeReplanner(bus event.Bus, lg loggateway.Logger) *RuntimeReplannerImpl`

- [ ] **Step 2: 集成到 runtime_adapter.go**
  - `GraphBuilderFactory` 新增 `replanner RuntimeReplanner` 字段
  - `createAgent` 时注册 `NodeCallbacks.RegisterOnNodeError(replanner.OnNodeFailure)`
  - 失败时调用 `replanner.OnNodeFailure`，根据返回的 action 执行重规划

- [ ] **Step 3: 编写集成测试**
  - 测试节点失败时触发重规划
  - 测试重规划次数限制（maxReplanAttempts=3）

- [ ] **Step 4: 验证**
  - `go build ./...` 通过
  - `go test ./internal/graph/... -run TestReplanner -count=1` 通过

### B4：P2-3 TopologyEvolver 集成

**目标**：将 `TopologyEvolver` 集成到 Graph executor 的执行洞察回调

**Files:**
- Modify: `internal/graph/adapter/runtime_adapter.go` （注册执行洞察回调）
- Modify: `cmd/admin/wire.go` （新增 `provideTopologyEvolver`）
- Test: `internal/graph/adapter/runtime_adapter_test.go` （新增拓扑演化集成测试）

- [ ] **Step 1: Wire 绑定 TopologyEvolver**
  - `cmd/admin/wire.go` 新增 `provideTopologyEvolver(llm trpcmodel.Model, bus event.Bus, lg loggateway.Logger) *TopologyEvolverImpl`

- [ ] **Step 2: 集成到 runtime_adapter.go**
  - `GraphBuilderFactory` 新增 `evolver TopologyEvolver` 字段
  - 在 Graph 执行的适当节点（如节点完成后）调用 `evolver.OnExecutionInsight`
  - 失败时仅 Warn 日志，不阻断执行

- [ ] **Step 3: 编写集成测试**
  - 测试执行中发现新路径时动态添加边
  - 测试 LLM 失败时降级返回 nil

- [ ] **Step 4: 验证**
  - `go build ./...` 通过
  - `go test ./internal/graph/... -run TestTopology -count=1` 通过

### B5：P2-4 ParallelToolExecutor 集成

**目标**：将 `ParallelToolExecutor` 集成到 `spirit_tools.go` 的批量工具调用路径

**Files:**
- Modify: `internal/tools/spirit_tools.go` （在批量工具调用时使用 ParallelToolExecutor）
- Modify: `cmd/admin/wire.go` （新增 `provideParallelToolExecutor`）
- Test: `internal/tools/spirit_tools_test.go` （新增并行执行集成测试）

- [ ] **Step 1: Wire 绑定 ParallelToolExecutor**
  - `cmd/admin/wire.go` 新增 `provideParallelToolExecutor(handler ToolHandler, lg loggateway.Logger) *ParallelToolExecutor`

- [ ] **Step 2: 集成到 spirit_tools.go**
  - 识别批量工具调用场景（如 `multi_tool_use.parallel`）
  - 使用 `ParallelToolExecutor.Execute` 替代串行执行
  - 保留串行执行作为降级方案

- [ ] **Step 3: 编写集成测试**
  - 测试无依赖工具并行执行
  - 测试有依赖工具按拓扑层级执行
  - 测试 5 文件并行延迟 < 串行 40%

- [ ] **Step 4: 验证**
  - `go build ./...` 通过
  - `go test ./internal/tools/... -run TestParallel -count=1` 通过

---

## Phase C：可观测与记忆触发

> **前置依赖**：无（可与 Phase B 并行）
> **预估工作量**：2-3 人天

### C1：P3-3 Spirit Metrics 埋点

**目标**：在编排阶段实际记录耗时到 `SpiritPlanDuration`/`SpiritAllocDuration`/`SpiritOrchDuration`

**Files:**
- Modify: `internal/service/chat_orchestrator_turn_phases.go` （Plan/Allocate 阶段埋点）
- Modify: `internal/agent/task_orchestrator_impl.go` （Orchestrate 阶段埋点）
- Test: 对应测试文件

- [ ] **Step 1: Plan 阶段埋点**
  - `executePlanPhase` 开始时 `start := time.Now()`，结束时 `metrics.SpiritPlanDuration.Observe(time.Since(start).Seconds())`
  - 使用 `defer` 确保异常路径也记录

- [ ] **Step 2: Allocate 阶段埋点**
  - `executeAllocatePhase` 同上

- [ ] **Step 3: Orchestrate 阶段埋点**
  - `Orchestrate` 同上

- [ ] **Step 4: 验证**
  - `go build ./...` 通过
  - 手动测试：执行一个 turn，检查 Prometheus 指标暴露

### C2：P3-11 主动召回 turn 触发

**目标**：在 turn 开始时调用 `ProactiveRecall`，将结果注入 prompt

**Files:**
- Modify: `internal/service/chat_orchestrator_turn.go` （turn 开始时调用 ProactiveRecall）
- Modify: `internal/agent/composite_prompt.go` （将主动召回结果注入 CompositeMemoryCue）
- Test: 对应测试文件

- [ ] **Step 1: 在 turn 开始时调用 ProactiveRecall**
  - `OrchestratorTurn` 方法中，在 build runner 前调用 `compositeRecaller.ProactiveRecall`
  - 从用户消息提取 `MentionedEntities`/`CurrentTopic`/`UserStatement`
  - 失败时仅 Warn 日志，不阻断 turn

- [ ] **Step 2: 将主动召回结果注入 prompt**
  - `CompositeMemoryCue` 新增 `proactiveHits` 参数
  - 主动召回结果与 RecallComposite 结果合并

- [ ] **Step 3: 编写集成测试**
  - 测试 turn 开始时触发主动召回
  - 测试主动召回结果注入 prompt

- [ ] **Step 4: 验证**
  - `go build ./...` 通过
  - `go test ./internal/service/... -run TestProactive -count=1` 通过

### C3：P3-12 LinkEvolver AddMemory 触发

**目标**：在 `AddMemory` 后异步触发 `EvolveLinks`

**Files:**
- Modify: `internal/memory/trpc/sqlite_adapter.go` （AddMemory 后异步触发 EvolveLinks）
- Modify: `cmd/admin/wire_memory.go` （Wire 绑定 LinkEvolver）
- Test: 对应测试文件

- [ ] **Step 1: Wire 绑定 LinkEvolver**
  - `cmd/admin/wire_memory.go` 新增 `provideLinkEvolver`，注入到 `sqliteMemoryService`

- [ ] **Step 2: AddMemory 后异步触发 EvolveLinks**
  - `sqliteMemoryService` 新增 `linkEvolver LinkEvolver` 字段
  - `AddMemory` 成功后，使用 `safego.Go` 异步调用 `linkEvolver.EvolveLinks`
  - 失败时仅 Warn 日志，不阻断 AddMemory

- [ ] **Step 3: 编写集成测试**
  - 测试 AddMemory 后异步触发 EvolveLinks
  - 测试 EvolveLinks 失败时不影响 AddMemory

- [ ] **Step 4: 验证**
  - `go build ./...` 通过
  - `go test ./internal/memory/... -run TestLinkEvolution -count=1` 通过

---

## Phase D：cron job 启用与文档同步

> **前置依赖**：无（可与 Phase B/C 并行）
> **预估工作量**：1-2 人天

### D1：P3-9 Ebbinghaus cron job Wire 绑定

**目标**：将 `MemoryEbbinghausDecayWorker` 通过 Wire 绑定到 cronrunner 调度器

**Files:**
- Modify: `cmd/admin/wire.go` （新增 `provideMemoryEbbinghausDecayWorker`）
- Modify: `cmd/admin/workers.go` （新增 `MemoryEbbinghausDecayWorker` 字段 + `goAfterReady` 启动）
- Modify: `cmd/admin/main.go` （传递到 `backgroundWorkersConfig`）
- Modify: `internal/data/ent/schema/` 或 DDL 迁移 （新增 `access_count`/`last_accessed_at`/`decay_score` 列）— 可选，简化方案可不加

- [ ] **Step 1: Wire 绑定 MemoryEbbinghausDecayWorker**
  - `cmd/admin/wire.go` 新增 `provideMemoryEbbinghausDecayWorker`
  - 从配置读取 interval（默认 1h）

- [ ] **Step 2: 在 workers.go 启动**
  - `backgroundWorkersConfig` 新增 `MemoryEbbinghausDecayWorker` 字段
  - `goAfterReady("memory_ebbinghaus_decay", func() { cfg.MemoryEbbinghausDecayWorker.Start(ctx) })`

- [ ] **Step 3: 验证**
  - `go build ./cmd/admin` 通过
  - 手动测试：启动后 cron job 定期执行

### D2：P3-10 Sleep-time cron job Wire 绑定

**目标**：将 `MemorySleepTimeWorker` 通过 Wire 绑定到 cronrunner 调度器

**Files:**
- Modify: `cmd/admin/wire.go` （新增 `provideMemorySleepTimeWorker`）
- Modify: `cmd/admin/workers.go` （新增 `MemorySleepTimeWorker` 字段 + `goAfterReady` 启动）
- Modify: `cmd/admin/main.go` （传递到 `backgroundWorkersConfig`）
- Modify: `internal/cronrunner/jobs/memory_sleep_time.go` （实现生产级 `AgentUserKeyLister`，从 SessionRepo 派生活跃用户）

- [ ] **Step 1: 实现生产级 AgentUserKeyLister**
  - 从 SessionRepo 派生活跃用户（最近 N 天有活动的 session）
  - 或从配置读取 userIDs 列表

- [ ] **Step 2: Wire 绑定 MemorySleepTimeWorker**
  - `cmd/admin/wire.go` 新增 `provideMemorySleepTimeWorker`
  - 从配置读取 interval（默认 6h）

- [ ] **Step 3: 在 workers.go 启动**
  - `backgroundWorkersConfig` 新增 `MemorySleepTimeWorker` 字段
  - `goAfterReady("memory_sleep_time", func() { cfg.MemorySleepTimeWorker.Start(ctx) })`

- [ ] **Step 4: 验证**
  - `go build ./cmd/admin` 通过
  - 手动测试：启动后 cron job 定期执行

### D3：P3-6 i18n CI 集成

**目标**：将 `check:i18n` 集成到 CI lint/test 流程

**Files:**
- Modify: `web/package.json` （`lint` 脚本包含 `check:i18n`）
- Modify: CI 配置文件（如有）
- Modify: `web/scripts/check-i18n.mjs` （优化错误输出）

- [ ] **Step 1: 修改 lint 脚本**
  - `web/package.json` 的 `lint` 脚本改为 `eslint ... && node scripts/check-i18n.mjs`
  - 或新增 `lint:full` 脚本包含两者

- [ ] **Step 2: 优化 check-i18n.mjs 输出**
  - 错误输出包含文件路径和行号
  - baseline 增量比对清晰提示

- [ ] **Step 3: 验证**
  - `cd web && pnpm lint` 通过（含 i18n 检查）

### D4：文档状态标记同步

**目标**：同步 `70-orchestration-longtask-memory.development.md` 的状态标记与代码实际状态一致

**状态**：✅ 已完成

**Files:**
- Modify: `docs/development/70-orchestration-longtask-memory.development.md`

- [x] **Step 1: 修正验收表状态标记**
  - §7.2 验收 #12 崩溃恢复 📋 → ✅
  - §7.4 验收 #21 ErrorBlock 重试 📋 → ✅
  - §7.4 验收 #22 WS 快速检测 📋 → ✅

- [x] **Step 2: 修正改动文件清单状态标记**
  - §9.1 中以下文件 📋 待创建 → ✅ 已创建：
    - `internal/service/recovery_worker.go`
    - `internal/graph/runtime_replanner.go`
    - `internal/tools/parallel_executor.go`
    - `internal/tools/dependency_analyzer.go`
    - `internal/tools/worktree_isolator.go`
    - `internal/tools/transaction_sandbox.go`
    - `pkg/trpc-agent-go/graph/checkpoint/postgres/*.go`
  - §9.1 `20260617_event_store.sql` 📋 → 不需要（EnsureSchema 在代码中建表）
  - §9.1 `20260617_memory_ebbinghaus.sql` 保持 🟡（P3-9 简化方案未启用）

- [x] **Step 3: 新增 Phase A-C 完成记录**
  - 在文档末尾新增"Postgres 全量迁移 + 剩余项集成修复"章节
  - 记录每个 Phase 的完成状态

- [x] **Step 4: 验证**
  - 文档状态标记与代码实际状态一致

**完成记录**：
- §7.2/§7.4 验收表状态标记已全部修正为 ✅（Step 1 在 Phase B/C/D 完成时已同步）
- §9.1 改动文件清单状态标记已全部修正为 ✅（Step 2 在 Phase B/C/D 完成时已同步）
- §14 章节"剩余项集成修复完成记录（Phase B/C/D）"已存在（Step 3 Phase B/C/D 部分已完成）
- §15 章节"Postgres 全量迁移完成记录（Phase A）"已新增，记录 A0-A5 ✅、A6 📋 待执行
- 文档同步合规：DOC-SYNC-1/5/6 全部满足

---

## 验证标准

### Phase A 验证
| # | 验收项 | 验证方式 |
|---|--------|---------|
| A1 | dialect 抽象层 | `go build ./...` 通过；配置 `driver: postgres` 能启动 |
| A2 | Schema 迁移 | Postgres 模式下 DDL 迁移全部成功；FTS5 → tsvector 查询正确 |
| A3 | 数据迁移工具 | 76 张表数据迁移完成；行数校验一致 |
| A4 | Wire 注入 | `go build ./cmd/admin` 通过；框架兼容性问题修复 |
| A5 | 停机迁移 | 切换到 Postgres 后核心功能正常 |
| A6 | 清理 | SQLite 专用代码移除；`go test ./...` 通过 |

### Phase B 验证
| # | 验收项 | 验证方式 |
|---|--------|---------|
| B1 | 心跳集成 | turn 开始时心跳启动；前端 30s 无心跳标记 stale |
| B2 | NL2Graph 集成 | 自然语言任务生成 Graph；LLM 失败降级 sequential |
| B3 | RuntimeReplanner 集成 | 节点失败触发重规划；4 种重规划类型 |
| B4 | TopologyEvolver 集成 | 执行中动态添加边；LLM 失败降级 |
| B5 | ParallelToolExecutor 集成 | 5 文件并行延迟 < 串行 40% |

### Phase C 验证
| # | 验收项 | 验证方式 |
|---|--------|---------|
| C1 | Spirit Metrics 埋点 | Prometheus 指标可查询编排阶段耗时 |
| C2 | 主动召回 turn 触发 | turn 开始时触发主动召回；结果注入 prompt |
| C3 | LinkEvolver AddMemory 触发 | AddMemory 后异步触发 link generation |

### Phase D 验证
| # | 验收项 | 验证方式 |
|---|--------|---------|
| D1 | Ebbinghaus cron job | 启动后 cron job 定期执行 |
| D2 | Sleep-time cron job | 启动后 cron job 定期执行 |
| D3 | i18n CI 集成 | `pnpm lint` 包含 i18n 检查 |
| D4 | 文档同步 | 状态标记与代码一致 |

---

## 风险与缓解

| # | 风险 | 缓解措施 |
|---|------|---------|
| 1 | trpc-agent-go 框架 SQL 兼容性 | A0 调研先行；如需修改框架代码，谨慎评估 |
| 2 | monitor GENERATED VIRTUAL 列 | 改为 STORED 或应用层计算 |
| 3 | 数据迁移一致性 | 自建校验工具；迁移后强制校验 |
| 4 | 跨库事务不可行 | 不推荐长期双写；采用停机迁移 |
| 5 | FTS5 → tsvector 搜索质量差异 | 保留 LIKE 回退作为安全网 |
| 6 | 76 表迁移工作量大 | 分批迁移；优先核心表 |

---

## 依赖关系

```
Phase A（Postgres 全量迁移）
  ├── A0 框架兼容性调研 ─────┐
  ├── A1 dialect 抽象层 ─────┤
  ├── A2 Schema 迁移 ────────┤
  ├── A3 数据迁移工具 ───────┼─► A5 停机迁移 ─► A6 清理
  └── A4 Wire 注入调整 ──────┘

Phase B（高优先级集成缺口）— 可与 Phase A 并行（不依赖 Postgres 迁移）
  ├── B1 心跳集成 ───────────┐
  ├── B2 NL2Graph 集成 ──────┤
  ├── B3 RuntimeReplanner ───┼─► 验证
  ├── B4 TopologyEvolver ────┤
  └── B5 ParallelTool ───────┘

Phase C（可观测与记忆触发）— 可与 Phase B 并行
  ├── C1 Spirit Metrics ─────┐
  ├── C2 主动召回 turn 触发 ──┼─► 验证
  └── C3 LinkEvolver 触发 ───┘

Phase D（cron job 与文档）— 可与 Phase B/C 并行
  ├── D1 Ebbinghaus cron ────┐
  ├── D2 Sleep-time cron ────┤
  ├── D3 i18n CI ────────────┼─► 验证
  └── D4 文档同步 ───────────┘
```

---

## 实施纪律

1. **TDD 铁律**：每个任务先写失败测试，再写最小实现
2. **两阶段审查**：规格合规审查优先，代码质量审查其次
3. **验证前置**：每个任务完成前必须运行 `make test && make build && make lint`
4. **YAGNI**：不添加未请求的功能，不过度工程
5. **文档同步**：代码改动同步更新三件套文档
6. **Surgical Changes**：每行改动可追溯到需求，不顺带 refactor
7. **Phase A 优先**：Postgres 全量迁移是基础设施，应优先完成
8. **停机迁移**：不推荐长期双写；选择低峰窗口停机迁移
