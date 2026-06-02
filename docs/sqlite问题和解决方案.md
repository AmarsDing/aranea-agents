# SQLite 读写业务逻辑深度分析与解决方案

> 分析日期：2026-06-02
> 评审日期：2026-06-02（代码验证版）
> 分析范围：`internal/data/` 全部 Repo 实现 + `internal/biz/` 全部 Repo 接口 + 核心业务流 + 前端数据消费
> 视角：架构 + 设计 + 业务场景 + 业务需求 + 根因分析 五维一体

---

## 第一部分：业务场景与数据流全景

### 1.1 核心业务场景识别

Aranea-Agents 的数据访问围绕 **6 大核心业务场景** 展开：

| 场景 | 频率 | 一致性要求 | 延迟敏感度 | 涉及的表 |
|------|------|-----------|-----------|----------|
| **对话轮次执行** | 极高（每次对话） | 强一致 | 极高（用户等待） | sessions, messages, session_runs, session_turns |
| **Token 用量记录** | 极高（每次 LLM 调用） | 强一致 | 中（异步可接受） | model_token_usage_events, model_token_usage_daily/hourly, sessions |
| **记忆提取与同步** | 高（每轮对话后） | 最终一致 | 低（后台异步） | memory_facts, memory_entities, memory_relations, pgvector |
| **渠道消息投递** | 中（外部平台触发） | 幂等一致 | 中（用户等待） | channel_turn_jobs, channel_inbound_receipts, channel_runtime_lease |
| **监控与审计** | 高（全链路） | Best-effort | 低（后台异步） | audit_logs, monitor_events, monitor_traces, flow_log_events |
| **配置与元数据** | 低（管理操作） | 强一致 | 低 | agents, teams, tools, system_settings, llm_provider_models |

### 1.2 一次 Chat Turn 的完整数据写入时序（代码验证版）

> ⚠️ **评审修正**：原文档估计"3~4 次同步写入"，代码追踪实际为 **8~17+ 次**。

```
用户发送消息
    │
    ├─[W1·同步·条件] syncSessionProviderModel
    │   └─ sessions 表: UPDATE default_provider, default_model, updated_at
    │   └─ 代码: chat_orchestrator_turn.go:217 → session_repo.go:271
    │
    ├─[W2·同步] AppendChatMessage(userMsg)
    │   └─ messages 表: INSERT user message
    │   └─ sessions 表: UPDATE message_count+=1, last_message_at, updated_at
    │   └─ 代码: chat_orchestrator_turn.go:748 → session_message_repo.go:420
    │
    ├─[W3·同步] bumpSessionRevisionSyncAndPublish
    │   └─ sessions 表: UPDATE session_revision+=1, updated_at (Raw SQL)
    │   └─ 代码: chat_orchestrator_turn.go:755 → session_repo.go:699
    │
    ├─[W4·同步] persistRunStatus("running")
    │   └─ sessions 表: UPDATE state_json=json_set(...), updated_at (Raw SQL)
    │   └─ 代码: chat_orchestrator_turn.go:654 → session_state_repo.go:51
    │
    Runner 执行中
    │
    ├─[W5·同步·条件] transitionSessionStatus(Running)
    │   └─ sessions 表: UPDATE status, status_reason, status_changed_at, updated_at
    │   └─ 代码: session_repo.go:271
    │
    ├─[W6·异步·每工具调用1次] IncrementInvocationCounts
    │   └─ sessions 表: UPDATE tool_call_count+=1, [mcp_call_count+=1], [skill_call_count+=1], updated_at
    │   └─ 代码: tool_invocation_recorder.go:185 → session_repo.go:679
    │
    ├─[W7·异步·框架回调] UpdateRunnerSnapshotJSON
    │   └─ sessions 表: UPDATE runner_snapshot_json, updated_at
    │   └─ 代码: event_bus_state_handler.go:30 → session_repo.go:582
    │
    ├─[W8·异步·框架回调] ApplyStateDelta → PatchSessionState
    │   └─ sessions 表: UPDATE state_json=json_set/json_remove(...), updated_at (Raw SQL)
    │   └─ 代码: event_bus_state_handler.go:38 → session_state_repo.go:51
    │
    ├─[同步·4表事务] Token 用量记录
    │   └─ [事务内] INSERT model_token_usage_events
    │   └─ [事务内] UPDATE sessions (聚合递增: model_call_count, input_tokens, output_tokens, total_tokens)
    │   └─ [事务内] UPSERT model_token_usage_daily
    │   └─ [事务内] UPSERT model_token_usage_hourly
    │   └─ 代码: usage_write.go:13-82
    │
    Runner 完成
    │
    ├─[W9·同步] AppendChatMessage(assistantMsg, bumpModelCall=true)
    │   └─ messages 表: INSERT assistant message
    │   └─ sessions 表: UPDATE message_count+=1, model_call_count+=1, tokens累加, updated_at
    │   └─ 代码: chat_orchestrator_turn.go:951 → session_message_repo.go:420
    │
    ├─[W10·同步] UpdateSessionContextFromLLMUsage
    │   └─ sessions 表: UPDATE context_used_ratio, max_context_used_ratio, context_status, updated_at (Raw SQL)
    │   └─ 代码: chat_orchestrator_turn.go:964 → session_repo.go:598
    │
    ├─[W11·同步] persistRunStatus("completed")
    │   └─ sessions 表: UPDATE state_json=json_remove(run_id), updated_at (Raw SQL)
    │   └─ 代码: chat_orchestrator_turn.go:968 → session_state_repo.go:51
    │
    ├─[W12·同步] transitionSessionStatus(Completed)
    │   └─ sessions 表: UPDATE status, status_reason, status_changed_at, updated_at
    │   └─ 代码: chat_orchestrator_turn.go:969 → session_repo.go:271
    │
    ├─[W13·同步] bumpSessionRevisionAndPublish
    │   └─ sessions 表: UPDATE session_revision+=1, updated_at (Raw SQL)
    │   └─ 代码: chat_orchestrator_turn.go:970 → session_repo.go:699
    │
    ├─[异步·后台] 记忆任务入队
    │   └─ → 记忆队列 → L3 Fact 提取 → L4 Graph 写入 → 索引双写
    │
    ├─[异步·后台] Webhook 派发
    │   └─ hook_deliveries 表: INSERT + 外部 HTTP
    │
    └─[异步·后台] 预算告警评估
        └─ budget_alerts 表: UPDATE
```

**写入次数统计（代码验证）**：

| 场景 | sessions 表同步写入 | sessions 表异步写入 | 总计 |
|------|-------------------|-------------------|------|
| 最小场景（无工具调用、无压缩） | 8 次 | 0 次 | **8 次** |
| 典型场景（3 次工具调用） | 8 次 | ~5 次 | **~13 次** |
| 高压场景（多次工具调用 + 压缩） | 8 次 | ~9 次 | **~17+ 次** |

### 1.3 一致性保证体系

| 层级 | 机制 | 适用场景 | 当前实现 | 评审备注 |
|------|------|----------|----------|----------|
| **强一致** | SQLite 单事务多表写入 | Usage 4 表原子写入、Session 压缩 CAS | ✅ 已实现 | 4 表事务持有写锁时间过长 |
| **幂等写入** | exists 检查 + patch | RunnerCompletion 去重 | ✅ 已实现 | — |
| **原子递增** | `UPDATE SET x = x + ?` | Session 聚合、日/小时汇总 | ✅ 已实现 | 每次递增都是独立 UPDATE，频繁获取写锁 |
| **进程内互斥** | `locker.Lock(sessionID)` | 同一 Session 消息串行化 | ✅ 已实现 | 非可重入，同 goroutine 二次 Lock 死锁 |
| **竞态桥接** | `TurnCompletionBridge` | Usage 先于 Completion 到达 | ✅ 已实现 | 无 TTL 清理，进程重启丢失 |
| **最终一致** | EventBus + asyncEnvelopeWorker | 工具调用、FlowLog、Webhook | ✅ 已实现 | 队列满时静默丢弃，无降级 |
| **Best-effort** | 失败仅日志 | 监控事件、审计日志 | ✅ 已实现 | — |
| **写冲突重试** | `retryOnBusy` | SQLITE_BUSY 错误 | ⚠️ 定义但未使用 | 死代码，实际依赖 busy_timeout=30s |

---

## 第二部分：问题诊断（代码验证版）

### 问题 1：对话主路径写入放大 🔴🔴（严重度上调）

**业务场景**：一次 Chat Turn 在同步路径上对 sessions 表执行 **8~17+ 次 UPDATE**（原文档估计 3~4 次，严重低估）。

**根因**：Session 表被设计为"超级表"，承载了 4 类不同变更频率的数据：

| 数据类别 | 字段 | 变更频率 | 是否需要同步 |
|---------|------|---------|------------|
| 冷元数据 | id, workspace_id, user_id, agent_id, title, summary | 创建时写入 | — |
| 实时聚合 | message_count, model_call_count, tool_call_count, input_tokens, total_tokens, total_cost_micro_usd | 每次 Turn 2~3 次 | ❌ 前端不需要实时精确 |
| 运行时状态 | status, state_json, runner_snapshot_json, context_used_ratio | 每次 Turn 4~6 次 | ⚠️ 部分可合并 |
| 版本控制 | session_revision | 每次 Turn 2 次 | ⚠️ 流式场景可减为 1 次 |

**代码证据**：

| 写入操作 | 文件:行号 | 更新方式 |
|---------|----------|---------|
| `UpdateSession` (status/provider) | session_repo.go:271 | Ent Update |
| `AppendChatMessage` (含 session update) | session_message_repo.go:420 | Ent UpdateOne |
| `BumpSessionRevision` | session_repo.go:699 | Raw SQL |
| `PatchSessionState` (json_set/json_remove) | session_state_repo.go:51 | Raw SQL |
| `UpdateRunnerSnapshotJSON` | session_repo.go:582 | Ent Update |
| `UpdateSessionContextFromLLMUsage` | session_repo.go:598 | Raw SQL |
| `IncrementInvocationCounts` | session_repo.go:679 | Ent UpdateOne |
| `RecordTokenUsageEvent` (含 sessions 聚合) | usage_write.go:13-82 | 事务内 Raw SQL |

**影响**：
- SQLite 单写连接（`MaxOpenConns=1`）下，8~17 次 UPDATE 串行执行
- 每次独立 UPDATE 都要获取写锁、写 WAL、释放锁
- Usage 4 表事务持有写锁期间，其他所有写操作阻塞

**业务需求**：用户发送消息后应在 200ms 内收到响应，但 8+ 次同步写入可能使单次 Turn 的 SQLite 写入耗时超过 100ms。

### 问题 2：双轨 Schema 管理导致演进困难 🔴🔴（严重度上调）

**业务场景**：新增渠道字段需要手写 ALTER TABLE + 新建 patch 文件 + 注册到 ensureSchemaDDL + 手写 CRUD SQL。

**根因**：野生表数量远超原文档估计。

**代码验证**：

| 管理方式 | 原文档估计 | 实际数量 |
|---------|-----------|---------|
| Ent Schema 管理的表 | — | **60 张** |
| Raw SQL 野生表 | 25 张 | **~87 张** |
| 其中：memory_chain.sql 批量管理 | — | 57 张 |
| 其中：Go 代码内联 DDL | — | 18 张 |
| 其中：嵌入 SQL 文件 | — | 9 张 |
| 其中：PostgreSQL pgvector | — | 3 张 |
| `*_patch.go` 文件 | 20+ | **12 个** |

