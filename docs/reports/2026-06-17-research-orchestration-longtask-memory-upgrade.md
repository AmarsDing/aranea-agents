# 编排引擎 + 24h 长任务 + 领先记忆综合升级调研报告

> 生成日期：2026-06-17
> 调研范围：trpc-agent-go 框架原生能力、主流竞品架构、最新记忆论文与开源软件、Aranea-Agents 项目现状
> 关联文档：`docs/development/70-orchestration-longtask-memory.md`（需求）、`.design.md`（设计）、`.development.md`（开发计划）

---

## 一、调研背景

用户提出四大目标：
1. **24h 长任务执行**：系统支持 24 小时长时间任务
2. **Cursor 级并行**：任务并行与批量工具调用媲美 Cursor
3. **绝对友好体验**：客户体验绝对友好
4. **领先记忆系统**：记忆系统领先

并要求：用户发送指令有任务规划；根据任务属性调用或动态创建 Agent/Team；创建过程可观测；能根据任务使用 Graph 自动编排；综合业务需求和逻辑重新设计。

本报告汇总四轮调研证据，为方案落地提供事实基础。

---

## 二、trpc-agent-go 框架原生能力调研

### 2.1 长任务与即时反馈

**关键发现**：框架对"即时反馈 + 长任务"原生支持**中等偏上**。

| 能力 | 框架支持 | 证据 |
|------|---------|------|
| Runner.Run 事件 channel 小时级持续产出 | ✅ 支持 | `pkg/trpc-agent-go/runner/runner.go:788-818` 不设 MaxRunDuration 且父 ctx 无 deadline 时无超时上限 |
| ManagedRunner + DetachedCancel | ✅ 原生 | 父 ctx 取消不影响 run，由 MaxRunDuration 或手动 Cancel 终止 |
| taskrun 后台任务 | ✅ 原生 | Spawn/Wait/Cancel + 文件持久化 |
| taskrun 事件透传 | 🔴 缺口 | 内部消费事件流，不对外暴露 `<-chan *event.Event` |
| 跨进程事件流 | 🔴 缺口 | event.Bus 仅进程内，需自行桥接 |
| Session 跨进程恢复 | ✅ 原生 | Redis/MySQL/Postgres Session Service |
| CheckpointSaver | ✅ 原生 | inmemory/sqlite/redis 三后端，支持 Interrupt/Resume/TimeTravel |
| 原生 Pause | 🔴 无 | 需用 Graph Interrupt + Checkpoint 模拟 |
| Memory 增量写入 | ✅ AddMemory | 但自动提取仅 turn 级（`runner.go:2410` enqueueAutoMemoryJob） |

**框架推荐的长任务模式**（`docs/mkdocs/zh/runner.md`）：
1. 流式长任务：ManagedRunner + WithDetachedCancel(true) + WithMaxRunDuration()
2. 后台任务：taskrun.Controller 的 Spawn/Wait/Cancel/List
3. 可中断工作流：GraphAgent + CheckpointSaver + graph.Interrupt()/Resume
4. 跨进程恢复：Redis/MySQL/Postgres Session Service + 持久化 Checkpoint

### 2.2 记忆系统能力边界

**框架现状**：扁平的 `<appName,userID>` 隔离长期记忆，fact/episode 二分类。

| 能力 | 框架 | 项目 L0-L4 |
|------|------|-----------|
| 存储后端 | 8 种（向量/词法双轨） | SQLite + pgvector 混合 |
| 检索 | 向量 + BM25 + RRF 混合 | 多维度评分融合 |
| 自动提取 | LLM Extractor + reconcile 去重 | Path A/B 双路径 + 反馈驱动 |
| 分层 | ❌ 无 | ✅ L0-L4 五层 |
| 实体图 | ❌ 无 | ✅ L4EntityStore + 级联 Saga |
| 衰减遗忘 | ❌ 无 | ✅ DecayInterval + ForgetConfig |
| 冲突检测 | 仅 score+jaccard 去重 | ✅ 否定模式识别 |
| 多 scope | ❌ 单 user 维度 | ✅ agent/user/team/workspace/global |
| PII 保护 | ❌ 无 | ✅ applyPIIScan |
| 反馈学习 | ❌ 无 | ✅ OnUserFeedback + Path B |

