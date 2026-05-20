# A2A 协议模块 — 实现设计文档

> 对应需求：`26 a2a-protocol.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> 最后更新：2026-05-20

---

## 一、模块概述

Agent-to-Agent 通信协议：AgentCard 管理、call_agent 工具调用、远程 Agent 发现与通信。

项目 A2A 分为三个层次：

1. **平台内 A2A（已实现）**：同工作区内 Agent 通过 `call_agent` 工具互调，AgentCard 管理、审计、Prometheus 指标。
2. **Agent Kind 扩展（待实现）**：目录 Agent 增加 `agent_kind`；`a2a_proxy` 经 `BuildTRPCA2AAgent` 包装远程 A2A 服务；LLM Agent 经设置页配置 A2A Endpoint。
3. **跨实例 A2A（待实现）**：基于 trpc-agent-go `a2aagent` + `server/a2a`，AgentCard URL 发现、消息转换、流式通信、Graph 恢复。

**与 Agent 模块边界**：Agent Kind 字段与创建 UI 见 [2 agents-create.design.md](./2%20agents-create.design.md) §2.4；Endpoint 设置 Tab 见 [5 agent-setting.design.md](./5%20agent-setting.design.md) §7.4（待增）。

---

## 二、Proto 层

### 2.1 已实现

文件：`api/kratos/a2a/v1/a2a.proto`

```protobuf
service A2AService {
  rpc Discover(DiscoverRequest) returns (DiscoverResponse) {
    option (google.api.http) = { get: "/v1/a2a/discover" };
  }
  rpc Invoke(A2AInvokeRequest) returns (A2AInvokeResponse) {
    option (google.api.http) = { post: "/v1/a2a/invoke" body: "*" };
  }
  rpc UpdateAgentCard(UpdateAgentCardRequest) returns (A2AAgentCard) {
    option (google.api.http) = { put: "/v1/a2a/agents/{agent_id}/card" body: "*" };
  }
  rpc GetAgentCard(GetAgentCardRequest) returns (A2AAgentCard) {
    option (google.api.http) = { get: "/v1/a2a/agents/{agent_id}/card" };
  }
  rpc ListAudit(ListAuditRequest) returns (ListAuditResponse) {
    option (google.api.http) = { get: "/v1/a2a/audit" };
  }
}
```

核心消息：

| 消息 | 用途 |
|------|------|
| `A2ACapability` | 一个可调用能力（name, description, input/output_schema_json） |
| `A2AAgentCard` | Agent 的 A2A 公开档案（agent_id, display_name, workspace, enabled, capabilities） |
| `A2AInvokeRequest` | 跨 Agent 调用请求（callee_agent_id, capability, payload_json, caller_session_id, timeout_seconds） |
| `A2AInvokeResponse` | 调用结果（invoke_id, status, result_json, error_message, duration_ms） |
| `A2AAuditEntry` | 审计日志条目 |

### 2.2 待新增（跨实例 A2A）

```protobuf
rpc RegisterRemoteAgent(RegisterRemoteAgentRequest) returns (A2ARemoteAgent) {
  option (google.api.http) = { post: "/v1/a2a/remote-agents" body: "*" };
}
rpc ListRemoteAgents(ListRemoteAgentsRequest) returns (ListRemoteAgentsResponse) {
  option (google.api.http) = { get: "/v1/a2a/remote-agents" };
}
rpc DeleteRemoteAgent(DeleteRemoteAgentRequest) returns (google.protobuf.Empty) {
  option (google.api.http) = { delete: "/v1/a2a/remote-agents/{id}" };
}
rpc DiscoverRemoteAgent(DiscoverRemoteAgentRequest) returns (A2AAgentCard) {
  option (google.api.http) = { get: "/v1/a2a/remote-discover" };
}
```

---

## 三、Biz 层

### 3.1 已实现领域模型

文件：`internal/biz/a2a.go`

```go
type A2ACapability struct {
    Name             string
    Description      string
    InputSchemaJSON  string
    OutputSchemaJSON string
}

