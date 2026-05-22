# Channel 渠道 — 开发计划

> **版本**：2026-05-22 | **状态**：🟢 9 平台连接；Runtime 生产级重连 + 流式出站 MVP  
> **需求**：[17 channel.md](./17%20channel.md) · **设计**：[17 channel.design.md](./17%20channel.design.md) · **业务集成**：[17-channel-agent-team-integration.md](./17-channel-agent-team-integration.md)  
> **平台参考**：[MuseBot](https://github.com/yincongcyincong/MuseBot) `robot/`（MIT）  
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-08

---

## 1. 模块定位

Channel：在 Kratos 层实现外部 IM 平台连接，参考 MuseBot 的 SDK 选型与连接模式，桥接到 `ChatService` Agent 运行时。

**代码锚点**：

- `api/kratos/channel/v1/channel.proto`
- `internal/service/channel.go` / `channel_ingress*.go` / `channel_delivery_worker.go` / `channel_runtime.go`
- `internal/biz/channel*.go` / `channel_catalog.go`
- `internal/channel/{lark,dingtalk,wecom,slack,telegram,discord,wechat,onebot,qq,runtime}/`
- `internal/cronrunner/jobs/channel_delivery.go` / `channel_health.go`
- `web/src/features/channels/` · `web/src/components/channels/`

**MuseBot 对照文件**（连接层移植时阅读）：

| 平台 | MuseBot |
|------|---------|
| 飞书 | `robot/lark.go` |
| 钉钉 | `robot/ding.go` |
| 企微 | `robot/comwechat.go` |
| 微信 | `robot/wechat.go` |
| Slack | `robot/slack.go` |
| Telegram | `robot/telegram.go` |
| Discord | `robot/discord.go` |
| QQ | `robot/qq.go` |
| OneBot | `robot/personalqq.go` |
| Webhook 路由 | `http/http.go` · `http/communicate.go` |

---

## 2. 现状评估

| 项 | 状态 | 说明 |
|----|------|------|
| Webhook 入站 7 平台 | ✅ | feishu / dingtalk / wecom / slack / telegram / wechat / onebot |
| 统一 ProcessInbound | ✅ | webhook + runtime 共用；流式/一元分支 |
| 异步 delivery + 重试 | ✅ | worker 5s，指数退避，最多 3 次 |
| delivery Prometheus + dead-letter | ✅ | `aranea_channel_delivery_total{platform,status}` |
| DB 多实例 + 凭据加密 | ✅ | `channel` + `channel_credential` |
| Catalog bundled 标记 | ✅ | 9/10 平台 bundled（qq ✅ botgo） |
| MuseBot 全平台 Catalog 规格 | ✅ | 文档 + catalog 10 项 |
| 长连接 Runtime | ✅ | larkws / ding stream / socketmode / polling / discord |
| Manager.Reload reconcile | ✅ | config/enabled/receive_mode fingerprint |
| Runtime 断线重连 | ✅ | `runSupervised` 指数退避 1s→5m |
| Runtime fingerprint 含凭据 revision | ✅ | `CredentialsRevision` + CRUD reload |
| 流式 edit 回复 | ✅ MVP | Telegram / Feishu / Slack；其余 unary 回退 |
| 流式错误传播 + Prometheus | ✅ | `OnReplyDelta` 中断 + `aranea_channel_stream_update_total` |
| platformAdapters 统一出站/流式 | ✅ | `channel_platform_registry.go` |
| 全平台 webhook 单测 | 🟡 | 部分（lark/dingtalk/slack/telegram/wecom/wechat/onebot） |
| 前端 MuseBot 布局 + composable | ✅ | `useChannelEditorForm.ts` |

---

## 3. 平台矩阵（MuseBot 对齐）

| type | 连接模式（MuseBot） | Aranea 现状 | bundled | SDK |
|------|---------------------|-------------|---------|-----|
| 飞书 / feishu | larkws（默认） | larkws ✅ · Webhook UI 暂未开放 · 流式 patch ✅ | ✅ | larksuite/oapi-sdk-go |
| `dingtalk` | Stream / webhook | webhook ✅ · stream ✅ | ✅ | dingtalk-stream-sdk-go |
| `wecom` | webhook | ✅ | ✅ | PowerWeChat / 自研 |
| `wecom-app` | webhook | ✅ | ✅ | 同上 |
| `wechat` | webhook | webhook ✅ · outbound ✅ | ✅ | PowerWeChat |
| `slack` | Socket Mode / Events | Events ✅ · socketmode ✅ · 流式 update ✅ | ✅ | slack-go + socketmode |
| `telegram` | polling / webhook | webhook ✅ · polling ✅ · 流式 edit ✅ | ✅ | go-telegram-bot-api |
| `discord` | gateway WS | gateway ✅ · outbound ✅ | ✅ | discordgo |
| `qq` | webhook + botgo WS | webhook ✅ · outbound ✅ | ✅ | botgo |
| `personal_qq` | OneBot HTTP | webhook ✅ · outbound ✅ | ✅ | OneBot 协议 |

---

## 4. 路线图

### Phase A — 巩固 Webhook（P0）

| # | 任务 | 状态 |
|---|------|------|
| A1 | 文档按 MuseBot 重写 | ✅ |
| A2 | Catalog 扩展至 10 平台 | ✅ |
| A3 | 全平台 webhook 验签单测 | 🟡 |
| A4 | `channel/port.go` 接口草案 | ⏳ |

### Phase B — ChannelRuntimeManager（P1）

| # | 任务 | 状态 | MuseBot 参考 |
|---|------|------|--------------|
| B1 | `channel_runtime.go` + Wire | ✅ | `StartRobot()` |
| B2 | 飞书 `receive_mode=websocket` | ✅ | `StartLarkRobot` + larkws |
| B3 | 钉钉 `receive_mode=stream` | ✅ | `StartDingRobot` + StreamClient |
| B4 | Slack `receive_mode=socket_mode` | ✅ | `StartSlackRobot` |
| B5 | Telegram `receive_mode=polling` | ✅ | `StartTelegramRobot` |
| B6 | Discord gateway | ✅ | `discord.go` |
| B7 | Manager.Reload reconcile | ✅ | — |
| B8 | 断线重连 + Prometheus | ✅ | `runSupervised` |
| B9 | 定期 Reload 兜底 | ✅ | 2m ticker |

### Phase C — 新平台连接（P1–P2）

| # | 任务 | 状态 | MuseBot 参考 |
|---|------|------|--------------|
| C1 | 微信公众号 `wechat` | ✅ | `wechat.go` + PowerWeChat |
| C2 | Discord `discord` | ✅ | `discord.go` + discordgo |
| C3 | QQ 官方 `qq` | ✅ | `qq.go` + botgo |
| C4 | OneBot `personal_qq` | ✅ | `personalqq.go` |

### Phase D — 体验增强（P2）

| # | 任务 | 状态 | MuseBot 参考 |
|---|------|------|--------------|
| D1 | 流式回复 StreamOutbound | ✅ MVP | Telegram/Feishu/Slack |
| D2 | 群 @ 门控 + allowlist | ✅ | `internal/biz/channel_access.go` + `channel_ingress_access.go` |
| D3 | 路由 UI：Team / dm_scope / rules | 🟡 | Agent/Team 下拉 ✅；`dm_scope` 下拉 ✅；rules 表 ⏳ |
| D5 | Web Chat 同步 Channel 入站 | ✅ | `useChatInboundSync` + session `metadata_json.source=channel` |
| D6 | 路由变更重置 peer 绑定 | ✅ | `UpdateChannel` + `DeleteByChannelID` |
| D4 | delivery Prometheus + dead-letter | ✅ | `aranea_channel_delivery_*` |

---

## 5. 流式出站（W5）

**开关**：`config_json.config.streaming_enabled: true`

**路径**：

```
ProcessInbound → processInboundStreaming → RunNativeTurnStreaming
  → trpc_turn OnReplyDelta → platform StreamSender.Update
```

| 平台 | 首条 | 增量 | 实现 |
|------|------|------|------|
| `telegram` | sendMessage | editMessageText（2s 节流） | `internal/channel/telegram/stream_outbound.go` |
| `feishu` | im.v1 messages POST | im.v1 messages PATCH | `internal/channel/lark/stream_outbound.go` |
| `slack` | chat.postMessage | chat.update（2s 节流） | `internal/channel/slack/stream_outbound.go` |
| 其他 | — | unary 回退 | delivery 队列 |

流式回合不走 outbound delivery 队列；失败写入 `channel_delivery` audit（`status=streamed|error`）。

---

## 6. Runtime 运维

| 指标 | 说明 |
|------|------|
| `aranea_channel_runtime_connected{platform,receive_mode}` | 活跃长连接数 |
| `aranea_channel_runtime_reconnect_total{platform,receive_mode,outcome}` | disconnect / attempt |
| `aranea_channel_stream_update_total{platform,phase,result}` | 流式 delta / flush（ok / error） |
| `aranea_channel_delivery_total{platform,status}` | delivery 含 dead_letter |

**行为**：

- 意外断线：`runSupervised` 指数退避重连（1s → 5m）
- CRUD / 凭据变更：`ChannelService.reloadRuntime()` → fingerprint reconcile（含 `CredentialsRevision`）
- 定期兜底：启动后每 **2 分钟** `Manager.Reload()`（可配置/关闭）
- **fingerprint 不含 `UpdatedAt`**（2026-05-22）：避免健康检查 metadata 更新触发双 WebSocket；替换连接器前等待旧实例 `done`

**入站门控（2026-05-22）**：

- `lark.AcceptFeishuInbound`：WS / Webhook 统一；仅 `sender_type=user`、必须有 `message_id`
- `channel_inbound_receipt`：同一 `feishu:{message_id}` 只 Turn 一次
- 审计：`channel.inbound.receive` · `channel.runtime.connector_start`
- 详见 [changelog/2026-05-22-Channel-Inbound-Root-Cause.md](../changelog/2026-05-22-Channel-Inbound-Root-Cause.md) · [review/2026-05-22-Channel-Inbound-Review.md](../review/2026-05-22-Channel-Inbound-Review.md)

---

## 7. 验收标准

- [x] 文档以 MuseBot 平台连接为参考源（无 GoClaw 依赖）
- [x] 7 平台 Webhook 端到端可用
- [x] Runtime scaffold（5 长连接 + Reload reconcile）
- [x] Runtime 断线重连 + 定期 Reload
- [x] 流式出站 Telegram / Feishu / Slack
- [x] delivery Prometheus dead-letter 指标
- [x] 前端 MuseBot 布局 + `useChannelEditorForm`
- [x] Catalog 含 MuseBot 10 平台
- [x] `go test ./internal/channel/... ./internal/service/...` 全绿
- [ ] Phase B 多实例生产压测（可选）

---

## 8. Review 优化闭合（2026-05-22）

| 项 | 实现 |
|----|------|
| `trpc_turn.go` UTF-8 | 恢复中文 FlowLog + `OnReplyDelta` wiring |
| Feishu WS `receive_id_type` | `lark.ResolveReceiveTarget` + StreamSender 动态 type |
| Runtime 凭据 fingerprint | `runtime.CredentialsRevision` |
| 流式错误传播 | `turn_helpers.OnReplyDelta` 失败中断 turn |
| 流式指标 | `aranea_channel_stream_update_total` |
| 统一 platform registry | `platformAdapters`（`channel_platform_registry.go`） |
| Session SRP | `channel_ingress_session.go` + `prepareChannelChatRequest` |
| 流式失败 fail-fast | `streamPreviewTurnError` → `TurnErrStreamPreviewFailed` |
| Feishu unary receive_id_type | `FeishuTextSender` + `SendTextMessage` |
| Runtime 凭据读取失败 | `event.SysLogWarn` → `channel.runtime.credentials_fail` |
| Feishu WS 异步入站 | `safego.Go` + `chat_id` 出站 + 群 @ 门控 |
| Feishu 默认连接模式 | catalog / UI 默认 `websocket`；Webhook 暂未开放 |
| Channel ROUTING UI | Agent / Team 下拉联动系统实例 |

详见 [17-channel-review.md](../review/17-channel-review.md)（**92/100**）。

---

## 9. 运维

| 变量 | 作用 |
|------|------|
| `CHANNEL_DELIVERY_DISABLED=1` | 关闭出站 worker |
| `CHANNEL_HEALTH_DISABLED=1` | 关闭健康扫描 |
| `CHANNEL_RUNTIME_DISABLED=1` | 关闭长连接 Runtime |
| `CHANNEL_RUNTIME_RELOAD_INTERVAL` | 定期 Reload 间隔（默认 `2m`；`0`/`off` 关闭） |
| `ARANEA_CREDENTIAL_KEY` | 凭据加密 |

Webhook 默认：`/webhooks/{channel_key}`。
