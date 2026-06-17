# Runner 运行器 — 开发计划

> **版本**：2026-06-17 | **状态**：后端 ✅ 完成；前端 UI 组件待创建
> **需求**：[40-runner.md](./40-runner.md) · **设计**：[40-runner.design.md](./40-runner.design.md)
> **EP**：M20

---

## 1. 模块定位

Runner 运行器：管理 Agent/Team 的运行生命周期，包括启动、停止、状态监控和资源回收。对标 trpc-agent-go `runner` 包，将项目从 Service 层自管理的运行模式升级为框架层完整驱动的 ManagedRunner + SteerableRunner。

**代码锚点**：
- `internal/agent/trpc_runtime.go` — `TRPCRunnerDeps` + `NewTRPCRunner` + 辅助函数（`CancelTRPCRun`/`TRPCRunStatus`/`EnqueueTRPCUserMessage`）
- `internal/agent/turn_helpers.go` — `NewRunnerDepsFromRuntime` / `NewRunnerDepsFromRuntimeWithLogger` + `ConsumeEventStream`
- `internal/agent/trpc_build.go` — Agent 构建链入口（`BuildTRPCLLMAgent`）
- `internal/agent/builder_deps.go` — `TRPCBuilderDeps` 定义
- `internal/agent/cache.go` — `BuildCache`（LRU，无 TTL，显式失效）+ `BuildTRPCLLMAgentCached`
- `internal/agent/factory.go` — `BizAgentFactoryOptions`（AgentFactory 按 agent_key 注册）
- `internal/agent/lookup.go` — `BizAgentRegistryOptions`（AgentLookup 注册预构建实例）
- `internal/agent/ingestor.go` — `BizSessionIngestor`（SessionIngestor 占位实现）
- `internal/agent/ralph_loop.go` — `RalphLoopConfigFromSettings` / `ResolveRalphLoopTurn`
- `internal/biz/run_state_machine.go` — Run 显式状态机（AS-FSM-01）
- `internal/biz/ralph_loop.go` — `RalphLoopConfigured` / `ValidateRalphLoopSettings`
- `internal/runtime/run_registry.go` — `RunRegistry`（active run、pending cancel、run status）
- `internal/runtime/runner_manager.go` — `RunnerManager`（统一 Runner 装配）
- `internal/runtime/runner_registry.go` — `RunnerInstanceRegistry`（长生命周期 Runner 跟踪）
- `internal/runtime/run_status.go` — `FrameworkRunStatus` / `FrameworkRunStatusFromRunner`
- `internal/runtime/deps.go` — `PersistenceSet`（Session / Memory / AgentMCP / Artifact）+ `NewRunnerManagerFromPersist`
- `internal/runtime/pending_queue.go` — `PendingMessageQueue`
- `cmd/admin/wire.go` — `provideArtifactRuntimeService` → `NewPersistenceSet`；`NewRunnerManagerFromPersist` 注入 `TurnDeps.RunnerMgr`
- `internal/service/chat.go` — ChatService 桥接 + `EnqueueUserMessage` / `GetRunStatus` / `StopGeneration` / `AwaitUserReply` RPC
- `internal/service/chat_orchestrator.go` — `coalesceRunRegistry` / `coalescePendingQueue` + `NewChatOrchestrator`
- `internal/service/chat_orchestrator_turn.go` — `runSingleAgentViaTRPC`
- `internal/service/chat_orchestrator_turn_dispatch.go` — `processPendingQueue`
- `internal/team/runner.go` — Team `Runner` + `RunTurnFromInput`
- `internal/team/runner_config.go` — `RunnerConfig`（含 `Runs *rt.RunRegistry`）
- `internal/team/runner_team_trpc.go` — Team TRPC 运行（`runTeamTRPCFromInput`）
- `internal/plugin/trpc/runtime.go` — 插件热加载
- `internal/artifact/trpc/service.go` — ArtifactService 适配器
- `internal/tools/serviceawaitreply/tool.go` — AwaitUserReply ServiceTool
- `internal/data/ent/schema/agent_runtime_setting.go` — RalphLoop 字段
- `internal/data/sql/migrations/20260607_agent_runtime_patches.sql` — RalphLoop 列迁移
- `api/kratos/chat/v1/chat.proto` — Chat Service Proto（含 Runner 控制 RPC）
- `web/src/features/chat/api.ts` — 前端 API（`stopGeneration`/`enqueueMessage`/`getRunStatus`/`awaitUserReply`）
- `web/src/domain/types.ts` — `RunStatus` / `RunStatusValue` 类型
- `web/src/composables/useRunStatus.ts` — 运行状态轮询 composable
- `web/src/components/agents/AgentRalphLoopSection.vue` — Agent 设置页 Ralph Loop 表单
- `web/src/features/agents/ralphLoopConfig.ts` — Ralph Loop 前端配置
- `web/src/features/agents/useAgentRalphLoopForm.ts` — Ralph Loop 表单 composable
- `web/src/features/agents/useAgentPlannerForm.ts` — Planner 表单 composable

