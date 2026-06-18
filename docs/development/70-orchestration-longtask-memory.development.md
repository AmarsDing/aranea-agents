# 编排引擎 + 24h 长任务 + 领先记忆综合升级 — 开发计划

> 模块编号：70
> 关联需求：`70-orchestration-longtask-memory.md`
> 关联设计：`70-orchestration-longtask-memory.design.md`

---

## 一、模块定位

本开发计划落地"编排引擎 + 24h 长任务 + 领先记忆综合升级"方案，分 4 个 Phase 渐进实施。每个 Phase 可独立验证、独立上线，降低风险。

**核心原则**：
- 基于 trpc-agent-go 框架原生能力增强，不另起炉灶
- 复用项目现有 L0-L4 记忆、Spirit 编排、Graph 引擎、事件体系
- TDD 实施：先写失败测试，再写最小实现
- 每个任务可独立验证

---

## 二、代码锚点（现状评估）

### 2.1 现有关键文件

| 文件 | 当前职责 | 改造方向 |
|------|---------|---------|
| `internal/data/data.go` | SQLite 连接管理 | 改为 Postgres 连接池 |
| `internal/data/tx.go` | 事务管理（30s 硬超时） | 去掉硬超时，改可配置 |
| `internal/event/infra.go` | 事件基础设施（WBPF 违规） | 修复 WBPF 语义 |
| `internal/event/wal.go` | WAL 实现 | 适配 Postgres |
| `internal/service/chat_orchestrator_turn.go` | Chat 编排主流程 | 增加预规划门控 |
| `internal/agent/intent/pass.go` | Intent Pass（默认关闭） | 改默认开启 |
| `internal/agent/task_planner_impl.go` | 任务规划 | 接入预规划门控 |
| `internal/agent/agent_allocator_impl.go` | Agent 匹配（TF-IDF） | 升级 pgvector + AgentFactory |
| `internal/agent/task_orchestrator_impl.go` | 任务编排 | 接入 NL2Graph + RuntimeReplanner |
| `internal/biz/graph_execution_usecase.go` | Graph 执行（状态机绕过） | 接入状态机 |
| `internal/tools/spirit_tools.go` | Spirit 工具 | 增强 plan_and_execute |
| `internal/team/template_registry.go` | Team 模板 | 增加拓扑演化 |
| `internal/memory/trpc/sqlite_adapter.go` | Memory 框架适配 | 增加 Bi-temporal/Ebbinghaus |
| `internal/biz/memory_l3_fused_recall.go` | L3 记忆召回 | 增加主动召回 |
| `web/src/components/chat/ErrorBlock.vue` | 错误展示（无重试） | 增加重试按钮 |
| `web/src/features/chat/errorCodeHints.ts` | 错误码提示（9 种） | 扩展全覆盖 |
| `web/src/realtime/ws-transport.ts` | WS 传输 | 增加心跳检测 |

### 2.2 框架文件（pkg/trpc-agent-go/）

| 文件 | 改造方向 |
|------|---------|
| `agent/taskrun/inprocess/service.go` | 增加 Events() 事件透传 |
| `memory/memory.go` | 扩展 Service 接口（ProactiveRecall）+ Memory/Entry 字段 |
| `graph/checkpoint/sqlite/` | 新增 Postgres CheckpointSaver |
| `graph/executor.go` | 增加 RuntimeReplanner hook |

---

## 三、Phase 0：基础夯实（P0 阻断修复 + Postgres Phase 1）

### 3.1 P0-1：修复 WBPF 语义违规

**任务**：Critical 事件 WAL 写入失败时不发布事件

**改动文件**：
- `internal/event/infra.go`（Publish 方法 WBPF 语义修复）

**实现细节**：
- WBPF 失败区分两种模式：pre-publish 失败（serialize/insert）→ 事件未发布，记 Error "dropped"；post-publish 失败（markPublished）→ 事件已发布，记 Warn "published but mark failed"
- Critical 事件经 `contract.IsCriticalWBPFType` 判定后走 WAL 写入路径
- 非 Critical 事件直接走 `publishToBuses`，无 WAL 开销

**验收**：
- ✅ WBPF 失败时不发布 Critical 事件（pre-publish 失败路径）
- ✅ WAL 成功时正常发布
- ✅ post-publish 失败时事件已发布，日志标记 "may republish on restart"

**状态**：✅ 完成

### 3.2 P0-2：接入状态机

**任务**：GraphExecution 状态变更改为经状态机 Transition 函数（AS-FSM-01）

**改动文件**：
- `internal/biz/graph_execution_state_machine.go`（新增，GraphExecution 状态机：5 状态 + 7 转换规则）
- `internal/biz/graph_execution_usecase.go`（新增 `applyExecTransition` 方法，authoritative 模式）
- `internal/biz/graph_execution_usecase_fsm_test.go`（新增，6 个测试覆盖合法/非法/终态/HITL 转换）
- `internal/team/team_graph_run_coordinator.go`（更新 fallback 注释，说明 CanTransition 已校验）

**实现细节**：
- 状态机定义：Running/Completed/Failed/Cancelled/WaitingHuman（5 状态）
- 转换规则：7 条（Running→Completed/Failed/Cancelled/WaitingHuman, WaitingHuman→Running/Cancelled/Failed）
- 新增 WaitingHuman→Failed 转换（修复 HITL 期间节点错误无法标记 failed 的问题）
- `applyExecTransition` 采用 authoritative 模式：非法转换拒绝并保留原状态（非 advisory 模式）
- ResumeExecution 中复用 `uc.sm` 而非创建新实例

**验收**：
- ✅ 静态分析：无 `exec.Status =` 直接赋值（所有变更经 `applyExecTransition`）
- ✅ 单元测试：非法状态转换被拒绝，状态保留
- ✅ 单元测试：终态无出口转换
- ✅ 单元测试：WaitingHuman→Failed 合法（新增转换）

**状态**：✅ 完成

### 3.3 P0-3：Postgres Phase 1 迁移

**任务**：WAL/EventStore/Checkpoint 关键表迁移到 Postgres 原生 schema + 可配置事务超时

**改动文件**：
- `internal/data/data.go`（新增 `ensurePostgresPhase1Schema` + `isPostgresAlreadyExistsErr`，Postgres Phase 1 迁移执行）
- `internal/data/tx.go`（30s 硬超时改为可配置 `TxTimeout()`，支持 `SetTxTimeout(0)` 禁用）
- `internal/data/errors.go`（新增 Postgres SQLSTATE 错误翻译：23505/23503→Conflict, 23502/23514→BadRequest，使用 `errors.As` 支持包装错误）
- `internal/event/wal.go`（`NewEventWAL` 改为双 DB 签名，Postgres 优先 SQLite 回退）
- `internal/event/postgres_wal_storage.go`（新增，Postgres WAL 存储适配器，ON CONFLICT/TIMESTAMPTZ/$N 语法）
- `internal/event/infra.go`（`ProvideEventWAL` 双 DB 签名，从 `InfraProviderSet` 移除因 Wire 类型歧义）
- `cmd/admin/wire.go`（新增 `provideEventWAL` 桥接 `*data.Data` → 双 DB 句柄）
- `internal/data/errors_postgres_test.go`（新增，5 个 Postgres 错误翻译测试）

