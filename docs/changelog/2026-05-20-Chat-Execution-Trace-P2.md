# Chat 执行过程卡片 — P2 优化落地

> **日期**：2026-05-20  
> **需求**：[1 chat-execution-trace.md](../需求/1%20chat-execution-trace.md)

## 摘要

完成设计文档 P2 项与审查遗留项：组件重命名、StopGeneration 取消态持久化、前端脱敏与取消态 UI、元数据展示。

## 后端

- `internal/agent/activity_persist.go`：`CancelledActivityMessage`
- `internal/chatactivity/cancel.go`：`CancelRunningActivityMessages`
- `internal/service/chat.go`：`StopGeneration` 后取消 `tool_running` 卡片

## 前端

- `ChatExecutionCard.vue`（`ChatToolCallCard.vue` 保留兼容别名）
- `cancelRunningToolMessages`：`run_status=cancelled` 时本地 running 卡片 → cancelled
- `maskSensitiveJSON`：参数/结果展示二次脱敏
- 取消态图标/边框、`run_id`/`trace_id` 元数据区
- `mergeSessionMessages`：保留 `tool_blocked` 进行中行

## 测试

- `go test ./internal/agent/... ./internal/chatactivity/...`
- `pnpm test -- src/features/chat/__tests__`

## 仍为后续项（未在本批实施）

| 项 | 原因 |
|----|------|
| 单轮 ≥50 卡虚拟滚动 | 需改 `ChatMessagePanel` 列表结构，影响面大 |
| `activity_*` 独立 Envelope 类型 | 设计评估：tool_call v2 尚未过载 |
| 详情复制审计提示 | P2 安全 UX，非功能阻塞 |
| Team `CancelTeamRun` 发布 `run_status` | 与 Chat Stop 路径不一致，可单独迭代 |
