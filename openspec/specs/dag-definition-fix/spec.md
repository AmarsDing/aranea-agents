# dag-definition-fix Specification

## Purpose
TBD - created by archiving change spirit-orchestration-review-fixes. Update Purpose after archive.
## Requirements
### Requirement: orchestrateDAG 中写入编译后的 Definition JSON
- TaskOrchestrator SHALL 在 DAG 编译成功后将 defJSON 写入 Team 的 DefinitionJSON 字段。
- 在 `orchestrateDAG()` 中，assembler.AssembleTeam() 创建 Team 后，将 DAG 编译的 defJSON 写入 Team 的 DefinitionJSON 字段
- 需要通过 SpiritTeamUsecase.UpdateTeamDefinitionJSON 更新
- 更新失败为非致命错误（Team 已有有效 definition），仅记录日志

#### Scenario: DAG 编译后的 Definition JSON 正确写入 Team
- Given TaskOrchestrator 执行 DAG 策略编排
- When assembler 创建 Team 成功且 DAG 编译成功
- Then 编译后的 defJSON 写入 Team 的 DefinitionJSON 字段
- And 如果写入失败，记录警告日志但不中断流程

### Requirement: SpiritTeamUsecase 新增 UpdateTeamDefinitionJSON 方法
- SpiritTeamUsecase SHALL 提供 UpdateTeamDefinitionJSON 方法用于更新 Team 的 DefinitionJSON。
- 方法签名：`UpdateTeamDefinitionJSON(ctx context.Context, teamID string, definitionJSON string) error`
- 通过 TeamUsecase.Update 更新 Team 的 DefinitionJSON

#### Scenario: 更新 Team Definition JSON
- Given 存在一个已创建的 Team
- When 调用 UpdateTeamDefinitionJSON 传入新的 definitionJSON
- Then Team 的 DefinitionJSON 被更新并持久化

