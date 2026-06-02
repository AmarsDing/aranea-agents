# Gateway + Follow-up Queue 优化（2026-05-21）

## 摘要

P1–P3 网关与对话队列优化：Webhook `enabled` 部分更新、PendingQueue 下沉 runtime、StopGeneration 统一终态路径、Webhook 投递可观测性与查询优化、Follow-up Queue 前端 UX。

## 变更

| 优先级 | 项 | 文件 |
|--------|-----|------|
| P1 | `UpdateWebhookRequest.enabled` → `optional bool`；merge 未传时保留原值 | `gateway.proto`, `webhook.go`, `gateway.go` |
| P2 | `PendingMessageQueue` 迁至 `internal/runtime/pending_queue.go` | 删除 `service/chat_pending.go` |
| P2 | `StopGeneration` → `setRunStatus(cancelled)` | `chat.go` |
| P3 | Webhook `ListEnabled` + FlowLog 投递失败 | `webhook.go`, `webhook_dispatcher.go`, `data/webhook.go` |
| P3 | Gateway Webhook **API-only**（无管理页） | `frontend-pages.md` |
| UX | 运行中连续发送：`inputDisabled` + `message_queued` 刷新 Pending | `useChatSender.ts`, `useChatWorkspace.ts`, `ChatMessagePanel.vue` |
| 清理 | 删除 `ChatService.publishMessageQueued` | `chat.go` |

## 验证

```bash
make api
go test aranea-agents/internal/biz -run "TestMergeWebhook|TestWebhook" -count=1
go test aranea-agents/internal/runtime -run TestPendingMessageQueue -count=1
```
