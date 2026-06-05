## Context

Spirit 是 Aranea-Agents 的唯一对话入口（总管家），当前编排流程存在三重架构断裂：

1. **决策层断裂**：`assess_complexity` 是 LLM 工具调用（浪费 round-trip），规则引擎 simple 优先导致误判，moderate 路径无实现
2. **编排层断裂**：`assemble_team` 混合三个职责（任务分解+Agent分配+团队创建），DAG 依赖由 Spirit 手写 `scheduleDependentTeams` 调度而非委托框架 GraphAgent
3. **持久化断裂**：三阶段中间产物无持久化，异常关闭后全部丢失；Team+Session 非事务创建；合成结果仅内存

当前技术约束：
- trpc-agent-go 框架提供完整的 GraphAgent（DAG 调度+Checkpoint+Interrupt/Resume）、Team（Coordinator/Swarm）、ParallelAgent 编排能力
- 项目架构红线：biz 层不得 import trpc-agent-go，框架交互通过 agent/tools 层桥接
- Wire DI：`provideChatServiceDeps` 已有 33 个参数，新增端口应注入子结构体 `TeamOrchestrationDeps`

## Goals / Non-Goals

**Goals:**
- 三阶段单一职责架构：TaskPlanner（评估+拆分+策略）→ AgentAllocator（匹配）→ TaskOrchestrator（编排执行）
- 每阶段输出持久化到 DB，崩溃后可恢复
- DAG 依赖由 GraphAgent 原生调度，替代手写 scheduleDependentTeams
- spirit_trace_id 贯穿三阶段，StepID 注册表统一命名，每步结构化日志
- 启动时自动恢复 interrupted 编排
- moderate 路径实现（Agent-as-Tool）
- 修复 P0 Bug（DAG 团队 AutoStart=false + Status=active 矛盾）

**Non-Goals:**
- 不修改 trpc-agent-go 框架代码
- 不修改现有 Team 编译流程（CompileToCompiledTeam），DAGToGraphCompiler 作为前置步骤生成 Definition
- 不实现 Embedding 路由（Phase 3 远期目标）
- 不实现 DQ Score 三元分解（保留现有时间惩罚代理）
- 不实现动态任务生成（Graph 运行时动态插入节点）
- 不实现前端 UI 重构（仅扩展事件消费）

## Decisions

### D1: 三阶段顺序执行，阶段间通过 DB 解耦

**选择**：Phase 1 写 TaskPlan → Phase 2 读 TaskPlan 写 AllocationPlan → Phase 3 读两者

**替代方案**：
- A) 三阶段在内存中顺序调用，不持久化中间产物 → 被否决，崩溃后全部丢失
- B) 三阶段通过 EventBus 异步解耦 → 被否决，增加复杂度且无法保证顺序

**理由**：DB 解耦保证每阶段可独立恢复；同一请求内顺序执行避免并发问题；TaskPlan/AllocationPlan 可被 Spirit LLM 审查调整

### D2: TaskPlanner 中 AI 任务拆分使用独立 LLM 调用

**选择**：在 Intent Pass 之后，complex 级别任务触发一次独立的 LLM 调用做任务拆分

**替代方案**：
- A) 复用 Intent Pass 的 LLM 调用，扩展输出 → 被否决，Intent Pass 的 prompt 已很长，增加拆分职责会降低两个任务的质量
- B) 让 Spirit LLM 在工具调用中自行拆分 → 被否决，当前 assemble_team 已证明 LLM 拆分质量不稳定，需要结构化 prompt 约束

**理由**：独立 LLM 调用可用专门的拆分 prompt，输出结构化 SubTask[] + required_capabilities；成本可控（仅 complex 级别触发）

### D3: DAGToGraphCompiler 生成 Definition 而非直接生成 GraphBuildConfig

**选择**：DAGToGraphCompiler 将 TaskDAG + AllocationPlan 转换为 Team Definition JSON，再走现有 CompileToCompiledTeam 编译