**框架可增强的 5 项技术**（与现有 Service 接口兼容）：
1. Bi-temporal 失效标记（Zep）
2. Ebbinghaus 衰减评分（OBLIVION）
3. Sleep-time Agent 异步整理（Letta）
4. 主动召回触发器（MemCog）
5. 记忆链接图 + Evolution（A-MEM）

---

## 三、竞品架构调研

### 3.1 主流模式：统一流式 + 生命周期解耦

**关键结论**：没有竞品用纯异步 Job 模式，都保留了流式即时反馈。

| 竞品 | 模式 | 关键设计 |
|------|------|---------|
| Cursor | SSE 流式 + 多 Agent 并行 | `multi_tool_use.parallel` + Git worktree 隔离 + 事务沙箱 |
| Claude Code | Foreground + Background 双模式 | LAUNCH→MONITOR→NOTIFY + Monitor 工具实时流式 |
| Devin | 单线程线性 + 压缩模型 | 持久化状态 + 压缩历史 + 异步工作流 + 人类介入 |
| LangGraph | stream + checkpoint | 4 种流式模式（token/node/state/custom）+ 跨机 checkpoint 恢复 |
| OpenAI Assistants v2 | runs.stream() SSE | thread.run.queued→in_progress→step.created→message.delta→completed |
| Vercel AI SDK | SSE + 懒加载背压 | pull-based 流处理 + Data Stream Protocol |

### 3.2 批量工具调用最佳实践

- **Cursor**：`multi_tool_use.parallel` 最大化并行，无数据依赖时必须并行；Git worktree 隔离多 agent 并行编辑
- **Claude Code**：Background Task + 自动 worktree 隔离
- **依赖判定**：构建 DAG，无依赖并行，有依赖串行

### 3.3 长任务持久化

- **LangGraph**：Checkpointer（短期）+ Store（长期），checkpoint 序列化为 MsgPack，可在任意机器任意时间恢复
- **Devin**：压缩模型蒸馏历史动作/决策
- **Claude Code**：notification queue + Agent View 仪表盘

---

## 四、最新记忆论文与开源软件调研

### 4.1 开源软件

| 软件 | 核心创新 | 评测 |
|------|---------|------|
| **MemGPT/Letta** | OS 式虚拟上下文管理（Main/Recall/Archival 三层）+ Sleep-time Agent 异步整理 | 长对话连贯性 |
| **Mem0** | ADD-only + 检索时融合（semantic+BM25+entity）+ Graph Memory | LoCoMo 92.5, LongMemEval 94.4, <7k tokens/query |
| **Cognee** | ECL 管道 + 本体知识图谱 + Temporal Cognification | 14 种检索模式 |
| **A-MEM** | Zettelkasten 自组织记忆 + Memory Evolution（新记忆触发历史记忆更新） | NeurIPS 2025 |
| **Zep/Graphiti** | Bi-temporal Model（t_valid/t_invalid）+ 三层子图 + 增量更新 | P95 300ms |
| **LangMem** | Semantic/Episodic/Procedural 三类 + namespace + Background Manager | LangChain 生态 |

### 4.2 论文（2023-2026）

| 论文 | 核心创新 |
|------|---------|
| MemGPT (2023) | LLM as Operating Systems，虚拟内存管理 |
| A-MEM (NeurIPS 2025) | Agentic Memory，Zettelkasten + LLM 驱动动态组织 |
| Generative Agents (2023) | recency+importance+relevance 三评分 + Reflection |
| Reflexion (2023) | 失败后生成自然语言反思，存入 episodic memory |
| ExpeL (2024) | 跨任务积累可迁移 insights |
| **OBLIVION (2026)** | 遗忘建模为可达性衰减 `R_t = exp(-n_t/S_t)`，非删除 |
| **MRAgent (2026)** | "记忆是重构而非检索"，Cue-Tag-Content 图 + 主动重构，LOCOMO +23% |
| **MemCog (2026)** | Memory-as-Cognition，Proactive Reasoning Protocol |
| **Human-Inspired (2026)** | 6 大认知机制：睡眠期巩固 + 干扰遗忘 + engram 成熟 + 检索时再巩固 + 实体图 + 混合检索 |

