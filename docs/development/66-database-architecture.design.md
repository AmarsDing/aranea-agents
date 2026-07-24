# Database Architecture — 设计文档

> **对应需求**：[66-database-architecture.md](./66-database-architecture.md)
>
> **范围**：数据库架构的分层设计、连接管理、事务机制、迁移系统、Repo 模式、特殊特性实现。开发进度见 [66-database-architecture.development.md](./66-database-architecture.development.md)。

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
│  PostgreSQL (主库)   │    │  PostgreSQL (测试)       │
│  Ent ORM + pgvector  │    │  schema-per-test 隔离    │
│  读写分离            │    │  testhelper.SetupTestPG  │
└─────────────────────┘    └─────────────────────────┘

SQLite 已完全退出主代码路径，仅存于离线工具
（cmd/migrate-sqlite-to-postgres、cmd/session-consistency-check）。
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

### 2.1 测试连接（schema-per-test 隔离）

测试基础设施不再使用 in-memory SQLite，而是为每个测试创建独立的
PostgreSQL schema（`internal/data/testhelper/pg.go`）：

```
SetupTestPG(t)                     SetupTestPGRaw(t)
  │                                  │
  ├── 创建 schema test_<rand8>        ├── 创建 schema test_<rand8>
  ├── search_path 指向该 schema       ├── search_path 指向该 schema
  ├── Ent Schema.Create 自动迁移      ├── 不建表（由测试自行 DDL，
  ├── 返回 (entClient, *sql.DB)       │   镜像生产迁移文件，TEXT/BYTEA
  └── t.Cleanup: DROP SCHEMA CASCADE  │   等与 Ent 生成物不同）
                                      └── 返回 *sql.DB
                                          t.Cleanup: DROP SCHEMA CASCADE
```

**设计决策**：

| 决策 | 原因 |
|------|------|
| schema-per-test 而非共享库 | 并行测试完全隔离，无数据互相污染 |
| search_path 绑定 | 测试 SQL 无需带 schema 前缀，与生产 SQL 一致 |
| `current_schema()` 元数据查询 | Dialect 的 TableExists/ColumnExists/IndexExists 查询用 `current_schema()` 而非硬编码 `public`，保证隔离 schema 下检查正确 |
| DROP SCHEMA CASCADE 清理 | 测试结束后不留残留，数据库长期运行不膨胀 |
| SetupTestPGRaw 不做 Ent 迁移 | raw-SQL Repo 的生产 DDL（TEXT 时间戳、BYTEA 负载）与 Ent 生成物不同，由测试镜像迁移文件自行建表 |
| DSN 来自 `ARANEA_TEST_PG_DSN` | 无本地 PG 时跳过或指向远程实例 |

**已移除的 SQLite 测试路径**：`testhelper.SetupTestDB`（in-memory SQLite）、`OpenSQLiteEntClient`、`NewCLIData`、`SQLiteVectorStore`、messages FTS5 搜索均已删除。

### 2.2 PostgreSQL 双连接池

```
pg (*sql.DB) - 写池          pgRead (*sql.DB) - 读池
MaxOpenConns=16              MaxOpenConns=32
MaxIdleConns=4               MaxIdleConns=8
ConnMaxLifetime=30min        ConnMaxLifetime=30min
ConnMaxIdleTime=5min         ConnMaxIdleTime=5min
     │                            │
     ├── Knowledge / pgvector schema
     ├── Session run checkpoints
     └── DDL migrations
```

**当前策略**：PostgreSQL 是生产唯一主库，启动时强制初始化（`data.go`：`Postgres is the only supported primary database`）。测试基础设施使用 schema-per-test PostgreSQL 隔离（见 §2.1），SQLite 仅存于离线迁移工具。生产环境不允许降级为 SQLite 模式。

> **D-06 注**：历史「WAL/EventStore/Checkpoint 写入」路径中的 EventWAL / EventStore 已删除（`20260901_drop_event_store_subsystem.sql`）。Checkpoint 仍由 session_run_checkpoints 承担。

### 2.3 连接初始化流程