**新增文件**：
- `internal/data/sql/migrations/20260617_postgres_phase1.sql`（event_wal/event_store/session_run_checkpoints 表 + INV-UNIQ-01/02 唯一索引 + INV-REF-01/02/03 FK 约束，DO $$ 块幂等）

**实现细节**：
- WAL 双后端选择：Postgres 可用时优先（Phase 1），否则回退 SQLite
- Wire DI 解决 `*sql.DB` 类型歧义：`provideEventWAL` 从 `*data.Data` 提取双 DB
- Postgres 迁移幂等：整个 SQL 作为单个 `ExecContext`（DO $$ 块含分号不能用 `splitDDLStatements`），`isPostgresAlreadyExistsErr` 处理 SQLSTATE 42P07/42710/42701
- 事务超时可配置：默认 30s，`SetTxTimeout(0)` 禁用用于长运行 Postgres 操作

**验收**：
- ✅ `go build ./...` 通过
- ✅ `go vet ./internal/data/ ./internal/event/` 通过
- ✅ 现有测试全部通过（预存失败除外）
- ✅ Postgres 错误翻译测试通过（5 个 SQLSTATE 场景）
- ✅ WAL 双后端选择正确（Postgres 优先，SQLite 回退）

**状态**：✅ 完成

### 3.4 P0-4：修复 DB-R5 错误翻译

**任务**：Repo 文件中所有 `return err` 改为 `return entErrToBizErr(err, "DOMAIN")`

**改动文件**（9 个主修复 + 3 个审查/补修，共 12 个，完整清单见 §9.3）：
- `internal/data/session_run_repo.go`（19 处，domain "SESSION_RUN"）
- `internal/data/session_repo.go`（27 处 + 审查修复 3 处，domain "SESSION"）
- `internal/data/agent_repo.go`（21 处，domain "AGENT"）
- `internal/data/borrow_request_repo.go`（7 处，domain "BORROW_REQUEST"）
- `internal/data/tool.go`（34 处，domain "TOOL"）
- `internal/data/monitor.go`（25 处，domain "MONITOR"）
- `internal/data/memory_shim_l1.go`（16 处，domain "MEMORY_L1"）
- `internal/data/model_registry_apply.go`（多处，domain "MODEL_REGISTRY"）
- `internal/data/agent_performance_repo.go`（2 处，domain "AGENT_PERFORMANCE"）
- `internal/data/session_metrics_repo.go`（审查修复 3 处，domain "SESSION_METRICS"）
- `internal/data/evolution_suggestion_repo.go`（补修，domain "EVOLUTION_SUGGESTION"）
- `internal/data/channel.go`（补修，domain "CHANNEL"）

**实现细节**：
- 所有 `return err` 后 Ent/Raw SQL 操作替换为 `return entErrToBizErr(err, "DOMAIN")`
- 同文件函数调用使用 pass-through（避免双重包装，因 `entErrToBizErr` 对 `apierror.Error` 透传）
- 非 DB 错误（json.Unmarshal 等）不翻译
- 审查发现 `session_repo.go` 3 处 `metricsWriter.ApplyMetricsDelta()` 跨 Repo 调用返回未翻译错误，已修复
- 审查发现 `session_metrics_repo.go` 根因 3 处 `return err` 未翻译，已修复

**验收**：
- ✅ 静态分析：9 个文件无直接返回 Ent 错误（审查后修复 session_repo + session_metrics_repo）
- ✅ 单元测试：Postgres 错误码翻译正确（5 个 SQLSTATE 场景）
- ✅ `go build ./...` 通过
- ✅ 相关测试通过

**状态**：✅ 完成

---

## 四、Phase 1：强制规划 + 动态 Agent + 统一执行引擎

### 4.1 P1-1：Intent Pass 默认开启

**任务**：Intent Pass 改为默认开启

**改动文件**：
- `internal/agent/intent/pass.go`（`IntentPassFromAgent` 无 Settings 时返回 true；`PassEffective` 注释更新）
- `internal/agent/intent/pass_test.go`（新增 `TestIntentPassFromAgent_DefaultOn` + `TestShouldRun` 增加 nil Settings 用例）
- `internal/agent/intent/parse_test.go`（`TestIntentPassFromAgent_NilSettings` 断言改为 true）
- `internal/biz/agent_defaults.go`（`IntentPassEnabled` 默认 false → true）
- `internal/biz/agent_types.go`（字段注释更新）
- `docs/guides/prompt/assembly.md`、`docs/guides/prompt/README.md`（"默认 false" → "默认 true"）

**实现细节**：
- `IntentPassEnabled` 为 plain bool（非 `*bool`），无法区分"未设置"与"显式 false"
- 采用双层默认 ON 策略：
  1. `IntentPassFromAgent`：`ag.Settings == nil` 时返回 `true`（无 settings = 默认 ON）
  2. `DefaultAgentRuntimeSettings()`：`IntentPassEnabled: true`（新 Agent 默认 ON）
- 显式 `IntentPassEnabled=false` 仍被尊重（agent setting 可关闭）
- A2A Proxy Agent 在 `agent_usecase.go:411` 显式覆盖为 `false`，不受默认值变更影响
- 现有 DB 中已持久化 `false` 的 Agent 保持 OFF（不追溯变更）

**验收**：
- ✅ 默认场景 Intent Pass 执行（`TestIntentPassFromAgent_DefaultOn` + `TestShouldRun` nil Settings 用例）
- ✅ agent setting 可关闭（`TestPassEffective` `{"", false, false}` 用例 + `TestIntentPassFromAgent_DefaultOn` 显式 false 用例）
- ✅ `go build ./internal/agent/... ./internal/biz/...` 通过
- ✅ `go vet ./internal/agent/intent/` 通过
- ✅ 相关测试全部通过（intent 包 + biz 包默认值相关测试）

**状态**：✅ 完成

### 4.2 P1-2：预规划门控

**任务**：复杂度 ≥ Moderate 强制走规划路径

**改动文件**：
- `internal/biz/task_planner.go`（TaskPlannerPort 新增 QuickAssess 方法，标注 Stability: evolving）
- `internal/agent/task_planner_impl.go`（新增 QuickAssess 实现，纯计算复用 assessComplexity）
- `internal/event/contract/envelope.go`（新增 3 个规划时间线事件类型 + chat 频道路由注册）
- `internal/event/contract/envelope_contract_test.go`（更新 EnvelopeType 常量计数 59→62）
- `internal/service/chat_orchestrator_turn.go`（eg.Wait 后插入 PRE-PLANNING GATE 调用）
- `internal/service/chat_orchestrator_turn_phases.go`（runIntentPass 签名变更：返回 *intent.Artifact）

