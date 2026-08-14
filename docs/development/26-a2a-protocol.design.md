# A2A 协议模块 — 实现设计文档

> 对应需求：[26-a2a-protocol.md](./26-a2a-protocol.md)
> 遵循规范：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)
> 关联：Agent 创建 [2-agents-create.design.md](./2-agents-create.design.md) · Agent 设置 [5-agent-setting.design.md](./5-agent-setting.design.md)
> 进度差距：[26-a2a-protocol.development.md](./26-a2a-protocol.development.md)

---

## 一、模块概述

Agent-to-Agent：平台内 `call_agent`、Admin Invoke、LLM **公开 Endpoint**、`a2a_proxy` 远程代理、工作区远程注册与联邦 Discover。

**设计目标**：

1. **biz 不 import trpc-agent-go** — 领域在 `internal/biz/a2a/`；框架包装在 `internal/a2a/trpc`
2. **单一 Invoker 派发** — tool 与 HTTP Invoke 共用 `NewInvoker` + `RunAgentTurn` / `InvokeRemoteRegistry`
3. **Agent Kind 路由** — `BuildTRPCAgent` 分发 `llm` | `a2a_proxy`
4. **传输三分工** — A2A HTTP（外部）/ Admin JSON（运维）/ WebSocket（Chat UI）

**与 Agent 模块边界**：`agent_kind`、`A2AProxyConfig` 持久化见 `internal/biz/agent_kind.go`；创建/设置 UI 见 Agent 模块设计文档。

---

## 二、分层架构

```
┌─────────────────────────────────────────────────────────────┐
│  Server                                                      │
│  http.go — A2AService + HandlePrefix(/v1/a2a/public/...)     │
│         — HandlePrefix(/a2a) A2AExtensionCompatService       │
│  auth — RegisterNoAuthPathPrefix 免 Admin JWT                │
├─────────────────────────────────────────────────────────────┤
│  Service                                                     │
│  a2a.go                — Discover/Invoke/Card/Audit/Remote/  │
│                          Gateway + Prometheus + 限流          │
│  a2a_endpoint.go       — A2AEndpointBuilder → 公开 handler   │
│  a2a_public_base.go    — A2APublicBaseReloader（热更新+清缓存）│
│  a2a_extension_compat.go — A2AExtensionCompatService（/a2a）  │
│  chat_native.go        — RunAgentTurn（AgentTurnRunner 实现） │
│  chat_orchestrator_turn_dispatch.go — injectA2AContext        │
│  trpc_turn.go          — runCtx 注入 A2A；Proxy 跳过 intent   │
├─────────────────────────────────────────────────────────────┤
│  internal/a2a（协议桥接，可 import biz，不 import trpc 运行时）│
│  tool.go              — call_agent                            │
│  invoker.go           — NewInvoker、工作区校验、InjectRunContext│
│  callee_resolve.go    — ResolveInvokeTarget                   │
│  card_validate.go     — CheckCalleeCard                       │
│  remote_client.go     — FetchRemoteAgentCard、MTLSHTTPClient、│
│                          ClientAuthOptions                    │
│  remote_invoke.go     — InvokeRemoteRegistry                  │
│  capability_metadata.go — CapabilityMetadata                  │
│  invoke_workspace.go  — ValidateAdminInvokeWorkspace          │
│  public_base_url.go   — ResolvePublicBaseURL                  │
│  public_base_store.go — PublicBaseURLStore                    │
│  health/              — Runner（周期探测 + Prometheus）       │
├─────────────────────────────────────────────────────────────┤
│  internal/a2a/trpc（框架包装）                                │
│  agent.go             — BuildTRPCA2AAgent                     │
│  server.go            — BuildA2AEndpointServer                │
│  registry.go          — EndpointRegistry + PublicPathPrefix   │
│  auth.go              — ProxyClientAuthOptions                │
├─────────────────────────────────────────────────────────────┤
│  Biz                                                         │
│  a2a.go（别名层）· a2a/a2a.go（核心领域）· a2a/limiter*.go    │
│  a2a/agent_lookup.go · agent_kind.go                         │
├─────────────────────────────────────────────────────────────┤
│  Data                                                        │
│  a2a.go — EnsureA2ASchema（raw SQL）：                       │
│           a2a_agent_cards / a2a_invocations / a2a_audit /   │
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
  → Limiter.Allow（fail-closed）
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

### 2.3 A2A Extension Compat 数据流

```
GET/POST /a2a/...
  → A2AExtensionCompatService.Handler(ctx)（lazy init）
       → OpenAIRunnerBuilder.BuildOpenAIRunner（默认 agent key）
       → trpca2a.New(WithRunner, WithAgentCard, WithSessionService, WithHost)
  → 框架 A2A server 处理 JSON-RPC / SSE
