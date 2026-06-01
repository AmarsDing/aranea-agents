# OutboundRouter 出站路由

## 一、需求文档

### 1.1 背景

当前项目 `internal/channel/` 已有 12 个渠道适配器（Telegram/Slack/Feishu/Discord/WeCom/WeChat/QQ/Line/Mattermost/Teams/DingTalk/OneBot），但所有出站能力仅用于**被动回复**——收到 Inbound 消息后，通过 `channel.OutboundText.SendText()` 回复同一会话。

Agent 无法**主动**向渠道发送消息。例如：
- Agent 完成长时间任务后主动通知用户
- 定时任务 Agent 推送报告
- 子 Agent 完成后通知父 Agent 的渠道

框架 `pkg/trpc-agent-go/openclaw/internal/outbound/` 提供了完整的出站路由参考实现：
- `Router`：注册 `channel.TextSender`/`channel.MessageSender`，按 `DeliveryTarget` 路由消息
- `Tool`：`message` 工具，Agent 可调用发送文本/文件到指定渠道
- `ResolveTarget`：从显式参数、Runtime State、Session 上下文三级解析出站目标
- `SentTextRecorder`：去重记录，防止重复发送

项目 `internal/channel/surface.go` 已定义 `OutboundText` 接口（对齐框架 `channel.TextSender`），各渠道适配器已实现该接口。但缺少：
- 将已有 `OutboundText` 实现桥接到框架 `channel.TextSender`/`channel.MessageSender` 的适配层
- 出站路由器（Router）的实例化和注册
- 让 Agent 可主动发消息的 `message` 工具
- 目标解析逻辑（从 Session/Channel 配置推断出站目标）

### 1.2 目标

1. 实现出站路由器，将已有渠道出站能力统一注册，Agent 可按 `DeliveryTarget` 路由消息
2. 新增 `message` 工具，Agent 可主动向任意已注册渠道发送文本/文件
3. 实现目标解析，从 Session 上下文、Runtime State、渠道配置自动推断出站目标
4. 支持多渠道同时注册，Agent 无需关心底层渠道差异

### 1.3 功能需求

| # | 功能 | 优先级 | 说明 |
|---|------|--------|------|
| F1 | OutboundRouter 核心 | P0 | 基于 `outbound.Router`，注册所有渠道的 TextSender/MessageSender |
| F2 | OutboundText → channel.TextSender 桥接 | P0 | 适配层，让已有 `channel.OutboundText` 实现满足框架 `channel.TextSender` 接口 |
| F3 | message 工具 | P0 | Agent 可调用 `message` 工具发送文本/文件到指定渠道 |
| F4 | 目标解析 | P0 | `ResolveTarget` 三级解析：显式参数 > Runtime State > Session 上下文 |
| F5 | SentTextRecorder 去重 | P1 | 同一轮 Turn 内防止向同一目标重复发送相同文本 |
| F6 | 渠道注册自动化 | P1 | Channel Runtime Manager 启动连接器时自动注册出站 Sender |
| F7 | OutboundMessage 文件发送 | P2 | 支持通过 `channel.MessageSender` 发送文件（需渠道支持） |

### 1.4 非功能需求

- 出站消息发送延迟 < 1s（不含渠道 API 延迟）
- Router 注册/查询操作线程安全（`sync.RWMutex`）
- 目标解析失败返回明确错误，不静默丢弃
- 日志统一使用 `internal/event FlowLog`
- 所有 goroutine 使用 `pkg/safego.Go`

### 1.5 验收标准

1. Agent 调用 `message` 工具指定 `channel` + `target` 能成功发送文本
2. Agent 调用 `message` 工具不指定 `channel`/`target`，能自动从当前 Session 解析出站目标
3. 多个渠道同时注册到 Router，消息按 `DeliveryTarget.Channel` 正确路由
4. 向未注册渠道发送消息返回明确错误
5. 同一轮 Turn 内重复发送相同文本到同一目标被去重
6. Channel Runtime Manager 启动连接器后，对应渠道的 TextSender 自动注册到 Router

---

## 二、设计文档

### 2.1 框架参考（trpc-agent-go / OpenClaw）

#### Router 核心

**`outbound.Router`** — `pkg/trpc-agent-go/openclaw/internal/outbound/router.go`

