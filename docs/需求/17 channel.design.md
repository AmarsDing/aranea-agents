# Channel 通道模块 — 实现设计文档

> 对应需求：`17 channel.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

外部通道接入（飞书/钉钉/微信等），管理通道配置、凭据、消息路由和投递。

---

## 二、Proto 层

### 2.1 现有 Proto

文件：`api/kratos/channel/v1/channel.proto`

```protobuf
service ChannelService {
  rpc ListChannels(ListChannelsRequest) returns (ListChannelsResponse) {
    option (google.api.http) = { get: "/v1/channels" };
  }
  rpc CreateChannel(CreateChannelRequest) returns (Channel) {
    option (google.api.http) = { post: "/v1/channels" body: "*" };
  }
  rpc UpdateChannel(UpdateChannelRequest) returns (Channel) {
    option (google.api.http) = { patch: "/v1/channels/{id}" body: "*" };
  }
  rpc DeleteChannel(DeleteChannelRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/channels/{id}" };
  }
}
```

### 2.2 待新增

| RPC | 路径 | 用途 |
|-----|------|------|
| `TestChannelConnection` | `POST /v1/channels/{id}/test` | 测试通道连接 |
| `GetChannelCatalog` | `GET /v1/channels/catalog` | 可用通道类型目录 |

---

## 三、Biz 层

### 3.1 领域模型

```go
type Channel struct {
    ID           string
    ChannelType  string  // "feishu"/"dingtalk"/"wechat"/"webhook"
    DisplayName  string
    Config       map[string]interface{}
    Status       string  // "active"/"inactive"
    AgentID      string  // 绑定的 Agent
    TeamID       string  // 绑定的 Team
    CreatedAt    string
    UpdatedAt    string
}

type ChannelCredential struct {
    ID        string
    ChannelID string
    Key       string
    Value     string  // 加密存储
}

type ChannelPeerSession struct {
    ID           string
    ChannelID    string
    PeerID       string  // 外部用户标识
    SessionID    string
}
```

### 3.2 Usecase

```go
func (uc *ChannelUsecase) List(ctx, query) (ChannelListResult, error)
func (uc *ChannelUsecase) Create(ctx, ch Channel) (Channel, error)
func (uc *ChannelUsecase) Update(ctx, ch Channel) (Channel, error)
func (uc *ChannelUsecase) Delete(ctx, id) error
func (uc *ChannelUsecase) RouteMessage(ctx, channelID, peerID, content string) error
```

---

## 四、Data 层

### 4.1 Ent Schema

- `platform_channel.go` — 通道主表
- `platform_channel_credential.go` — 凭据表
- `platform_channel_delivery.go` — 投递记录
- `platform_channel_peer_session.go` — 对端会话映射

---

## 五、Service 层

### 5.1 ChannelService

```go
func (s *ChannelService) ListChannels(ctx, req) (*ListChannelsResponse, error)
func (s *ChannelService) CreateChannel(ctx, req) (*Channel, error)
```

### 5.2 ChannelIngress

```go
// internal/service/channel_ingress.go
type ChannelIngress struct {
    channels  *biz.ChannelUsecase
    peers     biz.ChannelPeerSessionRepo
    sessions  *biz.SessionUsecase
    agents    biz.AgentRepository
    teams     biz.TeamRepository
    chat      *ChatService
}

func (ing *ChannelIngress) HandleIncomingMessage(ctx, channelID, peerID, content string) error
```

---

## 六、Wire 注入

已有，无需新增。

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/features/channels/
├── api.ts
├── types.ts
├── ChannelCatalogPicker.vue
├── ChannelEditorDialog.vue
└── components/
    ├── ChannelListPage.vue
    ├── FeishuConfigForm.vue
    └── WebhookConfigForm.vue
```

### 7.2 组件设计

**ChannelEditorDialog.vue**：通道编辑弹窗，根据 `channelType` 动态渲染配置表单

**ChannelCatalogPicker.vue**：通道类型选择器

### 7.3 API

```typescript
export async function listChannels(query: ChannelListQuery): Promise<ChannelListResult>
export async function createChannel(req: CreateChannelRequest): Promise<Channel>
export async function updateChannel(id: string, req: UpdateChannelRequest): Promise<Channel>
export async function deleteChannel(id: string): Promise<void>
export async function testChannelConnection(id: string): Promise<TestResult>
```
