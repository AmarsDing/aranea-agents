# Channel 飞书 WebSocket 入站修复 + FlowLog 合规

> **日期**：2026-05-22  
> **模块**：Channel / 飞书 larkws  
> **规范**：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · [52-flow-logger.design.md](../需求/52-flow-logger.design.md)

## 背景

飞书 Channel「保存并测试」通过但用户发消息无响应。根因：

1. 出站 `receive_id` 误用 `open_id`，群聊/会话内回复不可见（MuseBot 使用 `chat_id`）。
2. larkws `onMessage` 同步跑完整 Agent Turn，事件 ctx 取消后链路中断。
3. 新增代码误用 `log/slog` 与裸 `go func()`，违反项目 FlowLog v2 与 `safego` 红线。

## 代码变更

| 文件 | 变更 |
|------|------|
| `internal/channel/lark/config.go` | 新增 `AppAndRegionFromConfig` / `WSAppCredentials`（配置解析 SRP） |
| `internal/channel/lark/parse_message.go` | `InboundEventFromWSMessage`、@ 剥离、群聊 @ 门控 |
| `internal/channel/lark/ws_inbound.go` | `HandleWSInbound`、失败时用户可见错误提示 |
| `internal/channel/lark/ws.go` | 仅连接注册；`safego.Go` 异步入站 |
| `internal/channel/lark/receive_id.go` | `ResolveReceiveTarget` 优先 `chat_id` |
| `internal/channel/runtime/manager.go` | `slog` → `event.SysLogWarn` |
| `internal/service/channel_feishu_config.go` | 委托 `lark.AppAndRegionFromConfig` |
| `internal/service/channel_ingress.go` | Webhook 出站 meta 对齐 `chat_id` |
| `web/src/features/channels/*` | 飞书默认 `websocket`；ROUTING Agent/Team 下拉 |

## FlowLog 步骤（system 域）

| step_id | 说明 |
|---------|------|
| `channel.feishu.ws.panic` | WebSocket 入站 goroutine panic |
| `channel.feishu.ws.inbound_fail` | `ProcessInbound` 失败 |
| `channel.runtime.credentials_fail` | Runtime Reload 读凭据失败 |

## 验证

```bash
go test ./internal/channel/... ./internal/service/... -count=1
```

## 运维提示

- 群聊须 **@ 机器人**；私聊直接发文字。
- 首条回复可能有 ≤5s delivery worker 延迟。
- 禁用 Runtime：`CHANNEL_RUNTIME_DISABLED=1`；禁用出站 worker：`CHANNEL_DELIVERY_DISABLED=1`。