```go
type DeliveryTarget struct {
    Channel string `json:"channel,omitempty"`
    Target  string `json:"target,omitempty"`
}

type Router struct {
    mu             sync.RWMutex
    textSenders    map[string]channel.TextSender
    messageSenders map[string]channel.MessageSender
}

func NewRouter() *Router
func (r *Router) Register(ch channel.Channel)
func (r *Router) RegisterSender(sender channel.TextSender)
func (r *Router) RegisterMessageSender(sender channel.MessageSender)
func (r *Router) Channels() []string
func (r *Router) SendText(ctx context.Context, target DeliveryTarget, text string) error
func (r *Router) SendMessage(ctx context.Context, target DeliveryTarget, msg channel.OutboundMessage) error
```

#### Channel 接口

**`channel.TextSender`** — `pkg/trpc-agent-go/openclaw/channel/channel.go`

```go
type Channel interface {
    ID() string
    Run(ctx context.Context) error
}

type OutboundFile struct {
    Path    string
    Name    string
    AsVoice bool
}

type OutboundMessage struct {
    Text  string
    Files []OutboundFile
}

type TextSender interface {
    Channel
    SendText(ctx context.Context, target string, text string) error
}

type MessageSender interface {
    Channel
    SendMessage(ctx context.Context, target string, msg OutboundMessage) error
}
```

#### message 工具

**`outbound.Tool`** — `pkg/trpc-agent-go/openclaw/internal/outbound/tool.go`

```go
type Tool struct {
    router *Router
}

func NewTool(router *Router) *Tool
func (t *Tool) Declaration() *tool.Declaration
func (t *Tool) Call(ctx context.Context, args []byte) (any, error)
```

工具输入 `toolInput`：
```go
type toolInput struct {
    Text         string   `json:"text"`
    File         string   `json:"file,omitempty"`
    Files        []string `json:"files,omitempty"`
    Media        []string `json:"media,omitempty"`
    Channel      string   `json:"channel,omitempty"`
    Target       string   `json:"target,omitempty"`
    AsVoice      bool     `json:"as_voice,omitempty"`
    AudioAsVoice bool     `json:"audio_as_voice,omitempty"`
}
```

#### 目标解析

**`outbound.ResolveTarget`** — `pkg/trpc-agent-go/openclaw/internal/outbound/resolve.go`

```go
func ResolveTarget(ctx context.Context, explicit DeliveryTarget) (DeliveryTarget, error)
func RuntimeStateForTarget(target DeliveryTarget) map[string]any
func ResolveTargetFromSessionID(sessionID string) (DeliveryTarget, bool)
```

解析链：`sanitizeTarget` → `fillTargetFromOpaqueValue` → `fillTargetFromRuntime` → `fillTargetFromSession`

Runtime State 键：
- `openclaw.delivery.channel` — 渠道 ID
- `openclaw.delivery.target` — 渠道目标

#### 去重记录

**`outbound.SentTextRecorder`** — `pkg/trpc-agent-go/openclaw/internal/outbound/record.go`

```go
type SentTextRecorder struct { ... }
func NewSentTextRecorder() *SentTextRecorder
func WithSentTextRecorder(ctx context.Context, recorder *SentTextRecorder) context.Context
func (r *SentTextRecorder) Record(target DeliveryTarget, text string)
func (r *SentTextRecorder) Contains(target DeliveryTarget, text string) bool
func (r *SentTextRecorder) ContainsTarget(target DeliveryTarget) bool
```

### 2.2 项目现有出站能力

**`channel.OutboundText`** — `internal/channel/surface.go`

```go
type OutboundText interface {
    Identified
    SendText(ctx context.Context, recipient string, text string) error
}
```

已实现的 `OutboundText` 适配器（`internal/channel/contract_test.go` 编译时断言）：

| 渠道 | 类型 | 文件 |
|------|------|------|
| feishu | `lark.FeishuTextSender` | `internal/channel/lark/feishu_outbound.go` |
| slack | `slack.TextSender` | `internal/channel/slack/outbound.go` |
| telegram | `telegram.TextSender` | `internal/channel/telegram/outbound.go` |
| line | `line.TextSender` | `internal/channel/line/outbound.go` |
| mattermost | `mattermost.TextSender` | `internal/channel/mattermost/outbound.go` |
| teams | `teams.TextSender` | `internal/channel/teams/outbound.go` |
| discord | `discord.TextSender` | `internal/channel/discord/outbound.go` |
| wecom | `wecom.TextSender` | `internal/channel/wecom/webhook.go` |
| wechat | `wechat.TextSender` | `internal/channel/wechat/outbound.go` |
| qq | `qq.TextSender` | `internal/channel/qq/outbound.go` |

