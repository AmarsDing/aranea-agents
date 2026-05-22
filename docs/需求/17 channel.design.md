# Channel 渠道 — 实现设计文档

> 对应需求：[17 channel.md](./17%20channel.md)  
> **跨模块集成**：[17-channel-agent-team-integration.md](./17-channel-agent-team-integration.md) · [integration.design.md](./17-channel-agent-team-integration.design.md)  
> 遵循：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)、[AGENT_RUNTIME_BOUNDARY.md](../guides/AGENT_RUNTIME_BOUNDARY.md)  
> 平台连接参考：[MuseBot](https://github.com/yincongcyincong/MuseBot) `robot/` + `http/`（MIT）

---

## 一、模块概述

Channel 在 **Kratos 传输层** 负责外部 IM 平台连接：凭据管理、入站标准化、路由到 Agent/Team、异步出站。

**设计原则**：

1. **只移植 MuseBot 的连接层**（SDK 初始化、收消息、发消息、验签）— 不移植 `Robot` 接口内的 LLM/`requestLLM`/`executeLLM`
2. **DB 多实例** — 每个 `channel` 行独立连接；非 MuseBot 全局 singleton
3. **双入站路径** — Webhook（✅）+ Runtime 长连接（✅ scaffold，见 `internal/channel/runtime/`）
4. **`internal/biz` 禁止 import trpc-agent-go** — Agent 调用仅在 `internal/service`

**当前实现状态**（2026-05-22，P1 优化后）：

| 能力 | 状态 |
|------|------|
| Proto CRUD + Catalog + Test + Credentials | ✅ |
| Webhook 入站：feishu / dingtalk / wecom / slack / telegram / wechat / onebot | ✅ |
| 统一入站：`ProcessInbound` + `processInboundHTTP` | ✅ |
| 异步 `channel_delivery` + Worker（3 次重试） | ✅ |
| 长连接 Runtime（larkws / ding stream / socketmode / polling / discord gateway） | ✅ scaffold |
| `Manager.Reload` 配置 fingerprint reconcile | ✅ |
| 流式回复（edit-in-place） | ❌ |
| wechat 出站（被动 ReplyXML + 主动 API） | ✅ |
| discord 出站 | ✅ |
| personal_qq / onebot 出站 | ✅ |
| qq 官方 botgo | ❌ |

---

## 二、架构与数据流

```
┌─────────────────────────────────────────────────────────────────┐
│ 入站 A：Webhook（已实现）                                          │
│   POST /webhooks/{channel_key}  →  ChannelIngress               │
└────────────────────────────┬────────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────────┐
│ 入站 B：Runtime（已实现 scaffold）                                 │
│   ChannelRuntime.Start → Manager.Reload(reconcile)               │
│     ├ feishu:   larkws.Client          [MuseBot lark.go]         │
│     ├ dingtalk: StreamClient           [MuseBot ding.go]        │
│     ├ slack:    socketmode.Client      [MuseBot slack.go]       │
│     ├ telegram: GetUpdatesChan         [MuseBot telegram.go]    │
│     └ discord:  discordgo.Session      [MuseBot discord.go]     │
└────────────────────────────┬────────────────────────────────────┘
                             │  NormalizedInbound { channelID, peerID, text, meta }
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│  internal/service/channel_consumer.go（规划，逻辑同 ingress）      │
│   ParseChannelRouting → ResolveChannelTarget                     │
│   channel_peer_session → Session                                 │
│   ChatService.RunNativeTurnUnary                                 │
│   enqueueOutboundReply                                           │
└────────────────────────────┬────────────────────────────────────┘
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│  channel_delivery → ChannelDeliveryWorker → platform SendText    │
│  Phase 2: StreamSender（MsgChunk → edit message）                │
└─────────────────────────────────────────────────────────────────┘
```

---

## 三、Adapter 接口（`internal/channel/`）

MuseBot 的 `Robot` 接口混合了 LLM 与传输；Aranea **拆分为纯传输 port**：

```go
// internal/channel/port.go（规划）

type InboundEvent struct {
    ChannelID      string
    ChannelKey     string
    PlatformType   string
    PeerID         string
    PeerKey        string
    Text           string
    IdempotencyKey string
    Meta           map[string]string // thread_id, reply_target, session_webhook, ...
}

// Connector：长连接平台实现
type Connector interface {
    Identified
    Start(ctx context.Context, cfg RuntimeConfig) error
    Stop(ctx context.Context) error
}

// WebhookAdapter：HTTP 回调平台实现（当前）
type WebhookAdapter interface {
    Verify(ctx context.Context, r *http.Request, creds []biz.ChannelCredential) ([]byte, error)
    ParseInbound(raw []byte, ch biz.Channel) (InboundEvent, error)
}

// OutboundAdapter：出站
type OutboundAdapter interface {
    Identified
    SendText(ctx context.Context, ch biz.Channel, creds []biz.ChannelCredential, ev OutboundPayload) error
}

// StreamOutbound：可选，MuseBot StreamRobot.sendTextStream
type StreamOutbound interface {
    OutboundAdapter
    SendStream(ctx context.Context, ch biz.Channel, target string, chunks <-chan string) error
}
```

现有 `internal/channel/surface.go`（`Identified` / `Runner` / `OutboundText`）逐步收敛到上述 port。

---

## 四、Proto / Biz / Data

### 4.1 Proto

`api/kratos/channel/v1/channel.proto` — CRUD、Catalog、Test、Credentials、Deliveries。  
Webhook 路由在 `internal/server/http.go`，不进 Proto。

### 4.2 Biz

| 文件 | 职责 |
|------|------|
| `channel.go` | CRUD、Test commit |
| `channel_catalog.go` | MuseBot 对齐的 10 平台目录 |
| `channel_routing.go` | Agent/Team/dm_scope/rules |
| `channel_access.go` | `allowed_user_ids` / `allowed_group_ids` / `require_mention` 策略解析与判定 |
| `channel_rules.go` | 凭据校验、requiredCredentials |

### 4.3 Data

| 表 | 说明 |
|----|------|
| `channel` | 实例主表 |
| `channel_credential` | `secret_ref`（enc:/env:） |
| `channel_delivery` | 出站队列 |
| `channel_peer_session` | peer → session |

---

## 五、Service 层

### 5.1 ChannelIngress（Webhook）

`internal/service/channel_ingress*.go` — 平台 switch → `internal/channel/{lark,dingtalk,wecom,slack,telegram}/webhook.go`

**入站访问控制**（`channel_ingress_access.go`，在 Agent Turn 之前）：

```
InboundEvent + channel.config_json
  → biz.ParseChannelAccessPolicy
  → inboundAccessContextFromEvent(ev)   // PeerID, chat_id, chat_type, mentioned
  → policy.Allows(ctx)
       ├ false → recordDelivery(access_denied) + enqueueOutboundReply(拒绝文案)
       └ true  → ResolveChannelTarget → ChatService
```

| 入站 meta 字段 | 用途 |
|----------------|------|
| `sender_open_id` / `sender_user_id` / `PeerID` | 匹配 `allowed_user_ids` |
| `chat_id` + `chat_type=group` | 匹配 `allowed_group_ids` |
| `mentioned` / `mentions` | 匹配 `require_mention` |

飞书 WS：`internal/channel/lark/parse_message.go` 写入 `sender_open_id`、`chat_type`、`mentioned`。Webhook 路径由 `channel_ingress.go` 补全 `chat_type`（`InferChatTypeFromChatID`）。

### 5.2 ChannelRuntimeManager（规划）

```go
// internal/service/channel_runtime.go
type ChannelRuntimeManager struct {
    channels *biz.ChannelUsecase
    ingress  *ChannelIngress  // 复用 runChatTurn / enqueueOutboundReply
    runners  map[string]context.CancelFunc
}

func (m *ChannelRuntimeManager) Reload(ctx context.Context) error
func (m *ChannelRuntimeManager) startInstance(ctx context.Context, ch biz.Channel) error
```

启动逻辑参考 MuseBot `StartRobot()`：按 `config_json.type` + `receive_mode` 选择 Connector，**每个 DB 实例一个 goroutine**。

Wire：admin 进程启动时 `Reload()`；Toggle/Update 后触发单实例重启。

### 5.3 出站

`channel_delivery_worker.go` — 按 type 分发；钉钉 Webhook 模式用 `session_webhook`（MuseBot 同）；Stream 模式用 OpenAPI（MuseBot `ding.go`）。

---

## 六、MuseBot 平台实现对照

| type | Aranea 包 | 当前 | MuseBot 文件 | MuseBot 连接 | 目标 SDK |
|------|-----------|------|--------------|-------------|----------|
| `feishu` | `lark/` | webhook ✅ | `lark.go` | larkws | `larksuite/oapi-sdk-go/v3` |
| `dingtalk` | `dingtalk/` | webhook ✅ | `ding.go` | StreamClient | `open-dingtalk/dingtalk-stream-sdk-go` |
| `wecom` | `wecom/` | webhook ✅ | `comwechat.go` | HTTP `/com/wechat` | PowerWeChat work |
| `wecom-app` | `wecom/` | webhook ✅ | 同上 | 同上 | 同上 |
| `wechat` | — | ❌ | `wechat.go` | HTTP `/wechat` | PowerWeChat officialAccount |
| `slack` | `slack/` | Events webhook ✅ | `slack.go` | Socket Mode | `slack-go/slack` + socketmode |
| `telegram` | `telegram/` | webhook ✅ | `telegram.go` | Long polling | `go-telegram-bot-api/v5` |
| `discord` | — | ❌ | `discord.go` | discordgo Open | `bwmarrin/discordgo` |
| `qq` | — | ❌ | `qq.go` | webhook + botgo WS | `tencent-connect/botgo` |
| `personal_qq` | — | ❌ | `personalqq.go` | OneBot POST `/onebot` | 自定义 + OneBot HTTP |

### 6.1 MuseBot HTTP 路由 → Aranea 映射

| MuseBot 路径 | 平台 | Aranea 建议 |
|--------------|------|-------------|
| `/wechat` | 微信公众号 | `/webhooks/{channel_key}` 或 `/channels/wechat/{key}` |
| `/com/wechat` | 企微 | 同上（已实现 wecom handler） |
| `/qq` | QQ 官方 | 专用路由 + botgo 事件 |
| `/onebot` | OneBot | `/webhooks/{channel_key}` + HMAC 验签 |

统一原则：**优先** `/webhooks/{channel_key}`；企微/微信 AES 回调可保留类型前缀便于调试。

### 6.2 从 MuseBot 迁移连接代码 Checklist

1. 阅读 MuseBot `robot/<platform>.go` 中 **Start* / handler / send* 函数**（跳过 `requestLLM`、`ExecCmd`）
2. 提取 SDK 初始化与消息 parse/send 到 `internal/channel/<platform>/`
3. 入站转为 `InboundEvent`，调用 `ChannelIngress.runChatTurn`（或 consumer 等价物）
4. 出站从 `OutboundPayload` 调 MuseBot 同款 API
5. 凭据字段映射到 `channel_credential`（见需求 §5）
6. 添加 `webhook_test.go` 或 integration test
7. catalog `bundled=true`，前端 schema 补字段

**许可证**：MuseBot MIT — 迁移时保留版权声明。

### 6.3 不应从 MuseBot 复制

- `Robot` / `RobotInfo` 整体接口与 LLM 耦合
- 全局变量（`TelegramBot`、`LarkBotClient` 等）— 改为实例级 DI
- `SendMsg()` 巨型 switch — 改为 registry `map[type]OutboundAdapter`
- `go RobotInfo.Exec()` 无界并发 — 使用 bounded worker pool
- MuseBot 内置 HTTP `:36060` 混布 — Webhook 挂 Kratos `internal/server`

---

## 七、流式回复（Phase 2）

MuseBot 模式：

```go
type MsgChan struct {
    NormalMessageChan chan *MsgInfo  // { Content, PartContent, Finished }
    StrMessageChan    chan string    // QQ 专用
}
```

Aranea 映射：

- Agent 流式事件（已有 Envelope/SSE）→ `StreamOutbound.SendStream`
- 平台实现：Telegram `EditMessageText`、飞书卡片更新、Slack `UpdateMessage`
- 配置：`config_json.config.streaming_enabled`

---

## 八、Web 前端

```
web/src/features/channels/
web/src/components/channels/
```

- `receive_mode` 下拉：选项来自 catalog `receive_modes`
- 非 bundled 平台：Catalog 展示「即将支持」
- 微信 `active_mode`、钉钉 Stream 凭据分区：Phase 2 schema 驱动

---

## 九、Wire 与运维

| 组件 | 说明 |
|------|------|
| `ChannelService` | gRPC/HTTP API |
| `ChannelIngress` | Webhook |
| `ChannelRuntimeManager` | 长连接（规划） |
| `ChannelDeliveryWorker` | cron 5s |

| 环境变量 | 作用 |
|----------|------|
| `CHANNEL_DELIVERY_DISABLED=1` | 关闭出站 worker |
| `CHANNEL_HEALTH_DISABLED=1` | 关闭健康扫描 |
| `CHANNEL_RUNTIME_DISABLED=1` | 关闭长连接 Runtime（规划） |
| `ARANEA_CREDENTIAL_KEY` | 凭据加密 |

---

## 十、测试策略

| 层级 | 内容 |
|------|------|
| `internal/channel/*/webhook_test.go` | 验签、parse |
| `internal/channel/*/connector_test.go` | mock SDK 入出站（规划） |
| `internal/biz/channel_*_test.go` | routing、config |
| E2E | 各平台 sandbox 账号 |

---

## 十一、新增平台优先级（MuseBot 顺序建议）

1. **P0 巩固**：现有 6 平台 Webhook 单测 + Runtime 骨架  
2. **P1 连接升级**：飞书 larkws、钉钉 Stream、Slack Socket Mode、Telegram polling  
3. **P1 国内补全**：微信公众号（PowerWeChat）  
4. **P2 海外/QQ**：Discord、QQ 官方、OneBot  
