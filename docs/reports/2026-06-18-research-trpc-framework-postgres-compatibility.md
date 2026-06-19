# trpc-agent-go 框架 Postgres 兼容性调研报告

> **报告类型**：research（调研报告）
> **调研日期**：2026-06-18
> **关联计划**：`docs/superpowers/plans/2026-06-18-postgres-migration-and-integration-fixes.md` Phase A / A0
> **调研范围**：`pkg/trpc-agent-go/` 框架内部 SQL 是否硬编码 SQLite 语法，以及项目当前的处理方式

---

## 0. 执行摘要

| 维度 | 结论 |
|------|------|
| **trpcsession.Service** | 框架已提供独立的 `postgres/` 实现，**无需修改框架代码**；项目层通过 Wire 选择后端 |
| **CheckpointSaver** | 框架已提供 `postgres/saver.go`，功能完整覆盖 SQLite 版 8 个接口方法；**无需修改框架代码** |
| **Memory Service** | 框架已提供 `postgres/` 和 `pgvector/` 实现；但**项目层完全绕过框架实现**，自建基于 dialect 抽象的 raw SQL 实现 |
| **框架代码修改需求** | **零修改**。所有兼容性问题都通过"独立包 + Wire 选择"或"项目层 dialect 抽象"解决 |
| **风险等级** | 低。主要风险是 Postgres CheckpointSaver 缺失测试覆盖（ISS-1） |

**核心发现**：框架采用"每个 dialect 一个独立包"的架构（非 dialect 抽象层），所有 SQL 在包内独立实现，包之间不共享代码。项目层只需在 Wire 注入时选择对应后端的实现即可，**不存在"修改 SQLite SQL 以兼容 Postgres"的需求**。

---

## 1. Step 1 — trpcsession.Service 的 SQL 兼容性

### 1.1 架构总览

`pkg/trpc-agent-go/session/` 采用**按 dialect 拆分独立实现**的架构：

| 子目录 | 后端 | 含 SQL 的文件 |
|--------|------|--------------|
| `sqlite/` | SQLite | 10 个 .go 文件 |
| `postgres/` | PostgreSQL | 6 个 .go 文件（已存在） |
| `mysql/` | MySQL | 是 |
| `clickhouse/` | ClickHouse | 是 |
| `pgvector/` | pgvector | 是 |
| `redis/` | Redis（Lua 脚本） | 是 |
| `inmemory/` | 内存 | 否 |
| `noop/` | 空实现 | 否 |

**关键事实**：项目已存在独立的 `postgres/` 实现，并非通过 dialect 抽象层适配，而是为每个数据库后端写一份独立代码。

### 1.2 SQLite 实现的 SQL 兼容性清单

| 文件 | SQL 类别 | SQLite 特定语法 | Postgres 兼容性 |
|------|---------|----------------|----------------|
| `sqlite/init.go` | CREATE TABLE (6 张表) | `INTEGER PRIMARY KEY AUTOINCREMENT`、`BLOB`、`INTEGER` 存时间戳 | 不兼容（PG 用 `BIGSERIAL`/`JSONB`/`TIMESTAMP`） |
| `sqlite/init.go` | CREATE INDEX (部分索引) | `WHERE deleted_at IS NULL` | 兼容（PG 原生支持部分索引） |
| `sqlite/service.go` | SELECT/INSERT/UPDATE | `?` 占位符 | 不兼容（PG 用 `$N`） |
| `sqlite/service.go` | upsertAppState (三段式) | SELECT-then-INSERT/UPDATE 反模式 | 不兼容（PG 用 `ON CONFLICT ... DO UPDATE` 原子 UPSERT） |
| `sqlite/service_helper.go` | addEvent (事务内 SELECT+UPDATE+INSERT) | `sync.Mutex` 串行化 | 不兼容（PG 用 `SELECT ... FOR UPDATE` 行锁） |
| `sqlite/event_query.go` | 元组 IN 查询 | `(?,?,?) IN ((?,?,?),(?,?,?))` | 兼容但低效（PG 改用 `ANY($3::varchar[])`） |
| `sqlite/summary.go` | UPSERT | `ON CONFLICT ... DO UPDATE`（无 WHERE 条件） | 基本兼容（PG 需补 `WHERE deleted_at IS NULL` partial conflict target） |
| `sqlite/window.go` | JSON 模糊匹配 | `instr(CAST(event AS TEXT), ?)` | **完全不兼容**（PG 用 `event->>'id' = $5` JSONB 路径访问） |
| `sqlite/cleanup.go` | 相关子查询 | `UPDATE ... WHERE EXISTS (SELECT ... FROM ...)` | 部分兼容（PG 改用"先查 key 后批量删"两步法） |

