# Database Architecture — 设计文档

> **对应需求**：[66-database-architecture.md](./66-database-architecture.md)
>
> **状态**：✅ 核心架构已落地（2026-06）

---

## 1. 架构总览

### 1.1 分层架构

```
┌───────────────────────────────────────────────────────────────┐
│                      biz 层                                    │
│  Usecase → Repo 接口（定义在 biz） → SpiritTransactor 接口     │
└──────────────────────────┬────────────────────────────────────┘
                           │ 依赖倒置
                           ▼
┌───────────────────────────────────────────────────────────────┐
│                      data 层                                   │
│  Repo 实现 → Data (连接管理) → ReadWriteClient → Ent Client    │
│            → ReadWriteDB → *sql.DB (原生 SQL)                  │
└──────────┬────────────────────────────┬───────────────────────┘
           │                            │
           ▼                            ▼
┌─────────────────────┐    ┌─────────────────────────┐
│   SQLite (主库)      │    │  PostgreSQL (可选)       │
│   Ent ORM            │    │  database/sql + pgvector │
│   WAL 读写分离       │    │  向量搜索 + 知识库       │
└─────────────────────┘    └─────────────────────────┘
```

### 1.2 依赖方向

```
biz (Repo 接口) ← data (Repo 实现) ← Ent/SQL (数据库)
     ↑                                ↑
     └── Wire DI 编译期注入 ──────────┘
```

- biz 层定义 Repo 接口，data 层实现
- data 层不反向依赖 biz 层（除 `biz.SpiritTransactor` 适配器）
- Wire 在编译期解析所有依赖图

---

## 2. 连接管理设计

### 2.1 SQLite 双连接池

```
┌─────────────────────────────────────────────┐
│                  Data                        │
│                                              │
│  writeDB (*sql.DB)     readDB (*sql.DB)      │
│  MaxOpenConns=1        MaxOpenConns=2        │
│  MaxIdleConns=1        MaxIdleConns=2        │
│       │                     │                │
│  writeClient (*ent.Client)  readClient (*ent.Client)
│       │                     │                │
│  Write(ctx) → 事务? → tx.Client() : writeClient
│  Read(ctx)  → 事务? → tx.Client() : readClient
└─────────────────────────────────────────────┘
```

**设计决策**：

| 决策 | 原因 |
|------|------|
| 写连接 MaxOpen=1 | SQLite 单写限制，多连接并发写会导致 SQLITE_BUSY |
| 读连接 MaxOpen=2 | WAL 模式允许并发读，2 连接平衡性能和资源 |
| 事务感知路由 | 事务中必须使用事务客户端，确保读写一致性 |
| PRAGMA WAL | 允许读写并发，避免读写互斥 |
| PRAGMA busy_timeout=30s | 避免短暂锁冲突直接报错 |

### 2.2 PostgreSQL 连接

```
pgDB (*sql.DB)
MaxOpenConns=8
ConnMaxLifetime=0 (无限)
     │
     ├── pgvector.EnsureSchema()
     ├── EnsureKnowledgeSchema()
     └── VectorStore / KnowledgeRepo
```

**降级策略**：PostgreSQL 连接失败不阻断启动，`log.Warn` 后降级为纯 SQLite 模式。

### 2.3 连接初始化流程

```
NewData(bc, lg)
  ├── SQLite 写连接 → PRAGMA → Ent Client → Schema.Create (自动迁移)
  ├── SQLite 读连接 → PRAGMA → Ent Client
  ├── PostgreSQL 连接 (可选) → Ping → pgvector/knowledge schema
  ├── ReadinessGate 初始化
  └── 后台 goroutine (P1)
       ├── ensureSchemaDDL (DDL 迁移)
       ├── ensurePostgresSchemas
       ├── runPendingDataMigrations (数据迁移)
       └── seedP1Data (种子数据)
```

---

## 3. 事务设计

### 3.1 ExecInTx 流程

