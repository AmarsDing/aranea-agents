# M55 Run 生命周期 — Code Review 与优化计划

> **依据**：[`docs/README.md`](../README.md) · [`docs/review/README.md`](./README.md) · [55-chat-channel-cursor-solution.md §2.1/§6.6](../需求/55-chat-channel-cursor-solution.md) · [55-chat-channel-cursor-development.md §Phase R](../需求/55-chat-channel-cursor-development.md)  
> **审查时间**：2026-05-23 · **范围**：CC-R-01~05 · CC-F-02（Phase 1）· Feishu 卡片 · Jobs→TurnBlock  
> **Changelog**：[2026-05-23-M55-Run-Lifecycle-Optimization-Plan.md](../changelog/2026-05-23-M55-Run-Lifecycle-Optimization-Plan.md)

---

## 综合评级

| 指标 | 结果 |
|------|------|
| **总分** | **76 / 100** |
| **风险等级** | **P1**（Worker 幂等 · 卡片鉴权 · 硬预算 phase/checkpoint 时序） |
| **需求闭环** | P0–P1 用户故事可演示；CC-F-02 真·invocation restore 未达设计终态 |

### 六维得分

| 维度 | 权重 | 得分 | 说明 |
|------|------|------|------|
| 需求符合度 | 20 | 17 | CC-R 主链路 ✅；CC-F-02 文档过度承诺 |
| 架构一致性 | 25 | 21 | biz/service 分层正确；orchestrator 继续膨胀 |
| 后端实现质量 | 20 | 14 | Worker 无 claim；错误吞没；卡片无 ownership |
| 前端实现质量 | 15 | 12 | focusTurn 链路清晰；缺高亮与 Team 降级 |
| 测试与验证 | 10 | 5 | 卡片解析单测有；缺 Run E2E / Worker 集成 |
| 文档一致性 | 10 | 7 | execution-plan 与实现有 CC-F-02 偏差 |

---

## 优化项复核（需求 × 设计 × 代码）

> 对上一轮 Review 中 RL-P* 条目逐项对照 **M55 蓝图**、**Channel 设计惯例** 与 **当前落点**，给出是否纳入计划、优先级与任务 ID。

| 原 ID | 结论 | 需求/设计依据 | 代码现状 | 计划 ID | 优先级 |
|-------|------|---------------|----------|---------|--------|
| RL-P1-01 Worker 重复续跑 | **采纳** | §2.1 Job 平面「进程重启后 Job 不丢」；§6.6 CC-F-01 合并 R-03 要求 Worker **可重入、不重复执行**；Channel 入站已有 `TryClaimInbound` 幂等范式 | `ListDurablePending` 每 5s 扫 `phase=durable`；`ResumeDurableSessionRun` 仅 `HasActiveRun` 门禁 + `safego` 即发 | **CC-R-OPT-01** | **P1** |
| RL-P1-02 卡片 callback 无鉴权 | **采纳** | [17 channel.design.md](../需求/17%20channel.design.md) 入站访问控制；卡片为 IM 新攻击面 | `EscalateSessionRun(id)` 不校验 run 与 channel session / operator peer | **CC-R-OPT-02** | **P1** |
| RL-P1-03 硬预算 phase 先于 checkpoint | **采纳** | Run 状态机：Worker 仅应在 **checkpoint 就绪** 后 resume；否则违背 CC-R-03 验收 | `fireHard` 先 `MarkPhase(Durable)` 再调 `escalateSessionRunToDurable` | **CC-R-OPT-03** | **P1** |
| RL-P1-04 CC-F-02 名实不符 | **采纳（文档 + 分期）** | Phase F CC-F-02 设计为 Graph/trpc **checkpoint restore**；当前为 **Phase 1 会话快照续跑** | 仍注入 `DurableResumePrompt()`；`TrpcInvocationID` 存而未用于 trpc RunOption | **CC-R-OPT-04** 文档 · **CC-F-02b** 实现 | **P1 文档 / P2 实现** |
| RL-P1-05 orchestrator 膨胀 | **部分采纳** | §6.7 CHAT-R2-03 TurnExecutor 为 P3 横切；不应阻塞 P1 稳定性 | `runSingleAgentViaTRPC` durable 分支 ~200 行 | **CC-R-OPT-05** 抽子函数 · **CHAT-R2-03** 全量 | **P2 / P3** |
| RL-P2-01 错误吞没 | **采纳** | §10 守则：FlowLog 为真相 | escalate/checkpoint/notify 多处 `_, _ =` | **CC-R-OPT-06** | **P2** |
| RL-P2-02 binding 仅内存 | **降级为 P2** | 重启后 escalate 应靠 DB；checkpoint payload 已含 dialog/provider/model | `sessionRunBindings sync.Map`；checkpoint 创建时 binding 为空则字段缺失 | **CC-R-OPT-07** | **P2** |
| RL-P2-03 agent_id 未 scan | **采纳** | CC-D-01 Jobs 按 agent 过滤 | `ListForJobs` WHERE `r.agent_id` 但 SELECT 未含列 | **CC-R-OPT-08** | **P2** |
| RL-P2-04 ingressFirstNonEmpty 耦合 | **采纳** | 纯可读性 | orchestrator 引用 ingress 命名 helper | **CC-R-OPT-09** | **P3** |
| RL-P2-05 前端 Jobs 跳转 UX | **采纳** | §12.2 CC-D-05 Job↔Turn 联动 | 无 scroll 高亮；Team 无 TurnBlock | **CC-R-OPT-10** | **P3** |
| RL-P2-06 非 Feishu 仅文本 | **文档化** | 产品可接受 | `runEscalation` 平台分支 | **CC-R-OPT-11** | **P3** |
| RL-P3-* | ** backlog** | 不影响可用性 | — | 并入 CC-E2E / UX polish | **P3** |