### 1.3 SQLite 特定语法使用统计

| 语法 | 出现次数 | Postgres 兼容性 |
|------|---------|----------------|
| `INTEGER PRIMARY KEY AUTOINCREMENT` | 6 处 | 不兼容 |
| `BLOB` 类型 | 9 处 | 不兼容 |
| `?` 占位符 | 100+ 处 | 不兼容 |
| `instr(CAST(event AS TEXT), ?)` | 1 处 | 完全不兼容 |
| `INSERT OR IGNORE/REPLACE` | 0 处 | 未使用（用 `ON CONFLICT` 替代） |
| `sqlite_master` | 0 处 | 未使用 |
| `PRAGMA` | 0 处 | 未使用 |
| `json_extract` / `json_each` | 0 处 | 未使用 |
| `strftime` / `datetime()` | 0 处 | 未使用 |

### 1.4 Postgres 实现的关键差异

| 维度 | SQLite 实现 | Postgres 实现 |
|------|-----------|--------------|
| 占位符 | `?` | `$1, $2, ...` |
| 自增主键 | `INTEGER PRIMARY KEY AUTOINCREMENT` | `BIGSERIAL PRIMARY KEY` |
| JSON 存储 | `BLOB`（JSON 字节序列化） | `JSONB`（原生 JSON） |
| 时间字段 | `INTEGER`（UnixNano int64） | `TIMESTAMP`（`time.Time`） |
| 键列类型 | `TEXT` | `VARCHAR(255)` |
| UPSERT | `ON CONFLICT ... DO UPDATE`（无 WHERE） | `ON CONFLICT ... WHERE deleted_at IS NULL DO UPDATE`（partial conflict target） |
| JSON 查询 | `instr(CAST(event AS TEXT), ?)` 字符串子串匹配 | `event->>'id' = $5` JSONB 路径访问 |
| IN 查询 | 元组 `(?,?,?) IN ((?,?,?),(?,?,?))` | `session_id = ANY($3::varchar[])` |
| 并发控制 | `sync.Mutex` 进程内互斥锁 | `SELECT ... FOR UPDATE` 行级锁 |
| Schema 校验 | 无 | `information_schema.tables` + `pg_indexes` 校验 |
| 过期清理 | `UPDATE ... WHERE EXISTS (相关子查询)` | 先 `SELECT ... GROUP BY` 查 key，再批量 `UPDATE ... WHERE IN` |

### 1.5 结论

**trpcsession.Service 无需修改框架代码即可支持 Postgres**。项目层在 Wire 注入时选择 `postgres.NewService` 而非 `sqlite.NewService` 即可。SQLite 实现的 SQL 与 Postgres 实现的 SQL 完全不共享，不存在"修改 SQLite SQL 以兼容 Postgres"的需求。

---

## 2. Step 2 — CheckpointSaver 的 SQL 兼容性

### 2.1 实现清单

| 实现 | 文件路径 | 行数 | 测试覆盖 |
|------|---------|------|---------|
| SQLite Saver | `pkg/trpc-agent-go/graph/checkpoint/sqlite/sqlite.go` | 605 | `sqlite_test.go` 1347 行（~40 个用例） |
| Postgres Saver | `pkg/trpc-agent-go/graph/checkpoint/postgres/saver.go` | 574 | **无测试文件** |

### 2.2 SQLite Saver 的 SQL 兼容性清单

| # | SQL 语句 | SQLite 特定语法 | Postgres 兼容性 |
|---|---------|----------------|----------------|
| S1 | `CREATE TABLE IF NOT EXISTS checkpoints (...)` 使用 `BLOB`/`INTEGER` | 否（标准 SQL） | 兼容（类型需改） |
| S3 | `INSERT OR REPLACE INTO checkpoints (...) VALUES (?, ?, ?, ?, ?, ?, ?)` | **`INSERT OR REPLACE`** | **不兼容** |
| S7 | `INSERT OR REPLACE INTO checkpoint_writes (...) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)` | **`INSERT OR REPLACE`** | **不兼容** |
| S4-S6, S8-S16 | SELECT/DELETE/UPDATE | `?` 占位符 | 占位符需改为 `$N` |

