# M15: A2A 协议 — 详细需求

> 对标 `pkg/trpc-agent-go/agent/a2aagent` + `internal/a2a`，实现 Agent-to-Agent 通信协议。
>
> **2026-05-20 现状对齐**：
> - ✅ `internal/a2a/tool.go` 已暴露 `call_agent` 工具；`internal/biz/a2a.go` / `internal/data/a2a.go` / `internal/service/a2a.go` 已通；`api/kratos/a2a/v1/*` proto/HTTP 已生成；Wire 注入已完成；前端 API + Store + `A2APage` 已实现。
> - ⚠️ `call_agent` 工具已通过 `toolsets.go` 条件注入（当 Agent 的 effective tools 包含 `call_agent` 时），但 **Context 注入缺失**：`WithA2AUsecase`、`WithCallerAgentID`、`WithInvoker` 在 service 层运行时未被调用，导致工具执行时报 "invoker not configured"。
> - ❌ **Agent Kind `a2a_proxy`**（创建时选择「A2A 远程代理」）尚未实现；目录内 Agent 仍全部按 LLM 构建。
> - ❌ LLM Agent 的 **A2A Endpoint**（设置页暴露为 A2A 服务 + AgentCard）尚未产品化。
> - ❌ 跨实例 A2A（A2A Server、远程注册、流式通信、Graph 恢复）尚未实现。
> - ❌ 远端鉴权 / mTLS 尚未实现。
>
> **产品模型（2026-05-20）**：A2A 拆为两种语义 — **A2A Proxy**（远程代理，作为 Agent 创建变种）与 **A2A Endpoint**（本地 LLM Agent 的能力开关）。详见 §2.5、§3.10–§3.11。技术设计见 [26 a2a-protocol.design.md](./26%20a2a-protocol.design.md) §十一。
>
> 后续以 `guides/execution-plan.md` 为准。

---

## 1. 现状分析

### 1.1 已实现

| 项 | 状态 | 证据 |
|----|------|------|
| Proto 定义 | ✅ | `api/kratos/a2a/v1/a2a.proto`：Discover, Invoke, UpdateAgentCard, GetAgentCard, ListAudit |
| Biz 领域模型 | ✅ | `internal/biz/a2a.go`：A2ACapability, A2AAgentCard, A2AInvocation, A2AAuditEntry, A2ARepo, A2AUsecase |
| Data 持久化 | ✅ | `internal/data/a2a.go`：SQLite 三表 + EnsureA2ASchema |
| Service 层 | ✅ | `internal/service/a2a.go`：5 个 RPC + Prometheus 指标 |
| call_agent 工具 | ✅ | `internal/a2a/tool.go`：工具定义 + Card 验证 + 审计写入 |
| 工具条件注入 | ✅ | `internal/tools/trpc/toolsets.go`：`cfg.CallAgent` 时注入 |
| Wire 注入 | ✅ | data/biz/service ProviderSet 均已注册 |
| HTTP/gRPC 注册 | ✅ | `internal/server/http.go` + `grpc.go` |
| 前端 API + Store | ✅ | `web/src/features/a2a/` + `web/src/stores/a2a/` |
| Team A2A 配置 | ✅ | `TeamEditorDialog.vue` 中 A2A 协议区 |

### 1.2 未实现

| 项 | 状态 | 说明 |
|----|------|------|
| Context 注入 | ❌ | `WithA2AUsecase`/`WithCallerAgentID`/`WithInvoker` 未在 service 层调用 |
| Invoke 实际派发 | ❌ | `A2AService.Invoke` 仅记录 pending，未执行目标 Agent |
| 跨工作区校验 | ❌ | Invoke 未校验 caller/callee 同工作区 |
| 远程 Agent 发现 | ❌ | 无 Agent 注册中心，无 URL 发现 |
| A2A Server | ❌ | 未将本地 Agent 暴露为 A2A 服务 |
| A2A Agent | ❌ | 未使用 trpc-agent-go a2aagent 调用远程 Agent |
| 消息转换 | ❌ | trpc Event ↔ A2A Message 双向转换未实现 |
| 流式通信 | ❌ | A2A SSE 流式响应未实现 |
| Graph 恢复 | ❌ | A2A 任务与 Graph 中断/恢复未集成 |
| 远端鉴权 / mTLS | ❌ | 跨实例安全未实现 |
| 前端管理页面 | ❌ | 仅有 API/Store，无独立 A2A 管理页面 |

