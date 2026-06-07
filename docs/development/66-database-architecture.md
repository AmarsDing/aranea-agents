# Database Architecture — 需求文档

> **状态**：✅ 核心架构已落地（2026-06）
>
> **定位**：Aranea-Agents 数据库架构权威规范。精简参考，聚焦规则、约束与结构。

---

## 1. 双库架构

项目采用 SQLite + PostgreSQL 双库架构，职责分明：

```
┌───────────────────────────────────────────────────────────────┐
│                      应用层                                    │
│  biz.Usecase → biz.Repo 接口 → data.Repo 实现                │
└──────────┬────────────────────────────┬───────────────────────┘
           │                            │
           ▼                            ▼
┌─────────────────────┐    ┌─────────────────────────┐
│   SQLite (主库)      │    │  PostgreSQL (可选)       │
│   Ent ORM            │    │  原生 database/sql       │
│   业务 CRUD          │    │  pgvector 向量搜索       │
│   67 Schema          │    │  Knowledge 知识库        │
│   读写分离 (WAL)     │    │  MaxOpenConns=8          │
│   写=1 / 读=2        │    │                          │
└─────────────────────┘    └─────────────────────────┘
```

| 数据库 | 用途 | ORM/访问方式 | 连接池 |
|--------|------|-------------|--------|
| **SQLite** (主库) | Agent/Team/Session/Message/ToolInvocation 等全部业务实体 | Ent ORM + 原生 SQL | 写 MaxOpen=1, 读 MaxOpen=2 |
| **PostgreSQL** (可选) | 向量存储 (pgvector)、知识库 (Knowledge) | 原生 `database/sql` + `pgvector-go` | MaxOpen=8 |

---

## 2. 红线约束

| 编号 | 规则 | 验证方式 |
|------|------|----------|
| #DB-1 | 禁止在 `internal/biz/` 中直接导入 `ent` 包，Repo 接口必须定义在 biz 层 | `grep -r "ent\." internal/biz/` 应为零 |
| #DB-2 | 禁止在 `internal/data/` 外直接操作数据库，所有数据访问必须通过 Repo | 代码审查 |
| #DB-3 | 禁止 Ent Schema 定义 Edge，关系通过字符串外键字段手动维护 | `grep -r "\.Edge(" internal/data/ent/schema/` 应为零 |
| #DB-4 | 禁止手动创建 DDL 迁移而不注册到 `ddlMigrationRegistry` | 代码审查 |
| #DB-5 | 事务必须使用 `Data.ExecInTx()`，禁止直接操作 `sql.Tx`（PostgreSQL 除外） | `grep -r "sql.Tx" internal/data/` 应仅限 Postgres 方法 |

---

## 3. Ent Schema 规范

### 3.1 Schema 总表

共 67 个 Schema，位于 `internal/data/ent/schema/`。

