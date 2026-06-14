# Always-Ready Agent 架构优化实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Agent 构建从"按需构建 + LRU 缓存"转变为"常驻预热 + 变更驱动重建"，消除用户发送指令后 2-15 秒的 Agent 构建延迟和 0.2-5 秒的 MCP 刷新延迟。

**Architecture:** Agent Pool（缓存增强：消除 TTL + 启动预热 + 标记脏后台重建 + 缓存键简化）+ MCP Tool Snapshot（后台预刷新 + 请求时快照读取）+ Turn 进度反馈（复用 execution_progress）+ Intent Pass 并行化 + 关键路径并行化。

**Tech Stack:** Go + trpc-agent-go + EventBus + WebSocket

---

## 文件结构

| 操作 | 文件 | 职责 |
|------|------|------|
| 修改 | `internal/agent/cache.go` | Agent Pool：消除 TTL、启动预热、标记脏后台重建、缓存键简化 |
| 修改 | `internal/agent/trpc_build.go` | Agent 构建时不再固定 Provider/Model |
| 修改 | `internal/service/chat_orchestrator_turn_phases.go` | 请求时注入 Model RunOption + BUILD 阶段发送进度事件 + Intent Pass 并行化 |
| 修改 | `internal/service/chat_orchestrator_turn.go` | 关键路径并行化（Sessions.Get + hydratedAgent） |
| 修改 | `internal/service/chat_orch_agent_build.go` | BuildTRPCDeps 内部 DB 查询并行化 |
| 新增 | `internal/agent/mcp_snapshot.go` | MCP Tool Snapshot Manager |
| 修改 | `internal/service/mcp_server.go` | MCP CRUD 触发 Snapshot 刷新 |
| 修改 | `web/src/features/chat/streamHandlers.ts` | 前端处理进度事件 + streaming 占位提前创建 |
| 修改 | `web/src/features/chat/composables/useChatStreamManager.ts` | streaming 占位逻辑 |

---

## Task 1: Agent Pool 缓存键简化

**Files:**
- Modify: `internal/agent/cache.go`
- Modify: `internal/agent/trpc_build.go`

**影响范围分析：**
- `BuildCacheKey` 当前包含 Provider/Model/DialogMode，移除后同一 Agent 不同模型共享缓存条目
- `resolveBaseModel()` 已支持 `RunOptions.Model` / `RunOptions.ModelName` 覆盖（llm_agent.go:1416-1436），无需修改框架
- 所有 8 个 `BuildTRPCAgentCached` 调用点不再需要传入 Provider/Model/DialogMode 参数
- 风险：如果请求时未注入 Model RunOption，Agent 会使用默认模型。需要确保所有调用点都注入了正确的 Model

- [ ] **Step 1: 修改 BuildCacheKey 移除 Provider/Model/DialogMode**

修改 `internal/agent/cache.go` 中的 `BuildCacheKey` 函数，从 fingerprint 中移除 Provider/Model/DialogMode 字段。同时修改 `fingerprint` 结构体。

- [ ] **Step 2: 修改 BuildTRPCAgentCached 调用签名**

修改 `BuildTRPCAgentCached` 不再接收 Provider/Model/DialogMode 参数。修改 `BuildTRPCLLMAgent` 不再使用 deps 中的 Provider/Model 作为 WithModel 参数，改为使用 Agent 自身的 Provider/Model 作为默认值。

- [ ] **Step 3: 更新所有 BuildTRPCAgentCached 调用点**

更新 8 个调用点，移除 Provider/Model/DialogMode 参数传递。

- [ ] **Step 4: 验证编译通过**

Run: `go build ./internal/agent/... ./internal/service/... ./internal/team/... ./internal/graph/...`

---

## Task 2: 请求时注入 Provider/Model RunOption

**Files:**
- Modify: `internal/service/chat_orchestrator_turn_phases.go`
- Modify: `internal/agent/trpc_runtime.go`