**`port.OutboundTarget`** — `internal/channel/port/types.go`

```go
type OutboundTarget struct {
    Recipient string
    Meta      map[string]string
}
```

**`port.InboundEvent`** — `internal/channel/port/types.go`

```go
type InboundEvent struct {
    PlatformType   string
    PeerID         string
    PeerKey        string
    Text           string
    IdempotencyKey string
    OutboundMeta   map[string]string
}
```

**`port.Meta*` 常量** — `internal/channel/port/meta.go`

```go
const (
    MetaRecipient         = "recipient"
    MetaChatID            = "chat_id"
    MetaChatType          = "chat_type"
    MetaThreadID          = "thread_id"
    MetaSessionID         = "session_id"
    MetaSessionWebhook    = "session_webhook"
    MetaResponseURL       = "response_url"
    MetaServiceURL        = "service_url"
    MetaConversationID    = "conversation_id"
    MetaReplyToken        = "reply_token"
    ...
)
```

### 2.3 架构设计

#### 核心思路

项目 `channel.OutboundText` 接口与框架 `channel.TextSender` 接口签名一致（`ID() string` + `SendText(ctx, target, text) error`），但类型系统不同。设计桥接适配器将已有 `OutboundText` 实现包装为框架 `channel.TextSender`，避免修改已有渠道代码。

```
┌─────────────────────────────────────────────────────────┐
│  internal/outbound                                       │
│  ┌──────────────┐    ┌──────────────────────────────┐   │
│  │  Router       │    │  outboundAdapter             │   │
│  │  (框架 Router) │◄───│  channel.TextSender 适配层   │   │
│  │              │    │  包装 OutboundText → TextSender│   │
│  └──────┬───────┘    └──────────────────────────────┘   │
│         │                                                │
│  ┌──────┴───────┐    ┌──────────────────────────────┐   │
│  │  message 工具  │    │  TargetResolver              │   │
│  │  (Agent 调用)  │    │  三级解析：显式>Runtime>Session│   │
│  └──────────────┘    └──────────────────────────────┘   │
│                                                          │
│  ┌──────────────────┐                                   │
│  │  SentTextRecorder │                                   │
│  │  (去重记录)        │                                   │
│  └──────────────────┘                                   │
└─────────────────────────────────────────────────────────┘
         │ 注册
         ▼
┌─────────────────────────────────────────────────────────┐
│  internal/channel/* (已有渠道适配器)                       │
│  telegram.TextSender / slack.TextSender / feishu...      │
│  discord.TextSender / wecom.TextSender / wechat...       │
└─────────────────────────────────────────────────────────┘
```

#### 2.3.1 OutboundText → channel.TextSender 桥接适配器

**文件**：`internal/outbound/adapter.go`

```go
package outbound

import (
    "context"

    ch "aranea-agents/internal/channel"
    trpcchannel "trpc.group/trpc-go/trpc-agent-go/openclaw/channel"
)

type outboundTextAdapter struct {
    inner ch.OutboundText
}

func WrapOutboundText(inner ch.OutboundText) trpcchannel.TextSender {
    return &outboundTextAdapter{inner: inner}
}

func (a *outboundTextAdapter) ID() string {
    return a.inner.ID()
}

func (a *outboundTextAdapter) Run(_ context.Context) error {
    return nil
}

func (a *outboundTextAdapter) SendText(ctx context.Context, target string, text string) error {
    return a.inner.SendText(ctx, target, text)
}
```

#### 2.3.2 OutboundRouter 服务

**文件**：`internal/outbound/router.go`

