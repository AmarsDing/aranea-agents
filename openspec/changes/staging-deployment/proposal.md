## Why

当前项目无 staging 环境，release.yml 仅包含 GoReleaser + Docker push，缺少 staging 部署、冒烟测试和 production promote 步骤。需要基础设施先行。

## What Changes

- 新增 staging 部署步骤到 release.yml（需 K8s 集群）
- 新增 staging 冒烟测试步骤
- 新增 production promote 步骤

## Capabilities

### New Capabilities

- `staging-deployment`: Staging 环境部署 + 冒烟测试 + Production promote

### Modified Capabilities

（无）

## Impact

- **CI/CD**: `.github/workflows/release.yml` 需要新增 staging 相关 job
- **基础设施**: 需要 K8s 集群和 staging 命名空间