type A2AAgentCard struct {
    AgentID      string
    DisplayName  string
    Workspace    string
    Enabled      bool
    Capabilities []A2ACapability
    UpdatedAt    string
}

type A2AInvocation struct {
    ID              string
    CallerAgentID   string
    CalleeAgentID   string
    CallerSessionID string
    Capability      string
    PayloadJSON     string
    Status          string // pending | running | success | error | timeout
    ResultJSON      string
    ErrorMessage    string
    DurationMs      int
    TimeoutSeconds  int
}

type A2AAuditEntry struct {
    ID            string
    InvokeID      string
    CallerAgentID string
    CalleeAgentID string
    Capability    string
    Status        string
    DurationMs    int
    Workspace     string
    CreatedAt     string
}
```

### 3.2 已实现 Repo 接口

```go
type A2ARepo interface {
    UpsertAgentCard(ctx context.Context, card A2AAgentCard) (A2AAgentCard, error)
    GetAgentCard(ctx context.Context, agentID string) (A2AAgentCard, error)
    ListEnabledCards(ctx context.Context, workspace, capability string) ([]A2AAgentCard, error)

    CreateInvocation(ctx context.Context, inv A2AInvocation) (A2AInvocation, error)
    UpdateInvocation(ctx context.Context, inv A2AInvocation) error

    InsertAudit(ctx context.Context, entry A2AAuditEntry) error
    ListAudit(ctx context.Context, callerID, calleeID string, limit, offset int) ([]A2AAuditEntry, int, error)
}
```

### 3.3 已实现 Usecase

```go
func (u *A2AUsecase) UpdateAgentCard(ctx, card A2AAgentCard) (A2AAgentCard, error)
func (u *A2AUsecase) GetAgentCard(ctx, agentID string) (A2AAgentCard, error)
func (u *A2AUsecase) Discover(ctx, workspace, capability string) ([]A2AAgentCard, error)
func (u *A2AUsecase) StartInvocation(ctx, inv A2AInvocation) (A2AInvocation, error)
func (u *A2AUsecase) FinishInvocation(ctx, inv A2AInvocation) error
func (u *A2AUsecase) AppendAudit(ctx, entry A2AAuditEntry) error
func (u *A2AUsecase) ListAudit(ctx, callerID, calleeID string, limit, offset int) ([]A2AAuditEntry, int, error)
```

### 3.4 待新增领域模型（跨实例 A2A）

```go
type A2ARemoteAgent struct {
    ID          string
    Name        string
    RemoteURL   string
    AgentCard   *A2AAgentCard
    Workspace   string
    Status      string // online | offline | unknown
    AuthType    string // none | api_key | mtls
    AuthConfig  string
    CreatedAt   string
    UpdatedAt   string
}
```

### 3.5 待新增 Usecase（跨实例 A2A）

```go
func (u *A2AUsecase) RegisterRemoteAgent(ctx, cfg A2ARemoteAgent) (A2ARemoteAgent, error)
func (u *A2AUsecase) DiscoverRemoteAgent(ctx, url string) (*A2AAgentCard, error)
func (u *A2AUsecase) SendRemoteMessage(ctx, remoteURL string, msg A2AInvocation) (A2AInvocation, error)
```

---

## 四、Data 层

### 4.1 已实现

文件：`internal/data/a2a.go`

使用 raw SQL + SQLite，由 `EnsureA2ASchema(ctx, db)` 自动建表。

数据库表：

| 表 | 用途 |
|----|------|
| `a2a_agent_cards` | AgentCard 持久化（agent_id PK, display_name, workspace, enabled, capabilities JSON, updated_at） |
| `a2a_invocations` | 调用记录（id PK, caller/callee_agent_id, capability, payload_json, status, result_json, error_message, duration_ms, timeout_seconds, created_at） |
| `a2a_audit` | 审计日志（id PK, invoke_id, caller/callee_agent_id, capability, status, duration_ms, workspace, created_at） |

Repo 实现：`NewA2ARepo(db *sql.DB) biz.A2ARepo`，Wire 注入通过 `NewA2ARepoFromData(d *Data) biz.A2ARepo`。

### 4.2 待新增（跨实例 A2A）

- `a2a_remote_agents` 表：远程 Agent 注册信息
- `internal/a2a/client.go`：A2A HTTP Client，封装远程 AgentCard 发现和消息发送

```go
type A2AClient struct {
    httpClient *http.Client
}

