# self-iteration-engine Specification

## Purpose
TBD - created by archiving change self-iteration-remediation. Update Purpose after archive.
## Requirements
### Requirement: Husky/lint-staged/commitlint 配置
原要求安装 Husky + lint-staged + commitlint。现改为**正式弃用**，系统 SHALL 不再使用 Husky hooks 进行本地 lint 检查，MUST 依赖 CI lint 检查代替。`.husky/` 目录 SHALL 保留但标记为 deprecated。

#### Scenario: 弃用 Husky
- **WHEN** 开发者 commit 代码
- **THEN** 不触发 pre-commit/commit-msg hook，系统 SHALL 依赖 CI lint 检查

### Requirement: 集成测试骨架
原要求 chat_integration_test.go / agent_integration_test.go。现改为**补齐实际断言**。集成测试 MUST 使用 Ent Client 对真实 PostgreSQL 容器执行 CRUD 操作并验证结果，SHALL 包含创建、查询、更新、删除的完整断言。

#### Scenario: 集成测试断言
- **WHEN** 运行 `go test -tags=integration ./internal/service/... -run TestIntegration -count=1`
- **THEN** 测试 SHALL 启动容器、执行 Ent Client CRUD 操作、验证数据正确性