```
ExecInTx(ctx, fn)
  │
  ├── 检查 ctx 中是否已有事务 (txClientKey)
  │   └── 已有 → 直接执行 fn(ctx)
  │
  ├── 创建独立上下文 (30s 超时)
  │   context.WithTimeout(context.Background(), 30s)
  │
  ├── 开启事务
  │   tx = writeClient.Tx(independentCtx)
  │
  ├── 注入事务到上下文
  │   txCtx = context.WithValue(ctx, txClientKey{}, tx)
  │
  ├── 执行 fn(txCtx)
  │   └── fn 中的 Read/Write 调用自动路由到事务客户端
  │
  ├── 检查原始 ctx 是否已取消
  │   └── 已取消 → tx.Rollback()
  │
  ├── fn 成功 → tx.Commit()
  └── fn 失败 → tx.Rollback()
```

**关键设计决策**：

| 决策 | 原因 |
|------|------|
| 分离上下文 | HTTP 请求取消不应中断数据库事务，事务有独立 30s 超时 |
| 嵌套事务检测 | SQLite 不支持嵌套事务，已事务中则直接执行 |
| 调用者取消检测 | fn 执行成功但调用者已放弃，应回滚而非提交 |
| 30s 硬超时 | SQLite 单写限制下，长事务会阻塞所有写操作 |

### 3.2 事务传播机制

```go
// data 层
func (d *Data) RW() *ReadWriteClient

// ReadWriteClient
func (c *ReadWriteClient) Read(ctx context.Context) *ent.Client {
    if tx, ok := ctx.Value(txClientKey{}).(*ent.Tx); ok {
        return tx.Client()  // 事务中
    }
    return c.read  // 非事务
}

func (c *ReadWriteClient) Write(ctx context.Context) *ent.Client {
    if tx, ok := ctx.Value(txClientKey{}).(*ent.Tx); ok {
        return tx.Client()  // 事务中
    }
    return c.write  // 非事务
}
```

**原生 SQL 同理**：`ReadWriteDB.ReadDB(ctx)` / `ReadWriteDB.WriteDB(ctx)` 确保原生 SQL 也参与事务。

### 3.3 biz 层事务抽象

```go
// biz 层接口
type SpiritTransactor interface {
    ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// data 层适配器
type spiritTransactorAdapter struct {
    data *Data
}
func (a *spiritTransactorAdapter) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
    return a.data.ExecInTx(ctx, fn)
}
```

biz 层通过接口依赖事务能力，不直接依赖 `*Data`。

---

## 4. 迁移系统设计

### 4.1 三层迁移架构

```
┌───────────────────────────────────────────────────┐
│ 第一层: Ent 自动迁移                               │
│ Schema.Create(ctx) → 自动建表/加列                  │
│ 开发模式: migrateDev (允许删索引)                    │
│ 生产模式: Schema.Create (只增不减)                   │
└───────────────────────────────────────────────────┘
         ↓ (Ent 无法管理的表/列/索引)
┌───────────────────────────────────────────────────┐
│ 第二层: DDL 迁移注册表                              │
│ ddlMigration{Version, Name, SQL, Func}             │
│ → schema_migrations 表记录已执行版本                 │
│ → 幂等: duplicate column/already exists 视为成功    │
└───────────────────────────────────────────────────┘
         ↓ (需要数据回填/转换)
┌───────────────────────────────────────────────────┐
│ 第三层: 数据迁移                                    │
│ 独立版本号管理，在 DDL 迁移后执行                    │
│ → 回填字段、转换数据格式                             │
└───────────────────────────────────────────────────┘
```

### 4.2 DDL 迁移注册表设计

```go
type ddlMigration struct {
    Version int       // 版本号 (如 20260601)
    Name    string    // 迁移名称
    SQL     string    // SQL 文件路径 (可选)
    Func    func(ctx, db, lg) error  // Go 函数 (可选)
}
```

**执行流程**：

```
ensureSchemaDDL()
  ├── 遍历 ddlMigrations (按 Version 排序)
  ├── 查询 schema_migrations 表是否已有该 Version
  ├── 未执行 → 执行 SQL (如有) → 执行 Func (如有)
  ├── 记录 Version 到 schema_migrations
  └── 幂等: 已执行则跳过
```

