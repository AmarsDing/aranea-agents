# trpc-agent-go 框架说明

> **定位**：本文是 Aranea-Agents 项目对 `pkg/trpc-agent-go` 框架的工程化解读，帮助 AI 在实现功能时准确定位到参考依据。框架官方文档见 `pkg/trpc-agent-go/docs/mkdocs/zh/`。

---

## 1. 目录导读

```
pkg/trpc-agent-go/
├── agent/                  ← Agent 核心抽象与实现
│   ├── agent.go            ← Agent 接口定义（Run/Tools/Info/SubAgents）
│   ├── invocation.go       ← Invocation 运行上下文（Session/Model/Message/RunOptions）
│   ├── callbacks.go        ← Before/After Agent 回调
│   ├── plugins.go          ← PluginManager 接口（Agent/Model/Tool 回调 + OnEvent）
│   ├── stream_hub.go       ← 事件流 Hub（多订阅者广播）
│   ├── stream_mode.go      ← StreamMode 枚举
│   ├── llmagent/           ← LLMAgent（最常用）：LLM 驱动的单 Agent
│   │   ├── llm_agent.go    ← LLMAgent 结构体与 Run 实现
│   │   └── option.go       ← 全量 Option（Model/Instruction/Tools/Skill/Planner/…）
│   ├── graphagent/         ← GraphAgent：图工作流 Agent
│   │   └── option.go       ← GraphAgent Option（Graph/Checkpoint/Concurrency/…）
│   ├── chainagent/         ← ChainAgent：链式顺序执行
│   ├── cycleagent/         ← CycleAgent：循环迭代
│   ├── a2aagent/           ← A2A Agent：跨服务 Agent 互操作
│   ├── claudecode/         ← Claude Code Agent
│   ├── dify/               ← Dify Agent 适配
│   ├── n8n/                ← N8N Agent 适配
│   ├── structure/          ← Agent 结构导出
│   └── trace/              ← 执行追踪
├── runner/                 ← Runner：Agent 执行器
│   └── runner.go           ← Runner/ManagedRunner/SteerableRunner 接口 + 实现
├── model/                  ← LLM 模型层
│   ├── model.go            ← Model 接口（GenerateContent）+ IterModel 扩展
│   ├── request.go          ← Request 结构（Messages/Tools/Config/…）
│   ├── response.go         ← Response 结构（Choices/Usage/Error/Done/…）
│   ├── registry.go         ← 模型注册与 Context Window 查询
│   ├── openai/             ← OpenAI 兼容适配器
│   ├── anthropic/          ← Anthropic Claude 适配器
│   ├── gemini/             ← Google Gemini 适配器
│   ├── ollama/             ← Ollama 本地模型适配器
│   ├── hunyuan/            ← 腾讯混元适配器
│   ├── bedrock/            ← AWS Bedrock 适配器
│   ├── failover/           ← 故障转移模型
│   ├── hedge/              ← 对冲模型（竞速）
│   └── provider/           ← 统一 Provider 工厂（按名称创建 Model）
├── session/                ← 会话管理
│   ├── session.go          ← Session 结构（ID/Events/State/Summaries）
│   ├── state.go            ← State（Value + Delta 双层状态）
│   ├── ingestor.go         ← Session Ingestor（完成后回调）
│   ├── hook.go             ← Session Hook
│   ├── track.go            ← Track 事件轨道
│   ├── inmemory/           ← 内存 SessionService
│   ├── sqlite/             ← SQLite SessionService
│   ├── redis/              ← Redis SessionService
│   ├── postgres/           ← PostgreSQL SessionService
│   ├── mysql/              ← MySQL SessionService
│   └── pgvector/           ← pgvector SessionService
├── memory/                 ← 记忆系统
│   ├── memory.go           ← Service 接口（Add/Update/Delete/Search/Tools/EnqueueAutoMemoryJob）
│   ├── inmemory/           ← 内存 MemoryService
│   ├── sqlite/             ← SQLite MemoryService
│   ├── sqlitevec/          ← SQLite + 向量 MemoryService
│   ├── pgvector/           ← pgvector MemoryService
│   ├── postgres/           ← PostgreSQL MemoryService
│   ├── mysql/              ← MySQL MemoryService
│   ├── mysqlvec/           ← MySQL + 向量 MemoryService
│   ├── redis/              ← Redis MemoryService
│   ├── mem0/               ← Mem0 平台适配
│   ├── extractor/          ← 自动记忆提取器
│   └── tool/               ← 记忆工具（memory_add/search/load/…）
├── event/                  ← 事件系统
│   ├── event.go            ← Event 结构（Response/Author/Branch/Tag/StateDelta/…）
│   └── options.go          ← Event Option（WithResponse/WithBranch/WithTag/…）
├── tool/                   ← 工具系统
│   ├── tool.go             ← Tool/CallableTool/StreamableTool 接口 + Declaration/Schema
│   ├── function/           ← Function Tool（从 Go 函数创建）
│   ├── mcp/                ← MCP 协议工具
│   ├── agent/              ← AgentTool（子 Agent 作为工具）
│   ├── skill/              ← SkillTool（技能作为工具）
│   ├── transfer/           ← TransferTool（Agent 间转移控制权）
│   ├── awaitreply/         ← AwaitUserReply Tool
│   └── workspaceexec/      ← WorkspaceExec Tool
├── skill/                  ← 技能系统
│   ├── repository.go       ← Repository 接口（Summaries/Get/Path）
│   ├── context_repository.go ← ContextRepository（运行时技能加载/卸载）
│   ├── state_keys.go       ← Session State Key 常量
│   └── url_root.go         ← URL Root 解析
├── graph/                  ← 图工作流
│   ├── graph.go            ← Graph 构建（AddNode/AddEdge/ConditionalEdge）
│   ├── state_graph.go      ← StateGraph（状态驱动的图）
│   ├── executor.go         ← 执行器（BSP/DAG 调度）
│   ├── executor_dag.go     ← DAG 并行执行器
│   ├── checkpoint.go       ← 检查点（持久化/恢复）
│   ├── interrupt.go        ← 中断/恢复机制
│   ├── state.go            ← State 接口（Get/Set/Update）
│   ├── cache.go            ← 缓存
│   ├── retry.go            ← 节点重试策略
│   ├── visualize.go        ← 图可视化
│   └── time_travel.go      ← 时间旅行调试
├── team/                   ← 多 Agent 协作
│   ├── team.go             ← Team 结构（Coordinator/Swarm 模式）
│   ├── swarm.go            ← Swarm 模式实现
│   ├── swarm_members.go    ← Swarm 成员管理
│   ├── runtime.go          ← Team 运行时
│   ├── options.go          ← Team Option
│   └── structure_export.go ← Team 结构导出
├── planner/                ← 规划器
│   └── planner.go          ← Planner 接口（BuildPlanningInstruction/ProcessPlanningResponse）
├── plugin/                 ← 插件系统
│   ├── manager.go          ← Plugin Manager（注册 Before/After/OnEvent 钩子）
│   ├── logging.go          ← 日志插件
│   └── global_instruction.go ← 全局指令插件
├── artifact/               ← 制品管理
│   ├── artifact.go         ← Artifact 定义（Data/MimeType/URL/Name）
│   ├── service.go          ← Service 接口
│   ├── inmemory/           ← 内存制品服务
│   ├── cos/                ← 腾讯云 COS 制品服务
│   └── s3/                 ← S3 制品服务
├── codeexecutor/           ← 代码执行器
│   ├── codeexecutor.go     ← CodeExecutor 接口（ExecuteCode）
│   ├── e2b/                ← E2B 沙箱执行器
│   ├── local/              ← 本地执行器
│   └── registry.go         ← 执行器注册
├── knowledge/              ← 知识库
│   ├── knowledge.go        ← Knowledge 接口
│   ├── query/              ← 查询引擎
│   ├── ocr/                ← OCR 处理
│   ├── chunking/           ← 文档分块
│   ├── source/             ← 数据源
│   └── tool/               ← 知识检索工具
├── evaluation/             ← 评估系统
│   └── evaluation.go       ← Evaluation 框架
├── internal/               ← 内部实现（不直接依赖）
│   ├── flow/               ← LLM Flow（llmflow/processor）
│   ├── tool/               ← 内部工具辅助
│   ├── trace/              ← 追踪辅助
│   ├── a2a/                ← A2A 协议内部实现
│   ├── state/              ← 状态管理（flush/steer/barrier/appender）
│   └── jsonschema/         ← JSON Schema 处理
├── log/                    ← 日志
└── docs/mkdocs/zh/         ← 框架官方中文文档
```