```

> `A2AExtensionCompatService`（`internal/service/a2a_extension_compat.go`）是独立于 `/v1/a2a/public/{agent_id}` 的框架级 A2A 扩展端点，路径 `/a2a`，懒初始化，默认 AgentCard 广告 streaming + text 模式。与 OpenAICompatService 同构。

---

## 三、Proto 层

文件：`api/kratos/a2a/v1/a2a.proto`

| RPC | HTTP | 职责 |
|-----|------|------|
| `Discover` | GET `/v1/a2a/discover` | 扁平列表（本地 Card + 远程 registry） |
| `GatewayDiscover` | GET `/v1/a2a/gateway/discover` | 联邦条目（source、healthy、endpoint_url） |
| `Invoke` | POST `/v1/a2a/invoke` | Admin 测试调用 |
| `UpdateAgentCard` / `GetAgentCard` | PUT/GET `/v1/a2a/agents/{agent_id}/card` | Card CRUD |
| `ListAudit` | GET `/v1/a2a/audit` | 审计 |
| `RegisterRemoteAgent` / `ListRemoteAgents` / `DeleteRemoteAgent` | POST/GET/DELETE `/v1/a2a/remote-agents` | 远程注册 |
| `DiscoverRemoteAgent` | POST `/v1/a2a/remote-discover` | 预览 Card（不落库） |
| `GetA2AConfig` | GET `/v1/a2a/config` | 公开 URL + source |

`A2AAgentCard` 扩展字段：`source`、`endpoint_url`、`remote_url`（Discover/Gateway 展示用）。
`A2ARemoteAgent` 健康字段：`healthy`、`last_health_at`（由网关健康探测填充）。
`A2AGatewayEntry`：`card`、`source`、`registry_id`、`endpoint_url`、`remote_url`、`healthy`。

Agent proto：`api/kratos/agent/v1/agent.proto` — `agent_kind`、`A2AProxyConfig`、`a2a_endpoint_enabled`（列表 enrichment）。

### 3.1 消息格式（Invoke）

**请求**（字段名以 proto 为准）：

```json
{
  "callee_agent_id": "agent-456",
  "capability": "summarize",
  "payload_json": "{\"text\": \"...\"}",
  "caller_session_id": "sess-789",
  "timeout_seconds": 30,
  "workspace": "workspace-A"
}
```

**响应**：

```json
{
  "invoke_id": "a2a-xxxxxxxx",
  "status": "success",
  "result_json": "{}",
  "error_message": "",
  "duration_ms": 142
}
```

---

## 四、Biz 层

### 4.1 核心模型 — `internal/biz/a2a.go`（别名层）+ `internal/biz/a2a/a2a.go`（领域真相）

- `a2a.go`（别名层）：`A2AUsecase = a2a.Usecase`、`A2AAgentCard`、`A2ARemoteAgent`、`A2ARepo`、`A2ACardRepo`、`A2AInvocationRepo`、`A2AAuditRepo`、`A2ARemoteAgentRepo` 等类型别名；`A2ARunnerFactory` 接口；`A2AAgentLookup`、`A2AAgentMeta`、`A2ARetryPolicy` 别名；`NewA2AID`、`NewAgentLookupAdapter`、`A2ADefaultRetryPolicy`、`A2ASourceLocal/Remote` 变量别名
- `a2a/a2a.go`（核心领域）：`Capability`、`AgentCard`、`Invocation`、`AuditEntry`、`RemoteAgent`、`GatewayEntry`、`RetryPolicy`；`CardRepo`、`InvocationRepo`、`AuditRepo`、`RemoteAgentRepo` 接口；`Repo` 聚合接口（**Deprecated**，消费者应依赖各子接口）；`Usecase` 结构体及全部业务方法（Discover、Start/FinishInvocation、AppendAudit、RegisterRemoteAgent、DiscoverRemoteAgent、GatewayDiscover、PersistRemoteHealth、MapEndpointEnabled）
- `a2a/agent_lookup.go`：`AgentLookupAdapter`（将 broader agent reader 适配为 `AgentLookup`，避免 a2a 包 import 完整 Agent 类型）

**接口稳定性标注**：

| 接口 | 稳定性 |
|------|--------|
| `CardRepo` | `// Stability:stable` |
| `InvocationRepo` | `// Stability:stable` |
| `AuditRepo` | `// Stability:stable` |
| `RemoteAgentRepo` | `// Stability:evolving` |
| `Repo`（聚合） | `Deprecated` |

### 4.2 速率限制 — `internal/biz/a2a/limiter*.go`

限流器已重构为接口 + 多后端实现 + 工厂模式：

| 文件 | 职责 |
|------|------|
| `limiter.go` | `Limiter` 接口（`Allow(ctx, caller, callee) (bool, error)`）+ `LimiterConfig`（WindowSize/MaxInvokes/KeyPrefix）+ `DefaultLimiterConfig()`（60次/60s） |
| `limiter_memory.go` | `MemorySlidingWindowLimiter` — 单进程内存滑动窗口 |
| `limiter_redis.go` | `RedisSlidingWindowLimiter` — 分布式 Redis 滑动窗口（原子操作） |
| `limiter_factory.go` | `NewLimiter(cfg, client, lg)` — 工厂：`client != nil` 返回 Redis 实现，否则返回内存实现并 Warn 提示多 Pod 风险 |

- `A2AService.Invoke` 调用 `Limiter.Allow(ctx, callerKey, calleeID)`，**fail-closed**：limiter 报错时拒绝并返回 429（`apierror.RateLimit`）
- 默认 60 次/分钟（`DefaultLimiterConfig`）
- Redis 可用时自动切换分布式限流；KeyPrefix 默认 `aranea:a2a:invoke:`

### 4.3 远程调用重试 — `RetryPolicy`

`internal/biz/a2a/a2a.go` 定义 `RetryPolicy`：