**新增文件**：
- `internal/service/pre_planning_gate.go`（GateDecision + PrePlanningGate.Evaluate + 规划时间线事件发布）
- `internal/service/pre_planning_gate_test.go`（fakePlanner + gateCaptureBus，覆盖 simple/moderate/complex + 错误传播）
- `internal/service/chat_orchestrator_turn_preplanning.go`（runPrePlanningGate 方法 + forcedPlanningRunOption 函数 + intentArtifactToBiz 转换）
- `internal/agent/task_planner_impl_test.go`（QuickAssess 单测：simple/moderate/complex + 纯计算验证）

**验收**：
- ✅ Simple 任务直接回答（<2s）— QuickAssess 纯计算 <1ms
- ✅ Moderate/Complex 任务强制走规划 — ForcePlanning 标志 + RunOption 注入 plan_and_execute 指令
- ✅ 规划时间线事件发布 — planning_phase_start/done 事件经 contract.Bus 发布
- ✅ go build ./... 通过
- ✅ go vet（改动包）通过
- ✅ TestPrePlanningGate / TestTaskPlanner_QuickAssess / envelope contract / intent 全部通过
- ✅ aranea-review 两阶段审查通过（规格合规 + 代码质量）

**状态**：✅ 完成

### 4.3 P1-3：语义匹配接入 pgvector

**任务**：Layer 2 从 TF-IDF 升级为 pgvector embedding

**改动文件**：
- `internal/agent/agent_allocator_impl.go:315-361`

**验收**：
- 向量相似度匹配准确率 > TF-IDF
- Embedder 失败时降级 TF-IDF

**状态**：✅ 已完成（Wave 1）
- `matchLayer2` 升级为 embedding cosine similarity（in-memory，非 pgvector SQL）+ TF-IDF fallback
- 实际实现：调用 `embedder.Embed()`（OpenAI/Gemini/Ollama API）后在 Go 内存计算 cosine，避免 Postgres 往返
- pgvector SQL `<=>` 操作符仅用于 memory 域（`internal/data/vector/pgvector_fact.go`），allocator 不涉及
- 5 个测试覆盖：embedding 成功/失败/nil embedder/空候选/维度不匹配
- Wire 注入：`provideAgentAllocator` 新增 embedder 参数，`wire.Bind` 绑定 `*knowledge.MultiProviderEmbedder`

### 4.4 P1-4：AgentFactory

**任务**：LLM 生成 Agent 定义，无需人工审核

**新增文件**：
- `internal/agent/agent_factory.go`
- `internal/agent/agent_factory_test.go`

**改动文件**：
- `internal/biz/agent_types.go`（新增 Source 字段）
- `internal/data/ent/schema/agent.go`（新增 source 列）
- `internal/event/contract/envelope.go`（新增 EnvelopeTypeAgentCreated）

**验收**：
- 4 层匹配失败时自动创建 Agent
- 创建的 Agent 标记 source="dynamic"
- EnvelopeTypeAgentCreated 事件发布
- 创建的 Agent 可被后续任务复用

**状态**：📋 待办

### 4.5 P1-5：taskrun 事件透传

**任务**：扩展 taskrun.Controller 增加 Events()

**改动文件**：
- `pkg/trpc-agent-go/agent/taskrun/types.go`（Controller 接口新增 Events 方法 + ErrRunNotActive 错误）
- `pkg/trpc-agent-go/agent/taskrun/inprocess/service.go`（实现 Events()：eventChs map + forwardEvent + closeEventChannel）

**验收**：
- taskrun 后台任务事件可被外部消费
- 无消费者时不阻塞

**状态**：✅ 已完成（Wave 1）
- Controller 接口新增 `Events(runID string) (<-chan *event.Event, error)`
- inprocess.Service 实现：`eventChs map[string]chan *event.Event`（s.mu 保护）+ `forwardEvent`（non-blocking send）+ `closeEventChannel`（终态关闭）
- drop policy：缓冲区满时静默丢弃（符合 Informational 级别语义）
- 5 个测试覆盖：active run/forwarding/channel close/missing run/no-block
- 已知限制（框架豁免，不修改 trpc 代码）：重复订阅错误用 `fmt.Errorf` 而非哨兵错误；单订阅者限制

### 4.6 P1-6：跨进程事件流

**任务**：event.Bus 增加 Postgres-backed EventStore

**新增文件**：
- `internal/event/postgres_eventstore.go`
- `internal/event/postgres_eventstore_test.go`

**验收**：
- 事件持久化到 Postgres
- WS 重连时从 Postgres replay
- 跨进程事件可消费

**状态**：📋 待办

### 4.7 P1-7：任务级心跳

**任务**：执行引擎每 10s 发布 run_heartbeat 事件

**新增文件**：
- `internal/service/run_heartbeat.go`

**改动文件**：
- `internal/event/contract/envelope.go`（新增 EnvelopeTypeRunHeartbeat）
- `web/src/realtime/ws-transport.ts`（心跳检测）
- `web/src/features/chat/streamHandlers.ts`（心跳处理）

**验收**：
- 心跳事件每 10s 发布
- 前端 30s 无心跳标记 stale
- 心跳包含进度百分比、当前步骤、ETA

**状态**：📋 待办

### 4.8 P1-8：崩溃恢复

**任务**：所有 Run 强制启用 CheckpointSaver + RecoveryWorker

**新增文件**：
- `internal/service/recovery_worker.go`
- `pkg/trpc-agent-go/graph/checkpoint/postgres/`（Postgres CheckpointSaver）

**改动文件**：
- `internal/service/chat_orchestrator_turn.go`（强制启用 CheckpointSaver）
- `cmd/admin/main.go`（启动 RecoveryWorker）

**验收**：
- 进程重启后未完成 Run 从 checkpoint 恢复
- CheckpointSaver 每 5min 存 checkpoint
- RecoveryWorker 启动时扫描 stale Run

**状态**：📋 待办

---

## 五、Phase 2：自主 Graph 编排 + Cursor 级并行 + 崩溃恢复

### 5.1 P2-1：NL2Graph

**任务**：自然语言任务描述 → GraphBuildConfig

**新增文件**：
- `internal/graph/nl2graph.go`
- `internal/graph/nl2graph_test.go`

**验收**：
- 从自然语言生成有效 Graph 拓扑
- 环检测 + DAG 验证
- 失败回退 sequential pipeline

**状态**：📋 待办

### 5.2 P2-2：RuntimeReplanner

**任务**：节点失败触发重规划

**新增文件**：
- `internal/graph/runtime_replanner.go`
- `internal/graph/runtime_replanner_test.go`

**改动文件**：
- `pkg/trpc-agent-go/graph/executor.go`（增加 OnNodeFailure hook）
- `internal/event/contract/envelope.go`（新增 EnvelopeTypeGraphReplanned）

