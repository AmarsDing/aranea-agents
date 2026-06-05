## MODIFIED Requirements

### Requirement: Team 创建时支持 agent_key 成员引用
TeamUsecase SHALL 支持在创建/更新 Team 时，成员通过 agent_key 引用而非仅通过 agent_id。

**注意**：`TeamUsecase.SaveTeamWithGraph` 方法已实现，但 Pack 导入引擎实际通过 `ImporterRepo` → `PackRepoAdapter` → 底层 Repo 直接写入，不经过 TeamUsecase。此方法为其他场景预留。

#### Scenario: 成员 agent_key 解析
- **WHEN** Pack 导入引擎创建 Team，members 中包含 agent_key 字段
- **THEN** 系统 SHALL 将 agent_key 解析为 agent_id，填充到 OrchestrationMember.AgentID

#### Scenario: agent_key 解析失败
- **WHEN** 成员引用的 agent_key 在目标系统不存在
- **THEN** 系统 SHALL 返回校验错误，列出未找到的 agent_key

### Requirement: Team 创建时支持 Graph 关联
TeamUsecase SHALL 支持在创建 Team 时关联 GraphDefinition。

**注意**：Pack 导入引擎通过 `ImporterRepo` 直接写入 Team（含 Graph 关联），不经过 TeamUsecase。

#### Scenario: linked_graph_id 设置
- **WHEN** Pack 导入引擎创建 Team 并提供 graph_id 引用
- **THEN** 系统 SHALL 将解析后的 graph_id 写入 Team 的 definition_json 的 linked_graph_id 字段

#### Scenario: 内嵌 Graph 定义写入
- **WHEN** Pack 导入引擎创建 Team 并提供内嵌 Graph 定义
- **THEN** 系统 SHALL 将 Graph 定义（节点中 agent_key 已转换为 agent_id）写入 Team 的 definition_json 的 graph 字段
