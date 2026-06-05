## Context

原 spirit-orchestration-redesign 变更归档后审计发现 4 项已知缺口。

## Goals / Non-Goals

**Goals:**
- Checkpoint 加载后重建 GraphAgent
- 评估 TF-IDF → Embedding 升级可行性
- 集成 AgentPerformance.GetBestForTaskType
- 补齐集成测试

**Non-Goals:**
- 不改变 Spirit 整体架构

## Decisions

### D1: Checkpoint 重建策略

**决策**: 在 `SpiritOrchestrator.RestoreFromCheckpoint` 中，加载 checkpoint 后重建 GraphAgent 实例，恢复对话上下文。

### D2: TF-IDF → Embedding

**决策**: 评估后决定。如果 Embedding 服务不可用，标记为 tech-debt 并在代码中添加 TODO。

### D3: AgentPerformance 集成

**决策**: 在 Spirit 编排选择 Agent 时，优先使用 GetBestForTaskType 结果。

## Risks / Trade-offs

- **[Risk] Checkpoint 重建可能丢失运行时状态** → 重建后验证关键状态字段
- **[Risk] Embedding 服务不可用** → 保留 TF-IDF 作为 fallback
