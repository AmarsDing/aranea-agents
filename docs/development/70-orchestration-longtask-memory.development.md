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
- `internal/event/infra.go:142-160`

**验收**：
- 单元测试：WAL 失败时不发布 Critical 事件
- 单元测试：WAL 成功时正常发布

**状态**：📋 待办

### 3.2 P0-2：接入状态机

**任务**：GraphExecution 8 处直接赋值改为 Transition 调用

**改动文件**：
- `internal/biz/graph_execution_usecase.go:194,237,283,426,458,528,548,678`
- `internal/team/team_graph_run_coordinator.go:186-190,223-227,258-259`

**验收**：
- 静态分析：无 `exec.Status =` 直接赋值
- 单元测试：非法状态转换被拒绝

**状态**：📋 待办

### 3.3 P0-3：Postgres Phase 1 迁移

**任务**：EventStore/WAL/Checkpoint/Run/SessionRun/TeamRun/GraphExecution 迁移到 Postgres

**改动文件**：
- `internal/data/data.go`（新增 Postgres 连接池）
- `internal/data/tx.go`（去掉 30s 硬超时）
- `internal/data/ent_err.go`（适配 Postgres 错误码）
- `internal/data/ent/schema/*.go`（补齐 FK + 唯一约束）
- `internal/event/wal.go`（适配 Postgres）
- `internal/biz/event_store.go`（适配 Postgres）
- `internal/biz/session_run_checkpoint.go`（适配 Postgres）

**新增文件**：
- `internal/data/postgres.go`（Postgres 连接管理）
- `sql/migrations/20260617_postgres_phase1.sql`

**验收**：
- 现有测试全部通过
- Postgres 连接池配置正确（写 16/读 32）
- FK 约束生效（插入孤儿记录失败）
- 唯一约束生效（并发创建活跃 Run 失败）

**状态**：📋 待办

### 3.4 P0-4：修复 DB-R5 错误翻译

**任务**：11 个 Repo 文件 52+ 处错误翻译

**改动文件**：
- `internal/data/evolution_suggestion_repo.go`
- `internal/data/session_run_repo.go`
- `internal/data/session_repo.go`
- `internal/data/agent_repo.go`
- `internal/data/borrow_request_repo.go`
- `internal/data/agent_performance_repo.go`
- `internal/data/monitor.go`
- `internal/data/tool.go`
- `internal/data/channel.go`
- `internal/data/memory_shim_l1.go`
- `internal/data/model_registry_apply.go`

**验收**：
- 静态分析：无直接返回 Ent 错误
- 单元测试：错误码翻译正确

**状态**：📋 待办

---

## 四、Phase 1：强制规划 + 动态 Agent + 统一执行引擎

### 4.1 P1-1：Intent Pass 默认开启

**任务**：Intent Pass 改为默认开启

**改动文件**：
- `internal/agent/intent/pass.go:50-63`

**验收**：
- 默认场景 Intent Pass 执行
- agent setting 可关闭

**状态**：📋 待办

### 4.2 P1-2：预规划门控

**任务**：复杂度 ≥ Moderate 强制走规划路径

**改动文件**：
- `internal/service/chat_orchestrator_turn.go`（新增 prePlanningGate）
- `internal/agent/task_planner_impl.go`（新增 QuickAssess）

**新增文件**：
- `internal/service/pre_planning_gate.go`

**验收**：
- Simple 任务直接回答（<2s）
- Moderate/Complex 任务强制走规划
- 规划时间线事件发布

**状态**：📋 待办

### 4.3 P1-3：语义匹配接入 pgvector

**任务**：Layer 2 从 TF-IDF 升级为 pgvector embedding

**改动文件**：
- `internal/agent/agent_allocator_impl.go:315-361`

**验收**：
- 向量相似度匹配准确率 > TF-IDF
- Embedder 失败时降级 TF-IDF

**状态**：📋 待办

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
- `pkg/trpc-agent-go/agent/taskrun/inprocess/service.go`

**验收**：
- taskrun 后台任务事件可被外部消费
- 无消费者时不阻塞

**状态**：📋 待办

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

**状态**：📋 待办

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

**状态**：📋 待办

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

**状态**：📋 待办

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

**状态**：📋 待办

---

## 七、验收标准

### 7.1 Phase 0 验收

| # | 验收项 | 验证方式 |
|---|--------|---------|
| 1 | WBPF 语义修复 | WAL 失败时不发布 Critical 事件 |
| 2 | 状态机接入 | 无直接赋值，非法转换被拒绝 |
| 3 | Postgres Phase 1 | 关键表迁移完成，FK/唯一约束生效 |
| 4 | DB-R5 修复 | 无直接返回 Ent 错误 |

### 7.2 Phase 1 验收

