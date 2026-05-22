# Channel IM Preview & 飞书 Tool Card — Code Review

> **依据**：[`docs/README.md`](../README.md) · [`docs/review/README.md`](./README.md) · [`17 channel.design.md` §12.9](../需求/17%20channel.design.md)  
> **审查时间**：2026-05-23 · **优化闭合**：2026-05-23（P0–P3 全量）

---

## 综合评级

| 指标 | 结果 |
|------|------|
| **总分** | **90 / 100** |
| **风险等级** | **P2**（真实飞书 tenant E2E 手工验收） |

---

## 优化闭合项

| ID | 状态 | 落点 |
|----|------|------|
| IM-P0-01 心跳阻塞 consume | ✅ | `Start` select 合并 heartbeat ticker |
| IM-P1-E2E | ✅ | [channel-im-preview-e2e.md](../guides/channel-im-preview-e2e.md) LT-01–07 |
| IM-P2-HTTP-BLOCK | ✅ | Card `safego` + `cardSerial` + segment 快照 |
| IM-P2-CRED-SILENT | ✅ | `buildTurnPreviewDelivery` FlowLog warn |
| IM-P2-CARD-UPDATE | ✅ | `lark.UpsertToolCard` + `toolCardMessageIDs` |
| IM-P3-DEDUP | ✅ | `preview/tool_status.go` |
| IM-P3-FLOW-CONST | ✅ | `service/channel_flow_steps.go` |
| IM-P3-URL-ENCODE | ✅ | `BuildSessionWebURL` `url.QueryEscape` |

---

## 验证

```bash
go test ./internal/channel/preview/... ./internal/channel/lark/... -count=1
go test ./internal/service/ -run "TurnPreview|Interactive" -count=1
```

手工：见 [channel-im-preview-e2e.md](../guides/channel-im-preview-e2e.md)