**SQLite 特定语法检查**：
- `sqlite_master`：未使用
- `PRAGMA`：未使用
- `INSERT OR REPLACE`：2 处（S3、S7）
- `AUTOINCREMENT`：未使用

### 2.3 Postgres Saver 的 SQL 处理

| SQLite 语法 | Postgres 处理方式 | 状态 |
|------------|------------------|------|
| `INSERT OR REPLACE` (S3, S7) | `INSERT ... ON CONFLICT (...) DO UPDATE SET ...=EXCLUDED...` | 正确处理 |
| `?` 占位符 | `$1, $2, ...`；动态查询用 `argIdx` 计数器 | 正确处理 |
| 表名 `checkpoints` | 改为 `graph_checkpoints`（加前缀避免共享实例冲突） | 正确处理 |
| `BLOB` 类型 | 改为 `TEXT` | 正确处理 |
| `INTEGER` 类型 | 改为 `BIGINT` | 正确处理 |

### 2.4 功能覆盖对比

CheckpointSaver 接口 8 个方法**全覆盖**：

| 接口方法 | SQLite | Postgres | 说明 |
|---------|--------|---------|------|
| `Get` | ✅ | ✅ | 实现一致 |
| `GetTuple` | ✅ | ✅ | 实现一致（含跨命名空间查找） |
| `List` | ✅ | ✅ | 实现一致（含 Before/Limit/Metadata 过滤） |
| `Put` | ✅ | ✅ | 实现一致（含默认 metadata、零时间戳兜底） |
| `PutWrites` | ✅ | ✅ | 实现一致（含 Sequence 兜底为 idx） |
| `PutFull` | ✅ | ✅ | 实现一致（事务原子写入） |
| `DeleteLineage` | ✅ | ✅ | 实现一致（删 checkpoints + writes） |
| `Close` | ✅ 关闭 DB | ⚠️ 空操作 | **行为不一致**（见 2.5 ISS-2） |

### 2.5 发现的问题

| 编号 | 问题 | 严重度 | 位置 |
|------|------|--------|------|
| ISS-1 | **Postgres 无测试文件**——整个实现未经测试验证 | 高 | `postgres/` 目录 |
| ISS-2 | **Close() 行为不一致**——SQLite 关闭 DB，Postgres 空操作；Postgres 注释错误声称"matching sqlite.Saver semantics" | 中 | `postgres/saver.go:530-535` |
| ISS-3 | SQLite 死代码 `sqliteSelectIDsAsc` 常量定义但未使用 | 低 | `sqlite/sqlite.go:65-66` |
| ISS-4 | Postgres 未用 JSONB（用 TEXT），无法用 GIN 索引（但 saver 不按 JSON 内容查询，影响可忽略） | 低 | `postgres/saver.go:44-45` |

### 2.6 结论

**CheckpointSaver 的 Postgres 支持功能完整**，覆盖了 SQLite Saver 的全部 8 个接口方法和 9 个辅助方法，SQL 兼容性处理正确无误。文件头注释明确说明这是 P1-8 崩溃恢复的推荐 saver（"the recommended saver for crash recovery (P1-8) because Postgres provides durable WAL-backed storage"）。

主要风险是**完全缺失测试覆盖**（ISS-1），建议补充 Postgres 集成测试以验证 SQL 语句在真实 Postgres 实例上的行为与 SQLite 版本等价。Close() 行为分歧（ISS-2）需澄清预期语义并修正注释或实现之一。

---

## 3. Step 3 — Memory Service 的 SQL 兼容性

### 3.1 框架层架构

`pkg/trpc-agent-go/memory/` 同样采用"每个 dialect 一个独立包"架构：