---

## 2. 核心概念与接口速查

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

**关键实现**：

| 实现 | 包路径 | 适用场景 |
|------|--------|----------|
| LLMAgent | `agent/llmagent` | 单 Agent 对话，最常用 |
| GraphAgent | `agent/graphagent` | 图工作流编排 |
| ChainAgent | `agent/chainagent` | 链式顺序执行 |
| CycleAgent | `agent/cycleagent` | 循环迭代优化 |
| Team | `team` | 多 Agent 协作（Coordinator/Swarm） |

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

**Runner 职责**：获取/创建 Session → 生成 InvocationID → 调用 Agent → 追加事件到 Session → 发出 `runner.completion` 事件。

### 2.3 Model 接口

```go
type Model interface {
    GenerateContent(ctx context.Context, request *Request) (<-chan *Response, error)
    Info() Info
}
```

**双层错误处理**：函数返回 `error` = 系统级错误；`Response.Error` = API 级错误。

### 2.4 Event 结构

```go
type Event struct {
    *model.Response
    RequestID       string
    InvocationID    string
    Author          string
    Branch          string
    Tag             string
    StateDelta      map[string][]byte
    Extensions      map[string]json.RawMessage
    Actions         *EventActions
}
```

**关键判断方法**：
- `IsRunnerCompletion()` → 运行结束信号
- `IsError()` / `IsTerminalError()` → 错误判断
- `ContainsTag(tag)` → 标签过滤
- `Filter(filterKey)` → 分支过滤

