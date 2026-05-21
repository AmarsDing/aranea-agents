# Monitor Logs — 流程/进程 Tab 拆分

> 日期：2026-05-20  
> 关联：I8-MON-01 · [18 monitor.md](../需求/18%20monitor.md) · [52-flow-logger.design.md](../需求/52-flow-logger.design.md) · [Review](./2026-05-20-Monitor-Logs-Review.md)

## 摘要

Monitor **Logs** 一级 Tab 内拆分为 **流程日志** / **进程日志** 两个二级 Tab，共享单条 `session_id=*` WebSocket（`useLogStreamHub`），各自独立缓冲。进程日志由 **`server.monitor.process_log_enabled`**（默认 true）控制，UI 无手动开关，进程 Tab 切离暂停、切回自动恢复。

## 变更

### 前端

- 新增 `LogStreamPanel.vue`、`FlowLogStream.vue`、`ProcessLogStream.vue`、`useLogStreamHub.ts`
- 连接状态：`connecting` → `connected`（WS `connected` 帧）→ `live`（首条日志）
- 流程 Tab：进入即收 `flow_log`；可暂停/清除
- 进程 Tab：无开启/暂停按钮；`getMonitorLogs().enabled` 驱动；默认 paused，切 Tab 恢复
- 全高布局：`MonitorPage` flex + `.monitor-log-console--fill`

### 后端

- `internal/server/ws.go`：`enable_log(false)` 在 globalMode 下保留 `monitor` channel；config 关时忽略 `enable_log(true)`
- `internal/conf`：`server.monitor.process_log_enabled`（默认 true）
- `internal/service/monitor.go`：`GetMonitorLogs.enabled` 镜像 config
- 删除重复 `EnvelopeTypeLog`：`intent-pass`、`publishTeamMonitor`
- `chat_native` / `session_compress` 迁移至 `flow_log`（TraceEmitter / SysLog）

### 文档

- `18 monitor.md` / `18 monitor.design.md` / `18-monitor-development.md`
- `52-flow-logger.design.md` §6.1 · `52-flow-logger-development.md` Phase 1c
- `execution-plan.md` I8-MON-01 · `frontend-pages.md`
- [2026-05-20-Monitor-Logs-Review.md](./2026-05-20-Monitor-Logs-Review.md)

## 验收

- [x] Logs 两 Tab 独立操作互不影响
- [x] 无对话时流程 Tab 显示「已连接」而非永久「连接中」
- [x] config 关闭进程日志后流程日志仍更新
- [x] `process_log_enabled: false` 时客户端无法 `enable_log` 绕过
- [x] `go build ./...` · `cd web && pnpm build`
