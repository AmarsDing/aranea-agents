# 微信个人号渠道（iLink 官方 API）设计文档

## 状态：提议

---

## 一、背景

项目当前已有微信公众号（wechat）和企业微信（wecom）渠道，均为 Webhook 回调模式。2026 年 3 月，腾讯通过 OpenClaw 框架正式开放**个人微信 Bot API（iLink 协议）**，接入域名为 `ilinkai.weixin.qq.com`。这是微信首次提供合法的个人 Bot 开发接口，采用 HTTP/JSON 长轮询模式，无需公网域名，无封号风险。

本设计在现有 channel 框架内新增 `wechat_ilink` 渠道类型，复用 telegram polling 的运行时模式。

## 二、目标

1. 用户在前端渠道页新建「微信（个人号·iLink）」渠道，扫码登录后关联 Agent
2. 支持私聊和群聊的文本消息收发
3. 支持图片、语音、文件、视频消息的接收和发送
4. 支持「正在输入」状态指示
5. 会话过期（errcode -14）自动检测并提示重新扫码

## 三、架构设计

### 3.1 整体链路

```
用户微信 ──扫码──▶ iLink 服务器 (ilinkai.weixin.qq.com)
                    │
    ┌───────────────┼───────────────┐
    │ ① 登录: get_bot_qrcode        │  扫码确认 → bot_token（存渠道凭证）
    │ ② 收信: getupdates 长轮询 35s │  游标 get_updates_buf 持久化
    │ ③ 发信: sendmessage           │  必须回传 context_token（按用户缓存）
    │ ④ 媒体: CDN AES-128-ECB       │  上传/下载加密媒体
    │ ⑤ 输入: sendtyping            │  显示"正在输入"状态
    └───────────────────────────────┘
                    │
            ┌───────▼────────┐
            │ internal/      │
            │ channel/       │
            │ wechatilink/   │
            │   client.go    │  HTTP client + 签名头
            │   login.go     │  扫码登录 + token 持久化
            │   polling.go   │  长轮询 starter
            │   parse.go     │  消息解析（文本/图片/语音/文件/视频）
            │   outbound.go  │  文本/媒体发送 + context_token 缓存
            │   media.go     │  CDN 上传/下载 + AES-128-ECB 加解密
            │   typing.go    │  输入状态指示
            │   errors.go    │  错误类型
            └────────────────┘
                    │
            ┌───────▼────────┐
            │ internal/      │
            │ channel/       │
            │ runtime/       │
            │   manager.go   │  统一管理长连接生命周期
            └────────────────┘
```

### 3.2 与现有框架的映射

| 组件 | telegram（参考） | wechat_ilink（新建） |
|------|-----------------|---------------------|
| 渠道包 | `internal/channel/telegram/` | `internal/channel/wechatilink/` |
| 运行时注册 | `runtime.RegisterStarterWithLogger("telegram", "polling", RunPolling)` | `runtime.RegisterStarterWithLogger("wechat_ilink", "polling", RunPolling)` |
| 入站 | `telegram.ParsePollingUpdate` → `port.InboundEvent` | `wechatilink.ParseMessage` → `port.InboundEvent` |
| 出站 | `telegram.TextSender` (HTTP POST api.telegram.org) | `wechatilink.TextSender` (HTTP POST ilinkai.weixin.qq.com) |
| 媒体 | 直接 URL | CDN + AES-128-ECB |
| 流式 | `telegram.StreamSender` | Phase 3 支持 |

### 3.3 渠道类型注册（两处）

> **评审修正**：类型注册是单点白名单 `catalogHasType`（`channel_rules.go`），注册需同时改两个文件。

```go
// ① internal/biz/channel_catalog.go — item 定义
func wechatILinkTypeItem() ChannelTypeItem {
    item := channelTypeItem("wechat_ilink", "微信（个人号·iLink）", "国内", "polling",
        "腾讯 iLink 官方 Bot API，扫码登录，支持私聊/群聊/多媒体", 45, true, false, false)
    item.ReceiveModes = []string{"polling"}
    // ConfigSchema 需显式声明群聊门控字段，前端才能渲染：
    //   group_enabled (bool, 默认 false)、require_mention (bool, 默认 true)、bot_nickname (string)
    return item
}

// ② internal/biz/channel_type_registry.go — init() specs 追加
{
    TypeItem:            wechatILinkTypeItem(),
    RequiredCredentials: []string{"bot_token"},
    CredentialProps: []CredentialProperty{
        {Key: "bot_token", Title: "wechat_ilink_bot_token", Format: "password", Required: true},
    },
    SupportsLightTest: true,
}
```