**当前迁移清单**（30+ 个）：

| 版本号 | 名称 | 说明 |
|--------|------|------|
| 20260601 | session_memory_patches | Session 记忆补丁 |
| 20260603 | messages_turn_number | 消息轮次编号 |
| 20260607 | agent_runtime_patches | Agent 运行时补丁 |
| 20260624 | message_fts_schema | FTS5 全文搜索 |
| 20260628 | session_run_schema | Session 运行 Schema |
| 20260705 | compiled_team_schema | 编译 Team Schema |
| 20260708 | session_table_split | Session 表拆分 |
| 20260709 | vector_embedding_ref | 向量 Embedding 引用 |
| 20260716 | missing_indexes | 补缺失索引 |
| 20260718 | ecosystem_preset_schema | 生态预设 Schema |
| 20260719 | agent_source_column | Agent source 列 |

### 4.3 数据迁移设计

独立于 DDL 迁移，处理数据回填场景：

| 迁移 | 版本号 | 说明 |
|------|--------|------|
| LegacyTRPCMemoryFacts | 20260524 | 从旧框架迁移 memory facts |
| TurnIndexToTurnID | 20260528 | turn_index → turn_id |
| SessionStatusIdle | 20260531 | active → idle |

**执行时序**：DDL 迁移 → 数据迁移 → 种子数据

---

## 5. Repository 设计

### 5.1 接口组合模式

```
AgentRepository (组合接口)
  ├── AgentReader (只读)
  │   ├── GetByID(ctx, id) → *Agent
  │   ├── GetByKey(ctx, key) → *Agent
  │   ├── ListByFilter(ctx, opts) → []*Agent
  │   └── ...
  ├── AgentWriter (写入)
  │   ├── Create(ctx, opts) → *Agent
  │   ├── Update(ctx, id, opts) → *Agent
  │   └── Delete(ctx, id) → error
  ├── AgentAtomicWriter (事务化)
  │   └── AtomicCreateWithSettings(ctx, ...) → *Agent
  ├── AgentRuntimeSettingsRepo
  │   ├── GetSettings(ctx, agentID) → *RuntimeSettings
  │   └── UpsertSettings(ctx, agentID, ...) → *RuntimeSettings
  ├── AgentPromptFileRepo
  │   └── ListByAgent(ctx, agentID) → []*PromptFile
  └── AgentReferenceChecker
      └── IsReferencedBySession(ctx, agentID) → (bool, error)
```

**设计原则**：

- Usecase 只依赖需要的最小接口集
- 接口按职责拆分（读写分离、原子操作、引用检查）
- 组合接口在 biz 层定义，data 层实现完整组合

### 5.2 查询构建模式

```
Repo 方法 → ReadWriteClient.Read/Write(ctx)
  → Ent Client 查询构建器
    ├── Where 条件 (软删除过滤 + 业务条件)
    ├── Order 排序
    ├── Offset/Limit 分页
    └── Select 字段选择
```

**软删除过滤**：几乎所有查询包含 `deleted_at = ""` 条件。

### 5.3 原生 SQL 查询

当 Ent 查询构建器无法满足时（如 FTS5、聚合统计、跨表 JOIN），使用原生 SQL：

```go
func (r *sessionRepo) SearchMessages(ctx, sessionID, query, ...) ([]*Message, error) {
    // 优先 FTS5
    rows, err := r.d.RWDB().ReadDB(ctx).QueryContext(ctx, fts5SQL, ...)
    // 回退 LIKE
    rows, err := r.d.RWDB().ReadDB(ctx).QueryContext(ctx, likeSQL, ...)
}
```

---

## 6. 级联删除设计

### 6.1 为什么不用数据库外键

| 原因 | 说明 |
|------|------|
| Ent Schema 无 Edge | 项目设计决策，关系通过字符串外键维护 |
| 灵活性 | 应用层级联可控制删除顺序和策略（软删/硬删混合） |
| SQLite 限制 | 外键约束在 WAL 模式下性能影响 |