---

## 2. trpc 框架参照

```
pkg/trpc-agent-go/
├── agent/a2aagent/
│   ├── a2a_agent.go        # A2AAgent：与远程 A2A Agent 通信
│   ├── a2a_agent_option.go  # 选项配置
│   └── a2a_converter.go     # 事件与 A2A 消息转换
├── internal/a2a/
│   ├── a2a.go              # A2A 常量和工具函数
│   └── ...
├── server/a2a/             # A2A Server：将 Agent 暴露为 A2A 服务
└── docs/mkdocs/zh/a2a.md   # A2A 集成指南
```

### A2AAgent

```go
type A2AAgent struct {
    name            string
    description     string
    agentCard       *server.AgentCard
    agentURL        string
    eventConverter  A2AEventConverter
    a2aClient       *client.A2AClient
    enableStreaming  *bool
}

func New(opts ...Option) (*A2AAgent, error)
```

A2AAgent 实现了 `agent.Agent` 接口，可作为子 Agent 或独立 Agent 使用。

### A2A Server

```go
// trpc.group/trpc-go/trpc-agent-go/server/a2a
func New(opts ...Option) (*A2AServer, error)
```

一键将本地 Agent 转换为 A2A 网络服务，自动生成 AgentCard，支持流式/非流式。

### 核心能力

1. **AgentCard 自动发现**：通过 URL 获取远程 Agent 的 AgentCard
2. **消息转换**：将 trpc Event 转换为 A2A Message，反之亦然
3. **流式通信**：支持 SSE 流式响应
4. **DataPart 映射**：FunctionCall/FunctionResponse/CodeExecution 等 DataPart 类型映射
5. **状态传递**：通过 metadata 传递 session state
6. **Graph 恢复**：A2A 任务与 Graph 工作流的中断/恢复集成

### 2.5 产品模型：Agent Kind 与 A2A 语义拆分

trpc-agent-go 中 `a2aagent.A2AAgent` 与 `llmagent.LLMAgent` 均实现 `agent.Agent` 接口，**运行时是一等公民**；产品层需区分两种 A2A 语义，避免与独立注册表页 `/a2a` 混淆：

| 语义 | 产品形态 | 创建入口 | 运行时 |
|------|----------|----------|--------|
| **A2A Proxy（远程代理）** | Agent 目录中的一种 **Agent Kind**（`a2a_proxy`） | 创建 Agent 时选择「A2A 远程代理」 | 包装 `a2aagent`，对话经 A2A 协议转发至远程服务 |
| **A2A Endpoint（本地暴露）** | 普通 LLM Agent 的 **能力开关** | Agent 设置页「A2A」Tab | LLM Agent + `server/a2a` + AgentCard；可被 `call_agent` 或外部客户端调用 |
| **平台内互调** | LLM Agent 的工具能力 | Agent 设置启用 `call_agent` | 同工作区经 `A2AUsecase` 派发，非新 Agent 类型 |

**页面分工**：

| 页面 | 职责 |
|------|------|
| `/agents` 创建对话框 | 选择 LLM / A2A 远程代理；Proxy 采集远程 URL、鉴权、流式开关 |
| `/agents/:id/settings` → A2A Tab | LLM Agent 的 Endpoint：AgentCard、capabilities、暴露开关 |
| `/a2a`（注册表） | 工作区级发现、审计、Invoke 测试、远程 Agent 注册表（运维视图） |

