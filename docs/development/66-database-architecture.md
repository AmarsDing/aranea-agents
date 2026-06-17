# Database Architecture — 需求文档

> **定位**：Aranea-Agents 数据库架构模块的需求规格。
>
> **范围**：定义数据库架构需要支持的业务能力、非功能性约束与验收标准。技术设计见 [66-database-architecture.design.md](./66-database-architecture.design.md)，开发进度见 [66-database-architecture.development.md](./66-database-architecture.development.md)。

---

## 1. 模块定位

数据库架构是 Aranea-Agents 多智能体编排平台的数据持久化基座。它为 Agent 编排、Team 协作、Session 管理、Message 存储、Tool 调用、Memory 系统、Graph 任务、监控告警、用量统计等全部业务域提供统一的数据访问能力。

本模块不直接面向终端用户，但它的可用性、性能、一致性直接决定上层所有业务模块的行为契约。

---

## 2. 用户故事

### 2.1 平台运维者视角

| 编号 | 用户故事 | 优先级 |
|------|---------|--------|
| US-1 | 作为平台运维者，我希望系统能够在单机 SQLite 模式下开箱即用，无需额外部署数据库，以便快速验证和开发。 | P0 |
| US-2 | 作为平台运维者，我希望系统能够可选地接入 PostgreSQL 用于向量搜索和知识库，以便在生产环境扩展语义检索能力。 | P1 |
| US-3 | 作为平台运维者，我希望数据库 Schema 变更能够通过版本化迁移自动执行，无需手动操作数据库，以便降低部署风险。 | P0 |
| US-4 | 作为平台运维者，我希望系统能够在数据库迁移完成前拒绝外部请求，以避免请求命中未就绪的 Schema。 | P0 |
| US-5 | 作为平台运维者，我希望系统能够在 PostgreSQL 不可用时自动降级为纯 SQLite 模式，以保证核心业务不中断。 | P1 |
| US-6 | 作为平台运维者，我希望数据库错误能够被翻译为统一的业务错误码（404/409/400/500），以便上层模块和前端能够正确处理。 | P0 |

### 2.2 业务开发者视角

| 编号 | 用户故事 | 优先级 |
|------|---------|--------|
| US-7 | 作为业务开发者，我希望能够通过 Repo 接口访问数据，而不需要关心底层 SQL 细节，以便聚焦业务逻辑。 | P0 |
| US-8 | 作为业务开发者，我希望事务能够自动传播，在事务内的所有读写操作使用同一连接，以保证一致性。 | P0 |
| US-9 | 作为业务开发者，我希望 HTTP 请求取消不会中断正在执行的事务，以避免数据库状态不一致。 | P0 |
| US-10 | 作为业务开发者，我希望软删除能够自动过滤，查询时不需要每次手动添加 `deleted_at = ""` 条件。 | P1 |
| US-11 | 作为业务开发者，我希望能够使用 AIP-160 风格的过滤表达式和 AIP-132 风格的排序表达式进行列表查询，以便提供灵活的查询能力。 | P1 |
| US-12 | 作为业务开发者，我希望删除 Agent/Session/Team/Channel 时能够自动级联清理关联数据，以避免数据残留。 | P0 |

### 2.3 上层业务模块视角

| 编号 | 用户故事 | 优先级 |
|------|---------|--------|
| US-13 | 作为 Session 模块，我希望能够存储和检索会话、消息、轮次、参与者、运行记录等数据，以支撑多轮对话。 | P0 |
| US-14 | 作为 Memory 模块，我希望能够持久化 L0-L4 分层记忆（摘要、工作记忆、情景记忆、语义记忆、实体记忆），以支撑长期记忆能力。 | P0 |
| US-15 | 作为 Message 模块，我希望能够对消息内容进行全文搜索，以支持历史消息检索。 | P0 |
| US-16 | 作为 Memory/Knowledge 模块，我希望能够进行向量相似度搜索，以支撑语义检索和 RAG。 | P1 |
| US-17 | 作为 Tool 模块，我希望能够记录工具调用、参数、结果、审计日志，以支撑工具调用追踪。 | P0 |
| US-18 | 作为 Usage 模块，我希望能够统计 Token 用量、计算费用、管理配额和预算告警，以支撑成本控制。 | P1 |

---

## 3. 功能需求

### 3.1 双库架构

| 编号 | 需求 | 验收标准 |
|------|------|---------|
| FR-1 | 系统必须支持 SQLite 作为主库，承载全部业务实体 CRUD | 启动后所有业务实体的增删改查均通过 SQLite 完成 |
| FR-2 | 系统必须支持可选的 PostgreSQL 作为向量库和知识库 | 配置 PostgreSQL 连接串后，向量搜索和知识库查询走 PostgreSQL |
| FR-3 | SQLite 必须启用 WAL 模式以支持并发读写 | 启动后 `PRAGMA journal_mode` 返回 `wal` |
| FR-4 | SQLite 必须启用外键约束 | 启动后 `PRAGMA foreign_keys` 返回 `1` |
| FR-5 | PostgreSQL 不可用时必须降级为纯 SQLite 模式 | PostgreSQL 连接失败时系统仍能启动，向量搜索回退到 SQLite 实现 |

