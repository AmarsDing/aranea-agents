# M15: A2A 协议 — 需求规格

> 对标 `pkg/trpc-agent-go/agent/a2aagent` + `server/a2a`，实现 Agent-to-Agent 通信与平台内互调。
>
> **2026-05-21 现状对齐**：
> - ✅ 平台内 `call_agent`、Admin Invoke、AgentCard CRUD、审计、Prometheus。
> - ✅ `agent_kind=a2a_proxy` 远程代理；LLM Agent A2A Endpoint 设置 Tab；`/a2a` 运维页（Discover / Audit / Invoke / 远程注册）。
> - ✅ 公开 A2A HTTP（`/v1/a2a/public/{agent_id}`）、远程注册表、mTLS/api_key/bearer、联邦 `GatewayDiscover`、Graph resume metadata。
> - 🟡 网关健康仅同步探测；无后台 Cron。
> - ❌ Admin Invoke 流式、Ingress 速率限制、Server 侧 mTLS 内置终止（建议走反向代理）。
>
> 进度与差距以 [26-a2a-development.md](./26-a2a-development.md) 与 [execution-plan.md](../guides/execution-plan.md) 为准；技术方案见 [26 a2a-protocol.design.md](./26%20a2a-protocol.design.md)。

---

## 1. 现状分析

### 1.1 已实现

| 能力 | 说明 |
|------|------|
| AgentCard 管理 | 启用/能力列表；默认 `enabled=false` |
| 平台内互调 | `call_agent` 工具 + 同工作区 Invoke |
| Agent Kind | `llm`（默认）与 `a2a_proxy`（远程代理） |
| A2A Endpoint | LLM Agent 可暴露为公开 A2A 服务 |
| 远程注册表 | 工作区级外部 A2A URL 注册与预览 |
| 运维页 `/a2a` | 发现、审计、Invoke 测试、远程管理、运行时配置 Banner |
| 联邦发现 | `GatewayDiscover` 聚合本地 Endpoint + 远程 registry |
| 安全基线 | 工作区隔离、审计、Card 校验、鉴权类型配置 |

### 1.2 仍缺失或半成品

1. **网关健康 Cron**：无周期性探测与专用指标（同步 `check_health` 已有）。
2. **Admin Invoke 流式**：运维 Invoke 为一元 JSON 聚合（有意设计，见设计 §十二）。
3. **API 速率限制**：未在应用层实现，建议 Ingress。
4. **Server 侧 mTLS 终止**：客户端 mTLS 已支持；服务端建议 Nginx/Ingress。

---

## 2. 产品模型

trpc-agent-go 中 `a2aagent.A2AAgent` 与 `llmagent.LLMAgent` 均实现 `agent.Agent`；产品层区分三种语义：

| 语义 | 产品形态 | 创建/配置入口 | 运行时 |
|------|----------|---------------|--------|
| **A2A Proxy（远程代理）** | Agent Kind `a2a_proxy` | 创建 Agent →「A2A 远程代理」 | 对话经 A2A 协议转发至远程服务 |
| **A2A Endpoint（本地暴露）** | LLM Agent 能力开关 | Agent 设置 → A2A Tab | 公开 HTTP + AgentCard；可被 `call_agent` 或外部客户端调用 |
| **平台内互调** | LLM 工具 `call_agent` | Agent 设置启用工具 | 同工作区经 A2A 用例派发，非新 Agent 类型 |

**页面分工**：

| 页面 | 职责 |
|------|------|
| `/agents` 创建 | LLM / A2A Proxy；Proxy 采集 URL、鉴权、流式 |
| `/agents/:id/settings` → A2A | LLM：Endpoint + Card；Proxy：远程连接信息与只读 Card |
| `/a2a` | 工作区级发现、审计、Invoke 测试、远程注册、联邦 Gateway |

与 Team / Graph：A2A Proxy 可作为成员或节点，与 LLM Agent 同等可选。创建流程见 [2 agents-create.md](./2%20agents-create.md)；Endpoint 见 [5 agent-setting.md](./5%20agent-setting.md)。

---

## 3. 需求清单

### 3.1 平台内 call_agent（P0）— ✅

**用户故事**：作为 Agent 所有者，我希望启用 `call_agent` 后，模型可调用同工作区另一 Agent 的命名能力。

**验收标准**：
- [x] 同工作区、目标已启用 A2A 且能力存在时可成功调用
- [x] 未启用或能力不存在时返回明确错误
- [x] 每次调用写入 `a2a_audit`

### 3.2 Admin Invoke（P0）— ✅

**用户故事**：作为管理员，我希望通过 API 测试对某 Agent 的能力调用并看到聚合结果。

**验收标准**：
- [x] `POST /v1/a2a/invoke` 触发目标 Agent 执行
- [x] 跨工作区（caller/callee workspace 不一致）返回 403
- [x] 远程 registry id 可作为 callee 派发

