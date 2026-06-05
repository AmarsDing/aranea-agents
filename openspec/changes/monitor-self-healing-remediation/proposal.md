## Why

原 `2026-06-05-monitor-self-healing` 变更归档时 81.2% 完成（52/64），审计发现 12 项真实缺口：4 项单元测试未编写、4 项框架层集成未实现、1 项 cron 缺失、circuit breaker 时间窗口设计偏差。

## What Changes

- 补齐 4 项单元测试（Phase 1.1/1.3/1.5/2.3）
- 补齐 TTL cleanup cron job（Phase 2.2）
- 修正 circuit breaker 为"10 分钟内 5 次"（非"连续 5 次"）+ 添加 30 分钟重置 + `heal_circuit_open` 事件
- 框架层集成（Phase 1.2 LLM call auto-heal / Phase 1.4 AutoHealStrategy）标记为 defer，需 trpc-agent-go 框架支持

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `monitor-self-healing`: 补齐测试、cron、circuit breaker 修正

## Impact

- **data 层**: `internal/data/monitor.go` circuit breaker 逻辑修正
- **cron 层**: 新增 TTL cleanup job
- **biz 层**: 单元测试覆盖

## Non-goals

- 不实现 Phase 1.2/1.4（需 trpc-agent-go 框架层支持，独立迭代）