| 域 | Schema | 表名 | 说明 |
|----|--------|------|------|
| **Agent** | Agent | agents | Agent 配置（20+ 字段，kind/source 枚举） |
| | AgentPerformance | agent_performances | Agent 性能评估 |
| | AgentPromptFile | agent_prompt_files | Agent Prompt 文件 |
| | AgentRuntimeSetting | agent_runtime_settings | Agent 运行时配置 |
| | AgentTemplate | agent_templates | Agent 模板 |
| **Team** | Team | teams | Team 编排（20+ 字段，topology/kind/source） |
| | CompiledTeam | compiled_teams | 编译后 Team |
| | TeamRun | team_runs | Team 运行记录 |
| | TeamRunStep | team_run_steps | Team 运行步骤 |
| **Session** | Session | sessions | 会话（40+ 字段，最重 Schema） |
| | SessionRun | session_runs | 会话运行 |
| | SessionRunCheckpoint | session_run_checkpoints | 运行检查点 |
| | SessionRuntime | session_runtime | 运行时状态 |
| | SessionMetrics | session_metrics | 会话指标 |
| | SessionParticipant | session_participants | 会话参与者 |
| | SessionTurn | session_turns | 会话轮次 |
| **Message** | Message | messages | 消息（16 字段，FTS5 全文搜索） |
| **Tool** | ToolInvocation | tool_invocations | 工具调用（24 字段） |
| | ToolInvocationAudit | tool_invocation_audits | 工具调用审计 |
| | ToolInvocationParam | tool_invocation_params | 工具调用参数 |
| | ToolResultBlob | tool_result_blobs | 工具结果 Blob |
| | ToolResultReplacement | tool_result_replacements | 工具结果替换 |
| | ToolAgentOverride | tool_agent_overrides | 工具 Agent 覆盖 |
| **Channel** | PlatformChannel | platform_channels | 平台渠道 |
| | PlatformChannelCredential | channel_credential | 渠道凭证 |
| | PlatformChannelDelivery | channel_delivery | 渠道投递 |
| | PlatformChannelPeerSession | channel_peer_session | 渠道对端会话 |
| | ChannelInboundReceipt | channel_inbound_receipt | 入站消息回执 |
| | ChannelRuntimeLease | channel_runtime_lease | 渠道运行时租约 |
| | ChannelTurnJob | channel_turn_job | 渠道轮次任务 |
| **Graph** | GraphDefinition | graph_definitions | Graph 定义 |
| | GraphExecution | graph_executions | Graph 执行 |
| | GraphTask | graph_tasks | Graph 任务 |
| | GraphTaskComment | graph_task_comments | 任务评论 |
| | GraphTaskEvent | graph_task_events | 任务事件 |
| | GraphTaskLink | graph_task_links | 任务链接 |
| | GraphTaskLog | graph_task_logs | 任务日志 |
| | GraphTaskRun | graph_task_runs | 任务运行 |
| **Cron** | CronTask | cron_tasks | 定时任务 |
| | CronTaskRun | cron_task_runs | 定时任务运行 |
| **Memory** | EventStore | event_store | 事件存储 |
| **Monitor** | FlowLogEvent | flow_log_events | 流日志事件 |
| | SelfCheckReport | self_check_reports | 自检报告 |
| **Usage** | ModelPricingRule | model_pricing_rules | 模型定价规则 |
| | ModelTokenUsageHourly | model_token_usage_hourly | Token 用量小时统计 |
| | UsageQuota | usage_quotas | 用量配额 |
| | BudgetAlert | budget_alerts | 预算告警 |
| **Plugin** | Plugin | plugins | 插件 |
| | Hook | hooks | Hook |
| | PlatformPlugin | platform_plugins | 平台插件 |
| | PlatformHook | platform_hooks | 平台 Hook |
| **Skill** | PlatformSkill | platform_skills | 平台技能 |
| | SkillInvocation | skill_invocation | 技能调用 |
| | SkillVersion | skill_versions | 技能版本 |
| | SkillEvolutionSuggestion | skill_evolution_suggestions | 技能演化建议 |
| **Ecosystem** | PlatformTool | platform_tools | 平台工具 |
| | PlatformMcpServer | platform_mcp_servers | MCP 服务器 |
| | PlatformMcpUserCredential | platform_mcp_user_credential | MCP 用户凭证 |
| | IndustryTaxonomy | industry_taxonomies | 行业分类 |
| | AvatarAsset | avatar_assets | Avatar 资源 |
| **System** | Admin | admins | 管理员 |
| | SystemSetting | system_settings | 系统设置 |
| | SchemaMigration | schema_migrations | 迁移版本记录 |
| | BackgroundJob | background_jobs | 后台任务 |
| | Orchestration | orchestrations | 编排 |
| | OrchestrationStep | orchestration_steps | 编排步骤 |
| | AllocationPlan | allocation_plans | 分配计划 |
| | TaskDeadLetter | task_dead_letters | 死信 |
| | TaskPlan | task_plans | 任务计划 |
| | GatewayWebhook | gateway_webhooks | Webhook |
| | HealRecord | heal_records | 自愈记录 |
| | EvolutionSuggestion | evolution_suggestions | 演化建议 |
| | ExperienceReport | experience_reports | 经验报告 |
| | FailurePattern | failure_patterns | 失败模式 |
| | UserEmbeddingSetting | user_embedding_settings | 用户 Embedding 设置 |