### 3.3 A2A 远程代理 Agent（P1）— ✅

**用户故事**：创建「A2A 远程代理」，在 Chat 中像本地 Agent 一样使用外部 A2A 服务。

**验收标准**：
- [x] 可创建 `a2a_proxy`，列表展示 `A2A ↗`
- [x] Chat 对话经 A2A 到达远程；不可达时明确错误
- [x] 不要求 Provider/Model；`agent_kind` 创建后不可变

### 3.4 LLM A2A Endpoint（P1）— ✅

**用户故事**：在设置页启用「暴露为 A2A 服务」并编辑 capabilities。

**验收标准**：
- [x] 启用后出现在 Discover / `call_agent` 目标
- [x] 未启用时调用返回明确错误
- [x] 公开 URL 可经系统设置 / 环境变量配置

### 3.5 远程注册与对外 Endpoint（P1）— ✅

**验收标准**：
- [x] 远程 Agent CRUD + URL 预览 AgentCard
- [x] 外部客户端可通过 `/v1/a2a/public/{agent_id}` 调用已启用 LLM Agent
- [x] 客户端鉴权：none / api_key / bearer / mtls

### 3.6 联邦与 Graph（P2）— ✅

**验收标准**：
- [x] `GET /v1/a2a/gateway/discover` 返回 local + remote 联邦视图
- [x] Graph resume 元数据可由项目层编码（与框架键名对齐）
- [x] Proxy/Endpoint 流式能力可配置

### 3.7 运维页（P1）— ✅

**验收标准**：
- [x] `/a2a`：Discover、Audit、Invoke、远程注册、运行时 Banner

### 3.8 网关增强（P3）— 待实现

| 项 | 说明 |
|----|------|
| 健康 Cron | 周期性探测远程 registry |
| 联邦路由 | 按 healthy/source 选路 |
| 速率限制 | API 网关 / middleware |
| Admin Invoke 流式 | 低优先级 |

---

## 4. 验收标准总览

| # | 项 | 状态 |
|---|-----|------|
| 1 | AgentCard API 管理 | ✅ |
| 2 | 审计日志 | ✅ |
| 3 | call_agent 同工作区互调 | ✅ |
| 4 | Admin Invoke 实际执行 | ✅ |
| 5 | 跨工作区拒绝 | ✅ |
| 6 | 远程 A2A Proxy Chat | ✅ |
| 7 | 外部公开 Endpoint | ✅ |
| 8 | 远程注册 + 联邦 Discover | ✅ |
| 9 | 流式 Proxy/Endpoint | ✅ |
| 10 | Graph resume metadata | ✅ |
| 11 | 网关健康 Cron | ❌ |
| 12 | API 速率限制 | ❌ |

---

## 5. 运维指南（摘要）

### 5.1 核心原则

1. **默认关闭** — AgentCard `enabled=false`，显式启用后才可被发现或调用。
2. **工作区隔离** — Discover / Invoke / `call_agent` 受工作区约束。
3. **审计** — 每次调用写入 `a2a_audit`。
4. **最小信任面** — 分发前校验 Card 与 capability。

### 5.2 消息格式（Invoke）

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

### 5.3 API 端点（摘要）

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/v1/a2a/discover` | 扁平发现（本地 + 远程） |
| GET | `/v1/a2a/gateway/discover` | 联邦视图（含 healthy） |
| POST | `/v1/a2a/invoke` | Admin 测试调用 |
| PUT/GET | `/v1/a2a/agents/{id}/card` | AgentCard |
| GET | `/v1/a2a/audit` | 审计 |
| POST/GET/DELETE | `/v1/a2a/remote-agents` | 远程注册 |
| GET | `/v1/a2a/config` | 公开 URL 等运行时配置 |
| * | `/v1/a2a/public/{agent_id}` | 对外 A2A 协议（免 Admin JWT） |

完整分层、表结构、指标见 [设计文档](./26%20a2a-protocol.design.md)。

### 5.4 公开地址配置优先级

`环境变量 A2A_PUBLIC_BASE_URL` > **系统设置** `a2a_public_base_url` > `configs` `server.a2a_public_base_url` > 由 HTTP 监听地址推导。

- 编辑：`/settings`（系统设置）
- 只读展示：`/a2a` Banner + `GET /v1/a2a/config`

### 5.5 安全控制

| 控制 | 状态 |
|------|------|
| 默认关闭 | ✅ |
| 工作区隔离 | ✅ |
| 审计 | ✅ |
| Card/capability 校验 | ✅ |
| 客户端鉴权配置 | ✅ |
| API 速率限制 | ❌ |
| Server mTLS 内置 | ❌（建议 Ingress） |

### 5.6 Prometheus

| 指标 | 说明 |
|------|------|
| `aranea_a2a_invoke_total` | 调用次数（caller/callee/status） |
| `aranea_a2a_invoke_duration_seconds` | 延迟直方图 |
