# M55 · Channel Chat P1–P2 优化

> **日期**：2026-05-23 · **模块**：Chat × Channel (M55)

## 摘要

落实 [m55-chat-channel-enterprise-blueprint.md](../需求/55-chat-channel-cursor-solution.md#9-附录企业级蓝图与-ai-落地指南) §12.2 中 **P1** 与可快速落地的 **P2** 项。

## P1 已交付

| ID | 变更 |
|----|------|
| CC-B-07 | `MergeSourceIntoUserOptionsJSON` + `trpc_turn` 写入 `source`；`UserBubble` / `TurnBlock` 来源 chip |
| CC-C-06 | `runner_completion` 仅 `after_revision` 增量 hydrate，避免全量 replace |
| CC-C-07 | Session 顶栏 `N 条 · rev · WS · ctx%` 诊断行 |
| CC-C-05 | 虚拟列表 slice 48、行高估算 200px |
| CC-HOT-02 | `DeleteSession` 时清理 `channel_peer_session` |
| CC-E2E-01 | [channel-im-preview-e2e.md](../需求/17-channel-development.md#12-im-preview--e2e-验收清单lt-0107) 追加 M55 手工验收表 |

## P2 已交付（interim）

| ID | 变更 |
|----|------|
| CC-F-01 | `ChannelAsyncJobWatchMax = 24h` 取代 `asyncWatchTimeout = 2h`（进程内 watch；durable worker 仍待排） |

## 仍待排（P2+）

- CC-E-01 `@` 引用 · CC-E-03 Apply diff · CHAT-R2-03 TurnExecutor · CC-F-01 持久化 `deadline_at` + Worker

## 验证

```bash
go test ./internal/agent/ -run MergeSource -count=1
go test ./internal/service/ -run 'EnsureChannelSession|afterRevision' -count=1
cd web && pnpm test -- messageSourceMeta
```