### 6.2 级联删除实现

```
cascadeDeleteByAgent(ctx, agentID)
  ├── 硬删 agent_runtime_settings
  ├── 硬删 agent_prompt_files
  ├── 软删 sessions (设置 deleted_at)
  └── 硬删 tool_agent_overrides

cascadeDeleteBySession(ctx, sessionID)  // 事务内
  ├── 硬删 session_turns
  ├── 硬删 session_participants
  ├── 硬删 tool_invocations
  ├── 硬删 messages (+ FTS 同步)
  ├── 硬删 event_store
  ├── 硬删 session_runs
  ├── 硬删 session_runtime
  ├── 硬删 session_metrics
  ├── 硬删 channel_turn_job
  └── ... (共 14 个关联表)

cascadeDeleteByTeam(ctx, teamID)
  ├── 硬删 team_run_steps
  ├── 硬删 team_runs
  └── 硬删 compiled_teams

cascadeDeleteByChannel(ctx, channelID)
  ├── 硬删 channel_peer_session
  ├── 硬删 channel_credential
  ├── 硬删 channel_delivery
  ├── 硬删 channel_inbound_receipt
  ├── 硬删 channel_turn_job
  └── 硬删 channel_runtime_lease
```

---

## 7. 向量存储设计

### 7.1 VectorStore 接口

```go
type VectorStore interface {
    Upsert(ctx context.Context, id string, embedding []float64, meta map[string]string) error
    Search(ctx context.Context, embedding []float64, topK int, minScore float64) ([]VectorHit, error)
    Delete(ctx context.Context, id string) error
}
```

### 7.2 双实现

```
┌──────────────────────────────────────────────────────┐
│                  VectorStore 接口                     │
└──────────┬──────────────────────────┬────────────────┘
           │                          │
           ▼                          ▼
┌─────────────────────┐    ┌─────────────────────────┐
│ SQLiteVectorStore   │    │ PgVectorStore           │
│ embedding → JSON    │    │ embedding → vector(1536) │
│ Go 侧余弦相似度     │    │ DB 侧余弦距离            │
│ 全表扫描            │    │ 索引加速                 │
│ 开发/回退           │    │ 生产                     │
└─────────────────────┘    └─────────────────────────┘
```

**选择逻辑**：

```
conf.DAOVectorPgVector() == true && PostgreSQL 可用?
  → Yes: PgVectorStore
  → No:  SQLiteVectorStore
```

### 7.3 记忆系统 embedding_blob

记忆系统（L0-L4）的向量存储不走 VectorStore 接口，而是在原生 SQL 表中使用 `embedding_blob BLOB` 字段：

- `memory_episodes.embedding_blob`
- `memory_l2_index_meta.embedding_blob`
- `memory_facts.embedding_blob`
- `memory_entities.embedding_blob`

配合 `embedding_norm` 字段做余弦相似度计算。

---

## 8. FTS5 全文搜索设计

### 8.1 虚拟表

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
  message_id UNINDEXED,
  session_id UNINDEXED,
  content_markdown,
  tokenize = 'unicode61'
);
```

### 8.2 自动同步触发器

```
messages 表 INSERT → messages_fts_ai 触发器 → FTS 插入
messages 表 DELETE → messages_fts_ad 触发器 → FTS 删除
messages 表 UPDATE → messages_fts_au 触发器 → FTS 更新
```

### 8.3 查询模式

```sql
SELECT m.*, snippet(messages_fts, 2, '>>>', '<<<', '...', 32) as highlight
FROM messages_fts f
JOIN messages m ON m.id = f.message_id
WHERE messages_fts MATCH ?
ORDER BY bm25(messages_fts)
LIMIT ? OFFSET ?
```

**回退策略**：FTS 表不存在时回退到 `content_markdown LIKE ?`。

---

## 9. Wire DI 注入设计

### 9.1 ProviderSet

```go
// data 层
var ProviderSet = wire.NewSet(
    NewData,
    NewAdminRepo,
    NewAgentRepo,
    NewTeamRepo,
    NewSessionRepo,
    // ... 80+ 个 Provider
)