func (c *A2AClient) Discover(ctx context.Context, url string) (*A2AAgentCard, error)
func (c *A2AClient) SendMessage(ctx context.Context, url string, msg A2AInvocation) (A2AInvocation, error)
```

---

## 五、运行时层

### 5.1 已实现：call_agent 工具

文件：`internal/a2a/tool.go`

`call_agent` 工具已实现并通过 `toolsets.go` 条件注入：

```
trpc_build.go → buildToolsetsForAgent → cfg.CallAgent = eff["call_agent"]
toolsets.go → if cfg.CallAgent { customTools = append(customTools, a2a.NewCallAgentTool()) }
```

工具流程：
1. 解析 `callAgentInput`（agent_id, capability, payload, timeout_seconds）
2. 验证目标 Agent 的 AgentCard（enabled=true + capability 存在）
3. 调用 `invokerFunc` 执行实际调用
4. 写入审计条目（success 或 error）

**关键缺口**：`WithA2AUsecase`、`WithCallerAgentID`、`WithInvoker` 三个 Context 注入函数已定义，但 service 层在创建 Agent 运行时上下文时**未调用**，导致 `call_agent` 工具运行时 `invokerFromContext` 返回 nil，报错 "invoker not configured"。

### 5.2 待实现：Context 注入

在 `internal/service/trpc_turn.go`（或其他创建 Agent 运行时上下文的位置）注入 A2A 上下文：

```go
ctx = a2a.WithA2AUsecase(ctx, a2aUsecase)
ctx = a2a.WithCallerAgentID(ctx, agentID)
ctx = a2a.WithInvoker(ctx, invokerFunc)
```

`invokerFunc` 签名：

```go
type invokerFunc func(ctx context.Context, calleeAgentID, capability, payloadJSON string, timeoutSec int) (string, error)
```

实现策略：invokerFunc 内部调用 `A2AUsecase.StartInvocation`，然后通过 Agent 运行时执行目标 Agent，最后 `FinishInvocation` 更新结果。

### 5.3 待实现：A2A Agent 构建（跨实例）

```go
// internal/a2a/trpc/agent.go
func BuildA2AAgent(ctx context.Context, cfg A2ARemoteAgent, deps) (agent.Agent, error)
```

包装 trpc-agent-go `a2aagent.New`，将远程 Agent 包装为本地可用的 `agent.Agent`。

### 5.4 待实现：A2A Server（跨实例）

```go
// internal/a2a/trpc/server.go
func NewA2AServer(ctx context.Context, agent agent.Agent, opts ...a2aserver.Option) (*a2aserver.A2AServer, error)
```

使用 `trpc.group/trpc-go/trpc-agent-go/server/a2a` 将本地 Agent 暴露为 A2A 服务。

### 5.5 待实现：Graph 恢复集成

```go
// internal/a2a/graph_resume.go
func ResumeFromA2A(ctx context.Context, graph *trpcgraph.StateGraph, msg A2AInvocation) error
```

对标 trpc-agent-go `internal/a2a/graph_resume.go`，A2A 长时间任务触发 Graph 中断，任务完成后恢复执行。

---

## 六、Service 层

### 6.1 已实现

文件：`internal/service/a2a.go`

| 方法 | HTTP | 说明 |
|------|------|------|
| `Discover` | `GET /v1/a2a/discover` | 按工作区/能力发现 A2A Agent |
| `Invoke` | `POST /v1/a2a/invoke` | 发起跨 Agent 调用（当前为 stub，仅记录 pending） |
| `UpdateAgentCard` | `PUT /v1/a2a/agents/{agent_id}/card` | 更新 Agent A2A Card |
| `GetAgentCard` | `GET /v1/a2a/agents/{agent_id}/card` | 获取 Agent A2A Card |
| `ListAudit` | `GET /v1/a2a/audit` | 浏览审计日志 |

Prometheus 指标：

| 指标 | 标签 | 说明 |
|------|------|------|
| `aranea_a2a_invoke_total` | `caller, callee, status` | 总调用次数 |
| `aranea_a2a_invoke_duration_seconds` | — | 调用延迟直方图 |

### 6.2 待新增（跨实例 A2A）

| 方法 | HTTP | 说明 |
|------|------|------|
| `RegisterRemoteAgent` | `POST /v1/a2a/remote-agents` | 注册远程 A2A Agent |
| `ListRemoteAgents` | `GET /v1/a2a/remote-agents` | 列出已注册远程 Agent |
| `DeleteRemoteAgent` | `DELETE /v1/a2a/remote-agents/{id}` | 移除远程 Agent |
| `DiscoverRemoteAgent` | `GET /v1/a2a/remote-discover` | 通过 URL 发现远程 AgentCard |

---

## 七、Wire 注入

### 7.1 已实现

```
data.ProviderSet  → NewA2ARepoFromData
biz.ProviderSet   → NewA2AUsecase
service.ProviderSet → NewA2AService
```

HTTP/gRPC 注册：`internal/server/http.go` + `grpc.go` 已注册 `A2AService`。

### 7.2 待新增（跨实例 A2A）

```
data.ProviderSet  → NewA2ARemoteAgentRepo
biz.ProviderSet   → (扩展 A2AUsecase 或新增 A2ARemoteUsecase)
service.ProviderSet → (扩展 A2AService)
```

---

## 八、Web 前端设计

### 8.1 已实现

| 文件 | 说明 |
|------|------|
| `web/src/features/a2a/types.ts` | A2A 类型定义（A2ACapability, A2AAgentCard, A2AInvokeInput, A2AInvokeResult, A2AAuditEntry） |
| `web/src/features/a2a/api.ts` | API 调用封装（discoverAgents, getAgentCard, updateAgentCard, invokeA2A, listA2AAudit） |
| `web/src/stores/a2a/index.ts` | Pinia Store（useA2AStore：discover, refreshCard, updateCard, invoke, loadAudit） |
| `web/src/components/teams/TeamEditorDialog.vue` | Team 编辑器中的 A2A 协议配置区（envelope, message_format, max_payload_chars, include_trace） |

### 8.2 待实现

| 组件 | 说明 |
|------|------|
| `A2AAgentCardPage.vue` | A2A AgentCard 管理：列表 + 启用/禁用 + 能力编辑 |
| `A2AAuditPage.vue` | A2A 审计日志浏览 |
| `A2ARemoteAgentDialog.vue` | 注册远程 Agent，输入 URL → 自动发现 AgentCard |

### 8.3 待新增 API（跨实例 A2A）

```typescript
export async function registerRemoteAgent(req: RegisterRemoteAgentRequest): Promise<A2ARemoteAgent>
export async function listRemoteAgents(): Promise<A2ARemoteAgent[]>
export async function deleteRemoteAgent(id: string): Promise<void>
export async function discoverRemoteAgent(url: string): Promise<A2AAgentCard>
```

---

## 九、安全设计

| 控制点 | 实现 | 状态 |
|--------|------|------|
| 默认关闭 | 每个 Agent `enabled=false` | ✅ |
| 工作区隔离 | `ListEnabledCards` 按工作区过滤 | ✅ |
| 审计 | 每次调用写入 `a2a_audit` | ✅ |
| AgentCard 验证 | call_agent 工具验证 enabled + capability | ✅ |
| 速率限制 | API 网关层 | ❌ |
| 远端鉴权 / mTLS | 跨实例 A2A | ❌ |
| 跨工作区调用禁止 | service 层检查 | ❌（需在 Invoke 中增加 workspace 校验） |

---

## 十、涉及文件总览

### 已实现

| 文件 | 说明 |
|------|------|
| `api/kratos/a2a/v1/a2a.proto` | Proto 定义 + HTTP 映射 |
| `internal/biz/a2a.go` | 领域模型 + A2ARepo 接口 + A2AUsecase |
| `internal/data/a2a.go` | SQLite 持久化 + EnsureA2ASchema |
| `internal/service/a2a.go` | Kratos 服务适配器 + Prometheus |
| `internal/a2a/tool.go` | call_agent 工具 + Context 辅助函数 |
| `internal/tools/trpc/toolsets.go` | call_agent 条件注入 |
| `internal/biz/agent_mcp_effective.go` | ToolKeyCallAgent 常量 |
| `internal/server/http.go` | HTTP 路由注册 |
| `internal/server/grpc.go` | gRPC 注册 |
| `web/src/features/a2a/types.ts` | 前端类型 |
| `web/src/features/a2a/api.ts` | 前端 API |
| `web/src/stores/a2a/index.ts` | Pinia Store |

### 待新增/修改

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/service/trpc_turn.go` | 修改 | 注入 A2A 上下文（WithA2AUsecase, WithCallerAgentID, WithInvoker） |
| `internal/a2a/trpc/agent.go` | 新建 | A2AAgent 适配器（跨实例） |
| `internal/a2a/trpc/server.go` | 新建 | A2A Server（跨实例） |
| `internal/a2a/trpc/converter.go` | 新建 | 消息转换（trpc Event ↔ A2A Message） |
| `internal/a2a/client.go` | 新建 | A2A HTTP Client |
| `internal/a2a/graph_resume.go` | 新建 | Graph 恢复集成 |
| `api/kratos/a2a/v1/a2a.proto` | 修改 | 新增远程 Agent RPC |
| `internal/biz/a2a.go` | 修改 | 新增 A2ARemoteAgent 模型 + 方法 |
| `internal/data/a2a.go` | 修改 | 新增 a2a_remote_agents 表 + Repo |
| `internal/service/a2a.go` | 修改 | 新增远程 Agent 服务方法 |
| `web/src/features/a2a/` | 修改 | 新增远程 Agent API + 管理页面 |
| `api/kratos/agent/v1/agent.proto` | 修改 | 新增 `agent_kind`、`a2a_proxy_config` |
| `internal/biz/agent_types.go` | 修改 | `Agent.Kind`、`A2AProxyConfig` |
| `internal/agent/trpc_build_router.go` | 新建 | `BuildTRPCAgent` 按 kind 分发 |
| `internal/agent/factory.go` | 修改 | AgentFactory 走 `BuildTRPCAgent` |
| `internal/service/trpc_turn.go` | 修改 | Runner 装配走 `BuildTRPCAgent` |
| `web/src/components/agents/AgentCreateDialog.vue` | 修改 | Agent 类型选择 + Proxy 表单 |
| `web/src/pages/agent-settings/` | 修改 | 新增 A2A Tab（Endpoint） |