```go
package outbound

import (
    "context"
    "sync"

    ch "aranea-agents/internal/channel"
    trpcchannel "trpc.group/trpc-go/trpc-agent-go/openclaw/channel"
    trpcoutbound "trpc.group/trpc-go/trpc-agent-go/openclaw/internal/outbound"
)

type DeliveryTarget = trpcoutbound.DeliveryTarget
type OutboundMessage = trpcchannel.OutboundMessage
type OutboundFile = trpcchannel.OutboundFile

type Router struct {
    inner *trpcoutbound.Router
    mu    sync.RWMutex
}

func NewRouter() *Router {
    return &Router{
        inner: trpcoutbound.NewRouter(),
    }
}

func (r *Router) RegisterOutboundText(sender ch.OutboundText) {
    r.inner.RegisterSender(WrapOutboundText(sender))
}

func (r *Router) RegisterTextSender(sender trpcchannel.TextSender) {
    r.inner.RegisterSender(sender)
}

func (r *Router) RegisterMessageSender(sender trpcchannel.MessageSender) {
    r.inner.RegisterMessageSender(sender)
}

func (r *Router) Channels() []string {
    return r.inner.Channels()
}

func (r *Router) SendText(ctx context.Context, target DeliveryTarget, text string) error {
    return r.inner.SendText(ctx, target, text)
}

func (r *Router) SendMessage(ctx context.Context, target DeliveryTarget, msg OutboundMessage) error {
    return r.inner.SendMessage(ctx, target, msg)
}

func (r *Router) Inner() *trpcoutbound.Router {
    return r.inner
}
```

#### 2.3.3 目标解析器

**文件**：`internal/outbound/resolve.go`

```go
package outbound

import (
    "context"
    "fmt"
    "strings"

    "aranea-agents/internal/channel/port"
    trpcoutbound "trpc.group/trpc-go/trpc-agent-go/openclaw/internal/outbound"
    "trpc.group/trpc-go/trpc-agent-go/agent"
)

const (
    runtimeStateDeliveryChannel = "aranea.delivery.channel"
    runtimeStateDeliveryTarget  = "aranea.delivery.target"
)

func ResolveTarget(ctx context.Context, explicit DeliveryTarget) (DeliveryTarget, error) {
    target := sanitizeTarget(explicit)
    target = fillFromRuntime(ctx, target)
    target = fillFromSession(ctx, target)
    if strings.TrimSpace(target.Channel) == "" || strings.TrimSpace(target.Target) == "" {
        return DeliveryTarget{}, fmt.Errorf("outbound: unable to resolve target")
    }
    return target, nil
}

func RuntimeStateForTarget(target DeliveryTarget) map[string]any {
    clean := sanitizeTarget(target)
    if clean.Channel == "" || clean.Target == "" {
        return nil
    }
    return map[string]any{
        runtimeStateDeliveryChannel: clean.Channel,
        runtimeStateDeliveryTarget:  clean.Target,
    }
}

func sanitizeTarget(target DeliveryTarget) DeliveryTarget {
    return DeliveryTarget{
        Channel: strings.TrimSpace(target.Channel),
        Target:  strings.TrimSpace(target.Target),
    }
}

func fillFromRuntime(ctx context.Context, target DeliveryTarget) DeliveryTarget {
    if ctx == nil {
        return target
    }
    if target.Channel == "" {
        if v, ok := agent.GetRuntimeStateValueFromContext[string](ctx, runtimeStateDeliveryChannel); ok {
            target.Channel = strings.TrimSpace(v)
        }
    }
    if target.Target == "" {
        if v, ok := agent.GetRuntimeStateValueFromContext[string](ctx, runtimeStateDeliveryTarget); ok {
            target.Target = strings.TrimSpace(v)
        }
    }
    return target
}

func fillFromSession(ctx context.Context, target DeliveryTarget) DeliveryTarget {
    if ctx == nil {
        return target
    }
    inv, ok := agent.InvocationFromContext(ctx)
    if !ok || inv == nil || inv.Session == nil {
        return target
    }
    resolved, ok := ResolveTargetFromSessionID(inv.Session.ID)
    if !ok {
        return target
    }
    if target.Channel == "" {
        target.Channel = resolved.Channel
    }
    if target.Target == "" {
        target.Target = resolved.Target
    }
    return target
}

func ResolveTargetFromSessionID(sessionID string) (DeliveryTarget, bool) {
    return trpcoutbound.ResolveTargetFromSessionID(sessionID)
}
```

#### 2.3.4 message 工具

**文件**：`internal/outbound/tool.go`