与 Team / Graph 的关系：Team、Graph 为独立编排实体；**A2A Proxy 可作为 Team 成员或 Graph Agent 节点**，与 LLM Agent 同等可选。创建流程见 [2 agents-create.md](./2%20agents-create.md) §9；Endpoint 设置见 [5 agent-setting.md](./5%20agent-setting.md) §10。

---

## 3. 需求清单

### 3.1 Context 注入与 call_agent 打通（P0）

**需求**：使 `call_agent` 工具在运行时可用

**实现要点**：
- 在 `internal/service/trpc_turn.go`（或 Agent 运行时上下文创建处）调用 `a2a.WithA2AUsecase`、`a2a.WithCallerAgentID`、`a2a.WithInvoker`
- 实现 `invokerFunc`：内部调用 `A2AUsecase.StartInvocation` → 执行目标 Agent → `FinishInvocation`
- `A2AUsecase` 需注入到 `TRPCBuilderDeps` 或 service 层

**验收标准**：
- Agent A 启用 `call_agent` 工具后，可在对话中调用同工作区 Agent B
- 调用成功后 `a2a_invocations` 记录 status=success，`a2a_audit` 有对应条目
- 调用失败（Agent B 未启用 A2A / 能力不存在）返回明确错误

### 3.2 Invoke 端点实际派发（P0）

**需求**：`POST /v1/a2a/invoke` 端点不仅记录 pending，还需实际执行目标 Agent

**实现要点**：
- `A2AService.Invoke` 中增加目标 Agent 执行逻辑
- 增加工作区隔离校验（caller 与 callee 必须同工作区）
- 调用完成后更新 invocation 状态和审计

**验收标准**：
- 通过 API 调用 `POST /v1/a2a/invoke` 可触发目标 Agent 执行并返回结果
- 跨工作区调用返回 403 Forbidden

### 3.3 A2AAgent 集成（P1）

**需求**：支持与远程 A2A Agent 通信

**实现要点**：
- 新建 `internal/a2a/trpc/agent.go`
- 包装 trpc `a2aagent.New` 为项目可用组件
- 支持通过 AgentCard URL 发现远程 Agent

**验收标准**：项目 Agent 可与远程 A2A Agent 通信

### 3.4 A2A Server（P1）

**需求**：将项目 Agent 暴露为 A2A 服务

**实现要点**：
- 新建 `internal/a2a/trpc/server.go`
- 使用 `trpc.group/trpc-go/trpc-agent-go/server/a2a` 创建 A2A Server
- 自动生成 AgentCard
- 注册 Agent 处理器

**验收标准**：外部 A2A 客户端可发现和调用项目 Agent

### 3.5 消息转换（P2）

**需求**：trpc Event 与 A2A Message 双向转换

**实现要点**：
- 集成 trpc `a2aagent/a2a_converter.go`
- FunctionCall → A2A DataPart
- CodeExecution → A2A DataPart
- StateDelta → A2A metadata

**验收标准**：消息在两个方向正确转换，无信息丢失

### 3.6 流式通信（P2）

**需求**：支持 A2A 流式响应

**实现要点**：
- 使用 A2A SSE 传输
- 事件流式转发
- 支持中途取消

**验收标准**：A2A 通信支持流式响应

### 3.7 Graph 恢复集成（P2）

**需求**：A2A 任务与 Graph 工作流的中断/恢复集成

**实现要点**：
- 集成 trpc `internal/a2a/graph_resume.go`
- A2A 长时间任务触发 Graph 中断
- 任务完成后恢复 Graph 执行

**验收标准**：A2A 长时间任务可中断 Graph，完成后恢复

### 3.8 A2A 网关注册中心（P3）

**需求**：集中管理 A2A Agent 注册和发现

**实现要点**：
- 新建 `internal/a2a/registry/`
- Agent 注册：名称、描述、URL、能力
- Agent 发现：按能力搜索
- 健康检查：定期检查 Agent 可用性

