# trpc-agent-go 框架接口速查

> **定位**：本文是 `pkg/trpc-agent-go` 框架的**接口与映射速查**，帮助 AI 在实现功能时准确定位到框架 API 和项目桥接点。
> **与 SPEC 的关系**：[AI-DEVELOPMENT-SPECIFICATION.md](./AI-DEVELOPMENT-SPECIFICATION.md) 是唯一行为准则（项目层约束），本文是框架层参考。
> **框架官方文档**：`pkg/trpc-agent-go/docs/mkdocs/zh/`（深度用法时查阅）。

---

## 目录

- [1. 框架目录速查](#1-框架目录速查)
- [2. 核心接口速查](#2-核心接口速查)
- [3. 项目内框架映射](#3-项目内框架映射)
- [4. 常见实现场景速查](#4-常见实现场景速查)
- [5. 框架官方文档索引](#5-框架官方文档索引)

---

## 1. 框架目录速查

```
pkg/trpc-agent-go/
├── agent/                  ← Agent 核心抽象
│   ├── agent.go            ← Agent 接口（Run/Tools/Info/SubAgents）
│   ├── invocation.go       ← Invocation 运行上下文
│   ├── callbacks.go        ← Before/After Agent 回调
│   ├── plugins.go          ← PluginManager 接口
│   ├── llmagent/           ← LLMAgent（最常用）：LLM 驱动的单 Agent
│   ├── graphagent/         ← GraphAgent：图工作流 Agent
│   ├── chainagent/         ← ChainAgent：链式顺序执行
│   ├── cycleagent/         ← CycleAgent：循环迭代
│   ├── a2aagent/           ← A2A Agent：跨服务互操作
│   ├── claudecode/         ← Claude Code Agent
│   └── structure/          ← Agent 结构导出
├── runner/                 ← Runner：Agent 执行器
├── model/                  ← LLM 模型层
│   ├── model.go            ← Model 接口 + IterModel 扩展
│   ├── request.go          ← Request 结构
│   ├── response.go         ← Response 结构（Choices/Usage/Error/Done）
│   ├── registry.go         ← 模型注册与 Context Window 查询
│   ├── openai/anthropic/gemini/ollama/hunyuan/bedrock/  ← 各厂商适配
│   ├── failover/           ← 故障转移模型
│   ├── hedge/              ← 对冲模型（竞速）
│   └── provider/           ← 统一 Provider 工厂
├── session/                ← 会话管理（inmemory/sqlite/redis/postgres/mysql/pgvector）
├── memory/                 ← 记忆系统（inmemory/sqlite/sqlitevec/pgvector/redis/mem0 + extractor + tool）
├── event/                  ← 事件系统（Event 结构 + Options）
├── tool/                   ← 工具系统
│   ├── tool.go             ← Tool/CallableTool/StreamableTool 接口
│   ├── function/           ← Function Tool
│   ├── mcp/                ← MCP 协议工具
│   ├── agent/              ← AgentTool（子 Agent 作为工具）
│   ├── skill/              ← SkillTool
│   ├── transfer/           ← TransferTool（Agent 间转移）
│   ├── awaitreply/         ← AwaitUserReply Tool
│   └── workspaceexec/      ← WorkspaceExec Tool
├── skill/                  ← 技能系统（Repository + ContextRepository）
├── graph/                  ← 图工作流（Graph/StateGraph/Executor/Checkpoint/Interrupt）
├── team/                   ← 多 Agent 协作（Coordinator/Swarm）
├── planner/                ← 规划器
├── plugin/                 ← 插件系统（Manager + logging + global_instruction）
├── artifact/               ← 制品管理（inmemory/cos/s3）
├── codeexecutor/           ← 代码执行器（e2b/local）
├── knowledge/              ← 知识库（query/ocr/chunking/source/tool）
├── evaluation/             ← 评估系统
└── internal/               ← 内部实现（不直接依赖）
```

---

## 2. 核心接口速查

### 2.1 Agent 接口

```go
type Agent interface {
    Run(ctx context.Context, invocation *Invocation) (<-chan *event.Event, error)
    Tools() []tool.Tool
    Info() Info
    SubAgents() []Agent
    FindSubAgent(name string) Agent
}
```

| 实现 | 包路径 | 适用场景 |
|------|--------|----------|
| LLMAgent | `agent/llmagent` | 单 Agent 对话，最常用 |
| GraphAgent | `agent/graphagent` | 图工作流编排 |
| ChainAgent | `agent/chainagent` | 链式顺序执行 |
| CycleAgent | `agent/cycleagent` | 循环迭代优化 |
| Team | `team` | 多 Agent 协作 |

### 2.2 Runner 接口

```go
type Runner interface {
    Run(ctx, userID, sessionID string, message model.Message, runOpts ...RunOption) (<-chan *event.Event, error)
    Close() error
}
type ManagedRunner interface {
    Runner
    Cancel(requestID string) bool
    RunStatus(requestID string) (RunStatus, bool)
}
type SteerableRunner interface {
    ManagedRunner
    EnqueueUserMessage(requestID string, message model.Message) error
}
```

### 2.3 Model 接口

```go
type Model interface {
    GenerateContent(ctx context.Context, request *Request) (<-chan *Response, error)
    Info() Info
}
```

**双层错误**：函数返回 `error` = 系统级；`Response.Error` = API 级。

### 2.4 Event 结构

```go
type Event struct {
    *model.Response
    RequestID, InvocationID, Author, Branch, Tag string
    StateDelta      map[string][]byte
    Extensions      map[string]json.RawMessage
    Actions         *EventActions
}
```

**关键判断**：`IsRunnerCompletion()` / `IsError()` / `IsTerminalError()` / `ContainsTag(tag)` / `Filter(filterKey)`

### 2.5 Session 结构

```go
type Session struct {
    ID, AppName, UserID string
    State     StateMap
    Events    []event.Event
    Summaries map[string]*Summary
}
```

**State 双层模型**：`Value`（已提交）+ `Delta`（待提交），`State.Get` 优先读 Delta。

### 2.6 Memory Service 接口

```go
type Service interface {
    AddMemory(ctx, userKey, memory, topics, ...AddOption) error
    UpdateMemory(ctx, memoryKey, memory, topics, ...UpdateOption) error
    DeleteMemory(ctx, memoryKey) error
    ClearMemories(ctx, userKey) error
    ReadMemories(ctx, userKey, limit) ([]*Entry, error)
    SearchMemories(ctx, userKey, query, ...SearchOption) ([]*Entry, error)
    Tools() []tool.Tool
    EnqueueAutoMemoryJob(ctx, sess) error
    Close() error
}
```

### 2.7 Tool 接口

```go
type Tool interface { Declaration() *Declaration }
type CallableTool interface { Call(ctx, jsonArgs) (any, error); Tool }
type StreamableTool interface { StreamableCall(ctx, jsonArgs) (*StreamReader, error); Tool }
```

---

## 3. 项目内框架映射

### 3.1 框架包 → 项目包对照

| 框架包 | 项目包 | 职责 |
|--------|--------|------|
| `agent/llmagent` | `internal/agent` | Agent 构建（BuildTRPCLLMAgent） |
| `runner` | `internal/agent` | Runner 创建（NewTRPCRunner） |
| `model/*` | `internal/provider` | LLM 模型适配（TRPCModelForProviderModel） |
| `session/*` | `internal/session` | 会话存储适配 |
| `memory/*` | `internal/memory` | 记忆服务适配 |
| `tool/*` | `internal/tools/trpc` | 工具集适配 |
| `skill` | `internal/skill/trpc` | 技能仓库适配 |
| `team` | `internal/team` | Team 编排 |
| `graph` | `internal/graph` | 图编排 |
| `plugin` | `internal/agent` | 插件注册（DefaultRunnerPlugins） |
| `event` | `internal/service` | 事件投影为 SSE/proto 响应 |

### 3.2 关键桥接函数

| 桥接函数 | 位置 | 作用 |
|----------|------|------|
| `BuildTRPCLLMAgent` | `internal/agent/trpc_build.go` | biz.Agent → trpcagent.Agent |
| `NewTRPCRunner` | `internal/agent/trpc_runtime.go` | 创建 Runner 并注入 Session/Memory |
| `RunTRPCUserTurn` | `internal/agent/trpc_runtime.go` | 执行一轮用户对话 |
| `TRPCModelForProviderModel` | `internal/provider/trpc_llm.go` | biz Provider 配置 → trpcmodel.Model |
| `BuildWorkflowRoot` | `internal/team/` | biz Team → trpcagent.Agent（Team 模式） |

### 3.3 依赖方向

```
internal/service
    ↓ 调用
internal/agent (BuildTRPCLLMAgent / NewTRPCRunner)
    ↓ 调用
internal/provider / internal/tools/trpc / internal/skill/trpc
    ↓ 依赖
pkg/trpc-agent-go/* (框架核心)
```

**铁律**：`internal/biz` 不得 import `pkg/trpc-agent-go` 任何包。

---

## 4. 常见实现场景速查

### 4.1 新增 LLM Provider

1. 在 `internal/provider/trpc_llm.go` 的 `MapProviderType` 中添加映射
2. 如需自定义选项，在 `buildProviderOptions` 中添加分支
3. 确保框架 `model/<provider>/` 包已实现 `model.Model` 接口

### 4.2 新增 Tool 类型

1. 在 `internal/tools/trpc/` 中创建 ToolSet 适配
2. 在 `internal/tools/registry.go` 的 `Registry()` 中注册 `ToolRegistration`
3. 在 `internal/tools/builtin_tools_seed.go` 中添加种子数据
4. 框架侧实现 `tool.CallableTool` 接口

### 4.3 新增 Session 后端

1. 在 `internal/session/` 中创建适配器
2. 通过 Wire 注入到 Runner 的 SessionService
3. 框架侧实现 `session.Service` 接口

### 4.4 新增 Memory 后端

1. 在 `internal/memory/` 中创建适配器
2. 通过 Wire 注入到 Runner 的 MemoryService
3. 框架侧实现 `memory.Service` 接口

### 4.5 新增 Agent 类型

1. 框架侧实现 `agent.Agent` 接口（Run/Tools/Info/SubAgents）
2. 在 `internal/agent/` 中创建构建函数
3. 在 `internal/service/` 中集成到 Runner 装配流程

### 4.6 事件流处理

1. `Runner.Run` 返回 `<-chan *event.Event`
2. 遍历事件流，`event.IsRunnerCompletion()` 判断结束
3. 通过 `event.Object` 区分事件类型
4. 在 `internal/service/trpc_turn.go` 中投影为 SSE 或 proto 响应

---

## 5. 框架官方文档索引

| 主题 | 路径 |
|------|------|
| 总览 | `pkg/trpc-agent-go/docs/mkdocs/zh/index.md` |
| Agent | `pkg/trpc-agent-go/docs/mkdocs/zh/agent.md` |
| Runner | `pkg/trpc-agent-go/docs/mkdocs/zh/runner.md` |
| Model | `pkg/trpc-agent-go/docs/mkdocs/zh/model.md` |
| Tool | `pkg/trpc-agent-go/docs/mkdocs/zh/tool.md` |
| Session | `pkg/trpc-agent-go/docs/mkdocs/zh/session.md` |
| Memory | `pkg/trpc-agent-go/docs/mkdocs/zh/memory.md` |
| Skill | `pkg/trpc-agent-go/docs/mkdocs/zh/skill.md` |
| Graph | `pkg/trpc-agent-go/docs/mkdocs/zh/graph.md` |
| Team | `pkg/trpc-agent-go/docs/mkdocs/zh/team.md` |
| Planner | `pkg/trpc-agent-go/docs/mkdocs/zh/planner.md` |
| Plugin | `pkg/trpc-agent-go/docs/mkdocs/zh/plugin.md` |
| Event | `pkg/trpc-agent-go/docs/mkdocs/zh/event.md` |
| Artifact | `pkg/trpc-agent-go/docs/mkdocs/zh/artifact.md` |
| CodeExecutor | `pkg/trpc-agent-go/docs/mkdocs/zh/codeexecutor.md` |
| Knowledge | `pkg/trpc-agent-go/docs/mkdocs/zh/knowledge/index.md` |
| Evaluation | `pkg/trpc-agent-go/docs/mkdocs/zh/evaluation.md` |
| Callbacks | `pkg/trpc-agent-go/docs/mkdocs/zh/callbacks.md` |
| Multi-Agent | `pkg/trpc-agent-go/docs/mkdocs/zh/multiagent.md` |
| A2A | `pkg/trpc-agent-go/docs/mkdocs/zh/a2a.md` |
| 错误处理 | `pkg/trpc-agent-go/docs/mkdocs/zh/error-handling.md` |
| 可观测性 | `pkg/trpc-agent-go/docs/mkdocs/zh/observability.md` |
