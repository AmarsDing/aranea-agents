## Non-goals

- 不重新设计已完成变更的核心架构
- 不修改 trpc-agent-go 框架层
- 不修改 Proto API 对外接口
- 不实施 learning-loop-frontend、chat-sidebar-pinned-collapse-drag
- 不实施 modelregistry-refactor
- 不实施 memory-skills-butler 完整 Usecase（仅收敛类型定义）
- 不实施 skill-intelligence 完整 Usecase（仅收敛类型定义）
- 不实施 monitor-self-healing / monitor-selfcheck-repair 完整功能（仅统一类型和事件契约）

---

## 1. Phase 0: 基础设施 — 类型收敛与事件注册

- [ ] 1.1 创建 `internal/biz/types/` 包，添加 `doc.go` 文件声明包用途
  - DoD: `internal/biz/types/doc.go` 存在，`go build ./internal/biz/types/...` 通过
- [ ] 1.2 创建 `internal/biz/types/skill_health.go`，定义 `SkillHealth`、`ToolWeightReport`、`ExperienceReport` 结构体
  - DoD: 类型定义与 `internal/tools/skills_butler/` 中现有定义字段一致，`go build` 通过
- [ ] 1.3 创建 `internal/biz/types/monitor_condition.go`，定义 `HealRecord`、`SelfCheckResult`、`AutoHealedCondition`、`SelfCheckStatusCondition` 结构体
  - DoD: 类型定义与归档变更 design.md 中描述一致，`go build` 通过
- [ ] 1.4 创建 `internal/biz/types/butler_types.go`，定义 `ButlerTier`、`ButlerCapability` 枚举，`OrchestrationStep`、`TaskPlan`、`AllocationPlan`、`OrchestrationHandle` 结构体
  - DoD: 类型定义与 spirit-orchestration-redesign 和 system-builtin-agents 的 design.md 一致，`go build` 通过
- [ ] 1.5 创建 `internal/biz/types/session_types.go`，定义 `SessionStatus` 枚举、`StatusReason` 类型、`SessionTreeNode` 结构体
  - DoD: SessionStatus 枚举值与 session-status-monitoring 一致（idle/running/completed/interrupted/awaiting_confirmation），`go build` 通过
- [ ] 1.6 在 `internal/event/envelope.go` 中注册 7 个新 EnvelopeType 常量：ButlerOrchestrationStarted/Completed/Failed、SkillHealthChanged、SkillEvolutionProposed、MonitorAutoHealed、MonitorSelfCheckCompleted
  - DoD: 常量定义存在，数值不与现有常量冲突，`go build` 通过
- [ ] 1.7 将 `internal/tools/skills_butler/` 中的 SkillHealth/ToolWeightReport 类型替换为 `biz/types` 的 import
  - DoD: `grep -r "type SkillHealth struct" internal/tools/skills_butler/` 返回 0 结果，`go build ./internal/tools/skills_butler/...` 通过
- [ ] 1.8 将 `internal/biz/task_orchestrator.go` 中的 TaskPlan/AllocationPlan/OrchestrationHandle 类型替换为 `biz/types` 的 import
  - DoD: 本地类型定义已删除，import `biz/types` 存在，`go build ./internal/biz/...` 通过
- [ ] 1.9 全量验证 Phase 0
  - DoD: `go build ./...` 通过，`go test ./internal/biz/... ./internal/tools/... -count=1` 通过

## 2. Phase 1: 管家体系统一 — 工具收敛

- [ ] 2.1 创建 `internal/tools/spirit/orchestration_steps.go`，将 8 个细粒度工具的逻辑提取为普通 Go 函数（classifyIndustry, searchPositions, findAgentsByPosition, instantiateAgentFromPosition, estimateTask, assembleTeam, reportTaskResult, queryAgentStatus）
  - DoD: 8 个函数存在，接收和返回类型与原工具定义一致，`go build` 通过