### 3.2 读写分离

| 编号 | 需求 | 验收标准 |
|------|------|---------|
| FR-6 | SQLite 必须实现读写连接分离 | 写连接 MaxOpenConns=1，读连接 MaxOpenConns=2 |
| FR-7 | 事务中的读写操作必须使用同一连接 | 事务内所有操作走事务客户端，提交后恢复读写分离 |
| FR-8 | 原生 SQL 操作必须同样参与事务传播 | 事务内的原生 SQL 使用事务句柄，非事务使用读写分离句柄 |

### 3.3 事务管理

| 编号 | 需求 | 验收标准 |
|------|------|---------|
| FR-9 | 必须提供统一的事务入口 `ExecInTx` | 所有需要事务的操作通过 `ExecInTx` 执行 |
| FR-10 | 事务必须使用独立的 30 秒超时上下文 | HTTP 请求取消不影响事务执行，事务 30 秒后自动超时 |
| FR-11 | 必须支持嵌套事务检测 | 已在事务中调用 `ExecInTx` 时复用外层事务，不创建 savepoint |
| FR-12 | 事务 fn 执行成功但调用者已取消时必须回滚 | 调用者 context 已取消时，即使 fn 成功也回滚事务 |
| FR-13 | 必须提供 PostgreSQL 独立事务方法 | PostgreSQL 事务使用标准 `BeginTx` + `Commit`/`Rollback` |

### 3.4 迁移系统

| 编号 | 需求 | 验收标准 |
|------|------|---------|
| FR-14 | 必须提供三层迁移机制 | L1 Ent 自动迁移、L2 DDL 迁移注册表、L3 数据迁移 |
| FR-15 | DDL 迁移必须版本化管理 | 每个迁移有唯一版本号，已执行的迁移记录在 `schema_migrations` 表 |
| FR-16 | DDL 迁移必须幂等 | `duplicate column` / `already exists` 错误视为成功 |
| FR-17 | 数据迁移必须在 DDL 迁移后执行 | 启动时序：DDL 迁移 → Postgres Schema → 数据迁移 → 种子数据 |
| FR-18 | 迁移失败必须标记就绪门为 Failed | 迁移失败后 `ReadinessGate.IsReady()` 返回 false，健康检查失败 |

### 3.5 数据访问

| 编号 | 需求 | 验收标准 |
|------|------|---------|
| FR-19 | biz 层必须定义 Repo 接口，data 层实现 | `grep -r "ent\." internal/biz/` 为零 |
| FR-20 | Repo 接口必须按职责拆分（Reader/Writer） | 单接口方法数 ≤ 5，复合接口仅用于 Wire 绑定 |
| FR-21 | 所有数据库错误必须经过 `entErrToBizErr` 翻译 | NotFound→404, Constraint→409, NotLoaded→400, 其他→500 |
| FR-22 | 必须支持软删除 | 通过 `deleted_at` 字段，空字符串=未删除 |
| FR-23 | 必须支持分页查询 | 默认 page=1, size=20, 最大 100 |
| FR-24 | 必须支持 AIP-160 过滤和 AIP-132 排序 | 列表接口接受 `filter` 和 `order_by` 参数 |

### 3.6 特殊特性

| 编号 | 需求 | 验收标准 |
|------|------|---------|
| FR-25 | 必须支持 FTS5 全文搜索 | `messages_fts` 虚拟表 + 3 触发器自动同步 |
| FR-26 | FTS5 不可用时必须回退到 LIKE 查询 | FTS 表不存在时使用 `content_markdown LIKE ?` |
| FR-27 | 必须支持向量存储双实现 | SQLiteVectorStore（JSON+Go 余弦）+ PgVectorStore（pgvector） |
| FR-28 | 必须支持级联删除 | 删除 Agent/Session/Team/Channel 时自动清理关联表 |
| FR-29 | 必须提供启动就绪门控 | `ReadinessGate` 三态：Pending → Ready / Failed |

---

## 4. 非功能需求

### 4.1 性能

| 编号 | 需求 | 指标 |
|------|------|------|
| NFR-1 | SQLite 写连接单连接，避免 SQLITE_BUSY | MaxOpenConns=1, busy_timeout=30s |
| NFR-2 | SQLite 读连接并发，平衡性能和资源 | MaxOpenConns=2 |
| NFR-3 | PostgreSQL 写连接池支持中等并发 | MaxOpenConns=16 |
| NFR-4 | PostgreSQL 读连接池支持高并发检索 | MaxOpenConns=32 |
| NFR-5 | 关键查询路径必须有索引覆盖 | 通过 DDL 迁移补缺失索引 |
| NFR-6 | 消息全文搜索必须避免 LIKE 全表扫描 | 使用 FTS5 + bm25 排序 |

### 4.2 可靠性

