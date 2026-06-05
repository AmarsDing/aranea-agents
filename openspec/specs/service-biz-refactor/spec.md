# service-biz-refactor Specification

## Purpose
TBD - created by archiving change spirit-orchestration-review-fixes. Update Purpose after archive.
## Requirements
### Requirement: SpiritTeamUsecase 构造函数新增依赖
- SpiritTeamUsecase 构造函数 SHALL 新增 orchCache 和 evolutionSugg 依赖参数。
- 新增 `orchCache *OrchestrationCache` 和 `evolutionSugg EvolutionSuggestionRepo` 参数
- NewSpiritTeamUsecase 签名更新为 7 个参数

#### Scenario: SpiritTeamUsecase 可访问编排缓存和进化建议
- Given SpiritTeamUsecase 被构造
- When 需要记录 DQ Score 或创建进化建议
- Then 通过 orchCache 和 evolutionSugg 依赖执行

### Requirement: RecordTeamCompletion 下沉到 biz
- RecordTeamCompletion 逻辑 SHALL 从 Service 层下沉到 biz.SpiritTeamUsecase。
- 将 DQ Score 计算、拓扑推断、进化建议创建逻辑抽取到 `biz.SpiritTeamUsecase.RecordTeamCompletion()`
- Service 层 `recordTeamCompletion` 改为委托调用 `s.team.SpiritUC.RecordTeamCompletion(ctx, team, durationMs)`

#### Scenario: 团队完成时记录 DQ Score 和进化建议
- Given 一个 Spirit 团队完成执行
- When Service 层调用 recordTeamCompletion
- Then biz 层计算 DQ Score、推断拓扑、记录缓存、创建进化建议

### Requirement: ScheduleDependentTeams 下沉到 biz
- ScheduleDependentTeams 逻辑 SHALL 从 Service 层下沉到 biz.SpiritTeamUsecase。
- 将 DAG 依赖解析、依赖失败传播、团队激活调度逻辑抽取到 `biz.SpiritTeamUsecase.ScheduleDependentTeams()`
- biz 层返回 `[]DependentTeamAction`（activate/fail 动作列表）
- Service 层执行动作（状态转换 + 事件发布 + Runner 启动）

#### Scenario: DAG 依赖调度返回动作列表
- Given 一个团队完成且有 DAG 依赖关系
- When Service 层调用 scheduleDependentTeams
- Then biz 层返回需要激活或失败的动作列表
- And Service 层执行状态转换、发布事件、启动 Runner

### Requirement: CheckAllTeamsCompleted 下沉到 biz
- CheckAllTeamsCompleted 逻辑 SHALL 从 Service 层下沉到 biz.SpiritTeamUsecase。
- 将全部团队完成检查逻辑抽取到 `biz.SpiritTeamUsecase.CheckAllTeamsCompleted()`
- biz 层返回 `AllTeamsCompletedResult{AllDone bool, TeamIDs []string}`
- Service 层根据结果发布事件

#### Scenario: 检查所有团队是否完成
- Given 一个 Spirit session 的所有团队处于终态
- When Service 层调用 checkAllTeamsCompleted
- Then biz 层返回 AllDone=true 和 teamIDs
- And Service 层发布 SpiritTeamsAllCompleted 事件

### Requirement: TeamStarter 构造函数简化
- TeamStarter 构造函数 SHALL 移除已移至 biz 层的 orchCache 和 evolutionSugg 参数。
- 移除 `orchCache` 和 `evolutionSugg` 参数（已移到 biz 层）
- NewTeamStarter 签名从 6 参数简化为 4 参数

#### Scenario: TeamStarter 不再持有业务逻辑依赖
- Given TeamStarter 被构造
- When 需要记录完成或调度依赖
- Then 通过 SpiritTeamUsecase 委托执行

