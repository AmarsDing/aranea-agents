## Context

Aranea-Agents 的数据访问层经过 Phase 1-3 治理后，单 Turn 写入从 8~17 次降至 ~3 次，但存在 6 大结构性问题无法通过渐进式修补解决：

- sessions 表（52 字段）承载 4 类不同变更频率的数据（冷元数据/热聚合/运行时状态/版本控制），一次 Turn 同步写入 7~14 次 UPDATE
- sessionmemory.Store 是 96 方法的 God Object，直接满足 22 个 biz 接口，绕过 Ent ORM、绕过统一事务
- memory_chain.sql 含 34 张纯野生表 + 23 张与 Ent 重叠的表（共 57 张）
- 读写分离合规率仅 3%（2/59 Repo），~10 个 Repo 写操作绕过 txClient，~13 个 Repo 读操作走写连接
- 向量存储双写（SQLite + Postgres），一致性无保障
- 错误处理 3 种风格不统一，data 层零 DB 指标

技术约束：
- SQLite 单写者模型（MaxOpenConns=1），WAL 模式支持并发读
- Ent v0.14.6 的 `Schema.Create` 对 SQLite 仅支持创建新表/新列，不支持修改已有列
- FTS5 虚拟表完全在 Ent 体系之外
- Proto `v1.Session` 包含 16 个 metrics 字段，前端 API 契约需保持兼容

## Goals / Non-Goals

**Goals:**

1. Session 表冷热拆分，同步写入降至 2~3 次/Turn
2. sessionmemory.Store 拆分为 6 个独立 Repo，消除架构旁路
3. 统一读写分离和事务感知（ReadWriteClient 抽象），合规率 100%
4. 向量存储统一为策略模式（VectorStore 接口）
5. 统一错误翻译（entErrToBizErr），data 层 DB 指标
6. 野生表渐进式纳入 Ent Schema，Schema 单一真相源
7. 所有变更通过 feature flag 控制，支持灰度和回滚

**Non-Goals:**

- 不更换 SQLite 为 Postgres 作为主数据库
- 不重写前端（仅适配新事件和 API 响应格式）
- 不引入分布式缓存（Redis 等）
- 不修改 Proto 定义
- 不在本次变更中实施完整的 Session 冷热分离（仅做表拆分基础）

### 量化对比

| 维度 | 当前 | 治理后（Phase 4-6） | 全新设计 |
|------|------|---------------------|---------|
| sessions 表同步写入/Turn | 7~14 次 | ~3 次 | **2~3 次** |
| 纯野生表数量 | 34 张 | ~15 张 | **0** |
| Store 方法数 | 96 | 96（拆为 6 Repo） | **6×4~16** |
| Store 直接满足 biz 接口数 | 22 | 0 | **0** |
| 读写分离合规率 | 3% | ~80% | **100%** |
| 事务模式种类 | 5 种 | 2 种 | **1 种** |
| Schema 真相源 | 3 轨 | 2 轨 | **1 轨** |
| 向量存储 | 双写 | 双写 | **策略模式单写** |
| Patch 文件数 | 12 | 0（迁移框架替代） | **0** |

## Decisions

### Decision 1：Session 表拆分策略

**选择**：三表拆分（sessions + session_metrics + session_runtime）

```
sessions（冷元数据，创建后几乎不变）
├── id, workspace_id, user_id, owner_type
├── agent_id, team_id
├── title, summary, tags_json
├── dialog_mode, default_provider, default_model
├── default_context_window_tokens
├── status, status_reason, status_changed_at
├── pinned, archived_at
└── created_at, updated_at

session_metrics（热聚合，异步刷新）
├── session_id (PK, FK→sessions.id)
├── message_count, model_call_count
├── tool_call_count, skill_call_count, mcp_call_count
├── input_tokens, output_tokens, total_tokens
├── total_cost_micro_usd, avg_latency_ms, error_count
├── last_message_at
└── updated_at

session_runtime（运行时状态，Turn 内频繁变更）
├── session_id (PK, FK→sessions.id)
├── session_revision
├── state_json
├── runner_snapshot_json
├── context_used_ratio, max_context_used_ratio
├── context_status, context_used_tokens
└── updated_at
```

