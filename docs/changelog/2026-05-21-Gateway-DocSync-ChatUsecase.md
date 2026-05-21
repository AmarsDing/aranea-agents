# Gateway — 文档同步 + ChatUsecase 接入（2026-05-21）

## 摘要

Gateway 模块文档与代码对齐；`ChatUsecase` 正式接入 `ChatService`，消除 Service 层重复的入队/排队/锁/await channel 编排逻辑。

## 代码变更

| 文件 | 变更 |
|------|------|
| `internal/service/chat.go` | 使用 `chatUC *biz.ChatUsecase` 替代直接持有 pending/sessionLocks/awaitChans |
| `internal/service/chat_run_gateway.go` | 新增 `NewChatUsecaseFromDeps` |
| `internal/service/chat_native.go` | 活跃运行入队委托 `chatUC.EnqueueUserMessage` |
| `internal/service/trpc_turn.go` | `processPendingQueue` 委托 `chatUC.DequeuePendingMessage` |
| `internal/biz/chat_usecase.go` | Steerable 入队成功时也 `PublishMessageQueued` |
| `internal/service/knowledge.go` | 移除未使用 import（构建修复） |

## 架构现状（文档已更新）

- **Runtime 层**：`RunRegistry` + `RunGateway` + `RunnerManager`（`internal/runtime/`）
- **Biz 层**：`ChatUsecase` 编排状态/排队/入队/await；`WebhookUsecase` + `WebhookDispatcher`
- **Service 层**：`PendingMessageQueue` 实现仍在 service（待 Phase 2 下沉）；终态 Webhook 触发留 Service
- **待做**：PendingQueue 下沉 runtime、API 版本策略、Swagger、Webhook 管理 UI

## 文档同步

- `docs/需求/35 gateway.md`
- `docs/需求/35 gateway.design.md`
- `docs/需求/35-gateway-development.md`
- `docs/需求/0-system-development.md`（SYS-01）
- `docs/需求/0 系统框图.md`（AH-02/AH-07/AH-09）
- `docs/guides/execution-plan.md`（M1 Webhook）

## 验证

```bash
go test ./internal/service/ -run "Pending|StopGeneration" -count=1
go test ./internal/biz/... ./internal/runtime/... -count=1
```
