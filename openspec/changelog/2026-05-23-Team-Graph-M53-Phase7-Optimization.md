# 2026-05-23 Team Graph M53 Phase 7 — 优化修复（Review P1）

## 摘要

按 [`docs/review/2026-05-23-Team-Graph-M53-Phase7-Review.md`](../review/2026-05-23-Team-Graph-M53-Phase7-Review.md) P1 清单修复 Graph HITL 首跑 step 语义、Coordinator 超时/内存、Resume finalize 指标与 FP-04 Resolve 校验。

## 变更

| ID | 修复 |
|----|------|
| BL-01 | `runner_team_trpc.go`：HITL defer **先于** bulk `persistStep`；defer 时跳过 bulk |
| BL-02 | `team_graph_run_coordinator.go`：watch 30min 超时 → failed finalize |
| ARCH-02 | finalize 后 `evictSession` 清理 coordinator sessions |
| BE-01 | `PersistResumeGraphStep` 失败 FlowLog |
| BE-02 | `FinalizeGraphTeamRun` 从 steps 聚合 token / output_preview |
| BE-05 | `ResolveTaskDeadLetter` 仅 pending 可 resolve；resolved 幂等 |

## 仍待 P2

- BL-03：Graph 非 HITL 完成路径的 per-member step 语义
- TG-RT-PARITY：run 级 E2E（当前 build + key 对齐）
- ARCH-01：Finisher 接口 DTO 化

## 验证

```bash
go test ./internal/team/... ./internal/service/... -run 'TeamGraph|TaskDeadLetter|Finisher|Parity' -count=1
go build ./...
```
