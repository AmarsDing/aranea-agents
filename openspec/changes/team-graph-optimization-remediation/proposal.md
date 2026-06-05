## Why

原 `2026-06-05-team-graph-optimization` 变更归档时 83.3% 完成（35/42），7 项待办：M5.7 FunctionResolver 集成最后一步未完成、6.1~6.4 文档同步未执行。

## What Changes

- 完成 M5.7：wireNode 消费 FunctionResolver 接口
- 完成 6.1~6.4：文档同步（team-graph 问题与方案 / architecture-blueprint / module-cross-reference）+ 复审

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `team-graph-optimization`: 补齐 M5.7 + 文档同步

## Impact

- **biz 层**: `internal/biz/team_graph.go` wireNode 消费 FunctionResolver
- **文档**: `docs/team-graph问题与方案.md` / `openspec/specs/architecture-blueprint.md` / `openspec/specs/module-cross-reference-full.md`

## Non-goals

- 不实现 DES-03/04/05 端口优化（已 defer）
- 不实现 Q-01~Q-11 代码质量批量修复（已 defer）
