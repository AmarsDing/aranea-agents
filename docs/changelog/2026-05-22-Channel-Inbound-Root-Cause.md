# Channel 飞书入站根因修复

**日期**：2026-05-22

## 问题现象

Web Chat 出现多条相同用户消息（如两次「你好」），用户未在 Web 端发送。间隔可达数分钟。

## 根因（非 Web 自动发送）

0. **Runtime 定期 Reload 导致双 WebSocket（终端可验证）**  
   `runtimeFingerprint` 曾包含 `UpdatedAt`；健康检查 `updateTestMetadata` 会 `Update` 渠道行并刷新 `UpdatedAt`。每 **2 分钟** `Manager.Reload` 误判配置变更 → 新 `larkws` 连接（新 `conn_id`）而旧连接未及时关闭 → **同一飞书消息可被处理两次**。  
   **修复**：fingerprint 仅含 `config_json` / `enabled` / `receive_mode` / 凭据 revision；替换连接器前 `waitRuntimeInstanceDone`。

1. **Webhook 与 WebSocket 入站规则不一致**  
   WS 路径有 `sender_type=user`、群聊 @ 门控；Webhook 路径此前未校验 `sender_type`，机器人/应用消息或群聊未 @ 也可能入站。

2. **`sender_type` 缺失时默认当作用户**  
   `isFeishuUserSender` 在 `SenderType == nil` 时返回 `true`，易把机器人回声或字段不全的事件当成用户消息。

3. **两条不同 `message_id` 的合法入站**  
   飞书若投递两次独立事件（用户飞书端连发、平台重试、或多 admin 进程各跑一条 WS），会在 Session 中产生两条用户消息。  
   **2 分钟同文本去重**无法覆盖分钟级间隔，且会误伤用户重复提问。

4. **幂等 fallback（按分钟 + peer + text）**  
   无 `message_id` 时用时间桶哈希，既不能可靠防重，又可能在同分钟误合并不同消息。

## 修复（源头）

| 项 | 做法 |
|----|------|
| 统一门控 | `lark.AcceptFeishuInbound` + `BuildFeishuInboundEvent`，WS / Webhook 共用 |
| 严格发送方 | 仅 `sender_type=user`；缺失或 `app` 等一律拒绝 |
| Webhook 对齐 | 解析 `sender_type`、`chat_type`、`mentions`、`message_type` |
| 移除启发式 | 删除 2 分钟同 Session 同文本跳过；删除分钟桶 idempotency fallback |
| 保留平台幂等 | 有 `feishu:{message_id}` 时 `channel_inbound_receipt` 防同一事件重放 |
| 可观测 | `channel.inbound.receive` 记录 `ingress_source`、`idempotency_key` |

## 运维注意

- 同一飞书应用 **只应有一个 admin 进程** 跑 `receive_mode=websocket` 长连接；多实例会各收一份事件（`message_id` 相同时 receipt 可挡，竞态下仍可能双 Turn）。
- `receive_mode=websocket` 时飞书控制台 Webhook 仍会 POST，服务端已 200 忽略，不再入站。
- 若仍见重复，查 Flow Log `channel.inbound.receive` 的 `idempotency_key`：相同则为重放；不同则为飞书两次独立消息。

## 代码锚点

- `internal/channel/lark/inbound_gate.go`
- `internal/channel/lark/inbound_build.go`
- `internal/channel/lark/inbound_webhook.go`
- `internal/service/channel_ingress_guard.go`
- `internal/channel/runtime/manager.go`（fingerprint、`done` 关停等待、`channel.runtime.connector_start`）