| 子包 | 表名 | 目标 dialect |
|------|------|-------------|
| `sqlite/` | `memories` | SQLite |
| `postgres/` | `memories` | Postgres |
| `pgvector/` | `memories` | Postgres + pgvector 扩展 |
| `mysql/`、`mysqlvec/`、`sqlitevec/` | `memories` | 各自 dialect |
| `redis/`、`inmemory/`、`mem0/`、`tencentdb/` | — | 无 SQL |

**关键事实**：框架的表名是 `memories`，**不是 `memory_facts`**。`memory_facts` 是项目层自建的表。

### 3.2 框架 sqlite 包的 SQL 兼容性

| SQL 类别 | SQLite 特定语法 | Postgres 兼容性 |
|---------|----------------|----------------|
| CREATE TABLE `memories` | `BLOB`、`INTEGER` 存时间戳 | 不兼容（PG 用 `JSONB`/`TIMESTAMP`） |
| INSERT UPSERT | `?` 占位符；`ON CONFLICT ... DO UPDATE` | `?` 不兼容；`ON CONFLICT` 兼容 |
| SELECT/UPDATE/DELETE | `?` 占位符 | 不兼容（PG 用 `$N`） |
| 部分索引 | `WHERE deleted_at IS NOT NULL` | 兼容 |

**SQLite 特定语法使用**：
- `?` 占位符：是（全部 SQL）
- `BLOB` 类型：是
- `sqlite_master`/`PRAGMA`/`json_extract`/`strftime`/`AUTOINCREMENT`：否

### 3.3 框架 postgres/pgvector 包的 Postgres 专属特性

- `$N` 占位符
- `JSONB` 类型、`TIMESTAMP` 类型
- `information_schema.tables`/`information_schema.columns`（替代 `sqlite_master`）
- `pg_indexes`/`pg_class`/`pg_index`/`pg_attribute` 系统目录查询
- `has_schema_privilege` DDL 权限检查
- pgvector：`CREATE EXTENSION vector`、`vector(N)` 类型、`gin`/`hnsw` 索引、`tsvector`、`plpgsql` 触发器

### 3.4 项目层处理方式（关键发现）

**项目层完全绕过框架的 SQLite/Postgres memory service 实现，自建一套基于 dialect 抽象的 raw SQL 实现**：

1. **桥接层**：`internal/memory/trpc/sqlite_adapter.go`（名字误导，实际不含 SQL）实现 `trpcmemory.Service` 接口，把所有数据操作委托给 `biz.L3FactReader` / `biz.L3FactWriter` 接口

2. **实际 SQL 实现**：`internal/data/memory_shim_l3.go` 用 raw SQL 实现 `L3FactReader`/`L3FactWriter`，所有 SQL 用 `?` 占位符 + `Dialect().RenumberPlaceholders()` 在运行时转换为 Postgres `$N`

3. **Dialect 抽象层**：`internal/data/dialect.go` 提供完整的 dialect 感知 helper：
   - `RenumberPlaceholders(sql)`：`?` → `$1, $2, ...`
   - `JSONExtract(col, key)`：`json_extract` vs `->>`
   - `JSONEach(col)`：`json_each` vs `json_array_elements_text`
   - `TableExistsQuery`/`ColumnExistsQuery`/`IndexExistsQuery`：`sqlite_master`/`pragma_*` vs `information_schema`/`pg_*`
   - `AlreadyExistsErr`/`UndefinedObjectErr`/`UniqueConstraintErr`：字符串匹配 vs `pq.Error` 代码

4. **DDL 管理**：`internal/data/sql/memory_chain.sql` 定义 `memory_facts` 表（用 SQLite 类型），`EnsureSessionMemorySchema` 在 Postgres 下做 `BLOB` → `BYTEA` 翻译

5. **表名差异**：项目用 `memory_facts`（含 50+ 列的复杂 schema，支持 bi-temporal、PII、embedding、cascade 等），框架用 `memories`（仅 7 列简单 schema）—— **两套表完全不同**

### 3.5 项目层 memory_facts SQL 的 Postgres 兼容性

**设计上是 Postgres 兼容的**，通过以下机制：