```
NewData(bc, lg)
  ├── PostgreSQL 写池 + 读池 → Ping → Ent Client → Schema.Create (自动迁移)
  ├── pgvector.EnsureSchema() + EnsureKnowledgeSchema()
  ├── ReadinessGate 初始化（Pending 态）
  └── 后台 goroutine (P1)
       ├── ensureSchemaDDL (DDL 迁移)
       ├── ensurePostgresSchemas
       ├── runPendingDataMigrations (数据迁移)
       └── seedP1Data (种子数据)
            │
            ├── 全部成功 → MarkReady()
            └── 任一失败 → MarkFailed()
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
  ├── 注入事务到上下文（双 key）
  │   txCtx = context.WithValue(detached, txClientKey{}, tx)
  │   txCtx = context.WithValue(txCtx, rawTxKey{}, tx)
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
| 嵌套事务检测 | 已在事务中则直接复用外层事务，不创建 savepoint（与 SQLite 时代行为一致） |
| 调用者取消检测 | fn 执行成功但调用者已放弃，应回滚而非提交 |
| 30s 硬超时 | 防止长事务无限占用写连接池（可通过 SetTxTimeout(0) 关闭，仅限长迁移） |
| 双 key 注入 | `txClientKey{}`（Ent 客户端）+ `rawTxKey{}`（Raw SQL execer），确保 Ent 和 Raw SQL 在同一事务 |

### 3.2 ExecInTxWithRetry 重试包装（T2.1）

`ExecInTxWithRetry` 包装 `ExecInTx`，对瞬态 DB 错误自动重试，实现 No-Timeout 原则：

```
ExecInTxWithRetry(ctx, fn)
  │
  ├── for attempt := 0..3
  │   ├── 检查 ctx.Err() → 已取消则返回
  │   ├── ExecInTx(ctx, fn)
  │   │   └── 成功 → 返回 nil
  │   ├── isRetryableDBError(err)?
  │   │   └── 不可重试 → 返回 err
  │   ├── 指数退避等待 (1s/2s/4s)
  │   │   └── select { ctx.Done → 返回; time.After → 继续 }
  │
  └── 返回 lastErr
```

**可重试错误分类**：

| 错误类型 | 可重试 | 原因 |
|---------|--------|------|
| `apierror.CodeInternal` | ✅ | 未知 DB 错误，可能是瞬态 |
| Postgres `deadlock_detected` (40P01) | ✅ | 死锁，重试通常成功 |
| Postgres `serialization_failure` (40001) | ✅ | 序列化冲突，重试通常成功 |
| `context.DeadlineExceeded` | ✅ | tx 超时（非 caller 取消），瞬态 |
| `context.Canceled` | ❌ | caller 主动取消，不应重试 |
| `apierror.CodeConflict` | ❌ | 业务冲突（唯一键等），重试无意义 |
| `apierror.CodeBadRequest` | ❌ | 输入错误，重试无意义 |
| `apierror.CodeNotFound` | ❌ | 数据不存在，重试无意义 |

**幂等性要求**：`fn` 回调必须是幂等的——重试可能多次执行 fn，副作用（事件发布、WS 推送）必须延迟到事务提交后。

### 3.3 事务传播机制

```go
// data 层
func (d *Data) RW() *ReadWriteClient
func (d *Data) RWDB() *ReadWriteDB

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

**原生 SQL 同理**：`ReadWriteDB.ReadDB(ctx)` / `ReadWriteDB.WriteDB(ctx)` 通过 `rawTxKey{}` 确保原生 SQL 也参与事务。

**事务感知辅助函数**：

```go
func EntClientFromCtx(ctx context.Context, fallback *ent.Client) *ent.Client
func TxExecerFromCtx(ctx context.Context, fallback *sql.DB) execer
```

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

### 3.4 PostgreSQL 事务

```go
func (d *Data) PostgresExecInTx(ctx context.Context, fn func(ctx context.Context, tx *sql.Tx) error) error
```

独立的 PostgreSQL 事务方法，标准 `BeginTx` + `Rollback` + `Commit` 模式。

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
    SQL     string    // SQL 文件路径 (可选，相对项目根)
    Func    func(ctx, db, entClient, lg) error  // Go 函数 (可选)
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

**当前迁移清单**（共 54 个，版本号 20260601 ~ 20260724）：