**替代方案**：
- A) DAGToGraphCompiler 直接生成 GraphBuildConfig → 被否决，与现有编译流程分歧，两套编译逻辑不可预测
- B) 绕过 Team 概念，直接构建 GraphAgent → 被否决，丧失 Team 层面的管理能力（进度/取消/重试/TeamRun 记录）

**理由**：复用现有编译流程保证一致性；Team 仍作为执行单元存在（TeamRun/TeamRunStep 记录）；DAGToGraphCompiler 只负责"翻译"，不负责"编译"

### D4: 旧工具双写过渡期

**选择**：旧 7 工具保留 2 个版本，标记 deprecated，新 3 工具并行注册；旧工具内部委托新实现

**替代方案**：
- A) 一步替换旧工具 → 被否决，数据库种子数据和前端硬编码会断裂
- B) 工具名别名映射 → 被否决，增加间接层但无法解决 prompt 中旧工具名引用

**理由**：双写保证向后兼容；2 个版本后移除旧工具；prompt 文件同步更新

### D5: Graph Checkpoint 使用 SQLite Saver

**选择**：使用 trpc-agent-go 已有的 `graph/checkpoint/sqlite` Saver

**替代方案**：
- A) 内存 Saver → 被否决，进程崩溃后 Checkpoint 丢失
- B) Redis Saver → 被否决，项目不依赖 Redis

**理由**：SQLite Saver 已有实现，与项目 SQLite 存储一致，PutFull 使用事务保证原子性

### D6: Team 状态机归一

**选择**：将 `active`（未启动）改为 `pending`，`active`（运行中）改为 `running`，删除 `assembled` 状态

**状态机**：
```
pending → running → completed
                 → failed
                 → cancelled
         → cancelled
running → interrupted（新增，异常关闭后可恢复）
interrupted → running（从 Checkpoint 恢复）
```

**理由**：消除 `active` 的语义歧义（当前既表示"已创建未启动"又表示"正在运行"）；与框架 Agent 生命周期对齐

### D7: Agent 匹配三层策略

**选择**：Layer 1 精确匹配（<1ms）→ Layer 2 语义匹配（~10ms，Phase 3 实现）→ Layer 3 LLM 冷启动（~2s）

**理由**：精确匹配覆盖 80% 场景（Agent Domains 标签 + 历史成功率），语义匹配处理模糊场景，LLM 冷启动兜底新 Agent；渐进式实现

## Risks / Trade-offs

| 风险 | 缓解措施 |
|------|---------|
| [R1] Wire 注入参数膨胀 | 新端口注入 TeamOrchestrationDeps 子结构体，不增加 provideChatServiceDeps 参数 |
| [R2] 前后端事件协议断裂 | 双发过渡期：新事件 + 旧事件并行发布，前端双消费 |
| [R3] DAGToGraphCompiler 生成的 Definition 与手写 Definition 语义差异 | 编写集成测试验证编译结果一致性；Definition 生成后走同一 CompileToCompiledTeam 流程 |
| [R4] AI 任务拆分质量不稳定 | 结构化 prompt 约束 + required_capabilities 标签体系 + Spirit LLM 可审查调整（ConfirmPlan） |
| [R5] Graph Checkpoint 存储膨胀 | 实现 DefaultMaxCheckpointsPerLineage=100 限制；编排完成后清理 Checkpoint |
| [R6] 三阶段顺序执行增加延迟 | simple/moderate 路径跳过 Phase 2/3；complex 路径 AI 拆分约 2-5s，可接受 |
| [R7] 旧工具双写期间 LLM 调用两次 | 旧工具内部委托新实现，不重复调用 LLM；prompt 更新引导 LLM 使用新工具 |
| [R8] Team 状态机变更影响现有数据 | 迁移脚本将 active→pending/running；waiting_deps→pending+depends_on |

## Migration Plan

### Phase 1（2 周）— 基础能力 + P0 修复

