## Why

原 `2026-06-05-team-graph-review-fixes` 变更归档时 87.0% 完成（40/46），6 项 deferred：3.2 测试缺失、6.5/6.6 Knowledge/Plugin 接口抽象、6.7 Wire 绑定、7.6 常量化、7.8 单元测试。

## What Changes

- 补齐 3.2 linked graph + adaptive mode 测试用例
- 实现 6.5 Knowledge 接口抽象
- 实现 6.6 Plugin 接口抽象
- 实现 6.7 Knowledge/Plugin Wire 绑定
- 实现 7.6 magic string 常量化
- 补齐 7.8 CompiledTeamRepo 单元测试

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `team-graph-review-fixes`: 补齐 6 项 deferred 技术债

## Impact

- **biz 层**: Knowledge/Plugin 接口抽象 + Wire 绑定
- **data 层**: magic string 常量化
- **测试**: linked graph 测试 + CompiledTeamRepo 单测

## Non-goals

- 不改变 Team Graph 核心编排逻辑
