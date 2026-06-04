# Tasks: spirit-orchestration-redesign

## Phase 1: 基础能力 + P0 修复（2 周）

### T1.1: 修复 P0 Bug — DAG 团队状态矛盾
- **ID**: T1.1
- **Spec**: spirit-team/REQ-ST-02
- **Description**: 修复 assembleDAGTeams 中 AutoStart=false 的无依赖节点初始状态为 active 但永不启动的问题。改为 pending 状态。
- **Files**: internal/tools/spirit_tools.go, internal/biz/spirit_team_usecase.go, internal/service/spirit_team.go
- **Acceptance**: AutoStart=false 的 DAG 根节点初始状态为 pending；checkAllTeamsCompleted 将 pending/running 视为"仍在进行中"
- **Test**: 单元测试验证 DAG 根节点初始状态

### T1.2: Team 状态机归一
- **ID**: T1.2
- **Spec**: spirit-team/REQ-ST-01
- **Description**: 删除 active/assembled 状态，新增 pending/interrupted 状态，定义合法状态转换。迁移脚本将现有 active→pending/running。
- **Files**: internal/biz/spirit_team_usecase.go, internal/service/spirit_team.go, internal/data/team_repo.go
- **Acceptance**: Team 状态机只有 pending/running/completed/failed/cancelled/interrupted；非法状态转换返回错误
- **Test**: 状态转换单元测试

### T1.3: Team+Session 联合事务
- **ID**: T1.3
- **Spec**: spirit-team/REQ-ST-04, spirit-recovery/REQ-SR-06
- **Description**: AssembleTeam 中 Team 创建和 Session 创建在同一 Ent 事务中。
- **Files**: internal/biz/spirit_team_usecase.go, internal/data/team_repo.go, internal/data/session_repo.go
- **Acceptance**: Team 和 Session 原子创建；部分失败时回滚
- **Test**: 事务回滚测试

### T1.4: 扩展 Intent Pass 输出
- **ID**: T1.4
- **Spec**: task-planner/REQ-TP-01
- **Description**: 在 Artifact 结构中增加 complexity_score, complexity_signals, suggested_agents, suggested_topology 字段。扩展 intentSystemCoding 和 intentSystemGeneral 的 system prompt。
- **Files**: internal/agent/intent/pass.go
- **Acceptance**: Intent Pass 输出包含新字段；新字段有合理默认值
- **Test**: Intent Pass 集成测试

### T1.5: 实现 TaskPlanner 端口和基础实现
- **ID**: T1.5
- **Spec**: task-planner/REQ-TP-01, REQ-TP-03, REQ-TP-04, REQ-TP-06
- **Description**: 定义 TaskPlannerPort 接口，实现六维复杂度评估（语义+结构+工具+上下文+历史，领域跨度 Phase 3），策略输出，TaskPlan 持久化。AI 任务拆分（REQ-TP-02）在 T1.6 单独实现。
- **Files**: internal/biz/task_planner.go, internal/biz/task_plan.go, internal/agent/task_planner_impl.go, internal/data/task_plan_repo.go, internal/data/ent/schema/task_plan.go
- **Acceptance**: Plan() 返回 TaskPlan 含复杂度评估+策略；TaskPlan 持久化到 DB
- **Test**: 六维评估单元测试；策略选择测试；DB 持久化测试

### T1.6: 实现 AI 任务拆分
- **ID**: T1.6
- **Spec**: task-planner/REQ-TP-02
- **Description**: 实现 complex 级别任务的 LLM 拆分。结构化 prompt 约束输出 SubTask[] + required_capabilities + depends_on。生成 TaskDAG。
- **Files**: internal/agent/task_planner_impl.go
- **Acceptance**: complex 任务被拆分为 SubTask[]；每个 SubTask 含 required_capabilities；TaskDAG 无环
- **Test**: 任务拆分集成测试；DAG 环检测测试

### T1.7: 实现 moderate 路径（Agent-as-Tool）
- **ID**: T1.7
- **Spec**: spirit-team/REQ-ST-03
- **Description**: 使用 agenttool.NewTool 将匹配的 Agent 包装为工具，Spirit 调用该工具完成任务。删除 list_butlers / query_butler_status。
- **Files**: internal/tools/spirit_tools.go, internal/service/chat_orchestrator.go
- **Acceptance**: moderate 级别任务通过 Agent-as-Tool 执行；list_butlers/query_butler_status 不再注册
- **Test**: Agent-as-Tool 集成测试

