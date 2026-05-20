# Flow Log v2：移除 SlogBridge，全项目 slog 迁移

> **日期**：2026-05-20  
> **关联**：[Agent 无响应与 FlowLogger 初版](./2026-05-20-Agent-No-Response-Debug-And-FlowLogger.md) · [52-flow-logger 开发计划](../需求/52-flow-logger-development.md) · [步骤注册表](../guides/flow-log-step-registry.md)

---

## 1. 背景

v1 通过 `SlogBridge` 将 Go `slog` 桥接到 `EnvelopeTypeLog`，在 Turn/插件热路径与 EventBus 订阅者之间形成**同步重入死锁**（见初版 changelog §2.1）。v2 以 `TraceEmitter` + `EnvelopeTypeFlowLog` 为 Monitor 业务日志主输出；本次完成 **删除 SlogBridge** 并将 `internal/` 内全部 `slog` 调用迁移为 Flow Log v2 API。

---

## 2. 代码变更摘要

| 变更 | 说明 |
|------|------|
| **删除** | `internal/event/slog_bridge.go`（`SlogBridge`、`InstallSlogBridge`、`traceHandler`） |
| **新增** | `internal/event/system_flow.go` — `SetGlobalBus`、`SysLog*`、`SessionSysLog*` |
| **扩展** | `flow_context.go` — `CtxFlowLogWarn`；`flow_log.go` — 系统域 `step_id` 中文 title |
| **启动** | `cmd/admin/main.go` — `event.SetGlobalBus(eventBus)` 替代 `InstallSlogBridge` |
| **迁移** | `internal/` 全量：`TraceEmitter` / `CtxFlowLog*` / `SysLog*` / `SessionSysLog*` |
| **pkg/** | `safego`、`auth` — 写 stderr `[flow][system]`（不 import `internal/event`） |
| **插件** | 热路径继续 `PluginSafeLogger`（stderr + 异步 `EnvelopeTypeLog`） |

---

## 3. 打点约定（开发者）

| 场景 | API | `domain` / 出口 |
|------|-----|-----------------|
| Chat Turn | `NewTraceEmitterForRun` + `emitter.Log*` | `chat` → WS `flow_log` |
| 上下文内子步骤 | `CtxFlowLogWarn/Done/Error` | 同上或 `system`（无 emitter 时） |
| 基础设施（Bus/WS/Cron/遥测） | `SysLogWarn/Error/Info` | `system` → WS `flow_log` |
| 带 session 的系统事件 | `SessionSysLogWarn/Info` | 可 Monitor 按 session 过滤 |
| 插件回调 | `PluginSafeLogger` | stderr + `log`（非 slog） |
| 本地调试 | `FLOW_LOG_STDERR=1` | stderr `[flow]` 同步行 |

**禁止**：Turn/插件热路径新增 `slog`；在 `bus.Publish` 同步路径内再 Publish flow（Bus drop 已改为异步 `SessionSysLogWarn`）。

---

## 4. 环境变量

| 变量 | 变更 |
|------|------|
| `LOG_BRIDGE_ENABLED` | **已废弃**（无 SlogBridge） |
| `LOG_BRIDGE_LEVEL` | **已废弃** |
| `FLOW_LOG_STDERR` | 保留：无 Bus 或显式调试时 stderr 输出 |

进程 Gateway 文本日志（`enable_log` / `EnvelopeTypeLog`）与 **`flow_log` 分流**：Monitor Logs Tab 以 `flow_log` 为主；进程 log 仍走 `PluginSafeLogger` 等显式路径。

---

## 5. 文档同步

已更新：`52-flow-logger*.md`、`34-event-development.md`、`24-telemetry-development.md`、`flow-log-step-registry.md`、`execution-plan.md`、消息/事件设计文档中对 SlogBridge 的引用。

---

## 6. Phase 1b（2026-05-20 续）

Monitor **Traces 详情** 已增加「流程」Tab、`FlowLogExportButton` JSONL 导出、按 `trace_id` 过滤的 WS 收录。前端：`FlowTracePanel.vue`、`flow.spec.ts`。

---

## 7. 验证

```bash
go build ./...
go test ./internal/event/... -count=1
```

**手动**：发 Chat → Monitor Logs 见中文 `flow_log` 行；失败 Turn 见 `error`/`critical` severity；不再依赖 `LOG_BRIDGE_ENABLED=1`。