---

## 十一、Agent Kind 与运行时构建（2026-05-20）

### 11.1 领域模型扩展

文件：`internal/biz/agent_types.go`（`internal/biz` **禁止** import trpc）

```go
const (
    AgentKindLLM      = "llm"       // 默认
    AgentKindA2AProxy = "a2a_proxy"
)

type Agent struct {
    // ...existing...
    Kind string // "" | "llm" | "a2a_proxy"
}

type A2AProxyConfig struct {
    RemoteURL       string
    AgentCardURL    string // 可选，默认同 RemoteURL + /.well-known/agent.json
    EnableStreaming bool
    AuthType        string // none | api_key | mtls
    AuthConfigJSON  string
    TimeoutSeconds  int
}
```

- Proxy 配置持久化：`agents.config_json.a2a_proxy` 或独立表 `agent_a2a_proxy_configs`（agent_id PK）
- LLM Endpoint 配置：继续用 `a2a_agent_cards` + 设置页字段 `expose_enabled`、`streaming`

### 11.2 Proto 扩展

文件：`api/kratos/agent/v1/agent.proto`

```protobuf
message A2AProxyConfig {
  string remote_url = 1;
  string agent_card_url = 2;
  bool enable_streaming = 3;
  string auth_type = 4;
  string auth_config_json = 5;
  int32 timeout_seconds = 6;
}

message Agent {
  // ...existing fields...
  string agent_kind = 21; // "" | "llm" | "a2a_proxy"
  A2AProxyConfig a2a_proxy_config = 22;
}

message CreateAgentRequest {
  // ...existing...
  string agent_kind = 14;
  A2AProxyConfig a2a_proxy_config = 15;
}
```

