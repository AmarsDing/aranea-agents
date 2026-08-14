# Channel 渠道管理

本文档定义 Channel **对外平台连接** 的产品需求：多 IM/协作平台接入、凭据与连接模式配置、消息路由与投递验收。

**平台连接参考**：

- [MuseBot](https://github.com/yincongcyincong/MuseBot)（`robot/*.go` + `http/communicate.go`）— MIT；SDK 选型与连接模式  
- [Hermes Agent](https://github.com/NousResearch/hermes-agent)（`gateway/platforms/feishu.py` + `gateway/run.py`）— MIT；**飞书入站/outbound/流式/心跳/会话隔离** 等产品与连接层行为对照，见 [17 channel.design.md §十二](./17%20channel.design.md#十二hermes-agent-对照消息流转与飞书特殊处理)

Aranea **只借鉴连接层与 IM 体验模式**，Agent 运行时仍走 `internal/service.ChatService`，不在 Channel 层耦合 LLM。

与 **`2 agents-create.md`**（Agent 能力配置）、**`11 multi-agent.md`**（Team 编排）、**`10-session-development.md`**（会话）、**`51 消息机制.md`**（实时事件）、**`17-channel-agent-team-integration.md`**（跨模块业务主链）、**`frontend-pages.md`**（`/channels`）、**`docs/README.md`**（Kratos + trpc 分层）对齐。

> **跨模块说明**：Channel 不「拥有」Agent/Team，仅通过 `config_json.routing` 将 IM 对端映射到已存在的 Agent 或 Team，再经 `channel_peer_session` 绑定 Session。详见 [17-channel-agent-team-integration.md](./17-channel-agent-team-integration.md)。

> **架构与代码分层**：本文档仅描述用户视角的产品需求。代码分层、Proto/API 契约、数据模型、Adapter 接口、Service/Biz/Data 文件职责见 [17-channel.design.md](./17-channel.design.md)；开发进度、Phase 划分、任务清单、改动文件清单见 [17-channel.development.md](./17-channel.development.md)。

---

## 1. 模块定位

| 项目 | 说明 |
|------|------|
| **用户目标** | 配置外部消息平台连接，将用户消息路由到 Agent/Team，异步回发回复 |
| **路由** | `/channels`（列表 + 编辑弹窗） |
| **非目标** | Channel 内编排 Agent；存完整消息明文；全局单例配置（MuseBot 式 flat conf） |

Aranea 保留 MuseBot 验证过的 **SDK 与连接模式**，采用 Kratos 分层与 DB 多实例模型。架构差异对照（配置/启动/入站/出站/LLM 维度）见 [17-channel.design.md §一](./17-channel.design.md#一模块概述)。

---

## 2. 支持的平台（Catalog）

Catalog 由 `ListChannelTypes` 返回。

- **`bundled=true`**：本 binary 已实现 adapter，可创建并运行  
- **`bundled=false`**：规格已定义，连接能力待实现（参考 MuseBot 同平台文件）

| type | 标签 | MuseBot 参考 | 连接模式 | 状态 | 入站 | 出站 |
|------|------|-------------|----------|------|------|------|
| `feishu` | 飞书 / Lark | `robot/lark.go` | webhook · **websocket**（Catalog 默认仅开放 websocket） | ✅ | ✅ | ✅ |
| `dingtalk` | 钉钉 | `robot/ding.go` | webhook · **stream** | ✅ | ✅ | ✅ |
| `wecom` | 企微智能机器人 | `robot/comwechat.go` | webhook | ✅ | ✅ | ✅ |
| `wecom-app` | 企微自建应用 | `robot/comwechat.go` | webhook | ✅ | ✅ | ✅ |
| `wechat` | 微信公众号 | `robot/wechat.go` | webhook（被动/客服） | ✅ | ✅ | ✅ |
| `wechat_ilink` | 微信（个人号·iLink） | 腾讯 iLink 官方 Bot API（无 MuseBot 对应） | **polling**（扫码登录） | ✅ | ✅ | ✅（文本） |
| `slack` | Slack | `robot/slack.go` | event · **socket_mode** | ✅ | ✅ | ✅ |
| `telegram` | Telegram | `robot/telegram.go` | webhook · **polling** | ✅ | ✅ | ✅ |
| `discord` | Discord | `robot/discord.go` | **gateway** WebSocket | ✅ | ✅ | ✅ |
| `qq` | QQ 官方机器人 | `robot/qq.go` | webhook + WS 事件 | ✅ | ✅ | ✅ |
| `personal_qq` | QQ（OneBot） | `robot/personalqq.go` | OneBot HTTP 推送 | ✅ | ✅ | ✅ |
| `line` | LINE | — | webhook | ✅ | ✅ | ✅ |
| `mattermost` | Mattermost | — | webhook · **websocket** | ✅ | ✅ | ✅ |
| `teams` | Microsoft Teams | — | Bot Framework webhook | ✅ | ✅ | ✅ |

**SDK 对照**（实现时优先采用）：

| 平台 | Go 依赖 |
|------|---------|
| 飞书/Lark | `larksuite/oapi-sdk-go/v3` + `larkws` |
| 钉钉 | `open-dingtalk/dingtalk-stream-sdk-go` |
| 企微 / 微信 | `ArtisanCloud/PowerWeChat/v3` |
| Slack | `slack-go/slack` + `socketmode` |
| Telegram | `go-telegram-bot-api/v5` |
| Discord | `bwmarrin/discordgo` |
| QQ 官方 | `tencent-connect/botgo` |
| LINE | `line-bot-sdk-go`（仅类型引用，核心逻辑自研） |
| Mattermost | `gorilla/websocket` + REST API v4（无官方 SDK） |
| Teams | Bot Framework OAuth2 + REST API（无 Go SDK） |

---

## 3. 连接模式说明

| 模式 | 适用平台 | 说明 | 公网要求 |
|------|----------|------|----------|
| `webhook` | 飞书、企微、微信、Slack Events、Telegram、QQ、LINE、Mattermost、Teams | Kratos HTTP 回调；路径 `/webhooks/{channel_key}` 或平台专用路径 | 需 HTTPS |
| `websocket` | 飞书 Lark WS、Mattermost | 长连接收事件；MuseBot `larkws.Client.Start` | 出站即可 |
| `stream` | 钉钉 Stream | `dingtalk-stream-sdk-go`；替代传统机器人 Webhook | 出站即可 |
| `socket_mode` | Slack | App Token + Bot Token；MuseBot Socket Mode | 出站即可 |
| `polling` | Telegram、微信 iLink | Telegram `GetUpdatesChan`；iLink `getupdates` 长轮询 | 出站即可 |
| `gateway` | Discord | discordgo Gateway | 出站即可 |
| `onebot` | Personal QQ | 接收 OneBot POST；发送调 OneBot HTTP API | 视 NapCat 部署 |

`config_json.receive_mode` 取值：`webhook` | `websocket` | `stream` | `socket_mode` | `polling` | `gateway` | `onebot`。

---

## 4. 信息架构与 UI

### 4.1 页面（当前实现）

| 区域 | 组件 |
|------|------|
| 列表 | `ChannelsTable.vue` |
| 编辑 | `ChannelEditorDialog.vue` |
| 平台选择 | `ChannelCatalogPicker.vue`（`bundled` 可选，非 bundled 展示「即将支持」） |

### 4.2 编辑弹窗区块

1. 选择平台（新建）
2. 基础信息：`name`、`key`、`description`、`enabled`
3. **连接方式**：`receive_mode`（选项随 catalog `receive_modes`）
4. 凭据：按 `credential_schema`；留空不修改
5. 回调地址：webhook 类平台展示只读 URL + 复制
6. 路由：`routing.default_agent_id`（Team / rules / dm_scope UI 待补）

### 4.3 列表列

名称（Avatar）、平台、连接模式、外部 ID、启用、连接状态、最近更新、操作（编辑 / 删除 / 复制 Webhook）。

---

## 5. 各平台凭据与配置

字段写入 `config_json.config`（非敏感）与 `channel_credential`（敏感）。括号内为 MuseBot `conf.BaseConfInfo` 字段名。

### 5.1 飞书 / Lark（`feishu`）

| 字段 | 位置 | MuseBot 字段 |
|------|------|--------------|
| `app_id` | config | `lark_app_id` |
| `region` | config | — |
| `app_secret` | credential | `lark_app_secret` |
| `connection_mode` | config | webhook / websocket |
| `ws_ping_interval_sec` | config | （规划 F-01）larkws ping 间隔 |
| `ws_reconnect_interval_sec` | config | （规划 F-01）重连间隔，Hermes 默认 120s |
| `thread_sessions_per_user` | config | （规划 F-06）话题线程独立 session |
| `inbound_text_debounce_ms` | config | （规划 F-04）连续 text 合并，默认 600 |

### 5.2 钉钉（`dingtalk`）

| 字段 | 位置 | MuseBot 字段 |
|------|------|--------------|
| `client_id` | config | `ding_client_id` |
| `client_secret` | credential | `ding_client_secret` |
| `secret` | credential | 机器人加签（Webhook 模式） |

Stream 模式使用 `client_id` + `client_secret`；当前 Webhook 模式使用 `secret` 验签。

### 5.3 企微（`wecom` / `wecom-app`）

| 字段 | 位置 | MuseBot 字段 |
|------|------|--------------|
| `corp_id` | config | `com_wechat_corp_id` |
| `agent_id` | config | `com_wechat_agent_id` |
| `secret` | credential | `com_wechat_secret` |
| `token` | credential | `com_wechat_token` |
| `encoding_aes_key` | credential | `com_wechat_encoding_aes_key` |

### 5.4 微信公众号（`wechat`）

| 字段 | 位置 | MuseBot 字段 |
|------|------|--------------|
| `app_id` | config | `wechat_app_id` |
| `app_secret` | credential | `wechat_app_secret` |
| `token` | credential | `wechat_token` |
| `encoding_aes_key` | credential | `wechat_encoding_aes_key` |
| `active_mode` | config | `wechat_active`（被动回复 vs 客服 API） |

### 5.5 Slack（`slack`）

| 字段 | 位置 | MuseBot 字段 |
|------|------|--------------|
| `bot_token` | credential | `slack_bot_token` |
| `app_token` | credential | `slack_app_token`（Socket Mode） |
| `signing_secret` | credential | Events API 验签 |

### 5.6 Telegram（`telegram`）

| 字段 | 位置 | MuseBot 字段 |
|------|------|--------------|
| `bot_token` | credential | `telegram_bot_token` |

### 5.7 Discord（`discord`）

| 字段 | 位置 | MuseBot 字段 |
|------|------|--------------|
| `bot_token` | credential | `discord_bot_token` |

### 5.8 QQ 官方（`qq`）

| 字段 | 位置 | MuseBot 字段 |
|------|------|--------------|
| `app_id` | config | `qq_app_id` |
| `app_secret` | credential | `qq_app_secret` |

### 5.9 QQ OneBot（`personal_qq`）

| 字段 | 位置 | MuseBot 字段 |
|------|------|--------------|
| `receive_token` | credential | `qq_one_bot_receive_token` |
| `send_token` | credential | `qq_one_bot_send_token` |
| `onebot_base_url` | config | `qq_one_bot_http_server` |

### 5.10 微信个人号（`wechat_ilink`，2026-08-12）

腾讯 iLink 官方 Bot API（`ilinkai.weixin.qq.com`），扫码登录自动获取凭证，无需公网回调。

| 字段 | 位置 | 获取方式 |
|------|------|----------|
| `bot_token` | credential（必填） | **扫码登录自动写入**（推荐），或手动粘贴 |
| `baseurl` | credential（可选） | 扫码登录自动写入（自定义 API 域名时用手动） |
| `ilink_user_id` | credential（可选） | 扫码登录自动写入（typing 状态用） |

**扫码登录流程**（编辑弹窗「微信扫码登录」区块，仅已保存渠道可用）：

1. 点「获取登录二维码」→ `WechatILinkLogin` RPC 返回 iLink 原始 `qrcode_img_content`（扫码内容，实为 liteapp URL，非图片）并启动服务端后台轮询（3 分钟上限）；前端用 `qrcode` 包把该内容编码成 SVG data URL 渲染二维码
2. 微信扫码 + 手机确认 → 服务端写入 `bot_token` / `baseurl` / `ilink_user_id` 三个凭证并触发 runtime reload
3. 前端每 2s 轮询 `WechatILinkPoll`（凭证已写入即返回 `confirmed`），3 分钟超时提示过期可重新获取

**config 配置项**（`config_json.config`）：

| 字段 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `group_enabled` | bool | `false` | 是否处理群聊消息 |
| `require_mention` | bool | `true` | 群聊需 @ `bot_nickname` 才响应 |
| `bot_nickname` | string | 空 | Bot 在群内的昵称（mention 检测用）；留空且 `require_mention=true` 时全响应 |

**登录态持久化**：状态文件 `bin/data/channel-state/wechat_ilink-<channel_id>.json` 保存 `get_updates_buf` 游标、各用户 `context_token` 缓存与 `login_status`；进程重启后从游标续收，不丢消息。

---

## 6. 配置语义（`config_json`）

```json
{
  "type": "feishu",
  "receive_mode": "webhook",
  "webhook": { "path": "/webhooks/{channel_key}" },
  "routing": {
    "default_agent_id": "main",
    "default_team_id": "",
    "dm_scope": "per-channel-peer",
    "rules": []
  },
  "config": {
    "app_id": "cli_xxx",
    "require_mention": true,
    "allowed_user_ids": [],
    "allowed_group_ids": []
  }
}
```

**策略字段**（借鉴 MuseBot `allowed_user_ids` / 群 @ 规则，写入 `config`）：

| 字段 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `require_mention` | bool | `false` | 群聊需 @ 机器人才进入 Agent；与飞书平台「群须 @ 才推送事件」叠加生效 |
| `allowed_user_ids` | string / JSON 数组 | 空 | 发送者白名单；**非空时**仅列出的用户可触发回复 |
| `allowed_group_ids` | string / JSON 数组 | 空 | 群聊白名单（`chat_id` / 会话 ID）；**非空时**仅列出的群可触发；**单聊不受此字段约束** |

**语义（Aranea，2026-05-22 起在 `ProcessInbound` 强制执行）**：

1. 各维度 **留空 = 该维度不限制**（与 MuseBot 一致）。
2. 多个维度 **同时配置时取交集（AND）**：例如既配了 `allowed_user_ids` 又配了 `allowed_group_ids`，则必须是「允许的用户 **且** 在允许的群内」。
3. 哨兵值 **`"0"`**（单独一项或数组首项）：该维度 **拒绝所有人/所有群**（MuseBot 兼容）。
4. 拒绝时：写入 `channel_delivery` 状态 `access_denied`，并向用户回复「暂无使用权限」类提示（不进入 Agent Turn）。

**如何填写 ID**

| 平台 | `allowed_user_ids` | `allowed_group_ids` |
|------|-------------------|---------------------|
| 飞书 / Lark | 发送者 `open_id`（`ou_xxx`）或 `user_id` | 群 `chat_id`（`oc_xxx`） |
| 钉钉 | `senderStaffId` / 用户 ID | `conversationId`（群会话 ID） |
| 企微 / Slack / Telegram 等 | 平台用户 ID（见各 adapter 入站 `PeerID` / meta） | 群/频道 ID（见入站 `OutboundMeta.chat_id` 或等价字段） |

**配置示例**

仅允许两名飞书用户私聊与 @ 群聊：

```json
"config": {
  "require_mention": true,
  "allowed_user_ids": ["ou_abc123", "ou_def456"]
}
```

仅允许指定飞书群（群内任意成员 @ 机器人即可，若同时配 `require_mention` 则仍须 @）：

```json
"config": {
  "allowed_group_ids": ["oc_group_chat_id_1", "oc_group_chat_id_2"]
}
```

临时关闭所有访问（维护模式）：

```json
"config": {
  "allowed_user_ids": "0"
}
```

UI 支持 **JSON 数组** 或 **英文逗号分隔** 字符串；保存后 **重启 Channel Runtime / 后端** 即可生效（配置来自 DB，无需改代码）。

> **实现位置**：策略解析与入站校验的代码分层见 [17-channel.design.md §五](./17-channel.design.md#五service-层)。

---

## 7. 运行时行为

### 7.1 双路径入站

**A. Webhook 路径（已实现；长任务 Phase E 将改为 Accept 后 200）**  
`POST /webhooks/{channel_key}` → 验签 → 解析 → 路由 → Session → Agent Turn → 异步入队出站

**B. 长连接路径（已实现）**  
`ChannelRuntimeManager` 按实例启动 goroutine（larkws / ding stream / socketmode / polling / discordgo）→ 标准化 `InboundEvent` → 同 A 后半段

**微信 iLink 会话过期自愈（2026-08-12）**  
polling 检测 `errcode=-14` → 状态文件标记 `login_status=expired` 并退出 connector → 用户重新扫码登录写入新凭证 → 触发 runtime reload 重启 polling。

**长任务（Phase E，规划）**  
Webhook 与 WS 统一 **Accept（ACK + 200）→ 异步 Execute Turn**；详见 [§8](#8-长任务场景飞书-channel)。

**入站统一门禁（2026-05-22）**  
飞书 WS / Webhook 先经统一门禁（仅 `sender_type=user`、必须有 `message_id`、群聊需 @）→ 入站幂等（同一 `feishu:{message_id}` 只 Turn 一次）→ 访问控制 → Agent Turn。

**Hermes 对照（2026-05-24）**：Hermes 额外有 text debounce 合并、Reaction 处理中反馈、thread 会话隔离、Webhook IP 限流；Aranea 额外有 IM Preview 单条演进、Turn Job、Tool Card、preview 心跳。详见 [17 channel.design.md §十二](./17%20channel.design.md#十二hermes-agent-对照消息流转与飞书特殊处理)。

详见 [changelog/2026-05-22-Channel-Inbound-Root-Cause.md](../changelog/2026-05-22-Channel-Inbound-Root-Cause.md)。

> **代码锚点**：入站门禁、幂等、访问控制的代码分层与文件位置见 [17-channel.design.md §五](./17-channel.design.md#五service-层)。

### 7.2 出站与流式

- 默认：完整回复经 `channel_delivery` 异步发送  
- 流式（MVP ✅）：Telegram / 飞书 / Slack / LINE / Mattermost — `config.streaming_enabled`；长任务场景 **建议开启**（见 §8）  
- 文本降级（2026-08-12）：`wechat_ilink` 出站经 `markdownToWechat` 将 Markdown 降级为纯文本（标题去 `#`、列表转 `•`、强调符剥离）  
- 长任务 ACK / 进度 / 排队提示：Phase E（见 [开发计划 §10](./17-channel-development.md#10-长任务异步执行phase-e)）

### 7.3 健康与运维

- `ChannelHealthScanner`（10min）  
- `CHANNEL_DELIVERY_DISABLED=1` / `CHANNEL_HEALTH_DISABLED=1`

> **运维变量与 Runtime 指标**：见 [17-channel.development.md §9](./17-channel-development.md#9-运维) 与 [§6 Runtime 运维](./17-channel-development.md#6-runtime-运维)。

---

## 8. 长任务场景（飞书 Channel）

> **技术设计**：[17 channel.design.md §十一](./17%20channel.design.md#十一长任务异步执行设计)  
> **开发计划**：[17-channel-development.md §10](./17-channel-development.md#10-长任务异步执行phase-e)  
> **跨模块**：[17-channel-agent-team-integration.md §3.4](./17-channel-agent-team-integration.md#34-长任务与-im-体验)

### 8.1 问题陈述

管理员通过飞书 Channel 向 Agent / Team 下发命令时，常见任务耗时远超 IM 平台回调 SLA（通常 3–5 秒）或单次 Turn 默认上限（5 分钟）。用户侧表现为：长时间无回复、Webhook 重试导致重复入站、并发消息静默、工具/Team 编排阶段「假死」、超时后仅收到笼统错误。

Channel **不负责** LLM 编排；本需求定义 **IM 侧体验与受理语义**，运行时仍经 `ChatService` 与 Web Chat 共用 Session / Turn 机制。

### 8.2 用户故事

| ID | 角色 | 故事 | 价值 |
|----|------|------|------|
| LT-U01 | 飞书用户 | 发送分析类指令后 **1–2 秒内** 收到「已受理」反馈 | 确认机器人在线，降低重复发送 |
| LT-U02 | 飞书用户 | 任务执行 **数分钟** 内能看到 **进度或流式文本** | 长生成/多工具/Team 流水线可感知 |
| LT-U03 | 飞书用户 | 当前任务未完成时再发消息，收到 **「已排队」** 提示 | 与 Web Chat 排队语义一致 |
| LT-U04 | 飞书用户 | 任务失败或超时，收到 **明确原因**（非空白） | 可重试或联系运维 |
| LT-U05 | 管理员 | 为客服/研报 Channel 配置 **更长 Turn 超时** 与 **流式/进度** 开关 | 按业务线差异化 SLA |
| LT-U06 | 运维 | 按 peer / session / job 追溯长任务状态 | 排障不依赖用户截图 |

### 8.3 产品能力（分阶段）

| 阶段 | 能力 | 说明 |
|------|------|------|
| **P0** | 入站快速 ACK | Webhook HTTP 在验签与幂等后立即 200；飞书侧即时文案 |
| **P0** | 流式默认推荐 | 长生成场景开启 `streaming_enabled`，IM 消息随输出 PATCH |
| **P0** | 入队可见反馈 | Session 有 active run 时，Channel 出站「已排队」文案 |
| **P1** | Channel 级超时 | `turn_timeout_sec` / `first_byte_timeout_sec` 覆盖全局默认 |
| **P1** | Turn Job 可追踪 | 每次入站 Turn 有 job 状态（accepted / running / completed / failed / timeout） |
| **P1** | 长静默进度 | 工具调用、Team 成员切换期间 PATCH 进度文案或心跳 |
| **P2** | 超长任务异步模式 | 预期 >15min 走 Graph / Cron，IM 立即返回 task_id |
| **P2** | 飞书取消 / 卡片 | 用户发「取消」或卡片按钮终止 Turn（可选） |

### 8.4 配置项（`config_json.config`，P0–P1）

| 字段 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `streaming_enabled` | bool | `false` | 流式 PATCH（飞书/Telegram/Slack）；长任务 **建议 true** |
| `ack_message` | string | `收到，正在处理…` | 受理后立即出站；空则不发 ACK |
| `ack_on_queued` | string | 见设计文档 | active run 时入队提示；支持 `{{pending_id}}` |
| `turn_timeout_sec` | int | `300` | 覆盖 Channel Turn 上限（秒）；Team 流水线可设 900 |
| `first_byte_timeout_sec` | int | `30` | 首 token/首 delta 超时（秒）；重工具场景可设 120 |
| `progress_mode` | enum | `off` | `off` \| `text` \| `steps`（Team 成员摘要，P1） |
| `progress_quiet_sec` | int | `20` | 无输出时心跳间隔（秒）；0 关闭 |
| `heartbeat_message` | string | `仍在处理中…` | 心跳文案；可含 `{{elapsed}}` |
| `execution_mode` | enum | `sync` | `sync`：同步 Turn；`async`：一律走 Graph/Cron Job；`auto`：**仅当消息以 `/async` 开头**才升格 Job（关键词只作 UX 提示，见 CC-R-05） |

配置写入 DB，保存后 **Runtime Reload** 生效；不要求改代码。

### 8.5 运行时行为（产品语义）

1. **受理与执行分离**：飞书回调 SLA 与 Agent Turn 耗时解耦；用户先见 ACK，再见进度/结果。  
2. **与 Web Chat 一致**：同一 `session_id` 下 Session 锁、pending queue、Run 状态与 Web 共用；Channel 额外负责 IM 出站投影。  
3. **幂等不变**：同一 `message_id` 仍只 Turn 一次；快速 200 不触发重复执行。  
4. **失败可观测**：拒绝、超时、流式 PATCH 失败均写入投递/Job 审计，运维可按 Session 反查。

### 8.6 推荐配置（飞书长任务）

单 Agent + 重工具：

```json
{
  "receive_mode": "websocket",
  "config": {
    "streaming_enabled": true,
    "ack_message": "收到，正在处理，请稍候…",
    "turn_timeout_sec": 600,
    "first_byte_timeout_sec": 120,
    "progress_mode": "text",
    "progress_quiet_sec": 20
  }
}
```

Team 流水线（群 @）：

```json
{
  "config": {
    "require_mention": true,
    "streaming_enabled": true,
    "turn_timeout_sec": 900,
    "progress_mode": "steps"
  }
}
```

### 8.7 验收标准

| ID | 场景 | 预期 |
|----|------|------|
| LT-01 | 单 Agent 生成约 2 分钟 | ≤2s ACK；流式 PATCH；最终完整回复 |
| LT-02 | Team 流水线约 5 分钟 | 进度文案；不因 30s 首字节默认超时失败 |
| LT-03 | 任务进行中再发一条 | 飞书收到「已排队」；队列满按 Web Chat 规则拒绝 |
| LT-04 | Webhook 模式 10 分钟任务 | HTTP ≤3s 返回 200；无重复 Turn |
| LT-05 | Channel 设 `turn_timeout_sec=900` | 5min 全局默认不提前 kill |
| LT-06 | Turn 超时或失败 | 飞书明确错误 + Job/投递可审计 |
| LT-07 | 运维 | Monitor / Session 可按 `session_id` 查看 Channel Turn 与 FlowLog |

### 8.8 卡 Turn / 飞书无回复（排查）

> 完整分析：[2026-05-23-M55-Stuck-Turn-Inbound-Sync-Analysis.md](../changelog/2026-05-23-M55-Stuck-Turn-Inbound-Sync-Analysis.md)

| 症状 | 常见根因 | 优先检查 |
|------|----------|----------|
| Web 工具永久「正在执行」 | WS 未收 `tool_result`；本地保留 running 行 | Session WS 连接；FlowLog `chat.activity.finalize_stuck` |
| Web 思考 `…` 无正文 | Turn 未结束或缺 `text_done` / `runner_completion` | `run-status`；FlowLog `channel.turn.execute` |
| 飞书无最终回复 | 同步 Turn 未返回；空正文未 Flush；durable 升格无 completion outbound | `channel_delivery` 投递记录；`session_runs.phase` |
| 仅有「已排队」「建议 /async」 | 排队 ACK 与超时提示叠加 | 前序 Run 是否卡死；CC-UX-01 |
| 卡片 200340 | 未订阅 `card.action.trigger` 或未发布应用版本 | 飞书开发者后台 |

**修复任务**：CC-FIX-TOOL-01~03 · CC-FIX-CHANNEL-01 · CC-FEISHU-OPS-01（见 M55 Phase R-UX）。

> **代码层排查锚点**：见 [17-channel.development.md §12 IM Preview E2E 验收清单](./17-channel-development.md#12-im-preview--e2e-验收清单lt-0107)。

---

## 9. 验收标准（基础能力）

| ID | 场景 | 预期 |
|----|------|------|
| CH-01 | bundled 平台可创建、测试连接 | 对应 live/结构测试通过 |
| CH-02 | Webhook 平台收发消息 | Agent 回复送达 |
| CH-03 | 复制 Webhook URL | 完整 HTTPS |
| CH-04 | 禁用 Channel | 入站拒绝 |
| CH-05 | 非 bundled 平台 | Catalog 展示「即将支持」，不可保存（当前 13/13 平台 bundled） |
| CH-06 | 飞书 Stream / WS 模式 | 无公网 IP 可收消息 |
| CH-07 | 钉钉 Stream 模式 | 替代 Webhook 机器人 |
| CH-08 | `allowed_user_ids` / `allowed_group_ids` | 非白名单用户/群收到拒绝提示，`access_denied` 投递记录，不触发 Agent |
| CH-09 | LINE / Mattermost / Teams Webhook | 入站验签 + 出站可达 |
| CH-10 | LINE / Mattermost 流式出站 | edit-in-place PATCH 正常 |

---

## 10. 文档修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| 3.0 | 2026-05-22 | 以 MuseBot 平台连接能力为参考重写；移除 GoClaw/trpc channel 主导；扩展 Catalog 至 MuseBot 10 平台 |
| 3.1 | 2026-05-22 | §6 访问控制：`allowed_user_ids` / `allowed_group_ids` / `require_mention` 入站强制执行与用法说明 |
| 3.2 | 2026-05-22 | §8 长任务场景：用户故事、配置项、验收 LT-01–07；与 Phase E 开发计划对齐 |
| 3.3 | 2026-05-23 | §8.8 卡 Turn / 飞书无回复排查；链至 M55 Stuck-Turn 分析 |
| 3.4 | 2026-06-06 | §2 Catalog 扩展至 13 平台（+LINE/Mattermost/Teams）；全部 bundled ✅；§3 连接模式补充新平台；§7.2 流式补充 LINE/Mattermost；§9 验收 CH-09/10 |
| 3.5 | 2026-06-17 | 三件套内容边界重组：移除代码层信息（迁移至 `.design.md`）；移除子模块「Channel Agent Team 会话消息 业务集成说明」（独立文档 `17-channel-agent-team-integration.md`）；原位置保留指引 |