| 版本号 | 名称 | 说明 |
|--------|------|------|
| 20260601 | session_memory_patches | Session 记忆补丁 |
| 20260602 | memory_facts_index_status | 记忆索引状态（已合入 memory_chain.sql） |
| 20260603 | messages_turn_number | 消息轮次编号 |
| 20260604 | session_memory_schema | Session 记忆 Schema |
| 20260605 | memory_relation_patches | 记忆关系补丁（已合入 memory_chain.sql） |
| 20260606 | monitor_schema_patches | 监控 Schema 补丁 |
| 20260607 | agent_runtime_patches | Agent 运行时补丁 |
| 20260608 | entity_reinforcements_schema | 实体强化 Schema |
| 20260609 | cascade_saga_patches | 级联 Saga 补丁 |
| 20260610 | builtin_platform_tools | 内置平台工具 |
| 20260611 | system_setting_patches | 系统设置补丁 |
| 20260612 | pricing_rule_patches | 定价规则补丁 |
| 20260613 | llm_provider_model_capability | LLM 模型能力 |
| 20260614 | default_system_setting | 默认系统设置 |
| 20260615 | credential_encryption_key | 凭证加密密钥 |
| 20260616 | eval_schema | 评测 Schema |
| 20260617 | a2a_schema | A2A Schema |
| 20260618 | a2a_remote_health_patches | A2A 远程健康补丁 |
| 20260619 | team_run_summary_patches | Team 运行摘要补丁 |
| 20260620 | session_revision_patches | Session 版本补丁 |
| 20260621 | plugin_run_schema | 插件运行 Schema |
| 20260622 | hook_delivery_schema | Hook 投递 Schema |
| 20260623 | flow_log_schema | 流日志 Schema |
| 20260624 | message_fts_schema | FTS5 全文搜索 |
| 20260625 | channel_inbound_schema | 渠道入站 Schema |
| 20260626 | channel_turn_job_schema | 渠道轮次任务 Schema |
| 20260627 | channel_runtime_lease_schema | 渠道运行时租约 Schema |
| 20260628 | session_run_schema | Session 运行 Schema |
| 20260629 | session_participant_schema | Session 参与者 Schema |
| 20260630 | session_run_checkpoint_schema | Session 运行检查点 Schema |
| 20260701 | session_run_column_patches | Session 运行列补丁（已合入 session_run_schema） |
| 20260702 | monitor_alert_schema | 监控告警 Schema |
| 20260703 | ecosystem_schema | 生态 Schema |
| 20260704 | team_graph_session_schema | Team Graph Session Schema |
| 20260705 | compiled_team_schema | 编译 Team Schema |
| 20260706 | skill_evolution_schema | 技能演化 Schema |
| 20260707 | memory_facts_extra_patches | 记忆事实额外补丁 |
| 20260708 | session_table_split | Session 表拆分 |
| 20260709 | vector_embedding_ref | 向量 Embedding 引用 |
| 20260710 | task_plan_schema | 任务计划 Schema |
| 20260711 | allocation_plan_schema | 分配计划 Schema |
| 20260712 | agent_performance_schema | Agent 性能 Schema |
| 20260713 | orchestration_schema | 编排 Schema |
| 20260714 | compiled_team_session_id | 编译 Team Session ID（已合入 compiled_team_schema） |
| 20260715 | self_check_report_schema | 自检报告 Schema |
| 20260716 | missing_indexes | 补缺失索引 |
| 20260717 | usage_events_schema | 用量事件 Schema |
| 20260718 | ecosystem_preset_schema | 生态预设 Schema |
| 20260719 | agent_source_column | Agent source 列 |
| 20260720 | unified_evolution_schema | 统一演化 Schema |
| 20260721 | evolution_suggestion_pre_apply_snapshot | 演化建议预应用快照 |
| 20260722 | activity_schema | 活动 Schema |
| 20260723 | activity_token_columns | 活动 Token 列 |
| 20260724 | invariant_constraints | 不变量约束 |

### 4.3 数据迁移设计

独立于 DDL 迁移，处理数据回填场景。当前共 4 个数据迁移：

| 迁移 | 版本号 | 说明 |
|------|--------|------|
| LegacyTRPCMemoryFacts | 20260524 | 从旧 trpc-agent-go 框架迁移 memory facts |
| TurnIndexToTurnID | 20260528 | turn_index 迁移到 turn_id |
| SessionStatusIdle | 20260531 | session status 从 active 改为 idle |
| OrganizationRedesign | （内联） | 组织架构重设计数据迁移 |

**执行时序**：DDL 迁移 → Postgres Schema → 数据迁移 → 种子数据

---

## 5. Ent Schema 设计

### 5.1 Schema 总表

共 91 个 Ent Schema，位于 `internal/data/ent/schema/`。其中 88 个使用 `entsql.Annotation{Table: ...}` 显式映射表名，3 个使用 Ent 默认复数化规则。