校验（biz / service）：
- `agent_kind=a2a_proxy`：`remote_url` 必填；`provider`/`model` 可空
- `agent_kind=llm`（默认）：`provider`/`model` 必填；忽略 `a2a_proxy_config`

### 11.3 运行时构建路由

```
internal/service/trpc_turn.go
    ↓
internal/agent/trpc_build_router.go  →  BuildTRPCAgent(ctx, ag, deps)
    ├── kind "" | "llm"  →  BuildTRPCLLMAgentCached
    └── kind "a2a_proxy" →  BuildTRPCA2AAgent  →  internal/a2a/trpc/agent.go
                              ↓
                         a2aagent.New (pkg/trpc-agent-go)
```

`internal/agent/factory.go` 中 `bizAgentFactoryForKey` 同步改为 `BuildTRPCAgent`，Team / Graph / Swarm transfer 解析一致。

### 11.4 BuildTRPCA2AAgent 适配器

文件：`internal/a2a/trpc/agent.go`

```go
func BuildTRPCA2AAgent(ctx context.Context, ag biz.Agent, cfg biz.A2AProxyConfig) (trpcagent.Agent, error) {
    opts := []a2aagent.Option{
        a2aagent.WithName(ag.AgentKey),
        a2aagent.WithDescription(ag.AgentDescription),
        a2aagent.WithAgentURL(cfg.RemoteURL),
    }
    if cfg.EnableStreaming {
        opts = append(opts, a2aagent.WithEnableStreaming(true))
    }
    // auth: WithExtraA2AOptions(client.WithHeader(...)) 等
    return a2aagent.New(opts...)
}
```

