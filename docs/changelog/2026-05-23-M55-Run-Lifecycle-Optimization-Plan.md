# M55 Run 生命周期 — Review 优化计划

> **日期**：2026-05-23 · **模块**：Chat × Channel (M55 Phase R)  
> **Review**：[2026-05-23-M55-Run-Lifecycle-Review.md](../review/2026-05-23-M55-Run-Lifecycle-Review.md)

| CC-R-OPT-05 | `chat_orchestrator_durable.go` 抽出 durable 续跑上下文 | ✅ |
| CC-R-OPT-10 | Jobs→TurnBlock `turn-block--focused` 高亮 2s | ✅ |

## 已交付（2026-05-23 · CC-R-OPT · 续）

| ID | 变更 |
|----|------|
| CC-R-OPT-01 | `session_runs.resume_started_at` + `TryClaimDurableResume` / `ClearResumeClaim` |
| CC-R-OPT-02 | 卡片 callback 校验 `run.session_id` ↔ channel session |
| CC-R-OPT-03 | 硬预算不再预 `MarkPhase(durable)`；escalate 先写 checkpoint |
| CC-R-OPT-06~08 | escalate FlowLog warn · binding/checkpoint 降级 · Jobs scan `agent_id` |

## 摘要

对 CC-R-01~05 / CC-F-02（Phase 1）/ Feishu 卡片 / Jobs→TurnBlock 实现做六维 Review（**76/100，P1**）。功能闭环达标，**稳定性三项**（Worker 幂等、卡片鉴权、硬预算时序）须在下一 sprint 修复后再视为生产就绪。

## Review 结论（相对需求/设计）

| 维度 | 结论 |
|------|------|
| **需求** | CC-R P0–P1 ✅；CC-F-02 当前为「会话快照续跑 Phase 1」，非设计终态 invocation restore |
| **架构** | 分层正确；Worker 缺 claim 与 §6.6 CC-F-01「重启不丢、不重复」冲突 |
| **代码** | Channel ingress 拆分良好；`runSingleAgentViaTRPC` SRP 债务加重 |
| **测试** | 卡片解析单测有；缺 Worker / escalate 集成与 M55-RUN E2E |

## 优化计划（任务 ID）

### P1 — 当前 sprint（~3d）

| ID | 任务 | 状态 |
|----|------|------|
| CC-R-OPT-01 | Durable Worker `TryClaimDurableResume` / `resume_started_at` | ✅ |
| CC-R-OPT-02 | 飞书卡片 callback session ownership 校验 | ✅ |
| CC-R-OPT-03 | 硬预算：checkpoint 先于 `phase=durable` | ✅ |
| CC-R-OPT-04 | CC-F-02 文档诚实化（Phase 1 vs CC-F-02b） | ✅ |

### P2 — 下一 sprint（~2d）

| ID | 任务 | 状态 |
|----|------|------|
| CC-R-OPT-05 | 抽出 `runDurableResumeTurn`（CHAT-R2-03 前置） | 📋 |
| CC-R-OPT-06 | escalate/checkpoint FlowLog warn | ✅ |
| CC-R-OPT-07 | binding 丢失降级读 run/checkpoint | ✅ |
| CC-R-OPT-08 | `ListForJobs` scan `agent_id` | ✅ |
| CC-E2E-RUN-01~04 | Run 生命周期手工验收 | 📋 |

### P0 — 卡 Turn / 入站（2026-05-23 排查）

| ID | 任务 | 状态 |
|----|------|------|
| CC-FIX-TOOL-01~03 | 工具假死 WS + 前端 merge | 📋 |
| CC-FIX-CHANNEL-01 | Durable 完成 → 飞书 outbound | 📋 |
| CC-WEB-REASONING-01 | 思考流式 Store 缓存 | 📋 |

详见 [Stuck-Turn-Inbound-Sync-Analysis.md](./2026-05-23-M55-Stuck-Turn-Inbound-Sync-Analysis.md)。

### P3 / Phase F

| ID | 任务 | 状态 |
|----|------|------|
| CC-R-OPT-10 | Jobs→TurnBlock scroll 高亮 | 📋 |
| CC-R-OPT-11 | IM 平台矩阵（Feishu 卡片 vs 文本） | 📋 |
| CC-F-02b | trpc invocation 级 restore | 📋 Phase F |
| CC-F-01 | 24h `deadline_at` + Worker 超时 | 🚧 合并 OPT-01 |

## 文档同步

- [x] `docs/review/2026-05-23-M55-Run-Lifecycle-Review.md`
- [x] `docs/guides/execution-plan.md` — CC-R-OPT-* · CC-F-02 🟡
- [x] `docs/需求/55-chat-channel-cursor-development.md` — §Phase R-OPT
- [x] `docs/需求/17-channel-development.md` — §M55-RUN

## 验证（优化落地后）

```bash
go test ./internal/service/... -run 'DurableWorker|EscalateSessionRun|BudgetWatcher' -count=1
go test ./internal/biz/... -run SessionRun -count=1
make runtime-boundary
```
