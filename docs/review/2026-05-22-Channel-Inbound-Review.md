# Channel 入站与 Runtime 审查报告

> **日期**：2026-05-22  
> **依据**：[docs/README.md](../README.md) · [AI-DEVELOPMENT-SPECIFICATION](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · [17 channel.md](../需求/17%20channel.md) · [17-channel-development.md](../需求/17-channel-development.md)  
> **关联变更**：[Channel-Inbound-Root-Cause](../changelog/2026-05-22-Channel-Inbound-Root-Cause.md) · [Channel-Inbound-Dedup](../changelog/2026-05-22-Channel-Inbound-Dedup.md)

---

## 1. 终端证据（根因）

```
17:28:27 connected ... [conn_id=7642645622231894977]
17:30:28 connected ... [conn_id=7642646129968942301]
```

- 间隔 **≈2 分钟**，与 `CHANNEL_RUNTIME_RELOAD_INTERVAL` 默认 `2m` 一致。
- **不同 `conn_id`**：同一进程内存在两条飞书长连接，而非单纯 UI 重复渲染。
- 与「7 分钟两次你好」可并存：除 Runtime 双连接外，仍可能有飞书两次独立 `message_id` 入站。

**架构根因（已修）**：`runtimeFingerprint` 曾包含 `ch.UpdatedAt`；`RunHealthChecks` → `updateTestMetadata` → `repo.Update` 会刷新 `UpdatedAt`，导致定期 `Reload` 误判配置变更，**取消旧 WS 并启动新 WS**，旧连接未及时断开时出现双连接、双入站。

---

## 2. 审查结论（按 README 维度）

| 维度 | 评级 | 说明 |
|------|------|------|
| **需求符合度** | ✅ | 入站仅用户文本、群 @、`message_id` 幂等与需求一致；Runtime 双连接属实现缺陷，已修 fingerprint |
| **架构一致性** | ✅ | 适配器 `lark.AcceptFeishuInbound` → `service.ChannelIngress` → `ChatService`，未违反 biz 不 import 框架 |
| **单一职责** | ✅ | `inbound_gate`（规则）/ `inbound_build`（构造）/ `inbound_webhook`（Webhook）/ `ingress_guard`（幂等与模式）分层清晰 |
| **代码质量** | ✅ | 删除 2 分钟文本启发式与分钟桶 fallback；严格 `sender_type` |
| **影响域** | 见 §4 | 仅 Channel 入站 + Runtime 生命周期；不改 Chat/WebSocket 契约 |

**综合**：入站门控修改 **通过**；需配合 Runtime fingerprint 修复才闭合「双连接」根因。

---

## 3. 入站修改审查（lark + service）

### 3.1 做得好的

| 项 | 评价 |
|----|------|
| `AcceptFeishuInbound` 单点门控 | WS/Webhook 规则一致，符合「适配器规范化、service 编排」 |
| 严格 `sender_type=user` | 修复旧逻辑 nil→当用户，避免机器人回声入站 |
| `channel_inbound_receipt` | 仅对同一 `feishu:{message_id}` 防重放，语义正确 |
| `channel.inbound.receive` 审计 | 可对照 `idempotency_key` 排障 |
| Webhook 在 `receive_mode=websocket` 时 200 忽略 | 避免 HTTP 与 WS 双处理（配置正确时） |

### 3.2 已移除的欠妥设计

| 项 | 问题 |
|----|------|
| 2 分钟同 Session 同文本 | 误伤合法重复提问；挡不住 7 分钟间隔 |
| `InboundIdempotencyKey` 分钟桶 fallback | 无 `message_id` 时猜测去重，可能误合并或漏防 |

### 3.3 小建议（P3，非阻塞）

- `logInboundAccepted` 的 `viaWebhook` 参数可改为 `ingressSource string`，避免与 meta 字段语义重叠。
- Flow Log 步骤表可登记 `channel.inbound.receive` / `channel.runtime.connector_start`（见 52-flow-logger.design.md 扩展）。

---

## 4. 影响域矩阵

| 层级 | 文件/行为 | 影响 |
|------|-----------|------|
| `internal/channel/lark/*` | 门控、Webhook 解析 | 飞书入站过滤更严；非 user 消息不再 Turn |
| `internal/service/channel_ingress*` | ProcessInbound 门禁 | 无 id/key 拒绝；审计字段增加 |
| `internal/biz/channel_inbound_receipt` | 幂等键 | 仅 platform message id |
| `internal/data/channel_inbound_receipt` | 表 | 需 admin 重启迁移 schema |
| `internal/channel/runtime/manager` | fingerprint + 关停等待 | **防止双 WS**；健康检查不再触发重连 |
| Web Chat / `RunNativeTurn*` | 无 API 变更 | 仅减少重复用户消息 |
| 其他 IM 平台 | 无 | 未改 |

---

## 5. Runtime 修复（2026-05-22 审查同步）

| 变更 | 目的 |
|------|------|
| `runtimeFingerprint` 去掉 `UpdatedAt` | 健康检查/metadata 更新不重启连接器 |
| `runningInstance.done` + `waitRuntimeInstanceDone` | Reload 替换前等待旧 goroutine 退出，降低双连接窗口 |
| `channel.runtime.connector_start` | 日志对齐 conn_id 排障 |

**运维**：若仍见双 `connected`，检查是否多 admin 进程或飞书侧多应用凭证；单进程应仅一条 `connector_start` / 渠道 / 2 分钟内。

---

## 6. 验证清单

- [ ] 重启 admin 后仅一条飞书 `connected`（2 分钟内无第二条不同 `conn_id`）
- [ ] 飞书发一条消息 → 一条 `channel.inbound.receive` + 一条用户消息
- [ ] Flow Log / delivery 中 `skipped_non_user_sender` 等对机器人消息生效
- [ ] `go test ./internal/channel/... ./internal/service/...`

---

## 7. 文档同步

| 文档 | 更新 |
|------|------|
| [17-channel-development.md](../需求/17-channel-development.md) | §入站门控 + Runtime fingerprint |
| [17-channel-agent-team-integration.md](../需求/17-channel-agent-team-integration.md) | §3.2 入站步骤 |
| [17-channel-review.md](./17-channel-review.md) | 本审查摘要 + CH-P1-DUAL-WS |
| [README.md](../README.md) | 索引入站根因 changelog |
| [2026-05-22-Channel-Inbound-Root-Cause.md](../changelog/2026-05-22-Channel-Inbound-Root-Cause.md) | 补充 Runtime 双连接根因 |
