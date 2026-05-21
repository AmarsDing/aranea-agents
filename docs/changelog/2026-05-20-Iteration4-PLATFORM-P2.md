# 2026-05-20 — 迭代 4：MCP 重连可观测 / model_router 真路由 / Await 跨重启 / 告警出站

## 摘要

在统一观测面（EventBus / EventBuffer / `monitor_events` / Prometheus / 前端）下完成四项 P2/P3 能力：MCP 会话重连可观测、Plugin `model_router` 经 `ModelSelector` 真改模型、Chat `awaiting_user` 进程重启后 resume（新 turn）、Monitor 告警 Webhook/Channel 出站。

## 变更

### 观测契约

- `internal/event/envelope.go`：`mcp.session.reconnect`、`alert.notify`；`RouteChannel` → `monitor`
- `internal/metrics/vars.go`：`aranea_mcp_session_reconnect_total`、`aranea_alert_notify_total`、`aranea_model_router_fallback_total`

### MCP 重连（I4-MCP-01）

- `pkg/trpc-agent-go/tool/mcp`：`ReconnectObserver` + `WithReconnectObserver`；`executeWithSessionReconnect` 成功/失败/耗尽回调
- `internal/tools/mcpobserve`：Prometheus + EventBus；`EffectiveSessionReconnectMax`（`sse`/`streamable_http` 默认 3）
- `internal/tools/toolset.go`：接线 observer + 默认重连次数
- `cmd/admin/wire.go`：`mcpobserve.SetBus(eventBus)`
- 前端 `McpServerItem.vue` / `types.ts`：`last_reconnect_at`、24h「近期重连」chip

### model_router（I4-PLG-02）

- `internal/plugin/trpc/model_router.go`：导出 `ResolveModelAPI` / `ModelRouterConfig`
- `internal/agent/model_selector.go` + `trpc_build.go`：`WithModelSelector`；catalog 失败回退 base + fallback 指标
- `internal/plugin/trpc/runtime.go`：`ModelRouterConfigForAgent`

### Chat await 跨重启 MVP（I4-CHAT-02）

- `internal/service/chat.go`：`AwaitUserReply` 无 `awaitChans` 时若 `state_json` 为 `awaiting_user` → `resumeAwaitAfterRestart`（新 turn）
- `internal/service/chat_await_resume.go`：`await_resumed` 元数据 + `run_status` WS
- `internal/service/run_status_store.go`：`runtime.await_run_id` / `runtime.await_since`
- 前端 `useChatWorkspace.ts`：resume 失败时提示重启后重发

**非目标**：不恢复 mid-turn goroutine；EventBuffer 仍进程内。

### Monitor 告警出站（I4-MON-02）

- Proto/SQL：`notify_webhook_url`、`notify_channel_id`、`cooldown_minutes`
- `internal/biz/monitor.go`：`AlertNotifier` + 冷却 `sync.Map`
- `internal/service/monitor_notify.go`：Webhook POST + Channel `webhook_url` 凭据；`alert.notify` 事件
- `cmd/admin/wire.go`：`provideMonitorUsecase` / `provideMonitorAlertNotifier`
- 前端 `MonitorAlertRules.vue`：出站字段

### 加固（P0–P2，同日）

- **Await resume**：`clearAwaitingRunStateSync` + `resumeInFlight` 去重；`canResumeAwait` 含 `PendingAwaitUserReplyRoute`
- **告警**：`shouldFireAlert` 同时约束 `alert.fired` 与 notify；`alert.notify` 分 `webhook_status` / `channel_status`
- **model_router**：`EventMu` 锁内仅拷贝 prompt；启用插件时跳过 runtime `ModelSelector=auto` 覆盖
- **MCP**：`RecordReconnectMetadata` 写 `last_reconnect_at` / `reconnect_count`（`mcpobserve.SetMetadataRecorder`）

## 验证

```bash
make api && make wire && make build && make test && make runtime-boundary
cd web && pnpm lint && pnpm test && pnpm build
go test ./internal/service/... -run Await -count=1
```

手工：断 SSE 后工具调用 → Monitor Events `mcp.session.reconnect`；启用 `model_router` + code 启发式 → Usage model 切换；`awaiting_user` 后杀进程 → `AwaitUserReply` 触发新 turn；告警规则 + webhook URL → POST 收到 JSON。
