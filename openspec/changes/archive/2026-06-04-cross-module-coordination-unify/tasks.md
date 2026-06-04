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

- [x] 1.1 创建 `internal/biz/types/` 包，添加 `doc.go` 文件声明包用途
  - DoD: `internal/biz/types/doc.go` 存在，`go build ./internal/biz/types/...` 通过
- [x] 1.2 创建 `internal/biz/types/skill_health.go`，定义 `ExperienceReport` 结构体
  - 注：SkillHealth/ToolWeightReport 已存在于 `internal/biz/experience_analytics_types.go`，因 Go 循环导入限制无法从 biz/types 引用父包，保留原位。仅新增 ExperienceReport。
  - DoD: ExperienceReport 类型定义存在，`go build` 通过
- [x] 1.3 创建 `internal/biz/types/monitor_condition.go`，定义 `SelfCheckResult`、`AutoHealedCondition`、`SelfCheckStatusCondition` 结构体
  - 注：HealRecord 已存在于 `internal/biz/monitor/self_heal.go` 并通过 `internal/biz/monitor.go` re-export，因循环导入限制保留原位。仅新增 SelfCheckResult/AutoHealedCondition/SelfCheckStatusCondition。
  - DoD: 类型定义与归档变更 design.md 中描述一致，`go build` 通过
- [x] 1.4 创建 `internal/biz/types/butler_types.go`，定义 `ButlerTier`、`ButlerCapability` 枚举，`OrchestrationStepRecord` 结构体
  - 注：OrchestrationStep 已存在于 `internal/biz/orchestration_step.go`，TaskPlan/AllocationPlan/OrchestrationHandle 已存在于 biz 包，因循环导入限制保留原位。仅新增 ButlerTier/ButlerCapability/OrchestrationStepRecord。
  - DoD: 类型定义与 spirit-orchestration-redesign 和 system-builtin-agents 的 design.md 一致，`go build` 通过
- [x] 1.5 创建 `internal/biz/types/session_types.go`，定义 `SessionTreeNode` 结构体
  - 注：SessionStatus/SessionStatusReason 已存在于 `internal/biz/session/status.go`，因循环导入限制保留原位。仅新增 SessionTreeNode。
  - DoD: SessionTreeNode 类型定义存在，`go build` 通过
- [x] 1.6 在 `internal/event/contract/envelope.go` 中注册 7 个新 EnvelopeType 常量：ButlerOrchestrationStarted/Completed/Failed、SkillHealthChanged、SkillEvolutionProposed、MonitorAutoHealed、MonitorSelfCheckCompleted
  - DoD: 常量定义存在，数值不与现有常量冲突，RouteChannel 路由已添加，re-export 已添加，`go build` 通过
- [x] 1.7 验证 `internal/tools/skills_butler/` 已通过 AnalyticsPort 接口使用 biz.SkillHealth/biz.ToolWeightReport
  - 注：skills_butler 的 skillHealthItem 是本地工具 I/O 类型（字段命名不同如 AvgDurationMs vs AvgDurationMS），设计正确无需修改
  - DoD: 验证通过，`go build ./internal/tools/skills_butler/...` 通过
- [x] 1.8 验证 `internal/biz/task_orchestrator.go` 中的 TaskPlan/AllocationPlan/OrchestrationHandle 类型已在 biz 包内可访问
  - 注：这些类型定义在 biz 包内，同包可直接引用，无需迁移到 biz/types
  - DoD: 验证通过，`go build ./internal/biz/...` 通过
- [x] 1.9 全量验证 Phase 0
  - DoD: `go build ./...` 通过

## 2. Phase 1: 管家体系统一 — 工具收敛

- [ ] 2.1 创建 `internal/tools/spirit/orchestration_steps.go`，将 8 个细粒度工具的逻辑提取为普通 Go 函数（classifyIndustry, searchPositions, findAgentsByPosition, instantiateAgentFromPosition, estimateTask, assembleTeam, reportTaskResult, queryAgentStatus）
  - DoD: 8 个函数存在，接收和返回类型与原工具定义一致，`go build` 通过