### 3.2 Schema 约定

| 约定 | 说明 |
|------|------|
| **无 Edge** | 所有关系通过字符串外键字段（如 `session_id`, `agent_id`）手动维护 |
| **表名注解** | 每个 Schema 通过 `entsql.Annotation{Table: "xxx"}` 显式映射 |
| **时间戳 String** | 大部分时间字段用 `field.String()` 存储 RFC3339 文本 |
| **软删除** | 通过 `deleted_at` 字段（空字符串 = 未删除） |
| **JSON 字段** | 大量使用 `field.Text("xxx_json").Default("{}")` 存储结构化数据 |
| **枚举值** | 使用 `field.Enum()` 定义（如 Agent.kind, Agent.source） |

---

## 4. 原生 SQL 表（非 Ent 管理）

### 4.1 记忆系统核心表

| 层级 | 表名 | 用途 |
|------|------|------|
| L0 | `session_summaries`, `memory_l0_assembly_snapshots` | 会话摘要和组装快照 |
| L1 | `memory_l1_tasks`, `memory_l1_fields`, `memory_l1_field_history`, `memory_l1_schemas` | 工作记忆 |
| L2 | `memory_episodes`, `memory_l2_index_meta`, `memory_event_marks` | 情景记忆 |
| L3 | `memory_facts`, `memory_fact_versions`, `memory_fact_feedback`, `memory_fact_conflicts`, `memory_fact_index` | 语义记忆 |
| L4 | `memory_entities`, `memory_relations`, `memory_entity_facts`, `memory_entity_versions` | 持久/演化记忆 |
| 审计 | `memory_action_log`, `memory_cascade_proposals` | 策略审计和级联提案 |
| 演化 | `agent_evolution_events`, `agent_evolution_proposals`, `agent_skill_stats` | Agent 演化 |

### 4.2 其他原生 SQL 表

| SQL 文件 | 表/功能 | 说明 |
|----------|---------|------|
| `message_fts.sql` | `messages_fts` | FTS5 全文搜索虚拟表 + 3 触发器 |
| `flow_log.sql` | flow_log 相关表 | 流日志持久化 |
| `plugin_run.sql` | plugin_run 相关表 | 插件运行记录 |
| `monitor_alert.sql` | monitor_alert 相关表 | 监控告警 |
| `learning_loop.sql` | learning_loop 相关表 | 学习循环 |
| `skill_evolution.sql` | skill_evolution 相关表 | 技能演化 |
| `plan.sql` | plan 相关表 | 计划管理 |
| `hook_delivery.sql` | hook_delivery 相关表 | Hook 投递 |
| `ecosystem_product.sql` | ecosystem_product 相关表 | 生态产品 |
| `memory_job_deadletter.sql` | 死信表 | 记忆任务死信 |
| `monitor_alert_firing_state.sql` | firing_state 表 | 告警触发状态 |

---

## 5. 迁移策略

### 5.1 三层迁移机制

| 层级 | 机制 | 管理对象 | 文件 |
|------|------|---------|------|
| 第一层 | Ent 自动迁移 | Ent Schema 定义的表 | `internal/data/data.go` |
| 第二层 | DDL 迁移注册表 | 原生 SQL 表 + 索引 + 列变更 | `internal/data/ddl_migration_registry.go` |
| 第三层 | 数据迁移 | 数据回填/转换 | `internal/data/schema_migrations.go` |

### 5.2 DDL 迁移注册表

```go
type ddlMigration struct {
    Version int
    Name    string
    SQL     string  // SQL 文件路径（可选）
    Func    func(...) error  // Go 函数（可选）
}
```

- 版本号式迁移，记录在 `schema_migrations` 表
- 幂等设计：`duplicate column` / `already exists` 错误视为成功
- 当前注册 30+ 个迁移，版本号从 20260601 到 20260719