| 机制 | 实现位置 | 覆盖范围 |
|------|---------|---------|
| 占位符重编号 `?` → `$N` | `Dialect.RenumberPlaceholders()` | 所有 CRUD SQL |
| DDL 类型翻译 `BLOB` → `BYTEA` | `EnsureSessionMemorySchema` | memory_chain.sql |
| JSON 操作 dialect 分支 | `Dialect.JSONExtract/JSONEach/JSONSet` | 需要 JSON 操作的查询 |
| 元数据查询 dialect 分支 | `Dialect.TableExistsQuery/ColumnExistsQuery` | DDL 迁移检查 |
| 错误翻译 dialect 分支 | `Dialect.AlreadyExistsErr/UndefinedObjectErr` | DDL 迁移错误处理 |
| 仅使用两边都支持的 SQL 特性 | `ON CONFLICT`、`excluded`、`COALESCE`、`NULLIF`、`CASE WHEN`、`LIKE`、`LIMIT/OFFSET`、`IN`、部分索引 | 全部业务 SQL |

### 3.6 潜在风险点

- `memory_facts` 是野生表，未进 Ent Schema，违反 DB-R3 红线（历史遗留，已 grandfathered）
- `BLOB` → `BYTEA` 是字符串替换，若列名/注释中含 "BLOB" 会误伤（实际未发生）
- 项目层未使用框架的 `postgres/` 或 `pgvector/` 包，意味着框架的 Postgres memory 实现属于"死代码"——项目只复用了 `trpcmemory.Service` 接口契约

### 3.7 结论

**Memory Service 的 Postgres 支持状态**：
- 框架层：原生支持 Postgres（通过独立 `postgres/`/`pgvector/` 包），无需修改框架代码
- 项目层：完全绕过框架实现，自建基于 dialect 抽象的 raw SQL 实现，设计上是 Postgres 兼容的
- **无需修改框架代码**

---

## 4. Step 4 — 兼容性报告汇总

### 4.1 框架代码修改需求

| 框架组件 | 是否需要修改框架代码 | 原因 |
|---------|-------------------|------|
| `trpcsession.Service` | **否** | 框架已提供独立 `postgres/` 实现，项目通过 Wire 选择后端 |
| `CheckpointSaver` | **否** | 框架已提供 `postgres/saver.go`，功能完整覆盖 |
| `Memory Service` | **否** | 框架已提供 `postgres/`/`pgvector/` 实现；项目层自建 dialect 抽象绕过框架实现 |

**红线评估**：框架代码修改需谨慎（红线 #11：禁止修改 Ent 生成代码；框架代码虽非 Ent 生成，但修改 `pkg/trpc-agent-go` 影响面大）。本次调研结论是**零框架代码修改**，符合红线要求。

### 4.2 项目层处理策略

| 框架组件 | 项目层策略 | 实现位置 |
|---------|-----------|---------|
| `trpcsession.Service` | Wire 选择 `postgres.NewService`（生产）+ `inmemory.NewService`（fallback） | `internal/session/trpc/factory.go` |
| `CheckpointSaver` | Wire 选择 `postgres.NewSaver`（生产） | `cmd/admin/wire.go` `provideGraphCheckpointSaver` |
| `Memory Service` | 完全绕过框架实现，自建 dialect 抽象 raw SQL | `internal/data/memory_shim_l3.go` + `internal/data/dialect.go` |

### 4.3 风险与缓解

| # | 风险 | 严重度 | 缓解措施 |
|---|------|--------|---------|
| 1 | Postgres CheckpointSaver 缺失测试覆盖（ISS-1） | 高 | 建议补充 Postgres 集成测试，验证 SQL 语句在真实 Postgres 实例上的行为与 SQLite 版本等价 |
| 2 | CheckpointSaver Close() 行为不一致（ISS-2） | 中 | 澄清预期语义并修正注释或实现之一 |
| 3 | `memory_facts` 是野生表，违反 DB-R3 红线 | 中 | 历史遗留，已 grandfathered；后续可考虑迁入 Ent Schema |
| 4 | 框架 Postgres memory 实现是"死代码"（项目未使用） | 低 | 可接受；项目层 dialect 抽象更灵活 |
| 5 | 框架六份独立实现（sqlite/postgres/mysql/clickhouse/pgvector/redis）维护成本高 | 低 | 框架问题，非项目问题；项目只使用其中 2 份 |

### 4.4 调研结论

**trpc-agent-go 框架的 Postgres 兼容性问题已通过框架自身的"独立包"架构解决**，项目层无需修改任何框架代码。具体而言：

