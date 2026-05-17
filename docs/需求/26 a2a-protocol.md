# M15: A2A 协议 — 详细需求

> 对标 `pkg/trpc-agent-go/agent/a2aagent` + `internal/a2a`，实现 Agent-to-Agent 通信协议。
>
> **2026-05-17 现状对齐**：
> - ✅ `internal/a2a/tool.go` 已暴露 `call_agent` 工具；`internal/biz/a2a.go` / `internal/data/a2a.go` / `internal/service/a2a.go` 已通；`api/kratos/a2a/v1/*` proto/HTTP 已生成。
> - ❌ **`call_agent` 工具未在 `internal/agent/trpc_build.go` 注入到 Agent toolset**，Agent 实际无法调用远端 Agent。
> - ❌ 远端鉴权 / mTLS / 流式请求 尚未实现。
>
> 后续以 `guides/execution-plan.md` §3 EP-BIZ-02 为准。运维要点见下方 §6。

---

## 1. 现状分析（已过期，保留参考）

项目无 A2A 协议能力。Agent 间通信仅通过 Team 内部的 TransferTool 和 AgentTool，无法与外部 Agent 通信。

---

## 2. trpc 框架参照

```
pkg/trpc-agent-go/
├── agent/a2aagent/
│   ├── a2a_agent.go       # A2AAgent：与远程 A2A Agent 通信
│   ├── a2a_agent_option.go # 选项配置
│   ├── a2a_converter.go   # 事件与 A2A 消息转换
│   └── example_test.go
├── internal/a2a/
│   ├── a2a.go             # A2A 常量和工具函数
│   ├── graph_resume.go    # Graph 恢复与 A2A 集成
│   ├── state_delta.go     # StateDelta 与 A2A 映射
│   └── url.go             # A2A URL 工具
└── planner/a2ui/
    ├── a2ui.go            # A2UI Planner
    └── schema.go          # A2UI Schema
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
    // ...
}

func New(opts ...Option) (*A2AAgent, error)
```

A2AAgent 实现了 `agent.Agent` 接口，可作为子 Agent 或独立 Agent 使用。

### 核心能力

1. **AgentCard 自动发现**：通过 URL 获取远程 Agent 的 AgentCard
2. **消息转换**：将 trpc Event 转换为 A2A Message，反之亦然
3. **流式通信**：支持 SSE 流式响应
4. **DataPart 映射**：FunctionCall/FunctionResponse/CodeExecution 等 DataPart 类型映射
5. **状态传递**：通过 metadata 传递 session state
6. **Graph 恢复**：A2A 任务与 Graph 工作流的中断/恢复集成

---

## 3. 需求清单

### 3.1 A2AAgent 集成

**需求**：支持与远程 A2A Agent 通信

**实现要点**：
- 新建 `internal/a2a/trpc/agent.go`
- 包装 trpc `a2aagent.New` 为项目可用组件
- 支持通过 AgentCard URL 发现远程 Agent

**验收标准**：项目 Agent 可与远程 A2A Agent 通信

### 3.2 A2A Server

**需求**：将项目 Agent 暴露为 A2A 服务

**实现要点**：
- 新建 `internal/a2a/trpc/server.go`
- 使用 `trpc-a2a-go/server` 包创建 A2A Server
- 自动生成 AgentCard
- 注册 Agent 处理器

**验收标准**：外部 A2A 客户端可发现和调用项目 Agent

### 3.3 消息转换

**需求**：trpc Event 与 A2A Message 双向转换

**实现要点**：
- 集成 trpc `a2aagent/a2a_converter.go`
- FunctionCall → A2A DataPart
- CodeExecution → A2A DataPart
- StateDelta → A2A metadata

**验收标准**：消息在两个方向正确转换，无信息丢失

### 3.4 流式通信

**需求**：支持 A2A 流式响应

**实现要点**：
- 使用 A2A SSE 传输
- 事件流式转发
- 支持中途取消

**验收标准**：A2A 通信支持流式响应