说明：`bot_token` 标记 Required 只影响 `TestChannel` 的 `pending_auth` 提示（`evaluateChannelTest`），**不阻塞渠道创建**——用户可先建渠道后扫码，凭证由登录流程自动写入。`baseurl`/`ilink_user_id` 非用户填写项，不进凭证 schema，登录后由后端写入凭证行（非敏感）。

### 3.4 运行时模式映射

```go
// internal/channel/runtime/config.go
func defaultReceiveMode(channelType string) string {
    switch strings.TrimSpace(strings.ToLower(channelType)) {
    // ... existing cases ...
    case "wechat_ilink":
        return "polling"
    }
}
```

## 四、协议对接

### 4.1 认证与请求头

所有 API 请求携带以下公共 HTTP 头：

```
iLink-App-Id: bot
iLink-App-ClientVersion: {uint32}  // 0x00MMNNPP 格式
```

POST 请求额外携带：

```
Content-Type: application/json
AuthorizationType: ilink_bot_token
Authorization: Bearer {bot_token}
X-WECHAT-UIN: {base64(String(randomUint32()))}  // 每次请求重新生成，防重放
```

### 4.2 扫码登录流程

```
GET /ilink/bot/get_bot_qrcode?bot_type=3
  → { qrcode: "会话标识符", qrcode_img_content: "data:image/png;base64,..." }

GET /ilink/bot/get_qrcode_status?qrcode={qrcode}
  → { status: "wait" | "scaned" | "scaned_but_redirect" | "expired" | "confirmed",
      bot_token: "...", baseurl: "https://ilinkai.weixin.qq.com", ilink_user_id: "..." }
```

状态流转：`wait` → `scaned` → `confirmed`（或 `expired` → 重新获取）

### 4.3 消息收取：长轮询

```
POST /ilink/bot/getupdates
{
  "get_updates_buf": "",  // 首次空字符串，之后回传上次响应值
  "base_info": { "channel_version": "2.1.1" }
}
```

服务器挂起连接最长 35 秒，有新消息时立即返回：

```json
{
  "ret": 0,
  "msgs": [WeixinMessage, ...],
  "get_updates_buf": "新的同步游标",
  "longpolling_timeout_ms": 35000
}
```

**关键机制**：
- `get_updates_buf` 是同步游标，必须持久化（重启续收）
- 不回传或回传旧值会导致重复收到消息
- 无历史消息查询 API，只能通过长轮询获取实时消息

### 4.4 消息发送

```json
POST /ilink/bot/sendmessage
{
  "msg": {
    "to_user_id": "user@im.wechat",
    "client_id": "自定义客户端ID",
    "message_type": 2,
    "message_state": 2,
    "context_token": "从 getUpdates 收到的 token",
    "item_list": [
      { "type": 1, "text_item": { "text": "回复内容" } }
    ]
  },
  "base_info": { "channel_version": "2.1.1" }
}
```

**context_token 规则**：
- 每条收到的消息都带有 `context_token`
- 回复时必须回传同一个 token 以关联到正确的对话窗口
- 按 `(userId)` 缓存，跨重启持久化
- 会话过期（errcode -14）或重新登录时清除

### 4.5 消息结构

```go
type WeixinMessage struct {
    Seq           int64        `json:"seq"`
    MessageID     int64        `json:"message_id"`
    FromUserID    string       `json:"from_user_id"`    // xxx@im.wechat
    ToUserID      string       `json:"to_user_id"`      // xxx@im.bot
    ClientID      string       `json:"client_id"`
    CreateTimeMs  int64        `json:"create_time_ms"`
    SessionID     string       `json:"session_id"`
    GroupID       string       `json:"group_id"`        // 群聊非空
    MessageType   int          `json:"message_type"`    // 1=USER, 2=BOT
    MessageState  int          `json:"message_state"`   // 0=NEW, 1=GENERATING, 2=FINISH
    ItemList      []MessageItem `json:"item_list"`
    ContextToken  string       `json:"context_token"`
}

type MessageItem struct {
    Type         int       `json:"type"`         // 1=TEXT, 2=IMAGE, 3=VOICE, 4=FILE, 5=VIDEO
    TextItem     *TextItem     `json:"text_item,omitempty"`
    ImageItem    *ImageItem    `json:"image_item,omitempty"`
    VoiceItem    *VoiceItem    `json:"voice_item,omitempty"`
    FileItem     *FileItem     `json:"file_item,omitempty"`
    VideoItem    *VideoItem    `json:"video_item,omitempty"`
}
```

