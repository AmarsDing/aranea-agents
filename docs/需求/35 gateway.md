# M16: Gateway 网关 — 详细需求

> 对标 `pkg/trpc-agent-go/runner` 包的网关能力，完善项目的会话管理和 API 网关。

---

## 1. 现状分析

项目已有基础的 HTTP/gRPC 服务和 SSE 推流：
- `internal/server/sse.go`：SSE 推流
- `internal/service/chat_native.go`：聊天服务
- `internal/server/http.go`：HTTP 服务器
- `internal/server/grpc.go`：gRPC 服务器

**缺失能力**：
1. 无会话并发控制（同一会话同时多个请求）
2. 无运行状态查询（status）
3. 无运行取消（cancel）
4. 无 AwaitUserReply 路由
5. 无 QueuedUserMessage 排队
6. 无 Webhook 回调

---

## 2. trpc 框架参照

```
pkg/trpc-agent-go/runner/
├── runner.go              # Runner 接口：Run/Status/Cancel
├── await_user_reply.go    # AwaitUserReply 路由
├── ralph_loop.go          # RALPH 循环（排队消息）
└── agent_lookup.go        # Agent 查找
```

### Runner 接口

```go
type Runner interface {
    Run(ctx context.Context, userID, sessionID string, message model.Message, opts ...agent.RunOption) (<-chan *event.Event, error)
    Status(ctx context.Context, requestID string) (RunStatus, error)
    Cancel(ctx context.Context, requestID string) error
}
```

### AwaitUserReply

当 Agent 调用 `await_user_reply` 工具时，Runner 记录路由信息到 Session State。
下一次用户消息到来时，Runner 自动路由到指定 Agent。

### QueuedUserMessage

在 Agent 执行过程中，用户可以发送排队消息。
Agent 当前轮次完成后，Runner 自动将排队消息注入下一轮次。

---

## 3. 需求清单

### 3.1 会话并发控制

**需求**：同一会话同时只允许一个运行

**实现要点**：
- Runner 维护 `map[sessionID]requestID` 活跃运行表
- 新请求到来时检查是否有活跃运行
- 拒绝或排队新请求

**验收标准**：同一会话同时只有一个运行，其他请求被拒绝或排队

### 3.2 运行状态查询

**需求**：查询当前运行的进度

**实现要点**：
- 实现 `Status(ctx, requestID)` 方法
- 返回运行状态：running/completed/failed/cancelled
- 返回已生成的事件数量

**验收标准**：可通过 API 查询运行进度

### 3.3 运行取消

**需求**：取消正在运行的请求

**实现要点**：
- 实现 `Cancel(ctx, requestID)` 方法
- 取消运行中的 context
- 清理资源

**验收标准**：可通过 API 取消正在运行的请求

### 3.4 AwaitUserReply 路由

**需求**：Agent 可指定下一轮用户消息路由

**实现要点**：
- 集成 trpc `runner.WithAwaitUserReplyRouting(true)`
- Agent 调用 `await_user_reply` 工具时记录路由
- 下一轮用户消息自动路由到指定 Agent

**验收标准**：Agent 可指定下一轮用户消息路由到特定 Agent

### 3.5 QueuedUserMessage 排队

**需求**：Agent 执行过程中用户可发送排队消息

**实现要点**：
- 集成 trpc `runner/ralph_loop.go`
- 用户消息排队等待
- Agent 当前轮次完成后自动处理排队消息

**验收标准**：Agent 执行中用户消息被排队，轮次结束后自动处理

### 3.6 Webhook 回调

**需求**：Agent 运行完成后回调外部系统

**实现要点**：
- 新建 `internal/gateway/webhook.go`
- 支持配置 Webhook URL
- 运行完成后发送回调（成功/失败/取消）
- 支持自定义回调 payload

**验收标准**：Agent 运行完成后自动回调外部系统

### 3.7 API 端点

**需求**：完善 Gateway API

**实现要点**：
- `GET /gateway/runs/:id/status` — 查询运行状态
- `POST /gateway/runs/:id/cancel` — 取消运行
- `POST /gateway/runs/:id/queue` — 排队消息
- `POST /gateway/webhooks` — 配置 Webhook

**验收标准**：通过 API 可管理运行生命周期

---

## 4. 涉及文件

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/agent/trpc_runtime.go` | 修改 | 增加 Status/Cancel/AwaitUserReply |
| `internal/gateway/concurrency.go` | 新建 | 会话并发控制 |
| `internal/gateway/webhook.go` | 新建 | Webhook 回调 |
| `internal/service/gateway.go` | 新建 | Gateway 服务层 |
| `internal/server/register_gateway.go` | 新建 | Gateway HTTP 端点 |

---

## 5. 验收标准总览

1. 同一会话同时只有一个运行
2. 可通过 API 查询运行进度
3. 可通过 API 取消正在运行的请求
4. Agent 可指定下一轮用户消息路由
5. 用户消息可排队等待
6. Agent 运行完成后自动回调外部系统
