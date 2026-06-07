# A2A 协议 — 开发计划

> **版本**：2026-06-06 | **状态**：🟢 Phase 1–4 部分已落地；Phase 4 剩余联邦路由 + HTTP 限流
> **需求**：[26 a2a-protocol.md](./26%20a2a-protocol.md) · **设计**：[26 a2a-protocol.design.md](./26%20a2a-protocol.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) I7-A2A · **变更**：[Phase1-2](../changelog/2026-05-20-A2A-Phase1-2.md) · [Phase3](../changelog/2026-05-20-A2A-Phase3.md) · [Phase35](../changelog/2026-05-20-A2A-Phase35.md) · [Review](../changelog/2026-05-20-A2A-Review-Fixes.md) · [DocSync](../changelog/2026-05-21-A2A-DocSync.md)

---

## 1. 模块定位

Agent-to-Agent：同工作区 `call_agent`、Admin Invoke、LLM **A2A Endpoint** 对外暴露、`a2a_proxy` 远程代理、工作区远程注册表与联邦 Discover。

**传输原则**：A2A 外部协议用 **A2A HTTP（一元 + SSE）**；Chat UI 用 **`/v1/ws`**；Admin Invoke 用 **一元 JSON**（运维聚合，非 token 流）。详见设计文档 §十二。

**代码锚点**：

| 包 / 文件 | 职责 |
|-----------|------|
| `internal/a2a/` | 协议桥接：tool、invoker、remote_client、remote_invoke、callee_resolve、card_validate、graph_resume、capability_metadata、invoke_workspace、public_base_url、public_base_store |
| `internal/a2a/health/` | 网关健康 Cron：Runner、probeAll/One、Prometheus 指标 |
| `internal/a2a/trpc/` | trpc-agent-go 包装：agent、server、registry、auth |
| `internal/biz/a2a.go` | 别名层（A2AUsecase = a2a.Usecase 等） |
| `internal/biz/a2a/a2a.go` | 核心领域：Card/Invocation/Audit/Remote/Gateway 用例 |
| `internal/biz/a2a_limit.go` | 业务层限流器（A2AInvokeLimiter） |
| `internal/biz/agent_kind.go` | Agent Kind 常量 + A2AProxyConfig |
| `internal/service/a2a.go` | A2AService RPC + Prometheus + 限流 |
| `internal/service/a2a_endpoint.go` | 公开 Endpoint 构建（`A2AEndpointBuilder`） |
| `internal/service/a2a_public_base.go` | 公开 URL 热更新（`A2APublicBaseReloader`） |
| `internal/service/chat_native.go` | `RunAgentTurn` + `injectA2AContext` |
| `internal/agent/trpc_build_router.go` | `BuildTRPCAgent`（llm / a2a_proxy） |
| `web/src/pages/A2APage.vue` | 运维页；`features/a2a/useA2APage.ts` + 面板组件 |

---

## 2. 架构质量与 SRP（2026-05-21）

| 原则 | 落地 |
|------|------|
| **biz 不 import trpc** | `A2AUsecase`、Card/远程/联邦逻辑均在 `internal/biz/a2a/`；`agent_kind` 在 `agent_kind.go`；限流在 `a2a_limit.go` |
| **单一派发路径** | `NewInvoker` → 本地 `RunAgentTurn` 或 `InvokeRemoteRegistry`；`A2AService.Invoke` 复用同一 Invoker |
| **Card 校验集中** | `CheckCalleeCard`（`card_validate.go`）供 tool 与 HTTP Invoke |
| **目标解析集中** | `ResolveInvokeTarget`（`callee_resolve.go`）：本地 Card 优先，disabled 不 fallback 远程 |
| **Endpoint 组装在 service** | `A2AEndpointBuilder` 读 catalog + Card，调用 `BuildA2AEndpointServer`；`internal/server` 只注册前缀 |
| **前端分层** | `features/a2a`（api/types/mappers/a2aTableUi/authUtils）+ `stores/a2a` + `useA2APage` + `components/a2a/*`；mapper 单测 |

**依赖方向**（不变）：

```
api/kratos/a2a/v1/*.proto
  → internal/service (A2AService, A2AEndpointBuilder, A2APublicBaseReloader)
  → internal/biz (A2AUsecase, A2ALimit, Gateway)
  → internal/data (SQLite)
internal/a2a/trpc → pkg/trpc-agent-go (a2aagent, server/a2a)
internal/service/chat_native → internal/a2a (InjectRunContext, Invoker)
```

---

