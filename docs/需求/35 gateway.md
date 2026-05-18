# M16: Gateway 网关 — 需求规格

> 对标 `pkg/trpc-agent-go/runner` 包的网关能力，完善项目的会话管理和 API 网关。

> **2026-05-19 现状对齐**：
> - ✅ **并发控制**：`ChatService.activeRuns`（`sync.Map`）已实现会话级互斥，运行中请求被排队。
> - ✅ **消息排队**：`ChatService.pendingQueue`（`sync.Map`）+ `enqueuePending`/`dequeuePending`/`processPendingQueue` 已端到端可用；Chat Proto 已定义 `GetPendingMessages`/`CancelPendingMessage`/`UpdatePendingMessage`。
> - ✅ **运行取消**：`StopGeneration` API + `CancelRun` 内部方法已实现，支持 Team 和单 Agent 两种运行类型。
> - ✅ **RunStatus 查询**：`GetRunStatus` API 已落地，返回 `idle | pending | running | awaiting_user | completed | failed | cancelled`；前端 `useRunStatus` composable 已实现轮询。
> - ✅ **AwaitUserReply**：`AwaitUserReply` API + `makeAwaitReplyFunc` 已实现，Agent 可暂停等待用户回复；前端 `submitReply` 已可用。
> - ✅ **WebSocket 网关**：`internal/server/ws.go` 已挂入 Kratos HTTP，为事件与聊天主通道之一。
> - ✅ **认证中间件**：JWT 认证（`pkg/auth/middleware.go`）+ Workspace 过滤（`internal/server/middleware/workspace.go`）。
> - 🟡 **统一 Gateway 抽象**：并发控制/排队/状态管理逻辑散落在 `ChatService` 内（`activeRuns`/`pendingQueue`/`runStatuses`/`awaitChans` 四个 `sync.Map`），未提取为独立 Biz 层。
> - 🟡 **trpc AwaitUserReplyRouting**：`internal/agent/trpc_runtime.go` 的 `NewTRPCRunner` 未启用 `WithAwaitUserReplyRouting(true)`，当前通过 Service 层 `makeAwaitReplyFunc` 手动实现。
> - 🟡 **trpc SteerableRunner**：`EnqueueTRPCUserMessage` 已封装但未端到端联调，当前排队通过 `pendingQueue` 手动实现而非 trpc `SteerableRunner.EnqueueUserMessage`。
> - ❌ **出站 Webhook**：运行完成通知第三方系统未实现（Channel 入站 webhook 已有，但运行生命周期出站回调不存在）。
> - ❌ **API 版本管理**：当前 API 已使用 `/v1/` 前缀，但无版本管理策略文档。
> - ❌ **API 文档**：无 Swagger/OpenAPI 自动生成。
> - 进度真相以 `guides/execution-plan.md` 附录 A 为准。

---

## 1. 现状分析

### 1.1 已有能力

| 能力 | 实现位置 | 说明 |
|------|----------|------|
| 会话并发控制 | `ChatService.activeRuns` | `sync.Map` 跟踪活跃运行，运行中请求被排队 |
| 消息排队 | `ChatService.pendingQueue` | enqueue/dequeue/processPendingQueue 端到端，上限 32 条/会话 |
| 运行取消 | `StopGeneration` API | 支持 Team `teamRunGuard.cancel()` 和单 Agent `CancelTRPCRun` |
| 运行状态查询 | `GetRunStatus` API | 返回 7 种状态 + run_id + error_message + updated_at |
| 用户回复路由 | `AwaitUserReply` API | `makeAwaitReplyFunc` + `awaitChans`，Agent 暂停等待用户回复 |
| WebSocket 网关 | `internal/server/ws.go` | 挂入 Kratos HTTP，支持 Chat/Monitor/Team 多通道 |
| 认证 | `pkg/auth/middleware.go` | JWT 认证 + Webhook 路径白名单 |
| Workspace 隔离 | `internal/server/middleware/workspace.go` | X-Workspace-ID header/query 注入 |

### 1.2 仍缺失或半成品

1. **并发控制/排队逻辑未提取为 Biz 层**：`activeRuns`/`pendingQueue`/`runStatuses`/`awaitChans` 四个 `sync.Map` 散落在 `ChatService`，无法被其他服务复用。
2. **trpc AwaitUserReplyRouting 未启用**：`NewTRPCRunner` 未传 `WithAwaitUserReplyRouting(true)`，当前通过 Service 层手动实现而非框架原生路由。
3. **trpc SteerableRunner 未联调**：`EnqueueTRPCUserMessage` 已封装但未端到端使用，排队仍走手动 `pendingQueue`。
4. **出站 Webhook 未实现**：运行完成（成功/失败/取消）后无回调第三方系统的能力。
5. **API 版本管理策略缺失**：当前 `/v1/` 前缀已存在但无版本演进策略文档。
6. **API 文档自动生成缺失**：无 Swagger/OpenAPI，前端开发者需手动查阅 proto。

---

## 2. trpc 框架参照

```
pkg/trpc-agent-go/runner/
├── runner.go              # Runner/ManagedRunner/SteerableRunner 接口
├── await_user_reply.go    # AwaitUserReply 路由（会话状态驱动）
├── ralph_loop.go          # RalphLoop 迭代执行（带验证和完成承诺）
└── agent_lookup.go        # Agent 查找（注册表 + 工厂回退）
```

### Runner 接口层级