```go
package outbound

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"

    "trpc.group/trpc-go/trpc-agent-go/tool"
)

const toolName = "message"

type messageToolInput struct {
    Text    string   `json:"text"`
    File    string   `json:"file,omitempty"`
    Files   []string `json:"files,omitempty"`
    Channel string   `json:"channel,omitempty"`
    Target  string   `json:"target,omitempty"`
}

type MessageTool struct {
    router *Router
}

func NewMessageTool(router *Router) *MessageTool {
    return &MessageTool{router: router}
}

func (t *MessageTool) Declaration() *tool.Declaration {
    return &tool.Declaration{
        Name:        toolName,
        Description: "Send text and optional files through registered channels. If channel/target are omitted, resolves from current session context.",
        InputSchema: &tool.Schema{
            Type: "object",
            Properties: map[string]*tool.Schema{
                "text": {
                    Type:        "string",
                    Description: "Message text to send. Required unless files are provided.",
                },
                "files": {
                    Type:  "array",
                    Items: &tool.Schema{Type: "string"},
                    Description: "Optional local file paths to send. Only supported on channels with MessageSender capability.",
                },
                "file": {
                    Type:        "string",
                    Description: "Alias for a single file path.",
                },
                "channel": {
                    Type:        "string",
                    Description: "Channel id (e.g. telegram, slack, feishu). When omitted, resolves from runtime/session context.",
                },
                "target": {
                    Type:        "string",
                    Description: "Channel-specific target (e.g. chat_id, open_id, channel_id). When omitted, resolves from runtime/session context.",
                },
            },
        },
    }
}

func (t *MessageTool) Call(ctx context.Context, args []byte) (any, error) {
    if t == nil || t.router == nil {
        return nil, fmt.Errorf("message tool: not configured")
    }
    var in messageToolInput
    if err := json.Unmarshal(args, &in); err != nil {
        return nil, fmt.Errorf("message tool: invalid args: %w", err)
    }
    text := strings.TrimSpace(in.Text)
    paths := collectPaths(in.File, in.Files)
    if text == "" && len(paths) == 0 {
        return nil, fmt.Errorf("message tool: text or files required")
    }
    target, err := ResolveTarget(ctx, DeliveryTarget{
        Channel: in.Channel,
        Target:  in.Target,
    })
    if err != nil {
        return nil, err
    }
    msg := OutboundMessage{Text: text}
    for _, p := range paths {
        msg.Files = append(msg.Files, OutboundFile{Path: p})
    }
    if err := t.router.SendMessage(ctx, target, msg); err != nil {
        return nil, err
    }
    return map[string]any{
        "ok":         true,
        "channel":    target.Channel,
        "target":     target.Target,
        "files_sent": len(msg.Files),
    }, nil
}

func collectPaths(single string, multi []string) []string {
    seen := make(map[string]struct{})
    var out []string
    for _, p := range append([]string{single}, multi...) {
        p = strings.TrimSpace(p)
        if p == "" {
            continue
        }
        if _, ok := seen[p]; ok {
            continue
        }
        seen[p] = struct{}{}
        out = append(out, p)
    }
    return out
}
```

#### 2.3.5 SentTextRecorder 集成

**文件**：`internal/outbound/recorder.go`

```go
package outbound

import (
    "context"
    "sync"

    trpcoutbound "trpc.group/trpc-go/trpc-agent-go/openclaw/internal/outbound"
)

type SentTextRecorder = trpcoutbound.SentTextRecorder

func NewSentTextRecorder() *SentTextRecorder {
    return trpcoutbound.NewSentTextRecorder()
}

func WithSentTextRecorder(ctx context.Context, recorder *SentTextRecorder) context.Context {
    return trpcoutbound.WithSentTextRecorder(ctx, recorder)
}
```

#### 2.3.6 渠道注册集成

**文件**：`internal/outbound/register.go`

```go
package outbound

import (
    "aranea-agents/internal/channel/port"
)

func RegisterFromInboundEvent(router *Router, platformType string, meta map[string]string, sender OutboundTextSender) {
    if sender == nil {
        return
    }
    router.RegisterOutboundText(sender)
}
```

#### 2.3.7 工具注册

**文件**：`internal/tools/toolset.go`（修改）

在 `Registry()` 中新增 `message` 工具注册项：

```go
{
    Name:        "message",
    Description: "Send text and optional files through registered channels (outbound messaging)",
    Category:    "communication",
    Tags:        []string{"communication", "outbound", "channel", "message"},
    EnabledByDefault:     false,
    RiskLevel:            "high",
    RequiresConfirmation: true,
}
```

**文件**：`internal/tools/toolset.go`（修改 `AssemblyConfig`）

