# A2A 协议模块 — 实现设计文档

> 对应需求：[26 a2a-protocol.md](./26%20a2a-protocol.md)
> 遵循规范：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)
> 关联：Agent 创建 [2 agents-create.design.md](./2%20agents-create.design.md) · Agent 设置 [5 agent-setting.design.md](./5%20agent-setting.design.md)
> 进度差距：[26-a2a-development.md](./26-a2a-development.md)

---

## 一、模块概述

Agent-to-Agent：平台内 `call_agent`、Admin Invoke、LLM **公开 Endpoint**、`a2a_proxy` 远程代理、工作区远程注册与联邦 Discover。

**设计目标**：

1. **biz 不 import trpc-agent-go** — 领域在 `internal/biz`；框架包装在 `internal/a2a/trpc`
2. **单一 Invoker 派发** — tool 与 HTTP Invoke 共用 `NewInvoker` + `RunAgentTurn` / `InvokeRemoteRegistry`
3. **Agent Kind 路由** — `BuildTRPCAgent` 分发 `llm` | `a2a_proxy`
4. **传输三分工** — A2A HTTP（外部）/ Admin JSON（运维）/ WebSocket（Chat UI）

**与 Agent 模块边界**：`agent_kind`、`A2AProxyConfig` 持久化见 `internal/biz/agent_kind.go`；创建/设置 UI 见 Agent 模块设计文档。

---

## 二、分层架构（已实现）

```
┌─────────────────────────────────────────────────────────────┐
│  Server                                                      │
│  http.go — A2AService + HandlePrefix(/v1/a2a/public/...)     │
│  auth — PublicPathPrefix 免 Admin JWT                        │
├─────────────────────────────────────────────────────────────┤
│  Service                                                     │
│  a2a.go           — Discover/Invoke/Card/Audit/Remote/Gateway│
│  a2a_endpoint.go  — A2AEndpointBuilder → 公开 handler 缓存    │
│  a2a_public_base.go — A2APublicBaseReloader（热更新 + 清缓存）│
│  chat_native.go   — RunAgentTurn + injectA2AContext          │
│  trpc_turn.go     — runCtx 注入 A2A；Proxy 跳过 intent        │
├─────────────────────────────────────────────────────────────┤
│  internal/a2a（协议桥接，可 import biz，不 import trpc 运行时）│
│  tool.go          — call_agent                               │
│  invoker.go       — NewInvoker、工作区校验、InjectRunContext  │
│  callee_resolve.go— ResolveInvokeTarget                      │
│  card_validate.go — CheckCalleeCard                          │
│  remote_client.go — FetchRemoteAgentCard、MTLSHTTPClient     │
│  remote_invoke.go — InvokeRemoteRegistry                     │
│  graph_resume.go  — BuildGraphResumeMetadata                 │
│  capability_metadata.go — CapabilityMetadata                 │
│  invoke_workspace.go — ValidateAdminInvokeWorkspace          │
│  public_base_url.go — ResolvePublicBaseURL                   │
│  public_base_store.go — PublicBaseURLStore                   │
│  health/          — Runner（周期探测 + Prometheus）          │
├─────────────────────────────────────────────────────────────┤
│  internal/a2a/trpc（框架包装）                              │
│  agent.go         — BuildTRPCA2AAgent                        │
│  server.go        — BuildA2AEndpointServer                   │
│  registry.go      — EndpointRegistry                         │
│  auth.go          — ProxyClientAuthOptions                   │
├─────────────────────────────────────────────────────────────┤
│  Biz                                                         │
│  a2a.go（别名层）· a2a/a2a.go（核心领域）· a2a_limit.go      │
│  agent_kind.go                                               │
├─────────────────────────────────────────────────────────────┤
│  Data                                                        │
│  a2a.go — a2a_agent_cards / a2a_invocations / a2a_audit /   │
│           a2a_remote_agents                                  │
└─────────────────────────────────────────────────────────────┘
         ↓
pkg/trpc-agent-go — a2aagent、server/a2a（框架真相源）
```

### 2.1 平台内调用数据流

