# Aranea-Agents Agent 编排系统 — Channel 需求分析

> **文档版本**: v1.0  
> **日期**: 2026-05-10  
> **项目**: aranea-agents（Kratos + Google ADK Go Agent 编排平台）  
> **当前状态**: Channel 管理后台（CRUD / Catalog / 凭据）已基本完成，运行时适配器层（ADK 集成）为占位状态，待补充。

---

## 一、概述与定义

### 1.1 Channel 是什么

在 Aranea-Agents 编排系统中，**Channel（渠道/交互通道）** 是 **外部通信平台与系统内部 Agent / Team 之间的桥梁**。

Channel 负责：

- **接入**：接收来自外部平台（飞书、Telegram、Slack、微信等 30+ 平台）的用户消息
- **适配**：将各平台特定的消息格式归一化为系统内部的统一消息结构
- **路由**：将标准化消息投递给目标 Agent 或 Team 进行推理处理
- **响应**：将 Agent 的回复反向适配为对应平台的响应格式并发送回用户

### 1.2 产品定位

Channel 是 Aranea「多智能体任务编排平台」的 **入口层（Ingress Layer）**，与以下子系统协同：

```
┌─────────────────────────────────────────────────────┐
│  用户 ──→ [飞书/Telegram/微信/Slack/...]              │
│              │                                       │
│      ┌───────▼────────┐                             │
│      │   Channel 层    │ ◀── 本文档分析范围            │
│      │  (接入/适配/路由) │                             │
│      └───────┬────────┘                             │
│              ▼                                       │
│      ┌───────────────┐                              │
│      │  Agent / Team  │ ◀── ADK Runner 执行          │
│      └───────┬───────┘                              │
│              ▼                                       │
│      ┌───────────────┐                              │
│      │  Memory / Tool │ ◀── 记忆 / 工具 / MCP        │
│      └───────────────┘                              │
└─────────────────────────────────────────────────────┘
```

### 1.3 与 Chat 子系统的关系

| 维度 | Chat（原生聊天） | Channel（渠道接入） |
|------|------------------|---------------------|
| 入口 | Web UI 内嵌聊天窗口 | 第三方平台（飞书/Telegram/...） |
| 消息格式 | 内部格式（已知） | 各平台异构格式（需适配） |
| 认证 | Kratos Web 登录态 | 各平台 Webhook 签名/Token 校验 |
| 会话 | ADK Session（内部） | Channel Peer → Session 映射 |
| 路由 | 直接指定 Agent | routing.default_agent_id + DM scope |

---

## 二、现状分析

### 2.1 已完成的 Phase 1：Channel 管理后台

| 层级 | 文件 | 状态 |
|------|------|------|
| **Proto API** | `api/kratos/channel/v1/channel.proto` | ✅ 完成 |
| **Service 层** | `internal/service/channel.go` | ✅ 完成（12 个 RPC handler） |
| **Biz 层** | `internal/biz/channel.go` / `channel_catalog.go` / `channel_rules.go` | ✅ 完成 |
| **Data 层** | `internal/data/channel.go`（Ent ORM） | ✅ 完成 |
| **Web 前端** | `web/src/pages/ChannelsPage.vue` | ✅ 完成 |
| **Database** | `PlatformChannel` / `PlatformChannelCredential` / `PlatformChannelDelivery` 表 | ✅ 完成 |

**已实现的功能清单**：

- ✅ 30+ 平台 Catalog（飞书、Telegram、Slack、微信、QQ、Discord 等）
- ✅ Channel CRUD（创建/读取/更新/软删除）
- ✅ Channel 启用/停用切换
- ✅ 凭据管理（创建/更新/删除/脱敏展示）
- ✅ 结构校验（type 必须在 catalog 中、config_json 合法性等）
- ✅ 测试连接（结构级别校验，非真实 API 调用）
- ✅ Delivery 日志记录
- ✅ 前端管理页面（搜索/筛选/表格/弹窗编辑）

### 2.2 待完成的 Phase 2：Channel 运行时（ADK 集成）

| 层级 | 文件 | 状态 |
|------|------|------|
| **ADK 占位** | `internal/channel/adk/doc.go` | ❌ 仅占位注释 |
| **Channel Adapter** | 各平台适配器实现 | ❌ 未实现 |
| **Webhook Handler** | 各平台 Webhook 接收端点 | ❌ 未实现 |
| **消息格式化** | ChannelMessage 归一化 | ❌ 未实现 |
| **路由引擎** | Channel → Agent/Team 路由 | ❌ 未实现 |
| **真实密钥解析** | Secret Ref → 真实密钥（KMS/加密） | ❌ 未实现 |
| **真实连接测试** | 各平台 API 连通性测试 | ❌ 未实现（目前仅结构校验） |