> **已删除（勿再实现）**：`Message`（messages）与 `EventStore`（event_store）已于 2026-09 删除（`20260901_drop_event_store_subsystem.sql`、`20260902_drop_messages_subsystem.sql`）。下表**不**再列出二者。WS replay 已移除；历史事件用 `ListActivities` RPC。V2 Activity 相关 Schema 见下表「Activity V2」行。

| 域 | Schema | 表名 | 说明 |
|----|--------|------|------|
| **Agent** | Agent | agents | Agent 配置（kind/source 枚举） |
| | AgentPerformance | agent_performances | Agent 性能评估 |
| | AgentPromptFile | agent_prompt_files | Agent Prompt 文件 |
| | AgentRuntimeSetting | agent_runtime_settings | Agent 运行时配置（约 140 字段，TECH-DEBT） |
| | AgentTemplate | agent_templates | Agent 模板 |
| **Team** | Team | teams | Team 编排（topology/kind/source） |
| | CompiledTeam | compiled_teams | 编译后 Team |
| | TeamRun | team_runs | Team 运行记录 |
| | TeamRunStep | team_run_steps | Team 运行步骤 |
| **Session** | Session | sessions | 会话（核心 Schema） |
| | SessionRun | session_runs | 会话运行 |
| | SessionRunCheckpoint | session_run_checkpoints | 运行检查点 |
| | SessionRuntime | session_runtime | 运行时状态 |
| | SessionMetrics | session_metrics | 会话指标 |
| | SessionParticipant | session_participants | 会话参与者 |
| | SessionTurn | session_turns | 会话轮次 |
| **Tool** | ToolInvocation | tool_invocations | 工具调用 |
| | ToolInvocationAudit | tool_invocation_audit | 工具调用审计 |
| | ToolInvocationParam | tool_invocation_params | 工具调用参数 |
| | ToolResultBlob | tool_result_blobs | 工具结果 Blob |
| | ToolResultReplacement | tool_result_replacements | 工具结果替换 |
| | ToolAgentOverride | tool_agent_overrides | 工具 Agent 覆盖 |
| **Channel** | PlatformChannel | channel | 平台渠道 |
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
| **Cron** | CronTask | cron_task | 定时任务 |
| | CronTaskRun | cron_task_run | 定时任务运行 |
| **Activity V2** | SessionV2 | sessions_v2 | V2 会话（替代原 sessions 子集） |
| | TurnV2 | turns_v2 | V2 轮次 |
| | TaskV2 | tasks_v2 | V2 任务 |
| | StepV2 | steps_v2 | V2 步骤 |
| | PlanBoardV2 | plan_boards_v2 | V2 计划看板 |
| | PlanStepV2 | plan_steps_v2 | V2 计划步骤 |
| | TeamRunV2 | team_runs_v2 | V2 Team 运行 |
| | TeamStageV2 | team_stages_v2 | V2 Team 阶段 |
| | MemberSessionV2 | member_sessions_v2 | V2 成员会话 |
| | GraphStageV2 | graph_stages_v2 | V2 Graph 阶段 |
| | GraphNodeV2 | graph_nodes_v2 | V2 Graph 节点 |
| **Monitor** | FlowLogEvent | flow_log_events | 流日志事件 |
| | SelfCheckReport | self_check_reports | 自检报告 |
| **Usage** | ModelPricingRule | model_pricing_rules | 模型定价规则 |
| | ModelTokenUsageHourly | model_token_usage_hourly | Token 用量小时统计 |
| | UsageQuota | usage_quotas | 用量配额 |
| | BudgetAlert | budget_alerts | 预算告警 |
| **Plugin** | Plugin | plugins | 插件 |
| | Hook | hooks | Hook |
| **Skill** | PlatformSkill | skill | 平台技能 |
| | SkillImportJob | skill_import_jobs | 技能导入任务 |
| | SkillInvocation | skill_invocation | 技能调用 |
| | SkillVersion | skill_version | 技能版本 |
| | SkillEvolutionSuggestion | skill_evolution_suggestions | 技能演化建议 |
| **Ecosystem** | PlatformTool | tools | 平台工具 |
| | PlatformMcpServer | mcp_server | MCP 服务器 |
| | PlatformMcpUserCredential | mcp_server_user_credential | MCP 用户凭证 |
| | AvatarAsset | avatar_assets | Avatar 资源 |
| **Eval** | EvalDataset | eval_datasets | 评测数据集 |
| | EvalCase | eval_cases | 评测用例 |
| | EvalCaseResult | eval_case_results | 评测用例结果 |
| | EvalRun | eval_runs | 评测运行 |
| **System** | Admin | admins（默认复数化） | 管理员 |
| | SystemSetting | system_settings（默认复数化） | 系统设置 |
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
| | FailurePattern | failure_pattern | 失败模式 |
| | UserEmbeddingSetting | user_embedding_settings（默认复数化） | 用户 Embedding 设置 |
| | LlmProviderModel | llm_provider_models | LLM Provider 模型 |
| | Organization | organizations | 组织 |
| | BorrowRequest | borrow_requests | 借用请求 |
| | Activity | activities | 活动 |
| | CircuitBreakerState | circuit_breaker_states | 熔断器状态 |