```
LLM Turn (call_agent 工具)
  → tool.go 校验 Card/capability
  → invokerFunc (NewInvoker)
       ├─ ResolveInvokeTarget → 本地 catalog agent
       │     → ChatService.RunAgentTurn
       └─ 或 remote registry id
             → InvokeRemoteRegistry (remote_client)
Admin POST /v1/a2a/invoke
  → ValidateAdminInvokeWorkspace
  → 同一 NewInvoker
  → FinishInvocation + audit + Prometheus
```

### 2.2 公开 Endpoint 数据流

```
GET/POST /v1/a2a/public/{agent_id}/...
  → EndpointRegistry.Get(agentID)
  → A2AEndpointBuilder.BuildHandler
       → catalog Agent (非 a2a_proxy) + Card.enabled
       → BuildA2AEndpointServer(runner, card, publicURL, streaming)
  → trpc-agent-go server/a2a 处理 A2A JSON-RPC / SSE
```

---

## 三、Proto 层（已实现）

文件：`api/kratos/a2a/v1/a2a.proto`

| RPC | HTTP | 职责 |
|-----|------|------|
| `Discover` | GET `/v1/a2a/discover` | 扁平列表（本地 Card + 远程 registry） |
| `GatewayDiscover` | GET `/v1/a2a/gateway/discover` | 联邦条目（source、healthy、endpoint_url） |
| `Invoke` | POST `/v1/a2a/invoke` | Admin 测试调用 |
| `UpdateAgentCard` / `GetAgentCard` | PUT/GET `.../card` | Card CRUD |
| `ListAudit` | GET `/v1/a2a/audit` | 审计 |
| `RegisterRemoteAgent` 等 | `/v1/a2a/remote-agents` | 远程注册 |
| `DiscoverRemoteAgent` | POST `/v1/a2a/remote-discover` | 预览 Card（不落库） |
| `GetA2AConfig` | GET `/v1/a2a/config` | 公开 URL + source |

`A2AAgentCard` 扩展字段：`source`、`endpoint_url`、`remote_url`（Discover/Gateway 展示用）。

Agent proto：`api/kratos/agent/v1/agent.proto` — `agent_kind`、`A2AProxyConfig`、`a2a_endpoint_enabled`（列表 enrichment）。

---

## 四、Biz 层（已实现）

### 4.1 核心模型 — `internal/biz/a2a.go`（别名层）+ `internal/biz/a2a/a2a.go`（领域真相）

- `a2a.go`（别名层）：`A2AUsecase = a2a.Usecase`、`A2AAgentCard`、`A2ARemoteAgent`、`A2ARepo` 等类型别名；`A2ARunnerFactory` 接口
- `a2a/a2a.go`（核心领域）：`Capability`、`AgentCard`、`Invocation`、`AuditEntry`、`RemoteAgent`、`GatewayEntry`；`CardRepo`、`InvocationRepo`、`AuditRepo`、`RemoteAgentRepo`、`Repo` 接口；`Usecase` 结构体及全部业务方法（Discover、Start/FinishInvocation、AppendAudit、RegisterRemoteAgent、DiscoverRemoteAgent、GatewayDiscover、PersistRemoteHealth、MapEndpointEnabled）
- `a2a/agent_lookup.go`：`AgentLookupAdapter`

### 4.2 速率限制 — `internal/biz/a2a_limit.go`

- `A2AInvokeLimiter`：基于 caller→callee 键的滑动窗口限流器
- 默认 60 次/分钟（`defaultA2ALimiter`）
- `A2AService.Invoke` 调用 `Allow()`，超限返回 429

### 4.3 Agent Kind — `internal/biz/agent_kind.go`

```go
const (
    AgentKindLLM      = "llm"
    AgentKindA2AProxy = "a2a_proxy"
)
type A2AProxyConfig struct {
    RemoteURL, AgentCardURL, AuthType, AuthConfigJSON string
    EnableStreaming bool
    TimeoutSeconds  int
}
```

- 持久化：`config_json` 内 `agent_kind` + `a2a_proxy`；`Kind` 字段与 Hydrate/Embed 同步
- `agent_kind` 更新不可变（BadRequest）

---

## 五、Data 层（已实现）

