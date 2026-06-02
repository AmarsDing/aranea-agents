# Chat 执行过程卡片 — P3 优化落地

> **日期**：2026-05-20  
> **需求**：[1 chat-execution-trace.md](../需求/1%20chat-execution-trace.md)

## 摘要

完成 P2 遗留的三项 UX / 一致性优化：长对话虚拟滚动、Team 取消路径与 Chat Stop 对齐、执行卡片展开区复制审计提示。

## 后端

- `internal/service/run_status_publish.go`：抽取 `PublishRunStatus`、`CancelSessionRunSideEffects`（WS `run_status` + `chatactivity.CancelRunningActivityMessages`）
- `internal/service/team.go`：`CancelTeamRun` 注入 `event.Bus`，取消后调用 `CancelSessionRunSideEffects`
- `internal/service/chat.go`：`publishRunStatus` 复用 `PublishRunStatus`
- `internal/service/team_cancel_test.go`：断言取消 Team Run 发布 `run_status=cancelled`

## 前端

- `ChatMessagePanel.vue`：消息数 ≥50 时启用 `q-virtual-scroll`；`<50` 仍用普通列表；`scrollToBottom` 虚拟模式下 `scrollTo(lastIndex)`
- `ChatMessageRow.vue` + `useChatMessageRow.ts` + `chatMessageMarkdown.ts`：单条消息行组件化，供虚拟/非虚拟列表复用
- `chatListVirtual.ts`：`CHAT_VIRTUAL_SCROLL_THRESHOLD = 50`，`CHAT_VIRTUAL_ROW_ESTIMATE = 132`
- `ChatExecutionCard.vue`：展开且展示参数/结果时 footer 显示 `chat.activity.copyAuditHint`

## 测试

- `go test ./internal/service/...`
- `pnpm test -- src/features/chat/__tests__`
- `pnpm build`
