# Spirit-orchestration-redesign 审计修复 — 任务清单

**Goal**: 补齐原 spirit-orchestration-redesign 变更的 4 项缺口。

**Non-goals:**
- 不改变 Spirit 整体架构

---

## 1. Checkpoint 重建

- [ ] 1.1 在 `internal/biz/spirit_orchestrator.go` 的 `RestoreFromCheckpoint` 中实现 GraphAgent 重建。DoD: 重建后关键状态字段与 checkpoint 一致
- [ ] 1.2 编写单元测试。DoD: `go test ./internal/biz/... -run TestCheckpointRestore -count=1` 绿色

## 2. Layer 2 语义匹配评估

- [ ] 2.1 评估 Embedding 服务可用性（检查项目是否已有 Embedding 调用基础设施）。DoD: 评估报告输出
- [ ] 2.2 如果可用：实现 Embedding 替换 TF-IDF。如果不可用：在代码中添加 `TODO(embedding-upgrade)` 注释。DoD: 决策落地

## 3. AgentPerformance 集成

- [ ] 3.1 在 Spirit 编排选择 Agent 时，优先调用 `GetBestForTaskType`，fallback 到轮询。DoD: `go build ./internal/biz/...` 通过
- [ ] 3.2 编写单元测试。DoD: `go test ./internal/biz/... -run TestAgentPerformance -count=1` 绿色

## 4. 集成测试

- [ ] 4.1 编写 Spirit 编排端到端集成测试。DoD: `go test ./internal/service/... -run TestSpiritIntegration -count=1` 绿色

## 5. 全量验证

- [ ] 5.1 运行 `make build && make test && make lint`。DoD: 全部通过