### 5.2 Schema 约定

| 约定 | 说明 |
|------|------|
| **无 Edge** | 所有关系通过字符串外键字段（如 `session_id`, `agent_id`）手动维护（仅 Eval 域使用 4 个 Schema 8 条边） |
| **表名注解** | 79 个 Schema 通过 `entsql.Annotation{Table: "xxx"}` 显式映射，3 个使用默认复数化 |
| **时间戳 String** | 大部分时间字段用 `field.String()` 存储 RFC3339 文本 |
| **软删除** | 通过 `deleted_at` 字段（空字符串 = 未删除） |
| **JSON 字段** | 大量使用 `field.Text("xxx_json").Default("{}")` 或 `field.JSON()` 存储结构化数据 |
| **枚举值** | 使用 `field.Enum()` 定义（如 Agent.kind, Agent.source） |
| **敏感字段** | 凭证、API Key 等标记 `.Sensitive()` 防止日志泄漏 |

---

## 6. 原生 SQL 表设计（非 Ent 管理）

### 6.1 记忆系统核心表（memory_chain.sql）

| 层级 | 表名 | 用途 |
|------|------|------|
| L0 | `session_summaries`, `memory_l0_assembly_snapshots`, `memory_items` | 会话摘要、组装快照、记忆项 |
| L1 | `memory_l1_tasks`, `memory_l1_fields`, `memory_l1_field_history`, `memory_l1_schemas` | 工作记忆 |
| L2 | `memory_episodes`, `memory_event_marks` | 情景记忆（`memory_l2_index_meta` 已在 20260620 迁移中删除） |
| L3 | `memory_facts`, `memory_fact_versions`, `memory_fact_feedback`, `memory_fact_conflicts`, `memory_fact_index` | 语义记忆 |
| L4 | `memory_entities`, `memory_relations`, `memory_entity_facts`, `memory_entity_versions` | 持久/演化记忆 |
| 演化 | `agent_identity`, `agent_strategy_profile`, `agent_evolution_events`, `agent_evolution_proposals`, `agent_skill_stats` | Agent 演化 |
| 审计 | `memory_action_log`, `memory_cascade_proposals` | 策略审计和级联提案 |

### 6.2 其他原生 SQL 表

| SQL 文件 / 迁移 | 表名 | 用途 |
|----------------|------|------|
| ~~`message_fts.sql`~~ | ~~`messages_fts`~~ | ⚠️ 已随 messages 子系统删除（20260902）；FTS 迁移入口保留为空操作 |
| `flow_log.sql` | `flow_log_events` | 流日志持久化 |
| `plugin_run.sql` | `plugin_runs` | 插件运行记录 |
| `monitor_alert.sql` | `monitor_alert_rules` | 监控告警规则 |
| `monitor_alert_firing_state.sql` | （列补丁） | 告警触发状态列（ALTER TABLE，非新表） |
| `learning_loop.sql` | `learning_observations`, `learning_patterns`, `learning_proposals` | 学习循环 |
| `skill_evolution.sql` | `skill_proposals` | 技能演化提案 |
| `plan.sql` | `plans` | 计划管理 |
| `hook_delivery.sql` | `hook_deliveries` | Hook 投递 |
| `ecosystem_product.sql` | `ecosystem_products`, `ecosystem_installs` | 生态产品 |
| `memory_job_deadletter.sql` | `memory_job_deadletter` | 记忆任务死信 |
| `unified_evolution.sql` | `unified_evolution_suggestions` | 统一演化建议 |
| 迁移 20260608 | `entity_reinforcements` | 实体强化 |
| 迁移 20260609 | `cascade_saga_steps` | 级联 Saga 步骤 |
| 迁移 20260617 | ~~`event_wal`~~, ~~`event_store`~~, `session_run_checkpoints` | Phase 1；WAL/EventStore 表已于 20260901 删除，Checkpoint 保留 |
| 迁移 20260708 | `session_metrics`, `session_runtime` | Session 表拆分 |
| 迁移 20260715 | `self_check_reports` | 自检报告 |
| 迁移 20260717 | `model_token_usage_events`, `model_token_usage_daily` | 用量事件和日统计 |
| 迁移 20260722 | `activities` | 活动 |