### 4.3 五大前沿趋势

1. **Memory-as-Cognition**：从被动检索到主动重构（MRAgent, MemCog）
2. **智能遗忘**：Ebbinghaus 曲线 + 干扰遗忘 + 多维评分（OBLIVION, M.A.K.S）
3. **时序知识图谱**：Bi-temporal model + 增量更新（Zep, Cognee）
4. **Sleep-time 整理**：后台 Agent 异步 consolidate（Letta）
5. **记忆链接演化**：新记忆触发历史记忆更新（A-MEM）

---

## 五、Aranea-Agents 现状评估

### 5.1 任务规划能力

| 维度 | 现状 | 评估 |
|------|------|------|
| Intent Pass | 轻量意图识别 | 🟡 默认关闭，需 `ARANEA_INTENT_PASS=true` |
| 复杂度评估 | 六维评分 | ✅ 完善 |
| 任务分解 | LLM DAG（2-6 子任务） | ✅ 完善 |
| 规划强制触发 | 依赖 Spirit LLM 调用 `plan_and_execute` | 🔴 非强制 |
| 规划结果人工确认 | `ConfirmPlan` 接口未暴露 | 🔴 缺失 |

### 5.2 Agent/Team 动态选择创建

| 维度 | 现状 | 评估 |
|------|------|------|
| Agent 匹配 | 4 层（Performance/Exact/Semantic/LLM 冷启动） | 🟡 语义层是 TF-IDF，非向量 |
| 动态 Team 组建 | parallel/coordinator/dag | ✅ 完善 |
| **动态 Agent 创建** | 无，fallback 到现有 catalog | 🔴 **完全缺失** |
| Capacity 负载均衡 | 字段空转（DEV-03） | 🔴 缺失 |

### 5.3 Graph 自动编排

| 维度 | 现状 | 评估 |
|------|------|------|
| Graph 执行引擎 | BSP + DAG 双引擎 | ✅ 生产级 |
| HITL | Interrupt/Resume/WaitingHuman | ✅ 完整 |
| 失败处理 | 节点重试 + fallback + Graph 级策略 | ✅ 多层 |
| LLM 构建 Graph | `build_orchestration_graph` 工具 | 🟡 需结构化输入 |
| **Adaptive 自适应** | transfer overlay 边（预定义） | 🔴 运行时不可重构拓扑 |
| **运行时重规划** | 无 | 🔴 失败只能重试/fallback/skip |

### 5.4 可观测性

| 维度 | 现状 | 评估 |
|------|------|------|
| 事件覆盖 | 100+ 类型 + AS-EVT-01 三级分级 | ✅ 完整 |
| 持久化 | EventStore(7天) + RingBuffer + WAL | ✅ 三层 |
| 前端可视化 | SpiritStatusBar + AgentTreeTimeline + TeamRunObservatory | 🟡 分散 |
| 分布式 Trace | OTel + SpanCollector | 🔴 仅 chat-scoped |
| 编排时间线 | 无 | 🔴 缺失 |

### 5.5 24h 长任务阻断点