### 4.6 CDN 媒体协议

**上传流程**：
1. `POST /ilink/bot/getuploadurl` → 获取预签名上传地址
2. AES-128-ECB 加密文件（PKCS7 填充，随机 16 字节密钥）
3. `POST {cdn_url}/upload` → 上传加密后的二进制数据
4. `sendmessage` 中引用 CDNMedia

**下载流程**：
1. 使用 `CDNMedia.full_url`（v2.1.1+ 优先）或拼接下载 URL
2. HTTP GET 下载密文
3. AES-128-ECB 解密（兼容两种密钥编码格式）
4. PKCS7 去填充

### 4.7 输入状态指示

```
POST /ilink/bot/getconfig
  → { typing_ticket: "base64编码的ticket" }  // 缓存 TTL 24h

POST /ilink/bot/sendtyping
{ "ilink_user_id": "...", "typing_ticket": "...", "status": 1 }
  // status: 1=TYPING, 2=CANCEL
```

## 五、数据模型

### 5.1 渠道配置（config_json）

```json
{
  "type": "wechat_ilink",
  "receive_mode": "polling"
}
```

### 5.2 渠道凭证（credentials）

| credential_key | 说明 | 来源 |
|---------------|------|------|
| `bot_token` | iLink Bot Token（Bearer） | 扫码登录后自动获取 |
| `ilink_user_id` | iLink 用户标识 | 扫码登录后自动获取 |
| `baseurl` | iLink API 基础 URL | 扫码登录后自动获取（默认 ilinkai.weixin.qq.com） |

凭证由扫码登录流程自动写入，用户无需手动填写。

### 5.3 持久化状态（本地状态文件）

> **评审修正（2026-08-12）**：原设计写 `metadata_json`，但 runtime starter 签名（`ch, creds, lookup, handler, lg`）无 usecase 访问权，无法写 DB。且 iLink 协议丢失 `get_updates_buf` 的后果是**丢消息**（协议 §sync_buf：未回传正确 buf 将无法收到错过的消息），必须可靠持久化。改为包内状态文件，自包含、无跨层改动。

存储位置：`bin/data/channel-state/wechat_ilink-<channel_id>.json`（`bin/` 为既有运行产物目录，符合根目录规范）。

```json
{
  "get_updates_buf": "上次同步游标",
  "context_tokens": {
    "user@im.wechat": "context_token_value"
  },
  "last_login_at": "2026-08-12T10:00:00Z",
  "login_status": "active | expired"
}
```

写入策略：每轮 getupdates 成功返回后原子写入（临时文件 + rename）；启动时读取恢复。

## 六、API 设计

### 6.1 新增 Proto RPC

```protobuf
// api/kratos/channel/v1/channel.proto

message WechatILinkLoginRequest {
  string channel_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message WechatILinkLoginResponse {
  string qrcode_data_url = 1;  // data:image/png;base64,...
  string qrcode_session = 2;   // 会话标识符，用于轮询状态
  string status = 3;           // wait | scaned | confirmed | expired
}

message WechatILinkPollRequest {
  string channel_id = 1 [(google.api.field_behavior) = REQUIRED];
  string qrcode_session = 2 [(google.api.field_behavior) = REQUIRED];
}

message WechatILinkPollResponse {
  string status = 1;           // wait | scaned | confirmed | expired
  string error_message = 2;    // 过期或失败时的错误信息
}

service ChannelService {
  // ... existing RPCs ...
  
  rpc WechatILinkLogin(WechatILinkLoginRequest) returns (WechatILinkLoginResponse) {
    option (google.api.http) = {
      post: "/v1/channels/{channel_id}/wechat-ilink/login"
      body: "*"
    };
  }
  
  rpc WechatILinkPoll(WechatILinkPollRequest) returns (WechatILinkPollResponse) {
    option (google.api.http) = {
      post: "/v1/channels/{channel_id}/wechat-ilink/poll"
      body: "*"
    };
  }
}
```