---

## 7. Repository 设计

### 7.1 接口组合模式

```
AgentRepository (组合接口，定义于 biz/agent_usecase.go)
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
- 接口标注稳定性等级（`// Stability:stable` / `evolving` / `internal`）

### 7.2 主要 Repo 接口

| 接口名 | 定义位置 | 组成 |
|--------|----------|------|
| `AgentRepository` | `biz/agent_usecase.go` | AgentReader + AgentWriter + AgentAtomicWriter + AgentRuntimeSettingsRepo + AgentPromptFileRepo + AgentReferenceChecker + ExecInTx |
| `TeamRunRepo` | `biz/team_usecase.go` | TeamRunReader + TeamRunWriter（TeamReader/TeamWriter 独立使用，不组合为 TeamRepository） |
| `SessionRepo` | `biz/session/usecase.go` | 会话 CRUD、消息 CRUD、搜索、统计（方法数 40+，TECH-DEBT） |
| `GraphRepo` | `biz/graph.go` | GraphReader + GraphWriter |
| `OrchestrationRepository` | `biz/task_orchestrator.go` | Create/GetByID/Update/List |
| `knowledge.Repo` | `biz/knowledge/knowledge.go` | CollectionRepo + DocumentRepo + ChunkRepo |
| `a2a.Repo` | `biz/a2a/a2a.go` | CardRepo + InvocationRepo + AuditRepo + RemoteAgentRepo |
| `SpiritTransactor` | `biz/spirit_team_usecase.go` | ExecInTx（事务抽象） |

### 7.3 查询构建模式

```
Repo 方法 → ReadWriteClient.Read/Write(ctx)
  → Ent Client 查询构建器
    ├── Where 条件 (软删除过滤 + 业务条件)
    ├── Order 排序
    ├── Offset/Limit 分页
    └── Select 字段选择
```

**软删除过滤**：几乎所有查询包含 `deleted_at = ""` 条件。

### 7.4 原生 SQL 查询

当 Ent 查询构建器无法满足时（如 FTS、聚合统计、跨表 JOIN），使用原生 SQL（例如 Activity / knowledge / usage 报表查询）。历史 `SearchMessages` / `Message` 路径已删除，勿再引用。
### 7.5 查询辅助

| 辅助 | 位置 | 说明 |
|------|------|------|
| `PageToLimitOffset` | `biz/shared/shared.go` | 分页标准化：默认 page=1, size=20, 最大 100 |
| `ListOptions` | `biz/shared/shared.go` | Filter (AIP-160) + OrderBy (AIP-132) + Offset + Limit |
| `EntClientFromCtx` | `data/tx.go` | 从 ctx 获取事务客户端或 fallback |
| `TxExecerFromCtx` | `data/tx.go` | 从 ctx 获取事务 execer 或 fallback |

---

## 8. 级联删除设计

### 8.1 为什么不用数据库外键

| 原因 | 说明 |
|------|------|
| Ent Schema 无 Edge | 项目设计决策，关系通过字符串外键维护（仅 Eval 域例外） |
| 灵活性 | 应用层级联可控制删除顺序和策略（软删/硬删混合） |
| 历史 SQLite 限制 | 早期外键约束在 WAL 模式下有性能影响（现 PG 已启用 FK，应用层级联仍保留以控制删除顺序） |

### 8.2 级联删除实现