1. 修复 P0 Bug：DAG 团队 AutoStart=false + Status=active → Status=pending
2. 扩展 Intent Pass：增加 complexity_score, suggested_agents, suggested_topology
3. 实现 TaskPlanner：六维评估 + LLM 任务拆分 + TaskPlan 持久化
4. 实现 moderate 路径：Agent-as-Tool
5. StepID 注册表 + spirit_trace_id 贯穿
6. 启动时自动恢复：扩展 RecoverOrphanedRunningSessions 覆盖 Team 状态
7. Team 状态机归一：active→pending/running

### Phase 2（3 周）— Agent 分配 + Graph 编排

1. 实现 AgentAllocator：三层匹配 + AllocationPlan 持久化
2. 实现 TaskOrchestrator：DAG→Definition→Graph 编译+执行
3. 实现 DAGToGraphCompiler：TaskDAG+AllocationPlan → Definition JSON
4. 合成结果持久化：SynthesisOutput 写入 DB
5. Graph Checkpoint 自动恢复：启动时检测 interrupted 编排
6. 新增 EnvelopeType + 前端双消费

### Phase 3（4 周）— 工具重构 + 智能增强

1. Spirit 工具集重构：7→3，旧工具双写过渡
2. DECISION.md / CAPABILITIES.md prompt 更新
3. Agent 能力 Embedding：Layer 2 语义匹配
4. AgentPerformance 追踪：Agent 级执行历史+成功率
5. 记忆驱动路由：OrchestrationCache + AgentPerformance 联合查询
6. 在线学习闭环：DQ Score → 自动调整权重和拓扑

### Rollback Strategy

- Phase 1：Team 状态机变更通过迁移脚本可回滚（pending→active）
- Phase 2：DAGToGraphCompiler 是新增代码，不影响现有路径；旧 assemble_team 路径保留
- Phase 3：旧工具双写期间可随时切回旧工具

## 实现偏差记录

> 以下记录了实际代码实现与规格设计之间的偏差，供后续迭代参考。

### DEV-01: DAG 编译后的 Definition JSON 未实际替换 Team 的 DefinitionJSON

- **关联规格**: REQ-TO-02 — Generated Definition should go through existing CompileToCompiledTeam compilation
- **实际实现**: `DAGToGraphCompiler.Compile()` 生成了 Definition JSON，但在 `orchestrateDAG()` 中，编译后的 Definition JSON 仅用于日志记录（"For now, we log the compiled definition for observability"）。实际 Team 创建仍通过 `assembler.AssembleTeam()` 走原始路径。
- **影响**: **高** — DAG 编译当前仅为可观测性用途，未替换 Team 的 Definition
- **文件**: `internal/agent/task_orchestrator_impl.go`（~318-324 行）

### DEV-02: Graph Checkpoint 恢复未完整实现

- **关联规格**: REQ-SR-02 — Load latest Checkpoint → Rebuild GraphAgent → ResumeFromLatest
- **实际实现**: `Recover()` 加载了 Checkpoint，但存在 TODO 注释："TODO: Rebuild GraphAgent from checkpoint state and resume execution. The current implementation marks the orchestration as running and relies on the team/agent infrastructure to pick up the work."
- **影响**: **高** — Checkpoint 恢复不完整，仅标记状态未真正恢复执行
- **文件**: `internal/agent/task_orchestrator_impl.go`（~546-551 行）

### DEV-03: AgentCapability.Capacity 字段未使用

- **关联规格**: REQ-AA-03 — 同一 Agent 被分配到多个并行子任务时的冲突检测和负载均衡
- **实际实现**: 无显式的容量检查或冲突检测逻辑。`AgentCapability` 有 `Capacity` 字段但未在匹配中使用。无 `needs_human_decision` 标记。
- **影响**: **中**
- **文件**: `internal/biz/agent_capability.go`

### DEV-04: spirit profile (agent_effective_tools.go) 仍引用旧工具名

- **关联规格**: REQ-SKT-06 — 更新 complexAvailableTools / moderateAvailableTools 工具名
- **实际实现**: `agent_effective_tools.go` 中的 spirit profile 仍引用旧工具名（assemble_team, list_butlers, query_butler_status, check_team_progress, cancel_team），未更新为新工具名。
- **影响**: **中**
- **文件**: `internal/biz/agent_effective_tools.go`（~191 行）

