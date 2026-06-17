# M20: Runner 运行器 — 详细需求

> 对标 `pkg/trpc-agent-go/runner` 包，完善项目的 Agent 运行器。
> 现状评估与开发进度：详见 [40-runner.development.md §2 现状评估](./40-runner.development.md#2-现状评估)

---

## 1. 需求清单

### 1.1 AgentFactory 动态创建

**用户故事**：作为平台用户，我希望 Team/Swarm 中的 Agent 可以按名称动态创建，无需在启动时预注册所有 Agent 实例。

**功能规格**：
- Runner 支持通过 `WithAgentFactory` 注册 Agent 工厂
- 当 Runner 按名称查找 Agent 时，先查注册的 Agent 实例，再查 AgentFactory
- AgentFactory 根据请求参数（模型、prompt 等）动态构建 Agent
- 支持按 Agent Key 查找

**验收标准**：
1. Runner 可按名称动态创建 Agent
2. 查找顺序：已注册实例 → AgentFactory → 未找到

### 1.2 PluginManager 集成

**用户故事**：作为平台管理员，我希望 Runner 执行过程中插件回调能正确触发，实现审计、拦截、增强等能力。

**功能规格**：
- Runner 通过 `WithPlugins` 接收插件列表
- 插件在 Agent/Model/Tool 三层回调点触发
- 插件可通过 `plugintrpc.Runtime` 热加载

**验收标准**：
1. Runner 执行中插件回调正确触发
2. 插件热加载后下次 Runner 创建生效

### 1.3 ArtifactService 集成

**用户故事**：作为 Agent 开发者，我希望 Agent 在执行过程中可以保存和加载制品（文件、数据等），实现持久化输出。

**功能规格**：
- Runner 通过 `WithArtifactService` 注入制品服务
- Agent 执行中可调用制品工具保存/加载制品
- 制品按 Session 隔离，支持版本管理

**验收标准**：
1. Agent 可通过 ArtifactService 保存制品
2. Agent 可通过 ArtifactService 加载制品
3. 制品按 Session 隔离

### 1.4 SessionIngestor 集成

**用户故事**：作为平台管理员，我希望 Session 完成后自动摄入到外部长期记忆平台，使 Agent 后续对话可以利用历史会话知识。

**功能规格**：
- Runner 通过 `WithSessionIngestor` 注入摄入器
- Session 完成后自动调用 Ingestor
- 可对接 Mem0 等外部记忆平台
- 摄入失败不阻塞主流程

**验收标准**：
1. Session 完成后自动摄入到外部记忆平台
2. 摄入失败不影响 Agent 运行结果

### 1.5 AwaitUserReplyRouting（框架层）

**用户故事**：作为 Agent 开发者，我希望 Agent 调用 `await_user_reply` 后，下一轮用户消息能自动路由到该 Agent，无需前端手动指定。

**功能规格**：
- Runner 通过 `WithAwaitUserReplyRouting(true)` 启用框架层路由
- Agent 调用 `await_user_reply` 工具时在 Session 状态中记录路由
- 下一轮用户消息到达时，Runner 自动从 Session 状态读取路由并选择对应 Agent
- 路由消费后自动清除（一次性路由）

**验收标准**：
1. Agent 调用 `await_user_reply` 后，下一轮用户消息自动路由到该 Agent
2. 路由消费后自动清除

### 1.6 运行控制 API 完善

**用户故事**：作为前端用户，我希望可以通过 API 查询运行状态、取消运行、在运行中追加消息，实现更精细的运行控制。

**功能规格**：
- 取消运行：通过 `StopGeneration` RPC（HTTP）或 WS `cancel` 取消指定 session 的运行
- `EnqueueUserMessage` RPC：在运行中的 Agent 工具调用边界后注入用户消息
- `GetRunStatus` RPC：查询运行状态（与 ManagedRunner.RunStatus 对齐）

**验收标准**：
1. 可通过 API 取消运行（`StopGeneration` / WS `cancel`）
2. 可通过 API 在运行中追加用户消息（`POST /v1/chat/enqueue`）
3. 运行状态查询返回 ManagedRunner 的完整状态信息

### 1.7 AgentLookup

**用户故事**：作为 Agent 开发者，我希望 Team/Swarm 中的 TransferTool 可以通过 Agent 名称查找目标 Agent，实现 Agent 间协作。

**功能规格**：
- Runner 维护 Agent 注册表（名称 → Agent 实例）
- `WithAgent(name, agent)` 注册 Agent 实例
- `WithAgentFactory(name, factory)` 注册 Agent 工厂
- TransferTool 通过 AgentLookup 查找目标 Agent

**验收标准**：
1. TransferTool 可通过名称查找目标 Agent
2. 查找支持已注册实例和工厂回退

### 1.8 RalphLoop

**用户故事**：作为 Agent 开发者，我希望 Agent 可以在验证循环中反复执行，直到满足完成条件或达到最大迭代次数，提高任务完成质量。

**功能规格**：
- Runner 通过 `WithRalphLoop` 配置迭代验证循环
- 支持完成承诺（CompletionPromise）：Agent 输出包含特定标记时停止
- 支持验证命令（VerifyCommand）：执行外部命令验证任务完成
- 支持最大迭代次数限制
- 验证失败时将反馈注入下一轮迭代

**验收标准**：
1. Agent 在验证循环中反复执行直到满足完成条件
2. 达到最大迭代次数后自动停止
3. 验证失败反馈注入下一轮迭代

### 1.9 多 Runner 实例

**用户故事**：作为平台管理员，我希望多个 Agent/Team 可以拥有独立的 Runner 实例并行运行，互不干扰。

**功能规格**：
- 每个 Agent/Team 可有独立的 Runner 实例
- Runner 实例间共享 Session/Memory 服务
- Runner 实例间不共享 Agent 注册表
- 支持 Runner 的注册、查找、注销

**验收标准**：
1. 多个 Runner 实例可并行运行
2. Runner 实例间 Agent 注册表隔离
3. Runner 实例可注册和注销

---

## 2. 非功能需求

| 维度 | 要求 |
|------|------|
| 并发安全 | RunRegistry 基于 `sync.Map` 提供类型安全的并发访问；每 session 互斥锁防止并发 turn |
| 可观测性 | Runner 创建、turn 启动、cancel、enqueue 等关键节点经 `loggateway.Logger` 输出结构化日志 |
| 错误隔离 | SessionIngestor 摄入失败不阻塞主流程；插件回调异常不影响 Runner 主循环 |
| 资源回收 | 每 turn 默认 `Close` Runner；长生命周期实例经 `RegistryKey` 注册，由 `CloseRunner` 释放 |
| 状态一致性 | Run 实体状态转换经显式状态机校验（AS-FSM-01，详见设计文档 §三） |

---

## 3. 验收标准总览

1. Runner 可按名称动态创建 Agent
2. Runner 执行中插件回调正确触发
3. Agent 可通过 ArtifactService 管理制品
4. Session 完成后自动摄入到外部记忆平台
5. Agent 可指定下一轮用户消息路由
6. 可通过 API 查询运行状态、取消运行（`StopGeneration`）、追加消息（`EnqueueUserMessage`）
7. TransferTool 可通过名称查找目标 Agent
8. Agent 可在验证循环中反复执行直到满足完成条件
9. 多个 Runner 实例可并行运行
