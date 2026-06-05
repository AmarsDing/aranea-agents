## MODIFIED Requirements

### Requirement: Husky/lint-staged/commitlint 配置
原要求安装 Husky + lint-staged + commitlint。现改为**正式弃用**，从 tasks.md 移除相关步骤，保留 `.husky/` 目录但标记为 deprecated。

#### Scenario: 弃用 Husky
- **WHEN** 开发者 commit 代码
- **THEN** 不触发 pre-commit/commit-msg hook，依赖 CI lint 检查

### Requirement: 集成测试骨架
原要求 chat_integration_test.go / agent_integration_test.go。现改为**补齐实际断言**。

#### Scenario: 集成测试断言
- **WHEN** 运行 `go test ./internal/service/... -run TestIntegration -count=1`
- **THEN** 测试启动容器、调用 API、验证响应状态码和关键字段