---

## 三、Phase 2 核心需求：Channel 运行时

### 3.1 Channel Adapter 接口定义

**目标**：定义一个统一的 `ChannelAdapter` 接口，各平台实现各自的适配器。

```go
// internal/channel/adk/adapter.go（建议）

// ChannelMessage 是归一化后的内部消息
type ChannelMessage struct {
    ChannelID   string            // channel 实例 ID
    ChannelKey  string            // channel_key（如 telegram、feishu）
    ChannelType string            // 平台类型（config_json.type）
    AccountID   string            // 多账号时的账号标识
    PeerID      string            // 对话对端标识（用户/群/频道）
    PeerName    string            // 对端可读名称
    SenderID    string            // 发送人标识
    SenderName  string            // 发送人名称
    MessageID   string            // 平台消息 ID
    Text        string            // 消息文本
    Attachments []Attachment      // 附件列表
    Timestamp   time.Time         // 时间戳
    RawPayload  json.RawMessage   // 原始平台消息
}

type Attachment struct {
    Type     string // image, file, audio, video, location
    URL      string
    MimeType string
    FileName string
    Size     int64
}

// ChannelAdapter 定义各平台的接入适配器
type ChannelAdapter interface {
    // Type 返回平台类型标识
    Type() string

    // Start 启动适配器（注册 Webhook、启动 Polling 等）
    Start(ctx context.Context, config ChannelRuntimeConfig) error

    // Stop 停止适配器
    Stop(ctx context.Context) error

    // Health 健康检查（真实 API 调用）
    Health(ctx context.Context) (*HealthResult, error)

    // SendMessage 向平台发送消息
    SendMessage(ctx context.Context, target PeerTarget, msg OutgoingMessage) error
}

type ChannelRuntimeConfig struct {
    ID          string
    Key         string
    Name        string
    Type        string
    Enabled     bool
    ConfigJSON  string
    MetadataJSON string
    Credentials map[string]string // credential_key → secret_ref 映射
}
```

### 3.2 接入方式支持矩阵

| 接入方式 | 说明 | 适用平台 | 优先级 |
|----------|------|----------|--------|
| **Webhook** | 被动接收 HTTP POST | 飞书、Telegram、Slack、微信、Facebook、WhatsApp | P0 |
| **WebSocket** | 长连接双向通信 | QQ(NapCat)、Mattermost、Discord Gateway | P1 |
| **Polling** | 主动轮询拉取 | Telegram(可选)、LINE、Nextcloud Talk | P2 |
| **Gateway** | 平台网关协议 | Discord Interaction、MS Teams | P2 |
| **Bridge** | 桥接/代理 | iMessage、BlueBubbles | P3 |
| **QRCode** | 扫码登录 | 微信 ClawBot | P3 |

### 3.3 Webhook 注册与消息接收流程

```
用户 ─→ 第三方平台 ─→ [POST webhook] ─→ Kratos HTTP Server
                                              │
                                    ┌─────────▼─────────┐
                                    │  Webhook Dispatcher │
                                    │  (路径 → ChannelID)  │
                                    └─────────┬─────────┘
                                              │
                                    ┌─────────▼─────────┐
                                    │  签名/Token 校验    │
                                    └─────────┬─────────┘
                                              │
                                    ┌─────────▼─────────┐
                                    │  消息格式适配      │
                                    │  (平台 → ChannelMessage) │
                                    └─────────┬─────────┘
                                              │
                                    ┌─────────▼─────────┐
                                    │  路由决策          │
                                    │  (routing → Agent)  │
                                    └─────────┬─────────┘
                                              │
                                    ┌─────────▼─────────┐
                                    │  ADK Runner.Run()  │
                                    └─────────┬─────────┘
                                              │
                                    ┌─────────▼─────────┐
                                    │  ChannelAdapter    │
                                    │  SendMessage()     │
                                    └─────────┬─────────┘
                                              ▼
                                          第三方平台 ─→ 用户
```

### 3.4 Webhook 路径约定

```
/{base_prefix}/{channel_key}
示例: /webhooks/ch_telegram_support
      /webhooks/ch_feishu_sales
```