### 2.5 Session 结构

```go
type Session struct {
    ID        string
    AppName   string
    UserID    string
    State     StateMap
    Events    []event.Event
    Summaries map[string]*Summary
}
```

**State 双层模型**：`Value`（已提交）+ `Delta`（待提交），通过 `State.Get` 优先读 Delta。

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
type Tool interface {
    Declaration() *Declaration
}
type CallableTool interface {
    Call(ctx context.Context, jsonArgs []byte) (any, error)
    Tool
}
```

---

## 3. 模块实现规则

### 3.1 Agent 构建

**规则**：通过 `llmagent.New(name, ...Option)` 构建，不直接实例化结构体。

```go
agent := llmagent.New("assistant",
    llmagent.WithModel(m),
    llmagent.WithInstruction("系统提示词"),
    llmagent.WithTools([]tool.Tool{...}),
    llmagent.WithGenerationConfig(model.GenerationConfig{Stream: true}),
)
```

**项目映射**：`internal/agent/trpc_build.go` → `BuildTRPCLLMAgent` 封装了上述构建逻辑，注入 Provider/Model/Skill/ToolSet。

### 3.2 Runner 创建

**规则**：通过 `runner.NewRunner(appName, agent, ...Option)` 创建。

```go
r := runner.NewRunner("my-app", rootAgent,
    runner.WithSessionService(sessSvc),
    runner.WithMemoryService(memSvc),
)
```

**项目映射**：`internal/agent/trpc_runtime.go` → `NewTRPCRunner` 封装了 Runner 创建，注入 SessionService/MemoryService。

### 3.3 Model 适配

**规则**：通过 `model/provider` 统一工厂创建，不直接 new 具体适配器。

```go
m, err := trpcprovider.Model("openai", "gpt-4o-mini", opts...)
```

**项目映射**：`internal/provider/trpc_llm.go` → `TRPCModelForProviderModel` 从 biz 层 Catalog 解析 Provider 配置，调用 `trpcprovider.Model` 创建 Model 实例，并包装 HA（failover/hedge）。

**支持的 Provider**：

| Provider | 包路径 | 说明 |
|----------|--------|------|
| openai | `model/openai` | OpenAI 及兼容 API |
| anthropic | `model/anthropic` | Claude 系列 |
| gemini | `model/gemini` | Google Gemini |
| ollama | `model/ollama` | Ollama 本地模型 |
| hunyuan | `model/hunyuan` | 腾讯混元 |
| bedrock | `model/bedrock` | AWS Bedrock |

### 3.4 Session 管理

**规则**：Runner 自动管理 Session 生命周期，业务层不直接操作 Session 内部。

**SessionService 后端**：

| 后端 | 包路径 | 适用场景 |
|------|--------|----------|
| inmemory | `session/inmemory` | 测试、临时会话 |
| sqlite | `session/sqlite` | 单机持久化 |
| redis | `session/redis` | 分布式 |
| postgres | `session/postgres` | 生产级 |
| mysql | `session/mysql` | 生产级 |
| pgvector | `session/pgvector` | 向量检索 |

**项目映射**：`internal/session/` 封装了 TRPC SessionService 适配。

### 3.5 Memory 集成

**规则**：通过 `runner.WithMemoryService` 注入，框架自动在 Agent 运行时调用 Memory 工具。

**记忆层级**：

| 层级 | Kind | 说明 |
|------|------|------|
| L0-L2 | fact | 稳定属性、偏好 |
| L2-L4 | episode | 时间事件 |

**项目映射**：`internal/memory/` 封装了 TRPC MemoryService 适配。

### 3.6 Tool 装配

**规则**：通过 `llmagent.WithTools` 或 `llmagent.WithToolSets` 注册，框架自动处理工具调用循环。

**工具类型**：

| 类型 | 包路径 | 说明 |
|------|--------|------|
| FunctionTool | `tool/function` | 从 Go 函数创建 |
| MCPTool | `tool/mcp` | MCP 协议工具 |
| AgentTool | `tool/agent` | 子 Agent 作为工具 |
| SkillTool | `tool/skill` | 技能作为工具 |
| TransferTool | `tool/transfer` | Agent 间转移 |
| MemoryTool | `memory/tool` | 记忆操作工具 |
| KnowledgeTool | `knowledge/tool` | 知识检索工具 |

**项目映射**：`internal/tools/trpc/` 封装了 TRPC ToolSet 适配。

### 3.7 Skill 系统

**规则**：通过 `llmagent.WithSkills(repo)` 注入技能仓库，框架自动管理技能加载/卸载。

```go
repo, _ := skill.NewDirectoryRepository("/path/to/skills")
agent := llmagent.New("assistant",
    llmagent.WithSkills(repo),
    llmagent.WithSkillToolProfile(llmagent.SkillToolProfileFull),
)
```

**SkillLoadMode**：
- `SkillLoadModeOnce`：加载一次后立即卸载
- `SkillLoadModeTurn`：当前 Invocation 内保持（默认）
- `SkillLoadModeSession`：跨 Invocation 保持

**项目映射**：`internal/skill/trpc/` 封装了 TRPC Skill Repository 适配。

### 3.8 Team 协作

**规则**：通过 `team.New(name, mode, ...Option)` 创建，Team 本身实现 Agent 接口。

```go
t := team.New("my-team", team.ModeSwarm,
    team.WithMembers(memberA, memberB),
    team.WithEntryMember("member-a"),
)
```

**模式**：

| 模式 | 说明 |
|------|------|
| ModeCoordinator | 协调者 Agent 调度成员作为工具 |
| ModeSwarm | 成员间 `transfer_to_agent` 传递控制权 |

**项目映射**：`internal/team/` 封装了 Team Builder 和 Workflow 构建。

### 3.9 Graph 工作流

**规则**：通过 `graph.New()` 构建图，`graphagent.New(name, g, ...Option)` 包装为 Agent。

```go
g := graph.New()
g.AddNode("step1", step1Func)
g.AddNode("step2", step2Func)
g.AddEdge(graph.Start, "step1")
g.AddEdge("step1", "step2")
g.AddEdge("step2", graph.End)

