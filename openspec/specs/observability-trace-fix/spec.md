# observability-trace-fix Specification

## Purpose
TBD - created by archiving change spirit-orchestration-review-fixes. Update Purpose after archive.
## Requirements
### Requirement: ChatOrchestrator turn 入口生成 spirit_trace_id
- ChatOrchestrator SHALL 在 turn 入口处确保 spirit_trace_id 存在，缺失时生成。
- 在 ChatOrchestrator.RunNativeAgentTurnWithOutcome 方法中，检查 context 是否已有 spirit_trace_id
- 如果没有，调用 `biz.NewSpiritTraceID()` 生成并通过 `biz.ContextWithSpiritTraceID(ctx, traceID)` 注入
- 仅在缺失时生成，不覆盖已有的 trace ID（如 TaskPlanner 已设置的）

#### Scenario: 所有 Spirit 编排路径都有 trace ID
- Given ChatOrchestrator 接收到一个 turn 请求
- When context 中没有 spirit_trace_id
- Then 在 turn 入口处生成新的 spirit_trace_id 并注入 context
- When context 中已有 spirit_trace_id（如 TaskPlanner 已设置）
- Then 保留已有的 trace ID，不覆盖