```go
type RetryPolicy struct {
    MaxRetries     int           // 默认 2
    InitialBackoff time.Duration // 默认 500ms
    MaxBackoff     time.Duration // 默认 5s
}
const DefaultRemoteInvokeTimeoutSec = 30
```

- `NewInvoker` 接收 `retryPolicy a2abiz.RetryPolicy` 参数，传递给 `InvokeRemoteRegistry`
- `DefaultRetryPolicy()` 返回生产默认值
- `A2AService.Invoke` 使用 `a2abiz.DefaultRetryPolicy()`

### 4.4 Agent Kind — `internal/biz/agent_kind.go`

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

- 持久化：`config_json` 内 `agent_kind` + `a2a_proxy`；`Kind` 字段与 `HydrateAgentKind`/`EmbedAgentKindInConfigJSON` 同步
- `NormalizeAgentKind`：`""`/`"llm"`/`"open"` → `llm`；`"a2a_proxy"`/`"a2a"`/`"a2a-proxy"` → `a2a_proxy`
- `agent_kind` 更新不可变（BadRequest）

---

## 五、Data 层

`internal/data/a2a.go` — Raw SQL（**非 Ent Schema**），`EnsureA2ASchema` 建表：

| 表 | 用途 | 关键字段 |
|----|------|---------|
| `a2a_agent_cards` | Card | agent_id PK, capabilities JSON, enabled |
| `a2a_invocations` | 调用记录 | id PK, status, result_json, duration_ms |
| `a2a_audit` | 审计 | id PK, invoke_id, caller/callee, workspace |
| `a2a_remote_agents` | 远程注册 | id PK, auth_config_json, card_json, last_health_at/ok/error |

> **TECH-DEBT(debt): DEV-10** — A2A schema 迁移应移至正式迁移框架（如 golang-migrate）。当前 raw `ALTER TABLE ADD COLUMN` + 吞掉 duplicate-column 错误的方式脆弱且无版本号。`a2a_remote_agents` 的 `last_health_at`/`last_health_ok`/`last_health_error` 列通过此方式补丁添加。

`a2aRepo` 实现 `biza2a.Repo`/`CardRepo`/`InvocationRepo`/`AuditRepo`/`RemoteAgentRepo` 全部接口。`GetAgentCard`：无行时返回 `ErrNotFound`（非 disabled 空卡）。所有错误经 `entErrToBizErr(err, "A2A")` 翻译。

---

## 六、运行时与 Agent 构建

### 6.1 call_agent

- 注入：`InjectRunContext(ctx, a2aUC, callerAgentID, NewInvoker(...))`（`internal/a2a/invoker.go`）
- 调用点：`ChatOrchestrator.injectA2AContext`（`internal/service/chat_orchestrator_turn_dispatch.go`），在 turn phase 中调用（`chat_orchestrator_turn_phases.go`）
- 条件装配：`toolsets.go` 在 effective tools 含 `call_agent` 时注入

### 6.2 BuildTRPCAgent — `internal/agent/trpc_build_router.go`

```
kind "" | "llm"  → BuildTRPCLLMAgentCached
kind "a2a_proxy" → BuildTRPCA2AAgent (internal/a2a/trpc/agent.go)
```

- `BuildTRPCAgentCached`：a2a_proxy 不走缓存路径，直接调 `BuildTRPCAgent`
- Proxy：跳过 intent pass（`trpc_turn.go`）
- Endpoint：仅 LLM + enabled Card；`A2AEndpointBuilder` 拒绝 Proxy

### 6.3 Invoker 职责边界（SRP）

| 模块 | 职责 |
|------|------|
| `card_validate.go` | Card enabled + capability 存在 |
| `callee_resolve.go` | 本地 vs 远程 target；disabled 不 fallback |
| `invoker.go` | 编排校验 + 调用 exec；接收 RetryPolicy |
| `remote_invoke.go` | 远程 HTTP 消息发送 + capability metadata + 重试 |
| `chat_native.go` | `AgentTurnRunner` 实现（`RunAgentTurn`，会话创建 + Turn） |

---

## 七、Service 层

| 文件 | 职责 |
|------|------|
| `internal/service/a2a.go` | `A2AService` RPC 适配、Prometheus 指标、`Invoke` 工作区校验 + `Limiter` 限流（fail-closed） |
| `internal/service/a2a_endpoint.go` | `A2AEndpointBuilder` 实现 `a2atrpc.EndpointBuilder`；`ChatService.BuildA2ARunner` |
| `internal/service/a2a_public_base.go` | `A2APublicBaseReloader`（系统设置保存后热更新 + 清 Endpoint 缓存） |
| `internal/service/a2a_extension_compat.go` | `A2AExtensionCompatService`（`/a2a` 框架级 A2A 扩展端点，懒初始化） |

Wire：`NewA2AService`、`NewA2AEndpointBuilder`、`NewA2APublicBaseReloader`、`NewA2AExtensionCompatService`、`NewLimiter`（limiter 工厂）。

---

## 八、Web 前端