### 不纳入本轮（已有设计排期）

| 项 | 原因 |
|----|------|
| CC-F-01 24h `deadline_at` 持久化 + 独立 runtime worker | Phase F sprint；与 CC-R-OPT-01 claim 可同 PR 预埋列 |
| CHAT-R2-03 全量 TurnExecutor | 蓝图 §6.7 明确 Phase C 后再做，避免与稳定性修复互阻 |
| Graph Pregel checkpoint resume | CC-F-02 范围外；依赖 Graph `ResumeExecution` |

---

## 优化计划（执行顺序）

### Sprint 1 — 稳定性（P1，~3d）

| ID | 任务 | 验收 | 代码锚点 |
|----|------|------|----------|
| **CC-R-OPT-01** | Durable Worker **Claim 幂等** | 同一 `session_run_id` 并发 poll 仅一次 resume；admin 重启后不叠 goroutine | 新增 `resume_started_at` / `TryClaimDurableResume`；`session_run_durable_worker.go` · `session_run_repo.go` |
| **CC-R-OPT-03** | 硬预算 **checkpoint 先于 durable phase** | 硬预算触发后 Worker 首次 scan 必能 `GetCheckpoint` | `session_run_budget.go` 移除 hard 路径预 MarkPhase；或 escalate 内 CAS `escalating→durable` |
| **CC-R-OPT-02** | 飞书卡片 **ownership 校验** | 他人 run_id 回调 toast 拒绝；同 channel session 允许 | `channel_ingress_card_action.go` · `EscalateSessionRun` 增 session 校验 |
| **CC-R-OPT-04** | **文档诚实化** CC-F-02 | execution-plan / development 区分 Phase 1 vs 2b | 本文 + changelog + execution-plan |

**单测（Sprint 1 必带）**：

- `TestSessionRunDurableWorker_claimIdempotent`
- `TestEscalateSessionRun_cardActionOwnership`
- `TestBudgetWatcher_hardBudgetCheckpointOrder`

### Sprint 2 — 质量与数据（P2，~2d）