### 5.3 数据迁移

| 迁移 | 版本号 | 说明 |
|------|--------|------|
| LegacyTRPCMemoryFacts | 20260524 | 从旧 trpc-agent-go 迁移 memory facts |
| TurnIndexToTurnID | 20260528 | turn_index 迁移到 turn_id |
| SessionStatusIdle | 20260531 | session status 从 active 改为 idle |

### 5.4 迁移执行时序

启动时在后台 goroutine (P1) 中按序执行：

```
1. ensureSchemaDDL (DDL 迁移)
2. ensurePostgresSchemas (pgvector/knowledge)
3. runPendingDataMigrations (数据迁移)
4. seedP1Data (种子数据)
```

所有步骤通过 `ReadinessGate` 管理：成功后 `MarkReady()`，失败 `MarkFailed()`，HTTP 健康检查可感知。

---

## 6. 事务处理

### 6.1 ExecInTx

```go
func (d *Data) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error
```

**设计要点**：

| 特性 | 说明 |
|------|------|
| 嵌套事务检测 | 通过 `txClientKey{}` context key，已在事务中则直接执行 fn |
| 分离上下文 | `context.WithTimeout(context.Background(), 30s)`，防止 HTTP 取消中断事务 |
| 30 秒硬超时 | 防止单个事务无限阻塞 SQLite 写连接 |
| 调用者取消检测 | fn 执行后检查原始 ctx 是否已取消，若是则回滚 |
| 事务传播 | 通过 `context.WithValue` 将事务注入上下文 |

### 6.2 PostgresExecInTx

```go
func (d *Data) PostgresExecInTx(ctx context.Context, fn func(ctx context.Context, tx *sql.Tx) error) error
```

独立的 PostgreSQL 事务方法，标准 `BeginTx` + `Rollback` + `Commit` 模式。

---

## 7. 读写分离

### 7.1 ReadWriteClient

```go
type ReadWriteClient struct {
    write *ent.Client  // 写客户端 (MaxOpenConns=1)
    read  *ent.Client  // 读客户端 (MaxOpenConns=2)
}
```

- 读操作：`d.RW().Read(ctx)` — 事务中返回事务客户端，否则返回读客户端
- 写操作：`d.RW().Write(ctx)` — 事务中返回事务客户端，否则返回写客户端

### 7.2 ReadWriteDB

同理操作 `*sql.DB` 句柄，确保原生 SQL 也参与事务。

### 7.3 SQLite PRAGMA

```sql
PRAGMA foreign_keys=ON;     -- 启用外键约束
PRAGMA journal_mode=WAL;    -- Write-Ahead Logging，允许并发读写
PRAGMA busy_timeout=30000;  -- 30 秒忙等待超时
PRAGMA synchronous=NORMAL;  -- 平衡性能和安全
```

---

## 8. Repository 模式

### 8.1 接口定义位置

Repo 接口分散定义在 `internal/biz/` 各子模块中，遵循 DDD 聚合边界。

### 8.2 主要 Repo 接口

| 接口名 | 定义位置 | 核心方法 |
|--------|----------|---------|
| `AgentRepository` | `biz/agent_usecase.go` | 组合 AgentReader + AgentWriter + AgentAtomicWriter + AgentRuntimeSettingsRepo + AgentPromptFileRepo + AgentReferenceChecker + ExecInTx |
| `TeamRepository` | `biz/team_usecase.go` | 组合 TeamReader + TeamWriter + TeamRunReader + TeamRunWriter + OrchestrationStepRepo + TaskDeadLetterRepo |
| `SessionRepo` | `biz/session/` | 会话 CRUD、消息 CRUD、搜索、统计 |
| `GraphRepo` | `biz/graph.go` | GraphReader + GraphWriter |
| `OrchestrationRepository` | `biz/task_orchestrator.go` | Create/GetByID/Update/List |
| `knowledge.Repo` | `biz/knowledge/knowledge.go` | CollectionRepo + DocumentRepo + ChunkRepo |
| `a2a.Repo` | `biz/a2a/a2a.go` | CardRepo + InvocationRepo + AuditRepo + RemoteAgentRepo |

