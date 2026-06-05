## Why

原 `2026-06-05-spirit-orchestration-redesign` 变更归档时 82.6% 完成（19/23），4 项已知缺口：Checkpoint 加载后 GraphAgent 重建未实现、Layer 2 用 TF-IDF 占位、AgentPerformance.GetBestForTaskType 未集成、集成测试未验证。

## What Changes

- 实现 Checkpoint 加载后 GraphAgent 重建（当前仅标记状态）
- 将 TF-IDF 占位升级为 Embedding（或正式标记为 tech-debt）
- 集成 AgentPerformance.GetBestForTaskType 到 Spirit 编排
- 补齐集成测试

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `spirit-orchestration-redesign`: 补齐 4 项缺口

## Impact

- **biz 层**: `internal/biz/spirit_orchestrator.go` Checkpoint 重建逻辑
- **tools 层**: `internal/tools/spirit/` Layer 2 升级
- **biz 层**: `internal/biz/agent_performance.go` 集成

## Non-goals

- 不改变 Spirit 整体架构
- TF-IDF → Embedding 升级可能 defer（需评估 Embedding 服务可用性）