| 接口 | 方法 | 说明 |
|------|------|------|
| `Runner` | `Run` + `Close` | 基础运行器 |
| `ManagedRunner` | + `Cancel` + `RunStatus` | 可管理的运行器 |
| `SteerableRunner` | + `EnqueueUserMessage` | 可转向的运行器（排队消息注入） |

### AwaitUserReply

当 Agent 调用 `await_user_reply` 工具时，Runner 记录路由信息到 Session State。
下一次用户消息到来时，Runner 自动路由到指定 Agent。
需通过 `WithAwaitUserReplyRouting(true)` 启用。

### QueuedUserMessage / SteerableRunner

通过 `SteerableRunner.EnqueueUserMessage` 在运行中注入排队消息。
Agent 当前轮次完成后，Runner 自动将排队消息注入下一轮次。

---

## 3. 需求清单

### 3.1 统一 Gateway 抽象（P1）

**用户故事**：作为开发者，我希望并发控制、消息排队、运行状态管理逻辑从 ChatService 提取为独立的 Biz 层组件，以便其他服务（Channel、Cron、A2A）复用。

**功能规格**：
- 将 `activeRuns`/`pendingQueue`/`runStatuses`/`awaitChans` 提取为 Biz 层独立结构
- ChatService 通过 Biz 层接口调用，不再直接持有 `sync.Map`
- 其他服务可通过同一接口管理运行生命周期

**验收标准**：
- [ ] 并发控制/排队/状态管理逻辑位于 Biz 层独立文件
- [ ] ChatService 不再直接持有 `sync.Map`，通过 Biz 层接口调用
- [ ] 现有 Chat API 行为不变（并发控制、排队、取消、状态查询）

### 3.2 trpc AwaitUserReplyRouting 启用（P2）

**用户故事**：作为 Agent 开发者，我希望 Runner 启用 `WithAwaitUserReplyRouting(true)`，使 Agent 可通过框架原生机制指定下一轮用户消息路由。

**功能规格**：
- `NewTRPCRunner` 启用 `WithAwaitUserReplyRouting(true)`
- Agent 调用 `await_user_reply` 工具时，Runner 自动记录路由到 Session State
- 下一轮用户消息自动路由到指定 Agent

**验收标准**：
- [ ] `NewTRPCRunner` 传入 `WithAwaitUserReplyRouting(true)`
- [ ] Agent 可通过 `await_user_reply` 工具指定下一轮路由
- [ ] 现有 `makeAwaitReplyFunc` 行为兼容

### 3.3 trpc SteerableRunner 联调（P2）

**用户故事**：作为用户，我希望在 Agent 运行中发送的消息通过 trpc `SteerableRunner.EnqueueUserMessage` 注入，而非手动排队。

**功能规格**：
- 使用 `SteerableRunner.EnqueueUserMessage` 替代手动 `pendingQueue` 排队
- Agent 当前轮次完成后，Runner 自动处理排队消息
- 保留排队消息的查询/取消/编辑 API

**验收标准**：
- [ ] 运行中用户消息通过 `EnqueueUserMessage` 注入
- [ ] 排队消息查询/取消/编辑 API 行为不变
- [ ] `processPendingQueue` 逻辑与 SteerableRunner 协同工作

### 3.4 出站 Webhook 回调（P2）

**用户故事**：作为系统管理员，我希望 Agent 运行完成后自动回调外部系统，以便与 CI/CD、通知平台等第三方集成。

**功能规格**：
- 支持配置 Webhook URL 和事件类型（run.completed / run.failed / run.cancelled）
- 运行完成后自动发送回调
- 支持 HMAC-SHA256 签名验证
- 支持自定义 HTTP Headers
- 支持启用/禁用 Webhook

**验收标准**：
- [ ] 可通过 API 创建/更新/删除/列出 Webhook 配置
- [ ] 运行完成（成功/失败/取消）后自动触发回调
- [ ] 回调请求包含 HMAC-SHA256 签名
- [ ] 可按事件类型过滤回调

### 3.5 API 版本管理策略（P3）

**用户故事**：作为 API 消费者，我希望 API 有明确的版本管理策略，以便在版本升级时有清晰的迁移路径。

**功能规格**：
- 文档化当前 `/v1/` 版本策略
- 定义版本升级规则（何时引入 `/v2/`、废弃策略）
- 确保 proto 包名与版本前缀一致

**验收标准**：
- [ ] API 版本管理策略已文档化
- [ ] 当前所有 API 路径包含 `/v1/` 前缀

### 3.6 API 文档自动生成（P3）

**用户故事**：作为前端开发者，我希望有自动生成的 API 文档，以便快速查阅接口定义。

**功能规格**：
- 从 proto 文件自动生成 Swagger/OpenAPI 文档
- 提供 Swagger UI 供交互式查阅

**验收标准**：
- [ ] Swagger UI 可访问
- [ ] 文档与 proto 定义保持同步

---

## 4. 验收标准总览

1. 并发控制/排队/状态管理逻辑提取为 Biz 层独立组件，ChatService 不再直接持有 `sync.Map`
2. `NewTRPCRunner` 启用 `WithAwaitUserReplyRouting(true)`
3. 运行中用户消息通过 `SteerableRunner.EnqueueUserMessage` 注入
4. 可通过 API 管理 Webhook 配置，运行完成后自动回调
5. API 版本管理策略已文档化
6. Swagger UI 可访问