| 路径 | 说明 |
|------|------|
| `pages/A2APage.vue` | 运维主页 |
| `features/a2a/useA2APage.ts` | 页面编排（SRP：页面薄、逻辑在 composable） |
| `features/a2a/api.ts` | API 调用层 |
| `features/a2a/types.ts` | DTO 类型 |
| `features/a2a/mappers.ts` + `__tests__/mappers.spec.ts` | DTO 映射 + 单测 |
| `features/a2a/a2aTableUi.ts` | 表格 UI 辅助 |
| `features/a2a/authUtils.ts` | 鉴权工具 |
| `stores/a2a/index.ts` | Pinia store |
| `components/a2a/A2ADiscoverPanel.vue` | 发现面板 |
| `components/a2a/A2AAuditPanel.vue` | 审计面板 |
| `components/a2a/A2AInvokePanel.vue` | Invoke 测试面板 |
| `components/a2a/A2ARemoteAgentPanel.vue` | 远程注册面板 |
| `components/a2a/A2AGatewayPanel.vue` | 联邦 Gateway 面板 |
| `components/a2a/A2ARuntimeConfigBanner.vue` | 运行时配置 Banner |
| `components/agents/AgentSettingsA2ATab.vue` | Proxy 远程连接设置 |
| `components/agents/AgentSettingsA2AEndpointTab.vue` | LLM Endpoint + Card 设置 |

数据流：`features/a2a/api.ts` → store / composable → 面板组件（符合 frontend-guide 分层）。

---

## 九、安全设计

| 控制点 | 实现 |
|--------|------|
| 默认关闭 | Card `enabled=false` |
| 工作区隔离 | Invoker + Admin Invoke `workspace` 字段（`ValidateAdminInvokeWorkspace`） |
| 审计 | 每次调用写 `a2a_audit` |
| Card 校验 | `CheckCalleeCard`（`card_validate.go`） |
| 公开路径 | 仅 enabled LLM Endpoint；`RegisterNoAuthPathPrefix(PublicPathPrefix)` 免 JWT |
| 远程鉴权 | `ClientAuthOptions`（`remote_client.go`）支持 `none` / `api_key` / `bearer` / `mtls` |
| 速率限制 | `Limiter` 接口（Redis 优先，内存兜底）；`A2AService.Invoke` fail-closed 返回 429 |
| Server mTLS 终止 | 文档化；建议反向代理/Ingress |

### 9.1 客户端鉴权类型

`internal/a2a/remote_client.go` `ClientAuthOptions` 实际支持：

| auth_type | 处理 | auth_config_json 字段 |
|-----------|------|----------------------|
| `""` / `none` | 无鉴权 | 忽略 |
| `api_key` / `apikey` | API Key header | `api_key` 或 `token`，`header_name`（默认 `X-Api-Key`） |
| `bearer` | Bearer token | `token`，`header_name`（默认 `Authorization`，自动加 `Bearer ` 前缀） |
| `mtls` | mTLS 客户端证书 | `cert_file` + `key_file`（必填），`ca_file`（可选） |

> biz 层 `ValidateAuthConfig`（`internal/biz/a2a/a2a.go`）与 `remote_client.go` 支持集合一致：`none` / `api_key` / `bearer` / `mtls`。`api_key` 与 `bearer` 均接受 `api_key` 或 `token` 字段（互备）；`basic` 已移除（运行时不支持）。

---

## 十、涉及文件（代码锚点）

### 后端

| 文件 | 说明 |
|------|------|
| `api/kratos/a2a/v1/a2a.proto` | 契约 |
| `internal/biz/a2a.go`（别名层）· `internal/biz/a2a/a2a.go`（领域）· `a2a/limiter*.go` · `a2a/agent_lookup.go` · `internal/biz/agent_kind.go` | 用例 + 限流 + Agent Kind |
| `internal/data/a2a.go` | 持久化（Raw SQL） |
| `internal/service/a2a.go` · `a2a_endpoint.go` · `a2a_public_base.go` · `a2a_extension_compat.go` | 传输 |
| `internal/a2a/*.go` · `internal/a2a/health/*.go` · `internal/a2a/trpc/*.go` | 协议桥接 + 健康探测 |
| `internal/agent/trpc_build_router.go` | Kind 路由 |
| `internal/service/chat_native.go` · `chat_orchestrator_turn_dispatch.go` · `trpc_turn.go` | Turn 派发 + A2A 注入 |
| `internal/server/http.go` | 路由注册（`/v1/a2a/public/` + `/a2a`） |

### 前端

| 文件 | 说明 |
|------|------|
| `web/src/features/a2a/` | api · types · mappers · useA2APage · a2aTableUi · authUtils |
| `web/src/pages/A2APage.vue` | 路由页 |
| `web/src/components/a2a/` | Discover / Audit / Invoke / Remote / Gateway / Banner 面板 |
| `web/src/components/agents/AgentSettingsA2A*.vue` | 设置 Tab |
| `web/src/stores/a2a/index.ts` | Pinia store |

---

## 十一、Agent Kind 与 Endpoint

与需求 §1 产品模型一致：`a2a_proxy` 使用 `BuildTRPCA2AAgent`；LLM Endpoint 使用 `BuildA2AEndpointServer`。

- 消息转换：使用框架 `a2a_converter.go`，**禁止**复制到 `internal/`
- 公开 URL：`ResolvePublicBaseURL`；系统设置字段 `a2a_public_base_url` 经 `PublicBaseURLStore` 热更新

---

## 十二、传输与流式语义

> **结论：SSE 不是全平台唯一通道；按场景选型。**

### 12.1 三层传输

