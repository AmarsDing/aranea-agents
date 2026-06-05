## Context

原 team-graph-optimization 变更归档后审计发现 M5.7 FunctionResolver 集成差最后一步（wireNode 未消费），以及 6.1~6.4 文档同步未执行。

## Goals / Non-Goals

**Goals:**
- 完成 M5.7：wireNode 消费 FunctionResolver
- 完成 6.1~6.4 文档同步

**Non-Goals:**
- DES-03/04/05 端口优化（deferred）
- Q-01~Q-11 代码质量批量修复（deferred）

## Decisions

### D1: wireNode 消费 FunctionResolver

**决策**: 在 `CompiledTeamRepo.CompileTeam` 中，当 wireNode.Type 为 "function" 时，调用 FunctionResolver.Resolve 获取实际函数定义。

## Risks / Trade-offs

- **[Risk] FunctionResolver 可能返回空** → 降级为原始行为，记录 warning 日志