| # | 阻断点 | 位置 | 后果 |
|---|--------|------|------|
| 1 | HTTP server timeout 600s | configs/config.yaml | 同步阻塞，单请求 ≤10 分钟 |
| 2 | Turn 硬上限 2h | chat_orchestrator_turn.go:293 | 单 turn 最多 2 小时 |
| 3 | LLM 单调用 5min | wire.go:680 | 单次模型调用上限 |
| 4 | DB 事务 30s 硬超时 | tx.go:42 | 长事务被强制取消 |
| 5 | 无通用 BackgroundJob Worker | 表存在但无消费者 | 异步任务队列形同虚设 |
| 6 | Interactive 阶段崩溃不可恢复 | 仅 Durable 阶段有 checkpoint | 进程重启丢失 in-flight turn |
| 7 | 无任务级心跳 | ws_io_pump.go 仅 WS ping/pong | 长任务期间前端无进度感知 |
| 8 | WBPF 语义违规 | infra.go:142-160 | Critical 事件可能丢失 |
| 9 | 状态机被绕过 | graph_execution_usecase.go 8 处直接赋值 | 非法状态转换无声发生 |

### 5.6 并发能力上限

| 资源 | 上限 | 位置 |
|------|------|------|
| SQLite 写连接 | 1（绝对串行） | data.go:651 |
| SQLite 读连接 | 2 | data.go:699 |
| WebSocket/会话 | 5 | ws_conn_manager.go:9 |
| Subagent 并发 | 5 | service.go:45 |
| Team 并行 | 3 团队 × 2 内部 | spirit_parallel_config.go:24 |
| Graph worker | GOMAXPROCS | executor.go:2659 |
| event.Bus buffer | 128-512 | bus.go:42 |
| logpipeline buffer | 4096（满则丢） | pipeline.go:104 |

**主要瓶颈**：SQLite 单写连接是物理天花板，要突破必须迁移 Postgres。

---

## 六、综合方案核心决策

基于调研证据，确定以下核心决策：

### 6.1 数据库底座：全量迁移 Postgres

**理由**：SQLite 单写连接是 24h 长任务可靠性、并行处理速度、记忆系统高频读写的共同物理瓶颈。Postgres 突破单写瓶颈，支持 16+ 写并发，原生 FK/事务/崩溃恢复更强。

### 6.2 执行引擎：基于 trpc-agent-go 框架增强

**理由**：用户明确要求"完全跟 trpc 框架对齐，在这个框架基础上功能增强"。框架的 ManagedRunner + taskrun + Session + Checkpoint 体系已具备长任务骨架，需补齐三个缺口：
1. taskrun 事件透传（对外暴露流式事件）
2. 跨进程事件流（Postgres-backed EventStore）
3. 任务级心跳（run_heartbeat 事件）

### 6.3 编排引擎：四层增强（强制规划 + 动态 Agent + 自主 Graph + 全链路可观测）

**用户确认**：
1. 四层增强可以，预留扩展；与项目中现有 graph、team、agent 和记忆先搜索关联，没有时动态创建
2. AgentFactory 不需要人工审核
3. 运行时重规划的局部 Graph 重建可接受

### 6.4 记忆系统：5 项前沿技术集成

对齐 trpc-agent-go Memory Service 接口，集成 Bi-temporal + Ebbinghaus + Sleep-time + 主动召回 + 记忆链接图。

---

## 七、方案与现有项目的关系

### 7.1 复用现有能力

| 现有能力 | 方案复用点 |
|---------|-----------|
| Spirit 三阶段编排（Plan→Allocate→Orchestrate） | Layer 1 强制规划的基础 |
| 4 层 Agent 匹配 | Layer 2 动态 Agent 供给的匹配层 |
| Graph 执行引擎 + 6 模板 | Layer 3 自主 Graph 编排的执行层 |
| 100+ 事件类型 + AS-EVT-01 分级 | Layer 4 全链路可观测的事件层 |
| L0-L4 五层记忆 | 领先记忆系统的基础 |
| Durable Resume + WAL + Checkpoint | 24h 长任务的恢复层 |

### 7.2 新增能力

