# Chat 执行过程卡片 — P0/P1 审查修复

> **日期**：2026-05-20  
> **需求**：[1 chat-execution-trace.md](../需求/1%20chat-execution-trace.md)

## 修复摘要

按开发规范完成 Code Review 项修复：活动卡片落库失败可观测、持久化与投影解耦、agent_key/agent_id 语义修正、blocked/cancelled 独立状态、Team 成员标签条件展示、消息合并按 turn_index 排序。

## 后端

- **持久化解耦**：`EventProjector` 仅投影 + 发布；`ConsumeEventStream` 在 turn 循环中调用 `PublishActivityEnvelopes`，失败写入 `CtxFlowLogWarn`
- **Service 边界**：catalog 解析与 session upsert 迁至 `internal/chatactivity`（`internal/service` 薄封装；Team 同路径注入，避免 service↔team 循环依赖）
- **删除** `internal/agent/activity_bridge.go`
- **`activity_persist.go`**：`ActivityMessageID` 空 id 返回 error；`ActivityMessageStatus` 区分 `tool_blocked` / `tool_cancelled`；`agent_key` 仅用 author，不再回退 `AgentDisplayName`
- **`envelope.go`**：`EnvelopeToolCall.agent_id`
- **`activity_meta.go`**：`ResolveAgentID`；`CatalogLookupKeysForRuntimeName` 导出
- **`event_projector.go`**：去除重复 resolver；member_delta 增量流式（`visibleStreamDelta`）

## 前端

- `mergeSessionMessages`：先 `turn_index` 再 `created_at`
- `envelopeToolCall`：映射 `agent_id`
- `toolEventMarkdown`：`tool_blocked` / `tool_cancelled` 状态
- `ChatToolCallCard`：`showMemberLabel`（Team 会话才显示成员行）；blocked 不再归入 failed 样式
- `ChatPage` → `ChatMessagePanel`：`isTeamSession`

## 测试

- `go test ./internal/agent/... ./internal/chatactivity/...`
- `pnpm test -- src/features/chat/__tests__`
