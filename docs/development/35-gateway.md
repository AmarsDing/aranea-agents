# M16: Gateway 网关 — 需求规格

> 对标 `pkg/trpc-agent-go/runner` 包的网关能力，完善项目的会话管理和 API 网关。
>
> 架构与接口设计详见 [35-gateway.design.md](./35-gateway.design.md)；
> 开发进度与任务清单详见 [35-gateway.development.md](./35-gateway.development.md)。

---

## 1. 模块目标

将会话运行编排（并发闸门、消息排队、状态查询、Steerable 入队）从传输层提取为可复用组件，供 Chat / Team / Cron / Channel / WS 共用；新增出站 Webhook 回调系统；启用 trpc 框架原生 `AwaitUserReplyRouting` 和 `SteerableRunner`。

**与 Runner 模块的边界**：
- **Runner（40）**：Agent 运行器构建和生命周期（AgentFactory、PluginManager、ManagedRunner 封装）
- **Gateway（本文）**：运行编排层（并发闸门、消息排队、状态查询、Webhook 出站），是 Runner 的上层编排

---

## 2. 需求清单

### 2.1 对话阶段连续发送 — Follow-up Queue（P1 UX / P2 实现）

**用户故事**：作为用户，在 Agent 生成或工具执行过程中，我可以像 Cursor 聊天窗口一样连续发送多条后续消息；消息进入对话队列，当前 turn 不被打断，队列内容可见、可编辑、可取消。

**功能规格**：
- 运行中 `SendChatMessage` / WS `user_message` 自动入队（非拒绝）
- SteerableRunner 直注优先；降级 Pending FIFO（32 条/session）
- 入队成功 WS 推送 `run_status` + `hint: message_queued`
- Pending CRUD API；turn 结束后自动 `processPendingQueue`
- 拒绝码：`CHAT_RUN_ENDED`（409）、`CHAT_QUEUE_FULL`（400）

**验收标准**：
- 后端双路径入队 + Pending CRUD + 自动出队
- `ChatUsecase` 编排 + `PublishMessageQueued`
- 前端运行中可连续发送
- 前端监听 `message_queued` 刷新队列

> 产品规格详见 [1 chat.md §1.9](./1%20chat.md#19-对话阶段连续发送follow-up-queue--待发送队列)
> 设计详见 [35-gateway.design.md §3.9](./35-gateway.design.md#39-follow-up-queue对话阶段连续发送)

### 2.2 PendingMessageQueue 下沉（P2）

**用户故事**：作为开发者，我希望消息排队逻辑下沉到 Runtime 层，使 Service 层保持薄传输桥职责。

**功能规格**：
- `PendingMessageQueue` 位于 Runtime 层
- Service 层仅通过 Usecase / 接口调用
- 现有排队 CRUD API 行为不变

**验收标准**：
- `PendingMessageQueue` 位于 `internal/runtime`
- ChatService 仅通过 `ChatUsecase` / 接口调用
- 现有排队 CRUD API 行为不变

### 2.3 出站 Webhook 回调（P2）

**用户故事**：作为系统管理员，我希望 Agent 运行完成后自动回调外部系统。

**功能规格**：
- 支持配置 Webhook URL 和事件类型（run.completed / run.failed / run.cancelled / graph.task.status）
- HMAC-SHA256 签名、自定义 Headers、启用/禁用
- 异步分发，不阻塞主流程，含重试

**验收标准**：
- 可通过 API 创建/更新/删除/列出 Webhook 配置
- 运行完成（成功/失败/取消）后自动触发回调
- 回调请求包含 HMAC-SHA256 签名
- 可按 event_types 过滤订阅

### 2.4 API 版本管理策略（P3）

**用户故事**：作为 API 消费者，我希望了解 API 版本演进规则，以便规划集成升级。

**功能规格**：
- 文档化 API 版本演进策略
- 当前所有 API 路径包含 `/v1/` 前缀

**验收标准**：
- API 版本管理策略已文档化
- 当前所有 API 路径包含 `/v1/` 前缀

### 2.5 API 文档自动生成（P3）

**用户故事**：作为 API 消费者，我希望通过 Swagger UI 浏览和试用 API。

**功能规格**：
- Swagger UI 可访问
- 文档与 proto 定义保持同步

**验收标准**：
- Swagger UI 可访问
- 文档与 proto 定义保持同步

---

## 3. 非功能需求

| 维度 | 要求 |
|------|------|
| 并发安全 | 会话级互斥，防止同一 session 并发 turn |
| 可靠性 | Webhook 异步分发 + 3 次重试，不阻塞主流程；终态事件持久化 |
| 安全 | JWT 认证 + Workspace 隔离 + Webhook 路径安全（EP-SEC-03）；Webhook secret 脱敏返回 |
| 可观测性 | 运行状态查询 API；WS 实时推送 run_status |
| 兼容性 | Biz / Runtime 重构对 Proto 层透明，对外 API 路径不变 |

---

## 4. 验收标准总览

1. Chat / Team / Cron / Channel 共用 `RunRegistry` + `RunGateway`
2. SteerableRunner 入队 + PendingMessageQueue 降级
3. `ChatUsecase` 编排运行状态/排队/入队
4. 可通过 API 管理 Webhook 配置，运行完成后自动回调
5. API 版本管理策略已文档化
6. Swagger UI 可访问

> 各项验收标准的实现状态详见 [35-gateway.development.md §2 现状评估](./35-gateway.development.md#2-现状评估)