| 层 | 入口 | 传输 | 说明 |
|----|------|------|------|
| A2A 外部 | `/v1/a2a/public/{agent_id}` | A2A JSON-RPC + HTTP 流（SSE） | 遵循 A2A 规范；按 AgentCard `streaming` |
| A2A Extension | `/a2a` | A2A JSON-RPC + SSE | 框架级扩展端点（`A2AExtensionCompatService`，懒初始化） |
| Admin 运维 | `POST /v1/a2a/invoke` | 一元 JSON | 聚合最终文本，非 token 流 |
| Chat UI | `/v1/ws` | WebSocket Envelope | 平台主通道；非标准 A2A |

### 12.2 流式矩阵

| 场景 | 配置 | 框架 |
|------|------|------|
| Public Endpoint | Card streaming + `BuildA2AEndpointServer(..., streaming)` | `server/a2a` |
| A2A Extension | 默认 AgentCard `streaming=true` | `trpca2a.New` |
| A2A Proxy Chat | `A2AProxyConfig.enable_streaming` | `a2aagent` |
| call_agent / Admin Invoke | 非流式 | `RunAgentTurn` 聚合 |

### 12.3 Graph Resume

- Public Endpoint：`server/a2a` 内置 `GraphResumeStateFromMetadata`
- 项目：`BuildGraphResumeMetadata`（flattened 根级字段，与框架 envelope 对齐）

### 12.4 GatewayDiscover vs Discover

| API | 用途 |
|-----|------|
| `Discover` | 运维/工具扁平列表 |
| `GatewayDiscover` | 联邦：`source`、`endpoint_url`、`remote_url`、`healthy`；支持 `check_health` 实时探测 |

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

## 十三、网关健康探测

`internal/a2a/health/runner.go`：

| 项 | 说明 |
|----|------|
| 触发 | `Runner.Start(ctx, interval)`；默认 10 分钟间隔 |
| 间隔配置 | 环境变量 `A2A_HEALTH_INTERVAL`（解析失败或 ≤0 回退默认 10min） |
| 禁用 | 环境变量 `A2A_HEALTH_DISABLED=1`（`cmd/admin/wire.go` `provideA2AHealthRunner` 返回 nil） |
| 并发 | `sync.Mutex` `TryLock` 防重入 |
| 持久化 | `Usecase.PersistRemoteHealth` → `RemoteAgentRepo.UpdateRemoteAgentHealth` |

### 13.1 Prometheus 指标

| 指标 | 类型 | 说明 |
|------|------|------|
| `aranea_a2a_invoke_total` | CounterVec | 调用次数（labels: caller/callee/status） |
| `aranea_a2a_invoke_duration_seconds` | Histogram | 延迟直方图 |
| `aranea_a2a_gateway_healthy` | GaugeVec | 网关健康状态（labels: registry_id/workspace；1=健康/0=不健康） |
| `aranea_a2a_health_probe_total` | CounterVec | 健康探测次数（labels: registry_id/status） |

---

## 十四、依赖方向

```
api/kratos/a2a/v1/*.proto
  → internal/service (A2AService, A2AEndpointBuilder, A2APublicBaseReloader, A2AExtensionCompatService)
  → internal/biz (A2AUsecase, Limiter, RetryPolicy, Gateway)
  → internal/data (SQLite Raw SQL)
internal/a2a/trpc → pkg/trpc-agent-go (a2aagent, server/a2a)
internal/service/chat_orchestrator_turn_dispatch → internal/a2a (InjectRunContext, Invoker)
```

**不变量**：biz 不 import `pkg/trpc-agent-go`；框架包装隔离在 `internal/a2a/trpc`。

---

## 子模块：联邦 A2A 网络

> 2026-07-28 立项评审。原 `phase5-差异化创新/04-联邦A2A网络.md` 的设计按代码现状修订后并入本节。

### F.1 评审修订记录（原规划 → 落地设计）

| # | 原规划 | 修订 | 理由 |
|---|--------|------|------|
| 1 | 新建 `internal/federation/` 顶级包（registry/trust/audit/quota/security 5 子包） | 联邦领域逻辑全部放 `internal/biz/a2a/federation_*.go` | TrustManager/PolicyEngine/AuditLogger/QuotaChecker 为纯领域逻辑，按分层规范属 biz 层，与 `limiter*.go` 同模式；5 子包属过度拆分（CS-B18） |
| 2 | 新增 `FederationAgent` 表 | 复用 `a2a_remote_agents` + 新增 `org_id` 列关联组织 | 远程 Agent 的认证/健康/重试/Card 缓存链路已存在，重复建表浪费 |
| 3 | `internal/federation/quota/rate_limiter.go` | 复用 `a2a.Limiter`（QPS）+ 审计表当日计数（日配额） | 零新存储；Limiter 已支持 Redis/内存双实现 |
| 4 | `internal/federation/security/`（mTLS + OAuth2） | 复用 `ClientAuthOptions`（none/api_key/bearer/mtls）；OAuth2 移出本期 | 现有 4 种认证已覆盖需求 FED-F8 |
| 5 | 扩展 `ResolveInvokeTarget` / `InvokeRemoteRegistry` 支持联邦目标 | 不改主路径；联邦调用走 `FederationService.InvokeFederatedAgent` 专用入口，内部新增 `internal/a2a/federation_invoke.go` 编排并复用 `InvokeRemoteRegistry` | `call_agent`/Admin Invoke 语义保持不变（本地 + 工作区远程注册表）；YAGNI |
| 6 | 前端新建联邦管理页面 | A2APage 新增「联邦」Tab | 子模块定位，避免页面膨胀 |
| 7 | 审批流（PolicyAction=approval） | 保留枚举，不实现审批链路 | 审批需要人机交互回路，复杂度超出本期 |