| # | 验收项 | 验证方式 |
|---|--------|---------|
| 5 | Intent Pass 默认开启 | 默认场景执行 |
| 6 | 预规划门控 | Simple <2s，Moderate+ 强制规划 |
| 7 | pgvector 语义匹配 | 准确率 > TF-IDF |
| 8 | AgentFactory | 无匹配时自动创建，可观测，可复用 |
| 9 | taskrun 事件透传 | 后台任务事件可消费 |
| 10 | 跨进程事件流 | WS 重连从 Postgres replay |
| 11 | 任务级心跳 | 10s 间隔，30s 检测 stale |
| 12 | 崩溃恢复 | 进程重启从 checkpoint 恢复 |

### 7.3 Phase 2 验收

| # | 验收项 | 验证方式 |
|---|--------|---------|
| 13 | NL2Graph | 自然语言生成有效拓扑 |
| 14 | RuntimeReplanner | 失败触发重规划，4 种类型 |
| 15 | Graph 拓扑演化 | 动态添加边，有记录 |
| 16 | ParallelToolExecutor | 5 文件并行延迟 < 串行 40% |
| 17 | Team 并行组装 | errgroup 并行 |

### 7.4 Phase 3 验收

| # | 验收项 | 验证方式 |
|---|--------|---------|
| 18 | 编排时间线 | Plan→Allocate→Orchestrate→Delivery 全阶段 |
| 19 | 跨边界 Trace | Spirit→Team→Graph 传播 |
| 20 | Spirit Metrics | 耗时直方图可查询 |
| 21 | ErrorBlock 重试 | 内联按钮，动作联动 |
| 22 | WS 快速检测 | 30s 内检测 stale |
| 23 | i18n 全覆盖 | 覆盖率 100%，CI 拦截 |
| 24 | 移动端折叠 | <1024px 折叠策略 |
| 25 | Bi-temporal | 冲突不删除，标记失效 |
| 26 | Ebbinghaus 衰减 | R_t 计算，低频降权 |
| 27 | Sleep-time 整理 | 后台合并/反思/更新 |
| 28 | 主动召回 | 准确率 >80% |
| 29 | 记忆链接图 | link generation + 演化 |
| 30 | mid-run 提取 | 24h 任务记忆 <1000 |

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
- `internal/data/postgres.go`
- `internal/service/pre_planning_gate.go`
- `internal/service/run_heartbeat.go`
- `internal/service/recovery_worker.go`
- `internal/event/postgres_eventstore.go`
- `internal/agent/agent_factory.go`
- `internal/graph/nl2graph.go`
- `internal/graph/runtime_replanner.go`
- `internal/graph/topology_evolution.go`
- `internal/tools/parallel_executor.go`
- `internal/tools/dependency_analyzer.go`
- `internal/tools/worktree_isolator.go`
- `internal/tools/transaction_sandbox.go`
- `internal/memory/ebbinghaus.go`
- `internal/memory/sleep_time.go`
- `internal/memory/link_evolution.go`
- `internal/cronrunner/jobs/memory_ebbinghaus_decay.go`
- `internal/cronrunner/jobs/memory_sleep_time.go`
- `pkg/trpc-agent-go/graph/checkpoint/postgres/*.go`

**前端**：
- `web/src/features/orchestration/OrchestrationTimeline.vue`
- `web/src/features/orchestration/timelineTypes.ts`

**SQL 迁移**：
- `sql/migrations/20260617_postgres_phase1.sql`
- `sql/migrations/20260617_memory_bitemporal.sql`
- `sql/migrations/20260617_memory_ebbinghaus.sql`
- `sql/migrations/20260617_event_store.sql`

### 9.2 改动文件

**后端**（主要）：
- `internal/data/data.go`、`internal/data/tx.go`、`internal/data/ent_err.go`
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

### 9.3 DB-R5 修复文件（11 个）

- `internal/data/evolution_suggestion_repo.go`
- `internal/data/session_run_repo.go`
- `internal/data/session_repo.go`
- `internal/data/agent_repo.go`
- `internal/data/borrow_request_repo.go`
- `internal/data/agent_performance_repo.go`
- `internal/data/monitor.go`
- `internal/data/tool.go`
- `internal/data/channel.go`
- `internal/data/memory_shim_l1.go`
- `internal/data/model_registry_apply.go`

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

## 十二、实施纪律

1. **TDD 铁律**：每个任务先写失败测试，再写最小实现
2. **两阶段审查**：规格合规审查优先，代码质量审查其次
3. **验证前置**：每个任务完成前必须运行 `make test && make build && make lint`
4. **YAGNI**：不添加未请求的功能，不过度工程
5. **文档同步**：代码改动同步更新三件套文档
6. **Surgical Changes**：每行改动可追溯到需求，不顺带 refactor