| ID | 任务 | 验收 | 代码锚点 |
|----|------|------|----------|
| **CC-R-OPT-06** | escalate 路径 **FlowLog warn** | checkpoint 失败、MarkPhase 失败可 Monitor 检索 | `chat_orchestrator_session_run.go` · `session_run_escalation_notifier.go` |
| **CC-R-OPT-07** | binding 丢失 **降级读 run/checkpoint** | 进程重启后 hard escalate 仍写出完整 payload | `escalateSessionRunToDurable` fallback `run.AgentID` + 空字段告警 |
| **CC-R-OPT-08** | Jobs **agent_id scan** | `ListForJobs?agent_id=` 返回行含 AgentID | `session_run_repo.go` scanSessionRunRows |
| **CC-R-OPT-05** | 抽出 **`runDurableResumeTurn`** | `runSingleAgentViaTRPC` 不增新分支；行为不变 | `chat_orchestrator_turn.go` 或 `chat_orchestrator_durable.go` |

### Sprint 3 — 体验与 E2E（P2–P3，~2d）

| ID | 任务 | 验收 | 代码锚点 |
|----|------|------|----------|
| **CC-E2E-RUN-01~04** | Run 生命周期手工清单 | 见 [17-channel-development.md §M55-RUN](../需求/17-channel-development.md) | — |
| **CC-R-OPT-10** | TurnBlock **scroll 高亮** | Jobs 跳转后 2s 高亮 `data-turn-id` | `ChatMessagePanel.vue` · `useChatMessageScroll.ts` |
| **CC-R-OPT-11** | IM 平台矩阵文档 | Feishu 卡片 vs 文本 | `17-channel-development.md` · `55-chat-channel-cursor-development.md` |

### Phase F 延续（P2，独立 sprint）

| ID | 任务 | 说明 |
|----|------|------|
| **CC-F-02b** | trpc **invocation 级** restore | 对接 `trpcagent.WithInvocationID`（或框架等价 API）；替换合成 prompt |
| **CC-F-01** | 24h deadline 持久化 | `deadline_at` 列 + 超时 Fail；合并 CC-R-OPT-01 claim 列 |

---

## 架构与 SRP 评估（摘要）

**保留的设计决策**

- `SessionRun` / checkpoint 在 biz+data；`ChatOrchestrator` 编排；Channel 经 `ChatService` 窄接口 — 符合红线。
- `event.WithDurableResume` 上下文传递 — 与 Envelope 模式一致，优于 proto 膨胀。
- Channel ingress 按类型拆文件 — 与 `cancel` / `background` 一致，SRP 良好。

**待收敛**

- **Worker = 调度器**：需 DB claim，不能仅依赖内存 `HasActiveRun`（与 Graph task `claimed_at` 同思路）。
- **Turn 编排**：durable 分支应最小提取；全量 TurnExecutor 仍归 CHAT-R2-03。

---

## 影响域

| 层 | 文件/契约 |
|----|-----------|
| Schema | `session_runs.resume_started_at`（OPT-01）；可选 `deadline_at`（F-01） |
| biz/data | `session_run_repo.go` · claim API |
| service | worker · budget · escalate · card_action |
| Channel | 飞书 `card.action.trigger` 订阅（运维） |
| Web | Jobs 跳转高亮（OPT-10） |
| 文档 | execution-plan · M55 development · 17-channel E2E |

---

## 验证矩阵

```bash
# Sprint 1 后
go test ./internal/biz/... -run 'SessionRun|Budget' -count=1
go test ./internal/service/... -run 'DurableWorker|EscalateSessionRun|CardAction' -count=1
go test ./internal/channel/lark/... ./internal/channel/preview/... -count=1
make runtime-boundary
```

手工： [17-channel-development.md §M55-RUN](../需求/17-channel-development.md)

---

## 文档同步清单

| 文档 | 更新 |
|------|------|
| [execution-plan.md §迭代 CC](../guides/execution-plan.md) | CC-R-OPT-* · CC-F-02 → 🟡 |
| [55-chat-channel-cursor-development.md](../需求/55-chat-channel-cursor-development.md) | §2 现状 · §Phase R-OPT |
| [17-channel-development.md](../需求/17-channel-development.md) | M55-RUN E2E · card.action.trigger |
| [docs/review/README.md](./README.md) | 本 Review 链接 |