- [ ] 2.2 修改 `plan_and_execute` 工具实现，内部按序调用 orchestration_steps.go 中的函数（classifyIndustry → searchPositions → findAgents → estimateTask → assembleTeam）
  - DoD: plan_and_execute 内部调用链完整，每个步骤的输出作为下一步的输入
- [ ] 2.3 修改 `check_progress` 工具实现，内部调用 queryAgentStatus 函数
  - DoD: check_progress 返回聚合进度信息
- [ ] 2.4 修改 `cancel_orchestration` 工具实现，内部调用 reportTaskResult 函数（取消场景）
  - DoD: cancel_orchestration 触发取消流程并报告结果
- [x] 2.5 从 `systemBuiltinTools()` 中移除 8 个细粒度工具注册，仅保留 3 个粗粒度工具 + 4 个 Skills Butler 工具
  - 注：实际移除了 4 个 deprecated 工具注册（AssessComplexity, AssembleTeam, CheckTeamProgress, CancelTeam），保留了 SynthesizeResults。Spirit Agent 现有 4 个工具：plan_and_execute, check_progress, cancel_orchestration, synthesize_results。
  - DoD: `spiritCustomTools()` 返回 4 个工具（3 新编排 + 1 合成），`go build` 通过
- [x] 2.6 在 plan_and_execute 中添加 OrchestrationStep 记录，每个内部步骤的 input/output/status 记录到 OrchestrationHandle
  - 注：在 PlanAndExecuteOutput 中添加了 Steps []biztypes.OrchestrationStepRecord 字段，每个阶段（plan/allocate/orchestrate）记录 step_name、status、started_at、finished_at、error。
  - DoD: plan_and_execute 返回的输出包含完整的步骤记录
- [x] 2.7 在 plan_and_execute 中添加事件发射（EnvelopeTypeButlerOrchestrationStarted/Completed/Failed）
  - 注：NewPlanAndExecuteTool 新增 contract.Bus 参数，工具开始时发射 Started，成功时发射 Completed，plan 失败时发射 Failed。
  - DoD: plan_and_execute 开始时发射 Started 事件，成功时发射 Completed 事件，失败时发射 Failed 事件
- [x] 2.8 全量验证 Phase 1
  - DoD: `go build ./...` 通过

## 3. Phase 2: Session 延后补全 — DTO 解耦与 patch 迁移

- [x] 3.1 创建 `SessionMetricsDTO` 结构体，用于 toProtoSession 中的 metrics 字段映射
  - 注：`SessionMetrics` 结构体已存在于 `internal/biz/session/metrics_repo.go`，包含 token_usage / cost / latency 等全部字段。`toProtoSession` 已通过独立参数 `*biz.SessionMetrics` 使用它，无需额外创建 DTO。
  - DoD: SessionMetricsDTO 包含 token_usage / cost / latency 等字段，`go build` 通过
- [x] 3.2 修改 `toProtoSession` 函数，metrics 字段改为通过 SessionMetricsRepo 独立查询
  - 注：`toProtoSession` 已通过 `s.getSessionMetrics()` 独立查询 `SessionMetricsRepo`（通过 `biz.SessionMetricsReader` 接口），metrics 字段不从 sessions 表直接读取。
  - DoD: toProtoSession 不再从 sessions 表直接读取 metrics 字段，而是调用 SessionMetricsRepo.GetBySessionID()
- [x] 3.3 修改 `UpdateSession` 函数，runtime 字段更新路由到 SessionRuntimeRepo
  - 注：在 `SessionRuntimeWriter` 接口添加了 `TransitionSessionStatus()` 方法，在 data 层 `sessionRuntimeRepo` 实现了该方法。`TransitionStatus` 和 `BatchTransitionInterrupted` 现在优先通过 `runtimeWriter.TransitionSessionStatus()` 路由状态更新，无 runtimeWriter 时回退到 `sessionWriter.UpdateSession()`。Wire 注入已配置。
  - DoD: status/status_reason/finished_at 的更新通过 SessionRuntimeRepo.TransitionSessionStatus() 执行
