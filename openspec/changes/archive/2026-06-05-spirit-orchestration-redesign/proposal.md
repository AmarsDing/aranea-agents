## Why

Spirit 编排模块存在三重架构断裂：(1) 任务复杂度评估（assess_complexity）是 LLM 工具调用，规则引擎 simple 优先导致误判，moderate 路径无实现；(2) 任务分解、Agent 分配、团队创建混合在 assemble_team 一个工具中，职责不清；(3) DAG 依赖由 Spirit 手写 scheduleDependentTeams 调度，而非委托 trpc-agent-go 框架的 GraphAgent 原生调度，导致 DAG 团队 AutoStart=false 时创建为 active 状态但永不启动（P0 Bug）。此外，三阶段中间产物无持久化，异常关闭后全部丢失；可观测性覆盖率仅 ~60%，无法串联一个完整对话的编排链路。

## What Changes

- **新增三阶段编排架构**：TaskPlanner（评估+拆分+策略）→ AgentAllocator（Agent/Team 匹配）→ TaskOrchestrator（Graph 编排执行），每阶段单一职责、输出持久化
- **新增 TaskPlanner 端口**：六维复杂度评估 + AI 任务拆分 + 策略输出，替代 assess_complexity 工具调用
- **新增 AgentAllocator 端口**：三层 Agent 匹配（精确→语义→LLM 冷启动），替代 assemble_team 中硬编码的 Agent 分配
- **新增 TaskOrchestrator 端口**：DAG 直接编译为 GraphAgent，替代手写 scheduleDependentTeams 调度
- **重构 Spirit 工具集**：7 工具 → 3 工具（plan_and_execute / check_progress / cancel_orchestration），旧工具双写过渡
- **新增可观测性体系**：spirit_trace_id 贯穿三阶段，StepID 注册表统一命名，每步结构化日志
- **新增持久化保障**：TaskPlan / AllocationPlan / OrchestrationHandle 持久化到 DB，Graph Checkpoint 自动保存，合成结果写入 DB
- **新增异常恢复机制**：启动时自动恢复 interrupted 编排，从 Graph Checkpoint 恢复执行
- **实现 moderate 路径**：Agent-as-Tool（agenttool.NewTool）
- **修复 P0 Bug**：DAG 团队 AutoStart=false + Status=active 矛盾

## Capabilities

### New Capabilities

- `task-planner`: 任务复杂度评估与 AI 任务拆分，输出 TaskPlan（持久化），单一职责：只做评估+拆分+策略
- `agent-allocator`: Agent/Team 匹配分配器，三层匹配（精确→语义→LLM 冷启动），输出 AllocationPlan（持久化），单一职责：只为子任务匹配最优 Agent
- `task-orchestrator`: 任务编排执行器，DAG→Graph 编译+执行+合成+恢复，输出 OrchestrationHandle（持久化），委托 trpc-agent-go GraphAgent 原生调度
- `spirit-observability`: Spirit 编排可观测性体系，spirit_trace_id 贯穿三阶段，StepID 注册表，结构化日志，新增 EnvelopeType
- `spirit-recovery`: Spirit 编排异常恢复机制，启动时自动恢复 interrupted 编排，Graph Checkpoint 恢复，Team 超时检测

### Modified Capabilities

- `spirit-team`: SpiritTeamUsecase 的 AssembleTeam 流程重构，Team+Session 联合事务创建，Team 状态机归一（active→pending/running），moderate 路径实现（Agent-as-Tool）
- `spirit-tools`: Spirit 工具集从 7 工具精简为 3 工具，旧工具双写过渡期保留

## Impact

- **biz 层**：新增 5 个端口接口（TaskPlannerPort, AgentAllocatorPort, TaskOrchestratorPort, AgentCapabilityRepo, AgentPerformanceRepo），新增 3 个数据模型（TaskPlan, AllocationPlan, OrchestrationHandle），修改 SpiritTeamUsecase
- **data 层**：新增 3 个 Ent Schema（task_plan, allocation_plan, orchestration），新增 3 个 Repo 实现，修改 Team+Session 创建为联合事务
- **service 层**：修改 ChatOrchestratorDeps/TeamOrchestrationDeps 注入新端口，修改 spiritCustomTools 工具注册
- **agent 层**：新增 DAGToGraphCompiler（TaskDAG→Definition→GraphBuildConfig），新增三阶段实现类
- **tools 层**：重构 spirit_tools.go，更新 spirit_complexity.go 工具引用
- **event 层**：新增 5 个 EnvelopeType 常量
- **Wire**：修改 provideTeamOrchestrationDeps 增加新端口参数，重新生成 wire_gen.go
- **前端**：envelope.ts 新增事件类型，useSpiritTeamStore 新增状态，DECISION.md/CAPABILITIES.md 更新工具名
- **数据库**：新增 3 张表，Team 表无需修改，OrchestrationCache 数据迁移
- **trpc-agent-go**：无修改，仅使用现有 GraphAgent/Checkpoint/Team API
