# M16: Gateway 网关 — 需求规格

> 对标 `pkg/trpc-agent-go/runner` 包的网关能力，完善项目的会话管理和 API 网关。

> **2026-05-21 现状对齐**：
> - ✅ **RunRegistry / RunGateway**：`internal/runtime/run_registry.go` + `RunGateway` 接口；Chat / Team / Cron / Channel / WS 共用。
> - ✅ **RunnerManager**：`internal/runtime/runner_manager.go` 统一 trpc Runner 构建。
> - ✅ **并发控制**：会话级互斥，`HasActive` + placeholder 清理。
> - ✅ **消息排队**：SteerableRunner `EnqueueUserMessage` 优先，不支持时降级 `PendingMessageQueue`（32 条/会话上限 + 可选磁盘快照）。
> - ✅ **运行取消**：`StopGeneration` API + `RunRegistry.Cancel`（ManagedRunner / context cancel）。
> - ✅ **RunStatus 查询**：`GetRunStatus` API，7 种状态 + 框架字段合并；前端 `useRunStatus` 轮询。
> - ✅ **AwaitUserReply**：`AwaitUserReply` API + `makeAwaitReplyFunc`；`AwaitUserReplyRouting` 在注入 AwaitHook 时由 `RunnerManager` 启用。
> - ✅ **WebSocket 网关**：`internal/server/ws.go` 挂入 Kratos HTTP。
> - ✅ **认证中间件**：JWT（`pkg/auth/middleware.go`）+ Workspace 过滤（`internal/server/middleware/workspace.go`）。
> - ✅ **Biz 编排层**：`ChatUsecase` 已接入；`PendingMessageQueue` 位于 `internal/runtime`。
> - ✅ **出站 Webhook**：`GatewayService` CRUD + `WebhookDispatcher` 终态回调（HMAC-SHA256）。
> - ❌ **API 版本管理策略**：`/v1/` 前缀已存在，无版本演进策略文档。
> - ❌ **API 文档自动生成**：无 Swagger/OpenAPI。
> - 进度真相以 `35-gateway-development.md` 与 `guides/execution-plan.md` 附录 A 为准。

---

## 1. 现状分析

### 1.1 已有能力

| 能力 | 实现位置 | 说明 |
|------|----------|------|
| 运行注册表 | `internal/runtime/run_registry.go` | 活跃运行、取消、状态、Steerable 入队 |
| RunGateway 接口 | `internal/runtime/gateway.go` | Chat / Team / Cron / Channel / WS 共用 |
| Runner 构建 | `internal/runtime/runner_manager.go` | 统一 TurnRunner 装配 + AwaitUserReplyRouting |
| Biz 编排 | `internal/biz/chat_usecase.go` | 入队/排队/状态/锁/await channel 编排 |
| 消息排队 | `internal/runtime/pending_queue.go` | Follow-up Queue：Steerable 优先 + Pending FIFO（32 条/会话） |
| 运行取消 | `StopGeneration` API | ManagedRunner.Cancel + context cancel |
| 运行状态查询 | `GetRunStatus` API | 7 种状态 + trpc RunStatus 字段合并 |
| 用户回复路由 | `AwaitUserReply` API | ServiceTool 暂停 + 跨重启 resume |
| 出站 Webhook | `GatewayService` + `WebhookDispatcher` | CRUD + 终态回调（HMAC-SHA256） |
| WebSocket 网关 | `internal/server/ws.go` | Chat / Monitor / Team 多通道 |
| 认证 | `pkg/auth/middleware.go` | JWT + Webhook 路径白名单 |
| Workspace 隔离 | `internal/server/middleware/workspace.go` | X-Workspace-ID header/query |

### 1.2 仍缺失或半成品

1. **API 版本管理策略缺失**：当前 `/v1/` 前缀已存在但无版本演进策略文档。
2. **API 文档自动生成缺失**：无 Swagger/OpenAPI 托管 UI（`make api` 已生成 openapi.yaml）。
3. **Gateway Webhook 管理 UI**：后端 CRUD 已通，**API-only**（见 `frontend-pages.md`）。

---

## 2. trpc 框架参照

