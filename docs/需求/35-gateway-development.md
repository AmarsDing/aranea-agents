# Gateway 网关 — 开发计划

> **版本**：2026-05-19 | **状态**：🟡 核心功能已实现，待提取重构 + Webhook 新增
> **需求**：[35 gateway.md](./35%20gateway.md) · **设计**：[35 gateway.design.md](./35%20gateway.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-RT-01/02 ✅

---

## 1. 模块定位

Gateway 网关：运行编排层，负责会话并发控制、消息排队、运行状态管理、用户回复路由和出站 Webhook 回调。将散落在 `ChatService` 的运行管理逻辑提取为独立 Biz 层，并启用 trpc 框架原生能力。

**代码锚点**：
- `internal/service/chat.go` — 当前运行管理逻辑所在（activeRuns/pendingQueue/runStatuses/awaitChans）
- `internal/service/chat_native.go` — 并发控制 + 排队消息处理
- `internal/service/trpc_turn.go` — 单 Agent 运行 + processPendingQueue
- `internal/agent/trpc_runtime.go` — Runner 构建（待启用 AwaitUserReplyRouting + SteerableRunner）
- `internal/server/ws.go` — WebSocket 网关
- `internal/server/http.go` — HTTP 路由注册
- `pkg/auth/middleware.go` — JWT 认证中间件
- `internal/server/middleware/workspace.go` — Workspace 过滤器

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| 会话并发控制 | ✅ | `ChatService.activeRuns`（sync.Map），运行中请求被排队 |
| 消息排队 | ✅ | `ChatService.pendingQueue`（sync.Map），enqueue/dequeue/processPendingQueue 端到端 |
| 排队消息 CRUD | ✅ | Chat Proto: GetPendingMessages/CancelPendingMessage/UpdatePendingMessage |
| 运行取消 | ✅ | `StopGeneration` API + `CancelRun` 内部方法 |
| 运行状态查询 | ✅ | `GetRunStatus` API，7 种状态 |
| 用户回复路由 | ✅ | `AwaitUserReply` API + `makeAwaitReplyFunc` + `awaitChans` |
| 前端运行状态 | ✅ | `useRunStatus` composable + `submitReply` |
| WebSocket 网关 | ✅ | `ws.go` 挂入 Kratos HTTP |
| 认证中间件 | ✅ | JWT + Workspace 过滤 |
| Biz 层提取 | ❌ | 并发/排队/状态逻辑散落在 ChatService，4 个 sync.Map |
| trpc AwaitUserReplyRouting | ❌ | `NewTRPCRunner` 未传 `WithAwaitUserReplyRouting(true)` |
| trpc SteerableRunner | 🟡 | `EnqueueTRPCUserMessage` 已封装但未端到端联调 |
| 出站 Webhook | ❌ | 运行完成回调第三方系统未实现 |
| API 版本管理策略 | 🟡 | `/v1/` 前缀已存在，但无版本演进策略文档 |
| API 文档自动生成 | ❌ | 无 Swagger/OpenAPI |

---

## 3. 差距与优先级

| # | 差距 | 优先级 | 对应需求 | 说明 |
|---|------|--------|----------|------|
| 1 | Biz 层提取 | P1 | 3.1 | 并发/排队/状态逻辑从 ChatService 提取为独立 Biz 层 |
| 2 | trpc AwaitUserReplyRouting | P2 | 3.2 | 启用框架原生路由，与 makeAwaitReplyFunc 协同 |
| 3 | trpc SteerableRunner 联调 | P2 | 3.3 | 用 EnqueueUserMessage 替代手动 pendingQueue |
| 4 | 出站 Webhook | P2 | 3.4 | 运行完成回调第三方系统 |
| 5 | API 版本管理策略 | P3 | 3.5 | 文档化版本演进规则 |
| 6 | API 文档自动生成 | P3 | 3.6 | protoc-gen-openapi 生成 Swagger |

---

## 4. 开发阶段

### Phase 1：Biz 层提取（P1）

将 `ChatService` 的运行管理逻辑提取为独立 Biz 层组件，保持现有 API 行为不变。

**任务清单**：

| # | 任务 | 涉及文件 | 验证 |
|---|------|----------|------|
| 1.1 | 新建 `RunRegistry` | `internal/biz/run_registry.go` | 单元测试：Register/Unregister/IsRunning/CancelRun/GetStatus |
| 1.2 | 新建 `MessageQueue` | `internal/biz/message_queue.go` | 单元测试：Enqueue/Dequeue/List/Cancel/Update，上限 32 |
| 1.3 | 新建 `GatewayUsecase` | `internal/biz/gateway.go` | 单元测试：编排 RunRegistry + MessageQueue |
| 1.4 | ChatService 使用 GatewayUsecase | `internal/service/chat.go`, `chat_native.go`, `trpc_turn.go` | 现有 Chat API 行为不变 |
| 1.5 | 更新 ProviderSet + Wire | `internal/biz/biz.go`, `cmd/admin/wire.go` | `make wire && make build` |

**验收标准**：
- [ ] ChatService 不再直接持有 `activeRuns`/`pendingQueue`/`runStatuses` 三个 sync.Map
- [ ] `awaitChans` 可暂时保留在 ChatService（与 makeAwaitReplyFunc 紧耦合）
- [ ] 现有 Chat API 行为不变（并发控制、排队、取消、状态查询）
- [ ] `make wire && make build && make test` 通过

### Phase 2：trpc 框架原生能力启用（P2）

启用 `WithAwaitUserReplyRouting` 和 `SteerableRunner`，与现有逻辑协同。

**任务清单**：

| # | 任务 | 涉及文件 | 验证 |
|---|------|----------|------|
| 2.1 | 启用 WithAwaitUserReplyRouting | `internal/agent/trpc_runtime.go` | Agent 可通过 await_user_reply 指定下一轮路由 |
| 2.2 | SteerableRunner 联调 | `internal/service/chat_native.go`, `trpc_turn.go` | 运行中消息通过 EnqueueUserMessage 注入 |
| 2.3 | 降级策略实现 | `internal/service/chat_native.go` | Runner 不实现 SteerableRunner 时回退到 MessageQueue |

**验收标准**：
- [ ] `NewTRPCRunner` 传入 `WithAwaitUserReplyRouting(true)`
- [ ] 运行中用户消息优先通过 `EnqueueUserMessage` 注入
- [ ] 降级到 `MessageQueue.Enqueue` 正常工作
- [ ] 现有 `makeAwaitReplyFunc` + `AwaitUserReply` API 行为不变

### Phase 3：出站 Webhook 系统（P2）

新增 Webhook 配置持久化和运行完成回调分发。

**任务清单**：

| # | 任务 | 涉及文件 | 验证 |
|---|------|----------|------|
| 3.1 | 新建 GatewayWebhook Ent Schema | `internal/data/ent/schema/gateway_webhook.go` | `go generate ./internal/data/ent` |
| 3.2 | 新建 WebhookRepository + WebhookUsecase | `internal/biz/webhook.go` | 单元测试：CRUD |
| 3.3 | 新建 WebhookDispatcher | `internal/biz/webhook_dispatcher.go` | 单元测试：Dispatch + HMAC 签名 |
| 3.4 | 新建 WebhookRepo 实现 | `internal/data/webhook.go` | 集成测试 |
| 3.5 | 新建 Gateway Proto（Webhook CRUD） | `api/kratos/gateway/v1/gateway.proto` | `make api` |
| 3.6 | 新建 GatewayService | `internal/service/gateway.go` | Webhook CRUD API 可用 |
| 3.7 | 注册 GatewayService + Wire | `internal/server/http.go`, `cmd/admin/wire.go` | `make wire && make build` |
| 3.8 | 集成 Webhook 触发 | `internal/service/chat.go`, `chat_native.go`, `trpc_turn.go` | 运行完成/失败/取消触发回调 |

**验收标准**：
- [ ] 可通过 API 创建/更新/删除/列出 Webhook 配置
- [ ] 运行完成（成功/失败/取消）后自动触发回调
- [ ] 回调请求包含 `X-Webhook-Signature` HMAC-SHA256 签名
- [ ] 可按事件类型过滤回调
- [ ] `make wire && make build && make test` 通过

### Phase 4：API 版本管理 + 文档（P3）

**任务清单**：

| # | 任务 | 涉及文件 | 验证 |
|---|------|----------|------|
| 4.1 | 文档化 API 版本演进策略 | `docs/guides/` | 策略文档可查阅 |
| 4.2 | protoc-gen-openapi 生成 Swagger | `Makefile`, `api/` | Swagger UI 可访问 |

**验收标准**：
- [ ] API 版本管理策略已文档化
- [ ] 当前所有 API 路径包含 `/v1/` 前缀
- [ ] Swagger UI 可访问

---

## 5. 依赖与风险

| 风险 | 影响 | 缓解 |
|------|------|------|
| Biz 层提取可能引入回归 | Chat API 行为变化 | Phase 1 完成后全量回归测试 |
| WithAwaitUserReplyRouting 与 makeAwaitReplyFunc 冲突 | Agent 暂停/恢复行为异常 | 两者处理不同层面（路由 vs 暂停），需集成测试验证 |
| SteerableRunner 接口不被所有 Runner 实现 | 排队消息注入失败 | 降级到 MessageQueue |
| Webhook 回调目标不可达 | 通知丢失 | 异步发送 + 日志记录，不阻塞主流程 |
| API 版本管理需与前端路由同步 | 前端调用失败 | 版本升级时前后端同步发布 |

---

## 6. 与 Runner 模块（40）的协调

| 任务 | Gateway 负责 | Runner 负责 |
|------|-------------|-------------|
| Runner 构建 | — | AgentFactory、PluginManager、ManagedRunner 封装 |
| WithAwaitUserReplyRouting | 在 `trpc_runtime.go` 启用选项 | 框架提供路由能力 |
| SteerableRunner 联调 | 在 ChatService 使用 EnqueueUserMessage | 框架提供接口 |
| 运行编排（并发/排队/状态） | RunRegistry + MessageQueue + GatewayUsecase | — |
| Webhook 出站 | WebhookDispatcher | — |
