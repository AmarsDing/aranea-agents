# 2026-05-23 Hermes Kanban HK-INT-01 Webhook 实接

**影响**：🟢 低 | **模块**：Biz / Service (M54 / G13)

## 变更

- 新增 `WebhookEventGraphTaskStatus = "graph.task.status"`（`internal/biz/webhook.go`）。
- `GraphTaskRuntime.PublishTaskStatus` 在 WS 投影后调用 `WebhookDispatcher.Dispatch`，与 `TaskStatusPublisher` 单路径对齐。
- `WireGraphTaskRuntime` 注入 `*WebhookDispatcher`；Wire 已重新生成。

## 载荷

出站 JSON 沿用 `WebhookPayload`：`event_type`、`run_id`（task_id）、`session_id`、`status`（任务状态）、`data`（graph_id、execution_id、node_id、assignee、summary 及 extra）。

## 配置

Webhook 需在 Gateway 配置中将 `graph.task.status` 加入 `event_types_json` 才会投递（默认仍仅 run.* 三类）。

## 回归风险

| 区域 | 风险 | 缓解 |
|------|------|------|
| 无订阅 webhook | 无 HTTP 副作用 | `Dispatch` 早退 |
| 高频状态变更 | 出站量增加 | 异步 safego + 与 WS 同频（已有行为） |

## 验收

```bash
go test ./internal/biz/ -run TestWebhookSubscribes -count=1
go build ./cmd/admin/...
```
