# Runner 运行器 — 开发计划

> **版本**：2026-05-19 | **状态**：🔄 开发中
> **需求**：[40 runner.md](./40%20runner.md) · **设计**：[40 runner.design.md](./40%20runner.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：M20

---

## 1. 模块定位

Runner 运行器：管理 Agent/Team 的运行生命周期，包括启动、停止、状态监控和资源回收。对标 trpc-agent-go `runner` 包，将项目从 Service 层自管理的运行模式升级为框架层完整驱动的 ManagedRunner + SteerableRunner。

**代码锚点**：
- `internal/agent/trpc_runtime.go` — NewTRPCRunner + 辅助函数
- `internal/agent/turn_helpers.go` — NewRunnerDepsFromRuntime + ConsumeEventStream
- `internal/agent/trpc_build.go` — Agent 构建链 + BuildCache
- `internal/service/chat.go` — activeRuns / pendingQueue / runStatuses / awaitChans
- `internal/service/trpc_turn.go` — runSingleAgentViaTRPC / processPendingQueue
- `internal/team/runner_team_trpc.go` — Team 运行
- `internal/plugin/trpc/runtime.go` — 插件热加载
- `internal/artifact/trpc/service.go` — ArtifactService 适配器
- `internal/tools/serviceawaitreply/tool.go` — AwaitUserReply ServiceTool

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Agent 运行 + 事件流 | ✅ | `NewTRPCRunner` 返回 `ManagedRunner` |
| Team 运行 | ✅ | `teamsNative.RunTurn` |
| 停止运行 | ✅ | `StopGeneration` + `CancelRun` + `CancelTRPCRun` |
| 运行状态查询 | ✅ | `GetRunStatus` RPC + `runStatuses` sync.Map |
| 待执行队列 | ✅ | `pendingQueue` + `processPendingQueue` |
| 插件注入 | ✅ | `plugintrpc.Runtime` + `WithPlugins` |
| AwaitUserReply（Service 层） | ✅ | `serviceawaitreply.ServiceTool` + `AwaitUserReply` RPC |
| ArtifactService 适配器 | ✅ | `internal/artifact/trpc/service.go` |
| SteerableRunner 支持 | ✅ | `EnqueueTRPCUserMessage` 辅助函数 |
| BuildCache LRU + TTL | ✅ | `cache.go` 10min TTL |
| ArtifactService 注入 Runner | 🟡 | 适配器已有，未通过 `WithArtifactService` 注入 |
| AgentFactory | ❌ | 未注册 `WithAgentFactory` |
| SessionIngestor | ❌ | 未实现 `WithSessionIngestor` |
| AwaitUserReplyRouting（框架层） | ❌ | 未启用 `WithAwaitUserReplyRouting` |
| AgentLookup | ❌ | Runner 未维护 Agent 注册表 |
| RalphLoop | ❌ | 未配置 `WithRalphLoop` |
| CancelRun RPC | ❌ | 仅有 `StopGeneration` |
| EnqueueUserMessage RPC | ❌ | 仅有辅助函数，无 RPC 入口 |
| RunnerRegistry / RunnerManager | ❌ | 每次请求创建临时 Runner |
| GetRunStatus 与 ManagedRunner 对齐 | ❌ | 仅返回 Service 层状态，缺少框架层详情 |

---

## 3. 差距与优先级

1. **P1**：ArtifactService 注入 Runner — 适配器已有，仅需连接
2. **P1**：CancelRun / EnqueueUserMessage RPC — 辅助函数已有，仅需 Proto + Service 入口
3. **P1**：GetRunStatus 与 ManagedRunner 对齐 — Proto 扩展 + Service 逻辑
4. **P2**：AgentFactory + AgentLookup — Team/Swarm 协作基础
5. **P2**：AwaitUserReplyRouting（框架层） — 跨 turn 路由
6. **P2**：SessionIngestor — 外部记忆摄入
7. **P3**：RalphLoop — 迭代验证循环
8. **P3**：RunnerRegistry / RunnerManager — 多 Runner 实例管理
9. **P3**：Web 前端 ChatRunnerStatus + ChatEnqueueMessage — UI 完善

---

## 4. 开发阶段

### Phase 1：低挂果实（已有基础，快速连接）

- ArtifactService 注入 Runner
- CancelRun / EnqueueUserMessage RPC
- GetRunStatus 与 ManagedRunner 对齐

### Phase 2：框架层集成

- AgentFactory + AgentLookup
- AwaitUserReplyRouting（框架层）
- SessionIngestor

### Phase 3：高级能力

- RalphLoop
- RunnerRegistry / RunnerManager

### Phase 4：前端完善

- ChatRunnerStatus.vue
- ChatEnqueueMessage.vue
- RunStatus 类型扩展

---

## 5. 任务清单

| # | 任务 | 优先级 | 阶段 | 涉及文件 |
|---|------|--------|------|---------|
| 1 | `TRPCRunnerDeps` 增加 `ArtifactService` 字段 + `NewTRPCRunner` 传入 `WithArtifactService` | P1 | 1 | `internal/agent/trpc_runtime.go`, `internal/agent/turn_helpers.go` |
| 2 | `NewRunnerDepsFromRuntime` 接收 `ArtifactService` 参数 | P1 | 1 | `internal/agent/turn_helpers.go` |
| 3 | Chat Proto 新增 `CancelRun` / `EnqueueUserMessage` RPC | P1 | 1 | `api/kratos/chat/v1/chat.proto` |
| 4 | `make api` 生成 Proto 代码 | P1 | 1 | — |
| 5 | `ChatService` 实现 `CancelRunRPC` / `EnqueueUserMessageRPC` | P1 | 1 | `internal/service/chat.go` |
| 6 | Chat Proto `RunStatus` 消息扩展（invocation_id 等字段） | P1 | 1 | `api/kratos/chat/v1/chat.proto` |
| 7 | `ChatService.GetRunStatus` 查询 `ManagedRunner.RunStatus` | P1 | 1 | `internal/service/chat.go` |
| 8 | 新建 `BizAgentFactory` + `trpcrunner.AgentFactory` 适配 | P2 | 2 | `internal/agent/factory.go`（新建） |
| 9 | `TRPCRunnerDeps` 增加 `AgentFactories` 字段 + `NewTRPCRunner` 传入 | P2 | 2 | `internal/agent/trpc_runtime.go` |
| 10 | `NewRunnerDepsFromRuntime` 接收 `AgentFactories` 参数 | P2 | 2 | `internal/agent/turn_helpers.go` |
| 11 | `TRPCRunnerDeps` 增加 `AwaitUserReplyRouting` 字段 + `NewTRPCRunner` 传入 | P2 | 2 | `internal/agent/trpc_runtime.go` |
| 12 | 新建 `BizSessionIngestor` 实现 `trpcsession.Ingestor` | P2 | 2 | `internal/agent/ingestor.go`（新建） |
| 13 | `TRPCRunnerDeps` 增加 `Ingestor` 字段 + `NewTRPCRunner` 传入 | P2 | 2 | `internal/agent/trpc_runtime.go` |
| 14 | AgentRuntimeSettings Ent Schema 新增 Runner/RalphLoop 字段 | P3 | 3 | `internal/data/ent/schema/agent_runtime_setting.go` |
| 15 | `make ent` 生成 Ent 代码 | P3 | 3 | — |
| 16 | 新建 `ralphLoopConfigFromSettings` 映射函数 | P3 | 3 | `internal/agent/trpc_runtime.go` |
| 17 | `TRPCRunnerDeps` 增加 `RalphLoop` 字段 + `NewTRPCRunner` 传入 | P3 | 3 | `internal/agent/trpc_runtime.go` |
| 18 | 新建 `RunnerRegistry` + `RunnerManager` | P3 | 3 | `internal/agent/runner_manager.go`（新建） |
| 19 | Wire 注入 `NewBizAgentFactory` / `NewBizSessionIngestor` / `NewRunnerRegistry` / `NewRunnerManager` | P2 | 2-3 | `internal/agent/wire.go` |
| 20 | 前端 `api.ts` 新增 `cancelRun` / `enqueueUserMessage` | P3 | 4 | `web/src/features/chat/api.ts` |
| 21 | 前端 `RunStatus` 类型扩展 | P3 | 4 | `web/src/features/chat/api.ts` |
| 22 | 新建 `ChatRunnerStatus.vue` | P3 | 4 | `web/src/features/chat/components/` |
| 23 | 新建 `ChatEnqueueMessage.vue` | P3 | 4 | `web/src/features/chat/components/` |

---

## 6. 验收标准

- [ ] Agent 可通过 ArtifactService 保存/加载制品
- [ ] 可通过 CancelRun RPC 按 requestID 取消运行
- [ ] 可通过 EnqueueUserMessage RPC 在运行中追加用户消息
- [ ] GetRunStatus 返回 ManagedRunner 完整状态信息
- [ ] Runner 可按名称动态创建 Agent（AgentFactory）
- [ ] TransferTool 可通过名称查找目标 Agent（AgentLookup）
- [ ] Agent 调用 await_user_reply 后下一轮消息自动路由（框架层）
- [ ] Session 完成后自动摄入外部记忆平台
- [ ] RalphLoop 支持迭代执行和验证
- [ ] 多个 Runner 实例可并行运行
- [ ] 前端显示运行状态、支持取消和追加消息

---

## 7. 依赖与风险

- **ArtifactService 注入**：需确认 `internal/artifact/trpc/service.go` 的 `ServiceAdapter` 与当前 Runner 创建流程的集成点
- **AgentFactory**：需确认 `BuildTRPCLLMAgent` 在工厂模式下是否能正确获取 `TRPCBuilderDeps`（Provider/Model 等参数来自请求上下文）
- **AwaitUserReplyRouting**：框架层路由与 Service 层 `serviceawaitreply.ServiceTool` 的交互需验证，避免两层机制冲突
- **RalphLoop**：需 AgentRuntimeSettings 数据库字段支持，依赖 Ent Schema 迁移
- **RunnerManager**：长生命周期 Runner 的资源回收策略需设计，避免内存泄漏
- **Proto 扩展**：每次修改需 `make api` 重新生成
