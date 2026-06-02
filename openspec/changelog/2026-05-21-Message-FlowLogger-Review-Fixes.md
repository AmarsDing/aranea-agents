# Message / FlowLogger Review 修复（2026-05-21）

## P0

- **ToolInvocation upsert**：冲突时 Update 终态字段并保留 `created_at`；运行时 `source=trpc`（trpc-agent-go hook），EventBus 为 `event_bus`。
- **Team 成员消息**：`messageStoreConsumer` 使用 `role=member`、确定性 ID、`bumpModelCall=false`；`AppendChatMessage` 主键冲突静默成功（重放幂等）。

## P1

- **Bus 背压**：侧效消费者经 `asyncEnvelopeWorker` 有界队列异步落库（`EVENT_BUS_SIDE_QUEUE`）。
- **搜索隔离**：`SearchMessages` 强制 `session_id`（biz + data）。
- **Wire 环**：`ProvideChatService` + `ProvideEvaluationRunner` 打破 `ChatService ↔ EvaluationRunner` 循环；`make wire` 可再生成。

## P2

- 删除 `ChatService.dispatchRunWebhook`；Webhook 仅 `callbackConsumer`。
- `ChatService` 不再注入 `WebhookDispatcher`（仍由 `callbackConsumer` 使用）。
- 文档同步：`51` / `51a` §10.2、`35 gateway.design.md`。

## P3

- **Flow log TTL**：`FlowLogUsecase.PurgeExpired` + `jobs.FlowLogCleanup`（`FLOW_LOG_TTL_DAYS` 默认 30，`FLOW_LOG_CLEANUP_DISABLED`）。
- **ListFlowLogs**：proto `since` / `until`（RFC3339）+ MonitorService 解析。

## 验证

```bash
make api && make wire && go build ./...
go test ./internal/biz/... ./internal/data/... ./internal/service/... -count=1
```