### T1.8: StepID 注册表 + spirit_trace_id
- **ID**: T1.8
- **Spec**: spirit-observability/REQ-SO-01, REQ-SO-02
- **Description**: 创建 spirit_step_ids.go 定义所有 StepID 常量。在 ChatOrchestrator 的 turn 入口生成 spirit_trace_id 并贯穿三阶段。
- **Files**: internal/biz/spirit_step_ids.go, internal/service/chat_orchestrator.go
- **Acceptance**: 所有 Spirit 日志携带 TraceID + StepID；StepID 命名一致
- **Test**: 日志输出验证测试

### T1.9: 启动时自动恢复扩展
- **ID**: T1.9
- **Spec**: spirit-recovery/REQ-SR-01
- **Description**: 扩展 RecoverOrphanedRunningSessions 同时恢复 Team 状态（running→interrupted）和 TeamRun 状态（running→failed）。
- **Files**: internal/biz/session/recovery.go, internal/service/spirit_team.go
- **Acceptance**: 启动时 running 的 Team 被转为 interrupted；running 的 TeamRun 被转为 failed
- **Test**: 恢复逻辑单元测试

## Phase 2: Agent 分配 + Graph 编排（3 周）

### T2.1: 实现 AgentAllocator 端口和基础实现
- **ID**: T2.1
- **Spec**: agent-allocator/REQ-AA-01, REQ-AA-02, REQ-AA-04, REQ-AA-07
- **Description**: 定义 AgentAllocatorPort 接口，实现 Layer 1 精确匹配 + Layer 3 LLM 冷启动。AllocationPlan 持久化。Layer 2 语义匹配在 T3.3 实现。
- **Files**: internal/biz/agent_allocator.go, internal/biz/allocation_plan.go, internal/agent/agent_allocator_impl.go, internal/data/allocation_plan_repo.go, internal/data/ent/schema/allocation_plan.go
- **Acceptance**: Allocate() 返回 AllocationPlan；精确匹配基于 Agent Domains + 历史成功率；LLM 冷启动兜底
- **Test**: 三层匹配单元测试；DB 持久化测试

### T2.2: 实现 Agent 能力注册表和执行历史
- **ID**: T2.2
- **Spec**: agent-allocator/REQ-AA-05, REQ-AA-06
- **Description**: 实现 AgentCapability 模型（从 Agent 目录自动构建）和 AgentPerformance 模型（每次编排完成后更新）。
- **Files**: internal/biz/agent_capability.go, internal/biz/agent_performance.go, internal/data/agent_performance_repo.go
- **Acceptance**: AgentCapability 从 Agent 目录自动构建；AgentPerformance 每次编排后更新
- **Test**: 能力构建测试；性能更新测试

### T2.3: 实现 TaskOrchestrator 端口和 DAG 编排
- **ID**: T2.3
- **Spec**: task-orchestrator/REQ-TO-01, REQ-TO-02, REQ-TO-05
- **Description**: 定义 TaskOrchestratorPort 接口，实现 DAGToGraphCompiler（TaskDAG+AllocationPlan→Definition JSON），5 种编排策略执行。OrchestrationHandle 持久化。
- **Files**: internal/biz/task_orchestrator.go, internal/agent/task_orchestrator_impl.go, internal/agent/dag_graph_compiler.go, internal/data/orchestration_repo.go, internal/data/ent/schema/orchestration.go
- **Acceptance**: Orchestrate() 返回 OrchestrationHandle；DAG 编译为 Definition 后走 CompileToCompiledTeam；5 种策略均可执行
- **Test**: DAG 编译测试；5 种策略执行测试；DB 持久化测试

### T2.4: Graph Checkpoint 集成
- **ID**: T2.4
- **Spec**: task-orchestrator/REQ-TO-03
- **Description**: 在 DAG 编排中使用 SQLite CheckpointSaver，每步自动保存。OrchestrationHandle 记录最新 checkpoint_id。
- **Files**: internal/agent/task_orchestrator_impl.go
- **Acceptance**: DAG 编排每步保存 Checkpoint；OrchestrationHandle.CheckpointID 非空
- **Test**: Checkpoint 保存和加载测试

### T2.5: 合成结果持久化
- **ID**: T2.5
- **Spec**: task-orchestrator/REQ-TO-04
- **Description**: SynthesisOutput 写入 OrchestrationHandle.SynthesisResultJSON，不再仅通过 EventBus 发布。
- **Files**: internal/agent/task_orchestrator_impl.go, internal/service/spirit_synthesis.go
- **Acceptance**: 合成结果可在 DB 中查询；异常关闭后不丢失
- **Test**: 合成结果持久化测试