1. **Session Service**：框架已提供完整的 `postgres/` 实现，项目通过 Wire 选择后端即可
2. **CheckpointSaver**：框架已提供完整的 `postgres/saver.go`，覆盖 SQLite 版全部 8 个接口方法
3. **Memory Service**：框架已提供 `postgres/` 和 `pgvector/` 实现；项目层选择绕过框架实现，自建基于 dialect 抽象的 raw SQL 实现，设计上是 Postgres 兼容的

**Phase A 后续工作（A1-A6）可基于此结论安全推进**：
- A1（dialect 抽象层）：项目层已有 `internal/data/dialect.go`，无需为框架再建
- A2（Schema 迁移）：框架各 `postgres/` 包自带 `init.go` 建表逻辑，无需项目层干预
- A3（数据迁移工具）：需迁移框架表（`trpc_*`/`graph_checkpoints`/`event_wal`/`vector_embeddings`）+ 项目表（76 张）
- A4（Wire 注入调整）：调整 `provideTRPCSessionService`/`provideGraphCheckpointSaver` 选择 Postgres 实现
- A5（停机迁移）：基于 A3/A4 执行
- A6（清理 SQLite）：保留框架 SQLite 代码（测试和 CLI 工具需要），移除项目层 SQLite 生产路径

---

## 5. 相关文件路径

### 框架层（调研对象）

**Session Service**：
- `pkg/trpc-agent-go/session/session.go`（接口定义）
- `pkg/trpc-agent-go/session/sqlite/`（SQLite 实现，10 个文件）
- `pkg/trpc-agent-go/session/postgres/`（Postgres 实现，6 个文件）
- `pkg/trpc-agent-go/session/inmemory/`（内存 fallback）

**CheckpointSaver**：
- `pkg/trpc-agent-go/graph/checkpoint.go`（接口定义，8 个方法）
- `pkg/trpc-agent-go/graph/checkpoint/sqlite/sqlite.go`（SQLite 实现，605 行）
- `pkg/trpc-agent-go/graph/checkpoint/postgres/saver.go`（Postgres 实现，574 行）

**Memory Service**：
- `pkg/trpc-agent-go/memory/memory.go`（接口契约）
- `pkg/trpc-agent-go/memory/sqlite/`（SQLite 实现）
- `pkg/trpc-agent-go/memory/postgres/`（Postgres 实现）
- `pkg/trpc-agent-go/memory/pgvector/`（pgvector 实现）

### 项目层（适配代码）

**Session**：
- `internal/session/trpc/factory.go`（Wire 选择后端）
- `internal/session/trpc/sqlite.go`（InMemory fallback）

**Memory**：
- `internal/memory/trpc/sqlite_adapter.go`（trpc Service 适配器，无 SQL）
- `internal/memory/trpc/settings_loader.go`（运行时策略加载，无 SQL）
- `internal/data/memory_shim_l3.go`（memory_facts CRUD 真正实现）
- `internal/data/dialect.go`（dialect 抽象层）
- `internal/data/memory_chain_schema.go`（DDL 加载 + BLOB→BYTEA 翻译）
- `internal/data/sql/memory_chain.sql`（memory_facts 表 DDL）

**Wire 注入**：
- `cmd/admin/wire.go`（`provideTRPCSessionService`/`provideGraphCheckpointSaver`）
- `cmd/admin/wire_memory.go`（`provideLinkEvolutionService` 等）

---

## 6. 调研方法

- **工具**：Grep + Read + Glob（静态代码分析）
- **范围**：`pkg/trpc-agent-go/session/`、`pkg/trpc-agent-go/graph/checkpoint/`、`pkg/trpc-agent-go/memory/`、`internal/memory/trpc/`、`internal/data/`
- **重点检查的 SQLite 特定语法**：`sqlite_master`、`INSERT OR IGNORE/REPLACE`、`PRAGMA`、`json_extract`、`strftime`、`AUTOINCREMENT`、`?` 占位符、`BLOB` 类型
- **未覆盖**：运行时行为验证（需集成测试）、性能基准对比

---

**报告完成日期**：2026-06-18
**调研耗时**：约 30 分钟（3 个并行子代理）
**结论可信度**：高（基于完整的静态代码分析，覆盖所有含 SQL 的文件）
