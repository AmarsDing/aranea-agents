## MODIFIED Requirements

### Requirement: Agent 按 agent_key 幂等 upsert
AgentUsecase SHALL 支持通过 agent_key 进行幂等创建/更新操作，供 Pack 导入引擎使用。

**注意**：`AgentUsecase.UpsertByKey` 方法已实现，但 Pack 导入引擎实际通过 `ImporterRepo` → `PackRepoAdapter` → 底层 Repo 直接写入，不经过 AgentUsecase。此方法为其他场景（如未来前端 API）预留。

#### Scenario: agent_key 不存在时创建
- **WHEN** Pack 导入引擎调用 AgentUsecase 的 upsert 方法，agent_key 在目标系统不存在
- **THEN** 系统 SHALL 创建新 Agent，使用 Pack 中定义的 agent_key

#### Scenario: agent_key 已存在时更新
- **WHEN** Pack 导入引擎调用 AgentUsecase 的 upsert 方法，agent_key 已存在且冲突策略为 overwrite
- **THEN** 系统 SHALL 更新已有 Agent 的可修改字段，保留原 ID 和 created_at

#### Scenario: agent_key 已存在时跳过
- **WHEN** Pack 导入引擎调用 AgentUsecase 的 upsert 方法，agent_key 已存在且冲突策略为 skip
- **THEN** 系统 SHALL 跳过该 Agent，返回已有 Agent 的 ID

### Requirement: Agent 创建时支持 Prompt 文件批量写入
AgentUsecase SHALL 支持在创建 Agent 时批量写入 Prompt 文件。

**注意**：`AgentUsecase.CreateWithFilesAndSettings` 方法已实现，但 Pack 导入引擎实际通过 `ImporterRepo.ReplaceAgentPromptFiles` 直接写入，不经过此 Usecase 方法。

#### Scenario: 创建 Agent 同时写入文件
- **WHEN** Pack 导入引擎创建 Agent 并提供 files 列表
- **THEN** 系统 SHALL 在同一个事务中创建 Agent 记录和所有 Prompt 文件记录

### Requirement: Agent 创建时支持 RuntimeSettings 写入
AgentUsecase SHALL 支持在创建 Agent 时写入可移植的 RuntimeSettings。

**注意**：Pack 导入引擎通过 `ImporterRepo.UpsertAgentRuntimeSettings` 直接写入 RuntimeSettings，不经过 AgentUsecase。

#### Scenario: 创建 Agent 同时写入 RuntimeSettings
- **WHEN** Pack 导入引擎创建 Agent 并提供 runtime 配置
- **THEN** 系统 SHALL 在创建 Agent 后写入 RuntimeSettings，实例绑定字段使用默认值
