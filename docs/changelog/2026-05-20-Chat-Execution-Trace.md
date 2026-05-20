# Chat 执行过程卡片 — P0 实施

> **日期**：2026-05-20  
> **需求**：[1 chat-execution-trace.md](../需求/1%20chat-execution-trace.md) · **设计**：[1 chat-execution-trace.design.md](../需求/1%20chat-execution-trace.design.md)

## 摘要

在 Chat 时间线中以可折叠执行卡片展示 Agent 工具 / Skill / MCP 调用过程；扩展 WS `EnvelopeToolCall` v2 元数据，前后端以 `act-{tool_call_id}` 稳定 upsert。

## 后端

- `internal/event/envelope.go`：`EnvelopeToolCall` 增加 `activity_kind`、`display_label`、`icon_key`、`summary`、时间戳等 v2 字段
- `internal/agent/activity_meta.go`：ActivityKind 分类、展示名/图标/摘要、JSON 脱敏
- `internal/agent/event_projector.go`：tool_call / tool_result 投影填充 v2 元数据；tool 调用缓存计算 `duration_ms`；从 extensions 读取参数

## 前端

- `web/src/features/chat/envelopeToolCall.ts`：v2 映射、`mergeToolEvents`、`act-{id}` upsert
- `web/src/features/chat/activityPresentation.ts`：图标/标签 fallback
- `web/src/components/chat/ChatToolCallCard.vue`：默认折叠 `q-expansion-item`、执行中/耗时/状态图标
- `options_json.schema`：`chat.activity/v1`

## P1 续（同日内）

### 后端

- `internal/chatactivity`：ToolUC catalog `display_name`、Agent 成员名/id；`SessionActivityPersister` 实时 upsert（Service 经 `NewChatStreamConsumeOptions` 注入）
- `internal/agent/activity_persist.go`：`ChatMessageFromToolActivity`（`chat.activity/v1`、id=`act-{tool_call_id}`）
- `internal/data/session_repo.go`：`UpsertChatActivityMessage`
- `internal/agent/event_projector.go`：`text_delta` 仅发送增量 suffix（修复流式重复）
- `StreamConsumeOptions` 注入单 Agent / Team turn

### 前端

- `mergeSessionMessages.ts`：刷新时保留 `ws-stream-*`、running 卡片
- `ChatToolCallCard.vue`：Team 成员标识（仅 Team 会话）
- `app.ts`：`loadMessages` 合并服务端与本地 ephemeral 行

## 审查修复（同日）

见 [2026-05-20-Chat-Execution-Trace-Review-Fixes.md](./2026-05-20-Chat-Execution-Trace-Review-Fixes.md)

## 测试

- `go test ./internal/agent/... ./internal/chatactivity/...`
- `pnpm test -- src/features/chat/__tests__`
