# Channel 全量迁移计划（MuseBot → Aranea）

> **参考**：[MuseBot](https://github.com/yincongcyincong/MuseBot) `robot/*.go` + `http/communicate.go`（MIT）  
> **需求**：[17 channel.md](./17%20channel.md) · **设计**：[17 channel.design.md](./17%20channel.design.md)  
> **原则**：只迁移**平台连接层**（SDK 初始化、收消息、发消息、验签）；LLM/Agent 仍走 `ChatService`

---

## 总览

| 波次 | 内容 | 平台数 | 状态 |
|------|------|--------|------|
| **W0** | 运行时骨架 + 统一入站 API | — | ✅ |
| **W1** | 已有 Webhook 加固 + 凭据对齐 | 6 | ⏳ |
| **W2** | 长连接升级（MuseBot 主模式） | 4 | ✅ |
| **W3** | 国内 Webhook 补全 | 2 | ⏳ |
| **W4** | 海外 / QQ | 2 | ⏳ |
| **W5** | 流式出站 + 策略 | 全部 | ⏳ |
| **W6** | 前端 schema + E2E | 全部 | ⏳ |

---

## W0 — 运行时基础设施

| ID | 任务 | 产出 | 状态 |
|----|------|------|------|
| W0-1 | 统一消息类型 | `internal/channel/port/types.go` | ✅ |
| W0-2 | Connector 注册表 | `internal/channel/runtime/manager.go` | ✅ |
| W0-3 | RuntimeManager | `internal/channel/runtime/manager.go` | ✅ |
| W0-4 | 入站桥接 | `ChannelIngress.ProcessInbound` | ✅ |
| W0-5 | Service 层 | `internal/service/channel_runtime.go` | ✅ |
| W0-6 | Wire + 启动 | `cmd/admin/wire.go` · `main.go` | ✅ |
| W0-7 | 环境变量 | `CHANNEL_RUNTIME_DISABLED=1` | ✅ |
| W0-8 | 单测 | `runtime/manager_test.go` | ✅ |

**验收**：admin 启动后 `Reload()` 扫描 enabled channel；webhook 模式不启 goroutine；Toggle 后重启实例。

---

## W1 — 已有 Webhook 加固（6 平台）

| 平台 | type | MuseBot 文件 | Aranea 包 | 任务 | 状态 |
|------|------|-------------|-----------|------|------|
| 飞书 | `feishu` | `lark.go` | `lark/` | 凭据对齐 app_id/secret；验签单测 | ✅ 基础 |
| 钉钉 | `dingtalk` | `ding.go` | `dingtalk/` | Webhook 加签；Stream 凭据字段 | ✅ 基础 |
| 企微机器人 | `wecom` | `comwechat.go` | `wecom/` | PowerWeChat 验签路径评估 | ✅ 基础 |
| 企微应用 | `wecom-app` | `comwechat.go` | `wecom/` | corp_id/agent_id 配置 | ✅ 基础 |
| Slack | `slack` | `slack.go` | `slack/` | Events 验签；app_token 字段 | ✅ 基础 |
| Telegram | `telegram` | `telegram.go` | `telegram/` | getMe 测试；webhook 模式 | ✅ 基础 |

| ID | 横切任务 | 状态 |
|----|----------|------|
| W1-1 | 全部 `webhook_test.go` 覆盖验签 | ⏳ |
| W1-2 | `requiredCredentials` 与 MuseBot conf 字段 1:1 | ✅ |
| W1-3 | catalog `bundled=true` | ✅ |

---

## W2 — 长连接升级（MuseBot 默认模式）

| 平台 | receive_mode | MuseBot 入口 | SDK | Aranea 文件 | 状态 |
|------|--------------|-------------|-----|-------------|------|
| 飞书 | `websocket` | `StartLarkRobot` | `larksuite/oapi-sdk-go/v3` + larkws | `lark/ws.go` | ✅ |
| 钉钉 | `stream` | `StartDingRobot` | `dingtalk-stream-sdk-go` | `dingtalk/stream.go` | ✅ |
| Slack | `socket_mode` | `StartSlackRobot` | `slack-go/slack` + socketmode | `slack/socketmode.go` | ✅ |
| Telegram | `polling` | `StartTelegramRobot` | `go-telegram-bot-api/v5` | `telegram/polling.go` | ✅ |

| ID | 任务 | 状态 |
|----|------|------|
| W2-1 | `go.mod` 引入上述 SDK | ✅ |
| W2-2 | 各 Connector 实现 `runtime.Starter` | ✅ |
| W2-3 | 群 @ 门控 `require_mention` | ⏳ |
| W2-4 | 编辑页 `receive_mode` 下拉 | ⏳ |

**验收**：无公网 IP 下飞书 WS / 钉钉 Stream / Telegram polling 可收发。

---

## W3 — 国内 Webhook 补全

| 平台 | type | MuseBot | SDK | 路由 | 状态 |
|------|------|---------|-----|------|------|
| 微信公众号 | `wechat` | `wechat.go` + `WechatComm` | 轻量 XML 验签 | `/webhooks/{key}` | ✅ 基础 |
| 企微增强 | `wecom-app` | `ComWechatComm` | PowerWeChat work | 同上 | ⏳ |

| ID | 任务 | 状态 |
|----|------|------|
| W3-1 | `internal/channel/wechat/` PowerWeChat 适配 | ⏳ |
| W3-2 | 被动回复 vs `active_mode` 客服 API | ⏳ |
| W3-3 | ingress 注册 wechat type | ✅ |
| W3-4 | catalog `bundled=true` | ✅ |

---

## W4 — Discord + QQ

| 平台 | type | 连接 | MuseBot | Aranea | 状态 |
|------|------|------|---------|--------|------|
| Discord | `discord` | gateway WS | `discord.go` | `discord/gateway.go` | ✅ |
| QQ 官方 | `qq` | webhook + botgo WS | `qq.go` | `qq/` | ⏳ |
| OneBot | `personal_qq` | HTTP 推送 | `personalqq.go` | `onebot/` | ✅ 基础 |

| ID | 任务 | 状态 |
|----|------|------|
| W4-1 | discordgo Gateway + 出站 | ✅ |
| W4-2 | botgo webhook 验签 + 事件 WS | ⏳ |
| W4-3 | OneBot HMAC + 反向 HTTP 发送 | ✅ |

---

## W5 — 流式出站与策略

| ID | 任务 | MuseBot 参考 | 状态 |
|----|------|-------------|------|
| W5-1 | `StreamOutbound` 接口 | `MsgChan` | ⏳ |
| W5-2 | Telegram edit 流式 | `sendTextStream` | ⏳ |
| W5-3 | 飞书卡片更新 | lark im update | ⏳ |
| W5-4 | Slack message update | slack.go | ⏳ |
| W5-5 | `allowed_user_ids` / `allowed_group_ids` | conf allowlist | ⏳ |
| W5-6 | delivery Prometheus 指标 | — | ⏳ |

---

## W6 — 前端与 E2E

| ID | 任务 | 状态 |
|----|------|------|
| W6-1 | schema 驱动凭据表单（10 平台） | ⏳ |
| W6-2 | receive_mode / connection_mode UI | ⏳ |
| W6-3 | 微信 active_mode、钉钉 client_id 分区 | ⏳ |
| W6-4 | 路由 Team / dm_scope / rules | ⏳ |
| W6-5 | 各平台 sandbox E2E 清单 | ⏳ |

---

## SDK 依赖清单

```
github.com/go-telegram-bot-api/telegram-bot-api/v5
github.com/larksuite/oapi-sdk-go/v3
github.com/open-dingtalk/dingtalk-stream-sdk-go
github.com/slack-go/slack
github.com/bwmarrin/discordgo
github.com/ArtisanCloud/PowerWeChat/v3
github.com/tencent-connect/botgo
```

---

## MuseBot → Aranea 文件映射

| MuseBot | Aranea |
|---------|--------|
| `robot/telegram.go` | `internal/channel/telegram/{webhook,polling,outbound}.go` |
| `robot/lark.go` | `internal/channel/lark/{webhook,ws,outbound}.go` |
| `robot/ding.go` | `internal/channel/dingtalk/{webhook,stream,outbound}.go` |
| `robot/slack.go` | `internal/channel/slack/{webhook,socketmode,outbound}.go` |
| `robot/discord.go` | `internal/channel/discord/gateway.go` |
| `robot/comwechat.go` | `internal/channel/wecom/` |
| `robot/wechat.go` | `internal/channel/wechat/` |
| `robot/qq.go` | `internal/channel/qq/` |
| `robot/personalqq.go` | `internal/channel/onebot/` |
| `http/communicate.go` | `internal/service/channel_ingress_*.go` |
| `StartRobot()` | `ChannelRuntimeManager.Reload()` |

---

## 进度汇总

| 平台 | Webhook | 长连接 | 出站 | bundled |
|------|---------|--------|------|---------|
| feishu | ✅ | ✅ WS | ✅ | ✅ |
| dingtalk | ✅ | ✅ Stream | ✅ | ✅ |
| wecom | ✅ | — | ✅ | ✅ |
| wecom-app | ✅ | — | ✅ | ✅ |
| wechat | ✅ 基础 | — | ⏳ 被动回复 | ✅ |
| slack | ✅ Events | ✅ Socket | ✅ | ✅ |
| telegram | ✅ | ✅ Poll | ✅ | ✅ |
| discord | — | ✅ GW | ✅ | ✅ |
| qq | ⏳ | ⏳ | ⏳ | ❌ |
| personal_qq | ✅ OneBot | — | ✅ | ✅ |

**完成定义（全部迁移）**：上表全部 ✅ + W5 流式 + W6 前端 + `go test ./internal/channel/...` 全绿。