### F.2 分层架构

```
api/kratos/a2a/v1/federation.proto           ← 新增：FederationService（9 RPC）
        ↓
internal/service/a2a_federation.go           ← 新增：RPC 适配（proto ↔ biz，无业务逻辑）
        ↓
internal/biz/a2a/federation.go               ← 领域模型 + 窄接口
internal/biz/a2a/federation_trust.go         ← TrustManager
internal/biz/a2a/federation_policy.go        ← PolicyEngine（内存缓存 + 失效）
internal/biz/a2a/federation_quota.go         ← QuotaChecker（Limiter + 日计数）
internal/biz/a2a/federation_audit.go         ← AuditLogger（决策/结果语义）
internal/biz/a2a/federation_directory.go     ← Directory + AgentCardSync
internal/biz/a2a/federation_usecase.go       ← FederationUsecase（治理链编排）
        ↓
internal/data/ent/schema/federation_org.go        ← Ent Schema（3 张新表）
internal/data/ent/schema/federation_policy.go
internal/data/ent/schema/federation_audit_log.go
internal/data/a2a_federation_repo.go              ← Ent Repo 实现
        ↓
internal/a2a/federation_invoke.go            ← 联邦调用编排（复用 InvokeRemoteRegistry）
```

依赖方向不变量：biz 不 import `pkg/trpc-agent-go`；`internal/a2a` 为协议桥接层；Wire 绑定在 service 层。

### F.3 领域模型（`internal/biz/a2a/federation.go`）

```go
// 信任等级
const (
    TrustLevelTrusted   = "trusted"
    TrustLevelNeutral   = "neutral"
    TrustLevelUntrusted = "untrusted"
)

// 策略动作（approval 保留枚举，本期不实现审批链路）
const (
    PolicyActionAllow    = "allow"
    PolicyActionDeny     = "deny"
    PolicyActionApproval = "approval"
)

// 审计决策
const (
    DecisionAllowed      = "allowed"
    DecisionDeniedTrust  = "denied_trust"
    DecisionDeniedPolicy = "denied_policy"
    DecisionDeniedQuota  = "denied_quota"
)

// 审计方向（本期仅 outbound 落地）
const (
    AuditDirectionOutbound = "outbound"
    AuditDirectionInbound  = "inbound"
)

// 组织状态
const (
    OrgStatusActive    = "active"
    OrgStatusSuspended = "suspended"
)

// FederationLocalOrgID 表示本组织（出站方向的 caller）。
// 策略与审计中 caller_org_id = "local" 即本组织；前端映射为「本组织」。
const FederationLocalOrgID = "local"

type FederationOrg struct {
    ID             string
    Name           string
    Domain         string // 唯一约束
    PublicBaseURL  string
    TrustLevel     string
    AuthType       string // 复用 AuthType* 常量
    AuthConfigJSON string
    Status         string
    JoinedAt       time.Time
    UpdatedAt      time.Time
}

type FederationPolicy struct {
    ID          string
    CallerOrgID string // FederationLocalOrgID = 本组织（出站策略）
    CalleeOrgID string
    Action      string
    MaxPerMin   int // 每分钟调用上限（与 Limiter 滑动窗口语义对齐）；0 = 不限
    DailyQuota  int // 每日调用上限（按 decision=allowed 计数）；0 = 不限
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type FederationAuditLog struct {
    ID            string
    Direction     string
    CallerOrgID   string
    CalleeOrgID   string
    CallerAgentID string
    CalleeAgentID string
    Capability    string
    Decision      string
    Status        string // pending | success | error | timeout
    LatencyMs     int64
    ErrorMessage  string
    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

窄接口（≤5 方法，CS-B4）：

```go
// FederationOrgRepo 组织持久化。
// Stability:evolving
type FederationOrgRepo interface {
    UpsertOrg(ctx context.Context, org FederationOrg) (FederationOrg, error) // 按 domain upsert
    GetOrg(ctx context.Context, id string) (FederationOrg, error)
    ListOrgs(ctx context.Context) ([]FederationOrg, error)
    UpdateOrgTrust(ctx context.Context, id, trustLevel string) error
    DeleteOrg(ctx context.Context, id string) error
}

// FederationPolicyRepo 策略持久化。
// Stability:evolving
type FederationPolicyRepo interface {
    UpsertPolicy(ctx context.Context, p FederationPolicy) (FederationPolicy, error)
    GetPolicy(ctx context.Context, callerOrgID, calleeOrgID string) (FederationPolicy, error)
    ListPolicies(ctx context.Context) ([]FederationPolicy, error)
    DeletePolicy(ctx context.Context, id string) error
}

