# M15: A2A 协议 — 需求规格

> 对标 `pkg/trpc-agent-go/agent/a2aagent` + `server/a2a`，实现 Agent-to-Agent 通信与平台内互调。
>
> 本文档仅描述用户故事、功能需求、验收标准与非功能需求。技术方案（分层/Proto/数据模型/API 契约）见 [26-a2a-protocol.design.md](./26-a2a-protocol.design.md)；进度状态与任务清单见 [26-a2a-protocol.development.md](./26-a2a-protocol.development.md)。

---

## 1. 产品模型

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

与 Team / Graph：A2A Proxy 可作为成员或节点，与 LLM Agent 同等可选。创建流程见 [2-agents-create.md](./2-agents-create.md)；Endpoint 见 [5-agent-setting.md](./5-agent-setting.md)。

---

## 2. 需求清单

### 2.1 平台内 call_agent（P0）

**用户故事**：作为 Agent 所有者，我希望启用 `call_agent` 后，模型可调用同工作区另一 Agent 的命名能力。

**验收标准**：
- 同工作区、目标已启用 A2A 且能力存在时可成功调用
- 未启用或能力不存在时返回明确错误
- 每次调用写入审计日志

### 2.2 Admin Invoke（P0）

**用户故事**：作为管理员，我希望通过 API 测试对某 Agent 的能力调用并看到聚合结果。

**验收标准**：
- `POST /v1/a2a/invoke` 触发目标 Agent 执行
- 跨工作区（caller/callee workspace 不一致）返回 403
- 远程 registry id 可作为 callee 派发

### 2.3 A2A 远程代理 Agent（P1）

**用户故事**：创建「A2A 远程代理」，在 Chat 中像本地 Agent 一样使用外部 A2A 服务。

**验收标准**：
- 可创建 `a2a_proxy`，列表展示 `A2A ↗`
- Chat 对话经 A2A 到达远程；不可达时明确错误
- 不要求 Provider/Model；`agent_kind` 创建后不可变

### 2.4 LLM A2A Endpoint（P1）

**用户故事**：在设置页启用「暴露为 A2A 服务」并编辑 capabilities。

**验收标准**：
- 启用后出现在 Discover / `call_agent` 目标
- 未启用时调用返回明确错误
- 公开 URL 可经系统设置 / 环境变量配置

### 2.5 远程注册与对外 Endpoint（P1）

**验收标准**：
- 远程 Agent CRUD + URL 预览 AgentCard
- 外部客户端可通过公开 Endpoint 调用已启用 LLM Agent
- 客户端鉴权：none / api_key / bearer / mtls

### 2.6 联邦与 Graph（P2）

**验收标准**：
- 联邦 Discover 返回 local + remote 视图
- Graph resume 元数据可由项目层编码（与框架键名对齐）
- Proxy/Endpoint 流式能力可配置

### 2.7 运维页（P1）

**验收标准**：
- `/a2a`：Discover、Audit、Invoke、远程注册、Gateway 联邦视图、运行时 Banner

### 2.8 网关增强（P3）

**验收标准**：
- 网关健康周期探测，远程离线可告警
- 调用速率限制，超限返回 429
- 联邦路由按 healthy/source 选路

---

## 3. 验收标准总览

| # | 项 |
|---|-----|
| 1 | AgentCard API 管理 |
| 2 | 审计日志 |
| 3 | call_agent 同工作区互调 |
| 4 | Admin Invoke 实际执行 |
| 5 | 跨工作区拒绝 |
| 6 | 远程 A2A Proxy Chat |
| 7 | 外部公开 Endpoint |
| 8 | 远程注册 + 联邦 Discover |
| 9 | 流式 Proxy/Endpoint |
| 10 | Graph resume metadata |
| 11 | 网关健康探测 |
| 12 | API 速率限制 |

> 各项的完成状态见 [开发计划 §3 现状评估](./26-a2a-protocol.development.md)。

---

## 4. 非功能需求

### 4.1 安全基线

| 控制 | 用户可见行为 |
|------|-------------|
| 默认关闭 | AgentCard `enabled=false`，显式启用后才可被发现或调用 |
| 工作区隔离 | Discover / Invoke / `call_agent` 受工作区约束 |
| 审计 | 每次调用写入审计日志，可在 `/a2a` Audit 面板查询 |
| 最小信任面 | 分发前校验 Card 与 capability；未启用返回明确错误 |
| 客户端鉴权 | 远程代理与公开 Endpoint 支持配置鉴权类型 |

### 4.2 运维原则

1. **默认关闭** — AgentCard `enabled=false`，显式启用后才可被发现或调用。
2. **工作区隔离** — Discover / Invoke / `call_agent` 受工作区约束。
3. **审计** — 每次调用写入审计日志。
4. **最小信任面** — 分发前校验 Card 与 capability。

---

## 5. 交互规格（用户视角）

### 5.1 公开地址配置

用户可通过以下入口配置 A2A 公开地址：

- **编辑入口**：`/settings`（系统设置）字段 `a2a_public_base_url`，保存即生效
- **只读展示**：`/a2a` 运行时 Banner + `GET /v1/a2a/config`

> 配置优先级解析（环境变量 > 系统设置 > yaml > 推导）属技术实现，见 [设计文档 §十二](./26-a2a-protocol.design.md)。

### 5.2 安全控制（用户可见行为）

| 控制 | 用户可见行为 |
|------|-------------|
| 默认关闭 | ✅ AgentCard 默认不启用 |
| 工作区隔离 | ✅ 跨工作区调用被拒绝 |
| 审计 | ✅ `/a2a` Audit 面板可查 |
| Card/capability 校验 | ✅ 未启用/无能力返回明确错误 |
| 客户端鉴权配置 | ✅ 创建远程代理时可配置 |
| API 速率限制 | 超限返回 429 |
| Server mTLS 内置终止 | 建议走反向代理/Ingress |

> 安全控制的技术实现细节见 [设计文档 §九](./26-a2a-protocol.design.md)。

---

## 6. 关联文档

| 文档 | 内容 |
|------|------|
| [26-a2a-protocol.design.md](./26-a2a-protocol.design.md) | 分层架构、Proto 契约、数据模型、API 端点、消息格式、Prometheus 指标、传输语义 |
| [26-a2a-protocol.development.md](./26-a2a-protocol.development.md) | 模块定位、代码锚点、现状评估、差距优化、Phase 划分、任务清单 |