## 3. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| call_agent + Context 注入 | ✅ | `injectA2AContext` / `InjectRunContext` |
| Admin Invoke 实际派发 | ✅ | `A2AService.Invoke` + `NewInvoker` |
| 工作区隔离 | ✅ | `ValidateSameWorkspace` + Admin `workspace` 字段 |
| agent_kind=a2a_proxy | ✅ | `trpc_build_router` + `BuildTRPCA2AAgent` |
| LLM A2A Endpoint Tab | ✅ | `AgentSettingsA2AEndpointTab.vue` |
| 公开 A2A HTTP | ✅ | `/v1/a2a/public/{agent_id}` + `EndpointRegistry` |
| 远程注册 CRUD | ✅ | `a2a_remote_agents` + `/v1/a2a/remote-agents` |
| 客户端鉴权 api_key/bearer/mtls | ✅ | `remote_client.go` + `trpc/auth.go` |
| 公开 URL 配置 | ✅ | env > 系统设置 DB > yaml > derived；`GET /v1/a2a/config` |
| 远程 registry Invoke | ✅ | `remote_invoke.go` + invoker 路由 |
| GatewayDiscover 联邦 | ✅ | `GET /v1/a2a/gateway/discover` |
| Graph resume metadata | ✅ | `graph_resume.go` |
| Proxy/Endpoint 流式 | ✅ | `enable_streaming` / AgentCard streaming |
| `/a2a` 运维页 | ✅ | Discover / Audit / Invoke / 远程 / Banner |
| 网关健康 Cron | ✅ | `internal/a2a/health/runner.go`；默认 10 分钟；`A2A_HEALTH_DISABLED=1` 可禁用 |
| 业务层速率限制 | ✅ | `A2AInvokeLimiter`（60次/分钟）；`A2AService.Invoke` 超限返回 429 |
| Admin Invoke SSE | ❌ | 有意非流式 |
| HTTP 中间件层限流 | ❌ | 待 Ingress / Kratos middleware |
| Ent 列级 agent_kind | 🟡 | 现用 `config_json` + `Kind` 字段 |

---

## 4. 差距与优先级（剩余）

| # | 差距 | 优先级 | 说明 |
|---|------|--------|------|
| 1 | 联邦路由策略 | **P3** | 按 healthy/source 选路 |
| 2 | HTTP 中间件层限流 | **P2** | Ingress / Kratos middleware（业务层已实现） |
| 3 | Admin Invoke 可选流式 | **P3** | 外部客户端应走 Public Endpoint |
| 4 | Ent `agent_kind` 列迁移 | **P3** | 可选 schema 硬化 |

---

## 5. 开发阶段

### Phase 1 — P0 平台内互调 ✅

| # | 任务 | 状态 |
|---|------|------|
| 1.1 | call_agent 工具 + 条件注入 | ✅ |
| 1.2 | Invoker + RunAgentTurn 派发 | ✅ |
| 1.3 | Admin Invoke 实际执行 | ✅ |
| 1.4 | 工作区校验 | ✅ |

### Phase 2 — P1 Agent Kind + 前端 ✅

| # | 任务 | 状态 |
|---|------|------|
| 2.1 | `agent_kind` / `a2a_proxy` 创建与构建 | ✅ |
| 2.2 | A2A 设置 Tab + 列表徽章 | ✅ |
| 2.3 | `/a2a` Discover / Audit / Invoke | ✅ |

### Phase 3 — 跨实例 Server + 远程注册 ✅

| # | 任务 | 状态 |
|---|------|------|
| 3.1 | Public Endpoint + EndpointRegistry | ✅ |
| 3.2 | 远程 Agent CRUD + Discover 预览 | ✅ |
| 3.3 | mTLS / api_key / bearer 客户端 | ✅ |
| 3.4 | Admin Invoke workspace 策略 | ✅ |

### Phase 3.5 — 联邦与 Graph ✅

| # | 任务 | 状态 |
|---|------|------|
| 3.5.1 | 传输模型文档（§十二） | ✅ |
| 3.5.2 | Graph resume metadata | ✅ |
| 3.5.3 | 远程 registry Invoke | ✅ |
| 3.5.4 | GatewayDiscover | ✅ |
| 3.5.5 | Discover enrichment（source/url） | ✅ |

### Phase 4 — 网关增强（部分实现）

| # | 任务 | 状态 | 验收标准 |
|---|------|------|----------|
| 4.1 | 健康探测 Cron | ✅ | `internal/a2a/health/runner.go`；远程离线告警 + `aranea_a2a_gateway_healthy` 指标 |
| 4.2 | 业务层速率限制 | ✅ | `A2AInvokeLimiter`（60次/分钟）；超限返回 429 |
| 4.3 | 联邦路由策略 | ❌ | 按 healthy/source 选路 |
| 4.4 | HTTP 中间件层限流 | ❌ | Ingress 指引 + 可选 Kratos middleware |
| 4.5 | Admin Invoke 可选流式 | ❌ | 低优先级；外部客户端应走 Public Endpoint |

---

## 6. 验收标准

### Phase 3.5

- [x] 文档明确：A2A HTTP vs `/v1/ws` vs Admin Invoke
- [x] Public Endpoint 广告 streaming；Proxy 可配置 streaming
- [x] `BuildGraphResumeMetadata` 可编码 resume 字段
- [x] Discover/Gateway 远程条目可用 registry id Invoke
- [x] `GET /v1/a2a/gateway/discover` 返回 local + remote 联邦视图
- [x] `call_agent` 同工作区互调成功并写审计

### Phase 4（已完成项）

- [x] 健康探测 Cron：`internal/a2a/health/runner.go` 周期探测 + `aranea_a2a_gateway_healthy` 指标
- [x] 业务层速率限制：`A2AInvokeLimiter` 60次/分钟滑动窗口；超限 429

---

## 7. 依赖与风险

- **勿混用传输**：Chat WS 上复刻 A2A 流式会增加双栈维护；外部集成用 `/v1/a2a/public/...`。
- **Admin Invoke 非流式**：长任务用 Public Endpoint 或 Proxy Agent Chat。
- **Graph resume**：语义以 `pkg/trpc-agent-go/server/a2a` 为准；项目层仅 metadata 桥接。
- **公开 Endpoint**：须 catalog Agent + `a2a_agent_cards.enabled`；`a2a_proxy` 不挂载 Endpoint。