**验收标准**：Agent 可通过注册中心发现和调用其他 Agent

### 3.9 前端管理页面（P1）

**需求**：A2A 注册表页（工作区级治理）

**实现要点**：
- `A2APage.vue`：Discover / Audit / Invoke 三 Tab（✅ 已实现）
- 待补：`A2ARemoteAgentDialog` — 注册远程 Agent 至工作区注册表（与 catalog 中 `a2a_proxy` Agent 可互相同步，见设计文档）

**验收标准**：通过 `/a2a` 可发现 AgentCard、浏览审计、测试 Invoke

### 3.10 Agent Kind：A2A 远程代理（P1）

**需求**：创建 Agent 时可选择 **A2A 远程代理**，在目录中表现为普通 Agent，运行时通过 A2A 协议连接外部服务。

**用户故事**：
- 作为管理员，我希望在创建 Agent 时选择「A2A 远程代理」，填写远程 URL 并自动发现 AgentCard，以便在 Chat 中像本地 Agent 一样使用外部 A2A 服务。
- 作为开发者，我希望 A2A Proxy Agent 出现在 Agent 列表并带 `A2A ↗` 标识，以便与 LLM Agent 区分。

**功能要点**：
- 创建对话框增加 Agent 类型：`LLM Agent`（默认）| `A2A 远程代理`
- Proxy 类型：必填远程 URL；可选「发现 AgentCard」；流式开关、超时、鉴权（none / api_key / mtls）
- Proxy 类型**不要求** Provider / Model；隐藏记忆 L0–L4、Skill、进化等 LLM 专属配置
- 创建成功后写入 `agents` 行（`agent_kind=a2a_proxy`）并同步/缓存远程 AgentCard
- Chat、Team 成员、Graph Agent 节点均可选用 A2A Proxy Agent

**验收标准**：
- [ ] 可通过创建对话框创建 `a2a_proxy` Agent，列表展示 `A2A ↗` 徽章
- [ ] Chat 选择该 Agent 可对话，事件流经 A2A 协议到达远程服务
- [ ] 远程 URL 不可达时返回明确错误，不 silent fallback 为 LLM

### 3.11 LLM Agent 的 A2A Endpoint 配置（P1）

**需求**：普通 LLM Agent 可在设置页配置为 **A2A 提供方**（Endpoint），而非新建独立 Agent 类型。

**用户故事**：
- 作为 Agent 所有者，我希望在 Agent 设置中启用「暴露为 A2A 服务」并编辑 AgentCard 能力列表，以便同工作区其他 Agent 通过 `call_agent` 或外部客户端调用我。
- 作为管理员，我希望 Endpoint 默认关闭，显式启用后才出现在 Discover 列表。

**功能要点**：
- Agent 设置页新增 **A2A** Tab（仅 `agent_kind=llm` 显示完整 Endpoint 配置；Proxy 类型仅展示远程连接信息与只读 Card）
- 配置项：启用 A2A、capabilities 编辑、流式能力声明、暴露路径（由平台分配）
- 与现有 `a2a_agent_cards` 表及 `UpdateAgentCard` API 对齐
- 默认 `enabled=false`（与 §6.1 一致）

**验收标准**：
- [ ] LLM Agent 在设置页可启用 A2A 并发布 capabilities
- [ ] 启用后出现在 `/a2a` Discover 与 `call_agent` 可选目标中
- [ ] 未启用时 `call_agent` 调用返回明确错误

---

## 4. 涉及文件

### 4.1 已实现

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