### 8.3 接口组合模式

```go
type AgentRepository interface {
    AgentReader       // 只读方法
    AgentWriter       // 写入方法
    AgentAtomicWriter // 事务化原子写入
    AgentRuntimeSettingsRepo
    AgentPromptFileRepo
    AgentReferenceChecker
}
```

Usecase 只依赖需要的最小接口集，而非整个 Repository。

### 8.4 data 层实现

```go
func NewAgentRepo(d *Data, lg loggateway.Logger) biz.AgentRepository { ... }
func NewTeamRepo(d *Data, lg loggateway.Logger) biz.TeamRepository { ... }
func NewSessionRepo(d *Data, lg loggateway.Logger) biz.SessionRepo { ... }
```

---

## 9. 查询模式

### 9.1 分页

```go
func PageToLimitOffset(page, pageSize int32) (limit, offset int, pageOut, pageSizeOut int32)
// 默认: page>=1, size 默认 20, 最大 100
```

### 9.2 AIP 风格过滤/排序

```go
type ListOptions struct {
    Filter  filtering.Filter   // AIP-160 过滤表达式
    OrderBy ordering.OrderBy   // AIP-132 排序
    Offset  int
    Limit   int
}
```

使用 `go.einride.tech/aip/filtering` 和 `go.einride.tech/aip/ordering` 库。

### 9.3 搜索模式

| 模式 | 实现 | 适用场景 |
|------|------|---------|
| FTS5 全文搜索 | `messages_fts` 虚拟表 + `snippet()` + `bm25()` | 消息内容搜索 |
| LIKE 回退 | `content_markdown LIKE ?` | FTS 表不存在时 |
| 向量搜索 | pgvector `<=>` 或 SQLite Go 侧余弦 | 语义搜索 |
| BM25 稀疏搜索 | Knowledge `SearchChunksBM25` | 知识库搜索 |

### 9.4 软删除

几乎所有查询包含 `deleted_at = ""` 条件过滤。

---

## 10. 特殊数据库特性