```
cascadeDeleteByAgent(ctx, agentID)
  ├── 硬删 agent_runtime_settings
  ├── 硬删 agent_prompt_files
  ├── 软删 sessions (设置 deleted_at)
  └── 硬删 tool_agent_overrides

cascadeDeleteBySession(ctx, sessionID)  // 事务内，14 个 DELETE
  ├── 硬删 session_turns
  ├── 硬删 session_participants
  ├── 硬删 session_run_checkpoints
  ├── 硬删 tool_invocation_params (通过子查询)
  ├── 硬删 tool_invocations
  ├── 硬删 skill_invocation
  ├── 硬删 tool_result_replacements
  ├── 硬删 tool_result_blobs
  ├── （messages / event_store 已随子系统删除，不再 cascade）
  ├── 硬删 session_runs
  ├── 硬删 session_runtime
  ├── 硬删 session_metrics
  └── 硬删 channel_turn_job

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

## 9. 向量存储设计

### 9.1 VectorStore 接口

```go
type VectorStore interface {
    Upsert(ctx context.Context, id string, embedding []float64, meta map[string]string) error
    Search(ctx context.Context, embedding []float64, topK int, minScore float64) ([]VectorHit, error)
    Delete(ctx context.Context, id string) error
}
```

### 9.2 唯一实现：PgVectorStore

```
┌──────────────────────────────────────────────────────┐
│                  VectorStore 接口                     │
└──────────┬───────────────────────────────────────────┘
           │
           ▼
