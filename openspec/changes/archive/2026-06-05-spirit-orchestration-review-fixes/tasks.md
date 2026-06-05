# Tasks: spirit-orchestration-review-fixes

## Phase 1: 阻断级修复

### T1.1: 修复 task_orchestrator_impl.go 错误处理 ✅
- **ID**: T1.1
- **Spec**: error-handling-fix/REQ-EH-01
- **Description**: 将 8 处 fmt.Errorf 业务错误改为 kerrors（InternalServer/NotFound/BadRequest）
- **Files**: internal/agent/task_orchestrator_impl.go
- **Acceptance**: 所有业务错误返回 kerrors 类型；grep 无 fmt.Errorf 返回业务错误
- **Test**: 单元测试验证错误类型和 HTTP 状态码
- **Status**: [x] 已实现

### T1.2: 修复 dag_graph_compiler.go 和 agent_as_tool.go 错误处理 ✅
- **ID**: T1.2
- **Spec**: error-handling-fix/REQ-EH-02, REQ-EH-03
- **Description**: dag_graph_compiler.go 行 35 改为 kerrors.BadRequest；agent_as_tool.go 行 24 改为 kerrors.NotFound
- **Files**: internal/agent/dag_graph_compiler.go, internal/agent/agent_as_tool.go
- **Acceptance**: 参数校验错误返回 400；Agent 未找到返回 404
- **Test**: 单元测试
- **Status**: [x] 已实现

### T1.3: 修复 DAG Definition JSON 未替换 Team DefinitionJSON ✅
- **ID**: T1.3
- **Spec**: dag-definition-fix/REQ-DD-01, REQ-DD-02
- **Description**: 在 orchestrateDAG() 中，assembler.AssembleTeam() 后将 defJSON 写入 Team.DefinitionJSON 并重新编译
- **Files**: internal/agent/task_orchestrator_impl.go
- **Acceptance**: DAG 编译后的 Definition JSON 替换 Team 的 DefinitionJSON；Team 可正常启动执行
- **Test**: 集成测试验证 DAG Definition 写入和 Team 运行
- **Status**: [x] 已实现（通过 SpiritTeamUsecase.UpdateTeamDefinitionJSON 写入，失败时非致命降级）

### T1.4: Service 层业务逻辑下沉到 biz ✅
- **ID**: T1.4
- **Spec**: service-biz-refactor/REQ-SB-01~04
- **Description**: 将 recordTeamCompletion/scheduleDependentTeams/checkAllTeamsCompleted 从 spirit_team.go 移到 biz.SpiritTeamUsecase
- **Files**: internal/service/spirit_team.go, internal/biz/spirit_team_usecase.go
- **Acceptance**: Service 层无业务逻辑；biz 层方法可独立测试；make wire && make build 通过
- **Test**: biz 层单元测试；Service 层委托测试
- **Status**: [x] 已实现（Service 层 TeamStarter 委托调用 biz.SpiritTeamUsecase 的 RecordTeamCompletion/ScheduleDependentTeams/CheckAllTeamsCompleted）

## Phase 2: 建议级修复

### T2.1: 补充 spirit_trace_id 在 ChatOrchestrator turn 入口生成 ✅
- **ID**: T2.1
- **Spec**: observability-trace-fix/REQ-OT-01, REQ-OT-02
- **Description**: 在 ChatOrchestrator 处理 Spirit session 的 turn 入口生成 spirit_trace_id 并注入 context
- **Files**: internal/service/chat_orchestrator.go
- **Acceptance**: simple/moderate/complex 路径的日志均携带 spirit_trace_id
- **Test**: 日志输出验证测试
- **Status**: [x] 已实现（chat_orchestrator_turn.go 行 61-65，在 turn 入口检查并生成 spirit_trace_id）

### T2.2: 更新 spirit profile 工具名引用 ⚠️
- **ID**: T2.2
- **Spec**: tool-name-update/REQ-TN-01
- **Description**: 将 agent_effective_tools.go 中 spirit profile 的工具名从旧名更新为新名
- **Files**: internal/biz/agent_effective_tools.go
- **Acceptance**: complexAvailableTools/moderateAvailableTools 引用新工具名
- **Test**: 编译验证
- **Status**: [x] 部分实现（spirit profile 现在同时包含新旧工具名：plan_and_execute, check_progress, cancel_orchestration + assemble_team, check_team_progress, cancel_team，双写过渡期）

### T2.3: 修复前端 features→components 反向依赖 ✅
- **ID**: T2.3
- **Spec**: frontend-deps-fix/REQ-FD-01
- **Description**: 将 buildGraphFromDefinition 从 components/teams/teamUtils.ts 下移到 features/teams/
- **Files**: web/src/features/orchestration/teamGraphAdapter.ts, web/src/components/teams/teamUtils.ts, web/src/features/teams/graphUtils.ts (新增)
- **Acceptance**: features 不再 import components；pnpm build 通过
- **Test**: 前端编译验证
- **Status**: [x] 已实现（buildGraphFromDefinition 已移至 features/teams/graphUtils.ts，components/teams/teamUtils.ts 改为从 features 重新导出）

### T2.4: 标记技术债务 ✅
- **ID**: T2.4
- **Spec**: tech-debt-markers/REQ-TD-01~05
- **Description**: 为 DEV-02/03/05/06/07 添加 `// TODO(debt):` 标记
- **Files**: internal/agent/task_orchestrator_impl.go, internal/service/spirit_team.go, internal/biz/agent_capability.go, internal/agent/agent_allocator_impl.go
- **Acceptance**: 所有未完成实现有 TODO(debt) 标记
- **Test**: grep 验证
- **Status**: [x] 已实现（DEV-02/03/05/07 均有 TODO(debt) 标记）

## Phase 3: 验证

### T3.1: 全量验证
- **ID**: T3.1
- **Spec**: all
- **Description**: 运行完整验证：make api && make wire && make build && make test && make lint；前端：cd web && pnpm lint && pnpm test && pnpm build
- **Files**: all
- **Acceptance**: 全部验证通过
- **Test**: CI 验证