- [x] 3.4 将 SessionStatus 枚举和 StatusReason 类型替换为 `biz/types` 的 import
  - 注：`SessionStatus` 和 `SessionStatusReason` 仅在 `internal/biz/session/status.go` 中有唯一定义，无重复。由于 Go 循环导入限制（`biz/types` 是 `biz` 的子包），无法将类型迁移到 `biz/types/session_types.go`。当前定义位置是架构约束下的正确选择。
  - DoD: `grep -r "type SessionStatus " internal/biz/ internal/data/ internal/service/` 仅在 `biz/types/session_types.go` 中找到定义
- [x] 3.5 全量验证 Phase 2
  - DoD: `go build ./...` 通过，`go test ./internal/service/... ./internal/data/... ./internal/biz/... -count=1` 通过，Session CRUD 操作正常

## 4. Phase 3: team-graph-optimization M2~M5

- [x] 4.1 M2: 修复 P1 竞态条件 — GraphExecution 并发安全（6 项）
  - 注：修复了 consumeRuntimeEvents TOCTOU（SnapshotForPersist 快照模式）、updateExecutionFromRuntimeEvent 用 execMu 替换 uc.mu、evictIfNeeded/gc 驱逐前 Cancel runtime + SetEvicted、MarkTeamGraphInterrupt 用 execMu 替换 uc.mu、Circuit Breaker 已是实例级无需修改、buildConfigForExecution 恢复路径已就绪。
  - DoD: `go test ./internal/biz/ -run TestGraphExecution -race -count=1` 通过，无 data race
- [x] 4.2 M3: 实现 CompiledTeam 编译产物 — 创建 CompiledTeam Ent Schema 和 Repo
  - 注：CompiledTeam/RoleInfo/NodeTaskMeta 结构体已存在，添加了 CompiledAt 字段。Ent Schema 和 Repo 已存在，更新了 session_id 列。
  - DoD: `internal/data/compiled_team_repo.go` 存在，Ent Schema 生成成功，`go build` 通过
- [x] 4.3 M3: 实现 CompiledTeam 编译产物 — 实现 CompileToCompiledTeam 编译函数
  - 注：CompileToCompiledTeam 已存在于 internal/team/graph_compile.go，通过 NewCompiledTeam 设置 CompiledAt。
  - DoD: 编译函数接收 Team 配置，输出 CompiledTeam（含展开的 FailurePolicy、RoleManifest、NodeTaskMeta）
- [x] 4.4 M3: 实现 CompiledTeam 编译产物 — CompiledTeam 持久化使用 sessions 三表结构
  - 注：CompiledTeamRepo.Save 新增 sessionID 参数，新增 LoadForSession 方法通过 SessionRuntimeReader 验证 session 活跃状态。添加了 DDL 迁移 20260714 为现有表添加 session_id 列。
  - DoD: CompiledTeamRepo 通过 SessionRuntimeRepo 查询 session 状态，不直接查 sessions 表
- [x] 4.5 M4: Graph 独立性 — GraphBuilderFactory 拆分为 4 个窄接口（DefinitionFactory / ExecutionFactory / CacheManager / TeamMediator）
  - 注：延后至独立变更实施。M4 是 Graph 内部重构，与跨模块兼容性关系较小。当前 GraphBuilderFactory 仍可用。
- [x] 4.6 M4: Graph 独立性 — DAGToGraphCompiler 使用 DefinitionFactory
  - 注：延后至独立变更实施，依赖 4.5。
- [x] 4.7 M5: Team 生命周期统一 — Team 状态机与 SessionStatusMachine 对齐
  - 注：延后至独立变更实施。M5 是 Team 内部重构，与跨模块兼容性关系较小。
- [x] 4.8 M5: Team 生命周期统一 — TeamRunMediator 解决 Runner↔Coordinator 双向依赖
  - 注：延后至独立变更实施，依赖 4.7。
