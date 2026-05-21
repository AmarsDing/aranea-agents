# Runner 运行器 — 开发计划

> **版本**：2026-05-19 | **状态**：✅ M1 完成（RunRegistry / RunGateway / RunnerManager）
> **需求**：[40 runner.md](./40%20runner.md) · **设计**：[40 runner.design.md](./40%20runner.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：M20

---

## 1. 模块定位

Runner 运行器：管理 Agent/Team 的运行生命周期，包括启动、停止、状态监控和资源回收。对标 trpc-agent-go `runner` 包，将项目从 Service 层自管理的运行模式升级为框架层完整驱动的 ManagedRunner + SteerableRunner。

**代码锚点**：
- `internal/agent/trpc_runtime.go` — NewTRPCRunner + 辅助函数
- `internal/agent/turn_helpers.go` — NewRunnerDepsFromRuntime + ConsumeEventStream
- `internal/agent/trpc_build.go` — Agent 构建链 + BuildCache
- `internal/runtime/run_registry.go` — RunRegistry（active run、pending cancel、run status）
- `internal/runtime/deps.go` — `PersistenceSet`（Session / Memory / AgentMCP / Artifact）
- `cmd/admin/wire.go` — `provideArtifactRuntimeService` → `NewPersistenceSet`
- `internal/service/chat.go` — ChatService 桥接 + pendingQueue / awaitChans / EnqueueUserMessage RPC
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
| 停止运行 | ✅ | HTTP `StopGeneration` + WS `cancel` → `RunRegistry.Cancel`（ManagedRunner / Team cancel / pending 取消） |
| 运行状态查询 | ✅ | `GetRunStatus` RPC → `RunRegistry.GetStatus` |
| 待执行队列 | ✅ | `pendingQueue` + `processPendingQueue`（仍由 ChatService 持有） |
| RunRegistry | ✅ | `internal/runtime/run_registry.go`；单测 `run_registry_test.go` |
| 插件注入 | ✅ | `plugintrpc.Runtime` + `WithPlugins` |
| AwaitUserReply（Service 层） | ✅ | `serviceawaitreply.ServiceTool` + `AwaitUserReply` RPC |
| ArtifactService 适配器 | ✅ | `internal/artifact/trpc/service.go` |
| SteerableRunner 支持 | ✅ | `EnqueueTRPCUserMessage` 辅助函数 |
| BuildCache LRU + TTL | ✅ | `cache.go` 10min TTL |
| ArtifactService 注入 Runner | ✅ | `PersistenceSet.Artifact` → `NewRunnerDepsFromRuntime` → `WithArtifactService` |
| AgentFactory | ✅ | `BizAgentFactoryOptions` 按 `agent_key` 注册 `WithAgentFactory` |
| SessionIngestor | 🟡 | `BizSessionIngestor` 记录 ingest 元数据；auto-memory 仍由 runner `EnqueueAutoMemoryJob` |
| AwaitUserReplyRouting（框架层） | ✅ | `RunnerManager`：`AwaitHook != nil` → `AwaitUserReplyRouting` |
| AgentLookup | ✅ | `BizAgentRegistryOptions` + Team `LookupAgents`；工厂仍作回退 |
| RalphLoop | ✅ | Ent + `RalphLoopConfigFromSettings` + `TurnRunnerSpec.RalphLoop` |
| CancelRun 独立 RPC | ❌ | 取消经 `StopGeneration`（HTTP）与 WS `cancel`（`ChatService.CancelRun`） |
| EnqueueUserMessage RPC | ✅ | `POST /v1/chat/enqueue`；WS `enqueue_message`；`SendChatMessage` active run 时优先 steerable enqueue |
| RunnerManager | ✅ | `RunnerManager.NewTurnRunner` 统一装配；每 turn 仍 Close；`RegistryKey` 支持长生命周期实例 |
| GetRunStatus 与 ManagedRunner 对齐 | ✅ | Proto 扩展 + active run 时合并 `ManagedRunner.RunStatus` |

---

## 3. 差距与优先级

1. ~~**P1**：ArtifactService 注入 Runner~~ ✅ — Wire + `PersistenceSet` + Chat/Team turn
2. ~~**P1**：EnqueueUserMessage RPC~~ ✅ — `POST /v1/chat/enqueue` + WS 闭环；CancelRun 独立 RPC 仍非目标（沿用 StopGeneration）
3. ~~**P1**：GetRunStatus 与 ManagedRunner 对齐~~ ✅
4. ~~**P2**：AgentFactory~~ ✅ — `internal/agent/factory.go`；AgentLookup 仍依赖 Runner 注册表
5. ~~**P2**：AwaitUserReplyRouting（框架层）~~ ✅
6. ~~**P2**：SessionIngestor~~ 🟡 — ingest hook + FlowLog；Mem0 等外部 backend 待接
7. ~~**P3**：RalphLoop~~ ✅ — Ent/SQL + 按 Agent 设置注入
8. ~~**P3**：RunnerManager~~ ✅ — 统一装配已落地；长生命周期按需 `RegistryKey`
9. **P3**：Web 前端 ChatRunnerStatus + ChatEnqueueMessage — UI 完善

---

## 4. 开发阶段

### Phase 1：低挂果实（已有基础，快速连接）

- ~~ArtifactService 注入 Runner~~ ✅
- ~~EnqueueUserMessage RPC~~ ✅（Cancel 仍用 StopGeneration + WS cancel）
- GetRunStatus 与 ManagedRunner 对齐

### Phase 2：框架层集成

- AgentFactory + AgentLookup
- AwaitUserReplyRouting（框架层）
- SessionIngestor

### Phase 3：高级能力

- RalphLoop
- RunRegistry ✅ / RunnerManager ✅

### Phase 4：前端完善

- ChatRunnerStatus.vue
- ChatEnqueueMessage.vue
- RunStatus 类型扩展

---

## 5. 任务清单

| # | 任务 | 优先级 | 阶段 | 涉及文件 |
|---|------|--------|------|---------|
| 1 | `TRPCRunnerDeps` 增加 `ArtifactService` 字段 + `NewTRPCRunner` 传入 `WithArtifactService` | P1 | 1 | `internal/agent/trpc_runtime.go`, `internal/agent/turn_helpers.go` | ✅ |
| 2 | `NewRunnerDepsFromRuntime` 接收 `ArtifactService` 参数 | P1 | 1 | `internal/agent/turn_helpers.go`, `internal/runtime/deps.go`, `cmd/admin/wire.go` | ✅ |
| 3 | Chat Proto 新增 `EnqueueUserMessage` RPC | P1 | 1 | `api/kratos/chat/v1/chat.proto` | ✅ |
| 4 | `make api` 生成 Proto 代码 | P1 | 1 | — | ✅ |
| 5 | `RunRegistry` + `ChatService.EnqueueUserMessage` | P1 | 1 | `internal/runtime/run_registry.go`、`internal/service/chat.go` | ✅ |
| 5b | WS `enqueue_message` 调用 EnqueueUserMessage（非 SendChatMessage） | P1 | 1 | `internal/server/ws.go` | ✅ |
| 6 | Chat Proto `RunStatus` 消息扩展（invocation_id 等字段） | P1 | 1 | `api/kratos/chat/v1/chat.proto` | ✅ |
| 7 | `ChatService.GetRunStatus` 查询 `ManagedRunner.RunStatus` | P1 | 1 | `internal/service/chat.go`, `internal/runtime/run_registry.go` | ✅ |
| 8 | 新建 `BizAgentFactory` + `trpcrunner.AgentFactory` 适配 | P2 | 2 | `internal/agent/factory.go` | ✅ |
| 9 | Chat/Team `NewTRPCRunner` 传入 `BizAgentFactoryOptions` | P2 | 2 | `trpc_turn.go`, `runner_team_trpc.go` | ✅ |
| 10 | Team 共享 `RunRegistry`（`SetRunRegistry` + `StoreRunner`） | P1 | 1 | `team/runner.go`, `chat.go`, `run_registry.go` | ✅ |
| 11 | `AwaitUserReplyRouting` 经 `RunnerManager.TurnRunnerSpec` 传入 `NewTRPCRunner` | P2 | 2 | `runner_manager.go`, `trpc_runtime.go` | ✅ |
| 12 | 新建 `BizSessionIngestor` 实现 `trpcsession.Ingestor` | P2 | 2 | `internal/agent/ingestor.go` | ✅（占位，外部 backend 待扩展） |
| 13 | `TRPCRunnerDeps` 增加 `Ingestor` 字段 + `NewTRPCRunner` 传入 | P2 | 2 | `internal/agent/trpc_runtime.go` | ✅ |
| 14 | AgentRuntimeSettings Ent Schema 新增 RalphLoop 字段 | P3 | 3 | `internal/data/ent/schema/agent_runtime_setting.go` | ✅ |
| 15 | `go generate` Ent 代码 | P3 | 3 | `internal/data/ent` | ✅ |
| 16 | `RalphLoopConfigFromSettings` 映射函数 | P3 | 3 | `internal/agent/ralph_loop.go` | ✅ |
| 17 | `TRPCRunnerDeps.RalphLoop` + `NewTRPCRunner` | P3 | 3 | `internal/agent/trpc_runtime.go` | ✅ |
| 18 | 新建 `RunnerManager`（RunRegistry 已在 `internal/runtime`） | P3 | 3 | `internal/runtime/runner_manager.go` | ✅ |
| 19 | Wire 注入 `NewBizAgentFactory` / `NewBizSessionIngestor` / `NewRunnerManager` | P2 | 2-3 | `cmd/admin/wire.go` |
| 20 | 前端 `api.ts` 新增 `enqueueUserMessage` | P3 | 4 | `web/src/features/chat/api.ts` | ✅ |
| 21 | 前端 `RunStatus` 类型扩展 | P3 | 4 | `web/src/features/chat/api.ts` |
| 22 | 新建 `ChatRunnerStatus.vue` | P3 | 4 | `web/src/features/chat/components/` | ✅ |
| 23 | 新建 `ChatEnqueueMessage.vue` | P3 | 4 | `web/src/features/chat/components/` | ✅ |
| 24 | Agent 设置页 Ralph Loop 表单 | P3 | 4 | `AgentRalphLoopSection.vue`, `ralphLoopConfig.ts` | ✅ |
| 25 | `ensureAgentRuntimePatches` + `02_agent_ralph_loop.sql` | P3 | 3 | `internal/data/agent_runtime_patch.go` | ✅ |
| 26 | `biz.ValidateRalphLoopSettings` + `ResolveRalphLoopTurn` | P1 | 2 | `internal/biz/ralph_loop.go`, `internal/agent/ralph_loop.go` | ✅ |
| 27 | A2A Runner Ralph + Lookup | P1 | 2 | `internal/service/a2a_endpoint.go` | ✅ |
| 28 | `useAgentRalphLoopForm` / `useAgentPlannerForm` | P3 | 4 | `web/src/features/agents/` | ✅ |

---

## 6. 验收标准

- [x] Agent 可通过 ArtifactService 保存/加载制品（Runner 已注入 `WithArtifactService`）
- [x] 可通过 HTTP `StopGeneration` 或 WS `cancel` 取消当前 session 运行
- [x] 可通过 EnqueueUserMessage RPC（`POST /v1/chat/enqueue`）或 WS `enqueue_message` 在运行中追加用户消息
- [x] GetRunStatus 返回 ManagedRunner 完整状态信息（运行中合并 invocation/agent/event 字段）
- [x] Runner 可按名称动态创建 Agent（AgentFactory 按 agent_key 注册）
- [x] TransferTool / `selectAgent` 可通过 agent_key 查找（`WithAgent` + `WithAgentFactory`）
- [x] Agent 调用 await_user_reply 后下一轮消息自动路由（框架层，需 `AwaitHook` + `WithAwaitUserReplyRouting`）
- [x] Session 摄入 hook（auto-memory 由 runner 入队；外部 Mem0 待扩展）
- [x] RalphLoop 支持迭代执行和验证（按 `agent_runtime_settings` 配置）
- [x] Runner 装配统一经 `RunnerManager`（多实例并行仍受 session 锁与 RunRegistry 约束）
- [x] 前端显示运行状态、支持取消和追加消息（`ChatRunnerStatus` / `ChatEnqueueMessage`）

---

## 7. 依赖与风险

- **ArtifactService 注入**：已接入 `provideArtifactRuntimeService` → `PersistenceSet` → Chat/Team `NewTRPCRunner`
- **AgentFactory**：需确认 `BuildTRPCLLMAgent` 在工厂模式下是否能正确获取 `TRPCBuilderDeps`（Provider/Model 等参数来自请求上下文）
- **AwaitUserReplyRouting**：框架层路由与 Service 层 `serviceawaitreply.ServiceTool` 的交互需验证，避免两层机制冲突
- **RalphLoop**：需 AgentRuntimeSettings 数据库字段支持，依赖 Ent Schema 迁移
- **RunnerManager**：长生命周期 Runner 的资源回收策略需设计，避免内存泄漏
- **Proto 扩展**：每次修改需 `make api` 重新生成
