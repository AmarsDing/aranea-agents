# Channel 入站去重与安全门禁

**日期**：2026-05-22

## 问题

飞书入站会在 Chat 中产生多条相同用户消息（如两次「你好」），即使用户未在 Web 端发送。根因包括：

- 入站仅对**出站**做 idempotency，Turn 可被重复触发
- Webhook 与 WebSocket 可能双通道同时处理
- 机器人自身消息未过滤，可能形成回声
- 飞书平台重试或重复 `message_id` 无持久化去重

## 措施

| 措施 | 实现 |
|------|------|
| 入站幂等表 | `channel_inbound_receipt` UNIQUE(channel_id, idempotency_key) |
| 进程内 inflight 锁 | 同一 dedup key 并发只处理一次 |
| ~~短窗重复文本~~ | **已移除**（见 Root-Cause changelog；会误伤合法重复提问） |
| 飞书 sender_type | 仅处理 `sender_type=user` |
| Webhook/WS 互斥 | `receive_mode=websocket` 时忽略 Webhook 入站 |
| 审计 | `channel_delivery` 状态 `skipped_duplicate_*` |

## 代码锚点

- `internal/biz/channel_inbound_receipt.go`
- `internal/data/channel_inbound_receipt.go`
- `internal/service/channel_ingress_guard.go`
- `internal/channel/lark/parse_message.go` (`isFeishuUserSender`)
