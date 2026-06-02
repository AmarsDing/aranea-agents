# Event Store P2 — 持久化 + 回放 API + TTL（2026-05-21）

## 摘要

实现 `event_store` SQLite 持久化、独立 `event_persist_handler` 异步写入、`GET /v1/events` 回放 API、每小时 TTL 清理。修复 Chat Pipeline 与 WS/Consumer 共用同一 `event.Buffer` 单例。

## 架构

```
Bus.Publish → EventBusConsumer
                 ├─ eventBufferHandler   (内存重放)
                 ├─ eventPersistHandler  (异步 SQLite，排除 log/flow_log)
                 ├─ runnerCompletionHandler
                 └─ stateDeltaHandler

GET /v1/events → EventService → EventStoreUsecase → EventStoreRepo (Ent)
EventStoreCleanup (1h) → PurgeExpired → EVENT_STORE_TTL_DAYS (default 7)
```

## 新增文件

- `api/kratos/event/v1/event.proto`
- `internal/data/ent/schema/event_store.go`
- `docs/sql/18_event_store.sql`
- `internal/biz/event_store.go`
- `internal/biz/event_persist_handler.go`
- `internal/data/event_store_repo.go`
- `internal/service/event.go`
- `internal/cronrunner/jobs/event_store_cleanup.go`

## 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `EVENT_STORE_TTL_DAYS` | 7 | 过期删除天数 |
| `EVENT_STORE_CLEANUP_DISABLED` | — | `1` 禁用 TTL 任务 |

## API

`GET /v1/events?session_id=xxx&since=...&until=...&type=tool_call&limit=100&offset=0`