// biz 层
var ProviderSet = wire.NewSet(
    NewAdminUsecase,
    NewAgentUsecase,
    NewTeamUsecase,
    // ... 30+ 个 Provider
)
```

### 9.2 注入链路

```
conf.Data → NewData → *Data
*Data + loggateway.Logger → NewAgentRepo → biz.AgentRepository
biz.AgentRepository + ... → NewAgentUsecase → *biz.AgentUsecase
```

### 9.3 事务注入

```
*Data → spiritTransactorAdapter → biz.SpiritTransactor
biz.SpiritTransactor → biz.Usecase (需要事务的 Usecase)
```

---

## 10. 错误处理设计

### 10.1 entErrToBizErr

```
Ent 错误 → entErrToBizErr(err, domain, msg)
  ├── NotFound → biz.ErrNotFound (404)
  ├── ConstraintError → biz.ErrConflict (409)
  ├── NotLoaded → biz.ErrBadRequest (400)
  └── 其他 → biz.ErrInternal (500)
```

### 10.2 迁移错误处理

| 场景 | 处理 |
|------|------|
| DDL 迁移 `duplicate column` | 视为成功（幂等） |
| DDL 迁移 `already exists` | 视为成功（幂等） |
| 数据迁移失败 | `log.Error` + `MarkFailed()` → 健康检查失败 |
| PostgreSQL 连接失败 | `log.Warn` + 降级为纯 SQLite |

---

## 11. 性能设计

### 11.1 SQLite 优化

| 优化 | 实现 |
|------|------|
| WAL 模式 | 允许并发读写 |
| 读写分离 | 写=1连接，读=2连接 |
| busy_timeout=30s | 避免短暂锁冲突报错 |
| synchronous=NORMAL | 平衡性能和安全 |
| FTS5 | 消息全文搜索，避免 LIKE 全表扫描 |
| 索引 | 关键查询路径均有索引覆盖 |

### 11.2 PostgreSQL 优化

| 优化 | 实现 |
|------|------|
| pgvector 索引 | 向量搜索使用 IVFFlat/HNSW 索引 |
| 连接池 | MaxOpenConns=8 |
| 降级 | 连接失败不阻断启动 |

### 11.3 查询优化

| 模式 | 实现 |
|------|------|
| 分页 | `PageToLimitOffset`，默认 20 条，最大 100 条 |
| AIP 过滤 | `go.einride.tech/aip/filtering`，AIP-160 表达式 |
| AIP 排序 | `go.einride.tech/aip/ordering`，AIP-132 排序 |
| 软删除 | `deleted_at = ""` 条件过滤 |
| 字段选择 | Ent `Select()` 只查询需要的字段 |

---

## 12. 关键文件索引

| 文件 | 设计职责 |
|------|---------|
| `internal/data/data.go` | Data 结构体、初始化流程、ProviderSet |
| `internal/data/tx.go` | 事务管理、嵌套检测、上下文分离 |
| `internal/data/readwrite.go` | ReadWriteClient 读写分离 |
| `internal/data/readwrite_db.go` | ReadWriteDB 原生 SQL 读写分离 |
| `internal/data/errors.go` | 错误转换映射 |
| `internal/data/readiness.go` | ReadinessGate 三态门控 |
| `internal/data/ddl_migration_registry.go` | DDL 迁移注册表 |
| `internal/data/schema_migrations.go` | 数据迁移门控 |
| `internal/data/cascade_delete.go` | 级联删除逻辑 |
| `internal/data/vector/store.go` | VectorStore 接口 |
| `internal/data/vector/sqlite.go` | SQLite 向量实现 |
| `internal/data/vector/pgvector.go` | PgVector 向量实现 |
| `internal/data/sql/message_fts.sql` | FTS5 全文搜索 |
| `internal/data/sql/memory_chain.sql` | 记忆系统核心表 |
| `internal/biz/shared/shared.go` | 分页、ListOptions |