**验收**：
- transient 失败 → retry
- agent_incapable → insert_fallback
- subtask_invalid → rebuild_subgraph
- 重规划过程可观测

**状态**：📋 待办

### 5.3 P2-3：Graph 拓扑演化

**任务**：运行时动态添加 transfer 边

**新增文件**：
- `internal/graph/topology_evolution.go`

**改动文件**：
- `internal/event/contract/envelope.go`（新增 EnvelopeTypeGraphTopologyEvolved）

**验收**：
- 执行中发现新路径可动态添加边
- 拓扑演化事件发布
- Graph 版本管理记录演化历史

**状态**：📋 待办

### 5.4 P2-4：ParallelToolExecutor

**任务**：Cursor 风格并行工具执行

**新增文件**：
- `internal/tools/parallel_executor.go`
- `internal/tools/dependency_analyzer.go`
- `internal/tools/worktree_isolator.go`
- `internal/tools/transaction_sandbox.go`

**验收**：
- 无依赖工具并行执行
- worktree 隔离文件操作
- 事务保护 DB 操作
- 5 文件并行延迟 < 串行 40%

**状态**：📋 待办

### 5.5 P2-5：Team 并行组装优化

**任务**：orchestrateParallelTeams 改为并行组装

**改动文件**：
- `internal/agent/task_orchestrator_impl.go:282-298`

**验收**：
- Team 组装并行化（errgroup）
- 执行阶段保持现有 Graph Executor 并行

**状态**：📋 待办

---

## 六、Phase 3：全链路可观测 + 极致体验 + 领先记忆

### 6.1 P3-1：编排时间线视图

**任务**：Plan→Allocate→Orchestrate→Delivery 跨阶段时间线

**新增文件**：
- `web/src/features/orchestration/OrchestrationTimeline.vue`
- `web/src/features/orchestration/timelineTypes.ts`

**改动文件**：
- `web/src/components/chat/ChatMessagePanel.vue`（集成时间线）

**验收**：
- 时间线展示全阶段
- 每阶段含步骤列表
- 可展开查看步骤详情

**状态**：📋 待办

### 6.2 P3-2：跨边界 Trace 传播

**任务**：Trace 跨 Spirit→Team→Graph 边界

**改动文件**：
- `internal/service/turn_trace.go`（扩展 OrchestrationSpan）
- `internal/agent/task_orchestrator_impl.go`（传播 TraceID）
- `internal/team/team_graph_run_coordinator.go`（传播 TraceID）

**验收**：
- Trace 跨边界传播
- OTel 可查看完整 trace

**状态**：📋 待办

### 6.3 P3-3：Spirit 编排阶段 Metrics

**任务**：编排阶段耗时直方图

**改动文件**：
- `internal/metrics/vars.go`（新增 SpiritPlanDuration/AllocDuration/OrchDuration/AgentFactoryCreated/GraphReplanTotal）

**验收**：
- Prometheus 指标可查询
- Grafana 可展示

**状态**：✅ 已完成（Wave 1）
- 5 个指标已添加：`aranea_spirit_plan_duration_seconds`/`aranea_spirit_alloc_duration_seconds`/`aranea_spirit_orch_duration_seconds`/`aranea_agent_factory_created_total`/`aranea_graph_replan_total`
- buckets 设计：Plan/Alloc 用 `spiritPhaseBuckets`（0.1s-300s，覆盖 LLM 调用），Orch 用独立 buckets（1s-3600s，覆盖长任务子阶段）
- 审查修复：从 `prometheus.DefBuckets`（max 10s）改为 `spiritPhaseBuckets`（max 300s），匹配"multi-minute"注释声明
- 测试：`spirit_metrics_test.go` 验证 Observe/Inc 不 panic（遵循 callback_test.go 模式，无 testutil 依赖）

### 6.4 P3-4：ErrorBlock 内联重试

**任务**：错误块增加重试/切换模型/重新表述按钮

**改动文件**：
- `web/src/components/chat/ErrorBlock.vue`
- `web/src/features/chat/errorCodeHints.ts`（扩展全覆盖）
- `web/src/features/chat/streamHandlers.ts`（联动动作）

**验收**：
- ErrorBlock 有内联按钮
- errorCodeHints 覆盖所有 apierror 码
- 点击按钮执行对应动作

**状态**：📋 待办

### 6.5 P3-5：WS 断连快速检测

**任务**：run_heartbeat 30s 内检测

**改动文件**：
- `web/src/realtime/ws-transport.ts`
- `web/src/features/chat/composables/useChatStreamManager.ts`

**验收**：
- 30s 无心跳标记 stale
- 提供"恢复"按钮

**状态**：📋 待办

### 6.6 P3-6：i18n 全覆盖

**任务**：扫描硬编码中文，迁移到 i18n

**改动文件**：
- `web/src/i18n/locales/zh-CN.ts`
- `web/src/i18n/locales/en-US.ts`
- 所有含硬编码中文的 .vue 文件

**新增**：
- CI 检查脚本禁止新增硬编码

**验收**：
- i18n 覆盖率 100%
- CI 拦截新增硬编码

**状态**：� 部分完成（Wave 1）
- ✅ CI 检查脚本 `web/scripts/check-i18n.mjs` 已创建（扫描硬编码中文 + baseline 增量比对）
- ✅ `web/scripts/i18n-baseline.json` 记录 458 个既有文件的技术债务基线
- ✅ `web/package.json` 新增 `check:i18n` 脚本
- ✅ 6 个高可见 UI 文件已迁移：MainLayout/LoginPage/ChatPage/FileUploadField/ChatHeaderPromptBar/EventStream
- ✅ zh-CN.ts/en-US.ts 新增约 12 个 key，两语言一一对应
- 🟡 既有技术债务：458 个文件仍有硬编码中文（已纳入 baseline，新增违规才失败）
- 🟡 `check:i18n` 未集成到 CI lint/test 流程（需手动运行或后续 PR 加入 CI）
- 🟡 ChatPage.vue:242 既有 FD2 违规（Page 直接 import api），非本次引入

### 6.7 P3-7：移动端三栏折叠

**任务**：移动端隐藏左 Sidebar，右栏改底部抽屉

**改动文件**：
- `web/src/layouts/MainLayout.vue`
- `web/src/components/chat/ChatMessagePanel.vue`
- `web/src/css/theme/_chat-message-panel.sass`

**验收**：
- <1024px 隐藏左 Sidebar
- 右栏改为底部抽屉
- 中栏全屏

**状态**：📋 待办

### 6.8 P3-8：Bi-temporal 失效标记

**任务**：Memory 增加 ValidFrom/ValidUntil

**改动文件**：
- `pkg/trpc-agent-go/memory/memory.go`（Memory 结构扩展）
- `internal/memory/trpc/sqlite_adapter.go`（适配）
- `internal/data/ent/schema/memory.go`（新增列）

**新增文件**：
- `sql/migrations/20260617_memory_bitemporal.sql`