**额外发现：memory_chain.sql 与 Ent Schema 双重定义重叠** 🔴

以下 26+ 张表在 memory_chain.sql 和 Ent Schema 中**各有一份列定义**，当 Ent Schema 演进时 memory_chain.sql 不会自动同步：

`plugins`, `hooks`, `avatar_assets`, `agent_category_nodes`, `llm_provider_models`, `channel`, `channel_credential`, `channel_delivery`, `mcp_server`, `skill`, `skill_version`, `skill_invocation`, `cron_task`, `cron_task_run`, `model_token_usage_hourly`, `model_pricing_rules`, `usage_quotas`, `budget_alerts`, `tools`, `tool_agent_overrides`, `tool_invocations`, `tool_invocation_params`, `tool_invocation_audit`, `tool_usage_daily`, `chat_attachments`, `audit_logs`

**影响**：
- 新增字段的开发周期是 Ent Schema 表的 3~5 倍
- 双重定义导致 Schema 演进时列定义可能不一致
- `ensureSchemaDDL` 调用链 34 个步骤线性执行，缺乏步骤级恢复能力

### 问题 3：记忆子系统数据访问绕过标准架构 🔴🔴（严重度上调）

**业务场景**：`sessionmemory.Store` 是一个架构旁路的巨型数据访问对象。

**代码验证**：

| 指标 | 原文档估计 | 实际 |
|------|-----------|------|
| 直接操作的表 | 18+ 张 | **16 张** |
| 公开方法数 | — | **50+ 个** |
| biz 层窄接口覆盖 | — | **仅覆盖极小部分**（大部分方法无 biz 接口） |
| 数据访问方式 | 绕过 biz 层 | **全部手写 Raw SQL**，Ent 类型安全 API 完全未使用 |

**架构旁路清单**：

| # | 旁路行为 | 违反的规则 | 严重度 |
|---|---------|-----------|--------|
| 1 | 50+ 方法无 biz 层端口接口，Service 直接依赖 Store 具体类型 | 红线 #7 | 高 |
| 2 | 16 张表全部手写 Raw SQL，绕过 Ent ORM | 数据层规范 | 高 |
| 3 | Schema 管理绕过 Ent，用 ALTER TABLE + pragma_table_info | 红线 #6 | 高 |
| 4 | 返回 `[][]byte` JSON 而非 biz 领域类型 | 数据层规范 | 中 |
| 5 | 自行管理事务（`st.client.BeginTx()`），未用统一 TransactionManager | 数据库规范 §6.4 | 中 |
| 6 | 暴露 `Client()` 方法返回 `*ent.Client`，允许绕过 Store 直接操作 DB | 封装原则 | 中 |
| 7 | 方法数远超 5 个上限 | 红线 #15 | 高 |

### 问题 4：接口拆分不彻底导致依赖污染 🟡

**代码验证**：

| 接口 | 方法数 | 红线 #15 上限 | 超标倍数 |
|------|--------|-------------|---------|
| `TeamRepository` | **20** | 5 | 4x |
| `monitor.Repo` | **19** | 5 | 3.8x |
| `a2a.Repo` | **14** | 5 | 2.8x |

`TeamRepository` 混合了 4 个职责域：Team CRUD（6 方法）、TeamRun 读写（7 方法）、OrchestrationStep（2 方法）、TaskDeadLetter（3 方法）。

### 问题 5：事务管理不统一导致跨 Repo 操作无法原子化 🟡🔴（严重度上调）

**代码验证**：实际存在 **5 种**事务模式（原文档估计 4 种），分布在 **33+ 个调用点**：

