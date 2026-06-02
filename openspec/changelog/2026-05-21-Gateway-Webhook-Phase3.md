# Gateway Phase 3 Webhook + chat_native 入队文案（2026-05-21）

## 摘要

- 出站 Webhook 系统落地：CRUD API、Ent 持久化、HMAC-SHA256 签名、运行终态异步回调。
- `chat_native` / `ChatUsecase` 入队拒绝区分「运行已结束」与「队列已满」。

## Webhook

| 层 | 文件 |
|----|------|
| Proto | `api/kratos/gateway/v1/gateway.proto` |
| Ent | `internal/data/ent/schema/gateway_webhook.go` |
| SQL | `docs/sql/19_gateway_webhook.sql` |
| Biz | `internal/biz/webhook.go`, `webhook_dispatcher.go` |
| Data | `internal/data/webhook.go` |
| Service | `internal/service/gateway.go`, `chat_enqueue.go`, `callbackConsumer` |

**API**：`POST/GET/PUT/DELETE /v1/gateway/webhooks`

**触发点**：`setRunStatus` 终态（completed/failed/cancelled）、`StopGeneration` 取消。

**签名**：`X-Webhook-Signature: HMAC-SHA256(body, secret)` 十六进制。

## chat_native 修复

| 拒绝原因 | 错误码 | 文案 |
|----------|--------|------|
| `no_active_run` | `CHAT_RUN_ENDED` (409) | agent run has ended; send your message again to start a new turn |
| `queue_full` | `CHAT_QUEUE_FULL` (400) | pending queue is full for this session |

## 验证

```bash
make api && make wire && go build ./cmd/admin/...
go test ./internal/biz/... -run Webhook -count=1
go test ./internal/service/ -run "Pending|StopGeneration" -count=1
```
