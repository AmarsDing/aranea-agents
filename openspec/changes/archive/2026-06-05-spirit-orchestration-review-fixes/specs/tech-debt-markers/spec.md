# Tech Debt Markers

## Overview
为未完成实现添加 `// TODO(debt):` 标记和 issue 编号，符合 CS-B17 编程规范。

## ADDED Requirements

### Requirement: DEV-02 Graph Checkpoint 恢复不完整
- TaskOrchestrator.Recover() SHALL 添加 TODO(debt) 标记标识未完成实现。
- 在 task_orchestrator_impl.go Recover() 方法添加 `// TODO(debt): DEV-02` 标记

#### Scenario: Checkpoint 恢复标记为技术债务
- Given TaskOrchestrator.Recover() 仅标记状态为 running
- When 开发者阅读代码
- Then 可通过 TODO(debt) 标记识别此为已知未完成实现

### Requirement: DEV-06 Team 超时检测未实现
- ParallelConfig.TeamTimeoutSeconds 字段 SHALL 添加 TODO(debt) 标记标识未使用功能。
- 在 spirit_parallel_config.go TeamTimeoutSeconds 字段添加 `// TODO(debt): DEV-06` 标记

#### Scenario: Team 超时标记为技术债务
- Given ParallelConfig.TeamTimeoutSeconds 已定义但未使用
- When 开发者阅读代码
- Then 可通过 TODO(debt) 标记识别此为已知未完成实现

### Requirement: DEV-07 Phase 1/2 中断恢复未实现
- RecoverAllInterrupted() 方法 SHALL 添加 TODO(debt) 标记标识未完成实现。
- 在 task_orchestrator_impl.go RecoverAllInterrupted() 方法添加 `// TODO(debt): DEV-07` 标记

#### Scenario: Phase 1/2 恢复标记为技术债务
- Given RecoverAllInterrupted() 仅恢复 OrchestrationHandle
- When 开发者阅读代码
- Then 可通过 TODO(debt) 标记识别此为已知未完成实现

### Requirement: DEV-03 AgentCapability.Capacity 未使用
- AgentCapability.Capacity 字段 SHALL 添加 TODO(debt) 标记标识未使用功能。
- 在 agent_capability.go Capacity 字段添加 `// TODO(debt): DEV-03` 标记

#### Scenario: Capacity 字段标记为技术债务
- Given AgentCapability.Capacity 已定义但无冲突检测逻辑
- When 开发者阅读代码
- Then 可通过 TODO(debt) 标记识别此为已知未完成实现

### Requirement: DEV-05 Layer 2 TF-IDF 占位
- computeSemanticScore() 函数 SHALL 添加 TODO(debt) 标记标识占位实现。
- 在 agent_allocator_impl.go computeSemanticScore() 函数添加 `// TODO(debt): DEV-05` 标记

#### Scenario: TF-IDF 占位标记为技术债务
- Given computeSemanticScore() 使用 TF-IDF 而非 embedding
- When 开发者阅读代码
- Then 可通过 TODO(debt) 标记识别此为已知占位实现