### T2.6: Graph Checkpoint 自动恢复
- **ID**: T2.6
- **Spec**: spirit-recovery/REQ-SR-02
- **Description**: 启动时检测 status=interrupted 的 OrchestrationHandle，加载最新 Checkpoint，重建 GraphAgent，ResumeFromLatest。
- **Files**: internal/agent/task_orchestrator_impl.go, internal/service/chat_orchestrator.go
- **Acceptance**: interrupted 编排可从 Checkpoint 恢复执行
- **Test**: 恢复流程集成测试

### T2.7: 新增 EnvelopeType + 前端双消费
- **ID**: T2.7
- **Spec**: spirit-observability/REQ-SO-03, REQ-SO-04
- **Description**: 新增 5 个 EnvelopeType 常量。后端双发新旧事件。前端 envelope.ts 新增类型，handleSpiritEnvelope 双消费。
- **Files**: internal/event/contract/envelope.go, internal/event/envelope.go, web/src/realtime/envelope.ts, web/src/stores/spirit/index.ts
- **Acceptance**: 新事件可被前端消费；旧事件仍可消费
- **Test**: 事件发布/消费测试

### T2.8: Wire 注入新端口
- **ID**: T2.8
- **Spec**: all
- **Description**: 在 TeamOrchestrationDeps 中新增 TaskPlanner/AgentAllocator/TaskOrchestrator 字段。修改 provideTeamOrchestrationDeps。运行 make wire。
- **Files**: cmd/admin/wire.go, internal/service/chat_orchestrator.go
- **Acceptance**: make wire && go build ./cmd/admin 通过
- **Test**: 编译验证

## Phase 3: 工具重构 + 智能增强（4 周）

### T3.1: Spirit 工具集重构
- **ID**: T3.1
- **Spec**: spirit-tools/REQ-SKT-01~06
- **Description**: 实现 plan_and_execute / check_progress / cancel_orchestration 3 个新工具。旧工具双写过渡。更新 builtin_tools_seed.go。
- **Files**: internal/tools/spirit_tools.go, internal/data/builtin_tools_seed.go
- **Acceptance**: 新工具可调用；旧工具委托新实现；make build 通过
- **Test**: 新旧工具集成测试

### T3.2: Prompt 文件更新
- **ID**: T3.2
- **Spec**: spirit-tools/REQ-SKT-05
- **Description**: 更新 DECISION.md 和 CAPABILITIES.md 中的工具名引用。
- **Files**: internal/scenario/system/prompts/DECISION.md, internal/scenario/system/prompts/CAPABILITIES.md
- **Acceptance**: Prompt 中只引用新工具名；旧工具名标记 deprecated
- **Test**: 无

### T3.3: Agent 能力 Embedding（Layer 2 语义匹配）
- **ID**: T3.3
- **Spec**: agent-allocator/REQ-AA-01
- **Description**: 为每个 Agent 生成能力 Embedding，使用 pgvector 做余弦相似度匹配。复用 knowledge.Embedder。
- **Files**: internal/agent/agent_allocator_impl.go, internal/biz/agent_capability.go
- **Acceptance**: Layer 2 匹配返回结果；score = cosine_sim * 0.6 + success_rate * 0.4
- **Test**: 语义匹配测试

### T3.4: 记忆驱动路由
- **ID**: T3.4
- **Spec**: task-planner/REQ-TP-05
- **Description**: OrchestrationCache + AgentPerformance 联合查询。记忆命中 DQ>0.7 时直接复用历史拓扑和 Agent 组合。
- **Files**: internal/agent/task_planner_impl.go, internal/biz/spirit_orchestration_cache.go
- **Acceptance**: 记忆命中时跳过完整评估；复用历史拓扑
- **Test**: 记忆路由测试

### T3.5: 在线学习闭环
- **ID**: T3.5
- **Spec**: task-orchestrator/REQ-TO-08
- **Description**: 编排完成后更新 OrchestrationCache（DQ Score）和 AgentPerformance。DQ Score 自动调整权重和拓扑推荐。
- **Files**: internal/agent/task_orchestrator_impl.go, internal/biz/spirit_orchestration_cache.go, internal/data/agent_performance_repo.go
- **Acceptance**: 编排完成后 DQ Score 和 AgentPerformance 自动更新
- **Test**: 学习闭环测试

### T3.6: 全量验证
- **ID**: T3.6
- **Spec**: all
- **Description**: 运行完整验证：make api && make wire && make build && make test && make lint。前端：cd web && pnpm lint && pnpm test && pnpm build。
- **Files**: all
- **Acceptance**: 全部验证通过
- **Test**: CI 验证