### 6.2 前端交互流程

```
用户点击「扫码登录」
  → 调用 WechatILinkLogin → 返回二维码 data URL
  → 前端显示二维码图片
  → 每 3 秒轮询 WechatILinkPoll
    → status=wait: 继续轮询
    → status=scaned: 提示"已扫码，请确认登录"
    → status=confirmed: 登录成功，自动刷新渠道状态
    → status=expired: 提示二维码已过期，提供「刷新二维码」按钮
```

## 七、错误处理

| 错误码 | 含义 | 处理策略 |
|--------|------|---------|
| `ret=0` | 成功 | — |
| `errcode=-14` | 会话超时/过期 | starter 记 Error 日志 + `EmitConnectError`（"微信登录已过期，请重新扫码"）后返回错误；supervisor backoff 重启并重读凭证；用户重扫后 credential revision 变化触发 fingerprint 变更 → 自动重启恢复（自愈，无需 park 逻辑）；状态文件标记 `login_status=expired` 供登录 RPC 查询 |
| `errcode=-2` | 参数错误 | 记录错误日志，跳过当前消息 |
| HTTP 4xx | 客户端错误 | 记录日志，指数退避重试（最多 3 次） |
| HTTP 5xx | 服务端错误 | 指数退避重试（最多 3 次） |
| 长轮询连续失败 | 连接异常 | 1-2 次等待 2s，3 次以上等待 30s |
| CDN 上传失败 | 网络/服务端问题 | 最多重试 3 次，4xx 立即失败 |

## 八、安全考虑

1. **bot_token 存储**：写入渠道凭证表（`channel_credential`），与现有凭证体系一致，支持 secret_ref 加密存储
2. **X-WECHAT-UIN 防重放**：每次请求生成新的随机 uint32，base64 编码后发送
3. **context_token 缓存**：按用户隔离，不跨会话泄漏
4. **媒体加密**：CDN 上传/下载使用 AES-128-ECB + PKCS7，密钥随机生成
5. **SSRF 防护**：CDN URL 白名单校验（仅限 `*.cdn.weixin.qq.com` 域名）

## 九、测试策略

| 层级 | 范围 | 方式 |
|------|------|------|
| 单元测试 | `wechatilink/` 包内所有函数 | Go 标准 testing + httptest 模拟 iLink API |
| 集成测试 | 长轮询 + 发送 + 媒体加解密 | 使用 mock HTTP server 模拟 iLink 响应 |
| 契约测试 | `channel.OutboundText` 接口实现 | `internal/channel/contract_test.go` 添加断言 |
| 端到端 | 扫码登录流程（前端 + 后端） | 手动测试（需真实微信扫码） |

## 十、改动文件清单

### 新增文件

```
internal/channel/wechatilink/
  client.go       // HTTP client + 请求签名
  client_test.go
  login.go        // 扫码登录流程
  login_test.go
  polling.go      // 长轮询 starter（注册到 runtime）
  polling_test.go
  parse.go        // 消息解析（文本/图片/语音/文件/视频）
  parse_test.go
  outbound.go     // 文本/媒体发送 + context_token 缓存
  outbound_test.go
  media.go        // CDN 上传/下载 + AES-128-ECB 加解密
  media_test.go
  typing.go       // 输入状态指示
  typing_test.go
  errors.go       // 错误类型定义
  types.go        // 消息结构体定义
```

### 修改文件

