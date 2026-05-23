# 2026-05-23 Team Graph M53 Phase 7 — P2 优化

## 摘要

按 Review P2 清单：Graph 首跑 step 事件驱动、Finisher DTO、死信 UI 增强、Circuit breaker half-open、TG-RT-PARITY diff 文档。

## 变更

| ID | 修复 |
|----|------|
| BL-03 | `StartGraphStepWatch`（steps-only watch）；Graph 路径跳过 bulk persist；anchor fallback |
| ARCH-01 | `GraphRunStepContext` + `PersistGraphRunStep` / `FinalizeGraphTeamRun` DTO |
| FP-04-UI | 死信 Observatory 跳转 + payload JSON 展开 |
| FP-02 | `breakerHalfOpen` 状态转换 |
| TG-RT-PARITY | [`docs/guides/tg-rt-parity-diff.md`](../guides/tg-rt-parity-diff.md) |

## 验证

```bash
go test ./internal/team/... ./internal/graph/trpc/... -count=1
go build ./...
```