┌─────────────────────────┐
│ PgVectorStore           │
│ embedding → vector(1536) │
│ DB 侧余弦距离            │
│ 索引加速                 │
│ 生产 + 测试              │
└─────────────────────────┘
```

> SQLiteVectorStore（Go 侧余弦、全表扫描）已随 SQLite 弃用删除
> （原 `internal/data/vector/sqlite.go`）。

### 9.3 记忆系统 embedding_blob

记忆系统（L0-L4）的向量存储不走 VectorStore 接口，而是在原生 SQL 表中使用 `embedding_blob BLOB` 字段：

- `memory_episodes.embedding_blob`
- `memory_facts.embedding_blob`
- `memory_entities.embedding_blob`

配合 `embedding_norm` 字段做余弦相似度计算。

### 9.4 旧版 pgvector 存储（已废弃）

`internal/data/pgvector/` 目录下存在旧版 PgVectorStore 实现，已被 `internal/data/vector/pgvector.go` 替代，但代码仍保留，待清理。

---

## 10. FTS5 全文搜索设计

> ⚠️ **已废弃（D-06 / 20260902）**：`messages` / `messages_fts` 随 messages 子系统删除。历史检索改走 `activities` + `ListActivities` RPC。下列 SQL 仅作考古记录。

### 10.1 虚拟表（历史）

```sql
-- REMOVED: CREATE VIRTUAL TABLE messages_fts USING fts5(...)
```

### 10.2–10.3 触发器与查询

已删除。勿再依赖 `messages_fts` / `content_markdown LIKE` 消息全文路径。

---

## 11. Wire DI 注入设计

### 11.1 ProviderSet

```go
// data 层
var ProviderSet = wire.NewSet(
    NewData,
    NewAdminRepo,
    NewAgentRepo,
    NewTeamRepo,
    NewSessionRepo,
    NewSpiritTransactor,
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

### 11.2 注入链路

```
conf.Data → NewData → *Data
*Data + loggateway.Logger → NewAgentRepo → biz.AgentRepository
biz.AgentRepository + ... → NewAgentUsecase → *biz.AgentUsecase
```

### 11.3 事务注入

```
*Data → NewSpiritTransactor → biz.SpiritTransactor
biz.SpiritTransactor → biz.Usecase (需要事务的 Usecase)
```

---

## 12. 错误处理设计

### 12.1 entErrToBizErr

**签名**：`func entErrToBizErr(err error, domain string) error`

```
Ent 错误 → entErrToBizErr(err, domain)
  ├── nil → nil
  ├── *apierror.Error (已是) → 透传
  ├── shared.ErrMessageDuplicate / shared.ErrAgentKeyConflict → CodeConflict (409)
  ├── Ent NotFound / sql.ErrNoRows → CodeNotFound (404)
  ├── Ent ConstraintError → CodeConflict (409)
  ├── Ent NotLoaded → CodeBadRequest (400)
  ├── Postgres 23505 (unique) / 23503 (foreign key) → CodeConflict (409)
  ├── Postgres 23502 (not null) / 23514 (check) → CodeBadRequest (400)
  └── 其他 → CodeInternal (500)
```

### 12.2 迁移错误处理

| 场景 | 处理 |
|------|------|
| DDL 迁移 `duplicate column` | 视为成功（幂等） |
| DDL 迁移 `already exists` | 视为成功（幂等） |
| 数据迁移失败 | `log.Error` + `MarkFailed()` → 健康检查失败 |
| PostgreSQL 连接失败 | NewData 返回错误，拒绝启动（不降级） |

---

## 13. 性能设计

### 13.1 测试基础设施

| 优化 | 实现 |
|------|------|
| schema-per-test 隔离 | 每测试独立 PG schema，并行安全，无串行锁等待 |
| DROP SCHEMA CASCADE | 测试后自动清理，无残留 |
| Ent 自动迁移复用 | SetupTestPG 直接复用生产 Ent Schema，无需维护测试专用 DDL（raw-SQL 测试除外） |

### 13.2 PostgreSQL 优化

| 优化 | 实现 |
|------|------|
| pgvector 索引 | 向量搜索使用 IVFFlat/HNSW 索引 |
| 双连接池 | 写池 16 连接，读池 32 连接 |
| 故障语义 | 连接失败拒绝启动，无降级路径 |

### 13.3 查询优化

| 模式 | 实现 |
|------|------|
| 分页 | `PageToLimitOffset`，默认 20 条，最大 100 条 |
| AIP 过滤 | `go.einride.tech/aip/filtering`，AIP-160 表达式 |
| AIP 排序 | `go.einride.tech/aip/ordering`，AIP-132 排序 |
| 软删除 | `deleted_at = ""` 条件过滤 |
| 字段选择 | Ent `Select()` 只查询需要的字段 |

---

## 14. 配置 Proto 契约

```protobuf
message Data {
  message Database {
    string driver = 1;
    string source = 2;
  }
  message Sqlite {
    bool enable = 1;
    string source = 2;
  }
  message Postgres {
    string source = 1;
    int32 vector_dim = 2;  // embedding 维度，0=默认 1536
  }
  message InitialAdmin {
    string name = 1;
    string email = 2;
    string password = 3;  // YAML 明文，持久化前 MD5
    string access = 4;
  }
  message Redis {
    string network = 1;
    string addr = 2;
    google.protobuf.Duration read_timeout = 3;
    google.protobuf.Duration write_timeout = 4;
  }
  Database database = 1;
  Redis redis = 2;
  // Field 3 was `Sqlite sqlite`, removed in A6 (Postgres is the only primary DB).
  reserved 3;
  reserved "sqlite";
  Postgres postgres = 4;
  InitialAdmin initial_admin = 5;
}
```

---

## 15. 关键文件索引

| 文件 | 设计职责 |
|------|---------|
| `internal/data/data.go` | Data 结构体、初始化流程、ProviderSet、Postgres 双连接池 |
| `internal/data/tx.go` | 事务管理、嵌套检测、上下文分离、双 key 注入 |
| `internal/data/tx_retry.go` | DB 事务重试包装（ExecInTxWithRetry，T2.1） |
| `internal/data/spirit_transactor.go` | biz.SpiritTransactor 适配器 |
| `internal/data/readwrite.go` | ReadWriteClient 读写分离（Ent） |
| `internal/data/readwrite_db.go` | ReadWriteDB 读写分离（原生 SQL） |
| `internal/data/errors.go` | entErrToBizErr 错误转换（含 Postgres 错误） |
| `internal/data/readiness.go` | ReadinessGate 三态门控 |
| `internal/data/ddl_migration_registry.go` | DDL 迁移注册表（54 个迁移） |
| `internal/data/schema_migrations.go` | 数据迁移门控 + 迁移记录管理 |
| `internal/data/cascade_delete.go` | 级联删除逻辑（Agent/Session/Team/Channel） |
| `internal/data/lazy_seeder.go` | 延迟种子数据 |
| `internal/data/testhelper/pg.go` | 测试基础设施：schema-per-test PG 隔离（SetupTestPG/SetupTestPGRaw） |
| `internal/data/sqlite_db.go` | 通用查询辅助（entQueryRowScan，历史命名残留） |
| `internal/data/ent/schema/*.go` | 82 个 Ent Schema 定义 |
| `internal/data/sql/*.sql` | 原生 DDL SQL 文件（非迁移） |
| `internal/data/sql/migrations/*.sql` | DDL 迁移 SQL 文件（28 个版本化文件） |
| `internal/data/vector/store.go` | VectorStore 接口 |
| `internal/data/vector/pgvector.go` | PgVector 向量实现（唯一实现） |
| `internal/data/pgvector/` | 旧版 pgvector 存储（已废弃，待清理） |
| `internal/data/memory_chain_schema.go` | 记忆系统 Schema 初始化 |
| `internal/biz/shared/shared.go` | PageToLimitOffset、ListOptions |
| `internal/biz/spirit_team_usecase.go` | SpiritTransactor 接口定义 |
| `internal/conf/conf.proto` | Data 配置 Proto 定义 |