```go
type AssemblyConfig struct {
    ...
    OutboundRouter *outbound.Router
    ...
}
```

**文件**：`internal/tools/toolset.go`（修改 `Assemble` 函数）

在 `Assemble` 函数末尾，当 `enabled["message"]` 且 `cfg.OutboundRouter != nil` 时，构造 `outbound.NewMessageTool(cfg.OutboundRouter)` 并追加到 `out.Tools`。

#### 2.3.8 Service 层集成

**文件**：`internal/service/chat.go`（修改）

Runner 装配时注入 Runtime State：

```go
runtimeState := outbound.RuntimeStateForTarget(outbound.DeliveryTarget{
    Channel: channelID,
    Target:  targetKey,
})
```

通过 `agent.WithRuntimeState(runtimeState)` 注入到 Runner 上下文。

**文件**：`cmd/admin/wire.go`（修改）

新增 `outbound.Router` 的 Wire Provider。

### 2.4 数据流

```
Agent 调用 message 工具
    │
    ▼
messageTool.Call(ctx, args)
    │
    ├─ 1. 解析 toolInput (text/files/channel/target)
    │
    ├─ 2. ResolveTarget(ctx, explicit)
    │      ├─ fillFromRuntime: 从 agent.RuntimeState 读取
    │      └─ fillFromSession: 从 agent.Invocation.Session.ID 推断
    │
    ├─ 3. router.SendMessage(ctx, target, msg)
    │      ├─ 查找 messageSenders[channel]
    │      ├─ 查找 textSenders[channel]
    │      └─ 调用 sender.SendText/SendMessage
    │
    └─ 4. recordSentText(ctx, target, msg)  [去重]
```

### 2.5 文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/outbound/adapter.go` | 新增 | OutboundText → channel.TextSender 桥接适配器 |
| `internal/outbound/router.go` | 新增 | Router 封装（委托框架 `outbound.Router`） |
| `internal/outbound/resolve.go` | 新增 | 目标解析（三级链：显式 > Runtime > Session） |
| `internal/outbound/tool.go` | 新增 | `message` 工具实现 |
| `internal/outbound/recorder.go` | 新增 | SentTextRecorder 薄封装 |
| `internal/outbound/register.go` | 新增 | 渠道注册辅助函数 |
| `internal/tools/toolset.go` | 修改 | Registry 新增 message 注册项 + AssemblyConfig 新增 OutboundRouter + Assemble 构造 MessageTool |
| `internal/service/chat.go` | 修改 | Runner 装配时注入 Runtime State（delivery.channel/target） |
| `internal/channel/runtime/manager.go` | 修改 | 连接器启动时自动注册 OutboundText 到 Router |
| `cmd/admin/wire.go` | 修改 | 新增 outbound.Router Wire Provider |
| `internal/outbound/adapter_test.go` | 新增 | 适配器单元测试 |
| `internal/outbound/resolve_test.go` | 新增 | 目标解析单元测试 |
| `internal/outbound/tool_test.go` | 新增 | message 工具单元测试 |
| `internal/outbound/router_test.go` | 新增 | Router 集成测试 |

---

## 三、开发计划

### P5-01：创建 outbound 包 + 适配器

**文件**：`internal/outbound/adapter.go`

- 定义 `outboundTextAdapter` struct，包装 `channel.OutboundText` 为 `channel.TextSender`
- 实现 `ID()` / `Run()` / `SendText()` 三方法
- `Run()` 返回 nil（被动出站不需要长运行）
- 编写 `adapter_test.go`：验证适配后 `ID()` 和 `SendText()` 正确委托

### P5-02：Router 封装

**文件**：`internal/outbound/router.go`

- 定义 `Router` struct，内嵌 `trpcoutbound.Router`
- 定义类型别名 `DeliveryTarget` / `OutboundMessage` / `OutboundFile`
- 实现 `NewRouter()` / `RegisterOutboundText()` / `RegisterTextSender()` / `RegisterMessageSender()` / `Channels()` / `SendText()` / `SendMessage()` / `Inner()`
- `RegisterOutboundText` 内部调用 `WrapOutboundText` 后委托给 `inner.RegisterSender`
- 编写 `router_test.go`：验证多渠道注册和路由分发

### P5-03：目标解析器

**文件**：`internal/outbound/resolve.go`

