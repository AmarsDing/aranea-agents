# Message P2/P3 + FlowLogger Phase 2（2026-05-21）

## 摘要

落地消息全文检索、EventBus 按类型独立订阅消费者、FlowLogger HTTP 历史查询与落库。

## Message P2 — FTS5 搜索

| 项 | 实现 |
|----|------|
| SQL | `docs/sql/16_message_fts.sql` + `internal/data/sql/message_fts.sql`（FTS5 + 触发器） |
| Repo | `internal/data/message_search.go`（FTS5 + LIKE 回退） |
| API | `GET /v1/sessions/messages/search` · `SearchSessionMessages` |
| 前端 | `web/src/features/session/api.ts` · `searchSessionMessages` |

## Message P3 — 独立 Bus 消费者

| 消费者 | 订阅 | 职责 |
|--------|------|------|
| `toolCallConsumer` | `tool_result` | `ToolUsecase.RecordToolInvocation`（`source=event_bus`，`tinv-{tool_call_id}` 幂等） |
| `callbackConsumer` | `run_status` 终态 | `WebhookDispatcher.Dispatch`（从 `ChatService.setRunStatus` 移除直连） |
| `messageStoreConsumer` | `member_message_done` | Team 成员回复 `AppendChatMessage` |
| `flowLogPersistConsumer` | `flow_log` | `flow_log_events` 落库 |

编排：`biz.EventBusSideConsumers` · `cmd/admin/main.go` BeforeStart。

## FlowLogger Phase 2

| 项 | 实现 |
|----|------|
| SQL | `docs/sql/15_flow_log.sql` |
| Ent | `flow_log_event` schema + `internal/data/flow_log_repo.go` |
| API | `GET /v1/monitor/flow-logs` · `ListFlowLogs` |
| 前端 | `web/src/features/monitor/api.ts` · `listFlowLogs` |

## 验证

```bash
go build ./...
go test ./internal/biz/... ./internal/data/... -count=1
```