**替代方案**：
- A) 两表拆分（sessions + session_metrics）：runtime 字段留在 sessions → 运行时 UPDATE 仍锁 sessions 行，收益有限
- B) 四表拆分（+ session_version）：revision 独立表 → 过度拆分，JOIN 开销增加

**理由**：三表拆分将 3 类变更频率完全隔离。metrics 异步写入（已有 Delta 机制），runtime 合并写入（4~5 次 state_json patch → 1~2 次），sessions 主表仅在创建/状态变更时写入。

**API 兼容**：service 层 `toProtoSession` 聚合三表数据。Session 列表查询通过 LEFT JOIN session_metrics 一次获取，无需二次查询。单 Session 查询同样 JOIN。

### Decision 2：sessionmemory.Store 拆分方案

**选择**：按 Memory 层级拆分为 6 个独立 Repo struct

```
internal/data/memory/
├── l0_snapshot_repo.go      L0SnapshotRepo (4 方法)
├── l1_working_memory_repo.go L1WorkingMemoryRepo (8 方法)
├── l2_episode_repo.go       L2EpisodeRepo (12 方法)
├── l3_fact_repo.go          L3FactRepo (16 方法)
├── l4_entity_repo.go        L4EntityRepo (12 方法)
└── cascade_repo.go          CascadeRepo (14 方法)
```

**替代方案**：
- A) 保持 Store 不变，仅增加 adapter：God Object 仍在，维护成本持续增长
- B) 完全删除 Store，方法分散到各文件但仍是同一 struct：方法数不变，仅文件拆分

**理由**：独立 struct 意味着独立生命周期、独立测试、独立事务边界。每个 Repo 持有 `*Data` 而非 `*ent.Client`，通过 `ReadWriteClient` 访问连接。Store 的 `TxManager` 接口废弃，统一使用 `Data.ExecInTx`。

**biz 接口映射**：

| data Repo | biz 端口接口 | 方法数 |
|-----------|-------------|--------|
| L0SnapshotRepo | L0AdminStore | 4 |
| L1WorkingMemoryRepo | L1TaskWriter + L1FieldWriter + L1AdminReader + L1IdleTaskReader | 8 |
| L2EpisodeRepo | L2ConsolidationStore + L2RecallStore + L2EpisodeWriter + MemoryEpisodeDecayer + MemoryEpisodeBackfillReader | 12 |
| L3FactRepo | L3FactReader + L3FactWriter + L3ConflictStore + PIIReviewStore + MemoryFactDecayer + MemoryFactIndexMaintainer + MemoryFactIndexCounter | 16 |
| L4EntityRepo | L4EntityStore + L4EvolutionStore + L4GraphRepo + L4DecayWriter | 12 |
| CascadeRepo | CascadeProposalStore + CascadeGraphReader + CascadeFactMutator + CascadeSagaStore | 14 |

**迁移策略**：
1. 创建 6 个新 Repo，每个方法委托到 Store 对应方法（shim 阶段）
2. 逐步将 Store 内的 Raw SQL 迁移到新 Repo（Ent API 优先，Raw SQL 保留）
3. 删除 Store，所有调用指向新 Repo

### Decision 3：ReadWriteClient 自动路由抽象

**选择**：引入 `ReadWriteClient` struct，封装读写分离 + 事务感知逻辑