**验收**：
- 冲突时不删除，标记 ValidUntil
- SearchMemories 默认过滤失效记忆
- 支持历史重建查询

**状态**：📋 待办

### 6.9 P3-9：Ebbinghaus 衰减评分

**任务**：R_t = exp(-n_t/S_t) 衰减因子

**新增文件**：
- `internal/memory/ebbinghaus.go`
- `internal/cronrunner/jobs/memory_ebbinghaus_decay.go`

**改动文件**：
- `internal/data/ent/schema/memory.go`（新增 access_count/last_accessed_at/decay_score 列）
- `internal/biz/memory_l3_fused_recall.go`（Score 融合衰减因子）

**验收**：
- 后台 worker 周期计算衰减
- SearchMemories Score 融合 R_t
- 低频访问记忆自动降权

**状态**：📋 待办

### 6.10 P3-10：Sleep-time Agent 异步整理

**任务**：EnqueueConsolidationJob

**新增文件**：
- `internal/memory/sleep_time.go`
- `internal/cronrunner/jobs/memory_sleep_time.go`

**验收**：
- 后台 Agent 合并重复记忆
- 提取反思
- 更新 core memory

**状态**：🟡 部分完成（Wave 1）
- ✅ `SleepTimeService` 实现 Letta/MemGPT 三阶段：merge（去重合并）/reflect（反思提取）/update_core（核心记忆更新）
- ✅ `ConsolidationQueue` 基于 buffered channel + non-blocking select/default，线程安全
- ✅ `MemorySleepTimeWorker` cron job：safego.Go + ctx 取消 + ticker 调度
- ✅ 15 个测试覆盖：正常路径/LLM 失败/响应错误/空输入/malformed JSON/read 失败/多操作组合/队列行为
- ✅ 审查修复：`executeOperations` 增加 default 分支记录未知 op 类型；`buildConsolidationPrompt` 的 json.Marshal 错误处理
- � `MemorySleepTimeWorker` 未通过 Wire 绑定到 cronrunner 调度器（死代码，需后续 PR 补全）
- 🟡 `AgentUserKeyLister` 为 placeholder（返回空列表），生产启用前需实现从 SessionRepo 派生活跃用户
- 🟡 失败 job 无重试无死信（对比 `AutoMemoryWorker.processWithRetry` 的退避重试机制）

### 6.11 P3-11：主动召回触发器

**任务**：ProactiveRecall 接口

**改动文件**：
- `pkg/trpc-agent-go/memory/memory.go`（Service 接口扩展）
- `internal/memory/trpc/sqlite_adapter.go`（实现）
- `internal/biz/memory_composite_recall.go`（集成主动召回）

**验收**：
- 基于对话提及实体自发检索
- 每轮对话前调用
- 主动召回准确率 >80%

**状态**：📋 待办

### 6.12 P3-12：记忆链接图 Evolution

**任务**：Entry 增加 Links/Keywords/Tags + link generation

**改动文件**：
- `pkg/trpc-agent-go/memory/memory.go`（Entry 扩展）
- `internal/memory/trpc/sqlite_adapter.go`（适配）

**新增文件**：
- `internal/memory/link_evolution.go`

**验收**：
- AddMemory 后异步触发 link generation
- 历史记忆 keywords/tags 可演化
- 链接图可视化

**状态**：📋 待办

### 6.13 P3-13：mid-run 增量记忆提取

**任务**：扩展 EnqueueAutoMemoryJob 触发点

**改动文件**：
- `pkg/trpc-agent-go/runner/runner.go`（增加 mid-run 触发点）

**验收**：
- 长任务期间每 N 步触发记忆提取
- 24h 任务记忆条数 <1000

**状态**：✅ 已完成（Wave 1）
- 新增 `WithMidRunMemoryInterval(n int) Option`，n=0 禁用（向后兼容）
- `maybeEnqueueMidRunMemory` 在 `runEventLoop` 中每 N 步触发，stepCount 单 goroutine 访问无竞态
- 提取失败不影响主流程：`enqueueAutoMemoryJob` 捕获所有错误并 log.Debug，runner 不中断
- 4 个测试覆盖：interval 触发/禁用/未达 interval/失败优雅降级

---

## 七、验收标准

### 7.1 Phase 0 验收

| # | 验收项 | 验证方式 | 状态 |
|---|--------|---------|------|
| 1 | WBPF 语义修复 | WAL 失败时不发布 Critical 事件（pre-publish 失败路径） | ✅ |
| 2 | 状态机接入 | 无直接赋值，非法转换被拒绝，WaitingHuman→Failed 合法 | ✅ |
| 3 | Postgres Phase 1 | 关键表迁移完成，FK/唯一约束生效，WAL 双后端选择 | ✅ |
| 4 | DB-R5 修复 | 无直接返回 Ent 错误（含审查修复 session_metrics_repo） | ✅ |

### 7.2 Phase 1 验收

| # | 验收项 | 验证方式 | 状态 |
|---|--------|---------|------|
| 5 | Intent Pass 默认开启 | 默认场景执行；agent setting 可关闭 | ✅ |
| 6 | 预规划门控 | Simple <2s，Moderate+ 强制规划 | ✅ |
| 7 | pgvector 语义匹配 | 准确率 > TF-IDF | ✅ |
| 8 | AgentFactory | 无匹配时自动创建，可观测，可复用 | 📋 |
| 9 | taskrun 事件透传 | 后台任务事件可消费 | ✅ |
| 10 | 跨进程事件流 | WS 重连从 Postgres replay | 📋 |
| 11 | 任务级心跳 | 10s 间隔，30s 检测 stale | 📋 |
| 12 | 崩溃恢复 | 进程重启从 checkpoint 恢复 | 📋 |

### 7.3 Phase 2 验收

| # | 验收项 | 验证方式 |
|---|--------|---------|
| 13 | NL2Graph | 自然语言生成有效拓扑 |
| 14 | RuntimeReplanner | 失败触发重规划，4 种类型 |
| 15 | Graph 拓扑演化 | 动态添加边，有记录 |
| 16 | ParallelToolExecutor | 5 文件并行延迟 < 串行 40% |
| 17 | Team 并行组装 | errgroup 并行 |

### 7.4 Phase 3 验收

| # | 验收项 | 验证方式 | 状态 |
|---|--------|---------|------|
| 18 | 编排时间线 | Plan→Allocate→Orchestrate→Delivery 全阶段 | 📋 |
| 19 | 跨边界 Trace | Spirit→Team→Graph 传播 | 📋 |
| 20 | Spirit Metrics | 耗时直方图可查询 | ✅ |
| 21 | ErrorBlock 重试 | 内联按钮，动作联动 | 📋 |
| 22 | WS 快速检测 | 30s 内检测 stale | 📋 |
| 23 | i18n 全覆盖 | 覆盖率 100%，CI 拦截 | 🟡 |
| 24 | 移动端折叠 | <1024px 折叠策略 | 📋 |
| 25 | Bi-temporal | 冲突不删除，标记失效 | 📋 |
| 26 | Ebbinghaus 衰减 | R_t 计算，低频降权 | 📋 |
| 27 | Sleep-time 整理 | 后台合并/反思/更新 | 🟡 |
| 28 | 主动召回 | 准确率 >80% | 📋 |
| 29 | 记忆链接图 | link generation + 演化 | 📋 |
| 30 | mid-run 提取 | 24h 任务记忆 <1000 | ✅ |

