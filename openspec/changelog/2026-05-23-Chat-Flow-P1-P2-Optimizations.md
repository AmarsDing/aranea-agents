# Chat Flow P1–P2 优化（2026-05-23）

## 背景

对话流深度审查（Web/WS/HTTP/Channel/Team）发现 WS 投影与 DB 聚合在 delta 处理上不一致、Team 缺首字节超时、Follow-up 队列前端未走 `enqueue_message`、`OnReplyDelta` 死路径等问题。本文记录 P1–P2 收口改动。

## 改动摘要

| ID | 问题 | 修复 |
|----|------|------|
| FLOW-P1-01 | `EventProjector` 对 partial delta `TrimSpace`，与 `ConsumeEventStream` 不一致 | 抽取 `ChoiceStreamContent`（`internal/agent/choice_stream.go`），投影与聚合共用 |
| FLOW-P1-02 | Team `ConsumeEventStream` 无首字节超时 | `runner_team_trpc.go` 使用 `ConsumeEventStreamWithFirstByte` + `DefaultFirstByteTimeout` |
| FLOW-P1-03 | WS 无 E2E 协议测试 | `ws_protocol_test.go`：`user_message` 失败时 `request_id` 关联 |
| FLOW-P1-04 | `ConsumeEventStream` 职责过重 | 抽取 `turnStreamConsumer`（`stream_consumer.go`） |
| FLOW-P1-05 | 活跃 run 时前端仍发 `user_message` | `useChatSender.ts` 改发 `enqueue_message` |
| FLOW-P2-02 | `OnReplyDelta` / `RunNativeTurnStreaming` 死路径 | 移除 `trpc_turn` 回调接线；API 标记 Deprecated |
| FLOW-P2-03 | WS 上行无 `request_id` 关联 | 前后端传递 `request_id`；错误 envelope 回传 |
| FLOW-P2-04 | `processPendingQueue` / WS handler 硬编码 600s | 统一 `DefaultTurnTimeout`（5m） |
| FLOW-P2-05 | 文档未同步 | 本 changelog + `docs/review/01-chat-review.md` 更新 |

## 架构说明

```
trpc events
  → turnStreamConsumer（投影 + 聚合 + tool 跟踪）
      → EventProjector.Project（ChoiceStreamContent）
      → EventBus → WS / TurnPreviewCoordinator
  → 持久化 assistant + activity
```

Channel IM 预览路径：**EventBus → TurnPreviewCoordinator**，不再经 `OnReplyDelta`。

## 验证

```bash
go test ./internal/agent/... ./internal/server/... ./internal/team/...
cd web && pnpm test -- streamContentPatch envelopeToolCall mergeSessionMessages
```