- 定义 Runtime State 键常量 `aranea.delivery.channel` / `aranea.delivery.target`
- 实现 `ResolveTarget()` 三级解析链
- 实现 `RuntimeStateForTarget()` 生成 Runtime State
- 实现 `fillFromRuntime()` / `fillFromSession()` / `sanitizeTarget()`
- 委托 `ResolveTargetFromSessionID()` 到框架 `trpcoutbound.ResolveTargetFromSessionID`
- 编写 `resolve_test.go`：测试显式参数、Runtime State 回退、Session 回退、解析失败

### P5-04：message 工具

**文件**：`internal/outbound/tool.go`

- 定义 `messageToolInput` struct
- 定义 `MessageTool` struct，持有 `*Router`
- 实现 `NewMessageTool()` / `Declaration()` / `Call()`
- `Call` 流程：解析输入 → `ResolveTarget` → 构建 `OutboundMessage` → `router.SendMessage` → 返回结果
- 实现 `collectPaths()` 辅助函数
- 编写 `tool_test.go`：测试正常发送、缺少参数、解析失败、未注册渠道

### P5-05：SentTextRecorder 封装

**文件**：`internal/outbound/recorder.go`

- 类型别名 `SentTextRecorder = trpcoutbound.SentTextRecorder`
- 封装 `NewSentTextRecorder()` / `WithSentTextRecorder()`

### P5-06：渠道注册辅助

**文件**：`internal/outbound/register.go`

- 实现 `RegisterFromInboundEvent()` 辅助函数
- 在 Inbound 处理流程中，当渠道适配器创建 `OutboundText` 实例时，自动注册到 Router

### P5-07：工具注册

**文件**：`internal/tools/toolset.go`

- `Registry()` 新增 `message` 注册项（Category: communication, RiskLevel: high, RequiresConfirmation: true）
- `AssemblyConfig` 新增 `OutboundRouter *outbound.Router` 字段
- `Assemble()` 中当 `enabled["message"]` 且 `cfg.OutboundRouter != nil` 时，构造 `outbound.NewMessageTool(cfg.OutboundRouter)` 追加到 `out.Tools`

### P5-08：Service 层 Runtime State 注入

**文件**：`internal/service/chat.go`

- 在 Runner 装配时，从渠道上下文提取 `channelID` 和 `targetKey`
- 调用 `outbound.RuntimeStateForTarget()` 生成 Runtime State
- 通过 `agent.WithRuntimeState(runtimeState)` 注入到 Runner 上下文
- 确保 Agent 运行时可通过 `agent.GetRuntimeStateValueFromContext` 读取

### P5-09：Channel Runtime Manager 集成

**文件**：`internal/channel/runtime/manager.go`

- `Manager` 新增 `router *outbound.Router` 字段
- `NewManager` 新增 `router` 参数
- 连接器启动后，如果渠道适配器实现了 `channel.OutboundText`，自动调用 `router.RegisterOutboundText(sender)`
- 连接器停止时，从 Router 注销（可选，框架 Router 当前不支持注销）

### P5-10：Wire 注入

**文件**：`cmd/admin/wire.go`

- 新增 `outbound.NewRouter` Wire Provider
- 将 `*outbound.Router` 注入到 `runtime.Manager` 和 `tools.Assemble` 的 `AssemblyConfig`
- 运行 `make wire` 重新生成 `wire_gen.go`

### P5-11：集成测试

**文件**：`internal/outbound/integration_test.go`

- 模拟完整流程：注册渠道 → Agent 调用 message 工具 → 验证消息到达
- 测试多渠道路由
- 测试 Runtime State 注入后的自动解析
- 测试 SentTextRecorder 去重

### P5-12：渠道适配器扩展（P2）

**文件**：各渠道 `outbound.go`

- 为支持文件发送的渠道（Telegram/Feishu/Slack）实现 `channel.MessageSender` 接口
- 新增 `SendMessage(ctx, target, msg)` 方法
- 在 `internal/outbound/adapter.go` 新增 `outboundMessageAdapter`

### P5-13：前端 Outbound 配置 UI（P2）

**文件**：前端相关

- Agent 配置页面新增 `message` 工具开关
- 渠道配置页面显示 Outbound 能力状态
- 消息发送历史展示

### P5-14：文档和验收

- 更新模块交叉参考手册
- 更新架构蓝图
- 全量验证：`make api && make wire && make build && make test && make lint`