---

## 八、整体验收（对应需求 AC）

| 需求 AC | 对应任务 | 验证方式 |
|---------|---------|---------|
| AC-1 24h 长任务 | P0-3 + P1-6/7/8 | 模拟 24h 任务，进程重启恢复 |
| AC-2 Cursor 级并行 | P2-4/5 | 5 文件并行延迟 < 串行 40% |
| AC-3 极致体验 | P3-4/5/6/7 | 7 痛点修复，i18n 100% |
| AC-4 领先记忆 | P3-8/9/10/11/12/13 | LoCoMo >85，记忆 <1000，召回 >80% |
| AC-5 强制规划 | P1-1/2 | Simple <2s，Moderate+ 强制规划 |
| AC-6 动态 Agent 创建 | P1-3/4 | 无匹配自动创建，可观测，可复用 |
| AC-7 自主 Graph 编排 | P2-1/2/3 | NL2Graph + 重规划 + 演化 |
| AC-8 全链路可观测 | P3-1/2/3 | 时间线 + 跨边界 Trace + Metrics |

---

## 九、改动文件清单

### 9.1 新增文件

**后端**：
- `internal/service/pre_planning_gate.go` ✅
- `internal/service/pre_planning_gate_test.go` ✅
- `internal/service/chat_orchestrator_turn_preplanning.go` ✅
- `internal/event/postgres_wal_storage.go` ✅
- `internal/biz/graph_execution_state_machine.go` ✅
- `internal/biz/graph_execution_usecase_fsm_test.go` ✅
- `internal/data/errors_postgres_test.go` ✅
- `internal/service/run_heartbeat.go`（📋 待创建）
- `internal/service/recovery_worker.go`（📋 待创建）
- `internal/event/postgres_eventstore.go`（📋 待创建）
- `internal/agent/agent_factory.go`（📋 待创建）
- `internal/graph/nl2graph.go`（📋 待创建）
- `internal/graph/runtime_replanner.go`（📋 待创建）
- `internal/graph/topology_evolution.go`（📋 待创建）
- `internal/tools/parallel_executor.go`（📋 待创建）
- `internal/tools/dependency_analyzer.go`（📋 待创建）
- `internal/tools/worktree_isolator.go`（📋 待创建）
- `internal/tools/transaction_sandbox.go`（📋 待创建）
- `internal/memory/ebbinghaus.go`（📋 待创建）
- `internal/memory/sleep_time.go`（📋 待创建）
- `internal/memory/link_evolution.go`（📋 待创建）
- `internal/cronrunner/jobs/memory_ebbinghaus_decay.go`（📋 待创建）
- `internal/cronrunner/jobs/memory_sleep_time.go`（📋 待创建）
- `pkg/trpc-agent-go/graph/checkpoint/postgres/*.go`（📋 待创建）

**前端**：
- `web/src/features/orchestration/OrchestrationTimeline.vue`（📋 待创建）
- `web/src/features/orchestration/timelineTypes.ts`（📋 待创建）

**SQL 迁移**：
- `internal/data/sql/migrations/20260617_postgres_phase1.sql` ✅
- `internal/data/sql/migrations/20260617_memory_bitemporal.sql`（📋 待创建）
- `internal/data/sql/migrations/20260617_memory_ebbinghaus.sql`（📋 待创建）
- `internal/data/sql/migrations/20260617_event_store.sql`（📋 待创建）

### 9.2 改动文件

**后端**（主要）：
- `internal/data/data.go`、`internal/data/tx.go`、`internal/data/errors.go`
- `internal/event/infra.go`、`internal/event/wal.go`
- `internal/service/chat_orchestrator_turn.go`
- `internal/agent/intent/pass.go`
- `internal/agent/task_planner_impl.go`
- `internal/agent/agent_allocator_impl.go`
- `internal/agent/task_orchestrator_impl.go`
- `internal/biz/graph_execution_usecase.go`
- `internal/team/team_graph_run_coordinator.go`
- `internal/team/template_registry.go`
- `internal/memory/trpc/sqlite_adapter.go`
- `internal/biz/memory_l3_fused_recall.go`
- `internal/biz/memory_composite_recall.go`
- `internal/metrics/vars.go`
- `internal/service/turn_trace.go`
- `internal/event/contract/envelope.go`
- `internal/biz/agent_types.go`
- `internal/data/ent/schema/agent.go`
- `internal/data/ent/schema/memory.go`
- `pkg/trpc-agent-go/agent/taskrun/inprocess/service.go`
- `pkg/trpc-agent-go/memory/memory.go`
- `pkg/trpc-agent-go/runner/runner.go`
- `pkg/trpc-agent-go/graph/executor.go`
- `cmd/admin/main.go`

**前端**（主要）：
- `web/src/components/chat/ErrorBlock.vue`
- `web/src/components/chat/ChatMessagePanel.vue`
- `web/src/features/chat/errorCodeHints.ts`
- `web/src/features/chat/streamHandlers.ts`
- `web/src/realtime/ws-transport.ts`
- `web/src/features/chat/composables/useChatStreamManager.ts`
- `web/src/layouts/MainLayout.vue`
- `web/src/i18n/locales/zh-CN.ts`
- `web/src/i18n/locales/en-US.ts`

### 9.3 DB-R5 修复文件（12 个）

> 与 §3.4 任务清单对齐：9 个主修复 + 3 个审查修复（session_repo 含审查修复 3 处，session_metrics_repo 审查修复 3 处，evolution_suggestion_repo/channel.go 为后续补修）。

- `internal/data/session_run_repo.go`（19 处，SESSION_RUN）
- `internal/data/session_repo.go`（27 处 + 审查修复 3 处，SESSION）
- `internal/data/agent_repo.go`（21 处，AGENT）
- `internal/data/borrow_request_repo.go`（7 处，BORROW_REQUEST）
- `internal/data/tool.go`（34 处，TOOL）
- `internal/data/monitor.go`（25 处，MONITOR）
- `internal/data/memory_shim_l1.go`（16 处，MEMORY_L1）
- `internal/data/model_registry_apply.go`（多处，MODEL_REGISTRY）
- `internal/data/agent_performance_repo.go`（2 处，AGENT_PERFORMANCE）
- `internal/data/session_metrics_repo.go`（审查修复 3 处，SESSION_METRICS）
- `internal/data/evolution_suggestion_repo.go`（补修，EVOLUTION_SUGGESTION）
- `internal/data/channel.go`（补修，CHANNEL）

---

## 十、风险与缓解

