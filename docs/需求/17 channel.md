# Channel 渠道管理

本文档定义 Channel **对外平台连接** 的产品需求：多 IM/协作平台接入、凭据与连接模式配置、消息路由与投递验收。

**平台连接参考**：[MuseBot](https://github.com/yincongcyincong/MuseBot)（`robot/*.go` + `http/communicate.go`）— MIT 许可，覆盖国内（飞书/钉钉/企微/微信/QQ）与海外（Telegram/Discord/Slack）主流接入方式。Aranea **只借鉴其平台 SDK 选型与连接模式**，Agent 运行时仍走 `internal/service.ChatService`，不在 Channel 层耦合 LLM。

与 **`2 agents-create.md`**（Agent 能力配置）、**`11 multi-agent.md`**（Team 编排）、**`10-session-development.md`**（会话）、**`51 消息机制.md`**（实时事件）、**`17-channel-agent-team-integration.md`**（跨模块业务主链）、**`frontend-pages.md`**（`/channels`）、**`docs/README.md`**（Kratos + trpc 分层）对齐。

> **跨模块说明**：Channel 不「拥有」Agent/Team，仅通过 `config_json.routing` 将 IM 对端映射到已存在的 Agent 或 Team，再经 `channel_peer_session` 绑定 Session。详见 [17-channel-agent-team-integration.md](./17-channel-agent-team-integration.md)。

---

## 1. 模块定位

| 项目 | 说明 |
|------|------|
| **用户目标** | 配置外部消息平台连接，将用户消息路由到 Agent/Team，异步回发回复 |
| **路由** | `/channels`（列表 + 编辑弹窗） |
| **非目标** | Channel 内编排 Agent；存完整消息明文；全局单例配置（MuseBot 式 flat conf） |

### 1.1 与 MuseBot / Aranea 架构差异

| 维度 | MuseBot | Aranea |
|------|---------|--------|
| 配置 | 全局 `conf.BaseConfInfo` + 环境变量 | DB `channel` + `channel_credential`（多实例、多 Agent） |
| 启动 | `StartRobot()` 按非空 token 启 goroutine | `ChannelRuntimeManager` 按 enabled 实例启连接 |
| 入站 | 平台文件内 `go RobotInfo.Exec()` | 统一 `ChannelIngress` / Runtime → `ChatService` |
| 出站 | `RobotInfo.SendMsg()` 巨型 type switch | `channel_delivery` worker + 平台 `SendText` |
| LLM | 耦合在 `Robot` 接口 | **禁止**；`internal/biz` 不触达 LLM |

Aranea 保留 MuseBot 验证过的 **SDK 与连接模式**，采用 Kratos 分层与 DB 多实例模型。

---

## 2. 支持的平台（Catalog）

Catalog 由 `ListChannelCatalog` 返回。

- **`bundled=true`**：本 binary 已实现 adapter，可创建并运行  
- **`bundled=false`**：规格已定义，连接能力待实现（参考 MuseBot 同平台文件）

| type | 标签 | MuseBot 参考 | 连接模式 | 状态 | 入站 | 出站 |
|------|------|-------------|----------|------|------|------|
| `feishu` | 飞书 / Lark | `robot/lark.go` | webhook · **websocket** | webhook ✅ | ✅ | ✅ |
| `dingtalk` | 钉钉 | `robot/ding.go` | webhook · **stream** | webhook ✅ | ✅ | ✅ |
| `wecom` | 企微智能机器人 | `robot/comwechat.go` | webhook | ✅ | ✅ | ✅ |
| `wecom-app` | 企微自建应用 | `robot/comwechat.go` | webhook | ✅ | ✅ | ✅ |
| `wechat` | 微信公众号 | `robot/wechat.go` | webhook（被动/客服） | ❌ | — | — |
| `slack` | Slack | `robot/slack.go` | event · **socket_mode** | event ✅ | ✅ | ✅ |
| `telegram` | Telegram | `robot/telegram.go` | webhook · **polling** | webhook ✅ | ✅ | ✅ |
| `discord` | Discord | `robot/discord.go` | **gateway** WebSocket | ❌ | — | — |
| `qq` | QQ 官方机器人 | `robot/qq.go` | webhook + WS 事件 | ❌ | — | — |
| `personal_qq` | QQ（OneBot） | `robot/personalqq.go` | OneBot HTTP 推送 | ❌ | — | — |

**MuseBot SDK 对照**（实现时优先采用）：

| 平台 | Go 依赖 |
|------|---------|
| 飞书/Lark | `larksuite/oapi-sdk-go/v3` + `larkws` |
| 钉钉 | `open-dingtalk/dingtalk-stream-sdk-go` |
| 企微 / 微信 | `ArtisanCloud/PowerWeChat/v3` |
| Slack | `slack-go/slack` + `socketmode` |
| Telegram | `go-telegram-bot-api/v5` |
| Discord | `bwmarrin/discordgo` |
| QQ 官方 | `tencent-connect/botgo` |

---

## 3. 连接模式说明

| 模式 | 适用平台 | 说明 | 公网要求 |
|------|----------|------|----------|
| `webhook` | 飞书、企微、微信、Slack Events、Telegram、QQ | Kratos HTTP 回调；路径 `/webhooks/{channel_key}` 或平台专用路径 | 需 HTTPS |
| `websocket` | 飞书 Lark WS | 长连接收事件；MuseBot `larkws.Client.Start` | 出站即可 |
| `stream` | 钉钉 Stream | `dingtalk-stream-sdk-go`；替代传统机器人 Webhook | 出站即可 |
| `socket_mode` | Slack | App Token + Bot Token；MuseBot Socket Mode | 出站即可 |
| `polling` | Telegram | `GetUpdatesChan`；无需 Webhook | 出站即可 |
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

**实现位置**：策略解析 `internal/biz/channel_access.go`；入站校验 `internal/service/channel_ingress_access.go`（在路由与 Agent Turn 之前）。

---

## 7. 运行时行为

### 7.1 双路径入站

**A. Webhook 路径（已实现）**  
`POST /webhooks/{channel_key}` → 验签 → 解析 → 路由 → Session → Agent Turn → 异步入队出站

**B. 长连接路径（已实现）**  
`ChannelRuntimeManager` 按实例启动 goroutine（larkws / ding stream / socketmode / polling / discordgo）→ 标准化 `InboundEvent` → 同 A 后半段

**入站统一门禁（2026-05-22）**  
Webhook 与 Runtime 均经 `ChannelIngress.ProcessInbound` → `checkInboundAccess`（读取 `config.allowed_*` / `require_mention`）→ 通过后才 `ResolveChannelTarget` + Agent Turn。

### 7.2 出站与流式

- 默认：完整回复经 `channel_delivery` 异步发送  
- Phase 2：流式编辑（MuseBot `MsgChan` + 平台 edit message）— Telegram / 飞书 / Slack

### 7.3 健康与运维

- `ChannelHealthScanner`（10min）  
- `CHANNEL_DELIVERY_DISABLED=1` / `CHANNEL_HEALTH_DISABLED=1`

---

## 8. 验收标准

| ID | 场景 | 预期 |
|----|------|------|
| CH-01 | bundled 平台可创建、测试连接 | 对应 live/结构测试通过 |
| CH-02 | Webhook 平台收发消息 | Agent 回复送达 |
| CH-03 | 复制 Webhook URL | 完整 HTTPS |
| CH-04 | 禁用 Channel | 入站拒绝 |
| CH-05 | 非 bundled 平台 | Catalog 展示「即将支持」，不可保存 |
| CH-06 | 飞书 Stream / WS 模式（Phase 2） | 无公网 IP 可收消息 |
| CH-07 | 钉钉 Stream 模式（Phase 2） | 替代 Webhook 机器人 |
| CH-08 | `allowed_user_ids` / `allowed_group_ids` | 非白名单用户/群收到拒绝提示，`access_denied` 投递记录，不触发 Agent |

---

## 9. 文档修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| 3.0 | 2026-05-22 | 以 MuseBot 平台连接能力为参考重写；移除 GoClaw/trpc channel 主导；扩展 Catalog 至 MuseBot 10 平台 |
| 3.1 | 2026-05-22 | §6 访问控制：`allowed_user_ids` / `allowed_group_ids` / `require_mention` 入站强制执行与用法说明 |