### 4.2 待新增/修改

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/service/trpc_turn.go` | 修改 | 注入 A2A 上下文 |
| `internal/agent/trpc_build.go` | 修改 | TRPCBuilderDeps 增加 A2AUsecase |
| `internal/a2a/trpc/agent.go` | 新建 | A2AAgent 适配器 |
| `internal/a2a/trpc/server.go` | 新建 | A2A Server |
| `internal/a2a/trpc/converter.go` | 新建 | 消息转换 |
| `internal/a2a/client.go` | 新建 | A2A HTTP Client |
| `internal/a2a/graph_resume.go` | 新建 | Graph 恢复集成 |
| `api/kratos/a2a/v1/a2a.proto` | 修改 | 新增远程 Agent RPC |
| `internal/biz/a2a.go` | 修改 | 新增 A2ARemoteAgent 模型 |
| `internal/data/a2a.go` | 修改 | 新增 a2a_remote_agents 表 |
| `internal/service/a2a.go` | 修改 | 新增远程 Agent 方法 + Invoke 实际派发 |
| `web/src/features/a2a/` | 修改 | 新增管理页面 |

---

## 5. 验收标准总览

1. ✅ AgentCard 可通过 API 管理（CRUD + 发现）
2. ✅ 审计日志可浏览
3. ❌ Agent 可通过 `call_agent` 工具调用其他 Agent（Context 注入缺失）
4. ❌ `POST /v1/a2a/invoke` 可实际执行目标 Agent（当前仅 stub）
5. ❌ 跨工作区调用被拒绝（403）
6. ❌ 项目 Agent 可与远程 A2A Agent 通信
7. ❌ 外部 A2A 客户端可发现和调用项目 Agent
8. ❌ 消息双向转换无信息丢失
9. ❌ A2A 通信支持流式响应
10. ❌ A2A 长时间任务可中断/恢复 Graph
11. ❌ A2A 网关注册中心
12. ❌ 创建 Agent 时可选择 A2A 远程代理（`agent_kind=a2a_proxy`）
13. ❌ LLM Agent 设置页 A2A Endpoint Tab
14. ❌ A2A Proxy Agent 可在 Chat / Team / Graph 中选用

---

## 6. 运维指南

A2A 协议使 Aranea 平台内的 Agent 之间可进行结构化通信。Agent 可通过 `call_agent` 工具调用另一个 Agent 的命名能力，需显式启用、能力声明和工作区隔离。

### 6.1 核心原则

1. **默认关闭** — 每个 Agent 默认 A2A 禁用，管理员或 Agent 所有者必须显式启用 A2A 并发布能力列表。
2. **工作区隔离** — biz 层禁止跨工作区调用，`Discover` 和 `Invoke` 端点仅返回/接受同一工作区内的 Agent。
3. **审计日志** — 每次调用（成功或失败）写入 `a2a_audit` 记录。
4. **最小信任面** — `call_agent` 工具在分发前验证 Agent Card。

### 6.2 消息格式

#### 调用请求

```json
{
  "callee_agent_id": "agent-456",
  "capability": "summarize",
  "payload_json": "{\"text\": \"Long document...\"}",
  "caller_session_id": "sess-789",
  "timeout_seconds": 30
}
```

#### 调用响应

```json
{
  "invoke_id": "a2a-xxxxxxxx",
  "status": "pending | success | error | timeout",
  "result_json": "{}",
  "error_message": "",
  "duration_ms": 142
}
```

### 6.3 Agent Card

每个 A2A 启用的 Agent 发布一张 Card：

```json
{
  "agent_id": "agent-123",
  "display_name": "Research Assistant",
  "workspace": "workspace-A",
  "enabled": true,
  "capabilities": [
    {
      "name": "summarize",
      "description": "Summarize a block of text.",
      "input_schema_json": "{\"type\":\"object\",\"properties\":{\"text\":{\"type\":\"string\"}}}",
      "output_schema_json": "{\"type\":\"object\",\"properties\":{\"summary\":{\"type\":\"string\"}}}"
    }
  ]
}
```

### 6.4 组件

| 组件 | 路径 | 用途 | 状态 |
|------|------|------|------|
| Proto | `api/kratos/a2a/v1/a2a.proto` | HTTP + gRPC API | ✅ |
| Biz | `internal/biz/a2a.go` | 领域类型 + `A2ARepo` 接口 | ✅ |
| Data | `internal/data/a2a.go` | SQLite/Postgres 持久化 | ✅ |
| Tool | `internal/a2a/tool.go` | `call_agent` trpc 工具 | ✅ |
| Service | `internal/service/a2a.go` | Kratos 服务适配器 | ✅ |
| Context 注入 | `internal/service/trpc_turn.go` | 运行时上下文注入 | ❌ |
| A2A Agent | `internal/a2a/trpc/agent.go` | 远程 Agent 适配器 | ❌ |
| A2A Server | `internal/a2a/trpc/server.go` | A2A 服务暴露 | ❌ |
| 消息转换 | `internal/a2a/trpc/converter.go` | Event ↔ A2A Message | ❌ |
| Graph 恢复 | `internal/a2a/graph_resume.go` | A2A + Graph 集成 | ❌ |

### 6.5 数据库 Schema

由 `data.EnsureA2ASchema(ctx, db)` 创建：

```sql
a2a_agent_cards   (agent_id PK, display_name, workspace, enabled, capabilities JSON, updated_at)
a2a_invocations   (id PK, caller_agent_id, callee_agent_id, capability, payload_json, status, ...)
a2a_audit         (id PK, invoke_id, caller_agent_id, callee_agent_id, status, duration_ms, ...)
```

### 6.6 API 端点

| 方法 | 路径 | 说明 | 状态 |
|------|------|------|------|
| GET | `/v1/a2a/discover` | 发现已启用的 Agent | ✅ |
| POST | `/v1/a2a/invoke` | 调用能力（当前 stub） | ⚠️ |
| PUT | `/v1/a2a/agents/{agent_id}/card` | 更新 Agent A2A Card | ✅ |
| GET | `/v1/a2a/agents/{agent_id}/card` | 获取 Agent A2A Card | ✅ |
| GET | `/v1/a2a/audit` | 浏览审计日志 | ✅ |

### 6.7 Agent 工具：`call_agent`

运行前附加上下文辅助：

```go
ctx = a2a.WithA2AUsecase(ctx, a2aUsecase)
ctx = a2a.WithCallerAgentID(ctx, "agent-123")
ctx = a2a.WithInvoker(ctx, myInvokerFunc)
```

模型可调用：

```json
{
  "agent_id": "agent-456",
  "capability": "summarize",
  "payload": { "text": "Long document..." },
  "timeout_seconds": 30
}
```

工具流程：
1. 验证被调用方 Card（`enabled=true`）。
2. 验证能力在 Card 列表中。
3. 调用 `invokerFunc` 分发实际工作。
4. 写入审计条目（成功或错误）。

### 6.8 安全

| 控制 | 实现 | 状态 |
|------|------|------|
| 默认关闭 | 每个 Agent `enabled=false` | ✅ |
| 工作区隔离 | `ListEnabledCards` 按工作区过滤 | ✅ |
| 审计 | 每次调用写入 `a2a_audit`，含 caller/callee/status | ✅ |
| 速率限制 | 建议：在 `/v1/a2a/invoke` 上应用 API 网关限流 | ❌ |
| 跨工作区拒绝 | Invoke 端点校验 | ❌ |
| 远端鉴权 / mTLS | 跨实例 A2A | ❌ |

### 6.9 Prometheus 指标

| 指标 | 标签 | 说明 |
|------|------|------|
| `aranea_a2a_invoke_total` | `caller, callee, status` | 总调用次数 |
| `aranea_a2a_invoke_duration_seconds` | — | 调用延迟直方图 |

### 6.10 路由

- **同工作区**：通过服务层直接调用。
- **跨工作区**：默认禁止；未来网关路由为 S7+ 候选。

### 6.11 运维验收标准

- Agent A 调用不同工作区的 Agent B → `403 Forbidden`。
- Agent A 调用同工作区但 A2A 未启用的 Agent B → `call_agent` 报错。
- Agent A 调用同工作区且 A2A 启用的 Agent B → 成功；审计日志记录已写入。
