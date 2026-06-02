# Channel Phase 3（2026-05-21）

## 摘要

Slack/Telegram Webhook 适配、异步出站投递 worker、前端 Webhook URL 复制与外部 ID 列。

## 变更

### Slack / Telegram

- `internal/channel/slack/` — 签名验证、事件解析、`chat.postMessage`、`auth.test`
- `internal/channel/telegram/` — Update 解析、可选 secret token、`sendMessage`、`getMe`
- `internal/service/channel_ingress_slack.go` / `channel_ingress_telegram.go`
- `TestChannel` 增加 Slack/Telegram live 探测

### 异步投递

- `biz/channel_delivery.go` — `EnqueueOutboundDelivery`、幂等键、`MarkOutboundAttempt`（最多 3 次重试）
- `data/channel.go` — `ListPendingDeliveries` / `UpdateDelivery`
- 全部 ingress 出站改为入队；`service/channel_delivery_worker.go` 统一发送
- `cronrunner/jobs/channel_delivery.go`（5s，`CHANNEL_DELIVERY_DISABLED=1` 可关）

### 前端

- `channelUi.ts` — `channelWebhookURL`、`copyChannelWebhookURL`、`channelExternalID`
- `ChannelsTable.vue` — 外部 ID 列、复制 Webhook 按钮
- `ChannelEditorDialog.vue` — 只读 Webhook URL + 复制

## 验证

```bash
go test ./internal/channel/... ./internal/biz/... ./internal/service/... -count=1
make wire && make build
cd web && pnpm lint && pnpm build
```