- [ ] 2.2 修改 `plan_and_execute` 工具实现，内部按序调用 orchestration_steps.go 中的函数（classifyIndustry → searchPositions → findAgents → estimateTask → assembleTeam）
  - DoD: plan_and_execute 内部调用链完整，每个步骤的输出作为下一步的输入
- [ ] 2.3 修改 `check_progress` 工具实现，内部调用 queryAgentStatus 函数
  - DoD: check_progress 返回聚合进度信息
- [ ] 2.4 修改 `cancel_orchestration` 工具实现，内部调用 reportTaskResult 函数（取消场景）
  - DoD: cancel_orchestration 触发取消流程并报告结果
- [ ] 2.5 从 `systemBuiltinTools()` 中移除 8 个细粒度工具注册，仅保留 3 个粗粒度工具 + 4 个 Skills Butler 工具
  - DoD: `systemBuiltinTools()` 返回 7 个工具（3 Spirit + 4 Skills Butler），`go build` 通过
- [ ] 2.6 在 plan_and_execute 中添加 OrchestrationStep 记录，每个内部步骤的 input/output/status 记录到 OrchestrationHandle
  - DoD: plan_and_execute 返回的 OrchestrationHandle 包含完整的步骤记录
- [ ] 2.7 在 plan_and_execute 中添加事件发射（EnvelopeTypeButlerOrchestrationStarted/Completed/Failed）
  - DoD: plan_and_execute 开始时发射 Started 事件，成功时发射 Completed 事件，失败时发射 Failed 事件
- [ ] 2.8 全量验证 Phase 1
  - DoD: `go build ./...` 通过，`go test ./internal/tools/spirit/... ./internal/biz/... -count=1` 通过，Spirit Agent 工具列表仅包含 3 个粗粒度工具

## 3. Phase 2: Session 延后补全 — DTO 解耦与 patch 迁移

- [ ] 3.1 创建 `SessionMetricsDTO` 结构体，用于 toProtoSession 中的 metrics 字段映射
  - DoD: SessionMetricsDTO 包含 token_usage / cost / latency 等字段，`go build` 通过
- [ ] 3.2 修改 `toProtoSession` 函数，metrics 字段改为通过 SessionMetricsRepo 独立查询
  - DoD: toProtoSession 不再从 sessions 表直接读取 metrics 字段，而是调用 SessionMetricsRepo.GetBySessionID()
- [ ] 3.3 修改 `UpdateSession` 函数，runtime 字段更新路由到 SessionRuntimeRepo
  - DoD: status/status_reason/finished_at 的更新通过 SessionRuntimeRepo.TransitionSessionStatus() 执行
- [ ] 3.4 将 SessionStatus 枚举和 StatusReason 类型替换为 `biz/types` 的 import
  - DoD: `grep -r "type SessionStatus " internal/biz/ internal/data/ internal/service/` 仅在 `biz/types/session_types.go` 中找到定义
- [ ] 3.5 全量验证 Phase 2
  - DoD: `go build ./...` 通过，`go test ./internal/service/... ./internal/data/... ./internal/biz/... -count=1` 通过，Session CRUD 操作正常

## 4. Phase 3: team-graph-optimization M2~M5

- [ ] 4.1 M2: 修复 P1 竞态条件 — GraphExecution 并发安全（6 项）
  - DoD: `go test ./internal/biz/ -run TestGraphExecution -race -count=1` 通过，无 data race
- [ ] 4.2 M3: 实现 CompiledTeam 编译产物 — 创建 CompiledTeam Ent Schema 和 Repo
  - DoD: `internal/data/compiled_team_repo.go` 存在，Ent Schema 生成成功，`go build` 通过
- [ ] 4.3 M3: 实现 CompiledTeam 编译产物 — 实现 CompileToCompiledTeam 编译函数
  - DoD: 编译函数接收 Team 配置，输出 CompiledTeam（含展开的 FailurePolicy、RoleManifest、NodeTaskMeta）
