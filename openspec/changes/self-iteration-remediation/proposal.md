## Why

原 `2026-06-05-self-iteration-engine` 变更归档时 tasks.md 标记 78.6% 完成（66/84），但审计发现 17 项"假完成"（标记 [x] 但附 ⚠️ 说明实际未做）和 18 项验证步骤未执行。核心问题：Husky/lint-staged/commitlint 实际未安装（hooks 已禁用）、CI 配置名错误、staging 部署步骤缺失、集成测试仅为骨架。

## What Changes

- 修正 17 项假完成：要么真正实现，要么降级为 deferred
- 补齐 18 项验证步骤的执行证据
- 修复 Husky/lint-staged/commitlint 配置（或正式弃用并从 tasks.md 移除）
- 修复 CI 配置名错误
- 补齐集成测试骨架的实际断言
- 补齐 admin --version ldflags（commit/date）
- 补齐 staging 部署/冒烟/production promote 步骤（或正式 defer）

## Capabilities

### New Capabilities

（无新能力）

### Modified Capabilities

- `self-iteration-engine`: 修正假完成项，补齐验证步骤

## Impact

- **CI/CD**: `.github/workflows/release.yml` 可能需要修改
- **dev 工具链**: Husky/lint-staged/commitlint 配置
- **集成测试**: `internal/service/*_integration_test.go`
- **构建**: `cmd/admin/main.go` ldflags

## Non-goals

- 不新增功能，仅修正已有变更的偏差
- 如果 Husky/lint-staged/commitlint 决策为"弃用"，则从 tasks.md 移除而非实现