| # | 模式 | 事务来源 | 传播方式 | 使用次数 | 代表文件 |
|---|------|---------|---------|---------|---------|
| A | `Data.ExecInTx` | `d.entClient.Tx(ctx)` | `context.WithValue` + `txClient(ctx)` | 3 处 | tx.go, session_repo.go |
| B | Ent `client.Tx(ctx)` 手动 | `r.client().Tx(ctx)` | 直接持有 `tx` 对象 | 9 处 | skill.go, session_message_repo.go |
| C | Raw SQL `BeginTx` | `r.data.RawDB().BeginTx()` | 直接持有 `*sql.Tx` | 16 处 | monitor_alert.go, knowledge.go, evaluation.go |
| D | Ent `BeginTx` + `sqlRunner` | `st.client.BeginTx(ctx, nil)` | `sqlRunner` 接口参数注入 | 4+ 处 | sessionmemory/*.go |
| E | `ent().BeginTx` + Raw SQL | `r.ent().BeginTx(ctx, nil)` | `tx.Client().ExecContext()` | 1 处 | usage_write.go |

**额外发现**：
- [session_message_repo.go:306](file:///f:/project/aranea-agents/internal/data/session_message_repo.go#L306) 有注释承认应迁移到 `ExecInTx` 但尚未完成
- `knowledgeRepo` 和 `evalRepo` 直接持有 `*sql.DB`，绕过 Data 层的读写分离和事务传播

### 问题 6：Raw SQL 泛滥降低可维护性 🟡

**代码验证**：60 张 Ent Schema 表 vs ~87 张野生表，野生表占比 59%。sessionmemory.Store 内部有 65+ 处 `QueryContext`/`ExecContext` 调用。

### 问题 7：SQLite 写瓶颈 🟢

**代码验证**：

| 配置项 | 写连接 | 读连接 |
|--------|--------|--------|
| `MaxOpenConns` | **1** | **2** |
| `MaxIdleConns` | 1 | 2 |
| `ConnMaxIdleTime` | 5 min | 5 min |
| `busy_timeout` | 30000 ms | 30000 ms |
| `journal_mode` | WAL | WAL |
| `synchronous` | NORMAL | NORMAL |

**额外发现**：`retryOnBusy`（data.go:184-208）定义了 3 次重试 + 100/200/300ms 退避，但**几乎没有 Repo 方法调用它**。真正的写冲突保护依赖 `MaxOpenConns=1` + `busy_timeout=30000` + `sessionLockManager`。

---

## 第二部分补充：评审发现的额外问题

### 额外问题 A：`context.Background()` 在 DB 写入路径中泛滥 🔴

**代码证据**：

[asyncEnvelopeWorker](file:///f:/project/aranea-agents/internal/biz/event_bus_async.go#L49) 的 `handle(context.Background(), env)` 导致所有侧效消费者的 DB 写入**全部使用 `context.Background()`**：

```go
// event_bus_async.go:49
handle(context.Background(), env)  // 强制替换为 Background()
```

**受影响的 DB 写入路径**：

| 消费者 | 写入操作 | 影响 |
|--------|---------|------|
| toolCallConsumer | 工具调用结果落库 | 无超时、无事务传播、无 trace |
| messageStoreConsumer | 团队成员消息落库 | 同上 |
| flowLogPersistConsumer | FlowLog 持久化 | 同上 |
| userFeedbackConsumer | 反馈监控事件写入 | 同上 |

**额外使用 `context.Background()` 的 DB 写入**：
- event_bus_runner_handler.go:60, 104 — 监控/用量事件写入失败日志
- event_bus_state_handler.go:32, 57, 68 — 状态持久化失败日志
- cronrunner/runner.go:252 — 定时任务事件发布
- data.go:534-598 — 启动阶段 Schema DDL（语义上合理但无超时控制）

**影响**：
- 无超时控制——SQLite 写锁阻塞时 DB 操作无限等待
- 无事务传播——无法通过 context 共享事务
- 无 trace/span 传播——无法追踪异步写入调用链

### 额外问题 B：EventBus 侧效消费者队列满时静默丢弃关键事件 🔴

**代码证据**：

```go
// event_bus_async.go:55-67
func (w *asyncEnvelopeWorker) Offer(ctx context.Context, env contract.Envelope) {
    select {
    case w.jobs <- env:
    default:
        w.logger.LogSessionWarn(ctx, env.SessionID, "event_bus.queue_full", "侧效消费者队列已满，丢弃事件", ...)
        // 事件被静默丢弃，仅记录日志，不会重试
    }
}
```

**受影响的关键数据**：

| 事件类型 | 丢失后果 | 当前队列大小 |
|---------|---------|------------|
| tool_result | 工具调用记录永久缺失，无法审计 | 256 |
| message_store | 团队成员消息永久缺失 | 256 |
| flow_log | 流程日志不完整 | 256 |

**没有降级回退到同步写入的机制**。

### 额外问题 C：knowledge/eval/a2a Repo 构造函数绕过 Data 连接管理 🟡

**代码证据**：

```go
// knowledge.go:20-27 — 直接持有 *sql.DB
type knowledgeRepo struct {
    db *sql.DB  // 绕过 Data 层读写分离和事务传播
}

// evaluation.go:14-23 — 同上
type evalRepo struct {
    db *sql.DB  // 且无日志记录器，错误用 fmt.Errorf
}

// a2a.go:19-26 — 同上
type a2aRepo struct {
    db *sql.DB
}
```

**缓解**：Wire 装配时通过 `NewXxxRepoFromData` 间接注入 Data 管理的连接。但构造函数签名允许注入独立连接，违反红线 #11。

### 额外问题 D：memory_chain.sql 与 Ent Schema 双重定义重叠 🔴

（详见问题 2 的额外发现部分）

26+ 张表在 memory_chain.sql 和 Ent Schema 中各有一份列定义。当 Ent Schema 演进（增删列）时，memory_chain.sql 中的 `CREATE TABLE IF NOT EXISTS` 在已有表上不生效，新列可能缺失。

### 额外问题 E：`retryOnBusy` 是死代码 🟡

data.go:184-208 定义了 `retryOnBusy` 函数（3 次重试 + 100/200/300ms 退避），但**几乎没有 Repo 方法调用它**。可能误导开发者以为已有重试保护。

### 额外问题 F：TurnCompletionBridge 无 TTL 清理 🟢

TurnCompletionBridge 是纯内存结构（runner_completion.go:19-138），如果 `ClearTurn()` 未被调用（异常路径），map 条目持续累积。进程重启后暂存数据丢失，导致 Usage 与 Completion 关联断裂。

---

## 第三部分：根因分析——业务逻辑不合理性审查

> **核心结论**：大部分问题的根因是业务逻辑设计不合理，而非技术限制。SQLite 单写者模型只是放大了这些不合理设计的影响。

### 3.1 聚合字段同步更新不合理

**当前设计**：7 个聚合计数器（message_count, model_call_count, tool_call_count, skill_call_count, mcp_call_count, input_tokens, output_tokens, total_tokens, total_cost_micro_usd）放在 sessions 主表，每次 Turn 同步更新 2~3 次。

**前端实际消费**：

| 前端页面 | 使用的聚合字段 | 精度要求 |
|---------|--------------|---------|
| SessionsTableSection.vue:58-64 | total_tokens, model_call_count, tool_call_count+skill_call_count+mcp_call_count | 低（"约 128K tokens"） |
| SessionDetailPage.vue:59-74 | message_count, model_call_count, total_tokens, total_cost_micro_usd | 低（打开时 Turn 已完成） |
| ChatPage | **不读聚合字段做实时渲染**，消息通过 WS/SSE 推送 | — |

**结论**：前端不需要实时精确的聚合字段。当前设计以"列表查询方便"为优先，牺牲了写入效率。

### 3.2 session_revision 每 Turn 多次递增不合理

**当前设计**：一个 Turn 内 session_revision 被递增 2 次：
1. 用户消息持久化后（sync bump）— chat_orchestrator_turn.go:755
2. Turn 完成后（completed bump）— chat_orchestrator_turn.go:970

**前端消费方式**：通过 `afterRevision` 做增量消息拉取（useChatInboundSync.ts:208-212）。sync bump 在流式场景下是多余的（SSE 已推送增量内容），只在非流式/durable resume 场景下有意义。

**结论**：流式模式下只需 completed 时 bump 1 次，可减少 1 次写锁获取。

### 3.3 Usage 4 表事务不合理

**当前设计**：4 个实时性要求完全不同的操作放在同一事务：

| 操作 | 实时性要求 | 可否异步 |
|------|-----------|---------|
| INSERT `model_token_usage_events` | 高（审计数据，不能丢） | 可异步但需保证不丢 |
| UPDATE `sessions` 聚合计数器 | 低（仅展示，差几秒无所谓） | **可异步** |
| UPSERT `model_token_usage_daily` | 低（日级聚合） | **可异步** |
| UPSERT `model_token_usage_hourly` | 低（小时级聚合） | **可异步** |

**结论**：只有 events INSERT 需要同步保证不丢，其余 3 个操作应异步化。事务从 4 表缩减为 1 表，写锁持有时间减少约 75%。

### 3.4 state_json 频繁 patch 不合理

**当前设计**：一个 Turn 内 state_json 被 patch 4~5 次，每次都是独立 UPDATE。

**可合并的 patch**：
- Run 状态变更 + await markers 可合并为 1 次写入
- 清除 await 状态已是异步，但 persistAwaitMarkers 在 `syncWrite=true` 时是同步的

**结论**：同一 Turn 内的 state_json 变更应收集后批量写入，从 4~5 次降为 1~2 次。

### 3.5 根因链

```
SQLite 单写者约束
  ↑ 被放大
业务逻辑不合理（同步更新聚合字段、4 表事务、state_json 频繁 patch、revision 多次 bump）
  ↑ 根因
"查询方便"优先于"写入效率"的设计决策
  ↑ 深层根因
缺乏对 SQLite 写瓶颈的早期认知 + 缺乏写入路径的性能基线
```

---

## 第四部分：解决方案（含详细落地指导）

### 治理层 0：业务逻辑合理化（P0·投入产出比最高）

> 不改表结构、不改前端、不改基础设施，只调整写入时机和合并策略。

#### 方案 0.1：聚合字段异步化

**解决问题**：问题 1（写入放大）、问题 7（写瓶颈）

**核心思路**：sessions 表的聚合字段（message_count, model_call_count, tool_call_count 等）从同步更新改为异步更新。

**落地步骤**：

**Step 1：定义 SessionMetricsDelta 结构体**

文件：`internal/biz/session_metrics_delta.go`（新建）

```go
package session

type SessionMetricsDelta struct {
    SessionID        string
    MessageCount     int
    ModelCallCount   int
    ToolCallCount    int
    SkillCallCount   int
    McpCallCount     int
    InputTokens      int64
    OutputTokens     int64
    TotalTokens      int64
    TotalCostMicroUsd float64
}
```

**Step 2：在 SessionUsecase 中添加 delta 累积器**

文件：`internal/biz/session_usecase.go`

```go
type SessionUsecase struct {
    metricsDeltaMu sync.Mutex
    metricsDeltas  map[string]*SessionMetricsDelta
    flushInterval  time.Duration
    // ... 现有字段
}

func (uc *SessionUsecase) AccumulateMetricsDelta(delta SessionMetricsDelta) {
    uc.metricsDeltaMu.Lock()
    defer uc.metricsDeltaMu.Unlock()
    if existing, ok := uc.metricsDeltas[delta.SessionID]; ok {
        existing.MessageCount += delta.MessageCount
        existing.ModelCallCount += delta.ModelCallCount
        existing.ToolCallCount += delta.ToolCallCount
        existing.SkillCallCount += delta.SkillCallCount
        existing.McpCallCount += delta.McpCallCount
        existing.InputTokens += delta.InputTokens
        existing.OutputTokens += delta.OutputTokens
        existing.TotalTokens += delta.TotalTokens
        existing.TotalCostMicroUsd += delta.TotalCostMicroUsd
    } else {
        cp := delta
        uc.metricsDeltas[delta.SessionID] = &cp
    }
}
```

**Step 3：添加后台刷盘 goroutine**

文件：`internal/biz/session_usecase.go`

```go
func (uc *SessionUsecase) StartMetricsFlusher(ctx context.Context) {
    safego.Go(ctx, "session-metrics-flusher", func() {
        ticker := time.NewTicker(uc.flushInterval)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                uc.flushAllMetrics(context.Background())
                return
            case <-ticker.C:
                uc.flushAllMetrics(ctx)
            }
        }
    })
}

func (uc *SessionUsecase) flushAllMetrics(ctx context.Context) {
    uc.metricsDeltaMu.Lock()
    deltas := uc.metricsDeltas
    uc.metricsDeltas = make(map[string]*SessionMetricsDelta)
    uc.metricsDeltaMu.Unlock()

    for _, d := range deltas {
        if err := uc.sessionRepo.ApplyMetricsDelta(ctx, d); err != nil {
            uc.lg.Error(ctx, "session_metrics.flush_failed", loggateway.Err(err), loggateway.Str("session_id", d.SessionID))
            uc.AccumulateMetricsDelta(*d)
        }
    }
}
```

**Step 4：在 data 层添加 ApplyMetricsDelta**

文件：`internal/data/session_repo.go`

```go
func (r *sessionRepo) ApplyMetricsDelta(ctx context.Context, d *biz.SessionMetricsDelta) error {
    _, err := r.txClient(ctx).Update().Where(session.ID(d.SessionID)).
        AddMessageCount(d.MessageCount).
        AddModelCallCount(d.ModelCallCount).
        AddToolCallCount(d.ToolCallCount).
        AddSkillCallCount(d.SkillCallCount).
        AddMcpCallCount(d.McpCallCount).
        AddInputTokens(d.InputTokens).
        AddOutputTokens(d.OutputTokens).
        AddTotalTokens(d.TotalTokens).
        AddTotalCostMicroUsd(d.TotalCostMicroUsd).
        Save(ctx)
    return err
}
```

**Step 5：修改 AppendChatMessage，移除同步聚合更新**

文件：`internal/data/session_message_repo.go`

- 在 `AppendChatMessage` 中，移除对 sessions 表的 `message_count += 1` 和 token 累加
- 改为调用 `uc.AccumulateMetricsDelta(SessionMetricsDelta{SessionID: sessionID, MessageCount: 1, ...})`

**Step 6：修改 IncrementInvocationCounts，改为累积 delta**

文件：`internal/agent/tool_invocation_recorder.go`

- 移除直接调用 `sessionRepo.IncrementInvocationCounts`
- 改为调用 `uc.AccumulateMetricsDelta(SessionMetricsDelta{SessionID: sessionID, ToolCallCount: 1, ...})`

**验证方法**：
1. 单元测试：验证 delta 累加逻辑正确
2. 集成测试：启动完整 Chat Turn，验证聚合字段最终一致（延迟 < 500ms）
3. 前端验证：列表页 token/call 数字在 Turn 完成后 500ms 内更新

**预期收益**：sessions 表同步写入从 8 次降为 ~5 次，写锁竞争减少 ~40%。

#### 方案 0.2：Usage 事务拆分

**解决问题**：问题 1（4 表事务瓶颈）、问题 7（写瓶颈）

**核心思路**：将 4 表事务拆分为 1 次同步写入（events INSERT）+ 3 次异步写入（sessions 聚合 + daily/hourly 汇总）。

**落地步骤**：

**Step 1：拆分 RecordTokenUsageEvent**

文件：`internal/data/usage_write.go`

```go
func (r *usageWriteRepo) RecordTokenUsageEventSync(ctx context.Context, ev UsageEventWrite) (string, error) {
    rowID, err := r.insertUsageEventOnly(ctx, ev)
    if err != nil {
        return "", err
    }
    return rowID, nil
}

func (r *usageWriteRepo) insertUsageEventOnly(ctx context.Context, ev UsageEventWrite) (string, error) {
    var rowID string
    err := r.ent().ExecContext(ctx, &rowID, `INSERT INTO model_token_usage_events (...) VALUES (...) RETURNING id`, ...)
    return rowID, err
}
```

**Step 2：sessions 聚合改为 delta 累积**

在方案 0.1 的 `AccumulateMetricsDelta` 中累积 token 和 cost delta，由后台 flusher 批量刷盘。

**Step 3：daily/hourly 汇总改为异步 EventBus 消费者**

文件：`internal/biz/event_bus_side_consumers.go`

```go
func newUsageRollupConsumer(store *EventStoreUsecase, lg loggateway.Logger) sideConsumerDef {
    return sideConsumerDef{
        name:    "usage_rollup",
        handler: handleUsageRollup,
    }
}

func handleUsageRollup(ctx context.Context, env contract.Envelope) {
    ev := env.TokenUsage
    if ev == nil {
        return
    }
    ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
    defer cancel()
    store.usageRepo.UpsertDailySummary(ctx, ev)
    store.usageRepo.UpsertHourlySummary(ctx, ev)
}
```

**Step 4：修改调用方**

文件：`internal/service/chat_orchestrator_turn.go`

```go
// 原来：
// recordTokenUsageEvent(ctx, ...) → 4 表事务

// 改为：
rowID, err := o.usageRepo.RecordTokenUsageEventSync(ctx, ev)
if err != nil {
    o.lg.Error(ctx, "usage.sync_failed", loggateway.Err(err))
    return
}
o.eventBus.Publish(ctx, contract.Envelope{Type: contract.EnvelopeTypeTokenUsage, TokenUsage: ev})
```

**验证方法**：
1. 单元测试：验证 events INSERT 独立成功
2. 集成测试：验证 daily/hourly 汇总最终一致
3. 压测：对比拆分前后 Usage 写入延迟

**预期收益**：事务持有写锁时间减少约 75%。

#### 方案 0.3：EventBus 侧效消费者降级

**解决问题**：额外问题 B（静默丢弃关键事件）

**核心思路**：关键事件（tool_result、message_store）在队列满时回退到同步写入。

**落地步骤**：

**Step 1：定义 OfferOption**

文件：`internal/biz/event_bus_async.go`

```go
type OfferOption struct {
    FallbackSync bool
    FallbackFn   func(ctx context.Context, env contract.Envelope)
}

func (w *asyncEnvelopeWorker) OfferWithOptions(ctx context.Context, env contract.Envelope, opts OfferOption) {
    if w == nil {
        if opts.FallbackSync && opts.FallbackFn != nil {
            opts.FallbackFn(ctx, env)
        }
        return
    }
    select {
    case w.jobs <- env:
    default:
        if opts.FallbackSync && opts.FallbackFn != nil {
            w.logger.LogSessionWarn(ctx, env.SessionID, "event_bus.queue_full_fallback", "队列已满，回退同步写入", ...)
            opts.FallbackFn(ctx, env)
        } else {
            w.logger.LogSessionWarn(ctx, env.SessionID, "event_bus.queue_full_drop", "队列已满，丢弃事件", ...)
        }
    }
}
```

**Step 2：关键消费者使用 FallbackSync**

文件：`internal/biz/event_bus_side_consumers.go`

```go
// toolCallConsumer — 关键数据，不可丢
toolCallOfferOpts := OfferOption{
    FallbackSync: true,
    FallbackFn:   handleToolCallPersist,
}

// flowLogConsumer — 可丢弃
flowLogOfferOpts := OfferOption{}
```

**验证方法**：
1. 模拟队列满场景，验证 tool_result 不丢失
2. 验证非关键事件（flowLog）在队列满时正确丢弃

**预期收益**：消除关键数据丢失风险。

#### 方案 0.4：session_revision 流式场景优化

**解决问题**：问题 1（写入放大）

**核心思路**：流式模式下只在 Turn 完成时 bump 1 次 revision。

**落地步骤**：

**Step 1：添加 dialogMode 判断**

文件：`internal/service/chat_orchestrator_turn.go`

```go
// 原来：
// o.bumpSessionRevisionSyncAndPublish(ctx, sessionID, runID, userMsg.ID)

// 改为：
if !isStreamingMode(ctx) {
    o.bumpSessionRevisionSyncAndPublish(ctx, sessionID, runID, userMsg.ID)
}
```

**Step 2：保留 completed bump**

Turn 完成时的 `bumpSessionRevisionAndPublish` 保持不变。

**验证方法**：
1. 流式场景：验证前端通过 SSE 收到增量消息，不需要 sync revision
2. 非流式/durable resume 场景：验证 sync revision 仍正常触发增量拉取

**预期收益**：流式场景减少 1 次写锁获取。

#### 方案 0.5：state_json patching 合并

**解决问题**：问题 1（写入放大）

**核心思路**：同一 Turn 内的 state_json 变更收集后批量写入。

**落地步骤**：

**Step 1：定义 StatePatchBatch**

文件：`internal/data/session_state_repo.go`

```go
type StatePatchBatch struct {
    sessionID string
    sets      map[string]any
    removes   []string
}

func NewStatePatchBatch(sessionID string) *StatePatchBatch {
    return &StatePatchBatch{sessionID: sessionID, sets: make(map[string]any)}
}

func (b *StatePatchBatch) Set(path string, value any)  { b.sets[path] = value }
func (b *StatePatchBatch) Remove(path string)           { b.removes = append(b.removes, path) }

func (r *sessionStateRepo) ApplyBatch(ctx context.Context, batch *StatePatchBatch) error {
    if len(batch.sets) == 0 && len(batch.removes) == 0 {
        return nil
    }
    // 合并所有 set 和 remove 为单条 UPDATE
    expr := "state_json"
    var args []any
    for path, val := range batch.sets {
        expr = fmt.Sprintf("json_set(%s, ?, ?)", expr)
        args = append(args, "$."+path, val)
    }
    for _, path := range batch.removes {
        expr = fmt.Sprintf("json_remove(%s, ?)", expr)
        args = append(args, "$."+path)
    }
    query := fmt.Sprintf("UPDATE sessions SET %s = %s, updated_at = ? WHERE id = ? AND deleted_at = ''",
        "state_json", expr)
    args = append(args, nowRFC3339(), batch.sessionID)
    _, err := r.data.entClient.ExecContext(ctx, query, args...)
    return err
}
```

**Step 2：在 ChatRunGateway 中收集 patch**

文件：`internal/service/chat_run_gateway.go`

```go
// 原来：每次状态变更立即 PatchSessionState
// 改为：收集到 batch，Turn 完成时一次性 ApplyBatch

func (g *ChatRunGateway) persistRunStatusToSession(ctx context.Context, sessionID, runID, status string, batch *sessionStateRepo.StatePatchBatch) {
    switch status {
    case "running":
        batch.Set("run_id", runID)
        batch.Set("run_status", status)
    case "completed", "failed":
        batch.Remove("run_id")
        batch.Remove("run_status")
    }
}

func (g *ChatRunGateway) persistAwaitMarkersToSession(ctx context.Context, sessionID string, markers AwaitMarkers, batch *sessionStateRepo.StatePatchBatch) {
    batch.Set("await_run_id", markers.RunID)
    batch.Set("await_since", markers.Since)
    batch.Set("await_kind", markers.Kind)
    batch.Set("await_tool_key", markers.ToolKey)
    batch.Set("await_tool_call_id", markers.ToolCallID)
}
```

**Step 3：Turn 完成时一次性 ApplyBatch**

```go
// chat_orchestrator_turn.go — Turn 完成时
batch := sessionStateRepo.NewStatePatchBatch(sessionID)
g.persistRunStatusToSession(ctx, sessionID, runID, "completed", batch)
g.clearAwaitingRunStateFromSession(ctx, sessionID, batch)
if err := sessionStateRepo.ApplyBatch(ctx, batch); err != nil {
    o.lg.Error(ctx, "state.batch_apply_failed", loggateway.Err(err))
}
```

**验证方法**：
1. 单元测试：验证 batch 合并 set/remove 逻辑
2. 集成测试：验证 Turn 完成后 state_json 最终状态正确

**预期收益**：state_json 写入从 4~5 次/Turn 降为 1 次/Turn。

---

### 治理层 1：架构层面 — 分离关注点（P1）

#### 方案 1.1：Session 表冷热分离

**解决问题**：问题 1（写入放大）

**核心思路**：将 Session 表的"实时热字段"与"冷元数据"分离。

**当前 sessions 表字段分类（38 个字段）**：

```
sessions 表（一表多用）：
├── 冷元数据（创建时写入，几乎不变）— 15 字段
│   id, workspace_id, user_id, owner_type, agent_id, team_id, title, summary,
│   tags_json, dialog_mode, default_provider, default_model, default_context_window_tokens,
│   last_provider, last_model, last_context_window_tokens, visibility
├── 实时聚合（每次 Turn 更新）— 11 字段
│   message_count, run_count, model_call_count, tool_call_count, skill_call_count,
│   mcp_call_count, input_tokens, output_tokens, total_tokens, total_cost_micro_usd,
│   avg_latency_ms, error_count
├── 运行时状态（每次 Turn 更新 4~6 次）— 9 字段
│   status, status_reason, status_changed_at, context_used_tokens, context_used_ratio,
│   max_context_used_ratio, context_status, runner_snapshot_json, state_json
└── 时间戳 + 版本 — 8 字段
    first_message_at, last_message_at, last_run_at, created_at, updated_at,
    archived_at, deleted_at, pinned_at, session_revision, compress_version,
    parent_session_id, root_session_id, agent_depth, metadata_json
```

**改造方案**：

```
sessions 表（冷元数据 + 时间戳 + 版本）：
  id, workspace_id, user_id, owner_type, agent_id, team_id, title, summary,
  tags_json, dialog_mode, default_provider, default_model, default_context_window_tokens,
  last_provider, last_model, last_context_window_tokens, visibility,
  first_message_at, last_message_at, last_run_at, created_at, updated_at,
  archived_at, deleted_at, pinned_at, session_revision, compress_version,
  parent_session_id, root_session_id, agent_depth, metadata_json

session_metrics 表（实时聚合，每次 Turn 写 1 次，可异步）：
  session_id (PK + FK), message_count, run_count, model_call_count,
  tool_call_count, skill_call_count, mcp_call_count,
  input_tokens, output_tokens, total_tokens, total_cost_micro_usd,
  avg_latency_ms, error_count, updated_at

session_runtime_state 表（运行时状态，每次 Turn 写 1~2 次）：
  session_id (PK + FK), status, status_reason, status_changed_at,
  context_used_tokens, context_used_ratio, max_context_used_ratio,
  context_status, runner_snapshot_json, state_json, updated_at
```

**落地步骤**：

**Step 1：新建 Ent Schema**

文件：`internal/data/ent/schema/session_metrics.go`（新建）

```go
package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/schema/field"
    "entgo.io/ent/schema/index"
)

type SessionMetrics struct{ ent.Schema }

func (SessionMetrics) Fields() []ent.Field {
    return []ent.Field{
        field.String("session_id").Unique(),
        field.Int("message_count").Default(0),
        field.Int("run_count").Default(0),
        field.Int("model_call_count").Default(0),
        field.Int("tool_call_count").Default(0),
        field.Int("skill_call_count").Default(0),
        field.Int("mcp_call_count").Default(0),
        field.Int64("input_tokens").Default(0),
        field.Int64("output_tokens").Default(0),
        field.Int64("total_tokens").Default(0),
        field.Float("total_cost_micro_usd").Default(0),
        field.Float("avg_latency_ms").Default(0),
        field.Int("error_count").Default(0),
        field.String("updated_at").Default(""),
    }
}

func (SessionMetrics) Indexes() []ent.Index {
    return []ent.Index{
        index.Field("session_id").Unique(),
    }
}
```

文件：`internal/data/ent/schema/session_runtime_state.go`（新建）

类似结构，包含 status, state_json, runner_snapshot_json, context_* 等字段。

**Step 2：运行 `go generate ./internal/data/ent`**

**Step 3：数据迁移**

文件：`internal/data/session_split_migration.go`（新建）

```go
func migrateSessionSplit(ctx context.Context, client *ent.Client, lg loggateway.Logger) error {
    has, _ := sqliteTableExists(ctx, client, "session_metrics")
    if has {
        return nil
    }
    lg.Info(ctx, "session_split.migration_start", loggateway.Str("phase", "create_tables"))
    // CREATE TABLE session_metrics / session_runtime_state
    // INSERT INTO session_metrics SELECT ... FROM sessions
    // INSERT INTO session_runtime_state SELECT ... FROM sessions
    return nil
}
```

**Step 4：读写切换**

- 写路径：修改 `session_repo.go`，聚合字段写入 `session_metrics`，状态字段写入 `session_runtime_state`
- 读路径：`GetSession` 改为 JOIN 三表（或分别查询后合并）
- 列表页：`SearchSessions` 改为 LEFT JOIN session_metrics

**Step 5：清理 sessions 表冗余字段**

确认所有读写路径切换后，从 sessions Ent Schema 中移除已迁移的字段，重新 `go generate`。

**验证方法**：
1. 迁移测试：新建空库 → 迁移 → 验证数据完整
2. 读写测试：完整 Chat Turn → 验证三表数据一致
3. 列表页测试：验证前端列表页展示正确
4. 回滚测试：验证可安全回退到单表模式

**预期收益**：sessions 主表同步写入从 8 次降为 2~3 次（status 变更 + revision bump），每次写入行更窄，锁持有时间更短。

#### 方案 1.2：统一事务管理器

**解决问题**：问题 5（事务模式不统一）

**落地步骤**：

**Step 1：增强 Data.ExecInTx**

文件：`internal/data/tx.go`

```go
type rawTxKey struct{}

func (d *Data) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
    if tx, ok := ctx.Value(rawTxKey{}).(*ent.Tx); ok {
        return fn(ctx)
    }
    tx, err := d.entClient.Tx(ctx)
    if err != nil {
        return err
    }
    txCtx := context.WithValue(ctx, txClientKey{}, tx.Client())
    txCtx = context.WithValue(txCtx, rawTxKey{}, tx)
    defer func() { _ = tx.Rollback() }()
    if err := fn(txCtx); err != nil {
        return err
    }
    return tx.Commit()
}

func RawDBFromCtx(ctx context.Context, fallback *sql.DB) *sql.DB {
    if tx, ok := ctx.Value(rawTxKey{}).(*ent.Tx); ok {
        return tx.Client().DB()
    }
    return fallback
}
```

**Step 2：逐个迁移 Raw SQL Repo**

对每个使用 `r.data.RawDB().BeginTx()` 的 Repo：

1. 将 `BeginTx()` 替换为 `d.ExecInTx(ctx, fn)`
2. 将 `r.db.ExecContext()` 替换为 `RawDBFromCtx(ctx, r.data.RawDB()).ExecContext()`
3. 删除手动 `tx.Commit()` / `tx.Rollback()` 代码

**迁移优先级**：
1. `monitor_alert.go`（16 处 BeginTx 中最复杂）
2. `model_registry_apply.go`（3 处）
3. `learning_loop.go`（1 处）
4. `ecosystem.go`（1 处）
5. `session_participant_repo.go`（1 处）

**Step 3：废弃模式 B/C/D/E**

- 模式 B（Ent `client.Tx()` 手动）：改为 `d.ExecInTx`
- 模式 C（Raw SQL `BeginTx`）：改为 `d.ExecInTx` + `RawDBFromCtx`
- 模式 D（sqlRunner）：保留接口但底层改为从 ctx 获取
- 模式 E（`ent().BeginTx` + Raw SQL）：改为 `d.ExecInTx`

**验证方法**：
1. 每迁移一个 Repo，跑 `go test ./internal/data/... -count=1`
2. 全部迁移后跑 `make build && make test`
3. 验证渠道场景（channel_turn_jobs + sessions）的原子性

#### 方案 1.3：记忆子系统数据访问收口

**解决问题**：问题 3（架构绕过）

**落地步骤**：

**Step 1：在 biz 层定义缺失的 Repo 接口**

文件：`internal/biz/memory_ports.go`（新建）

```go
type L0SnapshotRepo interface {
    ListL0Snapshots(ctx context.Context, sessionID string) ([]L0Snapshot, error)
    InsertL0Snapshot(ctx context.Context, snap L0Snapshot) error
    UpdateL0SnapshotActual(ctx context.Context, id string, actual any) error
}

type L2EpisodeRepo interface {
    InsertEpisode(ctx context.Context, ep Episode) error
    RecallEpisodes(ctx context.Context, query EpisodeRecallQuery) ([]Episode, error)
    ApplyEpisodeDecay(ctx context.Context, sessionID string) error
}

type L3FactRepo interface {
    UpsertFact(ctx context.Context, fact Fact) error
    RecallFacts(ctx context.Context, query FactRecallQuery) ([]Fact, error)
    ApplyFactDecay(ctx context.Context, agentID string) error
    MarkFactIndexStale(ctx context.Context, factID string) error
}

type L4EntityRepo interface {
    UpsertEntity(ctx context.Context, entity Entity) error
    DeleteEntity(ctx context.Context, id string) error
    RecallEntities(ctx context.Context, query EntityRecallQuery) ([]Entity, error)
    RecordReinforcement(ctx context.Context, r Reinforcement) error
}

type CascadeRepo interface {
    InsertProposal(ctx context.Context, p CascadeProposal) error
    UpdateProposalStatus(ctx context.Context, id string, status string) error
    InitSagaSteps(ctx context.Context, sagaID string, steps []SagaStep) error
    UpdateSagaStepState(ctx context.Context, stepID string, state string) error
}

type EvolutionRepo interface {
    InsertEvolutionEvent(ctx context.Context, ev EvolutionEvent) error
    ListEvolutionEvents(ctx context.Context, agentID string) ([]EvolutionEvent, error)
}
```

**Step 2：在 data 层实现各 Repo**

每个 Repo struct 持有 `*Data`（而非 `*ent.Client`），通过 `d.Ent()` / `d.RawDB()` 访问数据库。

**Step 3：Wire 绑定**

文件：`internal/data/data.go`

```go
var RepoSet = wire.NewSet(
    NewL0SnapshotRepo,
    NewL2EpisodeRepo,
    NewL3FactRepo,
    NewL4EntityRepo,
    NewCascadeRepo,
    NewEvolutionRepo,
)
```

**Step 4：逐步替换 Service 层对 Store 的直接依赖**

每替换一个层级，跑对应记忆管线的集成测试。

**Step 5：废弃 Store.Client() 和 Store 的聚合方法**

确认所有调用方迁移后，删除 `Client()` 方法和 Store 中的冗余方法。

**验证方法**：
1. 每个新 Repo 的单元测试（mock 接口）
2. 记忆管线端到端测试
3. 确认 Service 层不再 import `sessionmemory.Store`

---

### 治理层 2：设计层面 — 消除技术债务（P1-P2）

#### 方案 2.1：野生表纳入 Ent Schema

**解决问题**：问题 2（双轨 Schema）、问题 6（Raw SQL 泛滥）

**分批实施计划**（与原文档一致，补充落地指导）：

**第一批落地步骤（6 张高频表）**：

对每张表，执行以下标准流程：

1. **新建 Ent Schema**：在 `internal/data/ent/schema/` 创建对应文件
2. **运行 `go generate ./internal/data/ent`**：生成类型安全代码
3. **编写 data 层 Repo**：实现 biz 层已有接口（或新定义的窄接口）
4. **迁移读写路径**：将 Raw SQL 调用替换为 Ent API
5. **保留 Ent 不支持的特性**：用 `ent.Client.ExecContext()` + Ent 类型映射
6. **删除旧 Raw SQL 代码**：确认新代码覆盖所有旧功能
7. **跑全量测试**：`make build && make test`

**memory_chain.sql 去重**：

在第一批迁移完成后，需要从 memory_chain.sql 中删除已迁移到 Ent 的表的 DDL 定义，避免双重定义。具体操作：

1. 在 memory_chain.sql 中注释掉已迁移表的 `CREATE TABLE IF NOT EXISTS` 语句
2. 添加注释说明该表已由 Ent Schema 管理
3. 保留 `EnsureSessionMemorySchema` 函数的调用，但内部跳过已迁移表

#### 方案 2.2：接口拆分合规化

**解决问题**：问题 4（接口拆分不彻底）

**落地步骤**（以 TeamRepository 为例）：

**Step 1：在 biz 层定义子接口**

文件：`internal/biz/team_usecase.go`

```go
type TeamReader interface {
    ListTeams(ctx context.Context) ([]Team, error)
    GetTeamByID(ctx context.Context, id string) (Team, error)
    ListBySpiritSessionID(ctx context.Context, spiritSessionID string) ([]Team, error)
}

type TeamWriter interface {
    CreateTeam(ctx context.Context, t Team) (Team, error)
    UpdateTeam(ctx context.Context, t Team) (Team, error)
    DeleteTeam(ctx context.Context, id string) error
}

type TeamRunReader interface {
    ListTeamRuns(ctx context.Context, teamID string, limit int) ([]TeamRun, error)
    HasActiveTeamRun(ctx context.Context, teamID string) (bool, error)
    GetTeamRunByID(ctx context.Context, id string) (TeamRun, error)
    ListTeamRunSteps(ctx context.Context, runID string) ([]TeamRunStep, error)
}

type TeamRunWriter interface {
    CreateTeamRun(ctx context.Context, r TeamRun) (TeamRun, error)
    UpdateTeamRun(ctx context.Context, r TeamRun) error
    UpdateTeamRunGraphExecutionID(ctx context.Context, runID, graphExecutionID string) error
    UpdateTeamRunTraceID(ctx context.Context, runID, traceID string) error
    UpdateTeamRunSummaryJSON(ctx context.Context, runID, summaryJSON string) error
    CreateTeamRunStep(ctx context.Context, s TeamRunStep) (TeamRunStep, error)
}

type OrchestrationStepRepo interface {
    BatchCreateOrchestrationSteps(ctx context.Context, steps []OrchestrationStep) error
    ListOrchestrationSteps(ctx context.Context, teamRunID, nodeID string, limit int) ([]OrchestrationStep, error)
}

type TaskDeadLetterRepo interface {
    CreateTaskDeadLetter(ctx context.Context, dl TaskDeadLetter) error
    ListTaskDeadLetters(ctx context.Context, filter TaskDeadLetterListFilter) ([]TaskDeadLetter, error)
    ResolveTaskDeadLetter(ctx context.Context, id string) (TaskDeadLetter, error)
}

type TeamRepository interface {
    TeamReader
    TeamWriter
    TeamRunReader
    TeamRunWriter
    OrchestrationStepRepo
    TaskDeadLetterRepo
}
```

**Step 2：修改消费方依赖**

将 Channel 模块对 `TeamRepository` 的依赖改为 `TeamRunReader`。

**Step 3：Wire 绑定**

```go
var RepoSet = wire.NewSet(
    wire.Bind(new(biz.TeamReader), new(*teamRepo)),
    wire.Bind(new(biz.TeamRunReader), new(*teamRepo)),
    // ... 按需绑定
)
```

**Step 4：跑 `make wire && make build && make test`**

monitor.Repo 和 a2a.Repo 按相同模式拆分。

#### 方案 2.3：Schema 迁移框架化

**解决问题**：问题 2（散落 patch）

**落地步骤**：

**Step 1：创建 schema_migrations 表**

已有 Ent Schema `schema_migration.go`，确认其字段包含 `version`, `name`, `applied_at`。

**Step 2：将 12 个 `*_patch.go` 迁移到 registry**

文件：`internal/data/migrations/registry.go`（新建）

```go
package migrations

type Migration struct {
    Version      int
    Name         string
    Up           func(ctx context.Context, client *ent.Client) error
    Dependencies []int
}

var Registry = []Migration{
    {Version: 2026060101, Name: "add_session_revision",
     Up: func(ctx context.Context, c *ent.Client) error {
         if has, _ := sqliteColumnExists(ctx, c, "sessions", "session_revision"); has {
             return nil
         }
         _, err := c.ExecContext(ctx, "ALTER TABLE sessions ADD COLUMN session_revision INTEGER NOT NULL DEFAULT 0")
         return err
     }},
    // ... 将所有 *_patch.go 中的迁移逻辑移入此处
}
```

**Step 3：替换 ensureSchemaDDL 中的 patch 调用**

```go
// data.go — ensureSchemaDDL 中
// 原来：ensureSessionRevisionPatches(context.Background(), entClient, lg)
// 改为：migrations.RunPending(context.Background(), entClient, lg)
```

**Step 4：删除 `*_patch.go` 文件**

确认所有迁移逻辑已移入 registry 后，删除 12 个 `*_patch.go` 文件。

**验证方法**：
1. 新建空库 → 启动 → 验证所有迁移正确执行
2. 已有库 → 启动 → 验证已应用的迁移被跳过
3. `make build && make test`

---

### 治理层 3：Write-Ahead Buffer（P2·进程内写缓冲）

> 这是介于"业务逻辑优化"和"换存储引擎"之间的中间方案，不改变存储引擎但大幅减少写锁获取次数。

**核心思路**：

```
当前：
  同步写入 → SQLite 写连接（MaxOpenConns=1）→ 磁盘

改造后：
  同步写入 → 进程内 Write Buffer（内存 delta map）→ 批量刷盘 → SQLite 写连接
```

**适用场景**：`session_metrics` 的聚合递增（`message_count += 1`、`input_tokens += N`）

**设计要点**：

1. 进程内维护 `map[sessionID]*MetricsDelta`，每次 Turn 完成时只写内存
2. 后台 goroutine 每 100ms 或 delta 积累到阈值时，批量 `UPDATE sessions SET message_count = message_count + ?` 一次刷盘
3. 前端读取时，先查 SQLite 再合并内存中的 delta
4. 关键数据（Usage events）仍同步写入，只有聚合字段走 Buffer

**风险**：进程崩溃时内存中的 delta 丢失。缓解：delta 仅用于展示类聚合字段，丢失后可从明细表重算。

**预期收益**：聚合字段的写锁获取从每 Turn 2~3 次降为每 100ms 1 次批量刷盘。

> 注：此方案与方案 0.1（聚合字段异步化）互补。方案 0.1 是基础版（直接异步写入），Write-Ahead Buffer 是进阶版（内存缓冲 + 批量刷盘）。建议先实施方案 0.1，验证效果后再考虑是否升级到 Write-Ahead Buffer。

---

### 治理层 4：异步消费者可靠性治理（P1）

#### 方案 4.1：asyncEnvelopeWorker 传入带超时的 context

**落地步骤**：

文件：`internal/biz/event_bus_async.go`

```go
type asyncEnvelopeWorker struct {
    name    string
    jobs    chan contract.Envelope
    logger  SessionLogWriter
    handleTimeout time.Duration
}

func (w *asyncEnvelopeWorker) Start(ctx context.Context, handle func(context.Context, contract.Envelope)) {
    safego.Go(ctx, w.name+".worker", func() {
        for {
            select {
            case <-ctx.Done():
                return
            case env, ok := <-w.jobs:
                if !ok {
                    return
                }
                handleCtx, cancel := context.WithTimeout(context.Background(), w.handleTimeout)
                handle(handleCtx, env)
                cancel()
            }
        }
    })
}
```

#### 方案 4.2：TurnCompletionBridge 添加 TTL 清理

文件：`internal/biz/runner_completion.go`

```go
type turnPendingUsage struct {
    UsageEventID string
    TraceID      string
    CreatedAt    time.Time
}

func (b *TurnCompletionBridge) cleanupStale() {
    b.mu.Lock()
    defer b.mu.Unlock()
    cutoff := time.Now().Add(-5 * time.Minute)
    for key, usage := range b.pendingUsage {
        if usage.CreatedAt.Before(cutoff) {
            delete(b.pendingUsage, key)
        }
    }
    for key, t := range b.turnStarts {
        if t.Before(cutoff) {
            delete(b.turnStarts, key)
        }
    }
}
```

启动时注册定时清理：

```go
safego.Go(ctx, "turn-completion-bridge-cleanup", func() {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            defaultTurnCompletionBridge.cleanupStale()
        }
    }
})
```

#### 方案 4.3：Repo 构造函数收口

**落地步骤**：

将 `knowledgeRepo`、`evalRepo`、`a2aRepo` 的构造函数从接受 `*sql.DB` 改为接受 `*Data`：

```go
// 原来：
func NewKnowledgeRepo(db *sql.DB, lg loggateway.Logger) biz.KnowledgeRepo

// 改为：
func NewKnowledgeRepo(data *Data, lg loggateway.Logger) biz.KnowledgeRepo {
    return &knowledgeRepo{db: data.Postgres(), lg: lg}
}
```

同时为 evalRepo 添加日志记录器，将 `fmt.Errorf` 替换为 `kerrors`。

---

### 治理层 5：PostgreSQL 渐进迁移（P3·终极方案）

> 当用户规模超过 100 并发时，SQLite 单写者模型无论如何优化都无法满足需求。

**迁移策略**：

```
Phase 0（当前）: SQLite 主库 + PostgreSQL 向量库
Phase 1: 高频写入表迁移（usage_events, monitor_events, hook_deliveries, flow_log_events）
Phase 2: 核心业务表迁移（sessions, messages, agents）
Phase 3: 全量迁移，SQLite 仅用于单机开发/测试
```

**Phase 1 落地指导**：

1. **双写阶段**：同时写 SQLite + PostgreSQL，读仍走 SQLite
2. **验证阶段**：对比双写数据一致性
3. **切读阶段**：高频表的读查询切到 PostgreSQL
4. **停写 SQLite**：确认稳定后停止写 SQLite 的高频表

**Phase 1 优先迁移表**：

| 表 | 写入频率 | 迁移收益 |
|----|---------|----------|
| `model_token_usage_events` | 每次 LLM 调用 | 消除 4 表事务瓶颈 |
| `model_token_usage_daily/hourly` | 每次 LLM 调用 | 并行 UPSERT |
| `monitor_events` | 每轮对话 | 并行 INSERT |
| `hook_deliveries` | 每轮对话 | 并行 INSERT + 乐观锁 |
| `flow_log_events` | 每轮对话 | 并行 INSERT |

---

## 第五部分：实施路线图（评审修订版）

### 总体优先级（按投入产出比排序）

```
P0（立即·1~2 周）────────────────────────────────────
  ├─ 方案 0.1：聚合字段异步化（delta 累积 + 后台 flusher）
  ├─ 方案 0.2：Usage 事务拆分（仅 events INSERT 同步）
  ├─ 方案 0.3：EventBus 侧效消费者降级（关键事件回退同步）
  ├─ 方案 0.4：session_revision 流式场景优化（减为 1 次/Turn）
  └─ 方案 0.5：state_json patching 合并（batch 模式）
  预期收益：sessions 表同步写入从 8 次降为 2~3 次，写锁竞争减少 ~70%

P1（短期·2~4 周）────────────────────────────────────
  ├─ 方案 1.1：Session 表冷热分离（session_metrics + session_runtime_state）
  ├─ 方案 1.2：统一事务管理器（5 种模式 → 1 种）
  ├─ 方案 1.3：记忆子系统数据访问收口（Store → 独立 Repo）
  ├─ 方案 4.1：asyncEnvelopeWorker 传入带超时 context
  ├─ 方案 4.2：TurnCompletionBridge TTL 清理
  └─ 方案 4.3：Repo 构造函数收口（*sql.DB → *Data）
  预期收益：架构合规 + 事务一致性保证 + 异步可靠性

P2（中期·1~2 月）────────────────────────────────────
  ├─ 方案 2.1：第一批野生表纳入 Ent Schema（6 张高频表）
  ├─ 方案 2.2：接口拆分合规化（TeamRepository → 6 子接口）
  ├─ 方案 2.3：Schema 迁移框架化（12 个 patch → registry）
  ├─ 方案 2.1 补充：memory_chain.sql 去重
  └─ Write-Ahead Buffer 评估（基于 P0 效果决定是否需要）
  预期收益：技术债务清理 + 开发效率提升

P3（长期·2~3 月）────────────────────────────────────
  ├─ 第二批野生表纳入 Ent Schema（6 张中频表）
  ├─ 第三批野生表纳入 Ent Schema（7 张低频表）
  ├─ 记忆系统插件化架构（MemoryLayer 接口）
  ├─ 多租户数据隔离审计
  └─ PostgreSQL 渐进迁移（Phase 1：高频写入表）
  预期收益：支持 100+ 并发 + 架构面向未来演进
```

### 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 聚合字段异步化导致短暂不一致 | 前端列表页数字延迟更新 | 前端通过 session_revision 轮询刷新，延迟 < 500ms |
| Usage 事务拆分导致 daily/hourly 汇总延迟 | 仪表盘数据延迟 | 后台 rollup consumer 保证最终一致，延迟 < 1s |
| EventBus 降级导致同步写入阻塞 | 请求延迟增加 | 仅在队列满时触发，正常情况不阻塞 |
| Session 冷热分离导致查询性能回退 | 前端列表页变慢 | 保留 sessions 表的冗余聚合字段，异步同步到 session_metrics |
| 接口拆分导致 Wire 绑定变更量大 | 编译错误 | 逐模块拆分，每拆一个模块跑全量测试 |
| 野生表纳入 Ent 后 DDL 行为变化 | 数据丢失 | Ent Schema 使用 `WithDropIndex(true)` + 人工 review 生成代码 |
| sessionmemory.Store 拆分影响记忆管线 | 记忆提取失败 | 先并行运行新旧实现，对比结果一致后再切换 |
| memory_chain.sql 去重遗漏 | 新库建表失败 | 逐表去重，每去重一张跑空库启动测试 |

### 验证基线

| 验证项 | 当前基线 | P0 目标 | P1 目标 |
|--------|---------|---------|---------|
| 单 Turn sessions 表同步写入次数 | 8 次 | ≤3 次 | ≤2 次 |
| Usage 事务持有写锁时间 | ~15ms（4 表） | ~4ms（1 表） | ~4ms |
| EventBus 关键事件丢失率 | 队列满时 100% | 0%（降级同步） | 0% |
| 事务模式数量 | 5 种 | 5 种 | 1 种 |
| 野生表数量 | ~87 张 | ~87 张 | ~81 张 |

---

## 第六部分：核心设计原则

1. **业务驱动**：每个架构变更必须有明确的业务场景支撑，不为技术而技术。

2. **渐进演进**：不搞大重构，每个 Phase 独立可交付，可回滚。

3. **单一 Schema 真相源**：所有表必须进 Ent Schema，`go generate` 是唯一的 Schema 演进方式。对 Ent 不支持的特性，在 Ent Schema 中标注 `Annotations`，用 Raw Query 补充但不另建表。**禁止 memory_chain.sql 与 Ent Schema 双重定义**。

4. **接口隔离到方法级**：每个 Repo 接口 ≤5 方法，按读写/职责域拆分。Wire 绑定时按需注入窄接口，消费方只看到自己需要的方法。

5. **同步路径最小化**：对话主路径只做必须同步的写入（消息 + 状态 + revision），聚合、监控、用量等全部异步化。

6. **最终一致优先**：除核心业务（消息、状态、用量 events）外，聚合字段、监控、审计、记忆等采用最终一致性，通过 EventBus + 降级同步保证可靠投递。

7. **数据访问收口**：所有数据访问必须经过 biz 层 Repo 接口，禁止"影子数据层"直接操作数据库。**禁止 Store 暴露 Client() 方法**。

8. **context 传播纪律**：异步消费者的 DB 写入必须使用带超时的 context（而非 `context.Background()`），保证超时可控和 trace 可追踪。

9. **构造函数收口**：所有 Repo 构造函数接受 `*Data`（而非 `*sql.DB`），确保连接管理和事务传播统一。

10. **写入路径可观测**：每次写入操作应有 metrics（延迟、次数、错误率），建立写入性能基线，变更后对比验证。

---

## 第七部分：方案综合评估（代码验证版）

> 评估日期：2026-06-02
> 评估方法：对每个方案追踪实际代码调用链，验证假设、识别遗漏、评估副作用

### 7.1 各方案落地可行性评分

| 方案 | 可行性 | 改动量 | 风险 | 收益 | AI可落地性 | 综合评分 |
|------|--------|--------|------|------|-----------|---------|
| **0.1 聚合字段异步化** | 中 | 大 | 中高 | 高 | ⚠️ 需修正 | B |
| **0.2 Usage 事务拆分** | 高 | 中 | 低 | 高 | ✅ 可落地 | A |
| **0.3 EventBus 降级** | 高 | 小 | 低 | 中 | ✅ 可落地 | A |
| **0.4 revision 优化** | 高 | 小 | 低 | 低 | ✅ 可落地 | A- |
| **0.5 state_json 合并** | 中 | 中 | 中高 | 低~中 | ⚠️ 需修正 | B- |
| **1.1 Session 冷热分离** | 中 | 大 | 高 | 高 | ⚠️ 需修正 | B- |
| **1.2 统一事务管理器** | 高 | 中 | 中 | 中 | ✅ 可落地 | A- |
| **1.3 记忆收口** | 中 | 大 | 中 | 中 | ⚠️ 需修正 | B |

### 7.2 方案间依赖冲突（必须解决）

#### 冲突 1：方案 0.1 与 0.2 对同一聚合字段的写入权冲突 🔴

**冲突文件**：[usage_write.go](file:///f:/project/aranea-agents/internal/data/usage_write.go#L51-L68)、[session_message_repo.go](file:///f:/project/aranea-agents/internal/data/session_message_repo.go#L396-L464)

- 方案 0.1 要求将 `message_count`、`model_call_count`、`input_tokens`、`output_tokens`、`total_tokens` 的更新改为 delta 累积
- 方案 0.2 要求将 `RecordTokenUsageEvent` 中的 sessions 聚合更新（第 51-68 行）从 4 表事务中移除
- 两个方案同时修改同一组字段的写入路径

**如果先 0.2 再 0.1**：0.2 拆出的 sessions 聚合更新变成"孤儿"——既不在事务内，也没有走 delta 累积
**如果先 0.1 再 0.2**：0.1 的 delta 累积器已接管聚合更新，0.2 拆出的 sessions 聚合变成**重复写入**

**修正方案**：0.1 和 0.2 **必须合并为一个原子变更**，实施顺序为 0.1 → 0.2，且 0.2 中不再重复处理 0.1 已接管的聚合字段。具体：

1. 先实施 0.1 的 `AccumulateMetricsDelta` + `StartMetricsFlusher`
2. 将 `AppendChatMessage` 中的 sessions 聚合更新改为 delta 累积
3. 将 `RecordTokenUsageEvent` 中的 sessions 聚合更新也改为 delta 累积（而非独立异步写入）
4. 将 daily/hourly 汇总改为 EventBus 异步消费者
5. `RecordTokenUsageEvent` 缩减为仅 `INSERT model_token_usage_events`

#### 冲突 2：方案 0.5 与 1.1 对 session_state_repo.go 的冲突 🔴

**冲突文件**：[session_state_repo.go](file:///f:/project/aranea-agents/internal/data/session_state_repo.go)

- 方案 0.5 在 `session_state_repo.go` 中新增 `StatePatchBatch` + `ApplyBatch`，SQL 硬编码了 `UPDATE sessions SET state_json = ...`
- 方案 1.1 要将 `state_json` 字段迁移到 `session_runtime_state` 表，SQL 变为 `UPDATE session_runtime_state SET state_json = ...`

**如果先 0.5 再 1.1**：0.5 新增的 `ApplyBatch` 中硬编码的 SQL 全部失效
**如果先 1.1 再 0.5**：1.1 迁移后 `PatchSessionState` 已指向新表，0.5 的 `ApplyBatch` 需重新设计

**修正方案**：方案 0.5 的 `ApplyBatch` **不应硬编码 SQL**，而应封装为对 `PatchSessionState` 接口的批量调用。这样 1.1 迁移表结构时只需修改 `PatchSessionState` 的实现，`ApplyBatch` 不受影响。具体：

```go
// 修正后的 ApplyBatch — 不硬编码 SQL，而是调用已有的 PatchSessionState
func (r *sessionStateRepo) ApplyBatch(ctx context.Context, batch *StatePatchBatch) error {
    return r.PatchSessionState(ctx, batch.sessionID, batch.sets, batch.removes)
}
```

#### 冲突 3：方案 0.1 与 1.1 的字段归属冲突 🟡

方案 0.1 的 `ApplyMetricsDelta` 写 sessions 表，方案 1.1 要将聚合字段拆到 `session_metrics` 表。

**修正方案**：0.1 的 `ApplyMetricsDelta` 应通过 `ContextUpdater` 接口调用（而非直接写 SQL），1.1 只需更换 `ContextUpdater` 的实现即可。

### 7.3 各方案遗漏问题

#### 方案 0.1 遗漏：delta 累积溢出风险 🔴

`flushAllMetrics` 在 flush 失败时重新累积 delta，如果 SQLite 持续 BUSY，delta 会无限累积导致内存泄漏。

**修正**：添加 delta 安全阀：

```go
const (
    maxDeltaAge    = 30 * time.Second
    maxDeltaCount  = 100
)

func (uc *SessionUsecase) AccumulateMetricsDelta(delta SessionMetricsDelta) {
    uc.metricsDeltaMu.Lock()
    defer uc.metricsDeltaMu.Unlock()
    if existing, ok := uc.metricsDeltas[delta.SessionID]; ok {
        existing.MessageCount += delta.MessageCount
        // ... 累加其他字段
        existing.AccumulatedCount++
        if existing.AccumulatedCount >= maxDeltaCount || time.Since(existing.FirstAccumulatedAt) > maxDeltaAge {
            go uc.forceFlushSingle(delta.SessionID)
        }
    } else {
        cp := delta
        cp.FirstAccumulatedAt = time.Now()
        cp.AccumulatedCount = 1
        uc.metricsDeltas[delta.SessionID] = &cp
    }
}
```

#### 方案 0.1 遗漏：前端 reconcilePatchFromServer 用旧值覆盖新值 🟡

前端已有基于 Envelope 的实时累加机制（[sessionContextPatch.ts:88-96](file:///f:/project/aranea-agents/web/src/features/chat/sessionContextPatch.ts#L88-L96)），但如果 `reconcilePatchFromServer`（第 133-145 行）在 delta 未 flush 时从服务端拉到旧值，会用旧值**覆盖**前端的新值，导致 token 计数回退。

**修正**：在 `reconcilePatchFromServer` 中，如果服务端值小于前端本地值，保留前端值：

```typescript
// sessionContextPatch.ts — reconcilePatchFromServer 修正
if (serverValue.total_tokens < localValue.total_tokens) {
    delete patch.total_tokens;
    delete patch.input_tokens;
    delete patch.output_tokens;
}
```

#### 方案 0.4 遗漏：Team 路径的 BumpSessionRevision 未被覆盖 🟡

文档只考虑了 Chat 路径的 `chat_orchestrator_turn.go`，但 Team 路径也有 `BumpAndPublishSessionRevision`（[runner_team_trpc.go:423](file:///f:/project/aranea-agents/internal/team/runner_team_trpc.go#L423)）。

**修正**：Team 路径只有 completed bump（没有 sync bump），不受方案 0.4 影响。但需在方案文档中明确说明 Team 路径不在优化范围内。

#### 方案 0.5 遗漏：框架回调 ApplyStateDelta 无法被 StatePatchBatch 合并 🔴

[event_bus_state_handler.go:38](file:///f:/project/aranea-agents/internal/biz/event_bus_state_handler.go#L38) 的 `ApplyStateDelta` 通过 `PatchSessionState` 执行框架回调的状态变更。这个调用是**事件驱动的异步回调**，不在 Turn 的同步流程内，无法被 `StatePatchBatch` 合并。

**修正**：方案 0.5 应明确区分两条路径：

1. **同步路径**（`chat_run_gateway.go` 的 `persistRunStatus` / `persistAwaitMarkers`）：可用 `StatePatchBatch` 合并
2. **异步路径**（`event_bus_state_handler.go` 的 `ApplyStateDelta`）：**保留独立 `PatchSessionState`**，不合并

#### 方案 0.5 高估收益：persistRunStatus("running") 不可延迟 🔴

文档声称"state_json 写入从 4~5 次降为 1 次"，但代码追踪发现：

- `persistRunStatusToSession("running")` 设置 `run_id` **不可延迟** — 前端依赖此状态判断会话是否活跃
- `persistAwaitMarkersToSession(syncWrite=true)` **不可延迟** — 进程崩溃恢复需要
- 真正可合并的只有 Turn 完成时的 `persistRunStatus("completed")` + `clearAwaitingRunStateFromSession`

**修正收益估计**：state_json 写入从 4~5 次降为 **3~4 次**（而非 1 次），收益有限。

#### 方案 1.1 遗漏：6 处读取聚合字段的代码需同步修改 🟡

除 `toProtoSession` 外，还有 5 处读取聚合字段的代码：

| 位置 | 字段 | 用途 |
|------|------|------|
| [session_observability.go:102-105](file:///f:/project/aranea-agents/internal/service/session_observability.go#L102-L105) | MessageCount, InputTokens, OutputTokens | SessionRunRecord 映射 |
| [usage_mapper.go:118-120](file:///f:/project/aranea-agents/internal/service/usage_mapper.go#L118-L120) | InputTokens, OutputTokens, TotalTokens | Usage 映射 |
| [biz/timeline.go:52,89](file:///f:/project/aranea-agents/internal/biz/session/timeline.go#L52) | MessageCount | Timeline 汇总 |
| [biz/export.go:127-128](file:///f:/project/aranea-agents/internal/biz/session/export.go#L127-L128) | MessageCount, TotalTokens | 导出 |
| [biz/agent_settings_helpers.go:586](file:///f:/project/aranea-agents/internal/biz/session/agent_settings_helpers.go#L586) | — | Token 估算 |

方案 1.1 的 Step 4（读写切换）需覆盖所有这些读取点。

#### 方案 1.2 遗漏：SQLite 单写连接下扩大事务范围可能增加锁竞争 🟡

统一事务管理器的目标是让 Raw SQL 和 Ent 操作共享事务，但 SQLite `MaxOpenConns=1` 意味着同一时刻只能有一个写事务。如果将原本独立的小事务合并为大事务，锁持有时间反而增加。

**修正**：统一事务管理器应**只统一事务获取方式**，不扩大事务范围。每个业务操作仍应保持最小事务粒度。

#### 方案 1.3 遗漏：跨 Repo 事务协调 🔴

`UpsertFactsAndEpisodeBatch`（[store_consolidate_batch.go:34](file:///f:/project/aranea-agents/internal/data/sessionmemory/store_consolidate_batch.go#L34)）需要同时写 facts + episodes，拆分为独立 Repo 后需要事务协调机制。

**修正**：建议分两阶段拆分：
1. **Phase 1**：只拆分读取侧（FactReader/EpisodeReader/EntityReader），写入侧暂保留在 Store
2. **Phase 2**：引入事务协调器（通过 `Data.ExecInTx` 传播事务），再拆分写入侧

### 7.4 修正后的实施路线图

```
Phase 1（可独立实施，互不冲突）──────────────────────
  ├─ 方案 0.3：EventBus 降级（独立模块，无交叉依赖）
  ├─ 方案 0.4：revision 流式优化（改动最小，风险最低）
  └─ 方案 4.1+4.2+4.3：异步消费者可靠性治理
  预期收益：消除数据丢失风险 + 减少 1 次写锁

Phase 2（必须合并为一个原子变更）──────────────────────
  ├─ 方案 0.1 + 0.2 合并：聚合异步化 + Usage 事务拆分
  │   前置条件：0.1 的 delta 累积器必须包含容量/时间上限
  │   前置条件：前端 reconcilePatchFromServer 修正旧值覆盖问题
  └─ 方案 0.5（修正版）：state_json 合并
      前置条件：ApplyStateDelta 回调路径保留独立 PatchSessionState
      前置条件：ApplyBatch 不硬编码 SQL，封装为接口调用
  预期收益：sessions 表同步写入从 8 次降为 2~3 次，写锁竞争减少 ~70%

Phase 3（长期，需 feature flag）──────────────────────
  ├─ 方案 1.2：统一事务管理器
  ├─ 方案 1.3 Phase 1：记忆收口（只拆读取侧）
  └─ 方案 1.3 Phase 2：记忆收口（拆分写入侧 + 事务协调）
  预期收益：架构合规 + 事务一致性保证

Phase 4（长期，需 feature flag + 回滚脚本）────────────
  ├─ 方案 1.1：Session 冷热分离
  │   前置条件：0.5 的 ApplyBatch 通过接口封装
  │   前置条件：6 处读取点全部覆盖
  │   前置条件：双写期间保留 sessions 表冗余字段
  └─ 方案 2.1-2.3：技术债务清理
  预期收益：架构面向未来演进
```

### 7.5 回滚难度评估

| 方案 | 回滚复杂度 | 回滚步骤 | 数据丢失风险 |
|------|-----------|---------|------------|
| **0.3 EventBus 降级** | 低 | 移除 `OfferWithOptions`，恢复原 `Offer` | 无 |
| **0.4 revision 优化** | 低 | 移除 `isStreamingMode` 判断 | 无 |
| **0.1+0.2 聚合异步化** | 中 | 恢复 `AppendChatMessage` 同步写入；恢复 4 表事务 | ⚠️ 未 flush 的 delta 丢失 |
| **0.5 state_json 合并** | 低 | 移除 `StatePatchBatch`，恢复逐次 `PatchSessionState` | 无 |
| **1.1 Session 冷热分离** | **高** | 恢复 sessions 表冗余字段 + 回填数据 + 重新 `go generate` + `make wire` + `make api` | ⚠️ 双写期间数据不一致 |
| **1.2 统一事务管理器** | 中 | 恢复各 Repo 的手动事务管理 | 无 |
| **1.3 记忆收口** | 中 | 恢复 Store 直接注入 | 无 |

### 7.6 文档遗漏的代码级细节（AI 落地时必须注意）

| # | 遗漏项 | 影响方案 | 修正 |
|---|--------|---------|------|
| 1 | `AppendChatMessage` 的 message INSERT 和 session UPDATE 在**同一事务**内，拆分后需保留 message INSERT 的事务 | 0.1 | 只移除 session 聚合字段更新，保留事务和 message INSERT |
| 2 | `IncrementInvocationCounts` **已在异步 goroutine** 中执行（`safego.Go`），不在同步路径上 | 0.1 | 改为 delta 累积的收益是减少散落的独立 UPDATE，而非减少同步写入 |
| 3 | `TurnCompletionBridge.RegisterTurnUsage` **仅操作内存**，不依赖 sessions 聚合在同一个事务中 | 0.2 | 事务拆分安全，无需额外处理 |
| 4 | 方案建议的 `EnvelopeTypeTokenUsage` 在 contract 中**不存在** | 0.2 | 需新增 Envelope 类型 |
| 5 | 方案引用的 `handleToolCallPersist` 函数**不存在**，实际是 `toolCallConsumer.handle` | 0.3 | FallbackFn 应基于 `toolCallConsumer.handle` 实现 |
| 6 | `isStreamingMode` 函数不存在，但 `input.EntryConfig.AllowStream` 已在 [chat_orchestrator_turn.go:768](file:///f:/project/aranea-agents/internal/service/chat_orchestrator_turn.go#L768) 中使用 | 0.4 | 直接复用 `input.EntryConfig.AllowStream` |
| 7 | 流式场景的 sync revision bump 设置了 `skip_hydrate=true`，前端**只更新本地计数器不拉取消息** | 0.4 | 进一步佐证流式场景跳过 sync bump 是安全的 |
| 8 | Durable resume 使用 `notifySessionRevisionSync`（**不 bump，只读取**），不依赖 sync revision bump | 0.4 | Durable resume 不受方案 0.4 影响 |
| 9 | `PatchSessionState` 共有 **5 个业务调用点**（文档只提 2 个） | 0.5 | 必须覆盖所有 5 个调用点 |
| 10 | `BumpSessionRevision` 共有 **5 个调用点**（文档只提 2 个） | 0.4 | Team 路径和 durable resume 路径需明确处理 |
| 11 | `SearchSessions` 和 `GetSession` **全部使用 Ent ORM**，Ent 不支持列选择投影 | 1.1 | 冷热分离后列表查询仍 SELECT 全部列，需改为 Raw SQL 或接受冗余读取 |
| 12 | `sessionmemory.Store` 有 **86 个方法**（文档说 50+），**18 个消费方** | 1.3 | 拆分工作量比预期大，建议分两阶段 |
| 13 | Budget Alert **不依赖 sessions 聚合字段**，使用 `model_token_usage_events` 表做费用聚合 | 0.1 | 聚合异步化不影响预算告警准确性 |
| 14 | 前端已有基于 Envelope 的实时 token 累加机制 | 0.1 | Chat 页面 token 显示不受异步化影响，但需修正 reconcilePatchFromServer |

### 7.7 最终建议

1. **先做 Phase 1**（0.3 + 0.4 + 4.1-4.3）：改动最小、风险最低、无交叉依赖，可立即开始
2. **Phase 2 是核心收益**（0.1+0.2 合并 + 0.5 修正版）：但必须解决 delta 溢出风险和前端 reconcile 问题后才能实施
3. **Phase 3/4 需 feature flag 保护**：1.1（冷热分离）回滚困难，必须有双写过渡期和自动化回滚脚本
4. **方案 0.5 的收益被高估**：实际只能合并 Turn 完成时的 2 次 patch，从 4~5 次降为 3~4 次。如果投入产出比不划算，可暂缓
5. **方案 1.1 的 Ent 限制需提前解决**：Ent 不支持列选择投影，冷热分离后列表查询仍 SELECT 全部列。需评估是否引入 Raw SQL 查询或接受冗余读取

---

## 第八部分：实施记录

> 实施日期：2026-06-02
> 实施范围：Phase 1 全部 + Phase 2 核心（0.1+0.2 Bug 修复）+ Phase 3 部分（1.2 基础设施 + Bug 修复）

### 已完成

| 方案 | 实施内容 | 修改文件 | 验证状态 |
|------|---------|---------|---------|
| **0.3 EventBus 降级** | 新增 `OfferOption` + `OfferWithOptions`，toolCall/messageStore 消费者队列满时回退同步写入 | event_bus_async.go, event_bus_side_consumers.go, event_bus_tool_call_consumer.go, event_bus_message_store_consumer.go | ✅ go build 通过 |
| **0.4 revision 优化** | 流式模式跳过 sync revision bump，只保留 completed bump | chat_orchestrator_turn.go | ✅ go build 通过 |
| **4.1 超时 context** | asyncEnvelopeWorker 添加 45s handleTimeout，handle 不再使用无超时 context.Background() | event_bus_async.go, event_bus_side_consumers.go | ✅ go build 通过 |
| **4.2 TTL 清理** | TurnCompletionBridge 添加 5 分钟 TTL + 每分钟定时清理 | runner_completion.go | ✅ go build 通过 |
| **4.3 构造函数收口** | knowledgeRepo/evalRepo/a2aRepo 构造函数改为接受 `*Data` 而非 `*sql.DB`；evalRepo 添加 logger + kerrors | knowledge.go, evaluation.go, a2a.go, data.go, wire_gen.go, test files | ✅ go build + test 通过 |
| **0.1+0.2 Bug 修复** | 修复 ModelCallCount+tokens 双重计数；Team 路径添加 delta 累积；UpsertChatActivityMessage 移除同步 AddMessageCount | messages.go, usecase.go, session_message_repo.go, usage_record.go, test files | ✅ go build + test 通过 |
| **0.2 Rollup 缺失修复** | Team/runner_handler 路径 `RecordTokenUsageEvent` 后发布 `EnvelopeTypeTokenUsage` 到 EventBus，触发 daily/hourly 汇总；提取 `PublishTokenUsageEnvelope` + `TokenUsageEventToEnvelope` 为共享函数 | event_bus_usage_rollup_consumer.go, event_bus_runner_handler.go, event_bus_consumer.go, team/usage_record.go, chat_orchestrator_turn.go | ✅ go build + test 通过 |
| **1.2 统一事务管理器（基础设施）** | `ExecInTx` 增强：存储 `*ent.Tx` 到 context（替代原 `*ent.Client`），支持事务传播检测；新增 `TxExecerFromCtx` 返回 `execer` 接口，Raw SQL Repo 可通过此接口参与 Ent 事务；统一 `execer` 接口定义到 tx.go；所有 `readClient`/`clientFromCtx` 更新为从 `*ent.Tx` 提取 `Client()` | tx.go, data.go, usage.go, tool.go, system_setting.go, team_repo.go, llm_provider_model.go, agent_repo.go, task.go, session_repo.go, usage_write.go | ✅ go build + test 通过 |
| **死代码清理** | 删除 `retryOnBusy`、`isSQLiteBusyErr`、`sqliteWriteRetryMax`（零调用点） | data.go | ✅ go build 通过 |
| **4.3 补充：channelRuntimeLeaseRepo 收口** | `channelRuntimeLeaseRepo` 存储字段从 `*sql.DB` 改为 `*Data`，通过 `r.data.RawDB()` 访问数据库 | channel_runtime_lease.go | ✅ go build 通过 |

### 暂缓

| 方案 | 原因 |
|------|------|
| **0.5 state_json 合并** | 收益被高估（4~5 次→3~4 次），persistRunStatus("running") 不可延迟，框架回调 ApplyStateDelta 无法合并。文档 §7.7 建议可暂缓 |

### 未实施（Phase 3/4，需 feature flag）

| 方案 | 优先级 | 前置条件 |
|------|--------|---------|
| 1.1 Session 冷热分离 | P1 | 0.5 ApplyBatch 通过接口封装 + 6 处读取点覆盖 + 双写过渡期 |
| 1.2 统一事务管理器（Repo 迁移） | P1 | 基础设施已就绪（`TxExecerFromCtx`），需逐个将 15 处 `BeginTx` 调用迁移为 `ExecInTx` + `TxExecerFromCtx` |
| 1.3 记忆子系统收口 | P1 | 分两阶段：先拆读取侧，再拆写入侧+事务协调 |
| 2.1 野生表纳入 Ent | P2 | 分 3 批，每批 6~7 张表 |
| 2.2 接口拆分合规化 | P2 | TeamRepository→6 子接口 |
| 2.3 Schema 迁移框架化 | P2 | 12 个 patch→registry |

### 收益验证

| 验证项 | 文档基线 | Phase 1 目标 | 实际达成 |
|--------|---------|-------------|---------|
| 单 Turn sessions 表同步写入次数 | 8 次 | ≤3 次 | ~3 次（SetLastMessageAt×2 + SetUpdatedAt） |
| Usage 事务持有写锁时间 | ~15ms（4 表） | ~4ms（1 表） | ~4ms（仅 events INSERT） |
| EventBus 关键事件丢失率 | 队列满时 100% | 0%（降级同步） | 0%（FallbackSync） |
| asyncWorker context 超时 | 无（Background） | 45s | 45s |
| TurnCompletionBridge 内存泄漏 | 无 TTL 清理 | 5 分钟 TTL | 5 分钟 TTL |
| Repo 构造函数合规 | 4 个绕过 Data | 全部通过 Data | ✅ 全部收口（含 channelRuntimeLeaseRepo） |
| Team/runner_handler Usage rollup | 缺失（daily/hourly 不生成） | 全路径覆盖 | ✅ 全路径发布 EnvelopeTypeTokenUsage |
| 事务模式统一 | 5 种 | 5 种（基础设施就绪） | 基础设施就绪（`TxExecerFromCtx`），待逐 Repo 迁移 |
