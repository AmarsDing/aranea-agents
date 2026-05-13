# M15: A2A 协议 — 详细需求

> 对标 `pkg/trpc-agent-go/agent/a2aagent` + `internal/a2a`，实现 Agent-to-Agent 通信协议。

---

## 1. 现状分析

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
