# DECO-01 — Channel/Web 同步 Holistic Fix

> **日期**：2026-05-24  
> **Review**：[2026-05-24-DECO-01-Channel-Sync-Holistic-Fix-Review.md](../review/2026-05-24-DECO-01-Channel-Sync-Holistic-Fix-Review.md)（**78/100 · P1 开放项**）  
> **前置**：[E2E 归档](./2026-05-24-DECO-01-Feishu-Web-E2E-Archive.md) · FIX-D-01~05 · 回归「助手回复完全消失」

---

## 背景

在 FIX-D-01~05（跨 Agent focus、增量 merge、toast）之后，出现新回归：

- 助手回复**完全消失**
- 流式 thinking/tools 曾「一次性出现」
- 多次单点 patch（`channelLiveTurn` 推迟 session WS）引入更多竞态

**根因**：`envRev === localRev` 时跳过 hydrate + `dropStaleInFlight` 删除 `ws-stream-*` + 空 `afterRevision` API → 无持久化助手行。

---

## 变更摘要

| 文件 | 变更 |
|------|------|
| `useChatInboundSync.ts` | 重写：`ownsEnvelope` 含 channel stream/complete；`finalizeTurn` 强制全量 hydrate；per-session writer；删除 live-turn 门控 |
| `useChatStreamManager.ts` | 恢复 session WS；`lastEventId` ← `channelWsCursor`；Agent reload 改全量 merge |
| `useChatEntityNav.ts` | focus 不再 `clearSessionMessages`；始终 `loadMessages` |
| `messageStore.ts` | `mergeIncrementalSessionMessages`；空增量 + `dropStaleInFlight` fallback 全量 |
| `channelWsCursor.ts` | **新增** — 记录 global hub 最后 envelope id |
| `channelLiveTurn.ts` | **删除** — 由 cursor 替代 defer |
| `mergeSessionMessages.ts` | `mergeIncrementalSessionMessages`（FIX-D-03 延续） |

---

## 开放问题（Review 登记）

> 完整条目见 [Review §3](../review/2026-05-24-DECO-01-Channel-Sync-Holistic-Fix-Review.md#3-问题清单待办)

| ID | 级别 | 问题 |
|----|------|------|
| DECO-R-P1-01 | P1 | Global hub 与 session WS 对 Web turn **双 patch** stream |
| DECO-R-P1-02 | P1 | turn complete **双 hydrate**（inbound + runner_completion） |
| DECO-R-P2-01 | P2 | `await focusChannelSession` 阻塞 envelope 链 |
| DECO-R-P2-02 | P2 | Team `reloadTeamAfterCompletion` 与 Agent 不对称 |
| DECO-R-P2-03 | P2 | 缺 `useChatInboundSync` 集成测 |

---

## 验证

```bash
cd web && pnpm test -- channelInboundSession mergeSessionMessages channelWsCursor --run
go test ./internal/service/ -run TestDECO01_SessionRevisionChannelToWebSync -count=1
```

**手工**：复跑 [E2E 归档](./2026-05-24-DECO-01-Feishu-Web-E2E-Archive.md) T1–T4。

---

## 相关文档

- [17-channel-development.md §14 DECO](../需求/17-channel-development.md#14-phase-deco--四层架构解耦deco)
- [55-chat-channel-cursor-development.md](../需求/55-chat-channel-cursor-development.md)
- [Chat-Flow-Full-Review §2.5](../review/2026-05-23-Chat-Flow-Full-Review.md#25-前端对话-ux)