消息转换使用框架内置 `a2a_converter.go`，**禁止**复制到 `internal/`。

### 11.5 A2A Endpoint（LLM Agent 暴露）

文件：`internal/a2a/trpc/server.go`

- 触发条件：`agent_kind=llm` 且 `a2a_agent_cards.enabled=true`
- 使用 `server/a2a.New(WithRunner(...), WithAgentCard(...))` 挂载 HTTP 路由
- AgentCard 与 DB `a2a_agent_cards` 双向同步；capabilities 编辑走现有 `UpdateAgentCard` RPC

### 11.6 前端分工

| 位置 | 组件 | 说明 |
|------|------|------|
| 创建 | `AgentCreateDialog.vue` | `QBtnToggle` / 卡片单选：LLM \| A2A Proxy；Proxy 分支显示 URL / 发现 / 鉴权 |
| 列表 | `AgentsPage.vue` | `agent_kind=a2a_proxy` 显示 `A2A ↗`；Endpoint 启用显示 `A2A ↙` |
| 设置 | `AgentSettingsA2ATab.vue`（新建） | LLM：Endpoint + AgentCard；Proxy：只读远程信息与连接测试 |
| 注册表 | `A2APage.vue` | 工作区级 Discover / Audit / Invoke；远程注册对话框 |

### 11.7 依赖方向（不变）

```
internal/biz          ← Agent.Kind, A2AProxyConfig（无 trpc import）
internal/agent        ← BuildTRPCAgent 路由
internal/a2a/trpc     ← 包装 a2aagent / server/a2a
pkg/trpc-agent-go     ← 框架真相源
```

---

## 十二、传输与流式语义（2026-05-20）

> **结论：SSE 不是全平台唯一通道；A2A 外部协议、Admin 运维、Chat UI 使用不同传输，按场景选型。**

### 12.1 三层传输模型