```go
// internal/data/readwrite.go
type ReadWriteClient struct {
    write *ent.Client  // entClient (MaxOpenConns=1)
    read  *ent.Client  // readClient (MaxOpenConns=2)
}

func (c *ReadWriteClient) Read(ctx context.Context) *ent.Client {
    if tx := EntTxFromCtx(ctx); tx != nil {
        return tx.Client()  // 事务中用事务 client
    }
    return c.read
}

func (c *ReadWriteClient) Write(ctx context.Context) *ent.Client {
    if tx := EntTxFromCtx(ctx); tx != nil {
        return tx.Client()  // 事务中用事务 client
    }
    return c.write
}

// RawSQL 版本
type ReadWriteDB struct {
    write *sql.DB  // rawDB
    read  *sql.DB  // readDB
}

func (c *ReadWriteDB) ReadDB(ctx context.Context) *sql.DB {
    if tx := RawTxFromCtx(ctx); tx != nil {
        return tx  // 事务中用事务连接
    }
    return c.read
}

func (c *ReadWriteDB) WriteDB(ctx context.Context) *sql.DB {
    if tx := RawTxFromCtx(ctx); tx != nil {
        return tx
    }
    return c.write
}
```

**替代方案**：
- A) 保持每个 Repo 手动实现 readClient/txClient：合规率仅 3%，重复代码多
- B) 在 Data 层提供 `ReadClient(ctx)` / `WriteClient(ctx)` 方法：可行但需每个 Repo 调用 `r.data.ReadClient(ctx)`

**理由**：ReadWriteClient 作为 Repo 的组合字段，调用更简洁（`r.rw.Read(ctx)` vs `r.data.ReadClient(ctx)`），且可独立测试。所有 Repo 统一模式，消除手动实现的遗漏。

### Decision 4：VectorStore 策略模式

**选择**：定义 `VectorStore` 接口，SQLite/Postgres 各一个实现

```go
// internal/data/vector/store.go
type VectorStore interface {
    Upsert(ctx context.Context, id string, embedding []float32, model string, dim int) error
    Search(ctx context.Context, embedding []float32, limit int, minScore float64) ([]VectorHit, error)
    Delete(ctx context.Context, id string) error
}

// SQLite 实现：json_set 存储 + Go 侧余弦相似度
type SQLiteVectorStore struct { db *ReadWriteDB }

// Postgres 实现：pgvector + IVFFlat/HNSW
type PgVectorStore struct { db *sql.DB; dim int }
```

**替代方案**：
- A) 保持双写：一致性无保障，维护成本高
- B) 仅用 Postgres：开发/单机部署无 Postgres 可用
- C) 仅用 SQLite：大规模向量搜索性能不足

**理由**：策略模式允许按配置选择。开发环境用 SQLite（零依赖），生产环境用 Postgres（高性能）。memory_facts 表的 `embedding_blob`/`embedding_norm` 列改为 `embedding_ref TEXT`（向量存储中的 ID），消除双写。

### Decision 5：统一错误翻译

**选择**：`entErrToBizErr()` 函数 + Repo 级别可选覆盖

```go
// internal/data/errors.go
func entErrToBizErr(err error, domain, msg string) error {
    if err == nil { return nil }
    if ent.IsNotFound(err) {
        return kerrors.NotFound(domain, msg)
    }
    if ent.IsConstraintError(err) {
        return kerrors.Conflict(domain, msg)
    }
    if ent.IsNotLoaded(err) {
        return kerrors.BadRequest(domain, msg)
    }
    return kerrors.InternalServer(domain, msg)
}
```

**理由**：当前 3 种错误翻译风格（kerrors / biz 领域错误 / fmt.Errorf）导致上层无法统一处理。统一为 kerrors 后，service 层可直接映射为 HTTP 状态码。

### Decision 6：野生表纳入 Ent 的渐进策略

**选择**：按频率分 3 批纳入，每批独立可交付