// FederationAuditRepo 审计持久化。
// Stability:evolving
type FederationAuditRepo interface {
    CreateAudit(ctx context.Context, log FederationAuditLog) (FederationAuditLog, error)
    UpdateAuditResult(ctx context.Context, id, status string, latencyMs int64, errMsg string) error
    ListAudits(ctx context.Context, filter FederationAuditFilter) ([]FederationAuditLog, int, error)
    CountCallsSince(ctx context.Context, callerOrgID, calleeOrgID string, since time.Time) (int, error)
}
```

### F.4 数据模型（Ent Schema + DDL 迁移）

**新表（Ent Schema，遵守 DB-R3）**：

| 表 | 关键列 | 约束/索引 |
|----|--------|-----------|
| `federation_orgs` | id / name / domain / public_base_url / trust_level / auth_type / auth_config_json / status / joined_at / updated_at | `domain` 唯一索引；`auth_config_json` 标 `.Sensitive()`（DB-N8） |
| `federation_policies` | id / caller_org_id / callee_org_id / action / max_per_min / daily_quota / created_at / updated_at | `(caller_org_id, callee_org_id)` 唯一索引 |
| `federation_audit_logs` | id / direction / caller_org_id / callee_org_id / caller_agent_id / callee_agent_id / capability / decision / status / latency_ms / error_message / created_at / updated_at | 索引 `(callee_org_id, created_at)`、`(caller_org_id, created_at)`（审计查询 + 日配额计数） |

**既有表迁移（DDL Migration Registry 版本化）**：

| 表 | 变更 | 说明 |
|----|------|------|
| `a2a_remote_agents` | `ADD COLUMN org_id TEXT NOT NULL DEFAULT ''` | 关联联邦组织；空 = 工作区级远程 Agent（非联邦） |

**既有模型扩展**：`a2a.RemoteAgent` + `RegisterRemoteAgentInput` 增加 `OrgID` 字段（可选）；scan/insert 同步。**不新增** `RemoteAgentRepo` 接口方法——联邦目录用 `ListRemoteAgents(workspace)` 内存过滤 `OrgID`（远程注册表规模 <<1000，YAGNI）。

### F.5 治理组件语义

**TrustManager**（`federation_trust.go`）：

| 信任等级 | 调用判定 |
|----------|----------|
| trusted | 允许（仍受策略/配额约束） |
| neutral | 允许（仍受策略/配额约束） |
| untrusted | 拒绝（403，`denied_trust`） |

**PolicyEngine**（`federation_policy.go`）：内存缓存 `map[caller→callee]Policy`（`sync.RWMutex`），启动时 `ListPolicies` 全量加载；`UpsertPolicy`/`DeletePolicy` 写库后同步刷新缓存。Evaluate 顺序：显式策略（精确匹配 caller+callee）> 信任等级默认。`approval` 按 `deny` 处理并 Warn（本期无审批链路）。

**QuotaChecker**（`federation_quota.go`）：
- `MaxPerMin > 0`：复用 `Limiter.Allow(ctx, "fed:"+callerOrgID, calleeOrgID)`（滑动窗口每分钟语义）
- `DailyQuota > 0`：`AuditRepo.CountCallsSince(caller, callee, 当日 0 点 UTC)`（仅计 `decision=allowed`）≥ DailyQuota 则拒绝（429，`denied_quota`）
- 均为 0 直接放行

**AuditLogger**（`federation_audit.go`）：
- `RecordAllowed`：同步创建决策审计（decision=allowed + status=pending）；**创建失败 fail-closed**（FED-NFR1 审计完整性），返回 500 并拒绝调用
- `RecordResult`：调用结束后更新 status/latency/error；**失败仅 Warn 不阻断**（结果已产生，不因审计更新失败改写调用结果）
- `RecordDenied`：被拒绝的调用（trust/policy/quota）写决策审计（denied_trust/denied_policy/denied_quota）；此处创建失败不 fail-closed（调用本已被拒），仅 Error 日志

**Directory**（`federation_directory.go`）：
- `ListFederationAgents(ctx, capability, orgID)`：`ListOrgs` 过滤 `trust != untrusted && status == active`（可选按 orgID 精确）→ `ListRemoteAgents` 按 `OrgID` 分组 → capability 过滤 → 返回 `{Org, RemoteAgent, Card}` 聚合
- **读缓存目录**，不实时拉取（FED-NFR4 < 500ms）

**AgentCardSync**（`federation_directory.go`）：
- `SyncOrgCards(ctx, orgID)`：遍历组织下 enabled remote agents → `DiscoverRemoteCard` 逐个拉取 → 经窄端口 `RemoteAgentCardWriter.UpdateRemoteAgentCard` 更新 `card_json`（data 层 `a2aRepo` 实现）；单个失败 Warn 跳过，不中断整体；返回成功数
- 触发方式：手动 RPC（本期）；定期 Cron 后续迭代（可复用 health runner 模式）

### F.6 联邦调用链（`FederationUsecase.InvokeFederated`）

> 实现要点（T11-T13）：治理链四组件（Trust/Policy/Quota/Audit）打包为 `FederationGovernance` struct 注入，`FederationUsecase` 依赖数 6（≤ AS-COG-01 上限 8）；远程执行经窄端口 `RemoteInvokeExecutor`（1 方法），由 `internal/a2a.FederationRemoteInvoker` 适配 `InvokeRemoteRegistry`；审计查询经 `AuditLogger.ListAudits` 委托；`DeleteOrg` 在 `Data.ExecInTx` 内原子删 org + 清 `a2a_remote_agents.org_id`。

```
入参：orgID, agentID, capability, payloadJSON, timeoutSec, workspace, callerAgentID(可选)
 1. 参数校验（orgID/agentID/capability 非空）→ 400
 2. OrgRepo.GetOrg(orgID)                      → 404 组织未注册
 3. org.Status != active                       → 403 组织已暂停
 4. TrustManager.Check                         → untrusted：AuditLogger.RecordDenied(denied_trust) + 403
 5. PolicyEngine.Evaluate                      → deny：RecordDenied(denied_policy) + 403
 6. QuotaChecker.Check                         → 超限：RecordDenied(denied_quota) + 429
 7. AuditLogger.RecordAllowed                  → 创建失败 fail-closed 500
 8. 解析目标：ListRemoteAgents(workspace) 内存过滤 OrgID==orgID 且 ID==agentID → 404 Agent 未注册
 9. internal/a2a/federation_invoke.go：
      复用 InvokeRemoteRegistry（重试/SSRF 校验/ClientAuthOptions）