| 层 | 路径 / 入口 | 传输 | 是否必须 SSE | 说明 |
|----|-------------|------|--------------|------|
| **A2A 外部协议** | `/v1/a2a/public/{agent_id}`、Proxy → 远程 A2A | **A2A JSON-RPC + HTTP 流**（`message/send` 一元 / `message/stream` SSE） | **否** | 遵循 [A2A 规范](https://a2a-protocol.org/latest/specification/)；客户端按 AgentCard `capabilities.streaming` 选择一元或流式；trpc-a2a-go `StreamMessage` 走 SSE 事件 |
| **Admin 运维** | `POST /v1/a2a/invoke` | **一元 JSON** | 否 | 测试/审计用途；聚合最终文本结果，不推送 token 流 |
| **Chat UI 实时** | `/v1/ws` | **WebSocket Envelope** | 否 | 平台主通道（见 execution-plan 红线）；与 A2A 外部协议 ** deliberately 分离** |

**WebSocket 不是 A2A 规范的必选传输**。外部 A2A 客户端应使用 A2A 协议的 HTTP 接口；仅在需要与 Chat 页同一 UX 时才走 `/v1/ws`（平台内，非标准 A2A）。

### 12.2 流式能力矩阵

| 场景 | 默认 | 配置 | 框架实现 |
|------|------|------|----------|
| **Public Endpoint** | 广告 `streaming=true` | AgentCard + `BuildA2AEndpointServer(..., streaming)` | `trpc-agent-go/server/a2a` → Runner 事件 → TaskArtifactUpdate / Message 流 |
| **A2A Proxy（Chat）** | `enable_streaming=true` | Agent 设置 `A2AProxyConfig.enable_streaming` | `a2aagent` → 远程 `StreamMessage` 或 `SendMessage` |
| **call_agent / Admin Invoke** | 非流式 | — | `RunAgentTurn` / `InvokeRemoteRegistry` 聚合输出 |
| **远程 registry Invoke** | 非流式（Admin） | registry id 作为 `callee_agent_id` | `internal/a2a/remote_invoke.go` |

### 12.3 Graph Resume

- **Public Endpoint**：`trpc-agent-go/server/a2a` 已内置 `GraphResumeStateFromMetadata`；A2A 消息 `metadata.state_delta` 携带 checkpoint / ResumeMap 即可恢复 Graph。
- **项目桥接**：`internal/a2a/graph_resume.go` 提供 `BuildGraphResumeMetadata`，供出站 A2A 消息编码 resume 字段（与框架 internal/a2a 键名对齐）。
- **参考**：`pkg/trpc-agent-go/examples/graph/a2a_interrupt/`。

### 12.4 网关注册中心（MVP）

- **RPC**：`GET /v1/a2a/gateway/discover` — 联邦 **本地 Endpoint + 远程 registry**。
- **Biz**：`GatewayDiscover`（`internal/biz/a2a_gateway.go`）；可选 `check_health` 探测远程 AgentCard。
- **与 Discover 区别**：Discover 面向运维/工具扁平列表；Gateway 显式携带 `source`、`endpoint_url`、`remote_url`、`healthy`，供联邦路由与监控。

### 12.5 远程 registry 直接 Invoke

- Discover / Gateway 中远程条目的 `agent_id` = registry `id`（如 `remote-…`）。
- `call_agent` 与 `POST /v1/a2a/invoke` 均经 `ResolveInvokeTarget` → 本地或 `InvokeRemoteRegistry`。
- 本地 AgentCard **disabled 时不** fallback 远程同 ID。

### 12.6 公开 Endpoint 配置

| 方式 | 键 | 说明 |
|------|-----|------|
| 环境变量（最高） | `A2A_PUBLIC_BASE_URL` | 容器 / GitOps 覆盖 |
| **系统设置（DB）** | `a2a_public_base_url` | **推荐**：`/settings` UI 可编辑，**保存后立即生效** |
| 配置文件 | `server.a2a_public_base_url` | `configs/config.yaml`，GitOps 备选 |
| 推导（开发） | HTTP `addr` | `0.0.0.0`/`::`/`:port` → `127.0.0.1`；启动 warn |

优先级：`env` > `db`（系统设置）> `config` > `derived`

- 只读查询：`GET /v1/a2a/config` → `public_base_url` + `public_base_url_source`（env/db/config/derived）
- UI：`/settings` 编辑；`/a2a` 页 `A2ARuntimeConfigBanner` 只读展示生效地址
