# Channel 渠道 — 实现设计文档

> 对应需求：[17 channel.md](./17%20channel.md)  
> **跨模块集成**：[17-channel-agent-team-integration.md](./17-channel-agent-team-integration.md) · [integration.design.md](./17-channel-agent-team-integration.design.md)  
> 遵循：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)、[AGENT_RUNTIME_BOUNDARY.md](../guides/AGENT_RUNTIME_BOUNDARY.md)  
> 平台连接参考：[MuseBot](https://github.com/yincongcyincong/MuseBot) `robot/` + `http/`（MIT）；飞书 IM 行为对照 [Hermes Agent](https://github.com/NousResearch/hermes-agent) `gateway/platforms/feishu.py` — 见 **§十四**

---

## 一、模块概述

Channel 在 **Kratos 传输层** 负责外部 IM 平台连接：凭据管理、入站标准化、路由到 Agent/Team、异步出站。

**设计原则**：

1. **只移植 MuseBot 的连接层**（SDK 初始化、收消息、发消息、验签）— 不移植 `Robot` 接口内的 LLM/`requestLLM`/`executeLLM`
2. **DB 多实例** — 每个 `channel` 行独立连接；非 MuseBot 全局 singleton
3. **双入站路径** — Webhook（✅）+ Runtime 长连接（✅ scaffold，见 `internal/channel/runtime/`）
4. **`internal/biz` 禁止 import trpc-agent-go** — Agent 调用仅在 `internal/service`

**当前实现状态**（2026-06-06，Phase K+L 全部闭合）：

| 能力 | 状态 |
|------|------|
| Proto CRUD + Catalog + Test + Credentials | ✅ |
| Webhook 入站：feishu / dingtalk / wecom / slack / telegram / wechat / onebot / line / mattermost / teams | ✅ |
| 统一入站：`ProcessInbound` + `processInboundHTTP` | ✅ |
| 异步 `channel_delivery` + Worker（3 次重试） | ✅ |
| 长连接 Runtime（larkws / ding stream / socketmode / polling / discord gateway / mattermost websocket） | ✅ |
| `Manager.Reload` 配置 fingerprint reconcile | ✅ |
| 流式回复（edit-in-place） | ✅ MVP（Telegram / Feishu / Slack / LINE / Mattermost） |
| 长任务：Webhook 同步阻塞 Turn | ✅ Phase E1（async execute） |
| 长任务：入队 IM 反馈 | ✅ Phase E1-4 |
| 长任务：ChannelTurnJob | ✅ Phase E3 + ListChannelTurnJobs API |
| wechat 出站（被动 ReplyXML + 主动 API） | ✅ |
| discord 出站 | ✅ |
| personal_qq / onebot 出站 | ✅ |
| qq 官方 botgo | ✅ |
| LINE 出站 + 流式 | ✅ |
| Mattermost 出站 + 流式 + WS 长连接 | ✅ |
| Teams 出站（Bot Framework OAuth2） | ✅ |
| safego 合规 | ✅ |
| Service 层 kerrors 合规 | ✅ |
| Proto 隔离合规 | ✅ |
| Service 层无直接 Repo | ✅ |

---

## 二、架构与数据流

```
┌─────────────────────────────────────────────────────────────────┐
│ 入站 A：Webhook（已实现）                                          │
│   POST /webhooks/{channel_key}  →  ChannelIngress               │
└────────────────────────────┬────────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────────┐
│ 入站 B：Runtime（已实现）                                         │
│   ChannelRuntime.Start → Manager.Reload(reconcile)               │
│     ├ feishu:   larkws.Client          [MuseBot lark.go]         │
│     ├ dingtalk: StreamClient           [MuseBot ding.go]        │
│     ├ slack:    socketmode.Client      [MuseBot slack.go]       │
│     ├ telegram: GetUpdatesChan         [MuseBot telegram.go]    │
│     ├ discord:  discordgo.Session      [MuseBot discord.go]     │
│     └ mattermost: gorilla/websocket    [自研]                    │
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

#### 5.3.1 OutboundMeta 契约（CH-BOR-10）

入站 adapter 将平台回复目标写入 `port.InboundEvent.OutboundMeta`；**禁止**在 ingress/service 层散落硬编码路由字段。

| Key | 用途 |
|-----|------|
| `recipient` | 出站 receive_id（飞书必填） |
| `receive_id_type` | open_id / chat_id / … |
| `chat_id` · `chat_type` | 群/DM 路由与白名单 |
| `thread_id` · `reply_in_thread` | 话题线程（F-06） |
| `inbound_message_id` | 幂等 / reaction 锚点 |
| `session_webhook` · `response_url` | 钉钉 / Slack 等 webhook 回复 |
| `x_*` | 平台扩展（校验放行） |

**代码**：`internal/channel/port/meta.go`

- `LocalKeyFromMeta(platform, meta)` — preview/run 路由键（`chat_id:thread_id` 或 `recipient` 回退）
- `ValidateOutboundMeta` — 入站契约告警（`channel_ingress_guard` SysLog）
- `NormalizeOutboundMeta` — 出站透传前 trim

**新平台 checklist**：adapter build 阶段填满必填 meta → 单测 `ValidateOutboundMeta` → 禁止在 `channel_ingress_*` 新增 platform 特判路由字段。

---

## 六、MuseBot 平台实现对照

| type | Aranea 包 | 当前 | MuseBot 文件 | MuseBot 连接 | 目标 SDK |
|------|-----------|------|--------------|-------------|----------|
| `feishu` | `lark/` | webhook ✅ · WS ✅ · 流式 ✅ | `lark.go` | larkws | `larksuite/oapi-sdk-go/v3` |
| `dingtalk` | `dingtalk/` | webhook ✅ · stream ✅ | `ding.go` | StreamClient | `open-dingtalk/dingtalk-stream-sdk-go` |
| `wecom` | `wecom/` | webhook ✅ | `comwechat.go` | HTTP `/com/wechat` | PowerWeChat work |
| `wecom-app` | `wecom/` | webhook ✅ | 同上 | 同上 | 同上 |
| `wechat` | `wechat/` | webhook ✅ · outbound ✅ | `wechat.go` | HTTP `/wechat` | PowerWeChat officialAccount |
| `slack` | `slack/` | Events webhook ✅ · socketmode ✅ · 流式 ✅ | `slack.go` | Socket Mode | `slack-go/slack` + socketmode |
| `telegram` | `telegram/` | webhook ✅ · polling ✅ · 流式 ✅ | `telegram.go` | Long polling | `go-telegram-bot-api/v5` |
| `discord` | `discord/` | gateway ✅ · outbound ✅ | `discord.go` | discordgo Open | `bwmarrin/discordgo` |
| `qq` | `qq/` | webhook ✅ · outbound ✅ | `qq.go` | webhook + botgo WS | `tencent-connect/botgo` |
| `personal_qq` | `onebot/` | webhook ✅ · outbound ✅ | `personalqq.go` | OneBot POST `/onebot` | 自定义 + OneBot HTTP |
| `line` | `line/` | webhook ✅ · outbound ✅ · 流式 ✅ | — | — | `line-bot-sdk-go`（仅类型引用） |
| `mattermost` | `mattermost/` | webhook ✅ · WS ✅ · outbound ✅ · 流式 ✅ | — | — | `gorilla/websocket` + REST API v4 |
| `teams` | `teams/` | webhook ✅ · outbound ✅ | — | — | Bot Framework OAuth2 + REST API |

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
| `ChannelRuntimeManager` | 长连接 ✅ |
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

## 十一、新增平台优先级（已完成）

1. **P0 巩固** ✅：现有 10 平台 Webhook + Runtime  
2. **P1 连接升级** ✅：飞书 larkws、钉钉 Stream、Slack Socket Mode、Telegram polling  
3. **P1 国内补全** ✅：微信公众号（PowerWeChat）  
4. **P1 海外扩展** ✅：LINE、Mattermost、Teams  
5. **P2 海外/QQ** ✅：Discord、QQ 官方、OneBot  

---

## 十二、长任务异步执行设计

> **需求**：[17 channel.md §8](./17%20channel.md#8-长任务场景飞书-channel)  
> **开发计划**：[17-channel-development.md §10](./17-channel-development.md#10-长任务异步执行phase-e)  
> **跨模块 Turn**：[17-channel-agent-team-integration.design.md §4.4](./17-channel-agent-team-integration.design.md#44-长任务路径)

### 12.1 设计目标

| 目标 | 手段 |
|------|------|
| 飞书回调 SLA 与 Turn 解耦 | Ingress **Accept** 与 **Execute** 分两阶段 |
| 单一职责 | Ingress 受理；Worker 执行 Turn；Projector 进度；Delivery 出站 |
| 复用运行时 | `ChatService.RunNativeTurn*`；不新建 Channel 专用 LLM 路径 |
| biz 不触框架 | Job 状态、配置解析在 `internal/biz`；Runner 仅在 `internal/service` |

### 12.2 现状瓶颈（代码锚点）

| 瓶颈 | 位置 | 影响 |
|------|------|------|
| Webhook 同步阻塞 | `channel_ingress_http.go` → `ProcessInboundWebhook` | HTTP 等待 Turn 完成（最长 5min） |
| 入队无 IM 反馈 | `runNativeAgentTurn` 入队返回空 assistant → `enqueueOutboundReply` 跳过空 reply | 用户静默 |
| 全局 Turn 超时 | `trpc_turn.go` `defaultTurnTimeout = 5min` | Team/重工具易超时 |
| 首字节 30s | `trpc_turn.go` `firstByteTimeout` | 工具阶段无 text delta 即失败 |
| 流式仅 text delta | `processInboundStreaming` | 工具/Team 长静默无 PATCH |

WS 入站已异步（`lark/ws.go` `safego.Go` + `HandleWSInbound` `WithoutCancel`），Webhook 需对齐。

### 12.3 目标架构

```
InboundEvent
  → ChannelIngress.acceptInbound()     // 验签、幂等、访问控制、写 Job=accepted、发 ACK
  → HTTP 200 / WS handler return

ChannelTurnWorker / safego
  → ChannelIngress.executeInboundTurn()
       → prepareChannelChatRequest
       → RunNativeTurnStreaming | Unary（ctx 带 Channel 级 timeout）
       → ChannelProgressProjector（可选，订阅 EventBus）
       → StreamSender | enqueueOutboundReply
       → Job=completed|failed|timeout
```

**与 Web Chat 边界**：Turn、Session 锁、pending queue 仍在 `ChatService`；Channel 层只增加 **IM 投影**（ACK / 进度 / 最终文本）与 **Job 审计**。

### 12.4 包职责（SRP）

| 包 / 文件 | 职责 | 禁止 |
|-----------|------|------|
| `internal/biz/channel_turn_job.go` | Job 实体、Repo 接口、状态机、`ParseChannelLongTaskConfig` | import trpc-agent-go |
| `internal/biz/channel_config_helpers.go` | `streaming_enabled` 等已有 helper；扩展 timeout/ack/progress 解析 | Turn 执行 |
| `internal/data/channel_turn_job.go` | Ent schema + Repo 实现 | 业务规则 |
| `internal/service/channel_ingress_accept.go` | `acceptInbound`：幂等、ACK 出站、创建 Job | LLM |
| `internal/service/channel_ingress_execute.go` | `executeInboundTurn`：调 `RunNativeTurn*`、更新 Job | 平台 SDK |
| `internal/service/channel_ingress_http.go` | Webhook：Accept → 200 → async Execute | Turn 逻辑 |
| `internal/service/channel_turn_worker.go` | 扫 Job / 或仅 async goroutine 调度 | 验签 |
| `internal/service/channel_progress_projector.go` | 订阅 EventBus → StreamSender 进度 PATCH | 路由 / Session 创建 |
| `internal/service/trpc_turn.go` | 从 ctx 读取 Channel 注入的 timeout（可选值） | 飞书 API |
| `internal/channel/lark/stream_outbound.go` | 平台 PATCH（已有） | Agent Turn |

### 12.5 数据模型：`channel_turn_job`

| 列 | 类型 | 说明 |
|----|------|------|
| `id` | UUID PK | |
| `channel_id` | FK | |
| `session_id` | string | 绑定 Session |
| `peer_id` / `peer_key` | string | IM 对端 |
| `idempotency_key` | string UNIQUE(channel_id, key) | 与 receipt 对齐 |
| `status` | enum | accepted / running / completed / failed / timeout / cancelled |
| `preview_message_id` | string nullable | 飞书流式消息 ID |
| `error_message` | text nullable | |
| `started_at` / `finished_at` | timestamp | |

SQL 增量：`docs/sql/04_channel.sql`（或新编号文件）；Ent schema 与 migration 随 Phase E1。

**与现有表**：`channel_inbound_receipt` 保幂等；`channel_delivery` 保 unary 出站重试；Job 表保 **Turn 生命周期** 审计。

### 12.6 配置解析（biz）

扩展 `ChannelLongTaskConfig`（自 `config_json.config`）：

```go
type ChannelLongTaskConfig struct {
    AckMessage          string
    AckOnQueued         string
    TurnTimeoutSec      int  // 0 = 用 service 默认
    FirstByteTimeoutSec int
    ProgressMode        string // off | text | steps
    ProgressQuietSec    int
    HeartbeatMessage    string
    ExecutionMode       string // sync | async | auto (P2)
}
```

解析函数放 `internal/biz/channel_config_helpers.go`；Ingress 读取后注入 `context` 或 `SendChatMessageRequest` metadata（**不**写入 proto 对外字段，除非后续 Admin API 需要列表 Job）。

### 12.7 Accept 路径（P0）

**Webhook**（`channel_ingress_http.go`）：

1. `shouldProcessInbound` / `checkInboundAccess`（不变）  
2. `sendAckIfConfigured` → `StreamSender` 首条或 `enqueueOutboundReply`  
3. `w.WriteHeader(200)`  
4. `safego.Go` → `executeInboundTurn(context.WithoutCancel(...))`

**WebSocket**（`lark/ws_inbound.go`）：在现有 `HandleWSInbound` 内先 ACK 再 `ProcessInbound`；最终统一调用 `executeInboundTurn`。

**幂等**：Accept 阶段已 `TryClaimInbound`；Execute 阶段 Job `idempotency_key` 唯一；重复 Execute  no-op。

### 12.8 Execute 路径与 ChatService 集成

**Turn 结果分类**（`executeInboundTurn` 需识别）：

| `RunNativeTurn` 结果 | Channel 行为 |
|---------------------|--------------|
| 正常 assistant 文本 | 流式 flush 或 unary enqueue |
| `EnqueueUserMessage` queued | 出站 `ack_on_queued`；Job=`completed`（受理完成，非 Turn 完成） |
| `EnqueueUserMessage` 拒绝 | 出站拒绝文案；Job=`failed` |
| Turn 错误 / 超时 | `notifyFeishuInboundError` + Job 终态 |

**Channel 级 timeout**：在 `executeInboundTurn` 用 `context.WithTimeout` 包裹；**若 parent 已有更短 deadline 则尊重 parent**（与 `trpc_turn.go` 现有逻辑一致）。

**入队检测**：扩展 `runChatTurn` / `runChatTurnStreaming` 返回 `TurnOutcome`（`biz` 或 `service` 小 struct），避免通过「空 reply」猜测。

### 12.9 IM Preview 投影（TurnPreviewCoordinator，2026-05-23）

> **替代** 原 `ChannelProgressProjector` 与 `OnReplyDelta` 双轨 PATCH；进度语义并入 transcript。

#### 12.9.1 现状 vs 目标

```mermaid
flowchart TB
  subgraph before["改造前"]
    TRPC1[trpc event]
    EP1[EventProjector]
    EB1[(EventBus)]
    OD[OnReplyDelta 仅正文]
    PR[ProgressProjector 覆盖]
    SS1[StreamSender]
    DQ[Delivery ACK 独立消息]
    TRPC1 --> EP1 --> EB1
    TRPC1 --> OD --> SS1
    EB1 --> PR --> SS1
    DQ --> IM1[飞书消息1 ACK]
    SS1 --> IM2[飞书消息2 流式]
  end

  subgraph after["改造后"]
    TRPC2[trpc event]
    EP2[EventProjector]
    EB2[(EventBus)]
    TPC[TurnPreviewCoordinator]
    SS2[StreamSender]
    TRPC2 --> EP2 --> EB2 --> TPC --> SS2
    SS2 --> IM[飞书单条 preview 演进]
  end
```

#### 12.9.2 分层职责（SRP）

| 包 | 类型 | 职责 |
|----|------|------|
| `internal/biz` | `ChannelIMRenderPolicy` | 配置解析、legacy `progress_mode` 映射 |
| `internal/channel/preview` | `Transcript` / `RenderPlainText` | Envelope → 有序 segment → 纯文本 |
| `internal/service` | `TurnPreviewCoordinator` | 订阅 EventBus、节流 PATCH、心跳、写 `preview_message_id` |
| `internal/channel/lark` 等 | `StreamSender` | 平台 HTTP send/patch（无业务语义） |

**禁止**：在 `internal/agent` 或 `trpc_turn` 内嵌 IM 格式逻辑；Channel 仅订阅已投影的 Envelope。

#### 12.9.3 端到端序列

```mermaid
sequenceDiagram
  participant IM as 飞书用户
  participant IG as ChannelIngress
  participant TPC as TurnPreviewCoordinator
  participant CS as ChatService
  participant EP as EventProjector
  participant EB as EventBus
  participant SS as StreamSender

  IM->>IG: 入站
  Note over IG: streaming 时 Accept 不发独立 ACK
  IG->>TPC: Start(session_id)
  TPC->>SS: Update(ack_message) 首条 preview
  IG->>CS: RunNativeTurnUnary
  CS->>EP: trpc events
  EP->>EB: text_delta / tool_call / tool_result
  EB->>TPC: Apply + Render + PATCH
  CS-->>IG: Turn 完成
  TPC->>SS: Flush(final)
  IG->>IG: Job.preview_message_id
```

#### 12.9.4 Envelope → Transcript 映射

| Envelope | Segment |
|----------|---------|
| `text_delta` / `text_done` | `text` / `reasoning`（可配置展示） |
| `tool_call` / `tool_result` | `tool`（DisplayLabel / ActivityKind / Summary） |
| `member_*` | `member`（`im_team_mode`） |

#### 12.9.5 配置项（`config_json.config`）

| 字段 | 默认 | 说明 |
|------|------|------|
| `im_render_mode` | `reply_only` | `reply_only` \| `transcript` \| `transcript_with_reasoning` |
| `im_show_reasoning` | `false` | 思考链展示 |
| `im_tool_detail` | `label_summary` | `off` \| `label` \| `label_summary` |
| `im_team_mode` | `off` | `off` \| `inline` \| `steps` |
| `im_max_preview_chars` | `4000` | 渲染截断（尾部优先） |
| `im_tool_card_mode` | `off` | `off` \| `feishu_append`（工具**终态**时追加 Interactive Card） |
| `im_split_overflow` | `false` | Turn 结束超出平台上限时首条 PATCH + 分页 enqueue |
| `progress_mode` | — | **deprecated**；映射到 `im_render_mode` / `im_team_mode` |

`metadata.web_app_origin`：Web 管理端 origin，供 IM Card「Web 详情」深链；未配置时回退 `public_webhook_origin`（`biz.ResolveChannelWebOrigin`）。

`streaming_enabled=true` 且平台支持流式时，`ack_message` 由 Coordinator 写入首条 preview（`ChannelACKDeferredToPreview`）。

**飞书 Tool Card（`im_tool_card_mode=feishu_append`）— 单行模板**

| 元素 | 规则 |
|------|------|
| 布局 | 单行 `column_set`：左侧 lark_md 信息带 · 右侧 **Web 详情** 按钮 |
| 类型辨识 | MCP=📡 · Skill=⚡ · Agent=🤖 · 知识库=📚 · 记忆=🧠 · 工具=🔧（加粗类型标签） |
| 状态 | 完成 `<font color='green'>✓</font>` · 进行中 `<font color='orange'>🔄</font>`（**仅 preview PATCH**）· 失败 `<font color='red'>✕</font>` |
| Header | 成功=green / 进行中=orange / 失败=red；标题 `{emoji} {类型} · {label}` |
| Web 详情 | `{web_app_origin}/sessions/{session_id}?focus=tool&tool_id={id}`（回退 webhook origin） |
| 生命周期 | `tool_call` POST 进行中 Card → 同 `message_id` PATCH 终态；每工具 1 条 IM 消息 |
| Preview 文本 | 同策略下工具行与 Card 语义一致的单行压缩 |
| 心跳 | 静默期 PATCH `RenderPlainText(transcript) + "\n\n" + heartbeat`（与 EventBus 消费同 select 循环） |
| Card 异步 | Card HTTP 在 `safego` + `cardSerial` 队列，不阻塞 EventBus |

**Card 可观测性**：发送失败写 FlowLog `channel.tool.card`；指标 `aranea_channel_tool_card_total{platform,status}`。

#### 12.9.6 测试锚点

| 文件 | 覆盖 |
|------|------|
| `biz/channel_im_render_test.go` | Policy 解析、ACK defer |
| `biz/channel_web_origin_test.go` | `web_app_origin` 优先 |
| `channel/preview/transcript_test.go` | 顺序：正文→工具→正文 |
| `channel/lark/interactive_card_test.go` | Card HTTP 契约 |
| `service/channel_turn_preview_test.go` | Coordinator + 心跳保留 transcript |
| `service/channel_turn_preview_scenario_test.go` | ACK defer、overflow、终态 Card |
| `service/channel_ingress_accept_test.go` | streaming 时 ACK defer |

---

### 12.10 进度投影器（已合并，deprecated）

原 `ChannelProgressProjector` 已并入 `TurnPreviewCoordinator`。`progress_mode=text|steps` 仅作 legacy 映射，新配置请使用 `im_render_mode` / `im_team_mode`。

### 12.11 超长任务 async 模式（P2）

`execution_mode=async|auto` 时 Accept 路径：

1. 解析路由 → Agent/Team/Graph 触发器（`auto` 可经意图或命令前缀 `/async`）  
2. 创建 `GraphExecution` 或 `CronTask`（复用 `internal/biz` + 既有 service）  
3. IM 立即回复 `task_id` + Web Session 链接  
4. 订阅 `graph.execution.completed` / Cron 完成事件 → `enqueueOutboundReply`  

**影响域**：Channel Ingress、`GraphService`/`CronService` 触发 API、EventBus 消费者；**不在** Channel 内实现 Graph 节点逻辑。

### 12.12 影响域矩阵

| 变更 | 影响模块 | 风险 |
|------|----------|------|
| Webhook 异步 | 飞书/钉钉等 webhook 入站 | 低；WS 已异步 |
| ACK 文案 | 全平台 outbound | 低；可配置关闭 |
| TurnOutcome | `ChatService` 返回契约 | 中；需保持 Web Chat RPC 兼容（Web 忽略 queued outbound） |
| Channel timeout ctx | `trpc_turn` | 中；仅 Channel 注入 ctx 时生效 |
| channel_turn_job 表 | data/migration | 中 |
| ProgressProjector | event 订阅顺序 | 中；需防 goroutine 泄漏，Turn 结束 unregister |
| async → Graph | Graph/Cron/Event | 高；P2 独立迭代 |

### 12.13 指标与 FlowLog

| 指标 | 标签 |
|------|------|
| `aranea_channel_turn_job_total` | channel_id, status |
| `aranea_channel_turn_duration_seconds` | platform, owner_type |
| `aranea_channel_ack_total` | platform, result |

FlowLog 步骤（注册表追加）：

| step | 说明 |
|------|------|
| `channel.inbound.accept` | 受理 + ACK |
| `channel.turn.start` / `channel.turn.done` | Job 执行 |
| `channel.preview.patch` | IM preview PATCH（debug 级；原 channel.progress.patch） |

### 12.14 测试策略

| 层级 | 内容 |
|------|------|
| `biz/channel_config_helpers_test.go` | 长任务配置解析默认值 |
| `service/channel_ingress_accept_test.go` | Webhook 200 先于 Turn 完成 |
| `service/channel_ingress_execute_test.go` | queued / timeout / ACK |
| `service/channel_turn_preview_test.go` | Coordinator 节流 + transcript PATCH |
| `service/channel_ingress_accept_test.go` | streaming ACK defer |
| 集成 | mock StreamSender + fake ChatService 延迟 Turn |

---

## 十四、Hermes Agent 对照：消息流转与飞书特殊处理

> **外部参考**：[Hermes Agent v0.14](https://github.com/NousResearch/hermes-agent) — `gateway/run.py`、`gateway/platforms/feishu.py`、`gateway/session.py`  
> **Aranea 锚点**：`internal/service/channel_ingress*.go`、`internal/channel/lark/`、`TurnPreviewCoordinator`  
> **开发 backlog**：[17-channel-development.md §11 Phase F](./17-channel-development.md#11-phase-f--hermes-飞书借鉴p1p2)

### 14.1 产品形态差异（为何对照 Hermes）

| 维度 | Hermes | Aranea |
|------|--------|--------|
| 定位 | 单用户 Gateway + 多 Profile | 企业 Admin + DB 多 Channel 实例 |
| Agent 调用 | `AIAgent.run_conversation()` 单体 | `ChatService.RunNativeTurnUnary` + Kratos 分层 |
| 会话 | `SessionStore` + SQLite FTS5 | `channel_peer_session` + Session 实体 |
| 出站 | Adapter 直发 + 可选 `GatewayStreamConsumer` | `channel_delivery` worker + `StreamSender` PATCH |
| 配置 | 单文件 `config.yaml` + Env | `channel.config_json` + `channel_credential` |

Hermes 飞书适配器是 **生产级个人 Gateway 实现**，在文本分批、线程回复、Reaction 代替 typing、WS 重连参数等方面有可借鉴细节；Aranea 在 **IM Preview、Turn Job、Tool Card、长任务 async** 上已超越 Hermes 企业场景需求。

### 14.2 Hermes 端到端消息流转（Channel → Agent → 回复）

```mermaid
sequenceDiagram
  participant FS as 飞书 IM
  participant FA as FeishuAdapter
  participant Base as BasePlatformAdapter
  participant GW as GatewayRunner
  participant SS as SessionStore
  participant AG as AIAgent
  participant SC as GatewayStreamConsumer

  FS->>FA: im.message.receive_v1 (WS/Webhook)
  FA->>FA: dedup · admit · extract · batch
  FA->>Base: handle_message (per-chat Lock)
  alt session active
    Base->>GW: interrupt / queue pending
  else new session
    Base->>GW: _process_message_background
  end
  GW->>GW: auth · pairing · slash · clarify
  GW->>SS: get_or_create_session
  GW->>AG: _run_agent(transcript)
  opt streaming
    AG->>SC: on_delta
    SC->>FA: edit_message (PATCH)
  end
  AG-->>GW: response text
  GW->>FA: send / _send_with_retry
  FA->>FS: im.v1.message.create / reply
```

**Hermes 关键阶段（文件锚点）**

| 阶段 | 路径 | 要点 |
|------|------|------|
| 传输 | `gateway/platforms/feishu.py` | WS 默认（`lark-oapi`）；Webhook 可选 |
| 入站解析 | 同上 `_process_inbound_message` | text/image/audio/post/card；媒体下载 |
| 分批 | `_enqueue_text_batch` / `_flush_text_batch` | 0.6s debounce；≥4000 字等 2s 合并客户端拆条 |
| 串行 | `_handle_message_with_guards` | **每 chat_id 一把 asyncio.Lock** |
| 活跃会话 | `gateway/platforms/base.py` `handle_message` | interrupt / queue / bypass slash |
| Gateway | `gateway/run.py` `_handle_message` → `_handle_message_with_agent` | pairing、slash ACL、approval |
| 会话键 | `gateway/session.py` `build_session_key` | union_id 优先；thread 可选隔离 |
| 流式 | `gateway/stream_consumer.py` + `FeishuAdapter.edit_message` | 首条 create → 后续 update |
| 出站 | `FeishuAdapter.send` | 8000 字分片；markdown→post |

### 14.3 Aranea 端到端消息流转（对照）

```mermaid
sequenceDiagram
  participant FS as 飞书 IM
  participant WS as lark.RunWebSocket
  participant IG as ChannelIngress
  participant Peer as channel_peer_session
  participant CS as ChatService
  participant TPC as TurnPreviewCoordinator
  participant SS as StreamSender
  participant DW as DeliveryWorker

  FS->>WS: P2MessageReceiveV1
  WS->>WS: AcceptFeishuInbound · safego
  WS->>IG: ProcessInbound
  IG->>IG: shouldProcessInbound · acceptInbound
  IG->>Peer: ensureChannelSession
  alt streaming_enabled
    IG->>TPC: Start · EventBus subscribe
    IG->>CS: RunNativeTurnUnary
    CS->>TPC: text_delta / tool_*
    TPC->>SS: Update PATCH (2s throttle)
    TPC->>TPC: maybeHeartbeat (progress_quiet_sec)
    TPC->>SS: Flush final
  else unary
    IG->>CS: RunNativeTurnUnary
    CS-->>IG: reply
    IG->>DW: enqueueOutboundReply
    DW->>FS: SendTextMessage
  end
```

**Aranea 关键阶段（文件锚点）**

| 阶段 | 路径 | 要点 |
|------|------|------|
| WS 连接 | `internal/channel/lark/ws.go` `RunWebSocket` | `larkws.NewClient`；card action 同连接 |
| 入站门控 | `lark/inbound_gate.go` `AcceptFeishuInbound` | **仅 text**；user；群 @ |
| 幂等 | `biz/channel_inbound_receipt.go` | `feishu:{message_id}` DB receipt |
| 受理 | `service/channel_ingress_accept.go` | ACK 策略；async/sync；长任务路由 |
| 执行 | `service/channel_ingress_execute.go` | Turn Job；timeout ctx |
| IM Preview | `service/channel_turn_preview.go` | Transcript + 心跳 + Tool Card |
| 流式 | `lark/stream_outbound.go` `StreamSender` | POST 首条 → PATCH；tenant token 缓存 |
| 重连 | `channel/runtime/supervisor.go` | 指数退避 1s→5m（**应用层**） |

### 14.4 飞书特殊处理对照

#### 14.4.1 连接层：WS 心跳 / 重连 / 单 App 锁

| 机制 | Hermes | Aranea | 建议 |
|------|--------|--------|------|
| WS Ping/Pong | `lark-oapi` SDK 内建；可配 `ws_ping_interval` / `ws_ping_timeout` | 依赖 `larkws.Client.Start`；**未暴露 ping 参数** | **F-01**：`RunWebSocket` 增加可选 config：`ws_ping_interval_sec`、`ws_reconnect_interval_sec`（对齐 Hermes 默认 120s） |
| 启动重连 | `_connect_with_retry` 3 次指数退避 | `runSupervised` 断线后无限重连 | Aranea 已覆盖；补充 **connected 时长** 指标 |
| 同 App 双连 | `acquire_scoped_lock("feishu-app-id")` 防两 Gateway 抢 WS | fingerprint reconcile 防双实例；**无跨进程 app_id 锁** | **F-02**：文档强调「一 app_id 仅一 enabled channel」；可选启动时检测冲突 |
| Webhook 限流 | 120 req/min/IP + 1MB body | 验签 + 200 快返；**无 IP 限流** | **F-03**：Kratos middleware 或 ingress 层 per-channel_key 限流 |
| WS/Webhook 双收 | 模式互斥（config） | WS 模式忽略 webhook IM 事件 | 一致；UI 需明确互斥说明 ✅ |

#### 14.4.2 入站：去重、分批、富媒体

| 机制 | Hermes | Aranea | 建议 |
|------|--------|--------|------|
| message_id 去重 | 持久化 JSON LRU 24h + 内存 | DB `channel_inbound_receipt` | Aranea 更可靠 ✅ |
| 客户端拆条合并 | 0.6s debounce；大段 +2s | **无** | **F-04**：`AcceptFeishuInbound` 后增加 per-peer 文本 debounce（500–800ms），合并连续 text 再 Turn |
| 富媒体 | image/audio/post/merge-forward 下载+STT | **仅 text**，其余丢弃 | **F-05** P2：post 转 plain；image 走 multimodal（对齐 Chat 附件） |
| 群 @ | adapter `_admit` + require_mention | `AcceptFeishuInbound` + `checkInboundAccess` | 双层合理 ✅ |
| Card 入站 | 按钮 → synthetic `/card` COMMAND | `card.action.trigger` → background/cancel | Aranea 卡片动作更贴近 M55 ✅ |

#### 14.4.3 会话隔离（Peer / Thread）

| 场景 | Hermes `build_session_key` | Aranea `PeerKeyForSession` |
|------|---------------------------|----------------------------|
| DM | `feishu:dm:{chat_id}` | `peer_key` = open_id / user_id |
| 群 per-user | `feishu:group:{chat_id}:{union_id}` | `dm_scope=per-channel-peer` |
| 群共享 | `feishu:group:{chat_id}` | `dm_scope=main`（单 session） |
| Topic 线程 | `{chat_id}:{thread_id}` 可选 | `OutboundMeta.thread_id` + `thread_sessions_per_user` | ✅ F-06 |

Hermes 优先 **union_id** 作用户隔离；Aranea 当前 PeerID 顺序为 open_id → user_id → chat_id，群聊场景建议文档化 **union_id** 字段采集（若 SDK 事件含 union_id）。

#### 14.4.4 出站：流式、分片、线程回复

| 机制 | Hermes | Aranea | 建议 |
|------|--------|--------|------|
| 流式 | `edit_message` 逐 token | `StreamSender` PATCH + 2s 节流 | 相当；Aranea 有 Transcript ✅ |
| 首条 ACK | 独立消息或 stream 首条 | Coordinator 首条 preview 含 ACK（`ChannelACKDeferredToPreview`） | Aranea 更优（单条演进）✅ |
| 静默心跳 | **无** preview 级心跳 | `TurnPreviewCoordinator.maybeHeartbeat` | Aranea 更优 ✅ |
| 长度分片 | 8000 字多段 **send** | `im_split_overflow` Turn 结束分页 enqueue | **F-07**：流式 PATCH 达 12000 rune 上限时 Hermes 式主动 split 新消息 |
| 线程回复 | `reply_in_thread` + `thread_id` receive | `ResolveReceiveTarget` chat_id 为主 | **F-06** 同上 |
| Markdown | post 类型；table→plain | `RenderPlainText` 纯文本 | **F-08** P2：Hermes 式 post 出站（保留粗体/链接） |

#### 14.4.5 「Processing」反馈（typing / reaction）

| 机制 | Hermes | Aranea |
|------|--------|--------|
| Typing API | **不支持**（no-op） | 无 |
| 处理中反馈 | message **Reaction** `Typing`；失败 `CrossMark` | preview ACK + 心跳文案 + Tool Card 🔄 |
| 配置 | `FEISHU_REACTIONS` env | 无 reaction |

**建议 F-09（P2）**：长 Turn 首字节前（`first_byte_timeout_sec` 内无 delta）对入站 `message_id` 添加飞书 emoji reaction（如 THUMBSUP/Typing 等价物），Turn 结束移除 — 补充「用户尚未看到 preview 前」的即时反馈。需评估 API 权限与频率。

#### 14.4.6 并发与中断

| 机制 | Hermes | Aranea |
|------|--------|--------|
| 同 session 新消息 | **interrupt** 默认：cancel + pending queue | `RunNativeTurnUnary` → queued + `ack_on_queued` |
| 模式 | interrupt / queue / steer 可配 | queued 为主；cancel via `/cancel` |
| per-chat 锁 | asyncio.Lock | Session `HasActiveRun` |

**建议 F-10（P2）**：Channel 配置 `busy_input_mode: queue | interrupt`（Hermes 对齐）；interrupt 时调用 `ChatService.StopGeneration` + 合并 pending 文本。

#### 14.4.7 Token 与凭据

| 机制 | Hermes | Aranea |
|------|--------|--------|
| tenant_access_token | `lark.Client` 内建刷新 | `StreamSender.tenantTokenLocked` 30s skew 缓存 |
| Webhook 验签 | verification_token + encrypt_key HMAC | `VerifyHTTPRequest` encrypt_key |
| 加密事件体 | Webhook 模式拒绝 encrypted payload | **未实现 AES 解密** |

**建议 F-11（P1）**：飞书开放平台「加密配置」开启时，Webhook 路径增加 `decrypt_event`（与 MuseBot/Hermes 一致），避免生产只能关加密。

### 14.5 能力矩阵：Hermes vs Aranea（飞书 Channel）

| 能力 | Hermes | Aranea | 优势方 |
|------|--------|--------|--------|
| IM Preview 单条演进 | 流式 edit 无 transcript | Transcript + Tool Card + 心跳 | Aranea |
| Turn Job 审计 | 无 | `channel_turn_job` + Admin API | Aranea |
| 长任务 async Graph | 无 | `execution_mode=async` | Aranea |
| 文本入站分批 | ✅ debounce | ❌ | Hermes |
| Reaction 处理中 | ✅ | ❌ | Hermes |
| 富媒体入站 | ✅ | ❌ text only | Hermes |
| Thread/话题隔离 | ✅ | ❌ | Hermes |
| 8000 字主动分片 | ✅ | 溢出配置可选 | Hermes |
| DM pairing | ✅ 8 位码 | allowlist | 各适用场景 |
| Exec approval 卡片 | ✅ 四键 | Tool confirm + escalate card | 各有侧重 |
| Webhook IP 限流 | ✅ | ❌ | Hermes |
| 同 app 跨进程锁 | ✅ | 部分 | Hermes |

### 14.6 建议优先级（摘要）

| ID | 优先级 | 建议 | 落点 |
|----|--------|------|------|
| F-01 | P1 | WS ping/reconnect 可配置 + 连接状态暴露 Admin | `lark/ws.go` · `ChannelsTable` chip |
| F-03 | P1 | Webhook 入站 rate limit | `channel_ingress_http.go` |
| F-04 | P1 | 飞书连续 text debounce 合并 | `lark/inbound_batch.go`（新） |
| F-06 | P1 | thread_id 会话隔离 + 线程回复 | `inbound_build.go` · routing config |
| F-07 | P1 | 流式 PATCH 达平台上限主动 split | `stream_outbound.go` · preview |
| F-11 | P1 | Webhook 加密事件体解密 | `lark/webhook.go` |
| F-02 | P2 | 同 app_id 多 channel 冲突检测 | `runtime/manager.go` |
| F-05 | P2 | post/图片入站 | `parse_message.go` · Chat 附件 |
| F-08 | P2 | post 类型出站 | `feishu_outbound.go` |
| F-09 | P2 | Reaction 处理中反馈 | `lark/reaction.go`（新） |
| F-10 | P2 | `busy_input_mode` interrupt | `channel_config_helpers.go` · ingress |

详细任务板见 [17-channel-development.md §11](./17-channel-development.md#11-phase-f--hermes-飞书借鉴p1p2)。

---

## 十三、文档修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| 1.0 | 2026-05-22 | §十二 长任务异步执行：Accept/Execute 分离、Job 模型、ProgressProjector、影响域 |
| 1.1 | 2026-05-24 | §十四 Hermes Agent 对照：消息流转、飞书 WS/心跳/分批/Reaction、借鉴项 F-01–F-11 |
| 1.2 | 2026-06-06 | §一 实现状态更新：13 平台全部 bundled ✅；§二 Runtime 补 mattermost；§六 平台矩阵 10→13 行，状态全部 ✅；§九 ChannelRuntimeManager ✅；§十一 优先级全部完成 |


---

## 子模块：Channel  Agent  Team  会话消息  集成设计

> 对应业务说明：[17-channel-agent-team-integration.md](./17-channel-agent-team-integration.md)  
> **目标态架构（1 Turn + 2 Projections + 3 Anchors）**：[55-chat-channel-cursor-solution.md §1.5](../需求/55-chat-channel-cursor-solution.md#15-架构收敛模型1-turn--2-projections--3-anchors)  
> 遵循：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · [AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)  
> **运行时边界**：Channel 与 Chat 均在 `internal/service` 组装 `trpc-agent-go`；`internal/biz` 不 import 框架。

---

## 一、模块职责矩阵（实现视角）

| 层 | 包/锚点 | 职责 | 禁止 |
|----|---------|------|------|
| 接入 | `internal/channel/lark/*`、`channel_ingress*.go` | 验签、解析 `InboundEvent`、出站/流式 | 调用 LLM、解析 Agent 配置 |
| 路由 | `internal/biz/channel_routing.go` | `ParseChannelRouting`、`ResolveChannelTarget`、`PeerKeyForSession` | 创建 Runner |
| 会话绑定 | `channel_peer_session` + `channel_ingress_session.go` | peer → `session_id` 稳定映射 | 修改已存在 Session 的 owner |
| 运行时 | `ChatService.RunNativeTurn*`、`chat_native.go` | Agent Turn / Team Turn、落库、EventBus | — |
| 编排 | `internal/team` | Team 定义 → trpc Team Runner | — |
| 能力 | `internal/agent` + `internal/biz/agent_*` | `BuildTRPCLLMAgent`、工具/记忆装配 | — |
| 消息 | `internal/biz/session_usecase`、Event 投影 | `ChatMessage`、Envelope | Channel 专用消息表（无，复用 Session） |
| 实时 | `internal/event` + `/v1/ws` | Envelope 多路复用 | Channel 独立 Broker |

---

## 二、端到端数据流（飞书 WS）

```
larkws / webhook
  → port.InboundEvent { PeerID, Text, OutboundMeta, ... }
  → ChannelIngress.ProcessInbound
       ├ channel_ingress_access.go   (allowed_* / require_mention)
       ├ ParseChannelRouting
       ├ PeerKeyForSession(dm_scope, peer_id)
       ├ ensureChannelSession → sessions.Create(owner_type, agent_id|team_id)
       ├ prepareChannelChatRequest → SendChatMessageRequest{ session_id, content, team_id? }
       └ runChatTurn / runChatTurnStreaming
            → ChatService.RunNativeTurnUnary|Streaming
                 ├ owner_type=agent → runSingleAgentViaTRPC
                 └ owner_type=team  → teamsNative.RunTurn
  → assistant ContentMarkdown
  → enqueueOutboundReply | StreamSender.Update (streaming_enabled)
```

**关键代码锚点**：

```62:88:internal/service/channel_ingress_session.go
func (h *ChannelIngress) prepareChannelChatRequest(...) (*chatv1.SendChatMessageRequest, error) {
	// ParseChannelRouting → ensureChannelSession → SendChatMessageRequest
	// team 会话时设置 req.TeamId
}
```

```85:113:internal/biz/channel_routing.go
func ResolveChannelTarget(...) (ownerType, agentID, teamID string, err error) {
	// rules → default_team_id → default_agent_id (UUID 或 agent_key)
}
```

```218:251:internal/service/chat_native.go
	if strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team") {
		// teamsNative.RunTurn — 与 Web Chat Team 会话同路径
	}
```

---

## 三、绑定模型

### 3.1 三张表语义

| 表 | 键 | 值 | 说明 |
|----|----|----|------|
| `channel` | `id` / `key` | `config_json`（含 routing、access、streaming） | 一个飞书应用实例 |
| `channel_peer_session` | `(channel_id, peer_key)` | `session_id` | IM 对端与会话 1:1（在 dm_scope 语义下） |
| `sessions` | `id` | `owner_type`, `agent_id`, `team_id` | 会话归属**创建时**写入 |

### 3.2 `dm_scope` 与 `peer_key`

| dm_scope | peer_key | 业务含义 |
|----------|----------|----------|
| `main` | `""` | 全 Channel 共用一个 Session（适合全群广播助手） |
| `per-channel-peer`（默认） | `peer_id` | 每个 IM 对端独立 Session（私聊用户 / 群 ID 各一会话） |
| `per-peer` | `peer_id` | MVP 与 per-channel-peer 相同 |

飞书 WS 入站：`PeerID` 多为 `open_id`；群场景 `OutboundMeta.chat_id` 用于群白名单与出站 recipient。

### 3.3 路由变更策略（现状 vs 建议）

| 策略 | 现状 | 建议（产品+实现） |
|------|------|-------------------|
| 修改 `default_agent_id` | 已存在 `channel_peer_session` **不变** | 文档明确；可选提供「重置 peer 绑定」运维操作 |
| 修改 `default_team_id` | 同上 | 同上 |
| 修改 `rules` | 仅影响**新 peer 模式**下未建绑定的入站 | 规则变更审计日志 |
| Agent 删除 | `ResolveChannelTarget` NotFound | Channel 保存时校验 + 列表展示引用 |

---

## 四、Team 与消息机制在 Channel 路径上的行为

### 4.1 Team Turn

- Channel 准备的 Session 若 `owner_type=team`，`runNativeAgentTurn` 走 `teamsNative.RunTurn`，与 Web Chat 选择 Team 一致。
- `team_runs` / `team_run_steps` 正常写入；`EventProjector` 仍会向 `EventBus` 发布 `member_delta` 等。
- **IM 出站**：`processInboundCore` 仅取 `assistantMsg.ContentMarkdown` 全文；**不**把成员条投影到飞书（除非后续做「卡片分段」产品）。

### 4.2 单 Agent Turn

- `sess.AgentID` 驱动 `hydratedAgent` → `BuildTRPCLLMAgent` → `runSingleAgentViaTRPC`。
- 工具调用、FlowLog 仍产生 Envelope；飞书用户默认只见文本（或流式 PATCH 的文本）。

### 4.3 与 [51 消息机制](./51%20消息机制.md) 的关系

```mermaid
flowchart LR
  subgraph ChannelPath["Channel 入站"]
    IN[InboundEvent]
    NT[RunNativeTurn]
  end
  subgraph Runtime["共享运行时"]
    TRPC[trpc event.Event]
    EP[EventProjector]
    EB[EventBus]
    DB[(Session Messages)]
  end
  subgraph Clients["消费者"]
    WS[WebSocket /v1/ws]
    IM[飞书 outbound]
  end
  IN --> NT
  NT --> TRPC
  TRPC --> EP
  EP --> EB
  EP --> DB
  NT --> IM
  EB --> WS
```

- **真相源**：trpc `event.Event` → 投影 → Envelope + DB 消息。
- **Channel 额外约束**：出站适配器只消费**文本聚合结果**（流式时由 `OnReplyDelta` 推平台）。
- **可观测**：运维应用 `session_id`（来自 `channel_peer_session`）在 Monitor / Session 页订阅 WS，与 Web Chat 共用 Envelope 类型。

### 4.4 长任务路径

> **需求**：[17 channel.md §8](./17%20channel.md#8-长任务场景飞书-channel) · **设计**：[17 channel.design.md §十二](./17%20channel.design.md#十二长任务异步执行设计)

**现状问题**（2026-05-22）：

```
Webhook: processInboundHTTP 同步 ProcessInbound → 阻塞至 Turn 结束
Channel: runChatTurn → RunNativeTurnUnary → 最长 5min
入队:    hasActive → EnqueueUserMessage → reply="" → 无 IM 出站
Team:    teamsNative.RunTurn 数分钟 → 仅最终 ContentMarkdown 回飞书
```

**目标数据流（Phase E）**：

```mermaid
sequenceDiagram
  participant IM as 飞书
  participant IG as ChannelIngress
  participant JOB as channel_turn_job
  participant RT as ChatService
  participant EB as EventBus
  participant PR as ProgressProjector

  IM->>IG: 入站消息
  IG->>IG: acceptInbound（幂等+ACK）
  IG->>JOB: status=accepted
  IG-->>IM: ACK 文案
  Note over IG,IM: Webhook HTTP 200

  IG->>RT: executeInboundTurn（async）
  IG->>JOB: status=running
  RT->>EB: tool/member/delta 事件
  EB->>PR: 进度 PATCH
  PR->>IM: 流式/进度消息
  RT->>IG: assistant 完成
  IG->>JOB: status=completed
  IG->>IM: flush 最终文本
```

**与 Team / Chat 边界**：

| 层 | 长任务职责 |
|----|------------|
| `ChatService` | Session 锁、Turn 超时（可被 Channel ctx 覆盖）、pending queue |
| `teamsNative.RunTurn` | 成员编排、落库、`member_*` Envelope（不变） |
| `ChannelIngress` | ACK、Job、IM 投影；**不**解析 Team 图 |
| `ChannelProgressProjector` | 消费 Envelope → 飞书 PATCH；Web Chat UI 不变 |

**TurnOutcome 契约**（service 层，Web RPC 兼容）：

| Outcome | IM 动作 | Session 消息 |
|---------|---------|--------------|
| `completed` | flush 回复 | user + assistant 落库 |
| `queued` | `ack_on_queued` | 仅 user 入 pending |
| `failed` / `timeout` | 错误文案 | 按 Turn 错误策略 |

---

## 五、差距清单与实现建议

| ID | 项 | 状态 | 建议实现 |
|----|-----|------|----------|
| I-01 | 路由 UI：`dm_scope`、`rules` | 🟡 | `dm_scope` 下拉 ✅；`rules` 表 ⏳ |
| I-02 | 路由变更迁移 | ✅ | `UpdateChannel` 在 `RoutingTargetChanged` 时 `DeleteByChannelID` |
| I-03 | Agent 被 Channel 引用列表 | ✅ | `biz.ListChannelsReferencingAgent` + `AgentChannelRefsSection.vue` |
| I-08 | Web Chat 同步 Channel Turn | ✅ | `web/.../useChatInboundSync.ts` + `sessions.metadata_json.source=channel` |
| I-04 | Team 飞书卡片化中间步骤 | ❌ | 可选：`config_json.config.team_notify_mode=summary|steps` |
| I-05 | Channel Turn 与 WS 默认订阅 | 🟡 | 文档约定；可选在 Envelope 加 `source=channel` |
| I-06 | `ensureChannelSession` 忽略 routing 解析的 agent/team | 代码 smell | 创建 Session 时已用 `ResolveChannelTarget`；删除无用 `_ = ownerType` 或用于校验一致性 |
| I-07 | 并发入站 | 🟡 Session 锁 | 与 Web Chat 相同：`lockSession` + enqueue；飞书侧重试由平台保证 |
| I-09 | Webhook 同步阻塞 Turn | ❌ Phase E1 | `acceptInbound` + HTTP 200 后 async execute |
| I-10 | 入队无 IM 反馈 | ❌ Phase E1-4 | `TurnOutcome=queued` → `ack_on_queued` |
| I-11 | 长静默无进度 | ❌ Phase E4 | `ChannelProgressProjector` + `progress_mode` |
| I-12 | Turn Job 审计 | ❌ Phase E3 | `channel_turn_job` 表 |
| I-13 | Channel 级 Turn 超时 | ❌ Phase E2 | `turn_timeout_sec` / `first_byte_timeout_sec` |
| I-14 | 超长任务 async | ❌ Phase E6 | `execution_mode` → Graph/Cron |

---

## 六、推荐业务配置模板（飞书）

### 6.1 单 Agent 客服

```json
{
  "routing": {
    "default_agent_id": "customer-service",
    "dm_scope": "per-channel-peer"
  },
  "config": {
    "require_mention": true,
    "streaming_enabled": true
  }
}
```

### 6.2 Team 流水线（群聊）

```json
{
  "routing": {
    "default_team_id": "<team-uuid>",
    "dm_scope": "per-channel-peer"
  },
  "config": {
    "require_mention": true,
    "allowed_group_ids": ["oc_xxx"],
    "streaming_enabled": true,
    "turn_timeout_sec": 900,
    "progress_mode": "steps",
    "ack_message": "收到，Team 流水线已启动…"
  }
}
```

### 6.3 多群分流（规则）

```json
{
  "routing": {
    "default_agent_id": "main",
    "rules": [
      { "peer_pattern": "oc_sales_*", "team_id": "<sales-team-uuid>" },
      { "peer_pattern": "oc_support_*", "agent_id": "support-bot" }
    ]
  }
}
```

---

## 七、测试与验收映射

| 验收 ID | 测试建议 |
|---------|----------|
| CAT-01 | 集成：`ProcessInbound` + 同 peer 二次入站同 `session_id` |
| CAT-02 | `owner_type=team` + `team_runs` 计数 + 飞书 mock outbound |
| CAT-03 | 改 routing 后旧 peer session `agent_id` 不变 |
| CAT-04 | `channel_ingress_access_test.go` |
| CAT-05 | `streaming_enabled` + `stream_outbound` 假客户端 |
| LT-01 | mock 慢 Turn + ACK 时序断言 |
| LT-03 | active run + 第二条入站 → queued outbound |

---

## 八、文档修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| 1.0 | 2026-05-22 | 集成设计首版：数据流、绑定模型、Team/消息机制、差距表 |
| 1.1 | 2026-05-22 | §4.4 长任务路径；差距 I-09–I-14；Team 模板增长任务配置 |
