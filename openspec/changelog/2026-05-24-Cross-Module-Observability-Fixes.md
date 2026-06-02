# 2026-05-24 跨模块可观测性与 Chat 契约修复

## 背景

对照 Agent / Channel / Chat 三模块评审，落地 P0–P2 修复项：Token 语义、错误模型、TurnObserver、反馈闭环、知识库 per-turn 作用域、编排通知、深链与压缩通知。

## 变更摘要

### Token 语义（P0）

- `accumulateStreamUsage`：ReAct 多轮 LLM 时 **prompt 取 max、completion 按轮累加**。
- `runner_completion.usage` 增加 `max_tokens`（上下文窗口）、`turn_total_tokens`（本轮累计）。
- Chat Composer 区分 **「当前上下文占用 %」** 与 **「累计 tokens」** 文案。

### EnvelopeError（P1）

- `EnvelopeError` 增加 `code`、`hint`；HTTP `TurnErrorCode` 可映射为 WS 错误载荷。
- 前端 `error` 事件展示 `message + hint`。

### TurnObserver（P1）

- 新增 `internal/event/turn_observer.go`：`EventProjector` 发布 chat envelope 经 `TurnObserver.PublishChat` 统一出口（FlowLog 仍走 `TraceEmitter`）。

### 消息反馈（P1）

- `POST /v1/chat/messages/{message_id}/feedback`（rating: positive | negative）。
- EventBus `user_feedback` envelope；持久化至 `messages.options_json.feedback`。
- Chat 助手消息 👍/👎 按钮。

### 知识库 per-turn（P1）

- `SendMessageOptions.knowledge_bases[]` → `knowledge_search` 工具 collection 白名单。
- Chat Composer 多选知识库（有集合时显示）。

### 编排 Chat 感知（P1）

- WS `transfer` / `intent_pass` 触发 info 通知（转接、意图识别）。

### 深链与压缩（P2）

- 会话详情「继续会话」→ `/chat?session={id}`（原有 query 深链已支持）。
- 自动压缩完成后写入 system 消息 + `text_done` envelope（`metadata.kind=system.session.compress`）。

### 反馈闭环扩展（Phase 3）

- **Monitor**：`EventBusSideConsumers` 订阅 `user_feedback` → `monitor_events`（`event_key=chat.user_feedback`，negative 为 `warning`）。
- **Memory**：`FeedbackMemoryEnqueuer` + `AutoMemoryWorker.extractFeedback` 将带 comment 的反馈写入 preference 记忆（topics: `feedback`, `preference`）。
- **Turn 失败 WS**：`publishTurnFailure` 统一 HTTP turn 失败路径发布 `EnvelopeTypeError`，经 `envelopeErrorFromTurn` 填充 `code`/`hint`（含 team turn、await resume、pending queue）。

## 验证

```bash
go test ./internal/biz/... ./internal/agent/... ./internal/service/... -count=1
go build ./cmd/admin/...
```

## 文档

- `docs/需求/1 chat.design.md` §5.5 EnvelopeError / Usage / Feedback 已同步。