---

## 2. 现状评估

### 2.1 已有能力

| 能力 | 状态 | 证据 |
|------|------|------|
| Agent 运行 + 事件流 | ✅ | `NewTRPCRunner` → `ManagedRunner` |
| Team 运行 | ✅ | `Runner.RunTurnFromInput`（`internal/team/runner.go`） |
| 运行状态查询 | ✅ | `GetRunStatus` RPC；`RunRegistry` + 运行中合并 `ManagedRunner.RunStatus` |
| 停止/取消运行 | ✅ | `StopGeneration` / WS `cancel` → `RunRegistry.Cancel` |
| 运行中追加消息 | ✅ | `EnqueueUserMessage` RPC + WS `enqueue_message` |
| 待执行队列 | ✅ | `internal/runtime/pending_queue.go` + `ChatUsecase` |
| 插件注入 | ✅ | `plugintrpc.Runtime` + `WithPlugins` |
| ArtifactService | ✅ | `PersistenceSet.Artifact` → `WithArtifactService` |
| AgentFactory | ✅ | `BizAgentFactoryOptions` 按 `agent_key` 注册 |
| AgentLookup | ✅ | `BizAgentRegistryOptions` + Team `LookupAgents`；工厂仍作回退 |
| SessionIngestor | 🟡 | `BizSessionIngestor` 已注入；外部 backend 占位（避免与 auto-memory 重复） |
| AwaitUserReply（Service 层） | ✅ | `serviceawaitreply` + `AwaitUserReply` RPC |
| AwaitUserReplyRouting（框架层） | ✅ | `AwaitHook != nil` 时 `RunnerManager` 启用 |
| SteerableRunner | ✅ | `trpcrunner.EnqueueUserMessage` / `RunRegistry.EnqueueUserMessage` |
| BuildCache LRU | ✅ | `internal/agent/cache.go`（无 TTL，显式失效 + LRU 驱逐） |
| RunRegistry / RunnerManager | ✅ | `internal/runtime/run_registry.go`、`runner_manager.go` |
| Run 状态机（AS-FSM-01） | ✅ | `internal/biz/run_state_machine.go`（6 状态 + 8 转换规则） |
| RalphLoop | ✅ | Ent + `RalphLoopConfigFromSettings` + `TurnRunnerSpec.RalphLoop` |
| RunnerInstanceRegistry | ✅ | `internal/runtime/runner_registry.go`（长生命周期 Runner 跟踪） |

### 2.2 缺失或规划项

| 能力 | 状态 | 说明 |
|------|------|------|
| 外部 Session 摄入（Mem0 等） | 🟡 | ingest hook 已接；Mem0 等外部 backend 待扩展 |
| 独立 `CancelRun` RPC | — | 非目标；沿用 `StopGeneration` + WS `cancel` |
| Web 运行状态 / 追加消息 UI | 🟡 | API/类型层已就绪；`ChatRunnerStatus.vue`、`ChatEnqueueMessage.vue` 组件未创建 |

### 2.3 与 trpc-agent-go 框架对比

| 能力 | trpc-agent-go | 当前项目 | 状态 |
|------|--------------|---------|------|
| Runner.Run / Close | ✅ | ✅ | `NewTRPCRunner` → `ManagedRunner` |
| ManagedRunner.Cancel | ✅ | ✅ | `RunRegistry.Cancel` + `CancelTRPCRun` |
| ManagedRunner.RunStatus | ✅ | ✅ | `GetRunStatus` 合并 `FrameworkRunStatusFromRunner` |
| SteerableRunner.EnqueueUserMessage | ✅ | ✅ | `EnqueueUserMessage` RPC + `RunRegistry` |
| PluginManager 注入 | ✅ | ✅ | `plugintrpc.Runtime` + `WithPlugins` |
| AwaitUserReply（Service 层） | — | ✅ | `serviceawaitreply` + `AwaitUserReply` RPC |
| GetRunStatus RPC | — | ✅ | `ChatService.GetRunStatus` |
| StopGeneration / WS cancel | — | ✅ | `RunRegistry.Cancel` |
| PendingQueue | — | ✅ | `internal/runtime/pending_queue.go` |
| BuildCache LRU | — | ✅ | `internal/agent/cache.go`（无 TTL） |
| ArtifactService | ✅ | ✅ | `PersistenceSet` → `WithArtifactService` |
| AgentFactory | ✅ | ✅ | `BizAgentFactoryOptions` |
| SessionIngestor | ✅ | 🟡 | `BizSessionIngestor` 注入；外部 backend 待扩展 |
| AwaitUserReplyRouting（框架层） | ✅ | ✅ | `AwaitHook != nil` 时启用 |
| AgentLookup | ✅ | ✅ | `BizAgentRegistryOptions` + Team lookup map |
| RalphLoop | ✅ | ✅ | Ent + `RalphLoopConfigFromSettings` |
| 独立 CancelRun RPC | — | — | 非目标；沿用 StopGeneration |
| RunnerInstanceRegistry / RunnerManager | — | ✅ | `RunnerManager.NewTurnRunner`；每 turn 仍 `Close` |
| Web 运行状态 UI | — | 🟡 | API/类型层已就绪；`ChatRunnerStatus.vue`、`ChatEnqueueMessage.vue` 组件未创建 |

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
9. **P3**：Web 前端 ChatRunnerStatus + ChatEnqueueMessage — API/类型层已就绪，Vue 组件未创建