- 路径信息从 `config_json.webhook.path` 读取
- 路径唯一性由 `channel_key` 保证（数据库 UNIQUE 约束）
- Webhook Dispatcher 通过路径反查 `channel_key` → 加载 Channel 配置

### 3.5 消息路由策略

`config_json.routing` 字段定义路由规则：

```json
{
  "routing": {
    "default_agent_id": "main",
    "dm_scope": "per-channel-peer",
    "rules": [
      {
        "peer_pattern": "oc_*",
        "agent_id": "support_agent"
      }
    ]
  }
}
```

| 路由策略 | 说明 |
|----------|------|
| `default_agent_id` | 无特殊规则时的默认目标 Agent |
| `dm_scope` | 会话隔离级别：`main`（全局共享）、`per-peer`（按对端）、`per-channel-peer`（按渠道+对端） |
| `rules` | 可选的对端匹配规则：支持 peer_pattern 通配符匹配 |

### 3.6 测试连接（真实 API 调用）

当前实现仅做结构校验，需要改为真实 API 调用：

| 平台 | 测试 API | 说明 |
|------|----------|------|
| Telegram | `getMe` | 校验 bot_token |
| 飞书 | `tenant_access_token` | 校验 app_id + app_secret |
| Slack | `auth.test` | 校验 bot_token |
| Discord | Gateway 连接测试 | 校验 bot_token |
| 微信 | `gettoken` | 校验 app_id + app_secret |
| WhatsApp | Graph API 健康检查 | 校验 access_token |

### 3.7 密钥管理

当前凭据存储使用 `sha256(channelID:key:secret)` 哈希，**不存储真实密钥**。需要升级为：

- **方案 A（推荐 MVP）**：环境变量注入 `ARANEA_CHANNEL_SECRET_{KEY}_{CHANNEL_ID}=xxx`，`secret_ref` 格式为 `env:ARANEA_CHANNEL_SECRET_xxx`
- **方案 B（生产）**：对接 KMS / Vault，`secret_ref` 格式为 `kms:arn:...` 或 `vault:path/to/secret`

---

## 四、Phase 2 MVP 实施路线

### 4.1 MVP 范围（P0 - 必做）

| 序号 | 功能 | 说明 |
|------|------|------|
| 1 | **Channel Adapter 接口** | 定义 `ChannelAdapter` + `ChannelMessage` + 路由接口 |
| 2 | **Webhook Dispatcher** | HTTP 路径 → Channel 映射 + 签名校验框架 |
| 3 | **Telegram Adapter** | webhook + bot_token 校验，消息收发闭环 |
| 4 | **飞书 Adapter** | webhook + app_secret 校验，消息收发闭环 |
| 5 | **真实密钥解析** | env 方案，Adapter 启动时解析 secret_ref |
| 6 | **真实连接测试** | Telegram `getMe` + 飞书 token 获取 |
| 7 | **路由集成** | ChannelMessage → ADK Runner.Run() → SendMessage 回写 |

### 4.2 P1（扩展）

| 序号 | 功能 | 说明 |
|------|------|------|
| 8 | Slack Adapter | Events API + Bot Token |
| 9 | Discord Adapter | Gateway / Interaction |
| 10 | 微信 Adapter | 公众号消息回调 |
| 11 | Channel 健康监控 | 自动检测连接状态，更新 `status` |

### 4.3 P2-P3（长期）

| 序号 | 功能 | 说明 |
|------|------|------|
| 12 | WebSocket 支持 | QQ(NapCat)、Mattermost |
| 13 | Polling 支持 | Telegram long polling 降级方案 |
| 14 | KMS 集成 | 生产级密钥存储 |
| 15 | 多账号支持 | 一个 Channel 多个账号（飞书多租户等） |

---

## 五、非功能需求

### 5.1 安全性

- Webhook 签名校验**强制启用**（不允许跳过）
- 密钥永不到达前端、日志、审计 JSON
- 消息内容不入 `channel_delivery` 表（仅存事件 ID、hash、状态码）
- Channel 凭据变更必须记录审计日志
- 凭据本地 SHA256 哈希仅用于结构化校验，不做真实 API 调用

### 5.2 可靠性

- Webhook 接收失败 → 返回 5xx → 平台重试（飞书/Telegram 均有重试机制）
- Adapter panic → recover + 记录错误 + 不阻塞其他 Channel
- 消息发送失败 → 记录 `channel_delivery` + 通知管理员

### 5.3 可扩展性