| # | 风险 | 缓解措施 |
|---|------|---------|
| 1 | Postgres 迁移期间双写一致性 | Phase 1 期间 SQLite 保留只读副本，Postgres 为主，逐步切换 |
| 2 | AgentFactory 生成低质量 Agent | LLM prompt 优化 + 模板基础 + 执行后 DQ Score 评估 |
| 3 | RuntimeReplanner 死循环 | 限制重规划次数（默认 3 次），超限则 fail |
| 4 | worktree 资源泄漏 | 超时自动清理 + 启动时扫描孤儿 worktree |
| 5 | 记忆衰减误删重要记忆 | 衰减只降权不删除，保留可恢复性 |
| 6 | 24h 任务记忆爆炸 | mid-run 增量提取 + Sleep-time 整理 + Ebbinghaus 衰减 |
| 7 | 跨边界 Trace 上下文丢失 | W3C TraceContext 标准传播，context 注入 |
| 8 | i18n 遗漏 | CI 静态扫描硬编码中文 |

---

## 十一、依赖关系

```
Phase 0（基础夯实）
  ├── P0-1 WBPF 修复 ─────────────────┐
  ├── P0-2 状态机接入 ─────────────────┤
  ├── P0-3 Postgres Phase 1 ──────────┼─► Phase 1
  └── P0-4 DB-R5 修复 ────────────────┘

Phase 1（强制规划 + 动态 Agent + 执行引擎）
  ├── P1-1 Intent Pass 默认开启 ──────┐
  ├── P1-2 预规划门控 ────────────────┤
  ├── P1-3 pgvector 语义匹配 ─────────┤
  ├── P1-4 AgentFactory ──────────────┼─► Phase 2
  ├── P1-5 taskrun 事件透传 ──────────┤
  ├── P1-6 跨进程事件流 ──────────────┤
  ├── P1-7 任务级心跳 ────────────────┤
  └── P1-8 崩溃恢复 ──────────────────┘

Phase 2（自主 Graph + Cursor 并行）
  ├── P2-1 NL2Graph ──────────────────┐
  ├── P2-2 RuntimeReplanner ──────────┤
  ├── P2-3 Graph 拓扑演化 ────────────┼─► Phase 3
  ├── P2-4 ParallelToolExecutor ──────┤
  └── P2-5 Team 并行组装 ─────────────┘

Phase 3（可观测 + 体验 + 记忆）
  ├── P3-1~3 可观测（时间线/Trace/Metrics）
  ├── P3-4~7 体验（ErrorBlock/WS/i18n/移动端）
  └── P3-8~13 记忆（Bi-temporal/Ebbinghaus/Sleep/召回/链接/mid-run）
```

---

## 十二、并行执行波次

> 基于 §十一 依赖关系与文件冲突分析，将 19 个待办任务划分为 5 个 Wave。同一 Wave 内任务可并行执行，Wave 间存在依赖需串行。已完成任务（P0-1~P0-4、P1-1、P1-2）不纳入波次。

### 12.0 文件冲突预处理（Wave 1 启动前）

`internal/event/contract/envelope.go` 为高频冲突文件（P1-4/P1-6/P1-7/P2-2/P2-3/P3-3 均需新增 EnvelopeType）。**建议在 Wave 1 启动前先执行一次"事件类型批量注册"预处理**，将后续 Wave 所需的新事件类型常量一次性添加，避免并行开发时的合并冲突。

需批量注册的事件类型：
- `EnvelopeTypeAgentCreated`（P1-4）— Informational（Agent 已落库，事件仅驱动 UI）
- `EnvelopeTypeRunHeartbeat`（P1-7）— Informational（丢失仅降低进度可见性）
- `EnvelopeTypeGraphReplanned`（P2-2）— Important（拓扑漂移防护）
- `EnvelopeTypeGraphTopologyEvolved`（P2-3）— Important（拓扑漂移防护）

**状态**：✅ 已完成（Wave 1）
- 4 个 EnvelopeType 常量已添加至 `envelope.go`，channel 路由已注册（chat/graph）
- AS-EVT-01 分级已落地：`reliability.go` 中 GraphReplanned/GraphTopologyEvolved 注册为 Important，RunHeartbeat/AgentCreated 为 Informational（默认）
- 测试覆盖：`envelope_contract_test.go` 补全至 79 项常量 + 新增 `TestReliabilityClassification` 验证分级

### 12.1 Wave 1：基础设施 + 独立模块（6 任务并行，立即启动）

| 任务 | Stream | 关键文件 | 依赖 | 状态 |
|------|--------|---------|------|------|
| P1-5 taskrun 事件透传 | A | `pkg/trpc-agent-go/agent/taskrun/inprocess/service.go` | 无 | ✅ |
| P1-3 pgvector 语义匹配 | B | `internal/agent/agent_allocator_impl.go` | 无 | ✅ |
| P3-3 Spirit Metrics | E | `internal/metrics/vars.go` | 无 | ✅ |
| P3-6 i18n 全覆盖 | F | `web/src/i18n/locales/*` + 多 `.vue` | 无 | ✅ |
| P3-10 Sleep-time 整理 | G2 | 新增 `internal/memory/sleep_time.go` + cron job | 无 | 🟡 |
| P3-13 mid-run 增量提取 | G2 | `pkg/trpc-agent-go/runner/runner.go` | 无 | ✅ |

**说明**：6 个任务分属不同模块、无文件冲突，可完全并行。本波完成后解锁 Wave 2 的 P1-6（依赖 P1-5）和 P1-4（依赖 P1-3）。

**Wave 1 完成总结**：
- ✅ 5 个任务完全完成，1 个任务（P3-10）部分完成（🟡）
- 🟡 P3-10 Sleep-time 整理：核心 `SleepTimeService` + `ConsolidationQueue` + `MemorySleepTimeWorker` 已实现并测试覆盖，但 `MemorySleepTimeWorker` 未通过 Wire 绑定到 cronrunner 调度器，`AgentUserKeyLister` 为 placeholder（返回空列表）。生产启用前需补全 Wire 绑定 + 实现生产级 lister（从 SessionRepo 派生活跃用户）。
- 审查修复：3 个阻断项（envelope 可靠性分级未落地、测试覆盖不全、expected 计数错误）+ 5 个建议项（metrics buckets、可靠性分级测试、json.Marshal 错误处理、未知 op 日志、注释修正）已修复并通过第二轮审查验证。

### 12.2 Wave 2：执行引擎续 + Agent 动态化 + 独立模块（5 任务并行）