`internal/data/a2a.go` — SQLite，`EnsureA2ASchema`：

| 表 | 用途 |
|----|------|
| `a2a_agent_cards` | Card（agent_id PK, capabilities JSON, enabled） |
| `a2a_invocations` | 调用记录 |
| `a2a_audit` | 审计 |
| `a2a_remote_agents` | 远程注册 |

`GetAgentCard`：无行时返回 `ErrNotFound`（非 disabled 空卡）。

---

## 六、运行时与 Agent 构建（已实现）

### 6.1 call_agent

- 注入：`InjectRunContext(ctx, a2aUC, callerAgentID, NewInvoker(...))`
- 条件装配：`toolsets.go` 在 effective tools 含 `call_agent` 时注入

### 6.2 BuildTRPCAgent — `internal/agent/trpc_build_router.go`

```
kind "" | "llm"  → BuildTRPCLLMAgentCached
kind "a2a_proxy" → BuildTRPCA2AAgent (internal/a2a/trpc/agent.go)
```

- Proxy：跳过 intent pass（`trpc_turn.go`）
- Endpoint：仅 LLM + enabled Card；`A2AEndpointBuilder` 拒绝 Proxy

### 6.3 Invoker 职责边界（SRP）

| 模块 | 职责 |
|------|------|
| `card_validate.go` | Card enabled + capability 存在 |
| `callee_resolve.go` | 本地 vs 远程 target；disabled 不 fallback |
| `invoker.go` | 编排校验 + 调用 exec |
| `remote_invoke.go` | 远程 HTTP 消息发送 + capability metadata |
| `chat_native.go` | `AgentTurnRunner` 实现（会话创建 + Turn） |

---

## 七、Service 层（已实现）

`internal/service/a2a.go`：RPC 适配、Prometheus、`Invoke` 工作区校验 + 业务层限流（`A2AInvokeLimiter`）。

`internal/service/a2a_endpoint.go`：`A2AEndpointBuilder` 实现 `a2atrpc.EndpointBuilder`。

`internal/service/a2a_public_base.go`：`A2APublicBaseReloader`（系统设置保存后热更新 + 清 Endpoint 缓存）。

Wire：`NewA2AEndpointBuilder`、`NewA2APublicBaseReloader`。

---

## 八、Web 前端（已实现）

| 路径 | 说明 |
|------|------|
| `pages/A2APage.vue` | 运维主页 |
| `features/a2a/useA2APage.ts` | 页面编排（SRP：页面薄、逻辑在 composable） |
| `features/a2a/mappers.ts` + `__tests__/mappers.spec.ts` | DTO 映射 |
| `components/a2a/*` | Discover / Audit / Invoke / Remote / Banner 面板 |
| `stores/a2a/index.ts` | Pinia |
| `AgentSettingsA2ATab.vue` | Proxy 远程连接 |
| `AgentSettingsA2AEndpointTab.vue` | LLM Endpoint + Card |

数据流：`features/a2a/api.ts` → store / composable → 面板组件（符合 frontend-guide 分层）。

---

## 九、安全设计

| 控制点 | 实现 |
|--------|------|
| 默认关闭 | Card `enabled=false` |
| 工作区隔离 | Invoker + Admin Invoke `workspace` |
| 审计 | 每次调用 `a2a_audit` |
| Card 校验 | `CheckCalleeCard` |
| 公开路径 | 仅 enabled LLM Endpoint；免 JWT 前缀注册 |
| 远程鉴权 | api_key / bearer / mtls（`auth_config_json`） |
| 速率限制 | ✅ 业务层 `A2AInvokeLimiter`（60次/分钟）；HTTP/Ingress 层待做 |
| Server mTLS 终止 | 文档化；建议反向代理 |

---

## 十、涉及文件（代码锚点）

### 后端