### DEV-05: Layer 2 语义匹配使用 TF-IDF 占位

- **关联规格**: REQ-AA-01 — Embedding 余弦相似度匹配
- **实际实现**: `matchLayer2()` 使用 TF-IDF 关键词匹配作为占位实现，带 TODO 注释："Replace with true embedding cosine similarity via pgvector"
- **影响**: **低**（计划在 Phase 3 T3.3 实现）
- **文件**: `internal/agent/agent_allocator_impl.go`（~282-330 行）

### DEV-06: Team 超时检测和 waiting_deps 超时恢复未完整实现

- **关联规格**: REQ-SR-03/SR-04 — pending/running Team 的超时检测和 waiting_deps 超时恢复
- **实际实现**: `ParallelConfig` 有 `TeamTimeoutSeconds` 字段，`spirit_team.go` 有单个 Team Turn 的超时检查，但无全局 pending/running Team 超时检测定时任务。`scheduleDependentTeams` 仅在 Team 完成时触发，无主动超时检测。
- **影响**: **中**
- **文件**: `internal/service/spirit_team.go`

### DEV-07: Phase 1/2 中断恢复未实现

- **关联规格**: REQ-SR-05 — 通过重新执行后续阶段恢复 draft 状态的 TaskPlan/AllocationPlan
- **实际实现**: `RecoverAllInterrupted` 仅处理 `OrchestrationHandle`，未处理 `TaskPlan` 和 `AllocationPlan` 的 draft 状态恢复。
- **影响**: **中**

### DEV-08: list_butlers/query_butler_status 在 builtin_tools_seed.go 中仍注册

- **关联规格**: REQ-ST-03 — "Delete list_butlers / query_butler_status tools"
- **实际实现**: 仍在 `builtin_tools_seed.go` 中注册（标记为 DEPRECATED）。与 REQ-ST-03 "删除"措辞矛盾，但与 REQ-SKT-05 双写过渡期一致。
- **影响**: **低**

### DEV-09: TaskOrchestratorPort 额外增加 RecoverAllInterrupted 方法

- **关联规格**: 接口定义 5 个方法（Orchestrate/CheckProgress/Cancel/Synthesize/Recover）
- **实际实现**: 代码额外增加了 `RecoverAllInterrupted` 方法。被 `SessionStatusGuard` 使用。合理的扩展但不在规格中。
- **文件**: `internal/biz/task_orchestrator.go`

### DEV-10: orchestration_steps 表未在规格中提及

- **关联规格**: 无
- **实际实现**: `internal/data/ent/schema/orchestration_step.go` 存在，包含 team_run_id, graph_execution_id, node_id, activity_snapshot_json 字段。未在任何规格文档中提及。
- **文件**: `internal/data/ent/schema/orchestration_step.go`

### DEV-11: spirit_trace_id 未在 ChatOrchestrator turn 入口生成

- **关联规格**: REQ-SO-01 — spirit_trace_id 在 ChatOrchestrator turn 入口生成
- **实际实现**: spirit_trace_id 生成逻辑在 `task_planner_impl.go` 和 `task_orchestrator_impl.go` 中，但不在 `ChatOrchestrator` 的 turn 入口处。跳过 TaskPlanner 的 simple/moderate 路径可能缺失 trace ID。
- **影响**: **中** — 非复杂编排路径可能缺失 trace ID

## Open Questions

1. AI 任务拆分的 LLM 调用是否应该使用 Spirit Agent 的模型，还是使用独立的轻量模型？
2. AllocationPlan 中 Team 类型的分配，是否应该复用现有 Team 定义，还是动态创建临时 Team？
3. Graph Checkpoint 清理策略：编排完成后立即清理，还是保留一段时间供审计？
4. 前端是否需要在 Phase 1 就展示 TaskPlan/AllocationPlan 信息，还是仅在后端持久化？
