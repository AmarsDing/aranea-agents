## Why

spirit-orchestration-redesign 变更已归档，但代码审查发现 3 个阻断级问题和 10 个建议级问题，涵盖错误处理、架构合规、Agent 运行时集成和前端分层四个维度。阻断项会导致 HTTP 状态码语义丢失、DAG 编译核心路径未生效、Service 层业务逻辑泄漏等严重问题。

## What Changes

- **修复 task_orchestrator_impl.go 中 8 处 fmt.Errorf 业务错误**：改为 kerrors，恢复 HTTP 状态码语义
- **修复 DAG 编译后的 Definition JSON 未替换 Team DefinitionJSON**：orchestrateDAG() 中将编译产物写入 Team，使 DAG 编译核心路径生效
- **将 Service 层业务逻辑下沉到 biz 层**：recordTeamCompletion（DQ Score/拓扑推断/进化建议）、scheduleDependentTeams（DAG 依赖调度）、checkAllTeamsCompleted 抽取到 biz.SpiritTeamUsecase
- **修复 dag_graph_compiler.go 和 agent_as_tool.go 中 fmt.Errorf 业务错误**：改为 kerrors.BadRequest/kerrors.NotFound
- **补充 spirit_trace_id 在 ChatOrchestrator turn 入口生成**：修复 DEV-11，确保 simple/moderate 路径也有 trace ID
- **更新 spirit profile 工具名引用**：修复 DEV-04，将旧工具名更新为新工具名
- **修复前端 features→components 反向依赖**：将 buildGraphFromDefinition 下移到 features/teams/
- **标记未完成实现为技术债务**：DEV-02（Checkpoint 恢复）、DEV-06（Team 超时检测）、DEV-07（Phase 1/2 中断恢复）

## Capabilities

### Modified Capabilities

- `task-orchestrator-impl`: 修复 8 处 fmt.Errorf 业务错误为 kerrors；修复 DAG Definition JSON 未替换 Team DefinitionJSON
- `spirit-team-service`: 将 recordTeamCompletion/scheduleDependentTeams/checkAllTeamsCompleted 业务逻辑下沉到 biz.SpiritTeamUsecase
- `dag-compiler`: 修复 fmt.Errorf 为 kerrors.BadRequest
- `agent-as-tool`: 修复 fmt.Errorf 为 kerrors.NotFound
- `spirit-observability`: 补充 spirit_trace_id 在 ChatOrchestrator turn 入口生成
- `spirit-tools-config`: 更新 agent_effective_tools.go 中 spirit profile 工具名引用
- `orchestration-frontend`: 修复 features/orchestration/teamGraphAdapter.ts 反向依赖

## Impact

- **biz 层**：SpiritTeamUsecase 新增 3 个方法（RecordTeamCompletion/ScheduleDependentTeams/CheckAllTeamsCompleted）
- **service 层**：spirit_team.go 删除业务逻辑方法，改为调用 biz 层
- **agent 层**：task_orchestrator_impl.go 修复错误处理 + DAG Definition 写入
- **前端**：teamGraphAdapter.ts 修改 import 路径
- **Wire**：如 SpiritTeamUsecase 构造函数签名变化则需重新 make wire
