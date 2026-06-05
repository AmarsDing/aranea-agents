## MODIFIED Requirements

### Requirement: Checkpoint 恢复
原仅标记状态。现改为加载 checkpoint 后重建 GraphAgent 实例。

#### Scenario: Checkpoint 重建
- **WHEN** SpiritOrchestrator.RestoreFromCheckpoint 被调用
- **THEN** 重建 GraphAgent 实例，恢复对话上下文，关键状态字段与 checkpoint 一致

### Requirement: Layer 2 语义匹配
原用 TF-IDF 占位。现改为评估 Embedding 升级可行性，不可用时标记 tech-debt。

#### Scenario: Embedding 可用
- **WHEN** Embedding 服务可用
- **THEN** Layer 2 使用 Embedding 计算语义相似度

#### Scenario: Embedding 不可用
- **WHEN** Embedding 服务不可用
- **THEN** 保留 TF-IDF，代码中添加 TODO(embedding-upgrade)

### Requirement: AgentPerformance 集成
原 GetBestForTaskType 未被调用。现改为 Spirit 编排选择 Agent 时优先使用。

#### Scenario: Agent 选择
- **WHEN** Spirit 编排需要选择 Agent
- **THEN** 优先使用 GetBestForTaskType 结果，fallback 到轮询