### 3.5 Graph 恢复集成

**需求**：A2A 任务与 Graph 工作流的中断/恢复集成

**实现要点**：
- 集成 trpc `internal/a2a/graph_resume.go`
- A2A 长时间任务触发 Graph 中断
- 任务完成后恢复 Graph 执行

**验收标准**：A2A 长时间任务可中断 Graph，完成后恢复

### 3.6 A2A 网关注册中心（超越层）

**需求**：集中管理 A2A Agent 注册和发现

**实现要点**：
- 新建 `internal/a2a/registry/`
- Agent 注册：名称、描述、URL、能力
- Agent 发现：按能力搜索
- 健康检查：定期检查 Agent 可用性

**验收标准**：Agent 可通过注册中心发现和调用其他 Agent

### 3.7 API 端点

**需求**：通过 API 管理 A2A 连接

**实现要点**：
- `POST /a2a/agents` — 注册远程 A2A Agent
- `GET /a2a/agents` — 列出已注册 Agent
- `DELETE /a2a/agents/:id` — 移除 Agent
- `GET /a2a/agents/:id/card` — 获取 AgentCard

**验收标准**：通过 API 可管理 A2A 连接

---

## 4. 涉及文件

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/a2a/trpc/agent.go` | 新建 | A2AAgent 适配器 |
| `internal/a2a/trpc/server.go` | 新建 | A2A Server |
| `internal/a2a/trpc/converter.go` | 新建 | 消息转换 |
| `internal/a2a/registry/registry.go` | 新建 | 注册中心（超越层） |
| `internal/service/a2a.go` | 新建 | A2A 服务层 |
| `internal/server/register_a2a.go` | 新建 | A2A HTTP 端点 |
| `web/src/features/a2a/` | 新建 | 前端 A2A 管理 |

---

## 5. 验收标准总览

1. 项目 Agent 可与远程 A2A Agent 通信
2. 外部 A2A 客户端可发现和调用项目 Agent
3. 消息双向转换无信息丢失
4. A2A 通信支持流式响应
5. A2A 长时间任务可中断/恢复 Graph
6. A2A 网关注册中心（超越层）

---

## 6. 运维指南

> 原 `guides/a2a-protocol.md` 内容，2026-05-17 合入。

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

| 组件 | 路径 | 用途 |
|------|------|------|
| Proto | `api/kratos/a2a/v1/a2a.proto` | HTTP + gRPC API |
| Biz | `internal/biz/a2a.go` | 领域类型 + `A2ARepo` 接口 |
| Data | `internal/data/a2a.go` | SQLite/Postgres 持久化 |
| Tool | `internal/a2a/tool.go` | `call_agent` trpc 工具 |
| Service | `internal/service/a2a.go` | Kratos 服务适配器 |

### 6.5 数据库 Schema

由 `data.EnsureA2ASchema(ctx, db)` 创建：

```sql
a2a_agent_cards   (agent_id PK, display_name, workspace, enabled, capabilities JSON, updated_at)
a2a_invocations   (id PK, caller_agent_id, callee_agent_id, capability, payload_json, status, ...)
a2a_audit         (id PK, invoke_id, caller_agent_id, callee_agent_id, status, duration_ms, ...)
```

### 6.6 API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/a2a/discover` | 发现已启用的 Agent |
| POST | `/v1/a2a/invoke` | 调用能力 |
| PUT | `/v1/a2a/agents/{agent_id}/card` | 更新 Agent A2A Card |
| GET | `/v1/a2a/agents/{agent_id}/card` | 获取 Agent A2A Card |
| GET | `/v1/a2a/audit` | 浏览审计日志 |

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

| 控制 | 实现 |
|------|------|
| 默认关闭 | 每个 Agent `enabled=false` |
| 工作区隔离 | `ListEnabledCards` 按工作区过滤 |
| 审计 | 每次调用写入 `a2a_audit`，含 caller/callee/status |
| 速率限制 | 建议：在 `/v1/a2a/invoke` 上应用 API 网关限流 |

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
