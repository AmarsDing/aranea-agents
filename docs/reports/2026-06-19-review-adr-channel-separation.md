# ADR-01: 通道职责分离 — 新增 SubmitChatMessage 异步 RPC

## 状态：已接受

## 背景

### 问题

当前 `SendChatMessage` RPC（POST `/v1/chat/messages`）是**同步阻塞**语义：调用方阻塞直到整个 turn 执行完成，响应中返回完整的 `user_message` 和 `agent_message`。

这与 B2 通道职责分离架构（详见 `2026-06-18-review-issues-and-solutions.md` 方案 B）产生矛盾：

1. **前端已实现通道分离**：`web/src/realtime/command_channel.ts` 将 HTTP 定位为"命令通道"（fire-and-ack），`data_channel.ts` 将 WS 定位为"唯一数据通道"。前端 `sendMessage`（`api.ts:61-99`）已仅提取 ACK 字段（messageId/turnId/status），忽略响应体中的完整消息数据。

2. **后端仍为同步语义**：`nativeSendChatMessage`（`chat_orchestrator_turn_api.go:17-51`）调用 `o.Execute(ctx, ...)` 阻塞等待 turn 完成。HTTP 请求挂起直到 LLM 响应、工具调用、持久化全部完成，可能长达数分钟（长任务场景下更久）。

3. **次生问题**：
   - HTTP 连接长时间占用服务器资源（goroutine + 连接句柄）
   - 反向代理/负载均衡可能因超时切断空闲连接
   - 与"无超时长任务"设计原则冲突（任务持续运行，HTTP 不应阻塞）
   - WS 断连重连后，HTTP 响应可能与 WS 事件流产生竞态

### 约束

- **不可破坏现有契约**：`SendChatMessage` 被多个入口调用（WS fallback 路径、A2A、Channel），修改其语义为异步会破坏所有调用方
- **Proto 破坏性变更风险**：直接修改 `SendChatMessageResponse` 删除 `user_message`/`agent_message` 字段是 wire-format 破坏性变更
- **前端已就绪**：前端已按命令/数据分离模式实现，仅需后端提供异步 RPC

## 决策

**新增 `SubmitChatMessage` RPC（additive，非破坏性变更）**：

1. **Proto 定义**：在 `chat.proto` 的 `service ChatService` 块内新增 `SubmitChatMessage` RPC，复用 `SendChatMessageRequest` 作为请求，新增 `SubmitChatMessageResponse`（仅含 ACK 字段）作为响应

2. **HTTP 路由**：`POST /v1/chat/messages/submit`（与现有 `POST /v1/chat/messages` 共存）

3. **服务实现**：
   - 在 goroutine 中启动 turn（使用 `appctx.Ctx()` 进程级上下文，不随 HTTP 请求结束而取消）
   - 立即返回 ACK：`{message_id: "", turn_id: "", status: "accepted"}`
   - turn 执行结果通过 WS 数据通道推送（`run_status`/`activity_*`/`runner_completion` 等事件）
   - 失败时通过 WS 推送 `error` envelope

4. **前端迁移**：`api.ts` 的 `sendMessage` 函数从调用 `SendChatMessage` 改为调用 `SubmitChatMessage`

5. **保留 `SendChatMessage`**：不修改、不删除，作为兼容入口（WS fallback 路径、A2A、Channel 等仍可使用）

## 后果

### 正面影响

- **HTTP 不再阻塞**：`SubmitChatMessage` 立即返回，HTTP 连接占用时间 < 100ms
- **通道分离完整闭环**：HTTP = 命令（fire-and-ack），WS = 数据（唯一数据源），消除双通道竞态
- **长任务兼容**：HTTP 请求不再因长任务挂起，与"无超时长任务"设计原则一致
- **非破坏性**：现有 `SendChatMessage` 保持不变，所有现有调用方不受影响
- **资源效率**：服务器不再为每个 HTTP 消息请求保持长连接 goroutine

### 负面影响

- **Proto 表面积增加**：新增 1 个 RPC + 1 个 Response 消息类型
- **两套入口并存**：`SendChatMessage`（同步）和 `SubmitChatMessage`（异步）并存，需文档说明使用场景
- **messageId 延迟分配**：ACK 中 `message_id` 为空（消息在 turn 启动时才持久化），前端需通过 WS `message.persisted` 事件获取真实 ID
- **错误反馈异步化**：turn 启动失败（如 admission gate 拒绝）通过 WS error envelope 推送，而非 HTTP 错误响应

## 替代方案

### 方案 A：修改 `SendChatMessage` 为异步（破坏性变更）

**否决原因**：
- Wire-format 破坏性变更：删除 `user_message`/`agent_message` 字段破坏所有客户端
- WS fallback 路径（`ws_message_handler.go:151-184`）依赖 `SendChatMessage` 的同步语义
- A2A、Channel 等入口可能依赖同步响应
- 需要灰度迁移，复杂度高

### 方案 B：HTTP SSE 流式响应

**否决原因**：
- 与通道分离架构矛盾（HTTP 不应承担数据传输职责）
- SSE 在反向代理环境下不可靠
- 前端已实现 WS 数据通道，无需 SSE

### 方案 C：HTTP 204 No Content + WS 推送

**否决原因**：
- 204 不含 body，无法返回 ACK 字段（messageId/turnId/status）
- 前端需要 ACK 字段进行状态管理（如 pending-user 占位消息匹配）
- 与 `EnqueueUserMessage` 的 ACK 模式不一致

## 关联

- **报告**：`docs/reports/2026-06-18-review-issues-and-solutions.md` 方案 B（B2 通道职责分离）
- **完整消息链报告**：`docs/reports/2026-06-18-review-full-message-chain-and-solutions.md` T2.4
- **前端实现**：`web/src/realtime/command_channel.ts`、`web/src/realtime/data_channel.ts`
- **后端实现**：`api/kratos/chat/v1/chat.proto`、`internal/service/chat_orchestrator_turn_api.go`