```
api/kratos/channel/v1/channel.proto           // 新增 WechatILinkLogin/WechatILinkPoll RPC
internal/biz/channel_catalog.go               // wechatILinkTypeItem() item 定义
internal/biz/channel_type_registry.go         // init() specs 注册 wechat_ilink（凭证 schema 白名单）
internal/channel/runtime/config.go            // 默认模式映射 wechat_ilink → polling
internal/channel/port/meta.go                 // 新增 MetaContextToken well-known key
internal/service/channel_platform_registry.go // 注册 wechat_ilink outbound handler
internal/service/channel.go                   // 注册 wechat_ilink live tester（getconfig 只读探活）
internal/service/channel_wechat_ilink_login.go // 新增：扫码登录 RPC 实现
internal/channel/contract_test.go             // 添加 wechatilink.TextSender 契约断言
web/src/services/kratos/channel/v1/index.ts   // make api 自动再生成（protoc-gen-typescript-http）
web/src/features/channels/api.ts              // 新增 wechatIlinkLogin/wechatIlinkPoll 方法
web/src/features/channels/ChannelEditorDialog.vue + useChannelEditorForm.ts // 扫码登录区块（二维码展示+轮询+过期刷新）
web/src/features/channels/channelPlatformFields.ts // wechat_ilink 平台字段
docs/development/17-channel.md                // 文档同步（DOC-SYNC 红线）：§2 平台表 + §5 凭据配置 + §7 运行时
docs/development/17-channel.design.md         // §三 Adapter + §五 Service 层
docs/development/17-channel.development.md    // 任务清单
```

> **说明**：`internal/channel/preview/platform.go`（PlatformTextLimit）使用默认 4000 即可，无需改动；`biz/channel_im_render.go` 的 `ChannelACKDeferredToPreview` 与 `biz/avatar_channel_refresh.go` 平台映射在 P3（流式预览）时才需追加 wechat_ilink。

## 十一、Phase 划分

| Phase | 内容 | 验收标准 |
|-------|------|---------|
| P0 | 扫码登录 + bot_token 持久化 + 文本私聊收发 | 扫码后能在微信私聊中收到 AI 回复 |
| P1 | 群聊支持 + context_token 缓存 + 会话过期检测 | 群聊中按门控策略响应，过期后前端提示重扫 |
| P2 | 媒体消息：入站接收 + 出站发送能力 + CDN 加解密 | 能接收多媒体（见 §11.1），出站具备媒体发送能力（见 §11.2） |
| P3 | 「正在输入」状态 + 流式回复预览 | 处理消息时微信显示"对方正在输入" |

### 11.1 P2 入站媒体落地方式

`port.InboundEvent` 当前仅支持文本（无附件字段），因此入站媒体在一期内落地为：

| 消息类型 | 处理方式 |
|---------|---------|
| 语音 | 优先使用微信服务端转写结果 `voice_item.text` 作为文本入站；无转写时降级为占位符 `[语音消息，未识别]` |
| 图片 | CDN 下载 + AES 解密 + 存本地媒体目录，注入占位符文本 `[图片]` |
| 文件 | 同上，注入 `[文件: 文件名]` |
| 视频 | 同上，注入 `[视频]` |

> 让 Agent「看到」图片内容（多模态入站）需要扩展 `InboundEvent` 附件字段及会话管线，属于跨模块改造，列入后续迭代，不在本设计范围。

### 11.2 P2 出站媒体范围

出站投递载荷 `biz.ChannelOutboundPayload` 仅含 `Text`/`CardJSON`/`Extra`，无文件字段。一期内：

- `wechatilink` 出站适配器实现完整的媒体上传/发送能力（getuploadurl → AES 加密 → CDN 上传 → sendmessage 引用 CDNMedia），并有完整单元测试
- 触发入口：识别 `Extra["x_file_path"]`（预留扩展点），当前上游管线无触发源，故**业务侧「Agent 主动发图到微信」的管线集成列入后续迭代**
- 一期内 Agent 回复仍以文本为主；回复中的 Markdown 按 Hermes 实践做微信友好降级（标题转【标题】等）

### 11.3 群聊门控策略（需运行时验证）

iLink 协议原生携带 `group_id`，但「bot 在群内能收到哪些消息」（全部 or 仅 @）腾讯未公开文档化。一期策略：

- `config_json` 新增 `group_enabled`（bool，默认 false）与 `require_mention`（bool，默认 true）
- mention 检测：文本包含 `@<bot昵称>`（bot 昵称可配置，默认尝试从登录信息推断）
- 具体可收消息范围在联调时实测确认，并更新本文档

## 十二、参考

- iLink 协议文档：[wechat-clawbot/docs/ilink-protocol.md](https://github.com/nightsailer/wechat-clawbot/blob/master/docs/ilink-protocol.md)
- Go SDK 参考：[the-yex/wechat-ilink-sdk](https://github.com/the-yex/wechat-ilink-sdk)
- 官方 npm 包：`@tencent-weixin/openclaw-weixin` v2.1.1
- OpenClaw 文档：https://docs.openclaw.ai
