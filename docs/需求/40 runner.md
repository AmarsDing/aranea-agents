# M20: Runner 运行器 — 详细需求

> 对标 `pkg/trpc-agent-go/runner` 包，完善项目的 Agent 运行器。

---

## 1. 现状分析

项目已有基础的 Runner 封装：
- `internal/agent/trpc_runtime.go`：`NewTRPCRunner` + `RunTRPCUserTurn`
- 支持基本的 Agent 运行和事件流

**缺失能力**：
1. 无 AgentFactory（动态 Agent 创建）
2. 无 PluginManager（插件管理）
3. 无 ArtifactService（制品服务）
4. 无 SessionIngestor（Session 摄入器）
5. 无 AwaitUserReplyRouting（用户回复路由）
6. 无 Status/Cancel（运行状态/取消）
7. 无 AgentLookup（Agent 查找）

---

## 2. trpc 框架参照

```
pkg/trpc-agent-go/runner/
├── runner.go              # Runner 接口和实现
├── await_user_reply.go    # AwaitUserReply 路由
├── ralph_loop.go          # 排队消息处理
└── agent_lookup.go        # Agent 查找
```

### Runner 接口

```go
type Runner interface {
    Run(ctx context.Context, userID, sessionID string, message model.Message, opts ...agent.RunOption) (<-chan *event.Event, error)
}
```

### Runner 构造选项

```go
func WithSessionService(service session.Service) Option
func WithMemoryService(service memory.Service) Option
func WithSessionIngestor(ingestor session.Ingestor) Option
func WithArtifactService(service artifact.Service) Option
func WithAgent(name string, ag agent.Agent) Option
func WithAgentFactory(name string, factory AgentFactory) Option
func WithPlugins(plugins ...plugin.Plugin) Option
func WithAwaitUserReplyRouting(enabled bool) Option
```

### AgentFactory

```go
type AgentFactory func(ctx context.Context, ro agent.RunOptions) (agent.Agent, error)
```

当 Runner 需要按名称查找 Agent 时，先查注册的 Agent 实例，再查 AgentFactory。

### SessionIngestor

```go
type Ingestor interface {
    IngestSession(ctx context.Context, sess *session.Session) error
}
```

Session 完成后自动摄入到外部长期记忆平台（如 Mem0）。

---

## 3. 需求清单

### 3.1 AgentFactory 动态创建

**需求**：Runner 支持按名称动态创建 Agent

**实现要点**：
- 在 `NewTRPCRunner` 中通过 `WithAgentFactory` 注册
- AgentFactory 根据请求参数（模型、prompt 等）动态构建 Agent
- 支持按 Agent Key 查找

**验收标准**：Runner 可按名称动态创建 Agent

### 3.2 PluginManager 集成

**需求**：Runner 支持注入 PluginManager

**实现要点**：
- 在 `NewTRPCRunner` 中通过 `WithPlugins` 注入
- PluginManager 管理 Agent/Model/Tool 三层回调
- OnEvent 事件回调

**验收标准**：Runner 执行中回调正确触发

### 3.3 ArtifactService 集成

**需求**：Runner 支持注入 ArtifactService

**实现要点**：
- 在 `TRPCRunnerDeps` 中增加 `ArtifactService`
- 通过 `WithArtifactService` 注入
- Agent 执行中可保存/加载制品

**验收标准**：Agent 可通过 ArtifactService 管理制品

### 3.4 SessionIngestor 集成

**需求**：Session 完成后自动摄入到外部记忆平台

**实现要点**：
- 在 `TRPCRunnerDeps` 中增加 `SessionIngestor`
- 通过 `WithSessionIngestor` 注入
- Session 完成后自动调用 Ingestor
- 可对接 Mem0 等外部平台

**验收标准**：Session 完成后自动摄入到外部记忆平台

### 3.5 AwaitUserReplyRouting

**需求**：支持用户回复路由

**实现要点**：
- 在 `NewTRPCRunner` 中通过 `WithAwaitUserReplyRouting(true)` 启用
- Agent 调用 `await_user_reply` 工具时记录路由
- 下一轮用户消息自动路由到指定 Agent

**验收标准**：Agent 可指定下一轮用户消息路由

### 3.6 Status/Cancel

**需求**：支持运行状态查询和取消

**实现要点**：
- Runner 维护活跃运行表
- `Status(requestID)` 返回运行状态
- `Cancel(requestID)` 取消运行

**验收标准**：可通过 API 查询运行状态和取消运行

### 3.7 AgentLookup

**需求**：支持按名称查找 Agent

**实现要点**：
- Runner 维护 Agent 注册表
- `WithAgent(name, agent)` 注册 Agent 实例
- `WithAgentFactory(name, factory)` 注册 Agent 工厂
- Team/Swarm 中 TransferTool 通过 AgentLookup 查找目标 Agent

**验收标准**：TransferTool 可通过名称查找目标 Agent

### 3.8 多 Runner 实例

**需求**：支持多个 Runner 实例并行

**实现要点**：
- 每个 Agent/Team 可有独立的 Runner 实例
- Runner 实例间共享 Session/Memory 服务
- Runner 实例间不共享 Agent 注册表

**验收标准**：多个 Runner 实例可并行运行

---

## 4. 涉及文件

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/agent/trpc_runtime.go` | 修改 | 完善 Runner 构造 |
| `internal/agent/trpc_factory.go` | 新建 | AgentFactory 实现 |
| `internal/plugin/trpc/manager.go` | 新建 | PluginManager |
| `internal/runtimedeps/deps.go` | 修改 | 增加 ArtifactService/Ingestor |

---

## 5. 验收标准总览

1. Runner 可按名称动态创建 Agent
2. Runner 执行中回调正确触发
3. Agent 可通过 ArtifactService 管理制品
4. Session 完成后自动摄入到外部记忆平台
5. Agent 可指定下一轮用户消息路由
6. 可通过 API 查询运行状态和取消运行
7. TransferTool 可通过名称查找目标 Agent
8. 多个 Runner 实例可并行运行