| 新增能力 | 解决的缺口 |
|---------|-----------|
| 预规划门控 | 规划非强制 |
| AgentFactory | 无动态 Agent 创建 |
| NL2Graph | Graph 需结构化输入 |
| RuntimeReplanner | 运行时不可重构拓扑 |
| 编排时间线视图 | 可视化分散 |
| 跨边界 Trace | trace 仅 chat-scoped |
| Bi-temporal 失效标记 | 记忆冲突时删除而非失效 |
| Ebbinghaus 衰减 | 线性衰减不够智能 |
| Sleep-time 整理 | 自动提取仅 turn 级 |
| 主动召回 | 被动检索 |
| 记忆链接图 Evolution | 无记忆链接 |
| Postgres 全量迁移 | SQLite 单写瓶颈 |
| 统一执行引擎 | HTTP 同步阻塞 |
| Cursor 级并行工具 | 无 worktree 隔离 |

---

## 八、实施路径

### Phase 0：基础夯实（P0 阻断修复 + Postgres Phase 1）
- 修复 WBPF 语义违规
- 接入状态机
- Postgres Phase 1 迁移（EventStore/WAL/Checkpoint/Run/SessionRun/TeamRun/GraphExecution）
- 补齐 FK + 唯一约束
- 修复 DB-R5

### Phase 1：强制规划 + 动态 Agent + 统一执行引擎
- Intent Pass 默认开启
- 预规划门控
- AgentFactory（LLM 生成 Agent 定义，无需人工审核）
- 语义匹配接入 pgvector
- 统一执行引擎（taskrun 事件透传 + 跨进程事件流 + 任务级心跳）

### Phase 2：自主 Graph 编排 + Cursor 级并行 + 崩溃恢复
- NL2Graph
- RuntimeReplanner（局部 Graph 重建）
- Graph 拓扑演化
- ParallelToolExecutor（worktree 隔离 + 事务沙箱）
- 崩溃恢复（CheckpointSaver 强制启用 + RecoveryWorker）

### Phase 3：全链路可观测 + 极致体验 + 领先记忆
- 编排时间线视图
- 跨边界 Trace 传播
- Spirit 编排阶段 Metrics
- 统一编排仪表盘
- 7 体验痛点修复
- Bi-temporal 失效标记
- Ebbinghaus 衰减评分
- Sleep-time Agent 异步整理
- 主动召回触发器
- 记忆链接图 Evolution

---

## 九、验证标准

| 目标 | 验证指标 |
|------|---------|
| 24h 长任务 | 模拟 24h 任务，进程中途重启能从 checkpoint 恢复；WAL 写入失败时不发布 Critical 事件 |
| Cursor 级并行 | 5 文件并行编辑延迟 < 现有串行 40%；worktree 隔离验证；事务回滚验证 |
| 极致体验 | 7 痛点全部修复；ErrorBlock 重试按钮可用；WS 断连 30s 内检测；i18n 覆盖率 100% |
| 领先记忆 | LoCoMo 基准 >85；长任务记忆不爆炸（24h 任务记忆条数 <1000）；主动召回准确率 >80% |
| 强制规划 | Simple 任务直接回答（<2s）；Moderate/Complex 任务强制走规划，规划时间线可见 |
| 动态 Agent 创建 | 无匹配 Agent 时自动创建，创建过程可观测，创建的 Agent 可复用 |
| 自主 Graph 编排 | NL2Graph 从自然语言生成有效拓扑；节点失败触发重规划；拓扑演化有记录 |
| 全链路可观测 | 编排时间线展示 Plan→Allocate→Orchestrate→Delivery 全阶段；Trace 跨边界传播 |

---

## 十、结论

本调研基于 trpc-agent-go 框架原生能力、6 个主流竞品、10+ 记忆论文与开源软件、Aranea-Agents 项目现状的综合分析，确定了"全量迁移 Postgres + 基于 trpc 框架增强 + 四层编排增强 + 5 项记忆前沿技术"的综合升级路径。

**核心原则**：
1. 不另起炉灶，基于 trpc-agent-go 框架原生能力增强
2. 复用项目现有 L0-L4 记忆、Spirit 编排、Graph 引擎、事件体系
3. 补齐四个关键缺口：规划非强制、无动态 Agent 创建、Graph 不可重构、可观测性分散
4. 渐进式集成，每个 Phase 可独立验证

详细的需求、设计、开发计划见 `docs/development/70-orchestration-longtask-memory.*` 三件套。
