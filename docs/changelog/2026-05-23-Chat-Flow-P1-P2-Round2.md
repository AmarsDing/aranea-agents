# Chat Flow P1–P2 第二轮优化（2026-05-23）

## 背景

深度 review 后针对并发 admission、排队处理、Channel queued 误判、前端 Await/双 WS 消费等问题做第二轮收口。

## 改动摘要

| ID | 问题 | 修复 |
|----|------|------|
| FLOW-P1-01 | `processPendingQueue` 无锁、可与新 turn 竞态 | 处理前 `lockSession`；若仍 active 则重新入队 |
| FLOW-P1-02 | placeholder 阶段 Cancel 无效 / 并行 turn | `StoreCancelable` 替代 `StorePlaceholder`；starting 阶段返回 `CHAT_TURN_BUSY` |
| FLOW-P1-03 | 单 Agent 纯 reasoning 判 empty_reply | 与 Team 一致：`displayMarkdown = reply \|\| reasoning` |
| FLOW-P1-05 | Channel queued 靠 `wasActive && reply==""` 启发式 | `ErrTurnMessageQueued` 显式 sentinel |
| FLOW-P1-06 | `pending-user-*` merge 后不清理 | `dropPendingUserPlaceholders` + `runner_completion` |
| FLOW-P1-07 | Inbound hub 与 session WS 双 patch | 当前 session 的 text_delta/done 由 session WS 独占 |
| FLOW-P1-08 | Team send 后立即 `markSendingDone` | 与 Agent 一致，等 `runner_completion` |
| FLOW-P2-01 | `trpc_turn` 双重 timeout 包装 | 移除第二处 deadline 覆盖 |
| FLOW-P2-02 | WS cancel 重复 error + run_status | WS 仅调 `CancelRun`，状态由 ChatService 发布 |
| FLOW-P2-03 | await resume 吞掉 turn 错误 | 失败写 run_status + error envelope |
| — | 首字节过宽 | `countsAsFirstByte` 仅在内容/tool/error/completion 时标记 |

## 验证

```bash
go test ./internal/agent/... ./internal/server/... ./internal/service/... -count=1
cd web && pnpm exec vue-tsc --noEmit && pnpm test -- mergeSessionMessages
```