- 新增平台仅需实现 `ChannelAdapter` 接口 + 注册到 `AdapterRegistry`
- `config_json` 和 `credential_schema` 由 catalog 描述，前端自动渲染表单
- 路由规则通过 `config_json.routing` 扩展，无需改代码

### 5.4 性能

- Webhook 接收路径延迟 < 50ms（签名校验 + 入队）
- 消息异步处理（入队后由 Worker 调用 ADK Runner）
- Channel adapter 并发处理，互不阻塞

---

## 六、集成点架构图

```
┌──────────────────────────────────────────────────────────────┐
│                      Kratos HTTP Server                       │
│                                                              │
│  /api/v1/channels/*        /webhooks/{channel_key}           │
│  (管理 API)                 (消息入口)                         │
│       │                          │                           │
│       ▼                          ▼                           │
│  ┌──────────┐          ┌──────────────────┐                  │
│  │ Channel   │          │  Webhook          │                  │
│  │ Service   │          │  Dispatcher       │                  │
│  └────┬─────┘          └────────┬─────────┘                  │
│       │                         │                            │
│       ▼                         ▼                            │
│  ┌──────────────────────────────────────┐                    │
│  │           ChannelUseCase              │                    │
│  │  (biz 层 - 配置/凭据/合法性校验)        │                    │
│  └────┬─────────────────┬───────────────┘                    │
│       │                 │                                    │
│       ▼                 ▼                                    │
│  ┌─────────┐    ┌───────────────────┐                       │
│  │ Channel  │    │  AdapterRegistry   │                       │
│  │ Repo     │    │  (Telegram/飞书/...) │                      │
│  └────┬────┘    └────────┬──────────┘                       │
│       │                  │                                   │
│       ▼                  ▼                                   │
│  ┌─────────┐    ┌───────────────────┐                       │
│  │ Ent/DB  │    │  ADK Runner.Run()  │                       │
│  │         │    │  (Agent 推理)       │                       │
│  └─────────┘    └───────────────────┘                       │
└──────────────────────────────────────────────────────────────┘
```

---

## 七、验收标准

| 编号 | 测试场景 | 预期结果 |
|------|----------|----------|
| AC1 | 创建 Telegram Channel + 配置 bot_token | Channel 列表显示「正常」状态 |
| AC2 | 测试连接（Telegram getMe） | 返回 `ok: true, status: "ok"` |
| AC3 | 用户在 Telegram 发送消息 | 消息被接收、路由到 Agent、Agent 回复发回 Telegram |
| AC4 | 停用 Channel | Channel 不再接收消息 |
| AC5 | 删除 Channel | 软删除，运行时停止适配器 |
| AC6 | Webhook 签名校验失败 | 返回 403，不投递消息 |
| AC7 | Agent 处理超时/错误 | 记录 `channel_delivery` 错误信息 |
| AC8 | 新增平台（Slack） | 仅需实现 Adapter 接口，前端自动渲染表单 |

---

## 八、附录

### 8.1 当前 Catalog 优先级矩阵

| 平台 | 优先级 | 接入方式 | 难度 |
|------|--------|----------|------|
| Telegram | P0 | webhook | 低 |
| 飞书 | P0 | webhook | 中 |
| Slack | P1 | events | 中 |
| Discord | P1 | gateway | 中 |
| 微信(公众号) | P1 | webhook | 高 |
| WhatsApp | P2 | webhook | 中 |
| QQ(NapCat) | P2 | websocket | 中 |
| 钉钉 | P2 | webhook | 中 |
| 企业微信 | P2 | webhook | 中 |
| 其他 20+ | P3 | 各异 | 各异 |

### 8.2 现有文档索引

| 文档 | 路径 |
|------|------|
| Channel 设计文档 | `docs/需求/17 channel.md` |
| 完善计划 | `docs/plan.md` |
| 产品需求总览 | `docs/需求/产品需求总览.md` |
| 平台架构 | `docs/architecture/platform-architecture.md` |
| Agent 运行时边界 | `docs/architecture/runtime-boundary.md` |

### 8.3 术语对照

| 中文 | 英文 | 说明 |
|------|------|------|
| 渠道 | Channel | 外部通信平台接入点 |
| 适配器 | Adapter | 各平台的消息收发实现 |
| 凭据 | Credential | 密钥/Token/Secret 的存储引用 |
| 投递 | Delivery | 消息入站后的处理状态记录 |
| 目录 | Catalog | 可接入平台的元数据清单 |
| 对端 | Peer | 第三方平台上的对话对象（用户/群/频道） |
