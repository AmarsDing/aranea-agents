# 17 Channel Review

> **评分**：92 / 100 | **风险等级**：P2（生产 soak / 全链路 E2E）  
> **文档**：[17-channel-development.md](../需求/17-channel-development.md) · [17 channel.design.md](../需求/17%20channel.design.md)  
> **代码锚点**：`internal/channel/` · `internal/channel/runtime/` · `internal/service/channel*.go` · `web/src/features/channels/`  
> **审查时间**：2026-05-22（Review 优化项闭合后复审）

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 19 | 20 | 10 平台 catalog、9 平台入站/出站、QQ botgo、流式三平台、Prometheus、Runtime 重连 + 定期 Reload ✅ |
| 架构一致性 | 23 | 25 | Webhook + Runtime 统一 `ProcessInbound`；`platformAdapters` 合并 outbound/stream；Runtime fingerprint 含凭据 revision ✅ |
| 后端实现质量 | 19 | 20 | 流式 fail-fast、Feishu unary `receive_id_type`、`prepareChannelChatRequest` 去重 |
| 前端实现质量 | 14 | 15 | MuseBot 分区 + `useChannelEditorForm` + catalog 凭据 ✅ |
| 测试与验证 | 8 | 10 | stream preview / send_message / streaming fallback 单测；全链路 E2E 仍缺 |
| 文档一致性 | 9 | 10 | development + review 已同步；`17 channel.design.md` 流式章节可续补 |

---

## 已验收平台（2026-05-22）

| 平台 | Webhook 入站 | Runtime 长连接 | 出站 | 流式 edit | bundled |
|------|-------------|----------------|------|-----------|---------|
| 飞书 / feishu | ✅ | ✅ larkws | ✅ | ✅ patch | ✅ |
| 钉钉 / dingtalk | ✅ | ✅ stream | ✅ | unary 回退 | ✅ |
| 企微 / wecom | ✅ | — | ✅ | unary 回退 | ✅ |
| Slack | ✅ | ✅ socketmode | ✅ | ✅ update | ✅ |
| Telegram | ✅ | ✅ polling | ✅ | ✅ edit | ✅ |
| Discord | — | ✅ gateway | ✅ | unary 回退 | ✅ |
| 微信公众号 / wechat | ✅ | — | ✅ | unary 回退 | ✅ |
| OneBot / personal_qq | ✅ | — | ✅ | unary 回退 | ✅ |
| QQ 官方 / qq | ✅ | — | ✅ | unary 回退 | ✅ |

---

## Review 优化闭合项（2026-05-22）

| ID | 问题 | 状态 |
|----|------|------|
| CH-P1-UTF8 | `trpc_turn.go` 日志 UTF-8 损坏 | ✅ 自 3ab60a0 恢复 + 保留 `OnReplyDelta` |
| CH-P1-FEISHU-ID | WS 流式 recipient 类型不一致 | ✅ `ResolveReceiveTarget` + `receive_id_type` meta |
| CH-P1-CRED-FP | Runtime fingerprint 不含凭据 | ✅ `CredentialsRevision` 纳入 fingerprint |
| CH-P2-STREAM-ERR | `OnReplyDelta` 静默失败 | ✅ 错误中断 stream consume |
| CH-P2-STREAM-METRICS | 流式无 Prometheus | ✅ `aranea_channel_stream_update_total` |
| CH-P2-DUAL-REG | outbound/stream 双 registry | ✅ `platformAdapters` 统一注册 |
| CH-P2-SESSION-SRP | `ensureChannelSession` 位置 | ✅ `channel_ingress_session.go` + `prepareChannelChatRequest` |
| CH-P2-TURN-ERR | 部分 Reply + HasError 仍成功 | ✅ `streamPreviewTurnError` + `TurnErrStreamPreviewFailed` |
| CH-P2-FEISHU-UNARY | unary 出站未用 receive_id_type | ✅ `SendTextMessage` + `FeishuTextSender.ReceiveIDType` |
| CH-P3-RELOAD-WARN | Reload 凭据失败静默 | ✅ `channel.runtime.credentials_fail`（FlowLog，禁 slog） |
| CH-P3-FEISHU-WS | larkws 同步入站 + open_id 出站 | ✅ `safego` 异步 + `chat_id` + FlowLog |
| CH-P3-REGISTRY-NAME | registry 文件命名 | ✅ `channel_platform_registry.go` |

### 历史 P1–P2（已闭合）

Webhook 统一入站 · delivery 指数退避 · OutboundRegistry · QQ botgo · catalog 凭据 · Runtime 断线重连 · 定期 Reload

---

## 入站消息路径（当前）

```
Webhook POST /webhooks/{key}  ─┐
Runtime Manager (connectors) ──┼→ ProcessInbound
                               │    ├─ streaming_enabled → processInboundStreaming
                               │    │       → RunNativeTurnStreaming → OnReplyDelta → StreamSender
                               │    └─ unary → processInboundCore → delivery queue → platformAdapters.outbound
```

---

## 仍开放风险（P2）

| ID | 问题 | 建议 |
|----|------|------|
| CH-P2-E2E | 无 streaming 全链路集成测 | mock ChatService + httptest 平台 API（部分：fallback 单测） |
| CH-P2-SOAK | 多实例 Runtime 生产 soak | 断网 / 凭据轮换压测 |
| CH-P2-STREAM-MORE | 钉钉/Discord 流式 | Phase D 后续 |
| CH-P2-WEBHOOK-TEST | 全平台验签单测 | 补 discord、qq 负例 |

---

## 运维指标

| 指标 | 标签 |
|------|------|
| `aranea_channel_delivery_total` | `platform`, `status` |
| `aranea_channel_runtime_reconnect_total` | `platform`, `receive_mode`, `outcome` |
| `aranea_channel_stream_update_total` | `platform`, `phase`, `result` |

环境变量：`CHANNEL_RUNTIME_RELOAD_INTERVAL`（默认 `2m`）

---

## 建议后续

1. 流式 E2E + Grafana 面板（delivery + stream + runtime）。
2. 钉钉卡片流式 / Discord 流式（W5 扩展）。
3. 群 @ 门控 + allowlist（D2）。