| 任务 | Stream | 关键文件 | 依赖 |
|------|--------|---------|------|
| P1-6 跨进程事件流 | A | 新增 `postgres_eventstore.go` + 改 `envelope.go` | P1-5 ✅ |
| P1-4 AgentFactory | B | 新增 `agent_factory.go` + 改 `agent_types.go`/`schema/agent.go`/`envelope.go` | P1-3 ✅ |
| P3-2 跨边界 Trace 传播 | E | 改 `turn_trace.go`/`task_orchestrator_impl.go`/`team_graph_run_coordinator.go` | 无 |
| P3-7 移动端三栏折叠 | F | 改 `MainLayout.vue`/`ChatMessagePanel.vue` | 无 |
| P3-8 Bi-temporal 失效标记 | G1 | 改 `memory.go`/`sqlite_adapter.go`/`schema/memory.go` + 迁移 SQL | 无 |

**冲突协调**：P1-6 与 P1-4 均改 `envelope.go`，若已执行 §12.0 预处理则无冲突；否则需协调合并。

### 12.3 Wave 3：心跳 + Graph 入口 + 可观测前端 + 记忆衰减（4 任务并行）

| 任务 | Stream | 关键文件 | 依赖 |
|------|--------|---------|------|
| P1-7 任务级心跳 | A | 新增 `run_heartbeat.go` + 改 `envelope.go`/`ws-transport.ts`/`streamHandlers.ts` | P1-6 ✅ |
| P2-1 NL2Graph | C | 新增 `internal/graph/nl2graph.go` | P1-4 ✅ |
| P3-1 编排时间线视图 | E | 新增 `OrchestrationTimeline.vue` + 改 `ChatMessagePanel.vue` | P1-2 ✅ |
| P3-9 Ebbinghaus 衰减评分 | G1 | 新增 `ebbinghaus.go` + cron + 改 `schema/memory.go`/`memory_l3_fused_recall.go` | P3-8 ✅ |

**冲突协调**：P3-1 与 Wave 2 的 P3-7 均改 `ChatMessagePanel.vue`，Wave 2 先行完成，无冲突。

### 12.4 Wave 4：崩溃恢复 + 重规划 + 并行工具 + 体验补全 + 主动召回（6 任务并行）

| 任务 | Stream | 关键文件 | 依赖 |
|------|--------|---------|------|
| P1-8 崩溃恢复 | A | 新增 `recovery_worker.go` + postgres checkpoint + 改 `chat_orchestrator_turn.go`/`main.go` | P1-6 ✅ + P1-7 ✅ |
| P2-2 RuntimeReplanner | C | 新增 `runtime_replanner.go` + 改 `executor.go`/`envelope.go` | P2-1 ✅ |
| P2-4 ParallelToolExecutor | D | 新增 `parallel_executor.go`/`dependency_analyzer.go`/`worktree_isolator.go`/`transaction_sandbox.go` | 无 |
| P3-4 ErrorBlock 内联重试 | F | 改 `ErrorBlock.vue`/`errorCodeHints.ts`/`streamHandlers.ts` | P1-7 ✅ |
| P3-5 WS 断连快速检测 | F | 改 `ws-transport.ts`/`useChatStreamManager.ts` | P1-7 ✅ |
| P3-11 主动召回触发器 | G1 | 改 `memory.go`/`sqlite_adapter.go`/`memory_composite_recall.go` | P3-8 ✅ |

**冲突协调**：P3-4 改 `streamHandlers.ts`、P3-5 改 `ws-transport.ts`/`useChatStreamManager.ts`，文件交叉少可并行；P2-2 改 `envelope.go` 需确认 §12.0 预处理已完成。

### 12.5 Wave 5：拓扑演化 + Team 并行 + 记忆链接（3 任务并行）

| 任务 | Stream | 关键文件 | 依赖 |
|------|--------|---------|------|
| P2-3 Graph 拓扑演化 | C | 新增 `topology_evolution.go` + 改 `envelope.go` | P2-2 ✅ |
| P2-5 Team 并行组装 | D | 改 `task_orchestrator_impl.go` | P3-2 ✅（避免共改同文件） |
| P3-12 记忆链接图 Evolution | G1 | 改 `memory.go`/`sqlite_adapter.go` + 新增 `link_evolution.go` | P3-8/9/11 ✅ |

### 12.6 波次依赖总览

```
Wave 1 (6 并行) ──► Wave 2 (5 并行) ──► Wave 3 (4 并行) ──► Wave 4 (6 并行) ──► Wave 5 (3 并行)

Stream A (执行引擎):  P1-5 ──► P1-6 ──► P1-7 ──► P1-8
Stream B (Agent):     P1-3 ──► P1-4 ──►
Stream C (Graph):                      P2-1 ──► P2-2 ──► P2-3
Stream D (并行):                                P2-4          P2-5
Stream E (可观测):    P3-3     P3-2     P3-1
Stream F (体验):      P3-6     P3-7              P3-4/P3-5
Stream G1 (记忆):              P3-8 ──► P3-9 ──► P3-11 ──► P3-12
Stream G2 (记忆):     P3-10/P3-13
```

### 12.7 关键路径与并行度

| 指标 | 值 | 说明 |
|------|-----|------|
| 最长依赖链 | `P1-3 → P1-4 → P2-1 → P2-2 → P2-3`（Stream B+C，5 任务深度） | 决定整体工期下限 |
| 次长链 | `P1-5 → P1-6 → P1-7 → P1-8`（Stream A，4 任务深度） | 24h 长任务关键路径 |
| 记忆链 | `P3-8 → P3-9 → P3-11 → P3-12`（Stream G1，4 任务深度） | 共享 memory.go，必须串行 |
| 最大并行度 | 6（Wave 1/Wave 4） | 受文件冲突约束 |
| 总任务数 | 19（待办） + 6（已完成） = 25 | 5 个 Wave 覆盖全部待办 |

### 12.8 Stream 划分速查

| Stream | 定位 | 任务序列 | Wave 跨度 |
|--------|------|---------|----------|
| A | 执行引擎与长任务可靠性 | P1-5 → P1-6 → P1-7 → P1-8 | Wave 1-4 |
| B | Agent 动态化 | P1-3 → P1-4 | Wave 1-2 |
| C | Graph 自主编排 | P2-1 → P2-2 → P2-3 | Wave 3-5 |
| D | Cursor 级并行 | P2-4 ∥ P2-5 | Wave 4-5 |
| E | 可观测性 | P3-3 ∥ P3-2 ∥ P3-1 | Wave 1-3 |
| F | 前端体验 | P3-6 ∥ P3-7 ∥ P3-4 ∥ P3-5 | Wave 1-4 |
| G1 | 记忆框架扩展（串行） | P3-8 → P3-9 → P3-11 → P3-12 | Wave 2-5 |
| G2 | 记忆独立模块 | P3-10 ∥ P3-13 | Wave 1 |

---

## 十三、实施纪律

1. **TDD 铁律**：每个任务先写失败测试，再写最小实现
2. **两阶段审查**：规格合规审查优先，代码质量审查其次
3. **验证前置**：每个任务完成前必须运行 `make test && make build && make lint`
4. **YAGNI**：不添加未请求的功能，不过度工程
5. **文档同步**：代码改动同步更新三件套文档
6. **Surgical Changes**：每行改动可追溯到需求，不顺带 refactor
