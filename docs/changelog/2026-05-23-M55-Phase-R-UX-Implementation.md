# M55 Phase R-UX — 卡 Turn / 入站同步实现

> **日期**：2026-05-23 · **分析**：[Stuck-Turn-Inbound-Sync-Analysis.md](./2026-05-23-M55-Stuck-Turn-Inbound-Sync-Analysis.md) · **续篇**：[Channel 格式化 / 思考 UX](./2026-05-23-M55-Phase-R-UX-Channel-Format-Reasoning.md)

## 已交付

| ID | 变更 |
|----|------|
| CC-FIX-TOOL-01 | `PublishStuckToolResultEnvelopes` — Turn 结束补发 failed `tool_result` WS |
| CC-FIX-TOOL-02 | `useChatInboundSync` Turn 完成时 `finalizeOrphanToolMessages` + `dropStaleInFlight` hydrate |
| CC-FIX-TOOL-03 | `mergeSessionMessages({ dropStaleInFlight })` |
| CC-FIX-CHANNEL-01 | `NotifyRunCompleted` — Durable resume 完成后飞书 outbound |
| CC-FIX-CHANNEL-02 | `TurnPreviewCoordinator` 无正文时 PATCH「正在思考与执行工具…」 |
| CC-B-06b | Channel `run_status=running` → `focusSessionById`（`channel_auto_focus` 可关） |
| CC-WEB-NOTIFY | `InboundNotificationBell` + `useGlobalInboundNotifications`（MainLayout 全局 WS） |
| CC-WEB-REASONING-01 | `chatStreamingSnapshots` + `applyStreamingSnapshotToSession` |
| CC-WEB-REASONING-02~04 | `ChatReasoningPeek` — 思考/正文分离 · live tail 最后两行 · 单击/滚轮/双击 |
| CC-WEB-SESSION-01 | Channel 入站刷新 Session 列表（`channelInboundSessionRefresh`） |
| CC-CHANNEL-FMT-01~06 | IM 出站格式化 · 思考/正文标签 · 飞书回复 Card 2.0 |
| CC-FEISHU-02 | 升格卡片「取消执行」+ Card 2.0 callback · `CancelSessionRunForCard` ownership |
| CC-UX-01~02 | 超时文案改 `/background`；escalating/durable 时跳过重复 timeout IM |

## 仍待运维

- **CC-FEISHU-OPS-01**：飞书控制台订阅 `card.action.trigger` 并发布应用版本（200340）

## 验证

```bash
go test ./internal/channel/preview/... -count=1
go test ./internal/service/ -run "TurnPreview|CardAction|Format" -count=1
cd web && pnpm test -- mergeSessionMessages envelopeToolCall
go build ./...
```
