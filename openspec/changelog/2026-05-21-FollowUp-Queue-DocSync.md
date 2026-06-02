# Follow-up Queue 文档同步（2026-05-21）

## 摘要

将「对话阶段连续发送 / 待发送队列」产品语义正式文档化，对标 Cursor 聊天窗口：Agent 运行中用户可连续发送多条消息，经 Steerable 直注或 Pending FIFO 处理。澄清 `publishMessageQueued` 已收敛至 `ChatUsecase`，`ChatService.publishMessageQueued` 标记废弃。

## 文档变更

| 文件 | 变更 |
|------|------|
| `docs/需求/1 chat.md` | 新增 §1.9 Follow-up Queue 产品规格、WS `message_queued`、前端差距表 |
| `docs/需求/1 chat.design.md` | ChatService 结构对齐 chatUC；Follow-up Queue 设计锚点 |
| `docs/需求/1-chat-development.md` | Phase 1.5 UX 待办 |
| `docs/需求/35 gateway.md` | §3.0 Follow-up Queue 需求与验收 |
| `docs/需求/35 gateway.design.md` | §3.6 架构与时序 |
| `docs/需求/35-gateway-development.md` | Phase 1.5 任务拆分 |
| `docs/需求/README-development.md` | Gateway 接入度更新 |

## 架构要点（代码现状）

- **编排**：`ChatUsecase.EnqueueUserMessage`（锁 → active run → Steerable / Pending → `PublishMessageQueued`）
- **通知**：`publishMessageQueuedToBus` → `run_status` + `{ status: "queued", hint: "message_queued" }`
- **出队**：`processPendingQueue` → `chatUC.DequeuePendingMessage` → 新 turn
- **待做**：前端 `sending` 阻塞解除；监听 `message_queued`；删除废弃 `ChatService.publishMessageQueued`

## 验证

文档-only；实现验收见 `35-gateway-development.md` Phase 1.5。