agent := graphagent.New("workflow", g,
    graphagent.WithInitialState(graph.NewState()),
)
```

**项目映射**：`internal/graph/` 封装了 TRPC Graph Builder。

### 3.10 Plugin 插件

**规则**：通过 `runner.WithPlugins(plugin)` 注册，全局作用于该 Runner 的所有 Invocation。

```go
r := runner.NewRunner("app", agent,
    runner.WithPlugins(myPlugin),
)
```

**Plugin 接口**：

```go
type Plugin interface {
    Name() string
    Register(r *Registry)
}
```

**Registry 暴露的钩子**：`BeforeAgent`、`AfterAgent`、`BeforeModel`、`AfterModel`、`BeforeTool`、`AfterTool`、`OnEvent`。

---

## 4. 项目内框架映射

### 4.1 框架包 → 项目包对照表

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

### 4.2 关键桥接函数

| 桥接函数 | 位置 | 作用 |
|----------|------|------|
| `BuildTRPCLLMAgent` | `internal/agent/trpc_build.go` | biz.Agent → trpcagent.Agent |
| `NewTRPCRunner` | `internal/agent/trpc_runtime.go` | 创建 Runner 并注入 Session/Memory |
| `RunTRPCUserTurn` | `internal/agent/trpc_runtime.go` | 执行一轮用户对话 |
| `TRPCModelForProviderModel` | `internal/provider/trpc_llm.go` | biz Provider 配置 → trpcmodel.Model |
| `BuildWorkflowRoot` | `internal/team/` | biz Team → trpcagent.Agent（Team 模式） |

### 4.3 依赖方向

```
internal/service
    ↓ 调用