| 批次 | 表 | 频率 | 策略 |
|------|---|------|------|
| Batch 1 | session_runs, session_participants, session_run_checkpoints, channel_inbound_receipts, channel_turn_jobs, channel_runtime_lease | 高 | Ent Schema + API，Raw SQL 仅保留 UPSERT 特殊逻辑 |
| Batch 2 | memory_facts, memory_entities, memory_relations, memory_episodes, memory_l1_tasks, memory_l1_fields | 中 | Ent Schema 定义 + Raw SQL 查询（复杂查询保留 Raw SQL） |
| Batch 3 | 其余 ~40 张表 | 低 | Ent Schema 定义，查询逐步迁移 |

**替代方案**：
- A) 一次性全量纳入：风险太高，工作量大
- B) 仅纳入 Batch 1：不够，Memory 子系统的表仍是野生

**理由**：Batch 1 的 6 张表是当前 Raw SQL Repo 中读写分离违规最严重的（6 个 Repo 的 `db()` 全部返回 `RawDB()`），纳入后可直接使用 `ReadWriteClient`。

### Decision 7：Schema 迁移系统增强

**选择**：增强现有 `ddl_migration_registry`，支持 SQL 文件版本化

```go
// 当前：Go 函数注册
ddlMigrations = []ddlMigration{
    {Version: 20260601, Name: "xxx", Func: ensureXxxPatch},
}

// 增强：支持 SQL 文件注册
ddlMigrations = []ddlMigration{
    {Version: 20260710, Name: "session_table_split", SQL: "sql/migrations/20260710_session_table_split.sql"},
    {Version: 20260711, Name: "memory_repo_shim", Func: ensureMemoryRepoShim},
}
```

**理由**：Ent 的 `Schema.Create` 对 SQLite 的增量支持有限（不修改已有列、不支持 FTS5），自研迁移系统是必要的。增强方向是支持 SQL 文件注册（而非仅 Go 函数），减少 Go 代码中的 SQL 字符串。

### Decision 8：Session Metrics 缓存策略

**选择**：进程内 LRU 缓存 + EventBus 事件通知

```go
// internal/data/session_metrics_cache.go
type SessionMetricsCache struct {
    cache *lru.Cache[string, biz.SessionMetrics]
    repo  SessionMetricsReader
    lg    loggateway.Logger
}

func (c *SessionMetricsCache) Get(ctx context.Context, sessionID string) (*biz.SessionMetrics, error) {
    if v, ok := c.cache.Get(sessionID); ok {
        return &v, nil
    }
    m, err := c.repo.GetSessionMetrics(ctx, sessionID)
    if err != nil { return nil, err }
    c.cache.Add(sessionID, *m)
    return m, nil
}

// Metrics 写入后失效缓存 + 发布事件
func (c *SessionMetricsCache) Invalidate(sessionID string) {
    c.cache.Remove(sessionID)
}
```

**替代方案**：
- A) 无缓存，每次 JOIN 查询：拆表后列表查询性能下降
- B) Redis 缓存：引入外部依赖，违背 Non-Goals

**理由**：Session 列表是最高频的读操作，拆表后 JOIN session_metrics 增加查询开销。LRU 缓存（容量 ~500）可覆盖活跃 Session，命中率预计 >90%。Metrics 异步写入时失效缓存 + 发布 `EnvelopeTypeMetricsUpdated` 事件通知前端。

## Risks / Trade-offs

### [Risk] 拆表后 Session 列表查询性能下降

拆为 3 表后，`SearchSessions` 需要 LEFT JOIN session_metrics。当前 SQLite 读连接池 MaxOpenConns=2，JOIN 查询可能增加延迟。

→ **Mitigation**：引入 SessionMetricsCache 缓存层。列表查询先从 sessions 表获取 ID 列表，再从缓存批量获取 metrics。缓存未命中时回退到 JOIN 查询。缓存容量 500，TTL 30s。

### [Risk] Store 拆分导致 Wire 绑定大量变更

Store 拆为 6 个 Repo 后，Wire 图中 ~30 个绑定需要更新。wire_memory.go 中的 `wireSessionAdminStoreAdapter` 需要重写。

