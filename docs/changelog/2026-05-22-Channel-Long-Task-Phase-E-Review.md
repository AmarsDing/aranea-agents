# Channel 长任务 Phase E — Review 跟进（2026-05-22）

## 摘要

闭合 Phase E Code Review 四项跟进：QQ Webhook 异步入站、微信被动模式长任务边界、`async_queued` Job 保护、IM 错误文案去敏感化，并补充单测与文档。

## 变更

### P1 — 入站路径对齐

| 项 | 变更 |
|----|------|
| QQ Webhook | `ProcessInbound` → `processInboundHTTP`（200 后后台 Turn，保留 DispatchACK） |
| 微信被动 | 保留同步 XML 回复；`processInboundCore` 支持 `TurnOutcomeQueued`；UI 在 `!active_mode` 时隐藏 LONG TASK 字段 |
| biz | `ChannelSupportsLongTaskIngress` / `ParseWeChatActiveMode` |

### P2 — Job / 错误出站

| 项 | 变更 |
|----|------|
| 幂等 | `ON CONFLICT` 保护 `async_queued`；`IsChannelTurnJobIdempotentLockedStatus` |
| Async watch | `watchAsyncGraphCompletion` 完成 → `completed` + IM；超时 → `timeout` + IM 错误 |
| IM 错误 | 非超时统一 `channelTurnErrorGenericMsg`，细节仅 `recordDelivery` / FlowLog |

### P3 — 测试与文档

- 单测：`channel_ingress_accept_test.go`、Job 幂等、`ChannelSupportsLongTaskIngress`、错误文案
- 更新 `docs/review/17-channel-review.md` 入站路径图
- 本 changelog

## 影响域

- **Channel ingress**（QQ / 微信被动 / async graph watch）
- **前端** Channel 编辑器 LONG TASK 分区条件显示
- **不影响** Web Chat、gRPC Chat、Runtime WS 同步 `ProcessInbound` 语义

## 验证

```bash
go build ./...
go test ./internal/biz/... ./internal/data/... ./internal/service/... -count=1
make runtime-boundary
```

## P2 跟进（第二轮）

| 项 | 变更 |
|----|------|
| QQ DispatchACK | `processInboundHTTP` 返回 `inboundHTTPResult`；handler 统一写 200/500 + ACK |
| 微信被动 | `gateInboundBeforeTurn` + `processWeChatPassiveInbound`（幂等/访问/cancel XML） |
| Async watch | Job 更新使用 `context.Background()`；Graph 失败/超时；Cron 轮询终态 |