### 10.1 FTS5 全文搜索

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
  message_id UNINDEXED,
  session_id UNINDEXED,
  content_markdown,
  tokenize = 'unicode61'
);
```

配套 3 个触发器自动同步 INSERT/DELETE/UPDATE。

### 10.2 向量存储双实现

| 实现 | 特点 |
|------|------|
| `SQLiteVectorStore` | embedding 存为 JSON TEXT，Go 侧余弦相似度（全表扫描） |
| `PgVectorStore` | pgvector 扩展，数据库侧余弦距离计算 |

初始化逻辑：优先 pgvector（需特性开关 + PostgreSQL 可用），失败回退 SQLite。

### 10.3 级联删除

应用层实现（因无 Ent Edge / 数据库外键）：

- `cascadeDeleteByAgent` — 删除 runtime_settings, prompt_files, 软删 sessions, 硬删 tool_overrides
- `cascadeDeleteBySession` — 事务内硬删 14 个关联表
- `cascadeDeleteByTeam` — 硬删 run_steps, runs, compiled_teams
- `cascadeDeleteByChannel` — 硬删 peer_session, credential, delivery, inbound_receipt, turn_job, runtime_lease

### 10.4 错误转换

```go
func entErrToBizErr(err error, domain, msg string) error
// NotFound → 404, ConstraintError → 409, NotLoaded → 400, default → 500
```

### 10.5 ReadinessGate

三态门控（Pending → Ready / Failed），确保数据库迁移完成前不接收请求。

---

## 11. 配置规格

### 11.1 Proto 定义

```protobuf
message Data {
  message Database { string driver = 1; string source = 2; }
  message Sqlite { bool enable = 1; string source = 2; }
  message Postgres { string source = 1; int32 vector_dim = 2; }
  message InitialAdmin { string name = 1; string email = 2; string password = 3; string access = 4; }
  Database database = 1;
  Sqlite sqlite = 3;
  Postgres postgres = 4;
  InitialAdmin initial_admin = 5;
}
```

---

## 12. Data 层结构

`internal/data/` 包含约 150+ 个 Go 文件：

| 类别 | 文件 | 说明 |
|------|------|------|
| 核心框架 | `data.go`, `tx.go`, `readwrite.go`, `readwrite_db.go`, `errors.go`, `readiness.go` | 初始化、事务、读写分离、错误转换、就绪门控 |
| 迁移 | `ddl_migration_registry.go`, `schema_migrations.go` | DDL 迁移 + 数据迁移 |
| 级联删除 | `cascade_delete.go` | 应用层级联删除 |
| Agent Repo | `agent_repo.go`, `agent_runtime_patch.go`, `agent_list_extras.go`, `agent_performance_repo.go`, `agent_template_repo.go` | Agent 领域数据访问 |
| Session Repo | `session_repo.go`, `session_state_repo.go`, `session_runtime_repo.go`, `session_metrics_repo.go`, `session_message_repo.go`, `session_turn_repo.go`, `session_participant_repo.go`, `session_run_repo.go` 等 | Session 领域（最重） |
| Team Repo | `team_repo.go`, `compiled_team_repo.go`, `team_graph_session_repo.go` | Team 领域 |
| Channel Repo | `channel.go`, `channel_peer_session.go`, `channel_inbound_receipt.go`, `channel_turn_job.go`, `channel_runtime_lease.go` | Channel 领域 |
| Memory Repo | `memory.go`, `memory_shim_l0.go` ~ `memory_shim_l4.go`, `memory_composite_adapter.go` 等 | 记忆系统（L0-L4） |
| 其他 | `tool.go`, `skill.go`, `usage.go`, `monitor.go`, `graph.go`, `knowledge.go`, `a2a.go` 等 | 各领域 Repo |

---

## 13. 关键文件索引

| 文件 | 作用 |
|------|------|
| `internal/data/data.go` | Data 结构体、NewData 初始化、Wire ProviderSet |
| `internal/data/tx.go` | 事务管理（ExecInTx、PostgresExecInTx） |
| `internal/data/readwrite.go` | ReadWriteClient（Ent 读写分离） |
| `internal/data/readwrite_db.go` | ReadWriteDB（原生 SQL 读写分离） |
| `internal/data/errors.go` | entErrToBizErr 错误转换 |
| `internal/data/readiness.go` | ReadinessGate 启动就绪门控 |
| `internal/data/ddl_migration_registry.go` | DDL 迁移注册表 |
| `internal/data/schema_migrations.go` | 数据迁移门控 |
| `internal/data/cascade_delete.go` | 级联删除逻辑 |
| `internal/data/lazy_seeder.go` | 延迟种子数据 |
| `internal/data/ent/schema/*.go` | 67 个 Ent Schema 定义 |
| `internal/data/sql/*.sql` | 原生 DDL SQL 文件 |
| `internal/data/vector/store.go` | VectorStore 接口定义 |
| `internal/data/vector/sqlite.go` | SQLite 向量存储实现 |
| `internal/data/vector/pgvector.go` | PgVector 向量存储实现 |
| `internal/conf/conf.proto` | Data 配置 Proto 定义 |

---

## 14. 已知偏差

| 编号 | 严重性 | 描述 | 状态 |
|------|--------|------|------|
| DB-1 | 黄 | Ent Schema 无 Edge，级联删除在应用层实现，无数据库外键约束 | 设计决策，暂不改变 |
| DB-2 | 黄 | 时间戳用 String 而非 Time，查询排序需 CAST | 历史遗留，迁移成本高 |
| DB-3 | 黄 | pgvector 旧版存储（`internal/data/pgvector/`）已废弃但仍保留 | 待清理 |
| DB-4 | 黄 | SQLite 写连接池 MaxOpen=1，高并发写场景可能成为瓶颈 | 设计决策，SQLite 单写限制 |
| DB-5 | 低 | 部分 Repo 方法过长（如 session_repo.go），职责可进一步拆分 | 待重构 |