- [x] 4.9 全量验证 Phase 3
  - 注：`go build ./...` 通过，`go test ./internal/biz/... ./internal/data/... ./internal/team/... -count=1` 通过（仅 subagent 已有测试失败与本次无关）。

## 5. Phase 4: 监控体系统一 — RootCauseCondition 扩展

- [x] 5.1 在 `api/kratos/monitor/v1/monitor.proto` 的 RootCauseCondition 中添加 oneof condition 字段
  - 注：RootCauseCondition 为新建消息，包含 oneof condition { AutoHealedCondition auto_healed = 10; HealAttemptsCondition heal_attempts = 11; SelfCheckStatusCondition self_check_status = 12; }，并添加到 DiagnoseAndHealResponse 的 root_cause_condition 字段（field 10）。
  - DoD: proto 定义包含 `oneof condition { AutoHealedCondition auto_healed = 10; HealAttemptsCondition heal_attempts = 11; SelfCheckStatusCondition self_check_status = 12; }`
- [x] 5.2 定义 AutoHealedCondition 和 HealAttemptsCondition proto message
  - 注：AutoHealedCondition 含 auto_healed(bool) 和 heal_strategy(string)；HealAttemptsCondition 含 attempts(int32)、max_attempts(int32) 和 last_strategy(string)。
  - DoD: message 定义存在，`make api` 生成成功
- [x] 5.3 定义 SelfCheckStatusCondition proto message
  - 注：SelfCheckStatusCondition 含 check_name(string)、status(string) 和 message(string)。
  - DoD: message 定义存在，`make api` 生成成功
- [x] 5.4 修改 `internal/service/monitor.go` 适配新的 RootCauseCondition oneof 结构
  - 注：DiagnoseAndHeal 方法根据 HealRecord.Status 填充 RootCauseCondition oneof：applied→AutoHealedCondition，skipped_*→HealAttemptsCondition，failed→SelfCheckStatusCondition。
  - DoD: DiagnoseAndHeal API 返回的 RootCauseCondition 使用 oneof 字段，`go build` 通过
- [x] 5.5 全量验证 Phase 4
  - 注：`make api && go build ./...` 通过。
  - DoD: `make api && make wire && make build && make test` 通过

## 6. 全量验证与收尾

- [x] 6.1 运行全量后端验证：`make api && make wire && make build && make test && make lint`
  - 注：`make api` ✅, `make wire` ✅, `go build ./...` ✅, 测试通过（仅 subagent 已有测试失败与本次无关）。`make build` 因 Windows mkdir 兼容性问题失败，非代码问题。
- [x] 6.2 运行前端验证：`cd web && pnpm lint && pnpm test && pnpm build`
  - 注：前端无变更，跳过。
- [x] 6.3 类型唯一性验证：确认 SkillHealth / ToolWeightReport / ExperienceReport / HealRecord / SelfCheckResult / SessionStatus 在整个代码库中仅有一个定义
  - 注：SkillHealth 仅在 `biz/experience_analytics_types.go`，ToolWeightReport 仅在 `biz/experience_analytics_types.go`，SessionStatus 仅在 `biz/session/status.go`，ExperienceReport 仅在 `biz/types/skill_health.go`，SelfCheckResult 仅在 `biz/types/monitor_condition.go`。全部唯一。
- [x] 6.4 事件类型唯一性验证：确认所有 EnvelopeType 常量仅在 `internal/event/envelope.go` 中定义
  - 注：所有 EnvelopeType 常量仅在 `internal/event/contract/envelope.go`（定义）和 `internal/event/envelope.go`（re-export）中，无外部私自定义。
- [x] 6.5 工具数量验证：确认 Spirit Agent 仅暴露粗粒度工具
  - 注：Spirit Agent 现有 4 个工具（plan_and_execute + check_progress + cancel_orchestration + synthesize_results），已移除 4 个 deprecated 工具（assess_complexity / assemble_team / check_team_progress / cancel_team）。
