## Context

原 team-graph-review-fixes 变更归档后审计发现 6 项 deferred 技术债。

## Goals / Non-Goals

**Goals:**
- 补齐 6 项 deferred 技术债

**Non-Goals:**
- 不改变 Team Graph 核心编排逻辑

## Decisions

### D1: Knowledge/Plugin 接口抽象

**决策**: 定义 `KnowledgeProvider` 和 `PluginProvider` 接口，现有实现改为默认实现，通过 Wire 注入。

### D2: magic string 常量化

**决策**: 提取到 `internal/biz/team_graph_constants.go`，集中管理。

## Risks / Trade-offs

- **[Risk] 接口抽象可能过度设计** → 当前仅有 1 个实现，但为未来扩展预留
- **[Risk] 常量化可能引入拼写错误** → 全局搜索替换 + 编译验证