| 文件 | 说明 |
|------|------|
| `api/kratos/a2a/v1/a2a.proto` | 契约 |
| `internal/biz/a2a.go`（别名层）· `internal/biz/a2a/a2a.go`（领域）· `a2a_limit.go` · `agent_kind.go` | 用例 |
| `internal/data/a2a.go` | 持久化 |
| `internal/service/a2a.go` · `a2a_endpoint.go` · `a2a_public_base.go` | 传输 |
| `internal/a2a/*.go` · `internal/a2a/health/*.go` · `internal/a2a/trpc/*.go` | 协议桥接 + 健康探测 |
| `internal/agent/trpc_build_router.go` | Kind 路由 |

### 前端

| 文件 | 说明 |
|------|------|
| `web/src/features/a2a/` | api · types · mappers · useA2APage · a2aTableUi · authUtils |
| `web/src/pages/A2APage.vue` | 路由页 |
| `web/src/components/a2a/` | 面板 |
| `web/src/components/agents/AgentSettingsA2A*.vue` | 设置 Tab |

---

## 十一、Agent Kind 与 Endpoint（已实现）

与需求 §2 产品模型一致：`a2a_proxy` 使用 `BuildTRPCA2AAgent`；LLM Endpoint 使用 `BuildA2AEndpointServer`。

- 消息转换：使用框架 `a2a_converter.go`，**禁止**复制到 `internal/`
- 公开 URL：`ResolvePublicBaseURL`；系统设置字段 `a2a_public_base_url` 经 `PublicBaseURLStore` 热更新

---

## 十二、传输与流式语义

> **结论：SSE 不是全平台唯一通道；按场景选型。**

### 12.1 三层传输

| 层 | 入口 | 传输 | 说明 |
|----|------|------|------|
| A2A 外部 | `/v1/a2a/public/{agent_id}` | A2A JSON-RPC + HTTP 流（SSE） | 遵循 A2A 规范；按 AgentCard `streaming` |
| Admin 运维 | `POST /v1/a2a/invoke` | 一元 JSON | 聚合最终文本，非 token 流 |
| Chat UI | `/v1/ws` | WebSocket Envelope | 平台主通道；非标准 A2A |

### 12.2 流式矩阵

| 场景 | 配置 | 框架 |
|------|------|------|
| Public Endpoint | Card streaming + `BuildA2AEndpointServer(..., streaming)` | `server/a2a` |
| A2A Proxy Chat | `A2AProxyConfig.enable_streaming` | `a2aagent` |
| call_agent / Admin Invoke | 非流式 | `RunAgentTurn` 聚合 |

### 12.3 Graph Resume

- Public Endpoint：`server/a2a` 内置 `GraphResumeStateFromMetadata`
- 项目：`BuildGraphResumeMetadata`（flattened 根级字段，与框架 envelope 对齐）

### 12.4 GatewayDiscover vs Discover

| API | 用途 |
|-----|------|
| `Discover` | 运维/工具扁平列表 |
| `GatewayDiscover` | 联邦：`source`、`endpoint_url`、`remote_url`、`healthy` |

远程 registry 的 `agent_id` = registry `id`；`ResolveInvokeTarget` 路由至 `InvokeRemoteRegistry`。

### 12.5 公开 URL 优先级

```
env (A2A_PUBLIC_BASE_URL)
  > system_settings.a2a_public_base_url (DB，/settings 编辑，保存即生效)
  > configs server.a2a_public_base_url
  > derived (HTTP listen addr)
```

- 只读：`GET /v1/a2a/config` + `/a2a` `A2ARuntimeConfigBanner`

---

## 十三、Phase 4 设计预留

| 项 | 状态 | 方向 |
|----|------|------|
| 健康 Cron | ✅ 已实现 | `internal/a2a/health/runner.go`；默认 10 分钟间隔；`A2A_HEALTH_DISABLED=1` 可禁用；Prometheus `aranea_a2a_gateway_healthy` + `aranea_a2a_health_probe_total` |
| 联邦路由 | ❌ 待实现 | `GatewayDiscover` 消费 healthy 标记选路 |
| 速率限制 | 🟡 部分实现 | 业务层 `A2AInvokeLimiter`（60次/分钟）已实现；HTTP/Ingress 层待做 |
| Admin Invoke 流式 | ❌ 待实现 | 可选 SSE；默认保持非流式 |

实现任务与验收见 [26-a2a-development.md](./26-a2a-development.md) Phase 4。