**影响范围分析：**
- 当前 `BuildTRPCLLMAgent` 中 `WithModel(m)` 固定 Provider/Model
- 改为 Agent 以默认模型构建，请求时通过 `agent.WithModel(m)` RunOption 注入
- `resolveBaseModel()` 优先级：SurfacePatch > RunOptions.Model > RunOptions.ModelName > Agent 默认 model
- 风险：需要确保 `TRPCModelForProviderModel` 在请求时也能正确解析模型

- [ ] **Step 1: 修改 BuildTRPCLLMAgent 使用 Agent 默认模型**

在 `BuildTRPCLLMAgent` 中，Provider/Model 使用 Agent 自身的值（`ag.Provider`/`ag.Model`），不再从 deps 中获取。

- [ ] **Step 2: 在 buildTurnRunOptions 中注入 Model RunOption**

在 `chat_orchestrator_turn_phases.go` 的 `buildTurnRunOptions()` 中，添加 `agent.WithModel(resolvedModel)` RunOption，其中 resolvedModel 通过 `provider.TRPCModelForProviderModel` 解析。

- [ ] **Step 3: 验证编译通过**

Run: `go build ./internal/service/... ./internal/agent/...`

---

## Task 3: Agent Pool 消除 TTL + 启动预热 + 标记脏后台重建

**Files:**
- Modify: `internal/agent/cache.go`

**影响范围分析：**
- 消除 TTL：当前 10 分钟空闲后缓存自动过期，改为常驻只在配置变更时重建
- 启动预热：在 ReadinessGate 后遍历活跃 Agent 触发构建，增加启动时间但消除首次请求 miss
- 标记脏后台重建：`InvalidateAgentCache` 不再直接清空，而是标记脏 + 后台异步重建
- 风险：内存占用可能增加（不再自动淘汰空闲 Agent），但 128 条目上限的 LRU 仍然有效
- 风险：配置变更后有短暂的旧配置服务窗口（直到重建完成），但不会阻塞请求

- [ ] **Step 1: 消除 TTL**

修改 `buildCacheEntry` 移除 `expiresAt` 字段，修改 `get()` 移除 TTL 检查，修改 GC 协程移除 `sweepExpired` 逻辑。

- [ ] **Step 2: 添加标记脏 + 后台重建机制**

在 `BuildCache` 中添加 `dirtySet map[string]struct{}` 和 `rebuildCh chan string`。修改 `InvalidateAgentCache` 为标记脏 + 发送重建任务。添加后台 goroutine 消费 rebuildCh 执行重建。

- [ ] **Step 3: 添加启动预热函数**

添加 `WarmupAgentPool(ctx context.Context, agents []biz.Agent, depsFunc func(biz.Agent) (TRPCBuilderDeps, error))` 函数。

- [ ] **Step 4: 验证编译通过**

Run: `go build ./internal/agent/...`

---

## Task 4: MCP Tool Snapshot Manager

**Files:**
- Create: `internal/agent/mcp_snapshot.go`
- Modify: `internal/agent/trpc_build.go`
- Modify: `internal/service/mcp_server.go`

**影响范围分析：**
- 新增 MCPToolSnapshotManager：后台定期刷新 MCP 工具列表，请求时快照读取
- 保持 per-Agent 连接模型（不共享连接），per-User 认证和 stdio 传输不受影响
- 禁用 `WithRefreshToolSetsOnRun`，改为从 SnapshotManager 读取工具列表
- 风险：工具列表有最多 30 秒延迟窗口，但 MCP 工具变更频率极低
- 风险：连接断开时需要降级为同步刷新

- [ ] **Step 1: 创建 MCPToolSnapshotManager**

定义接口和实现，包含 Register/Snapshot/Refresh/Close 方法。

- [ ] **Step 2: 禁用 WithRefreshToolSetsOnRun**

修改 `trpc_build.go` 中 `WithRefreshToolSetsOnRun(true)` 为 `false`。

- [ ] **Step 3: 在 getFilteredTools 路径中集成 SnapshotManager**

修改工具列表获取逻辑，优先从 SnapshotManager 读取快照。

- [ ] **Step 4: MCP CRUD 触发 Snapshot 刷新**

在 `mcp_server.go` 的 CRUD 操作中，调用 SnapshotManager.Refresh。