- [ ] 4.4 M3: 实现 CompiledTeam 编译产物 — CompiledTeam 持久化使用 sessions 三表结构
  - DoD: CompiledTeamRepo 通过 SessionRuntimeRepo 查询 session 状态，不直接查 sessions 表
- [ ] 4.5 M4: Graph 独立性 — GraphBuilderFactory 拆分为 4 个窄接口（DefinitionFactory / ExecutionFactory / CacheManager / TeamMediator）
  - DoD: 原 GraphBuilderFactory 接口删除，4 个新接口存在，`go build` 通过
- [ ] 4.6 M4: Graph 独立性 — DAGToGraphCompiler 使用 DefinitionFactory
  - DoD: TaskOrchestrator 的 DAGToGraphCompiler 通过 DefinitionFactory.BuildDefinition() 编译 Graph
- [ ] 4.7 M5: Team 生命周期统一 — Team 状态机与 SessionStatusMachine 对齐
  - DoD: Team running → Session running，Team failed → Session interrupted，状态转换通过 SessionRuntimeRepo 执行
- [ ] 4.8 M5: Team 生命周期统一 — TeamRunMediator 解决 Runner↔Coordinator 双向依赖
  - DoD: TeamRunMediator 接口存在，Runner 和 Coordinator 通过 Mediator 交互，无直接 import 循环
- [ ] 4.9 全量验证 Phase 3
  - DoD: `go build ./...` 通过，`go test ./internal/biz/... ./internal/data/... -count=1` 通过，Team/Graph 编译和执行正常

## 5. Phase 4: 监控体系统一 — RootCauseCondition 扩展

- [ ] 5.1 在 `api/kratos/monitor/v1/monitor.proto` 的 RootCauseCondition 中添加 oneof condition 字段
  - DoD: proto 定义包含 `oneof condition { AutoHealedCondition auto_healed = 10; HealAttemptsCondition heal_attempts = 11; SelfCheckStatusCondition self_check_status = 12; }`
- [ ] 5.2 定义 AutoHealedCondition 和 HealAttemptsCondition proto message
  - DoD: message 定义存在，`make api` 生成成功
- [ ] 5.3 定义 SelfCheckStatusCondition proto message
  - DoD: message 定义存在，`make api` 生成成功
- [ ] 5.4 修改 `internal/service/monitor.go` 适配新的 RootCauseCondition oneof 结构
  - DoD: DiagnoseAndHeal API 返回的 RootCauseCondition 使用 oneof 字段，`go build` 通过
- [ ] 5.5 全量验证 Phase 4
  - DoD: `make api && make wire && make build && make test` 通过

## 6. 全量验证与收尾

- [ ] 6.1 运行全量后端验证：`make api && make wire && make build && make test && make lint`
  - DoD: 所有命令通过，0 错误
- [ ] 6.2 运行前端验证：`cd web && pnpm lint && pnpm test && pnpm build`
  - DoD: 所有命令通过，0 错误
- [ ] 6.3 类型唯一性验证：确认 SkillHealth / ToolWeightReport / ExperienceReport / HealRecord / SelfCheckResult / SessionStatus 在整个代码库中仅有一个定义
  - DoD: `grep -r "type SkillHealth struct\|type ToolWeightReport struct\|type ExperienceReport struct\|type HealRecord struct\|type SelfCheckResult struct\|type SessionStatus " internal/` 仅在 `biz/types/` 中找到
- [ ] 6.4 事件类型唯一性验证：确认所有 EnvelopeType 常量仅在 `internal/event/envelope.go` 中定义
  - DoD: `grep -r "EnvelopeType" internal/ | grep "= " | grep -v "envelope.go"` 返回 0 结果
- [ ] 6.5 工具数量验证：确认 Spirit Agent 仅暴露 3 个粗粒度工具
  - DoD: `systemBuiltinTools()` 返回的工具列表长度为 7（3 Spirit + 4 Skills Butler），无细粒度编排工具