10. AuditLogger.RecordResult(status/latency/err) → 失败仅 Warn
11. 返回远程结果 JSON
```

错误映射：

| 场景 | apierror | HTTP |
|------|----------|------|
| 组织未注册 / Agent 未注册 | `NotFound` | 404 |
| untrusted / 组织暂停 / 策略拒绝 | `Forbidden` | 403 |
| 配额超限 | `TooManyRequests` | 429 |
| 决策审计创建失败 | `Internal` | 500 |
| 远程不可达/调用失败 | `Internal`（含 cause） | 500 |
| 参数缺失 | `BadRequest` | 400 |

### F.7 Proto 契约（`api/kratos/a2a/v1/federation.proto`）

与 `a2a.proto` 同 package `kratos.a2a.v1`，独立 service：

| RPC | HTTP | 说明 |
|-----|------|------|
| `RegisterFederationOrg` | POST `/v1/a2a/federation/orgs` | 注册/更新组织（按 domain upsert） |
| `ListFederationOrgs` | GET `/v1/a2a/federation/orgs` | 组织列表 |
| `DeleteFederationOrg` | DELETE `/v1/a2a/federation/orgs/{id}` | 删除组织（不级联删 remote agents，仅解关联） |
| `SetFederationTrustLevel` | PUT `/v1/a2a/federation/orgs/{id}/trust` | 设置信任等级 |
| `SyncFederationOrgCards` | POST `/v1/a2a/federation/orgs/{id}/sync` | 手动同步组织 Agent Card 到目录缓存（FED-F7，单 Agent 失败跳过，返回成功数） |
| `UpsertFederationPolicy` | POST `/v1/a2a/federation/policies` | 配置调用策略 |
| `DiscoverFederationAgents` | GET `/v1/a2a/federation/agents` | 联邦目录（capability/org_id 过滤） |
| `InvokeFederatedAgent` | POST `/v1/a2a/federation/invoke` | 跨组织调用 |
| `QueryFederationAuditLogs` | GET `/v1/a2a/federation/audits` | 审计查询（分页 + 过滤） |

> 原规划 6 RPC 修订为 9 RPC：补 `DeleteFederationOrg`、`UpsertFederationPolicy`（策略管理无独立入口则 FED-F6 不可用）、`SyncFederationOrgCards`（biz 已实现 SyncOrgCards，无 RPC 入口则 FED-F7 前端不可触发）。

### F.8 安全与多租户

| 控制 | 落地 |
|------|------|
| 联邦 RPC 鉴权 | 与 A2AService 一致：Kratos admin JWT 中间件（不新增免鉴权路径） |
| 认证配置保密 | `auth_config_json` Ent `.Sensitive()`；日志用 `loggateway.Redacted()`；RPC 响应不回传明文（返 masked） |
| SSRF | 复用 `InvokeRemoteRegistry` 内置 `validateRemoteURL` + SSRF-safe HTTP client |
| 工作区 | 联邦目标解析限定 caller 所在 workspace 的 remote agents |
| 敏感字段 | 审计不记录 payload 明文，仅 capability/status/latency |

### F.9 前端设计

- `web/src/features/a2a/` 扩展：`federationApi.ts`、`federationTypes.ts`、mappers（多语言枚举映射）
- `web/src/components/a2a/federation/`：`FederationOrgPanel.vue`（组织列表 + 注册 Dialog + 信任等级编辑 + 同步按钮）、`FederationDirectoryPanel.vue`（目录搜索）、`FederationInvokePanel.vue`（调用）、`FederationAuditPanel.vue`（审计查询）
- `A2APage.vue` 新增「联邦」Tab；`stores/a2a` 扩展 federation state
- 多语言键：`a2a.federation.*`（zh-CN/en-US）；信任/状态/决策枚举映射中文（见需求 §子模块.5）

### F.10 与既有模块的关系

| 既有能力 | 联邦复用方式 |
|----------|-------------|
| `a2a_remote_agents` + `RemoteAgentRepo` | 联邦 Agent 载体（+`org_id`）；认证/健康/Card 缓存不变 |
| `InvokeRemoteRegistry` | 联邦调用的执行器（重试/SSRF/认证原样继承） |
| `Limiter` | QPS 配额执行器（key 前缀 `fed:`） |
| `ClientAuthOptions` | 组织级认证配置直接透传 |
| health cron / Prometheus | 联邦组织健康监控复用既有指标（不新建监控） |
