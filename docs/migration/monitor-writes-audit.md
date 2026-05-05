# Legacy `pkg/backend` 监控写入审计（Phase 7）

**结论（2026-05）：** 在 **`pkg/backend/internal/transport/monitor.go`** 与 **`handler.go` 挂载的 `/api/v1/monitor/*`** 路径上，**未发现 HTTP POST/PATCH 类写入**。公开端点均为 **GET**，或 SSE 占位（`/monitor/logs`、`/monitor/logs/stream` 返回占位/心跳，不向 `monitor_*` 表写入）。

## 数据来源

| 端点族 | 方法 | 数据源 |
|--------|------|--------|
| `/api/v1/monitor/audit` | GET | `auditSvc.List`（审计日志表，非本节「monitor_events」写入） |
| `/api/v1/monitor/events` / `events/{id}` | GET | `platformSvc.List/Get("monitor-events")` |
| `/api/v1/monitor/traces` / `traces/{id}` | GET | `platformSvc.List/Get("monitor-traces")` |
| `/api/v1/monitor/logs`、`logs/stream` | GET / SSE | 占位响应，无可配置上游时不落库 |

结构化 **monitor_events / monitor_traces** 行的**业务写入链**在历史上可能经 **capabilities / adapters** 或非 HTTP 管线完成；若以 **`cmd/admin` `monitor/v1`** 为读面单一来源，应保持 **仅此进程或明确作业**负责平台资源镜像，并与 [runbook-operational-baseline.md](runbook-operational-baseline.md) 双进程约束一致。

## 收口建议

- 读路径：继续 **`api/kratos/monitor/v1`** + Web `features/monitor`。
- 如需正式「写入」语义：新增 **仅在 proto 中印出**的 RPC（如 ingest events），并实现于 admin **单连接 Ent/SQLite**，禁止再依赖遗留 handler 的手工路由。