internal/agent (BuildTRPCLLMAgent / NewTRPCRunner)
    ↓ 调用
internal/provider (TRPCModelForProviderModel)
internal/tools/trpc (ToolSet 适配)
internal/skill/trpc (Skill Repository 适配)
    ↓ 依赖
pkg/trpc-agent-go/* (框架核心)
```

**铁律**：`internal/biz` 不得 import `pkg/trpc-agent-go` 任何包。

---

## 5. 常见实现场景速查

### 5.1 新增 LLM Provider

1. 在 `internal/provider/trpc_llm.go` 的 `MapProviderType` 中添加映射
2. 如需自定义选项，在 `buildProviderOptions` 中添加分支
3. 确保框架 `model/<provider>/` 包已实现 `model.Model` 接口

### 5.2 新增 Tool 类型

1. 在 `internal/tools/trpc/` 中创建 ToolSet 适配
2. 在 `internal/agent/trpc_build.go` 的 `buildToolsetsForAgent` 中注册
3. 框架侧实现 `tool.CallableTool` 接口

### 5.3 新增 Session 后端

1. 在 `internal/session/` 中创建适配器
2. 通过 Wire 注入到 `TRPCRunnerDeps.SessionService`
3. 框架侧实现 `session.Service` 接口

### 5.4 新增 Memory 后端

1. 在 `internal/memory/` 中创建适配器
2. 通过 Wire 注入到 `TRPCRunnerDeps.MemoryService`
3. 框架侧实现 `memory.Service` 接口

### 5.5 新增 Agent 类型

1. 框架侧实现 `agent.Agent` 接口（Run/Tools/Info/SubAgents）
2. 在 `internal/agent/` 中创建构建函数
3. 在 `internal/service/` 中集成到 Runner 装配流程

### 5.6 事件流处理

1. Runner.Run 返回 `<-chan *event.Event`
2. 遍历事件流，通过 `event.IsRunnerCompletion()` 判断结束
3. 通过 `event.Object` 区分事件类型（`chat.completion.chunk`/`tool.call`/`tool.response`/`error`）
4. 在 `internal/service/trpc_turn.go` 中投影为 SSE 或 proto 响应

---

## 6. 框架官方文档索引

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