- [ ] **Step 5: 验证编译通过**

Run: `go build ./internal/agent/... ./internal/service/...`

---

## Task 5: Turn 进度反馈

**Files:**
- Modify: `internal/service/chat_orchestrator_turn_phases.go`

**影响范围分析：**
- 复用已有 `execution_progress` Envelope 类型，无需新增事件类型
- 前端已有 `onExecutionProgress` handler，无需前端改动即可工作
- EventBus 负载增量可忽略（每 turn 2-3 次事件）
- 风险：无，旧前端静默忽略未知事件

- [ ] **Step 1: 在 BUILD 阶段发送 execution_progress 事件**

在 `buildTurnRunner()` 开始和结束时，通过 `emitter.EmitProgress()` 发送进度事件。

- [ ] **Step 2: 在 Intent Pass 阶段发送 execution_progress 事件**

在 `runIntentPass()` 开始和结束时，发送进度事件。

- [ ] **Step 3: 验证编译通过**

Run: `go build ./internal/service/...`

---

## Task 6: Intent Pass 与 BUILD 并行化

**Files:**
- Modify: `internal/service/chat_orchestrator_turn_phases.go`

**影响范围分析：**
- 当前 Intent Pass 在 BUILD 之后串行执行
- 改为与 BUILD 并行执行，Intent Pass 结果注入后续 step
- 风险：Intent Pass 结果（intentRunOpts）当前在 `invokeTurnLLMAndStream` 中使用，需要确保并行执行后结果正确注入
- 风险：Intent Pass 失败不应阻塞 BUILD 和 LLM 调用

- [ ] **Step 1: 重构 executeTurn 使 BUILD 和 Intent Pass 并行**

使用 errgroup 并行执行 buildTurnRunner 和 runIntentPass，合并结果后继续执行。

- [ ] **Step 2: 验证编译通过**

Run: `go build ./internal/service/...`

---

## Task 7: 关键路径并行化

**Files:**
- Modify: `internal/service/chat_orchestrator_turn.go`
- Modify: `internal/service/chat_orch_agent_build.go`

**影响范围分析：**
- Sessions.Get 和 hydratedAgent 可以并行（无依赖关系）
- BuildTRPCDeps 中的 GetEffectiveTools、computeSkillHash、computeMCPHash 可以并行
- 风险：并行化增加了代码复杂度，但减少了 100-300ms 延迟
- 风险：errgroup 中的错误处理需要正确聚合

- [ ] **Step 1: Sessions.Get + hydratedAgent 并行化**

在 `runNativeAgentTurnBody()` 中使用 errgroup 并行执行 Sessions.Get 和 hydratedAgent。

- [ ] **Step 2: BuildTRPCDeps 内部 DB 查询并行化**

在 `BuildTRPCDeps()` 中使用 errgroup 并行执行 GetEffectiveTools、computeSkillHash、computeMCPHash。

- [ ] **Step 3: 验证编译通过**

Run: `go build ./internal/service/...`

---

## Task 8: 前端 streaming 占位提前创建

**Files:**
- Modify: `web/src/features/chat/streamHandlers.ts`

**影响范围分析：**
- 当前 streaming 消息只在 text_delta 到达时创建
- 改为在 run_status=running 时创建占位消息，显示"正在准备"状态
- 风险：如果后续没有 text_delta（例如 LLM 调用失败），需要清理占位消息

- [ ] **Step 1: 在 run_status=running 时创建 streaming 占位消息**

修改 `streamHandlers.ts` 中 `run_status` handler，当 status=running 时创建 `ws-stream-{sessionId}` 占位消息。

- [ ] **Step 2: 验证前端编译通过**

Run: `cd web && pnpm build`

---

## Task 9: 全量验证

- [ ] **Step 1: 后端全量构建**

Run: `make wire && make build`

- [ ] **Step 2: 后端测试**

Run: `make test`

- [ ] **Step 3: 前端全量验证**

Run: `cd web && pnpm lint && pnpm build`

- [ ] **Step 4: aranea-review 审查**

使用 aranea-review SKILL 对所有变更进行代码审查。