```
pkg/trpc-agent-go/runner/
├── runner.go              # Runner/ManagedRunner/SteerableRunner 接口
├── await_user_reply.go    # AwaitUserReply 路由（会话状态驱动）
├── ralph_loop.go          # RalphLoop 迭代执行
└── agent_lookup.go        # Agent 查找
```

### Runner 接口层级

| 接口 | 方法 | 说明 |
|------|------|------|
| `Runner` | `Run` + `Close` | 基础运行器 |
| `ManagedRunner` | + `Cancel` + `RunStatus` | 可管理的运行器 |
| `SteerableRunner` | + `EnqueueUserMessage` | 可转向的运行器（排队消息注入） |

### AwaitUserReply

当 Agent 调用 `await_user_reply` 工具时，Runner 记录路由信息到 Session State。  
`RunnerManager.NewTurnRunner` 在 `AwaitHook != nil` 时传入 `AwaitUserReplyRouting: true`。

### QueuedUserMessage / SteerableRunner

`RunRegistry.EnqueueUserMessage` 调用 `trpcrunner.EnqueueUserMessage`；不支持时降级到 `PendingMessageQueue`。

---

## 3. 需求清单

### 3.0 对话阶段连续发送 — Follow-up Queue（P1 UX / P2 实现）

**用户故事**：作为用户，在 Agent 生成或工具执行过程中，我可以像 Cursor 聊天窗口一样连续发送多条后续消息；消息进入对话队列，当前 turn 不被打断，队列内容可见、可编辑、可取消。

**功能规格**：
- 运行中 `SendChatMessage` / WS `user_message` 自动入队（非拒绝）
- SteerableRunner 直注优先；降级 Pending FIFO（32 条/session）
- 入队成功 WS 推送 `run_status` + `hint: message_queued`
- Pending CRUD API；turn 结束后自动 `processPendingQueue`
- 拒绝码：`CHAT_RUN_ENDED`（409）、`CHAT_QUEUE_FULL`（400）

**验收标准**：
- [x] 后端双路径入队 + Pending CRUD + 自动出队
- [x] `ChatUsecase` 编排 + `PublishMessageQueued`
- [x] 前端运行中可连续发送
- [x] 前端监听 `message_queued` 刷新队列

详见 [1 chat.md §1.9](./1%20chat.md#19-对话阶段连续发送follow-up-queue--待发送队列) · [35 gateway.design.md §3.6](./35%20gateway.design.md#36-follow-up-queue对话阶段连续发送)

### 3.1 PendingMessageQueue 下沉（P2） ✅

**验收标准**：
- [x] `PendingMessageQueue` 位于 `internal/runtime`
- [x] ChatService 仅通过 `ChatUsecase` / 接口调用
- [x] 现有排队 CRUD API 行为不变

### 3.2 出站 Webhook 回调（P2）

**用户故事**：作为系统管理员，我希望 Agent 运行完成后自动回调外部系统。

**功能规格**：
- 支持配置 Webhook URL 和事件类型（run.completed / run.failed / run.cancelled）
- HMAC-SHA256 签名、自定义 Headers、启用/禁用

**验收标准**：
- [x] 可通过 API 创建/更新/删除/列出 Webhook 配置
- [x] 运行完成（成功/失败/取消）后自动触发回调
- [x] 回调请求包含 HMAC-SHA256 签名

### 3.3 API 版本管理策略（P3）

**验收标准**：
- [ ] API 版本管理策略已文档化
- [ ] 当前所有 API 路径包含 `/v1/` 前缀

### 3.4 API 文档自动生成（P3）

**验收标准**：
- [ ] Swagger UI 可访问
- [ ] 文档与 proto 定义保持同步

---

## 4. 验收标准总览

1. Chat / Team / Cron / Channel 共用 `RunRegistry` + `RunGateway` ✅
2. SteerableRunner 入队 + PendingMessageQueue 降级 ✅
3. `ChatUsecase` 编排运行状态/排队/入队 ✅
4. 可通过 API 管理 Webhook 配置，运行完成后自动回调 ✅
5. API 版本管理策略已文档化 ❌
6. Swagger UI 可访问 ❌