→ **Mitigation**：分两阶段——shim 阶段（新 Repo 委托到 Store，Wire 绑定不变）+ 独立阶段（删除 Store，更新 Wire）。每阶段独立可验证。

### [Risk] 迁移过程中数据不一致

sessions 表拆分需要将现有数据迁移到 session_metrics 和 session_runtime。迁移期间新写入可能分散到新旧表。

→ **Mitigation**：使用 feature flag 控制写入路径。迁移流程：
1. 创建新表（DDL migration）
2. 回填历史数据（data migration）
3. 开启双写（feature flag = dual_write）
4. 验证数据一致性
5. 切换到新表（feature flag = new_table）
6. 停止旧字段写入

### [Risk] Ent AutoMigrate 对 SQLite 的限制

Ent `Schema.Create` 对 SQLite 不支持修改已有列、不支持 FTS5。野生表纳入 Ent 后，列变更仍需 DDL migration。

→ **Mitigation**：保持自研迁移系统作为 Ent 的补充。Ent Schema 是"结构定义的真相源"，DDL migration 是"结构变更的执行器"。新列添加通过 DDL migration 注册，不依赖 Ent 的增量迁移。

### [Risk] 前端 reconcilePatchFromServer 旧值覆盖

Session metrics 异步写入后，前端可能在 metrics 刷新前拉取到旧值，覆盖本地更新。

→ **Mitigation**：前端 `reconcilePatchFromServer` 增加 revision 比较逻辑——仅当服务端 revision > 本地 revision 时才更新 metrics 字段。同时新增 `EnvelopeTypeMetricsUpdated` WebSocket 事件，前端收到后主动拉取最新 metrics。

## Migration Plan

### Phase 4：收口（Week 1-2）

1. 创建 6 个 Memory Repo（shim 阶段，委托到 Store）
2. wire 适配器归位到 data 层
3. 消除 Store 直接满足 biz 接口
4. Store 方法参数去 biz 依赖
5. monitor.Repo / a2a.Repo 接口拆分
6. Delta 溢出安全阀
7. 移除 Store.Client()

### Phase 5：基础设施（Week 3-4）

1. 实现 ReadWriteClient / ReadWriteDB 抽象
2. 实现 entErrToBizErr() 统一错误翻译
3. 实现 data 层 DB 指标
4. 实现 SessionMetricsCache
5. 所有 Repo 迁移到 ReadWriteClient

### Phase 6：表拆分（Week 5-7）

1. 创建 session_metrics / session_runtime Ent Schema
2. DDL migration 创建新表 + 回填数据
3. Feature flag 双写过渡
4. service 层 toProtoSession 适配
5. 新增 EnvelopeTypeMetricsUpdated 事件
6. 前端适配

### Phase 7：Store 独立化 + 野生表纳入（Week 8-11）

1. Store 方法逐步迁移到独立 Repo（替换 Raw SQL 为 Ent API）
2. Batch 1 野生表纳入 Ent Schema
3. memory_chain.sql 缩减
4. 删除 Store

### 回滚策略

每个 Phase 独立可回滚：
- Phase 4-5：纯代码变更，git revert 即可
- Phase 6：feature flag 控制写入路径，回滚 = 切换 flag
- Phase 7：保留 Store shim 作为回退路径

## Open Questions

1. **session_runtime 的 state_json 合并写入**：当前 4~5 次 patch 能否合并为 1~2 次？需要评估 trpc-agent-go 框架回调的时序约束。
2. **VectorStore 的 SQLite 实现性能**：Go 侧余弦相似度在大向量集（>10K）上的性能是否可接受？可能需要 SQLite 的 vector 扩展。
3. **SessionMetricsCache 的一致性模型**：缓存 TTL 30s 是否可接受？是否需要 write-through 而非 write-behind？
4. **Batch 2 Memory 表纳入 Ent 的 ROI**：Memory 子系统的查询复杂度高（向量搜索、级联查询、JSON 聚合），纳入 Ent 后是否真的能减少 Raw SQL？