| 编号 | 需求 | 指标 |
|------|------|------|
| NFR-7 | 事务必须保证原子性 | 全成功或全回滚，无部分成功 |
| NFR-8 | 迁移必须幂等可重试 | 重复执行不报错，失败可重试 |
| NFR-9 | PostgreSQL 降级不能阻断启动 | 连接失败时 `log.Warn` 后继续 |
| NFR-10 | 数据库错误必须保留原始错误链 | `apierror.Wrap` 保留 Cause 字段 |

### 4.3 可维护性

| 编号 | 需求 | 指标 |
|------|------|------|
| NFR-11 | Schema 变更必须通过 Ent Schema + DDL 迁移 | 禁止手动建表，禁止野生 `*_patch.go` |
| NFR-12 | Repo 接口必须标注稳定性等级 | Stable / Evolving / Internal |
| NFR-13 | 单 Repo 方法数 ≤ 5 | 超过按读写职责拆分 |
| NFR-14 | 单方法行数 ≤ 80，圈复杂度 ≤ 15 | linter 强制 |

### 4.4 安全

| 编号 | 需求 | 指标 |
|------|------|------|
| NFR-15 | 敏感字段必须标记 `.Sensitive()` | 凭证、API Key 等不泄漏到日志 |
| NFR-16 | 外键约束必须启用 | `PRAGMA foreign_keys=ON` |

---

## 5. 交互规格

### 5.1 启动流程

```
系统启动
  │
  ├── 1. 初始化 SQLite 写连接 + PRAGMA + Ent Client
  ├── 2. 初始化 SQLite 读连接 + PRAGMA + Ent Client
  ├── 3. 初始化 PostgreSQL 连接（可选，失败降级）
  ├── 4. 初始化 ReadinessGate（Pending 态）
  └── 5. 后台 goroutine (P1)
       ├── ensureSchemaDDL (DDL 迁移)
       ├── ensurePostgresSchemas (pgvector/knowledge)
       ├── runPendingDataMigrations (数据迁移)
       └── seedP1Data (种子数据)
            │
            ├── 全部成功 → MarkReady() → 接受流量
            └── 任一失败 → MarkFailed() → 健康检查失败
```

### 5.2 请求处理

```
HTTP 请求 → 中间件 → Service → Usecase → Repo
                                        │
                                        ├── 读操作 → RW().Read(ctx) → 读客户端
                                        └── 写操作 → RW().Write(ctx) → 写客户端
                                                              │
                                              事务? → 事务客户端
                                              非事务 → 写客户端
```

### 5.3 错误响应

| 数据库错误 | HTTP 状态码 | 业务语义 |
|-----------|------------|---------|
| NotFound / sql.ErrNoRows | 404 | 资源不存在 |
| ConstraintError / 唯一冲突 | 409 | 资源冲突 |
| NotLoaded | 400 | 请求参数错误 |
| 其他 | 500 | 内部错误 |

---

## 6. 验收标准

### 6.1 功能验收

- [x] SQLite 单机模式可启动，全部业务实体 CRUD 正常
- [x] PostgreSQL 可选模式可启动，向量搜索和知识库走 PostgreSQL
- [x] PostgreSQL 不可用时降级为纯 SQLite 模式
- [x] 事务原子性保证（全成功或全回滚）
- [x] 嵌套事务检测正常
- [x] HTTP 取消不中断事务
- [x] 三层迁移机制正常工作
- [x] ReadinessGate 三态门控正常
- [x] FTS5 全文搜索正常
- [x] 向量搜索双实现正常
- [x] 级联删除正常
- [x] 错误转换统一

### 6.2 红线合规

- [x] `grep -r "ent\." internal/biz/` 为零（biz 不直接依赖 ent）
- [x] `grep -r "\.Edge(" internal/data/ent/schema/` 为零（无 Ent Edge，仅 Eval 域例外）
- [x] DDL 迁移均注册到 `ddlMigrations`（无野生迁移）
- [x] 所有数据库错误经 `entErrToBizErr` 翻译
- [x] 事务均通过 `ExecInTx` 执行（PostgreSQL 除外）

### 6.3 性能验收

- [x] SQLite 读写分离正常
- [x] busy_timeout 生效，无 SQLITE_BUSY 错误
- [x] FTS5 搜索性能优于 LIKE
- [x] 关键查询路径有索引覆盖

---

## 7. 已知限制

| 编号 | 描述 | 影响 | 状态 |
|------|------|------|------|
| LIM-1 | SQLite 单写限制，写连接 MaxOpen=1 | 高并发写场景可能成为瓶颈 | 设计决策，SQLite 固有限制 |
| LIM-2 | 时间戳用 String 而非 Time | 查询排序需 CAST | 历史遗留，迁移成本高 |
| LIM-3 | Ent Schema 无 Edge（仅 Eval 域例外） | 级联删除在应用层实现 | 设计决策，灵活性优先 |
| LIM-4 | pgvector 旧版存储（`internal/data/pgvector/`）已废弃 | 代码冗余 | 待清理 |
| LIM-5 | 部分 Repo 方法过长（如 session_repo.go） | 可维护性 | 待重构 |
