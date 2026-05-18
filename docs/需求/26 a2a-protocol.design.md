# A2A 协议模块 — 实现设计文档

> 对应需求：`26 a2a-protocol.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> 最后更新：2026-05-19

---

## 一、模块概述

Agent-to-Agent 通信协议：AgentCard 管理、call_agent 工具调用、远程 Agent 发现与通信。

项目 A2A 分为两个层次：

1. **平台内 A2A（已实现）**：同工作区内 Agent 通过 `call_agent` 工具互调，AgentCard 管理、审计、Prometheus 指标。
2. **跨实例 A2A（待实现）**：基于 trpc-agent-go `a2aagent` + `a2aserver`，与远程 A2A Agent 通信，支持 AgentCard URL 发现、消息转换、流式通信、Graph 恢复。

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
