## Context

原 self-iteration-engine 变更归档后审计发现 17 项假完成 + 18 项验证未执行。本变更不新增功能，仅修正偏差。

**当前状态**:
- `.husky/` 目录存在但 hook 内容为 `# disabled`
- `.commitlintrc.yml` 不存在
- 根目录无 `package.json`（Husky/lint-staged 依赖未安装）
- `release.yml` 中无 staging 步骤
- 集成测试文件存在但仅有骨架

## Goals / Non-Goals

**Goals:**
- 对 17 项假完成做出明确决策：实现或 defer
- 对 18 项验证步骤提供执行证据或 defer
- 修正 CI 配置名错误
- 补齐集成测试断言

**Non-Goals:**
- 不新增功能
- 不改变已有代码的架构

## Decisions

### D1: Husky/lint-staged/commitlint 处理方式

**决策**: 正式弃用。项目为 Go 后端 + Vue 前端，Go 侧有 `make lint`，前端有 `pnpm lint`，CI 已覆盖。Husky 需要 Node.js 运行时，在纯 Go 开发环境下增加复杂度。

**替代方案**: 保留 Husky → 需要 package.json + Node.js → 与 Go 项目定位不符

### D2: Staging 部署步骤

**决策**: defer 到独立变更。当前项目无 staging 环境，需要基础设施先行。

### D3: 集成测试骨架

**决策**: 补齐实际断言。当前骨架仅启动容器，无 API 调用验证。

## Risks / Trade-offs

- **[Risk] 弃用 Husky 可能降低 commit 质量** → CI lint 已覆盖，且 Go 社区更倾向 pre-push hook
- **[Risk] Staging defer 可能延迟生产验证** → 当前已有 CI 全量验证，风险可控