---

## 4. 开发阶段

### Phase 1：低挂果实（已有基础，快速连接）

- ~~ArtifactService 注入 Runner~~ ✅
- ~~EnqueueUserMessage RPC~~ ✅（Cancel 仍用 StopGeneration + WS cancel）
- ~~GetRunStatus 与 ManagedRunner 对齐~~ ✅

### Phase 2：框架层集成

- ~~AgentFactory + AgentLookup~~ ✅
- ~~AwaitUserReplyRouting（框架层）~~ ✅
- ~~SessionIngestor~~ ✅（占位，外部 backend 待扩展）

### Phase 3：高级能力

- ~~RalphLoop~~ ✅
- ~~RunRegistry~~ ✅ / ~~RunnerManager~~ ✅
- ~~Run 状态机（AS-FSM-01）~~ ✅

### Phase 4：前端完善

- ~~RunStatus 类型扩展~~ ✅（`web/src/domain/types.ts`）
- ~~api.ts 接口~~ ✅（`enqueueMessage` / `getRunStatus` / `stopGeneration` / `awaitUserReply`）
- ChatRunnerStatus.vue — 待创建
- ChatEnqueueMessage.vue — 待创建

---

## 5. 任务清单

| # | 任务 | 优先级 | 阶段 | 涉及文件 | 状态 |
|---|------|--------|------|---------|------|
| 1 | `TRPCRunnerDeps` 增加 `ArtifactService` 字段 + `NewTRPCRunner` 传入 `WithArtifactService` | P1 | 1 | `internal/agent/trpc_runtime.go`, `internal/agent/turn_helpers.go` | ✅ |
| 2 | `NewRunnerDepsFromRuntime` 接收 `ArtifactService` 参数 | P1 | 1 | `internal/agent/turn_helpers.go`, `internal/runtime/deps.go`, `cmd/admin/wire.go` | ✅ |
| 3 | Chat Proto 新增 `EnqueueUserMessage` RPC | P1 | 1 | `api/kratos/chat/v1/chat.proto` | ✅ |
| 4 | `make api` 生成 Proto 代码 | P1 | 1 | — | ✅ |
| 5 | `RunRegistry` + `ChatService.EnqueueUserMessage` | P1 | 1 | `internal/runtime/run_registry.go`、`internal/service/chat.go` | ✅ |
| 5b | WS `enqueue_message` 调用 EnqueueUserMessage（非 SendChatMessage） | P1 | 1 | `internal/server/ws.go` | ✅ |
| 6 | Chat Proto `RunStatus` 消息扩展（invocation_id 等字段） | P1 | 1 | `api/kratos/chat/v1/chat.proto` | ✅ |
| 7 | `ChatService.GetRunStatus` 查询 `ManagedRunner.RunStatus` | P1 | 1 | `internal/service/chat.go`, `internal/runtime/run_status.go` | ✅ |
| 8 | 新建 `BizAgentFactoryOptions` + `trpcrunner.AgentFactory` 适配 | P2 | 2 | `internal/agent/factory.go` | ✅ |
| 9 | Chat/Team `NewTurnRunner` 传入 `BizAgentFactoryOptions` | P2 | 2 | `chat_orchestrator_turn.go`, `runner_team_trpc.go` | ✅ |
| 10 | Team 共享 `RunRegistry`（经 `RunnerConfig.Runs` 注入） | P1 | 1 | `internal/team/runner_config.go`, `internal/service/chat.go`, `internal/runtime/run_registry.go` | ✅ |
| 11 | `AwaitUserReplyRouting` 经 `RunnerManager.TurnRunnerSpec` 传入 `NewTRPCRunner` | P2 | 2 | `internal/runtime/runner_manager.go`, `internal/agent/trpc_runtime.go` | ✅ |
| 12 | 新建 `BizSessionIngestor` 实现 `trpcsession.Ingestor` | P2 | 2 | `internal/agent/ingestor.go` | ✅（占位，外部 backend 待扩展） |
| 13 | `TRPCRunnerDeps` 增加 `Ingestor` 字段 + `NewTRPCRunner` 传入 | P2 | 2 | `internal/agent/trpc_runtime.go`, `internal/agent/turn_helpers.go` | ✅ |
| 14 | AgentRuntimeSettings Ent Schema 新增 RalphLoop 字段 | P3 | 3 | `internal/data/ent/schema/agent_runtime_setting.go` | ✅ |
| 15 | `go generate` Ent 代码 | P3 | 3 | `internal/data/ent` | ✅ |
| 16 | `RalphLoopConfigFromSettings` 映射函数 | P3 | 3 | `internal/agent/ralph_loop.go` | ✅ |
| 17 | `TRPCRunnerDeps.RalphLoop` + `NewTRPCRunner` | P3 | 3 | `internal/agent/trpc_runtime.go` | ✅ |
| 18 | 新建 `RunnerManager`（`RunnerInstanceRegistry` 在 `internal/runtime/runner_registry.go`） | P3 | 3 | `internal/runtime/runner_manager.go` | ✅ |
| 19 | Wire 注入 `NewRunnerManagerFromPersist` + `provideArtifactRuntimeService` | P2 | 2-3 | `cmd/admin/wire.go` | ✅ |
| 20 | 前端 `api.ts` 新增 `enqueueUserMessage` / `enqueueMessage` | P3 | 4 | `web/src/features/chat/api.ts` | ✅ |
| 21 | 前端 `RunStatus` 类型扩展 | P3 | 4 | `web/src/domain/types.ts` | ✅ |
| 22 | 新建 `ChatRunnerStatus.vue` | P3 | 4 | `web/src/features/chat/components/` | ❌ |
| 23 | 新建 `ChatEnqueueMessage.vue` | P3 | 4 | `web/src/features/chat/components/` | ❌ |
| 24 | Agent 设置页 Ralph Loop 表单 | P3 | 4 | `web/src/components/agents/AgentRalphLoopSection.vue`, `web/src/features/agents/ralphLoopConfig.ts` | ✅ |
| 25 | RalphLoop 列迁移（SQL 迁移文件） | P3 | 3 | `internal/data/sql/migrations/20260607_agent_runtime_patches.sql` | ✅ |
| 26 | `biz.ValidateRalphLoopSettings` + `ResolveRalphLoopTurn` | P1 | 2 | `internal/biz/ralph_loop.go`, `internal/agent/ralph_loop.go` | ✅ |
| 27 | A2A Runner Ralph + Lookup | P1 | 2 | `internal/service/a2a_endpoint.go` | ✅ |
| 28 | `useAgentRalphLoopForm` / `useAgentPlannerForm` | P3 | 4 | `web/src/features/agents/` | ✅ |
| 29 | Run 显式状态机（AS-FSM-01） | P2 | 3 | `internal/biz/run_state_machine.go` | ✅ |

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
- [x] Run 实体状态转换经显式状态机校验（`internal/biz/run_state_machine.go`，AS-FSM-01）
- [ ] 前端独立运行状态/追加消息组件（`ChatRunnerStatus.vue` / `ChatEnqueueMessage.vue` 待创建；API/类型层已就绪）

---

## 7. 依赖与风险

- **ArtifactService 注入**：✅ 已接入 `provideArtifactRuntimeService` → `PersistenceSet` → Chat/Team `NewTurnRunner`
- **AgentFactory**：✅ `BizAgentFactoryOptions` 按 `agent_key` 注册，`BuildTRPCAgentCached` 构建并缓存
- **AwaitUserReplyRouting**：✅ 框架层路由与 Service 层 `serviceawaitreply.ServiceTool` 互补运行，未发现冲突
- **RalphLoop**：✅ AgentRuntimeSettings 数据库字段已迁移，Ent Schema 已更新
- **RunnerManager**：✅ 统一装配已落地；长生命周期 Runner 通过 `RegistryKey` 支持，每 turn 仍默认 Close
- **SessionIngestor**：🟡 外部 Mem0 等 backend 待扩展，当前仅记录摄入元数据
- **前端 UI 组件**：❌ `ChatRunnerStatus.vue` / `ChatEnqueueMessage.vue` 待创建，API/类型层已就绪
- **Proto 扩展**：每次修改需 `make api` 重新生成
